---
title: "github"
description: "Read github.com as data, with no API token."
heroTitle: "GitHub, as records"
heroLead: "Every repository, user, organization, issue, pull request, discussion, commit, branch, tag, release, file, topic, gist, package, and marketplace action, with every field its source returned, a canonical github:// address, and edges to everything it names."
heroPrimaryURL: "/getting-started/quick-start/"
heroPrimaryText: "Get started"
---

`github` is one pure-Go binary.
It reads github.com over plain HTTPS, shapes what comes back into typed records, and prints output that pipes into the rest of your tools.

There is no API token anywhere in it and there will not be one.

```bash
github repo gohugoio/hugo             # every field the page states
github owned torvalds -n 10           # an account's repositories
github graph golang/go                # the node and its edges
github rdf gohugoio/hugo --format ttl # schema.org triples
```

Output adapts to where it goes: an aligned table on your terminal, JSONL the moment you pipe it somewhere.

![github reading a repository, listing an account, showing dependencies, printing graph edges, and emitting schema.org triples](/demo.gif)

## Why no token

The unauthenticated REST API allows sixty requests an hour, which is not enough to read one organization.
The pages sit behind a CDN, so they are faster than the API even where the API would work, and they carry more: trending, contribution calendars, dependency graphs, and download counts have no API route at all.

So this tool reads the same JSON the site's own browser code reads, plus the Atom feeds, the search JSON, the raw host, and the git protocol.
The cost is that it is read-only and public-only.
For anything else, use the official [gh](https://cli.github.com).

## What makes it different

- **Six surfaces, one record.** A repository record is assembled from the embedded React payload, the route JSON, a deferred XHR fragment, and the search index, and it records which URL each field came from.
- **It is a graph.** Repository references, mentions, closing keywords, forks, and dependencies all become typed edges, each one carrying the source that stated it.
- **It exports linked data.** N-Triples, Turtle, JSON-LD, and N-Quads, keyed on the same `github://` addresses.
- **Nothing is hidden.** `github page` prints the whole extraction of any URL, so a field no record models yet is still reachable today.

## Two ways to use it

- **As a command** for reading GitHub by hand or in a script.
  Start with the [quick start](/getting-started/quick-start/).
- **As a resource-URI driver** so a host like [ant](https://github.com/tamnd/ant) can address the site as `github://` URIs and follow links across sites.
  See [resource URIs](/guides/resource-uris/).

Both are the same code: one operation, declared once, is a CLI command, an HTTP route, an MCP tool, and a URI dereference.

Not affiliated with GitHub or Microsoft.
It reads the public site the way any reader does.

## Where to go next

- New here?
  Read the [introduction](/getting-started/introduction/), then the [quick start](/getting-started/quick-start/).
- Installing?
  See [installation](/getting-started/installation/).
- Doing a specific job?
  The [guides](/guides/) are task-first.
- Need every flag?
  The [CLI reference](/reference/cli/) is the full surface.
