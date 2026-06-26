package cmd

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/arimxyer/pass-cli/internal/vault"
)

// feedStdin redirects os.Stdin to a pipe carrying input (one line) for the
// duration of the test, and enables PASS_CLI_TEST so readPassword reads from it
// via the shared test scanner instead of a TTY. Only one password read per test
// process is safe (the scanner initializes once), so keep a single reader here.
func feedStdin(t *testing.T, input string) {
	t.Helper()
	t.Setenv("PASS_CLI_TEST", "1")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig; _ = r.Close() })

	go func() {
		_, _ = io.WriteString(w, input)
		_ = w.Close()
	}()
}

// unlockVaultWithSync's password branch overlaps the network pull (goroutine)
// with the master-password prompt (main thread), then joins before decrypting.
// This drives that branch end-to-end against a real, sync-enabled, initialized
// vault with no keychain — exercising the concurrency so `go test -race` proves
// the goroutine + join is data-race free and the vault still unlocks correctly.
func TestUnlockVaultWithSync_PasswordBranchConcurrent(t *testing.T) {
	const masterPassword = "TestPass!1234"

	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")
	cfgPath := filepath.Join(tmpDir, "config.yml")

	// Sync enabled with a bogus remote: SyncPull fails fast / no-ops (offline-
	// friendly) and returns quickly, so the goroutine joins without real network.
	cfg := "vault_path: " + vaultPath + "\nsync:\n  enabled: true\n  remote: \"mock-remote:bucket\"\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("PASS_CLI_CONFIG", cfgPath)

	// Initialize the vault (no keychain) with a first service.
	initVS, err := vault.New(vaultPath)
	if err != nil {
		t.Fatalf("vault.New (init): %v", err)
	}
	if err := initVS.Initialize([]byte(masterPassword), false, "", ""); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// A fresh, locked service is what a command would unlock.
	vs, err := vault.New(vaultPath)
	if err != nil {
		t.Fatalf("vault.New (locked): %v", err)
	}
	if !vs.IsSyncEnabled() {
		t.Fatal("expected sync to be enabled for the concurrent branch")
	}

	setOffline(t, false)
	setVerbose(t, false)
	feedStdin(t, masterPassword+"\n")

	if err := unlockVaultWithSync(vs); err != nil {
		t.Fatalf("unlockVaultWithSync: %v", err)
	}
	if !vs.IsUnlocked() {
		t.Error("expected vault to be unlocked after the concurrent password branch")
	}
}
