package agent

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/arimxyer/pass-cli/internal/envmap"
	"github.com/arimxyer/pass-cli/internal/resolver"
	"github.com/arimxyer/pass-cli/internal/vault"
)

// Clock abstracts time so auto-lock timers are testable without real sleeps.
type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// Logger records agent events. Its signature accepts an event name and optional
// service names ONLY — there is no parameter through which a field value could be
// passed — so it is structurally incapable of logging a secret. (The protocol
// carries secrets; the log must not.)
type Logger interface {
	Event(event string, services []string)
}

type nopLogger struct{}

func (nopLogger) Event(string, []string) {}

// WriterLogger writes one line per event to W, emitting only the event name and
// service names.
type WriterLogger struct{ W io.Writer }

func (l WriterLogger) Event(event string, services []string) {
	// Best-effort logging; a failed write to stderr must not affect resolve.
	if len(services) > 0 {
		_, _ = fmt.Fprintf(l.W, "[agent] %s services=%v\n", event, services)
	} else {
		_, _ = fmt.Fprintf(l.W, "[agent] %s\n", event)
	}
}

// Options configures an Agent. Zero values mean: no idle lock, no max-TTL,
// system clock, no-op logger.
type Options struct {
	IdleTimeout time.Duration
	MaxTTL      time.Duration
	Clock       Clock
	Logger      Logger
}

// Agent is the mutex-guarded, resident unlocked vault. It answers read-only
// resolve requests and auto-locks on idle/max-TTL. It does not own any transport;
// a server feeds it decoded Requests via Handle.
type Agent struct {
	mu           sync.Mutex
	vs           *vault.VaultService
	clock        Clock
	idleTimeout  time.Duration
	maxTTL       time.Duration
	log          Logger
	unlockedAt   time.Time
	lastActivity time.Time
	locked       bool

	// Revalidating-cache state: the vault file and the (modtime,size) last loaded,
	// so a resolve can detect a sibling process's write and reload before serving.
	vaultPath   string
	lastModTime time.Time
	lastSize    int64
}

// New wraps an already-unlocked VaultService. The caller unlocks the vault (via a
// password prompt or keychain) before constructing the Agent; the Agent then owns
// its lifecycle (Lock on idle/max-TTL/shutdown).
func New(vs *vault.VaultService, opts Options) *Agent {
	clock := opts.Clock
	if clock == nil {
		clock = systemClock{}
	}
	log := opts.Logger
	if log == nil {
		log = nopLogger{}
	}
	now := clock.Now()
	a := &Agent{
		vs:           vs,
		clock:        clock,
		idleTimeout:  opts.IdleTimeout,
		maxTTL:       opts.MaxTTL,
		log:          log,
		unlockedAt:   now,
		lastActivity: now,
		vaultPath:    vs.Path(),
	}
	// Record the vault file's current state so the first change is detectable.
	if fi, err := os.Stat(a.vaultPath); err == nil {
		a.lastModTime = fi.ModTime()
		a.lastSize = fi.Size()
	}
	return a
}

// Handle processes one decoded request under the agent mutex and returns the
// response. Every call first enforces auto-lock, so a request that arrives after
// the idle/max-TTL window finds the agent already locked.
func (a *Agent) Handle(req Request) Response {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Version 0 is treated as "unspecified" and accepted for forward compatibility
	// in tests; any explicit mismatch is refused so the client falls back.
	if req.Version != 0 && req.Version != ProtocolVersion {
		return errResponse(fmt.Sprintf("unsupported protocol version %d (agent speaks %d)", req.Version, ProtocolVersion))
	}

	a.enforceAutoLock()

	switch req.Method {
	case MethodStatus:
		return a.handleStatus()
	case MethodLock:
		a.lockLocked("lock")
		return okResponse()
	case MethodShutdown:
		a.lockLocked("shutdown")
		return okResponse()
	case MethodResolve:
		return a.handleResolve(req.Resolve)
	default:
		return errResponse(fmt.Sprintf("unknown method %q", req.Method))
	}
}

// Locked reports whether the resident vault has been locked (test/introspection).
func (a *Agent) Locked() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.enforceAutoLock()
	return a.locked
}

func (a *Agent) handleResolve(p *ResolveParams) Response {
	if a.locked {
		return errResponse("agent is locked; re-unlock with 'pass-cli agent'")
	}
	if p == nil {
		return errResponse("resolve: missing parameters")
	}

	// Revalidating cache: if a sibling process rewrote the vault, reload before
	// serving so we never hand out a stale snapshot.
	if err := a.refreshIfChanged(); err != nil {
		return errResponse(err.Error())
	}

	mappings := make([]envmap.Mapping, len(p.Refs))
	services := make([]string, len(p.Refs))
	for i, ref := range p.Refs {
		mappings[i] = envmap.Mapping{Service: ref.Service, Field: ref.Field}
		services[i] = ref.Service
	}

	// The resident vault is served through the same strictly read-only resolver the
	// one-shot CLI uses: no usage write, no sync push.
	values, err := resolver.NewDirect(a.vs).ResolveValues(mappings, p.DefaultField)
	if err != nil {
		// The error may name a service, but never a field value.
		a.log.Event("resolve_error", services)
		return errResponse(err.Error())
	}

	a.lastActivity = a.clock.Now()
	a.log.Event("resolve", services) // service names only — never values
	return Response{Version: ProtocolVersion, OK: true, Values: values}
}

func (a *Agent) handleStatus() Response {
	now := a.clock.Now()
	st := &Status{Unlocked: !a.locked}
	if !a.locked {
		st.IdleSeconds = int64(now.Sub(a.lastActivity).Seconds())
		if a.maxTTL > 0 {
			rem := a.maxTTL - now.Sub(a.unlockedAt)
			if rem < 0 {
				rem = 0
			}
			st.MaxTTLRemainingSeconds = int64(rem.Seconds())
		}
	}
	return Response{Version: ProtocolVersion, OK: true, Status: st}
}

// refreshIfChanged reloads the resident vault snapshot when vault.enc changed on
// disk since the last load (a sibling process wrote it). Must be called with a.mu
// held. A transient stat error is ignored (serve the current snapshot). A reload
// failure — the on-disk vault can no longer be decrypted with the held password,
// e.g. the master password was rotated — locks the agent and returns an error;
// the agent never serves a stale snapshot in that case.
func (a *Agent) refreshIfChanged() error {
	fi, err := os.Stat(a.vaultPath)
	if err != nil {
		return nil // transient; serve the current snapshot
	}
	if fi.ModTime().Equal(a.lastModTime) && fi.Size() == a.lastSize {
		return nil // unchanged
	}
	if err := a.vs.Reload(); err != nil {
		a.lockLocked("reload_failed")
		return fmt.Errorf("vault changed on disk and could not be reloaded (locking): %w", err)
	}
	a.lastModTime = fi.ModTime()
	a.lastSize = fi.Size()
	a.log.Event("reloaded", nil)
	return nil
}

// enforceAutoLock locks the resident vault if the idle timeout or max-TTL has
// elapsed. Must be called with a.mu held.
func (a *Agent) enforceAutoLock() {
	if a.locked {
		return
	}
	now := a.clock.Now()
	if a.idleTimeout > 0 && now.Sub(a.lastActivity) >= a.idleTimeout {
		a.lockLocked("idle_lock")
		return
	}
	if a.maxTTL > 0 && now.Sub(a.unlockedAt) >= a.maxTTL {
		a.lockLocked("max_ttl_lock")
	}
}

// lockLocked zeros the resident secrets and marks the agent locked. Idempotent.
// Must be called with a.mu held.
func (a *Agent) lockLocked(reason string) {
	if a.locked {
		return
	}
	a.vs.Lock() // zero secrets via crypto.ClearBytes
	a.locked = true
	a.log.Event(reason, nil)
}
