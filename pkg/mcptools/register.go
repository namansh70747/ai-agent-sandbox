// pkg/mcptools/register.go
//
// Shared MCP tool registration used by both cmd/mcp-server and cmd/api-server.
// Each of the 4 tool types is registered as an MCP tool with a simple
// {"command": ["cmd","arg1",...]} input schema.
//
// Isolation is enforced at the sandbox.Manager layer — from the MCP client's
// perspective, tools just return text; the microVM lifecycle is invisible.
package mcptools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/namansh70747/ai-agent-sandbox/pkg/sandbox"
	"github.com/namansh70747/ai-agent-sandbox/pkg/tool"
)

// RegisterAll registers every tool in the manager's registry as an MCP tool.
// Tools come from the declarative policy (or built-in defaults), so new tools
// appear over MCP automatically with no code change here.
// Input schema for every tool: {"command": ["cmd","arg1",...]}
// Output: plain text with exit_code, stdout, stderr, and the nerdctl command used.
func RegisterAll(s *server.MCPServer, mgr *sandbox.Manager) {
	for _, def := range mgr.Registry().All() {
		add(s, mgr, def.Type, def.Name, describe(def))
	}
}

// describe builds the MCP tool description from the policy fields, with a
// concise isolation summary appended so the agent understands the sandbox.
func describe(def *tool.ToolDef) string {
	desc := def.Profile.Description
	if desc == "" {
		desc = def.Profile.Rationale
	}
	if desc == "" {
		desc = "Run a command inside an isolated urunc microVM."
	}

	p := def.Profile
	net := string(p.Network)
	if net == "" {
		net = "none"
	}
	mounts := "no filesystem mounts"
	if len(p.Mounts) > 0 {
		mounts = fmt.Sprintf("%d mount(s)", len(p.Mounts))
	}
	return fmt.Sprintf("%s [isolation: network=%s, %s]", desc, net, mounts)
}

func add(s *server.MCPServer, mgr *sandbox.Manager, tt tool.ToolType, name, desc string) {
	t := mcp.NewTool(name,
		mcp.WithDescription(desc),
		mcp.WithArray("command",
			mcp.Required(),
			mcp.Description(`Command and arguments as a string array, e.g. ["echo","hello"]`),
			mcp.Items(map[string]any{"type": "string"}),
		),
	)

	// capturedType captures the loop value — without this, all 4 closures
	// would reference the same variable and all execute as database_tool.
	capturedType := tt

	s.AddTool(t, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// req.Params.Arguments is typed as `any` in mcp-go v0.32; type-assert before indexing.
		argsMap, ok := req.Params.Arguments.(map[string]any)
		if !ok {
			return mcp.NewToolResultError("invalid arguments: expected object"), nil
		}
		raw, ok := argsMap["command"].([]any)
		if !ok || len(raw) == 0 {
			return mcp.NewToolResultError("'command' must be a non-empty array of strings"), nil
		}
		cmd := make([]string, len(raw))
		for i, v := range raw {
			cmd[i] = fmt.Sprint(v)
		}

		result, err := mgr.Execute(ctx, capturedType, cmd)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("sandbox error: %v", err)), nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "tool:         %s\n", result.ToolName)
		fmt.Fprintf(&sb, "exit_code:    %d\n", result.ExitCode)
		fmt.Fprintf(&sb, "duration_ms:  %d\n", result.Duration.Milliseconds())
		if result.BootTelemetry != nil && result.BootTelemetry.Attributed {
			fmt.Fprintf(&sb, "boot_time_ms: %d\n", result.BootTelemetry.BootTimeMs)
		}
		fmt.Fprintf(&sb, "nerdctl_cmd:  %s\n", result.DockerCmd)
		if result.Stdout != "" {
			fmt.Fprintf(&sb, "stdout:\n%s\n", strings.TrimRight(result.Stdout, "\n"))
		}
		if result.Stderr != "" {
			fmt.Fprintf(&sb, "stderr:\n%s\n", strings.TrimRight(result.Stderr, "\n"))
		}
		if result.Error != nil && result.ExitCode != 0 {
			fmt.Fprintf(&sb, "error: %v\n", result.Error)
		}

		return mcp.NewToolResultText(sb.String()), nil
	})
}
