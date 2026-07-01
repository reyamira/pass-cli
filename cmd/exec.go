package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/envmap"
	"github.com/arimxyer/pass-cli/internal/resolver"
	"github.com/arimxyer/pass-cli/internal/vault"
)

var (
	execSets     []string
	execField    string
	execEnvFiles []string
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
applies to every mapping. A single mapping can override it with a '/field'
suffix, which is how you inject two fields of the same entry as separate
variables (e.g. a database username and password):

  pass-cli exec --set DB_USER=postgres/username --set DB_PASSWORD=postgres/password -- ./run.sh

The legacy ':field' separator ('postgres:password') is still accepted, but '/'
is preferred: in slash form any ':' in the service name is a literal character.

A third source is --env-file: a file of 'KEY=<template>' lines whose values may
embed ${pass:service/field} references (the same templating as 'pass-cli inject').
This resolves composite values an ENV_NAME=service mapping cannot express:

  # .env.tmpl:  DATABASE_URL=postgres://app:${pass:db/password}@localhost/app
  pass-cli exec --env-file .env.tmpl -- ./server

--env-file composes with --set; blank lines and '#' comments are ignored, and each
KEY is validated as an environment variable name.

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
  pass-cli exec --set DB_USER=postgres/username --set DB_PASSWORD=postgres/password -- ./run-migration.sh

  # Convenience form: service name -> env name (openai-api -> OPENAI_API)
  pass-cli exec openai-api -- python train.py`,
	Args: cobra.ArbitraryArgs,
	RunE: runExec,
}

func init() {
	rootCmd.AddCommand(execCmd)
	execCmd.Flags().StringArrayVar(&execSets, "set", nil, "map an environment variable to a credential: ENV_NAME=service[/field] (repeatable; ':field' also accepted)")
	execCmd.Flags().StringVarP(&execField, "field", "f", "password", "field to inject for all mappings (username, password, category, url, notes, service)")
	execCmd.Flags().StringArrayVar(&execEnvFiles, "env-file", nil, "read KEY=${pass:service/field} template lines from a file (repeatable)")
}

// parseExecArgs splits the parsed positional args into credential mappings and the
// child command argv. dashIdx is cmd.ArgsLenAtDash(): the number of positional args
// that appeared before the "--" terminator (or -1 if there was no "--"). The
// per-spec grammar lives in internal/envmap; this function owns only the exec-CLI
// shape (the "--" split and the --set-vs-positional forms). envFileCount is the
// number of --env-file sources: when > 0, an empty --set/positional set is allowed
// because the env-files supply the mappings instead.
func parseExecArgs(sets []string, envFileCount int, args []string, dashIdx int) (mappings []envmap.Mapping, childArgv []string, err error) {
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
			m, perr := envmap.ParseSetSpec(s)
			if perr != nil {
				return nil, nil, perr
			}
			mappings = append(mappings, m)
		}
		return mappings, childArgv, nil
	}

	switch len(preDash) {
	case 0:
		// No --set and no positional: the env-files must supply the mappings.
		if envFileCount == 0 {
			return nil, nil, errors.New("no credentials to inject: use --set ENV_NAME=service, a positional <service>, or --env-file")
		}
		return nil, childArgv, nil
	case 1:
		// Convenience form: a single positional service, env name derived from it.
		service := preDash[0]
		envName := envmap.DeriveEnvName(service)
		if envName == "" {
			return nil, nil, fmt.Errorf("cannot derive an environment variable name from service %q; use --set ENV_NAME=%s", service, service)
		}
		return []envmap.Mapping{{EnvName: envName, Service: service}}, childArgv, nil
	default:
		return nil, nil, errors.New("expected exactly one <service> before '--' (or use --set / --env-file)")
	}
}

// envFileEntry is one KEY=<template> line from an --env-file. Template is the raw
// right-hand side, resolved with envmap.RenderTemplate at injection time.
type envFileEntry struct {
	Key      string
	Template string
}

// readEnvFileTemplates parses KEY=<template> lines from each --env-file, skipping
// blank lines and '#' comments. Each KEY is validated as an environment variable
// name. The template (RHS) is trimmed of surrounding whitespace but otherwise
// preserved verbatim for RenderTemplate.
func readEnvFileTemplates(paths []string) ([]envFileEntry, error) {
	var entries []envFileEntry
	for _, p := range paths {
		data, err := os.ReadFile(p) // #nosec G304 -- user-specified env-file path
		if err != nil {
			return nil, fmt.Errorf("failed to read env-file %q: %w", p, err)
		}
		for n, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			key, tmpl, ok := strings.Cut(line, "=")
			key = strings.TrimSpace(key)
			if !ok || key == "" {
				return nil, fmt.Errorf("%s:%d: expected KEY=value, got %q", p, n+1, line)
			}
			if !envmap.ValidEnvName(key) {
				return nil, fmt.Errorf("%s:%d: invalid environment variable name %q", p, n+1, key)
			}
			entries = append(entries, envFileEntry{Key: key, Template: strings.TrimSpace(tmpl)})
		}
	}
	return entries, nil
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
	mappings, childArgv, err := parseExecArgs(execSets, len(execEnvFiles), args, cmd.ArgsLenAtDash())
	if err != nil {
		return err
	}
	// Read env-file templates (before touching the vault so a read/parse error
	// fails fast). Their values are resolved after unlock.
	envEntries, err := readEnvFileTemplates(execEnvFiles)
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

	// Pull from remote and unlock, overlapping the pull with the password prompt
	// (#103; read-only: no push after).
	if err := unlockVaultWithSync(vaultService); err != nil {
		return err
	}
	defer vaultService.Lock()

	// Build the child's extra environment via the shared resolver. exec is
	// deliberately read-only: the resolver does NOT call RecordFieldAccess or
	// syncPushAfterCommand, so repeated invocations on a hot path don't mutate the
	// vault hash and trigger a sync push every time. The direct backend does not own
	// the vault, so Close is a no-op; runExec owns unlock/Lock.
	r := resolver.NewDirect(vaultService)
	defer func() { _ = r.Close() }()

	extraEnv := make([]string, 0, len(mappings)+len(envEntries))

	setEnv, err := resolver.ResolveEnv(r, mappings, execField)
	if err != nil {
		return err
	}
	extraEnv = append(extraEnv, setEnv...)

	// --env-file entries: render each KEY=<template> value (composite/embedded
	// ${pass:...} references) against the same read-only resolver.
	for _, e := range envEntries {
		value, rerr := envmap.RenderTemplate(e.Template, func(refs []envmap.TemplateRef) ([]string, error) {
			return r.ResolveValues(templateMappings(refs), execField)
		})
		if rerr != nil {
			return fmt.Errorf("env-file key %q: %w", e.Key, rerr)
		}
		extraEnv = append(extraEnv, e.Key+"="+value)
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
