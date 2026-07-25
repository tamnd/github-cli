// Package gh reads github.com without a token.
//
// Every byte this package fetches is a byte a logged-out browser would get:
// public HTML, the JSON those pages ship inside themselves, the JSON their own
// front end asks for, Atom feeds, and the git smart protocol. There is no REST
// client here and there never will be one. The unauthenticated REST API allows
// sixty requests an hour, which is not enough to read one organization, and the
// pages are behind a CDN, which makes them faster than the API even when the
// API would work.
//
// The consequence is a read-only tool. It cannot see a private repository and
// it cannot write anything. For that, use the official gh.
package gh

import "time"

// The hosts. All five are public and none of them accept a credential from us.
const (
	BaseURL   = "https://github.com"
	RawURL    = "https://raw.githubusercontent.com"
	CodeLoad  = "https://codeload.github.com"
	GistRaw   = "https://gist.githubusercontent.com"
	AvatarURL = "https://avatars.githubusercontent.com"
	OpenGraph = "https://opengraph.githubassets.com"
)

// UserAgentBase is the honest half of the User-Agent. The version is appended
// at runtime. It is deliberately not configurable: making it configurable would
// be making impersonation a feature, and a tool that reads only public pages
// has no reason to hide.
const UserAgentBase = "github-cli"

// Defaults are the pacing numbers every command starts from. GitHub publishes
// no rate limit for the pages, so these are chosen to be quieter than a person
// browsing with a few tabs open: eight requests a second across four workers.
var Defaults = Config{
	Rate:    125 * time.Millisecond,
	Retries: 4,
	Workers: 4,
	Timeout: 30 * time.Second,
}

// Config is the resolved per-run configuration. It carries no credential field
// because there is no credential.
type Config struct {
	UserAgent string
	Rate      time.Duration
	Retries   int
	Workers   int
	Timeout   time.Duration
	CacheDir  string
	NoCache   bool
	CacheTTL  time.Duration
	Deep      bool
}
