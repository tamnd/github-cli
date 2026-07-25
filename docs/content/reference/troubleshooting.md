---
title: "Troubleshooting"
description: "The things that actually go wrong, and what each one means."
weight: 40
---

Start here:

```bash
github doctor
```

Five checks, each upstream of a different class of problem: whether a token is in the environment, whether github.com answers, whether the page payload is where the readers expect it, how large the cache has grown, and what pacing this run would use.

## A command comes back empty

If `doctor` says the page check failed, GitHub moved its payload and everything is downstream of that.
Open an issue; the fix is in the readers, not in your setup.

If `doctor` is green and one command is still empty, that one route moved.
Look at what actually arrived:

```bash
github page gohugoio/hugo --section payload | jq 'keys'
```

That prints the blocks the page carries now, which is usually enough to see which selector stopped matching.

## Requests start failing

GitHub throttles like any public site.
The tool already paces at eight requests a second across four workers and retries the transient failures, but a long crawl can still hit a wall.
Slow down:

```bash
github crawl golang --depth 2 --rate 500ms --jobs 2
```

A burst of 429 or 5xx is the site asking you to wait, not a defect.
Exit code 5 means rate limited.

## Something needs a session

Parts of GitHub are not served to a signed-out reader at all.
Code search is the big one, and file symbols are the other.
Those commands say so rather than returning an empty result that looks like an answer:

```console
$ github code 'func main'
  ERROR

  Not available without a session: code search, the route answers 200 with an
  empty result set to an anonymous client, which is indistinguishable from no
  matches.
```

There is no flag that changes this, because there is no credential anywhere in the tool.
For anything gated, private, or write-shaped, use the official `gh`.

## The wrong kind of reference

```console
$ github org https://github.com/golang/go
  ERROR

  Wrong kind: "https://github.com/golang/go" is a repo, not an org.
```

A URL states its own kind, so it is taken at its word.
A bare word is a guess, which is why `github org golang` works and the URL form does not.

To find out what a reference names before running anything:

```bash
github url https://github.com/golang/go/blob/master/go.mod
```

## Stale data

Responses are cached for fifteen minutes, and anything addressed by a full commit sha is cached forever, because that content cannot change.
If you want the live answer now:

```bash
github repo gohugoio/hugo --no-cache
```

The cache directory and its size are in `github doctor`.
Deleting it is safe.

## The first contributors call hangs

It is not hanging.
GitHub computes the contributor graph on demand and answers 202 with an empty body while it works, so the first call on a large repository waits a few seconds and every call after it is instant.

## `--recursive` is slow

`github tree --recursive` is one request per directory.
On anything large, get the whole thing in one request instead:

```bash
github archive gohugoio/hugo | tar tz
```

## The binary is not on your PATH

`go install` puts it in `$(go env GOPATH)/bin`, usually `~/go/bin`.
A release archive leaves it wherever you unpacked it.
See [installation](/getting-started/installation/).

The Homebrew formula is `tamnd/tap/github-cli`.
`brew install github` is GitHub Desktop, which is a different thing entirely.

## Seeing what it actually did

```bash
github repo gohugoio/hugo -v
```

`-v` prints every request: the URL, the surface it used, the status, whether it came from the cache, and how long it took.
That is usually enough to tell a throttle apart from a genuinely empty result.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Fine |
| 1 | Something unclassified |
| 2 | A usage error: bad flags, or the wrong kind of reference |
| 3 | No results |
| 4 | Needs a session, which this tool does not have |
| 5 | Rate limited |
| 6 | Not found |
| 7 | Not supported, usually because GitHub does not serve it publicly |
| 8 | A network failure |

They are the same across the command line, `serve`, and `mcp`, because they come from the same errors.
