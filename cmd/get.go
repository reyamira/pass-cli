package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/vault"
)

var (
	getQuiet       bool
	getField       string
	getNoClipboard bool
	getMasked      bool
	getTOTP        bool   // Output TOTP code instead of password
	getTOTPQR      bool   // Display TOTP QR code in terminal
	getTOTPQRFile  string // Export TOTP QR code to file
)

var getCmd = &cobra.Command{
	Use:     "get <service>",
	GroupID: "credentials",
	Short:   "Retrieve a credential from the vault",
	Long: `Get retrieves a credential from your vault and copies the password to clipboard.

By default, the password is copied to the clipboard and credential details
are displayed. Use flags to customize the output:

  --quiet      Output only the requested value (for scripts)
  --field      Extract a specific field (username, password, category, url, notes, service)
  --no-clipboard  Skip copying to clipboard
  --masked     Display password as asterisks (default shows full password)
  --totp       Output TOTP code instead of password (requires TOTP to be configured)
  --totp-qr    Display TOTP QR code in terminal (for adding to another device)
  --totp-qr-file  Export TOTP QR code to a PNG file

Automatic usage tracking records where credentials are accessed based on
your current working directory.`,
	Example: `  # Get credential with clipboard copy
  pass-cli get github

  # Get for scripts (outputs only password)
  pass-cli get github --quiet

  # Get specific field for scripts
  pass-cli get github --field username --quiet

  # Get without clipboard
  pass-cli get github --no-clipboard

  # Get with masked password display
  pass-cli get github --masked

  # Get TOTP code
  pass-cli get github --totp

  # Get TOTP code for scripts
  pass-cli get github --totp --quiet

  # Display TOTP QR code in terminal (to add to another device)
  pass-cli get github --totp-qr

  # Export TOTP QR code to a PNG file
  pass-cli get github --totp-qr-file totp-github.png`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func init() {
	rootCmd.AddCommand(getCmd)
	getCmd.Flags().BoolVarP(&getQuiet, "quiet", "q", false, "output only the requested value (script-friendly)")
	getCmd.Flags().StringVarP(&getField, "field", "f", "password", "field to extract (username, password, category, url, notes, service)")
	getCmd.Flags().BoolVar(&getNoClipboard, "no-clipboard", false, "do not copy to clipboard")
	getCmd.Flags().BoolVar(&getMasked, "masked", false, "display password as asterisks")
	getCmd.Flags().BoolVar(&getTOTP, "totp", false, "output TOTP code instead of password")
	getCmd.Flags().BoolVar(&getTOTPQR, "totp-qr", false, "display TOTP QR code in terminal")
	getCmd.Flags().StringVar(&getTOTPQRFile, "totp-qr-file", "", "export TOTP QR code to PNG file")
}

func runGet(cmd *cobra.Command, args []string) error {
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

	// Smart sync pull before unlock to get latest version
	syncPullBeforeUnlock(vaultService)

	// Unlock vault
	if err := unlockVault(vaultService); err != nil {
		return err
	}
	defer vaultService.Lock()

	// Get credential (no automatic tracking)
	cred, err := vaultService.GetCredential(service, false)
	if err != nil {
		return fmt.Errorf("failed to get credential: %w", err)
	}

	// TOTP QR code display mode
	if getTOTPQR {
		return outputTOTPQRMode(cred, service)
	}

	// TOTP QR code file export mode
	if getTOTPQRFile != "" {
		return exportTOTPQRFile(cred, service, getTOTPQRFile)
	}

	// TOTP mode - output TOTP code
	if getTOTP {
		return outputTOTPMode(cred, vaultService, service)
	}

	// Quiet mode - output only requested field
	if getQuiet {
		return outputQuietMode(cred, vaultService, service)
	}

	// Normal mode - display credential details
	return outputNormalMode(cred, vaultService, service)
}

func outputQuietMode(cred *vault.Credential, vaultService *vault.VaultService, service string) error {
	// Shared field resolver keeps the valid-field list in sync with `exec`.
	value, fieldName, err := resolveCredentialField(cred, getField)
	if err != nil {
		return err
	}

	// Track field access
	if err := vaultService.RecordFieldAccess(service, fieldName); err != nil {
		// Log warning but don't fail the operation
		fmt.Fprintf(os.Stderr, "Warning: failed to track field access: %v\n", err)
	}

	fmt.Println(value)
	return nil
}

// outputTOTPMode generates and displays the TOTP code
func outputTOTPMode(cred *vault.Credential, vaultService *vault.VaultService, service string) error {
	// Check time sync in background (don't block code generation)
	timeSyncChan := make(chan vault.TimeSyncResult, 1)
	go func() {
		timeSyncChan <- vault.CheckTimeSync()
	}()

	// Generate TOTP code with audit logging
	code, remaining, err := vaultService.GetTOTPCode(service)
	if err != nil {
		return fmt.Errorf("failed to generate TOTP code: %w", err)
	}

	// Track TOTP access for usage statistics
	if err := vaultService.RecordFieldAccess(service, "totp"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to track TOTP access: %v\n", err)
	}

	// Quiet mode - just output the code
	if getQuiet {
		fmt.Println(code)
		return nil
	}

	// Check for time sync warning (non-blocking, with short timeout)
	select {
	case result := <-timeSyncChan:
		if warning := vault.FormatTimeSyncWarning(result); warning != "" {
			fmt.Fprintln(os.Stderr, warning)
			fmt.Fprintln(os.Stderr)
		}
	case <-time.After(100 * time.Millisecond):
		// Don't wait too long - time check is best-effort
	}

	// Normal mode - show code with countdown
	fmt.Printf("🔐 TOTP Code: %s\n", code)
	fmt.Printf("⏱  Valid for: %ds\n", remaining)

	// Show progress bar
	period := 30
	if cred.TOTPPeriod > 0 {
		period = cred.TOTPPeriod
	}
	progress := float64(remaining) / float64(period)
	barWidth := 20
	filled := int(progress * float64(barWidth))
	empty := barWidth - filled
	fmt.Printf("   [%s%s]\n", strings.Repeat("█", filled), strings.Repeat("░", empty))

	// Copy to clipboard unless disabled
	if !getNoClipboard {
		if err := clipboard.WriteAll(code); err != nil {
			fmt.Fprintf(os.Stderr, "\n⚠️  Warning: failed to copy to clipboard: %v\n", err)
		} else {
			fmt.Println("\n✅ TOTP code copied to clipboard!")

			// Schedule clipboard clear in background (based on remaining validity)
			go func() {
				time.Sleep(time.Duration(remaining) * time.Second)
				// Only clear if the clipboard still contains our code
				if current, err := clipboard.ReadAll(); err == nil && current == code {
					_ = clipboard.WriteAll("")
					if IsVerbose() {
						fmt.Fprintln(os.Stderr, "🧹 Clipboard cleared")
					}
				}
			}()
		}
	}

	return nil
}

// outputTOTPQRMode displays the TOTP QR code in the terminal
func outputTOTPQRMode(cred *vault.Credential, service string) error {
	if !cred.HasTOTP() {
		return fmt.Errorf("no TOTP configured for credential: %s", service)
	}

	fmt.Printf("🔐 TOTP QR Code for: %s\n", service)
	if cred.TOTPIssuer != "" {
		fmt.Printf("   Issuer: %s\n", cred.TOTPIssuer)
	}
	fmt.Println()
	fmt.Println("Scan this QR code with your authenticator app:")
	fmt.Println()

	if err := cred.DisplayQRCode(os.Stdout); err != nil {
		return fmt.Errorf("failed to display QR code: %w", err)
	}

	fmt.Println()
	fmt.Println("⚠️  Keep this QR code private - it contains your TOTP secret!")

	return nil
}

// exportTOTPQRFile exports the TOTP QR code to a PNG file
func exportTOTPQRFile(cred *vault.Credential, service string, filename string) error {
	if !cred.HasTOTP() {
		return fmt.Errorf("no TOTP configured for credential: %s", service)
	}

	// Default size of 256x256 pixels
	size := 256

	if err := cred.ExportQRCode(filename, size); err != nil {
		return fmt.Errorf("failed to export QR code: %w", err)
	}

	fmt.Printf("✅ TOTP QR code exported to: %s\n", filename)
	fmt.Printf("   Service: %s\n", service)
	if cred.TOTPIssuer != "" {
		fmt.Printf("   Issuer: %s\n", cred.TOTPIssuer)
	}
	fmt.Println()
	fmt.Println("⚠️  Keep this file private - it contains your TOTP secret!")

	return nil
}

func outputNormalMode(cred *vault.Credential, vaultService *vault.VaultService, service string) error {
	// Display credential details
	fmt.Printf("📝 Service: %s\n", cred.Service)

	if cred.Username != "" {
		fmt.Printf("👤 Username: %s\n", cred.Username)
	}

	// Display password (masked or full)
	if getMasked {
		fmt.Printf("🔑 Password: %s\n", strings.Repeat("*", len(cred.Password)))
	} else {
		// T020d: Convert []byte to string for display
		fmt.Printf("🔑 Password: %s\n", string(cred.Password))
	}

	if cred.Category != "" {
		fmt.Printf("🏷️ Category: %s\n", cred.Category)
	}

	if cred.URL != "" {
		fmt.Printf("🔗 URL: %s\n", cred.URL)
	}

	if cred.Notes != "" {
		fmt.Printf("📋 Notes: %s\n", cred.Notes)
	}

	// Display TOTP status if configured
	if cred.HasTOTP() {
		issuer := cred.TOTPIssuer
		if issuer == "" {
			issuer = "configured"
		}
		fmt.Printf("🔐 TOTP: %s (use --totp to get code)\n", issuer)
	}

	// Display timestamps
	fmt.Printf("📅 Created: %s\n", cred.CreatedAt.Format("2006-01-02 15:04:05"))
	if !cred.UpdatedAt.Equal(cred.CreatedAt) {
		fmt.Printf("📅 Updated: %s\n", cred.UpdatedAt.Format("2006-01-02 15:04:05"))
	}

	// Track field access for displayed credential (normal mode shows password + username)
	if cred.Username != "" {
		if err := vaultService.RecordFieldAccess(service, "username"); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to track username access: %v\n", err)
		}
	}
	if err := vaultService.RecordFieldAccess(service, "password"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to track password access: %v\n", err)
	}

	// Copy to clipboard unless disabled
	if !getNoClipboard {
		// T020g: Convert []byte to string for clipboard, then immediately zero the byte slice
		passwordStr := string(cred.Password)

		if err := clipboard.WriteAll(passwordStr); err != nil {
			fmt.Fprintf(os.Stderr, "\n⚠️  Warning: failed to copy to clipboard: %v\n", err)
		} else {

			// T020g: Zero the password bytes immediately after clipboard write
			// Note: This only zeros the source []byte in cred, not the string copy
			// The string copy is necessary for clipboard API and will be GC'd
			for i := range cred.Password {
				cred.Password[i] = 0
			}

			fmt.Println("\n✅ Password copied to clipboard!")

			// Schedule clipboard clear in background (5 seconds)
			go func() {
				time.Sleep(5 * time.Second)
				// Only clear if the clipboard still contains our password
				if current, err := clipboard.ReadAll(); err == nil && current == passwordStr {
					_ = clipboard.WriteAll("")
					if IsVerbose() {
						fmt.Fprintln(os.Stderr, "🧹 Clipboard cleared")
					}
				}
			}()
		}
	}

	return nil
}
