// Package cli assembles the github command tree from the github domain
// on top of the any-cli/kit framework.
package cli

import (
	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/github-cli/github"
)

// Build metadata, set via -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// NewApp assembles the kit application from the github domain. The domain's
// Register installs the client factory and every operation, so the binary and a
// multi-domain host (ant) share one source of truth. kit.Run turns the App into
// the CLI, the serve surface, and the MCP tool surface.
//
// To add a command, declare it in github/ops.go with kit.Handle and it appears
// here automatically. Reach for app.AddCommand only for a verb that does not fit
// the emit-records shape.
func NewApp() *kit.App {
	id := github.Domain{}.Info().Identity
	id.Version = Version

	app := kit.New(id)
	(github.Domain{}).Register(app)
	return app
}
