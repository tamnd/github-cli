---
title: "Look inside a repository"
description: "Trees, files, diffs, archives, and the one command that no longer works."
weight: 30
---

Every command here takes a repository, a github.com URL, or a `github://` URI, and every one of them accepts `--rev` to read a branch, a tag, or a commit sha instead of the default branch.

## The tree

```bash
github tree gohugoio/hugo                       # the root
github tree gohugoio/hugo commands              # one directory
github tree gohugoio/hugo --recursive           # the whole thing
github tree gohugoio/hugo --rev v0.164.0
```

`--recursive` is one request per directory, so on anything large it is worth reaching for `github archive` instead and reading the tarball locally.

Ids carry the commit, not the branch, which is what makes them stable:

```
gohugoio/hugo@7df45f615a8ce8611a2288b5ce23eb81a6158b18/commands
```

## One file

`blob` is the metadata: size, language, whether it is binary, whether it is LFS, whether GitHub thinks it is generated, and the raw URL:

```bash
github blob gohugoio/hugo go.mod
github blob gohugoio/hugo go.mod --content    # the bytes, as a field
github blob gohugoio/hugo go.mod --styled     # the rendered lines and highlight spans
```

`cat` is the bytes and nothing else, streamed:

```bash
github cat gohugoio/hugo go.mod
github cat gohugoio/hugo commands/commands.go --rev v0.164.0
github cat https://github.com/gohugoio/hugo/blob/master/go.mod
```

`cat` goes straight through `raw.githubusercontent.com`, so a large file costs no memory and is never cached.

## The README

```bash
github readme gohugoio/hugo          # rendered text
github readme gohugoio/hugo --html   # the markup the page carries
```

GitHub renders the README server-side, so what comes back is what the page shows: badges resolved, relative links rewritten.

## Diffs

```bash
github diff golang/go 6db72bb                       # one commit
github diff gohugoio/hugo v0.163.0 v0.164.0         # a range
github diff https://github.com/gohugoio/hugo/pull/13000  # a pull request
github diff golang/go 6db72bb --patch               # the format-patch mailbox
```

`--patch` gives you something `git am` will apply, with the author, date, and message of every commit in it.

`github compare` is the record form of the same thing, if you want the list of changed files rather than the diff text:

```bash
github compare gohugoio/hugo v0.163.0 v0.164.0
```

## The whole thing at once

```bash
github archive gohugoio/hugo -o hugo.tar.gz
github archive gohugoio/hugo v0.164.0 --format zip -o hugo.zip
github archive gohugoio/hugo | tar tz | head
```

One request to codeload gets the entire tree.
For anything past a few directories this beats walking `github tree --recursive`.

## Symbols, and why they do not work

`github symbols` asks GitHub for the definitions it extracted from a file.
GitHub still runs that extractor, still sets `symbolsEnabled` on every blob, and still renders the button in the page.
It no longer serves the list to a signed-out reader: the `symbols` key comes back null on every file of every repository tried, on both the page and the route JSON, with or without a cache.

The command stays because there is no unauthenticated REST equivalent anywhere, so if that ever comes back this is where it will show up.
Until then it reports that plainly rather than pretending the file has no symbols:

```console
$ github symbols gohugoio/hugo commands/commands.go
  ERROR

  No symbol list for commands/commands.go: GitHub serves none to a signed-out reader, though it still offers the panel.
```

Next: [read what the API will not tell you](/guides/pages/).
