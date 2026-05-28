package oci
// =============================================================================
// FILE: pkg/oci/bundle.go
// SOURCE: https://urunc.io/design/ (Execution flow)
// SOURCE: https://github.com/urunc-dev/urunc (OCI-compatible runtime)
//
// DOCS STATE:
//   "containerd unpacks the image into a supported snapshotter (for example, devmapper)
//    and invokes urunc, as any other OCI runtime.
//    urunc parses the image's rootfs and annotations, initiating the required
//    setup procedures. In particular, it creates essential pipes for stdio,
//    it creates the container's state file and runs the prestart hooks"
// =============================================================================

package oci

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opencontainers/runtime-spec/specs-go"
)

// Bundle represents an OCI runtime bundle (config.json plus rootfs).
type Bundle struct {
	ID     string
	Path   string
	Spec   *specs.Spec
	RootFS string
}

// NewToolBundle creates an OCI bundle for an AI agent tool sandbox.
// It follows the OCI Runtime Spec that urunc adheres to.
func NewToolBundle(id, rootfsPath string, annotations map[string]string, mounts []specs.Mount, memoryLimit int64) (*Bundle, error) {
	// Ensure rootfs exists
	if err := os.MkdirAll(rootfsPath, 0755); err != nil {
		return nil, fmt.Errorf("create rootfs: %w", err)
	}

	// Write urunc.json inside rootfs (base64 encoded) as fallback for annotations
	uj := NewUruncJSON(annotations)
	if err := uj.Write(rootfsPath); err != nil {
		return nil, fmt.Errorf("write urunc.json: %w", err)
	}

	// Build OCI runtime spec
	spec := &specs.Spec{
		Version: specs.Version,
		Process: &specs.Process{
			Terminal: false,
			User: specs.User{
				UID: 0,
				GID: 0,
			},
			Args: []string{"/urunit"}, // urunit init for Linux guests
			Env:  []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
			Cwd:  "/",
		},
		Root: &specs.Root{
			Path:     rootfsPath,
			Readonly: false,
		},
		Hostname: id,
		Linux: &specs.Linux{
			Namespaces: []specs.LinuxNamespace{
				{Type: specs.NetworkNamespace},
				{Type: specs.PIDNamespace},
				{Type: specs.IPCNamespace},
				{Type: specs.UTSNamespace},
				{Type: specs.MountNamespace},
			},
			Resources: &specs.LinuxResources{
				Memory: &specs.LinuxMemory{
					Limit: &memoryLimit, // urunc passes memory limit to VMM
				},
			},
		},
		Mounts:      mounts,
		Annotations: annotations, // High-level runtime may or may not pass these
	}

	bundlePath := filepath.Join("/var/run/ai-sandbox", id)
	if err := os.MkdirAll(bundlePath, 0755); err != nil {
		return nil, fmt.Errorf("create bundle path: %w", err)
	}

	configPath := filepath.Join(bundlePath, "config.json")
	configData, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal config.json: %w", err)
	}
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return nil, fmt.Errorf("write config.json: %w", err)
	}

	return &Bundle{
		ID:     id,
		Path:   bundlePath,
		Spec:   spec,
		RootFS: rootfsPath,
	}, nil
}
