#!/usr/bin/env bash
# Update the Worker's TARGET after the quick-tunnel URL rotates (reboot etc),
# then redeploy, so https://<name>.workers.dev always points at the live app.
#
# Usage: ./update-target.sh https://<new-name>.trycloudflare.com
set -euo pipefail

URL="${1:?usage: update-target.sh https://<new-quick-tunnel-url>}"
case "$URL" in
  https://*.trycloudflare.com) ;&
  https://*) [ "${URL:0:8}" = "https://" ] || { echo "must be an https URL" >&2; exit 1; };;
  *) echo "must be an https URL" >&2; exit 1;;
esac

cd "$(dirname "$0")"
sed -i.bak -E "s|TARGET: \"https://[^\"]+\"|TARGET: \"$URL\"|" wrangler.toml
rm -f wrangler.toml.bak
npx wrangler deploy
echo "Worker now proxies to $URL"
