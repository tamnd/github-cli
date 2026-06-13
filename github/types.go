package github

import "fmt"

// Repo is the record emitted for repository commands.
type Repo struct {
	Rank        int    `json:"rank"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Language    string `json:"language"`
	Stars       int    `json:"stars"`
	Forks       int    `json:"forks"`
	License     string `json:"license"`
	PushedAt    string `json:"pushed_at"`
	URL         string `json:"url"`
}

// User is the record emitted for user commands.
type User struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	Company   string `json:"company"`
	Location  string `json:"location"`
	Followers int    `json:"followers"`
	Repos     int    `json:"repos"`
	Bio       string `json:"bio"`
	URL       string `json:"url"`
}

// Release is the record emitted for the releases command.
type Release struct {
	Rank       int    `json:"rank"`
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Prerelease bool   `json:"prerelease"`
	CreatedAt  string `json:"created_at"`
	URL        string `json:"url"`
}

// ─── wire types from GitHub REST API ─────────────────────────────────────────

type wireRepo struct {
	ID          int     `json:"id"`
	FullName    string  `json:"full_name"`
	Description *string `json:"description"`
	HTMLURL     string  `json:"html_url"`
	Stars       int     `json:"stargazers_count"`
	Forks       int     `json:"forks_count"`
	Language    *string `json:"language"`
	License     *struct {
		SPDXID string `json:"spdx_id"`
	} `json:"license"`
	PushedAt string `json:"pushed_at"`
}

type wireUser struct {
	Login       string  `json:"login"`
	Name        *string `json:"name"`
	Company     *string `json:"company"`
	Location    *string `json:"location"`
	Bio         *string `json:"bio"`
	PublicRepos int     `json:"public_repos"`
	Followers   int     `json:"followers"`
	HTMLURL     string  `json:"html_url"`
}

type wireRelease struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
	CreatedAt  string `json:"created_at"`
	HTMLURL    string `json:"html_url"`
}

type searchReposResp struct {
	TotalCount int        `json:"total_count"`
	Items      []wireRepo `json:"items"`
}

// ─── converters ──────────────────────────────────────────────────────────────

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func wireRepoToRepo(w wireRepo, rank int) Repo {
	lic := ""
	if w.License != nil {
		lic = w.License.SPDXID
	}
	return Repo{
		Rank:        rank,
		FullName:    w.FullName,
		Description: deref(w.Description),
		Language:    deref(w.Language),
		Stars:       w.Stars,
		Forks:       w.Forks,
		License:     lic,
		PushedAt:    w.PushedAt,
		URL:         w.HTMLURL,
	}
}

func wireUserToUser(w wireUser) User {
	return User{
		Login:     w.Login,
		Name:      deref(w.Name),
		Company:   deref(w.Company),
		Location:  deref(w.Location),
		Followers: w.Followers,
		Repos:     w.PublicRepos,
		Bio:       deref(w.Bio),
		URL:       fmt.Sprintf("https://github.com/%s", w.Login),
	}
}

func wireReleaseToRelease(w wireRelease, rank int) Release {
	return Release{
		Rank:       rank,
		TagName:    w.TagName,
		Name:       w.Name,
		Prerelease: w.Prerelease,
		CreatedAt:  w.CreatedAt,
		URL:        w.HTMLURL,
	}
}
