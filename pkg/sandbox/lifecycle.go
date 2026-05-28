// =============================================================================
// FILE: pkg/sandbox/lifecycle.go
// SOURCE: https://urunc.io/design/ (Execution flow / Lifecycle)
// SOURCE: https://github.com/urunc-dev/urunc (cmd/urunc/create.go, start.go, delete.go, kill.go)
//
// DOCS STATE (Execution flow):
//   1. containerd unpacks image into snapshotter and invokes urunc.
//   2. urunc parses rootfs and annotations; creates stdio pipes, state file,
//      runs prestart hooks.
//   3. urunc spawns new process in network namespace, stores PID, invokes
//      createRuntime and createContainer hooks.
//   4. containerd start: urunc configures block devices and network interfaces,
//      runs startContainer hooks.
//   5. urunc selects VMM and boots unikernel.
//
// DOCS STATE (Lifecycle commands):
//   urunc create <container-id> <path-to-bundle>
//   urunc start <container-id>
//   urunc kill <container-id> <signal>
//   urunc delete <container-id>
//   urunc state <container-id>
//
// NOTE: In practice, containerd-shim-urunc-v2 invokes these. We wrap them
// for direct debugging or use containerd Go client for production.
// =============================================================================

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// OCILifecycle provides direct OCI runtime command wrappers for urunc.
// This is useful for debugging; production code uses containerd client.
type OCILifecycle struct {
	RuntimePath string // /usr/local/bin/urunc
	RootPath    string // /var/run/ai-sandbox
}

func NewOCILifecycle() *OCILifecycle {
	return &OCILifecycle{
		RuntimePath: "/usr/local/bin/urunc",
		RootPath:    "/var/run/ai-sandbox",
	}
}

// Create executes `urunc create` with the bundle.
func (l *OCILifecycle) Create(id, bundlePath string) error {
	cmd := exec.Command(l.RuntimePath, "create", id, "--bundle", bundlePath)
	cmd.Env = append(os.Environ(), "PATH=/usr/local/bin:/usr/bin:/bin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("urunc create failed: %w\noutput: %s", err, out)
	}
	return nil
}

// Start executes `urunc start`.
func (l *OCILifecycle) Start(id string) error {
	cmd := exec.Command(l.RuntimePath, "start", id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("urunc start failed: %w\noutput: %s", err, out)
	}
	return nil
}

// Kill sends signal to the container (default SIGTERM).
func (l *OCILifecycle) Kill(id string, sig syscall.Signal) error {
	if sig == 0 {
		sig = syscall.SIGTERM
	}
	cmd := exec.Command(l.RuntimePath, "kill", id, fmt.Sprintf("%d", sig))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("urunc kill failed: %w\noutput: %s", err, out)
	}
	return nil
}

// Delete removes the container state.
func (l *OCILifecycle) Delete(id string) error {
	cmd := exec.Command(l.RuntimePath, "delete", id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("urunc delete failed: %w\noutput: %s", err, out)
	}
	return nil
}

// State returns container state JSON.
func (l *OCILifecycle) State(id string) ([]byte, error) {
	cmd := exec.Command(l.RuntimePath, "state", id)
	return cmd.CombinedOutput()
}

// WaitForExit polls until the container stops.
func (l *OCILifecycle) WaitForExit(ctx context.Context, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_, err := l.State(id)
		if err != nil {
			// State fails when container is deleted or stopped
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for container %s to exit", id)
}
