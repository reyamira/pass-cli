package components

import (
	"fmt"
	"sort"
	"time"

	"github.com/arimxyer/pass-cli/cmd/tui/models"
	"github.com/arimxyer/pass-cli/cmd/tui/styles"
	"github.com/arimxyer/pass-cli/internal/vault"
	"github.com/rivo/tview"
)

// CredentialTable wraps tview.Table to display credentials in tabular format.
// Supports filtering by category and selection handling.
type CredentialTable struct {
	*tview.Table

	appState      *models.AppState
	filteredCreds []vault.CredentialMetadata
	selectedIndex int
}

// NewCredentialTable creates and configures a new CredentialTable component.
// Creates Table with fixed header row and builds initial credential list.
func NewCredentialTable(appState *models.AppState) *CredentialTable {
	table := tview.NewTable()

	ct := &CredentialTable{
		Table:         table,
		appState:      appState,
		filteredCreds: make([]vault.CredentialMetadata, 0),
		selectedIndex: 0,
	}

	// Configure table
	ct.SetSelectable(true, false) // Select rows, not columns
	ct.SetFixed(1, 0)             // Fix header row

	// Apply styling
	ct.applyStyles()

	// Build header
	ct.buildHeader()

	// Setup selection handlers
	// SetSelectionChangedFunc handles arrow key navigation
	ct.SetSelectionChangedFunc(func(row, col int) { ct.applySelection(row) })
	// SetSelectedFunc handles Enter key activation
	ct.SetSelectedFunc(func(row, col int) { ct.applySelection(row) })

	// Initial population
	ct.Refresh()

	return ct
}

// buildHeader creates the fixed header row with column titles.
// Header row is not selectable and uses accent color.
func (ct *CredentialTable) buildHeader() {
	theme := styles.GetCurrentTheme()
	headers := []string{"Service (UID)", "Username", "Last Used"}

	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(theme.TableHeader). // Purple header
			SetAlign(tview.AlignLeft).
			SetSelectable(false).
			SetExpansion(1)
		ct.SetCell(0, col, cell)
	}
}

// Refresh rebuilds the table from filtered credentials.
// Gets credentials from AppState, filters by selected category and search query, and updates rows.
// Uses incremental updates: reuses existing rows instead of full rebuild for better performance.
func (ct *CredentialTable) Refresh() {
	// Get credentials and filter by category (thread-safe read)
	allCreds := ct.appState.GetCredentials()
	category := ct.appState.GetSelectedCategory()
	categoryFiltered := ct.filterByCategory(allCreds, category)

	// Apply search filter on top of category filter
	searchState := ct.appState.GetSearchState()
	ct.filteredCreds = ct.filterBySearch(categoryFiltered, searchState)

	// Sort by service alphabetically (case-insensitive) so the table matches
	// the sidebar's ordering; GetCredentials returns map-iteration order.
	sort.Slice(ct.filteredCreds, func(i, j int) bool {
		return lessFold(ct.filteredCreds[i].Service, ct.filteredCreds[j].Service)
	})

	// Get current row count (excluding header)
	currentRowCount := ct.GetRowCount() - 1
	newRowCount := len(ct.filteredCreds)

	theme := styles.GetCurrentTheme()

	// Update existing rows and add new ones if needed
	for i, cred := range ct.filteredCreds {
		row := i + 1 // +1 to skip header row

		if i < currentRowCount {
			// Reuse existing row - update cell contents
			ct.GetCell(row, 0).SetText(cred.Service).SetReference(cred)
			ct.GetCell(row, 1).SetText(cred.Username)

			lastUsed := "Never"
			if !cred.LastAccessed.IsZero() {
				lastUsed = formatRelativeTime(cred.LastAccessed)
			}
			ct.GetCell(row, 2).SetText(lastUsed)
		} else {
			// Add new row (same as populateRows logic)
			serviceCell := tview.NewTableCell(cred.Service).
				SetTextColor(theme.TextPrimary).
				SetAlign(tview.AlignLeft).
				SetReference(cred)

			usernameCell := tview.NewTableCell(cred.Username).
				SetTextColor(theme.TableHeader).
				SetAlign(tview.AlignLeft)

			lastUsed := "Never"
			if !cred.LastAccessed.IsZero() {
				lastUsed = formatRelativeTime(cred.LastAccessed)
			}
			lastUsedCell := tview.NewTableCell(lastUsed).
				SetTextColor(theme.TextSecondary).
				SetAlign(tview.AlignLeft)

			ct.SetCell(row, 0, serviceCell)
			ct.SetCell(row, 1, usernameCell)
			ct.SetCell(row, 2, lastUsedCell)
		}
	}

	// Remove excess rows if new list is shorter
	if newRowCount < currentRowCount {
		for row := currentRowCount; row > newRowCount; row-- {
			ct.RemoveRow(row)
		}
	}

	// Update title with count
	ct.SetTitle(fmt.Sprintf(" Credentials (%d) ", len(ct.filteredCreds)))

	// Restore selection if possible
	if len(ct.filteredCreds) > 0 {
		// Select first row if no selection or out of bounds
		if ct.selectedIndex >= len(ct.filteredCreds) {
			ct.selectedIndex = 0
		}
		ct.Select(ct.selectedIndex+1, 0) // +1 to account for header row
	}
}

// applySelection applies selection for a given row by updating AppState.
// Used by both arrow key navigation and Enter key activation handlers.
// Sources credentials via FindCredentialByService to ensure consistency and avoid stale pointers.
func (ct *CredentialTable) applySelection(row int) {
	if row == 0 {
		return // Header row, ignore
	}

	// Get credential from cell reference to extract service name
	cell := ct.GetCell(row, 0)
	if cell != nil {
		if cred, ok := cell.GetReference().(vault.CredentialMetadata); ok {
			ct.selectedIndex = row - 1 // Store index without header offset

			// Source credential via FindCredentialByService for consistency
			// This ensures we get a fresh pointer from AppState, avoiding stale references
			if credMeta, found := ct.appState.FindCredentialByService(cred.Service); found {
				ct.appState.SetSelectedCredential(credMeta)
			}
		}
	}
}

// filterByCategory filters credentials by selected category.
// Empty category returns all credentials.
func (ct *CredentialTable) filterByCategory(creds []vault.CredentialMetadata, category string) []vault.CredentialMetadata {
	if category == "" {
		return creds // Show all
	}

	filtered := make([]vault.CredentialMetadata, 0)
	for _, cred := range creds {
		if cred.Category == category {
			filtered = append(filtered, cred)
		}
	}
	return filtered
}

// filterBySearch filters credentials by search query.
// Returns all credentials if search is inactive or query is empty.
func (ct *CredentialTable) filterBySearch(creds []vault.CredentialMetadata, searchState *models.SearchState) []vault.CredentialMetadata {
	if searchState == nil || !searchState.Active || searchState.Query == "" {
		return creds // No search filter
	}

	filtered := make([]vault.CredentialMetadata, 0)
	for _, cred := range creds {
		if searchState.MatchesCredential(&cred) {
			filtered = append(filtered, cred)
		}
	}
	return filtered
}

// applyStyles applies borders, colors, and title to the table.
// Uses rounded borders with cyan accent color and dark background.
func (ct *CredentialTable) applyStyles() {
	theme := styles.GetCurrentTheme()
	styles.ApplyBorderedStyle(ct.Table, "Credentials", true)
	styles.ApplyTableStyle(ct.Table)
	// Explicitly set background to ensure it applies to table area
	ct.SetBackgroundColor(theme.Background)
}

// formatRelativeTime formats a timestamp as a relative time string.
// Examples: "2m ago", "5h ago", "3d ago"
func formatRelativeTime(t time.Time) string {
	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return "Just now"
	case duration < time.Hour:
		return fmt.Sprintf("%dm ago", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(duration.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(duration.Hours()/24))
	}
}
