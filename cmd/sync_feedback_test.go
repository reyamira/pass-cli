package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arimxyer/pass-cli/internal/vault"
)

// captureOutErr captures both os.Stdout and os.Stderr produced by fn.
// Declared once here; cmd/exec_test.go already owns captureStdout — do not
// redeclare shared helpers.
func captureOutErr(t *testing.T, fn func()) (stdoutStr, stderrStr string) {
	t.Helper()

	origOut, origErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stderr pipe: %v", err)
	}
	os.Stdout = wOut
	os.Stderr = wErr

	outDone := make(chan string, 1)
	errDone := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rOut)
		outDone <- buf.String()
	}()
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rErr)
		errDone <- buf.String()
	}()

	fn()

	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout = origOut
	os.Stderr = origErr
	stdoutStr = <-outDone
	stderrStr = <-errDone
	_ = rOut.Close()
	_ = rErr.Close()
	return stdoutStr, stderrStr
}

// newVaultServiceForSyncTest builds a real VaultService whose sync is enabled or
// disabled per the flag, by pointing config loading at a temp config file via
// PASS_CLI_CONFIG. A bogus remote is used: when sync runs it fails fast (no real
// network), which is fine — these tests assert on the cmd-layer feedback/no-op
// behavior, not on rclone itself.
func newVaultServiceForSyncTest(t *testing.T, syncEnabled bool) *vault.VaultService {
	t.Helper()

	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")
	cfgPath := filepath.Join(tmpDir, "config.yml")

	cfg := "vault_path: " + vaultPath + "\n"
	if syncEnabled {
		cfg += "sync:\n  enabled: true\n  remote: \"mock-remote:bucket\"\n"
	}
	if err := os.WriteFile(cfgPath, []byte(cfg), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	t.Setenv("PASS_CLI_CONFIG", cfgPath)

	vs, err := vault.New(vaultPath)
	if err != nil {
		t.Fatalf("failed to create VaultService: %v", err)
	}
	if vs.IsSyncEnabled() != syncEnabled {
		t.Fatalf("IsSyncEnabled() = %v, want %v", vs.IsSyncEnabled(), syncEnabled)
	}
	return vs
}

// setOffline sets the package-level offline flag and restores it after the test.
// IsOffline() reads the package var directly, so toggling it (not viper) is the
// reliable lever in a unit test where flags are never parsed.
func setOffline(t *testing.T, v bool) {
	t.Helper()
	orig := offline
	offline = v
	t.Cleanup(func() { offline = orig })
}

func setVerbose(t *testing.T, v bool) {
	t.Helper()
	orig := verbose
	verbose = v
	t.Cleanup(func() { verbose = orig })
}

func TestIsOffline(t *testing.T) {
	setOffline(t, false)
	if IsOffline() {
		t.Error("IsOffline() = true with offline=false, want false")
	}
	offline = true
	if !IsOffline() {
		t.Error("IsOffline() = false with offline=true, want true")
	}
}

// --offline must skip the pre-unlock pull entirely (no output, no rclone),
// even when sync is enabled. A sync-enabled service is required to prove the
// no-op comes from the offline check and not from sync being disabled.
func TestSyncPullBeforeUnlock_OfflineNoOp(t *testing.T) {
	vs := newVaultServiceForSyncTest(t, true)
	setOffline(t, true)
	setVerbose(t, false)

	stdout, stderr := captureOutErr(t, func() {
		syncPullBeforeUnlock(vs)
	})

	if stdout != "" {
		t.Errorf("expected empty stdout when offline, got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr when offline, got %q", stderr)
	}
}

// --offline must skip the post-command push entirely (correctness: no blind
// overwrite of a newer remote). Sync enabled to prove the offline check fires.
func TestSyncPushAfterCommand_OfflineNoOp(t *testing.T) {
	vs := newVaultServiceForSyncTest(t, true)
	setOffline(t, true)
	setVerbose(t, false)

	stdout, stderr := captureOutErr(t, func() {
		syncPushAfterCommand(vs)
	})

	if stdout != "" {
		t.Errorf("expected empty stdout when offline, got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr when offline, got %q", stderr)
	}
}

// When sync is enabled and not offline, the pull shows a transient indicator on
// stderr and nothing on stdout.
func TestSyncPullBeforeUnlock_FeedbackOnStderr(t *testing.T) {
	vs := newVaultServiceForSyncTest(t, true)
	setOffline(t, false)
	setVerbose(t, false)

	stdout, stderr := captureOutErr(t, func() {
		syncPullBeforeUnlock(vs)
	})

	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "Checking remote") {
		t.Errorf("expected stderr to contain the progress indicator, got %q", stderr)
	}
	// Indicator must be transient: cleared with CR + ANSI erase-to-end-of-line.
	if !strings.Contains(stderr, "\r\033[K") {
		t.Errorf("expected stderr to contain the clear sequence, got %q", stderr)
	}
}

// When sync is disabled, the pull shows nothing at all — even under verbose.
func TestSyncPullBeforeUnlock_DisabledNoOutput(t *testing.T) {
	vs := newVaultServiceForSyncTest(t, false)
	setOffline(t, false)

	for _, v := range []bool{false, true} {
		setVerbose(t, v)
		stdout, stderr := captureOutErr(t, func() {
			syncPullBeforeUnlock(vs)
		})
		if stdout != "" {
			t.Errorf("verbose=%v: expected empty stdout, got %q", v, stdout)
		}
		if stderr != "" {
			t.Errorf("verbose=%v: expected empty stderr when sync disabled, got %q", v, stderr)
		}
	}
}
