FROM alpine:latest
RUN printf '#!/bin/sh\n\
steps="Initializing kernel Mounting filesystems Starting network daemon Calibrating flux capacitor Loading personality matrix Warming up quantum core Establishing uplink Almost there Ready"\n\
while true; do\n\
  for s in $steps; do\n\
    :\n\
  done\n\
  echo "$steps" | tr " " "\\n" > /tmp/steps\n\
  while read -r line; do\n\
    echo "[BOOT] $line..."\n\
    sleep 1\n\
  done < /tmp/steps\n\
  echo "System online. Uptime reset."\n\
  echo "---- REBOOTING FOR DRAMATIC EFFECT ----" >&2\n\
  sleep 2\n\
done\n' > /run.sh && chmod +x /run.sh
ENTRYPOINT ["/bin/sh", "/run.sh"]