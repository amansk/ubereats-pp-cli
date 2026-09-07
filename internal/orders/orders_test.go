// Copyright 2026 Amandeep Khurana and contributors. Licensed under Apache-2.0. See LICENSE.

package orders

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "past_orders_page.json"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParsePastOrdersFixture(t *testing.T) {
	page, err := ParsePastOrders(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Orders) != 3 {
		t.Fatalf("want 3 orders, got %d", len(page.Orders))
	}
	if page.HasMore {
		t.Fatal("fixture says hasMore=false")
	}
	byID := map[string]Order{}
	for _, o := range page.Orders {
		byID[o.ID] = o
	}
	shake := byID["11111111-1111-1111-1111-111111111111"]
	if shake.RestaurantName != "Shake Shack" || shake.TotalCents != 2899 || shake.Currency != "USD" {
		t.Fatalf("unexpected order: %+v", shake)
	}
	if len(shake.Items) != 2 || shake.Items[0].Title != "Cheeseburger" || shake.Items[0].Quantity != 2 || shake.Items[0].UnitCents != 1299 {
		t.Fatalf("unexpected items: %+v", shake.Items)
	}
	ramen := byID["22222222-2222-2222-2222-222222222222"]
	if len(ramen.Items) != 1 || ramen.Items[0].UnitCents != 1695 || ramen.Items[0].Title != "Tonkotsu Ramen" {
		t.Fatalf("nested price / alt field names not handled: %+v", ramen.Items)
	}
	if len(page.RawByID) != 3 {
		t.Fatalf("raw wire blobs missing: %d", len(page.RawByID))
	}
}

func TestParseRejectsFailureEnvelope(t *testing.T) {
	if _, err := ParsePastOrders([]byte(`{"status":"failure","data":{}}`)); err == nil {
		t.Fatal("expected envelope failure")
	}
}

func TestMoneyAndTime(t *testing.T) {
	if got := moneyCents(12.34); got != 1234 {
		t.Fatalf("float dollars: %d", got)
	}
	if got := moneyCents("$5.00"); got != 500 {
		t.Fatalf("dollar string: %d", got)
	}
	if got := moneyCents(float64(2899)); got != 2899 {
		t.Fatalf("integer cents: %d", got)
	}
	if ts, ok := parseTime(float64(1710530520000)); !ok || ts.Year() != 2024 {
		t.Fatalf("millis: %v %v", ts, ok)
	}
	if FormatMoney(-105) != "-1.05" {
		t.Fatal("negative money")
	}
}

func openTestMirror(t *testing.T) *Mirror {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	for _, m := range Migrations {
		if _, err := db.Exec(m); err != nil {
			t.Fatal(err)
		}
	}
	return NewMirror(db)
}

func TestMirrorQueries(t *testing.T) {
	ctx := context.Background()
	m := openTestMirror(t)
	page, err := ParsePastOrders(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if n, items, err := m.UpsertOrders(ctx, page.Orders, page.RawByID); err != nil || n != 3 || items != 4 {
		t.Fatalf("upsert: n=%d items=%d err=%v", n, items, err)
	}
	// Re-upsert is idempotent.
	if n, items, err := m.UpsertOrders(ctx, page.Orders, page.RawByID); err != nil || n != 3 || items != 4 {
		t.Fatalf("re-upsert: n=%d items=%d err=%v", n, items, err)
	}
	if c, _ := m.Count(ctx); c != 3 {
		t.Fatalf("count %d", c)
	}

	list, err := m.ListOrders(ctx, 10, nil, nil, false)
	if err != nil || len(list) != 3 || list[0].ID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("list newest first: %v %+v", err, list)
	}
	since := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	list, _ = m.ListOrders(ctx, 10, &since, nil, false)
	if len(list) != 1 {
		t.Fatalf("since bound: %d", len(list))
	}

	o, err := m.GetOrder(ctx, "22222222-2222-2222-2222-222222222222")
	if err != nil || len(o.Items) != 1 || o.Total != "21.40" {
		t.Fatalf("get: %v %+v", err, o)
	}
	if _, err := m.GetOrder(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	spend, err := m.Spend(ctx, "year")
	if err != nil || len(spend) != 2 || spend[0].Bucket != "2026" || spend[0].TotalCents != 4649 {
		t.Fatalf("spend: %v %+v", err, spend)
	}
	if _, err := m.Spend(ctx, "week"); err == nil {
		t.Fatal("spend by week should be rejected")
	}

	items, err := m.TopItems(ctx, 5)
	if err != nil || len(items) != 3 || items[0].Name != "Cheeseburger" || items[0].Quantity != 3 || items[0].Orders != 2 {
		t.Fatalf("top items: %v %+v", err, items)
	}
	rest, err := m.TopRestaurants(ctx, 5)
	if err != nil || len(rest) != 2 || rest[0].Name != "Shake Shack" || rest[0].Orders != 2 {
		t.Fatalf("top restaurants: %v %+v", err, rest)
	}

	found, err := m.Find(ctx, "RAMEN", 10)
	if err != nil || len(found) != 1 || found[0].RestaurantName != "Ippudo" {
		t.Fatalf("find: %v %+v", err, found)
	}
	found, _ = m.Find(ctx, "%", 10)
	if len(found) != 0 {
		t.Fatalf("LIKE wildcard must be literal, got %d", len(found))
	}
	if _, err := m.Find(ctx, "  ", 10); err == nil {
		t.Fatal("empty query should error")
	}
}
