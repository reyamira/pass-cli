---
title: "Cloud Sync"
weight: 5
toc: true
---

Synchronize your vault across multiple devices using rclone. This enables seamless access to your credentials from different computers (e.g., dual-boot setups, work/home machines).

## Overview

Pass-CLI integrates with [rclone](https://rclone.org/) to sync your vault directory to cloud storage providers. The sync feature:

- **Pulls** the latest vault from the cloud before any operation (once per session)
- **Pushes** changes to the cloud after write operations (add, update, delete) with "Syncing... done" feedback
- **Skips push on reads** - `get` and `list` commands never trigger a push, keeping reads fast
- **Works offline** - operations continue if sync fails (with warning)
- **Supports 70+ cloud providers** via rclone (Google Drive, Dropbox, OneDrive, S3, etc.)

### How Sync Works

```text
┌─────────────┐     pull (on first use)      ┌─────────────┐
│   Device A  │ ◄──────────────────────────► │   Cloud     │
│  (Windows)  │                              │  Storage    │
└─────────────┘     push (after writes)      └─────────────┘
                                                    ▲
                                                    │
┌─────────────┐     pull (on first use)             │
│   Device B  │ ◄───────────────────────────────────┘
│   (Linux)   │     push (after writes)
└─────────────┘
```

**Smart sync**: Pull happens before unlock to ensure you have the latest vault. Push only happens after write operations (`add`, `update`, `delete`) — read-only commands like `get` and `list` never trigger a push. When a push occurs, you'll see `Syncing... done` feedback on stderr. In the TUI, push only happens if you made changes during your session.

## Prerequisites

### 1. Install rclone

**Windows (Scoop)**:
```bash
scoop install rclone
```

**Windows (Chocolatey)**:
```bash
choco install rclone
```

**macOS**:
```bash
brew install rclone
```

**Linux (Debian/Ubuntu)**:
```bash
sudo apt install rclone
```

**Linux (Arch)**:
```bash
sudo pacman -S rclone
```

Verify installation:
```bash
rclone version
```

### 2. Configure a Remote

Configure rclone with your cloud provider. Example for Google Drive:

```bash
rclone config
```

Follow the interactive prompts:
1. Choose `n` for new remote
2. Name it (e.g., `gdrive`)
3. Select your provider (e.g., `drive` for Google Drive)
4. Follow provider-specific OAuth flow
5. For Google Drive, select scope `3` (drive.file) for minimal permissions

Test the remote:
```bash
# List remote contents
rclone ls gdrive:

# Create a test directory
rclone mkdir gdrive:.pass-cli
```

## Enabling Sync

There are two ways to enable sync on your vault:

### Option 1: During Vault Initialization (Recommended for New Vaults)

When running `pass-cli init` to create a new vault, you'll be offered the option to enable cloud sync:

```bash
pass-cli init

# ... after vault creation ...
Enable cloud sync? (requires rclone) (y/n) [n]: y

Enter your rclone remote path.
Examples:
  gdrive:.pass-cli         (Google Drive)
  dropbox:Apps/pass-cli    (Dropbox)
  onedrive:.pass-cli       (OneDrive)

Remote path: gdrive:.pass-cli
```

The sync configuration is automatically saved to your config file.

### Option 2: On an Existing Vault

To enable sync on a vault you've already created, use the `sync enable` command:

```bash
pass-cli sync enable
```

This command:
1. Checks that your vault exists
2. Verifies rclone is installed and configured
3. Prompts you to enter your rclone remote path
4. Tests connectivity to the remote
5. Warns if the remote already contains files (use `--force` to overwrite)
6. Performs an initial push of your vault to the remote
7. Saves the configuration automatically

**Examples:**

```bash
# Enable sync interactively
pass-cli sync enable

# Force overwrite if remote already has files
pass-cli sync enable --force
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--force` | Overwrite remote if it already contains vault files |

### Manual Configuration

You can also enable sync by editing your config file directly (`~/.pass-cli/config.yml`):

```yaml
sync:
  enabled: true
  remote: "gdrive:.pass-cli"  # Format: <remote-name>:<path>
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable/disable sync |
| `remote` | string | `""` | rclone remote and path (e.g., `gdrive:.pass-cli`) |

### Remote Path Format

The `remote` field uses rclone's remote path format:

```yaml
# Google Drive
remote: "gdrive:.pass-cli"

# Dropbox
remote: "dropbox:Apps/pass-cli"

# OneDrive
remote: "onedrive:Documents/pass-cli"

# S3-compatible storage
remote: "s3:my-bucket/pass-cli"

# SFTP server
remote: "myserver:/home/user/.pass-cli"
```

## Usage

Once configured, sync happens automatically:

```bash
# Any command that unlocks the vault - pulls from cloud first
pass-cli list

# Read commands - no push (fast, no network overhead)
pass-cli get github

# Write operations - push to cloud after completion
pass-cli add newservice --username user --password pass
# Output: Syncing... done
```

### Manual Sync

Sync is automatic, but you can manually trigger it by restarting your terminal session or using rclone directly:

```bash
# Manual pull
rclone sync gdrive:.pass-cli ~/.pass-cli

# Manual push
rclone sync ~/.pass-cli gdrive:.pass-cli
```

## Portable Audit Keys

When sync is enabled, pass-cli automatically uses **portable audit key derivation**. This means:

- Audit keys are derived from your master password (not stored in OS keychain)
- `verify-audit` command works on any synced device
- Audit log integrity can be verified cross-platform

The audit salt is stored in your vault's metadata file and syncs with your vault.

### How It Works

```text
Master Password + Audit Salt
         │
         ▼ PBKDF2-SHA256 (100k iterations)
         │
    Audit Key
         │
         ▼
   HMAC Signatures (audit log entries)
```

This replaces the default keychain-based audit key storage when sync is enabled.

## Dual-Boot Setup Example

Perfect for users who dual-boot between Windows and Linux:

**1. Configure rclone on both OSes** (with same remote name):

```bash
# On Windows
rclone config
# Create remote named "gdrive"

# On Linux
rclone config
# Create remote named "gdrive" (same name, same account)
```

**2. Enable sync on both OSes** (`~/.pass-cli/config.yml`):

```yaml
sync:
  enabled: true
  remote: "gdrive:.pass-cli"
```

**3. Initialize vault on one OS**:

```bash
# On Windows
pass-cli init
pass-cli add github --username myuser --password mypass
# Vault syncs to cloud
```

**4. Use on other OS**:

```bash
# On Linux - first use pulls vault from cloud
pass-cli list
# Shows: github
```

## Connecting to an Existing Synced Vault

If you already have pass-cli set up with sync on another device and want to connect from a new machine, use the guided flow during initialization:

> **Important**: When running `pass-cli init`, select "Connect to existing synced vault" instead of "Create new vault" to download your existing vault instead of creating a new one.

### Quick Setup

When running `pass-cli init` on a new machine, you'll be prompted:

```bash
pass-cli init

Is this a new installation or are you connecting to an existing vault?

  [1] Create new vault (first time setup)
  [2] Connect to existing synced vault (requires rclone)

Enter choice (1/2) [1]: 2

🔗 Connect to existing synced vault

Enter your rclone remote path where your vault is stored.
Examples:
  gdrive:.pass-cli         (Google Drive)
  dropbox:Apps/pass-cli    (Dropbox)
  onedrive:.pass-cli       (OneDrive)

Remote path: gdrive:.pass-cli
✓ Vault downloaded
✓ Vault unlocked successfully

✅ Connected to synced vault!
```

### Step-by-Step Manual Setup

**1. Install pass-cli and rclone on the new machine**:

```bash
# macOS
brew tap reyamira/homebrew-tap && brew install pass-cli rclone

# Windows
scoop bucket add reyamira https://github.com/reyamira/scoop-bucket
scoop install pass-cli rclone

# Linux
# Install pass-cli from releases, then:
sudo apt install rclone
```

**2. Configure rclone with the same cloud account**:

```bash
rclone config
# Create remote with the SAME name as your other device (e.g., "gdrive")
# Log in with the SAME cloud account
```

**3. Verify you can see your existing vault**:

```bash
rclone ls gdrive:.pass-cli
# Should show: vault.enc, vault.enc.meta.json, etc.
```

**4. Create the pass-cli config file manually**:

```bash
# Create config directory
mkdir -p ~/.pass-cli

# Create config file
cat > ~/.pass-cli/config.yml << 'EOF'
sync:
  enabled: true
  remote: "gdrive:.pass-cli"
EOF
```

On Windows (PowerShell):
```powershell
mkdir -Force ~\.pass-cli
@"
sync:
  enabled: true
  remote: "gdrive:.pass-cli"
"@ | Out-File -FilePath ~\.pass-cli\config.yml -Encoding UTF8
```

**5. Run any pass-cli command** - it will pull your vault:

```bash
pass-cli list
# Output:
# Syncing vault...
# Enter master password: ****
# github
# aws-prod
# ...
```

Use your **existing master password** from the original device.

### What Happens Behind the Scenes

1. pass-cli sees sync is enabled
2. Calls `rclone sync <remote> <local>` (pull)
3. Your vault downloads from cloud to `~/.pass-cli/`
4. pass-cli prompts for master password
5. Vault unlocks with your existing password
6. You're connected!

### Common Mistakes

| Mistake | Result | Fix |
|---------|--------|-----|
| Running `pass-cli init` first | Creates new vault, may overwrite cloud on push | Delete local vault, follow steps above |
| Different rclone remote name | Sync can't find remote | Use same remote name on all devices |
| Wrong cloud account | Downloads empty or different vault | Reconfigure rclone with correct account |
| Typo in remote path | "Remote not found" error | Check with `rclone listremotes` |

## Conflict Handling

Pass-CLI uses rclone's sync behavior which **overwrites** the destination with the source. This means:

- **Pull**: Cloud overwrites local (ensures you have latest)
- **Push**: Local overwrites cloud (your changes take precedence)

### Change detection

To decide whether the remote actually changed (and avoid needless pulls), pass-cli
compares its own SHA-256 of the vault content rather than trusting the file's
modification time and size. On each push it writes a tiny zero-byte marker named
`vault.enc.<sha256>.synchash` next to `vault.enc` in your remote; the content hash
is read straight from that filename during the pre-unlock listing, so detection
costs no extra network round-trip. **You may see this `*.synchash` object in your
cloud bucket — it is expected and safe to leave alone** (pass-cli replaces it on
every push). Older vaults without a marker fall back to the modtime+size heuristic.

**To avoid conflicts**:
1. Always use the same device for a session
2. Don't run pass-cli simultaneously on multiple devices
3. If unsure, manually check with `rclone ls <remote>` before operations

## Offline Operation

Sync failures don't block operations:

```bash
# If offline or cloud unreachable
pass-cli add service --username user --password pass

# Output:
# Warning: sync push failed: <error details>
# Credential 'service' added successfully
```

Your local vault is updated successfully. Changes sync on next successful push.

### Skipping sync explicitly with `--offline`

When you *know* you're offline — or you just want to avoid the network round-trip
(e.g. in CI or a tight scripted loop) — pass the global `--offline` flag:

```bash
pass-cli --offline get github      # no pre-unlock pull, no post-command push
pass-cli --offline list -q
```

`--offline` makes the command **fully local in both directions**: it skips the
pre-unlock pull *and* the post-command push. Skipping only the pull would be
unsafe — the push has no independent remote-conflict check, so it could blind-
overwrite a newer remote — which is why `--offline` disables both. Any local
changes you make offline simply aren't propagated until the next online run.

## Troubleshooting

### Sync Not Working

1. **Verify rclone is installed**:
   ```bash
   rclone version
   ```

2. **Test remote connectivity**:
   ```bash
   rclone ls gdrive:.pass-cli
   ```

3. **Check configuration**:
   ```bash
   pass-cli config validate
   ```

4. **Verify sync is enabled**:
   ```yaml
   # ~/.pass-cli/config.yml
   sync:
     enabled: true      # Must be true
     remote: "gdrive:.pass-cli"  # Must not be empty
   ```

### "rclone not found" Warning

Install rclone and ensure it's in your PATH:

```bash
# Check if rclone is in PATH
which rclone    # Linux/macOS
where rclone    # Windows
```

### Permission Denied Errors

Ensure your rclone remote has write permissions:

```bash
# Test write access
echo "test" > /tmp/test.txt
rclone copy /tmp/test.txt gdrive:.pass-cli/
rclone ls gdrive:.pass-cli/
```

### Audit Verification Fails After Sync

If `verify-audit` fails on a synced device:

1. Ensure you're using the **same master password** on all devices
2. Check that the vault metadata file (`.meta.json`) synced correctly
3. The audit salt must be present in metadata for portable key derivation

### Slow Sync Operations

For large vaults or slow connections:

```bash
# Check what will be synced without transferring
rclone sync ~/.pass-cli gdrive:.pass-cli --dry-run

# Show transfer progress
rclone sync ~/.pass-cli gdrive:.pass-cli --progress
```

## Security Considerations

### What Gets Synced

The entire vault directory is synced, including:
- `vault.enc` - Encrypted vault (AES-256-GCM)
- `vault.enc.meta.json` - Vault metadata (audit salt, timestamps)
- `audit.log` - Audit log (HMAC-signed entries)
- Backup files (if present)

### What Stays Local

- Master password (never stored)
- Keychain entries (OS-specific)
- Session state (in-memory only)

### Cloud Storage Security

Your vault is already encrypted with AES-256-GCM before sync. The cloud provider only sees encrypted data. However:

- Use a strong master password
- Enable 2FA on your cloud account
- Consider using end-to-end encrypted providers (e.g., rclone crypt)

### Additional Encryption Layer (Optional)

For extra security, use rclone's crypt remote:

```bash
# Create encrypted remote on top of cloud storage
rclone config
# Choose 'crypt' type
# Set remote to "gdrive:.pass-cli"
# This adds another encryption layer
```

## See Also

- [Configuration Reference](../03-reference/configuration) - Full configuration options
- [Security Architecture](../03-reference/security-architecture) - Encryption and audit details
- [Backup & Restore](./backup-restore) - Local backup strategies
- [rclone Documentation](https://rclone.org/docs/) - Official rclone docs
