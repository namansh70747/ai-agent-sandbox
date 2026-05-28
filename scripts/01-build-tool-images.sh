#!/bin/bash
# =============================================================================
# FILE: scripts/01-build-tool-images.sh
# SOURCES:
#   - https://urunc.io/package/
#   - https://urunc.io/quickstart/
# =============================================================================

set -e

echo "=== Building AI Agent Tool Images with bunny ==="

# Build base tool image using bunny Containerfile syntax
# From docs: docker build -f Containerfile -t go-dev-opencode:containerfile .
docker build -f build/Containerfile -t localhost/ai-sandbox/base-tool:latest build/

# Build specific tool images (tagging only; in production use bunnyfile for kernel and urunit)
docker tag localhost/ai-sandbox/base-tool:latest localhost/ai-sandbox/file-tool:latest
docker tag localhost/ai-sandbox/base-tool:latest localhost/ai-sandbox/code-tool:latest
docker tag localhost/ai-sandbox/base-tool:latest localhost/ai-sandbox/web-tool:latest
docker tag localhost/ai-sandbox/base-tool:latest localhost/ai-sandbox/db-tool:latest

echo "=== Images ready ==="
docker images | grep ai-sandbox

echo ""
echo "=== Running Basic Test (from https://urunc.io/quickstart/) ==="
# Quickstart test: run nginx unikernel to verify urunc installation
docker run --rm -d --runtime io.containerd.urunc.v2 harbor.nbfc.io/nubificus/urunc/nginx-qemu-unikraft-initrd:latest

echo ""
echo "=== Running AI Sandbox Manager ==="
go run ./cmd/sandbox-manager/main.go
