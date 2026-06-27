---
title: "Contributing"
weight: 1
toc: true
---
This guide covers the development workflow for Pass-CLI contributors.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)


## Prerequisites

### Required Tools

- **Go 1.25+**: [Download](https://go.dev/dl/)
- **Git**: For version control
- **golangci-lint**: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
- **GoReleaser**: `go install github.com/goreleaser/goreleaser/v2@latest`

### Optional Tools

- **gosec**: Security scanner (`go install github.com/securego/gosec/v2/cmd/gosec@latest`)
- **govulncheck**: Vulnerability checker (`go install golang.org/x/vuln/cmd/govulncheck@latest`)

## Quick Start

```bash
# Clone the repository
git clone https://github.com/reyamira/pass-cli.git
cd pass-cli

# Install dependencies
go mod download

# Build the binary
go build -o pass-cli .

# Run tests
go test ./...

# Run the binary
./pass-cli --help
```

## Common Commands

### Building

```bash
# Build the binary
go build -o pass-cli .

# Build with debug info
go build -gcflags="all=-N -l" -o pass-cli .

# Install to GOPATH
go install .

# Cross-compile (example for Windows)
GOOS=windows GOARCH=amd64 go build -o pass-cli.exe .
```

### Testing

```bash
# Run unit tests
go test ./...

# Run tests with race detection
go test -race ./...

# Generate HTML coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Show coverage summary
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# Run integration tests (5min timeout)
go test -v -tags=integration -timeout 5m ./test

# Run integration tests (skip slow tests)
go test -v -tags=integration -short -timeout 2m ./test
```

### Code Quality

```bash
# Format code
go fmt ./...

# Run go vet
go vet ./...

# Run golangci-lint
golangci-lint run

# Pre-commit checks (run all)
go fmt ./...
go vet ./...
golangci-lint run
go test -race ./...
gosec ./...
```

### Security

```bash
# Run gosec security scanner
gosec ./...

# Check for vulnerable dependencies
govulncheck ./...
```

### Release

```bash
# Validate GoReleaser config
goreleaser check

# Test full release (no publish)
goreleaser release --snapshot --clean --skip=publish

# Build snapshot release locally
goreleaser build --snapshot --clean
```

### Dependencies

```bash
# Tidy and verify go.mod
go mod tidy
go mod verify

# Update all dependencies
go get -u ./...
go mod tidy

# Show dependency graph
go mod graph | grep pass-cli | head -20
```

### Cleanup

```bash
# Remove build artifacts
go clean
rm -f pass-cli pass-cli.exe coverage.out coverage.html
```

## Development Workflow

### Making Changes

1. **Create a branch**:
   ```bash
   git checkout -b feature/my-feature
   ```

2. **Make your changes**:
   - Write code following Go best practices
   - Add tests for new functionality
   - Update documentation as needed

3. **Test your changes**:
   ```bash
   go test ./...                              # Run unit tests
   go test -v -tags=integration ./test        # Run integration tests
   golangci-lint run                          # Check code quality
   gosec ./...                                # Security check
   ```

4. **Commit your changes**:
   ```bash
   git add .
   git commit -m "feat: add new feature"
   ```

5. **Push and create PR**:
   ```bash
   git push origin feature/my-feature
   # Then create a pull request on GitHub
   ```

### Before Committing

Run the pre-commit checks:

```bash
go fmt ./...
go vet ./...
golangci-lint run
go test -race ./...
gosec ./...
```

This ensures:
- Code is properly formatted
- No suspicious constructs (`go vet`)
- Passes linter checks
- No race conditions
- No security issues

### Before Creating a PR

Ensure all tests pass:

```bash
go test ./...                              # Unit tests
go test -v -tags=integration ./test        # Integration tests
golangci-lint run                          # Linting
gosec ./...                                # Security check
```

### Before Releasing

Run full pre-release validation:

```bash
go fmt ./...
go vet ./...
golangci-lint run
go test -race ./...
go test -v -tags=integration ./test
gosec ./...
govulncheck ./...
goreleaser check
```

This runs:
- All code quality checks
- All tests (unit + integration)
- Security scanning
- Vulnerability checking
- GoReleaser validation

## Testing

### Unit Tests

```bash
# Run all unit tests
go test ./...

# With verbose output
go test -v ./...

# With race detection
go test -race ./...

# With coverage
go test -cover ./...
```

### Integration Tests

Integration tests are marked with build tags:

```bash
# Run integration tests
go test -v -tags=integration ./test

# Skip slow tests
go test -v -tags=integration -short ./test
```

### Coverage

```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...

# View coverage in terminal
go tool cover -func=coverage.out

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html
```

## Code Style

### Formatting

Code is formatted with `gofmt`:

```bash
go fmt ./...
```

### Linting

We use golangci-lint with strict configuration:

```bash
golangci-lint run
```

### Documentation

- All exported functions must have Go doc comments
- Comments should explain "why" not "what"
- Keep comments concise and clear

Example:

```go
// GeneratePassword creates a cryptographically secure random password
// with the specified length and character requirements.
func GeneratePassword(length int, opts PasswordOptions) (string, error) {
    // Implementation...
}
```

## Project Structure

```text
pass-cli/
├── cmd/                   # Cobra command definitions
│   ├── root.go           # Root command
│   ├── init.go           # Init command
│   ├── add.go            # Add command
│   └── ...
├── internal/             # Private application code
│   ├── crypto/           # Encryption/decryption
│   ├── storage/          # Vault file operations
│   ├── keychain/         # OS keychain integration
│   ├── vault/            # Vault service (business logic)
│   └── models/           # Data models
├── test/                 # Integration tests
│   └── integration_test.go
├── docs/                 # Documentation
├── .github/              # GitHub Actions workflows
├── main.go               # Application entry point
└── .goreleaser.yml       # Release configuration
```

## Security

### Secure Coding Practices

1. **Never log sensitive data**: Passwords, encryption keys, etc.
2. **Use constant-time operations**: For cryptographic comparisons
3. **Clear sensitive data from memory**: After use
4. **Validate all inputs**: Especially user-provided data
5. **Use secure defaults**: Safe configuration out of the box

### Security Scanning

Run gosec regularly:

```bash
gosec ./...
```

Check for vulnerable dependencies:

```bash
govulncheck ./...
```

## Debugging

### Enable Verbose Logging

```bash
./pass-cli --verbose <command>
```

### Build with Debug Info

```bash
go build -gcflags="all=-N -l" -o pass-cli .
```

### Use Delve Debugger

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug the application
dlv debug . -- init
```

## Performance

### Benchmarking

```bash
# Run benchmarks
go test -bench=. -benchmem ./...

# Profile CPU usage
go test -cpuprofile=cpu.prof -bench=.

# Profile memory
go test -memprofile=mem.prof -bench=.

# Analyze profiles
go tool pprof cpu.prof
go tool pprof mem.prof
```

### Performance Targets

- First vault unlock: <500ms
- Cached operations: <100ms
- Support 1000+ credentials efficiently

## Release Process

See [Release Process](release) for detailed release instructions.

Quick reference:

```bash
# 1. Test everything
go test ./...
go test -v -tags=integration ./test
golangci-lint run
gosec ./...
govulncheck ./...
goreleaser check

# 2. Create tag
git tag -a v1.0.0 -m "Release v1.0.0"

# 3. Push tag (triggers CI/CD)
git push origin v1.0.0

# 4. Monitor GitHub Actions
# 5. Verify release artifacts
```

## Troubleshooting

### Common Issues

**"golangci-lint: command not found"**:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

**Tests failing with keychain errors**:
- Some keychain tests may fail without proper OS configuration
- Integration tests handle this gracefully

**Build fails with module errors**:
```bash
go mod tidy
go mod verify
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes following the development workflow above
4. Run all tests and quality checks (see "Before Committing" section)
5. Submit a pull request

For questions or discussions, visit [GitHub Discussions](https://github.com/reyamira/pass-cli/discussions).

## Getting Help

- **Issues**: [GitHub Issues](https://github.com/reyamira/pass-cli/issues)
- **Discussions**: [GitHub Discussions](https://github.com/reyamira/pass-cli/discussions)
- **Documentation**: [docs/](../_index.md)

## Resources

- [Go Documentation](https://go.dev/doc/)
- [Cobra CLI Framework](https://github.com/spf13/cobra)
- [GoReleaser](https://goreleaser.com/)
- [golangci-lint](https://golangci-lint.run/)
