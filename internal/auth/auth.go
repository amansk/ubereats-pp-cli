// Package auth imports and stores Uber Eats session cookies without printing values.
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
)

const (
	cookieFileName = "cookies.json"
	fileMode       = 0o600
	dirMode        = 0o700
)

// Session-ish names we mention in docs. Presence of any cookie is enough to
// count as "imported"; these names are only used for status hints.
var sessionHintNames = []string{"sid", "csid", "jwt-session", "uev2.id.session", "_ua"}

// Store is the on-disk cookie jar (values never appear in String/JSON helpers).
type Store struct {
	Cookies    map[string]string `json:"cookies"`
	Domains    []string          `json:"domains,omitempty"`
	Source     string            `json:"source"`
	ImportedAt time.Time         `json:"imported_at"`
}

// Status is a redacted view of the stored session.
type Status struct {
	Present    bool      `json:"present"`
	Count      int       `json:"count"`
	Names      []string  `json:"names"`
	Source     string    `json:"source,omitempty"`
	ImportedAt time.Time `json:"imported_at,omitempty"`
	Path       string    `json:"path"`
	Hints      []string  `json:"session_hints,omitempty"`
}

// HomeDir resolves the CLI state directory.
func HomeDir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if env := os.Getenv("UBERATS_PP_HOME"); env != "" {
		return env, nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", exitcode.Authf("resolve home: %w", err)
	}
	return filepath.Join(dir, ".config", "ubereats-pp-cli"), nil
}

// CookiePath is $HOME/cookies.json.
func CookiePath(home string) string {
	return filepath.Join(home, cookieFileName)
}

// EnsureHome creates the state directory.
func EnsureHome(home string) error {
	if err := os.MkdirAll(home, dirMode); err != nil {
		return exitcode.Authf("create home %s: %w", home, err)
	}
	return nil
}

// Save writes the store with restrictive permissions.
func Save(home string, st Store) error {
	if err := EnsureHome(home); err != nil {
		return err
	}
	if st.ImportedAt.IsZero() {
		st.ImportedAt = time.Now().UTC()
	}
	if st.Cookies == nil {
		st.Cookies = map[string]string{}
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return exitcode.Authf("encode cookies: %w", err)
	}
	path := CookiePath(home)
	if err := os.WriteFile(path, raw, fileMode); err != nil {
		return exitcode.Authf("write cookies: %w", err)
	}
	return nil
}

// Load reads the store. Missing file is a typed auth error.
func Load(home string) (Store, error) {
	path := CookiePath(home)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Store{}, exitcode.Authf("no session; run auth login --cookie-file")
		}
		return Store{}, exitcode.Authf("read cookies: %w", err)
	}
	var st Store
	if err := json.Unmarshal(raw, &st); err != nil {
		return Store{}, exitcode.Authf("parse cookies: %w", err)
	}
	if st.Cookies == nil {
		st.Cookies = map[string]string{}
	}
	return st, nil
}

// LoadStatus returns a redacted status even when no session exists.
func LoadStatus(home string) Status {
	path := CookiePath(home)
	st, err := Load(home)
	if err != nil {
		return Status{Present: false, Path: path, Names: []string{}}
	}
	return st.Status(path)
}

// Status redacts values.
func (s Store) Status(path string) Status {
	names := make([]string, 0, len(s.Cookies))
	for n := range s.Cookies {
		names = append(names, n)
	}
	sort.Strings(names)
	var hints []string
	for _, h := range sessionHintNames {
		if _, ok := s.Cookies[h]; ok {
			hints = append(hints, h)
		}
	}
	return Status{
		Present:    len(s.Cookies) > 0,
		Count:      len(s.Cookies),
		Names:      names,
		Source:     s.Source,
		ImportedAt: s.ImportedAt,
		Path:       path,
		Hints:      hints,
	}
}

// Header builds a Cookie header. Callers must not log the result.
func (s Store) Header() string {
	if len(s.Cookies) == 0 {
		return ""
	}
	names := make([]string, 0, len(s.Cookies))
	for n := range s.Cookies {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, n+"="+s.Cookies[n])
	}
	return strings.Join(parts, "; ")
}

// Delete removes the cookie file.
func Delete(home string) error {
	path := CookiePath(home)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return exitcode.Authf("logout: %w", err)
	}
	return nil
}

// ContainsValue reports whether haystack includes any stored cookie value.
// Used by tests to assert redaction.
func (s Store) ContainsValue(haystack string) bool {
	for _, v := range s.Cookies {
		if v != "" && strings.Contains(haystack, v) {
			return true
		}
	}
	return false
}

// RedactError strips cookie values from an error string.
func RedactError(st Store, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	for _, v := range st.Cookies {
		if v != "" && strings.Contains(msg, v) {
			msg = strings.ReplaceAll(msg, v, "[redacted]")
		}
	}
	if msg == err.Error() {
		return err
	}
	return fmt.Errorf("%s", msg)
}
