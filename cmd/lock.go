package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/agent"
)

var lockCmd = &cobra.Command{
	Use:     "lock",
	GroupID: "vault",
	Short:   "Lock a running agent's vault without stopping the process",
	Long: `Lock zeroes the secrets held by a running agent, so subsequent lookups require
a fresh unlock, while leaving the agent process running. If no agent is running
there is nothing to lock (a one-shot command already locks the vault when it
exits).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if err := agent.LockAgent(); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "agent vault locked")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(lockCmd)
}
