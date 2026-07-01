package models

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rivo/tview"
	"github.com/stretchr/testify/require"

	"github.com/arimxyer/pass-cli/internal/vault"
)

// MockVaultService is a mock implementation of VaultService for testing.
type MockVaultService struct {
	mu sync.Mutex

	// Mock data
	credentials []vault.CredentialMetadata

	// Mock behaviors
	listError   error
	addError    error
	updateError error
	deleteError error
	getError    error

	// Call tracking
	listCalled   int
	addCalled    int
	updateCalled int
	deleteCalled int
	getCalled    int
}

// NewMockVaultService creates a new mock vault service.
func NewMockVaultService() *MockVaultService {
	return &MockVaultService{
		credentials: make([]vault.CredentialMetadata, 0),
	}
}

// ListCredentialsWithMetadata returns the mock credentials.
func (m *MockVaultService) ListCredentialsWithMetadata() ([]vault.CredentialMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.listCalled++
	if m.listError != nil {
		return nil, m.listError
	}
	return m.credentials, nil
}

// AddCredential adds a mock credential.
// T020d: Updated signature to accept []byte password
func (m *MockVaultService) AddCredential(service, username string, password []byte, category, url, notes string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.addCalled++
	if m.addError != nil {
		return m.addError
	}

	// Add credential to mock storage
	cred := vault.CredentialMetadata{
		Service:      service,
		Username:     username,
		Category:     category,
		URL:          url,
		Notes:        notes,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		LastAccessed: time.Time{},
	}
	m.credentials = append(m.credentials, cred)
	return nil
}

// UpdateCredential updates a mock credential.
func (m *MockVaultService) UpdateCredential(service string, opts vault.UpdateOpts) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.updateCalled++
	if m.updateError != nil {
		return m.updateError
	}

	// Find and update credential
	for i, cred := range m.credentials {
		if cred.Service == service {
			if opts.Username != nil {
				m.credentials[i].Username = *opts.Username
			}
			if opts.Category != nil {
				m.credentials[i].Category = *opts.Category
			}
			if opts.URL != nil {
				m.credentials[i].URL = *opts.URL
			}
			if opts.Notes != nil {
				m.credentials[i].Notes = *opts.Notes
			}
			m.credentials[i].UpdatedAt = time.Now()
			return nil
		}
	}
	return errors.New("credential not found")
}

// DeleteCredential deletes a mock credential.
func (m *MockVaultService) DeleteCredential(service string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.deleteCalled++
	if m.deleteError != nil {
		return m.deleteError
	}

	// Find and remove credential
	for i, cred := range m.credentials {
		if cred.Service == service {
			m.credentials = append(m.credentials[:i], m.credentials[i+1:]...)
			return nil
		}
	}
	return errors.New("credential not found")
}

// GetCredential returns a mock full credential.
func (m *MockVaultService) GetCredential(service string, trackUsage bool) (*vault.Credential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getCalled++
	if m.getError != nil {
		return nil, m.getError
	}

	// Find credential
	for _, cred := range m.credentials {
		if cred.Service == service {
			// T020d: Return []byte password
			return &vault.Credential{
				Service:     cred.Service,
				Username:    cred.Username,
				Password:    []byte("mock-password"),
				Notes:       cred.Notes,
				CreatedAt:   cred.CreatedAt,
				UpdatedAt:   cred.UpdatedAt,
				UsageRecord: make(map[string]vault.UsageRecord),
			}, nil
		}
	}
	return nil, errors.New("credential not found")
}

func (m *MockVaultService) RecordFieldAccess(service, field string) error {
	return nil
}

func (m *MockVaultService) GetTOTPCode(service string) (string, int, error) {
	return "", 0, errors.New("TOTP not configured")
}

// SetCredentials sets the mock credentials for testing.
func (m *MockVaultService) SetCredentials(creds []vault.CredentialMetadata) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.credentials = creds
}

// GetCalled returns the number of times GetCredential was called.
func (m *MockVaultService) GetCalled() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getCalled
}

// TestNewAppState verifies AppState creation.
func TestNewAppState(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	if state == nil {
		t.Fatal("NewAppState returned nil")
	}

	// Verify initial state
	if len(state.GetCredentials()) != 0 {
		t.Errorf("Expected empty credentials, got %d", len(state.GetCredentials()))
	}
	if len(state.GetCategories()) != 0 {
		t.Errorf("Expected empty categories, got %d", len(state.GetCategories()))
	}
	if state.GetSelectedCategory() != "" {
		t.Errorf("Expected empty selected category, got %s", state.GetSelectedCategory())
	}
	if state.GetSelectedCredential() != nil {
		t.Error("Expected nil selected credential")
	}
}

// TestLoadCredentials verifies credential loading from vault.
func TestLoadCredentials(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Setup mock data
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", CreatedAt: time.Now()},
		{Service: "GitHub", Username: "user", CreatedAt: time.Now()},
		{Service: "Database", Username: "dbuser", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)

	// Load credentials
	err := state.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials failed: %v", err)
	}

	// Verify credentials loaded
	creds := state.GetCredentials()
	if len(creds) != 3 {
		t.Errorf("Expected 3 credentials, got %d", len(creds))
	}

	// Verify categories extracted (all credentials have empty category → "Uncategorized")
	categories := state.GetCategories()
	if len(categories) != 1 {
		t.Errorf("Expected 1 category, got %d", len(categories))
	}
	if categories[0] != "Uncategorized" {
		t.Errorf("Expected category 'Uncategorized', got '%s'", categories[0])
	}
}

// TestLoadCredentials_Error verifies error handling in LoadCredentials.
func TestLoadCredentials_Error(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Setup error
	expectedErr := errors.New("vault error")
	mockVault.listError = expectedErr

	// Setup error callback
	var callbackErr error
	state.SetOnError(func(err error) {
		callbackErr = err
	})

	// Load credentials (should fail)
	err := state.LoadCredentials()
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// Verify error callback invoked
	if callbackErr == nil {
		t.Error("Error callback was not invoked")
	}
}

// TestAddCredential verifies adding a credential.
func TestAddCredential(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Track callback invocation
	callbackInvoked := false
	state.SetOnCredentialsChanged(func() {
		callbackInvoked = true
	})

	// Add credential with all fields
	err := state.AddCredential("AWS", "admin", "password123", "Cloud", "https://aws.amazon.com", "Test notes")
	if err != nil {
		t.Fatalf("AddCredential failed: %v", err)
	}

	// Verify callback invoked
	if !callbackInvoked {
		t.Error("onCredentialsChanged callback was not invoked")
	}

	// Verify credential added to mock vault
	if mockVault.addCalled != 1 {
		t.Errorf("Expected AddCredential called 1 time, got %d", mockVault.addCalled)
	}

	// Verify state updated
	creds := state.GetCredentials()
	if len(creds) != 1 {
		t.Errorf("Expected 1 credential, got %d", len(creds))
	}
	if creds[0].Service != "AWS" {
		t.Errorf("Expected service 'AWS', got '%s'", creds[0].Service)
	}
	if creds[0].Category != "Cloud" {
		t.Errorf("Expected category 'Cloud', got '%s'", creds[0].Category)
	}
	if creds[0].URL != "https://aws.amazon.com" {
		t.Errorf("Expected URL 'https://aws.amazon.com', got '%s'", creds[0].URL)
	}
	if creds[0].Notes != "Test notes" {
		t.Errorf("Expected notes 'Test notes', got '%s'", creds[0].Notes)
	}
}

// TestUpdateCredential verifies updating a credential.
func TestUpdateCredential(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Setup existing credential
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", Category: "Cloud", URL: "https://old-url.com", Notes: "Old notes", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	// Track callback invocation
	callbackInvoked := false
	state.SetOnCredentialsChanged(func() {
		callbackInvoked = true
	})

	// Update credential with all fields using UpdateCredentialOpts
	newuser := "newuser"
	newpass := []byte("newpass") // T020d: Convert to []byte
	newCategory := "Updated Category"
	newURL := "https://new-url.com"
	newNotes := "Updated notes"
	err := state.UpdateCredential("AWS", UpdateCredentialOpts{
		Username: &newuser,
		Password: &newpass,
		Category: &newCategory,
		URL:      &newURL,
		Notes:    &newNotes,
	})
	if err != nil {
		t.Fatalf("UpdateCredential failed: %v", err)
	}

	// Verify callback invoked
	if !callbackInvoked {
		t.Error("onCredentialsChanged callback was not invoked")
	}

	// Verify credential updated in mock vault
	if mockVault.updateCalled != 1 {
		t.Errorf("Expected UpdateCredential called 1 time, got %d", mockVault.updateCalled)
	}

	// Verify state updated
	creds := state.GetCredentials()
	if len(creds) != 1 {
		t.Errorf("Expected 1 credential, got %d", len(creds))
	}
	if creds[0].Username != "newuser" {
		t.Errorf("Expected username 'newuser', got '%s'", creds[0].Username)
	}
	if creds[0].Category != "Updated Category" {
		t.Errorf("Expected category 'Updated Category', got '%s'", creds[0].Category)
	}
	if creds[0].URL != "https://new-url.com" {
		t.Errorf("Expected URL 'https://new-url.com', got '%s'", creds[0].URL)
	}
	if creds[0].Notes != "Updated notes" {
		t.Errorf("Expected notes 'Updated notes', got '%s'", creds[0].Notes)
	}
}

// TestUpdateCredential_ClearFields verifies clearing fields to empty strings.
func TestUpdateCredential_ClearFields(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Setup existing credential with filled fields
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", Category: "Cloud", URL: "https://aws.amazon.com", Notes: "Important notes", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	// Clear category and notes by passing non-nil pointers to empty strings
	emptyCategory := ""
	emptyNotes := ""
	err := state.UpdateCredential("AWS", UpdateCredentialOpts{
		Category: &emptyCategory,
		Notes:    &emptyNotes,
	})
	if err != nil {
		t.Fatalf("UpdateCredential failed: %v", err)
	}

	// Verify fields were cleared
	creds := state.GetCredentials()
	if len(creds) != 1 {
		t.Fatalf("Expected 1 credential, got %d", len(creds))
	}
	if creds[0].Category != "" {
		t.Errorf("Expected empty category, got '%s'", creds[0].Category)
	}
	if creds[0].Notes != "" {
		t.Errorf("Expected empty notes, got '%s'", creds[0].Notes)
	}
	// Verify other fields unchanged
	if creds[0].Username != "admin" {
		t.Errorf("Expected username 'admin', got '%s'", creds[0].Username)
	}
	if creds[0].URL != "https://aws.amazon.com" {
		t.Errorf("Expected URL 'https://aws.amazon.com', got '%s'", creds[0].URL)
	}
}

// TestDeleteCredential verifies deleting a credential.
func TestDeleteCredential(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Setup existing credentials
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", CreatedAt: time.Now()},
		{Service: "GitHub", Username: "user", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	// Track callback invocation
	callbackInvoked := false
	state.SetOnCredentialsChanged(func() {
		callbackInvoked = true
	})

	// Delete credential
	err := state.DeleteCredential("AWS")
	if err != nil {
		t.Fatalf("DeleteCredential failed: %v", err)
	}

	// Verify callback invoked
	if !callbackInvoked {
		t.Error("onCredentialsChanged callback was not invoked")
	}

	// Verify credential deleted from mock vault
	if mockVault.deleteCalled != 1 {
		t.Errorf("Expected DeleteCredential called 1 time, got %d", mockVault.deleteCalled)
	}

	// Verify state updated
	creds := state.GetCredentials()
	if len(creds) != 1 {
		t.Errorf("Expected 1 credential remaining, got %d", len(creds))
	}
	if creds[0].Service != "GitHub" {
		t.Errorf("Expected remaining service 'GitHub', got '%s'", creds[0].Service)
	}
}

// TestCallbackInvocation_AfterUnlock is the CRITICAL deadlock prevention test.
// It verifies that callbacks are invoked AFTER releasing locks.
func TestCallbackInvocation_AfterUnlock(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Setup mock data
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)

	// CRITICAL TEST: Callback tries to read state (would deadlock if lock not released)
	callbackExecuted := false
	state.SetOnCredentialsChanged(func() {
		// This read would deadlock if callback was invoked while holding lock
		creds := state.GetCredentials()
		if len(creds) > 0 {
			callbackExecuted = true
		}
	})

	// Load credentials (should not deadlock)
	err := state.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials failed: %v", err)
	}

	// Verify callback executed successfully
	if !callbackExecuted {
		t.Error("Callback was not executed or failed to read state")
	}
}

// TestConcurrentAccess verifies thread-safety with concurrent operations.
func TestConcurrentAccess(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Setup initial data
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	// Track callback invocations
	var callbackCount int
	var mu sync.Mutex
	state.SetOnCredentialsChanged(func() {
		mu.Lock()
		callbackCount++
		mu.Unlock()
	})

	// Run concurrent operations
	var wg sync.WaitGroup
	operations := 10

	// Concurrent reads
	for i := 0; i < operations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = state.GetCredentials()
			_ = state.GetCategories()
			_ = state.GetSelectedCategory()
		}()
	}

	// Concurrent writes
	for i := 0; i < operations; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			service := "Service" + string(rune(n))
			_ = state.AddCredential(service, "user", "pass", "", "", "")
		}(i)
	}

	// Wait for all operations to complete
	wg.Wait()

	// Verify callbacks were invoked (at least once per add)
	mu.Lock()
	if callbackCount < operations {
		t.Errorf("Expected at least %d callback invocations, got %d", operations, callbackCount)
	}
	mu.Unlock()
}

// TestSetSelectedCategory verifies category selection with callback.
func TestSetSelectedCategory(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Track callback invocation
	callbackInvoked := false
	state.SetOnSelectionChanged(func() {
		callbackInvoked = true
	})

	// Set selected category
	state.SetSelectedCategory("AWS")

	// Verify callback invoked
	if !callbackInvoked {
		t.Error("onSelectionChanged callback was not invoked")
	}

	// Verify category set
	category := state.GetSelectedCategory()
	if category != "AWS" {
		t.Errorf("Expected category 'AWS', got '%s'", category)
	}
}

// TestSetSelectedCredential verifies credential selection with callback.
func TestSetSelectedCredential(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Track callback invocation
	callbackInvoked := false
	state.SetOnSelectionChanged(func() {
		callbackInvoked = true
	})

	// Set selected credential
	cred := &vault.CredentialMetadata{
		Service:  "AWS",
		Username: "admin",
	}
	state.SetSelectedCredential(cred)

	// Verify callback invoked
	if !callbackInvoked {
		t.Error("onSelectionChanged callback was not invoked")
	}

	// Verify credential set
	selected := state.GetSelectedCredential()
	require.NotNil(t, selected, "Expected selected credential")
	require.Equal(t, "AWS", selected.Service, "Expected service 'AWS'")
}

// TestComponentStorage verifies component storage and retrieval.
func TestComponentStorage(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Create mock components
	sidebar := tview.NewTreeView()
	table := tview.NewTable()
	detailView := tview.NewTextView()
	statusBar := tview.NewTextView()

	// Store components
	state.SetSidebar(sidebar)
	state.SetTable(table)
	state.SetDetailView(detailView)
	state.SetStatusBar(statusBar)

	// Retrieve and verify components
	if state.GetSidebar() != sidebar {
		t.Error("Sidebar component mismatch")
	}
	if state.GetTable() != table {
		t.Error("Table component mismatch")
	}
	if state.GetDetailView() != detailView {
		t.Error("DetailView component mismatch")
	}
	if state.GetStatusBar() != statusBar {
		t.Error("StatusBar component mismatch")
	}
}

// TestGetFullCredential verifies full credential fetching.
func TestGetFullCredential(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Setup mock data
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	// Get full credential
	fullCred, err := state.GetFullCredential("AWS")
	if err != nil {
		t.Fatalf("GetFullCredential failed: %v", err)
	}

	// Verify credential data
	if fullCred.Service != "AWS" {
		t.Errorf("Expected service 'AWS', got '%s'", fullCred.Service)
	}
	// T020d: Compare []byte password
	if string(fullCred.Password) != "mock-password" {
		t.Errorf("Expected password 'mock-password', got '%s'", string(fullCred.Password))
	}

	// Verify vault method called
	if mockVault.getCalled != 1 {
		t.Errorf("Expected GetCredential called 1 time, got %d", mockVault.getCalled)
	}
}

// TestGetCategories verifies category extraction from credentials.
func TestGetCategories(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Setup mock data with various categories
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", Category: "Cloud", CreatedAt: time.Now()},
		{Service: "GitHub", Username: "user", Category: "Development", CreatedAt: time.Now()},
		{Service: "Azure", Username: "admin", Category: "Cloud", CreatedAt: time.Now()},
		{Service: "LocalDB", Username: "root", Category: "", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)

	// Load credentials (triggers updateCategories)
	err := state.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials failed: %v", err)
	}

	// Get categories
	categories := state.GetCategories()

	// Verify categories extracted correctly
	expectedCategories := []string{"Cloud", "Development", "Uncategorized"}
	if len(categories) != len(expectedCategories) {
		t.Errorf("Expected %d categories, got %d", len(expectedCategories), len(categories))
	}

	// Verify specific categories exist
	categoryMap := make(map[string]bool)
	for _, cat := range categories {
		categoryMap[cat] = true
	}

	for _, expected := range expectedCategories {
		if !categoryMap[expected] {
			t.Errorf("Expected category '%s' not found in %v", expected, categories)
		}
	}

	// Verify "Uncategorized" appears for empty category credential
	if !categoryMap["Uncategorized"] {
		t.Error("Expected 'Uncategorized' category for credential with empty category")
	}
}

// TestGetCategories_ReturnsACopy verifies GetCategories returns a copy, not internal slice.
func TestGetCategories_ReturnsACopy(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Setup mock data
	mockCreds := []vault.CredentialMetadata{
		{Service: "AWS", Username: "admin", Category: "Cloud", CreatedAt: time.Now()},
		{Service: "GitHub", Username: "user", Category: "Development", CreatedAt: time.Now()},
	}
	mockVault.SetCredentials(mockCreds)
	_ = state.LoadCredentials()

	// Get categories
	categories1 := state.GetCategories()
	categories2 := state.GetCategories()

	// Verify we got copies (different slice instances)
	if len(categories1) != len(categories2) {
		t.Errorf("Expected same length, got %d and %d", len(categories1), len(categories2))
	}

	// Modify the returned slice
	if len(categories1) > 0 {
		categories1[0] = "Modified"
	}

	// Get categories again and verify internal state wasn't mutated
	categories3 := state.GetCategories()
	if len(categories3) > 0 && categories3[0] == "Modified" {
		t.Error("External modification affected internal state - GetCategories should return a copy")
	}
}

// TestUpdateCategories_EmptyCredentials verifies updateCategories handles empty credential lists.
func TestUpdateCategories_EmptyCredentials(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Load credentials with empty mock data
	err := state.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials failed: %v", err)
	}

	// Get categories
	categories := state.GetCategories()

	// Verify empty slice returned
	if len(categories) != 0 {
		t.Errorf("Expected empty categories slice, got %d categories", len(categories))
	}
}

// TestTriggerFilterChanged verifies onFilterChanged callback fires independently from onSelectionChanged.
func TestTriggerFilterChanged(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Track callback invocations separately
	filterChangedCount := 0
	selectionChangedCount := 0

	state.SetOnFilterChanged(func() {
		filterChangedCount++
	})

	state.SetOnSelectionChanged(func() {
		selectionChangedCount++
	})

	// Trigger filter changed (should only invoke onFilterChanged)
	state.TriggerFilterChanged()

	// Verify only filter callback invoked
	if filterChangedCount != 1 {
		t.Errorf("Expected onFilterChanged called 1 time, got %d", filterChangedCount)
	}
	if selectionChangedCount != 0 {
		t.Errorf("Expected onSelectionChanged not called, got %d", selectionChangedCount)
	}
}

// TestTriggerRefresh verifies TriggerRefresh still invokes onSelectionChanged.
func TestTriggerRefresh(t *testing.T) {
	mockVault := NewMockVaultService()
	state := NewAppState(mockVault)

	// Track callback invocations
	filterChangedCount := 0
	selectionChangedCount := 0

	state.SetOnFilterChanged(func() {
		filterChangedCount++
	})

	state.SetOnSelectionChanged(func() {
		selectionChangedCount++
	})

	// Trigger refresh (should invoke onSelectionChanged but not onFilterChanged)
	state.TriggerRefresh()

	// Verify only selection callback invoked
	if selectionChangedCount != 1 {
		t.Errorf("Expected onSelectionChanged called 1 time, got %d", selectionChangedCount)
	}
	if filterChangedCount != 0 {
		t.Errorf("Expected onFilterChanged not called, got %d", filterChangedCount)
	}
}

// TestAppStateSortState verifies the default sort, field cycling (with wrap),
// and direction toggling.
func TestAppStateSortState(t *testing.T) {
	state := NewAppState(NewMockVaultService())

	// Default is Service ascending.
	require.Equal(t, SortByService, state.GetSortField(), "default sort field")
	require.True(t, state.GetSortAscending(), "default should be ascending")

	// Cycling advances Service -> Username -> Last Used -> Service.
	want := []SortField{SortByUsername, SortByLastUsed, SortByService}
	for i, w := range want {
		state.CycleSortField()
		require.Equalf(t, w, state.GetSortField(), "after %d cycle(s)", i+1)
	}

	// Toggling flips direction and back.
	state.ToggleSortDirection()
	require.False(t, state.GetSortAscending(), "after one toggle")
	state.ToggleSortDirection()
	require.True(t, state.GetSortAscending(), "after two toggles")
}

// TestSortFieldString verifies the labels used in the table title.
func TestSortFieldString(t *testing.T) {
	require.Equal(t, "Service", SortByService.String())
	require.Equal(t, "Username", SortByUsername.String())
	require.Equal(t, "Last Used", SortByLastUsed.String())
}
