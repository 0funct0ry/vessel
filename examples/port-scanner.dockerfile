FROM alpine:latest
RUN apk add --no-cache netcat-openbsd
ENTRYPOINT ["sh", "-c", "echo 'Scanning common ports...'; for p in 22 80 443 3306 5432 6379 8080 8443; do nc -z -w1 host.docker.internal $p && echo \"Port $p open\" || true; done"]
