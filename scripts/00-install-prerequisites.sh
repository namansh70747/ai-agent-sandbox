#!/bin/bash
# =============================================================================
# FILE: scripts/00-install-prerequisites.sh
# SOURCES:
#   - https://urunc.io/installation/
#   - https://urunc.io/quickstart/
#   - https://nubificus.co.uk/blog/urunc_agent/
# =============================================================================

set -euo pipefail

# -----------------------------------------------------------------------------
# PART A: Common container tools (from https://urunc.io/installation/)
# -----------------------------------------------------------------------------

echo "=== Installing containerd ==="
sudo apt-get update
sudo apt-get install -y ca-certificates curl gnupg lsb-release

# Install runc (latest release)
RUNC_VERSION=$(curl -L -s -o /dev/null -w '%{url_effective}' "https://github.com/opencontainers/runc/releases/latest" | grep -oP "v\d+\.\d+\.\d+" | sed 's/v//')
wget -q "https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.$(dpkg --print-architecture)"
sudo install -m 755 "runc.$(dpkg --print-architecture)" /usr/local/sbin/runc
rm -f "./runc.$(dpkg --print-architecture)"

# Install containerd
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
rm get-docker.sh
sudo groupadd docker || true
sudo usermod -aG docker "$USER"

# Install CNI plugins (required for urunc network handling with tap0_urunc)
CNI_VERSION=$(curl -L -s -o /dev/null -w '%{url_effective}' "https://github.com/containernetworking/plugins/releases/latest" | grep -oP "v\d+\.\d+\.\d+" | sed 's/v//')
wget -q "https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-linux-$(dpkg --print-architecture)${CNI_VERSION}.tgz"
sudo mkdir -p /opt/cni/bin
sudo tar Cxzvf /opt/cni/bin "cni-plugins-linux-$(dpkg --print-architecture)${CNI_VERSION}.tgz"
rm -f "cni-plugins-linux-$(dpkg --print-architecture)${CNI_VERSION}.tgz"

# Install nerdctl
NERDCTL_VERSION=$(curl -L -s -o /dev/null -w '%{url_effective}' "https://github.com/containerd/nerdctl/releases/latest" | grep -oP "v\d+\.\d+\.\d+" | sed 's/v//')
wget -q "https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-$(dpkg --print-architecture).tar.gz"
sudo tar Cxzvf /usr/local/bin "nerdctl-${NERDCTL_VERSION}-linux-$(dpkg --print-architecture).tar.gz"
rm -f "nerdctl-${NERDCTL_VERSION}-linux-$(dpkg --print-architecture).tar.gz"

# -----------------------------------------------------------------------------
# PART B: Block-based snapshotter (devmapper) - from https://urunc.io/installation/
# -----------------------------------------------------------------------------
echo "=== Setting up devmapper snapshotter ==="
sudo apt-get install -y bc thin-provisioning-tools

git clone https://github.com/urunc-dev/urunc.git /tmp/urunc-src || true
sudo mkdir -p /usr/local/bin/scripts
sudo mkdir -p /usr/local/lib/systemd/system/
sudo cp /tmp/urunc-src/script/dm_create.sh /usr/local/bin/scripts/dm_create.sh
sudo cp /tmp/urunc-src/script/dm_reload.sh /usr/local/bin/scripts/dm_reload.sh
sudo chmod 755 /usr/local/bin/scripts/dm_create.sh
sudo chmod 755 /usr/local/bin/scripts/dm_reload.sh

sudo /usr/local/bin/scripts/dm_create.sh

sudo cp /tmp/urunc-src/script/dm_reload.service /usr/local/lib/systemd/system/dm_reload.service
sudo chmod 644 /usr/local/lib/systemd/system/dm_reload.service
sudo chown root:root /usr/local/lib/systemd/system/dm_reload.service
sudo systemctl daemon-reload
sudo systemctl enable dm_reload.service

# Configure containerd v2.x for devmapper (from installation docs)
sudo mkdir -p /etc/containerd
sudo tee /etc/containerd/config.toml > /dev/null <<'EOF'
version = 2
[plugins."io.containerd.snapshotter.v1.devmapper"]
  pool_name = "containerd-pool"
  root_path = "/var/lib/containerd/io.containerd.snapshotter.v1.devmapper"
  base_image_size = "10GB"
  discard_blocks = true
  fs_type = "ext2"
EOF

sudo systemctl restart containerd

# -----------------------------------------------------------------------------
# PART C: Install monitors (QEMU, Firecracker, Solo5, virtiofsd)
# From: https://urunc.io/installation/ (Option 1: monitors-build repository)
# -----------------------------------------------------------------------------
echo "=== Installing VM and sandbox monitors ==="
MONITOR_RELEASE="FC-v1.7.0_S5-v0.9.3_VFS_-v1.13.0_QM-v10.1.1-9a44e"
wget -q "https://github.com/urunc-dev/monitors-build/releases/download/${MONITOR_RELEASE}/release-amd64-${MONITOR_RELEASE}.tar.gz"
sudo tar Cxzvf /opt "release-amd64-${MONITOR_RELEASE}.tar.gz"
rm -f "release-amd64-${MONITOR_RELEASE}.tar.gz"

# -----------------------------------------------------------------------------
# PART D: Install urunc and shim (from https://urunc.io/installation/)
# -----------------------------------------------------------------------------
echo "=== Installing urunc ==="
URUNC_VERSION=$(curl -L -s -o /dev/null -w '%{url_effective}' "https://github.com/urunc-dev/urunc/releases/latest" | grep -oP "v\d+\.\d+\.\d+" | sed 's/v//')

URUNC_BINARY_FILENAME="urunc_static_$(dpkg --print-architecture)"
wget -q "https://github.com/urunc-dev/urunc/releases/download/v${URUNC_VERSION}/${URUNC_BINARY_FILENAME}"
chmod +x "${URUNC_BINARY_FILENAME}"
sudo mv "${URUNC_BINARY_FILENAME}" /usr/local/bin/urunc

CONTAINERD_BINARY_FILENAME="containerd-shim-urunc-v2_static_$(dpkg --print-architecture)"
wget -q "https://github.com/urunc-dev/urunc/releases/download/v${URUNC_VERSION}/${CONTAINERD_BINARY_FILENAME}"
chmod +x "${CONTAINERD_BINARY_FILENAME}"
sudo mv "${CONTAINERD_BINARY_FILENAME}" /usr/local/bin/containerd-shim-urunc-v2

# -----------------------------------------------------------------------------
# PART E: urunc configuration file (from https://urunc.io/installation/)
# -----------------------------------------------------------------------------
echo "=== Configuring urunc ==="
sudo mkdir -p /etc/urunc
sudo tee /etc/urunc/config.toml > /dev/null <<'EOF'
[monitors.qemu]
path = "/opt/urunc/bin/qemu-system-x86_64"
data_path = "/opt/urunc/share/qemu"

[monitors.firecracker]
path = "/opt/urunc/bin/firecracker"

[monitors.hvt]
path = "/opt/urunc/bin/solo5-hvt"

[monitors.spt]
path = "/opt/urunc/bin/solo5-spt"

[extra_binaries.virtiofsd]
path = "/opt/urunc/bin/virtiofsd"
EOF

# -----------------------------------------------------------------------------
# PART F: Add urunc to containerd runtimes (from https://urunc.io/installation/)
# -----------------------------------------------------------------------------
sudo tee -a /etc/containerd/config.toml > /dev/null <<'EOF'
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.urunc]
  runtime_type = "io.containerd.urunc.v2"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.urunc.options]
  BinaryName = "/usr/local/bin/urunc"
EOF

sudo systemctl restart containerd

echo "=== Installation complete. Log out and back in for docker group changes. ==="
