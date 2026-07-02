//go:build integration

package integration

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/arimxyer/pass-cli/test/helpers"
)

// shortSocketPath returns a unix-socket path under a short base dir. Unix socket
// paths have a ~104-char limit on macOS/BSD, and t.TempDir() embeds the (long)
// test name, so it can overflow — os.MkdirTemp with a short prefix stays well under.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pc")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "a.sock")
}

// startAgent launches `pass-cli agent` in the background on a temp socket, unlocks
// it via stdin, waits for the socket to appear (which happens only after unlock),
// and returns the socket path plus a stop func.
func startAgent(t *testing.T, configPath, password string) (sockPath string, pid int, stop func()) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("agent uses a unix socket; Windows named pipe is Phase 2f")
	}
	sockPath = shortSocketPath(t)

	cmd := exec.Command(binaryPath, "agent", "--idle", "1h", "--max-ttl", "1h")
	cmd.Stdin = strings.NewReader(helpers.BuildUnlockStdin(password))
	cmd.Env = append(os.Environ(),
		"PASS_CLI_TEST=1",
		"PASS_CLI_CONFIG="+configPath,
		"PASS_CLI_AGENT_SOCK="+sockPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start agent: %v", err)
	}
	pid = cmd.Process.Pid
	stopped := false
	stop = func() {
		if stopped {
			return
		}
		stopped = true
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
	}
	t.Cleanup(stop)

	// The listener is bound only after a successful unlock, so the socket file
	// appearing is a sufficient readiness signal.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(sockPath); err == nil {
			return sockPath, pid, stop
		}
		if time.Now().After(deadline) {
			stop()
			t.Fatalf("agent socket did not appear\nAgent stderr: %s", stderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// runWithAgent runs pass-cli with the agent socket set and NO stdin, so a resolve
// can only succeed via the agent (a fallback to direct-open would block/fail on the
// missing master password).
func runWithAgent(t *testing.T, configPath, sockPath string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(),
		"PASS_CLI_TEST=1",
		"PASS_CLI_CONFIG="+configPath,
		"PASS_CLI_AGENT_SOCK="+sockPath,
	)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

// TestIntegration_Agent_ExecResolvesViaSocket proves exec resolves through a
// running agent with no password prompt.
func TestIntegration_Agent_ExecResolvesViaSocket(t *testing.T) {
	configPath, password, service, secret := setupExecVault(t)
	sockPath, _, _ := startAgent(t, configPath, password)

	out, stderr, err := runWithAgent(t, configPath, sockPath,
		"exec", "--set", "TOK="+service, "--", "sh", "-c", `printf %s "$TOK"`)
	if err != nil {
		t.Fatalf("exec via agent failed: %v\nStderr: %s", err, stderr)
	}
	if strings.TrimSpace(out) != secret {
		t.Errorf("exec via agent: got %q, want %q", strings.TrimSpace(out), secret)
	}
}

// TestIntegration_Agent_FilterAppliedClientSide proves a value filter is applied
// by the client even when a running agent serves the raw value over the socket:
// the agent protocol carries no filter, so base64 must happen client-side and
// match direct-mode output.
func TestIntegration_Agent_FilterAppliedClientSide(t *testing.T) {
	configPath, password, service, secret := setupExecVault(t)
	sockPath, _, _ := startAgent(t, configPath, password)

	out, stderr, err := runWithAgent(t, configPath, sockPath,
		"exec", "--set", "TOK="+service+"|base64", "--", "sh", "-c", `printf %s "$TOK"`)
	if err != nil {
		t.Fatalf("filtered exec via agent failed: %v\nStderr: %s", err, stderr)
	}
	want := base64.StdEncoding.EncodeToString([]byte(secret))
	if strings.TrimSpace(out) != want {
		t.Errorf("filtered exec via agent: got %q, want %q", strings.TrimSpace(out), want)
	}
}

// TestIntegration_Agent_ExportResolvesViaSocket proves export also uses the agent.
func TestIntegration_Agent_ExportResolvesViaSocket(t *testing.T) {
	configPath, password, service, secret := setupExecVault(t)
	sockPath, _, _ := startAgent(t, configPath, password)

	out, stderr, err := runWithAgent(t, configPath, sockPath, "export", "--set", "TOK="+service)
	if err != nil {
		t.Fatalf("export via agent failed: %v\nStderr: %s", err, stderr)
	}
	stmt := strings.TrimSpace(out)
	if got := evalSh(t, stmt, "TOK"); got != secret {
		t.Errorf("export via agent: got %q, want %q\nstmt: %s", got, secret, stmt)
	}
}

// TestIntegration_Agent_Status queries a running agent.
func TestIntegration_Agent_Status(t *testing.T) {
	configPath, password, _, _ := setupExecVault(t)
	sockPath, _, _ := startAgent(t, configPath, password)

	out, stderr, err := runWithAgent(t, configPath, sockPath, "agent", "status")
	if err != nil {
		t.Fatalf("agent status failed: %v\nStderr: %s", err, stderr)
	}
	if !strings.Contains(out, "unlocked:      true") {
		t.Errorf("expected unlocked status, got: %s", out)
	}
}

// TestIntegration_Agent_MemoryHardened verifies PR_SET_DUMPABLE=0 took effect on
// the agent process: a non-dumpable process's /proc/<pid>/environ becomes
// root-owned, so the same user can no longer read it. This proves the privilege-
// free hardening ran (mlockall is best-effort and RLIMIT-dependent, so it is not
// asserted here).
func TestIntegration_Agent_MemoryHardened(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("PR_SET_DUMPABLE / mlock hardening is Linux-only for now")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: /proc access rules for non-dumpable processes differ")
	}
	configPath, password, service, secret := setupExecVault(t)
	sockPath, pid, _ := startAgent(t, configPath, password)

	// The agent must still resolve with hardening applied.
	out, stderr, err := runWithAgent(t, configPath, sockPath,
		"exec", "--set", "TOK="+service, "--", "sh", "-c", `printf %s "$TOK"`)
	if err != nil {
		t.Fatalf("resolve with hardening failed: %v\nStderr: %s", err, stderr)
	}
	if strings.TrimSpace(out) != secret {
		t.Errorf("resolve with hardening: got %q, want %q", strings.TrimSpace(out), secret)
	}

	// PR_SET_DUMPABLE=0 makes /proc/<pid>/environ root-owned, so the owning user can
	// no longer read it. (mlockall is best-effort / RLIMIT-dependent, so not asserted.)
	if _, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid)); err == nil {
		t.Error("agent /proc/<pid>/environ is readable — PR_SET_DUMPABLE=0 did not take effect")
	}
}

// TestIntegration_Agent_StartDaemonizes verifies `agent start` spawns a detached
// agent (unlocking via stdin), returns once it is listening, and that the detached
// agent then resolves with no prompt.
func TestIntegration_Agent_StartDaemonizes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("agent start is unsupported on Windows")
	}
	configPath, password, service, secret := setupExecVault(t)
	sockPath := shortSocketPath(t)

	// Always stop the detached agent, even on failure.
	t.Cleanup(func() {
		stop := exec.Command(binaryPath, "agent", "stop")
		stop.Env = append(os.Environ(),
			"PASS_CLI_TEST=1", "PASS_CLI_CONFIG="+configPath, "PASS_CLI_AGENT_SOCK="+sockPath)
		_ = stop.Run()
	})

	// Use real OS fds: the detached `serve` inherits stdin/stderr and holds them
	// open, so a bytes.Buffer (pipe) stderr would make start.Run()'s Wait block on a
	// never-closing copy goroutine — the very bug this feature had to fix. A pipe for
	// stdin (password) and a file for stderr are passed straight through, no copy
	// goroutine. (serve's stdout is /dev/null, so capturing start's stdout is safe.)
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = pw.WriteString(helpers.BuildUnlockStdin(password)); _ = pw.Close() }()
	errFile, err := os.CreateTemp(t.TempDir(), "start-stderr")
	if err != nil {
		t.Fatal(err)
	}

	start := exec.Command(binaryPath, "agent", "start", "--idle", "1h", "--max-ttl", "1h")
	start.Env = append(os.Environ(),
		"PASS_CLI_TEST=1", "PASS_CLI_CONFIG="+configPath, "PASS_CLI_AGENT_SOCK="+sockPath)
	start.Stdin = pr
	var startOut bytes.Buffer
	start.Stdout = &startOut
	start.Stderr = errFile
	t0 := time.Now()
	err = start.Run()
	_ = pr.Close()
	serveErr, _ := os.ReadFile(errFile.Name())
	t.Logf("agent start took %s\nstdout: %q\nstderr: %q", time.Since(t0), startOut.String(), string(serveErr))
	if err != nil {
		t.Fatalf("agent start failed: %v", err)
	}
	if !strings.Contains(startOut.String(), "agent started") {
		t.Errorf("expected an 'agent started' message, got: %q", startOut.String())
	}

	// The detached agent resolves with no stdin/prompt.
	out, stderr, err := runWithAgent(t, configPath, sockPath,
		"exec", "--set", "TOK="+service, "--", "sh", "-c", `printf %s "$TOK"`)
	if err != nil {
		t.Fatalf("resolve via started agent failed: %v\nstderr: %s", err, stderr)
	}
	if strings.TrimSpace(out) != secret {
		t.Errorf("resolve via started agent: got %q, want %q", strings.TrimSpace(out), secret)
	}
}

// TestIntegration_Agent_StopThenFallback stops the agent and verifies the socket is
// freed promptly and commands fall back to direct-open (password supplied on stdin).
func TestIntegration_Agent_StopThenFallback(t *testing.T) {
	configPath, password, service, secret := setupExecVault(t)
	sockPath, _, stop := startAgent(t, configPath, password)

	// Stop the agent over the socket.
	if _, stderr, err := runWithAgent(t, configPath, sockPath, "agent", "stop"); err != nil {
		t.Fatalf("agent stop failed: %v\nStderr: %s", err, stderr)
	}
	stop()

	// The socket should be freed promptly (server stopped on the shutdown).
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(sockPath); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("socket still present after stop — agent did not stop")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// PASS_CLI_AGENT_SOCK still points at the (now-absent) socket; exec must fall
	// back to direct-open. Supply the password on stdin.
	cmd := exec.Command(binaryPath, "exec", "--set", "TOK="+service, "--", "sh", "-c", `printf %s "$TOK"`)
	cmd.Env = append(os.Environ(),
		"PASS_CLI_TEST=1", "PASS_CLI_CONFIG="+configPath, "PASS_CLI_AGENT_SOCK="+sockPath)
	cmd.Stdin = strings.NewReader(helpers.BuildUnlockStdin(password))
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("exec fallback failed: %v\nStderr: %s", err, errb.String())
	}
	if strings.TrimSpace(out.String()) != secret {
		t.Errorf("exec fallback: got %q, want %q", strings.TrimSpace(out.String()), secret)
	}
}
