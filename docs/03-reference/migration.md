---
title: "Migration"
weight: 5
toc: true
---
Guide for upgrading Pass-CLI vaults and adapting to security hardening changes introduced in v0.3.0.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)


## Overview

The v0.3.0 security hardening release introduces several important changes:

1. **Vault Location Configuration**: `--vault` flag removed, use config file instead
2. **Increased PBKDF2 Iterations**: 100,000 → 600,000 (6x stronger)
3. **Password Policy Enforcement**: New complexity requirements for all passwords
4. **Audit Logging**: Optional tamper-evident logging with HMAC signatures
5. **Atomic Vault Operations**: Improved rollback safety

**Good News**: Existing vaults continue to work without migration. You can upgrade at your own pace.

## What Changed

### 1. Vault Location Configuration (Breaking Change)

**The `--vault` flag and `PASS_CLI_VAULT` environment variable have been removed.**

**Before** (Old way - no longer works):
```bash
pass-cli --vault /custom/path/vault.enc init
pass-cli --vault /custom/path/vault.enc add github
export PASS_CLI_VAULT=/custom/path/vault.enc
pass-cli get github
```

**After** (New way - use config file):
```bash
# Set custom vault location in config file
echo "vault_path: /custom/path/vault.enc" > ~/.pass-cli/config.yml

# Now use commands without --vault flag
pass-cli init
pass-cli add github
pass-cli get github
```

**Why the change?**
- **Simplicity**: One consistent way to configure vault location
- **Less error-prone**: No conflict between flag, env var, and config
- **Better UX**: Users don't need to remember to add `--vault` to every command

**Migration Steps:**
1. If you use a **default vault location** (`~/.pass-cli/vault.enc`): Nothing to do! Everything works as-is.
2. If you use a **custom vault location**:
   - Create or edit `~/.pass-cli/config.yml`
   - Add: `vault_path: /your/custom/path/vault.enc`
   - Remove `--vault` flag from scripts and commands
   - Remove `PASS_CLI_VAULT` from your environment

**Path Expansion Support:**
- Environment variables: `vault_path: $HOME/.pass-cli/vault.enc`
- Tilde expansion: `vault_path: ~/my-vault.enc`
- Relative paths: `vault_path: vault.enc` (resolved relative to home directory)
- Absolute paths: `vault_path: /custom/absolute/path/vault.enc`

### 2. PBKDF2 Iterations (Crypto Hardening)

| Version | Iterations | Unlock Time (Modern CPU) | Security Benefit |
|---------|------------|--------------------------|------------------|
| **Old** | 100,000 | ~15-20ms | Baseline |
| **New** | 600,000 | ~50-100ms | 6x brute-force resistance |

**Impact**: Vault unlock is slightly slower (~30-80ms slower) but significantly more secure.

**Security Rationale**: The increase from 100,000 to 600,000 iterations aligns with current industry standards:
- **OWASP**: Recommends 600,000+ iterations for PBKDF2-SHA256 (2023 guidance)
- **NIST SP 800-132**: Recommends iteration counts that result in ≥100ms processing time
- **Brute-Force Resistance**: 6x computational cost for attackers attempting password cracking

### 3. Vault Format Migration (V1 → V2)

**What's New**: V2 format fixes a critical bug in recovery phrase handling. V1 vaults had non-functional recovery phrases - they couldn't actually unlock the vault. V2 introduces proper key wrapping that makes recovery phrases fully functional.

**Key Differences**:

| Aspect | V1 Format | V2 Format |
|--------|-----------|-----------|
| **Recovery Phrase** | Non-functional (cannot unlock) | Fully functional recovery |
| **Key Derivation** | Single password-based KEK | Dual KEKs (password + recovery) |
| **DEK Wrapping** | Direct encryption | Wrapped with both KEKs |
| **Atomic Safety** | Basic file operations | Atomic migrations with verification |
| **25th Word Support** | Not supported | Optional passphrase protection |

**Recovery Phrase Example** (what you'll get after migration):
```text
dragon elegant ancient shadow forest machine quantum triumph vendor
success chapter biology network cousin shadow eternal puzzle symbol
```

**Should I Migrate?**

- **YES, if**: You want functional password recovery, or plan to use `pass-cli change-password --recover` if you forget your master password
- **NO, if**: You're satisfied with your current vault and never plan to recover via recovery phrase
- **REQUIRED, if**: You generated a recovery phrase - V1's recovery phrases are non-functional, so V2 migration creates a working one

### 4. Password Policy Enforcement

**New Requirements** (enforced for all passwords):
- Minimum 12 characters (was 8)
- At least one uppercase letter
- At least one lowercase letter
- At least one digit
- At least one special symbol (!@#$%^&*()-_=+[]{}|;:,.<>?)

**Impact**: Weak passwords will be rejected when creating/updating credentials.

### 5. Audit Logging (Optional Feature)

**New Feature**: Tamper-evident audit trail for vault operations.

- Enabled by default (use `--no-audit` to disable)
- HMAC-SHA256 signatures for tamper detection
- Keys stored in OS keychain
- Auto-rotation at 10MB with 7-day retention

**Impact**: Audit logging is now enabled by default for new vaults.

## V1 → V2 Migration Guide

### Checking Your Vault Version

Before migrating, determine which format your vault uses:

```bash
# Check vault version and migration status
pass-cli doctor
```

**Output for V1 vault:**
```text
Vault Information:
  Location: /home/user/.pass-cli/vault.enc
  Exists: Yes
  Format: V1 (legacy - recovery phrases non-functional)
  Migration Status: Required for working recovery phrases
```

**Output for V2 vault:**
```text
Vault Information:
  Location: /home/user/.pass-cli/vault.enc
  Exists: Yes
  Format: V2 (recovery phrases functional)
  Migration Status: Up to date
```

### Migration Process

The migration process is atomic and safe - your vault is preserved even if the process is interrupted.

#### Prerequisites

- Access to your current master password
- ~1 MB free disk space (for temporary files during atomic operation)
- No concurrent vault access (other processes shouldn't be accessing the vault)
- Optional: Backup of your vault (highly recommended for peace of mind)

#### Step 1: Create a Backup (Recommended)

While the migration is atomic and safe, backups are always a good practice:

```bash
# Create a backup before migration
cp ~/.pass-cli/vault.enc ~/backups/vault-backup-$(date +%Y%m%d-%H%M%S).enc

# Or backup to external drive
cp ~/.pass-cli/vault.enc /mnt/usb-drive/vault-backup-$(date +%Y%m%d).enc
```

#### Step 2: Run Migration Command

```bash
pass-cli vault migrate
```

This will:
1. Check if migration is needed (exits if already V2)
2. Display migration information
3. Prompt you to confirm
4. Ask for your current master password to unlock the vault
5. Optionally prompt for a 25th word passphrase (advanced security)
6. Generate a new 24-word recovery phrase
7. Perform atomic migration
8. Display your new recovery phrase

#### Step 3: Save Your Recovery Phrase

**CRITICAL**: Your recovery phrase is displayed once. If you lose it, password recovery becomes impossible.

The command will show your recovery phrase in this format:
```text
YOUR RECOVERY PHRASE (24 words):

1.  dragon           7.  ancient          13. forest           19. triumph
2.  elegant          8.  shadow           14. machine          20. vendor
3.  ancient          9.  shadow           15. quantum          21. success
4.  shadow           10. forest           16. triumph          22. chapter
5.  forest           11. machine          17. vendor           23. biology
6.  machine          12. quantum          18. success          24. symbol
```

**Recommended Storage**:
- Write down on paper (offline, physical backup)
- Store in a safe or safety deposit box
- Consider a metal seed phrase card (fire/water resistant)
- DO NOT store in:
  - Cloud services (Google Drive, Dropbox, iCloud)
  - Email accounts
  - Unencrypted password managers
  - Screenshots or photos

#### Step 4: Verify Your Backup

The command will optionally prompt you to verify your recovery phrase by asking for 3 random words:

```text
Verification (attempt 1/3):
  Enter word #4: triumph
  Enter word #7: forest
  Enter word #12: quantum
✓ Backup verified successfully!
```

This ensures you've written down all 24 words correctly. You get up to 3 attempts.

#### Step 5: Verify Migration Success

```bash
# Check that migration completed
pass-cli doctor

# Output should show:
#   Format: V2 (recovery phrases functional)

# Test accessing your credentials
pass-cli list

# Test accessing a specific credential
pass-cli get github
```

### Post-Migration Actions

#### Critical: Store Recovery Phrase

Your new recovery phrase is now active. Store it securely:
- Physically write down all 24 words in order
- Store in secure location (safe, safety deposit box, etc.)
- Keep separate from your master password
- Do NOT take photos or screenshots

#### Optional: With Passphrase Protection

If you added a 25th word (passphrase) during migration:
- Store the 25th word SEPARATELY from the 24-word phrase
- You'll need both the phrase AND the passphrase to recover
- This adds an extra security layer if the phrase is compromised

#### Test Recovery (Optional but Recommended)

If you want to verify recovery works (without actually changing your password):

```bash
# This will guide you through recovery process without making changes
pass-cli change-password --recover
```

You'll be prompted to:
1. Enter your 24-word recovery phrase
2. Enter the optional 25th word (if you set one)
3. Choose a new master password

**Note**: You can cancel after verification without changing your password.

### Migration Safety & Atomicity

The migration process is atomic - all or nothing:

**During Migration**:
- Temporary file is created: `vault.enc.tmp.YYYYMMDD-HHMMSS.RANDOM`
- Temporary file is verified (decrypted in-memory to ensure correctness)
- Automatic rename operation (all-or-nothing)
- Backup created: `vault.enc.backup`

**If Migration Fails**:
- Original vault file untouched
- Temporary files cleaned up
- You can retry the migration

**If Power Loss During Migration**:
- Automatic cleanup occurs on next vault access
- Original vault remains intact

### Migration Troubleshooting

#### "Your vault is already using the v2 format"

**Meaning**: Migration is not needed - your vault is already V2.

**Solution**: Your recovery phrase should already be functional. No action needed.

#### "migration failed: failed to unlock vault: incorrect password"

**Meaning**: The password you entered doesn't match the vault's master password.

**Solution**:
```bash
# Try migration again with correct password
pass-cli vault migrate
```

#### "failed to generate entropy" or "failed to generate mnemonic"

**Meaning**: System entropy source unavailable (rare).

**Solution**:
- Retry the migration command
- Ensure system has enough disk space
- Check system logs for entropy warnings
- Contact support if issue persists

#### "migration failed: disk full"

**Meaning**: Not enough free disk space for atomic operation.

**Solution**:
```bash
# Free up disk space (need ~1 MB minimum)
df -h  # Check available space
rm -rf /tmp/*  # Clean temp files (use caution)

# Retry migration
pass-cli vault migrate
```

#### "migration failed: permission denied"

**Meaning**: Insufficient file permissions for vault directory.

**Solution**:
```bash
# Check permissions
ls -la ~/.pass-cli/vault.enc

# Fix if needed (make file readable/writable by user)
chmod 600 ~/.pass-cli/vault.enc
```

#### "Cannot verify my recovery phrase"

**Meaning**: Words don't match the recovery phrase shown.

**Solution**:
- Carefully rewrite all 24 words from the migration output
- Ensure each word is spelled correctly (case-insensitive)
- Verify you have all 24 words in correct order
- If still failing, check your written backup for errors

#### "Lost my recovery phrase"

**Meaning**: You didn't write down the recovery phrase and migration is complete.

**Unfortunate Reality**: Recovery phrase cannot be regenerated. The only option is:

**Solution**:
```bash
# If you still know your master password:
# 1. You can still unlock your vault normally
# 2. Create a new vault and migrate credentials manually
# 3. Re-run migration to get a new recovery phrase

# Create backup of current vault
cp ~/.pass-cli/vault.enc ~/.pass-cli/vault.enc.backup

# Remove current vault to start fresh
pass-cli vault remove

# Initialize new vault with recovery phrase
pass-cli init
# This will generate a NEW recovery phrase - SAVE IT!

# Manually migrate credentials from backup vault
# (requires knowing master password for backup)
```

## Migration Scenarios

### Scenario 1: No Migration (Recommended for Most Users)

**Who**: Users satisfied with current vault security.

**Action**: None required. Your vault continues to work with 100k iterations.

**Pros**:
- Zero downtime
- No password changes required
- Vault remains compatible with older Pass-CLI versions

**Cons**:
- Lower brute-force resistance (still secure, but not optimal)

### Scenario 2: Migrate to 600k Iterations

**Who**: Users wanting maximum security.

**Action**: Re-initialize vault with new master password or use migration command (future feature).

**Pros**:
- 6x stronger brute-force resistance
- Future-proof security posture

**Cons**:
- Slightly slower vault unlock (~30-80ms)
- Requires password policy compliance

### Scenario 3: Enable Audit Logging

**Who**: Users in regulated industries or requiring compliance audit trails.

**Action**: Enable audit logging on existing or new vault.

**Pros**:
- Tamper-evident audit trail
- Compliance-ready logging
- Forensic investigation support

**Cons**:
- Additional disk space (~10MB per rotation)
- Requires OS keychain for HMAC keys

## Step-by-Step Migration

### Option A: Fresh Vault (Recommended)

**Best for**: Small vaults (< 50 credentials) or users wanting a clean start.

**Steps**:

1. **Backup current vault**:
   ```bash
   # Backup your vault
   cp ~/.pass-cli/vault.enc ~/backup/vault-old-$(date +%Y%m%d).enc

   # Export credentials (optional)
   pass-cli list --format json > ~/backup/credentials-$(date +%Y%m%d).json
   ```

2. **Initialize new vault**:
   ```bash
   # Create new vault (audit logging enabled by default)
   pass-cli init

   # Or without audit logging (not recommended)
   pass-cli init --no-audit
   ```

3. **Re-add credentials**:
   ```bash
   # Interactive mode (recommended for password policy compliance)
   pass-cli add service1
   pass-cli add service2

   # Or generate password separately, then add credential
   pass-cli generate  # Copy generated password
   pass-cli add service1 --username user@example.com  # Paste when prompted
   ```

4. **Verify migration**:
   ```bash
   # List all credentials
   pass-cli list

   # Test accessing a credential
   pass-cli get service1
   ```

5. **Delete old vault** (after verification):
   ```bash
   rm ~/backup/vault-old-*.enc
   ```

**Time Required**: ~5-10 minutes for 20 credentials.

### Option B: In-Place Migration (V1 → V2)

For users with existing V1 vaults who want automated in-place migration:

```bash
# Migrate V1 vault to V2 format (atomic, safe)
pass-cli vault migrate
```

See the [V1 → V2 Migration Guide](#v1--v2-migration-guide) section above for detailed steps.

**Best for**: Most users - atomic, safe, preserves all credentials.

**Advantages**:
- Automatic process (no manual credential re-entry)
- Atomic operation (all-or-nothing safety)
- Generates new functional recovery phrase
- Backup created automatically

### Option C: Hybrid Approach (Keep Old Vault)

**Best for**: Users wanting to test 600k iterations before full migration.

**Steps**:

1. **Create new vault in separate location**:
   ```bash
   # Backup current config
   cp ~/.pass-cli/config.yml ~/.pass-cli/config.yml.backup

   # Point config to new vault location
   echo "vault_path: ~/.pass-cli/vault-new.enc" > ~/.pass-cli/config.yml

   # Initialize new vault (audit logging enabled by default)
   pass-cli init
   ```

2. **Add new credentials to new vault**:
   ```bash
   pass-cli add newservice
   ```

3. **Switch back to old vault when needed**:
   ```bash
   # Restore original config to access old vault
   cp ~/.pass-cli/config.yml.backup ~/.pass-cli/config.yml
   pass-cli get oldservice
   ```

4. **Promote new vault when ready**:
   ```bash
   # Point config back to new vault
   echo "vault_path: ~/.pass-cli/vault-new.enc" > ~/.pass-cli/config.yml

   # Or rename new vault to default location
   mv ~/.pass-cli/vault-new.enc ~/.pass-cli/vault.enc
   rm ~/.pass-cli/config.yml  # Use default location
   ```

## Backward Compatibility

### Vault File Format

**100k Iteration Vaults**:
- [OK] Fully supported
- [OK] Auto-detected by iteration count in metadata
- [OK] No performance degradation
- [OK] Can be used alongside 600k vaults

**600k Iteration Vaults**:
- [WARNING] **Not compatible with Pass-CLI versions before v0.3.0**
- [OK] Auto-detected by iteration count in metadata
- [OK] Future-proof format

### Password Policy

**Existing Credentials**:
- [OK] Old passwords (not meeting policy) remain valid
- [WARNING] Policy enforced only when creating/updating credentials
- [OK] No forced password changes

**New/Updated Credentials**:
- [WARNING] Must meet new policy requirements
- [OK] Real-time validation with helpful error messages
- [OK] TUI shows password strength indicator

### Cross-Version Compatibility Matrix

| Vault Type | Pass-CLI < v0.3.0 | Pass-CLI v0.3.0+ |
|------------|----------------|---------------------|
| 100k iterations | [OK] Read/Write | [OK] Read/Write |
| 600k iterations | [ERROR] Incompatible | [OK] Read/Write |
| With audit logging | [ERROR] Incompatible | [OK] Read/Write |

## Troubleshooting

### Problem: "Password Does Not Meet Requirements"

**Symptom**: Error when creating/updating credentials.

**Solution**:
```bash
# Ensure password meets policy:
# - 12+ characters
# - Uppercase + lowercase + digit + symbol

# Good examples:
MySecureP@ss2025!
Correct-Horse-Battery-29!
Admin#2025$Password

# Or generate a policy-compliant password
pass-cli generate  # Automatically meets policy requirements
```

### Problem: Vault Unlock Is Slower After Upgrade

**Symptom**: Vault unlock takes 50-100ms instead of 15-20ms.

**Explanation**: This is expected behavior with 600k iterations. The slowdown is intentional for security.

**Solution**: No action needed. Performance is within normal range.

**Benchmark**:
- Modern CPU (2023+): 50-100ms
- Mid-range CPU (2018-2022): 200-500ms
- Older CPU (2015-2017): 500-1000ms

### Problem: Cannot Downgrade to Older Pass-CLI Version

**Symptom**: "Invalid vault format" error when using old Pass-CLI with new vault.

**Solution**:
1. Keep backup of old vault before migration
2. Or create new vault with old Pass-CLI version
3. Or upgrade to latest Pass-CLI version

### Problem: Audit Log Verification Fails

**Symptom**: `pass-cli verify-audit` reports HMAC verification failures.

**Causes**:
- Audit log file manually edited (tampering detected)
- Audit key deleted from OS keychain
- Audit log file corrupted

**Solution**:
```bash
# Check if audit key exists in keychain
# If missing, audit logging needs to be re-enabled

# Backup corrupted log
mv ~/.pass-cli/audit.log ~/.pass-cli/audit.log.corrupted

# Start fresh audit log (audit enabled by default)
pass-cli init
```

### Problem: "Vault File Corrupted" After Migration

**Symptom**: Cannot unlock vault after re-initialization.

**Solution**:
```bash
# Restore from backup
cp ~/backup/vault-old-*.enc ~/.pass-cli/vault.enc

# Verify restoration
pass-cli list

# Retry migration more carefully
```

## FAQ

### Q: Do I Have to Migrate?

**A**: No. Existing vaults with 100k iterations continue to work indefinitely. Migration is optional for users wanting stronger security.

### Q: Will Migration Delete My Credentials?

**A**: No. Migration is non-destructive. Always creates backup before changes. Credentials are preserved.

### Q: How Long Does Migration Take?

**A**: Depends on vault size:
- Small vault (< 20 credentials): 5-10 minutes
- Medium vault (20-100 credentials): 15-30 minutes
- Large vault (100+ credentials): 30-60 minutes

Time includes manual re-entry of credentials. Future in-place migration will be automatic (seconds).

### Q: Can I Migrate Back to 100k Iterations?

**A**: Technically yes (create new vault), but not recommended. Forward migration only makes sense for security.

### Q: Does Audit Logging Slow Down Vault Operations?

**A**: Minimal impact (~1-2ms per operation). Audit logging uses asynchronous writes and graceful degradation.

### Q: What If I Forget My Master Password After Migration?

**A**: If you enabled BIP39 recovery during `pass-cli init`, you can recover using `pass-cli change-password --recover` and your 24-word recovery phrase. If you used `--no-recovery` or are on an older vault without recovery, the vault is unrecoverable. Keep master password and recovery phrase backups secure.

### Q: Are Audit Logs Encrypted?

**A**: Audit logs are **not encrypted** (they contain service names, not passwords). Logs are **tamper-evident** via HMAC signatures. If encryption is required, use full-disk encryption.

### Q: Can I Disable Audit Logging After Enabling?

**A**: Yes, but audit logs remain on disk. You can manually delete old logs. Future releases may add a command to disable audit logging cleanly.

### Q: Will Old Pass-CLI Versions Work With Migrated Vaults?

**A**: No. 600k iteration vaults require Pass-CLI v0.3.0 or later. Keep old vault backup if you need compatibility with older versions.

### Q: Is There a Tool to Convert Vault Format?

**A**: Not yet. Currently requires manual re-initialization. In-place migration planned for future release.

## Best Practices

### Before Migration

1. [OK] Backup vault file: `cp ~/.pass-cli/vault.enc ~/backup/`
2. [OK] Export credentials: `pass-cli list --format json > backup.json`
3. [OK] Test new Pass-CLI version with test vault first
4. [OK] Read this migration guide completely

### During Migration

1. [OK] Use `pass-cli generate` command for policy-compliant passwords
2. [OK] Verify each credential after adding
3. [OK] Test vault unlock multiple times
4. [OK] Enable audit logging for compliance needs

### After Migration

1. [OK] Verify all credentials accessible
2. [OK] Test credential retrieval in scripts
3. [OK] Update documentation/runbooks with new requirements
4. [OK] Delete old vault backup after 30-day grace period
5. [OK] Run `pass-cli verify-audit` monthly (if audit logging enabled)

## Support

- **Documentation**: [Security Architecture](security-architecture), [Command Reference](command-reference)
- **Issues**: [GitHub Issues](https://github.com/reyamira/pass-cli/issues)

