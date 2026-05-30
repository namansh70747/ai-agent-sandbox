// pkg/audit/audit.go
//
// Structured audit logging + in-memory metrics for tool executions.
//
// Every sandboxed execution produces one JSON line in the audit log (who ran
// what, under which isolation, with what result) and updates counters that the
// /metrics endpoint exposes in Prometheus text format. Both are dependency-free.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
)

// Entry is one audit record — a single tool execution.
type Entry struct {
	Time       string `json:"time"`      // RFC3339, supplied by caller
	Tool       string `json:"tool"`      // tool name
	Type       string `json:"type"`      // tool type
	Monitor    string `json:"monitor"`   // qemu | firecracker
	Network    string `json:"network"`   // none | bridge
	Seccomp    string `json:"seccomp"`   // default | unconfined | <path>
	ReadOnly   bool   `json:"read_only"` // read-only rootfs?
	Command    string `json:"command"`   // joined command (audit-friendly)
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	BootTimeMs int64  `json:"boot_time_ms,omitempty"`
	EgressDecl bool   `json:"egress_declared"` // declared (NOT enforced) allowlist present
}

// Recorder appends audit entries to a file and maintains live metrics.
// Safe for concurrent use.
type Recorder struct {
	mu   sync.Mutex
	f    *os.File
	path string

	// metrics
	execTotal   map[string]int64 // by tool name
	exitTotal   map[int]int64    // by exit code
	durationSum map[string]int64 // ms, by tool
	bootSum     map[string]int64 // ms, by tool (attributed only)
	bootCount   map[string]int64 // attributed boot samples, by tool
}

// NewRecorder opens (or creates) the audit log at path. A nil error with a
// nil file is allowed — if the file cannot be opened, auditing degrades to
// metrics-only and never blocks execution.
func NewRecorder(path string) *Recorder {
	r := &Recorder{
		path:        path,
		execTotal:   map[string]int64{},
		exitTotal:   map[int]int64{},
		durationSum: map[string]int64{},
		bootSum:     map[string]int64{},
		bootCount:   map[string]int64{},
	}
	if path != "" {
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			r.f = f
		}
	}
	return r
}

// Record writes one entry (best-effort) and updates metrics.
func (r *Recorder) Record(e Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.execTotal[e.Tool]++
	r.exitTotal[e.ExitCode]++
	r.durationSum[e.Tool] += e.DurationMs
	if e.BootTimeMs > 0 {
		r.bootSum[e.Tool] += e.BootTimeMs
		r.bootCount[e.Tool]++
	}

	if r.f != nil {
		if b, err := json.Marshal(e); err == nil {
			r.f.Write(append(b, '\n'))
		}
	}
}

// Prometheus renders current metrics in Prometheus text exposition format.
func (r *Recorder) Prometheus() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var b []byte
	add := func(s string) { b = append(b, s...) }

	add("# HELP sandbox_executions_total Tool executions by tool.\n")
	add("# TYPE sandbox_executions_total counter\n")
	for _, tool := range sortedKeys(r.execTotal) {
		add(fmt.Sprintf("sandbox_executions_total{tool=%q} %d\n", tool, r.execTotal[tool]))
	}

	add("# HELP sandbox_exit_total Executions by exit code.\n")
	add("# TYPE sandbox_exit_total counter\n")
	codes := make([]int, 0, len(r.exitTotal))
	for c := range r.exitTotal {
		codes = append(codes, c)
	}
	sort.Ints(codes)
	for _, c := range codes {
		add(fmt.Sprintf("sandbox_exit_total{code=\"%d\"} %d\n", c, r.exitTotal[c]))
	}

	add("# HELP sandbox_duration_ms_avg Average wall-clock duration by tool (ms).\n")
	add("# TYPE sandbox_duration_ms_avg gauge\n")
	for _, tool := range sortedKeys(r.durationSum) {
		n := r.execTotal[tool]
		if n == 0 {
			continue
		}
		add(fmt.Sprintf("sandbox_duration_ms_avg{tool=%q} %d\n", tool, r.durationSum[tool]/n))
	}

	add("# HELP sandbox_boot_ms_avg Average attributed VM boot time by tool (ms).\n")
	add("# TYPE sandbox_boot_ms_avg gauge\n")
	for _, tool := range sortedKeys(r.bootSum) {
		n := r.bootCount[tool]
		if n == 0 {
			continue
		}
		add(fmt.Sprintf("sandbox_boot_ms_avg{tool=%q} %d\n", tool, r.bootSum[tool]/n))
	}

	return string(b)
}

func sortedKeys(m map[string]int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
