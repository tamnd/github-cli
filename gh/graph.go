package gh

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// graph.go turns records into triples. github.com is already a knowledge graph:
// a repository names its owner, its topics, its licence, and the repository it
// was forked from; a pull request names the issue it closes and the branch it
// targets; a commit names its parents. This file reads those declarations off a
// record and emits them as typed edges.
//
// It is pure. No network, no client, no ordering dependency, which is what makes
// the whole graph plane testable against a fixture and what makes
// `github edges golang/go#1 --min-trust id` answer without a request.

// Node is one entity. It is deliberately thin: the label and the two addresses
// and nothing else, because the full record is one `github get` away by URI and
// duplicating it here would make a crawl of ten thousand nodes unprintable.
type Node struct {
	URI   string `json:"uri"             table:"uri"`
	Kind  string `json:"kind"            table:"kind"`
	ID    string `json:"id"              table:"id"`
	Label string `json:"label,omitempty" table:"label,truncate"`
	URL   string `json:"url,omitempty"   table:"url,url"`
}

// Edge is one directed, typed relation between two entities.
//
// There is no inverse flag. Every predicate has exactly one direction, and
// where the inverse is what you want, the edge is emitted with the other node as
// its subject rather than with a flag saying to read it backwards.
type Edge struct {
	Subject   string     `json:"subject"          table:"subject"`
	Predicate string     `json:"predicate"        table:"predicate"`
	Object    string     `json:"object"           table:"object"`
	Source    string     `json:"source"           table:"source"`
	Weight    *int       `json:"weight,omitempty" table:"weight"`
	At        *time.Time `json:"at,omitempty"     table:"-"`
}

// Fact is a literal statement about a node: a star count, a description, a
// timestamp.
//
// It is a separate type from Edge on purpose. Edge.Object is a URI and every
// consumer of the graph is entitled to treat it as one, so putting "12000" in
// that field to carry a star count would break each of them for the sake of
// saving a struct. RDF emits both; `github edges` emits only edges, which is why
// its output reads as relations rather than as a flattened record.
type Fact struct {
	Subject   string `json:"subject"            table:"subject"`
	Predicate string `json:"predicate"          table:"predicate"`
	Value     string `json:"value"              table:"value,truncate"`
	Datatype  string `json:"datatype,omitempty" table:"-"`
}

// The five extraction rules, in descending order of trust. Every edge carries
// the one that produced it, which is the field a consumer uses to decide how
// much to believe.
const (
	// SrcID is derived from the id structure alone. No fetch, always correct.
	SrcID = "id"
	// SrcPayload is an explicit reference in a JSON payload or a Relay result.
	SrcPayload = "payload"
	// SrcFeed is an explicit reference in an Atom feed.
	SrcFeed = "feed"
	// SrcHTML was parsed out of rendered markup with a selector. Good, and it
	// degrades to a missing edge rather than a wrong one when a template moves.
	SrcHTML = "html"
	// SrcText is a pattern matched in free text: #42, a bare SHA. Heuristic, and
	// dropped by the default --min-trust.
	SrcText = "text"
)

// trustRank orders the rules. Higher is more trustworthy.
var trustRank = map[string]int{
	SrcID:      4,
	SrcPayload: 3,
	SrcFeed:    2,
	SrcHTML:    1,
	SrcText:    0,
}

// DefaultMinTrust keeps everything except free-text guesses.
const DefaultMinTrust = SrcHTML

// TrustAtLeast reports whether a source meets a floor. An unknown floor lets
// everything through rather than silently dropping the whole graph, and an
// unknown source is treated as the weakest thing there is.
func TrustAtLeast(source, min string) bool {
	floor, ok := trustRank[min]
	if !ok {
		return true
	}
	return trustRank[source] >= floor
}

// TrustLevels is the accepted set, for help text and for validation.
var TrustLevels = []string{SrcID, SrcPayload, SrcFeed, SrcHTML, SrcText}

// The predicate vocabulary. This is the complete set: an edge this tool emits
// has its predicate here, and adding a relation means adding a constant first.
const (
	// Ownership and membership.
	PredOwnedBy          = "ownedBy"
	PredMemberOf         = "memberOf"
	PredPartOf           = "partOf"
	PredBelongsToPackage = "belongsToPackage"

	// Derivation. The edges that make a graph worth walking.
	PredForkOf     = "forkOf"
	PredTemplateOf = "templateOf"
	PredMirrorOf   = "mirrorOf"
	PredDependsOn  = "dependsOn"
	PredUsedBy     = "usedBy"

	// Authorship and activity.
	PredAuthoredBy          = "authoredBy"
	PredCommittedBy         = "committedBy"
	PredContributedTo       = "contributedTo"
	PredAssignedTo          = "assignedTo"
	PredReviewedBy          = "reviewedBy"
	PredReviewRequestedFrom = "reviewRequestedFrom"
	PredMergedBy            = "mergedBy"

	// Reference.
	PredReferences    = "references"
	PredCloses        = "closes"
	PredClosedBy      = "closedBy"
	PredDuplicateOf   = "duplicateOf"
	PredSubIssueOf    = "subIssueOf"
	PredLinkedTo      = "linkedTo"
	PredTargetsBranch = "targetsBranch"
	PredFromBranch    = "fromBranch"
	PredPointsAt      = "pointsAt"
	PredParentOf      = "parentOf"

	// Classification.
	PredHasTopic      = "hasTopic"
	PredHasLabel      = "hasLabel"
	PredInMilestone   = "inMilestone"
	PredWrittenIn     = "writtenIn"
	PredLicensedUnder = "licensedUnder"
	PredRelatedTopic  = "relatedTopic"

	// Social. Opt-in everywhere, because the star list of a popular repository
	// is thousands of pages and nobody wants that by accident.
	PredStarredBy   = "starredBy"
	PredFollows     = "follows"
	PredSponsors    = "sponsors"
	PredReactedWith = "reactedWith"
)

// The literal predicates. These name Fact rows rather than edges.
const (
	FactName        = "name"
	FactDescription = "description"
	FactHomepage    = "homepage"
	FactCreated     = "created"
	FactUpdated     = "updated"
	FactStars       = "stars"
	FactForks       = "forks"
	FactWatchers    = "watchers"
	FactCommits     = "commits"
	FactURI         = "uri"
	FactAvatar      = "avatar"
	FactState       = "state"
	FactCount       = "count"
)

// SocialPredicates are the ones a command has to be asked for by name.
var SocialPredicates = []string{PredStarredBy, PredFollows, PredSponsors, PredReactedWith}

// DefaultFollow is the crawler's follow set. It deliberately excludes
// references, starredBy, follows, dependsOn, and usedBy: those five turn a
// bounded walk into an unbounded one, and each has to be asked for by name.
var DefaultFollow = []string{PredPartOf, PredOwnedBy, PredForkOf, PredHasTopic, PredAuthoredBy}

// LiteralPredicates are the two whose object is a bare string rather than a
// URI, because a language and a licence are not github.com entities. RDF gives
// them synthetic IRIs in the gh: namespace; `github edges` prints them as they
// are written on the page.
var LiteralPredicates = map[string]bool{
	PredWrittenIn:     true,
	PredLicensedUnder: true,
	PredReactedWith:   true,
}

// --- the builder ---

// builder accumulates one node with its edges and facts while an extractor
// walks a record.
type builder struct {
	node  Node
	edges []Edge
	facts []Fact
}

// start sets the node. Every extractor calls it first, and nothing is emitted
// for a record whose identity did not resolve.
func (b *builder) start(kind, id, label, url string) {
	if id == "" {
		return
	}
	if label == "" {
		label = id
	}
	if url == "" {
		if u, err := Locate(kind, id); err == nil {
			url = u
		}
	}
	b.node = Node{URI: URI(kind, id), Kind: kind, ID: id, Label: label, URL: url}
}

// to emits an edge from this node to another entity named by kind and id.
func (b *builder) to(pred, objKind, objID, source string) {
	if objID == "" {
		return
	}
	b.toURI(pred, URI(objKind, objID), source)
}

// toURI is to for an object whose URI is already built.
func (b *builder) toURI(pred, objURI, source string) {
	if b.node.URI == "" || objURI == "" {
		return
	}
	b.edges = append(b.edges, Edge{Subject: b.node.URI, Predicate: pred, Object: objURI, Source: source})
}

// toURL emits an edge to whatever a github.com URL names. Relay results carry
// links rather than ids for linked pull requests, duplicates, and cross
// references, and classification is exactly the function that turns one into
// the other.
func (b *builder) toURL(pred, rawURL, source string) {
	if uri := uriOfURL(rawURL); uri != "" {
		b.toURI(pred, uri, source)
	}
}

// raw emits an edge whose object is a bare string rather than a URI: a
// language, a licence, a reaction.
func (b *builder) raw(pred, value, source string) {
	if b.node.URI == "" || value == "" {
		return
	}
	b.edges = append(b.edges, Edge{Subject: b.node.URI, Predicate: pred, Object: value, Source: source})
}

// from emits an edge whose subject is not this node. A contributor edge points
// at the repository rather than away from it, and inverting it to make this node
// the subject would be a lie about which way the relation runs.
func (b *builder) from(subjURI, pred, objURI, source string) {
	if subjURI == "" || objURI == "" {
		return
	}
	b.edges = append(b.edges, Edge{Subject: subjURI, Predicate: pred, Object: objURI, Source: source})
}

// weigh attaches a count to the last edge appended. It is separate from the
// emitters so the common case stays a one-liner.
func (b *builder) weigh(n *int) {
	if n == nil || len(b.edges) == 0 {
		return
	}
	b.edges[len(b.edges)-1].Weight = n
}

// when attaches a time to the last edge appended.
func (b *builder) when(t *time.Time) {
	if t == nil || len(b.edges) == 0 {
		return
	}
	b.edges[len(b.edges)-1].At = t
}

// fact records a literal. An empty value is skipped, because "this repository
// has no description" is better said by the absence of a statement than by an
// empty one.
func (b *builder) fact(pred, value, datatype string) {
	if b.node.URI == "" || value == "" {
		return
	}
	b.facts = append(b.facts, Fact{Subject: b.node.URI, Predicate: pred, Value: value, Datatype: datatype})
}

// num records a count. A nil count is a count the surface did not state, which
// is not the same as zero and does not become a statement.
func (b *builder) num(pred string, n *int) {
	if n == nil {
		return
	}
	b.fact(pred, strconv.Itoa(*n), TypeInteger)
}

func (b *builder) at(pred string, t *time.Time) {
	if t == nil || t.IsZero() {
		return
	}
	b.fact(pred, t.UTC().Format(time.RFC3339), TypeDateTime)
}

// actorEdge emits an edge to a person. The actor's own type is used when it says
// one, so a bot or an organization does not silently become a user.
func (b *builder) actorEdge(pred string, a Actor, source string) {
	if a.Login == "" {
		return
	}
	b.to(pred, actorKind(a), a.Login, source)
}

func actorKind(a Actor) string {
	if strings.EqualFold(a.Type, "Organization") {
		return KindOrg
	}
	return KindUser
}

// uriOfURL classifies a github.com URL into a URI, and answers empty for
// anything that is not one. Extractors use it rather than Classify directly so
// a link to an external site drops out instead of producing an error nobody can
// act on.
func uriOfURL(raw string) string {
	if raw == "" {
		return ""
	}
	kind, id, err := Classify(raw)
	if err != nil {
		return ""
	}
	return URI(kind, id)
}

// --- extraction ---

// Extract turns one record into its node, its edges, and its facts. A record
// kind it does not know produces an empty node, which every caller treats as
// nothing to say rather than as an error.
func Extract(rec any) (Node, []Edge, []Fact) {
	b := &builder{}
	switch r := rec.(type) {
	case *Repo:
		b.repo(r)
	case *Trending:
		b.trending(r)
	case *Account:
		b.account(r)
	case *Org:
		b.org(r)
	case *Issue:
		b.issue(r)
	case *PullRequest:
		b.pull(r)
	case *Discussion:
		b.discussion(r)
	case *Thread:
		b.thread(r)
	case *Commit:
		b.commit(r)
	case *GitRef:
		b.gitRef(r)
	case *Release:
		b.release(r)
	case *Topic:
		b.topic(r)
	case *Package:
		b.pkg(r)
	case *WikiPage:
		b.wiki(r)
	case *Gist:
		b.gist(r)
	case *File:
		b.file(r)
	case *TreeEntry:
		b.treeEntry(r)
	case *Contributor:
		b.contributor(r)
	case *Dependency:
		b.dependency(r)
	case *Dependent:
		b.dependent(r)
	case *LanguageShare:
		b.languageShare(r)
	case *RepoStats:
		b.stats(r)
	default:
		return Node{}, nil, nil
	}
	return b.node, b.edges, b.facts
}

// repo is the centre of the graph. Everything else hangs off a repository, and
// most of what a walk finds interesting is stated on this one record.
func (b *builder) repo(r *Repo) {
	id := r.ID
	if id == "" && r.Owner != "" && r.Name != "" {
		id = r.Owner + "/" + r.Name
	}
	b.start(KindRepo, id, id, r.URL)

	// The owner comes from the id, which is why this edge costs nothing. Which
	// of the two account kinds it is comes from the page, so a repository read
	// from a surface that did not say defaults to user and is corrected the
	// moment the owner itself is fetched.
	if r.Owner != "" {
		b.to(PredOwnedBy, ownerKind(r), r.Owner, SrcID)
	}
	// ForkOf is the "Forked from" line in the header, and it is the only place
	// any keyless surface names the parent. templateOf and mirrorOf have their
	// constants in the vocabulary and no producer here, because the page states
	// that a repository is a template or a mirror without ever naming what it
	// was generated from or what it mirrors.
	b.to(PredForkOf, KindRepo, r.ForkOf, SrcHTML)

	for _, t := range r.Topics {
		b.to(PredHasTopic, KindTopic, t, SrcPayload)
	}
	b.raw(PredLicensedUnder, r.License, SrcHTML)
	b.languages(r)

	for i := range r.Tree {
		e := &r.Tree[i]
		if e.URI != "" {
			b.from(e.URI, PredPartOf, b.node.URI, SrcID)
		}
	}

	b.fact(FactName, id, "")
	b.fact(FactDescription, r.Description, "")
	b.fact(FactHomepage, r.Homepage, "")
	b.num(FactStars, r.Stars)
	b.num(FactForks, r.Forks)
	b.num(FactWatchers, r.Watchers)
	b.num(FactCommits, r.CommitCount)
	b.at(FactCreated, r.CreatedAt)
	b.at(FactUpdated, firstSetTime(r.PushedAt, r.UpdatedAt))
	b.fact(FactAvatar, r.OwnerAvatarURL, "")
	b.fact(FactURI, b.node.URI, "")
}

// ownerKind decides between a user and an organization. IsOrgOwned is set by
// the page template, which is the only surface that states it without a token.
func ownerKind(r *Repo) string {
	if r.IsOrgOwned {
		return KindOrg
	}
	return KindUser
}

// languages emits one writtenIn edge per language, weighted by the percentage
// the histogram gave.
//
// The source is honest about where the number came from: Via records
// sidebar-percent when the histogram was read from the deferred sidebar
// fragment, which is a payload, and anything else was read off the language bar
// in the markup.
func (b *builder) languages(r *Repo) {
	source := SrcHTML
	if r.Via["languages"] == "sidebar-percent" {
		source = SrcPayload
	}
	if len(r.Languages) == 0 {
		b.raw(PredWrittenIn, r.Language, source)
		return
	}
	for _, name := range sortedLanguages(r.Languages) {
		b.raw(PredWrittenIn, name, source)
		if n := r.Languages[name]; n > 0 {
			b.weigh(intp(int(n)))
		}
	}
}

// sortedLanguages orders a histogram by share and then by name, so two runs over
// the same repository produce the same edge order.
func sortedLanguages(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if m[out[i]] != m[out[j]] {
			return m[out[i]] > m[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

// trending is a repository plus the people the trending page credits.
func (b *builder) trending(t *Trending) {
	b.repo(&t.Repo)
	for _, a := range t.BuiltBy {
		if a.Login != "" {
			b.from(URI(actorKind(a), a.Login), PredContributedTo, b.node.URI, SrcHTML)
		}
	}
}

func (b *builder) account(a *Account) {
	kind := KindUser
	if strings.EqualFold(a.Type, "Organization") {
		kind = KindOrg
	}
	b.start(kind, a.Login, firstNonEmpty(a.Name, a.Login), a.URL)

	for _, org := range a.Organizations {
		b.to(PredMemberOf, KindOrg, org, SrcHTML)
	}
	// A pinned repository the person does not own is pinned work they
	// contributed to, and this tool cannot tell which from the profile alone.
	// Only the ones whose id carries this login become ownership edges; the rest
	// are left out rather than asserted wrongly.
	for _, repo := range a.PinnedRepos {
		if owner, _, ok := SplitRepo(repo); ok && strings.EqualFold(owner, a.Login) {
			b.from(URI(KindRepo, repo), PredOwnedBy, b.node.URI, SrcHTML)
		}
	}

	b.fact(FactName, firstNonEmpty(a.Name, a.Login), "")
	b.fact(FactDescription, a.Bio, "")
	b.fact(FactHomepage, a.Website, "")
	b.fact(FactAvatar, a.AvatarURL, "")
	b.at(FactCreated, a.CreatedAt)
	b.fact(FactURI, b.node.URI, "")
	if a.Followers != nil {
		b.fact("followers", strconv.Itoa(*a.Followers), TypeInteger)
	}
}

func (b *builder) org(o *Org) {
	b.account(&o.Account)
	// The node kind comes from the template that answered, and this one is the
	// organization template, so it is an organization whatever the account
	// record's Type string says.
	if b.node.URI != "" {
		b.node.Kind = KindOrg
		b.node.URI = URI(KindOrg, o.Login)
	}
	for _, m := range o.Members {
		b.from(URI(KindUser, m), PredMemberOf, b.node.URI, SrcHTML)
	}
	if o.MemberCount != nil {
		b.fact("members", strconv.Itoa(*o.MemberCount), TypeInteger)
	}
	// TopTopics and TopLanguages are aggregates over the organization's
	// repositories rather than properties of the organization, so they produce
	// no hasTopic or writtenIn edge here. The repositories state their own.
}

// thread covers what issues, pull requests, and discussions share.
func (b *builder) thread(t *Thread) {
	kind := t.Kind
	if kind == "" {
		kind = KindIssue
	}
	b.start(kind, t.ID, t.Title, t.URL)

	if t.Repo != "" {
		b.to(PredPartOf, KindRepo, t.Repo, SrcID)
	}
	b.actorEdge(PredAuthoredBy, t.Author, SrcPayload)
	for _, l := range t.Labels {
		if l.Name != "" && t.Repo != "" {
			b.to(PredHasLabel, KindLabel, t.Repo+"/"+l.Name, SrcPayload)
		}
	}
	if m := t.Milestone; m != nil && m.Number != nil && t.Repo != "" {
		b.to(PredInMilestone, KindMilestone, t.Repo+"/"+strconv.Itoa(*m.Number), SrcPayload)
	}
	for _, a := range t.Assignees {
		b.actorEdge(PredAssignedTo, a, SrcPayload)
	}
	for _, r := range t.Reactions {
		b.raw(PredReactedWith, strings.ToLower(r.Content), SrcPayload)
		b.weigh(intp(r.Count))
	}

	b.fact(FactName, t.Title, "")
	b.fact(FactState, t.State, "")
	b.at(FactCreated, t.CreatedAt)
	b.at(FactUpdated, t.UpdatedAt)
	b.fact(FactURI, b.node.URI, "")
}

func (b *builder) issue(i *Issue) {
	b.thread(&i.Thread)
	b.toURL(PredDuplicateOf, i.DuplicateOf, SrcPayload)
	for _, u := range i.LinkedPRs {
		b.toURL(PredLinkedTo, u, SrcPayload)
	}
	for _, u := range i.ClosedByPRs {
		b.toURL(PredClosedBy, u, SrcPayload)
	}
	// SubIssueTotal is a count and not a list, so subIssueOf has no producer on
	// this record. The timeline carries the parent, which is a separate read.
}

func (b *builder) pull(p *PullRequest) {
	b.thread(&p.Thread)
	if p.Repo != "" {
		b.to(PredTargetsBranch, KindBranch, refID(p.Repo, p.BaseRef), SrcPayload)
		b.to(PredFromBranch, KindBranch, refID(p.Repo, p.HeadRef), SrcPayload)
	}
	if p.MergedBy != nil {
		b.actorEdge(PredMergedBy, *p.MergedBy, SrcPayload)
	}
	for _, a := range p.ReviewRequests {
		b.actorEdge(PredReviewRequestedFrom, a, SrcPayload)
	}
	for _, u := range p.ClosesIssues {
		b.toURL(PredCloses, u, SrcPayload)
	}
}

// refID builds owner/name@ref, and answers empty for an empty ref so that a
// pull request read from a surface that did not state its base does not point
// at a branch called nothing.
func refID(repo, ref string) string {
	if repo == "" || ref == "" {
		return ""
	}
	return repo + "@" + ref
}

func (b *builder) discussion(d *Discussion) {
	b.thread(&d.Thread)
	// The answer's author is not the discussion's author and there is no
	// predicate for "answered by" in the vocabulary, so the fact records who it
	// was rather than inventing one.
	if d.AnswerAuthor != nil && d.AnswerAuthor.Login != "" {
		b.fact("answeredBy", d.AnswerAuthor.Login, "")
	}
}

func (b *builder) commit(c *Commit) {
	id := c.ID
	if id == "" && c.Repo != "" && c.SHA != "" {
		id = c.Repo + "@" + c.SHA
	}
	b.start(KindCommit, id, c.Subject, c.URL)

	if c.Repo != "" {
		b.to(PredPartOf, KindRepo, c.Repo, SrcID)
		for _, p := range c.Parents {
			// The arrow reads "that commit is the parent of this one", which is
			// why the parent is the subject and not the object.
			b.from(URI(KindCommit, c.Repo+"@"+p), PredParentOf, b.node.URI, SrcPayload)
		}
	}
	for _, a := range c.Authors {
		b.actorEdge(PredAuthoredBy, a, SrcPayload)
	}
	if c.Committer != nil {
		b.actorEdge(PredCommittedBy, *c.Committer, SrcPayload)
	}
	// GitHub resolved these references itself when it rendered the message, so
	// they are payload rather than the text rule that would find the same #N in
	// the raw subject line.
	for _, ref := range c.IssueRefs {
		b.toURL(PredReferences, ref.URL, SrcPayload)
	}

	b.fact(FactName, c.Subject, "")
	b.at(FactCreated, firstSetTime(c.AuthoredAt, c.CommittedAt))
	b.fact(FactURI, b.node.URI, "")
}

func (b *builder) gitRef(r *GitRef) {
	kind := r.Type
	if kind != KindTag {
		kind = KindBranch
	}
	id := r.ID
	if id == "" {
		id = refID(r.Repo, r.Name)
	}
	b.start(kind, id, r.Name, r.URL)

	if r.Repo != "" {
		b.to(PredPartOf, KindRepo, r.Repo, SrcID)
		// A ref read from the git protocol carries its object name, which is the
		// one edge in this whole file that comes from git rather than from
		// github.com.
		if r.SHA != "" {
			b.to(PredPointsAt, KindCommit, r.Repo+"@"+firstNonEmpty(r.PeeledSHA, r.SHA), SrcPayload)
		}
	}
	if r.Author != nil {
		b.actorEdge(PredAuthoredBy, *r.Author, SrcHTML)
	}
	b.fact(FactName, r.Name, "")
	b.at(FactCreated, r.AuthoredAt)
	b.fact(FactURI, b.node.URI, "")
}

func (b *builder) release(r *Release) {
	id := r.ID
	if id == "" {
		id = refID(r.Repo, r.Tag)
	}
	b.start(KindRelease, id, firstNonEmpty(r.Title, r.Tag), r.URL)

	if r.Repo != "" {
		b.to(PredPartOf, KindRepo, r.Repo, SrcID)
		b.to(PredPointsAt, KindCommit, refID(r.Repo, r.CommitSHA), SrcHTML)
	}
	if r.Author != nil {
		// The releases listing comes from the Atom feed, where the author is an
		// element rather than a selector.
		b.actorEdge(PredAuthoredBy, *r.Author, SrcFeed)
	}
	b.fact(FactName, firstNonEmpty(r.Title, r.Tag), "")
	b.at(FactCreated, r.PublishedAt)
	b.at(FactUpdated, r.UpdatedAt)
	b.fact(FactURI, b.node.URI, "")
}

func (b *builder) topic(t *Topic) {
	b.start(KindTopic, firstNonEmpty(t.ID, t.Name), firstNonEmpty(t.DisplayName, t.Name), t.URL)
	for _, rel := range t.Related {
		b.to(PredRelatedTopic, KindTopic, rel, SrcHTML)
	}
	b.fact(FactName, firstNonEmpty(t.DisplayName, t.Name), "")
	b.fact(FactDescription, firstNonEmpty(t.ShortDescription, t.Description), "")
	b.num(FactStars, t.StargazerCount)
	b.num(FactCount, t.AppliedCount)
	b.fact(FactURI, b.node.URI, "")
}

func (b *builder) pkg(p *Package) {
	id := p.ID
	if id == "" && p.Repo != "" {
		id = p.Repo + "/" + p.Name
	}
	b.start(KindPackage, id, p.Name, p.URL)
	// The direction is the vocabulary's: the package is the subject and the
	// repository it was published from is the object.
	b.to(PredBelongsToPackage, KindRepo, p.Repo, SrcPayload)
	for _, t := range p.Topics {
		b.to(PredHasTopic, KindTopic, t, SrcPayload)
	}
	b.fact(FactName, p.Name, "")
	b.fact(FactDescription, p.Summary, "")
	b.at(FactUpdated, p.UpdatedAt)
	b.fact(FactURI, b.node.URI, "")
}

func (b *builder) wiki(w *WikiPage) {
	id := w.ID
	if id == "" && w.Repo != "" {
		id = w.Repo + "/" + firstNonEmpty(w.Path, w.Title)
	}
	b.start(KindWiki, id, w.Title, w.URL)
	b.to(PredPartOf, KindRepo, w.Repo, SrcID)
	if w.Author != nil {
		b.actorEdge(PredAuthoredBy, *w.Author, SrcHTML)
	}
	b.fact(FactName, w.Title, "")
	b.at(FactUpdated, w.UpdatedAt)
	b.fact(FactURI, b.node.URI, "")
}

func (b *builder) gist(g *Gist) {
	b.start(KindGist, g.ID, firstNonEmpty(g.Description, g.ID), g.URL)
	b.to(PredOwnedBy, KindUser, g.Owner, SrcHTML)
	for _, f := range g.Files {
		b.raw(PredWrittenIn, f.Language, SrcHTML)
	}
	b.fact(FactDescription, g.Description, "")
	b.num(FactStars, g.Stars)
	b.num(FactForks, g.Forks)
	b.at(FactCreated, g.CreatedAt)
	b.fact(FactURI, b.node.URI, "")
}

func (b *builder) file(f *File) {
	b.start(KindFile, f.ID, f.Path, f.URL)
	b.to(PredPartOf, KindRepo, f.Repo, SrcID)
	b.raw(PredWrittenIn, f.Language, SrcPayload)
	b.fact(FactName, f.Path, "")
	b.fact(FactURI, b.node.URI, "")
}

func (b *builder) treeEntry(t *TreeEntry) {
	kind := KindFile
	if strings.Contains(t.Type, "directory") {
		kind = KindTree
	}
	b.start(kind, t.ID, t.Path, t.URL)
	b.to(PredPartOf, KindRepo, t.Repo, SrcID)
	b.fact(FactName, t.Path, "")
	b.fact(FactURI, b.node.URI, "")
}

// contributor is the one weighted authorship edge, and the weight is what makes
// `github edges --predicate contributedTo` a ranked list rather than a set.
func (b *builder) contributor(c *Contributor) {
	b.start(KindUser, c.Login, c.Login, BaseURL+"/"+c.Login)
	if c.Repo != "" {
		b.toURI(PredContributedTo, URI(KindRepo, c.Repo), SrcPayload)
		b.weigh(c.Commits)
		b.when(c.LastWeek)
	}
	b.fact(FactURI, b.node.URI, "")
}

func (b *builder) languageShare(l *LanguageShare) {
	if l.Repo == "" {
		return
	}
	b.start(KindRepo, l.Repo, l.Repo, "")
	b.raw(PredWrittenIn, l.Language, SrcPayload)
}

// dependency and dependent are the two halves of the same relation read off two
// different pages. Both put the page's repository on the subject side, so a
// dependency row from hugo says hugo dependsOn chroma and a dependent row from
// hugo says hugo usedBy someone. A package GitHub could not resolve to a
// repository has nothing to point at and produces no node.
func (b *builder) dependency(d *Dependency) {
	if d.SourceRepo == "" || d.Repo == "" {
		return
	}
	b.start(KindRepo, d.SourceRepo, d.SourceRepo, d.URL)
	b.from(URI(KindRepo, d.Repo), PredDependsOn, b.node.URI, SrcHTML)
}

func (b *builder) dependent(d *Dependent) {
	if d.Dependent == "" || d.Repo == "" {
		return
	}
	b.start(KindRepo, d.Dependent, d.Dependent, d.URL)
	b.from(URI(KindRepo, d.Repo), PredUsedBy, b.node.URI, SrcHTML)
	if d.Owner != "" {
		b.to(PredOwnedBy, KindUser, d.Owner, SrcID)
	}
	b.num(FactStars, d.Stars)
	b.num(FactForks, d.Forks)
}

func (b *builder) stats(s *RepoStats) {
	if s.Repo == "" {
		return
	}
	b.start(KindRepo, s.Repo, s.Repo, "")
	b.num(FactStars, s.Stars)
	b.num(FactForks, s.Forks)
	b.num(FactWatchers, s.Watchers)
	b.num(FactCommits, s.Commits)
	b.at(FactUpdated, s.PushedAt)
	b.fact(FactURI, b.node.URI, "")
}

func firstSetTime(ts ...*time.Time) *time.Time {
	for _, t := range ts {
		if t != nil && !t.IsZero() {
			return t
		}
	}
	return nil
}

// --- the materialised graph ---

// Graph is a set of nodes, edges, and facts held in memory. The streaming
// commands never build one; `github graph` for a single entity, `github rdf` for
// the buffered serialisations, and the tests all want the whole thing in hand.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
	Facts []Fact `json:"facts,omitempty"`
}

// Add folds a record into the graph, skipping a node already present so that a
// repeat visit does not duplicate it.
func (g *Graph) Add(rec any) {
	node, edges, facts := Extract(rec)
	if node.URI == "" {
		return
	}
	g.AddNode(node)
	g.Edges = append(g.Edges, edges...)
	g.Facts = append(g.Facts, facts...)
}

// AddNode adds one node if its URI is new.
func (g *Graph) AddNode(n Node) {
	if n.URI == "" {
		return
	}
	for _, have := range g.Nodes {
		if have.URI == n.URI {
			return
		}
	}
	g.Nodes = append(g.Nodes, n)
}

// Targets returns the object URIs reachable under an allowed predicate set,
// which is what the crawler walks. A bare-string object is never a target:
// there is no page for a language.
func (g *Graph) Targets(allow map[string]bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, e := range g.Edges {
		if !strings.HasPrefix(e.Object, Scheme+"://") {
			continue
		}
		if len(allow) > 0 && !allow[e.Predicate] {
			continue
		}
		if !seen[e.Object] {
			seen[e.Object] = true
			out = append(out, e.Object)
		}
	}
	return out
}

// FilterTrust drops the edges below a floor, in place.
func FilterTrust(edges []Edge, min string) []Edge {
	out := edges[:0]
	for _, e := range edges {
		if TrustAtLeast(e.Source, min) {
			out = append(out, e)
		}
	}
	return out
}

// SortEdges gives an export a stable order, which is what makes a diff of two
// runs readable.
func SortEdges(edges []Edge) {
	sort.SliceStable(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.Subject != b.Subject {
			return a.Subject < b.Subject
		}
		if a.Predicate != b.Predicate {
			return a.Predicate < b.Predicate
		}
		return a.Object < b.Object
	})
}
