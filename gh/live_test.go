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
// `make fixtures` runs these with recording on, which is how the offline
// scenario suite gets its data.

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
		// GitHub's symbol analyser answers null about half the time and the
		// same list a second later, on both surfaces, with any headers. Blob
		// retries twice, and past that the honest report is "unavailable"
		// rather than a hard failure here. not_analyzed for a Go file would be
		// a real change and does fail.
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
			t.Logf("symbols %s after three tries, the analyser was cold", f.SymbolsStatus)
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
