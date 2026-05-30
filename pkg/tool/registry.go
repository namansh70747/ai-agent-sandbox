// pkg/tool/registry.go
//
// Defines every tool an AI agent can call, and the isolation profile each
// tool must run inside when executed via urunc.
//
// Design reference: https://nubificus.co.uk/blog/urunc_agent/
//   "urunc can be used in two ways: a) as a sandbox for the entire agent,
//    or b) as a sandbox for specific application executions triggered by
//    the agent."
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

// MountSpec describes a single bind-mount for the sandbox.
// References: https://nubificus.co.uk/blog/urunc_agent/ (Step 3: Sharing data)
//   "nerdctl run --runtime io.containerd.urunc.v2 -v ${PWD}/mydir:/mydir"
//   "use with caution"
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
	// Pre-built urunc images: harbor.nbfc.io/nubificus/urunc/…
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
}

// ToolDef combines the logical identity of a tool with its isolation profile.
type ToolDef struct {
	Name    string
	Type    ToolType
	Profile IsolationProfile
}

// Registry holds all registered tool definitions.
type Registry struct {
	tools map[ToolType]*ToolDef
}

// NewRegistry builds the default tool registry.
// Every entry follows the principle of least privilege:
// a tool gets only the access it strictly requires.
func NewRegistry(workspaceDir string) *Registry {
	r := &Registry{tools: make(map[ToolType]*ToolDef)}

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

func (r *Registry) register(t *ToolDef) {
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

// All returns every registered tool definition.
func (r *Registry) All() []*ToolDef {
	out := make([]*ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}
