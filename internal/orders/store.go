// Copyright 2026 Amandeep Khurana and contributors. Licensed under Apache-2.0. See LICENSE.

package orders

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound is returned by GetOrder when the id is not in the mirror.
var ErrNotFound = errors.New("order not found")

// Migrations are the CREATE statements for the order mirror. They are
// idempotent and are run from the store's migrateExtras hook.
var Migrations = []string{
	`CREATE TABLE IF NOT EXISTS ue_orders (
  id              TEXT PRIMARY KEY,
  workflow_uuid   TEXT,
  restaurant_uuid TEXT,
  restaurant_name TEXT NOT NULL DEFAULT '',
  currency        TEXT NOT NULL DEFAULT '',
  total_cents     INTEGER NOT NULL DEFAULT 0,
  ordered_at      TEXT,
  status          TEXT,
  raw_json        TEXT NOT NULL,
  synced_at       TEXT NOT NULL
)`,
	`CREATE TABLE IF NOT EXISTS ue_items (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  order_id    TEXT NOT NULL REFERENCES ue_orders(id) ON DELETE CASCADE,
  item_uuid   TEXT,
  title       TEXT NOT NULL DEFAULT '',
  quantity    INTEGER NOT NULL DEFAULT 1,
  unit_cents  INTEGER NOT NULL DEFAULT 0,
  raw_json    TEXT NOT NULL
)`,
	`CREATE INDEX IF NOT EXISTS idx_ue_orders_ordered_at ON ue_orders(ordered_at)`,
	`CREATE INDEX IF NOT EXISTS idx_ue_orders_restaurant ON ue_orders(restaurant_name)`,
	`CREATE INDEX IF NOT EXISTS idx_ue_items_title ON ue_items(title)`,
	`CREATE INDEX IF NOT EXISTS idx_ue_items_order ON ue_items(order_id)`,
}

// Mirror wraps the shared SQLite handle with order-history queries.
type Mirror struct {
	db *sql.DB
}

// NewMirror binds the order queries to an open database.
func NewMirror(db *sql.DB) *Mirror { return &Mirror{db: db} }

// KnownIDs returns all stored order ids.
func (m *Mirror) KnownIDs(ctx context.Context) (map[string]struct{}, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id FROM ue_orders`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// UpsertOrders replaces items for each order id and returns (orders, item rows).
func (m *Mirror) UpsertOrders(ctx context.Context, orders []Order, rawByID map[string][]byte) (int, int, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	upserted, itemRows := 0, 0
	for _, o := range orders {
		raw := []byte("{}")
		if r, ok := rawByID[o.ID]; ok && len(r) > 0 {
			raw = r
		}
		ordered := ""
		if !o.OrderedAt.IsZero() {
			ordered = o.OrderedAt.UTC().Format(time.RFC3339)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO ue_orders (id, workflow_uuid, restaurant_uuid, restaurant_name, currency, total_cents, ordered_at, status, raw_json, synced_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  workflow_uuid=excluded.workflow_uuid,
  restaurant_uuid=excluded.restaurant_uuid,
  restaurant_name=excluded.restaurant_name,
  currency=excluded.currency,
  total_cents=excluded.total_cents,
  ordered_at=excluded.ordered_at,
  status=excluded.status,
  raw_json=excluded.raw_json,
  synced_at=excluded.synced_at`,
			o.ID, o.WorkflowUUID, o.RestaurantUUID, o.RestaurantName, o.Currency, o.TotalCents, ordered, o.Status, string(raw), now); err != nil {
			return 0, 0, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM ue_items WHERE order_id = ?`, o.ID); err != nil {
			return 0, 0, err
		}
		for _, it := range o.Items {
			ib, _ := json.Marshal(it)
			if _, err := tx.ExecContext(ctx, `
INSERT INTO ue_items (order_id, item_uuid, title, quantity, unit_cents, raw_json)
VALUES (?, ?, ?, ?, ?, ?)`, o.ID, it.UUID, it.Title, it.Quantity, it.UnitCents, string(ib)); err != nil {
				return 0, 0, err
			}
			itemRows++
		}
		upserted++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return upserted, itemRows, nil
}

const orderColumns = `id, workflow_uuid, restaurant_uuid, restaurant_name, currency, total_cents, ordered_at, status, synced_at`

// ListOrders returns orders newest first, optionally bounded by date.
func (m *Mirror) ListOrders(ctx context.Context, limit int, since, until *time.Time, includeItems bool) ([]Order, error) {
	q := `SELECT ` + orderColumns + ` FROM ue_orders WHERE 1=1`
	var args []any
	if since != nil {
		q += ` AND ordered_at >= ?`
		args = append(args, since.UTC().Format(time.RFC3339))
	}
	if until != nil {
		q += ` AND ordered_at <= ?`
		args = append(args, until.UTC().Format(time.RFC3339))
	}
	q += ` ORDER BY ordered_at DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanOrders(rows)
	if err != nil {
		return nil, err
	}
	if includeItems {
		for i := range out {
			items, err := m.itemsFor(ctx, out[i].ID)
			if err != nil {
				return nil, err
			}
			out[i].Items = items
		}
	}
	return out, nil
}

// GetOrder returns one order with items, or ErrNotFound.
func (m *Mirror) GetOrder(ctx context.Context, id string) (Order, error) {
	row := m.db.QueryRowContext(ctx, `SELECT `+orderColumns+` FROM ue_orders WHERE id = ?`, id)
	o, err := scanOrder(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, err
	}
	items, err := m.itemsFor(ctx, id)
	if err != nil {
		return Order{}, err
	}
	o.Items = items
	return o, nil
}

func (m *Mirror) itemsFor(ctx context.Context, orderID string) ([]Item, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT item_uuid, title, quantity, unit_cents FROM ue_items WHERE order_id = ? ORDER BY id`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		var it Item
		var uuid sql.NullString
		if err := rows.Scan(&uuid, &it.Title, &it.Quantity, &it.UnitCents); err != nil {
			return nil, err
		}
		it.UUID = uuid.String
		items = append(items, it)
	}
	return items, rows.Err()
}

// Spend groups total_cents by month or year.
func (m *Mirror) Spend(ctx context.Context, by string) ([]SpendRow, error) {
	var expr string
	switch by {
	case "month":
		expr = `substr(ordered_at, 1, 7)`
	case "year":
		expr = `substr(ordered_at, 1, 4)`
	default:
		return nil, fmt.Errorf("--by must be month or year")
	}
	rows, err := m.db.QueryContext(ctx, fmt.Sprintf(`
SELECT COALESCE(NULLIF(%s, ''), 'unknown'), COUNT(*), SUM(total_cents), MAX(currency)
FROM ue_orders GROUP BY 1 ORDER BY 1 DESC`, expr))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SpendRow{}
	for rows.Next() {
		var r SpendRow
		var cur sql.NullString
		if err := rows.Scan(&r.Bucket, &r.Orders, &r.TotalCents, &cur); err != nil {
			return nil, err
		}
		r.Currency = cur.String
		r.Total = FormatMoney(r.TotalCents)
		out = append(out, r)
	}
	return out, rows.Err()
}

// TopItems ranks line items by quantity across all orders.
func (m *Mirror) TopItems(ctx context.Context, limit int) ([]Ranked, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := m.db.QueryContext(ctx, `
SELECT title, COUNT(DISTINCT order_id), SUM(quantity), SUM(quantity * unit_cents)
FROM ue_items
GROUP BY lower(title)
ORDER BY SUM(quantity) DESC, title ASC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRanked(rows, true)
}

// TopRestaurants ranks restaurants by order count.
func (m *Mirror) TopRestaurants(ctx context.Context, limit int) ([]Ranked, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := m.db.QueryContext(ctx, `
SELECT restaurant_name, COUNT(*), 0, SUM(total_cents)
FROM ue_orders
GROUP BY lower(restaurant_name)
ORDER BY COUNT(*) DESC, restaurant_name ASC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRanked(rows, false)
}

// Find searches restaurant names and item titles, case-insensitively.
func (m *Mirror) Find(ctx context.Context, query string, limit int) ([]Order, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("find requires a query")
	}
	if limit <= 0 {
		limit = 50
	}
	like := "%" + escapeLike(q) + "%"
	rows, err := m.db.QueryContext(ctx, `
SELECT DISTINCT o.id, o.workflow_uuid, o.restaurant_uuid, o.restaurant_name, o.currency, o.total_cents, o.ordered_at, o.status, o.synced_at
FROM ue_orders o
LEFT JOIN ue_items i ON i.order_id = o.id
WHERE o.restaurant_name LIKE ? ESCAPE '\' COLLATE NOCASE
   OR i.title LIKE ? ESCAPE '\' COLLATE NOCASE
ORDER BY o.ordered_at DESC
LIMIT ?`, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanOrders(rows)
	if err != nil {
		return nil, err
	}
	for i := range out {
		items, err := m.itemsFor(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Items = items
	}
	return out, nil
}

// Count returns the number of mirrored orders.
func (m *Mirror) Count(ctx context.Context) (int, error) {
	var n int
	err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ue_orders`).Scan(&n)
	return n, err
}

// escapeLike neutralizes LIKE wildcards so a user query matches literally.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOrders(rows *sql.Rows) ([]Order, error) {
	out := []Order{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func scanOrder(row rowScanner) (Order, error) {
	var o Order
	var workflow, restaurantUUID, ordered, status, synced sql.NullString
	if err := row.Scan(&o.ID, &workflow, &restaurantUUID, &o.RestaurantName, &o.Currency, &o.TotalCents, &ordered, &status, &synced); err != nil {
		return Order{}, err
	}
	o.WorkflowUUID = workflow.String
	o.RestaurantUUID = restaurantUUID.String
	o.Status = status.String
	o.Total = FormatMoney(o.TotalCents)
	if ordered.String != "" {
		if t, err := time.Parse(time.RFC3339, ordered.String); err == nil {
			o.OrderedAt = t
		}
	}
	if synced.String != "" {
		if t, err := time.Parse(time.RFC3339, synced.String); err == nil {
			o.SyncedAt = t
		}
	}
	return o, nil
}

func scanRanked(rows *sql.Rows, withQty bool) ([]Ranked, error) {
	out := []Ranked{}
	for rows.Next() {
		var r Ranked
		var total sql.NullInt64
		if err := rows.Scan(&r.Name, &r.Orders, &r.Quantity, &total); err != nil {
			return nil, err
		}
		r.TotalCents = total.Int64
		if !withQty {
			r.Quantity = 0
		}
		r.Total = FormatMoney(r.TotalCents)
		out = append(out, r)
	}
	return out, rows.Err()
}
