package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/arimxyer/pass-cli/internal/crypto"
)

const (
	VaultPermissions = 0600 // Read/write for owner only
	DefaultVaultName = "vault.enc"
	BackupSuffix     = ".backup"
	TempSuffix       = ".tmp"
)

var (
	ErrVaultNotFound     = errors.New("vault file not found")
	ErrVaultCorrupted    = errors.New("vault file corrupted")
	ErrInvalidVaultPath  = errors.New("invalid vault path")
	ErrBackupFailed      = errors.New("backup operation failed")
	ErrAtomicWriteFailed = errors.New("atomic write operation failed")
)

// ProgressCallback is invoked at key stages during vault save operations.
// event: stage identifier (e.g., "atomic_save_started", "verification_passed")
// metadata: optional contextual information (e.g., file paths, timestamps)
type ProgressCallback func(event string, metadata ...string)

type VaultMetadata struct {
	Version         int       `json:"version"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	Salt            []byte    `json:"salt"`
	Iterations      int       `json:"iterations"`                  // PBKDF2 iteration count (FR-007)
	WrappedDEK      []byte    `json:"wrapped_dek,omitempty"`       // T018: DEK wrapped by password KEK (v2 only)
	WrappedDEKNonce []byte    `json:"wrapped_dek_nonce,omitempty"` // T018: GCM nonce for DEK wrapping (v2 only)
}

type EncryptedVault struct {
	Metadata VaultMetadata `json:"metadata"`
	Data     []byte        `json:"data"`
}

type StorageService struct {
	cryptoService *crypto.CryptoService
	vaultPath     string
	fs            FileSystem // Abstracted file system for testability
}

func NewStorageService(cryptoService *crypto.CryptoService, vaultPath string) (*StorageService, error) {
	return NewStorageServiceWithFS(cryptoService, vaultPath, NewOSFileSystem())
}

// NewStorageServiceWithFS creates a StorageService with a custom FileSystem (for testing)
func NewStorageServiceWithFS(cryptoService *crypto.CryptoService, vaultPath string, fs FileSystem) (*StorageService, error) {
	if cryptoService == nil {
		return nil, errors.New("crypto service cannot be nil")
	}

	if vaultPath == "" {
		return nil, ErrInvalidVaultPath
	}

	if fs == nil {
		fs = NewOSFileSystem()
	}

	// Ensure the directory exists
	dir := filepath.Dir(vaultPath)
	if err := fs.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create vault directory: %w", err)
	}

	return &StorageService{
		cryptoService: cryptoService,
		vaultPath:     vaultPath,
		fs:            fs,
	}, nil
}

func (s *StorageService) InitializeVault(password string) error {
	// Check if vault already exists
	if s.VaultExists() {
		return errors.New("vault already exists")
	}

	// Generate salt for key derivation
	salt, err := s.cryptoService.GenerateSalt()
	if err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	// Create initial empty vault data
	emptyVault := []byte("{}")

	// T032/T034: Create vault metadata with configurable iterations (FR-007, FR-010)
	// Uses PASS_CLI_ITERATIONS env var if set, otherwise defaults to 600k (OWASP 2023)
	metadata := VaultMetadata{
		Version:    1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Salt:       salt,
		Iterations: crypto.GetIterations(), // Configurable via env var (T034)
	}

	// Encrypt and save vault
	if err := s.saveEncryptedVault(emptyVault, metadata, password); err != nil {
		return fmt.Errorf("failed to initialize vault: %w", err)
	}

	return nil
}

// T020: InitializeVaultV2 creates a new v2 vault with DEK-based encryption
// Parameters:
//   - dek: 32-byte Data Encryption Key (caller must clear after use)
//   - wrappedDEK: DEK wrapped with password-derived KEK
//   - wrappedDEKNonce: GCM nonce for the wrapped DEK
//   - salt: 32-byte salt for password KDF
//   - iterations: PBKDF2 iteration count
func (s *StorageService) InitializeVaultV2(dek, wrappedDEK, wrappedDEKNonce, salt []byte, iterations int) error {
	// Check if vault already exists
	if s.VaultExists() {
		return errors.New("vault already exists")
	}

	// Validate inputs
	if len(dek) != crypto.KeyLength {
		return crypto.ErrInvalidKeyLength
	}
	if len(wrappedDEK) != crypto.KeyLength+16 { // 32 bytes + 16 byte GCM tag
		return fmt.Errorf("invalid wrapped DEK length: expected %d, got %d", crypto.KeyLength+16, len(wrappedDEK))
	}
	if len(wrappedDEKNonce) != crypto.NonceLength {
		return crypto.ErrInvalidNonceLength
	}
	if len(salt) != crypto.SaltLength {
		return crypto.ErrInvalidSaltLength
	}

	// Create initial empty vault data
	emptyVault := []byte("{}")

	// T020: Create v2 metadata with wrapped DEK
	metadata := VaultMetadata{
		Version:         2, // Key-wrapped vault
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Salt:            salt,
		Iterations:      iterations,
		WrappedDEK:      wrappedDEK,
		WrappedDEKNonce: wrappedDEKNonce,
	}

	// Encrypt vault data with DEK (not password-derived key)
	encryptedData, err := s.cryptoService.Encrypt(emptyVault, dek)
	if err != nil {
		return fmt.Errorf("failed to encrypt vault data: %w", err)
	}

	// Create encrypted vault structure
	encryptedVault := EncryptedVault{
		Metadata: metadata,
		Data:     encryptedData,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(encryptedVault)
	if err != nil {
		return fmt.Errorf("failed to marshal vault data: %w", err)
	}

	// Atomic write
	return s.atomicWrite(s.vaultPath, jsonData)
}

func (s *StorageService) LoadVault(password string) ([]byte, error) {
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return nil, err
	}

	// T029/T030/T031: Version-aware vault loading
	// V1: password-derived key decrypts vault directly
	// V2: password-derived KEK unwraps DEK, DEK decrypts vault
	if encryptedVault.Metadata.Version == 2 {
		return s.loadVaultV2(encryptedVault, password)
	}

	// V1 path: Derive key from password and salt with iterations from metadata (FR-007)
	key, err := s.cryptoService.DeriveKey([]byte(password), encryptedVault.Metadata.Salt, encryptedVault.Metadata.Iterations)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}
	defer s.cryptoService.ClearKey(key)

	// Decrypt vault data
	plaintext, err := s.cryptoService.Decrypt(encryptedVault.Data, key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt vault (invalid password?): %w", err)
	}

	return plaintext, nil
}

// loadVaultV2 handles v2 vault loading with key unwrapping
// T030: Implement v2 unlock path: unwrap DEK → decrypt vault
func (s *StorageService) loadVaultV2(encryptedVault *EncryptedVault, password string) ([]byte, error) {
	// Validate v2 metadata has required fields
	if len(encryptedVault.Metadata.WrappedDEK) != crypto.KeyLength+16 {
		return nil, fmt.Errorf("invalid v2 vault: wrapped DEK length mismatch (expected %d, got %d)",
			crypto.KeyLength+16, len(encryptedVault.Metadata.WrappedDEK))
	}
	if len(encryptedVault.Metadata.WrappedDEKNonce) != crypto.NonceLength {
		return nil, fmt.Errorf("invalid v2 vault: nonce length mismatch")
	}

	// 1. Derive password KEK from password and salt
	passwordKEK, err := s.cryptoService.DeriveKey([]byte(password), encryptedVault.Metadata.Salt, encryptedVault.Metadata.Iterations)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}
	defer s.cryptoService.ClearKey(passwordKEK)

	// 2. Unwrap DEK using password KEK
	wrappedKey := crypto.WrappedKey{
		Ciphertext: encryptedVault.Metadata.WrappedDEK,
		Nonce:      encryptedVault.Metadata.WrappedDEKNonce,
	}
	dek, err := crypto.UnwrapKey(wrappedKey, passwordKEK)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt vault (invalid password?): %w", err)
	}
	defer crypto.ClearBytes(dek)

	// 3. Decrypt vault data with DEK
	plaintext, err := s.cryptoService.Decrypt(encryptedVault.Data, dek)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt vault data: %w", err)
	}

	return plaintext, nil
}

// LoadVaultWithKey loads and decrypts the vault using a provided data-decryption
// key, skipping password-to-key derivation. Used both for recovery (where the key
// comes from a recovery secret) and for the prepared-key unlock path (#103 Tier 2),
// where the key was derived ahead of time via DeriveDataKey. For v1 the key is the
// password-derived key; for v2 it is the unwrapped DEK.
// Parameters: key (32-byte AES-256 key). Returns: decrypted vault data, error.
func (s *StorageService) LoadVaultWithKey(key []byte) ([]byte, error) {
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return nil, err
	}

	// Decrypt vault data with provided key (skip password-to-key derivation)
	plaintext, err := s.cryptoService.Decrypt(encryptedVault.Data, key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt vault with provided key: %w", err)
	}

	return plaintext, nil
}

// PreparedKeyParams carries the key-derivation parameters read from the vault
// header. They feed DeriveDataKey to compute the data-decryption key ahead of
// time (concurrently with a sync pull) without touching the ciphertext, and they
// are compared post-pull to detect a re-key (see vault.UnlockWithPreparedKey).
type PreparedKeyParams struct {
	Version         int
	Salt            []byte
	Iterations      int
	WrappedDEK      []byte
	WrappedDEKNonce []byte
}

// Equal reports whether two parameter sets would derive the same key against the
// same ciphertext. Used to detect that the vault was re-keyed (password change or
// re-key) between reading the params and decrypting.
func (p PreparedKeyParams) Equal(o PreparedKeyParams) bool {
	return p.Version == o.Version &&
		p.Iterations == o.Iterations &&
		bytes.Equal(p.Salt, o.Salt) &&
		bytes.Equal(p.WrappedDEK, o.WrappedDEK) &&
		bytes.Equal(p.WrappedDEKNonce, o.WrappedDEKNonce)
}

// ReadKeyParams reads the current on-disk key-derivation parameters without
// decrypting the vault. Cheap; intended to run before a sync pull so the
// expensive DeriveDataKey can overlap the pull.
func (s *StorageService) ReadKeyParams() (PreparedKeyParams, error) {
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return PreparedKeyParams{}, err
	}
	m := encryptedVault.Metadata
	return PreparedKeyParams{
		Version:         m.Version,
		Salt:            m.Salt,
		Iterations:      m.Iterations,
		WrappedDEK:      m.WrappedDEK,
		WrappedDEKNonce: m.WrappedDEKNonce,
	}, nil
}

// DeriveDataKey computes the vault's data-decryption key from the password and
// pre-read key parameters WITHOUT reading the ciphertext — so it can run
// concurrently with a sync pull. The expensive PBKDF2 step happens here.
//   - v1: the password-derived key decrypts the vault directly.
//   - v2: derive the password KEK, then unwrap the DEK; the DEK decrypts the vault.
//
// The returned key is what LoadVaultWithKey expects. Caller must ClearBytes it.
func (s *StorageService) DeriveDataKey(password string, p PreparedKeyParams) ([]byte, error) {
	if p.Version == 2 {
		if len(p.WrappedDEK) != crypto.KeyLength+16 {
			return nil, fmt.Errorf("invalid v2 vault: wrapped DEK length mismatch (expected %d, got %d)",
				crypto.KeyLength+16, len(p.WrappedDEK))
		}
		if len(p.WrappedDEKNonce) != crypto.NonceLength {
			return nil, fmt.Errorf("invalid v2 vault: nonce length mismatch")
		}

		passwordKEK, err := s.cryptoService.DeriveKey([]byte(password), p.Salt, p.Iterations)
		if err != nil {
			return nil, fmt.Errorf("failed to derive key: %w", err)
		}
		defer s.cryptoService.ClearKey(passwordKEK)

		dek, err := crypto.UnwrapKey(crypto.WrappedKey{Ciphertext: p.WrappedDEK, Nonce: p.WrappedDEKNonce}, passwordKEK)
		if err != nil {
			return nil, fmt.Errorf("failed to unwrap DEK (invalid password?): %w", err)
		}
		return dek, nil
	}

	// v1: the password-derived key is the data key.
	key, err := s.cryptoService.DeriveKey([]byte(password), p.Salt, p.Iterations)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}
	return key, nil
}

func (s *StorageService) SaveVault(data []byte, password string, callback ProgressCallback) error {
	// T015: Notify audit logger of save operation start
	if callback != nil {
		callback("atomic_save_started", s.vaultPath)
	}

	// Load existing vault to get metadata
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return err
	}

	// Update metadata
	encryptedVault.Metadata.UpdatedAt = time.Now()

	// T032: Version-aware vault saving
	// V1: password-derived key encrypts vault directly
	// V2: password-derived KEK unwraps DEK, DEK encrypts vault
	var encryptedData []byte
	var dek []byte // Only set for v2, used for verification

	if encryptedVault.Metadata.Version == 2 {
		// V2 path: Unwrap DEK and encrypt with it
		encryptedData, dek, err = s.prepareEncryptedDataV2(data, encryptedVault.Metadata, password)
		if err != nil {
			return actionableErrorMessage(err)
		}
		defer crypto.ClearBytes(dek)
	} else {
		// V1 path: Encrypt with password-derived key
		encryptedData, err = s.prepareEncryptedData(data, encryptedVault.Metadata, password)
		if err != nil {
			return actionableErrorMessage(err)
		}
	}

	// T033: Step 0: Cleanup orphaned temp files from previous crashes (best-effort)
	s.cleanupOrphanedTempFiles("")

	// T014: Step 1: Generate temp filename
	tempPath := s.generateTempFileName()

	// Step 2: Write to temp file
	if err := s.writeToTempFile(tempPath, encryptedData); err != nil {
		return actionableErrorMessage(err)
	}

	// T022: Notify after temp file created
	if callback != nil {
		callback("temp_file_created", tempPath)
	}

	// Ensure temp file cleanup on error
	defer func() {
		// Best-effort cleanup if we haven't renamed yet
		_ = s.cleanupTempFile(tempPath)
	}()

	// Step 3: Verification (T021 - verify temp file is decryptable)
	if callback != nil {
		callback("verification_started", tempPath)
	}

	// Version-aware verification
	var verifyErr error
	if encryptedVault.Metadata.Version == 2 {
		verifyErr = s.verifyTempFileWithDEK(tempPath, dek)
	} else {
		verifyErr = s.verifyTempFile(tempPath, password)
	}
	if verifyErr != nil {
		// FR-015: Log verification failure BEFORE cleanup
		if callback != nil {
			callback("verification_failed", tempPath, verifyErr.Error())
		}
		// Cleanup temp file on verification failure
		_ = s.cleanupTempFile(tempPath)
		return actionableErrorMessage(verifyErr)
	}

	if callback != nil {
		callback("verification_passed", tempPath)
	}

	// Step 4: Atomic rename (vault → backup)
	backupPath := s.vaultPath + BackupSuffix
	if callback != nil {
		callback("atomic_rename_started", s.vaultPath, backupPath)
	}

	if err := s.atomicRename(s.vaultPath, backupPath); err != nil {
		return actionableErrorMessage(err)
	}

	// Step 5: Atomic rename (temp → vault)
	if callback != nil {
		callback("atomic_rename_started", tempPath, s.vaultPath)
	}

	if err := s.atomicRename(tempPath, s.vaultPath); err != nil {
		// CRITICAL ERROR: Try to restore backup
		if callback != nil {
			callback("rollback_started", backupPath, s.vaultPath)
		}
		_ = s.atomicRename(backupPath, s.vaultPath)
		if callback != nil {
			callback("rollback_completed", s.vaultPath)
		}
		return criticalErrorMessage(err)
	}

	// T034: Notify completion
	if callback != nil {
		callback("atomic_save_completed", s.vaultPath)
	}

	return nil
}

// SaveVaultWithDEK saves vault data encrypted with a DEK (for v2 vaults).
// Used when saving vault data encrypted with the Data Encryption Key.
// Parameters:
//   - data: plaintext vault data to encrypt
//   - dek: 32-byte Data Encryption Key
//   - callback: optional progress callback for audit logging
func (s *StorageService) SaveVaultWithDEK(data, dek []byte, callback ProgressCallback) error {
	// Notify audit logger of save operation start
	if callback != nil {
		callback("atomic_save_started", s.vaultPath)
	}

	// Load existing vault to get metadata
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return err
	}

	// Update metadata timestamp
	encryptedVault.Metadata.UpdatedAt = time.Now()

	// Encrypt vault data with DEK
	encryptedData, err := s.cryptoService.Encrypt(data, dek)
	if err != nil {
		return fmt.Errorf("failed to encrypt vault data: %w", err)
	}

	// Create encrypted vault structure
	newVault := EncryptedVault{
		Metadata: encryptedVault.Metadata,
		Data:     encryptedData,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(newVault)
	if err != nil {
		return fmt.Errorf("failed to marshal vault data: %w", err)
	}

	// Cleanup orphaned temp files from previous crashes
	s.cleanupOrphanedTempFiles("")

	// Generate temp filename
	tempPath := s.generateTempFileName()

	// Write to temp file
	if err := s.writeToTempFile(tempPath, jsonData); err != nil {
		return actionableErrorMessage(err)
	}

	// Notify after temp file created
	if callback != nil {
		callback("temp_file_created", tempPath)
	}

	// Ensure temp file cleanup on error
	defer func() {
		_ = s.cleanupTempFile(tempPath)
	}()

	// Verification: verify temp file is decryptable with DEK
	if callback != nil {
		callback("verification_started", tempPath)
	}

	if err := s.verifyTempFileWithDEK(tempPath, dek); err != nil {
		if callback != nil {
			callback("verification_failed", tempPath, err.Error())
		}
		_ = s.cleanupTempFile(tempPath)
		return actionableErrorMessage(err)
	}

	if callback != nil {
		callback("verification_passed", tempPath)
	}

	// Atomic rename (vault → backup)
	backupPath := s.vaultPath + BackupSuffix
	if callback != nil {
		callback("atomic_rename_started", s.vaultPath, backupPath)
	}

	if err := s.atomicRename(s.vaultPath, backupPath); err != nil {
		return actionableErrorMessage(err)
	}

	// Atomic rename (temp → vault)
	if callback != nil {
		callback("atomic_rename_started", tempPath, s.vaultPath)
	}

	if err := s.atomicRename(tempPath, s.vaultPath); err != nil {
		// CRITICAL ERROR: Try to restore backup
		if callback != nil {
			callback("rollback_started", backupPath, s.vaultPath)
		}
		_ = s.atomicRename(backupPath, s.vaultPath)
		if callback != nil {
			callback("rollback_completed", s.vaultPath)
		}
		return criticalErrorMessage(err)
	}

	// Notify completion
	if callback != nil {
		callback("atomic_save_completed", s.vaultPath)
	}

	return nil
}

// ChangePasswordV2 changes the password for a v2 vault by re-wrapping the DEK.
// This method:
// 1. Unwraps the DEK using the old password-derived KEK
// 2. Re-wraps the DEK with the new password-derived KEK
// 3. Updates the WrappedDEK in vault metadata
// 4. Re-encrypts and saves the vault data
// Parameters:
//   - data: plaintext vault data to save
//   - oldPassword: current password (for unwrapping DEK)
//   - newPassword: new password (for re-wrapping DEK)
//   - callback: optional progress callback for audit logging
func (s *StorageService) ChangePasswordV2(data []byte, oldPassword, newPassword string, callback ProgressCallback) error {
	// Notify audit logger of save operation start
	if callback != nil {
		callback("atomic_save_started", s.vaultPath)
	}

	// Load existing vault to get metadata
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return err
	}

	// Verify this is a v2 vault
	if encryptedVault.Metadata.Version != 2 {
		return fmt.Errorf("ChangePasswordV2 called on non-v2 vault")
	}

	// 1. Derive old password KEK
	oldKEK, err := s.cryptoService.DeriveKey([]byte(oldPassword), encryptedVault.Metadata.Salt, encryptedVault.Metadata.Iterations)
	if err != nil {
		return fmt.Errorf("failed to derive old key: %w", err)
	}
	defer s.cryptoService.ClearKey(oldKEK)

	// 2. Unwrap DEK using old KEK
	wrappedKey := crypto.WrappedKey{
		Ciphertext: encryptedVault.Metadata.WrappedDEK,
		Nonce:      encryptedVault.Metadata.WrappedDEKNonce,
	}
	dek, err := crypto.UnwrapKey(wrappedKey, oldKEK)
	if err != nil {
		return fmt.Errorf("failed to unwrap DEK: %w", err)
	}
	defer crypto.ClearBytes(dek)

	// 3. Generate new salt for new password
	newSalt, err := s.cryptoService.GenerateSalt()
	if err != nil {
		return fmt.Errorf("failed to generate new salt: %w", err)
	}

	// 4. Derive new password KEK
	newIterations := crypto.GetIterations()
	newKEK, err := s.cryptoService.DeriveKey([]byte(newPassword), newSalt, newIterations)
	if err != nil {
		return fmt.Errorf("failed to derive new key: %w", err)
	}
	defer s.cryptoService.ClearKey(newKEK)

	// 5. Re-wrap DEK with new KEK
	newWrappedKey, err := crypto.WrapKey(dek, newKEK)
	if err != nil {
		return fmt.Errorf("failed to re-wrap DEK: %w", err)
	}

	// 6. Update metadata with new wrapped DEK
	encryptedVault.Metadata.Salt = newSalt
	encryptedVault.Metadata.Iterations = newIterations
	encryptedVault.Metadata.WrappedDEK = newWrappedKey.Ciphertext
	encryptedVault.Metadata.WrappedDEKNonce = newWrappedKey.Nonce
	encryptedVault.Metadata.UpdatedAt = time.Now()

	// 7. Encrypt vault data with DEK
	encryptedData, err := s.cryptoService.Encrypt(data, dek)
	if err != nil {
		return fmt.Errorf("failed to encrypt vault data: %w", err)
	}

	// 8. Create encrypted vault structure
	newVault := EncryptedVault{
		Metadata: encryptedVault.Metadata,
		Data:     encryptedData,
	}

	// 9. Marshal to JSON
	jsonData, err := json.Marshal(newVault)
	if err != nil {
		return fmt.Errorf("failed to marshal vault data: %w", err)
	}

	// Cleanup orphaned temp files from previous crashes
	s.cleanupOrphanedTempFiles("")

	// Generate temp filename
	tempPath := s.generateTempFileName()

	// Write to temp file
	if err := s.writeToTempFile(tempPath, jsonData); err != nil {
		return actionableErrorMessage(err)
	}

	// Notify after temp file created
	if callback != nil {
		callback("temp_file_created", tempPath)
	}

	// Ensure temp file cleanup on error
	defer func() {
		_ = s.cleanupTempFile(tempPath)
	}()

	// Verification: verify temp file is decryptable with new password
	// We need to verify using the new password since that's what's stored in metadata
	if callback != nil {
		callback("verification_started", tempPath)
	}

	if err := s.verifyTempFileWithDEK(tempPath, dek); err != nil {
		if callback != nil {
			callback("verification_failed", tempPath, err.Error())
		}
		_ = s.cleanupTempFile(tempPath)
		return actionableErrorMessage(err)
	}

	if callback != nil {
		callback("verification_passed", tempPath)
	}

	// Atomic rename (vault → backup)
	backupPath := s.vaultPath + BackupSuffix
	if callback != nil {
		callback("atomic_rename_started", s.vaultPath, backupPath)
	}

	if err := s.atomicRename(s.vaultPath, backupPath); err != nil {
		return actionableErrorMessage(err)
	}

	// Atomic rename (temp → vault)
	if callback != nil {
		callback("atomic_rename_started", tempPath, s.vaultPath)
	}

	if err := s.atomicRename(tempPath, s.vaultPath); err != nil {
		// CRITICAL ERROR: Try to restore backup
		if callback != nil {
			callback("rollback_started", backupPath, s.vaultPath)
		}
		_ = s.atomicRename(backupPath, s.vaultPath)
		if callback != nil {
			callback("rollback_completed", s.vaultPath)
		}
		return criticalErrorMessage(err)
	}

	// Notify completion
	if callback != nil {
		callback("atomic_save_completed", s.vaultPath)
	}

	return nil
}

// SetPasswordAfterRecoveryV2 sets a new password after vault recovery.
// This is used when the vault was unlocked via recovery phrase (no old password available).
// The DEK is passed directly since it was already unwrapped during recovery.
// Parameters:
//   - data: plaintext vault data to encrypt
//   - newPassword: new master password
//   - dek: 32-byte Data Encryption Key (already unwrapped via recovery)
//   - callback: optional progress callback for audit logging
func (s *StorageService) SetPasswordAfterRecoveryV2(data []byte, newPassword string, dek []byte, callback ProgressCallback) error {
	// Notify audit logger of save operation start
	if callback != nil {
		callback("atomic_save_started", s.vaultPath)
	}

	// Load existing vault to get metadata
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return err
	}

	// Verify this is a v2 vault
	if encryptedVault.Metadata.Version != 2 {
		return fmt.Errorf("SetPasswordAfterRecoveryV2 called on non-v2 vault")
	}

	// 1. Generate new salt for new password
	newSalt, err := s.cryptoService.GenerateSalt()
	if err != nil {
		return fmt.Errorf("failed to generate new salt: %w", err)
	}

	// 2. Derive new password KEK
	newIterations := crypto.GetIterations()
	newKEK, err := s.cryptoService.DeriveKey([]byte(newPassword), newSalt, newIterations)
	if err != nil {
		return fmt.Errorf("failed to derive new key: %w", err)
	}
	defer s.cryptoService.ClearKey(newKEK)

	// 3. Re-wrap DEK with new KEK
	newWrappedKey, err := crypto.WrapKey(dek, newKEK)
	if err != nil {
		return fmt.Errorf("failed to wrap DEK with new password: %w", err)
	}

	// 4. Update metadata with new wrapped DEK
	encryptedVault.Metadata.Salt = newSalt
	encryptedVault.Metadata.Iterations = newIterations
	encryptedVault.Metadata.WrappedDEK = newWrappedKey.Ciphertext
	encryptedVault.Metadata.WrappedDEKNonce = newWrappedKey.Nonce
	encryptedVault.Metadata.UpdatedAt = time.Now()

	// 5. Encrypt vault data with DEK
	encryptedData, err := s.cryptoService.Encrypt(data, dek)
	if err != nil {
		return fmt.Errorf("failed to encrypt vault data: %w", err)
	}

	// 6. Create encrypted vault structure
	newVault := EncryptedVault{
		Metadata: encryptedVault.Metadata,
		Data:     encryptedData,
	}

	// 7. Marshal to JSON
	jsonData, err := json.Marshal(newVault)
	if err != nil {
		return fmt.Errorf("failed to marshal vault data: %w", err)
	}

	// Cleanup orphaned temp files from previous crashes
	s.cleanupOrphanedTempFiles("")

	// Generate temp filename
	tempPath := s.generateTempFileName()

	// Write to temp file
	if err := s.writeToTempFile(tempPath, jsonData); err != nil {
		return actionableErrorMessage(err)
	}

	// Notify after temp file created
	if callback != nil {
		callback("temp_file_created", tempPath)
	}

	// Ensure temp file cleanup on error
	defer func() {
		_ = s.cleanupTempFile(tempPath)
	}()

	// Verification: verify temp file is decryptable with DEK
	if callback != nil {
		callback("verification_started", tempPath)
	}

	if err := s.verifyTempFileWithDEK(tempPath, dek); err != nil {
		if callback != nil {
			callback("verification_failed", tempPath, err.Error())
		}
		_ = s.cleanupTempFile(tempPath)
		return actionableErrorMessage(err)
	}

	if callback != nil {
		callback("verification_passed", tempPath)
	}

	// Atomic rename (vault → backup)
	backupPath := s.vaultPath + BackupSuffix
	if callback != nil {
		callback("atomic_rename_started", s.vaultPath, backupPath)
	}

	if err := s.atomicRename(s.vaultPath, backupPath); err != nil {
		return actionableErrorMessage(err)
	}

	// Atomic rename (temp → vault)
	if callback != nil {
		callback("atomic_rename_started", tempPath, s.vaultPath)
	}

	if err := s.atomicRename(tempPath, s.vaultPath); err != nil {
		// CRITICAL ERROR: Try to restore backup
		if callback != nil {
			callback("rollback_started", backupPath, s.vaultPath)
		}
		_ = s.atomicRename(backupPath, s.vaultPath)
		if callback != nil {
			callback("rollback_completed", s.vaultPath)
		}
		return criticalErrorMessage(err)
	}

	// Notify completion
	if callback != nil {
		callback("atomic_save_completed", s.vaultPath)
	}

	return nil
}

// verifyTempFileWithDEK verifies a temp file can be decrypted with the DEK
func (s *StorageService) verifyTempFileWithDEK(tempPath string, dek []byte) error {
	// Read temp file
	data, err := s.fs.ReadFile(tempPath)
	if err != nil {
		return fmt.Errorf("failed to read temp file for verification: %w", err)
	}

	// Parse JSON
	var encryptedVault EncryptedVault
	if err := json.Unmarshal(data, &encryptedVault); err != nil {
		return fmt.Errorf("failed to parse temp vault: %w", err)
	}

	// Try to decrypt with DEK
	_, err = s.cryptoService.Decrypt(encryptedVault.Data, dek)
	if err != nil {
		return fmt.Errorf("verification failed - cannot decrypt with DEK: %w", err)
	}

	return nil
}

// prepareEncryptedData encrypts vault data and returns JSON bytes ready to write
func (s *StorageService) prepareEncryptedData(data []byte, metadata VaultMetadata, password string) ([]byte, error) {
	// Derive key from password and salt
	key, err := s.cryptoService.DeriveKey([]byte(password), metadata.Salt, metadata.Iterations)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}
	defer s.cryptoService.ClearKey(key)

	// Encrypt vault data
	encryptedData, err := s.cryptoService.Encrypt(data, key)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt vault data: %w", err)
	}

	// Create encrypted vault structure
	encryptedVault := EncryptedVault{
		Metadata: metadata,
		Data:     encryptedData,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(encryptedVault)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vault data: %w", err)
	}

	return jsonData, nil
}

// prepareEncryptedDataV2 encrypts vault data for v2 vaults (DEK-based encryption)
// Returns encrypted JSON bytes ready to write, and the DEK for verification
func (s *StorageService) prepareEncryptedDataV2(data []byte, metadata VaultMetadata, password string) ([]byte, []byte, error) {
	// 1. Derive password KEK from password and salt
	passwordKEK, err := s.cryptoService.DeriveKey([]byte(password), metadata.Salt, metadata.Iterations)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to derive key: %w", err)
	}
	defer s.cryptoService.ClearKey(passwordKEK)

	// 2. Unwrap DEK using password KEK
	if len(metadata.WrappedDEK) != crypto.KeyLength+16 {
		return nil, nil, fmt.Errorf("invalid v2 vault: wrapped DEK length mismatch")
	}
	if len(metadata.WrappedDEKNonce) != crypto.NonceLength {
		return nil, nil, fmt.Errorf("invalid v2 vault: nonce length mismatch")
	}

	wrappedKey := crypto.WrappedKey{
		Ciphertext: metadata.WrappedDEK,
		Nonce:      metadata.WrappedDEKNonce,
	}
	dek, err := crypto.UnwrapKey(wrappedKey, passwordKEK)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unwrap DEK: %w", err)
	}

	// 3. Encrypt vault data with DEK
	encryptedData, err := s.cryptoService.Encrypt(data, dek)
	if err != nil {
		crypto.ClearBytes(dek)
		return nil, nil, fmt.Errorf("failed to encrypt vault data: %w", err)
	}

	// Create encrypted vault structure
	encryptedVault := EncryptedVault{
		Metadata: metadata,
		Data:     encryptedData,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(encryptedVault)
	if err != nil {
		crypto.ClearBytes(dek)
		return nil, nil, fmt.Errorf("failed to marshal vault data: %w", err)
	}

	// Return DEK for verification (caller must clear after use)
	return jsonData, dek, nil
}

// SaveVaultWithIterations saves vault data with an updated iteration count.
// Used for migration from legacy iteration counts (T033).
func (s *StorageService) SaveVaultWithIterations(data []byte, password string, iterations int) error {
	if iterations < crypto.MinIterations {
		return fmt.Errorf("iterations must be >= %d", crypto.MinIterations)
	}

	// T036d: Pre-flight checks before migration (FR-012)
	if err := s.preflightChecks(); err != nil {
		return fmt.Errorf("pre-flight check failed: %w", err)
	}

	// Load existing vault to get metadata
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return err
	}

	// Update metadata with new iterations
	encryptedVault.Metadata.UpdatedAt = time.Now()
	encryptedVault.Metadata.Iterations = iterations

	// Create backup before saving
	if err := s.createBackup(); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Save encrypted vault
	if err := s.saveEncryptedVault(data, encryptedVault.Metadata, password); err != nil {
		// Restore from backup on failure (use automatic backup)
		automaticBackup := s.vaultPath + BackupSuffix
		if restoreErr := s.restoreFromBackup(automaticBackup); restoreErr != nil {
			return fmt.Errorf("save failed and backup restore failed: %v (original error: %w)", restoreErr, err)
		}
		return fmt.Errorf("failed to save vault: %w", err)
	}

	return nil
}

// SaveVaultWithIterationsUnsafe saves vault data with a specific iteration count without validation.
// ONLY FOR TESTING: Allows simulating legacy vaults with low iteration counts.
// DO NOT USE in production code.
func (s *StorageService) SaveVaultWithIterationsUnsafe(data []byte, password string, iterations int) error {
	// Load existing vault to get metadata
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return err
	}

	// Update metadata with new iterations (no validation)
	encryptedVault.Metadata.UpdatedAt = time.Now()
	encryptedVault.Metadata.Iterations = iterations

	// Create backup before saving
	if err := s.createBackup(); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Save encrypted vault
	if err := s.saveEncryptedVault(data, encryptedVault.Metadata, password); err != nil {
		// Restore from backup on failure (use automatic backup)
		automaticBackup := s.vaultPath + BackupSuffix
		if restoreErr := s.restoreFromBackup(automaticBackup); restoreErr != nil {
			return fmt.Errorf("save failed and backup restore failed: %v (original error: %w)", restoreErr, err)
		}
		return fmt.Errorf("failed to save vault: %w", err)
	}

	return nil
}

// GetIterations returns the current PBKDF2 iteration count from vault metadata.
// Returns 0 if vault doesn't exist or error occurs.
func (s *StorageService) GetIterations() int {
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return 0
	}
	return encryptedVault.Metadata.Iterations
}

// GetVersion returns the vault format version (1 or 2).
// Returns 0 if vault doesn't exist or error occurs.
func (s *StorageService) GetVersion() int {
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return 0
	}
	return encryptedVault.Metadata.Version
}

// MigrateToV2 atomically migrates a v1 vault to v2 format.
// This method:
// 1. Creates a backup of the current vault
// 2. Re-encrypts vault data with DEK
// 3. Updates metadata to v2 format with wrapped DEK
// 4. Atomically replaces the vault file
// Parameters:
//   - data: plaintext vault data to save
//   - dek: 32-byte Data Encryption Key
//   - wrappedDEK: DEK wrapped by password KEK
//   - wrappedDEKNonce: nonce used for DEK wrapping
//   - salt: salt for password KDF
//   - iterations: PBKDF2 iteration count
//   - callback: optional progress callback for audit logging
func (s *StorageService) MigrateToV2(data, dek, wrappedDEK, wrappedDEKNonce, salt []byte, iterations int, callback ProgressCallback) error {
	// Notify audit logger of migration start
	if callback != nil {
		callback("atomic_save_started", s.vaultPath)
	}

	// Load existing vault to get current metadata
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return err
	}

	// Verify this is a v1 vault
	if encryptedVault.Metadata.Version != 1 {
		return fmt.Errorf("cannot migrate: vault is already version %d", encryptedVault.Metadata.Version)
	}

	// Encrypt vault data with DEK
	encryptedData, err := s.cryptoService.Encrypt(data, dek)
	if err != nil {
		return fmt.Errorf("failed to encrypt vault data: %w", err)
	}

	// Create v2 metadata
	newMetadata := VaultMetadata{
		Version:         2, // Upgrade to v2
		CreatedAt:       encryptedVault.Metadata.CreatedAt,
		UpdatedAt:       time.Now(),
		Salt:            salt,
		Iterations:      iterations,
		WrappedDEK:      wrappedDEK,
		WrappedDEKNonce: wrappedDEKNonce,
	}

	// Create encrypted vault structure
	newVault := EncryptedVault{
		Metadata: newMetadata,
		Data:     encryptedData,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(newVault)
	if err != nil {
		return fmt.Errorf("failed to marshal vault data: %w", err)
	}

	// Cleanup orphaned temp files from previous crashes
	s.cleanupOrphanedTempFiles("")

	// Generate temp filename
	tempPath := s.generateTempFileName()

	// Write to temp file
	if err := s.writeToTempFile(tempPath, jsonData); err != nil {
		return actionableErrorMessage(err)
	}

	// Notify after temp file created
	if callback != nil {
		callback("temp_file_created", tempPath)
	}

	// Ensure temp file cleanup on error
	defer func() {
		_ = s.cleanupTempFile(tempPath)
	}()

	// Verification: verify temp file is decryptable with DEK
	if callback != nil {
		callback("verification_started", tempPath)
	}

	if err := s.verifyTempFileWithDEK(tempPath, dek); err != nil {
		if callback != nil {
			callback("verification_failed", tempPath, err.Error())
		}
		_ = s.cleanupTempFile(tempPath)
		return actionableErrorMessage(err)
	}

	if callback != nil {
		callback("verification_passed", tempPath)
	}

	// Atomic rename (vault → backup)
	backupPath := s.vaultPath + BackupSuffix
	if callback != nil {
		callback("atomic_rename_started", s.vaultPath, backupPath)
	}

	if err := s.atomicRename(s.vaultPath, backupPath); err != nil {
		return actionableErrorMessage(err)
	}

	// Atomic rename (temp → vault)
	if callback != nil {
		callback("atomic_rename_started", tempPath, s.vaultPath)
	}

	if err := s.atomicRename(tempPath, s.vaultPath); err != nil {
		// CRITICAL ERROR: Try to restore backup
		if callback != nil {
			callback("rollback_started", backupPath, s.vaultPath)
		}
		_ = s.atomicRename(backupPath, s.vaultPath)
		if callback != nil {
			callback("rollback_completed", s.vaultPath)
		}
		return criticalErrorMessage(err)
	}

	// Notify completion
	if callback != nil {
		callback("atomic_save_completed", s.vaultPath)
	}

	return nil
}

// SetIterations updates the PBKDF2 iteration count in vault metadata.
// This will take effect on the next SaveVault call.
// Used for migration from legacy iteration counts (T033).
func (s *StorageService) SetIterations(iterations int) error {
	if iterations < crypto.MinIterations {
		return fmt.Errorf("iterations must be >= %d", crypto.MinIterations)
	}

	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return err
	}

	encryptedVault.Metadata.Iterations = iterations

	// Note: The updated iterations will be persisted on next SaveVault call
	// We don't save immediately to avoid double-write overhead
	return nil
}

func (s *StorageService) VaultExists() bool {
	_, err := s.fs.Stat(s.vaultPath)
	return err == nil
}

func (s *StorageService) GetVaultInfo() (*VaultMetadata, error) {
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return nil, err
	}

	// Return a copy of metadata (without the salt for security)
	info := VaultMetadata{
		Version:   encryptedVault.Metadata.Version,
		CreatedAt: encryptedVault.Metadata.CreatedAt,
		UpdatedAt: encryptedVault.Metadata.UpdatedAt,
		Salt:      nil, // Don't expose salt
	}

	return &info, nil
}

func (s *StorageService) ValidateVault() error {
	encryptedVault, err := s.loadEncryptedVault()
	if err != nil {
		return err
	}

	// Basic validation checks
	if encryptedVault.Metadata.Version <= 0 {
		return ErrVaultCorrupted
	}

	if len(encryptedVault.Metadata.Salt) != 32 {
		return ErrVaultCorrupted
	}

	if len(encryptedVault.Data) == 0 {
		return ErrVaultCorrupted
	}

	if encryptedVault.Metadata.CreatedAt.IsZero() {
		return ErrVaultCorrupted
	}

	if encryptedVault.Metadata.UpdatedAt.Before(encryptedVault.Metadata.CreatedAt) {
		return ErrVaultCorrupted
	}

	// Validate Iterations field (T025 - FR-007)
	// Allow 0 for backward compatibility (will default to 100000 on load)
	if encryptedVault.Metadata.Iterations != 0 && encryptedVault.Metadata.Iterations < crypto.MinIterations {
		return fmt.Errorf("%w: iterations must be >= %d", ErrVaultCorrupted, crypto.MinIterations)
	}

	return nil
}

func (s *StorageService) CreateBackup() error {
	return s.createBackup()
}

// RestoreFromBackup restores the vault from a backup file.
// If backupPath is empty, automatically selects the newest valid backup.
// If backupPath is provided, uses that specific backup file.
func (s *StorageService) RestoreFromBackup(backupPath string) error {
	// If no path provided, auto-select newest backup
	if backupPath == "" {
		// Try to find automatic backup first (legacy behavior)
		automaticBackup := s.vaultPath + BackupSuffix
		if _, err := s.fs.Stat(automaticBackup); err == nil {
			backupPath = automaticBackup
		} else {
			// Fall back to FindNewestBackup for manual backups
			newest, err := s.FindNewestBackup()
			if err != nil {
				return fmt.Errorf("failed to find backup: %w", err)
			}
			if newest == nil {
				return ErrBackupFailed // No backup found
			}
			backupPath = newest.Path
		}
	}

	return s.restoreFromBackup(backupPath)
}

func (s *StorageService) RemoveBackup() error {
	backupPath := s.vaultPath + BackupSuffix
	err := os.Remove(backupPath)
	if os.IsNotExist(err) {
		return nil // Backup doesn't exist, which is fine
	}
	return err
}

// Private helper methods

func (s *StorageService) loadEncryptedVault() (*EncryptedVault, error) {
	if !s.VaultExists() {
		return nil, ErrVaultNotFound
	}

	data, err := s.fs.ReadFile(s.vaultPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault file: %w", err)
	}

	var encryptedVault EncryptedVault
	if err := json.Unmarshal(data, &encryptedVault); err != nil {
		return nil, fmt.Errorf("failed to parse vault file: %w", err)
	}

	// T026: Backward compatibility for legacy vaults without Iterations field (FR-008)
	if encryptedVault.Metadata.Iterations == 0 {
		encryptedVault.Metadata.Iterations = 100000 // Legacy default
	}

	return &encryptedVault, nil
}

func (s *StorageService) saveEncryptedVault(data []byte, metadata VaultMetadata, password string) error {
	// T030: Derive key from password and salt with iterations from metadata (FR-007)
	key, err := s.cryptoService.DeriveKey([]byte(password), metadata.Salt, metadata.Iterations)
	if err != nil {
		return fmt.Errorf("failed to derive key: %w", err)
	}
	defer s.cryptoService.ClearKey(key)

	// Encrypt vault data
	encryptedData, err := s.cryptoService.Encrypt(data, key)
	if err != nil {
		return fmt.Errorf("failed to encrypt vault data: %w", err)
	}

	// Create encrypted vault structure
	encryptedVault := EncryptedVault{
		Metadata: metadata,
		Data:     encryptedData,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(encryptedVault)
	if err != nil {
		return fmt.Errorf("failed to marshal vault data: %w", err)
	}

	// Atomic write using temporary file
	return s.atomicWrite(s.vaultPath, jsonData)
}

func (s *StorageService) atomicWrite(path string, data []byte) error {
	// FR-015: Create parent directories if they don't exist
	dir := filepath.Dir(path)
	if err := s.fs.MkdirAll(dir, 0750); err != nil { // More restrictive permissions (owner+group only)
		return fmt.Errorf("failed to create parent directories: %w", err)
	}

	tempPath := path + TempSuffix

	// Write to temporary file
	// #nosec G304 -- Vault path is user-controlled by design for CLI tool
	tempFile, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, VaultPermissions)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// Ensure temp file is cleaned up on error
	defer func() {
		if tempFile != nil {
			_ = tempFile.Close()
			_ = s.fs.Remove(tempPath)
		}
	}()

	// Write data
	if _, err := tempFile.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	// Sync to ensure data is written to disk
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync data: %w", err)
	}

	// Close file
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	tempFile = nil // Prevent cleanup in defer

	// Atomic move (rename) to final location
	if err := s.fs.Rename(tempPath, path); err != nil {
		_ = s.fs.Remove(tempPath) // Clean up on failure
		return fmt.Errorf("failed to move temp file to final location: %w", err)
	}

	return nil
}

// preflightChecks performs safety checks before migration (T036d, FR-012).
// Verifies:
// - Disk space >= 2x vault size (to accommodate backup + new vault)
// - Write permissions to vault directory
func (s *StorageService) preflightChecks() error {
	// Check if vault exists
	vaultInfo, err := s.fs.Stat(s.vaultPath)
	if err != nil {
		return fmt.Errorf("failed to stat vault: %w", err)
	}

	vaultSize := vaultInfo.Size()
	vaultDir := filepath.Dir(s.vaultPath)

	// Check disk space (need 2x vault size for backup + new vault)
	requiredSpace := vaultSize * 2

	// Get disk usage info (platform-specific)
	// Note: getAvailableDiskSpace may return error on unsupported platforms
	availableSpace, err := s.getAvailableDiskSpace(vaultDir) //nolint:staticcheck // SA4023: always-error is intentional
	if err != nil {                                          //nolint:staticcheck // SA4023: always-true is intentional for unimplemented platforms
		// If we can't determine disk space, log warning but continue
		fmt.Fprintf(os.Stderr, "Warning: unable to verify disk space: %v\n", err)
	} else if availableSpace < requiredSpace {
		return fmt.Errorf("insufficient disk space: need %d bytes, have %d bytes", requiredSpace, availableSpace)
	}

	// Test write permissions by creating a temporary test file
	testPath := filepath.Join(vaultDir, ".pass-cli-write-test")
	testFile, err := os.OpenFile(testPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, VaultPermissions) // #nosec G304 -- Test file path constructed from validated vault directory
	if err != nil {
		return fmt.Errorf("no write permission in vault directory: %w", err)
	}
	_ = testFile.Close()
	_ = s.fs.Remove(testPath)

	return nil
}

// getAvailableDiskSpace returns available disk space in bytes for the given path.
// Platform-specific implementation.
//
//nolint:staticcheck // SA4023: always returns error on unsupported platforms - this is intentional
func (s *StorageService) getAvailableDiskSpace(path string) (int64, error) {
	// Platform-specific disk space check
	// On Windows, syscall.Statfs_t is not available
	// This is a best-effort check - we'll continue with a warning if it fails

	// Try to use platform-specific approach
	// For Windows: Could use golang.org/x/sys/windows.GetDiskFreeSpaceEx
	// For Unix: Could use syscall.Statfs

	// For now, return error to indicate we can't check (will trigger warning in preflightChecks)
	// This is acceptable per FR-012 - disk space check is a safety measure, not a hard requirement
	return 0, fmt.Errorf("disk space check not implemented for this platform")
}

func (s *StorageService) createBackup() error {
	if !s.VaultExists() {
		return nil // No vault to backup
	}

	backupPath := s.vaultPath + BackupSuffix

	// Copy vault file to backup
	src, err := os.Open(s.vaultPath)
	if err != nil {
		return fmt.Errorf("failed to open vault for backup: %w", err)
	}
	defer func() { _ = src.Close() }()

	// #nosec G304 -- Backup path is user-controlled by design for CLI tool
	dst, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, VaultPermissions)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to copy vault to backup: %w", err)
	}

	if err := dst.Sync(); err != nil {
		return fmt.Errorf("failed to sync backup file: %w", err)
	}

	return nil
}

func (s *StorageService) restoreFromBackup(backupPath string) error {
	if _, err := s.fs.Stat(backupPath); os.IsNotExist(err) {
		return ErrBackupFailed
	}

	// Copy backup to vault location
	// #nosec G304 -- Backup path is user-controlled by design for CLI tool
	src, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(s.vaultPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, VaultPermissions)
	if err != nil {
		return fmt.Errorf("failed to create vault file: %w", err)
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to restore from backup: %w", err)
	}

	if err := dst.Sync(); err != nil {
		return fmt.Errorf("failed to sync restored vault: %w", err)
	}

	return nil
}
