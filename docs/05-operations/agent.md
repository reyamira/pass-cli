# Background Agent

`pass-cli agent` unlocks the vault once and holds it in memory, answering read-only
credential lookups over a local socket. With an agent running, `exec`, `export`,
and `inject` need **no master-password prompt and no key derivation** on each call.
When no agent is running, every command transparently falls back to opening and
unlocking the vault directly — the agent is a pure optimization, never a
dependency.

## Running on demand

```sh
pass-cli agent &                                  # unlock once, serve in background
pass-cli exec --set GITHUB_TOKEN=github -- gh repo list   # resolves via the agent
pass-cli agent status                             # unlocked? idle? max-ttl left?
pass-cli lock                                     # lock without stopping the process
pass-cli agent stop                               # lock and exit
```

Flags:

- `--idle 15m` — lock the vault after this much inactivity (`0` = never).
- `--max-ttl 8h` — hard cap on how long the vault stays unlocked, regardless of use.

The agent locks and exits on `SIGINT`/`SIGTERM`, and re-locks (freeing the socket)
when idle/max-TTL elapses.

## Security model

- The agent serves resolved field **values only**. The master password and derived
  key never cross the socket — there is no protocol method that returns them.
- **Socket access control:** the socket directory is `0700` and the socket `0600`.
  On Linux, connections are additionally authorized by peer credential
  (`SO_PEERCRED`): only a process owned by the same user may talk to the agent
  (fail-closed). macOS/Windows peer authorization is planned; until then those
  platforms rely on the `0600` socket permission.
- **Memory hardening (Linux):** the agent disables core dumps and casual
  ptrace/`/proc/<pid>/mem` access (`PR_SET_DUMPABLE=0`) and attempts to lock its
  memory into RAM (`mlockall`) so secrets never reach swap. The memory lock needs
  `CAP_IPC_LOCK` or a raised `RLIMIT_MEMLOCK` — the systemd unit below sets
  `LimitMEMLOCK=infinity` to make it effective.
- **Honest ceiling** (same as ssh-agent/gpg-agent): root, or a same-user process
  that can still obtain ptrace, can read the agent's memory. This is strictly
  better than a long-lived shell environment variable or a plaintext file.

## Socket location

Resolved in order:

1. `$PASS_CLI_AGENT_SOCK` (explicit override)
2. `$XDG_RUNTIME_DIR/pass-cli/agent.sock` (tmpfs; cleared on logout)
3. `~/.pass-cli/agent.sock`

## Snapshot freshness

The agent holds an in-memory snapshot and takes **no exclusive lock**, so other
`pass-cli` commands (`add`, `update`, …) keep working. Before serving, the agent
revalidates against `vault.enc`: if a sibling process wrote it, the agent reloads
so it never serves stale data. If the on-disk vault can no longer be decrypted with
the held key (the master password was rotated elsewhere), the agent **locks** and
fails the request rather than serving stale data. Changes pushed from **another
machine** are not seen until the next unlock.

## Running as a service (optional)

Service managers are non-interactive, so **running the agent as a service requires
keychain unlock** (`pass-cli keychain enable`) — otherwise the agent has no way to
obtain the master password at startup. Templates (not auto-installed):

- **Linux (systemd `--user`):** [`packaging/systemd/pass-cli-agent.service`](../../packaging/systemd/pass-cli-agent.service)
  — sets `LimitMEMLOCK=infinity` so `mlockall` works, and stops at logout.
- **macOS (launchd LaunchAgent):** [`packaging/launchd/com.reyamira.pass-cli.agent.plist`](../../packaging/launchd/com.reyamira.pass-cli.agent.plist)

Windows is not yet supported (the agent uses a unix socket; a named-pipe transport
is planned).
