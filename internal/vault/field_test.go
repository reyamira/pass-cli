package vault

import "testing"

// TestResolveCredentialField verifies the shared field resolver covers every alias
// used by both `get` and `exec`, returns the canonical name, and rejects bad fields.
func TestResolveCredentialField(t *testing.T) {
	cred := &Credential{
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
			value, canonical, err := ResolveCredentialField(cred, tt.field)
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
		_, _, err := ResolveCredentialField(cred, "totp")
		if err == nil {
			t.Fatal("expected error for invalid field, got nil")
		}
	})
}
