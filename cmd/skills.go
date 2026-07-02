package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/arimxyer/pass-cli/internal/skills"

	"github.com/spf13/cobra"
)

var (
	skillsGetFull      bool
	skillsInstallDir   string
	skillsInstallForce bool
)

var skillsCmd = &cobra.Command{
	Use:     "skills",
	GroupID: "utilities",
	Short:   "Agent-facing usage guides shipped with this binary",
	Long: `Skills are version-matched usage guides for driving pass-cli as an AI
agent. They ship embedded in this binary, so the guidance always matches the
installed version and never goes stale.

For AI agents: start with the core safe-usage guide.

  pass-cli skills get core          # exec/export/inject/agent/list/get + leak traps
  pass-cli skills get core --full   # also include the full command reference

Run 'pass-cli skills install' to drop a small discovery stub into your agent's
skills directory (e.g. ~/.claude/skills) that points back at this command.

Examples:
  # List available skills
  pass-cli skills list

  # Print the core agent guide
  pass-cli skills get core

  # Install the discovery stub for AI agents
  pass-cli skills install`,
	// No RunE: bare `pass-cli skills` prints help (its subcommands), matching
	// the other parent commands (vault, config, keychain).
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available skills",
	Args:  cobra.NoArgs,
	RunE:  runSkillsList,
}

var skillsGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Print a skill's guide to stdout",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		content, err := skills.Get(args[0], skillsGetFull)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), content)
		return err
	},
}

var skillsInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Write the agent discovery stub into a skills directory",
	Long: `Install writes a small discovery stub (a Claude Code / agent skill) that
points AI agents at 'pass-cli skills get core'. The stub is intentionally thin
so it never goes stale; the real guidance is served by the CLI.

By default the stub is written to the first existing of ~/.claude/skills or
~/.agents/skills (falling back to creating ~/.claude/skills), as
<dir>/pass-cli/SKILL.md. Use --dir to choose a different location.

An existing stub that differs is left untouched unless --force is given.`,
	Args: cobra.NoArgs,
	RunE: runSkillsInstall,
}

func init() {
	rootCmd.AddCommand(skillsCmd)
	skillsCmd.AddCommand(skillsListCmd)
	skillsCmd.AddCommand(skillsGetCmd)
	skillsCmd.AddCommand(skillsInstallCmd)

	skillsGetCmd.Flags().BoolVar(&skillsGetFull, "full", false, "include the full command reference")
	skillsInstallCmd.Flags().StringVar(&skillsInstallDir, "dir", "", "skills directory to install into (default: auto-detect)")
	skillsInstallCmd.Flags().BoolVar(&skillsInstallForce, "force", false, "overwrite an existing, differing stub")
}

func runSkillsList(cmd *cobra.Command, _ []string) error {
	list, err := skills.List()
	if err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString("Available skills:\n")
	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	for _, s := range list {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\n", s.Name, summaryLine(s.Description))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	sb.WriteString("\nRun 'pass-cli skills get <name>' to print a guide (add --full for the command reference).\n")

	_, err = fmt.Fprint(cmd.OutOrStdout(), sb.String())
	return err
}

func runSkillsInstall(cmd *cobra.Command, _ []string) error {
	dir, err := resolveSkillsDir(skillsInstallDir)
	if err != nil {
		return err
	}
	msg, err := installStub(dir, skillsInstallForce)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), msg)
	return err
}

// installStub writes the discovery stub to <dir>/pass-cli/SKILL.md and returns a
// human-readable status message. An existing stub that already matches is left
// untouched (idempotent); an existing stub that differs is preserved unless
// force is set. Kept separate from the Cobra command so the write behavior is
// unit-testable against a temp dir.
func installStub(dir string, force bool) (string, error) {
	stub, err := skills.StubContent()
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, "pass-cli", "SKILL.md")

	if existing, err := os.ReadFile(target); err == nil {
		if string(existing) == stub {
			return "Already up to date: " + target, nil
		}
		if !force {
			return "", fmt.Errorf("a different stub already exists at %s; re-run with --force to overwrite", target)
		}
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("failed to create %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, []byte(stub), 0o644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", target, err)
	}

	return "Installed discovery stub: " + target +
		"\nAI agents will now be pointed at 'pass-cli skills get core'.", nil
}

// resolveSkillsDir picks the directory to install the stub into. An explicit
// override is expanded and returned as-is; otherwise the first existing of
// ~/.claude/skills or ~/.agents/skills is used, falling back to ~/.claude/skills.
func resolveSkillsDir(override string) (string, error) {
	if override != "" {
		return expandHome(override), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	candidates := []string{
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c, nil
		}
	}
	return candidates[0], nil
}

func expandHome(path string) string {
	if path == "~" || len(path) >= 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// summaryLine renders a skill's one-line summary for `skills list`. The
// frontmatter description is written long and keyword-rich for AI-agent
// discovery matching, so for the human list we take just its first sentence
// (each description leads with a self-contained one), with a generous rune cap
// as a backstop against a description that has no sentence break.
func summaryLine(desc string) string {
	desc = strings.TrimSpace(desc)
	if i := strings.Index(desc, ". "); i >= 0 {
		desc = desc[:i+1] // keep the period, drop the rest
	}
	return truncate(desc, 120)
}

// truncate shortens s to at most max runes (not bytes), appending an ellipsis
// when it cuts. Rune-aware so a multi-byte character near the limit isn't split.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max == 1 {
		return string(runes[:1])
	}
	return string(runes[:max-1]) + "…"
}
