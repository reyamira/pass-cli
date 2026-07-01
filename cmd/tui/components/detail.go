package components

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/arimxyer/pass-cli/cmd/tui/models"
	"github.com/arimxyer/pass-cli/cmd/tui/styles"
	"github.com/arimxyer/pass-cli/internal/vault"

	"github.com/atotto/clipboard"
	"github.com/rivo/tview"
)

const (
	// UI separator for detail panel sections (with small indent for visual balance)
	detailSeparator = "  ═══════════════════════════════════\n"

	// Width of detail separator for centering calculations (excludes indent)
	detailSeparatorWidth = 35

	// Maximum path length before truncation in usage locations
	maxPathDisplayLength = 60

	// Hybrid timestamp threshold: switch from relative to absolute format
	timestampHybridThreshold = 7 * 24 * time.Hour
)

// DetailView displays full credential information with password masking and copy support.
// Wraps tview.TextView with credential formatting and clipboard integration.
type DetailView struct {
	*tview.TextView

	appState                *models.AppState
	passwordVisible         bool   // Toggle for password visibility (false = masked)
	totpVisible             bool   // Toggle for TOTP code visibility (false = hidden)
	cachedCredentialService string // Cache last refreshed credential service to avoid unnecessary vault calls
}

// NewDetailView creates and configures a new DetailView component.
// Password is masked by default for security.
func NewDetailView(appState *models.AppState) *DetailView {
	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWordWrap(true)

	dv := &DetailView{
		TextView:        textView,
		appState:        appState,
		passwordVisible: false, // Default to masked for security
	}

	dv.applyStyles()
	dv.Refresh()

	return dv
}

// Refresh rebuilds the detail view from the currently selected credential.
// Displays formatted credential information or empty state if no selection.
// Uses caching to avoid expensive vault operations when the same credential is refreshed.
func (dv *DetailView) Refresh() {
	// Debug: Uncomment to trace selection changes
	// fmt.Printf("[DetailView] Refresh called, selected: %v\n", dv.appState.GetSelectedCredential())

	cred := dv.appState.GetSelectedCredential()

	if cred == nil {
		dv.cachedCredentialService = "" // Clear cache
		dv.showEmptyState()
		return
	}

	// Cache optimization: skip refresh if same credential and not password toggle
	if cred.Service == dv.cachedCredentialService {
		// Same credential already displayed, no need to rebuild
		return
	}

	// Update cache
	dv.cachedCredentialService = cred.Service

	content := dv.formatCredential(cred)
	dv.SetText(content)
	dv.ScrollToBeginning()
}

// formatCredential creates formatted text display for a credential.
// Uses color tags with explicit backgrounds to support themes while using SetDynamicColors(true).
func (dv *DetailView) formatCredential(cred *vault.CredentialMetadata) string {
	var b strings.Builder

	// Header with service name
	b.WriteString(separator())
	b.WriteString(fmt.Sprintf("%sService (UID):%s %s\n", colorWithBg("lightSlateGray"), colorWithBg("yellow"), cred.Service))
	b.WriteString(separator())
	b.WriteString(textColor() + "\n")

	// Main credential fields
	b.WriteString(fmt.Sprintf("%sUsername:%s   %s\n", colorWithBg("lightSlateGray"), textColor(), cred.Username))

	// Category (if present)
	if cred.Category != "" {
		b.WriteString(fmt.Sprintf("%sCategory:%s   %s\n", colorWithBg("lightSlateGray"), textColor(), cred.Category))
	}

	// URL (if present)
	if cred.URL != "" {
		b.WriteString(fmt.Sprintf("%sURL:%s        %s\n", colorWithBg("lightSlateGray"), textColor(), cred.URL))
	}

	// Password field with masking
	dv.formatPasswordField(&b, cred)

	// TOTP field (if configured)
	if cred.HasTOTP {
		dv.formatTOTPField(&b, cred)
	}

	// Notes (if present)
	if cred.Notes != "" {
		b.WriteString(fmt.Sprintf("\n%sNotes:%s\n", colorWithBg("lightSlateGray"), textColor()))
		// Indent multi-line notes
		indentedNotes := strings.ReplaceAll(cred.Notes, "\n", "\n  ")
		b.WriteString(fmt.Sprintf("  %s\n", indentedNotes))
	}

	// Metadata section
	b.WriteString(textColor() + "\n")
	b.WriteString(separator())
	b.WriteString(fmt.Sprintf("%s%s%s\n", colorWithBg("yellow"), centerText("Metadata"), textColor()))
	b.WriteString(separator())
	b.WriteString(textColor() + "\n")

	b.WriteString(fmt.Sprintf("%sCreated:%s     %s\n", colorWithBg("lightSlateGray"), textColor(), cred.CreatedAt.Format("2006-01-02 03:04 PM")))
	b.WriteString(fmt.Sprintf("%sModified:%s    %s\n", colorWithBg("lightSlateGray"), textColor(), cred.UpdatedAt.Format("2006-01-02 03:04 PM")))

	// Display modification count
	if cred.ModifiedCount > 0 {
		timesText := "time"
		if cred.ModifiedCount > 1 {
			timesText = "times"
		}
		b.WriteString(fmt.Sprintf("%s# Modified:%s  %d %s\n", colorWithBg("lightSlateGray"), textColor(), cred.ModifiedCount, timesText))
	}

	if !cred.LastAccessed.IsZero() {
		relativeTime := formatRelativeTime(cred.LastAccessed)
		b.WriteString(fmt.Sprintf("%sLast Used:%s   %s\n", colorWithBg("lightSlateGray"), textColor(), relativeTime))
	}

	if cred.UsageCount > 0 {
		b.WriteString(fmt.Sprintf("%sUsage Count:%s %d times\n", colorWithBg("lightSlateGray"), textColor(), cred.UsageCount))
	}

	// Locations (if any)
	if len(cred.Locations) > 0 {
		b.WriteString(fmt.Sprintf("%sLocations:%s  %d unique locations\n", colorWithBg("lightSlateGray"), textColor(), len(cred.Locations)))
	}

	// T047: Integrate usage locations display into detail panel
	// Fetch full credential to get UsageRecord data
	fullCred, err := dv.appState.GetFullCredential(cred.Service)
	if err == nil && fullCred != nil {
		// Append usage locations section
		usageSection := FormatUsageLocations(fullCred)
		b.WriteString(usageSection)
	}

	return b.String()
}

// formatTOTPField adds the TOTP field with optional code display and toggle hint.
// When visible, fetches and displays current TOTP code with remaining seconds.
func (dv *DetailView) formatTOTPField(b *strings.Builder, cred *vault.CredentialMetadata) {
	issuerDisplay := cred.TOTPIssuer
	if issuerDisplay == "" {
		issuerDisplay = "configured"
	}

	if dv.totpVisible {
		code, remaining, err := dv.appState.GetTOTPCode(cred.Service)
		if err == nil {
			fmt.Fprintf(b, "%sTOTP:%s       %s  %s[%s]%s  %s(%ds remaining, 'T' to hide, 't' to copy)%s\n",
				colorWithBg("lightSlateGray"), textColor(), issuerDisplay,
				colorWithBg("green"), code, textColor(),
				colorWithBg("lightSlateGray"), remaining, textColor())
		} else {
			fmt.Fprintf(b, "%sTOTP:%s       %s  %sError: %v%s\n",
				colorWithBg("lightSlateGray"), textColor(), issuerDisplay,
				colorWithBg("red"), err, textColor())
		}
	} else {
		fmt.Fprintf(b, "%sTOTP:%s       %s  %s('T' to reveal, 't' to copy)%s\n",
			colorWithBg("lightSlateGray"), textColor(), issuerDisplay,
			colorWithBg("lightSlateGray"), textColor())
	}
}

// formatPasswordField adds the password field with masking and toggle hint.
// Fetches full credential to display password when visible.
func (dv *DetailView) formatPasswordField(b *strings.Builder, cred *vault.CredentialMetadata) {
	password := "********" // Default masked display
	hint := fmt.Sprintf("  %s(Press 'p' to reveal)%s", colorWithBg("lightSlateGray"), textColor())

	if dv.passwordVisible {
		// Fetch full credential to get password
		fullCred, err := dv.appState.GetFullCredential(cred.Service)
		if err == nil && fullCred != nil {
			// T020d: Convert []byte to string for display
			password = string(fullCred.Password)
			hint = fmt.Sprintf("  %s(Press 'p' to hide)%s", colorWithBg("lightSlateGray"), textColor())
		} else {
			password = fmt.Sprintf("%sError loading password%s", colorWithBg("red"), textColor()) // #nosec G101 -- UI error message, not actual credentials
			hint = ""
		}
	}

	fmt.Fprintf(b, "%sPassword:%s   %s%s\n", colorWithBg("lightSlateGray"), textColor(), password, hint)
}

// showEmptyState displays a message when no credential is selected.
func (dv *DetailView) showEmptyState() {
	content := fmt.Sprintf(`%s
%s
        %sNo Credential Selected%s

    Select a credential from the list
    to view its details.

%s
`, textColor(), separator(), colorWithBg("lightSlateGray"), textColor(), separator())
	dv.SetText(content)
}

// TogglePasswordVisibility toggles the password display state and refreshes.
// Alternates between masked (••••••••) and plaintext display.
// Invalidates cache to force refresh with new password visibility state.
func (dv *DetailView) TogglePasswordVisibility() {
	dv.passwordVisible = !dv.passwordVisible
	dv.cachedCredentialService = "" // Invalidate cache to force refresh
	dv.Refresh()
}

// ToggleTOTPVisibility toggles the TOTP code display state and refreshes.
// When visible, shows the current 6-digit code with remaining seconds.
// Invalidates cache to force refresh with new TOTP visibility state.
func (dv *DetailView) ToggleTOTPVisibility() {
	dv.totpVisible = !dv.totpVisible
	dv.cachedCredentialService = "" // Invalidate cache to force refresh
	dv.Refresh()
}

// CopyPasswordToClipboard copies the selected credential's password to clipboard.
// Returns error if no credential selected or clipboard operation fails.
// T020g: Added explicit memory zeroing after clipboard write
func (dv *DetailView) CopyPasswordToClipboard() error {
	cred := dv.appState.GetSelectedCredential()
	if cred == nil {
		return fmt.Errorf("no credential selected")
	}

	// Fetch full credential to get password
	fullCred, err := dv.appState.GetFullCredential(cred.Service)
	if err != nil {
		return fmt.Errorf("failed to get credential: %w", err)
	}

	// T020g: Convert []byte to string for clipboard, then immediately zero the byte slice
	passwordStr := string(fullCred.Password)

	// Copy password to clipboard
	err = clipboard.WriteAll(passwordStr)
	if err != nil {
		return fmt.Errorf("failed to copy to clipboard: %w", err)
	}

	// Track password access (copy to clipboard = usage)
	if err := dv.appState.RecordFieldAccess(cred.Service, "password"); err != nil {
		// Log warning but don't fail the operation
		fmt.Fprintf(os.Stderr, "Warning: failed to track password access: %v\n", err)
	}

	// T020g: Zero the password bytes immediately after clipboard write
	// Note: This only zeros the source []byte in fullCred, not the string copy
	// The string copy is necessary for clipboard API and will be GC'd
	for i := range fullCred.Password {
		fullCred.Password[i] = 0
	}

	return nil
}

// CopyFieldToClipboard copies a specified field to clipboard.
// Supported fields: "username", "password", "url", "notes", "service", "category"
// Returns error if no credential selected, invalid field, or clipboard operation fails.
func (dv *DetailView) CopyFieldToClipboard(field string) error {
	cred := dv.appState.GetSelectedCredential()
	if cred == nil {
		return fmt.Errorf("no credential selected")
	}

	var value string
	needsFullCred := field == "password" // Only password requires full credential fetch

	if needsFullCred {
		// Fetch full credential for password
		fullCred, err := dv.appState.GetFullCredential(cred.Service)
		if err != nil {
			return fmt.Errorf("failed to get credential: %w", err)
		}
		value = string(fullCred.Password)

		// Zero password bytes after use
		defer func() {
			for i := range fullCred.Password {
				fullCred.Password[i] = 0
			}
		}()
	} else {
		// Get value from credential summary
		switch field {
		case "username":
			value = cred.Username
		case "url":
			value = cred.URL
		case "notes":
			value = cred.Notes
		case "service":
			value = cred.Service
		case "category":
			value = cred.Category
		default:
			return fmt.Errorf("invalid field: %s", field)
		}
	}

	// Check if field is empty
	if value == "" {
		return fmt.Errorf("%s is empty", field)
	}

	// Copy to clipboard
	err := clipboard.WriteAll(value)
	if err != nil {
		return fmt.Errorf("failed to copy to clipboard: %w", err)
	}

	// Track field access
	if err := dv.appState.RecordFieldAccess(cred.Service, field); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to track %s access: %v\n", field, err)
	}

	return nil
}

// CopyTOTPToClipboard generates and copies the TOTP code to clipboard.
// Returns the remaining seconds until the code expires, or error if no TOTP configured.
func (dv *DetailView) CopyTOTPToClipboard() (int, error) {
	cred := dv.appState.GetSelectedCredential()
	if cred == nil {
		return 0, fmt.Errorf("no credential selected")
	}

	if !cred.HasTOTP {
		return 0, fmt.Errorf("no TOTP configured for %s", cred.Service)
	}

	// Get TOTP code from vault service (includes audit logging)
	code, remaining, err := dv.appState.GetTOTPCode(cred.Service)
	if err != nil {
		return 0, fmt.Errorf("failed to generate TOTP code: %w", err)
	}

	// Copy to clipboard
	if err := clipboard.WriteAll(code); err != nil {
		return 0, fmt.Errorf("failed to copy to clipboard: %w", err)
	}

	// Track TOTP access
	if err := dv.appState.RecordFieldAccess(cred.Service, "totp"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to track TOTP access: %v\n", err)
	}

	return remaining, nil
}

// applyStyles applies theme colors and borders to the detail view.
// Uses theme system for consistent styling.
func (dv *DetailView) applyStyles() {
	theme := styles.GetCurrentTheme()
	styles.ApplyBorderedStyle(dv.TextView, "Details", true)
	// Explicitly set background again after border styling to ensure it applies to text area
	dv.SetBackgroundColor(theme.Background)
}

// colorWithBg returns a color tag with theme background to prevent terminal transparency.
// When SetDynamicColors is true, color tags need explicit backgrounds or they show terminal default.
func colorWithBg(colorName string) string {
	theme := styles.GetCurrentTheme()
	r, g, b := theme.Background.RGB()
	bgHex := fmt.Sprintf("#%02x%02x%02x", r, g, b)
	return fmt.Sprintf("[%s:%s]", colorName, bgHex)
}

// textColor returns the theme's primary text color tag with background.
func textColor() string {
	theme := styles.GetCurrentTheme()
	r, g, b := theme.TextPrimary.RGB()
	fgHex := fmt.Sprintf("#%02x%02x%02x", r, g, b)
	rBg, gBg, bBg := theme.Background.RGB()
	bgHex := fmt.Sprintf("#%02x%02x%02x", rBg, gBg, bBg)
	return fmt.Sprintf("[%s:%s]", fgHex, bgHex)
}

// separator returns a styled separator line using theme accent color.
func separator() string {
	return colorWithBg("yellow") + detailSeparator + textColor()
}

// centerText centers text within the separator width by adding leading spaces.
// Includes 2-space indent to match separator.
func centerText(text string) string {
	padding := (detailSeparatorWidth - len(text)) / 2
	if padding < 0 {
		padding = 0
	}
	return "  " + strings.Repeat(" ", padding) + text
}

// ========================================
// Usage Location Display (User Story 3)
// ========================================

// SortUsageLocations converts a map of usage records to a sorted slice.
// Returns records sorted by timestamp in descending order (most recent first).
// T044: Helper function for sorting usage locations by timestamp
func SortUsageLocations(records map[string]vault.UsageRecord) []vault.UsageRecord {
	// Convert map to slice
	locations := make([]vault.UsageRecord, 0, len(records))
	for _, record := range records {
		locations = append(locations, record)
	}

	// Sort by timestamp descending (most recent first)
	sort.Slice(locations, func(i, j int) bool {
		return locations[i].Timestamp.After(locations[j].Timestamp)
	})

	return locations
}

// FormatTimestamp formats a timestamp with hybrid logic for usage locations.
// Returns relative format (<7 days) or absolute date format (≥7 days).
// T045: Helper function implementing hybrid timestamp formatting
//
// Format rules:
//   - Age < 1 hour   → "X minutes ago"
//   - Age < 24 hours → "X hours ago"
//   - Age < 7 days   → "X days ago"
//   - Age >= 7 days  → "YYYY-MM-DD" (absolute date)
func FormatTimestamp(t time.Time) string {
	age := time.Since(t)

	if age < timestampHybridThreshold {
		// Relative format for recent activity
		if age < time.Hour {
			minutes := int(age.Minutes())
			return fmt.Sprintf("%d minutes ago", minutes)
		} else if age < 24*time.Hour {
			hours := int(age.Hours())
			if hours == 1 {
				return "1 hour ago"
			}
			return fmt.Sprintf("%d hours ago", hours)
		} else {
			days := int(age.Hours() / 24)
			if days == 1 {
				return "1 day ago"
			}
			return fmt.Sprintf("%d days ago", days)
		}
	}

	// Absolute format for older activity
	return t.Format("2006-01-02") // YYYY-MM-DD
}

// FormatUsageLocations formats usage location data for display in detail panel.
// Returns formatted string with usage locations, timestamps, git repos, and counts.
// T046: Main formatting function for usage locations display
//
// Handles:
//   - Empty state (no usage records)
//   - Multiple locations sorted by recency
//   - GitRepo display when available
//   - Line numbers when available (format: path:lineNumber)
//   - Long path truncation (T052)
//   - Missing file paths (displays path even if file no longer exists)
func FormatUsageLocations(cred *vault.Credential) string {
	var b strings.Builder

	// Usage Locations section header (T048)
	b.WriteString(textColor() + "\n")
	b.WriteString(separator())
	b.WriteString(fmt.Sprintf("%s%s%s\n", colorWithBg("yellow"), centerText("Usage Locations"), textColor()))
	b.WriteString(separator())
	b.WriteString(textColor() + "\n")

	// Handle empty state (T049)
	if len(cred.UsageRecord) == 0 {
		b.WriteString(fmt.Sprintf("%sNo usage recorded%s\n", colorWithBg("lightSlateGray"), textColor()))
		return b.String()
	}

	// Sort locations by timestamp (most recent first) - T044
	sortedLocations := SortUsageLocations(cred.UsageRecord)

	// Format each location
	for _, record := range sortedLocations {
		// Format path with line number if available (T051)
		path := record.Location
		if record.LineNumber > 0 {
			path = fmt.Sprintf("%s:%d", record.Location, record.LineNumber)
		}

		// Truncate long paths with ellipsis (T052)
		if len(path) > maxPathDisplayLength {
			// Truncate in middle with ellipsis
			half := (maxPathDisplayLength - 3) / 2
			path = path[:half] + "..." + path[len(path)-half:]
		}

		// Format timestamp using hybrid logic (T045)
		timestamp := FormatTimestamp(record.Timestamp)

		// Build location line
		b.WriteString(fmt.Sprintf("  %s%s%s", textColor(), path, textColor()))

		// Add git repo if available (T050)
		if record.GitRepo != "" {
			b.WriteString(fmt.Sprintf(" %s(%s)%s", colorWithBg("lightSlateGray"), record.GitRepo, textColor()))
		}

		// Add timestamp and access count
		b.WriteString(fmt.Sprintf(" %s-%s %s", colorWithBg("lightSlateGray"), textColor(), timestamp))
		// Format total access count
		countText := "1 time"
		if record.Count > 1 {
			countText = fmt.Sprintf("%d times", record.Count)
		}
		b.WriteString(fmt.Sprintf(" %s- accessed%s %s", colorWithBg("lightSlateGray"), textColor(), countText))

		// Show field-level breakdown if available
		if len(record.FieldAccess) > 0 {
			b.WriteString(fmt.Sprintf(" %s(", colorWithBg("lightSlateGray")))
			fieldParts := []string{}
			for field, count := range record.FieldAccess {
				fieldParts = append(fieldParts, fmt.Sprintf("%s:%d", field, count))
			}
			// Sort field names for consistent display (case-insensitive)
			sort.Slice(fieldParts, func(i, j int) bool { return lessFold(fieldParts[i], fieldParts[j]) })
			b.WriteString(strings.Join(fieldParts, ", "))
			b.WriteString(fmt.Sprintf(")%s", textColor()))
		}

		b.WriteString("\n")
	}

	return b.String()
}
