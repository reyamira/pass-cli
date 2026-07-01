package agent

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/arimxyer/pass-cli/internal/envmap"
	"github.com/arimxyer/pass-cli/internal/resolver"
)

// socketResolver implements resolver.Resolver by asking a running agent over the
// socket. It holds no vault and needs no unlock — that is the whole point: the
// daemon already holds the unlocked vault, so this path skips the prompt and PBKDF2.
type socketResolver struct{ path string }

// DialResolver returns a Resolver backed by a reachable agent, or (nil, false) if
// no agent answers — the caller then falls back to direct-open. This is what makes
// the daemon a transparent optimization rather than a hard dependency.
func DialResolver() (resolver.Resolver, bool) {
	path := SocketPath()
	if path == "" {
		return nil, false
	}
	conn, err := dialSocket(path)
	if err != nil {
		return nil, false
	}
	_ = conn.Close()
	return &socketResolver{path: path}, true
}

func (s *socketResolver) ResolveValues(mappings []envmap.Mapping, defaultField string) ([]string, error) {
	conn, err := dialSocket(s.path)
	if err != nil {
		return nil, fmt.Errorf("agent unreachable: %w", err)
	}
	defer func() { _ = conn.Close() }()

	refs := make([]Ref, len(mappings))
	for i, m := range mappings {
		refs[i] = Ref{Service: m.Service, Field: m.Field}
	}
	req := Request{
		Version: ProtocolVersion,
		Method:  MethodResolve,
		Resolve: &ResolveParams{Refs: refs, DefaultField: defaultField},
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send resolve request: %w", err)
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to read agent response: %w", err)
	}
	if !resp.OK {
		return nil, errors.New(resp.Error)
	}
	if len(resp.Values) != len(mappings) {
		return nil, fmt.Errorf("agent returned %d values for %d references", len(resp.Values), len(mappings))
	}
	return resp.Values, nil
}

func (s *socketResolver) Close() error { return nil }
