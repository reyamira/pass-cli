package vault

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Path returns the vault file path this service operates on.
func (v *VaultService) Path() string { return v.vaultPath }

// Reload re-reads and re-decrypts the on-disk vault with the currently-held master
// password, replacing the in-memory snapshot. The background agent's revalidating
// cache calls it when vault.enc changed on disk (e.g. a sibling process wrote it),
// so the agent serves fresh data rather than a stale snapshot.
//
// It is read-only with respect to the vault file. If the on-disk vault can no
// longer be decrypted with the held password — for example the master password was
// rotated by another process — it returns an error and never serves a stale
// snapshot; the caller (the agent) locks on that error.
func (v *VaultService) Reload() error {
	if !v.unlocked {
		return errors.New("vault is locked")
	}
	// string() copies the bytes, so the held masterPassword is not cleared (same
	// hygiene as Unlock's LoadVault call).
	data, err := v.storageService.LoadVault(string(v.masterPassword))
	if err != nil {
		return fmt.Errorf("failed to reload vault: %w", err)
	}
	var vaultData VaultData
	if err := json.Unmarshal(data, &vaultData); err != nil {
		return fmt.Errorf("failed to parse reloaded vault data: %w", err)
	}
	v.vaultData = &vaultData
	return nil
}
