package auth

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode"

	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
)

// ParseCookies accepts Netscape cookies.txt, a Chrome/Playwright JSON export,
// or a raw Cookie header. Values are kept; callers must not print them.
func ParseCookies(r io.Reader, source string) (Store, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return Store{}, exitcode.Authf("read cookie input: %w", err)
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return Store{}, exitcode.Authf("empty cookie input")
	}

	trimmed := bytes.TrimLeftFunc(raw, unicode.IsSpace)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		st, err := parseJSON(trimmed, source)
		if err != nil {
			return Store{}, err
		}
		if len(st.Cookies) == 0 {
			return Store{}, exitcode.Authf("json cookie export contained no cookies for ubereats.com / uber.com")
		}
		return st, nil
	}

	text := string(raw)
	if looksNetscape(text) {
		st, err := parseNetscape(text, source)
		if err != nil {
			return Store{}, err
		}
		if len(st.Cookies) == 0 {
			return Store{}, exitcode.Authf("netscape cookie file contained no cookies for ubereats.com / uber.com")
		}
		return st, nil
	}

	st := parseHeader(text, source)
	if len(st.Cookies) == 0 {
		return Store{}, exitcode.Authf("could not parse cookies (tried json, netscape, cookie header)")
	}
	return st, nil
}

func looksNetscape(s string) bool {
	if strings.Contains(s, "# Netscape HTTP Cookie File") || strings.Contains(s, "# HTTP Cookie File") {
		return true
	}
	// Tab-separated cookie rows: domain \t flag \t path \t secure \t expiry \t name \t value
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Count(line, "\t") >= 5 {
			return true
		}
	}
	return false
}

func parseNetscape(s, source string) (Store, error) {
	st := Store{Cookies: map[string]string{}, Source: source}
	domains := map[string]struct{}{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		domain := fields[0]
		name := fields[5]
		value := strings.Join(fields[6:], "\t")
		if name == "" {
			continue
		}
		if !AllowedCookieHost(domain) {
			continue
		}
		st.Cookies[name] = value
		if domain != "" {
			domains[domain] = struct{}{}
		}
	}
	for d := range domains {
		st.Domains = append(st.Domains, d)
	}
	return st, sc.Err()
}

func parseHeader(s, source string) Store {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "Cookie:")
	s = strings.TrimPrefix(s, "cookie:")
	s = strings.TrimSpace(s)
	st := Store{Cookies: map[string]string{}, Source: source}
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		st.Cookies[name] = value
	}
	return st
}

type chromeCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
}

func parseJSON(raw []byte, source string) (Store, error) {
	st := Store{Cookies: map[string]string{}, Source: source}

	var arr []chromeCookie
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		domains := map[string]struct{}{}
		for _, c := range arr {
			if c.Name == "" {
				continue
			}
			if c.Domain != "" && !AllowedCookieHost(c.Domain) {
				continue
			}
			st.Cookies[c.Name] = c.Value
			if c.Domain != "" {
				domains[c.Domain] = struct{}{}
			}
		}
		for d := range domains {
			st.Domains = append(st.Domains, d)
		}
		return st, nil
	}

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return Store{}, exitcode.Authf("parse cookie json: %w", err)
	}

	// { "cookies": { "sid": "..." } } or { "cookies": [ {name,value} ] }
	if v, ok := obj["cookies"]; ok {
		switch t := v.(type) {
		case map[string]any:
			for k, val := range t {
				st.Cookies[k] = stringify(val)
			}
		case []any:
			for _, item := range t {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				name := stringify(m["name"])
				if name == "" {
					continue
				}
				domain := stringify(m["domain"])
				if domain != "" && !AllowedCookieHost(domain) {
					continue
				}
				st.Cookies[name] = stringify(m["value"])
			}
		}
	}

	// { "sid": "...", "csid": "..." } — only if values look like strings
	if len(st.Cookies) == 0 {
		for k, v := range obj {
			if k == "cookies" || k == "domains" || k == "source" || k == "imported_at" {
				continue
			}
			if s, ok := v.(string); ok && s != "" {
				st.Cookies[k] = s
			}
		}
	}
	return st, nil
}

func stringify(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return strings.Trim(string(b), `"`)
}
