//go:build !linux

package agent

import "net"

// authorizePeer is a no-op on platforms without a peer-credential implementation
// yet: macOS getpeereid and the Windows named-pipe SID check land in Phase 2f.
// Until then the socket's 0600 permissions are the access control. It must NOT
// fail-closed here — doing so would reject the owner's own connections and break
// the agent on these platforms. (On Windows the server never runs, so this is
// never called there.)
func authorizePeer(conn net.Conn) error {
	return nil
}
