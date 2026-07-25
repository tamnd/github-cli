// Package cli assembles the github command tree from the gh domain on top of
// the any-cli/kit framework.
package cli

import (
	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/github-cli/gh"
)

// Build metadata, set via -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// NewApp assembles the kit application from the gh domain. The domain's
// Register installs the client factory and every operation, so the binary and a
// multi-domain host (ant, which blank-imports the package) share one source of
// truth. kit.Run turns the App into the CLI, plus the serve and mcp surfaces and
// the typed-error-to-exit-code mapping.
//
// To add a command, declare it in gh/ops.go with kit.Handle and it appears here
// automatically. Reach for app.AddCommand only for a verb that does not fit the
// emit-records shape, the way the byte-plane commands below do not.
func NewApp() *kit.App {
	id := gh.Domain{}.Info().Identity
	id.Version = Version

	// WithDefaults is how the site's own baseline reaches the resolved config.
	// Without it the run would use the framework's numbers, which are tuned for
	// an API with a published rate limit rather than for a CDN.
	app := kit.New(id, kit.WithDefaults(gh.DomainDefaults))
	(gh.Domain{}).Register(app)

	app.AddCommand(newVersionCmd())
	app.AddCommand(newCatCmd())
	app.AddCommand(newReadmeCmd())
	app.AddCommand(newArchiveCmd())
	app.AddCommand(newDiffCmd())
	return app
}
