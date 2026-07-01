package components

import (
	"sort"
	"strings"

	"github.com/arimxyer/pass-cli/cmd/tui/models"
	"github.com/arimxyer/pass-cli/cmd/tui/styles"
	"github.com/arimxyer/pass-cli/internal/vault"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// lessFold reports whether a should sort before b, compared case-insensitively.
// It falls back to a case-sensitive comparison as a tie-break so that strings
// differing only in case keep a stable, deterministic order.
func lessFold(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	if la != lb {
		return la < lb
	}
	return a < b
}

// NodeReference identifies the type and value of a tree node.
// Used to distinguish categories from credentials without relying on tree position.
type NodeReference struct {
	Kind  string // "category" or "credential"
	Value string // Category name or service name
}

// Sidebar wraps tview.TreeView to display credential categories.
// Provides category navigation with "All Credentials" root and category children.
type Sidebar struct {
	*tview.TreeView

	appState *models.AppState
	rootNode *tview.TreeNode
}

// NewSidebar creates and configures a new Sidebar component.
// Creates TreeView with root "All Credentials" node and builds initial tree.
func NewSidebar(appState *models.AppState) *Sidebar {
	theme := styles.GetCurrentTheme()

	// Create root node with theme background
	rootStyle := tcell.StyleDefault.
		Foreground(theme.BorderColor).
		Background(theme.Background)
	root := tview.NewTreeNode("All Credentials").
		SetTextStyle(rootStyle).
		SetSelectable(true).
		SetExpanded(true)

	// Create tree view
	tree := tview.NewTreeView().
		SetRoot(root).
		SetCurrentNode(root)

	sidebar := &Sidebar{
		TreeView: tree,
		appState: appState,
		rootNode: root,
	}

	// Apply styling
	sidebar.applyStyles()

	// Setup selection handlers
	// SetChangedFunc handles arrow key navigation
	sidebar.SetChangedFunc(func(node *tview.TreeNode) {
		if node != nil {
			sidebar.onSelect(node)
		}
	})
	// SetSelectedFunc handles Enter/Space/Click activation
	sidebar.SetSelectedFunc(sidebar.onSelect)

	// Initial tree build
	sidebar.Refresh()

	return sidebar
}

// Refresh rebuilds the category tree from current AppState.
// Clears existing children and builds category-grouped tree with credential nodes.
func (s *Sidebar) Refresh() {
	theme := styles.GetCurrentTheme()

	// Get credentials from state (single snapshot for consistency)
	credentials := s.appState.GetCredentials()

	// Clear existing children
	s.rootNode.ClearChildren()

	// Pre-group credentials by category for O(N+C) performance instead of O(C×N)
	groups := make(map[string][]vault.CredentialMetadata)
	for _, cred := range credentials {
		category := cred.Category
		if category == "" {
			category = "Uncategorized"
		}
		groups[category] = append(groups[category], cred)
	}

	// Build category list from groups (avoids snapshot mismatch with credentials)
	categories := make([]string, 0, len(groups))
	for category := range groups {
		categories = append(categories, category)
	}
	// Sort categories alphabetically (case-insensitive)
	sort.Slice(categories, func(i, j int) bool {
		return lessFold(categories[i], categories[j])
	})

	// Build category nodes with credential children
	for _, category := range categories {
		// Create category node with theme background
		categoryStyle := tcell.StyleDefault.
			Foreground(theme.TextPrimary).
			Background(theme.Background)
		categoryNode := tview.NewTreeNode(category).
			SetSelectable(true).
			SetTextStyle(categoryStyle).
			SetReference(NodeReference{Kind: "category", Value: category}).
			SetExpanded(false) // Collapsed by default

		// Sort credentials within category alphabetically (case-insensitive)
		credList := groups[category]
		sort.Slice(credList, func(i, j int) bool {
			return lessFold(credList[i].Service, credList[j].Service)
		})

		// Add credential nodes from sorted list
		credStyle := tcell.StyleDefault.
			Foreground(theme.TextSecondary).
			Background(theme.Background)
		for _, cred := range credList {
			// Create credential node with theme background
			credNode := tview.NewTreeNode(cred.Service).
				SetSelectable(true).
				SetTextStyle(credStyle).
				SetReference(NodeReference{Kind: "credential", Value: cred.Service})

			categoryNode.AddChild(credNode)
		}

		// Add category node to root
		s.rootNode.AddChild(categoryNode)
	}

	// Ensure root is expanded
	s.rootNode.SetExpanded(true)
}

// onSelect handles node selection by updating AppState.
// Root node shows all, category nodes filter by category, credential nodes select specific credential.
func (s *Sidebar) onSelect(node *tview.TreeNode) {
	if node == s.rootNode {
		// Root selected - show all credentials and clear detail view
		// Use SetSelection for atomic update with single notification
		s.appState.SetSelection("", nil)
		return
	}

	// Get node reference to determine type
	ref := node.GetReference()
	if ref == nil {
		// Safety fallback - treat as root
		// Use SetSelection for atomic update with single notification
		s.appState.SetSelection("", nil)
		return
	}

	// Type assert to NodeReference and switch on Kind
	if nodeRef, ok := ref.(NodeReference); ok {
		switch nodeRef.Kind {
		case "category":
			// Category node - filter by category and clear credential selection
			// Use SetSelection for atomic update with single notification
			s.appState.SetSelection(nodeRef.Value, nil)

		case "credential":
			// Credential node - lookup credential by service and select it
			if credMeta, found := s.appState.FindCredentialByService(nodeRef.Value); found {
				// Select specific credential (fresh lookup avoids stale pointers)
				s.appState.SetSelectedCredential(credMeta)
			}

		default:
			// Unknown kind - treat as root
			// Use SetSelection for atomic update with single notification
			s.appState.SetSelection("", nil)
		}
	}
}

// applyStyles applies borders, colors, and title to the sidebar.
// Uses rounded borders with cyan accent color and dark background.
func (s *Sidebar) applyStyles() {
	theme := styles.GetCurrentTheme()
	styles.ApplyBorderedStyle(s.TreeView, "Categories", true)
	// Explicitly set background to ensure it applies to tree area
	s.SetBackgroundColor(theme.Background)
	// Set graphics color for tree structure lines
	s.SetGraphicsColor(theme.TextSecondary)
}
