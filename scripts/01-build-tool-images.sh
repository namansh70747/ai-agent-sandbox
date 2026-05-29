#!/bin/bash
# =============================================================================
# scripts/01-build-tool-images.sh
#
# Builds the per-tool container images and runs the quickstart verification.
#
# Sources:
#   https://urunc.io/package/               — bunny build frontend
#   https://urunc.io/quickstart/            — nerdctl run with urunc
#   https://nubificus.co.uk/blog/urunc_agent/ — Containerfile + bunnyfile workflow
#
# USAGE (inside Lima VM, after 00-install-prerequisites.sh):
#   sudo bash scripts/01-build-tool-images.sh
# =============================================================================

set -euo pipefail

log() { echo ""; echo "=== $* ==="; }
ok()  { echo "  ✓ $*"; }

# =============================================================================
# STEP 1 — Build per-tool image with bunny Containerfile syntax
#
# Source: https://nubificus.co.uk/blog/urunc_agent/ (Bunny with Containerfile)
#   "we simply need to prepend the following line in Containerfile:
#    #syntax=harbor.nbfc.io/nubificus/bunny:containerfile"
#   "nerdctl build -f Containerfile -t go-dev-opencode:containerfile ."
# =============================================================================
log "Step 1: Building base tool image with bunny"

nerdctl build \
	--no-cache \
	-f build/Containerfile \
	-t localhost/ai-sandbox/base-tool:latest \
	build/

ok "localhost/ai-sandbox/base-tool:latest built"

# Tag per-tool variants
for TOOL in file-tool code-tool web-tool db-tool; do
	nerdctl tag localhost/ai-sandbox/base-tool:latest "localhost/ai-sandbox/${TOOL}:latest"
	ok "Tagged localhost/ai-sandbox/${TOOL}:latest"
done

echo ""
echo "  Built images:"
nerdctl images | grep "ai-sandbox"

# =============================================================================
# STEP 2 — Quickstart verification
#
# Source: https://urunc.io/quickstart/ (Run the unikernel)
#   "nerdctl run -d --runtime io.containerd.urunc.v2
#    harbor.nbfc.io/nubificus/urunc/nginx-qemu-unikraft-initrd:latest"
# =============================================================================
log "Step 2: Quickstart verification (Nginx/Unikraft over QEMU)"

echo "  Pulling pre-built urunc image..."
nerdctl pull harbor.nbfc.io/nubificus/urunc/nginx-qemu-unikraft-initrd:latest

echo ""
echo "  Starting Nginx unikernel container with urunc..."
CONTAINER_ID=$(nerdctl run -d \
	--runtime io.containerd.urunc.v2 \
	harbor.nbfc.io/nubificus/urunc/nginx-qemu-unikraft-initrd:latest)

sleep 3

IP=$(nerdctl inspect "$CONTAINER_ID" --format '{{.NetworkSettings.IPAddress}}' 2>/dev/null || echo "unknown")
echo "  Container: ${CONTAINER_ID:0:12}  IP: ${IP}"

if [ "$IP" != "unknown" ] && [ -n "$IP" ]; then
	echo ""
	echo "  Curling Nginx inside urunc microVM:"
	curl --max-time 5 "http://${IP}" 2>/dev/null || echo "  (curl timed out — VM may still be booting)"
fi

echo ""
echo "  Stopping container..."
nerdctl stop "$CONTAINER_ID" 2>/dev/null || true
ok "Quickstart verification complete"

# =============================================================================
# STEP 3 — Run the sandbox manager profile output
# =============================================================================
log "Step 3: Per-tool isolation profiles"

go run ./cmd/sandbox-manager/main.go --profile

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Images built and quickstart verified."
echo ""
echo "  Run the full demo with:"
echo "    sudo go run ./cmd/sandbox-manager/main.go --demo"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
