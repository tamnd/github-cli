---
title: "Quick start"
description: "Read a repository, list an account, walk the graph, export triples."
weight: 30
---

Once `github` is on your `PATH`, there is nothing to configure.
No login, no token, no config file.

## Read one thing

```bash
github repo gohugoio/hugo
github user torvalds
github org golang
github issue golang/go 1
github pr kubernetes/kubernetes 100000
github release gohugoio/hugo            # the latest one
github commit golang/go 3e5c0f3
```

Or let it work out what a reference names:

```bash
github get gohugoio/hugo
github get https://github.com/golang/go/issues/1
github get github://pr/golang/go#1
```

A reference can be `owner/name`, a github.com URL, or a `github://` URI, on every command that takes one.
`github url <ref>` parses one and tells you what it decided, which is the fastest way to check a shape:

```console
$ github url gohugoio/hugo
{"kind":"repo","id":"gohugoio/hugo","uri":"github://repo/gohugoio/hugo","url":"https://github.com/gohugoio/hugo"}
```

## List things

```bash
github owned torvalds -n 10       # an account's repositories
github releases gohugoio/hugo
github commits golang/go -n 20
github contributors gohugoio/hugo
github stars torvalds
github members golang
github trending --language go --since weekly
```

## Search

```bash
github repos "language:go stars:>10000" --sort stars
github issues "repo:golang/go is:open label:NeedsInvestigation"
github users "location:vietnam followers:>500"
github search "site generator"          # every type at once
```

## Look inside a repository

```bash
github tree gohugoio/hugo                    # the root
github tree gohugoio/hugo commands --recursive
github cat gohugoio/hugo go.mod              # bytes to stdout
github readme gohugoio/hugo
github diff golang/go 3e5c0f3
github archive gohugoio/hugo --format zip -o hugo.zip
```

## Walk the graph

```bash
github graph gohugoio/hugo        # the node, its edges, and its facts
github edges golang/go            # edges only
github crawl golang/go --depth 2 --kinds repo,user
github rdf gohugoio/hugo --format ttl
```

## Where output goes

A table when you are looking at it, JSONL the moment you pipe it:

```bash
github releases gohugoio/hugo -n 5                    # a table
github releases gohugoio/hugo -n 5 | jq -r .tag       # JSONL
github releases gohugoio/hugo --fields tag,author,published
```

## When something looks wrong

```bash
github doctor              # environment, site, page shape, cache, pacing
github routes              # which surface answers for which route
github page gohugoio/hugo  # everything the page carries, organised
```

Next: the [guides](/guides/) are task-first, and the [CLI reference](/reference/cli/) is the whole surface.
