package agent

import (
	"encoding/json"
	"net"
	"sync"
	"time"
)

// connDeadline bounds how long a single client may take to send its request and
// read its response, so a hung or slow client cannot stall the agent.
const connDeadline = 5 * time.Second

// Server serves an Agent over a net.Listener: one request/response per connection,
// newline-free JSON framed by connection close. The Agent's own mutex serializes
// the actual work, so connections are handled concurrently only to keep a slow
// client from blocking others' accept.
type Server struct {
	agent *Agent
	ln    net.Listener
	log   Logger
	stop  chan struct{}
	once  sync.Once
}

// NewServer wraps an Agent and a bound listener.
func NewServer(a *Agent, ln net.Listener, log Logger) *Server {
	if log == nil {
		log = nopLogger{}
	}
	return &Server{agent: a, ln: ln, log: log, stop: make(chan struct{})}
}

// Serve accepts connections until Stop is called (or the listener errors).
func (s *Server) Serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return // listener closed by Stop, or a fatal accept error
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(connDeadline))

	// Authorize the peer BEFORE reading or acting on any request: only a process
	// owned by the same user as the agent may talk to it (defense-in-depth over the
	// socket's 0600 permissions). Fail-closed on any error obtaining the credential.
	if err := authorizePeer(conn); err != nil {
		s.log.Event("rejected_peer", nil)
		_ = json.NewEncoder(conn).Encode(errResponse("unauthorized"))
		return
	}

	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(errResponse("malformed request"))
		return
	}

	resp := s.agent.Handle(req)
	_ = json.NewEncoder(conn).Encode(resp)

	// Free the socket promptly whenever a request leaves the agent locked — an
	// explicit `lock`/shutdown, or idle/max-TTL enforcement that fired during this
	// request. Serve() then returns and runAgent removes the socket, so clients fall
	// back to direct-open and a fresh `pass-cli agent` can rebind, instead of hitting
	// a locked-but-running agent for up to one expiry tick.
	if s.agent.Locked() {
		s.Stop()
	}
}

// Stop closes the listener, causing Serve to return. Idempotent.
func (s *Server) Stop() {
	s.once.Do(func() {
		close(s.stop)
		_ = s.ln.Close()
	})
}
