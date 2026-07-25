// Package page turns a github.com HTML document into structured data.
//
// github.com is two applications sharing a domain. The React half ships its
// route props as JSON inside a script tag, and the Rails half ships schema.org
// microdata. Both are structured, which means most of what looks like scraping
// is really JSON decoding with an HTML document as the envelope.
//
// Pulling text out of rendered markup is the third choice here, not the first.
// Where it is unavoidable, the selector lives in selectors.go with the date it
// was last checked against a live page, so a break is a one-file diff.
package page

import (
	"encoding/json"
	"strings"

	"golang.org/x/net/html"
)

// Plane says which of the two applications rendered a page. It is worth
// knowing because it decides where the data is: React pages carry an app
// payload and Rails pages carry microdata.
type Plane string

const (
	PlaneReact Plane = "react"
	PlaneRails Plane = "rails"
)

// Page is the whole HTML plane of one document in one struct. Every
// page-derived record is built from a Page and never from a raw string, which
// is what makes `github page` possible: it prints this, and every record is a
// projection of it.
type Page struct {
	URL       string `json:"url"`
	Canonical string `json:"canonical,omitempty"`
	Title     string `json:"title,omitempty"`
	Plane     Plane  `json:"plane"`

	// Payload is the React route props, the union of every route object the
	// server sent for this page.
	Payload map[string]json.RawMessage `json:"payload,omitempty"`

	// Queries holds the Relay results the server preloaded, keyed by query
	// name. This is a GraphQL response without a GraphQL token, and on issue
	// and pull request pages it is where everything lives.
	Queries map[string]json.RawMessage `json:"queries,omitempty"`

	// StructuredData is payload.structured_data, GitHub's own schema.org view
	// of the page. Its url field is wrong, see Canonical.
	StructuredData json.RawMessage `json:"structured_data,omitempty"`

	// LinkedData is every <script type="application/ld+json"> block, verbatim.
	LinkedData []json.RawMessage `json:"linked_data,omitempty"`

	// Partials are the react-partial blocks worth keeping. Most are chrome
	// (header, footer, command palette) and are dropped by size and key count.
	Partials map[string]json.RawMessage `json:"partials,omitempty"`

	// Meta is og: and twitter: content, keyed by the full property name.
	Meta map[string]string `json:"meta,omitempty"`

	// Microdata is a multimap of itemprop name to the values found, which is
	// how the Rails pages publish their fields.
	Microdata map[string][]string `json:"microdata,omitempty"`

	// Fragments are the deferred loads the page names for itself, from
	// <include-fragment src> and <turbo-frame src>. Discovering them by
	// scanning rather than by hardcoded path means a new one shows up in
	// `github page` before any record models it.
	Fragments []string `json:"fragments,omitempty"`

	Bytes int `json:"bytes"`

	// HTML is the original document, kept for the selector passes. It is not
	// serialised: printing 300 KB of markup inside a JSON record helps nobody.
	HTML []byte `json:"-"`

	doc *html.Node
}

// Extract runs the nine-step read: find the script blocks, classify them,
// unpack the app payload and its preloaded queries, then collect the linked
// data, the meta tags, the microdata, the canonical URL, and the deferred
// fragments.
//
// It never returns an error. A page that carries nothing this understands is a
// Page with nothing in it, and deciding whether that is a failure belongs to
// the caller who knows what it was looking for.
func Extract(url string, doc []byte) *Page {
	p := &Page{URL: url, Bytes: len(doc), HTML: doc, Plane: PlaneRails}

	for _, b := range scanJSONScripts(doc) {
		switch {
		case b.dataTarget == "react-app.embeddedData":
			p.Plane = PlaneReact
			p.readAppData(b.body)
		case b.dataTarget == "react-partial.embeddedData":
			p.readPartial(b.body)
		case b.id == "client-env":
			// Feature-flag plumbing. Never useful.
		}
	}
	for _, b := range scanLDJSON(doc) {
		if json.Valid(b) {
			p.LinkedData = append(p.LinkedData, json.RawMessage(b))
		}
	}
	p.readDOM()
	return p
}

// appData is the top level of the react-app block.
type appData struct {
	Payload map[string]json.RawMessage `json:"payload"`
	Title   string                     `json:"title"`
}

func (p *Page) readAppData(body []byte) {
	var d appData
	if err := json.Unmarshal(body, &d); err != nil {
		return
	}
	if p.Payload == nil {
		p.Payload = map[string]json.RawMessage{}
	}
	for k, v := range d.Payload {
		p.Payload[k] = v
	}
	if d.Title != "" {
		p.Title = d.Title
	}
	if sd, ok := p.Payload["structured_data"]; ok {
		p.StructuredData = sd
	}
	p.readPreloadedQueries()
}

// readPreloadedQueries unpacks the Relay results the server ran for the page.
// The shape is an array of {queryName, variables, result:{data}}, and the
// result data is what a GraphQL client would have received.
func (p *Page) readPreloadedQueries() {
	raw, ok := p.Payload["preloadedQueries"]
	if !ok {
		return
	}
	var qs []struct {
		QueryName string `json:"queryName"`
		Result    struct {
			Data json.RawMessage `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &qs); err != nil {
		return
	}
	if p.Queries == nil {
		p.Queries = map[string]json.RawMessage{}
	}
	for _, q := range qs {
		if q.QueryName != "" && len(q.Result.Data) > 0 {
			p.Queries[q.QueryName] = q.Result.Data
		}
	}
}

// readPartial keeps a react-partial block only when it looks like content. The
// chrome partials (header, footer, notification indicator, command palette)
// are small and have few keys, and the count of them varies page to page, so
// they are filtered by shape rather than by position.
func (p *Page) readPartial(body []byte) {
	if len(body) < 512 {
		return
	}
	var d struct {
		Props map[string]json.RawMessage `json:"props"`
	}
	if err := json.Unmarshal(body, &d); err != nil || len(d.Props) <= 3 {
		return
	}
	if p.Partials == nil {
		p.Partials = map[string]json.RawMessage{}
	}
	// Key by the first prop name, which is stable enough to tell two content
	// partials apart and costs nothing when it is not.
	key := partialKey(d.Props)
	p.Partials[key] = json.RawMessage(body)
}

func partialKey(props map[string]json.RawMessage) string {
	best := ""
	for k := range props {
		if best == "" || k < best {
			best = k
		}
	}
	if best == "" {
		return "partial"
	}
	return best
}

// Query returns one preloaded Relay result by name, or by suffix when the
// caller does not want to spell out the whole generated name.
func (p *Page) Query(name string) (json.RawMessage, bool) {
	if v, ok := p.Queries[name]; ok {
		return v, true
	}
	for k, v := range p.Queries {
		if strings.Contains(k, name) {
			return v, true
		}
	}
	return nil, false
}

// FirstQuery returns the first preloaded result, which on the thread pages is
// the one that matters. Map order is random, so the names are sorted first:
// a decoder that works on one run and not the next is worse than no decoder.
func (p *Page) FirstQuery() (string, json.RawMessage, bool) {
	best := ""
	for k := range p.Queries {
		if best == "" || k < best {
			best = k
		}
	}
	if best == "" {
		return "", nil, false
	}
	return best, p.Queries[best], true
}

// Route returns one React route object from the payload. Route props are a
// delta against the mounted layout, so a route that was present on one fetch
// can be absent on the next; every caller checks the second return value.
func (p *Page) Route(name string) (json.RawMessage, bool) {
	v, ok := p.Payload[name]
	return v, ok
}

// MetaContent returns an og: or twitter: value.
func (p *Page) MetaContent(name string) string { return p.Meta[name] }

// Prop returns the first value of a microdata itemprop.
func (p *Page) Prop(name string) string {
	if v := p.Microdata[name]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// Props returns every value of a microdata itemprop.
func (p *Page) Props(name string) []string { return p.Microdata[name] }
