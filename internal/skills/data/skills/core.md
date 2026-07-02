---
name: core
description: Core guide for driving pass-cli safely as an AI agent. Read this before running any pass-cli command that touches a secret. Covers the decision between exec/export/inject/list/get, the env-injection grammar (--set, --field, service/field, value filters base64/base64url/basicauth, --env-file, --from manifest), the background agent daemon for promptless resolves, and the leak traps to avoid so a secret never lands in the transcript, logs, or a watched file.
---

# Using pass-cli safely as an agent

pass-cli is a local, offline-first password manager. Its vault is a single
AES-GCM-encrypted file. As an agent you will mostly need it to **give a stored
secret to a command you're about to run** — and the cardinal rule is:

> **Never let a secret value land in the transcript, a log, CI output, or any
> file a tool watches.** Once a secret is in the conversation log on disk it is
> compromised and must be rotated.

Every command below exists to move a secret from the vault into a consumer
**without it passing through stdout, the clipboard, your shell history, or a
`set -x` trace.**

## The decision: how do you need the secret?

| You need to… | Use | Why |
|---|---|---|
| Run a command that reads the secret from its environment | **`exec`** | Secret goes straight into the child's env; scoped to that one process; nothing on stdout |
| Load a secret into your *current* shell (last resort for env) | `export` | Weaker than `exec` — the whole shell + its children can read it |
| Build a config file / connection string that *embeds* a secret | **`inject`** | Templating: `${pass:svc/field}` references get replaced; the only tool for composite/derived secrets |
| Know *which* services exist | **`list -q`** | Bare service names; usernames hidden by default |
| Capture a raw value into a variable (last resort) | `get --quiet --no-clipboard` | Only when nothing else can express it; see the trap below |

**Default to `exec`.** Reach for `export`/`inject` only when `exec` can't express
the shape, and `get` only when nothing else works.

If an agent will do **many** lookups in a session, start the **agent daemon**
once (see below) so every `exec`/`export`/`inject` resolves with no
master-password prompt.

## `exec` — hand a secret to a command (the default)

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

# Per-mapping field with service/field (overrides -f for that one entry)
pass-cli exec --set DB_USER=postgres/username --set DB_PASSWORD=postgres/password -- ./run.sh

# Convenience form: derive ENV name from the service (openai-api -> OPENAI_API)
pass-cli exec openai-api -- python train.py
```

Key facts:
- **`--`** separates pass-cli's flags from the child command. Everything after
  `--` is the child's argv. Omitting it is an error ("no command to run").
- **`-f/--field`** (default `password`) selects the field for *all* `--set`
  mappings. Valid fields: `username, password, category, url, notes, service`.
- **`service/field`** in a `--set` overrides `-f` for that single mapping. The
  legacy `service:field` separator still works, but prefer `/` — in slash form
  any `:` in the service name is a literal character.
- **Exit code is propagated** — `pass-cli exec … -- sh -c 'exit 7'` exits 7.
  Safe in `&&`/`||` chains and CI gates.
- **`exec` is read-only**: it does NOT record field-access usage and does NOT
  trigger a sync push, so calling it in a hot loop won't mutate the vault or hit
  the network on every run.
- **Two more mapping sources** for composite/committed setups (compose with
  `--set`):
  - `--env-file <file>` — lines of `KEY=<template>` whose values may embed
    `${pass:service/field}` references (same templating as `inject`). Resolves
    values a plain `ENV=service` mapping can't, e.g.
    `DATABASE_URL=postgres://app:${pass:db/password}@localhost/app`.
  - `--from .pass-cli.toml` — a committable manifest whose `[env]` table maps
    `ENV_NAME = "service/field"` (references only, never values), so a launcher
    or `.envrc` needn't repeat long `--set` chains.
- **Honest limit:** the value lives in the child's environment — readable via
  `/proc/<pid>/environ` by the same user and inherited by descendants. This is
  the same model as `op run` / `aws-vault exec`: far safer than files,
  clipboards, or history, but it is not process isolation.

## `export` — load secrets into the current shell (weaker than `exec`)

Prints shell statements for your shell to evaluate. Same mapping grammar as
`exec` (`--set`, `-f`, `service/field`, `--from`).

```bash
eval "$(pass-cli export --set GITHUB_TOKEN=github)"     # sh/bash/zsh
pass-cli export --set GITHUB_TOKEN=github --format fish | source
pass-cli export --format powershell --set GITHUB_TOKEN=github | iex
```

`--format` is `sh` (default), `fish`, or `powershell`. **Prefer `exec`:**
`export` materializes the secret into your current shell for its whole lifetime
(and every process it launches can read it). Use `export` as the blessed
replacement for `VAR="$(pass-cli get …)"`, *not* as a replacement for `exec`.

## `inject` — materialize secrets embedded in text (composite secrets)

Reads a template and writes it back with every `${pass:service/field}` reference
replaced by the credential's value. This is the tool for a whole config file or
a single connection string — anything where the secret is *embedded* in other
text.

```bash
# A connection string with an embedded secret
echo 'postgres://app:${pass:db/password}@localhost/app' | pass-cli inject

# A whole config file (references only in the committed template)
pass-cli inject -i config.tmpl -o config.ini
```

Reference syntax:
- `${pass:service}` — the service's default field (see `-f/--field`).
- `${pass:service/field}` — a specific field.
- `${pass:service/field | filter}` — apply a **value filter** (see below).

Only `${pass:...}` is special; `$VAR`, `${VAR}`, and `$(...)` pass through
untouched. An unknown or malformed reference — including an unknown filter — is
a **hard error and nothing is written** (fail-closed).

Caveats:
- **Piped-on-stdin templates unlock via the OS keychain only** — the master-
  password prompt also reads stdin, so it would consume the piped template. Use
  `-i/--in-file` if you need the interactive password prompt.
- **The rendered output is plaintext secrets.** `-o/--out-file` is created
  `0600`; writing to stdout puts the secret on your terminal/pipe. Prefer `exec`
  when you can; use `inject` for the composite/derived cases `exec` can't express.

### Value filters (base64 / base64url / basicauth)

A reference (or a `--set`/manifest mapping) can carry one trailing `| filter`.
Filters transform the value *before* it's injected, so you never have to pipe a
secret through an external `base64` (which would put it on stdout):

- `base64` — standard base64 of the value (e.g. a Bearer token).
- `base64url` — URL-safe base64.
- `basicauth` — `base64("username:password")` from a single credential (takes
  no field; it uses that credential's own username + password).

```bash
# Authorization: Basic header from one credential's user:password
echo 'Authorization: Basic ${pass:api | basicauth}' | pass-cli inject

# base64 a token in a --set mapping (quote the '|' so the shell doesn't pipe)
pass-cli exec --set 'TOKEN=api/token|base64' -- ./server
```

Filters work on `--set`, the `--from` manifest, and `${pass:...}` templates.
They are **not** supported on the positional convenience form
(`pass-cli exec service …`). One filter per reference; case-sensitive
(`base64`, not `BASE64`).

## `agent` — unlock once, resolve promptlessly (for busy sessions)

The background agent unlocks the vault once and holds it in memory, then answers
read-only lookups over a local socket so `exec`/`export`/`inject` need **no
master-password prompt and no key derivation** on each call.

```bash
pass-cli agent start                 # unlock once, detach into the background
pass-cli exec --set T=github -- gh repo list   # resolves via the agent, no prompt
pass-cli agent status                # never prints secrets
pass-cli agent stop                  # locks the vault and exits
```

- Serves resolved field **values only** — the master password and derived key
  never leave the agent process.
- Auto-locks after `--idle` inactivity (default 15m) and always after
  `--max-ttl` (default 8h); locks + exits on SIGINT/SIGTERM.
- **Transparent fallback:** when no agent is running, every command opens and
  unlocks the vault directly, so scripts work either way.

Start the agent at the top of a session where you'll resolve many credentials;
otherwise the direct-unlock path is fine.

## `list` — listing is the *safe* step

```bash
pass-cli list            # default table: NO username column (usernames can be sensitive)
pass-cli list -q         # bare service names, one per line — ideal for agents/scripts
pass-cli list --show-usernames   # opt the username column back in (only if you need it)
pass-cli list --format json      # full metadata incl. usernames — explicit, structured opt-in
```

Usernames are **hidden by default** because that field often holds sensitive
values (card/account/routing numbers stored as an entry's "username"). Use
`pass-cli list -q` to discover service names without dumping anything sensitive.

## `get` — last resort, and the trap

```bash
pass-cli get github --quiet --no-clipboard --field password
```

`--quiet` prints **only** the field value to stdout; `--no-clipboard` skips the
clipboard. But this still puts the secret on stdout — and the trap is what you do
with it next:

```bash
# ❌ NEVER — the value is now in the transcript / log
echo "$(pass-cli get github --quiet --no-clipboard)"
TOKEN=$(pass-cli get github --quiet --no-clipboard); curl -H "Authorization: $TOKEN" ...
#   ^ if any layer runs `set -x`, or a file-watcher captures the command, the token leaks

# ✅ Prefer exec — the value never becomes a shell variable or a transcript line
pass-cli exec --set TOKEN=github -- sh -c 'curl -H "Authorization: Bearer $TOKEN" ...'
#   (the child reads $TOKEN from its own env; pass-cli printed nothing)
```

If you genuinely must capture into a variable (a tool with no env-var path),
pipe it directly into the consumer in the **same** command, never `echo` it, and
never enable shell tracing in that shell.

## Leak-trap checklist (before you run anything)

- Am I about to `echo`/print a secret, or interpolate it into a logged command?
  → use `exec`.
- Is `set -x` / xtrace active in this shell? → a `get`/substitution will dump the
  value. Disable it or use `exec`.
- Am I writing the secret into a `.env*` or any file a tool watches? → the
  harness echoes file changes; don't. Use `exec`, or `inject -o` to a `0600`
  file the harness doesn't watch.
- Do I just need to *see what's stored*? → `pass-cli list -q` (names only).

If a secret value ever does reach the transcript: tell the user immediately so
they can rotate it. The leak cannot be undone.

---

Run `pass-cli skills get core --full` for a condensed flag reference of every
agent-facing command, the mapping grammar, and the `.pass-cli.toml` manifest
format.
