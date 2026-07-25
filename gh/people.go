package gh

import (
	"context"
	"encoding/xml"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/tamnd/any-cli/kit/errs"
	"github.com/tamnd/github-cli/pkg/page"
)

// people.go is everything that hangs off an account: who follows whom, what
// they starred, what they published, and what they did.
//
// None of it has a JSON payload. The profile tabs, the organization roster, and
// the gist index are all Rails, and the activity stream is Atom. That is the
// whole reason this file exists as its own unit: the surfaces here share a
// pager and a row shape with each other and with nothing else in the tool.
//
// The pager is the "rails" one from surface.go: fetch a page, decode the rows,
// look for the link that says next. Two templates write that link two ways and
// nextPageHref knows both.

// Followers lists the accounts following a login, newest first, which is the
// order the page uses and the only order it offers.
func (c *Client) Followers(ctx context.Context, login string, limit int, emit func(Account) error) error {
	return c.profileAccounts(ctx, login, "followers", limit, emit)
}

// Following lists the accounts a login follows.
func (c *Client) Following(ctx context.Context, login string, limit int, emit func(Account) error) error {
	return c.profileAccounts(ctx, login, "following", limit, emit)
}

func (c *Client) profileAccounts(ctx context.Context, login, tab string, limit int, emit func(Account) error) error {
	if login == "" || strings.Contains(login, "/") {
		return usageBadID("account", login, "a bare login")
	}
	base := query(accountURL(login), "tab", tab)
	fetch := func(ctx context.Context, token string) ([]Account, string, error) {
		u := base
		if n := pageToken(token); n > 1 {
			u = query(u, "page", strconv.Itoa(n))
		}
		res, err := c.GetHTML(ctx, u)
		if err != nil {
			return nil, "", err
		}
		doc := page.Extract(res.FinalURL, res.Body).Doc()
		if doc == nil {
			return nil, "", structureChanged(login + " " + tab)
		}
		var out []Account
		for _, row := range page.FindAll(doc, page.Sel{Class: "d-table"}) {
			a, ok := followRow(row, res.FinalURL)
			if ok {
				out = append(out, a)
			}
		}
		return out, railsNext(doc, token), nil
	}
	return paginate(ctx, limit, fetch, emit)
}

// followRow reads one row of a followers or following list. The row has the
// login twice, once in the avatar link and once in the muted span, and the
// display name in the primary span when the person set one.
func followRow(row *html.Node, source string) (Account, bool) {
	link := page.Find(row, page.Sel{Tag: "a", Attr: "data-hovercard-type", AttrValue: "user"})
	if link == nil {
		return Account{}, false
	}
	who := actorFromHref(page.Attr(link, "href"))
	if who.Login == "" {
		return Account{}, false
	}
	a := Account{Login: who.Login, Type: "User"}
	a.setIdentity(KindUser, who.Login)
	if n := page.Find(row, page.Sel{Class: "Link--primary"}); n != nil {
		a.Name = page.Text(n)
	}
	if img := page.Find(row, page.Sel{Tag: "img", Class: "avatar-user"}); img != nil {
		a.AvatarURL = page.Attr(img, "src")
	}
	a.addSource(source)
	return a, true
}

// Members lists an organization's public members. The roster is at
// /orgs/{login}/people rather than on the profile, and the profile's avatar
// strip is a sample of it rather than a short version of it.
func (c *Client) Members(ctx context.Context, login string, limit int, emit func(Account) error) error {
	if login == "" || strings.Contains(login, "/") {
		return usageBadID("organization", login, "a bare login")
	}
	base := BaseURL + "/orgs/" + login + "/people"
	fetch := func(ctx context.Context, token string) ([]Account, string, error) {
		u := base
		if n := pageToken(token); n > 1 {
			u = query(u, "page", strconv.Itoa(n))
		}
		res, err := c.GetHTML(ctx, u)
		if err != nil {
			return nil, "", err
		}
		doc := page.Extract(res.FinalURL, res.Body).Doc()
		if doc == nil {
			return nil, "", structureChanged(login + " members")
		}
		var out []Account
		for _, li := range page.FindAll(doc, page.Sel{Class: "member-list-item"}) {
			m, ok := memberRow(li, res.FinalURL)
			if ok {
				out = append(out, m)
			}
		}
		return out, railsNext(doc, token), nil
	}
	return paginate(ctx, limit, fetch, emit)
}

// memberRow reads one member. The role is behind a batch-deferred fragment that
// needs a session, so it is absent here rather than wrong.
func memberRow(li *html.Node, source string) (Account, bool) {
	name := page.Find(li, page.Sel{Tag: "a", Attr: "id", AttrPrefix: "member-"})
	if name == nil {
		return Account{}, false
	}
	who := actorFromHref(page.Attr(name, "href"))
	if who.Login == "" {
		return Account{}, false
	}
	a := Account{Login: who.Login, Type: "User", Name: page.Text(name)}
	a.setIdentity(KindUser, who.Login)
	// The name anchor falls back to the login when the person set no display
	// name, and a Name that repeats the Login says nothing.
	if a.Name == a.Login {
		a.Name = ""
	}
	if img := page.Find(li, page.Sel{Tag: "img", Class: "avatar-user"}); img != nil {
		a.AvatarURL = page.Attr(img, "src")
		if id := avatarUserID(page.Attr(img, "src")); id > 0 {
			a.DatabaseID = intp(id)
		}
	}
	a.addSource(source)
	return a, true
}

// avatarUserID pulls the numeric account id out of an avatar URL. It is the
// only place a listing states it, and having it lets a record join to search
// results, which key on the same number.
func avatarUserID(src string) int {
	_, rest, ok := strings.Cut(src, "/u/")
	if !ok {
		return 0
	}
	digits, _, _ := strings.Cut(rest, "?")
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}
	return n
}

// Starred lists the repositories an account has starred. It is a different
// template from the repositories tab, so it gets its own row reader even though
// the two records are the same shape.
func (c *Client) Starred(ctx context.Context, login string, limit int, emit func(Repo) error) error {
	return c.profileRepos(ctx, login, "stars", limit, emit)
}

// ReposAsShown lists an account's repositories in the order and with the
// filters the profile tab itself uses. `github repos --user x` runs a search
// instead, which sorts and pages better; this is what --as-shown selects when
// the exact page order is the point.
func (c *Client) ReposAsShown(ctx context.Context, login string, limit int, emit func(Repo) error) error {
	return c.profileRepos(ctx, login, "repositories", limit, emit)
}

func (c *Client) profileRepos(ctx context.Context, login, tab string, limit int, emit func(Repo) error) error {
	if login == "" || strings.Contains(login, "/") {
		return usageBadID("account", login, "a bare login")
	}
	base := query(accountURL(login), "tab", tab)
	fetch := func(ctx context.Context, token string) ([]Repo, string, error) {
		u := base
		if n := pageToken(token); n > 1 {
			u = query(u, "page", strconv.Itoa(n))
		}
		res, err := c.GetHTML(ctx, u)
		if err != nil {
			return nil, "", err
		}
		doc := page.Extract(res.FinalURL, res.Body).Doc()
		if doc == nil {
			return nil, "", structureChanged(login + " " + tab)
		}
		var out []Repo
		for _, h := range repoCardHeadings(doc) {
			if r, ok := repoCard(h, res.FinalURL); ok {
				out = append(out, r)
			}
		}
		return out, railsNext(doc, token), nil
	}
	return paginate(ctx, limit, fetch, emit)
}

// repoCardHeadings finds the heading of every repository card on a listing
// page. The repositories tab marks the name with microdata and the stars tab
// does not, so both hooks are tried and the results are kept in document order
// rather than merged, since no page uses both.
func repoCardHeadings(doc *html.Node) []*html.Node {
	if named := page.FindAll(doc, page.Sel{Attr: "itemprop", AttrValue: "name codeRepository"}); len(named) > 0 {
		return named
	}
	var out []*html.Node
	for _, h := range page.FindAll(doc, page.Sel{Tag: "h3"}) {
		if a := page.Find(h, page.Sel{Tag: "a", Attr: "href"}); a != nil {
			if p := hrefPath(page.Attr(a, "href")); strings.Count(p, "/") == 1 {
				out = append(out, a)
			}
		}
	}
	return out
}

// repoCard reads a repository out of a listing card, walking up from the name
// link to the row that holds the rest of the fields.
func repoCard(nameLink *html.Node, source string) (Repo, bool) {
	id := hrefPath(page.Attr(nameLink, "href"))
	if strings.Count(id, "/") != 1 {
		return Repo{}, false
	}
	owner, name, ok := SplitRepo(id)
	if !ok {
		return Repo{}, false
	}
	r := Repo{Owner: owner, Name: name}
	r.setIdentity(KindRepo, id)
	r.addSource(source)

	row := cardRow(nameLink)
	if row == nil {
		return r, true
	}
	if d := page.Find(row, page.Sel{Attr: "itemprop", AttrValue: "description"}); d != nil {
		r.Description = page.Text(d)
	}
	if l := page.Find(row, page.Sel{Attr: "itemprop", AttrValue: "programmingLanguage"}); l != nil {
		r.Language = page.Text(l)
	}
	if c := page.Find(row, page.Sel{Class: "repo-language-color"}); c != nil {
		r.LanguageColor = styleColor(page.Attr(c, "style"))
	}
	for _, a := range page.FindAll(row, page.Sel{Tag: "a", Attr: "href"}) {
		href := page.Attr(a, "href")
		n, _, ok := page.ParseCompactCount(page.Text(a))
		if !ok {
			continue
		}
		switch {
		case strings.HasSuffix(href, "/stargazers"):
			r.Stars = intp(n)
		case strings.HasSuffix(href, "/forks"):
			r.Forks = intp(n)
		}
	}
	if t := page.Find(row, page.RelTimeEl); t != nil {
		r.PushedAt = parseTime(page.Attr(t, "datetime"))
	}
	for _, tag := range page.FindAll(row, page.Sel{Class: "topic-tag"}) {
		if s := page.Text(tag); s != "" {
			r.Topics = append(r.Topics, s)
		}
	}
	return r, true
}

// cardRow walks up to the element that contains a whole listing card. Four
// levels is what separates the name link from the row on every template that
// uses one, and stopping there keeps a malformed page from handing back the
// document root and with it every field on it.
func cardRow(n *html.Node) *html.Node {
	for i := 0; i < 4 && n != nil; i++ {
		n = n.Parent
		if n == nil {
			return nil
		}
		if n.Type == html.ElementNode && (n.Data == "li" || page.HasClass(n, "col-12") || page.HasClass(n, "Box-row")) {
			return n
		}
	}
	return n
}

// styleColor pulls a colour out of an inline style, which is where GitHub puts
// the language colour on every listing template.
func styleColor(style string) string {
	_, rest, ok := strings.Cut(style, "background-color:")
	if !ok {
		return ""
	}
	v, _, _ := strings.Cut(rest, ";")
	return strings.TrimSpace(v)
}

// --- gists ---

// Gists lists an account's public gists.
func (c *Client) Gists(ctx context.Context, login string, limit int, emit func(Gist) error) error {
	if login == "" || strings.Contains(login, "/") {
		return usageBadID("account", login, "a bare login")
	}
	base := GistURL + "/" + login
	fetch := func(ctx context.Context, token string) ([]Gist, string, error) {
		u := base
		if n := pageToken(token); n > 1 {
			u = query(u, "page", strconv.Itoa(n))
		}
		res, err := c.GetHTML(ctx, u)
		if err != nil {
			return nil, "", err
		}
		doc := page.Extract(res.FinalURL, res.Body).Doc()
		if doc == nil {
			return nil, "", structureChanged(login + " gists")
		}
		var out []Gist
		for _, snip := range page.FindAll(doc, page.Sel{Class: "gist-snippet"}) {
			if g, ok := gistSnippet(snip, res.FinalURL); ok {
				out = append(out, g)
			}
		}
		return out, railsNext(doc, token), nil
	}
	return paginate(ctx, limit, fetch, emit)
}

// gistSnippet reads one entry of a gist index. The counts are in the text of
// the links beside it: "1 file", "6 forks", "62 stars".
func gistSnippet(snip *html.Node, source string) (Gist, bool) {
	var id, owner string
	for _, a := range page.FindAll(snip, page.Sel{Tag: "a", Attr: "href"}) {
		p := hrefPath(page.Attr(a, "href"))
		o, rest, ok := strings.Cut(p, "/")
		if !ok {
			continue
		}
		hex, _, _ := strings.Cut(rest, "/")
		if isGistID(hex) {
			owner, id = o, hex
			break
		}
	}
	if id == "" {
		return Gist{}, false
	}
	g := Gist{Owner: owner, IsPublic: true}
	g.setIdentity(KindGist, id)
	g.addSource(source)
	for _, a := range page.FindAll(snip, page.Sel{Tag: "a", Attr: "href"}) {
		text := page.Text(a)
		n, _, ok := page.CountIn(text)
		if !ok {
			continue
		}
		switch {
		case strings.HasSuffix(text, "file"), strings.HasSuffix(text, "files"):
			g.FileCount = intp(n)
		case strings.HasSuffix(text, "fork"), strings.HasSuffix(text, "forks"):
			g.Forks = intp(n)
		case strings.HasSuffix(text, "star"), strings.HasSuffix(text, "stars"):
			g.Stars = intp(n)
		}
	}
	if d := page.Find(snip, page.Sel{Class: "gist-snippet-meta"}); d != nil {
		if p := page.Find(d, page.Sel{Tag: "span", Class: "f6"}); p != nil {
			g.Description = page.Text(p)
		}
	}
	if t := page.Find(snip, page.RelTimeEl); t != nil {
		g.UpdatedAt = parseTime(page.Attr(t, "datetime"))
	}
	return g, true
}

// Gist reads one gist and its file list. Contents are a second request per file
// and are opt-in, because a gist can hold a megabyte of log paste.
func (c *Client) Gist(ctx context.Context, id string, withContent bool) (*Gist, error) {
	id = strings.TrimSpace(id)
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	if !isGistID(id) {
		return nil, usageBadID("gist", id, "a hexadecimal gist id")
	}
	res, err := c.GetHTML(ctx, GistURL+"/"+id)
	if err != nil {
		return nil, err
	}
	p := page.Extract(res.FinalURL, res.Body)
	doc := p.Doc()
	if doc == nil {
		return nil, structureChanged(id)
	}
	g := &Gist{IsPublic: true}
	g.setIdentity(KindGist, id)
	g.addSource(res.FinalURL)
	g.Owner = hrefOwner(res.FinalURL)
	if d := page.Find(doc, page.Sel{Attr: "itemprop", AttrValue: "about"}); d != nil {
		g.Description = page.Text(d)
	}
	if t := page.Find(doc, page.RelTimeEl); t != nil {
		g.UpdatedAt = parseTime(page.Attr(t, "datetime"))
	}
	for _, box := range page.FindAll(doc, page.Sel{Class: "file"}) {
		f, ok := gistFile(box)
		if !ok {
			continue
		}
		g.Files = append(g.Files, f)
	}
	if len(g.Files) == 0 {
		return nil, structureChanged(id)
	}
	g.FileCount = intp(len(g.Files))
	if withContent {
		for i := range g.Files {
			text, err := c.text(ctx, g.Files[i].RawURL)
			if err != nil {
				return nil, err
			}
			g.Files[i].Content = text
		}
	}
	return g, nil
}

// gistFile reads one file block. The raw link is the useful half: it is the
// only address on the page that returns the bytes rather than the rendering.
func gistFile(box *html.Node) (GistFile, bool) {
	name := page.Find(box, page.Sel{Class: "gist-blob-name"})
	if name == nil {
		return GistFile{}, false
	}
	f := GistFile{Name: page.Text(name)}
	if f.Name == "" {
		return GistFile{}, false
	}
	for _, a := range page.FindAll(box, page.Sel{Tag: "a", Attr: "href", AttrContains: "/raw/"}) {
		f.RawURL = absoluteGistURL(page.Attr(a, "href"))
		break
	}
	if f.RawURL == "" {
		return GistFile{}, false
	}
	if i := strings.LastIndex(f.Name, "."); i > 0 {
		f.Language = f.Name[i+1:]
	}
	return f, true
}

// text fetches a URL and returns it as a string. It is the small sibling of
// Raw, for the addresses that are already absolute.
func (c *Client) text(ctx context.Context, rawURL string) (string, error) {
	res, err := c.Get(ctx, rawURL, SurfaceRaw)
	if err != nil {
		return "", err
	}
	return string(res.Body), nil
}

func absoluteGistURL(href string) string {
	if strings.Contains(href, "://") {
		return href
	}
	return GistURL + "/" + strings.TrimPrefix(href, "/")
}

func hrefOwner(rawURL string) string {
	p := hrefPath(rawURL)
	owner, rest, ok := strings.Cut(p, "/")
	if !ok || !isGistID(rest) {
		return ""
	}
	return owner
}

// isGistID matches the twenty-or-more hexadecimal characters a gist is named
// with. Anything shorter is a login or a route word.
func isGistID(s string) bool {
	if len(s) < 20 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// --- the contribution calendar ---

// Contributions reads a year of a profile's contribution graph, one record per
// day. This is the only representation of the numbers that exists without a
// token: the GraphQL field that carries them refuses anonymous callers.
//
// A year is the largest window the fragment serves. Asking for a wider range
// gets the last year, so the range is stated rather than inferred.
func (c *Client) Contributions(ctx context.Context, login string, year int, emit func(ContributionDay) error) error {
	if login == "" || strings.Contains(login, "/") {
		return usageBadID("account", login, "a bare login")
	}
	if year == 0 {
		year = time.Now().UTC().Year()
	}
	from := strconv.Itoa(year) + "-01-01"
	to := strconv.Itoa(year) + "-12-31"
	u := query(BaseURL+"/users/"+login+"/contributions", "from", from, "to", to)
	// HTML rather than XHR. This is a fragment the front end swaps into the
	// profile, so it serves markup and answers 406 to a request that asks for
	// JSON, which the client reports as a response rather than an error and
	// would show up here as an empty calendar.
	res, err := c.Get(ctx, u, SurfaceHTML)
	if err != nil {
		return err
	}
	doc := page.Extract(res.FinalURL, res.Body).Doc()
	if doc == nil {
		return structureChanged(login + " contributions")
	}

	// The count is not on the cell. Each cell points at a tooltip by id and the
	// tooltip holds the sentence with the number in it, so the tooltips are
	// indexed first and the cells read against that index.
	counts := map[string]int{}
	for _, tip := range page.FindAll(doc, page.Sel{Tag: "tool-tip"}) {
		counts[page.Attr(tip, "for")] = leadingCount(page.Text(tip))
	}
	found := false
	for _, td := range page.FindAll(doc, page.Sel{Tag: "td", Attr: "data-date"}) {
		day := ContributionDay{Login: login}
		at := parseTime(page.Attr(td, "data-date"))
		if at == nil {
			continue
		}
		found = true
		day.Date = *at
		day.Level, _ = strconv.Atoi(page.Attr(td, "data-level"))
		day.Count = counts[page.Attr(td, "id")]
		day.Kind = KindContribution
		day.ID = login + "@" + page.Attr(td, "data-date")
		day.URI = URI(KindContribution, day.ID)
		day.URL = accountURL(login)
		day.addSource(res.FinalURL)
		if err := emit(day); err != nil {
			return err
		}
	}
	if !found {
		return structureChanged(login + " contributions")
	}
	return nil
}

// leadingCount reads the number off the front of "7 contributions on January
// 4th." and treats "No contributions" as the zero it is.
func leadingCount(s string) int {
	field, _, _ := strings.Cut(strings.TrimSpace(s), " ")
	n, err := strconv.Atoi(strings.ReplaceAll(field, ",", ""))
	if err != nil {
		return 0
	}
	return n
}

// --- activity ---

// Activity reads a public event stream. The same feed shape serves an account
// and a repository, so the argument is either a login or owner/name and the
// URL is the only thing that differs.
//
// This replaces the REST events endpoint outright. The feed is public, cheap,
// and needs no credential, and the event class is encoded in each entry's id,
// so the type comes from a field rather than from matching on prose.
func (c *Client) Activity(ctx context.Context, ref string, limit int, emit func(Event) error) error {
	ref = strings.Trim(ref, "/")
	if ref == "" {
		return usageBadID("account or repository", ref, "a login or owner/name")
	}
	u := feedURL(ref + ".atom")
	if _, _, ok := SplitRepo(ref); ok {
		u = repoSubURL(ref, "commits.atom")
	}
	res, err := c.Get(ctx, u, SurfaceFeed)
	if err != nil {
		return err
	}
	var feed atomFeed
	if err := xml.Unmarshal(res.Body, &feed); err != nil {
		return badPayload(shortURL(u), err)
	}
	seen := 0
	for _, e := range feed.Entries {
		ev := e.event(res.FinalURL)
		if err := emit(ev); err != nil {
			return err
		}
		seen++
		if limit > 0 && seen >= limit {
			return nil
		}
	}
	if seen == 0 {
		return errs.NotFound("empty feed: %s carried no entries", shortURL(u))
	}
	return nil
}

// atomFeed is the shape all five of GitHub's feeds share. Only the id encoding
// differs between them, and that is read per entry rather than per feed.
type atomFeed struct {
	Title   string      `xml:"title"`
	Updated string      `xml:"updated"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID        string `xml:"id"`
	Title     string `xml:"title"`
	Published string `xml:"published"`
	Updated   string `xml:"updated"`
	Content   string `xml:"content"`
	Link      struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Author struct {
		Name string `xml:"name"`
		URI  string `xml:"uri"`
	} `xml:"author"`
	Thumbnail struct {
		URL string `xml:"url,attr"`
	} `xml:"thumbnail"`
}

// event turns one entry into a record. The id is
// "tag:github.com,2008:push/15757005823", so the segment after the colon is the
// event class and the tool never has to read the localised title to find out
// what happened.
func (e atomEntry) event(source string) Event {
	ev := Event{Title: page.FragmentText(e.Title)}
	_, tail, _ := strings.Cut(e.ID, "2008:")
	class, rest, _ := strings.Cut(tail, "/")
	ev.Type = eventType(class)
	ev.Kind = KindEvent
	ev.ID = e.ID
	if ev.Type != "" && rest != "" {
		ev.ID = ev.Type + "/" + rest
	}
	ev.URI = URI(KindEvent, ev.ID)
	ev.URL = e.Link.Href
	ev.Target = e.Link.Href
	ev.At = firstTime(e.Published, e.Updated)
	ev.BodyHTML = e.Content
	if e.Author.Name != "" {
		ev.Actor = actor(e.Author.Name)
		ev.Actor.AvatarURL = e.Thumbnail.URL
	}
	// The alternate link points at whatever the event touched, and the first
	// two segments of it are the repository whenever there is one.
	if p := hrefPath(e.Link.Href); strings.Count(p, "/") >= 1 {
		owner, rest, _ := strings.Cut(p, "/")
		name, _, _ := strings.Cut(rest, "/")
		if owner != "" && name != "" && !routeWord[name] {
			ev.Repo = owner + "/" + name
		}
	}
	ev.addSource(source)
	return ev
}

// eventType turns the class out of the entry id into one word.
//
// A person's feed names the class in lower case, push or fork or watch. A
// repository's commit feed names it after the Ruby object that used to render
// it, Grit::Commit, which is an implementation detail from 2008 and not a thing
// anyone should have to filter on.
func eventType(class string) string {
	if _, tail, ok := strings.Cut(class, "::"); ok {
		class = tail
	}
	return strings.ToLower(class)
}

func firstTime(ss ...string) *time.Time {
	for _, s := range ss {
		if t := parseTime(s); t != nil {
			return t
		}
	}
	return nil
}

// --- the rails pager ---

// railsNext returns the page number to ask for next, or empty when the page
// says there is no next.
//
// Two templates write the same link two ways: the organization roster marks it
// rel="next" and the profile tabs use a plain anchor whose text is Next. Both
// are checked because both are load-bearing.
func railsNext(doc *html.Node, token string) string {
	if !hasNextLink(doc) {
		return ""
	}
	return strconv.Itoa(pageToken(token) + 1)
}

func hasNextLink(doc *html.Node) bool {
	if page.Find(doc, page.NextPage) != nil {
		return true
	}
	for _, a := range page.FindAll(doc, page.Sel{Tag: "a", Attr: "href"}) {
		if strings.EqualFold(strings.TrimSpace(page.Text(a)), "next") {
			return true
		}
	}
	return false
}
