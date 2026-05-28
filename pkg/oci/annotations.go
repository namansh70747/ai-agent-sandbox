package oci
// =============================================================================
// FILE: pkg/oci/annotations.go
// SOURCE: https://urunc.io/package/ (Building/Packaging unikernels)
// SOURCE: https://urunc.io/design/ (Image Format and Annotations)
//
// DOCS STATE:
//   "For the time being, the required annotations are the following:
//    - com.urunc.unikernel.unikernelType
//    - com.urunc.unikernel.hypervisor
//    - com.urunc.unikernel.binary
//    - com.urunc.unikernel.cmdline
//    Optional:
//    - com.urunc.unikernel.initrd
//    - com.urunc.unikernel.unikernelVersion
//    - com.urunc.unikernel.block
//    - com.urunc.unikernel.blkMntPoint
//    - com.urunc.unikernel.mountRootfs"
// =============================================================================

package oci

// Required OCI annotations for urunc. DO NOT modify key names.
const (
	// Required: unikernel framework type.
	// Supported: unikraft, rumprun, mirage, linux, mewz, hermit
	AnnotationUnikernelType = "com.urunc.unikernel.unikernelType"

	// Required: VMM or sandbox monitor.
	// Supported: qemu, firecracker, spt, hvt
	AnnotationHypervisor = "com.urunc.unikernel.hypervisor"

	// Required: absolute path to unikernel binary inside container rootfs
	AnnotationBinary = "com.urunc.unikernel.binary"

	// Required: application command line passed to the unikernel
	AnnotationCmdline = "com.urunc.unikernel.cmdline"

	// Optional: path to initrd inside container rootfs
	AnnotationInitrd = "com.urunc.unikernel.initrd"

	// Optional: version of unikernel framework (for example, 0.17.0)
	AnnotationUnikernelVersion = "com.urunc.unikernel.unikernelVersion"

	// Optional: path to block image inside container rootfs
	AnnotationBlock = "com.urunc.unikernel.block"

	// Optional: mount point for block image inside unikernel
	AnnotationBlkMntPoint = "com.urunc.unikernel.blkMntPoint"

	// Optional: boolean "true" to mount container rootfs in unikernel via shared-fs or block
	AnnotationMountRootfs = "com.urunc.unikernel.mountRootfs"
)

// UnikernelType constants from official docs.
const (
	UnikernelTypeUnikraft = "unikraft"
	UnikernelTypeRumprun  = "rumprun"
	UnikernelTypeMirage   = "mirage"
	UnikernelTypeLinux    = "linux"
	UnikernelTypeMewz     = "mewz"
	UnikernelTypeHermit   = "hermit"
)

// Hypervisor constants from official docs.
const (
	HypervisorQemu        = "qemu"
	HypervisorFirecracker = "firecracker"
	HypervisorSpt         = "spt"
	HypervisorHvt         = "hvt"
)

// ToolAnnotations returns the exact annotations for a sandboxed Linux tool.
// SOURCE: https://nubificus.co.uk/blog/urunc_agent/ (Linux over QEMU approach)
func ToolAnnotations(toolName, cmdline string) map[string]string {
	_ = toolName
	return map[string]string{
		AnnotationUnikernelType: UnikernelTypeLinux,
		AnnotationHypervisor:    HypervisorQemu,
		AnnotationBinary:        "/urunit", // urunit acts as init for Linux guests
		AnnotationCmdline:       cmdline,
		AnnotationMountRootfs:   "true", // request rootfs mount via virtiofs or 9pfs
	}
}
