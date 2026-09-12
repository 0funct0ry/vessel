FROM alpine:latest
ENTRYPOINT ["sh", "-c", "while true; do clear; echo '=== Disk & Inode Usage ==='; date; df -h; echo; df -i; sleep 3; done"]
