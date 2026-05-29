// pkg/sandbox/manager.go
//
// SandboxManager coordinates per-tool microVM sandbox creation and teardown.
//
// Design: each AI agent tool call → isolated urunc microVM → destroyed on exit.
// References:
//   https://nubificus.co.uk/blog/urunc_agent/ (per-tool sandboxing concept)
//   https://urunc.io/design/                  (container lifecycle)
//   https://urunc.io/installation/            (runtime registration)
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/namansh70747/ai-agent-sandbox/pkg/tool"
)

// ExecResult holds the full outcome of one tool invocation inside a microVM.
type ExecResult struct {
	ToolName  string
	ToolType  tool.ToolType
	Command   []string
	DockerCmd string        // auditable nerdctl run line
	Stdout    string
	Stderr    string
	ExitCode  int
	Duration  time.Duration
	Error     error
}

// Manager coordinates per-tool sandbox creation, execution, and teardown.
type Manager struct {
	registry *tool.Registry
	spawner  *Spawner
	logger   *log.Logger
}

// NewManager creates a Manager with workspaceDir bound to file_tool sandboxes.
func NewManager(workspaceDir string, logger *log.Logger) *Manager {
	return &Manager{
		registry: tool.NewRegistry(workspaceDir),
		spawner:  NewSpawner(),
		logger:   logger,
	}
}

// Execute runs cmd inside a dedicated urunc microVM for the given toolType.
//
// Per-tool isolation:
//   file_tool     → --network=none  + workspace mount
//   code_tool     → --network=none  + no mounts  (strongest)
//   web_tool      → bridge network  + no mounts
//   database_tool → bridge network  + no mounts
//
// The microVM is created, used, and destroyed in one nerdctl run --rm call.
// Reference: https://urunc.io/quickstart/ (nerdctl run --runtime io.containerd.urunc.v2)
func (m *Manager) Execute(ctx context.Context, toolType tool.ToolType, cmd []string) (*ExecResult, error) {
	def, err := m.registry.Get(toolType)
	if err != nil {
		return nil, err
	}

	dockerCmd := m.spawner.CommandString(def, cmd)
	m.logger.Printf("[sandbox] spawning  tool=%-15s  isolation=%q", def.Name, def.Profile.Network)
	m.logger.Printf("[sandbox] command:  %s", dockerCmd)

	execCmd := m.spawner.BuildCommand(def, cmd)
	// Attach context so the caller can cancel if needed
	ctxCmd := exec.CommandContext(ctx, execCmd.Args[0], execCmd.Args[1:]...)

	var stdout, stderr bytes.Buffer
	ctxCmd.Stdout = &stdout
	ctxCmd.Stderr = &stderr

	start := time.Now()
	runErr := ctxCmd.Run()
	elapsed := time.Since(start)

	exitCode := 0
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}

	result := &ExecResult{
		ToolName:  def.Name,
		ToolType:  toolType,
		Command:   cmd,
		DockerCmd: dockerCmd,
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		ExitCode:  exitCode,
		Duration:  elapsed,
		Error:     runErr,
	}

	m.logger.Printf("[sandbox] finished  tool=%-15s  exit=%d  duration=%s",
		def.Name, exitCode, elapsed.Round(time.Millisecond))
	return result, nil
}

// PrintProfile prints the isolation profile for every registered tool.
// No containers are started; this works without urunc installed.
func (m *Manager) PrintProfile() {
	fmt.Println("┌────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  Per-Tool Isolation Profiles                                       │")
	fmt.Println("│  Runtime: io.containerd.urunc.v2  (urunc microVM per tool call)    │")
	fmt.Println("│  Source:  https://nubificus.co.uk/blog/urunc_agent/                │")
	fmt.Println("└────────────────────────────────────────────────────────────────────┘")
	fmt.Println()

	for _, tt := range []tool.ToolType{
		tool.ToolTypeFile, tool.ToolTypeCode, tool.ToolTypeWeb, tool.ToolTypeDatabase,
	} {
		def, _ := m.registry.Get(tt)
		p := def.Profile

		fmt.Printf("  ▶ %-16s\n", def.Name)
		fmt.Printf("    Memory : %dMB   CPUs: %.1f\n", p.MemoryMB, p.CPUCount)
		fmt.Printf("    Network: %s\n", p.Network)

		if len(p.Mounts) == 0 {
			fmt.Printf("    Mounts : none  (host filesystem NOT exposed)\n")
		} else {
			for _, mv := range p.Mounts {
				ro := ""
				if mv.ReadOnly {
					ro = " :ro"
				}
				fmt.Printf("    Mounts : %s → %s%s\n", mv.HostPath, mv.ContainerPath, ro)
			}
		}

		fmt.Printf("    Why    : %s\n", p.Rationale)
		fmt.Printf("    nerdctl : %s\n", m.spawner.CommandString(def, []string{"<cmd>"}))
		fmt.Println()
	}
}
