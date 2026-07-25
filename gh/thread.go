package gh

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/tamnd/github-cli/pkg/page"
)

// thread.go reads issues, pull requests, and discussions.
//
// The spec files these three together because they are one concept, and they
// are one concept in GitHub's data model too. They are not one surface. Each of
// the three ships on a different plane, and the differences are large enough
// that pretending otherwise would cost more than it saves:
//
//	issue       React, Relay. The whole GraphQL issue node is preloaded into
//	            the page, labels, milestone, reactions, and fifteen timeline
//	            items included. This is the richest keyless surface on the site.
//	pull        React, route props. pullRequestsLayoutRoute carries the merge
//	            metadata and nothing else: no body, no timeline, no labels. The
//	            body comes from the hovercard fragment and is truncated, and
//	            that is the best a logged-out client can do.
//	discussion  Rails. No payload at all. A schema.org QAPage block carries the
//	            body, the upvotes, and the accepted answer; the category, the
//	            labels, and the participants come from the markup.
//
// The asymmetry is worth stating in the record rather than hiding, so a pull
// request read this way says where each field came from in Sources and leaves
// the fields it could not reach empty instead of guessing them.

// --- issues ---

// Issue reads one issue. repo is owner/name.
//
// A pull request number handed to this function does not 404: GitHub redirects
// /issues/{n} to /pull/{n}, and the read follows that redirect and hands off to
// PullRequest rather than returning a half-decoded record.
func (c *Client) Issue(ctx context.Context, repo string, number int) (*Issue, error) {
	if _, _, ok := SplitRepo(repo); !ok {
		return nil, usageBadID("repository", repo, "owner/name")
	}
	if number <= 0 {
		return nil, usageBadID("issue number", strconv.Itoa(number), "a positive integer")
	}
	res, err := c.GetHTML(ctx, threadURL(repo, "issues", number))
	if err != nil {
		return nil, err
	}
	if strings.Contains(res.FinalURL, "/pull/") {
		pr, err := c.PullRequest(ctx, repo, number)
		if err != nil {
			return nil, err
		}
		// The caller asked for an issue and got a pull request. Returning the
		// pull request's thread half is the honest answer: same number, same
		// repository, same conversation, and Kind says which it is.
		return &Issue{Thread: pr.Thread}, nil
	}

	p := page.Extract(res.FinalURL, res.Body)
	raw, ok := issueNode(p)
	if !ok {
		return nil, structureChanged(repo + "#" + strconv.Itoa(number))
	}

	iss, err := decodeRelayIssue(repo, raw)
	if err != nil {
		return nil, err
	}
	iss.addSource(res.FinalURL)
	if p.Canonical != "" {
		iss.URL = p.Canonical
	}
	return iss, nil
}

// issueNode digs the issue out of the preloaded Relay result. The query is
// named IssueViewerViewQuery today and the name is generated, so the lookup is
// by suffix with the first query as the fallback: on a thread page there is
// only ever one.
func issueNode(p *page.Page) (json.RawMessage, bool) {
	data, ok := p.Query("IssueViewerViewQuery")
	if !ok {
		_, data, ok = p.FirstQuery()
	}
	if !ok {
		return nil, false
	}
	var q struct {
		Repository struct {
			Issue json.RawMessage `json:"issue"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(data, &q); err != nil || len(q.Repository.Issue) == 0 {
		return nil, false
	}
	return q.Repository.Issue, true
}

// relayIssue is the GraphQL issue node, modelled field for field. Everything
// not named here lands in Extra, which is how the day GitHub adds a field
// becomes a failing test rather than a year of silently dropping it.
type relayIssue struct {
	ID          string `json:"id"`
	Number      int    `json:"number"`
	DatabaseID  *int   `json:"databaseId"`
	Title       string `json:"title"`
	TitleHTML   string `json:"titleHTML"`
	URL         string `json:"url"`
	State       string `json:"state"`
	StateReason string `json:"stateReason"`
	Locked      bool   `json:"locked"`
	IsPinned    bool   `json:"isPinned"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	Body        string `json:"body"`
	BodyHTML    string `json:"bodyHTML"`

	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
		DatabaseID    *int   `json:"databaseId"`
	} `json:"repository"`

	Author    relayActor `json:"author"`
	IssueType *struct {
		Name string `json:"name"`
	} `json:"issueType"`
	DuplicateOf *struct {
		URL string `json:"url"`
	} `json:"duplicateOf"`

	Labels struct {
		Edges []struct {
			Node relayLabel `json:"node"`
		} `json:"edges"`
	} `json:"labels"`

	Milestone *relayMilestone `json:"milestone"`

	AssignedActors struct {
		Nodes []relayActor `json:"nodes"`
	} `json:"assignedActors"`

	ReactionGroups []relayReactionGroup `json:"reactionGroups"`

	SubIssuesSummary *struct {
		Total     *int `json:"total"`
		Completed *int `json:"completed"`
	} `json:"subIssuesSummary"`

	LinkedPullRequests struct {
		Nodes []struct {
			URL string `json:"url"`
		} `json:"nodes"`
	} `json:"linkedPullRequests"`
	ClosedByPullRequestsReferences struct {
		Nodes []struct {
			URL string `json:"url"`
		} `json:"nodes"`
	} `json:"closedByPullRequestsReferences"`

	ProjectItems struct {
		Edges []struct {
			Node struct {
				Project struct {
					Title string `json:"title"`
					URL   string `json:"url"`
				} `json:"project"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"projectItems"`

	FrontTimelineItems relayTimeline `json:"frontTimelineItems"`
	BackTimelineItems  relayTimeline `json:"backTimelineItems"`
}

type relayActor struct {
	Typename   string `json:"__typename"`
	Login      string `json:"login"`
	Name       string `json:"name"`
	ID         string `json:"id"`
	AvatarURL  string `json:"avatarUrl"`
	ProfileURL string `json:"profileUrl"`
}

func (a relayActor) actor() Actor {
	if a.Login == "" {
		return Actor{}
	}
	out := actor(a.Login)
	out.Name = a.Name
	out.Type = a.Typename
	out.NodeID = a.ID
	out.AvatarURL = a.AvatarURL
	if a.ProfileURL != "" {
		out.URL = a.ProfileURL
	}
	return out
}

type relayLabel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

func (l relayLabel) label() Label {
	return Label{
		Name:        l.Name,
		Color:       l.Color,
		Description: l.Description,
		URL:         l.URL,
		NodeID:      l.ID,
	}
}

type relayMilestone struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Number   *int     `json:"number"`
	Closed   bool     `json:"closed"`
	DueOn    string   `json:"dueOn"`
	ClosedAt string   `json:"closedAt"`
	Progress *float64 `json:"progressPercentage"`
	URL      string   `json:"url"`
}

func (m *relayMilestone) milestone() *Milestone {
	if m == nil || m.Title == "" {
		return nil
	}
	out := &Milestone{
		Title:    m.Title,
		Number:   m.Number,
		Closed:   m.Closed,
		DueOn:    parseTime(m.DueOn),
		ClosedAt: parseTime(m.ClosedAt),
		Progress: m.Progress,
		URL:      m.URL,
	}
	// The milestone number is not in the node, but it is the last segment of
	// the URL, and a milestone without a number is awkward to join on.
	if out.Number == nil {
		if i := strings.LastIndex(m.URL, "/"); i >= 0 {
			if n, err := strconv.Atoi(m.URL[i+1:]); err == nil {
				out.Number = intp(n)
			}
		}
	}
	return out
}

type relayReactionGroup struct {
	Content  string `json:"content"`
	Reactors struct {
		TotalCount int `json:"totalCount"`
	} `json:"reactors"`
}

// reactions drops the zero groups. All eight always arrive, most of them empty,
// and eight zeroes on every record is noise that would bury the real ones.
func reactions(groups []relayReactionGroup) []Reaction {
	var out []Reaction
	for _, g := range groups {
		if g.Reactors.TotalCount == 0 {
			continue
		}
		out = append(out, Reaction{Content: g.Content, Count: g.Reactors.TotalCount})
	}
	return out
}

type relayTimeline struct {
	TotalCount *int `json:"totalCount"`
	PageInfo   struct {
		HasNextPage     bool   `json:"hasNextPage"`
		HasPreviousPage bool   `json:"hasPreviousPage"`
		EndCursor       string `json:"endCursor"`
		StartCursor     string `json:"startCursor"`
	} `json:"pageInfo"`
	Edges []struct {
		Cursor string          `json:"cursor"`
		Node   json.RawMessage `json:"node"`
	} `json:"edges"`
}

func decodeRelayIssue(repo string, raw json.RawMessage) (*Issue, error) {
	var v relayIssue
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, badPayload(repo, err)
	}
	if v.Number == 0 {
		return nil, structureChanged(repo)
	}
	if v.Repository.NameWithOwner != "" {
		// A transferred issue reports its new home here, and the new home is
		// the truth. The caller's repo argument is where the redirect started.
		repo = v.Repository.NameWithOwner
	}

	iss := &Issue{}
	t := &iss.Thread
	t.Repo = repo
	t.Number = v.Number
	t.setIdentity(KindIssue, repo+"#"+strconv.Itoa(v.Number))
	if v.URL != "" {
		t.URL = v.URL
	}
	t.Title = v.Title
	if v.TitleHTML != v.Title {
		t.TitleHTML = v.TitleHTML
	}
	t.State = enum(v.State)
	t.StateReason = enum(v.StateReason)
	t.Body = v.Body
	t.BodyHTML = v.BodyHTML
	t.Author = v.Author.actor()
	t.Locked = v.Locked
	t.IsPinned = v.IsPinned
	t.CreatedAt = parseTime(v.CreatedAt)
	t.UpdatedAt = parseTime(v.UpdatedAt)
	t.NodeID = v.ID
	t.DatabaseID = v.DatabaseID
	for _, e := range v.Labels.Edges {
		t.Labels = append(t.Labels, e.Node.label())
	}
	t.Milestone = v.Milestone.milestone()
	for _, a := range v.AssignedActors.Nodes {
		t.Assignees = append(t.Assignees, a.actor())
	}
	t.Reactions = reactions(v.ReactionGroups)
	// CommentCount stays nil deliberately. The only count on this payload is
	// the timeline total, and a timeline counts labellings and assignments as
	// well as comments, so reporting it as the comment count would be wrong by
	// however much housekeeping the thread has had. Search states the real one.
	//
	// ClosedAt is not on the issue node at all. It is a ClosedEvent on the
	// timeline, so it gets filled from there when the timeline carries one.
	t.ClosedAt = closedAtFrom(v.FrontTimelineItems, v.BackTimelineItems)

	if v.IssueType != nil {
		iss.IssueType = v.IssueType.Name
	}
	if v.DuplicateOf != nil {
		iss.DuplicateOf = v.DuplicateOf.URL
	}
	if s := v.SubIssuesSummary; s != nil {
		iss.SubIssueTotal = s.Total
		iss.SubIssueDone = s.Completed
	}
	for _, n := range v.LinkedPullRequests.Nodes {
		iss.LinkedPRs = append(iss.LinkedPRs, n.URL)
	}
	for _, n := range v.ClosedByPullRequestsReferences.Nodes {
		iss.ClosedByPRs = append(iss.ClosedByPRs, n.URL)
	}
	for _, e := range v.ProjectItems.Edges {
		if e.Node.Project.Title != "" {
			iss.ProjectItems = append(iss.ProjectItems, e.Node.Project.Title)
		}
	}

	t.addExtra("issue", decodeExtra(raw, &v, issueSkips...))
	return iss, nil
}

// issueSkips are the keys the issue node carries that a logged-out reader has
// no use for. Every one of them is a permission or a draft-state flag scoped to
// the viewer, and this tool has no viewer.
var issueSkips = []string{
	// Session state. All of these are false for us by definition.
	"viewer*",
	// Relay type discriminators. They exist so the client can pick a fragment,
	// and __typename is already read where it decides something.
	"__is*", "__typename",
	// Editing scaffolding: the body hash the editor posts back, the suggestion
	// drafts, the agent handoff, and the pinned-comment slot. None of them are
	// facts about the issue.
	"bodyVersion", "pendingSuggestions", "agentAssignments", "pinnedIssueComment",
	// Custom issue fields, which are private-repository projects plumbing and
	// arrive empty on every public issue seen so far.
	"issueFieldValues",
}

// closedAtFrom digs the close date out of the timeline, because the issue node
// does not carry one. The back half is searched first: a close is usually the
// last thing that happened, and the back half is the end of the thread.
func closedAtFrom(front, back relayTimeline) *time.Time {
	for _, tl := range []relayTimeline{back, front} {
		for i := len(tl.Edges) - 1; i >= 0; i-- {
			var n struct {
				Typename  string `json:"__typename"`
				CreatedAt string `json:"createdAt"`
			}
			if err := json.Unmarshal(tl.Edges[i].Node, &n); err != nil {
				continue
			}
			if n.Typename == "ClosedEvent" {
				return parseTime(n.CreatedAt)
			}
		}
	}
	return nil
}

// --- the timeline ---

// Timeline emits the events the issue page carries.
//
// It is deliberately not a pager. The page preloads the first fifteen events
// and, on a long thread, the last few; walking past those needs the GraphQL
// endpoint, which needs a session. So this returns what the page had and
// Truncated says whether there is more, rather than pretending to a
// completeness it cannot deliver.
func (c *Client) Timeline(ctx context.Context, repo string, number int, limit int, emit func(TimelineItem) error) error {
	if _, _, ok := SplitRepo(repo); !ok {
		return usageBadID("repository", repo, "owner/name")
	}
	res, err := c.GetHTML(ctx, threadURL(repo, "issues", number))
	if err != nil {
		return err
	}
	p := page.Extract(res.FinalURL, res.Body)
	raw, ok := issueNode(p)
	if !ok {
		return structureChanged(repo + "#" + strconv.Itoa(number))
	}
	var v relayIssue
	if err := json.Unmarshal(raw, &v); err != nil {
		return badPayload(repo, err)
	}

	thread := repo + "#" + strconv.Itoa(number)
	seen := 0
	for _, tl := range []relayTimeline{v.FrontTimelineItems, v.BackTimelineItems} {
		for _, e := range tl.Edges {
			item, ok := decodeTimelineItem(thread, e.Cursor, e.Node)
			if !ok {
				continue
			}
			item.addSource(res.FinalURL)
			if err := emit(item); err != nil {
				return err
			}
			seen++
			if limit > 0 && seen >= limit {
				return nil
			}
		}
	}
	return nil
}

// timelineNode is the union of every field the timeline typenames use. Relay
// sends a different shape per __typename, and the fields do not collide, so one
// struct decodes all of them and the typename decides which half is filled.
type timelineNode struct {
	Typename        string     `json:"__typename"`
	DatabaseID      *int       `json:"databaseId"`
	ID              string     `json:"id"`
	CreatedAt       string     `json:"createdAt"`
	Actor           relayActor `json:"actor"`
	Author          relayActor `json:"author"`
	Body            string     `json:"body"`
	BodyHTML        string     `json:"bodyHTML"`
	URL             string     `json:"url"`
	Association     string     `json:"authorAssociation"`
	IsHidden        bool       `json:"isHidden"`
	MinimizedReason string     `json:"minimizedReason"`
	CreatedViaEmail bool       `json:"createdViaEmail"`
	LastEditedAt    string     `json:"lastEditedAt"`

	Label     *relayLabel          `json:"label"`
	Milestone *relayMilestone      `json:"milestone"`
	Assignee  *relayActor          `json:"assignee"`
	Reactions []relayReactionGroup `json:"reactionGroups"`

	PreviousTitle string `json:"previousTitle"`
	CurrentTitle  string `json:"currentTitle"`

	Commit *struct {
		OID string `json:"oid"`
		URL string `json:"url"`
	} `json:"commit"`
	Source *struct {
		URL string `json:"url"`
	} `json:"source"`
	Subject *struct {
		URL string `json:"url"`
	} `json:"subject"`
}

func decodeTimelineItem(thread, cursor string, raw json.RawMessage) (TimelineItem, bool) {
	var n timelineNode
	if err := json.Unmarshal(raw, &n); err != nil {
		return TimelineItem{}, false
	}
	if n.Typename == "" {
		return TimelineItem{}, false
	}

	item := TimelineItem{Thread: thread, Type: snakeCase(n.Typename), Cursor: cursor}
	id := thread + ":" + item.Type
	if n.DatabaseID != nil {
		id = thread + ":" + strconv.Itoa(*n.DatabaseID)
	}
	item.setIdentity(KindIssue, id)
	// setIdentity built a URL from the id, which is wrong for an event: it is
	// not an issue. The node's own URL is right when it has one, and no URL is
	// better than one that resolves somewhere else.
	item.URL = n.URL

	item.CreatedAt = parseTime(n.CreatedAt)
	if a := n.Actor.actor(); a.Login != "" {
		item.Actor = &a
	} else if a := n.Author.actor(); a.Login != "" {
		item.Actor = &a
	}
	item.Body = n.Body
	item.BodyHTML = n.BodyHTML
	if n.Label != nil {
		l := n.Label.label()
		item.Label = &l
	}
	item.Milestone = n.Milestone.milestone()
	if n.Assignee != nil {
		a := n.Assignee.actor()
		item.Assignee = &a
	}
	item.FromTitle = n.PreviousTitle
	item.ToTitle = n.CurrentTitle
	if n.Commit != nil {
		item.Commit = n.Commit.OID
	}
	switch {
	case n.Source != nil && n.Source.URL != "":
		item.Source = n.Source.URL
	case n.Subject != nil:
		item.Source = n.Subject.URL
	}
	item.Reactions = reactions(n.Reactions)
	item.Minimized = n.IsHidden
	item.MinimizedReason = n.MinimizedReason
	item.CreatedViaEmail = n.CreatedViaEmail
	item.LastEditedAt = parseTime(n.LastEditedAt)

	item.addExtra("timeline", decodeExtra(raw, &n, timelineSkips...))
	return item, true
}

// timelineSkips is the same reasoning as issueSkips, plus the back-references
// the node carries to its own thread and repository, which the caller already
// has because it asked for them.
var timelineSkips = []string{
	"viewer*", "__is*",
	"bodyVersion", "issue", "pullRequest", "repository",
	// Sponsorship badges, spam flags, and the app that posted the comment.
	// They are display decisions, not events.
	"authorToRepoOwnerSponsorship", "showSpammyBadge", "viaApp",
	"lastUserContentEdit", "pinnedBy", "intent", "willCloseTarget",
	"willCloseSubject", "innerSource", "referencedAt",
}

// snakeCase turns a GraphQL typename into the wire form the records use:
// IssueComment becomes issue_comment, CrossReferencedEvent becomes
// cross_referenced_event.
func snakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r - 'A' + 'a')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// --- pull requests ---

// PullRequest reads one pull request.
//
// This one takes the HTML page rather than the route JSON, and the reason is
// worth writing down. Asking a React route for JSON returns the props for that
// route only, as a delta against the layout that is already mounted in a real
// browser. /pull/{n} with Accept: application/json is a kilobyte of websocket
// channel tokens: no title, no author, no merge state. The layout route that
// holds those lives in the page's embedded payload and nowhere else, so the
// page is what we fetch. It is about 165 KB on the wire and that is the price.
//
// One fetch then covers everything a logged-out client can see:
//
//	pullRequestsLayoutRoute        title, author, refs, head sha, merge metadata
//	pullRequestsConversationsRoute node id and the lock flag
//	the sidebar markup             labels
//	og:description                 the body, truncated to 200 characters
//
// The body really is a snippet. GitHub renders the conversation client-side and
// serves none of it to a logged-out client, so 200 characters of open graph
// text is the whole of what is reachable. _via records body: og for it, and the
// hovercard fragment is the fallback when the page has no open graph body.
func (c *Client) PullRequest(ctx context.Context, repo string, number int) (*PullRequest, error) {
	if _, _, ok := SplitRepo(repo); !ok {
		return nil, usageBadID("repository", repo, "owner/name")
	}
	if number <= 0 {
		return nil, usageBadID("pull request number", strconv.Itoa(number), "a positive integer")
	}

	url := threadURL(repo, "pull", number)
	res, err := c.GetHTML(ctx, url)
	if err != nil {
		return nil, err
	}
	p := page.Extract(res.FinalURL, res.Body)
	raw, ok := p.Route("pullRequestsLayoutRoute")
	if !ok {
		return nil, structureChanged(repo + "#" + strconv.Itoa(number))
	}

	pr, err := decodePullRoute(repo, number, raw)
	if err != nil {
		return nil, err
	}
	pr.addSource(res.FinalURL)
	if p.Canonical != "" {
		pr.URL = p.Canonical
	}
	readPullPage(p, pr)

	// The hovercard is the fallback for the body and for the labels, and it is
	// only worth a second request when the page gave neither. A failure here
	// keeps the record: a pull request with no body text is still useful.
	if pr.Body == "" && len(pr.Labels) == 0 {
		if hc, err := c.hovercard(ctx, url); err == nil {
			hc.applyTo(pr)
			pr.addSource(url + "/hovercard")
			recordVia(&pr.Base, "body", "xhr")
		}
	}
	return pr, nil
}

// readPullPage takes the three facts the layout route does not carry off the
// rest of the page: the node id and the lock flag from the conversation route,
// the labels from the server-rendered sidebar, and the body from open graph.
func readPullPage(p *page.Page, pr *PullRequest) {
	if raw, ok := p.Route("pullRequestsConversationsRoute"); ok {
		var conv struct {
			ID     string `json:"id"`
			Locked bool   `json:"locked"`
		}
		if json.Unmarshal(raw, &conv) == nil {
			if pr.NodeID == "" {
				pr.NodeID = conv.ID
			}
			pr.Locked = conv.Locked
		}
	}

	if doc := p.Doc(); doc != nil {
		seen := map[string]bool{}
		for _, n := range page.FindAll(doc, page.HovercardLabel) {
			// The sidebar renders each label twice, once for the wide layout
			// and once for the narrow one, so the same name comes back twice.
			name := page.Attr(n, "data-name")
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			pr.Labels = append(pr.Labels, Label{Name: name})
		}
	}

	// og:description is the body on a thread page and the repository blurb on a
	// page with no body, and meta description is always the repository blurb.
	// Comparing the two is how you tell an empty body from a real one.
	if body := p.MetaContent("og:description"); body != "" && body != p.MetaContent("description") {
		pr.Body = body
		recordVia(&pr.Base, "body", "og")
	}
}

type pullRoute struct {
	PullRequest struct {
		ID          *int   `json:"id"`
		RelayID     string `json:"relayId"`
		Number      int    `json:"number"`
		Title       string `json:"title"`
		TitleHTML   string `json:"titleHtml"`
		State       string `json:"state"`
		BaseBranch  string `json:"baseBranch"`
		HeadBranch  string `json:"headBranch"`
		HeadSha     string `json:"headSha"`
		HeadRepo    string `json:"headRepositoryName"`
		HeadOwner   string `json:"headRepositoryOwnerLogin"`
		CommitCount *int   `json:"commitsCount"`
		CreatedTime string `json:"createdTime"`
		ClosedTime  string `json:"closedTime"`
		MergedTime  string `json:"mergedTime"`
		MergedBy    string `json:"mergedBy"`
		MergedName  string `json:"mergedByName"`
		MergedAvat  string `json:"mergedByAvatarUrl"`
		Author      struct {
			Login       string `json:"login"`
			DisplayName string `json:"displayName"`
			AvatarURL   string `json:"avatarUrl"`
		} `json:"author"`
	} `json:"pullRequest"`
	Repository struct {
		ID            *int   `json:"id"`
		Name          string `json:"name"`
		OwnerLogin    string `json:"ownerLogin"`
		DefaultBranch string `json:"defaultBranch"`
	} `json:"repository"`
}

func decodePullRoute(repo string, number int, raw json.RawMessage) (*PullRequest, error) {
	var v pullRoute
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, badPayload(repo, err)
	}
	p := v.PullRequest
	if p.Number == 0 {
		return nil, structureChanged(repo + "#" + strconv.Itoa(number))
	}
	if v.Repository.OwnerLogin != "" && v.Repository.Name != "" {
		repo = v.Repository.OwnerLogin + "/" + v.Repository.Name
	}

	pr := &PullRequest{}
	t := &pr.Thread
	t.Repo = repo
	t.Number = p.Number
	t.setIdentity(KindPR, repo+"#"+strconv.Itoa(p.Number))
	t.Title = p.Title
	if p.TitleHTML != p.Title {
		t.TitleHTML = p.TitleHTML
	}
	t.State = enum(p.State)
	t.CreatedAt = parseTime(p.CreatedTime)
	t.ClosedAt = parseTime(p.ClosedTime)
	t.NodeID = p.RelayID
	t.DatabaseID = p.ID
	t.Author = actor(p.Author.Login)
	t.Author.Name = p.Author.DisplayName
	t.Author.AvatarURL = p.Author.AvatarURL

	pr.BaseRef = p.BaseBranch
	pr.HeadRef = p.HeadBranch
	if p.HeadOwner != "" && p.HeadOwner+"/"+p.HeadRepo != repo {
		// A fork's head is only unambiguous with the owner on it, and the
		// owner is the whole point of the field on a cross-repository PR.
		pr.HeadRef = p.HeadOwner + ":" + p.HeadBranch
	}
	pr.HeadOID = p.HeadSha
	pr.CommitCount = p.CommitCount
	pr.MergedAt = parseTime(p.MergedTime)
	pr.Merged = pr.MergedAt != nil || enum(p.State) == "merged"
	if p.MergedBy != "" {
		a := actor(p.MergedBy)
		a.Name = p.MergedName
		a.AvatarURL = p.MergedAvat
		pr.MergedBy = &a
	}
	pr.IsDraft = enum(p.State) == "draft"

	t.addExtra("pullRequestsLayoutRoute", decodeExtra(raw, &v, pullSkips...))
	return pr, nil
}

// pullSkips: the pull request route is mostly front-end plumbing. What is left
// after these is the twenty-odd fields decoded above.
var pullSkips = []string{
	// Live-update websocket tokens. They expire in minutes.
	"aliveChannel", "aliveChannels", "mergeboxChannels", "markAsReadChannel",
	// Feature flags, banners, and view state.
	"featureStates", "bannersData", "viewSettings", "viewerPendingReview",
	"mergeStatusButtonData", "user", "stack", "helpUrl",
	// urls and pageTitle restate what the record already computes.
	"urls", "pageTitle",
}

// --- hovercards ---

// hovercardData is the handful of facts the hovercard fragment carries that the
// pull request route does not.
type hovercardData struct {
	Body   string
	State  string
	Labels []Label
	Base   string
	Head   string
}

// hovercard fetches the XHR fragment behind a link's popover. Any thread URL
// plus /hovercard serves it, and it is the only keyless source for a pull
// request's body text.
func (c *Client) hovercard(ctx context.Context, threadURL string) (*hovercardData, error) {
	res, err := c.Get(ctx, threadURL+"/hovercard", SurfaceXHR)
	if err != nil {
		return nil, err
	}
	doc, err := html.Parse(strings.NewReader(string(res.Body)))
	if err != nil {
		return nil, wrapNetwork(threadURL, err)
	}

	hc := &hovercardData{}
	if n := page.Find(doc, page.HovercardBody); n != nil {
		hc.Body = page.Text(n)
	}
	if n := page.Find(doc, page.HovercardState); n != nil {
		// The title reads "Status: Merged". The word after the colon is the
		// state, and the element text is the same word, so either works and
		// the text is the one that survives a title rewording.
		hc.State = enum(page.Text(n))
	}
	for _, n := range page.FindAll(doc, page.HovercardLabel) {
		if name := page.Attr(n, "data-name"); name != "" {
			hc.Labels = append(hc.Labels, Label{Name: name})
		}
	}
	refs := page.FindAll(doc, page.HovercardRef)
	if len(refs) == 2 {
		// Rendered as "base <- head", in that order, and there is nothing else
		// on the card with this class.
		hc.Base = page.Text(refs[0])
		hc.Head = page.Text(refs[1])
	}
	return hc, nil
}

func (hc *hovercardData) applyTo(pr *PullRequest) {
	if pr.Body == "" {
		pr.Body = hc.Body
	}
	if pr.State == "" {
		pr.State = hc.State
	}
	if len(pr.Labels) == 0 {
		pr.Labels = hc.Labels
	}
	if pr.BaseRef == "" {
		pr.BaseRef = hc.Base
	}
	if pr.HeadRef == "" {
		pr.HeadRef = hc.Head
	}
}

// --- pull request commits ---

// PullCommits emits the commits on a pull request. The route serves them all in
// one response grouped by push, which is why there is no pager here.
func (c *Client) PullCommits(ctx context.Context, repo string, number int, limit int, emit func(Commit) error) error {
	if _, _, ok := SplitRepo(repo); !ok {
		return usageBadID("repository", repo, "owner/name")
	}
	var env struct {
		Payload struct {
			Route struct {
				CommitGroups []struct {
					Commits []routeCommit `json:"commits"`
				} `json:"commitGroups"`
			} `json:"pullRequestsCommitsRoute"`
		} `json:"payload"`
	}
	url := threadURL(repo, "pull", number) + "/commits"
	res, err := c.GetJSON(ctx, url, SurfaceRouteJSON, &env)
	if err != nil {
		return err
	}

	seen := 0
	for _, g := range env.Payload.Route.CommitGroups {
		for _, rc := range g.Commits {
			cm := rc.commit(repo)
			cm.addSource(res.FinalURL)
			if err := emit(cm); err != nil {
				return err
			}
			seen++
			if limit > 0 && seen >= limit {
				return nil
			}
		}
	}
	return nil
}

// routeCommit is the commit shape the pull request commits route uses. It is
// not the same shape as the repository commits route, which is why it is here
// and not in commit.go.
type routeCommit struct {
	OID           string       `json:"oid"`
	ShortMessage  string       `json:"shortMessage"`
	BodyHTML      string       `json:"bodyMessageHtml"`
	AuthoredDate  string       `json:"authoredDate"`
	CommittedDate string       `json:"committedDate"`
	Authors       []routeActor `json:"authors"`
	Committer     *routeActor  `json:"committer"`
}

type routeActor struct {
	Login       string `json:"login"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
	Path        string `json:"path"`
	IsGitHub    bool   `json:"isGitHub"`
}

func (a routeActor) actor() Actor {
	if a.Login == "" {
		// Commits from an address with no GitHub account have a display name
		// and nothing else. Dropping them would lose the authorship entirely.
		return Actor{Name: a.DisplayName, AvatarURL: a.AvatarURL}
	}
	out := actor(a.Login)
	out.Name = a.DisplayName
	out.AvatarURL = a.AvatarURL
	return out
}

func (rc routeCommit) commit(repo string) Commit {
	cm := Commit{Repo: repo, SHA: rc.OID}
	cm.setIdentity(KindCommit, repo+"@"+rc.OID)
	cm.Subject = rc.ShortMessage
	cm.BodyHTML = rc.BodyHTML
	cm.Body = stripTags(rc.BodyHTML)
	cm.AuthoredAt = parseTime(rc.AuthoredDate)
	cm.CommittedAt = parseTime(rc.CommittedDate)
	for _, a := range rc.Authors {
		cm.Authors = append(cm.Authors, a.actor())
	}
	if rc.Committer != nil {
		c := rc.Committer.actor()
		cm.Committer = &c
	}
	return cm
}

// --- discussions ---

// Discussion reads one discussion.
//
// Discussions never migrated to React, so there is no payload to decode. What
// there is instead is a schema.org QAPage block, which carries the body, the
// upvote count, and the accepted answer, and is the most reliable thing on the
// page because GitHub publishes it for search engines and therefore keeps it
// working. Everything the block does not have comes from the markup.
//
// repo may be owner/name or an organization login: organization-level
// discussions live at /orgs/{login}/discussions/{n} and the read follows the
// redirect either way, then takes the true repository off the sidebar.
func (c *Client) Discussion(ctx context.Context, repo string, number int) (*Discussion, error) {
	if number <= 0 {
		return nil, usageBadID("discussion number", strconv.Itoa(number), "a positive integer")
	}
	url := threadURL(repo, "discussions", number)
	if !strings.Contains(repo, "/") {
		url = BaseURL + "/orgs/" + repo + "/discussions/" + strconv.Itoa(number)
	}
	res, err := c.GetHTML(ctx, url)
	if err != nil {
		return nil, err
	}
	p := page.Extract(res.FinalURL, res.Body)
	doc := p.Doc()
	if doc == nil {
		return nil, structureChanged(url)
	}

	sidebar := page.Find(doc, page.DiscussionSidebar)
	if sidebar == nil {
		return nil, structureChanged(url)
	}

	d := &Discussion{}
	t := &d.Thread
	t.Repo = repoFromSidebar(sidebar, repo)
	t.Number = number
	t.setIdentity(KindDiscussion, t.Repo+"#"+strconv.Itoa(number))
	t.NodeID = page.Attr(sidebar, "data-gid")
	t.addSource(res.FinalURL)
	if p.Canonical != "" {
		t.URL = p.Canonical
	}

	readDiscussionHeader(d, doc)
	readDiscussionSidebar(d, sidebar)
	readDiscussionBody(d, doc)
	readQAPage(d, p)
	return d, nil
}

// repoFromSidebar reads the true repository off the sidebar's deferred-load
// URL, which is /{owner}/{repo}/discussions/{n}/sidebar even when the page was
// served from the /orgs/ path.
func repoFromSidebar(sidebar *html.Node, fallback string) string {
	u := page.Attr(sidebar, "data-url")
	parts := strings.Split(strings.Trim(u, "/"), "/")
	if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
		return parts[0] + "/" + parts[1]
	}
	return fallback
}

func readDiscussionHeader(d *Discussion, doc *html.Node) {
	if n := page.Find(doc, page.DiscussionTitle); n != nil {
		d.Title = page.Text(n)
	}
	for _, n := range page.FindAll(doc, page.DiscussionState) {
		title := page.Attr(n, "title")
		switch {
		case strings.HasPrefix(title, "Status: "):
			// "Status: Closed as resolved" is more than the state: it is the
			// state and the reason, and both are worth keeping.
			rest := strings.TrimPrefix(title, "Status: ")
			state, reason, found := strings.Cut(rest, " as ")
			d.State = enum(state)
			if found {
				d.StateReason = enum(reason)
			}
		case title == "Answered":
			d.IsAnswered = true
		}
	}
	if n := page.Find(doc, page.DiscussionAuthor); n != nil {
		d.Author = actorFromHref(page.Attr(n, "href"))
	}
	if n := page.Find(doc, page.RelTimeEl); n != nil {
		d.CreatedAt = parseTime(page.Attr(n, "datetime"))
	}
	if n := page.Find(doc, page.DiscussionUpvote); n != nil {
		// aria-label is "Upvote: 12". The number in the button text is the
		// same, but the label survives the button being replaced by a form.
		if _, after, ok := strings.Cut(page.Attr(n, "aria-label"), ":"); ok {
			if v, err := strconv.Atoi(strings.TrimSpace(after)); err == nil {
				d.Upvotes = intp(v)
			}
		}
	}
	if n := page.Find(doc, page.DiscussionAnswerLink); n != nil {
		d.IsAnswered = true
		// The answer author is the bold link right after the "by" that follows
		// the answer link, and the answer link's parent holds both.
		if parent := n.Parent; parent != nil {
			for _, a := range page.FindAll(parent, page.ProfileAnyLink) {
				if who := actorFromHref(page.Attr(a, "href")); who.Login != "" {
					d.AnswerAuthor = &who
					break
				}
			}
		}
	}
}

func readDiscussionSidebar(d *Discussion, sidebar *html.Node) {
	if n := page.Find(sidebar, page.DiscussionCategory); n != nil {
		d.Category = page.Text(n)
	}
	for _, n := range page.FindAll(sidebar, page.DiscussionLabel) {
		d.Labels = append(d.Labels, Label{
			Name: page.Attr(n, "data-name"),
			URL:  absolute(page.Attr(n, "href")),
		})
	}
}

func readDiscussionBody(d *Discussion, doc *html.Node) {
	// The first comment container is the discussion body itself; the ones after
	// it are replies. The container carries the node id, which is how the body
	// is told apart from a reply that happens to come first in the markup.
	for _, n := range page.FindAll(doc, page.DiscussionComment) {
		gid := page.Attr(n, "data-gid")
		if !strings.Contains(decodeGID(gid), "Discussion") ||
			strings.Contains(decodeGID(gid), "DiscussionComment") {
			continue
		}
		if body := page.Find(n, page.DiscussionBody); body != nil {
			d.BodyHTML = page.OuterHTML(body)
			d.Body = page.BlockText(body)
		}
		return
	}
}

// readQAPage takes what the schema.org block says and lets it win where the two
// sources disagree, because it is the one GitHub maintains for search engines
// and the markup is the one that gets restyled.
func readQAPage(d *Discussion, p *page.Page) {
	for _, raw := range p.LinkedData {
		var v struct {
			Type       string `json:"@type"`
			MainEntity struct {
				Type           string `json:"@type"`
				Name           string `json:"name"`
				Text           string `json:"text"`
				UpvoteCount    *int   `json:"upvoteCount"`
				AnswerCount    *int   `json:"answerCount"`
				AcceptedAnswer *struct {
					Text string `json:"text"`
				} `json:"acceptedAnswer"`
			} `json:"mainEntity"`
		}
		if err := json.Unmarshal(raw, &v); err != nil || v.Type != "QAPage" {
			continue
		}
		e := v.MainEntity
		if e.Name != "" {
			d.Title = e.Name
		}
		if e.Text != "" {
			// Both halves move together. Letting the markup win here while the
			// prose still came off the DOM would leave a record whose two body
			// fields describe different text.
			d.BodyHTML = e.Text
			d.Body = page.FragmentText(e.Text)
		}
		if e.UpvoteCount != nil {
			d.Upvotes = e.UpvoteCount
		}
		if e.AnswerCount != nil {
			d.CommentCount = e.AnswerCount
		}
		if e.AcceptedAnswer != nil {
			d.IsAnswered = true
		}
		recordVia(&d.Base, "body_html", "ld-json")
		return
	}
}

// --- small shared helpers ---

// enum lowercases a GraphQL enum so that a state means the same string whatever
// surface it arrived on: the Relay node says CLOSED, the route says MERGED, and
// search says open.
func enum(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// decodeGID unpacks a base64 global node id far enough to read the type off the
// front of it. "MDEwOkRpc2N1c3Npb24yNDQyOA==" decodes to "010:Discussion24428",
// which is how a discussion body is told apart from a reply without relying on
// document order. A gid that does not decode returns empty, and the caller
// treats that as "not the type I wanted", which is the safe direction.
func decodeGID(gid string) string {
	if gid == "" {
		return ""
	}
	b, err := base64.StdEncoding.DecodeString(gid)
	if err != nil {
		b, err = base64.RawURLEncoding.DecodeString(gid)
		if err != nil {
			return ""
		}
	}
	return string(b)
}

// absolute turns a site-relative href into a full URL and leaves an already
// absolute one alone.
func absolute(href string) string {
	switch {
	case href == "":
		return ""
	case strings.HasPrefix(href, "http"):
		return href
	case strings.HasPrefix(href, "/"):
		return BaseURL + href
	default:
		return BaseURL + "/" + href
	}
}
