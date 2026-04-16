#!/bin/bash
# Start the dev server on a random free port and open the browser once bound.

PETRAPP_ADDR=localhost:0 ./bin/petrapp 2>&1 | while IFS= read -r line; do
    printf '%s\n' "$line"
    case "$line" in
        *'msg="starting server"'*)
            addr=$(printf '%s' "$line" | grep -oE 'addr=[^ ]+' | cut -d= -f2)
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
done
