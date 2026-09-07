package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
)

// chromeCookieDBCandidates returns likely Chrome/Chromium Cookies sqlite paths.
func chromeCookieDBCandidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var out []string
	switch runtime.GOOS {
	case "linux":
		out = []string{
			filepath.Join(home, ".config", "google-chrome", "Default", "Cookies"),
			filepath.Join(home, ".config", "chromium", "Default", "Cookies"),
			filepath.Join(home, ".config", "google-chrome", "Profile 1", "Cookies"),
		}
	case "darwin":
		out = []string{
			filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Cookies"),
			filepath.Join(home, "Library", "Application Support", "Chromium", "Default", "Cookies"),
		}
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		if local != "" {
			out = []string{
				filepath.Join(local, "Google", "Chrome", "User Data", "Default", "Cookies"),
			}
		}
	}
	var existing []string
	for _, p := range out {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
		}
	}
	return existing
}

// ImportChrome is a best-effort read of Chrome cookies for ubereats.com / uber.com.
// Values are never returned in errors. Recent Chrome builds often encrypt cookies
// in a way we cannot decrypt here; callers should fall back to --cookie-file.
func ImportChrome() (Store, error) {
	dbs := chromeCookieDBCandidates()
	if len(dbs) == 0 {
		return Store{}, exitcode.Authf("chrome cookie database not found; export cookies and use --cookie-file")
	}

	// Best-effort: try kooky-free raw sqlite read of the name/value columns.
	// Chrome encrypts `encrypted_value`; plaintext `value` is usually empty.
	st, err := readChromeSQLite(dbs[0])
	if err != nil {
		return Store{}, exitcode.Authf("chrome import failed (%s); export a Cookie header or cookies.txt and use --cookie-file", summarizeChromeErr(err))
	}
	if len(st.Cookies) == 0 {
		return Store{}, exitcode.Authf("chrome cookies for ubereats.com/uber.com were empty or encrypted; use --cookie-file")
	}
	st.Source = "chrome"
	st.Domains = []string{".ubereats.com", ".uber.com", "auth.uber.com"}
	return st, nil
}

func summarizeChromeErr(err error) string {
	if err == nil {
		return "unknown"
	}
	// Never include the original error text if it might embed cookie bytes.
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "cookie") && strings.Contains(msg, "=") {
		return "encrypted or unreadable cookie db"
	}
	if len(msg) > 120 {
		msg = msg[:120]
	}
	return msg
}

func relevantChromeHost(host string) bool {
	h := strings.ToLower(host)
	h = strings.TrimPrefix(h, ".")
	return strings.HasSuffix(h, "ubereats.com") ||
		h == "uber.com" || strings.HasSuffix(h, ".uber.com") ||
		h == "auth.uber.com"
}

func readChromeSQLite(path string) (Store, error) {
	// Imported lazily so auth tests do not require a browser.
	return readChromeCookiesFromDB(path)
}

// chromeReadHook lets tests inject a fake Chrome reader.
var chromeReadHook = readChromeCookiesFromDBImpl

func readChromeCookiesFromDB(path string) (Store, error) {
	return chromeReadHook(path)
}
