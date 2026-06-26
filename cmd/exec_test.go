package cmd

import (
	"bytes"
	"io"
	"os"
	"runtime"
	"testing"

	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/vault"
)

// TestResolveCredentialField verifies the shared field resolver covers every alias
// used by both `get` and `exec`, returns the canonical name, and rejects bad fields.
func TestResolveCredentialField(t *testing.T) {
	cred := &vault.Credential{
		Service:  "github",
		Username: "octocat",
		Password: []byte("s3cr3t-pw"),
		Category: "vcs",
		URL:      "https://github.com",
		Notes:    "personal token",
	}

	tests := []struct {
		field         string
		wantValue     string
		wantCanonical string
	}{
		{"username", "octocat", "username"},
		{"user", "octocat", "username"},
		{"u", "octocat", "username"},
		{"password", "s3cr3t-pw", "password"},
		{"pass", "s3cr3t-pw", "password"},
		{"p", "s3cr3t-pw", "password"},
		{"PASSWORD", "s3cr3t-pw", "password"}, // case-insensitive
		{"category", "vcs", "category"},
		{"cat", "vcs", "category"},
		{"c", "vcs", "category"},
		{"url", "https://github.com", "url"},
		{"notes", "personal token", "notes"},
		{"note", "personal token", "notes"},
		{"n", "personal token", "notes"},
		{"service", "github", "service"},
		{"s", "github", "service"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			value, canonical, err := resolveCredentialField(cred, tt.field)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if value != tt.wantValue {
				t.Errorf("value = %q, want %q", value, tt.wantValue)
			}
			if canonical != tt.wantCanonical {
				t.Errorf("canonical = %q, want %q", canonical, tt.wantCanonical)
			}
		})
	}

	t.Run("invalid field", func(t *testing.T) {
		_, _, err := resolveCredentialField(cred, "totp")
		if err == nil {
			t.Fatal("expected error for invalid field, got nil")
		}
	})
}

// TestDeriveEnvName verifies service -> env var name derivation.
func TestDeriveEnvName(t *testing.T) {
	tests := []struct {
		service string
		want    string
	}{
		{"openai-api", "OPENAI_API"},
		{"github", "GITHUB"},
		{"aws.prod", "AWS_PROD"},
		{"my service", "MY_SERVICE"},
		{"api/v2:key", "API_V2_KEY"},
		{"already_ok", "ALREADY_OK"},
		{"GH123", "GH123"},
	}

	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			if got := deriveEnvName(tt.service); got != tt.want {
				t.Errorf("deriveEnvName(%q) = %q, want %q", tt.service, got, tt.want)
			}
		})
	}
}

// TestParseExecArgs covers the credential-mapping / child-argv split, the
// convenience form, and every error path, using a hand-set dash index.
func TestParseExecArgs(t *testing.T) {
	tests := []struct {
		name         string
		sets         []string
		args         []string
		dashIdx      int
		wantMappings []envMapping
		wantChild    []string
		wantErr      bool
	}{
		{
			name:         "single --set mapping",
			sets:         []string{"GITHUB_TOKEN=github"},
			args:         []string{"gh", "repo", "list"},
			dashIdx:      0,
			wantMappings: []envMapping{{"GITHUB_TOKEN", "github", ""}},
			wantChild:    []string{"gh", "repo", "list"},
		},
		{
			name:         "multiple --set mappings",
			sets:         []string{"AWS_ACCESS_KEY_ID=aws-id", "AWS_SECRET_ACCESS_KEY=aws-secret"},
			args:         []string{"aws", "s3", "ls"},
			dashIdx:      0,
			wantMappings: []envMapping{{"AWS_ACCESS_KEY_ID", "aws-id", ""}, {"AWS_SECRET_ACCESS_KEY", "aws-secret", ""}},
			wantChild:    []string{"aws", "s3", "ls"},
		},
		{
			name:         "convenience form derives env name",
			sets:         nil,
			args:         []string{"openai-api", "python", "train.py"},
			dashIdx:      1,
			wantMappings: []envMapping{{"OPENAI_API", "openai-api", ""}},
			wantChild:    []string{"python", "train.py"},
		},
		{
			name:         "per-mapping field override",
			sets:         []string{"DB_USER=postgres:username"},
			args:         []string{"mycmd"},
			dashIdx:      0,
			wantMappings: []envMapping{{"DB_USER", "postgres", "username"}},
			wantChild:    []string{"mycmd"},
		},
		{
			name:         "two fields of one entry as separate vars",
			sets:         []string{"DB_USER=pg:username", "DB_PASSWORD=pg:password"},
			args:         []string{"./run.sh"},
			dashIdx:      0,
			wantMappings: []envMapping{{"DB_USER", "pg", "username"}, {"DB_PASSWORD", "pg", "password"}},
			wantChild:    []string{"./run.sh"},
		},
		{
			name:    "empty field after colon",
			sets:    []string{"K=svc:"},
			args:    []string{"mycmd"},
			dashIdx: 0,
			wantErr: true,
		},
		{
			name:    "empty service before colon",
			sets:    []string{"K=:username"},
			args:    []string{"mycmd"},
			dashIdx: 0,
			wantErr: true,
		},
		{
			name:    "missing -- terminator",
			sets:    []string{"K=svc"},
			args:    []string{"mycmd"},
			dashIdx: -1,
			wantErr: true,
		},
		{
			name:    "no command after --",
			sets:    []string{"K=svc"},
			args:    []string{},
			dashIdx: 0,
			wantErr: true,
		},
		{
			name:    "positional service combined with --set",
			sets:    []string{"K=svc"},
			args:    []string{"extra", "mycmd"},
			dashIdx: 1,
			wantErr: true,
		},
		{
			name:    "invalid --set value (no =)",
			sets:    []string{"NOEQUALS"},
			args:    []string{"mycmd"},
			dashIdx: 0,
			wantErr: true,
		},
		{
			name:    "invalid --set value (empty service)",
			sets:    []string{"K="},
			args:    []string{"mycmd"},
			dashIdx: 0,
			wantErr: true,
		},
		{
			name:    "convenience form with multiple positionals",
			sets:    nil,
			args:    []string{"a", "b", "mycmd"},
			dashIdx: 2,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mappings, child, err := parseExecArgs(tt.sets, tt.args, tt.dashIdx)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got mappings=%v child=%v", mappings, child)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalMappings(mappings, tt.wantMappings) {
				t.Errorf("mappings = %v, want %v", mappings, tt.wantMappings)
			}
			if !equalStrings(child, tt.wantChild) {
				t.Errorf("child argv = %v, want %v", child, tt.wantChild)
			}
		})
	}
}

// TestExecCmd_ArgsLenAtDash verifies the real cobra parse path: that the dash index
// returned by cmd.ArgsLenAtDash() splits the positional args exactly where
// parseExecArgs assumes. This pins our one external assumption empirically.
func TestExecCmd_ArgsLenAtDash(t *testing.T) {
	parse := func(argv []string) (mappings []envMapping, child []string, parseErr, execErr error) {
		var sets []string
		c := &cobra.Command{
			Use:  "exec",
			Args: cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				mappings, child, parseErr = parseExecArgs(sets, args, cmd.ArgsLenAtDash())
				return nil
			},
		}
		c.Flags().StringArrayVar(&sets, "set", nil, "")
		c.Flags().StringP("field", "f", "password", "")
		c.SetArgs(argv)
		c.SetOut(io.Discard)
		c.SetErr(io.Discard)
		execErr = c.Execute()
		return
	}

	t.Run("set form splits at --", func(t *testing.T) {
		mappings, child, parseErr, execErr := parse([]string{"--set", "K=svc", "--", "mycmd", "arg1"})
		if execErr != nil || parseErr != nil {
			t.Fatalf("execErr=%v parseErr=%v", execErr, parseErr)
		}
		if !equalMappings(mappings, []envMapping{{"K", "svc", ""}}) {
			t.Errorf("mappings = %v", mappings)
		}
		if !equalStrings(child, []string{"mycmd", "arg1"}) {
			t.Errorf("child = %v, want [mycmd arg1]", child)
		}
	})

	t.Run("convenience form splits at --", func(t *testing.T) {
		mappings, child, parseErr, execErr := parse([]string{"svc", "--", "mycmd"})
		if execErr != nil || parseErr != nil {
			t.Fatalf("execErr=%v parseErr=%v", execErr, parseErr)
		}
		if !equalMappings(mappings, []envMapping{{"SVC", "svc", ""}}) {
			t.Errorf("mappings = %v", mappings)
		}
		if !equalStrings(child, []string{"mycmd"}) {
			t.Errorf("child = %v, want [mycmd]", child)
		}
	})

	t.Run("child flags after -- are not parsed as exec flags", func(t *testing.T) {
		mappings, child, parseErr, execErr := parse([]string{"--set", "K=svc", "--", "mycmd", "--child-flag", "v"})
		if execErr != nil || parseErr != nil {
			t.Fatalf("execErr=%v parseErr=%v", execErr, parseErr)
		}
		if !equalMappings(mappings, []envMapping{{"K", "svc", ""}}) {
			t.Errorf("mappings = %v", mappings)
		}
		if !equalStrings(child, []string{"mycmd", "--child-flag", "v"}) {
			t.Errorf("child = %v", child)
		}
	})

	t.Run("missing -- surfaces parse error", func(t *testing.T) {
		_, _, parseErr, execErr := parse([]string{"--set", "K=svc", "mycmd"})
		if execErr != nil {
			t.Fatalf("execErr=%v", execErr)
		}
		if parseErr == nil {
			t.Fatal("expected parse error for missing --, got nil")
		}
	})
}

// TestRunChild_ExitCodePropagation verifies the child's non-zero exit code is
// returned unchanged (this is the value runExec then passes to os.Exit).
func TestRunChild_ExitCodePropagation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh; covered end-to-end by the integration suite")
	}

	code, err := runChild([]string{"sh", "-c", "exit 7"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 7 {
		t.Errorf("exit code = %d, want 7", code)
	}
}

// TestRunChild_Success verifies a clean child exit yields code 0 and no error.
func TestRunChild_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh; covered end-to-end by the integration suite")
	}

	code, err := runChild([]string{"sh", "-c", "exit 0"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// TestRunChild_EnvInjectionAndStdoutNoSecret verifies that the injected env var
// reaches the child, and that pass-cli itself writes nothing of its own to stdout:
// when the child does not echo the secret, the secret never appears on stdout.
func TestRunChild_EnvInjectionAndStdoutNoSecret(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh; covered end-to-end by the integration suite")
	}

	const secret = "super-secret-value-xyz"

	// Sub-case 1: child echoes the env var -> proves injection works.
	out := captureStdout(t, func() {
		code, err := runChild([]string{"sh", "-c", `printf %s "$MYVAR"`}, []string{"MYVAR=" + secret})
		if err != nil || code != 0 {
			t.Fatalf("runChild failed: code=%d err=%v", code, err)
		}
	})
	if out != secret {
		t.Errorf("env injection: stdout = %q, want %q", out, secret)
	}

	// Sub-case 2: child does NOT echo the secret -> stdout has only child output,
	// and the secret never appears (pass-cli prints nothing of its own).
	out = captureStdout(t, func() {
		code, err := runChild([]string{"sh", "-c", "echo CHILD_RAN"}, []string{"MYVAR=" + secret})
		if err != nil || code != 0 {
			t.Fatalf("runChild failed: code=%d err=%v", code, err)
		}
	})
	if got := trimNewline(out); got != "CHILD_RAN" {
		t.Errorf("stdout = %q, want %q", got, "CHILD_RAN")
	}
	if bytes.Contains([]byte(out), []byte(secret)) {
		t.Errorf("secret leaked to stdout: %q", out)
	}
}

// --- test helpers ---

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = orig
	out := <-done
	_ = r.Close()
	return out
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func equalMappings(a, b []envMapping) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
