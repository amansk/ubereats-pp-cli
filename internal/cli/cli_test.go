package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
	"github.com/amansk/ubereats-pp-cli/internal/store"
)

var fixtureSecrets = []string{
	"fixture-sid-SECRET-do-not-print",
	"fixture-csid-SECRET-do-not-print",
	"fixture-jwt-SECRET-do-not-print",
	"json-sid-SECRET-do-not-print",
	"json-csid-SECRET-do-not-print",
	"piped-SECRET",
}

func runCLI(t *testing.T, home string, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRoot()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(append([]string{"--home", home}, args...))
	err := cmd.Execute()
	assertNoCookieLeak(t, args, out.String(), errb.String())
	return out.String(), errb.String(), err
}

func assertNoCookieLeak(t *testing.T, args []string, blobs ...string) {
	t.Helper()
	joined := strings.Join(blobs, "\n")
	for _, secret := range fixtureSecrets {
		if strings.Contains(joined, secret) {
			t.Fatalf("cookie value %q leaked from %v:\n%s", secret, args, joined)
		}
	}
}

func fixtures(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "fixtures")
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDoctorGreenAfterCookieImport(t *testing.T) {
	home := t.TempDir()
	cookies := filepath.Join(fixtures(t), "cookies.txt")
	_, stderr, err := runCLI(t, home, "auth", "login", "--cookie-file", cookies, "--json")
	if err != nil {
		t.Fatalf("login: %v stderr=%s", err, stderr)
	}
	stdout, stderr, err := runCLI(t, home, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v\n%s\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "doctor: green") {
		t.Fatalf("want green doctor, got %q", stdout)
	}

	stdout, _, err = runCLI(t, home, "auth", "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var wrap map[string]any
	if err := json.Unmarshal([]byte(stdout), &wrap); err != nil {
		t.Fatal(err)
	}
}

func TestSyncFixtureAndQueries(t *testing.T) {
	home := t.TempDir()
	fx := fixtures(t)
	if _, _, err := runCLI(t, home, "auth", "login", "--cookie-file", filepath.Join(fx, "cookies.txt")); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := runCLI(t, home, "sync", "--from-fixture", filepath.Join(fx, "past_orders_page.json"), "--json")
	if err != nil {
		t.Fatalf("sync: %v %s", err, stderr)
	}
	if !strings.Contains(stdout, `"upserted"`) {
		t.Fatalf("sync json = %s", stdout)
	}

	stdout, _, err = runCLI(t, home, "orders", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Shake Shack") {
		t.Fatalf("list = %s", stdout)
	}

	stdout, _, err = runCLI(t, home, "orders", "get", "22222222-2222-2222-2222-222222222222", "--agent")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Ippudo") || !strings.Contains(stdout, `"ok":true`) {
		t.Fatalf("get = %s", stdout)
	}

	stdout, _, err = runCLI(t, home, "spend", "--by", "month", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "2026-03") || !strings.Contains(stdout, "2025-12") {
		t.Fatalf("spend month = %s", stdout)
	}

	stdout, _, err = runCLI(t, home, "spend", "--by", "year", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "2026") || !strings.Contains(stdout, "2025") {
		t.Fatalf("spend = %s", stdout)
	}

	stdout, _, err = runCLI(t, home, "top-items", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Cheeseburger") {
		t.Fatalf("top-items = %s", stdout)
	}

	stdout, _, err = runCLI(t, home, "top-restaurants", "--agent")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Shake Shack") {
		t.Fatalf("top-restaurants = %s", stdout)
	}

	stdout, _, err = runCLI(t, home, "find", "ramen", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Ippudo") {
		t.Fatalf("find = %s", stdout)
	}

	_, _, err = runCLI(t, home, "orders", "get", "missing-id")
	if err == nil {
		t.Fatal("expected not found")
	}
	var ex *exitcode.Error
	if !exitcode.As(err, &ex) || ex.Code != exitcode.NotFound {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestDoctorFailsWithoutCookies(t *testing.T) {
	home := t.TempDir()
	_, _, err := runCLI(t, home, "doctor")
	if err == nil {
		t.Fatal("expected doctor failure")
	}
	var ex *exitcode.Error
	if !exitcode.As(err, &ex) || ex.Code != exitcode.Auth {
		t.Fatalf("want auth exit, got %v", err)
	}
}

func TestDoctorJSONFailureEnvelopeOkFalse(t *testing.T) {
	home := t.TempDir()
	stdout, _, err := runCLI(t, home, "doctor", "--json")
	if err == nil {
		t.Fatal("expected doctor failure")
	}
	var wrap map[string]any
	if err := json.Unmarshal([]byte(stdout), &wrap); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	ok, _ := wrap["ok"].(bool)
	if ok {
		t.Fatalf("top-level ok should be false on failure: %s", stdout)
	}
	if wrap["error"] == nil || wrap["error"] == "" {
		t.Fatalf("want error field on failure envelope: %s", stdout)
	}
}

func TestIncrementalSyncSkipsKnown(t *testing.T) {
	home := t.TempDir()
	fx := filepath.Join(fixtures(t), "past_orders_page.json")
	if _, _, err := runCLI(t, home, "sync", "--from-fixture", fx, "--json"); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := runCLI(t, home, "sync", "--from-fixture", fx, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"upserted": 0`) && !strings.Contains(stdout, `"upserted":0`) {
		t.Fatalf("expected no new upserts: %s", stdout)
	}
}

func TestParseUntilDateCoversEveningUTC(t *testing.T) {
	u, err := parseUntilDate("2026-03-15")
	if err != nil || u == nil {
		t.Fatalf("parse: %v", err)
	}
	evening := time.Date(2026, 3, 15, 19, 22, 0, 0, time.UTC)
	if evening.After(*u) {
		t.Fatalf("until %s excludes evening %s", u.Format(time.RFC3339Nano), evening)
	}
	s, err := parseDate("2026-03-15")
	if err != nil || s == nil || !s.Equal(time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("since should stay midnight: %v %v", s, err)
	}
}

func TestUntilIncludesCalendarDay(t *testing.T) {
	home := t.TempDir()
	fx := filepath.Join(fixtures(t), "past_orders_page.json")
	if _, _, err := runCLI(t, home, "sync", "--from-fixture", fx, "--json"); err != nil {
		t.Fatal(err)
	}
	const midDayID = "11111111-1111-1111-1111-111111111111" // 2026-03-15T19:22:00Z
	stdout, _, err := runCLI(t, home, "orders", "list", "--until", "2026-03-15", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, midDayID) {
		t.Fatalf("--until 2026-03-15 should include same-day order: %s", stdout)
	}
	stdout, _, err = runCLI(t, home, "orders", "list", "--until", "2026-03-14", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout, midDayID) {
		t.Fatalf("--until 2026-03-14 should exclude 2026-03-15 order: %s", stdout)
	}
}

func TestSyncStoresWireJSON(t *testing.T) {
	home := t.TempDir()
	fx := filepath.Join(fixtures(t), "past_orders_page.json")
	if _, _, err := runCLI(t, home, "sync", "--from-fixture", fx, "--json"); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	raw, err := db.OrderRawJSON("11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"storeInfo"`) || !strings.Contains(raw, `"fareInfo"`) {
		t.Fatalf("raw_json is not wire blob: %s", raw)
	}
	if strings.Contains(raw, `"restaurant_name"`) || strings.Contains(raw, `"total_cents"`) {
		t.Fatalf("raw_json looks remashed: %s", raw)
	}
}

func TestLoginJSONCookieFixture(t *testing.T) {
	home := t.TempDir()
	cookies := filepath.Join(fixtures(t), "cookies.json")
	stdout, stderr, err := runCLI(t, home, "auth", "login", "--cookie-file", cookies, "--json")
	if err != nil {
		t.Fatalf("json login: %v stderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, `"imported": 2`) && !strings.Contains(stdout, `"imported":2`) {
		t.Fatalf("json login = %s", stdout)
	}

	stdout, stderr, err = runCLI(t, home, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor after json login: %v\n%s\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, `"ok": true`) && !strings.Contains(stdout, `"ok":true`) {
		t.Fatalf("want green doctor json, got %s", stdout)
	}
}

func TestHumanLoginOmitsValues(t *testing.T) {
	home := t.TempDir()
	cookies := filepath.Join(fixtures(t), "cookies.txt")
	stdout, stderr, err := runCLI(t, home, "auth", "login", "--cookie-file", cookies)
	if err != nil {
		t.Fatalf("login: %v stderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "imported 3 cookies") {
		t.Fatalf("human login = %s", stdout)
	}
	if !strings.Contains(stdout, "sid") || !strings.Contains(stdout, "csid") {
		t.Fatalf("human login should list names: %s", stdout)
	}
}

func TestLoginStdinHeader(t *testing.T) {
	home := t.TempDir()
	cmd := NewRoot()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetIn(strings.NewReader("sid=piped-SECRET; csid=other"))
	cmd.SetArgs([]string{"--home", home, "auth", "login", "--cookie-file", "-", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	assertNoCookieLeak(t, []string{"auth", "login", "--cookie-file", "-"}, out.String(), errb.String())
}
