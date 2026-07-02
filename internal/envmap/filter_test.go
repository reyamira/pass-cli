package envmap

import "testing"

func TestIsKnownFilter(t *testing.T) {
	known := []string{"base64", "base64url", "basicauth"}
	for _, n := range known {
		if !IsKnownFilter(n) {
			t.Errorf("IsKnownFilter(%q) = false, want true", n)
		}
	}
	unknown := []string{"", "bogus", "BASE64", "Base64", "b64", "base64 "}
	for _, n := range unknown {
		if IsKnownFilter(n) {
			t.Errorf("IsKnownFilter(%q) = true, want false", n)
		}
	}
}

func TestApplyValueFilter(t *testing.T) {
	// A value whose std base64 differs from base64url (contains bytes that map to
	// '+' and '/' in the standard alphabet).
	const raw = "\xfb\xff" // std: "+/8=", url: "-_8="
	tests := []struct {
		name    string
		filter  string
		value   string
		want    string
		wantErr bool
	}{
		{name: "base64 ascii", filter: "base64", value: "user:pass", want: "dXNlcjpwYXNz"},
		{name: "base64 std alphabet", filter: "base64", value: raw, want: "+/8="},
		{name: "base64url alphabet", filter: "base64url", value: raw, want: "-_8="},
		{name: "base64 empty", filter: "base64", value: "", want: ""},
		{name: "basicauth rejected here", filter: "basicauth", value: "x", wantErr: true},
		{name: "unknown", filter: "bogus", value: "x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ApplyValueFilter(tt.filter, tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ApplyValueFilter(%q, %q) = %q, want %q", tt.filter, tt.value, got, tt.want)
			}
		})
	}
}
