---
title: "Recovery Phrase"
weight: 3
toc: true
---

Complete guide to using BIP39 recovery phrases to recover vault access if you forget your master password.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)

## Overview

Pass-CLI's BIP39 recovery feature generates a 24-word recovery phrase when you create your vault. If you ever forget your master password, you can reset it using just 6 words from your recovery phrase.

**Key Benefits**:
- [OK] **Industry Standard**: Uses BIP39 (same as hardware wallets)
- [OK] **Secure**: 6 words = 73.8 quintillion combinations
- [OK] **Fast**: Recover in under 30 seconds
- [OK] **Optional**: Can skip with `--no-recovery` flag if you use keychain integration

{{< callout type="warning" >}}
**V1 Vault Users**: If you created your vault before v0.2.0, your recovery phrase will NOT work. V1 vaults have a bug where recovery phrases cannot unlock the vault. You must migrate to V2 format first. See [Migrating to V2 Format](#migrating-to-v2-format) below.
{{< /callout >}}

## Setting Up Recovery

### During Vault Initialization

When you run `pass-cli init`, recovery is **enabled by default**:

```bash
$ pass-cli init
Enter master password: ****
Confirm master password: ****

[PASS] Vault created

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Recovery Phrase Setup
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Write down these 24 words in order:

 1. abandon    7. device    13. hover    19. spatial
 2. ability    8. diagram   14. hurdle   20. sphere
 3. about      9. dial      15. hybrid   21. spike
 4. above     10. diamond   16. icon     22. spin
 5. absent    11. diary     17. idea     23. spirit
 6. absorb    12. diesel    18. identify 24. split

⚠  WARNINGS:
   • Anyone with this phrase can access your vault
   • Store offline (write on paper, use a safe)
   • Recovery requires 6 random words from this list

Advanced: Add passphrase protection? (y/N): n

Verify your backup? (Y/n): y

Enter word #7: device
[PASS] (1/3)

Enter word #18: identify
[PASS] (2/3)

Enter word #22: spin
[PASS] (3/3)

[PASS] Recovery phrase verified
[PASS] Vault initialized successfully
```

### Skipping Recovery Phrase

If you prefer to rely solely on keychain integration or have another backup strategy:

```bash
# Skip recovery phrase generation
pass-cli init --no-recovery
```

**Warning**: If you skip recovery phrase generation, you cannot recover vault access if you forget your master password.

## Migrating to V2 Format

### Why Migration is Necessary

V1 vaults (created before pass-cli v0.2.0) have a critical bug: recovery phrases cannot actually unlock the vault. V2 vaults fix this by implementing proper key wrapping, making recovery phrases fully functional.

{{< callout type="info" >}}
**Check Your Vault Version**: Run `pass-cli doctor` to see if your vault is v1 or v2. If it shows "Vault Format: v1", you need to migrate.
{{< /callout >}}

### Migration Steps

#### Step 1: Run the Migration Command

```bash
pass-cli vault migrate
```

You'll see:
```text
🔄 Vault Migration
📁 Vault location: ~/.pass-cli/vault.enc

Your vault is using the legacy v1 format.
The v1 format has a bug where recovery phrases cannot unlock the vault.

Migration will:
  • Re-encrypt your vault with the new v2 format
  • Generate a NEW 24-word recovery phrase
  • Preserve all your existing credentials

Proceed with migration? (Y/n): y
```

#### Step 2: Unlock Your Vault

You'll be prompted for your current master password:

```bash
Enter master password: ****
[PASS] Vault unlocked
```

#### Step 3: Optional Passphrase Protection

You can add optional passphrase protection to your recovery phrase (the "25th word"):

```bash
Advanced: Add passphrase protection (25th word) to recovery phrase? (y/N): y

⚠️  Passphrase Protection:
   • Adds an extra layer of security to your recovery phrase
   • You will need BOTH the 24 words AND the passphrase to recover
   • Store the passphrase separately from your 24-word phrase
   • If you lose the passphrase, recovery will be impossible

Enter recovery passphrase: ****
Confirm recovery passphrase: ****
```

#### Step 4: Write Down Your New Recovery Phrase

A new 24-word recovery phrase is generated:

```text
🔄 Migrating vault...

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Recovery Phrase Setup
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Write down these 24 words in order:

 1. abandon    7. device    13. hover    19. spatial
 2. ability    8. diagram   14. hurdle   20. sphere
 ... (24 words total)

⚠  WARNINGS:
   • Anyone with this phrase can access your vault
   • Store offline (write on paper, use a safe)
   • Your old recovery phrase (if any) is now invalid
```

{{< callout type="error" >}}
**Important**: Your OLD recovery phrase no longer works. The new recovery phrase shown here is what you must use to recover your vault.
{{< /callout >}}

#### Step 5: Verify Your Backup

You'll be asked to verify your backup by entering a few random words from the new phrase:

```bash
Verify your backup? (Y/n): y

Verification (attempt 1/3):
Enter word #7: device
[PASS] (1/3)

Enter word #18: identify
[PASS] (2/3)

Enter word #22: spin
[PASS] (3/3)

✓ Backup verified successfully!
```

#### Step 6: Done!

```bash
✅ Vault migrated successfully to v2 format!

Your recovery phrase is now fully functional.
You can use 'pass-cli change-password --recover' if you forget your password.
```

### Post-Migration: Securing Your New Recovery Phrase

After migration, your vault uses the new recovery phrase. Store it securely:

- **Write it down** on paper using permanent ink
- **Store offline** in a safe, lockbox, or safety deposit box
- **Keep it separate** from your vault file and computer
- **Don't digitize it** (no photos, cloud storage, email, etc.)
- **Keep it separate** from your optional passphrase (if you created one)

### What to Do Next

**CRITICAL**: Write down your 24-word phrase **on paper** (not digitally). Store it securely:

**Secure Storage** (Recommended):
- [OK] Physical safe or lockbox
- [OK] Safety deposit box at bank
- [OK] Fireproof/waterproof document safe at home
- [OK] Split across multiple secure locations (advanced)

**Insecure Storage** (Avoid):
- [ERROR] Digital notes apps (Apple Notes, Google Keep, etc.)
- [ERROR] Cloud storage (Dropbox, Google Drive, iCloud)
- [ERROR] Email or messaging apps
- [ERROR] Screenshots or photos on phone
- [ERROR] Password managers (defeats the purpose)

**Keep your phrase offline**. If someone gets your phrase, they can access your vault.

## Recovering Your Vault

### When to Use Recovery

Use recovery if:
- [OK] You forgot your master password
- [OK] You have your 24-word recovery phrase
- [OK] Your vault is V2 format (or migrated to V2)

{{< callout type="warning" >}}
**V1 Vaults Cannot Use Recovery**: If you haven't migrated to V2 format yet, recovery will not work. See [Migrating to V2 Format](#migrating-to-v2-format) above.
{{< /callout >}}

**Note**: If keychain is enabled and accessible, you don't need recovery. Your master password is stored securely in your OS keychain.

### Recovery Steps

#### Step 1: Run Recovery Command

```bash
pass-cli change-password --recover
```

This command unlocks your vault using your recovery phrase and sets a new master password.

#### Step 2: Enter Your Recovery Phrase

You'll be asked for 6 random words from your 24-word phrase in random order:

```text
🔐 Vault Recovery
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
You will be asked for 6 words from your 24-word phrase.
Have your recovery phrase ready.

Enter word #18: identify
[PASS] (1/6)

Enter word #3: about
[PASS] (2/6)

Enter word #22: spin
[PASS] (3/6)

Enter word #7: device
[PASS] (4/6)

Enter word #11: diary
[PASS] (5/6)

Enter word #15: hybrid
[PASS] (6/6)

[PASS] Recovery phrase verified
[PASS] Vault unlocked
```

#### Step 3: Set New Master Password

```text
Enter new master password: ****
Confirm new master password: ****

[PASS] Master password changed successfully
Your vault has been re-encrypted with the new password.
```

#### Step 4: Done!

Your vault is now accessible with your new master password. If keychain integration is enabled, the new password is automatically stored in your OS keychain.

### Recovery Tips

- **Use your written phrase**: Look at the paper where you wrote down the 24 words
- **Position numbers matter**: "word #7" refers to the 7th word in your original list
- **Order is randomized**: The system asks for words in random order each time
- **Typos caught immediately**: Invalid words (not in BIP39 wordlist) are rejected instantly
- **6 attempts maximum**: After 6 failed attempts, you must wait before trying again

## Security Best Practices

### Secure Storage

**Physical Security**:
- Write recovery phrase on **archival-quality paper** (acid-free, long-lasting)
- Use **permanent ink** (not pencil, which can smudge)
- Store in **fireproof/waterproof safe** or safety deposit box
- Consider **metal backup plates** for extreme durability

**Redundancy**:
- Keep **multiple copies** in separate secure locations
- **Don't** keep all copies in same building (fire/flood risk)
- **Don't** store with vault or on same device

**Access Control**:
- Only trusted family members should know where phrase is stored
- Consider **sealed envelope** with tamper-evident security
- Update beneficiaries if phrase location changes

### What Never to Do

**Never Store Digitally**:
- [ERROR] Photos or screenshots
- [ERROR] Cloud storage services
- [ERROR] Email or messaging apps
- [ERROR] Password managers
- [ERROR] Digital note-taking apps

**Never Share**:
- [ERROR] Don't tell anyone your recovery phrase
- [ERROR] Pass-CLI will never ask for your full phrase
- [ERROR] No support person needs your recovery phrase
- [ERROR] Recovery phrase = full vault access

**Never Memorize Only**:
- [ERROR] Human memory is fallible
- [ERROR] Always have physical backup
- [ERROR] Don't rely on memory alone

### Testing Your Backup

After writing down your recovery phrase:

1. **Verify you wrote it correctly** during initialization (3-word challenge)
2. **Store phrase securely** before testing recovery
3. **Optional**: Test recovery in safe environment:
   ```bash
   # Backup existing config (if any)
   cp ~/.pass-cli/config.yml ~/.pass-cli/config.yml.backup 2>/dev/null

   # Point to temporary test vault
   echo "vault_path: /tmp/test-vault.enc" > ~/.pass-cli/config.yml

   # Create test vault and test recovery
   pass-cli init
   pass-cli change-password --recover

   # Restore original config and clean up
   mv ~/.pass-cli/config.yml.backup ~/.pass-cli/config.yml 2>/dev/null || rm ~/.pass-cli/config.yml
   rm -f /tmp/test-vault.enc
   ```

## Advanced: Passphrase Protection

### What is a Passphrase?

A **BIP39 passphrase** (sometimes called the "25th word") is an additional secret you can add to your recovery phrase. It's like a second password that works alongside your 24 words.

### How to Enable

During `pass-cli init`:

```bash
Advanced: Add passphrase protection? (y/N): y

Enter passphrase (optional 25th word): ****
Confirm passphrase: ****

[PASS] Passphrase protection enabled
```

### Security Trade-offs

**Benefits**:
- [OK] Even if someone finds your 24 words, they still need the passphrase
- [OK] Plausible deniability (can have multiple vaults with same phrase + different passphrases)
- [OK] Extra layer of security

**Risks**:
- [ERROR] If you lose the passphrase, you **cannot** recover your vault
- [ERROR] Must remember/store passphrase separately from 24-word phrase
- [ERROR] More complex recovery process

**Recommendation**: Only use passphrase protection if you:
- Understand the risks
- Have secure way to store passphrase separately
- Are comfortable with added complexity

## Troubleshooting

### "Recovery not enabled for this vault" or "Recovery with mnemonic only supported for v2 vaults"

**Cause**: Your vault is still in V1 format.

**Solution**: Migrate to V2 format first. See [Migrating to V2 Format](#migrating-to-v2-format) above.

```bash
pass-cli vault migrate
```

### "Recovery phrase not enabled for this vault"

**Cause**: Vault was initialized with `--no-recovery` flag.

**Solution**: Recovery is not possible. You must remember your master password or restore from backup.

### "Invalid recovery word"

**Cause**: Word you entered is not in the BIP39 wordlist or doesn't match your phrase.

**Solutions**:
1. Check spelling carefully
2. Verify word position (word #7 = 7th word in your list)
3. Ensure you're reading from correct recovery phrase backup (not an old one from before migration)
4. Try typing word manually (not copy-paste)

### "Recovery verification failed"

**Cause**: Too many incorrect words entered, or wrong recovery phrase used.

**Solutions**:
1. Double-check your written recovery phrase (especially after migration)
2. Verify you're using the correct vault
3. If you migrated recently, make sure you're using the NEW recovery phrase (old one no longer works)
4. Ensure recovery phrase hasn't been transcribed incorrectly
5. If phrase is correct but failing, vault may be corrupted (restore from backup)

### Lost Recovery Phrase

**Unfortunately**: If you've lost your recovery phrase AND forgotten your master password, your vault is unrecoverable.

**Options**:
1. Check all secure storage locations
2. Check with trusted family members
3. Check backups (if you created them before losing the phrase)
4. If truly lost, you must reinitialize vault and re-add credentials

**Prevention**:
- Store recovery phrase in multiple secure locations
- Tell trusted family member where phrase is stored
- Include phrase location in your estate planning
- Keep a backup of your vault file in a secure location (separate from recovery phrase)

## See Also

- [Security Architecture](../03-reference/security-architecture.md#bip39-recovery-phrase) - Technical details of BIP39 implementation
- [Command Reference](../03-reference/command-reference.md#change-password---change-master-password) - `change-password --recover` command
- [Keychain Setup](keychain-setup.md) - Alternative to recovery phrase (OS keychain integration)
- [Backup & Restore](backup-restore.md) - Vault backup strategies
