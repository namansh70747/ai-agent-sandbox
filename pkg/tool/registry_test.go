package tool

import "testing"

func TestNewRegistry_HasFourDefaults(t *testing.T) {
	r := NewRegistry("/tmp/ws")
	if got := len(r.All()); got != 4 {
		t.Fatalf("built-in registry: want 4 tools, got %d", got)
	}
	for _, tt := range []ToolType{ToolTypeFile, ToolTypeCode, ToolTypeWeb, ToolTypeDatabase} {
		if _, err := r.Get(tt); err != nil {
			t.Errorf("Get(%s): %v", tt, err)
		}
	}
}

func TestNewRegistryFromDefs_PreservesOrder(t *testing.T) {
	defs := []*ToolDef{
		{Name: "a", Type: "a", Profile: IsolationProfile{Image: "i"}},
		{Name: "b", Type: "b", Profile: IsolationProfile{Image: "i"}},
		{Name: "c", Type: "c", Profile: IsolationProfile{Image: "i"}},
	}
	r := NewRegistryFromDefs(defs)
	all := r.All()
	if len(all) != 3 {
		t.Fatalf("want 3, got %d", len(all))
	}
	for i, want := range []string{"a", "b", "c"} {
		if all[i].Name != want {
			t.Errorf("order[%d]: want %s got %s", i, want, all[i].Name)
		}
	}
}

func TestRegistry_GetUnknown(t *testing.T) {
	r := NewRegistryFromDefs(nil)
	if _, err := r.Get("nope"); err == nil {
		t.Fatal("Get(unknown): want error, got nil")
	}
}

func TestWorkspaceMountWired(t *testing.T) {
	r := NewRegistry("/my/ws")
	ft, err := r.Get(ToolTypeFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(ft.Profile.Mounts) != 1 || ft.Profile.Mounts[0].HostPath != "/my/ws" {
		t.Errorf("file_tool workspace mount not wired: %+v", ft.Profile.Mounts)
	}
}
