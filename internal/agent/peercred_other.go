//go:build !linux && !darwin

package agent

import "net"

// authorizePeer is a no-op on platforms without a peer-credential implementation
// (Linux and macOS are covered; the Windows named-pipe SID check is future work,
// and the agent's socket server does not run on Windows anyway, so this is never
// actually called there). It must NOT fail-closed — doing so would reject the
// owner's own connections on any such platform.
func authorizePeer(conn net.Conn) error {
	return nil
}
