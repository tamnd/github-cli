package gh

import (
	"context"
	"strings"

	"golang.org/x/net/html"

	"github.com/tamnd/github-cli/pkg/page"
)

// account.go reads user and organization profiles.
//
// Profiles are the one part of GitHub with no JSON payload at all. Asking for
// application/json on /{login} gets you the 2012 deprecation notice for the
// old v2 API, and the page carries no react-app.embeddedData block, so every
// field here comes from microdata, a microformat class, a stable data
// attribute, or a counted link. That makes accounts the most selector-dependent
// records in this tool, and the reason each field is read from the most
// meaningful hook available rather than the most convenient one.
//
// A user profile and an organization page are different templates, not one
// template with parts hidden. The user template has a vcard with microformats;
// the organization template has a pagehead with none. Which one arrived is how
// Type gets decided, rather than guessing from the login, which cannot be done:
// nothing about the string "golang" says organization.

// Account reads a user or organization profile. The concrete type is decided by
// the page, so a caller that does not know which it has can just call this.
func (c *Client) Account(ctx context.Context, login string) (*Account, error) {
	org, err := c.readProfile(ctx, login)
	if err != nil {
		return nil, err
	}
	return &org.Account, nil
}

// Org reads an organization. It is an error to point this at a user, because a
// caller that asked for an organization and silently got a user back will not
// notice until something downstream is confusing.
func (c *Client) Org(ctx context.Context, login string) (*Org, error) {
	org, err := c.readProfile(ctx, login)
	if err != nil {
		return nil, err
	}
	if org.Type != "Organization" {
		return nil, usageBadID("organization", login, "an organization login (this one is a user)")
	}
	return org, nil
}

// readProfile does the work for both. It always returns an Org, and the
// organization-only fields stay empty for a user, which is cheaper than two
// near-identical decoders that drift apart.
func (c *Client) readProfile(ctx context.Context, login string) (*Org, error) {
	if login == "" || strings.Contains(login, "/") {
		return nil, usageBadID("account", login, "a bare login")
	}
	res, err := c.GetHTML(ctx, accountURL(login))
	if err != nil {
		return nil, err
	}
	p := page.Extract(res.FinalURL, res.Body)
	doc := p.Doc()
	if doc == nil {
		return nil, structureChanged(login)
	}

	o := &Org{}
	o.Login = login
	o.addSource(res.FinalURL)
	if p.Canonical != "" {
		o.URL = p.Canonical
	}
	o.SocialImageURL = p.MetaContent("og:image")

	switch {
	case page.Find(doc, page.OrgHead) != nil:
		o.Type = "Organization"
		o.setIdentity(KindOrg, login)
		o.readOrgHead(doc)
	case page.Find(doc, page.ProfileNames) != nil:
		o.Type = "User"
		o.setIdentity(KindUser, login)
		o.readVCard(doc)
	default:
		return nil, structureChanged(login)
	}

	// Shared between the two templates.
	o.readTabCounts(doc)
	o.readAvatar(doc)
	o.readPinned(doc)
	o.readOrganizations(doc)
	o.readAchievements(doc)
	o.readReadme(doc)
	return o, nil
}

// --- the user template ---

func (o *Org) readVCard(doc *html.Node) {
	if n := page.Find(doc, page.ProfileFullName); n != nil {
		o.Name = page.Text(n)
	}
	// The microdata says the same two things independently. When they
	// disagree the page changed, and that is worth recording rather than
	// quietly picking one.
	if v := firstProp(doc, "name"); v != "" {
		if o.Name == "" {
			o.Name = v
		} else if v != o.Name {
			recordConflicts(&o.Base, map[string][]string{"name": {o.Name, v}})
		}
	}
	if n := page.Find(doc, page.ProfileNickname); n != nil {
		if got := page.Text(n); got != "" && got != o.Login {
			// The login in the URL and the login on the page disagree, which
			// happens on a rename before the redirect settles.
			recordConflicts(&o.Base, map[string][]string{"login": {o.Login, got}})
		}
	}

	// The bio is in a data attribute as well as in the element text. The
	// attribute is the source form, before GitHub's link and emoji rewriting,
	// so it is the one that gets kept.
	if n := page.Find(doc, page.ProfileBio); n != nil {
		o.Bio = strings.TrimSpace(page.Attr(n, "data-bio-text"))
		if o.Bio == "" {
			o.Bio = page.Text(n)
		}
	}

	// One list, five kinds of row, each labelled by its itemprop. Reading the
	// label rather than the icon means a new row type shows up in Extra
	// instead of being silently mapped onto the wrong field.
	for _, li := range page.FindAll(doc, page.ProfileDetail) {
		switch page.Attr(li, "itemprop") {
		case "worksFor":
			if n := page.Find(li, page.ProfileVCardOrg); n != nil {
				o.Company = page.Text(n)
			}
		case "homeLocation":
			if n := page.Find(li, page.ProfileVCardLabel); n != nil {
				o.Location = page.Text(n)
			}
		case "url":
			if a := page.Find(li, page.ProfileDetailURL); a != nil {
				o.Website = page.Attr(a, "href")
			}
		case "social":
			if a := page.Find(li, page.ProfileAnyLink); a != nil {
				o.SocialLinks = append(o.SocialLinks, page.Attr(a, "href"))
			}
		case "email":
			if a := page.Find(li, page.ProfileAnyLink); a != nil {
				o.Email = strings.TrimPrefix(page.Attr(a, "href"), "mailto:")
			}
		}
	}
	if o.Company == "" {
		o.Company = firstProp(doc, "worksFor")
	}
	if o.Location == "" {
		o.Location = firstProp(doc, "homeLocation")
	}

	o.Followers, o.FollowersDisplay = countIn(doc, page.ProfileFollowers)
	o.Following, _ = countIn(doc, page.ProfileFollowing)
}

// --- the organization template ---

func (o *Org) readOrgHead(doc *html.Node) {
	head := page.Find(doc, page.OrgHead)
	if head == nil {
		return
	}
	if n := page.Find(head, page.OrgHeading); n != nil {
		o.Name = page.Text(n)
	}
	// The description is the first muted block after the heading. There is no
	// itemprop and no class of its own, so it is found positionally, which is
	// the weakest kind of hook and is why an empty description here is not
	// treated as meaning the organization has none.
	if d := orgDescription(head); d != "" {
		o.Bio = d
	}
	if a := page.Find(head, page.OrgWebsite); a != nil {
		o.Website = firstNonEmpty(page.Attr(a, "title"), page.Attr(a, "href"))
	}
	if li := page.Find(head, page.OrgLocationRow); li != nil {
		o.Location = page.Text(li)
	}
	if li := page.Find(head, page.OrgEmailRow); li != nil {
		o.Email = strings.TrimSpace(page.Text(li))
	}
	o.Followers, o.FollowersDisplay = countIn(head, page.OrgFollowers)

	// The members strip is a sample, not the roster, so it fills the member
	// list but never the count. /orgs/{login}/people is the count, and it is a
	// separate request that `github members` makes.
	for _, a := range page.FindAll(doc, page.OrgMemberLink) {
		if login := loginFromHref(page.Attr(a, "href")); login != "" {
			o.Members = appendUnique(o.Members, login)
		}
	}
}

// orgDescription walks the heading's siblings for the muted block holding the
// tagline.
func orgDescription(head *html.Node) string {
	h := page.Find(head, page.OrgHeading)
	if h == nil {
		return ""
	}
	for n := h.NextSibling; n != nil; n = n.NextSibling {
		if n.Type != html.ElementNode {
			continue
		}
		if page.HasClass(n, "color-fg-muted") {
			return page.Text(n)
		}
	}
	return ""
}

// --- shared between both templates ---

// readTabCounts reads the navigation. Each tab carries its name in a data
// attribute and its size in a Counter span, which is the only place the
// repository, project, package, and star counts appear on either template.
//
// On an organization the counters are always empty, and GitHub says why in the
// markup: the span carries title="Not available". Those counts are behind a
// session and no amount of selector work will produce them, so an organization
// record leaves them nil rather than inventing a zero. The names differ too
// (org-header-repositories-tab against repositories), which is why the switch
// normalises before matching.
func (o *Org) readTabCounts(doc *html.Node) {
	for _, tab := range page.FindAll(doc, page.ProfileTabItem) {
		c := page.Find(tab, page.ProfileTabCounter)
		if c == nil {
			continue
		}
		n, _, ok := page.ParseCompactCount(page.Text(c))
		if !ok {
			continue
		}
		name := page.Attr(tab, "data-tab-item")
		name = strings.TrimSuffix(strings.TrimPrefix(name, "org-header-"), "-tab")
		switch name {
		case "repositories":
			o.RepoCount = intp(n)
		case "stars":
			o.Starred = intp(n)
		case "packages":
			o.PackageCount = intp(n)
		case "projects":
			o.ProjectCount = intp(n)
		case "sponsoring":
			o.SponsoringCount = intp(n)
			o.Sponsorable = true
		case "people", "members":
			o.MemberCount = intp(n)
		}
	}
}

// readAvatar takes the numeric account id out of the avatar URL. It is the only
// place on either template where the database id is stated plainly, and it is
// worth having because it is what joins a profile to the ids search returns.
func (o *Org) readAvatar(doc *html.Node) {
	img := page.Find(doc, page.OrgAvatarImg)
	if img == nil {
		img = page.Find(doc, page.Sel{Tag: "img", Class: "avatar-user"})
	}
	if img == nil {
		return
	}
	src := page.Attr(img, "src")
	o.AvatarURL = src
	if id, ok := avatarID(src); ok {
		o.DatabaseID = intp(id)
	}
}

func (o *Org) readPinned(doc *html.Node) {
	for _, li := range page.FindAll(doc, page.ProfilePinnedItem) {
		for _, a := range page.FindAll(li, page.ProfileAnyLink) {
			if id, ok := repoFromHref(page.Attr(a, "href")); ok {
				o.PinnedRepos = appendUnique(o.PinnedRepos, id)
				break
			}
		}
	}
}

func (o *Org) readOrganizations(doc *html.Node) {
	for _, a := range page.FindAll(doc, page.ProfileOrgAvatar) {
		if login := loginFromHref(page.Attr(a, "href")); login != "" {
			o.Organizations = appendUnique(o.Organizations, login)
		}
	}
}

// readAchievements records the slug from the href rather than the label from
// the image, because the slug is stable and the label is a display string.
func (o *Org) readAchievements(doc *html.Node) {
	for _, a := range page.FindAll(doc, page.ProfileAchievement) {
		href := page.Attr(a, "href")
		i := strings.Index(href, "achievement=")
		if i < 0 {
			continue
		}
		slug := href[i+len("achievement="):]
		if j := strings.IndexAny(slug, "&#"); j >= 0 {
			slug = slug[:j]
		}
		if slug != "" {
			o.Achievements = appendUnique(o.Achievements, slug)
		}
	}
}

// readReadme keeps both forms. The HTML is what GitHub rendered and the text is
// what it says, and a knowledge-graph consumer wants the second while a viewer
// wants the first.
func (o *Org) readReadme(doc *html.Node) {
	n := page.Find(doc, page.ProfileReadme)
	if n == nil {
		return
	}
	body := page.Find(n, page.MarkdownBody)
	if body == nil {
		body = n
	}
	o.ReadmeHTML = page.OuterHTML(body)
	o.ReadmeText = page.Text(body)
}

// --- small shared helpers ---

// countIn reads a "12.4k followers" style link: the number is in a bold span
// inside the anchor and is compact, so both the parsed value and the string as
// printed are kept.
func countIn(root *html.Node, sel page.Sel) (*int, string) {
	a := page.Find(root, sel)
	if a == nil {
		return nil, ""
	}
	if b := page.Find(a, page.ProfileBoldCount); b != nil {
		if n, display, ok := page.ParseCompactCount(page.Text(b)); ok {
			return intp(n), display
		}
	}
	if n, display, ok := page.CountIn(page.Text(a)); ok {
		return intp(n), display
	}
	return nil, ""
}

// firstProp reads a microdata property off the page-level index.
func firstProp(doc *html.Node, name string) string {
	n := page.Find(doc, page.Sel{Attr: "itemprop", AttrValue: name})
	if n == nil {
		return ""
	}
	if v := page.Attr(n, "content"); v != "" {
		return v
	}
	return page.Text(n)
}

// loginFromHref turns "/torvalds" into "torvalds" and rejects anything with
// more structure, which is what keeps a /torvalds/linux link out of a list of
// people.
func loginFromHref(href string) string {
	s := strings.TrimPrefix(href, BaseURL)
	s = strings.TrimPrefix(s, "/")
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if s == "" || strings.Contains(s, "/") {
		return ""
	}
	return s
}

// repoFromHref turns "/owner/name" into "owner/name" and rejects the rest.
func repoFromHref(href string) (string, bool) {
	s := strings.TrimPrefix(href, BaseURL)
	s = strings.TrimPrefix(s, "/")
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return s, true
}

// avatarID pulls the numeric account id out of an avatar URL, which looks like
// https://avatars.githubusercontent.com/u/1024025?v=4.
func avatarID(src string) (int, bool) {
	i := strings.Index(src, "/u/")
	if i < 0 {
		return 0, false
	}
	rest := src[i+3:]
	if j := strings.IndexAny(rest, "?/&"); j >= 0 {
		rest = rest[:j]
	}
	n, _, ok := page.ParseCompactCount(rest)
	if !ok {
		return 0, false
	}
	return n, true
}

func appendUnique(ss []string, s string) []string {
	for _, existing := range ss {
		if existing == s {
			return ss
		}
	}
	return append(ss, s)
}
