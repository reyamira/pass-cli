package vault

import (
	"fmt"
	"strings"
)

// ResolveCredentialField returns the requested field's value from a credential
// along with its canonical name (for usage tracking). It is the single source of
// truth for the field aliases accepted by `get` and `exec`, keeping the valid-field
// list from drifting between the two commands.
//
// Security note: for the password field this returns string(cred.Password); the
// caller is responsible for clearing the source []byte (crypto.ClearBytes) and for
// never printing or copying the value where the brief forbids it.
func ResolveCredentialField(cred *Credential, field string) (value string, canonical string, err error) {
	switch strings.ToLower(field) {
	case "username", "user", "u":
		return cred.Username, "username", nil
	case "password", "pass", "p":
		return string(cred.Password), "password", nil
	case "category", "cat", "c":
		return cred.Category, "category", nil
	case "url":
		return cred.URL, "url", nil
	case "notes", "note", "n":
		return cred.Notes, "notes", nil
	case "service", "s":
		return cred.Service, "service", nil
	default:
		return "", "", fmt.Errorf("invalid field: %s (valid: username, password, category, url, notes, service)", field)
	}
}
