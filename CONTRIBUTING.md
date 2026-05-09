# Contributing to Pass-CLI
Thank you for your interest in contributing to Pass-CLI! This document provides guidelines for contributing to the project.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?label=Last%20Updated)


## Documentation Governance

Pass-CLI maintains a [Documentation Lifecycle Policy](docs/DOCUMENTATION_LIFECYCLE.md) that defines retention periods, archival triggers, and decision workflows for all repository documentation. Contributors should consult the policy when adding new documentation or proposing changes to existing docs. The policy ensures documentation remains current and maintainable while preserving historical design context.

## How to Contribute

Contributions are welcome in the following areas:
- Bug reports and feature requests via GitHub Issues
- Code contributions via Pull Requests
- Documentation improvements
- Testing and quality assurance

## Pull Request Process

1. Fork the repository and create a feature branch
2. Make your changes following the project's coding standards
3. Add tests for any new functionality
4. Ensure all tests pass before submitting
5. Submit a pull request with a clear description of your changes

## Code Standards

- Follow Go best practices and idioms
- Write clear, concise commit messages
- Add documentation for new features
- Maintain backward compatibility where possible

## Testing

All code contributions should include appropriate tests. Run the test suite before submitting:

```bash
go test ./...
```

## Documentation Verification

Pass-CLI maintains accurate, trustworthy documentation through systematic verification workflows. When documentation discrepancies are identified (e.g., documented flags that don't exist, broken examples), we follow a structured audit and remediation process.

### Verification Workflow

The documentation verification process includes:

1. **Audit Scope**: Identify documentation files to verify (README.md, docs/, code examples, configuration)
2. **Verification Execution**: Test documented commands, flags, examples, and features against actual implementation
3. **Discrepancy Tracking**: Document all issues in structured audit reports with severity levels and remediation plans
4. **Remediation**: Fix documentation (not code) to match actual implementation
5. **Success Criteria Validation**: Verify 100% accuracy across all verification categories

### Detailed Test Procedures

For complete verification methodology, category definitions, and detailed test procedures, see:

- **Verification Procedures**: [specs/010-documentation-accuracy-verification/verification-procedures.md](specs/010-documentation-accuracy-verification/verification-procedures.md)
- **Example Audit Report**: [specs/010-documentation-accuracy-verification/audit-report.md](specs/010-documentation-accuracy-verification/audit-report.md)

The verification-procedures.md document covers all 10 verification categories:
- CLI Interface (commands, flags, aliases)
- Code Examples (bash/PowerShell execution)
- File Paths (config, vault, audit log paths)
- Configuration (YAML examples, validation)
- Feature Claims (documented features vs implementation)
- Architecture (package structure, crypto descriptions)
- Metadata (version numbers, dates)
- Output Examples (command output format)
- Cross-References (internal markdown links)
- Behavioral Descriptions (keyboard shortcuts, workflows)

When contributing documentation changes, consider running verification tests for affected categories to ensure accuracy.

## Questions?

If you have questions about contributing, please open a GitHub Issue for discussion.
