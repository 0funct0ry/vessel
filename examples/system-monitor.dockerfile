FROM alpine:latest
RUN apk add --no-cache procps
ENTRYPOINT ["sh", "-c", "while true; do clear; echo '=== System Monitor ==='; date; uptime; free -h; echo; cat /proc/loadavg; sleep 2; done"]
