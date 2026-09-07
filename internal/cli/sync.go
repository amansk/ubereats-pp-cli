package cli

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"time"

	"github.com/amansk/ubereats-pp-cli/internal/auth"
	"github.com/amansk/ubereats-pp-cli/internal/client"
	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
	"github.com/amansk/ubereats-pp-cli/internal/model"
	"github.com/amansk/ubereats-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

const maxPages = 250

func newSyncCmd(opt *Options) *cobra.Command {
	var full bool
	var fixture string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Pull past orders into SQLite (incremental by default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, home, err := opt.OpenStore()
			if err != nil {
				return err
			}
			defer db.Close()

			mode := "incremental"
			if full {
				mode = "full"
			}

			var result model.SyncResult
			if fixture != "" {
				result, err = syncFromFixture(db, fixture, mode)
			} else {
				result, err = syncFromAPI(cmd.Context(), home, db, mode)
			}
			if err != nil {
				_ = db.SetState("last_error", err.Error())
				return err
			}
			_ = db.SetState("last_error", "")
			_ = db.SetState("last_sync_at", time.Now().UTC().Format(time.RFC3339))
			_ = db.SetState("last_mode", result.Mode)
			n, _ := db.Count()
			_ = db.SetState("order_count", itoa(n))
			result.Mode = mode
			return writeOut(cmd, opt, result)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Re-walk from the first page (no early stop)")
	cmd.Flags().StringVar(&fixture, "from-fixture", "", "Load getPastOrdersV1 JSON instead of the network")
	return cmd
}

func syncFromFixture(db *store.DB, path, mode string) (model.SyncResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return model.SyncResult{}, exitcode.Usagef("read fixture: %w", err)
	}
	pages, err := loadFixturePages(raw)
	if err != nil {
		return model.SyncResult{}, err
	}
	return ingestPages(db, pages, mode, "fixture:"+path)
}

func loadFixturePages(raw []byte) ([]client.Page, error) {
	// Array of envelopes, or a single envelope.
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 && raw[0] == '[' {
		var pages []client.Page
		for _, item := range arr {
			p, err := client.ParsePastOrders(item)
			if err != nil {
				return nil, err
			}
			pages = append(pages, p)
		}
		return pages, nil
	}
	p, err := client.ParsePastOrders(raw)
	if err != nil {
		return nil, err
	}
	return []client.Page{p}, nil
}

func syncFromAPI(ctx context.Context, home string, db *store.DB, mode string) (model.SyncResult, error) {
	st, err := auth.Load(home)
	if err != nil {
		return model.SyncResult{}, err
	}
	base := os.Getenv("UBERATS_PP_BASE_URL")
	c := client.New(st, base)
	known := map[string]struct{}{}
	if mode == "incremental" {
		known, err = db.KnownIDs()
		if err != nil {
			return model.SyncResult{}, err
		}
	}

	var pages []client.Page
	cursor := ""
	for i := 0; i < maxPages; i++ {
		page, err := c.GetPastOrders(ctx, cursor)
		if err != nil {
			return model.SyncResult{}, err
		}
		pages = append(pages, page)
		if len(page.Orders) == 0 {
			break
		}
		if mode == "incremental" && len(known) > 0 && allKnown(page.Orders, known) {
			break
		}
		if !page.HasMore || page.NextCursor == "" || page.NextCursor == cursor {
			break
		}
		cursor = page.NextCursor
	}
	return ingestPages(db, pages, mode, "getPastOrdersV1")
}

func allKnown(orders []model.Order, known map[string]struct{}) bool {
	if len(orders) == 0 {
		return false
	}
	for _, o := range orders {
		if _, ok := known[o.ID]; !ok {
			return false
		}
	}
	return true
}

func ingestPages(db *store.DB, pages []client.Page, mode, source string) (model.SyncResult, error) {
	known, err := db.KnownIDs()
	if err != nil {
		return model.SyncResult{}, err
	}
	seen := map[string]struct{}{}
	var batch []model.Order
	rawByID := map[string][]byte{}
	skipped := 0
	stoppedEarly := false
	for i, page := range pages {
		unknown := 0
		for _, o := range page.Orders {
			if _, dup := seen[o.ID]; dup {
				continue
			}
			seen[o.ID] = struct{}{}
			if mode == "incremental" {
				if _, ok := known[o.ID]; ok {
					skipped++
					continue
				}
			}
			unknown++
			batch = append(batch, o)
			if b, err := json.Marshal(o); err == nil {
				rawByID[o.ID] = b
			}
		}
		if mode == "incremental" && len(known) > 0 && unknown == 0 && i > 0 {
			stoppedEarly = true
			break
		}
	}
	upserted, items, err := db.UpsertOrders(batch, rawByID)
	if err != nil {
		return model.SyncResult{}, err
	}
	return model.SyncResult{
		Mode:         mode,
		Pages:        len(pages),
		Upserted:     upserted,
		SkippedKnown: skipped,
		ItemRows:     items,
		Source:       source,
		StoppedEarly: stoppedEarly,
	}, nil
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
