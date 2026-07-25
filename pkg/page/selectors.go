package page

// selectors.go is every CSS-shaped selector this tool uses, in one file, each
// with the page it came from and the date it was last checked against a live
// fetch. A selector change is then a one-file diff instead of an archaeology
// project across a dozen decoders.
//
// The order of preference for reading a field is: JSON payload, JSON-LD,
// microdata, microformats, stable data attributes, class selectors, text.
// Everything in this file is in the bottom three tiers by definition, so
// everything in this file is the fallback for something.
//
// Every one of these degrades to a missing field, never to a wrong one. A
// selector that stops matching produces an absent value, and absent is a
// truthful answer.

// --- repository page, /{owner}/{repo} ---

// LicenseLink is the About-sidebar licence link. There is no licence field in
// any JSON payload on any keyless surface, so this anchor is the only source.
// It is the most fragile selector in the tool: it is identified by the
// octicon-law SVG inside it rather than by a class on the anchor, because the
// anchor's classes change more often than the icon does.
// Verified 2026-07-25 against gohugoio/hugo.
var LicenseLink = Sel{Tag: "a", HasDescendantClass: "octicon-law"}

// LanguageBarItem is one segment of the coloured language bar under the About
// box. Used only when sections.languages arrives empty, which is the common
// case on a cold page.
// Verified 2026-07-25 against gohugoio/hugo.
var LanguageBarItem = Sel{Tag: "a", Attr: "href", AttrContains: "/search?l="}

// LanguageBarName is the bold span holding just the language name, so the
// percentage that follows it in the anchor text can be told apart from it
// without splitting on whitespace and hoping.
// Verified 2026-07-25 against gohugoio/hugo.
var LanguageBarName = Sel{Tag: "span", Class: "text-bold"}

// --- profile pages, /{login} ---

// ProfileBio is the bio, which is carried in a data attribute as well as in
// the element text. The attribute is the one to read: it is the source form,
// before GitHub's own link and emoji rewriting.
// Verified 2026-07-25 against torvalds.
var ProfileBio = Sel{Class: "user-profile-bio", Attr: "data-bio-text"}

// ProfileNames anchors the display-name block when the microdata hooks change.
// The container carries js-profile-editable-names, which was confirmed present.
// Verified 2026-07-25 against torvalds.
var ProfileNames = Sel{Class: "vcard-names-container"}

// The microformat pair inside vcard-names. p-name is the display name and
// p-nickname is the login, and they carry the same two facts the microdata
// carries independently, which is why both are read: a disagreement between
// them is recorded as a conflict rather than resolved, because it would mean
// the page changed under us.
// Verified 2026-07-25 against torvalds and sindresorhus.
var (
	ProfileFullName = Sel{Class: "p-name"}
	ProfileNickname = Sel{Class: "p-nickname"}
	ProfileVCardOrg = Sel{Class: "p-org"}
	// p-label holds the location text inside the homeLocation list item.
	ProfileVCardLabel = Sel{Class: "p-label"}
)

// ProfileDetail is one row of the vcard details list. Each row carries an
// itemprop naming what it holds (worksFor, homeLocation, url, social, email),
// so one selector plus the itemprop covers all five instead of five selectors
// that each break separately.
// Verified 2026-07-25 against sindresorhus, which has all of them but email.
var (
	ProfileDetail    = Sel{Tag: "li", Class: "vcard-detail"}
	ProfileDetailURL = Sel{Tag: "a", Attr: "rel", AttrContains: "me"}
	ProfileAnyLink   = Sel{Tag: "a", Attr: "href"}
)

// The tab links carrying the follower and following counts. The count is in a
// bold span inside the anchor and is a compact string like "313k", which is
// what ParseCompactCount is for. The href is absolute on a profile and relative
// on an organization, so these match on a suffix.
// Verified 2026-07-25 against torvalds and sindresorhus.
var (
	ProfileFollowers = Sel{Tag: "a", Attr: "href", AttrSuffix: "?tab=followers"}
	ProfileFollowing = Sel{Tag: "a", Attr: "href", AttrSuffix: "?tab=following"}
	ProfileStars     = Sel{Tag: "a", Attr: "href", AttrSuffix: "?tab=stars"}
	ProfileBoldCount = Sel{Tag: "span", Class: "text-bold"}
)

// ProfileTabItem is one entry of the profile navigation. Each carries a
// data-tab-item naming the tab and, when the tab is non-empty, a Counter span
// with the number in it. That is where the repository, project, package, star,
// and sponsoring counts come from.
// Verified 2026-07-25 against sindresorhus.
var (
	ProfileTabItem    = Sel{Attr: "data-tab-item"}
	ProfileTabCounter = Sel{Class: "Counter"}
)

// ProfileAchievement is one achievement badge. The label lives in the img alt
// as "Achievement: Pair Extraordinaire", and the slug lives in the href query,
// so the slug is what gets recorded.
// Verified 2026-07-25 against torvalds.
var ProfileAchievement = Sel{Tag: "a", Attr: "href", AttrContains: "achievement="}

// ProfilePinnedItem is one pinned repository. The repository link inside it is
// the plain anchor whose href has two path segments.
// Verified 2026-07-25 against sindresorhus.
var ProfilePinnedItem = Sel{Tag: "li", Class: "js-pinned-item-list-item"}

// --- organization pages, /{login} ---

// The organization page is a different template from a user profile: no vcard,
// no microformats, one pagehead block instead. These five are the whole of it.
// Verified 2026-07-25 against golang.
var (
	OrgHead       = Sel{Tag: "header", Class: "orghead"}
	OrgHeading    = Sel{Tag: "h1", Class: "h2"}
	OrgAvatarImg  = Sel{Tag: "img", Attr: "itemprop", AttrValue: "image"}
	OrgWebsite    = Sel{Tag: "a", Attr: "itemprop", AttrValue: "url"}
	OrgFollowers  = Sel{Tag: "a", Attr: "href", AttrContains: "/followers"}
	OrgMemberLink = Sel{Tag: "a", Class: "member-avatar"}
	// The location and email rows have no class or itemprop of their own, so
	// they are found by the icon inside them, the same trick the repository
	// licence link needs.
	OrgLocationRow = Sel{Tag: "li", HasDescendantClass: "octicon-location"}
	OrgEmailRow    = Sel{Tag: "li", HasDescendantClass: "octicon-mail"}
)

// ProfilePinned is the pinned-repository list, and ProfileOrgAvatar is the
// organization strip. The hovercard type attribute is the reliable hook on
// both: it is what the front end uses to decide which popover to fetch, so it
// is load-bearing for GitHub too and does not drift casually.
// Verified 2026-07-25 against torvalds.
var (
	ProfilePinnedList = Sel{Tag: "ol", Class: "js-pinned-items-reorder-list"}
	ProfileOrgAvatar  = Sel{Tag: "a", Attr: "data-hovercard-type", AttrValue: "organization"}
	ProfileUserLink   = Sel{Tag: "a", Attr: "data-hovercard-type", AttrValue: "user"}
	ProfileReadme     = Sel{Class: "js-profile-readme"}
	ProfileVCardList  = Sel{Class: "vcard-details"}
	ProfileAchieve    = Sel{Class: "js-profile-achievements"}
)

// --- discussion pages, /{owner}/{repo}/discussions/{n} ---

// Discussions are the one thread type with no React payload at all: the page is
// Rails, and the only structured block on it is a schema.org QAPage. So the
// fields that block does not carry (category, labels, participants, the node
// id) come from these, and everything here is checked against the JSON-LD where
// the two overlap.
// Verified 2026-07-25 against orgs/community#1 and google/docsy-example#479.
var (
	// The sidebar carries data-gid (the base64 node id) and data-url, which is
	// the canonical /{owner}/{repo}/discussions/{n}/sidebar path and therefore
	// the authority on which repository an /orgs/ URL belongs to.
	DiscussionSidebar = Sel{Attr: "id", AttrValue: "partial-discussion-sidebar"}
	DiscussionTitle   = Sel{Class: "js-issue-title"}
	DiscussionNumber  = Sel{Class: "gh-header-number"}
	// Every status pill in the header is a span.State. Which one it is comes
	// from the title attribute: "Status: Closed as resolved", "Answered".
	DiscussionState    = Sel{Tag: "span", Class: "State", Attr: "title"}
	DiscussionCategory = Sel{Tag: "a", Attr: "href", AttrContains: "/discussions/categories/"}
	// Labels carry the name in data-name, which is the raw name before the
	// truncation span gets at it.
	DiscussionLabel       = Sel{Tag: "a", Class: "IssueLabel", Attr: "data-name"}
	DiscussionUpvote      = Sel{Class: "js-upvote-button", Attr: "aria-label"}
	DiscussionComment     = Sel{Class: "js-comment-container", Attr: "data-gid"}
	DiscussionBody        = Sel{Class: "js-comment-body"}
	DiscussionAuthor      = Sel{Tag: "a", Class: "author"}
	DiscussionParticipant = Sel{Tag: "a", Class: "participant-avatar"}
	DiscussionAnswerLink  = Sel{Class: "js-discussions-goto-answer-button"}
)

// --- hovercards, /{owner}/{repo}/pull/{n}/hovercard ---

// The hovercard is a small HTML fragment served to XHR requests. It is the only
// keyless source for a pull request's body: the conversation route ships merge
// metadata and nothing else. The body it gives is truncated to about a line and
// a half, which is why the record calls it a snippet.
// Verified 2026-07-25 against cli/cli#9000.
var (
	HovercardBody  = Sel{Class: "markdown-body-short"}
	HovercardTitle = Sel{Class: "markdown-title"}
	HovercardState = Sel{Tag: "span", Class: "State", Attr: "title"}
	HovercardLabel = Sel{Class: "IssueLabel", Attr: "data-name"}
	HovercardRef   = Sel{Class: "commit-ref"}
)

// --- shared ---

// RelTimeEl is the <relative-time> custom element. Its datetime attribute is an
// ISO timestamp; its text is "3 days ago" and is never parsed.
var RelTimeEl = Sel{Tag: "relative-time", Attr: "datetime"}

// --- repositories tab, /{login}?tab=repositories ---

// RepoListItem is one row. The anchor inside carries
// itemprop="name codeRepository", which is the microdata hook and the reason
// this parser is anchored on meaning rather than on layout.
// Verified 2026-07-25 against torvalds?tab=repositories, 12 rows.
var (
	RepoListItem  = Sel{Tag: "li", Attr: "itemprop", AttrValue: "owns"}
	RepoNameLink  = Sel{Tag: "a", Attr: "itemprop", AttrValue: "name codeRepository"}
	RepoListDesc  = Sel{Attr: "itemprop", AttrValue: "description"}
	RepoListLang  = Sel{Attr: "itemprop", AttrValue: "programmingLanguage"}
	RepoLangColor = Sel{Class: "repo-language-color"}
	RepoStarsLink = Sel{Tag: "a", Attr: "href", AttrSuffix: "/stargazers"}
	RepoForksLink = Sel{Tag: "a", Attr: "href", AttrSuffix: "/forks"}
	RepoTopicTag  = Sel{Tag: "a", Class: "topic-tag"}
	RepoLabel     = Sel{Class: "Label"}
)

// --- trending, /trending ---

// TrendingRow is one repository card. The page is Rails and is the only source
// anywhere for the trending list: there is no JSON equivalent, tokened or not.
// Verified 2026-07-25 against /trending.
var (
	TrendingRow      = Sel{Tag: "article", Class: "Box-row"}
	TrendingHeading  = Sel{Tag: "h2"}
	TrendingDesc     = Sel{Tag: "p"}
	TrendingPeriod   = Sel{Class: "float-sm-right"}
	TrendingBuiltBy  = Sel{Tag: "img", Class: "avatar-user"}
	TrendingDevRow   = Sel{Class: "Box-row"}
	TrendingDevName  = Sel{Tag: "h1", Class: "h3"}
	TrendingDevRepo  = Sel{Tag: "h1", Class: "h4"}
	TrendingSponsors = Sel{Tag: "a", Attr: "href", AttrPrefix: "/sponsors/"}
)

// --- topic page, /topics/{slug} ---

// The topic page carries metadata the search result does not: the long
// description, the logo, the creator, the release date, the Wikipedia link,
// and the aliases.
// Verified 2026-07-25 against /topics/go.
var (
	TopicHeading   = Sel{Tag: "h1"}
	TopicShortDesc = Sel{Tag: "p", Class: "f4"}
	TopicMarkdown  = Sel{Class: "markdown-body"}
	TopicLogo      = Sel{Tag: "img", Class: "rounded-2"}
	TopicWikipedia = Sel{Tag: "a", Attr: "href", AttrPrefix: "https://en.wikipedia.org"}
	TopicRepoRow   = Sel{Tag: "article", Class: "border"}
)

// --- releases, /{owner}/{repo}/releases/tag/{tag} ---

// The release list page is lazy: a cold fetch of /releases carries no
// /releases/tag/ links at all, which is why the list comes from releases.atom
// and only the per-release page is parsed here. Download counts exist nowhere
// else on a keyless surface, which is what makes the extra request worth it.
// Verified 2026-07-25 against gohugoio/hugo.
var (
	ReleaseBox       = Sel{Class: "Box"}
	ReleaseMarkdown  = Sel{Class: "markdown-body"}
	ReleaseAssetRow  = Sel{Class: "Box-row"}
	ReleaseTagIcon   = Sel{Class: "octicon-tag"}
	ReleaseLabel     = Sel{Class: "Label"}
	ReleaseAssetLink = Sel{Tag: "a", Attr: "href", AttrPrefix: "/"}
)

// --- gist, gist.github.com/{id} ---

// Verified 2026-07-25 against gist.github.com.
var (
	GistFileBox    = Sel{Class: "file"}
	GistFileHeader = Sel{Class: "file-header", Attr: "data-path"}
	GistBlobWrap   = Sel{Class: "blob-wrapper"}
)

// --- shared ---

// BoxRow is the generic Rails list row. It appears on the org people page, the
// tags page, the release assets list, and half a dozen others, which is why it
// is here once rather than in six decoders.
// Verified 2026-07-25.
var (
	BoxRow       = Sel{Class: "Box-row"}
	MarkdownBody = Sel{Class: "markdown-body"}
	Pagination   = Sel{Class: "pagination"}
	NextPage     = Sel{Tag: "a", Attr: "rel", AttrValue: "next"}
)
