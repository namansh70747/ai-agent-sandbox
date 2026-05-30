# urunc Per-Tool AI Agent Sandbox Platform

![CI](https://github.com/namansh70747/ai-agent-sandbox/actions/workflows/ci.yml/badge.svg)

> **One tool call → one isolated urunc microVM → destroyed on return.**
>
> Connect any AI agent. Every tool it calls automatically runs in hardware-isolated VM.

---

## What This Is

The [Nubificus blog](https://nubificus.co.uk/blog/urunc_agent/) shows running an entire AI agent inside a single [urunc](https://urunc.io/) microVM. This project goes further: **each individual tool call the agent makes gets its own microVM**, with a permission profile matched to exactly what that tool needs — and nothing more.

Tools are declared in [`configs/policies.yaml`](configs/policies.yaml). Each tool gets its own isolation profile: which VM monitor, network access, filesystem mounts, seccomp policy, read-only rootfs, and timeout. Any AI agent — OpenCode, Claude, GPT-4, LangChain, or a custom script — connects over **MCP** or a **REST API** with zero code changes.

### The Core Security Idea

```
Blog approach (whole agent in one sandbox):
  nerdctl run --runtime io.containerd.urunc.v2 opencode:latest
  └─ If compromised: attacker has full agent capabilities

This project (per-tool micro-sandboxes):
  file_tool  → microVM (no network, workspace only)
  code_tool  → microVM (no network, no mounts, seccomp, read-only rootfs)
  web_tool   → microVM (bridge network, no mounts)
  git_tool   → microVM (bridge network, workspace rw)
  ...
  └─ If web_tool compromised: attacker is in a VM with no filesystem access.
     They cannot reach the workspace, the database, or any other tool's data.
```

---

## Features

| Feature | Detail |
|---------|--------|
| **Per-tool microVM isolation** | Every tool call spawns a fresh urunc VM, destroyed on return |
| **Declarative YAML policy** | Add/tune tools by editing `configs/policies.yaml` — no recompile |
| **11 built-in tools** | file, code, web, database, git, python, shell, json, pdf, image, unikernel showcase |
| **MCP stdio** | Works with OpenCode, Claude Code, Cursor, Zed — add one config stanza |
| **MCP over SSE** | Remote MCP clients connect to `http://host:8080/mcp/sse` |
| **REST API** | Universal `POST /api/v1/execute` — any language, any agent |
| **Live dashboard** | `GET /` shows capability matrix, run form, live metrics |
| **Prometheus metrics** | `GET /metrics` — per-tool counters, durations, boot times |
| **Structured audit log** | JSON line per execution with tool, isolation profile, exit code, timing |
| **Boot telemetry** | Reads urunc's own `timestamps.log` and surfaces VM boot time per call |
| **Unit + integration tests** | `go test ./...` + `scripts/integration-test.sh` proves isolation live |
| **GitHub Actions CI** | gofmt, vet, build, test on every push |
| **Honest capability ledger** | `egress_allowlist` is declared-but-not-enforced — documented in `docs/URUNC.md` |

---

## Architecture

```
Your AI Agent (OpenCode / Claude / GPT-4 / LangChain / any)
        │
        ├─ MCP stdio          →  bin/mcp-server    (subprocess)
        ├─ MCP over SSE       →  :8080/mcp/sse
        └─ REST API           →  :8080/api/v1/execute
                                        │
                                 sandbox.Manager
                                        │
                              pkg/policy/loader.go
                              (reads configs/policies.yaml)
                                        │
                    ┌───────────────────┼────────────────────┐
                    ▼                   ▼                    ▼
              file_tool           code_tool             web_tool  ...
           urunc microVM       urunc microVM         urunc microVM
         (no net, ws mount)  (no net, no mount     (bridge, no mount
                              seccomp, ro-rootfs)   egress declared)
                    │                   │                    │
                    └──────── all destroyed on return ───────┘

Runtime: io.containerd.urunc.v2
Monitor: QEMU (default) · Firecracker (opt-in, if host supports)
Unikernel showcase: harbor.nbfc.io Unikraft nginx (real unikernel, not Linux VM)
```

---

## Tool Catalog

Defined in [`configs/policies.yaml`](configs/policies.yaml). Add a new tool = add a YAML block.

| Tool | Category | Network | Mounts | Seccomp | Read-Only | What it's for |
|------|----------|---------|--------|---------|-----------|---------------|
| `file_tool` | filesystem | none | workspace rw | default | no | Read/write workspace files |
| `code_tool` | compute | none | none | **strict** | **yes** | Untrusted shell/scripts — strongest profile |
| `web_tool` | network | bridge | none | default | no | HTTP/HTTPS requests |
| `database_tool` | network | bridge | none | default | no | SQL queries (DB_HOST/PORT injected) |
| `git_tool` | vcs | bridge | workspace rw | default | no | git clone/log/diff |
| `python_tool` | compute | none | workspace rw | default | no | Python scripts against workspace |
| `shell_tool` | compute | none | workspace **ro** | default | no | Inspect workspace, cannot mutate |
| `json_tool` | data | none | workspace **ro** | default | no | jq queries over workspace files |
| `pdf_tool` | data | none | workspace rw | default | no | pdftotext / pdfinfo (untrusted PDFs) |
| `image_tool` | data | none | workspace rw | default | no | ImageMagick identify/convert |
| `nginx_showcase` | showcase | bridge | none | default | no | **Real Unikraft unikernel** (not Linux VM) |

**Honesty:** `egress_allowlist` in the policy is **declared but not enforced** — bridge = full egress. Documented in [`docs/URUNC.md`](docs/URUNC.md) §8 and [`docs/adr/0004`](docs/adr/0004-egress-allowlist-declared-not-enforced.md).

---

## Quick Start (inside Lima shell)

### 1. Clone and set up

```bash
git clone https://github.com/namansh70747/ai-agent-sandbox
cd ai-agent-sandbox

# Install urunc stack (first time only, ~5 min)
sudo bash scripts/00-install-prerequisites.sh

# Install Go
GO_VERSION=1.22.5
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
wget -q "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz"
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-${ARCH}.tar.gz"
sudo chmod -R a+rx /usr/local/go
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc && source ~/.bashrc
```

### 2. Build the tool image and platform binaries

```bash
# Build the container image (Alpine + bash/curl/python3/jq/poppler/imagemagick)
sudo bash scripts/01-build-tool-images.sh

# Allow nerdctl without password (nerdctl needs root; platform runs as user)
echo "${USER} ALL=(ALL) NOPASSWD: /usr/local/bin/nerdctl" \
  | sudo tee /etc/sudoers.d/nerdctl-nopasswd
sudo chmod 440 /etc/sudoers.d/nerdctl-nopasswd

# Enable urunc boot telemetry
sudo cp configs/urunc/config.toml /etc/urunc/config.toml
sudo mkdir -p /var/log/urunc && sudo chmod 777 /var/log/urunc
sudo systemctl restart containerd

# Build all three binaries
bash scripts/03-build-servers.sh
```

### 3. Verify

```bash
# Unit tests (no urunc needed)
go test ./...

# Show all 11 tools from the policy
./bin/sandbox-manager --profile --policy configs/policies.yaml

# Which VM monitors work on this host?
make verify-monitors
```

### 4. Start the platform

```bash
./bin/api-server --policy configs/policies.yaml --addr :8080 --audit audit.log &

# Open the dashboard (Lima auto-forwards :8080 to macOS)
# → http://localhost:8080

# Confirm all 11 tools are live
curl -s http://localhost:8080/api/v1/tools | python3 -m json.tool
```

---

## Connect Your Agent

### Option A — OpenCode (terminal AI agent, MCP stdio)

```bash
BINARY=$(realpath ~/ai-agent-sandbox/bin/mcp-server)
POLICY=$(realpath ~/ai-agent-sandbox/configs/policies.yaml)

mkdir -p ~/.config/opencode
cat > ~/.config/opencode/config.json << EOF
{
  "mcp": {
    "urunc-sandbox": {
      "type": "local",
      "command": ["${BINARY}"],
      "env": {
        "SANDBOX_WORKSPACE": "/tmp/ai-sandbox-workspace",
        "SANDBOX_POLICY": "${POLICY}"
      }
    }
  }
}
EOF

opencode   # all 11 tools appear automatically
```

### Option B — Claude Code CLI (MCP stdio, reads `.mcp.json`)

The repo ships `.mcp.json` in the root. Update the binary path:

```bash
BINARY=$(realpath ~/ai-agent-sandbox/bin/mcp-server)
cat > .mcp.json << EOF
{
  "mcpServers": {
    "urunc-sandbox": {
      "command": "${BINARY}",
      "env": {
        "SANDBOX_WORKSPACE": "/tmp/ai-sandbox-workspace",
        "SANDBOX_POLICY": "$(realpath configs/policies.yaml)"
      }
    }
  }
}
EOF

claude   # run from project root; .mcp.json is picked up automatically
```

### Option C — Any HTTP agent (REST API)

```python
import requests

SANDBOX = "http://localhost:8080"

# Discover what tools are available and their isolation profiles
tools = requests.get(f"{SANDBOX}/api/v1/tools").json()
for t in tools:
    print(f"{t['name']}: network={t['network']} seccomp={t['seccomp']}")

# Execute any tool — isolation is automatic
def run(tool_type, command):
    r = requests.post(f"{SANDBOX}/api/v1/execute",
                      json={"tool_type": tool_type, "command": command})
    return r.json()

result = run("code_tool", ["python3", "-c", "print('hello from microVM')"])
print(result["stdout"])        # hello from microVM
print(result["exit_code"])     # 0
print(result["boot_time_ms"])  # VM boot time in ms
print(result["nerdctl_cmd"])   # full auditable nerdctl command
```

### Option D — LangChain / GPT function calling

```python
from langchain.tools import tool
import requests

SANDBOX = "http://localhost:8080"

@tool
def code_tool(command: list) -> str:
    """Execute a command in an isolated urunc microVM. No network, no filesystem."""
    r = requests.post(f"{SANDBOX}/api/v1/execute",
                      json={"tool_type": "code_tool", "command": command})
    result = r.json()
    return result["stdout"] if result["exit_code"] == 0 else f"Error: {result['stderr']}"

@tool
def web_tool(command: list) -> str:
    """Make HTTP requests in an isolated urunc microVM. Network only, no filesystem."""
    r = requests.post(f"{SANDBOX}/api/v1/execute",
                      json={"tool_type": "web_tool", "command": command})
    return r.json()["stdout"]
```

---

## REST API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/` | Live dashboard (capability matrix, run form, metrics) |
| `GET` | `/api/v1/tools` | All tools + their full isolation profiles |
| `POST` | `/api/v1/execute` | Execute a command in a sandboxed microVM |
| `GET` | `/metrics` | Prometheus-format metrics |
| `GET` | `/mcp/sse` | MCP over SSE event stream |
| `POST` | `/mcp/message` | MCP over SSE message endpoint |

### POST `/api/v1/execute`

**Request:**
```json
{
  "tool_type": "code_tool",
  "command": ["python3", "-c", "print('hello')"]
}
```

**Response:**
```json
{
  "tool_name":    "code_tool",
  "tool_type":    "code_tool",
  "nerdctl_cmd":  "sudo nerdctl run --rm --runtime io.containerd.urunc.v2 -m512M ...",
  "stdout":       "hello\n",
  "stderr":       "",
  "exit_code":    0,
  "duration_ms":  4270,
  "boot_time_ms": 4101
}
```

`nerdctl_cmd` is the **exact command that ran** — your full audit trail.

---

## Adding a New Tool

Edit [`configs/policies.yaml`](configs/policies.yaml). No code change, no recompile:

```yaml
tools:
  - name: node_tool
    type: node_tool          # unique type string
    category: compute
    network: none
    mounts:
      - "${WORKSPACE}:/workspace:rw"
    timeout_seconds: 60
    description: "Run Node.js scripts against the workspace."
    rationale: "JS compute on workspace files; no network; workspace rw."
```

Restart the server — `node_tool` appears over MCP, REST, and the dashboard automatically.

---

## Proving Isolation (Live Demo)

```bash
# In one terminal — watch VMs spawn and die in real time
watch -n 0.3 'sudo nerdctl ps --format "table {{.ID}}\t{{.Image}}\t{{.Status}}"'

# In another terminal — fire a 10-second tool
curl -s -X POST http://localhost:8080/api/v1/execute \
  -d '{"tool_type":"code_tool","command":["sleep","10"]}'
# Watch the container appear, run 10 seconds, disappear

# Run the automated isolation proof suite (requires running api-server)
bash scripts/integration-test.sh
# Expected:
#   [1] code_tool has NO network         PASS
#   [2] file_tool can read/write workspace PASS
#   [3] web_tool CANNOT see workspace    PASS
#   [4] shell_tool workspace is READ-ONLY PASS
#   ALL ISOLATION PROOFS PASSED
```

---

## Observability

### Dashboard

Open `http://localhost:8080` in your browser:
- **Capability matrix** — every tool, what it can/cannot do
- **Run form** — execute any tool directly from the browser
- **Live metrics** — auto-refreshes every 4 seconds

### Metrics (`/metrics`)

```
sandbox_executions_total{tool="code_tool"} 12
sandbox_executions_total{tool="json_tool"} 3
sandbox_exit_total{code="0"} 14
sandbox_exit_total{code="1"} 1
sandbox_duration_ms_avg{tool="code_tool"} 4271
sandbox_boot_ms_avg{tool="code_tool"} 4101
```

### Audit log

Every execution appends one JSON line to `audit.log`:

```json
{
  "time": "2026-05-31T01:36:04+05:30",
  "tool": "code_tool",
  "type": "code_tool",
  "monitor": "qemu",
  "network": "none",
  "seccomp": "configs/seccomp/strict.json",
  "read_only": true,
  "command": "echo hello",
  "exit_code": 0,
  "duration_ms": 4270,
  "boot_time_ms": 4101
}
```

---

## Project Structure

```
ai-agent-sandbox/
├── cmd/
│   ├── api-server/
│   │   ├── main.go          # HTTP REST + MCP over SSE + dashboard + metrics
│   │   └── dashboard.html   # embedded single-page dashboard
│   ├── mcp-server/
│   │   └── main.go          # stdio MCP server (OpenCode, Claude Code, Cursor)
│   └── sandbox-manager/
│       └── main.go          # CLI demo (--profile, --verify, --demo)
├── pkg/
│   ├── policy/
│   │   ├── loader.go        # YAML policy parser, defaults merge, validation
│   │   └── loader_test.go
│   ├── tool/
│   │   ├── registry.go      # ToolDef, IsolationProfile, Registry (order-preserving)
│   │   └── registry_test.go
│   ├── sandbox/
│   │   ├── manager.go       # Execute(), NewManagerFromPolicy(), ProfileSummary()
│   │   ├── spawner.go       # nerdctl command builder (monitor, seccomp, read-only, etc.)
│   │   ├── telemetry.go     # urunc boot-time telemetry from timestamps.log
│   │   └── spawner_test.go
│   ├── mcptools/
│   │   └── register.go      # dynamic MCP tool registration from registry
│   └── audit/
│       └── audit.go         # structured JSON audit log + Prometheus metrics
├── configs/
│   ├── policies.yaml        # ← ALL tool definitions live here
│   ├── seccomp/
│   │   └── strict.json      # deny-dangerous seccomp profile for code_tool
│   ├── mcp/
│   │   ├── opencode.json    # OpenCode MCP config snippet
│   │   └── dot_mcp.json     # generic .mcp.json (Claude Code, Cursor)
│   ├── containerd/config.toml
│   ├── urunc/config.toml    # timestamps enabled for boot telemetry
│   └── cni/10-urunc-bridge.conf
├── docs/
│   ├── URUNC.md             # cited urunc reference: monitors, annotations, seccomp
│   └── adr/                 # Architecture Decision Records
│       ├── 0001-declarative-policy-engine.md
│       ├── 0002-qemu-default-firecracker-opt-in.md
│       ├── 0003-seccomp-hardens-the-monitor.md
│       └── 0004-egress-allowlist-declared-not-enforced.md
├── scripts/
│   ├── 00-install-prerequisites.sh  # full urunc stack install
│   ├── 01-build-tool-images.sh      # build + verify container image
│   ├── 02-deploy-k8s.sh             # optional Kubernetes deployment
│   ├── 03-build-servers.sh          # build all platform binaries
│   └── integration-test.sh          # live cross-tool isolation proofs
├── examples/
│   └── demo_agent.py        # Python agent demo that exercises all 4 isolation guarantees
├── .github/
│   └── workflows/ci.yml     # gofmt + vet + build + test on every push
├── .mcp.json                # project-root MCP config (Claude Code)
├── Makefile
└── go.mod                   # go 1.23, only dep: mark3labs/mcp-go + gopkg.in/yaml.v3
```

---

## Honest Capability Ledger

This project only claims what urunc/nerdctl actually enforce. No security theater.

| Dimension | Enforced? | How |
|-----------|-----------|-----|
| Network isolation (`none`) | ✅ | No NIC attached to VM |
| Network bridge | ✅ | CNI bridge `urunc-net` |
| **Egress allowlist** | ❌ declared only | Bridge = full egress. No per-dest filter in nerdctl/urunc. Logged as warning. Real path: chained CNI firewall plugin. |
| VM monitor (QEMU/Firecracker) | ✅* | `--annotation com.urunc.unikernel.hypervisor` (*Firecracker-on-Linux verified per host) |
| Seccomp | ✅ | `--security-opt seccomp=` — hardens the **host-side monitor** (QEMU), not syscalls inside the guest |
| Read-only rootfs | ✅ | `--read-only` |
| Memory / CPU | ✅ | `-m` / `--cpus` |
| Per-tool timeout | ✅ | `context.WithTimeout` |
| Filesystem mounts | ✅ | `-v host:container[:ro]` |
| Boot telemetry | ✅ (serialized) | urunc `timestamps.log`, mutex-guarded attribution |

Full technical justification in [`docs/URUNC.md`](docs/URUNC.md) and [`docs/adr/`](docs/adr/).

---

## Knowledge Docs

- [`docs/URUNC.md`](docs/URUNC.md) — unikernel×monitor×storage matrix, OCI annotations, seccomp model, full capability ledger. Every claim cites `urunc.io`.
- [`docs/adr/0001`](docs/adr/0001-declarative-policy-engine.md) — why tools are YAML, not code
- [`docs/adr/0002`](docs/adr/0002-qemu-default-firecracker-opt-in.md) — why QEMU is default; Firecracker opt-in rationale
- [`docs/adr/0003`](docs/adr/0003-seccomp-hardens-the-monitor.md) — what seccomp actually protects in urunc
- [`docs/adr/0004`](docs/adr/0004-egress-allowlist-declared-not-enforced.md) — why egress can't be enforced and what would

---

## How It Differs from the Blog

| | [Nubificus blog](https://nubificus.co.uk/blog/urunc_agent/) | This project |
|--|--|--|
| What's isolated | Entire agent in one VM | Each tool call in its own VM |
| Threat model | Agent-level isolation | Per-tool, per-permission isolation |
| If compromised | Attacker has full agent capabilities | Attacker is trapped in that tool's VM |
| Tools | All share the same sandbox | Each gets only what it needs |
| Policy | None (single container) | Declarative YAML per tool |
| Integration | Agent must run inside urunc | Any agent, any language, zero changes |

---

## Troubleshooting

**`rootless containerd not running` error**
The api-server must use `sudo nerdctl`. If you see this, an old binary without `sudo` is running. Kill all server processes and restart:
```bash
pkill -f "api-server" && sleep 1
./bin/api-server --policy configs/policies.yaml --addr :8080 &
```

**`exec format error` from tools**
The tool image was built for wrong architecture. Rebuild:
```bash
sudo bash scripts/01-build-tool-images.sh
```

**Port already in use**
```bash
pkill -f "api-server" || true; pkill -f "bin/api-server" || true
sleep 1 && ./bin/api-server --policy configs/policies.yaml --addr :8080 &
```

**`devmapper status is not ok`**
```bash
sudo /usr/local/bin/scripts/dm_reload.sh
sudo systemctl restart containerd
```

**`go: command not found` when running with sudo**
```bash
sudo /usr/local/go/bin/go run ./cmd/...
# or: sudo env "PATH=$PATH:/usr/local/go/bin" go run ./cmd/...
```

---

## References

| Doc | URL |
|-----|-----|
| urunc overview | <https://urunc.io/> |
| urunc design | <https://urunc.io/design/> |
| urunc package/annotations | <https://urunc.io/package/> |
| urunc hypervisor support | <https://urunc.io/hypervisor-support/> |
| urunc seccomp | <https://urunc.io/design/seccomp/> |
| urunc configuration | <https://urunc.io/configuration/> |
| urunc quickstart | <https://urunc.io/quickstart/> |
| Blog that inspired this | <https://nubificus.co.uk/blog/urunc_agent/> |
| Lima VM | <https://lima-vm.io/> |
| MCP specification | <https://modelcontextprotocol.io/> |
| mcp-go library | <https://github.com/mark3labs/mcp-go> |
| bunny packaging tool | <https://github.com/nubificus/bunny> |
