package gh

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// live_test.go talks to github.com. It is skipped unless GITHUB_LIVE=1, so the
// normal test run stays offline and deterministic, and this is what you reach
// for when you want to know whether a surface still looks the way the spec says
// it does.
//
// These are the tests that catch the failure this tool cannot survive: GitHub
// moving something. Nothing offline can see that, because an offline test
// checks the parser against bytes that were already parsed once. So the
// assertions here are deliberately about shape rather than values. A star count
// changes hourly and pinning one turns a test into a clock.

func liveClient(t *testing.T) *Client {
	t.Helper()
	if os.Getenv("GITHUB_LIVE") != "1" {
		t.Skip("set GITHUB_LIVE=1 to run against github.com")
	}
	cfg := Defaults
	cfg.CacheDir = t.TempDir()
	return NewClient(cfg)
}

func TestLiveRepo(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	r, err := c.Repo(ctx, "gohugoio/hugo", RepoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Owner != "gohugoio" || r.Name != "hugo" {
		t.Fatalf("identity: %+v", r.Base)
	}
	// Each of these comes from a different block of the page, so between them
	// they say the whole merge worked and not just one decoder.
	if r.DefaultBranch == "" {
		t.Error("default branch missing, codeViewLayoutRoute did not decode")
	}
	if r.Stars == nil {
		t.Error("stars missing, sidebarAbout did not decode")
	}
	if r.HeadSHA == "" {
		t.Error("head sha missing, codeViewRepoRoute did not decode")
	}
	if len(r.Tree) == 0 {
		t.Error("tree empty")
	}
	if r.License == "" {
		t.Error("licence missing, the octicon-law selector stopped matching")
	}
	if r.Language == "" {
		t.Error("language missing, both the language bar and the search fallback came up empty")
	}
	if len(r.Extra) > 0 {
		// Not a failure by itself, but it is how a new upstream field announces
		// itself, so it gets printed.
		t.Logf("unmodelled keys: %s", string(r.Extra))
	}
	out, _ := json.MarshalIndent(r, "", "  ")
	t.Logf("%s", out)
}

// TestLiveAccount covers both profile templates. sindresorhus has every vcard
// row a user profile can have except email, torvalds has achievements and no
// bio, and golang is the organization template, so between the three every
// branch of the decoder runs.
func TestLiveAccount(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Run("user", func(t *testing.T) {
		a, err := c.Account(ctx, "sindresorhus")
		if err != nil {
			t.Fatal(err)
		}
		if a.Type != "User" {
			t.Fatalf("type %q, the vcard template stopped matching", a.Type)
		}
		if a.Name == "" {
			t.Error("name missing, p-name stopped matching")
		}
		if a.Bio == "" {
			t.Error("bio missing, data-bio-text stopped matching")
		}
		if a.Website == "" {
			t.Error("website missing, the itemprop=url row stopped matching")
		}
		if len(a.SocialLinks) == 0 {
			t.Error("social links missing, the itemprop=social rows stopped matching")
		}
		if a.Followers == nil {
			t.Error("followers missing, the tab=followers link stopped matching")
		}
		if a.RepoCount == nil {
			t.Error("repo count missing, the tab counters stopped matching")
		}
		if a.DatabaseID == nil {
			t.Error("database id missing, the avatar URL shape changed")
		}
		if len(a.PinnedRepos) == 0 {
			t.Error("pinned repos missing")
		}
		if len(a.Organizations) == 0 {
			t.Error("organizations missing, the hovercard-type hook changed")
		}
		logExtra(t, "user", a.Extra)
	})

	t.Run("user_achievements", func(t *testing.T) {
		a, err := c.Account(ctx, "torvalds")
		if err != nil {
			t.Fatal(err)
		}
		if a.Company == "" {
			t.Error("company missing, the worksFor row stopped matching")
		}
		if a.Location == "" {
			t.Error("location missing, the homeLocation row stopped matching")
		}
		if len(a.Achievements) == 0 {
			t.Error("achievements missing")
		}
		logExtra(t, "torvalds", a.Extra)
	})

	t.Run("org", func(t *testing.T) {
		o, err := c.Org(ctx, "golang")
		if err != nil {
			t.Fatal(err)
		}
		if o.Type != "Organization" {
			t.Fatalf("type %q, the orghead template stopped matching", o.Type)
		}
		if o.Name == "" {
			t.Error("name missing, the orghead h1 stopped matching")
		}
		if o.Bio == "" {
			t.Error("description missing, the muted sibling stopped matching")
		}
		if o.Website == "" {
			t.Error("website missing, itemprop=url stopped matching")
		}
		if o.Followers == nil {
			t.Error("followers missing, the /followers link stopped matching")
		}
		if len(o.Members) == 0 {
			t.Error("members strip empty, member-avatar stopped matching")
		}
		// The organization counters are empty spans marked "Not available"
		// without a session. If one ever arrives populated the tool should
		// start reading it, so this asserts the absence rather than ignoring it.
		if o.RepoCount != nil {
			t.Errorf("repo count %d arrived on an org page, the counters are no longer session-gated", *o.RepoCount)
		}
		logExtra(t, "org", o.Extra)
	})

	t.Run("org_rejects_user", func(t *testing.T) {
		if _, err := c.Org(ctx, "torvalds"); err == nil {
			t.Error("Org accepted a user profile")
		}
	})
}

// TestLiveThread covers the three thread surfaces, which is really three
// unrelated decoders sharing a record. golang/go#1 is the oldest issue on the
// site and has a milestone, a label, and eighty-eight timeline events, so it
// exercises every branch of the Relay decoder at once.
func TestLiveThread(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("issue", func(t *testing.T) {
		iss, err := c.Issue(ctx, "golang/go", 1)
		if err != nil {
			t.Fatal(err)
		}
		if iss.Kind != KindIssue || iss.Number != 1 {
			t.Fatalf("identity: %+v", iss.Base)
		}
		if iss.Title == "" {
			t.Error("title missing, the issue node stopped decoding")
		}
		if iss.Body == "" {
			t.Error("body missing")
		}
		if iss.State != "closed" {
			t.Errorf("state %q, expected the lowercased enum", iss.State)
		}
		if iss.Author.Login == "" {
			t.Error("author missing")
		}
		if len(iss.Labels) == 0 {
			t.Error("labels missing, the label edges moved")
		}
		if iss.Milestone == nil {
			t.Error("milestone missing")
		}
		if iss.NodeID == "" || iss.DatabaseID == nil {
			t.Error("ids missing")
		}
		logExtra(t, "issue", iss.Extra)
	})

	t.Run("issue_timeline", func(t *testing.T) {
		n, comments := 0, 0
		err := c.Timeline(ctx, "golang/go", 1, 20, func(it TimelineItem) error {
			n++
			if it.Type == "" {
				t.Error("timeline item with no type")
			}
			if it.Type == "issue_comment" {
				comments++
				if it.Body == "" {
					t.Error("comment with no body")
				}
			}
			logExtra(t, "timeline "+it.Type, it.Extra)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatal("no timeline items, frontTimelineItems stopped decoding")
		}
		if comments == 0 {
			t.Error("no comments in the timeline, the IssueComment shape changed")
		}
	})

	t.Run("pull", func(t *testing.T) {
		pr, err := c.PullRequest(ctx, "cli/cli", 9000)
		if err != nil {
			t.Fatal(err)
		}
		if pr.Kind != KindPR || pr.Number != 9000 {
			t.Fatalf("identity: %+v", pr.Base)
		}
		if pr.State != "merged" || !pr.Merged {
			t.Errorf("state %q merged %v, the route stopped reporting the merge", pr.State, pr.Merged)
		}
		if pr.BaseRef == "" || pr.HeadRef == "" {
			t.Error("refs missing")
		}
		if pr.HeadOID == "" {
			t.Error("head sha missing")
		}
		if pr.MergedBy == nil {
			t.Error("merged-by missing")
		}
		// The body only ever comes from the hovercard. If it is empty the
		// fragment stopped answering, and the pull request record loses the one
		// field the route cannot supply.
		if pr.Body == "" {
			t.Error("body missing, the hovercard fragment stopped answering")
		}
		logExtra(t, "pull", pr.Extra)
	})

	t.Run("pull_commits", func(t *testing.T) {
		n := 0
		err := c.PullCommits(ctx, "cli/cli", 9000, 10, func(cm Commit) error {
			n++
			if cm.SHA == "" || cm.Subject == "" {
				t.Errorf("thin commit: %+v", cm)
			}
			if len(cm.Authors) == 0 {
				t.Errorf("%s has no authors", cm.SHA)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatal("no commits, commitGroups stopped decoding")
		}
	})

	t.Run("discussion", func(t *testing.T) {
		d, err := c.Discussion(ctx, "google/docsy-example", 479)
		if err != nil {
			t.Fatal(err)
		}
		if d.Repo != "google/docsy-example" {
			t.Errorf("repo %q, the sidebar data-url shape changed", d.Repo)
		}
		if d.Title == "" {
			t.Error("title missing")
		}
		if d.BodyHTML == "" {
			t.Error("body missing, the QAPage block stopped carrying text")
		}
		if !d.IsAnswered {
			t.Error("answered flag missing, both the pill and acceptedAnswer stopped matching")
		}
		if d.Category == "" {
			t.Error("category missing, the sidebar link stopped matching")
		}
		if d.Upvotes == nil {
			t.Error("upvotes missing")
		}
		if d.NodeID == "" {
			t.Error("node id missing, data-gid stopped matching")
		}
		logExtra(t, "discussion", d.Extra)
	})

	t.Run("org_discussion", func(t *testing.T) {
		// An organization discussion is served from /orgs/{login}/ and the
		// record still has to name the repository that owns it.
		d, err := c.Discussion(ctx, "community", 1)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(d.Repo, "/") {
			t.Errorf("repo %q is not owner/name", d.Repo)
		}
		if d.Author.Login == "" {
			t.Error("author missing")
		}
		if len(d.Labels) == 0 {
			t.Error("labels missing")
		}
	})
}

// TestLiveSearch walks every search type that works without a session. It is
// one test rather than nine because the value is in the comparison: when one
// type changes shape and the other eight do not, the failure says so.
func TestLiveSearch(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Each case asserts the one field that proves the decoder ran rather than
	// just that a result came back.
	cases := []struct {
		typ   string
		query string
		run   func(context.Context, string) (int, string, error)
	}{
		{SearchRepos, "hugo", func(ctx context.Context, q string) (int, string, error) {
			n, bad := 0, ""
			err := c.SearchRepositories(ctx, q, 5, func(r Repo) error {
				n++
				if r.Owner == "" || r.Name == "" {
					bad = "identity empty"
				}
				if r.Stars == nil {
					bad = "stars nil for " + r.ID
				}
				logExtra(t, "repo "+r.ID, r.Extra)
				return nil
			})
			return n, bad, err
		}},
		{SearchIssues, "repo:golang/go generics", func(ctx context.Context, q string) (int, string, error) {
			n, bad := 0, ""
			err := c.SearchIssuesAndPulls(ctx, q, SearchIssues, 5, func(th Thread) error {
				n++
				if th.Number == 0 || th.Repo == "" {
					bad = "identity empty"
				}
				if th.Author.Login == "" {
					bad = "author empty, author_name moved"
				}
				logExtra(t, "issue "+th.ID, th.Extra)
				return nil
			})
			return n, bad, err
		}},
		{SearchPulls, "repo:golang/go generics", func(ctx context.Context, q string) (int, string, error) {
			n, bad := 0, ""
			err := c.SearchIssuesAndPulls(ctx, q, SearchPulls, 5, func(th Thread) error {
				n++
				if th.Kind != KindPR {
					bad = th.ID + " is not classified as a pull request"
				}
				return nil
			})
			return n, bad, err
		}},
		{SearchUsers, "torvalds", func(ctx context.Context, q string) (int, string, error) {
			n, bad := 0, ""
			err := c.SearchAccounts(ctx, q, 5, func(a Account) error {
				n++
				if a.Login == "" {
					bad = "login empty"
				}
				logExtra(t, "user "+a.ID, a.Extra)
				return nil
			})
			return n, bad, err
		}},
		{SearchCommits, "repo:golang/go fix", func(ctx context.Context, q string) (int, string, error) {
			n, bad := 0, ""
			err := c.SearchCommitsBy(ctx, q, 5, func(cm Commit) error {
				n++
				if cm.SHA == "" {
					bad = "sha empty"
				}
				if cm.Subject == "" {
					bad = "subject empty for " + cm.SHA
				}
				logExtra(t, "commit "+cm.SHA, cm.Extra)
				return nil
			})
			return n, bad, err
		}},
		{SearchDiscussions, "hugo", func(ctx context.Context, q string) (int, string, error) {
			n, bad := 0, ""
			err := c.SearchDiscussionsBy(ctx, q, 5, func(d Discussion) error {
				n++
				if d.Number == 0 {
					bad = "number zero"
				}
				// hl_title arrives entity-escaped here and nowhere else, so a
				// stray &#x2F; means stripHL stopped unescaping.
				if strings.Contains(d.Title, "&#") {
					bad = "title still escaped: " + d.Title
				}
				logExtra(t, "discussion "+d.ID, d.Extra)
				return nil
			})
			return n, bad, err
		}},
		{SearchTopics, "go", func(ctx context.Context, q string) (int, string, error) {
			n, bad := 0, ""
			err := c.SearchTopicsBy(ctx, q, 5, func(tp Topic) error {
				n++
				if tp.Name == "" {
					bad = "name empty"
				}
				logExtra(t, "topic "+tp.ID, tp.Extra)
				return nil
			})
			return n, bad, err
		}},
		{SearchPackages, "hugo", func(ctx context.Context, q string) (int, string, error) {
			n, bad := 0, ""
			err := c.SearchPackagesBy(ctx, q, 5, func(pk Package) error {
				n++
				if pk.Name == "" {
					bad = "name empty"
				}
				if pk.Type == "" {
					bad = "type empty for " + pk.Name
				}
				logExtra(t, "package "+pk.ID, pk.Extra)
				return nil
			})
			return n, bad, err
		}},
		{SearchWikis, "hugo", func(ctx context.Context, q string) (int, string, error) {
			n, bad := 0, ""
			err := c.SearchWikisBy(ctx, q, 5, func(w WikiPage) error {
				n++
				if w.Repo == "" {
					bad = "repo empty"
				}
				logExtra(t, "wiki "+w.ID, w.Extra)
				return nil
			})
			return n, bad, err
		}},
		{SearchMarket, "lint", func(ctx context.Context, q string) (int, string, error) {
			n, bad := 0, ""
			err := c.SearchMarketplace(ctx, q, 5, func(a Action) error {
				n++
				if a.Slug == "" {
					bad = "slug empty"
				}
				logExtra(t, "action "+a.ID, a.Extra)
				return nil
			})
			return n, bad, err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			n, bad, err := tc.run(ctx, tc.query)
			if err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				t.Fatalf("no results for %q, the type or the envelope changed", tc.query)
			}
			if bad != "" {
				t.Error(bad)
			}
		})
	}
}

// TestLiveContents covers the tree route, the blob route, and raw bytes. The
// three are one test because the interesting question is whether the metadata
// and the bytes still agree with each other.
func TestLiveContents(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	t.Run("tree", func(t *testing.T) {
		var got []TreeEntry
		err := c.Tree(ctx, "cli/cli", "pkg", TreeOptions{}, func(e TreeEntry) error {
			got = append(got, e)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) < 5 {
			t.Fatalf("only %d entries under pkg/", len(got))
		}
		e := got[0]
		if e.Name == "" || !strings.HasPrefix(e.Path, "pkg/") {
			t.Errorf("path is not repository-relative: %+v", e)
		}
		if e.Type == "" {
			t.Error("contentType missing")
		}
		// An empty ref means HEAD in the URL and a resolved SHA on the record.
		if len(e.Ref) != 40 {
			t.Errorf("ref %q is not a resolved commit", e.Ref)
		}
		logExtra(t, "tree", e.Extra)
	})

	t.Run("tree_recursive", func(t *testing.T) {
		n := 0
		err := c.Tree(ctx, "cli/cli", "pkg/iostreams", TreeOptions{Recursive: true, Limit: 12}, func(TreeEntry) error {
			n++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatal("recursive walk emitted nothing")
		}
	})

	t.Run("blob", func(t *testing.T) {
		f, err := c.Blob(ctx, "cli/cli", "pkg/iostreams/iostreams.go", BlobOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if f.Language != "Go" {
			t.Errorf("language %q, the blob layout route stopped decoding", f.Language)
		}
		if f.Lines == nil || *f.Lines < 100 {
			t.Errorf("line count %v for a 13 KB file", f.Lines)
		}
		// GitHub's symbol analyser answers null on every file of every
		// repository tried now, on both surfaces, signed out. So "unavailable"
		// is the expected answer here rather than a failure, and if the block
		// ever comes back this asserts it is shaped right. not_analyzed for a
		// Go file would be a real change and does fail.
		switch f.SymbolsStatus {
		case "ok":
			if len(f.Symbols) == 0 {
				t.Fatal("status ok with no symbols")
			}
			s := f.Symbols[0]
			if s.Name == "" || s.Kind == "" || s.ExtentEnd == 0 {
				t.Errorf("symbol is half empty: %+v", s)
			}
		case "unavailable", "timed_out":
			t.Logf("symbols %s, the analyser did not answer", f.SymbolsStatus)
		default:
			t.Errorf("symbols status %q for a Go file", f.SymbolsStatus)
		}
		logExtra(t, "blob", f.Extra)
	})

	t.Run("blob_markdown", func(t *testing.T) {
		f, err := c.Blob(ctx, "cli/cli", "README.md", BlobOptions{Content: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(f.TOC) == 0 {
			t.Error("no table of contents on a rendered markdown file")
		}
		if f.RichText == "" {
			t.Error("no rendered html")
		}
		if !strings.Contains(f.Content, "gh") {
			t.Errorf("content does not look like the readme: %.60q", f.Content)
		}
		if f.Via["content"] != "raw" {
			t.Errorf("content provenance is %q", f.Via["content"])
		}
	})

	t.Run("raw", func(t *testing.T) {
		b, err := c.Raw(ctx, "cli/cli", "trunk", "go.mod")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(b), "module ") {
			t.Errorf("go.mod does not start with a module line: %.40q", b)
		}
	})
}

// TestLiveHistory covers the six surfaces the history layer reads. They are one
// test because they are one question asked six ways, and when GitHub changes a
// payload it is usually the disagreement between two of them that shows it.
func TestLiveHistory(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	t.Run("commits", func(t *testing.T) {
		var got []Commit
		err := c.Commits(ctx, "cli/cli", CommitOptions{Limit: 40}, func(cm Commit) error {
			got = append(got, cm)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		// A page is 35, so 40 proves the cursor came back and was accepted.
		if len(got) != 40 {
			t.Fatalf("walked %d commits, want the limit of 40", len(got))
		}
		cm := got[0]
		if len(cm.SHA) != 40 {
			t.Errorf("sha %q", cm.SHA)
		}
		if cm.Subject == "" {
			t.Error("subject empty, shortMessage is often null and the markdown fallback did not run")
		}
		if cm.AuthoredAt == nil {
			t.Error("no authored date")
		}
		if len(cm.Authors) == 0 {
			t.Error("no authors")
		}
		if cm.DateGroup == "" {
			t.Error("no day heading, commitGroups lost its title")
		}
		if cm.ID != "cli/cli@"+cm.SHA {
			t.Errorf("id %q", cm.ID)
		}
		logExtra(t, "commit", cm.Extra)
	})

	t.Run("commits_filtered", func(t *testing.T) {
		// The filters go to GitHub, so a path that exists and an author who
		// touched it should come back non-empty and every record should be on
		// that path.
		n := 0
		err := c.Commits(ctx, "cli/cli", CommitOptions{Path: "go.mod", Limit: 5}, func(Commit) error {
			n++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatal("no commits touched go.mod, the path filter is being dropped")
		}
	})

	t.Run("commit", func(t *testing.T) {
		cm, err := c.CommitInfo(ctx, "cli/cli", "trunk", CommitInfoOptions{Files: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(cm.SHA) != 40 {
			t.Errorf("a branch name did not resolve to a sha: %q", cm.SHA)
		}
		if cm.Additions == nil || cm.Deletions == nil {
			t.Error("headerInfo did not decode")
		}
		if len(cm.Files) == 0 {
			t.Fatal("no files, diffEntryData did not decode")
		}
		f := cm.Files[0]
		if f.Path == "" || f.Status == "" {
			t.Errorf("half a file change: %+v", f)
		}
		if len(cm.Parents) == 0 {
			t.Error("no parents on a commit that is not the root")
		}
		logExtra(t, "commit info", cm.Extra)
	})

	t.Run("verify", func(t *testing.T) {
		// GitHub signs every commit it makes itself, so a merge on cli/cli is
		// the reliable case. This is the only surface that says so.
		var head []*Commit
		err := c.Commits(ctx, "cli/cli", CommitOptions{Limit: 5}, func(cm Commit) error {
			head = append(head, &cm)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.VerifyCommits(ctx, "cli/cli", head); err != nil {
			t.Fatal(err)
		}
		signed := 0
		for _, cm := range head {
			if cm.Verification != "" {
				signed++
			}
		}
		if signed == 0 {
			t.Error("commit search reported verification on none of five commits")
		}
	})

	t.Run("branches", func(t *testing.T) {
		var got []GitRef
		err := c.Branches(ctx, "cli/cli", RefOptions{Limit: 10}, func(r GitRef) error {
			got = append(got, r)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatal("no branches")
		}
		// The page carries what the protocol cannot: who last pushed and when.
		hasAuthor := false
		for _, r := range got {
			if r.Type != "branch" {
				t.Errorf("type %q on a branch", r.Type)
			}
			if r.Author != nil && r.AuthoredAt != nil {
				hasAuthor = true
			}
		}
		if !hasAuthor {
			t.Error("no branch carried an author, which is the only reason to read the page")
		}
		if got[0].Repo != "cli/cli" {
			t.Errorf("repo %q", got[0].Repo)
		}
	})

	t.Run("branches_complete", func(t *testing.T) {
		// The advertisement has no cap, so it should beat the page's list and
		// every entry should carry a SHA.
		var got []GitRef
		err := c.Branches(ctx, "cli/cli", RefOptions{Complete: true}, func(r GitRef) error {
			got = append(got, r)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) < 5 {
			t.Fatalf("the advertisement gave %d branches", len(got))
		}
		def := 0
		for _, r := range got {
			if len(r.SHA) != 40 {
				t.Errorf("%s has sha %q", r.Name, r.SHA)
			}
			if r.IsDefault {
				def++
			}
		}
		if def != 1 {
			t.Errorf("%d branches claim to be the default", def)
		}
	})

	t.Run("tags", func(t *testing.T) {
		var got []GitRef
		err := c.Tags(ctx, "cli/cli", RefOptions{Limit: 50}, func(r GitRef) error {
			got = append(got, r)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		// The point of reading the protocol by default is that neither the feed
		// nor the page gives more than ten.
		if len(got) != 50 {
			t.Fatalf("%d tags, want 50 from a repository with hundreds", len(got))
		}
		annotated := 0
		for _, r := range got {
			if r.Type != "tag" {
				t.Errorf("type %q on a tag", r.Type)
			}
			if r.PeeledSHA != "" {
				annotated++
			}
		}
		t.Logf("%d of %d tags are annotated", annotated, len(got))
	})

	t.Run("refs", func(t *testing.T) {
		// No limit: it is one response either way, and a limit here truncates
		// inside the branch list and never reaches the tags.
		heads, tags := 0, 0
		err := c.Refs(ctx, "cli/cli", RefOptions{}, func(r GitRef) error {
			switch r.Type {
			case "branch":
				heads++
			case "tag":
				tags++
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if heads == 0 || tags == 0 {
			t.Errorf("refs gave %d branches and %d tags, it should give both", heads, tags)
		}
	})

	t.Run("default_branch", func(t *testing.T) {
		// symref=HEAD comes free with the advertisement and should agree with
		// the repository page, which reads it from a completely different place.
		name, err := c.DefaultBranch(ctx, "cli/cli")
		if err != nil {
			t.Fatal(err)
		}
		if name != "trunk" {
			t.Errorf("default branch %q, want trunk", name)
		}
	})

	t.Run("releases", func(t *testing.T) {
		var got []Release
		err := c.Releases(ctx, "cli/cli", ReleaseOptions{Limit: 15, Body: true}, func(r Release) error {
			got = append(got, r)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		// Ten a page, so fifteen proves rel="next" was found and followed.
		if len(got) != 15 {
			t.Fatalf("%d releases, want the limit of 15", len(got))
		}
		latest := 0
		for _, r := range got {
			if r.Tag == "" {
				t.Errorf("release with no tag: %+v", r.Base)
			}
			if r.IsLatest {
				latest++
			}
		}
		if latest != 1 {
			t.Errorf("%d releases are labelled Latest", latest)
		}
		if got[0].PublishedAt == nil {
			t.Error("no publish date on the newest release")
		}
		if got[0].Body == "" {
			t.Error("no release notes with Body set")
		}
		logExtra(t, "release", got[0].Extra)
	})

	t.Run("release_assets", func(t *testing.T) {
		rel, err := c.Release(ctx, "cli/cli", "v2.63.2", ReleaseOptions{Assets: true, Body: true})
		if err != nil {
			t.Fatal(err)
		}
		if rel.Tag != "v2.63.2" {
			t.Errorf("tag %q", rel.Tag)
		}
		if rel.Title == "" {
			t.Error("no title")
		}
		if rel.Author == nil {
			t.Error("no publisher, the byline link did not match")
		}
		if rel.PublishedAt == nil {
			t.Error("no publish date")
		}
		if rel.Body == "" {
			t.Error("no release notes")
		}
		// The commit the tag points at is the one thing the per-tag page has
		// that a list entry does not.
		if len(rel.CommitSHA) != 40 {
			t.Errorf("commit sha %q", rel.CommitSHA)
		}
		if len(rel.Assets) < 5 {
			t.Fatalf("%d assets, the expanded_assets fragment did not decode", len(rel.Assets))
		}
		a := rel.Assets[0]
		if a.Name == "" || a.URL == "" {
			t.Errorf("half an asset: %+v", a)
		}
		if a.SizeDisplay == "" {
			t.Error("no size on an asset")
		}
		if a.Label == "" {
			t.Error("no label, the row's first truncated span stopped matching")
		}
		if a.UpdatedAt == nil {
			t.Error("no upload time on an asset")
		}
		// Download counts are gone for a logged-out client. If they ever come
		// back this logs it rather than failing.
		if a.DownloadCount != nil {
			t.Logf("download counts are being served again: %d", *a.DownloadCount)
		}
	})

	t.Run("release_latest", func(t *testing.T) {
		// "latest" is a redirect, so this proves the decoder reads whatever it
		// lands on rather than the tag it was handed. It is also the release
		// that carries digests: GitHub started attaching them recently and old
		// releases do not have them.
		rel, err := c.Release(ctx, "cli/cli", "latest", ReleaseOptions{Assets: true})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(rel.Tag, "v") {
			t.Errorf("tag %q, the redirect target did not decode", rel.Tag)
		}
		if !rel.IsLatest {
			t.Error("the latest release is not labelled Latest")
		}
		if len(rel.Assets) == 0 {
			t.Fatal("no assets on the latest release")
		}
		digests := 0
		for _, a := range rel.Assets {
			if strings.HasPrefix(a.Digest, "sha256:") {
				digests++
			}
		}
		if digests == 0 {
			t.Error("no sha256 digests, which is the one thing that replaced download counts")
		}
	})

	t.Run("compare", func(t *testing.T) {
		cmp, err := c.CompareRefs(ctx, "cli/cli", "v2.63.1", "v2.63.2", CompareOptions{Files: true})
		if err != nil {
			t.Fatal(err)
		}
		if cmp.CommitCount == 0 {
			t.Fatal("no commits in the range, the mailbox did not split")
		}
		if cmp.FileCount == 0 {
			t.Fatal("no files in the range")
		}
		if cmp.Additions == 0 && cmp.Deletions == 0 {
			t.Error("a release range with no line changes")
		}
		first := cmp.Commits[0]
		if len(first.SHA) != 40 || first.Subject == "" || first.AuthoredAt == nil {
			t.Errorf("half a commit from the patch: %+v", first.Base)
		}
		// The mailbox has names and emails, not logins, except where the email
		// is a noreply address. At least one of these should be.
		logins := 0
		for _, cm := range cmp.Commits {
			for _, a := range cm.Authors {
				if a.Login != "" {
					logins++
				}
			}
		}
		t.Logf("%d of %d commits gave a login through a noreply address", logins, cmp.CommitCount)
		if cmp.ID != "cli/cli@v2.63.1...v2.63.2" {
			t.Errorf("id %q", cmp.ID)
		}
	})

	t.Run("diff", func(t *testing.T) {
		// The diff is the patch without the mail headers, so it should be
		// smaller and it should not carry a From line.
		url := BaseURL + "/cli/cli/compare/v2.63.1...v2.63.2"
		diff, err := c.Diff(ctx, url)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(diff, "diff --git ") {
			t.Errorf("a diff should start with a diff header: %.60q", diff)
		}
		patch, err := c.Patch(ctx, url)
		if err != nil {
			t.Fatal(err)
		}
		if len(patch) <= len(diff) {
			t.Errorf("patch %d bytes is not bigger than diff %d bytes", len(patch), len(diff))
		}
	})
}

// TestLiveCodeSearchStaysRefused guards the one search type that answers 200
// with nothing. If GitHub ever opens it up this test fails, which is the
// notification to go implement it.
func TestLiveCodeSearchStaysRefused(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var env searchEnvelope
	if _, err := c.GetJSON(ctx, searchURL("func main", SearchCode, 1), SurfaceSearch, &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Payload.Results) > 0 {
		t.Errorf("code search returned %d results without a session, it can be implemented now",
			len(env.Payload.Results))
	}
}

func logExtra(t *testing.T, what string, extra json.RawMessage) {
	t.Helper()
	if len(extra) > 0 {
		t.Logf("%s unmodelled: %s", what, string(extra))
	}
}
