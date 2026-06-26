package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/vault"
)

var (
	deleteForce bool
)

var deleteCmd = &cobra.Command{
	Use:     "delete <service> [service...]",
	GroupID: "credentials",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete credentials from the vault",
	Long: `Delete removes one or more credentials from your vault.

By default, you'll see a usage warning if the credential has been accessed before,
showing where and when it was last used. This helps prevent accidental deletion
of actively-used credentials.

You can delete multiple credentials at once by providing multiple service names.
Use --force to skip all confirmation prompts (dangerous!).`,
	Example: `  # Delete a single credential
  pass-cli delete github

  # Delete multiple credentials
  pass-cli delete github gitlab bitbucket

  # Delete with alias
  pass-cli rm old-service

  # Force delete without confirmation (dangerous!)
  pass-cli delete github --force`,
	Args: cobra.MinimumNArgs(1),
	RunE: runDelete,
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "skip confirmation prompts")
}

func runDelete(cmd *cobra.Command, args []string) error {
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

	// Process each service to delete
	deleted := 0
	skipped := 0

	for _, service := range args {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}

		// Check if credential exists and get metadata
		cred, err := vaultService.GetCredential(service, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error: %s - %v\n", service, err)
			skipped++
			continue
		}

		// Show usage warning if credential has been accessed
		if !deleteForce {
			stats, _ := vaultService.GetUsageStats(service)
			if len(stats) > 0 {
				fmt.Printf("\n⚠️  Warning: Deleting '%s'\n", service)

				totalCount := 0
				var lastAccessed string
				for _, record := range stats {
					totalCount += record.Count
					if lastAccessed == "" || record.Timestamp.After(cred.UpdatedAt) {
						lastAccessed = formatRelativeTime(record.Timestamp)
					}
				}

				fmt.Printf("   Used in %d location(s), last used %s\n", len(stats), lastAccessed)
				fmt.Printf("   Total access count: %d\n", totalCount)
				fmt.Println()
			} else {
				fmt.Printf("\n🗑️  Deleting '%s' (never used)\n", service)
			}

			// Ask for confirmation
			fmt.Print("Confirm deletion? (y/N): ")
			var confirm string
			_, _ = fmt.Scanln(&confirm)
			confirm = strings.ToLower(strings.TrimSpace(confirm))

			if confirm != "y" && confirm != "yes" {
				fmt.Printf("⏭️  Skipped: %s\n", service)
				skipped++
				continue
			}
		}

		// Delete the credential
		if err := vaultService.DeleteCredential(service); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error deleting %s: %v\n", service, err)
			skipped++
			continue
		}

		fmt.Printf("✅ Deleted: %s\n", service)
		deleted++
	}

	// Summary
	fmt.Println()
	if deleted > 0 {
		fmt.Printf("Successfully deleted %d credential(s)\n", deleted)
	}
	if skipped > 0 {
		fmt.Printf("Skipped %d credential(s)\n", skipped)
	}

	syncPushAfterCommand(vaultService)
	return nil
}
