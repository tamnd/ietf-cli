// Package ietf is the library behind the ietf command line:
// the HTTP client, request shaping, and the typed data models for the IETF
// Datatracker API (RFC documents and working groups).
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
package ietf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultUserAgent identifies the client to the IETF Datatracker. A real,
// honest User-Agent is both polite and the thing most likely to keep you
// unblocked.
const DefaultUserAgent = "ietf-cli/dev (+https://github.com/tamnd/ietf-cli)"

// Host is the site this client talks to.
const Host = "datatracker.ietf.org"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// Config holds tunable client parameters so callers and the kit factory
// share one place to adjust them.
type Config struct {
	BaseURL string
	Rate    time.Duration
	Retries int
	Timeout time.Duration
}

// DefaultConfig returns production-safe defaults: 100 ms pacing, 3 retries,
// 15 s timeout.
func DefaultConfig() Config {
	return Config{
		BaseURL: BaseURL,
		Rate:    100 * time.Millisecond,
		Retries: 3,
		Timeout: 15 * time.Second,
	}
}

// Client talks to the IETF Datatracker over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	BaseURL   string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with DefaultConfig values.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: DefaultUserAgent,
		BaseURL:   cfg.BaseURL,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// Get fetches url and returns the response body. It paces and retries
// according to the client's settings.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- Output types ---

// RFC is a single IETF RFC document.
type RFC struct {
	Name     string `kit:"id" json:"name"`
	Title    string `json:"title"`
	Pages    int    `json:"pages"`
	Abstract string `json:"abstract"`
	StdLevel string `json:"std_level"` // e.g. "ps", "ds", "std", "bcp", "info", "exp", "hist"
	Updated  string `json:"updated"`
}

// WorkingGroup is an IETF working group.
type WorkingGroup struct {
	Acronym     string `kit:"id" json:"acronym"`
	Name        string `json:"name"`
	Description string `json:"description"`
	State       string `json:"state"`
}

// --- Wire types ---

type wireRFC struct {
	Name     string `json:"name"`
	Title    string `json:"title"`
	Pages    int    `json:"pages"`
	Abstract string `json:"abstract"`
	StdLevel string `json:"std_level"` // URL like /api/v1/doc/stdlevelname/ps/
	Time     string `json:"time"`
}

type wireRFCList struct {
	Objects []wireRFC `json:"objects"`
	Meta    struct {
		TotalCount int `json:"total_count"`
	} `json:"meta"`
}

type wireWG struct {
	Acronym     string `json:"acronym"`
	Name        string `json:"name"`
	Description string `json:"description"`
	State       string `json:"state"`
}

type wireWGList struct {
	Objects []wireWG `json:"objects"`
}

// --- Client methods ---

// GetRFC fetches one RFC by number. The number may be "2616" or "rfc2616";
// both are accepted.
func (c *Client) GetRFC(ctx context.Context, number string) (*RFC, error) {
	// Normalize: strip "rfc" prefix, then add it back for the URL.
	num := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(number)), "rfc")
	u := c.BaseURL + "/api/v1/doc/document/rfc" + num + "/?format=json"
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var w wireRFC
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("decode rfc: %w", err)
	}
	return toRFC(w), nil
}

// SearchRFCs searches RFCs whose title contains query (case-insensitive).
func (c *Client) SearchRFCs(ctx context.Context, query string, limit int) ([]RFC, error) {
	u := c.BaseURL + "/api/v1/doc/document/?format=json&type=rfc&title__icontains=" +
		url.QueryEscape(query) + "&limit=" + fmt.Sprint(limit)
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var wl wireRFCList
	if err := json.Unmarshal(body, &wl); err != nil {
		return nil, fmt.Errorf("decode rfc list: %w", err)
	}
	out := make([]RFC, 0, len(wl.Objects))
	for _, w := range wl.Objects {
		out = append(out, *toRFC(w))
	}
	return out, nil
}

// ListWGs lists IETF working groups. state="" returns all; "active" filters to
// active groups.
func (c *Client) ListWGs(ctx context.Context, state string, limit int) ([]WorkingGroup, error) {
	u := c.BaseURL + "/api/v1/group/group/?format=json&type=wg&limit=" + fmt.Sprint(limit)
	if state != "" {
		u += "&state__in=" + url.QueryEscape(state)
	}
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var wl wireWGList
	if err := json.Unmarshal(body, &wl); err != nil {
		return nil, fmt.Errorf("decode wg list: %w", err)
	}
	out := make([]WorkingGroup, 0, len(wl.Objects))
	for _, w := range wl.Objects {
		state := w.State
		// The API may return a URL like /api/v1/name/groupstatename/active/;
		// parse out just the slug.
		if strings.Contains(state, "/") {
			state = parseLastSegment(state)
		}
		out = append(out, WorkingGroup{
			Acronym:     w.Acronym,
			Name:        w.Name,
			Description: w.Description,
			State:       state,
		})
	}
	return out, nil
}

// --- helpers ---

// toRFC converts a wire RFC to the public type, parsing the StdLevel URL.
func toRFC(w wireRFC) *RFC {
	return &RFC{
		Name:     w.Name,
		Title:    w.Title,
		Pages:    w.Pages,
		Abstract: w.Abstract,
		StdLevel: parseLastSegment(w.StdLevel),
		Updated:  w.Time,
	}
}

// parseLastSegment extracts the last non-empty path segment from a URL string.
// "/api/v1/doc/stdlevelname/ps/" → "ps". Returns the original string on failure.
func parseLastSegment(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parts := strings.Split(strings.TrimRight(rawURL, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return rawURL
}
