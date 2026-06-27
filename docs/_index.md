---
title: "Pass-CLI Documentation"
cascade:
  type: docs
---

<div class="hx-mt-6"></div>

![pass-cli](/images/social-preview.svg)

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)

Welcome to the **pass-cli** documentation. A secure, cross-platform, always-free, and open-source alternative to 1password, bitwarden, etc., Password and API key manager for folks who live in the command line. (CLI + TUI)

## Quick Links

- [Quick Start](01-getting-started/quick-start) - First-time setup and initialization (5 minutes)
- [Quick Install](01-getting-started/quick-install) - Installation instructions for all platforms
- [Command Reference](03-reference/command-reference) - Complete command reference
- [Recovery Phrase](02-guides/recovery-phrase) - BIP39 recovery phrase setup and usage
- [Backup & Restore Guide](02-guides/backup-restore) - Manual vault backup management
- [TOTP & 2FA Support](02-guides/totp-guide) - Store and generate 2FA codes
- [Security Architecture](03-reference/security-architecture) - Security features and cryptography
- [Troubleshooting](04-troubleshooting/_index) - Common issues and solutions by category

## Features

- **Strong Encryption**: AES-256-GCM with PBKDF2 key derivation (600,000 iterations)
- **BIP39 Recovery**: 24-word mnemonic for vault password recovery
- **Cross-Platform**: Works on Windows, macOS, and Linux
- **Keychain Integration**: Optional OS keychain support for automatic unlocking
- **Interactive TUI**: Beautiful terminal UI built with tview
- **Clipboard Support**: Secure clipboard integration with auto-clear
- **Usage Tracking**: Per-credential usage statistics
- **Audit Logging**: HMAC-signed audit logs for all operations
- **Manual Backups**: Create and restore vault backups on demand

## Getting Help

- **GitHub Issues**: [Report bugs or request features](https://github.com/reyamira/pass-cli/issues)
- **GitHub Discussions**: [Ask questions and share ideas](https://github.com/reyamira/pass-cli/discussions)
- **Documentation**: You're reading it!

## Contributing

See [Contributing Guide](06-development/contributing) for developer documentation and contribution guidelines.
