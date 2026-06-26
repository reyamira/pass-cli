package sync

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arimxyer/pass-cli/internal/config"
)

// mockExecutor records calls and returns configured responses.
type mockExecutor struct {
	runCalls      [][]string
	runNoOutCalls [][]string
	runOutput     []byte
	runErr        error
	runNoOutErr   error
}

func (m *mockExecutor) Run(name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	m.runCalls = append(m.runCalls, call)
	return m.runOutput, m.runErr
}

func (m *mockExecutor) RunNoOutput(name string, args ...string) error {
	call := append([]string{name}, args...)
	m.runNoOutCalls = append(m.runNoOutCalls, call)
	return m.runNoOutErr
}

func enabledConfig() config.SyncConfig {
	return config.SyncConfig{Enabled: true, Remote: "gdrive:.pass-cli"}
}

// --- Existing tests ---

func TestNewService(t *testing.T) {
	service := NewService(enabledConfig())
	if service == nil {
		t.Fatal("NewService returned nil")
	}
	if !service.IsEnabled() {
		t.Error("Expected service to be enabled")
	}
}

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   config.SyncConfig
		expected bool
	}{
		{"enabled with remote", config.SyncConfig{Enabled: true, Remote: "gdrive:.pass-cli"}, true},
		{"disabled", config.SyncConfig{Enabled: false, Remote: "gdrive:.pass-cli"}, false},
		{"enabled but empty remote", config.SyncConfig{Enabled: true, Remote: ""}, false},
		{"disabled and empty remote", config.SyncConfig{Enabled: false, Remote: ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(tt.config)
			if got := service.IsEnabled(); got != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsRcloneInstalled(t *testing.T) {
	service := NewService(config.SyncConfig{})
	_, err := exec.LookPath("rclone")
	expected := err == nil
	if got := service.IsRcloneInstalled(); got != expected {
		t.Errorf("IsRcloneInstalled() = %v, want %v", got, expected)
	}
}

func TestGetVaultDir(t *testing.T) {
	tests := []struct {
		name, vaultPath, expected string
	}{
		{"standard path", filepath.Join("home", "user", ".pass-cli", "vault.enc"), filepath.Join("home", "user", ".pass-cli")},
		{"relative path", filepath.Join(".", ".pass-cli", "vault.enc"), filepath.Join(".", ".pass-cli")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetVaultDir(tt.vaultPath); got != tt.expected {
				t.Errorf("GetVaultDir(%q) = %q, want %q", tt.vaultPath, got, tt.expected)
			}
		})
	}
}

func TestPull_Disabled(t *testing.T) {
	service := NewService(config.SyncConfig{Enabled: false})
	if err := service.Pull("/tmp/test-vault"); err != nil {
		t.Errorf("Pull() with disabled sync returned error: %v", err)
	}
}

func TestPush_Disabled(t *testing.T) {
	service := NewService(config.SyncConfig{Enabled: false})
	if err := service.Push("/tmp/test-vault"); err != nil {
		t.Errorf("Push() with disabled sync returned error: %v", err)
	}
}

// --- SmartPush tests ---

func TestSmartPush_Disabled(t *testing.T) {
	service := NewService(config.SyncConfig{Enabled: false})
	if _, err := service.SmartPush("/tmp/vault.enc"); err != nil {
		t.Errorf("SmartPush disabled returned error: %v", err)
	}
}

func TestSmartPush_SkipsWhenUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")
	_ = os.WriteFile(vaultPath, []byte("vault-data"), 0600)

	// Compute hash and save state with matching hash
	hash, _ := HashFile(vaultPath)
	_ = SaveState(tmpDir, &SyncState{LastPushHash: hash})

	mock := &mockExecutor{}
	service := NewServiceWithExecutor(enabledConfig(), mock)

	if _, err := service.SmartPush(vaultPath); err != nil {
		t.Fatalf("SmartPush returned error: %v", err)
	}

	// Should NOT have called rclone sync
	if len(mock.runNoOutCalls) > 0 {
		t.Errorf("expected no rclone calls when unchanged, got %d", len(mock.runNoOutCalls))
	}
}

func TestSmartPush_PushesWhenChanged(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")
	_ = os.WriteFile(vaultPath, []byte("vault-data"), 0600)

	// Save state with different hash
	_ = SaveState(tmpDir, &SyncState{LastPushHash: "old-hash"})

	// Mock lsjson response for post-push remote metadata query
	remoteTime := time.Date(2026, 1, 29, 20, 0, 0, 0, time.UTC)
	lsjsonOutput, _ := json.Marshal([]RemoteFileInfo{
		{Name: "vault.enc", Size: 10, ModTime: remoteTime},
	})
	mock := &mockExecutor{runOutput: lsjsonOutput}
	service := NewServiceWithExecutor(enabledConfig(), mock)

	if _, err := service.SmartPush(vaultPath); err != nil {
		t.Fatalf("SmartPush returned error: %v", err)
	}

	// Should have called rclone sync (RunNoOutput) and lsjson (Run)
	if len(mock.runNoOutCalls) != 1 {
		t.Fatalf("expected 1 RunNoOutput call (sync), got %d", len(mock.runNoOutCalls))
	}
	if len(mock.runCalls) != 1 {
		t.Fatalf("expected 1 Run call (lsjson), got %d", len(mock.runCalls))
	}

	// Verify --exclude .sync-state in sync args
	syncArgs := mock.runNoOutCalls[0]
	foundExclude := false
	for i, arg := range syncArgs {
		if arg == "--exclude" && i+1 < len(syncArgs) && syncArgs[i+1] == ".sync-state" {
			foundExclude = true
			break
		}
	}
	if !foundExclude {
		t.Errorf("expected --exclude .sync-state in sync args, got %v", syncArgs)
	}

	// Verify state was updated with hash and remote metadata
	state, _ := LoadState(tmpDir)
	expectedHash, _ := HashFile(vaultPath)
	if state.LastPushHash != expectedHash {
		t.Errorf("state hash = %q, want %q", state.LastPushHash, expectedHash)
	}
	if !state.RemoteModTime.Equal(remoteTime) {
		t.Errorf("state RemoteModTime = %v, want %v", state.RemoteModTime, remoteTime)
	}
	if state.RemoteSize != 10 {
		t.Errorf("state RemoteSize = %d, want 10", state.RemoteSize)
	}
}

// --- CheckRemoteMetadata tests ---

func TestCheckRemoteMetadata_DoesNotRequestHash(t *testing.T) {
	// lsjson must NOT pass "--hash": RemoteFileInfo has no hash field and no
	// decision uses a remote hash, so requesting it only adds backend cost.
	mock := &mockExecutor{runOutput: []byte("[]")}
	service := NewServiceWithExecutor(enabledConfig(), mock)

	if _, err := service.CheckRemoteMetadata(); err != nil {
		t.Fatalf("CheckRemoteMetadata returned error: %v", err)
	}

	if len(mock.runCalls) != 1 {
		t.Fatalf("expected 1 Run call (lsjson), got %d", len(mock.runCalls))
	}

	args := mock.runCalls[0]
	foundLsjson := false
	for _, arg := range args {
		if arg == "--hash" {
			t.Errorf("lsjson must not pass --hash, got args %v", args)
		}
		if arg == "lsjson" {
			foundLsjson = true
		}
	}
	if !foundLsjson {
		t.Errorf("expected lsjson in args, got %v", args)
	}
}

// --- SmartPull tests ---

func TestSmartPull_Disabled(t *testing.T) {
	service := NewService(config.SyncConfig{Enabled: false})
	if err := service.SmartPull("/tmp/vault.enc"); err != nil {
		t.Errorf("SmartPull disabled returned error: %v", err)
	}
}

func TestSmartPull_SkipsWhenRemoteUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")
	_ = os.WriteFile(vaultPath, []byte("vault-data"), 0600)

	remoteTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	_ = SaveState(tmpDir, &SyncState{
		RemoteModTime: remoteTime,
		RemoteSize:    100,
	})

	// Mock lsjson returns same metadata
	lsjsonOutput, _ := json.Marshal([]RemoteFileInfo{
		{Name: "vault.enc", Size: 100, ModTime: remoteTime},
	})

	mock := &mockExecutor{runOutput: lsjsonOutput}
	service := NewServiceWithExecutor(enabledConfig(), mock)

	if err := service.SmartPull(vaultPath); err != nil {
		t.Fatalf("SmartPull returned error: %v", err)
	}

	// Should have called lsjson but NOT sync
	if len(mock.runCalls) != 1 {
		t.Errorf("expected 1 Run call (lsjson), got %d", len(mock.runCalls))
	}
	if len(mock.runNoOutCalls) != 0 {
		t.Errorf("expected 0 RunNoOutput calls (no sync), got %d", len(mock.runNoOutCalls))
	}
}

func TestSmartPull_PullsWhenRemoteChanged(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")
	_ = os.WriteFile(vaultPath, []byte("vault-data"), 0600)

	oldTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)

	hash, _ := HashFile(vaultPath)
	_ = SaveState(tmpDir, &SyncState{
		LastPushHash:  hash, // Local matches last push = no local changes
		RemoteModTime: oldTime,
		RemoteSize:    100,
	})

	// Mock lsjson returns different metadata
	lsjsonOutput, _ := json.Marshal([]RemoteFileInfo{
		{Name: "vault.enc", Size: 200, ModTime: newTime},
	})

	mock := &mockExecutor{runOutput: lsjsonOutput}
	service := NewServiceWithExecutor(enabledConfig(), mock)

	if err := service.SmartPull(vaultPath); err != nil {
		t.Fatalf("SmartPull returned error: %v", err)
	}

	// Should have called lsjson AND sync
	if len(mock.runCalls) != 1 {
		t.Errorf("expected 1 Run call (lsjson), got %d", len(mock.runCalls))
	}
	if len(mock.runNoOutCalls) != 1 {
		t.Errorf("expected 1 RunNoOutput call (sync), got %d", len(mock.runNoOutCalls))
	}
}

func TestSmartPull_DetectsConflict(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")
	_ = os.WriteFile(vaultPath, []byte("local-modified"), 0600)

	oldTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)

	// State has different hash than current local file = local changes
	_ = SaveState(tmpDir, &SyncState{
		LastPushHash:  "old-hash-from-before-local-edit",
		RemoteModTime: oldTime,
		RemoteSize:    100,
	})

	// Remote also changed
	lsjsonOutput, _ := json.Marshal([]RemoteFileInfo{
		{Name: "vault.enc", Size: 200, ModTime: newTime},
	})

	mock := &mockExecutor{runOutput: lsjsonOutput}
	service := NewServiceWithExecutor(enabledConfig(), mock)

	err := service.SmartPull(vaultPath)
	if !errors.Is(err, ErrSyncConflict) {
		t.Errorf("expected ErrSyncConflict, got: %v", err)
	}

	// Should NOT have called sync (no overwrite on conflict)
	if len(mock.runNoOutCalls) != 0 {
		t.Errorf("expected 0 RunNoOutput calls on conflict, got %d", len(mock.runNoOutCalls))
	}
}

func TestSmartPull_NoRemoteVaultFile(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")

	// Remote has files but not vault.enc
	lsjsonOutput, _ := json.Marshal([]RemoteFileInfo{
		{Name: "other-file.txt", Size: 50},
	})

	mock := &mockExecutor{runOutput: lsjsonOutput}
	service := NewServiceWithExecutor(enabledConfig(), mock)

	if err := service.SmartPull(vaultPath); err != nil {
		t.Fatalf("SmartPull returned error: %v", err)
	}

	// No sync should happen
	if len(mock.runNoOutCalls) != 0 {
		t.Errorf("expected no sync calls, got %d", len(mock.runNoOutCalls))
	}
}

func TestSmartPull_LsjsonFailure(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")

	mock := &mockExecutor{runErr: errors.New("network error")}
	service := NewServiceWithExecutor(enabledConfig(), mock)

	// Should not fail - allows offline operation
	if err := service.SmartPull(vaultPath); err != nil {
		t.Errorf("SmartPull should allow offline operation, got error: %v", err)
	}
}

// --- #102: content-marker tests ---

func hashA() string { return strings.Repeat("a", 64) }
func hashB() string { return strings.Repeat("b", 64) }

func TestParseRemoteMarkerHash(t *testing.T) {
	tests := []struct {
		name     string
		files    []RemoteFileInfo
		wantHash string
		wantOK   bool
	}{
		{
			name: "marker present",
			files: []RemoteFileInfo{
				{Name: "vault.enc"},
				{Name: markerFileName("vault.enc", hashA())},
			},
			wantHash: hashA(), wantOK: true,
		},
		{
			name:   "no marker (legacy remote)",
			files:  []RemoteFileInfo{{Name: "vault.enc"}},
			wantOK: false,
		},
		{
			name: "ignores backup and non-hex lookalikes",
			files: []RemoteFileInfo{
				{Name: "vault.enc"},
				{Name: "vault.enc.backup"},
				{Name: "vault.enc.not-a-hash.synchash"}, // wrong length / non-hex
			},
			wantOK: false,
		},
		{
			name: "ambiguous (two distinct markers) falls back",
			files: []RemoteFileInfo{
				{Name: markerFileName("vault.enc", hashA())},
				{Name: markerFileName("vault.enc", hashB())},
			},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHash, gotOK := parseRemoteMarkerHash(tt.files, "vault.enc")
			if gotOK != tt.wantOK || (tt.wantOK && gotHash != tt.wantHash) {
				t.Errorf("parseRemoteMarkerHash() = (%q, %v), want (%q, %v)", gotHash, gotOK, tt.wantHash, tt.wantOK)
			}
		})
	}
}

// THE #102 regression: a remote edit that keeps vault.enc's Size AND ModTime
// identical (the legacy heuristic's blind spot) must still be detected as
// changed, because the marker name carries a different content hash. Without the
// marker this skips the pull (silent stale read; on a write path, silent clobber).
func TestSmartPull_MarkerCatchesSameSizeSameModtimeEdit(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")
	_ = os.WriteFile(vaultPath, []byte("vault-data"), 0600)

	localHash, _ := HashFile(vaultPath)
	sameTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Local matches last push (no local changes). Recorded remote modtime/size
	// match what lsjson will report — so the (ModTime, Size) heuristic says
	// "unchanged" and would skip.
	_ = SaveState(tmpDir, &SyncState{
		LastPushHash:  localHash,
		RemoteModTime: sameTime,
		RemoteSize:    100,
	})

	// Remote vault.enc is byte-identical in Size+ModTime, but the marker encodes
	// a DIFFERENT content hash → remote really changed.
	lsjsonOutput, _ := json.Marshal([]RemoteFileInfo{
		{Name: "vault.enc", Size: 100, ModTime: sameTime},
		{Name: markerFileName("vault.enc", hashB())},
	})
	mock := &mockExecutor{runOutput: lsjsonOutput}
	service := NewServiceWithExecutor(enabledConfig(), mock)

	if err := service.SmartPull(vaultPath); err != nil {
		t.Fatalf("SmartPull returned error: %v", err)
	}
	// Must have pulled (sync) despite identical modtime+size.
	if len(mock.runNoOutCalls) != 1 {
		t.Errorf("expected 1 sync call (pull) — marker should force detection, got %d", len(mock.runNoOutCalls))
	}
}

// The marker is authoritative: when it matches LastPushHash the content is
// unchanged, so SmartPull must skip even if ModTime/Size differ (modtime noise
// must not trigger a needless pull or a false conflict).
func TestSmartPull_MarkerSkipsOnModtimeNoise(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")
	_ = os.WriteFile(vaultPath, []byte("vault-data"), 0600)

	localHash, _ := HashFile(vaultPath)
	_ = SaveState(tmpDir, &SyncState{
		LastPushHash:  localHash,
		RemoteModTime: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
		RemoteSize:    100,
	})

	// vault.enc reports a different modtime+size, but the marker == LastPushHash.
	lsjsonOutput, _ := json.Marshal([]RemoteFileInfo{
		{Name: "vault.enc", Size: 999, ModTime: time.Date(2026, 2, 2, 2, 0, 0, 0, time.UTC)},
		{Name: markerFileName("vault.enc", localHash)},
	})
	mock := &mockExecutor{runOutput: lsjsonOutput}
	service := NewServiceWithExecutor(enabledConfig(), mock)

	if err := service.SmartPull(vaultPath); err != nil {
		t.Fatalf("SmartPull returned error: %v", err)
	}
	if len(mock.runNoOutCalls) != 0 {
		t.Errorf("expected no sync (marker says unchanged), got %d", len(mock.runNoOutCalls))
	}
}

// Marker says remote changed AND local diverged from last push → conflict.
func TestSmartPull_MarkerConflict(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")
	_ = os.WriteFile(vaultPath, []byte("local-modified"), 0600)

	// Local differs from last push (local changed).
	_ = SaveState(tmpDir, &SyncState{LastPushHash: hashA()})

	// Remote marker differs from last push (remote changed).
	lsjsonOutput, _ := json.Marshal([]RemoteFileInfo{
		{Name: "vault.enc", Size: 200},
		{Name: markerFileName("vault.enc", hashB())},
	})
	mock := &mockExecutor{runOutput: lsjsonOutput}
	service := NewServiceWithExecutor(enabledConfig(), mock)

	if err := service.SmartPull(vaultPath); !errors.Is(err, ErrSyncConflict) {
		t.Errorf("expected ErrSyncConflict, got: %v", err)
	}
	if len(mock.runNoOutCalls) != 0 {
		t.Errorf("expected no sync on conflict, got %d", len(mock.runNoOutCalls))
	}
}

// SmartPush writes exactly one content marker named for the new hash and removes
// any stale marker from a previous push.
func TestSmartPush_WritesContentMarker(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.enc")
	_ = os.WriteFile(vaultPath, []byte("vault-data"), 0600)

	// A stale marker from a previous push that must be cleaned up.
	staleMarker := filepath.Join(tmpDir, markerFileName("vault.enc", hashA()))
	_ = os.WriteFile(staleMarker, nil, 0600)

	_ = SaveState(tmpDir, &SyncState{LastPushHash: "old-hash"})
	mock := &mockExecutor{runOutput: []byte(`[{"Name":"vault.enc","Size":10}]`)}
	service := NewServiceWithExecutor(enabledConfig(), mock)

	if _, err := service.SmartPush(vaultPath); err != nil {
		t.Fatalf("SmartPush returned error: %v", err)
	}

	expectedHash, _ := HashFile(vaultPath)
	wantMarker := markerFileName("vault.enc", expectedHash)

	entries, _ := os.ReadDir(tmpDir)
	var markers []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), markerSuffix) {
			markers = append(markers, e.Name())
		}
	}
	if len(markers) != 1 || markers[0] != wantMarker {
		t.Errorf("expected exactly one marker %q, got %v", wantMarker, markers)
	}
	if _, err := os.Stat(staleMarker); !os.IsNotExist(err) {
		t.Errorf("stale marker should have been removed")
	}
}
