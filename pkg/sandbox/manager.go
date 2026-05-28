// =============================================================================
// FILE: pkg/sandbox/manager.go
// SOURCE: https://urunc.io/design/ (Execution flow)
// SOURCE: https://github.com/urunc-dev/urunc (containerd-shim integration)
//
// DOCS STATE:
//   "containerd invokes urunc with the bundle and storage backend.
//    urunc parses the bundle. urunc constructs the appropriate command-line
//    parameters for the respective hypervisor and spawns the unikernel."
//
// This is the ADVANCED path: using containerd Go client to create tasks
// with the io.containerd.urunc.v2 runtime. This is how Kubernetes
// integration works internally.
// =============================================================================

package sandbox

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	coci "github.com/containerd/containerd/oci"
	uruncoci "github.com/example/ai-agent-sandbox/pkg/oci"
	"github.com/example/ai-agent-sandbox/pkg/tool"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// Manager uses containerd client to manage urunc sandboxes.
type Manager struct {
	client    *containerd.Client
	namespace string
}

// NewManager connects to containerd.
func NewManager(socketPath string) (*Manager, error) {
	if socketPath == "" {
		socketPath = "/run/containerd/containerd.sock"
	}
	client, err := containerd.New(socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to containerd: %w", err)
	}
	return &Manager{
		client:    client,
		namespace: "ai-sandbox",
	}, nil
}

// Close closes the containerd client.
func (m *Manager) Close() error {
	return m.client.Close()
}

// CreateSandbox creates a container using the urunc runtime via containerd.
func (m *Manager) CreateSandbox(ctx context.Context, t tool.Tool, id string) (containerd.Container, error) {
	ctx = namespaces.WithNamespace(ctx, m.namespace)

	// Prepare workspace mount
	hostPath, guestPath, err := PrepareWorkspace(id)
	if err != nil {
		return nil, err
	}
	_ = guestPath

	// Build OCI spec with exact urunc annotations
	annotations := uruncoci.ToolAnnotations(t.Name, t.Command)

	// Build mounts based on capabilities
	var mounts []specs.Mount
	if t.Cap.CanReadFS || t.Cap.CanWriteFS {
		for _, p := range t.Cap.AllowedFSPaths {
			// Map workspace host path to guest path
			src := hostPath
			dst := p
			mounts = append(mounts, specs.Mount{
				Destination: dst,
				Source:      src,
				Type:        "bind",
				Options:     []string{"rbind", "rw"},
			})
		}
	}

	// Memory limit: urunc passes this to QEMU via -m flag
	memLimit := int64(1024 * 1024 * 1024) // 1GB

	// Create OCI bundle on disk (urunc.json plus config.json)
	bundle, err := uruncoci.NewToolBundle(id, hostPath, annotations, mounts, memLimit)
	if err != nil {
		return nil, fmt.Errorf("create OCI bundle: %w", err)
	}

	// Write a dummy rootfs if image not pulled; in production, use the pulled image ref.
	// For this example, we assume the image is already built with bunny.
	rootfsPath := bundle.RootFS

	// Create container in containerd with urunc runtime
	// The runtime name MUST match what is registered in containerd:
	// io.containerd.urunc.v2
	opts := []coci.SpecOpts{
		coci.WithDefaultSpecForPlatform("linux/amd64"),
		coci.WithRootFSPath(rootfsPath),
		coci.WithAnnotations(annotations),
		coci.WithMounts(mounts),
		coci.WithMemoryLimit(memLimit),
	}

	container, err := m.client.NewContainer(
		ctx,
		id,
		containerd.WithRuntime("io.containerd.urunc.v2", nil),
		containerd.WithNewSpec(opts...),
	)
	if err != nil {
		return nil, fmt.Errorf("containerd create container: %w", err)
	}

	return container, nil
}

// Start starts the container task.
func (m *Manager) Start(ctx context.Context, c containerd.Container) (containerd.Task, error) {
	ctx = namespaces.WithNamespace(ctx, m.namespace)

	// cio.NewCreator creates stdio pipes as documented in urunc execution flow
	task, err := c.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return nil, fmt.Errorf("containerd new task: %w", err)
	}

	if err := task.Start(ctx); err != nil {
		return nil, fmt.Errorf("containerd task start: %w", err)
	}

	return task, nil
}

// ExecIntoTask executes a process inside a running task.
func (m *Manager) ExecIntoTask(ctx context.Context, task containerd.Task, execID string, args []string) error {
	ctx = namespaces.WithNamespace(ctx, m.namespace)

	processSpec := &specs.Process{
		Args: args,
		Env:  []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
		Cwd:  "/",
	}

	process, err := task.Exec(ctx, execID, processSpec, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return fmt.Errorf("task exec: %w", err)
	}

	return process.Start(ctx)
}

// Stop stops and deletes a container and its task.
func (m *Manager) Stop(ctx context.Context, c containerd.Container) error {
	ctx = namespaces.WithNamespace(ctx, m.namespace)

	task, err := c.Task(ctx, nil)
	if err == nil && task != nil {
		// Kill with SIGTERM then SIGKILL
		_ = task.Kill(ctx, syscall.SIGTERM)
		select {
		case <-ctx.Done():
			_ = task.Kill(ctx, syscall.SIGKILL)
		case <-time.After(10 * time.Second):
			_ = task.Kill(ctx, syscall.SIGKILL)
		case <-task.Wait(ctx):
		}
		_, _ = task.Delete(ctx, containerd.WithProcessKill)
	}

	return c.Delete(ctx, containerd.WithSnapshotCleanup)
}
