//go:build windows

package agent

import (
	"errors"
	"net"
)

// ErrUnsupported is returned by the agent transport on Windows until the named-pipe
// implementation lands (Phase 2f). Until then, Windows clients transparently fall
// back to direct-open (DialResolver returns not-reachable), and `pass-cli agent`
// reports this error.
var ErrUnsupported = errors.New("the pass-cli agent is not yet supported on Windows (named-pipe transport is planned)")

// SocketPath returns "" on Windows so DialResolver treats the agent as unreachable.
func SocketPath() string { return "" }

// Listen is unsupported on Windows until the named-pipe transport lands.
func Listen(path string) (net.Listener, error) { return nil, ErrUnsupported }

func dialSocket(path string) (net.Conn, error) { return nil, ErrUnsupported }
