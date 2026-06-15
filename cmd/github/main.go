// Command github is a command-line for GitHub that scrapes HTML pages and
// Atom feeds. No API key or authentication is required.
//
// github is an independent tool and is not affiliated with GitHub or Microsoft.
package main

import (
	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/github-cli/cli"
)

func main() {
	kit.Main(cli.NewApp())
}
