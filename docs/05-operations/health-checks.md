---
title: "Health Checks"
weight: 1
toc: true
---
The `pass-cli doctor` command provides comprehensive health checks for your pass-cli installation. Use it to diagnose issues, verify your setup, and ensure everything is working correctly.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)
.

## Overview

```bash
pass-cli doctor [flags]
```

The doctor command runs a series of health checks and reports the status of your pass-cli installation. It's designed to be both human-readable and script-friendly.

## What It Checks

The doctor command performs 6 comprehensive health checks:

1. **Version Check**: Compares your installed version against the latest GitHub release
2. **Vault Check**: Verifies vault file existence, permissions, and integrity
3. **Config Check**: Validates configuration file syntax and settings
4. **Keychain Check**: Tests OS keychain integration (Windows/macOS/Linux)
5. **Backup Check**: Verifies backup file accessibility and integrity
6. **Sync Check** (if enabled): Verifies rclone installation, remote configuration, and connectivity

## Command Options

### Standard Output (Human-Readable)

```bash
pass-cli doctor
```

Produces formatted output with status indicators:

```text
Health Check Results
====================

[PASS] Version: v1.2.3 (up to date)
[PASS] Vault: vault.enc accessible (600 permissions)
[PASS] Config: Valid configuration
[PASS] Keychain: Integration active
[PASS] Backup: 3 backup files found

Overall Status: HEALTHY
```

### JSON Output (Script-Friendly)

```bash
pass-cli doctor --json
```

Produces machine-readable JSON:

```json
{
  "overall_status": "pass",
  "checks": [
    {
      "name": "version",
      "status": "pass",
      "message": "v1.2.3 (up to date)",
      "details": {
        "current": "v1.2.3",
        "latest": "v1.2.3",
        "up_to_date": true
      }
    },
    {
      "name": "vault",
      "status": "pass",
      "message": "vault.enc accessible (600 permissions)",
      "details": {
        "path": "/home/user/.pass-cli/vault.enc",
        "exists": true,
        "readable": true,
        "permissions": "600"
      }
    }
  ]
}
```

### Quiet Mode (Exit Code Only)

```bash
pass-cli doctor --quiet
```

Produces no output. Use the exit code to determine health status:

```bash
if pass-cli doctor --quiet; then
  echo "System healthy"
else
  echo "Issues detected"
fi
```

### Custom Vault Path

To check a custom vault, configure `vault_path` in your config file:

```bash
# Configure custom vault in config file
echo "vault_path: /path/to/custom/vault.enc" > ~/.pass-cli/config.yml

# Run doctor
pass-cli doctor
```

## Exit Codes

The doctor command uses exit codes to indicate overall health:

- **0**: All checks passed (HEALTHY)
- **1**: One or more warnings detected (WARNINGS)
- **2**: One or more errors detected (ERRORS)

Exit codes enable script integration:

```bash
#!/bin/bash
pass-cli doctor --quiet
case $? in
  0) echo "All systems operational" ;;
  1) echo "Warnings detected - review recommended" ;;
  2) echo "Errors detected - action required" ;;
esac
```

## Common Issues and Recommendations

### Version Check

#### Update Available (Warning)

**Symptom**:
```text
⚠ Version: Update available: v1.2.3 → v1.2.4
  Recommendation: Update to latest version: https://github.com/reyamira/pass-cli/releases/tag/v1.2.4
```

**Solution**: Update pass-cli to the latest version using your package manager or download from GitHub.

#### Network Timeout (Pass With Error)

**Symptom**:
```text
[PASS] Version: Current version: v1.2.3 (unable to check for updates: offline)
```

**Details**: The check gracefully falls back when offline. This is not an error - it just means the version check couldn't reach GitHub. Your current version information is still displayed.

### Vault Check

#### Vault Not Found (Error)

**Symptom**:
```text
[FAIL] Vault: Vault file not found
  Recommendation: Run 'pass-cli init' to create a new vault
```

**Solution**: Initialize a new vault with `pass-cli init` or configure the correct vault path in your config file (`vault_path` setting).

#### Permission Issues (Error)

**Symptom**:
```text
[FAIL] Vault: Vault file has insecure permissions (644)
  Recommendation: Run 'chmod 600 /home/user/.pass-cli/vault.enc'
```

**Solution**: Fix file permissions to restrict vault access:

```bash
chmod 600 ~/.pass-cli/vault.enc
```

On Windows, ensure only your user account has read/write access.

#### Vault Corrupted (Error)

**Symptom**:
```text
[FAIL] Vault: Vault file is corrupted or tampered
  Recommendation: Restore from backup or reinitialize vault
```

**Solution**:
1. Check for backup files: `ls ~/.pass-cli/vault.enc.backup*`
2. Restore from backup: `cp ~/.pass-cli/vault.enc.backup.1 ~/.pass-cli/vault.enc`
3. If no backups exist, you may need to reinitialize: `pass-cli init`

### Config Check

#### Invalid Configuration (Error)

**Symptom**:
```text
[FAIL] Config: Invalid YAML syntax at line 5
  Recommendation: Fix configuration syntax or delete to use defaults
```

**Solution**: Edit `~/.pass-cli/config.yaml` to fix syntax errors, or delete the file to regenerate defaults.

#### Missing Configuration (Pass)

**Symptom**:
```text
[PASS] Config: Using default configuration
```

**Details**: This is normal if you haven't customized your configuration. The system uses sensible defaults.

### Keychain Check

#### Keychain Unavailable (Warning)

**Symptom**:
```text
⚠ Keychain: OS keychain not available (running in SSH session)
  Recommendation: Use local environment or enable keychain access
```

**Solution**:
- **SSH sessions**: Keychain may not be available remotely. Use password entry instead.
- **Linux**: Ensure `libsecret` is installed (`apt-get install libsecret-1-0` on Debian/Ubuntu)
- **macOS**: Grant Terminal/iTerm2 keychain access in System Preferences
- **Windows**: Ensure Credential Manager service is running

#### Permission Denied (Error)

**Symptom**:
```text
[FAIL] Keychain: Access denied by OS
  Recommendation: Grant keychain access in system settings
```

**Solution**:
- **macOS**: System Preferences → Security & Privacy → Privacy → Keychain → Allow pass-cli
- **Windows**: Run as administrator once to grant initial access
- **Linux**: Check user group membership for `keyring` or `gnome-keyring`

### Backup Check

#### No Backups Found (Warning)

**Symptom**:
```text
⚠ Backup: No backup files found
  Recommendation: Backups are created automatically after modifications
```

**Details**: This is normal for new vaults. Backups are created automatically when you add/update/delete credentials.

#### Backup Integrity Issue (Warning)

**Symptom**:
```text
⚠ Backup: Backup file corrupted: vault.enc.backup.2
  Recommendation: Remove corrupted backup or restore from valid backup
```

**Solution**: Remove the corrupted backup file:

```bash
rm ~/.pass-cli/vault.enc.backup.2
```

### Sync Check

This check only appears if sync is enabled in your configuration.

#### Sync Enabled and Working (Pass)

**Symptom**:
```text
[PASS] Sync: Enabled and healthy
  Remote: gdrive:.pass-cli
  rclone installed: Yes
```

**Details**: Sync is properly configured and working. Your vault will sync automatically on operations.

#### Rclone Not Installed (Error)

**Symptom**:
```text
[FAIL] Sync: rclone not found
  Recommendation: Install rclone to enable sync: https://rclone.org/install.sh
```

**Solution**: Install rclone using your package manager or the installation script:

```bash
# macOS
brew install rclone

# Windows
scoop install rclone

# Linux
curl https://rclone.org/install.sh | sudo bash
```

#### Remote Not Configured (Error)

**Symptom**:
```text
[FAIL] Sync: Remote not configured or invalid
  Recommendation: Check sync.remote setting in ~/.pass-cli/config.yml
```

**Solution**: Verify the remote is properly configured:

```bash
# List configured remotes
rclone listremotes

# Test connectivity to your configured remote
rclone ls gdrive:.pass-cli
```

#### Remote Connectivity Failed (Error)

**Symptom**:
```text
[FAIL] Sync: Cannot reach remote 'gdrive:.pass-cli'
  Recommendation: Check rclone configuration or network connectivity
```

**Solution**:

1. Verify your rclone configuration:
   ```bash
   rclone config
   ```

2. Test remote connectivity:
   ```bash
   rclone ls gdrive:.pass-cli
   ```

3. Check network connectivity:
   ```bash
   ping google.com
   ```

## Script Integration Examples

### Pre-Operation Health Check

Run a health check before automated operations:

```bash
#!/bin/bash
# Pre-flight check before automated password retrieval

if ! pass-cli doctor --quiet; then
  echo "ERROR: Health check failed" >&2
  pass-cli doctor  # Show details
  exit 1
fi

# Proceed with operation
password=$(pass-cli get myservice --quiet --field password)
```

### Scheduled Monitoring

Add to cron for periodic health checks:

```bash
# Run health check daily and email if issues detected
0 9 * * * pass-cli doctor --quiet || echo "pass-cli health issues detected" | mail -s "pass-cli Alert" admin@example.com
```

### CI/CD Integration

Verify pass-cli health in deployment pipelines:

```yaml
# GitHub Actions example
- name: Verify pass-cli health
  run: |
    pass-cli doctor --json > health-report.json
    if [ $? -ne 0 ]; then
      cat health-report.json
      exit 1
    fi
```

### JSON Output Processing

Parse JSON output for specific checks:

```bash
#!/bin/bash
# Check if vault needs backup

result=$(pass-cli doctor --json)
backup_status=$(echo "$result" | jq -r '.checks[] | select(.name=="backup") | .status')

if [ "$backup_status" = "warning" ]; then
  echo "Backup warning detected - triggering manual backup"
  cp ~/.pass-cli/vault.enc ~/.pass-cli/vault.enc.backup.manual
fi
```

### Prometheus/Monitoring Export

Convert health checks to metrics:

```bash
#!/bin/bash
# Export health status as Prometheus metrics

result=$(pass-cli doctor --json)
overall=$(echo "$result" | jq -r '.overall_status')

case "$overall" in
  pass) metric=0 ;;
  warning) metric=1 ;;
  error) metric=2 ;;
esac

echo "pass_cli_health_status $metric" >> /var/lib/prometheus/node_exporter/pass_cli.prom
```

## Troubleshooting Tips

### Run With Verbose Logging

For detailed diagnostic information:

```bash
pass-cli doctor --verbose
```

### Check Specific Component

Use `--json` and `jq` to filter specific checks:

```bash
# Check only vault status
pass-cli doctor --json | jq '.checks[] | select(.name=="vault")'

# Check only keychain status
pass-cli doctor --json | jq '.checks[] | select(.name=="keychain")'
```

### Offline Mode

Doctor command works offline - only the version check requires network access. All other checks run locally.

### First-Time Setup

If you run `doctor` on a fresh installation:

```bash
$ pass-cli doctor

[FAIL] Vault: Vault file not found
  Recommendation: Run 'pass-cli init' to create a new vault

Overall Status: ERROR (exit code 2)
```

This is expected. Run `pass-cli init` to create your vault, then `doctor` will show healthy status.

## See Also

- [Quick Start Guide](../01-getting-started/quick-start) - First-time setup and initialization
- [Troubleshooting](../04-troubleshooting/_index) - Common issues and solutions
- [Security Operations](security-operations) - Security best practices
- [Command Reference](../03-reference/command-reference) - Complete command reference

