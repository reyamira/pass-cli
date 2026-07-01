package sync

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const syncStateFile = ".sync-state"

// SyncState tracks sync metadata to avoid unnecessary rclone operations.
type SyncState struct {
	LastPushHash  string    `json:"last_push_hash"`
	LastPushTime  time.Time `json:"last_push_time"`
	RemoteModTime time.Time `json:"remote_mod_time"`
	RemoteSize    int64     `json:"remote_size"`
	// LastPullCheck is when we last successfully contacted the remote (a
	// metadata probe or a post-push refresh). The TTL-gate skips the probe while
	// this is within the configured window, so a burst of commands makes one
	// remote round-trip instead of one per command.
	LastPullCheck time.Time `json:"last_pull_check"`
}

// LoadState reads the sync state from the vault directory.
// Returns a zero-value SyncState if the file doesn't exist.
func LoadState(vaultDir string) (*SyncState, error) {
	path := filepath.Join(vaultDir, syncStateFile)
	data, err := os.ReadFile(path) // #nosec G304 -- path is constructed from user-configured vault dir
	if err != nil {
		if os.IsNotExist(err) {
			return &SyncState{}, nil
		}
		return nil, fmt.Errorf("failed to read sync state: %w", err)
	}

	var state SyncState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse sync state: %w", err)
	}
	return &state, nil
}

// SaveState writes the sync state to the vault directory.
func SaveState(vaultDir string, state *SyncState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sync state: %w", err)
	}

	path := filepath.Join(vaultDir, syncStateFile)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write sync state: %w", err)
	}
	return nil
}

// HashFile computes the SHA-256 hex digest of the file at the given path.
func HashFile(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- path is user-configured vault file
	if err != nil {
		return "", fmt.Errorf("failed to open file for hashing: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// StatePath returns the full path to the sync state file in a vault directory.
func StatePath(vaultDir string) string {
	return filepath.Join(vaultDir, syncStateFile)
}
