package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
	"github.com/tamnd/github-cli/gh"
	"github.com/tamnd/github-cli/pkg/page"
)

// page.go is the 1:1 view. Everything else in this tool decides what matters on
// a page and throws the rest away; this command throws nothing away, which makes
// it three things at once.
//
// It is the escape hatch. A consumer who wants a field no record models yet can
// have it today instead of waiting for a release.
//
// It is the debugging tool. When a record comes back thin the first question is
// always whether the data was missing from the page or dropped by the decoder,
// and this is the only way to tell the two apart.
//
// It is how a recorded fixture is read back, since a fixture is the bytes and
// nothing else.
//
// It is a byte-plane command because its output is one document, not a stream of
// records, and pretending otherwise would put a table renderer in front of a
// GraphQL response.

type pageCmd struct {
	section string
	query   string
	raw     bool
	compact bool
}

func newPageCmd() kit.Command {
	c := &pageCmd{}
	return kit.Command{
		Use:   "page <ref>",
		Short: "Print everything a page carries, organised",
		Long: "page fetches one page and prints the whole extraction as JSON: the React\n" +
			"route payload, the preloaded Relay queries, GitHub's own schema.org block,\n" +
			"the ld+json, the og: and twitter: meta, the microdata, and the deferred\n" +
			"fragments the page names for itself.\n\n" +
			"--section narrows it to one of payload, queries, structured_data,\n" +
			"linked_data, partials, meta, microdata, or fragments. --query prints one\n" +
			"preloaded query result by name, which is where issue and pull request pages\n" +
			"keep everything. With no argument, --query lists the names.\n\n" +
			"--raw writes the original markup instead, which is what you want when the\n" +
			"question is about the HTML rather than about the data in it.",
		Group: "meta",
		Args:  kit.ExactArgs(1),
		Flags: c.flags,
		Run:   c.run,
	}
}

func (c *pageCmd) flags(f *kit.FlagSet) {
	f.StringVar(&c.section, "section", "", "print one section only")
	f.StringVar(&c.query, "query", "", "print one preloaded query by name (empty lists them)")
	f.BoolVar(&c.raw, "raw", false, "print the original markup instead of the extraction")
	f.BoolVar(&c.compact, "compact", false, "one line of JSON rather than indented")
}

func (c *pageCmd) run(ctx context.Context, args []string) error {
	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}
	url, err := pageURL(args[0])
	if err != nil {
		return err
	}
	p, err := cl.Page(ctx, url)
	if err != nil {
		return err
	}

	w := bufio.NewWriter(os.Stdout)
	defer func() { _ = w.Flush() }()

	if c.raw {
		_, err := w.Write(p.HTML)
		return err
	}

	enc := json.NewEncoder(w)
	if !c.compact {
		enc.SetIndent("", "  ")
	}

	if c.query != "" {
		q, ok := p.Queries[c.query]
		if !ok {
			// The names are the useful half of this failure. Query names are
			// GitHub's internal Relay identifiers, nobody knows them by heart,
			// and a bare "not found" would send the reader off to dump the
			// whole queries section to find out what to ask for.
			// Both messages lead with a word rather than the URL because the
			// error renderer capitalises what it starts with, and a
			// title-cased URL reads as a typo.
			if len(p.Queries) == 0 {
				// Worth saying separately. Most pages preload nothing, so
				// listing the names it has would be an empty list, and an empty
				// list reads like the lookup broke rather than like the page
				// carries no queries at all.
				return errs.NotFound("no preloaded queries on %s at all; that is normal, only a few page kinds have them", url)
			}
			return errs.NotFound("no query named %q on %s; it has %s",
				c.query, url, strings.Join(queryNames(p.Queries), ", "))
		}
		return enc.Encode(q)
	}
	if c.section != "" {
		v, err := section(p, c.section)
		if err != nil {
			return err
		}
		return enc.Encode(v)
	}
	return enc.Encode(p)
}

// pageURL turns anything a person might paste into the page to fetch. A full
// URL is taken as given, including the parts of the site that name no entity,
// like /trending and /explore, because the debugging tool is least useful on
// exactly the pages the model does not cover yet.
func pageURL(ref string) (string, error) {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref, nil
	}
	kind, id, err := gh.Classify(ref)
	if err != nil {
		return "", err
	}
	return gh.Locate(kind, id)
}

func section(p *page.Page, name string) (any, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "payload":
		return p.Payload, nil
	case "queries":
		return p.Queries, nil
	case "structured_data", "structured-data", "structured":
		return p.StructuredData, nil
	case "linked_data", "linked-data", "ld", "ld+json":
		return p.LinkedData, nil
	case "partials":
		return p.Partials, nil
	case "meta":
		return p.Meta, nil
	case "microdata":
		return p.Microdata, nil
	case "fragments":
		return p.Fragments, nil
	default:
		return nil, errs.Usage("unknown --section %q: payload, queries, structured_data, linked_data, partials, meta, microdata, or fragments", name)
	}
}

// queryNames is sorted because the map order would otherwise change between two
// runs against the same bytes, and this output gets diffed.
func queryNames(q map[string]json.RawMessage) []string {
	out := make([]string, 0, len(q))
	for k := range q {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
