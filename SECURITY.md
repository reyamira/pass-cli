# Security Policy

## Supported Versions

We release security updates for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 0.9.x   | :white_check_mark: |
| < 0.9   | :x:                |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please report security vulnerabilities through GitHub's private vulnerability reporting:

1. Go to the [Security tab](https://github.com/reyamira/pass-cli/security)
2. Click "Report a vulnerability"
3. Fill out the advisory form with the details below

### What to Include

Please include the following information:
- Type of vulnerability (e.g., injection, cryptographic weakness, information disclosure)
- Full paths of affected source files
- Location of the affected code (tag/branch/commit or direct URL)
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact assessment and potential attack scenarios

### Response Timeline

- **Initial Response**: Within 48 hours
- **Status Update**: Within 7 days
- **Fix Timeline**: Critical issues within 30 days, others within 90 days

### Disclosure Policy

We follow coordinated disclosure:
1. You report the vulnerability privately
2. We confirm and investigate
3. We develop and test a fix
4. We release the fix in a new version
5. We publish a security advisory (crediting you if desired)
6. After 90 days, full details may be publicly disclosed

### Security Considerations

Pass-CLI is a security-focused tool with the following protections:

**Cryptography**:
- AES-256-GCM encryption with PBKDF2-SHA256 (600,000 iterations)
- Unique salt per vault, unique IV per credential
- Built-in authentication tags prevent tampering

**Secure Storage**:
- Credentials stored in encrypted vault files
- Master passwords stored in OS keychain (Windows Credential Manager, macOS Keychain, Linux Secret Service)
- Password memory cleared after use

**BIP39 Recovery Phrase**:
- 24-word mnemonic phrase for vault recovery (industry-standard BIP39)
- 6-word challenge provides 2^66 security (73.8 quintillion combinations)
- Encrypted storage of 18 words using AES-256-GCM
- Optional passphrase protection (25th word) for additional security layer
- Memory clearing for mnemonic, seeds, and keys using `crypto.ClearBytes()`
- **CRITICAL**: Store recovery phrase offline (paper, not digital)
- **WARNING**: Recovery phrase provides vault access - treat as master password equivalent

**Recovery Phrase Security Best Practices**:
- ✅ Write on archival-quality paper with permanent ink
- ✅ Store in physical safe, lockbox, or safety deposit box
- ✅ Keep separate from vault location and master password
- ✅ Test recovery once after initialization to verify backup
- ❌ Never store digitally (photos, screenshots, notes apps, cloud storage)
- ❌ Never share with anyone (including support personnel)
- ❌ Never type into untrusted devices or online tools
- ❌ Never memorize only - always maintain physical backup

**Audit Logging**:
- Optional HMAC-SHA256 signed audit trail
- Tamper-evident operation tracking

For detailed security architecture, see [docs/03-reference/security-architecture.md](docs/03-reference/security-architecture.md).

## Bug Bounty Program

We do not currently offer a paid bug bounty program, but we will acknowledge security researchers in our release notes and CHANGELOG.

## Security Updates

Security updates are published as GitHub releases and documented in:
- [CHANGELOG.md](CHANGELOG.md)
- GitHub Security Advisories

Subscribe to repository releases to receive security notifications.
