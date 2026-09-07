// Copyright 2026 Amandeep Khurana and contributors. Licensed under Apache-2.0. See LICENSE.

// Shared plumbing for the hand-written order-history commands. Everything
// here reads the local mirror (internal/orders) through the shared store.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"ubereats-pp-cli/internal/orders"
	"ubereats-pp-cli/internal/store"
)

// openMirror opens the shared SQLite store and binds the order mirror to it.
func openMirror(ctx context.Context) (*store.Store, *orders.Mirror, error) {
	s, err := store.OpenWithContext(ctx, learnDBPath(""))
	if err != nil {
		return nil, nil, configErr(fmt.Errorf("open local store: %w", err))
	}
	return s, orders.NewMirror(s.DB()), nil
}

// requireLocalSource rejects --data-source live for commands that only read
// the local mirror. auto and local are both fine.
func requireLocalSource(flags *rootFlags, command string) error {
	if strings.EqualFold(strings.TrimSpace(flags.dataSource), "live") {
		return usageErr(fmt.Errorf("%s reads the local mirror only; run `ubereats-pp-cli sync` first and drop --data-source live", command))
	}
	return nil
}

// emitRows prints v as machine output, or a human table built from the
// same rows when stdout is a terminal and no machine format was requested.
func emitRows(cmd *cobra.Command, flags *rootFlags, v any) error {
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		var items []map[string]any
		if json.Unmarshal(raw, &items) == nil {
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no rows (run `ubereats-pp-cli sync` to mirror your order history)")
				return nil
			}
			return printAutoTable(cmd.OutOrStdout(), items)
		}
	}
	return flags.printJSON(cmd, v)
}

// orderRow flattens an order for tables and compact output.
func orderRow(o orders.Order) map[string]any {
	when := ""
	if !o.OrderedAt.IsZero() {
		when = o.OrderedAt.UTC().Format("2006-01-02")
	}
	itemTitles := make([]string, 0, len(o.Items))
	for _, it := range o.Items {
		itemTitles = append(itemTitles, it.Title)
	}
	return map[string]any{
		"id":          o.ID,
		"date":        when,
		"restaurant":  o.RestaurantName,
		"total":       o.Total,
		"currency":    orDefault(o.Currency, "USD"),
		"status":      o.Status,
		"items":       strings.Join(itemTitles, ", "),
		"total_cents": o.TotalCents,
	}
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// parseDateBound parses YYYY-MM-DD or RFC3339. A date-only until bound is
// inclusive of the whole UTC calendar day.
func parseDateBound(flag, s string, until bool) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		u := t.UTC()
		if until {
			u = u.Add(24*time.Hour - time.Nanosecond)
		}
		return &u, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		u := t.UTC()
		return &u, nil
	}
	return nil, usageErr(fmt.Errorf("invalid --%s %q (use YYYY-MM-DD or RFC3339)", flag, s))
}
