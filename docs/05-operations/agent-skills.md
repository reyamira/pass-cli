---
title: "AI Agent Skills"
weight: 4
toc: true
---

pass-cli ships an **agent usage guide inside the binary**. Any AI agent (Claude
Code, Cursor, Codex, …) driving pass-cli can load version-matched guidance
straight from the CLI, so the instructions never drift from the installed
version.

## For agents: start here

```sh
pass-cli skills get core          # safe-usage guide: exec/export/inject/agent/list/get + leak traps
pass-cli skills get core --full   # also include the full command reference
pass-cli skills list              # list every skill shipped with this version
```

`skills get core` is the entry point. It explains the one rule that matters —
**never let a secret value land in the transcript, a log, or a watched file** —
and which command to reach for (`exec` by default; `inject` for composite
secrets; `list -q` to discover services; `get` only as a last resort).

## Installing the discovery stub

`skills install` drops a small skill file into your agent's skills directory so
the agent discovers pass-cli automatically and is pointed at `skills get core`:

```sh
pass-cli skills install                 # auto-detects ~/.claude/skills or ~/.agents/skills
pass-cli skills install --dir <path>    # choose a specific directory
pass-cli skills install --force         # overwrite an existing, differing stub
```

The stub is intentionally thin — the real, version-matched guidance stays in the
binary. Re-running `skills install` after upgrading pass-cli is safe (it's a
no-op when the stub is already current).

## Why embedded, not a static doc

The skill content is compiled into the binary via `//go:embed`, so it's always
in lockstep with the commands and flags that binary actually has. A stale copy
pasted into a project's docs or a user's dotfiles can't drift out of sync,
because agents read the guide from the CLI itself.
