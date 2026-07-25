---
title: "Release notes"
linkTitle: "Release notes"
description: "What changed in each github release, newest first."
weight: 40
---

What shipped in each release, newest first.
Every tagged version builds the same set of artifacts: archives for Linux, macOS, Windows, and FreeBSD, Linux packages (deb, rpm, apk), a multi-arch container image on GHCR, and entries for the package managers.
Binaries are pure Go, so there is nothing to install alongside them.
Archives and checksums are signed with cosign; see [installation](/getting-started/installation/) for the verify command.

## v0.2.0

The keyless rewrite.
The tool now reads github.com through seven surfaces and hands back records with `github://` addresses and typed edges, instead of shelling out to anything.

- Fifty-odd commands across read, contents, history, people, discover, search, graph, and meta.
- Twenty-four addressable kinds, each with a canonical id, a `github://` URI, and a live URL that round trip in both directions.
- The graph plane: typed edges with a fixed predicate vocabulary, five ranked trust sources, breadth-first crawling, and RDF output as N-Triples, Turtle, JSON-LD, or N-Quads.
- The page plane: `github page` prints everything a page carries, organised into payload, preloaded queries, structured data, meta, microdata, and fragments, and `--deep` merges the fragments a page defers.
- `github routes` prints which surface answers for which route, with the fallback and the reason.
- `github doctor` checks the environment, the site, the page payload, the cache, and the pacing.
- `serve` and `mcp` expose the same operations over HTTP and MCP, from the same handlers.
- A response cache, on by default, fifteen minutes for a page and forever for anything addressed by a full commit sha.
- No token, and a test that fails the build if one ever appears.

`code` and `symbols` are present and report that GitHub does not serve them to a signed-out reader.
They stay because there is no unauthenticated equivalent anywhere, so this is where they will show up if that changes.

## v0.1.0

The first tag, before the rewrite.
