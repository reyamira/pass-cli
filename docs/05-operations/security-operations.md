---
title: "Security Operations"
weight: 2
toc: true
---

Best practices, security checklist, incident response procedures, and security audits for Pass-CLI.

## Best Practices

### Password Management

1. **Strong Master Password**
   ```text
   [OK] Good: "correct-horse-battery-staple-29!" (33 chars)
   [OK] Good: "MyD0g!sN@med$potAnd1L0veH1m" (29 chars)
   [ERROR] Bad:  "password123" (11 chars, common)
   [ERROR] Bad:  "MyPassword1" (11 chars, predictable)
   ```

2. **Password Storage**
   - Write master password in password manager (ironic but practical)
   - Or write on paper, store in safe place
   - Don't store in plaintext file

3. **Password Rotation**
   - Change master password periodically
   - Rotate individual credentials regularly
   - Use `pass-cli generate` for new credentials

### Operational Security

1. **Vault Backups**
   ```bash
   # Regular backups
   cp ~/.pass-cli/vault.enc ~/backups/vault-$(date +%Y%m%d).enc

   # Store backups securely (encrypted drive, safe location)
   ```

2. **Clipboard Security**
   - Clipboard cleared automatically after 5 seconds
   - Avoid pasting into untrusted applications
   - Use `--no-clipboard` if concerned

3. **Script Security**
   ```bash
   # [OK] Good: Use quiet mode to avoid logging
   export API_KEY=$(pass-cli get service --quiet)

   # [ERROR] Bad: Full output might be logged
   export API_KEY=$(pass-cli get service)
   ```

4. **Audit Usage**
   ```bash
   # Review unused credentials monthly
   pass-cli list --unused --days 90

   # Delete obsolete credentials
   pass-cli delete old-service
   ```

### TUI-Specific Security

1. **Screen Privacy**
   - [WARNING] **Shoulder Surfing Risk**: TUI displays credential list on screen
   - Use privacy screen protector in public spaces
   - Be aware of people nearby when using TUI
   - Consider using CLI mode for sensitive environments

2. **Password Visibility Toggle**
   - `Ctrl+P` in add/edit forms shows passwords in plaintext
   - **Only use in private, trusted environments**
   - Password resets to masked when form closes
   - Be cautious in:
     - Open offices
     - Coffee shops
     - Shared workspaces
     - Screen sharing sessions
     - Video calls with screen share

3. **Screen Recording Protection**
   - TUI displays service names and usernames by default
   - Pause screen recording before launching TUI
   - Use CLI mode with `--quiet` when recording tutorials
   - Consider: `pass-cli list --format simple` for screen shares

4. **Shared Terminal Sessions**
   - **Never use TUI on shared terminal sessions**
   - tmux/screen sessions visible to other users
   - Use CLI mode with `--no-clipboard` instead
   - SSH sessions: ensure you control the connection

5. **Terminal History**
   - TUI mode doesn't log to shell history
   - CLI commands may appear in history
   - Clear history after sensitive operations:
     ```bash
     history -c  # Clear session history
     ```

### System Security

1. **Secure Your OS Account**
   - Use strong OS login password
   - Enable full-disk encryption
   - Keep system updated

2. **File System Security**
   - Don't commit vault to version control
   - Add to `.gitignore`:
     ```ini
     .pass-cli/
     *.enc
     ```

3. **Access Control**
   - Don't run Pass-CLI as root/admin
   - Use regular user account
   - Verify vault file permissions

### Development Security

1. **Testing**
   ```bash
   # Use separate vault for testing (configure in config file)
   echo "vault_path: /tmp/test-vault.enc" > ~/.pass-cli/config-test.yml
   pass-cli init

   # Clean up after testing
   rm -f /tmp/test-vault.enc
   rm -f ~/.pass-cli/config-test.yml
   ```

2. **Debugging**
   - Use `--verbose` flag, not hardcoded logging
   - Don't log credential values
   - Clear terminal after debugging

## Security Checklist

### Initial Setup
- [ ] Strong master password (20+ characters)
- [ ] Master password backed up securely
- [ ] Vault file permissions verified (0600)
- [ ] System keychain configured correctly

### Regular Maintenance
- [ ] Vault backed up monthly
- [ ] Unused credentials reviewed quarterly
- [ ] Master password rotated annually
- [ ] Pass-CLI updated to latest version

### Incident Response
- [ ] Master password changed if compromised
- [ ] Vault file restored from backup if corrupted
- [ ] All credentials rotated if vault possibly compromised
- [ ] System scan for malware if suspicious activity

## Incident Response

### Master Password Compromised

1. **Immediate Actions**
   - Change master password: `pass-cli init` (if you have access)
   - Or delete vault and start fresh
   - Rotate all credentials stored in vault

2. **Investigation**
   - Scan system for malware
   - Check keychain access logs (if available)
   - Review who had access to system

3. **Prevention**
   - Use stronger master password
   - Enable full-disk encryption
   - Review system security practices

### Vault File Corrupted

1. **Recovery**
   ```bash
   # Restore from backup
   cp ~/.pass-cli/vault.enc.backup ~/.pass-cli/vault.enc

   # Or from manual backup
   cp ~/backups/vault-20250120.enc ~/.pass-cli/vault.enc
   ```

2. **Verification**
   ```bash
   # Test vault access
   pass-cli list
   ```

3. **Prevention**
   - Regular backups
   - Atomic writes (built-in)
   - Don't manually edit vault file

### Credential Leaked

1. **Immediate**
   - Rotate credential immediately on actual service
   - Generate new password: `pass-cli generate` (copy output)
   - Update in Pass-CLI: `pass-cli update service` (paste when prompted)

2. **Investigation**
   - Identify leak source (logs, clipboard, screen share)
   - Review usage tracking: `pass-cli get service --json`

3. **Prevention**
   - Use `--quiet` mode in scripts
   - Clear shell history: `history -c`
   - Review script logging

## Security Audits

### Internal Audits

Run security checks regularly:

```bash
# Check vault permissions
ls -la ~/.pass-cli/

# Verify no plaintext secrets in code
grep -r "password.*=" .

# Run security scanner
gosec ./...

# Check for vulnerable dependencies
govulncheck ./...
```

### External Audits

Pass-CLI has not yet undergone external security audit. We welcome security researchers to review the code.

### Reporting Security Issues

**DO NOT** file public issues for security vulnerabilities.

Instead, use GitHub's private security advisory feature to report vulnerabilities:
- Visit: https://github.com/reyamira/pass-cli/security/advisories/new
- Include: Detailed description, reproduction steps, impact assessment
- Disclosure: Coordinated disclosure after fix

### Security Updates

Security updates are released as:
- **Critical**: Immediate release, notification to users
- **High**: Release within 7 days
- **Medium**: Release in next version

Check for updates:
```bash
pass-cli version
# Compare with latest: https://github.com/reyamira/pass-cli/releases
```

## Cryptographic Algorithm Details

### AES-256-GCM Parameters

- **Block Size**: 128 bits
- **Key Size**: 256 bits
- **Nonce Size**: 96 bits (12 bytes) - NIST recommended
- **Tag Size**: 128 bits (16 bytes) - Full authentication
- **Additional Data**: None (not needed for our use case)

### PBKDF2 Parameters

- **Iteration Count**: 600,000 (hardened)
  - Provides ~50-100ms delay on modern CPUs (2023+)
  - Older hardware: 500-1000ms (acceptable per NIST recommendations)
  - Significantly increases brute-force cost
- **Salt Size**: 256 bits (32 bytes)
  - Unique per vault
  - Prevents rainbow table attacks
- **Hash Function**: SHA-256
  - NIST approved
  - 256-bit output matches key size

## Compliance and Standards

### Standards Compliance

- **NIST SP 800-38D**: AES-GCM mode
- **NIST SP 800-132**: PBKDF2 recommendations
- **NIST FIPS 197**: AES algorithm
- **RFC 5869**: PBKDF2 specification

### Best Practices Followed

- **OWASP**: Secure coding practices
- **CWE**: Common weakness mitigation
- **SANS**: Security implementation guidelines

## Further Reading

- [AES-GCM Specification (NIST SP 800-38D)](https://nvlpubs.nist.gov/nistpubs/Legacy/SP/nistspecialpublication800-38d.pdf)
- [PBKDF2 Specification (RFC 2898)](https://www.rfc-editor.org/rfc/rfc2898)
- [Go Cryptography Documentation](https://pkg.go.dev/crypto)
- [OWASP Cryptographic Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html)

