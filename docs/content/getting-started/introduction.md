---
title: "Introduction"
description: "What github is, why it uses no token, and how it is put together."
weight: 10
---

`github` is a single binary that reads github.com over plain HTTPS and gives back records rather than pages.
There is nothing to sign up for and nothing to run alongside it.

## Why there is no token

The unauthenticated REST API allows sixty requests an hour.
That is not enough to read one organization, let alone crawl a graph, and the moment a tool needs a token it needs a place to put it, a way to rotate it, and a story about what happens when it leaks.

The public site has none of those problems.
It sits behind a CDN, so it answers faster than the API does even for the things the API covers, and it covers more: trending, contribution calendars, the dependency graph, release download counts, and the marketplace have no API route at all.

So this tool sends no `Authorization` header, never reads `GITHUB_TOKEN` or `GH_TOKEN`, and never touches `api.github.com`.
A test in the package parses every source file and fails the build if any of those four strings appear outside the doctor that warns about them.

The cost is that this is read-only and public-only.
For anything else, use the official [gh](https://cli.github.com).

## Seven surfaces

GitHub serves the same facts several different ways, and no single one of them is complete.
`github` reads all of them and treats the choice as a routing problem:

| Surface | What it is |
|---|---|
| `html` | The rendered page, and the React payload embedded in it |
| `route-json` | The same route asked for with `Accept: application/json`, which is what the site's own navigation uses |
| `xhr` | Fragments the page defers and fetches separately |
| `search` | The search backend, which answers in JSON for ten types |
| `feed` | The Atom feeds for commits, releases, tags, wikis, and accounts |
| `raw` | `raw.githubusercontent.com` and `codeload.github.com`, for file bytes and archives |
| `git` | The dumb HTTP git protocol, for a complete ref list in one request |

`github routes` prints the whole mapping, including which surface is primary for each route and which one it falls back to.

## How it is built

- The **`gh` package** holds the HTTP client, the surface router, the response cache, and the typed records.
  It paces requests, sets an honest User-Agent, retries the transient failures any public site throws, and records for every field which URL stated it.
- The **`pkg/page` package** does the extraction: the React payload, the preloaded Relay queries, schema.org blocks, ld+json, og and twitter meta, microdata, and the deferred fragments a page names for itself.
- The **`cli` package** wraps operations in subcommands.
- One **`cmd/github`** entry point ties them together.

Operations are declared once with [kit](https://github.com/tamnd/any-cli) and appear as a CLI command, an HTTP route, an MCP tool, and a `github://` URI type without any per-surface code.

Next: [install it](/getting-started/installation/), then take the [quick start](/getting-started/quick-start/).
