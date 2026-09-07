package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
)

func runCLI(t *testing.T, home string, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRoot()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(append([]string{"--home", home}, args...))
	err := cmd.Execute()
	return out.String(), errb.String(), err
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
	stdout, stderr, err := runCLI(t, home, "auth", "login", "--cookie-file", cookies, "--json")
	if err != nil {
		t.Fatalf("login: %v stderr=%s", err, stderr)
	}
	if strings.Contains(stdout+stderr, "SECRET") {
		t.Fatalf("login leaked cookie value:\n%s\n%s", stdout, stderr)
	}

	stdout, stderr, err = runCLI(t, home, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v\n%s\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "doctor: green") {
		t.Fatalf("want green doctor, got %q", stdout)
	}
	if strings.Contains(stdout+stderr, "SECRET") {
		t.Fatalf("doctor leaked cookie value")
	}

	stdout, stderr, err = runCLI(t, home, "auth", "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout+stderr, "SECRET") {
		t.Fatalf("status leaked cookie value: %s", stdout)
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
	if strings.Contains(out.String()+errb.String(), "piped-SECRET") {
		t.Fatalf("stdin login leaked value: %s", out.String())
	}
}
