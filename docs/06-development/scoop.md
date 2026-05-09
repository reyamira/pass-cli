---
title: "Scoop Manifest"
weight: 6
toc: true
---
This document explains how to use, test, and submit the Scoop manifest for Pass-CLI on Windows.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)


## Overview

The Scoop manifest enables easy installation of Pass-CLI on Windows systems through the Scoop package manager.

## Manifest Location

- **Manifest File**: `scoop/pass-cli.json`
- **Purpose**: Distribution via Scoop bucket or official Scoop repository

## Installation for Users

### From a Bucket (Recommended for Testing)

```powershell
# Add the bucket
scoop bucket add pass-cli https://github.com/reyamira/scoop-bucket

# Install Pass-CLI
scoop install pass-cli
```

### From Official Scoop Buckets (After Submission)

```powershell
# From main bucket
scoop install pass-cli

# Or from extras bucket
scoop bucket add extras
scoop install pass-cli
```

## Setting Up a Scoop Bucket

A Scoop bucket is a GitHub repository that contains Scoop manifests.

### 1. Create the Bucket Repository

```powershell
# Create a new repository on GitHub named: scoop-pass-cli
# Note: Unlike Homebrew, the naming is flexible but should be descriptive
```

### 2. Initialize the Bucket Repository

```powershell
# Clone your new repository
git clone https://github.com/reyamira/scoop-bucket.git
cd scoop-bucket

# Create bucket directory structure
mkdir bucket

# Copy the manifest
cp path\to\pass-cli\scoop\pass-cli.json bucket\

# Commit and push
git add bucket\pass-cli.json
git commit -m "Add Pass-CLI manifest"
git push origin main
```

### 3. Update SHA256 Hashes

After creating a release, you need to update the SHA256 hashes in the manifest:

```powershell
# Download each release artifact and calculate its hash
$url = "https://github.com/reyamira/pass-cli/releases/download/v0.0.1/pass-cli_0.0.1_windows_amd64.zip"
$hash = (Get-FileHash (Invoke-WebRequest $url -OutFile temp.zip -PassThru).FullName -Algorithm SHA256).Hash

# Repeat for each architecture:
# - windows_amd64
# - windows_arm64

# Update the hash values in the manifest
```

Alternatively, use Scoop's built-in hash tool:

```powershell
scoop hash https://github.com/reyamira/pass-cli/releases/download/v0.0.1/pass-cli_0.0.1_windows_amd64.zip
```

## Testing the Manifest

### Local Testing

```powershell
# Install from local manifest file
scoop install .\scoop\pass-cli.json

# Or test from the bucket
scoop bucket add pass-cli https://github.com/reyamira/scoop-bucket
scoop install pass-cli

# Test the installation
pass-cli version
pass-cli --help

# Uninstall for re-testing
scoop uninstall pass-cli
```

### Validate the Manifest

```powershell
# Check manifest format and schema
scoop checkver pass-cli

# Test autoupdate (dry run)
scoop checkver pass-cli -u

# Full validation
cd scoop-pass-cli
.\bin\checkver.ps1 pass-cli -u
```

### Test on Multiple Architectures

Test on all supported Windows architectures:
- Windows x64 (amd64)
- Windows ARM64

## Manifest Structure Explained

### Basic Fields

```json
{
  "version": "0.0.1",              // Current version
  "description": "...",             // Short description
  "homepage": "...",                // Project homepage
  "license": "MIT"                  // License identifier
}
```

### Architecture-Specific URLs

```json
{
  "architecture": {
    "64bit": {
      "url": "...",                 // Download URL for x64
      "hash": "..."                 // SHA256 hash
    },
    "arm64": {
      "url": "...",                 // Download URL for ARM64
      "hash": "..."                 // SHA256 hash
    }
  }
}
```

### Binary Configuration

```json
{
  "bin": "pass-cli.exe"            // Executable to add to PATH
}
```

### Autoupdate Configuration

```json
{
  "checkver": {
    "github": "https://github.com/reyamira/pass-cli"
  },
  "autoupdate": {
    "architecture": {
      "64bit": {
        "url": "https://.../$version/pass-cli_$version_windows_amd64.zip"
      },
      "arm64": {
        "url": "https://.../$version/pass-cli_$version_windows_arm64.zip"
      }
    },
    "hash": {
      "url": "$baseurl/checksums.txt",
      "regex": "$sha256\\s+$basename"
    }
  }
}
```

### Post-Install Messages

```json
{
  "post_install": [
    "Write-Host 'Installation complete!' -ForegroundColor Green"
  ],
  "notes": [
    "Quick start guide here..."
  ]
}
```

## Checksums File

For autoupdate to work, create a `checksums.txt` file with each release:

```text
abc123...  pass-cli_0.0.1_windows_amd64.zip
def456...  pass-cli_0.0.1_windows_arm64.zip
```

This can be automated in your release process:

```powershell
# Generate checksums.txt
$files = Get-ChildItem .\dist\*.zip
foreach ($file in $files) {
    $hash = (Get-FileHash $file -Algorithm SHA256).Hash.ToLower()
    Add-Content checksums.txt "$hash  $($file.Name)"
}
```

Or in your `.goreleaser.yml`:

```yaml
checksum:
  name_template: 'checksums.txt'
  algorithm: sha256
```

## Submitting to Official Scoop Buckets

### Prerequisites

1. **Stable Release**: Must have a stable version with release artifacts
2. **Open Source**: Must have an OSI-approved license (MIT [PASS])
3. **Windows Binary**: Must provide Windows executables
4. **Documentation**: README and LICENSE required

### Choosing the Right Bucket

- **Main Bucket**: Command-line tools that don't require GUI
  - Repository: [ScoopInstaller/Main](https://github.com/ScoopInstaller/Main)
  - Best fit for Pass-CLI [PASS]

- **Extras Bucket**: GUI apps and less common tools
  - Repository: [ScoopInstaller/Extras](https://github.com/ScoopInstaller/Extras)

### Submission Process

1. **Fork the Bucket Repository**
   ```powershell
   gh repo fork ScoopInstaller/Main --clone
   cd Main
   ```

2. **Create a New Branch**
   ```powershell
   git checkout -b pass-cli
   ```

3. **Add Your Manifest**
   ```powershell
   cp path\to\pass-cli\scoop\pass-cli.json bucket\
   ```

4. **Test Thoroughly**
   ```powershell
   # Test installation
   scoop install .\bucket\pass-cli.json

   # Test the binary
   pass-cli version
   pass-cli --help

   # Test checkver
   .\bin\checkver.ps1 pass-cli -u

   # Validate manifest
   .\bin\formatjson.ps1 bucket\pass-cli.json
   ```

5. **Create Pull Request**
   ```powershell
   git add bucket\pass-cli.json
   git commit -m "pass-cli: Add version 0.0.1"
   git push origin pass-cli

   # Create PR on GitHub
   gh pr create --title "pass-cli: Add version 0.0.1" `
                --body "Secure CLI password manager with AES-256-GCM encryption"
   ```

### PR Requirements

Your submission must include:

- [ ] Manifest file in `bucket/pass-cli.json`
- [ ] Valid JSON format
- [ ] Correct SHA256 hashes
- [ ] Working autoupdate configuration
- [ ] Tested installation
- [ ] Commit message format: `pass-cli: Add version 0.0.1`
- [ ] PR description explaining the app

## Updating the Manifest

When releasing a new version:

### Manual Update

1. **Update Version**
   ```json
   "version": "1.1.0"
   ```

2. **Update URLs**
   ```json
   "url": "https://github.com/reyamira/pass-cli/releases/download/v1.1.0/..."
   ```

3. **Update Hashes**
   ```powershell
   scoop hash https://github.com/reyamira/pass-cli/releases/download/v1.1.0/pass-cli_1.1.0_windows_amd64.zip
   ```

4. **Test and Submit**
   ```powershell
   scoop uninstall pass-cli
   scoop install .\bucket\pass-cli.json
   git commit -am "pass-cli: Update to version 1.1.0"
   ```

### Automatic Update (With Checkver)

If autoupdate is configured correctly, maintainers can update automatically:

```powershell
# Check for new version
scoop checkver pass-cli

# Update manifest automatically
scoop checkver pass-cli -u

# Verify and commit
git diff bucket\pass-cli.json
git commit -am "pass-cli: Update to version X.Y.Z"
```

## Automation Script

Create a script to automate hash updates:

```powershell
# update-scoop-manifest.ps1
param(
    [Parameter(Mandatory=$true)]
    [string]$Version
)

$baseUrl = "https://github.com/reyamira/pass-cli/releases/download/v$Version"
$manifest = Get-Content scoop\pass-cli.json | ConvertFrom-Json

# Update version
$manifest.version = $Version

# Update hashes
$amd64Url = "$baseUrl/pass-cli_${Version}_windows_amd64.zip"
$arm64Url = "$baseUrl/pass-cli_${Version}_windows_arm64.zip"

Write-Host "Calculating hash for amd64..."
$manifest.architecture.'64bit'.hash = (scoop hash $amd64Url)

Write-Host "Calculating hash for arm64..."
$manifest.architecture.arm64.hash = (scoop hash $arm64Url)

# Save manifest
$manifest | ConvertTo-Json -Depth 10 | Set-Content scoop\pass-cli.json

Write-Host "Manifest updated to version $Version" -ForegroundColor Green
```

Usage:

```powershell
.\update-scoop-manifest.ps1 -Version "1.1.0"
```

## Manifest Best Practices

### Do's
- [OK] Use stable release URLs (not `latest`)
- [OK] Include accurate SHA256 hashes
- [OK] Support all relevant architectures
- [OK] Configure autoupdate correctly
- [OK] Provide helpful post-install messages
- [OK] Use proper JSON formatting
- [OK] Test on all supported architectures
- [OK] Follow Scoop manifest conventions

### Don'ts
- [ERROR] Don't use `latest` tag in URLs
- [ERROR] Don't skip hash validation
- [ERROR] Don't hardcode version numbers in autoupdate URLs
- [ERROR] Don't include unnecessary dependencies
- [ERROR] Don't use deprecated manifest features
- [ERROR] Don't forget to update checksums.txt

## Troubleshooting

### Hash Mismatch

```powershell
# Recalculate the hash
scoop hash https://github.com/reyamira/pass-cli/releases/download/v0.0.1/pass-cli_0.0.1_windows_amd64.zip

# Or manually
$hash = (Get-FileHash .\pass-cli_0.0.1_windows_amd64.zip).Hash
```

### Installation Fails

```powershell
# Check detailed output
scoop install pass-cli -v

# Validate manifest format
scoop checkver pass-cli

# Check Scoop status
scoop status
```

### Autoupdate Not Working

```powershell
# Test checkver
scoop checkver pass-cli

# Verify checksums.txt format
# Should be: <hash>  <filename>

# Test autoupdate regex
scoop checkver pass-cli -u
```

### Manifest Rejected

Common reasons:
- Invalid JSON format → Run through JSON validator
- Missing required fields → Check manifest schema
- Incorrect hash → Recalculate with `scoop hash`
- Autoupdate not configured → Add checkver/autoupdate blocks
- Not following naming conventions → Review Scoop guidelines

## Resources

- [Scoop Documentation](https://scoop.sh/)
- [App Manifests](https://github.com/ScoopInstaller/Scoop/wiki/App-Manifests)
- [Creating an App Manifest](https://github.com/ScoopInstaller/Scoop/wiki/Creating-an-app-manifest)
- [Autoupdate](https://github.com/ScoopInstaller/Scoop/wiki/App-Manifest-Autoupdate)
- [Main Bucket](https://github.com/ScoopInstaller/Main)
- [Extras Bucket](https://github.com/ScoopInstaller/Extras)
- [Scoop Directory](https://scoop.sh/#/apps) - Browse available apps

## Support

For issues with the manifest:
1. Check the [Scoop Wiki](https://github.com/ScoopInstaller/Scoop/wiki)
2. Review [existing manifests](https://github.com/ScoopInstaller/Main/tree/master/bucket)
3. Ask in [Scoop Discussions](https://github.com/ScoopInstaller/Scoop/discussions)
4. File an issue in the pass-cli repository

## Maintenance Checklist

For maintainers updating the manifest:

- [ ] Update version number
- [ ] Update all architecture URLs
- [ ] Calculate and update all SHA256 hashes
- [ ] Test installation on Windows x64
- [ ] Test installation on Windows ARM64 (if available)
- [ ] Verify autoupdate configuration
- [ ] Run `scoop checkver pass-cli -u`
- [ ] Update bucket repository (if applicable)
- [ ] Submit PR to ScoopInstaller/Main (if applicable)
- [ ] Ensure checksums.txt is uploaded with release
- [ ] Tag release in pass-cli repository
- [ ] Update release notes

## Integration With CI/CD

Add to your release workflow:

```yaml
# .github/workflows/release.yml
- name: Generate Scoop checksums
  run: |
    cd dist
    Get-ChildItem *.zip | ForEach-Object {
      $hash = (Get-FileHash $_ -Algorithm SHA256).Hash.ToLower()
      Add-Content checksums.txt "$hash  $($_.Name)"
    }

- name: Upload checksums
  uses: actions/upload-release-asset@v1
  with:
    upload_url: ${{ steps.create_release.outputs.upload_url }}
    asset_path: ./dist/checksums.txt
    asset_name: checksums.txt
    asset_content_type: text/plain
```

## Testing Checklist

Before submitting:

- [ ] JSON is valid (`scoop checkver pass-cli`)
- [ ] Both architectures install correctly
- [ ] Binary is in PATH after installation
- [ ] `pass-cli version` works
- [ ] `pass-cli --help` works
- [ ] Post-install messages display correctly
- [ ] Autoupdate detects new versions
- [ ] Hash validation passes
- [ ] Checksums.txt format is correct
- [ ] Uninstall works cleanly
