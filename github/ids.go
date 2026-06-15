package github

import (
	"fmt"
	"path"
	"strings"
)

// ParseRepoSlug splits an "owner/repo" string into its two parts.
// Returns an error if the string is empty, has no slash, or has more than
// one slash (e.g. "owner/repo/extra").
func ParseRepoSlug(slug string) (owner, repo string, err error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "", "", fmt.Errorf("repo slug is empty")
	}
	parts := strings.SplitN(slug, "/", 3)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo %q: must be owner/repo", slug)
	}
	return parts[0], parts[1], nil
}

// repoURL returns the canonical GitHub HTML URL for a repo.
func repoURL(owner, repo string) string {
	return "https://github.com/" + owner + "/" + repo
}

// userURL returns the canonical GitHub HTML URL for a user.
func userURL(username string) string {
	return "https://github.com/" + username
}

// lastPathSegment returns the last non-empty segment of a URL path.
func lastPathSegment(u string) string {
	return path.Base(u)
}
