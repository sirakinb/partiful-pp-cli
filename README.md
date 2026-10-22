# Partiful CLI and MCP

A small tool that lets you, or an AI assistant like Claude, manage your own [Partiful](https://partiful.com) events without clicking through the website.

If you host events on Partiful, you probably do the same things over and over: check who's coming, copy last month's event for next month, fix a time or an address. This tool does those for you. You can type a short command, or just ask your AI assistant in plain English.

It's unofficial and not made by Partiful.

## What it helps with

**Seeing your events at a glance.** List what's coming up, what you're hosting, and what already happened, with dates, locations and how many people said yes.

**Knowing who's coming.** Get the guest list for any event you host, or a quick headcount: how many are going, maybe, or on the waitlist, including plus-ones. You can export the "going" list as a spreadsheet.

**Setting up the next event in a series.** Start a new event from an old one. It keeps the same image, look and description, so you only change the date and title.

**Fixing details.** Change the title, time, description, location, cover image, or whether the event is public.

**Letting an AI assistant help.** Connect it to Claude (or another assistant that supports MCP) and ask things like:

- "Who RSVP'd to Breakfast and Building?"
- "How many people came to my last three events?"
- "Draft next month's Breakfast and Building for December 12 at 9am, same as last time."
- "Move my November event to start at 9:30."

## You stay in control

Nothing on Partiful changes by accident. Anything that would create, edit or publish an event first shows you a preview of exactly what would change, like this:

```
title:     Breakfast and Building  →  Breakfast and Building 02
startDate: 9:00 AM                 →  9:30 AM
```

The change only happens when you confirm it. An AI assistant has to ask for that confirmation too.

## What it can't do

- It can't delete events, message guests, or send invites. Use the Partiful app for those.
- It only works with events on your own account. It can show guest lists for events you host or co-host, not ones you were invited to.
- Partiful doesn't offer an official way for apps to connect, so this tool talks to Partiful the same way its website does. If Partiful changes its website, parts of this tool may stop working until it's updated.

## Is it safe?

- It signs in with your own Partiful login, which you copy once from your browser (steps below). There's no password involved, and it never asks for one.
- Your login is saved only on your computer, in a file that only your user account can open. It's never put in this repo or sent anywhere except Partiful and Google, which runs Partiful's sign-in.
- You can sign out at any time with `partiful-pp-cli auth logout`.

## Getting started

1. **Install it** (needs [Go](https://go.dev/dl/)):

   ```bash
   go install github.com/sirakinb/partiful-pp-cli/cmd/partiful-pp-cli@latest
   go install github.com/sirakinb/partiful-pp-cli/cmd/partiful-pp-mcp@latest
   ```

2. **Sign in once** by following [Signing in, step by step](#signing-in-step-by-step) below. It takes about a minute.

3. **Try it:**

   ```bash
   partiful-pp-cli events list
   ```

To use it with Claude Code, run `claude mcp add partiful -- ~/go/bin/partiful-pp-mcp`. For Claude Desktop, see [Use as an MCP server](#use-as-an-mcp-server).

---

The rest of this page is the technical reference.

Created by [@sirakinb](https://github.com/sirakinb).

## Setup

Requires Go 1.26.6 or newer. Install straight from GitHub:

```bash
go install github.com/sirakinb/partiful-pp-cli/cmd/partiful-pp-cli@latest
go install github.com/sirakinb/partiful-pp-cli/cmd/partiful-pp-mcp@latest
```

Or from a clone of this repo:

```bash
go install ./cmd/partiful-pp-cli ./cmd/partiful-pp-mcp
```

That puts `partiful-pp-cli` and `partiful-pp-mcp` in `~/go/bin`. Make sure it is on your `PATH`, then check the install:

```bash
partiful-pp-cli doctor
```

## Authentication

Sign in at partiful.com in your browser, copy your Firebase refresh token with the steps in "Signing in, step by step" below, and run `partiful-pp-cli auth login`. The token is stored locally with 0600 permissions and exchanged for short-lived ID tokens on demand.

## Signing in, step by step

Partiful has no API keys. The CLI uses your own signed-in session: the login token that partiful.com keeps in your browser. You copy it once.

1. In Chrome, open partiful.com and sign in.
2. Open the Console: View > Developer > JavaScript Console (or Cmd+Option+J). If Chrome warns about pasting, type `allow pasting` and press Enter first.
3. Paste this and press Enter. It loads your token without showing it:

   ```js
   (async()=>{const db=await new Promise(r=>{const q=indexedDB.open('firebaseLocalStorageDb');q.onsuccess=()=>r(q.result)});const rows=await new Promise(r=>{const q=db.transaction('firebaseLocalStorage').objectStore('firebaseLocalStorage').getAll();q.onsuccess=()=>r(q.result)});window.__pt=rows.map(x=>x.value).find(v=>v&&v.stsTokenManager).stsTokenManager.refreshToken;console.log('ready')})()
   ```

4. When it prints `ready`, paste this to copy the token to your clipboard:

   ```js
   copy(window.__pt); delete window.__pt
   ```

5. In a terminal, save it and clear your clipboard:

   ```bash
   pbpaste | partiful-pp-cli auth login && printf '' | pbcopy
   ```

You should see `"signedIn": true`. The token is saved in `~/.config/partiful-pp-cli/session.json`, readable only by your user. To keep it somewhere else, set `PARTIFUL_SESSION_FILE`. To skip the file, set `PARTIFUL_REFRESH_TOKEN` instead.

```bash
partiful-pp-cli auth status   # am I signed in?
partiful-pp-cli auth logout   # remove the saved session
```

If the token stops working (for example after you sign out of Partiful everywhere), repeat these steps. Never commit `session.json`.

## Writes are previewed

`events create`, `events update` and `events publish` only print what they would do. Nothing changes on Partiful until you add `--apply`. The global `--yes` and `--agent` flags do not apply writes, so an agent has to ask for `--apply` explicitly.

```bash
partiful-pp-cli events update hqWkFzikB1frVdihGKTE --start "2026-11-14 09:30"           # shows a before/after diff
partiful-pp-cli events update hqWkFzikB1frVdihGKTE --start "2026-11-14 09:30" --apply   # saves it
```

`events create --copy-from <event>` reuses an earlier event's image, theme, description and settings, which suits a recurring series. Times like `"2026-12-12 09:00"` are read in the event's timezone (default `America/New_York`).

## Use as an MCP server

The MCP server exposes the same commands as tools. Read tools are marked read-only, and write tools take an explicit `apply` argument.

Claude Code:

```bash
claude mcp add partiful -- ~/go/bin/partiful-pp-mcp
```

Claude Desktop (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{ "mcpServers": { "partiful": { "command": "/Users/YOU/go/bin/partiful-pp-mcp" } } }
```

Run `auth login` once first. The MCP server reads the same session file.

## How it works

Partiful's web app talks to Firebase. This CLI makes the same calls:

| What | Endpoint |
|------|----------|
| List events, event details, create, publish, set location | `POST https://api.partiful.com/<function>` (Firebase callable functions) |
| Edit event fields, host guest list | Firestore REST, project `getpartiful` (`events/<id>`, `events/<id>/guests`) |
| Cover image upload | Firebase Storage, bucket `getpartiful.appspot.com` |
| Session refresh | `securetoken.googleapis.com` |

This is an unofficial, reverse-engineered client. Partiful can change these endpoints at any time.

## Known gaps

- `events update --apply` has been tested live (a title change and its revert, 2026-10-21). `events create` and image uploads (`--image`) are implemented and unit-tested, but have not yet been run against a live account.
- `--location` sets a plain-text location, the same as typing one in the web app. It replaces a map-linked (structured) address with text.
- There are no commands for deleting events or messaging guests. Use `functions <name>` for other web-app calls at your own risk.

## Quick Start

```bash
# Check config and connectivity
partiful-pp-cli doctor --dry-run

# See events you host
partiful-pp-cli events list --hosting

# Read one event
partiful-pp-cli events get hqWkFzikB1frVdihGKTE

# Headcount by RSVP status
partiful-pp-cli guests summary hqWkFzikB1frVdihGKTE

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Host tools
- **`guests summary`**: See going, maybe, declined and waitlist counts for an event you host, with plus-ones.

  _Use it when an agent needs headcount for an event without paging the full guest list._

  ```bash
  partiful-pp-cli guests summary hqWkFzikB1frVdihGKTE --json
  ```

## Recipes

### Upcoming hosted events, compact

```bash
partiful-pp-cli events list --hosting --agent --select id,title,startDate
```

Narrow the event list to the fields an agent needs.

### Preview an edit

```bash
partiful-pp-cli events update hqWkFzikB1frVdihGKTE --title "Breakfast and Building"
```

Shows a field-by-field diff without changing anything; add --apply to save it (confirm with the user first).

### Going guests as CSV

```bash
partiful-pp-cli guests list hqWkFzikB1frVdihGKTE --status GOING --csv
```

Export the people who said yes.

## Usage

Run `partiful-pp-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data such as `data.db` |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `PARTIFUL_CONFIG_DIR`, `PARTIFUL_DATA_DIR`, `PARTIFUL_STATE_DIR`, or `PARTIFUL_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `PARTIFUL_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export PARTIFUL_HOME=/srv/partiful
partiful-pp-cli doctor
```

Under `PARTIFUL_HOME=/srv/partiful`, the four dirs resolve to `/srv/partiful/config`, `/srv/partiful/data`, `/srv/partiful/state`, and `/srv/partiful/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "partiful": {
      "command": "partiful-pp-mcp",
      "env": {
        "PARTIFUL_HOME": "/srv/partiful"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `PARTIFUL_DATA_DIR` overrides an explicit `--home` for that kind. Use `PARTIFUL_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `PARTIFUL_HOME` does not move files back to platform defaults, and `doctor` cannot find files left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. Run `partiful-pp-cli doctor --fail-on warn` to check path warnings in automation.

## Commands

### Events and guests

- **`partiful-pp-cli events list`** - Your upcoming events (`--past`, `--all`, `--hosting`, `--limit`)
- **`partiful-pp-cli events get <id-or-url>`** - Full details for one event
- **`partiful-pp-cli events create`** - Preview, or with `--apply` create, a new event (`--title --start --end --timezone --description --description-file --location --image --public --copy-from`)
- **`partiful-pp-cli events update <id-or-url>`** - Field-by-field diff, or with `--apply` save it
- **`partiful-pp-cli events publish <id-or-url>`** - Publish a draft (`--apply`)
- **`partiful-pp-cli guests list <id-or-url>`** - Host guest list (`--status GOING`)
- **`partiful-pp-cli guests summary <id-or-url>`** - Counts and headcount by RSVP status
- **`partiful-pp-cli auth login|status|logout`** - Manage the saved session

### functions

Call a raw Partiful API function (advanced escape hatch; prefer the events and guests commands)

- **`partiful-pp-cli functions <function>`** - Call a Partiful callable function by name with a raw JSON body


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`partiful-pp-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`partiful-pp-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`partiful-pp-cli learnings list`** - Inspect taught rows
- **`partiful-pp-cli learnings forget <query>`** - Undo a teach
- **`partiful-pp-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`partiful-pp-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`partiful-pp-cli teach-pattern`** - Install a query/resource template up front
- **`partiful-pp-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `PARTIFUL_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `partiful-pp-cli` opens the database, older binaries refuse it with a version error: upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
partiful-pp-cli functions mock-value

# JSON for scripting and agents
partiful-pp-cli functions mock-value --json

# Filter to specific fields
partiful-pp-cli functions mock-value --json --select id,name,status

# Dry run: show the request without sending
partiful-pp-cli functions mock-value --dry-run

# Agent mode: JSON + compact + no prompts in one flag
partiful-pp-cli functions mock-value --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
partiful-pp-cli doctor
```

Verifies configuration and connectivity to the API.

## Configuration

Run `partiful-pp-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/partiful-pp-cli/config.toml`; `--home`, `PARTIFUL_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

## Troubleshooting
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **not signed in / token refresh failed**: Run partiful-pp-cli auth login again with a fresh refresh token from the browser
- **Missing or insufficient permissions on guests list**: Guest lists are only readable for events you host or cohost

## HTTP Transport

This CLI uses Chrome-compatible HTTP transport for browser-facing endpoints. It does not require a resident browser process for normal API calls.

TLS certificates are verified by default. For a trusted development or self-signed endpoint only, pass `--insecure` for one invocation, set `PARTIFUL_SKIP_TLS_VERIFY=true` for the current environment, or set `skip_tls_verify = true` in the config file for a persistent override.

---

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
