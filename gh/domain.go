package gh

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/tamnd/any-cli/kit"
)

// domain.go is the seam between this library and the kit framework. It declares
// what the site is called, how its addresses are parsed, and how a client is
// built from the resolved config. Every verb a person can type is registered in
// ops.go; nothing else in the package imports kit.

// Domain is the kit driver for github.com. A blank import of this package
// enables it in any multi-domain host, the way a database driver registers
// itself, and the same Domain builds the single github binary.
type Domain struct{}

func init() { kit.Register(Domain{}) }

// Info names the domain and every hostname that means it. The extra hosts are
// not decoration: a pasted raw.githubusercontent.com or gist.github.com link is
// a github address and has to resolve here rather than fall through as an
// unknown site.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme:  Scheme,
		Aliases: []string{"gh"},
		Hosts: []string{
			"github.com", "www.github.com",
			"raw.githubusercontent.com", "gist.github.com",
			"gist.githubusercontent.com", "codeload.github.com",
		},
		Identity: kit.Identity{
			Binary: "github",
			Short:  "Read all of GitHub as structured data, with no token",
			Long: "github reads github.com and gives back records rather than pages.\n\n" +
				"Every repository, user, organization, issue, pull request, discussion,\n" +
				"commit, branch, tag, release, file, topic, gist, package, and marketplace\n" +
				"action has a canonical github:// address, a typed record carrying every\n" +
				"field its source returned, and edges to the other things it names.\n\n" +
				"There is no API token anywhere in this tool and there will not be one.\n" +
				"The unauthenticated REST API allows sixty requests an hour, which is not\n" +
				"enough to read one organization, while the pages are behind a CDN and are\n" +
				"faster than the API even where the API would work. The cost is that this\n" +
				"is read-only and public-only. For anything else, use the official gh.",
			Site: BaseURL,
			Repo: "https://github.com/tamnd/github-cli",
		},
	}
}

// Classify satisfies kit.Resolver, so a URI typed at a multi-domain host and one
// typed at github are read by the same parser.
func (Domain) Classify(input string) (uriType, id string, err error) {
	return Classify(input)
}

// Locate satisfies kit.Resolver: the https location of one resource.
func (Domain) Locate(uriType, id string) (string, error) {
	return Locate(uriType, id)
}

// DomainDefaults overlays this site's baseline onto the framework's. GitHub
// publishes no rate limit for the pages, so these are chosen to be quieter than
// a person browsing with a few tabs open: eight requests a second across four
// workers.
func DomainDefaults(c *kit.Config) {
	c.Rate = Defaults.Rate
	c.Retries = Defaults.Retries
	c.Workers = Defaults.Workers
	c.Timeout = Defaults.Timeout
}

// flags holds the domain's own global flags. kit resolves the framework globals
// (--limit, --rate, --timeout, --no-cache) itself; these are the ones only this
// tool has, and they are read once when the client is built.
//
// Package-level state is the framework's contract here: GlobalFlags binds to the
// domain's variables and the client factory reads them, and there is exactly one
// run per process.
var flags struct {
	deep  bool
	jobs  int
	cache string
}

// Register installs the client factory, the domain globals, and every
// operation. It does no I/O and is deterministic, so a host can call it at
// startup.
func (d Domain) Register(app *kit.App) {
	app.SetClient(newClientFor)
	app.GlobalFlags(bindFlags)
	registerOps(app)
}

func bindFlags(f *kit.FlagSet) {
	f.BoolVar(&flags.deep, "deep", false, "also fetch the fragments a page defers, and merge what only they carry")
	f.IntVarP(&flags.jobs, "jobs", "j", 0, "concurrent requests (0 = the default 4)")
	f.StringVar(&flags.cache, "cache", "", "response cache directory (default under the data dir)")
}

// newClientFor builds the one client a run shares. Every command reaches it
// through a kit:"inject" field, so pacing and the cache are shared across a
// whole pipeline rather than per command.
func newClientFor(_ context.Context, cfg kit.Config) (any, error) {
	conf := Defaults
	if cfg.UserAgent != "" {
		conf.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		conf.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		conf.Retries = cfg.Retries
	}
	if cfg.Workers > 0 {
		conf.Workers = cfg.Workers
	}
	if cfg.Timeout > 0 {
		conf.Timeout = cfg.Timeout
	}
	conf.NoCache = cfg.NoCache
	conf.CacheDir = filepath.Join(cfg.CacheDir, "http")
	if flags.cache != "" {
		conf.CacheDir = flags.cache
	}
	if flags.jobs > 0 {
		conf.Workers = flags.jobs
	}
	conf.Deep = flags.deep

	c := NewClient(conf)
	c.HTTP = &http.Client{Timeout: conf.Timeout}
	return c, nil
}
