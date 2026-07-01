package agent

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/arimxyer/pass-cli/internal/envmap"
)

// startTestAgent spins up a real socket-backed agent on a temp path and returns a
// stop func. PASS_CLI_AGENT_SOCK points SocketPath()/DialResolver at it.
func startTestAgent(t *testing.T) func() {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unix-socket transport; Windows named pipe is Phase 2f")
	}
	sockPath := filepath.Join(t.TempDir(), "agent.sock")
	t.Setenv("PASS_CLI_AGENT_SOCK", sockPath)

	ln, err := Listen(SocketPath())
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	srv := NewServer(New(newTestVault(t), Options{}), ln, nil)
	go srv.Serve()

	// Wait for the socket to accept a connection.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := DialResolver(); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("agent socket did not come up")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return srv.Stop
}

func TestSocketResolver_ResolvesOverSocket(t *testing.T) {
	stop := startTestAgent(t)
	defer stop()

	r, ok := DialResolver()
	if !ok {
		t.Fatal("expected agent to be reachable")
	}
	defer func() { _ = r.Close() }()

	values, err := r.ResolveValues([]envmap.Mapping{
		{Service: "github"},
		{Service: "github", Field: "username"},
	}, "password")
	if err != nil {
		t.Fatalf("ResolveValues over socket: %v", err)
	}
	if len(values) != 2 || values[0] != testSecret || values[1] != "octocat" {
		t.Errorf("values = %v, want [%s octocat]", values, testSecret)
	}
}

func TestDialResolver_FallbackWhenAbsent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-socket transport")
	}
	// Point at a path with no listener.
	t.Setenv("PASS_CLI_AGENT_SOCK", filepath.Join(t.TempDir(), "absent.sock"))
	if _, ok := DialResolver(); ok {
		t.Error("expected DialResolver to report unreachable when no agent is listening")
	}
}

func TestListen_RefusesSecondAgent(t *testing.T) {
	stop := startTestAgent(t)
	defer stop()

	// A second Listen on the same live socket must fail ("already running").
	if _, err := Listen(SocketPath()); err == nil {
		t.Error("expected second Listen to fail while an agent is running")
	}
}

func TestServer_ShutdownStops(t *testing.T) {
	stop := startTestAgent(t)
	defer stop()

	r, ok := DialResolver()
	if !ok {
		t.Fatal("agent should be reachable")
	}
	// A resolve works before shutdown.
	if _, err := r.ResolveValues([]envmap.Mapping{{Service: "github"}}, "password"); err != nil {
		t.Fatalf("pre-shutdown resolve failed: %v", err)
	}
}
