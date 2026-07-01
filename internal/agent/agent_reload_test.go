package agent

import (
	"bytes"
	"os"
	"testing"

	"github.com/arimxyer/pass-cli/internal/vault"
)

// openSibling opens a second VaultService on the same vault file and unlocks it,
// simulating another process writing the vault while the agent holds it.
func openSibling(t *testing.T, path string) *vault.VaultService {
	t.Helper()
	sib, err := vault.New(path)
	if err != nil {
		t.Fatalf("sibling vault.New: %v", err)
	}
	if err := sib.Unlock([]byte("TestPassword123!")); err != nil {
		t.Fatalf("sibling Unlock: %v", err)
	}
	return sib
}

// TestAgent_RevalidatesOnSiblingWrite proves the agent reloads and serves fresh
// data after another process writes the vault, rather than serving a stale snapshot.
func TestAgent_RevalidatesOnSiblingWrite(t *testing.T) {
	vs := newTestVault(t)
	a := New(vs, Options{})

	// A credential added after the agent unlocked is initially invisible...
	if resp := a.Handle(resolveReq(Ref{Service: "gitlab"})); resp.OK {
		t.Fatal("gitlab should not exist yet")
	}

	sib := openSibling(t, vs.Path())
	if err := sib.AddCredential("gitlab", "u", []byte("gl-secret"), "", "", ""); err != nil {
		t.Fatalf("sibling AddCredential: %v", err)
	}

	// ...but the next resolve revalidates against the on-disk change and sees it.
	resp := a.Handle(resolveReq(Ref{Service: "gitlab"}))
	if !resp.OK {
		t.Fatalf("expected gitlab to resolve after revalidation: %s", resp.Error)
	}
	if len(resp.Values) != 1 || resp.Values[0] != "gl-secret" {
		t.Errorf("values = %v, want [gl-secret]", resp.Values)
	}
}

// TestAgent_LocksOnRotatedPassword is the failure path a green build won't cover:
// if the on-disk vault is re-encrypted under a new master password, the agent's
// held key can no longer decrypt it — it must LOCK and fail, never serve stale.
func TestAgent_LocksOnRotatedPassword(t *testing.T) {
	vs := newTestVault(t)
	a := New(vs, Options{})

	// Sanity: resolves before rotation.
	if resp := a.Handle(resolveReq(Ref{Service: "github"})); !resp.OK {
		t.Fatalf("pre-rotation resolve failed: %s", resp.Error)
	}

	// Another process rotates the master password (re-encrypts vault.enc).
	sib := openSibling(t, vs.Path())
	if err := sib.ChangePassword([]byte("NewPassword456!")); err != nil {
		t.Fatalf("sibling ChangePassword: %v", err)
	}

	// The agent detects the on-disk change, fails to reload with its old key, and
	// locks rather than serving a stale snapshot.
	resp := a.Handle(resolveReq(Ref{Service: "github"}))
	if resp.OK {
		t.Fatal("resolve must fail after the password was rotated")
	}
	if !a.Locked() {
		t.Error("agent must lock when it can no longer decrypt the on-disk vault")
	}
}

// TestAgent_ResolveDoesNotMutateVault extends the read-only guarantee to the agent:
// serving a resolve (which may stat and even reload) never writes the vault file.
func TestAgent_ResolveDoesNotMutateVault(t *testing.T) {
	vs := newTestVault(t)
	a := New(vs, Options{})

	before, err := os.ReadFile(vs.Path())
	if err != nil {
		t.Fatal(err)
	}
	if resp := a.Handle(resolveReq(Ref{Service: "github"})); !resp.OK {
		t.Fatalf("resolve failed: %s", resp.Error)
	}
	after, err := os.ReadFile(vs.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("agent resolve mutated the vault file; the agent must be strictly read-only")
	}
}
