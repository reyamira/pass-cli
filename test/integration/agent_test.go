//go:build integration

package integration

import (
	"bytes"
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

// startAgent launches `pass-cli agent` in the background on a temp socket, unlocks
// it via stdin, waits for the socket to appear (which happens only after unlock),
// and returns the socket path plus a stop func.
func startAgent(t *testing.T, configPath, password string) (sockPath string, stop func()) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("agent uses a unix socket; Windows named pipe is Phase 2f")
	}
	sockPath = filepath.Join(t.TempDir(), "agent.sock")

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
			return sockPath, stop
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
	sockPath, _ := startAgent(t, configPath, password)

	out, stderr, err := runWithAgent(t, configPath, sockPath,
		"exec", "--set", "TOK="+service, "--", "sh", "-c", `printf %s "$TOK"`)
	if err != nil {
		t.Fatalf("exec via agent failed: %v\nStderr: %s", err, stderr)
	}
	if strings.TrimSpace(out) != secret {
		t.Errorf("exec via agent: got %q, want %q", strings.TrimSpace(out), secret)
	}
}

// TestIntegration_Agent_ExportResolvesViaSocket proves export also uses the agent.
func TestIntegration_Agent_ExportResolvesViaSocket(t *testing.T) {
	configPath, password, service, secret := setupExecVault(t)
	sockPath, _ := startAgent(t, configPath, password)

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
	sockPath, _ := startAgent(t, configPath, password)

	out, stderr, err := runWithAgent(t, configPath, sockPath, "agent", "status")
	if err != nil {
		t.Fatalf("agent status failed: %v\nStderr: %s", err, stderr)
	}
	if !strings.Contains(out, "unlocked:      true") {
		t.Errorf("expected unlocked status, got: %s", out)
	}
}

// TestIntegration_Agent_StopThenFallback stops the agent and verifies commands fall
// back to direct-open (with the password supplied on stdin).
func TestIntegration_Agent_StopThenFallback(t *testing.T) {
	configPath, password, service, secret := setupExecVault(t)
	sockPath, stop := startAgent(t, configPath, password)

	// Stop the agent, then resolve via direct-open with the password on stdin.
	if _, stderr, err := runWithAgent(t, configPath, sockPath, "agent", "stop"); err != nil {
		t.Fatalf("agent stop failed: %v\nStderr: %s", err, stderr)
	}
	stop()

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
