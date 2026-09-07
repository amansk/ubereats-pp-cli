package cli

import (
	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
	"github.com/amansk/ubereats-pp-cli/internal/model"
	"github.com/spf13/cobra"
)

func newSpendCmd(opt *Options) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "spend",
		Short: "Sum synced spend by month or year",
		RunE: func(cmd *cobra.Command, args []string) error {
			if by != "month" && by != "year" {
				return exitcode.Usagef("--by must be month or year")
			}
			db, err := mustOpenStore(opt)
			if err != nil {
				return err
			}
			defer db.Close()
			rows, err := db.Spend(by)
			if err != nil {
				return err
			}
			if rows == nil {
				rows = []model.SpendRow{}
			}
			table := make([][]string, 0, len(rows))
			var grand int64
			cur := ""
			for _, r := range rows {
				grand += r.TotalCents
				if cur == "" {
					cur = r.Currency
				}
				cc := r.Currency
				if cc == "" {
					cc = "USD"
				}
				table = append(table, []string{r.Bucket, itoa(r.Orders), cc + " " + model.FormatMoney(r.TotalCents)})
			}
			if cur == "" {
				cur = "USD"
			}
			return writeHumanTable(cmd, opt,
				[]string{"BUCKET", "ORDERS", "TOTAL"},
				table,
				map[string]any{"by": by, "rows": rows, "total_cents": grand, "currency": cur},
			)
		},
	}
	cmd.Flags().StringVar(&by, "by", "month", "Group by month or year")
	return cmd
}

func newTopItemsCmd(opt *Options) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "top-items",
		Short: "Most-ordered items from the local store",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := mustOpenStore(opt)
			if err != nil {
				return err
			}
			defer db.Close()
			rows, err := db.TopItems(limit)
			if err != nil {
				return err
			}
			if rows == nil {
				rows = []model.Ranked{}
			}
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{r.Name, itoa(r.Quantity), itoa(r.Orders), model.FormatMoney(r.TotalCents)})
			}
			return writeHumanTable(cmd, opt,
				[]string{"ITEM", "QTY", "ORDERS", "SPEND"},
				table,
				map[string]any{"items": rows, "count": len(rows)},
			)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "Max rows")
	return cmd
}

func newTopRestaurantsCmd(opt *Options) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "top-restaurants",
		Short: "Most-ordered restaurants from the local store",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := mustOpenStore(opt)
			if err != nil {
				return err
			}
			defer db.Close()
			rows, err := db.TopRestaurants(limit)
			if err != nil {
				return err
			}
			if rows == nil {
				rows = []model.Ranked{}
			}
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{r.Name, itoa(r.Orders), model.FormatMoney(r.TotalCents)})
			}
			return writeHumanTable(cmd, opt,
				[]string{"RESTAURANT", "ORDERS", "SPEND"},
				table,
				map[string]any{"restaurants": rows, "count": len(rows)},
			)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "Max rows")
	return cmd
}

func newFindCmd(opt *Options) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "find <query>",
		Short: "Search restaurant names and item titles",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := args[0]
			if len(args) > 1 {
				// allow find foo bar without quotes
				q = joinArgs(args)
			}
			db, err := mustOpenStore(opt)
			if err != nil {
				return err
			}
			defer db.Close()
			orders, err := db.Find(q, limit)
			if err != nil {
				return err
			}
			if orders == nil {
				orders = []model.Order{}
			}
			table := make([][]string, 0, len(orders))
			for _, o := range orders {
				when := ""
				if !o.OrderedAt.IsZero() {
					when = o.OrderedAt.UTC().Format("2006-01-02")
				}
				table = append(table, []string{o.ID, when, o.RestaurantName, model.FormatMoney(o.TotalCents)})
			}
			return writeHumanTable(cmd, opt,
				[]string{"ID", "DATE", "RESTAURANT", "TOTAL"},
				table,
				map[string]any{"query": q, "orders": orders, "count": len(orders)},
			)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Max rows")
	return cmd
}

func joinArgs(args []string) string {
	out := args[0]
	for i := 1; i < len(args); i++ {
		out += " " + args[i]
	}
	return out
}
