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

// Resolver materializes credential mappings into their field values.
type Resolver interface {
	// ResolveValues returns one value per mapping, in order. A mapping with an
	// empty Field falls back to defaultField; EnvName is ignored (callers that need
	// "NAME=value" use ResolveEnv). It is a single batch call — the socket backend
	// resolves N mappings in one round-trip — and read-only: no usage tracking, no
	// sync push.
	ResolveValues(mappings []envmap.Mapping, defaultField string) ([]string, error)
	// Close releases any backend resources. For the direct backend the caller owns
	// the vault's lifecycle, so Close is a no-op.
	Close() error
}

// ResolveEnv is a convenience over ResolveValues that returns "NAME=value" entries,
// one per mapping. Building the entry here (rather than in the backend) keeps the
// resolver primitive value-only, so template rendering can reuse ResolveValues
// without parsing a "NAME=" prefix back off.
func ResolveEnv(r Resolver, mappings []envmap.Mapping, defaultField string) ([]string, error) {
	values, err := r.ResolveValues(mappings, defaultField)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(mappings))
	for i, m := range mappings {
		out[i] = m.EnvName + "=" + values[i]
	}
	return out, nil
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

func (d *directResolver) ResolveValues(mappings []envmap.Mapping, defaultField string) ([]string, error) {
	out := make([]string, 0, len(mappings))
	for _, m := range mappings {
		// A per-mapping field (service/field) overrides the caller's default field.
		field := m.Field
		if field == "" {
			field = defaultField
		}
		value, err := resolveOne(d.vs, m.Service, field)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

// Close is a no-op: the direct backend does not own the vault.
func (d *directResolver) Close() error { return nil }

// resolveOne fetches one credential and returns the requested field's value. The
// credential's secret bytes are cleared before the function returns; the field
// value is read (copied into a new string) first, so the wiped bytes never affect
// the returned value.
func resolveOne(vs *vault.VaultService, service, field string) (string, error) {
	// GetCredential returns a deep copy, so clearing Password does not touch the vault.
	cred, err := vs.GetCredential(service, false)
	if err != nil {
		return "", fmt.Errorf("failed to get credential %q: %w", service, err)
	}
	defer crypto.ClearBytes(cred.Password)

	value, _, err := vault.ResolveCredentialField(cred, field)
	if err != nil {
		return "", err
	}
	return value, nil
}
