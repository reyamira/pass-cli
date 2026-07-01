package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/agent"
)

// runLock is shared by the top-level `pass-cli lock` and `pass-cli agent lock`.
func runLock(cmd *cobra.Command, _ []string) error {
	if err := agent.LockAgent(); err != nil {
		if errors.Is(err, agent.ErrNoAgent) {
			// Nothing to lock is not a failure — a one-shot command already locks the
			// vault when it exits.
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no agent is running (nothing to lock)")
			return nil
		}
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "agent locked and stopped")
	return nil
}

const lockLong = `Lock zeroes the secrets held by a running agent and stops it, freeing the
socket. Subsequent commands fall back to opening the vault directly (with a
prompt); to re-establish the agent, run 'pass-cli agent' again. If no agent is
running there is nothing to lock (a one-shot command already locks the vault
when it exits).`

// lockCmd is the top-level shortcut: `pass-cli lock`.
var lockCmd = &cobra.Command{
	Use:     "lock",
	GroupID: "vault",
	Short:   "Lock the running agent (zero its secrets and stop it)",
	Long:    lockLong,
	Args:    cobra.NoArgs,
	RunE:    runLock,
}

// agentLockCmd is the same action under `agent`, where it sits next to
// `agent stop` and `agent status` — the first place people look for it.
var agentLockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Lock the running agent (zero its secrets and stop it)",
	Long:  lockLong,
	Args:  cobra.NoArgs,
	RunE:  runLock,
}

func init() {
	rootCmd.AddCommand(lockCmd)
	agentCmd.AddCommand(agentLockCmd)
}
