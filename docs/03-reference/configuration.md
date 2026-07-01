---
title: "Configuration"
weight: 2
toc: true
---

Complete configuration options for pass-cli including vault location, clipboard settings, TUI theme, and keyboard shortcuts.

**Configuration Location**:
- **All platforms**: `~/.pass-cli/config.yml`

**Management Commands**:
```bash
# Initialize default config
pass-cli config init

# Edit config in default editor
pass-cli config edit

# Validate config syntax
pass-cli config validate

# Reset to defaults
pass-cli config reset
```

### Example Configuration

```yaml
# Custom vault location (optional)
vault_path: /custom/path/vault.enc  # Supports env vars ($HOME), tilde (~), relative, absolute paths

# TUI theme (optional)
theme: "dracula"  # Valid values: dracula, nord, gruvbox, monokai (default: dracula)

# Terminal display thresholds (TUI mode)
terminal:
  # Enable terminal size warnings (default: true)
  warning_enabled: true
  min_width: 60   # Minimum columns (default: 60)
  min_height: 30  # Minimum rows (default: 30)
  # Detail panel positioning (default: auto)
  detail_position: "auto"  # Valid values: auto, right, bottom
  # Width threshold for auto positioning (default: 120)
  detail_auto_threshold: 120  # Range: 80-500

# Cloud sync configuration (optional)
sync:
  enabled: false              # Enable rclone-based sync
  remote: "gdrive:.pass-cli"  # rclone remote:path

# Custom keyboard shortcuts (TUI mode)
keybindings:
  quit: "q"                  # Quit application
  add_credential: "a"        # Add new credential
  edit_credential: "e"       # Edit credential
  delete_credential: "d"     # Delete credential
  toggle_detail: "i"         # Toggle detail panel
  toggle_sidebar: "s"        # Toggle sidebar
  help: "?"                  # Show help modal
  search: "/"                # Activate search
  confirm: "enter"           # Confirm actions in forms
  cancel: "esc"                # Cancel actions in forms

# Supported key formats for keybindings:
# - Single letters: a-z
# - Numbers: 0-9
# - Function keys: f1-f12
# Modifiers: ctrl+, alt+, shift+
# Examples: ctrl+q, alt+a, shift+f1
```

### Vault Path Configuration

The `vault_path` config field supports flexible path formats:

**Environment Variables (Unix):**
```yaml
vault_path: $HOME/.pass-cli/vault.enc
vault_path: $HOME/secure/vault.enc
```

**Environment Variables (Windows):**
```yaml
vault_path: %USERPROFILE%\Documents\vault.enc
```

**Tilde Expansion:**
```yaml
vault_path: ~/Dropbox/vault.enc
vault_path: ~/.pass-cli/vault.enc
```

**Relative Paths** (resolved relative to home directory):
```yaml
vault_path: vault.enc  # Resolved to $HOME/vault.enc
```

**Absolute Paths:**
```yaml
vault_path: /custom/absolute/path/vault.enc
```

If `vault_path` is not specified, defaults to `~/.pass-cli/vault.enc`.

### Keybinding Customization

**Configurable Actions**:
- `quit`, `add_credential`, `edit_credential`, `delete_credential`
- `toggle_detail`, `toggle_sidebar`, `help`, `search`

**Hardcoded Shortcuts** (cannot be changed):
- Navigation: Tab, Shift+Tab, ↑/↓, Enter, Esc
- Forms: Ctrl+P, Ctrl+S, Ctrl+C
- Detail view: p, c

**Validation**:
- Duplicate key assignments rejected (conflict detection)
- Unknown actions rejected
- Invalid config shows warning modal, app continues with defaults
- UI hints automatically update to reflect custom keybindings

### Sync Configuration

Enable cloud synchronization using [rclone](https://rclone.org/) to access your vault across multiple devices.

```yaml
sync:
  enabled: true
  remote: "gdrive:.pass-cli"
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable/disable rclone sync |
| `remote` | string | `""` | rclone remote and path |
| `pull_ttl_seconds` | int | `0` (30s) | Window in which a command serves the local vault without re-probing the remote; also the failure-backoff window. `0` uses the default (30s); negative disables the gate (probe every command). |
| `probe_timeout_seconds` | int | `0` (8s) | Timeout for the pre-unlock remote metadata probe. Raise it for a slow/high-latency remote so it isn't misclassified as failed. `0` uses the default (8s); negative disables the bound (unbounded probe). Heavy pull/push transfers are always unbounded. |

**Sync Behavior**:
- **Pull**: Happens once per CLI session (before first vault access)
- **Push**: Happens after every write operation (add, update, delete)
- **Graceful degradation**: Sync failures warn but don't block operations

**Remote Path Examples**:
```yaml
# Google Drive
remote: "gdrive:.pass-cli"

# Dropbox
remote: "dropbox:Apps/pass-cli"

# OneDrive
remote: "onedrive:Documents/pass-cli"

# S3-compatible
remote: "s3:my-bucket/pass-cli"
```

**Prerequisites**:
1. Install rclone: `brew install rclone` (macOS) or `scoop install rclone` (Windows)
2. Configure remote: `rclone config`
3. Test connectivity: `rclone ls <remote>:`

**Validation**: When `enabled: true`, the `remote` field must not be empty.

See the [Cloud Sync Guide](../02-guides/sync-guide) for detailed setup instructions.

### Configuration Priority

1. Command-line flags (highest priority)
2. Environment variables
3. Configuration file
4. Built-in defaults (lowest priority)

## Vault Metadata Structure

The vault file stores metadata alongside encrypted credential data. This metadata is not encrypted and is used to manage key derivation and decryption.

### Metadata Fields

| Field | Type | V1 | V2 | Description |
|-------|------|----|----|-------------|
| `version` | int | ✓ | ✓ | Vault format version (1 or 2) |
| `created_at` | timestamp | ✓ | ✓ | ISO 8601 creation timestamp |
| `updated_at` | timestamp | ✓ | ✓ | ISO 8601 last update timestamp |
| `salt` | bytes (base64) | ✓ | ✓ | 32-byte salt for password key derivation (PBKDF2) |
| `iterations` | int | ✓ | ✓ | PBKDF2 iteration count (minimum 100,000 per OWASP 2023) |
| `wrapped_dek` | bytes (base64) | - | ✓ | Password-wrapped Data Encryption Key (48 bytes: 32-byte key + 16-byte auth tag) |
| `wrapped_dek_nonce` | bytes (base64) | - | ✓ | GCM nonce for DEK wrapping (12 bytes) |
| `recovery_wrapped_dek` | bytes (base64) | - | ✓ | Recovery-phrase-wrapped DEK (48 bytes) |
| `recovery_wrapped_dek_nonce` | bytes (base64) | - | ✓ | GCM nonce for recovery wrapping (12 bytes) |
| `recovery_salt` | bytes (base64) | - | ✓ | 32-byte salt for recovery phrase key derivation |
| `audit_salt` | bytes (base64) | ✓ | ✓ | 32-byte salt for portable audit key derivation (sync mode) |

### Version Differences

**V1 Format**:
- Direct password-based encryption
- Password is derived using PBKDF2 with stored salt and iterations
- Derived key directly encrypts vault data (AES-256-GCM)
- Simple but doesn't support recovery mechanisms

**V2 Format**:
- Key wrapping architecture with Data Encryption Key (DEK)
- Password-derived KEK wraps the DEK (for password-based unlock)
- Recovery-derived KEK wraps the DEK (for recovery phrase unlock)
- DEK encrypts vault data (AES-256-GCM)
- Supports both password and recovery phrase authentication
- Enables secure recovery without password knowledge

### V2 Vault Metadata Example

```json
{
  "metadata": {
    "version": 2,
    "created_at": "2025-12-05T10:30:00Z",
    "updated_at": "2025-12-05T14:22:45Z",
    "salt": "abcd1234efgh5678ijkl9012mnop3456qrst7890uvwx1234yzab5678cdef90",
    "iterations": 600000,
    "wrapped_dek": "base64encodedwrappeddek48bytes==",
    "wrapped_dek_nonce": "base64encodednonce12bytes==",
    "recovery_wrapped_dek": "base64encodedrecoverydek48bytes==",
    "recovery_wrapped_dek_nonce": "base64encodedrecoverynonce12bytes=="
  },
  "data": "base64encodedencryptedvaultdata=="
}
```

### V1 Vault Metadata Example

```json
{
  "metadata": {
    "version": 1,
    "created_at": "2025-12-05T10:30:00Z",
    "updated_at": "2025-12-05T10:30:00Z",
    "salt": "abcd1234efgh5678ijkl9012mnop3456qrst7890uvwx1234yzab5678cdef90",
    "iterations": 100000
  },
  "data": "base64encodedencryptedvaultdata=="
}
```

### Key Derivation

**V1 Password Path**:
```text
password + salt + iterations
    ↓ PBKDF2
encryption_key (32 bytes)
    ↓ AES-256-GCM
encrypted vault data
```

**V2 Password Path**:
```text
password + salt + iterations
    ↓ PBKDF2
password KEK (32 bytes)
    ↓ AES-256-GCM (with wrapped_dek + wrapped_dek_nonce)
DEK (32 bytes)
    ↓ AES-256-GCM
encrypted vault data
```

**V2 Recovery Path**:
```text
recovery phrase + recovery_salt
    ↓ Argon2id
recovery KEK (32 bytes)
    ↓ AES-256-GCM (with recovery_wrapped_dek + recovery_wrapped_dek_nonce)
DEK (32 bytes)
    ↓ AES-256-GCM
encrypted vault data
```

### Field Specifications

**salt**: 32 cryptographic random bytes used as input to PBKDF2. Each vault has unique salt to prevent rainbow table attacks.

**iterations**: PBKDF2 iteration count. Default is 600,000 (current OWASP recommendation). Minimum required is 100,000 for backward compatibility. Can be configured via `PASS_CLI_ITERATIONS` environment variable.

**wrapped_dek**: AES-256-GCM ciphertext containing the encrypted DEK. Total size is 48 bytes (32-byte key + 16-byte authentication tag). Authentication tag ensures integrity and authenticity.

**wrapped_dek_nonce**: 12-byte GCM nonce used during DEK encryption. Must be unique for each wrap operation to maintain security.

**recovery_wrapped_dek**: AES-256-GCM ciphertext containing the DEK encrypted with recovery KEK. Allows vault access using recovery phrase instead of password.

**recovery_salt**: 32 cryptographic random bytes used in recovery key derivation. Separate from password salt to avoid key reuse.

For detailed information on key wrapping architecture and security properties, refer to `internal/crypto/keywrap.go` in the source code.

