---
title: "CLI"
description: "Every command, grouped the way the help groups them."
weight: 10
---

```
github <command> [arguments] [--flags]
```

Run `github <command> --help` for the exact flags on any command.
Every command that takes a `<ref>` accepts the same three forms: `owner/name`, a github.com URL, or a `github://` URI.
Most of them also accept any URL from the repository, so a file URL or an issue URL names the repository it lives in.

## Read

| Command | What it does |
|---|---|
| `get <ref>` | Read whatever a reference points at |
| `repo <ref>` | Read one repository with every field the page states |
| `user <name>` | Read one user profile |
| `org <name>` | Read one organization |
| `issue <ref> [num]` | Read one issue with its body, labels, and participants |
| `pr <ref> [num]` | Read one pull request, merge state and review state included |
| `discussion <ref> [num]` | Read one discussion, its category, and its answer |
| `commit <ref> [sha]` | Read one commit, with its author, verification, and changed files |
| `compare <ref> [base] [head]` | Compare two refs and list what changed between them |
| `release <ref> [tag]` | Read one release by tag, or the latest one |

## Contents

| Command | What it does |
|---|---|
| `tree <ref> [path]` | List a directory, or the whole tree with `--recursive` |
| `blob <ref> [path]` | Read one file's metadata, and its bytes on request |
| `cat <repo> <path>` | Write one file from a repository to stdout |
| `readme <repo>` | Write a repository's README to stdout |
| `diff <ref> [base] [head]` | Write a commit's or a range's diff to stdout |
| `archive <repo> [ref]` | Download a repository as a tarball or a zip |
| `symbols <ref> [path]` | List the definitions GitHub extracted from a file |

`symbols` returns nothing today.
GitHub still extracts them and no longer serves the list to a signed-out reader, and the command says so rather than pretending the file has none.
See [look inside a repository](/guides/contents/).

## History

| Command | What it does |
|---|---|
| `commits <ref>` | Walk a repository's history |
| `branches <ref>` | List branches |
| `tags <ref>` | List tags |
| `refs <ref>` | List every ref in a repository |
| `releases <ref>` | List releases |
| `timeline <ref> [num]` | List everything that happened on one issue or pull request |

## People

| Command | What it does |
|---|---|
| `owned <name>` | List an account's repositories as the profile shows them |
| `stars <name>` | List what an account has starred |
| `followers <name>` | List who follows an account |
| `following <name>` | List who an account follows |
| `members <name>` | List an organization's public members |
| `activity <ref>` | Read a public activity feed |
| `contributions <name>` | Read an account's contribution calendar, one record per day |
| `gists <name>` | List an account's public gists |
| `gist <ref>` | Read one gist with its files |

## Discover

| Command | What it does |
|---|---|
| `trending` | List what is trending |
| `topic <name>` | Read one topic page |
| `forks <ref>` | List a repository's public forks |
| `contributors <ref>` | List contributors with their commit, addition, and deletion counts |
| `languages <ref>` | Report the language histogram, one record per language |
| `stats <ref>` | Report a repository's counts and nothing else |

`contributors` reads a route that answers 202 while GitHub computes the numbers, so the first call on a large repository waits a few seconds and every call after it is instant.

## Search

| Command | What it does |
|---|---|
| `search <query>` | Search every type at once |
| `repos [query]` | Search repositories |
| `issues [query]` | Search issues |
| `prs [query]` | Search pull requests |
| `users [query]` | Search users and organizations |
| `topics [query]` | Search topics |
| `packages [query]` | Search published packages |
| `actions [query]` | Search the marketplace for actions |
| `wikis [query]` | Search wiki pages |
| `code <query>` | Search code, which needs a session and so does not work here |

`code` is listed because leaving it out would be the more confusing choice.
GitHub gates code search behind a login, so the command reports that rather than returning an empty result that looks like an answer.

## Graph

| Command | What it does |
|---|---|
| `graph <ref>` | Emit the node, edges, and facts for one entity |
| `edges <ref>` | Emit only the edges for one entity |
| `crawl <ref>` | Walk the graph breadth-first from a seed |
| `export <ref>` | Write a whole graph to one file |
| `rdf <ref>` | Write an entity as RDF triples |
| `deps <ref>` | List what a repository depends on |
| `dependents <ref>` | List the repositories that depend on this one |

See [walk the graph](/guides/graph/) and [export linked data](/guides/linked-data/).

## Meta

| Command | What it does |
|---|---|
| `url <ref>` | Parse a reference and report what it names |
| `page <ref>` | Print everything a page carries, organised |
| `routes` | List which surface answers for which route |
| `doctor` | Check the environment, the site, and the cache |
| `version` | Print version information |

## Servers

| Command | What it does |
|---|---|
| `serve` | Serve the operations over HTTP as NDJSON |
| `mcp` | Run as an MCP server over stdio |

Both expose the same operations as the command line, from the same handlers.
Arguments go in the query string under `serve`, because most of them contain a slash and a path would swallow it:

```bash
github serve --addr :7777

curl 'localhost:7777/v1/repo?ref=gohugoio/hugo'
curl 'localhost:7777/v1/blob?ref=gohugoio/hugo&path=go.mod'
curl 'localhost:7777/v1/openapi.json'
```
