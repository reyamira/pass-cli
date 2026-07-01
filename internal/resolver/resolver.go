// Package resolver materializes credential mappings into "NAME=value" env entries.
//
// It is the substrate shared by every injection surface (exec today; export, run,
// and inject later). A Resolver has two interchangeable backends: directResolver,
// which reads an already-unlocked in-process vault, and — once the agent exists —
// a socket backend that asks a resident daemon. Consumers try the socket and fall
// back to direct-open, so the daemon is a transparent optimization.
//
// Resolve is read-only by contract: it never records field access and never
// triggers a sync push, preserving exec's deliberate no-write semantics for every
// consumer.
package resolver

import (
	"fmt"

	"github.com/arimxyer/pass-cli/internal/crypto"
	"github.com/arimxyer/pass-cli/internal/envmap"
	"github.com/arimxyer/pass-cli/internal/vault"
)

// Resolver materializes credential mappings into "NAME=value" strings.
type Resolver interface {
	// Resolve turns each mapping into "NAME=value". A mapping with an empty Field
	// falls back to defaultField. It is read-only: no usage tracking, no sync push.
	Resolve(mappings []envmap.Mapping, defaultField string) ([]string, error)
	// Close releases any backend resources. For the direct backend the caller owns
	// the vault's lifecycle, so Close is a no-op.
	Close() error
}

// directResolver resolves against an already-unlocked, in-process VaultService.
// The caller owns unlock/Lock; this type only reads.
type directResolver struct {
	vs *vault.VaultService
}

// NewDirect returns a Resolver backed by an already-unlocked vault. The caller
// remains responsible for locking the vault when done.
func NewDirect(vs *vault.VaultService) Resolver {
	return &directResolver{vs: vs}
}

func (d *directResolver) Resolve(mappings []envmap.Mapping, defaultField string) ([]string, error) {
	out := make([]string, 0, len(mappings))
	for _, m := range mappings {
		// A per-mapping field (service:field) overrides the caller's default field.
		field := m.Field
		if field == "" {
			field = defaultField
		}
		entry, err := buildEnvEntry(d.vs, m, field)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

// Close is a no-op: the direct backend does not own the vault.
func (d *directResolver) Close() error { return nil }

// buildEnvEntry fetches one credential, resolves the requested field, and returns
// the "NAME=value" string. The credential's secret bytes are cleared before the
// function returns; the field value is read first, so the wiped bytes never affect
// the returned string.
func buildEnvEntry(vs *vault.VaultService, m envmap.Mapping, field string) (string, error) {
	// GetCredential returns a deep copy, so clearing Password does not touch the vault.
	cred, err := vs.GetCredential(m.Service, false)
	if err != nil {
		return "", fmt.Errorf("failed to get credential %q: %w", m.Service, err)
	}
	defer crypto.ClearBytes(cred.Password)

	value, _, err := vault.ResolveCredentialField(cred, field)
	if err != nil {
		return "", err
	}
	return m.EnvName + "=" + value, nil
}
