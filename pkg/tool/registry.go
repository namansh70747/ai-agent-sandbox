// =============================================================================
// FILE: pkg/tool/registry.go
// SOURCE: Original architecture from user guide (PART 2)
// IMPLEMENTATION: Custom permission model mapped to urunc capabilities.
//
// DESIGN CHOICE:
//   We map each tool to a Linux-based urunc sandbox (framework: linux, monitor: qemu)
//   because AI agent tools require general-purpose execution (shell, Python, HTTP).
//   This follows the Nubificus blog approach:
//   https://nubificus.co.uk/blog/urunc_agent/
// =============================================================================

package tool

import "fmt"

// Capability defines what a tool is allowed to do.
type Capability struct {
	CanReadFS      bool
	CanWriteFS     bool
	CanExecute     bool
	CanNetwork     bool
	CanDatabase    bool
	AllowedFSPaths []string // for example, ["/workspace"]
	AllowedDomains []string // for example, ["api.github.com"]
	AllowedPorts   []int    // for example, [5432, 3306]
}

// Tool defines a sandboxed AI agent tool.
type Tool struct {
	Name        string
	Description string
	Cap         Capability
	// ImageRef is the OCI image built with bunny for this tool.
	ImageRef string
	// Command is the default cmdline passed to urunit inside the VM.
	Command string
}

// Registry holds all tool definitions.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry initializes the registry with least-privilege defaults.
func NewRegistry() *Registry {
	r := &Registry{tools: make(map[string]Tool)}

	r.tools["file_tool"] = Tool{
		Name:        "file_tool",
		Description: "Read and write files in workspace only",
		Cap: Capability{
			CanReadFS:      true,
			CanWriteFS:     true,
			CanExecute:     false,
			CanNetwork:     false,
			CanDatabase:    false,
			AllowedFSPaths: []string{"/workspace"},
		},
		ImageRef: "localhost/ai-sandbox/file-tool:latest",
		Command:  "sleep infinity", // waits for exec requests
	}

	r.tools["code_tool"] = Tool{
		Name:        "code_tool",
		Description: "Execute shell commands and code",
		Cap: Capability{
			CanReadFS:      false,
			CanWriteFS:     false,
			CanExecute:     true,
			CanNetwork:     false,
			CanDatabase:    false,
			AllowedFSPaths: []string{},
		},
		ImageRef: "localhost/ai-sandbox/code-tool:latest",
		Command:  "sleep infinity",
	}

	r.tools["web_tool"] = Tool{
		Name:        "web_tool",
		Description: "Make HTTP requests to specific domains",
		Cap: Capability{
			CanReadFS:      false,
			CanWriteFS:     false,
			CanExecute:     false,
			CanNetwork:     true,
			CanDatabase:    false,
			AllowedDomains: []string{"api.github.com", "api.openai.com"},
			AllowedPorts:   []int{443, 80},
		},
		ImageRef: "localhost/ai-sandbox/web-tool:latest",
		Command:  "sleep infinity",
	}

	r.tools["database_tool"] = Tool{
		Name:        "database_tool",
		Description: "Connect to database",
		Cap: Capability{
			CanReadFS:   false,
			CanWriteFS:  false,
			CanExecute:  false,
			CanNetwork:  true,
			CanDatabase: true,
			AllowedPorts: []int{5432, 3306},
		},
		ImageRef: "localhost/ai-sandbox/db-tool:latest",
		Command:  "sleep infinity",
	}

	return r
}

func (r *Registry) Get(name string) (Tool, error) {
	t, ok := r.tools[name]
	if !ok {
		return Tool{}, fmt.Errorf("tool %q not found", name)
	}
	return t, nil
}

func (r *Registry) List() map[string]Tool {
	out := make(map[string]Tool, len(r.tools))
	for k, v := range r.tools {
		out[k] = v
	}
	return out
}
