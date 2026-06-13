// Package github is the library behind the ghb command: the HTTP client,
// request shaping, and the typed data models for GitHub.
//
// The GitHub REST API v3 at https://api.github.com is open for public data
// with no authentication required. Unauthenticated access is rate-limited to
// 60 requests per hour per IP address. The client paces to 1 request per second
// to stay safely within that budget.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// DefaultUserAgent identifies the client to the GitHub API.
const DefaultUserAgent = "ghb/0.1.0 (+https://github.com/tamnd/github-cli)"

// ErrNotFound is returned when the API responds with HTTP 404.
var ErrNotFound = errors.New("not found")

// ErrRateLimit is returned when the unauthenticated rate limit (60 req/hr) is exhausted.
var ErrRateLimit = errors.New("GitHub rate limit reached (60 req/hr for unauthenticated); retry after a minute")

// Config holds constructor parameters for Client.
type Config struct {
	BaseURL   string        // default: "https://api.github.com"
	UserAgent string        // default: DefaultUserAgent
	Rate      time.Duration // default: 1s
	Retries   int           // default: 2
	Timeout   time.Duration // default: 30s
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://api.github.com",
		UserAgent: DefaultUserAgent,
		Rate:      1 * time.Second,
		Retries:   2,
		Timeout:   30 * time.Second,
	}
}

// Client talks to the GitHub REST API v3.
type Client struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
	rate       time.Duration
	retries    int
	mu         sync.Mutex
	last       time.Time
}

// NewClient returns a Client with the given config.
func NewClient(cfg Config) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: cfg.Timeout},
		baseURL:    cfg.BaseURL,
		userAgent:  cfg.UserAgent,
		rate:       cfg.Rate,
		retries:    cfg.Retries,
	}
}

// get fetches a URL with pacing and retries.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden {
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return nil, false, ErrRateLimit
		}
		return nil, false, fmt.Errorf("http 403")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, false, ErrNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rate <= 0 {
		return
	}
	if wait := c.rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

func (c *Client) getJSON(ctx context.Context, rawURL string, v any) error {
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode %s: %w", rawURL, err)
	}
	return nil
}

// ─── API methods ──────────────────────────────────────────────────────────────

// SearchRepoOptions controls repository search.
type SearchRepoOptions struct {
	Query    string // raw keyword query
	Language string // language filter (empty = all)
	Sort     string // stars|forks|updated|help-wanted-issues
	Limit    int    // max records to return
}

// SearchRepos searches repositories.
func (c *Client) SearchRepos(ctx context.Context, opts SearchRepoOptions) ([]Repo, error) {
	q := opts.Query
	if opts.Language != "" {
		q += " language:" + opts.Language
	}
	sort := opts.Sort
	if sort == "" {
		sort = "stars"
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	params := url.Values{}
	params.Set("q", q)
	params.Set("sort", sort)
	params.Set("order", "desc")
	params.Set("per_page", strconv.Itoa(perPage))
	params.Set("page", "1")

	rawURL := c.baseURL + "/search/repositories?" + params.Encode()
	var resp searchReposResp
	if err := c.getJSON(ctx, rawURL, &resp); err != nil {
		return nil, err
	}

	out := make([]Repo, 0, len(resp.Items))
	for i, w := range resp.Items {
		if i >= limit {
			break
		}
		out = append(out, wireRepoToRepo(w, i+1))
	}
	return out, nil
}

// GetRepo fetches a single repository.
func (c *Client) GetRepo(ctx context.Context, owner, repo string) (Repo, error) {
	rawURL := fmt.Sprintf("%s/repos/%s/%s", c.baseURL,
		url.PathEscape(owner), url.PathEscape(repo))
	var w wireRepo
	if err := c.getJSON(ctx, rawURL, &w); err != nil {
		return Repo{}, err
	}
	return wireRepoToRepo(w, 1), nil
}

// TrendingOptions controls the trending proxy query.
type TrendingOptions struct {
	Language string // language filter (empty = all)
	Days     int    // 7|30|365
	Limit    int    // max records
}

// Trending returns repos created after a cutoff date sorted by stars.
func (c *Client) Trending(ctx context.Context, opts TrendingOptions) ([]Repo, error) {
	days := opts.Days
	if days <= 0 {
		days = 7
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	q := "stars:>10 created:>" + cutoff
	if opts.Language != "" {
		q += " language:" + opts.Language
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 25
	}
	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	params := url.Values{}
	params.Set("q", q)
	params.Set("sort", "stars")
	params.Set("order", "desc")
	params.Set("per_page", strconv.Itoa(perPage))
	params.Set("page", "1")

	rawURL := c.baseURL + "/search/repositories?" + params.Encode()
	var resp searchReposResp
	if err := c.getJSON(ctx, rawURL, &resp); err != nil {
		return nil, err
	}

	out := make([]Repo, 0, len(resp.Items))
	for i, w := range resp.Items {
		if i >= limit {
			break
		}
		out = append(out, wireRepoToRepo(w, i+1))
	}
	return out, nil
}

// GetUser fetches a single user profile.
func (c *Client) GetUser(ctx context.Context, username string) (User, error) {
	rawURL := fmt.Sprintf("%s/users/%s", c.baseURL, url.PathEscape(username))
	var w wireUser
	if err := c.getJSON(ctx, rawURL, &w); err != nil {
		return User{}, err
	}
	return wireUserToUser(w), nil
}

// UserRepos returns a user's public repos sorted by most recently pushed.
func (c *Client) UserRepos(ctx context.Context, username string, limit int) ([]Repo, error) {
	if limit <= 0 {
		limit = 10
	}
	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	params := url.Values{}
	params.Set("sort", "pushed")
	params.Set("per_page", strconv.Itoa(perPage))

	rawURL := fmt.Sprintf("%s/users/%s/repos?%s",
		c.baseURL, url.PathEscape(username), params.Encode())

	var items []wireRepo
	if err := c.getJSON(ctx, rawURL, &items); err != nil {
		return nil, err
	}

	out := make([]Repo, 0, len(items))
	for i, w := range items {
		if i >= limit {
			break
		}
		out = append(out, wireRepoToRepo(w, i+1))
	}
	return out, nil
}

// Releases returns releases for a repo, most recent first.
func (c *Client) Releases(ctx context.Context, owner, repo string, limit int) ([]Release, error) {
	if limit <= 0 {
		limit = 20
	}
	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	params := url.Values{}
	params.Set("per_page", strconv.Itoa(perPage))

	rawURL := fmt.Sprintf("%s/repos/%s/%s/releases?%s",
		c.baseURL, url.PathEscape(owner), url.PathEscape(repo), params.Encode())

	var items []wireRelease
	if err := c.getJSON(ctx, rawURL, &items); err != nil {
		return nil, err
	}

	out := make([]Release, 0, len(items))
	for i, w := range items {
		if i >= limit {
			break
		}
		out = append(out, wireReleaseToRelease(w, i+1))
	}
	return out, nil
}
