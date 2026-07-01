//go:build linux || darwin

package agent

// authorizedUID reports whether a connecting peer's uid may use the agent: only
// the agent owner. It is the shared authorization decision for the unix-socket
// platforms (Linux SO_PEERCRED, macOS LOCAL_PEERCRED), separated from the
// platform-specific syscall that fetches the uid so the decision is unit-testable
// table-driven, independent of any real socket.
func authorizedUID(peerUID, ownerUID uint32) bool {
	return peerUID == ownerUID
}
