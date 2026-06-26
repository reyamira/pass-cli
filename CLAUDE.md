# pass-cli Development Guidelines

## Active Technologies
- Go 1.25+
- Cobra (CLI framework), Viper (configuration management)
- rivo/tview + tcell (TUI)
- go-keyring (OS keychain integration)
- pquerna/otp + qrterminal (TOTP/QR support)
- rclone (cloud sync via CLI, not a Go dependency)

## Project Structure

```
pass-cli/
├── cmd/                      # CLI commands (Cobra-based)
│   ├── tui/                  # TUI components (rivo/tview)
│   ├── root.go               # Root command and global flags
│   └── ...                   # Individual command files
├── internal/                 # Internal library packages
│   ├── vault/                # Vault operations and credential management
│   ├── crypto/               # Encryption/decryption (AES-GCM, password clearing)
│   ├── keychain/             # OS keychain integration (Windows/macOS/Linux)
│   ├── security/             # Audit logging with HMAC signatures
│   ├── sync/                 # Cloud sync via rclone (SmartPull/SmartPush)
│   ├── storage/              # File operations
│   ├── config/               # Configuration handling
│   └── health/               # Health checks for doctor command
├── test/                     # Integration and unit tests
│   ├── unit/                 # Unit tests
│   ├── integration/          # Integration tests
│   └── helpers/              # Test utilities
├── docs/                     # Documentation
├── main.go                   # Application entry point
└── go.mod                    # Go module dependencies
```

**Architecture**: Library-first design. CLI commands (`cmd/`) are thin wrappers that delegate to `internal/` packages.

## Commands

```bash
# Use mise tasks for all CLI operations
mise tasks                    # List available tasks
mise run test                 # Run unit tests
mise run test:integration     # Run integration tests
mise run lint                 # Run linter
mise run build                # Build binary
mise run fmt                  # Format code
mise run vet                  # Run go vet
mise run test:all             # Run unit + integration tests
mise run git <args>           # Git operations
mise run gh <args>            # GitHub CLI operations
```

## Code Style

**Password Handling**:
- Use `[]byte` type (never `string`)
- Apply `defer crypto.ClearBytes(password)` immediately after allocation

**Error Handling**:
- Wrap errors with context: `fmt.Errorf("failed to unlock vault: %w", err)`
- Graceful degradation (e.g., keychain unavailable should not crash)

**Testing**:
- Unit tests: `internal/` packages
- Integration tests: `test/integration/` with `-tags=integration`
- Use `runtime.GOOS` for platform-specific test behavior

## Platform-Specific Gotchas

**macOS keychain**: Tests that override `HOME` env var break keychain access (tied to user session). Use `runtime.GOOS != "darwin"` checks before setting fake HOME in integration tests.

## Cobra Patterns

**Config loading order**: `cobra.OnInitialize` runs BEFORE flags are parsed. For flag-dependent config (like `--config`), use `PersistentPreRunE` instead.

## Sync Architecture

- Vault is a single encrypted file — all pushes/pulls transfer one file regardless of change count
- `SmartPush` hashes local file, skips network if hash matches `LastPushHash` in `.sync-state`
- `SmartPull` runs before unlock, `SmartPush` runs after command completes (synchronous, blocks prompt)
- `RecordFieldAccess` (called by `get`) writes usage timestamps, changing vault hash and triggering push
- Two network round-trips per push: `rclone sync` + `rclone lsjson`
- Async push risks: silent failures, false conflicts (kill between push and SaveState), partial uploads on some backends

## Environment Setup

- **Required**: Go 1.25+, mise, golangci-lint v2.5+
- **For sync**: rclone configured with a remote
- **Linux CI/tests**: dbus-x11, gnome-keyring (for keychain tests)
