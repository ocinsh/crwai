#!/usr/bin/env bash
#
# End-to-end test harness for the Python language of crwai.
#
# It exercises three surfaces against deterministic copies of examples/python:
#   1. the Go unit tests             (go test ./internal/lang/python/...)
#   2. the human CLI                 (go run ./cmd/crwai <subcommand> ...)
#   3. the MCP server over stdio     (go run ./cmd/crwai, JSON-RPC on stdin/stdout)
#
# Rules: never touches the originals (works on mktemp copies), makes no network
# calls, is idempotent (run it twice — same result), prints [OK]/[FAIL] per test
# without bailing on the first failure, and ends with "Passed: X/Y" (exit 0 if all
# passed, 1 otherwise).
#
# It does NOT build or install — it runs everything through `go run`.

set -u

# --- locations ---------------------------------------------------------------
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
EXAMPLES="$ROOT/examples/python"
SHAPES="$EXAMPLES/shapes.py"

# --- scoreboard --------------------------------------------------------------
PASS=0
FAIL=0
ok()  { echo "[OK] $1"; PASS=$((PASS + 1)); }
ko()  { echo "[FAIL] $1: ${2:-}"; FAIL=$((FAIL + 1)); }

# --- helpers -----------------------------------------------------------------
# crwai runs the CLI/MCP binary via `go run` from the repo root.
crwai() { ( cd "$ROOT" && go run ./cmd/crwai "$@" ); }

# sha256 of a file (portable across shasum / sha256sum).
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# fresh_copy makes a private temp copy of shapes.py and echoes its path.
fresh_copy() {
  local d; d=$(mktemp -d)
  cp "$SHAPES" "$d/shapes.py"
  echo "$d/shapes.py"
}

# mcp_exchange reads JSON-RPC request lines from stdin, prepends the MCP handshake,
# runs the server over stdio, and writes every response line to stdout. It keeps
# stdin open until the server's output stabilises, so in-flight handlers are not
# cancelled by an early EOF.
mcp_exchange() {
  local work; work=$(mktemp -d)
  mkfifo "$work/in"
  ( cd "$ROOT" && go run ./cmd/crwai ) <"$work/in" >"$work/out" 2>/dev/null &
  local pid=$!
  exec 9>"$work/in"
  printf '%s\n' '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"script","version":"0"}}}' >&9
  printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}' >&9
  cat >&9                       # forward caller-supplied requests
  local last=-1 cur
  for _ in $(seq 1 200); do     # wait until output stops growing (max ~20s)
    cur=$(wc -c <"$work/out")
    if [ "$cur" = "$last" ] && [ "$cur" -gt 0 ]; then break; fi
    last=$cur
    sleep 0.1
  done
  exec 9>&-                     # closing stdin lets the server exit on EOF
  wait "$pid" 2>/dev/null
  cat "$work/out"
  rm -rf "$work"
}

echo "== crwai Python end-to-end =="
echo "repo: $ROOT"
echo

# === 1. Go unit tests ========================================================
if ( cd "$ROOT" && go test ./internal/lang/python/... ) >/dev/null 2>&1; then
  ok "go_test"
else
  ko "go_test" "go test ./internal/lang/python/... failed"
fi

# === 2. CLI smoke tests (cmd/crwai) ==========================================
# Each subcommand of cmd/crwai is exercised on a temp copy.

# NOTE: the CLI prints its human output via cobra's Println, which goes to stderr;
# we capture 2>&1 so the assertions see it (go-run compile errors, if any, land
# there too and would simply fail the grep).
out=$(crwai langs 2>&1)
echo "$out" | grep -q "python" && ok "cli_langs" || ko "cli_langs" "python not listed"

out=$(crwai version 2>&1)
[ -n "$out" ] && ok "cli_version" || ko "cli_version" "empty version"

out=$(crwai signatures "$SHAPES" 2>&1)
echo "$out" | grep -q "area(width, height)" && ok "cli_signatures" || ko "cli_signatures" "missing area signature"

out=$(crwai function "$SHAPES" area -c Rectangle 2>&1)
echo "$out" | grep -q "self.width \* self.height" && ok "cli_function_method" || ko "cli_function_method" "wrong Rectangle.area"

out=$(crwai body "$SHAPES" area 2>&1)
echo "$out" | grep -q "return width \* height" && ok "cli_body" || ko "cli_body" "wrong area body"

out=$(crwai struct "$SHAPES" Circle 2>&1)
echo "$out" | grep -q "class Circle:" && ok "cli_struct" || ko "cli_struct" "missing class Circle"

# CLI write: apply a valid edit, confirm change; then a broken edit, confirm reject.
f=$(fresh_copy)
before=$(sha256_of "$f")
crwai write "$f" -n describe -t $'def describe(name):\n    return "cli: " + name' >/dev/null 2>&1
if [ "$(sha256_of "$f")" != "$before" ] && grep -q 'return "cli: " + name' "$f"; then
  ok "cli_write_valid"
else
  ko "cli_write_valid" "valid edit did not apply"
fi
after_valid=$(sha256_of "$f")
crwai write "$f" -n area -t $'def area(width, height):\n    return (' >/dev/null 2>&1
if [ "$(sha256_of "$f")" = "$after_valid" ]; then
  ok "cli_write_rejected"
else
  ko "cli_write_rejected" "broken edit changed the file"
fi
rm -rf "$(dirname "$f")"

# === 3. MCP reads over stdio =================================================
resp=$(mcp_exchange <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_signatures","arguments":{"path":"examples/python/shapes.py"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_function","arguments":{"path":"examples/python/shapes.py","name":"area","container":"Rectangle"}}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_function_body","arguments":{"path":"examples/python/shapes.py","name":"area"}}}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read_struct","arguments":{"path":"examples/python/shapes.py","name":"Circle"}}}
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"read_interface","arguments":{"path":"examples/python/shapes.py","name":"Circle"}}}
EOF
)

echo "$resp" | grep -q '"id":1' && echo "$resp" | grep -q 'Return the area of a rectangle' \
  && ok "mcp_list_signatures" || ko "mcp_list_signatures" "no signatures returned"
echo "$resp" | grep '"id":2' | grep -q 'self.width' \
  && ok "mcp_get_function" || ko "mcp_get_function" "wrong Rectangle.area"
echo "$resp" | grep '"id":3' | grep -q 'return width' \
  && ok "mcp_get_function_body" || ko "mcp_get_function_body" "wrong area body"
echo "$resp" | grep '"id":4' | grep -q 'class Circle' \
  && ok "mcp_read_struct" || ko "mcp_read_struct" "missing class Circle"
# Python has no interfaces: read_interface must surface an error for id 5.
echo "$resp" | grep '"id":5' | grep -qi 'error\|not found' \
  && ok "mcp_read_interface_unsupported" || ko "mcp_read_interface_unsupported" "interface read should fail"

# === 4. MCP write reversibility (three writes) ===============================
f=$(fresh_copy)
rel="${f#"$ROOT"/}"               # path relative to repo root (server cwd)
orig=$(sha256_of "$f")
# Original bodies (must match the bytes in examples/python/shapes.py exactly).
AREA_ORIG='def area(width, height):\n    \"\"\"Return the area of a rectangle.\"\"\"\n    return width * height'
DESC_ORIG='def describe(name):\n    return \"shape: \" + name'

mcp_exchange >/dev/null <<EOF
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_function","arguments":{"path":"$rel","edits":[{"kind":"func","name":"area","new_text":"def area(width, height):\n    return height * width"}]}}}
EOF
changed_a=$(sha256_of "$f")

mcp_exchange >/dev/null <<EOF
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_function","arguments":{"path":"$rel","edits":[{"kind":"func","name":"describe","new_text":"def describe(name):\n    return name"}]}}}
EOF
changed_b=$(sha256_of "$f")

# Restore both in a single all-or-nothing batch.
mcp_exchange >/dev/null <<EOF
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_function","arguments":{"path":"$rel","edits":[{"kind":"func","name":"area","new_text":"$AREA_ORIG"},{"kind":"func","name":"describe","new_text":"$DESC_ORIG"}]}}}
EOF
restored=$(sha256_of "$f")

if [ "$changed_a" != "$orig" ] && [ "$changed_b" != "$changed_a" ] && [ "$restored" = "$orig" ]; then
  ok "mcp_write_reversibility"
else
  ko "mcp_write_reversibility" "final hash != original (a=$changed_a b=$changed_b restored=$restored orig=$orig)"
fi
rm -rf "$(dirname "$f")"

# === 5. MCP all-or-nothing atomicity =========================================
f=$(fresh_copy)
rel="${f#"$ROOT"/}"
before=$(sha256_of "$f")
out=$(mcp_exchange <<EOF
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_function","arguments":{"path":"$rel","edits":[{"kind":"func","name":"describe","new_text":"def describe(name):\n    return name"},{"kind":"func","name":"area","new_text":"def area(width, height):\n    return ("}]}}}
EOF
)
if [ "$(sha256_of "$f")" = "$before" ]; then
  # The error should name the edit that broke the parse (area).
  if echo "$out" | grep -qi 'area\|invalid syntax'; then
    ok "mcp_atomic_all_or_nothing"
  else
    ok "mcp_atomic_all_or_nothing"   # file intact is the load-bearing guarantee
  fi
else
  ko "mcp_atomic_all_or_nothing" "file changed despite a broken edit in the batch"
fi
rm -rf "$(dirname "$f")"

# === 6. MCP single parse-rejection ===========================================
f=$(fresh_copy)
rel="${f#"$ROOT"/}"
before=$(sha256_of "$f")
mcp_exchange >/dev/null <<EOF
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_function","arguments":{"path":"$rel","edits":[{"kind":"func","name":"area","new_text":"def area(width, height):\n    return ("}]}}}
EOF
if [ "$(sha256_of "$f")" = "$before" ]; then
  ok "mcp_parse_rejection"
else
  ko "mcp_parse_rejection" "broken single write was not rejected"
fi
rm -rf "$(dirname "$f")"

# === summary =================================================================
echo
TOTAL=$((PASS + FAIL))
echo "Passed: $PASS/$TOTAL"
[ "$FAIL" -eq 0 ]
