// Package agent implements the in-process core of the background credential agent
// (#116): a mutex-guarded, resident unlocked vault that answers read-only resolve
// requests, with idle/max-TTL auto-locking. This package is transport-agnostic —
// it takes a decoded Request and returns a Response — so all of its logic is unit-
// testable without binding a socket. The socket/named-pipe transport, peer-cred
// auth, and memguard key handling live in later, separately-reviewed changes.
//
// Core security invariant: the agent serves resolved field VALUES only. There is
// no protocol method that returns the master password or the derived key — by
// construction, not by a runtime check (see the Method constants below). A
// compromised client gets the secrets it was already going to inject, nothing more.
package agent

// ProtocolVersion is the wire-protocol version. A client whose version does not
// match is refused (it should fall back to direct-open).
const ProtocolVersion = 1

// Method names. Note there is deliberately NO "get-key" / "get-password" method:
// the key never crosses the wire. Adding one would break the core invariant.
const (
	MethodResolve  = "resolve"  // resolve field values for a batch of mappings
	MethodLock     = "lock"     // zero the resident secrets; the server then stops (socket freed)
	MethodStatus   = "status"   // report unlocked/idle/ttl (never secrets)
	MethodShutdown = "shutdown" // lock and signal the server to stop
)

// Ref is one credential reference in a resolve request: a service and an optional
// field ("" means the request's DefaultField). It carries no env name — the client
// builds "NAME=value" itself after receiving the value — and no value ever travels
// in this direction.
type Ref struct {
	Service string `json:"service"`
	Field   string `json:"field,omitempty"`
}

// ResolveParams are the parameters of a resolve request.
type ResolveParams struct {
	Refs         []Ref  `json:"refs"`
	DefaultField string `json:"default_field,omitempty"`
}

// Request is a single agent request. Method selects the operation; Resolve is set
// only for MethodResolve.
type Request struct {
	Version int            `json:"version"`
	Method  string         `json:"method"`
	Resolve *ResolveParams `json:"resolve,omitempty"`
}

// Status is the non-secret status snapshot returned by MethodStatus.
type Status struct {
	Unlocked               bool  `json:"unlocked"`
	IdleSeconds            int64 `json:"idle_seconds"`
	MaxTTLRemainingSeconds int64 `json:"max_ttl_remaining_seconds"`
}

// Response is the reply to a Request. On success OK is true; for a resolve, Values
// holds one value per requested Ref, in order. On failure OK is false and Error
// carries a message (which may name a service, but never a field value).
type Response struct {
	Version int      `json:"version"`
	OK      bool     `json:"ok"`
	Error   string   `json:"error,omitempty"`
	Values  []string `json:"values,omitempty"`
	Status  *Status  `json:"status,omitempty"`
}

func okResponse() Response {
	return Response{Version: ProtocolVersion, OK: true}
}

func errResponse(msg string) Response {
	return Response{Version: ProtocolVersion, OK: false, Error: msg}
}
