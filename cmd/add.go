package cmd

import (
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
	addUsername         string
	addPassword         string
	addCategory         string
	addURL              string
	addNotes            string
	addGeneratePassword bool
	addGenLength        int
	addTOTPURI          string // TOTP otpauth:// URI
	addTOTP             bool   // Prompt for TOTP secret interactively
)

var addCmd = &cobra.Command{
	Use:     "add <service>",
	GroupID: "credentials",
	Short:   "Add a new credential to the vault",
	Long: `Add stores a new credential (username and password) for a service in your vault.

You will be prompted for the username and password. The password input will be
hidden for security. If you want to provide these values via flags, use:
  --username (-u) for the username
  --password (-p) for the password (not recommended for security)
  --generate (-g) to auto-generate a secure password
  --gen-length to specify generated password length (default: 20)
  --category (-c) for organizing credentials (e.g., 'Cloud', 'Databases')
  --url for the service URL (e.g., login page URL)
  --notes for additional information
  --totp-uri to add TOTP/2FA support with an otpauth:// URI
  --totp to be prompted for TOTP secret interactively

The service name should be descriptive and unique (e.g., "github", "aws-prod", "db-staging").`,
	Example: `  # Add a credential with prompts
  pass-cli add github

  # Add with username flag
  pass-cli add github --username user@example.com

  # Add with category and URL
  pass-cli add github -u user@example.com -c "Version Control" --url "https://github.com"

  # Add with notes
  pass-cli add github --notes "My GitHub account"

  # Add with auto-generated password
  pass-cli add github -u user@example.com --generate

  # Add with auto-generated 32-character password
  pass-cli add github -u user@example.com -g --gen-length 32

  # Add with all metadata fields
  pass-cli add github -u user@example.com -c "Version Control" --url "https://github.com" --notes "Work account"

  # Add with TOTP/2FA support
  pass-cli add github -u user@example.com --totp-uri "otpauth://totp/GitHub:user?secret=JBSWY3DPEHPK3PXP&issuer=GitHub"

  # Add with interactive TOTP prompt
  pass-cli add github -u user@example.com --totp`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVarP(&addUsername, "username", "u", "", "username for the credential")
	addCmd.Flags().StringVarP(&addPassword, "password", "p", "", "password for the credential (not recommended, use prompt instead)")
	addCmd.Flags().BoolVarP(&addGeneratePassword, "generate", "g", false, "auto-generate a secure password")
	addCmd.Flags().IntVar(&addGenLength, "gen-length", 20, "length of generated password (default: 20)")
	addCmd.Flags().StringVarP(&addCategory, "category", "c", "", "category for organizing credentials (e.g., 'Cloud', 'Databases')")
	addCmd.Flags().StringVar(&addURL, "url", "", "URL associated with the credential (e.g., login page)")
	addCmd.Flags().StringVar(&addNotes, "notes", "", "optional notes about the credential")
	addCmd.Flags().StringVar(&addTOTPURI, "totp-uri", "", "TOTP/2FA otpauth:// URI (from QR code or authenticator app)")
	addCmd.Flags().BoolVar(&addTOTP, "totp", false, "prompt for TOTP secret interactively")

	// Mark --password and --generate as mutually exclusive
	addCmd.MarkFlagsMutuallyExclusive("password", "generate")
	// Mark --totp-uri and --totp as mutually exclusive
	addCmd.MarkFlagsMutuallyExclusive("totp-uri", "totp")
}

func runAdd(cmd *cobra.Command, args []string) error {
	service := args[0]

	// Validate service name
	service = strings.TrimSpace(service)
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

	// Get username if not provided
	if addUsername == "" {
		fmt.Print("Username: ")
		if _, err := fmt.Scanln(&addUsername); err != nil {
			return fmt.Errorf("failed to read username: %w", err)
		}
		addUsername = strings.TrimSpace(addUsername)
	}

	// Get password if not provided
	if addPassword == "" {
		if addGeneratePassword {
			// Generate a secure password
			generated, err := generatePasswordForAdd(addGenLength)
			if err != nil {
				return fmt.Errorf("failed to generate password: %w", err)
			}
			addPassword = generated

			// Copy to clipboard
			if err := clipboard.WriteAll(addPassword); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  Warning: failed to copy password to clipboard: %v\n", err)
			} else {
				fmt.Println("🔐 Generated password (copied to clipboard)")
			}
		} else {
			// Prompt for password
			fmt.Print("Password: ")
			password, err := readPassword()
			if err != nil {
				return fmt.Errorf("failed to read password: %w", err)
			}
			fmt.Println()                  // newline after password input
			addPassword = string(password) // TODO: Remove string conversion in Phase 3 (T020d)
		}
	}

	// Validate password is not empty
	if addPassword == "" {
		return fmt.Errorf("password cannot be empty")
	}

	// T020d: Convert string password to []byte for vault
	passwordBytes := []byte(addPassword)

	// Add credential to vault with all metadata fields
	if err := vaultService.AddCredential(service, addUsername, passwordBytes, addCategory, addURL, addNotes); err != nil {
		return fmt.Errorf("failed to add credential: %w", err)
	}

	// Handle TOTP if provided
	var totpConfigured bool
	if addTOTPURI != "" || addTOTP {
		totpURI := addTOTPURI

		// If --totp flag is set, prompt for TOTP secret
		if addTOTP {
			fmt.Print("TOTP Secret (base32) or otpauth:// URI: ")
			var totpInput string
			if _, err := fmt.Scanln(&totpInput); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  Warning: failed to read TOTP input, skipping TOTP setup: %v\n", err)
			} else {
				totpURI = strings.TrimSpace(totpInput)
			}
		}

		if totpURI != "" {
			// Parse and validate TOTP
			totpConfig, err := vault.ParseTOTPURI(totpURI)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  Warning: invalid TOTP configuration: %v\n", err)
			} else {
				// Update credential with TOTP fields
				opts := vault.UpdateOpts{
					TOTPSecret:    &totpConfig.Secret,
					TOTPAlgorithm: &totpConfig.Algorithm,
					TOTPDigits:    &totpConfig.Digits,
					TOTPPeriod:    &totpConfig.Period,
				}
				if totpConfig.Issuer != "" {
					opts.TOTPIssuer = &totpConfig.Issuer
				}

				if err := vaultService.UpdateCredential(service, opts); err != nil {
					fmt.Fprintf(os.Stderr, "⚠️  Warning: failed to save TOTP configuration: %v\n", err)
				} else {
					totpConfigured = true
				}
			}
		}
	}

	// Success message
	fmt.Printf("✅ Credential added successfully!\n")
	fmt.Printf("📝 Service: %s\n", service)
	if addUsername != "" {
		fmt.Printf("👤 Username: %s\n", addUsername)
	}
	if addCategory != "" {
		fmt.Printf("🏷️  Category: %s\n", addCategory)
	}
	if addURL != "" {
		fmt.Printf("🔗 URL: %s\n", addURL)
	}
	if addNotes != "" {
		fmt.Printf("📋 Notes: %s\n", addNotes)
	}
	if totpConfigured {
		fmt.Printf("🔐 TOTP: configured\n")
	}

	syncPushAfterCommand(vaultService)
	return nil
}

// generatePasswordForAdd generates a cryptographically secure password
// Reuses the same logic as the generate command
func generatePasswordForAdd(length int) (string, error) {
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
