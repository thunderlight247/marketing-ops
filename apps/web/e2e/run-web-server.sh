#!/usr/bin/env bash
set -euo pipefail

api_base_url=${1:?API base URL required}
web_port=${2:?web port required}
dist_dir=".next-e2e-${web_port}"
env_backup=".env.local.e2e-backup"
tsconfig_backup=$(mktemp)
next_env_backup=$(mktemp)
cp tsconfig.json "$tsconfig_backup"
cp next-env.d.ts "$next_env_backup"
had_env_file=0

restore_build_files() {
  cp "$tsconfig_backup" tsconfig.json
  cp "$next_env_backup" next-env.d.ts
  rm -f "$tsconfig_backup" "$next_env_backup" tsconfig.tsbuildinfo
  if (( had_env_file )) && [[ -f "$env_backup" ]]; then
    mv "$env_backup" .env.local
  fi
}

if [[ -f .env.local ]]; then
  mv .env.local "$env_backup"
  had_env_file=1
fi
trap restore_build_files EXIT
NEXT_DIST_DIR="$dist_dir" API_BASE_URL="$api_base_url" WEB_AUTH_DISABLED=true npm run build
restore_build_files
trap - EXIT
# WEB_ORIGIN is the browser's origin for this server. It is explicit because
# Next derives its own request origin as localhost, while the suite (and the
# bind address) use 127.0.0.1: without it every same-origin POST to the API
# proxy is rejected as a cross-origin request.
exec env NEXT_DIST_DIR="$dist_dir" API_BASE_URL="$api_base_url" WEB_AUTH_DISABLED=true \
  WEB_ORIGIN="http://127.0.0.1:$web_port" \
  ./node_modules/.bin/next start --hostname 127.0.0.1 --port "$web_port"
