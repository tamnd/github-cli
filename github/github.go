// Package github is the scraper library behind the github CLI.
// It reads public GitHub data from HTML pages, Atom feeds, and
// raw.githubusercontent.com. No API key or authentication is required.
//
// github is an independent tool and is not affiliated with GitHub or Microsoft.
package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	defaultUA      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	defaultRawUA   = "go-http-client/1.1"
	defaultRate    = 500 * time.Millisecond
	defaultTimeout = 30 * time.Second
	defaultRetries = 3
)

// ErrRateLimit is returned when GitHub returns HTTP 429 on search.
var ErrRateLimit = fmt.Errorf("rate limited by GitHub search; try again later")

// Config holds constructor parameters for Client.
type Config struct {
	BaseURL    string        // "https://github.com"
	RawBaseURL string        // "https://raw.githubusercontent.com"
	UserAgent  string
	Rate       time.Duration
	Retries    int
	Timeout    time.Duration
	SearchWait time.Duration // extra delay before search pages; default 3s
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:    "https://github.com",
		RawBaseURL: "https://raw.githubusercontent.com",
		UserAgent:  defaultUA,
		Rate:       defaultRate,
		Retries:    defaultRetries,
		Timeout:    defaultTimeout,
		SearchWait: 3 * time.Second,
	}
}

// Client scrapes public GitHub data.
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	last time.Time
}

// NewClient creates a Client with the given Config.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// fetchHTML fetches a GitHub HTML page with browser-like headers.
func (c *Client) fetchHTML(ctx context.Context, u string) (string, error) {
	headers := map[string]string{
		"User-Agent":      c.cfg.UserAgent,
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "en-US,en;q=0.9",
	}
	b, status, err := c.do(ctx, u, headers)
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", fmt.Errorf("not found: %s", u)
	}
	if status == http.StatusTooManyRequests {
		return "", ErrRateLimit
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("http %d: %s", status, u)
	}
	return string(b), nil
}

// fetchAtom fetches an Atom feed.
func (c *Client) fetchAtom(ctx context.Context, u string) (string, error) {
	headers := map[string]string{
		"User-Agent": c.cfg.UserAgent,
		"Accept":     "application/atom+xml, application/xml;q=0.9, */*;q=0.8",
	}
	b, status, err := c.do(ctx, u, headers)
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", fmt.Errorf("not found: %s", u)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("http %d: %s", status, u)
	}
	return string(b), nil
}

// fetchRaw fetches raw content from raw.githubusercontent.com.
func (c *Client) fetchRaw(ctx context.Context, u string) (string, error) {
	headers := map[string]string{
		"User-Agent": defaultRawUA,
	}
	b, status, err := c.do(ctx, u, headers)
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", fmt.Errorf("not found: %s", u)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("http %d: %s", status, u)
	}
	return string(b), nil
}

// do is the low-level sender: paces requests, then retries on 429/5xx.
// Returns (body, statusCode, error).
func (c *Client) do(ctx context.Context, u string, headers map[string]string) ([]byte, int, error) {
	var lastErr error
	var lastStatus int
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(wait):
			}
		}

		c.pace(ctx)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, 0, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			lastStatus = resp.StatusCode
			continue
		}

		// handle Retry-After on 429
		if resp.StatusCode == http.StatusTooManyRequests {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, parseErr := strconv.Atoi(ra); parseErr == nil && secs > 0 {
					select {
					case <-ctx.Done():
						return nil, resp.StatusCode, ctx.Err()
					case <-time.After(time.Duration(secs) * time.Second):
					}
				}
			}
			lastStatus = resp.StatusCode
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
			continue
		}

		// retry on 5xx
		if resp.StatusCode >= 500 {
			lastStatus = resp.StatusCode
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
			continue
		}

		return body, resp.StatusCode, nil
	}
	if lastErr != nil {
		return nil, lastStatus, fmt.Errorf("fetch %s: %w (after %d retries)", u, lastErr, c.cfg.Retries)
	}
	return nil, lastStatus, fmt.Errorf("fetch %s: http %d (after %d retries)", u, lastStatus, c.cfg.Retries)
}

// pace enforces the minimum gap between outbound requests.
func (c *Client) pace(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
	}
	c.last = time.Now()
}
