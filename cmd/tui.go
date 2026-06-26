package cmd

import (
	"fmt"
	"os"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"

	"github.com/arimxyer/pass-cli/cmd/tui/components"
	"github.com/arimxyer/pass-cli/cmd/tui/events"
	"github.com/arimxyer/pass-cli/cmd/tui/layout"
	"github.com/arimxyer/pass-cli/cmd/tui/models"
	"github.com/arimxyer/pass-cli/cmd/tui/styles"
	"github.com/arimxyer/pass-cli/internal/config"
	"github.com/arimxyer/pass-cli/internal/vault"
)

var tuiCmd = &cobra.Command{
	Use:     "tui",
	GroupID: "utilities",
	Short:   "Launch interactive TUI dashboard",
	Long: `Launch an interactive terminal user interface (TUI) for managing credentials.

The TUI provides a visual interface with keyboard shortcuts for:
  • Browsing credentials by category
  • Viewing credential details
  • Adding, editing, and deleting credentials
  • Copying passwords to clipboard
  • Toggling password visibility

Keyboard shortcuts:
  a - New credential
  e - Edit credential
  d - Delete credential
  c - Copy password
  p - Toggle password visibility
  i - Toggle detail panel
  s - Toggle sidebar
  / - Search/filter
  ? - Show help
  Tab - Cycle focus between panels
  q - Quit

The TUI will automatically unlock the vault using the system keychain if available,
otherwise it will prompt for the master password.`,
	Run: runTUI,
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

func runTUI(cmd *cobra.Command, args []string) {
	// Get vault path
	vaultPath := GetVaultPath()

	// Initialize vault service
	vaultService, err := vault.New(vaultPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize vault: %v\n", err)
		os.Exit(1)
	}

	// Smart sync pull before unlock. Delegates to the shared helper so the TUI
	// honors --offline (skip sync entirely) and shows the same feedback as the CLI.
	syncPullBeforeUnlock(vaultService)

	// Try keychain unlock first
	err = vaultService.UnlockWithKeychain()
	if err != nil {
		// Keychain failed, prompt for password
		password, err := promptForMasterPassword()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to read password: %v\n", err)
			os.Exit(1)
		}

		err = vaultService.Unlock(password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to unlock vault: %v\n", err)
			os.Exit(1)
		}
	}

	// Launch TUI
	if err := launchTUI(vaultService); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}

// launchTUI initializes and runs the TUI application
func launchTUI(vaultService *vault.VaultService) error {
	// Set rounded borders globally
	styles.SetRoundedBorders()

	// Load user configuration
	cfg, _ := config.Load()

	// Create tview application
	app := createTUIApp()

	// Defer terminal restoration for panic recovery
	defer restoreTerminal()

	// Initialize AppState with vault service
	appState := models.NewAppState(vaultService)

	// Load credentials
	if err := appState.LoadCredentials(); err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}

	// Create UI components
	sidebar := components.NewSidebar(appState)
	table := components.NewCredentialTable(appState)
	detailView := components.NewDetailView(appState)
	statusBar := components.NewStatusBar(app, appState, cfg)

	// Store components in AppState
	appState.SetSidebar(sidebar.TreeView)
	appState.SetTable(table.Table)
	appState.SetDetailView(detailView.TextView)
	appState.SetStatusBar(statusBar.TextView)

	// Register callbacks
	appState.SetOnCredentialsChanged(func() {
		// Refresh all components that depend on credentials
		sidebar.Refresh()
		table.Refresh()
		detailView.Refresh()
	})

	appState.SetOnSelectionChanged(func() {
		// Refresh detail view when selection changes
		detailView.Refresh()
	})

	appState.SetOnError(func(err error) {
		// Display error in status bar
		statusBar.ShowError(err)
	})

	// Create NavigationState
	nav := models.NewNavigationState(app, appState)

	// Register focus change callback to update statusbar
	nav.SetOnFocusChanged(func(focus models.FocusableComponent) {
		events.OnFocusChanged(focus, statusBar)
	})

	// Create LayoutManager and build layout
	layoutMgr := layout.NewLayoutManager(app, appState, cfg)
	mainLayout := layoutMgr.CreateMainLayout()

	// Create PageManager
	pageManager := layout.NewPageManager(app)

	// Create EventHandler and setup shortcuts
	eventHandler := events.NewEventHandler(app, appState, nav, pageManager, statusBar, detailView, layoutMgr, cfg)
	eventHandler.SetupGlobalShortcuts()

	// Set root primitive (use pages for modal support over main layout)
	pageManager.ShowPage("main", mainLayout)
	app.SetRoot(pageManager.Pages, true)

	// Run application (blocking)
	runErr := app.Run()

	// Push any changes made during the TUI session (only if writes occurred).
	// syncPushAfterCommand handles the --offline gate, the sync-enabled check, and
	// the "Syncing..." feedback — keeping TUI and CLI push behavior identical.
	if appState.HasWriteOperations() {
		syncPushAfterCommand(vaultService)
	}

	return runErr
}

// promptForMasterPassword prompts the user for the master password
func promptForMasterPassword() ([]byte, error) {
	fmt.Print("Enter master password: ")
	return readPassword()
}

// createTUIApp creates and configures a new tview.Application
func createTUIApp() *tview.Application {
	app := tview.NewApplication()
	app.EnableMouse(true)
	return app
}

// restoreTerminal performs emergency terminal restoration in case of panic
func restoreTerminal() {
	if r := recover(); r != nil {
		// Attempt to restore terminal state
		screen, err := tcell.NewScreen()
		if err == nil {
			screen.Fini()
		}
		fmt.Fprintf(os.Stderr, "Panic: %v\n", r)
		os.Exit(1)
	}
}
