package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/internal/vault"
)

var (
	listFormat        string
	listUnused        bool
	listDays          int
	listByProject     bool   // T029: --by-project flag
	listLocation      string // T042: --location flag (for User Story 3)
	listRecursive     bool   // T043: --recursive flag (for User Story 3)
	listShowUsernames bool   // #95: opt the Username column back into the default table
	listQuiet         bool   // #95: -q/--quiet alias for --format simple
)

var listCmd = &cobra.Command{
	Use:     "list",
	GroupID: "credentials",
	Short:   "List all credentials in the vault",
	Long: `List displays all stored credentials with metadata.

Output formats:
  table    Display as formatted table (default)
  json     Output as JSON array
  simple   Simple list of service names only

Safety: the table hides usernames by default. The "username" field can hold
sensitive values (card, account, or routing numbers stored as a username), so
listing should not dump them. Pass --show-usernames to include the column.
(The --format json output is an explicit structured opt-in and still emits the
full metadata, including usernames.)

The -q/--quiet flag is a shorthand for --format simple: it prints bare service
names, one per line. -q takes precedence over --format.

The --unused flag filters credentials that haven't been accessed recently
or have never been accessed. Use --days to configure the threshold
(default: 30 days).

The --by-project flag groups credentials by git repository context,
showing which credentials are used in which projects.

The --location flag filters credentials accessed from a specific directory.
Use --recursive to include subdirectories.`,
	Example: `  # List all credentials as table (usernames hidden)
  pass-cli list

  # Include the username column (may reveal sensitive values)
  pass-cli list --show-usernames

  # List bare service names, one per line
  pass-cli list -q

  # List as JSON
  pass-cli list --format json

  # List simple service names
  pass-cli list --format simple

  # Show unused credentials (>30 days)
  pass-cli list --unused

  # Show credentials unused for >90 days
  pass-cli list --unused --days 90

  # Group credentials by project
  pass-cli list --by-project

  # Group by project with JSON output
  pass-cli list --by-project --format json

  # Filter by location
  pass-cli list --location /path/to/project

  # Filter by location (recursive) and group by project
  pass-cli list --location /path/to/project --recursive --by-project`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().StringVarP(&listFormat, "format", "f", "table", "output format: table, json, simple")
	listCmd.Flags().BoolVar(&listUnused, "unused", false, "show only unused or rarely used credentials")
	listCmd.Flags().IntVar(&listDays, "days", 30, "days threshold for --unused flag")
	listCmd.Flags().BoolVar(&listByProject, "by-project", false, "group credentials by git repository")              // T029
	listCmd.Flags().StringVar(&listLocation, "location", "", "filter credentials by directory path")                 // T042
	listCmd.Flags().BoolVar(&listRecursive, "recursive", false, "include subdirectories with --location")            // T043
	listCmd.Flags().BoolVar(&listShowUsernames, "show-usernames", false, "include the username column in the table") // #95
	listCmd.Flags().BoolVarP(&listQuiet, "quiet", "q", false, "print bare service names only (alias for --format simple)")
}

func runList(cmd *cobra.Command, args []string) error {
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

	// Get credential metadata
	metadata, err := vaultService.ListCredentialsWithMetadata()
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	// Filter for unused if requested
	if listUnused {
		metadata = filterUnused(metadata, listDays)
	}

	// T044-T048: Filter by location if requested (User Story 3)
	if listLocation != "" {
		filtered, err := filterCredentialsByLocation(metadata, listLocation, listRecursive)
		if err != nil {
			return fmt.Errorf("failed to filter by location: %w", err)
		}
		metadata = filtered

		// T050: Handle empty results
		if len(metadata) == 0 {
			fmt.Printf("No credentials found for location: %s\n", listLocation)
			return nil
		}
	}

	// #95: -q/--quiet is an alias for --format simple and takes precedence.
	effectiveFormat := resolveListFormat(listFormat, listQuiet)

	// T030-T034: Handle --by-project mode (User Story 2)
	// T048: Works with --location (filter first, then group)
	if listByProject {
		projects := groupCredentialsByProject(metadata)
		return outputByProject(projects, effectiveFormat)
	}

	// Standard list mode (existing behavior)
	// Sort by service name
	sort.Slice(metadata, func(i, j int) bool {
		return metadata[i].Service < metadata[j].Service
	})

	// Output in requested format
	switch strings.ToLower(effectiveFormat) {
	case "json":
		return outputJSON(metadata)
	case "simple":
		return outputSimple(metadata)
	case "table":
		return outputTable(metadata)
	default:
		return fmt.Errorf("invalid format: %s (valid: table, json, simple)", effectiveFormat)
	}
}

// resolveListFormat returns the output format to use. #95: -q/--quiet is a
// shorthand for "simple" and takes precedence over any --format value.
func resolveListFormat(format string, quiet bool) string {
	if quiet {
		return "simple"
	}
	return format
}

func filterUnused(metadata []vault.CredentialMetadata, days int) []vault.CredentialMetadata {
	threshold := time.Now().AddDate(0, 0, -days)
	filtered := make([]vault.CredentialMetadata, 0)

	for _, meta := range metadata {
		// Include if never accessed or not accessed since threshold
		if meta.UsageCount == 0 || meta.LastAccessed.Before(threshold) {
			filtered = append(filtered, meta)
		}
	}

	return filtered
}

// T044: filterCredentialsByLocation filters credentials by access location
// T045: Resolves relative paths to absolute
// T046: Exact match by default
// T047: Prefix match with recursive flag
func filterCredentialsByLocation(metadata []vault.CredentialMetadata, location string, recursive bool) ([]vault.CredentialMetadata, error) {
	// T045: Resolve relative path to absolute
	absLocation, err := filepath.Abs(location)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	// Resolve symlinks to get canonical path (fixes macOS /var -> /private/var symlink issue)
	canonicalLocation, err := filepath.EvalSymlinks(absLocation)
	if err != nil {
		// If symlink resolution fails (e.g., path doesn't exist), use absolute path
		canonicalLocation = absLocation
	}

	filtered := make([]vault.CredentialMetadata, 0)

	for _, meta := range metadata {
		// Check if this credential was accessed from the specified location
		for _, credLocation := range meta.Locations {
			// Convert credential location to absolute path for comparison
			absCredLocation, err := filepath.Abs(credLocation)
			if err != nil {
				// If we can't resolve the credential location, skip it
				continue
			}

			// Resolve symlinks to get canonical path
			canonicalCredLocation, err := filepath.EvalSymlinks(absCredLocation)
			if err != nil {
				// If symlink resolution fails, use absolute path
				canonicalCredLocation = absCredLocation
			}

			matched := false

			if recursive {
				// T047: Recursive mode - check if credential location is under the specified location
				// Use filepath.HasPrefix logic (check if credLocation starts with location)
				if canonicalCredLocation == canonicalLocation || strings.HasPrefix(canonicalCredLocation, canonicalLocation+string(filepath.Separator)) {
					matched = true
				}
			} else {
				// T046: Exact match mode - credential location must exactly match
				if canonicalCredLocation == canonicalLocation {
					matched = true
				}
			}

			if matched {
				filtered = append(filtered, meta)
				break // Found a match, no need to check other locations for this credential
			}
		}
	}

	return filtered, nil
}

func outputSimple(metadata []vault.CredentialMetadata) error {
	for _, meta := range metadata {
		fmt.Println(meta.Service)
	}
	return nil
}

func outputJSON(metadata []vault.CredentialMetadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func outputTable(metadata []vault.CredentialMetadata) error {
	if len(metadata) == 0 {
		fmt.Println("No credentials found.")
		return nil
	}

	table := tablewriter.NewWriter(os.Stdout)

	// Prepare header.
	// #95: The Username column is omitted by default because that field can hold
	// sensitive values (e.g. card/account/routing numbers). It is included only
	// when --show-usernames is set (omitted entirely, not masked).
	var header []string
	if listShowUsernames {
		header = []string{"Service", "Username", "Usage", "Last Used", "Created"}
	} else {
		header = []string{"Service", "Usage", "Last Used", "Created"}
	}

	// Prepare data rows
	var data [][]string
	for _, meta := range metadata {
		// Format usage string
		usageStr := fmt.Sprintf("%d", meta.UsageCount)
		if meta.UsageCount == 0 {
			usageStr = "Never"
		} else if len(meta.Locations) > 0 {
			usageStr = fmt.Sprintf("%d (%d loc)", meta.UsageCount, len(meta.Locations))
		}

		// Format last used
		lastUsedStr := "Never"
		if meta.UsageCount > 0 {
			lastUsedStr = formatRelativeTime(meta.LastAccessed)
		}

		// Format created
		createdStr := formatRelativeTime(meta.CreatedAt)

		if listShowUsernames {
			// Truncate username if too long
			username := meta.Username
			if len(username) > 30 {
				username = username[:27] + "..."
			}

			data = append(data, []string{
				meta.Service,
				username,
				usageStr,
				lastUsedStr,
				createdStr,
			})
		} else {
			data = append(data, []string{
				meta.Service,
				usageStr,
				lastUsedStr,
				createdStr,
			})
		}
	}

	// Set table configuration
	table.Header(header)
	_ = table.Bulk(data)
	_ = table.Render()

	// Show summary
	fmt.Printf("\nTotal: %d credential(s)\n", len(metadata))

	return nil
}

// T030: groupCredentialsByProject groups credentials by git repository
// Returns map of project name → sorted list of credential service names
func groupCredentialsByProject(metadata []vault.CredentialMetadata) map[string][]string {
	projects := make(map[string]map[string]bool) // project → set of credentials

	for _, meta := range metadata {
		// Get git repositories for this credential
		repos := meta.GitRepositories

		// If no git repository found, mark as "Ungrouped" (T034)
		if len(repos) == 0 {
			repos = []string{"Ungrouped"}
		}

		// Add credential to each project it belongs to
		for _, project := range repos {
			if projects[project] == nil {
				projects[project] = make(map[string]bool)
			}
			projects[project][meta.Service] = true
		}
	}

	// Convert map[string]bool to []string and sort
	result := make(map[string][]string)
	for project, credSet := range projects {
		creds := make([]string, 0, len(credSet))
		for cred := range credSet {
			creds = append(creds, cred)
		}
		sort.Strings(creds)
		result[project] = creds
	}

	return result
}

// outputByProject dispatches to format-specific output functions
func outputByProject(projects map[string][]string, format string) error {
	switch strings.ToLower(format) {
	case "json":
		return outputByProjectJSON(projects) // T032
	case "simple":
		return outputByProjectSimple(projects) // T033
	case "table":
		return outputByProjectTable(projects) // T031
	default:
		return fmt.Errorf("invalid format: %s (valid: table, json, simple)", format)
	}
}

// T031: outputByProjectTable displays grouped credentials in table format
func outputByProjectTable(projects map[string][]string) error {
	if len(projects) == 0 {
		fmt.Println("No credentials found.")
		return nil
	}

	// Sort project names alphabetically
	projectNames := make([]string, 0, len(projects))
	for project := range projects {
		projectNames = append(projectNames, project)
	}
	sort.Strings(projectNames)

	// Display each project group
	for _, project := range projectNames {
		creds := projects[project]
		credCount := len(creds)

		// Plural handling
		plural := "credentials"
		if credCount == 1 {
			plural = "credential"
		}

		// Project header
		fmt.Printf("%s (%d %s):\n", project, credCount, plural)

		// List credentials (indented)
		for _, cred := range creds {
			fmt.Printf("  %s\n", cred)
		}

		fmt.Println() // Blank line between groups
	}

	return nil
}

// T032: outputByProjectJSON displays grouped credentials in JSON format
func outputByProjectJSON(projects map[string][]string) error {
	output := map[string]interface{}{
		"projects": projects,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

// T033: outputByProjectSimple displays grouped credentials in simple format
// Format: "project-name: cred1 cred2 cred3"
func outputByProjectSimple(projects map[string][]string) error {
	if len(projects) == 0 {
		return nil
	}

	// Sort project names alphabetically
	projectNames := make([]string, 0, len(projects))
	for project := range projects {
		projectNames = append(projectNames, project)
	}
	sort.Strings(projectNames)

	// Output each project on one line
	for _, project := range projectNames {
		creds := projects[project]
		fmt.Printf("%s: %s\n", project, strings.Join(creds, " "))
	}

	return nil
}
