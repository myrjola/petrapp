#!/bin/bash
# Start the dev server on a random free port and open the browser once bound.
set -euo pipefail

# VAPID keys: PETRAPP_VAPID_PUBLIC / PETRAPP_VAPID_PRIVATE.
# Unset → binary generates ephemeral pair on startup and logs the public key.

PETRAPP_ADDR=localhost:0 ./bin/petrapp 2>&1 | while IFS= read -r line; do
    printf '%s\n' "$line"
    case "$line" in
        *'msg="starting server"'*)
            addr=$(printf '%s' "$line" | grep -oE '(^|[[:space:]])addr=[^[:space:]]+' | grep -oE 'addr=[^[:space:]]+' | cut -d= -f2)
            if [ -n "$addr" ]; then
                url="http://$addr"
                if command -v open > /dev/null 2>&1; then
                    open "$url" &
                elif command -v xdg-open > /dev/null 2>&1; then
                    xdg-open "$url" &
                fi
            fi
            ;;
    esac
done; exit "${PIPESTATUS[0]}"
