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

// agentServeCmd is an explicit alias for the foreground behavior of bare
// `pass-cli agent`. Pair it with `&`, or use `agent start` to background it.
var agentServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the agent in the foreground (same as bare `pass-cli agent`)",
	Long: `Serve runs the agent in the FOREGROUND, blocking until it locks or is stopped.
Background it with a shell '&', or use 'pass-cli agent start' to daemonize it.`,
	Args: cobra.NoArgs,
	RunE: runAgent,
}

// agentStartCmd daemonizes: it spawns a detached `agent serve` and returns once
// the agent is unlocked and listening — the ergonomic replacement for `agent &`.
var agentStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the agent in the background (unlock once, then detach)",
	Long: `Start launches the agent as a background process and returns as soon as it is
unlocked and listening — no shell '&' needed. The one-time unlock happens on your
terminal (a prompt, or silently via the keychain); after that the agent runs
detached and survives closing the terminal. Stop it with 'pass-cli agent stop'
(or 'pass-cli lock').`,
	Args: cobra.NoArgs,
	RunE: runAgentStart,
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(agentStopCmd, agentStatusCmd, agentServeCmd, agentStartCmd)
	// Persistent so `agent`, `agent serve`, and `agent start` all accept them
	// (and `start` forwards them to the serve process it spawns).
	agentCmd.PersistentFlags().DurationVar(&agentIdleTimeout, "idle", 15*time.Minute, "lock the vault after this much inactivity (0 = never)")
	agentCmd.PersistentFlags().DurationVar(&agentMaxTTL, "max-ttl", 8*time.Hour, "hard cap on how long the vault stays unlocked (0 = no cap)")
}

func runAgent(cmd *cobra.Command, _ []string) error {
	// Refuse to start a second agent.
	if _, ok := agent.DialResolver(); ok {
		return fmt.Errorf("an agent is already running at %s", agent.SocketPath())
	}

	// Harden the daemon's memory BEFORE unlocking, so the master password and
	// decrypted credentials never touch swap and the process cannot be core-dumped
	// or casually ptraced. Best-effort: a failure (e.g. a low RLIMIT_MEMLOCK) is a
	// warning, not fatal — the agent is then no worse off than a one-shot command.
	if err := agent.HardenProcessMemory(); err != nil {
		// PR_SET_DUMPABLE=0 is applied first and rarely fails, so core-dump/ptrace
		// protection is active even here — only the swap-lock (mlock) is unavailable,
		// which is expected without a raised RLIMIT_MEMLOCK. Keep the tone calm.
		_, _ = fmt.Fprintf(os.Stderr, "note: agent memory not locked into RAM (%v).\n"+
			"      core-dump and ptrace protection are still active; raise RLIMIT_MEMLOCK "+
			"(e.g. systemd LimitMEMLOCK=infinity) to enable mlock.\n", err)
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
