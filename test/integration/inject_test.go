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

// TestIntegration_Inject_FileToStdout renders a template file (with a composite
// ${pass:...} reference) to stdout. --in-file leaves stdin free for the password.
func TestIntegration_Inject_FileToStdout(t *testing.T) {
	configPath, password, service, secret := setupExecVault(t)

	tmpl := filepath.Join(t.TempDir(), "conn.tmpl")
	if err := os.WriteFile(tmpl, []byte("url=postgres://app:${pass:"+service+"}@host/db\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, helpers.BuildUnlockStdin(password),
		"inject", "--in-file", tmpl)
	if err != nil {
		t.Fatalf("inject failed: %v\nStderr: %s", err, stderr)
	}
	want := "url=postgres://app:" + secret + "@host/db\n"
	if stdout != want {
		t.Errorf("inject output = %q, want %q", stdout, want)
	}
}

// TestIntegration_Inject_OutFileIs0600 verifies the rendered secret file is created
// owner-only.
func TestIntegration_Inject_OutFileIs0600(t *testing.T) {
	configPath, password, service, secret := setupExecVault(t)

	dir := t.TempDir()
	tmpl := filepath.Join(dir, "in.tmpl")
	outFile := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(tmpl, []byte("TOKEN=${pass:"+service+"/password}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, stderr, err := helpers.RunCmd(t, binaryPath, configPath, helpers.BuildUnlockStdin(password),
		"inject", "--in-file", tmpl, "--out-file", outFile); err != nil {
		t.Fatalf("inject failed: %v\nStderr: %s", err, stderr)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "TOKEN="+secret+"\n" {
		t.Errorf("out-file content = %q, want %q", got, "TOKEN="+secret+"\n")
	}
	// Unix permission bits are not meaningful on Windows (files report 0666).
	if runtime.GOOS != "windows" {
		info, err := os.Stat(outFile)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("out-file perms = %o, want 0600", perm)
		}
	}
}

// TestIntegration_Inject_UnknownRefFailsClosed verifies an unknown reference errors
// and writes nothing.
func TestIntegration_Inject_UnknownRefFailsClosed(t *testing.T) {
	configPath, password, _, _ := setupExecVault(t)

	tmpl := filepath.Join(t.TempDir(), "bad.tmpl")
	if err := os.WriteFile(tmpl, []byte("X=${pass:does-not-exist}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := helpers.RunCmd(t, binaryPath, configPath, helpers.BuildUnlockStdin(password),
		"inject", "--in-file", tmpl)
	if err == nil {
		t.Fatal("expected error for unknown reference, got success")
	}
	if strings.Contains(stdout, "X=") {
		t.Errorf("expected no output on failure, got %q", stdout)
	}
}

// TestIntegration_Exec_EnvFile injects a KEY=<template> env-file into the child and
// verifies the composite value materializes in the child environment.
func TestIntegration_Exec_EnvFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}
	configPath, password, service, secret := setupExecVault(t)

	envFile := filepath.Join(t.TempDir(), ".env.tmpl")
	content := "# db connection\nDATABASE_URL=postgres://app:${pass:" + service + "}@localhost/app\n"
	if err := os.WriteFile(envFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := helpers.RunCmd(t, binaryPath, configPath, helpers.BuildUnlockStdin(password),
		"exec", "--env-file", envFile, "--", "sh", "-c", `printf %s "$DATABASE_URL"`)
	if err != nil {
		t.Fatalf("exec --env-file failed: %v\nStderr: %s", err, stderr)
	}
	want := "postgres://app:" + secret + "@localhost/app"
	if strings.TrimSpace(stdout) != want {
		t.Errorf("env-file injection: stdout = %q, want %q", strings.TrimSpace(stdout), want)
	}
}
