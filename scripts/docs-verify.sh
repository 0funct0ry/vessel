#!/usr/bin/env bash
# docs-verify.sh — extracts the webhook signature-verification snippets from
# docs/src/content/docs/webhooks.mdx and proves they are byte-for-byte
# correct against internal/webhook/signature.go's own Signature(), against
# one fixed fixture. This is a genuine correctness check: if any of the Go,
# Node, or Python snippets computed the wrong thing, the assertion below
# would fail.
set -euo pipefail

cd "$(dirname "$0")/.."
REPO_ROOT="$(pwd)"

# real_sig.go must live under the module tree to import internal/webhook (Go
# forbids importing another module's/tree's internal/ package from outside
# it); the extracted snippets have no such restriction and go in a plain
# temp dir.
GOWORK="$REPO_ROOT/scripts/.docs-verify-tmp"
rm -rf "$GOWORK"
mkdir -p "$GOWORK"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR" "$GOWORK"' EXIT

SECRET="whsec_docsverify_fixture_secret"
UNIX_TS="1731000000"
BODY='{"id":"ev_fixture","delivery_id":"dl_fixture","webhook_id":"wh_fixture","type":"container.die","created_at":"2024-11-07T17:20:00Z","host":{"name":"fixture-host","engine":"27.3.1"},"container":{"id":"c_fixture","name":"redis","image":"redis:7.4-alpine","exit_code":0}}'

echo "docs-verify: computing the real signature from internal/webhook/signature.go ..."
cat > "$GOWORK/real_sig.go" <<'EOF'
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/0funct0ry/vessel/internal/webhook"
)

func main() {
	unix, err := strconv.ParseInt(os.Args[2], 10, 64)
	if err != nil {
		panic(err)
	}
	fmt.Print(webhook.Signature(os.Args[1], unix, []byte(os.Args[3])))
}
EOF
REAL_HEADER="$(go run "$GOWORK/real_sig.go" "$SECRET" "$UNIX_TS" "$BODY")"
echo "docs-verify: real header = $REAL_HEADER"

REAL_V1="${REAL_HEADER#*v1=}"
if [[ -z "$REAL_V1" || "$REAL_V1" == "$REAL_HEADER" ]]; then
  echo "docs-verify: FAIL — could not parse v1= out of the real header" >&2
  exit 1
fi

extract_block() {
  # extract_block <language-tag-in-docs> -> writes the fenced code block body to stdout
  local marker="$1"
  awk -v marker="$marker" '
    $0 == "```" marker { grab=1; next }
    grab && $0 ~ "^```$" { exit }
    grab { print }
  ' docs/src/content/docs/webhooks.mdx
}

echo "docs-verify: extracting Go/Node/Python snippets from webhooks.mdx ..."
extract_block "go" > "$WORKDIR/verify.go"
extract_block "js" > "$WORKDIR/verify.js"
extract_block "py" > "$WORKDIR/verify.py"

for f in verify.go verify.js verify.py; do
  if [[ ! -s "$WORKDIR/$f" ]]; then
    echo "docs-verify: FAIL — could not extract a non-empty $f from webhooks.mdx" >&2
    exit 1
  fi
done

FAIL=0

echo "docs-verify: running Go snippet ..."
if OUT="$(cd "$WORKDIR" && go run verify.go "$SECRET" "$BODY" "$REAL_HEADER" 2>&1)"; then
  echo "  $OUT"
else
  echo "docs-verify: FAIL — Go snippet rejected the real signature" >&2
  echo "  $OUT" >&2
  FAIL=1
fi

echo "docs-verify: running Node snippet ..."
if command -v node >/dev/null 2>&1; then
  if OUT="$(node "$WORKDIR/verify.js" "$SECRET" "$BODY" "$REAL_HEADER" 2>&1)"; then
    echo "  $OUT"
  else
    echo "docs-verify: FAIL — Node snippet rejected the real signature" >&2
    echo "  $OUT" >&2
    FAIL=1
  fi
else
  echo "docs-verify: FAIL — node is not installed" >&2
  FAIL=1
fi

echo "docs-verify: running Python snippet ..."
PYTHON_BIN="$(command -v python3 || true)"
if [[ -n "$PYTHON_BIN" ]]; then
  if OUT="$("$PYTHON_BIN" "$WORKDIR/verify.py" "$SECRET" "$BODY" "$REAL_HEADER" 2>&1)"; then
    echo "  $OUT"
  else
    echo "docs-verify: FAIL — Python snippet rejected the real signature" >&2
    echo "  $OUT" >&2
    FAIL=1
  fi
else
  echo "docs-verify: FAIL — python3 is not installed" >&2
  FAIL=1
fi

echo "docs-verify: negative control — a tampered body must NOT verify ..."
TAMPERED_BODY="${BODY/redis/tampered}"
if go run "$WORKDIR/verify.go" "$SECRET" "$TAMPERED_BODY" "$REAL_HEADER" >/dev/null 2>&1; then
  echo "docs-verify: FAIL — tampered-body fixture verified successfully; the check is a no-op" >&2
  FAIL=1
else
  echo "  ok: tampered body correctly rejected"
fi

if [[ "$FAIL" -ne 0 ]]; then
  echo "docs-verify: FAILED" >&2
  exit 1
fi

echo "docs-verify: PASSED — Go, Node, and Python webhook signature snippets all match internal/webhook/signature.go"
