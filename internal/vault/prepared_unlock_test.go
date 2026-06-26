package vault

import (
	"path/filepath"
	"testing"
)

const (
	tier2Pass    = "TestPass!1234"
	tier2NewPass = "NewPass!5678"
)

func initVaultForTier2(t *testing.T) (string, *VaultService) {
	t.Helper()
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := v.Initialize([]byte(tier2Pass), false, "", ""); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	// Initialize does not leave the vault unlocked; unlock so callers can
	// ChangePassword on the returned service.
	if err := v.Unlock([]byte(tier2Pass)); err != nil {
		t.Fatalf("Unlock after init: %v", err)
	}
	return vaultPath, v
}

// Happy path: parameters unchanged since derivation → the prepared key is used,
// the vault unlocks, masterPassword is retained (a subsequent save works), and it
// is NOT a recovery unlock.
func TestUnlockWithPreparedKey_HappyPath(t *testing.T) {
	vaultPath, _ := initVaultForTier2(t)

	// Fresh, locked service (as a command would have).
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New(locked): %v", err)
	}

	prep, err := v.PrepareUnlock()
	if err != nil {
		t.Fatalf("PrepareUnlock: %v", err)
	}
	dataKey, err := prep.DeriveDataKey([]byte(tier2Pass))
	if err != nil {
		t.Fatalf("DeriveDataKey: %v", err)
	}

	if err := v.UnlockWithPreparedKey(prep, dataKey, []byte(tier2Pass)); err != nil {
		t.Fatalf("UnlockWithPreparedKey: %v", err)
	}
	if !v.IsUnlocked() {
		t.Fatal("expected vault unlocked")
	}
	// masterPassword must be retained (not a recovery unlock) so saves work.
	if err := v.AddCredential("svc", "user", []byte("secret"), "", "", ""); err != nil {
		t.Errorf("AddCredential after prepared-key unlock failed (masterPassword not retained?): %v", err)
	}
	if v.WasUnlockedViaRecovery() {
		t.Error("prepared-key unlock must not be flagged as a recovery unlock")
	}
}

// Re-key fallback: the vault is re-keyed (password change) AFTER the params were
// captured, so the prepared key is stale. UnlockWithPreparedKey must detect the
// param mismatch and fall back to a full unlock with the CURRENT password —
// succeeding, identical to a sequential unlock.
func TestUnlockWithPreparedKey_RekeyFallsBackToCurrentPassword(t *testing.T) {
	vaultPath, v1 := initVaultForTier2(t)

	// Capture params + derive against the ORIGINAL key (v1 is unlocked from init).
	prep, err := v1.PrepareUnlock()
	if err != nil {
		t.Fatalf("PrepareUnlock: %v", err)
	}
	dataKey, err := prep.DeriveDataKey([]byte(tier2Pass))
	if err != nil {
		t.Fatalf("DeriveDataKey: %v", err)
	}

	// Re-key on disk (new salt + wrapped key) via the still-unlocked v1.
	if err := v1.ChangePassword([]byte(tier2NewPass)); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	// Fresh locked service unlocks with the stale prepared key + the NEW password.
	v2, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New(locked): %v", err)
	}
	if err := v2.UnlockWithPreparedKey(prep, dataKey, []byte(tier2NewPass)); err != nil {
		t.Fatalf("expected fallback unlock to succeed with current password, got: %v", err)
	}
	if !v2.IsUnlocked() {
		t.Fatal("expected vault unlocked via fallback")
	}
}

// Security edge (the reason for param-comparison over decrypt-failure): after a
// remote re-key, a STALE keychain password must fail cleanly at unlock — never
// unlock-then-break-saves. The param mismatch routes to the fallback, which fails
// because the stale password can't derive the current key.
func TestUnlockWithPreparedKey_StalePasswordFailsCleanly(t *testing.T) {
	vaultPath, v1 := initVaultForTier2(t)

	prep, err := v1.PrepareUnlock()
	if err != nil {
		t.Fatalf("PrepareUnlock: %v", err)
	}
	dataKey, err := prep.DeriveDataKey([]byte(tier2Pass))
	if err != nil {
		t.Fatalf("DeriveDataKey: %v", err)
	}

	if err := v1.ChangePassword([]byte(tier2NewPass)); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	v2, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New(locked): %v", err)
	}
	// Pass the OLD (now stale) password — must fail, not silently unlock.
	if err := v2.UnlockWithPreparedKey(prep, dataKey, []byte(tier2Pass)); err == nil {
		t.Fatal("expected stale-password unlock to fail, but it succeeded")
	}
	if v2.IsUnlocked() {
		t.Error("vault must remain locked after a stale-password unlock attempt")
	}
}
