package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
	"github.com/tamnd/github-cli/gh"
)

// export.go writes a whole graph to one file. Everything it does can be had by
// redirecting `github crawl` or `github rdf`, and it exists because what people
// want at the end of a walk is one file they can load somewhere else, named once
// rather than assembled out of two commands and a shell operator.

type exportCmd struct {
	format   string
	out      string
	depth    int
	follow   []string
	minTrust string
	limit    int
}

func newExportCmd() kit.Command {
	c := &exportCmd{}
	return kit.Command{
		Use:   "export <ref>",
		Short: "Write a whole graph to one file",
		Long: "export walks from the seed and writes the result in one go. The formats are\n" +
			"jsonl (one node, edge, or fact per line), json (a single object), and the four\n" +
			"RDF serialisations nt, ttl, jsonld, and nq.\n\n" +
			"Without --out it writes to stdout, which makes it a drop-in for a pipeline.",
		Group: "graph",
		Args:  kit.ExactArgs(1),
		Flags: c.flags,
		Run:   c.run,
	}
}

func (c *exportCmd) flags(f *kit.FlagSet) {
	f.StringVar(&c.format, "format", "jsonl", "jsonl, json, nt, ttl, jsonld, or nq")
	f.StringVarP(&c.out, "out", "O", "", "write here instead of stdout")
	f.IntVar(&c.depth, "depth", 1, "walk this many edges out")
	f.StringSliceVar(&c.follow, "follow", nil, "predicates to follow (default: the structural ones)")
	f.StringVar(&c.minTrust, "min-trust", gh.DefaultMinTrust, "drop edges below this rule")
	f.IntVar(&c.limit, "limit", 0, "stop after this many nodes")
}

func (c *exportCmd) run(ctx context.Context, args []string) error {
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

	out := io.Writer(os.Stdout)
	if c.out != "" {
		f, err := os.Create(c.out)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		out = f
	}
	w := bufio.NewWriter(out)
	defer func() { _ = w.Flush() }()

	switch c.format {
	case "jsonl":
		return writeJSONL(w, g)
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(g)
	case gh.FormatNT, gh.FormatTurtle, gh.FormatJSONLD, gh.FormatNQuads:
		graph, _ := gh.Locate(kind, id)
		return gh.WriteRDF(w, g, gh.RDFOptions{Format: c.format, Graph: graph})
	default:
		return errs.Usage("unknown --format %q", c.format)
	}
}

// writeJSONL puts the nodes first and everything that points at them after, so a
// reader building an index in one pass never sees an edge before both of its
// ends.
func writeJSONL(w io.Writer, g *gh.Graph) error {
	enc := json.NewEncoder(w)
	for i := range g.Nodes {
		if err := enc.Encode(&g.Nodes[i]); err != nil {
			return err
		}
	}
	for i := range g.Edges {
		if err := enc.Encode(&g.Edges[i]); err != nil {
			return err
		}
	}
	for i := range g.Facts {
		if err := enc.Encode(&g.Facts[i]); err != nil {
			return err
		}
	}
	return nil
}
