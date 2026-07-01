// Package envmap holds the shared grammar for mapping stored credentials to
// environment variables: the Mapping type, env-name derivation, and the parser
// for a single "NAME=service[:field]" spec.
//
// It is the single home for this grammar so that every surface that consumes it
// — `exec --set`, the project manifest, and (later) `${pass:...}` templates —
// parses the same way. Phase 0a extracts today's colon grammar verbatim; Phase 0b
// adds the slash-delimited form additively (see the plan's §3.1).
package envmap

import (
	"fmt"
	"strings"
)

// Mapping pairs a target environment variable name with the credential service
// whose field value should be injected into it. Field is the optional per-mapping
// field override ("" means fall back to the caller's default field).
type Mapping struct {
	EnvName string
	Service string
	Field   string
}

// ParseSetSpec parses a single "NAME=service[/field]" spec into a Mapping. The
// path is split by SplitPath: slash is the preferred separator, with the legacy
// colon form still accepted. An empty name or empty service is an error.
func ParseSetSpec(spec string) (Mapping, error) {
	name, rest, ok := strings.Cut(spec, "=")
	if !ok || name == "" || rest == "" {
		return Mapping{}, fmt.Errorf("invalid mapping %q: expected NAME=service[/field]", spec)
	}
	service, field, err := SplitPath(rest)
	if err != nil {
		return Mapping{}, fmt.Errorf("invalid mapping %q: %w", spec, err)
	}
	return Mapping{EnvName: name, Service: service, Field: field}, nil
}

// SplitPath splits a "service[/field]" credential reference into its service and
// optional field. It is the single home for the path separator, shared by
// --set, the project manifest, and ${pass:...} templates.
//
//   - Slash is preferred and wins: if the reference contains '/', it is split on
//     '/', and any ':' in it is a literal character (the fragility fix). Exactly
//     one slash — two segments, service/field — is accepted for now; three or
//     more segments error, reserving vault/service/field for a future multi-vault.
//   - Otherwise the legacy colon form applies, byte-for-byte the original
//     behavior: the first ':' separates service from an optional field.
//   - With no separator at all, the whole reference is the service and the field
//     is empty (the caller falls back to its default field).
func SplitPath(ref string) (service, field string, err error) {
	if strings.Contains(ref, "/") {
		segs := strings.Split(ref, "/")
		if len(segs) > 2 {
			return "", "", fmt.Errorf("multi-segment paths not yet supported (expected service/field): %q", ref)
		}
		service, field = segs[0], segs[1]
		if service == "" || field == "" {
			return "", "", fmt.Errorf("expected service/field: %q", ref)
		}
		return service, field, nil
	}

	// Legacy colon form: the first ':' separates the service from the field, so a
	// field always wins when present.
	service, field, hasField := strings.Cut(ref, ":")
	if hasField && (service == "" || field == "") {
		return "", "", fmt.Errorf("expected service:field: %q", ref)
	}
	return service, field, nil
}

// DeriveEnvName converts a service name into an environment variable name:
// uppercased, with every non-alphanumeric ASCII character replaced by '_'.
// e.g. "openai-api" -> "OPENAI_API".
func DeriveEnvName(service string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(service) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
