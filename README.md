# github-cli

A command-line for GitHub that reads public data by scraping HTML pages and
Atom feeds. No API key required. No rate limit from the official REST API.

**Not affiliated with GitHub or Microsoft Corporation.**

## Install

```bash
go install github.com/tamnd/github-cli/cmd/github@latest
```

Or grab a prebuilt binary from the
[releases](https://github.com/tamnd/github-cli/releases):

```bash
# Linux/macOS
curl -sSL https://github.com/tamnd/github-cli/releases/latest/download/github_linux_amd64.tar.gz | tar xz
./github --help
```

Or run the container image:

```bash
docker run --rm ghcr.io/tamnd/github:latest --help
```

## Commands

| Command | Description | Source |
|---------|-------------|--------|
| `github trending` | Top trending repositories | HTML |
| `github user <username>` | User profile | HTML |
| `github repos <username>` | User's public repositories | HTML |
| `github repo <owner/repo>` | Repository metadata | HTML |
| `github commits <owner/repo>` | Recent commits | Atom feed |
| `github releases <owner/repo>` | Releases | Atom feed |
| `github tags <owner/repo>` | Tags | Atom feed |
| `github issues <owner/repo>` | Issues | HTML |
| `github pulls <owner/repo>` | Pull requests | HTML |
| `github readme <owner/repo>` | README content | raw.githubusercontent.com |
| `github file <owner/repo> <path>` | Any file | raw.githubusercontent.com |
| `github search <query>` | Search repositories | HTML |
| `github followers <username>` | User followers | HTML |
| `github following <username>` | Users followed by user | HTML |
| `github stars <username>` | Starred repositories | HTML |

## Examples

```bash
# Trending Go repos today
github trending --lang go

# User profile as JSON
github user torvalds -o json

# Recent commits on main
github commits golang/go

# List open issues
github issues golang/go

# Search for HTTP libraries
github search "http client"

# Fetch the README
github readme torvalds/linux
```

## Output formats

Every command supports `-o table|json|jsonl|csv|tsv|url` and `--fields`.

```bash
github trending -o jsonl | jq '.full_name'
github repos torvalds --fields full_name,stars
```

## Notes

- HTML structure can change without notice. Parsers return empty strings on
  missing fields rather than crashing.
- Search may return HTTP 429 from datacenter IPs. The binary exits with code 5
  when throttled. Add `--page 1` and wait a moment before retrying.
- The default pacing is 500 ms between requests. Use `--delay` to adjust.

## License

Apache-2.0
