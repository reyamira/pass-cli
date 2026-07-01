package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/envmap"
)

// TestParseExecArgs_EnvFileAllowsEmptyMappings verifies that with an --env-file
// present (envFileCount > 0), no --set or positional service is required.
func TestParseExecArgs_EnvFileAllowsEmptyMappings(t *testing.T) {
	mappings, child, err := parseExecArgs(nil, 1, []string{"server"}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mappings) != 0 {
		t.Errorf("expected no --set mappings, got %v", mappings)
	}
	if len(child) != 1 || child[0] != "server" {
		t.Errorf("child argv = %v, want [server]", child)
	}

	// Without any source (no --set, no positional, no env-file), it still errors.
	if _, _, err := parseExecArgs(nil, 0, []string{"server"}, 0); err == nil {
		t.Error("expected error when no credential source is given")
	}
}

// TestReadEnvFileTemplates covers parsing, comment/blank skipping, and validation.
func TestReadEnvFileTemplates(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.env")
	content := "" +
		"# a comment\n" +
		"\n" +
		"DATABASE_URL=postgres://app:${pass:db/password}@localhost/app\n" +
		"  TOKEN = ${pass:github}  \n" + // surrounding spaces trimmed
		"# trailing comment\n"
	if err := os.WriteFile(good, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	entries, err := readEnvFileTemplates([]string{good})
	if err != nil {
		t.Fatalf("readEnvFileTemplates: %v", err)
	}
	want := []envFileEntry{
		{Key: "DATABASE_URL", Template: "postgres://app:${pass:db/password}@localhost/app"},
		{Key: "TOKEN", Template: "${pass:github}"},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, entries[i], want[i])
		}
	}

	// Missing path is an error.
	if _, err := readEnvFileTemplates([]string{filepath.Join(dir, "nope.env")}); err == nil {
		t.Error("expected error for missing env-file")
	}

	// A line without '=' is an error.
	bad := filepath.Join(dir, "bad.env")
	if err := os.WriteFile(bad, []byte("NO_EQUALS_HERE\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readEnvFileTemplates([]string{bad}); err == nil {
		t.Error("expected error for a line without '='")
	}

	// An invalid env name is an error.
	badName := filepath.Join(dir, "badname.env")
	if err := os.WriteFile(badName, []byte("BAD;NAME=${pass:x}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readEnvFileTemplates([]string{badName}); err == nil {
		t.Error("expected error for invalid env name in env-file")
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
		wantMappings []envmap.Mapping
		wantChild    []string
		wantErr      bool
	}{
		{
			name:         "single --set mapping",
			sets:         []string{"GITHUB_TOKEN=github"},
			args:         []string{"gh", "repo", "list"},
			dashIdx:      0,
			wantMappings: []envmap.Mapping{{EnvName: "GITHUB_TOKEN", Service: "github"}},
			wantChild:    []string{"gh", "repo", "list"},
		},
		{
			name:         "multiple --set mappings",
			sets:         []string{"AWS_ACCESS_KEY_ID=aws-id", "AWS_SECRET_ACCESS_KEY=aws-secret"},
			args:         []string{"aws", "s3", "ls"},
			dashIdx:      0,
			wantMappings: []envmap.Mapping{{EnvName: "AWS_ACCESS_KEY_ID", Service: "aws-id"}, {EnvName: "AWS_SECRET_ACCESS_KEY", Service: "aws-secret"}},
			wantChild:    []string{"aws", "s3", "ls"},
		},
		{
			name:         "convenience form derives env name",
			sets:         nil,
			args:         []string{"openai-api", "python", "train.py"},
			dashIdx:      1,
			wantMappings: []envmap.Mapping{{EnvName: "OPENAI_API", Service: "openai-api"}},
			wantChild:    []string{"python", "train.py"},
		},
		{
			name:         "per-mapping field override",
			sets:         []string{"DB_USER=postgres:username"},
			args:         []string{"mycmd"},
			dashIdx:      0,
			wantMappings: []envmap.Mapping{{EnvName: "DB_USER", Service: "postgres", Field: "username"}},
			wantChild:    []string{"mycmd"},
		},
		{
			name:         "two fields of one entry as separate vars",
			sets:         []string{"DB_USER=pg:username", "DB_PASSWORD=pg:password"},
			args:         []string{"./run.sh"},
			dashIdx:      0,
			wantMappings: []envmap.Mapping{{EnvName: "DB_USER", Service: "pg", Field: "username"}, {EnvName: "DB_PASSWORD", Service: "pg", Field: "password"}},
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
			mappings, child, err := parseExecArgs(tt.sets, 0, tt.args, tt.dashIdx)
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
	parse := func(argv []string) (mappings []envmap.Mapping, child []string, parseErr, execErr error) {
		var sets []string
		c := &cobra.Command{
			Use:  "exec",
			Args: cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				mappings, child, parseErr = parseExecArgs(sets, 0, args, cmd.ArgsLenAtDash())
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
		if !equalMappings(mappings, []envmap.Mapping{{EnvName: "K", Service: "svc"}}) {
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
		if !equalMappings(mappings, []envmap.Mapping{{EnvName: "SVC", Service: "svc"}}) {
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
		if !equalMappings(mappings, []envmap.Mapping{{EnvName: "K", Service: "svc"}}) {
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

func equalMappings(a, b []envmap.Mapping) bool {
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
