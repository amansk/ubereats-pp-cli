// Package store is the local SQLite cache of synced orders.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
	"github.com/amansk/ubereats-pp-cli/internal/model"
	_ "modernc.org/sqlite"
)

const dbFile = "ubereats.db"

// DB wraps the sqlite connection.
type DB struct {
	sql  *sql.DB
	Path string
}

// Open creates/migrates $home/ubereats.db.
func Open(home string) (*DB, error) {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, fmt.Errorf("store home: %w", err)
	}
	path := filepath.Join(home, dbFile)
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(`PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;`); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("pragma: %w", err)
	}
	d := &DB{sql: sqlDB, Path: path}
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error { return d.sql.Close() }

func (d *DB) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS orders (
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
);
CREATE TABLE IF NOT EXISTS items (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  order_id    TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  item_uuid   TEXT,
  title       TEXT NOT NULL DEFAULT '',
  quantity    INTEGER NOT NULL DEFAULT 1,
  unit_cents  INTEGER NOT NULL DEFAULT 0,
  raw_json    TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sync_state (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_orders_ordered_at ON orders(ordered_at);
CREATE INDEX IF NOT EXISTS idx_orders_restaurant ON orders(restaurant_name);
CREATE INDEX IF NOT EXISTS idx_items_title ON items(title);
CREATE INDEX IF NOT EXISTS idx_items_order ON items(order_id);
`
	if _, err := d.sql.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if _, err := d.sql.Exec(`PRAGMA user_version = 1`); err != nil {
		return err
	}
	return nil
}

// KnownIDs returns all stored order ids.
func (d *DB) KnownIDs() (map[string]struct{}, error) {
	rows, err := d.sql.Query(`SELECT id FROM orders`)
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

// UpsertOrders replaces items for each order id.
func (d *DB) UpsertOrders(orders []model.Order, rawByID map[string][]byte) (int, int, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	upserted := 0
	itemRows := 0
	for _, o := range orders {
		raw := []byte("{}")
		if r, ok := rawByID[o.ID]; ok && len(r) > 0 {
			raw = r
		} else {
			b, err := json.Marshal(o)
			if err == nil {
				raw = b
			}
		}
		ordered := ""
		if !o.OrderedAt.IsZero() {
			ordered = o.OrderedAt.UTC().Format(time.RFC3339)
		}
		_, err := tx.Exec(`
INSERT INTO orders (id, workflow_uuid, restaurant_uuid, restaurant_name, currency, total_cents, ordered_at, status, raw_json, synced_at)
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
  synced_at=excluded.synced_at
`, o.ID, o.WorkflowUUID, o.RestaurantUUID, o.RestaurantName, o.Currency, o.TotalCents, ordered, o.Status, string(raw), now)
		if err != nil {
			return 0, 0, err
		}
		if _, err := tx.Exec(`DELETE FROM items WHERE order_id = ?`, o.ID); err != nil {
			return 0, 0, err
		}
		for _, it := range o.Items {
			ib, _ := json.Marshal(it)
			if _, err := tx.Exec(`
INSERT INTO items (order_id, item_uuid, title, quantity, unit_cents, raw_json)
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

func (d *DB) SetState(key, value string) error {
	_, err := d.sql.Exec(`INSERT INTO sync_state(key, value) VALUES(?, ?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (d *DB) State(key string) (string, error) {
	var v string
	err := d.sql.QueryRow(`SELECT value FROM sync_state WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// ListOrders returns orders newest first. includeItems attaches line items.
func (d *DB) ListOrders(limit int, since, until *time.Time, includeItems bool) ([]model.Order, error) {
	q := `SELECT id, workflow_uuid, restaurant_uuid, restaurant_name, currency, total_cents, ordered_at, status, synced_at FROM orders WHERE 1=1`
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
	rows, err := d.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if includeItems {
		for i := range out {
			items, err := d.itemsFor(out[i].ID)
			if err != nil {
				return nil, err
			}
			out[i].Items = items
		}
	}
	return out, nil
}

func (d *DB) GetOrder(id string) (model.Order, error) {
	row := d.sql.QueryRow(`SELECT id, workflow_uuid, restaurant_uuid, restaurant_name, currency, total_cents, ordered_at, status, synced_at FROM orders WHERE id = ?`, id)
	o, err := scanOrder(row)
	if err == sql.ErrNoRows {
		return model.Order{}, exitcode.NotFoundf("order %s not found", id)
	}
	if err != nil {
		return model.Order{}, err
	}
	items, err := d.itemsFor(id)
	if err != nil {
		return model.Order{}, err
	}
	o.Items = items
	return o, nil
}

func (d *DB) itemsFor(orderID string) ([]model.Item, error) {
	rows, err := d.sql.Query(`SELECT item_uuid, title, quantity, unit_cents FROM items WHERE order_id = ?`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.Item
	for rows.Next() {
		var it model.Item
		if err := rows.Scan(&it.UUID, &it.Title, &it.Quantity, &it.UnitCents); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (d *DB) Spend(by string) ([]model.SpendRow, error) {
	var expr string
	switch by {
	case "month":
		expr = `substr(ordered_at, 1, 7)`
	case "year":
		expr = `substr(ordered_at, 1, 4)`
	default:
		return nil, exitcode.Usagef("--by must be month or year")
	}
	rows, err := d.sql.Query(fmt.Sprintf(`
SELECT COALESCE(%s, 'unknown'), COUNT(*), SUM(total_cents), MAX(currency)
FROM orders GROUP BY 1 ORDER BY 1 DESC`, expr))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.SpendRow
	for rows.Next() {
		var r model.SpendRow
		if err := rows.Scan(&r.Bucket, &r.Orders, &r.TotalCents, &r.Currency); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) TopItems(limit int) ([]model.Ranked, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := d.sql.Query(`
SELECT title, COUNT(DISTINCT order_id), SUM(quantity), SUM(quantity * unit_cents)
FROM items
GROUP BY lower(title)
ORDER BY SUM(quantity) DESC, title ASC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRanked(rows, true)
}

func (d *DB) TopRestaurants(limit int) ([]model.Ranked, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := d.sql.Query(`
SELECT restaurant_name, COUNT(*), 0, SUM(total_cents)
FROM orders
GROUP BY lower(restaurant_name)
ORDER BY COUNT(*) DESC, restaurant_name ASC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRanked(rows, false)
}

func (d *DB) Find(query string, limit int) ([]model.Order, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, exitcode.Usagef("find requires a query")
	}
	if limit <= 0 {
		limit = 50
	}
	like := "%" + q + "%"
	rows, err := d.sql.Query(`
SELECT DISTINCT o.id, o.workflow_uuid, o.restaurant_uuid, o.restaurant_name, o.currency, o.total_cents, o.ordered_at, o.status, o.synced_at
FROM orders o
LEFT JOIN items i ON i.order_id = o.id
WHERE o.restaurant_name LIKE ? COLLATE NOCASE
   OR i.title LIKE ? COLLATE NOCASE
ORDER BY o.ordered_at DESC
LIMIT ?`, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		items, err := d.itemsFor(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Items = items
	}
	return out, nil
}

func (d *DB) Count() (int, error) {
	var n int
	err := d.sql.QueryRow(`SELECT COUNT(*) FROM orders`).Scan(&n)
	return n, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOrder(row rowScanner) (model.Order, error) {
	var o model.Order
	var ordered, synced string
	if err := row.Scan(&o.ID, &o.WorkflowUUID, &o.RestaurantUUID, &o.RestaurantName, &o.Currency, &o.TotalCents, &ordered, &o.Status, &synced); err != nil {
		return model.Order{}, err
	}
	if ordered != "" {
		if t, err := time.Parse(time.RFC3339, ordered); err == nil {
			o.OrderedAt = t
		}
	}
	if synced != "" {
		if t, err := time.Parse(time.RFC3339, synced); err == nil {
			o.SyncedAt = t
		}
	}
	return o, nil
}

func scanRanked(rows *sql.Rows, withQty bool) ([]model.Ranked, error) {
	var out []model.Ranked
	for rows.Next() {
		var r model.Ranked
		if err := rows.Scan(&r.Name, &r.Orders, &r.Quantity, &r.TotalCents); err != nil {
			return nil, err
		}
		if !withQty {
			r.Quantity = 0
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
