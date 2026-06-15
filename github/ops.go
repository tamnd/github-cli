package github

import (
	"context"

	"github.com/tamnd/any-cli/kit"
)

// RegisterOps installs all 15 GitHub operations onto app.
func RegisterOps(app *kit.App) {

	// trending: top trending repositories
	kit.Handle(app, kit.OpMeta{
		Name:    "trending",
		Group:   "read",
		List:    true,
		Summary: "List trending GitHub repositories",
		Long:    "Fetches the github.com/trending page and returns repository cards.",
	}, func(ctx context.Context, in trendingIn, emit func(TrendingRepo) error) error {
		results, err := in.Client.Trending(ctx, in.Lang, in.Since)
		if err != nil {
			return err
		}
		for _, r := range results {
			if err := emit(r); err != nil {
				return err
			}
		}
		return nil
	})

	// user: GitHub user profile
	kit.Handle(app, kit.OpMeta{
		Name:    "user",
		Group:   "read",
		Single:  true,
		Summary: "Show a GitHub user profile",
		Args:    []kit.Arg{{Name: "username", Help: "GitHub username"}},
	}, func(ctx context.Context, in userIn, emit func(User) error) error {
		u, err := in.Client.GetUser(ctx, in.Username)
		if err != nil {
			return err
		}
		return emit(u)
	})

	// repos: list a user's public repos
	kit.Handle(app, kit.OpMeta{
		Name:    "repos",
		Group:   "read",
		List:    true,
		Summary: "List a user's public repositories",
		Args:    []kit.Arg{{Name: "username", Help: "GitHub username"}},
	}, func(ctx context.Context, in reposIn, emit func(Repo) error) error {
		results, err := in.Client.UserRepos(ctx, in.Username, in.Page)
		if err != nil {
			return err
		}
		for _, r := range results {
			if err := emit(r); err != nil {
				return err
			}
		}
		return nil
	})

	// repo: single repository metadata
	kit.Handle(app, kit.OpMeta{
		Name:    "repo",
		Group:   "read",
		Single:  true,
		Summary: "Show a repository's metadata",
		Args:    []kit.Arg{{Name: "repo", Help: "owner/repo slug"}},
	}, func(ctx context.Context, in repoIn, emit func(Repo) error) error {
		owner, repoName, err := ParseRepoSlug(in.Repo)
		if err != nil {
			return err
		}
		r, err := in.Client.GetRepo(ctx, owner, repoName)
		if err != nil {
			return err
		}
		return emit(r)
	})

	// commits: commits from the Atom feed
	kit.Handle(app, kit.OpMeta{
		Name:    "commits",
		Group:   "read",
		List:    true,
		Summary: "List recent commits (from Atom feed)",
		Args:    []kit.Arg{{Name: "repo", Help: "owner/repo slug"}},
	}, func(ctx context.Context, in commitsIn, emit func(Commit) error) error {
		owner, repoName, err := ParseRepoSlug(in.Repo)
		if err != nil {
			return err
		}
		results, err := in.Client.Commits(ctx, owner, repoName, in.Branch)
		if err != nil {
			return err
		}
		for _, c := range results {
			if err := emit(c); err != nil {
				return err
			}
		}
		return nil
	})

	// releases: releases from the Atom feed
	kit.Handle(app, kit.OpMeta{
		Name:    "releases",
		Group:   "read",
		List:    true,
		Summary: "List releases (from Atom feed)",
		Args:    []kit.Arg{{Name: "repo", Help: "owner/repo slug"}},
	}, func(ctx context.Context, in releasesIn, emit func(Release) error) error {
		owner, repoName, err := ParseRepoSlug(in.Repo)
		if err != nil {
			return err
		}
		results, err := in.Client.Releases(ctx, owner, repoName)
		if err != nil {
			return err
		}
		for _, r := range results {
			if err := emit(r); err != nil {
				return err
			}
		}
		return nil
	})

	// tags: tags from the Atom feed
	kit.Handle(app, kit.OpMeta{
		Name:    "tags",
		Group:   "read",
		List:    true,
		Summary: "List tags (from Atom feed)",
		Args:    []kit.Arg{{Name: "repo", Help: "owner/repo slug"}},
	}, func(ctx context.Context, in tagsIn, emit func(Tag) error) error {
		owner, repoName, err := ParseRepoSlug(in.Repo)
		if err != nil {
			return err
		}
		results, err := in.Client.Tags(ctx, owner, repoName)
		if err != nil {
			return err
		}
		for _, t := range results {
			if err := emit(t); err != nil {
				return err
			}
		}
		return nil
	})

	// issues: open issues list
	kit.Handle(app, kit.OpMeta{
		Name:    "issues",
		Group:   "read",
		List:    true,
		Summary: "List issues for a repository",
		Args:    []kit.Arg{{Name: "repo", Help: "owner/repo slug"}},
	}, func(ctx context.Context, in issuesIn, emit func(Issue) error) error {
		owner, repoName, err := ParseRepoSlug(in.Repo)
		if err != nil {
			return err
		}
		results, err := in.Client.Issues(ctx, owner, repoName, in.State, in.Page)
		if err != nil {
			return err
		}
		for _, iss := range results {
			if err := emit(iss); err != nil {
				return err
			}
		}
		return nil
	})

	// pulls: pull requests list
	kit.Handle(app, kit.OpMeta{
		Name:    "pulls",
		Group:   "read",
		List:    true,
		Summary: "List pull requests for a repository",
		Args:    []kit.Arg{{Name: "repo", Help: "owner/repo slug"}},
	}, func(ctx context.Context, in pullsIn, emit func(PullRequest) error) error {
		owner, repoName, err := ParseRepoSlug(in.Repo)
		if err != nil {
			return err
		}
		results, err := in.Client.Pulls(ctx, owner, repoName, in.State, in.Page)
		if err != nil {
			return err
		}
		for _, pr := range results {
			if err := emit(pr); err != nil {
				return err
			}
		}
		return nil
	})

	// readme: fetch README content
	kit.Handle(app, kit.OpMeta{
		Name:    "readme",
		Group:   "read",
		Single:  true,
		Summary: "Fetch the README of a repository",
		Args:    []kit.Arg{{Name: "repo", Help: "owner/repo slug"}},
	}, func(ctx context.Context, in readmeIn, emit func(FileContent) error) error {
		owner, repoName, err := ParseRepoSlug(in.Repo)
		if err != nil {
			return err
		}
		fc, err := in.Client.Readme(ctx, owner, repoName, in.Branch)
		if err != nil {
			return err
		}
		return emit(fc)
	})

	// file: fetch any file from a repo
	kit.Handle(app, kit.OpMeta{
		Name:    "file",
		Group:   "read",
		Single:  true,
		Summary: "Fetch a file from a repository",
		Args: []kit.Arg{
			{Name: "repo", Help: "owner/repo slug"},
			{Name: "path", Help: "file path in the repository"},
		},
	}, func(ctx context.Context, in fileIn, emit func(FileContent) error) error {
		owner, repoName, err := ParseRepoSlug(in.Repo)
		if err != nil {
			return err
		}
		fc, err := in.Client.File(ctx, owner, repoName, in.Branch, in.Path)
		if err != nil {
			return err
		}
		return emit(fc)
	})

	// search: search repositories
	kit.Handle(app, kit.OpMeta{
		Name:    "search",
		Group:   "read",
		List:    true,
		Summary: "Search GitHub repositories",
		Long:    "Scrapes github.com/search. May be rate-limited from datacenter IPs (exit 5).",
		Args:    []kit.Arg{{Name: "query", Help: "search query"}},
	}, func(ctx context.Context, in searchIn, emit func(SearchRepo) error) error {
		results, err := in.Client.Search(ctx, in.Query, in.Page)
		if err != nil {
			return err
		}
		for _, r := range results {
			if err := emit(r); err != nil {
				return err
			}
		}
		return nil
	})

	// followers: list a user's followers
	kit.Handle(app, kit.OpMeta{
		Name:    "followers",
		Group:   "read",
		List:    true,
		Summary: "List a user's followers",
		Args:    []kit.Arg{{Name: "username", Help: "GitHub username"}},
	}, func(ctx context.Context, in followersIn, emit func(User) error) error {
		results, err := in.Client.Followers(ctx, in.Username, in.Page)
		if err != nil {
			return err
		}
		for _, u := range results {
			if err := emit(u); err != nil {
				return err
			}
		}
		return nil
	})

	// following: list users that a user follows
	kit.Handle(app, kit.OpMeta{
		Name:    "following",
		Group:   "read",
		List:    true,
		Summary: "List users that a user follows",
		Args:    []kit.Arg{{Name: "username", Help: "GitHub username"}},
	}, func(ctx context.Context, in followingIn, emit func(User) error) error {
		results, err := in.Client.Following(ctx, in.Username, in.Page)
		if err != nil {
			return err
		}
		for _, u := range results {
			if err := emit(u); err != nil {
				return err
			}
		}
		return nil
	})

	// stars: list starred repositories for a user
	kit.Handle(app, kit.OpMeta{
		Name:    "stars",
		Group:   "read",
		List:    true,
		Summary: "List repositories starred by a user",
		Args:    []kit.Arg{{Name: "username", Help: "GitHub username"}},
	}, func(ctx context.Context, in starsIn, emit func(StarredRepo) error) error {
		results, err := in.Client.Stars(ctx, in.Username, in.Page)
		if err != nil {
			return err
		}
		for _, r := range results {
			if err := emit(r); err != nil {
				return err
			}
		}
		return nil
	})
}

// ── input structs ──────────────────────────────────────────────────────────

type trendingIn struct {
	Client *Client `kit:"inject"`
	Lang   string  `kit:"flag" help:"language filter (e.g. go, python, c++)"`
	Since  string  `kit:"flag" help:"time window: daily, weekly, monthly" default:"daily"`
}

type userIn struct {
	Client   *Client `kit:"inject"`
	Username string  `kit:"arg" help:"GitHub username"`
}

type reposIn struct {
	Client   *Client `kit:"inject"`
	Username string  `kit:"arg" help:"GitHub username"`
	Page     int     `kit:"flag" help:"page number (30 repos per page)" default:"1"`
}

type repoIn struct {
	Client *Client `kit:"inject"`
	Repo   string  `kit:"arg" help:"owner/repo slug"`
}

type commitsIn struct {
	Client *Client `kit:"inject"`
	Repo   string  `kit:"arg" help:"owner/repo slug"`
	Branch string  `kit:"flag" help:"branch name" default:"main"`
}

type releasesIn struct {
	Client *Client `kit:"inject"`
	Repo   string  `kit:"arg" help:"owner/repo slug"`
}

type tagsIn struct {
	Client *Client `kit:"inject"`
	Repo   string  `kit:"arg" help:"owner/repo slug"`
}

type issuesIn struct {
	Client *Client `kit:"inject"`
	Repo   string  `kit:"arg" help:"owner/repo slug"`
	State  string  `kit:"flag" help:"open, closed, or all" default:"open"`
	Page   int     `kit:"flag" help:"page number" default:"1"`
}

type pullsIn struct {
	Client *Client `kit:"inject"`
	Repo   string  `kit:"arg" help:"owner/repo slug"`
	State  string  `kit:"flag" help:"open, closed, or all" default:"open"`
	Page   int     `kit:"flag" help:"page number" default:"1"`
}

type readmeIn struct {
	Client *Client `kit:"inject"`
	Repo   string  `kit:"arg" help:"owner/repo slug"`
	Branch string  `kit:"flag" help:"branch name (default: try main then master)"`
}

type fileIn struct {
	Client *Client `kit:"inject"`
	Repo   string  `kit:"arg" help:"owner/repo slug"`
	Path   string  `kit:"arg" help:"file path in the repository"`
	Branch string  `kit:"flag" help:"branch name" default:"main"`
}

type searchIn struct {
	Client *Client `kit:"inject"`
	Query  string  `kit:"arg" help:"search query"`
	Page   int     `kit:"flag" help:"page number" default:"1"`
}

type followersIn struct {
	Client   *Client `kit:"inject"`
	Username string  `kit:"arg" help:"GitHub username"`
	Page     int     `kit:"flag" help:"page number" default:"1"`
}

type followingIn struct {
	Client   *Client `kit:"inject"`
	Username string  `kit:"arg" help:"GitHub username"`
	Page     int     `kit:"flag" help:"page number" default:"1"`
}

type starsIn struct {
	Client   *Client `kit:"inject"`
	Username string  `kit:"arg" help:"GitHub username"`
	Page     int     `kit:"flag" help:"page number" default:"1"`
}
