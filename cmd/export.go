package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/envmap"
	"github.com/arimxyer/pass-cli/internal/resolver"
	"github.com/arimxyer/pass-cli/internal/vault"
)

var (
	exportSets   []string
	exportField  string
	exportFormat string
)

var exportCmd = &cobra.Command{
	Use:     "export [<service>...]",
	GroupID: "credentials",
	Short:   "Print shell statements that set credentials as environment variables",
	Long: `Export prints shell statements that assign stored credentials to environment
variables, for evaluation by your shell:

  eval "$(pass-cli export --set GITHUB_TOKEN=github)"

The mapping grammar is the same as 'exec':

  # Explicit mapping (repeatable): --set ENV_NAME=service[/field]
  pass-cli export --set GITHUB_TOKEN=github --set DB_PASSWORD=postgres/password

  # Convenience form: derive the env name from each service name
  # (uppercased, non-alphanumeric characters become '_')
  pass-cli export openai-api            # emits: export OPENAI_API='...'

The -f/--field flag selects which field to export (default: password) and applies
to every mapping; a single mapping can override it with a '/field' suffix.

--format selects the shell syntax:
  sh          export NAME='value'      (default; POSIX sh/bash/zsh, for eval)
  fish        set -gx NAME 'value'      (for: pass-cli export ... | source)
  powershell  $env:NAME = 'value'

Security note: 'export' materializes the secret into your CURRENT shell for that
shell's lifetime (and any process it launches can read it via the environment).
That is a weaker boundary than 'exec', which scopes the secret to a single child
process. Prefer 'exec' when you only need to launch a command; use 'export' as the
blessed replacement for VAR="$(pass-cli get ...)", not as a replacement for 'exec'.`,
	Example: `  # Load a token into the current shell
  eval "$(pass-cli export --set GITHUB_TOKEN=github)"

  # Multiple credentials, convenience form
  eval "$(pass-cli export openai-api anthropic-api)"

  # fish shell
  pass-cli export --set GITHUB_TOKEN=github --format fish | source`,
	Args: cobra.ArbitraryArgs,
	RunE: runExport,
}

func init() {
	rootCmd.AddCommand(exportCmd)
	exportCmd.Flags().StringArrayVar(&exportSets, "set", nil, "map an environment variable to a credential: ENV_NAME=service[/field] (repeatable; ':field' also accepted)")
	exportCmd.Flags().StringVarP(&exportField, "field", "f", "password", "field to export for all mappings (username, password, category, url, notes, service)")
	exportCmd.Flags().StringVar(&exportFormat, "format", "sh", "shell syntax: sh, fish, or powershell")
}

// parseExportArgs turns --set specs or positional services into mappings and
// validates every resulting env name. Names are validated here, before the vault
// is opened, so a bad name fails fast without fetching any secret. Unlike exec,
// export emits shell text to be eval'd, so an invalid or attacker-controlled name
// is a shell-injection vector — hence the ValidEnvName gate.
func parseExportArgs(sets, positionals []string) ([]envmap.Mapping, error) {
	var mappings []envmap.Mapping

	switch {
	case len(sets) > 0:
		if len(positionals) > 0 {
			return nil, fmt.Errorf("cannot combine a positional <service> (%q) with --set; use one form or the other", positionals[0])
		}
		for _, s := range sets {
			m, err := envmap.ParseSetSpec(s)
			if err != nil {
				return nil, err
			}
			mappings = append(mappings, m)
		}
	case len(positionals) > 0:
		for _, svc := range positionals {
			name := envmap.DeriveEnvName(svc)
			mappings = append(mappings, envmap.Mapping{EnvName: name, Service: svc})
		}
	default:
		return nil, errors.New("specify a <service> or --set ENV_NAME=service")
	}

	for _, m := range mappings {
		if !envmap.ValidEnvName(m.EnvName) {
			return nil, fmt.Errorf("invalid environment variable name %q (must match [A-Za-z_][A-Za-z0-9_]*)", m.EnvName)
		}
	}
	return mappings, nil
}

func runExport(cmd *cobra.Command, args []string) error {
	format, err := exportFormatter(exportFormat)
	if err != nil {
		return err
	}

	mappings, err := parseExportArgs(exportSets, args)
	if err != nil {
		return err
	}

	vaultPath := GetVaultPath()
	if _, err := os.Stat(vaultPath); os.IsNotExist(err) {
		return fmt.Errorf("vault not found at %s\nRun 'pass-cli init' to create a vault first", vaultPath)
	}

	vaultService, err := vault.New(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to create vault service at %s: %w", vaultPath, err)
	}

	// Pull from remote and unlock, overlapping the pull with the password prompt
	// (#103; read-only: no push after).
	if err := unlockVaultWithSync(vaultService); err != nil {
		return err
	}
	defer vaultService.Lock()

	// export is read-only, like exec: the resolver never records field access or
	// triggers a sync push.
	r := resolver.NewDirect(vaultService)
	defer func() { _ = r.Close() }()

	values, err := r.ResolveValues(mappings, exportField)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	for i, m := range mappings {
		if _, err := fmt.Fprintln(out, format(m.EnvName, values[i])); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
	}
	return nil
}

// exportFormatter returns the statement formatter for a shell format name.
func exportFormatter(name string) (func(envName, value string) string, error) {
	switch strings.ToLower(name) {
	case "sh", "bash", "zsh", "":
		return formatSh, nil
	case "fish":
		return formatFish, nil
	case "powershell", "pwsh", "ps":
		return formatPowerShell, nil
	default:
		return nil, fmt.Errorf("unknown --format %q (valid: sh, fish, powershell)", name)
	}
}

// formatSh emits a POSIX single-quoted assignment. Inside single quotes every byte
// is literal except ' itself, which is closed, escaped as \', and reopened.
func formatSh(envName, value string) string {
	return "export " + envName + "='" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// formatFish emits a fish single-quoted assignment. In fish single quotes only \
// and ' are special, escaped as \\ and \'. Backslash must be escaped first so the
// backslashes introduced for quotes are not doubled.
func formatFish(envName, value string) string {
	v := strings.ReplaceAll(value, `\`, `\\`)
	v = strings.ReplaceAll(v, "'", `\'`)
	return "set -gx " + envName + " '" + v + "'"
}

// formatPowerShell emits a PowerShell single-quoted assignment. In a single-quoted
// PowerShell string the only special character is ', escaped by doubling it.
func formatPowerShell(envName, value string) string {
	return "$env:" + envName + " = '" + strings.ReplaceAll(value, "'", "''") + "'"
}
