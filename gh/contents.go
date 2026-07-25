package gh

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// contents.go reads directories and files.
//
// The two halves of a file live on two different hosts and it is worth being
// blunt about why. github.com/{repo}/blob/{ref}/{path} renders the file: it
// knows the language, the table of contents, and the extracted symbols, and it
// costs a few hundred kilobytes of page to say so. raw.githubusercontent.com
// serves the bytes and nothing else, in one request, with no page around them.
// So metadata comes from the route and content comes from raw, and a caller
// that only wants one of the two pays for one of the two.
//
// The symbol list is the part with no equivalent anywhere else. GitHub runs a
// symbol extractor over every blob it renders and ships the result in the route
// payload, tokened API or not, which is why `github symbols` exists at all.

// TreeOptions controls a directory listing.
type TreeOptions struct {
	// Ref is a branch, a tag, or a SHA. Empty means the default branch, which
	// GitHub resolves itself when the ref in the URL is HEAD, so an empty ref
	// costs no extra request.
	Ref string
	// Recursive walks subdirectories breadth-first. There is no ?recursive=1 on
	// this route, so this is one request per directory. For a whole large tree,
	// Archive is one request instead of hundreds.
	Recursive bool
	// Sizes fills Size on file entries, at one HEAD request each.
	Sizes bool
	// Limit stops the walk after this many entries. Zero means no limit.
	Limit int
}

// Tree emits the entries of one directory, or of a whole subtree under
// Recursive. path is repository-relative and empty means the root.
//
// Entries are emitted as each directory arrives rather than collected, so a
// recursive walk of a large repository starts printing immediately.
func (c *Client) Tree(ctx context.Context, repo, path string, opts TreeOptions, emit func(TreeEntry) error) error {
	if _, _, ok := SplitRepo(repo); !ok {
		return usageBadID("repository", repo, "owner/name")
	}
	ref := opts.Ref
	if ref == "" {
		ref = "HEAD"
	}

	sent := 0
	queue := []string{strings.Trim(path, "/")}
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return wrapNetwork("", err)
		}
		dir := queue[0]
		queue = queue[1:]

		entries, oid, err := c.treeDir(ctx, repo, ref, dir)
		if err != nil {
			return err
		}
		if oid != "" && opts.Recursive {
			// Pin the rest of the walk to the commit the first directory
			// resolved to. A push halfway through a recursive listing would
			// otherwise give a tree that never existed.
			ref = oid
		}
		for _, e := range entries {
			if opts.Sizes && strings.HasSuffix(e.Type, "file") {
				// A failed size lookup leaves the field absent rather than
				// zero. Absent means unknown here, and zero means empty file.
				if n, err := c.rawSize(ctx, repo, e.Ref, e.Path); err == nil {
					e.Size = &n
				}
			}
			if err := emit(e); err != nil {
				return err
			}
			sent++
			if opts.Limit > 0 && sent >= opts.Limit {
				return nil
			}
			if opts.Recursive && e.Type == "directory" {
				queue = append(queue, e.Path)
			}
		}
	}
	return nil
}

// treeDir reads one directory. It returns the entries and the commit the ref
// resolved to, which is what makes a recursive walk consistent: the caller can
// pin the rest of the walk to that SHA instead of racing a push.
func (c *Client) treeDir(ctx context.Context, repo, ref, dir string) ([]TreeEntry, string, error) {
	var env struct {
		Payload struct {
			Route json.RawMessage `json:"codeViewTreeRoute"`
		} `json:"payload"`
	}
	url := treeURL(repo, ref, dir)
	res, err := c.GetJSON(ctx, url, SurfaceRouteJSON, &env)
	if err != nil {
		return nil, "", err
	}
	if len(env.Payload.Route) == 0 {
		return nil, "", structureChanged(repo + " " + ref + ":" + dir)
	}

	var route struct {
		Path    string `json:"path"`
		RefInfo struct {
			Name       string `json:"name"`
			RefType    string `json:"refType"`
			CurrentOid string `json:"currentOid"`
		} `json:"refInfo"`
		Tree struct {
			Items []struct {
				Name        string `json:"name"`
				Path        string `json:"path"`
				ContentType string `json:"contentType"`
			} `json:"items"`
			TotalCount int `json:"totalCount"`
		} `json:"tree"`
	}
	if err := json.Unmarshal(env.Payload.Route, &route); err != nil {
		return nil, "", badPayload(repo, err)
	}

	// The ref that goes on the records is the resolved one. HEAD is what we
	// asked for and a branch name is what the URL said; neither is a fact you
	// can come back to in a week and get the same bytes from.
	resolved := firstNonEmpty(route.RefInfo.CurrentOid, route.RefInfo.Name, ref)

	out := make([]TreeEntry, 0, len(route.Tree.Items))
	for _, it := range route.Tree.Items {
		e := TreeEntry{Repo: repo, Ref: resolved, Name: it.Name, Path: it.Path, Type: it.ContentType}
		e.setIdentity(KindFile, repo+"@"+resolved+"/"+it.Path)
		e.URL = treeURL(repo, ref, it.Path)
		if strings.HasSuffix(it.ContentType, "file") {
			e.URL = blobURL(repo, ref, it.Path)
		}
		e.addSource(res.FinalURL)
		out = append(out, e)
	}
	return out, route.RefInfo.CurrentOid, nil
}

// rawSize asks raw.githubusercontent.com how big a file is without fetching it.
// A HEAD there answers with Content-Length and no body, which is what makes
// `github tree --sizes` merely slow rather than enormous.
func (c *Client) rawSize(ctx context.Context, repo, ref, path string) (int64, error) {
	h, err := c.Head(ctx, rawURL(repo, ref, path))
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(h.Get("Content-Length"), 10, 64)
	if err != nil {
		return 0, badPayload(path, err)
	}
	return n, nil
}

// defaultRef resolves a repository's default branch name. The repository root
// answers this on the route surface in about five kilobytes, which is the
// cheapest existence-and-default-branch probe github.com has: the HTML page is
// three hundred.
//
// A failure returns HEAD. HEAD works in every URL this package builds, so a
// probe that fails costs a feature (symbol extraction) and not the read.
func (c *Client) defaultRef(ctx context.Context, repo string) string {
	var env struct {
		Payload struct {
			Route struct {
				RefInfo struct {
					Name    string `json:"name"`
					RefType string `json:"refType"`
				} `json:"refInfo"`
			} `json:"codeViewRepoRoute"`
		} `json:"payload"`
	}
	if _, err := c.GetJSON(ctx, repoURL(repo), SurfaceRouteJSON, &env); err != nil {
		return "HEAD"
	}
	info := env.Payload.Route.RefInfo
	if info.Name == "" || info.RefType == "tree" {
		return "HEAD"
	}
	return info.Name
}

// --- blobs ---

// BlobOptions controls a file read.
type BlobOptions struct {
	Ref string
	// Content fetches the bytes from raw.githubusercontent.com, one extra
	// request. Without it the record is metadata only.
	Content bool
	// Styled fetches the page HTML for rawLines and the syntax highlighting
	// spans that run parallel to them. It is expensive and only a consumer that
	// re-renders the file wants it.
	Styled bool
}

// Blob reads one file: its metadata, its rendered view, and its symbols.
func (c *Client) Blob(ctx context.Context, repo, path string, opts BlobOptions) (*File, error) {
	if _, _, ok := SplitRepo(repo); !ok {
		return nil, usageBadID("repository", repo, "owner/name")
	}
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, usageBadID("file path", path, "a path inside the repository")
	}
	ref := opts.Ref
	if ref == "" {
		// HEAD works in the URL but costs the symbol list: GitHub calls a HEAD
		// page refType "tree" and skips the analysis, so symbols come back
		// null. A named branch or a SHA gets them. Resolving is 5 KB and it is
		// cached, so it is cheaper than the surprise.
		ref = c.defaultRef(ctx, repo)
	}

	f := &File{Repo: repo, Ref: ref, Path: path}
	f.setIdentity(KindFile, repo+"@"+ref+"/"+path)
	f.URL = blobURL(repo, ref, path)
	f.RawURL = rawURL(repo, ref, path)

	url := blobURL(repo, ref, path)
	final, err := c.blobRoute(ctx, f, url)
	if err != nil {
		return nil, err
	}
	f.addSource(final)

	// Symbols are served by a background analyser whose result is cached for a
	// short while, so the same URL answers with the symbol list one second and
	// null the next, on either surface, with any headers. Nothing about the
	// request changes it. So an unavailable list is retried: once on the page,
	// which is a different cache, and once more on the route with our own cache
	// entry dropped. Two extra requests is worth the difference between a
	// symbol list and silence, and after that the record says unavailable and
	// means it.
	if opts.Styled || f.SymbolsStatus == "unavailable" {
		if err := c.readBlobPage(ctx, f, opts.Styled); err == nil {
			f.addSource(url)
		}
	}
	if f.SymbolsStatus == "unavailable" {
		c.cacheDrop(url, SurfaceRouteJSON)
		if _, err := c.blobRoute(ctx, f, url); err != nil {
			return nil, err
		}
	}
	if opts.Content && !f.IsBinary {
		b, err := c.Raw(ctx, repo, ref, path)
		if err != nil {
			return nil, err
		}
		f.Content = string(b)
		f.addSource(f.RawURL)
		recordVia(&f.Base, "content", "raw")
		if f.Size == nil {
			n := int64(len(b))
			f.Size = &n
		}
		if f.Lines == nil {
			f.Lines = intp(strings.Count(f.Content, "\n") + 1)
		}
	}
	return f, nil
}

// blobRoute fetches and decodes the render half of a blob into f, returning the
// URL it ended up reading. It is a function rather than inline code because the
// page fallback decodes the same block a second time.
func (c *Client) blobRoute(ctx context.Context, f *File, url string) (string, error) {
	var env struct {
		Payload struct {
			Route json.RawMessage `json:"codeViewBlobRoute"`
		} `json:"payload"`
	}
	res, err := c.GetJSON(ctx, url, SurfaceRouteJSON, &env)
	if err != nil {
		return "", err
	}
	if len(env.Payload.Route) == 0 {
		return "", structureChanged(f.Repo + ":" + f.Path)
	}
	if err := decodeBlobRoute(f, env.Payload.Route); err != nil {
		return "", err
	}
	return res.FinalURL, nil
}

// blobRouteData is the render half of a blob: what GitHub worked out about the file
// while displaying it. The bytes are not in here and that is deliberate on
// their side, not an omission on ours.
type blobRouteData struct {
	HeaderInfo struct {
		TOC []struct {
			Level  int    `json:"level"`
			Text   string `json:"text"`
			Anchor string `json:"anchor"`
		} `json:"toc"`
	} `json:"headerInfo"`
	RichText          string `json:"richText"`
	RichTextTruncated bool   `json:"richTextTruncated"`
	// A pointer because null and an empty object mean different things here:
	// null is "this page did not run the analyser", an empty symbol list with
	// no flags set is "this file has no definitions".
	Symbols *struct {
		TimedOut    bool `json:"timed_out"`
		NotAnalyzed bool `json:"not_analyzed"`
		Symbols     []struct {
			Name        string `json:"name"`
			Kind        string `json:"kind"`
			FQN         string `json:"fully_qualified_name"`
			IdentStart  int    `json:"ident_start"`
			IdentEnd    int    `json:"ident_end"`
			ExtentStart int    `json:"extent_start"`
			ExtentEnd   int    `json:"extent_end"`
		} `json:"symbols"`
	} `json:"symbols"`
}

func decodeBlobRoute(f *File, raw json.RawMessage) error {
	var v blobRouteData
	if err := json.Unmarshal(raw, &v); err != nil {
		return badPayload(f.Path, err)
	}
	// Assigned, not appended. This block gets decoded twice when the page
	// fallback runs, and appending would give a file two of every heading.
	f.TOC = nil
	for _, h := range v.HeaderInfo.TOC {
		f.TOC = append(f.TOC, Heading{Level: h.Level, Text: h.Text, Anchor: h.Anchor})
	}
	f.RichText = v.RichText
	// Four states, not two. An empty list with not_analyzed means the language
	// is unsupported, an empty list with no flags means the file genuinely has
	// no definitions, timed_out means ask again later, and a missing block
	// means the page never ran the analyser at all. Collapsing those into "no
	// symbols" would be a lie in three of the four cases.
	switch {
	case v.Symbols == nil:
		f.SymbolsStatus = "unavailable"
	case v.Symbols.TimedOut:
		f.SymbolsStatus = "timed_out"
	case v.Symbols.NotAnalyzed:
		f.SymbolsStatus = "not_analyzed"
	default:
		f.SymbolsStatus = "ok"
	}
	if v.Symbols != nil {
		f.Symbols = nil
		for _, s := range v.Symbols.Symbols {
			f.Symbols = append(f.Symbols, Symbol{
				Name:               s.Name,
				Kind:               s.Kind,
				FullyQualifiedName: s.FQN,
				IdentStart:         s.IdentStart,
				IdentEnd:           s.IdentEnd,
				ExtentStart:        s.ExtentStart,
				ExtentEnd:          s.ExtentEnd,
			})
		}
	}

	f.addExtra("codeViewBlobRoute", decodeExtra(raw, &v,
		// Spreadsheet rendering, and the two template editors. All three are
		// front-end views of the same bytes the record already points at.
		"csv", "csvError", "issueTemplate", "discussionTemplate",
		"renderedFileInfo", "richTextTruncated",
	))
	return nil
}

// readBlobPage reads the page for the three things the route JSON does not
// reliably give: the blob's own metadata, a symbol list that is actually there,
// and, when styled is set, the per-line source with its highlight spans.
//
// The styled key really does contain a dot in its name and really is not
// nested. It is payload["codeViewBlobLayoutRoute.StyledBlob"], one key, and a
// Go decoder that assumes otherwise silently gets nothing. This has caught
// everyone who has read this page once.
func (c *Client) readBlobPage(ctx context.Context, f *File, styled bool) error {
	res, err := c.GetHTML(ctx, blobURL(f.Repo, f.Ref, f.Path))
	if err != nil {
		return err
	}
	var env struct {
		Payload map[string]json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(embeddedPayload(res.Body), &env); err != nil {
		return badPayload(f.Path, err)
	}
	if raw, ok := env.Payload["codeViewBlobRoute"]; ok && f.SymbolsStatus != "ok" {
		if err := decodeBlobRoute(f, raw); err != nil {
			return err
		}
		recordVia(&f.Base, "symbols", "embedded")
	}
	if raw, ok := env.Payload["codeViewBlobLayoutRoute"]; ok {
		var meta struct {
			Blob struct {
				Language   string `json:"language"`
				Image      bool   `json:"image"`
				Viewable   bool   `json:"viewable"`
				Large      bool   `json:"large"`
				Truncated  bool   `json:"truncated"`
				HeaderInfo struct {
					BlobSize string `json:"blobSize"`
					IsGitLFS bool   `json:"isGitLfs"`
					LineInfo struct {
						// Strings, not numbers, and "truncated" here is
						// GitHub's own word for "lines of code": loc counts
						// every line, sloc skips the blank ones.
						TruncatedLoc  string `json:"truncatedLoc"`
						TruncatedSloc string `json:"truncatedSloc"`
					} `json:"lineInfo"`
				} `json:"headerInfo"`
			} `json:"blob"`
		}
		if json.Unmarshal(raw, &meta) == nil {
			b := meta.Blob
			f.Language = firstNonEmpty(f.Language, b.Language)
			f.SizeDisplay = firstNonEmpty(f.SizeDisplay, b.HeaderInfo.BlobSize)
			f.IsLFS = f.IsLFS || b.HeaderInfo.IsGitLFS
			// A file GitHub will not render as text is a binary as far as a
			// reader is concerned, and image is the common case of that.
			f.IsBinary = f.IsBinary || b.Image || !b.Viewable
			f.IsTruncated = f.IsTruncated || b.Truncated || b.Large
			if f.Lines == nil {
				if n, err := strconv.Atoi(b.HeaderInfo.LineInfo.TruncatedLoc); err == nil {
					f.Lines = intp(n)
				}
			}
		}
	}
	if !styled {
		return nil
	}
	raw, ok := env.Payload["codeViewBlobLayoutRoute.StyledBlob"]
	if !ok {
		return structureChanged(f.Path)
	}
	var blob struct {
		RawLines []string `json:"rawLines"`
	}
	if err := json.Unmarshal(raw, &blob); err != nil {
		return badPayload(f.Path, err)
	}
	f.RawLines = blob.RawLines
	if f.Lines == nil && len(blob.RawLines) > 0 {
		f.Lines = intp(len(blob.RawLines))
	}
	recordVia(&f.Base, "raw_lines", "embedded")
	return nil
}

// --- raw bytes ---

// Raw returns the bytes of a file. One request to raw.githubusercontent.com,
// no page, no negotiation, and the same contract for a binary as for text.
func (c *Client) Raw(ctx context.Context, repo, ref, path string) ([]byte, error) {
	if _, _, ok := SplitRepo(repo); !ok {
		return nil, usageBadID("repository", repo, "owner/name")
	}
	if ref == "" {
		ref = "HEAD"
	}
	res, err := c.Get(ctx, rawURL(repo, ref, strings.TrimPrefix(path, "/")), SurfaceRaw)
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

// Archive streams a repository tarball or zipball from codeload. The caller
// closes the reader. Nothing here is cached or buffered: an archive is measured
// in tens of megabytes and belongs on a disk, not in a map.
//
// format is tar.gz or zip. ref may be a branch, a tag, or a SHA, and the three
// take different codeload paths, which is what refPath sorts out.
func (c *Client) Archive(ctx context.Context, repo, ref, format string) (io.ReadCloser, error) {
	if _, _, ok := SplitRepo(repo); !ok {
		return nil, usageBadID("repository", repo, "owner/name")
	}
	switch format {
	case "", "tar.gz":
		format = "tar.gz"
	case "zip":
	default:
		return nil, usageBadID("archive format", format, "tar.gz or zip")
	}
	if ref == "" {
		ref = "HEAD"
	}
	url := CodeLoad + "/" + repo + "/" + format + "/" + ref
	body, _, err := c.Stream(ctx, url)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// --- shared ---

// embeddedPayload finds the react-app embedded data block in a page and returns
// it. pkg/page does this properly for every block on a page; this is the one
// case that needs the raw envelope back, because the interesting key contains a
// dot and has to be read out of a map rather than a struct.
func embeddedPayload(doc []byte) []byte {
	const open = `data-target="react-app.embeddedData">`
	i := strings.Index(string(doc), open)
	if i < 0 {
		return []byte("{}")
	}
	rest := string(doc[i+len(open):])
	j := strings.Index(rest, "</script>")
	if j < 0 {
		return []byte("{}")
	}
	return []byte(strings.TrimSpace(rest[:j]))
}

// Head issues a HEAD and returns the response headers. It exists for one
// question, "how big is this file", which is worth asking without downloading
// the answer.
func (c *Client) Head(ctx context.Context, url string) (http.Header, error) {
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.release()
	c.pace(ctx)

	req, err := c.newRequest(ctx, url, SurfaceRaw)
	if err != nil {
		return nil, err
	}
	req.Method = http.MethodHead
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, wrapNetwork(url, err)
	}
	_ = res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, statusError(url, res.StatusCode, nil)
	}
	return res.Header, nil
}
