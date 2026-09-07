package store

import (
	"testing"
	"time"

	"github.com/amansk/ubereats-pp-cli/internal/model"
)

func TestUpsertListSpendFind(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	t1 := time.Date(2026, 3, 15, 19, 22, 0, 0, time.UTC)
	t2 := time.Date(2025, 12, 1, 12, 10, 0, 0, time.UTC)
	orders := []model.Order{
		{
			ID: "a", RestaurantName: "Shake Shack", Currency: "USD",
			TotalCents: 2899, OrderedAt: t1,
			Items: []model.Item{{Title: "Cheeseburger", Quantity: 2, UnitCents: 1299}},
		},
		{
			ID: "b", RestaurantName: "Ippudo", Currency: "USD",
			TotalCents: 2140, OrderedAt: t2,
			Items: []model.Item{{Title: "Tonkotsu Ramen", Quantity: 1, UnitCents: 1695}},
		},
	}
	if _, _, err := db.UpsertOrders(orders, nil); err != nil {
		t.Fatal(err)
	}

	list, err := db.ListOrders(10, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "a" {
		t.Fatalf("list = %+v", list)
	}

	got, err := db.GetOrder("b")
	if err != nil {
		t.Fatal(err)
	}
	if got.RestaurantName != "Ippudo" || len(got.Items) != 1 {
		t.Fatalf("get = %+v", got)
	}

	year, err := db.Spend("year")
	if err != nil {
		t.Fatal(err)
	}
	if len(year) != 2 {
		t.Fatalf("year rows = %+v", year)
	}

	topI, err := db.TopItems(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(topI) == 0 || topI[0].Name != "Cheeseburger" {
		t.Fatalf("top items = %+v", topI)
	}

	topR, err := db.TopRestaurants(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(topR) != 2 {
		t.Fatalf("top restaurants = %+v", topR)
	}

	found, err := db.Find("ramen", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != "b" {
		t.Fatalf("find = %+v", found)
	}

	// upsert is idempotent
	if _, _, err := db.UpsertOrders(orders[:1], nil); err != nil {
		t.Fatal(err)
	}
	n, err := db.Count()
	if err != nil || n != 2 {
		t.Fatalf("count = %d %v", n, err)
	}
}

func TestUntilIncludesWholeUTCDay(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	when := time.Date(2026, 3, 15, 19, 22, 0, 0, time.UTC)
	orders := []model.Order{{
		ID: "mid-day", RestaurantName: "Shake Shack", Currency: "USD",
		TotalCents: 100, OrderedAt: when,
	}}
	if _, _, err := db.UpsertOrders(orders, map[string][]byte{"mid-day": []byte(`{"wire":true}`)}); err != nil {
		t.Fatal(err)
	}
	midnight := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	got, err := db.ListOrders(10, nil, &midnight, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("midnight until should exclude evening order, got %+v", got)
	}
	end := midnight.Add(24*time.Hour - time.Nanosecond)
	got, err = db.ListOrders(10, nil, &end, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "mid-day" {
		t.Fatalf("end-of-day until should include 2026-03-15: %+v", got)
	}
	raw, err := db.OrderRawJSON("mid-day")
	if err != nil {
		t.Fatal(err)
	}
	if raw != `{"wire":true}` {
		t.Fatalf("raw_json = %s", raw)
	}
}

func TestGetMissing(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.GetOrder("nope")
	if err == nil {
		t.Fatal("expected not found")
	}
}

func TestFindEscapesLikeWildcards(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	orders := []model.Order{
		{ID: "a", RestaurantName: "100% Taqueria", OrderedAt: time.Now()},
		{ID: "b", RestaurantName: "Plain Diner", OrderedAt: time.Now()},
	}
	if _, _, err := db.UpsertOrders(orders, nil); err != nil {
		t.Fatal(err)
	}
	got, err := db.Find("%", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("want only the literal %% match, got %+v", got)
	}
	got, err = db.Find("_", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("underscore should not act as a wildcard, got %+v", got)
	}
}
