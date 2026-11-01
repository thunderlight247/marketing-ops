#!/usr/bin/env bash
# Boots the whole local stack the dashboard needs: the Go API and the web
# server in front of it, database up and migrations applied. Whenever the web
# server runs locally, the API runs with it — no more "the API at :8081 is not
# running" surprises.
#
#   bash e2e/run-stack.sh http://localhost:8081 3001          # e2e web server
#   bash e2e/run-stack.sh http://localhost:8081 3000 --dev    # next dev
#
# Ports are arguments and nothing is reserved for this project: other apps
# sharing the machine may hold any port. The launcher therefore negotiates:
# the dashboard takes the requested web port or the next free one of
# {3000,3001,3002}; the API takes a port that is free among {8080,8081} or is
# already a marketing-ops API that would accept this dashboard's origin (so a
# foreign server on an API port is never silently paired in). Every picked
# port is reported; CORS and the browser's inlined API base derive from the
# actual choices, so any combination needs no source changes. E2E mode honors
# the requested ports exactly (the Playwright specs read
# PLAYWRIGHT_API_BASE_URL themselves, so silently shifting it would desync
# them) and fails fast with that explanation instead.
#
# A compatible ready API is reused untouched; the script kills only the API
# it started itself when the web server stops. If neither component was up,
# this command brings the database (Docker) and migrations up first, best
# effort.
set -euo pipefail

api_base_url_arg=${1:?API base URL required, e.g. http://localhost:8081}
web_port_arg=${2:?web port required, e.g. 3001}
mode=${3:-e2e}

repo_root=$(cd "$(dirname "$0")"/../../.. && pwd)
api_dir="$repo_root/apps/api"

ready() {
  curl --fail --silent --max-time 2 "$1/api/v1/ready" >/dev/null 2>&1
}

port_open() {
  (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
}

# cors_ok checks that a running API would let a page from this dashboard's
# origin talk to it (CORS echo of our exact origin). Servers from other
# projects can answer /api/v1/ready-shaped requests only by accident; a
# strict preflight keeps them from ever being paired with the dashboard.
cors_ok() { # $1 = API base, $2 = web port
  curl --silent --max-time 3 -X OPTIONS \
    -H "Origin: http://127.0.0.1:$2" \
    -H "Access-Control-Request-Method: GET" \
    -D - -o /dev/null "$1/api/v1/health" 2>/dev/null \
    | grep -qi "access-control-allow-origin: http://127.0.0.1:$2"
}

bring_api_up() { # $1 = base URL the API must answer on
  local base=$1
  # Database first: best effort (Docker), never fatal when absent or down —
  # the API still boots; /health simply reports the database as unreachable
  # until the database appears.
  docker compose \
    --env-file "$repo_root/infra/.env" \
    -f "$repo_root/infra/docker-compose.yml" up -d >/dev/null 2>&1 || true

  # API environment: apps/api/.env is the source of truth (exactly what the
  # Makefile's api-run target sources); when it omits a database URL, derive
  # one from infra/.env's connection facts.
  set -a
  if [[ -f "$api_dir/.env" ]]; then
    # shellcheck disable=SC1091
    . "$api_dir/.env"
  fi
  if [[ "${DATABASE_URL:-}" == "" && -f "$repo_root/infra/.env" ]]; then
    # shellcheck disable=SC1091
    . "$repo_root/infra/.env"
    DATABASE_URL="postgres://${POSTGRES_USER:?Set POSTGRES_* in infra/.env}:${POSTGRES_PASSWORD:?Set POSTGRES_PASSWORD in infra/.env}@127.0.0.1:${POSTGRES_PORT:-5432}/${POSTGRES_DB:?Set POSTGRES_DB in infra/.env}?sslmode=disable"
  fi
  export DATABASE_URL
  : "${APP_ENV:=development}"
  export APP_ENV
  export API_PORT="${base##*:}"
  # Allow every known dashboard port in both host forms (plus whatever was
  # already configured in apps/api/.env). This machine may run several
  # projects at once and several instances of this one; keeping the whole
  # {3000,3001,3002} set allowed means a restarted or second dashboard can
  # reuse the first instance's API instead of colliding. The page origin is
  # http://127.0.0.1:$port, and dropping that form while keeping
  # localhost:$port makes every browser request fail CORS — the API logs the
  # 200s but the dashboard sees nothing.
  local wp origin
  declare -A seen_origin=()
  local origins=""
  for wp in 3000 3001 3002; do
    for origin in "http://localhost:$wp" "http://127.0.0.1:$wp"; do
      [[ -z "${seen_origin[$origin]:-}" ]] || continue
      seen_origin[$origin]=1
      origins="${origins:+${origins},}${origin}"
    done
  done
  for origin in ${CORS_ALLOWED_ORIGINS:-}; do
    origin="${origin// /}"
    [[ -n "$origin" ]] || continue
    [[ -z "${seen_origin[$origin]:-}" ]] || continue
    seen_origin[$origin]=1
    origins="${origins:+${origins},}${origin}"
  done
  export CORS_ALLOWED_ORIGINS="$origins"

  # Migrations are idempotent; if the database is genuinely unreachable the
  # message shows up here instead of as silent e2e connect failures later.
  if ! (cd "$api_dir" && go run ./cmd/migrate up); then
    echo "run-stack: database unreachable, migrations not applied (API /health will say so)" >&2
  fi

  (cd "$api_dir" && go run ./cmd/api) &
  api_pid=$!
  local _attempt
  for _attempt in $(seq 1 60); do
    if ready "$base"; then
      echo "run-stack: API ready at $base"
      return 0
    fi
    if ! kill -0 "$api_pid" 2>/dev/null; then
      echo "run-stack: the API exited before becoming ready on $base" >&2
      return 1
    fi
    sleep 2
  done
  echo "run-stack: the API did not answer $base/api/v1/ready in time" >&2
  return 1
}

api_pid=""
cleanup() {
  if [[ -n "$api_pid" ]]; then
    # go run wraps the actual binary in a child process; stop both.
    pkill -P "$api_pid" 2>/dev/null || true
    kill "$api_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT

# --------------------------------------------------------------- web port ---
# Nothing is reserved: the requested port, then the next free one of the set.
web_port_used="$web_port_arg"
if [[ "$mode" == "--dev" ]]; then
  web_candidates=("$web_port_arg")
  case "$web_port_arg" in
    3000) web_candidates+=(3001 3002) ;;
    3001) web_candidates+=(3000 3002) ;;
    3002) web_candidates+=(3000 3001) ;;
  esac
  web_port_used=""
  for candidate in "${web_candidates[@]}"; do
    if ! port_open "$candidate"; then
      web_port_used=$candidate
      break
    fi
  done
  if [[ "$web_port_used" == "" ]]; then
    echo "run-stack: no free web port among ${web_candidates[*]} — every one is held by another app" >&2
    exit 1
  fi
  if [[ "$web_port_used" != "$web_port_arg" ]]; then
    echo "run-stack: web port $web_port_arg was busy; using $web_port_used instead" >&2
  fi
elif port_open "$web_port_arg"; then
  echo "run-stack: note web port $web_port_arg is already in use; if this run then fails with EADDRINUSE, free that port or set PLAYWRIGHT_WEB_PORT" >&2
fi

# ---------------------------------------------------------------- API port ---
api_port_arg=${api_base_url_arg##*:}
api_candidates=("$api_base_url_arg")
if [[ "$mode" == "--dev" ]]; then
  case "$api_port_arg" in
    8080) api_candidates+=("http://localhost:8081") ;;
    8081) api_candidates+=("http://localhost:8080") ;;
  esac
fi

api_base_used=""
for candidate in "${api_candidates[@]}"; do
  if ready "$candidate"; then
    if cors_ok "$candidate" "$web_port_used"; then
      echo "run-stack: reusing the ready API at $candidate" >&2
      api_base_used=$candidate
      break
    fi
    echo "run-stack: server on ${candidate##*:} is ready but does not allow a dashboard from port $web_port_used; skipping it" >&2
    continue
  fi
  candidate_port=${candidate##*:}
  if port_open "$candidate_port"; then
    echo "run-stack: port $candidate_port is taken, but not by a ready marketing-ops API" >&2
    continue
  fi
  if bring_api_up "$candidate" "$web_port_used"; then
    api_base_used=$candidate
    break
  fi
done

if [[ -z "$api_base_used" ]]; then
  echo "run-stack: no compatible API available" >&2
  exit 1
fi
# A reused API can still require the key from apps/api/.env. Pass only that
# key to the server-side web proxy; never publish it through NEXT_PUBLIC_*.
if [[ -z "${API_KEY:-}" && -f "$api_dir/.env" ]]; then
  API_KEY=$(set -a; . "$api_dir/.env"; printf '%s' "${API_KEY:-}")
  export API_KEY
fi

case "$mode" in
  --dev)
    echo "run-stack: dashboard dev server on http://localhost:${web_port_used} (API: ${api_base_used})" >&2
    cd "$repo_root/apps/web"
    API_BASE_URL="$api_base_used" WEB_AUTH_DISABLED=true \
      WEB_ORIGIN="http://127.0.0.1:$web_port_used" \
      npm run dev -- --hostname 127.0.0.1 --port "$web_port_used"
    ;;
  *)
    cd "$repo_root/apps/web"
    bash e2e/run-web-server.sh "$api_base_used" "$web_port_used"
    ;;
esac
