package github

import (
	"encoding/xml"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// ── Atom wire types ──────────────────────────────────────────────────────────

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID        string `xml:"id"`
	Title     string `xml:"title"`
	Published string `xml:"published"`
	Updated   string `xml:"updated"`
	Author    struct {
		Name  string `xml:"name"`
		Email string `xml:"email"`
	} `xml:"author"`
	Link struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Content string `xml:"content"`
}

// ── compile-time regexes ─────────────────────────────────────────────────────

// Trending
var (
	reTrendingArticle = regexp.MustCompile(`(?s)<article\b[^>]*class="[^"]*Box-row[^"]*"[^>]*>(.*?)</article>`)
	reTrendingLink    = regexp.MustCompile(`href="/([^/"]+/[^/"]+)"`)
	reTrendingDesc    = regexp.MustCompile(`(?s)<p\b[^>]*class="[^"]*col-9[^"]*"[^>]*>(.*?)</p>`)
	reTrendingLang    = regexp.MustCompile(`itemprop="programmingLanguage"[^>]*>\s*([^<]+?)\s*<`)
	reTrendingStars   = regexp.MustCompile(`href="[^"]+/stargazers"[^>]*>\s*(?:<[^>]+>)*\s*([0-9,]+)`)
	reTrendingForks   = regexp.MustCompile(`href="[^"]+/network/members"[^>]*>\s*(?:<[^>]+>)*\s*([0-9,]+)`)
	reTrendingPeriod  = regexp.MustCompile(`([0-9,]+)\s+stars?\s+(?:today|this week|this month)`)
)

// User profile
var (
	reUserName      = regexp.MustCompile(`(?:itemprop="name"|class="[^"]*p-name[^"]*")[^>]*>\s*([^<\n]+?)\s*<`)
	reUserBio       = regexp.MustCompile(`class="[^"]*p-note[^"]*"[^>]*>\s*<div[^>]*>\s*(.*?)\s*</div>`)
	reUserBioAlt    = regexp.MustCompile(`class="[^"]*p-note[^"]*"[^>]*>([^<]+)<`)
	reUserCompany   = regexp.MustCompile(`class="[^"]*p-org[^"]*"[^>]*>(?:<[^>]+>)*\s*([^<\n]+?)\s*(?:</|<span)`)
	reUserLocation  = regexp.MustCompile(`aria-label="Home location"[^>]*>(?:[^<]*<[^/][^>]*>)*\s*([^<]+?)\s*<`)
	reUserEmail     = regexp.MustCompile(`class="[^"]*u-email[^"]*"[^>]*>([^<]+)<`)
	reUserBlog      = regexp.MustCompile(`href="(https?://[^"]+)"[^>]*rel="nofollow me"`)
	reUserFollowers = regexp.MustCompile(`tab=followers"[^>]*>\s*<span[^>]*>([\d,k]+)<`)
	reUserFollowing = regexp.MustCompile(`tab=following"[^>]*>\s*<span[^>]*>([\d,k]+)<`)
	reUserRepos     = regexp.MustCompile(`tab=repositories"[^>]*>\s*<span[^>]*>([\d,k]+)<`)
)

// Repo list (repos tab) — each repo in an <li itemprop="owns"> block
var (
	reRepoItem      = regexp.MustCompile(`(?s)<li\b[^>]*itemprop="owns"[^>]*>(.*?)</li>`)
	reRepoItemLink  = regexp.MustCompile(`href="/([^/"]+/[^/"]+)"`)
	reRepoItemDesc  = regexp.MustCompile(`(?s)itemprop="description"[^>]*>\s*(.*?)\s*</`)
	reRepoItemLang  = regexp.MustCompile(`itemprop="programmingLanguage"[^>]*>([^<]+)<`)
	reRepoItemStars = regexp.MustCompile(`href="[^"]+/stargazers"[^>]*>\s*([0-9,k]+)`)
	reRepoItemForks = regexp.MustCompile(`href="[^"]+/network/members"[^>]*>\s*([0-9,k]+)`)
	reRepoItemDate  = regexp.MustCompile(`<relative-time[^>]+datetime="([^"]+)"`)
)

// Single repo page
var (
	reRepoDesc    = regexp.MustCompile(`(?s)class="[^"]*f4 my-3[^"]*"[^>]*>\s*(.*?)\s*</p>`)
	reRepoDescAlt = regexp.MustCompile(`(?s)class="[^"]*about-description[^"]*"[^>]*>\s*(.*?)\s*</p>`)
	reRepoLang    = regexp.MustCompile(`class="[^"]*color-fg-default[^"]*"\s+itemprop="programmingLanguage"[^>]*>([^<]+)<`)
	reRepoLangAlt = regexp.MustCompile(`itemprop="programmingLanguage"[^>]*>([^<]+)<`)
	reRepoStars   = regexp.MustCompile(`href="/[^/]+/[^/]+/stargazers[^"]*"[^>]*>(?:[^<]*<[^>]+>)*\s*([0-9,]+)`)
	reRepoForks   = regexp.MustCompile(`href="/[^/]+/[^/]+/forks[^"]*"[^>]*>(?:[^<]*<[^>]+>)*\s*([0-9,]+)`)
	reRepoTopics  = regexp.MustCompile(`class="[^"]*topic-tag[^"]*"[^>]*>\s*([^<]+?)\s*<`)
	reRepoLicense = regexp.MustCompile(`/blob/[^"]*LICENSE[^"]*"[^>]*>(?:[^<]*<[^>]+>)*\s*([^<]+?)\s*<`)
	reRepoBranch  = regexp.MustCompile(`data-menu-button[^>]*>\s*(?:<[^>]+>)*\s*([^\s<]+)\s*(?:<|$)`)
	reRepoFork    = regexp.MustCompile(`Forked from`)
	reRepoArchive = regexp.MustCompile(`(?i)archived`)
	reRepoIssues  = regexp.MustCompile(`href="/[^/]+/[^/]+/issues"[^>]*>(?:[^<]*<[^>]+>)*\s*([0-9,]+)`)
	reRepoWatchers = regexp.MustCompile(`href="/[^/]+/[^/]+/watchers[^"]*"[^>]*>(?:[^<]*<[^>]+>)*\s*([0-9,]+)`)
)

// Issues
var (
	reIssueBlock    = regexp.MustCompile(`(?s)id="issue_(\d+)"[^>]*>(.*?)(?:id="issue_\d+"|</div>\s*</div>\s*</div>)`)
	reIssueTitle    = regexp.MustCompile(`class="Link--primary[^"]*"[^>]*href="([^"]+)"[^>]*>\s*\n?\s*([^<\n]+)`)
	reIssueDate     = regexp.MustCompile(`<relative-time[^>]+datetime="([^"]+)"`)
	reIssueAuthor   = regexp.MustCompile(`opened by\s*<a[^>]*>([^<]+)<`)
	reIssueLabel    = regexp.MustCompile(`class="[^"]*IssueLabel[^"]*"[^>]*>([^<]+)<`)
	reIssueComments = regexp.MustCompile(`(\d+)\s+comment`)
)

// Pull requests
var (
	rePRTitle = regexp.MustCompile(`class="Link--primary[^"]*"[^>]*href="(/[^/]+/[^/]+/pull/(\d+))"[^>]*>\s*\n?\s*([^<\n]+)`)
)

// Search
var (
	reSearchName  = regexp.MustCompile(`href="/([^/"]+/[^/"]+)"[^>]*class="v-align-middle`)
	reSearchNameB = regexp.MustCompile(`class="v-align-middle[^"]*"[^>]*href="/([^/"]+/[^/"]+)"`)
	reSearchDesc  = regexp.MustCompile(`(?s)<p\b[^>]*class="[^"]*mb-1[^"]*"[^>]*>\s*(.*?)\s*</p>`)
	reSearchStars = regexp.MustCompile(`([0-9,]+)\s+stars?`)
	reSearchLang  = regexp.MustCompile(`(?s)<span\b[^>]*class="[^"]*search-match[^"]*"[^>]*>([^<]+)<`)
	reSearchDate  = regexp.MustCompile(`<relative-time[^>]+datetime="([^"]+)"`)
)

// Followers/Following
var (
	reFollowerLogin = regexp.MustCompile(`data-hovercard-type="user"[^>]*href="/([^"]+)"`)
	reFollowerName  = regexp.MustCompile(`class="[^"]*Link--secondary[^"]*"[^>]*>([^<]+)<`)
)

// Stars tab
var (
	reStarName  = regexp.MustCompile(`href="/([^/"]+/[^/"]+)"[^>]*class="[^"]*Link--primary`)
	reStarNameB = regexp.MustCompile(`class="[^"]*Link--primary[^"]*"[^>]*href="/([^/"]+/[^/"]+)"`)
	reStarDesc  = regexp.MustCompile(`(?s)<p\b[^>]*class="[^"]*col-9[^"]*"[^>]*>(.*?)</p>`)
	reStarLang  = regexp.MustCompile(`itemprop="programmingLanguage"[^>]*>([^<]+)<`)
	reStarStars = regexp.MustCompile(`href="[^"]+/stargazers"[^>]*>\s*([0-9,]+)`)
)

// ── helpers ──────────────────────────────────────────────────────────────────

// cleanInt parses a comma-formatted or k-suffixed integer string.
// "12,345" → 12345; "3.2k" → 3200; returns 0 on failure.
func cleanInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// handle k suffix
	if strings.HasSuffix(s, "k") || strings.HasSuffix(s, "K") {
		f, err := strconv.ParseFloat(strings.ReplaceAll(s[:len(s)-1], ",", ""), 64)
		if err != nil {
			return 0
		}
		return int(f * 1000)
	}
	s = strings.ReplaceAll(s, ",", "")
	n, _ := strconv.Atoi(s)
	return n
}

// cleanStr strips HTML tags, decodes HTML entities, and trims whitespace.
func cleanStr(s string) string {
	// strip tags
	reTag := regexp.MustCompile(`<[^>]+>`)
	s = reTag.ReplaceAllString(s, " ")
	// decode entities
	s = html.UnescapeString(s)
	// collapse whitespace
	reWS := regexp.MustCompile(`\s+`)
	s = reWS.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// extractSHA extracts the commit SHA from a GitHub Atom entry ID.
// IDs look like: tag:github.com,2008:Grit::Commit/abc1234567890
func extractSHA(id string) string {
	if idx := strings.LastIndex(id, "/"); idx >= 0 {
		return id[idx+1:]
	}
	return id
}

// first returns the first capture group match, or "".
func first(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if m == nil || len(m) < 2 {
		return ""
	}
	return cleanStr(m[1])
}

// ── ParseTrending ────────────────────────────────────────────────────────────

// ParseTrending parses the github.com/trending HTML page.
func ParseTrending(body string) []TrendingRepo {
	articles := reTrendingArticle.FindAllStringSubmatch(body, -1)
	out := make([]TrendingRepo, 0, len(articles))
	for i, m := range articles {
		block := m[1]

		fullName := first(reTrendingLink, block)
		if fullName == "" {
			continue
		}

		desc := ""
		if dm := reTrendingDesc.FindStringSubmatch(block); dm != nil {
			desc = cleanStr(dm[1])
		}

		lang := first(reTrendingLang, block)
		stars := 0
		if sm := reTrendingStars.FindStringSubmatch(block); sm != nil {
			stars = cleanInt(sm[1])
		}
		forks := 0
		if fm := reTrendingForks.FindStringSubmatch(block); fm != nil {
			forks = cleanInt(fm[1])
		}
		period := 0
		if pm := reTrendingPeriod.FindStringSubmatch(block); pm != nil {
			period = cleanInt(pm[1])
		}

		out = append(out, TrendingRepo{
			Rank:        i + 1,
			FullName:    fullName,
			Description: desc,
			Language:    lang,
			Stars:       stars,
			Forks:       forks,
			PeriodStars: period,
			URL:         "https://github.com/" + fullName,
		})
	}
	return out
}

// ── ParseUser ────────────────────────────────────────────────────────────────

// ParseUser parses a github.com/{username} profile page.
func ParseUser(body, username string) (User, error) {
	name := first(reUserName, body)

	bio := ""
	if bm := reUserBio.FindStringSubmatch(body); bm != nil {
		bio = cleanStr(bm[1])
	} else if bm2 := reUserBioAlt.FindStringSubmatch(body); bm2 != nil {
		bio = cleanStr(bm2[1])
	}

	company := first(reUserCompany, body)
	location := first(reUserLocation, body)
	email := first(reUserEmail, body)
	blog := first(reUserBlog, body)

	followers := 0
	if fm := reUserFollowers.FindStringSubmatch(body); fm != nil {
		followers = cleanInt(fm[1])
	}
	following := 0
	if fm := reUserFollowing.FindStringSubmatch(body); fm != nil {
		following = cleanInt(fm[1])
	}
	repos := 0
	if rm := reUserRepos.FindStringSubmatch(body); rm != nil {
		repos = cleanInt(rm[1])
	}

	return User{
		Login:     username,
		Name:      name,
		Bio:       bio,
		Company:   company,
		Location:  location,
		Email:     email,
		Blog:      blog,
		Followers: followers,
		Following: following,
		Repos:     repos,
		URL:       userURL(username),
	}, nil
}

// ── ParseRepos ───────────────────────────────────────────────────────────────

// ParseRepos parses the github.com/{username}?tab=repositories HTML page.
func ParseRepos(body, username string) []Repo {
	items := reRepoItem.FindAllStringSubmatch(body, -1)
	out := make([]Repo, 0, len(items))
	for _, m := range items {
		block := m[1]

		fullName := first(reRepoItemLink, block)
		if fullName == "" {
			continue
		}

		desc := ""
		if dm := reRepoItemDesc.FindStringSubmatch(block); dm != nil {
			desc = cleanStr(dm[1])
		}

		lang := first(reRepoItemLang, block)
		stars := 0
		if sm := reRepoItemStars.FindStringSubmatch(block); sm != nil {
			stars = cleanInt(sm[1])
		}
		forks := 0
		if fm := reRepoItemForks.FindStringSubmatch(block); fm != nil {
			forks = cleanInt(fm[1])
		}
		pushedAt := first(reRepoItemDate, block)

		out = append(out, Repo{
			FullName:    fullName,
			Description: desc,
			Language:    lang,
			Stars:       stars,
			Forks:       forks,
			PushedAt:    pushedAt,
			URL:         "https://github.com/" + fullName,
		})
	}
	return out
}

// ── ParseRepo ────────────────────────────────────────────────────────────────

// ParseRepo parses the github.com/{owner}/{repo} page.
func ParseRepo(body, owner, repo string) (Repo, error) {
	fullName := owner + "/" + repo

	desc := ""
	if dm := reRepoDesc.FindStringSubmatch(body); dm != nil {
		desc = cleanStr(dm[1])
	} else if dm2 := reRepoDescAlt.FindStringSubmatch(body); dm2 != nil {
		desc = cleanStr(dm2[1])
	}

	lang := ""
	if lm := reRepoLang.FindStringSubmatch(body); lm != nil {
		lang = cleanStr(lm[1])
	} else if lm2 := reRepoLangAlt.FindStringSubmatch(body); lm2 != nil {
		lang = cleanStr(lm2[1])
	}

	stars := 0
	if sm := reRepoStars.FindStringSubmatch(body); sm != nil {
		stars = cleanInt(sm[1])
	}
	forks := 0
	if fm := reRepoForks.FindStringSubmatch(body); fm != nil {
		forks = cleanInt(fm[1])
	}
	openIssues := 0
	if im := reRepoIssues.FindStringSubmatch(body); im != nil {
		openIssues = cleanInt(im[1])
	}
	watchers := 0
	if wm := reRepoWatchers.FindStringSubmatch(body); wm != nil {
		watchers = cleanInt(wm[1])
	}

	// topics
	topicMatches := reRepoTopics.FindAllStringSubmatch(body, -1)
	topics := make([]string, 0, len(topicMatches))
	for _, tm := range topicMatches {
		t := cleanStr(tm[1])
		if t != "" {
			topics = append(topics, t)
		}
	}

	license := first(reRepoLicense, body)
	branch := first(reRepoBranch, body)
	if branch == "" {
		branch = "main"
	}

	isFork := reRepoFork.MatchString(body)
	isArchived := reRepoArchive.MatchString(body)

	return Repo{
		FullName:      fullName,
		Description:   desc,
		Language:      lang,
		Stars:         stars,
		Forks:         forks,
		Watchers:      watchers,
		OpenIssues:    openIssues,
		DefaultBranch: branch,
		License:       license,
		Topics:        topics,
		Fork:          isFork,
		Archived:      isArchived,
		URL:           repoURL(owner, repo),
	}, nil
}

// ── Atom feeds ───────────────────────────────────────────────────────────────

// ParseAtomCommits parses the /commits/{branch}.atom feed.
func ParseAtomCommits(body string) ([]Commit, error) {
	var feed atomFeed
	if err := xml.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}
	out := make([]Commit, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		sha := extractSHA(e.ID)
		if len(sha) > 7 {
			sha = sha[:7]
		}
		out = append(out, Commit{
			SHA:     sha,
			Message: strings.TrimSpace(e.Title),
			Author:  strings.TrimSpace(e.Author.Name),
			Date:    e.Published,
			URL:     e.Link.Href,
		})
	}
	return out, nil
}

// ParseAtomReleases parses the /releases.atom feed.
func ParseAtomReleases(body string) ([]Release, error) {
	var feed atomFeed
	if err := xml.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}
	out := make([]Release, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		tag := lastPathSegment(e.Link.Href)
		out = append(out, Release{
			Tag:       tag,
			Name:      strings.TrimSpace(e.Title),
			Author:    strings.TrimSpace(e.Author.Name),
			Published: e.Published,
			URL:       e.Link.Href,
		})
	}
	return out, nil
}

// ParseAtomTags parses the /tags.atom feed.
func ParseAtomTags(body string) ([]Tag, error) {
	var feed atomFeed
	if err := xml.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}
	out := make([]Tag, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		out = append(out, Tag{
			Name:    strings.TrimSpace(e.Title),
			Updated: e.Updated,
			URL:     e.Link.Href,
		})
	}
	return out, nil
}

// ── ParseIssues ──────────────────────────────────────────────────────────────

// ParseIssues parses the /{owner}/{repo}/issues HTML page.
func ParseIssues(body, owner, repo, state string) []Issue {
	// find all issue-N id blocks
	matches := reIssueBlock.FindAllStringSubmatch(body, -1)
	out := make([]Issue, 0, len(matches))
	for _, m := range matches {
		numStr := m[1]
		block := m[2]
		num, _ := strconv.Atoi(numStr)
		if num == 0 {
			continue
		}

		title := ""
		issueURL := ""
		if tm := reIssueTitle.FindStringSubmatch(block); tm != nil {
			issueURL = "https://github.com" + tm[1]
			title = cleanStr(tm[2])
		}

		createdAt := first(reIssueDate, block)
		author := first(reIssueAuthor, block)

		labelMatches := reIssueLabel.FindAllStringSubmatch(block, -1)
		labels := make([]string, 0, len(labelMatches))
		for _, lm := range labelMatches {
			labels = append(labels, cleanStr(lm[1]))
		}

		comments := 0
		if cm := reIssueComments.FindStringSubmatch(block); cm != nil {
			comments, _ = strconv.Atoi(cm[1])
		}

		if issueURL == "" {
			issueURL = "https://github.com/" + owner + "/" + repo + "/issues/" + numStr
		}

		out = append(out, Issue{
			Number:    num,
			Title:     title,
			State:     state,
			Author:    author,
			Comments:  comments,
			Labels:    strings.Join(labels, ", "),
			CreatedAt: createdAt,
			URL:       issueURL,
		})
	}
	return out
}

// ── ParsePulls ───────────────────────────────────────────────────────────────

// ParsePulls parses the /{owner}/{repo}/pulls HTML page.
// The PR list uses the same HTML structure as issues with a different URL path.
func ParsePulls(body, owner, repo, state string) []PullRequest {
	// use the same issue_N id structure
	matches := reIssueBlock.FindAllStringSubmatch(body, -1)
	out := make([]PullRequest, 0, len(matches))
	for _, m := range matches {
		numStr := m[1]
		block := m[2]
		num, _ := strconv.Atoi(numStr)
		if num == 0 {
			continue
		}

		title := ""
		prURL := ""
		prNum := num
		if tm := rePRTitle.FindStringSubmatch(block); tm != nil {
			prURL = "https://github.com" + tm[1]
			n, _ := strconv.Atoi(tm[2])
			if n > 0 {
				prNum = n
			}
			title = cleanStr(tm[3])
		} else if tm2 := reIssueTitle.FindStringSubmatch(block); tm2 != nil {
			prURL = "https://github.com" + tm2[1]
			title = cleanStr(tm2[2])
		}

		createdAt := first(reIssueDate, block)
		author := first(reIssueAuthor, block)

		comments := 0
		if cm := reIssueComments.FindStringSubmatch(block); cm != nil {
			comments, _ = strconv.Atoi(cm[1])
		}

		if prURL == "" {
			prURL = "https://github.com/" + owner + "/" + repo + "/pull/" + numStr
		}

		out = append(out, PullRequest{
			Number:    prNum,
			Title:     title,
			State:     state,
			Author:    author,
			Comments:  comments,
			CreatedAt: createdAt,
			URL:       prURL,
		})
	}
	return out
}

// ── ParseSearch ──────────────────────────────────────────────────────────────

// ParseSearch parses the github.com/search?type=repositories results page.
func ParseSearch(body string) []SearchRepo {
	// find all result items by looking for full_name links
	nameMatches := reSearchName.FindAllStringSubmatch(body, -1)
	if len(nameMatches) == 0 {
		nameMatches = reSearchNameB.FindAllStringSubmatch(body, -1)
	}

	out := make([]SearchRepo, 0, len(nameMatches))
	// split body on each result card anchor to get per-card blocks
	// Use a simpler approach: find all v-align-middle hrefs
	reCard := regexp.MustCompile(`(?s)class="v-align-middle[^"]*"[^>]*href="/([^/"]+/[^/"]+)"[^>]*>.*?(?:class="v-align-middle|$)`)
	_ = reCard

	for i, nm := range nameMatches {
		fullName := nm[1]
		// carve out a block around this match to extract nearby metadata
		idx := strings.Index(body, nm[0])
		block := ""
		if idx >= 0 {
			end := idx + 2000
			if end > len(body) {
				end = len(body)
			}
			block = body[idx:end]
		}

		desc := ""
		if dm := reSearchDesc.FindStringSubmatch(block); dm != nil {
			desc = cleanStr(dm[1])
		}
		stars := 0
		if sm := reSearchStars.FindStringSubmatch(block); sm != nil {
			stars = cleanInt(sm[1])
		}
		lang := first(reSearchLang, block)
		updatedAt := first(reSearchDate, block)

		out = append(out, SearchRepo{
			Rank:        i + 1,
			FullName:    fullName,
			Description: desc,
			Language:    lang,
			Stars:       stars,
			UpdatedAt:   updatedAt,
			URL:         "https://github.com/" + fullName,
		})
	}
	return out
}

// ── ParseFollowers / ParseFollowing ─────────────────────────────────────────

// ParseFollowers parses the ?tab=followers HTML page.
func ParseFollowers(body string) []User {
	return parseUserGrid(body)
}

// ParseFollowing parses the ?tab=following HTML page.
func ParseFollowing(body string) []User {
	return parseUserGrid(body)
}

func parseUserGrid(body string) []User {
	loginMatches := reFollowerLogin.FindAllStringSubmatch(body, -1)
	out := make([]User, 0, len(loginMatches))
	seen := map[string]bool{}
	for _, lm := range loginMatches {
		login := cleanStr(lm[1])
		if login == "" || seen[login] {
			continue
		}
		// skip orgs and special pages
		if strings.Contains(login, "/") || strings.HasPrefix(login, "?") {
			continue
		}
		seen[login] = true

		// carve out a block near this login to look for display name
		idx := strings.Index(body, lm[0])
		name := ""
		if idx >= 0 {
			end := idx + 500
			if end > len(body) {
				end = len(body)
			}
			block := body[idx:end]
			name = first(reFollowerName, block)
		}

		out = append(out, User{
			Login: login,
			Name:  name,
			URL:   userURL(login),
		})
	}
	return out
}

// ── ParseStars ───────────────────────────────────────────────────────────────

// ParseStars parses the ?tab=stars HTML page.
func ParseStars(body string) []StarredRepo {
	nameMatches := reStarName.FindAllStringSubmatch(body, -1)
	if len(nameMatches) == 0 {
		nameMatches = reStarNameB.FindAllStringSubmatch(body, -1)
	}
	out := make([]StarredRepo, 0, len(nameMatches))
	seen := map[string]bool{}
	for _, nm := range nameMatches {
		fullName := nm[1]
		if fullName == "" || seen[fullName] {
			continue
		}
		if !strings.Contains(fullName, "/") {
			continue
		}
		seen[fullName] = true

		idx := strings.Index(body, nm[0])
		block := ""
		if idx >= 0 {
			end := idx + 1000
			if end > len(body) {
				end = len(body)
			}
			block = body[idx:end]
		}

		desc := ""
		if dm := reStarDesc.FindStringSubmatch(block); dm != nil {
			desc = cleanStr(dm[1])
		}
		lang := first(reStarLang, block)
		stars := 0
		if sm := reStarStars.FindStringSubmatch(block); sm != nil {
			stars = cleanInt(sm[1])
		}

		out = append(out, StarredRepo{
			FullName:    fullName,
			Description: desc,
			Language:    lang,
			Stars:       stars,
			URL:         "https://github.com/" + fullName,
		})
	}
	return out
}
