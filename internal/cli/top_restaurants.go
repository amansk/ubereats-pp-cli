// Copyright 2026 Amandeep Khurana and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: most-ordered restaurants from the local mirror.
// pp:data-source local
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelTopRestaurantsCmd(flags *rootFlags) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "top-restaurants",
		Short: "Rank restaurants by order count and total spend",
		Example: strings.Trim(`
  ubereats-pp-cli top-restaurants --limit 10 --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:data-source":      "local",
			"pp:happy-args":       "--limit=5",
			"pp:typed-exit-codes": "0",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageErr(fmt.Errorf("top-restaurants takes no positional arguments (got %q); use --limit", args[0]))
			}
			if err := requireLocalSource(flags, "top-restaurants"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "rank ue_orders by restaurant in the local mirror")
			}
			st, mirror, err := openMirror(cmd.Context())
			if err != nil {
				return err
			}
			defer st.Close()
			rows, err := mirror.TopRestaurants(cmd.Context(), limit)
			if err != nil {
				return err
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				table := make([]map[string]any, 0, len(rows))
				for _, r := range rows {
					table = append(table, map[string]any{"restaurant": r.Name, "orders": r.Orders, "spend": r.Total})
				}
				return emitRows(cmd, flags, table)
			}
			return flags.printJSON(cmd, map[string]any{"restaurants": rows, "count": len(rows)})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum rows to return")
	return cmd
}
