// =============================================================================
// FILE: pkg/mcp/server.go
// SOURCE: Original architecture (PART 2: MCP Server Integration)
// IMPLEMENTATION: Custom MCP-to-urunc bridge.
//
// This maps MCP tool calls to sandboxed executions. It is OPTIONAL per the
// architecture diagram but included here for completeness.
// =============================================================================

package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/example/ai-agent-sandbox/pkg/sandbox"
	"github.com/example/ai-agent-sandbox/pkg/tool"
)

// Server bridges MCP tool requests to urunc sandboxes.
type Server struct {
	registry *tool.Registry
	spawner  *sandbox.BasicSpawner
	// advanced manager for containerd client path
	manager *sandbox.Manager
}

func NewServer(reg *tool.Registry, spawner *sandbox.BasicSpawner, mgr *sandbox.Manager) *Server {
	return &Server{
		registry: reg,
		spawner:  spawner,
		manager:  mgr,
	}
}

// ToolRequest is a generic MCP-style tool invocation.
type ToolRequest struct {
	Tool    string            `json:"tool"`
	Input   string            `json:"input"`
	Options map[string]string `json:"options,omitempty"`
}

// ToolResponse returns the result.
type ToolResponse struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// Execute routes the tool request to the correct sandbox.
func (s *Server) Execute(ctx context.Context, req ToolRequest) ToolResponse {
	t, err := s.registry.Get(req.Tool)
	if err != nil {
		return ToolResponse{Error: err.Error()}
	}

	// Use basic spawner for simplicity (docker run --runtime io.containerd.urunc.v2)
	sandboxID := fmt.Sprintf("%s-%d", t.Name, time.Now().UnixNano())
	wsHost, _, _ := sandbox.PrepareWorkspace(sandboxID)

	containerID, err := s.spawner.Spawn(ctx, t, sandboxID, wsHost)
	if err != nil {
		return ToolResponse{Error: fmt.Sprintf("spawn failed: %v", err)}
	}
	defer s.spawner.Destroy(ctx, containerID)

	// Execute the tool input inside the sandbox
	var execCmd []string
	switch t.Name {
	case "file_tool":
		// Example: write a file
		execCmd = []string{"sh", "-c", fmt.Sprintf("echo '%s' > /workspace/output.txt && cat /workspace/output.txt", req.Input)}
	case "code_tool":
		execCmd = []string{"sh", "-c", req.Input}
	case "web_tool":
		execCmd = []string{"curl", "-s", "-L", req.Input}
	case "database_tool":
		execCmd = []string{"sh", "-c", fmt.Sprintf("psql %s -c '\\dt'", req.Input)}
	default:
		execCmd = strings.Fields(req.Input)
	}

	output, err := s.spawner.Exec(ctx, containerID, execCmd)
	if err != nil {
		return ToolResponse{Error: fmt.Sprintf("exec failed: %v", err), Output: output}
	}

	return ToolResponse{Output: strings.TrimSpace(output)}
}
