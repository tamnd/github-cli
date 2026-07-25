package page

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// dom.go is the parsed-document half of the extractor, and the matcher it
// needs. The matcher handles tag name, id, class membership, attribute
// presence, attribute value, prefix and suffix on an attribute, and one level
// of descendant. That is every selector in selectors.go and nothing more,
// which is the reason there is no CSS selector dependency here.

// Doc parses the page once and caches the tree. Most reads never call it: the
// React pages carry their data as JSON and the scanner alone is enough.
func (p *Page) Doc() *html.Node {
	if p.doc != nil || len(p.HTML) == 0 {
		return p.doc
	}
	n, err := html.Parse(bytes.NewReader(p.HTML))
	if err != nil {
		return nil
	}
	p.doc = n
	return p.doc
}

// Sel is one selector. A zero field is "do not care", so a selector that only
// sets Class matches on class alone.
type Sel struct {
	Tag          string
	ID           string
	Class        string // one class, matched against the whitespace-separated list
	Attr         string // attribute that must be present
	AttrValue    string // and, if set, must equal this
	AttrPrefix   string
	AttrSuffix   string
	AttrContains string
	// HasDescendantClass requires a descendant element carrying this class.
	// It exists for the licence link, which is identified by the icon inside
	// it because the icon changes less often than the anchor's own classes.
	HasDescendantClass string
}

// Match reports whether n satisfies every field the selector set.
func (s Sel) Match(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	if s.Tag != "" && n.Data != s.Tag {
		return false
	}
	if s.ID != "" && Attr(n, "id") != s.ID {
		return false
	}
	if s.Class != "" && !HasClass(n, s.Class) {
		return false
	}
	if s.Attr != "" {
		v, ok := lookupAttr(n, s.Attr)
		if !ok {
			return false
		}
		if s.AttrValue != "" && v != s.AttrValue {
			return false
		}
		if s.AttrPrefix != "" && !strings.HasPrefix(v, s.AttrPrefix) {
			return false
		}
		if s.AttrSuffix != "" && !strings.HasSuffix(v, s.AttrSuffix) {
			return false
		}
		if s.AttrContains != "" && !strings.Contains(v, s.AttrContains) {
			return false
		}
	}
	if s.HasDescendantClass != "" && Find(n, Sel{Class: s.HasDescendantClass}) == nil {
		return false
	}
	return true
}

// Attr reads one attribute, empty when it is absent.
func Attr(n *html.Node, name string) string {
	v, _ := lookupAttr(n, name)
	return v
}

func lookupAttr(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

// HasClass matches one class in the whitespace-separated list, never a
// substring. `Box-row` must not match `Box-row-hover`.
func HasClass(n *html.Node, want string) bool {
	v, ok := lookupAttr(n, "class")
	if !ok {
		return false
	}
	for _, c := range strings.Fields(v) {
		if c == want {
			return true
		}
	}
	return false
}

// Find returns the first matching element, or nil.
func Find(root *html.Node, s Sel) *html.Node {
	var found *html.Node
	Walk(root, func(n *html.Node) bool {
		// Walk keeps visiting siblings after a subtree says stop, so the
		// answer has to be latched. Without this the last match wins instead
		// of the first, which on a page where a dialog repeats the same shape
		// as the content silently returns the dialog.
		if found != nil {
			return false
		}
		if s.Match(n) {
			found = n
			return false
		}
		return true
	})
	return found
}

// FindAll returns every matching element in document order.
func FindAll(root *html.Node, s Sel) []*html.Node {
	var out []*html.Node
	Walk(root, func(n *html.Node) bool {
		if s.Match(n) {
			out = append(out, n)
		}
		return true
	})
	return out
}

// Walk visits every node depth-first. Returning false from fn stops the
// traversal of that subtree and, once found is set, the search as a whole.
func Walk(n *html.Node, fn func(*html.Node) bool) {
	if n == nil {
		return
	}
	if !fn(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		Walk(c, fn)
	}
}

// Text returns the concatenated, whitespace-collapsed text of a subtree. It is
// the last-resort extractor and every caller of it is marked as such.
func Text(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	Walk(n, func(x *html.Node) bool {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		return true
	})
	return collapse(b.String())
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// blockTag is the set of elements that end a line of prose. It does not need to
// be the full HTML block list, only the tags GitHub's renderer actually emits
// into a README, a release note, or a comment body.
var blockTag = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"br": true, "dd": true, "details": true, "div": true, "dl": true,
	"dt": true, "figcaption": true, "figure": true, "footer": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "hr": true, "li": true, "main": true, "nav": true,
	"ol": true, "p": true, "pre": true, "section": true, "summary": true,
	"table": true, "tbody": true, "td": true, "th": true, "thead": true,
	"tr": true, "ul": true,
}

// BlockText returns the prose of a subtree with the line structure the markup
// implies, which is what Text deliberately throws away.
//
// Text collapses a whole subtree onto one line, which is right for a label and
// wrong for a document: a twenty-kilobyte README as a single line is not a
// readable rendering of anything. This keeps one line per block element, one
// blank line between paragraphs, and the interior whitespace of a <pre> exactly
// as it was, since indentation is the meaning of a code block rather than
// decoration on it.
func BlockText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var t textLines
	t.walk(n)
	return t.done()
}

// FragmentText is BlockText over an HTML fragment that arrived as a string.
// Several of GitHub's payloads carry rendered markup as a JSON value rather
// than as part of the document, so there is no node to walk until this parses
// one.
func FragmentText(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return ""
	}
	return BlockText(doc)
}

// textLines accumulates prose one line at a time. It exists because whether a
// line keeps its whitespace depends on where the line started, which a single
// pass over a string builder cannot know after the fact.
type textLines struct {
	out []string
	cur strings.Builder
	pre int  // depth inside <pre>
	raw bool // the line being built started inside a <pre>
}

func (t *textLines) walk(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		t.text(n.Data)
		return
	case html.ElementNode:
		switch n.Data {
		case "script", "style", "template":
			return
		case "pre":
			t.pre++
			defer func() { t.pre-- }()
		}
		if blockTag[n.Data] {
			t.brk()
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		t.walk(c)
	}
	if n.Type == html.ElementNode && blockTag[n.Data] {
		t.brk()
	}
}

func (t *textLines) text(s string) {
	if t.pre == 0 {
		t.cur.WriteString(s)
		return
	}
	t.raw = true
	for i, part := range strings.Split(s, "\n") {
		if i > 0 {
			t.brk()
			t.raw = true
		}
		t.cur.WriteString(part)
	}
}

func (t *textLines) brk() {
	line := t.cur.String()
	t.cur.Reset()
	if t.raw {
		line = strings.TrimRight(line, " \t\r")
	} else {
		line = collapse(line)
	}
	t.raw = false
	t.out = append(t.out, line)
}

// done joins the lines, dropping runs of blank ones. A rendered document is
// full of wrapper divs, and one blank line between paragraphs is the intent
// while six is an artifact of the markup.
func (t *textLines) done() string {
	t.brk()
	var b strings.Builder
	blank := false
	for _, line := range t.out {
		if line == "" {
			blank = true
			continue
		}
		if blank && b.Len() > 0 {
			b.WriteString("\n")
		}
		blank = false
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(line)
	}
	return b.String()
}

// RelTime returns the datetime attribute of the first <relative-time>
// descendant. The element's own text is never read: it is localised and
// relative, and parsing it would be a whole class of bug for no gain.
func RelTime(n *html.Node) string {
	rt := Find(n, Sel{Tag: "relative-time"})
	if rt == nil {
		rt = Find(n, Sel{Tag: "time-ago"})
	}
	if rt == nil {
		return ""
	}
	return Attr(rt, "datetime")
}

// readDOM is steps 5 to 8 of the extraction: meta tags, canonical, microdata,
// and the deferred fragments. It runs on every page because all four are cheap
// once the tree exists and all four are useful on both planes.
func (p *Page) readDOM() {
	doc := p.Doc()
	if doc == nil {
		return
	}
	p.Meta = map[string]string{}
	p.Microdata = map[string][]string{}

	Walk(doc, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch n.Data {
		case "meta":
			key := firstAttr(n, "property", "name")
			content := Attr(n, "content")
			if content == "" {
				return true
			}
			if strings.HasPrefix(key, "og:") || strings.HasPrefix(key, "twitter:") ||
				key == "description" || key == "octolytics-dimension-user_login" ||
				key == "octolytics-dimension-repository_id" || key == "route-pattern" {
				p.Meta[key] = content
			}
		case "link":
			if Attr(n, "rel") == "canonical" {
				p.Canonical = Attr(n, "href")
			}
		case "include-fragment", "turbo-frame":
			src := Attr(n, "src")
			// In-product messaging is growth tooling, not content, and it is on
			// nearly every page.
			if src != "" && !strings.Contains(src, "/in-product-messaging") {
				p.Fragments = appendUnique(p.Fragments, src)
			}
		case "title":
			if p.Title == "" {
				p.Title = Text(n)
			}
		}
		if v, ok := lookupAttr(n, "itemprop"); ok {
			// itemprop can name several properties at once, as in
			// itemprop="name codeRepository" on the repositories tab.
			for _, name := range strings.Fields(v) {
				p.Microdata[name] = append(p.Microdata[name], itemValue(n))
			}
		}
		return true
	})
	if len(p.Meta) == 0 {
		p.Meta = nil
	}
	if len(p.Microdata) == 0 {
		p.Microdata = nil
	}
}

// itemValue reads a microdata property the way the specification says to: the
// attribute that carries the machine-readable form when there is one, and the
// element text otherwise.
func itemValue(n *html.Node) string {
	switch n.Data {
	case "meta":
		return Attr(n, "content")
	case "a", "area", "link":
		return Attr(n, "href")
	case "img", "audio", "embed", "iframe", "source", "video":
		return Attr(n, "src")
	case "time":
		if v := Attr(n, "datetime"); v != "" {
			return v
		}
	case "data":
		if v := Attr(n, "value"); v != "" {
			return v
		}
	}
	return Text(n)
}

func firstAttr(n *html.Node, names ...string) string {
	for _, name := range names {
		if v, ok := lookupAttr(n, name); ok {
			return v
		}
	}
	return ""
}

func appendUnique(ss []string, s string) []string {
	for _, x := range ss {
		if x == s {
			return ss
		}
	}
	return append(ss, s)
}

// OuterHTML re-renders a subtree. It is how a README makes it into a record
// with its markup intact: the alternative is refetching the fragment, and the
// fragment is already here.
func OuterHTML(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	if err := html.Render(&b, n); err != nil {
		return ""
	}
	return b.String()
}
