FROM alpine:latest
RUN printf '#!/bin/sh\n\
chars="01"\n\
while true; do\n\
  line=""\n\
  n=0\n\
  while [ $n -lt 40 ]; do\n\
    r=$(( $$ + n + $(date +%%N | cut -c1-3) ))\n\
    bit=$(( r %% 2 ))\n\
    line="${line}$(echo $chars | cut -c$((bit+1)))"\n\
    n=$((n+1))\n\
  done\n\
  echo "$line"\n\
  sleep 1\n\
done\n' > /run.sh && chmod +x /run.sh
ENTRYPOINT ["/run.sh"]