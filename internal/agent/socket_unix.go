//go:build !windows

package agent

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// SocketPath returns the agent socket path: $PASS_CLI_AGENT_SOCK if set, else
// $XDG_RUNTIME_DIR/pass-cli/agent.sock (tmpfs, cleared on logout), else
// ~/.pass-cli/agent.sock.
func SocketPath() string {
	if p := os.Getenv("PASS_CLI_AGENT_SOCK"); p != "" {
		return p
	}
	if rt := os.Getenv("XDG_RUNTIME_DIR"); rt != "" {
		return filepath.Join(rt, "pass-cli", "agent.sock")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pass-cli", "agent.sock")
}

// Listen creates the socket directory (0700) and binds a unix listener with the
// socket file at 0600 — the filesystem permissions are the PRIMARY access control
// (peer-cred in 2c is defense-in-depth). A stale socket left by a crashed agent is
// reclaimed; a socket answered by a live agent is an "already running" error.
func Listen(path string) (net.Listener, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create socket directory %q: %w", dir, err)
	}

	if _, err := os.Stat(path); err == nil {
		// Something is at the path. If it answers, an agent is already running.
		if c, derr := net.DialTimeout("unix", path, 200*time.Millisecond); derr == nil {
			_ = c.Close()
			return nil, fmt.Errorf("an agent is already running at %s", path)
		}
		// Not answering → stale socket from a crashed agent; reclaim it.
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("failed to remove stale socket %q: %w", path, err)
		}
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %q: %w", path, err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("failed to set socket permissions: %w", err)
	}
	return ln, nil
}

func dialSocket(path string) (net.Conn, error) {
	return net.DialTimeout("unix", path, 2*time.Second)
}
