// Package sync provides rclone-based vault synchronization for cross-device access.
package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	// 3. Check if remote is unchanged
	if remoteVault.ModTime.Equal(state.RemoteModTime) && remoteVault.Size == state.RemoteSize {
		return nil // Remote unchanged, skip pull
	}

	// 4. Check for local changes (conflict detection)
	if _, statErr := os.Stat(vaultPath); statErr == nil {
		localHash, hashErr := HashFile(vaultPath)
		if hashErr == nil && state.LastPushHash != "" && localHash != state.LastPushHash {
			// Local has unpushed changes AND remote has changed = conflict
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
