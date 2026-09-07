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

func TestFileImportDropsOffDomainCookies(t *testing.T) {
	netscape := `# Netscape HTTP Cookie File
.google.com	TRUE	/	TRUE	1999999999	NID	google-secret-drop
.ubereats.com	TRUE	/	TRUE	1999999999	sid	keep-eats
.uber.com	TRUE	/	TRUE	1999999999	csid	keep-uber
`
	st, err := ParseCookies(strings.NewReader(netscape), "file")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Cookies["NID"]; ok {
		t.Fatalf("google cookie kept: %#v", st.Cookies)
	}
	if st.Cookies["sid"] != "keep-eats" || st.Cookies["csid"] != "keep-uber" {
		t.Fatalf("eats cookies missing: %#v", st.Cookies)
	}
	for _, d := range st.Domains {
		if strings.Contains(d, "google") {
			t.Fatalf("google domain persisted: %v", st.Domains)
		}
	}

	js := `[
	  {"name":"NID","value":"google-json-drop","domain":".google.com"},
	  {"name":"sid","value":"keep-json","domain":".ubereats.com"}
	]`
	got, err := ParseCookies(strings.NewReader(js), "json")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Cookies["NID"]; ok {
		t.Fatalf("json google cookie kept: %#v", got.Cookies)
	}
	if got.Cookies["sid"] != "keep-json" {
		t.Fatalf("json eats cookie missing: %#v", got.Cookies)
	}
}

func TestFileImportRejectsOnlyOffDomainCookies(t *testing.T) {
	_, err := ParseCookies(strings.NewReader(`[{"name":"NID","value":"x","domain":".google.com"}]`), "json")
	if err == nil {
		t.Fatal("expected error when no allowed-domain cookies remain")
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
