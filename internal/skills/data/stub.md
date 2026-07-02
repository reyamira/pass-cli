---
name: pass-cli
description: How an AI agent should safely drive the pass-cli password manager. Use whenever a task needs a stored secret/credential to run a command, when reading from or listing a pass-cli vault, or when you're about to capture a secret with `get` or command substitution — there is a safer path. pass-cli can hand a secret to a child process without it ever touching stdout (exec), materialize composite secrets into config files (inject), and serve promptless resolves from a background agent. Triggers include "run this with my API key", "use my stored token", "inject a secret into this config", "list my credentials", or any task that needs a value out of the pass-cli vault.
allowed-tools: Bash(pass-cli:*)
hidden: true
---

# pass-cli

Local, offline-first password manager for humans and their AI agents. The vault
is a single AES-GCM-encrypted file.

## Start here

This file is a discovery stub, not the usage guide. Before running any
`pass-cli` command that touches a secret, load the actual guide from the CLI:

```bash
pass-cli skills get core          # the safe-usage guide: exec/export/inject/agent/list/get + leak traps
pass-cli skills get core --full   # also include the full command reference
```

The CLI serves skill content that always matches the installed version, so the
guidance never goes stale. The content in this stub cannot change between
releases, which is why it just points at `skills get core`.

## The one rule you must not break

**Never let a secret value land in the transcript, a log, CI output, or any file
a tool watches.** Prefer `pass-cli exec --set ENV=service -- <command>`: the
secret goes straight into the child's environment and never touches stdout, the
clipboard, or your shell history. If a secret ever does reach the transcript,
tell the user immediately so they can rotate it.

## List available skills

```bash
pass-cli skills list              # every skill shipped with the installed version
```
