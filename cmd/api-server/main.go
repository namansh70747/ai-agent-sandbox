// cmd/api-server/main.go
//
// HTTP server that exposes the urunc sandbox platform over two interfaces
// from a single port:
//
//  1. REST API — works with any AI agent that can make HTTP calls:
//     GET  /api/v1/tools            list all tools and their isolation profiles
//     POST /api/v1/execute          run a command in a sandboxed microVM
//
//  2. MCP over SSE — for remote MCP-compatible agents:
//     GET  /mcp/sse                 SSE event stream (MCP transport)
//     POST /mcp/message             MCP message endpoint
//
// Lima port-forwarding makes :8080 inside the VM reachable at
// http://localhost:8080 from macOS without any extra configuration.
//
// Example (any language, any agent):
//
//	curl -X POST http://localhost:8080/api/v1/execute \
//	  -H "Content-Type: application/json" \
//	  -d '{"tool_type":"code_tool","command":["echo","hello"]}'
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/namansh70747/ai-agent-sandbox/pkg/mcptools"
	"github.com/namansh70747/ai-agent-sandbox/pkg/sandbox"
	"github.com/namansh70747/ai-agent-sandbox/pkg/tool"
)

// executeRequest is the body for POST /api/v1/execute.
type executeRequest struct {
	ToolType string   `json:"tool_type"` // "file_tool" | "code_tool" | "web_tool" | "database_tool"
	Command  []string `json:"command"`   // e.g. ["echo", "hello"]
}

// executeResponse mirrors sandbox.ExecResult for JSON serialisation.
type executeResponse struct {
	ToolName   string   `json:"tool_name"`
	ToolType   string   `json:"tool_type"`
	Command    []string `json:"command"`
	NerdctlCmd string   `json:"nerdctl_cmd"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr"`
	ExitCode   int      `json:"exit_code"`
	DurationMs int64    `json:"duration_ms"`
	Error      string   `json:"error,omitempty"`
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	workspace := flag.String("workspace", "/tmp/ai-sandbox-workspace", "workspace dir for file_tool")
	flag.Parse()

	if err := os.MkdirAll(*workspace, 0o755); err != nil {
		log.Printf("[api] warning: cannot create workspace %s: %v", *workspace, err)
	}

	logger := log.New(os.Stdout, "[api] ", log.Ltime)
	mgr := sandbox.NewManager(*workspace, logger)

	// ── MCP server (shared between SSE transport and the REST path) ───────────
	mcpSrv := server.NewMCPServer("urunc-sandbox", "1.0.0",
		server.WithToolCapabilities(false),
	)
	mcptools.RegisterAll(mcpSrv, mgr)

	// ── SSE transport — mcp-go SSEServer implements http.Handler ─────────────
	// WithBaseURL tells the SSE server what URL to embed in the `endpoint` event
	// so connecting clients get a fully-qualified message URL.
	// WithBaseURL must include the /mcp prefix so the SSE server generates
	// the correct message endpoint URL: http://localhost:8080/mcp/message
	// Without it clients are told to POST to /message which is not registered.
	sseSrv := server.NewSSEServer(mcpSrv,
		server.WithBaseURL(fmt.Sprintf("http://localhost%s/mcp", *addr)),
	)

	// ── HTTP mux ──────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// MCP over SSE — delegate /mcp/ prefix to the SSE server.
	// The SSE server routes /sse and /message internally.
	mux.Handle("/mcp/", http.StripPrefix("/mcp", sseSrv))

	// REST: list isolation profiles — no urunc needed, safe to call anytime
	mux.HandleFunc("/api/v1/tools", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(mgr.ProfileSummary()); err != nil {
			logger.Printf("encode error: %v", err)
		}
	})

	// REST: execute a tool — the universal sandboxed execution endpoint
	mux.HandleFunc("/api/v1/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}

		var req executeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(req.Command) == 0 {
			http.Error(w, "'command' must be a non-empty array", http.StatusBadRequest)
			return
		}
		if req.ToolType == "" {
			http.Error(w, "'tool_type' is required (file_tool|code_tool|web_tool|database_tool)", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		result, err := mgr.Execute(ctx, tool.ToolType(req.ToolType), req.Command)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp := executeResponse{
			ToolName:   result.ToolName,
			ToolType:   string(result.ToolType),
			Command:    result.Command,
			NerdctlCmd: result.DockerCmd,
			Stdout:     result.Stdout,
			Stderr:     result.Stderr,
			ExitCode:   result.ExitCode,
			DurationMs: result.Duration.Milliseconds(),
		}
		if result.Error != nil {
			resp.Error = result.Error.Error()
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Printf("encode error: %v", err)
		}
	})

	// Root: human-readable index so anyone hitting the server knows what's here
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w,
			"urunc AI Agent Sandbox Platform\n"+
				"Runtime: io.containerd.urunc.v2 (one microVM per tool call)\n\n"+
				"REST (any AI agent):\n"+
				"  GET  /api/v1/tools            isolation profiles for all 4 tools\n"+
				"  POST /api/v1/execute          {\"tool_type\":\"...\",\"command\":[...]}\n\n"+
				"MCP over SSE (remote MCP clients):\n"+
				"  GET  /mcp/sse                 SSE event stream\n"+
				"  POST /mcp/message             MCP message endpoint\n\n"+
				"MCP over stdio (local agents):\n"+
				"  run  ./bin/mcp-server         subprocess transport (OpenCode, Cursor, etc.)\n\n"+
				"Source: https://nubificus.co.uk/blog/urunc_agent/\n",
		)
	})

	logger.Printf("urunc sandbox platform listening on %s", *addr)
	logger.Printf("  REST tools:    http://localhost%s/api/v1/tools", *addr)
	logger.Printf("  REST execute:  http://localhost%s/api/v1/execute", *addr)
	logger.Printf("  MCP SSE:       http://localhost%s/mcp/sse", *addr)

	if err := http.ListenAndServe(*addr, mux); err != nil {
		logger.Fatalf("server: %v", err)
	}
}
