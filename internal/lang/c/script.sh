#!/usr/bin/env bash
#
# End-to-end test for the C language support of crwai.
#
# It exercises three layers, all against a TEMPORARY COPY of examples/c (the
# originals are never touched):
#
#   1. the Go unit tests for internal/lang/c
#   2. the human CLI (cmd/crwai subcommands) — see the cheat-sheet below
#   3. the MCP stdio server (bare `crwai` speaks JSON-RPC over stdin/stdout)
#
# Every check prints "[OK] <name>" or "[FAIL] <name>: <reason>"; the script runs
# ALL checks (no early exit), prints "Passed: X/Y", and exits 0 iff all passed.
# It is idempotent (fresh temp dir each run) and makes no network calls.
#
# CLI cheat-sheet (file paths are positional; see cmd/crwai/*.go):
#   crwai langs                       # list languages + extensions   (alias: lng)
#   crwai sig   <file>                # list top-level signatures      (alias: ls)
#   crwai fn    <file> <name> [-c C]  # whole function (doc+sig+body)  (alias: function)
#   crwai bd    <file> <name> [-c C]  # function body only             (alias: body)
#   crwai st    <file> <name>         # struct definition              (alias: struct)
#   crwai iface <file> <name>         # interface definition           (alias: interface)
#   crwai wr    <file> -n N [-k func] (--text T | --from F)  # surgical write (alias: write)
#   crwai                             # no subcommand => MCP server over stdio

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
cd "$ROOT" || exit 2

PASS=0
TOTAL=0
ok()   { TOTAL=$((TOTAL + 1)); PASS=$((PASS + 1)); echo "[OK] $1"; }
fail() { TOTAL=$((TOTAL + 1)); echo "[FAIL] $1: $2"; }

# --- sha256 helper (macOS shasum / Linux sha256sum) ------------------------
if command -v sha256sum >/dev/null 2>&1; then
	sha() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
	sha() { shasum -a 256 "$1" | awk '{print $1}'; }
else
	echo "[FAIL] setup: no sha256sum/shasum available"; echo "Passed: 0/1"; exit 1
fi

# --- keep stdin open briefly so the stdio server can flush responses -------
hold() {
	if command -v perl >/dev/null 2>&1; then perl -e 'select(undef,undef,undef,0.9)'
	elif command -v python3 >/dev/null 2>&1; then python3 -c 'import time;time.sleep(0.9)'
	else sleep 1; fi
}

# --- temp workspace (copy of the examples) ---------------------------------
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
cp "$ROOT"/examples/c/*.c "$WORK"/ 2>/dev/null
SIMPLE="$WORK/simple.c"
SHAPES="$WORK/shapes.c"
COMPLEX="$WORK/complex.c"

# Build a throwaway server/CLI binary once (fast, deterministic, no install;
# removed with the temp dir). CGO is required — never CGO_ENABLED=0.
BIN="$WORK/crwai"
if ! go build -o "$BIN" ./cmd/crwai 2>"$WORK/build.log"; then
	echo "[FAIL] build cmd/crwai:"; sed 's/^/    /' "$WORK/build.log"
	echo "Passed: 0/1"; exit 1
fi

# mcp <id> <tool> <args-json> : run one tools/call against a fresh server and
# echo the single response line whose id matches (after the initialize handshake).
mcp() {
	local id="$1" tool="$2" args="$3"
	{
		printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"script","version":"0"}}}'
		printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
		printf '{"jsonrpc":"2.0","id":%s,"method":"tools/call","params":{"name":"%s","arguments":%s}}\n' "$id" "$tool" "$args"
		hold
	} | "$BIN" 2>/dev/null | grep "\"id\":$id"
}

# json-escape a string for embedding as a JSON value (no surrounding quotes).
jsonesc() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g; s/	/\\t/g' | awk 'BEGIN{ORS=""} {if(NR>1)print "\\n"; print}'; }

# ===========================================================================
# 1. Go unit tests
# ===========================================================================
if go test ./internal/lang/c/... >"$WORK/gotest.log" 2>&1; then
	ok "go test ./internal/lang/c/..."
else
	fail "go test ./internal/lang/c/..." "$(tail -n 3 "$WORK/gotest.log" | tr '\n' ' ')"
fi

# ===========================================================================
# 2. CLI checks (cmd/crwai)
# ===========================================================================
# NOTE: the CLI renders through cobra's cmd.Println, which writes to stderr by
# default, so every CLI capture merges stderr into stdout (2>&1).

# crwai langs  -> C must be listed with its extensions.
out="$("$BIN" langs 2>&1)"
if printf '%s' "$out" | grep -q '\.h'; then ok "cli langs lists C (.c/.h)"; else fail "cli langs lists C" "no .h extension in output"; fi

# crwai sig <file>  -> complex.c lists many symbols.
out="$("$BIN" sig "$COMPLEX" 2>&1)"
if printf '%s' "$out" | grep -q 'imax' && printf '%s' "$out" | grep -q 'fib'; then
	ok "cli sig complex.c (imax, fib present)"
else
	fail "cli sig complex.c" "expected symbols not listed"
fi

# crwai fn <file> add  -> whole function with signature.
if "$BIN" fn "$SIMPLE" add 2>&1 | grep -q 'int add(int a, int b)'; then
	ok "cli fn simple.c add"
else
	fail "cli fn simple.c add" "signature not found"
fi

# crwai bd <file> add  -> body only.
if "$BIN" bd "$SIMPLE" add 2>&1 | grep -q 'return a + b'; then
	ok "cli bd simple.c add"
else
	fail "cli bd simple.c add" "body not found"
fi

# crwai st <file> Point  -> struct definition.
if "$BIN" st "$SHAPES" Point 2>&1 | grep -q 'struct Point'; then
	ok "cli st shapes.c Point"
else
	fail "cli st shapes.c Point" "struct not found"
fi

# crwai iface <file> X  -> C has no interfaces => must fail (non-zero exit).
if "$BIN" iface "$SIMPLE" X >/dev/null 2>&1; then
	fail "cli iface rejects C" "expected non-zero exit, got success"
else
	ok "cli iface rejects C (no interface concept)"
fi

# ===========================================================================
# 3. MCP stdio: reads
# ===========================================================================
if mcp 2 list_signatures "{\"path\":\"$SIMPLE\"}" | grep -q '"add"'; then
	ok "mcp list_signatures"
else
	fail "mcp list_signatures" "add not in response"
fi

if mcp 2 get_function "{\"path\":\"$SIMPLE\",\"name\":\"add\"}" | grep -q 'int add(int a, int b)'; then
	ok "mcp get_function"
else
	fail "mcp get_function" "signature not in response"
fi

if mcp 2 get_function_body "{\"path\":\"$SIMPLE\",\"name\":\"add\"}" | grep -q 'return a + b'; then
	ok "mcp get_function_body"
else
	fail "mcp get_function_body" "body not in response"
fi

if mcp 2 read_struct "{\"path\":\"$SHAPES\",\"name\":\"Point\"}" | grep -q 'struct Point'; then
	ok "mcp read_struct"
else
	fail "mcp read_struct" "struct not in response"
fi

# read_interface on C must surface a typed error (isError).
if mcp 2 read_interface "{\"path\":\"$SIMPLE\",\"name\":\"X\"}" | grep -q '"isError":true'; then
	ok "mcp read_interface reports unsupported"
else
	fail "mcp read_interface reports unsupported" "expected isError:true"
fi

# ===========================================================================
# 4. Write batch — three writes, reversibility (hash must return to baseline)
# ===========================================================================
# Exact original whole-symbol texts (must match examples/c/simple.c).
ADD_ORIG='int add(int a, int b) {
    return a + b;
}'
SUB_ORIG='int sub(int a, int b) {
    return a - b;
}'
ADD_V1='int add(int a, int b) {
    int s = a + b;
    return s;
}'
SUB_V1='int sub(int a, int b) {
    int d = a - b;
    return d;
}'

BASE_HASH="$(sha "$SIMPLE")"

w1=$(mcp 2 write_function "{\"path\":\"$SIMPLE\",\"edits\":[{\"kind\":\"func\",\"name\":\"add\",\"new_text\":\"$(jsonesc "$ADD_V1")\"}]}")
if printf '%s' "$w1" | grep -q '"applied":true' && ! [ "$(sha "$SIMPLE")" = "$BASE_HASH" ]; then
	ok "mcp write #1 (add changed)"
else
	fail "mcp write #1 (add changed)" "not applied or file unchanged"
fi

w2=$(mcp 2 write_function "{\"path\":\"$SIMPLE\",\"edits\":[{\"kind\":\"func\",\"name\":\"sub\",\"new_text\":\"$(jsonesc "$SUB_V1")\"}]}")
if printf '%s' "$w2" | grep -q '"applied":true'; then
	ok "mcp write #2 (sub changed)"
else
	fail "mcp write #2 (sub changed)" "not applied"
fi

# Restore both in a single atomic batch.
w3=$(mcp 2 write_function "{\"path\":\"$SIMPLE\",\"edits\":[{\"kind\":\"func\",\"name\":\"add\",\"new_text\":\"$(jsonesc "$ADD_ORIG")\"},{\"kind\":\"func\",\"name\":\"sub\",\"new_text\":\"$(jsonesc "$SUB_ORIG")\"}]}")
if printf '%s' "$w3" | grep -q '"applied":true' && [ "$(sha "$SIMPLE")" = "$BASE_HASH" ]; then
	ok "mcp write #3 (batch restore -> hash matches baseline)"
else
	fail "mcp write #3 (reversibility)" "hash differs from baseline after restore"
fi

# ===========================================================================
# 5. Atomicity — a batch with one valid + one broken edit must change nothing
# ===========================================================================
BEFORE="$(sha "$SIMPLE")"
atom=$(mcp 2 write_function "{\"path\":\"$SIMPLE\",\"edits\":[{\"kind\":\"func\",\"name\":\"add\",\"new_text\":\"$(jsonesc "$ADD_V1")\"},{\"kind\":\"func\",\"name\":\"sub\",\"new_text\":\"int sub(int a, int b) { return a -\"}]}")
if printf '%s' "$atom" | grep -q '"isError":true' && [ "$(sha "$SIMPLE")" = "$BEFORE" ]; then
	# the error message should name the offending edit (sub)
	if printf '%s' "$atom" | grep -q 'sub'; then
		ok "mcp atomic batch rejected, file intact, blames 'sub'"
	else
		ok "mcp atomic batch rejected, file intact"
	fi
else
	fail "mcp atomic all-or-nothing" "file changed or no error on broken batch"
fi

# ===========================================================================
# 6. Single parse-rejection — broken body must be refused, file intact
# ===========================================================================
BEFORE="$(sha "$SIMPLE")"
rej=$(mcp 2 write_function "{\"path\":\"$SIMPLE\",\"edits\":[{\"kind\":\"func\",\"name\":\"add\",\"new_text\":\"int add(int a, int b) { return a +\"}]}")
if printf '%s' "$rej" | grep -q '"isError":true' && [ "$(sha "$SIMPLE")" = "$BEFORE" ]; then
	ok "mcp single parse-rejection, file intact"
else
	fail "mcp single parse-rejection" "file changed or no error"
fi

# ===========================================================================
echo "Passed: $PASS/$TOTAL"
[ "$PASS" -eq "$TOTAL" ]
