# ubereats-pp-cli

Agent-native Uber Eats **buyer** CLI (Printing Press style): cookie auth → unofficial web RPC → SQLite → JSON.

Read-only. It does not place orders, tip, pay, or talk to Uber rides.

See [PLAN.md](PLAN.md) for auth, hypothesized wire shapes, schema, and the v1 cut line.

## Install

Requires Go 1.22+.

```bash
go install github.com/amansk/ubereats-pp-cli/cmd/ubereats-pp-cli@latest
```

From a checkout:

```bash
go build -o ubereats-pp-cli ./cmd/ubereats-pp-cli
```

## Cookie import

Uber Eats has no public buyer history API. This CLI replays the same `getPastOrdersV1` call the website uses, with **your** browser cookies.

1. Sign in at [https://www.ubereats.com](https://www.ubereats.com) (or `auth.uber.com`).
2. Export cookies for `.ubereats.com` / `.uber.com` as one of:
   - Netscape `cookies.txt`
   - Chrome/Playwright JSON (`[{name,value,domain}]`)
   - The raw `Cookie` request header from DevTools
3. Import (values are stored at `~/.config/ubereats-pp-cli/cookies.json` mode `0600` and **never printed**):

```bash
ubereats-pp-cli auth login --cookie-file ./cookies.txt
# or
cat cookies.header | ubereats-pp-cli auth login --cookie-file -
# best-effort local Chrome (often fails when Chrome encrypts the DB):
ubereats-pp-cli auth login --chrome
```

4. Check the session (names + count only) and local store:

```bash
ubereats-pp-cli auth status
ubereats-pp-cli doctor
```

`doctor` is green after a successful import without a live Uber call. Add `--live` to ping `getUserV1`.

## Sync and query

```bash
ubereats-pp-cli sync                  # incremental
ubereats-pp-cli sync --full
ubereats-pp-cli orders list --json
ubereats-pp-cli orders get <id> --agent
ubereats-pp-cli spend --by month
ubereats-pp-cli spend --by year --json
ubereats-pp-cli top-items
ubereats-pp-cli top-restaurants
ubereats-pp-cli find 'ramen' --json
```

`--agent` expands to compact JSON, no color, no prompts.

Offline / CI (reconstructed fixture, not a live capture):

```bash
ubereats-pp-cli --home /tmp/ue auth login --cookie-file testdata/fixtures/cookies.txt
ubereats-pp-cli --home /tmp/ue doctor
ubereats-pp-cli --home /tmp/ue sync --from-fixture testdata/fixtures/past_orders_page.json
ubereats-pp-cli --home /tmp/ue orders list --json
```

State lives in `$UBEREATS_PP_HOME` or `~/.config/ubereats-pp-cli` (`cookies.json`, `ubereats.db`).

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | ok |
| 2 | usage |
| 3 | not found |
| 4 | auth / missing session |
| 5 | API / parse |
| 7 | transient / rate limit |

## Non-goals

Place, tip, pay, cart writes, Uber rides.

Agents: see [SKILL.md](SKILL.md).

## Security notes

- Cookies are stored in plaintext at `cookies.json` with mode `0600`. Treat that file like a password; `auth logout` deletes it.
- Cookie values are never written to stdout, stderr, logs, or error messages. Tests assert this against the fixtures.
- This CLI replays an unofficial, undocumented web endpoint with your own session. Uber can change or block it at any time, and use may be subject to Uber's terms of service. Use it only on your own account.

## Contributing to the Printing Press library

This CLI follows [Printing Press](https://printingpress.dev) conventions (agent flags, typed exit codes, local SQLite). Note that the public [printing-press-library](https://github.com/mvanhorn/printing-press-library) only accepts entries produced by the `/printing-press` generator, which adds the provenance manifest, manuscripts, and proof artifacts its CI requires. To submit this CLI there, run it through the generator (or `/printing-press-reprint`) rather than opening a hand-built PR.

## Development

```bash
go test ./...
go vet ./...
gofmt -l .
```

CI runs the same checks plus the offline fixture smoke test above.
