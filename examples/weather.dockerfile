FROM alpine:latest
RUN apk add --no-cache coreutils
RUN printf '#!/bin/sh\n\
i=0\n\
while true; do\n\
  i=$((i+1))\n\
  temp=$(( (i * 7) %% 40 - 5 ))\n\
  humidity=$(( (i * 13) %% 100 ))\n\
  echo "Station-01: Temp=${temp}C Humidity=${humidity}%%"\n\
  if [ $humidity -gt 90 ]; then\n\
    echo "STORM WARNING: humidity critical (${humidity}%%)" >&2\n\
  fi\n\
  sleep 2\n\
done\n' > /run.sh && chmod +x /run.sh
ENTRYPOINT ["/run.sh"]