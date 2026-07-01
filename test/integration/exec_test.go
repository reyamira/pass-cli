//go:build integration

package integration

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/arimxyer/pass-cli/test/helpers"
)

// setupExecVault initializes a fresh vault with one known credential and returns
// the config path, the master password, the service name, and the secret value.
func setupExecVault(t *testing.T) (configPath, password, service, secret string) {
	t.Helper()

	password = "Exec-Master-Pass@123"
	service = "exectest"
	secret = "super-secret-exec-value-xyz"

	vaultPath := helpers.SetupTestVault(t)
	configPath, cleanup := helpers.SetupTestVaultConfig(t, vaultPath)
	t.Cleanup(cleanup)

	// Initialize the vault (declines keychain, so exec prompts for the password).
	initStdin := helpers.BuildInitStdin(helpers.DefaultInitOptions(password))
	if _, stderr, err := helpers.RunCmd(t, binaryPath, configPath, initStdin, "init"); err != nil {
		t.Fatalf("init failed: %v\nStderr: %s", err, stderr)
	}

	// Add a credential whose password is the secret we will inject.
	addStdin := helpers.BuildUnlockStdin(password)
	if _, stderr, err := helpers.RunCmd(t, binaryPath, configPath, addStdin,
		"add", service, "--username", "execuser", "--password", secret); err != nil {
		t.Fatalf("add failed: %v\nStderr: %s", err, stderr)
	}

	return configPath, password, service, secret
}

// exitCode extracts the process exit code from a *exec.ExitError (or -1).
func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// TestIntegration_Exec_ExitCodePropagation verifies pass-cli exits with the child's
// exit code (this is the end-to-end pin for runExec's os.Exit call).
func TestIntegration_Exec_ExitCodePropagation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}

	configPath, password, service, _ := setupExecVault(t)

	stdin := helpers.BuildUnlockStdin(password)
	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, stdin,
		"exec", "--set", "K="+service, "--", "sh", "-c", "exit 7")

	if err == nil {
		t.Fatalf("expected non-zero exit, got success\nStdout: %s\nStderr: %s", stdout, stderr)
	}
	if code := exitCode(err); code != 7 {
		t.Errorf("exit code = %d, want 7\nStderr: %s", code, stderr)
	}
}

// TestIntegration_Exec_EnvInjection verifies the secret reaches the child process
// environment under the requested variable name.
func TestIntegration_Exec_EnvInjection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}

	configPath, password, service, secret := setupExecVault(t)

	stdin := helpers.BuildUnlockStdin(password)
	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, stdin,
		"exec", "--set", "MY_TOKEN="+service, "--", "sh", "-c", `printf %s "$MY_TOKEN"`)
	if err != nil {
		t.Fatalf("exec failed: %v\nStderr: %s", err, stderr)
	}
	if strings.TrimSpace(stdout) != secret {
		t.Errorf("injected value: stdout = %q, want %q", strings.TrimSpace(stdout), secret)
	}
}

// TestIntegration_Exec_StdoutHasNoSecret verifies that when the child does not echo
// the secret, pass-cli writes nothing of its own to stdout - the secret never leaks.
func TestIntegration_Exec_StdoutHasNoSecret(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}

	configPath, password, service, secret := setupExecVault(t)

	stdin := helpers.BuildUnlockStdin(password)
	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, stdin,
		"exec", "--set", "MY_TOKEN="+service, "--", "sh", "-c", "echo CHILD_RAN")
	if err != nil {
		t.Fatalf("exec failed: %v\nStderr: %s", err, stderr)
	}
	if strings.TrimSpace(stdout) != "CHILD_RAN" {
		t.Errorf("stdout = %q, want CHILD_RAN", strings.TrimSpace(stdout))
	}
	if strings.Contains(stdout, secret) {
		t.Errorf("secret leaked to stdout: %q", stdout)
	}
}

// TestIntegration_Exec_ConvenienceForm verifies the `exec <service> -- cmd` form
// derives the env name from the service name (exectest -> EXECTEST).
func TestIntegration_Exec_ConvenienceForm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}

	configPath, password, service, secret := setupExecVault(t)

	stdin := helpers.BuildUnlockStdin(password)
	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, stdin,
		"exec", service, "--", "sh", "-c", `printf %s "$EXECTEST"`)
	if err != nil {
		t.Fatalf("exec failed: %v\nStderr: %s", err, stderr)
	}
	if strings.TrimSpace(stdout) != secret {
		t.Errorf("convenience-form injection: stdout = %q, want %q", strings.TrimSpace(stdout), secret)
	}
}

// TestIntegration_Exec_FieldSelection verifies -f/--field selects a non-password field.
func TestIntegration_Exec_FieldSelection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}

	configPath, password, service, _ := setupExecVault(t)

	stdin := helpers.BuildUnlockStdin(password)
	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, stdin,
		"exec", "--set", "DB_USER="+service, "--field", "username", "--", "sh", "-c", `printf %s "$DB_USER"`)
	if err != nil {
		t.Fatalf("exec failed: %v\nStderr: %s", err, stderr)
	}
	if strings.TrimSpace(stdout) != "execuser" {
		t.Errorf("field selection: stdout = %q, want execuser", strings.TrimSpace(stdout))
	}
}

// TestIntegration_Exec_PerMappingField verifies that two fields of the SAME entry
// can be injected as separate variables via the service:field override — the
// database-credentials case (DB_USER + DB_PASSWORD from one entry) that a single
// global --field cannot express.
func TestIntegration_Exec_PerMappingField(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}

	configPath, password, service, secret := setupExecVault(t)

	stdin := helpers.BuildUnlockStdin(password)
	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, stdin,
		"exec",
		"--set", "DB_USER="+service+":username",
		"--set", "DB_PASSWORD="+service+":password",
		"--", "sh", "-c", `printf '%s:%s' "$DB_USER" "$DB_PASSWORD"`)
	if err != nil {
		t.Fatalf("exec failed: %v\nStderr: %s", err, stderr)
	}
	want := "execuser:" + secret
	if strings.TrimSpace(stdout) != want {
		t.Errorf("per-mapping field: stdout = %q, want %q", strings.TrimSpace(stdout), want)
	}
}

// TestIntegration_Exec_PerMappingFieldSlash verifies the preferred slash
// separator (service/field) works end-to-end through the real binary, exactly
// like the colon form above. The colon test stays as the back-compat proof.
func TestIntegration_Exec_PerMappingFieldSlash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}

	configPath, password, service, secret := setupExecVault(t)

	stdin := helpers.BuildUnlockStdin(password)
	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, stdin,
		"exec",
		"--set", "DB_USER="+service+"/username",
		"--set", "DB_PASSWORD="+service+"/password",
		"--", "sh", "-c", `printf '%s:%s' "$DB_USER" "$DB_PASSWORD"`)
	if err != nil {
		t.Fatalf("exec failed: %v\nStderr: %s", err, stderr)
	}
	want := "execuser:" + secret
	if strings.TrimSpace(stdout) != want {
		t.Errorf("slash per-mapping field: stdout = %q, want %q", strings.TrimSpace(stdout), want)
	}
}

// TestIntegration_Exec_MissingDashTerminator verifies a clear error when no "--"
// separates the command.
func TestIntegration_Exec_MissingDashTerminator(t *testing.T) {
	configPath, password, service, _ := setupExecVault(t)

	stdin := helpers.BuildUnlockStdin(password)
	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, stdin,
		"exec", "--set", "K="+service, "somecmd")
	if err == nil {
		t.Fatalf("expected error for missing --, got success\nStdout: %s\nStderr: %s", stdout, stderr)
	}
	if !strings.Contains(stderr, "--") {
		t.Errorf("expected error mentioning '--', got stderr: %s", stderr)
	}
}
