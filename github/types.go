// Package github is the scraper library behind the github CLI.
// It reads public GitHub data from HTML pages, Atom feeds, and
// raw.githubusercontent.com. No API key or authentication is required.
//
// github is an independent tool and is not affiliated with GitHub or Microsoft.
package github

// TrendingRepo is one entry from the GitHub trending page.
type TrendingRepo struct {
	Rank        int    `json:"rank"          table:"Rank,right"`
	FullName    string `json:"full_name"     table:"Repo"`
	Description string `json:"description"   table:"Description"`
	Language    string `json:"language"      table:"Lang"`
	Stars       int    `json:"stars"         table:"Stars,right"`
	Forks       int    `json:"forks"         table:"Forks,right"`
	PeriodStars int    `json:"period_stars"  table:"New Stars,right"`
	URL         string `json:"url"           table:"-"             kit:"url"`
}

// User is a GitHub user profile record.
// It is also used for the followers and following listings;
// counts are 0 on listing pages where they are not shown.
type User struct {
	Login     string `json:"login"       table:"Login"`
	Name      string `json:"name"        table:"Name"`
	Bio       string `json:"bio"         table:"-"`
	Company   string `json:"company"     table:"Company"`
	Location  string `json:"location"    table:"Location"`
	Email     string `json:"email"       table:"-"`
	Blog      string `json:"blog"        table:"-"`
	Followers int    `json:"followers"   table:"Followers,right"`
	Following int    `json:"following"   table:"Following,right"`
	Repos     int    `json:"repos"       table:"Repos,right"`
	URL       string `json:"url"         table:"-"              kit:"url"`
}

// Repo is a repository record used by both repo (single) and repos (list).
type Repo struct {
	FullName      string   `json:"full_name"       table:"Repo"`
	Description   string   `json:"description"     table:"Description"`
	Language      string   `json:"language"        table:"Lang"`
	Stars         int      `json:"stars"           table:"Stars,right"`
	Forks         int      `json:"forks"           table:"Forks,right"`
	Watchers      int      `json:"watchers"        table:"-"`
	OpenIssues    int      `json:"open_issues"     table:"Issues,right"`
	DefaultBranch string   `json:"default_branch"  table:"-"`
	License       string   `json:"license"         table:"License"`
	Topics        []string `json:"topics"          table:"-"`
	Fork          bool     `json:"fork"            table:"-"`
	Archived      bool     `json:"archived"        table:"-"`
	PushedAt      string   `json:"pushed_at"       table:"Pushed"`
	CreatedAt     string   `json:"created_at"      table:"-"`
	UpdatedAt     string   `json:"updated_at"      table:"-"`
	URL           string   `json:"url"             table:"-"              kit:"url"`
}

// Commit is one entry from the commits Atom feed.
type Commit struct {
	SHA     string `json:"sha"      table:"SHA"`
	Message string `json:"message"  table:"Message"`
	Author  string `json:"author"   table:"Author"`
	Date    string `json:"date"     table:"Date"`
	URL     string `json:"url"      table:"-"    kit:"url"`
}

// Release is one entry from the releases Atom feed.
type Release struct {
	Tag       string `json:"tag"       table:"Tag"`
	Name      string `json:"name"      table:"Name"`
	Author    string `json:"author"    table:"Author"`
	Published string `json:"published" table:"Published"`
	URL       string `json:"url"       table:"-"    kit:"url"`
}

// Tag is one entry from the tags Atom feed.
type Tag struct {
	Name    string `json:"name"    table:"Tag"`
	Updated string `json:"updated" table:"Updated"`
	URL     string `json:"url"     table:"-"    kit:"url"`
}

// Issue is one issue row scraped from the issues HTML page.
type Issue struct {
	Number    int    `json:"number"     table:"#,right"`
	Title     string `json:"title"      table:"Title"`
	State     string `json:"state"      table:"State"`
	Author    string `json:"author"     table:"Author"`
	Comments  int    `json:"comments"   table:"Comments,right"`
	Labels    string `json:"labels"     table:"Labels"`
	CreatedAt string `json:"created_at" table:"Created"`
	URL       string `json:"url"        table:"-"    kit:"url"`
}

// PullRequest is one PR row scraped from the pulls HTML page.
type PullRequest struct {
	Number    int    `json:"number"     table:"#,right"`
	Title     string `json:"title"      table:"Title"`
	State     string `json:"state"      table:"State"`
	Author    string `json:"author"     table:"Author"`
	Comments  int    `json:"comments"   table:"Comments,right"`
	CreatedAt string `json:"created_at" table:"Created"`
	URL       string `json:"url"        table:"-"    kit:"url"`
}

// SearchRepo is one repository card from the search results page.
type SearchRepo struct {
	Rank        int    `json:"rank"         table:"Rank,right"`
	FullName    string `json:"full_name"    table:"Repo"`
	Description string `json:"description"  table:"Description"`
	Language    string `json:"language"     table:"Lang"`
	Stars       int    `json:"stars"        table:"Stars,right"`
	UpdatedAt   string `json:"updated_at"   table:"Updated"`
	URL         string `json:"url"          table:"-"   kit:"url"`
}

// StarredRepo is one repository card from the stars tab.
type StarredRepo struct {
	FullName    string `json:"full_name"    table:"Repo"`
	Description string `json:"description"  table:"Description"`
	Language    string `json:"language"     table:"Lang"`
	Stars       int    `json:"stars"        table:"Stars,right"`
	URL         string `json:"url"          table:"-"   kit:"url"`
}

// FileContent is the result of the readme and file commands.
type FileContent struct {
	Path    string `json:"path"    table:"Path"`
	Content string `json:"content" table:"-"`
	URL     string `json:"url"     table:"-"   kit:"url"`
}
