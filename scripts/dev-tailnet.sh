#!/bin/bash
# Start the dev server bound to the tailnet IP so it's reachable from other tailnet devices.
set -euo pipefail

TAILNET_IP=""
if command -v tailscale > /dev/null 2>&1; then
    # Returns empty string if daemon is not running; Linux fallback handles that.
    TAILNET_IP=$(tailscale ip -4 2>/dev/null) || TAILNET_IP=""
fi

# Linux fallback: parse the tailscale0 interface directly (not available on macOS).
if [ -z "$TAILNET_IP" ]; then
    TAILNET_IP=$(ip addr show tailscale0 2>/dev/null | grep -oE 'inet [0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | cut -d' ' -f2)
fi

if [ -z "$TAILNET_IP" ]; then
    echo "error: could not detect tailnet IP — is tailscale CLI in PATH? Try: tailscale ip -4" >&2
    exit 1
fi

echo "Tailnet IP: $TAILNET_IP"

PETRAPP_ADDR="${TAILNET_IP}:0" ./bin/petrapp 2>&1 | while IFS= read -r line; do
    printf '%s\n' "$line"
    case "$line" in
        *'msg="starting server"'*)
            addr=$(printf '%s' "$line" | grep -oE '(^|[[:space:]])addr=[^[:space:]]+' | grep -oE 'addr=[^[:space:]]+' | cut -d= -f2)
            if [ -n "$addr" ]; then
                echo "Dev server URL: http://$addr"
            fi
            ;;
    esac
done; exit "${PIPESTATUS[0]}"
