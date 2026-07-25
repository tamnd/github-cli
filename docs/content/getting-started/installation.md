---
title: "Installation"
description: "Install github from a release, a package manager, go install, or source."
weight: 20
---

The binary is called `github`.
It does not replace the official `gh`, which does the authenticated half of the site far better.
This one does the public half without asking you to log in.

## With Go

```bash
go install github.com/tamnd/github-cli/cmd/github@latest
```

That puts `github` in `$(go env GOPATH)/bin`, which is `~/go/bin` unless you moved it.
Make sure that directory is on your `PATH`.

## Package managers

```bash
# Homebrew (macOS)
brew install --cask tamnd/tap/github-cli

# Scoop (Windows)
scoop bucket add tamnd https://github.com/tamnd/scoop-bucket
scoop install github-cli

# apt (Debian, Ubuntu)
curl -fsSL https://tamnd.github.io/linux-repo/gpg.key | sudo gpg --dearmor -o /usr/share/keyrings/tamnd.gpg
echo "deb [signed-by=/usr/share/keyrings/tamnd.gpg] https://tamnd.github.io/linux-repo/apt stable main" | sudo tee /etc/apt/sources.list.d/tamnd.list
sudo apt update && sudo apt install github-cli

# dnf (Fedora, RHEL)
sudo dnf config-manager --add-repo https://tamnd.github.io/linux-repo/dnf/tamnd.repo
sudo dnf install github-cli
```

The Homebrew formula is a cask rather than a formula because `brew install github` already means GitHub Desktop.

## Prebuilt binaries

Every [release](https://github.com/tamnd/github-cli/releases) carries archives for Linux, macOS, Windows, and FreeBSD on amd64 and arm64, plus deb, rpm, and apk packages for Linux.
Download, unpack, put `github` on your `PATH`, done.

Each release also ships an SBOM and a `checksums.txt` signed with keyless [cosign](https://docs.sigstore.dev/), if you want to verify before running:

```bash
cosign verify-blob checksums.txt \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-identity-regexp 'https://github\.com/tamnd/github-cli/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Container image

```bash
docker run --rm ghcr.io/tamnd/github-cli:latest repo gohugoio/hugo
```

## From source

```bash
git clone https://github.com/tamnd/github-cli
cd github-cli
make build        # produces ./bin/github
./bin/github version
```

## Checking the install

```bash
github version
github doctor
```

`doctor` is the more useful of the two.
It reports whether the site answers, whether the page still carries the payload every reader expects, whether the cache is writable, what pacing this run is using, and whether you have a token set that this tool is about to ignore.
