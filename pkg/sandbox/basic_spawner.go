// =============================================================================
// FILE: pkg/sandbox/basic_spawner.go
// SOURCE: https://nubificus.co.uk/blog/urunc_agent/ (Step 2 and 3)
// SOURCE: https://urunc.io/quickstart/ (Docker example)
//
// DOCS STATE (Blog):
//   "sudo docker run --runtime=io.containerd.urunc.v2 -v ${PWD}/mydir:/mydir opencode:latest"
//
// DOCS STATE (Quickstart):
//   "docker run --rm -d --runtime io.containerd.urunc.v2 harbor.nbfc.io/..."
//
// This is the BASIC path: using Docker or nerdctl CLI with the urunc runtime.
// It is the fastest way to get per-tool sandboxing working.
// =============================================================================

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/example/ai-agent-sandbox/pkg/tool"
)

// BasicSpawner uses docker or nerdctl CLI to spawn urunc containers.
type BasicSpawner struct {
	runtime string // io.containerd.urunc.v2
}

func NewBasicSpawner() *BasicSpawner {
	return &BasicSpawner{runtime: "io.containerd.urunc.v2"}
}

// Spawn creates and starts a urunc container for the given tool.
// It strictly follows the docker run semantics from the official blog.
func (s *BasicSpawner) Spawn(ctx context.Context, t tool.Tool, sandboxID string, workspaceHostPath string) (string, error) {
	// Build docker run command exactly as documented:
	// docker run -m 1024M --rm --runtime io.containerd.urunc.v2 -v ... -it image:tag
	args := []string{
		"run", "-d", "--rm",
		"-m", "1024M", // urunc passes memory limit to VMM
		"--runtime", s.runtime,
		"--name", sandboxID,
	}

	// Filesystem restriction: mount only workspace if allowed
	if t.Cap.CanReadFS || t.Cap.CanWriteFS {
		if len(t.Cap.AllowedFSPaths) > 0 {
			for _, p := range t.Cap.AllowedFSPaths {
				// SOURCE: Blog Step 3: "-v ${PWD}/mydir:/mydir"
				// SECURITY WARNING from blog: shared data is no longer protected.
				src := workspaceHostPath
				dst := p
				args = append(args, "-v", fmt.Sprintf("%s:%s", src, dst))
			}
		}
	} else {
		// No filesystem access: use read-only rootfs
		args = append(args, "--read-only")
	}

	// Network restriction: if no network, use --network none
	if !t.Cap.CanNetwork {
		args = append(args, "--network", "none")
	} else {
		// Default bridge; domain and port filtering requires additional CNI firewall
		// which we configure separately via NetworkPolicy.
		args = append(args, "--network", "bridge")
	}

	// Image and default command (urunit will receive this as cmdline)
	args = append(args, t.ImageRef)
	args = append(args, strings.Fields(t.Command)...)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run failed: %w\noutput: %s", err, out)
	}

	containerID := strings.TrimSpace(string(out))
	return containerID, nil
}

// Exec runs a command inside a running urunc container via docker exec.
func (s *BasicSpawner) Exec(ctx context.Context, containerID string, command []string) (string, error) {
	args := append([]string{"exec", containerID}, command...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Destroy kills and removes the container.
func (s *BasicSpawner) Destroy(ctx context.Context, containerID string) error {
	// Force kill then rm
	_ = exec.CommandContext(ctx, "docker", "kill", containerID).Run()
	time.Sleep(1 * time.Second)
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker rm failed: %w\noutput: %s", err, out)
	}
	return nil
}
