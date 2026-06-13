package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	return NewClient(cfg), srv
}

func TestGetSendsHeaders(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
			t.Errorf("Accept header = %q, want application/vnd.github.v3+json", r.Header.Get("Accept"))
		}
		_, _ = w.Write([]byte(`"hello"`))
	})

	body, err := c.get(context.Background(), c.baseURL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `"hello"` {
		t.Errorf("body = %q", body)
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`"recovered"`))
	})
	c.retries = 5

	start := time.Now()
	body, err := c.get(context.Background(), c.baseURL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `"recovered"` {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestSearchRepos(t *testing.T) {
	desc := "A great project"
	lang := "Go"
	license := "MIT"
	resp := searchReposResp{
		TotalCount: 1,
		Items: []wireRepo{
			{
				ID:          1,
				FullName:    "owner/repo",
				Description: &desc,
				HTMLURL:     "https://github.com/owner/repo",
				Stars:       5000,
				Forks:       1200,
				Language:    &lang,
				License: &struct {
					SPDXID string `json:"spdx_id"`
				}{SPDXID: license},
				PushedAt: "2024-06-01T12:00:00Z",
			},
		},
	}

	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/search/repositories") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	repos, err := c.SearchRepos(context.Background(), SearchRepoOptions{
		Query: "great project",
		Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("got %d repos, want 1", len(repos))
	}
	r := repos[0]
	if r.FullName != "owner/repo" {
		t.Errorf("FullName = %q", r.FullName)
	}
	if r.Stars != 5000 {
		t.Errorf("Stars = %d, want 5000", r.Stars)
	}
	if r.License != "MIT" {
		t.Errorf("License = %q, want MIT", r.License)
	}
	if r.URL != "https://github.com/owner/repo" {
		t.Errorf("URL = %q", r.URL)
	}
	if r.Rank != 1 {
		t.Errorf("Rank = %d, want 1", r.Rank)
	}
}

func TestGetRepo(t *testing.T) {
	desc := "Linux kernel source tree"
	lang := "C"
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/torvalds/linux" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(wireRepo{
			ID:          1234,
			FullName:    "torvalds/linux",
			Description: &desc,
			HTMLURL:     "https://github.com/torvalds/linux",
			Stars:       220000,
			Forks:       64000,
			Language:    &lang,
			PushedAt:    "2024-06-01T00:00:00Z",
		})
	})

	repo, err := c.GetRepo(context.Background(), "torvalds", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if repo.FullName != "torvalds/linux" {
		t.Errorf("FullName = %q", repo.FullName)
	}
	if repo.Stars != 220000 {
		t.Errorf("Stars = %d, want 220000", repo.Stars)
	}
	if repo.Language != "C" {
		t.Errorf("Language = %q, want C", repo.Language)
	}
	if repo.License != "" {
		t.Errorf("License = %q, want empty", repo.License)
	}
}

func TestReleases(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/golang/go/releases" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]wireRelease{
			{
				TagName:    "go1.22.0",
				Name:       "Go 1.22",
				Prerelease: false,
				Draft:      false,
				CreatedAt:  "2024-02-06T00:00:00Z",
				HTMLURL:    "https://github.com/golang/go/releases/tag/go1.22.0",
			},
			{
				TagName:    "go1.22rc1",
				Name:       "Go 1.22 RC1",
				Prerelease: true,
				Draft:      false,
				CreatedAt:  "2024-01-23T00:00:00Z",
				HTMLURL:    "https://github.com/golang/go/releases/tag/go1.22rc1",
			},
		})
	})

	releases, err := c.Releases(context.Background(), "golang", "go", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 2 {
		t.Fatalf("got %d releases, want 2", len(releases))
	}
	if releases[0].TagName != "go1.22.0" {
		t.Errorf("TagName = %q", releases[0].TagName)
	}
	if releases[0].Rank != 1 {
		t.Errorf("Rank = %d, want 1", releases[0].Rank)
	}
	if releases[1].Prerelease != true {
		t.Errorf("Prerelease = %v, want true", releases[1].Prerelease)
	}
	if releases[0].URL != "https://github.com/golang/go/releases/tag/go1.22.0" {
		t.Errorf("URL = %q", releases[0].URL)
	}
}

func TestGetUser(t *testing.T) {
	name := "Linus Torvalds"
	company := "Linux Foundation"
	location := "Portland, OR"
	bio := "Creator of Linux"
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/torvalds" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(wireUser{
			Login:       "torvalds",
			Name:        &name,
			Company:     &company,
			Location:    &location,
			Bio:         &bio,
			PublicRepos: 10,
			Followers:   200000,
			HTMLURL:     "https://github.com/torvalds",
		})
	})

	user, err := c.GetUser(context.Background(), "torvalds")
	if err != nil {
		t.Fatal(err)
	}
	if user.Login != "torvalds" {
		t.Errorf("Login = %q", user.Login)
	}
	if user.Name != "Linus Torvalds" {
		t.Errorf("Name = %q", user.Name)
	}
	if user.Followers != 200000 {
		t.Errorf("Followers = %d, want 200000", user.Followers)
	}
	if user.URL != "https://github.com/torvalds" {
		t.Errorf("URL = %q, want https://github.com/torvalds", user.URL)
	}
}

func TestGetUserNullableName(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(wireUser{
			Login:       "nobody",
			Name:        nil,
			PublicRepos: 0,
			Followers:   0,
			HTMLURL:     "https://github.com/nobody",
		})
	})

	user, err := c.GetUser(context.Background(), "nobody")
	if err != nil {
		t.Fatal(err)
	}
	if user.Name != "" {
		t.Errorf("Name = %q, want empty for nil", user.Name)
	}
	if user.URL != "https://github.com/nobody" {
		t.Errorf("URL = %q", user.URL)
	}
}

func TestTrendingQueryContainsCutoff(t *testing.T) {
	var gotQuery string
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		sort := r.URL.Query().Get("sort")
		if sort != "stars" {
			t.Errorf("sort = %q, want stars", sort)
		}
		_ = json.NewEncoder(w).Encode(searchReposResp{})
	})

	_, err := c.Trending(context.Background(), TrendingOptions{Days: 7, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "created:>") {
		t.Errorf("query %q missing created:>", gotQuery)
	}
	if !strings.Contains(gotQuery, "stars:>10") {
		t.Errorf("query %q missing stars:>10", gotQuery)
	}
}

func TestRateLimitError(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	})

	_, err := c.get(context.Background(), c.baseURL)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != ErrRateLimit {
		t.Errorf("err = %v, want ErrRateLimit", err)
	}
}
