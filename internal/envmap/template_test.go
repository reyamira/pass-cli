package envmap

import (
	"errors"
	"fmt"
	"testing"
)

// echoResolver resolves each ref to "<service>:<field-or-default>", so tests can
// assert both substitution and that field defaulting is left to the caller.
func echoResolver(refs []TemplateRef) ([]string, error) {
	out := make([]string, len(refs))
	for i, r := range refs {
		field := r.Field
		if field == "" {
			field = "password"
		}
		out[i] = r.Service + ":" + field
	}
	return out, nil
}

func TestRenderTemplate_Substitutes(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"no refs here", "no refs here"},
		{"${pass:github}", "github:password"},
		{"${pass:github/username}", "github:username"},
		{"postgres://u:${pass:db/password}@host/db", "postgres://u:db:password@host/db"},
		{"a ${pass:x} b ${pass:y/url} c", "a x:password b y:url c"},
		// non-pass expansions pass through untouched:
		{"$HOME ${HOME} $(id) ${pass:x}", "$HOME ${HOME} $(id) x:password"},
		{"literal $ and ${notpass:foo}", "literal $ and ${notpass:foo}"},
	}
	for _, tt := range tests {
		got, err := RenderTemplate(tt.in, echoResolver)
		if err != nil {
			t.Errorf("RenderTemplate(%q) error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("RenderTemplate(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRenderTemplate_SinglePass proves a resolved value containing ${pass:...} is
// NOT re-scanned — the classic template-injection guard.
func TestRenderTemplate_SinglePass(t *testing.T) {
	resolve := func(refs []TemplateRef) ([]string, error) {
		out := make([]string, len(refs))
		for i, r := range refs {
			if r.Service == "evil" {
				out[i] = "${pass:secret/password}" // must appear literally, not re-resolved
			} else {
				out[i] = "RESOLVED-" + r.Service
			}
		}
		return out, nil
	}
	got, err := RenderTemplate("x=${pass:evil}", resolve)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != "x=${pass:secret/password}" {
		t.Errorf("single-pass violated: got %q", got)
	}
}

// TestRenderTemplate_FailClosed verifies a resolve error or malformed reference
// aborts the whole render (no partial output).
func TestRenderTemplate_FailClosed(t *testing.T) {
	boom := errors.New("boom")
	failing := func([]TemplateRef) ([]string, error) { return nil, boom }
	if _, err := RenderTemplate("a ${pass:x} b", failing); !errors.Is(err, boom) {
		t.Errorf("expected resolve error to propagate, got %v", err)
	}

	malformed := []string{
		"${pass:}",            // empty
		"${pass:svc/}",        // empty field
		"${pass:/field}",      // empty service
		"${pass:a/b/c}",       // multi-segment reserved
		"${pass:unterminated", // no closing brace
	}
	for _, m := range malformed {
		if _, err := RenderTemplate(m, echoResolver); err == nil {
			t.Errorf("expected error for %q, got nil", m)
		}
	}
}

// TestRenderTemplate_BatchesRefs verifies all references are resolved in a single
// call (Phase 2's socket does one round-trip), and in template order.
func TestRenderTemplate_BatchesRefs(t *testing.T) {
	var calls int
	var seen []TemplateRef
	resolve := func(refs []TemplateRef) ([]string, error) {
		calls++
		seen = refs
		out := make([]string, len(refs))
		for i := range refs {
			out[i] = fmt.Sprintf("v%d", i)
		}
		return out, nil
	}
	got, err := RenderTemplate("${pass:a} ${pass:b/username} ${pass:c}", resolve)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if calls != 1 {
		t.Errorf("resolveAll called %d times, want 1 (must batch)", calls)
	}
	if len(seen) != 3 || seen[0].Service != "a" || seen[1].Service != "b" || seen[1].Field != "username" || seen[2].Service != "c" {
		t.Errorf("refs not collected in order: %+v", seen)
	}
	if got != "v0 v1 v2" {
		t.Errorf("got %q", got)
	}
}
