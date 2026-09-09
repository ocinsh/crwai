#!/usr/bin/env bash
#
# End-to-end test of the Dart language support in crwai.
#
# It exercises three layers, in order:
#   1. the Go unit tests for internal/lang/dart;
#   2. the human CLI (cmd/crwai) — one example per read subcommand plus a write;
#   3. the MCP server over stdio — the read tools, plus the three-write
#      reversibility rule, all-or-nothing atomicity, and single parse-rejection.
#
# Properties:
#   - works only on COPIES of examples/dart (never the originals);
#   - idempotent: running it twice gives the same result;
#   - no network access;
#   - prints [OK]/[FAIL] per test, never exits on the first failure, and ends
#     with "Passed: X/Y", exiting 0 iff every test passed.
#
# Usage:  bash internal/lang/dart/script.sh

set -u

# --- locate the repo root (this script lives in internal/lang/dart/) ---------
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
cd "$ROOT" || { echo "cannot cd to repo root"; exit 1; }

EX="examples/dart"
PASS=0
TOTAL=0

ok()   { PASS=$((PASS + 1)); TOTAL=$((TOTAL + 1)); echo "[OK] $1"; }
fail() { TOTAL=$((TOTAL + 1)); echo "[FAIL] $1: $2"; }

# assert_contains <name> <haystack> <needle>
assert_contains() {
  case "$2" in
    *"$3"*) ok "$1" ;;
    *)      fail "$1" "expected to contain '$3'; got: $(printf '%s' "$2" | head -c 200)" ;;
  esac
}

# assert_eq <name> <got> <want>
assert_eq() {
  if [ "$2" = "$3" ]; then ok "$1"; else fail "$1" "got '$2', want '$3'"; fi
}

sha() { shasum -a 256 "$1" | awk '{print $1}'; }

# cli runs the human front-end; build is cached by `go run`.
cli() { go run ./cmd/crwai "$@"; }

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

echo "== 1. Go unit tests =========================================="
if go test ./internal/lang/dart/... >"$WORK/gotest.log" 2>&1; then
  ok "go test ./internal/lang/dart/..."
else
  fail "go test ./internal/lang/dart/..." "$(tail -n 3 "$WORK/gotest.log" | tr '\n' ' ')"
fi

echo
echo "== 2. CLI (cmd/crwai) ========================================"
cp "$EX/shapes.dart" "$WORK/shapes.dart"
CLI_FILE="$WORK/shapes.dart"

# langs: the tool advertises Dart and its extension.
out=$(cli langs 2>&1)
assert_contains "cli langs lists dart" "$out" "dart"

# signatures: top-level symbols of the file.
out=$(cli signatures "$CLI_FILE" 2>&1)
assert_contains "cli signatures shows describe" "$out" "describe"
assert_contains "cli signatures shows Rectangle" "$out" "Rectangle"

# function (method, needs --container): whole method incl. doc.
out=$(cli function "$CLI_FILE" area --container Rectangle 2>&1)
assert_contains "cli function area@Rectangle has signature" "$out" "double area()"
assert_contains "cli function area@Rectangle has body" "$out" "return width * height"

# body: only the body block.
out=$(cli body "$CLI_FILE" area --container Rectangle 2>&1)
assert_contains "cli body area@Rectangle" "$out" "return width * height"

# struct: concrete class definition.
out=$(cli struct "$CLI_FILE" Rectangle 2>&1)
assert_contains "cli struct Rectangle" "$out" "class Rectangle extends Shape"

# interface: abstract class definition.
out=$(cli interface "$CLI_FILE" Shape 2>&1)
assert_contains "cli interface Shape" "$out" "abstract class Shape"

# write (single edit, valid): replaces the body, file changes and still parses.
before=$(sha "$CLI_FILE")
newbody='/// Computes width times height.
  @override
  double area() {
    return height * width;
  }'
if cli write "$CLI_FILE" --kind method --name area --container Rectangle --text "$newbody" >/dev/null 2>&1; then
  if [ "$(sha "$CLI_FILE")" != "$before" ] && cli signatures "$CLI_FILE" >/dev/null 2>&1; then
    ok "cli write (valid) changed the file and it still parses"
  else
    fail "cli write (valid)" "file unchanged or no longer parses"
  fi
else
  fail "cli write (valid)" "command returned non-zero"
fi

# write (single edit, broken): rejected, file left intact.
cp "$EX/shapes.dart" "$CLI_FILE"
before=$(sha "$CLI_FILE")
if cli write "$CLI_FILE" --kind method --name area --container Rectangle --text "@@@ not dart @@@" >/dev/null 2>&1; then
  fail "cli write (broken) rejected" "command unexpectedly succeeded"
else
  if [ "$(sha "$CLI_FILE")" = "$before" ]; then
    ok "cli write (broken) rejected, file intact"
  else
    fail "cli write (broken) rejected" "file changed despite rejection"
  fi
fi

# write --from: replacement text read from a file.
cp "$EX/shapes.dart" "$CLI_FILE"
printf '%s\n' "$newbody" >"$WORK/newbody.dart"
before=$(sha "$CLI_FILE")
if cli write "$CLI_FILE" -k method -n area -c Rectangle --from "$WORK/newbody.dart" >/dev/null 2>&1 \
   && [ "$(sha "$CLI_FILE")" != "$before" ]; then
  ok "cli write --from changed the file"
else
  fail "cli write --from" "command failed or file unchanged"
fi

echo
echo "== 3. MCP server over stdio =================================="
if ! command -v jq >/dev/null 2>&1; then
  echo "(jq not found — skipping the MCP section; Go tests + CLI already cover the write pipeline)"
else
  # One long-lived server. mcp_open holds stdin open (FD3) so the server never
  # sees EOF mid-conversation; mcp_call sends ONE request and reads responses
  # until the matching id comes back — which also serializes writes (only one
  # request is ever in flight, so disk mutations never race).
  MCPD=""; MCPSRV=""
  mcp_open() {
    MCPD=$(mktemp -d)
    mkfifo "$MCPD/in" "$MCPD/out"
    go run ./cmd/crwai <"$MCPD/in" >"$MCPD/out" 2>"$MCPD/err" &
    MCPSRV=$!
    exec 3>"$MCPD/in"; exec 4<"$MCPD/out"
    printf '%s\n' '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"script","version":"0"}}}' >&3
    printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}' >&3
    IFS= read -t 15 -r _ <&4   # discard the initialize result (id 0)
  }
  mcp_close() { exec 3>&- 2>/dev/null; exec 4<&- 2>/dev/null; wait "$MCPSRV" 2>/dev/null; rm -rf "$MCPD"; }
  # mcp_call <id> <request-json> -> prints the matching response line
  mcp_call() {
    local id=$1 req=$2 line rid
    printf '%s\n' "$req" >&3
    while IFS= read -t 15 -r line <&4; do
      rid=$(printf '%s' "$line" | jq -r '.id // empty' 2>/dev/null)
      if [ "$rid" = "$id" ]; then printf '%s\n' "$line"; return 0; fi
    done
    return 1
  }
  ok_text()  { printf '%s' "$1" | jq -r '.result.content[0].text' 2>/dev/null; }
  is_error() { printf '%s' "$1" | jq -r '.result.isError // false' 2>/dev/null; }

  cp "$EX/shapes.dart" "$WORK/m.dart"
  MP="$WORK/m.dart"

  mcp_open

  # --- read tools -----------------------------------------------------------
  r=$(mcp_call 1 "$(jq -cn --arg p "$MP" '{jsonrpc:"2.0",id:1,method:"tools/call",params:{name:"list_signatures",arguments:{path:$p}}}')")
  n=$(ok_text "$r" | jq -r '.signatures | length' 2>/dev/null)
  assert_eq "mcp list_signatures count" "${n:-0}" "4"

  r=$(mcp_call 2 "$(jq -cn --arg p "$MP" '{jsonrpc:"2.0",id:2,method:"tools/call",params:{name:"get_function",arguments:{path:$p,name:"area",container:"Rectangle"}}}')")
  assert_contains "mcp get_function area" "$(ok_text "$r" | jq -r '.function' 2>/dev/null)" "double area()"

  r=$(mcp_call 3 "$(jq -cn --arg p "$MP" '{jsonrpc:"2.0",id:3,method:"tools/call",params:{name:"get_function_body",arguments:{path:$p,name:"area",container:"Rectangle"}}}')")
  assert_contains "mcp get_function_body area" "$(ok_text "$r" | jq -r '.body' 2>/dev/null)" "return width * height"

  r=$(mcp_call 4 "$(jq -cn --arg p "$MP" '{jsonrpc:"2.0",id:4,method:"tools/call",params:{name:"read_struct",arguments:{path:$p,name:"Rectangle"}}}')")
  assert_contains "mcp read_struct Rectangle" "$(ok_text "$r" | jq -r '.definition' 2>/dev/null)" "class Rectangle extends Shape"

  r=$(mcp_call 5 "$(jq -cn --arg p "$MP" '{jsonrpc:"2.0",id:5,method:"tools/call",params:{name:"read_interface",arguments:{path:$p,name:"Shape"}}}')")
  assert_contains "mcp read_interface Shape" "$(ok_text "$r" | jq -r '.definition' 2>/dev/null)" "abstract class Shape"

  # --- write tools: three-write reversibility, atomicity, parse-rejection ---
  cp "$EX/shapes.dart" "$MP"
  H0=$(sha "$MP")

  ORIG_AREA='/// Computes width times height.
  @override
  double area() {
    return width * height;
  }'
  MOD_AREA='/// Computes width times height.
  @override
  double area() {
    return height * width;
  }'
  ORIG_DESC='/// Describes this rectangle.
  @override
  String describe() {
    return "rectangle";
  }'
  MOD_DESC='/// Describes this rectangle.
  @override
  String describe() {
    return "rect";
  }'

  # one_edit <id> <name> <newtext>  -> a single-edit write_function request
  one_edit() {
    jq -cn --arg p "$MP" --arg n "$2" --arg t "$3" \
      "{jsonrpc:\"2.0\",id:$1,method:\"tools/call\",params:{name:\"write_function\",arguments:{path:\$p,edits:[{kind:\"method\",name:\$n,container:\"Rectangle\",new_text:\$t}]}}}"
  }
  applied() { ok_text "$1" | jq -r '.result.applied' 2>/dev/null; }

  # Write #1: modify area. Write #2: modify describe. (both valid)
  r=$(mcp_call 11 "$(one_edit 11 area "$MOD_AREA")")
  assert_eq "mcp write #1 (area) applied" "$(applied "$r")" "true"
  r=$(mcp_call 12 "$(one_edit 12 describe "$MOD_DESC")")
  assert_eq "mcp write #2 (describe) applied" "$(applied "$r")" "true"

  # Write #3: restore BOTH in a single atomic batch (two edits, one call).
  r=$(mcp_call 13 "$(jq -cn --arg p "$MP" --arg a "$ORIG_AREA" --arg d "$ORIG_DESC" \
    '{jsonrpc:"2.0",id:13,method:"tools/call",params:{name:"write_function",arguments:{path:$p,edits:[{kind:"method",name:"area",container:"Rectangle",new_text:$a},{kind:"method",name:"describe",container:"Rectangle",new_text:$d}]}}}')")
  assert_eq "mcp write #3 (restore batch) applied" "$(applied "$r")" "true"

  # Reversibility: after the three writes the file equals the original byte-for-byte.
  assert_eq "mcp three-write reversibility (sha256)" "$(sha "$MP")" "$H0"

  # Parse-rejection (single, broken): rejected, names the symbol, file intact.
  r=$(mcp_call 14 "$(one_edit 14 area "@@@ not dart @@@")")
  assert_eq "mcp parse-rejection isError" "$(is_error "$r")" "true"
  assert_contains "mcp parse-rejection names symbol" "$(printf '%s' "$r" | jq -r '.result.content[0].text' 2>/dev/null)" "area"
  assert_eq "mcp parse-rejection left file intact" "$(sha "$MP")" "$H0"

  # All-or-nothing (batch: one valid + one broken): rejected, names the broken
  # edit (describe, not area), file untouched.
  r=$(mcp_call 15 "$(jq -cn --arg p "$MP" --arg a "$MOD_AREA" \
    '{jsonrpc:"2.0",id:15,method:"tools/call",params:{name:"write_function",arguments:{path:$p,edits:[{kind:"method",name:"area",container:"Rectangle",new_text:$a},{kind:"method",name:"describe",container:"Rectangle",new_text:"@@@ broken @@@"}]}}}')")
  assert_eq "mcp atomicity isError" "$(is_error "$r")" "true"
  assert_contains "mcp atomicity names broken edit" "$(printf '%s' "$r" | jq -r '.result.content[0].text' 2>/dev/null)" "describe"
  assert_eq "mcp atomicity left file intact" "$(sha "$MP")" "$H0"

  mcp_close
fi

echo
echo "=============================================================="
echo "Passed: $PASS/$TOTAL"
[ "$PASS" -eq "$TOTAL" ]
