# PLAN.md — ubereats-pp-cli v1

Agent-native Printing Press–style Uber Eats **buyer** CLI. Read-only: cookie session → unofficial web RPC → local SQLite → JSON.

This plan is the binding product cut for v1. **API field names below are hunches until a live `getPastOrdersV1` payload is captured.** The parser is deliberately tolerant and always stores `raw_json`.

## Goals

- Import a browser session from `ubereats.com` / `auth.uber.com` without printing cookie values.
- Sync past orders into SQLite (full + incremental).
- Query orders, spend, top items/restaurants, and text search.
- `--json` / `--agent` on every command.
- `doctor` green after cookie import (fixtures acceptable for CI).

## Non-goals (v1)

- Place, tip, pay, cart, checkout, substitutions, ratings.
- Uber **rides** (`riders.uber.com/graphql`).
- Official Marketplace / merchant APIs (wrong audience).
- Live driver tracking, invoices, receipts-as-PDF.
- Shipping a Chrome extension or headed browser login flow.

---

## Auth

Uber Eats consumer APIs are **cookie-session**, not OAuth. The public Eats Marketplace OAuth scopes (`eats.order`, etc.) are merchant-only and cannot read a buyer’s personal history.

### Session sources

| Source | Command | Notes |
| --- | --- | --- |
| Cookie file | `auth login --cookie-file PATH` | Netscape cookies.txt, Chrome JSON export (`[{name,value,domain}]`), or a raw `Cookie:` header line |
| Stdin | `auth login --cookie-file -` or piped stdin | Same formats; `--no-input` refuses a TTY prompt |
| Chrome (best-effort) | `auth login --chrome` | Read via `browserutils/kooky` from the local Chrome/Chromium cookie DB. Feasible on Linux/macOS when the DB is readable. Recent Chrome on macOS often cannot decrypt (Keychain). Fail with a clear fallback, do not invent cookies. |

Accepted domains: `.ubereats.com`, `www.ubereats.com`, `.uber.com`, `auth.uber.com`. Uber Rides and Eats share a login but **not** the same cookie jar — Eats calls must send Eats-domain cookies.

### Cookie names we treat as session-ish (non-binding)

Observed / commonly reported: `sid`, `csid`, `jwt-session`, `uev2.id.session`, `_ua`. Doctor treats presence of **any** named cookie as “imported”; a live ping is optional (`doctor --live`).

### Storage

- Home: `$UBEREATS_PP_HOME` or `~/.config/ubereats-pp-cli`
- Files: `cookies.json` (0600), `ubereats.db`
- On-disk cookie file stores name→value plus metadata (`imported_at`, `source`, domains). **CLI output never includes values** — only names, count, source, and timestamps.
- `auth status` fingerprints the session as `cookies=N names=sid,csid,...` — never values.

### Request headers (Eats web)

Verified by multiple independent captures (AgentOS 2026-04-02 browse-capture, openweb-org, public gists):

```
POST https://www.ubereats.com/_p/api/<Operation>V1
Content-Type: application/json
x-csrf-token: x          # literal "x", not derived from cookies
Cookie: <imported>
Origin: https://www.ubereats.com
Referer: https://www.ubereats.com/orders
```

Optional (browser sends; **not required** for basic reads per AgentOS): `x-uber-session-id`, `x-uber-client-gitref`, `x-uber-ciid`, `x-uber-request-id`, `x-uber-target-location-*`.

---

## Order-history APIs

Consumer Eats is **RPC-style**, not GraphQL. Base: `POST https://www.ubereats.com/_p/api/<op>`.

Envelope (openweb-org + replay scripts): `{ "status": "success"|"failure", "data": { ... } }`.

### Verified enough for v1 (still treat field names as hunches)

| Op | Purpose | Request (reported) | Response (reported) |
| --- | --- | --- | --- |
| `getPastOrdersV1` | Paginated history | `{ "lastWorkflowUUID": "" }` | `data.ordersMap` object; `data.meta.hasMore`; ~10 rows/page |
| `getUserV1` | Identity ping | `{ "shouldGetSubsMetadata": true }` | User profile — **shape unverified**; used only as optional live doctor probe |

Pagination cursor (reported, two variants — try both):

1. Next page: last row’s `baseEaterOrder.uuid` as `lastWorkflowUUID` (gist / AgentOS).
2. Or `data.lastWorkflowUUID` / `data.pagination.nextCursor` if present.

Stop when `meta.hasMore` is false, the page is empty, or the cursor does not advance.

### Order object — hypothesized fields (NON-BINDING)

Reconstructed from [neopheus gist](https://gist.github.com/neopheus/3cc521ed9a299c99a33959192592664a), [ai-food-order](https://github.com/Coffee-Boyy/ai-food-order), and AgentOS notes. **Do not treat as a schema contract.**

```
order.baseEaterOrder.uuid              # order / workflow id
order.baseEaterOrder.currencyCode
order.baseEaterOrder.lastStateChangeAt # ISO-ish timestamp
order.baseEaterOrder.completedAt       # alt timestamp
order.baseEaterOrder.shoppingCart.items[]
order.storeInfo.uuid
order.storeInfo.title
order.storeInfo.heroImageUrl
order.fareInfo.totalPrice              # integer cents
order.workflowUUID                     # sometimes top-level
order.interactionType
```

Item hunches inside `shoppingCart.items[]` or `items[]`: `uuid`/`id`, `title`/`name`/`itemTitle`, `quantity`, `price` (cents int **or** nested `{amount|unitPrice}`).

Parser strategy: walk several candidate paths, coerce cents vs dollars carefully (`totalPrice` is reported as **cents**), keep the original blob in `orders.raw_json` / `items.raw_json`.

### Hypothesized / stubbed (do not block v1)

| Op | Purpose | Status |
| --- | --- | --- |
| `getOrderEntitiesV1` | Detail for an order | Request `{}` in one capture; likely needs an id we have not verified. **Stub:** try `{orderUuid}` then skip on 4xx. |
| `getReceiptByWorkflowUuidV1` | Line items via HTML receipt | AgentOS: parse with lxml. **Stub:** optional follow-up if list payload has no items. |
| `getOrderEntityByUuidV1` | Active orders only | Reported 404 on completed. Do not use for history. |
| `getInvoiceFilesV1` | PDF invoice | `{ orderUUID }` — out of v1 scope. |
| `getActiveOrdersV1` | Live orders | Out of v1 scope. |

Rides GraphQL `Activities` with `order_types: "EATS"` is **invalid** (`RVWebCommonActivityOrderType`). Do not call `riders.uber.com`.

If a live response diverges, prefer raw JSON + a warning over a failed sync.

---

## SQLite schema

File: `$HOME/ubereats.db`. WAL. Migrations are additive `CREATE TABLE IF NOT EXISTS` + `PRAGMA user_version`.

```sql
CREATE TABLE orders (
  id              TEXT PRIMARY KEY,   -- stable: workflow/base uuid
  workflow_uuid   TEXT,
  restaurant_uuid TEXT,
  restaurant_name TEXT NOT NULL DEFAULT '',
  currency        TEXT NOT NULL DEFAULT '',
  total_cents     INTEGER NOT NULL DEFAULT 0,
  ordered_at      TEXT,               -- RFC3339 UTC when parseable
  status          TEXT,
  raw_json        TEXT NOT NULL,
  synced_at       TEXT NOT NULL
);

CREATE TABLE items (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  order_id    TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  item_uuid   TEXT,
  title       TEXT NOT NULL DEFAULT '',
  quantity    INTEGER NOT NULL DEFAULT 1,
  unit_cents  INTEGER NOT NULL DEFAULT 0,
  raw_json    TEXT NOT NULL
);

CREATE TABLE sync_state (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
-- keys: last_sync_at, last_cursor, last_mode, last_error, order_count
```

Indexes: `orders(ordered_at)`, `orders(restaurant_name)`, `items(title)`, `items(order_id)`.

Money is **integer cents**. Display divides by 100 with the stored currency (default USD if empty).

---

## Command surface

Global flags: `--json`, `--agent` (implies `--json --compact --no-input --no-color --yes`), `--quiet`, `--home`, `--no-color`, `--no-input`.

| Command | Behavior |
| --- | --- |
| `auth login --cookie-file PATH` | Import cookies; never echo values |
| `auth login --chrome` | Best-effort browser extract |
| `auth status` | Cookie count/names/source/time — no values |
| `auth logout` | Delete cookie file |
| `doctor` | Config dir, cookies present, SQLite open. `--live` pings `getUserV1` or a 1-page `getPastOrdersV1` |
| `sync` | Incremental (stop when a page is all known ids). `--full` rewrites from page 1. `--from-fixture FILE` for CI |
| `orders list` | `--limit`, `--since`, `--until` |
| `orders get <id>` | One order + items; exit 3 if missing |
| `spend --by month\|year` | Sum `total_cents` |
| `top-items` | Group items by normalized title |
| `top-restaurants` | Group orders by restaurant |
| `find '<query>'` | Case-insensitive LIKE on restaurant + item title |

`--json` / `--agent` emit a single JSON document (`{ok, data, error?}` for errors). Human mode is a compact table.

Typed exit codes (Printing Press golden): `0` ok, `2` usage, `3` not found, `4` auth, `5` API, `7` transient/rate-limit.

---

## Risks

| Risk | Mitigation |
| --- | --- |
| Unofficial API; fields rotate | Flexible parser + `raw_json`; version the fixture; warn, don’t crash |
| DataDome / WAF from datacenter IPs | Browser-like headers; treat HTML/403 as API error with a “WAF?” hint; no TLS impersonation in v1 |
| Cookie theft if we log secrets | Redact all cookie values in logs/errors/JSON; 0600 files; `.gitignore` |
| Chrome decrypt fails | File/stdin path is the supported path; `--chrome` is best-effort |
| Pagination loops | Max pages (250), cursor-must-advance |
| ToS / account risk | Read-only; user-owned session; no writes |

---

## v1 cut line

**In**

- PLAN.md, SKILL.md, README (`go install` / `go build`)
- Cookie import (file + stdin + `--chrome` stub-or-real)
- SQLite sync from `getPastOrdersV1` **or** `--from-fixture`
- Query commands listed above with `--json`/`--agent`
- Tests against fixtures (parser, store, CLI, redaction)
- `doctor` green after fixture cookie import without a live Uber session

**Out / stub-marked**

- Live receipt HTML parsing (`getReceiptByWorkflowUuidV1`)
- Detail RPC until a real payload is captured
- Writes of any kind
- Rides

**Success check for CI**

```
go test ./...
ubereats-pp-cli --home /tmp/ue auth login --cookie-file testdata/fixtures/cookies.txt
ubereats-pp-cli --home /tmp/ue doctor   # green
ubereats-pp-cli --home /tmp/ue sync --from-fixture testdata/fixtures/past_orders_page.json
ubereats-pp-cli --home /tmp/ue orders list --json
```
