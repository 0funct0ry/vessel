FROM alpine:latest
RUN printf '#!/bin/sh\nwhile true; do\n  { echo -e "HTTP/1.1 200 OK\\r\\nContent-Type: text/plain\\r\\n\\r\\n"; cat; } | nc -l -p 8080 -q 1\ndone' > /server.sh && chmod +x /server.sh
EXPOSE 8080
ENTRYPOINT ["/server.sh"]
