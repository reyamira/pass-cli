package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/crypto"
	"github.com/arimxyer/pass-cli/internal/vault"
)

var (
	execSets  []string
	execField string
)

var execCmd = &cobra.Command{
	Use:     "exec [<service>] -- <command> [args...]",
	GroupID: "credentials",
	Short:   "Run a command with credentials injected as environment variables",
	Long: `Exec runs a child command with stored credentials injected as environment
variables. The secret value is passed only through the child process's
environment - it never touches a file, the clipboard, or your shell history.

There are two ways to map credentials to environment variables:

  # Explicit mapping (repeatable): --set ENV_NAME=service
  pass-cli exec --set GITHUB_TOKEN=github -- gh repo list

  # Convenience form: derive the env name from the service name
  # (uppercased, non-alphanumeric characters become '_')
  pass-cli exec openai-api -- python train.py   # sets OPENAI_API

The -f/--field flag selects which field to inject (default: password) and
applies to every mapping. A single mapping can override it with a ':field'
suffix, which is how you inject two fields of the same entry as separate
variables (e.g. a database username and password):

  pass-cli exec --set DB_USER=postgres:username --set DB_PASSWORD=postgres:password -- ./run.sh

Everything after '--' is the command to run; pass-cli writes nothing of its
own to stdout, and the child's exit code is propagated unchanged.

Security note: the injected value lives in the child process's environment.
On Linux it is readable via /proc/<pid>/environ by the same user and is
inherited by descendant processes. This is the same model as 'op run' and
'aws-vault exec' - far safer than files, clipboards, or shell history, but
it is not process isolation.`,
	Example: `  # Inject a password as GITHUB_TOKEN and run gh
  pass-cli exec --set GITHUB_TOKEN=github -- gh repo list

  # Multiple credentials at once
  pass-cli exec --set AWS_ACCESS_KEY_ID=aws-id --set AWS_SECRET_ACCESS_KEY=aws-secret -- aws s3 ls

  # Inject a non-password field
  pass-cli exec --set DB_USER=postgres --field username -- ./run-migration.sh

  # Inject two fields of one entry as separate variables (per-mapping field override)
  pass-cli exec --set DB_USER=postgres:username --set DB_PASSWORD=postgres:password -- ./run-migration.sh

  # Convenience form: service name -> env name (openai-api -> OPENAI_API)
  pass-cli exec openai-api -- python train.py`,
	Args: cobra.ArbitraryArgs,
	RunE: runExec,
}

func init() {
	rootCmd.AddCommand(execCmd)
	execCmd.Flags().StringArrayVar(&execSets, "set", nil, "map an environment variable to a credential: ENV_NAME=service[:field] (repeatable)")
	execCmd.Flags().StringVarP(&execField, "field", "f", "password", "field to inject for all mappings (username, password, category, url, notes, service)")
}

// envMapping pairs a target environment variable name with the credential service
// whose field value should be injected into it. field is the optional per-mapping
// field override ("" means fall back to the global --field flag).
type envMapping struct {
	envName string
	service string
	field   string
}

// parseExecArgs splits the parsed positional args into credential mappings and the
// child command argv. dashIdx is cmd.ArgsLenAtDash(): the number of positional args
// that appeared before the "--" terminator (or -1 if there was no "--").
func parseExecArgs(sets []string, args []string, dashIdx int) (mappings []envMapping, childArgv []string, err error) {
	var preDash []string
	if dashIdx < 0 {
		// No "--" terminator at all: we cannot tell the service from the command.
		return nil, nil, errors.New("no command to run: separate it with '--', e.g. pass-cli exec --set NAME=service -- mycmd")
	}
	preDash = args[:dashIdx]
	childArgv = args[dashIdx:]

	if len(childArgv) == 0 {
		return nil, nil, errors.New("no command to run after '--': specify a command, e.g. pass-cli exec --set NAME=service -- mycmd")
	}

	if len(sets) > 0 {
		if len(preDash) > 0 {
			return nil, nil, fmt.Errorf("cannot combine a positional <service> (%q) with --set; use one form or the other", preDash[0])
		}
		for _, s := range sets {
			name, spec, ok := strings.Cut(s, "=")
			if !ok || name == "" || spec == "" {
				return nil, nil, fmt.Errorf("invalid --set value %q: expected ENV_NAME=service[:field]", s)
			}
			// Optional per-mapping field override: service:field. The first ':'
			// separates the service from the field, so a field always wins when present.
			service, field, hasField := strings.Cut(spec, ":")
			if hasField && (service == "" || field == "") {
				return nil, nil, fmt.Errorf("invalid --set value %q: expected ENV_NAME=service:field", s)
			}
			mappings = append(mappings, envMapping{envName: name, service: service, field: field})
		}
		return mappings, childArgv, nil
	}

	// Convenience form: a single positional service, env name derived from it.
	if len(preDash) != 1 {
		return nil, nil, errors.New("expected exactly one <service> before '--' (or use --set ENV_NAME=service)")
	}
	service := preDash[0]
	envName := deriveEnvName(service)
	if envName == "" {
		return nil, nil, fmt.Errorf("cannot derive an environment variable name from service %q; use --set ENV_NAME=%s", service, service)
	}
	mappings = append(mappings, envMapping{envName: envName, service: service})
	return mappings, childArgv, nil
}

// deriveEnvName converts a service name into an environment variable name:
// uppercased, with every non-alphanumeric ASCII character replaced by '_'.
// e.g. "openai-api" -> "OPENAI_API".
func deriveEnvName(service string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(service) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// runChild executes the child process, inheriting the parent's stdio. It returns
// the child's exit code (0 on success) and a non-nil error only when the child
// could not be started or completed abnormally (e.g. killed by a signal). This
// keeps os.Exit out of the runner so exit-code propagation is unit-testable.
func runChild(childArgv []string, extraEnv []string) (int, error) {
	c := exec.Command(childArgv[0], childArgv[1:]...) // #nosec G204 - command comes from the user's own argv after '--'
	c.Env = append(os.Environ(), extraEnv...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr

	err := c.Run()
	if err == nil {
		return 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		if code < 0 {
			// Terminated by a signal (no portable exit code); surface as an error.
			return 0, fmt.Errorf("command %q terminated abnormally: %w", childArgv[0], err)
		}
		return code, nil
	}
	return 0, fmt.Errorf("failed to execute %q: %w", childArgv[0], err)
}

func runExec(cmd *cobra.Command, args []string) error {
	mappings, childArgv, err := parseExecArgs(execSets, args, cmd.ArgsLenAtDash())
	if err != nil {
		return err
	}

	vaultPath := GetVaultPath()

	// Check if vault exists
	if _, err := os.Stat(vaultPath); os.IsNotExist(err) {
		return fmt.Errorf("vault not found at %s\nRun 'pass-cli init' to create a vault first", vaultPath)
	}

	// Create vault service
	vaultService, err := vault.New(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to create vault service at %s: %w", vaultPath, err)
	}

	// Smart sync pull before unlock to get latest version (read-only: no push after)
	syncPullBeforeUnlock(vaultService)

	// Unlock vault
	if err := unlockVault(vaultService); err != nil {
		return err
	}
	defer vaultService.Lock()

	// Build the child's extra environment. exec is deliberately read-only: it does
	// NOT call RecordFieldAccess or syncPushAfterCommand, so repeated invocations on
	// a hot path don't mutate the vault hash and trigger a sync push every time.
	extraEnv := make([]string, 0, len(mappings))
	for _, m := range mappings {
		// A per-mapping field (--set ENV=service:field) overrides the global --field.
		field := m.field
		if field == "" {
			field = execField
		}
		entry, err := buildEnvEntry(vaultService, m, field)
		if err != nil {
			return err
		}
		extraEnv = append(extraEnv, entry)
	}

	exitCode, err := runChild(childArgv, extraEnv)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		// Execute() flattens any returned error to exit 1, so we must propagate the
		// child's exit code ourselves. Lock explicitly because the deferred Lock will
		// not run after os.Exit (memory zeroing is moot at process exit).
		vaultService.Lock()
		os.Exit(exitCode)
	}
	return nil
}

// buildEnvEntry fetches one credential, resolves the requested field, and returns
// the "ENV_NAME=value" string. The credential's secret bytes are cleared before the
// function returns; the field value is read first, so the wiped bytes never affect
// the returned string.
func buildEnvEntry(vaultService *vault.VaultService, m envMapping, field string) (string, error) {
	// GetCredential returns a deep copy, so clearing Password does not touch the vault.
	cred, err := vaultService.GetCredential(m.service, false)
	if err != nil {
		return "", fmt.Errorf("failed to get credential %q: %w", m.service, err)
	}
	defer crypto.ClearBytes(cred.Password)

	value, _, err := resolveCredentialField(cred, field)
	if err != nil {
		return "", err
	}
	return m.envName + "=" + value, nil
}
