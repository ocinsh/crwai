#!/usr/bin/env bash
#
# End-to-end test harness for the Go language implementation.
#
# It exercises three layers, in order:
#   1. the Go unit tests (go test ./internal/lang/golang/...);
#   2. the MCP server over stdio (the bare binary speaks JSON-RPC on stdin/stdout)
#      — every read tool, the three-write reversibility rule, all-or-nothing
#      atomicity, and single-edit parse-rejection;
#   3. the human CLI (cmd/crwai) — one example per subcommand.
#
# Properties:
#   - Never touches examples/golang/** — always works on a mktemp copy.
#   - No network access.
#   - Idempotent: two consecutive runs produce the same result.
#   - Runs ALL tests (no early exit); prints [OK]/[FAIL] per test, a final
#     "Passed: X/Y" summary, and exits 0 iff every test passed.
#
# The server is launched with `go run ./cmd/crwai` (no build, no install). The
# bare command — with no subcommand — is the MCP server.

set -u

REPO="$(cd "$(dirname "$0")/../../.." && pwd)"
PASS=0
TOTAL=0
WORK="$(mktemp -d)"
trap 'cleanup' EXIT

cleanup() {
	# Close session fds if still open; remove the scratch copy.
	exec 3>&- 2>/dev/null || true
	exec 4<&- 2>/dev/null || true
	[ -n "${SESS_PID:-}" ] && wait "$SESS_PID" 2>/dev/null
	rm -rf "$WORK"
}

ok()   { echo "[OK] $1"; PASS=$((PASS + 1)); TOTAL=$((TOTAL + 1)); }
fail() { echo "[FAIL] $1: $2"; TOTAL=$((TOTAL + 1)); }

# check NAME NEEDLE HAYSTACK — pass iff HAYSTACK contains NEEDLE.
check() {
	case "$3" in
	*"$2"*) ok "$1" ;;
	*) fail "$1" "expected to contain '$2' (got: $(printf '%.200s' "$3"))" ;;
	esac
}

# refute NAME NEEDLE HAYSTACK — pass iff HAYSTACK does NOT contain NEEDLE.
refute() {
	case "$3" in
	*"$2"*) fail "$1" "did not expect '$2' (got: $(printf '%.200s' "$3"))" ;;
	*) ok "$1" ;;
	esac
}

sha() {
	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		sha256sum "$1" | awk '{print $1}'
	fi
}

# ---------------------------------------------------------------------------
# Step 1: Go unit tests.
# ---------------------------------------------------------------------------
echo "== Go unit tests =="
if (cd "$REPO" && go test ./internal/lang/golang/... >"$WORK/gotest.log" 2>&1); then
	ok "go test ./internal/lang/golang/..."
else
	fail "go test ./internal/lang/golang/..." "see output below"
	sed 's/^/    /' "$WORK/gotest.log"
fi

# Fresh copy of the examples to operate on (read + write tests).
EX="$WORK/examples"
mkdir -p "$EX"
cp "$REPO"/examples/golang/*.go "$EX"/
BASIC="$EX/basic.go"
METHODS="$EX/methods.go"
TYPES="$EX/types.go"
BIG="$EX/big.go"

# ---------------------------------------------------------------------------
# MCP session helpers (newline-delimited JSON-RPC over stdio, via FIFOs).
#
# Real MCP clients keep stdin open while awaiting replies; piping a finite file
# makes the server shut down on EOF before draining queued requests. We hold the
# input FIFO open on fd 3 and read replies from fd 4, closing fd 3 only at the
# end so every request is answered.
# ---------------------------------------------------------------------------
SESS_PID=""
open_session() {
	mkfifo "$WORK/in" "$WORK/out"
	(cd "$REPO" && exec go run ./cmd/crwai) <"$WORK/in" >"$WORK/out" 2>"$WORK/srv.err" &
	SESS_PID=$!
	exec 3>"$WORK/in"
	exec 4<"$WORK/out"
	printf '%s\n' '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"script","version":"0"}}}' >&3
	IFS= read -r -t 120 INIT <&4 || INIT=""
	printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}' >&3
}

close_session() {
	exec 3>&-
	wait "$SESS_PID" 2>/dev/null
	exec 4<&-
	SESS_PID=""
	rm -f "$WORK/in" "$WORK/out"
}

# call ID NAME ARGS_JSON — send a tools/call and read one reply line into RESP.
RESP=""
call() {
	printf '%s\n' "{\"jsonrpc\":\"2.0\",\"id\":$1,\"method\":\"tools/call\",\"params\":{\"name\":\"$2\",\"arguments\":$3}}" >&3
	IFS= read -r -t 60 RESP <&4 || RESP="(no response / timeout)"
}

echo "== MCP server over stdio =="
open_session
if [ -z "$INIT" ]; then
	fail "mcp initialize" "no handshake response (server log: $(printf '%.200s' "$(cat "$WORK/srv.err" 2>/dev/null)"))"
else
	check "mcp initialize" '"result"' "$INIT"
fi

# --- read tools ---
call 1 list_signatures "{\"path\":\"$BIG\"}"
check "list_signatures big.go" "BigConst22" "$RESP"

call 2 get_function "{\"path\":\"$METHODS\",\"name\":\"Greet\",\"container\":\"Greeter\"}"
check "get_function Greeter.Greet" 'func (g *Greeter) Greet(name string) string' "$RESP"

call 3 get_function_body "{\"path\":\"$BASIC\",\"name\":\"Add\"}"
check "get_function_body Add" "return a + b" "$RESP"

call 4 read_struct "{\"path\":\"$TYPES\",\"name\":\"Point\"}"
check "read_struct Point" "type Point struct" "$RESP"

call 5 read_interface "{\"path\":\"$TYPES\",\"name\":\"Shape\"}"
check "read_interface Shape" "Area() float64" "$RESP"

# --- three-write reversibility rule ---
# These restore strings MUST be byte-identical to the original declarations in
# examples/golang/basic.go (tabs included) so the final hash matches.
ADD_ORIG='func Add(a int, b int) int {\n\treturn a + b\n}'
VAR_ORIG='func Variadic(nums ...int) int {\n\ttotal := 0\n\tfor _, n := range nums {\n\t\ttotal += n\n\t}\n\treturn total\n}'
ADD_EDIT='func Add(a int, b int) int {\n\treturn b + a\n}'
VAR_EDIT='func Variadic(nums ...int) int {\n\tsum := 0\n\tfor _, n := range nums {\n\t\tsum += n\n\t}\n\treturn sum\n}'

H0="$(sha "$BASIC")"

call 6 write_function "{\"path\":\"$BASIC\",\"edits\":[{\"kind\":\"func\",\"name\":\"Add\",\"new_text\":\"$ADD_EDIT\"}]}"
check "write #1 (Add) applied" '"Applied":true' "$RESP"
[ "$(sha "$BASIC")" != "$H0" ] && ok "write #1 changed the file" || fail "write #1 changed the file" "hash unchanged"
call 7 get_function_body "{\"path\":\"$BASIC\",\"name\":\"Add\"}"
check "write #1 still parses (re-read)" "return b + a" "$RESP"

call 8 write_function "{\"path\":\"$BASIC\",\"edits\":[{\"kind\":\"func\",\"name\":\"Variadic\",\"new_text\":\"$VAR_EDIT\"}]}"
check "write #2 (Variadic) applied" '"Applied":true' "$RESP"
call 9 get_function_body "{\"path\":\"$BASIC\",\"name\":\"Variadic\"}"
check "write #2 still parses (re-read)" "sum += n" "$RESP"

# Write #3: restore BOTH in a single atomic batch.
call 10 write_function "{\"path\":\"$BASIC\",\"edits\":[{\"kind\":\"func\",\"name\":\"Add\",\"new_text\":\"$ADD_ORIG\"},{\"kind\":\"func\",\"name\":\"Variadic\",\"new_text\":\"$VAR_ORIG\"}]}"
check "write #3 (restore batch) applied" '"Applied":true' "$RESP"
if [ "$(sha "$BASIC")" = "$H0" ]; then
	ok "reversibility: final hash equals original"
else
	fail "reversibility: final hash equals original" "hash drift after restore"
fi

# --- all-or-nothing atomicity: one valid + one broken edit ---
HB="$(sha "$BASIC")"
call 11 write_function "{\"path\":\"$BASIC\",\"edits\":[{\"kind\":\"func\",\"name\":\"Add\",\"new_text\":\"$ADD_EDIT\"},{\"kind\":\"func\",\"name\":\"Variadic\",\"new_text\":\"func Variadic(nums ...int) int { return \"}]}"
check "atomic batch rejected (isError)" '"isError":true' "$RESP"
check "atomic batch names the broken edit" "Variadic" "$RESP"
if [ "$(sha "$BASIC")" = "$HB" ]; then
	ok "atomic batch left the file untouched"
else
	fail "atomic batch left the file untouched" "file changed despite rejection"
fi

# --- single-edit parse rejection ---
HC="$(sha "$BASIC")"
call 12 write_function "{\"path\":\"$BASIC\",\"edits\":[{\"kind\":\"func\",\"name\":\"Add\",\"new_text\":\"func Add(a int, b int) int { return a + \"}]}"
check "single broken write rejected (isError)" '"isError":true' "$RESP"
if [ "$(sha "$BASIC")" = "$HC" ]; then
	ok "single rejection left the file untouched"
else
	fail "single rejection left the file untouched" "file changed despite rejection"
fi

close_session

# ---------------------------------------------------------------------------
# Step 3: human CLI (cmd/crwai). One example per subcommand, on a fresh copy.
# ---------------------------------------------------------------------------
echo "== CLI (cmd/crwai) =="
CLI="$WORK/cli"
mkdir -p "$CLI"
cp "$REPO"/examples/golang/*.go "$CLI"/
cli() { (cd "$REPO" && go run ./cmd/crwai "$@"); }

OUT="$(cli version 2>&1)";                          check "cli version" "crwai" "$OUT"
OUT="$(cli langs 2>&1)";                            check "cli langs lists go" ".go" "$OUT"
OUT="$(cli ls "$CLI/basic.go" 2>&1)";               check "cli signatures (ls)" "MultiReturn" "$OUT"
OUT="$(cli bd "$CLI/basic.go" Add 2>&1)";           check "cli body (bd)" "return a + b" "$OUT"
OUT="$(cli fn "$CLI/methods.go" Greet -c Greeter 2>&1)"; check "cli function (fn) with -c" "func (g *Greeter) Greet" "$OUT"
OUT="$(cli iface "$CLI/types.go" Shape 2>&1)";      check "cli interface (iface)" "Perimeter() float64" "$OUT"
OUT="$(cli st "$CLI/types.go" Point 2>&1)";         check "cli struct (st)" "type Point struct" "$OUT"

# write apply, then restore; verify the file returns to its original bytes.
HCLI="$(sha "$CLI/basic.go")"
printf 'func Add(a int, b int) int {\n\treturn b + a\n}' >"$CLI/new_add.txt"
printf 'func Add(a int, b int) int {\n\treturn a + b\n}' >"$CLI/orig_add.txt"
OUT="$(cli wr "$CLI/basic.go" -n Add -k func -f "$CLI/new_add.txt" 2>&1)"
check "cli write applied" "applied 1 edit" "$OUT"
[ "$(sha "$CLI/basic.go")" != "$HCLI" ] && ok "cli write changed the file" || fail "cli write changed the file" "hash unchanged"
OUT="$(cli wr "$CLI/basic.go" -n Add -k func -f "$CLI/orig_add.txt" 2>&1)"
check "cli write restore applied" "applied 1 edit" "$OUT"
if [ "$(sha "$CLI/basic.go")" = "$HCLI" ]; then
	ok "cli write round-trip restores original"
else
	fail "cli write round-trip restores original" "hash drift"
fi

# error path: write without --name must fail (non-zero exit).
if cli wr "$CLI/basic.go" -t 'x' >/dev/null 2>&1; then
	fail "cli write without --name errors" "command unexpectedly succeeded"
else
	ok "cli write without --name errors"
fi

# error path: broken syntax via CLI must be rejected, file untouched.
HD="$(sha "$CLI/basic.go")"
cli wr "$CLI/basic.go" -n Add -k func -t 'func Add(a int, b int) int { return a + ' >/dev/null 2>&1
if [ "$(sha "$CLI/basic.go")" = "$HD" ]; then
	ok "cli broken write left file untouched"
else
	fail "cli broken write left file untouched" "file changed"
fi

# ---------------------------------------------------------------------------
echo
echo "Passed: $PASS/$TOTAL"
[ "$PASS" -eq "$TOTAL" ]
