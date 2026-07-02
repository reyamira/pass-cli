# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.20.0] - 2026-07-02

### Added
- **`skills` command — self-served, version-matched agent guides** — `pass-cli skills` ships an AI-agent usage guide embedded in the binary, mirroring the pattern that makes tools like agent-browser easy for agents to adopt: `pass-cli skills list` lists the available skills and `pass-cli skills get core` prints the safe-usage guide (`exec`/`export`/`inject`/`agent`/`list`/`get` plus the leak traps to avoid), with `--full` appending a complete command reference. Because the content ships *in the binary* (via `//go:embed`), the guidance always matches the installed version and can't drift from shipped behavior. `pass-cli skills install` writes a small discovery stub into a detected agent skills directory (`~/.claude/skills` or `~/.agents/skills`, or `--dir <path>`) that points agents at `pass-cli skills get core`; an existing, differing stub is preserved unless `--force` is given. This is the first step toward pass-cli being a first-class tool for humans **and** their AI agents.

## [0.19.0] - 2026-07-02

### Added
- **TUI: sort the credential table by field** (#141) — press `o` to cycle the sort field (Service → Username → Last Used) and `O` to reverse the direction; the current sort shows in the table title (e.g. `Credentials (12) · Last Used ↓`). Session-only, defaulting to Service ascending so existing behavior is unchanged. Applies to the main table; the category-grouped sidebar stays alphabetical.
- **Env-injection: value filters on resolved references** (#138) — a credential reference may end with `| filter` to transform the value before injection, resolved through the same shared grammar so it works in `${pass:...}` templates, `--set`, and `.pass-cli.toml` manifests: `${pass:api/token | base64}`. Filters: `base64` (standard) and `base64url` (URL-safe) encode a single value; `basicauth` combines a credential's own username and password into `base64("user:pass")` for an HTTP `Authorization: Basic` header (takes no field). One filter per reference; an unknown or empty filter is a hard parse error and nothing is injected (fail closed) — for `--set` and manifests this is caught before the vault is even opened. Filters are applied client-side after resolution, so the agent's values-only protocol is unchanged (identical output in direct and daemon modes). On the `--set` form the `|` must be shell-quoted; filters are not applied to the positional `<service>` form. Item 3 of the #115 epic, completing it.
- **Sync: configurable remote-probe timeout** (#137) — the pre-unlock metadata probe (`rclone lsjson`) was bounded at a hardcoded 8s, so a slow-but-alive remote on a high-latency backend would time out, be misclassified as failed, and enter the failure-backoff (#133) — deferring pulls/pushes for the whole window even though the remote was reachable. New `sync.probe_timeout_seconds` mirrors the `pull_ttl_seconds` tri-state: `0` uses the built-in default (8s), a positive value raises (or lowers) the bound, and a negative value disables it (unbounded probe). Only the metadata probe is bounded — the heavy pull/push transfers remain unbounded regardless.
- **Background agent (`pass-cli agent`)** (#116) — an optional daemon that unlocks the vault once and holds it in memory, answering read-only credential lookups over a local unix socket so `exec`/`export`/`inject` need no master-password prompt and no key derivation on each call. It serves resolved field **values only** — the master password and derived key never cross the socket. Auto-locks after `--idle` inactivity (default 15m) and always after `--max-ttl` (default 8h), and locks + exits on SIGINT/SIGTERM. **`pass-cli agent start`** backgrounds the agent (unlock once on your terminal, then detach — no shell `&` needed); **`pass-cli agent serve`** (or bare `pass-cli agent`) runs it in the foreground. **`agent stop`** zeroes the resident secrets and stops the agent (freeing the socket so the next command falls back to direct-open; re-run `agent start` to re-establish it); `agent status` reports its state (never prints secrets). When no agent is running, every command **transparently falls back** to opening and unlocking the vault directly, so the agent is a pure optimization, never a dependency. Socket path: `$PASS_CLI_AGENT_SOCK`, else `$XDG_RUNTIME_DIR/pass-cli/agent.sock`, else `~/.pass-cli/agent.sock` (directory `0700`, socket `0600`). Connections are additionally authorized by peer credential — only a process owned by the same user may talk to the agent, and any failure to read the credential is a rejection (fail-closed): Linux via `SO_PEERCRED`, macOS via `getsockopt(LOCAL_PEERCRED)`. (Windows would use a named pipe + ACL; the Windows agent is not yet implemented and falls back to direct-open.) POSIX only for now; a Windows named-pipe transport is planned.
- **`export` command — print shell statements that set credentials as env vars** (#115) — `pass-cli export` emits `export NAME='value'` for `eval`/`source`, the blessed replacement for `VAR="$(pass-cli get …)"`: `eval "$(pass-cli export --set GITHUB_TOKEN=github)"`. Uses the same mapping grammar as `exec` (repeatable `--set ENV_NAME=service[/field]` and the convenience form `pass-cli export <service>`, which derives the name from the service), and the same `-f/--field` selection. `--format sh|fish|powershell` selects the shell syntax (default `sh`). Read-only like `exec` (records no usage, triggers no sync push). Because `export` output is meant to be eval'd, env names are validated against `[A-Za-z_][A-Za-z0-9_]*` before the vault is opened, and every field value is shell-quoted per target shell.
- **`inject` command — render a template, substituting `${pass:service/field}` references** (#115) — `pass-cli inject` reads a template from `--in-file`/stdin and writes it back with every `${pass:service/field}` reference replaced by the credential's value: `echo 'postgres://app:${pass:db/password}@host/db' | pass-cli inject`. This is the composite/derived-secret tool (whole config files, connection strings) that a single `ENV=service` mapping cannot express. Only `${pass:...}` is special (`$VAR`/`${VAR}`/`$(...)` pass through); resolution is single-pass and fail-closed (an unknown or malformed reference errors and writes nothing). `--out-file` is created `0600`. Read-only.
- **`exec --env-file <path>`** (#115) — a third credential source for `exec`: a file of `KEY=<template>` lines whose values may embed `${pass:service/field}` references, resolved and injected into the child environment (never written to disk). Composes with `--set`; blank lines and `#` comments are ignored, and each `KEY` is validated as an env var name. This supersedes a separate `run` command (folded into `exec` to avoid duplicating its inject-then-run behavior).
- **`.pass-cli.toml` project manifest + `--from <file>`** (#115) — a committable manifest whose `[env]` table maps `ENV_NAME = "service/field"` (references only, never values) so a launcher or `.envrc` need not repeat long `--set` chains. `--from <manifest>` (repeatable) is accepted by both `exec` and `export` and composes with `--set`; entries are validated (env name + reference) and emitted sorted by name. Example: `eval "$(pass-cli export --from .pass-cli.toml)"`.

### Changed
- **Agent memory hardening** (#116) — on Linux, `pass-cli agent` now disables core dumps and casual ptrace/`/proc/<pid>/mem` access (`PR_SET_DUMPABLE=0`) and attempts to lock its memory into RAM so secrets never reach swap (`mlockall`). Core-dump/ptrace protection is unconditional; the memory lock is best-effort and needs `CAP_IPC_LOCK` or a raised `RLIMIT_MEMLOCK` (a raised limit is set by the systemd unit). macOS/Windows hardening is planned.
- **Agent revalidating cache** (#116) — a running `pass-cli agent` now reloads its in-memory vault snapshot when `vault.enc` changes on disk (e.g. another shell ran `add`/`update`), so it serves fresh data instead of a stale snapshot, without taking an exclusive lock that would block those other operations. If the on-disk vault can no longer be decrypted with the held key (the master password was rotated by another process), the agent locks and fails the request rather than serving stale data. The resolve path remains strictly read-only.
- **Env-injection grammar: prefer `/` over `:`** (#115) — credential references now accept a slash separator (`service/field`) in addition to the legacy colon form (`service:field`, still accepted unchanged). Slash is preferred because in slash form a `:` inside a service name is a literal character, and it lines up with a future `op://`-style path. Applies to `--set` on both `exec` and `export`.

### Fixed
- **Consistent, case-insensitive ordering everywhere** (#139) — credential and category lists were sorted case-sensitively, so lowercase entries sorted after all uppercase ones (e.g. `azure` after `GitHub`), and the TUI's main credential table was not sorted at all (map-iteration order). Every name ordering — the TUI sidebar and main table, `list`, the add/edit category dropdowns, `backup preview`, and usage field breakdowns — now sorts case-insensitively and agrees with the others.
- **Sync: an unreachable remote no longer costs the probe timeout on every command** (#133) — the pre-unlock TTL-gate (#103) only opened on a *successful* probe, so with sync enabled and the remote down/slow every command re-probed and blocked for the full 8s (`Warning: failed to check remote state: … timed out after 8s`). A failed probe now stamps a failure timestamp and enters a backoff (the same window as `pull_ttl_seconds`, default 30s): subsequent commands skip the probe and serve the local vault, so a dead remote costs the timeout at most once per window. A successful contact clears the backoff immediately. To stay safe, a write during the backoff is **deferred** (not pushed) rather than blind-pushed — so a remote that recovered mid-window can't be silently overwritten — and syncs on the next command after the window expires.

## [0.18.0] - 2026-06-26

### Added
- **`exec` command — inject credentials as environment variables** (#98) — `pass-cli exec` runs a child command with stored credentials passed only through its environment, so the secret never touches a file, the clipboard, or shell history. Supports an explicit, repeatable `--set ENV_NAME=service[:field]` mapping and a convenience form (`pass-cli exec <service> -- <cmd>`) that derives the env name from the service (uppercased, non-alphanumeric → `_`, e.g. `openai-api` → `OPENAI_API`). The `-f/--field` flag selects the field for all mappings (default `password`); a per-mapping `:field` suffix overrides it, allowing two fields of one entry to be injected as separate variables. Everything after `--` is the child argv, and the child's exit code is propagated unchanged. `exec` is read-only: it records no usage and triggers no sync push, making it safe on a hot path.
- **`--offline` global flag** (#104) — run a command fully local: skips both the pre-unlock remote pull and the post-command push (offline changes sync on the next online run).

### Changed
- **`list` is now safe-by-default** (#95, #97) — the default table hides the Username column, because the "username" field can hold sensitive values (card, account, or routing numbers stored as a username). New `--show-usernames` re-adds the column, and new `-q/--quiet` is a shorthand for `--format simple` (bare service names, one per line) that takes precedence over `--format`. `--format json` is unchanged and still emits full metadata including usernames as an explicit, structured opt-in.
- **Sync: content-hash change detection** (#102) — push change-detection now uses a name-encoded zero-byte marker (`vault.enc.<sha256>.synchash`) written next to the vault on each push, replacing the old modtime+size heuristic; older vaults without a marker fall back to the previous behavior.
- **Sync: lower unlock latency by overlapping the pull** (#103) — the pre-unlock remote pull now runs concurrently with the master-password prompt (Tier 1, #109) and with PBKDF2-SHA256 key derivation on keychain unlock (Tier 2, #110). Internal only; no flag or UX change.
- **Sync: cut sync-related startup latency** (#104) — dropped the dead `rclone lsjson --hash` flag (no decision used the remote hash; it only added backend hashing cost) and added a transient progress indicator on stderr during the pre-unlock remote probe.
- **Org transfer to `reyamira`** (#112) — the repository and its package sources (Homebrew tap, Scoop bucket, GitHub Pages, badges) moved from `arimxyer` to `reyamira`. Install commands now reference `reyamira/...` (old URLs redirect). The Go module path (`github.com/arimxyer/pass-cli`) is intentionally unchanged.

### Fixed
- **Test: resolve `captureStdout` redeclaration across `cmd` test files** (#99)

### Infrastructure
- **Tracked `mise.toml`** (#100) — `mise.toml` is now committed so the documented `mise run …` tasks work on a fresh clone (previously gitignored).
- **CI runs on all PRs** (#105) — required status checks now report for non-code-change PRs as well.
- **pass-cli agent skill** (#101) — added an agent skill under `.claude/skills` and tracked the directory.

## [0.17.2] - 2026-01-31

### Changed
- **Sync: Skip push on reads** — `get` command no longer triggers a sync push, eliminating unnecessary file hashing and network round-trips on every read
- **Sync: Skip push on read-only TUI sessions** — browsing credentials without editing no longer triggers a push on exit
- **Sync: "Syncing... done" feedback** — write commands (`add`, `update`, `delete`) and TUI write sessions now show sync progress on stderr
- **Sync: SmartPush returns status** — `SmartPush` now returns whether a push was actually performed, avoiding redundant hash checks

### Fixed
- **CI: Skip duplicate CI on tag pushes** — tag pushes no longer trigger a redundant CI run (the commit already passed CI on main)

### Infrastructure
- **GitHub tag ruleset** — `v*` tags now require all CI status checks to pass before creation

## [0.17.1] - 2026-01-29

### Changed
- **Sync: Move push to command layer** — sync push moved from vault `save()` to individual command handlers, reducing unnecessary pushes during multi-save operations

## [0.17.0] - 2026-01-29

### Added
- **Smart sync with change detection** — `SmartPush` hashes local vault and compares to last push hash, skipping network calls when nothing changed; `SmartPull` checks remote metadata before pulling

## [0.16.2] - 2026-01-24

### Fixed
- **Config Flag Loading**: `--config` flag now works correctly without requiring a vault at the default location (#68, fixes #65)
  - Config loading moved from `cobra.OnInitialize` to `PersistentPreRunE` to ensure flags are parsed first
  - Users can now specify `vault_path` in a custom config file and use `--config` flag without creating a dummy vault
- **Lightweight Commands**: `version` and `help` commands skip config loading for faster startup
- **macOS CI**: Fixed integration test timeout by handling keychain environment requirements

## [0.16.1] - 2026-01-05

### Added
- **TUI TOTP Visibility Toggle**: View TOTP codes directly in the detail panel
  - Press `T` (Shift+t) to toggle TOTP code visibility
  - Displays 6-digit code with remaining seconds countdown
  - Press `t` to copy code to clipboard (unchanged)
  - Updated help modal with new shortcut

## [0.16.0] - 2025-12-31

### Added
- **Sync Enable Command**: New `pass-cli sync enable` command to add cloud sync to existing vaults
  - Interactive remote path configuration
  - Validates rclone installation and remote connectivity
  - Detects existing files on remote with `--force` option to overwrite
  - Performs initial push after configuration
- **Sync Health Check**: `pass-cli doctor` now reports sync status
  - Checks rclone installation and version
  - Validates remote configuration
  - Reports sync enabled/disabled state
- **Connect to Synced Vault**: `pass-cli init` now offers option to connect to existing synced vault
  - Downloads vault from remote during initialization
  - Validates downloaded vault with master password
  - Configures sync automatically after successful connection
- **Audit MachineID**: Audit log entries now include `MachineID` field (hostname)
  - Enables tracking vault access across synced devices
  - Included in HMAC signature for tamper detection

### Changed
- **Init Flow**: Now prompts to create new vault or connect to existing synced vault
- **Config Management**: `saveSyncConfig` uses proper YAML marshaling instead of string concatenation

### Fixed
- Clipboard test reliability for parallel execution
- Keychain backend detection for Linux Secret Service variants

## [0.15.0] - 2025-12-30

### Added
- **Cloud Sync**: Sync vault across devices using rclone
  - Automatic pull on first CLI usage per session
  - Automatic push after write operations (add, update, delete)
  - Supports 70+ cloud providers via rclone (Google Drive, Dropbox, OneDrive, S3, etc.)
  - Graceful degradation: sync failures warn but don't block operations
  - Configuration via `sync.enabled` and `sync.remote` in config.yml
- **Portable Audit Keys**: Cross-OS audit log verification for synced vaults
  - Audit keys derived from master password + salt (PBKDF2-SHA256, 100k iterations)
  - Salt stored in vault metadata for portability
  - `verify-audit` works on any synced device with same master password
- **Sync Documentation**: Comprehensive guide at `docs/02-guides/sync-guide.md`
- **Config Template**: Sync configuration examples in `pass-cli config init` output

### Changed
- **README**: Updated "Offline First" to "Local-First" with optional sync mention
- **Configuration Reference**: Added sync configuration section

## [0.14.0] - 2025-12-29

### Added
- **TOTP/2FA Support**: Complete Time-based One-Time Password implementation per RFC 6238
  - CLI: `--totp-uri` and `--totp` flags for `add` command
  - CLI: `--totp`, `--totp-qr`, `--totp-qr-file` flags for `get` command with countdown display
  - CLI: `--totp-uri` and `--clear-totp` flags for `update` command
  - TUI: TOTP display in detail view with issuer when available
  - TUI: `t` key binding to copy TOTP code to clipboard
  - TUI: TOTP input fields in Add and Edit forms
  - Supports both base32 secrets and `otpauth://` URIs
  - Configurable algorithm (SHA1/SHA256/SHA512), digits (6/8), and period (1-300s)
  - Audit logging for TOTP operations: `EventTOTPAccess`, `EventTOTPAdd`, `EventTOTPUpdate`, `EventTOTPClear`
- **TOTP Documentation**: Comprehensive guide at `docs/02-guides/totp-guide.md`
- **Social Preview**: GitHub social preview image for repository
- **README Improvements**: TUI screenshot, roadmap section, updated FAQ

### Fixed
- **Critical: Vault Corruption on Password Copy**: Fixed shallow copy in `GetCredential()` causing vault corruption
  - Password `[]byte` slice shared memory with vault's internal data
  - Callers zeroing their copy (security best practice) corrupted vault's canonical data
  - TUI: pressing `c` twice would panic due to NUL bytes sent to Windows clipboard
  - Now returns deep copy of password bytes for each caller
- **Scoop Bucket URL**: Fixed incorrect URL in README

### Changed
- **Repository Username**: Updated from `ari1110` to `arimxyer` across all configs and documentation

### Dependencies
- Added `github.com/pquerna/otp v1.5.0` for RFC 6238 TOTP implementation
- Bumped `golang.org/x/crypto` from 0.45.0 to 0.46.0
- Bumped `actions/cache` from 4 to 5
- Bumped `actions/upload-artifact` from 5 to 6
- Bumped `actions/download-artifact` from 6 to 7
- Bumped `DavidAnson/markdownlint-cli2-action` from 21 to 22

### Testing
- Added Linux keychain testing with D-Bus + gnome-keyring in CI
- Improved HOME directory isolation in TUI tests
- Enhanced keychain cleanup patterns across all integration tests
- Migrated tests to use `helpers.SetupTestVaultWithName` for consistency

## [0.13.0] - 2025-12-11

### Added
- **Vault-Specific Keychain Entries**: Each vault now stores its master password under a unique keychain account
  - Account name format: `master-password-<vaultID>` (e.g., `master-password-my-vault`)
  - VaultID derived from vault directory name for consistency
  - Enables multiple vaults to have separate keychain entries on the same system
  - Automatic migration from global entry when vault-specific entry not found
  - `HasGlobalEntry()`, `MigrateFromGlobal()`, `DeleteGlobal()` methods for migration support

### Fixed
- **Integration Tests**: Fixed vault-specific keychain account mismatch in 13 test functions
  - Tests now correctly derive vaultID from vault path to match CLI behavior
  - Resolved CI failures on macOS and Windows (Ubuntu skipped due to no keychain)

### Changed
- **Test Structure**: Reorganized integration tests into `test/integration/` directory
  - Added centralized test helpers package
  - Improved keychain cleanup in test teardown

## [0.12.2] - 2025-12-10

### Fixed
- **Audit Log HMAC Verification**: Fixed VaultID inconsistency causing HMAC verification failures
  - `vault.New()` autodiscovery was using full vault path as VaultID
  - `init` and `verify-audit` commands use directory name as VaultID
  - This mismatch caused entries logged during autodiscovery to fail verification
  - Now all code paths consistently use directory name as VaultID

### Added
- **Integration Test for verify-audit**: Added comprehensive tests to prevent regression
  - `TestIntegration_VerifyAudit`: Full workflow test (init → add → get → verify)
  - `TestIntegration_VerifyAudit_ConsistentVaultID`: Tests VaultID consistency across operations

## [0.12.1] - 2025-12-10

### Fixed
- **TUI Help Modal**: Improved styling and usability
  - Changed scroll instructions from "PgUp/PgDn" to "↑/↓ Arrow Keys"
  - Added visible row highlight (navy background + bold) for better focus tracking
  - Centered title "Keyboard Shortcuts" above shortcuts table
  - Split footer into two lines for better readability
- **TUI Detail Panel**: Improved visual styling
  - Changed label color from gray to lightSlateGray for better contrast
  - Centered section headers ("Metadata", "Usage Locations") within separator width
  - Added 2-space indent to separators for visual balance

### Testing
- Added keychain cleanup to integration test teardown to prevent orphaned credentials

## [0.12.0] - 2025-12-10

### Added
- **Backup Restore Selection**: New options for restoring from specific backups
  - `--file` flag to restore from a specific backup file path
  - `--interactive` / `-i` flag for numbered list selection of available backups
- **Backup Preview Command**: `vault backup preview` to inspect backup contents before restoring
  - Shows credential names in backup (requires backup's password)
  - `--verbose` flag for detailed output with timestamps and categories
  - Helpful error messages when wrong password used
- **Audit Logging Default**: Audit logging now enabled by default during vault initialization
  - Use `--no-audit` flag to disable if not wanted
  - Existing vaults retain their current audit settings on upgrade
- **Config Template**: Added `vault_path` configuration example to config template

### Fixed
- **TUI: PowerShell Shift+Tab**: Pin tcell to v2.11.0 to fix Shift+Tab regression on Windows PowerShell
  - Root cause: tcell v2.12.0 Win32 input mode redesign broke VT sequence handling
  - Also fixes status bar styling issues
  - Filed upstream: https://github.com/gdamore/tcell/issues/901

### Changed
- **Usage Command Styling**: Refactored to use styled tablewriter for consistent table output
- **Documentation**: Updated all `--enable-audit` references to `--no-audit` pattern

### Testing
- Added keychain persistence tests for binary upgrade scenarios
- Fixed integration tests for tablewriter v1.x uppercase header format
- Added `--no-audit` to tests that don't specifically test audit functionality

## [0.11.2] - 2025-12-06

### Fixed
- **Critical**: 6-word challenge recovery now works correctly for v2 vaults
  - V2 vaults created in v0.11.0 were missing challenge data (`ChallengePositions`, `EncryptedStoredWords`, `NonceStored`, `SaltChallenge`)
  - `change-password --recover` would fail immediately with "invalid word" error due to empty challenge positions
  - Users with affected v2 vaults should run `pass-cli vault migrate` to regenerate recovery phrase with proper challenge data
- **Critical**: Password change after recovery unlock now works
  - Added `SetPasswordAfterRecovery()` method that uses the DEK from recovery unlock
  - Previously failed with "vault was unlocked via recovery, set a new password first"
- **Tests**: Keychain tests now use isolated service name to prevent conflicts with real CLI usage

### Changed
- **Refactor**: Extracted `RecoveryMetadata` and `KDFParams` types to `internal/shared` package
  - Eliminates ~200 lines of duplicated challenge setup code
  - Breaks import cycle between `vault` and `recovery` packages
  - Single source of truth for `SetupChallengeRecovery()` in recovery package
  - Backward-compatible type aliases in vault package

## [0.11.0] - 2025-12-05

### Added
- **V2 Vault Format**: New key wrapping architecture with Data Encryption Key (DEK) and dual Key Encryption Keys (KEKs)
  - Password-derived KEK for normal vault access
  - Recovery-derived KEK for recovery phrase unlock
  - Both KEKs wrap the same DEK, enabling secure recovery without password knowledge
- **Vault Migration Command**: `pass-cli vault migrate` to upgrade V1 vaults to V2 format
  - Preserves all existing credentials
  - Generates new recovery phrase with proper key wrapping
  - Interactive verification of new recovery phrase backup
  - Optional BIP-39 passphrase protection ("25th word")
- **Recovery Key Integration**: BIP-39 recovery phrases now fully functional for V2 vaults
  - 6-word challenge recovery (73.8 quintillion combinations)
  - Argon2id key derivation for recovery KEK
  - Recovery-wrapped DEK stored in vault metadata
- **New Vault Metadata Fields**: `wrapped_dek`, `wrapped_dek_nonce`, `recovery_wrapped_dek`, `recovery_wrapped_dek_nonce`, `recovery_salt`

### Fixed
- **Critical**: V1 vaults had a bug where recovery phrases could not unlock the vault - V2 format resolves this
- Recovery tests updated for V2 key wrapping format
- Stale keychain state handling in vault tests
- JSON unmarshal error return value checking

### Changed
- Vault initialization now uses V2 format by default with `InitializeWithRecovery`
- Recovery unlock path uses `RecoverWithMnemonic` with proper DEK unwrapping
- Documentation updated with V2 architecture details, migration guide, and recovery workflows

### Security
- AES-256-GCM encryption for DEK wrapping with unique nonces
- Argon2id (memory-hard) for recovery phrase key derivation
- PBKDF2-SHA256 (600,000 iterations) for password key derivation
- Separate salts for password and recovery derivation paths

## [0.10.0] - 2025-11-12

### Added
- **Manual Vault Backup Commands**: Three new CLI commands for manual backup management
  - `pass vault backup create` - Create timestamped manual backups (vault.enc.YYYYMMDD-HHMMSS.manual.backup)
  - `pass vault backup restore` - Restore vault from newest available backup (manual or automatic)
  - `pass vault backup info` - View backup status, history, and integrity
- **Smart Backup Selection**: Restore automatically selects newest valid backup with fallback to manual backups
- **Backup Integrity Verification**: Structural validation before backup creation and during restore
- **Interactive Restore Confirmation**: User prompts with backup details, `--force` for scripting, `--dry-run` for preview
- **Comprehensive Backup Status**: Lists all backups with age, size, integrity, and restore priority
- **Backup Warnings**: Alerts for old backups (>30 days) and excessive disk usage
- **Cross-Platform Support**: Works on Windows, macOS, Linux with platform-specific path handling
- **Backup Restore Guide**: 484-line comprehensive guide covering workflows, best practices, and troubleshooting

### Changed
- CI integration test timeout increased from 2m to 4m to accommodate Windows CI infrastructure

### Performance
- Integration test suite optimized: 96 tests complete in <3m across all platforms
- Backup operations exceed performance targets (create: 176ms < 5s, restore: 191ms < 30s, info: 191ms < 1s)

### Testing
- Added 6 comprehensive test files with 96 integration tests (100% pass rate)
- Storage package coverage increased to 81.4%
- Error handling tests for corrupted backups, missing vault, permission denied scenarios
- Platform-specific tests for Windows/Unix path handling

## [0.9.5] - 2025-11-11

### Added
- **TUI Password Generator**: In-form password generation with Ctrl+G shortcut for Add forms
- **CLI Password Generation**: `--generate` flag for `add` and `update` commands with configurable length
- **Clipboard Support**: Copy username (u), URL (l), notes (n), and password (c) from TUI detail view
- **Command Grouping**: CLI commands organized into logical groups (vault, credentials, security, utilities)
- **Multiple Color Themes**: Dracula (default), Nord, Gruvbox, and Monokai themes for TUI
- **Responsive Layout**: Configurable detail panel positioning (right/bottom/auto) with auto-threshold
- **Theme Configuration**: Terminal settings for theme, detail position, and auto-threshold in config.yaml

### Changed
- CLI help output now displays commands in organized groups for better discoverability
- TUI detail panel now uses dynamic color helpers for consistent theming
- Medium layout mode now supports detail panel in bottom position
- Detail panel auto-switches to bottom when terminal width < 120 columns

### Performance
- CI workflow optimized: reduced from 9+ minutes to ~4.5 minutes (50% improvement)
- Removed race detector tests due to fundamental conflict with security validation requirements
- Parallel job execution for lint, unit-tests, integration-tests, and security scans

### Dependencies
- Bumped github.com/fatih/color from 1.15.0 to 1.18.0
- Bumped github.com/olekukonko/tablewriter from 1.1.0 to 1.1.1
- Bumped golangci/golangci-lint-action from 8 to 9

## [0.9.0] - 2025-11-10

### Added
- **Atomic Save Pattern**: Crash-safe vault operations using write-to-temp, verify, atomic-rename workflow
- **Actionable Error Messages (FR-011)**: Clear error messages with specific failure reason, vault status confirmation, and actionable guidance
- **Complete Audit Logging (FR-015)**: All atomic save state transitions logged (9 events tracked)
- In-memory verification before committing vault changes to prevent corruption
- N-1 backup strategy with automatic cleanup after successful unlock
- Orphaned temporary file cleanup from crashed save operations
- Custom error types: ErrVerificationFailed, ErrDiskSpaceExhausted, ErrPermissionDenied, ErrFilesystemNotAtomic
- FileSystem abstraction interface for testability and error injection
- 8 new comprehensive test files with 80.8% coverage in storage package

### Changed
- Vault save operations now use atomic rename pattern instead of direct writes
- Error messages now include vault status and recovery guidance
- All vault modifications protected against crashes and power loss

### Fixed
- Vault corruption during save operations now impossible due to atomic pattern
- Clear error messages when save fails (disk space, permissions, verification)
- Temp files automatically cleaned up after successful saves

## [0.8.76] - 2025-11-08

### Fixed
- Documentation now uses correct repository URLs instead of placeholders
- Post-install messages updated for Homebrew and Scoop package managers
- Config debug output removed from production builds

## [0.8.75] - 2025-11-08

### Fixed
- Vault metadata handling and audit logging consistency
- Consistent vaultID usage for audit key storage and retrieval
- Tests updated to reflect metadata always created for all vaults
- First-run detection now works correctly in TUI entry point
- Vault initialization properly creates metadata during guided setup
- Password prompt no longer appears when vault doesn't exist on first run

### Added
- TUI now launches by default with first-run guided initialization
- Vault remove command deletes audit log and offers complete directory removal
- Enhanced keychain availability checking on-demand

### Changed
- Configuration location consolidated to `~/.pass-cli` for cross-platform consistency

## [0.8.74] - 2025-11-07

### Added
- Standalone security scan workflow for continuous security monitoring

### Fixed
- SARIF format sanitization for gosec security scan output
- Invalid artifactChanges fields removed from security scan results

## [0.8.73] - 2025-11-07

### Changed
- Documentation badges now use dynamic GitHub badges for version and last updated
- Removed static status badges in favor of dynamic alternatives
- Logo positioning improved in documentation

## [0.8.72] - 2025-11-06

### Fixed
- Keychain tests updated for lazy initialization pattern
- Prevented keyring.Set() blocking on macOS in CI environment
- Lazy keychain initialization prevents macOS CI hangs
- Missing keychain prompt added to TestDefaultVaultPath_Init

## [0.8.71] - 2025-11-06

### Changed
- List and usage tests refactored to use production flags
- Tests now use production flags instead of stdin for better reliability
- Stdin buffering conflicts in test mode resolved

### Fixed
- Cross-platform stdin reading reliability improved with bufio.Scanner
- macOS stdin blocking issues resolved
- Custom vault path and --config flag issues fixed

## [0.8.70] - 2025-11-05

### Changed
- Integration test timeout reduced from 5m to 2m for faster CI feedback
- Test mode detection using PASS_CLI_TEST environment variable
- golangci-lint now uses CLI args instead of config file

### Fixed
- First-run check now skipped in test mode
- All golangci-lint errors resolved in integration tests

## [0.8.55] - 2025-11-04

### Changed
- Documentation files reorganized for better clarity
- Removed test artifacts and temporary files from repository

### Fixed
- All linting issues resolved for code quality compliance
- Test failures fixed in Phase 6 quality improvements

## [0.8.52] - 2025-11-03

### Added
- Keychain status command with audit logging and consistency checks
- Keychain enable command with metadata integration

### Fixed
- Metadata file paths corrected in integration tests

## [0.8.51] - 2025-11-03

### Added
- Vault remove command with complete cleanup functionality

---

## Format Guidelines

This changelog follows these principles:
- **Added** for new features
- **Changed** for changes in existing functionality
- **Deprecated** for soon-to-be removed features
- **Removed** for now removed features
- **Fixed** for any bug fixes
- **Security** for vulnerability fixes

For detailed commit-level changes, see [GitHub Releases](https://github.com/reyamira/pass-cli/releases).
