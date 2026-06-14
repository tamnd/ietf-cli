package ietf_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/ietf-cli/ietf"
)

// newTestClient builds a Client pointed at the provided test server, with
// pacing and retries stripped so tests run fast.
func newTestClient(baseURL string) *ietf.Client {
	c := ietf.NewClient()
	c.BaseURL = baseURL
	c.Rate = 0
	c.Retries = 1
	c.HTTP = &http.Client{Timeout: 5 * time.Second}
	return c
}

func TestGetRFCByNumber(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/doc/document/rfc2616/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":      "rfc2616",
			"title":     "Hypertext Transfer Protocol -- HTTP/1.1",
			"pages":     176,
			"abstract":  "The Hypertext Transfer Protocol (HTTP) is an application-level protocol.",
			"std_level": "/api/v1/doc/stdlevelname/ps/",
			"time":      "2024-10-11T00:00:00",
		})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	// Test with plain number "2616".
	r, err := c.GetRFC(context.Background(), "2616")
	if err != nil {
		t.Fatalf("GetRFC(\"2616\"): %v", err)
	}
	if r.Name != "rfc2616" {
		t.Errorf("Name = %q, want rfc2616", r.Name)
	}
	if r.Title != "Hypertext Transfer Protocol -- HTTP/1.1" {
		t.Errorf("Title = %q", r.Title)
	}
	if r.Pages != 176 {
		t.Errorf("Pages = %d, want 176", r.Pages)
	}
	if r.StdLevel != "ps" {
		t.Errorf("StdLevel = %q, want ps", r.StdLevel)
	}
}

func TestGetRFCByNamePrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/doc/document/rfc2616/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":      "rfc2616",
			"title":     "Hypertext Transfer Protocol -- HTTP/1.1",
			"pages":     176,
			"abstract":  "",
			"std_level": "/api/v1/doc/stdlevelname/ps/",
			"time":      "2024-10-11T00:00:00",
		})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	// Test with "rfc2616" (prefix already present).
	r, err := c.GetRFC(context.Background(), "rfc2616")
	if err != nil {
		t.Fatalf("GetRFC(\"rfc2616\"): %v", err)
	}
	if r.Name != "rfc2616" {
		t.Errorf("Name = %q, want rfc2616", r.Name)
	}
}

func TestSearchRFCs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/doc/document/" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("title__icontains") != "oauth" {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"objects": []map[string]any{
				{
					"name":      "rfc5849",
					"title":     "The OAuth 1.0 Protocol",
					"pages":     75,
					"abstract":  "OAuth protocol.",
					"std_level": "/api/v1/doc/stdlevelname/info/",
					"time":      "2010-04-01T00:00:00",
				},
				{
					"name":      "rfc6749",
					"title":     "The OAuth 2.0 Authorization Framework",
					"pages":     76,
					"abstract":  "OAuth 2.0 framework.",
					"std_level": "/api/v1/doc/stdlevelname/ps/",
					"time":      "2012-10-01T00:00:00",
				},
			},
			"meta": map[string]any{"total_count": 2, "limit": 10, "offset": 0},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	results, err := c.SearchRFCs(context.Background(), "oauth", 10)
	if err != nil {
		t.Fatalf("SearchRFCs: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len = %d, want 2", len(results))
	}
	if results[0].Name != "rfc5849" {
		t.Errorf("results[0].Name = %q, want rfc5849", results[0].Name)
	}
	if results[1].StdLevel != "ps" {
		t.Errorf("results[1].StdLevel = %q, want ps", results[1].StdLevel)
	}
}

func TestListWGs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/group/group/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"objects": []map[string]any{
				{
					"acronym":     "httpbis",
					"name":        "HTTP",
					"description": "HTTP working group.",
					"state":       "active",
				},
				{
					"acronym":     "oauth",
					"name":        "Web Authorization Protocol",
					"description": "OAuth working group.",
					"state":       "active",
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	groups, err := c.ListWGs(context.Background(), "active", 20)
	if err != nil {
		t.Fatalf("ListWGs: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("len = %d, want 2", len(groups))
	}
	if groups[0].Acronym != "httpbis" {
		t.Errorf("groups[0].Acronym = %q, want httpbis", groups[0].Acronym)
	}
	if groups[1].Name != "Web Authorization Protocol" {
		t.Errorf("groups[1].Name = %q", groups[1].Name)
	}
}

func TestGetRetryOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := ietf.NewClient()
	c.Rate = 0
	c.Retries = 5
	c.HTTP = &http.Client{Timeout: 5 * time.Second}

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}
