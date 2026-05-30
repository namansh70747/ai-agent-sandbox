// pkg/tool/registry.go
//
// Defines every tool an AI agent can call, and the isolation profile each
// tool must run inside when executed via urunc.
//
// Design reference: https://nubificus.co.uk/blog/urunc_agent/
//
//	"urunc can be used in two ways: a) as a sandbox for the entire agent,
//	 or b) as a sandbox for specific application executions triggered by
//	 the agent."
//
// This project implements approach (b): per-tool sandboxing.
//
// urunc annotations reference: https://urunc.io/package/
// Network isolation reference: https://urunc.io/design/
package tool

import "fmt"

// ToolType is the logical class of operation an AI tool performs.
type ToolType string

const (
	ToolTypeFile     ToolType = "file_tool"     // read / write workspace files
	ToolTypeCode     ToolType = "code_tool"     // execute shell commands / scripts
	ToolTypeWeb      ToolType = "web_tool"      // outbound HTTP requests
	ToolTypeDatabase ToolType = "database_tool" // database queries
)

// NetworkMode controls what network access the sandbox receives.
// References: https://urunc.io/design/ (Network handling section)
type NetworkMode string

const (
	NetworkNone   NetworkMode = "none"   // --network=none  – no NIC attached
	NetworkBridge NetworkMode = "bridge" // default bridge via CNI
)

// Monitor is the urunc VM/sandbox monitor (hypervisor) a tool runs under.
// Mapped to the OCI annotation com.urunc.unikernel.hypervisor.
// Reference: https://urunc.io/hypervisor-support/
// NOTE: Linux-based tool images only run on QEMU or Firecracker. Solo5
// (hvt/spt) supports Rumprun/MirageOS only — do not use it for Linux tools.
type Monitor string

const (
	MonitorQEMU        Monitor = "qemu"        // full Linux compatibility, most tested
	MonitorFirecracker Monitor = "firecracker" // fast cold-start; Linux support can be flaky
)

// UnikernelType is the com.urunc.unikernel.unikernelType annotation value.
// Reference: https://urunc.io/package/
type UnikernelType string

const (
	UnikernelLinux    UnikernelType = "linux"    // default: Linux kernel as a microVM
	UnikernelUnikraft UnikernelType = "unikraft" // real unikernel (showcase tools)
	UnikernelRumprun  UnikernelType = "rumprun"  // real unikernel (Solo5 monitors)
)

// SeccompMode selects the seccomp profile applied to the sandbox.
// "" or "default" → nerdctl default profile; "unconfined" → no filtering;
// any other value → path to a custom seccomp profile JSON.
type SeccompMode string

// MountSpec describes a single bind-mount for the sandbox.
// References: https://nubificus.co.uk/blog/urunc_agent/ (Step 3: Sharing data)
//
//	"nerdctl run --runtime io.containerd.urunc.v2 -v ${PWD}/mydir:/mydir"
//	"use with caution"
type MountSpec struct {
	HostPath      string // absolute path on the host / Lima guest
	ContainerPath string // path inside the microVM
	ReadOnly      bool   // true  → ro  (safe for workspace reads)
}

// EnvVar is a single environment variable passed into the sandbox.
type EnvVar struct {
	Key   string
	Value string
}

// IsolationProfile is the complete set of sandbox parameters for one tool.
// Every field maps to a concrete nerdctl flag or urunc annotation.
type IsolationProfile struct {
	// Container image to use.
	// Built locally by scripts/01-build-tool-images.sh → localhost/ai-sandbox/base-tool:latest
	// Alpine + bash + curl + python3 + /urunit init, packaged with bunny Containerfile syntax.
	Image string

	// Memory limit for the microVM (maps to nerdctl -m / --memory).
	MemoryMB int

	// vCPU count (maps to nerdctl --cpus).
	CPUCount float64

	// Network access level.
	Network NetworkMode

	// Bind mounts.  blog warning: "that data is no longer protected"
	Mounts []MountSpec

	// Extra environment variables forwarded into the VM.
	Env []EnvVar

	// Human-readable reason for each permission decision (for audit log).
	Rationale string

	// ── Extended isolation dimensions (all zero-value safe) ──────────────────
	// A zero value reproduces the original behaviour exactly, so existing
	// code that builds an IsolationProfile without these fields is unaffected.

	// Monitor overrides the hypervisor via com.urunc.unikernel.hypervisor.
	// "" → use the image's baked-in monitor (no override).
	Monitor Monitor

	// UnikernelType is informational + guards the monitor override (we only
	// override the hypervisor for Linux unikernels). "" is treated as linux.
	UnikernelType UnikernelType

	// EgressAllowlist is a DECLARED policy of allowed destinations.
	// HONEST LIMITATION: nerdctl/urunc cannot enforce per-destination egress
	// today — bridge grants full egress. This is surfaced in audit/metrics and
	// logged as a warning; real enforcement needs a chained CNI firewall plugin.
	EgressAllowlist []string

	// Seccomp profile mode. "" / "default" / "unconfined" / "<path-to-json>".
	Seccomp SeccompMode

	// ReadOnlyRootfs maps to nerdctl --read-only (immutable guest rootfs).
	ReadOnlyRootfs bool

	// TimeoutSeconds caps a single execution. 0 → manager's default applies.
	TimeoutSeconds int

	// Annotations are extra com.urunc.unikernel.* (or other) annotations
	// passed verbatim as --annotation k=v.
	Annotations map[string]string

	// Category groups tools for discovery in /api/v1/tools (e.g. "compute").
	Category string

	// Description is a human-facing summary distinct from Rationale; used as
	// the MCP tool description.
	Description string
}

// ToolDef combines the logical identity of a tool with its isolation profile.
type ToolDef struct {
	Name    string
	Type    ToolType
	Profile IsolationProfile
}

// Registry holds all registered tool definitions.
// order preserves registration order so All() / listings are deterministic.
type Registry struct {
	tools map[ToolType]*ToolDef
	order []ToolType
}

// NewRegistry builds the default tool registry.
// Every entry follows the principle of least privilege:
// a tool gets only the access it strictly requires.
func NewRegistry(workspaceDir string) *Registry {
	r := &Registry{tools: make(map[ToolType]*ToolDef), order: make([]ToolType, 0, 4)}

	// ── file_tool ────────────────────────────────────────────────────────────
	// Reads / writes files inside the workspace only.
	// Network: none  – file operations need no network.
	// Mount:   workspace read-write so the agent can persist results.
	// Image:   pre-built Linux/QEMU urunc image (full shell available).
	r.register(&ToolDef{
		Name: "file_tool",
		Type: ToolTypeFile,
		Profile: IsolationProfile{
			Image:    "localhost/ai-sandbox/base-tool:latest",
			MemoryMB: 256,
			CPUCount: 1,
			Network:  NetworkNone,
			Mounts: []MountSpec{
				{HostPath: workspaceDir, ContainerPath: "/workspace", ReadOnly: false},
			},
			Rationale: "File I/O only; no network; workspace mount rw; no other paths accessible.",
		},
	})

	// ── code_tool ────────────────────────────────────────────────────────────
	// Executes arbitrary shell commands / scripts.
	// Network: none  – code execution should be air-gapped by default.
	// Mount:   none  – do not expose host filesystem to arbitrary code.
	r.register(&ToolDef{
		Name: "code_tool",
		Type: ToolTypeCode,
		Profile: IsolationProfile{
			Image:    "localhost/ai-sandbox/base-tool:latest",
			MemoryMB: 512,
			CPUCount: 2,
			Network:  NetworkNone,
			Mounts:   nil, // no filesystem exposure
			Rationale: "Code execution; no network; no host filesystem mounts; " +
				"strongest isolation profile because untrusted code runs here.",
		},
	})

	// ── web_tool ─────────────────────────────────────────────────────────────
	// Performs outbound HTTP/HTTPS requests.
	// Network: bridge – must reach the internet.
	// Mount:   none  – fetched data is returned as stdout only.
	r.register(&ToolDef{
		Name: "web_tool",
		Type: ToolTypeWeb,
		Profile: IsolationProfile{
			Image:    "localhost/ai-sandbox/base-tool:latest",
			MemoryMB: 256,
			CPUCount: 1,
			Network:  NetworkBridge,
			Mounts:   nil,
			Rationale: "HTTP requests only; bridge network enabled; " +
				"no filesystem mounts to prevent data exfiltration via web.",
		},
	})

	// ── database_tool ────────────────────────────────────────────────────────
	// Runs SQL queries against a database endpoint.
	// Network: bridge – needs to reach the database host.
	// Mount:   none.
	r.register(&ToolDef{
		Name: "database_tool",
		Type: ToolTypeDatabase,
		Profile: IsolationProfile{
			Image:    "localhost/ai-sandbox/base-tool:latest",
			MemoryMB: 256,
			CPUCount: 1,
			Network:  NetworkBridge,
			Mounts:   nil,
			Env: []EnvVar{
				{Key: "DB_HOST", Value: "localhost"},
				{Key: "DB_PORT", Value: "5432"},
			},
			Rationale: "Database access; bridge network for DB connectivity; " +
				"no filesystem mounts; env vars carry connection params only.",
		},
	})

	return r
}

// NewRegistryFromDefs builds a Registry from explicit tool definitions.
// Used by pkg/policy to construct a registry from a parsed YAML policy file,
// since the tools map and register() are unexported.
func NewRegistryFromDefs(defs []*ToolDef) *Registry {
	r := &Registry{tools: make(map[ToolType]*ToolDef), order: make([]ToolType, 0, len(defs))}
	for _, d := range defs {
		r.register(d)
	}
	return r
}

func (r *Registry) register(t *ToolDef) {
	if _, exists := r.tools[t.Type]; !exists {
		r.order = append(r.order, t.Type)
	}
	r.tools[t.Type] = t
}

// Get returns the ToolDef for the given type, or an error.
func (r *Registry) Get(tt ToolType) (*ToolDef, error) {
	t, ok := r.tools[tt]
	if !ok {
		return nil, fmt.Errorf("tool type %q not registered", tt)
	}
	return t, nil
}

// All returns every registered tool definition in registration order.
func (r *Registry) All() []*ToolDef {
	out := make([]*ToolDef, 0, len(r.order))
	for _, tt := range r.order {
		out = append(out, r.tools[tt])
	}
	return out
}
