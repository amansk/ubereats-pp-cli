// Package model is the normalized buyer-order view stored in SQLite.
package model

import (
	"fmt"
	"time"
)

// Order is a completed (or historically listed) Eats order.
type Order struct {
	ID             string    `json:"id"`
	WorkflowUUID   string    `json:"workflow_uuid,omitempty"`
	RestaurantUUID string    `json:"restaurant_uuid,omitempty"`
	RestaurantName string    `json:"restaurant_name"`
	Currency       string    `json:"currency"`
	TotalCents     int64     `json:"total_cents"`
	OrderedAt      time.Time `json:"ordered_at"`
	Status         string    `json:"status,omitempty"`
	Items          []Item    `json:"items,omitempty"`
	SyncedAt       time.Time `json:"synced_at,omitempty"`
}

// Item is a line from shoppingCart.items or a reconstructed equivalent.
type Item struct {
	UUID      string `json:"item_uuid,omitempty"`
	Title     string `json:"title"`
	Quantity  int    `json:"quantity"`
	UnitCents int64  `json:"unit_cents"`
}

// SpendRow is a grouped spend bucket.
type SpendRow struct {
	Bucket     string `json:"bucket"`
	Orders     int    `json:"orders"`
	TotalCents int64  `json:"total_cents"`
	Currency   string `json:"currency,omitempty"`
}

// Ranked is a top-items / top-restaurants row.
type Ranked struct {
	Name       string `json:"name"`
	Orders     int    `json:"orders"`
	Quantity   int    `json:"quantity,omitempty"`
	TotalCents int64  `json:"total_cents"`
}

// SyncResult is returned by sync.
type SyncResult struct {
	Mode         string `json:"mode"`
	Pages        int    `json:"pages"`
	Upserted     int    `json:"upserted"`
	SkippedKnown int    `json:"skipped_known"`
	ItemRows     int    `json:"item_rows"`
	Source       string `json:"source"`
	StoppedEarly bool   `json:"stopped_early,omitempty"`
}

// FormatMoney renders cents as a decimal string without a symbol.
func FormatMoney(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	s := fmt.Sprintf("%d.%02d", cents/100, cents%100)
	if neg {
		return "-" + s
	}
	return s
}
