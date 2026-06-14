package ietf

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes ietf as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/ietf-cli/ietf"
//
// exactly as a database/sql program enables a driver with
// `import _ "github.com/lib/pq"`. The init below registers it; the host
// then dereferences ietf:// URIs by routing to the operations Register
// installs. The same Domain also builds the standalone ietf binary
// (see cli/root.go), so the binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the IETF Datatracker driver.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "ietf",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "ietf",
			Short:  "A command line for the IETF Datatracker (RFC documents and working groups).",
			Long: `A command line for the IETF Datatracker.

ietf reads public RFC documents and working group data from the IETF
Datatracker API over plain HTTPS, shapes it into clean records, and prints
output that pipes into the rest of your tools. No API key, nothing to run
alongside it.`,
			Site: Host,
			Repo: "https://github.com/tamnd/ietf-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// rfc: fetch a single RFC by number.
	kit.Handle(app, kit.OpMeta{
		Name: "rfc", Group: "read", Single: true,
		Summary: "Fetch an RFC by number (e.g. 9110 or rfc9110)",
		URIType: "rfc", Resolver: true,
		Args: []kit.Arg{{Name: "number", Help: "RFC number or name (2616 or rfc2616)"}},
	}, getRFC)

	// search: search RFCs by title keyword.
	kit.Handle(app, kit.OpMeta{
		Name: "search", Group: "read", List: true,
		Summary: "Search RFCs by title keyword",
		URIType: "rfc",
		Args:    []kit.Arg{{Name: "query", Help: "title keyword to search for"}},
	}, searchRFCs)

	// wg: list IETF working groups.
	kit.Handle(app, kit.OpMeta{
		Name: "wg", Group: "read", List: true,
		Summary: "List IETF working groups",
		URIType: "wg",
	}, listWGs)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type rfcRef struct {
	Number string  `kit:"arg" help:"RFC number or name (2616 or rfc2616)"`
	Client *Client `kit:"inject"`
}

type searchIn struct {
	Query  string  `kit:"arg" help:"title keyword to search for"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type wgIn struct {
	State  string  `kit:"flag" help:"filter by state (default: active)"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func getRFC(ctx context.Context, in rfcRef, emit func(*RFC) error) error {
	r, err := in.Client.GetRFC(ctx, in.Number)
	if err != nil {
		return mapErr(err)
	}
	return emit(r)
}

func searchRFCs(ctx context.Context, in searchIn, emit func(*RFC) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	results, err := in.Client.SearchRFCs(ctx, in.Query, limit)
	if err != nil {
		return mapErr(err)
	}
	for i := range results {
		if err := emit(&results[i]); err != nil {
			return err
		}
	}
	return nil
}

func listWGs(ctx context.Context, in wgIn, emit func(*WorkingGroup) error) error {
	state := in.State
	if state == "" {
		state = "active"
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	groups, err := in.Client.ListWGs(ctx, state, limit)
	if err != nil {
		return mapErr(err)
	}
	for i := range groups {
		if err := emit(&groups[i]); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: URI-native string functions, pure and network-free ---

// Classify turns any accepted input into the canonical (type, id).
// Inputs starting with "rfc" or all-digits → ("rfc", normalised name).
// Anything else → ("query", input).
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty IETF reference")
	}
	lower := strings.ToLower(input)
	if strings.HasPrefix(lower, "rfc") || isDigits(lower) {
		num := strings.TrimPrefix(lower, "rfc")
		return "rfc", "rfc" + num, nil
	}
	return "query", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "rfc":
		num := strings.TrimPrefix(strings.ToLower(id), "rfc")
		return "https://datatracker.ietf.org/doc/rfc" + num + "/", nil
	case "query":
		return "https://datatracker.ietf.org/doc/search/?name=" + id + "&rfcs=true", nil
	case "wg":
		return "https://datatracker.ietf.org/wg/" + strings.ToLower(id) + "/", nil
	default:
		return "", errs.Usage("ietf has no resource type %q", uriType)
	}
}

// --- helpers ---

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func mapErr(err error) error {
	return err
}
