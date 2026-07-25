package gh

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

// base.go holds what every record has in common and the guard that keeps the
// records honest.
//
// The guard is decodeExtra. Every decoder runs it, and it puts anything the
// struct did not claim into Extra. The scenario suite then asserts Extra is
// empty. The effect is that the day GitHub adds a field, a test fails and
// names it, instead of the field being silently dropped for a year.

// Base is embedded in every record. Kind and ID are the identity, URI and URL
// are the two addresses, Sources records which pages were read, Via records
// which extraction tier produced a field, and Extra is the data-loss guard.
//
// Via is deliberately not part of Extra. Extra means "GitHub sent this and
// nobody modelled it" and the suite asserts it is empty; provenance we wrote
// ourselves has no business making that assertion fail.
type Base struct {
	Kind    string            `json:"kind"              table:"kind"`
	ID      string            `json:"id"                table:"id" kit:"id"`
	URI     string            `json:"uri,omitempty"     table:"-"`
	URL     string            `json:"url,omitempty"     table:"-"`
	Sources []string          `json:"sources,omitempty" table:"-"`
	Via     map[string]string `json:"via,omitempty"     table:"-"`
	Extra   json.RawMessage   `json:"extra,omitempty"   table:"-"`
}

// setIdentity fills Kind, ID, URI, and URL from a kind and an id. Every
// constructor calls it, so no record can exist with a URI that disagrees with
// its id.
func (b *Base) setIdentity(kind, id string) {
	b.Kind = kind
	b.ID = id
	b.URI = URI(kind, id)
	if u, err := Locate(kind, id); err == nil {
		b.URL = u
	}
}

// addSource records a URL a field came from. Duplicates are dropped and the
// order is the order they were read, which makes the field useful for
// debugging a merge as well as for provenance.
func (b *Base) addSource(urls ...string) {
	for _, u := range urls {
		if u == "" {
			continue
		}
		if !contains(b.Sources, u) {
			b.Sources = append(b.Sources, u)
		}
	}
}

// addExtra files a block of unmodelled keys under the name of the payload block
// they came from. Namespacing matters: "twelve unknown keys" is not actionable,
// "twelve unknown keys in sidebarAbout" is.
func (b *Base) addExtra(name string, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	m := map[string]json.RawMessage{}
	if len(b.Extra) > 0 {
		if err := json.Unmarshal(b.Extra, &m); err != nil {
			return
		}
	}
	m[name] = raw
	out, err := json.Marshal(m)
	if err != nil {
		return
	}
	b.Extra = out
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// Actor is a person or an organization as it appears inside another record: on
// a commit, an issue, a release. It is deliberately small. The full account is
// a separate read, and inlining it would turn one request into hundreds.
// Both id forms are kept. The numeric one is what avatar URLs and the older
// links use, the base64 global one is what Relay results carry, and a joiner
// downstream will have one or the other and not both.
type Actor struct {
	Login      string `json:"login"                 table:"login"`
	Name       string `json:"name,omitempty"        table:"name"`
	Type       string `json:"type,omitempty"        table:"-"`
	NodeID     string `json:"node_id,omitempty"     table:"-"`
	DatabaseID *int   `json:"database_id,omitempty" table:"-"`
	AvatarURL  string `json:"avatar_url,omitempty"  table:"-"`
	URL        string `json:"url,omitempty"         table:"-"`
	URI        string `json:"uri,omitempty"         table:"-"`
}

// actor builds an Actor from a login, filling the derived fields. An empty
// login gives an empty Actor rather than one with a URL to nowhere.
func actor(login string) Actor {
	if login == "" {
		return Actor{}
	}
	return Actor{
		Login: login,
		URL:   BaseURL + "/" + login,
		URI:   URI(KindUser, login),
	}
}

// hrefPath reduces a link to its site-relative path, with no leading slash and
// no query or fragment.
//
// It exists because GitHub writes the same link two ways on two templates:
// "/BagToad" on a release page and "https://github.com/BagToad" on the release
// list. A decoder that trims a leading slash and stops there works on one of
// them and produces a login with a whole URL inside it on the other.
func hrefPath(href string) string {
	s := strings.TrimSpace(href)
	if i := strings.Index(s, "://"); i >= 0 {
		_, rest, ok := strings.Cut(s[i+3:], "/")
		if !ok {
			return ""
		}
		s = rest
	}
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	return strings.Trim(s, "/")
}

// actorFromHref builds an Actor from a profile link however the template wrote
// it. A link with more than one path segment is not a profile, so it gives an
// empty result rather than a login with a slash in it.
func actorFromHref(href string) Actor {
	p := hrefPath(href)
	if p == "" || strings.Contains(p, "/") {
		return Actor{}
	}
	return actor(p)
}

// --- the data-loss guard ---

// decodeExtra returns the keys of raw that v did not claim, minus the keys in
// skip. It is what stands between this tool and silently dropping a field
// GitHub added last Tuesday.
//
// skip lists are explicit, short, and commented one entry at a time. A key
// dropped without a reason is a bug waiting to be found by someone six months
// from now, so the convention is that every skip list entry says why.
func decodeExtra(raw json.RawMessage, v any, skip ...string) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	for _, k := range claimedKeys(reflect.TypeOf(v)) {
		delete(m, k)
	}
	for _, k := range skip {
		if strings.HasSuffix(k, "*") {
			prefix := strings.TrimSuffix(k, "*")
			for key := range m {
				if strings.HasPrefix(key, prefix) {
					delete(m, key)
				}
			}
			continue
		}
		delete(m, k)
	}
	// Drop the keys whose value is null or an empty container. A key GitHub
	// sends as null carries no information, and reporting it as unmodelled
	// data would make Extra noisy enough that nobody would read it.
	for k, val := range m {
		if isEmptyJSON(val) {
			delete(m, k)
		}
	}
	if len(m) == 0 {
		return nil
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return out
}

func isEmptyJSON(v json.RawMessage) bool {
	s := strings.TrimSpace(string(v))
	return s == "" || s == "null" || s == "{}" || s == "[]" || s == `""`
}

// claimedKeys walks a struct's json tags, following embedded structs, and
// returns every key the type would decode.
var claimedCache sync.Map // reflect.Type -> []string

func claimedKeys(t reflect.Type) []string {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	if v, ok := claimedCache.Load(t); ok {
		return v.([]string)
	}
	seen := map[string]bool{}
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			tag := f.Tag.Get("json")
			name, _, _ := strings.Cut(tag, ",")
			if name == "-" {
				continue
			}
			if f.Anonymous && name == "" {
				ft := f.Type
				for ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct {
					walk(ft)
					continue
				}
			}
			if name == "" {
				name = f.Name
			}
			seen[name] = true
		}
	}
	walk(t)
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	claimedCache.Store(t, keys)
	return keys
}

// --- small shared helpers ---

// parseTime accepts the time formats GitHub uses across its surfaces: RFC 3339
// with a zone, RFC 3339 in UTC with a Z, the bare date on a commit calendar,
// and the space-separated form the activity Atom feed puts in its published
// element, which is the only surface that does not use RFC 3339.
func parseTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z0700",
		"2006-01-02 15:04:05 MST",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			u := t.UTC()
			return &u
		}
	}
	return nil
}

func intp(v int) *int { return &v }

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
