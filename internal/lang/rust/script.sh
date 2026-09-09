#!/usr/bin/env bash
#
# End-to-end smoke test for the Rust language support of crwai.
#
# It exercises three surfaces, all on TEMPORARY COPIES of the examples (the
# originals in examples/rust/ are never touched):
#
#   1. the Go unit tests              (go test ./internal/lang/rust/...)
#   2. the MCP server over stdio      (go run ./cmd/crwai, JSON-RPC on stdin/out)
#   3. the human CLI subcommands      (go run ./cmd/crwai <cmd> ...)
#
# Every check prints "[OK] <name>" or "[FAIL] <name>: <reason>". The script never
# exits on the first failure: it runs every check, prints "Passed: X/Y", and exits
# 0 only if all passed (1 otherwise). It makes no network calls and is idempotent:
# running it twice in a row yields the same result.
#
# Note: per the project contract we use `go run ./cmd/crwai` (no build/install of
# dist artifacts). A one-off `go build -o /dev/null` compile check up front detects
# a broken tree (e.g. a sibling language mid-implementation) and warms the build
# cache so the JSON-RPC round-trips don't race the compiler.

set -u

# --- locations -------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
EXAMPLES="$ROOT/examples/rust"
cd "$ROOT" || { echo "cannot cd to repo root"; exit 1; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
export NO_COLOR=1

PASS=0
TOTAL=0
ok()   { PASS=$((PASS + 1)); TOTAL=$((TOTAL + 1)); echo "[OK] $1"; }
fail() { TOTAL=$((TOTAL + 1)); echo "[FAIL] $1: $2"; }

HAVE_JQ=0
command -v jq >/dev/null 2>&1 && HAVE_JQ=1

# sha256 helper (sha256sum on Linux, shasum on macOS).
sha() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# --- MCP plumbing ----------------------------------------------------------
INIT='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"script","version":"0"}}}'
INITED='{"jsonrpc":"2.0","method":"notifications/initialized"}'

# mcp_call <tool-call-json-with-id-2> -> prints the id:2 response line.
# A fresh stateless server per call; disk is the only state between calls.
mcp_call() {
  local D; D="$(mktemp -d)"
  mkfifo "$D/in" "$D/out"
  go run ./cmd/crwai < "$D/in" > "$D/out" 2>/dev/null &
  local pid=$!
  exec 3<> "$D/in"
  exec 4<> "$D/out"
  printf '%s\n%s\n%s\n' "$INIT" "$INITED" "$1" >&3
  local r1 r2
  IFS= read -t 120 -r r1 <&4   # initialize response
  IFS= read -t 120 -r r2 <&4   # tool response
  exec 3>&- || true
  exec 4>&- || true
  wait "$pid" 2>/dev/null
  rm -rf "$D"
  printf '%s' "$r2"
}

# call_tool <name> <arguments-json> -> prints the response line.
call_tool() {
  mcp_call '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"'"$1"'","arguments":'"$2"'}}'
}

# jget <jq-filter> <json> -> extracts a field (empty if jq missing/!found).
jget() { [ "$HAVE_JQ" -eq 1 ] && printf '%s' "$2" | jq -r "$1" 2>/dev/null || printf ''; }

# --- 0. compile check (warms cache, detects a broken tree) -----------------
SERVER_OK=0
if go build -o /dev/null ./cmd/crwai 2>"$WORK/build.err"; then
  SERVER_OK=1
fi

# --- 1. Go unit tests ------------------------------------------------------
if go test ./internal/lang/rust/... >"$WORK/gotest.log" 2>&1; then
  ok "go_unit_tests"
else
  fail "go_unit_tests" "see $(tail -1 "$WORK/gotest.log")"
fi

# Guard the MCP/CLI sections behind a buildable tree.
if [ "$SERVER_OK" -ne 1 ]; then
  fail "build_cmd_crwai" "go build failed: $(tail -1 "$WORK/build.err")"
  echo
  echo "Passed: $PASS/$TOTAL"
  [ "$PASS" -eq "$TOTAL" ] && exit 0 || exit 1
fi

# --- 2. MCP reads ----------------------------------------------------------
BASIC="$EXAMPLES/basic.rs"
METHODS="$EXAMPLES/methods.rs"
TRAITS="$EXAMPLES/traits.rs"
COMPLEX="$EXAMPLES/complex.rs"

resp="$(call_tool list_signatures '{"path":"'"$BASIC"'"}')"
if [ "$HAVE_JQ" -eq 1 ]; then
  n="$(jget '.result.structuredContent.signatures | length' "$resp")"
  [ "${n:-0}" -ge 3 ] 2>/dev/null && ok "mcp_list_signatures" || fail "mcp_list_signatures" "got ${n:-none}"
else
  printf '%s' "$resp" | grep -q '"add"' && ok "mcp_list_signatures" || fail "mcp_list_signatures" "add not found"
fi

resp="$(call_tool get_function '{"path":"'"$BASIC"'","name":"add"}')"
printf '%s' "$resp" | grep -q 'fn add' && ok "mcp_get_function" || fail "mcp_get_function" "fn add missing"

resp="$(call_tool get_function_body '{"path":"'"$BASIC"'","name":"documented"}')"
printf '%s' "$resp" | grep -q 'true' && ok "mcp_get_function_body" || fail "mcp_get_function_body" "body missing"

resp="$(call_tool read_struct '{"path":"'"$METHODS"'","name":"Stack"}')"
printf '%s' "$resp" | grep -q 'struct Stack' && ok "mcp_read_struct" || fail "mcp_read_struct" "struct missing"

resp="$(call_tool read_interface '{"path":"'"$TRAITS"'","name":"Shape"}')"
printf '%s' "$resp" | grep -q 'trait Shape' && ok "mcp_read_interface" || fail "mcp_read_interface" "trait missing"

# --- 3. CLI reads (examples of using the utility by hand) ------------------
# crwai langs  -> must list the rust/.rs row
if go run ./cmd/crwai langs 2>&1 | grep -q 'rust'; then
  ok "cli_langs"; else fail "cli_langs" "rust not listed"; fi

# crwai version
if go run ./cmd/crwai version 2>&1 | grep -q 'crwai'; then
  ok "cli_version"; else fail "cli_version" "no version output"; fi

# crwai sig <file>  (alias of signatures) on the 24-symbol file
if go run ./cmd/crwai sig "$COMPLEX" 2>&1 | grep -q 'c16'; then
  ok "cli_signatures"; else fail "cli_signatures" "c16 not listed"; fi

# crwai fn <file> <name> -c <container>  (a method)
if go run ./cmd/crwai fn "$METHODS" size -c Stack 2>&1 | grep -q 'self.items.len()'; then
  ok "cli_function_method"; else fail "cli_function_method" "method body missing"; fi

# crwai fn <file> <name>  (a free function, same name, no container)
if go run ./cmd/crwai fn "$METHODS" size 2>&1 | grep -q 'fn size() -> usize'; then
  ok "cli_function_free"; else fail "cli_function_free" "free function missing"; fi

# crwai bd <file> <name>  (body only)
if go run ./cmd/crwai bd "$BASIC" documented 2>&1 | grep -q 'true'; then
  ok "cli_body"; else fail "cli_body" "body missing"; fi

# crwai st <file> <name>
if go run ./cmd/crwai st "$METHODS" Stack 2>&1 | grep -q 'struct Stack'; then
  ok "cli_struct"; else fail "cli_struct" "struct missing"; fi

# crwai iface <file> <name>
if go run ./cmd/crwai iface "$TRAITS" Shape 2>&1 | grep -q 'trait Shape'; then
  ok "cli_interface"; else fail "cli_interface" "trait missing"; fi

# --- 4. MCP write reversibility (three writes -> original hash) ------------
# Original node texts (declaration only, doc excluded) of add and documented.
ADD_ORIG='fn add(a: i32, b: i32) -> i32 {\n    a + b\n}'
DOC_ORIG='fn documented() -> bool {\n    true\n}'

CP="$WORK/rev.rs"; cp "$BASIC" "$CP"
H0="$(sha "$CP")"

resp="$(call_tool write_function '{"path":"'"$CP"'","edits":[{"kind":"func","name":"add","new_text":"fn add(a: i32, b: i32) -> i32 {\n    a + b + 0\n}"}]}')"
applied="$(jget '.result.structuredContent.result.applied' "$resp")"
if { [ "$HAVE_JQ" -eq 1 ] && [ "$applied" = "true" ]; } || { [ "$HAVE_JQ" -ne 1 ] && printf '%s' "$resp" | grep -q '"applied":true'; }; then
  [ "$(sha "$CP")" != "$H0" ] && ok "mcp_write_A" || fail "mcp_write_A" "file unchanged"
else
  fail "mcp_write_A" "not applied"
fi

resp="$(call_tool write_function '{"path":"'"$CP"'","edits":[{"kind":"func","name":"documented","new_text":"fn documented() -> bool {\n    false\n}"}]}')"
if printf '%s' "$resp" | grep -q '"applied":true'; then
  ok "mcp_write_B"; else fail "mcp_write_B" "not applied"; fi

resp="$(call_tool write_function '{"path":"'"$CP"'","edits":[{"kind":"func","name":"add","new_text":"'"$ADD_ORIG"'"},{"kind":"func","name":"documented","new_text":"'"$DOC_ORIG"'"}]}')"
printf '%s' "$resp" | grep -q '"applied":true' || fail "mcp_write_restore_batch" "restore not applied"
if [ "$(sha "$CP")" = "$H0" ]; then
  ok "mcp_write_reversible"; else fail "mcp_write_reversible" "final hash != original"; fi

# --- 5. CLI write (example of `crwai wr`), then restore --------------------
CP2="$WORK/cli.rs"; cp "$BASIC" "$CP2"
H2="$(sha "$CP2")"
go run ./cmd/crwai wr "$CP2" -n add -k func -t 'fn add(a: i32, b: i32) -> i32 { a + b + 1 }' >/dev/null 2>&1
if [ "$(sha "$CP2")" != "$H2" ] && go run ./cmd/crwai sig "$CP2" >/dev/null 2>&1; then
  ok "cli_write_changes_and_parses"; else fail "cli_write_changes_and_parses" "no change or broke parse"; fi
go run ./cmd/crwai wr "$CP2" -n add -k func -t "$(printf 'fn add(a: i32, b: i32) -> i32 {\n    a + b\n}')" >/dev/null 2>&1
[ "$(sha "$CP2")" = "$H2" ] && ok "cli_write_restore" || fail "cli_write_restore" "did not restore"

# --- 6. MCP all-or-nothing (valid + broken edit -> nothing applied) -------
CP3="$WORK/atomic.rs"; cp "$BASIC" "$CP3"
H3="$(sha "$CP3")"
resp="$(call_tool write_function '{"path":"'"$CP3"'","edits":[{"kind":"func","name":"add","new_text":"fn add(a: i32, b: i32) -> i32 {\n    a + b + 2\n}"},{"kind":"func","name":"documented","new_text":"fn documented() -> bool { true "}]}')"
iserr="$(jget '.result.isError' "$resp")"
names_broken=1; printf '%s' "$resp" | grep -q 'documented' || names_broken=0
if { [ "$iserr" = "true" ] || printf '%s' "$resp" | grep -q '"isError":true'; } && [ "$(sha "$CP3")" = "$H3" ]; then
  if [ "$names_broken" -eq 1 ]; then ok "mcp_atomic_all_or_nothing"; else fail "mcp_atomic_all_or_nothing" "rejected but did not name the broken edit"; fi
else
  fail "mcp_atomic_all_or_nothing" "batch not rejected or file changed"
fi

# --- 7. MCP single parse-rejection ----------------------------------------
CP4="$WORK/reject.rs"; cp "$BASIC" "$CP4"
H4="$(sha "$CP4")"
resp="$(call_tool write_function '{"path":"'"$CP4"'","edits":[{"kind":"func","name":"add","new_text":"fn add(a: i32) -> i32 { a + }"}]}')"
if { printf '%s' "$resp" | grep -q '"isError":true'; } && [ "$(sha "$CP4")" = "$H4" ]; then
  ok "mcp_parse_rejection"; else fail "mcp_parse_rejection" "not rejected or file changed"; fi

# --- summary ---------------------------------------------------------------
echo
echo "Passed: $PASS/$TOTAL"
[ "$PASS" -eq "$TOTAL" ] && exit 0 || exit 1
