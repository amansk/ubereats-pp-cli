package cli

import (
	"github.com/amansk/ubereats-pp-cli/internal/model"
	"github.com/spf13/cobra"
)

func newOrdersCmd(opt *Options) *cobra.Command {
	cmd := &cobra.Command{Use: "orders", Short: "List or show synced orders"}
	cmd.AddCommand(newOrdersListCmd(opt))
	cmd.AddCommand(newOrdersGetCmd(opt))
	return cmd
}

func newOrdersListCmd(opt *Options) *cobra.Command {
	var limit int
	var since, until string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List synced past orders (newest first)",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := mustOpenStore(opt)
			if err != nil {
				return err
			}
			defer db.Close()
			s, err := parseDate(since)
			if err != nil {
				return err
			}
			u, err := parseDate(until)
			if err != nil {
				return err
			}
			orders, err := db.ListOrders(limit, s, u, false)
			if err != nil {
				return err
			}
			if orders == nil {
				orders = []model.Order{}
			}
			rows := make([][]string, 0, len(orders))
			for _, o := range orders {
				when := ""
				if !o.OrderedAt.IsZero() {
					when = o.OrderedAt.UTC().Format("2006-01-02")
				}
				cur := o.Currency
				if cur == "" {
					cur = "USD"
				}
				rows = append(rows, []string{o.ID, when, o.RestaurantName, cur + " " + model.FormatMoney(o.TotalCents)})
			}
			return writeHumanTable(cmd, opt,
				[]string{"ID", "DATE", "RESTAURANT", "TOTAL"},
				rows,
				map[string]any{"orders": orders, "count": len(orders)},
			)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Max rows")
	cmd.Flags().StringVar(&since, "since", "", "Inclusive lower bound (YYYY-MM-DD)")
	cmd.Flags().StringVar(&until, "until", "", "Inclusive upper bound (YYYY-MM-DD)")
	return cmd
}

func newOrdersGetCmd(opt *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show one synced order and its items",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := mustOpenStore(opt)
			if err != nil {
				return err
			}
			defer db.Close()
			o, err := db.GetOrder(args[0])
			if err != nil {
				return err
			}
			if opt.JSON || opt.Agent {
				return writeOut(cmd, opt, o)
			}
			when := ""
			if !o.OrderedAt.IsZero() {
				when = o.OrderedAt.UTC().Format("2006-01-02 15:04")
			}
			cur := o.Currency
			if cur == "" {
				cur = "USD"
			}
			cmd.Printf("%s  %s  %s  %s %s\n", o.ID, when, o.RestaurantName, cur, model.FormatMoney(o.TotalCents))
			if len(o.Items) == 0 {
				cmd.Println("(no line items parsed — raw payload stored; see PLAN.md)")
				return nil
			}
			rows := make([][]string, 0, len(o.Items))
			for _, it := range o.Items {
				rows = append(rows, []string{it.Title, itoa(it.Quantity), model.FormatMoney(it.UnitCents)})
			}
			return writeHumanTable(cmd, opt, []string{"ITEM", "QTY", "UNIT"}, rows, o)
		},
	}
}
