## Full command reference

Condensed flag reference for the agent-facing commands. `pass-cli <cmd> --help`
always has the authoritative, version-matched detail.

### Mapping grammar (shared by `exec` and `export`)

```
--set ENV_NAME=service[/field][|filter]      # repeatable
```

- `service` — the credential's service name.
- `/field` — one of `username, password, category, url, notes, service`
  (default `password`). Legacy `:field` also accepted; prefer `/`.
- `|filter` — one of `base64`, `base64url`, `basicauth` (quote the `|` in a
  shell). `basicauth` takes no field.
- Convenience positional form: `pass-cli exec service …` derives the ENV name
  from the service (uppercased, non-alphanumerics → `_`). Filters are **not**
  supported on this form.

### `exec` — run a command with injected env

| Flag | Meaning |
|---|---|
| `--set ENV=service[/field][\|filter]` | map an env var to a credential (repeatable) |
| `-f, --field <field>` | field for all `--set` mappings (default `password`) |
| `--env-file <file>` | read `KEY=${pass:svc/field}` template lines (repeatable) |
| `--from <file>` | read `ENV=service/field` from a `.pass-cli.toml` manifest (repeatable) |

Everything after `--` is the child command; its exit code is propagated.
Read-only: no usage tracking, no sync push.

### `export` — print shell statements to eval

| Flag | Meaning |
|---|---|
| `--set`, `-f/--field`, `--from` | same as `exec` |
| `--format <sh\|fish\|powershell>` | shell syntax (default `sh`) |

`sh` → `export NAME='value'` (for `eval "$(…)"`); `fish` → `set -gx NAME 'value'`
(for `… | source`); `powershell` → `$env:NAME = 'value'`.

### `inject` — render a template with `${pass:...}` refs

| Flag | Meaning |
|---|---|
| `-i, --in-file <file>` | template to read (default stdin) |
| `-o, --out-file <file>` | output file, created `0600` (default stdout) |
| `-f, --field <field>` | default field for refs without an explicit `/field` |

Reference forms: `${pass:service}`, `${pass:service/field}`,
`${pass:service/field | filter}`. Unknown/malformed ref → hard error, nothing
written. Piped-stdin templates unlock via keychain only (use `-i` for the
password prompt).

### `agent` — background daemon holding the unlocked vault

| Subcommand | Meaning |
|---|---|
| `agent start` | unlock once, detach into the background |
| `agent serve` | run in the foreground (same as bare `agent`) |
| `agent status` | show status (never prints secrets) |
| `agent stop` | lock the vault and exit |

| Flag | Meaning |
|---|---|
| `--idle <dur>` | lock after this much inactivity (default `15m`, `0` = never) |
| `--max-ttl <dur>` | hard cap on unlocked lifetime (default `8h`, `0` = no cap) |

Socket: `$PASS_CLI_AGENT_SOCK`, else `$XDG_RUNTIME_DIR/pass-cli/agent.sock`, else
`~/.pass-cli/agent.sock`. Serves resolved **values only**. Commands fall back to
direct unlock when no agent is running.

### `list` — discover services

| Flag | Meaning |
|---|---|
| `-q, --quiet` | bare service names, one per line (alias for `--format simple`) |
| `-f, --format <table\|json\|simple>` | output format (default `table`) |
| `--show-usernames` | include the username column (hidden by default) |
| `--unused [--days N]` | only credentials unused for ≥ N days (default 30) |
| `--by-project` | group by git repository |
| `--location <dir> [--recursive]` | filter by access directory |

Usernames hidden by default (they can hold sensitive values). `--format json`
emits full metadata including usernames.

### `get` — retrieve one credential (last resort for raw values)

| Flag | Meaning |
|---|---|
| `-q, --quiet` | output only the requested value (script-friendly) |
| `-f, --field <field>` | field to extract (default `password`) |
| `--no-clipboard` | do not copy to clipboard |
| `--masked` | display password as asterisks |
| `--totp` / `--totp-qr` / `--totp-qr-file <png>` | TOTP code / QR |

Even with `--quiet --no-clipboard` the value is on stdout — see the leak trap in
the core guide. Prefer `exec`.

### `generate` — cryptographically secure password

| Flag | Meaning |
|---|---|
| `-l, --length N` | length (default 20) |
| `--no-lower` / `--no-upper` / `--no-digits` / `--no-symbols` | exclude a class |
| `--no-clipboard` | do not copy to clipboard |

Aliases: `gen`, `pwd`.

### `.pass-cli.toml` manifest (for `--from`)

A committable file — references only, never values:

```toml
[env]
GITHUB_TOKEN = "github"
DB_PASSWORD  = "postgres/password"
API_TOKEN    = "api/token|base64"
```

`pass-cli exec --from .pass-cli.toml -- ./server` or
`eval "$(pass-cli export --from .pass-cli.toml)"`.

### Global flags (all commands)

| Flag | Meaning |
|---|---|
| `--config <file>` | config file (default `$HOME/.pass-cli/config.yaml`) |
| `--offline` | skip cloud sync for this command |
| `-v, --verbose` | verbose output |
