package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arimxyer/pass-cli/internal/config"
	"github.com/arimxyer/pass-cli/internal/vault"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

var (
	cfgFile string
	verbose bool

	// Version information (set via ldflags during build)
	version = "dev"
	commit  = "none"
	date    = "unknown"

	rootCmd = &cobra.Command{
		Use:   "pass-cli",
		Short: "A secure CLI password and API key manager",
		Long: `Pass-CLI is a secure, cross-platform command-line password and API key manager
designed for developers. It provides local encrypted storage with optional system
keychain integration, allowing developers to securely manage credentials without
relying on cloud services.

Features:
  • AES-256-GCM encryption with PBKDF2 key derivation
  • Native OS keychain integration (Windows Credential Manager, macOS Keychain, Linux Secret Service)
  • Script-friendly output for CI/CD integration
  • Automatic usage tracking
  • Offline-first design with no cloud dependencies

Examples:
  # Initialize a new vault
  pass-cli init

  # Add a new credential
  pass-cli add github

  # Retrieve a credential
  pass-cli get github

  # List all credentials
  pass-cli list

For more information, visit: https://github.com/reyamira/pass-cli`,
		PersistentPreRunE: checkFirstRun,
		Run:               runRootCommand,
	}
)

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// NOTE: Config loading moved to PersistentPreRunE to ensure --config flag is parsed first
	// See issue #65: https://github.com/reyamira/pass-cli/issues/65

	// T037: Custom flag error handler for migration guidance
	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		// Check if error is about --vault flag
		if strings.Contains(err.Error(), "vault") && strings.Contains(err.Error(), "flag") {
			return fmt.Errorf(`the --vault flag has been removed

Instead, configure your vault location in the config file:
  1. Edit %s/.pass-cli/config.yml
  2. Add: vault_path: /your/custom/path/vault.enc
  3. Run your command without the --vault flag

For more details, see the migration guide:
  https://github.com/reyamira/pass-cli/blob/main/docs/MIGRATION.md

Original error: %w`, os.Getenv("HOME"), err)
		}
		// Return original error for other flag issues
		return err
	})

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.pass-cli/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Bind flags to viper
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

	// Configure command groups for better help organization
	rootCmd.AddGroup(
		&cobra.Group{ID: "vault", Title: "Vault Management:"},
		&cobra.Group{ID: "credentials", Title: "Credential Operations:"},
		&cobra.Group{ID: "security", Title: "Security & Integration:"},
		&cobra.Group{ID: "utilities", Title: "Utilities:"},
	)
}

// GetVaultPath returns the vault path from config or default
// Exits with error if config validation fails (FR-012)
func GetVaultPath() string {
	// Check if viper has vault_path set (from --config flag or default config)
	// This is necessary because config.Load() uses os.UserConfigDir() and doesn't respect --config flag
	var vaultPath string
	if viper.IsSet("vault_path") {
		vaultPath = viper.GetString("vault_path")
	} else {
		// Load config and check validation only if viper doesn't have it
		cfg, result := config.Load()

		// FR-012: Validate vault_path during config loading and report errors
		if !result.Valid {
			fmt.Fprintf(os.Stderr, "Configuration validation failed:\n")
			for _, err := range result.Errors {
				fmt.Fprintf(os.Stderr, "  - %s: %s\n", err.Field, err.Message)
			}
			fmt.Fprintf(os.Stderr, "\nPlease fix your configuration file and try again.\n")
			os.Exit(1)
		}

		if cfg.VaultPath != "" {
			vaultPath = cfg.VaultPath
		}
	}

	// If still no vault path, use default
	if vaultPath == "" {
		// Default vault path
		home, err := os.UserHomeDir()
		if err != nil {
			return ".pass-cli/vault.enc"
		}
		return filepath.Join(home, ".pass-cli", "vault.enc")
	}

	// Expand environment variables
	vaultPath = os.ExpandEnv(vaultPath)

	// Expand ~ prefix
	if strings.HasPrefix(vaultPath, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return vaultPath // Return as-is if home unknown
		}
		vaultPath = filepath.Join(home, vaultPath[1:])
	}

	// Convert relative to absolute path
	if !filepath.IsAbs(vaultPath) {
		home, err := os.UserHomeDir()
		if err == nil {
			vaultPath = filepath.Join(home, vaultPath)
		}
	}

	return vaultPath
}

// GetVaultPathWithSource returns the vault path and its source ("config" or "default")
// Exits with error if config validation fails (FR-012)
func GetVaultPathWithSource() (path string, source string) {
	// Check if viper has vault_path set (from --config flag or default config)
	// This is necessary because config.Load() uses os.UserConfigDir() and doesn't respect --config flag
	var vaultPath string
	var pathSource string

	if viper.IsSet("vault_path") {
		vaultPath = viper.GetString("vault_path")
		if vaultPath != "" {
			pathSource = "config"
		}
	} else {
		// Load config and check validation only if viper doesn't have it
		cfg, result := config.Load()

		// FR-012: Validate vault_path during config loading and report errors
		if !result.Valid {
			fmt.Fprintf(os.Stderr, "Configuration validation failed:\n")
			for _, err := range result.Errors {
				fmt.Fprintf(os.Stderr, "  - %s: %s\n", err.Field, err.Message)
			}
			fmt.Fprintf(os.Stderr, "\nPlease fix your configuration file and try again.\n")
			os.Exit(1)
		}

		if cfg.VaultPath != "" {
			vaultPath = cfg.VaultPath
			pathSource = "config"
		}
	}

	// If still no vault path, use default
	if vaultPath == "" {
		// Default vault path
		home, err := os.UserHomeDir()
		if err != nil {
			return ".pass-cli/vault.enc", "default"
		}
		vaultPath = filepath.Join(home, ".pass-cli", "vault.enc")
		pathSource = "default"
	}

	// Expand environment variables
	vaultPath = os.ExpandEnv(vaultPath)

	// Expand ~ prefix
	if strings.HasPrefix(vaultPath, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return vaultPath, pathSource // Return as-is if home unknown
		}
		vaultPath = filepath.Join(home, vaultPath[1:])
	}

	// Convert relative to absolute path
	if !filepath.IsAbs(vaultPath) {
		home, err := os.UserHomeDir()
		if err == nil {
			vaultPath = filepath.Join(home, vaultPath)
		}
	}

	return vaultPath, pathSource
}

// IsVerbose returns whether verbose mode is enabled
func IsVerbose() bool {
	return verbose || viper.GetBool("verbose")
}

// runRootCommand runs when pass-cli is invoked with no subcommand
// Launches TUI mode by default, with first-run detection
func runRootCommand(cmd *cobra.Command, args []string) {
	// Skip first-run check in test mode - show help instead
	if os.Getenv("PASS_CLI_TEST") == "1" {
		_ = cmd.Help()
		return
	}

	// Check if custom vault path is configured
	var customVaultPath string
	cfg, _ := config.Load()
	if cfg != nil && cfg.VaultPath != "" {
		customVaultPath = cfg.VaultPath
	}

	// Check for first-run scenario
	state := vault.DetectFirstRun("", customVaultPath)

	// If vault doesn't exist at default location, prompt for guided init
	if !state.VaultExists && !state.CustomVaultPath {
		// Check if running in TTY
		isTTY := term.IsTerminal(int(os.Stdin.Fd()))

		fmt.Println("\n👋 Welcome to Pass-CLI!")
		fmt.Println("\nIt looks like this is your first time using pass-cli.")

		// Run guided initialization
		if err := vault.RunGuidedInit(state.VaultPath, isTTY); err != nil {
			// If user declined or error, exit
			fmt.Println()
			return
		}
		// After successful init, continue to launch TUI
	}

	// Verify vault exists before launching TUI
	vaultPath := GetVaultPath()
	if _, err := os.Stat(vaultPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Vault not found at %s\n", vaultPath)
		fmt.Fprintf(os.Stderr, "Run 'pass-cli init' to create a new vault.\n")
		os.Exit(1)
	}

	// Launch TUI (same as `pass-cli tui`)
	runTUI(cmd, args)
}

// checkFirstRun detects first-run scenarios and triggers guided initialization
// T065: PersistentPreRunE hook for first-run detection
func checkFirstRun(cmd *cobra.Command, args []string) error {
	// Lightweight commands don't need config loading or first-run detection.
	// Skip early to improve startup time and avoid errors from malformed configs.
	switch cmd.Name() {
	case "version", "help":
		return nil
	}

	// Initialize config FIRST - this must happen after flags are parsed
	// so that --config flag is available. This fixes issue #65 where
	// custom config files were not being loaded properly.
	initConfig()

	// Skip first-run check in test mode
	if os.Getenv("PASS_CLI_TEST") == "1" {
		return nil
	}

	// Get custom vault path from config (now properly loaded)
	var customVaultPath string
	if viper.IsSet("vault_path") {
		customVaultPath = viper.GetString("vault_path")
	}

	// Detect first-run scenario with custom vault path if configured
	state := vault.DetectFirstRun(cmd.Name(), customVaultPath)

	// If guided init should be triggered
	if state.ShouldPrompt {
		// Check if running in TTY
		isTTY := term.IsTerminal(int(os.Stdin.Fd()))

		// Get actual vault path (flag or default)
		actualVaultPath := GetVaultPath()

		// Run guided initialization
		if err := vault.RunGuidedInit(actualVaultPath, isTTY); err != nil {
			return err
		}
	}

	return nil
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".pass-cli" (without extension).
		viper.AddConfigPath(home + "/.pass-cli")
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		if viper.GetBool("verbose") {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}
