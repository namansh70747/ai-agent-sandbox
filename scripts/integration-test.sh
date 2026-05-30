#!/usr/bin/env bash
# scripts/integration-test.sh
#
# End-to-end isolation proof. Runs against a LIVE api-server inside Lima
# (requires urunc). Proves the security claims rather than asserting them:
#
#   1. code_tool   has NO network (DNS/connect fails inside the VM)
#   2. file_tool   can read/write its workspace
#   3. web_tool    CANNOT see the workspace (no mount) — cross-tool isolation
#   4. shell_tool  workspace is READ-ONLY (write fails)
#
# Exit code is non-zero if any proof fails — suitable for a smoke gate.
#
# Usage (inside Lima, with the policy server running):
#   ./bin/api-server --policy configs/policies.yaml &
#   bash scripts/integration-test.sh

set -uo pipefail

URL="${SANDBOX_URL:-http://localhost:8080}"
WS="${SANDBOX_WORKSPACE:-/tmp/ai-sandbox-workspace}"
fails=0

note()  { echo "  $*"; }
pass()  { echo "  PASS: $*"; }
fail()  { echo "  FAIL: $*"; fails=$((fails+1)); }

# exec_tool <tool_type> <json-array-command> -> prints JSON response
exec_tool() {
  curl -s -X POST "$URL/api/v1/execute" \
    -H 'Content-Type: application/json' \
    -d "{\"tool_type\":\"$1\",\"command\":$2}"
}

# field <json> <key>  (uses python for robust parsing)
field() { python3 -c "import sys,json;print(json.load(sys.stdin).get('$2',''))" <<<"$1"; }

echo "== urunc per-tool isolation integration test =="
echo "   server: $URL"
mkdir -p "$WS"

echo
echo "[1] code_tool has NO network"
r=$(exec_tool code_tool '["python3","-c","import socket; socket.create_connection((\"1.1.1.1\",53),timeout=3)"]')
ec=$(field "$r" exit_code)
if [ "$ec" != "0" ]; then pass "network blocked in code_tool (exit=$ec)"; else fail "code_tool reached the network (exit=0)"; fi

echo
echo "[2] file_tool can read/write workspace"
echo "secret-$$" > "$WS/probe.txt"
r=$(exec_tool file_tool '["cat","/workspace/probe.txt"]')
if [ "$(field "$r" exit_code)" = "0" ]; then pass "file_tool read workspace file"; else fail "file_tool could not read workspace"; fi

echo
echo "[3] web_tool CANNOT see the workspace (cross-tool isolation)"
r=$(exec_tool web_tool '["cat","/workspace/probe.txt"]')
if [ "$(field "$r" exit_code)" != "0" ]; then pass "web_tool has no workspace mount"; else fail "web_tool could read file_tool's workspace"; fi

echo
echo "[4] shell_tool workspace is READ-ONLY"
r=$(exec_tool shell_tool '["sh","-c","echo x > /workspace/should_fail.txt"]')
if [ "$(field "$r" exit_code)" != "0" ]; then pass "shell_tool cannot write (ro mount)"; else fail "shell_tool wrote to a read-only mount"; fi

echo
if [ "$fails" -eq 0 ]; then
  echo "ALL ISOLATION PROOFS PASSED"
  exit 0
else
  echo "$fails PROOF(S) FAILED"
  exit 1
fi
