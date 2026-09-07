// Copyright 2026 Amandeep Khurana and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: spend by month or year from the local mirror.
// pp:data-source local
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"ubereats-pp-cli/internal/orders"
)

func newNovelSpendCmd(flags *rootFlags) *cobra.Command {
	var by string

	cmd := &cobra.Command{
		Use:   "spend",
		Short: "Sum mirrored order totals into monthly or yearly buckets",
		Example: strings.Trim(`
  ubereats-pp-cli spend --by month --agent
  ubereats-pp-cli spend --by year --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:data-source":      "local",
			"pp:happy-args":       "--by=month",
			"pp:typed-exit-codes": "0",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageErr(fmt.Errorf("spend takes no positional arguments (got %q); use --by month|year", args[0]))
			}
			by = strings.ToLower(strings.TrimSpace(by))
			if by != "month" && by != "year" {
				return usageErr(fmt.Errorf("--by must be month or year (got %q)", by))
			}
			if err := requireLocalSource(flags, "spend"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "aggregate ue_orders.total_cents by "+by)
			}
			st, mirror, err := openMirror(cmd.Context())
			if err != nil {
				return err
			}
			defer st.Close()
			rows, err := mirror.Spend(cmd.Context(), by)
			if err != nil {
				return err
			}
			var grand int64
			currency := ""
			for _, r := range rows {
				grand += r.TotalCents
				if currency == "" {
					currency = r.Currency
				}
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				table := make([]map[string]any, 0, len(rows))
				for _, r := range rows {
					table = append(table, map[string]any{"bucket": r.Bucket, "orders": r.Orders, "total": r.Total, "currency": orDefault(r.Currency, "USD")})
				}
				if err := emitRows(cmd, flags, table); err != nil {
					return err
				}
				if len(rows) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "grand total: %s %s over %d buckets\n", orDefault(currency, "USD"), orders.FormatMoney(grand), len(rows))
				}
				return nil
			}
			return flags.printJSON(cmd, map[string]any{
				"by":          by,
				"rows":        rows,
				"total_cents": grand,
				"total":       orders.FormatMoney(grand),
				"currency":    orDefault(currency, "USD"),
			})
		},
	}
	cmd.Flags().StringVar(&by, "by", "month", "Bucket size: month or year")
	return cmd
}
