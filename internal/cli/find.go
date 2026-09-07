// Copyright 2026 Amandeep Khurana and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: offline text search over the local mirror.
// pp:data-source local
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelFindCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var query string

	cmd := &cobra.Command{
		Use:   "find [query]",
		Short: "Search restaurant names and item titles in the local mirror",
		Long: `Case-insensitive substring search across mirrored restaurant names and
line-item titles. Pass the query as positional words or with --query.`,
		Example: strings.Trim(`
  ubereats-pp-cli find ramen --agent
  ubereats-pp-cli find "shake shack" --limit 5 --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:data-source":      "local",
			"pp:happy-args":       "query=burger",
			"pp:typed-exit-codes": "0",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			q := strings.TrimSpace(strings.Join(args, " "))
			if q == "" {
				q = strings.TrimSpace(query)
			}
			if q == "" && !hasChangedLocalFlags(cmd) && !flags.dryRun {
				return cmd.Help()
			}
			if err := requireLocalSource(flags, "find"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "LIKE search over ue_orders.restaurant_name and ue_items.title")
			}
			if q == "" {
				return usageErr(fmt.Errorf("find requires a query: `ubereats-pp-cli find ramen` or --query ramen"))
			}
			st, mirror, err := openMirror(cmd.Context())
			if err != nil {
				return err
			}
			defer st.Close()
			list, err := mirror.Find(cmd.Context(), q, limit)
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
			return flags.printJSON(cmd, map[string]any{"query": q, "orders": list, "count": len(list)})
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Search text (alternative to the positional query)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum orders to return")
	return cmd
}
