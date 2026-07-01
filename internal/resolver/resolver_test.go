package resolver

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/arimxyer/pass-cli/internal/envmap"
	"github.com/arimxyer/pass-cli/internal/vault"
)

// newUnlockedVault creates a temp vault with a single "github" credential and
// returns the unlocked service plus its file path. useKeychain=false keeps it off
// the OS keyring so it runs in CI.
func newUnlockedVault(t *testing.T) (*vault.VaultService, string) {
	t.Helper()
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.enc")
	auditPath := filepath.Join(dir, "audit.log")

	vs, err := vault.New(vaultPath)
	if err != nil {
		t.Fatalf("vault.New: %v", err)
	}
	// Initialize clears the password bytes, so Unlock gets its own copy.
	if err := vs.Initialize([]byte("TestPassword123!"), false, auditPath, "test-vault-id"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := vs.Unlock([]byte("TestPassword123!")); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if err := vs.AddCredential("github", "octocat", []byte("s3cr3t-pw"), "vcs", "https://github.com", "note"); err != nil {
		t.Fatalf("AddCredential: %v", err)
	}
	return vs, vaultPath
}

// TestDirectResolver_ResolvesFields covers the default field, a per-mapping field
// override, and the "NAME=value" shape.
func TestDirectResolver_ResolvesFields(t *testing.T) {
	vs, _ := newUnlockedVault(t)
	defer vs.Lock()

	r := NewDirect(vs)
	defer func() { _ = r.Close() }()

	got, err := ResolveEnv(r, []envmap.Mapping{
		{EnvName: "GITHUB_TOKEN", Service: "github"},               // default field
		{EnvName: "GH_USER", Service: "github", Field: "username"}, // per-mapping override
	}, "password")
	if err != nil {
		t.Fatalf("ResolveEnv: %v", err)
	}

	want := []string{"GITHUB_TOKEN=s3cr3t-pw", "GH_USER=octocat"}
	if len(got) != len(want) {
		t.Fatalf("Resolve returned %d entries, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestDirectResolver_DefaultFieldFallback verifies the defaultField argument is
// applied when a mapping has no per-mapping field.
func TestDirectResolver_DefaultFieldFallback(t *testing.T) {
	vs, _ := newUnlockedVault(t)
	defer vs.Lock()

	r := NewDirect(vs)
	defer func() { _ = r.Close() }()

	got, err := ResolveEnv(r, []envmap.Mapping{{EnvName: "GH", Service: "github"}}, "username")
	if err != nil {
		t.Fatalf("ResolveEnv: %v", err)
	}
	if len(got) != 1 || got[0] != "GH=octocat" {
		t.Errorf("got %v, want [GH=octocat]", got)
	}
}

// TestDirectResolver_ResolveValuesOrder verifies ResolveValues returns bare values
// in mapping order (the primitive the template renderer builds on).
func TestDirectResolver_ResolveValuesOrder(t *testing.T) {
	vs, _ := newUnlockedVault(t)
	defer vs.Lock()

	r := NewDirect(vs)
	defer func() { _ = r.Close() }()

	got, err := r.ResolveValues([]envmap.Mapping{
		{Service: "github", Field: "username"},
		{Service: "github"}, // default field
	}, "password")
	if err != nil {
		t.Fatalf("ResolveValues: %v", err)
	}
	want := []string{"octocat", "s3cr3t-pw"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("ResolveValues = %v, want %v", got, want)
	}
}

// TestDirectResolver_ReadOnly_DoesNotMutateVault is the load-bearing guarantee:
// resolving must never write the vault file. A write would change the vault hash
// and trigger a sync push on the hot path — exactly the slowness #120 removed.
func TestDirectResolver_ReadOnly_DoesNotMutateVault(t *testing.T) {
	vs, vaultPath := newUnlockedVault(t)
	defer vs.Lock()

	before, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatalf("read vault before: %v", err)
	}

	r := NewDirect(vs)
	defer func() { _ = r.Close() }()
	if _, err := r.ResolveValues([]envmap.Mapping{{EnvName: "GH", Service: "github"}}, "password"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	after, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatalf("read vault after: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("Resolve mutated the vault file; the resolve path must be strictly read-only")
	}
}

// TestDirectResolver_UnknownService surfaces the vault error for a missing service.
func TestDirectResolver_UnknownService(t *testing.T) {
	vs, _ := newUnlockedVault(t)
	defer vs.Lock()

	r := NewDirect(vs)
	defer func() { _ = r.Close() }()

	if _, err := r.ResolveValues([]envmap.Mapping{{EnvName: "X", Service: "nonexistent"}}, "password"); err == nil {
		t.Fatal("expected error for unknown service, got nil")
	}
}

// TestDirectResolver_InvalidField surfaces the field-resolver error.
func TestDirectResolver_InvalidField(t *testing.T) {
	vs, _ := newUnlockedVault(t)
	defer vs.Lock()

	r := NewDirect(vs)
	defer func() { _ = r.Close() }()

	if _, err := r.ResolveValues([]envmap.Mapping{{EnvName: "X", Service: "github", Field: "totp"}}, "password"); err == nil {
		t.Fatal("expected error for invalid field, got nil")
	}
}
