# github

[![ci](https://github.com/tamnd/github-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/tamnd/github-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/tamnd/github-cli)](https://github.com/tamnd/github-cli/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/tamnd/github-cli.svg)](https://pkg.go.dev/github.com/tamnd/github-cli)
[![Go Report Card](https://goreportcard.com/badge/github.com/tamnd/github-cli)](https://goreportcard.com/report/github.com/tamnd/github-cli)
[![License](https://img.shields.io/github/license/tamnd/github-cli)](./LICENSE)

**github** reads github.com as data, with no token, ever.
One pure-Go binary turns the site into typed records: every repository, user, organization, issue, pull request, discussion, commit, branch, tag, release, file, topic, gist, package, and marketplace action, each with a canonical `github://` address, every field its page stated, and typed edges to everything it names.
Read one thing, list a million, walk the dependency graph, or export the lot as RDF.

[Install](#install) • [Quick start](#quick-start) • [Read one thing](#read-one-thing) • [List and search](#list-and-search) • [Contents](#look-inside-a-repository) • [Graph](#walk-the-graph) • [Linked data](#export-linked-data) • [Output](#output) • [No token](#no-token) • [Serve](#serve-it) • [Driver](#use-it-as-a-resource-uri-driver)

![github reading a repository, listing an account, resolving a dependency graph, printing graph edges, and emitting schema.org triples](docs/static/demo.gif)

GitHub is already a knowledge graph that happens to be served as a website.
Repositories depend on repositories, issues reference commits, commits belong to people, people belong to organizations, and every one of those relations is written down on a page somewhere.
Most tools hand you back a page, or the subset of fields somebody decided you needed.
`github` reads all of it, keeps all of it, and gives each entity an address.

Not affiliated with GitHub or Microsoft.
Full docs and guides live at **[tamnd.github.io/github-cli](https://tamnd.github.io/github-cli/)**.

## Install

```bash
go install github.com/tamnd/github-cli/cmd/github@latest
```

Prefer a prebuilt binary?
Grab an archive, a `.deb`/`.rpm`/`.apk`, or a signed checksum from [releases](https://github.com/tamnd/github-cli/releases).
Or let a package manager handle it:

```bash
# Homebrew (macOS)
brew install --cask tamnd/tap/github-cli

# Scoop (Windows)
scoop bucket add tamnd https://github.com/tamnd/scoop-bucket
scoop install github-cli

# apt (Debian, Ubuntu)
curl -fsSL https://tamnd.github.io/linux-repo/gpg.key | sudo gpg --dearmor -o /usr/share/keyrings/tamnd.gpg
echo "deb [signed-by=/usr/share/keyrings/tamnd.gpg] https://tamnd.github.io/linux-repo/apt stable main" | sudo tee /etc/apt/sources.list.d/tamnd.list
sudo apt update && sudo apt install github-cli

# dnf (Fedora, RHEL)
sudo dnf config-manager --add-repo https://tamnd.github.io/linux-repo/dnf/tamnd.repo
sudo dnf install github-cli

# container
docker run --rm ghcr.io/tamnd/github-cli:latest repo gohugoio/hugo
```

The binary is called `github`.
It does not replace the official `gh`, which does the authenticated half of the site far better.
This one does the public half without asking you to log in.

## Quick start

```bash
github repo gohugoio/hugo                        # a record, not a page
github owned torvalds -n 10                      # a list, streamed
github get https://github.com/golang/go/pull/1   # paste anything
```

`github get` takes a bare id, a URL you copied out of a browser, or a `github://` URI, works out what it points at, and reads it.
Every other read command is the same thing with the kind already decided.

Output adapts to where it goes: an aligned table on your terminal, JSONL the moment you pipe it somewhere.

## Read one thing

```bash
github repo gohugoio/hugo             # every field the page states
github repo gohugoio/hugo --deep      # plus what only the deferred fragments carry
github user torvalds
github org golang
github issue golang/go 1234
github pr golang/go 1
github commit golang/go abc1234       # author, verification, changed files
github release cli/cli                # the latest one, or pass a tag
github discussion vercel/next.js 12345
github compare golang/go go1.22.0 go1.23.0
```

Records carry what the page carried, not a curated subset.
A repository page states its head commit, its commit and release counts, its licence, its topics, its funding and citation flags, and its whole root tree, so the record does too, from one request.

## List and search

```bash
github repos --language rust --sort stars -n 100
github issues "repo:golang/go is:open label:NeedsInvestigation"
github prs --owner golang
github users --language go
github search kubernetes            # every entity kind at once
github topics machine-learning
github actions lint                 # the marketplace
github trending --language go
```

Listing streams.
`-n` stops early without fetching the next page, and no command holds a full result set in memory unless a format forces it to.

`github code` is the one command that does not work: code search needs a signed-in session, and this tool has none.
It says so rather than returning nothing.

## Look inside a repository

```bash
github tree golang/go src/net/http
github tree gohugoio/hugo --recursive
github cat gohugoio/hugo go.mod
github blob golang/go src/runtime/proc.go
github readme gohugoio/hugo
github diff golang/go abc1234
github archive golang/go master --format tar.gz
```

`archive` is the one to reach for on a whole repository.
One request to codeload streams the entire tree, where `tree --recursive` is one request per directory.

There is also `github symbols`, which reads the code navigation index GitHub builds for every file.
It currently returns nothing, and says so: the blob page still renders the symbols button, and the list behind it is empty for a signed-out reader.
The command stays because the field is still in the payload and may fill in again.

## History and people

```bash
github commits golang/go -n 50
github branches golang/go
github tags golang/go
github releases cli/cli
github refs golang/go
github timeline golang/go 1234        # everything that happened on one issue

github owned torvalds                 # repositories as the profile shows them
github stars torvalds
github followers torvalds
github members golang
github contributions torvalds         # the calendar, one record per day
github gists torvalds
github activity torvalds
```

## Walk the graph

```bash
github graph golang/go                # the node, its edges, and its facts
github edges golang/go                # just the edges
github deps gohugoio/hugo             # what it depends on, with versions and licences
github dependents gohugoio/hugo       # the repositories that depend on it
github crawl golang/go --depth 2
github crawl golang/go --depth 3 --dry-run   # size the walk before running it
github contributors golang/go
github forks golang/go
```

Edges come from five places: explicit ids, the embedded React payload, the atom feeds, parsed HTML, and text.
Each edge records which one it came from, so a consumer can decide how much to trust it, and a walk can drop everything below a floor.

`deps` and `dependents` read GitHub's dependency graph, which is the part of the site with no API at all.
`deps` gives you the package, the version, the ecosystem, the manifest it came from, the licence, and whether it is direct or transitive.

## Export linked data

```bash
github rdf gohugoio/hugo --format ttl
github rdf gohugoio/hugo --format jsonld
github export golang/go --depth 2 --format jsonl > go.jsonl
```

RDF comes out as N-Triples, Turtle, JSON-LD, or N-Quads, over `schema.org` where a term exists and a `gh:` namespace where none does.
N-Triples and N-Quads stream, so exporting a large organization never needs the graph in memory.

## The page plane

Every reader in this tool works from one extraction of the page, and `github page` prints that extraction whole:

```bash
github page gohugoio/hugo                     # everything, organised
github page gohugoio/hugo --section payload   # just the embedded React payload
github page gohugoio/hugo --section meta      # just the og: and twitter: tags
github page golang/go#1234 --query IssueViewerViewQuery
github page https://github.com/trending --raw > trending.html
```

This is the debugging tool.
When a field comes back empty, `page` shows you the same view the reader had, so the answer is either "the page stopped carrying it" or "the selector is wrong", and you can tell which in one command.

## Output

Every command shares one contract: `-o table|markdown|list|json|jsonl|csv|tsv|url|raw`, `--fields` to pick columns, `--template` for a custom line, `-n` to limit.

```bash
github repos --language go --fields id,stars,forks
github repos --language go --template '{{.id}} has {{.stars}} stars'
github repo gohugoio/hugo -o json | jq .tree
```

`-o url` is the one the others are measured against.
It prints one URL per record and nothing else, so this composes with no glue:

```bash
github owned torvalds -o url | xargs -n1 github get
```

Failures are typed too.
Every surface exits 3 on an empty result, 4 when a page is not public, 5 on a rate limit, 6 on not found, 7 on unsupported, and 8 on a network failure, so a script can branch on the code without reading the message.

## No token

There is no API token in this tool and there will not be one.
The unauthenticated REST API allows sixty requests an hour, which is not enough to read one organization, while the pages sit behind a CDN and are faster than the API even where the API would work.

That is a promise you cannot check by using the tool.
You can see that a command works without logging in, but not that no code path would send a credential if one happened to be lying around.
So it is asserted instead: `gh/noauth_test.go` parses every source file with comments dropped and fails the build if the word `Authorization`, `GITHUB_TOKEN`, `GH_TOKEN`, or `api.github.com` shows up in code anywhere outside the one file whose job is to say those names out loud.

If you do have a token in your environment, `github doctor` will tell you it is being ignored, because a token that does nothing looks exactly like a token that is wrong:

```bash
github doctor
```

`doctor` checks the environment, whether the site answers, whether the page still carries the payload every reader expects, whether the cache is writable, and what pacing this run is using.
It is the first thing to reach for when something comes back wrong.

The cost of no token is that this is read-only and public-only.
For anything else, use the official [gh](https://cli.github.com).

## Serve it

The same operations are available over HTTP and as an MCP tool set for agents, with no extra code:

```bash
github serve --addr :7777    # every read verb becomes GET /v1/<verb>, streaming NDJSON
github mcp                   # speak MCP over stdio
```

Arguments go in the query string, because most of them contain a slash and a path would swallow it:

```bash
curl 'localhost:7777/v1/repo?ref=gohugoio/hugo'
curl 'localhost:7777/v1/blob?ref=gohugoio/hugo&path=go.mod'
curl 'localhost:7777/v1/trending?language=go&limit=5'
curl localhost:7777/v1/openapi.json
```

## Use it as a resource-URI driver

`github` registers a `github` domain the way a program registers a database driver with `database/sql`.
A host enables it with one blank import:

```go
import _ "github.com/tamnd/github-cli/gh"
```

Then [ant](https://github.com/tamnd/ant), or any program that links the package, dereferences `github://` URIs without knowing anything about the site:

```bash
ant get github://repo/gohugoio/hugo
ant cat github://file/gohugoio/hugo@master/go.mod
ant ls  github://org/golang
ant url github://pr/golang/go#1
```

## How it works

One `kit.Handle` registration per operation, and every surface updates itself:

```
cmd/github/    thin main: hands cli.NewApp to kit.Run
cli/           assembles the kit App and registers the byte-plane commands
gh/            the library: client, records, graph, RDF, doctor, domain.go
pkg/page/      one extraction of an HTML page, shared by every reader
pkg/gitproto/  the git smart HTTP protocol, for the refs no page lists
docs/          tago documentation site and the demo tape
```

That single declaration becomes a CLI command, an HTTP route, an MCP tool, and a URI dereference, so there is no second implementation to keep in step.

Underneath, seven surfaces answer for different routes: the embedded React payload, the JSON a route returns when asked for JSON, the fragments a page defers to XHR, the search backend, the atom feeds, raw.githubusercontent.com, and the git protocol itself.
`github routes` prints which surface answers for which route and which one it falls back to.

Responses are cached on disk, keyed by surface and URL, for fifteen minutes.
Anything addressed by a commit SHA is kept forever, because it cannot change.

## Development

```bash
make build      # ./bin/github
make test       # go test ./..., offline and deterministic
make live       # the smoke tests that actually talk to github.com
make vet
make lint
```

The offline tests check the parsers against bytes that were already parsed once, which is useful but cannot see the failure that matters: GitHub moving something.
`make live` is what sees that, so the assertions there are about shape rather than values.
A star count changes hourly, and pinning one turns a test into a clock.

The demo above is a tape, not a screen recording.
Regenerate it with [ascii-gif](https://github.com/tamnd/ascii-gif):

```bash
ascii-gif render docs/demo/github.tape -o docs/static/demo.gif
```

## Releasing

Push a version tag and GitHub Actions runs GoReleaser, which builds the archives, Linux packages, the multi-arch GHCR image, checksums, SBOMs, and a cosign signature:

```bash
git tag -a v0.2.0 -m "v0.2.0"
git push --tags
```

The Homebrew and Scoop steps self-disable until their tokens exist, so a release works with no extra secrets.

## License

Apache-2.0.
See [LICENSE](LICENSE).
