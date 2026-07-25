package gh

import (
	"context"
	"fmt"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
)

// crawl.go walks the graph. The algorithm is a breadth-first frontier with a
// visited set and a hard budget, and that is all it should ever be: the
// interesting decisions here are about what not to follow and when to stop, not
// about traversal.
//
// The walk is sequential. Doc 04 section 5 gives the crawler a Concurrency
// field, and it would be a lie in this client: every request already queues
// through one rate limiter, so N workers would only queue N deep behind the same
// pacer while making the output order unpredictable. If the pacer ever grows a
// real parallel mode, this is the place to add the workers.

// CrawlOptions bounds a walk. The budgets are the point of the struct. A tool
// that can accidentally send a million requests at somebody else's servers
// should be hard to point that way by accident.
type CrawlOptions struct {
	Depth    int
	Follow   []string
	Kinds    []string
	MinTrust string
	Limit    int

	NodesOnly bool
	EdgesOnly bool
}

// CrawlSink receives what the walk finds. Emission is streaming: a crawl of a
// large organization must never need the whole graph in memory, and a crawl that
// is interrupted has already emitted everything it found.
type CrawlSink struct {
	Node func(*Node) error
	Edge func(*Edge) error
	Fact func(*Fact) error
}

// defaults fills in the spec's defaults. Depth 1 and the structural follow set
// are the safe walk: references, stars, follows, and the two dependency
// predicates fan out without bound, so each has to be asked for by name.
func (o *CrawlOptions) defaults() {
	if o.Depth <= 0 {
		o.Depth = 1
	}
	if o.MinTrust == "" {
		o.MinTrust = DefaultMinTrust
	}
	if len(o.Follow) == 0 {
		o.Follow = DefaultFollow
	}
}

// followSet accepts a predicate written either way, so --follow ownedBy and
// --follow gh:ownedBy mean the same thing. The word "all" turns the filter off.
func followSet(follow []string) map[string]bool {
	out := map[string]bool{}
	for _, f := range follow {
		for _, part := range strings.Split(f, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if part == "all" {
				return nil
			}
			out[strings.TrimPrefix(part, "gh:")] = true
		}
	}
	return out
}

// kindSet is the same idea for --kinds. An empty set expands every kind.
func kindSet(kinds []string) map[string]bool {
	out := map[string]bool{}
	for _, k := range kinds {
		for _, part := range strings.Split(k, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out[part] = true
			}
		}
	}
	return out
}

// splitURI takes a github:// URI apart. The crawler holds URIs rather than
// records, which is what keeps its memory proportional to the number of nodes
// seen and not to their size.
func splitURI(uri string) (kind, id string, ok bool) {
	k, i, _, err := parseURI(uri)
	if err != nil {
		return "", "", false
	}
	return k, i, true
}

// Crawl walks outward from a seed reference. Nodes, edges, and facts come out as
// they are discovered, and the walk stops cleanly at either bound and reports
// what it had rather than failing.
func (c *Client) Crawl(ctx context.Context, seed string, o CrawlOptions, sink CrawlSink) error {
	o.defaults()
	kind, id, err := Classify(seed)
	if err != nil {
		return err
	}
	allow := followSet(o.Follow)
	expand := kindSet(o.Kinds)

	type item struct {
		uri   string
		depth int
	}
	start := URI(kind, id)
	visited := map[string]bool{start: true}
	frontier := []item{{start, 0}}
	nodes := 0

	for len(frontier) > 0 {
		cur := frontier[0]
		frontier = frontier[1:]

		// The limit is checked before the fetch, which is what makes it a
		// budget rather than a suggestion.
		if o.Limit > 0 && nodes >= o.Limit {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return wrapNetwork("", err)
		}
		curKind, curID, ok := splitURI(cur.uri)
		if !ok {
			continue
		}
		rec, err := c.fetchOne(ctx, curKind, curID)
		if err != nil {
			// One unreachable node must not end a walk that has already
			// produced useful output. A deleted repository, a kind this tool
			// cannot dereference yet, and a page that has moved are all normal
			// mid-crawl, and the alternative is a two-hour walk that throws
			// away its results on the last hop.
			if softSkip(err) {
				continue
			}
			return err
		}
		node, edges, facts := Extract(rec)
		if node.URI == "" {
			continue
		}
		nodes++
		if !o.EdgesOnly && sink.Node != nil {
			n := node
			if err := sink.Node(&n); err != nil {
				return err
			}
		}
		edges = FilterTrust(edges, o.MinTrust)
		if !o.NodesOnly {
			for i := range edges {
				if sink.Edge != nil {
					e := edges[i]
					if err := sink.Edge(&e); err != nil {
						return err
					}
				}
			}
			for i := range facts {
				if sink.Fact != nil {
					f := facts[i]
					if err := sink.Fact(&f); err != nil {
						return err
					}
				}
			}
		}
		if cur.depth >= o.Depth {
			continue
		}
		for _, e := range edges {
			// A language or a licence is a bare string with no page behind it,
			// so it is an edge but never a target.
			if !strings.HasPrefix(e.Object, Scheme+"://") {
				continue
			}
			if len(allow) > 0 && !allow[e.Predicate] {
				continue
			}
			objKind, _, ok := splitURI(e.Object)
			if !ok {
				continue
			}
			if len(expand) > 0 && !expand[objKind] {
				continue
			}
			// Cycles are normal on this graph. A fork points at its parent and
			// the parent's fork list points back, and the visited set is the
			// only defence that needs.
			if visited[e.Object] {
				continue
			}
			visited[e.Object] = true
			frontier = append(frontier, item{e.Object, cur.depth + 1})
		}
	}
	return nil
}

// softSkip reports whether an error is one node's problem rather than the
// walk's. Not found, needs a login, and a kind that has no reader yet are all
// "skip this one", and anything else stops the crawl.
func softSkip(err error) bool {
	switch errs.KindOf(err) {
	case errs.KindNotFound, errs.KindNeedAuth, errs.KindUnsupported:
		return true
	default:
		return false
	}
}

// CrawlPlan is what --dry-run answers with. It is a record rather than a line
// on stderr so the answer goes through the same renderer, formats, and pipes as
// every other command, and so a script can size a walk without reading prose.
type CrawlPlan struct {
	Base

	Seed  string `json:"seed"  table:"seed"`
	Depth int    `json:"depth" table:"depth"`
	Nodes int    `json:"nodes" table:"nodes"`

	Note string `json:"note" table:"note"`
}

// Estimate reads the seed and reports what one more level would cost. It is
// deliberately a lower bound and the note says so: the first level is countable
// because the seed's edges are in hand, and everything past it depends on a
// branching factor that cannot be seen from here without doing the walk.
func (c *Client) Estimate(ctx context.Context, seed string, o CrawlOptions) (*CrawlPlan, error) {
	o.defaults()
	kind, id, err := Classify(seed)
	if err != nil {
		return nil, err
	}
	rec, err := c.fetchOne(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	_, edges, _ := Extract(rec)
	edges = FilterTrust(edges, o.MinTrust)
	allow := followSet(o.Follow)
	expand := kindSet(o.Kinds)

	seen := map[string]bool{URI(kind, id): true}
	next := 0
	for _, e := range edges {
		if !strings.HasPrefix(e.Object, Scheme+"://") || seen[e.Object] {
			continue
		}
		if len(allow) > 0 && !allow[e.Predicate] {
			continue
		}
		objKind, _, ok := splitURI(e.Object)
		if !ok || (len(expand) > 0 && !expand[objKind]) {
			continue
		}
		seen[e.Object] = true
		next++
	}
	nodes := 1 + next
	if o.Limit > 0 && nodes > o.Limit {
		nodes = o.Limit
	}
	note := fmt.Sprintf("at least %d nodes through depth 1, about one request each", nodes)
	if o.Depth > 1 {
		note += fmt.Sprintf(", and more at depth %d depending on how the next level branches", o.Depth)
	}
	plan := &CrawlPlan{Seed: seed, Depth: o.Depth, Nodes: nodes, Note: note}
	plan.setIdentity(kind, id)
	return plan, nil
}

// GraphOf builds the node, edges, and facts for one entity. `github graph`,
// `github edges`, and `github rdf` all call it, so the three never disagree
// about what an entity's edges are.
func (c *Client) GraphOf(ctx context.Context, kind, id string) (*Graph, error) {
	rec, err := c.fetchOne(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	g := &Graph{}
	g.Add(rec)
	return g, nil
}

// GraphOfRef is GraphOf for a reference that has not been classified yet, and it
// reports the kind it turned out to be so a caller can say what it read.
func (c *Client) GraphOfRef(ctx context.Context, ref string) (string, string, *Graph, error) {
	kind, id, err := Classify(ref)
	if err != nil {
		return "", "", nil, err
	}
	g, err := c.GraphOf(ctx, kind, id)
	if err != nil {
		return "", "", nil, err
	}
	return kind, id, g, nil
}
