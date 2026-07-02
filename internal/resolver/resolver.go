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
	values, err := ResolveValuesFiltered(r, mappings, defaultField)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(mappings))
	for i, m := range mappings {
		out[i] = m.EnvName + "=" + values[i]
	}
	return out, nil
}

// ResolveValuesFiltered resolves each mapping's value and applies its Filter,
// returning one value per mapping in order. It is the filtered counterpart to
// Resolver.ResolveValues used by every injection surface (--set, manifests, and
// ${pass:...} templates).
//
// Filters are applied client-side, AFTER the backend returns raw values, so the
// agent's values-only protocol is untouched — base64 is identical whether a
// direct vault or a resident daemon served the value. Most mappings resolve one
// field; a "basicauth" mapping resolves TWO fields (username and password) of its
// service and combines them into base64("username:password"). Both sub-resolves
// go through the normal value path, so a socket backend still answers the whole
// request in one batch round-trip.
func ResolveValuesFiltered(r Resolver, mappings []envmap.Mapping, defaultField string) ([]string, error) {
	// Expand each mapping into its sub-requests (1, or 2 for basicauth), recording
	// how to fold the results back into one value per mapping.
	type plan struct {
		filter string
		start  int // index of this mapping's first value in the flat result
	}
	subs := make([]envmap.Mapping, 0, len(mappings))
	plans := make([]plan, len(mappings))
	for i, m := range mappings {
		plans[i] = plan{filter: m.Filter, start: len(subs)}
		if m.Filter == envmap.FilterBasicAuth {
			subs = append(subs,
				envmap.Mapping{Service: m.Service, Field: "username"},
				envmap.Mapping{Service: m.Service, Field: "password"},
			)
			continue
		}
		subs = append(subs, envmap.Mapping{Service: m.Service, Field: m.Field})
	}

	values, err := r.ResolveValues(subs, defaultField)
	if err != nil {
		return nil, err
	}

	out := make([]string, len(mappings))
	for i, p := range plans {
		switch p.filter {
		case "":
			out[i] = values[p.start]
		case envmap.FilterBasicAuth:
			// HTTP Basic auth: base64("username:password") (standard encoding).
			out[i], err = envmap.ApplyValueFilter("base64", values[p.start]+":"+values[p.start+1])
		default:
			out[i], err = envmap.ApplyValueFilter(p.filter, values[p.start])
		}
		if err != nil {
			return nil, err
		}
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
