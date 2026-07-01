//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/agent"
)

// startPollInterval / startTimeout bound how long `agent start` waits for the
// spawned agent to unlock and begin listening. The timeout is generous because
// unlock may include a password prompt and a sync pull; it is only a backstop
// against a wedged child.
const (
	startPollInterval = 50 * time.Millisecond
	startTimeout      = 2 * time.Minute
)

// runAgentStart daemonizes the agent: it spawns `agent serve` in a new session
// (so it survives the terminal closing) with stdio still on the current terminal
// so the one-time unlock prompt works, then returns once the agent is listening.
func runAgentStart(cmd *cobra.Command, _ []string) error {
	if _, ok := agent.DialResolver(); ok {
		return fmt.Errorf("an agent is already running at %s", agent.SocketPath())
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate the pass-cli binary to spawn: %w", err)
	}

	// A daemon has no stdout; sending it to /dev/null is what keeps the child from
	// holding a caller's captured pipe open forever — otherwise `out=$(pass-cli
	// agent start)` (and any parent that pipes our stdout) hangs waiting for an EOF
	// that never comes, because the detached child inherited the write end.
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", os.DevNull, err)
	}
	defer func() { _ = devNull.Close() }()

	child := exec.Command(exe, "agent", "serve",
		"--idle", agentIdleTimeout.String(),
		"--max-ttl", agentMaxTTL.String())
	// stdin + stderr stay on the terminal so the one-time unlock prompt is visible
	// and answerable (keychain unlock is silent); stdout goes to /dev/null. The
	// environment carries PASS_CLI_* / XDG so the child uses the same config, vault,
	// and socket path.
	child.Stdin = os.Stdin
	child.Stdout = devNull
	child.Stderr = os.Stderr
	// Setsid detaches the child into its own session/process group so it is not
	// killed when this command returns or the terminal is closed (SIGHUP).
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := child.Start(); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	// Watch for an early exit (unlock failed / vault missing) while polling for the
	// socket to come up.
	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()

	deadline := time.Now().Add(startTimeout)
	for {
		select {
		case werr := <-exited:
			if werr != nil {
				return fmt.Errorf("agent exited during startup: %w", werr)
			}
			return fmt.Errorf("agent exited during startup before it began listening")
		default:
		}
		if _, ok := agent.DialResolver(); ok {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "agent started (pid %d) at %s\n",
				child.Process.Pid, agent.SocketPath())
			return nil
		}
		if time.Now().After(deadline) {
			_ = child.Process.Kill()
			return fmt.Errorf("agent did not come up within %s", startTimeout)
		}
		time.Sleep(startPollInterval)
	}
}
