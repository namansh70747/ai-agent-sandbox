#!/bin/bash
# =============================================================================
# FILE: scripts/02-deploy-k8s.sh
# SOURCE: https://urunc.io/tutorials/How-to-urunc-on-k8s/
# =============================================================================

set -e

echo "=== Deploying urunc on Kubernetes ==="

# Step 1: Apply urunc-deploy DaemonSet (installs binaries on nodes)
kubectl apply -f configs/k8s/urunc-deploy.yaml

# Step 2: Wait for nodes to be labeled urunc.io/urunc-runtime=true
echo "Waiting for urunc runtime label on nodes..."
sleep 30

# Step 3: Apply RuntimeClass
kubectl apply -f configs/k8s/urunc-runtimeClass.yaml

# Step 4: Deploy tool sandbox
kubectl apply -f configs/k8s/tool-sandbox-deployment.yaml

echo "=== Verify ==="
kubectl get runtimeclass
kubectl get pods -l app=ai-agent-web-tool
