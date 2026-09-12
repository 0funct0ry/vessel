FROM alpine:latest

RUN printf '#!/bin/sh\n\
echo "Base64 tool (type text or base64, Ctrl+D to quit)"\n\
while IFS= read -r line; do\n\
  echo "$line" | base64 -w0\n\
  echo\n\
  echo "$line" | base64 -d 2>/dev/null || true\n\
  echo "---"\n\
done\n' > /run.sh && chmod +x /run.sh

ENTRYPOINT ["/bin/sh", "/run.sh"]
