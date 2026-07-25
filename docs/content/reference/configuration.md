---
title: "Configuration"
description: "Global flags, environment, and where things are kept on disk."
weight: 20
---

There is nothing to configure before the first run, and nothing to authenticate.
Everything below has a working default.

## Global flags

Every command accepts these.

| Flag | Default | Meaning |
|---|---|---|
| `-o`, `--output` | `auto` | Output format: `auto`, `table`, `markdown`, `list`, `json`, `jsonl`, `csv`, `tsv`, `url`, `raw` |
| `-n`, `--limit` | `0` | Stop after N records, 0 for no limit |
| `--fields` | all | Comma-separated columns to keep |
| `--no-header` | off | Omit the header row |
| `--template` | none | Go template applied per record |
| `--color` | `auto` | `auto`, `always`, `never` |
| `-q`, `--quiet` | off | Suppress progress output |
| `-v`, `--verbose` | off | Per-request detail, repeatable |
| `--dry-run` | off | Print what would be done, do nothing |
| `--db` | none | Tee every record into a store, such as `out.db` or a `postgres://` DSN |

## Pacing and network

GitHub publishes no rate limit for the pages, so the defaults are chosen to be quieter than a person browsing with a few tabs open: eight requests a second across four workers.

| Flag | Default | Meaning |
|---|---|---|
| `--rate` | `125ms` | Minimum delay between requests |
| `-j`, `--jobs` | `4` | Concurrent requests |
| `--retries` | `4` | Retry attempts on a rate limit or a 5xx |
| `--timeout` | `30s` | Per-request timeout |
| `--deep` | off | Also fetch the fragments a page defers, and merge what only they carry |

Raising `--rate` is the polite thing to do on a long crawl.
Lowering it is how you get throttled.

## Caching

Every response is cached on disk for fifteen minutes, and a URL that names a full 40-character commit sha is cached forever, because that content cannot change.
A crawl that reaches the same repository from four directions therefore fetches it once.

| Flag | Default | Meaning |
|---|---|---|
| `--cache` | under the data dir | Response cache directory |
| `--no-cache` | off | Bypass the cache for this run |
| `--data-dir` | see below | Override the data directory |

`github doctor` prints where the cache is, how many entries it holds, and how large it has grown.

## Where things are kept

Paths follow the XDG conventions, so the data directory is `~/.local/share/github` and the cache sits under it in `cache/http`.

| Variable | Effect |
|---|---|
| `GITHUB_DATA_DIR` | The data directory, cache and store included |
| `GITHUB_CONFIG_DIR` | The config directory |
| `XDG_DATA_HOME` | The parent of the data directory, when `GITHUB_DATA_DIR` is unset |
| `XDG_CONFIG_HOME` | The parent of the config directory |
| `NO_COLOR` | Any value turns colour off, the same as `--color never` |
| `COLUMNS` | Overrides the detected terminal width |

## Environment variables that are deliberately ignored

`GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_API_TOKEN`, and `GH_ENTERPRISE_TOKEN` are read by nothing here.
`github doctor` looks for them and reports what it found:

```console
$ github doctor
{"name":"auth","status":"ok","detail":"no token in the environment, which is what this tool wants"}
```

That is not a policy you can flip.
There is no code path that attaches a credential to a request, and `gh/noauth_test.go` fails the build if one appears.

## Verifying the setup

```bash
github doctor
```

Five checks: whether a token is in the environment, whether github.com answers, whether the page payload is where the readers expect it, how large the cache has grown, and what pacing this run would use.
If a command comes back empty, this is the first thing to run.
