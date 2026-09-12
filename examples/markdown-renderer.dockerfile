FROM alpine:latest
RUN apk add --no-cache python3 py3-markdown
ENTRYPOINT ["python3", "-c", "import sys, markdown; print(markdown.markdown(sys.stdin.read()))"]
