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

// TestParseSetSpec covers the per-spec grammar for both separators. The colon
// cases are the Phase 0a behavior and must stay green as the back-compat proof;
// the slash cases are the additive Phase 0b form.
func TestParseSetSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    Mapping
		wantErr bool
	}{
		// --- legacy colon form (back-compat, unchanged) ---
		{name: "service only", spec: "GITHUB_TOKEN=github", want: Mapping{"GITHUB_TOKEN", "github", ""}},
		{name: "colon field override", spec: "DB_USER=postgres:username", want: Mapping{"DB_USER", "postgres", "username"}},
		{name: "empty field after colon", spec: "K=svc:", wantErr: true},
		{name: "empty service before colon", spec: "K=:username", wantErr: true},
		{name: "no equals", spec: "NOEQUALS", wantErr: true},
		{name: "empty service", spec: "K=", wantErr: true},
		{name: "empty name", spec: "=github", wantErr: true},
		// --- additive slash form (Phase 0b) ---
		{name: "slash field", spec: "DB_USER=postgres/username", want: Mapping{"DB_USER", "postgres", "username"}},
		{name: "slash colon is literal in service", spec: "URL=my:svc/password", want: Mapping{"URL", "my:svc", "password"}},
		{name: "slash empty field", spec: "K=svc/", wantErr: true},
		{name: "slash empty service", spec: "K=/field", wantErr: true},
		{name: "slash multi-segment reserved", spec: "K=vault/svc/field", wantErr: true},
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

// TestValidEnvName covers the shell-safe env-name gate export relies on.
func TestValidEnvName(t *testing.T) {
	valid := []string{"X", "_", "GITHUB_TOKEN", "_leading", "A1", "MY_VAR_2"}
	invalid := []string{"", "2FA", "1", "HAS-DASH", "HAS SPACE", "HAS;SEMI", "a.b", "a/b", "X=Y"}
	for _, n := range valid {
		if !ValidEnvName(n) {
			t.Errorf("ValidEnvName(%q) = false, want true", n)
		}
	}
	for _, n := range invalid {
		if ValidEnvName(n) {
			t.Errorf("ValidEnvName(%q) = true, want false", n)
		}
	}
}

// TestSplitPath exercises the shared separator directly, including the
// slash-wins / colon-literal rule that later surfaces (manifest, templates) rely on.
func TestSplitPath(t *testing.T) {
	tests := []struct {
		ref       string
		wantSvc   string
		wantField string
		wantErr   bool
	}{
		{ref: "github", wantSvc: "github", wantField: ""},
		{ref: "postgres:username", wantSvc: "postgres", wantField: "username"},
		{ref: "postgres/username", wantSvc: "postgres", wantField: "username"},
		{ref: "my:svc/password", wantSvc: "my:svc", wantField: "password"}, // colon literal in slash mode
		{ref: "svc/", wantErr: true},
		{ref: "/field", wantErr: true},
		{ref: "vault/svc/field", wantErr: true}, // 3+ segments reserved
		{ref: "svc:", wantErr: true},
		{ref: ":field", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			svc, field, err := SplitPath(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got service=%q field=%q", svc, field)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if svc != tt.wantSvc || field != tt.wantField {
				t.Errorf("SplitPath(%q) = (%q, %q), want (%q, %q)", tt.ref, svc, field, tt.wantSvc, tt.wantField)
			}
		})
	}
}
