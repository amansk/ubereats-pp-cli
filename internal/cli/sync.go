// Copyright 2026 Amandeep Khurana and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: mirror getPastOrdersV1 into the local SQLite store.
// pp:data-source live
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"ubereats-pp-cli/internal/cliutil"
	"ubereats-pp-cli/internal/orders"
)

const (
	pastOrdersPath = "/_p/api/getPastOrdersV1"
	syncMaxPages   = 250
)

func newNovelSyncCmd(flags *rootFlags) *cobra.Command {
	var full bool
	var fixture string
	var maxPages int

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Mirror your Uber Eats order history into the local SQLite store",
		Long: `Walk every page of getPastOrdersV1 with your browser session and mirror
orders plus line items into the local store. Incremental by default: the walk
stops at the first page whose orders are all already mirrored. --full re-walks
from the newest order with no early stop. --from-fixture ingests a saved
getPastOrdersV1 JSON body (one envelope or an array of envelopes) instead of
calling Uber Eats, which keeps CI and offline demos honest.

Wire field names are hunches until verified against a live capture; the parser
is tolerant and the original per-order JSON is kept in raw_json.`,
		Example: strings.Trim(`
  ubereats-pp-cli sync --agent
  ubereats-pp-cli sync --full --json
  ubereats-pp-cli sync --from-fixture testdata/fixtures/past_orders_page.json --agent
`, "\n"),
		// sync never writes remote state: it reads getPastOrdersV1 and writes
		// only the local SQLite mirror, so it is read-only from Uber's side.
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "--max-pages=1",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageErr(fmt.Errorf("sync takes no positional arguments (got %q)", args[0]))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "POST "+pastOrdersPath+" until meta.hasMore is false, then upsert into ue_orders/ue_items")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			s, mirror, err := openMirror(ctx)
			if err != nil {
				return err
			}
			defer s.Close()

			mode := "incremental"
			if full {
				mode = "full"
			}
			if maxPages <= 0 || maxPages > syncMaxPages {
				maxPages = syncMaxPages
			}
			if cliutil.IsDogfoodEnv() && maxPages > 1 {
				// Live dogfood runs every command under a flat timeout; one page
				// proves the wire path without walking years of history.
				maxPages = 1
			}

			var result orders.SyncResult
			if fixture != "" {
				result, err = syncFromFixture(ctx, mirror, fixture, mode)
			} else {
				result, err = syncFromAPI(ctx, cmd, flags, mirror, mode, maxPages)
			}
			if err != nil {
				return err
			}
			n, _ := mirror.Count(ctx)
			result.TotalOrders = n
			_ = s.SaveSyncState("ue_orders", "", n)
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "synced %d new/updated orders (%d line items) from %s; %d orders mirrored\n",
					result.Upserted, result.ItemRows, result.Source, result.TotalOrders)
				return nil
			}
			return flags.printJSON(cmd, result)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Re-walk from the newest order with no early stop")
	cmd.Flags().StringVar(&fixture, "from-fixture", "", "Ingest a saved getPastOrdersV1 JSON body instead of calling Uber Eats")
	cmd.Flags().IntVar(&maxPages, "max-pages", syncMaxPages, "Stop after this many pages (safety bound)")
	return cmd
}

func syncFromFixture(ctx context.Context, mirror *orders.Mirror, path, mode string) (orders.SyncResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return orders.SyncResult{}, usageErr(fmt.Errorf("read --from-fixture %s: %w", path, err))
	}
	pages, err := loadFixturePages(raw)
	if err != nil {
		return orders.SyncResult{}, apiErr(err)
	}
	return ingestPages(ctx, mirror, pages, mode, "fixture:"+path)
}

func loadFixturePages(raw []byte) ([]orders.Page, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, fmt.Errorf("fixture is not a JSON array of envelopes: %w", err)
		}
		pages := make([]orders.Page, 0, len(arr))
		for _, item := range arr {
			p, err := orders.ParsePastOrders(item)
			if err != nil {
				return nil, err
			}
			pages = append(pages, p)
		}
		return pages, nil
	}
	p, err := orders.ParsePastOrders(raw)
	if err != nil {
		return nil, err
	}
	return []orders.Page{p}, nil
}

func syncFromAPI(ctx context.Context, cmd *cobra.Command, flags *rootFlags, mirror *orders.Mirror, mode string, maxPages int) (orders.SyncResult, error) {
	c, err := flags.newClient()
	if err != nil {
		return orders.SyncResult{}, err
	}
	known := map[string]struct{}{}
	if mode == "incremental" {
		if known, err = mirror.KnownIDs(ctx); err != nil {
			return orders.SyncResult{}, err
		}
	}

	var pages []orders.Page
	cursor := ""
	for i := 0; i < maxPages; i++ {
		raw, _, err := c.Post(ctx, pastOrdersPath, map[string]string{"lastWorkflowUUID": cursor})
		if err != nil {
			return orders.SyncResult{}, classifyAPIError(cmd.OutOrStdout(), err, flags)
		}
		page, err := orders.ParsePastOrders(raw)
		if err != nil {
			return orders.SyncResult{}, apiErr(err)
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
	return ingestPages(ctx, mirror, pages, mode, "getPastOrdersV1")
}

func allKnown(list []orders.Order, known map[string]struct{}) bool {
	if len(list) == 0 {
		return false
	}
	for _, o := range list {
		if _, ok := known[o.ID]; !ok {
			return false
		}
	}
	return true
}

func ingestPages(ctx context.Context, mirror *orders.Mirror, pages []orders.Page, mode, source string) (orders.SyncResult, error) {
	known, err := mirror.KnownIDs(ctx)
	if err != nil {
		return orders.SyncResult{}, err
	}
	seen := map[string]struct{}{}
	var batch []orders.Order
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
			if b := page.RawByID[o.ID]; len(b) > 0 {
				rawByID[o.ID] = b
			}
		}
		if mode == "incremental" && len(known) > 0 && unknown == 0 && i > 0 {
			stoppedEarly = true
			break
		}
	}
	upserted, items, err := mirror.UpsertOrders(ctx, batch, rawByID)
	if err != nil {
		return orders.SyncResult{}, err
	}
	return orders.SyncResult{
		Mode:         mode,
		Pages:        len(pages),
		Upserted:     upserted,
		SkippedKnown: skipped,
		ItemRows:     items,
		Source:       source,
		StoppedEarly: stoppedEarly,
	}, nil
}
