// Package timing provides opt-in, low-overhead stage timing for diagnosing
// latency in hot paths (notably vault unlock + cloud sync).
//
// It is a diagnostic aid, not a feature: instrumentation is compiled in but
// dormant. Timing is emitted only when the PASS_CLI_TIMING environment variable
// is set to a non-empty value other than "0". When disabled, Track performs a
// single cached boolean check and returns a no-op closure — no allocations, no
// syscalls, no measurable overhead on the normal path.
//
// Lines are written to stderr (stdout stays clean for script consumers) in the
// form:
//
//	[timing] <stage>            <duration>
package timing

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	enabledOnce sync.Once
	enabled     bool
)

// Enabled reports whether timing output is turned on. The environment is read
// once and cached for the lifetime of the process.
func Enabled() bool {
	enabledOnce.Do(func() {
		v := os.Getenv("PASS_CLI_TIMING")
		enabled = v != "" && v != "0"
	})
	return enabled
}

// Track starts a timer for a named stage and returns a stop function that, when
// called, emits the elapsed duration. The intended use is:
//
//	defer timing.Track("rclone lsjson (probe)")()
//
// When timing is disabled, Track returns a no-op closure and does no work.
func Track(stage string) func() {
	if !Enabled() {
		return func() {}
	}
	start := time.Now()
	return func() {
		fmt.Fprintf(os.Stderr, "[timing] %-26s %v\n", stage, time.Since(start).Round(time.Microsecond))
	}
}
