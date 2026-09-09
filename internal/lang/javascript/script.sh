#!/usr/bin/env bash
#
# End-to-end test harness for the JavaScript language of crwai.
#
# It exercises three surfaces, in order:
#   1. the Go unit tests for internal/lang/javascript
#   2. the human CLI (cmd/crwai subcommands), with real example invocations
#   3. the MCP stdio server (the bare binary speaking JSON-RPC over stdin/stdout)
#
# Properties:
#   - idempotent: always works on a fresh mktemp copy of the examples, never on
#     the originals; running it twice in a row yields the same result.
#   - no network access.
#   - never exits on the first failure: every test runs, then a "Passed: X/Y"
#     summary is printed. Exit code 0 iff every test passed, else 1.
#
# Compatible with bash 3.2 (macOS default): no `coproc`, no associative arrays.
# The MCP session is driven through two FIFOs so the server's stdin stays open
# until every response has been read (closing stdin early makes the server race
# its own shutdown and drop in-flight responses).

set -u

ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
cd "$ROOT"

PASS=0
TOTAL=0
pass() { TOTAL=$((TOTAL + 1)); PASS=$((PASS + 1)); echo "[OK] $1"; }
fail() { TOTAL=$((TOTAL + 1)); echo "[FAIL] $1: $2"; }

# assert NAME HAYSTACK NEEDLE — pass iff HAYSTACK contains NEEDLE.
assert() {
	case "$2" in
	*"$3"*) pass "$1" ;;
	*) fail "$1" "expected to contain '$3', got: $(printf '%s' "$2" | tr '\n' ' ' | cut -c1-160)" ;;
	esac
}

sha() {
	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		sha256sum "$1" | awk '{print $1}'
	fi
}

WORK=$(mktemp -d)
BINDIR=$(mktemp -d)
MCPTMP=$(mktemp -d)
BIN="$BINDIR/crwai"

cleanup() {
	# Close MCP fds and reap the server if the session was opened.
	exec 8>&- 2>/dev/null || true
	exec 9<&- 2>/dev/null || true
	[ -n "${SRVPID:-}" ] && wait "$SRVPID" 2>/dev/null
	rm -rf "$WORK" "$BINDIR" "$MCPTMP"
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# 1. Go unit tests
# ---------------------------------------------------------------------------
if go test ./internal/lang/javascript/... >"$MCPTMP/gotest.log" 2>&1; then
	pass "go test ./internal/lang/javascript/..."
else
	fail "go test ./internal/lang/javascript/..." "$(tail -1 "$MCPTMP/gotest.log")"
fi

# Build the CLI/MCP binary once (faster and more reliable than `go run` per call).
if go build -o "$BIN" ./cmd/crwai >"$MCPTMP/build.log" 2>&1; then
	pass "build cmd/crwai binary"
	HAVE_BIN=1
else
	fail "build cmd/crwai binary" "$(tail -1 "$MCPTMP/build.log")"
	HAVE_BIN=0
fi

# Fresh, isolated copies of the examples for every mutating test.
cp examples/javascript/*.js "$WORK/"
MATH="$WORK/math.js"
SHAPES="$WORK/shapes.js"

# Original whole-symbol texts (used to restore files to a byte-identical state).
# A top-level function symbol spans the declaration only — its doc comment sits
# outside the replaced range and is never touched.
ADD_ORIG=$'function add(a, b) {\n  return a + b;\n}'
SUB_ORIG=$'function subtract(a, b) {\n  return a - b;\n}'

# ---------------------------------------------------------------------------
# 2. CLI checks (cmd/crwai subcommands)
# ---------------------------------------------------------------------------
if [ "$HAVE_BIN" = 1 ]; then
	# langs — the supported-language list must mention javascript.
	assert "cli: langs lists javascript" "$("$BIN" langs 2>&1)" "javascript"

	# version — prints the product name + version.
	assert "cli: version" "$("$BIN" version 2>&1)" "crwai"

	# signatures <file> — the cheap map; must list known symbols.
	SIGOUT=$("$BIN" signatures "$MATH" 2>&1)
	assert "cli: signatures lists add" "$SIGOUT" "add"
	assert "cli: signatures lists multiply" "$SIGOUT" "multiply"

	# function <file> <name> — whole function incl. body.
	assert "cli: function multiply" "$("$BIN" function "$MATH" multiply 2>&1)" "return a * b"

	# body <file> <name> — body only.
	assert "cli: body multiply" "$("$BIN" body "$MATH" multiply 2>&1)" "return a * b"

	# struct <file> <name> — a class maps to KindStruct.
	assert "cli: struct Circle" "$("$BIN" struct "$SHAPES" Circle 2>&1)" "class Circle"

	# write <file> -n <name> -t <text> — surgical, atomic replacement.
	CLIFILE="$WORK/cli.js"
	cp examples/javascript/math.js "$CLIFILE"
	WROUT=$("$BIN" write "$CLIFILE" -n subtract -t $'function subtract(a, b) {\n  return a - b - 0;\n}' 2>&1)
	assert "cli: write applied" "$WROUT" "applied"
	# The file must remain valid (re-listing succeeds and still shows 4 symbols).
	assert "cli: file still valid after write" "$("$BIN" signatures "$CLIFILE" 2>&1)" "subtract"
else
	fail "cli: checks" "binary unavailable"
fi

# ---------------------------------------------------------------------------
# 3-6. MCP stdio server
# ---------------------------------------------------------------------------
SRVPID=""
RESP=""
RID=1

mcp_start() {
	local fin="$MCPTMP/in" fout="$MCPTMP/out"
	mkfifo "$fin" "$fout"
	"$BIN" <"$fin" >"$fout" 2>/dev/null &
	SRVPID=$!
	exec 8>"$fin"
	exec 9<"$fout"
	printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"crwai-script","version":"1"}}}' >&8
	IFS= read -r -t 15 RESP <&9 || return 1
	printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}' >&8
	case "$RESP" in *'"result"'*) return 0 ;; *) return 1 ;; esac
}

# mcp_call NAME ARGS_JSON — sets RESP to the response line for the call.
mcp_call() {
	RID=$((RID + 1))
	printf '{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"%s","arguments":%s}}\n' "$RID" "$1" "$2" >&8
	IFS= read -r -t 15 RESP <&9 || RESP=""
}

if [ "$HAVE_BIN" = 1 ] && mcp_start; then
	pass "mcp: initialize handshake"

	# --- 3a. reads ---
	mcp_call list_signatures "$(jq -nc --arg p "$MATH" '{path:$p}')"
	assert "mcp: list_signatures" "$(printf '%s' "$RESP" | jq -rc '.result.structuredContent.signatures[].name' 2>/dev/null | tr '\n' ' ')" "multiply"

	mcp_call get_function "$(jq -nc --arg p "$MATH" '{path:$p, name:"multiply"}')"
	assert "mcp: get_function" "$(printf '%s' "$RESP" | jq -r '.result.structuredContent.function' 2>/dev/null)" "return a * b"

	mcp_call get_function_body "$(jq -nc --arg p "$MATH" '{path:$p, name:"add"}')"
	assert "mcp: get_function_body" "$(printf '%s' "$RESP" | jq -r '.result.structuredContent.body' 2>/dev/null)" "return a + b"

	mcp_call read_struct "$(jq -nc --arg p "$SHAPES" '{path:$p, name:"Circle"}')"
	assert "mcp: read_struct" "$(printf '%s' "$RESP" | jq -r '.result.structuredContent.definition' 2>/dev/null)" "class Circle"

	# read_interface is unsupported for JavaScript: must report an error.
	mcp_call read_interface "$(jq -nc --arg p "$SHAPES" '{path:$p, name:"Shape"}')"
	assert "mcp: read_interface unsupported" "$(printf '%s' "$RESP" | jq -r '.result.isError' 2>/dev/null)" "true"

	# --- 4. three-write reversibility ---
	REV="$WORK/rev.js"
	cp examples/javascript/math.js "$REV"
	H0=$(sha "$REV")

	mcp_call write_function "$(jq -nc --arg p "$REV" --arg t $'function add(a, b) {\n  return a + b + 1;\n}' '{path:$p, edits:[{kind:"func", name:"add", new_text:$t}]}')"
	W1=$(printf '%s' "$RESP" | jq -r '.result.structuredContent.result.applied' 2>/dev/null)
	H1=$(sha "$REV")
	if [ "$W1" = "true" ] && [ "$H1" != "$H0" ]; then pass "mcp: write #1 (A changed)"; else fail "mcp: write #1 (A changed)" "applied=$W1 hashChanged=$([ "$H1" != "$H0" ] && echo yes || echo no)"; fi

	mcp_call write_function "$(jq -nc --arg p "$REV" --arg t $'function subtract(a, b) {\n  return a - b - 1;\n}' '{path:$p, edits:[{kind:"func", name:"subtract", new_text:$t}]}')"
	W2=$(printf '%s' "$RESP" | jq -r '.result.structuredContent.result.applied' 2>/dev/null)
	assert "mcp: write #2 (B changed)" "$W2" "true"

	mcp_call write_function "$(jq -nc --arg p "$REV" --arg a "$ADD_ORIG" --arg b "$SUB_ORIG" '{path:$p, edits:[{kind:"func", name:"add", new_text:$a}, {kind:"func", name:"subtract", new_text:$b}]}')"
	W3=$(printf '%s' "$RESP" | jq -r '.result.structuredContent.result.applied' 2>/dev/null)
	H3=$(sha "$REV")
	if [ "$W3" = "true" ] && [ "$H3" = "$H0" ]; then pass "mcp: write #3 restores original (hash matches)"; else fail "mcp: write #3 restores original" "applied=$W3 hash $([ "$H3" = "$H0" ] && echo match || echo mismatch)"; fi

	# --- 5. all-or-nothing atomicity ---
	ATOM="$WORK/atom.js"
	cp examples/javascript/math.js "$ATOM"
	HA=$(sha "$ATOM")
	mcp_call write_function "$(jq -nc --arg p "$ATOM" --arg ok $'function subtract(a, b) {\n  return a - b - 9;\n}' '{path:$p, edits:[{kind:"func", name:"subtract", new_text:$ok}, {kind:"func", name:"add", new_text:"function add(a, b) { return a + "}]}')"
	ERR=$(printf '%s' "$RESP" | jq -r '.result.isError' 2>/dev/null)
	TXT=$(printf '%s' "$RESP" | jq -r '.result.content[0].text' 2>/dev/null)
	HA2=$(sha "$ATOM")
	if [ "$ERR" = "true" ] && [ "$HA2" = "$HA" ]; then
		case "$TXT" in *syntax*) pass "mcp: atomic batch rejected, file intact, error names cause" ;; *) fail "mcp: atomic batch" "rejected but error text unexpected: $TXT" ;; esac
	else
		fail "mcp: atomic batch" "isError=$ERR hash $([ "$HA2" = "$HA" ] && echo intact || echo changed)"
	fi

	# --- 6. single broken-write rejection ---
	REJ="$WORK/rej.js"
	cp examples/javascript/math.js "$REJ"
	HR=$(sha "$REJ")
	mcp_call write_function "$(jq -nc --arg p "$REJ" '{path:$p, edits:[{kind:"func", name:"add", new_text:"function add(a, b) {{{"}]}')"
	ERR2=$(printf '%s' "$RESP" | jq -r '.result.isError' 2>/dev/null)
	HR2=$(sha "$REJ")
	if [ "$ERR2" = "true" ] && [ "$HR2" = "$HR" ]; then pass "mcp: broken write rejected, file intact"; else fail "mcp: broken write" "isError=$ERR2 hash $([ "$HR2" = "$HR" ] && echo intact || echo changed)"; fi
else
	[ "$HAVE_BIN" = 1 ] && fail "mcp: session" "could not start MCP server"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo
echo "Passed: $PASS/$TOTAL"
[ "$PASS" = "$TOTAL" ]
