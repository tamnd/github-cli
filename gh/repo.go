package gh

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/tamnd/github-cli/pkg/page"
)

// repo.go reads a repository.
//
// It reads HTML rather than the JSON route, and that is not laziness. The JSON
// route returns the file tree and the ids and nothing else; description,
// homepage, topics, stars, watchers, forks, licence, and the release and tag
// counts live in payload.sidebarAbout, which only ever ships inside the HTML
// document. One request gets all of it.
//
// Merge order, later sources overwriting earlier ones field by field and only
// where they actually said something:
//
//  1. codeViewLayoutRoute.repo   ids and structure
//  2. sidebarAbout               counts and metadata
//  3. codeViewRepoRoute          head SHA, tree, commit count, README
//  4. the DOM                    licence and the language bar
//  5. deferred fragments         languages and dependents, --deep only

// RepoOptions controls how much a repository read costs.
type RepoOptions struct {
	// Deep runs the fragment fetches: the language histogram and the dependent
	// count. Four to six requests instead of one.
	Deep bool
	// Readme keeps the rendered README, which is most of the response body on a
	// well-documented repository.
	Readme bool
}

// Repo reads one repository. id is owner/name.
func (c *Client) Repo(ctx context.Context, id string, opts RepoOptions) (*Repo, error) {
	owner, name, ok := SplitRepo(id)
	if !ok {
		return nil, usageBadID("repository", id, "owner/name")
	}
	res, err := c.GetHTML(ctx, repoURL(id))
	if err != nil {
		return nil, err
	}
	p := page.Extract(res.FinalURL, res.Body)

	r := &Repo{Owner: owner, Name: name}
	r.setIdentity(KindRepo, id)
	r.addSource(res.FinalURL)
	if p.Canonical != "" {
		// Renames 301, and the payload then reports the new name while the
		// caller asked for the old one. The canonical link is the authority.
		r.URL = p.Canonical
	}

	found := false
	if raw, ok := p.Route("codeViewLayoutRoute"); ok {
		found = r.readLayoutRoute(raw) || found
	}
	if raw, ok := p.Payload["sidebarAbout"]; ok {
		found = r.readSidebar(raw) || found
	}
	if raw, ok := p.Route("codeViewRepoRoute"); ok {
		found = r.readRepoRoute(raw, opts.Readme) || found
	}
	if !found {
		return nil, structureChanged(id)
	}
	r.readDOM(p)

	// The language is worth a second request. It is one of the fields people
	// most expect on a repository record, the page carries it only in a bar
	// that is a loading skeleton on a cold fetch, and search answers it in one
	// hop. This runs only when the page did not already say.
	if r.Language == "" {
		if lang, color, err := c.searchLanguage(ctx, id); err == nil && lang != "" {
			r.Language = lang
			r.LanguageColor = color
			recordVia(&r.Base, "language", "search")
		}
	}

	if opts.Deep {
		if err := c.deepenRepo(ctx, r); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// --- payload blocks ---

type layoutRepo struct {
	ID            *int   `json:"id"`
	Name          string `json:"name"`
	OwnerLogin    string `json:"ownerLogin"`
	DefaultBranch string `json:"defaultBranch"`
	CreatedAt     string `json:"createdAt"`
	IsFork        bool   `json:"isFork"`
	IsEmpty       bool   `json:"isEmpty"`
	Private       bool   `json:"private"`
	IsOrgOwned    bool   `json:"isOrgOwned"`
	OwnerAvatar   string `json:"ownerAvatar"`
}

func (r *Repo) readLayoutRoute(raw json.RawMessage) bool {
	var v struct {
		Repo json.RawMessage `json:"repo"`
	}
	if err := json.Unmarshal(raw, &v); err != nil || len(v.Repo) == 0 {
		return false
	}
	var lr layoutRepo
	if err := json.Unmarshal(v.Repo, &lr); err != nil {
		return false
	}
	r.DatabaseID = lr.ID
	if lr.Name != "" {
		r.Name = lr.Name
	}
	if lr.OwnerLogin != "" {
		r.Owner = lr.OwnerLogin
	}
	r.DefaultBranch = lr.DefaultBranch
	r.CreatedAt = parseTime(lr.CreatedAt)
	r.IsFork = lr.IsFork
	r.IsEmpty = lr.IsEmpty
	r.IsPrivate = lr.Private
	r.IsOrgOwned = lr.IsOrgOwned
	r.OwnerAvatarURL = lr.OwnerAvatar

	r.addExtra("codeViewLayoutRoute.repo", decodeExtra(v.Repo, &lr,
		// The inverse of private, and both always arrive.
		"public",
		// Viewer permissions, uniformly false without a session.
		"currentUserCanPush", "currentUserCanFork", "currentUserIsOwner",
	))
	return true
}

type sidebarAbout struct {
	Description     string `json:"description"`
	Website         string `json:"website"`
	StargazerCount  *int   `json:"stargazerCount"`
	WatcherCount    *int   `json:"watcherCount"`
	ForksCount      *int   `json:"forksCount"`
	StargazersPath  string `json:"stargazersPath"`
	ForkNetworkPath string `json:"forkNetworkPath"`
	ActivityPath    string `json:"activityPath"`
	OwnerLogin      string `json:"ownerLogin"`
	RepoName        string `json:"repoName"`
	IsOrg           bool   `json:"isOrg"`
	HasCitation     bool   `json:"hasCitation"`

	Topics []struct {
		Name string `json:"name"`
	} `json:"topics"`

	// Sections is deliberately untyped. Most of its members are a plain bool
	// meaning "this box is on the page", but a few are objects carrying the
	// counts, and which is which changes with what the repository has. A typed
	// struct here loses the whole block the moment one member arrives as true
	// instead of {}, so each member is decoded on its own below and a member
	// that does not fit costs only that member.
	Sections map[string]json.RawMessage `json:"sections"`
}

// releasesSection and usedBySection are the two members that carry numbers.
type releasesSection struct {
	ReleaseCount *int `json:"releaseCount"`
	TagCount     *int `json:"tagCount"`
}

type usedBySection struct {
	DependentsCount *int `json:"dependentsCount"`
}

// section decodes one member of sidebarAbout.sections, and returns false for
// the bool form rather than treating it as a failure.
func section[T any](m map[string]json.RawMessage, name string) (T, bool) {
	var out T
	raw, ok := m[name]
	if !ok || len(raw) == 0 || raw[0] != '{' {
		return out, false
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, false
	}
	return out, true
}

func (r *Repo) readSidebar(raw json.RawMessage) bool {
	var sa sidebarAbout
	if err := json.Unmarshal(raw, &sa); err != nil {
		return false
	}
	r.Description = sa.Description
	r.Homepage = sa.Website
	r.Stars = sa.StargazerCount
	r.Watchers = sa.WatcherCount
	r.Forks = sa.ForksCount
	r.StargazersPath = sa.StargazersPath
	r.ForkNetworkPath = sa.ForkNetworkPath
	r.ActivityPath = sa.ActivityPath
	r.HasCitation = sa.HasCitation
	r.IsOrgOwned = r.IsOrgOwned || sa.IsOrg
	if rel, ok := section[releasesSection](sa.Sections, "releases"); ok {
		r.ReleaseCount = rel.ReleaseCount
		r.TagCount = rel.TagCount
	}
	if used, ok := section[usedBySection](sa.Sections, "usedBy"); ok && used.DependentsCount != nil {
		r.DependentCount = used.DependentsCount
	}
	for _, t := range sa.Topics {
		if t.Name != "" {
			r.Topics = append(r.Topics, t.Name)
		}
	}
	if langs := decodeLanguages(sa.Sections["languages"]); len(langs) > 0 {
		r.Languages = langs
		r.Language = topLanguage(langs)
	}

	r.addExtra("sidebarAbout", decodeExtra(raw, &sa,
		// The description again with emoji shortcodes expanded. The raw form is
		// what the record carries.
		"formattedDescription",
		// UI state and viewer permissions.
		"showInsights", "canEditMetadata", "canEditTopics",
		// Routes to pages with nothing public on them.
		"reportPath", "customPropertiesPath", "watchersPath",
	))
	return true
}

type repoRoute struct {
	Path    string `json:"path"`
	RefInfo struct {
		Name       string `json:"name"`
		RefType    string `json:"refType"`
		CurrentOid string `json:"currentOid"`
	} `json:"refInfo"`
	Tree struct {
		Items      []treeItem `json:"items"`
		TotalCount *int       `json:"totalCount"`
	} `json:"tree"`
	Overview struct {
		CommitCount   string `json:"commitCount"`
		OverviewFiles []struct {
			DisplayName       string `json:"displayName"`
			Path              string `json:"path"`
			PreferredFileType string `json:"preferredFileType"`
			RichText          string `json:"richText"`
		} `json:"overviewFiles"`
	} `json:"overview"`
}

type treeItem struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	ContentType string `json:"contentType"`
}

func (r *Repo) readRepoRoute(raw json.RawMessage, keepReadme bool) bool {
	var rr repoRoute
	if err := json.Unmarshal(raw, &rr); err != nil {
		return false
	}
	r.HeadSHA = rr.RefInfo.CurrentOid
	if r.DefaultBranch == "" {
		r.DefaultBranch = rr.RefInfo.Name
	}
	r.FileCount = rr.Tree.TotalCount

	ref := firstNonEmpty(rr.RefInfo.Name, r.DefaultBranch)
	for _, it := range rr.Tree.Items {
		r.Tree = append(r.Tree, newTreeEntry(r.ID, ref, it))
	}

	// "8,112" in one locale and "8.112" in another, for the same number. Both
	// forms are kept so that the ambiguity stays visible.
	if n, display, ok := page.ParseCompactCount(rr.Overview.CommitCount); ok {
		r.CommitCount = intp(n)
		r.CommitCountDisplay = display
	}

	for _, f := range rr.Overview.OverviewFiles {
		if f.PreferredFileType != "readme" {
			continue
		}
		r.ReadmePath = f.Path
		if keepReadme {
			r.ReadmeHTML = f.RichText
			// The payload has no plain-text form, and a README is the one
			// field most consumers want as prose rather than as markup.
			r.ReadmeText = page.FragmentText(f.RichText)
		}
		break
	}

	r.addExtra("codeViewRepoRoute", decodeExtra(raw, &rr,
		// Chrome: which panels the front end opens, which buttons it draws.
		"banners", "codeButton", "popovers", "treeExpanded", "symbolsExpanded",
		"isOverview", "showBranchInfobar", "userNameDisplayConfiguration",
		// Copilot entitlement, which is a property of the viewer, not the repo.
		"copilot*",
	))
	return true
}

// newTreeEntry builds one entry. The id form is owner/name@ref/path with a
// slash, not a colon: Locate splits on the first slash after the ref, so a colon
// there lands the whole filename inside the ref and produces a URL nobody can
// follow.
func newTreeEntry(repo, ref string, it treeItem) TreeEntry {
	e := TreeEntry{Repo: repo, Ref: ref, Name: it.Name, Path: it.Path, Type: it.ContentType}
	kind := KindFile
	if it.ContentType == "directory" {
		kind = KindTree
	}
	e.setIdentity(kind, repo+"@"+ref+"/"+it.Path)
	return e
}

// decodeLanguages reads the language histogram out of sections.languages. The
// block is lazily populated and is an empty object on a cold page, which is why
// there is a language-bar fallback below.
func decodeLanguages(raw json.RawMessage) map[string]int64 {
	if len(raw) == 0 {
		return nil
	}
	var v struct {
		Languages []struct {
			Name  string `json:"name"`
			Size  *int64 `json:"size"`
			Bytes *int64 `json:"bytes"`
		} `json:"languages"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	out := map[string]int64{}
	for _, l := range v.Languages {
		switch {
		case l.Size != nil:
			out[l.Name] = *l.Size
		case l.Bytes != nil:
			out[l.Name] = *l.Bytes
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func topLanguage(langs map[string]int64) string {
	best, bestN := "", int64(-1)
	for _, k := range sortedKeys(langs) {
		if langs[k] > bestN {
			best, bestN = k, langs[k]
		}
	}
	return best
}

// --- the DOM pass ---

// readDOM picks up the two fields that exist nowhere in any payload: the
// licence and the language bar. Both are class selectors, both are in the
// bottom tier of the preference ladder, and both degrade to absent rather than
// to wrong.
func (r *Repo) readDOM(p *page.Page) {
	doc := p.Doc()
	if doc == nil {
		return
	}
	if r.SocialImageURL == "" {
		r.SocialImageURL = p.MetaContent("og:image")
	}
	if lic := licenseFrom(doc); lic != "" {
		r.License = lic
	}
	if len(r.Languages) == 0 {
		if langs, pct := languageBar(doc); len(langs) > 0 {
			r.Languages = langs
			r.Language = topLanguage(langs)
			if pct {
				// Percentages are not byte counts and the record must not
				// pretend otherwise.
				recordVia(&r.Base, "languages", "bar")
			}
		}
	}
	if r.Description == "" {
		r.Description = descriptionFromOG(p.MetaContent("og:title"), r.ID)
	}
}

// licenseFrom reads the About-sidebar licence link. This is the single most
// fragile extraction in the tool: the anchor is found by the icon inside it,
// because the icon outlives the anchor's classes. When it stops matching, the
// field is absent, which is a truthful answer, rather than empty, which would
// be a claim.
func licenseFrom(doc *html.Node) string {
	a := page.Find(doc, page.LicenseLink)
	if a == nil {
		return ""
	}
	text := page.Text(a)
	text = strings.TrimSuffix(text, " license")
	text = strings.TrimSuffix(text, " License")
	return strings.TrimSpace(text)
}

// languageBar reads the coloured bar under the About box. The second return
// says the values are percentages times one hundred rather than byte counts,
// which the caller then records so that nobody aggregates the two together.
func languageBar(doc *html.Node) (map[string]int64, bool) {
	links := page.FindAll(doc, page.LanguageBarItem)
	if len(links) == 0 {
		return nil, false
	}
	out := map[string]int64{}
	for _, a := range links {
		nameNode := page.Find(a, page.LanguageBarName)
		if nameNode == nil {
			continue
		}
		name := page.Text(nameNode)
		rest := strings.TrimSpace(strings.TrimPrefix(page.Text(a), name))
		pct, err := strconv.ParseFloat(strings.TrimSuffix(rest, "%"), 64)
		if err != nil || name == "" {
			continue
		}
		out[name] = int64(pct * 100)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// descriptionFromOG recovers a description from the Open Graph title, which on
// a repository reads "GitHub - owner/name: the description". This looks like a
// hack until the sidebar has not loaded, which happens, and then it is the only
// description on the page.
func descriptionFromOG(title, id string) string {
	marker := id + ": "
	i := strings.Index(title, marker)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(title[i+len(marker):])
}

// --- the deep pass ---

// deepenRepo runs the extra fetch --deep opts into: the dependent count off the
// dependency graph. The failure is soft. A dependency graph that is disabled is
// a fact about the repository, not an error in the read.
func (c *Client) deepenRepo(ctx context.Context, r *Repo) error {
	if n, err := c.dependents(ctx, r.ID); err == nil && n != nil {
		r.DependentCount = n
	}
	r.addSource(repoSubURL(r.ID, "network/dependents"))
	return nil
}

// searchLanguage asks repository search for the repository by name, because the
// search result carries the primary language and its colour and the repository
// page does not.
//
// The obvious place to look is /{owner}/{repo}/graphs/languages, and it is a
// dead end: it 301s back to the repository page for an anonymous client, and
// none of show_partial, /languages, or /graphs/languages-data exist. The
// language bar in the sidebar is the other source, and on a cold page it is a
// skeleton with no /search?l= links in it at all. So the histogram with real
// byte counts has no keyless source, and the language name does, one search away.
func (c *Client) searchLanguage(ctx context.Context, id string) (lang, color string, err error) {
	owner, name, ok := SplitRepo(id)
	if !ok {
		return "", "", usageBadID("repository", id, "owner/name")
	}
	q := "repo:" + owner + "/" + name
	err = c.SearchRepositories(ctx, q, 5, func(r Repo) error {
		if r.ID == id && r.Language != "" {
			lang, color = r.Language, r.LanguageColor
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}
	return lang, color, nil
}

// dependents reads the count off the /network/dependents heading. Prose, marked
// fragile, and absent when the graph is off or private.
func (c *Client) dependents(ctx context.Context, id string) (*int, error) {
	res, err := c.GetHTML(ctx, repoSubURL(id, "network/dependents"))
	if err != nil {
		return nil, err
	}
	p := page.Extract(res.FinalURL, res.Body)
	doc := p.Doc()
	if doc == nil {
		return nil, nil
	}
	for _, a := range page.FindAll(doc, page.Sel{Tag: "a", Attr: "href", AttrContains: "dependent_type=REPOSITORY"}) {
		if n, _, ok := page.CountIn(page.Text(a)); ok {
			return intp(n), nil
		}
	}
	return nil, nil
}
