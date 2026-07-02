package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arimxyer/pass-cli/internal/skills"
)

func TestInstallStub(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "pass-cli", "SKILL.md")
	stub, err := skills.StubContent()
	if err != nil {
		t.Fatalf("StubContent: %v", err)
	}

	// Fresh install writes the stub verbatim.
	msg, err := installStub(dir, false)
	if err != nil {
		t.Fatalf("fresh install: %v", err)
	}
	if !strings.Contains(msg, "Installed discovery stub") {
		t.Errorf("fresh install message = %q", msg)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading written stub: %v", err)
	}
	if string(got) != stub {
		t.Error("written stub does not match StubContent()")
	}

	// Re-install with identical content is an idempotent no-op.
	msg, err = installStub(dir, false)
	if err != nil {
		t.Fatalf("idempotent install: %v", err)
	}
	if !strings.Contains(msg, "Already up to date") {
		t.Errorf("idempotent message = %q", msg)
	}

	// A differing existing stub is preserved unless --force.
	if err := os.WriteFile(target, []byte("changed by user\n"), 0o644); err != nil {
		t.Fatalf("mutating stub: %v", err)
	}
	if _, err := installStub(dir, false); err == nil {
		t.Error("install without force overwrote a differing stub")
	}
	if kept, _ := os.ReadFile(target); string(kept) != "changed by user\n" {
		t.Error("differing stub was clobbered despite no --force")
	}

	// --force overwrites it back to the canonical stub.
	if _, err := installStub(dir, true); err != nil {
		t.Fatalf("force install: %v", err)
	}
	if forced, _ := os.ReadFile(target); string(forced) != stub {
		t.Error("--force did not restore the canonical stub")
	}
}

func TestSummaryLine(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// First sentence only — no mid-word truncation.
		{"Core guide for driving pass-cli safely. Read this before anything.", "Core guide for driving pass-cli safely."},
		// No sentence break → returned as-is (within the cap).
		{"A single clause with no period", "A single clause with no period"},
		{"  leading/trailing space trimmed. rest ", "leading/trailing space trimmed."},
	}
	for _, c := range cases {
		if got := summaryLine(c.in); got != c.want {
			t.Errorf("summaryLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"short", 100, "short"},
		{"", 5, ""},
		{"abcdef", 4, "abc…"},
		{"abc", 3, "abc"},
		{"abcd", 0, ""},
		// Multi-byte runes must not be split mid-character.
		{"héllo wörld", 5, "héll…"},
	}
	for _, c := range cases {
		if got := truncate(c.in, c.max); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
	}
}
