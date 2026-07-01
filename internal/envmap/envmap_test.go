package envmap

import "testing"

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
			if got := DeriveEnvName(tt.service); got != tt.want {
				t.Errorf("DeriveEnvName(%q) = %q, want %q", tt.service, got, tt.want)
			}
		})
	}
}

// TestParseSetSpec covers the per-spec grammar: NAME=service, the ':field'
// override, and every error path. This is the colon grammar extracted verbatim
// from cmd/exec.go in Phase 0a; the slash form is added in 0b.
func TestParseSetSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    Mapping
		wantErr bool
	}{
		{name: "service only", spec: "GITHUB_TOKEN=github", want: Mapping{"GITHUB_TOKEN", "github", ""}},
		{name: "field override", spec: "DB_USER=postgres:username", want: Mapping{"DB_USER", "postgres", "username"}},
		{name: "empty field after colon", spec: "K=svc:", wantErr: true},
		{name: "empty service before colon", spec: "K=:username", wantErr: true},
		{name: "no equals", spec: "NOEQUALS", wantErr: true},
		{name: "empty service", spec: "K=", wantErr: true},
		{name: "empty name", spec: "=github", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSetSpec(tt.spec)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseSetSpec(%q) = %+v, want %+v", tt.spec, got, tt.want)
			}
		})
	}
}
