#!/usr/bin/env bash
# Fails if the gzipped main JS chunk in web/dist/assets exceeds the 400 KB
# budget (SPEC M20 item 4). "Main chunk" = the largest built JS file, since
# this build emits one main app bundle plus a couple of small route chunks.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
assets_dir="$repo_root/web/dist/assets"
budget_bytes=409600 # 400 KB

if [[ ! -d "$assets_dir" ]]; then
	echo "check-bundle-size: $assets_dir not found; run 'make web-build' first" >&2
	exit 1
fi

largest_file=""
largest_gzip_size=0

for f in "$assets_dir"/*.js; do
	[[ -e "$f" ]] || continue
	size="$(gzip -c "$f" | wc -c | tr -d ' ')"
	if (( size > largest_gzip_size )); then
		largest_gzip_size="$size"
		largest_file="$f"
	fi
done

if [[ -z "$largest_file" ]]; then
	echo "check-bundle-size: no .js files found under $assets_dir" >&2
	exit 1
fi

echo "check-bundle-size: largest chunk $(basename "$largest_file") = $largest_gzip_size bytes gzipped (budget $budget_bytes)"

if (( largest_gzip_size > budget_bytes )); then
	echo "check-bundle-size: FAIL — exceeds 400 KB gzipped budget" >&2
	exit 1
fi

echo "check-bundle-size: OK"
