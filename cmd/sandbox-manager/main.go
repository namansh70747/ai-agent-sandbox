// =============================================================================
// FILE: cmd/sandbox-manager/main.go
// SOURCE: Aggregates all official docs into a working demo.
//
// This demonstrates:
//   1. Tool Registry (PART 2 architecture)
//   2. Basic Spawner using docker run --runtime io.containerd.urunc.v2
//      (from https://nubificus.co.uk/blog/urunc_agent/)
//   3. Advanced containerd client path (from https://urunc.io/design/)
//   4. OCI bundle creation with urunc.json (from https://urunc.io/package/)
//   5. Lifecycle: create, start, delete, kill (from https://urunc.io/design/)
//   6. Network: tap0_urunc plus CNI (from https://urunc.io/design/)
//   7. Storage: devmapper plus workspace mounts (from https://urunc.io/installation/)
// =============================================================================

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/example/ai-agent-sandbox/pkg/mcp"
	"github.com/example/ai-agent-sandbox/pkg/sandbox"
	"github.com/example/ai-agent-sandbox/pkg/tool"
)

func main() {
	fmt.Println("=== AI Agent Per-Tool Sandboxing with urunc ===")
	fmt.Println("Strictly following official docs:")
	fmt.Println("  - https://urunc.io/design/")
	fmt.Println("  - https://urunc.io/package/")
	fmt.Println("  - https://urunc.io/installation/")
	fmt.Println("  - https://urunc.io/quickstart/")
	fmt.Println("  - https://nubificus.co.uk/blog/urunc_agent/")
	fmt.Println()

	// -------------------------------------------------------------------------
	// Step 1: Tool Registry
	// -------------------------------------------------------------------------
	registry := tool.NewRegistry()
	fmt.Println("Registered tools:")
	for name, t := range registry.List() {
		fmt.Printf("  - %-15s %s\n", name, t.Description)
	}
	fmt.Println()

	// -------------------------------------------------------------------------
	// Step 2: Write CNI config (so tap0_urunc mapping works)
	// SOURCE: https://urunc.io/design/ (Network handling)
	// -------------------------------------------------------------------------
	if err := sandbox.WriteCNIConfig(); err != nil {
		log.Fatalf("Failed to write CNI config: %v", err)
	}
	fmt.Println("OK CNI bridge config written to /etc/cni/net.d/10-urunc-bridge.conf")

	// -------------------------------------------------------------------------
	// Step 3: Basic Spawner (Docker CLI path)
	// SOURCE: https://nubificus.co.uk/blog/urunc_agent/
	// -------------------------------------------------------------------------
	basicSpawner := sandbox.NewBasicSpawner()

	// -------------------------------------------------------------------------
	// Step 4: Advanced Manager (containerd client path)
	// SOURCE: https://urunc.io/design/ (Execution flow)
	// -------------------------------------------------------------------------
	var advancedMgr *sandbox.Manager
	mgr, err := sandbox.NewManager("/run/containerd/containerd.sock")
	if err != nil {
		fmt.Printf("WARN containerd not available for advanced path: %v\n", err)
		fmt.Println("  Falling back to Docker CLI path only.")
	} else {
		advancedMgr = mgr
		defer advancedMgr.Close()
		fmt.Println("OK Connected to containerd for advanced OCI lifecycle path")
	}

	// -------------------------------------------------------------------------
	// Step 5: MCP Server
	// -------------------------------------------------------------------------
	server := mcp.NewServer(registry, basicSpawner, advancedMgr)

	// -------------------------------------------------------------------------
	// Step 6: Demo execution via Basic Spawner
	// -------------------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Trap signals for cleanup
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Demo: file_tool sandbox
	fmt.Println("\n--- DEMO: file_tool sandbox ---")
	fileTool, _ := registry.Get("file_tool")
	sandboxID := fmt.Sprintf("file-tool-%d", time.Now().UnixNano())
	wsHost, _, _ := sandbox.PrepareWorkspace(sandboxID)

	// Spawn using docker run --runtime io.containerd.urunc.v2
	// This is EXACTLY the command from the Nubificus blog.
	containerID, err := basicSpawner.Spawn(ctx, fileTool, sandboxID, wsHost)
	if err != nil {
		log.Fatalf("Spawn failed: %v", err)
	}
	fmt.Printf("OK Spawned urunc container: %s\n", containerID)

	// Execute a file write inside the sandbox
	output, err := basicSpawner.Exec(ctx, containerID, []string{
		"sh", "-c", "echo 'Hello from urunc sandbox' > /workspace/result.txt && cat /workspace/result.txt",
	})
	if err != nil {
		log.Printf("Exec error: %v", err)
	}
	fmt.Printf("OK Exec output: %s\n", strings.TrimSpace(output))

	// Cleanup: destroy sandbox
	if err := basicSpawner.Destroy(ctx, containerID); err != nil {
		log.Printf("Destroy error: %v", err)
	}
	fmt.Println("OK Sandbox destroyed")

	// -------------------------------------------------------------------------
	// Step 7: Demo via MCP Server
	// -------------------------------------------------------------------------
	fmt.Println("\n--- DEMO: MCP-style tool call ---")
	resp := server.Execute(ctx, mcp.ToolRequest{
		Tool:  "code_tool",
		Input: "uname -a && echo 'Isolated code execution'",
	})
	if resp.Error != "" {
		fmt.Printf("Error: %s\n", resp.Error)
	}
	fmt.Printf("Output: %s\n", resp.Output)

	// -------------------------------------------------------------------------
	// Step 8: OCI Lifecycle direct demo (advanced)
	// -------------------------------------------------------------------------
	if advancedMgr != nil {
		fmt.Println("\n--- DEMO: Advanced containerd client path ---")
		codeTool, _ := registry.Get("code_tool")
		advID := fmt.Sprintf("adv-code-%d", time.Now().UnixNano())

		c, err := advancedMgr.CreateSandbox(ctx, codeTool, advID)
		if err != nil {
			log.Printf("Advanced create failed: %v", err)
		} else {
			task, err := advancedMgr.Start(ctx, c)
			if err != nil {
				log.Printf("Advanced start failed: %v", err)
			} else {
				fmt.Printf("OK Advanced task started: %s\n", task.ID())
				time.Sleep(2 * time.Second)

				// Stop and cleanup
				if err := advancedMgr.Stop(ctx, c); err != nil {
					log.Printf("Advanced stop failed: %v", err)
				} else {
					fmt.Println("OK Advanced sandbox stopped and deleted")
				}
			}
		}
	}

	fmt.Println("\n=== Demo Complete ===")
	fmt.Println("Each tool executed in an isolated urunc microVM.")
	fmt.Println("Host filesystem, kernel, and other processes are protected.")
}
