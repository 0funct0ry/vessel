FROM alpine:latest
ENTRYPOINT ["sh", "-c", "while true; do clear; echo '=== Network Stats ==='; date; cat /proc/net/dev; sleep 1; done"]
