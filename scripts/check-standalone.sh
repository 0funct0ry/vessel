#!/usr/bin/env bash
# Asserts that bin/vessel is a genuinely self-contained binary: copied alone
# into a directory with no web/dist nearby, it must still serve the real UI
# (not the "UI not built" fallback page), proving the frontend was embedded.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bin_path="$repo_root/bin/vessel"

if [[ ! -x "$bin_path" ]]; then
	echo "check-standalone: $bin_path not found or not executable; run 'make build' first" >&2
	exit 1
fi

work_dir="$(mktemp -d)"
cleanup() {
	if [[ -n "${server_pid:-}" ]]; then
		kill "$server_pid" 2>/dev/null || true
		wait "$server_pid" 2>/dev/null || true
	fi
	rm -rf "$work_dir"
}
trap cleanup EXIT

cp "$bin_path" "$work_dir/vessel"

port=18173
(
	cd "$work_dir"
	./vessel serve --addr 127.0.0.1 --port "$port" >standalone.log 2>&1 &
	echo $! >server.pid
)
server_pid="$(cat "$work_dir/server.pid")"

deadline=$((SECONDS + 10))
body=""
while (( SECONDS < deadline )); do
	if body="$(curl -fsS "http://127.0.0.1:$port/" 2>/dev/null)"; then
		break
	fi
	sleep 0.2
done

if [[ -z "$body" ]]; then
	echo "check-standalone: server never responded on http://127.0.0.1:$port/" >&2
	cat "$work_dir/standalone.log" >&2 || true
	exit 1
fi

if [[ "$body" == *"UI not built"* ]]; then
	echo "check-standalone: binary served the 'UI not built' fallback page — frontend was not embedded" >&2
	exit 1
fi

if [[ "$body" != *"vessel-base-path"* ]]; then
	echo "check-standalone: response did not look like the real index.html (no vessel-base-path meta tag found)" >&2
	echo "$body" >&2
	exit 1
fi

echo "check-standalone: OK — binary served the embedded UI from $work_dir (no web/dist present)"
