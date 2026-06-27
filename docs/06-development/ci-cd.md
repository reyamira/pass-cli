---
title: "CI/CD Pipeline"
weight: 3
toc: true
---
This document describes the automated CI/CD pipeline for Pass-CLI.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)


## Workflows

### CI Workflow (`ci.yml`)

**Trigger**: Push to `main` branch or pull requests to `main`

**Jobs**:

1. **Test** (Matrix: Ubuntu, macOS, Windows)
   - Runs unit tests with race detection
   - Generates code coverage reports
   - Uploads coverage to Codecov

2. **Integration Test** (Matrix: Ubuntu, macOS, Windows)
   - Runs integration tests with build tags
   - Tests E2E workflows across platforms
   - 5-minute timeout for complete test suite

3. **Lint**
   - Runs golangci-lint with comprehensive checks
   - Enforces code quality standards
   - Fails on any linting issues

4. **Security Scan**
   - Runs Gosec security scanner
   - Generates SARIF report for GitHub Security
   - Identifies potential security vulnerabilities

5. **Build**
   - Runs GoReleaser in snapshot mode
   - Builds for all platforms without publishing
   - Uploads build artifacts for verification

### Release Workflow (`release.yml`)

**Trigger**: Git tags matching `v*` pattern

**Jobs**:

1. **Test Before Release**
   - Runs complete unit test suite
   - Runs integration tests
   - Must pass before release proceeds

2. **Lint Before Release**
   - Runs full linting suite
   - Ensures code quality standards met

3. **Security Scan Before Release**
   - Final security check before release
   - Must complete successfully

4. **Release**
   - Runs GoReleaser for production release
   - Builds all platform binaries
   - Creates GitHub release with artifacts
   - Generates checksums and SBOMs

5. **Verify Release** (Matrix: Ubuntu, macOS, Windows)
   - Downloads released artifacts
   - Verifies checksums
   - Tests binary extraction
   - Runs version command verification

## Workflow Features

### Fail-Fast Strategy

- Tests and linting must pass before builds
- Security scans must complete before release
- Any failure stops the pipeline

### Matrix Testing

- **Platforms**: Ubuntu, macOS, Windows
- **Go Version**: 1.25 (pinned for consistency)
- Ensures cross-platform compatibility

### Caching

- Go module cache enabled
- Build cache enabled
- Speeds up CI runs significantly

### Artifact Management

- Build artifacts retained for 7 days
- Release artifacts retained for 30 days
- Coverage reports uploaded to Codecov

## Dependabot Integration

**Configuration**: `.github/dependabot.yml`

**Updates**:
- GitHub Actions (weekly)
- Go modules (weekly)
- Automatic PR creation for updates

**Features**:
- Groups patch updates together
- Labels PRs automatically
- Assigns reviewers

## Required Secrets

### GitHub Token

`GITHUB_TOKEN` is automatically provided by GitHub Actions for:
- Creating releases
- Uploading artifacts
- Commenting on PRs

### Optional Secrets

For advanced features, you may configure:

- **CODECOV_TOKEN**: For private repository coverage uploads
- **GPG_KEY**: For signing releases (if configured)
- **SLACK_WEBHOOK**: For release notifications

## Local Testing

### Test CI Workflow Locally

```bash
# Run unit tests like CI
go test -v -race -coverprofile=coverage.txt ./...

# Run integration tests like CI
go test -v -tags=integration -timeout 5m ./test

# Run linter like CI
golangci-lint run --timeout=5m

# Run security scan like CI
gosec ./...

# Build like CI
goreleaser build --snapshot --clean
```

### Test Release Workflow Locally

```bash
# Full release dry run
goreleaser release --snapshot --clean --skip=publish

# Verify checksums
cd dist
sha256sum -c checksums.txt
```

## Triggering Releases

### Creating a Release

```bash
# Ensure on main branch and up-to-date
git checkout main
git pull origin main

# Run full test suite
go test ./...
go test -v -tags=integration -timeout 5m ./test

# Create and push tag
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

### Release Process

1. Tag is pushed to GitHub
2. Release workflow is triggered
3. Tests run across all platforms
4. Linting and security scans execute
5. GoReleaser builds all binaries
6. GitHub release is created
7. Artifacts are uploaded
8. Verification runs on all platforms

### Monitoring Releases

- Watch GitHub Actions tab for progress
- Check release page for artifacts
- Review logs if any failures occur
- Download and test binaries

## Workflow Permissions

### CI Workflow

- `contents: read` - Read repository code

### Release Workflow

- `contents: write` - Create releases and upload artifacts
- `packages: write` - Publish packages (if configured)

## Troubleshooting

### Workflow Failures

**Test Failures**:
```bash
# Run tests locally to debug
go test -v -race ./...
go test -v -tags=integration ./test
```

**Lint Failures**:
```bash
# Fix linting issues
golangci-lint run --fix
```

**Build Failures**:
```bash
# Test cross-compilation
GOOS=linux GOARCH=amd64 go build .
GOOS=darwin GOARCH=arm64 go build .
GOOS=windows GOARCH=amd64 go build .
```

**Release Failures**:
```bash
# Validate GoReleaser config
goreleaser check

# Dry run release
goreleaser release --snapshot --clean --skip=publish
```

### Common Issues

**"Go language version mismatch" (golangci-lint)**:
- **Issue**: golangci-lint fails with "binary was built with go X but current version is Y"
- **Root Cause**: golangci-lint must be built with a Go version >= the project's Go version
- **Solution**: Pin golangci-lint to a version built with compatible Go
- **Example**: For Go 1.21+ projects, use golangci-lint v1.55+ with golangci-lint-action v6
  ```yaml
  - name: Run golangci-lint
    uses: golangci/golangci-lint-action@v6
    with:
      version: v1.55  # Built with Go 1.21+
  ```
- **Reference**: https://github.com/golangci/golangci-lint/issues/5873

**"Resource not accessible by integration"**:
- Check workflow permissions
- Ensure GITHUB_TOKEN has proper scopes
- For cross-repo updates (Homebrew/Scoop), use Personal Access Tokens:
  ```yaml
  env:
    HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
    SCOOP_BUCKET_TOKEN: ${{ secrets.SCOOP_BUCKET_TOKEN }}
  ```

**"No matching tag"**:
- Verify tag format matches `v*`
- Ensure tag is pushed to remote

**"Build timeout"**:
- Increase timeout in workflow
- Optimize build process

**"Artifact upload failed"**:
- Check artifact size limits
- Verify artifact paths exist

## Best Practices

1. **Always test locally first**: Run tests and builds before pushing
2. **Use semantic versioning**: Follow semver for tags
3. **Write good commit messages**: They become release notes
4. **Monitor workflow runs**: Check Actions tab after pushing
5. **Review release artifacts**: Verify before announcing
6. **Keep dependencies updated**: Review Dependabot PRs
7. **Document breaking changes**: Update CHANGELOG.md

## Workflow Badges

Add to README.md:

```markdown
[![CI](https://github.com/reyamira/pass-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/reyamira/pass-cli/actions/workflows/ci.yml)
[![Release](https://github.com/reyamira/pass-cli/actions/workflows/release.yml/badge.svg)](https://github.com/reyamira/pass-cli/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/reyamira/pass-cli)](https://goreportcard.com/report/github.com/reyamira/pass-cli)
[![codecov](https://codecov.io/gh/reyamira/pass-cli/branch/main/graph/badge.svg)](https://codecov.io/gh/reyamira/pass-cli)
```

## Security

### Code Scanning

- Gosec runs on every push and PR
- SARIF results uploaded to GitHub Security
- Vulnerabilities appear in Security tab

### Dependency Scanning

- Dependabot scans for vulnerable dependencies
- Creates PRs for security updates
- Alerts appear in Security tab

### Best Practices

- Review security scan results
- Update dependencies promptly
- Don't commit secrets to repository
- Use GitHub Secrets for sensitive data

## See Also

- [GoReleaser Documentation](https://goreleaser.com)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Release Process](release)
- [Branch Workflow](branch-workflow)
