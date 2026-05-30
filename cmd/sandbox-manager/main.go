// cmd/sandbox-manager/main.go
//
// # AI Agent Per-Tool Sandbox Manager — Demo Entry Point
//
// What this demonstrates
// ──────────────────────
// The original Nubificus blog (https://nubificus.co.uk/blog/urunc_agent/)
// shows running an ENTIRE AI agent inside one urunc microVM.
//
// This project goes one step further: each TOOL the agent uses gets
// its OWN microVM with a permission profile matched to what that tool
// actually needs.  A file tool never gets network access.  A code
// execution tool never sees the host filesystem.  A web tool gets
// network but no mounts.
//
// The result is defence-in-depth:
//
//	compromised tool → escape only that tool's microVM, nothing else.
//
// References used throughout:
//
//	https://urunc.io/installation/
//	https://urunc.io/quickstart/
//	https://urunc.io/design/
//	https://nubificus.co.uk/blog/urunc_agent/
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/namansh70747/ai-agent-sandbox/pkg/sandbox"
	"github.com/namansh70747/ai-agent-sandbox/pkg/tool"
)

func main() {
	workspaceDir := flag.String("workspace", "/tmp/ai-sandbox-workspace", "host directory mounted into file_tool sandboxes")
	demoMode := flag.Bool("demo", false, "run live tool execution (requires urunc installed)")
	profileOnly := flag.Bool("profile", false, "print isolation profiles and exit (no containers needed)")
	verifyOnly := flag.Bool("verify", false, "run urunc quickstart verification and exit")
	policyPath := flag.String("policy", "", "path to declarative policy YAML (e.g. configs/policies.yaml); empty uses built-in defaults")
	flag.Parse()

	logger := log.New(os.Stdout, "", log.Ltime)
	var mgr *sandbox.Manager
	if *policyPath != "" {
		mgr = sandbox.NewManagerFromPolicy(*policyPath, *workspaceDir, logger)
	} else {
		mgr = sandbox.NewManager(*workspaceDir, logger)
	}

	// ── Banner ───────────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("  ╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("  ║   Per-Tool AI Agent Sandboxing with urunc                    ║")
	fmt.Println("  ║   Each tool → its own microVM → its own isolation profile    ║")
	fmt.Println("  ║   Built on: https://urunc.io                                 ║")
	fmt.Println("  ╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// ── Profile-only mode ────────────────────────────────────────────────────
	if *profileOnly {
		mgr.PrintProfile()
		return
	}

	// ── Verify mode ──────────────────────────────────────────────────────────
	if *verifyOnly {
		runVerification()
		return
	}

	// Always print profiles so reviewers can see the design
	mgr.PrintProfile()

	// ── Live demo mode ───────────────────────────────────────────────────────
	if !*demoMode {
		fmt.Println("  ℹ  Run with --demo to execute tools inside urunc microVMs.")
		fmt.Println("     (requires urunc installed; see README.md for setup)")
		fmt.Println()
		return
	}

	// Ensure workspace directory exists
	if err := os.MkdirAll(*workspaceDir, 0o755); err != nil {
		logger.Fatalf("cannot create workspace dir %s: %v", *workspaceDir, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  LIVE DEMO: Executing each tool inside its own urunc microVM")
	fmt.Println("  Runtime: io.containerd.urunc.v2")
	fmt.Println("  Source:  https://nubificus.co.uk/blog/urunc_agent/")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println()

	// ── 1. urunc quickstart verification ─────────────────────────────────────
	// Mirrors the quickstart example from https://urunc.io/quickstart/
	fmt.Println("─── Step 1: urunc quickstart verification ───────────────────────")
	fmt.Println("  Running: nerdctl run -d --runtime io.containerd.urunc.v2")
	fmt.Println("           harbor.nbfc.io/nubificus/urunc/nginx-qemu-unikraft-initrd:latest")
	fmt.Println()

	quickstartCmd := exec.CommandContext(ctx, "nerdctl", "run", "-d",
		"--runtime", "io.containerd.urunc.v2",
		"harbor.nbfc.io/nubificus/urunc/nginx-qemu-unikraft-initrd:latest",
	)
	quickstartCmd.Stdout = os.Stdout
	quickstartCmd.Stderr = os.Stderr
	if err := quickstartCmd.Run(); err != nil {
		fmt.Printf("  ✗ quickstart failed: %v\n", err)
		fmt.Println("    Make sure urunc is installed: sudo bash scripts/00-install-prerequisites.sh")
	} else {
		fmt.Println("  ✓ urunc microVM started successfully (Nginx/Unikraft running)")
	}
	fmt.Println()

	// ── 2. Per-tool demos ─────────────────────────────────────────────────────
	demos := []struct {
		toolType tool.ToolType
		label    string
		cmd      []string
		desc     string
	}{
		{
			toolType: tool.ToolTypeCode,
			label:    "Step 2: code_tool — isolated code execution",
			cmd:      []string{"echo", "hello from isolated code microVM"},
			desc:     "No network. No host mounts. Strongest isolation.",
		},
		{
			toolType: tool.ToolTypeFile,
			label:    "Step 3: file_tool — workspace file access",
			cmd:      []string{"ls", "/workspace"},
			desc:     "No network. Workspace mounted read-write.",
		},
		{
			toolType: tool.ToolTypeWeb,
			label:    "Step 4: web_tool — network allowed, no mounts",
			cmd:      []string{"curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "https://urunc.io"},
			desc:     "Bridge network enabled. No filesystem exposure.",
		},
	}

	for _, d := range demos {
		fmt.Printf("─── %s ───\n", d.label)
		fmt.Printf("  Isolation: %s\n", d.desc)

		result, err := mgr.Execute(ctx, d.toolType, d.cmd)
		if err != nil && result == nil {
			fmt.Printf("  ✗ error: %v\n\n", err)
			continue
		}

		fmt.Printf("  nerdctl command: %s\n", result.DockerCmd)
		fmt.Printf("  Duration: %s\n", result.Duration.Round(time.Millisecond))

		if result.ExitCode == 0 {
			fmt.Printf("  ✓ exit 0\n")
			if result.Stdout != "" {
				fmt.Printf("  stdout: %s\n", result.Stdout)
			}
		} else {
			fmt.Printf("  ✗ exit %d\n", result.ExitCode)
			if result.Stderr != "" {
				fmt.Printf("  stderr: %s\n", result.Stderr)
			}
			if result.Error != nil {
				fmt.Printf("  error:  %v\n", result.Error)
			}
			fmt.Println("  NOTE: If image pull fails, check harbour.nbfc.io connectivity.")
			fmt.Println("        The isolation profile is still applied regardless.")
		}
		fmt.Println()
	}

	// ── Summary ───────────────────────────────────────────────────────────────
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  SUMMARY: Per-Tool Isolation vs. Single-Agent Sandbox")
	fmt.Println()
	fmt.Println("  Original blog (whole agent in one sandbox):")
	fmt.Println("    nerdctl run --runtime io.containerd.urunc.v2 opencode:latest")
	fmt.Println()
	fmt.Println("  This project (per-tool micro-sandboxes):")
	fmt.Println("    file_tool  → microVM: no network, workspace only")
	fmt.Println("    code_tool  → microVM: no network, NO filesystem exposure")
	fmt.Println("    web_tool   → microVM: network, NO filesystem exposure")
	fmt.Println("    db_tool    → microVM: network to DB port, NO filesystem")
	fmt.Println()
	fmt.Println("  Result: one compromised tool cannot reach another tool's data.")
	fmt.Println("  Each microVM is destroyed immediately after the tool returns.")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
}

// runVerification reproduces the quickstart test from https://urunc.io/quickstart/
func runVerification() {
	fmt.Println("── urunc Installation Verification ──────────────────────────────")
	fmt.Println()

	checks := []struct {
		name string
		cmd  []string
	}{
		{"nerdctl info", []string{"nerdctl", "info"}},
		{"urunc binary", []string{"which", "urunc"}},
		{"shim binary", []string{"which", "containerd-shim-urunc-v2"}},
		{"urunc version", []string{"urunc", "--version"}},
	}

	allOK := true
	for _, c := range checks {
		out, err := exec.Command(c.cmd[0], c.cmd[1:]...).Output()
		if err != nil {
			fmt.Printf("  ✗ %-30s  MISSING (%v)\n", c.name, err)
			allOK = false
		} else {
			fmt.Printf("  ✓ %-30s  %s", c.name, out)
			if len(out) == 0 || out[len(out)-1] != '\n' {
				fmt.Println()
			}
		}
	}

	fmt.Println()
	if allOK {
		fmt.Println("  ✓ All checks passed. Run with --demo to test per-tool sandboxing.")
	} else {
		fmt.Println("  ✗ Some checks failed. Follow README.md to complete installation.")
	}
}
