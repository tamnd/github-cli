package gh

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/tamnd/github-cli/pkg/page"
)

// discover.go reads the pages that answer "what is out there": trending, topic
// pages, fork networks, and repository statistics.
//
// Trending is the clearest case for this whole tool. There is no JSON version
// of it anywhere, with a token or without, so a page decoder is not a fallback
// here, it is the only implementation that can exist.

// TrendingOptions are the three knobs the trending page has.
type TrendingOptions struct {
	// Since is daily, weekly, or monthly. Empty means daily, which is what the
	// page defaults to.
	Since string
	// Language filters by the language slug in the URL, not by a query.
	Language string
	// SpokenLanguage is the natural-language filter, a two-letter code.
	SpokenLanguage string
	Limit          int
}

// Trending lists the trending repositories. Rank is the position on the page,
// which is the only ordering the surface has and is worth keeping, since the
// list has no other stable key.
func (c *Client) Trending(ctx context.Context, opts TrendingOptions, emit func(Trending) error) error {
	u := trendingURL("", opts)
	res, err := c.GetHTML(ctx, u)
	if err != nil {
		return err
	}
	doc := page.Extract(res.FinalURL, res.Body).Doc()
	if doc == nil {
		return structureChanged("trending")
	}
	period := firstNonEmpty(opts.Since, "daily")
	rank := 0
	for _, row := range page.FindAll(doc, page.TrendingRow) {
		t, ok := trendingRow(row, period, res.FinalURL)
		if !ok {
			continue
		}
		rank++
		t.Rank = rank
		if err := emit(t); err != nil {
			return err
		}
		if opts.Limit > 0 && rank >= opts.Limit {
			return nil
		}
	}
	if rank == 0 {
		return structureChanged("trending")
	}
	return nil
}

// TrendingDevelopers lists the trending developers, each with the repository
// the page picked out for them.
func (c *Client) TrendingDevelopers(ctx context.Context, opts TrendingOptions, emit func(Account) error) error {
	u := trendingURL("developers", opts)
	res, err := c.GetHTML(ctx, u)
	if err != nil {
		return err
	}
	doc := page.Extract(res.FinalURL, res.Body).Doc()
	if doc == nil {
		return structureChanged("trending developers")
	}
	seen := 0
	for _, row := range page.FindAll(doc, page.Sel{Tag: "article", Class: "Box-row"}) {
		a, ok := trendingDev(row, res.FinalURL)
		if !ok {
			continue
		}
		if err := emit(a); err != nil {
			return err
		}
		seen++
		if opts.Limit > 0 && seen >= opts.Limit {
			return nil
		}
	}
	if seen == 0 {
		return structureChanged("trending developers")
	}
	return nil
}

// trendingURL builds the trending address. The language is a path segment and
// the period is a query parameter, which is the site's own split and not one
// worth normalising away.
func trendingURL(section string, opts TrendingOptions) string {
	u := BaseURL + "/trending"
	if section != "" {
		u += "/" + section
	} else if opts.Language != "" {
		u += "/" + strings.ToLower(opts.Language)
	}
	var kv []string
	if opts.Since != "" {
		kv = append(kv, "since", opts.Since)
	}
	if opts.SpokenLanguage != "" {
		kv = append(kv, "spoken_language_code", opts.SpokenLanguage)
	}
	if len(kv) == 0 {
		return u
	}
	return query(u, kv...)
}

// trendingRow reads one card. The three counts on it are the same shape and
// only their link tells them apart: stargazers, forks, and the period figure,
// which has no link at all.
func trendingRow(row *html.Node, period, source string) (Trending, bool) {
	h := page.Find(row, page.Sel{Tag: "h2"})
	if h == nil {
		return Trending{}, false
	}
	a := page.Find(h, page.Sel{Tag: "a", Attr: "href"})
	if a == nil {
		return Trending{}, false
	}
	id := hrefPath(page.Attr(a, "href"))
	owner, name, ok := SplitRepo(id)
	if !ok {
		return Trending{}, false
	}
	t := Trending{Period: period}
	t.Owner, t.Name = owner, name
	t.setIdentity(KindRepo, id)
	t.addSource(source)

	if p := page.Find(row, page.Sel{Tag: "p"}); p != nil {
		t.Description = page.Text(p)
	}
	if l := page.Find(row, page.Sel{Attr: "itemprop", AttrValue: "programmingLanguage"}); l != nil {
		t.Language = page.Text(l)
	}
	if col := page.Find(row, page.Sel{Class: "repo-language-color"}); col != nil {
		t.LanguageColor = styleColor(page.Attr(col, "style"))
	}
	for _, link := range page.FindAll(row, page.Sel{Tag: "a", Attr: "href"}) {
		n, _, ok := page.ParseCompactCount(page.Text(link))
		if !ok {
			continue
		}
		switch href := page.Attr(link, "href"); {
		case strings.HasSuffix(href, "/stargazers"):
			t.Stars = intp(n)
		case strings.HasSuffix(href, "/forks"):
			t.Forks = intp(n)
		}
	}
	if s := page.Find(row, page.Sel{Class: "float-sm-right"}); s != nil {
		if n, _, ok := page.CountIn(page.Text(s)); ok {
			t.StarsInPeriod = intp(n)
		}
	}
	for _, img := range page.FindAll(row, page.Sel{Tag: "img", Class: "avatar-user"}) {
		login := strings.TrimPrefix(page.Attr(img, "alt"), "@")
		if login == "" {
			continue
		}
		who := actor(login)
		who.AvatarURL = page.Attr(img, "src")
		t.BuiltBy = append(t.BuiltBy, who)
	}
	return t, true
}

// trendingDev reads one developer card. The popular repository on it is a
// pointer, not a record: it has a name and a description and nothing else, so
// it goes into PinnedRepos where the profile's own picks go.
func trendingDev(row *html.Node, source string) (Account, bool) {
	link := page.Find(row, page.Sel{Tag: "h1", Class: "h3"})
	if link == nil {
		return Account{}, false
	}
	nameLink := page.Find(link, page.Sel{Tag: "a", Attr: "href"})
	if nameLink == nil {
		return Account{}, false
	}
	login := hrefPath(page.Attr(nameLink, "href"))
	if login == "" || strings.Contains(login, "/") {
		return Account{}, false
	}
	a := Account{Login: login, Type: "User", Name: page.Text(nameLink)}
	a.setIdentity(KindUser, login)
	a.addSource(source)
	if a.Name == a.Login {
		a.Name = ""
	}
	if img := page.Find(row, page.Sel{Tag: "img", Class: "avatar-user"}); img != nil {
		a.AvatarURL = page.Attr(img, "src")
	}
	if h := page.Find(row, page.Sel{Tag: "h1", Class: "h4"}); h != nil {
		if repo := page.Find(h, page.Sel{Tag: "a", Attr: "href"}); repo != nil {
			if id := hrefPath(page.Attr(repo, "href")); strings.Count(id, "/") == 1 {
				a.PinnedRepos = append(a.PinnedRepos, id)
			}
		}
	}
	return a, true
}

// --- topic pages ---

// TopicPage reads one topic. The search result for a topic carries the name and
// a short blurb; the page carries the long description, the logo, who created
// the thing, when it was released, the Wikipedia link, and the related topics,
// which is most of what makes a topic worth having a record for.
func (c *Client) TopicPage(ctx context.Context, slug string) (*Topic, error) {
	slug = strings.Trim(slug, "/")
	if slug == "" || strings.Contains(slug, "/") {
		return nil, usageBadID("topic", slug, "a topic slug")
	}
	res, err := c.GetHTML(ctx, BaseURL+"/topics/"+slug)
	if err != nil {
		return nil, err
	}
	p := page.Extract(res.FinalURL, res.Body)
	doc := p.Doc()
	if doc == nil {
		return nil, structureChanged(slug)
	}
	t := &Topic{Name: slug}
	t.setIdentity(KindTopic, slug)
	t.addSource(res.FinalURL)
	t.GitHubURL = t.URL

	if h := page.Find(doc, page.Sel{Tag: "h1", Class: "h1"}); h != nil {
		t.DisplayName = page.Text(h)
	}
	if trigger := page.Find(doc, page.Sel{Tag: "topic-feeds-toast-trigger"}); trigger != nil {
		t.DisplayName = firstNonEmpty(page.Attr(trigger, "data-topic-display-name"), t.DisplayName)
	}
	if md := page.Find(doc, page.MarkdownBody); md != nil {
		t.DescriptionHTML = page.OuterHTML(md)
		t.Description = page.BlockText(md)
		// The page has one description where the search result has two. The
		// first paragraph is the same string the short one would be, so it is
		// filled from here rather than left empty for no reason.
		t.ShortDescription, _, _ = strings.Cut(t.Description, "\n")
	}
	if img := page.Find(doc, page.Sel{Tag: "img", Attr: "alt", AttrSuffix: " logo"}); img != nil {
		t.LogoURL = page.Attr(img, "src")
	}
	if w := page.Find(doc, page.TopicWikipedia); w != nil {
		t.WikipediaURL = page.Attr(w, "href")
	}
	t.CreatedBy = labelledText(doc, "Created by")
	t.Released = labelledText(doc, "Released")
	if n := page.Find(doc, page.Sel{Tag: "h2", Class: "h3"}); n != nil {
		// "Here are 89,195 public repositories matching this topic..."
		if count, _, ok := page.CountIn(strings.TrimPrefix(page.Text(n), "Here are ")); ok {
			t.AppliedCount = intp(count)
		}
	}
	for _, dd := range page.FindAll(doc, page.Sel{Tag: "dd"}) {
		if n, _, ok := page.ParseCompactCount(strings.TrimSuffix(page.Text(dd), " followers")); ok &&
			strings.HasSuffix(page.Text(dd), "followers") {
			t.StargazerCount = intp(n)
		}
	}
	t.Related = append(t.Related, relatedTopics(doc, slug)...)
	if t.DisplayName == "" && t.Description == "" {
		return nil, structureChanged(slug)
	}
	return t, nil
}

// relatedTopics reads the sidebar's related topics.
//
// They are not in a container. The heading and the links are siblings, and the
// same link class is on every topic chip of every repository in the result
// list below, so scoping by class alone pulls in a few hundred unrelated
// topics. The heading is the only boundary the markup gives, so the walk
// starts there and stops at the next heading.
func relatedTopics(doc *html.Node, slug string) []string {
	var head *html.Node
	for _, h := range page.FindAll(doc, page.Sel{Tag: "h2"}) {
		if page.Text(h) == "Related topics" {
			head = h
			break
		}
	}
	if head == nil {
		return nil
	}
	var out []string
	for n := head.NextSibling; n != nil; n = n.NextSibling {
		if n.Type == html.ElementNode && (n.Data == "h2" || n.Data == "h3") {
			break
		}
		for _, a := range page.FindAll(n, page.Sel{Tag: "a", Class: "topic-tag-link"}) {
			rel := strings.TrimPrefix(hrefPath(page.Attr(a, "href")), "topics/")
			if rel != "" && rel != slug && !contains(out, rel) {
				out = append(out, rel)
			}
		}
	}
	return out
}

// labelledText reads the value beside a muted label in the topic sidebar. The
// label is a span inside the paragraph and the value is the rest of it, which
// is the only structure the markup offers.
func labelledText(doc *html.Node, label string) string {
	for _, p := range page.FindAll(doc, page.Sel{Tag: "p"}) {
		span := page.Find(p, page.Sel{Tag: "span", Class: "color-fg-muted"})
		if span == nil || page.Text(span) != label {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(page.Text(p), label))
	}
	return ""
}

// --- fork networks ---

// Forks lists the public forks of a repository. The page is the only keyless
// source: the network graph route needs a session and the search index does not
// model the parent link.
func (c *Client) Forks(ctx context.Context, repo string, limit int, emit func(Repo) error) error {
	if _, _, ok := SplitRepo(repo); !ok {
		return usageBadID("repository", repo, "owner/name")
	}
	base := repoSubURL(repo, "forks")
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
			return nil, "", structureChanged(repo + " forks")
		}
		var out []Repo
		for _, row := range page.FindAll(doc, page.BoxRow) {
			f, ok := forkRow(row, repo, res.FinalURL)
			if ok {
				out = append(out, f)
			}
		}
		return out, railsNext(doc, token), nil
	}
	return paginate(ctx, limit, fetch, emit)
}

// forkRow reads one row of a fork list. The owner and the name are separate
// links, so the id is assembled rather than read off one href.
func forkRow(row *html.Node, parent, source string) (Repo, bool) {
	h := page.Find(row, page.Sel{Tag: "h2"})
	if h == nil {
		return Repo{}, false
	}
	var owner, name string
	for _, a := range page.FindAll(h, page.Sel{Tag: "a", Attr: "href"}) {
		p := hrefPath(page.Attr(a, "href"))
		switch {
		case owner == "" && !strings.Contains(p, "/"):
			owner = p
		case strings.Count(p, "/") == 1:
			owner, name, _ = SplitRepo(p)
		}
	}
	if owner == "" || name == "" {
		return Repo{}, false
	}
	id := owner + "/" + name
	r := Repo{Owner: owner, Name: name, IsFork: true, ForkOf: parent}
	r.setIdentity(KindRepo, id)
	r.addSource(source)
	for _, a := range page.FindAll(row, page.Sel{Tag: "a", Attr: "href"}) {
		n, _, ok := page.ParseCompactCount(page.Text(a))
		if !ok {
			continue
		}
		switch href := page.Attr(a, "href"); {
		case strings.HasSuffix(href, "/stargazers"):
			r.Stars = intp(n)
		case strings.HasSuffix(href, "/forks"):
			r.Forks = intp(n)
		}
	}
	if t := page.Find(row, page.RelTimeEl); t != nil {
		r.PushedAt = parseTime(page.Attr(t, "datetime"))
	}
	return r, true
}

// --- statistics ---

// Contributors reads the contributor graph's own data route.
//
// The route answers 202 with an empty body while GitHub computes the numbers,
// which is normal rather than an error and is why this polls. A large
// repository takes a few seconds the first time and is instant afterwards.
func (c *Client) Contributors(ctx context.Context, repo string, opts ContributorOptions, emit func(Contributor) error) error {
	if _, _, ok := SplitRepo(repo); !ok {
		return usageBadID("repository", repo, "owner/name")
	}
	u := repoSubURL(repo, "graphs/contributors-data")
	res, err := c.Poll(ctx, u, SurfaceXHR)
	if err != nil {
		return err
	}
	var raw []contributorData
	if err := json.Unmarshal(res.Body, &raw); err != nil {
		return badPayload(shortURL(u), err)
	}
	if len(raw) == 0 {
		return structureChanged(repo + " contributors")
	}
	// The route answers in ascending order of contribution, which is the
	// reverse of what anyone asking for contributors wants.
	seen := 0
	for i := len(raw) - 1; i >= 0; i-- {
		rec := raw[i].contributor(repo, res.FinalURL, opts.Weeks)
		if err := emit(rec); err != nil {
			return err
		}
		seen++
		if opts.Limit > 0 && seen >= opts.Limit {
			return nil
		}
	}
	return nil
}

// ContributorOptions is what to do with the week array.
type ContributorOptions struct {
	// Weeks keeps the per-week breakdown. It is off by default because the
	// route sends every week since the repository began for every contributor,
	// which on an old project is a few hundred entries each and megabytes of
	// mostly zeroes for an answer whose question was "who wrote this".
	Weeks bool
	Limit int
}

type contributorData struct {
	Author *struct {
		ID     int    `json:"id"`
		Login  string `json:"login"`
		Avatar string `json:"avatar"`
		Path   string `json:"path"`
	} `json:"author"`
	Total int `json:"total"`
	Weeks []struct {
		W int64 `json:"w"`
		A int   `json:"a"`
		D int   `json:"d"`
		C int   `json:"c"`
	} `json:"weeks"`
}

// contributor folds the week array into the record. The array is six hundred
// entries for an old repository and nearly all of them are zero, so the
// summable fields are summed here and the first and last weeks with any
// activity are kept as dates, which is what a table can show.
func (d contributorData) contributor(repo, source string, keepWeeks bool) Contributor {
	rec := Contributor{Repo: repo, Commits: intp(d.Total)}
	if d.Author != nil {
		rec.Login = d.Author.Login
		rec.AvatarURL = d.Author.Avatar
		if d.Author.ID > 0 {
			rec.DatabaseID = intp(d.Author.ID)
		}
	}
	rec.setIdentity(KindContributor, repo+"@"+rec.Login)
	rec.addSource(source)

	adds, dels := 0, 0
	for _, w := range d.Weeks {
		adds += w.A
		dels += w.D
		if w.A == 0 && w.D == 0 && w.C == 0 {
			continue
		}
		at := time.Unix(w.W, 0).UTC()
		if rec.FirstWeek == nil {
			first := at
			rec.FirstWeek = &first
		}
		last := at
		rec.LastWeek = &last
		if keepWeeks {
			rec.Weeks = append(rec.Weeks, ContributorWeek{Week: at, Additions: w.A, Deletions: w.D, Commits: w.C})
		}
	}
	rec.Additions = intp(adds)
	rec.Deletions = intp(dels)
	return rec
}

// Languages reports the language histogram as one record per language. The
// numbers are on the repository record already; this exists because "what is
// this written in, in what proportion" is a question worth one command rather
// than a field selector on another one.
// Languages reports the language breakdown, largest first.
//
// This reads the sidebar fragment rather than a whole repository page, because
// the fragment is where the numbers are and it is 3 KB where the page is 300.
// The numbers are percentages: GitHub computes byte counts and publishes only
// the proportions, so a byte count is not something this can report honestly.
func (c *Client) Languages(ctx context.Context, repo string, emit func(LanguageShare) error) error {
	sb, err := c.sidebar(ctx, repo)
	if err != nil {
		return err
	}
	langs := sb.langs()
	if len(langs) == 0 {
		return structureChanged(repo + " languages")
	}
	source := repoSubURL(repo, "_sidebar")
	for _, l := range langs {
		share := LanguageShare{
			Repo:     repo,
			Language: l.Name,
			Percent:  l.Percentage,
			Color:    l.Color,
		}
		share.setIdentity(KindRepo, repo)
		share.addSource(source)
		if err := emit(share); err != nil {
			return err
		}
	}
	return nil
}

// Stats is the counts in one record. Everything in it is already on the
// repository record; the point is a record with nothing else in it, so
// `github stats x -o json` is a thing you can diff week to week.
func (c *Client) Stats(ctx context.Context, repo string) (*RepoStats, error) {
	// Deep, because the contributor and dependent counts are behind their own
	// fragments and a counts record missing two of the counts is not worth
	// having.
	r, err := c.Repo(ctx, repo, RepoOptions{Deep: true})
	if err != nil {
		return nil, err
	}
	s := &RepoStats{
		Repo:         repo,
		Stars:        r.Stars,
		Forks:        r.Forks,
		Watchers:     r.Watchers,
		OpenIssues:   r.OpenIssues,
		Commits:      r.CommitCount,
		Releases:     r.ReleaseCount,
		Tags:         r.TagCount,
		Contributors: r.ContributorCount,
		Dependents:   r.DependentCount,
		PushedAt:     r.PushedAt,
	}
	s.setIdentity(KindRepo, repo)
	s.addSource(r.Sources...)
	return s, nil
}
