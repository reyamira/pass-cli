---
title: "Scripting Guide"
weight: 6
toc: true
---

Automate pass-cli with scripts using quiet mode, JSON output, and environment variable integration.

> **Using an AI coding agent?** See the [AI Agent Integration](ai-agents) guide for the safe way to hand secrets to agents — injected into a command's environment, never through the chat transcript.

## Output Modes

Pass-CLI supports multiple output modes for different use cases.

### Human-Readable (Default)

Formatted tables and colored output for terminal viewing.

```bash
pass-cli get github
# Service:  github
# Username: user@example.com
# Password: ****** (or full password)
```

### Quiet Mode

Single-line output, perfect for scripts.

```bash
pass-cli get github --quiet
# mySecretPassword123!

pass-cli get github --field username --quiet
# user@example.com
```

### Simple Mode (List Only)

Service names only, one per line. The `-q/--quiet` flag is a shorthand for
`--format simple` and takes precedence over `--format`.

```bash
pass-cli list -q
# github
# aws-prod
# database

# Equivalent, explicit form
pass-cli list --format simple
```

**Username safety:** the default `list` table **hides the Username column**, because the
"username" field can hold sensitive values (card, account, or routing numbers stored as a
username). Pass `--show-usernames` to add the column back:

```bash
pass-cli list --show-usernames
```

Note that `--format json` is an explicit, structured opt-in and still emits the full
metadata, **including usernames**:

```bash
pass-cli list --format json
```

## Inject Credentials with `exec` (Recommended)

When the goal is to hand a credential to a child command, prefer `pass-cli exec`.
It injects the secret as an **environment variable** in the child process only - the
value never touches a file, the clipboard, or your shell history, and `pass-cli`
writes nothing of its own to stdout. This avoids the leak surface of command
substitution (covered [below](#capturing-values-with-command-substitution-fallback)),
where any layer that echoes the command - `set -x`, CI job logs, file-watching tools -
can capture the value.

`exec` is read-only: it does not record usage or trigger a sync push, so it is safe to
call repeatedly on a hot path (for example, inside an agent or a tight CI loop).

**Explicit mapping (`--set ENV_NAME=service`, repeatable):**

```bash
# Inject the password of "github" as GITHUB_TOKEN, then run gh
pass-cli exec --set GITHUB_TOKEN=github -- gh repo list
```

Everything after `--` is the command to run. There must be a `--` followed by a
command, or `exec` errors.

**Convenience form (derive the env name from the service):**

```bash
# Service name -> env name: uppercased, every non-alphanumeric char -> '_'
# "openai-api" sets OPENAI_API to the password field.
pass-cli exec openai-api -- python train.py
```

Do not combine a positional `<service>` with `--set` - pick one form per command.

**Multiple credentials at once:**

```bash
pass-cli exec \
  --set AWS_ACCESS_KEY_ID=aws-id \
  --set AWS_SECRET_ACCESS_KEY=aws-secret \
  -- aws s3 ls
```

**Selecting a field.** `-f/--field` (default `password`) selects the field for every
mapping; valid fields are `username`, `password`, `category`, `url`, `notes`, `service`.

```bash
# Inject the username field instead of the password
pass-cli exec --set DB_USER=postgres --field username -- ./run-migration.sh
```

**Two fields of one entry as separate variables.** A per-mapping `:field` suffix
overrides `--field`, so you can inject both a username and a password from the same
credential:

```bash
pass-cli exec \
  --set DB_USER=postgres:username \
  --set DB_PASSWORD=postgres:password \
  -- ./run-migration.sh
```

**Exit-code propagation.** The child's exit code is propagated unchanged, so error
handling works exactly as if you had run the command directly:

```bash
pass-cli exec --set GITHUB_TOKEN=github -- gh repo list
echo "exit code: $?"   # the exit code of gh, untouched by pass-cli
```

**Security note (honest scope).** The injected value lives in the child process's
environment. On Linux it is readable via `/proc/<pid>/environ` by the same user and is
inherited by descendant processes. This is the same model as `op run` and
`aws-vault exec` - far safer than files, clipboards, or shell history, but it is **not**
process isolation.

## Script Integration

### Capturing Values with Command Substitution (Fallback)

When you need the secret *inside your own script* - not just to launch a child command -
capture it with command substitution. This pattern works, but the value is exposed to
anything that echoes the command (`set -x`, CI job logs, file-watching tools), so prefer
[`pass-cli exec`](#inject-credentials-with-exec-recommended) whenever you are simply
handing the credential to another command.

### Bash/Zsh Examples

**Export to environment variable:**

```bash
#!/bin/bash

# Export password
export SERVICE_PASSWORD=$(pass-cli get testservice --quiet)

# Export specific field
export SERVICE_USER=$(pass-cli get testservice --field username --quiet)

# Use in command
mysql -u "$(pass-cli get testservice -f username -q)" \
      -p"$(pass-cli get testservice -q)" \
      mydb
```

> **Tip:** To run `mysql` without the password landing in your shell, use
> `exec` instead: `pass-cli exec --set MYSQL_PWD=testservice -- mysql -u <user> mydb`.

**Conditional execution:**

```bash
# Check if credential exists
if pass-cli get testservice --quiet &>/dev/null; then
    echo "Credential exists"
    export API_KEY=$(pass-cli get testservice --quiet)
else
    echo "Credential not found"
    exit 1
fi
```

**Loop through credentials:**

```bash
# Process all credentials
for service in $(pass-cli list -q); do
    echo "Processing $service..."
    username=$(pass-cli get "$service" --field username --quiet)
    echo "  Username: $username"
done
```

### PowerShell Examples

**Export to environment variable:**

```powershell
# Export password
$env:SERVICE_PASSWORD = pass-cli get testservice --quiet

# Export specific field
$env:SERVICE_USER = pass-cli get testservice --field username --quiet

# Use in command
$apiKey = pass-cli get github --quiet
Invoke-RestMethod -Uri "https://api.github.com" -Headers @{
    "Authorization" = "Bearer $apiKey"
}
```

**Conditional execution:**

```powershell
# Check if credential exists
try {
    $password = pass-cli get testservice --quiet 2>$null
    Write-Host "Credential exists"
    $env:API_KEY = $password
} catch {
    Write-Host "Credential not found"
    exit 1
}
```

### Python Examples

```python
import subprocess

# Get password only
result = subprocess.run(
    ['pass-cli', 'get', 'github', '--quiet'],
    capture_output=True,
    text=True,
    check=True
)
password = result.stdout.strip()

# Get specific field
result = subprocess.run(
    ['pass-cli', 'get', 'github', '--field', 'username', '--quiet'],
    capture_output=True,
    text=True,
    check=True
)
username = result.stdout.strip()
```

### Makefile Examples

Lead with `exec` so the secrets are not interpolated into recipe lines (which `make`
echoes by default unless prefixed with `@`):

```makefile
.PHONY: deploy
deploy:
	@pass-cli exec \
	  --set AWS_KEY=aws:username \
	  --set AWS_SECRET=aws:password \
	  -- ./deploy.sh
```

Command substitution is still the right tool when you must *build* a value (like a DSN)
inside the recipe; keep it under `@` so the line is not echoed:

```makefile
.PHONY: test-db
test-db:
	@DB_URL="postgres://$$(pass-cli get testdb -f username -q):$$(pass-cli get testdb -q)@localhost/testdb" \
	go test ./...
```

## Environment Variables

### PASS_CLI_VERBOSE

Enable verbose logging.

```bash
# Bash
export PASS_CLI_VERBOSE=1
pass-cli get github

# PowerShell
$env:PASS_CLI_VERBOSE = "1"
pass-cli get github
```

**Note**: To use a custom vault location, configure `vault_path` in the config file (`~/.pass-cli/config.yml`) instead of using environment variables. See the [Configuration reference](../03-reference/configuration.md).

## Best Practices

### Security

1. **Never pass passwords via flags** - Use prompts or `--generate`
2. **Use quiet mode in scripts** - Prevents logging sensitive data
3. **Clear shell history** - When testing commands with passwords
4. **Use strong master passwords** - 20+ characters recommended

### Workflow

1. **Generate passwords** - Use `--generate` for new credentials
2. **Update regularly** - Rotate credentials periodically
3. **Track usage** - Review unused credentials monthly
4. **Backup vault** - Copy `~/.pass-cli/vault.enc` regularly

### Scripting

1. **Prefer `exec` to hand a credential to a command** - Keeps the value out of files, history, and logs
2. **Use `--quiet` when you must capture a value** - Clean output for variables
3. **Check exit codes** - Handle errors properly (`exec` propagates the child's code unchanged)
4. **Use `--field`** - Extract exactly what you need
5. **Redirect stderr** - Control error output

### Examples

**Good:**
```bash
export API_KEY=$(pass-cli get service --quiet 2>/dev/null)
if [ -z "$API_KEY" ]; then
    echo "Failed to get credential" >&2
    exit 1
fi
```

**Bad:**
```bash
# Don't do this - exposes password in process list
pass-cli add service --password mySecretPassword
```

## Common Patterns

### CI/CD Pipeline

Inject the deployment credentials straight into the deploy command with `exec`, so the
values never appear in the job log (unlike `export VAR=$(...)`, which CI logging or
`set -x` can capture). `deploy.sh` reads `$DEPLOY_KEY` and `$DB_PASSWORD` from its
environment, and its exit code propagates unchanged:

```bash
pass-cli exec \
  --set DEPLOY_KEY=production \
  --set DB_PASSWORD=prod-db \
  -- ./deploy.sh
```

<details>
<summary>Command-substitution fallback (exposes values to job logs / <code>set -x</code>)</summary>

```bash
# Retrieve deployment credentials
export DEPLOY_KEY=$(pass-cli get production --quiet)
export DB_PASSWORD=$(pass-cli get prod-db --quiet)

# Run deployment
./deploy.sh
```

</details>

### Local Development

Pull several fields of one entry (`dev-db`) into separate variables with per-mapping
`:field` overrides, and launch the dev server in one step:

```bash
pass-cli exec \
  --set DB_HOST=dev-db:url \
  --set DB_USER=dev-db:username \
  --set DB_PASS=dev-db:password \
  -- npm run dev
```

<details>
<summary>Command-substitution fallback (exposes values to shell tracing)</summary>

```bash
# Set up environment from credentials
export DB_HOST=$(pass-cli get dev-db --field url --quiet)
export DB_USER=$(pass-cli get dev-db --field username --quiet)
export DB_PASS=$(pass-cli get dev-db --quiet)

# Start development server
npm run dev
```

</details>

### Credential Rotation

```bash
# Generate new password
NEW_PWD=$(pass-cli generate --length 32 --quiet)

# Update service
pass-cli update testservice --password "$NEW_PWD"

# Use new password
echo "$NEW_PWD" | some-service-update-command
```

