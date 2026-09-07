package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amansk/ubereats-pp-cli/internal/auth"
	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
)

func TestGetPastOrdersAgainstFixtureServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_p/api/getPastOrdersV1" {
			t.Fatalf("path %s", r.URL.Path)
		}
		if r.Header.Get("x-csrf-token") != "x" {
			t.Fatal("missing csrf")
		}
		if r.Header.Get("Cookie") == "" {
			t.Fatal("missing cookie")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"meta": map[string]any{"hasMore": false},
				"ordersMap": map[string]any{
					"o1": map[string]any{
						"baseEaterOrder": map[string]any{
							"uuid":         "o1",
							"currencyCode": "USD",
							"completedAt":  "2026-01-01T00:00:00Z",
						},
						"storeInfo": map[string]any{"title": "Cafe"},
						"fareInfo":  map[string]any{"totalPrice": 100},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := New(auth.Store{Cookies: map[string]string{"sid": "test"}}, srv.URL)
	page, err := c.GetPastOrders(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Orders) != 1 || page.Orders[0].RestaurantName != "Cafe" {
		t.Fatalf("page = %+v", page.Orders)
	}
}

func TestAuthErrorOn401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":"failure"}`))
	}))
	defer srv.Close()
	c := New(auth.Store{Cookies: map[string]string{"sid": "dead"}}, srv.URL)
	_, err := c.GetPastOrders(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	var ex *exitcode.Error
	if !exitcode.As(err, &ex) || ex.Code != exitcode.Auth {
		t.Fatalf("want auth, got %v", err)
	}
}

func TestHTMLIsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html>challenge</html>"))
	}))
	defer srv.Close()
	c := New(auth.Store{Cookies: map[string]string{"sid": "x"}}, srv.URL)
	_, err := c.GetPastOrders(context.Background(), "")
	if err == nil {
		t.Fatal("expected html error")
	}
}
