package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── parser unit tests ────────────────────────────────────────────────────────

const trendingFixture = `<!DOCTYPE html>
<html>
<body>
<article class="Box-row">
  <h2 class="h3 lh-condensed">
    <a class="Link--primary" href="/golang/go">golang / <strong>go</strong></a>
  </h2>
  <p class="col-9 color-fg-muted my-1 pr-4">The Go programming language</p>
  <span class="d-inline-block ml-0 mr-3" itemprop="programmingLanguage">Go</span>
  <a class="Link--muted d-inline-block mr-3" href="/golang/go/stargazers">
    <svg/> 123,456
  </a>
  <a class="Link--muted d-inline-block mr-3" href="/golang/go/network/members">
    <svg/> 18,900
  </a>
  <span class="d-inline-block float-sm-right">
    <span class="color-fg-muted text-small">42 stars today</span>
  </span>
</article>
<article class="Box-row">
  <h2 class="h3 lh-condensed">
    <a class="Link--primary" href="/kubernetes/kubernetes">kubernetes / <strong>kubernetes</strong></a>
  </h2>
  <p class="col-9 color-fg-muted my-1 pr-4">Production-Grade Container Scheduling</p>
  <span class="d-inline-block ml-0 mr-3" itemprop="programmingLanguage">Go</span>
  <a class="Link--muted d-inline-block mr-3" href="/kubernetes/kubernetes/stargazers">
    <svg/> 112,000
  </a>
  <a class="Link--muted d-inline-block mr-3" href="/kubernetes/kubernetes/network/members">
    <svg/> 40,200
  </a>
  <span class="d-inline-block float-sm-right">
    <span class="color-fg-muted text-small">37 stars today</span>
  </span>
</article>
</body>
</html>`

func TestParseTrending(t *testing.T) {
	repos := ParseTrending(trendingFixture)
	if len(repos) != 2 {
		t.Fatalf("want 2 repos, got %d", len(repos))
	}
	r := repos[0]
	if r.Rank != 1 {
		t.Errorf("rank: want 1, got %d", r.Rank)
	}
	if r.FullName != "golang/go" {
		t.Errorf("full_name: want golang/go, got %q", r.FullName)
	}
	if r.Stars != 123456 {
		t.Errorf("stars: want 123456, got %d", r.Stars)
	}
	if r.Language != "Go" {
		t.Errorf("language: want Go, got %q", r.Language)
	}
	if r.PeriodStars != 42 {
		t.Errorf("period_stars: want 42, got %d", r.PeriodStars)
	}
	if r.URL != "https://github.com/golang/go" {
		t.Errorf("url: got %q", r.URL)
	}

	r2 := repos[1]
	if r2.Rank != 2 {
		t.Errorf("rank[1]: want 2, got %d", r2.Rank)
	}
	if r2.FullName != "kubernetes/kubernetes" {
		t.Errorf("full_name[1]: got %q", r2.FullName)
	}
}

const userFixture = `<!DOCTYPE html>
<html>
<body>
<span itemprop="name" class="p-name">Linus Torvalds</span>
<div class="p-note user-profile-bio mb-3 js-user-profile-bio f4">
  <div>Just a geek</div>
</div>
<span class="p-org">@linux</span>
<a class="Link--primary" href="https://kernel.org" rel="nofollow me">kernel.org</a>
<a href="/torvalds?tab=followers" data-view-component="true">
  <span class="text-bold color-fg-default">230000</span>
</a>
<a href="/torvalds?tab=following" data-view-component="true">
  <span class="text-bold color-fg-default">0</span>
</a>
<a href="/torvalds?tab=repositories" data-view-component="true">
  <span class="Counter">6</span>
</a>
</body>
</html>`

func TestParseUser(t *testing.T) {
	u, err := ParseUser(userFixture, "torvalds")
	if err != nil {
		t.Fatal(err)
	}
	if u.Login != "torvalds" {
		t.Errorf("login: want torvalds, got %q", u.Login)
	}
	if u.Name != "Linus Torvalds" {
		t.Errorf("name: want 'Linus Torvalds', got %q", u.Name)
	}
	if u.URL != "https://github.com/torvalds" {
		t.Errorf("url: got %q", u.URL)
	}
}

const reposFixture = `<!DOCTYPE html>
<html>
<body>
<ul>
<li itemprop="owns">
  <a href="/torvalds/linux" itemprop="name codeRepository">linux</a>
  <p itemprop="description">Linux kernel source tree</p>
  <span itemprop="programmingLanguage">C</span>
  <a href="/torvalds/linux/stargazers">182,000</a>
  <a href="/torvalds/linux/network/members">52,000</a>
  <relative-time datetime="2024-01-15T10:00:00Z">Jan 15</relative-time>
</li>
<li itemprop="owns">
  <a href="/torvalds/subsurface-for-dirk" itemprop="name codeRepository">subsurface-for-dirk</a>
  <p itemprop="description">My personal dive log application</p>
  <span itemprop="programmingLanguage">C++</span>
  <a href="/torvalds/subsurface-for-dirk/stargazers">1,200</a>
  <a href="/torvalds/subsurface-for-dirk/network/members">400</a>
  <relative-time datetime="2024-01-10T08:00:00Z">Jan 10</relative-time>
</li>
</ul>
</body>
</html>`

func TestParseRepos(t *testing.T) {
	repos := ParseRepos(reposFixture, "torvalds")
	if len(repos) != 2 {
		t.Fatalf("want 2 repos, got %d", len(repos))
	}
	r := repos[0]
	if r.FullName != "torvalds/linux" {
		t.Errorf("full_name: want torvalds/linux, got %q", r.FullName)
	}
	if r.Description != "Linux kernel source tree" {
		t.Errorf("description: got %q", r.Description)
	}
	if r.Language != "C" {
		t.Errorf("language: want C, got %q", r.Language)
	}
	if r.Stars != 182000 {
		t.Errorf("stars: want 182000, got %d", r.Stars)
	}
}

const atomCommitsFixture = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>tag:github.com,2008:Grit::Commit/abc1234567890abcdef</id>
    <title>Fix memory leak in parser</title>
    <published>2024-01-15T12:00:00Z</published>
    <author><name>Jane Doe</name><email>jane@example.com</email></author>
    <link href="https://github.com/foo/bar/commit/abc1234567890abcdef"/>
  </entry>
  <entry>
    <id>tag:github.com,2008:Grit::Commit/def9876543210fedcba</id>
    <title>Add test coverage for edge cases</title>
    <published>2024-01-14T08:00:00Z</published>
    <author><name>John Smith</name><email>john@example.com</email></author>
    <link href="https://github.com/foo/bar/commit/def9876543210fedcba"/>
  </entry>
</feed>`

func TestParseAtomCommits(t *testing.T) {
	commits, err := ParseAtomCommits(atomCommitsFixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("want 2 commits, got %d", len(commits))
	}
	c := commits[0]
	if c.SHA != "abc1234" {
		t.Errorf("sha: want abc1234, got %q", c.SHA)
	}
	if c.Message != "Fix memory leak in parser" {
		t.Errorf("message: got %q", c.Message)
	}
	if c.Author != "Jane Doe" {
		t.Errorf("author: got %q", c.Author)
	}
	if c.Date != "2024-01-15T12:00:00Z" {
		t.Errorf("date: got %q", c.Date)
	}
	if c.URL != "https://github.com/foo/bar/commit/abc1234567890abcdef" {
		t.Errorf("url: got %q", c.URL)
	}

	c2 := commits[1]
	if c2.Author != "John Smith" {
		t.Errorf("author[1]: got %q", c2.Author)
	}
}

const atomReleasesFixture = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>tag:github.com,2008:Repository/12345:v1.23.0</id>
    <title>Go 1.23.0</title>
    <published>2024-08-13T18:00:00Z</published>
    <updated>2024-08-13T18:00:00Z</updated>
    <author><name>golang</name></author>
    <link href="https://github.com/golang/go/releases/tag/v1.23.0"/>
  </entry>
  <entry>
    <id>tag:github.com,2008:Repository/12345:v1.22.5</id>
    <title>Go 1.22.5</title>
    <published>2024-07-01T18:00:00Z</published>
    <updated>2024-07-01T18:00:00Z</updated>
    <author><name>golang</name></author>
    <link href="https://github.com/golang/go/releases/tag/v1.22.5"/>
  </entry>
</feed>`

func TestParseAtomReleases(t *testing.T) {
	releases, err := ParseAtomReleases(atomReleasesFixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 2 {
		t.Fatalf("want 2 releases, got %d", len(releases))
	}
	r := releases[0]
	if r.Tag != "v1.23.0" {
		t.Errorf("tag: want v1.23.0, got %q", r.Tag)
	}
	if r.Name != "Go 1.23.0" {
		t.Errorf("name: got %q", r.Name)
	}
	if r.Author != "golang" {
		t.Errorf("author: got %q", r.Author)
	}
	if r.Published != "2024-08-13T18:00:00Z" {
		t.Errorf("published: got %q", r.Published)
	}
}

const atomTagsFixture = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <title>v1.23.0</title>
    <updated>2024-08-13T18:00:00Z</updated>
    <link href="https://github.com/golang/go/releases/tag/v1.23.0"/>
  </entry>
  <entry>
    <title>v1.22.5</title>
    <updated>2024-07-01T18:00:00Z</updated>
    <link href="https://github.com/golang/go/releases/tag/v1.22.5"/>
  </entry>
</feed>`

func TestParseAtomTags(t *testing.T) {
	tags, err := ParseAtomTags(atomTagsFixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Fatalf("want 2 tags, got %d", len(tags))
	}
	if tags[0].Name != "v1.23.0" {
		t.Errorf("name: want v1.23.0, got %q", tags[0].Name)
	}
	if tags[0].Updated != "2024-08-13T18:00:00Z" {
		t.Errorf("updated: got %q", tags[0].Updated)
	}
	if tags[1].Name != "v1.22.5" {
		t.Errorf("name[1]: got %q", tags[1].Name)
	}
}

const issuesFixture = `<!DOCTYPE html>
<html>
<body>
<div id="issue_42" class="js-issue-row">
  <a class="Link--primary v-align-middle no-underline h4 js-navigation-open markdown-title"
     href="/golang/go/issues/42">Fix crash on empty input</a>
  <relative-time datetime="2024-01-15T10:00:00Z">Jan 15</relative-time>
  opened by <a class="Link--muted" href="/janedoe">janedoe</a>
  <span class="IssueLabel" title="bug">bug</span>
</div>
<div id="issue_99" class="js-issue-row">
  <a class="Link--primary v-align-middle no-underline h4 js-navigation-open markdown-title"
     href="/golang/go/issues/99">Add support for generics</a>
  <relative-time datetime="2024-01-10T08:00:00Z">Jan 10</relative-time>
  opened by <a class="Link--muted" href="/johnsmith">johnsmith</a>
  <span class="IssueLabel" title="enhancement">enhancement</span>
  5 comments
</div>
</body>
</html>`

func TestParseIssues(t *testing.T) {
	issues := ParseIssues(issuesFixture, "golang", "go", "open")
	if len(issues) < 1 {
		t.Fatalf("want at least 1 issue, got %d", len(issues))
	}
	iss := issues[0]
	if iss.Number != 42 {
		t.Errorf("number: want 42, got %d", iss.Number)
	}
	if !strings.Contains(iss.Title, "crash") && !strings.Contains(iss.Title, "empty") {
		t.Errorf("title should mention crash or empty, got %q", iss.Title)
	}
}

const searchFixture = `<!DOCTYPE html>
<html>
<body>
<div class="search-title">
  <a class="v-align-middle" href="/golang/go">golang/go</a>
</div>
<p class="mb-1">The Go programming language</p>
<span class="search-match">Go</span>
<span>112,000 stars</span>
<relative-time datetime="2024-01-15T10:00:00Z">Jan 15</relative-time>
<div class="search-title">
  <a class="v-align-middle" href="/rust-lang/rust">rust-lang/rust</a>
</div>
<p class="mb-1">Empowering everyone to build reliable and efficient software.</p>
<span class="search-match">Rust</span>
<span>90,000 stars</span>
<relative-time datetime="2024-01-14T10:00:00Z">Jan 14</relative-time>
</body>
</html>`

func TestParseSearch(t *testing.T) {
	results := ParseSearch(searchFixture)
	if len(results) < 1 {
		t.Fatalf("want at least 1 result, got %d", len(results))
	}
	r := results[0]
	if r.FullName != "golang/go" {
		t.Errorf("full_name: want golang/go, got %q", r.FullName)
	}
	if r.Stars != 112000 {
		t.Errorf("stars: want 112000, got %d", r.Stars)
	}
}

const followersFixture = `<!DOCTYPE html>
<html>
<body>
<a data-hovercard-type="user" href="/janedoe">janedoe</a>
<span class="Link--secondary">Jane Doe</span>
<a data-hovercard-type="user" href="/johnsmith">johnsmith</a>
<span class="Link--secondary">John Smith</span>
</body>
</html>`

func TestParseFollowers(t *testing.T) {
	users := ParseFollowers(followersFixture)
	if len(users) < 2 {
		t.Fatalf("want at least 2 followers, got %d", len(users))
	}
	if users[0].Login != "janedoe" {
		t.Errorf("login: want janedoe, got %q", users[0].Login)
	}
	if users[1].Login != "johnsmith" {
		t.Errorf("login[1]: want johnsmith, got %q", users[1].Login)
	}
}

const starsFixture = `<!DOCTYPE html>
<html>
<body>
<div class="col-12">
  <h3><a href="/golang/go" class="Link--primary">golang/go</a></h3>
  <p class="col-9">The Go programming language</p>
  <span itemprop="programmingLanguage">Go</span>
  <a href="/golang/go/stargazers">123,456</a>
</div>
<div class="col-12">
  <h3><a href="/rust-lang/rust" class="Link--primary">rust-lang/rust</a></h3>
  <p class="col-9">Empowering everyone to build reliable and efficient software.</p>
  <span itemprop="programmingLanguage">Rust</span>
  <a href="/rust-lang/rust/stargazers">90,000</a>
</div>
</body>
</html>`

func TestParseStars(t *testing.T) {
	repos := ParseStars(starsFixture)
	if len(repos) < 2 {
		t.Fatalf("want at least 2 starred repos, got %d", len(repos))
	}
	r := repos[0]
	if r.FullName != "golang/go" {
		t.Errorf("full_name: want golang/go, got %q", r.FullName)
	}
	if r.Language != "Go" {
		t.Errorf("language: want Go, got %q", r.Language)
	}
}

// ── cleanInt tests ───────────────────────────────────────────────────────────

func TestCleanInt(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"12,345", 12345},
		{"1,234,567", 1234567},
		{"3.2k", 3200},
		{"5k", 5000},
		{"0", 0},
		{"", 0},
		{"abc", 0},
		{"100", 100},
	}
	for _, tc := range cases {
		got := cleanInt(tc.in)
		if got != tc.want {
			t.Errorf("cleanInt(%q): want %d, got %d", tc.in, tc.want, got)
		}
	}
}

// ── HTTP integration tests ───────────────────────────────────────────────────

func TestClientTrending(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/trending") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(trendingFixture))
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0 // no pacing in tests
	c := NewClient(cfg)

	repos, err := c.Trending(context.Background(), "", "daily")
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("want 2 repos, got %d", len(repos))
	}
	if repos[0].FullName != "golang/go" {
		t.Errorf("full_name: got %q", repos[0].FullName)
	}
}

func TestClientRetry429(t *testing.T) {
	attempt := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(trendingFixture))
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	cfg.Timeout = 5 * time.Second
	c := NewClient(cfg)

	repos, err := c.Trending(context.Background(), "", "daily")
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Errorf("want 2 repos after retry, got %d", len(repos))
	}
	if attempt < 2 {
		t.Errorf("want at least 2 attempts, got %d", attempt)
	}
}

func TestClientRateLimit(t *testing.T) {
	times := make([]time.Time, 0, 3)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		times = append(times, time.Now())
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(trendingFixture))
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 100 * time.Millisecond
	c := NewClient(cfg)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, err := c.Trending(ctx, "", "daily")
		if err != nil {
			t.Fatal(err)
		}
	}

	if len(times) < 3 {
		t.Fatalf("want 3 requests, got %d", len(times))
	}
	for i := 1; i < len(times); i++ {
		gap := times[i].Sub(times[i-1])
		if gap < 90*time.Millisecond {
			t.Errorf("gap between request %d and %d: %v < 90ms (rate not enforced)", i-1, i, gap)
		}
	}
}

func TestClientAtomCommits(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ".atom") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(atomCommitsFixture))
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	c := NewClient(cfg)

	commits, err := c.Commits(context.Background(), "foo", "bar", "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("want 2 commits, got %d", len(commits))
	}
	if commits[0].SHA != "abc1234" {
		t.Errorf("sha: want abc1234, got %q", commits[0].SHA)
	}
}

func TestClientReadme(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "README.md") {
			_, _ = w.Write([]byte("# Hello\n\nThis is the README."))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.RawBaseURL = ts.URL
	cfg.Rate = 0
	c := NewClient(cfg)

	fc, err := c.Readme(context.Background(), "owner", "repo", "main")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fc.Content, "Hello") {
		t.Errorf("content should contain Hello, got %q", fc.Content)
	}
	if fc.Path != "README.md" {
		t.Errorf("path: want README.md, got %q", fc.Path)
	}
}
