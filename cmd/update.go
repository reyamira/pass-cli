package cmd

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/vault"
)

var (
	updateUsername         string
	updatePassword         string
	updateNotes            string
	updateCategory         string
	updateURL              string
	updateForce            bool
	clearCategory          bool
	clearURL               bool
	clearNotes             bool
	updateGeneratePassword bool
	updateGenLength        int
	updateTOTPURI          string // TOTP otpauth:// URI
	clearTOTP              bool   // Clear TOTP configuration
)

var updateCmd = &cobra.Command{
	Use:     "update <service>",
	GroupID: "credentials",
	Short:   "Update an existing credential",
	Long: `Update modifies an existing credential in your vault.

You can selectively update individual fields (username, password, category, url, notes) without
affecting the others. Empty values mean "don't change".

To explicitly clear optional fields (category, url, notes, totp) to empty, use the --clear-* flags.
These flags take precedence over corresponding value flags.

Use --generate to auto-generate a new secure password (password rotation). The generated
password will be copied to clipboard automatically.

Use --totp-uri to add or update TOTP/2FA configuration for the credential.
Use --clear-totp to remove TOTP configuration.

By default, you'll see a usage warning if the credential has been accessed before,
showing where and when it was last used. Use --force to skip the confirmation.`,
	Example: `  # Update password only (interactive prompt)
  pass-cli update github

  # Update username only
  pass-cli update github --username new-user@example.com

  # Update password only
  pass-cli update github --password newpass123

  # Update category only
  pass-cli update github --category "Work"

  # Update URL only
  pass-cli update github --url "https://github.com"

  # Update notes
  pass-cli update github --notes "Updated account"

  # Clear category field
  pass-cli update github --clear-category

  # Clear URL field
  pass-cli update github --clear-url

  # Clear notes field
  pass-cli update github --clear-notes

  # Update multiple fields
  pass-cli update github -u user -p pass --notes "New info"

  # Generate new password (password rotation)
  pass-cli update github --generate

  # Generate new 32-character password
  pass-cli update github -g --gen-length 32

  # Add or update TOTP/2FA
  pass-cli update github --totp-uri "otpauth://totp/GitHub:user?secret=JBSWY3DPEHPK3PXP&issuer=GitHub"

  # Remove TOTP/2FA configuration
  pass-cli update github --clear-totp

  # Skip confirmation
  pass-cli update github --force`,
	Args: cobra.ExactArgs(1),
	RunE: runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().StringVarP(&updateUsername, "username", "u", "", "new username")
	updateCmd.Flags().StringVarP(&updatePassword, "password", "p", "", "new password")
	updateCmd.Flags().BoolVarP(&updateGeneratePassword, "generate", "g", false, "auto-generate a new secure password")
	updateCmd.Flags().IntVar(&updateGenLength, "gen-length", 20, "length of generated password (default: 20)")
	updateCmd.Flags().StringVar(&updateNotes, "notes", "", "new notes")
	updateCmd.Flags().StringVar(&updateCategory, "category", "", "new category")
	updateCmd.Flags().StringVar(&updateURL, "url", "", "new URL")
	updateCmd.Flags().BoolVar(&clearCategory, "clear-category", false, "clear category field to empty")
	updateCmd.Flags().BoolVar(&clearURL, "clear-url", false, "clear URL field to empty")
	updateCmd.Flags().BoolVar(&clearNotes, "clear-notes", false, "clear notes field to empty")
	updateCmd.Flags().StringVar(&updateTOTPURI, "totp-uri", "", "TOTP/2FA otpauth:// URI to add or update")
	updateCmd.Flags().BoolVar(&clearTOTP, "clear-totp", false, "remove TOTP/2FA configuration")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "skip confirmation prompt")

	// Mark --password and --generate as mutually exclusive
	updateCmd.MarkFlagsMutuallyExclusive("password", "generate")
	// Mark --totp-uri and --clear-totp as mutually exclusive
	updateCmd.MarkFlagsMutuallyExclusive("totp-uri", "clear-totp")
}

func runUpdate(cmd *cobra.Command, args []string) error {
	service := strings.TrimSpace(args[0])
	if service == "" {
		return fmt.Errorf("service name cannot be empty")
	}

	vaultPath := GetVaultPath()

	// Check if vault exists
	if _, err := os.Stat(vaultPath); os.IsNotExist(err) {
		return fmt.Errorf("vault not found at %s\nRun 'pass-cli init' to create a vault first", vaultPath)
	}

	// Create vault service
	vaultService, err := vault.New(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to create vault service at %s: %w", vaultPath, err)
	}

	// Pull from remote and unlock, overlapping the pull with the password prompt (#103).
	if err := unlockVaultWithSync(vaultService); err != nil {
		return err
	}
	defer vaultService.Lock()

	// Check if credential exists
	cred, err := vaultService.GetCredential(service, false)
	if err != nil {
		return fmt.Errorf("failed to get credential: %w", err)
	}

	// Handle password generation
	if updateGeneratePassword {
		generated, err := generatePasswordForUpdate(updateGenLength)
		if err != nil {
			return fmt.Errorf("failed to generate password: %w", err)
		}
		updatePassword = generated

		// Copy to clipboard
		if err := clipboard.WriteAll(updatePassword); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: failed to copy password to clipboard: %v\n", err)
		} else {
			fmt.Println("🔐 Generated new password (copied to clipboard)")
		}
	}

	// If no flags provided (including clear flags), prompt for what to update
	if updateUsername == "" && updatePassword == "" && updateNotes == "" && updateCategory == "" && updateURL == "" &&
		updateTOTPURI == "" && !clearCategory && !clearURL && !clearNotes && !clearTOTP && !updateGeneratePassword {
		fmt.Println("What would you like to update? (leave empty to keep current value)")
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)

		// Prompt for username
		fmt.Printf("Username [%s]: ", cred.Username)
		username, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read username: %w", err)
		}
		updateUsername = strings.TrimSpace(username)

		// Prompt for password
		fmt.Print("Password (hidden): ")
		password, err := readPassword()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println()
		updatePassword = string(password) // TODO: Remove string conversion in Phase 3 (T020d)

		// Prompt for category
		fmt.Printf("Category [%s]: ", cred.Category)
		category, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read category: %w", err)
		}
		updateCategory = strings.TrimSpace(category)

		// Prompt for URL
		fmt.Printf("URL [%s]: ", cred.URL)
		url, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read URL: %w", err)
		}
		updateURL = strings.TrimSpace(url)

		// Prompt for notes
		fmt.Printf("Notes [%s]: ", cred.Notes)
		notes, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read notes: %w", err)
		}
		updateNotes = strings.TrimSpace(notes)
	}

	// Check if anything is being updated
	if updateUsername == "" && updatePassword == "" && updateNotes == "" && updateCategory == "" && updateURL == "" &&
		updateTOTPURI == "" && !clearCategory && !clearURL && !clearNotes && !clearTOTP && !updateGeneratePassword {
		fmt.Println("No changes specified.")
		return nil
	}

	// Show usage warning if credential has been accessed
	stats, _ := vaultService.GetUsageStats(service)
	if len(stats) > 0 && !updateForce {
		fmt.Println("\n⚠️  Usage Warning:")

		totalCount := 0
		var lastAccessed string
		for _, record := range stats {
			totalCount += record.Count
			if lastAccessed == "" || record.Timestamp.After(cred.UpdatedAt) {
				lastAccessed = formatRelativeTime(record.Timestamp)
			}
		}

		fmt.Printf("   Used in %d location(s), last used %s\n", len(stats), lastAccessed)
		fmt.Printf("   Total access count: %d\n\n", totalCount)

		// Ask for confirmation
		fmt.Print("Continue with update? (y/N): ")
		var confirm string
		_, _ = fmt.Scanln(&confirm)
		confirm = strings.ToLower(strings.TrimSpace(confirm))

		if confirm != "y" && confirm != "yes" {
			fmt.Println("Update cancelled.")
			return nil
		}
	}

	// Perform update using UpdateOpts (only update non-empty fields)
	opts := vault.UpdateOpts{}
	if updateUsername != "" {
		opts.Username = &updateUsername
	}
	if updatePassword != "" {
		// T020d: Convert string to []byte for vault storage
		passwordBytes := []byte(updatePassword)
		opts.Password = &passwordBytes
	}

	// Handle notes: clear flag takes precedence
	if clearNotes {
		emptyNotes := ""
		opts.Notes = &emptyNotes
	} else if updateNotes != "" {
		opts.Notes = &updateNotes
	}

	// Handle category: clear flag takes precedence
	if clearCategory {
		emptyCategory := ""
		opts.Category = &emptyCategory
	} else if updateCategory != "" {
		opts.Category = &updateCategory
	}

	// Handle URL: clear flag takes precedence
	if clearURL {
		emptyURL := ""
		opts.URL = &emptyURL
	} else if updateURL != "" {
		opts.URL = &updateURL
	}

	// Handle TOTP: clear flag takes precedence
	if clearTOTP {
		opts.ClearTOTP = true
	} else if updateTOTPURI != "" {
		// Parse and validate TOTP URI
		totpConfig, err := vault.ParseTOTPURI(updateTOTPURI)
		if err != nil {
			return fmt.Errorf("invalid TOTP URI: %w", err)
		}
		opts.TOTPSecret = &totpConfig.Secret
		opts.TOTPAlgorithm = &totpConfig.Algorithm
		opts.TOTPDigits = &totpConfig.Digits
		opts.TOTPPeriod = &totpConfig.Period
		if totpConfig.Issuer != "" {
			opts.TOTPIssuer = &totpConfig.Issuer
		}
	}

	if err := vaultService.UpdateCredential(service, opts); err != nil {
		return fmt.Errorf("failed to update credential: %w", err)
	}

	// Success message
	fmt.Printf("✅ Credential updated successfully!\n")
	fmt.Printf("📝 Service: %s\n", service)

	if updateUsername != "" {
		fmt.Printf("👤 New username: %s\n", updateUsername)
	}
	if updatePassword != "" {
		fmt.Printf("🔑 Password updated\n")
	}
	if clearCategory {
		fmt.Printf("🏷️  Category cleared\n")
	} else if updateCategory != "" {
		fmt.Printf("🏷️  New category: %s\n", updateCategory)
	}
	if clearURL {
		fmt.Printf("🔗 URL cleared\n")
	} else if updateURL != "" {
		fmt.Printf("🔗 New URL: %s\n", updateURL)
	}
	if clearNotes {
		fmt.Printf("📋 Notes cleared\n")
	} else if updateNotes != "" {
		fmt.Printf("📋 New notes: %s\n", updateNotes)
	}
	if clearTOTP {
		fmt.Printf("🔐 TOTP cleared\n")
	} else if updateTOTPURI != "" {
		fmt.Printf("🔐 TOTP configured\n")
	}

	syncPushAfterCommand(vaultService)
	return nil
}

// generatePasswordForUpdate generates a cryptographically secure password
// Reuses the same logic as the generate command
func generatePasswordForUpdate(length int) (string, error) {
	// Validate length
	if length < 8 {
		return "", fmt.Errorf("password length must be at least 8 characters")
	}
	if length > 128 {
		return "", fmt.Errorf("password length cannot exceed 128 characters")
	}

	// Build character set (always include all types for security)
	const (
		lowerChars  = "abcdefghijklmnopqrstuvwxyz"
		upperChars  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		digitChars  = "0123456789"
		symbolChars = "!@#$%^&*()_+-=[]{}|;:,.<>?"
	)

	charset := lowerChars + upperChars + digitChars + symbolChars
	password := make([]byte, length)

	// Ensure at least one character from each required set
	requiredSets := []string{lowerChars, upperChars, digitChars, symbolChars}
	for i, reqSet := range requiredSets {
		if i >= length {
			break
		}
		setLen := big.NewInt(int64(len(reqSet)))
		randomIndex, err := rand.Int(rand.Reader, setLen)
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		password[i] = reqSet[randomIndex.Int64()]
	}

	// Fill remaining positions with random chars from full charset
	charsetLen := big.NewInt(int64(len(charset)))
	for i := len(requiredSets); i < length; i++ {
		randomIndex, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		password[i] = charset[randomIndex.Int64()]
	}

	// Shuffle password to avoid predictable positions
	for i := length - 1; i > 0; i-- {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		j := randomIndex.Int64()
		password[i], password[j] = password[j], password[i]
	}

	return string(password), nil
}
