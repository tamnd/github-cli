package gh

import (
	"context"
	"strconv"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// ops.go is the table of contents for the whole tool. Every verb a person can
// type is registered here and nowhere else, so the answer to "what can github
// do" is one file, and each registration is simultaneously a CLI subcommand, an
// HTTP route under `github serve`, and an MCP tool under `github mcp`.
//
// The handlers are thin on purpose. Anything with a decision in it belongs in
// the library next to the data it decides about; what is left here is reference
// resolution and one call.

func registerOps(app *kit.App) {
	registerReadOps(app)
	registerSearchOps(app)
	registerContentOps(app)
	registerHistoryOps(app)
	registerPeopleOps(app)
	registerDiscoverOps(app)
	registerMetaOps(app)
}

// --- reference resolution ---

// ResolveRef resolves any accepted reference to the id for the kind a command
// names.
//
// Classify has two guesses in it: one bare word is a user, and one bare
// owner/name is a repository. Those are guesses because a pure function cannot
// tell a user from an organization or a repository from anything else without
// asking, so a guess yields to the command that names its own kind while an
// explicit URL or URI does not. That is what makes `github org golang` work and
// `github org https://github.com/golang/go` fail with a message that says why.
func ResolveRef(want, input string) (string, error) {
	kind, id, err := Classify(input)
	if err != nil {
		return "", err
	}
	if kind == want {
		return id, nil
	}
	if guessed(input, kind) {
		return id, nil
	}
	// A sub-resource names its repository, so a file URL or an issue URL is a
	// fine way to refer to the repository it lives in.
	if want == KindRepo {
		if repo, ok := RepoOf(kind, id); ok {
			return repo, nil
		}
	}
	return "", errs.Usage("%q is a %s, not a %s", input, kind, want)
}

// guessed reports whether Classify was guessing rather than reading. Anything
// with a scheme in it was read: github.com states the kind in the path and a
// github:// URI states it outright.
func guessed(input, kind string) bool {
	if strings.Contains(input, "://") {
		return false
	}
	return kind == KindUser || kind == KindRepo || kind == KindIssue
}

// ResolveRepo resolves any reference to the repository it belongs to. Commands
// that work on a repository use it so that a pasted file URL, issue URL, or
// release URL all name the repository they are part of, which is what makes
// `github commits <any URL from the repo>` work.
func ResolveRepo(input string) (string, error) {
	return ResolveRef(KindRepo, input)
}

// resolveThread resolves the two ways to name an issue, pull request, or
// discussion: a repository and a number, or one URL that already carries both.
// The second form is the one people have in their clipboard.
func resolveThread(want, ref string, num int) (repo string, number int, err error) {
	kind, id, err := Classify(ref)
	if err != nil {
		return "", 0, err
	}
	if r, n, ok := SplitThreadID(id); ok && num == 0 {
		// An issue URL and a pull URL are different paths, so a mismatch here
		// is a real error rather than a guess to be forgiven. A bare
		// owner/name#123 is a guess: nothing in it says which of the two it is.
		if kind != want && !guessed(ref, kind) {
			return "", 0, errs.Usage("%q is a %s, not a %s", ref, kind, want)
		}
		number, _ = strconv.Atoi(n)
		return r, number, nil
	}
	repo, err = ResolveRepo(ref)
	if err != nil {
		return "", 0, err
	}
	if num <= 0 {
		return "", 0, errs.Usage("%s needs a number, either as a second argument or in the URL", want)
	}
	return repo, num, nil
}

// resolveRev resolves the two ways to name a thing that hangs off a git
// revision: a repository and a rev, or one URL carrying both. rev may be empty
// when the caller has a default for it.
func resolveRev(want, ref, rev string) (repo, out string, err error) {
	kind, id, err := Classify(ref)
	if err != nil {
		return "", "", err
	}
	if r, v, ok := cutRev(id); ok && rev == "" {
		if kind != want && !guessed(ref, kind) {
			return "", "", errs.Usage("%q is a %s, not a %s", ref, kind, want)
		}
		return r, v, nil
	}
	repo, err = ResolveRepo(ref)
	if err != nil {
		return "", "", err
	}
	return repo, rev, nil
}

// resolvePath resolves a repository and a path inside it, from either a blob or
// tree URL or a repository plus a path argument.
func resolvePath(ref, path, rev string) (repo, outRef, outPath string, err error) {
	kind, id, err := Classify(ref)
	if err != nil {
		return "", "", "", err
	}
	if kind == KindFile || kind == KindTree {
		r, v, p, ok := SplitPathID(id)
		if ok {
			if rev != "" {
				v = rev
			}
			if path != "" {
				p = path
			}
			return r, v, p, nil
		}
	}
	repo, err = ResolveRepo(ref)
	if err != nil {
		return "", "", "", err
	}
	return repo, rev, strings.TrimPrefix(path, "/"), nil
}

// emitEach sends a slice one record at a time. The client methods that return a
// slice do so because one document holds the whole answer; the command surface
// still wants records, and a single document is never large enough to be worth
// streaming through a channel.
func emitEach[T any](items []T, emit func(*T) error) error {
	for i := range items {
		if err := emit(&items[i]); err != nil {
			return err
		}
	}
	return nil
}

// byValue adapts a client method that emits values to a handler that emits
// pointers. The client methods emit values because a record built inside a
// pager has no reason to escape to the heap; kit wants a pointer because the
// record may be rendered, stored, and serialised after the call returns.
func byValue[T any](emit func(*T) error) func(T) error {
	return func(v T) error { return emit(&v) }
}

// --- reading one thing ---

type repoIn struct {
	C      *Client `kit:"inject"`
	Ref    string  `kit:"arg" help:"owner/name, a github.com URL, or a github:// URI"`
	Readme bool    `kit:"flag" help:"include the rendered README, which is most of the bytes"`
	Deep   bool    `kit:"flag" help:"also fetch the language histogram and the dependent count"`
}

type nameIn struct {
	C    *Client `kit:"inject"`
	Name string  `kit:"arg" help:"a login, a profile URL, or a github:// URI"`
}

type threadIn struct {
	C   *Client `kit:"inject"`
	Ref string  `kit:"arg" help:"owner/name, or a full thread URL that already carries the number"`
	Num int     `kit:"arg" help:"the number, when the reference does not carry one"`
}

type bareRefIn struct {
	C   *Client `kit:"inject"`
	Ref string  `kit:"arg" help:"any github reference"`
}

func registerReadOps(app *kit.App) {
	kit.Handle(app, kit.OpMeta{
		Name: "repo", Group: "read", Single: true, URIType: KindRepo, Resolver: true,
		Summary: "Read one repository with every field the page states",
		Long: "The repository page carries more than the API does: the sidebar's about\n" +
			"block, the topic list, the license name as rendered, the release the page\n" +
			"is pointing at, and the counts for stars, forks, watchers, and open issues.",
		Args: []kit.Arg{{Name: "ref", Help: "owner/name, URL, or github:// URI"}},
	}, getRepo)

	kit.Handle(app, kit.OpMeta{
		Name: "user", Group: "read", Single: true, URIType: KindUser, Resolver: true,
		Summary: "Read one user profile",
		Args:    []kit.Arg{{Name: "name", Help: "login, profile URL, or github:// URI"}},
	}, getUser)

	kit.Handle(app, kit.OpMeta{
		Name: "org", Group: "read", Single: true, URIType: KindOrg, Resolver: true,
		Summary: "Read one organization",
		Long: "An organization renders a different template from a user, so this returns\n" +
			"the fields only that template has. For a name whose kind nobody knows yet,\n" +
			"use `github user`, which reads whichever template answers and reports the\n" +
			"kind it found.",
		Args: []kit.Arg{{Name: "name", Help: "login, profile URL, or github:// URI"}},
	}, getOrg)

	kit.Handle(app, kit.OpMeta{
		Name: "issue", Group: "read", Single: true, URIType: KindIssue, Resolver: true,
		Summary: "Read one issue with its body, labels, and participants",
		Args: []kit.Arg{
			{Name: "ref", Help: "owner/name, or an issue URL"},
			{Name: "num", Help: "issue number", Optional: true},
		},
	}, getIssue)

	kit.Handle(app, kit.OpMeta{
		Name: "pr", Group: "read", Single: true, URIType: KindPR, Resolver: true,
		Aliases: []string{"pull"},
		Summary: "Read one pull request, merge state and review state included",
		Args: []kit.Arg{
			{Name: "ref", Help: "owner/name, or a pull request URL"},
			{Name: "num", Help: "pull request number", Optional: true},
		},
	}, getPull)

	kit.Handle(app, kit.OpMeta{
		Name: "discussion", Group: "read", Single: true, URIType: KindDiscussion, Resolver: true,
		Summary: "Read one discussion, its category, and its answer",
		Args: []kit.Arg{
			{Name: "ref", Help: "owner/name, or a discussion URL"},
			{Name: "num", Help: "discussion number", Optional: true},
		},
	}, getDiscussion)

	kit.Handle(app, kit.OpMeta{
		Name: "commit", Group: "read", Single: true, URIType: KindCommit, Resolver: true,
		Summary: "Read one commit, with its author, verification, and changed files",
		Args: []kit.Arg{
			{Name: "ref", Help: "owner/name, or a commit URL"},
			{Name: "sha", Help: "commit sha", Optional: true},
		},
	}, getCommit)

	kit.Handle(app, kit.OpMeta{
		Name: "release", Group: "read", Single: true, URIType: KindRelease, Resolver: true,
		Summary: "Read one release by tag, or the latest one",
		Long: "The tag defaults to latest, which github.com redirects to whatever that is\n" +
			"today. Assets live behind a lazy fragment the page does not load until you\n" +
			"scroll, which is why a release page can be 238 KB and show no downloads at\n" +
			"all. Reading one release fetches the fragment, because a release without\n" +
			"its downloads is not the thing you asked for. The list command makes that\n" +
			"a flag, since there the cost is one request per release.",
		Args: []kit.Arg{
			{Name: "ref", Help: "owner/name, or a release URL"},
			{Name: "tag", Help: "tag name, or latest", Optional: true},
		},
	}, getRelease)

	kit.Handle(app, kit.OpMeta{
		Name: "compare", Group: "read", Single: true, URIType: KindCompare, Resolver: true,
		Aliases: []string{"range"},
		Summary: "Compare two refs and list what changed between them",
		Long: "Compare has no JSON route: the page answers Rails HTML however you ask.\n" +
			"The patch does have everything, so this reads the git-format-patch mailbox\n" +
			"and takes the commit list and the per-file changes out of it.",
		Args: []kit.Arg{
			{Name: "ref", Help: "owner/name, or a compare URL"},
			{Name: "base", Help: "the ref to compare from", Optional: true},
			{Name: "head", Help: "the ref to compare to", Optional: true},
		},
	}, getCompare)

	kit.Handle(app, kit.OpMeta{
		Name: "get", Group: "read", Single: true,
		Summary: "Read whatever a reference points at",
		Long: "get classifies the reference and dispatches to the right reader, which is\n" +
			"what makes `github commits ... -o url | xargs -n1 github get` work across\n" +
			"mixed kinds.",
		Args: []kit.Arg{{Name: "ref", Help: "any github reference"}},
	}, getAny)
}

func getRepo(ctx context.Context, in repoIn, emit func(*Repo) error) error {
	id, err := ResolveRef(KindRepo, in.Ref)
	if err != nil {
		return err
	}
	r, err := in.C.Repo(ctx, id, RepoOptions{Deep: in.Deep || in.C.Deep, Readme: in.Readme})
	if err != nil {
		return err
	}
	return emit(r)
}

// getUser reads a profile without caring which template answers. Account
// reports the kind it found, so a name that turns out to be an organization
// comes back as one rather than as an error.
func getUser(ctx context.Context, in nameIn, emit func(*Account) error) error {
	login, err := ResolveRef(KindUser, in.Name)
	if err != nil {
		return err
	}
	a, err := in.C.Account(ctx, login)
	if err != nil {
		return err
	}
	return emit(a)
}

func getOrg(ctx context.Context, in nameIn, emit func(*Org) error) error {
	login, err := ResolveRef(KindOrg, in.Name)
	if err != nil {
		return err
	}
	o, err := in.C.Org(ctx, login)
	if err != nil {
		return err
	}
	return emit(o)
}

func getIssue(ctx context.Context, in threadIn, emit func(*Issue) error) error {
	repo, num, err := resolveThread(KindIssue, in.Ref, in.Num)
	if err != nil {
		return err
	}
	i, err := in.C.Issue(ctx, repo, num)
	if err != nil {
		return err
	}
	return emit(i)
}

func getPull(ctx context.Context, in threadIn, emit func(*PullRequest) error) error {
	repo, num, err := resolveThread(KindPR, in.Ref, in.Num)
	if err != nil {
		return err
	}
	p, err := in.C.PullRequest(ctx, repo, num)
	if err != nil {
		return err
	}
	return emit(p)
}

func getDiscussion(ctx context.Context, in threadIn, emit func(*Discussion) error) error {
	repo, num, err := resolveThread(KindDiscussion, in.Ref, in.Num)
	if err != nil {
		return err
	}
	d, err := in.C.Discussion(ctx, repo, num)
	if err != nil {
		return err
	}
	return emit(d)
}

// commitIn has no --files flag for the same reason compareIn has none: the
// route ships the whole diff whether it is decoded or not, so the per-file list
// costs parsing rather than a request.
type commitIn struct {
	C     *Client `kit:"inject"`
	Ref   string  `kit:"arg" help:"owner/name, or a commit URL"`
	SHA   string  `kit:"arg" help:"commit sha, when the reference does not carry one"`
	Patch bool    `kit:"flag" help:"also fetch the patch, for applying the change rather than describing it"`
}

func getCommit(ctx context.Context, in commitIn, emit func(*Commit) error) error {
	repo, sha, err := resolveRev(KindCommit, in.Ref, in.SHA)
	if err != nil {
		return err
	}
	if sha == "" {
		return errs.Usage("commit needs a sha, either as a second argument or in the URL")
	}
	c, err := in.C.CommitInfo(ctx, repo, sha, CommitInfoOptions{Files: true, Patch: in.Patch})
	if err != nil {
		return err
	}
	return emit(c)
}

type releaseIn struct {
	C   *Client `kit:"inject"`
	Ref string  `kit:"arg" help:"owner/name, or a release URL"`
	Tag string  `kit:"arg" help:"tag name, or latest"`
}

func getRelease(ctx context.Context, in releaseIn, emit func(*Release) error) error {
	repo, tag, err := resolveRev(KindRelease, in.Ref, in.Tag)
	if err != nil {
		return err
	}
	if tag == "" {
		tag = "latest"
	}
	r, err := in.C.Release(ctx, repo, tag, ReleaseOptions{Assets: true, Body: true})
	if err != nil {
		return err
	}
	return emit(r)
}

// compareIn has no --files flag because the per-file list is free: the patch is
// already downloaded and parsing it costs nothing anyone would notice. --patch
// is a flag because keeping the raw stream on the record can be megabytes.
type compareIn struct {
	C     *Client `kit:"inject"`
	Ref   string  `kit:"arg" help:"owner/name, or a compare URL"`
	Base  string  `kit:"arg" help:"the ref to compare from"`
	Head  string  `kit:"arg" help:"the ref to compare to"`
	Patch bool    `kit:"flag" help:"keep the raw patch on the record"`
}

func getCompare(ctx context.Context, in compareIn, emit func(*Compare) error) error {
	repo, base, head, err := resolveCompare(in.Ref, in.Base, in.Head)
	if err != nil {
		return err
	}
	cmp, err := in.C.CompareRefs(ctx, repo, base, head, CompareOptions{Files: true, Patch: in.Patch})
	if err != nil {
		return err
	}
	return emit(cmp)
}

// resolveCompare accepts a compare URL, which carries both ends, or a
// repository and two refs.
func resolveCompare(ref, base, head string) (string, string, string, error) {
	kind, id, err := Classify(ref)
	if err != nil {
		return "", "", "", err
	}
	if kind == KindCompare && base == "" {
		repo, rng, ok := cutRev(id)
		if !ok {
			return "", "", "", errs.Usage("%q is not a range", ref)
		}
		// Three dots is the merge-base form and two is the direct diff.
		// github.com accepts both and means different things by them, so the
		// separator is kept as the caller wrote it.
		for _, sep := range []string{"...", ".."} {
			if a, b, found := strings.Cut(rng, sep); found {
				return repo, a, b, nil
			}
		}
		return "", "", "", errs.Usage("%q has no base...head in it", ref)
	}
	repo, err := ResolveRepo(ref)
	if err != nil {
		return "", "", "", err
	}
	if base == "" || head == "" {
		return "", "", "", errs.Usage("compare needs a base and a head, or a compare URL that carries both")
	}
	return repo, base, head, nil
}

// getAny dispatches on the kind the reference names. The kinds it declines are
// declined by name, because "not implemented yet" and "no such thing" are
// different answers and a script should be able to tell them apart.
func getAny(ctx context.Context, in bareRefIn, emit func(any) error) error {
	kind, id, err := Classify(in.Ref)
	if err != nil {
		return err
	}
	rec, err := in.C.fetchOne(ctx, kind, id)
	if err != nil {
		return err
	}
	return emit(rec)
}

// fetchOne reads one record of any kind by id. The graph walk and `github get`
// share it, so a kind that reads correctly in one reads correctly in both.
func (c *Client) fetchOne(ctx context.Context, kind, id string) (any, error) {
	switch kind {
	case KindRepo:
		return c.Repo(ctx, id, RepoOptions{Deep: c.Deep})
	case KindUser:
		return c.Account(ctx, id)
	case KindOrg:
		return c.Org(ctx, id)
	case KindIssue, KindPR, KindDiscussion:
		repo, n, ok := SplitThreadID(id)
		if !ok {
			return nil, errs.Usage("%q is not a thread id", id)
		}
		num, _ := strconv.Atoi(n)
		switch kind {
		case KindIssue:
			return c.Issue(ctx, repo, num)
		case KindPR:
			return c.PullRequest(ctx, repo, num)
		default:
			return c.Discussion(ctx, repo, num)
		}
	case KindCommit:
		repo, sha, ok := cutRev(id)
		if !ok {
			return nil, errs.Usage("%q is not a commit id", id)
		}
		return c.CommitInfo(ctx, repo, sha, CommitInfoOptions{})
	case KindRelease:
		repo, tag, ok := cutRev(id)
		if !ok {
			return nil, errs.Usage("%q is not a release id", id)
		}
		return c.Release(ctx, repo, tag, ReleaseOptions{Assets: true, Body: true})
	case KindBranch, KindTag:
		repo, name, ok := cutRev(id)
		if !ok {
			return nil, errs.Usage("%q is not a %s id", id, kind)
		}
		return c.oneRef(ctx, kind, repo, name)
	case KindCompare:
		repo, rng, ok := cutRev(id)
		if !ok {
			return nil, errs.Usage("%q is not a range", id)
		}
		base, head, found := strings.Cut(rng, "...")
		if !found {
			base, head, found = strings.Cut(rng, "..")
		}
		if !found {
			return nil, errs.Usage("%q has no base...head in it", id)
		}
		return c.CompareRefs(ctx, repo, base, head, CompareOptions{Files: true})
	case KindFile:
		repo, ref, path, ok := SplitPathID(id)
		if !ok {
			return nil, errs.Usage("%q is not a file id", id)
		}
		return c.Blob(ctx, repo, path, BlobOptions{Ref: ref})
	}
	return nil, errs.Unsupported("reading a %s is not implemented yet", kind)
}

// oneRef finds a single branch or tag by name. The git advertisement is one
// request for every ref in the repository, which beats paging the branches page
// looking for one row.
func (c *Client) oneRef(ctx context.Context, kind, repo, name string) (*GitRef, error) {
	var found *GitRef
	list := c.Branches
	if kind == KindTag {
		list = c.Tags
	}
	err := list(ctx, repo, RefOptions{Complete: true}, func(r GitRef) error {
		if r.Name == name {
			cp := r
			found = &cp
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, errs.NotFound("%s %s has no %s named %s", repo, kind, kind, name)
	}
	return found, nil
}

// --- searching ---

// searchIn is the query surface every search type shares. GitHub's search box
// takes qualifiers inline (repo:, org:, language:, is:open) and this passes the
// query through untouched, so anything that works in the box works here. The
// flags are sugar that appends a qualifier, for the ones people reach for often
// enough that quoting them gets old.
type searchIn struct {
	C        *Client `kit:"inject"`
	Query    string  `kit:"arg" help:"a search query, with the same qualifiers the search box takes"`
	Repo     string  `kit:"flag" help:"restrict to one repository, owner/name"`
	Owner    string  `kit:"flag" help:"restrict to one user or organization"`
	Language string  `kit:"flag" help:"restrict to one language"`
	Sort     string  `kit:"flag" help:"a sort qualifier, e.g. stars or updated"`
	Limit    int     `kit:"flag,inherit"`
}

// query folds the sugar flags into the one string search actually takes.
func (in searchIn) query() string {
	q := in.Query
	add := func(qualifier, value string) {
		if value == "" {
			return
		}
		q = strings.TrimSpace(q + " " + qualifier + ":" + value)
	}
	add("repo", in.Repo)
	add("user", in.Owner)
	add("language", in.Language)
	add("sort", in.Sort)
	return strings.TrimSpace(q)
}

func registerSearchOps(app *kit.App) {
	kit.Handle(app, kit.OpMeta{
		Name: "repos", Group: "search", URIType: KindRepo, List: true,
		Summary: "Search repositories",
		Args:    []kit.Arg{{Name: "query", Help: "a search query", Optional: true}},
	}, listRepos)

	kit.Handle(app, kit.OpMeta{
		Name: "issues", Group: "search", URIType: KindIssue, List: true,
		Summary: "Search issues",
		Args:    []kit.Arg{{Name: "query", Help: "a search query", Optional: true}},
	}, listIssues)

	kit.Handle(app, kit.OpMeta{
		Name: "prs", Group: "search", URIType: KindPR, List: true,
		Aliases: []string{"pulls"},
		Summary: "Search pull requests",
		Args:    []kit.Arg{{Name: "query", Help: "a search query", Optional: true}},
	}, listPulls)

	kit.Handle(app, kit.OpMeta{
		Name: "users", Group: "search", URIType: KindUser, List: true,
		Summary: "Search users and organizations",
		Args:    []kit.Arg{{Name: "query", Help: "a search query", Optional: true}},
	}, listUsers)

	kit.Handle(app, kit.OpMeta{
		Name: "topics", Group: "search", URIType: KindTopic, List: true,
		Summary: "Search topics",
		Args:    []kit.Arg{{Name: "query", Help: "a search query", Optional: true}},
	}, listTopics)

	kit.Handle(app, kit.OpMeta{
		Name: "packages", Group: "search", URIType: KindPackage, List: true,
		Summary: "Search published packages",
		Args:    []kit.Arg{{Name: "query", Help: "a search query", Optional: true}},
	}, listPackages)

	kit.Handle(app, kit.OpMeta{
		Name: "wikis", Group: "search", URIType: KindWiki, List: true,
		Summary: "Search wiki pages",
		Args:    []kit.Arg{{Name: "query", Help: "a search query", Optional: true}},
	}, listWikis)

	kit.Handle(app, kit.OpMeta{
		Name: "actions", Group: "search", URIType: KindAction, List: true,
		Summary: "Search the marketplace for actions",
		Args:    []kit.Arg{{Name: "query", Help: "a search query", Optional: true}},
	}, listActions)

	kit.Handle(app, kit.OpMeta{
		Name: "code", Group: "search",
		Summary: "Search code, which needs a session and so does not work here",
		Long: "Code search is the one search type that requires a signed-in session. It\n" +
			"answers 200 with an empty result set to an anonymous request, which looks\n" +
			"exactly like a query that found nothing, so this says so instead of\n" +
			"returning zero hits and letting you conclude the query was wrong.",
		Args: []kit.Arg{{Name: "query", Help: "a search query"}},
	}, listCode)

	kit.Handle(app, kit.OpMeta{
		Name: "search", Group: "search",
		Summary: "Search every type at once",
		Long: "Nine of GitHub's ten search types answer JSON to an anonymous request.\n" +
			"This asks all of them, or the ones named by --type, and streams the\n" +
			"records as they arrive.",
		Args: []kit.Arg{{Name: "query", Help: "what to look for"}},
	}, searchAll)
}

func listRepos(ctx context.Context, in searchIn, emit func(*Repo) error) error {
	return in.C.SearchRepositories(ctx, in.query(), in.Limit, byValue(emit))
}

func listIssues(ctx context.Context, in searchIn, emit func(*Thread) error) error {
	return in.C.SearchIssuesAndPulls(ctx, in.query(), SearchIssues, in.Limit, byValue(emit))
}

func listPulls(ctx context.Context, in searchIn, emit func(*Thread) error) error {
	return in.C.SearchIssuesAndPulls(ctx, in.query(), SearchPulls, in.Limit, byValue(emit))
}

func listUsers(ctx context.Context, in searchIn, emit func(*Account) error) error {
	return in.C.SearchAccounts(ctx, in.query(), in.Limit, byValue(emit))
}

func listTopics(ctx context.Context, in searchIn, emit func(*Topic) error) error {
	return in.C.SearchTopicsBy(ctx, in.query(), in.Limit, byValue(emit))
}

func listPackages(ctx context.Context, in searchIn, emit func(*Package) error) error {
	return in.C.SearchPackagesBy(ctx, in.query(), in.Limit, byValue(emit))
}

func listWikis(ctx context.Context, in searchIn, emit func(*WikiPage) error) error {
	return in.C.SearchWikisBy(ctx, in.query(), in.Limit, byValue(emit))
}

func listActions(ctx context.Context, in searchIn, emit func(*Action) error) error {
	return in.C.SearchMarketplace(ctx, in.query(), in.Limit, byValue(emit))
}

func listCode(ctx context.Context, in searchIn, emit func(*File) error) error {
	return in.C.SearchCodeBy(ctx, in.query(), in.Limit, byValue(emit))
}

type searchAllIn struct {
	C     *Client  `kit:"inject"`
	Query string   `kit:"arg" help:"what to look for"`
	Type  []string `kit:"flag" help:"restrict to some types, e.g. repositories,issues"`
	Limit int      `kit:"flag,inherit"`
}

func searchAll(ctx context.Context, in searchAllIn, emit func(any) error) error {
	types := in.Type
	if len(types) == 0 {
		types = SearchTypes
	}
	// The per-type limit is the whole limit: a caller asking for ten of
	// everything gets ten of each rather than ten split nine ways, which is
	// what anyone piping this into a filter wants.
	for _, t := range types {
		if err := in.C.searchOne(ctx, t, in.Query, in.Limit, emit); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) searchOne(ctx context.Context, typ, query string, limit int, emit func(any) error) error {
	any1 := func(v any) error { return emit(v) }
	switch typ {
	case SearchRepos:
		return c.SearchRepositories(ctx, query, limit, func(r Repo) error { return any1(&r) })
	case SearchIssues, SearchPulls:
		return c.SearchIssuesAndPulls(ctx, query, typ, limit, func(t Thread) error { return any1(&t) })
	case SearchUsers:
		return c.SearchAccounts(ctx, query, limit, func(a Account) error { return any1(&a) })
	case SearchCommits:
		return c.SearchCommitsBy(ctx, query, limit, func(v Commit) error { return any1(&v) })
	case SearchDiscussions:
		return c.SearchDiscussionsBy(ctx, query, limit, func(d Discussion) error { return any1(&d) })
	case SearchTopics:
		return c.SearchTopicsBy(ctx, query, limit, func(t Topic) error { return any1(&t) })
	case SearchPackages:
		return c.SearchPackagesBy(ctx, query, limit, func(p Package) error { return any1(&p) })
	case SearchWikis:
		return c.SearchWikisBy(ctx, query, limit, func(w WikiPage) error { return any1(&w) })
	case SearchMarket:
		return c.SearchMarketplace(ctx, query, limit, func(a Action) error { return any1(&a) })
	case SearchCode:
		return c.SearchCodeBy(ctx, query, limit, func(f File) error { return any1(&f) })
	}
	return errs.Usage("%q is not a search type; the types are %s", typ, strings.Join(SearchTypes, ", "))
}

// --- contents ---

type treeIn struct {
	C         *Client `kit:"inject"`
	Ref       string  `kit:"arg" help:"owner/name, or a tree URL"`
	Path      string  `kit:"arg" help:"a directory inside the repository"`
	Rev       string  `kit:"flag,name=rev" help:"branch, tag, or sha; the default branch when empty"`
	Recursive bool    `kit:"flag" help:"walk subdirectories, one request each"`
	Sizes     bool    `kit:"flag" help:"fill the byte size of each file, one request each"`
	Limit     int     `kit:"flag,inherit"`
}

type blobIn struct {
	C       *Client `kit:"inject"`
	Ref     string  `kit:"arg" help:"owner/name, or a blob URL"`
	Path    string  `kit:"arg" help:"a file inside the repository"`
	Rev     string  `kit:"flag,name=rev" help:"branch, tag, or sha; the default branch when empty"`
	Content bool    `kit:"flag" help:"include the bytes, fetched from raw"`
	Styled  bool    `kit:"flag" help:"include the rendered lines and the syntax highlighting spans"`
}

func registerContentOps(app *kit.App) {
	kit.Handle(app, kit.OpMeta{
		Name: "tree", Group: "contents", URIType: KindTree, List: true,
		Aliases: []string{"ls"},
		Summary: "List a directory, or the whole tree",
		Long: "There is no recursive parameter on this route, so --recursive is one\n" +
			"request per directory. For a whole large repository, `github archive` is\n" +
			"one request instead of hundreds.",
		Args: []kit.Arg{
			{Name: "ref", Help: "owner/name, or a tree URL"},
			{Name: "path", Help: "a directory inside the repository", Optional: true},
		},
	}, listTree)

	kit.Handle(app, kit.OpMeta{
		Name: "blob", Group: "contents", Single: true, URIType: KindFile, Resolver: true,
		Aliases: []string{"file"},
		Summary: "Read one file's metadata, and its bytes on request",
		Args: []kit.Arg{
			{Name: "ref", Help: "owner/name, or a blob URL"},
			{Name: "path", Help: "a file inside the repository", Optional: true},
		},
	}, getBlob)

	kit.Handle(app, kit.OpMeta{
		Name: "symbols", Group: "contents",
		Summary: "List the definitions GitHub extracted from a file",
		Long: "GitHub runs a symbol extractor over every blob it renders and ships the\n" +
			"result in the route payload. There is no unauthenticated REST equivalent\n" +
			"anywhere. The extractor is asynchronous, so an empty list can mean the\n" +
			"language is unsupported or that the analysis had not finished; the record\n" +
			"says which, and this command reports it rather than guessing.",
		Args: []kit.Arg{
			{Name: "ref", Help: "owner/name, or a blob URL"},
			{Name: "path", Help: "a file inside the repository", Optional: true},
		},
	}, listSymbols)
}

func listTree(ctx context.Context, in treeIn, emit func(*TreeEntry) error) error {
	repo, ref, path, err := resolvePath(in.Ref, in.Path, in.Rev)
	if err != nil {
		return err
	}
	return in.C.Tree(ctx, repo, path, TreeOptions{
		Ref:       ref,
		Recursive: in.Recursive,
		Sizes:     in.Sizes,
		Limit:     in.Limit,
	}, byValue(emit))
}

func getBlob(ctx context.Context, in blobIn, emit func(*File) error) error {
	repo, ref, path, err := resolvePath(in.Ref, in.Path, in.Rev)
	if err != nil {
		return err
	}
	if path == "" {
		return errs.Usage("blob needs a path, either as a second argument or in the URL")
	}
	f, err := in.C.Blob(ctx, repo, path, BlobOptions{Ref: ref, Content: in.Content, Styled: in.Styled})
	if err != nil {
		return err
	}
	return emit(f)
}

type symbolIn struct {
	C    *Client `kit:"inject"`
	Ref  string  `kit:"arg" help:"owner/name, or a blob URL"`
	Path string  `kit:"arg" help:"a file inside the repository"`
	Rev  string  `kit:"flag,name=rev" help:"branch, tag, or sha; the default branch when empty"`
}

func listSymbols(ctx context.Context, in symbolIn, emit func(*Symbol) error) error {
	repo, ref, path, err := resolvePath(in.Ref, in.Path, in.Rev)
	if err != nil {
		return err
	}
	if path == "" {
		return errs.Usage("symbols needs a path, either as a second argument or in the URL")
	}
	f, err := in.C.Blob(ctx, repo, path, BlobOptions{Ref: ref})
	if err != nil {
		return err
	}
	// The path goes in the middle of these sentences rather than at the front,
	// because the CLI title-cases the first word of an error and a path is the
	// one thing that must not be title-cased.
	switch f.SymbolsStatus {
	case "not_analyzed":
		return errs.Unsupported("GitHub does not extract symbols from the language %s is written in", path)
	case "unavailable", "timed_out":
		return errs.Network("GitHub's symbol analysis for %s had not finished; ask again in a moment", path)
	}
	return emitEach(f.Symbols, emit)
}

// --- history ---

type commitsIn struct {
	C      *Client `kit:"inject"`
	Ref    string  `kit:"arg" help:"owner/name, or any URL from the repository"`
	Rev    string  `kit:"flag,name=rev" help:"branch, tag, or sha to walk from"`
	Path   string  `kit:"flag" help:"limit history to one file or directory"`
	Author string  `kit:"flag" help:"a login, not an email address"`
	Since  string  `kit:"flag" help:"only commits after this date, YYYY-MM-DD"`
	Until  string  `kit:"flag" help:"only commits before this date, YYYY-MM-DD"`
	PR     int     `kit:"flag,name=pr" help:"walk one pull request's commits instead of the branch"`
	Limit  int     `kit:"flag,inherit"`
}

type refsIn struct {
	C        *Client `kit:"inject"`
	Ref      string  `kit:"arg" help:"owner/name, or any URL from the repository"`
	Complete bool    `kit:"flag" help:"read the git advertisement: every ref in one request, with shas"`
	Pulls    bool    `kit:"flag" help:"include refs/pull/*, which on a busy repository is most of the response"`
	Limit    int     `kit:"flag,inherit"`
}

type releasesIn struct {
	C      *Client `kit:"inject"`
	Ref    string  `kit:"arg" help:"owner/name, or any URL from the repository"`
	Assets bool    `kit:"flag" help:"fetch each release's lazy asset fragment, one request each"`
	Body   bool    `kit:"flag" help:"keep the rendered release notes"`
	Limit  int     `kit:"flag,inherit"`
}

func registerHistoryOps(app *kit.App) {
	kit.Handle(app, kit.OpMeta{
		Name: "commits", Group: "history", URIType: KindCommit, List: true,
		Aliases: []string{"log"},
		Summary: "Walk a repository's history",
		Long: "Every filter here is a query parameter the route already understands, so\n" +
			"the filtering happens on GitHub's side rather than after a full download.\n" +
			"A page is 35 commits, and an unbounded walk of a large repository is\n" +
			"thousands of requests, so set --limit unless you mean it.",
		Args: []kit.Arg{{Name: "ref", Help: "owner/name, or any URL from the repository"}},
	}, listCommits)

	kit.Handle(app, kit.OpMeta{
		Name: "refs", Group: "history", List: true,
		Summary: "List every ref in a repository",
		Long: "Without --complete this reads the refs fragment, which is names only but\n" +
			"is 6 KB where the advertisement is 588 KB. With it, one request returns\n" +
			"every branch, tag, and pull head with its sha, and no cap.",
		Args: []kit.Arg{{Name: "ref", Help: "owner/name, or any URL from the repository"}},
	}, listRefs)

	kit.Handle(app, kit.OpMeta{
		Name: "branches", Group: "history", URIType: KindBranch, List: true,
		Summary: "List branches",
		Args:    []kit.Arg{{Name: "ref", Help: "owner/name, or any URL from the repository"}},
	}, listBranches)

	kit.Handle(app, kit.OpMeta{
		Name: "tags", Group: "history", URIType: KindTag, List: true,
		Summary: "List tags",
		Args:    []kit.Arg{{Name: "ref", Help: "owner/name, or any URL from the repository"}},
	}, listTags)

	kit.Handle(app, kit.OpMeta{
		Name: "releases", Group: "history", URIType: KindRelease, List: true,
		Summary: "List releases",
		Args:    []kit.Arg{{Name: "ref", Help: "owner/name, or any URL from the repository"}},
	}, listReleases)

	kit.Handle(app, kit.OpMeta{
		Name: "timeline", Group: "history", List: true,
		Summary: "List everything that happened on one issue or pull request",
		Long: "The timeline is the comments and the events in one stream: labels applied,\n" +
			"milestones set, commits referenced, reviews requested, the lot.",
		Args: []kit.Arg{
			{Name: "ref", Help: "owner/name, or a thread URL"},
			{Name: "num", Help: "the number, when the reference does not carry one", Optional: true},
		},
	}, listTimeline)
}

func listCommits(ctx context.Context, in commitsIn, emit func(*Commit) error) error {
	repo, rev, err := resolveRev(KindCommit, in.Ref, in.Rev)
	if err != nil {
		return err
	}
	if in.PR > 0 {
		return in.C.PullCommits(ctx, repo, in.PR, in.Limit, byValue(emit))
	}
	return in.C.Commits(ctx, repo, CommitOptions{
		Ref:    rev,
		Path:   in.Path,
		Author: in.Author,
		Since:  in.Since,
		Until:  in.Until,
		Limit:  in.Limit,
	}, byValue(emit))
}

func listRefs(ctx context.Context, in refsIn, emit func(*GitRef) error) error {
	repo, err := ResolveRepo(in.Ref)
	if err != nil {
		return err
	}
	return in.C.Refs(ctx, repo, in.options(), byValue(emit))
}

func listBranches(ctx context.Context, in refsIn, emit func(*GitRef) error) error {
	repo, err := ResolveRepo(in.Ref)
	if err != nil {
		return err
	}
	return in.C.Branches(ctx, repo, in.options(), byValue(emit))
}

func listTags(ctx context.Context, in refsIn, emit func(*GitRef) error) error {
	repo, err := ResolveRepo(in.Ref)
	if err != nil {
		return err
	}
	return in.C.Tags(ctx, repo, in.options(), byValue(emit))
}

func (in refsIn) options() RefOptions {
	return RefOptions{Complete: in.Complete, Pulls: in.Pulls, Limit: in.Limit}
}

func listReleases(ctx context.Context, in releasesIn, emit func(*Release) error) error {
	repo, err := ResolveRepo(in.Ref)
	if err != nil {
		return err
	}
	return in.C.Releases(ctx, repo, ReleaseOptions{
		Assets: in.Assets,
		Body:   in.Body,
		Limit:  in.Limit,
	}, byValue(emit))
}

type timelineIn struct {
	C     *Client `kit:"inject"`
	Ref   string  `kit:"arg" help:"owner/name, or a thread URL"`
	Num   int     `kit:"arg" help:"the number, when the reference does not carry one"`
	Limit int     `kit:"flag,inherit"`
}

func listTimeline(ctx context.Context, in timelineIn, emit func(*TimelineItem) error) error {
	repo, num, err := resolveThread(KindIssue, in.Ref, in.Num)
	if err != nil {
		return err
	}
	return in.C.Timeline(ctx, repo, num, in.Limit, byValue(emit))
}

// --- people ---

type accountListIn struct {
	C     *Client `kit:"inject"`
	Name  string  `kit:"arg" help:"a login, a profile URL, or a github:// URI"`
	Limit int     `kit:"flag,inherit"`
}

type gistIn struct {
	C       *Client `kit:"inject"`
	Ref     string  `kit:"arg" help:"a gist id, a gist URL, or a github:// URI"`
	Content bool    `kit:"flag" help:"fetch each file's raw content, one request per file"`
}

type contributionsIn struct {
	C    *Client `kit:"inject"`
	Name string  `kit:"arg" help:"a login, a profile URL, or a github:// URI"`
	Year int     `kit:"flag" help:"calendar year, defaulting to the rolling last twelve months"`
}

type activityIn struct {
	C     *Client `kit:"inject"`
	Ref   string  `kit:"arg" help:"a login for a person's stream, or owner/name for a repository's"`
	Limit int     `kit:"flag,inherit"`
}

func registerPeopleOps(app *kit.App) {
	kit.Handle(app, kit.OpMeta{
		Name: "followers", Group: "people", URIType: KindUser, List: true,
		Summary: "List who follows an account",
		Long: "The tab is 50 people a page and the pager is a plain next link, so an\n" +
			"account with a hundred thousand followers is two thousand requests. Set\n" +
			"--limit unless you mean all of them.",
		Args: []kit.Arg{{Name: "name", Help: "login, profile URL, or github:// URI"}},
	}, listFollowers)

	kit.Handle(app, kit.OpMeta{
		Name: "following", Group: "people", URIType: KindUser, List: true,
		Summary: "List who an account follows",
		Args:    []kit.Arg{{Name: "name", Help: "login, profile URL, or github:// URI"}},
	}, listFollowing)

	kit.Handle(app, kit.OpMeta{
		Name: "members", Group: "people", URIType: KindUser, List: true,
		Summary: "List an organization's public members",
		Long: "Public members only, which is the organization's own choice per person\n" +
			"and not something a token would widen for someone outside the org.",
		Args: []kit.Arg{{Name: "name", Help: "organization login, URL, or github:// URI"}},
	}, listMembers)

	kit.Handle(app, kit.OpMeta{
		Name: "stars", Group: "people", URIType: KindRepo, List: true,
		Aliases: []string{"starred"},
		Summary: "List what an account has starred",
		Args:    []kit.Arg{{Name: "name", Help: "login, profile URL, or github:// URI"}},
	}, listStarred)

	kit.Handle(app, kit.OpMeta{
		Name: "owned", Group: "people", URIType: KindRepo, List: true,
		Summary: "List an account's repositories as the profile shows them",
		Long: "This reads the profile's repositories tab, which is the only surface that\n" +
			"lists forks and archived repositories in the account's own order. It is\n" +
			"not called repos because `github repos --owner name` already exists, goes\n" +
			"through search, and is the better tool when you want to filter or sort.",
		Args: []kit.Arg{{Name: "name", Help: "login, profile URL, or github:// URI"}},
	}, listAccountRepos)

	kit.Handle(app, kit.OpMeta{
		Name: "gists", Group: "people", URIType: KindGist, List: true,
		Summary: "List an account's public gists",
		Args:    []kit.Arg{{Name: "name", Help: "login, profile URL, or github:// URI"}},
	}, listGists)

	kit.Handle(app, kit.OpMeta{
		Name: "gist", Group: "people", URIType: KindGist, Single: true, Resolver: true,
		Summary: "Read one gist with its files",
		Long: "The index gives each file's first few lines only, which is what the page\n" +
			"renders. With --content each file is fetched whole from the raw host.",
		Args: []kit.Arg{{Name: "ref", Help: "gist id, gist URL, or github:// URI"}},
	}, getGist)

	kit.Handle(app, kit.OpMeta{
		Name: "contributions", Group: "people", URIType: KindContribution, List: true,
		Aliases: []string{"calendar"},
		Summary: "Read an account's contribution calendar, one record per day",
		Long: "The count is not on the square. Each square points at a tooltip by id and\n" +
			"the tooltip holds the sentence with the number in it, so this indexes the\n" +
			"tooltips first and reads the squares against that index.",
		Args: []kit.Arg{{Name: "name", Help: "login, profile URL, or github:// URI"}},
	}, listContributions)

	kit.Handle(app, kit.OpMeta{
		Name: "activity", Group: "people", URIType: KindEvent, List: true,
		Aliases: []string{"events", "feed"},
		Summary: "Read a public activity feed",
		Long: "One login gives that person's public events; one owner/name gives that\n" +
			"repository's commit feed. Both are Atom, both are public, and neither has\n" +
			"a pager, so a feed is however many entries GitHub decided to put in it.",
		Args: []kit.Arg{{Name: "ref", Help: "a login, or owner/name"}},
	}, listActivity)
}

func listFollowers(ctx context.Context, in accountListIn, emit func(*Account) error) error {
	login, err := ResolveRef(KindUser, in.Name)
	if err != nil {
		return err
	}
	return in.C.Followers(ctx, login, in.Limit, byValue(emit))
}

func listFollowing(ctx context.Context, in accountListIn, emit func(*Account) error) error {
	login, err := ResolveRef(KindUser, in.Name)
	if err != nil {
		return err
	}
	return in.C.Following(ctx, login, in.Limit, byValue(emit))
}

func listMembers(ctx context.Context, in accountListIn, emit func(*Account) error) error {
	login, err := ResolveRef(KindOrg, in.Name)
	if err != nil {
		return err
	}
	return in.C.Members(ctx, login, in.Limit, byValue(emit))
}

func listStarred(ctx context.Context, in accountListIn, emit func(*Repo) error) error {
	login, err := ResolveRef(KindUser, in.Name)
	if err != nil {
		return err
	}
	return in.C.Starred(ctx, login, in.Limit, byValue(emit))
}

func listAccountRepos(ctx context.Context, in accountListIn, emit func(*Repo) error) error {
	login, err := ResolveRef(KindUser, in.Name)
	if err != nil {
		return err
	}
	return in.C.ReposAsShown(ctx, login, in.Limit, byValue(emit))
}

func listGists(ctx context.Context, in accountListIn, emit func(*Gist) error) error {
	login, err := ResolveRef(KindUser, in.Name)
	if err != nil {
		return err
	}
	return in.C.Gists(ctx, login, in.Limit, byValue(emit))
}

func getGist(ctx context.Context, in gistIn, emit func(*Gist) error) error {
	id, err := ResolveRef(KindGist, in.Ref)
	if err != nil {
		return err
	}
	g, err := in.C.Gist(ctx, id, in.Content)
	if err != nil {
		return err
	}
	return emit(g)
}

func listContributions(ctx context.Context, in contributionsIn, emit func(*ContributionDay) error) error {
	login, err := ResolveRef(KindUser, in.Name)
	if err != nil {
		return err
	}
	return in.C.Contributions(ctx, login, in.Year, byValue(emit))
}

// listActivity does not resolve the reference, because the feed takes both a
// login and an owner/name and the reader tells them apart itself. Sending it
// through ResolveRef would force a choice that neither kind wins.
func listActivity(ctx context.Context, in activityIn, emit func(*Event) error) error {
	return in.C.Activity(ctx, strings.TrimPrefix(in.Ref, BaseURL+"/"), in.Limit, byValue(emit))
}

// --- discovery and statistics ---

type trendingIn struct {
	C              *Client `kit:"inject"`
	Since          string  `kit:"flag" help:"daily, weekly, or monthly"`
	Language       string  `kit:"flag" help:"a language slug, as it appears in the trending URL"`
	SpokenLanguage string  `kit:"flag,name=spoken" help:"a two-letter natural language code"`
	Developers     bool    `kit:"flag" help:"list trending developers instead of repositories"`
	Limit          int     `kit:"flag,inherit"`
}

type topicIn struct {
	C    *Client `kit:"inject"`
	Name string  `kit:"arg" help:"a topic slug, a topic URL, or a github:// URI"`
}

type repoListIn struct {
	C     *Client `kit:"inject"`
	Ref   string  `kit:"arg" help:"owner/name, or any URL from the repository"`
	Limit int     `kit:"flag,inherit"`
}

type repoRefIn struct {
	C   *Client `kit:"inject"`
	Ref string  `kit:"arg" help:"owner/name, or any URL from the repository"`
}

func registerDiscoverOps(app *kit.App) {
	kit.Handle(app, kit.OpMeta{
		Name: "trending", Group: "discover", URIType: KindRepo, List: true,
		Summary: "List what is trending",
		Long: "Trending is the clearest case for reading pages. There is no JSON version\n" +
			"of it anywhere, with a token or without, so a page decoder is not a\n" +
			"fallback here, it is the only implementation that can exist.",
	}, listTrending)

	kit.Handle(app, kit.OpMeta{
		Name: "topic", Group: "discover", URIType: KindTopic, Single: true, Resolver: true,
		Summary: "Read one topic page",
		Long: "The search result for a topic has a name and a blurb. The page has the long\n" +
			"description, the logo, who created the thing, when it was released, the\n" +
			"Wikipedia link, and the related topics, which is most of what makes a topic\n" +
			"worth a record.",
		Args: []kit.Arg{{Name: "name", Help: "topic slug, URL, or github:// URI"}},
	}, getTopic)

	kit.Handle(app, kit.OpMeta{
		Name: "forks", Group: "discover", URIType: KindRepo, List: true,
		Summary: "List a repository's public forks",
		Args:    []kit.Arg{{Name: "ref", Help: "owner/name, or any URL from the repository"}},
	}, listForks)

	kit.Handle(app, kit.OpMeta{
		Name: "contributors", Group: "discover", URIType: KindContributor, List: true,
		Summary: "List contributors with their commit, addition, and deletion counts",
		Long: "This reads the contributor graph's own data route, which answers 202 with\n" +
			"an empty body while GitHub computes the numbers. That is normal rather\n" +
			"than an error, so the first call on a large repository waits a few seconds\n" +
			"and every call after it is instant.",
		Args: []kit.Arg{{Name: "ref", Help: "owner/name, or any URL from the repository"}},
	}, listContributors)

	kit.Handle(app, kit.OpMeta{
		Name: "languages", Group: "discover", URIType: KindRepo, List: true,
		Summary: "Report the language histogram, one record per language",
		Args:    []kit.Arg{{Name: "ref", Help: "owner/name, or any URL from the repository"}},
	}, listLanguages)

	kit.Handle(app, kit.OpMeta{
		Name: "stats", Group: "discover", URIType: KindRepo, Single: true,
		Summary: "Report a repository's counts and nothing else",
		Long: "Every field here is on the repository record too. The point of having it\n" +
			"separately is that a record with eight numbers in it is something you can\n" +
			"store once a day and diff, and a record with a readme in it is not.",
		Args: []kit.Arg{{Name: "ref", Help: "owner/name, or any URL from the repository"}},
	}, getStats)
}

func listTrending(ctx context.Context, in trendingIn, emit func(any) error) error {
	opts := TrendingOptions{
		Since:          in.Since,
		Language:       in.Language,
		SpokenLanguage: in.SpokenLanguage,
		Limit:          in.Limit,
	}
	if in.Developers {
		return in.C.TrendingDevelopers(ctx, opts, func(a Account) error { return emit(&a) })
	}
	return in.C.Trending(ctx, opts, func(t Trending) error { return emit(&t) })
}

func getTopic(ctx context.Context, in topicIn, emit func(*Topic) error) error {
	slug, err := ResolveRef(KindTopic, in.Name)
	if err != nil {
		return err
	}
	t, err := in.C.TopicPage(ctx, slug)
	if err != nil {
		return err
	}
	return emit(t)
}

func listForks(ctx context.Context, in repoListIn, emit func(*Repo) error) error {
	repo, err := ResolveRepo(in.Ref)
	if err != nil {
		return err
	}
	return in.C.Forks(ctx, repo, in.Limit, byValue(emit))
}

type contributorsIn struct {
	C     *Client `kit:"inject"`
	Ref   string  `kit:"arg" help:"owner/name, or any URL from the repository"`
	Weeks bool    `kit:"flag" help:"keep the per-week breakdown, which is large"`
	Limit int     `kit:"flag,inherit"`
}

func listContributors(ctx context.Context, in contributorsIn, emit func(*Contributor) error) error {
	repo, err := ResolveRepo(in.Ref)
	if err != nil {
		return err
	}
	return in.C.Contributors(ctx, repo, ContributorOptions{Weeks: in.Weeks, Limit: in.Limit}, byValue(emit))
}

func listLanguages(ctx context.Context, in repoRefIn, emit func(*LanguageShare) error) error {
	repo, err := ResolveRepo(in.Ref)
	if err != nil {
		return err
	}
	return in.C.Languages(ctx, repo, byValue(emit))
}

func getStats(ctx context.Context, in repoRefIn, emit func(*RepoStats) error) error {
	repo, err := ResolveRepo(in.Ref)
	if err != nil {
		return err
	}
	s, err := in.C.Stats(ctx, repo)
	if err != nil {
		return err
	}
	return emit(s)
}

// --- meta ---

func registerMetaOps(app *kit.App) {
	kit.Handle(app, kit.OpMeta{
		Name: "url", Group: "meta", Single: true,
		Aliases: []string{"classify"},
		Summary: "Parse a reference and report what it names",
		Long: "url does no network work. It takes anything a person might paste, a\n" +
			"github.com URL, a github:// URI, owner/name, owner/name#123, a blob URL\n" +
			"with a line anchor, and returns the kind, the canonical id, the URI, and\n" +
			"the https location.",
		Args: []kit.Arg{{Name: "ref", Help: "any github reference"}},
	}, parseRef)

	kit.Handle(app, kit.OpMeta{
		Name: "routes", Group: "meta", List: true,
		Summary: "List which surface answers for which route",
		Long: "This is the index the readers work from: for each route, the surface that\n" +
			"answers it best and what to fall back to when that surface declines. A\n" +
			"route that is not in the table is unsupported rather than guessed at.",
	}, listRoutes)
}

type parseIn struct {
	Ref string `kit:"arg" help:"any github reference"`
}

func parseRef(_ context.Context, in parseIn, emit func(*Ident) error) error {
	id, err := Parse(in.Ref)
	if err != nil {
		return err
	}
	return emit(&id)
}

type noIn struct{}

func listRoutes(_ context.Context, _ noIn, emit func(*RouteInfo) error) error {
	return emitEach(Routes, emit)
}
