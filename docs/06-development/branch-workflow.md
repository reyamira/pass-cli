---
title: "Branch Workflow"
weight: 2
toc: true
---
This document describes the branching strategy and workflow for pass-cli development.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)


## Branch Structure

### `main` Branch
- **Purpose**: Production-ready code and active development
- **Protection**: Requires PR review and passing CI checks
- **Updates**: Via pull requests from feature/spec branches
- **Default**: Yes (users cloning repo get this)
- **Tags**: All release tags are created on main
- **CI**: Runs on every push and PR

## Daily Workflow

### Starting Work

```bash
# Clone repo (if new)
git clone https://github.com/reyamira/pass-cli.git
cd pass-cli

# Make sure you're up to date
git pull origin main
```

### Making Changes

**For features and bug fixes:**

```bash
# Create a feature branch from main
git checkout -b feat/my-feature-name

# Make your changes
# ... edit files ...

# Commit frequently with clear messages
git add .
git commit -m "feat: add new feature"

# Push your branch
git push origin feat/my-feature-name

# Create PR via GitHub
# Go to: https://github.com/reyamira/pass-cli/compare/main...feat/my-feature-name
# CI runs automatically on your PR
```

**For spec work:**

```bash
# Create a spec branch
git checkout -b spec/001-feature-name

# Implement the spec (commit after each task/phase)
git commit -m "feat: implement spec phase 1"
git commit -m "test: add tests for spec requirements"
git commit -m "docs: update spec completion report"

# Push and create PR
git push origin spec/001-feature-name
# Create PR with spec completion summary
```

### Creating Pull Requests

1. **Push your branch** to origin
2. **Go to GitHub** and create a pull request to `main`
3. **CI runs automatically** - lint, tests, security scan, build
4. **Wait for CI to pass** (required before merge)
5. **Review changes** if needed
6. **Merge PR** when CI is green

### After Merge

```bash
# Update your local main
git checkout main
git pull origin main

# Delete your local feature branch (optional)
git branch -d feat/my-feature-name
```

## Branch Naming Conventions

- **Features**: `feat/descriptive-name` or `feature/descriptive-name`
- **Bug fixes**: `fix/issue-description`
- **Specs**: `spec/NNN-feature-name`
- **Hotfixes**: `hotfix/critical-bug`
- **Experiments**: `exp/experiment-name`

## Release Process

When ready to release:

```bash
# Ensure main is clean and CI passing
git checkout main
git pull origin main

# Run full test suite locally
go test ./...
go test -v -tags=integration -timeout 5m ./test

# Create and push release tag
git tag -a v0.x.x -m "Release v0.x.x: Brief description"
git push origin v0.x.x

# Release workflow runs automatically
```

The release workflow will:
1. Run full CI suite (tests, lint, security)
2. Build binaries for all platforms
3. Create GitHub release with artifacts
4. Update package manifests (Homebrew, Scoop)

## Branch Protection

### Main Branch Protection

**Enabled protections:**
- [OK] Require pull request before merging
- [OK] Require status checks to pass:
  - `Detect Code Changes`
  - `Lint`
  - `Unit Tests`
  - `Integration Tests (ubuntu-latest)`
  - `Integration Tests (macos-latest)`
  - `Integration Tests (windows-latest)`
  - `Security Scan`
  - `Build`
- [OK] Block force pushes
- [OK] Restrict deletions
- [OK] Repository admins can bypass (for emergency fixes)

**Result**: All changes require PR and passing CI. Direct pushes blocked.

## CI/CD Pipeline

### On Feature Branch Push
```text
Feature branch push → No CI (saves compute time)
```

### On PR to `main`
```text
PR created → CI runs automatically (lint, tests, security, build)
          → PR shows CI status
          → Merge blocked until CI passes
```

**Smart filtering**: CI skips test jobs when only non-code files change (docs/, specs/, .md files).

### On Merge to `main`
```text
PR merged → main branch updated → CI runs on main (verification)
```

### On Release Tag
```text
Tag pushed to main (v*) →
  Release workflow runs →
  Build binaries for all platforms →
  Create GitHub release →
  Update Homebrew tap →
  Update Scoop bucket
```

## Emergency Hotfixes

For critical production bugs:

```bash
# Create hotfix branch from main
git checkout -b hotfix/critical-bug main

# Fix the bug
# ... make changes ...
git commit -m "fix: critical security vulnerability"

# Push and create PR
git push origin hotfix/critical-bug

# Create PR: hotfix/critical-bug → main
# Expedite review and merge
# After merge, tag release immediately
git checkout main
git pull origin main
git tag v0.x.x
git push origin v0.x.x
```

## Common Commands

### Check Which Branch You're On
```bash
git branch --show-current
```

### See Recent Commits on Main
```bash
git log main --oneline -10
```

### Compare Your Branch With Main
```bash
git diff main..HEAD
```

### Update Your Branch With Latest Main
```bash
# While on your feature branch
git fetch origin
git rebase origin/main

# Or if you prefer merging
git merge origin/main
```

### List All Branches
```bash
git branch -a
```

### Delete Merged Feature Branches
```bash
# Delete local branch
git branch -d feat/my-feature

# Delete remote branch
git push origin --delete feat/my-feature
```

## Tips

1. **Always work on feature branches** - Don't work directly on main
2. **Commit frequently** - Small, focused commits are better
3. **Test locally first** - Run `go test ./...` before pushing
4. **Write clear PR descriptions** - Explain what and why
5. **Keep PRs focused** - One feature/fix per PR
6. **Rebase before PR** - Keep history clean with `git rebase origin/main`
7. **Delete branches after merge** - Keep repo tidy

## Troubleshooting

### "Cannot Push to Main" Error
This is expected! Create a pull request from your feature branch instead.

```bash
# If you accidentally committed to main locally
git branch feat/my-feature  # Create branch from current main
git reset --hard origin/main  # Reset main to match remote
git checkout feat/my-feature  # Switch to your feature branch
# Now push and create PR
```

### PR Has Merge Conflicts

```bash
# Update your branch with latest main
git checkout your-feature-branch
git fetch origin
git rebase origin/main

# Resolve conflicts in your editor
# After resolving each file:
git add <resolved-file>
git rebase --continue

# Push updated branch
git push origin your-feature-branch --force-with-lease
```

### CI Failing on Your PR

```bash
# Pull the latest changes from your PR branch
git checkout your-feature-branch
git pull origin your-feature-branch

# Fix the failing tests/linting locally
go test ./...
golangci-lint run

# Commit the fixes
git commit -m "fix: resolve CI failures"
git push origin your-feature-branch

# CI will automatically re-run on the PR
```

### Forgot to Create Feature Branch

```bash
# If you made changes directly on main
git stash  # Save your changes
git checkout -b feat/my-feature  # Create feature branch
git stash pop  # Apply your changes
git add .
git commit -m "feat: description"
git push origin feat/my-feature
```

## References

- GitHub Actions: https://github.com/reyamira/pass-cli/actions
- Branch Settings: https://github.com/reyamira/pass-cli/settings/rules
- Release Workflow: `.github/workflows/release.yml`
- CI Workflow: `.github/workflows/ci.yml`
- Pull Requests: https://github.com/reyamira/pass-cli/pulls
