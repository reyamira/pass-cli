---
name: pass-cli
description: How an AI agent should safely drive the pass-cli password manager (this repo's binary). Use whenever a task needs a stored secret/credential to run a command, when reading from or listing a pass-cli vault, or when you're about to capture a secret with `get`/command substitution — there is a safer path. Covers `exec` (hand a secret to a child process without it ever touching stdout), the safe-by-default `list`, and the leak traps to avoid.
---

# Using pass-cli safely as an agent

pass-cli is a local, offline-first password manager. Its vault is a single
AES-GCM-encrypted file. As an agent you will mostly need it to **give a stored
secret to a command you're about to run** — and the cardinal rule is:

> **Never let a secret value land in the transcript, a log, CI output, or any
> file a tool watches.** Once a secret is in the conversation log on disk it is
> compromised and must be rotated.

The whole point of the commands below is to move a secret from the vault into a
child process **without it passing through stdout, the clipboard, your shell
history, or a `set -x` trace.**

## The decision: how do you need the secret?

| You need to… | Use | Why |
|---|---|---|
| Run a command that reads the secret from its environment | **`pass-cli exec`** | Secret goes straight into the child's env; never on stdout |
| Know *which* services exist | **`pass-cli list -q`** | Bare service names; usernames hidden by default |
| Capture a value into a variable (last resort) | `pass-cli get … --quiet --no-clipboard` | Only if `exec` truly can't express it; see the trap below |

**Default to `exec`.** Reach for `get` only when nothing else works.

## `pass-cli exec` — the safe way to hand a secret to a command

Runs a child command with credentials injected as environment variables. The
value is passed **only** through the child's environment — never a file, the
clipboard, or shell history. pass-cli writes nothing of its own to stdout, and
the child's exit code is propagated unchanged.

```bash
# Explicit mapping (repeatable): --set ENV_NAME=service
pass-cli exec --set GITHUB_TOKEN=github -- gh repo list

# Multiple credentials at once
pass-cli exec --set AWS_ACCESS_KEY_ID=aws-id --set AWS_SECRET_ACCESS_KEY=aws-secret -- aws s3 ls

# Pick a non-password field for ALL mappings with -f/--field
pass-cli exec --set DB_USER=postgres --field username -- ./run-migration.sh

# Per-mapping field with service:field (overrides -f for that one entry)
pass-cli exec --set DB_USER=postgres:username --set DB_PASS=postgres -- ./run-migration.sh

# Convenience form: derive ENV name from the service (openai-api -> OPENAI_API)
pass-cli exec openai-api -- python train.py
```

Key facts:
- **`--`** separates pass-cli's flags from the child command. Everything after
  `--` is the child's argv. Omitting it is an error ("no command to run").
- **`-f/--field`** (default `password`) selects the field for *all* `--set`
  mappings; valid fields: `username, password, category, url, notes, service`.
- **`service:field`** in a `--set` overrides `-f` for that single mapping.
- **Exit code is propagated** — `pass-cli exec … -- sh -c 'exit 7'` exits 7. Safe
  to use in `&&`/`||` chains and CI gates.
- **`exec` is read-only**: it does NOT record field-access usage and does NOT
  trigger a sync push, so calling it in a hot loop won't mutate the vault or
  hit the network on every run.
- **Honest limit:** the value lives in the child's environment — readable via
  `/proc/<pid>/environ` by the same user and inherited by descendants. This is
  the same model as `op run` / `aws-vault exec`: far safer than files,
  clipboards, or history, but it is not process isolation.

## `pass-cli list` — listing is the *safe* step

```bash
pass-cli list            # default table: NO username column (usernames can be sensitive)
pass-cli list -q         # bare service names, one per line — ideal for agents/scripts
pass-cli list --show-usernames   # opt the username column back in (only if you need it)
pass-cli list --format json      # full metadata incl. usernames — explicit, structured opt-in
```

Usernames are **hidden by default** because that field often holds sensitive
values (card/account/routing numbers stored as an entry's "username"). Use
`pass-cli list -q` to discover service names without dumping anything sensitive.

## `pass-cli get` — last resort, and the trap

```bash
pass-cli get github --quiet --no-clipboard --field password
```

`--quiet` prints **only** the field value to stdout; `--no-clipboard` skips the
clipboard. But this still puts the secret on stdout — and the trap is what you
do with it next:

```bash
# ❌ NEVER — the value is now in the transcript / log
echo "$(pass-cli get github --quiet --no-clipboard)"
TOKEN=$(pass-cli get github --quiet --no-clipboard); curl -H "Authorization: $TOKEN" ...
#   ^ if any layer runs `set -x`, or a file-watcher captures the command, the token leaks

# ✅ Prefer exec — the value never becomes a shell variable or a transcript line
pass-cli exec --set TOKEN=github -- curl -H 'Authorization: Bearer '"$TOKEN" ...
#   (the child reads $TOKEN from its own env; pass-cli printed nothing)
```

If you genuinely must capture into a variable (a tool with no env-var path),
pipe it directly into the consumer in the **same** command, never `echo` it, and
never enable shell tracing in that shell.

## Leak-trap checklist (before you run anything)

- Am I about to `echo`/print a secret, or interpolate it into a logged command? → use `exec`.
- Is `set -x` / xtrace active in this shell? → a `get`/substitution will dump the value. Disable it or use `exec`.
- Am I writing the secret into a `.env*` or any file a tool watches? → the harness echoes file changes; don't.
- Do I just need to *see what's stored*? → `pass-cli list -q` (names only).

If a secret value ever does reach the transcript: tell the user immediately so
they can rotate it. The leak cannot be undone.
