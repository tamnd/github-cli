package gh

import "time"

// types.go is the record model: twenty-five types that between them describe
// everything this tool can read off github.com without a token.
//
// Three conventions run through all of them.
//
// Counts are *int. nil means the surface did not carry it, 0 means the surface
// said zero. Collapsing those two is how aggregates end up quietly wrong.
//
// Timestamps are *time.Time and come from a datetime attribute or an ISO
// string. Rendered relative text ("3 days ago") is never parsed.
//
// Where a surface offers only a rendered count, the record carries both the
// parsed integer and the original string, because "8,112" and "8.112" are the
// same number in different locales and throwing away the original hides that.

// --- repository ---

// Repo is the centre of the model. A read from a page fills most of it, a read
// from a search result fills a thinner but honest subset, and Sources says
// which happened.
type Repo struct {
	Base

	Owner string `json:"owner" table:"owner"`
	Name  string `json:"name"  table:"name"`

	Description          string   `json:"description,omitempty"           table:"description,truncate"`
	DescriptionHighlight string   `json:"description_highlight,omitempty" table:"-"`
	Homepage             string   `json:"homepage,omitempty"              table:"-"`
	Topics               []string `json:"topics,omitempty"                table:"topics"`

	DatabaseID *int   `json:"database_id,omitempty" table:"-"`
	NodeID     string `json:"node_id,omitempty"     table:"-"`

	DefaultBranch string `json:"default_branch,omitempty" table:"branch"`
	HeadSHA       string `json:"head_sha,omitempty"       table:"-"`

	Language      string           `json:"language,omitempty"       table:"language"`
	LanguageColor string           `json:"language_color,omitempty" table:"-"`
	Languages     map[string]int64 `json:"languages,omitempty"      table:"-"`

	Stars        *int   `json:"stars,omitempty"         table:"stars"`
	StarsDisplay string `json:"stars_display,omitempty" table:"-"`
	Forks        *int   `json:"forks,omitempty"         table:"forks"`
	Watchers     *int   `json:"watchers,omitempty"      table:"watchers"`

	OpenIssues       *int `json:"open_issues,omitempty"        table:"issues"`
	GoodFirstIssues  *int `json:"good_first_issues,omitempty"  table:"-"`
	HelpWantedIssues *int `json:"help_wanted_issues,omitempty" table:"-"`

	CommitCount        *int   `json:"commit_count,omitempty"         table:"-"`
	CommitCountDisplay string `json:"commit_count_display,omitempty" table:"-"`
	ReleaseCount       *int   `json:"release_count,omitempty"        table:"-"`
	TagCount           *int   `json:"tag_count,omitempty"            table:"-"`
	FileCount          *int   `json:"file_count,omitempty"           table:"-"`
	DependentCount     *int   `json:"dependent_count,omitempty"      table:"-"`

	// License comes from one sidebar anchor and from nowhere else on any
	// keyless surface. See page.LicenseLink.
	License string `json:"license,omitempty" table:"license"`

	IsFork     bool   `json:"is_fork"              table:"-"`
	ForkOf     string `json:"fork_of,omitempty"    table:"-"`
	IsArchived bool   `json:"is_archived"          table:"-"`
	IsMirror   bool   `json:"is_mirror"            table:"-"`
	IsTemplate bool   `json:"is_template"          table:"-"`
	IsEmpty    bool   `json:"is_empty"             table:"-"`
	IsPrivate  bool   `json:"is_private"           table:"-"`
	IsOrgOwned bool   `json:"is_org_owned"         table:"-"`
	Visibility string `json:"visibility,omitempty" table:"-"`

	Sponsorable    bool `json:"sponsorable"     table:"-"`
	HasFunding     bool `json:"has_funding"     table:"-"`
	HasCitation    bool `json:"has_citation"    table:"-"`
	HasDiscussions bool `json:"has_discussions" table:"-"`
	HasWiki        bool `json:"has_wiki"        table:"-"`
	HasPages       bool `json:"has_pages"       table:"-"`

	CreatedAt *time.Time `json:"created_at,omitempty" table:"created,time"`
	PushedAt  *time.Time `json:"pushed_at,omitempty"  table:"pushed,time"`
	UpdatedAt *time.Time `json:"updated_at,omitempty" table:"updated,time"`

	OwnerAvatarURL string `json:"owner_avatar_url,omitempty" table:"-"`
	SocialImageURL string `json:"social_image_url,omitempty" table:"-"`

	ReadmePath string `json:"readme_path,omitempty" table:"-"`
	ReadmeHTML string `json:"readme_html,omitempty" table:"-"`
	ReadmeText string `json:"readme_text,omitempty" table:"-"`

	Tree []TreeEntry `json:"tree,omitempty" table:"-"`

	StargazersPath  string `json:"stargazers_path,omitempty"   table:"-"`
	ForkNetworkPath string `json:"fork_network_path,omitempty" table:"-"`
	ActivityPath    string `json:"activity_path,omitempty"     table:"-"`
}

// --- trees and files ---

// TreeEntry is one row of a directory listing. Size and SHA are absent from the
// tree route and cost one request each, which is what `--sizes` opts into.
type TreeEntry struct {
	Base

	Repo string `json:"repo" table:"-"`
	Ref  string `json:"ref"  table:"-"`

	Name string `json:"name" table:"name"`
	Path string `json:"path" table:"path"`
	// Type is contentType verbatim: file, directory, symlink_file,
	// symlink_directory, submodule.
	Type string `json:"type" table:"type"`

	Size *int64 `json:"size,omitempty" table:"size"`
	SHA  string `json:"sha,omitempty"  table:"-"`
}

// File is a blob. The interesting part is Symbols: GitHub runs a symbol
// extractor over every blob it renders and ships the result in the route
// payload, and there is no unauthenticated REST equivalent anywhere.
type File struct {
	Base

	Repo string `json:"repo" table:"-"`
	Ref  string `json:"ref"  table:"-"`
	Path string `json:"path" table:"path"`

	Size        *int64 `json:"size,omitempty"     table:"size"`
	Lines       *int   `json:"lines,omitempty"    table:"lines"`
	Language    string `json:"language,omitempty" table:"language"`
	IsBinary    bool   `json:"is_binary"          table:"-"`
	IsLFS       bool   `json:"is_lfs"             table:"-"`
	IsGenerated bool   `json:"is_generated"       table:"-"`

	RawURL string `json:"raw_url" table:"-"`

	Content  string   `json:"content,omitempty"   table:"-"`
	RawLines []string `json:"raw_lines,omitempty" table:"-"`
	RichText string   `json:"rich_text,omitempty" table:"-"`

	TOC     []Heading `json:"toc,omitempty"     table:"-"`
	Symbols []Symbol  `json:"symbols,omitempty" table:"-"`
	// SymbolsStatus is ok, timed_out, or not_analyzed. An empty symbol list
	// with not_analyzed means the language is unsupported, which is a different
	// fact from a file that genuinely has no symbols, and the caller should not
	// have to guess which one it got.
	SymbolsStatus string `json:"symbols_status,omitempty" table:"-"`
}

// Heading is one entry of a rendered markdown table of contents.
type Heading struct {
	Level  int    `json:"level"  table:"level"`
	Text   string `json:"text"   table:"text"`
	Anchor string `json:"anchor" table:"anchor"`
}

// Symbol is one extracted definition, with byte offsets into the blob.
type Symbol struct {
	Name               string `json:"name"          table:"name"`
	Kind               string `json:"kind"          table:"kind"`
	FullyQualifiedName string `json:"fqn,omitempty" table:"-"`
	IdentStart         int    `json:"ident_start"   table:"-"`
	IdentEnd           int    `json:"ident_end"     table:"-"`
	ExtentStart        int    `json:"extent_start"  table:"-"`
	ExtentEnd          int    `json:"extent_end"    table:"-"`
}

// --- accounts ---

// Account is a user or an organization. Profiles carry no JSON payload at all,
// so every field here comes from microdata, a microformat class, a stable data
// attribute, or a counted link. That makes accounts the most selector-dependent
// records in the tool, and the reason every field has a golden pinning it.
type Account struct {
	Base

	Login string `json:"login"          table:"login"`
	Name  string `json:"name,omitempty" table:"name"`
	// Type is User or Organization, decided by which blocks the page carries
	// rather than guessed from the login.
	Type string `json:"type" table:"type"`

	Bio         string   `json:"bio,omitempty"          table:"bio,truncate"`
	Company     string   `json:"company,omitempty"      table:"company"`
	Location    string   `json:"location,omitempty"     table:"location"`
	Website     string   `json:"website,omitempty"      table:"-"`
	Email       string   `json:"email,omitempty"        table:"-"`
	Pronouns    string   `json:"pronouns,omitempty"     table:"-"`
	SocialLinks []string `json:"social_links,omitempty" table:"-"`

	DatabaseID *int   `json:"database_id,omitempty" table:"-"`
	NodeID     string `json:"node_id,omitempty"     table:"-"`
	AvatarURL  string `json:"avatar_url,omitempty"  table:"-"`

	Followers        *int   `json:"followers,omitempty"         table:"followers"`
	FollowersDisplay string `json:"followers_display,omitempty" table:"-"`
	Following        *int   `json:"following,omitempty"         table:"following"`
	Starred          *int   `json:"starred,omitempty"           table:"-"`
	RepoCount        *int   `json:"repo_count,omitempty"        table:"repos"`

	CreatedAt *time.Time `json:"created_at,omitempty" table:"joined,time"`

	Sponsorable bool `json:"sponsorable" table:"-"`
	IsVerified  bool `json:"is_verified" table:"-"`
	IsHireable  bool `json:"is_hireable" table:"-"`

	ReadmeHTML string `json:"readme_html,omitempty" table:"-"`
	ReadmeText string `json:"readme_text,omitempty" table:"-"`

	PinnedRepos   []string `json:"pinned_repos,omitempty"  table:"-"`
	Organizations []string `json:"organizations,omitempty" table:"-"`
	Achievements  []string `json:"achievements,omitempty"  table:"-"`

	SocialImageURL string `json:"social_image_url,omitempty" table:"-"`
}

// Org is an Account plus the five things only an organization page has.
// TopLanguages and TopTopics are deferred fragments and arrive only with
// --deep.
type Org struct {
	Account

	VerifiedDomains []string       `json:"verified_domains,omitempty" table:"-"`
	MemberCount     *int           `json:"member_count,omitempty"     table:"members"`
	TopLanguages    map[string]int `json:"top_languages,omitempty"    table:"-"`
	TopTopics       []string       `json:"top_topics,omitempty"       table:"-"`
	IsEnterprise    bool           `json:"is_enterprise"              table:"-"`
}

// --- threads ---

// Thread is what issues, pull requests, and discussions have in common, which
// is most of it: their pages share a Relay payload shape.
type Thread struct {
	Base

	Repo   string `json:"repo"   table:"repo"`
	Number int    `json:"number" table:"number"`

	Title          string `json:"title"                     table:"title,truncate"`
	TitleHighlight string `json:"title_highlight,omitempty" table:"-"`
	TitleHTML      string `json:"title_html,omitempty"      table:"-"`

	State       string `json:"state"                  table:"state"`
	StateReason string `json:"state_reason,omitempty" table:"-"`

	Body     string `json:"body,omitempty"      table:"-"`
	BodyHTML string `json:"body_html,omitempty" table:"-"`

	Author Actor `json:"author" table:"author"`

	Labels    []Label    `json:"labels,omitempty"    table:"labels"`
	Milestone *Milestone `json:"milestone,omitempty" table:"-"`
	Assignees []Actor    `json:"assignees,omitempty" table:"-"`
	Reactions []Reaction `json:"reactions,omitempty" table:"-"`

	CommentCount *int `json:"comment_count,omitempty" table:"comments"`

	Locked   bool `json:"locked"    table:"-"`
	IsPinned bool `json:"is_pinned" table:"-"`

	CreatedAt *time.Time `json:"created_at,omitempty" table:"created,time"`
	UpdatedAt *time.Time `json:"updated_at,omitempty" table:"updated,time"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"  table:"-"`

	AuthorAssociation string `json:"author_association,omitempty" table:"-"`

	NodeID     string `json:"node_id,omitempty"     table:"-"`
	DatabaseID *int   `json:"database_id,omitempty" table:"-"`
}

// Issue adds the tracking relationships GitHub keeps between issues and the
// work that closes them.
type Issue struct {
	Thread

	IssueType     string   `json:"issue_type,omitempty"      table:"-"`
	SubIssueTotal *int     `json:"sub_issue_total,omitempty" table:"-"`
	SubIssueDone  *int     `json:"sub_issue_done,omitempty"  table:"-"`
	DuplicateOf   string   `json:"duplicate_of,omitempty"    table:"-"`
	LinkedPRs     []string `json:"linked_prs,omitempty"      table:"-"`
	ClosedByPRs   []string `json:"closed_by_prs,omitempty"   table:"-"`
	ProjectItems  []string `json:"project_items,omitempty"   table:"-"`
}

// PullRequest adds the diff and the merge state.
type PullRequest struct {
	Thread

	BaseRef string `json:"base_ref,omitempty" table:"base"`
	HeadRef string `json:"head_ref,omitempty" table:"head"`
	BaseOID string `json:"base_oid,omitempty" table:"-"`
	HeadOID string `json:"head_oid,omitempty" table:"-"`

	Merged    bool       `json:"merged"              table:"-"`
	MergedAt  *time.Time `json:"merged_at,omitempty" table:"merged,time"`
	MergedBy  *Actor     `json:"merged_by,omitempty" table:"-"`
	Mergeable string     `json:"mergeable,omitempty" table:"-"`
	IsDraft   bool       `json:"is_draft"            table:"-"`

	Additions *int `json:"additions,omitempty" table:"+"`
	// The render tag grammar uses "-" to mean "skip this column", and "-" is
	// also the natural header for deletions. The grammar wins: deletions is
	// hidden by default and shown with --fields deletions. This is deliberate,
	// please do not "fix" it.
	Deletions    *int `json:"deletions,omitempty"     table:"-"`
	ChangedFiles *int `json:"changed_files,omitempty" table:"files"`
	CommitCount  *int `json:"commit_count,omitempty"  table:"-"`

	ReviewDecision string   `json:"review_decision,omitempty" table:"review"`
	ReviewRequests []Actor  `json:"review_requests,omitempty" table:"-"`
	ClosesIssues   []string `json:"closes_issues,omitempty"   table:"-"`
}

// Discussion adds the answer, which is the thing discussions have that issues
// do not.
type Discussion struct {
	Thread

	Category       string     `json:"category,omitempty"         table:"category"`
	IsAnswered     bool       `json:"is_answered"                table:"answered"`
	AnswerChosenAt *time.Time `json:"answer_chosen_at,omitempty" table:"-"`
	AnswerAuthor   *Actor     `json:"answer_author,omitempty"    table:"-"`
	Upvotes        *int       `json:"upvotes,omitempty"          table:"upvotes"`
}

// Label is a thread label.
type Label struct {
	Name        string `json:"name"                  table:"name"`
	Color       string `json:"color,omitempty"       table:"color"`
	Description string `json:"description,omitempty" table:"description,truncate"`
	URL         string `json:"url,omitempty"         table:"url,url"`
	NodeID      string `json:"node_id,omitempty"     table:"-"`
}

// Milestone is a thread milestone.
type Milestone struct {
	Title    string     `json:"title"               table:"title"`
	Number   *int       `json:"number,omitempty"    table:"number"`
	Closed   bool       `json:"closed"              table:"closed"`
	DueOn    *time.Time `json:"due_on,omitempty"    table:"due,time"`
	ClosedAt *time.Time `json:"closed_at,omitempty" table:"-"`
	Progress *float64   `json:"progress,omitempty"  table:"progress"`
	URL      string     `json:"url,omitempty"       table:"url,url"`
}

// Reaction is one emoji group. Content is the GraphQL enum: THUMBS_UP,
// THUMBS_DOWN, LAUGH, HOORAY, CONFUSED, HEART, ROCKET, EYES. All eight always
// arrive, most with a zero count, and the decoder drops the zeroes so an
// unreacted thread has an empty list rather than eight noisy nothings.
type Reaction struct {
	Content string `json:"content" table:"content"`
	Count   int    `json:"count"   table:"count"`
}

// TimelineItem is one event on a thread. Type is the GraphQL __typename,
// lower-snake-cased.
//
// An unrecognised typename does not get dropped: Type is set, the common fields
// are filled, the whole node goes into Extra, and the fixture suite fails and
// names it. That is the entire strategy for union drift.
type TimelineItem struct {
	Base

	Thread string `json:"thread"           table:"-"`
	Type   string `json:"type"             table:"type"`
	Cursor string `json:"cursor,omitempty" table:"-"`

	Actor     *Actor     `json:"actor,omitempty"      table:"actor"`
	CreatedAt *time.Time `json:"created_at,omitempty" table:"created,time"`

	Body     string `json:"body,omitempty"      table:"body,truncate"`
	BodyHTML string `json:"body_html,omitempty" table:"-"`

	Label     *Label     `json:"label,omitempty"      table:"-"`
	Milestone *Milestone `json:"milestone,omitempty"  table:"-"`
	Assignee  *Actor     `json:"assignee,omitempty"   table:"-"`
	FromTitle string     `json:"from_title,omitempty" table:"-"`
	ToTitle   string     `json:"to_title,omitempty"   table:"-"`
	Commit    string     `json:"commit,omitempty"     table:"-"`
	Source    string     `json:"source,omitempty"     table:"-"`
	Reactions []Reaction `json:"reactions,omitempty"  table:"-"`

	Minimized       bool       `json:"minimized"                  table:"-"`
	MinimizedReason string     `json:"minimized_reason,omitempty" table:"-"`
	CreatedViaEmail bool       `json:"created_via_email"          table:"-"`
	LastEditedAt    *time.Time `json:"last_edited_at,omitempty"   table:"-"`
}

// --- commits ---

// Commit is one commit. Authors is a list because co-authored commits are
// common and the payload already ships an array; Committer is separate and only
// differs from the author when the surface says it does.
type Commit struct {
	Base

	Repo string `json:"repo" table:"-"`
	SHA  string `json:"sha"  table:"sha"`

	Subject          string `json:"subject"                     table:"subject,truncate"`
	SubjectHighlight string `json:"subject_highlight,omitempty" table:"-"`
	Body             string `json:"body,omitempty"              table:"-"`
	BodyHTML         string `json:"body_html,omitempty"         table:"-"`

	Authors   []Actor `json:"authors,omitempty"   table:"authors"`
	Committer *Actor  `json:"committer,omitempty" table:"-"`
	Pusher    *Actor  `json:"pusher,omitempty"    table:"-"`

	AuthoredAt  *time.Time `json:"authored_at,omitempty"  table:"authored,time"`
	CommittedAt *time.Time `json:"committed_at,omitempty" table:"-"`
	PushedAt    *time.Time `json:"pushed_at,omitempty"    table:"-"`

	// DateGroup is the calendar-day heading the commit list grouped this commit
	// under. It is kept because it is the only place the surface tells you what
	// timezone it grouped in.
	DateGroup string `json:"date_group,omitempty" table:"-"`

	Verification string `json:"verification,omitempty" table:"-"`
	// VerificationReason is the why behind Verification: "unsigned",
	// "valid", "expired_key", and so on. Verification alone says a commit is
	// unverified without saying whether that is because nobody signed it or
	// because the signature failed, which are very different facts.
	VerificationReason string `json:"verification_reason,omitempty" table:"-"`
	SignedByGitHub     bool   `json:"signed_by_github"             table:"-"`
	HasSignature       bool   `json:"has_signature"                table:"-"`
	KeyID              string `json:"key_id,omitempty"             table:"-"`
	KeyExpired         bool   `json:"key_expired"                  table:"-"`

	StatusRollup  string `json:"status_rollup,omitempty"  table:"status"`
	StatusSummary string `json:"status_summary,omitempty" table:"-"`
	CommentCount  *int   `json:"comment_count,omitempty"  table:"-"`

	// IssueRefs are the issues and pull requests this commit's message closes
	// or mentions, already resolved by GitHub. This is the commit-to-thread
	// edge of the graph, handed over for free, and it is the reason commit
	// search is worth reading even when you already have the commit.
	IssueRefs []ThreadRef `json:"issue_refs,omitempty" table:"-"`

	Parents   []string     `json:"parents,omitempty"   table:"-"`
	Additions *int         `json:"additions,omitempty" table:"-"`
	Deletions *int         `json:"deletions,omitempty" table:"-"`
	Files     []FileChange `json:"files,omitempty"     table:"-"`
}

// ThreadRef is a pointer to an issue or a pull request from somewhere else. It
// is not a Thread: it carries only what the referring surface knew, and the
// caller resolves it with `github get` when it wants the rest.
type ThreadRef struct {
	DatabaseID    *int   `json:"database_id,omitempty" table:"id"`
	Title         string `json:"title,omitempty"       table:"title,truncate"`
	State         string `json:"state,omitempty"       table:"state"`
	IsPullRequest bool   `json:"is_pull_request"       table:"-"`
	Merged        bool   `json:"merged"                table:"-"`
	URL           string `json:"url,omitempty"         table:"url"`
}

// FileChange is one file in a commit or a diff.
type FileChange struct {
	Path      string `json:"path"                table:"path"`
	PrevPath  string `json:"prev_path,omitempty" table:"-"`
	Status    string `json:"status"              table:"status"`
	Additions *int   `json:"additions,omitempty" table:"+"`
	// Hidden by the same tag-grammar collision as PullRequest.Deletions.
	Deletions *int `json:"deletions,omitempty" table:"-"`
	IsBinary  bool `json:"is_binary"           table:"-"`
}

// --- refs and releases ---

// GitRef is a branch or a tag. It is not called Ref because Ident already owns
// the word "reference" in this package, and a git ref and a parsed URI are very
// different things to confuse in a stack trace.
//
// Three surfaces carry refs and each is incomplete differently: the branches
// page has authors and dates but a truncated list, the refs XHR has every name
// and nothing else, and the git protocol has every name with its SHA. The
// commands pick per question, which is why `github refs --names-only` is 6 KB
// where `github refs` is 588 KB.
type GitRef struct {
	Base

	Repo string `json:"repo"          table:"-"`
	Name string `json:"name"          table:"name"`
	Type string `json:"type"          table:"type"`
	SHA  string `json:"sha,omitempty" table:"sha"`

	// PeeledSHA is set for annotated tags, from the ^{} entry in the git
	// protocol advertisement.
	PeeledSHA string `json:"peeled_sha,omitempty" table:"-"`
	IsDefault bool   `json:"is_default"           table:"default"`
	Protected bool   `json:"protected"            table:"protected"`

	Author     *Actor     `json:"author,omitempty"      table:"author"`
	AuthoredAt *time.Time `json:"authored_at,omitempty" table:"authored,time"`
}

// Release is one published release. Assets and download counts exist only on
// the per-release HTML page, so the feed-driven listing leaves Assets nil and
// `--assets` opts into one request per release.
type Release struct {
	Base

	Repo string `json:"repo" table:"-"`
	Tag  string `json:"tag"  table:"tag"`

	Title    string `json:"title,omitempty"     table:"title,truncate"`
	Body     string `json:"body,omitempty"      table:"-"`
	BodyHTML string `json:"body_html,omitempty" table:"-"`

	Author *Actor `json:"author,omitempty" table:"author"`

	PublishedAt *time.Time `json:"published_at,omitempty" table:"published,time"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"   table:"-"`

	IsPrerelease bool `json:"is_prerelease" table:"pre"`
	IsLatest     bool `json:"is_latest"     table:"latest"`
	IsDraft      bool `json:"is_draft"      table:"-"`

	CommitSHA string `json:"commit_sha,omitempty" table:"-"`

	Assets     []Asset `json:"assets,omitempty"      table:"-"`
	TarballURL string  `json:"tarball_url,omitempty" table:"-"`
	ZipballURL string  `json:"zipball_url,omitempty" table:"-"`

	// RepoDatabaseID comes free from the Atom <id>, which is
	// tag:github.com,2008:Repository/11180687/v0.164.0. That is how a release
	// read from a feed joins to a repository record without a second fetch.
	RepoDatabaseID *int `json:"repo_database_id,omitempty" table:"-"`
}

// Asset is one release download.
type Asset struct {
	Name          string     `json:"name"                     table:"name"`
	Size          *int64     `json:"size,omitempty"           table:"size"`
	SizeDisplay   string     `json:"size_display,omitempty"   table:"-"`
	DownloadCount *int       `json:"download_count,omitempty" table:"downloads"`
	URL           string     `json:"url"                      table:"url,url"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"     table:"-"`
	ContentType   string     `json:"content_type,omitempty"   table:"-"`
}

// --- the long tail ---

// Topic is a curated or uncurated topic. The search result carries most of it;
// the long description, the logo, the creator, the release year, the Wikipedia
// link, and the aliases need the topic page.
type Topic struct {
	Base

	Name        string `json:"name"                   table:"name"`
	DisplayName string `json:"display_name,omitempty" table:"display"`

	ShortDescription string `json:"short_description,omitempty" table:"description,truncate"`
	Description      string `json:"description,omitempty"       table:"-"`
	DescriptionHTML  string `json:"description_html,omitempty"  table:"-"`

	LogoURL      string   `json:"logo_url,omitempty"      table:"-"`
	WikipediaURL string   `json:"wikipedia_url,omitempty" table:"-"`
	GitHubURL    string   `json:"github_url,omitempty"    table:"-"`
	CreatedBy    string   `json:"created_by,omitempty"    table:"-"`
	Released     string   `json:"released,omitempty"      table:"released"`
	Aliases      []string `json:"aliases,omitempty"       table:"-"`
	Related      []string `json:"related,omitempty"       table:"-"`

	StargazerCount *int `json:"stargazer_count,omitempty" table:"stars"`
	AppliedCount   *int `json:"applied_count,omitempty"   table:"repos"`

	Featured bool `json:"featured" table:"-"`
	Curated  bool `json:"curated"  table:"-"`
}

// Package is a published package. Search is the only source, which means the
// record is complete the moment it is read.
type Package struct {
	Base

	Repo string `json:"repo,omitempty" table:"repo"`
	Name string `json:"name"           table:"name"`
	Type string `json:"type"           table:"type"`

	Summary   string   `json:"summary,omitempty"   table:"summary,truncate"`
	Downloads *int     `json:"downloads,omitempty" table:"downloads"`
	Topics    []string `json:"topics,omitempty"    table:"-"`
	Source    string   `json:"source,omitempty"    table:"-"`

	UpdatedAt *time.Time `json:"updated_at,omitempty" table:"updated,time"`
}

// WikiPage is one page of a repository wiki.
type WikiPage struct {
	Base

	Repo   string `json:"repo"             table:"repo"`
	Title  string `json:"title"            table:"title"`
	Path   string `json:"path"             table:"path"`
	Format string `json:"format,omitempty" table:"-"`

	Body     string `json:"body,omitempty"      table:"-"`
	BodyHTML string `json:"body_html,omitempty" table:"-"`

	UpdatedAt *time.Time `json:"updated_at,omitempty" table:"updated,time"`
	Author    *Actor     `json:"author,omitempty"     table:"author"`
}

// Gist is a gist and its files.
type Gist struct {
	Base

	Owner       string `json:"owner,omitempty"       table:"owner"`
	Description string `json:"description,omitempty" table:"description,truncate"`

	IsPublic  bool `json:"is_public"            table:"public"`
	FileCount *int `json:"file_count,omitempty" table:"files"`
	Forks     *int `json:"forks,omitempty"      table:"forks"`
	Stars     *int `json:"stars,omitempty"      table:"stars"`
	Revisions *int `json:"revisions,omitempty"  table:"-"`

	Files []GistFile `json:"files,omitempty" table:"-"`

	CreatedAt *time.Time `json:"created_at,omitempty" table:"created,time"`
	UpdatedAt *time.Time `json:"updated_at,omitempty" table:"-"`
}

// GistFile is one file in a gist.
type GistFile struct {
	Name     string `json:"name"               table:"name"`
	Language string `json:"language,omitempty" table:"language"`
	Size     *int64 `json:"size,omitempty"     table:"size"`
	RawURL   string `json:"raw_url"            table:"-"`
	Content  string `json:"content,omitempty"  table:"-"`
}

// Action is a marketplace listing. The type covers both actions and apps, and
// Type says which.
type Action struct {
	Base

	Name        string `json:"name"                  table:"name"`
	Slug        string `json:"slug"                  table:"-"`
	Owner       string `json:"owner,omitempty"       table:"owner"`
	Description string `json:"description,omitempty" table:"description,truncate"`

	// ShortDescription is the one-line blurb on the listing card, which is a
	// different string from Description on an app and the same one on a
	// repository action. Both are kept rather than picked between.
	ShortDescription    string `json:"short_description,omitempty"    table:"-"`
	FullDescription     string `json:"full_description,omitempty"     table:"-"`
	ExtendedDescription string `json:"extended_description,omitempty" table:"-"`

	Type              string   `json:"type,omitempty"               table:"type"`
	PrimaryCategory   string   `json:"primary_category,omitempty"   table:"category"`
	SecondaryCategory string   `json:"secondary_category,omitempty" table:"-"`
	Highlights        []string `json:"highlights,omitempty"         table:"-"`

	// A repository action and a marketplace app are both listings and the
	// search results are interleaved, but only one of these two groups is ever
	// populated for a given record. Which group is filled in is itself the
	// answer to "what kind of thing is this".
	Path         string `json:"path,omitempty"          table:"-"`
	RepositoryID *int   `json:"repository_id,omitempty" table:"-"`
	IconName     string `json:"icon_name,omitempty"     table:"-"`
	IconColor    string `json:"icon_color,omitempty"    table:"-"`

	ListingID         *int   `json:"listing_id,omitempty"         table:"-"`
	LogoURL           string `json:"logo_url,omitempty"           table:"-"`
	InstallationCount *int   `json:"installation_count,omitempty" table:"installs"`
	State             string `json:"state,omitempty"              table:"state"`
	CompanyURL        string `json:"company_url,omitempty"        table:"-"`
	DocumentationURL  string `json:"documentation_url,omitempty"  table:"-"`
	SupportURL        string `json:"support_url,omitempty"        table:"-"`
	PrivacyPolicyURL  string `json:"privacy_policy_url,omitempty" table:"-"`
	TermsURL          string `json:"terms_url,omitempty"          table:"-"`
	PricingURL        string `json:"pricing_url,omitempty"        table:"-"`

	Stars          *int `json:"stars,omitempty"           table:"stars"`
	DependentCount *int `json:"dependent_count,omitempty" table:"used_by"`

	IsFree          bool `json:"is_free"           table:"-"`
	IsVerifiedOwner bool `json:"is_verified_owner" table:"verified"`
	IsFeatured      bool `json:"is_featured"       table:"-"`
	IsRecommended   bool `json:"is_recommended"    table:"-"`
	ByGitHub        bool `json:"by_github"         table:"-"`
}

// Trending embeds Repo because a trending entry is a repository with three
// extra facts. Embedding is what makes `github trending -o url | xargs -n1
// github get` work with no special case anywhere.
type Trending struct {
	Repo

	StarsInPeriod *int    `json:"stars_in_period,omitempty" table:"period_stars"`
	Period        string  `json:"period"                    table:"period"`
	BuiltBy       []Actor `json:"built_by,omitempty"        table:"-"`
	Rank          int     `json:"rank"                      table:"rank"`
}

// --- contributions ---

// Contributor is one person's contribution statistics for a repository. Weeks
// arrives with the response so it is kept by default, and it is never a table
// column because a hundred weeks is not a column.
type Contributor struct {
	Base

	Repo  string `json:"repo"  table:"repo"`
	Login string `json:"login" table:"login"`

	Commits   *int `json:"commits,omitempty"   table:"commits"`
	Additions *int `json:"additions,omitempty" table:"+"`
	// Hidden by the tag-grammar collision, as everywhere else.
	Deletions *int `json:"deletions,omitempty" table:"-"`

	// FirstWeek and LastWeek are derived by trimming the leading and trailing
	// zero weeks, which turns a six-hundred-element array into two dates a
	// table can show.
	FirstWeek *time.Time `json:"first_week,omitempty" table:"first,time"`
	LastWeek  *time.Time `json:"last_week,omitempty"  table:"last,time"`

	Weeks []ContributorWeek `json:"weeks,omitempty" table:"-"`

	AvatarURL  string `json:"avatar_url,omitempty"  table:"-"`
	DatabaseID *int   `json:"database_id,omitempty" table:"-"`
}

// ContributorWeek is one week of one contributor's statistics.
type ContributorWeek struct {
	Week      time.Time `json:"week"      table:"week,time"`
	Additions int       `json:"additions" table:"+"`
	Deletions int       `json:"deletions" table:"-"`
	Commits   int       `json:"commits"   table:"commits"`
}

// ContributionDay is one square of a profile contribution graph.
type ContributionDay struct {
	Base

	Login string    `json:"login" table:"login"`
	Date  time.Time `json:"date"  table:"date,time"`
	Count int       `json:"count" table:"count"`
	Level int       `json:"level" table:"level"`
}

// Event is one entry of an activity feed. Type is derived from the entry id,
// which encodes the event class, rather than from the title text, which is
// prose and is localised.
type Event struct {
	Base

	Actor Actor  `json:"actor"          table:"actor"`
	Type  string `json:"type"           table:"type"`
	Repo  string `json:"repo,omitempty" table:"repo"`

	Title    string     `json:"title,omitempty"     table:"title,truncate"`
	BodyHTML string     `json:"body_html,omitempty" table:"-"`
	Target   string     `json:"target,omitempty"    table:"-"`
	At       *time.Time `json:"at,omitempty"        table:"at,time"`
}
