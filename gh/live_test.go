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
