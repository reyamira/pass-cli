package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/agent"
	"github.com/arimxyer/pass-cli/internal/vault"
)

var (
	agentIdleTimeout time.Duration
	agentMaxTTL      time.Duration
)

// expiryCheckInterval is how often the running agent proactively enforces its
// idle/max-TTL locks (independent of incoming requests), so an idle agent locks
// even when no client is asking.
const expiryCheckInterval = 30 * time.Second

var agentCmd = &cobra.Command{
	Use:     "agent",
	GroupID: "utilities",
	Short:   "Run a background agent holding the unlocked vault for fast, promptless access",
	Long: `Agent unlocks the vault once and holds it in memory, then answers read-only
credential lookups over a local socket so that exec, export, and inject need no
master-password prompt and no key derivation on each call.

  pass-cli agent &                 # unlock once, serve in the background
  pass-cli exec --set T=github -- gh repo list   # resolves via the agent, no prompt

The agent serves resolved field VALUES only — the master password and derived key
never leave the process. It auto-locks after --idle inactivity and always after
--max-ttl, and locks + exits on SIGINT/SIGTERM. When no agent is running, every
command transparently falls back to opening and unlocking the vault directly.

The socket is $PASS_CLI_AGENT_SOCK, else $XDG_RUNTIME_DIR/pass-cli/agent.sock,
else ~/.pass-cli/agent.sock (directory 0700, socket 0600).`,
	Args: cobra.NoArgs,
	RunE: runAgent,
}

var agentStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running agent (locks its vault and exits the process)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if err := agent.Stop(); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "agent stopped")
		return nil
	},
}

var agentStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show a running agent's status (never prints secrets)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		st, err := agent.QueryStatus()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(out, "unlocked:      %t\n", st.Unlocked)
		_, _ = fmt.Fprintf(out, "idle:          %ds\n", st.IdleSeconds)
		if st.MaxTTLRemainingSeconds > 0 {
			_, _ = fmt.Fprintf(out, "max-ttl left:  %ds\n", st.MaxTTLRemainingSeconds)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(agentStopCmd, agentStatusCmd)
	agentCmd.Flags().DurationVar(&agentIdleTimeout, "idle", 15*time.Minute, "lock the vault after this much inactivity (0 = never)")
	agentCmd.Flags().DurationVar(&agentMaxTTL, "max-ttl", 8*time.Hour, "hard cap on how long the vault stays unlocked (0 = no cap)")
}

func runAgent(cmd *cobra.Command, _ []string) error {
	// Refuse to start a second agent.
	if _, ok := agent.DialResolver(); ok {
		return fmt.Errorf("an agent is already running at %s", agent.SocketPath())
	}

	vaultPath := GetVaultPath()
	if _, err := os.Stat(vaultPath); os.IsNotExist(err) {
		return fmt.Errorf("vault not found at %s\nRun 'pass-cli init' to create a vault first", vaultPath)
	}
	vaultService, err := vault.New(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to create vault service at %s: %w", vaultPath, err)
	}
	// Unlock once. The agent owns the vault for its lifetime and locks it on
	// shutdown / idle / max-TTL, so we do NOT defer Lock here.
	if err := unlockVaultWithSync(vaultService); err != nil {
		return err
	}

	ln, err := agent.Listen(agent.SocketPath())
	if err != nil {
		vaultService.Lock()
		return err
	}

	logger := agent.WriterLogger{W: os.Stderr}
	ag := agent.New(vaultService, agent.Options{
		IdleTimeout: agentIdleTimeout,
		MaxTTL:      agentMaxTTL,
		Logger:      logger,
	})
	srv := agent.NewServer(ag, ln, logger)

	// Stop on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		srv.Stop()
	}()

	// Proactively enforce idle/max-TTL even with no incoming requests: once the
	// agent locks, stop serving so the socket is freed for a fresh unlock.
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(expiryCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if ag.Locked() {
					srv.Stop()
					return
				}
			}
		}
	}()

	_, _ = fmt.Fprintf(os.Stderr, "pass-cli agent listening on %s (idle=%s max-ttl=%s)\n",
		agent.SocketPath(), agentIdleTimeout, agentMaxTTL)

	srv.Serve() // blocks until Stop
	close(done)

	vaultService.Lock()
	_ = os.Remove(agent.SocketPath())
	return nil
}
