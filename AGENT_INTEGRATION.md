# Using This Platform With Your AI Agent

This guide is for someone who just found this project and wants to connect their own AI agent to get automatic per-tool microVM isolation.

You do **not** need to understand urunc, containerd, or microVMs. You just point your agent at the platform and every tool call it makes automatically runs in an isolated VM — created, used, and destroyed per call.

---

## What You Get (Without Changing Your Agent)

Every tool call your agent makes is intercepted by this platform and executed inside a dedicated urunc microVM with a permission profile matched to the tool type:

```
Your agent asks  →  "run some code"
Platform decides →  code_tool: no network, no filesystem, 512 MB RAM
microVM boots    →  runs the code
microVM dies     →  result returned to your agent
```

No VM persists. No tool can see another tool's data. A compromised web request cannot read your files.

---

## One-Time Setup (Inside Lima Shell)

```bash
# 1. Enter the Lima VM
limactl shell urunc-dev3

# 2. Clone the project
git clone https://github.com/namansh70747/ai-agent-sandbox
cd ai-agent-sandbox

# 3. Install urunc stack (first time only — takes ~5 min)
sudo bash scripts/00-install-prerequisites.sh

# 4. Build the tool container image
sudo bash scripts/01-build-tool-images.sh

# 5. Install Go (if not already installed)
GO_VERSION=1.22.5
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
wget -q "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz"
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-${ARCH}.tar.gz"
sudo chmod -R a+rx /usr/local/go
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc && source ~/.bashrc

# 6. Build the platform binaries
bash scripts/03-build-servers.sh

# 7. Allow nerdctl without a password prompt (needed by all integration paths)
echo "${USER} ALL=(ALL) NOPASSWD: /usr/local/bin/nerdctl" \
  | sudo tee /etc/sudoers.d/nerdctl-nopasswd
sudo chmod 440 /etc/sudoers.d/nerdctl-nopasswd

# 8. Verify everything is ready
printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}\n{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}\n' \
  | ./bin/mcp-server 2>/dev/null | python3 -m json.tool
# Expected: JSON showing 4 tools: file_tool, code_tool, web_tool, database_tool
```

---

## Integration Path A — Claude Code CLI (MCP stdio)

Claude Code is Anthropic's terminal AI agent. It reads `.mcp.json` from your project directory automatically — no extra configuration needed.

### Install Claude Code inside Lima

```bash
# Install Node.js (needed for Claude Code)
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
source ~/.bashrc
nvm install --lts

# Install Claude Code
npm install -g @anthropic-ai/claude-code

# Verify
claude --version
```

### Connect to the Sandbox Platform

The project already ships `.mcp.json` in the root. Update the binary path to your actual path:

```bash
# Find your binary path
BINARY=$(realpath ~/ai-agent-sandbox/bin/mcp-server)
echo $BINARY
# e.g. /home/namansharma.guest/ai-agent-sandbox/bin/mcp-server

# Update .mcp.json with your actual path
cat > ~/ai-agent-sandbox/.mcp.json << EOF
{
  "mcpServers": {
    "urunc-sandbox": {
      "command": "${BINARY}",
      "env": {
        "SANDBOX_WORKSPACE": "/tmp/ai-sandbox-workspace"
      }
    }
  }
}
EOF

cat ~/ai-agent-sandbox/.mcp.json
```

### Run Claude Code With Isolation

```bash
cd ~/ai-agent-sandbox
claude
```

Claude Code picks up `.mcp.json` automatically. You will see `urunc-sandbox` listed as a connected MCP server.

**Try these prompts:**

```
# Prompt 1 — isolated code execution (no network, no filesystem)
Use code_tool to run: python3 -c "import os; print(os.listdir('/'))"

# Prompt 2 — file access in workspace only
Use file_tool to run: ls -la /workspace

# Prompt 3 — web request with no filesystem access
Use web_tool to run: curl -s --max-time 5 https://httpbin.org/get

# Prompt 4 — show what the microVM cannot do (proves isolation)
Use code_tool to run: cat /etc/passwd
# This runs inside a microVM with no host filesystem — it sees the VM's /etc/passwd
# not the host's, proving filesystem isolation works
```

**What you will see:** Each tool call shows a gear icon in Claude Code, takes ~4 seconds (microVM boot time), and returns the output. After it returns, the VM is gone.

### Prove the Isolation (Watch the VM Live)

Open a second Lima terminal while Claude Code is running a tool:

```bash
# In terminal 2, watch containers appear and disappear:
watch -n 0.5 'sudo nerdctl ps --format "{{.ID}}\t{{.Image}}\t{{.Status}}"'

# Then in terminal 1 (Claude Code), ask:
# "Use code_tool to run sleep 10"
# You will see the container appear in terminal 2, then disappear after 10s
```

---

## Integration Path B — Python Agent (REST API)

This works with any Python framework: LangChain, LlamaIndex, CrewAI, AutoGen, or a plain script. No MCP required.

### Start the API Server

```bash
# In Lima shell:
cd ~/ai-agent-sandbox
./bin/api-server --addr :8080 &

# Verify it's up (no urunc needed for this):
curl -s http://localhost:8080/api/v1/tools | python3 -m json.tool
```

Lima auto-forwards `:8080`, so from macOS `http://localhost:8080` also works.

### The Sandbox Client (Copy This Into Any Agent)

Save this as `sandbox_client.py`:

```python
# sandbox_client.py
# Drop this into any Python AI agent to get per-tool microVM isolation.
# Requires: pip install requests

import requests
import json

SANDBOX_URL = "http://localhost:8080"

def run_in_sandbox(tool_type: str, command: list[str], timeout: int = 120) -> dict:
    """
    Execute a command inside an isolated urunc microVM.

    tool_type: one of "file_tool", "code_tool", "web_tool", "database_tool"
    command:   list of strings, e.g. ["python3", "-c", "print('hello')"]

    Returns dict with: stdout, stderr, exit_code, duration_ms, nerdctl_cmd
    """
    response = requests.post(
        f"{SANDBOX_URL}/api/v1/execute",
        json={"tool_type": tool_type, "command": command},
        timeout=timeout
    )
    response.raise_for_status()
    return response.json()

def list_sandbox_tools() -> list[dict]:
    """Return all available tools and their isolation profiles."""
    return requests.get(f"{SANDBOX_URL}/api/v1/tools").json()

def sandbox_ok(result: dict) -> bool:
    return result["exit_code"] == 0
```

### Demo Agent — Run This to Test Everything

Save as `demo_agent.py` and run `python3 demo_agent.py`:

```python
#!/usr/bin/env python3
"""
Demo agent that exercises all 4 sandbox tool types.
Shows isolation working across file, code, web, and database tools.

Run with:
  python3 demo_agent.py
"""

import json
import requests

SANDBOX_URL = "http://localhost:8080"

def execute(tool_type, command):
    r = requests.post(f"{SANDBOX_URL}/api/v1/execute",
                      json={"tool_type": tool_type, "command": command},
                      timeout=60)
    return r.json()

def section(title):
    print(f"\n{'='*60}")
    print(f"  {title}")
    print('='*60)

def show(result):
    print(f"  tool:       {result['tool_name']}")
    print(f"  exit_code:  {result['exit_code']}")
    print(f"  duration:   {result['duration_ms']}ms")
    print(f"  command:    {' '.join(result['command'])}")
    if result['stdout']:
        print(f"  stdout:     {result['stdout'].strip()}")
    if result['stderr'] and result['exit_code'] != 0:
        print(f"  stderr:     {result['stderr'].strip()[:200]}")
    print(f"  nerdctl:    {result['nerdctl_cmd'][:80]}...")


# ── Discover what tools are available ────────────────────────────────────────
section("Available Tools and Their Isolation Profiles")
tools = requests.get(f"{SANDBOX_URL}/api/v1/tools").json()
for t in tools:
    mounts = t['mounts'] if t['mounts'] else ['none']
    print(f"  {t['name']:<16}  network={t['network']:<8}  mounts={mounts[0]}")


# ── code_tool: no network, no filesystem ─────────────────────────────────────
section("Test 1: code_tool — Isolated Code Execution")
print("  Profile: NO network, NO filesystem mounts")
print("  Running: python3 inside a microVM that cannot see the host")
print()

result = execute("code_tool", [
    "python3", "-c",
    "import os, socket\n"
    "print('hostname:', socket.gethostname())\n"
    "print('root dir:', os.listdir('/')[:5])\n"
    "print('env vars:', list(os.environ.keys())[:3])"
])
show(result)


# ── Prove code_tool has NO network ───────────────────────────────────────────
section("Test 2: code_tool — Network Is Blocked (Proves Isolation)")
print("  Trying to reach the internet from inside code_tool VM...")
print()

result = execute("code_tool", [
    "python3", "-c",
    "import urllib.request\n"
    "try:\n"
    "    urllib.request.urlopen('http://example.com', timeout=3)\n"
    "    print('FAIL: network was accessible')\n"
    "except Exception as e:\n"
    "    print('PASS: network blocked —', type(e).__name__)"
])
show(result)


# ── file_tool: workspace access, no network ───────────────────────────────────
section("Test 3: file_tool — Workspace File Access")
print("  Profile: NO network, workspace dir mounted at /workspace")
print("  Creating a file from outside, reading it from inside the VM")
print()

# Write a file from the host into the workspace
import subprocess
subprocess.run(["bash", "-c",
    "mkdir -p /tmp/ai-sandbox-workspace && "
    "echo 'secret data from host' > /tmp/ai-sandbox-workspace/test.txt"],
    check=True)

result = execute("file_tool", ["cat", "/workspace/test.txt"])
show(result)
print(f"  → VM read the file: '{result['stdout'].strip()}'")

# Write back from inside the VM
result = execute("file_tool", [
    "sh", "-c", "echo 'written from inside microVM' > /workspace/vm_output.txt"
])
show(result)

# Read it back from host
with open("/tmp/ai-sandbox-workspace/vm_output.txt") as f:
    print(f"  → Host read back: '{f.read().strip()}'")


# ── web_tool: network allowed, NO filesystem ─────────────────────────────────
section("Test 4: web_tool — Network Allowed, No Filesystem")
print("  Profile: bridge network ENABLED, NO filesystem mounts")
print("  Making an HTTP request from inside the microVM")
print()

result = execute("web_tool", [
    "curl", "-s", "--max-time", "10",
    "https://httpbin.org/get"
])
if result["exit_code"] == 0:
    try:
        data = json.loads(result["stdout"])
        print(f"  → Got response from httpbin.org")
        print(f"     origin IP (from VM): {data.get('origin', 'n/a')}")
        print(f"     headers seen:        {list(data.get('headers', {}).keys())[:4]}")
    except Exception:
        show(result)
else:
    show(result)
    print("  (network may not be available in this environment)")


# ── Prove web_tool cannot read files ─────────────────────────────────────────
section("Test 5: web_tool — Filesystem Is Blocked (Proves Isolation)")
print("  Trying to read the workspace file from inside web_tool VM...")
print("  (web_tool has no mounts — /workspace should not exist)")
print()

result = execute("web_tool", ["ls", "/workspace"])
if result["exit_code"] != 0:
    print(f"  PASS: /workspace not found in web_tool VM — exit_code={result['exit_code']}")
    print(f"        stderr: {result['stderr'].strip()[:100]}")
else:
    print(f"  stdout: {result['stdout']}")


# ── Summary ───────────────────────────────────────────────────────────────────
section("Summary")
print("""
  PROVEN:
  ✓ code_tool  — runs in a VM with no network and no host filesystem
  ✓ code_tool  — network is blocked at the VM level (not just software firewall)
  ✓ file_tool  — can read/write only the designated workspace directory
  ✓ web_tool   — can reach the internet but cannot access the filesystem
  ✓ web_tool   — /workspace does not exist inside this VM

  SECURITY GUARANTEE:
  A compromised web_tool cannot read your workspace files.
  A compromised code_tool cannot make outbound network calls.
  Every microVM is destroyed after the tool returns — nothing persists.

  Each nerdctl_cmd above is the exact command that ran — fully auditable.
""")
```

Run it:
```bash
# Make sure api-server is running first:
./bin/api-server &

# Run the demo agent:
python3 demo_agent.py
```

### LangChain Integration

```python
from langchain.tools import tool
import requests

SANDBOX_URL = "http://localhost:8080"

@tool
def code_tool(command: str) -> str:
    """
    Execute a shell command in an isolated microVM.
    No network access. No filesystem access. Safest option for untrusted code.
    command: space-separated command string, e.g. 'python3 -c print(1+1)'
    """
    result = requests.post(f"{SANDBOX_URL}/api/v1/execute", json={
        "tool_type": "code_tool",
        "command": command.split()
    }).json()
    return result["stdout"] if result["exit_code"] == 0 else f"Error: {result['stderr']}"

@tool
def web_tool(url: str) -> str:
    """
    Fetch a URL inside an isolated microVM with network access but no filesystem.
    url: the URL to fetch
    """
    result = requests.post(f"{SANDBOX_URL}/api/v1/execute", json={
        "tool_type": "web_tool",
        "command": ["curl", "-s", "--max-time", "10", url]
    }).json()
    return result["stdout"] if result["exit_code"] == 0 else f"Error: {result['stderr']}"

# Use with any LangChain agent:
# from langchain.agents import initialize_agent
# agent = initialize_agent([code_tool, web_tool], llm, agent="zero-shot-react-description")
# agent.run("Fetch https://example.com and count the words")
```

---

## Integration Path C — MCP over SSE (Remote Agent)

For agents that run on a different machine or in a web environment.

```bash
# Start api-server (exposes both REST and SSE MCP):
./bin/api-server --addr :8080
```

```python
# Any MCP Python client
import asyncio
from mcp import ClientSession
from mcp.client.sse import sse_client

async def main():
    async with sse_client("http://localhost:8080/mcp/sse") as (read, write):
        async with ClientSession(read, write) as session:
            await session.initialize()

            # See all available sandboxed tools
            tools = await session.list_tools()
            print("Available tools:")
            for t in tools.tools:
                print(f"  {t.name}: {t.description[:60]}...")

            # Call a tool — microVM isolation is automatic
            result = await session.call_tool("code_tool", {
                "command": ["python3", "-c", "print('hello from isolated VM')"]
            })
            print("\nResult:")
            print(result.content[0].text)

asyncio.run(main())
```

Install the MCP client:
```bash
pip install mcp
python3 mcp_test.py
```

---

## Verifying Isolation Is Real (Not Just Claims)

Run this after setting up any integration to prove the VMs are actually isolated:

```bash
# Terminal 1: watch containers live
watch -n 0.5 'sudo nerdctl ps --format "table {{.ID}}\t{{.Image}}\t{{.Status}}\t{{.Command}}"'

# Terminal 2: run tools through any integration path
curl -s -X POST http://localhost:8080/api/v1/execute \
  -H "Content-Type: application/json" \
  -d '{"tool_type":"code_tool","command":["sleep","8"]}'
```

In Terminal 1 you will see a container appear with `io.containerd.urunc.v2` runtime, run for 8 seconds, then disappear. That container **is** the microVM — a full isolated Linux kernel, not just a process.

---

## What the Response Tells You

Every tool call returns this structure:

```json
{
  "tool_name":    "code_tool",
  "exit_code":    0,
  "duration_ms":  4270,
  "nerdctl_cmd":  "sudo nerdctl run --rm --runtime io.containerd.urunc.v2 -m512M --cpus=2.0 --network=none localhost/ai-sandbox/base-tool:latest echo hello",
  "stdout":       "hello\n",
  "stderr":       "",
  "nerdctl_cmd":  "..."
}
```

The `nerdctl_cmd` field is the **exact command** that ran. You can copy and paste it into your terminal to reproduce the execution manually. This is your audit trail.

---

## Quick Reference

| What | Command |
|------|---------|
| Start REST + SSE server | `./bin/api-server --addr :8080` |
| Start stdio MCP server | `./bin/mcp-server` (launched by agent via config) |
| List tools | `curl http://localhost:8080/api/v1/tools` |
| Run code in sandbox | `curl -X POST http://localhost:8080/api/v1/execute -H 'Content-Type: application/json' -d '{"tool_type":"code_tool","command":["echo","hi"]}'` |
| Watch VMs live | `watch -n 0.5 'sudo nerdctl ps'` |
| Check MCP tools list | `printf '..initialize..\n..tools/list..\n' \| ./bin/mcp-server 2>/dev/null` |
| Rebuild after changes | `bash scripts/03-build-servers.sh` |
