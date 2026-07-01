package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/crypto"
	"github.com/arimxyer/pass-cli/internal/storage"
	"github.com/arimxyer/pass-cli/internal/vault"
)

var (
	previewFile    string
	previewVerbose bool
)

var vaultBackupPreviewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Preview credentials inside a backup file",
	Long: `Preview the credentials stored inside a backup file without restoring it.

This command decrypts the backup in-memory to show which credentials it contains.
It does NOT modify any files.

Important: You must provide the password that was active when the backup was created.
If you changed your password since the backup, use the OLD password.`,
	Example: `  # Preview credentials in a specific backup file
  pass-cli vault backup preview --file vault.enc.20241210-143022.manual.backup

  # Preview with verbose output (shows more details)
  pass-cli vault backup preview --file vault.enc.backup --verbose`,
	Args: cobra.NoArgs,
	RunE: runVaultBackupPreview,
}

func init() {
	vaultBackupCmd.AddCommand(vaultBackupPreviewCmd)
	vaultBackupPreviewCmd.Flags().StringVar(&previewFile, "file", "", "backup file to preview (required)")
	vaultBackupPreviewCmd.Flags().BoolVarP(&previewVerbose, "verbose", "v", false, "show detailed credential information")
	_ = vaultBackupPreviewCmd.MarkFlagRequired("file")
}

func runVaultBackupPreview(cmd *cobra.Command, args []string) error {
	// Validate file exists
	info, err := os.Stat(previewFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("backup file not found: %s", previewFile)
		}
		return fmt.Errorf("failed to access backup file: %w", err)
	}

	logVerbose(previewVerbose, "Backup file: %s", previewFile)
	logVerbose(previewVerbose, "File size: %s", formatSize(info.Size()))

	// Prompt for password
	fmt.Printf("Enter the backup's master password: ")
	password, err := readPassword()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	defer crypto.ClearBytes(password)
	fmt.Println() // newline after password input

	logVerbose(previewVerbose, "Attempting to decrypt backup...")

	// Create temporary storage service pointing to the backup file
	cryptoSvc := crypto.NewCryptoService()
	storageSvc, err := storage.NewStorageService(cryptoSvc, previewFile)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Attempt to decrypt the backup
	data, err := storageSvc.LoadVault(string(password))
	if err != nil {
		// Check for common error patterns
		errStr := err.Error()
		if strings.Contains(errStr, "cipher: message authentication failed") ||
			strings.Contains(errStr, "failed to decrypt") {
			return fmt.Errorf("decryption failed - incorrect password: make sure you're using the password that was active when this backup was created, if you've changed your password since then use the OLD password")
		}
		return fmt.Errorf("failed to decrypt backup: %w", err)
	}

	logVerbose(previewVerbose, "Decryption successful")
	logVerbose(previewVerbose, "Parsing credential data...")

	// Parse vault data
	var vaultData vault.VaultData
	if err := json.Unmarshal(data, &vaultData); err != nil {
		return fmt.Errorf("failed to parse vault data: %w", err)
	}

	// Display results
	credCount := len(vaultData.Credentials)
	if credCount == 0 {
		fmt.Printf("Backup is valid but contains no credentials.\n")
		return nil
	}

	fmt.Printf("Found %d credential(s) in backup:\n\n", credCount)

	// Sort service names alphabetically (case-insensitive)
	services := make([]string, 0, credCount)
	for service := range vaultData.Credentials {
		services = append(services, service)
	}
	sort.Slice(services, func(i, j int) bool { return lessFold(services[i], services[j]) })

	if previewVerbose {
		// Verbose: show table with more details
		var builder strings.Builder
		table := tablewriter.NewWriter(&builder)
		table.Header([]string{"#", "Service", "Username", "Category", "Created"})

		for i, service := range services {
			cred := vaultData.Credentials[service]
			category := cred.Category
			if category == "" {
				category = "-"
			}
			created := cred.CreatedAt.Format("2006-01-02")
			_ = table.Append([]string{
				fmt.Sprintf("%d", i+1),
				service,
				cred.Username,
				category,
				created,
			})
		}

		_ = table.Render()
		fmt.Print(builder.String())
	} else {
		// Simple: just list service names
		for i, service := range services {
			fmt.Printf("  %d. %s\n", i+1, service)
		}
	}

	fmt.Printf("\nBackup file: %s\n", previewFile)
	fmt.Printf("Modified: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))

	return nil
}
