// Package sync provides rclone-based vault synchronization for cross-device access.
package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/arimxyer/pass-cli/internal/config"
)

// ErrSyncConflict indicates both local and remote have changed since last sync.
var ErrSyncConflict = errors.New("sync conflict: both local and remote vault have changed")

// CommandExecutor abstracts command execution for testing.
type CommandExecutor interface {
	Run(name string, args ...string) ([]byte, error)
	RunNoOutput(name string, args ...string) error
}

// execExecutor is the real implementation using os/exec.
type execExecutor struct{}

func (e *execExecutor) Run(name string, args ...string) ([]byte, error) {
	// #nosec G204 -- command args are constructed from user-configured values
	cmd := exec.Command(name, args...)
	return cmd.Output()
}

func (e *execExecutor) RunNoOutput(name string, args ...string) error {
	// #nosec G204 -- command args are constructed from user-configured values
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RemoteFileInfo represents metadata from rclone lsjson.
type RemoteFileInfo struct {
	Path    string    `json:"Path"`
	Name    string    `json:"Name"`
	Size    int64     `json:"Size"`
	ModTime time.Time `json:"ModTime"`
	IsDir   bool      `json:"IsDir"`
}

// markerSuffix is appended to a content-hash marker object. The marker is a
// zero-byte file whose NAME encodes pass-cli's own sha256 of the vault:
//
//	<vaultFileName>.<64-hex-sha256><markerSuffix>   e.g. vault.enc.9f3a…e21.synchash
//
// It is synced alongside vault.enc (it lives in the vault dir, only .sync-state
// is excluded), so the single `rclone lsjson` call SmartPull already makes also
// lists the marker — letting us read the remote's content identity for ZERO
// extra round-trips. This fixes the (ModTime, Size) false-negative in #102:
// a same-length, same-modtime remote edit changes the marker name, so it can no
// longer read as "unchanged".
const markerSuffix = ".synchash"

// markerFileName builds the marker object name for a vault file + content hash.
func markerFileName(vaultFileName, hash string) string {
	return vaultFileName + "." + hash + markerSuffix
}

// isHex64 reports whether s is exactly 64 lowercase-or-uppercase hex chars
// (a sha256 hex digest), guarding the marker parse against unrelated files.
func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// parseRemoteMarkerHash scans an rclone lsjson listing for the vault's content
// marker and returns the embedded sha256. ok is false when no marker is present
// (legacy remotes, or a device that pushed before markers existed) or when the
// listing is ambiguous (more than one distinct marker hash — an abnormal,
// interrupted state) — callers then fall back to the (ModTime, Size) heuristic.
func parseRemoteMarkerHash(files []RemoteFileInfo, vaultFileName string) (string, bool) {
	prefix := vaultFileName + "."
	found := ""
	for i := range files {
		name := files[i].Name
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, markerSuffix) {
			continue
		}
		hash := name[len(prefix) : len(name)-len(markerSuffix)]
		if !isHex64(hash) {
			continue
		}
		if found != "" && found != hash {
			return "", false // ambiguous — fall back to the heuristic
		}
		found = hash
	}
	if found == "" {
		return "", false
	}
	return found, true
}

// writeLocalMarker drops a zero-byte marker named for hash into vaultDir and
// removes any stale markers for the same vault, so the directory holds exactly
// one marker. The subsequent `rclone sync` of the dir mirrors that single marker
// to the remote (deleting the old one there too).
func writeLocalMarker(vaultDir, vaultFileName, hash string) error {
	prefix := vaultFileName + "."
	entries, err := os.ReadDir(vaultDir)
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, markerSuffix) {
				_ = os.Remove(filepath.Join(vaultDir, name))
			}
		}
	}
	markerPath := filepath.Join(vaultDir, markerFileName(vaultFileName, hash))
	return os.WriteFile(markerPath, nil, 0600)
}

// Service provides vault synchronization using rclone.
type Service struct {
	config          config.SyncConfig
	executor        CommandExecutor
	skipBinaryCheck bool // bypasses rclone PATH check when using mock executor in tests
}

// NewService creates a new sync service with the given configuration.
func NewService(cfg config.SyncConfig) *Service {
	return &Service{
		config:   cfg,
		executor: &execExecutor{},
	}
}

// NewServiceWithExecutor creates a new sync service with a custom command executor (for testing).
func NewServiceWithExecutor(cfg config.SyncConfig, executor CommandExecutor) *Service {
	return &Service{
		config:          cfg,
		executor:        executor,
		skipBinaryCheck: true,
	}
}

// IsEnabled returns true if sync is enabled in the configuration.
func (s *Service) IsEnabled() bool {
	return s.config.Enabled && s.config.Remote != ""
}

// IsRcloneInstalled checks if rclone is available in PATH.
func (s *Service) IsRcloneInstalled() bool {
	if s.skipBinaryCheck {
		return true
	}
	_, err := exec.LookPath("rclone")
	return err == nil
}

// Pull syncs the vault from the remote to the local directory.
// Deprecated: Use SmartPull instead for change-detection-based sync.
func (s *Service) Pull(vaultDir string) error {
	if !s.IsEnabled() {
		return nil
	}

	if !s.IsRcloneInstalled() {
		fmt.Fprintf(os.Stderr, "Warning: sync enabled but rclone not found in PATH\n")
		return nil
	}

	if err := os.MkdirAll(vaultDir, 0700); err != nil {
		return fmt.Errorf("failed to create vault directory: %w", err)
	}

	if err := s.executor.RunNoOutput("rclone", "sync", s.config.Remote, vaultDir, "--exclude", syncStateFile); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: sync pull failed: %v\n", err)
		return nil
	}

	return nil
}

// Push syncs the vault from the local directory to the remote.
// Deprecated: Use SmartPush instead for change-detection-based sync.
func (s *Service) Push(vaultDir string) error {
	if !s.IsEnabled() {
		return nil
	}

	if !s.IsRcloneInstalled() {
		fmt.Fprintf(os.Stderr, "Warning: sync enabled but rclone not found in PATH\n")
		return nil
	}

	if _, err := os.Stat(vaultDir); os.IsNotExist(err) {
		return fmt.Errorf("vault directory does not exist: %s", vaultDir)
	}

	if err := s.executor.RunNoOutput("rclone", "sync", vaultDir, s.config.Remote, "--exclude", syncStateFile); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: sync push failed: %v\n", err)
		return nil
	}

	return nil
}

// CheckRemoteMetadata fetches remote file metadata using rclone lsjson.
func (s *Service) CheckRemoteMetadata() ([]RemoteFileInfo, error) {
	// Note: no "--hash" flag — RemoteFileInfo carries no hash field and no
	// decision uses a remote hash (skip compares ModTime+Size; conflict detection
	// uses a local sha256 via HashFile). Requesting it only adds backend cost.
	output, err := s.executor.Run("rclone", "lsjson", s.config.Remote)
	if err != nil {
		return nil, fmt.Errorf("rclone lsjson failed: %w", err)
	}

	var files []RemoteFileInfo
	if err := json.Unmarshal(output, &files); err != nil {
		return nil, fmt.Errorf("failed to parse rclone lsjson output: %w", err)
	}
	return files, nil
}

// SmartPull checks remote metadata and only pulls if remote has changed.
// Returns ErrSyncConflict if both local and remote have changed.
func (s *Service) SmartPull(vaultPath string) error {
	if !s.IsEnabled() {
		return nil
	}

	if !s.IsRcloneInstalled() {
		fmt.Fprintf(os.Stderr, "Warning: sync enabled but rclone not found in PATH\n")
		return nil
	}

	vaultDir := filepath.Dir(vaultPath)

	// 1. Check remote metadata
	remoteFiles, err := s.CheckRemoteMetadata()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to check remote state: %v\n", err)
		return nil // Allow offline operation
	}

	// Find vault.enc in remote files
	var remoteVault *RemoteFileInfo
	vaultFileName := filepath.Base(vaultPath)
	for i := range remoteFiles {
		if remoteFiles[i].Name == vaultFileName {
			remoteVault = &remoteFiles[i]
			break
		}
	}

	// No remote vault file = nothing to pull
	if remoteVault == nil {
		return nil
	}

	// 2. Load sync state
	state, err := LoadState(vaultDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load sync state: %v\n", err)
		state = &SyncState{}
	}

	// 3. Decide whether the remote changed since our last push.
	//
	// Prefer the content marker (#102): its name encodes the remote's own
	// sha256, so a same-size + same-modtime remote edit (which the legacy
	// heuristic would miss) still reads as changed. Fall back to the
	// (ModTime, Size) heuristic only when no marker is present (legacy remotes
	// or a device that pushed before markers existed).
	//
	// Limitation: an interrupted remote push that uploaded vault.enc but not the
	// marker can leave a stale marker == LastPushHash, read here as "unchanged."
	// The next successful push self-heals the marker; we accept this rare window
	// in exchange for content-authoritative detection with no extra round-trip
	// and no false conflicts from modtime noise.
	remoteHash, hasMarker := parseRemoteMarkerHash(remoteFiles, vaultFileName)
	var remoteChanged bool
	if hasMarker {
		remoteChanged = remoteHash != state.LastPushHash
	} else {
		remoteChanged = !remoteVault.ModTime.Equal(state.RemoteModTime) || remoteVault.Size != state.RemoteSize
	}
	if !remoteChanged {
		return nil // Remote unchanged, skip pull
	}

	// 4. Remote changed — if local also diverged from our last push, it's a
	// conflict (both sides changed). This is content-based on both sides.
	if _, statErr := os.Stat(vaultPath); statErr == nil {
		localHash, hashErr := HashFile(vaultPath)
		if hashErr == nil && state.LastPushHash != "" && localHash != state.LastPushHash {
			return ErrSyncConflict
		}
	}

	// 5. Remote changed, local unchanged — pull
	if err := os.MkdirAll(vaultDir, 0700); err != nil {
		return fmt.Errorf("failed to create vault directory: %w", err)
	}

	if err := s.executor.RunNoOutput("rclone", "sync", s.config.Remote, vaultDir, "--exclude", syncStateFile); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: sync pull failed: %v\n", err)
		return nil
	}

	// 6. Update state with new remote metadata
	state.RemoteModTime = remoteVault.ModTime
	state.RemoteSize = remoteVault.Size

	// Update last push hash to match what we just pulled
	if newHash, err := HashFile(vaultPath); err == nil {
		state.LastPushHash = newHash
	}

	if err := SaveState(vaultDir, state); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save sync state: %v\n", err)
	}

	return nil
}

// SmartPush checks if local vault has changed and only pushes if needed.
// Returns true if a push was actually performed.
func (s *Service) SmartPush(vaultPath string) (bool, error) {
	if !s.IsEnabled() {
		return false, nil
	}

	if !s.IsRcloneInstalled() {
		fmt.Fprintf(os.Stderr, "Warning: sync enabled but rclone not found in PATH\n")
		return false, nil
	}

	vaultDir := filepath.Dir(vaultPath)

	// 1. Compute local vault hash
	localHash, err := HashFile(vaultPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to hash vault for sync: %v\n", err)
		return false, nil
	}

	// 2. Load state
	state, err := LoadState(vaultDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load sync state: %v\n", err)
		state = &SyncState{}
	}

	// 3. Skip if unchanged
	if localHash == state.LastPushHash {
		return false, nil
	}

	// 3b. Write the content marker (#102) into the vault dir so the rclone sync
	// below carries it to the remote, where the next device's SmartPull reads our
	// content hash straight from its name. Best-effort: a marker write failure
	// degrades change-detection to the legacy heuristic but must not block a push.
	if err := writeLocalMarker(vaultDir, filepath.Base(vaultPath), localHash); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write sync content marker: %v\n", err)
	}

	// 4. Push
	if err := s.executor.RunNoOutput("rclone", "sync", vaultDir, s.config.Remote, "--exclude", syncStateFile); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: sync push failed: %v\n", err)
		return false, nil
	}

	// 5. Update state
	state.LastPushHash = localHash
	state.LastPushTime = time.Now()

	// Query actual remote metadata after push so next SmartPull sees current state.
	// Using time.Now() would mismatch the provider's recorded ModTime.
	remoteFiles, err := s.CheckRemoteMetadata()
	if err == nil {
		vaultFileName := filepath.Base(vaultPath)
		for _, f := range remoteFiles {
			if f.Name == vaultFileName {
				state.RemoteModTime = f.ModTime
				state.RemoteSize = f.Size
				break
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "Warning: failed to refresh remote metadata after push: %v\n", err)
	}

	if err := SaveState(vaultDir, state); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save sync state: %v\n", err)
	}

	return true, nil
}

// GetVaultDir returns the directory containing the vault file.
func GetVaultDir(vaultPath string) string {
	return filepath.Dir(vaultPath)
}
