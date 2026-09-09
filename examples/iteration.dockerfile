FROM alpine:latest

# Install bash for nicer scripting (optional, sh works fine too)
RUN apk add --no-cache bash coreutils

# Simple long-running script that prints interesting output
RUN printf '#!/bin/sh\n\
i=0\n\
while true; do\n\
  i=$((i+1))\n\
  echo "[$(date +%%Y-%%m-%%dT%%H:%%M:%%S)] Iteration $i: $(head -c 100 /dev/urandom | md5sum | cut -d" " -f1)"\n\
  if [ $((i %% 10)) -eq 0 ]; then\n\
    echo "WARN: checkpoint reached at iteration $i" >&2\n\
  fi\n\
  sleep 2\n\
done\n' > /run.sh && chmod +x /run.sh

ENTRYPOINT ["/run.sh"]