// Copyright 2026 Amandeep Khurana and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testOrderID = "11111111-1111-1111-1111-111111111111"

func runOrdersCLI(t *testing.T, home string, args ...string) (string, error) {
	t.Helper()
	var flags rootFlags
	root := newRootCmd(&flags)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"--home", home, "--no-learn"}, args...))
	err := root.Execute()
	return out.String(), err
}

func fixturePath(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "fixtures", "past_orders_page.json")
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOrderHistoryCommandsFromFixture(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	out, err := runOrdersCLI(t, home, "sync", "--from-fixture", fixturePath(t), "--json")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	var sync map[string]any
	if err := json.Unmarshal([]byte(out), &sync); err != nil {
		t.Fatalf("sync output not JSON: %v\n%s", err, out)
	}
	if sync["upserted"] != float64(3) || sync["total_orders"] != float64(3) {
		t.Fatalf("unexpected sync result: %v", sync)
	}

	// Incremental re-sync skips known orders.
	out, err = runOrdersCLI(t, home, "sync", "--from-fixture", fixturePath(t), "--json")
	if err != nil {
		t.Fatalf("resync: %v\n%s", err, out)
	}
	_ = json.Unmarshal([]byte(out), &sync)
	if sync["skipped_known"] != float64(3) || sync["upserted"] != float64(0) {
		t.Fatalf("incremental resync should skip known orders: %v", sync)
	}

	out, err = runOrdersCLI(t, home, "history", "list", "--json", "--limit", "2")
	if err != nil {
		t.Fatalf("history list: %v\n%s", err, out)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(out), &list); err != nil || len(list) != 2 || list[0]["id"] != testOrderID {
		t.Fatalf("history list: %v\n%s", err, out)
	}

	out, err = runOrdersCLI(t, home, "history", "get", testOrderID, "--json", "--select", "id,items")
	if err != nil || !strings.Contains(out, "Cheeseburger") {
		t.Fatalf("history get: %v\n%s", err, out)
	}

	_, err = runOrdersCLI(t, home, "history", "get", "nope", "--json")
	if ExitCode(err) != 3 {
		t.Fatalf("missing order should exit 3, got %d (%v)", ExitCode(err), err)
	}

	out, err = runOrdersCLI(t, home, "spend", "--by", "year", "--json")
	if err != nil || !strings.Contains(out, `"2026"`) || !strings.Contains(out, `"total": "67.89"`) {
		t.Fatalf("spend: %v\n%s", err, out)
	}
	_, err = runOrdersCLI(t, home, "spend", "--by", "week", "--json")
	if ExitCode(err) != 2 {
		t.Fatalf("bad --by should exit 2, got %d", ExitCode(err))
	}

	out, err = runOrdersCLI(t, home, "top-items", "--json")
	if err != nil || !strings.Contains(out, "Cheeseburger") {
		t.Fatalf("top-items: %v\n%s", err, out)
	}
	out, err = runOrdersCLI(t, home, "top-restaurants", "--json")
	if err != nil || !strings.Contains(out, "Shake Shack") {
		t.Fatalf("top-restaurants: %v\n%s", err, out)
	}
	out, err = runOrdersCLI(t, home, "find", "ramen", "--json")
	if err != nil || !strings.Contains(out, "Ippudo") {
		t.Fatalf("find: %v\n%s", err, out)
	}
	out, err = runOrdersCLI(t, home, "find", "--json")
	if ExitCode(err) != 2 && !strings.Contains(out, "Usage") {
		t.Fatalf("find without query: %v\n%s", err, out)
	}

	// Dry runs never touch the store or the network.
	for _, args := range [][]string{{"sync"}, {"history", "list"}, {"spend"}, {"top-items"}, {"top-restaurants"}, {"find", "x"}} {
		out, err := runOrdersCLI(t, home, append(args, "--dry-run", "--json")...)
		if err != nil || !strings.Contains(out, `"dry_run":true`) {
			t.Fatalf("dry-run %v: %v\n%s", args, err, out)
		}
	}
}
