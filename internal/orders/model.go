// Copyright 2026 Amandeep Khurana and contributors. Licensed under Apache-2.0. See LICENSE.

package orders

import (
	"encoding/json"
	"fmt"
	"strings"
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
	Total          string    `json:"total"`
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
	Total      string `json:"total"`
	Currency   string `json:"currency,omitempty"`
}

// Ranked is a top-items / top-restaurants row.
type Ranked struct {
	Name       string `json:"name"`
	Orders     int    `json:"orders"`
	Quantity   int    `json:"quantity,omitempty"`
	TotalCents int64  `json:"total_cents"`
	Total      string `json:"total"`
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
	TotalOrders  int    `json:"total_orders"`
}

// Page is one getPastOrdersV1 response after tolerant parsing.
type Page struct {
	Orders     []Order
	HasMore    bool
	NextCursor string
	Raw        json.RawMessage
	RawByID    map[string][]byte // original per-order wire objects
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

// EnvelopeStatus extracts the top-level status when present.
func EnvelopeStatus(raw json.RawMessage) string {
	var env struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return ""
	}
	return env.Status
}

func failIfEnvelope(raw json.RawMessage, op string) error {
	st := EnvelopeStatus(raw)
	if st != "" && !strings.EqualFold(st, "success") {
		return fmt.Errorf("%s: envelope status %q", op, st)
	}
	return nil
}
