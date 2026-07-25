// Package gitproto reads the git smart HTTP advertisement.
//
// This is the one surface on github.com that answers "what refs does this
// repository have" completely, in a single request, with no login, no page
// limit, and no truncation. The branches page caps its list and says so, the
// refs fragment gives names and nothing else, and the tags feed gives ten. The
// advertisement gives every branch, every tag, every pull request head, and the
// object each one points at.
//
// The format is pkt-line, which is four hex digits of length followed by that
// many bytes including the four. 0000 is a flush packet. The first line after
// the service header carries the capability list after a NUL byte, and one of
// those capabilities is symref=HEAD:refs/heads/main, which is where the default
// branch comes from for free.
package gitproto

import (
	"errors"
	"strconv"
	"strings"
)

// Ref is one advertised ref.
type Ref struct {
	Name string
	SHA  string
	// Peeled is set on the ^{} entry of an annotated tag: the tag object's own
	// SHA is in SHA and the commit it points at is here. A lightweight tag has
	// no peeled entry, which is how you tell the two apart.
	Peeled string
}

// Advertisement is a parsed info/refs response.
type Advertisement struct {
	Refs []Ref
	// Head is the SHA the HEAD line advertised.
	Head string
	// DefaultBranch is the target of symref=HEAD:..., short form, so "main"
	// rather than "refs/heads/main". Empty when the server did not advertise it.
	DefaultBranch string
	Capabilities  []string
}

// ErrNotGit is returned when the body is not an advertisement. It usually means
// github.com answered with an HTML page, which is what a private or missing
// repository does.
var ErrNotGit = errors.New("not a git upload-pack advertisement")

// Parse reads an info/refs?service=git-upload-pack body.
//
// Peeled entries are folded into the ref they belong to rather than kept as
// separate refs, because "refs/tags/v1.0.0^{}" is not a ref anybody can check
// out and a caller that has to know about the fold is a caller doing the
// parser's job.
func Parse(body []byte) (*Advertisement, error) {
	lines, err := pktLines(body)
	if err != nil {
		return nil, err
	}
	ad := &Advertisement{}
	byName := map[string]int{}
	for _, line := range lines {
		line = strings.TrimRight(line, "\n")
		if line == "" || strings.HasPrefix(line, "# service=") {
			continue
		}
		// The first ref line carries the capabilities after a NUL.
		if i := strings.IndexByte(line, 0); i >= 0 {
			ad.Capabilities = strings.Fields(line[i+1:])
			for _, c := range ad.Capabilities {
				if v, ok := strings.CutPrefix(c, "symref=HEAD:"); ok {
					ad.DefaultBranch = shortName(v)
				}
			}
			line = line[:i]
		}
		sha, name, ok := strings.Cut(line, " ")
		if !ok || len(sha) != 40 {
			continue
		}
		if name == "HEAD" {
			ad.Head = sha
			continue
		}
		if base, ok := strings.CutSuffix(name, "^{}"); ok {
			if i, seen := byName[base]; seen {
				ad.Refs[i].Peeled = sha
			}
			continue
		}
		byName[name] = len(ad.Refs)
		ad.Refs = append(ad.Refs, Ref{Name: name, SHA: sha})
	}
	if len(ad.Refs) == 0 && ad.Head == "" {
		return nil, ErrNotGit
	}
	return ad, nil
}

// pktLines splits a pkt-line stream into its payloads.
//
// A malformed length is a hard error rather than a skipped line. Half-reading a
// binary protocol and carrying on gives a ref list that looks fine and is
// missing entries, which is worse than not answering.
func pktLines(body []byte) ([]string, error) {
	var out []string
	for len(body) > 0 {
		if len(body) < 4 {
			return nil, ErrNotGit
		}
		n, err := strconv.ParseUint(string(body[:4]), 16, 32)
		if err != nil {
			return nil, ErrNotGit
		}
		if n == 0 {
			// Flush packet. The advertisement carries one after the service
			// header and one at the end, and neither ends the stream for us.
			body = body[4:]
			continue
		}
		if n < 4 || int(n) > len(body) {
			return nil, ErrNotGit
		}
		out = append(out, string(body[4:n]))
		body = body[n:]
	}
	return out, nil
}

// Branches returns the refs under refs/heads, with the prefix stripped.
func (a *Advertisement) Branches() []Ref { return a.under("refs/heads/") }

// Tags returns the refs under refs/tags, with the prefix stripped. An annotated
// tag keeps both SHAs: SHA is the tag object and Peeled is the commit.
func (a *Advertisement) Tags() []Ref { return a.under("refs/tags/") }

// PullHeads returns the refs under refs/pull, which github.com advertises for
// every pull request ever opened against the repository. The name keeps its
// shape, "1234/head" or "1234/merge", because those two are different objects
// and flattening them would lose that.
func (a *Advertisement) PullHeads() []Ref { return a.under("refs/pull/") }

func (a *Advertisement) under(prefix string) []Ref {
	var out []Ref
	for _, r := range a.Refs {
		if name, ok := strings.CutPrefix(r.Name, prefix); ok {
			r.Name = name
			out = append(out, r)
		}
	}
	return out
}

func shortName(full string) string {
	for _, p := range []string{"refs/heads/", "refs/tags/", "refs/remotes/"} {
		if s, ok := strings.CutPrefix(full, p); ok {
			return s
		}
	}
	return full
}
