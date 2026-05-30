package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/namansh70747/ai-agent-sandbox/pkg/tool"
)

// writeTemp writes content to a temp policy file and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "policies.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp policy: %v", err)
	}
	return p
}

func TestLoad_MergesDefaultsAndExpandsWorkspace(t *testing.T) {
	yaml := `
version: 1
defaults:
  image: base:latest
  monitor: qemu
  memory_mb: 256
  cpus: 1
  network: none
  seccomp: default
tools:
  - name: file_tool
    type: file_tool
    network: none
    mounts:
      - "${WORKSPACE}:/workspace:rw"
  - name: code_tool
    type: code_tool
    memory_mb: 512
    cpus: 2
    seccomp: configs/seccomp/strict.json
    read_only_rootfs: true
`
	defs, err := Load(writeTemp(t, yaml), "/tmp/ws")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("want 2 tools, got %d", len(defs))
	}

	// file_tool inherits image+monitor+memory from defaults.
	ft := defs[0]
	if ft.Profile.Image != "base:latest" {
		t.Errorf("file_tool image: want base:latest, got %q", ft.Profile.Image)
	}
	if ft.Profile.MemoryMB != 256 {
		t.Errorf("file_tool memory: want 256 (default), got %d", ft.Profile.MemoryMB)
	}
	if ft.Profile.Monitor != tool.MonitorQEMU {
		t.Errorf("file_tool monitor: want qemu (default), got %q", ft.Profile.Monitor)
	}
	// ${WORKSPACE} expanded.
	if len(ft.Profile.Mounts) != 1 || ft.Profile.Mounts[0].HostPath != "/tmp/ws" {
		t.Errorf("workspace not expanded: %+v", ft.Profile.Mounts)
	}
	if ft.Profile.Mounts[0].ReadOnly {
		t.Errorf("rw mount parsed as ro")
	}

	// code_tool overrides memory/cpus and sets seccomp+read-only.
	ct := defs[1]
	if ct.Profile.MemoryMB != 512 || ct.Profile.CPUCount != 2 {
		t.Errorf("code_tool overrides not applied: mem=%d cpu=%v", ct.Profile.MemoryMB, ct.Profile.CPUCount)
	}
	if ct.Profile.Seccomp != "configs/seccomp/strict.json" {
		t.Errorf("code_tool seccomp: got %q", ct.Profile.Seccomp)
	}
	if !ct.Profile.ReadOnlyRootfs {
		t.Errorf("code_tool read_only_rootfs not set")
	}
}

func TestLoad_RejectsDuplicateType(t *testing.T) {
	yaml := `
version: 1
defaults: { image: base:latest }
tools:
  - name: a
    type: web_tool
  - name: b
    type: web_tool
`
	_, err := Load(writeTemp(t, yaml), "/tmp/ws")
	if err == nil {
		t.Fatal("expected error for duplicate type, got nil")
	}
}

func TestLoad_RejectsUnknownMonitor(t *testing.T) {
	yaml := `
version: 1
defaults: { image: base:latest }
tools:
  - name: a
    type: web_tool
    monitor: solo5
`
	if _, err := Load(writeTemp(t, yaml), "/tmp/ws"); err == nil {
		t.Fatal("expected error for unknown monitor solo5, got nil")
	}
}

func TestLoad_RejectsMissingImage(t *testing.T) {
	yaml := `
version: 1
tools:
  - name: a
    type: web_tool
`
	if _, err := Load(writeTemp(t, yaml), "/tmp/ws"); err == nil {
		t.Fatal("expected error for missing image, got nil")
	}
}

func TestParseMount(t *testing.T) {
	cases := []struct {
		in      string
		host    string
		cont    string
		ro      bool
		wantErr bool
	}{
		{"h:/c", "h", "/c", false, false},
		{"h:/c:ro", "h", "/c", true, false},
		{"h:/c:rw", "h", "/c", false, false},
		{"h:/c:bogus", "", "", false, true},
		{"justone", "", "", false, true},
	}
	for _, c := range cases {
		got, err := parseMount(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseMount(%q): want error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMount(%q): %v", c.in, err)
			continue
		}
		if got.HostPath != c.host || got.ContainerPath != c.cont || got.ReadOnly != c.ro {
			t.Errorf("parseMount(%q) = %+v", c.in, got)
		}
	}
}

func TestLoadInto_BuildsRegistryInOrder(t *testing.T) {
	yaml := `
version: 1
defaults: { image: base:latest }
tools:
  - { name: first, type: file_tool }
  - { name: second, type: code_tool }
  - { name: third, type: web_tool }
`
	reg, err := LoadInto(writeTemp(t, yaml), "/tmp/ws")
	if err != nil {
		t.Fatalf("LoadInto: %v", err)
	}
	all := reg.All()
	if len(all) != 3 {
		t.Fatalf("want 3, got %d", len(all))
	}
	wantOrder := []string{"first", "second", "third"}
	for i, w := range wantOrder {
		if all[i].Name != w {
			t.Errorf("order[%d]: want %s, got %s", i, w, all[i].Name)
		}
	}
}
