package envmap

import (
	"encoding/base64"
	"fmt"
)

// FilterBasicAuth is the one compound filter: it combines a credential's own
// username and password into base64("username:password"). It is NOT a value
// transform (it needs two fields), so it is applied in the resolver rather than
// by ApplyValueFilter — envmap only recognizes the name so "| basicauth" parses.
const FilterBasicAuth = "basicauth"

// valueFilters are pure string->string transforms applied to a single resolved
// value after it comes back from the resolver. Keeping them in one map is the
// single source of truth: IsKnownFilter and ApplyValueFilter both derive from
// it, so parse-time validation and apply-time transformation cannot drift.
var valueFilters = map[string]func(string) string{
	"base64":    func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) },
	"base64url": func(s string) string { return base64.URLEncoding.EncodeToString([]byte(s)) },
}

// IsKnownFilter reports whether name is a recognized filter, matched exactly
// (lowercase). Used at parse time to reject an unknown filter before the vault
// is opened (fail closed). The set is the value filters plus basicauth.
func IsKnownFilter(name string) bool {
	if name == FilterBasicAuth {
		return true
	}
	_, ok := valueFilters[name]
	return ok
}

// ApplyValueFilter applies a value transform to a single resolved value. It
// handles only the value filters; basicauth is a compound filter resolved in the
// resolver and is rejected here. An unknown name is an error (fail closed).
func ApplyValueFilter(name, value string) (string, error) {
	fn, ok := valueFilters[name]
	if !ok {
		return "", fmt.Errorf("unknown value filter %q", name)
	}
	return fn(value), nil
}
