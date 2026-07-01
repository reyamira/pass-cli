// Package models provides application state management for the tview TUI.
// AppState manages all credential data including full metadata (category, URL, notes)
// and provides thread-safe access with proper locking patterns.
package models

import (
	"fmt"
	"sort"
	"sync"

	"github.com/arimxyer/pass-cli/internal/vault"

	"github.com/rivo/tview"
)

// VaultService interface defines the vault operations needed by AppState.
// This interface enables testing with mock implementations.
// T020d: Updated AddCredential signature to accept []byte password
type VaultService interface {
	ListCredentialsWithMetadata() ([]vault.CredentialMetadata, error)
	AddCredential(service, username string, password []byte, category, url, notes string) error // T020d: []byte password
	UpdateCredential(service string, opts vault.UpdateOpts) error
	DeleteCredential(service string) error
	GetCredential(service string, trackUsage bool) (*vault.Credential, error)
	RecordFieldAccess(service, field string) error   // Track field-specific access
	GetTOTPCode(service string) (string, int, error) // Generate TOTP code with remaining seconds
}

// UpdateCredentialOpts mirrors vault.UpdateOpts for AppState layer.
// Using pointer fields allows distinguishing "don't update" (nil) from "clear to empty" (non-nil empty string).
// T020d: Password changed to *[]byte for memory security
type UpdateCredentialOpts struct {
	Username *string
	Password *[]byte // T020d: Changed to *[]byte
	Category *string
	URL      *string
	Notes    *string

	// TOTP fields (nil = don't change, non-nil = set value)
	TOTPSecret    *string
	TOTPAlgorithm *string
	TOTPDigits    *int
	TOTPPeriod    *int
	TOTPIssuer    *string
	ClearTOTP     bool // If true, clears all TOTP fields
}

// AppState holds all application state with thread-safe access.
// This is the single source of truth for the entire TUI.
// SortField identifies which column the credential table is ordered by.
type SortField int

const (
	SortByService SortField = iota
	SortByUsername
	SortByLastUsed
	sortFieldCount // sentinel: number of sortable fields (keep last)
)

// String returns the human-readable label shown in the table title.
func (f SortField) String() string {
	switch f {
	case SortByUsername:
		return "Username"
	case SortByLastUsed:
		return "Last Used"
	default:
		return "Service"
	}
}

type AppState struct {
	// Concurrency control
	mu sync.RWMutex // Protects all fields below

	// Vault service (interface for testability)
	vault VaultService

	// Credential data
	credentials []vault.CredentialMetadata
	categories  []string

	// Current selections
	selectedCategory   string
	selectedCredential *vault.CredentialMetadata

	// UI components (single instances, created once)
	sidebar    *tview.TreeView
	table      *tview.Table
	detailView *tview.TextView
	statusBar  *tview.TextView

	// Search state
	searchState *SearchState

	// Table sort state (session-only; defaults to Service ascending)
	sortField     SortField
	sortAscending bool

	// Write tracking for sync optimization
	hasWriteOperations bool

	// Notification callbacks
	onCredentialsChanged func()      // Called when credentials are loaded/modified
	onSelectionChanged   func()      // Called when selection changes
	onFilterChanged      func()      // Called when filter changes (search/category) without selection change
	onError              func(error) // Called when errors occur
}

// NewSearchState creates a new SearchState instance
func NewSearchState() *SearchState {
	return &SearchState{
		Active:     false,
		Query:      "",
		InputField: nil,
	}
}

// NewAppState creates a new AppState with the given vault service.
func NewAppState(vaultService VaultService) *AppState {
	return &AppState{
		vault:         vaultService,
		credentials:   make([]vault.CredentialMetadata, 0),
		categories:    make([]string, 0),
		searchState:   NewSearchState(),
		sortField:     SortByService,
		sortAscending: true,
	}
}

// GetCredentials returns a copy of the credentials slice (thread-safe read).
func (s *AppState) GetCredentials() []vault.CredentialMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.credentials
}

// GetCategories returns a copy of the categories slice (thread-safe read).
func (s *AppState) GetCategories() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external mutation
	categories := make([]string, len(s.categories))
	copy(categories, s.categories)
	return categories
}

// GetSelectedCredential returns a copy of the selected credential (thread-safe read).
func (s *AppState) GetSelectedCredential() *vault.CredentialMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selectedCredential
}

// GetSelectedCategory returns the selected category (thread-safe read).
func (s *AppState) GetSelectedCategory() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selectedCategory
}

// FindCredentialByService searches for a credential by service name (thread-safe read).
// Returns the credential metadata and true if found, nil and false otherwise.
func (s *AppState) FindCredentialByService(service string) (*vault.CredentialMetadata, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.credentials {
		if s.credentials[i].Service == service {
			return &s.credentials[i], true
		}
	}
	return nil, false
}

// GetFullCredential fetches the full credential including password from vault.
// This is used when password access is needed (display, clipboard).
// SECURITY: Only call when password is actually needed (on-demand fetching).
func (s *AppState) GetFullCredential(service string) (*vault.Credential, error) {
	return s.GetFullCredentialWithTracking(service, true)
}

// GetFullCredentialWithTracking retrieves a credential with optional usage tracking.
// Set track=false to avoid incrementing usage statistics (e.g., for form pre-population).
func (s *AppState) GetFullCredentialWithTracking(service string, track bool) (*vault.Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.vault.GetCredential(service, track)
}

// RecordFieldAccess tracks access to a specific credential field.
// Used to record when fields are actually accessed (e.g., password copied to clipboard).
func (s *AppState) RecordFieldAccess(service, field string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.vault.RecordFieldAccess(service, field)
}

// GetTOTPCode generates a TOTP code for the specified service.
// Returns the code, remaining seconds until expiration, and any error.
func (s *AppState) GetTOTPCode(service string) (string, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.vault.GetTOTPCode(service)
}

// LoadCredentials loads all credentials from the vault.
// CRITICAL: Follows Lock→Mutate→Unlock→Notify pattern to prevent deadlocks.
func (s *AppState) LoadCredentials() error {
	s.mu.Lock()

	// Load credentials from vault
	creds, err := s.vault.ListCredentialsWithMetadata()
	if err != nil {
		wrappedErr := fmt.Errorf("failed to load credentials: %w", err)
		s.mu.Unlock()             // ✅ RELEASE LOCK FIRST
		s.notifyError(wrappedErr) // ✅ THEN notify
		return wrappedErr
	}

	// Update state
	s.credentials = creds
	s.updateCategories() // Internal helper, safe to call while locked

	s.mu.Unlock()                // ✅ RELEASE LOCK
	s.notifyCredentialsChanged() // ✅ THEN notify

	return nil
}

// AddCredential adds a new credential to the vault.
// CRITICAL: Minimizes lock duration by releasing lock during vault I/O operations.
// T020d: Converts string password to []byte for vault storage
func (s *AppState) AddCredential(service, username, password, category, url, notes string) error {
	// T020d: Convert string password to []byte for vault
	passwordBytes := []byte(password)

	// Perform vault I/O without holding lock (vault has its own synchronization)
	err := s.vault.AddCredential(service, username, passwordBytes, category, url, notes)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to add credential: %w", err)
		s.notifyError(wrappedErr)
		return wrappedErr
	}

	s.MarkWriteOperation()

	// Reload credentials without holding lock
	creds, err := s.vault.ListCredentialsWithMetadata()
	if err != nil {
		wrappedErr := fmt.Errorf("failed to reload credentials: %w", err)
		s.notifyError(wrappedErr)
		return wrappedErr
	}

	// Only lock to update state
	s.mu.Lock()
	s.credentials = creds
	s.updateCategories() // Update categories while locked
	s.mu.Unlock()

	// Notify after releasing lock
	s.notifyCredentialsChanged()

	return nil
}

// UpdateCredential updates an existing credential in the vault.
// CRITICAL: Minimizes lock duration by releasing lock during vault I/O operations.
// Accepts UpdateCredentialOpts to allow clearing fields to empty strings (non-nil pointer to empty string).
func (s *AppState) UpdateCredential(service string, opts UpdateCredentialOpts) error {
	// Convert AppState UpdateCredentialOpts to vault.UpdateOpts
	vaultOpts := vault.UpdateOpts{
		Username:      opts.Username,
		Password:      opts.Password,
		Category:      opts.Category,
		URL:           opts.URL,
		Notes:         opts.Notes,
		TOTPSecret:    opts.TOTPSecret,
		TOTPAlgorithm: opts.TOTPAlgorithm,
		TOTPDigits:    opts.TOTPDigits,
		TOTPPeriod:    opts.TOTPPeriod,
		TOTPIssuer:    opts.TOTPIssuer,
		ClearTOTP:     opts.ClearTOTP,
	}

	// Perform vault I/O without holding lock (vault has its own synchronization)
	err := s.vault.UpdateCredential(service, vaultOpts)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to update credential: %w", err)
		s.notifyError(wrappedErr)
		return wrappedErr
	}

	s.MarkWriteOperation()

	// Reload credentials without holding lock
	creds, err := s.vault.ListCredentialsWithMetadata()
	if err != nil {
		wrappedErr := fmt.Errorf("failed to reload credentials: %w", err)
		s.notifyError(wrappedErr)
		return wrappedErr
	}

	// Only lock to update state
	s.mu.Lock()
	s.credentials = creds
	s.updateCategories() // Update categories while locked
	s.mu.Unlock()

	// Notify after releasing lock
	s.notifyCredentialsChanged()

	return nil
}

// DeleteCredential deletes a credential from the vault.
// CRITICAL: Minimizes lock duration by releasing lock during vault I/O operations.
func (s *AppState) DeleteCredential(service string) error {
	// Perform vault I/O without holding lock (vault has its own synchronization)
	err := s.vault.DeleteCredential(service)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to delete credential: %w", err)
		s.notifyError(wrappedErr)
		return wrappedErr
	}

	s.MarkWriteOperation()

	// Reload credentials without holding lock
	creds, err := s.vault.ListCredentialsWithMetadata()
	if err != nil {
		wrappedErr := fmt.Errorf("failed to reload credentials: %w", err)
		s.notifyError(wrappedErr)
		return wrappedErr
	}

	// Only lock to update state
	s.mu.Lock()
	s.credentials = creds
	s.updateCategories() // Update categories while locked
	s.mu.Unlock()

	// Notify after releasing lock
	s.notifyCredentialsChanged()

	return nil
}

// SetSelectedCategory updates the selected category.
// CRITICAL: Follows Lock→Mutate→Unlock→Notify pattern.
func (s *AppState) SetSelectedCategory(category string) {
	s.mu.Lock()
	s.selectedCategory = category
	s.mu.Unlock() // ✅ RELEASE LOCK

	s.notifySelectionChanged() // ✅ THEN notify
}

// SetSelectedCredential updates the selected credential.
// CRITICAL: Follows Lock→Mutate→Unlock→Notify pattern.
// Short-circuits if the selection hasn't actually changed (same service).
func (s *AppState) SetSelectedCredential(credential *vault.CredentialMetadata) {
	s.mu.Lock()
	// Debug: Uncomment to trace selection changes
	// fmt.Printf("[AppState] SetSelectedCredential called: %v\n", credential)

	// Short-circuit: Skip notification if selection hasn't changed
	// Compare by service name since that's the unique identifier
	if s.selectedCredential != nil && credential != nil && s.selectedCredential.Service == credential.Service {
		s.mu.Unlock()
		return // Same credential already selected, no notification needed
	}

	// Also short-circuit if both are nil
	if s.selectedCredential == nil && credential == nil {
		s.mu.Unlock()
		return
	}

	s.selectedCredential = credential
	s.mu.Unlock() // ✅ RELEASE LOCK

	s.notifySelectionChanged() // ✅ THEN notify (only when selection actually changed)
}

// SetSelection atomically updates both category and credential selection.
// This optimized method issues a single notification instead of two separate ones.
// CRITICAL: Follows Lock→Mutate→Unlock→Notify pattern.
func (s *AppState) SetSelection(category string, credential *vault.CredentialMetadata) {
	s.mu.Lock()
	s.selectedCategory = category
	s.selectedCredential = credential
	s.mu.Unlock() // ✅ RELEASE LOCK

	s.notifySelectionChanged() // ✅ THEN notify (single notification)
}

// SetSidebar stores the sidebar component reference.
func (s *AppState) SetSidebar(sidebar *tview.TreeView) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sidebar = sidebar
}

// GetSidebar retrieves the sidebar component reference.
func (s *AppState) GetSidebar() *tview.TreeView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sidebar
}

// SetTable stores the table component reference.
func (s *AppState) SetTable(table *tview.Table) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.table = table
}

// GetTable retrieves the table component reference.
func (s *AppState) GetTable() *tview.Table {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.table
}

// SetDetailView stores the detail view component reference.
func (s *AppState) SetDetailView(view *tview.TextView) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detailView = view
}

// GetDetailView retrieves the detail view component reference.
func (s *AppState) GetDetailView() *tview.TextView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.detailView
}

// SetStatusBar stores the status bar component reference.
func (s *AppState) SetStatusBar(bar *tview.TextView) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusBar = bar
}

// GetStatusBar retrieves the status bar component reference.
func (s *AppState) GetStatusBar() *tview.TextView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statusBar
}

// SetSearchState stores the search state reference.
func (s *AppState) SetSearchState(searchState *SearchState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.searchState = searchState
}

// GetSearchState retrieves the search state reference.
func (s *AppState) GetSearchState() *SearchState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.searchState
}

// GetSortField returns the field the credential table is currently sorted by.
func (s *AppState) GetSortField() SortField {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sortField
}

// GetSortAscending reports whether the current sort is ascending.
func (s *AppState) GetSortAscending() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sortAscending
}

// CycleSortField advances the sort field to the next column, wrapping around.
func (s *AppState) CycleSortField() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sortField = (s.sortField + 1) % sortFieldCount
}

// ToggleSortDirection flips the sort between ascending and descending.
func (s *AppState) ToggleSortDirection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sortAscending = !s.sortAscending
}

// SetOnCredentialsChanged registers a callback for credential changes.
func (s *AppState) SetOnCredentialsChanged(callback func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onCredentialsChanged = callback
}

// SetOnSelectionChanged registers a callback for selection changes.
func (s *AppState) SetOnSelectionChanged(callback func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSelectionChanged = callback
}

// SetOnError registers a callback for errors.
func (s *AppState) SetOnError(callback func(error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onError = callback
}

// SetOnFilterChanged registers a callback for filter changes (search/category).
// Used to refresh table without refreshing detail view during search.
func (s *AppState) SetOnFilterChanged(callback func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onFilterChanged = callback
}

// notifyCredentialsChanged invokes the credentials changed callback.
// CRITICAL: Must be called AFTER releasing locks to prevent deadlocks.
func (s *AppState) notifyCredentialsChanged() {
	// Read callback without holding lock
	s.mu.RLock()
	callback := s.onCredentialsChanged
	s.mu.RUnlock()

	if callback != nil {
		callback()
	}
}

// notifySelectionChanged invokes the selection changed callback.
// CRITICAL: Must be called AFTER releasing locks to prevent deadlocks.
func (s *AppState) notifySelectionChanged() {
	// Read callback without holding lock
	s.mu.RLock()
	callback := s.onSelectionChanged
	s.mu.RUnlock()

	// Debug: Uncomment to trace callback invocation
	// fmt.Printf("[AppState] notifySelectionChanged: callback=%v\n", callback != nil)

	if callback != nil {
		callback()
	}
}

// TriggerRefresh manually triggers the selection changed callback to refresh UI components.
// DEPRECATED: Use TriggerFilterChanged for search/filter updates to avoid unnecessary detail view refreshes.
func (s *AppState) TriggerRefresh() {
	s.notifySelectionChanged()
}

// TriggerFilterChanged triggers only the filter changed callback (not selection changed).
// Used by search filtering to update table display without refreshing detail view.
func (s *AppState) TriggerFilterChanged() {
	s.notifyFilterChanged()
}

// notifyFilterChanged invokes the filter changed callback.
// CRITICAL: Must be called AFTER releasing locks to prevent deadlocks.
func (s *AppState) notifyFilterChanged() {
	// Read callback without holding lock
	s.mu.RLock()
	callback := s.onFilterChanged
	s.mu.RUnlock()

	if callback != nil {
		callback()
	}
}

// notifyError invokes the error callback.
// CRITICAL: Must be called AFTER releasing locks to prevent deadlocks.
func (s *AppState) notifyError(err error) {
	// Read callback without holding lock
	s.mu.RLock()
	callback := s.onError
	s.mu.RUnlock()

	if callback != nil {
		callback(err)
	}
}

// MarkWriteOperation records that a write operation occurred during this session.
func (s *AppState) MarkWriteOperation() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasWriteOperations = true
}

// HasWriteOperations returns true if any write operations occurred during this session.
func (s *AppState) HasWriteOperations() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasWriteOperations
}

// updateCategories extracts unique categories from credentials.
// CRITICAL: Must be called while holding a write lock.
func (s *AppState) updateCategories() {
	categoryMap := make(map[string]bool)

	for _, cred := range s.credentials {
		// Extract category from credential's Category field
		if cred.Category != "" {
			categoryMap[cred.Category] = true
		} else {
			// Empty category becomes "Uncategorized"
			categoryMap["Uncategorized"] = true
		}
	}

	// Convert map to sorted slice
	categories := make([]string, 0, len(categoryMap))
	for category := range categoryMap {
		categories = append(categories, category)
	}
	sort.Strings(categories)

	s.categories = categories
}
