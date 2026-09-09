FROM alpine:latest
RUN printf '#!/bin/sh\n\
answers="Yes definitely No way Ask again later Absolutely Signs point to no Outlook good Very doubtful It is certain Cannot predict now"\n\
i=0\n\
while true; do\n\
  i=$((i+1))\n\
  a=$(echo $answers | tr " " "\\n" | shuf -n1 2>/dev/null || echo $answers | cut -d" " -f$((i%%10+1)))\n\
  echo "Q$i: Will it rain tomorrow? -> $a"\n\
  sleep 3\n\
done\n' > /run.sh && chmod +x /run.sh
RUN apk add --no-cache coreutils
ENTRYPOINT ["/run.sh"]