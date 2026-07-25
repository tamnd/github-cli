package gh

import (
	"context"
	"encoding/json"
	"html"
	"strconv"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
)

// search.go is the widest surface this tool has. Ten search types answer JSON
// to an anonymous Accept header, and nine of them return real results. Code
// search is the tenth: it answers 200 with an empty result set, because it
// needs a session, and this file says so out loud rather than returning zero
// hits and letting the caller conclude the query was wrong.
//
// Search results are decoded into the same record types a full read produces.
// A caller doing `github repos --org golang` gets Repo records, thinner but the
// same type, so --fields works the same and the output pipes into `github get`
// for the full read. A record from a listing is a real record with fewer
// fields, never a different type.

// The site's own names for the search types. They are not guessable (issues but
// pullrequests, registrypackages but wikis) which is exactly why they are
// constants.
const (
	SearchRepos       = "repositories"
	SearchIssues      = "issues"
	SearchPulls       = "pullrequests"
	SearchUsers       = "users"
	SearchCommits     = "commits"
	SearchDiscussions = "discussions"
	SearchTopics      = "topics"
	SearchPackages    = "registrypackages"
	SearchWikis       = "wikis"
	SearchMarket      = "marketplace"
	SearchCode        = "code"
)

// SearchTypes is every type in the order the commands present them.
var SearchTypes = []string{
	SearchRepos, SearchIssues, SearchPulls, SearchUsers, SearchCommits,
	SearchDiscussions, SearchTopics, SearchPackages, SearchWikis, SearchMarket,
}

// searchEnvelope is the shape every search type shares.
type searchEnvelope struct {
	Payload struct {
		Results     []json.RawMessage `json:"results"`
		Type        string            `json:"type"`
		Page        int               `json:"page"`
		PageCount   int               `json:"page_count"`
		ResultCount int               `json:"result_count"`
		Errors      []string          `json:"errors"`
		// WarnLimitedResults is set when GitHub capped the result set, which it
		// does silently otherwise.
		WarnLimitedResults bool `json:"warn_limited_results"`
	} `json:"payload"`
}

// searchFetch turns one search type into a pager. The numbered ?p=N form is the
// only pagination search has, and page_count is authoritative, so the walk stops
// on the count rather than on a short page.
func searchFetch[T any](c *Client, query, typ string, decode func(json.RawMessage) (T, bool)) fetchPage[T] {
	return func(ctx context.Context, token string) ([]T, string, error) {
		n := pageToken(token)
		var env searchEnvelope
		res, err := c.GetJSON(ctx, searchURL(query, typ, n), SurfaceSearch, &env)
		if err != nil {
			return nil, "", err
		}
		if len(env.Payload.Errors) > 0 {
			return nil, "", errs.Usage("search: %s", strings.Join(env.Payload.Errors, "; "))
		}
		out := make([]T, 0, len(env.Payload.Results))
		for _, raw := range env.Payload.Results {
			rec, ok := decode(raw)
			if !ok {
				continue
			}
			if b := baseOf(&rec); b != nil {
				b.addSource(res.FinalURL)
			}
			out = append(out, rec)
		}
		next := ""
		if n < env.Payload.PageCount {
			next = strconv.Itoa(n + 1)
		}
		return out, next, nil
	}
}

// baseOf reaches the embedded Base of a record so the pager can stamp the
// source URL without every decoder repeating it.
func baseOf(v any) *Base {
	type based interface{ base() *Base }
	if b, ok := v.(based); ok {
		return b.base()
	}
	return nil
}

func (b *Base) base() *Base { return b }

// --- the nine working types ---

// SearchRepositories streams repository records for a query.
func (c *Client) SearchRepositories(ctx context.Context, query string, limit int, emit func(Repo) error) error {
	return paginate(ctx, limit, searchFetch(c, query, SearchRepos, decodeSearchRepo), emit)
}

// SearchIssuesAndPulls streams thread records. typ is SearchIssues or
// SearchPulls: the result shape is identical and only the qualifier differs,
// which is why one decoder serves both.
func (c *Client) SearchIssuesAndPulls(ctx context.Context, query, typ string, limit int, emit func(Thread) error) error {
	return paginate(ctx, limit, searchFetch(c, query, typ, decodeSearchThread), emit)
}

// SearchAccounts streams user records.
func (c *Client) SearchAccounts(ctx context.Context, query string, limit int, emit func(Account) error) error {
	return paginate(ctx, limit, searchFetch(c, query, SearchUsers, decodeSearchAccount), emit)
}

// SearchCommitsBy streams commit records. Commit search is the only source for
// signature and verification state on a keyless surface.
func (c *Client) SearchCommitsBy(ctx context.Context, query string, limit int, emit func(Commit) error) error {
	return paginate(ctx, limit, searchFetch(c, query, SearchCommits, decodeSearchCommit), emit)
}

// SearchDiscussionsBy streams discussion records.
func (c *Client) SearchDiscussionsBy(ctx context.Context, query string, limit int, emit func(Discussion) error) error {
	return paginate(ctx, limit, searchFetch(c, query, SearchDiscussions, decodeSearchDiscussion), emit)
}

// SearchTopicsBy streams topic records.
func (c *Client) SearchTopicsBy(ctx context.Context, query string, limit int, emit func(Topic) error) error {
	return paginate(ctx, limit, searchFetch(c, query, SearchTopics, decodeSearchTopic), emit)
}

// SearchPackagesBy streams package records. Search is the only source for
// packages, so these records are complete rather than thin.
func (c *Client) SearchPackagesBy(ctx context.Context, query string, limit int, emit func(Package) error) error {
	return paginate(ctx, limit, searchFetch(c, query, SearchPackages, decodeSearchPackage), emit)
}

// SearchWikisBy streams wiki page records.
func (c *Client) SearchWikisBy(ctx context.Context, query string, limit int, emit func(WikiPage) error) error {
	return paginate(ctx, limit, searchFetch(c, query, SearchWikis, decodeSearchWiki), emit)
}

// SearchMarketplace streams action and app listings.
func (c *Client) SearchMarketplace(ctx context.Context, query string, limit int, emit func(Action) error) error {
	return paginate(ctx, limit, searchFetch(c, query, SearchMarket, decodeSearchAction), emit)
}

// SearchCodeBy is the honest gap. The route answers 200 with zero results
// without a session, which is the worst possible failure mode: it looks like
// the query matched nothing.
func (c *Client) SearchCodeBy(context.Context, string, int, func(File) error) error {
	return notPublic("code search", "the route answers 200 with an empty result set to an anonymous client, which is indistinguishable from no matches")
}

// --- decoders ---

type searchRepoResult struct {
	ID                  string   `json:"id"`
	Archived            bool     `json:"archived"`
	Color               string   `json:"color"`
	Followers           *int     `json:"followers"`
	HasFundingFile      bool     `json:"has_funding_file"`
	HLName              string   `json:"hl_name"`
	HLTruncDescription  string   `json:"hl_trunc_description"`
	Language            string   `json:"language"`
	Mirror              bool     `json:"mirror"`
	OwnedByOrganization bool     `json:"owned_by_organization"`
	Public              bool     `json:"public"`
	Sponsorable         bool     `json:"sponsorable"`
	Topics              []string `json:"topics"`
	Type                string   `json:"type"`
	HelpWanted          *int     `json:"help_wanted_issues_count"`
	GoodFirstIssue      *int     `json:"good_first_issue_issues_count"`
	Repo                repoNest `json:"repo"`
}

// repoNest is the doubly-wrapped repository reference search uses everywhere:
// {"repo":{"repository":{...}}}. Modelling it once keeps six decoders from each
// spelling it out.
type repoNest struct {
	Repository struct {
		ID         *int   `json:"id"`
		Name       string `json:"name"`
		OwnerLogin string `json:"owner_login"`
		OwnerID    *int   `json:"owner_id"`
		UpdatedAt  string `json:"updated_at"`
		HasIssues  bool   `json:"has_issues"`
	} `json:"repository"`
}

func (r repoNest) id() string {
	if r.Repository.OwnerLogin == "" || r.Repository.Name == "" {
		return ""
	}
	return r.Repository.OwnerLogin + "/" + r.Repository.Name
}

func decodeSearchRepo(raw json.RawMessage) (Repo, bool) {
	var s searchRepoResult
	if err := json.Unmarshal(raw, &s); err != nil {
		return Repo{}, false
	}
	id := s.Repo.id()
	if id == "" {
		id = stripHL(s.HLName)
	}
	owner, name, ok := SplitRepo(id)
	if !ok {
		return Repo{}, false
	}
	r := Repo{Owner: owner, Name: name}
	r.setIdentity(KindRepo, id)
	r.DatabaseID = s.Repo.Repository.ID
	r.Description = stripHL(s.HLTruncDescription)
	if r.Description != s.HLTruncDescription {
		r.DescriptionHighlight = s.HLTruncDescription
	}
	r.Language = s.Language
	r.LanguageColor = s.Color
	r.Stars = s.Followers
	r.Topics = s.Topics
	r.IsArchived = s.Archived
	r.IsMirror = s.Mirror
	r.IsOrgOwned = s.OwnedByOrganization
	r.IsPrivate = !s.Public
	r.Visibility = strings.ToLower(s.Type)
	r.Sponsorable = s.Sponsorable
	r.HasFunding = s.HasFundingFile
	r.HelpWantedIssues = s.HelpWanted
	r.GoodFirstIssues = s.GoodFirstIssue
	r.UpdatedAt = parseTime(s.Repo.Repository.UpdatedAt)

	r.addExtra("search", decodeExtra(raw, &s,
		// Viewer state, always false for an anonymous read.
		"starred_by_current_user", "followed_by_current_user", "is_current_user",
	))
	return r, true
}

type searchThreadResult struct {
	AuthorName      string   `json:"author_name"`
	AuthorAvatarURL string   `json:"author_avatar_url"`
	ID              string   `json:"id"`
	Repo            repoNest `json:"repo"`
	Labels          []string `json:"labels"`
	NumComments     *int     `json:"num_comments"`
	Number          int      `json:"number"`
	State           string   `json:"state"`
	StateReason     *string  `json:"state_reason"`
	HLTitle         string   `json:"hl_title"`
	HLText          string   `json:"hl_text"`
	Created         string   `json:"created"`
	ReviewableState *string  `json:"reviewable_state"`
	Merged          *bool    `json:"merged"`
	Issue           struct {
		Issue struct {
			PullRequestID *int `json:"pull_request_id"`
		} `json:"issue"`
	} `json:"issue"`
}

// decodeSearchThread produces a Thread. The caller decides whether it wanted
// issues or pull requests, and pull_request_id says which one this actually is,
// so a query that mixes them still classifies each result correctly.
func decodeSearchThread(raw json.RawMessage) (Thread, bool) {
	var s searchThreadResult
	if err := json.Unmarshal(raw, &s); err != nil {
		return Thread{}, false
	}
	repo := s.Repo.id()
	if repo == "" || s.Number == 0 {
		return Thread{}, false
	}
	kind := KindIssue
	if s.Issue.Issue.PullRequestID != nil {
		kind = KindPR
	}
	t := Thread{Repo: repo, Number: s.Number}
	t.setIdentity(kind, repo+"#"+strconv.Itoa(s.Number))
	t.Title = stripHL(s.HLTitle)
	if t.Title != s.HLTitle {
		t.TitleHighlight = s.HLTitle
	}
	t.Body = stripHL(s.HLText)
	t.State = s.State
	if s.StateReason != nil {
		t.StateReason = *s.StateReason
	}
	t.CommentCount = s.NumComments
	t.CreatedAt = parseTime(s.Created)
	t.Author = actor(s.AuthorName)
	t.Author.AvatarURL = s.AuthorAvatarURL
	for _, l := range s.Labels {
		t.Labels = append(t.Labels, Label{Name: l})
	}
	if n, err := strconv.Atoi(s.ID); err == nil {
		t.DatabaseID = intp(n)
	}

	t.addExtra("search", decodeExtra(raw, &s))
	return t, true
}

type searchUserResult struct {
	AvatarURL    string `json:"avatar_url"`
	HLLogin      string `json:"hl_login"`
	HLName       string `json:"hl_name"`
	HLProfileBio string `json:"hl_profile_bio"`
	Followers    *int   `json:"followers"`
	ID           string `json:"id"`
	Location     string `json:"location"`
	Login        string `json:"login"`
	DisplayLogin string `json:"display_login"`
	Name         string `json:"name"`
	ProfileBio   string `json:"profile_bio"`
	Sponsorable  bool   `json:"sponsorable"`
	Repos        *int   `json:"repos"`
}

func decodeSearchAccount(raw json.RawMessage) (Account, bool) {
	var s searchUserResult
	if err := json.Unmarshal(raw, &s); err != nil {
		return Account{}, false
	}
	login := firstNonEmpty(s.Login, stripHL(s.HLLogin))
	if login == "" {
		return Account{}, false
	}
	a := Account{Login: login, Type: "User"}
	a.setIdentity(KindUser, login)
	a.Name = s.Name
	a.Bio = s.ProfileBio
	a.Location = s.Location
	a.AvatarURL = s.AvatarURL
	a.Followers = s.Followers
	a.RepoCount = s.Repos
	a.Sponsorable = s.Sponsorable
	if n, err := strconv.Atoi(s.ID); err == nil {
		a.DatabaseID = intp(n)
	}

	a.addExtra("search", decodeExtra(raw, &s,
		// Viewer state.
		"followed_by_current_user", "is_current_user",
	))
	return a, true
}

type searchCommitResult struct {
	ID                 string   `json:"id"`
	SHA                string   `json:"sha"`
	AuthorDate         string   `json:"author_date"`
	HLSubject          string   `json:"hl_subject"`
	HLBody             string   `json:"hl_body"`
	Message            string   `json:"message"`
	Repository         repoNest `json:"repository"`
	VerificationStatus string   `json:"verification_status"`
	VerificationReason string   `json:"signature_verification_reason"`
	SignedByGitHub     bool     `json:"signed_by_github"`
	HasSignature       bool     `json:"has_signature"`
	KeyExpired         bool     `json:"key_expired"`
	KeyID              string   `json:"key_id"`
	ChecksState        string   `json:"checks_header_state"`
	ChecksSummary      string   `json:"checks_status_summary"`

	// The plural is not a typo. Commit search reports co-authors, so this is a
	// list where a single author field would lose the trailer.
	Authors []struct {
		Login       string `json:"login"`
		DisplayName string `json:"display_name"`
		AvatarURL   string `json:"avatar_url"`
	} `json:"authors"`

	// Committer is present only when it differs from the author, which on
	// golang/go means gopherbot on every commit and on most repositories means
	// nothing at all.
	Committer *struct {
		Login       string `json:"login"`
		DisplayName string `json:"display_name"`
		AvatarURL   string `json:"avatar_url"`
	} `json:"committer"`

	IssueReferences []struct {
		ID            *int   `json:"id"`
		Title         string `json:"title"`
		State         string `json:"state"`
		IsPullRequest bool   `json:"is_pull_request"`
		Permalink     string `json:"permalink"`
		Merged        bool   `json:"merged"`
	} `json:"issue_references"`
}

func decodeSearchCommit(raw json.RawMessage) (Commit, bool) {
	var s searchCommitResult
	if err := json.Unmarshal(raw, &s); err != nil {
		return Commit{}, false
	}
	repo := s.Repository.id()
	if repo == "" || s.SHA == "" {
		return Commit{}, false
	}
	c := Commit{Repo: repo, SHA: s.SHA}
	c.setIdentity(KindCommit, repo+"@"+s.SHA)
	subject, body, _ := strings.Cut(s.Message, "\n\n")
	c.Subject = strings.TrimSpace(subject)
	c.Body = strings.TrimSpace(body)
	// hl_subject arrives wrapped in an anchor rather than as plain text, which
	// is the one place search returns markup instead of a highlighted string.
	if hl := stripTags(s.HLSubject); hl != "" && c.Subject == "" {
		c.Subject = hl
	}
	c.SubjectHighlight = s.HLSubject
	c.AuthoredAt = parseTime(s.AuthorDate)
	c.Verification = s.VerificationStatus
	c.VerificationReason = s.VerificationReason
	c.SignedByGitHub = s.SignedByGitHub
	c.HasSignature = s.HasSignature
	c.KeyID = s.KeyID
	c.KeyExpired = s.KeyExpired
	c.StatusRollup = s.ChecksState
	c.StatusSummary = s.ChecksSummary
	for _, a := range s.Authors {
		act := actor(a.Login)
		act.Name = a.DisplayName
		act.AvatarURL = a.AvatarURL
		c.Authors = append(c.Authors, act)
	}
	if s.Committer != nil {
		act := actor(s.Committer.Login)
		act.Name = s.Committer.DisplayName
		act.AvatarURL = s.Committer.AvatarURL
		c.Committer = &act
	}
	for _, r := range s.IssueReferences {
		c.IssueRefs = append(c.IssueRefs, ThreadRef{
			DatabaseID:    r.ID,
			Title:         r.Title,
			State:         r.State,
			IsPullRequest: r.IsPullRequest,
			Merged:        r.Merged,
			URL:           r.Permalink,
		})
	}

	c.addExtra("search", decodeExtra(raw, &s,
		// A link to GitHub's own docs about signature verification.
		"help_url",
		// The whole CI rollup, which is a repository-shaped blob three levels
		// deep and belongs to `github checks`, not to a commit listing.
		"status_check_rollup",
		// Prose assembled from authors and committer_attribution for a
		// tooltip, and the flag that generated it.
		"commit_author_tooltip", "committer_attribution",
		// Viewer state.
		"is_viewer",
	))
	return c, true
}

type searchDiscussionResult struct {
	Body          string   `json:"body"`
	Created       string   `json:"created"`
	Updated       string   `json:"updated"`
	HLText        string   `json:"hl_text"`
	HLTitle       string   `json:"hl_title"`
	ID            string   `json:"id"`
	NumComments   *int     `json:"num_comments"`
	Number        int      `json:"number"`
	Repo          repoNest `json:"repo"`
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	UserAvatarURL string   `json:"user_avatar_url"`
	UserID        *int     `json:"user_id"`
	UserLogin     string   `json:"user_login"`
}

func decodeSearchDiscussion(raw json.RawMessage) (Discussion, bool) {
	var s searchDiscussionResult
	if err := json.Unmarshal(raw, &s); err != nil {
		return Discussion{}, false
	}
	repo := s.Repo.id()
	if repo == "" || s.Number == 0 {
		return Discussion{}, false
	}
	var d Discussion
	d.Repo = repo
	d.Number = s.Number
	d.setIdentity(KindDiscussion, repo+"#"+strconv.Itoa(s.Number))
	d.Title = firstNonEmpty(s.Title, stripHL(s.HLTitle))
	d.TitleHighlight = s.HLTitle
	d.Body = s.Body
	d.CommentCount = s.NumComments
	d.CreatedAt = parseTime(s.Created)
	d.UpdatedAt = parseTime(s.Updated)
	d.Author = actor(s.UserLogin)
	d.Author.AvatarURL = s.UserAvatarURL
	d.Author.DatabaseID = s.UserID
	if n, err := strconv.Atoi(s.ID); err == nil {
		d.DatabaseID = intp(n)
	}

	d.addExtra("search", decodeExtra(raw, &s))
	return d, true
}

type searchTopicResult struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	ShortDescription   string   `json:"short_description"`
	DisplayName        string   `json:"display_name"`
	Released           string   `json:"released"`
	WikipediaURL       string   `json:"wikipedia_url"`
	URL                string   `json:"url"`
	GitHubURL          string   `json:"github_url"`
	LogoURL            string   `json:"logo_url"`
	Featured           bool     `json:"featured"`
	Curated            bool     `json:"curated"`
	StargazerCount     *int     `json:"stargazer_count"`
	AppliedCount       *int     `json:"applied_count"`
	HLDisplayName      string   `json:"hl_display_name"`
	HLShortDescription string   `json:"hl_short_description"`
	CreatedBy          string   `json:"created_by"`
	Aliases            []string `json:"aliases"`
	Related            []string `json:"related"`
}

func decodeSearchTopic(raw json.RawMessage) (Topic, bool) {
	var s searchTopicResult
	if err := json.Unmarshal(raw, &s); err != nil || s.Name == "" {
		return Topic{}, false
	}
	t := Topic{Name: s.Name}
	t.setIdentity(KindTopic, s.Name)
	t.DisplayName = s.DisplayName
	t.ShortDescription = s.ShortDescription
	t.Released = s.Released
	t.WikipediaURL = s.WikipediaURL
	t.GitHubURL = s.GitHubURL
	t.LogoURL = s.LogoURL
	t.Featured = s.Featured
	t.Curated = s.Curated
	t.StargazerCount = s.StargazerCount
	t.AppliedCount = s.AppliedCount
	t.CreatedBy = s.CreatedBy
	t.Aliases = s.Aliases
	t.Related = s.Related

	t.addExtra("search", decodeExtra(raw, &s,
		// True when logo_url is set, which the record already carries.
		"has_logo_url",
		// A moderation flag with no public meaning.
		"flagged",
		// The highlight map repeats hl_display_name and hl_short_description in
		// a nested shape, and both are already modelled.
		"highlights",
		// A ceiling marker on repository_count, kept out because the count it
		// qualifies is not one this record claims.
		"repository_count", "repository_count_over_max_fetch_limit",
		// Viewer state.
		"starred_by_current_user",
	))
	return t, true
}

type searchPackageResult struct {
	ID          string   `json:"id"`
	Color       string   `json:"color"`
	Downloads   *int     `json:"downloads"`
	Name        string   `json:"name"`
	PackageURL  string   `json:"package_url"`
	PackageType string   `json:"package_type"`
	Public      bool     `json:"public"`
	Summary     string   `json:"summary"`
	Topics      []string `json:"topics"`
	UpdatedAt   string   `json:"updated_at"`
	Repo        struct {
		Name       string `json:"name"`
		OwnerLogin string `json:"owner_login"`
	} `json:"repo"`
	Source struct {
		PackageType string `json:"package_type"`
	} `json:"source"`
}

func decodeSearchPackage(raw json.RawMessage) (Package, bool) {
	var s searchPackageResult
	if err := json.Unmarshal(raw, &s); err != nil || s.Name == "" {
		return Package{}, false
	}
	p := Package{Name: s.Name}
	if s.Repo.OwnerLogin != "" && s.Repo.Name != "" {
		p.Repo = s.Repo.OwnerLogin + "/" + s.Repo.Name
	}
	p.setIdentity(KindPackage, firstNonEmpty(p.Repo+"/"+s.Name, s.Name))
	if s.PackageURL != "" {
		p.URL = BaseURL + s.PackageURL
	}
	// package_type is null at the top level and populated inside source, which
	// looks like an oversight upstream but is consistent enough to rely on.
	p.Type = firstNonEmpty(s.PackageType, s.Source.PackageType)
	p.Summary = s.Summary
	p.Downloads = s.Downloads
	p.Topics = s.Topics
	p.UpdatedAt = parseTime(s.UpdatedAt)

	p.addExtra("search", decodeExtra(raw, &s,
		// The language colour of the source repository, which belongs to the
		// repository record and not to the package.
		"color",
		// The whole source blob: registry internals, ids, and a version list
		// that `github package` reads properly.
		"source",
	))
	return p, true
}

type searchWikiResult struct {
	Body      string   `json:"body"`
	Filename  string   `json:"filename"`
	Format    string   `json:"format"`
	HLBody    string   `json:"hl_body"`
	HLTitle   string   `json:"hl_title"`
	ID        string   `json:"id"`
	Path      string   `json:"path"`
	Public    bool     `json:"public"`
	Repo      repoNest `json:"repo"`
	RepoID    *int     `json:"repo_id"`
	Title     string   `json:"title"`
	UpdatedAt string   `json:"updated_at"`
}

func decodeSearchWiki(raw json.RawMessage) (WikiPage, bool) {
	var s searchWikiResult
	if err := json.Unmarshal(raw, &s); err != nil {
		return WikiPage{}, false
	}
	repo := s.Repo.id()
	if repo == "" {
		return WikiPage{}, false
	}
	w := WikiPage{Repo: repo, Path: s.Path, Format: s.Format}
	w.Title = firstNonEmpty(s.Title, stripHL(s.HLTitle))
	w.setIdentity(KindWiki, repo+"/"+firstNonEmpty(s.Path, s.Filename))
	w.Body = s.Body
	w.UpdatedAt = parseTime(s.UpdatedAt)

	w.addExtra("search", decodeExtra(raw, &s))
	return w, true
}

// searchActionResult covers both listing shapes at once. A repository action
// fills repository_action and a marketplace app fills marketplace_listing, and
// the two never overlap, so decoding both and taking whichever arrived is
// simpler and more honest than branching on type.
type searchActionResult struct {
	Type              string `json:"type"`
	ID                string `json:"id"`
	Name              string `json:"name"`
	Free              bool   `json:"free"`
	PrimaryCategory   string `json:"primary_category"`
	SecondaryCategory string `json:"secondary_category"`
	IsVerifiedOwner   bool   `json:"is_verified_owner"`
	Slug              string `json:"slug"`
	OwnerLogin        string `json:"owner_login"`
	ResourcePath      string `json:"resource_path"`
	Description       string `json:"description"`
	ShortDescription  string `json:"short_description"`
	FullDescription   string `json:"full_description"`
	Stars             *int   `json:"stars"`
	DependentsCount   *int   `json:"dependents_count"`
	InstallationCount *int   `json:"installation_count"`
	ListingLogoURL    string `json:"listing_logo_url"`
	State             string `json:"state"`
	Recommended       bool   `json:"recommended"`

	RepositoryAction struct {
		RepositoryAction struct {
			ID           *int   `json:"id"`
			Path         string `json:"path"`
			Name         string `json:"name"`
			Description  string `json:"description"`
			IconName     string `json:"icon_name"`
			Color        string `json:"color"`
			Featured     bool   `json:"featured"`
			RepositoryID *int   `json:"repository_id"`
			Slug         string `json:"slug"`
		} `json:"repository_action"`
	} `json:"repository_action"`

	MarketplaceListing struct {
		Listing struct {
			ID                  *int   `json:"id"`
			Name                string `json:"name"`
			Slug                string `json:"slug"`
			ShortDescription    string `json:"short_description"`
			FullDescription     string `json:"full_description"`
			ExtendedDescription string `json:"extended_description"`
			PrivacyPolicyURL    string `json:"privacy_policy_url"`
			TOSURL              string `json:"tos_url"`
			CompanyURL          string `json:"company_url"`
			SupportURL          string `json:"support_url"`
			DocumentationURL    string `json:"documentation_url"`
			PricingURL          string `json:"pricing_url"`
			ByGitHub            bool   `json:"by_github"`
			ListableType        string `json:"listable_type"`
			ListableID          *int   `json:"listable_id"`
		} `json:"listing"`
	} `json:"marketplace_listing"`
}

func decodeSearchAction(raw json.RawMessage) (Action, bool) {
	var s searchActionResult
	if err := json.Unmarshal(raw, &s); err != nil {
		return Action{}, false
	}
	ra := s.RepositoryAction.RepositoryAction
	ml := s.MarketplaceListing.Listing

	slug := firstNonEmpty(s.Slug, ra.Slug, ml.Slug)
	if slug == "" {
		return Action{}, false
	}
	a := Action{Owner: s.OwnerLogin, Slug: slug}
	a.Name = firstNonEmpty(s.Name, ra.Name, ml.Name)
	a.setIdentity(KindAction, slug)
	if s.ResourcePath != "" {
		a.URL = BaseURL + s.ResourcePath
	}
	a.Description = firstNonEmpty(s.Description, ra.Description)
	a.ShortDescription = firstNonEmpty(s.ShortDescription, ml.ShortDescription)
	a.FullDescription = firstNonEmpty(s.FullDescription, ml.FullDescription)
	a.ExtendedDescription = ml.ExtendedDescription
	a.Type = s.Type
	a.PrimaryCategory = s.PrimaryCategory
	a.SecondaryCategory = s.SecondaryCategory
	a.IsFree = s.Free
	a.IsVerifiedOwner = s.IsVerifiedOwner
	a.IsRecommended = s.Recommended
	a.State = s.State
	a.Stars = s.Stars
	a.DependentCount = s.DependentsCount
	a.InstallationCount = s.InstallationCount
	a.LogoURL = s.ListingLogoURL

	a.Path = ra.Path
	a.IconName = ra.IconName
	a.IconColor = ra.Color
	a.RepositoryID = ra.RepositoryID
	a.IsFeatured = ra.Featured

	a.ListingID = ml.ID
	a.CompanyURL = ml.CompanyURL
	a.DocumentationURL = ml.DocumentationURL
	a.SupportURL = ml.SupportURL
	a.PrivacyPolicyURL = ml.PrivacyPolicyURL
	a.TermsURL = ml.TOSURL
	a.PricingURL = ml.PricingURL
	a.ByGitHub = ml.ByGitHub

	a.addExtra("search", decodeExtra(raw, &s,
		// Repeats name and description with <em> markers, both already modelled.
		"highlights",
		// The listing's own icon markup, which is a whole inline SVG document
		// and is a rendering concern rather than a fact about the action.
		"icon_svg",
	))
	return a, true
}

// --- highlight handling ---

// stripHL turns a hl_ field into plain text. Search wraps matches in <em> and
// entity-escapes the rest, so this is a fixed transform and not an HTML parse:
// nothing else ever appears in those fields.
func stripHL(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "<em>", "")
	s = strings.ReplaceAll(s, "</em>", "")
	return html.UnescapeString(s)
}

// stripTags is the wider hammer, needed for hl_subject on commit search, which
// arrives as a whole anchor element rather than as a highlighted string.
func stripTags(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>' && depth > 0:
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(html.UnescapeString(b.String()))
}
