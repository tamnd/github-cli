---
title: "Read what the API will not tell you"
description: "The page plane, --deep, and github page."
weight: 40
---

Some of the most useful things on GitHub have no API route at all: trending, contribution calendars, the dependency graph, release download counts, the marketplace, and the language histogram.
Others are served to a browser and not to `/api`.

This is not a scraping problem, because GitHub does not render its data as prose.
A modern GitHub page carries a React payload as JSON in a `<script type="application/json" data-target="react-app.embeddedData">` element, and the same route answers with that JSON directly when you ask for `Accept: application/json`.
`github` reads those, not the text around them.

## The whole page as JSON

```bash
github page gohugoio/hugo
github page https://github.com/golang/go/issues/1
github page https://github.com/trending
```

What comes back is everything the page carries, organised:

| Section | What it is |
|---|---|
| `payload` | The embedded React payload, which is where most of the data lives |
| `queries` | The preloaded Relay query results, which is where issue and pull request pages keep everything |
| `structured_data` | GitHub's own schema.org block |
| `linked_data` | Any `ld+json` on the page |
| `meta` | The `og:`, `twitter:`, and `octolytics-` meta tags |
| `microdata` | itemscope and itemprop, which is how profiles state their fields |
| `partials` | Named HTML fragments the page inlines |
| `fragments` | The deferred fragments the page names for itself and fetches separately |

Narrow it with `--section`:

```bash
github page gohugoio/hugo --section meta
github page gohugoio/hugo --section fragments
github page gohugoio/hugo --section payload | jq '.props.initialPayload.repo'
```

## Preloaded queries

Issue and pull request pages hydrate from Relay, so their data is in named query results rather than in the payload:

```bash
github page https://github.com/golang/go/issues/1 --query IssueViewerViewQuery
```

With no name, `--query` lists what the page has.
Most page kinds preload nothing at all, and the command says so rather than printing an empty list.

## The raw markup

When the question is about the HTML rather than about the data in it:

```bash
github page gohugoio/hugo --raw | grep -o 'data-target="[^"]*"' | sort -u
```

## --deep

A page names fragments it does not include, and fetches them after first paint.
The sponsor button, the language histogram, and the "used by" count all arrive that way.

`--deep` fetches them and merges what only they carry:

```bash
github repo gohugoio/hugo --deep
```

It costs extra requests, which is why it is not the default.
Everything a record can state without them, it states without them.

## Which surface answered

```bash
github routes
```

prints the whole mapping, one line per route, with the primary surface, the fallback, and a note where the choice is not obvious:

```json
{"route":"/{owner}/{repo}/refs","primary":"xhr","fallback":"git","note":"names only, 6 KB against 588 KB"}
{"route":"/{owner}/{repo}/commit/{sha}","primary":"route-json","fallback":"raw","note":"the diff comes from .patch"}
{"route":"/{owner}/{repo}/graphs/contributors","primary":"xhr","note":"answers 202 while it computes, so it polls"}
```

Every record also carries a `sources` array naming the URLs it was built from, so you can always tell where a field came from without re-reading anything.

## When a page moves

GitHub reorganises its pages, and when that happens this tool notices rather than returning something thin and plausible.
`github doctor` checks the plane directly:

```console
$ github doctor
{"name":"page","status":"ok","detail":"the react payload is where it should be, 4 keys in 298283 bytes"}
```

If that check fails, everything else is downstream of it.
If it passes and one command is still empty, the selector for that one route moved, and `github page <ref>` shows what arrived instead.

Next: [use it as a driver](/guides/resource-uris/).
