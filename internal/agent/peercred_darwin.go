//go:build darwin

package agent

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// authorizePeer rejects a connection whose peer process is not owned by the same
// user as the agent — the macOS counterpart of the Linux SO_PEERCRED check, using
// the LOCAL_PEERCRED socket option. Defense-in-depth over the socket's 0600
// permissions. Any failure to obtain the peer credential is a rejection
// (fail-closed).
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

// peerUID returns the connecting peer's user id via getsockopt(LOCAL_PEERCRED),
// which yields an xucred whose first uid is the peer's effective uid.
func peerUID(conn net.Conn) (uint32, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("connection is not a unix socket")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, err
	}
	var xucred *unix.Xucred
	var credErr error
	if ctrlErr := raw.Control(func(fd uintptr) {
		xucred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	}); ctrlErr != nil {
		return 0, ctrlErr
	}
	if credErr != nil {
		return 0, credErr
	}
	return xucred.Uid, nil
}
