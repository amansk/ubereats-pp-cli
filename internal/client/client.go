// Package client talks to unofficial Uber Eats web RPC endpoints.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/amansk/ubereats-pp-cli/internal/auth"
	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
	"github.com/amansk/ubereats-pp-cli/internal/model"
)

const (
	DefaultBaseURL = "https://www.ubereats.com"
	userAgent      = "Mozilla/5.0 (Macintosh; Intel X11) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
	maxBodyPeek    = 240
)

// Client is a cookie-authenticated Eats RPC client.
type Client struct {
	HTTP    *http.Client
	BaseURL string
	Store   auth.Store
}

// New builds a client from a cookie store.
func New(st auth.Store, baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		BaseURL: strings.TrimRight(baseURL, "/"),
		Store:   st,
	}
}

// Page is one getPastOrdersV1 response after tolerant parsing.
type Page struct {
	Orders     []model.Order
	HasMore    bool
	NextCursor string
	Raw        json.RawMessage
	RawByID    map[string][]byte // original per-order wire objects
}

// RPC POSTs /_p/api/<op> with browser-like headers.
func (c *Client) RPC(ctx context.Context, op string, body any) (json.RawMessage, int, error) {
	var payload []byte
	var err error
	if body == nil {
		payload = []byte("{}")
	} else if raw, ok := body.([]byte); ok {
		payload = raw
	} else {
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, 0, exitcode.APIf("encode %s: %w", op, err)
		}
	}

	url := c.BaseURL + "/_p/api/" + op
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, exitcode.APIf("build %s: %w", op, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-csrf-token", "x")
	req.Header.Set("Origin", "https://www.ubereats.com")
	req.Header.Set("Referer", "https://www.ubereats.com/orders")
	req.Header.Set("User-Agent", userAgent)
	if h := c.Store.Header(); h != "" {
		req.Header.Set("Cookie", h)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, exitcode.Transientf("%s: %w", op, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, exitcode.Transientf("%s read: %w", op, err)
	}

	if looksHTML(raw) {
		return raw, resp.StatusCode, exitcode.APIf("%s: HTML response (WAF/login wall?), HTTP %d", op, resp.StatusCode)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return raw, resp.StatusCode, exitcode.Authf("%s: HTTP %d (session expired or rejected)", op, resp.StatusCode)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return raw, resp.StatusCode, exitcode.Transientf("%s: rate limited (HTTP 429)", op)
	}
	if resp.StatusCode >= 500 {
		return raw, resp.StatusCode, exitcode.Transientf("%s: HTTP %d", op, resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return raw, resp.StatusCode, exitcode.APIf("%s: HTTP %d %s", op, resp.StatusCode, peek(raw))
	}
	return json.RawMessage(raw), resp.StatusCode, nil
}

// GetPastOrders fetches one history page.
func (c *Client) GetPastOrders(ctx context.Context, lastWorkflowUUID string) (Page, error) {
	raw, _, err := c.RPC(ctx, "getPastOrdersV1", map[string]string{
		"lastWorkflowUUID": lastWorkflowUUID,
	})
	if err != nil {
		return Page{}, auth.RedactError(c.Store, err)
	}
	page, err := ParsePastOrders(raw)
	if err != nil {
		return Page{}, err
	}
	return page, nil
}

// GetUser is an optional live probe. Shape is unverified.
func (c *Client) GetUser(ctx context.Context) (json.RawMessage, error) {
	raw, _, err := c.RPC(ctx, "getUserV1", map[string]bool{
		"shouldGetSubsMetadata": true,
	})
	if err != nil {
		return nil, auth.RedactError(c.Store, err)
	}
	return raw, nil
}

// GetOrderEntities is a STUB: request body is hypothesized, not verified.
func (c *Client) GetOrderEntities(ctx context.Context, orderUUID string) (json.RawMessage, error) {
	raw, status, err := c.RPC(ctx, "getOrderEntitiesV1", map[string]string{
		"orderUuid": orderUUID,
	})
	if err != nil {
		if status == http.StatusNotFound || status == http.StatusBadRequest {
			return nil, nil
		}
		return nil, auth.RedactError(c.Store, err)
	}
	return raw, nil
}

func looksHTML(b []byte) bool {
	s := strings.TrimSpace(string(b))
	return strings.HasPrefix(s, "<") || strings.Contains(strings.ToLower(s[:min(80, len(s))]), "<html")
}

func peek(b []byte) string {
	s := strings.TrimSpace(string(b))
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxBodyPeek {
		s = s[:maxBodyPeek] + "…"
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// EnvelopeStatus extracts data.status when present.
func EnvelopeStatus(raw json.RawMessage) string {
	var env struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return ""
	}
	return env.Status
}

func failIfEnvelope(raw json.RawMessage, op string) error {
	st := EnvelopeStatus(raw)
	if st != "" && !strings.EqualFold(st, "success") {
		return exitcode.APIf("%s: envelope status %q", op, st)
	}
	return nil
}
