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

// ParseSetSpec parses a single "NAME=service[:field]" spec into a Mapping. The
// first ':' separates the service from an optional field override, so a field
// always wins when present. An empty name, empty service, or (when a ':' is
// present) empty service/field is an error.
func ParseSetSpec(spec string) (Mapping, error) {
	name, rest, ok := strings.Cut(spec, "=")
	if !ok || name == "" || rest == "" {
		return Mapping{}, fmt.Errorf("invalid mapping %q: expected NAME=service[:field]", spec)
	}
	// Optional per-mapping field override: service:field. The first ':' separates
	// the service from the field, so a field always wins when present.
	service, field, hasField := strings.Cut(rest, ":")
	if hasField && (service == "" || field == "") {
		return Mapping{}, fmt.Errorf("invalid mapping %q: expected NAME=service:field", spec)
	}
	return Mapping{EnvName: name, Service: service, Field: field}, nil
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
