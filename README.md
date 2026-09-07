# Uber Eats CLI

**Your Uber Eats order history as a local database — spend, favourites, and search the app cannot answer.**

Uber Eats has no public buyer API and the website only shows ten orders at a time. This CLI replays the same order-history call the website makes with your own browser session, mirrors every past order and line item into SQLite, and answers the questions the app cannot: how much did I spend last month, what do I order most, and when did I last order ramen. It is strictly read-only: it never places, tips, pays, or touches Uber rides.

Learn more at [Uber Eats](https://www.ubereats.com).

Created by [@amansk](https://github.com/amansk) (Amandeep Khurana).

## Install

The recommended path installs both the `ubereats-pp-cli` binary and the `pp-ubereats` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install ubereats
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install ubereats --cli-only
```

For skill only — installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install ubereats --skill-only
```

To constrain the skill install to one or more specific agents (repeatable — agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install ubereats --agent claude-code
npx -y @mvanhorn/printing-press-library install ubereats --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.6 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/food-and-dining/ubereats/cmd/ubereats-pp-cli@latest
```

This installs the CLI only — no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/ubereats-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install ubereats --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-ubereats --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-ubereats --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install ubereats --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

The bundle reuses your local browser session — set it up first if you haven't:

```bash
ubereats-pp-cli auth login --chrome
```

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/ubereats-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/food-and-dining/ubereats/cmd/ubereats-pp-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "ubereats": {
      "command": "ubereats-pp-mcp"
    }
  }
}
```

</details>

## Authentication

Uber Eats consumer endpoints are cookie-session only; the public Eats Marketplace OAuth scopes are merchant-side and cannot read a buyer's history. Sign in at ubereats.com in Chrome, then run `ubereats-pp-cli auth login --chrome` (or `--cookies-file` with a Playwright storage-state export or a raw Cookie header). Cookie values are stored with mode 0600 and never printed.

## Quick Start

### 1. Install

See [Install](#install) above.

### 2. Authenticate

This CLI uses your browser session for authentication. Log in to .ubereats.com in Chrome, then:

```bash
ubereats-pp-cli auth login --chrome
```

Or import an existing browser capture:

```bash
ubereats-pp-cli auth login --cookies-file storage-state.json
```

`--cookies-file` accepts Playwright storage-state JSON or a raw `Cookie:` header text file. The Chrome path requires a cookie extraction tool. Install one:

```bash
pip install pycookiecheat          # Python (recommended)
brew install barnardb/cookies/cookies  # Homebrew
```

When your session expires, run `auth login --chrome` again.

### 3. Verify Setup

```bash
ubereats-pp-cli doctor
```

This checks your configuration and credentials.

### 4. Try Your First Command

```bash
ubereats-pp-cli orders
```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`sync`** — Walk every page of getPastOrdersV1 and mirror orders plus line items into SQLite, incrementally by default.

  _Run it once and every other command works offline in milliseconds._

  ```bash
  ubereats-pp-cli sync --agent
  ```
- **`history`** — List synced orders newest-first with date bounds, or fetch one order with its line items by id.

  _Stable UUIDs and date filters make follow-up questions cheap._

  ```bash
  ubereats-pp-cli history list --since 2026-01-01 --limit 20 --agent
  ```

### Analytics the app cannot do
- **`spend`** — Sum order totals into monthly or yearly buckets from the local mirror.

  _Answers 'how much did I spend on delivery this year' in one call._

  ```bash
  ubereats-pp-cli spend --by month --agent
  ```
- **`top-items`** — Rank line items by quantity across every synced order, with order counts and spend.

  _Grounds 'what do I usually get' in actual counts._

  ```bash
  ubereats-pp-cli top-items --limit 10 --agent
  ```
- **`top-restaurants`** — Rank restaurants by order count and total spend.

  _Shows where the money actually goes._

  ```bash
  ubereats-pp-cli top-restaurants --limit 10 --agent
  ```
- **`find`** — Case-insensitive search across restaurant names and item titles in the local mirror.

  _'When did I last order X' becomes a single local query._

  ```bash
  ubereats-pp-cli find ramen --agent
  ```

## Usage

Run `ubereats-pp-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `UBEREATS_CONFIG_DIR`, `UBEREATS_DATA_DIR`, `UBEREATS_STATE_DIR`, or `UBEREATS_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `UBEREATS_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export UBEREATS_HOME=/srv/ubereats
ubereats-pp-cli doctor
```

Under `UBEREATS_HOME=/srv/ubereats`, the four dirs resolve to `/srv/ubereats/config`, `/srv/ubereats/data`, `/srv/ubereats/state`, and `/srv/ubereats/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "ubereats": {
      "command": "ubereats-pp-mcp",
      "env": {
        "UBEREATS_HOME": "/srv/ubereats"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `UBEREATS_DATA_DIR` overrides an explicit `--home` for that kind. Use `UBEREATS_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `UBEREATS_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `ubereats-pp-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### orders

Past Uber Eats orders (buyer history)

- **`ubereats-pp-cli orders`** - List past orders, newest first (paginated by lastWorkflowUUID)

### user

The signed-in buyer profile

- **`ubereats-pp-cli user`** - Fetch the signed-in user profile (live session probe)


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`ubereats-pp-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`ubereats-pp-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`ubereats-pp-cli learnings list`** - Inspect taught rows
- **`ubereats-pp-cli learnings forget <query>`** - Undo a teach
- **`ubereats-pp-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`ubereats-pp-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`ubereats-pp-cli teach-pattern`** - Install a query/resource template up front
- **`ubereats-pp-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `UBEREATS_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `ubereats-pp-cli` opens the database, older binaries refuse it with a version error — upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
ubereats-pp-cli orders

# JSON for scripting and agents
ubereats-pp-cli orders --json
# Filter to specific fields
ubereats-pp-cli orders --json --select ordersMap,meta

# Dry run — show the request without sending
ubereats-pp-cli orders --dry-run

# Agent mode — JSON + compact + no prompts in one flag
ubereats-pp-cli orders --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
ubereats-pp-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `ubereats-pp-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/ubereats-pp-cli/config.toml`; `--home`, `UBEREATS_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `UBEREATS_COOKIES` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `ubereats-pp-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `ubereats-pp-cli doctor` to check credentials
- Verify the environment variable is set: `echo $UBEREATS_COOKIES`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**neopheus getPastOrdersV1 gist**](https://gist.github.com/neopheus/3cc521ed9a299c99a33959192592664a) — python
- [**Coffee-Boyy/ai-food-order**](https://github.com/Coffee-Boyy/ai-food-order) — python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
