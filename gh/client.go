package gh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tamnd/any-cli/kit/errs"
)

// Client reads github.com. It is safe for concurrent use: the pacer and the
// cache are synchronised, so a crawl running many workers still produces one
// polite stream of requests rather than one stream per worker.
//
// There is no Token field. That is not an oversight, it is the design: see the
// package comment, and see TestNoAuth, which fails the build if a credential
// ever appears in this package.
type Client struct {
	HTTP      *http.Client
	UserAgent string

	Rate    time.Duration // the minimum gap between requests, shared by every worker
	Retries int
	Workers int

	CacheDir string
	NoCache  bool
	CacheTTL time.Duration

	// Deep makes a record read follow the extra requests that fill in fields
	// the primary surface omits. Off by default because it multiplies requests.
	Deep bool

	// Verbose writes one line per request to stderr: the URL, the surface, the
	// status, and the byte count. It is the fastest way to see what a command
	// actually costs.
	Verbose bool

	mu   sync.Mutex
	last time.Time

	sem chan struct{}
}

// NewClient returns a client configured from cfg, falling back to Defaults for
// anything unset.
func NewClient(cfg Config) *Client {
	if cfg.Rate <= 0 {
		cfg.Rate = Defaults.Rate
	}
	if cfg.Retries <= 0 {
		cfg.Retries = Defaults.Retries
	}
	if cfg.Workers <= 0 {
		cfg.Workers = Defaults.Workers
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = Defaults.Timeout
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 15 * time.Minute
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = DefaultCacheDir()
	}
	c := &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: cfg.UserAgent,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
		Workers:   cfg.Workers,
		CacheDir:  cfg.CacheDir,
		NoCache:   cfg.NoCache,
		CacheTTL:  cfg.CacheTTL,
		Deep:      cfg.Deep,
		sem:       make(chan struct{}, cfg.Workers),
	}
	if c.UserAgent == "" {
		c.UserAgent = UserAgentBase + "/dev (+https://github.com/tamnd/github-cli)"
	}
	return c
}

// DefaultCacheDir is $XDG_CACHE_HOME/github-cli, or the platform equivalent.
func DefaultCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "github-cli")
}

// Surface names one of the eight ways this tool reads github.com. It decides
// the request headers and how a non-200 is read, which is why it travels with
// every request instead of being guessed from the URL.
type Surface int

const (
	// SurfaceHTML is a plain page fetch. Every route serves one.
	SurfaceHTML Surface = iota
	// SurfaceRouteJSON asks a React route for its props with Accept: json.
	SurfaceRouteJSON
	// SurfaceXHR sets X-Requested-With, which unlocks the fragments the front
	// end fetches for itself: refs, contributor statistics, hovercards.
	SurfaceXHR
	// SurfaceSearch is /search?q=&type=, which answers JSON for the asking.
	SurfaceSearch
	// SurfaceFeed is an .atom endpoint.
	SurfaceFeed
	// SurfaceRaw is raw.githubusercontent.com and codeload: bytes, no page.
	SurfaceRaw
	// SurfaceGit is the git smart protocol at /{owner}/{repo}.git/info/refs.
	SurfaceGit
)

func (s Surface) String() string {
	return [...]string{"html", "route-json", "xhr", "search", "feed", "raw", "git"}[s]
}

// Response is one completed exchange. FinalURL differs from URL when GitHub
// redirected, which is how a renamed repository is detected without a second
// request.
type Response struct {
	Body     []byte
	Status   int
	Header   http.Header
	URL      string
	FinalURL string
	Surface  Surface
}

// Get fetches a URL on a surface. Every request in the package goes through
// here, so pacing, caching, retry, and error classification each have exactly
// one home.
func (c *Client) Get(ctx context.Context, rawURL string, s Surface) (*Response, error) {
	if hit, ok := c.cacheGet(rawURL, s); ok {
		c.trace(hit, true)
		return hit, nil
	}
	var last error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt, last)):
			}
		}
		resp, retry, err := c.do(ctx, rawURL, s)
		if err == nil {
			c.cachePut(rawURL, s, resp)
			c.trace(resp, false)
			return resp, nil
		}
		last = err
		if !retry {
			return nil, err
		}
	}
	return nil, last
}

// GetJSON fetches and decodes in one step.
func (c *Client) GetJSON(ctx context.Context, rawURL string, s Surface, v any) (*Response, error) {
	resp, err := c.Get(ctx, rawURL, s)
	if err != nil {
		return nil, err
	}
	if v != nil {
		if err := json.Unmarshal(resp.Body, v); err != nil {
			return resp, errs.New(errs.KindNetwork, "%s: %v", shortURL(rawURL), err)
		}
	}
	return resp, nil
}

// GetHTML fetches a page.
func (c *Client) GetHTML(ctx context.Context, rawURL string) (*Response, error) {
	return c.Get(ctx, rawURL, SurfaceHTML)
}

// Stream opens a body without buffering, retrying, or caching. Release assets
// and repository archives go through here: a tarball does not belong in memory
// and does not belong in the cache. The caller closes the reader.
func (c *Client) Stream(ctx context.Context, rawURL string) (io.ReadCloser, http.Header, error) {
	if err := c.acquire(ctx); err != nil {
		return nil, nil, err
	}
	defer c.release()
	c.pace(ctx)
	req, err := c.newRequest(ctx, rawURL, SurfaceRaw)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, wrapNetwork(rawURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		_ = resp.Body.Close()
		return nil, nil, statusError(rawURL, resp.StatusCode, b)
	}
	return resp.Body, resp.Header, nil
}

func (c *Client) do(ctx context.Context, rawURL string, s Surface) (resp *Response, retry bool, err error) {
	if err := c.acquire(ctx); err != nil {
		return nil, false, err
	}
	defer c.release()
	c.pace(ctx)

	req, err := c.newRequest(ctx, rawURL, s)
	if err != nil {
		return nil, false, err
	}
	hr, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, wrapNetwork(rawURL, err)
	}
	defer func() { _ = hr.Body.Close() }()

	b, err := io.ReadAll(hr.Body)
	if err != nil {
		return nil, true, wrapNetwork(rawURL, err)
	}
	out := &Response{Body: b, Status: hr.StatusCode, Header: hr.Header, URL: rawURL, Surface: s}
	if hr.Request != nil && hr.Request.URL != nil {
		out.FinalURL = hr.Request.URL.String()
	}
	switch {
	case hr.StatusCode >= 200 && hr.StatusCode < 300:
		return out, false, nil
	case hr.StatusCode == http.StatusNotAcceptable, hr.StatusCode == http.StatusGone:
		// Not failures. 406 means "this route does not serve JSON" and 410
		// means "there is no JSON here at all". Both are answers about which
		// surface to use, so they come back as a response for the caller to
		// route on rather than as an error.
		return out, false, nil
	case hr.StatusCode == http.StatusAccepted:
		// A statistic GitHub is still computing. Only the contributor graph
		// does this, and it is the caller's job to poll.
		return out, false, nil
	}
	err = statusError(rawURL, hr.StatusCode, b)
	return nil, retryableStatus(hr.StatusCode), err
}

func (c *Client) newRequest(ctx context.Context, rawURL string, s Surface) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errs.Usage("bad url %q: %v", rawURL, err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	switch s {
	case SurfaceRouteJSON:
		// The one header that turns a React route into its own props.
		req.Header.Set("Accept", "application/json")
	case SurfaceXHR:
		// The front end's own fragment requests carry this, and several routes
		// answer JSON only when they see it.
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
	case SurfaceSearch:
		req.Header.Set("Accept", "application/json")
	case SurfaceFeed:
		req.Header.Set("Accept", "application/atom+xml, application/xml;q=0.9")
	case SurfaceRaw:
		req.Header.Set("Accept", "*/*")
	case SurfaceGit:
		req.Header.Set("Accept", "application/x-git-upload-pack-advertisement")
	default:
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
	}
	return req, nil
}

// acquire and release bound the number of requests in flight. The semaphore is
// on the client rather than on the caller so that a crawl at depth three does
// not get three times the concurrency because it has three levels.
func (c *Client) acquire(ctx context.Context) error {
	select {
	case c.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) release() { <-c.sem }

// pace holds the lock across the sleep on purpose. The point is that N workers
// produce one paced stream, not N paced streams.
func (c *Client) pace(ctx context.Context) {
	if c.Rate <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
	}
	c.last = time.Now()
}

func (c *Client) trace(r *Response, cached bool) {
	if !c.Verbose || r == nil {
		return
	}
	tag := ""
	if cached {
		tag = " (cached)"
	}
	fmt.Fprintf(os.Stderr, "%-10s %3d %7d  %s%s\n", r.Surface, r.Status, len(r.Body), shortURL(r.URL), tag)
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// backoff is exponential with full jitter and a thirty second ceiling.
func backoff(attempt int, _ error) time.Duration {
	d := time.Duration(1<<uint(attempt-1)) * 500 * time.Millisecond
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d/2 + time.Duration(rand.Int64N(int64(d/2)+1))
}

// Poll waits for a statistic GitHub computes in the background. Only the
// contributor graph needs it: the first request kicks off the job and answers
// 202 with an empty body, and the answer arrives some seconds later.
//
// Observed twice in a row several seconds apart on a cold repository, so one
// retry is not enough. Eight attempts over a sixty second budget, and never a
// silent empty list: "no contributors" and "not computed yet" are different
// answers and conflating them would make an empty result look authoritative.
func (c *Client) Poll(ctx context.Context, rawURL string, s Surface) (*Response, error) {
	wait := time.Second
	deadline := time.Now().Add(60 * time.Second)
	for attempt := 1; attempt <= 8; attempt++ {
		resp, err := c.Get(ctx, rawURL, s)
		if err != nil {
			return nil, err
		}
		if resp.Status != http.StatusAccepted && len(resp.Body) > 0 {
			return resp, nil
		}
		if attempt == 1 {
			fmt.Fprintf(os.Stderr, "github: %s is being computed, waiting\n", shortURL(rawURL))
		}
		// A 202 must never be cached: the whole point is that the answer is
		// not ready yet and the next request is what makes it ready.
		c.cacheDrop(rawURL, s)
		if time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		if wait < 30*time.Second {
			wait *= 2
		}
	}
	return nil, errs.Unsupported("%s: github is still computing this statistic, try again shortly", shortURL(rawURL))
}

// --- URL building ---

// Page builds a github.com URL from path segments, escaping each one.
func Page(parts ...string) string {
	esc := make([]string, 0, len(parts))
	for _, p := range parts {
		esc = append(esc, p)
	}
	return BaseURL + "/" + strings.Join(esc, "/")
}

// query appends parameters, skipping empty values so optional filters can be
// passed straight through.
func query(base string, kv ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			v.Add(kv[i], kv[i+1])
		}
	}
	if len(v) == 0 {
		return base
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + v.Encode()
}

// --- the on-disk response cache ---

// The cache key includes the surface, because /golang/go answers with wholly
// different bytes depending on the Accept header and a shared key would serve
// HTML to a JSON decoder.
func (c *Client) cacheKey(rawURL string, s Surface) string {
	sum := sha256.Sum256([]byte(s.String() + " " + rawURL))
	return hex.EncodeToString(sum[:])
}

func (c *Client) cachePath(rawURL string, s Surface) string {
	key := c.cacheKey(rawURL, s)
	return filepath.Join(c.CacheDir, key[:2], key)
}

func (c *Client) cacheGet(rawURL string, s Surface) (*Response, bool) {
	if c.NoCache || c.CacheDir == "" {
		return nil, false
	}
	path := c.cachePath(rawURL, s)
	st, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if ttl := c.ttlFor(rawURL); ttl > 0 && time.Since(st.ModTime()) > ttl {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var ent cacheEntry
	if err := json.Unmarshal(b, &ent); err != nil {
		return nil, false
	}
	return &Response{Body: ent.Body, Status: ent.Status, Header: ent.Header,
		URL: rawURL, FinalURL: ent.FinalURL, Surface: s}, true
}

func (c *Client) cachePut(rawURL string, s Surface, resp *Response) {
	if c.NoCache || c.CacheDir == "" || resp == nil || resp.Status == http.StatusAccepted {
		return
	}
	path := c.cachePath(rawURL, s)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	b, err := json.Marshal(cacheEntry{Body: resp.Body, Status: resp.Status, Header: resp.Header, FinalURL: resp.FinalURL})
	if err != nil {
		return
	}
	// Write through a temp file so a killed process cannot leave a half entry
	// that later decodes as valid.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func (c *Client) cacheDrop(rawURL string, s Surface) {
	if c.CacheDir == "" {
		return
	}
	_ = os.Remove(c.cachePath(rawURL, s))
}

type cacheEntry struct {
	Body     []byte      `json:"body"`
	Status   int         `json:"status"`
	Header   http.Header `json:"header,omitempty"`
	FinalURL string      `json:"finalUrl,omitempty"`
}

// ttlFor gives an immutable document an unlimited life. Anything pinned to a
// full object name cannot change, so re-fetching it is pure waste.
func (c *Client) ttlFor(rawURL string) time.Duration {
	if looksImmutable(rawURL) {
		return 0
	}
	return c.CacheTTL
}

func looksImmutable(rawURL string) bool {
	for _, seg := range strings.Split(rawURL, "/") {
		if len(seg) == 40 && isSHA(seg) {
			return true
		}
	}
	return false
}
