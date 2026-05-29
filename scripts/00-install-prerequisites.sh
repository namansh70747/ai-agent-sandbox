#!/bin/bash
# =============================================================================
# scripts/00-install-prerequisites.sh
#
# Installs every dependency needed to run urunc per-tool sandboxes.
# All commands are sourced verbatim from official urunc documentation.
#
# SOURCES (in order of dependency):
#   https://urunc.io/installation/   — primary reference
#   https://urunc.io/quickstart/     — verification
#   https://nubificus.co.uk/blog/urunc_agent/ — nerdctl/urunc workflow
#
# USAGE (run inside Lima VM or Ubuntu 22.04 host):
#   sudo bash scripts/00-install-prerequisites.sh
#
# WHAT THIS SCRIPT INSTALLS:
#   Part A — containerd (via Docker Engine install)
#   Part B — runc (low-level runtime for normal containers in k8s)
#   Part C — CNI plugins (container networking)
#   Part D — nerdctl (containerd CLI)
#   Part E — devmapper thinpool snapshotter
#   Part F — VM/Sandbox monitors (QEMU, Firecracker, Solo5, virtiofsd)
#   Part G — urunc + containerd-shim-urunc-v2
#   Part H — urunc configuration file
#   Part I — containerd runtime registration
# =============================================================================

set -euo pipefail

# ─── helpers ─────────────────────────────────────────────────────────────────
log() { echo ""; echo "=== $* ==="; }
ok()  { echo "  ✓ $*"; }

# =============================================================================
# PART A — containerd (installed via Docker Engine)
# Source: https://urunc.io/quickstart/ (Install Docker)
#   "The easiest and fastest way to try out urunc would be with nerdctl"
# =============================================================================
log "Part A: Installing containerd (via Docker Engine)"

sudo apt-get update -y
sudo apt-get install -y ca-certificates curl gnupg lsb-release

curl -fsSL https://get.docker.com -o /tmp/get-docker.sh
sudo sh /tmp/get-docker.sh
rm /tmp/get-docker.sh

sudo groupadd docker 2>/dev/null || true
sudo usermod -aG docker "$USER" || true
ok "Containerd installed (via Docker Engine)"

# =============================================================================
# PART B — runc (required for normal containers alongside urunc in k8s)
# Source: https://urunc.io/installation/ (Install runc)
#   "urunc delegates the management of normal containers to a typical
#    low-level container runtime like runc"
# =============================================================================
log "Part B: Installing runc"

RUNC_VERSION=$(curl -L -s -o /dev/null -w '%{url_effective}' \
  "https://github.com/opencontainers/runc/releases/latest" \
  | grep -oP "v\d+\.\d+\.\d+" | sed 's/v//')

wget -q "https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.$(dpkg --print-architecture)"
sudo install -m 755 "runc.$(dpkg --print-architecture)" /usr/local/sbin/runc
rm -f "./runc.$(dpkg --print-architecture)"
ok "runc ${RUNC_VERSION} installed at /usr/local/sbin/runc"

# =============================================================================
# PART C — CNI plugins
# Source: https://urunc.io/installation/ (Install CNI plugins)
#   "For container networking the CNI plugins are necessary."
# urunc network design: https://urunc.io/design/ (Network handling)
#   "urunc creates a tap device tap0_urunc and connects it via CNI veth"
# =============================================================================
log "Part C: Installing CNI plugins"

CNI_VERSION=$(curl -L -s -o /dev/null -w '%{url_effective}' \
  "https://github.com/containernetworking/plugins/releases/latest" \
  | grep -oP "v\d+\.\d+\.\d+" | sed 's/v//')

CNI_ARCH=$(dpkg --print-architecture)
wget -q "https://github.com/containernetworking/plugins/releases/download/v${CNI_VERSION}/cni-plugins-linux-${CNI_ARCH}-v${CNI_VERSION}.tgz"
sudo mkdir -p /opt/cni/bin
sudo tar Cxzvf /opt/cni/bin "cni-plugins-linux-${CNI_ARCH}-v${CNI_VERSION}.tgz" > /dev/null
rm -f "cni-plugins-linux-${CNI_ARCH}-v${CNI_VERSION}.tgz"
ok "CNI plugins ${CNI_VERSION} installed at /opt/cni/bin"

# Copy per-tool CNI bridge config
sudo mkdir -p /etc/cni/net.d
sudo cp configs/cni/10-urunc-bridge.conf /etc/cni/net.d/10-urunc-bridge.conf 2>/dev/null || true
ok "CNI bridge config installed"

# =============================================================================
# PART D — nerdctl
# Source: https://urunc.io/installation/ (Install nerdctl)
#   "nerdctl offers a containerd CLI experience"
# =============================================================================
log "Part D: Installing nerdctl"

NERDCTL_VERSION=$(curl -L -s -o /dev/null -w '%{url_effective}' \
  "https://github.com/containerd/nerdctl/releases/latest" \
  | grep -oP "v\d+\.\d+\.\d+" | sed 's/v//')

NERDCTL_ARCH=$(dpkg --print-architecture)
wget -q "https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-${NERDCTL_ARCH}.tar.gz"
sudo tar Cxzvf /usr/local/bin "nerdctl-${NERDCTL_VERSION}-linux-${NERDCTL_ARCH}.tar.gz" > /dev/null
rm -f "nerdctl-${NERDCTL_VERSION}-linux-${NERDCTL_ARCH}.tar.gz"
ok "nerdctl ${NERDCTL_VERSION} installed"

# =============================================================================
# PART E — Devmapper thinpool snapshotter
# Source: https://urunc.io/installation/ (Setting and configuring devmapper)
#   "urunc can leverage block-based snapshots to treat a container's snapshot
#    as a block device for a guest."
#   "The first dm_create.sh creates a thinpool"
# =============================================================================
log "Part E: Setting up devmapper thinpool"

sudo apt-get install -y bc thin-provisioning-tools dmsetup

# Clone urunc to get the dm helper scripts
git clone --depth 1 https://github.com/urunc-dev/urunc.git /tmp/urunc-src 2>/dev/null \
  || (cd /tmp/urunc-src && git pull)

sudo mkdir -p /usr/local/bin/scripts
sudo mkdir -p /usr/local/lib/systemd/system

sudo cp /tmp/urunc-src/script/dm_create.sh  /usr/local/bin/scripts/dm_create.sh
sudo cp /tmp/urunc-src/script/dm_reload.sh  /usr/local/bin/scripts/dm_reload.sh
sudo chmod 755 /usr/local/bin/scripts/dm_create.sh
sudo chmod 755 /usr/local/bin/scripts/dm_reload.sh

# Create the thinpool
sudo /usr/local/bin/scripts/dm_create.sh
ok "devmapper thinpool created"

# Set up systemd service for reload-on-reboot
# Source: https://urunc.io/installation/
#   "on systemd-based systems, a service can automatically reload the thinpool"
sudo cp /tmp/urunc-src/script/dm_reload.service /usr/local/lib/systemd/system/dm_reload.service
sudo chmod 644 /usr/local/lib/systemd/system/dm_reload.service
sudo chown root:root /usr/local/lib/systemd/system/dm_reload.service
sudo systemctl daemon-reload
sudo systemctl enable dm_reload.service
ok "devmapper reload service enabled"

# Configure containerd for devmapper snapshotter
# Source: https://urunc.io/installation/ (containerd v2.x)
#   [plugins.'io.containerd.snapshotter.v1.devmapper']
sudo mkdir -p /etc/containerd
sudo tee /etc/containerd/config.toml > /dev/null << 'EOF'
version = 2

[plugins.'io.containerd.snapshotter.v1.devmapper']
  pool_name       = "containerd-pool"
  root_path       = "/var/lib/containerd/io.containerd.snapshotter.v1.devmapper"
  base_image_size = "10GB"
  discard_blocks  = true
  fs_type         = "ext2"
EOF

sudo systemctl restart containerd

# Verify devmapper
# Source: https://urunc.io/installation/
#   "sudo ctr plugin ls | grep devmapper"  → should show "ok"
DEVMAPPER_STATUS=$(sudo ctr plugin ls 2>/dev/null | grep devmapper | awk '{print $NF}' || echo "unknown")
ok "devmapper snapshotter status: ${DEVMAPPER_STATUS}"

# =============================================================================
# PART F — VM and sandbox monitors (QEMU, Firecracker, Solo5, virtiofsd)
# Source: https://urunc.io/installation/ (Option 1: monitors-build repository)
#   "The monitor-builds repository provides a reference setup for building
#    and distributing static binaries of monitors and tools for urunc."
#
# Release used:
#   Firecracker v1.7.0  (note: "Unikraft has booting issues in newer versions")
#   Solo5 v0.9.3
#   virtiofsd v1.13.0
#   QEMU v10.1.1
# =============================================================================
log "Part F: Installing VM/Sandbox monitors"

MONITOR_RELEASE="FC-v1.7.0_S5-v0.9.3_VFS_-v1.13.0_QM-v10.1.1-9a44e"
MONITOR_ARCH="amd64"

wget "https://github.com/urunc-dev/monitors-build/releases/download/${MONITOR_RELEASE}/release-${MONITOR_ARCH}-${MONITOR_RELEASE}.tar.gz"
sudo tar Cxzvf /opt "release-${MONITOR_ARCH}-${MONITOR_RELEASE}.tar.gz" > /dev/null
rm -f "release-${MONITOR_ARCH}-${MONITOR_RELEASE}.tar.gz"
ok "Monitors installed at /opt/urunc/"

# Verify key binaries
for BIN in qemu-system-x86_64 firecracker solo5-hvt solo5-spt virtiofsd; do
  if [ -f "/opt/urunc/bin/${BIN}" ]; then
    ok "  /opt/urunc/bin/${BIN}"
  else
    echo "  ✗ MISSING: /opt/urunc/bin/${BIN}"
  fi
done

# =============================================================================
# PART G — urunc binary + containerd-shim-urunc-v2
# Source: https://urunc.io/installation/ (Option 2: Install latest release)
#   "URUNC_BINARY_FILENAME="urunc_static_$(dpkg --print-architecture)""
# =============================================================================
log "Part G: Installing urunc"

URUNC_VERSION=$(curl -L -s -o /dev/null -w '%{url_effective}' \
  "https://github.com/urunc-dev/urunc/releases/latest" \
  | grep -oP "v\d+\.\d+\.\d+" | sed 's/v//')

URUNC_ARCH=$(dpkg --print-architecture)

# Install urunc binary
URUNC_BINARY_FILENAME="urunc_static_${URUNC_ARCH}"
wget -q "https://github.com/urunc-dev/urunc/releases/download/v${URUNC_VERSION}/${URUNC_BINARY_FILENAME}"
chmod +x "${URUNC_BINARY_FILENAME}"
sudo mv "${URUNC_BINARY_FILENAME}" /usr/local/bin/urunc
ok "urunc ${URUNC_VERSION} → /usr/local/bin/urunc"

# Install containerd shim
SHIM_BINARY_FILENAME="containerd-shim-urunc-v2_static_${URUNC_ARCH}"
wget -q "https://github.com/urunc-dev/urunc/releases/download/v${URUNC_VERSION}/${SHIM_BINARY_FILENAME}"
chmod +x "${SHIM_BINARY_FILENAME}"
sudo mv "${SHIM_BINARY_FILENAME}" /usr/local/bin/containerd-shim-urunc-v2
ok "containerd-shim-urunc-v2 → /usr/local/bin/containerd-shim-urunc-v2"

# =============================================================================
# PART H — urunc configuration file
# Source: https://urunc.io/configuration/
#   "urunc looks for its configuration file at /etc/urunc/config.toml"
#   Monitor paths match the monitors-build release installed in Part F.
# =============================================================================
log "Part H: Writing urunc config"

sudo mkdir -p /etc/urunc
sudo tee /etc/urunc/config.toml > /dev/null << 'EOF'
[log]
level  = "info"
syslog = false

[timestamps]
enabled     = false
destination = "/var/log/urunc/timestamps.log"

[monitors.qemu]
default_memory_mb = 512
default_vcpus     = 2
path              = "/opt/urunc/bin/qemu-system-x86_64"
data_path         = "/opt/urunc/share/qemu"

[monitors.firecracker]
default_memory_mb = 256
default_vcpus     = 1
path              = "/opt/urunc/bin/firecracker"

[monitors.hvt]
default_memory_mb = 256
default_vcpus     = 1
path              = "/opt/urunc/bin/solo5-hvt"

[monitors.spt]
default_memory_mb = 256
default_vcpus     = 1
path              = "/opt/urunc/bin/solo5-spt"

[extra_binaries.virtiofsd]
path    = "/opt/urunc/bin/virtiofsd"
options = "--cache always --sandbox none"
EOF
ok "urunc config written to /etc/urunc/config.toml"

# =============================================================================
# PART I — Register urunc as a containerd runtime (used by nerdctl)
# Source: https://urunc.io/installation/ (Add urunc runtime to containerd)
#   containerd v2.x format:
#   [plugins.'io.containerd.cri.v1.runtime'.containerd.runtimes.urunc]
#     runtime_type = "io.containerd.urunc.v2"
# Also: https://nubificus.co.uk/blog/urunc_agent/
#   "nerdctl run --runtime io.containerd.urunc.v2"
# =============================================================================
log "Part I: Registering urunc runtime with containerd"

sudo tee -a /etc/containerd/config.toml > /dev/null << 'EOF'

[plugins.'io.containerd.cri.v1.runtime'.containerd.runtimes.urunc]
  runtime_type          = "io.containerd.urunc.v2"
  container_annotations = ["com.urunc.unikernel.*"]
  pod_annotations       = ["com.urunc.unikernel.*"]
  snapshotter           = "devmapper"
EOF

sudo systemctl restart containerd
ok "urunc runtime registered for nerdctl"

# =============================================================================
# VERIFICATION
# Source: https://urunc.io/quickstart/ (Run the unikernel)
#   "nerdctl run -d --runtime io.containerd.urunc.v2
#    harbor.nbfc.io/nubificus/urunc/nginx-qemu-unikraft-initrd:latest"
# =============================================================================
log "Verification"

echo ""
echo "  Checking installed binaries:"
for BIN in nerdctl urunc containerd-shim-urunc-v2; do
  LOC=$(which "$BIN" 2>/dev/null || echo "NOT FOUND")
  echo "    $BIN → $LOC"
done

echo ""
echo "  urunc version: $(urunc --version 2>&1 || echo 'run urunc --version manually')"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Installation complete!"
echo ""
echo "  IMPORTANT: Log out and back in so group membership changes take effect."
echo ""
echo "  Next steps:"
echo "    1. Verify: go run ./cmd/sandbox-manager/main.go --verify"
echo "    2. Profile: go run ./cmd/sandbox-manager/main.go --profile"
echo "    3. Demo:    go run ./cmd/sandbox-manager/main.go --demo"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
