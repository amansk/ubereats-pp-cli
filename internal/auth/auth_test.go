package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCookieHeader(t *testing.T) {
	st, err := ParseCookies(strings.NewReader("sid=abc123; csid=zzz"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if st.Cookies["sid"] != "abc123" || st.Cookies["csid"] != "zzz" {
		t.Fatalf("cookies = %#v", st.Cookies)
	}
}

func TestParseNetscapeAndJSON(t *testing.T) {
	root := testdata(t)
	f, err := os.Open(filepath.Join(root, "cookies.txt"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := ParseCookies(f, "file")
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if st.Cookies["sid"] != "fixture-sid-SECRET-do-not-print" {
		t.Fatalf("sid missing")
	}

	jf, err := os.Open(filepath.Join(root, "cookies.json"))
	if err != nil {
		t.Fatal(err)
	}
	js, err := ParseCookies(jf, "json")
	_ = jf.Close()
	if err != nil {
		t.Fatal(err)
	}
	if js.Cookies["sid"] != "json-sid-SECRET-do-not-print" {
		t.Fatalf("json sid missing")
	}
}

func TestStatusNeverIncludesValues(t *testing.T) {
	home := t.TempDir()
	st := Store{
		Cookies: map[string]string{"sid": "SUPER-SECRET-VALUE-xyz"},
		Source:  "test",
	}
	if err := Save(home, st); err != nil {
		t.Fatal(err)
	}
	got := LoadStatus(home)
	if !got.Present || got.Count != 1 || got.Names[0] != "sid" {
		t.Fatalf("status = %+v", got)
	}
	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "SUPER-SECRET") {
		t.Fatalf("status leaked cookie value: %s", blob)
	}
}

func TestEmptyInput(t *testing.T) {
	_, err := ParseCookies(strings.NewReader("  \n"), "x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func testdata(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "fixtures")
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	return p
}
