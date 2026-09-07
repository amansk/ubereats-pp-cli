// Copyright 2026 Amandeep Khurana and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: list and fetch mirrored orders.
// pp:data-source local
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"ubereats-pp-cli/internal/orders"
)

func newNovelHistoryCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "List synced orders newest-first, or show one order with its line items",
		Example: strings.Trim(`
  ubereats-pp-cli history list --since 2026-01-01 --limit 20 --agent
  ubereats-pp-cli history get 11111111-1111-1111-1111-111111111111 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newHistoryListCmd(flags))
	cmd.AddCommand(newHistoryGetCmd(flags))
	return cmd
}

func newHistoryListCmd(flags *rootFlags) *cobra.Command {
	var since, until string
	var limit int
	var withItems bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List mirrored orders newest first, with optional date bounds",
		Example: strings.Trim(`
  ubereats-pp-cli history list --limit 20 --agent
  ubereats-pp-cli history list --since 2026-01-01 --until 2026-03-31 --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:data-source":      "local",
			"pp:happy-args":       "--limit=5",
			"pp:typed-exit-codes": "0",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageErr(fmt.Errorf("history list takes no positional arguments (got %q); use --since/--until/--limit", args[0]))
			}
			if err := requireLocalSource(flags, "history list"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "read ue_orders from the local mirror")
			}
			s, err := parseDateBound("since", since, false)
			if err != nil {
				return err
			}
			u, err := parseDateBound("until", until, true)
			if err != nil {
				return err
			}
			st, mirror, err := openMirror(cmd.Context())
			if err != nil {
				return err
			}
			defer st.Close()
			list, err := mirror.ListOrders(cmd.Context(), limit, s, u, withItems || wantsHumanTable(cmd.OutOrStdout(), flags))
			if err != nil {
				return err
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				rows := make([]map[string]any, 0, len(list))
				for _, o := range list {
					r := orderRow(o)
					delete(r, "total_cents")
					delete(r, "status")
					rows = append(rows, r)
				}
				return emitRows(cmd, flags, rows)
			}
			return flags.printJSON(cmd, list)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Inclusive lower bound on order date (YYYY-MM-DD or RFC3339)")
	cmd.Flags().StringVar(&until, "until", "", "Inclusive upper bound on order date (YYYY-MM-DD or RFC3339)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum orders to return (0 for all)")
	cmd.Flags().BoolVar(&withItems, "items", false, "Include line items in JSON output")
	return cmd
}

func newHistoryGetCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [order-id]",
		Short: "Show one mirrored order with its line items",
		Example: strings.Trim(`
  ubereats-pp-cli history get 11111111-1111-1111-1111-111111111111 --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:data-source":      "local",
			"pp:happy-args":       "order-id=11111111-1111-1111-1111-111111111111",
			"pp:typed-exit-codes": "0,3",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !hasChangedLocalFlags(cmd) && !flags.dryRun {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "read one ue_orders row and its ue_items from the local mirror")
			}
			if len(args) != 1 {
				return usageErr(fmt.Errorf("history get requires exactly one <order-id> (from `history list`)"))
			}
			if err := requireLocalSource(flags, "history get"); err != nil {
				return err
			}
			st, mirror, err := openMirror(cmd.Context())
			if err != nil {
				return err
			}
			defer st.Close()
			o, err := mirror.GetOrder(cmd.Context(), args[0])
			if errors.Is(err, orders.ErrNotFound) {
				return notFoundErr(fmt.Errorf("order %s not in the local mirror; run `ubereats-pp-cli sync` or check `history list`", args[0]))
			}
			if err != nil {
				return err
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				when := ""
				if !o.OrderedAt.IsZero() {
					when = o.OrderedAt.UTC().Format("2006-01-02 15:04")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s  %s %s\n", o.ID, when, o.RestaurantName, orDefault(o.Currency, "USD"), o.Total)
				if len(o.Items) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "(no line items parsed; raw payload kept in the mirror)")
					return nil
				}
				rows := make([]map[string]any, 0, len(o.Items))
				for _, it := range o.Items {
					rows = append(rows, map[string]any{"item": it.Title, "qty": it.Quantity, "unit": orders.FormatMoney(it.UnitCents)})
				}
				return printAutoTable(cmd.OutOrStdout(), rows)
			}
			return flags.printJSON(cmd, o)
		},
	}
	return cmd
}
