//go:build linux

package agent

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// authorizePeer rejects a connection whose peer process is not owned by the same
// user as the agent. It is defense-in-depth on top of the socket's 0600
// permissions (§5.3). Any failure to obtain the peer credential is treated as a
// rejection — fail-closed, never default-open.
func authorizePeer(conn net.Conn) error {
	uid, err := peerUID(conn)
	if err != nil {
		return fmt.Errorf("cannot verify peer credentials: %w", err)
	}
	if !authorizedUID(uid, uint32(os.Getuid())) {
		return fmt.Errorf("peer uid %d is not the agent owner", uid)
	}
	return nil
}

// peerUID returns the connecting peer's user id via SO_PEERCRED.
func peerUID(conn net.Conn) (uint32, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("connection is not a unix socket")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, err
	}
	var ucred *unix.Ucred
	var credErr error
	if ctrlErr := raw.Control(func(fd uintptr) {
		ucred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); ctrlErr != nil {
		return 0, ctrlErr
	}
	if credErr != nil {
		return 0, credErr
	}
	return ucred.Uid, nil
}

// authorizedUID reports whether a connecting peer's uid may use the agent: only
// the agent owner. Separated from the syscall above so the decision is unit-
// testable table-driven, independent of any real socket.
func authorizedUID(peerUID, ownerUID uint32) bool {
	return peerUID == ownerUID
}
