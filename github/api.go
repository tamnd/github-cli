package github

import (
	"context"
	"fmt"
	"time"
)

// Trending returns trending repositories from github.com/trending.
// lang is optional (empty = all languages).
// since is "daily", "weekly", or "monthly" (default "daily").
func (c *Client) Trending(ctx context.Context, lang, since string) ([]TrendingRepo, error) {
	if since == "" {
		since = "daily"
	}
	u := c.cfg.BaseURL + "/trending"
	if lang != "" {
		u += "/" + lang
	}
	if since != "daily" {
		u += "?since=" + since
	}
	body, err := c.fetchHTML(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseTrending(body), nil
}

// GetUser fetches a GitHub user profile.
func (c *Client) GetUser(ctx context.Context, username string) (User, error) {
	u := c.cfg.BaseURL + "/" + username
	body, err := c.fetchHTML(ctx, u)
	if err != nil {
		return User{}, err
	}
	return ParseUser(body, username)
}

// UserRepos lists a user's public repositories (30 per page).
func (c *Client) UserRepos(ctx context.Context, username string, page int) ([]Repo, error) {
	if page <= 0 {
		page = 1
	}
	u := fmt.Sprintf("%s/%s?tab=repositories&page=%d", c.cfg.BaseURL, username, page)
	body, err := c.fetchHTML(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseRepos(body, username), nil
}

// GetRepo fetches a single repository's metadata.
func (c *Client) GetRepo(ctx context.Context, owner, repo string) (Repo, error) {
	u := c.cfg.BaseURL + "/" + owner + "/" + repo
	body, err := c.fetchHTML(ctx, u)
	if err != nil {
		return Repo{}, err
	}
	return ParseRepo(body, owner, repo)
}

// Commits lists commits from the Atom feed.
func (c *Client) Commits(ctx context.Context, owner, repo, branch string) ([]Commit, error) {
	if branch == "" {
		branch = "main"
	}
	u := fmt.Sprintf("%s/%s/%s/commits/%s.atom", c.cfg.BaseURL, owner, repo, branch)
	body, err := c.fetchAtom(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseAtomCommits(body)
}

// Releases lists releases from the Atom feed.
func (c *Client) Releases(ctx context.Context, owner, repo string) ([]Release, error) {
	u := fmt.Sprintf("%s/%s/%s/releases.atom", c.cfg.BaseURL, owner, repo)
	body, err := c.fetchAtom(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseAtomReleases(body)
}

// Tags lists tags from the Atom feed.
func (c *Client) Tags(ctx context.Context, owner, repo string) ([]Tag, error) {
	u := fmt.Sprintf("%s/%s/%s/tags.atom", c.cfg.BaseURL, owner, repo)
	body, err := c.fetchAtom(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseAtomTags(body)
}

// Issues lists issues from the HTML page.
// state is "open", "closed", or "all".
func (c *Client) Issues(ctx context.Context, owner, repo, state string, page int) ([]Issue, error) {
	if state == "" {
		state = "open"
	}
	if page <= 0 {
		page = 1
	}
	u := fmt.Sprintf("%s/%s/%s/issues?state=%s&page=%d", c.cfg.BaseURL, owner, repo, state, page)
	body, err := c.fetchHTML(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseIssues(body, owner, repo, state), nil
}

// Pulls lists pull requests from the HTML page.
func (c *Client) Pulls(ctx context.Context, owner, repo, state string, page int) ([]PullRequest, error) {
	if state == "" {
		state = "open"
	}
	if page <= 0 {
		page = 1
	}
	u := fmt.Sprintf("%s/%s/%s/pulls?state=%s&page=%d", c.cfg.BaseURL, owner, repo, state, page)
	body, err := c.fetchHTML(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParsePulls(body, owner, repo, state), nil
}

// Readme fetches the README of a repository.
// If branch is empty, it tries "main" then "master".
func (c *Client) Readme(ctx context.Context, owner, repo, branch string) (FileContent, error) {
	branches := []string{branch}
	if branch == "" {
		branches = []string{"main", "master"}
	}
	filenames := []string{"README.md", "README.rst", "README"}

	for _, br := range branches {
		for _, fn := range filenames {
			u := fmt.Sprintf("%s/%s/%s/%s/%s", c.cfg.RawBaseURL, owner, repo, br, fn)
			body, err := c.fetchRaw(ctx, u)
			if err != nil {
				continue
			}
			return FileContent{
				Path:    fn,
				Content: body,
				URL:     u,
			}, nil
		}
	}
	return FileContent{}, fmt.Errorf("README not found for %s/%s", owner, repo)
}

// File fetches a file from raw.githubusercontent.com.
func (c *Client) File(ctx context.Context, owner, repo, branch, path string) (FileContent, error) {
	if branch == "" {
		branch = "main"
	}
	u := fmt.Sprintf("%s/%s/%s/%s/%s", c.cfg.RawBaseURL, owner, repo, branch, path)
	body, err := c.fetchRaw(ctx, u)
	if err != nil {
		return FileContent{}, err
	}
	return FileContent{
		Path:    path,
		Content: body,
		URL:     u,
	}, nil
}

// Search searches repositories on github.com/search.
// GitHub may throttle search from datacenter IPs.
func (c *Client) Search(ctx context.Context, query string, page int) ([]SearchRepo, error) {
	if page <= 0 {
		page = 1
	}
	// extra courtesy delay for search to avoid 429
	if page > 1 && c.cfg.SearchWait > 0 {
		wait := c.cfg.SearchWait * time.Duration(page-1)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	u := fmt.Sprintf("%s/search?q=%s&type=repositories&p=%d", c.cfg.BaseURL, query, page)
	body, err := c.fetchHTML(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseSearch(body), nil
}

// Followers lists a user's followers.
func (c *Client) Followers(ctx context.Context, username string, page int) ([]User, error) {
	if page <= 0 {
		page = 1
	}
	u := fmt.Sprintf("%s/%s?tab=followers&page=%d", c.cfg.BaseURL, username, page)
	body, err := c.fetchHTML(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseFollowers(body), nil
}

// Following lists users that a user follows.
func (c *Client) Following(ctx context.Context, username string, page int) ([]User, error) {
	if page <= 0 {
		page = 1
	}
	u := fmt.Sprintf("%s/%s?tab=following&page=%d", c.cfg.BaseURL, username, page)
	body, err := c.fetchHTML(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseFollowing(body), nil
}

// Stars lists repositories starred by a user.
func (c *Client) Stars(ctx context.Context, username string, page int) ([]StarredRepo, error) {
	if page <= 0 {
		page = 1
	}
	u := fmt.Sprintf("%s/%s?tab=stars&page=%d", c.cfg.BaseURL, username, page)
	body, err := c.fetchHTML(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseStars(body), nil
}
