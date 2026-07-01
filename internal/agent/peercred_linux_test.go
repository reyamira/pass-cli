//go:build linux

package agent

import (
	"net"
	"os"
	"testing"
)

func TestAuthorizedUID(t *testing.T) {
	owner := uint32(os.Getuid())
	if !authorizedUID(owner, owner) {
		t.Error("owner uid should be authorized")
	}
	if authorizedUID(owner+1, owner) {
		t.Error("a different uid must be rejected")
	}
	if authorizedUID(0, owner) && owner != 0 {
		t.Error("root peer must not be treated as owner unless it is the owner")
	}
}

// TestPeerUID_SameUser dials a real unix socket in-process and verifies the peer
// uid is our own uid.
func TestPeerUID_SameUser(t *testing.T) {
	dir := shortSocketDir(t)
	path := dir + "/p.sock"
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- c
		}
	}()

	client, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = client.Close() }()

	srvConn := <-accepted
	defer func() { _ = srvConn.Close() }()

	uid, err := peerUID(srvConn)
	if err != nil {
		t.Fatalf("peerUID: %v", err)
	}
	if uid != uint32(os.Getuid()) {
		t.Errorf("peer uid = %d, want %d", uid, os.Getuid())
	}
}

// TestAuthorizePeer_FailClosedOnNonUnix verifies that a non-unix connection (whose
// peer credential cannot be read) is rejected rather than allowed.
func TestAuthorizePeer_FailClosedOnNonUnix(t *testing.T) {
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close() }()
	defer func() { _ = c2.Close() }()
	if err := authorizePeer(c1); err == nil {
		t.Error("authorizePeer must reject a connection whose peer credential cannot be read")
	}
}
