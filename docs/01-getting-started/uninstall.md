---
title: "Uninstall"
weight: 4
toc: true
---

Complete instructions for removing pass-cli from your system.

## Uninstallation

### Homebrew

```bash
# Uninstall Pass-CLI
brew uninstall pass-cli

# Remove tap (optional)
brew untap reyamira/homebrew-tap

# Remove vault (if desired)
rm -rf ~/.pass-cli
```

### Scoop

```powershell
# Uninstall Pass-CLI
scoop uninstall pass-cli

# Remove bucket (optional)
scoop bucket rm pass-cli

# Remove vault (if desired)
Remove-Item -Recurse -Force ~/.pass-cli
```

### Manual Installation

```bash
# Remove binary
sudo rm /usr/local/bin/pass-cli

# Or user installation
rm ~/.local/bin/pass-cli

# Remove vault (if desired)
rm -rf ~/.pass-cli
```

### Windows Manual Installation

```powershell
# Remove binary
Remove-Item "C:\Program Files\pass-cli\pass-cli.exe"

# Remove from PATH (if manually added)
# System Properties → Environment Variables → Edit Path

# Remove vault (if desired)
Remove-Item -Recurse -Force "$env:USERPROFILE\.pass-cli"
```

### Complete Removal

To completely remove all traces of Pass-CLI:

```bash
# 1. Uninstall binary (using method above)

# 2. Remove vault
rm -rf ~/.pass-cli

# 3. Remove master password from keychain
# macOS: Open Keychain Access → Search "pass-cli" → Delete
# Linux: Use your keyring manager (Seahorse, etc.)
# Windows: Credential Manager → Remove pass-cli entries

# 4. Remove config (if exists)
rm ~/.pass-cli.yaml

# 5. Clear shell history (optional)
history -c
```

## Platform-Specific Notes

### macOS

- **Apple Silicon**: Use ARM64 version for native performance
- **Intel**: Use amd64 version
- **Keychain**: Integration is automatic
- **Homebrew**: Recommended installation method

### Linux

- **Package Managers**: Homebrew works on Linux too
- **Keychain**: Requires Secret Service (GNOME Keyring or KWallet)
- **AppArmor/SELinux**: May need profile adjustments for keychain access
- **Distribution Packages**: May become available for specific distros

### Windows

- **Scoop**: Recommended installation method
- **Credential Manager**: Integration is automatic
- **Antivirus**: May need to whitelist pass-cli.exe
- **PATH**: Requires manual setup for manual installation

## Getting Help

If you encounter issues not covered here:

1. Check the [Troubleshooting Guide](../04-troubleshooting/_index)
2. Review [GitHub Issues](https://github.com/reyamira/pass-cli/issues)
3. Ask in [GitHub Discussions](https://github.com/reyamira/pass-cli/discussions)
4. File a [new issue](https://github.com/reyamira/pass-cli/issues/new)

## Next Steps

After uninstalling, you might want to:

- Review the [Security Architecture](../03-reference/security-architecture)
- Check [pass-cli Documentation](https://reyamira.github.io/pass-cli/) for other topics

