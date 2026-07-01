# Design: Background Agent (#116) + Env-Injection Ergonomics (#115)

> Status: design / plan. Tracks GitHub issues #115 (ergonomics) and #116 (agent/daemon substrate).
> Phase 0 in progress; Phases 1–3 not yet written against.

## Decisions baked in (2026-07-01)

Five steers from the owner, resolved into this plan:

1. **Grammar: slash-delimited path separator, added *additively*.** `/` becomes the preferred separator (`service/field`); the shipped `:` form keeps working unchanged as a back-compat alias — **no deprecation warning, no removal.** The colon isn't broken today (one colon, fixed field set), so this is *forward-consistency* (users learn one separator for `--set`, templates, and the manifest) and `op://` alignment, not a bug fix. This is the "first peg" set in Phase 0 so every later surface (templates §4, socket protocol §5) inherits the same separator. See §3.1 for the exact parse rule.
2. **Broader usage tracking is OK, but never on the sync-speed critical path.** Tracking writes must be *batched / deferred*, never a per-access sync push (that was the #120 slowness). See §5.5.
3. **The daemon holds an in-memory vault snapshot; concurrent CLI operations must still work.** No exclusive file lock — other processes read/write `vault.enc` freely. The daemon runs a *revalidating cache*: it detects an on-disk change via the #102/#120 content-hash marker and refreshes its snapshot rather than serving stale data or locking others out. See §5.8.
4. **memguard: approved.** Confined to the agent package; the one-shot CLI keeps `crypto.ClearBytes`. See §5.6.
5. **This session coordinates the implementation** (may fan out to subagents), building Phase 0 first.

Scope discipline: **only decision #1 touches Phase 0.** #2/#3/#4 are all Phase 2 (the daemon) — recorded here as text, not built in the first pass. If Phase-0 code starts looking daemon-adjacent, that's drift.

## 0. Framing — the substrate is a `Resolver`, not the daemon

The unifying abstraction is **not** "the daemon." It is a single interface that turns the existing `--set ENV=service[:field]` mappings into materialized `NAME=value` strings, with two interchangeable backends:

```go
type Resolver interface {
    // Resolve materializes each mapping into "NAME=value".
    // Read-only: it never records usage and never triggers a sync push.
    Resolve(mappings []envMapping, defaultField string) ([]string, error)
    Close() error
}
```

- **`directResolver`** — today's path: `vault.New` → `unlockVaultWithSync` → `buildEnvEntry` per mapping → `Lock()`. This already exists inside `cmd/exec.go`; Phase 0 extracts it.
- **`socketResolver`** — dials the agent, sends the mappings, receives the values. The agent already holds an unlocked `VaultService`, so there is no per-call PBKDF2 and no prompt.

Every consuming command (`exec`, `export`, `run`/`inject`, and optionally `get`) does the same thing: **try the socket, fall back to direct-open.** This makes the daemon a transparent optimization (satisfying #116's "daemon is not a hard dependency") and directly answers the "what can ship incrementally" question: the #115 surfaces (`export`, `inject`, manifest) ship on `directResolver` **before** the daemon exists, then transparently accelerate once the socket backend lands.

The sequencing is therefore **not** "#116 then #115." It is:

1. **Phase 0** — extract the shared resolver + grammar (no behavior change).
2. **Phase 1** — #115 surfaces on `directResolver` (`export`, `inject`/template, `.pass-cli.toml` manifest).
3. **Phase 2** — #116 agent: daemon process + IPC + `socketResolver`, auto-detected by the Phase-1 surfaces.
4. **Phase 3** — lifecycle hardening (auto-lock, locked memory, peer-cred per platform) and optional platform service units.

---

## 1. Current-state facts the plan is built on

| Concern | Where | Note |
|---|---|---|
| Mapping grammar `ENV=service[:field]` | `cmd/exec.go` `parseExecArgs`, `envMapping`, `deriveEnvName` | Pure parsing, already unit-testable. Move to a shared package. |
| Materialize one field | `cmd/exec.go` `buildEnvEntry` + `cmd/helpers.go` `resolveCredentialField` | `resolveCredentialField` is the single source of truth for field aliases shared by `get`/`exec`. Reuse verbatim. |
| Entry-point unlock | `cmd/helpers.go` `unlockVaultWithSync` | Overlaps sync pull with the password prompt (#103). The daemon calls this **once** at unlock. |
| In-memory unlocked state | `internal/vault/vault.go` `VaultService` (`masterPassword []byte`, `vaultData`, `unlocked`) | This whole object is what the daemon holds resident. `GetCredential` returns a deep copy; `Lock()` zeros secrets via `crypto.ClearBytes`. **Not concurrency-safe for mutation.** |
| Key handling today | `crypto.ClearBytes` zeroing; no `mlock`, no core-dump suppression | New capability for the daemon. |
| Keychain unlock | `vault.RetrieveKeychainPassword` / `UnlockWithKeychain` via `internal/keychain` (zalando/go-keyring) | Reads keyring without decrypting vault.enc. macOS keychain is session-bound (the test HOME trap). |
| `exec` is deliberately read-only | `cmd/exec.go` comment at `runExec` | No `RecordFieldAccess`, no `syncPushAfterCommand`. The resolver must preserve this. |
| No platform-specific files exist | `find` for `*_linux.go` etc. → none | The daemon introduces the build-tag pattern for the first time. |
| Test split | `internal/*_test.go` (no tag, unit) vs `test/integration/*` (`//go:build integration`, drives the built binary via `helpers.RunCmd`) | macOS HOME-override trap guarded by `runtime.GOOS == "darwin"` in `test/helpers/command.go:121-129`. |

---

## 2. Security model (state as invariants, do not oversell)

1. **The daemon serves resolved field values only. It never returns the master password or the derived key over the socket.** This is the ssh-agent property and the core invariant. A compromised client gets the secrets it was already going to inject — nothing more durable.
2. **The materialization ceiling (#115) is unchanged.** A consumer that reads `process.env` forces plaintext to exist at the boundary. The agent changes *who holds the long-lived key and for how long*, not whether plaintext must materialize.
3. **Same ceiling as ssh-agent/gpg-agent, stated per-platform — not a blanket "no protection":**
   - **Linux:** `mlock` (no swap) + `prctl(PR_SET_DUMPABLE, 0)` (no core dump, blocks casual same-user `/proc/<pid>/mem` and ptrace attach by non-root) is real hardening. The honest ceiling: **root, or a same-user process that can still ptrace, can read daemon memory.** Strictly better than a long-lived shell env var or a plaintext `.env`.
   - **macOS:** locked memory via memguard; task_for_pid is gated, but same-user with the right entitlements/SIP-off can read. Document parity with gpg-agent.
   - **Windows:** named-pipe ACL restricts the pipe to the owning SID; process memory is readable by same-user debuggers. Document parity.
4. **Never log values.** The protocol carries secrets; the daemon's logger must be structurally incapable of emitting a value (log mapping *names* and *service* keys only, never resolved bytes). Add a test that greps daemon stderr for a known secret and fails if found.

---

## 3. Phase 0 — extract the shared resolver + the slash "first peg"

Phase 0 ships as **two isolated commits** so the regression gate stays unambiguous — if a test goes red, exactly one change is implicated:

- **0a — pure extraction, zero behavior change.** Move `envMapping`, `parseExecArgs`, `deriveEnvName` from `cmd/exec.go` into a new pure package `internal/envmap` (no I/O). Add `internal/resolver` with the `Resolver` interface and `directResolver` (lifts `buildEnvEntry`'s body, reusing `resolveCredentialField`, preserving read-only semantics). `cmd/exec.go` becomes a thin client: parse → pick a resolver → `Resolve` → `runChild`. The **existing exec integration + unit tests pass unchanged** — that is the 0a gate. No grammar change in this commit.
- **0b — additive slash parsing + new slash tests.** Teach the extracted parser the slash rule (§3.1) *without* touching the colon path. New slash-input tests; the colon tests from 0a stay green as the back-compat proof.

### 3.1 The slash parse rule (decision #1)

Applied uniformly wherever a `service[/field]` spec is parsed (`--set` value, later the manifest and template refs):

- **Separator is picked by presence; slash wins.**
  - If the spec contains `/` → slash mode: split into segments on `/`. Require **exactly two** (`service/field`) for now; **3+ segments error** with "multi-segment paths not yet supported" — this reserves `vault/service/field` (field = last segment, `op://`-compatible) for a future multi-vault without designing vaults today. In slash mode **any `:` is a literal character** — this is the actual fragility fix: colons in service names or composite values stop being separators.
  - Else if the spec contains `:` → legacy mode: **byte-for-byte the current behavior** — `strings.Cut` on the *first* colon, service before, field after. Do **not** "improve" it to last-colon; that would silently change existing colon users.
  - Else (no separator) → whole spec is the service, field falls back to the global `--field` / default.
- **No deprecation warning on colon.** "Alias" means "still works, docs prefer slash." Output is unchanged for colon users.
- Template refs (§4) and the manifest (§4.3) are greenfield → slash-only from day one; `op://vault/item/field` reserved as a later alias (decision #1 keeps op-style on the table for multi-vault).

**Risk:** 0a is a mechanical refactor gated by the existing exec suite. 0b is additive and gated by keeping the colon tests unchanged; no shipped behavior is removed.

---

## 4. Phase 1 — #115 surfaces on direct-open (ship before the daemon)

Keep #115's own ranking: **1/2/4 are real (persistence, ceremony, source-of-truth); 3 (transforms) is a band-aid.**

### 4.1 `pass-cli export` (#115.1) — ranked #1
New `cmd/export.go`. Reuses the mapping grammar. Emits shell-quoted `export NAME='value'` to stdout for `eval`/direnv.
- **Quoting:** single-quote with `'\''` escaping (POSIX). Add `--format {sh,fish,powershell}` (fish: `set -gx NAME value`; pwsh: `$env:NAME='value'`). Default `sh`.
- **direnv:** ship a stdlib snippet `use pass_cli <args>` that wraps `eval "$(pass-cli export ...)"` in docs; no code dependency on direnv.
- **Honest caveat (documented):** `export` materializes into the *current* shell for that shell's lifetime — weaker than `exec`'s child-scoped injection. The doc must say: prefer `exec`/`run` when you only need to launch a command; `export` is the blessed alternative to `VAR="$(pass-cli get …)"`, not to `exec`.

### 4.2 `pass-cli inject` + `pass-cli exec --env-file` (#115.2) — ranked #2 — SHIPPED
A committable template with **references only** (`${pass:service/field}`) is resolved in-memory.
- `inject`: read a template from `--in-file`/stdin, write the rendered text to `--out-file` (0600) or stdout — solves composite/derived secrets like `postgres://user:${pass:db/password}@host`. A `--in-file` template is read before unlock (fail-fast on a bad path); a stdin template is read *after* unlock, because the master-password prompt also reads stdin (so a stdin template implies keychain unlock).
- **`run` folded into `exec --env-file` (owner decision 2026-07-01).** Issue #115 named a `run --env-file` command, but it would duplicate `exec` (both inject env → run child). Instead `--env-file <path>` is an additional env source on `exec`: each `KEY=<template>` line is rendered (`envmap.RenderTemplate`) and injected structurally into the child env (never to disk). Net surface: `exec` (run child) / `export` (shell statements) / `inject` (render to stdout) — three distinct output behaviors, no overlap, no separate `run`.
- **Template engine (`internal/envmap.RenderTemplate`)** is single-pass and fail-closed: all `${pass:...}` refs are collected from the *original* template and resolved in **one batch** call (Phase 2's socket → one round-trip), then substituted; a resolved value containing `${pass:...}` is never re-scanned (injection guard); an unknown/malformed ref aborts the whole render (no partial/silent-empty output). Only `${pass:...}` is special — `$VAR`, `${VAR}`, `$(...)` pass through.

### 4.3 Project manifest `.pass-cli.toml` / `--from <file>` (#115.4) — ranked #4
Names-only map so launchers/`.envrc` don't repeat long `--set` chains:
```toml
[env]
GITHUB_TOKEN = "github:password"
DB_PASSWORD  = "postgres:password"
```
`--from .pass-cli.toml` expands to the same `[]envMapping`. Parser lives in `internal/envmap`. Committable because it contains **references, never values**.

### 4.4 Transforms (#115.3) — band-aid, deferred
Do **not** build a general filter pipeline now. If pressure exists, scope to a single, explicitly-labeled-convenience `:base64` suffix for Basic-auth headers, gated behind a follow-up. Present it as a band-aid, not a peer feature.

### 4.5 Reference syntax — `${pass:service/field}` (slash path, decision #1)
**Native `${pass:service/field}`** — the `pass:` scheme prefix plus the slash path from §3.1. `op://vault/item/field` reserved as a later alias for 1Password migrators.
Why slash and not `${pass:service:field}`: the template is exactly where the colon bites — a scheme colon *plus* a separator colon, nested inside URIs full of their own colons (`postgres://u:${pass:db/password}@h`). Slash makes the inner colons literal (§3.1) and lines up with `op://…/…/…` so a future multi-vault segment prepends cleanly (`${pass:vault/service/field}`). The discriminator still holds — pass-cli binds **one vault per invocation** (`GetVaultPath`), so the leading vault segment stays absent until multi-vault exists; the two-segment `service/field` is the only accepted shape for now (§3.1's "3+ segments error").

---

## 5. Phase 2 — the agent (#116) and the socket backend

### 5.1 Process & commands
New `cmd/agent.go` group:
- `pass-cli agent` — start (foreground by default; `--daemonize` to background). Unlocks once via `unlockVaultWithSync`, then serves.
- `pass-cli agent stop` — graceful shutdown (sends a shutdown request or signals the pid).
- `pass-cli agent status` — running? idle time? max-TTL remaining? (never prints secrets).
- `pass-cli lock` — re-lock the agent's vault without killing the process.

### 5.2 IPC: framed request/response over a local stream
- **Transport:** `net` Unix-domain stream socket on POSIX; **named pipe on Windows** via `github.com/Microsoft/go-winio` (`winio.ListenPipe`). Abstract behind a small `agentconn` package with build-tagged listener/dialer.
- **Wire format:** newline-delimited JSON or length-prefixed JSON frames. JSON is fine — values are short and the channel is local; readability aids the "never log values" audit. **One method matters:** `resolve(mappings, defaultField) -> [{name, value}] | error`. Plus `lock`, `status`, `shutdown`. Explicitly **no** `get-master-password` / `get-key` method exists in the protocol (invariant #1).
- **Protocol versioning:** include a `version` field; the client falls back to direct-open on version mismatch rather than erroring.

### 5.3 Socket location & lifecycle hygiene
- **Linux:** `$XDG_RUNTIME_DIR/pass-cli/agent.sock` (tmpfs, auto-cleared on logout → doubles as logout-lock). Fallback `~/.pass-cli/agent.sock`, dir `0700`, socket `0600`.
- **macOS:** `~/.pass-cli/agent.sock` (or `$TMPDIR`), `0600`.
- **Windows:** `\\.\pipe\pass-cli-<SID>` with an ACL granting only the owning SID.
- **Stale-socket reclaim on startup:** dial the existing path; if connection refused, `unlink` + rebind; if it answers, exit "agent already running." (Classic footgun — make it explicit.)
- **Discovery by clients:** env var `PASS_CLI_AGENT_SOCK` overrides; else the default path. If dialing fails for any reason, fall back to direct-open silently (verbose: note the fallback on stderr).

### 5.4 Auth: peer-credential checks in build-tagged files
New files (introducing the platform-file pattern this repo lacks today):
- `agentconn/peercred_linux.go` — `SO_PEERCRED` (`unix.GetsockoptUcred`), assert `uid == os.Getuid()`.
- `agentconn/peercred_darwin.go` — `getpeereid` (via `golang.org/x/sys/unix`), assert uid.
- `agentconn/peercred_windows.go` — `GetNamedPipeClientProcessId` + token/SID comparison; the primary control is the pipe ACL. **Verify the exact go-winio / `x/sys/windows` API at implementation time** (go-winio was not resolvable in the docs index during planning; confirm it surfaces client PID/SID).
Reject any peer whose uid/SID ≠ the agent owner, **before** processing the request.

### 5.5 Concurrency: mutex-guarded, read-only resolve
`VaultService` is not safe for concurrent mutation. The agent serves N shells:
- Wrap the resident `VaultService` in a `sync.Mutex` (or RWMutex with resolve under RLock).
- Resolve path stays **read-only** (no `RecordFieldAccess`, no save) — same discipline as `exec` today, now enforced for all clients.
- **Usage tracking must never sit on the sync-speed critical path (decision #2).** Today `get` writes a usage timestamp → changes the vault hash → triggers a sync push → the exact slowness #120 fought. The owner wants *broader* tracking without paying that cost, so tracking is **batched/deferred, never a per-access push**:
  - **Daemon path (natural fit):** the resident agent accumulates `(service, field, ts)` access events in memory and flushes them into the vault **coalesced** — on idle, on `lock`, or on a bounded interval — as a single write+push, not one per access. Add an explicit fire-and-forget `track(service, field)` RPC (client does not block on it). The socket path can therefore keep tracking *and* stay fast, so `get`-over-socket is viable once the daemon exists.
  - **One-shot CLI path:** tracking stays local and lazy — record the access, but let the existing TTL-gate / dirty-flag (#120) defer or coalesce the push rather than forcing a round-trip per `get`. Never re-introduce a synchronous push on the hot path.
  - **First cut, no daemon:** `get` stays direct-open with today's (already deferred) local write. The `track` RPC and cross-command batching land with the daemon.

### 5.6 Key handling in memory
- **Decided (decision #4): memguard**, confined to the agent package. Hold the derived key / master password as an `Enclave` at rest (encrypted, not mlock-limited), `Open()` into a `LockedBuffer` only for the duration of a resolve, then `Destroy()`. memguard already does mlock + core-dump handling + a catch-signals/purge path — correctness of mlock/guard-pages by hand is easy to get wrong, so we take the dependency here.
- **Confinement:** the one-shot CLI keeps its existing `crypto.ClearBytes` path unchanged; memguard touches only the agent package, so the rest of the binary and its footprint are unaffected. Reshaping how the daemon's resident `VaultService` stores `masterPassword` is expected and scoped to that package.

### 5.7 Auto-lock & lifecycle
- **Idle timeout** (`--idle 15m`, configurable): reset on each resolve; on expiry call `VaultService.Lock()` but keep the process alive, re-prompt/keychain-unlock on next request — or refuse and require `pass-cli agent` re-unlock (recommend: refuse and require explicit re-unlock; auto-reprompt from a daemon is awkward without a controlling TTY).
- **Max-TTL re-lock** (`--max-ttl 8h`): hard cap regardless of activity.
- **Signals:** SIGTERM/SIGINT → lock + clean socket unlink + exit. Trap so secrets are zeroed on shutdown.
- **Suspend/logout:** rely on `$XDG_RUNTIME_DIR` clearing (Linux) and idle/max-TTL elsewhere; document that suspend does not auto-lock (same as ssh-agent) — set a short idle timeout if that matters.
- **Multi-shell:** one agent per user session serves all shells via the shared socket. Document that re-running `pass-cli agent` attaches to the existing one rather than starting a second.

### 5.8 Daemon snapshot + revalidating cache (decision #3)
The daemon holds an **in-memory vault snapshot** and must **not** take an exclusive lock on `vault.enc` — concurrent CLI processes (`add`/`update`/`get` from another shell) keep reading and writing the file freely. That means two independent staleness axes:

- **vs. the local file** (a sibling CLI process wrote the vault): the daemon runs a **revalidating cache**. Before serving a resolve, it cheaply checks whether `vault.enc` changed on disk since the snapshot was taken — reuse the #102/#120 content-hash marker (or mtime/size fallback), the same machinery `SmartPull` already reads, so it's a local stat, not a network call. On change: **refresh the snapshot** (re-read + re-decrypt with the resident key) rather than serving stale bytes. This is what keeps "other operations continue" coherent — the daemon yields to on-disk writers instead of fighting them.
- **vs. the remote** (someone pushed from another machine): the daemon pulls at unlock and serves that snapshot; remote changes are not seen until the next unlock/`lock`+unlock. Optional TTL-bounded background re-pull (`--sync-refresh 5m`) later. The #103 overlap code is one-shot-command-shaped and must not be assumed to keep a daemon fresh.

**Write coherence:** if the batched usage tracking (§5.5) flushes while a sibling CLI process has rewritten the vault, the daemon must re-read-modify-write against the current on-disk file (the revalidate step above), never blind-overwrite from a stale snapshot — same conflict-safety discipline as the sync layer.

### 5.9 Keychain unlock from the daemon (macOS trap)
The agent unlocks via master-password prompt **or** `UnlockWithKeychain`. On macOS, keychain access is bound to the user session — so the daemon must run in the real session with real `HOME`/`USER`/`TMPDIR`. This is exactly the constraint the test helper already encodes; the daemon's spawn path (and its tests) must not fake `HOME` on darwin.

---

## 6. Phase 3 — optional platform service

Pure run-on-demand first (user runs `pass-cli agent`). Then optionally ship templates (not auto-installed):
- **Linux:** `systemd --user` unit + socket activation (`pass-cli-agent.socket` → tmpfs path; systemd hands the listener fd, which also gives clean logout teardown).
- **macOS:** a `launchd` LaunchAgent plist.
- **Windows:** a user-session service or scheduled-task-at-logon wrapper.

**Recommendation:** ship run-on-demand + documented unit templates; do not auto-register a service (surprising for a security tool). Socket activation is a nice-to-have, not Phase-2 critical.

---

## 7. Testing strategy (fit the unit/integration split)

### 7.1 Unit (`internal/`, no build tag) — most logic lives here
- `internal/envmap`: `parseExecArgs`, `deriveEnvName`, `.pass-cli.toml` parsing, `${pass:service:field}` template parsing incl. malformed/escaped/composite cases.
- `internal/resolver`: `directResolver` against an in-memory/temp vault; assert read-only (no usage write, no sync push).
- Agent internals with **no live socket**: auto-lock timer with an **injected clock** (no real sleeps), max-TTL, peer-cred parsing (table-driven over fake ucred/SID), protocol marshal/unmarshal round-trips, and a "logger never emits a known secret" test. Keep the listener thin so the bulk is testable without binding a socket.

### 7.2 Integration (`test/integration/`, `//go:build integration`, drives the built binary)
New `agent_test.go` + `export_test.go` + `inject_test.go`:
- Add an agent spawn helper modeled on `helpers.RunCmd` that starts `pass-cli agent` as a background process on a temp socket + temp config, waits for the socket, runs `exec`/`export` against it, and asserts (a) the socket was actually used (e.g., agent log/status shows a resolve, or kill-the-agent-and-confirm-fallback), and (b) **fallback to direct-open when the agent is absent** produces identical output.
- **macOS guard:** the spawn helper must pass `HOME`/`USER`/`TMPDIR` through on darwin exactly like `RunCmdWithEnv` (`command.go:121-129`); keychain-via-daemon tests use the `runtime.GOOS != "darwin"` guard before any fake-HOME setup.
- **Windows:** named-pipe path needs its own `runtime.GOOS == "windows"`-targeted tests; the existing POSIX-sh tests already `t.Skip` on Windows — mirror that for the socket tests and add pipe-specific coverage.
- `export`/`inject` integration tests can run **without** the agent first (direct-open), proving the incremental-ship claim, then re-run with the agent up.

---

## 8. Risks, dependencies, sequencing

- **Phase 0 is a pure refactor** gated by the existing exec suite — low risk, do it first.
- **#115 ships independently of #116** on `directResolver`. `export` could ship in the very next release with the documented "current-shell lifetime" caveat.
- **Concurrency:** `VaultService` mutation is not thread-safe; the mutex + read-only-resolve discipline is mandatory, and `get`-over-socket's tracking divergence must be a conscious decision (recommend: `get` stays direct-open).
- **Sync staleness** is the only behavioral regression versus today's per-command pull — document, optionally TTL-refresh.
- **Cross-platform peer-cred** introduces the repo's first build-tagged platform files; budget for three implementations + the Windows ACL. Verify go-winio's client-identity API at implementation time.
- **memguard** is a new dependency confined to the agent; keep `crypto.ClearBytes` for one-shot commands so the CLI surface is unchanged.
- **macOS keychain session binding** constrains both the daemon runtime and its tests (no fake HOME on darwin).

---

## Top 5 design recommendations

1. **Make a `Resolver` interface the substrate, not the daemon.** Both backends implement it; all injection commands try-socket-then-fall-back. This makes #115 shippable before #116 exists and the daemon a true optional optimization.
2. **The agent serves resolved field values only — never the master password or derived key over the wire.** State this as the core security invariant; the protocol has no key-export method by construction. Honest ceiling = same as ssh-agent (root/ptrace can read memory), with `mlock`+`PR_SET_DUMPABLE=0` as real, per-platform hardening — not a blanket claim.
3. **Slash is the "first peg" (decision #1): `service/field` native, `:` kept as a silent back-compat alias, `op://` reserved for future multi-vault.** Set it in Phase 0 (additively, no breakage) so `--set`, the manifest, and `${pass:service/field}` templates all share one separator; slash makes inner colons literal, which is the real fragility fix.
4. **Guard the resident vault with a mutex and keep the resolve path read-only, but batch usage tracking off the critical path (decision #2).** `VaultService` isn't concurrency-safe; preserve `exec`'s no-push discipline. Tracking is a fire-and-forget `track` RPC coalesced into one deferred write — never a per-access push — so `get`-over-socket keeps tracking *and* stays fast once the daemon exists; first cut keeps `get` direct-open.
5. **Daemon = revalidating cache, not a lock holder (decision #3).** It serves an in-memory snapshot, takes no exclusive file lock, and refreshes from disk via the #102/#120 content-hash marker when a sibling CLI process writes — so concurrent operations continue. Remote staleness is the one deliberate semantic change (snapshot from unlock-time; optional TTL re-pull). Respect the macOS keychain session-binding trap in daemon runtime + spawn-helper tests (no fake `HOME` on darwin), with Windows named-pipe + ACL peer-auth as a separate path. memguard (decision #4) confined to the agent package.

---

## Critical files for implementation

- `cmd/exec.go` — grammar `parseExecArgs`/`envMapping`/`buildEnvEntry` to extract; becomes a thin resolver client.
- `cmd/helpers.go` — `resolveCredentialField`, `unlockVaultWithSync` — reused by both resolver backends and the daemon's one-time unlock.
- `internal/vault/vault.go` — `VaultService` is the resident unlocked state; `Lock`, `GetCredential`, keychain unlock — needs mutex + memguard-backed key.
- `internal/keychain/keychain.go` — daemon keychain unlock; macOS session-binding constraint.
- `test/helpers/command.go` — the `runtime.GOOS == "darwin"` HOME-passthrough pattern the agent spawn helper must follow.
