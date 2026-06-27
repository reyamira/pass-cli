---
title: "Release Process"
weight: 4
toc: true
---
This document describes the release process for Pass-CLI using GoReleaser.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)


## Prerequisites

1. **GoReleaser installed**: `go install github.com/goreleaser/goreleaser/v2@latest`
2. **Git tags**: Releases are triggered by pushing git tags
3. **GitHub token**: Required for GitHub releases (set `GITHUB_TOKEN` env var)
4. **Clean working directory**: No uncommitted changes

## Local Testing

### Build for Current Platform
```bash
# Test build for your current platform
goreleaser build --snapshot --clean --single-target

# Check the binary
./dist/pass-cli_<platform>/pass-cli version
```

### Build All Platforms
```bash
# Test full multi-platform build without publishing
goreleaser build --snapshot --clean

# Verify all binaries were created
ls -lh dist/
```

### Full Release Dry Run
```bash
# Simulate a complete release (no publishing)
goreleaser release --snapshot --clean --skip=publish
```

## Release Process

### 1. Prepare Release

```bash
# Ensure you're on main and up-to-date
git checkout main
git pull origin main

# Run all tests
go test ./...
go test -v -tags=integration -timeout 5m ./test

# Run code quality checks
go fmt ./...
go vet ./...
golangci-lint run
```

### 2. Create Release Tag

**Note:** Version numbers and dates in documentation are **automatically updated** by GitHub Actions when you push a tag. You don't need to manually update them.

The `update-docs-version.yml` workflow automatically updates:
- Documentation version footers (8 files in `docs/`)
- "Last Updated" dates to current month/year
- Package manifest versions (homebrew/pass-cli.rb, scoop/pass-cli.json)

**What you need to do manually:**
- Update CHANGELOG.md with release notes (if you maintain one)

```bash
# Create and push a version tag
git tag -a v0.0.1 -m "Release v0.0.1"
git push origin v0.0.1
```

### 3. Run GoReleaser

```bash
# Release to GitHub
export GITHUB_TOKEN="your-github-token"
goreleaser release --clean

# Or use goreleaser with GitHub Actions (recommended)
# Push the tag and let CI handle the release
```

## Configuration

### Version Injection

GoReleaser injects version information at build time via ldflags:

```yaml
ldflags:
  - -s -w  # Strip debug info (reduces binary size)
  - -X github.com/arimxyer/pass-cli/cmd.version={{.Version}}
  - -X github.com/arimxyer/pass-cli/cmd.commit={{.ShortCommit}}
  - -X github.com/arimxyer/pass-cli/cmd.date={{.Date}}
```

### Supported Platforms

- **Windows**: amd64, arm64
- **macOS**: amd64, arm64 (with universal binary)
- **Linux**: amd64, arm64

### Build Flags

- `-trimpath`: Remove file system paths from binaries
- `-mod=readonly`: Ensure go.mod is not modified
- `CGO_ENABLED=0`: Static linking (no C dependencies)
- `netgo` tag: Pure Go networking stack

### Artifacts Generated

For each release, GoReleaser creates:

1. **Binaries**: Cross-compiled for all platforms
2. **Archives**: `.tar.gz` for Unix, `.zip` for Windows
3. **Checksums**: SHA-256 checksums for verification
4. **SBOMs**: Software Bill of Materials (security compliance)
5. **Release notes**: Auto-generated from commits

## Versioning

Pass-CLI follows [Semantic Versioning](https://semver.org/):

- **MAJOR** (v1.0.0): Incompatible API changes
- **MINOR** (v0.1.0): New features (backward compatible)
- **PATCH** (v0.0.2): Bug fixes (backward compatible)

**Current Release**: v0.0.1 (Initial release)

## GitHub Actions Integration

Example `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25.1'

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

## Binary Sizes

Typical binary sizes (with `-s -w` flags):

- Windows amd64: ~6.2 MB
- macOS arm64: ~6.0 MB
- Linux amd64: ~6.1 MB

All well under the 20MB target.

## Verification

### Verify Checksums

```bash
# Download checksums.txt and verify
sha256sum -c checksums.txt
```

### Test Binaries

```bash
# Download and test each platform binary
./pass-cli-linux-amd64 version
./pass-cli-darwin-arm64 version
./pass-cli-windows-amd64.exe version
```

## Troubleshooting

### "dirty" Git State

```bash
# GoReleaser requires clean working directory
git status
git stash  # or commit changes
```

### Missing GitHub Token

```bash
# Set GitHub token for releases
export GITHUB_TOKEN=$(gh auth token)

# Or create a personal access token:
# https://github.com/settings/tokens
```

### Build Failures

```bash
# Check individual platform builds
GOOS=linux GOARCH=amd64 go build .
GOOS=darwin GOARCH=arm64 go build .
GOOS=windows GOARCH=amd64 go build .
```

### Archive Issues

```bash
# Ensure required files exist
ls LICENSE README.md CHANGELOG.md

# Or adjust archives.files in .goreleaser.yml
```

## Best Practices

1. **Test before tagging**: Always run full test suite and quality checks
2. **Use semantic versioning**: Follow semver strictly
3. **Write good commit messages**: They become release notes
4. **Keep CHANGELOG updated**: Manual changelog alongside auto-generated notes
5. **Test binaries**: Download and test at least one binary per platform
6. **Sign releases**: Consider adding GPG signing for security
7. **Document breaking changes**: Clearly mark in release notes

## Advanced Features

### Universal macOS Binaries

GoReleaser automatically creates universal binaries for macOS that work on both Intel and Apple Silicon:

```yaml
universal_binaries:
  - id: pass-cli-darwin
    replace: true
    name_template: "pass-cli"
```

### Custom Release Notes

Override auto-generated notes by creating `.goreleaser.yml`:

```yaml
release:
  header: |
    ## Custom Header
  footer: |
    ## Custom Footer
```

### Multiple Archives

Create different archives for different audiences:

```yaml
archives:
  - id: default
    # Standard archives
  - id: minimal
    # Minimal archives (binary only)
    files: []
```

## See Also

- [GoReleaser Documentation](https://goreleaser.com/)
- [Semantic Versioning](https://semver.org/)
- [Keep a Changelog](https://keepachangelog.com/)
- [GitHub Releases](https://docs.github.com/en/repositories/releasing-projects-on-github)
