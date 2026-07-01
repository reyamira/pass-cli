package envmap

import "testing"

func TestParseManifest(t *testing.T) {
	data := []byte(`
[env]
GITHUB_TOKEN = "github"
DB_PASSWORD  = "postgres/password"
DB_USER      = "postgres/username"
`)
	got, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	// Sorted by env name for deterministic output.
	want := []Mapping{
		{EnvName: "DB_PASSWORD", Service: "postgres", Field: "password"},
		{EnvName: "DB_USER", Service: "postgres", Field: "username"},
		{EnvName: "GITHUB_TOKEN", Service: "github", Field: ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d mappings, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("mapping %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseManifest_Empty(t *testing.T) {
	for _, data := range [][]byte{[]byte(""), []byte("[env]\n")} {
		got, err := ParseManifest(data)
		if err != nil {
			t.Errorf("ParseManifest(%q): %v", data, err)
		}
		if len(got) != 0 {
			t.Errorf("expected no mappings, got %+v", got)
		}
	}
}

func TestParseManifest_Errors(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"invalid toml", `[env}`},
		{"invalid env name", `[env]` + "\n" + `"2FA" = "svc"`},
		{"invalid name with semicolon", `[env]` + "\n" + `"BAD;NAME" = "svc"`},
		{"multi-segment ref", `[env]` + "\n" + `X = "vault/svc/field"`},
		{"empty ref", `[env]` + "\n" + `X = ""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseManifest([]byte(tt.data)); err == nil {
				t.Errorf("expected error for %s", tt.name)
			}
		})
	}
}
