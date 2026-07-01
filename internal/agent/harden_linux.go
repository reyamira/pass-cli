//go:build linux

package agent

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// HardenProcessMemory locks the daemon's memory into RAM and disables core dumps.
// It is called once at agent startup, before the vault is unlocked, so the master
// password and decrypted credentials held in the resident VaultService are covered:
//
//   - mlockall(MCL_CURRENT|MCL_FUTURE) keeps all current and future pages resident,
//     so no secret can be paged out to swap.
//   - prctl(PR_SET_DUMPABLE, 0) disables core dumps and blocks a casual same-user
//     process from ptrace-attaching or reading /proc/<pid>/mem.
//
// Honest ceiling (same as ssh-agent/gpg-agent): root, or a same-user process that
// can still obtain ptrace via other means, can read the memory. This is strictly
// better than a long-lived shell env var or a plaintext file.
//
// Best-effort and ordered so the privilege-free protection always applies:
//   - PR_SET_DUMPABLE=0 is done FIRST; it needs no privilege and essentially never
//     fails, so core-dump/ptrace protection is on even when mlock can't be.
//   - mlockall is attempted second; it needs CAP_IPC_LOCK or a sufficient
//     RLIMIT_MEMLOCK, and a Go runtime easily exceeds the common 8 MB default, so
//     this is the part that usually fails without setup (see the systemd unit's
//     LimitMEMLOCK=infinity in Phase 3). On failure the function returns an error
//     but PR_SET_DUMPABLE=0 has already taken effect; the caller logs and continues.
func HardenProcessMemory() error {
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl(PR_SET_DUMPABLE): %w", err)
	}
	if err := unix.Mlockall(unix.MCL_CURRENT | unix.MCL_FUTURE); err != nil {
		return fmt.Errorf("mlockall (need CAP_IPC_LOCK or a higher RLIMIT_MEMLOCK, e.g. systemd LimitMEMLOCK=infinity): %w", err)
	}
	return nil
}
