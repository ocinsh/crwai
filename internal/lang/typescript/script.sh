#!/usr/bin/env bash
#
# End-to-end test harness for the TypeScript language of crwai.
#
# It exercises two surfaces, in order:
#   1. the Go unit tests for internal/lang/typescript
#   2. the MCP stdio server (the bare binary speaking JSON-RPC over stdin/stdout),
#      run via `go run ./cmd/crwai` — never building or installing a binary.
#
# The MCP reads are checked on both a .ts file (pure TypeScript grammar) and a
# .tsx file (TSX grammar), then the write pipeline is exercised for reversibility,
# all-or-nothing atomicity, and single broken-write rejection.
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
MCPTMP=$(mktemp -d)

cleanup() {
	# Close MCP fds and reap the server if the session was opened.
	exec 8>&- 2>/dev/null || true
	exec 9<&- 2>/dev/null || true
	[ -n "${SRVPID:-}" ] && wait "$SRVPID" 2>/dev/null
	rm -rf "$WORK" "$MCPTMP"
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# 1. Go unit tests
# ---------------------------------------------------------------------------
if go test ./internal/lang/typescript/... >"$MCPTMP/gotest.log" 2>&1; then
	pass "go test ./internal/lang/typescript/..."
else
	fail "go test ./internal/lang/typescript/..." "$(tail -1 "$MCPTMP/gotest.log")"
fi

# Warm the build cache so the timed `go run` server start below is fast (this
# compiles but produces no installed/output binary — the prompt forbids building
# or installing an artefact, not exercising the compiler via `go run`).
go run ./cmd/crwai version >/dev/null 2>&1 || true

# Fresh, isolated copies of the examples for every test.
cp examples/typescript/*.ts "$WORK/"
cp examples/typescript/*.tsx "$WORK/"
MATH="$WORK/math.ts"
SHAPES="$WORK/shapes.ts"
WIDGET="$WORK/widget.tsx"

# Original whole-symbol texts (used to restore files to a byte-identical state).
# A top-level (non-exported) function symbol spans the declaration only — its doc
# comment sits outside the replaced range and is never touched.
ADD_ORIG=$'function add(a: number, b: number): number {\n  return a + b;\n}'
SUB_ORIG=$'function subtract(a: number, b: number): number {\n  return a - b;\n}'

# ---------------------------------------------------------------------------
# 2-5. MCP stdio server (driven through FIFOs)
# ---------------------------------------------------------------------------
SRVPID=""
RESP=""
RID=1

mcp_start() {
	local fin="$MCPTMP/in" fout="$MCPTMP/out"
	mkfifo "$fin" "$fout"
	go run ./cmd/crwai <"$fin" >"$fout" 2>/dev/null &
	SRVPID=$!
	exec 8>"$fin"
	exec 9<"$fout"
	printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"crwai-script","version":"1"}}}' >&8
	# The first read also waits out the one-time `go run` compile; allow generous time.
	IFS= read -r -t 90 RESP <&9 || return 1
	printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}' >&8
	case "$RESP" in *'"result"'*) return 0 ;; *) return 1 ;; esac
}

# mcp_call NAME ARGS_JSON — sets RESP to the response line for the call.
mcp_call() {
	RID=$((RID + 1))
	printf '{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"%s","arguments":%s}}\n' "$RID" "$1" "$2" >&8
	IFS= read -r -t 20 RESP <&9 || RESP=""
}

if ! command -v jq >/dev/null 2>&1; then
	fail "mcp: prerequisites" "jq not available"
elif mcp_start; then
	pass "mcp: initialize handshake"

	# --- 2a. reads on a .ts file (pure TypeScript grammar) ---
	mcp_call list_signatures "$(jq -nc --arg p "$MATH" '{path:$p}')"
	assert "mcp[.ts]: list_signatures" "$(printf '%s' "$RESP" | jq -rc '.result.structuredContent.signatures[].Name' 2>/dev/null | tr '\n' ' ')" "multiply"

	mcp_call get_function "$(jq -nc --arg p "$MATH" '{path:$p, name:"multiply"}')"
	assert "mcp[.ts]: get_function" "$(printf '%s' "$RESP" | jq -r '.result.structuredContent.function' 2>/dev/null)" "return a * b"

	mcp_call get_function_body "$(jq -nc --arg p "$MATH" '{path:$p, name:"add"}')"
	assert "mcp[.ts]: get_function_body" "$(printf '%s' "$RESP" | jq -r '.result.structuredContent.body' 2>/dev/null)" "return a + b"

	mcp_call read_struct "$(jq -nc --arg p "$SHAPES" '{path:$p, name:"Circle"}')"
	assert "mcp[.ts]: read_struct (class)" "$(printf '%s' "$RESP" | jq -r '.result.structuredContent.definition' 2>/dev/null)" "class Circle"

	mcp_call read_interface "$(jq -nc --arg p "$SHAPES" '{path:$p, name:"Shape"}')"
	assert "mcp[.ts]: read_interface" "$(printf '%s' "$RESP" | jq -r '.result.structuredContent.definition' 2>/dev/null)" "interface Shape"

	# --- 2b. reads on a .tsx file (TSX grammar, JSX-aware) ---
	mcp_call list_signatures "$(jq -nc --arg p "$WIDGET" '{path:$p}')"
	assert "mcp[.tsx]: list_signatures" "$(printf '%s' "$RESP" | jq -rc '.result.structuredContent.signatures[].Name' 2>/dev/null | tr '\n' ' ')" "Greeting"

	mcp_call get_function_body "$(jq -nc --arg p "$WIDGET" '{path:$p, name:"Greeting"}')"
	assert "mcp[.tsx]: get_function_body (JSX)" "$(printf '%s' "$RESP" | jq -r '.result.structuredContent.body' 2>/dev/null)" 'className="greeting"'

	mcp_call read_struct "$(jq -nc --arg p "$WIDGET" '{path:$p, name:"Panel"}')"
	assert "mcp[.tsx]: read_struct (class)" "$(printf '%s' "$RESP" | jq -r '.result.structuredContent.definition' 2>/dev/null)" "class Panel"

	# --- 3. three-write reversibility ---
	REV="$WORK/rev.ts"
	cp examples/typescript/math.ts "$REV"
	H0=$(sha "$REV")

	mcp_call write_function "$(jq -nc --arg p "$REV" --arg t $'function add(a: number, b: number): number {\n  return b + a;\n}' '{path:$p, edits:[{kind:"func", name:"add", new_text:$t}]}')"
	W1=$(printf '%s' "$RESP" | jq -r '.result.structuredContent.result.Applied' 2>/dev/null)
	H1=$(sha "$REV")
	if [ "$W1" = "true" ] && [ "$H1" != "$H0" ]; then pass "mcp: write #1 (A changed)"; else fail "mcp: write #1 (A changed)" "applied=$W1 hashChanged=$([ "$H1" != "$H0" ] && echo yes || echo no)"; fi

	mcp_call write_function "$(jq -nc --arg p "$REV" --arg t $'function subtract(a: number, b: number): number {\n  return -(b - a);\n}' '{path:$p, edits:[{kind:"func", name:"subtract", new_text:$t}]}')"
	W2=$(printf '%s' "$RESP" | jq -r '.result.structuredContent.result.Applied' 2>/dev/null)
	assert "mcp: write #2 (B changed)" "$W2" "true"

	mcp_call write_function "$(jq -nc --arg p "$REV" --arg a "$ADD_ORIG" --arg b "$SUB_ORIG" '{path:$p, edits:[{kind:"func", name:"add", new_text:$a}, {kind:"func", name:"subtract", new_text:$b}]}')"
	W3=$(printf '%s' "$RESP" | jq -r '.result.structuredContent.result.Applied' 2>/dev/null)
	H3=$(sha "$REV")
	if [ "$W3" = "true" ] && [ "$H3" = "$H0" ]; then pass "mcp: write #3 restores original (hash matches)"; else fail "mcp: write #3 restores original" "applied=$W3 hash $([ "$H3" = "$H0" ] && echo match || echo mismatch)"; fi

	# --- 4. all-or-nothing atomicity (one good edit + one broken edit) ---
	ATOM="$WORK/atom.ts"
	cp examples/typescript/math.ts "$ATOM"
	HA=$(sha "$ATOM")
	mcp_call write_function "$(jq -nc --arg p "$ATOM" --arg ok $'function subtract(a: number, b: number): number {\n  return a - b - 9;\n}' '{path:$p, edits:[{kind:"func", name:"subtract", new_text:$ok}, {kind:"func", name:"add", new_text:"function add(a: number, b: number): number { return a +"}]}')"
	ERR=$(printf '%s' "$RESP" | jq -r '.result.isError' 2>/dev/null)
	TXT=$(printf '%s' "$RESP" | jq -r '.result.content[0].text' 2>/dev/null)
	HA2=$(sha "$ATOM")
	if [ "$ERR" = "true" ] && [ "$HA2" = "$HA" ]; then
		case "$TXT" in *add*) pass "mcp: atomic batch rejected, file intact, error names cause" ;; *) fail "mcp: atomic batch" "rejected but error text does not name the culprit: $TXT" ;; esac
	else
		fail "mcp: atomic batch" "isError=$ERR hash $([ "$HA2" = "$HA" ] && echo intact || echo changed)"
	fi

	# --- 5. single broken-write rejection ---
	REJ="$WORK/rej.ts"
	cp examples/typescript/math.ts "$REJ"
	HR=$(sha "$REJ")
	mcp_call write_function "$(jq -nc --arg p "$REJ" '{path:$p, edits:[{kind:"func", name:"add", new_text:"function add(a: number, b: number): number {{{"}]}')"
	ERR2=$(printf '%s' "$RESP" | jq -r '.result.isError' 2>/dev/null)
	HR2=$(sha "$REJ")
	if [ "$ERR2" = "true" ] && [ "$HR2" = "$HR" ]; then pass "mcp: broken write rejected, file intact"; else fail "mcp: broken write" "isError=$ERR2 hash $([ "$HR2" = "$HR" ] && echo intact || echo changed)"; fi
else
	fail "mcp: session" "could not start MCP server"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo
echo "Passed: $PASS/$TOTAL"
[ "$PASS" = "$TOTAL" ]
