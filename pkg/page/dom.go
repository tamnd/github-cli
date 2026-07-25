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
