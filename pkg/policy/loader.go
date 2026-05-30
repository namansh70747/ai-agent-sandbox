// pkg/policy/loader.go
//
// Declarative policy engine: parses configs/policies.yaml into tool.ToolDef
// values and builds a tool.Registry. Tools become data, not code — adding a
// tool is a YAML edit, no recompile.
//
// Schema (see configs/policies.yaml):
//
//	version: 1
//	defaults: { ...profile fields... }   # merged into every tool
//	tools:
//	  - name: code_tool
//	    type: code_tool                  # must be a known tool.ToolType
//	    ...profile fields (override defaults)...
//
// ${WORKSPACE} in any mount/string is replaced with the --workspace value.
package policy

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/namansh70747/ai-agent-sandbox/pkg/tool"
)

// fileSchema is the top-level YAML document.
type fileSchema struct {
	Version  int         `yaml:"version"`
	Defaults profileYAML `yaml:"defaults"`
	Tools    []toolYAML  `yaml:"tools"`
}

// toolYAML is one tool entry; profile fields are inlined at the same level.
type toolYAML struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	profileYAML `yaml:",inline"`
}

// profileYAML mirrors the per-tool / defaults fields. Pointers/empty values
// let us distinguish "unset" (inherit default) from "explicitly set".
type profileYAML struct {
	Category        string            `yaml:"category"`
	Image           string            `yaml:"image"`
	Monitor         string            `yaml:"monitor"`
	UnikernelType   string            `yaml:"unikernel_type"`
	MemoryMB        int               `yaml:"memory_mb"`
	CPUs            float64           `yaml:"cpus"`
	Network         string            `yaml:"network"`
	EgressAllowlist []string          `yaml:"egress_allowlist"`
	Mounts          []string          `yaml:"mounts"` // "host:container[:ro|rw]"
	Env             map[string]string `yaml:"env"`
	Seccomp         string            `yaml:"seccomp"`
	ReadOnlyRootfs  *bool             `yaml:"read_only_rootfs"`
	TimeoutSeconds  int               `yaml:"timeout_seconds"`
	Annotations     map[string]string `yaml:"annotations"`
	Description     string            `yaml:"description"`
	Rationale       string            `yaml:"rationale"`
}

// Load reads a policy YAML file, merges defaults into each tool, expands
// ${WORKSPACE}, validates, and returns the tool definitions.
func Load(path, workspaceDir string) ([]*tool.ToolDef, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy %s: %w", path, err)
	}

	var doc fileSchema
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse policy %s: %w", path, err)
	}
	if len(doc.Tools) == 0 {
		return nil, fmt.Errorf("policy %s: no tools defined", path)
	}

	defs := make([]*tool.ToolDef, 0, len(doc.Tools))
	for i, t := range doc.Tools {
		merged := mergeDefaults(doc.Defaults, t.profileYAML)
		def, err := toToolDef(t.Name, t.Type, merged, workspaceDir)
		if err != nil {
			return nil, fmt.Errorf("tool #%d (%q): %w", i+1, t.Name, err)
		}
		defs = append(defs, def)
	}

	if err := validate(defs); err != nil {
		return nil, err
	}
	return defs, nil
}

// LoadInto parses the policy and returns a fully built *tool.Registry.
func LoadInto(path, workspaceDir string) (*tool.Registry, error) {
	defs, err := Load(path, workspaceDir)
	if err != nil {
		return nil, err
	}
	return tool.NewRegistryFromDefs(defs), nil
}

// mergeDefaults overlays a tool's explicitly-set fields onto the defaults.
func mergeDefaults(def, t profileYAML) profileYAML {
	out := def // start from defaults
	if t.Category != "" {
		out.Category = t.Category
	}
	if t.Image != "" {
		out.Image = t.Image
	}
	if t.Monitor != "" {
		out.Monitor = t.Monitor
	}
	if t.UnikernelType != "" {
		out.UnikernelType = t.UnikernelType
	}
	if t.MemoryMB != 0 {
		out.MemoryMB = t.MemoryMB
	}
	if t.CPUs != 0 {
		out.CPUs = t.CPUs
	}
	if t.Network != "" {
		out.Network = t.Network
	}
	if t.EgressAllowlist != nil {
		out.EgressAllowlist = t.EgressAllowlist
	}
	if t.Mounts != nil {
		out.Mounts = t.Mounts
	}
	if t.Env != nil {
		out.Env = t.Env
	}
	if t.Seccomp != "" {
		out.Seccomp = t.Seccomp
	}
	if t.ReadOnlyRootfs != nil {
		out.ReadOnlyRootfs = t.ReadOnlyRootfs
	}
	if t.TimeoutSeconds != 0 {
		out.TimeoutSeconds = t.TimeoutSeconds
	}
	if t.Annotations != nil {
		out.Annotations = t.Annotations
	}
	if t.Description != "" {
		out.Description = t.Description
	}
	if t.Rationale != "" {
		out.Rationale = t.Rationale
	}
	return out
}

// toToolDef converts a merged YAML profile into a tool.ToolDef.
func toToolDef(name, typ string, p profileYAML, workspaceDir string) (*tool.ToolDef, error) {
	if name == "" {
		return nil, fmt.Errorf("missing name")
	}
	if typ == "" {
		return nil, fmt.Errorf("missing type")
	}
	if p.Image == "" {
		return nil, fmt.Errorf("missing image")
	}

	mounts := make([]tool.MountSpec, 0, len(p.Mounts))
	for _, m := range p.Mounts {
		ms, err := parseMount(expand(m, workspaceDir))
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, ms)
	}

	env := make([]tool.EnvVar, 0, len(p.Env))
	for k, v := range p.Env {
		env = append(env, tool.EnvVar{Key: k, Value: expand(v, workspaceDir)})
	}

	readOnly := false
	if p.ReadOnlyRootfs != nil {
		readOnly = *p.ReadOnlyRootfs
	}

	return &tool.ToolDef{
		Name: name,
		Type: tool.ToolType(typ),
		Profile: tool.IsolationProfile{
			Image:           p.Image,
			MemoryMB:        p.MemoryMB,
			CPUCount:        p.CPUs,
			Network:         tool.NetworkMode(p.Network),
			Mounts:          mounts,
			Env:             env,
			Rationale:       p.Rationale,
			Monitor:         tool.Monitor(p.Monitor),
			UnikernelType:   tool.UnikernelType(p.UnikernelType),
			EgressAllowlist: p.EgressAllowlist,
			Seccomp:         tool.SeccompMode(p.Seccomp),
			ReadOnlyRootfs:  readOnly,
			TimeoutSeconds:  p.TimeoutSeconds,
			Annotations:     p.Annotations,
			Category:        p.Category,
			Description:     p.Description,
		},
	}, nil
}

// parseMount turns "host:container", "host:container:ro" or "...:rw" into a MountSpec.
func parseMount(s string) (tool.MountSpec, error) {
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 2:
		return tool.MountSpec{HostPath: parts[0], ContainerPath: parts[1]}, nil
	case 3:
		ro := false
		switch parts[2] {
		case "ro":
			ro = true
		case "rw":
			ro = false
		default:
			return tool.MountSpec{}, fmt.Errorf("mount %q: mode must be 'ro' or 'rw'", s)
		}
		return tool.MountSpec{HostPath: parts[0], ContainerPath: parts[1], ReadOnly: ro}, nil
	default:
		return tool.MountSpec{}, fmt.Errorf("mount %q: expected host:container[:ro|rw]", s)
	}
}

// validate enforces structural rules across all tools.
func validate(defs []*tool.ToolDef) error {
	seen := make(map[tool.ToolType]string)
	for _, d := range defs {
		// Duplicate ToolType would collide in the registry map.
		if prev, dup := seen[d.Type]; dup {
			return fmt.Errorf("duplicate type %q used by both %q and %q; "+
				"each tool needs a unique type", d.Type, prev, d.Name)
		}
		seen[d.Type] = d.Name

		switch tool.Monitor(d.Profile.Monitor) {
		case "", tool.MonitorQEMU, tool.MonitorFirecracker:
		default:
			return fmt.Errorf("tool %q: unknown monitor %q (use qemu or firecracker)", d.Name, d.Profile.Monitor)
		}

		switch d.Profile.Network {
		case "", tool.NetworkNone, tool.NetworkBridge:
		default:
			return fmt.Errorf("tool %q: unknown network %q (use none or bridge)", d.Name, d.Profile.Network)
		}
	}
	return nil
}

// expand replaces ${WORKSPACE} with the workspace directory.
func expand(s, workspaceDir string) string {
	return strings.ReplaceAll(s, "${WORKSPACE}", workspaceDir)
}
