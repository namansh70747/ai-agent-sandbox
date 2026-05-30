package sandbox

import (
	"strings"
	"testing"

	"github.com/namansh70747/ai-agent-sandbox/pkg/tool"
)

func cmdString(def *tool.ToolDef, cmd []string) string {
	return strings.Join(NewSpawner().BuildCommand(def, cmd).Args, " ")
}

func TestBuildCommand_BaselineNoNetworkNoMounts(t *testing.T) {
	def := &tool.ToolDef{
		Name: "code_tool", Type: "code_tool",
		Profile: tool.IsolationProfile{
			Image: "img:latest", MemoryMB: 512, CPUCount: 2, Network: tool.NetworkNone,
		},
	}
	got := cmdString(def, []string{"echo", "hi"})
	for _, want := range []string{
		"sudo nerdctl run --rm",
		"--runtime io.containerd.urunc.v2",
		"-m512M", "--cpus=2.0", "--network=none",
		"img:latest echo hi",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildCommand_MonitorOverrideOnlyForLinux(t *testing.T) {
	// Linux + firecracker → annotation present.
	linux := &tool.ToolDef{
		Name: "web_tool", Type: "web_tool",
		Profile: tool.IsolationProfile{
			Image: "img:latest", MemoryMB: 256, CPUCount: 1,
			Network: tool.NetworkBridge, Monitor: tool.MonitorFirecracker,
			UnikernelType: tool.UnikernelLinux,
		},
	}
	if got := cmdString(linux, []string{"curl"}); !strings.Contains(got, "--annotation com.urunc.unikernel.hypervisor=firecracker") {
		t.Errorf("linux tool should override monitor; got:\n%s", got)
	}

	// Unikraft image → monitor must NOT be overridden (baked by bunny).
	uni := &tool.ToolDef{
		Name: "nginx_showcase", Type: "nginx_showcase",
		Profile: tool.IsolationProfile{
			Image: "harbor/nginx:latest", MemoryMB: 128, CPUCount: 1,
			Network: tool.NetworkBridge, Monitor: tool.MonitorQEMU,
			UnikernelType: tool.UnikernelUnikraft,
		},
	}
	if got := cmdString(uni, nil); strings.Contains(got, "com.urunc.unikernel.hypervisor") {
		t.Errorf("unikraft tool must NOT override hypervisor; got:\n%s", got)
	}
}

func TestBuildCommand_Seccomp(t *testing.T) {
	mk := func(mode tool.SeccompMode) string {
		return cmdString(&tool.ToolDef{
			Name: "t", Type: "t",
			Profile: tool.IsolationProfile{Image: "i", MemoryMB: 1, CPUCount: 1, Seccomp: mode},
		}, nil)
	}
	if strings.Contains(mk(""), "seccomp") {
		t.Error(`empty seccomp should emit no flag`)
	}
	if strings.Contains(mk("default"), "seccomp") {
		t.Error(`"default" seccomp should emit no flag`)
	}
	if !strings.Contains(mk("unconfined"), "--security-opt seccomp=unconfined") {
		t.Error("unconfined seccomp flag missing")
	}
	if !strings.Contains(mk("p.json"), "--security-opt seccomp=p.json") {
		t.Error("custom seccomp path flag missing")
	}
}

func TestBuildCommand_ReadOnlyAndMountsAndEnv(t *testing.T) {
	def := &tool.ToolDef{
		Name: "t", Type: "t",
		Profile: tool.IsolationProfile{
			Image: "i", MemoryMB: 1, CPUCount: 1,
			ReadOnlyRootfs: true,
			Mounts:         []tool.MountSpec{{HostPath: "/h", ContainerPath: "/c", ReadOnly: true}},
			Env:            []tool.EnvVar{{Key: "K", Value: "V"}},
		},
	}
	got := cmdString(def, nil)
	for _, want := range []string{"--read-only", "-v /h:/c:ro", "-e K=V"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildCommand_AnnotationsPassthrough(t *testing.T) {
	def := &tool.ToolDef{
		Name: "t", Type: "t",
		Profile: tool.IsolationProfile{
			Image: "i", MemoryMB: 1, CPUCount: 1,
			Annotations: map[string]string{"com.urunc.unikernel.cmdline": "nginx"},
		},
	}
	if got := cmdString(def, nil); !strings.Contains(got, "--annotation com.urunc.unikernel.cmdline=nginx") {
		t.Errorf("annotation passthrough missing; got:\n%s", got)
	}
}
