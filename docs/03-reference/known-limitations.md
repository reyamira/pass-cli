---
title: "Known Limitations"
weight: 4
toc: true
---
Documentation of known technical limitations in Pass-CLI and their security implications.

![Version](https://img.shields.io/github/v/release/reyamira/pass-cli?label=Version) ![Last Updated](https://img.shields.io/github/last-commit/reyamira/pass-cli?path=docs&label=Last%20Updated)


## TUI Password Input Memory Handling

**Issue**: TUI mode uses tview's `InputField` which returns passwords as immutable Go strings.

**Impact**: Passwords entered in TUI mode may remain in memory longer than CLI mode due to string immutability.

### Technical Details

**CLI Mode** (Secure):
```go
// cmd/helpers.go - readPassword() returns []byte
password, err := term.ReadPassword(int(os.Stdin.Fd()))
// password is []byte - can be zeroed immediately after use
defer crypto.ClearBytes(password)
```

**TUI Mode** (Limitation):
```go
// cmd/tui/components/forms.go
password := passwordField.GetText()  // Returns string (immutable)
// Cannot zero string - Go strings are immutable
```

### Why This Happens

1. **tview Architecture**: tview's `InputField` is designed for general text input, not secure password handling
2. **String Immutability**: Go strings cannot be modified in place (by design)
3. **No []byte Alternative**: tview does not provide a `GetTextBytes()` method

### Security Implications

**Risk Level**: **Low to Medium**

- **Mitigation 1**: Passwords are still encrypted at rest with AES-256-GCM
- **Mitigation 2**: Passwords are cleared from vault operations after use (vault layer uses []byte)
- **Mitigation 3**: Go GC will eventually reclaim string memory
- **Mitigation 4**: Modern OS memory protection prevents cross-process access

**Attack Scenarios Requiring This**:
- Attacker has already compromised the process (in which case, attacker can hook ReadPassword anyway)
- Attacker has kernel-level memory access (full system compromise)
- Memory dump during process execution

**What This Does NOT Protect Against**:
- Memory forensics on process heap dump
- Memory scanning tools run with same privileges as pass-cli

### Comparison: CLI vs TUI

| Aspect | CLI Mode | TUI Mode |
|--------|----------|----------|
| **Password Input** | `term.ReadPassword()` → `[]byte` | `InputField.GetText()` → `string` |
| **Memory Clearing** | [OK] Immediate (deferred) | [WARNING] Relies on Go GC |
| **String Conversion** | [ERROR] Never converted | [WARNING] Converted once for vault ops |
| **Security Level** | High | Medium-High |
| **Usability** | Low (no visual feedback) | High (strength indicator, edit, etc.) |

### Recommended Mitigations

**For Users**:
1. **Prefer CLI mode for maximum security**:
   ```bash
   pass-cli init  # CLI mode - most secure
   ```

2. **Use TUI mode for convenience** (still secure for most threat models):
   ```bash
   pass-cli  # TUI mode - visual feedback
   ```

3. **Enable full-disk encryption** (protects against memory dumps on disk)

4. **Lock workstation** when not in use (prevents memory scanning)

**For Developers** (Future Improvements):

1. **Fork tview**: Add `GetTextBytes() []byte` method to `InputField`
2. **Custom Widget**: Implement secure password input widget from scratch
3. **Clear Input Buffer**: Call `SetText("")` immediately after `GetText()`
   - This doesn't zero the original string but limits exposure window
4. **Memory Scrubbing**: Use `runtime.GC()` + `debug.FreeOSMemory()` after password input
   - Not guaranteed to work (Go GC is not deterministic)

### Current Implementation

**What We Do**:
```go
// cmd/tui/components/forms.go:171
password := af.GetFormItem(2).(*tview.InputField).GetText()

// Convert to []byte for vault operations
passwordBytes := []byte(password)
defer crypto.ClearBytes(passwordBytes)

// Note: Original string 'password' cannot be cleared
```

**What We Don't Do**:
- [ERROR] Clear the original string (impossible in Go)
- [ERROR] Avoid string conversion (tview API limitation)
- [ERROR] Use custom input widget (out of scope for current release)

### Test Verification

Run memory tests to verify vault-layer clearing works:

```bash
# Verify vault operations clear []byte passwords
go test -v ./internal/vault -run TestPasswordClearing

# Verify CLI mode clears passwords
go test -v ./tests/security -run TestTerminalInputSecurity
```

**Expected Results**:
- [OK] CLI mode: passwords cleared immediately
- [WARNING] TUI mode: string lingers until GC (documented limitation)

### Related Issues

- Tracked as known limitation (not a bug)
- Low priority for fix (requires tview fork or custom widget)
- Acceptable per Spec Assumption 4: "Memory clearing is best-effort"

### References

- Go Strings: https://go.dev/blog/strings
- tview InputField: https://pkg.go.dev/github.com/rivo/tview#InputField
- Spec Assumption 4: `specs/005-security-hardening-address/spec.md`

## Memory Clearing and Go GC

**Issue**: Go's garbage collector may copy sensitive data before we can zero it.

**Impact**: Password bytes may exist in multiple memory locations temporarily.

### Technical Details

Go's GC is a concurrent, tri-color, mark-and-sweep collector that may:
1. Copy live objects during collection
2. Keep multiple copies temporarily
3. Clear old locations on its own schedule

**What This Means**:
- Even after `ClearBytes(password)`, copies may exist briefly
- GC will eventually clear all copies
- Attacker needs process memory access during brief window

### Mitigation

- Use `defer` to minimize exposure window
- Trust OS memory protection
- Accept as Go language limitation
- Document as Spec Assumption 4

### Risk Assessment

**Risk Level**: **Very Low**

- Requires active process memory scanning
- Window of exposure: milliseconds to seconds
- Mitigated by OS memory protection
- Acceptable trade-off for Go's memory safety

## Performance on Older Hardware

**Issue**: PBKDF2 with 600k iterations may take >1 second on CPUs older than 2015.

**Impact**: Vault unlock slower on older hardware, but still functional.

### Benchmark Data

| CPU Generation | Unlock Time | User Experience |
|----------------|-------------|-----------------|
| 2023+ | 50-100ms | Instant |
| 2018-2022 | 200-500ms | Fast |
| 2015-2017 | 500-1000ms | Acceptable |
| <2015 | 1000-2000ms | Slow but secure |

### Mitigation

**For Users**:
- Keep using 600k iterations (security > performance)
- Upgrade hardware if unlock time unacceptable
- Old vaults with 100k iterations remain compatible

**For Developers**:
- Do NOT lower iteration count (defeats security purpose)
- Document performance expectations clearly
- Consider Argon2id in future releases (better CPU utilization)

### Why We Accept This

Per Spec Assumption 1:
> "Slower machines may take longer for key derivation, but this is acceptable as long as the unlock time remains under 2 seconds on hardware from 2015 onwards."

Security > convenience for password managers.

