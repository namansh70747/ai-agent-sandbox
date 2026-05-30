// cmd/mcp-server/main.go
//
// Stdio MCP server — the primary integration point for any MCP-compatible agent
// running inside the Lima VM (OpenCode, Claude Code CLI, Cursor, Zed, etc.).
//
// The agent's MCP host launches this binary as a subprocess.
// stdin/stdout carry MCP JSON-RPC 2.0. All logs go to stderr so they
// never corrupt the JSON-RPC stream on stdout.
//
// Configuration via environment variables:
//
//	SANDBOX_WORKSPACE   host directory mounted into file_tool microVMs
//	                    (default: /tmp/ai-sandbox-workspace)
//
// Usage in OpenCode config (~/.config/opencode/config.json):
//
//	{
//	  "mcp": {
//	    "urunc-sandbox": {
//	      "type": "local",
//	      "command": ["/path/to/bin/mcp-server"],
//	      "env": { "SANDBOX_WORKSPACE": "/tmp/ai-sandbox-workspace" }
//	    }
//	  }
//	}
package main

import (
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/namansh70747/ai-agent-sandbox/pkg/mcptools"
	"github.com/namansh70747/ai-agent-sandbox/pkg/sandbox"
)

func main() {
	workspace := os.Getenv("SANDBOX_WORKSPACE")
	if workspace == "" {
		workspace = "/tmp/ai-sandbox-workspace"
	}

	if err := os.MkdirAll(workspace, 0o755); err != nil {
		// Non-fatal: file_tool will fail if workspace doesn't exist,
		// but code_tool / web_tool / database_tool don't need it.
		log.Printf("[mcp] warning: cannot create workspace %s: %v", workspace, err)
	}

	// Logger must write to stderr — stdout is owned by the MCP transport.
	logger := log.New(os.Stderr, "[mcp] ", log.Ltime)

	// SANDBOX_POLICY points at a declarative policy YAML. When set, tools come
	// from it; otherwise the built-in 4-tool registry is used.
	var mgr *sandbox.Manager
	if policyPath := os.Getenv("SANDBOX_POLICY"); policyPath != "" {
		mgr = sandbox.NewManagerFromPolicy(policyPath, workspace, logger)
	} else {
		mgr = sandbox.NewManager(workspace, logger)
	}

	s := server.NewMCPServer(
		"urunc-sandbox",
		"1.0.0",
		server.WithToolCapabilities(false),
	)
	mcptools.RegisterAll(s, mgr)

	logger.Printf("urunc MCP server ready")
	logger.Printf("  workspace = %s", workspace)
	logger.Printf("  tools     = file_tool, code_tool, web_tool, database_tool")

	if err := server.ServeStdio(s); err != nil {
		logger.Fatalf("server error: %v", err)
	}
}
