//go:build integration

package integration

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/arimxyer/pass-cli/test/helpers"
)

// evalSh eval's a shell statement in a real /bin/sh and returns the named
// variable's value with no added bytes.
func evalSh(t *testing.T, stmt, varName string) string {
	t.Helper()
	out, err := exec.Command("sh", "-c", stmt+`; printf '%s' "$`+varName+`"`).Output()
	if err != nil {
		t.Fatalf("sh eval failed for %q: %v", stmt, err)
	}
	return string(out)
}

// TestIntegration_Export_RoundTrip proves the full path: the binary emits an
// export statement, and eval'ing it in a real shell yields the exact secret.
func TestIntegration_Export_RoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}
	configPath, password, service, secret := setupExecVault(t)

	stdin := helpers.BuildUnlockStdin(password)
	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, stdin,
		"export", "--set", "MY_TOKEN="+service)
	if err != nil {
		t.Fatalf("export failed: %v\nStderr: %s", err, stderr)
	}
	stmt := strings.TrimSpace(stdout)
	if got := evalSh(t, stmt, "MY_TOKEN"); got != secret {
		t.Errorf("eval round-trip: got %q, want %q\nstmt: %s", got, secret, stmt)
	}
}

// TestIntegration_Export_AdversarialSecret round-trips a secret whose value
// contains quotes and command substitution — the end-to-end quoting proof.
func TestIntegration_Export_AdversarialSecret(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}
	configPath, password, _, _ := setupExecVault(t)
	tricky := `a'b"c$(id)` + "`id`" + `\x;|&`

	if _, stderr, err := helpers.RunCmd(t, binaryPath, configPath, helpers.BuildUnlockStdin(password),
		"add", "trickysvc", "--username", "u", "--password", tricky); err != nil {
		t.Fatalf("add failed: %v\nStderr: %s", err, stderr)
	}

	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, helpers.BuildUnlockStdin(password),
		"export", "--set", "TRICKY=trickysvc")
	if err != nil {
		t.Fatalf("export failed: %v\nStderr: %s", err, stderr)
	}
	stmt := strings.TrimSpace(stdout)
	got := evalSh(t, stmt, "TRICKY")
	if got != tricky {
		t.Errorf("adversarial round-trip: got %q, want %q\nstmt: %s", got, tricky, stmt)
	}
	if strings.Contains(got, "uid=") {
		t.Errorf("command substitution executed: %q", got)
	}
}

// TestIntegration_Export_FishFormat verifies the fish syntax is emitted.
func TestIntegration_Export_FishFormat(t *testing.T) {
	configPath, password, service, secret := setupExecVault(t)

	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, helpers.BuildUnlockStdin(password),
		"export", "--set", "TOK="+service, "--format", "fish")
	if err != nil {
		t.Fatalf("export failed: %v\nStderr: %s", err, stderr)
	}
	want := "set -gx TOK '" + secret + "'" // secret has no fish-special chars
	if strings.TrimSpace(stdout) != want {
		t.Errorf("fish format: got %q, want %q", strings.TrimSpace(stdout), want)
	}
}

// TestIntegration_Export_RejectsInvalidName verifies a shell-injecting env name is
// rejected before the vault is even opened.
func TestIntegration_Export_RejectsInvalidName(t *testing.T) {
	configPath, password, service, _ := setupExecVault(t)

	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, helpers.BuildUnlockStdin(password),
		"export", "--set", "BAD;NAME="+service)
	if err == nil {
		t.Fatalf("expected error for invalid env name, got success\nStdout: %s", stdout)
	}
	if !strings.Contains(stderr, "invalid environment variable name") {
		t.Errorf("expected invalid-name error, got stderr: %s", stderr)
	}
}
