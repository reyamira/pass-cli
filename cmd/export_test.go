package cmd

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/arimxyer/pass-cli/internal/envmap"
)

// adversarialValues are secret values chosen to break naive shell quoting: quotes,
// shell metacharacters, command substitution, backslashes, a newline, and a
// leading dash. If quoting is correct, each must round-trip byte-for-byte.
var adversarialValues = []string{
	"plain-token",
	"has'single",
	`has"double`,
	"has$dollar",
	"$HOME",
	"$(id)",
	"`id`",
	`back\slash`,
	"semi;colon",
	"pipe|amp&",
	"with space",
	"line1\nline2",
	"-leading-dash",
	`all'"$()` + "`\\;",
}

// TestFormatSh_RoundTripsThroughRealShell is the load-bearing quoting test: it
// eval's the emitted statement in a real /bin/sh and checks the variable holds the
// exact secret. Eyeballing the quoting is not enough — this proves it.
func TestFormatSh_RoundTripsThroughRealShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}

	for _, value := range adversarialValues {
		t.Run(value, func(t *testing.T) {
			stmt := formatSh("SECRET_VAR", value)
			// eval the assignment, then print the variable with no added bytes.
			script := stmt + `; printf '%s' "$SECRET_VAR"`
			out, err := exec.Command("sh", "-c", script).Output()
			if err != nil {
				t.Fatalf("sh failed for %q: %v (stmt: %s)", value, err, stmt)
			}
			if string(out) != value {
				t.Errorf("round-trip mismatch\n value = %q\n got   = %q\n stmt  = %s", value, string(out), stmt)
			}
		})
	}
}

// TestFormatSh_NoUnintendedExecution guards specifically against command
// substitution executing: if $(id) or `id` ran, the output would contain "uid=".
func TestFormatSh_NoUnintendedExecution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sh")
	}
	for _, value := range []string{"$(id)", "`id`", "${HOME}"} {
		stmt := formatSh("SECRET_VAR", value)
		out, err := exec.Command("sh", "-c", stmt+`; printf '%s' "$SECRET_VAR"`).Output()
		if err != nil {
			t.Fatalf("sh failed: %v", err)
		}
		if string(out) != value {
			t.Errorf("value %q was interpreted, got %q", value, string(out))
		}
		if strings.Contains(string(out), "uid=") {
			t.Errorf("command substitution executed for %q: %q", value, string(out))
		}
	}
}

// TestFormatFish verifies fish single-quote escaping (\ and ' are the only
// special characters).
func TestFormatFish(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{"plain", `set -gx V 'plain'`},
		{"has'quote", `set -gx V 'has\'quote'`},
		{`back\slash`, `set -gx V 'back\\slash'`},
		{`both\'`, `set -gx V 'both\\\''`},
		{"$dollar", `set -gx V '$dollar'`}, // $ is literal in fish single quotes
	}
	for _, tt := range tests {
		if got := formatFish("V", tt.value); got != tt.want {
			t.Errorf("formatFish(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

// TestFormatPowerShell verifies PowerShell single-quote escaping (double the quote).
func TestFormatPowerShell(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{"plain", `$env:V = 'plain'`},
		{"has'quote", `$env:V = 'has''quote'`},
		{`back\slash`, `$env:V = 'back\slash'`}, // backslash is literal in PS single quotes
	}
	for _, tt := range tests {
		if got := formatPowerShell("V", tt.value); got != tt.want {
			t.Errorf("formatPowerShell(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestExportFormatter(t *testing.T) {
	for _, name := range []string{"sh", "bash", "zsh", "", "fish", "powershell", "pwsh", "PS"} {
		if _, err := exportFormatter(name); err != nil {
			t.Errorf("exportFormatter(%q) unexpected error: %v", name, err)
		}
	}
	if _, err := exportFormatter("cmd.exe"); err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestParseExportArgs(t *testing.T) {
	tests := []struct {
		name        string
		sets        []string
		positionals []string
		want        []envmap.Mapping
		wantErr     bool
	}{
		{
			name: "single --set",
			sets: []string{"GITHUB_TOKEN=github"},
			want: []envmap.Mapping{{EnvName: "GITHUB_TOKEN", Service: "github"}},
		},
		{
			name: "slash field override",
			sets: []string{"DB_PASSWORD=postgres/password"},
			want: []envmap.Mapping{{EnvName: "DB_PASSWORD", Service: "postgres", Field: "password"}},
		},
		{
			name:        "multiple positionals derive names",
			positionals: []string{"openai-api", "github"},
			want:        []envmap.Mapping{{EnvName: "OPENAI_API", Service: "openai-api"}, {EnvName: "GITHUB", Service: "github"}},
		},
		{
			name:        "combine --set and positional",
			sets:        []string{"K=svc"},
			positionals: []string{"extra"},
			wantErr:     true,
		},
		{
			name:    "no args",
			wantErr: true,
		},
		{
			name:    "injection via --set name is rejected",
			sets:    []string{"X;rm -rf ~=github"}, // name "X;rm -rf ~" -> would be shell injection if emitted
			wantErr: true,
		},
		{
			name:        "derived name starting with digit is rejected",
			positionals: []string{"2fa"}, // -> "2FA", not a valid shell name
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExportArgs(tt.sets, tt.positionals)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d mappings, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("mapping %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
