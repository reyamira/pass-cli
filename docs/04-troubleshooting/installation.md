---
title: "Installation Issues"
weight: 1
toc: true
---
Solutions for installation and initialization problems.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)

## Installation Issues

### Command Not Found After Installation

**Symptom**: `pass-cli: command not found` or `'pass-cli' is not recognized`

**Cause**: Binary not in system PATH

**Solutions**:

**macOS/Linux:**
```bash
# Check if binary exists
which pass-cli

# If not found, check installation location
ls -la /usr/local/bin/pass-cli
ls -la ~/.local/bin/pass-cli

# Add to PATH if needed (add to ~/.bashrc or ~/.zshrc)
export PATH="$PATH:$HOME/.local/bin"
source ~/.bashrc

# Verify
pass-cli version
```

**Windows:**
```powershell
# Check if binary exists
where.exe pass-cli

# Add to PATH
$path = [Environment]::GetEnvironmentVariable("Path", "User")
$newPath = "$path;C:\path\to\pass-cli"
[Environment]::SetEnvironmentVariable("Path", $newPath, "User")

# Restart PowerShell
exit

# Verify
pass-cli version
```

---

### Permission Denied When Running

**Symptom**: `Permission denied` when executing pass-cli

**Cause**: Binary doesn't have execute permissions

**Solution (macOS/Linux)**:
```bash
# Add execute permission
chmod +x /path/to/pass-cli

# Or reinstall with correct permissions
sudo install -m 755 pass-cli /usr/local/bin/
```

---

### Homebrew Installation Fails

**Symptom**: `Error: No such file or directory` or tap not found

**Solutions**:
```bash
# Update Homebrew
brew update

# Check Homebrew status
brew doctor

# Remove and re-add tap
brew untap reyamira/pass-cli
brew tap reyamira/pass-cli

# Try verbose installation
brew install --verbose pass-cli

# Check logs if failed
brew gist-logs pass-cli
```

---

### Scoop Installation Fails

**Symptom**: Manifest not found or hash mismatch

**Solutions**:
```powershell
# Update Scoop
scoop update

# Check Scoop status
scoop status

# Remove and re-add bucket
scoop bucket rm pass-cli
scoop bucket add pass-cli https://github.com/reyamira/scoop-bucket

# Clear cache and retry
scoop cache rm pass-cli
scoop install pass-cli

# Check logs
scoop cat pass-cli
```

---

### macOS "Cannot Open" Security Warning

**Symptom**: "pass-cli cannot be opened because the developer cannot be verified"

**Cause**: macOS Gatekeeper blocks unsigned binaries

**Solutions**:

**Option 1: Remove quarantine attribute**
```bash
xattr -d com.apple.quarantine /usr/local/bin/pass-cli
```

**Option 2: Allow in System Preferences**
1. Try to run pass-cli
2. Open System Preferences → Security & Privacy
3. Click "Allow Anyway" next to pass-cli message
4. Run pass-cli again and confirm

**Option 3: Build from source** (trusted)
```bash
git clone https://github.com/reyamira/pass-cli
cd pass-cli
go build -o pass-cli .
sudo mv pass-cli /usr/local/bin/
```

---

## Initialization Issues

### "Vault Already Exists" Error

**Symptom**: `Error: vault already exists at ~/.pass-cli/vault.enc`

**Cause**: Trying to initialize when vault already exists

**Solutions**:

**Option 1: Use existing vault**
```bash
# Try to access existing vault
pass-cli list

# If you remember the password, continue using it
```

**Option 2: Backup and reinitialize**
```bash
# Backup existing vault
mv ~/.pass-cli/vault.enc ~/.pass-cli/vault.enc.old

# Initialize new vault
pass-cli init

# If needed, you can restore old vault later
# mv ~/.pass-cli/vault.enc.old ~/.pass-cli/vault.enc
```

**Option 3: Use different vault location**
```bash
# Configure custom vault location in config file
echo "vault_path: /path/to/new/vault.enc" > ~/.pass-cli/config.yml

# Then initialize
pass-cli init
```

---

### "Failed to Store Master Password" Error

**Symptom**: Error when saving master password to keychain

**Cause**: Keychain service not available or permission denied

**Solutions**:

**macOS:**
```bash
# Check keychain status
security list-keychains

# Unlock login keychain
security unlock-keychain ~/Library/Keychains/login.keychain-db

# Verify keychain access
security add-generic-password -a "$USER" -s "test" -w "test"
security delete-generic-password -a "$USER" -s "test"
```

**Linux:**
```bash
# Check if secret service is running
ps aux | grep -i "gnome-keyring\|kwallet"

# Start GNOME Keyring (if not running)
gnome-keyring-daemon --start

# Or install if missing
sudo apt install gnome-keyring  # Ubuntu/Debian
sudo dnf install gnome-keyring  # Fedora
```

**Windows:**
```powershell
# Run as administrator
# Check Credential Manager service
Get-Service -Name "VaultSvc"

# Start if stopped
Start-Service -Name "VaultSvc"
```

---

