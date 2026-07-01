//go:build !linux

package agent

// HardenProcessMemory is a no-op on platforms without a memory-hardening
// implementation yet. macOS (mlock + a task_for_pid / PT_DENY_ATTACH story) and
// Windows are a documented gap; until then the daemon's memory posture on those
// platforms equals a one-shot command's. Returns nil so the caller proceeds.
func HardenProcessMemory() error { return nil }
