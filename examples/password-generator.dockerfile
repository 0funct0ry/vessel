FROM alpine:latest
RUN apk add --no-cache openssl
ENTRYPOINT ["sh", "-c", "echo 'Generated password:'; openssl rand -base64 18 | tr -d '=+/' | cut -c1-24"]
