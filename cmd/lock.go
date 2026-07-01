package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/agent"
)

var lockCmd = &cobra.Command{
	Use:     "lock",
	GroupID: "vault",
	Short:   "Lock a running agent (zero its secrets and stop it)",
	Long: `Lock zeroes the secrets held by a running agent and stops it, freeing the
socket. Subsequent commands fall back to opening the vault directly (with a
prompt); to re-establish the agent, run 'pass-cli agent' again. If no agent is
running there is nothing to lock (a one-shot command already locks the vault
when it exits).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if err := agent.LockAgent(); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "agent locked and stopped")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(lockCmd)
}
