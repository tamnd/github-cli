package gh

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/tamnd/github-cli/pkg/page"
)

// deps.go reads the two dependency graph pages. They are the most valuable
// keyless surface on the site and the least reliable one: the graph is opt-in
// per repository, the rows are prose, and a package GitHub cannot resolve to a
// repository is a name and nothing else.
//
// Both pages are read rather than one, because they are not two views of the
// same list. Dependencies come from the manifests in this repository and
// dependents come from every other repository's manifests, so neither can be
// derived from the other.
//
// The two pagers disagree, which is why there are two of them here. The
// dependency list is a Rails pager with ?page=N and a rel="next" anchor, and the
// dependents list is a cursor in a button.

// Dependencies lists what a repository declares in its manifests.
//
// A repository with the dependency graph switched off answers with a page and
// no rows, which is an empty list rather than an error: the difference between
// "nothing to report" and "not enabled" is not on the page, so claiming to know
// which one it is would be making it up.
func (c *Client) Dependencies(ctx context.Context, repo string, limit int, emit func(Dependency) error) error {
	if _, _, ok := SplitRepo(repo); !ok {
		return usageBadID("repository", repo, "owner/name")
	}
	base := repoSubURL(repo, "network/dependencies")
	fetch := func(ctx context.Context, token string) ([]Dependency, string, error) {
		u := base
		if n := pageToken(token); n > 1 {
			u = query(u, "page", strconv.Itoa(n))
		}
		doc, final, err := c.rowPage(ctx, u, page.DependencyRow)
		if err != nil {
			return nil, "", err
		}
		if doc == nil {
			return nil, "", structureChanged(repo + " dependencies")
		}
		var out []Dependency
		for _, row := range page.FindAll(doc, page.BoxRow) {
			d, ok := dependencyRow(row, repo, final)
			if ok {
				out = append(out, d)
			}
		}
		return out, railsNext(doc, token), nil
	}
	return paginate(ctx, limit, fetch, emit)
}

// rowPage reads one page of a dependency graph listing.
//
// It exists because GitHub answers a cursor page with a 200, the right title,
// the right chrome, and no rows at all, often enough to matter. An empty page is
// indistinguishable from the end of the list, so the walk stops early and
// reports a third of the dependents as the whole set. Asking a second time gets
// the rows, so the read is repeated once before an empty page is believed.
//
// Dropping the cached copy first is the part that matters. Without it the retry
// reads the same empty bytes back and the wrong answer sticks for the life of
// the entry, which is how this was found: --no-cache returned two hundred rows
// and the cached run returned sixty, over and over.
//
// The cost is one extra request for a repository whose dependency graph really
// is empty, which is the right trade: a repository with the graph switched off
// is cheap to ask twice, and silently reporting an empty list for one with
// thousands of dependents is not recoverable by the caller.
func (c *Client) rowPage(ctx context.Context, u string, rows page.Sel) (*html.Node, string, error) {
	doc, final, n, err := c.readRows(ctx, u, rows)
	if err != nil || n > 0 {
		return doc, final, err
	}
	c.cacheDrop(u, SurfaceHTML)
	doc, final, n, err = c.readRows(ctx, u, rows)
	if err != nil {
		return nil, "", err
	}
	// An empty page that stays empty is not worth keeping either. The next run
	// would read it back and stop in the same place without ever asking again.
	if n == 0 {
		c.cacheDrop(u, SurfaceHTML)
	}
	return doc, final, nil
}

func (c *Client) readRows(ctx context.Context, u string, rows page.Sel) (*html.Node, string, int, error) {
	res, err := c.GetHTML(ctx, u)
	if err != nil {
		return nil, "", 0, err
	}
	doc := page.Extract(res.FinalURL, res.Body).Doc()
	if doc == nil {
		return nil, res.FinalURL, 0, nil
	}
	return doc, res.FinalURL, len(page.FindAll(doc, rows)), nil
}

// dependencyRow reads one manifest entry. The interesting half is the line under
// the package name, which is one span holding the ecosystem, the manifest, who
// detected it and when, and sometimes the licence, separated by middots.
func dependencyRow(row *html.Node, repo, source string) (Dependency, bool) {
	box := page.Find(row, page.DependencyRow)
	if box == nil {
		return Dependency{}, false
	}
	name := page.Find(box, page.DependencyName)
	if name == nil {
		return Dependency{}, false
	}
	d := Dependency{Repo: repo, Package: strings.TrimSpace(page.Text(name))}
	if d.Package == "" {
		return Dependency{}, false
	}
	if a := page.Find(box, page.DependencyLink); a != nil {
		if p := hrefPath(page.Attr(a, "href")); strings.Count(p, "/") == 1 {
			d.SourceRepo = p
			d.setIdentity(KindRepo, p)
		}
	}
	if v := page.Find(box, page.DependencyVersion); v != nil {
		d.Version = strings.TrimSpace(page.Text(v))
	}
	if r := page.Find(box, page.DependencyRelation); r != nil {
		d.Relationship = strings.ToLower(strings.TrimSpace(page.Text(r)))
	}
	if m := page.Find(row, page.DependencyManifest); m != nil {
		d.Manifest = strings.TrimSpace(page.Text(m))
		if m.Parent != nil {
			d.Ecosystem, d.License = manifestLine(page.Text(m.Parent), d.Manifest)
		}
	}
	d.addSource(source)
	return d, true
}

// manifestLine takes the middot-separated line apart. The ecosystem is always
// first and the licence, when there is one, is always last; the middle is the
// manifest name and the detection note, neither of which needs splitting out
// here.
//
// The note comes in two wordings, "Detected by dependabot on <date>" and
// "Detected automatically on <date>", and when there is no licence the note is
// what sits last, so both prefixes have to be recognised or the date ends up
// filed as the licence.
func manifestLine(text, manifest string) (ecosystem, license string) {
	var parts []string
	for _, p := range strings.Split(text, "·") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "", ""
	}
	ecosystem = parts[0]
	last := parts[len(parts)-1]
	if last != ecosystem && last != manifest && !strings.HasPrefix(last, "Detected ") {
		license = last
	}
	return ecosystem, license
}

// Dependents lists the repositories that depend on this one.
//
// The list is ordered by stars and it is long: a popular library has tens of
// thousands of rows at thirty a page, so --limit is the flag that matters here
// and the walk stops the moment it is reached.
func (c *Client) Dependents(ctx context.Context, repo string, limit int, emit func(Dependent) error) error {
	if _, _, ok := SplitRepo(repo); !ok {
		return usageBadID("repository", repo, "owner/name")
	}
	base := repoSubURL(repo, "network/dependents")
	fetch := func(ctx context.Context, token string) ([]Dependent, string, error) {
		u := base
		if token != "" {
			u = query(u, "dependents_after", token)
		}
		doc, final, err := c.rowPage(ctx, u, page.DependentRow)
		if err != nil {
			return nil, "", err
		}
		if doc == nil {
			return nil, "", structureChanged(repo + " dependents")
		}
		var out []Dependent
		for _, row := range page.FindAll(doc, page.DependentRow) {
			d, ok := dependentRow(row, repo, final)
			if ok {
				out = append(out, d)
			}
		}
		return out, dependentsCursor(doc), nil
	}
	return paginate(ctx, limit, fetch, emit)
}

// dependentRow reads one dependent. Owner and name are two anchors rather than
// one, the same shape the fork list uses, so the id is assembled.
func dependentRow(row *html.Node, repo, source string) (Dependent, bool) {
	link := page.Find(row, page.DependentRepo)
	if link == nil {
		return Dependent{}, false
	}
	id := hrefPath(page.Attr(link, "href"))
	owner, _, ok := SplitRepo(id)
	if !ok {
		return Dependent{}, false
	}
	d := Dependent{Repo: repo, Dependent: id, Owner: owner}
	d.setIdentity(KindRepo, id)
	if u := page.Find(row, page.DependentUser); u != nil {
		if login := hrefPath(page.Attr(u, "href")); login != "" {
			d.Owner = login
		}
	}
	if img := page.Find(row, page.Sel{Tag: "img", Class: "avatar"}); img != nil {
		d.AvatarURL = page.Attr(img, "src")
	}
	d.Stars = iconCount(row, page.DependentStars)
	d.Forks = iconCount(row, page.DependentForks)
	d.addSource(source)
	return d, true
}

// iconCount reads the number beside an icon. The icon is what says which count
// it is, because the two spans are otherwise identical.
func iconCount(row *html.Node, sel page.Sel) *int {
	el := page.Find(row, sel)
	if el == nil {
		return nil
	}
	if n, _, ok := page.CountIn(page.Text(el)); ok {
		return intp(n)
	}
	return nil
}

// dependentsCursor pulls the opaque cursor out of the Next button. There is no
// page number on this listing and no total to count against, so the cursor the
// page hands back is the only way forward.
func dependentsCursor(doc *html.Node) string {
	a := page.Find(doc, page.DependentNext)
	if a == nil {
		return ""
	}
	u, err := url.Parse(page.Attr(a, "href"))
	if err != nil {
		return ""
	}
	return u.Query().Get("dependents_after")
}
