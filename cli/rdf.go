package cli

import (
	"bufio"
	"context"
	"os"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/github-cli/gh"
)

// rdf.go holds the linked-data output. It is a byte-plane command for the same
// reason cat is: N-Triples and Turtle are not records, they are a serialisation
// with their own rules, and putting them through the record renderer would
// produce something that is neither.

type rdfCmd struct {
	format   string
	graph    string
	depth    int
	follow   []string
	minTrust string
	limit    int
}

func newRDFCmd() kit.Command {
	c := &rdfCmd{}
	return kit.Command{
		Use:   "rdf <ref>",
		Short: "Write an entity as RDF triples",
		Long: "rdf serialises one entity, its edges, and its literals. N-Triples is the\n" +
			"default because it streams line by line, so a deep walk never needs the whole\n" +
			"graph in memory; Turtle and JSON-LD do need it and are slower on large graphs\n" +
			"for that reason.\n\n" +
			"Subjects are the github.com URLs rather than the github:// URIs, so the output\n" +
			"is dereferenceable by anything on the web. The URI is kept as a gh:uri\n" +
			"literal, so nothing is lost.\n\n" +
			"With --depth it walks first and serialises the whole result, which is how you\n" +
			"get a loadable dataset rather than one subject.",
		Group: "graph",
		Args:  kit.ExactArgs(1),
		Flags: c.flags,
		Run:   c.run,
	}
}

func (c *rdfCmd) flags(f *kit.FlagSet) {
	f.StringVar(&c.format, "format", gh.FormatNT, "nt, ttl, jsonld, or nq")
	f.StringVar(&c.graph, "graph", "", "the named graph for nq output (default: the entity URL)")
	f.IntVar(&c.depth, "depth", 0, "walk this many edges out before serialising")
	f.StringSliceVar(&c.follow, "follow", nil, "predicates to follow when walking")
	f.StringVar(&c.minTrust, "min-trust", gh.DefaultMinTrust, "drop edges below this rule")
	f.IntVar(&c.limit, "limit", 0, "stop a walk after this many nodes")
}

func (c *rdfCmd) run(ctx context.Context, args []string) error {
	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}
	kind, id, g, err := buildGraph(ctx, cl, args[0], graphWalk{
		depth:    c.depth,
		follow:   c.follow,
		minTrust: c.minTrust,
		limit:    c.limit,
	})
	if err != nil {
		return err
	}
	gh.SortEdges(g.Edges)

	w := bufio.NewWriter(os.Stdout)
	defer func() { _ = w.Flush() }()

	graph := c.graph
	if graph == "" {
		graph, _ = gh.Locate(kind, id)
	}
	return gh.WriteRDF(w, g, gh.RDFOptions{Format: c.format, Graph: graph})
}

// graphWalk is the set of knobs rdf and export share. They are the same walk
// with a different writer on the end, so the flags are declared twice and read
// once.
type graphWalk struct {
	depth    int
	follow   []string
	minTrust string
	limit    int
}

// buildGraph resolves a reference and returns either the one entity or the whole
// walk, depending on depth. Both come back as a Graph, so the serialisers never
// need to know which it was.
//
// It holds the result in memory, which is the price of the formats that cannot
// stream. `github crawl` is the streaming answer for a walk too big for this.
func buildGraph(ctx context.Context, cl *gh.Client, ref string, o graphWalk) (string, string, *gh.Graph, error) {
	kind, id, g, err := cl.GraphOfRef(ctx, ref)
	if err != nil {
		return "", "", nil, err
	}
	if o.depth <= 0 {
		return kind, id, g, nil
	}
	g = &gh.Graph{}
	err = cl.Crawl(ctx, gh.URI(kind, id), gh.CrawlOptions{
		Depth:    o.depth,
		Follow:   o.follow,
		MinTrust: o.minTrust,
		Limit:    o.limit,
	}, gh.CrawlSink{
		Node: func(n *gh.Node) error { g.AddNode(*n); return nil },
		Edge: func(e *gh.Edge) error { g.Edges = append(g.Edges, *e); return nil },
		Fact: func(f *gh.Fact) error { g.Facts = append(g.Facts, *f); return nil },
	})
	if err != nil {
		return "", "", nil, err
	}
	return kind, id, g, nil
}
