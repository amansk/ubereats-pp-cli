package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePastOrdersFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "past_orders_page.json"))
	if err != nil {
		t.Fatal(err)
	}
	page, err := ParsePastOrders(raw)
	if err != nil {
		t.Fatal(err)
	}
	if page.HasMore {
		t.Fatal("hasMore should be false")
	}
	if len(page.Orders) != 3 {
		t.Fatalf("orders = %d, want 3", len(page.Orders))
	}

	byID := map[string]int{}
	var burgers int
	for _, o := range page.Orders {
		byID[o.ID]++
		if o.RestaurantName == "" {
			t.Fatalf("missing restaurant on %s", o.ID)
		}
		if o.TotalCents <= 0 {
			t.Fatalf("zero total on %s", o.ID)
		}
		for _, it := range o.Items {
			if it.Title == "Cheeseburger" {
				burgers += it.Quantity
			}
		}
	}
	if burgers != 3 {
		t.Fatalf("cheeseburger qty = %d, want 3", burgers)
	}

	var ramen bool
	for _, o := range page.Orders {
		if o.RestaurantName == "Ippudo" {
			if o.TotalCents != 2140 {
				t.Fatalf("ippudo total %d", o.TotalCents)
			}
			if len(o.Items) != 1 || o.Items[0].UnitCents != 1695 {
				t.Fatalf("ramen item = %+v", o.Items)
			}
			ramen = true
		}
	}
	if !ramen {
		t.Fatal("missing Ippudo")
	}
}

func TestParseFailureEnvelope(t *testing.T) {
	_, err := ParsePastOrders([]byte(`{"status":"failure","data":{}}`))
	if err == nil {
		t.Fatal("expected envelope error")
	}
}

func TestMoneyCentsHeuristic(t *testing.T) {
	if moneyCents(1299.0) != 1299 {
		t.Fatal("int cents")
	}
	if moneyCents(12.99) != 1299 {
		t.Fatal("float dollars")
	}
	if moneyCents("$3.50") != 350 {
		t.Fatal("dollar string")
	}
}
