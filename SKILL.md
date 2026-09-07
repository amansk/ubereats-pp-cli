---
name: ubereats-pp-cli
description: Read-only Uber Eats buyer CLI. Cookie auth, sync order history to SQLite, query spend and items as JSON.
---

# ubereats-pp-cli

Agent-native buyer CLI. **Read-only.** Never print cookie values. Never place, tip, or pay.

## Install

```bash
go install github.com/amansk/ubereats-pp-cli/cmd/ubereats-pp-cli@latest
# or from a checkout:
go build -o ubereats-pp-cli ./cmd/ubereats-pp-cli
```

Verify: `ubereats-pp-cli --help`

## Auth

User must already be logged into ubereats.com. Import cookies; do not log values.

```bash
ubereats-pp-cli auth login --cookie-file /path/to/cookies.txt --json
# stdin:
ubereats-pp-cli auth login --cookie-file - --no-input < cookies.header
# best-effort Chrome (often encrypted / unavailable):
ubereats-pp-cli auth login --chrome --json
ubereats-pp-cli auth status --json
ubereats-pp-cli doctor --agent
```

`doctor` is green after import without `--live`. Use `--live` only when a real session should ping Uber.

## Sync then query

```bash
ubereats-pp-cli sync --agent
ubereats-pp-cli sync --full --json
ubereats-pp-cli orders list --limit 20 --json
ubereats-pp-cli orders get 11111111-1111-1111-1111-111111111111 --agent
ubereats-pp-cli spend --by month --json
ubereats-pp-cli spend --by year --agent
ubereats-pp-cli top-items --limit 10 --json
ubereats-pp-cli top-restaurants --agent
ubereats-pp-cli find 'burger' --json
```

`--agent` = compact JSON + no prompts + no color.

CI / no network:

```bash
ubereats-pp-cli --home "$TMPDIR/ue" auth login --cookie-file testdata/fixtures/cookies.txt --json
ubereats-pp-cli --home "$TMPDIR/ue" doctor --agent
ubereats-pp-cli --home "$TMPDIR/ue" sync --from-fixture testdata/fixtures/past_orders_page.json --json
```

## Rules

- If `auth status` shows `present: false`, stop and ask the user for a cookie export. Do not invent cookies.
- Wire shapes in PLAN.md are hunches; empty item lists are possible. Prefer `--json` and the stored ids over guessing.
- Exit 4 = auth, 3 = missing order, 5 = API, 7 = transient.
