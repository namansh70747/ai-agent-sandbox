// pkg/sandbox/manager.go
//
// SandboxManager coordinates per-tool microVM sandbox creation and teardown.
//
// Design: each AI agent tool call → isolated urunc microVM → destroyed on exit.
// References:
//
//	https://nubificus.co.uk/blog/urunc_agent/ (per-tool sandboxing concept)
//	https://urunc.io/design/                  (container lifecycle)
//	https://urunc.io/installation/            (runtime registration)
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/namansh70747/ai-agent-sandbox/pkg/policy"
	"github.com/namansh70747/ai-agent-sandbox/pkg/tool"
)

// ExecResult holds the full outcome of one tool invocation inside a microVM.
type ExecResult struct {
	ToolName  string
	ToolType  tool.ToolType
	Command   []string
	DockerCmd string // auditable nerdctl run line
	Stdout    string
	Stderr    string
	ExitCode  int
	Duration  time.Duration
	Error     error

	// BootTelemetry is urunc's own VM boot timing for this run, when the
	// [timestamps] feature is enabled in /etc/urunc/config.toml. nil otherwise.
	BootTelemetry *BootTelemetry
}

// Manager coordinates per-tool sandbox creation, execution, and teardown.
type Manager struct {
	registry *tool.Registry
	spawner  *Spawner
	logger   *log.Logger

	// telemetryMu serialises the snapshot→run→read window ONLY when urunc
	// boot telemetry is active, so boot times can be attributed to the right
	// run (all VMs share one timestamps.log). When telemetry is off, it is
	// never taken and executions run fully concurrently.
	telemetryMu sync.Mutex
}

// NewManager creates a Manager with workspaceDir bound to file_tool sandboxes,
// using the built-in 4-tool registry.
func NewManager(workspaceDir string, logger *log.Logger) *Manager {
	return &Manager{
		registry: tool.NewRegistry(workspaceDir),
		spawner:  NewSpawner(),
		logger:   logger,
	}
}

// NewManagerFromPolicy builds a Manager from a declarative YAML policy file.
// On any load error it logs the reason and falls back to the built-in
// 4-tool registry, so the platform always starts.
func NewManagerFromPolicy(policyPath, workspaceDir string, logger *log.Logger) *Manager {
	reg, err := policy.LoadInto(policyPath, workspaceDir)
	if err != nil {
		logger.Printf("[sandbox] policy load failed (%v); using built-in defaults", err)
		reg = tool.NewRegistry(workspaceDir)
	} else {
		logger.Printf("[sandbox] loaded %d tools from %s", len(reg.All()), policyPath)
	}
	return &Manager{
		registry: reg,
		spawner:  NewSpawner(),
		logger:   logger,
	}
}

// Registry exposes the underlying tool registry (used by the MCP layer to
// register every policy-defined tool dynamically).
func (m *Manager) Registry() *tool.Registry { return m.registry }

// Execute runs cmd inside a dedicated urunc microVM for the given toolType.
//
// Per-tool isolation:
//
//	file_tool     → --network=none  + workspace mount
//	code_tool     → --network=none  + no mounts  (strongest)
//	web_tool      → bridge network  + no mounts
//	database_tool → bridge network  + no mounts
//
// The microVM is created, used, and destroyed in one nerdctl run --rm call.
// Reference: https://urunc.io/quickstart/ (nerdctl run --runtime io.containerd.urunc.v2)
func (m *Manager) Execute(ctx context.Context, toolType tool.ToolType, cmd []string) (*ExecResult, error) {
	def, err := m.registry.Get(toolType)
	if err != nil {
		return nil, err
	}

	// Per-tool timeout: if the policy sets one, cap this execution.
	if def.Profile.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(def.Profile.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	// Honesty: an egress allowlist is declared policy, not enforced by urunc.
	if len(def.Profile.EgressAllowlist) > 0 && def.Profile.Network == tool.NetworkBridge {
		m.logger.Printf("[sandbox] NOTE tool=%s declares egress_allowlist but bridge grants full egress (not enforced)", def.Name)
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

	// Boot telemetry: only when urunc's timestamps log is active. We serialise
	// the snapshot→run→read window so the appended log bytes can be attributed
	// to this run (all VMs share one log). When telemetry is off, no lock is
	// taken and executions run fully concurrently.
	telem := telemetryEnabled()
	var preSize int64 = -1
	if telem {
		m.telemetryMu.Lock()
		defer m.telemetryMu.Unlock()
		preSize = snapshotTimestampsSize()
	}

	start := time.Now()
	runErr := ctxCmd.Run()
	elapsed := time.Since(start)

	var bootTelemetry *BootTelemetry
	if telem {
		bootTelemetry = readBootTelemetry(preSize, elapsed)
	}

	exitCode := 0
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			// context cancelled, signal killed, binary not found, etc.
			exitCode = -1
		}
	}

	result := &ExecResult{
		ToolName:      def.Name,
		ToolType:      toolType,
		Command:       cmd,
		DockerCmd:     dockerCmd,
		Stdout:        stdout.String(),
		Stderr:        stderr.String(),
		ExitCode:      exitCode,
		Duration:      elapsed,
		Error:         runErr,
		BootTelemetry: bootTelemetry,
	}

	m.logger.Printf("[sandbox] finished  tool=%-15s  exit=%d  duration=%s",
		def.Name, exitCode, elapsed.Round(time.Millisecond))
	return result, nil
}

// ProfileSummary returns all tool isolation profiles as JSON-friendly maps.
// Used by the GET /api/v1/tools REST endpoint so any agent can discover
// what sandboxing is applied to each tool before calling it.
func (m *Manager) ProfileSummary() []map[string]any {
	defs := m.registry.All()
	out := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		p := def.Profile
		mounts := make([]string, 0, len(p.Mounts))
		for _, mv := range p.Mounts {
			s := mv.HostPath + ":" + mv.ContainerPath
			if mv.ReadOnly {
				s += ":ro"
			}
			mounts = append(mounts, s)
		}
		monitor := string(p.Monitor)
		if monitor == "" {
			monitor = "qemu" // image default
		}
		seccomp := string(p.Seccomp)
		if seccomp == "" {
			seccomp = "default"
		}
		out = append(out, map[string]any{
			"name":             def.Name,
			"type":             string(def.Type),
			"category":         p.Category,
			"description":      p.Description,
			"monitor":          monitor,
			"unikernel_type":   string(p.UnikernelType),
			"memory_mb":        p.MemoryMB,
			"cpus":             p.CPUCount,
			"network":          string(p.Network),
			"mounts":           mounts,
			"seccomp":          seccomp,
			"read_only_rootfs": p.ReadOnlyRootfs,
			"timeout_seconds":  p.TimeoutSeconds,
			"egress_allowlist": p.EgressAllowlist,
			"egress_enforced":  false, // honest: never enforced today
			"rationale":        p.Rationale,
		})
	}
	return out
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

	for _, def := range m.registry.All() {
		p := def.Profile

		monitor := string(p.Monitor)
		if monitor == "" {
			monitor = "qemu"
		}

		fmt.Printf("  ▶ %-16s [%s]\n", def.Name, p.Category)
		fmt.Printf("    Monitor: %-12s Memory: %dMB   CPUs: %.1f\n", monitor, p.MemoryMB, p.CPUCount)
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
