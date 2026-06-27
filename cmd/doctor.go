package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arimxyer/pass-cli/internal/config"
	"github.com/arimxyer/pass-cli/internal/health"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	doctorJSON    bool
	doctorQuiet   bool
	doctorVerbose bool
)

var doctorCmd = &cobra.Command{
	Use:     "doctor",
	GroupID: "utilities",
	Short:   "Check vault health and system configuration",
	Long: `Run comprehensive health checks on your pass-cli installation.

The doctor command verifies:
  • Binary version (checks for updates)
  • Vault file accessibility and permissions
  • Configuration file validity
  • Keychain integration status
  • Backup file status

Exit codes:
  0 - All checks passed (healthy)
  1 - Warnings detected (non-critical issues)
  2 - Errors detected (critical issues)

Examples:
  # Run health checks with human-readable output
  pass-cli doctor

  # Output as JSON for scripting
  pass-cli doctor --json

  # Quiet mode (exit code only, no output)
  pass-cli doctor --quiet

  # Verbose mode (detailed check execution)
  pass-cli doctor --verbose`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)

	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "Output results as JSON")
	doctorCmd.Flags().BoolVar(&doctorQuiet, "quiet", false, "Quiet mode (exit code only, no output)")
	doctorCmd.Flags().BoolVarP(&doctorVerbose, "verbose", "v", false, "Verbose output (detailed check execution)")
}

func runDoctor(cmd *cobra.Command, args []string) error {
	// Get vault path with source
	vaultPath, vaultSource := GetVaultPathWithSource()

	// Load config for sync settings (ARI-53)
	cfg, _ := config.Load()

	// Build check options
	opts := health.CheckOptions{
		CurrentVersion:  version,
		GitHubRepo:      "reyamira/pass-cli",
		VaultPath:       vaultPath,
		VaultPathSource: vaultSource,
		VaultDir:        filepath.Dir(vaultPath),
		ConfigPath:      getConfigPath(),
		SyncConfig:      cfg.Sync, // ARI-53: Pass sync config for health check
	}

	// Run all health checks
	ctx := context.Background()
	if doctorVerbose {
		fmt.Fprintln(os.Stderr, "Running health checks...")
	}

	report := health.RunChecks(ctx, opts)

	// Handle quiet mode
	if doctorQuiet {
		os.Exit(report.Summary.ExitCode)
		return nil
	}

	// Format output
	if doctorJSON {
		if err := outputHealthReportJSON(report, opts); err != nil {
			return fmt.Errorf("failed to output JSON: %w", err)
		}
	} else {
		outputHumanReadable(report, opts, doctorVerbose)
	}

	// Exit with appropriate code
	os.Exit(report.Summary.ExitCode)
	return nil
}

// outputHumanReadable formats the health report in a user-friendly way
func outputHumanReadable(report health.HealthReport, opts health.CheckOptions, verbose bool) {
	// Header
	fmt.Println()
	fmt.Println("Pass-CLI Health Check Report")
	fmt.Println("════════════════════════════════════════")
	fmt.Println()

	// Vault Path Information
	fmt.Printf("Vault Path: %s\n", opts.VaultPath)
	fmt.Printf("Path Source: %s\n", opts.VaultPathSource)
	fmt.Println()

	// Color functions
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()

	// Display each check result
	for _, check := range report.Checks {
		var icon string

		switch check.Status {
		case health.CheckPass:
			icon = "✅"
		case health.CheckWarning:
			icon = "⚠️ "
		case health.CheckError:
			icon = "❌"
		}

		// Check name and status
		fmt.Printf("%s %s: %s\n", icon, bold(check.Name), check.Message)

		// Show recommendation if present
		if check.Recommendation != "" {
			fmt.Printf("   → Recommendation: %s\n", check.Recommendation)
		}

		// Verbose mode: show details
		if verbose && check.Details != nil {
			fmt.Printf("   Details: %+v\n", check.Details)
		}

		fmt.Println()
	}

	// Summary
	fmt.Println("────────────────────────────────────────")
	fmt.Printf("Summary: %s passed, %s warnings, %s errors\n",
		green(fmt.Sprintf("%d checks", report.Summary.Passed)),
		yellow(fmt.Sprintf("%d", report.Summary.Warnings)),
		red(fmt.Sprintf("%d", report.Summary.Errors)),
	)

	// Exit code interpretation
	var exitStatus string
	switch report.Summary.ExitCode {
	case health.ExitHealthy:
		exitStatus = green("Healthy ✓")
	case health.ExitWarnings:
		exitStatus = yellow("Warnings detected")
	case health.ExitErrors:
		exitStatus = red("Errors detected")
	default:
		exitStatus = red(fmt.Sprintf("Unknown exit code: %d", report.Summary.ExitCode))
	}
	fmt.Printf("Status: %s (exit code %d)\n", exitStatus, report.Summary.ExitCode)
	fmt.Println()
}

// outputHealthReportJSON formats the health report as JSON
func outputHealthReportJSON(report health.HealthReport, opts health.CheckOptions) error {
	// Wrap report with vault path information
	output := map[string]interface{}{
		"vault_path":        opts.VaultPath,
		"vault_path_source": opts.VaultPathSource,
		"report":            report,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// getConfigPath returns the config file path
func getConfigPath() string {
	if cfgFile != "" {
		return cfgFile
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ".pass-cli/config.yaml"
	}

	return filepath.Join(home, ".pass-cli", "config.yaml")
}
