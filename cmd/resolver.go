package cmd

import (
	"fmt"
	"os"

	"github.com/arimxyer/pass-cli/internal/agent"
	"github.com/arimxyer/pass-cli/internal/resolver"
	"github.com/arimxyer/pass-cli/internal/vault"
)

// acquireResolver returns a read-only credential resolver, preferring a running
// agent (no prompt, no PBKDF2) and transparently falling back to opening and
// unlocking the local vault. The returned cleanup func must always be called; it
// closes the socket or locks the freshly-unlocked vault as appropriate.
//
// This is what makes the daemon an optimization rather than a dependency: every
// injection command (exec/export/inject) calls it and behaves identically whether
// or not an agent is running.
func acquireResolver() (resolver.Resolver, func(), error) {
	if r, ok := agent.DialResolver(); ok {
		return r, func() { _ = r.Close() }, nil
	}

	vaultPath := GetVaultPath()
	if _, err := os.Stat(vaultPath); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("vault not found at %s\nRun 'pass-cli init' to create a vault first", vaultPath)
	}
	vaultService, err := vault.New(vaultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create vault service at %s: %w", vaultPath, err)
	}
	// Pull from remote and unlock, overlapping the pull with the password prompt
	// (#103; read-only: no push after).
	if err := unlockVaultWithSync(vaultService); err != nil {
		return nil, nil, err
	}

	r := resolver.NewDirect(vaultService)
	cleanup := func() {
		_ = r.Close()
		vaultService.Lock()
	}
	return r, cleanup, nil
}
