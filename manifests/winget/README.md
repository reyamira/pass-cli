# winget Manifest for Pass-CLI

This directory contains the Windows Package Manager (winget) manifest for Pass-CLI.

## Submission Process

## Status

⚠️ **Future Distribution Channel** - winget distribution is planned for future releases.

After a stable release (v1.0.0+), follow these steps to submit Pass-CLI to the official winget repository:

### 1. Update the manifest with real checksums

After the release is built, update `pass-cli.yaml` with the actual SHA256 hashes:

```powershell
# Get checksums from the GitHub release checksums.txt file
# Replace VERSION with actual release version
curl -L https://github.com/reyamira/pass-cli/releases/download/vVERSION/checksums.txt
```

Replace `PLACEHOLDER_HASH_AMD64` and `PLACEHOLDER_HASH_ARM64` with the actual SHA256 values.

### 2. Validate the manifest

Install the winget manifest validation tool:

```powershell
winget install --id Microsoft.WingetCreate
```

Validate your manifest:

```powershell
wingetcreate validate manifests/winget/pass-cli.yaml
```

### 3. Fork the winget-pkgs repository

```bash
gh repo fork microsoft/winget-pkgs --clone=false
```

### 4. Create a pull request

Use the `wingetcreate` tool to submit:

```powershell
wingetcreate submit --token YOUR_GITHUB_TOKEN manifests/winget/pass-cli.yaml
```

Or manually:

1. Clone your fork of `microsoft/winget-pkgs`
2. Create a new branch: `git checkout -b arimxyer.pass-cli-1.0.0`
3. Copy manifest to: `manifests/a/arimxyer/pass-cli/1.0.0/`
4. Commit and push
5. Create PR to microsoft/winget-pkgs

### 5. Wait for review

- Automated checks will validate the manifest
- Microsoft reviewers will approve (typically 3-7 days)
- Once merged, Pass-CLI will be available via `winget install arimxyer.pass-cli`

## Testing Locally

Before submitting, test the manifest locally:

```powershell
# Install from local manifest
winget install --manifest manifests/winget/pass-cli.yaml

# Verify installation
pass-cli version

# Uninstall
winget uninstall arimxyer.pass-cli
```

## References

- [winget-pkgs repository](https://github.com/microsoft/winget-pkgs)
- [Manifest documentation](https://learn.microsoft.com/en-us/windows/package-manager/package/manifest)
- [Submission guidelines](https://github.com/microsoft/winget-pkgs/blob/master/CONTRIBUTING.md)
