# Complete Guide: Building Per-Tool Sandboxing for AI Agents with urunc

This repository is a docs-first, end-to-end implementation of per-tool sandboxing using urunc. Every script and source file includes doc references that match the official urunc documentation and the Nubificus blog.

## Part 1: What You Must Read (Documentation)

Required reading (in order):

1. urunc Official Design Documentation
   URL: https://urunc.io/design/
   What to learn:
   - How urunc works (execution flow from containerd to urunc to VMM)
   - OCI annotations urunc uses (com.urunc.unikernel.*)
   - Network handling (tap0_urunc and CNI integration)
   - Storage handling (block devices, devmapper)
   - Lifecycle commands (create, start, delete, kill)
   Key sections to read:
   - "Execution flow"
   - "OCI artifacts"
   - "Container (Unikernel) spawning"
   - "Network handling"
   - "k8s integration"

2. Nubificus AI Agent Blog (The Original)
   URL: https://nubificus.co.uk/blog/urunc_agent/
   What to learn:
   - How they ran opencode inside urunc
   - The exact docker command they used
   - The limitation they identified: shared data is no longer protected
   Key command:
   ```bash
   sudo docker run \
     --runtime=io.containerd.urunc.v2 \
     -v ${PWD}/mydir:/mydir \
     opencode:latest
   ```

3. urunc GitHub Repository
   URL: https://github.com/urunc-dev/urunc
   What to explore:
   - cmd/urunc/ (CLI commands: create, start, delete, kill)
   - pkg/containerd-shim/ (containerd shim integration)
   - pkg/unikontainers/ (unikernel management code)
   - pkg/unikontainers/hypervisors/ (QEMU, Firecracker, Cloud Hypervisor)
   - pkg/network/ (network_dynamic.go, network_static.go)
   - deployment/urunc-deploy/ (Kubernetes deployment files)

4. urunc Quickstart and Installation
   URL: https://urunc.io/quickstart/ and https://urunc.io/installation/
   What to learn:
   - How to install urunc
   - Required dependencies (containerd, CNI plugins)
   - How to run a first container with urunc

Additional references:
- https://blog.cloudkernels.net/posts/urunc/
- https://urunc.io/hypervisor-support/

## Part 2: What You Need to Build (Complete System)

System architecture:

```text
┌──────────────────────────────────────────────────────────────────┐
│                    AI Agent Sandbox Manager                      │
│                    (Your Implementation)                         │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  1. Tool Registry (Define tools and permissions)          │  │
│  │     - Tool: file_tool                                     │  │
│  │       Permissions: read, write (workspace only)           │  │
│  │       NO: execute, network, database                      │  │
│  │     - Tool: code_tool                                     │  │
│  │       Permissions: execute shell commands                 │  │
│  │       NO: filesystem, network                             │  │
│  │     - Tool: web_tool                                      │  │
│  │       Permissions: HTTP requests to specific domains      │  │
│  │       NO: filesystem, database, execute                   │  │
│  │     - Tool: database_tool                                 │  │
│  │       Permissions: database connection                    │  │
│  │       NO: filesystem, network, execute                    │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  2. Sandbox Spawner (Spawn urunc containers per tool)      │  │
│  │     - Parse tool permissions                               │  │
│  │     - Create urunc container with specific config          │  │
│  │     - Mount only allowed resources                         │  │
│  │     - Set network rules (allow or deny)                    │  │
│  │     - Start sandbox                                        │  │
│  │     - Execute tool in sandbox                              │  │
│  │     - Destroy sandbox after execution                      │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  3. Permissions Enforcer (Enforce sandboxes)               │  │
│  │     - File system: mount only workspace directory          │  │
│  │     - Network: use CNI to restrict to specific domains     │  │
│  │     - Execute: use seccomp to block specific syscalls      │  │
│  │     - Database: use network rules to allow only DB port    │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  4. MCP Server Integration (Optional)                      │  │
│  │     - Map MCP tools to urunc sandboxes                     │  │
│  │     - Return tool results to agent                         │  │
│  │     - Handle errors gracefully                             │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

## Project Directory Structure

```text
ai-agent-sandbox/
├── README.md
├── go.mod
├── scripts/
│   ├── 00-install-prerequisites.sh
│   ├── 01-build-tool-images.sh
│   └── 02-deploy-k8s.sh
├── configs/
│   ├── containerd/
│   │   └── config.toml
│   ├── urunc/
│   │   └── config.toml
│   ├── cni/
│   │   └── 10-urunc-bridge.conf
│   └── k8s/
│       ├── urunc-runtimeClass.yaml
│       ├── tool-sandbox-deployment.yaml
│       └── urunc-deploy.yaml
├── build/
│   ├── Containerfile
│   ├── bunnyfile
│   └── urunc.json.example
├── pkg/
│   ├── tool/
│   │   └── registry.go
│   ├── oci/
│   │   ├── annotations.go
│   │   ├── bundle.go
│   │   └── urunc_json.go
│   ├── sandbox/
│   │   ├── manager.go
│   │   ├── basic_spawner.go
│   │   ├── lifecycle.go
│   │   ├── network.go
│   │   └── storage.go
│   └── mcp/
│       └── server.go
└── cmd/
    └── sandbox-manager/
        └── main.go
```

## macOS (Lima) Commands

Source: https://lima-vm.io/

Run the Linux-only install and runtime steps inside a Linux VM. The scripts use apt-get and systemd and will not run on macOS directly.

```bash
# Install Lima
brew install lima

# Start an Ubuntu VM (example)
limactl start --name urunc template://ubuntu-22.04

# Enter the VM
limactl shell urunc

# Stop and delete the VM
limactl stop urunc
limactl delete urunc
```

Inside the VM, use the host-mounted repo path (default Lima templates mount your home directory). If needed, clone the repo inside the VM.

## Build and Run (Linux VM)

```bash
# 1. Install prerequisites
sudo bash scripts/00-install-prerequisites.sh

# 2. Build images
sudo bash scripts/01-build-tool-images.sh

# 3. Run the manager
sudo go run ./cmd/sandbox-manager/main.go
```

## Kubernetes Deployment (Optional)

```bash
sudo bash scripts/02-deploy-k8s.sh
```

## Doc Rules Checklist

| Doc Rule | Implementation Location |
|---|---|
| com.urunc.unikernel.unikernelType required | pkg/oci/annotations.go |
| com.urunc.unikernel.hypervisor required | pkg/oci/annotations.go |
| com.urunc.unikernel.binary required | pkg/oci/annotations.go |
| com.urunc.unikernel.cmdline required | pkg/oci/annotations.go |
| Optional: initrd, block, blkMntPoint, mountRootfs | pkg/oci/annotations.go |
| urunc.json in rootfs with base64 values | pkg/oci/urunc_json.go |
| Execution flow (stdio, hooks, VMM boot) | pkg/sandbox/manager.go, pkg/sandbox/lifecycle.go |
| Network: tap0_urunc plus CNI veth mapping | pkg/sandbox/network.go and configs/cni/10-urunc-bridge.conf |
| Storage: devmapper, block, initrd, 9pfs, virtiofs | pkg/sandbox/storage.go and scripts/00-install-prerequisites.sh |
| Lifecycle: create, start, delete, kill, state | pkg/sandbox/lifecycle.go |
| Docker runtime: --runtime io.containerd.urunc.v2 | pkg/sandbox/basic_spawner.go |
| Blog warning about shared data | pkg/sandbox/basic_spawner.go and pkg/sandbox/storage.go |
| k8s RuntimeClass urunc | configs/k8s/urunc-runtimeClass.yaml |
| k8s urunc-deploy DaemonSet | configs/k8s/urunc-deploy.yaml |
| Bunny syntax directive required | build/Containerfile and build/bunnyfile |
| urunc config.toml with monitor paths | configs/urunc/config.toml |

## Part 5: Complete Learning Path

Week 1: Foundations
- Read urunc design docs (complete)
- Read Nubificus AI agent blog (complete)
- Install urunc on your system
- Run hello world: sudo nerdctl run --runtime io.containerd.urunc.v2 ...

Week 2: Code Understanding
- Browse urunc GitHub repo
- Read cmd/urunc/create.go (how urunc creates containers)
- Read pkg/unikontainers/unikontainers.go (core logic)
- Read pkg/network/network_dynamic.go (networking)

Week 3: Build Your System
- Write tool registry
- Write sandbox spawner
- Write main demo
- Test with a simple Python script

Week 4: Integration
- Add MCP server support
- Add proper error handling
- Add logging and monitoring
- Create documentation
```

## Notes

- All scripts in scripts/ are Linux-only and should be executed inside a Linux VM or Linux host.
- The install script configures containerd, devmapper, CNI, urunc, and monitor binaries exactly as referenced in the official docs.
