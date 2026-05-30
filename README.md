# Per-Tool AI Agent Sandboxing with urunc

![CI](https://github.com/namansh70747/ai-agent-sandbox/actions/workflows/ci.yml/badge.svg)

> **One tool call → one isolated microVM → destroyed after return.**

**Tools are declared in [`configs/policies.yaml`](configs/policies.yaml), not hardcoded.** Each tool gets its own urunc microVM with a permission profile — monitor, network, mounts, seccomp, read-only rootfs, timeout — matched to exactly what it needs. Add or tune a tool by editing YAML; it appears over MCP, REST, and the CLI automatically. Deep design rationale lives in [`docs/URUNC.md`](docs/URUNC.md) and [`docs/adr/`](docs/adr/).

**Honesty first:** the platform claims only what urunc/nerdctl actually enforce. `egress_allowlist` is *declared but not enforced* (bridge = full egress) and seccomp hardens the *host-side monitor*, not the guest — both documented plainly in [`docs/URUNC.md`](docs/URUNC.md) §7–8.

The [Nubificus blog](https://nubificus.co.uk/blog/urunc_agent/) shows running an entire AI agent inside one [urunc](https://urunc.io/) microVM. This project goes further: **each tool call your agent makes gets its own microVM**, with a permission profile matched exactly to what that tool needs — and nothing more.

A compromised `web_tool` cannot read your workspace. A compromised `code_tool` cannot reach the database. Every microVM is created, used, and destroyed in a single `nerdctl run --rm` call.

---

## How It Works

```
Your AI Agent  (OpenCode / Claude / GPT / LangChain / anything)
      │
      │  calls a tool
      ▼
 ┌─────────────────────────────────────┐
 │   urunc Sandbox Platform            │
 │                                     │
 │   file_tool    → microVM (no net,   │
 │                   workspace mount)  │
 │   code_tool    → microVM (no net,   │
 │                   no mounts)        │
 │   web_tool     → microVM (network,  │
 │                   no mounts)        │
 │   database_tool→ microVM (network,  │
 │                   no mounts)        │
 └─────────────────────────────────────┘
      │
      │  result returned to agent
      ▼
 microVM destroyed — nothing persists
```

### Isolation Profiles

| Tool | Network | Filesystem | Memory | Why |
|------|---------|------------|--------|-----|
| `file_tool` | ❌ none | ✅ workspace only (rw) | 256 MB | File I/O needs no network |
| `code_tool` | ❌ none | ❌ none | 512 MB | Strongest — untrusted code |
| `web_tool` | ✅ bridge | ❌ none | 256 MB | HTTP only, no exfiltration path |
| `database_tool` | ✅ bridge | ❌ none | 256 MB | DB port only, no filesystem |

---

## Connect Your Agent (Quick Start)

There are **three ways** to connect any AI agent. Pick the one that matches your setup.

### Option A — MCP stdio (OpenCode, Claude Code, Cursor, Zed, Windsurf)

These agents launch the MCP server binary as a subprocess. No code changes required in your agent.

**1. Build the binary** (inside Lima shell):
```bash
git clone https://github.com/namansh70747/ai-agent-sandbox
cd ai-agent-sandbox
bash scripts/00-install-prerequisites.sh   # first time only
sudo bash scripts/01-build-tool-images.sh  # first time only
bash scripts/03-build-servers.sh           # builds bin/mcp-server
```

**2. Add one config stanza to your agent:**

```json
// OpenCode  →  ~/.config/opencode/config.json
{
  "mcp": {
    "urunc-sandbox": {
      "type": "local",
      "command": ["/home/<you>/ai-agent-sandbox/bin/mcp-server"],
      "env": { "SANDBOX_WORKSPACE": "/tmp/ai-sandbox-workspace" }
    }
  }
}
```

```json
// Claude Code CLI  →  .mcp.json in your project root
// Cursor / Zed     →  same format, check their MCP settings
{
  "mcpServers": {
    "urunc-sandbox": {
      "command": "/home/<you>/ai-agent-sandbox/bin/mcp-server",
      "env": { "SANDBOX_WORKSPACE": "/tmp/ai-sandbox-workspace" }
    }
  }
}
```

**3. Restart your agent.** Ask it to use `code_tool` — isolation happens automatically.

---

### Option B — REST API (GPT function calling, LangChain, LlamaIndex, CrewAI, any HTTP client)

Start the API server inside Lima:
```bash
sudo ./bin/api-server --addr :8080
# Lima auto-forwards :8080 → http://localhost:8080 on macOS
```

Call it from any language:

```python
# Python — drop this helper into any agent
import requests

SANDBOX = "http://localhost:8080"

def run_sandboxed(tool_type: str, command: list[str]) -> str:
    """
    tool_type: "code_tool" | "file_tool" | "web_tool" | "database_tool"
    command:   ["executable", "arg1", "arg2", ...]
    """
    r = requests.post(f"{SANDBOX}/api/v1/execute", json={
        "tool_type": tool_type,
        "command": command
    })
    result = r.json()
    if result["exit_code"] == 0:
        return result["stdout"]
    raise RuntimeError(f"exit {result['exit_code']}: {result['stderr']}")

# Examples:
run_sandboxed("code_tool", ["python3", "-c", "print('hello from microVM')"])
run_sandboxed("file_tool", ["ls", "/workspace"])
run_sandboxed("web_tool",  ["curl", "-s", "https://example.com"])
```

```javascript
// JavaScript / TypeScript
const SANDBOX = "http://localhost:8080"

async function runSandboxed(toolType, command) {
  const res = await fetch(`${SANDBOX}/api/v1/execute`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ tool_type: toolType, command })
  })
  const result = await res.json()
  if (result.exit_code !== 0) throw new Error(result.stderr)
  return result.stdout
}

// Examples:
await runSandboxed("code_tool", ["node", "-e", "console.log('hello')"])
await runSandboxed("file_tool", ["cat", "/workspace/notes.txt"])
```

**Discover available tools before calling:**
```bash
curl http://localhost:8080/api/v1/tools
```
Returns JSON with all 4 tools and their isolation profiles.

**OpenAI function calling integration:**
```python
import openai, requests

SANDBOX = "http://localhost:8080"

# Define tools in OpenAI format
tools = [
    {
        "type": "function",
        "function": {
            "name": "code_tool",
            "description": "Execute code in an isolated microVM. No network, no filesystem access.",
            "parameters": {
                "type": "object",
                "properties": {
                    "command": {"type": "array", "items": {"type": "string"}}
                },
                "required": ["command"]
            }
        }
    },
    {
        "type": "function",
        "function": {
            "name": "web_tool",
            "description": "Make HTTP requests in an isolated microVM. Network only, no filesystem.",
            "parameters": {
                "type": "object",
                "properties": {
                    "command": {"type": "array", "items": {"type": "string"}}
                },
                "required": ["command"]
            }
        }
    }
]

def handle_tool_call(name, args):
    r = requests.post(f"{SANDBOX}/api/v1/execute",
                      json={"tool_type": name, "command": args["command"]})
    return r.json()["stdout"]
```

---

### Option C — MCP over SSE (Remote agents, web-hosted agents)

For agents that support MCP HTTP transport (not running on the same machine):

```bash
# Start the server (SSE endpoint lives at /mcp/sse)
sudo ./bin/api-server --addr :8080
```

```python
from mcp import ClientSession
from mcp.client.sse import sse_client

async with sse_client("http://localhost:8080/mcp/sse") as (read, write):
    async with ClientSession(read, write) as session:
        await session.initialize()

        # Your agent calls sandboxed tools like any MCP tool
        result = await session.call_tool("code_tool", {
            "command": ["python3", "-c", "print('isolated')"]
        })
        print(result.content[0].text)
```

---

## What the Response Looks Like

Every tool call returns structured text:

```
tool:         code_tool
exit_code:    0
duration_ms:  4270
nerdctl_cmd:  sudo nerdctl run --rm --runtime io.containerd.urunc.v2 -m512M --cpus=2.0 --network=none localhost/ai-sandbox/base-tool:latest echo hello
stdout:
hello
```

The `nerdctl_cmd` line is the exact command that ran — fully auditable.

---

## REST API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/` | Human-readable index |
| `GET` | `/api/v1/tools` | List all tools and their isolation profiles |
| `POST` | `/api/v1/execute` | Execute a command in a sandboxed microVM |
| `GET` | `/mcp/sse` | MCP over SSE — event stream |
| `POST` | `/mcp/message` | MCP over SSE — message endpoint |

**POST `/api/v1/execute` request body:**
```json
{
  "tool_type": "code_tool",
  "command": ["echo", "hello world"]
}
```

**Response:**
```json
{
  "tool_name": "code_tool",
  "tool_type": "code_tool",
  "command": ["echo", "hello world"],
  "nerdctl_cmd": "sudo nerdctl run --rm --runtime io.containerd.urunc.v2 ...",
  "stdout": "hello world\n",
  "stderr": "",
  "exit_code": 0,
  "duration_ms": 4270
}
```

---

## Full Setup (First Time)

### Prerequisites

| Requirement | Version |
|-------------|---------|
| macOS | Sequoia or later |
| [Homebrew](https://brew.sh/) | latest |
| [Lima](https://lima-vm.io/) | ≥ 1.0 |

### Step 1 — Install Lima and start the VM

```bash
brew install lima
limactl start ~/urunc-dev.yaml    # use the Lima config from the repo
limactl shell urunc-dev
```

### Step 2 — Install urunc and all dependencies

```bash
# Inside Lima shell
cd ~/ai-agent-sandbox
sudo bash scripts/00-install-prerequisites.sh
```

This installs: containerd, runc, CNI plugins, nerdctl, devmapper, QEMU, Firecracker, Solo5, virtiofsd, urunc, and the containerd shim.

### Step 3 — Install Go

```bash
GO_VERSION=1.22.5
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
wget -q "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz"
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-${ARCH}.tar.gz"
sudo chmod -R a+rx /usr/local/go
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

### Step 4 — Build the tool images

```bash
sudo bash scripts/01-build-tool-images.sh
```

Builds a Linux/QEMU microVM image (Alpine + bash + curl + python3) and tags it for each tool type.

### Step 5 — Build the platform binaries

```bash
bash scripts/03-build-servers.sh
```

Produces three binaries in `bin/`:

| Binary | Purpose |
|--------|---------|
| `bin/mcp-server` | stdio MCP server — for OpenCode, Claude Code, Cursor |
| `bin/api-server` | HTTP REST + MCP over SSE — for any agent via HTTP |
| `bin/sandbox-manager` | CLI demo — shows isolation profiles, runs live demo |

### Step 6 — Allow nerdctl without password

The MCP server runs as a regular user but nerdctl needs root:

```bash
echo "${USER} ALL=(ALL) NOPASSWD: /usr/local/bin/nerdctl" \
  | sudo tee /etc/sudoers.d/nerdctl-nopasswd
sudo chmod 440 /etc/sudoers.d/nerdctl-nopasswd
```

### Step 7 — Verify everything works

```bash
# Show isolation profiles (no urunc needed)
./bin/sandbox-manager --profile

# Test MCP tools/list (no urunc needed)
printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}\n{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}\n' \
  | ./bin/mcp-server 2>/dev/null

# Test REST API profiles (no urunc needed)
./bin/api-server &
curl -s http://localhost:8080/api/v1/tools | python3 -m json.tool

# Test live execution (requires urunc)
curl -s -X POST http://localhost:8080/api/v1/execute \
  -H "Content-Type: application/json" \
  -d '{"tool_type":"code_tool","command":["echo","hello from microVM"]}' \
  | python3 -m json.tool
```

---

## Project Structure

```
ai-agent-sandbox/
├── cmd/
│   ├── mcp-server/main.go       # stdio MCP server entry point
│   ├── api-server/main.go       # HTTP REST + SSE MCP entry point
│   └── sandbox-manager/main.go  # demo CLI entry point
├── pkg/
│   ├── mcptools/register.go     # shared MCP tool registration
│   ├── tool/registry.go         # tool definitions + isolation profiles
│   └── sandbox/
│       ├── manager.go           # orchestrates per-tool execution
│       └── spawner.go           # builds nerdctl run commands
├── configs/
│   ├── mcp/
│   │   ├── opencode.json        # OpenCode MCP config snippet
│   │   └── dot_mcp.json         # generic .mcp.json for other clients
│   ├── containerd/config.toml   # containerd v2 (devmapper + urunc)
│   ├── urunc/config.toml        # /etc/urunc/config.toml
│   ├── cni/10-urunc-bridge.conf # CNI bridge for sandboxes
│   └── k8s/                     # optional Kubernetes deployment
├── build/
│   ├── Containerfile            # bunny syntax — builds the tool image
│   ├── bunnyfile                # kernel config
│   └── urunc.json.example       # OCI annotation reference
├── scripts/
│   ├── 00-install-prerequisites.sh  # full urunc stack install
│   ├── 01-build-tool-images.sh      # build + verify images
│   ├── 02-deploy-k8s.sh             # optional K8s deployment
│   └── 03-build-servers.sh          # build all platform binaries
├── Makefile
└── go.mod
```

---

## How This Differs from the Blog

The [Nubificus blog](https://nubificus.co.uk/blog/urunc_agent/) puts the **entire agent** in one microVM:

```bash
# Blog: whole agent → one sandbox
nerdctl run --runtime io.containerd.urunc.v2 opencode:latest
```

This project puts **each tool call** in its own microVM:

```
Blog:   [entire agent]  →  one microVM  →  if compromised: full agent access

This:   [file_tool]     →  microVM A  (no network)
        [code_tool]     →  microVM B  (no network, no filesystem)
        [web_tool]      →  microVM C  (network, no filesystem)
        [db_tool]       →  microVM D  (network, no filesystem)
        If web_tool is compromised → attacker is in a VM with no files, no DB
```

The blog notes: *"if we explicitly share data or resources with a urunc container, that data is no longer protected."* Per-tool sandboxing minimises this surface — each VM gets only what its tool actually needs.

---

## Troubleshooting

**`exec format error` when running tools**

The tool image was built for the wrong architecture, or the pre-built `nginx-qemu-linux-raw` image is being used. Run:
```bash
sudo bash scripts/01-build-tool-images.sh
```
to rebuild `localhost/ai-sandbox/base-tool:latest` for your architecture.

**`rootless containerd is not running`**

The MCP server is running as a non-root user. Fix with the sudoers step in Step 6 above, then rebuild the binaries.

**`nerdctl: unknown runtime io.containerd.urunc.v2`**

The urunc shim is not registered with containerd:
```bash
grep -n "runtimes.urunc" /etc/containerd/config.toml
sudo systemctl restart containerd
```

**`devmapper status is not ok`**

The thinpool needs reloading:
```bash
sudo /usr/local/bin/scripts/dm_reload.sh
sudo systemctl restart containerd
sudo ctr plugin ls | grep devmapper
```

**`go: permission denied`**

Go was installed as root. Fix:
```bash
sudo chmod -R a+rx /usr/local/go
```

---

## References

| Doc | URL |
|-----|-----|
| urunc overview | https://urunc.io/ |
| urunc installation | https://urunc.io/installation/ |
| urunc quickstart | https://urunc.io/quickstart/ |
| urunc design | https://urunc.io/design/ |
| urunc configuration | https://urunc.io/configuration/ |
| Blog that inspired this | https://nubificus.co.uk/blog/urunc_agent/ |
| Lima VM | https://lima-vm.io/ |
| MCP specification | https://modelcontextprotocol.io/ |
| mcp-go library | https://github.com/mark3labs/mcp-go |
