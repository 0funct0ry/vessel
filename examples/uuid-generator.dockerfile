FROM alpine:latest
RUN apk add --no-cache util-linux
ENTRYPOINT ["sh", "-c", "while true; do uuidgen; sleep 0.5; done"]
