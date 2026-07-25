---
title: "Walk the graph"
description: "How github turns the site into nodes and edges, and how to crawl them."
weight: 10
---

GitHub is already a graph.
A repository is owned by an org, forked from another repository, written in a language, and tagged with topics.
An issue is authored by a user, labelled, assigned, and closed by a pull request that targets a branch that points at a commit.
`github` reads all of that and gives you nodes and edges.

## Addresses

Every entity has one canonical address, and every command accepts it:

```
github://repo/gohugoio/hugo
github://user/torvalds
github://org/golang
github://issue/golang/go#1
github://pr/golang/go#1
github://commit/golang/go@3e5c0f3
github://file/gohugoio/hugo@master/go.mod
github://release/gohugoio/hugo@v0.164.0
github://topic/static-site-generator
github://gist/aaronsw/1234567
```

`github url <ref>` turns anything into one:

```console
$ github url https://github.com/golang/go/issues/1
{"kind":"issue","id":"golang/go#1","uri":"github://issue/golang/go#1","url":"https://github.com/golang/go/issues/1"}
```

The same three forms work as input everywhere: `owner/name`, a github.com URL, or a `github://` URI.

## One node and its edges

```console
$ github graph gohugoio/hugo
{"uri":"github://repo/gohugoio/hugo","kind":"repo","id":"gohugoio/hugo","label":"gohugoio/hugo","url":"https://github.com/gohugoio/hugo"}
{"subject":"github://repo/gohugoio/hugo","predicate":"ownedBy","object":"github://org/gohugoio","source":"id"}
{"subject":"github://repo/gohugoio/hugo","predicate":"hasTopic","object":"github://topic/blog-engine","source":"payload"}
```

`github edges <ref>` is the same walk with the node and the facts left out, which is what you want when you are building a graph rather than reading one:

```bash
github edges golang/go --predicate hasTopic
```

## The vocabulary

Every edge this tool emits uses a predicate from a fixed set.
Adding a relation means adding a constant first, so the vocabulary never drifts.

| Group | Predicates |
|---|---|
| Ownership | `ownedBy`, `memberOf`, `partOf`, `belongsToPackage` |
| Derivation | `forkOf`, `templateOf`, `mirrorOf`, `dependsOn`, `usedBy` |
| Authorship | `authoredBy`, `committedBy`, `contributedTo`, `assignedTo`, `reviewedBy`, `reviewRequestedFrom`, `mergedBy` |
| Reference | `references`, `closes`, `closedBy`, `duplicateOf`, `subIssueOf`, `linkedTo`, `targetsBranch`, `fromBranch`, `pointsAt`, `parentOf` |
| Classification | `hasTopic`, `hasLabel`, `inMilestone`, `writtenIn`, `licensedUnder`, `relatedTopic` |
| Social | `starredBy`, `follows`, `sponsors`, `reactedWith` |

`writtenIn` and `licensedUnder` take a bare string rather than a URI, because a language and a licence are not github.com entities.

Facts are the literal properties that are not edges at all: `name`, `description`, `homepage`, `created`, `updated`, `stars`, `forks`, `watchers`, `commits`, `avatar`, `state`, `count`.

## Where an edge came from

Every edge records the kind of evidence behind it, because not all of them are equally solid:

| Source | Means |
|---|---|
| `id` | Derived from the identifier itself, so it cannot be wrong: `gohugoio/hugo` is owned by `gohugoio` |
| `payload` | Stated in the embedded React payload or the route JSON |
| `feed` | Stated in an Atom feed |
| `html` | Read out of the rendered markup |
| `text` | Inferred from prose, such as a `#123` in an issue body |

`--min-trust` drops everything below a level, so a pipeline that cannot tolerate a guess asks for `payload` and gets only what the site stated as data:

```bash
github edges golang/go --min-trust payload
```

The default is `html`, which keeps everything except prose inference.

## Crawling

```bash
github crawl golang/go --depth 2
github crawl golang/go --depth 2 --kinds repo,user --limit 200
github crawl golang/go --depth 1 --edges-only
```

The default follow set is `partOf`, `ownedBy`, `forkOf`, `hasTopic`, `authoredBy`.
It deliberately leaves out `references`, `starredBy`, `follows`, `dependsOn`, and `usedBy`, because each of those turns a bounded walk into an unbounded one.
Ask for them by name when you want them:

```bash
github crawl golang/go --depth 2 --follow dependsOn,usedBy
github crawl golang/go --depth 1 --follow all
```

`--dry-run` prints the estimate and stops, which is worth doing before any crawl of something popular:

```bash
github crawl kubernetes/kubernetes --depth 3 --dry-run
```

## Keeping it

Any command can tee its records into a store as it runs, so a crawl doubles as an import:

```bash
github crawl golang/go --depth 2 --db out.db
github crawl golang/go --depth 2 --db 'postgres://localhost/graph'
```

Or write a whole subgraph to one file:

```bash
github export golang/go --depth 2 --out go.jsonl
```

Next: [export it as linked data](/guides/linked-data/).
