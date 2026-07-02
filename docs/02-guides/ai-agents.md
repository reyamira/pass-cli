---
title: "AI Agent Integration"
weight: 6
toc: true
---

pass-cli is built to be driven safely by **AI coding agents** (Claude Code,
Cursor, Codex, Continue, and similar) — not just humans at a keyboard. An agent
can use your real credentials to run real commands **without the secret ever
landing in the chat transcript, a log, CI output, or a file the agent's harness
watches**.

This is the differentiator: most ways of handing a secret to an agent — pasting
it, `echo`-ing it, capturing it into a shell variable — leak it into the
conversation log the moment they happen. pass-cli exists to move a secret from
your encrypted vault into the process that needs it through a channel the
transcript never sees.

## The safety model

> **Never let a secret value reach the transcript, a log, or a watched file.**
> Once a secret is in the conversation log on disk it is compromised and must be
> rotated.

Everything below is a way to satisfy that rule. The default — `exec` — passes
the secret **only** through a child process's environment, so it never touches
stdout, the clipboard, or shell history.

## Hand a secret to a command — `exec`

The primary tool. Runs a command with credentials injected as environment
variables; pass-cli writes nothing of its own to stdout, and the child's exit
code is propagated unchanged.

```bash
# Explicit mapping: ENV_NAME=service
pass-cli exec --set GITHUB_TOKEN=github -- gh repo list

# Multiple credentials at once
pass-cli exec --set AWS_ACCESS_KEY_ID=aws-id --set AWS_SECRET_ACCESS_KEY=aws-secret -- aws s3 ls

# Convenience form: derive the env name from the service (openai-api -> OPENAI_API)
pass-cli exec openai-api -- python train.py
```

`exec` is read-only: it records no usage and triggers no sync push, so it is safe
to call on a hot path.

## Composite and derived secrets — `export` and `inject`

- **`export`** prints shell statements to load credentials into the current
  shell (`eval "$(pass-cli export --set GITHUB_TOKEN=github)"`). A weaker
  boundary than `exec` — prefer `exec` when you only need to launch a command.
- **`inject`** renders a template, replacing `${pass:service/field}` references
  with values — the tool for a config file or a connection string that *embeds* a
  secret:

  ```bash
  echo 'postgres://app:${pass:db/password}@localhost/app' | pass-cli inject
  ```

  References support value filters — `base64`, `base64url`, and `basicauth`
  (`base64("user:pass")` for an HTTP `Authorization: Basic` header) — so an agent
  never has to shell out to `base64` and risk the value on stdout:

  ```bash
  echo 'Authorization: Basic ${pass:api | basicauth}' | pass-cli inject
  ```

## Promptless access — the background agent

For a session where an agent resolves many credentials, the optional background
agent unlocks the vault once and holds it in memory, so `exec`/`export`/`inject`
need **no master-password prompt and no key derivation** on each call:

```bash
pass-cli agent start                              # unlock once, then background itself
pass-cli exec --set GITHUB_TOKEN=github -- gh repo list   # resolves via the agent, no prompt
pass-cli agent stop                               # zero secrets and stop
```

It serves resolved field **values only** — the master password and derived key
never leave the agent process — and auto-locks after inactivity (`--idle`,
default 15m) and always after `--max-ttl` (default 8h). When no agent is running,
every command **transparently falls back** to opening the vault directly, so the
agent is a pure optimization, never a dependency. POSIX only for now; on Windows
commands fall back to direct-open. See the [Background Agent](../05-operations/agent)
reference for details.

## Self-describing: `pass-cli skills`

pass-cli ships an **agent usage guide inside the binary**, so an agent can load
version-matched guidance straight from the CLI — the instructions never drift
from the installed version:

```bash
pass-cli skills get core          # safe-usage guide: exec/export/inject/agent/list/get + leak traps
pass-cli skills get core --full   # also include the full command reference
pass-cli skills list              # list every skill shipped with this version
```

To make an agent discover pass-cli automatically, install a small discovery stub
into its skills directory:

```bash
pass-cli skills install           # auto-detects ~/.claude/skills or ~/.agents/skills
```

The stub is intentionally thin — it points the agent at `pass-cli skills get
core`, and the real, version-matched guidance stays in the binary. See
[AI Agent Skills](../05-operations/agent-skills) for the full command.

## Quickstart for agents

An agent working on a user's machine can bootstrap in three commands:

```bash
pass-cli skills get core                          # learn the safe-usage rules
pass-cli list -q                                  # discover available service names (no secrets)
pass-cli exec --set API_KEY=some-service -- <cmd> # run a command with the secret injected
```

## Learn more

- [Command Reference](../03-reference/command-reference) — every command and flag
- [Background Agent](../05-operations/agent) — the promptless daemon in depth
- [AI Agent Skills](../05-operations/agent-skills) — the `skills` command
- [Scripting Guide](scripting-guide) — non-agent automation and output modes
