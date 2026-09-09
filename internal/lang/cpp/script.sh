#!/usr/bin/env bash
#
# script.sh — end-to-end smoke test for the C++ language support.
#
# It exercises three layers, in order:
#   1. the Go unit tests for internal/lang/cpp;
#   2. the human CLI (cmd/crwai subcommands: langs, version, signatures,
#      function, body, struct, interface, write) — with real example invocations;
#   3. the MCP stdio protocol (the bare binary speaks JSON-RPC over stdin/stdout):
#      reads, a reversible 3-write batch, all-or-nothing atomicity, parse-rejection.
#
# Properties:
#   - Never touches examples/cpp/ — always works on a throwaway copy under mktemp.
#   - Idempotent: running it twice yields the same result.
#   - No network access.
#   - Runs ALL tests (no early exit), prints [OK]/[FAIL] per test and a final
#     "Passed: X/Y"; exits 0 iff every test passed, else 1.
#
# Note on the binary: the MCP server is the bare `cmd/crwai` binary run with no
# subcommand. For speed we compile it ONCE from source into the temp workdir (a
# throwaway, never installed and never committed) and reuse it for every CLI and
# MCP call — recompiling on each of ~20 calls via `go run` would be needlessly slow.

set -u

# Resolve repo root (two levels up from internal/lang/cpp).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
cd "$REPO_ROOT"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

PASS=0
TOTAL=0
ok()   { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo "[OK] $1"; }
fail() { TOTAL=$((TOTAL+1)); echo "[FAIL] $1: ${2:-}"; }

# sha256 of a file, portable across macOS (shasum) and Linux (sha256sum).
sha() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}';
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# ---------------------------------------------------------------------------
# 0. Build the binary once from source into the temp workdir.
# ---------------------------------------------------------------------------
BIN="$WORK/crwai"
if go build -o "$BIN" ./cmd/crwai 2>"$WORK/build.log"; then
  ok "build cmd/crwai from source"
else
  fail "build cmd/crwai from source" "$(tail -1 "$WORK/build.log")"
  echo "Passed: $PASS/$TOTAL"
  exit 1   # nothing else can run without the binary
fi

# Stage a private copy of the examples.
SRC="$WORK/examples"
mkdir -p "$SRC"
cp examples/cpp/*.cpp "$SRC/"
SHAPES="$SRC/shapes.cpp"
BIG="$SRC/big.cpp"

# ---------------------------------------------------------------------------
# 1. Go unit tests (first step).
# ---------------------------------------------------------------------------
if go test ./internal/lang/cpp/... >"$WORK/gotest.log" 2>&1; then
  ok "go test ./internal/lang/cpp/..."
else
  fail "go test ./internal/lang/cpp/..." "see $WORK/gotest.log"
  cat "$WORK/gotest.log"
fi

# ---------------------------------------------------------------------------
# 2. CLI (cmd/crwai) — real example invocations.
# ---------------------------------------------------------------------------
echo "--- CLI checks (cmd/crwai) ---"

# NOTE: the cobra CLI prints its rendered output to STDERR (cobra's default
# OutOrStderr), so the checks below merge stderr into stdout with 2>&1 before
# grepping. Examples double as a quick manual reference for using the tool.

# langs must advertise cpp with its extensions.
if "$BIN" langs 2>&1 | grep -q 'cpp'; then
  ok "cli: langs lists cpp"
else
  fail "cli: langs lists cpp"
fi

# version prints the product version.
if "$BIN" version 2>&1 | grep -qi 'crwai'; then
  ok "cli: version"
else
  fail "cli: version"
fi

# signatures (alias sig/ls) lists the file's symbols.
if "$BIN" signatures "$SHAPES" 2>&1 | grep -q 'area'; then
  ok "cli: signatures shapes.cpp"
else
  fail "cli: signatures shapes.cpp"
fi

# function: free area(double) vs Circle::area() — disambiguation by --container.
free_area="$("$BIN" function "$SHAPES" area 2>&1)"
circ_area="$("$BIN" function "$SHAPES" area --container Circle 2>&1)"
if echo "$free_area" | grep -q 'PI \* radius \* radius'; then
  ok "cli: function area (free)"
else
  fail "cli: function area (free)"
fi
if echo "$circ_area" | grep -q 'radius_ \* radius_'; then
  ok "cli: function area -c Circle"
else
  fail "cli: function area -c Circle"
fi

# body (alias bd): just the body of a method.
if "$BIN" body "$SHAPES" area -c Circle 2>&1 | grep -q 'return PI \* radius_'; then
  ok "cli: body area -c Circle"
else
  fail "cli: body area -c Circle"
fi

# struct (alias st): full class definition.
if "$BIN" struct "$SHAPES" Circle 2>&1 | grep -q 'class Circle'; then
  ok "cli: struct Circle"
else
  fail "cli: struct Circle"
fi

# interface: C++ has none — the command must fail with the typed message.
if "$BIN" interface "$SHAPES" Circle >/dev/null 2>"$WORK/iface.err"; then
  fail "cli: interface rejected" "command unexpectedly succeeded"
else
  if grep -qi 'interfaces are not a C++ construct' "$WORK/iface.err"; then
    ok "cli: interface rejected (typed error)"
  else
    fail "cli: interface rejected" "$(cat "$WORK/iface.err")"
  fi
fi

# write round-trip via CLI: modify area, then restore it, hash must match.
CLI_COPY="$WORK/cli_shapes.cpp"
cp "$SHAPES" "$CLI_COPY"
cli_sha0="$(sha "$CLI_COPY")"

# The whole-symbol span the writer replaces is [doc, declaration-end) and ends at
# the closing brace WITHOUT a trailing newline, so the fixture files must not add
# one (printf, not a heredoc) or the round-trip would accumulate bytes.
printf '%s' '// area computes the area of a circle from its radius. This FREE function shares
// its name with Circle::area to exercise SymbolID disambiguation by container.
double area(double radius) {
    return PI * radius * radius;
}' > "$WORK/area_orig.txt"
printf '%s' '// area computes the area of a circle from its radius. This FREE function shares
// its name with Circle::area to exercise SymbolID disambiguation by container.
double area(double radius) {
    return 2.0 * PI * radius * radius;
}' > "$WORK/area_mod.txt"

"$BIN" write "$CLI_COPY" -n area -f "$WORK/area_mod.txt" >/dev/null 2>&1
cli_sha1="$(sha "$CLI_COPY")"
"$BIN" write "$CLI_COPY" -n area -f "$WORK/area_orig.txt" >/dev/null 2>&1
cli_sha2="$(sha "$CLI_COPY")"
if [ "$cli_sha1" != "$cli_sha0" ] && [ "$cli_sha2" = "$cli_sha0" ]; then
  ok "cli: write round-trip (modify then restore)"
else
  fail "cli: write round-trip" "sha0=$cli_sha0 sha1=$cli_sha1 sha2=$cli_sha2"
fi

# ---------------------------------------------------------------------------
# 3. MCP stdio protocol.
# ---------------------------------------------------------------------------
echo "--- MCP stdio checks ---"

INIT='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"script","version":"0"}}}'
INITED='{"jsonrpc":"2.0","method":"notifications/initialized"}'
MCP_WAIT=2

# mcp_call <tools/call-json> -> server stdout (all JSON-RPC response lines).
# stdin is held open with a short sleep so the server flushes responses before EOF.
mcp_call() {
  { printf '%s\n' "$INIT" "$INITED" "$1"; sleep "$MCP_WAIT"; } | "$BIN" 2>/dev/null
}

# call_tool <name> <arguments-json> -> response
call_tool() {
  mcp_call '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"'"$1"'","arguments":'"$2"'}}'
}

MCP_BIG="$WORK/mcp_big.cpp"
cp "$BIG" "$MCP_BIG"

# 3a. Reads.
if call_tool list_signatures '{"path":"'"$MCP_BIG"'"}' | grep -q '"name":"add"'; then
  ok "mcp: list_signatures"
else
  fail "mcp: list_signatures"
fi

if call_tool get_function '{"path":"'"$MCP_BIG"'","name":"add"}' | grep -q 'return a + b;'; then
  ok "mcp: get_function add"
else
  fail "mcp: get_function add"
fi

if call_tool get_function_body '{"path":"'"$MCP_BIG"'","name":"sub"}' | grep -q 'return a - b;'; then
  ok "mcp: get_function_body sub"
else
  fail "mcp: get_function_body sub"
fi

if call_tool read_struct '{"path":"'"$MCP_BIG"'","name":"Stack"}' | grep -q 'class Stack'; then
  ok "mcp: read_struct Stack"
else
  fail "mcp: read_struct Stack"
fi

# read_interface must report the typed error (C++ has no interfaces).
if call_tool read_interface '{"path":"'"$MCP_BIG"'","name":"Stack"}' | grep -q 'interfaces are not a C++ construct'; then
  ok "mcp: read_interface rejected (typed error)"
else
  fail "mcp: read_interface rejected"
fi

# 3b. Reversible 3-write batch (rule of reversibility).
mcp_sha0="$(sha "$MCP_BIG")"
A_MOD='{"kind":"func","name":"add","new_text":"// add returns a + b.\nint add(int a, int b) { return a + b + 0; }"}'
B_MOD='{"kind":"func","name":"sub","new_text":"// sub returns a - b.\nint sub(int a, int b) { return a - b - 0; }"}'
A_ORIG='{"kind":"func","name":"add","new_text":"// add returns a + b.\nint add(int a, int b) { return a + b; }"}'
B_ORIG='{"kind":"func","name":"sub","new_text":"// sub returns a - b.\nint sub(int a, int b) { return a - b; }"}'

# Write #1: modify A.
call_tool write_function '{"path":"'"$MCP_BIG"'","edits":['"$A_MOD"']}' >/dev/null
mcp_sha1="$(sha "$MCP_BIG")"
# Write #2: modify B.
call_tool write_function '{"path":"'"$MCP_BIG"'","edits":['"$B_MOD"']}' >/dev/null
mcp_sha2="$(sha "$MCP_BIG")"
# Write #3: restore A and B in a single batch.
call_tool write_function '{"path":"'"$MCP_BIG"'","edits":['"$A_ORIG"','"$B_ORIG"']}' >/dev/null
mcp_sha3="$(sha "$MCP_BIG")"

if [ "$mcp_sha1" != "$mcp_sha0" ] && [ "$mcp_sha2" != "$mcp_sha1" ] && [ "$mcp_sha3" = "$mcp_sha0" ]; then
  ok "mcp: reversible 3-write batch (hash restored)"
else
  fail "mcp: reversible 3-write batch" "sha0=$mcp_sha0 sha1=$mcp_sha1 sha2=$mcp_sha2 sha3=$mcp_sha3"
fi

# 3c. All-or-nothing atomicity: one valid + one broken edit.
mcp_sha_pre="$(sha "$MCP_BIG")"
BROKEN='{"kind":"func","name":"sub","new_text":"int sub(int a, int b { return a - b; }"}'
atom_resp="$(call_tool write_function '{"path":"'"$MCP_BIG"'","edits":['"$A_MOD"','"$BROKEN"']}')"
mcp_sha_post="$(sha "$MCP_BIG")"
if [ "$mcp_sha_post" = "$mcp_sha_pre" ] && echo "$atom_resp" | grep -q 'invalid syntax'; then
  if echo "$atom_resp" | grep -q 'invalid syntax: sub'; then
    ok "mcp: atomicity (file untouched, culprit 'sub' named)"
  else
    ok "mcp: atomicity (file untouched, syntax error reported)"
  fi
else
  fail "mcp: atomicity" "sha_pre=$mcp_sha_pre sha_post=$mcp_sha_post resp=$atom_resp"
fi

# 3d. Single parse-rejection.
mcp_sha_pre2="$(sha "$MCP_BIG")"
rej_resp="$(call_tool write_function '{"path":"'"$MCP_BIG"'","edits":[{"kind":"func","name":"add","new_text":"int add(int a, int b { return 0; }"}]}')"
mcp_sha_post2="$(sha "$MCP_BIG")"
if [ "$mcp_sha_post2" = "$mcp_sha_pre2" ] && echo "$rej_resp" | grep -q 'invalid syntax'; then
  ok "mcp: parse-rejection (single, file untouched)"
else
  fail "mcp: parse-rejection" "sha_pre=$mcp_sha_pre2 sha_post=$mcp_sha_post2 resp=$rej_resp"
fi

# ---------------------------------------------------------------------------
# Summary.
# ---------------------------------------------------------------------------
echo "Passed: $PASS/$TOTAL"
[ "$PASS" -eq "$TOTAL" ]
