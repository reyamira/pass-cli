package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/security"
	"github.com/arimxyer/pass-cli/internal/vault"
)

var keychainStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Display keychain integration status",
	Long: `Display keychain integration status for the current vault, including keychain
availability, password storage status, and backend name.

This is a read-only operation that does not require unlocking the vault.`,
	Example: `  # Check keychain status for default vault
  pass-cli keychain status

  # For custom vault location, configure vault_path in ~/.pass-cli/config.yml`,
	RunE: runKeychainStatus,
}

func init() {
	keychainCmd.AddCommand(keychainStatusCmd)
}

func runKeychainStatus(cmd *cobra.Command, args []string) error {
	vaultPath := GetVaultPath()

	vaultService, err := vault.New(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to create vault service at %s: %w", vaultPath, err)
	}

	// T036: Load metadata to check audit and keychain enabled status
	meta, err := vault.LoadMetadata(vaultPath)
	if err != nil && !os.IsNotExist(err) {
		// Metadata exists but failed to load - warn but continue
		fmt.Fprintf(os.Stderr, "Warning: Failed to load metadata: %v\n", err)
	}

	// T036: Initialize audit if enabled
	if meta != nil && meta.AuditEnabled {
		vaultDir := filepath.Dir(vaultPath)
		auditLogPath := filepath.Join(vaultDir, "audit.log")
		// Use directory name as VaultID for consistency with init command (getVaultID)
		vaultID := filepath.Base(vaultDir)
		if err := vaultService.EnableAudit(auditLogPath, vaultID); err != nil {
			// Best effort - continue even if audit init fails
			fmt.Fprintf(os.Stderr, "Warning: Failed to initialize audit: %v\n", err)
		}
	}

	status := vaultService.GetKeychainStatus()

	// Display status
	fmt.Printf("Keychain Status for %s:\n\n", vaultPath)

	if status.Available {
		// Keychain is available
		fmt.Printf("✓ System Keychain:        Available (%s)\n", status.BackendName)
		if status.PasswordStored {
			fmt.Printf("✓ Password Stored:        Yes\n")
			fmt.Printf("✓ Backend:                %s\n", getBackendImplementation())

			// T036: Display vault configuration and consistency check
			if meta != nil && meta.KeychainEnabled {
				fmt.Printf("✓ Vault Configuration:    Keychain enabled\n\n")
				fmt.Println("✓ Keychain integration is properly configured.")
				fmt.Println("Your vault password is securely stored in the system keychain.")
				fmt.Println("Future commands will not prompt for password.")
			} else if meta != nil && !meta.KeychainEnabled {
				// T036: Consistency check - password in keychain but not enabled in metadata
				fmt.Printf("⚠ Vault Configuration:    Keychain not enabled\n\n")
				fmt.Println("⚠ Inconsistency detected: Password is stored in keychain, but metadata indicates keychain is not enabled.")
				fmt.Println("This may happen if keychain was manually configured outside of pass-cli commands.")
			} else {
				// No metadata - legacy vault
				fmt.Println()
				fmt.Println("Your vault password is securely stored in the system keychain.")
				fmt.Println("Future commands will not prompt for password.")
			}
		} else {
			fmt.Printf("✗ Password Stored:        No\n")

			// T036: Display vault configuration and consistency check
			if meta != nil && meta.KeychainEnabled {
				// T036: Consistency check - metadata says enabled but no password in keychain
				fmt.Printf("⚠ Vault Configuration:    Keychain enabled\n\n")
				fmt.Println("⚠ Inconsistency detected: Metadata indicates keychain is enabled, but no password is stored in the keychain.")
				fmt.Println("Run 'pass-cli keychain enable' to fix this issue.")
			} else if meta != nil && !meta.KeychainEnabled {
				fmt.Printf("✓ Vault Configuration:    Keychain not enabled\n\n")
				fmt.Println("The system keychain is available but no password is stored for this vault.")
				fmt.Println("Suggestion: Enable keychain integration with 'pass-cli keychain enable'")
			} else {
				// No metadata - legacy vault
				fmt.Println()
				fmt.Println("The system keychain is available but no password is stored for this vault.")
				fmt.Println("Suggestion: Enable keychain integration with 'pass-cli keychain enable'")
			}
		}
	} else {
		// Keychain is not available
		fmt.Printf("✗ System Keychain:        Not available on this platform\n")
		fmt.Printf("✗ Password Stored:        N/A\n")

		// T036: Display vault configuration
		if meta != nil && meta.KeychainEnabled {
			fmt.Printf("⚠ Vault Configuration:    Keychain enabled\n\n")
			fmt.Println("⚠ Warning: Metadata indicates keychain is enabled, but system keychain is not available.")
			fmt.Println("You will be prompted for password on each command.")
		} else if meta != nil && !meta.KeychainEnabled {
			fmt.Printf("✓ Vault Configuration:    Keychain not enabled\n\n")
			fmt.Println("System keychain is not accessible. You will be prompted for password on each command.")
			fmt.Println("See documentation for keychain setup: https://github.com/reyamira/pass-cli/blob/main/docs/GETTING_STARTED.md#keychain-integration")
		} else {
			// No metadata - legacy vault
			fmt.Println()
			fmt.Println("System keychain is not accessible. You will be prompted for password on each command.")
			fmt.Println("See documentation for keychain setup: https://github.com/reyamira/pass-cli/blob/main/docs/GETTING_STARTED.md#keychain-integration")
		}
	}

	// T036: Write audit log entry if audit enabled (FR-013)
	if meta != nil && meta.AuditEnabled {
		vaultService.LogAudit(security.EventKeychainStatus, security.OutcomeSuccess, vaultPath)
	}

	return nil
}

// T022: getKeychainBackendName already defined in cmd/helpers.go:78

// Get backend implementation details (for display purposes)
func getBackendImplementation() string {
	switch runtime.GOOS {
	case "windows":
		return "wincred"
	case "darwin":
		return "keychain"
	case "linux":
		return "gnome-keyring/kwallet"
	default:
		return "unknown"
	}
}
