package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/viper"
)

// Config represents the root configuration object containing all user settings
type Config struct {
	Terminal    TerminalConfig    `mapstructure:"terminal"`
	Keybindings map[string]string `mapstructure:"keybindings"`
	VaultPath   string            `mapstructure:"vault_path"`
	Theme       string            `mapstructure:"theme"`
	Sync        SyncConfig        `mapstructure:"sync"`

	// LoadErrors populated during config loading (not in YAML)
	LoadErrors []string `mapstructure:"-"`

	// ParsedKeybindings stores parsed keybinding objects (populated during Validate)
	ParsedKeybindings map[string]*Keybinding `mapstructure:"-"`
}

// TerminalConfig represents terminal size warning configuration
type TerminalConfig struct {
	WarningEnabled      bool   `mapstructure:"warning_enabled"`
	MinWidth            int    `mapstructure:"min_width"`
	MinHeight           int    `mapstructure:"min_height"`
	DetailPosition      string `mapstructure:"detail_position"`
	DetailAutoThreshold int    `mapstructure:"detail_auto_threshold"`
}

// SyncConfig represents rclone sync configuration for cross-device vault synchronization
type SyncConfig struct {
	Enabled bool   `mapstructure:"enabled"` // Enable/disable sync
	Remote  string `mapstructure:"remote"`  // rclone remote name + path (e.g., "gdrive:.pass-cli")
	// PullTTLSeconds gates the pre-unlock remote probe: within this window a
	// command serves the local vault without a remote round-trip (writes still do
	// a fresh conflict check at push time). 0 uses the built-in default (30s); a
	// negative value disables the gate (probe on every command). The same window
	// also doubles as the failure-backoff: after a probe fails (unreachable/slow
	// remote) the next commands skip the probe until it expires, so a dead remote
	// costs the probe timeout at most once per window instead of on every call.
	PullTTLSeconds int `mapstructure:"pull_ttl_seconds"`
	// ProbeTimeoutSeconds bounds the pre-unlock remote metadata probe (rclone
	// lsjson). A slow-but-alive remote whose probe exceeds this bound is treated
	// as failed and enters the failure-backoff, so users on a high-latency
	// backend can raise it to avoid being misclassified as down. 0 uses the
	// built-in default (8s); a negative value disables the bound (unbounded
	// probe). Only the metadata probe is bounded — the heavy pull/push transfers
	// are always unbounded. Mirrors the pull_ttl_seconds tri-state.
	ProbeTimeoutSeconds int `mapstructure:"probe_timeout_seconds"`
}

// ValidationResult represents the outcome of checking configuration correctness
type ValidationResult struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// ValidationError represents a validation error with context
type ValidationError struct {
	Field   string // e.g., "keybindings.add_credential"
	Message string // e.g., "conflicts with keybindings.delete_credential (both use 'd')"
	Line    int    // Line number in YAML (if available)
}

// ValidationWarning represents a non-fatal validation warning
type ValidationWarning struct {
	Field   string
	Message string
}

// GetDefaults returns the default configuration with hardcoded terminal and keybinding values
func GetDefaults() *Config {
	cfg := &Config{
		Terminal: TerminalConfig{
			WarningEnabled:      true,
			MinWidth:            60,
			MinHeight:           30,
			DetailPosition:      "auto",
			DetailAutoThreshold: 120,
		},
		Keybindings: map[string]string{
			"quit":              "q",
			"add_credential":    "a",
			"edit_credential":   "e",
			"delete_credential": "d",
			"toggle_detail":     "i",
			"toggle_sidebar":    "s",
			"help":              "?",
			"search":            "/",
			"confirm":           "enter",
			"cancel":            "esc",
		},
		Theme: "dracula",
		Sync: SyncConfig{
			Enabled: false,
			Remote:  "",
		},
		LoadErrors: []string{},
	}

	// Validate to populate ParsedKeybindings
	// This ensures defaults are always ready for use
	cfg.Validate()

	return cfg
}

// GetConfigPath returns the config file path in ~/.pass-cli/config.yml
func GetConfigPath() (string, error) {
	// Check for PASS_CLI_CONFIG environment variable first (for testing)
	if envPath := os.Getenv("PASS_CLI_CONFIG"); envPath != "" {
		return envPath, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".pass-cli")

	// Ensure directory exists
	if err := os.MkdirAll(configDir, 0750); err != nil { // More restrictive permissions (owner+group only)
		return "", fmt.Errorf("cannot create config directory: %w", err)
	}

	return filepath.Join(configDir, "config.yml"), nil
}

// GetEditor returns the editor to use for editing config files.
// Checks EDITOR environment variable first, then falls back to OS defaults.
func GetEditor() (string, error) {
	// Check EDITOR environment variable first
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor, nil
	}

	// Platform-specific defaults
	switch runtime.GOOS {
	case "windows":
		return "notepad.exe", nil
	case "darwin", "linux":
		// Check for common editors in order of preference
		for _, ed := range []string{"nano", "vim", "vi"} {
			if _, err := exec.LookPath(ed); err == nil {
				return ed, nil
			}
		}
		return "", fmt.Errorf("no editor found. Please set EDITOR environment variable (e.g., export EDITOR=nano)")
	default:
		return "", fmt.Errorf("unsupported platform for editor detection")
	}
}

// OpenEditor opens the config file in the user's editor.
func OpenEditor(filePath string) error {
	editor, err := GetEditor()
	if err != nil {
		return err
	}

	// #nosec G204 -- editor is user-configured via EDITOR env var or config, intended behavior for CLI tool
	cmd := exec.Command(editor, filePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// GetDefaultConfigTemplate returns the default config file content with comments.
func GetDefaultConfigTemplate() string {
	return `# Pass-CLI Configuration File
#
# This file allows you to customize terminal size warnings, keyboard shortcuts, and theme.
# All settings are optional - missing values will use defaults.

# TUI Theme (valid: dracula, nord, gruvbox, monokai)
# Default: dracula
theme: "dracula"

# Vault Path Configuration (optional)
# Uncomment to use a custom vault location instead of the default (~/.pass-cli/vault.enc)
# Supports: absolute paths, tilde expansion (~), environment variables ($HOME, %USERPROFILE%)
#
# Examples:
#   vault_path: "~/my-vault/vault.enc"           # Tilde expansion
#   vault_path: "/home/user/secure/vault.enc"   # Absolute path (Linux/macOS)
#   vault_path: "C:/Users/me/vault.enc"         # Absolute path (Windows)
#
# vault_path: ""

# Cloud Sync Configuration (optional)
# Sync your vault across devices using rclone (https://rclone.org)
# Requires: rclone installed and configured with a remote
#
# Setup:
#   1. Install rclone: brew install rclone (macOS) or scoop install rclone (Windows)
#   2. Configure remote: rclone config
#   3. Uncomment and set remote below
#
# sync:
#   enabled: true
#   remote: "gdrive:.pass-cli"    # Format: <remote-name>:<path>
#
# Examples:
#   remote: "gdrive:.pass-cli"     # Google Drive
#   remote: "dropbox:Apps/pass-cli" # Dropbox
#   remote: "onedrive:Documents/pass-cli" # OneDrive
#
# See: https://reyamira.github.io/pass-cli/docs/02-guides/sync-guide/

# Terminal size warning configuration
terminal:
  # Enable or disable terminal size warnings (default: true)
  warning_enabled: true
  
  # Minimum terminal width in columns before warning appears (default: 60)
  # Valid range: 1-10000
  min_width: 60
  
  # Minimum terminal height in rows before warning appears (default: 30)
  # Valid range: 1-1000
  min_height: 30

# Keyboard shortcuts
# Format: action: "key" or "modifier+key"
# Valid modifiers: ctrl, alt, shift
# Valid keys: letters, numbers, enter, esc, tab, space, f1-f12
#
# All shortcuts are lowercase (e.g., "ctrl+q" not "CTRL+Q")
keybindings:
  # Application control
  quit: "q"                    # Quit application (with confirmation)
  help: "?"                    # Show help modal with all shortcuts
  
  # Credential management
  add_credential: "a"          # Open form to add new credential
  edit_credential: "e"         # Edit selected credential
  delete_credential: "d"       # Delete selected credential (with confirmation)
  
  # View controls
  toggle_detail: "i"           # Toggle detail panel visibility
  toggle_sidebar: "s"          # Toggle sidebar visibility
  search: "/"                  # Activate search/filter mode
  
  # Form controls (used in modals/dialogs)
  confirm: "enter"             # Confirm action in forms
  cancel: "esc"                # Cancel action in forms

# Example custom keybindings:
#
# Vim-style bindings:
#   add_credential: "i"        # Insert mode
#   search: "/"                # Vim search
#   toggle_sidebar: "ctrl+w"   # Vim window command
#
# Emacs-style bindings:
#   quit: "ctrl+x"
#   search: "ctrl+s"
#   add_credential: "ctrl+n"
#
# Custom modifier keys:
#   quit: "ctrl+q"
#   add_credential: "n"
#   help: "f1"
`
}

// LoadFromPath loads configuration from a specific file path (useful for testing)

// T054: detectUnknownFields checks for unknown fields in the config YAML
func detectUnknownFields(v *viper.Viper) []ValidationWarning {
	var warnings []ValidationWarning

	// Get all keys from the config file
	allKeys := v.AllKeys()

	// Define known fields (all valid config keys)
	knownFields := map[string]bool{
		"terminal":                      true,
		"terminal.warning_enabled":      true,
		"terminal.min_width":            true,
		"terminal.min_height":           true,
		"keybindings":                   true,
		"keybindings.quit":              true,
		"keybindings.add_credential":    true,
		"keybindings.edit_credential":   true,
		"keybindings.delete_credential": true,
		"keybindings.toggle_detail":     true,
		"keybindings.toggle_sidebar":    true,
		"keybindings.help":              true,
		"keybindings.search":            true,
		"keybindings.confirm":           true,
		"keybindings.cancel":            true,
		"theme":                         true,
		"vault_path":                    true,
		"sync":                          true,
		"sync.enabled":                  true,
		"sync.remote":                   true,
	}

	// Check for unknown fields
	for _, key := range allKeys {
		if !knownFields[key] {
			warnings = append(warnings, ValidationWarning{
				Field:   key,
				Message: fmt.Sprintf("unknown field '%s' (will be ignored)", key),
			})
		}
	}

	return warnings
}

// shouldLogConfig returns true if config loading should produce log output.
// Only logs when PASS_CLI_DEBUG is explicitly set to avoid cluttering output.
func shouldLogConfig() bool {
	return os.Getenv("PASS_CLI_DEBUG") == "1"
}

func LoadFromPath(configPath string) (*Config, *ValidationResult) {
	// T051: Log config load attempt
	if shouldLogConfig() {
		fmt.Fprintf(os.Stderr, "[Config] Loading config from: %s\n", configPath)
	}

	// Check if config file exists
	fileInfo, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		// No config file, use defaults (not an error)
		if shouldLogConfig() {
			fmt.Fprintf(os.Stderr, "[Config] No config file found, using defaults\n")
		}
		return GetDefaults(), &ValidationResult{Valid: true}
	}
	if err != nil {
		// T051: Log file access error
		if shouldLogConfig() {
			fmt.Fprintf(os.Stderr, "[Config] Failed to access config file: %v\n", err)
		}
		// File stat error, use defaults
		return GetDefaults(), &ValidationResult{
			Valid: false,
			Errors: []ValidationError{
				{Field: "config_file", Message: fmt.Sprintf("cannot access config file: %v", err)},
			},
		}
	}

	// Check file size limit (100 KB)
	const maxFileSize = 100 * 1024 // 100 KB
	if fileInfo.Size() > maxFileSize {
		// T051: Log file size error
		if shouldLogConfig() {
			fmt.Fprintf(os.Stderr, "[Config] Config file too large: %d KB (max: 100 KB)\n", fileInfo.Size()/1024)
		}
		return GetDefaults(), &ValidationResult{
			Valid: false,
			Errors: []ValidationError{
				{
					Field:   "config_file",
					Message: fmt.Sprintf("config file too large (size: %d KB, max: 100 KB)", fileInfo.Size()/1024),
				},
			},
		}
	}

	// T018+T020: Load YAML with Viper and merge with defaults
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Set defaults for merging
	defaults := GetDefaults()
	v.SetDefault("terminal.warning_enabled", defaults.Terminal.WarningEnabled)
	v.SetDefault("terminal.min_width", defaults.Terminal.MinWidth)
	v.SetDefault("terminal.min_height", defaults.Terminal.MinHeight)
	for action, key := range defaults.Keybindings {
		v.SetDefault(fmt.Sprintf("keybindings.%s", action), key)
	}
	v.SetDefault("vault_path", "")
	v.SetDefault("theme", defaults.Theme)
	v.SetDefault("sync.enabled", defaults.Sync.Enabled)
	v.SetDefault("sync.remote", defaults.Sync.Remote)

	// Read and parse YAML
	if err := v.ReadInConfig(); err != nil {
		// T051: Log parse error
		if shouldLogConfig() {
			fmt.Fprintf(os.Stderr, "[Config] Failed to parse YAML: %v\n", err)
		}
		return GetDefaults(), &ValidationResult{
			Valid: false,
			Errors: []ValidationError{
				{Field: "config_file", Message: fmt.Sprintf("failed to parse YAML: %v", err)},
			},
		}
	}

	// T054: Detect unknown fields
	warnings := detectUnknownFields(v)

	// Unmarshal into Config struct (Viper will merge with defaults)
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		// T051: Log unmarshal error
		if shouldLogConfig() {
			fmt.Fprintf(os.Stderr, "[Config] Failed to unmarshal config: %v\n", err)
		}
		return GetDefaults(), &ValidationResult{
			Valid: false,
			Errors: []ValidationError{
				{Field: "config_file", Message: fmt.Sprintf("failed to unmarshal config: %v", err)},
			},
		}
	}

	// Validate the loaded config
	validationResult := cfg.Validate()

	// Add unknown field warnings to validation result
	validationResult.Warnings = append(validationResult.Warnings, warnings...)

	// T052: Log validation errors
	if !validationResult.Valid {
		if shouldLogConfig() {
			fmt.Fprintf(os.Stderr, "[Config] Validation failed with %d error(s)\n", len(validationResult.Errors))
			for _, err := range validationResult.Errors {
				fmt.Fprintf(os.Stderr, "[Config]   - %s: %s\n", err.Field, err.Message)
			}
		}
		return GetDefaults(), validationResult
	}

	// T051: Log successful load
	if shouldLogConfig() {
		fmt.Fprintf(os.Stderr, "[Config] Successfully loaded config\n")
	}

	return &cfg, validationResult
}

// Load loads configuration from the default config path
func Load() (*Config, *ValidationResult) {
	configPath, err := GetConfigPath()
	if err != nil {
		// Cannot determine config path, use defaults
		return GetDefaults(), &ValidationResult{
			Valid: true,
			Warnings: []ValidationWarning{
				{Field: "config_path", Message: fmt.Sprintf("cannot determine config path: %v", err)},
			},
		}
	}

	return LoadFromPath(configPath)
}

// Validate validates the configuration and returns a validation result
func (c *Config) Validate() *ValidationResult {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []ValidationError{},
		Warnings: []ValidationWarning{},
	}

	// Validate terminal configuration
	result = c.validateTerminal(result)

	// T032: Validate keybindings
	result = c.validateKeybindings(result)

	// Validate vault_path
	result = c.validateVaultPath(result)

	// Validate theme
	result = c.validateTheme(result)

	// Validate sync
	result = c.validateSync(result)

	// Set Valid flag based on error count
	if len(result.Errors) > 0 {
		result.Valid = false
	}

	return result
}

// validateTerminal validates terminal size configuration
func (c *Config) validateTerminal(result *ValidationResult) *ValidationResult {
	// T019: Validate min_width range (1-10000)
	if c.Terminal.MinWidth < 1 || c.Terminal.MinWidth > 10000 {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "terminal.min_width",
			Message: fmt.Sprintf("must be between 1 and 10000 (got: %d)", c.Terminal.MinWidth),
		})
	}

	// T019: Validate min_height range (1-1000)
	if c.Terminal.MinHeight < 1 || c.Terminal.MinHeight > 1000 {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "terminal.min_height",
			Message: fmt.Sprintf("must be between 1 and 1000 (got: %d)", c.Terminal.MinHeight),
		})
	}

	// T021: Warn if unusually large width (>500)
	if c.Terminal.MinWidth > 500 {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Field:   "terminal.min_width",
			Message: fmt.Sprintf("unusually large value (%d) - most terminals are <300 columns", c.Terminal.MinWidth),
		})
	}

	// T021: Warn if unusually large height (>200)
	if c.Terminal.MinHeight > 200 {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Field:   "terminal.min_height",
			Message: fmt.Sprintf("unusually large value (%d) - most terminals are <100 rows", c.Terminal.MinHeight),
		})
	}

	// Validate detail_position value
	if c.Terminal.DetailPosition == "" {
		c.Terminal.DetailPosition = "auto" // Default to auto if empty
	}
	validPositions := map[string]bool{"auto": true, "right": true, "bottom": true}
	if !validPositions[c.Terminal.DetailPosition] {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "terminal.detail_position",
			Message: fmt.Sprintf("must be 'auto', 'right', or 'bottom' (got: %s)", c.Terminal.DetailPosition),
		})
	}

	// Validate detail_auto_threshold range (80-500)
	if c.Terminal.DetailAutoThreshold == 0 {
		c.Terminal.DetailAutoThreshold = 120 // Default to 120 if not set
	}
	if c.Terminal.DetailAutoThreshold < 80 || c.Terminal.DetailAutoThreshold > 500 {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "terminal.detail_auto_threshold",
			Message: fmt.Sprintf("must be between 80 and 500 (got: %d)", c.Terminal.DetailAutoThreshold),
		})
	}

	return result
}

// T032: validateKeybindings validates and parses keybinding configuration
func (c *Config) validateKeybindings(result *ValidationResult) *ValidationResult {
	// If keybindings is empty, merge with defaults
	if len(c.Keybindings) == 0 {
		c.Keybindings = GetDefaults().Keybindings
	}

	// Step 1: Check for unknown actions
	actionErrors := ValidateActions(c.Keybindings)
	for _, errMsg := range actionErrors {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "keybindings",
			Message: errMsg,
		})
	}

	// Step 2: Check for conflicts (duplicate key assignments)
	conflicts := DetectKeybindingConflicts(c.Keybindings)
	for _, conflict := range conflicts {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "keybindings",
			Message: conflict,
		})
	}

	// Step 3: Parse each keybinding and store parsed versions
	c.ParsedKeybindings = make(map[string]*Keybinding)
	for action, keyStr := range c.Keybindings {
		key, r, mods, err := ParseKeybinding(keyStr)
		if err != nil {
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("keybindings.%s", action),
				Message: fmt.Sprintf("invalid key format '%s': %v", keyStr, err),
			})
			continue
		}

		// Store parsed keybinding
		c.ParsedKeybindings[action] = &Keybinding{
			Action:    action,
			KeyString: keyStr,
			Key:       key,
			Rune:      r,
			Modifiers: mods,
		}
	}

	return result
}

// validateVaultPath validates the vault_path configuration field
func (c *Config) validateVaultPath(result *ValidationResult) *ValidationResult {
	if c.VaultPath == "" {
		// Empty is valid - use default
		return result
	}

	// 1. Check for obviously malformed paths (null bytes)
	if containsNullByte(c.VaultPath) {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "vault_path",
			Message: "path contains null byte",
		})
		return result
	}

	// 2. Expand for validation purposes (don't modify original)
	expandedPath := os.ExpandEnv(c.VaultPath)
	if len(expandedPath) > 0 && expandedPath[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			expandedPath = filepath.Join(home, expandedPath[1:])
		}
	}

	// 3. Warn on relative paths (will be resolved to home directory)
	// Skip warning if path was originally absolute (even if Unix-style on Windows)
	if !filepath.IsAbs(expandedPath) && !isPathWithVariable(c.VaultPath) && !filepath.IsAbs(c.VaultPath) {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Field:   "vault_path",
			Message: fmt.Sprintf("relative path '%s' will be resolved relative to home directory", c.VaultPath),
		})
	}

	// 4. Check parent directory is accessible (if absolute)
	if filepath.IsAbs(expandedPath) {
		parentDir := filepath.Dir(expandedPath)
		if _, err := os.Stat(parentDir); err != nil {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Field:   "vault_path",
				Message: fmt.Sprintf("parent directory '%s' does not exist or is not accessible", parentDir),
			})
		}
	}

	return result
}

// containsNullByte checks if a string contains a null byte
func containsNullByte(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\x00' {
			return true
		}
	}
	return false
}

// isPathWithVariable checks if path contains ~ prefix or environment variable
func isPathWithVariable(path string) bool {
	if len(path) > 0 && path[0] == '~' {
		return true
	}
	// Check for $ or % (Unix and Windows env vars)
	for i := 0; i < len(path); i++ {
		if path[i] == '$' || path[i] == '%' {
			return true
		}
	}
	return false
}

// GetParsedKeybindings returns the parsed keybindings map
// Must call Validate() first to populate ParsedKeybindings
func (c *Config) GetParsedKeybindings() map[string]*Keybinding {
	if c.ParsedKeybindings == nil {
		return make(map[string]*Keybinding)
	}
	return c.ParsedKeybindings
}

// validateTheme validates the theme configuration
func (c *Config) validateTheme(result *ValidationResult) *ValidationResult {
	// Empty theme means use default
	if c.Theme == "" {
		c.Theme = "dracula"
		return result
	}

	// Check if theme is valid
	validThemes := map[string]bool{
		"dracula": true,
		"nord":    true,
		"gruvbox": true,
		"monokai": true,
	}

	if !validThemes[c.Theme] {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "theme",
			Message: fmt.Sprintf("unknown theme '%s' (valid themes: dracula, nord, gruvbox, monokai)", c.Theme),
		})
	}

	return result
}

// validateSync validates the sync configuration
func (c *Config) validateSync(result *ValidationResult) *ValidationResult {
	// If sync is not enabled, no validation needed
	if !c.Sync.Enabled {
		return result
	}

	// If sync is enabled, remote must be specified
	if c.Sync.Remote == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "sync.remote",
			Message: "sync.remote is required when sync.enabled is true (e.g., 'gdrive:.pass-cli')",
		})
	}

	return result
}
