---
title: "Homebrew Formula"
weight: 5
toc: true
---
This document explains how to use, test, and submit the Homebrew formula for Pass-CLI.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)

## Overview

The Homebrew formula enables easy installation of Pass-CLI on macOS and Linux systems through the Homebrew package manager.

## Formula Location

- **Formula File**: `homebrew/pass-cli.rb`
- **Purpose**: Distribution via Homebrew tap or official Homebrew repository

## Installation for Users

### From a Tap (Recommended for Testing)

```bash
# Add the tap
brew tap reyamira/homebrew-tap

# Install Pass-CLI
brew install pass-cli
```

### From Official Homebrew (After Submission)

```bash
brew install pass-cli
```

## Setting Up a Homebrew Tap

A Homebrew tap is a GitHub repository that contains Homebrew formulae.

### 1. Create the Tap Repository

```bash
# Create a new repository on GitHub named: homebrew-pass-cli
# The naming convention is important: homebrew-<tapname>
```

### 2. Initialize the Tap Repository

```bash
# Clone your new repository
git clone https://github.com/reyamira/homebrew-tap.git
cd homebrew-tap

# Create Formula directory
mkdir -p Formula

# Copy the formula
cp /path/to/pass-cli/homebrew/pass-cli.rb Formula/

# Commit and push
git add Formula/pass-cli.rb
git commit -m "Add Pass-CLI formula"
git push origin main
```

### 3. Update SHA256 Checksums

After creating a release, you need to update the SHA256 checksums in the formula:

```bash
# Download each release artifact and calculate its checksum
curl -L "https://github.com/reyamira/pass-cli/releases/download/v0.0.1/pass-cli_0.0.1_darwin_amd64.tar.gz" | sha256sum

# Repeat for each platform:
# - darwin_amd64
# - darwin_arm64
# - linux_amd64
# - linux_arm64

# Update the sha256 values in the formula
```

## Testing the Formula

### Local Testing

```bash
# Install from local formula file
brew install --build-from-source homebrew/pass-cli.rb

# Or use brew install with the tap
brew install --debug --verbose reyamira/homebrew-tap/pass-cli

# Test the installation
pass-cli version
pass-cli --help

# Run formula tests
brew test pass-cli

# Uninstall for re-testing
brew uninstall pass-cli
```

### Audit the Formula

Before submission, audit the formula to ensure it meets Homebrew standards:

```bash
# Check formula style and best practices
brew audit --new-formula pass-cli

# Strict audit (recommended before submission)
brew audit --strict --online pass-cli

# Style check
brew style pass-cli
```

### Test Installation on Multiple Platforms

Test on all supported platforms:

- macOS Intel (x86_64)
- macOS Apple Silicon (arm64)
- Linux Intel (x86_64)
- Linux ARM (aarch64)

## Submitting to Official Homebrew

### Prerequisites

1. **Stable Release**: Must have a stable version with release artifacts
2. **Open Source**: Must have an OSI-approved license (MIT [PASS])
3. **Notable Project**: Should have some community adoption
4. **Documentation**: README, LICENSE, and proper documentation

### Submission Process

1. **Fork the Homebrew Repository**
   ```bash
   gh repo fork homebrew/homebrew-core --clone
   cd homebrew-core
   ```

2. **Create a New Branch**
   ```bash
   git checkout -b pass-cli
   ```

3. **Add Your Formula**
   ```bash
   cp /path/to/pass-cli/homebrew/pass-cli.rb Formula/
   ```

4. **Test Thoroughly**
   ```bash
   brew install --build-from-source Formula/pass-cli.rb
   brew test pass-cli
   brew audit --strict --online pass-cli
   ```

5. **Create Pull Request**
   ```bash
   git add Formula/pass-cli.rb
   git commit -m "pass-cli 0.0.1 (new formula)"
   git push origin pass-cli

   # Create PR on GitHub
   gh pr create --title "pass-cli 0.0.1 (new formula)" \
                --body "Secure CLI password manager with AES-256-GCM encryption"
   ```

### PR Requirements

Your submission must include:

- [ ] Formula file in `Formula/pass-cli.rb`
- [ ] Working installation test
- [ ] All audits passing
- [ ] Accurate description and homepage
- [ ] Valid license
- [ ] SHA256 checksums for all platforms
- [ ] Commit message format: `pass-cli 0.0.1 (new formula)`

## Updating the Formula

When releasing a new version:

1. **Update Version Number**
   ```ruby
   version "1.1.0"
   ```

2. **Update URLs**
   ```ruby
   url "https://github.com/reyamira/pass-cli/releases/download/v1.1.0/..."
   ```

3. **Update SHA256 Checksums**
   - Download new release artifacts
   - Calculate new checksums
   - Update formula

4. **Test the Update**
   ```bash
   brew upgrade pass-cli
   brew test pass-cli
   ```

5. **Submit PR** (if in official Homebrew)
   ```bash
   git checkout -b pass-cli-1.1.0
   git add Formula/pass-cli.rb
   git commit -m "pass-cli 1.1.0"
   git push origin pass-cli-1.1.0
   gh pr create
   ```

## Automated Updates

Consider adding automation to update checksums:

```bash
# Example script to calculate checksums
#!/bin/bash
VERSION="0.0.1"
BASE_URL="https://github.com/reyamira/pass-cli/releases/download/v${VERSION}"

for os in darwin linux; do
  for arch in amd64 arm64; do
    URL="${BASE_URL}/pass-cli_${VERSION}_${os}_${arch}.tar.gz"
    echo "Downloading ${os}_${arch}..."
    SHA=$(curl -sL "$URL" | shasum -a 256 | cut -d' ' -f1)
    echo "${os}_${arch}: ${SHA}"
  done
done
```

## Formula Best Practices

### Do's
- [OK] Use stable release URLs (not `latest`)
- [OK] Include accurate SHA256 checksums
- [OK] Support all relevant platforms
- [OK] Include meaningful tests
- [OK] Add shell completion generation
- [OK] Provide helpful caveats for first-time users
- [OK] Keep formula simple and maintainable

### Don'ts
- [ERROR] Don't use `latest` tag in URLs
- [ERROR] Don't skip checksums
- [ERROR] Don't include build-time patches without good reason
- [ERROR] Don't add unnecessary dependencies
- [ERROR] Don't use deprecated Homebrew DSL features

## Troubleshooting

### Checksum Mismatch
```bash
# Recalculate the checksum
curl -sL "YOUR_URL" | shasum -a 256
```

### Installation Fails
```bash
# Check detailed logs
brew install --verbose --debug pass-cli

# Check formula syntax
brew audit pass-cli
```

### Formula Rejected
- Review Homebrew's [Acceptable Formulae](https://docs.brew.sh/Acceptable-Formulae)
- Check [Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
- Ensure all tests pass
- Follow reviewer feedback

## Resources

- [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
- [Homebrew Acceptable Formulae](https://docs.brew.sh/Acceptable-Formulae)
- [How to Create and Maintain a Tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap)
- [Ruby Formula DSL](https://rubydoc.brew.sh/Formula)
- [Homebrew GitHub](https://github.com/Homebrew/homebrew-core)

## Support

For issues with the formula:
1. Check the [Homebrew documentation](https://docs.brew.sh/)
2. Review existing [Homebrew PRs](https://github.com/Homebrew/homebrew-core/pulls)
3. Ask in [Homebrew Discussions](https://github.com/Homebrew/homebrew-core/discussions)
4. File an issue in the pass-cli repository

## Maintenance Checklist

For maintainers updating the formula:

- [ ] Update version number
- [ ] Update all platform URLs
- [ ] Calculate and update all SHA256 checksums
- [ ] Test installation on macOS (Intel and ARM)
- [ ] Test installation on Linux
- [ ] Run `brew audit --strict --online pass-cli`
- [ ] Run `brew test pass-cli`
- [ ] Update tap repository (if applicable)
- [ ] Submit PR to homebrew-core (if applicable)
- [ ] Tag release in pass-cli repository
- [ ] Update release notes
