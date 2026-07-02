---
title: "Command Reference"
weight: 1
toc: true
---
Complete reference for all pass-cli commands and their options.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)

## Global Options

Available for all commands:

| Flag | Description | Example |
|------|-------------|---------|
| `--verbose` | Enable verbose output | `--verbose` |
| `--offline` | Skip cloud sync for this command (run fully local) | `--offline` |
| `--help`, `-h` | Show help | `--help` |

### Global Flag Examples

```bash
# Enable verbose logging
pass-cli --verbose get github

# Run fully local — skip the pre-unlock pull and post-command push
# (useful in CI or tight loops; offline changes sync on the next online run)
pass-cli --offline get github

# Get help for any command
pass-cli get --help
```

### Custom Vault Location

To use a custom vault location, configure it in your config file (`~/.pass-cli/config.yml`):

```yaml
vault_path: /custom/path/vault.enc
```

See [Configuration](#configuration) section for details on path expansion (environment variables, tilde, relative paths).

## Commands

### init - Initialize Vault

Create a new password vault or connect to an existing synced vault.

#### Synopsis

```bash
pass-cli init [flags]
```

#### Description

Creates a new encrypted vault at `~/.pass-cli/vault.enc` or connects to an existing vault on a synced remote (via rclone). You will be prompted to choose between creating a new vault or connecting to an existing one.

When creating a new vault, you'll be prompted to create a master password and optionally enable cloud sync. When connecting to an existing vault, pass-cli will download it from your configured rclone remote.

#### Examples

```bash
# Initialize vault (interactive - choose create new or connect to existing)
pass-cli init

# Skip sync setup prompts
pass-cli init --no-sync

# For custom vault location, configure in config file first:
# Edit ~/.pass-cli/config.yml and add: vault_path: /custom/path/vault.enc
# Then run: pass-cli init
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--no-audit` | bool | Disable tamper-evident audit logging (enabled by default) |
| `--use-keychain` | bool | Store master password in OS keychain |
| `--no-recovery` | bool | Skip BIP39 recovery phrase generation |
| `--no-sync` | bool | Skip cloud sync setup prompts |

#### Password Policy

All master passwords must meet complexity requirements:
- **Minimum Length**: 12 characters
- **Uppercase**: At least one uppercase letter (A-Z)
- **Lowercase**: At least one lowercase letter (a-z)
- **Digit**: At least one digit (0-9)
- **Symbol**: At least one special symbol (!@#$%^&*()-_=+[]{}|;:,.<>?)

**Examples**:
- [OK] `MySecureP@ssw0rd2025!` (meets all requirements)
- [OK] `Correct-Horse-Battery-29!` (meets all requirements)
- [ERROR] `password123` (too short, no uppercase, no symbol)
- [ERROR] `MyPassword` (no digit, no symbol)

#### Audit Logging (Enabled by Default)

Audit logging records vault operations with HMAC signatures. It is **enabled by default** during initialization.

```bash
# Initialize vault (audit logging enabled by default)
pass-cli init

# Disable audit logging if not desired
pass-cli init --no-audit
```

**Audit features**:
- **Tamper-Evident**: HMAC-SHA256 signatures prevent log modification
- **Privacy**: Service names logged, passwords NEVER logged
- **Key Storage**: HMAC keys stored in OS keychain (separate from vault)
- **Auto-Rotation**: Logs rotate at 10MB with 7-day retention
- **Graceful Degradation**: Operations continue if logging fails

**Verification**:
```bash
# Verify audit log integrity
pass-cli verify-audit
```

#### BIP39 Recovery Phrase (Enabled by Default)

Pass-CLI generates a 24-word BIP39 recovery phrase during initialization. This allows you to recover vault access if you forget your master password.

**Default behavior (recovery enabled):**
```bash
pass-cli init

# You'll be prompted to:
# 1. Create master password
# 2. Write down 24-word recovery phrase
# 3. Verify recovery phrase (3 random words)
```

**Skip recovery phrase:**
```bash
# If you prefer keychain-only approach
pass-cli init --no-recovery
```

**Recovery phrase features**:
- **Industry Standard**: Uses BIP39 (same as hardware wallets)
- **Secure**: 6-word challenge = 2^66 combinations (~73.8 quintillion)
- **Offline Storage**: Write on paper, store in safe
- **Optional Passphrase**: Add 25th word for additional protection
- **Recovery Command**: `pass-cli change-password --recover`

**When to skip recovery phrase:**
- You use keychain integration and trust OS keychain
- You keep master password in another password manager
- You prefer single point of failure (master password only)

For detailed recovery procedures, see [Recovery Phrase Guide](../02-guides/recovery-phrase.md) and [Security Architecture](security-architecture.md#bip39-recovery-phrase).

#### Notes

- Master password must meet complexity requirements (12+ chars, uppercase, lowercase, digit, symbol)
- Strong passwords (20+ characters) recommended for master password
- Master password is stored in OS keychain for convenience (unless disabled)
- Recovery phrase is enabled by default (use `--no-recovery` to skip)
- Write recovery phrase on paper and store securely (safe, safety deposit box)
- Vault file is created with restricted permissions (0600)
- Audit logging is opt-in (disabled by default)

---

### add - Add Credential

Add a new credential to the vault.

#### Synopsis

```bash
pass-cli add <service> [flags]
```

#### Flags

| Flag | Short | Type | Description |
|------|-------|------|-------------|
| `--username` | `-u` | string | Username for the credential |
| `--password` | `-p` | string | Password (not recommended, use prompt) |
| `--generate` | `-g` | bool | Generate a random secure password |
| `--gen-length` | | int | Length of generated password (default: 20) |
| `--category` | `-c` | string | Category for organizing credentials (e.g., 'Cloud', 'Databases') |
| `--url` | | string | Service URL |
| `--notes` | | string | Additional notes |
| `--totp` | | bool | Prompt for TOTP secret interactively |
| `--totp-uri` | | string | TOTP URI (otpauth://totp/...) |

#### Examples

```bash
# Interactive mode (prompts for username/password)
pass-cli add github

# With username flag
pass-cli add github --username user@example.com

# With URL and notes
pass-cli add github \
  --username user@example.com \
  --url https://github.com \
  --notes "Personal account"

# With category
pass-cli add github -u user@example.com -c "Version Control"

# Generate random password (16 characters)
pass-cli add github -u user@example.com --generate

# Generate random password with custom length
pass-cli add github -u user@example.com --generate --gen-length 24

# Generate password with other metadata
pass-cli add github \
  -u user@example.com \
  --generate \
  --gen-length 20 \
  --url https://github.com \
  --notes "Work account"

# All flags (not recommended for password)
pass-cli add github \
  -u user@example.com \
  -p secret123 \
  --url https://github.com \
  --notes "Work account"

# Add credential with TOTP (interactive prompt for secret)
pass-cli add github --totp

# Add credential with TOTP URI directly
pass-cli add github --totp-uri "otpauth://totp/GitHub:user?secret=JBSWY3DPEHPK3PXP&issuer=GitHub"
```

> **Tip**: When adding TOTP, the `Service` and `Username` fields are used as defaults for the QR code's issuer and account name. See the [TOTP & 2FA Guide](../02-guides/totp-guide) for details on how these fields are used.

#### Interactive Prompts

When not using flags, you'll be prompted:

```text
Enter username: user@example.com
Enter password: ******* (hidden input)
Enter URL (optional): https://github.com
Enter notes (optional): Personal account
```

#### Password Policy

Credential passwords must meet the same complexity requirements as master passwords:
- Minimum 12 characters with uppercase, lowercase, digit, and symbol
- TUI mode shows real-time strength indicator
- Generated passwords automatically meet policy requirements

#### Notes

- Service names must be unique
- Password input is hidden by default
- Passing password via `-p` flag is insecure (visible in shell history)
- Use `pass-cli generate` to create strong random passwords that meet policy requirements
- Usage tracking begins when credential is first accessed
- **Sync**: Pushes changes after completion (displays `Syncing... done` when sync is enabled)

---

### get - Retrieve Credential

Retrieve a credential from the vault.

#### Synopsis

```bash
pass-cli get <service> [flags]
```

#### Flags

| Flag | Short | Type | Description |
|------|-------|------|-------------|
| `--quiet` | `-q` | bool | Output password only (for scripts) |
| `--field` | `-f` | string | Extract specific field |
| `--no-clipboard` | | bool | Skip clipboard copy |
| `--masked` | | bool | Display password as asterisks |
| `--totp` | | bool | Generate and display TOTP code |
| `--totp-qr` | | bool | Display TOTP QR code in terminal |
| `--totp-qr-file` | | string | Export TOTP QR code to PNG file |

#### Field Options

For `--field` flag:
- `username` - User's username
- `password` - User's password
- `category` - Credential category
- `url` - Service URL
- `notes` - Additional notes
- `service` - Service name
- `created` - Creation timestamp
- `modified` - Last modified timestamp
- `accessed` - Last accessed timestamp

#### Examples

```bash
# Default: Display credential and copy to clipboard
pass-cli get github

# Quiet mode (password only, for scripts)
pass-cli get github --quiet
pass-cli get github -q

# Get specific field
pass-cli get github --field username
pass-cli get github -f url

# Quiet mode with specific field
pass-cli get github --field username --quiet

# Display without clipboard
pass-cli get github --no-clipboard

# Display with masked password
pass-cli get github --masked

# Generate TOTP code (if TOTP configured for credential)
pass-cli get github --totp

# TOTP code only (for scripts)
pass-cli get github --totp --quiet

# Display TOTP QR code in terminal (to add to another device)
pass-cli get github --totp-qr

# Export TOTP QR code to file (use with caution - contains secret)
pass-cli get github --totp-qr-file totp-github.png
```

#### TOTP URI Labeling (Service & Username)

When generating a TOTP QR code or URI, Pass-CLI uses the following fields to identify the account in your authenticator app:

- **Issuer**: Uses `TOTPIssuer` field, or falls back to `Service` name if empty.
- **Account**: Uses `Username` field, or falls back to `Service` name if empty.

**Best Practice**: Set the `Username` field to distinguish between multiple accounts at the same service. If you only have one account, leaving `Username` empty is fine as the service name will be used.

For more details on TOTP configuration and usage, see the [TOTP & 2FA Guide](../02-guides/totp-guide).

#### Output Examples

**Default output:**
```text
Service:  github
Username: user@example.com
Password: mySecretPassword123!
URL:      https://github.com
Notes:    Personal account

[PASS] Password copied to clipboard (will clear in 5 seconds)
```

**Quiet mode:**
```bash
$ pass-cli get github --quiet
mySecretPassword123!
```

**Field extraction:**
```bash
$ pass-cli get github --field username --quiet
user@example.com
```

**TOTP code generation:**
```bash
$ pass-cli get github --totp
123456
```

#### Notes

- Clipboard auto-clears after 5 seconds
- Usage tracking records current directory
- Accessing a credential updates the "last accessed" timestamp
- **Sync**: Read-only — does not trigger a sync push (even with sync enabled)

---

### exec - Run a Command with Injected Credentials

Run a child command with stored credentials injected as environment variables.

#### Synopsis

```bash
pass-cli exec [<service>] -- <command> [args...]
```

#### Description

`exec` runs a child command with stored credentials injected as **environment variables**. The secret value is passed only through the child process's environment — it never touches a file, the clipboard, or your shell history.

Everything after the `--` separator is the command to run. There **must** be a `--` and a command after it. pass-cli writes nothing of its own to stdout, and the child's exit code is **propagated unchanged**.

`exec` is read-only: it does **not** record usage (no usage tracking) and does **not** trigger a sync push, so repeated invocations on a hot path (or inside an agent) don't mutate the vault on every run.

#### Mapping Forms

There are two ways to map credentials to environment variables:

**Explicit mapping (`--set`, repeatable):**

```bash
# --set ENV_NAME=service[:field]
pass-cli exec --set GITHUB_TOKEN=github -- gh repo list
```

**Convenience form (derive the env name from the service):**

The env name is derived from the service name by uppercasing it and replacing every non-alphanumeric character with `_`. For example, `openai-api` becomes `OPENAI_API`.

```bash
pass-cli exec openai-api -- python train.py   # sets OPENAI_API
```

The positional `<service>` form and `--set` are mutually exclusive — use one or the other.

#### Field Selection

The `-f`/`--field` flag selects which field to inject (default: `password`) and applies to **all** mappings. Valid fields:

- `username`
- `password`
- `category`
- `url`
- `notes`
- `service`

A single mapping can override the global field with a `:field` suffix (`--set ENV=service:field`). The per-mapping `:field` always wins over `--field`. This is how you inject two fields of the **same** entry as separate variables (for example, a database username and password):

```bash
pass-cli exec \
  --set DB_USER=postgres:username \
  --set DB_PASSWORD=postgres:password \
  -- ./run.sh
```

**Value filters (`| filter`):** a reference may end with a filter that transforms the value before injection. The same grammar works everywhere a reference is parsed — `--set`, `.pass-cli.toml` manifests, and `${pass:...}` templates (`inject`, `exec --env-file`).

| Filter | Result |
|--------|--------|
| `base64` | standard base64 of the value (e.g. a Bearer token) |
| `base64url` | URL-safe base64 of the value |
| `basicauth` | `base64("username:password")` from the credential — for an HTTP `Authorization: Basic` header; takes no field |

```bash
# base64 a token; combine a credential's user:password for Basic auth
pass-cli exec \
  --set 'TOKEN=api/token|base64' \
  --set 'AUTH=api|basicauth' \
  -- ./run.sh
```

One filter per reference. An unknown or empty filter is a hard error and nothing is injected (fail closed); for `--set` and manifests it is caught before the vault is even opened. On the command line the `|` must be **shell-quoted** (as above). Filters are not applied to the positional `<service>` form — use `--set` or a template.

#### Flags

| Flag | Short | Type | Description |
|------|-------|------|-------------|
| `--set` | | string (repeatable) | Map an environment variable to a credential: `ENV_NAME=service[:field]` |
| `--field` | `-f` | string | Field to inject for all mappings (default: `password`); per-mapping `:field` overrides it |

#### Examples

```bash
# Inject a password as GITHUB_TOKEN and run gh
pass-cli exec --set GITHUB_TOKEN=github -- gh repo list

# Multiple credentials at once
pass-cli exec \
  --set AWS_ACCESS_KEY_ID=aws-id \
  --set AWS_SECRET_ACCESS_KEY=aws-secret \
  -- aws s3 ls

# Inject a non-password field for all mappings
pass-cli exec --set DB_USER=postgres --field username -- ./run-migration.sh

# Inject two fields of one entry as separate variables (per-mapping override)
pass-cli exec \
  --set DB_USER=postgres:username \
  --set DB_PASSWORD=postgres:password \
  -- ./run-migration.sh

# Convenience form: service name -> env name (openai-api -> OPENAI_API)
pass-cli exec openai-api -- python train.py
```

#### Security Note

The injected value lives in the **child process's environment**. On Linux it is readable via `/proc/<pid>/environ` by the same user, and it is inherited by descendant processes. This is the same model as `op run` and `aws-vault exec` — far safer than files, clipboards, or shell history, but it is **not** process isolation.

#### Notes

- A `--` separator and a command after it are **required**.
- The child's exit code is propagated unchanged; pass-cli writes nothing to stdout itself.
- **Sync**: Read-only — does not record usage and does not trigger a sync push (even with sync enabled).

---

### list - List Credentials

List all credentials in the vault.

#### Synopsis

```bash
pass-cli list [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--format`, `-f` | string | Output format: table, json, simple (default: table) |
| `--show-usernames` | bool | Include the Username column in the table (hidden by default) |
| `--quiet`, `-q` | bool | Print bare service names only (alias for `--format simple`; takes precedence over `--format`) |
| `--unused` | bool | Show only unused credentials |
| `--days` | int | Days threshold for unused (default: 30) |
| `--by-project` | bool | Group credentials by git repository |
| `--location` | string | Filter credentials by directory path |
| `--recursive` | bool | Include subdirectories with --location |

#### Username Safety (Default)

By default, the table output **hides the Username column**. The `username` field can hold sensitive values (card, account, or routing numbers stored as a username), so a broad `list` should not dump them. Pass `--show-usernames` to add the column back (it is omitted entirely, not masked).

The `--format json` output is unchanged: it is an explicit, structured opt-in and still emits the full metadata, **including usernames**.

#### Examples

```bash
# List all credentials (table format, usernames hidden)
pass-cli list

# Include the username column (may reveal sensitive values)
pass-cli list --show-usernames

# List bare service names, one per line
pass-cli list -q

# List as JSON (includes usernames)
pass-cli list --format json

# Simple list (service names only)
pass-cli list --format simple

# Show unused credentials (not accessed in 30 days)
pass-cli list --unused

# Show credentials not used in 90 days
pass-cli list --unused --days 90

# Group credentials by git repository
pass-cli list --by-project

# Group by project with JSON output
pass-cli list --by-project --format json

# Filter by location (current directory)
pass-cli list --location .

# Filter by location with subdirectories
pass-cli list --location /home/user/projects --recursive

# Combine location filter with project grouping
pass-cli list --location ~/work --by-project --recursive
```

#### Output Examples

**Table format (default — usernames hidden):**
```text
+----------+-----------+---------------------+---------------------+
| SERVICE  | USAGE     | LAST USED           | CREATED             |
+----------+-----------+---------------------+---------------------+
| github   | 12 (2 loc)| 2 hours ago         | 3 months ago        |
| aws-prod | 5         | 2 days ago          | 6 months ago        |
| database | Never     | Never               | 1 month ago         |
+----------+-----------+---------------------+---------------------+
```

**Table format with `--show-usernames`:**
```text
+----------+----------------------+-----------+---------------------+---------------------+
| SERVICE  | USERNAME             | USAGE     | LAST USED           | CREATED             |
+----------+----------------------+-----------+---------------------+---------------------+
| github   | user@example.com     | 12 (2 loc)| 2 hours ago         | 3 months ago        |
| aws-prod | admin@company.com    | 5         | 2 days ago          | 6 months ago        |
| database | dbuser               | Never     | Never               | 1 month ago         |
+----------+----------------------+-----------+---------------------+---------------------+
```

**Simple format (also `-q`/`--quiet`):**
```text
github
aws-prod
database
```

#### Notes

- Passwords are never shown in list output
- Usernames are hidden in the table by default; use `--show-usernames` to include them (the `username` field can hold sensitive values)
- `--format json` still includes usernames (explicit structured opt-in)
- `-q`/`--quiet` is shorthand for `--format simple` and takes precedence over `--format`
- Table format is best for human viewing
- JSON format is best for parsing
- Simple format is best for shell scripts

---

### update - Update Credential

Update an existing credential.

#### Synopsis

```bash
pass-cli update <service> [flags]
```

#### Flags

| Flag | Short | Type | Description |
|------|-------|------|-------------|
| `--username` | `-u` | string | New username |
| `--password` | `-p` | string | New password (not recommended) |
| `--generate` | `-g` | bool | Generate a random secure password |
| `--gen-length` | | int | Length of generated password (default: 20) |
| `--category` | | string | New category |
| `--url` | | string | New URL |
| `--notes` | | string | New notes |
| `--totp-uri` | | string | New TOTP URI (otpauth://totp/...) |
| `--clear-totp` | | bool | Clear TOTP configuration |
| `--clear-category` | | bool | Clear category field to empty |
| `--clear-notes` | | bool | Clear notes field to empty |
| `--clear-url` | | bool | Clear URL field to empty |
| `--force` | `-f` | bool | Skip confirmation prompt |

#### Examples

```bash
# Update password (prompted)
pass-cli update github

# Update username
pass-cli update github --username newuser@example.com

# Update URL
pass-cli update github --url https://github.com/enterprise

# Update notes
pass-cli update github --notes "Updated account info"

# Update category
pass-cli update github --category "Work"

# Update TOTP configuration
pass-cli update github --totp-uri "otpauth://totp/GitHub:user?secret=JBSWY3DPEHPK3PXP&issuer=GitHub"

# Clear TOTP configuration
pass-cli update github --clear-totp

# Generate new random password (16 characters)
pass-cli update github --generate

# Generate new password with custom length
pass-cli update github --generate --gen-length 32

# Generate password and update other fields
pass-cli update github \
  --generate \
  --gen-length 24 \
  --notes "Password rotated on 2025-11-11"

# Clear category field
pass-cli update github --clear-category

# Update multiple fields
pass-cli update github \
  --username newuser@example.com \
  --url https://github.com/enterprise \
  --notes "Corporate account"
```

#### Interactive Mode

If no flags provided, prompts for password:

```text
Enter new password (leave blank to keep current): *******
Password updated successfully!
```

#### Notes

- At least one field must be updated
- Updating password clears usage history
- Original values preserved if not specified
- **Sync**: Pushes changes after completion (displays `Syncing... done` when sync is enabled)

---

### delete - Delete Credential

Delete a credential from the vault.

#### Synopsis

```bash
pass-cli delete <service> [flags]
```

#### Aliases

`rm`, `remove`

#### Flags

| Flag | Short | Type | Description |
|------|-------|------|-------------|
| `--force` | `-f` | bool | Skip confirmation prompt |

#### Examples

```bash
# Delete with confirmation
pass-cli delete github

# Force delete (no confirmation)
pass-cli delete github --force
pass-cli delete github -f
```

#### Interactive Confirmation

Without `--force`:

```text
Are you sure you want to delete 'github'? (yes/no): yes
Credential 'github' deleted successfully!
```

#### Notes

- Deletion is permanent (no undo)
- Confirmation required unless using `--force`
- Credential completely removed from vault
- **Sync**: Pushes changes after completion (displays `Syncing... done` when sync is enabled)

---

### change-password - Change Master Password

Change the master password used to encrypt and decrypt your vault.

#### Synopsis

```bash
pass-cli change-password [flags]
```

#### Description

Re-encrypts your entire vault with a new master password. You must provide your current password to authorize the change. The new password must meet the password policy requirements.

If you forgot your master password, you can use the `--recover` flag to recover access using your BIP39 recovery phrase (if enabled during vault initialization).

**Password Policy Requirements:**
- Minimum 12 characters
- At least one uppercase letter
- At least one lowercase letter
- At least one digit
- At least one special symbol (!@#$%^&*()-_=+[]{}|;:,.<>?)

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--recover` | bool | Use BIP39 recovery phrase instead of current password |

#### Examples

```bash
# Change master password (normal flow)
pass-cli change-password

# Recover access with BIP39 recovery phrase (if password forgotten)
pass-cli change-password --recover
```

#### Interactive Flow

**Normal password change:**
```text
🔐 Change Master Password
📁 Vault location: /home/user/.pass-cli/vault.enc

Enter current master password: ********

Enter new master password (min 12 characters with uppercase, lowercase, digit, symbol): ********

Password strength: Strong [OK]
- Length: 16 characters [OK]
- Uppercase: Yes [OK]
- Lowercase: Yes [OK]
- Digits: Yes [OK]
- Symbols: Yes [OK]

Confirm new master password: ********

[OK] Master password changed successfully!
```

**Recovery flow with BIP39 phrase:**
```text
🔐 Recover Vault Access
📁 Vault location: /home/user/.pass-cli/vault.enc

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
BIP39 Recovery Phrase Challenge
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

You will be asked to provide 6 words from your 24-word recovery phrase.

Enter word #7: device
[PASS] (1/6)

Enter word #12: diesel
[PASS] (2/6)

Enter word #18: identify
[PASS] (3/6)

Enter word #3: about
[PASS] (4/6)

Enter word #22: spin
[PASS] (5/6)

Enter word #15: hybrid
[PASS] (6/6)

[OK] Recovery phrase verified successfully!

Enter new master password: ********
Confirm new master password: ********

[OK] Master password changed successfully!
Your vault has been re-encrypted with the new password.
```

#### Keychain Integration

If keychain integration is enabled, the new password is automatically stored in your OS keychain, replacing the old one.

#### Security Notes

- **Current Password Required**: You must authenticate with your current password (or use `--recover`)
- **Recovery Phrase**: Use `--recover` flag if you forgot your master password (requires 24-word phrase)
- **Recovery Requirements**: Recovery only works if BIP39 phrase was enabled during `pass-cli init`
- **Policy Enforcement**: New password must meet all security requirements
- **Re-encryption**: All credentials are re-encrypted with the new password
- **Atomic Operation**: Vault is not modified if re-encryption fails
- **Audit Logging**: Password changes are logged (if audit logging enabled)

#### Recovery Prerequisites

To use `--recover` flag, you must:
1. Have enabled BIP39 recovery during `pass-cli init` (default behavior)
2. Have your 24-word recovery phrase written down
3. Be able to provide 6 random words from the phrase when challenged

If you used `--no-recovery` during initialization, the `--recover` flag will not work.

#### See Also

- [Password Policy](security-architecture#password-policy) - Password policy details
- [BIP39 Recovery](security-architecture#bip39-recovery-phrase) - Recovery phrase details
- [Recovery Guide](../02-guides/recovery-phrase) - Detailed recovery procedures
- [Keychain Setup](../02-guides/keychain-setup) - Keychain integration

---

### generate - Generate Password

Generate a cryptographically secure password.

#### Synopsis

```bash
pass-cli generate [flags]
```

#### Aliases

`gen`, `pwd`

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--length` | int | Password length (8-128, default: 20) |
| `--no-lower` | bool | Exclude lowercase letters |
| `--no-upper` | bool | Exclude uppercase letters |
| `--no-digits` | bool | Exclude digits |
| `--no-symbols` | bool | Exclude symbols |
| `--no-clipboard` | bool | Skip clipboard copy |

#### Examples

```bash
# Generate default password (20 chars, all character types)
pass-cli generate

# Custom length
pass-cli generate --length 32

# Alphanumeric only (no symbols)
pass-cli generate --no-symbols

# Digits and symbols only
pass-cli generate --no-lower --no-upper

# Letters only (no digits or symbols)
pass-cli generate --no-digits --no-symbols

# Display only (no clipboard)
pass-cli generate --no-clipboard
```

#### Character Sets

Default character sets:
- Lowercase: `a-z`
- Uppercase: `A-Z`
- Digits: `0-9`
- Symbols: `!@#$%^&*()_+-=[]{}|;:,.<>?`

#### Notes

- Uses `crypto/rand` for cryptographic randomness
- At least one character set must be enabled
- Minimum length: 8 characters
- Maximum length: 128 characters
- Clipboard auto-clears after 5 seconds

---

### version - Show Version

Display version information including build details.

#### Synopsis

```bash
pass-cli version
```

#### Examples

```bash
# Show version
pass-cli version
```

#### Output

```text
pass-cli X.Y.Z
  commit: abc123f
  built:  2025-01-20T10:30:00Z
```

---

### usage - View Credential Usage History

Display detailed usage history showing where and when a credential was accessed.

#### Synopsis

```bash
pass-cli usage <service> [flags]
```

#### Description

Shows credential access patterns across different working directories, including:
- Location paths (working directories where credential was used)
- Git repository context (if location is within a git repo)
- Last access timestamps
- Access counts per location
- Field-level usage breakdown (username, password, url, notes)

Useful for tracking where credentials are used across projects and identifying stale usage locations.

#### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `<service>` | Yes | Service name to view usage history for |

#### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | `table` | Output format: `table`, `json`, `simple` |
| `--limit` | int | `20` | Max locations to display (0 = unlimited) |

#### Examples

```bash
# View usage history (table format, 20 most recent locations)
pass-cli usage github

# View all locations (no limit)
pass-cli usage aws --limit 0

# View only 5 most recent locations
pass-cli usage postgres --limit 5

# JSON output for scripting
pass-cli usage heroku --format json

# Simple format (location paths only)
pass-cli usage redis --format simple
```

#### Output (Table Format)

```text
Usage History for 'github':

Location                                    Git Repo             Last Access          Count  Fields
──────────────────────────────────────────────────────────────────────────────────────────────────
/home/user/projects/webapp                  [PASS] webapp             2025-11-12 14:30     12     password(8), username(4)
/home/user/projects/api-service             [PASS] api-service        2025-11-10 09:15     5      password(5)
/home/user/scripts                          [FAIL] (not a git repo)   2025-11-08 16:45     3      password(2), url(1)

Total locations: 3
Total accesses: 20
```

#### Output (JSON Format)

```json
{
  "service": "github",
  "locations": [
    {
      "location": "/home/user/projects/webapp",
      "git_repository": "webapp",
      "path_exists": true,
      "last_access": "2025-11-12T14:30:22Z",
      "access_count": 12,
      "field_counts": {
        "password": 8,
        "username": 4
      }
    }
  ],
  "total_locations": 3,
  "total_accesses": 20
}
```

#### Output (Simple Format)

```text
/home/user/projects/webapp
/home/user/projects/api-service
/home/user/scripts
```

#### Notes

- **Path Validation**: Shows [PASS] if location path still exists, [FAIL] if deleted
- **Git Integration**: Detects git repositories and shows repo name
- **Field Tracking**: Counts which credential fields were accessed
- **Automatic**: Usage tracked automatically on every `get` command
- **Location Limit**: Default 20 locations prevents overwhelming output for heavily-used credentials

#### See Also

- [Usage Tracking Guide](../02-guides/usage-tracking) - Comprehensive usage tracking guide

---

### config - Manage Configuration

Manage Pass-CLI configuration settings for terminal warnings and keyboard shortcuts.

#### Synopsis

```bash
pass-cli config <subcommand>
```

#### Description

Configuration file location: `~/.pass-cli/config.yml`

Manages settings for:
- Terminal size warnings and minimum dimensions
- TUI keyboard shortcuts and keybindings
- Vault path location
- Other application preferences

#### Subcommands

##### Config Init

Create configuration file with commented examples.

**Synopsis:**
```bash
pass-cli config init
```

**Description:**
Creates a new config file at `~/.pass-cli/config.yml` with default settings and examples. Fails if file already exists (use `config reset` to overwrite).

**Examples:**
```bash
# Create default config file
pass-cli config init
```

**Output:**
```text
[OK] Configuration file created: /home/user/.pass-cli/config.yml

Edit the file to customize your settings:
- Terminal warnings
- Keyboard shortcuts
- Vault location
```

##### Config Edit

Open configuration file in your default editor.

**Synopsis:**
```bash
pass-cli config edit
```

**Description:**
Opens config file in editor determined by:
1. `EDITOR` environment variable
2. Platform defaults (notepad on Windows, nano/vim on Linux/macOS)

Creates config file with defaults if it doesn't exist.

**Examples:**
```bash
# Edit config file
pass-cli config edit

# Use specific editor
EDITOR=vim pass-cli config edit
```

##### Config Validate

Validate configuration file syntax and settings.

**Synopsis:**
```bash
pass-cli config validate
```

**Description:**
Checks configuration for errors:
- Terminal size ranges (1-10000 width, 1-1000 height)
- Keybinding conflicts (no duplicate key assignments)
- Unknown actions (all keybindings must map to known actions)
- Key format validation

**Exit codes:**
- `0` = Configuration valid
- `1` = Configuration has errors
- `2` = File system error

**Examples:**
```bash
# Validate config
pass-cli config validate
```

**Output (Valid):**
```text
[OK] Configuration is valid

Settings:
  Vault path: ~/.pass-cli/vault.enc
  Terminal warnings: enabled
  Keybindings: 15 custom shortcuts defined
```

**Output (Invalid):**
```text
[ERROR] Configuration has errors:

Line 12: Invalid terminal width: 0 (must be between 1-10000)
Line 25: Duplicate keybinding: Ctrl+S assigned to both 'save' and 'search'
Line 34: Unknown action: 'invalid_action'

Fix these errors and run 'config validate' again.
```

##### Config Reset

Reset configuration to default values.

**Synopsis:**
```bash
pass-cli config reset
```

**Description:**
Resets the configuration file to default values. Creates a backup of your current config at `<config-path>.backup` before overwriting.

**Examples:**
```bash
# Reset config to defaults (creates backup automatically)
pass-cli config reset
```

**Output:**
```text
Config file backed up to /home/user/.pass-cli/config.yml.backup
Config file reset to defaults at /home/user/.pass-cli/config.yml
```

#### See Also

- [Configuration Reference](configuration) - Configuration file reference

---

### keychain - Manage Keychain Integration

Manage system keychain integration for storing vault master passwords.

#### Synopsis

```bash
pass-cli keychain <subcommand>
```

#### Subcommands

##### Keychain Enable

Enable keychain integration for an existing vault by storing the master password in the system keychain.

**Synopsis:**
```bash
pass-cli keychain enable [flags]
```

**Description:**
Stores your vault master password in the OS keychain (Windows Credential Manager, macOS Keychain, or Linux Secret Service). Future commands will not prompt for password when keychain is available. This is useful for vaults created without the `--use-keychain` flag.

**Flags:**

| Flag | Type | Description |
|------|------|-------------|
| `--force` | bool | Overwrite existing keychain entry if already enabled |

**Examples:**
```bash
# Enable keychain for default vault
pass-cli keychain enable

# For custom vault location, configure vault_path in ~/.pass-cli/config.yml

# Force overwrite existing keychain entry
pass-cli keychain enable --force
```

**Output:**
```text
Master password: ********

[OK] Keychain integration enabled for vault at /home/user/.pass-cli/vault.enc

Future commands will not prompt for password when keychain is available.
```

##### Keychain Status

Display keychain integration status for the current vault.

**Synopsis:**
```bash
pass-cli keychain status [flags]
```

**Description:**
Shows keychain availability, password storage status, and backend name. This is a read-only operation that doesn't require unlocking the vault.

**Examples:**
```bash
# Check keychain status for default vault
pass-cli keychain status

# For custom vault location, configure vault_path in ~/.pass-cli/config.yml
```

**Output Examples:**

**When keychain is enabled:**
```text
Keychain Status for /home/user/.pass-cli/vault.enc:

[PASS] System Keychain:        Available (keychain)
[PASS] Password Stored:        Yes
[PASS] Backend:                keychain

Your vault password is securely stored in the system keychain.
Future commands will not prompt for password.
```

**When keychain is available but not enabled:**
```text
Keychain Status for /home/user/.pass-cli/vault.enc:

[PASS] System Keychain:        Available (wincred)
[FAIL] Password Stored:        No

The system keychain is available but no password is stored for this vault.
Run 'pass-cli keychain enable' to store your password and skip future prompts.
```

**When keychain is not available:**
```text
Keychain Status for /home/user/.pass-cli/vault.enc:

[FAIL] System Keychain:        Not available on this platform
[FAIL] Password Stored:        N/A

System keychain is not accessible. You will be prompted for password on each command.
```

#### Platform Support

| Platform | Backend | Service Name |
|----------|---------|--------------|
| Windows | wincred | Windows Credential Manager |
| macOS | keychain | Keychain Access |
| Linux | gnome-keyring/kwallet | Secret Service API |

### vault - Manage Vault Files

Manage pass-cli vault files and their lifecycle.

#### Synopsis

```bash
pass-cli vault <subcommand>
```

#### Subcommands

##### Vault Remove

Permanently delete a vault file and its associated keychain entry.

**Synopsis:**
```bash
pass-cli vault remove <path> [flags]
```

**Description:**
Permanently deletes:
1. The vault file from disk
2. The master password from the system keychain
3. Any orphaned keychain entries

**[WARNING] WARNING:** This operation is irreversible. All stored credentials will be lost. Ensure you have backups before proceeding.

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `<path>` | Yes | Path to vault file to remove |

**Flags:**

| Flag | Type | Description |
|------|------|-------------|
| `-y`, `--yes` | bool | Skip confirmation prompt (for automation) |
| `-f`, `--force` | bool | Force removal even if vault appears in use |

**Examples:**
```bash
# Remove vault with confirmation prompt
pass-cli vault remove /path/to/vault.enc

# Remove vault without confirmation (for automation)
pass-cli vault remove /path/to/vault.enc --yes

# Force removal even if file appears in use
pass-cli vault remove /path/to/vault.enc --force
```

**Output:**
```text
[WARNING]  WARNING: This will permanently delete the vault and all stored credentials.
Are you sure you want to remove /home/user/.pass-cli/vault.enc? (y/n): y

[OK] Vault removed successfully:
   • Vault file deleted
   • Keychain entry removed
   • Orphaned entries cleaned up
```

##### Vault Backup

Manage vault backups for disaster recovery.

**Synopsis:**
```bash
pass-cli vault backup <subcommand>
```

###### Vault Backup Create

Create a timestamped manual backup of the vault.

**Synopsis:**
```bash
pass-cli vault backup create [flags]
```

**Description:**
Creates a manual backup with naming pattern `vault.enc.YYYYMMDD-HHMMSS.manual.backup`. Works without requiring the master password (no vault unlock needed).

**Flags:**

| Flag | Type | Description |
|------|------|-------------|
| `-v`, `--verbose` | bool | Show detailed operation progress |

**Examples:**
```bash
# Create manual backup
pass-cli vault backup create

# Create backup with verbose output
pass-cli vault backup create --verbose
```

**Output:**
```text
[OK] Manual backup created successfully:
   /home/user/.pass-cli/vault.enc.20251112-143022.manual.backup
   Size: 2.45 MB
```

###### Vault Backup Restore

Restore vault from a backup file.

**Synopsis:**
```bash
pass-cli vault backup restore [flags]
```

**Description:**
Restores your vault from a backup file. By default, selects the newest valid backup. Use `--file` to specify a specific backup, or `--interactive` to choose from a list.

**[WARNING] WARNING:** This command overwrites your current vault file. After restore, use the **backup's password** to unlock (if you changed passwords since the backup, use the OLD password).

**Flags:**

| Flag | Type | Description |
|------|------|-------------|
| `--file` | string | Restore from a specific backup file |
| `-i`, `--interactive` | bool | Interactively select from available backups |
| `-f`, `--force` | bool | Skip confirmation prompt |
| `-v`, `--verbose` | bool | Show detailed operation progress |
| `--dry-run` | bool | Preview which backup would be restored (no changes) |

**Examples:**
```bash
# Restore from newest backup (with confirmation)
pass-cli vault backup restore

# Restore from a specific backup file
pass-cli vault backup restore --file vault.enc.20241210-143022.manual.backup

# Interactive mode: choose from available backups
pass-cli vault backup restore --interactive

# Restore without confirmation
pass-cli vault backup restore --force

# Preview which backup would be restored
pass-cli vault backup restore --dry-run

# Restore with detailed progress
pass-cli vault backup restore --verbose
```

**Output:**
```text
⚠️  Warning: After restore, use the backup's master password to unlock.
    If you changed your password since this backup, use the OLD password.

Backup to restore:
  File: /home/user/.pass-cli/vault.enc.20251112-143022.manual.backup
  Type: manual
  Modified: 2025-11-12 14:30:22

Are you sure you want to continue? (y/n): y

[OK] Vault restored successfully from backup
```

###### Vault Backup Preview

Preview credentials inside a backup file without restoring it.

**Synopsis:**
```bash
pass-cli vault backup preview --file <backup-file> [flags]
```

**Description:**
Decrypts a backup file in-memory to show which credentials it contains. Does NOT modify any files. Useful for finding specific credentials before deciding which backup to restore.

**Flags:**

| Flag | Type | Description |
|------|------|-------------|
| `--file` | string | Backup file to preview (required) |
| `-v`, `--verbose` | bool | Show detailed credential information (username, category, created date) |

**Examples:**
```bash
# Preview credentials in a backup
pass-cli vault backup preview --file vault.enc.20241210-143022.manual.backup

# Preview with detailed information
pass-cli vault backup preview --file vault.enc.backup --verbose
```

**Output (basic):**
```text
Enter the backup's master password: ********

Found 5 credential(s) in backup:

  1. github.com
  2. gitlab.com
  3. aws-console
  4. email-work
  5. vpn-office

Backup file: vault.enc.20241210-143022.manual.backup
Modified: 2024-12-10 14:30:22
```

**Output (verbose):**
```text
┌───┬──────────────┬─────────────────┬──────────┬────────────┐
│ # │ Service      │ Username        │ Category │ Created    │
├───┼──────────────┼─────────────────┼──────────┼────────────┤
│ 1 │ github.com   │ myuser          │ dev      │ 2025-10-15 │
│ 2 │ gitlab.com   │ myuser          │ dev      │ 2025-10-16 │
│ 3 │ aws-console  │ admin@company   │ cloud    │ 2025-09-20 │
└───┴──────────────┴─────────────────┴──────────┴────────────┘
```

###### Vault Backup Info

View backup status and information.

**Synopsis:**
```bash
pass-cli vault backup info [flags]
```

**Description:**
Displays all available backups with metadata:
- Backup type (automatic or manual)
- File size and creation time
- Age with warnings for backups >30 days old
- Which backup would be used for restore
- Disk space usage alerts (>5 manual backups)

**Flags:**

| Flag | Type | Description |
|------|------|-------------|
| `-v`, `--verbose` | bool | Show detailed backup information |

**Examples:**
```bash
# View all backups
pass-cli vault backup info

# View with detailed information
pass-cli vault backup info --verbose
```

**Output:**
```text
📦 Backup Status for: /home/user/.pass-cli/vault.enc

Automatic Backup:
  [OK] vault.enc.backup
     Size: 2.45 MB
     Created: 1 day ago (2025-11-11 14:30:22)

Manual Backups:
  [OK] vault.enc.20251112-143022.manual.backup ← Would be used for restore
     Size: 2.45 MB
     Created: 2 hours ago (2025-11-12 14:30:22)

  [OK] vault.enc.20251110-091545.manual.backup
     Size: 2.40 MB
     Created: 2 days ago (2025-11-10 09:15:45)

Total backups: 3
Total disk space: 7.30 MB
```

**See Also:**
- [Backup & Restore Guide](../02-guides/backup-restore) - Comprehensive backup guide

---

##### Vault Migrate

Migrate a vault from v1 to v2 format for working recovery phrase support.

**Synopsis:**
```bash
pass-cli vault migrate
```

**Description:**
Upgrades an existing v1 vault to the v2 format. The v2 format uses a key-wrapping scheme that enables recovery phrases to actually work (v1 recovery phrases were non-functional due to a bug in key derivation).

During migration:
1. Your vault will be re-encrypted with the new v2 format
2. A NEW 24-word recovery phrase will be generated (write it down immediately)
3. Your existing credentials remain unchanged
4. The migration is atomic (all-or-nothing, safe against power loss)

After migration, you can use `pass-cli change-password --recover` to recover your vault using the new recovery phrase if you forget your master password.

**Flags:**

None.

**Examples:**
```bash
# Migrate vault to v2 format
pass-cli vault migrate

# Migration prompts for confirmation and password verification
# It will display the new recovery phrase after successful migration
```

**Interactive Flow:**
```text
🔄 Vault Migration
📁 Vault location: /home/user/.pass-cli/vault.enc

Your vault is using the legacy v1 format.
The v1 format has a bug where recovery phrases cannot unlock the vault.

Migration will:
  • Re-encrypt your vault with the new v2 format
  • Generate a NEW 24-word recovery phrase
  • Preserve all your existing credentials

Proceed with migration? (yes/no): yes

Enter master password: ********

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Your New Recovery Phrase
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Write down these 24 words in order:
 1. about      2. absent    3. accent     4. account    5. achieve
 6. acid       7. acquire   8. across     9. act        10. action
 ... (remaining words)

Verify your backup? (yes/no): yes

Verification (attempt 1/3):
Word at position 7: device
[PASS] (1/6)

... (additional verification prompts)

✅ Vault migrated successfully to v2 format!

Your recovery phrase is now fully functional.
You can use 'pass-cli change-password --recover' if you forget your password.
```

**Important Notes:**

- **Backup Created Automatically**: A backup is created before migration begins
- **New Recovery Phrase**: Your old recovery phrase (if any) becomes invalid after migration
- **Write It Down**: Save the new 24-word phrase in a safe location (paper, safe, etc.)
- **Atomic Migration**: Safe against power loss and system crashes
- **No Credential Loss**: All your stored credentials are preserved exactly
- **Master Password Required**: You must unlock your vault to migrate
- **Optional Passphrase**: You can add a 25th word passphrase for additional security (advanced)

**Passphrase Protection (Advanced):**

For enhanced security, you can add an optional passphrase (25th word) to your recovery phrase:

```bash
pass-cli vault migrate
# When prompted for passphrase protection, select yes
# Enter and confirm your passphrase (stored separately from the phrase)
```

Important if using passphrase:
- You need BOTH the 24 words AND the passphrase to recover your vault
- Store the passphrase separately from your 24-word phrase
- If you lose the passphrase, recovery becomes impossible
- Document your passphrase securely

**Troubleshooting:**

**Vault is already v2 format:**
```
✓ Your vault is already using the v2 format.
  No migration needed - your recovery phrase will work correctly.
```

**Migration Verification Failed:**
If you can't verify the recovery phrase after writing it down, migration allows up to 3 attempts. If all fail, please ensure you write down the phrase correctly before migration completes.

**See Also:**
- [Recovery Phrase Guide](../02-guides/recovery-phrase) - Complete recovery phrase documentation
- [Change Password](./command-reference#change-password---change-master-password) - Use recovery phrase to recover access
- [Backup & Restore Guide](../02-guides/backup-restore) - Backup management

---

### verify-audit - Verify Audit Log Integrity

Verify the integrity of audit log entries by checking HMAC signatures.

#### Synopsis

```bash
pass-cli verify-audit [audit-log-path]
```

#### Description

Verifies that audit log entries have not been tampered with by validating HMAC-SHA256 signatures on each entry. The audit key is retrieved from the OS keychain for verification.

**What It Checks:**
- HMAC signature validity for each log entry
- JSON structure integrity
- Chronological consistency
- Presence of required fields (timestamp, event, outcome)

#### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `audit-log-path` | No | Path to audit log file (defaults to `<vault-dir>/audit.log`) |

#### Flags

None.

#### Environment Variables

| Variable | Description |
|----------|-------------|
| `PASS_AUDIT_LOG` | Custom audit log path (overridden by command argument) |

#### Examples

```bash
# Verify default audit log
pass-cli verify-audit

# Verify specific audit log
pass-cli verify-audit /path/to/audit.log

# Verify with environment variable
PASS_AUDIT_LOG=/custom/audit.log pass-cli verify-audit
```

#### Output (All Valid)

```text
🔍 Verifying audit log: /home/user/.pass-cli/audit.log

[OK] Audit log verification complete:
   Total entries: 127
   Valid entries: 127
   Invalid entries: 0
   Tampered entries: 0

Audit log integrity: VERIFIED [OK]
```

#### Output (Tampered Detected)

```text
🔍 Verifying audit log: /home/user/.pass-cli/audit.log

[WARNING]  Audit log verification failed:
   Total entries: 127
   Valid entries: 123
   Invalid entries: 4
   Tampered entries: 4

Invalid entries detected:
  Line 45: HMAC verification failed (possible tampering)
  Line 67: HMAC verification failed (possible tampering)
  Line 89: Invalid JSON structure
  Line 102: Missing required fields

Audit log integrity: FAILED [ERROR]

CRITICAL: Audit log may have been tampered with or corrupted.
Review the log file and investigate the flagged entries.
```

#### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All entries valid, log integrity verified |
| 1 | Tampered or invalid entries detected |
| 2 | Audit log not found or inaccessible |
| 3 | Audit key not found in keychain |

#### Security Notes

- **HMAC Key Required**: Audit key must exist in OS keychain
- **Tamper Evidence**: Failed verification indicates log modification
- **Read-Only**: Verification does not modify the audit log
- **Automatic Rotation**: Verifies all rotated log files if present

#### Troubleshooting

**Problem**: "audit log not found"
**Solution**: Audit logging is enabled by default. If you initialized with `--no-audit`, re-initialize without that flag.

**Problem**: "failed to retrieve audit key from keychain"
**Solution**: Audit key may have been deleted. Re-initialize the vault with `pass-cli init` (audit enabled by default)

**Problem**: "HMAC verification failed"
**Causes**:
- Audit log manually edited (tampering)
- Log file corrupted
- Audit key changed or regenerated
- File system corruption

#### See Also

- [Audit Logging](security-architecture#audit-logging) - Audit logging architecture
- [Security Operations](../05-operations/security-operations) - Security best practices

---

### sync - Manage Cloud Sync

Manage cloud synchronization for your pass-cli vault using rclone.

#### Synopsis

```bash
pass-cli sync <subcommand>
```

#### Description

Cloud sync uses rclone to synchronize your encrypted vault with cloud storage providers like Google Drive, Dropbox, OneDrive, and many others.

Prerequisites:
- rclone must be installed and configured with at least one remote
- Run 'rclone config' to set up a remote if you haven't already

#### Subcommands

##### sync enable - Enable Cloud Sync on Existing Vault

Enable cloud synchronization for an existing pass-cli vault.

**Synopsis:**
```bash
pass-cli sync enable [flags]
```

**Description:**
Configures an existing vault to sync with a cloud storage provider via rclone. Your encrypted vault will be pushed to the remote after setup.

**Flags:**

| Flag | Description |
|------|-------------|
| `--force` | Overwrite remote if it already contains vault files |

**Examples:**
```bash
# Enable sync interactively
pass-cli sync enable

# Force overwrite if remote already has files
pass-cli sync enable --force
```

**Output:**
```text
Enter your rclone remote path.
Examples:
  gdrive:.pass-cli         (Google Drive)
  dropbox:Apps/pass-cli    (Dropbox)
  onedrive:.pass-cli       (OneDrive)

Remote path: gdrive:.pass-cli
Checking remote connectivity...

✅ Sync enabled successfully!
   Remote: gdrive:.pass-cli

Your vault will now sync automatically on each operation.
Use 'pass-cli doctor' to check sync status.
```

**Prerequisites:**
- An existing pass-cli vault (run 'pass-cli init' first)
- rclone installed and configured with at least one remote

**Notes:**
- Performs automatic validation of remote connectivity
- Warns if remote already contains files (use `--force` to overwrite)
- Performs initial push of vault to remote
- Configuration is saved automatically

#### See Also

- [Cloud Sync Guide](../02-guides/sync-guide) - Comprehensive sync setup and usage
- [Health Checks](../05-operations/health-checks) - Verify sync status with doctor command

---

### doctor - System Health Check

Run diagnostic checks on your pass-cli installation.

#### Synopsis

```bash
pass-cli doctor [flags]
```

#### Description

Performs comprehensive health checks on your vault, configuration, keychain integration, and backups. Useful for troubleshooting issues or verifying system state.

**Checks Performed:**
1. **Version Check**: Compares installed version against latest release
2. **Vault Check**: Verifies vault file exists and has correct permissions
3. **Config Check**: Validates configuration syntax and settings
4. **Keychain Check**: Tests OS keychain integration status
5. **Backup Check**: Verifies backup files exist and are accessible
6. **Sync Check** (if enabled): Verifies rclone is installed, remote is configured, and connectivity works

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--json` | bool | Output results as JSON |
| `--quiet` | bool | Only show warnings and errors |
| `--verbose`, `-v` | bool | Verbose output with detailed check execution |

#### Examples

```bash
# Run all health checks
pass-cli doctor

# Output as JSON (for scripts)
pass-cli doctor --json

# Show only problems
pass-cli doctor --quiet

# Verbose mode (detailed check execution)
pass-cli doctor --verbose
```

#### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All checks passed (HEALTHY) |
| 1 | Warnings detected (review recommended) |
| 2 | Errors detected (action required) |

#### See Also

- [Health Checks Guide](../05-operations/health-checks) - Detailed documentation and troubleshooting

---

### tui - Interactive Terminal Interface

Launch the interactive terminal user interface.

#### Synopsis

```bash
pass-cli tui
pass-cli        # Also launches TUI when no command specified
```

#### Description

Opens an interactive terminal interface for browsing and managing credentials. The TUI provides keyboard-driven navigation, search, and credential operations without memorizing CLI commands.

**Features:**
- Sidebar navigation with credential tree
- Detail panel with credential information
- Search and filter credentials
- Add, edit, and delete credentials
- Copy passwords to clipboard
- Usage statistics display

#### Flags

None.

#### Examples

```bash
# Launch TUI explicitly
pass-cli tui

# Launch TUI (default when no command given)
pass-cli
```

#### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Tab` | Switch between panels |
| `Enter` | Select/expand item |
| `Esc` | Cancel/go back |
| `/` | Search |
| `a` | Add credential |
| `e` | Edit credential |
| `d` | Delete credential |
| `c` | Copy password |
| `t` | Copy TOTP code |
| `q` | Quit |

#### See Also

- [TUI Guide](../02-guides/tui-guide) - Complete TUI documentation and customization

---

### Troubleshooting

#### Why Does Doctor Report Orphaned Keychain Entries?

**Symptom**: Doctor reports "⚠ Keychain: Orphaned entry detected"

**Causes**:
- Vault file was deleted/moved but keychain entry remains
- Vault path changed but old keychain entry wasn't cleaned up
- Multiple vaults were created and old entries weren't removed

**Solutions**:

**macOS**:
```bash
open -a "Keychain Access"
# Search for "pass-cli" and delete old entries
```

**Windows**:
```powershell
control /name Microsoft.CredentialManager
# Navigate to "Windows Credentials" and remove old "pass-cli" entries
```

**Linux**:
```bash
secret-tool search service pass-cli
secret-tool clear service pass-cli vault /old/path/vault.enc
```

#### What If First-Run Detection Doesn't Trigger?

**Expected Behavior**: When running vault-requiring commands (`add`, `get`, `list`, `update`, `delete`) for the first time without an existing vault, pass-cli offers guided initialization.

**Scenarios where first-run detection is skipped**:

1. **Vault already exists**:
   ```bash
   # Check if vault exists
   ls ~/.pass-cli/vault.enc
   ```
   **Solution**: First-run detection is not needed - your vault is already set up.

2. **Custom vault configured**:
   ```bash
   # If vault_path is configured in config file
   # First-run detection uses that location
   ```
   **Solution**: Configuration is respected - first-run detection will create vault at configured location

3. **Non-TTY environment** (scripts, pipes, CI/CD):
   ```bash
   # This environment doesn't support interactive prompts
   echo "list" | pass-cli list
   ```
   **Solution**: Initialize vault manually in interactive session first, or use `pass-cli init` explicitly:
   ```bash
   # In CI/CD or scripts (configure vault_path in config file first)
   pass-cli init < password-input.txt
   ```

4. **Command doesn't require vault**:
   ```bash
   # These commands don't trigger first-run detection
   pass-cli version
   pass-cli doctor
   pass-cli help
   ```
   **Solution**: Run a vault-requiring command: `pass-cli list` or `pass-cli init`

**Manual initialization**: If first-run detection doesn't trigger and you need to create a vault:
```bash
pass-cli init
```

This provides the same guided setup as automatic first-run detection.

**Troubleshooting**: If first-run detection should trigger but doesn't:
```bash
# Verify no vault exists
ls ~/.pass-cli/vault.enc

# Check if running in TTY
tty  # Should show /dev/pts/X or similar, not "not a tty"

# Try explicit init
pass-cli init
```

See [Quick Start Guide](../01-getting-started/quick-start) for complete first-run documentation.

## Getting Help

- Run any command with `--help` flag
- See [pass-cli Documentation](https://reyamira.github.io/pass-cli/) for overview
- Check [Troubleshooting](../04-troubleshooting/_index) for common issues
- Visit [GitHub Issues](https://github.com/reyamira/pass-cli/issues)

