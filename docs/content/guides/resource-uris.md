---
title: "Resource URIs"
description: "Use github as a database/sql-style driver so a host program can address GitHub as github:// URIs."
weight: 50
---

`github` is a command line, but the `gh` Go package is also a driver that makes github.com addressable as resource URIs.
A host program registers it the way a program registers a database driver with `database/sql`, then dereferences `github://` URIs without knowing anything about how the site is read.

The host that does this today is [ant](https://github.com/tamnd/ant), a single binary that puts one URI namespace over a family of site tools.
The examples below use `ant`; any program that links the package gets the same behaviour.

## Mounting the driver

A host enables the driver with one blank import, exactly like `import _ "github.com/lib/pq"`:

```go
import _ "github.com/tamnd/github-cli/gh"
```

The package's `init` registers a domain with the scheme `github`, the alias `gh`, and every hostname that means GitHub: github.com, raw.githubusercontent.com, gist.github.com, gist.githubusercontent.com, and codeload.github.com.
A pasted raw or gist link resolves here rather than falling through as an unknown site.
The standalone `github` binary does not change.

## The addressable types

A URI is `scheme://authority/id`, and the authority is the kind:

| URI | What it is |
|---|---|
| `github://repo/golang/go` | a repository |
| `github://user/torvalds` | a user |
| `github://org/golang` | an organization |
| `github://issue/golang/go#1` | an issue |
| `github://pr/gohugoio/hugo#13000` | a pull request |
| `github://discussion/gohugoio/hugo#13396` | a discussion |
| `github://commit/golang/go@6db72bb` | a commit |
| `github://branch/golang/go@master` | a branch |
| `github://tag/golang/go@go1.24.0` | a tag |
| `github://release/golang/go@go1.24.0` | a release |
| `github://file/golang/go@master/src/net/http/server.go` | a file at a revision |
| `github://tree/golang/go@master/src` | a directory at a revision |
| `github://label/golang/go/NeedsFix` | a label |
| `github://milestone/golang/go/1` | a milestone |
| `github://topic/go` | a topic |
| `github://gist/33cc3a9b0e37f5eb62dbd2a8e0f0eb8f` | a gist |
| `github://package/open-telemetry/opentelemetry-collector-contrib/opentelemetry-collector-contrib/telemetrygen` | a published package |
| `github://action/checkout` | a marketplace action |
| `github://wiki/golang/go/Home` | a wiki page |
| `github://advisory/GHSA-2xrv-8j4g-h47j` | a security advisory |
| `github://compare/golang/go@go1.23.0...go1.24.0` | a range, which names no single thing |
| `github://contributor/golang/go@rsc` | one person's counts in one repository |
| `github://contribution/torvalds@2026-07-01` | one day of a calendar |
| `github://event/...` | one entry in an activity feed |

The grammar has three separators and each one means exactly one thing.
`/` separates a namespace from a name, and a repository from a path.
`#` introduces a thread number.
`@` introduces a git revision.
A path may contain slashes but the revision always comes straight after the repository and the number is always last, so `github://file/golang/go@master/src/net/http/server.go` parses without ambiguity.

The last three kinds are records GitHub derives rather than serves.
No page's address is one contributor's statistics or one day of a calendar, so they get a URI and no canonical URL, and `github url` points at the page they were read from instead of inventing one.
An event has neither, and says so: its own `url` field is the only address it has.

```bash
ant get github://repo/gohugoio/hugo         # the record
ant cat github://file/gohugoio/hugo@master/go.mod   # the bytes
ant ls  github://org/golang                 # the org's repos, each addressable
ant url github://pr/golang/go#1             # the live https URL
```

The same resolution works in the binary without a host:

```bash
github url https://github.com/golang/go/blob/master/go.mod
github url golang/go#1
github url github://release/golang/go@go1.24.0
```

Parsing does no I/O and never fails on a well-formed github.com URL.
Two of its answers are guesses and both are documented as such: a bare word is a user, because a pure function cannot tell a user from an organization without asking, and a bare `owner/name` is a repository.
`github get` reads the page and returns a record whose kind is the truth.
Classification is a routing hint, not an answer.

## Walking the graph

Every record carries typed edges to the other things it names, so a host can follow them and write the result to disk:

```bash
ant export github://org/golang --follow 1 --to ./data
```

Because the edges are typed and the vocabulary is fixed, `ant graph` walks them the same way `github crawl` does, and across tools when an edge points at another site's scheme.
See [walk the graph](/guides/graph/) for the predicate list.

## Why this is the same code

The driver and the binary share one definition per operation.
A resolver op answers both `github repo` on the command line and `ant get github://repo/...` through a host, from the same handler and the same client, with the same pacing and the same response cache.
There is no second implementation to keep in step.

Next: [add a command](/guides/adding-a-command/).
