package vault

import (
	"errors"
	"path/filepath"
	"testing"
)

// RetrieveKeychainPassword must report ErrKeychainNotEnabled (and return no
// bytes) when keychain unlock isn't configured — the signal the concurrent
// unlock path uses to fall through to the password prompt.
func TestRetrieveKeychainPassword_NotEnabled(t *testing.T) {
	tempDir := t.TempDir()
	vaultPath := filepath.Join(tempDir, "vault.enc")

	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := v.Initialize([]byte("TestPass!1234"), false, "", ""); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	pw, err := v.RetrieveKeychainPassword()
	if !errors.Is(err, ErrKeychainNotEnabled) {
		t.Errorf("expected ErrKeychainNotEnabled, got err=%v", err)
	}
	if pw != nil {
		t.Errorf("expected nil password on error, got %d bytes", len(pw))
	}
}

// The conflict getter defaults to false on a fresh service and is what the
// concurrent unlock path reads after the join to re-surface a swallowed conflict.
func TestSyncConflictDetected_DefaultsFalse(t *testing.T) {
	tempDir := t.TempDir()
	v, err := New(filepath.Join(tempDir, "vault.enc"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if v.SyncConflictDetected() {
		t.Error("expected SyncConflictDetected() == false on a fresh service")
	}
}
