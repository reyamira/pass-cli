package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/arimxyer/pass-cli/internal/config"
	"github.com/arimxyer/pass-cli/internal/crypto"
	"github.com/arimxyer/pass-cli/internal/keychain"
	"github.com/arimxyer/pass-cli/internal/recovery"
	"github.com/arimxyer/pass-cli/internal/security"
	"github.com/arimxyer/pass-cli/internal/storage"
	intsync "github.com/arimxyer/pass-cli/internal/sync"

	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/argon2"
)

// KeychainStatus represents the state of keychain integration.
type KeychainStatus struct {
	Available      bool
	PasswordStored bool
	BackendName    string
}

// RemoveVaultResult holds the results of a vault removal operation.
type RemoveVaultResult struct {
	FileDeleted      bool
	KeychainDeleted  bool
	FileNotFound     bool
	KeychainNotFound bool
	AuditLogDeleted  bool
	AuditLogNotFound bool
	DirectoryDeleted bool
}

var (
	// ErrVaultLocked indicates the vault is not unlocked
	ErrVaultLocked = errors.New("vault is locked")
	// ErrCredentialNotFound indicates the credential doesn't exist
	ErrCredentialNotFound = errors.New("credential not found")
	// ErrCredentialExists indicates a credential with that name already exists
	ErrCredentialExists = errors.New("credential already exists")
	// ErrInvalidCredential indicates the credential data is invalid
	ErrInvalidCredential = errors.New("invalid credential")
	// ErrKeychainAlreadyEnabled indicates that the keychain is already enabled for the vault.
	ErrKeychainAlreadyEnabled = errors.New("keychain is already enabled")
	// ErrKeychainNotEnabled indicates that keychain integration is not enabled for the vault.
	ErrKeychainNotEnabled = errors.New("keychain integration is not enabled for this vault")
)

// UsageRecord tracks where and when a credential was accessed
type UsageRecord struct {
	Location    string         `json:"location"`              // Working directory where accessed
	Timestamp   time.Time      `json:"timestamp"`             // When it was last accessed
	GitRepo     string         `json:"git_repo"`              // Git repository if available
	Count       int            `json:"count"`                 // Total number of accesses from this location (sum of all field accesses)
	LineNumber  int            `json:"line_number,omitempty"` // Line number in file where accessed (optional)
	FieldAccess map[string]int `json:"field_access"`          // Per-field access counts: "password": 5, "username": 2, etc.
}

// Credential represents a stored credential with usage tracking
// T020c: Password field changed from string to []byte for secure memory handling
type Credential struct {
	Service       string                 `json:"service"`
	Username      string                 `json:"username"`
	Password      []byte                 `json:"password"` // T020c: Changed to []byte for memory security
	Category      string                 `json:"category,omitempty"`
	URL           string                 `json:"url,omitempty"`
	Notes         string                 `json:"notes"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	ModifiedCount int                    `json:"modified_count"` // Number of times credential has been modified
	UsageRecord   map[string]UsageRecord `json:"usage_records"`  // Map of location -> UsageRecord

	// TOTP fields for 2FA support (all optional)
	TOTPSecret    string `json:"totp_secret,omitempty"`    // Base32 encoded TOTP secret
	TOTPAlgorithm string `json:"totp_algorithm,omitempty"` // SHA1, SHA256, SHA512 (default: SHA1)
	TOTPDigits    int    `json:"totp_digits,omitempty"`    // 6 or 8 (default: 6)
	TOTPPeriod    int    `json:"totp_period,omitempty"`    // Period in seconds (default: 30)
	TOTPIssuer    string `json:"totp_issuer,omitempty"`    // Issuer name for display
}

// VaultData is the decrypted vault structure
type VaultData struct {
	Credentials map[string]Credential `json:"credentials"` // Map of service name -> Credential
	Version     int                   `json:"version"`
	// Audit configuration persistence (fix for DISC-013)
	AuditEnabled bool   `json:"audit_enabled,omitempty"`  // Whether audit logging is enabled
	AuditLogPath string `json:"audit_log_path,omitempty"` // Path to audit log file
	VaultID      string `json:"vault_id,omitempty"`       // Vault identifier for audit key
}

// VaultService manages credentials with encryption and keychain integration
type VaultService struct {
	vaultPath       string
	cryptoService   *crypto.CryptoService
	storageService  *storage.StorageService
	keychainService *keychain.KeychainService

	// In-memory state
	unlocked       bool
	masterPassword []byte // Byte array for secure memory clearing (T009)
	vaultData      *VaultData
	recoveryDEK    []byte // DEK from recovery unlock (for SetPasswordAfterRecovery)

	// T066: Audit logging configuration (FR-025: default disabled)
	auditEnabled bool
	auditLogger  *security.AuditLogger

	// T051a: Rate limiting for password validation (FR-024)
	rateLimiter *security.ValidationRateLimiter

	// Smart sync service (nil if sync disabled)
	syncService          *intsync.Service
	syncConflictDetected bool // prevents auto-push after conflict
}

// New creates a new VaultService
func New(vaultPath string) (*VaultService, error) {
	// Expand home directory if needed
	if strings.HasPrefix(vaultPath, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		vaultPath = filepath.Join(home, vaultPath[1:])
	}

	cryptoService := crypto.NewCryptoService()
	storageService, err := storage.NewStorageService(cryptoService, vaultPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage service: %w", err)
	}

	// Extract vault ID from path for vault-specific keychain entries
	vaultDir := filepath.Dir(vaultPath)
	vaultID := filepath.Base(vaultDir)

	v := &VaultService{
		vaultPath:       vaultPath,
		cryptoService:   cryptoService,
		storageService:  storageService,
		keychainService: keychain.New(vaultID),
		unlocked:        false,
		auditEnabled:    false,                               // T066: Default disabled per FR-025
		rateLimiter:     security.NewValidationRateLimiter(), // T051a: Initialize rate limiter
	}

	// Initialize sync service from config (if sync enabled)
	cfg, _ := config.Load()
	if cfg != nil && cfg.Sync.Enabled {
		v.syncService = intsync.NewService(cfg.Sync)
	}

	// T010: Load metadata file (if exists) to enable audit logging before vault unlock
	meta, err := LoadMetadata(vaultPath)
	metadataFileExists := true
	if err != nil {
		// Metadata exists but corrupted - log warning and try fallback (T011)
		fmt.Fprintf(os.Stderr, "Warning: Failed to load metadata: %v\n", err)
		meta = nil
		metadataFileExists = false
	}

	// Check if metadata file actually exists (LoadMetadata returns default if missing)
	if meta != nil && !meta.AuditEnabled {
		// Metadata may be default (file missing) - check if file exists
		if _, statErr := os.Stat(MetadataPath(vaultPath)); os.IsNotExist(statErr) {
			metadataFileExists = false
		}
	}

	// Initialize audit from metadata
	if meta != nil && meta.AuditEnabled {
		// Audit is enabled in metadata - initialize it now
		vaultDir := filepath.Dir(vaultPath)
		auditLogPath := filepath.Join(vaultDir, "audit.log")
		// Use directory name as VaultID for consistency with init command (getVaultID)
		vaultID := filepath.Base(vaultDir)
		if err := v.EnableAudit(auditLogPath, vaultID); err != nil {
			// Non-fatal - continue without audit (graceful degradation)
			fmt.Fprintf(os.Stderr, "Warning: Failed to enable audit from metadata: %v\n", err)
		}
	}

	// T011: Fallback self-discovery if metadata missing/failed OR audit not enabled but log exists
	if !metadataFileExists || (meta != nil && !meta.AuditEnabled) {
		vaultDir := filepath.Dir(vaultPath)
		auditLogPath := filepath.Join(vaultDir, "audit.log")
		if _, err := os.Stat(auditLogPath); err == nil {
			// audit.log exists, enable best-effort audit
			// Use directory name as VaultID for consistency with init command (getVaultID)
			vaultID := filepath.Base(vaultDir)
			if err := v.EnableAudit(auditLogPath, vaultID); err != nil {
				// Best-effort failed, continue without audit (non-fatal)
				fmt.Fprintf(os.Stderr, "Warning: Self-discovery audit init failed: %v\n", err)
			}
		}
	}

	return v, nil
}

// SyncPull performs a smart sync pull if sync is enabled.
// Should be called before unlocking the vault.
func (v *VaultService) SyncPull() error {
	if v.syncService == nil || !v.syncService.IsEnabled() {
		return nil
	}
	err := v.syncService.SmartPull(v.vaultPath)
	if errors.Is(err, intsync.ErrSyncConflict) {
		v.syncConflictDetected = true
		fmt.Fprintf(os.Stderr, "Warning: %v\nUse `pass-cli sync resolve` to choose which version to keep.\n", err)
		return nil // Don't block operation on conflict
	}
	return err
}

// IsSyncEnabled returns true if sync is configured and enabled.
func (v *VaultService) IsSyncEnabled() bool {
	return v.syncService != nil && v.syncService.IsEnabled()
}

// SyncConflictDetected reports whether the most recent SyncPull detected a
// conflict (both local and remote changed). Used by the concurrent-unlock path
// to re-surface the conflict cleanly after the password prompt, since SyncPull's
// own warning may print mid-prompt and read commands never re-echo it (#103).
func (v *VaultService) SyncConflictDetected() bool {
	return v.syncConflictDetected
}

// SyncPush performs a smart sync push if sync is enabled.
// Should be called once at the end of a command, not per-save.
// Returns true if a push was actually performed.
func (v *VaultService) SyncPush() bool {
	if v.syncService == nil || !v.syncService.IsEnabled() {
		return false
	}
	if v.syncConflictDetected {
		fmt.Fprintf(os.Stderr, "Warning: skipping push due to unresolved sync conflict. Use `pass-cli sync resolve` to resolve.\n")
		return false
	}
	pushed, err := v.syncService.SmartPush(v.vaultPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: sync push failed: %v\n", err)
		return false
	}
	return pushed
}

// GetStorageService returns the underlying storage service.
// Used by CLI commands that need direct access to storage operations.
func (v *VaultService) GetStorageService() *storage.StorageService {
	return v.storageService
}

// T066: EnableAudit enables audit logging for this vault
// vaultID should be a unique identifier for the vault (e.g., filepath or UUID)
// DISC-013 fix: Now persists audit config to vault data
func (v *VaultService) EnableAudit(auditLogPath, vaultID string) error {
	if v.auditEnabled {
		return nil // Already enabled
	}

	logger, err := security.NewAuditLogger(auditLogPath, vaultID)
	if err != nil {
		return fmt.Errorf("failed to create audit logger: %w", err)
	}

	v.auditLogger = logger
	v.auditEnabled = true

	// DISC-013 fix: Persist audit configuration to vault data
	if v.vaultData != nil {
		v.vaultData.AuditEnabled = true
		v.vaultData.AuditLogPath = auditLogPath
		v.vaultData.VaultID = vaultID
		// Save vault data to persist audit configuration
		if err := v.save(); err != nil {
			return fmt.Errorf("failed to persist audit configuration: %w", err)
		}
	}

	// T026 (US2): Save metadata file for pre-unlock audit logging
	// Only save metadata if it already exists (explicit enable) or if vault explicitly requested it
	// Don't create metadata during autodiscovery (best-effort logging)
	existingMeta, err := LoadMetadata(v.vaultPath)
	if err == nil && existingMeta != nil {
		// Check if metadata file actually exists (LoadMetadata returns default if missing)
		if _, statErr := os.Stat(MetadataPath(v.vaultPath)); statErr == nil {
			// Metadata exists - update it
			existingMeta.AuditEnabled = true
			if err := SaveMetadata(v.vaultPath, existingMeta); err != nil {
				// Non-fatal: audit logger is enabled, metadata save failed
				fmt.Fprintf(os.Stderr, "Warning: Failed to save metadata: %v\n", err)
			}
		}
		// else: metadata file doesn't exist, this is autodiscovery, don't create metadata
	}

	return nil
}

// T066: DisableAudit disables audit logging
func (v *VaultService) DisableAudit() {
	v.auditEnabled = false
	v.auditLogger = nil
}

// EnableAuditPortable enables audit logging with portable password-based key derivation
// This enables audit verification across different OSes when syncing vaults
// The salt is stored in vault metadata for retrieval on other systems
func (v *VaultService) EnableAuditPortable(auditLogPath, vaultID string, password, existingSalt []byte) error {
	if v.auditEnabled {
		return nil // Already enabled
	}

	logger, newSalt, err := security.NewAuditLoggerPortable(auditLogPath, vaultID, password, existingSalt)
	if err != nil {
		return fmt.Errorf("failed to create portable audit logger: %w", err)
	}

	v.auditLogger = logger
	v.auditEnabled = true

	// DISC-013 fix: Persist audit configuration to vault data
	if v.vaultData != nil {
		v.vaultData.AuditEnabled = true
		v.vaultData.AuditLogPath = auditLogPath
		v.vaultData.VaultID = vaultID
		// Save vault data to persist audit configuration
		if err := v.save(); err != nil {
			return fmt.Errorf("failed to persist audit configuration: %w", err)
		}
	}

	// Save salt to metadata for cross-OS retrieval
	existingMeta, err := LoadMetadata(v.vaultPath)
	if err == nil && existingMeta != nil {
		// Update metadata with salt if we generated a new one
		if len(newSalt) > 0 || len(existingSalt) > 0 {
			if len(newSalt) > 0 {
				existingMeta.AuditSalt = newSalt
			} else {
				existingMeta.AuditSalt = existingSalt
			}
		}
		existingMeta.AuditEnabled = true
		if err := SaveMetadata(v.vaultPath, existingMeta); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to save audit salt to metadata: %v\n", err)
		}
	}

	return nil
}

// T074: LogAudit logs an audit event with graceful degradation (FR-026)
// Per FR-026: System MUST continue operation even if audit logging fails
// Exported for use by keychain lifecycle commands (FR-015)
func (v *VaultService) LogAudit(eventType, outcome, credentialName string) {
	if !v.auditEnabled || v.auditLogger == nil {
		return // Audit not enabled
	}

	entry := &security.AuditLogEntry{
		Timestamp:      time.Now(),
		EventType:      eventType,
		Outcome:        outcome,
		CredentialName: credentialName,
		MachineID:      security.GetMachineID(), // ARI-50: Track source machine
	}

	// FR-026: Log errors to stderr but continue operation
	if err := v.auditLogger.Log(entry); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: audit logging failed (operation continues): %v\n", err)
	}
}

// createAuditCallback returns a storage.ProgressCallback that logs atomic save events
// to the audit log. Returns nil if audit logging is disabled.
// T015/T022/T034: Integrate audit logging into atomic save operations
// FR-015: Log ALL atomic save state transitions
func (v *VaultService) createAuditCallback() storage.ProgressCallback {
	if !v.auditEnabled || v.auditLogger == nil {
		return nil // No callback if audit disabled
	}

	// Return closure that maps storage events to audit entries
	return func(event string, metadata ...string) {
		// FR-015: Log ALL atomic save state transitions
		switch event {
		case "atomic_save_started":
			v.LogAudit("vault_save", security.OutcomeInProgress, "vault save operation initiated")

		case "temp_file_created":
			tempPath := ""
			if len(metadata) > 0 {
				tempPath = filepath.Base(metadata[0]) // Log filename only, not full path
			}
			v.LogAudit("vault_save", security.OutcomeInProgress, fmt.Sprintf("temporary file created: %s", tempPath))

		case "verification_started":
			v.LogAudit("vault_save", security.OutcomeInProgress, "vault verification started")

		case "verification_passed":
			v.LogAudit("vault_save", security.OutcomeInProgress, "vault verification passed")

		case "verification_failed":
			reason := "unknown"
			if len(metadata) > 1 {
				reason = metadata[1]
			}
			v.LogAudit("vault_save", security.OutcomeFailure, fmt.Sprintf("vault verification failed: %s", reason))

		case "atomic_rename_started":
			// Log rename operations (called twice during save)
			oldFile := ""
			newFile := ""
			if len(metadata) >= 2 {
				oldFile = filepath.Base(metadata[0])
				newFile = filepath.Base(metadata[1])
			}
			v.LogAudit("vault_save", security.OutcomeInProgress, fmt.Sprintf("atomic rename: %s → %s", oldFile, newFile))

		case "rollback_started":
			v.LogAudit("vault_save", security.OutcomeFailure, "atomic save rollback initiated")

		case "rollback_completed":
			v.LogAudit("vault_save", security.OutcomeFailure, "atomic save rollback completed")

		case "atomic_save_completed":
			v.LogAudit("vault_save", security.OutcomeSuccess, "vault save completed successfully")
		}
	}
}

// Initialize creates a new vault with a master password
// T010: Updated signature to accept []byte, T014: Added deferred cleanup
// T045: Added password policy validation (FR-016)
// DISC-013 fix: Added audit parameters to set config during initialization
func (v *VaultService) Initialize(masterPassword []byte, useKeychain bool, auditLogPath, vaultID string) error {
	defer crypto.ClearBytes(masterPassword) // T014: Ensure cleanup even on error

	// T045 [US3]: Validate master password against policy (FR-016)
	// Import security package required at top of file
	passwordPolicy := &security.PasswordPolicy{
		MinLength:        12,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireDigit:     true,
		RequireSymbol:    true,
	}
	if err := passwordPolicy.Validate(masterPassword); err != nil {
		// T051a: Record failure and check rate limit
		if rateLimitErr := v.rateLimiter.CheckAndRecordFailure(); rateLimitErr != nil {
			return rateLimitErr // Rate limit triggered
		}
		return fmt.Errorf("password does not meet requirements: %w", err)
	}

	// T051a: Reset rate limiter on successful validation
	v.rateLimiter.Reset()

	// Check if vault already exists
	if _, err := os.Stat(v.vaultPath); err == nil {
		return errors.New("vault already exists")
	}

	// DISC-013 fix: Create vault data with audit config if provided
	vaultData := &VaultData{
		Credentials: make(map[string]Credential),
		Version:     1,
	}

	// Set audit configuration if provided (non-empty path means enabled)
	if auditLogPath != "" && vaultID != "" {
		vaultData.AuditEnabled = true
		vaultData.AuditLogPath = auditLogPath
		vaultData.VaultID = vaultID

		// DISC-013 fix: Create audit logger for immediate use
		logger, err := security.NewAuditLogger(auditLogPath, vaultID)
		if err != nil {
			// Don't fail init if audit logger creation fails (graceful degradation)
			fmt.Fprintf(os.Stderr, "Warning: failed to create audit logger: %v\n", err)
		} else {
			v.auditLogger = logger
			v.auditEnabled = true
		}
	}

	// Marshal to JSON
	data, err := json.Marshal(vaultData)
	if err != nil {
		return fmt.Errorf("failed to marshal vault data: %w", err)
	}

	// Convert to string for storage service (TODO: Phase 4 will update storage.go to accept []byte)
	masterPasswordStr := string(masterPassword)

	// Initialize storage (creates directory and vault file)
	if err := v.storageService.InitializeVault(masterPasswordStr); err != nil {
		return fmt.Errorf("failed to initialize vault: %w", err)
	}

	// Save initial empty vault
	// T015: Pass audit callback for atomic save logging
	if err := v.storageService.SaveVault(data, masterPasswordStr, v.createAuditCallback()); err != nil {
		return fmt.Errorf("failed to save initial vault: %w", err)
	}

	// Store master password in keychain if requested
	if useKeychain && v.keychainService.IsAvailable() {
		if err := v.keychainService.Store(masterPasswordStr); err != nil {
			// Log warning but don't fail initialization
			fmt.Fprintf(os.Stderr, "Warning: failed to store password in keychain: %v\n", err)
		}
	}

	// T067: Log vault creation event (FR-019)
	v.LogAudit(security.EventVaultUnlock, security.OutcomeSuccess, "")

	// Create metadata file to track vault configuration
	metadata := &Metadata{
		Version:         "1.0",
		AuditEnabled:    vaultData.AuditEnabled,
		KeychainEnabled: useKeychain && v.keychainService.IsAvailable(),
		CreatedAt:       time.Now(),
		LastModified:    time.Now(),
	}
	if err := SaveMetadata(v.vaultPath, metadata); err != nil {
		// Log warning but don't fail initialization (graceful degradation)
		fmt.Fprintf(os.Stderr, "Warning: failed to create metadata file: %v\n", err)
	}

	return nil
}

// T022: InitializeWithRecovery creates a new v2 vault with recovery phrase support
// This method generates a DEK, wraps it with both password and recovery KEKs,
// and stores the wrapped versions in the vault metadata.
// Parameters:
//   - masterPassword: master password for the vault
//   - useKeychain: whether to store password in OS keychain
//   - auditLogPath: path to audit log (empty to disable)
//   - vaultID: unique vault identifier for audit
//   - passphrase: optional recovery passphrase (25th word)
//
// Returns: mnemonic string (24 words) for user backup, error
func (v *VaultService) InitializeWithRecovery(masterPassword []byte, useKeychain bool, auditLogPath, vaultID string, passphrase []byte) (string, error) {
	defer crypto.ClearBytes(masterPassword) // Ensure cleanup even on error
	if passphrase != nil {
		defer crypto.ClearBytes(passphrase)
	}

	// Validate master password against policy
	passwordPolicy := &security.PasswordPolicy{
		MinLength:        12,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireDigit:     true,
		RequireSymbol:    true,
	}
	if err := passwordPolicy.Validate(masterPassword); err != nil {
		if rateLimitErr := v.rateLimiter.CheckAndRecordFailure(); rateLimitErr != nil {
			return "", rateLimitErr
		}
		return "", fmt.Errorf("password does not meet requirements: %w", err)
	}
	v.rateLimiter.Reset()

	// Check if vault already exists
	if _, err := os.Stat(v.vaultPath); err == nil {
		return "", errors.New("vault already exists")
	}

	// 1. Generate salt for password KDF
	salt, err := v.cryptoService.GenerateSalt()
	if err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// 2. Derive password KEK
	iterations := crypto.GetIterations()
	passwordKEK, err := v.cryptoService.DeriveKey(masterPassword, salt, iterations)
	if err != nil {
		return "", fmt.Errorf("failed to derive password KEK: %w", err)
	}
	defer crypto.ClearBytes(passwordKEK)

	// 3. Setup challenge-based recovery (generates mnemonic and challenge data)
	challengeSetup, err := recovery.SetupChallengeRecovery(&recovery.ChallengeSetupConfig{
		Passphrase: passphrase,
	})
	if err != nil {
		return "", fmt.Errorf("failed to setup recovery: %w", err)
	}
	defer crypto.ClearBytes(challengeSetup.RecoveryKEK)

	// 4. Generate and wrap DEK with both password KEK and recovery KEK
	keyWrapResult, err := crypto.GenerateAndWrapDEK(passwordKEK, challengeSetup.RecoveryKEK)
	if err != nil {
		return "", fmt.Errorf("failed to generate and wrap DEK: %w", err)
	}
	defer crypto.ClearBytes(keyWrapResult.DEK)

	// 5. Initialize v2 vault with DEK
	if err := v.storageService.InitializeVaultV2(
		keyWrapResult.DEK,
		keyWrapResult.PasswordWrapped.Ciphertext,
		keyWrapResult.PasswordWrapped.Nonce,
		salt,
		iterations,
	); err != nil {
		return "", fmt.Errorf("failed to initialize v2 vault: %w", err)
	}

	// 6. Create vault data structure
	vaultData := &VaultData{
		Credentials: make(map[string]Credential),
		Version:     1, // Vault data version (not vault format version)
	}

	// Set audit configuration if provided
	if auditLogPath != "" && vaultID != "" {
		vaultData.AuditEnabled = true
		vaultData.AuditLogPath = auditLogPath
		vaultData.VaultID = vaultID

		logger, err := security.NewAuditLogger(auditLogPath, vaultID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create audit logger: %v\n", err)
		} else {
			v.auditLogger = logger
			v.auditEnabled = true
		}
	}

	// 7. Marshal and save vault data
	data, err := json.Marshal(vaultData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal vault data: %w", err)
	}

	// Save vault data encrypted with DEK
	if err := v.storageService.SaveVaultWithDEK(data, keyWrapResult.DEK, v.createAuditCallback()); err != nil {
		return "", fmt.Errorf("failed to save initial vault: %w", err)
	}

	// 8. Store password in keychain if requested
	if useKeychain && v.keychainService.IsAvailable() {
		if err := v.keychainService.Store(string(masterPassword)); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to store password in keychain: %v\n", err)
		}
	}

	// 9. Log vault creation
	v.LogAudit(security.EventVaultUnlock, security.OutcomeSuccess, "")

	// 10. Complete recovery metadata with wrapped DEK
	recoveryMetadata := challengeSetup.Metadata
	recoveryMetadata.EncryptedRecoveryKey = keyWrapResult.RecoveryWrapped.Ciphertext
	recoveryMetadata.NonceRecovery = keyWrapResult.RecoveryWrapped.Nonce

	// 11. Create metadata file
	metadata := &Metadata{
		Version:         "1.0",
		AuditEnabled:    vaultData.AuditEnabled,
		KeychainEnabled: useKeychain && v.keychainService.IsAvailable(),
		CreatedAt:       time.Now(),
		LastModified:    time.Now(),
		Recovery:        recoveryMetadata,
	}
	if err := SaveMetadata(v.vaultPath, metadata); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create metadata file: %v\n", err)
	}

	// Return mnemonic for CLI to display and verify
	return challengeSetup.Mnemonic, nil
}

// Unlock opens the vault and loads credentials into memory
// T011: Updated signature to accept []byte, T015: Added deferred cleanup
// T036e: Auto-rollback on incomplete migration detection
func (v *VaultService) Unlock(masterPassword []byte) error {
	defer crypto.ClearBytes(masterPassword) // T015: Ensure cleanup even on error

	if v.unlocked {
		return nil // Already unlocked
	}

	if err := v.handleIncompleteMigration(); err != nil {
		return err
	}

	// Load + decrypt the vault (derives the key from the password — the
	// expensive PBKDF2 step happens inside LoadVault).
	data, err := v.storageService.LoadVault(string(masterPassword))
	if err != nil {
		// T068: Log unlock failure (FR-019)
		v.LogAudit(security.EventVaultUnlock, security.OutcomeFailure, "")
		return fmt.Errorf("failed to unlock vault: %w", err)
	}

	return v.finishUnlock(data, masterPassword)
}

// handleIncompleteMigration auto-recovers from an interrupted vault migration
// (a leftover vault.tmp): restores from backup if present, otherwise cleans up
// the temp file with a warning. No-op when no incomplete migration is detected.
// T036e/T036g.
func (v *VaultService) handleIncompleteMigration() error {
	vaultTmpPath := v.vaultPath + storage.TempSuffix
	vaultBackupPath := v.vaultPath + storage.BackupSuffix

	if _, err := os.Stat(vaultTmpPath); err != nil {
		return nil // no incomplete migration
	}

	// T036g: Incomplete migration detected - inform user with actionable message
	fmt.Fprintf(os.Stderr, "\n*** MIGRATION FAILURE DETECTED ***\n")
	fmt.Fprintf(os.Stderr, "An incomplete vault migration was found (power loss or system crash).\n")

	if _, err := os.Stat(vaultBackupPath); err == nil {
		// Backup exists - restore it
		fmt.Fprintf(os.Stderr, "Attempting automatic recovery from backup...\n")

		// Read backup
		backupData, err := os.ReadFile(vaultBackupPath) // #nosec G304 -- Vault backup path validated by storage layer
		if err != nil {
			return fmt.Errorf("failed to read backup for rollback: %w", err)
		}

		// Restore to main vault path
		if err := os.WriteFile(v.vaultPath, backupData, storage.VaultPermissions); err != nil {
			return fmt.Errorf("failed to restore backup: %w", err)
		}

		// Remove incomplete temp file
		_ = os.Remove(vaultTmpPath)

		fmt.Fprintf(os.Stderr, "SUCCESS: Vault restored from backup. Your data is safe.\n")
		fmt.Fprintf(os.Stderr, "You may continue using the vault normally.\n\n")
	} else {
		// No backup available - just remove temp file and warn
		fmt.Fprintf(os.Stderr, "WARNING: No backup file found. Cleaning up temporary files.\n")
		_ = os.Remove(vaultTmpPath)
		fmt.Fprintf(os.Stderr, "If you experience issues, please report this immediately.\n\n")
	}
	return nil
}

// finishUnlock completes an unlock once the plaintext vault data is in hand,
// independent of HOW it was decrypted (password path via Unlock, or prepared-key
// path via UnlockWithPreparedKey). It sets in-memory state — including a copy of
// masterPassword for later saves (NOT recoveryDEK; that distinction is why the
// recovery-oriented UnlockWithKey can't be reused for keychain unlock) — restores
// audit logging, synchronizes metadata, removes the post-migration backup, and
// logs success.
func (v *VaultService) finishUnlock(data []byte, masterPassword []byte) error {
	// Unmarshal vault data
	var vaultData VaultData
	if err := json.Unmarshal(data, &vaultData); err != nil {
		return fmt.Errorf("failed to parse vault data: %w", err)
	}

	// Store in memory (make a copy since we're clearing the parameter)
	v.unlocked = true
	v.masterPassword = make([]byte, len(masterPassword))
	copy(v.masterPassword, masterPassword)
	v.vaultData = &vaultData

	// DISC-013 fix: Restore audit logging if it was enabled
	if vaultData.AuditEnabled && vaultData.AuditLogPath != "" && vaultData.VaultID != "" {
		// Check if sync is enabled - if so, use portable audit mode for cross-OS verification
		cfg, _ := config.Load()
		if cfg != nil && cfg.Sync.Enabled {
			// Load audit salt from metadata for portable mode
			meta, metaErr := LoadMetadata(v.vaultPath)
			var auditSalt []byte
			if metaErr == nil && meta != nil && len(meta.AuditSalt) > 0 {
				auditSalt = meta.AuditSalt
			}
			// Use portable audit mode with password-derived key
			if err := v.EnableAuditPortable(vaultData.AuditLogPath, vaultData.VaultID, masterPassword, auditSalt); err != nil {
				// Log warning but don't fail unlock - audit logging is optional
				fmt.Fprintf(os.Stderr, "Warning: failed to restore portable audit logging: %v\n", err)
			}
		} else {
			// Use legacy keychain-based audit mode
			if err := v.EnableAudit(vaultData.AuditLogPath, vaultData.VaultID); err != nil {
				// Log warning but don't fail unlock - audit logging is optional
				fmt.Fprintf(os.Stderr, "Warning: failed to restore audit logging: %v\n", err)
			}
		}
	}

	// T027-T029: Metadata synchronization (User Story 2)
	// Load existing metadata (if any)
	meta, err := LoadMetadata(v.vaultPath)
	if err != nil {
		// Metadata corrupted - will be recreated if audit enabled
		fmt.Fprintf(os.Stderr, "Warning: Corrupted metadata, will recreate: %v\n", err)
		meta = nil
	}

	// T028: Check for metadata/vault config mismatch
	if meta != nil {
		mismatch := meta.AuditEnabled != vaultData.AuditEnabled

		// T029: Synchronize metadata when mismatch detected (vault settings take precedence per FR-012)
		if mismatch {
			updatedMeta := &Metadata{
				Version:         meta.Version,
				AuditEnabled:    vaultData.AuditEnabled,
				KeychainEnabled: meta.KeychainEnabled, // Preserve keychain setting
				CreatedAt:       meta.CreatedAt,       // Preserve original timestamp
			}

			if err := SaveMetadata(v.vaultPath, updatedMeta); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to sync metadata: %v\n", err)
			}
		}
	} else if vaultData.AuditEnabled {
		// T027: Create metadata if missing and audit enabled in vault
		newMeta := &Metadata{
			Version:         "1.0",
			AuditEnabled:    true,
			KeychainEnabled: false,
		}

		if err := SaveMetadata(v.vaultPath, newMeta); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to create metadata: %v\n", err)
		}
	}

	// T036f: Remove backup file after successful unlock
	// This confirms the vault is readable and migration (if any) was successful
	backupPath := v.vaultPath + storage.BackupSuffix
	if _, err := os.Stat(backupPath); err == nil {
		if err := os.Remove(backupPath); err != nil {
			// Log warning but don't fail unlock - backup cleanup is not critical
			fmt.Fprintf(os.Stderr, "Warning: failed to remove backup file: %v\n", err)
		}
	}

	// T068: Log unlock success (FR-019)
	v.LogAudit(security.EventVaultUnlock, security.OutcomeSuccess, "")

	return nil
}

// UnlockWithKey unlocks the vault using a provided encryption key (for recovery)
// Parameters: vaultKey (32-byte AES-256 encryption key from recovery)
// Returns: error
func (v *VaultService) UnlockWithKey(vaultKey []byte) error {
	// Note: We store the DEK for SetPasswordAfterRecovery, so don't clear it here
	// It will be cleared when Lock() is called or when SetPasswordAfterRecovery completes

	if v.unlocked {
		return nil // Already unlocked
	}

	// Load vault data using the recovery key
	data, err := v.storageService.LoadVaultWithKey(vaultKey)
	if err != nil {
		// Log unlock failure
		v.LogAudit(security.EventVaultUnlock, security.OutcomeFailure, "recovery")
		return fmt.Errorf("failed to unlock vault with recovery key: %w", err)
	}

	// Unmarshal vault data
	var vaultData VaultData
	if err := json.Unmarshal(data, &vaultData); err != nil {
		return fmt.Errorf("failed to parse vault data: %w", err)
	}

	// Store in memory (no master password for recovery unlock)
	v.unlocked = true
	v.masterPassword = nil // Recovery unlock doesn't have a password
	v.vaultData = &vaultData

	// Store the DEK for SetPasswordAfterRecovery
	v.recoveryDEK = make([]byte, len(vaultKey))
	copy(v.recoveryDEK, vaultKey)

	// Restore audit logging if enabled
	if vaultData.AuditEnabled && vaultData.AuditLogPath != "" && vaultData.VaultID != "" {
		if err := v.EnableAudit(vaultData.AuditLogPath, vaultData.VaultID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to restore audit logging: %v\n", err)
		}
	}

	// Load metadata (same as regular Unlock)
	meta, err := LoadMetadata(v.vaultPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Corrupted metadata, will recreate: %v\n", err)
		meta = nil
	}

	// Synchronize metadata if needed
	if meta != nil {
		mismatch := meta.AuditEnabled != vaultData.AuditEnabled

		if mismatch {
			updatedMeta := &Metadata{
				Version:         meta.Version,
				AuditEnabled:    vaultData.AuditEnabled,
				KeychainEnabled: meta.KeychainEnabled,
				Recovery:        meta.Recovery, // Preserve recovery metadata
				CreatedAt:       meta.CreatedAt,
			}

			if err := SaveMetadata(v.vaultPath, updatedMeta); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to sync metadata: %v\n", err)
			}
		}
	} else if vaultData.AuditEnabled {
		newMeta := &Metadata{
			Version:         "1.0",
			AuditEnabled:    true,
			KeychainEnabled: false,
		}

		if err := SaveMetadata(v.vaultPath, newMeta); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to create metadata: %v\n", err)
		}
	}

	// Remove backup if exists (successful unlock = migration succeeded)
	backupPath := v.vaultPath + storage.BackupSuffix
	if _, err := os.Stat(backupPath); err == nil {
		if err := os.Remove(backupPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove backup file: %v\n", err)
		}
	}

	// Log unlock success
	v.LogAudit(security.EventVaultUnlock, security.OutcomeSuccess, "recovery")

	return nil
}

// PreparedUnlock holds key-derivation parameters captured before a sync pull so
// the expensive PBKDF2 derivation can overlap the pull (#103 Tier 2). It is
// produced by PrepareUnlock, derives the data key via DeriveDataKey (safe to run
// while a pull is in flight), and is consumed by UnlockWithPreparedKey.
type PreparedUnlock struct {
	storageService *storage.StorageService
	params         storage.PreparedKeyParams
}

// PrepareUnlock reads the current key-derivation parameters (cheap, pre-pull).
// Call this before kicking off SyncPull, then run DeriveDataKey concurrently with
// the pull, then UnlockWithPreparedKey after the join.
func (v *VaultService) PrepareUnlock() (*PreparedUnlock, error) {
	params, err := v.storageService.ReadKeyParams()
	if err != nil {
		return nil, err
	}
	return &PreparedUnlock{storageService: v.storageService, params: params}, nil
}

// DeriveDataKey runs the expensive PBKDF2 derivation against the captured
// parameters (no file access) — safe to call while a sync pull is in flight.
// The returned key is handed to UnlockWithPreparedKey, which clears it.
func (p *PreparedUnlock) DeriveDataKey(password []byte) ([]byte, error) {
	return p.storageService.DeriveDataKey(string(password), p.params)
}

// UnlockWithPreparedKey completes a keychain unlock using dataKey derived ahead
// of time (concurrently with a sync pull). If the vault was re-keyed since the
// parameters were captured — a password change or re-key on another device that a
// just-completed pull brought in — the prepared key is stale. That is detected by
// comparing the current on-disk key parameters against the captured ones; on a
// mismatch it falls back to a full password unlock, making this path behave
// identically to a sequential keychain unlock in that edge (e.g. a stale-keychain
// password fails cleanly at unlock rather than unlocking then breaking saves).
// A decrypt failure is a belt-and-suspenders second fallback. masterPassword and
// dataKey are cleared before returning.
func (v *VaultService) UnlockWithPreparedKey(prep *PreparedUnlock, dataKey, masterPassword []byte) error {
	defer crypto.ClearBytes(masterPassword)
	defer crypto.ClearBytes(dataKey)

	if v.unlocked {
		return nil // Already unlocked
	}

	if err := v.handleIncompleteMigration(); err != nil {
		return err
	}

	// Re-read params from the (possibly pulled) file. If they differ from what we
	// derived against, the prepared key is stale → full re-derive against current
	// params, keeping behavior identical to a sequential unlock on any re-key.
	current, err := v.storageService.ReadKeyParams()
	preparedKeyUsable := err == nil && prep != nil && prep.params.Equal(current)

	if preparedKeyUsable {
		if data, decErr := v.storageService.LoadVaultWithKey(dataKey); decErr == nil {
			return v.finishUnlock(data, masterPassword)
		}
		// Belt-and-suspenders: params matched but decrypt unexpectedly failed —
		// fall through to the password path rather than failing the unlock.
	}

	// Fallback: full password unlock (re-derives against current params).
	data, err := v.storageService.LoadVault(string(masterPassword))
	if err != nil {
		v.LogAudit(security.EventVaultUnlock, security.OutcomeFailure, "")
		return fmt.Errorf("failed to unlock vault: %w", err)
	}
	return v.finishUnlock(data, masterPassword)
}

// UnlockWithKeychain attempts to unlock using keychain-stored password
func (v *VaultService) UnlockWithKeychain() error {
	password, err := v.RetrieveKeychainPassword()
	if err != nil {
		return err
	}
	return v.Unlock(password)
}

// RetrieveKeychainPassword returns the master password from the OS keychain
// without decrypting the vault. It reads metadata and the keyring (and performs
// best-effort migration from a legacy global entry) but never touches the
// vault.enc ciphertext — so callers may run it concurrently with a sync pull
// that replaces vault.enc, then decrypt with the returned password afterwards
// (see the concurrent-unlock path in cmd, #103). Returns ErrKeychainNotEnabled
// when keychain unlock is not configured. The caller owns the returned bytes and
// must crypto.ClearBytes them.
func (v *VaultService) RetrieveKeychainPassword() ([]byte, error) {
	// T018: Check metadata to see if keychain is enabled (FR-007)
	metadata, err := v.LoadMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	if !metadata.KeychainEnabled {
		return nil, ErrKeychainNotEnabled
	}

	// Attempt to retrieve password from vault-specific keychain entry
	// This uses keyring.Get() which doesn't require GUI authorization on macOS
	password, err := v.keychainService.Retrieve()

	// If vault-specific entry not found, try auto-migration from global entry
	if err == keychain.ErrPasswordNotFound {
		migrated, migrateErr := v.keychainService.MigrateFromGlobal()
		if migrateErr != nil {
			// Log warning but continue - migration is best-effort
			fmt.Fprintf(os.Stderr, "Warning: keychain migration check failed: %v\n", migrateErr)
		} else if migrated {
			// Migration succeeded, try retrieve again
			password, err = v.keychainService.Retrieve()
			if err == nil {
				fmt.Fprintf(os.Stderr, "Migrated keychain entry to vault-specific storage\n")
				// Clean up global entry after successful migration and unlock
				// This is safe because we have the password and can re-store if needed
				_ = v.keychainService.DeleteGlobal()
			}
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve password from keychain: %w", err)
	}

	return []byte(password), nil
}

// Lock clears in-memory credentials and password
// T013: Fixed to properly clear []byte password using crypto.ClearBytes
// T069: Added audit logging (FR-019)
func (v *VaultService) Lock() {
	// T069: Log lock event before clearing state (FR-019)
	v.LogAudit(security.EventVaultLock, security.OutcomeSuccess, "")

	v.unlocked = false

	// Clear sensitive data from memory
	if v.masterPassword != nil {
		crypto.ClearBytes(v.masterPassword)
		v.masterPassword = nil
	}

	// Clear recovery DEK if present
	if v.recoveryDEK != nil {
		crypto.ClearBytes(v.recoveryDEK)
		v.recoveryDEK = nil
	}

	v.vaultData = nil
}

// IsUnlocked returns whether the vault is currently unlocked
func (v *VaultService) IsUnlocked() bool {
	return v.unlocked
}

// save persists the current vault data to disk
func (v *VaultService) save() error {
	if !v.unlocked {
		return ErrVaultLocked
	}

	data, err := json.Marshal(v.vaultData)
	if err != nil {
		return fmt.Errorf("failed to marshal vault data: %w", err)
	}

	// Convert to string for storage service (TODO: Phase 4 will update storage.go to accept []byte)
	masterPasswordStr := string(v.masterPassword)

	// T022: Pass audit callback for atomic save logging
	if err := v.storageService.SaveVault(data, masterPasswordStr, v.createAuditCallback()); err != nil {
		return fmt.Errorf("failed to save vault: %w", err)
	}

	return nil
}

// AddCredential adds a new credential to the vault
// T020d: Password parameter changed to []byte for memory security
// T020e: Added deferred cleanup for password parameter
func (v *VaultService) AddCredential(service, username string, password []byte, category, url, notes string) error {
	defer crypto.ClearBytes(password) // T020e: Ensure cleanup even on error

	if !v.unlocked {
		return ErrVaultLocked
	}

	// Validate inputs
	if service == "" {
		return fmt.Errorf("%w: service name cannot be empty", ErrInvalidCredential)
	}
	if len(password) == 0 {
		return fmt.Errorf("%w: password cannot be empty", ErrInvalidCredential)
	}

	// Check for duplicates
	if _, exists := v.vaultData.Credentials[service]; exists {
		return fmt.Errorf("%w: %s", ErrCredentialExists, service)
	}

	// Create credential (make a copy of password to store)
	now := time.Now()
	passwordCopy := make([]byte, len(password))
	copy(passwordCopy, password)

	credential := Credential{
		Service:       service,
		Username:      username,
		Password:      passwordCopy, // T020d: Store []byte password
		Category:      category,
		URL:           url,
		Notes:         notes,
		CreatedAt:     now,
		UpdatedAt:     now,
		ModifiedCount: 0, // Initialize modification counter
		UsageRecord:   make(map[string]UsageRecord),
	}

	// Add to vault
	v.vaultData.Credentials[service] = credential

	// Save to disk
	if err := v.save(); err != nil {
		return err
	}

	// T071: Log credential add (FR-020)
	v.LogAudit(security.EventCredentialAdd, security.OutcomeSuccess, service)
	return nil
}

// GetCredential retrieves a credential without automatic tracking
// Callers should explicitly track field access using RecordFieldAccess
// Deprecated trackUsage parameter is ignored (kept for backward compatibility)
func (v *VaultService) GetCredential(service string, trackUsage bool) (*Credential, error) {
	if !v.unlocked {
		return nil, ErrVaultLocked
	}

	credential, exists := v.vaultData.Credentials[service]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrCredentialNotFound, service)
	}

	// NOTE: Automatic tracking removed - callers must explicitly call RecordFieldAccess
	// with the specific field being accessed (password, username, etc.)

	// T071: Log credential access (FR-020)
	v.LogAudit(security.EventCredentialAccess, security.OutcomeSuccess, service)

	// Return a deep copy to prevent external modification
	// (shallow copy would share the Password []byte backing array)
	cred := credential
	if credential.Password != nil {
		cred.Password = make([]byte, len(credential.Password))
		copy(cred.Password, credential.Password)
	}
	return &cred, nil
}

// RecordFieldAccess records access to a specific credential field at current location
func (v *VaultService) RecordFieldAccess(service, field string) error {
	credential, exists := v.vaultData.Credentials[service]
	if !exists {
		return ErrCredentialNotFound
	}

	// Get current working directory and resolve symlinks for canonical path
	// (fixes macOS /var -> /private/var symlink matching issue)
	location, err := os.Getwd()
	if err != nil {
		location = "unknown"
	} else {
		// Resolve symlinks to canonical path
		if canonical, err := filepath.EvalSymlinks(location); err == nil {
			location = canonical
		}
		// If symlink resolution fails, keep the original path
	}

	// Try to get git repo info
	gitRepo := v.getGitRepo(location)

	// Update or create usage record
	record, exists := credential.UsageRecord[location]
	if exists {
		// Increment total count
		record.Count++
		record.Timestamp = time.Now()

		// Initialize FieldAccess map if nil (backward compatibility)
		if record.FieldAccess == nil {
			record.FieldAccess = make(map[string]int)
		}

		// Increment field-specific count
		record.FieldAccess[field]++
	} else {
		// Create new record
		record = UsageRecord{
			Location:    location,
			Timestamp:   time.Now(),
			GitRepo:     gitRepo,
			Count:       1,
			FieldAccess: map[string]int{field: 1},
		}
	}

	credential.UsageRecord[location] = record
	v.vaultData.Credentials[service] = credential

	// Save to persist usage tracking
	return v.save()
}

// getGitRepo attempts to get the git repository for a directory
func (v *VaultService) getGitRepo(dir string) string {
	// Simple implementation - look for .git directory up the tree
	current := dir
	for {
		gitDir := filepath.Join(current, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			// Found .git directory, return the repo name (directory name)
			return filepath.Base(current)
		}

		parent := filepath.Dir(current)
		if parent == current {
			// Reached root
			break
		}
		current = parent
	}
	return ""
}

// ListCredentials returns all credential service names
func (v *VaultService) ListCredentials() ([]string, error) {
	if !v.unlocked {
		return nil, ErrVaultLocked
	}

	services := make([]string, 0, len(v.vaultData.Credentials))
	for service := range v.vaultData.Credentials {
		services = append(services, service)
	}

	return services, nil
}

// UpdateOpts contains optional fields for updating a credential
// Use pointers to distinguish between "don't change" (nil) and "set to empty/value" (non-nil)
// T020d: Password changed to *[]byte for memory security
type UpdateOpts struct {
	Username *string // nil = don't change, non-nil = set to value (even if empty)
	Password *[]byte // T020d: Changed to *[]byte for memory security
	Category *string
	URL      *string
	Notes    *string

	// TOTP fields (nil = don't change, non-nil = set value)
	TOTPSecret    *string // Base32 encoded TOTP secret (empty string clears TOTP)
	TOTPAlgorithm *string // SHA1, SHA256, SHA512
	TOTPDigits    *int    // 6 or 8
	TOTPPeriod    *int    // Period in seconds
	TOTPIssuer    *string // Issuer name
	ClearTOTP     bool    // If true, clears all TOTP fields
}

// CredentialMetadata contains non-sensitive credential information for listing
type CredentialMetadata struct {
	Service         string
	Username        string
	Category        string
	URL             string
	Notes           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ModifiedCount   int       // Number of times credential has been modified
	UsageCount      int       // Total usage count across all locations
	LastAccessed    time.Time // Most recent access time
	Locations       []string  // List of locations where accessed
	GitRepositories []string  // List of unique git repositories where accessed (for --by-project grouping)

	// TOTP metadata (non-sensitive)
	HasTOTP    bool   // Whether TOTP is configured for this credential
	TOTPIssuer string // Issuer name for display
}

// ListCredentialsWithMetadata returns all credentials with metadata (no passwords)
func (v *VaultService) ListCredentialsWithMetadata() ([]CredentialMetadata, error) {
	if !v.unlocked {
		return nil, ErrVaultLocked
	}

	metadata := make([]CredentialMetadata, 0, len(v.vaultData.Credentials))
	for _, cred := range v.vaultData.Credentials {
		meta := CredentialMetadata{
			Service:       cred.Service,
			Username:      cred.Username,
			Category:      cred.Category,
			URL:           cred.URL,
			Notes:         cred.Notes,
			CreatedAt:     cred.CreatedAt,
			UpdatedAt:     cred.UpdatedAt,
			ModifiedCount: cred.ModifiedCount,
		}

		// Calculate usage statistics
		var totalCount int
		var lastAccessed time.Time
		locations := make([]string, 0, len(cred.UsageRecord))
		gitRepos := make(map[string]bool) // Use map to track unique repos

		for loc, record := range cred.UsageRecord {
			totalCount += record.Count
			locations = append(locations, loc)
			if record.Timestamp.After(lastAccessed) {
				lastAccessed = record.Timestamp
			}
			// Collect unique git repositories
			if record.GitRepo != "" {
				gitRepos[record.GitRepo] = true
			}
		}

		// Convert git repos map to slice
		gitReposList := make([]string, 0, len(gitRepos))
		for repo := range gitRepos {
			gitReposList = append(gitReposList, repo)
		}

		meta.UsageCount = totalCount
		meta.LastAccessed = lastAccessed
		meta.Locations = locations
		meta.GitRepositories = gitReposList

		// TOTP metadata
		meta.HasTOTP = cred.TOTPSecret != ""
		meta.TOTPIssuer = cred.TOTPIssuer

		metadata = append(metadata, meta)
	}

	return metadata, nil
}

// UpdateCredential updates an existing credential using optional fields
// Use nil pointers to skip updating a field, non-nil to set (including to empty string)
// T020e: Added deferred cleanup for password if provided
func (v *VaultService) UpdateCredential(service string, opts UpdateOpts) error {
	// T020e: Clear password bytes after use (if provided)
	if opts.Password != nil {
		defer crypto.ClearBytes(*opts.Password)
	}

	if !v.unlocked {
		return ErrVaultLocked
	}

	credential, exists := v.vaultData.Credentials[service]
	if !exists {
		return fmt.Errorf("%w: %s", ErrCredentialNotFound, service)
	}

	// Track if any field was actually updated
	fieldUpdated := false

	// Update fields only if pointer is non-nil
	if opts.Username != nil {
		credential.Username = *opts.Username
		fieldUpdated = true
	}
	if opts.Password != nil {
		// T020e: Make a copy before storing to avoid clearing stored password
		passwordCopy := make([]byte, len(*opts.Password))
		copy(passwordCopy, *opts.Password)
		credential.Password = passwordCopy
		fieldUpdated = true
	}
	if opts.Category != nil {
		credential.Category = *opts.Category
		fieldUpdated = true
	}
	if opts.URL != nil {
		credential.URL = *opts.URL
		fieldUpdated = true
	}
	if opts.Notes != nil {
		credential.Notes = *opts.Notes
		fieldUpdated = true
	}

	// TOTP field updates with audit logging
	hadTOTPBefore := credential.TOTPSecret != ""
	totpCleared := false
	totpAdded := false
	totpUpdated := false

	if opts.ClearTOTP {
		// Clear all TOTP fields
		if hadTOTPBefore {
			totpCleared = true
		}
		credential.TOTPSecret = ""
		credential.TOTPAlgorithm = ""
		credential.TOTPDigits = 0
		credential.TOTPPeriod = 0
		credential.TOTPIssuer = ""
		fieldUpdated = true
	} else {
		if opts.TOTPSecret != nil {
			if hadTOTPBefore {
				totpUpdated = true
			} else if *opts.TOTPSecret != "" {
				totpAdded = true
			}
			credential.TOTPSecret = *opts.TOTPSecret
			fieldUpdated = true
		}
		if opts.TOTPAlgorithm != nil {
			credential.TOTPAlgorithm = *opts.TOTPAlgorithm
			fieldUpdated = true
		}
		if opts.TOTPDigits != nil {
			credential.TOTPDigits = *opts.TOTPDigits
			fieldUpdated = true
		}
		if opts.TOTPPeriod != nil {
			credential.TOTPPeriod = *opts.TOTPPeriod
			fieldUpdated = true
		}
		if opts.TOTPIssuer != nil {
			credential.TOTPIssuer = *opts.TOTPIssuer
			fieldUpdated = true
		}
	}

	// Only increment counter if something was actually modified
	if fieldUpdated {
		credential.ModifiedCount++
	}

	credential.UpdatedAt = time.Now()
	v.vaultData.Credentials[service] = credential

	if err := v.save(); err != nil {
		return err
	}

	// T071: Log credential update (FR-020)
	v.LogAudit(security.EventCredentialUpdate, security.OutcomeSuccess, service)

	// TOTP-specific audit events
	if totpCleared {
		v.LogAudit(security.EventTOTPClear, security.OutcomeSuccess, service)
	} else if totpAdded {
		v.LogAudit(security.EventTOTPAdd, security.OutcomeSuccess, service)
	} else if totpUpdated {
		v.LogAudit(security.EventTOTPUpdate, security.OutcomeSuccess, service)
	}

	return nil
}

// UpdateCredentialFields updates fields using the planned 6-parameter signature
// Empty strings mean "no change" to align with original plan semantics.
// Note: This wrapper cannot set a field to empty string. Use UpdateCredential with UpdateOpts for that.
// T020d: Converts string password to []byte for UpdateOpts
func (v *VaultService) UpdateCredentialFields(service, username, password, category, url, notes string) error {
	opts := UpdateOpts{}
	if username != "" {
		opts.Username = &username
	}
	if password != "" {
		// T020d: Convert string to []byte for opts.Password
		passwordBytes := []byte(password)
		opts.Password = &passwordBytes
	}
	if category != "" {
		opts.Category = &category
	}
	if url != "" {
		opts.URL = &url
	}
	if notes != "" {
		opts.Notes = &notes
	}
	return v.UpdateCredential(service, opts)
}

// DeleteCredential removes a credential from the vault
func (v *VaultService) DeleteCredential(service string) error {
	if !v.unlocked {
		return ErrVaultLocked
	}

	if _, exists := v.vaultData.Credentials[service]; !exists {
		return fmt.Errorf("%w: %s", ErrCredentialNotFound, service)
	}

	delete(v.vaultData.Credentials, service)

	if err := v.save(); err != nil {
		return err
	}

	// T071: Log credential delete (FR-020)
	v.LogAudit(security.EventCredentialDelete, security.OutcomeSuccess, service)
	return nil
}

// GetUsageStats returns usage statistics for a credential
func (v *VaultService) GetUsageStats(service string) (map[string]UsageRecord, error) {
	if !v.unlocked {
		return nil, ErrVaultLocked
	}

	credential, exists := v.vaultData.Credentials[service]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrCredentialNotFound, service)
	}

	// Return a copy to prevent external modification
	stats := make(map[string]UsageRecord, len(credential.UsageRecord))
	for loc, record := range credential.UsageRecord {
		stats[loc] = record
	}

	return stats, nil
}

// GetTOTPCode generates a TOTP code for the specified credential and logs the access
// Returns the code, remaining validity in seconds, and any error
func (v *VaultService) GetTOTPCode(service string) (string, int, error) {
	if !v.unlocked {
		return "", 0, ErrVaultLocked
	}

	credential, exists := v.vaultData.Credentials[service]
	if !exists {
		return "", 0, fmt.Errorf("%w: %s", ErrCredentialNotFound, service)
	}

	if !credential.HasTOTP() {
		return "", 0, fmt.Errorf("no TOTP configured for credential: %s", service)
	}

	code, remaining, err := credential.GetTOTPCode()
	if err != nil {
		v.LogAudit(security.EventTOTPAccess, security.OutcomeFailure, service)
		return "", 0, err
	}

	// Log TOTP access
	v.LogAudit(security.EventTOTPAccess, security.OutcomeSuccess, service)

	return code, remaining, nil
}

// ChangePassword changes the vault master password
// T012: Updated signature to accept []byte, T016: Added deferred cleanup
// T046: Added password policy validation (FR-016)
// T041: Updated to handle v2 vaults with DEK re-wrapping
func (v *VaultService) ChangePassword(newPassword []byte) error {
	defer crypto.ClearBytes(newPassword) // T016: Ensure cleanup even on error

	if !v.unlocked {
		return ErrVaultLocked
	}

	// T046 [US3]: Validate new password against policy (FR-016)
	passwordPolicy := &security.PasswordPolicy{
		MinLength:        12,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireDigit:     true,
		RequireSymbol:    true,
	}
	if err := passwordPolicy.Validate(newPassword); err != nil {
		// T051a: Record failure and check rate limit
		if rateLimitErr := v.rateLimiter.CheckAndRecordFailure(); rateLimitErr != nil {
			return rateLimitErr // Rate limit triggered
		}
		return fmt.Errorf("new password does not meet requirements: %w", err)
	}

	// T051a: Reset rate limiter on successful validation
	v.rateLimiter.Reset()

	// Marshal vault data
	data, err := json.Marshal(v.vaultData)
	if err != nil {
		return fmt.Errorf("failed to marshal vault data: %w", err)
	}

	// Check vault version to determine how to handle password change
	vaultVersion := v.storageService.GetVersion()
	newPasswordStr := string(newPassword)

	if vaultVersion == 2 {
		// T041: V2 vault - use ChangePasswordV2 to re-wrap DEK
		// V2 vaults require the old password to unwrap the DEK
		if v.masterPassword == nil {
			return errors.New("cannot change password: vault was unlocked via recovery, set a new password first")
		}
		oldPasswordStr := string(v.masterPassword)

		if err := v.storageService.ChangePasswordV2(data, oldPasswordStr, newPasswordStr, v.createAuditCallback()); err != nil {
			return fmt.Errorf("failed to save vault with new password: %w", err)
		}
	} else {
		// V1 vault - traditional password change
		// T033/T034: Check if iteration count needs upgrading
		targetIterations := crypto.GetIterations()
		currentIterations := v.storageService.GetIterations()

		needsMigration := currentIterations < targetIterations
		if needsMigration {
			// Migration opportunity: upgrade to stronger KDF
			fmt.Fprintf(os.Stderr, "Upgrading PBKDF2 iterations from %d to %d for improved security...\n",
				currentIterations, targetIterations)
			if err := v.storageService.SaveVaultWithIterations(data, newPasswordStr, targetIterations); err != nil {
				return fmt.Errorf("failed to save vault with new password: %w", err)
			}
		} else {
			// T034: Pass audit callback for atomic save logging
			if err := v.storageService.SaveVault(data, newPasswordStr, v.createAuditCallback()); err != nil {
				return fmt.Errorf("failed to save vault with new password: %w", err)
			}
		}
	}

	// Clear old password and update master password
	crypto.ClearBytes(v.masterPassword)
	v.masterPassword = make([]byte, len(newPassword))
	copy(v.masterPassword, newPassword)

	// Update keychain if available
	if v.keychainService.IsAvailable() {
		if err := v.keychainService.Store(newPasswordStr); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update password in keychain: %v\n", err)
		}
	}

	// T070: Log password change (FR-019)
	v.LogAudit(security.EventVaultPasswordChange, security.OutcomeSuccess, "")

	return nil
}

// SetPasswordAfterRecovery sets a new password after vault recovery.
// This is used when the vault was unlocked via recovery phrase (no old password available).
// The DEK is already available from the recovery unlock.
// Parameters: newPassword (new master password)
// Returns: error
func (v *VaultService) SetPasswordAfterRecovery(newPassword []byte) error {
	defer crypto.ClearBytes(newPassword)

	if !v.unlocked {
		return ErrVaultLocked
	}

	// Check that we have a recovery DEK
	if v.recoveryDEK == nil {
		return errors.New("no recovery DEK available: vault was not unlocked via recovery")
	}

	// Check vault version - must be v2
	vaultVersion := v.storageService.GetVersion()
	if vaultVersion != 2 {
		return errors.New("SetPasswordAfterRecovery only supported for v2 vaults")
	}

	// Validate new password against policy
	passwordPolicy := &security.PasswordPolicy{
		MinLength:        12,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireDigit:     true,
		RequireSymbol:    true,
	}
	if err := passwordPolicy.Validate(newPassword); err != nil {
		if rateLimitErr := v.rateLimiter.CheckAndRecordFailure(); rateLimitErr != nil {
			return rateLimitErr
		}
		return fmt.Errorf("new password does not meet requirements: %w", err)
	}
	v.rateLimiter.Reset()

	// Marshal vault data
	data, err := json.Marshal(v.vaultData)
	if err != nil {
		return fmt.Errorf("failed to marshal vault data: %w", err)
	}

	// Set password using the recovery DEK
	newPasswordStr := string(newPassword)
	if err := v.storageService.SetPasswordAfterRecoveryV2(data, newPasswordStr, v.recoveryDEK, v.createAuditCallback()); err != nil {
		return fmt.Errorf("failed to set new password: %w", err)
	}

	// Clear recovery DEK and set master password
	crypto.ClearBytes(v.recoveryDEK)
	v.recoveryDEK = nil

	v.masterPassword = make([]byte, len(newPassword))
	copy(v.masterPassword, newPassword)

	// Update keychain if available
	if v.keychainService.IsAvailable() {
		if err := v.keychainService.Store(newPasswordStr); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update password in keychain: %v\n", err)
		}
	}

	// Log password change
	v.LogAudit(security.EventVaultPasswordChange, security.OutcomeSuccess, "recovery")

	return nil
}

// WasUnlockedViaRecovery returns true if the vault was unlocked using recovery phrase
// and still has the recovery DEK available for SetPasswordAfterRecovery
func (v *VaultService) WasUnlockedViaRecovery() bool {
	return v.unlocked && v.recoveryDEK != nil
}

// EnableKeychain enables keychain integration for the vault.
func (v *VaultService) EnableKeychain(password []byte, force bool) error {
	if !v.keychainService.IsAvailable() {
		return keychain.ErrKeychainUnavailable
	}

	// T016: Load metadata to check if already enabled (FR-006)
	metadata, err := v.LoadMetadata()
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}

	// T016: Check if already enabled (idempotent behavior per FR-006)
	if metadata.KeychainEnabled && !force {
		return ErrKeychainAlreadyEnabled
	}

	// T016: Make a copy of password before Unlock() clears it
	// Unlock() has defer crypto.ClearBytes() which will zero the password
	passwordCopy := make([]byte, len(password))
	copy(passwordCopy, password)
	defer crypto.ClearBytes(passwordCopy)

	// Unlock the vault to verify the password (FR-005).
	if err := v.Unlock(passwordCopy); err != nil {
		v.LogAudit(security.EventKeychainEnable, security.OutcomeFailure, "")
		return fmt.Errorf("failed to unlock vault: %w", err)
	}
	defer v.Lock()

	// Store original password (not the cleared copy) in keychain
	if err := v.keychainService.Store(string(password)); err != nil {
		v.LogAudit(security.EventKeychainEnable, security.OutcomeFailure, "")
		return fmt.Errorf("failed to store password in keychain: %w", err)
	}

	// T016: Update metadata.KeychainEnabled = true (FR-003, FR-004)
	metadata.KeychainEnabled = true
	if err := v.SaveMetadata(metadata); err != nil {
		v.LogAudit(security.EventKeychainEnable, security.OutcomeFailure, "")
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	v.LogAudit(security.EventKeychainEnable, security.OutcomeSuccess, "")
	return nil
}

// GetKeychainStatus returns the current status of keychain integration.
func (v *VaultService) GetKeychainStatus() *KeychainStatus {
	available := v.keychainService.IsAvailable()
	var passwordStored bool
	if available {
		_, err := v.keychainService.Retrieve()
		passwordStored = (err == nil)
	}

	// This is a bit of a violation, as the backend name is a UI concern.
	// However, it's a small one, and keeps the cmd layer thinner.
	var backendName string
	switch runtime.GOOS {
	case "windows":
		backendName = "Windows Credential Manager"
	case "darwin":
		backendName = "macOS Keychain"
	case "linux":
		backendName = "Secret Service API (gnome-keyring/kwallet)"
	default:
		backendName = "unknown"
	}

	v.LogAudit(security.EventKeychainStatus, security.OutcomeSuccess, "")

	return &KeychainStatus{
		Available:      available,
		PasswordStored: passwordStored,
		BackendName:    backendName,
	}
}

// RemoveVault permanently deletes the vault file and its keychain entry.
func (v *VaultService) RemoveVault(force bool, removeAll bool) (*RemoveVaultResult, error) {
	// T016: Load metadata to check audit status before vault deletion
	meta, err := LoadMetadata(v.vaultPath)
	if err == nil && meta != nil && meta.AuditEnabled {
		// Initialize audit logging if enabled but not yet initialized
		if !v.auditEnabled {
			auditLogPath := filepath.Join(filepath.Dir(v.vaultPath), "audit.log")
			if err := v.EnableAudit(auditLogPath, v.vaultPath); err != nil {
				// Best-effort - continue even if audit init fails
				fmt.Fprintf(os.Stderr, "Warning: Failed to initialize audit: %v\n", err)
			}
		}
	}

	// T017: Log vault_remove_attempt before deletion
	v.LogAudit(security.EventVaultRemove, security.OutcomeAttempt, v.vaultPath)

	result := &RemoveVaultResult{}

	// Attempt to delete vault file
	err = os.Remove(v.vaultPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.FileNotFound = true
		} else if os.IsPermission(err) && !force {
			// T018: Log failure on permission error
			v.LogAudit(security.EventVaultRemove, security.OutcomeFailure, v.vaultPath)
			return nil, fmt.Errorf("vault file is in use or permission denied. Use --force to override")
		} else if !force {
			// T018: Log failure
			v.LogAudit(security.EventVaultRemove, security.OutcomeFailure, v.vaultPath)
			return nil, fmt.Errorf("failed to delete vault file: %w", err)
		}
		// If --force is set, continue even on errors
	} else {
		result.FileDeleted = true
	}

	// Attempt to delete keychain entry
	if v.keychainService.IsAvailable() {
		err = v.keychainService.Delete()
		if err != nil {
			if err == keychain.ErrPasswordNotFound {
				result.KeychainNotFound = true
			} else {
				// Keychain delete failed for other reason - warn but continue
				fmt.Fprintf(os.Stderr, "Warning: failed to delete keychain entry: %v\n", err)
			}
		} else {
			result.KeychainDeleted = true
		}
	} else {
		result.KeychainNotFound = true
	}

	// T018: Log success/failure based on deletion results
	if result.FileDeleted || result.KeychainDeleted {
		v.LogAudit(security.EventVaultRemove, security.OutcomeSuccess, v.vaultPath)
	} else {
		v.LogAudit(security.EventVaultRemove, security.OutcomeFailure, v.vaultPath)
	}

	// T019: Delete metadata file after final audit entry
	if err := DeleteMetadata(v.vaultPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to delete metadata: %v\n", err)
	}

	// Delete audit log after final audit entries are written
	auditLogPath := filepath.Join(filepath.Dir(v.vaultPath), "audit.log")
	err = os.Remove(auditLogPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.AuditLogNotFound = true
		} else {
			fmt.Fprintf(os.Stderr, "Warning: Failed to delete audit log: %v\n", err)
		}
	} else {
		result.AuditLogDeleted = true
	}

	// Optionally remove entire directory (including config)
	if removeAll {
		vaultDir := filepath.Dir(v.vaultPath)
		err = os.RemoveAll(vaultDir)
		if err != nil {
			return nil, fmt.Errorf("failed to remove directory %s: %w", vaultDir, err)
		}
		result.DirectoryDeleted = true
	}

	return result, nil
}

// LoadMetadata loads vault metadata
func (v *VaultService) LoadMetadata() (*Metadata, error) {
	return LoadMetadata(v.vaultPath)
}

// SaveMetadata saves vault metadata
func (v *VaultService) SaveMetadata(metadata *Metadata) error {
	return SaveMetadata(v.vaultPath, metadata)
}

// DeleteMetadata deletes vault metadata
func (v *VaultService) DeleteMetadata() error {
	return DeleteMetadata(v.vaultPath)
}

// PingKeychain checks if the keychain is available and responsive.
func (v *VaultService) PingKeychain() error {
	return v.keychainService.Ping()
}

// NeedsMigration checks if the vault needs migration.
// Returns true if:
//   - The vault is v1 format (needs migration to v2)
//   - The vault is v2 but missing challenge data (needs re-migration for 6-word recovery)
func (v *VaultService) NeedsMigration() (bool, error) {
	version := v.storageService.GetVersion()
	if version == 0 {
		return false, errors.New("vault does not exist or cannot be read")
	}

	// v1 always needs migration
	if version == 1 {
		return true, nil
	}

	// v2 may need re-migration if missing challenge data
	if version == 2 {
		meta, err := LoadMetadata(v.vaultPath)
		if err != nil {
			// Can't read metadata - assume needs re-migration
			return true, nil
		}
		if meta.Recovery == nil || len(meta.Recovery.ChallengePositions) == 0 {
			// v2 without challenge data - needs re-migration
			return true, nil
		}
	}

	return false, nil
}

// MigrateToV2 migrates a v1 vault to v2 format with DEK-based encryption.
// The vault must be unlocked before calling this method.
// Parameters:
//   - passphrase: optional recovery passphrase (25th word), can be nil
//
// Returns: mnemonic string (24 words) for user to write down, error
func (v *VaultService) MigrateToV2(passphrase []byte) (string, error) {
	if passphrase != nil {
		defer crypto.ClearBytes(passphrase)
	}

	if !v.unlocked {
		return "", ErrVaultLocked
	}

	// Verify vault is v1 OR v2 without challenge data (needs re-migration)
	version := v.storageService.GetVersion()
	needsRemigration := false
	if version == 2 {
		// Check if v2 vault is missing challenge data
		meta, err := LoadMetadata(v.vaultPath)
		if err != nil || meta.Recovery == nil || len(meta.Recovery.ChallengePositions) == 0 {
			needsRemigration = true
		} else {
			return "", errors.New("vault is already v2 with full recovery support")
		}
	} else if version != 1 {
		return "", errors.New("unsupported vault version")
	}

	// 1. Setup challenge-based recovery (generates mnemonic and challenge data)
	challengeSetup, err := recovery.SetupChallengeRecovery(&recovery.ChallengeSetupConfig{
		Passphrase: passphrase,
	})
	if err != nil {
		return "", fmt.Errorf("failed to setup recovery: %w", err)
	}
	defer crypto.ClearBytes(challengeSetup.RecoveryKEK)

	// 2. Generate new salt for password KEK
	salt, err := v.cryptoService.GenerateSalt()
	if err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// 3. Derive password KEK from current password
	iterations := crypto.GetIterations()
	passwordKEK, err := v.cryptoService.DeriveKey(v.masterPassword, salt, iterations)
	if err != nil {
		return "", fmt.Errorf("failed to derive password KEK: %w", err)
	}
	defer crypto.ClearBytes(passwordKEK)

	// 4. Generate and wrap DEK with both password KEK and recovery KEK
	keyWrapResult, err := crypto.GenerateAndWrapDEK(passwordKEK, challengeSetup.RecoveryKEK)
	if err != nil {
		return "", fmt.Errorf("failed to generate and wrap DEK: %w", err)
	}
	defer crypto.ClearBytes(keyWrapResult.DEK)

	// 5. Marshal current vault data
	data, err := json.Marshal(v.vaultData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal vault data: %w", err)
	}

	// 6. Perform atomic migration using storage service
	if needsRemigration {
		// For v2 re-migration, use the same MigrateToV2 path
		// The storage service will handle the atomic update
		err = v.storageService.MigrateToV2(
			data,
			keyWrapResult.DEK,
			keyWrapResult.PasswordWrapped.Ciphertext,
			keyWrapResult.PasswordWrapped.Nonce,
			salt,
			iterations,
			v.createAuditCallback(),
		)
	} else {
		err = v.storageService.MigrateToV2(
			data,
			keyWrapResult.DEK,
			keyWrapResult.PasswordWrapped.Ciphertext,
			keyWrapResult.PasswordWrapped.Nonce,
			salt,
			iterations,
			v.createAuditCallback(),
		)
	}
	if err != nil {
		return "", fmt.Errorf("migration failed: %w", err)
	}

	// 7. Complete recovery metadata with wrapped DEK
	recoveryMetadata := challengeSetup.Metadata
	recoveryMetadata.EncryptedRecoveryKey = keyWrapResult.RecoveryWrapped.Ciphertext
	recoveryMetadata.NonceRecovery = keyWrapResult.RecoveryWrapped.Nonce

	// 8. Update metadata file
	meta, err := LoadMetadata(v.vaultPath)
	if err != nil {
		// Create new metadata if it doesn't exist
		meta = &Metadata{
			Version:      "1.0",
			AuditEnabled: v.auditEnabled,
			CreatedAt:    time.Now(),
		}
	}
	meta.Recovery = recoveryMetadata
	meta.LastModified = time.Now()

	if err := SaveMetadata(v.vaultPath, meta); err != nil {
		// Log warning but don't fail migration - vault is already migrated
		fmt.Fprintf(os.Stderr, "Warning: failed to save metadata: %v\n", err)
	}

	// 9. Log migration event
	v.LogAudit(security.EventVaultPasswordChange, security.OutcomeSuccess, "migration_v1_to_v2")

	return challengeSetup.Mnemonic, nil
}

// RecoverWithMnemonic recovers vault access using a BIP39 mnemonic phrase.
// This is for v2 vaults that use the DEK architecture.
// Parameters:
//   - mnemonic: full 24-word BIP39 recovery phrase
//   - passphrase: optional passphrase (25th word), can be nil
//
// Returns: error
func (v *VaultService) RecoverWithMnemonic(mnemonic string, passphrase []byte) error {
	if passphrase != nil {
		defer crypto.ClearBytes(passphrase)
	}

	if v.unlocked {
		return nil // Already unlocked
	}

	// 1. Validate mnemonic
	if !bip39.IsMnemonicValid(mnemonic) {
		return errors.New("invalid recovery phrase")
	}

	// 2. Load recovery metadata from vault
	meta, err := LoadMetadata(v.vaultPath)
	if err != nil {
		return fmt.Errorf("failed to load vault metadata: %w", err)
	}

	if meta.Recovery == nil || !meta.Recovery.Enabled {
		return errors.New("recovery not enabled for this vault")
	}

	// 3. Check vault version - must be v2
	if meta.Recovery.Version != "2" {
		return errors.New("recovery with mnemonic only supported for v2 vaults")
	}

	// 4. Verify passphrase requirement
	if meta.Recovery.PassphraseRequired && len(passphrase) == 0 {
		return errors.New("passphrase required for recovery")
	}

	// 5. Derive recovery KEK from mnemonic
	seed := bip39.NewSeed(mnemonic, string(passphrase))
	defer crypto.ClearBytes(seed)

	recoveryKEK := argon2.IDKey(
		seed,
		meta.Recovery.KDFParams.SaltRecovery,
		meta.Recovery.KDFParams.Time,
		meta.Recovery.KDFParams.Memory,
		meta.Recovery.KDFParams.Threads,
		recovery.DefaultKeyLen,
	)
	defer crypto.ClearBytes(recoveryKEK)

	// 6. Unwrap DEK using recovery KEK
	wrappedKey := crypto.WrappedKey{
		Ciphertext: meta.Recovery.EncryptedRecoveryKey,
		Nonce:      meta.Recovery.NonceRecovery,
	}

	dek, err := crypto.UnwrapKey(wrappedKey, recoveryKEK)
	if err != nil {
		return errors.New("recovery failed: invalid phrase or passphrase")
	}
	defer crypto.ClearBytes(dek)

	// 7. Load vault data using the DEK
	data, err := v.storageService.LoadVaultWithKey(dek)
	if err != nil {
		return fmt.Errorf("failed to decrypt vault: %w", err)
	}

	// 8. Unmarshal vault data
	var vaultData VaultData
	if err := json.Unmarshal(data, &vaultData); err != nil {
		return fmt.Errorf("failed to parse vault data: %w", err)
	}

	// 9. Store in memory (no master password for recovery unlock)
	v.unlocked = true
	v.masterPassword = nil // Recovery unlock doesn't have a password yet
	v.vaultData = &vaultData

	// 10. Restore audit logging if enabled
	if vaultData.AuditEnabled && vaultData.AuditLogPath != "" && vaultData.VaultID != "" {
		if err := v.EnableAudit(vaultData.AuditLogPath, vaultData.VaultID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to restore audit logging: %v\n", err)
		}
	}

	// 11. Log recovery success
	v.LogAudit(security.EventVaultUnlock, security.OutcomeSuccess, "recovery")

	return nil
}
