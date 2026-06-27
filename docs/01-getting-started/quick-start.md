---
title: "Quick Start Guide"
weight: 3
toc: true
---

This 5-minute guide will walk you through initializing your vault and storing your first credential.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)

## Installation

See [Quick Install](quick-install) for platform-specific installation instructions (Homebrew, Scoop) or [Manual Installation](manual-install) for binary download.

After installation, verify pass-cli is available:

```bash
pass-cli version
```

## Initialize Your Vault

To get started, run the init command:

```bash
pass-cli init
```

This walks you through choosing whether to create a new vault or connect to an existing synced vault, then sets up your secure vault with a master password and recovery phrase.

### First Choice: New or Existing Vault

When you run `pass-cli init`, you're first asked if this is a new installation or if you're connecting to an existing vault:

```bash
$ pass-cli init

Is this a new installation or are you connecting to an existing vault?

  [1] Create new vault (first time setup)
  [2] Connect to existing synced vault (requires rclone)

Enter choice (1/2) [1]: 1
```

Select option 1 for a new vault (default), or option 2 if you already have pass-cli set up on another device with cloud sync enabled.

### Example Walkthrough (New Vault)

```bash
$ pass-cli init
[... choose option 1 ...]

🔐 Initializing new password vault
📁 Vault location: /home/user/.pass-cli/vault.enc
```

**Step 1: Master Password**

```bash
Enter master password: ••••••••••••••
Confirm master password: ••••••••••••••
✓ Password strength: Strong
```

**Step 2: Configuration Options**

```bash
Enable keychain storage for master password? (y/n) [y]: y

Audit logging tracks all vault operations (no credentials logged)
Enable audit logging? (y/n) [y]: y

Advanced: Add passphrase protection (25th word)?
   • Adds an extra layer of security to your recovery phrase
   • You will need BOTH the 24 words AND the passphrase to recover
Add passphrase? (y/n) [n]: n
```

**Step 3: Recovery Phrase**

```bash
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Recovery Phrase Setup
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Write down these 24 words in order:

  1. abandon       7. brother     13. country     19. fragile
  2. ability       8. brown       14. couple      20. frame
  3. able          9. brush       15. course      21. frequent
  4. about        10. bubble      16. cousin      22. fresh
  5. above        11. buddy       17. cover       23. friend
  6. absent       12. budget      18. crack       24. fringe

⚠  WARNINGS:
   • Anyone with this phrase can access your vault
   • Store offline (write on paper, use a safe)
   • Recovery requires 6 random words from this list
```

**Step 4: Backup Verification**

```bash
Verify your backup? (Y/n): y

Verification (attempt 1/3):
Enter word #4: about
Enter word #12: budget
Enter word #19: fragile
✓ Backup verified successfully!
```

**Setup Complete**

```bash
✅ Vault initialized successfully!
📍 Location: /home/user/.pass-cli/vault.enc
🔑 Master password stored in system keychain
📊 Audit logging enabled
🔑 You can recover your vault using the 24-word recovery phrase

💡 Next steps:
   • Add a credential: pass-cli add <service>
   • View help: pass-cli --help
```

> **Important**: The 24-word recovery phrase is your backup if you forget your master password. Write it down and store it securely offline. Anyone with this phrase can access your vault.

### Auto-Detection

You can also trigger initialization by running any vault command (`add`, `get`, `list`, etc.) without an existing vault. pass-cli will detect this and offer to create one.

> **Note**: Auto-detection requires an interactive terminal. In scripts or CI/CD, use `pass-cli init` explicitly.

### Advanced Options

#### Custom Vault Location

To use a custom vault location, configure it in your config file before initialization:

```bash
# Edit config file
echo "vault_path: /custom/path/vault.enc" > ~/.pass-cli/config.yml

# Then initialize
pass-cli init
```

Future commands will automatically use the configured vault location:

```bash
pass-cli add github
pass-cli get github
```

#### Skip Keychain Integration

During the interactive initialization, answer "n" when prompted about keychain storage:

```bash
pass-cli init
# When asked "Enable keychain storage for master password? (y/n) [y]:", enter "n"
```

This creates a vault without storing the master password in OS keychain. You'll need to enter your password for each operation.

#### Disable Audit Logging

Audit logging is enabled by default (recommended). To disable it during initialization:

```bash
pass-cli init --no-audit
```

This creates a vault without tamper-evident HMAC-signed audit logging. Only disable if you have specific storage constraints.

#### Skip Recovery Phrase (Not Recommended)

By default, pass-cli generates a 24-word BIP39 recovery phrase that can be used to recover your vault if you forget your master password. To create a password-only vault without recovery:

```bash
pass-cli init --no-recovery
```

> **Warning**: Without the recovery phrase, if you forget your master password, your vault cannot be recovered. Only use this option if you have another backup strategy.

#### Add Passphrase Protection (25th Word)

For additional security, you can add a passphrase (sometimes called the "25th word") to your recovery phrase. During initialization, answer "y" when prompted:

```bash
pass-cli init
# When asked "Advanced: Add passphrase protection (25th word)?", enter "y"
```

With passphrase protection:
- You need BOTH the 24 words AND the passphrase to recover
- Store the passphrase separately from your recovery phrase
- If you lose either, recovery is impossible

#### Skip Cloud Sync Prompts

If you don't want to set up cloud sync during initialization, use the `--no-sync` flag:

```bash
pass-cli init --no-sync
```

This skips the cloud sync setup prompts. You can enable sync later with:

```bash
pass-cli sync enable
```

See [Cloud Sync Guide](../02-guides/sync-guide) for details on setting up sync on existing vaults.

## Your First Credential

After initialization, add your first credential:

```bash
$ pass-cli add github

Enter username: your-github-username
Enter password: ••••••••••••
Confirm password: ••••••••••••

[PASS] Credential 'github' added successfully
```
