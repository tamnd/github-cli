---
title: "Output formats"
description: "The output contract every command shares: formats, fields, and templates."
weight: 30
---

Every command renders through one formatter, so the same flags work everywhere.
The default is `auto`: a table when the output is a terminal, JSONL when it is a pipe or a file.
That is why the same command reads well by hand and parses cleanly in a script.

```bash
github tags golang/go -n 3            # a table, because this is a terminal
github tags golang/go -n 3 | wc -l    # JSONL, because this is a pipe
```

## Formats

| Format | Best for |
|---|---|
| `table` | Reading on a terminal |
| `jsonl` | Piping into another tool, one object at a time |
| `json` | Loading a whole result as one array |
| `markdown` | Pasting into an issue or a document |
| `list` | One record per block, when a record is too wide for a row |
| `csv` / `tsv` | Spreadsheets and quick column maths |
| `url` | Feeding addresses into another command |
| `raw` | The record as it came, one compact JSON object per line |

```bash
github tags golang/go -n 3 -o json
github tags golang/go -n 3 -o csv
github repos hugo -n 3 -o url
```

`-o url` is the one worth knowing about, because it turns any listing into input for the next command:

```console
$ github repos hugo -n 3 -o url
https://github.com/gohugoio/hugo
https://github.com/JakeWharton/hugo
https://github.com/gohugoio/hugoDocs
```

The URL is not a table column, so it never takes up room on screen, and `-o url` still finds it.

## What is in a record

Every record carries the same five identity fields before its own:

| Field | What it is |
|---|---|
| `kind` | What sort of thing this is |
| `id` | The canonical id for that kind |
| `uri` | The `github://` address |
| `url` | The live github.com address, where the thing has one |
| `sources` | Every URL that was read to build it |

`sources` is there so you never have to guess where a field came from:

```console
$ github tags golang/go -n 1 -o json | jq -r '.[0].sources[]'
https://github.com/golang/go.git/info/refs?service=git-upload-pack
```

Two more appear when they apply.
`via` names which extraction tier produced each field, and `extra` holds anything GitHub sent that no field claims.
A non-empty `extra` is a bug report: it means the site added something and this tool has not modelled it yet.

## Narrowing columns

```bash
github tags golang/go --fields name,sha
```

`--fields` works on the column formats: `table`, `csv`, `tsv`, `markdown`, and `list`.
It can call back a field the table hides, since a hidden field keeps its name.

It does not touch `json`, `jsonl`, or `raw`, which always carry the whole record.
Narrow those with `jq`:

```bash
github tags golang/go -n 3 | jq '{name, sha}'
```

`--no-header` drops the header row from `table`, `csv`, and `tsv`, which is what a downstream tool usually wants.

## Templating rows

For full control over each line, apply a Go text/template.
The keys are the JSON keys, spelled exactly as they appear in the JSON:

```console
$ github tags golang/go -n 2 --template '{{.name}} {{.sha}}'
go1 6174b5e21e73714c63061e66efdbe180e1c5491d
go1.0.1 2fffba7fe19690e038314d17a117d6b87979c89f
```

Lower case, and underscored where the JSON is underscored: `{{.is_default}}`, not `{{.IsDefault}}`.

## Limiting and streaming

Records stream as they are produced, so `-n` stops the work rather than trimming the output:

```bash
github commits golang/go -n 20
```

On a walk that would otherwise page through years of history, that is the difference between one request and hundreds.

## Writing to a store

`--db` tees every record into a database as it streams, without changing what is printed:

```bash
github crawl gohugoio/hugo --depth 2 --db hugo.db
github crawl gohugoio/hugo --depth 2 --db 'postgres://localhost/graph'
```

## Bytes, not records

`cat`, `readme`, `diff`, and `archive` write bytes rather than records, so the format flags do not apply to them.
`cat` and `archive` stream straight through, which is why a large file costs no memory.
