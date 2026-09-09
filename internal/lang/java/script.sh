#!/usr/bin/env bash
#
# script.sh — end-to-end collaudo of the Java language support in crwai.
#
# It exercises the implementation two ways:
#   * the CLI front-end (cmd/crwai) — the primary coverage, one example per
#     subcommand, plus write reversibility / atomicity / parse-rejection;
#   * a minimal MCP-over-stdio smoke test — initialize + tools/list + a
#     tools/call, and the all-or-nothing batch write that the CLI can't express.
#
# Properties: no network; works only on a mktemp copy of examples/java (originals
# are never touched); idempotent (run it twice, same result); never exits at the
# first failure — every check runs, then a final "Passed: X/Y" and exit 0/1.
#
# Usage:  bash internal/lang/java/script.sh

set -u

# --- locate the repo root (three levels up from this script) ----------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
cd "$REPO_ROOT"

# --- scratch area (always a copy; cleaned on exit) --------------------------
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
cp -R examples/java "$WORK/java"
BIN="$WORK/crwai"
F="$WORK/java/Greeter.java"

# --- tally helpers ----------------------------------------------------------
PASS=0
TOTAL=0
ok()  { TOTAL=$((TOTAL + 1)); PASS=$((PASS + 1)); echo "[OK] $1"; }
bad() { TOTAL=$((TOTAL + 1)); echo "[FAIL] $1: $2"; }

# small sub-second pause (perl avoids depending on a fractional-second sleep)
pause() { perl -e 'select(undef,undef,undef,0.4)'; }

hash_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# mcp: run one full MCP stdio session. The JSON-RPC request line(s) to send AFTER
# the initialize handshake are read from stdin; the server's stdout is printed.
mcp() {
  {
    printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"crwai-script","version":"0"}}}'
    pause
    printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
    pause
    cat
    pause
  } | "$BIN" 2>/dev/null
}

# --- original symbol texts (whole-symbol replacements must be byte-exact) ----
read -r -d '' GREET_ORIG <<'JAVA'
public String greet(String who) {
        return name + " greets " + who;
    }
JAVA
read -r -d '' FAREWELL_ORIG <<'JAVA'
public String farewell(String who, int times) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < times; i++) {
            sb.append("bye ").append(who).append(' ');
        }
        return sb.toString().trim();
    }
JAVA
GREET_MOD='public String greet(String who) {
        return "HI " + who;
    }'
FAREWELL_MOD='public String farewell(String who, int times) {
        return "bye";
    }'
GREET_BROKEN='public String greet(String who) { return @@@ not valid'

echo "== crwai Java collaudo =="

# ----------------------------------------------------------------------------
# 0. Build the binary once.
# ----------------------------------------------------------------------------
if go build -o "$BIN" ./cmd/crwai 2>"$WORK/build.err"; then
  ok "build cmd/crwai"
else
  bad "build cmd/crwai" "$(tr '\n' ' ' <"$WORK/build.err")"
  echo "Passed: $PASS/$TOTAL"
  exit 1
fi

# ----------------------------------------------------------------------------
# 1. Go unit tests for the language package (first step, as required).
# ----------------------------------------------------------------------------
if go test ./internal/lang/java/... >"$WORK/gotest.out" 2>&1; then
  ok "go test ./internal/lang/java/..."
else
  bad "go test ./internal/lang/java/..." "$(tail -3 "$WORK/gotest.out" | tr '\n' ' ')"
fi

# ----------------------------------------------------------------------------
# 2. CLI read commands — one example per subcommand of cmd/crwai.
# ----------------------------------------------------------------------------
out="$("$BIN" langs 2>&1)"
echo "$out" | grep -q 'java' && ok "cli langs lists java" || bad "cli langs lists java" "$out"

out="$("$BIN" sig "$WORK/java/Catalog.java" 2>&1)"
# one tree branch (├─ or └─) per listed symbol; no emoji in the CLI any more.
n="$(echo "$out" | grep -cE '^(├─|└─)')"
[ "$n" -ge 20 ] && ok "cli signatures Catalog (>=20 symbols, got $n)" \
  || bad "cli signatures Catalog" "expected >=20, got $n"

out="$("$BIN" fn "$WORK/java/Greeter.java" greet -c Greeter 2>&1)"
echo "$out" | grep -q 'name + " greets " + who' && ok "cli function Greeter.greet" \
  || bad "cli function Greeter.greet" "$out"

out="$("$BIN" bd "$WORK/java/Greeter.java" identity -c Greeter 2>&1)"
echo "$out" | grep -q 'return value' && ok "cli body Greeter.identity" \
  || bad "cli body Greeter.identity" "$out"

out="$("$BIN" st "$WORK/java/Shapes.java" Point 2>&1)"
echo "$out" | grep -q 'record Point(int x, int y)' && ok "cli struct Point (record)" \
  || bad "cli struct Point" "$out"

out="$("$BIN" iface "$WORK/java/Shapes.java" Shape 2>&1)"
echo "$out" | grep -q 'interface Shape' && ok "cli interface Shape" \
  || bad "cli interface Shape" "$out"

# disambiguation: greet exists in Greeter (1 param) and Town (0 params)
out="$("$BIN" fn "$WORK/java/Shapes.java" greet -c Town 2>&1)"
echo "$out" | grep -q 'welcome to town' && ok "cli disambiguation Town.greet" \
  || bad "cli disambiguation Town.greet" "$out"

# ----------------------------------------------------------------------------
# 3. Write reversibility — modify A, modify B, restore both, hash must match.
# ----------------------------------------------------------------------------
H0="$(hash_of "$F")"

if "$BIN" wr "$F" -k method -n greet -c Greeter -t "$GREET_MOD" >/dev/null 2>&1 \
   && [ "$(hash_of "$F")" != "$H0" ] \
   && "$BIN" sig "$F" >/dev/null 2>&1; then
  ok "write #1 (greet) applied and still parses"
else
  bad "write #1 (greet)" "edit not applied or file no longer parses"
fi

if "$BIN" wr "$F" -k method -n farewell -c Greeter -t "$FAREWELL_MOD" >/dev/null 2>&1 \
   && [ "$(hash_of "$F")" != "$H0" ]; then
  ok "write #2 (farewell) applied"
else
  bad "write #2 (farewell)" "edit not applied"
fi

# restore both in a single MCP batch (two edits, all-or-nothing)
restore_req="$(jq -nc --arg p "$F" \
  --arg t1 "$GREET_ORIG" --arg t2 "$FAREWELL_ORIG" \
  '{jsonrpc:"2.0",id:10,method:"tools/call",params:{name:"write_function",arguments:{path:$p,edits:[{kind:"method",name:"greet",container:"Greeter",new_text:$t1},{kind:"method",name:"farewell",container:"Greeter",new_text:$t2}]}}}')"
resp="$(printf '%s\n' "$restore_req" | mcp)"
if echo "$resp" | grep -q '"applied":true' && [ "$(hash_of "$F")" = "$H0" ]; then
  ok "write #3 batch restore (hash back to original)"
else
  bad "write #3 batch restore" "hash mismatch or batch not applied"
fi

# ----------------------------------------------------------------------------
# 4. All-or-nothing atomicity — one valid + one broken edit, file untouched.
# ----------------------------------------------------------------------------
H1="$(hash_of "$F")"
atomic_req="$(jq -nc --arg p "$F" \
  --arg t1 "$GREET_MOD" --arg t2 "$GREET_BROKEN" \
  '{jsonrpc:"2.0",id:20,method:"tools/call",params:{name:"write_function",arguments:{path:$p,edits:[{kind:"method",name:"greet",container:"Greeter",new_text:$t1},{kind:"method",name:"farewell",container:"Greeter",new_text:$t2}]}}}')"
# A rejected batch comes back as a JSON-RPC tool error ("isError":true) whose
# message names the offending edit; the file must be byte-for-byte unchanged.
resp="$(printf '%s\n' "$atomic_req" | mcp)"
if echo "$resp" | grep -q '"isError":true' \
   && echo "$resp" | grep -q 'invalid syntax' \
   && [ "$(hash_of "$F")" = "$H1" ]; then
  ok "atomicity: broken edit rejects whole batch, file intact"
else
  bad "atomicity" "batch not rejected or file changed"
fi

# ----------------------------------------------------------------------------
# 5. Single parse-rejection via CLI — broken body refused, file intact.
# ----------------------------------------------------------------------------
H2="$(hash_of "$F")"
if "$BIN" wr "$F" -k method -n greet -c Greeter -t "$GREET_BROKEN" >/dev/null 2>&1; then
  bad "parse-rejection" "broken write unexpectedly succeeded"
elif [ "$(hash_of "$F")" = "$H2" ]; then
  ok "parse-rejection: broken body refused, file intact"
else
  bad "parse-rejection" "file changed despite rejection"
fi

# ----------------------------------------------------------------------------
# 6. MCP stdio smoke — initialize + tools/list + tools/call list_signatures.
# ----------------------------------------------------------------------------
list_req='{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
resp="$(printf '%s\n' "$list_req" | mcp)"
if echo "$resp" | grep -q '"name":"list_signatures"' \
   && echo "$resp" | grep -q '"name":"write_function"'; then
  ok "mcp tools/list exposes the v1 tools"
else
  bad "mcp tools/list" "expected tools missing from response"
fi

call_req="$(jq -nc --arg p "$WORK/java/Greeter.java" \
  '{jsonrpc:"2.0",id:3,method:"tools/call",params:{name:"list_signatures",arguments:{path:$p}}}')"
resp="$(printf '%s\n' "$call_req" | mcp)"
if echo "$resp" | grep -q '"name":"greet"' && echo "$resp" | grep -q '"name":"identity"'; then
  ok "mcp tools/call list_signatures returns Greeter symbols"
else
  bad "mcp tools/call list_signatures" "expected symbols missing from response"
fi

# ----------------------------------------------------------------------------
echo
echo "Passed: $PASS/$TOTAL"
[ "$PASS" -eq "$TOTAL" ]
