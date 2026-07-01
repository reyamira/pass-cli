//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/arimxyer/pass-cli/test/helpers"
)

// TestIntegration_Export_FromManifest exports mappings read from a .pass-cli.toml
// and round-trips the value through a real shell.
func TestIntegration_Export_FromManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}
	configPath, password, service, secret := setupExecVault(t)

	manifest := filepath.Join(t.TempDir(), ".pass-cli.toml")
	if err := os.WriteFile(manifest, []byte("[env]\nMY_TOKEN = \""+service+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, helpers.BuildUnlockStdin(password),
		"export", "--from", manifest)
	if err != nil {
		t.Fatalf("export --from failed: %v\nStderr: %s", err, stderr)
	}
	stmt := strings.TrimSpace(stdout)
	if got := evalSh(t, stmt, "MY_TOKEN"); got != secret {
		t.Errorf("export --from round-trip: got %q, want %q\nstmt: %s", got, secret, stmt)
	}
}

// TestIntegration_Exec_FromManifest injects manifest mappings (with a slash field)
// into a child process.
func TestIntegration_Exec_FromManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}
	configPath, password, service, secret := setupExecVault(t)

	manifest := filepath.Join(t.TempDir(), ".pass-cli.toml")
	content := "[env]\nAPI_KEY = \"" + service + "/password\"\n"
	if err := os.WriteFile(manifest, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, helpers.BuildUnlockStdin(password),
		"exec", "--from", manifest, "--", "sh", "-c", `printf %s "$API_KEY"`)
	if err != nil {
		t.Fatalf("exec --from failed: %v\nStderr: %s", err, stderr)
	}
	if strings.TrimSpace(stdout) != secret {
		t.Errorf("exec --from: stdout = %q, want %q", strings.TrimSpace(stdout), secret)
	}
}

// TestIntegration_Exec_FromManifest_InvalidName surfaces a manifest validation
// error (a bad env name) rather than injecting it.
func TestIntegration_Exec_FromManifest_InvalidName(t *testing.T) {
	configPath, password, service, _ := setupExecVault(t)

	manifest := filepath.Join(t.TempDir(), ".pass-cli.toml")
	if err := os.WriteFile(manifest, []byte("[env]\n\"2BAD\" = \""+service+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, helpers.BuildUnlockStdin(password),
		"exec", "--from", manifest, "--", "true")
	if err == nil {
		t.Fatalf("expected error for invalid manifest env name, got success\nStdout: %s", stdout)
	}
	if !strings.Contains(stderr, "invalid environment variable name") {
		t.Errorf("expected invalid-name error, got stderr: %s", stderr)
	}
}
