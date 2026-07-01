package components

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/arimxyer/pass-cli/cmd/tui/models"
	"github.com/arimxyer/pass-cli/internal/vault"
)

// TestNewCredentialTable verifies CredentialTable creation.
func TestNewCredentialTable(t *testing.T) {
	mockVault := NewMockVaultService()
	state := models.NewAppState(mockVault)

	table := NewCredentialTable(state)
	require.NotNil(t, table, "NewCredentialTable returned nil")

	// Verify header row exists (row 0 should be header)
	require.GreaterOrEqual(t, table.GetRowCount(), 1, "Expected at least header row")

	// Verify header cells
	serviceHeader := table.GetCell(0, 0)
	if serviceHeader == nil || serviceHeader.Text != "Service (UID)" {
		t.Error("Expected 'Service (UID)' header in column 0")
	}

	usernameHeader := table.GetCell(0, 1)
	if usernameHeader == nil || usernameHeader.Text != "Username" {
		t.Error("Expected 'Username' header in column 1")
	}

	lastUsedHeader := table.GetCell(0, 2)
	if lastUsedHeader == nil || lastUsedHeader.Text != "Last Used" {
		t.Error("Expected 'Last Used' header in column 2")
	}
}

// TestCredentialTableRefresh verifies table rebuilding.
func TestCredentialTableRefresh(t *testing.T) {
	mockVault := NewMockVaultService()
	state := models.NewAppState(mockVault)

	// Setup mock credentials
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", CreatedAt: time.Now(), LastAccessed: time.Now().Add(-2 * time.Hour)},
		{Service: "GitHub", Username: "user", CreatedAt: time.Now(), LastAccessed: time.Now().Add(-1 * time.Hour)},
		{Service: "Database", Username: "dbuser", CreatedAt: time.Now(), LastAccessed: time.Time{}},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	table := NewCredentialTable(state)

	// Refresh should populate rows
	table.Refresh()

	// Verify row count (header + 3 credentials)
	if table.GetRowCount() != 4 {
		t.Errorf("Expected 4 rows (1 header + 3 credentials), got %d", table.GetRowCount())
	}

	// Verify first credential row (row 1, after header)
	serviceCell := table.GetCell(1, 0)
	if serviceCell == nil || serviceCell.Text != "AWS" {
		t.Errorf("Expected 'AWS' in row 1 col 0, got '%s'", serviceCell.Text)
	}

	usernameCell := table.GetCell(1, 1)
	if usernameCell == nil || usernameCell.Text != "admin" {
		t.Errorf("Expected 'admin' in row 1 col 1, got '%s'", usernameCell.Text)
	}

	// Verify last used formatting
	lastUsedCell := table.GetCell(1, 2)
	if lastUsedCell == nil || lastUsedCell.Text == "" {
		t.Error("Expected last used time formatted")
	}
}

// TestCredentialTableRefresh_CategoryFilter verifies filtering by category.
func TestCredentialTableRefresh_CategoryFilter(t *testing.T) {
	mockVault := NewMockVaultService()
	state := models.NewAppState(mockVault)

	// Setup multiple credentials with Category field
	mockCreds := []vault.CredentialMetadata{
		{Service: "aws-prod", Username: "admin", Category: "Work", CreatedAt: time.Now()},
		{Service: "github-personal", Username: "user", Category: "Personal", CreatedAt: time.Now()},
		{Service: "aws-dev", Username: "backup", Category: "Work", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	table := NewCredentialTable(state)

	// Set category filter to "Work"
	state.SetSelectedCategory("Work")

	// Refresh with filter
	table.Refresh()

	// Verify only Work credentials shown (header + 2 Work credentials, Personal filtered out)
	if table.GetRowCount() != 3 {
		t.Errorf("Expected 3 rows (1 header + 2 Work), got %d", table.GetRowCount())
	}

	// Verify both rows are Work category
	row1Service := table.GetCell(1, 0)
	row2Service := table.GetCell(2, 0)
	require.NotNil(t, row1Service, "Expected credential row 1")
	require.NotNil(t, row2Service, "Expected credential row 2")
	// Both should be Work category credentials (aws-prod and aws-dev)
	if (row1Service.Text != "aws-prod" && row1Service.Text != "aws-dev") ||
		(row2Service.Text != "aws-prod" && row2Service.Text != "aws-dev") {
		t.Errorf("Expected only Work category credentials, got '%s' and '%s'", row1Service.Text, row2Service.Text)
	}
}

// TestCredentialTableRefresh_NoFilter verifies showing all credentials.
func TestCredentialTableRefresh_NoFilter(t *testing.T) {
	mockVault := NewMockVaultService()
	state := models.NewAppState(mockVault)

	// Setup credentials
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", CreatedAt: time.Now()},
		{Service: "GitHub", Username: "user", CreatedAt: time.Now()},
		{Service: "Database", Username: "dbuser", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	table := NewCredentialTable(state)

	// No category filter (empty string means show all)
	state.SetSelectedCategory("")

	// Refresh
	table.Refresh()

	// Verify all credentials shown (header + 3 credentials)
	if table.GetRowCount() != 4 {
		t.Errorf("Expected 4 rows (1 header + 3 credentials), got %d", table.GetRowCount())
	}
}

// TestCredentialTableSelection verifies selection handling.
func TestCredentialTableSelection(t *testing.T) {
	mockVault := NewMockVaultService()
	state := models.NewAppState(mockVault)

	// Setup credentials
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", CreatedAt: time.Now()},
		{Service: "GitHub", Username: "user", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	table := NewCredentialTable(state)

	// Clear auto-selection from Refresh() to test fresh selection
	state.SetSelectedCredential(nil)

	// Track selection changes
	selectionChanged := false
	state.SetOnSelectionChanged(func() {
		selectionChanged = true
	})

	// Simulate selecting row 1 (first credential after header)
	table.applySelection(1)

	// Verify callback invoked
	if !selectionChanged {
		t.Error("Selection change callback was not invoked")
	}

	// Verify correct credential selected
	selected := state.GetSelectedCredential()
	require.NotNil(t, selected, "Expected selected credential")
	require.Equal(t, "AWS", selected.Service, "Expected selected service 'AWS'")
}

// TestCredentialTableSelection_ShortCircuit verifies that reselecting the same credential doesn't trigger callback.
func TestCredentialTableSelection_ShortCircuit(t *testing.T) {
	mockVault := NewMockVaultService()
	state := models.NewAppState(mockVault)

	// Setup credentials
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", CreatedAt: time.Now()},
		{Service: "GitHub", Username: "user", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	table := NewCredentialTable(state)

	// First selection (AWS auto-selected during Refresh)
	firstSelected := state.GetSelectedCredential()
	if firstSelected == nil || firstSelected.Service != "AWS" {
		t.Fatal("Expected AWS to be auto-selected")
	}

	// Track selection changes AFTER initial selection
	callbackCount := 0
	state.SetOnSelectionChanged(func() {
		callbackCount++
	})

	// Reselect same credential (row 1 = AWS)
	table.applySelection(1)

	// Verify callback NOT invoked (short-circuit)
	if callbackCount != 0 {
		t.Errorf("Expected callback NOT to be invoked for same selection, but was called %d times", callbackCount)
	}

	// Verify selection unchanged
	selected := state.GetSelectedCredential()
	if selected == nil || selected.Service != "AWS" {
		t.Error("Expected AWS to remain selected")
	}

	// Now select a DIFFERENT credential (row 2 = GitHub)
	table.applySelection(2)

	// Verify callback WAS invoked (different selection)
	if callbackCount != 1 {
		t.Errorf("Expected callback to be invoked once for different selection, but was called %d times", callbackCount)
	}

	// Verify new selection
	selected = state.GetSelectedCredential()
	if selected == nil || selected.Service != "GitHub" {
		t.Error("Expected GitHub to be selected")
	}
}

// TestCredentialTableSelection_HeaderRow verifies header row is not selectable.
func TestCredentialTableSelection_HeaderRow(t *testing.T) {
	mockVault := NewMockVaultService()
	state := models.NewAppState(mockVault)

	// Setup credentials
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	table := NewCredentialTable(state)

	// Clear any auto-selection from Refresh() (which now triggers applySelection)
	state.SetSelectedCredential(nil)

	// Try selecting header row (row 0) via applySelection
	table.applySelection(0)

	// Verify no credential selected (should still be nil)
	selected := state.GetSelectedCredential()
	if selected != nil {
		t.Error("Header row selection should not set selected credential")
	}

	// Also verify calling applySelection multiple times on header has no effect
	table.applySelection(0)
	selected = state.GetSelectedCredential()
	if selected != nil {
		t.Error("Multiple header row selections should not set selected credential")
	}
}

// TestCredentialTablePopulateRows verifies row population with correct data.
func TestCredentialTablePopulateRows(t *testing.T) {
	mockVault := NewMockVaultService()
	state := models.NewAppState(mockVault)

	// Setup credentials with different last accessed times
	now := time.Now()
	// Includes a lowercase "azure" entry: under a case-sensitive sort it would
	// land last (ASCII 'a' > 'G'), but case-insensitively it belongs after AWS.
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", CreatedAt: now, LastAccessed: now.Add(-30 * time.Second)},
		{Service: "GitHub", Username: "user", CreatedAt: now, LastAccessed: now.Add(-5 * time.Minute)},
		{Service: "Database", Username: "dbuser", CreatedAt: now, LastAccessed: time.Time{}},
		{Service: "azure", Username: "svc", CreatedAt: now, LastAccessed: now.Add(-1 * time.Minute)},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	table := NewCredentialTable(state)
	table.Refresh()

	// Rows are sorted alphabetically by service (case-insensitive), so the
	// expected order is AWS, azure, Database, GitHub regardless of insertion order.
	wantOrder := []string{"AWS", "azure", "Database", "GitHub"}
	for i, want := range wantOrder {
		got := table.GetCell(i+1, 0).Text
		if got != want {
			t.Errorf("Expected row %d service '%s', got '%s'", i+1, want, got)
		}
	}

	// Verify credential reference stored in cell
	if table.GetCell(1, 0).GetReference() == nil {
		t.Error("Expected credential reference in cell, got nil")
	}

	// GitHub (row 4) last used formatted
	if got := table.GetCell(4, 2).Text; got == "" {
		t.Error("Expected formatted last used time")
	}

	// Database (row 3) shows "Never" for zero time
	if got := table.GetCell(3, 2).Text; got != "Never" {
		t.Errorf("Expected 'Never' for zero LastAccessed, got '%s'", got)
	}
}

// TestCredentialTableFilter_ByCategory verifies filterByCategory logic.
func TestCredentialTableFilter_ByCategory(t *testing.T) {
	mockVault := NewMockVaultService()
	state := models.NewAppState(mockVault)

	table := NewCredentialTable(state)

	// Test data with Category field properly set
	allCreds := []vault.CredentialMetadata{
		{Service: "aws-prod", Username: "admin", Category: "Work"},
		{Service: "github-personal", Username: "user", Category: "Personal"},
		{Service: "aws-dev", Username: "backup", Category: "Work"},
		{Service: "database", Username: "dbuser", Category: "Development"},
	}

	// Test filter for "Work" category (should return 2 credentials)
	filtered := table.filterByCategory(allCreds, "Work")
	if len(filtered) != 2 {
		t.Errorf("Expected 2 Work credentials, got %d", len(filtered))
	}
	for _, cred := range filtered {
		if cred.Category != "Work" {
			t.Errorf("Expected only Work category credentials, got Category='%s' Service='%s'", cred.Category, cred.Service)
		}
	}

	// Test filter for "Personal" category (should return 1 credential)
	filtered = table.filterByCategory(allCreds, "Personal")
	if len(filtered) != 1 {
		t.Errorf("Expected 1 Personal credential, got %d", len(filtered))
	}
	if filtered[0].Category != "Personal" {
		t.Errorf("Expected Personal category, got '%s'", filtered[0].Category)
	}

	// Test empty filter (show all)
	filtered = table.filterByCategory(allCreds, "")
	if len(filtered) != 4 {
		t.Errorf("Expected all 4 credentials, got %d", len(filtered))
	}

	// Test filter with no matches
	filtered = table.filterByCategory(allCreds, "NonExistent")
	if len(filtered) != 0 {
		t.Errorf("Expected 0 credentials for non-existent category, got %d", len(filtered))
	}
}

// TestCredentialTableRefresh_UpdatesTitle verifies title shows count.
func TestCredentialTableRefresh_UpdatesTitle(t *testing.T) {
	mockVault := NewMockVaultService()
	state := models.NewAppState(mockVault)

	// Setup 3 credentials
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", CreatedAt: time.Now()},
		{Service: "GitHub", Username: "user", CreatedAt: time.Now()},
		{Service: "Database", Username: "dbuser", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	table := NewCredentialTable(state)

	// Refresh
	table.Refresh()

	// Verify title includes count
	// Note: tview.Table doesn't expose GetTitle() easily, but we can verify the count internally
	if len(table.filteredCreds) != 3 {
		t.Errorf("Expected 3 filtered credentials, got %d", len(table.filteredCreds))
	}
}

// TestFormatRelativeTime verifies time formatting logic.
func TestFormatRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{"Just now", now.Add(-30 * time.Second), "Just now"},
		{"Minutes ago", now.Add(-5 * time.Minute), "5m ago"},
		{"Hours ago", now.Add(-3 * time.Hour), "3h ago"},
		{"Days ago", now.Add(-2 * 24 * time.Hour), "2d ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatRelativeTime(tt.time)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestCredentialTableSortByField verifies the table honors the AppState sort
// field and direction, with a Service tie-break for determinism.
func TestCredentialTableSortByField(t *testing.T) {
	mockVault := NewMockVaultService()
	state := models.NewAppState(mockVault)

	now := time.Now()
	mockCreds := []vault.CredentialMetadata{
		{Service: "github", Username: "amy", LastAccessed: now.Add(-1 * time.Hour)},
		{Service: "AWS", Username: "zoe", LastAccessed: now.Add(-5 * time.Minute)},
		{Service: "azure", Username: "bob", LastAccessed: time.Time{}}, // Never
	}
	mockVault.SetCredentials(mockCreds)
	require.NoError(t, state.LoadCredentials())

	table := NewCredentialTable(state)

	services := func() []string {
		out := make([]string, 0, 3)
		for r := 1; r <= 3; r++ {
			out = append(out, table.GetCell(r, 0).Text)
		}
		return out
	}

	// Default: Service ascending, case-insensitive.
	table.Refresh()
	require.Equal(t, []string{"AWS", "azure", "github"}, services(), "service asc")

	// Cycle to Username ascending: amy(github), bob(azure), zoe(AWS).
	state.CycleSortField()
	table.Refresh()
	require.Equal(t, []string{"github", "azure", "AWS"}, services(), "username asc")

	// Cycle to Last Used ascending: Never(azure), -1h(github), -5m(AWS).
	state.CycleSortField()
	table.Refresh()
	require.Equal(t, []string{"azure", "github", "AWS"}, services(), "last-used asc")

	// Reverse direction: Last Used descending.
	state.ToggleSortDirection()
	table.Refresh()
	require.Equal(t, []string{"AWS", "github", "azure"}, services(), "last-used desc")

	// Cycle wraps back to Service.
	state.CycleSortField()
	require.Equal(t, models.SortByService, state.GetSortField(), "cycle wraps to Service")
}
