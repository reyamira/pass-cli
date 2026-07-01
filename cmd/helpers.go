package cmd

import (
	"bufio"
	"fmt"
	"github.com/arimxyer/pass-cli/internal/crypto"
	"github.com/arimxyer/pass-cli/internal/recovery"
	"github.com/arimxyer/pass-cli/internal/timing"
	"github.com/arimxyer/pass-cli/internal/vault"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/howeyc/gopass"
	"github.com/olekukonko/tablewriter"
	"golang.org/x/term"
)

// Package-level scanner for test mode stdin reading
// Shared across ALL stdin reads (passwords, usernames, etc.) to avoid buffering issues
// This ensures consistent cross-platform behavior for piped stdin
var (
	testStdinScanner *bufio.Scanner
	scannerOnce      sync.Once
)

// readLine reads a line from stdin in test mode using the shared scanner
// This prevents multiple readers from conflicting when reading piped stdin
func readLine() (string, error) {
	if os.Getenv("PASS_CLI_TEST") != "1" {
		return "", fmt.Errorf("readLine should only be called in test mode")
	}

	// Initialize scanner once and reuse for all stdin reads
	scannerOnce.Do(func() {
		testStdinScanner = bufio.NewScanner(os.Stdin)
	})

	if !testStdinScanner.Scan() {
		if err := testStdinScanner.Err(); err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		return "", fmt.Errorf("no input provided")
	}
	return testStdinScanner.Text(), nil
}

// readLineInput reads a line from stdin, using the shared scanner in test mode
// or a fresh reader in normal mode. This is the general-purpose line reader
// for user prompts that aren't passwords.
func readLineInput() (string, error) {
	if os.Getenv("PASS_CLI_TEST") == "1" {
		return readLine()
	}

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// readPassword reads a password from stdin with asterisk masking.
// Returns []byte for secure memory handling (no string conversion).
func readPassword() ([]byte, error) {
	// Check if running in test mode first (before terminal check)
	// This is necessary because on macOS, term.IsTerminal() returns true even in test environments
	if os.Getenv("PASS_CLI_TEST") == "1" {
		// In test mode, use shared scanner for all stdin reads
		line, err := readLine()
		if err != nil {
			return nil, fmt.Errorf("failed to read password: %w", err)
		}
		return []byte(line), nil
	}

	// Get file descriptor for stdin
	fd := int(os.Stdin.Fd())

	// Check if stdin is a terminal
	if !term.IsTerminal(fd) {
		// Not a terminal, read normally (for testing/scripts)
		var password string
		_, err := fmt.Scanln(&password)
		return []byte(password), err
	}

	// Read password with asterisk masking using gopass
	passwordBytes, err := gopass.GetPasswdMasked()
	if err != nil {
		return nil, err
	}

	return passwordBytes, nil
}

// T072: getAuditLogPath returns the audit log path from environment variable or default
// Per FR-023: PASS_AUDIT_LOG environment variable for custom log location
func getAuditLogPath(vaultPath string) string {
	// Check environment variable first
	if auditPath := os.Getenv("PASS_AUDIT_LOG"); auditPath != "" {
		return auditPath
	}

	// Default: <vault-dir>/audit.log
	vaultDir := filepath.Dir(vaultPath)
	return filepath.Join(vaultDir, "audit.log")
}

// T072: getVaultID returns a unique identifier for the vault (used for keychain)
// Uses directory name as vault ID to match initialization behavior in firstrun.go
func getVaultID(vaultPath string) string {
	// Use directory name as vault ID to match how it's set during initialization
	// This ensures audit key retrieval matches how it was stored
	vaultDir := filepath.Dir(vaultPath)
	return filepath.Base(vaultDir)
}

// getKeychainUnavailableMessage returns platform-specific error message when keychain is unavailable
// Per research.md Decision 5 and FR-007 (clear, actionable error messages)
func getKeychainUnavailableMessage() string {
	unavailableMessages := map[string]string{
		"windows": "System keychain not available: Windows Credential Manager access denied.\nTroubleshooting: Check user permissions for Credential Manager access.",
		"darwin":  "System keychain not available: macOS Keychain access denied.\nTroubleshooting: Check Keychain Access.app permissions for pass-cli.",
		"linux":   "System keychain not available: Linux Secret Service not running or accessible.\nTroubleshooting: Ensure gnome-keyring or KWallet is installed and running.",
	}

	msg, ok := unavailableMessages[runtime.GOOS]
	if !ok {
		return "System keychain not available on this platform."
	}
	return msg
}

// T001: formatRelativeTime converts timestamp to human-readable relative time
// Per FR-016: Display timestamps in human-readable format (e.g., "2 hours ago", "3 days ago") for table output
func formatRelativeTime(timestamp time.Time) string {
	now := time.Now()
	duration := now.Sub(timestamp)

	// Handle future timestamps (shouldn't happen, but be defensive)
	if duration < 0 {
		return "in the future"
	}

	// Less than a minute
	if duration < time.Minute {
		return "just now"
	}

	// Less than an hour
	if duration < time.Hour {
		minutes := int(duration.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	}

	// Less than a day
	if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}

	// Less than a week
	if duration < 7*24*time.Hour {
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}

	// Less than a month (30 days)
	if duration < 30*24*time.Hour {
		weeks := int(duration.Hours() / (24 * 7))
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	}

	// Less than a year
	if duration < 365*24*time.Hour {
		months := int(duration.Hours() / (24 * 30))
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	}

	// Years
	years := int(duration.Hours() / (24 * 365))
	if years == 1 {
		return "1 year ago"
	}
	return fmt.Sprintf("%d years ago", years)
}

// T002: pathExists checks if a file or directory exists at the given path
// Per FR-018/FR-019: Check path existence for deleted directory handling
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// T003: formatFieldCounts formats field access counts for display
// Per FR-002: Display field-level usage breakdown (password:5, username:2, etc.)
func formatFieldCounts(fieldCounts map[string]int) string {
	if len(fieldCounts) == 0 {
		return "-"
	}

	// Sort field names for consistent output
	fields := make([]string, 0, len(fieldCounts))
	for field := range fieldCounts {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	// Build formatted string
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		count := fieldCounts[field]
		parts = append(parts, fmt.Sprintf("%s:%d", field, count))
	}

	return strings.Join(parts, ", ")
}

// T004: formatUsageTable formats usage records as a styled table
// Per contracts/commands.md: Table format with columns for Location, Repository, Last Used, Count, Fields
func formatUsageTable(records []vault.UsageRecord) string {
	if len(records) == 0 {
		return ""
	}

	var builder strings.Builder
	table := tablewriter.NewWriter(&builder)

	// Configure table style
	table.Header([]string{"Location", "Repository", "Last Used", "Count", "Fields"})

	// Prepare data rows
	var data [][]string
	for _, record := range records {
		location := record.Location
		repository := record.GitRepo
		if repository == "" {
			repository = "-"
		}
		lastUsed := formatRelativeTime(record.Timestamp)
		count := fmt.Sprintf("%d", record.Count)
		fields := formatFieldCounts(record.FieldAccess)

		data = append(data, []string{location, repository, lastUsed, count, fields})
	}

	_ = table.Bulk(data)
	_ = table.Render()
	return builder.String()
}

// resolveCredentialField returns the requested field's value from a credential
// along with its canonical name (for usage tracking). It is the single source of
// truth for the field aliases accepted by `get` and `exec`, keeping the valid-field
// list from drifting between the two commands.
//
// Security note: for the password field this returns string(cred.Password); the
// caller is responsible for clearing the source []byte (crypto.ClearBytes) and for
// never printing or copying the value where the brief forbids it.
func resolveCredentialField(cred *vault.Credential, field string) (value string, canonical string, err error) {
	switch strings.ToLower(field) {
	case "username", "user", "u":
		return cred.Username, "username", nil
	case "password", "pass", "p":
		return string(cred.Password), "password", nil
	case "category", "cat", "c":
		return cred.Category, "category", nil
	case "url":
		return cred.URL, "url", nil
	case "notes", "note", "n":
		return cred.Notes, "notes", nil
	case "service", "s":
		return cred.Service, "service", nil
	default:
		return "", "", fmt.Errorf("invalid field: %s (valid: username, password, category, url, notes, service)", field)
	}
}

// initVaultAndStorage initializes vault service and returns both vault and storage services
// This pattern is common across backup commands to avoid code duplication
func initVaultAndStorage(vaultPath string) (*vault.VaultService, error) {
	vaultService, err := vault.New(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize vault service: %w", err)
	}
	return vaultService, nil
}

// logVerbose logs a message to stderr if verbose mode is enabled
// Standardizes verbose logging format across all commands
func logVerbose(verbose bool, format string, args ...interface{}) {
	if verbose {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "[VERBOSE] %s\n", msg)
	}
}

// formatAge formats a duration as human-readable age (without "ago" suffix)
// Used for backup age display where context makes "ago" redundant
// This is a variant of formatRelativeTime for backup-specific display
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	if days < 7 {
		return fmt.Sprintf("%d days", days)
	}
	weeks := days / 7
	if weeks == 1 {
		return "1 week"
	}
	if weeks < 4 {
		return fmt.Sprintf("%d weeks", weeks)
	}
	months := days / 30
	if months == 1 {
		return "1 month"
	}
	return fmt.Sprintf("%d months", months)
}

// formatSize formats bytes as human-readable size (B, KB, MB, GB, etc.)
// Used for backup file size display across backup commands
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// unlockVault attempts to unlock the vault with keychain or prompts for password
func unlockVault(vaultService *vault.VaultService) error {
	// Try to unlock with keychain (if enabled and available)
	// This attempts keyring.Get() which doesn't require GUI authorization on macOS
	if err := vaultService.UnlockWithKeychain(); err == nil {
		if IsVerbose() {
			fmt.Fprintln(os.Stderr, "🔓 Unlocked vault using keychain")
		}
		return nil
	}

	// Prompt for master password if keychain fails or is unavailable
	fmt.Fprint(os.Stderr, "Master password: ")
	password, err := readPassword()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	fmt.Fprintln(os.Stderr) // newline after password input

	if err := vaultService.Unlock(password); err != nil {
		return fmt.Errorf("failed to unlock vault: %w", err)
	}

	return nil
}

// unlockVaultWithSync pulls from the remote and unlocks, overlapping the network
// pull with the master-password prompt to hide the pre-unlock probe latency (#103,
// Tier 1). It replaces the previous sequential `syncPullBeforeUnlock(vs)` +
// `unlockVault(vs)` pair at command entry points.
//
// Ordering constraint: Unlock reads and decrypts vault.enc in one step, and a pull
// replaces that file — so the decrypt must run strictly AFTER the pull finishes.
// Only work that does not touch vault.enc may overlap the pull:
//   - the master-password prompt touches stdin only → safe to overlap (Tier 1);
//   - keychain unlock decrypts vault.enc, but its expensive PBKDF2 key DERIVATION
//     touches only the password + pre-read key params, so it overlaps the pull and
//     we decrypt after the join (Tier 2).
func unlockVaultWithSync(vaultService *vault.VaultService) error {
	defer timing.Track("unlockVaultWithSync (wall)")()
	// No pull to overlap (sync disabled or --offline): today's exact behavior.
	if IsOffline() || !vaultService.IsSyncEnabled() {
		syncPullBeforeUnlock(vaultService) // no-op in these cases
		return unlockVault(vaultService)
	}

	// Keychain path (Tier 2): retrieving the keychain password reads metadata + the
	// OS keyring but never decrypts vault.enc. Read the key-derivation params first
	// (cheap, pre-pull), then overlap the expensive PBKDF2 derivation with the pull,
	// join, and unlock with the prepared key (which re-derives if the pull brought a
	// re-keyed vault).
	if password, err := vaultService.RetrieveKeychainPassword(); err == nil {
		defer crypto.ClearBytes(password)
		return unlockKeychainOverlappingPull(vaultService, password)
	}

	// Password path: run the pull concurrently with the prompt, then join before
	// decrypting against the now-current file.
	pullDone := make(chan struct{})
	var pullErr error
	go func() {
		defer close(pullDone)
		pullErr = vaultService.SyncPull()
	}()

	if IsVerbose() {
		// Full line (terminated by newline) so it cannot corrupt the prompt below.
		fmt.Fprintln(os.Stderr, "🔄 Checking remote for vault changes...")
	}
	// No transient "Checking remote..." indicator here: it would garble the prompt
	// line. The prompt itself signals that work is in flight; the pull runs quietly
	// in the background and any warnings are re-surfaced cleanly after the join.
	fmt.Fprint(os.Stderr, "Master password: ")
	password, readErr := readPassword()
	defer crypto.ClearBytes(password)
	fmt.Fprintln(os.Stderr) // newline after password input

	<-pullDone // join on ALL paths before returning, so rclone is never orphaned

	if readErr != nil {
		return fmt.Errorf("failed to read password: %w", readErr)
	}

	// Re-surface a sync conflict (or pull error) cleanly below the prompt. SyncPull
	// may have printed its own warning mid-prompt (garbled), and read commands never
	// re-echo it (only writes do, via SyncPush) — so a failed signal would be lost.
	if vaultService.SyncConflictDetected() {
		fmt.Fprintln(os.Stderr, "Warning: sync conflict — local and remote vault both changed. Use `pass-cli sync resolve` to choose which version to keep.")
	} else if pullErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: sync pull failed: %v\n", pullErr)
	}

	if err := vaultService.Unlock(password); err != nil {
		return fmt.Errorf("failed to unlock vault: %w", err)
	}
	return nil
}

// unlockKeychainOverlappingPull runs the keychain unlock (#103 Tier 2): it reads
// the vault's key-derivation params, starts the sync pull in a goroutine, derives
// the data key (the expensive PBKDF2 step) on this goroutine so it overlaps the
// pull, joins, then unlocks with the prepared key — which itself falls back to a
// full password unlock if the pull brought a re-keyed vault. The caller owns
// (and clears) password.
func unlockKeychainOverlappingPull(vaultService *vault.VaultService, password []byte) error {
	prep, prepErr := vaultService.PrepareUnlock()
	if prepErr != nil {
		// Couldn't read key params → fall back to a sequential keychain unlock.
		syncPullBeforeUnlock(vaultService)
		if err := vaultService.Unlock(append([]byte(nil), password...)); err != nil {
			return fmt.Errorf("failed to unlock vault: %w", err)
		}
		if IsVerbose() {
			fmt.Fprintln(os.Stderr, "🔓 Unlocked vault using keychain")
		}
		return nil
	}

	pullDone := make(chan struct{})
	go func() {
		defer close(pullDone)
		_ = vaultService.SyncPull()
	}()

	// A transient indicator is safe here — no password prompt to corrupt.
	if IsVerbose() {
		fmt.Fprintln(os.Stderr, "🔄 Checking remote for vault changes...")
	} else {
		fmt.Fprint(os.Stderr, "Checking remote...")
	}
	dataKey, deriveErr := prep.DeriveDataKey(password) // PBKDF2 overlaps the pull
	<-pullDone                                         // join before any decrypt
	if !IsVerbose() {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}

	if vaultService.SyncConflictDetected() {
		fmt.Fprintln(os.Stderr, "Warning: sync conflict — local and remote vault both changed. Use `pass-cli sync resolve` to choose which version to keep.")
	}

	if deriveErr != nil {
		// Derivation failed (e.g. malformed header) → full password unlock.
		if err := vaultService.Unlock(append([]byte(nil), password...)); err != nil {
			return fmt.Errorf("failed to unlock vault: %w", err)
		}
	} else if err := vaultService.UnlockWithPreparedKey(prep, dataKey, append([]byte(nil), password...)); err != nil {
		return err
	}
	if IsVerbose() {
		fmt.Fprintln(os.Stderr, "🔓 Unlocked vault using keychain")
	}
	return nil
}

// syncPullBeforeUnlock performs a smart sync pull before vault unlock.
// This ensures we have the latest version from remote before reading.
//
// When --offline is set, the pull is skipped entirely (no network either
// direction — see syncPushAfterCommand for the matching push skip).
//
// When sync is enabled and not offline, the network probe always runs
// (CheckRemoteMetadata is a round-trip; only the file transfer is conditional),
// so the user gets feedback that the command is hitting the network:
//   - verbose: a descriptive message on stderr
//   - otherwise: a transient indicator on stderr, cleared when the probe returns
//
// Nothing is ever written to stdout (it must stay byte-clean for pipes), and
// nothing is written at all when sync is disabled or --offline is set.
func syncPullBeforeUnlock(vaultService *vault.VaultService) {
	if IsOffline() {
		return
	}
	if !vaultService.IsSyncEnabled() {
		return
	}

	if IsVerbose() {
		fmt.Fprintln(os.Stderr, "🔄 Checking remote for vault changes...")
		if err := vaultService.SyncPull(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: sync pull failed: %v\n", err)
		}
		return
	}

	// Transient progress indicator on stderr, cleared with carriage-return +
	// ANSI erase-to-end-of-line (same technique as syncPushAfterCommand).
	fmt.Fprint(os.Stderr, "Checking remote...")
	err := vaultService.SyncPull()
	fmt.Fprint(os.Stderr, "\r\033[K")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: sync pull failed: %v\n", err)
	}
}

// syncPushAfterCommand performs a smart sync push after a command completes.
// This ensures local changes are pushed to remote once per command, not per-save.
// Shows "Syncing... done" feedback on stderr when a push actually occurs.
//
// When --offline is set, the push is skipped entirely. This is a correctness
// requirement, not just an optimization: SmartPush has no independent
// remote-conflict check — its only safety is that SmartPull ran first. If
// --offline skipped the pull but still pushed, a write could blind-overwrite a
// newer remote (silent cross-device data loss). So --offline means fully local.
func syncPushAfterCommand(vaultService *vault.VaultService) {
	if IsOffline() {
		return
	}
	if !vaultService.IsSyncEnabled() {
		return
	}
	fmt.Fprint(os.Stderr, "Syncing...")
	if vaultService.SyncPush() {
		fmt.Fprintln(os.Stderr, " done")
	} else {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
}

// T031: displayMnemonic formats 24-word mnemonic as 4x6 grid
// Used during vault initialization to display recovery phrase
func displayMnemonic(mnemonic string) {
	words := strings.Fields(mnemonic)
	if len(words) != 24 {
		fmt.Printf("Invalid mnemonic: expected 24 words, got %d\n", len(words))
		return
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Recovery Phrase Setup")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Write down these 24 words in order:")
	fmt.Println()

	// Display in 4 columns x 6 rows
	for row := 0; row < 6; row++ {
		line := ""
		for col := 0; col < 4; col++ {
			idx := col*6 + row
			if idx < len(words) {
				line += fmt.Sprintf("%3d. %-12s ", idx+1, words[idx])
			}
		}
		fmt.Println(line)
	}

	fmt.Println()
	fmt.Println("⚠  WARNINGS:")
	fmt.Println("   • Anyone with this phrase can access your vault")
	fmt.Println("   • Store offline (write on paper, use a safe)")
	fmt.Println("   • Recovery requires 6 random words from this list")
	fmt.Println()
}

// T031: promptForWord prompts user to enter a word at a specific position
// Used during backup verification to test user wrote down mnemonic correctly
// Returns word entered by user (trimmed, lowercase), error
func promptForWord(position int) (string, error) {
	fmt.Printf("Enter word #%d: ", position+1)

	// Use readLine in test mode, otherwise read normally
	var word string
	if os.Getenv("PASS_CLI_TEST") == "1" {
		line, err := readLine()
		if err != nil {
			return "", err
		}
		word = line
	} else {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read word: %w", err)
		}
		word = line
	}

	// Trim whitespace and convert to lowercase
	word = strings.ToLower(strings.TrimSpace(word))

	return word, nil
}

// T031: promptYesNo prompts user for yes/no confirmation
// Returns true for yes (Y/y), false for no (N/n)
// Uses defaultValue if user presses enter without typing
func promptYesNo(prompt string, defaultYes bool) (bool, error) {
	// Add default indicator to prompt
	if defaultYes {
		fmt.Printf("%s (Y/n): ", prompt)
	} else {
		fmt.Printf("%s (y/N): ", prompt)
	}

	// Read response
	var response string
	if os.Getenv("PASS_CLI_TEST") == "1" {
		line, err := readLine()
		if err != nil {
			return false, err
		}
		response = line
	} else {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("failed to read response: %w", err)
		}
		response = line
	}

	response = strings.TrimSpace(strings.ToLower(response))

	// Handle empty response (use default)
	if response == "" {
		return defaultYes, nil
	}

	// Parse response
	if response == "y" || response == "yes" {
		return true, nil
	}
	if response == "n" || response == "no" {
		return false, nil
	}

	// Invalid response, use default
	return defaultYes, nil
}

// T042: promptForWordWithValidation prompts for a word with BIP39 wordlist validation
// Allows retry on invalid word input
// Returns validated word (lowercase, trimmed), error
func promptForWordWithValidation(position int) (string, error) {
	const maxAttempts = 3

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Prompt for word
		word, err := promptForWord(position)
		if err != nil {
			return "", err
		}

		// Validate word is in BIP39 wordlist
		if !recovery.ValidateWord(word) {
			if attempt < maxAttempts {
				fmt.Printf("✗ Invalid word. Not in BIP39 wordlist. Try again (%d/%d)\n", attempt, maxAttempts)
				continue
			}
			return "", fmt.Errorf("invalid word after %d attempts", maxAttempts)
		}

		// Word is valid
		return word, nil
	}

	return "", fmt.Errorf("failed to read valid word after %d attempts", maxAttempts)
}
