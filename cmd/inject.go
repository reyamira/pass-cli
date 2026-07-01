package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/envmap"
)

var (
	injectInFile  string
	injectOutFile string
	injectField   string
)

var injectCmd = &cobra.Command{
	Use:     "inject",
	GroupID: "credentials",
	Short:   "Render a template, replacing ${pass:service/field} references with secrets",
	Long: `Inject reads a template and writes it back with every ${pass:service/field}
reference replaced by the credential's value. It is the composite-secret tool: it
resolves references embedded in arbitrary text, so a whole config file or a single
connection string can be materialized in one step.

  # A connection string with an embedded secret
  echo 'postgres://app:${pass:db/password}@localhost/app' | pass-cli inject

  # A whole config file (references only in the committed template)
  pass-cli inject -i config.tmpl -o config.ini

Reference syntax:
  ${pass:service}          the service's default field (see -f/--field)
  ${pass:service/field}    a specific field (username, password, url, ...)

Only ${pass:...} is special; $VAR, ${VAR}, and $(...) pass through untouched. An
unknown or malformed reference is a hard error and nothing is written.

When the template is piped on stdin, unlock via the OS keychain: the master-
password prompt also reads stdin, so a password prompt would consume the piped
template. Use --in-file if you need the interactive password prompt.

Security note: the rendered output contains plaintext secrets. When --out-file is
used the file is created with 0600 permissions; when writing to stdout the secret
is on your terminal/pipe. Prefer 'exec' (child-scoped env) when you can; use
'inject' for the composite/derived-secret cases 'exec' cannot express.`,
	Example: `  # Materialize a connection string
  echo 'redis://:${pass:redis/password}@cache:6379' | pass-cli inject

  # Render a template file to a 0600 output file
  pass-cli inject -i .env.tmpl -o .env`,
	Args: cobra.NoArgs,
	RunE: runInject,
}

func init() {
	rootCmd.AddCommand(injectCmd)
	injectCmd.Flags().StringVarP(&injectInFile, "in-file", "i", "", "template file to read (default: stdin)")
	injectCmd.Flags().StringVarP(&injectOutFile, "out-file", "o", "", "file to write the rendered output to, created 0600 (default: stdout)")
	injectCmd.Flags().StringVarP(&injectField, "field", "f", "password", "default field for references without an explicit /field")
}

func runInject(cmd *cobra.Command, _ []string) error {
	// A --in-file template is read before unlock so a bad path fails fast. A stdin
	// template is read AFTER unlock instead: the master-password prompt also reads
	// stdin, so reading the template first would consume the password. With keychain
	// unlock (no prompt) the full stdin is then available as the template; with a
	// password prompt, use --in-file so stdin carries the password.
	var (
		input []byte
		err   error
	)
	if injectInFile != "" {
		input, err = os.ReadFile(injectInFile) // #nosec G304 -- user-specified template path
		if err != nil {
			return fmt.Errorf("failed to read template %q: %w", injectInFile, err)
		}
	}

	// Acquire the resolver (a running agent, else open+unlock the vault) BEFORE
	// reading a stdin template: with direct unlock the password prompt reads stdin,
	// so the template must be read after. With an agent there is no prompt.
	r, cleanup, err := acquireResolver()
	if err != nil {
		return err
	}
	defer cleanup()

	if injectInFile == "" {
		input, err = io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("failed to read template from stdin: %w", err)
		}
	}

	// Read-only via the shared resolver (no usage write, no sync push). All
	// references in the template resolve in a single batch call.
	rendered, err := envmap.RenderTemplate(string(input), func(refs []envmap.TemplateRef) ([]string, error) {
		return r.ResolveValues(templateMappings(refs), injectField)
	})
	if err != nil {
		return err
	}

	if injectOutFile != "" {
		// The rendered output contains plaintext secrets: 0600, owner-only.
		if err := os.WriteFile(injectOutFile, []byte(rendered), 0600); err != nil {
			return fmt.Errorf("failed to write %q: %w", injectOutFile, err)
		}
		return nil
	}
	if _, err := io.WriteString(cmd.OutOrStdout(), rendered); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}
	return nil
}

// templateMappings converts template references into resolver mappings (EnvName is
// unused for value resolution).
func templateMappings(refs []envmap.TemplateRef) []envmap.Mapping {
	ms := make([]envmap.Mapping, len(refs))
	for i, ref := range refs {
		ms[i] = envmap.Mapping{Service: ref.Service, Field: ref.Field}
	}
	return ms
}
