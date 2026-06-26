package cmd

import (
	"strings"
	"testing"

	"github.com/arimxyer/pass-cli/internal/vault"
)

// captureStdout is defined in exec_test.go (same package) and shared across the
// cmd test suite; it redirects os.Stdout for the duration of fn and returns what
// was written, which is how the list output functions are observed here.

func sampleMetadata() []vault.CredentialMetadata {
	return []vault.CredentialMetadata{
		{Service: "github", Username: "4111-1111-1111-1111", UsageCount: 0},
		{Service: "aws", Username: "routing-021000021", UsageCount: 3},
	}
}

// #95: by default the table must NOT include the USERNAME column, because the
// username field can hold sensitive values.
func TestOutputTable_HidesUsernameByDefault(t *testing.T) {
	defer func() { listShowUsernames = false }()
	listShowUsernames = false

	out := captureStdout(t, func() {
		if err := outputTable(sampleMetadata()); err != nil {
			t.Fatalf("outputTable returned error: %v", err)
		}
	})

	upper := strings.ToUpper(out)
	if strings.Contains(upper, "USERNAME") {
		t.Errorf("default table must not contain a USERNAME header; got:\n%s", out)
	}
	// The sensitive username values themselves must not leak into the table.
	if strings.Contains(out, "4111-1111-1111-1111") || strings.Contains(out, "routing-021000021") {
		t.Errorf("default table leaked a username value; got:\n%s", out)
	}
	// Sanity: the service names should still be present.
	if !strings.Contains(out, "github") || !strings.Contains(out, "aws") {
		t.Errorf("expected service names in table output; got:\n%s", out)
	}
}

// #95: --show-usernames reintroduces the USERNAME column.
func TestOutputTable_ShowUsernamesAddsColumn(t *testing.T) {
	defer func() { listShowUsernames = false }()
	listShowUsernames = true

	out := captureStdout(t, func() {
		if err := outputTable(sampleMetadata()); err != nil {
			t.Fatalf("outputTable returned error: %v", err)
		}
	})

	upper := strings.ToUpper(out)
	if !strings.Contains(upper, "USERNAME") {
		t.Errorf("--show-usernames must add a USERNAME header; got:\n%s", out)
	}
	if !strings.Contains(out, "4111-1111-1111-1111") {
		t.Errorf("--show-usernames must show username values; got:\n%s", out)
	}
}

// #95: -q/--quiet prints bare service names, one per line, and ignores --format.
func TestQuietOutputsBareServiceNames(t *testing.T) {
	metadata := sampleMetadata()

	out := captureStdout(t, func() {
		if err := outputSimple(metadata); err != nil {
			t.Fatalf("outputSimple returned error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != len(metadata) {
		t.Fatalf("expected %d lines, got %d: %q", len(metadata), len(lines), out)
	}
	for i, meta := range metadata {
		if lines[i] != meta.Service {
			t.Errorf("line %d: expected bare service %q, got %q", i, meta.Service, lines[i])
		}
	}
	// Quiet output must never contain table chrome or usernames.
	if strings.Contains(out, "Usage") || strings.Contains(out, "Created") {
		t.Errorf("quiet output must not contain table headers; got:\n%s", out)
	}
	if strings.Contains(out, "4111-1111-1111-1111") {
		t.Errorf("quiet output leaked a username value; got:\n%s", out)
	}
}

// #95: -q takes precedence over --format. Exercises the shipped resolveListFormat
// helper that runList uses for dispatch.
func TestQuietTakesPrecedenceOverFormat(t *testing.T) {
	tests := []struct {
		name   string
		format string
		quiet  bool
		want   string
	}{
		{name: "quiet overrides table", format: "table", quiet: true, want: "simple"},
		{name: "quiet overrides json", format: "json", quiet: true, want: "simple"},
		{name: "no quiet keeps table", format: "table", quiet: false, want: "table"},
		{name: "no quiet keeps json", format: "json", quiet: false, want: "json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveListFormat(tt.format, tt.quiet); got != tt.want {
				t.Errorf("resolveListFormat(%q, %v) = %q, want %q", tt.format, tt.quiet, got, tt.want)
			}
		})
	}
}

// #95: the -q/--quiet and --show-usernames flags are registered on the list command.
func TestListFlagsRegistered(t *testing.T) {
	if listCmd.Flags().Lookup("show-usernames") == nil {
		t.Error("--show-usernames flag should be registered on list")
	}
	q := listCmd.Flags().ShorthandLookup("q")
	if q == nil {
		t.Error("-q shorthand should be registered on list")
	} else if q.Name != "quiet" {
		t.Errorf("-q shorthand should map to --quiet, got %q", q.Name)
	}
}
