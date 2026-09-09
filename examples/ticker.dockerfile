FROM alpine:latest
RUN printf '#!/bin/sh\n\
price=100\n\
while true; do\n\
  delta=$(( (RANDOM %% 21) - 10 ))\n\
  price=$((price + delta))\n\
  [ $price -lt 1 ] && price=1\n\
  if [ $delta -ge 0 ]; then arrow="UP"; else arrow="DOWN"; fi\n\
  echo "TICK CORP: \\$${price} ($arrow $delta)"\n\
  [ $price -lt 20 ] && echo "ALERT: TICK approaching penny-stock territory!" >&2\n\
  sleep 1\n\
done\n' > /run.sh && chmod +x /run.sh
ENTRYPOINT ["/bin/sh", "/run.sh"]