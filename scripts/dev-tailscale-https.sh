#!/usr/bin/env bash
set -euo pipefail

TAILSCALE_FQDN=$(tailscale status --json | jq -r '.Self.DNSName | rtrimstr(".")')
if [[ -z "$TAILSCALE_FQDN" ]]; then
    echo "Could not determine Tailscale FQDN. Is tailscale running?" >&2
    exit 1
fi

CERT_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/petrapp-ts-certs"
mkdir -p "$CERT_DIR"
CERT_FILE="$CERT_DIR/$TAILSCALE_FQDN.crt"
KEY_FILE="$CERT_DIR/$TAILSCALE_FQDN.key"

echo "Issuing/renewing cert for $TAILSCALE_FQDN …"
tailscale cert --cert-file "$CERT_FILE" --key-file "$KEY_FILE" "$TAILSCALE_FQDN"

TAILNET_IP=$(tailscale ip -4)
PORT=8443

export PETRAPP_ADDR="${TAILNET_IP}:${PORT}"
export PETRAPP_FQDN="$TAILSCALE_FQDN"
export PETRAPP_TLS_CERT="$CERT_FILE"
export PETRAPP_TLS_KEY="$KEY_FILE"

echo ""
echo "Access from iOS Safari:"
echo "  https://${TAILSCALE_FQDN}:${PORT}"
echo ""

exec ./bin/petrapp
