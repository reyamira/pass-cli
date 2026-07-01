package agent

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arimxyer/pass-cli/internal/vault"
)

const testSecret = "s3cr3t-pw-agent"

// fakeClock is a manually-advanced clock for deterministic auto-lock tests.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestVault(t *testing.T) *vault.VaultService {
	t.Helper()
	dir := t.TempDir()
	vs, err := vault.New(filepath.Join(dir, "vault.enc"))
	if err != nil {
		t.Fatalf("vault.New: %v", err)
	}
	if err := vs.Initialize([]byte("TestPassword123!"), false, "", "agent-test"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := vs.Unlock([]byte("TestPassword123!")); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if err := vs.AddCredential("github", "octocat", []byte(testSecret), "vcs", "https://github.com", ""); err != nil {
		t.Fatalf("AddCredential: %v", err)
	}
	return vs
}

func resolveReq(refs ...Ref) Request {
	return Request{Version: ProtocolVersion, Method: MethodResolve, Resolve: &ResolveParams{Refs: refs, DefaultField: "password"}}
}

func TestAgent_Resolve(t *testing.T) {
	a := New(newTestVault(t), Options{})
	resp := a.Handle(resolveReq(Ref{Service: "github"}, Ref{Service: "github", Field: "username"}))
	if !resp.OK {
		t.Fatalf("resolve failed: %s", resp.Error)
	}
	want := []string{testSecret, "octocat"}
	if len(resp.Values) != len(want) || resp.Values[0] != want[0] || resp.Values[1] != want[1] {
		t.Errorf("values = %v, want %v", resp.Values, want)
	}
}

func TestAgent_UnknownMethodAndNoKeyExport(t *testing.T) {
	a := New(newTestVault(t), Options{})
	// There is no key-export method by construction; any such request is unknown.
	for _, m := range []string{"get-key", "get-password", "getMasterPassword", "export-key", "bogus"} {
		resp := a.Handle(Request{Version: ProtocolVersion, Method: m})
		if resp.OK {
			t.Errorf("method %q unexpectedly handled", m)
		}
	}
}

func TestAgent_VersionMismatch(t *testing.T) {
	a := New(newTestVault(t), Options{})
	resp := a.Handle(Request{Version: ProtocolVersion + 1, Method: MethodStatus})
	if resp.OK {
		t.Error("expected version mismatch to be refused")
	}
}

func TestAgent_LockedRefusesResolve(t *testing.T) {
	a := New(newTestVault(t), Options{})
	if resp := a.Handle(Request{Version: ProtocolVersion, Method: MethodShutdown}); !resp.OK {
		t.Fatalf("shutdown failed: %s", resp.Error)
	}
	resp := a.Handle(resolveReq(Ref{Service: "github"}))
	if resp.OK {
		t.Error("resolve after lock should fail")
	}
	if !a.Locked() {
		t.Error("agent should report locked")
	}
}

func TestAgent_IdleTimeoutLocks(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	a := New(newTestVault(t), Options{IdleTimeout: 15 * time.Minute, Clock: clk})

	// Just under the timeout: still resolves.
	clk.advance(14 * time.Minute)
	if resp := a.Handle(resolveReq(Ref{Service: "github"})); !resp.OK {
		t.Fatalf("resolve within idle window failed: %s", resp.Error)
	}

	// Idle past the timeout from the last activity: locks.
	clk.advance(16 * time.Minute)
	if resp := a.Handle(resolveReq(Ref{Service: "github"})); resp.OK {
		t.Error("resolve after idle timeout should fail")
	}
	if !a.Locked() {
		t.Error("agent should be locked after idle timeout")
	}
}

func TestAgent_MaxTTLLocks(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	a := New(newTestVault(t), Options{MaxTTL: 8 * time.Hour, IdleTimeout: 24 * time.Hour, Clock: clk})

	// Active use keeps resetting idle, but max-TTL is a hard cap.
	for i := 0; i < 7; i++ {
		clk.advance(1 * time.Hour)
		if resp := a.Handle(resolveReq(Ref{Service: "github"})); !resp.OK {
			t.Fatalf("resolve at hour %d failed: %s", i, resp.Error)
		}
	}
	clk.advance(2 * time.Hour) // now past 8h since unlock
	if resp := a.Handle(resolveReq(Ref{Service: "github"})); resp.OK {
		t.Error("resolve after max-TTL should fail despite activity")
	}
}

func TestAgent_Status(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	a := New(newTestVault(t), Options{MaxTTL: time.Hour, Clock: clk})

	clk.advance(10 * time.Minute)
	resp := a.Handle(Request{Version: ProtocolVersion, Method: MethodStatus})
	if !resp.OK || resp.Status == nil {
		t.Fatalf("status failed: %+v", resp)
	}
	if !resp.Status.Unlocked {
		t.Error("expected unlocked")
	}
	if resp.Status.IdleSeconds != int64((10 * time.Minute).Seconds()) {
		t.Errorf("idle = %d, want %d", resp.Status.IdleSeconds, int64((10 * time.Minute).Seconds()))
	}
	if resp.Status.MaxTTLRemainingSeconds != int64((50 * time.Minute).Seconds()) {
		t.Errorf("ttl remaining = %d, want %d", resp.Status.MaxTTLRemainingSeconds, int64((50 * time.Minute).Seconds()))
	}
}

// TestAgent_LoggerNeverEmitsSecret resolves with a real WriterLogger and asserts
// the secret value never appears in the log output. The Logger interface cannot
// even receive a value, but this pins that no future path leaks one.
func TestAgent_LoggerNeverEmitsSecret(t *testing.T) {
	var buf bytes.Buffer
	a := New(newTestVault(t), Options{Logger: WriterLogger{W: &buf}})

	resp := a.Handle(resolveReq(Ref{Service: "github"}))
	if !resp.OK {
		t.Fatalf("resolve failed: %s", resp.Error)
	}
	// Trigger status/shutdown events too, to exercise more log paths.
	a.Handle(Request{Version: ProtocolVersion, Method: MethodStatus})
	a.Handle(Request{Version: ProtocolVersion, Method: MethodShutdown})

	logged := buf.String()
	if strings.Contains(logged, testSecret) {
		t.Errorf("secret leaked into agent log:\n%s", logged)
	}
	if !strings.Contains(logged, "resolve") || !strings.Contains(logged, "github") {
		t.Errorf("expected non-secret event context in log, got:\n%s", logged)
	}
}

// TestAgent_ResolveErrorNoValueLeak verifies a resolve error message names the
// service but not any value (unknown service here).
func TestAgent_ResolveErrorNoValueLeak(t *testing.T) {
	a := New(newTestVault(t), Options{})
	resp := a.Handle(resolveReq(Ref{Service: "nonexistent"}))
	if resp.OK {
		t.Fatal("expected error for unknown service")
	}
	if strings.Contains(resp.Error, testSecret) {
		t.Errorf("error leaked a secret: %s", resp.Error)
	}
}
