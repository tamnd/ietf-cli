package ietf

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string
// functions and the host wiring (mint, body, resolve), which need no network.
// The client's HTTP behaviour is covered in ietf_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "ietf" {
		t.Errorf("Scheme = %q, want ietf", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "ietf" {
		t.Errorf("Identity.Binary = %q, want ietf", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"2616", "rfc", "rfc2616"},
		{"rfc2616", "rfc", "rfc2616"},
		{"RFC2616", "rfc", "rfc2616"},
		{"9110", "rfc", "rfc9110"},
		{"rfc9110", "rfc", "rfc9110"},
		{"oauth", "query", "oauth"},
		{"http semantics", "query", "http semantics"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		uriType string
		id      string
		want    string
	}{
		{"rfc", "rfc2616", "https://datatracker.ietf.org/doc/rfc2616/"},
		{"rfc", "rfc9110", "https://datatracker.ietf.org/doc/rfc9110/"},
		{"query", "oauth", "https://datatracker.ietf.org/doc/search/?name=oauth&rfcs=true"},
		{"wg", "httpbis", "https://datatracker.ietf.org/wg/httpbis/"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.uriType, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("Locate(%q, %q) = (%q, %v), want (%q, nil)",
				tc.uriType, tc.id, got, err, tc.want)
		}
	}
}

func TestParseLastSegment(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/api/v1/doc/stdlevelname/ps/", "ps"},
		{"/api/v1/doc/stdlevelname/std/", "std"},
		{"/api/v1/doc/stdlevelname/bcp/", "bcp"},
		{"", ""},
	}
	for _, tc := range cases {
		got := parseLastSegment(tc.in)
		if got != tc.want {
			t.Errorf("parseLastSegment(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestHostWiring mounts the driver in a kit Host and checks the round trip.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	r := &RFC{Name: "rfc9110", Title: "HTTP Semantics", Pages: 194}
	u, err := h.Mint(r)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "ietf://rfc/rfc9110"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("ietf", "9110")
	if err != nil || got.String() != "ietf://rfc/rfc9110" {
		t.Errorf("ResolveOn = (%q, %v), want ietf://rfc/rfc9110", got.String(), err)
	}
}
