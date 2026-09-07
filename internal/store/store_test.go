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
