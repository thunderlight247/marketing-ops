#!/usr/bin/env bash
# ==============================================================================
# Marketing Ops Platform — Production Security Configuration Generator
#
# Generates cryptographically secure 256-bit random keys for:
#   1. API_KEY (Shared between Next.js server proxy and Go API)
#   2. WHATSAPP_DISPATCHER_SECRET (Shared between Go API and Node dispatcher)
#   3. CREDENTIAL_ENCRYPTION_KEY (Go API AES-GCM at-rest encryption)
#   4. Operator credentials for dashboard access
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

generate_key() {
  openssl rand -hex 32
}

API_KEY="$(generate_key)"
DISPATCHER_SECRET="$(generate_key)"
ENCRYPTION_KEY="$(generate_key)"
DEFAULT_OPERATOR_PASSWORD="$(openssl rand -base64 18 | tr -dc 'a-zA-Z0-9!@#%^&*')"

echo "================================================================================"
echo "Marketing Operations Platform — Production Secrets Generated"
echo "================================================================================"
echo ""
echo "API_KEY (Go API & Next.js proxy Bearer auth):"
echo "  ${API_KEY}"
echo ""
echo "WHATSAPP_DISPATCHER_SECRET (Internal X-Internal-Secret):"
echo "  ${DISPATCHER_SECRET}"
echo ""
echo "CREDENTIAL_ENCRYPTION_KEY (AES-GCM 32-byte key):"
echo "  ${ENCRYPTION_KEY}"
echo ""
echo "OPERATOR CREDENTIALS (Dashboard Web Auth):"
echo "  Username: operator"
echo "  Password: ${DEFAULT_OPERATOR_PASSWORD}"
echo ""
echo "================================================================================"
echo "To apply these for production:"
echo ""
echo "1. In apps/api/.env:"
echo "   APP_ENV=production"
echo "   API_KEY=${API_KEY}"
echo "   CREDENTIAL_ENCRYPTION_KEY=${ENCRYPTION_KEY}"
echo "   WHATSAPP_DISPATCHER_SECRET=${DISPATCHER_SECRET}"
echo ""
echo "2. In apps/web/.env.local:"
echo "   API_KEY=${API_KEY}"
echo "   WEB_AUTH_DISABLED=false"
echo "   OPERATOR_USERNAME=operator"
echo "   OPERATOR_PASSWORD=${DEFAULT_OPERATOR_PASSWORD}"
echo ""
echo "3. In services/whatsapp-dispatcher/.env:"
echo "   DISPATCHER_SECRET=${DISPATCHER_SECRET}"
echo "================================================================================"
