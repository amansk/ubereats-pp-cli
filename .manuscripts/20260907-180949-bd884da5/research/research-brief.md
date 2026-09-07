# Uber Eats buyer CLI — research brief

## Target

Uber Eats consumer (buyer) order history. There is no public buyer API: the
Eats Marketplace OAuth scopes (`eats.order`, `eats.store`, ...) are merchant
integrations and cannot read a personal account's history. The website itself
is the only reachable surface, so this CLI replays the browser's RPC calls.

## Wire surface (sniffed, community-corroborated)

| Op | Method | Purpose | Notes |
| --- | --- | --- | --- |
| `getPastOrdersV1` | `POST /_p/api/getPastOrdersV1` | Paginated buyer history | Body `{"lastWorkflowUUID": ""}`; response `data.ordersMap` keyed by workflow UUID, `data.meta.hasMore`; ~10 rows per page |
| `getUserV1` | `POST /_p/api/getUserV1` | Identity probe | Body `{"shouldGetSubsMetadata": true}`; used only by `doctor --live` / browser-session validation |

Required headers: `x-csrf-token: x` (literal), `Origin: https://www.ubereats.com`,
`Referer: https://www.ubereats.com/orders`, plus the browser cookie jar for
`.ubereats.com` (`sid`, `csid`, `jwt-session`, `uev2.id.session`, `_ua` are the
session-shaped names). Rides and Eats share a login but not a cookie jar.

Sources: neopheus getPastOrdersV1 replay gist, Coffee-Boyy/ai-food-order,
independent browser captures. Order-object field names are hunches until a live
capture is archived in `proofs/`; the parser walks several candidate paths and
keeps the original per-order JSON in `raw_json`.

## Alternatives and gaps

Existing scripts dump one page of raw JSON with no pagination loop, no local
store, no typed exit codes, and print the cookie header on error. Nothing
answers spend-by-month, most-ordered items, or "when did I last order X".

## Novel features (built)

`sync`, `history list|get`, `spend`, `top-items`, `top-restaurants`, `find`.
All read the local SQLite mirror except `sync`, which walks the live RPC.

## Risks

- Unofficial API: field names rotate. Mitigation: tolerant parser + raw_json.
- DataDome / WAF on datacenter IPs. Mitigation: browser-like headers; HTML
  responses are surfaced as API errors with a WAF hint.
- Cookie secrecy. Mitigation: values never printed; credentials file 0600.
- Terms of service. Read-only; user's own session; no writes of any kind.
