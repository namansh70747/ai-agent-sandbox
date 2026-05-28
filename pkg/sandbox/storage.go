// =============================================================================
// FILE: pkg/sandbox/storage.go
// SOURCE: https://urunc.io/design/ (Storage / Block devices / devmapper)
// SOURCE: https://urunc.io/installation/ (devmapper setup)
//
// DOCS STATE:
//   "urunc extracts the unikernel binary from the container image. Then, it
//    extracts any additional files present in the container image rootfs, to
//    be used as additional storage for the unikernel. urunc prepares and
//    attaches the storage backend to the unikernel via the relevant command
//    line directives of each hypervisor and unikernel type."
//
//   Supported storage for Linux and Qemu: Initrd, Block, Devmapper, 9pfs, Virtiofs
// =============================================================================

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/opencontainers/runtime-spec/specs-go"
)

// StorageType defines how the sandbox rootfs is presented to the guest.
type StorageType string

const (
	StorageInitrd    StorageType = "initrd"
	StorageBlock     StorageType = "block"
	StorageDevmapper StorageType = "devmapper"
	Storage9P        StorageType = "9pfs"
	StorageVirtioFS  StorageType = "virtiofs"
)

// StorageConfig defines the storage backend for a sandbox.
type StorageConfig struct {
	Type          StorageType
	DevmapperPool string // for example, "containerd-pool"
	BlockPath     string // path to block image inside rootfs
	MntPoint      string // guest mount point for block
	MountRootfs   bool   // maps to com.urunc.unikernel.mountRootfs
}

// PrepareWorkspace creates the host workspace directory and returns the mount spec.
// SOURCE: https://nubificus.co.uk/blog/urunc_agent/
// DOCS WARNING: "If we explicitly share data or resources with a urunc container,
// that data is no longer protected."
func PrepareWorkspace(sandboxID string) (hostPath, guestPath string, err error) {
	hostPath = filepath.Join("/var/lib/ai-sandbox/workspaces", sandboxID)
	guestPath = "/workspace"
	if err := os.MkdirAll(hostPath, 0755); err != nil {
		return "", "", fmt.Errorf("mkdir workspace: %w", err)
	}
	return hostPath, guestPath, nil
}

// ToMounts converts storage config to OCI mounts.
func (sc *StorageConfig) ToMounts(rootfs string) []specs.Mount {
	var mounts []specs.Mount
	if sc.Type == StorageVirtioFS || sc.Type == Storage9P {
		// Shared filesystem mounts are handled by urunc via hypervisor CLI args
		// when mountRootfs=true or via explicit volume mounts.
		mounts = append(mounts, specs.Mount{
			Destination: "/",
			Source:      rootfs,
			Type:        string(sc.Type),
			Options:     []string{"rw"},
		})
	}
	return mounts
}
