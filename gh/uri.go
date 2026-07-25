package gh

import (
	"net/url"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
)

// uri.go is the whole identity scheme. Everything downstream, the graph, the
// RDF subjects, the cache keys, the `github get` dispatch, resolves through
// here, so an id that parses wrong is a bug that shows up everywhere at once.
//
// The grammar has three separators and each one means exactly one thing:
//
//	/  separates a namespace from a name, and a repository from a path
//	#  introduces a thread number
//	@  introduces a git revision
//
// A path may contain slashes but never a `#` or an `@` in a position that
// matters, because the revision always comes immediately after the repository
// and the number is always last. So parsing splits on `#`, then on the first
// `@` after the second `/`, and the rest is a path. That is the entire trick.

// The kinds. Twenty are addressable as github:// URIs; compare is a recognised
// route that names a range rather than a thing, and is here because people
// paste compare URLs.
const (
	KindRepo       = "repo"
	KindUser       = "user"
	KindOrg        = "org"
	KindIssue      = "issue"
	KindPR         = "pr"
	KindDiscussion = "discussion"
	KindCommit     = "commit"
	KindBranch     = "branch"
	KindTag        = "tag"
	KindRelease    = "release"
	KindFile       = "file"
	KindTree       = "tree"
	KindLabel      = "label"
	KindMilestone  = "milestone"
	KindTopic      = "topic"
	KindGist       = "gist"
	KindPackage    = "package"
	KindAction     = "action"
	KindWiki       = "wiki"
	KindAdvisory   = "advisory"
	KindCompare    = "compare"

	// These three name records GitHub derives rather than serves. There is no
	// page whose address is one contributor's statistics or one day of a
	// calendar, so they get a URI and no canonical URL, and Locate points at
	// the page they were read from instead of inventing one.
	KindContributor  = "contributor"
	KindContribution = "contribution"
	KindEvent        = "event"
)

// Kinds is the whole set, in the order above, for help text and for the error a
// bad kind produces. Listing them is the difference between an error a reader
// can act on and one that sends them to the source.
var Kinds = []string{
	KindRepo, KindUser, KindOrg, KindIssue, KindPR, KindDiscussion,
	KindCommit, KindBranch, KindTag, KindRelease, KindFile, KindTree,
	KindLabel, KindMilestone, KindTopic, KindGist, KindPackage, KindAction,
	KindWiki, KindAdvisory, KindCompare,
	KindContributor, KindContribution, KindEvent,
}

// Scheme is the URI scheme this package mints and dereferences.
const Scheme = "github"

// Ident is a parsed reference: what kind of thing, its canonical id, and the
// fragment the URL carried. The fragment never changes the kind. A link to
// #issuecomment-66046293 is still a link to the issue, and a link to #L10-L20
// is still a link to the file, so the anchor is recorded and set aside.
type Ident struct {
	Kind   string `json:"kind"             table:"kind"`
	ID     string `json:"id"               table:"id"`
	Anchor string `json:"anchor,omitempty" table:"anchor"`
	URI    string `json:"uri"              table:"uri"`
	URL    string `json:"url"              table:"url,url"`
}

// reserved lists the top-level github.com paths that are site routes rather
// than accounts. Without it, `github get https://github.com/topics/go` would
// classify topics as a user, which is the kind of wrong answer that only shows
// up in someone else's script.
var reserved = map[string]bool{
	"about": true, "advisories": true, "apps": true, "collections": true,
	"contact": true, "customer-stories": true, "dashboard": true, "enterprise": true,
	"events": true, "explore": true, "features": true, "issues": true, "join": true,
	"login": true, "logout": true, "marketplace": true, "new": true, "notifications": true,
	"orgs": true, "pricing": true, "pulls": true, "search": true, "security": true,
	"settings": true, "site": true, "sponsors": true, "stars": true, "topics": true,
	"trending": true, "readme": true, "codespaces": true, "sessions": true,
}

// Classify turns anything a person might paste into a kind and an id. It does
// no I/O, and it never fails on a well-formed github.com URL.
//
// Two of its answers are guesses and both are documented as such. A bare word
// is a user, because a pure function cannot tell a user from an organization
// without asking. A bare owner/name is a repository. `github get` reads the
// page and returns a record whose Kind is the truth; classification is a
// routing hint, not an answer.
func Classify(input string) (kind, id string, err error) {
	r, err := Parse(input)
	if err != nil {
		return "", "", err
	}
	return r.Kind, r.ID, nil
}

// Parse is Classify with the fragment and the derived forms kept.
func Parse(input string) (Ident, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return Ident{}, errs.Usage("empty reference")
	}
	var anchor string
	switch {
	case strings.HasPrefix(s, Scheme+"://"):
		kind, id, a, err := parseURI(s)
		if err != nil {
			return Ident{}, err
		}
		return finish(kind, id, a)
	case strings.Contains(s, "://"):
		kind, id, a, err := parseURL(s)
		if err != nil {
			return Ident{}, err
		}
		return finish(kind, id, a)
	}
	// A bare reference. Strip a fragment the same way a URL would, so
	// golang/go#1#issuecomment-1 and a pasted anchor both behave.
	if i := strings.Index(s, "#"); i >= 0 {
		if j := strings.Index(s[i+1:], "#"); j >= 0 {
			anchor = s[i+1+j+1:]
			s = s[:i+1+j]
		}
	}
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, "/")
	kind, id, err := classifyBare(s)
	if err != nil {
		return Ident{}, err
	}
	return finish(kind, id, anchor)
}

func finish(kind, id, anchor string) (Ident, error) {
	u, err := Locate(kind, id)
	if err != nil {
		return Ident{}, err
	}
	return Ident{Kind: kind, ID: id, Anchor: anchor, URI: URI(kind, id), URL: u}, nil
}

// URI renders the github:// form. It is a string join and not a url.URL,
// because ids contain `#` and `@` on purpose and url.URL would escape them.
func URI(kind, id string) string { return Scheme + "://" + kind + "/" + id }

func parseURI(s string) (kind, id, anchor string, err error) {
	rest := strings.TrimPrefix(s, Scheme+"://")
	if i := strings.LastIndex(rest, "#issuecomment-"); i >= 0 {
		anchor, rest = rest[i+1:], rest[:i]
	}
	kind, id, ok := strings.Cut(rest, "/")
	if !ok || kind == "" || id == "" {
		return "", "", "", errs.Usage("not a %s:// URI: %q", Scheme, s)
	}
	if !knownKind(kind) {
		return "", "", "", errs.Usage("unknown kind %q; the kinds are %s", kind, strings.Join(Kinds, ", "))
	}
	return kind, strings.TrimSuffix(id, "/"), anchor, nil
}

// knownKind reads the same list the error message prints, so a kind cannot be
// accepted here and left out of the list a reader is shown.
func knownKind(k string) bool {
	for _, want := range Kinds {
		if k == want {
			return true
		}
	}
	return false
}

// parseURL handles every github.com host that serves content, plus the two
// static hosts. Query strings are dropped: ?tab=repositories names a tab on a
// profile, not a different profile.
func parseURL(raw string) (kind, id, anchor string, err error) {
	u, perr := url.Parse(raw)
	if perr != nil {
		return "", "", "", errs.Usage("not a URL: %q, %v", raw, perr)
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	path := strings.Trim(u.Path, "/")
	anchor = u.Fragment

	switch host {
	case "raw.githubusercontent.com":
		// /{owner}/{repo}/{ref}/{path...}
		p := strings.Split(path, "/")
		if len(p) < 4 {
			return "", "", "", errs.Usage("not a raw file URL: %q", raw)
		}
		return KindFile, p[0] + "/" + p[1] + "@" + p[2] + "/" + strings.Join(p[3:], "/"), anchor, nil
	case "gist.github.com", "gist.githubusercontent.com":
		p := strings.Split(path, "/")
		if len(p) == 0 || p[0] == "" {
			return "", "", "", errs.Usage("no gist named in %q", raw)
		}
		// A gist URL is either /{id} or /{owner}/{id}. The id is the last
		// segment that looks like one.
		return KindGist, p[len(p)-1], anchor, nil
	case "github.com", "codeload.github.com":
		return classifyPath(path, anchor, raw)
	default:
		return "", "", "", errs.Usage("not a github.com URL: %q", raw)
	}
}

func classifyPath(path, anchor, raw string) (kind, id, a string, err error) {
	if path == "" {
		return "", "", "", errs.Usage("no resource named in %q", raw)
	}
	p := strings.Split(path, "/")

	// Site routes first, so a repository named "topics" cannot shadow one.
	switch p[0] {
	case "topics":
		if len(p) >= 2 {
			return KindTopic, p[1], anchor, nil
		}
	case "marketplace":
		if len(p) >= 3 && p[1] == "actions" {
			return KindAction, p[2], anchor, nil
		}
	case "advisories":
		if len(p) >= 2 {
			return KindAdvisory, p[1], anchor, nil
		}
	case "orgs":
		if len(p) >= 2 {
			return KindOrg, p[1], anchor, nil
		}
	}
	if reserved[p[0]] {
		return "", "", "", errs.Usage("no resource behind %q; it is a github.com page, not a thing this tool reads", raw)
	}
	if len(p) == 1 {
		return KindUser, p[0], anchor, nil
	}
	owner, name := p[0], strings.TrimSuffix(p[1], ".git")
	repo := owner + "/" + name
	if len(p) == 2 {
		return KindRepo, repo, anchor, nil
	}

	rest := p[2:]
	switch rest[0] {
	case "issues":
		if len(rest) >= 2 && isNumber(rest[1]) {
			return KindIssue, repo + "#" + rest[1], anchor, nil
		}
		return KindRepo, repo, anchor, nil
	case "pull", "pulls":
		if len(rest) >= 2 && isNumber(rest[1]) {
			return KindPR, repo + "#" + rest[1], anchor, nil
		}
		return KindRepo, repo, anchor, nil
	case "discussions":
		if len(rest) >= 2 && isNumber(rest[1]) {
			return KindDiscussion, repo + "#" + rest[1], anchor, nil
		}
		return KindRepo, repo, anchor, nil
	case "commit":
		if len(rest) >= 2 {
			return KindCommit, repo + "@" + rest[1], anchor, nil
		}
	case "commits":
		if len(rest) >= 2 {
			return KindBranch, repo + "@" + rest[1], anchor, nil
		}
		return KindRepo, repo, anchor, nil
	case "tree":
		if len(rest) == 2 {
			return KindBranch, repo + "@" + rest[1], anchor, nil
		}
		if len(rest) > 2 {
			return KindTree, repo + "@" + rest[1] + "/" + strings.Join(rest[2:], "/"), anchor, nil
		}
	case "blob", "raw", "blame":
		if len(rest) >= 3 {
			return KindFile, repo + "@" + rest[1] + "/" + strings.Join(rest[2:], "/"), anchor, nil
		}
	case "releases":
		if len(rest) >= 3 && rest[1] == "tag" {
			return KindRelease, repo + "@" + strings.Join(rest[2:], "/"), anchor, nil
		}
		if len(rest) >= 3 && rest[1] == "download" {
			return KindRelease, repo + "@" + rest[2], anchor, nil
		}
		return KindRepo, repo, anchor, nil
	case "labels":
		if len(rest) >= 2 {
			name, _ := url.PathUnescape(strings.Join(rest[1:], "/"))
			return KindLabel, repo + "/" + name, anchor, nil
		}
	case "milestone":
		if len(rest) >= 2 {
			return KindMilestone, repo + "/" + rest[1], anchor, nil
		}
	case "wiki":
		if len(rest) >= 2 {
			return KindWiki, repo + "/" + strings.Join(rest[1:], "/"), anchor, nil
		}
		return KindWiki, repo + "/Home", anchor, nil
	case "pkgs":
		// /{owner}/{repo}/pkgs/{type}/{name}, where the name is usually the
		// repository and the thing inside it and so carries a %2F. Parsing
		// decoded that back into a slash before the split, so the name is
		// everything from the type onwards rather than the last segment.
		if len(rest) >= 3 {
			return KindPackage, repo + "/" + strings.Join(rest[2:], "/"), anchor, nil
		}
	case "compare":
		if len(rest) >= 2 {
			return KindCompare, repo + "@" + strings.Join(rest[1:], "/"), anchor, nil
		}
	case "archive":
		if len(rest) >= 2 {
			ref := strings.TrimSuffix(strings.TrimSuffix(rest[len(rest)-1], ".zip"), ".tar.gz")
			return KindBranch, repo + "@" + ref, anchor, nil
		}
	}
	// Every other repository tab (actions, settings, network, graphs, stargazers)
	// is a view of the repository, so that is what it resolves to.
	return KindRepo, repo, anchor, nil
}

// classifyBare reads the compact forms people type: owner/name, owner/name#12,
// owner/name@sha, owner/name@ref/path, and a bare login.
func classifyBare(s string) (kind, id string, err error) {
	if s == "" {
		return "", "", errs.Usage("empty reference")
	}
	if strings.HasPrefix(strings.ToUpper(s), "GHSA-") {
		return KindAdvisory, s, nil
	}
	if base, num, ok := strings.Cut(s, "#"); ok {
		if !isNumber(num) {
			return "", "", errs.Usage("the part after # must be a number, in %q", s)
		}
		if strings.Count(base, "/") != 1 {
			return "", "", errs.Usage("not a thread reference: %q, which should look like owner/name#123", s)
		}
		// Bare owner/name#N is an issue, which is the same guess github.com
		// makes: /issues/N redirects to /pull/N when N is a pull request.
		return KindIssue, base + "#" + num, nil
	}
	if i := strings.Index(s, "@"); i >= 0 && strings.Count(s[:i], "/") == 1 {
		repo, rev := s[:i], s[i+1:]
		if rev == "" {
			return "", "", errs.Usage("nothing after the @ in %q", s)
		}
		if r, path, ok := strings.Cut(rev, "/"); ok {
			return KindFile, repo + "@" + r + "/" + path, nil
		}
		if isSHA(rev) {
			return KindCommit, repo + "@" + rev, nil
		}
		// A short ref with no path is a branch by default. `github tag` and
		// `github release` name their own kind and override this.
		return KindBranch, repo + "@" + rev, nil
	}
	switch strings.Count(s, "/") {
	case 0:
		if reserved[s] {
			return "", "", errs.Usage("no account behind %q; it is a github.com page, not a profile", s)
		}
		return KindUser, s, nil
	case 1:
		return KindRepo, s, nil
	default:
		// owner/name/something. When "something" is one of github.com's own
		// route words this is a URL with the host left off, and the URL parser
		// already knows exactly what it means, so hand it over rather than
		// guess. That is what makes `github url cli/cli/blob/trunk/go.mod` and
		// the full URL agree, which they did not when this guessed first.
		p := strings.SplitN(s, "/", 3)
		if word, _, _ := strings.Cut(p[2], "/"); routeWord[word] {
			kind, id, _, err := classifyPath(s, "", s)
			return kind, id, err
		}
		// Anything else is ambiguous between wiki, label, milestone, and
		// package, so it goes to the one whose ids are numeric when it is
		// numeric and to a wiki page otherwise.
		if isNumber(p[2]) {
			return KindMilestone, s, nil
		}
		return KindWiki, s, nil
	}
}

// routeWord is the set of third segments that make a bare reference a route
// rather than a name. It is exactly the case list of the switch in
// classifyPath, and the two have to stay in step: a word here that the switch
// does not handle resolves to the repository instead of to the thing named.
var routeWord = map[string]bool{
	"issues": true, "pull": true, "pulls": true, "discussions": true,
	"commit": true, "commits": true, "tree": true, "blob": true, "raw": true,
	"blame": true, "releases": true, "labels": true, "milestone": true,
	"wiki": true, "pkgs": true, "compare": true, "archive": true,
}

// Locate turns a kind and id back into the canonical github.com URL.
// Locate(Classify(u)) is the canonical form of u, which is what makes -o url
// safe to pipe back into the tool.
func Locate(kind, id string) (string, error) {
	if id == "" {
		return "", errs.Usage("no id given for a %s", kind)
	}
	switch kind {
	case KindRepo:
		return BaseURL + "/" + id, nil
	case KindUser, KindOrg:
		return BaseURL + "/" + id, nil
	case KindIssue, KindPR, KindDiscussion:
		repo, num, ok := strings.Cut(id, "#")
		if !ok {
			return "", errs.Usage("missing number: the %s id %q needs one", kind, id)
		}
		seg := map[string]string{KindIssue: "issues", KindPR: "pull", KindDiscussion: "discussions"}[kind]
		return BaseURL + "/" + repo + "/" + seg + "/" + num, nil
	case KindCommit:
		repo, sha, ok := cutRev(id)
		if !ok {
			return "", errs.Usage("commit id %q is missing its sha", id)
		}
		return BaseURL + "/" + repo + "/commit/" + sha, nil
	case KindBranch:
		repo, ref, ok := cutRev(id)
		if !ok {
			return "", errs.Usage("branch id %q is missing its ref", id)
		}
		return BaseURL + "/" + repo + "/tree/" + ref, nil
	case KindTag, KindRelease:
		repo, tag, ok := cutRev(id)
		if !ok {
			return "", errs.Usage("missing tag: the %s id %q needs one", kind, id)
		}
		return BaseURL + "/" + repo + "/releases/tag/" + tag, nil
	case KindFile, KindTree:
		repo, ref, path, ok := SplitPathID(id)
		if !ok {
			return "", errs.Usage("wrong shape: the %s id %q is not owner/name@ref/path", kind, id)
		}
		seg := "blob"
		if kind == KindTree {
			seg = "tree"
		}
		if path == "" {
			return BaseURL + "/" + repo + "/tree/" + ref, nil
		}
		return BaseURL + "/" + repo + "/" + seg + "/" + ref + "/" + path, nil
	case KindLabel:
		repo, name, ok := cutRepoRest(id)
		if !ok {
			return "", errs.Usage("label id %q is not owner/name/label", id)
		}
		return BaseURL + "/" + repo + "/labels/" + url.PathEscape(name), nil
	case KindMilestone:
		repo, num, ok := cutRepoRest(id)
		if !ok {
			return "", errs.Usage("milestone id %q is not owner/name/number", id)
		}
		return BaseURL + "/" + repo + "/milestone/" + num, nil
	case KindWiki:
		repo, page, ok := cutRepoRest(id)
		if !ok {
			return "", errs.Usage("wiki id %q is not owner/name/page", id)
		}
		return BaseURL + "/" + repo + "/wiki/" + page, nil
	case KindPackage:
		repo, name, ok := cutRepoRest(id)
		if !ok {
			return "", errs.Usage("package id %q is not owner/name/package", id)
		}
		// The name is escaped because a container package is usually called
		// after the repository and the thing inside it, so it has a slash in
		// it. GitHub wants that slash as %2F: the unescaped form 404s and the
		// escaped one is the page.
		return BaseURL + "/" + repo + "/pkgs/container/" + url.PathEscape(name), nil
	case KindTopic:
		return BaseURL + "/topics/" + id, nil
	case KindAction:
		return BaseURL + "/marketplace/actions/" + id, nil
	case KindAdvisory:
		return BaseURL + "/advisories/" + id, nil
	case KindGist:
		return GistURL + "/" + id, nil
	case KindContributor:
		// The id is owner/name@login and the page that states it is the graph,
		// which is the whole roster rather than the one row. That is the
		// closest true address, so it is the one given.
		repo, _, ok := cutRev(id)
		if !ok {
			return "", errs.Usage("contributor id %q is not owner/name@login", id)
		}
		return BaseURL + "/" + repo + "/graphs/contributors", nil
	case KindContribution:
		login, _, ok := cutRev(id)
		if !ok {
			return "", errs.Usage("contribution id %q is not login@date", id)
		}
		return BaseURL + "/" + login, nil
	case KindEvent:
		// An event's address is the thing it happened to, which the feed states
		// per entry and no rule can reconstruct from the id.
		return "", errs.Usage("an event has no address of its own; read its url field")
	case KindCompare:
		repo, rng, ok := cutRev(id)
		if !ok {
			return "", errs.Usage("compare id %q is missing its range", id)
		}
		return BaseURL + "/" + repo + "/compare/" + rng, nil
	}
	return "", errs.Usage("unknown kind %q; the kinds are %s", kind, strings.Join(Kinds, ", "))
}

// cutRev splits owner/name@rev. It looks for the `@` after the second slash so
// that an owner with an `@` in it, which github.com does not allow but a
// hand-written id might contain, cannot confuse it.
func cutRev(id string) (repo, rev string, ok bool) {
	i := strings.Index(id, "@")
	if i <= 0 || i == len(id)-1 {
		return "", "", false
	}
	return id[:i], id[i+1:], true
}

// SplitPathID splits owner/name@ref/path/to/file into its three parts. The ref
// runs to the next slash, which means a branch with a slash in its name
// (feature/x) parses as ref "feature" and path "x/...". That is a real
// ambiguity in GitHub's own URLs and nothing here can resolve it; a caller who
// knows better passes --ref.
func SplitPathID(id string) (repo, ref, path string, ok bool) {
	repo, rest, ok := cutRev(id)
	if !ok {
		return "", "", "", false
	}
	ref, path, _ = strings.Cut(rest, "/")
	if ref == "" {
		return "", "", "", false
	}
	return repo, ref, path, true
}

// cutRepoRest splits owner/name/rest, keeping any slashes in rest.
func cutRepoRest(id string) (repo, rest string, ok bool) {
	p := strings.SplitN(id, "/", 3)
	if len(p) != 3 || p[0] == "" || p[1] == "" || p[2] == "" {
		return "", "", false
	}
	return p[0] + "/" + p[1], p[2], true
}

// SplitThreadID splits owner/name#123.
func SplitThreadID(id string) (repo string, num string, ok bool) {
	repo, num, ok = strings.Cut(id, "#")
	if !ok || repo == "" || !isNumber(num) {
		return "", "", false
	}
	return repo, num, true
}

// SplitRepo splits owner/name.
func SplitRepo(id string) (owner, name string, ok bool) {
	owner, name, ok = strings.Cut(id, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", "", false
	}
	return owner, name, true
}

// RepoOf returns the repository an id belongs to, for the kinds whose id
// carries one. This is what makes `github tree <a file URL>` work.
func RepoOf(kind, id string) (string, bool) {
	switch kind {
	case KindRepo:
		if _, _, ok := SplitRepo(id); ok {
			return id, true
		}
	case KindIssue, KindPR, KindDiscussion:
		if repo, _, ok := SplitThreadID(id); ok {
			return repo, true
		}
	case KindCommit, KindBranch, KindTag, KindRelease, KindCompare:
		if repo, _, ok := cutRev(id); ok {
			return repo, true
		}
	case KindFile, KindTree:
		if repo, _, _, ok := SplitPathID(id); ok {
			return repo, true
		}
	case KindLabel, KindMilestone, KindWiki, KindPackage:
		if repo, _, ok := cutRepoRest(id); ok {
			return repo, true
		}
	}
	return "", false
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isSHA reports whether a revision looks like an object name rather than a
// branch. Seven is git's own abbreviation floor, and a seven-character branch
// name made only of hex digits (`decade`, `facade` are six) is rare enough that
// this is the right default and --ref is the override.
func isSHA(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
