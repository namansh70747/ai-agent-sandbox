// pkg/sandbox/telemetry.go
//
// Best-effort VM boot-time telemetry sourced from urunc's own timestamps log.
//
// urunc, when [timestamps] enabled=true in /etc/urunc/config.toml, appends
// boot-phase timestamps to /var/log/urunc/timestamps.log on every VM start.
// We attribute the bytes appended during one Execute() to that run by
// snapshotting the file size before the run and reading the tail after.
//
// HONEST CAVEAT: all VMs append to the SAME log. Correct attribution requires
// the snapshot→run→read window to be serialised (Manager.telemetryMu). When
// telemetry is disabled (file absent), we never lock and never attribute — so
// the common path stays fully concurrent. Per-VM logs do not exist in urunc
// today; this is the robust-enough approach for a demo.
package sandbox

import (
	"io"
	"os"
	"strings"
	"time"
)

// timestampsLogPath is where urunc writes boot timings when enabled.
// Matches configs/urunc/config.toml [timestamps] destination.
const timestampsLogPath = "/var/log/urunc/timestamps.log"

// BootTelemetry holds parsed boot timing for a single VM start.
type BootTelemetry struct {
	Source     string        `json:"source"`         // the timestamps log path
	Raw        []string      `json:"raw,omitempty"`  // appended log lines for this run
	BootTime   time.Duration `json:"-"`              // derived span, if parseable
	BootTimeMs int64         `json:"boot_time_ms"`   // same, milliseconds, for JSON
	Attributed bool          `json:"attributed"`     // false => could not attribute safely
	Note       string        `json:"note,omitempty"` // explanation when not attributed
}

// telemetryEnabled reports whether urunc boot telemetry is active (log exists).
func telemetryEnabled() bool {
	info, err := os.Stat(timestampsLogPath)
	return err == nil && !info.IsDir()
}

// snapshotTimestampsSize returns the current size of the timestamps log, or -1
// if it cannot be read.
func snapshotTimestampsSize() int64 {
	info, err := os.Stat(timestampsLogPath)
	if err != nil {
		return -1
	}
	return info.Size()
}

// readBootTelemetry reads the bytes appended to the timestamps log since
// preSize and parses them into a BootTelemetry. It never errors — on any
// problem it returns a BootTelemetry with Attributed=false and a note.
func readBootTelemetry(preSize int64, wallClock time.Duration) *BootTelemetry {
	bt := &BootTelemetry{Source: timestampsLogPath}

	if preSize < 0 {
		bt.Note = "timestamps log not readable before run"
		return bt
	}
	f, err := os.Open(timestampsLogPath)
	if err != nil {
		bt.Note = "timestamps log not readable after run"
		return bt
	}
	defer f.Close()

	if _, err := f.Seek(preSize, io.SeekStart); err != nil {
		bt.Note = "could not seek to pre-run offset"
		return bt
	}
	appended, err := io.ReadAll(f)
	if err != nil {
		bt.Note = "could not read appended bytes"
		return bt
	}

	lines := splitNonEmpty(string(appended))
	if len(lines) == 0 {
		bt.Note = "no new timestamp lines (VM may not have logged, or telemetry off)"
		return bt
	}

	bt.Raw = lines
	bt.Attributed = true
	// urunc timestamp lines are phase markers; the wall-clock duration of the
	// run is the most robust single boot figure we can attribute without
	// coupling to urunc's exact log format. We expose both: the raw phase
	// lines (for the curious) and the measured wall-clock as BootTimeMs.
	bt.BootTime = wallClock
	bt.BootTimeMs = wallClock.Milliseconds()
	return bt
}

func splitNonEmpty(s string) []string {
	out := []string{}
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			out = append(out, t)
		}
	}
	return out
}
