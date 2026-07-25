package gh

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// surface.go is the route-to-surface table from the spec, written down as data
// rather than scattered through the decoders, plus the four pagers every
// listing in this tool is built from.
//
// The table is not decoration. `github surfaces` prints it, which means the
// answer to "can this tool read X" is a command rather than a reading of the
// source, and a route that is not in the table is Unsupported with exit code 7
// instead of a guess.

// RouteInfo is one row: what the route is, which surface answers it best, and
// what to fall back to when that surface declines.
type RouteInfo struct {
	Route    string `json:"route"              table:"route"`
	Primary  string `json:"primary"            table:"primary"`
	Fallback string `json:"fallback,omitempty" table:"fallback"`
	Note     string `json:"note,omitempty"     table:"note,truncate"`
}

// Routes is the whole index. Surface names are the ones in doc 01: html,
// route-json, xhr, search, feed, raw, git, embedded, ld-json.
var Routes = []RouteInfo{
	{"/{owner}/{repo}", "embedded", "route-json", "sidebarAbout lives only in the HTML payload"},
	{"/{owner}/{repo}/tree/{ref}/{path}", "route-json", "embedded", ""},
	{"/{owner}/{repo}/blob/{ref}/{path}", "embedded", "raw", "the route JSON dropped the metadata block, so the page is the read; bytes from raw"},
	{"/{owner}/{repo}/branches", "route-json", "xhr", ""},
	{"/{owner}/{repo}/refs", "xhr", "git", "names only, 6 KB against 588 KB"},
	{"/{owner}/{repo}/commits/{ref}", "route-json", "feed", ""},
	{"/{owner}/{repo}/commit/{sha}", "route-json", "raw", "the diff comes from .patch"},
	{"/{owner}/{repo}/compare/{a}...{b}", "route-json", "raw", ""},
	{"/{owner}/{repo}/issues/{n}", "embedded", "ld-json", "Relay preloaded queries"},
	{"/{owner}/{repo}/pull/{n}", "embedded", "raw", "Relay, plus .diff and .patch"},
	{"/{owner}/{repo}/discussions/{n}", "embedded", "", "Relay"},
	{"/{owner}/{repo}/issues", "search", "", "type=issues with a repo: qualifier"},
	{"/{owner}/{repo}/pulls", "search", "", "type=pullrequests"},
	{"/{owner}/{repo}/releases", "feed", "html", "releases.atom, then a page each for assets"},
	{"/{owner}/{repo}/releases/tag/{tag}", "html", "feed", "download counts exist nowhere else"},
	{"/{owner}/{repo}/tags", "feed", "git", "the feed is recent, git is complete"},
	{"/{owner}/{repo}/graphs/contributors", "xhr", "", "answers 202 while it computes, so it polls"},
	{"/{owner}/{repo}/wiki", "feed", "html", ""},
	{"/{owner}/{repo}.git/info/refs", "git", "", "every ref and its SHA in one request"},
	{"/{login}", "html", "ld-json", "microdata and microformats, no payload at all"},
	{"/{org}", "html", "xhr", "two deferred fragments under --deep"},
	{"/{login}?tab=repositories", "search", "html", "user: qualifier"},
	{"/{login}?tab=stars", "html", "", ""},
	{"/{login}.atom", "feed", "", ""},
	{"/users/{login}/hovercard", "xhr", "html", ""},
	{"/users/{login}/contributions", "xhr", "", "returns an HTML fragment, not JSON"},
	{"/orgs/{org}/people", "html", "", ""},
	{"/search", "search", "", "ten types, code is the one that needs a token"},
	{"/trending", "html", "", "no JSON equivalent exists, tokened or not"},
	{"/topics/{slug}", "html", "search", ""},
	{"gist.github.com/{id}", "html", "raw", ""},
	{"raw.githubusercontent.com/...", "raw", "", ""},
	{"codeload.github.com/...", "raw", "", ""},
}

// --- URL builders ---
//
// Every request in this tool goes through one of these. Building URLs in one
// place is what makes the escape hatch (`github url`) tell the truth about what
// the tool would actually fetch.

func repoURL(repo string) string { return BaseURL + "/" + repo }

func repoSubURL(repo, sub string) string { return BaseURL + "/" + repo + "/" + sub }

func treeURL(repo, ref, path string) string {
	u := BaseURL + "/" + repo + "/tree/" + ref
	if path != "" {
		u += "/" + path
	}
	return u
}

func blobURL(repo, ref, path string) string {
	return BaseURL + "/" + repo + "/blob/" + ref + "/" + path
}

func rawURL(repo, ref, path string) string {
	return RawURL + "/" + repo + "/" + ref + "/" + path
}

func commitURL(repo, sha string) string { return BaseURL + "/" + repo + "/commit/" + sha }

func threadURL(repo, segment string, number int) string {
	return BaseURL + "/" + repo + "/" + segment + "/" + strconv.Itoa(number)
}

func feedURL(path string) string { return BaseURL + "/" + strings.TrimPrefix(path, "/") }

func accountURL(login string) string { return BaseURL + "/" + login }

// gitRefsURL is the smart-protocol advertisement: every ref and its object id,
// in one request, with no page limit and no login.
func gitRefsURL(repo string) string {
	return BaseURL + "/" + repo + ".git/info/refs?service=git-upload-pack"
}

// searchURL builds a search request. Type is the site's own name for the type:
// repositories, issues, pullrequests, discussions, users, commits, registrypackages,
// wikis, topics, marketplace.
func searchURL(query, typ string, page int) string {
	v := url.Values{}
	v.Set("q", query)
	if typ != "" {
		v.Set("type", typ)
	}
	if page > 1 {
		v.Set("p", strconv.Itoa(page))
	}
	return BaseURL + "/search?" + v.Encode()
}

// --- the pagers ---

// A pager answers one question: given where we are, what is the next batch and
// where does that leave us. Four shapes cover every listing github.com has.
//
//	search  numbered pages, ?p=N, stops on a short page
//	cursor  Relay endCursor, stops when hasNextPage is false
//	rails   a rel="next" anchor in the markup, stops when it is absent
//	none    one response, and that is the whole list
//
// They share one driver so that --limit, cancellation, and the "stop asking for
// more once the caller has enough" rule are written once.

// fetchPage returns one batch and the token for the next batch. An empty next
// token ends the walk.
type fetchPage[T any] func(ctx context.Context, token string) (batch []T, next string, err error)

// paginate walks a listing, handing each record to emit as it arrives.
//
// Records are emitted as they decode, not collected and returned, because a
// listing of every repository in an organization should start printing on the
// first page rather than after the last one.
//
// limit <= 0 means no limit. The walk stops the moment the limit is reached, so
// asking for five records off a thousand-record listing costs one request.
func paginate[T any](ctx context.Context, limit int, fetch fetchPage[T], emit func(T) error) error {
	token := ""
	seen := 0
	for {
		if err := ctx.Err(); err != nil {
			return wrapNetwork("", err)
		}
		batch, next, err := fetch(ctx, token)
		if err != nil {
			return err
		}
		for _, rec := range batch {
			if err := emit(rec); err != nil {
				return err
			}
			seen++
			if limit > 0 && seen >= limit {
				return nil
			}
		}
		// A page that returned nothing ends the walk even when the surface
		// still claims a next token. Trusting the token alone is how a paginator
		// spins forever against a route that has started answering empty.
		if next == "" || next == token || len(batch) == 0 {
			return nil
		}
		token = next
	}
}

// pageToken decodes the numbered-page token. Keeping it as a helper rather than
// an inline strconv call means the "which page am I on" logic reads the same in
// all nine listings that use it.
func pageToken(token string) int {
	if token == "" {
		return 1
	}
	n, err := strconv.Atoi(token)
	if err != nil || n < 1 {
		return 1
	}
	return n
}
