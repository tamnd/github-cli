package github

import (
	"context"

	"github.com/tamnd/any-cli/kit"
)

// init registers the github domain so a multi-domain host (ant) can load it
// with a blank import.
func init() { kit.Register(Domain{}) }

// Domain is the GitHub scraper driver. It carries no state; the per-run client
// is built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "github",
		Hosts:  []string{"github.com", "raw.githubusercontent.com"},
		Identity: kit.Identity{
			Binary: "github",
			Short:  "A command-line for GitHub (scrapes HTML, no API key needed).",
			Long: `github reads public GitHub data by scraping HTML pages and Atom feeds.
No API token is required. No rate limit from the official REST API.

github is an independent tool and is not affiliated with GitHub or Microsoft.`,
			Site: "https://github.com",
			Repo: "https://github.com/tamnd/github-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClientFromConfig)
	RegisterOps(app)
}

// newClientFromConfig builds a Client from the kit-resolved Config.
func newClientFromConfig(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
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
		c.Timeout = cfg.Timeout
	}
	return NewClient(c), nil
}
