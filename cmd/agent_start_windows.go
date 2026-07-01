//go:build windows

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// runAgentStart is unsupported on Windows: the agent uses a unix-socket transport
// (a named-pipe transport is planned), so there is nothing to daemonize yet.
func runAgentStart(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("the background agent is not supported on Windows yet (unix-socket transport)")
}
