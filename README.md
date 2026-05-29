# Per-Tool AI Agent Sandboxing with urunc

>
> **One tool call → one isolated microVM → destroyed after return.**
>

The [Nubificus blog](https://nubificus.co.uk/blog/urunc_agent/) shows running an entire AI agent inside one [urunc](https://urunc.io/) microVM.

This project takes it further: **each tool the agent calls gets its own microVM**, with a permission profile matched exactly to what that tool needs.

Tool
Network
Filesystem mount
Why

`file_tool`
❌ none
✅ workspace only
File I/O needs no network

`code_tool`
❌ none
❌ none
Strongest isolation — untrusted code

`web_tool`
✅ bridge
❌ none
HTTP only, no data exfiltration path

`database_tool`
✅ bridge
❌ none
DB port only, no filesystem

A compromised `web_tool` cannot read the workspace. A compromised `code_tool` cannot reach the database. Each VM is created, used, and destroyed in a single `nerdctl run --rm` call.

---

## Architecture

```
AI Agent
    │
    ├─ calls file_tool  ──▶  urunc microVM A  (--network=none, -v /workspace:/workspace)
    ├─ calls code_tool  ──▶  urunc microVM B  (--network=none, no mounts)
    ├─ calls web_tool   ──▶  urunc microVM C  (--network=bridge, no mounts)
    └─ calls db_tool    ──▶  urunc microVM D  (--network=bridge, no mounts)

All VMs use runtime: io.containerd.urunc.v2
All VMs are destroyed on return (nerdctl run --rm)
```

References:

- urunc design: [https://urunc.io/design/](https://urunc.io/design/)

- urunc installation: [https://urunc.io/installation/](https://urunc.io/installation/)

- urunc quickstart: [https://urunc.io/quickstart/](https://urunc.io/quickstart/)

- Blog that inspired this: [https://nubificus.co.uk/blog/urunc_agent/](https://nubificus.co.uk/blog/urunc_agent/)

---

## Prerequisites (macOS M4)

Requirement
Version

macOS
Sequoia / any recent

[Homebrew](https://brew.sh/)
latest

[Lima](https://lima-vm.io/)
≥ 1.0

Go (for development only)
≥ 1.22

---

## Part 1 — Set Up the Lima VM

urunc runs Linux containers with microVM isolation. On macOS we need a Linux VM. Lima provides this cleanly.

### 1.1 Install Lima

```
brew install lima
```

Verify:

```
limactl --version
```

### 1.2 Create the Lima VM config

This config matches the install script exactly — QEMU type, nested virtualisation enabled (required for urunc's QEMU VMM to run inside Lima), and your home directory mounted.

```
cat > ~/urunc-dev.yaml 
```

Part
What
Doc Source

A
containerd + nerdctl
[quickstart](https://urunc.io/quickstart/)

B
runc
[installation](https://urunc.io/installation/)

C
CNI plugins
[installation](https://urunc.io/installation/)

D
nerdctl
[installation](https://urunc.io/installation/)

E
devmapper thinpool
[installation](https://urunc.io/installation/)

F
QEMU, Firecracker, Solo5, virtiofsd
[installation](https://urunc.io/installation/)

G
urunc + containerd-shim-urunc-v2
[installation](https://urunc.io/installation/)

H
`/etc/urunc/config.toml`
[configuration](https://urunc.io/configuration/)

I
urunc registered as containerd runtime (nerdctl uses it)
[installation](https://urunc.io/installation/)

>
> **Note:** Log out and back into the Lima shell after this step so group membership changes take effect:
>
> ```
> exit                         # leave Lima shell
> limactl shell urunc-dev      # re-enter
> ```
>
>

### 3.1 Verify devmapper snapshotter

```
sudo ctr plugin ls | grep devmapper
# io.containerd.snapshotter.v1  devmapper  linux/amd64  ok
```

If status is not `ok`, reload the thinpool:

```
sudo /usr/local/bin/scripts/dm_reload.sh
sudo systemctl restart containerd
```

### 3.2 Verify urunc binaries

```
which urunc
# /usr/local/bin/urunc

urunc --version

which containerd-shim-urunc-v2
# /usr/local/bin/containerd-shim-urunc-v2
```

### 3.3 Install Go (needed to run the sandbox manager)

The install script installs system tools but not Go. Install it now:

```
GO_VERSION=1.22.5
wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
rm "go${GO_VERSION}.linux-amd64.tar.gz"

echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

go version
# go version go1.22.5 linux/amd64
```

---

## Part 4 — Build the Tool Images

```
cd ~/ai-agent-sandbox
sudo bash scripts/01-build-tool-images.sh
```

This script:

1. Builds the base tool image using the [bunny](https://github.com/nubificus/bunny) Containerfile syntax.

2. Tags per-tool variants (`file-tool`, `code-tool`, `web-tool`, `db-tool`).

3. Runs the urunc quickstart to verify your install: starts an Nginx/Unikraft unikernel and curls it.

4. Prints the per-tool isolation profiles.

### Manual quickstart (from urunc docs)

You can also run the quickstart directly as documented at [https://urunc.io/quickstart/](https://urunc.io/quickstart/):

```
sudo nerdctl run -d \
   --runtime io.containerd.urunc.v2 \
   harbor.nbfc.io/nubificus/urunc/nginx-qemu-unikraft-initrd:latest
```

Get the container IP and curl it:

```
CONTAINER=$(sudo nerdctl ps -lq)
IP=$(sudo nerdctl inspect "$CONTAINER" --format '{{.NetworkSettings.IPAddress}}')
echo "Nginx at http://$IP"
curl "$IP"
# ...Powered by Unikraft...
```

Stop it:

```
sudo nerdctl stop "$CONTAINER"
```

---

## Part 5 — Run the Demo

### 5.1 Show isolation profiles (no containers needed)

```
cd ~/ai-agent-sandbox
go run ./cmd/sandbox-manager/main.go --profile
```

Expected output:

```
┌────────────────────────────────────────────────────────────────────┐
│  Per-Tool Isolation Profiles                                       │
│  Runtime: io.containerd.urunc.v2  (urunc microVM per tool call)    │
│  Source:  https://nubificus.co.uk/blog/urunc_agent/                │
└────────────────────────────────────────────────────────────────────┘

   ▶ file_tool
      Memory : 256MB   CPUs: 1.0
      Network: none
      Mounts : /tmp/ai-sandbox-workspace → /workspace
      Why    : File I/O only; no network; workspace mount rw; ...
      nerdctl : nerdctl run --rm --runtime io.containerd.urunc.v2 \
                   -m256M --cpus=1.0 --network=none \
                   -v /tmp/ai-sandbox-workspace:/workspace \
                   harbor.nbfc.io/nubificus/urunc/nginx-qemu-linux-raw:latest 

   ▶ code_tool
      Memory : 512MB   CPUs: 2.0
      Network: none
      Mounts : none  (host filesystem NOT exposed)
      ...
```

### 5.2 Verify the installation

```
sudo go run ./cmd/sandbox-manager/main.go --verify
```

### 5.3 Full live demo

```
sudo go run ./cmd/sandbox-manager/main.go --demo
```

This:

1. Runs the urunc quickstart (Nginx/Unikraft microVM).

2. Executes `code_tool` in an isolated microVM: no network, no mounts.

3. Executes `file_tool` in a microVM with workspace mounted.

4. Executes `web_tool` in a microVM with bridge network, no mounts.

5. Prints a summary comparing single-agent vs per-tool sandboxing.

### 5.4 Custom workspace

```
mkdir -p ~/myproject
echo "hello from host" > ~/myproject/note.txt

sudo go run ./cmd/sandbox-manager/main.go \
   --demo \
   --workspace ~/myproject
```

The `file_tool` microVM will see `note.txt` at `/workspace/note.txt`. The `code_tool` microVM will not see it at all.

---

## Part 6 — nerdctl Examples

urunc also integrates with nerdctl (containerd CLI). From the [quickstart docs](https://urunc.io/quickstart/):

```
# Redis on Rumprun over Solo5-hvt (from quickstart)
sudo nerdctl run -d \
   --snapshotter devmapper \
   --runtime io.containerd.urunc.v2 \
   harbor.nbfc.io/nubificus/urunc/redis-hvt-rumprun-raw:latest

# Check the running container
sudo nerdctl ps

# Inspect IP
sudo nerdctl inspect $(sudo nerdctl ps -lq) | grep IPAddress
```

---

## Part 7 — How It Differs from the Blog

The [Nubificus blog post](https://nubificus.co.uk/blog/urunc_agent/) isolates the *entire agent*:

```
# Blog: one sandbox for the whole agent
nerdctl run --runtime io.containerd.urunc.v2 opencode:latest
```

This project isolates *individual tool calls*:

```
Blog approach:
   [entire opencode agent] → one microVM
   If opencode is compromised: attacker has full agent capabilities

This project:
   [file_tool call]     → microVM A (no network)
   [code_tool call]     → microVM B (no mounts, no network)
   [web_tool call]      → microVM C (no mounts)
   [database_tool call] → microVM D (no mounts)
   If web_tool is compromised: attacker is in a microVM with no filesystem access
   They cannot reach the workspace, the database, or the code execution environment
```

The blog itself notes: *"if we explicitly share data or resources with a urunc container, that data is no longer protected."* Per-tool sandboxing minimises this surface — each microVM shares only what its tool actually requires.

---

## Project Structure

```
ai-agent-sandbox/
├── cmd/
│   └── sandbox-manager/
│       └── main.go              # Demo CLI entry point
├── pkg/
│   ├── tool/
│   │   └── registry.go          # Tool definitions + isolation profiles
│   └── sandbox/
│       ├── manager.go           # Orchestrates per-tool execution
│       └── spawner.go           # Builds nerdctl run commands
├── configs/
│   ├── containerd/config.toml   # containerd v2 config (devmapper + urunc)
│   ├── urunc/config.toml        # /etc/urunc/config.toml (monitor paths)
│   ├── cni/10-urunc-bridge.conf # CNI bridge for tool sandboxes
│   └── k8s/
│       ├── urunc-runtimeClass.yaml
│       ├── urunc-deploy.yaml
│       └── tool-sandbox-deployment.yaml
├── build/
│   ├── Containerfile            # bunny:containerfile syntax
│   ├── bunnyfile                # bunnyfile for custom kernel control
│   └── urunc.json.example       # Manual OCI annotation example
├── scripts/
│   ├── 00-install-prerequisites.sh  # Full install (docs-accurate)
│   ├── 01-build-tool-images.sh      # Build images + quickstart test
│   └── 02-deploy-k8s.sh             # Optional Kubernetes deployment
└── go.mod
```

---

## Stopping / Cleaning Up

```
# Stop and delete the Lima VM
exit                           # leave the Lima shell
limactl stop urunc-dev
limactl delete urunc-dev

# Remove the Lima config
rm ~/urunc-dev.yaml
```

---

## Supported urunc Monitors (Reference)

From [https://urunc.io/#current-support-of-unikernels-and-vmsandbox-monitors](https://urunc.io/#current-support-of-unikernels-and-vmsandbox-monitors):

Unikernel
VMM
Arch
Storage

Rumprun
Solo5-hvt, Solo5-spt
x86, aarch64
Block/Devmapper

Unikraft
Qemu, Firecracker
x86
Initrd, 9pfs

MirageOS
Qemu, Solo5-hvt, Solo5-spt
x86, aarch64
Block/Devmapper

Linux
Qemu, Firecracker
x86, aarch64
Initrd, Block, 9pfs, Virtiofs

Hermit
Qemu
x86
Initrd

This project uses **Linux over QEMU** for maximum compatibility.

---

## Troubleshooting

**`nerdctl: unknown runtime io.containerd.urunc.v2`**

The urunc runtime is not registered with containerd. Check:

```
grep -n "runtimes.urunc" /etc/containerd/config.toml
sudo systemctl restart containerd
```

**`devmapper status is not ok`**

The thinpool needs reloading:

```
sudo /usr/local/bin/scripts/dm_reload.sh
sudo systemctl restart containerd
sudo ctr plugin ls | grep devmapper
```

**`urunc: command not found`**

```
ls -la /usr/local/bin/urunc
# if missing, re-run Part G of the install script
```

**Lima VM won't start (nested virtualisation error)**

Your Mac must support nested virtualisation with QEMU. On Apple Silicon M4, Lima uses QEMU in TCG (emulation) mode for x86 guests — this is expected and slower but functional. The `nestedVirtualization: true` setting in the config enables KVM-inside-QEMU for the microVMs.

**`harbor.nbfc.io` image pull fails**

The harbor registry is public. Check connectivity inside the Lima VM:

```
curl -I https://harbor.nbfc.io
```

---

## References

All commands in this project trace back to official documentation:

Doc
URL

urunc overview
[https://urunc.io/](https://urunc.io/)

urunc installation
[https://urunc.io/installation/](https://urunc.io/installation/)

urunc quickstart
[https://urunc.io/quickstart/](https://urunc.io/quickstart/)

urunc configuration
[https://urunc.io/configuration/](https://urunc.io/configuration/)

urunc design
[https://urunc.io/design/](https://urunc.io/design/)

urunc hypervisor support
[https://urunc.io/hypervisor-support/](https://urunc.io/hypervisor-support/)

urunc unikernel support
[https://urunc.io/unikernel-support/](https://urunc.io/unikernel-support/)

AI agent blog
[https://nubificus.co.uk/blog/urunc_agent/](https://nubificus.co.uk/blog/urunc_agent/)

Lima VM
[https://lima-vm.io/](https://lima-vm.io/)

```

## Notes

- All scripts in scripts/ are Linux-only and should be executed inside a Linux VM or Linux host.
- The install script configures containerd, devmapper, CNI, urunc, and monitor binaries exactly as referenced in the official docs.
