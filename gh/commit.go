package gh

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/tamnd/github-cli/pkg/gitproto"
	"github.com/tamnd/github-cli/pkg/page"
)

// commit.go reads history: commits, branches, tags, refs, releases, and the
// range between two of them.
//
// Six surfaces answer here and each is incomplete in its own way, so the choice
// is written down per function rather than left to a reader to reconstruct:
//
//	commits list   route JSON, grouped by day, 35 a page, opaque cursor
//	one commit     route JSON, with the whole diff inline as diffEntryData
//	branches       route JSON, rich but capped, and it admits the cap
//	every ref      the git advertisement, complete, one request, no paging
//	releases       Rails pages, 10 a page, with the labels the feed lacks
//	a range        the plain-text patch mailbox, which no styling can break
//
// The one thing nothing here carries is signature verification. It exists on
// exactly one keyless surface, commit search, which is why VerifyCommits reads
// search rather than any of the above.

// --- commit listings ---

// CommitOptions controls a commit walk. Every field maps to a query parameter
// the route already understands, so filtering happens on GitHub's side and not
// after a full download.
type CommitOptions struct {
	// Ref is a branch, a tag, or a SHA. Empty means the default branch.
	Ref string
	// Path limits history to one file or directory.
	Path string
	// Author is a login, not an email.
	Author string
	// Since and Until are dates, YYYY-MM-DD.
	Since string
	Until string
	// Limit stops the walk. Zero means every commit, which on a large
	// repository is thousands of requests, so callers should set it.
	Limit int
}

// Commits streams a repository's history, newest first.
//
// The route groups commits under calendar-day headings and this flattens them,
// keeping the heading on each record as DateGroup. That heading is the only
// place the surface says which timezone it grouped in, so throwing it away
// would make an off-by-one-day question unanswerable.
func (c *Client) Commits(ctx context.Context, repo string, opts CommitOptions, emit func(Commit) error) error {
	if _, _, ok := SplitRepo(repo); !ok {
		return usageBadID("repository", repo, "owner/name")
	}
	ref := opts.Ref
	if ref == "" {
		ref = c.defaultRef(ctx, repo)
	}
	base := repoSubURL(repo, "commits/"+ref)
	if opts.Path != "" {
		base += "/" + strings.Trim(opts.Path, "/")
	}
	kv := []string{}
	for _, p := range [][2]string{
		{"author", opts.Author}, {"since", opts.Since}, {"until", opts.Until},
	} {
		if p[1] != "" {
			kv = append(kv, p[0], p[1])
		}
	}

	fetch := func(ctx context.Context, token string) ([]Commit, string, error) {
		u := base
		args := kv
		if token != "" {
			// The cursor is "<oid> <offset>" and goes back verbatim.
			args = append(append([]string{}, kv...), "after", token)
		}
		if len(args) > 0 {
			u = query(base, args...)
		}
		var env struct {
			Payload struct {
				CommitGroups []struct {
					Title   string            `json:"title"`
					Commits []json.RawMessage `json:"commits"`
				} `json:"commitGroups"`
				Filters struct {
					Pagination struct {
						EndCursor   string `json:"endCursor"`
						HasNextPage bool   `json:"hasNextPage"`
					} `json:"pagination"`
				} `json:"filters"`
				RefInfo struct {
					CurrentOid string `json:"currentOid"`
				} `json:"refInfo"`
			} `json:"payload"`
		}
		res, err := c.GetJSON(ctx, u, SurfaceRouteJSON, &env)
		if err != nil {
			return nil, "", err
		}
		var out []Commit
		for _, g := range env.Payload.CommitGroups {
			for _, raw := range g.Commits {
				cm, err := decodeCommitNode(repo, raw)
				if err != nil {
					return nil, "", err
				}
				cm.DateGroup = g.Title
				cm.addSource(res.FinalURL)
				out = append(out, *cm)
			}
		}
		next := ""
		if env.Payload.Filters.Pagination.HasNextPage {
			next = env.Payload.Filters.Pagination.EndCursor
		}
		return out, next, nil
	}
	return paginate(ctx, opts.Limit, fetch, emit)
}

// commitNode is the commit shape the list route and the single-commit route
// share. They really are the same object; the single-commit route adds parents
// and the two Relay ids on top.
type commitNode struct {
	Oid                      string `json:"oid"`
	URL                      string `json:"url"`
	AuthoredDate             string `json:"authoredDate"`
	CommittedDate            string `json:"committedDate"`
	PushedDate               string `json:"pushedDate"`
	ShortMessage             string `json:"shortMessage"`
	ShortMessageMarkdown     string `json:"shortMessageMarkdown"`
	ShortMessageMarkdownLink string `json:"shortMessageMarkdownLink"`
	BodyMessageHTML          string `json:"bodyMessageHtml"`
	Authors                  []struct {
		Login       string `json:"login"`
		DisplayName string `json:"displayName"`
		AvatarURL   string `json:"avatarUrl"`
		Path        string `json:"path"`
	} `json:"authors"`
	CommitterAttribution bool `json:"committerAttribution"`
	Committer            *struct {
		Login       string `json:"login"`
		DisplayName string `json:"displayName"`
		AvatarURL   string `json:"avatarUrl"`
		Path        string `json:"path"`
		ID          *int   `json:"id"`
		IsGitHub    bool   `json:"isGitHub"`
	} `json:"committer"`
	Pusher *struct {
		Login       string `json:"login"`
		DisplayName string `json:"displayName"`
		AvatarURL   string `json:"avatarUrl"`
	} `json:"pusher"`
	Parents       []string `json:"parents"`
	GlobalRelayID string   `json:"globalRelayId"`
}

func decodeCommitNode(repo string, raw json.RawMessage) (*Commit, error) {
	var v commitNode
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, badPayload(repo, err)
	}
	cm := &Commit{Repo: repo, SHA: v.Oid, NodeID: v.GlobalRelayID}
	cm.setIdentity(KindCommit, repo+"@"+v.Oid)
	cm.URL = commitURL(repo, v.Oid)

	// shortMessage is null as often as not and the markdown field carries the
	// subject with issue links already resolved. Stripping the tags back to
	// text is what makes a commit subject readable in a table, and the markup
	// is kept because it is where the issue references live.
	cm.Subject = firstNonEmpty(v.ShortMessage, stripTags(v.ShortMessageMarkdown))
	cm.BodyHTML = v.BodyMessageHTML
	cm.Body = stripTags(v.BodyMessageHTML)
	cm.AuthoredAt = parseTime(v.AuthoredDate)
	cm.CommittedAt = parseTime(v.CommittedDate)
	cm.PushedAt = parseTime(v.PushedDate)
	cm.Parents = v.Parents

	for _, a := range v.Authors {
		act := actor(a.Login)
		act.Name = a.DisplayName
		act.AvatarURL = a.AvatarURL
		cm.Authors = append(cm.Authors, act)
	}
	if v.Committer != nil && v.Committer.Login != "" {
		act := actor(v.Committer.Login)
		act.Name = v.Committer.DisplayName
		act.AvatarURL = v.Committer.AvatarURL
		act.DatabaseID = v.Committer.ID
		cm.Committer = &act
		// web-flow is GitHub's own committer identity on a squash or a merge
		// made through the web UI. It is not a person and calling it one makes
		// every "who committed this" query wrong on half of a busy repository.
		cm.SignedByGitHub = v.Committer.IsGitHub
	}
	if v.Pusher != nil && v.Pusher.Login != "" {
		act := actor(v.Pusher.Login)
		act.Name = v.Pusher.DisplayName
		act.AvatarURL = v.Pusher.AvatarURL
		cm.Pusher = &act
	}
	cm.IssueRefs = threadRefsIn(v.ShortMessageMarkdown)

	cm.addExtra("commit", decodeExtra(raw, &v,
		// Both are the parent and the commit again under older names, kept by
		// the front end for a diff widget that predates oid and parents.
		"sha1", "sha2",
		// True when the committer differs from the author, which the record
		// already shows by having both.
		"committerAttribution",
	))
	return cm, nil
}

// threadRefsIn pulls the issue and pull request links GitHub already resolved
// inside a commit subject. This is the commit-to-thread edge of the graph
// handed over for free, and it is why the markdown field is worth keeping.
func threadRefsIn(markup string) []ThreadRef {
	if markup == "" {
		return nil
	}
	doc, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		return nil
	}
	var out []ThreadRef
	for _, n := range page.FindAll(doc, page.Sel{Tag: "a", Class: "issue-link"}) {
		ref := ThreadRef{
			URL:           page.Attr(n, "href"),
			IsPullRequest: page.Attr(n, "data-hovercard-type") == "pull_request",
		}
		if id, err := strconv.Atoi(page.Attr(n, "data-id")); err == nil {
			ref.DatabaseID = &id
		}
		out = append(out, ref)
	}
	return out
}

// --- one commit ---

// CommitInfoOptions controls how much of a commit's diff comes back.
type CommitInfoOptions struct {
	// Files fills the per-file change list from the inline diff. The route
	// ships the whole thing whether we decode it or not, so this costs parsing
	// and memory rather than a request.
	Files bool
	// Patch fetches the .patch form as well, one extra request, for a caller
	// that wants to apply the change rather than describe it.
	Patch bool
}

// CommitInfo reads one commit. sha may be a full SHA, an abbreviation, a
// branch, or a tag: the route resolves all four, and the record reports what it
// resolved to.
func (c *Client) CommitInfo(ctx context.Context, repo, sha string, opts CommitInfoOptions) (*Commit, error) {
	if _, _, ok := SplitRepo(repo); !ok {
		return nil, usageBadID("repository", repo, "owner/name")
	}
	if sha == "" {
		return nil, usageBadID("commit", sha, "a sha, a branch, or a tag")
	}
	var env struct {
		Payload struct {
			Commit     json.RawMessage `json:"commit"`
			HeaderInfo struct {
				Additions    int `json:"additions"`
				Deletions    int `json:"deletions"`
				FilesChanged int `json:"filesChanged"`
			} `json:"headerInfo"`
			DiffEntryData   []json.RawMessage `json:"diffEntryData"`
			MoreDiffsToLoad bool              `json:"moreDiffsToLoad"`
		} `json:"payload"`
	}
	url := commitURL(repo, sha)
	res, err := c.GetJSON(ctx, url, SurfaceRouteJSON, &env)
	if err != nil {
		return nil, err
	}
	if len(env.Payload.Commit) == 0 {
		return nil, structureChanged(repo + "@" + sha)
	}
	cm, err := decodeCommitNode(repo, env.Payload.Commit)
	if err != nil {
		return nil, err
	}
	cm.addSource(res.FinalURL)
	cm.Additions = intp(env.Payload.HeaderInfo.Additions)
	cm.Deletions = intp(env.Payload.HeaderInfo.Deletions)

	if opts.Files {
		for _, raw := range env.Payload.DiffEntryData {
			fc, err := decodeDiffEntry(raw)
			if err != nil {
				return nil, err
			}
			cm.Files = append(cm.Files, fc)
		}
		// A commit that touches hundreds of files ships the first few and
		// defers the rest. Saying so beats handing back a short list that
		// looks complete.
		if env.Payload.MoreDiffsToLoad && len(cm.Files) < env.Payload.HeaderInfo.FilesChanged {
			recordVia(&cm.Base, "files", "partial")
		}
	}
	if opts.Patch {
		text, err := c.Patch(ctx, url)
		if err != nil {
			return nil, err
		}
		cm.Body = firstNonEmpty(cm.Body, patchBody(text))
		cm.addSource(url + ".patch")
	}
	return cm, nil
}

// diffEntry is one file in the inline diff. The line arrays are skipped: this
// package models the change, not the rendering of it, and `github patch` hands
// back the real thing for a caller that wants lines.
type diffEntry struct {
	Path         string `json:"path"`
	Status       string `json:"status"`
	LinesAdded   *int   `json:"linesAdded"`
	LinesDeleted *int   `json:"linesDeleted"`
	IsBinary     bool   `json:"isBinary"`
	IsTooBig     bool   `json:"isTooBig"`
	IsSubmodule  bool   `json:"isSubmodule"`
	OldTreeEntry *struct {
		Path        string `json:"path"`
		IsGenerated bool   `json:"isGenerated"`
	} `json:"oldTreeEntry"`
	NewTreeEntry *struct {
		Path        string `json:"path"`
		IsGenerated bool   `json:"isGenerated"`
	} `json:"newTreeEntry"`
}

func decodeDiffEntry(raw json.RawMessage) (FileChange, error) {
	var v diffEntry
	if err := json.Unmarshal(raw, &v); err != nil {
		return FileChange{}, badPayload("diff entry", err)
	}
	fc := FileChange{
		Path:      v.Path,
		Status:    strings.ToLower(v.Status),
		Additions: v.LinesAdded,
		Deletions: v.LinesDeleted,
		IsBinary:  v.IsBinary,
	}
	// A rename ships both entries with different paths. Nothing else in the
	// payload says "renamed", so the two paths are the evidence.
	if v.OldTreeEntry != nil && v.NewTreeEntry != nil && v.OldTreeEntry.Path != v.NewTreeEntry.Path {
		fc.PrevPath = v.OldTreeEntry.Path
	}
	return fc, nil
}

// VerifyCommits fills in signature state, which no commit route carries.
//
// Commit search is the only keyless surface that reports it, so this asks
// search for the exact SHAs and merges what comes back. It is a separate
// function rather than a flag because it is a different request against a
// different index, and a caller should see that in the code they wrote.
func (c *Client) VerifyCommits(ctx context.Context, repo string, commits []*Commit) error {
	for _, cm := range commits {
		if cm.SHA == "" {
			continue
		}
		q := "repo:" + repo + " hash:" + cm.SHA
		err := c.SearchCommitsBy(ctx, q, 1, func(found Commit) error {
			if found.SHA != cm.SHA {
				return nil
			}
			cm.Verification = found.Verification
			cm.VerificationReason = found.VerificationReason
			cm.HasSignature = found.HasSignature
			cm.KeyID = found.KeyID
			cm.KeyExpired = found.KeyExpired
			recordVia(&cm.Base, "verification", "search")
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// --- refs ---

// RefOptions controls a ref listing.
type RefOptions struct {
	// Complete reads the git advertisement instead of the branches page: every
	// ref in one request, with SHAs, and no cap. It costs the author and date
	// the page carries, because the protocol does not have them.
	Complete bool
	// Pulls includes refs/pull/*, which github.com advertises for every pull
	// request ever opened. On a busy repository that is most of the response.
	Pulls bool
	Limit int
}

// Branches lists branches. Without Complete this is the branches route, which
// carries the last author and the last authored date and is capped by GitHub;
// with it, this is the git advertisement, which is complete and carries SHAs.
//
// Neither is strictly better and the record says which one answered, so a
// consumer that finds Author empty knows why.
func (c *Client) Branches(ctx context.Context, repo string, opts RefOptions, emit func(GitRef) error) error {
	if _, _, ok := SplitRepo(repo); !ok {
		return usageBadID("repository", repo, "owner/name")
	}
	if opts.Complete {
		return c.advertisedRefs(ctx, repo, opts, KindBranch, emit)
	}
	base := repoSubURL(repo, "branches")
	fetch := func(ctx context.Context, token string) ([]GitRef, string, error) {
		n := pageToken(token)
		u := base
		if n > 1 {
			u = query(base, "page", strconv.Itoa(n))
		}
		var env struct {
			Payload struct {
				Branches struct {
					Default *branchNode  `json:"default"`
					Active  []branchNode `json:"active"`
				} `json:"branches"`
				HasMore struct {
					Active bool `json:"active"`
				} `json:"hasMore"`
			} `json:"payload"`
		}
		res, err := c.GetJSON(ctx, u, SurfaceRouteJSON, &env)
		if err != nil {
			return nil, "", err
		}
		var out []GitRef
		// The default branch is served separately from the active list and is
		// not repeated inside it, so it is prepended once, on the first page.
		if n == 1 && env.Payload.Branches.Default != nil {
			out = append(out, env.Payload.Branches.Default.toRef(repo, res.FinalURL))
		}
		for _, b := range env.Payload.Branches.Active {
			out = append(out, b.toRef(repo, res.FinalURL))
		}
		next := ""
		if env.Payload.HasMore.Active {
			next = strconv.Itoa(n + 1)
		}
		return out, next, nil
	}
	return paginate(ctx, opts.Limit, fetch, emit)
}

type branchNode struct {
	Name         string `json:"name"`
	IsDefault    bool   `json:"isDefault"`
	Path         string `json:"path"`
	Protected    bool   `json:"protectedByBranchProtections"`
	AuthoredDate string `json:"authoredDate"`
	Author       *struct {
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatarUrl"`
	} `json:"author"`
}

func (b branchNode) toRef(repo, source string) GitRef {
	r := GitRef{
		Repo:       repo,
		Name:       b.Name,
		Type:       "branch",
		IsDefault:  b.IsDefault,
		Protected:  b.Protected,
		AuthoredAt: parseTime(b.AuthoredDate),
	}
	r.setIdentity(KindBranch, repo+"@"+b.Name)
	if b.Author != nil && b.Author.Login != "" {
		a := actor(b.Author.Login)
		a.Name = b.Author.Name
		a.AvatarURL = b.Author.AvatarURL
		r.Author = &a
	}
	r.addSource(source)
	return r
}

// Tags lists tags. The git advertisement is the default here rather than an
// opt-in, because the tags feed gives ten and the tags page gives ten at a
// time, and a repository with four hundred tags is the normal case.
func (c *Client) Tags(ctx context.Context, repo string, opts RefOptions, emit func(GitRef) error) error {
	if _, _, ok := SplitRepo(repo); !ok {
		return usageBadID("repository", repo, "owner/name")
	}
	return c.advertisedRefs(ctx, repo, opts, KindTag, emit)
}

// Refs lists every ref of every kind in one request.
func (c *Client) Refs(ctx context.Context, repo string, opts RefOptions, emit func(GitRef) error) error {
	if _, _, ok := SplitRepo(repo); !ok {
		return usageBadID("repository", repo, "owner/name")
	}
	return c.advertisedRefs(ctx, repo, opts, "", emit)
}

// advertisedRefs reads the git smart-protocol advertisement and emits the kinds
// asked for. kind is KindBranch, KindTag, or empty for everything.
//
// The whole advertisement arrives in one response, so there is no paging and no
// cursor here, and Limit is applied by walking rather than by asking for less.
func (c *Client) advertisedRefs(ctx context.Context, repo string, opts RefOptions, kind string, emit func(GitRef) error) error {
	url := gitRefsURL(repo)
	res, err := c.Get(ctx, url, SurfaceGit)
	if err != nil {
		return err
	}
	ad, err := gitproto.Parse(res.Body)
	if err != nil {
		return badPayload(repo, err)
	}

	sent := 0
	send := func(r GitRef) error {
		r.Repo = repo
		r.addSource(url)
		if err := emit(r); err != nil {
			return err
		}
		sent++
		return nil
	}
	done := func() bool { return opts.Limit > 0 && sent >= opts.Limit }

	if kind == "" || kind == KindBranch {
		for _, b := range ad.Branches() {
			if done() {
				return nil
			}
			r := GitRef{Name: b.Name, Type: "branch", SHA: b.SHA, IsDefault: b.Name == ad.DefaultBranch}
			r.setIdentity(KindBranch, repo+"@"+b.Name)
			if err := send(r); err != nil {
				return err
			}
		}
	}
	if kind == "" || kind == KindTag {
		for _, t := range ad.Tags() {
			if done() {
				return nil
			}
			r := GitRef{Name: t.Name, Type: "tag", SHA: t.SHA, PeeledSHA: t.Peeled}
			r.setIdentity(KindTag, repo+"@"+t.Name)
			if err := send(r); err != nil {
				return err
			}
		}
	}
	if opts.Pulls && kind == "" {
		for _, p := range ad.PullHeads() {
			if done() {
				return nil
			}
			r := GitRef{Name: "pull/" + p.Name, Type: "pull", SHA: p.SHA}
			r.setIdentity(KindBranch, repo+"@refs/pull/"+p.Name)
			if err := send(r); err != nil {
				return err
			}
		}
	}
	return nil
}

// DefaultBranch asks the git advertisement which branch HEAD points at. It is
// the authoritative answer, where the route JSON's refInfo is whichever ref the
// URL happened to resolve to.
func (c *Client) DefaultBranch(ctx context.Context, repo string) (string, error) {
	res, err := c.Get(ctx, gitRefsURL(repo), SurfaceGit)
	if err != nil {
		return "", err
	}
	ad, err := gitproto.Parse(res.Body)
	if err != nil {
		return "", badPayload(repo, err)
	}
	if ad.DefaultBranch == "" {
		return "", structureChanged(repo + " HEAD")
	}
	return ad.DefaultBranch, nil
}

// --- releases ---

// ReleaseOptions controls a release listing.
type ReleaseOptions struct {
	// Assets fetches the lazy asset fragment for each release, one extra
	// request each. Without it a release record has no downloads.
	Assets bool
	// Body keeps the rendered release notes, which are most of the bytes on a
	// project that writes a changelog.
	Body  bool
	Limit int
}

// Releases streams a repository's releases, newest first.
//
// The list comes from the HTML pages rather than releases.atom, which is the
// opposite of what you would expect from a feed-shaped problem. The feed gives
// ten entries and does not page, so it cannot answer "every release"; the pages
// give ten at a time with a rel="next" and carry the Latest and Pre-release
// labels the feed has no room for.
func (c *Client) Releases(ctx context.Context, repo string, opts ReleaseOptions, emit func(Release) error) error {
	if _, _, ok := SplitRepo(repo); !ok {
		return usageBadID("repository", repo, "owner/name")
	}
	base := repoSubURL(repo, "releases")
	fetch := func(ctx context.Context, token string) ([]Release, string, error) {
		n := pageToken(token)
		u := base
		if n > 1 {
			u = query(base, "page", strconv.Itoa(n))
		}
		res, err := c.GetHTML(ctx, u)
		if err != nil {
			return nil, "", err
		}
		p := page.Extract(res.FinalURL, res.Body)
		doc := p.Doc()
		if doc == nil {
			return nil, "", structureChanged(repo + " releases")
		}
		var out []Release
		for _, sec := range page.FindAll(doc, releaseSection) {
			rel := decodeReleaseSection(repo, sec, opts.Body)
			if rel.Tag == "" {
				continue
			}
			rel.addSource(res.FinalURL)
			out = append(out, rel)
		}
		next := ""
		if len(out) > 0 && page.Find(doc, nextPageLink) != nil {
			next = strconv.Itoa(n + 1)
		}
		return out, next, nil
	}
	wrapped := emit
	if opts.Assets {
		wrapped = func(rel Release) error {
			if assets, err := c.releaseAssets(ctx, repo, rel.Tag); err == nil {
				rel.Assets = assets
				rel.addSource(assetsFragmentURL(repo, rel.Tag))
			}
			return emit(rel)
		}
	}
	return paginate(ctx, opts.Limit, fetch, wrapped)
}

// Release reads one release by tag. tag may also be "latest", which github.com
// redirects to whatever that is today.
//
// The per-tag page is not the list page with nine releases removed. It is a
// different template with a different shape, so it gets its own decoder rather
// than a selector that limps along on both. What it gains over a list entry is
// the commit the tag points at; what it lacks is nothing.
func (c *Client) Release(ctx context.Context, repo, tag string, opts ReleaseOptions) (*Release, error) {
	if _, _, ok := SplitRepo(repo); !ok {
		return nil, usageBadID("repository", repo, "owner/name")
	}
	if tag == "" {
		return nil, usageBadID("release", tag, "a tag name")
	}
	sub := "releases/tag/" + tag
	if tag == "latest" {
		sub = "releases/latest"
	}
	url := repoSubURL(repo, sub)
	res, err := c.GetHTML(ctx, url)
	if err != nil {
		return nil, err
	}
	p := page.Extract(res.FinalURL, res.Body)
	doc := p.Doc()
	if doc == nil {
		return nil, structureChanged(repo + "@" + tag)
	}
	rel, ok := decodeReleasePage(repo, doc)
	if !ok {
		return nil, structureChanged(repo + "@" + tag)
	}
	if rel.Tag == "" {
		rel.Tag = tag
		rel.setIdentity(KindRelease, repo+"@"+tag)
	}
	rel.addSource(res.FinalURL)
	tag = rel.Tag
	if !opts.Body {
		rel.Body, rel.BodyHTML = "", ""
	}
	if opts.Assets {
		assets, err := c.releaseAssets(ctx, repo, rel.Tag)
		if err != nil {
			return nil, err
		}
		rel.Assets = assets
		rel.addSource(assetsFragmentURL(repo, rel.Tag))
	}
	return &rel, nil
}

// releaseSection anchors on the id GitHub gives every release box,
// release-{tag}. It survives the styling churn that renames every class around
// it, because the anchor is what the in-page release menu links to.
var releaseSection = page.Sel{Tag: "section", Attr: "id", AttrPrefix: "release-"}

var nextPageLink = page.Sel{Tag: "a", Attr: "rel", AttrValue: "next"}

func decodeReleaseSection(repo string, sec *html.Node, keepBody bool) Release {
	rel := Release{Repo: repo}
	for _, a := range page.FindAll(sec, page.Sel{Tag: "a", Attr: "href", AttrContains: "/releases/tag/"}) {
		href := page.Attr(a, "href")
		if i := strings.LastIndex(href, "/releases/tag/"); i >= 0 {
			rel.Tag = strings.TrimPrefix(href[i:], "/releases/tag/")
			rel.Title = firstNonEmpty(rel.Title, page.Text(a))
			break
		}
	}
	if rel.Tag == "" {
		return rel
	}
	rel.setIdentity(KindRelease, repo+"@"+rel.Tag)

	// The labels are the whole reason this reads the page and not the feed.
	for _, l := range page.FindAll(sec, page.Sel{Class: "Label"}) {
		switch page.Text(l) {
		case "Latest":
			rel.IsLatest = true
		case "Pre-release":
			rel.IsPrerelease = true
		case "Draft":
			rel.IsDraft = true
		}
	}
	if a := page.Find(sec, page.Sel{Tag: "a", Attr: "data-hovercard-type", AttrValue: "user"}); a != nil {
		rel.Author = releaseAuthor(page.Attr(a, "href"))
	}
	if t := page.Find(sec, page.RelTimeEl); t != nil {
		rel.PublishedAt = parseTime(page.Attr(t, "datetime"))
	}
	if body := page.Find(sec, page.Sel{Class: "markdown-body"}); body != nil && keepBody {
		rel.BodyHTML = page.OuterHTML(body)
		rel.Body = page.BlockText(body)
	}
	for _, a := range page.FindAll(sec, page.Sel{Tag: "a", Attr: "href", AttrContains: "/archive/refs/tags/"}) {
		href := page.Attr(a, "href")
		switch {
		case strings.HasSuffix(href, ".tar.gz"):
			rel.TarballURL = absoluteURL(href)
		case strings.HasSuffix(href, ".zip"):
			rel.ZipballURL = absoluteURL(href)
		}
	}
	return rel
}

// primaryContent is the div GitHub marks with data-hpc, which is its own
// "hero primary content" flag. Anchoring on it keeps the header, the sidebar,
// and half a dozen dialogs out of every selector below, which matters on a
// template where a search overlay also has an h1 and the repository header also
// has something with class Label.
var primaryContent = page.Sel{Tag: "div", Attr: "data-hpc"}

// releaseAuthor turns a publisher link into an actor, whichever of the two
// release templates wrote it.
//
// A release cut by a workflow links to /apps/github-actions, which is not a
// login: passing it through actor gives a profile URL that 404s. The app name
// is the useful half, and the type says why it has no profile.
func releaseAuthor(href string) *Actor {
	p := hrefPath(href)
	if p == "" {
		return nil
	}
	if name, ok := strings.CutPrefix(p, "apps/"); ok {
		return &Actor{Login: name, Type: "Bot", URL: BaseURL + "/apps/" + name}
	}
	if strings.Contains(p, "/") {
		return nil
	}
	a := actor(p)
	return &a
}

// decodeReleasePage reads the single-release template.
//
// The false return is a real answer: a tag that has no release, which happens
// on every repository that tags more often than it publishes, renders a page
// that looks fine and has no release box on it.
func decodeReleasePage(repo string, doc *html.Node) (Release, bool) {
	root := page.Find(doc, primaryContent)
	if root == nil {
		root = doc
	}
	box := page.Find(root, page.Sel{Tag: "div", Class: "Box"})
	if box == nil {
		return Release{}, false
	}
	rel := Release{Repo: repo}
	// The breadcrumb above the box is the only place the page states the tag
	// as a tag rather than as a heading somebody typed.
	for _, a := range page.FindAll(root, page.Sel{Tag: "a", Attr: "href", AttrContains: "/releases/tag/"}) {
		href := page.Attr(a, "href")
		if i := strings.LastIndex(href, "/releases/tag/"); i >= 0 {
			rel.Tag = strings.TrimPrefix(href[i:], "/releases/tag/")
			break
		}
	}
	if rel.Tag == "" {
		if a := page.Find(root, page.Sel{Tag: "a", Attr: "href", AttrContains: "/tree/"}); a != nil {
			href := page.Attr(a, "href")
			rel.Tag = href[strings.LastIndex(href, "/tree/")+len("/tree/"):]
		}
	}
	if rel.Tag == "" {
		return Release{}, false
	}
	rel.setIdentity(KindRelease, repo+"@"+rel.Tag)

	// The first h1 inside the box is the release name. The later ones belong
	// to the tag-picker dialog, which is nested in the same box.
	if h := page.Find(box, page.Sel{Tag: "h1"}); h != nil {
		rel.Title = strings.TrimSpace(page.Text(h))
	}
	for _, l := range page.FindAll(root, page.Sel{Class: "Label"}) {
		switch strings.TrimSpace(page.Text(l)) {
		case "Latest":
			rel.IsLatest = true
		case "Pre-release":
			rel.IsPrerelease = true
		case "Draft":
			rel.IsDraft = true
		}
	}
	// The publisher is the bold link in the byline row, which is a user on a
	// hand-cut release and /apps/something when a workflow cut it. Reading the
	// first user hovercard instead would pick a contributor avatar from the
	// footer, which is a different person and a wrong answer.
	if a := page.Find(root, page.Sel{Tag: "a", Class: "text-bold"}); a != nil {
		rel.Author = releaseAuthor(page.Attr(a, "href"))
	}
	if t := page.Find(root, page.RelTimeEl); t != nil {
		rel.PublishedAt = parseTime(page.Attr(t, "datetime"))
	}
	if a := page.Find(root, page.Sel{Tag: "a", Attr: "href", AttrContains: "/commit/"}); a != nil {
		href := page.Attr(a, "href")
		if sha := href[strings.LastIndex(href, "/commit/")+len("/commit/"):]; len(sha) == 40 && isHex(sha) {
			rel.CommitSHA = sha
		}
	}
	if body := page.Find(root, page.Sel{Class: "markdown-body"}); body != nil {
		rel.BodyHTML = page.OuterHTML(body)
		rel.Body = page.BlockText(body)
	}
	for _, a := range page.FindAll(root, page.Sel{Tag: "a", Attr: "href", AttrContains: "/archive/refs/tags/"}) {
		href := page.Attr(a, "href")
		switch {
		case strings.HasSuffix(href, ".tar.gz"):
			rel.TarballURL = absoluteURL(href)
		case strings.HasSuffix(href, ".zip"):
			rel.ZipballURL = absoluteURL(href)
		}
	}
	return rel, true
}

func assetsFragmentURL(repo, tag string) string {
	return repoSubURL(repo, "releases/expanded_assets/"+tag)
}

// releaseAssets reads the deferred asset fragment.
//
// The release page ships an include-fragment where the asset table should be,
// so a page fetch alone never sees the downloads no matter how large it is.
// The fragment itself is small and is the only place the sha256 digests exist.
func (c *Client) releaseAssets(ctx context.Context, repo, tag string) ([]Asset, error) {
	res, err := c.Get(ctx, assetsFragmentURL(repo, tag), SurfaceXHR)
	if err != nil {
		return nil, err
	}
	doc, err := html.Parse(strings.NewReader(string(res.Body)))
	if err != nil {
		return nil, badPayload(repo+"@"+tag, err)
	}
	var out []Asset
	for _, row := range page.FindAll(doc, page.Sel{Tag: "li", Class: "Box-row"}) {
		a := Asset{}
		if link := page.Find(row, page.Sel{Tag: "a", Attr: "href", AttrContains: "/releases/download/"}); link != nil {
			href := page.Attr(link, "href")
			a.URL = absoluteURL(href)
			a.Name = href[strings.LastIndex(href, "/")+1:]
		}
		if link := page.Find(row, page.Sel{Tag: "a", Attr: "href", AttrContains: "/archive/refs/tags/"}); link != nil && a.URL == "" {
			href := page.Attr(link, "href")
			a.URL = absoluteURL(href)
			a.Name = href[strings.LastIndex(href, "/")+1:]
		}
		if a.Name == "" {
			continue
		}
		for _, span := range page.FindAll(row, page.Sel{Tag: "span", Class: "Truncate-text"}) {
			switch text := page.Text(span); {
			case strings.HasPrefix(text, "sha256:"):
				a.Digest = text
			case a.Label == "" && text != "":
				// The first truncated span in the row is the label, and the
				// second half of the same pair is the empty overflow tail.
				a.Label = text
			}
		}
		if t := page.Find(row, page.RelTimeEl); t != nil {
			a.UpdatedAt = parseTime(page.Attr(t, "datetime"))
		}
		// The size is the one bare span in the row with no class of its own,
		// so it is found by shape: the last span whose text reads like a size.
		for _, span := range page.FindAll(row, page.Sel{Tag: "span"}) {
			if s := page.Text(span); looksLikeSize(s) {
				a.SizeDisplay = s
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// looksLikeSize matches "13.1 MB" and "742 Bytes" and not much else. It is
// deliberately narrow: a false positive here would put a random span's text in
// the size column.
func looksLikeSize(s string) bool {
	n, unit, ok := strings.Cut(s, " ")
	if !ok {
		return false
	}
	switch unit {
	case "Bytes", "KB", "MB", "GB", "TB":
	default:
		return false
	}
	_, err := strconv.ParseFloat(n, 64)
	return err == nil
}

func absoluteURL(href string) string {
	if strings.HasPrefix(href, "http") {
		return href
	}
	return BaseURL + href
}

// --- ranges ---

// CompareOptions controls a range read.
type CompareOptions struct {
	// Files parses the per-file changes out of the patch. Free, since the
	// patch is already downloaded.
	Files bool
	// Patch keeps the raw stream on the record.
	Patch bool
}

// CompareRefs reads the range between two refs.
//
// It reads the plain-text patch mailbox, not the compare page. The page has no
// JSON payload of any kind, it is more than twice the size, and every field on
// it is a class name away from breaking. git-format-patch output is a format
// GitHub does not own and cannot restyle, and it carries every commit with its
// author, date, subject, and diff.
//
// The trade is that the mailbox has no logins, only names and emails, so the
// authors on these commits have Name set and Login empty. That is honest: the
// patch really does not say who the GitHub user was.
func (c *Client) CompareRefs(ctx context.Context, repo, base, head string, opts CompareOptions) (*Compare, error) {
	if _, _, ok := SplitRepo(repo); !ok {
		return nil, usageBadID("repository", repo, "owner/name")
	}
	if base == "" || head == "" {
		return nil, usageBadID("range", base+"..."+head, "base...head")
	}
	rng := base + "..." + head
	url := repoSubURL(repo, "compare/"+rng)
	text, err := c.Patch(ctx, url)
	if err != nil {
		return nil, err
	}

	cmp := &Compare{Repo: repo, BaseRef: base, HeadRef: head, PatchURL: url + ".patch", DiffURL: url + ".diff"}
	cmp.setIdentity(KindCompare, repo+"@"+rng)
	cmp.addSource(cmp.PatchURL)
	if opts.Patch {
		cmp.Patch = text
	}
	for _, part := range splitMailbox(text) {
		cm := commitFromPatch(repo, part)
		if cm == nil {
			continue
		}
		cm.addSource(cmp.PatchURL)
		if opts.Files {
			cm.Files = filesInPatch(part)
		}
		cmp.Commits = append(cmp.Commits, *cm)
	}
	cmp.CommitCount = len(cmp.Commits)
	if opts.Files {
		cmp.Files = filesInPatch(text)
		cmp.FileCount = len(cmp.Files)
		for _, f := range cmp.Files {
			if f.Additions != nil {
				cmp.Additions += *f.Additions
			}
			if f.Deletions != nil {
				cmp.Deletions += *f.Deletions
			}
		}
	}
	return cmp, nil
}

// Patch returns the git patch for a commit, a pull request, or a range. url is
// any github.com URL naming one of the three; the .patch suffix is appended.
//
// This is the cheapest complete view of a change on the whole site: no
// negotiation, no payload, no page, and no token.
func (c *Client) Patch(ctx context.Context, url string) (string, error) {
	res, err := c.Get(ctx, strings.TrimSuffix(url, "/")+".patch", SurfaceRaw)
	if err != nil {
		return "", err
	}
	return string(res.Body), nil
}

// Diff returns the unified diff, which is the patch without the commit
// metadata. On a wide range it is a third of the size.
func (c *Client) Diff(ctx context.Context, url string) (string, error) {
	res, err := c.Get(ctx, strings.TrimSuffix(url, "/")+".diff", SurfaceRaw)
	if err != nil {
		return "", err
	}
	return string(res.Body), nil
}

// --- patch parsing ---

// splitMailbox cuts a git-format-patch stream into one string per commit.
//
// A commit starts at a line reading "From <40 hex> Mon Sep 17 00:00:00 2001",
// which is git's fixed magic date and not a real one. Matching the whole shape
// rather than just "From " is what keeps a diff line reading "From " in a
// changed file from splitting the mailbox in half.
func splitMailbox(text string) []string {
	var out []string
	var cur []string
	for _, line := range strings.Split(text, "\n") {
		if isMailboxHeader(line) {
			if len(cur) > 0 {
				out = append(out, strings.Join(cur, "\n"))
			}
			cur = cur[:0]
		}
		if len(cur) > 0 || isMailboxHeader(line) {
			cur = append(cur, line)
		}
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, "\n"))
	}
	return out
}

func isMailboxHeader(line string) bool {
	rest, ok := strings.CutPrefix(line, "From ")
	if !ok {
		return false
	}
	sha, tail, ok := strings.Cut(rest, " ")
	return ok && len(sha) == 40 && isHex(sha) && tail == "Mon Sep 17 00:00:00 2001"
}

func isHex(s string) bool {
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// commitFromPatch reads the mail headers off one mailbox entry.
func commitFromPatch(repo, part string) *Commit {
	head, body, _ := strings.Cut(part, "\n\n")
	lines := strings.Split(head, "\n")
	if len(lines) == 0 {
		return nil
	}
	sha := ""
	if rest, ok := strings.CutPrefix(lines[0], "From "); ok {
		sha, _, _ = strings.Cut(rest, " ")
	}
	if len(sha) != 40 {
		return nil
	}
	cm := &Commit{Repo: repo, SHA: sha}
	cm.setIdentity(KindCommit, repo+"@"+sha)
	cm.URL = commitURL(repo, sha)

	for i, line := range lines[1:] {
		switch {
		case strings.HasPrefix(line, "From: "):
			name, email := splitAddress(strings.TrimPrefix(line, "From: "))
			a := Actor{Name: name}
			if login, ok := loginFromNoreply(email); ok {
				a = actor(login)
				a.Name = name
			}
			cm.Authors = append(cm.Authors, a)
		case strings.HasPrefix(line, "Date: "):
			cm.AuthoredAt = parseMailDate(strings.TrimPrefix(line, "Date: "))
		case strings.HasPrefix(line, "Subject: "):
			// git wraps a long subject onto continuation lines that begin with
			// whitespace, and the [PATCH n/m] prefix is git's, not the author's.
			subject := strings.TrimPrefix(line, "Subject: ")
			for _, cont := range lines[i+2:] {
				if !strings.HasPrefix(cont, " ") && !strings.HasPrefix(cont, "\t") {
					break
				}
				subject += " " + strings.TrimSpace(cont)
			}
			cm.Subject = stripPatchPrefix(subject)
		}
	}
	// The message body is everything before the diffstat separator.
	if msg, _, ok := strings.Cut(body, "\n---\n"); ok {
		cm.Body = strings.TrimSpace(msg)
	}
	return cm
}

// splitAddress cuts `Name <mail@example.com>` into its two halves.
func splitAddress(s string) (name, email string) {
	s = strings.TrimSpace(s)
	i := strings.LastIndex(s, "<")
	if i < 0 || !strings.HasSuffix(s, ">") {
		return s, ""
	}
	return strings.TrimSpace(s[:i]), s[i+1 : len(s)-1]
}

// loginFromNoreply recovers a GitHub login from the noreply address GitHub
// hands out, 12345+octocat@users.noreply.github.com or the older
// octocat@users.noreply.github.com. It is the only place a patch carries a
// login, and it is worth taking when it is there.
func loginFromNoreply(email string) (string, bool) {
	local, host, ok := strings.Cut(email, "@")
	if !ok || host != "users.noreply.github.com" {
		return "", false
	}
	if _, login, ok := strings.Cut(local, "+"); ok {
		return login, login != ""
	}
	return local, local != ""
}

// parseMailDate reads the RFC 2822 date git writes, "Thu, 7 Nov 2024 14:39:11
// -0700". Every other record in this package carries RFC 3339, and normalising
// here is what keeps one date format in the output instead of six in the input.
//
// The day is not zero-padded in git's output and RFC1123Z requires that it is,
// so both layouts are tried.
func parseMailDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC1123Z, "Mon, 2 Jan 2006 15:04:05 -0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			u := t.UTC()
			return &u
		}
	}
	return nil
}

func stripPatchPrefix(subject string) string {
	s := strings.TrimSpace(subject)
	if !strings.HasPrefix(s, "[PATCH") {
		return s
	}
	if i := strings.Index(s, "]"); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}

// patchBody returns the message body of a single-commit patch.
func patchBody(text string) string {
	parts := splitMailbox(text)
	if len(parts) == 0 {
		return ""
	}
	cm := commitFromPatch("", parts[0])
	if cm == nil {
		return ""
	}
	return cm.Body
}

// filesInPatch reads the per-file changes out of a unified diff by counting the
// + and - lines. git's own diffstat is in the mailbox header, but only for a
// single commit, and a range has no combined one, so counting is the only way
// to get the same number for both.
//
// Three lines in a patch start with a + or a - and are not content: the two
// file headers, and the "-- " that ends a mailbox entry before the git version
// string. Miscounting those is how a two-line change becomes a three-line one.
func filesInPatch(text string) []FileChange {
	var out []FileChange
	var cur *FileChange
	add, del := 0, 0
	inBody := false
	flush := func() {
		if cur == nil {
			return
		}
		cur.Additions = intp(add)
		cur.Deletions = intp(del)
		out = append(out, *cur)
		cur, add, del, inBody = nil, 0, 0, false
	}
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			cur = &FileChange{Path: pathFromDiffHeader(line), Status: "modified"}
		case cur == nil:
		case line == "--" || line == "-- ":
			// The mailbox signature separator. Everything after it belongs to
			// the mail, not to the diff.
			flush()
		case strings.HasPrefix(line, "new file mode"):
			cur.Status = "added"
		case strings.HasPrefix(line, "deleted file mode"):
			cur.Status = "removed"
		case strings.HasPrefix(line, "rename from "):
			cur.PrevPath = strings.TrimPrefix(line, "rename from ")
			cur.Status = "renamed"
		case strings.HasPrefix(line, "rename to "):
			cur.Path = strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "Binary files "):
			cur.IsBinary = true
		case strings.HasPrefix(line, "--- "):
			// The old-side header. It is also the authority on the path when
			// the new side is /dev/null, which is what a deletion looks like.
			if p, ok := strings.CutPrefix(line, "--- a/"); ok && cur.PrevPath == "" {
				cur.PrevPath = p
			}
		case strings.HasPrefix(line, "+++ "):
			// The new-side header, and the only unambiguous statement of the
			// path in the whole patch: the `diff --git a/x b/x` line cannot be
			// split reliably when a path contains a space.
			if p, ok := strings.CutPrefix(line, "+++ b/"); ok {
				cur.Path = p
			}
			inBody = true
		case !inBody:
			// Still in the file header block, where a line beginning with a
			// dash is metadata rather than a removed line.
		case strings.HasPrefix(line, "+"):
			add++
		case strings.HasPrefix(line, "-"):
			del++
		}
	}
	flush()
	// A rename with no content change has no +++ header at all, so its path
	// still came from the diff --git line. That is fine: rename to said it.
	for i := range out {
		if out[i].Status == "modified" && out[i].PrevPath != "" && out[i].PrevPath != out[i].Path {
			out[i].Status = "renamed"
		}
		if out[i].Status != "renamed" {
			out[i].PrevPath = ""
		}
	}
	return out
}

// pathFromDiffHeader reads the b-side path off `diff --git a/x b/x`.
//
// This is a guess and it has to be. git writes both paths on one line with no
// quoting for a plain space, so `a/my file b/my file` cannot be split without
// knowing the answer already. The guess is the first " b/", which is right
// whenever no path contains that sequence, and filesInPatch overwrites it from
// the +++ header, which is unambiguous, as soon as one shows up.
func pathFromDiffHeader(line string) string {
	rest := strings.TrimPrefix(line, "diff --git ")
	i := strings.Index(rest, " b/")
	if i < 0 {
		return strings.TrimPrefix(rest, "a/")
	}
	return strings.TrimPrefix(rest[i+1:], "b/")
}
