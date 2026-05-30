#!/usr/bin/env bash
# scripts/03-build-servers.sh
#
# Build all three server binaries to bin/ inside the Lima shell.
#
# Run from the project root:
#   bash scripts/03-build-servers.sh
#
# Requires Go ≥ 1.22 at /usr/local/go/bin/go (installed by README Part 3.3).
# Must be run inside the Lima VM (limactl shell urunc-dev).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO="${GO_BIN:-/usr/local/go/bin/go}"
BIN="$ROOT/bin"

log() { echo "=== $* ==="; }

cd "$ROOT"

log "Fetching dependencies"
"$GO" mod tidy

mkdir -p "$BIN"

log "Building sandbox-manager (existing demo CLI)"
"$GO" build -o "$BIN/sandbox-manager" ./cmd/sandbox-manager

log "Building mcp-server (stdio MCP — for OpenCode, Cursor, Zed, Claude Code)"
"$GO" build -o "$BIN/mcp-server" ./cmd/mcp-server

log "Building api-server (HTTP REST + MCP over SSE — for any agent)"
"$GO" build -o "$BIN/api-server" ./cmd/api-server

echo ""
echo "Built binaries:"
ls -lh "$BIN/"
echo ""
echo "Next steps:"
echo ""
echo "  1. Verify tools list (no urunc needed):"
echo "     printf '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"clientInfo\":{\"name\":\"test\",\"version\":\"1\"}}}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\",\"params\":{}}\n' | $BIN/mcp-server 2>/dev/null"
echo ""
echo "  2. REST API profiles (no urunc needed):"
echo "     $BIN/api-server &"
echo "     curl -s http://localhost:8080/api/v1/tools | python3 -m json.tool"
echo ""
echo "  3. Live sandbox execution (requires urunc + sudoers step from README):"
echo "     $BIN/api-server &"
echo "     curl -s -X POST http://localhost:8080/api/v1/execute \\"
echo "       -H 'Content-Type: application/json' \\"
echo "       -d '{\"tool_type\":\"code_tool\",\"command\":[\"echo\",\"hello from microVM\"]}'"
echo ""
echo "  4. OpenCode integration:"
echo "     Update configs/mcp/opencode.json with the actual binary path:"
echo "       $(realpath "$BIN/mcp-server")"
echo "     Merge into ~/.config/opencode/config.json, restart OpenCode."
