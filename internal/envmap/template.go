package envmap

import (
	"fmt"
	"strings"
)

// TemplateRef is one ${pass:service[/field][ | filter]} reference found in a template.
type TemplateRef struct {
	Service string
	Field   string // "" means the caller's default field
	Filter  string // optional value transform (e.g. "base64"); "" = none
}

const passMarker = "${pass:"

// parseTemplate splits s into literal segments and the ${pass:...} references
// between them. literals has exactly len(refs)+1 entries, so the rendered output
// is literals[0] + values[0] + literals[1] + ... + literals[len-1]. Only
// "${pass:" is special; every other byte ($FOO, ${HOME}, $(...), a bare $) is
// literal. A malformed or unterminated reference is a hard error.
func parseTemplate(s string) (literals []string, refs []TemplateRef, err error) {
	i := 0
	for {
		idx := strings.Index(s[i:], passMarker)
		if idx < 0 {
			literals = append(literals, s[i:])
			return literals, refs, nil
		}
		start := i + idx
		literals = append(literals, s[i:start])

		rest := s[start+len(passMarker):]
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return nil, nil, fmt.Errorf("unterminated %s...} reference: %q", passMarker, s[start:])
		}
		inner := rest[:end]
		service, field, filter, e := SplitPath(inner)
		if e != nil {
			return nil, nil, fmt.Errorf("invalid reference %s%s}: %w", passMarker, inner, e)
		}
		if service == "" {
			return nil, nil, fmt.Errorf("empty service in reference %s%s}", passMarker, inner)
		}
		refs = append(refs, TemplateRef{Service: service, Field: field, Filter: filter})
		i = start + len(passMarker) + end + 1
	}
}

// RenderTemplate substitutes every ${pass:service[/field]} reference in s with a
// resolved value. It is single-pass by construction: all references are collected
// from the ORIGINAL template and resolved in one batch call, then substituted; a
// resolved value that itself contains "${pass:...}" is never re-scanned. Any
// resolve error, or a malformed/unknown reference, fails the whole render and
// emits nothing (fail closed) — never a silent empty substitution.
func RenderTemplate(s string, resolveAll func([]TemplateRef) ([]string, error)) (string, error) {
	literals, refs, err := parseTemplate(s)
	if err != nil {
		return "", err
	}
	if len(refs) == 0 {
		return s, nil
	}
	values, err := resolveAll(refs)
	if err != nil {
		return "", err
	}
	if len(values) != len(refs) {
		return "", fmt.Errorf("resolver returned %d values for %d references", len(values), len(refs))
	}

	var b strings.Builder
	for k := range refs {
		b.WriteString(literals[k])
		b.WriteString(values[k])
	}
	b.WriteString(literals[len(literals)-1])
	return b.String(), nil
}
