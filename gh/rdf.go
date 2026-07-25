package gh

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// rdf.go serialises the graph. The graph plane is already triples, so this is a
// serialisation and not a transformation.
//
// The one real decision here is the schema.org alignment. A consumer that has
// never heard of gh:forkOf still understands schema:author and schema:isPartOf,
// and using the standard term where one exists is what lets a github export and
// an hf export join in the same triple store.
//
// Subject IRIs are the canonical github.com URLs rather than the github:// URIs.
// A triple whose subject is https://github.com/golang/go is dereferenceable by
// anything on the web; one whose subject is github://repo/golang/go is
// dereferenceable only by this tool. The github:// form survives as a gh:uri
// literal so nothing is lost.

// The namespaces.
const (
	NSSchema = "https://schema.org/"
	NSGH     = "https://github.com/ns#"
	NSGHR    = "https://github.com/"
	NSRdf    = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	NSRdfs   = "http://www.w3.org/2000/01/rdf-schema#"
	NSXsd    = "http://www.w3.org/2001/XMLSchema#"
	NSDoap   = "http://usefulinc.com/ns/doap#"
	NSFoaf   = "http://xmlns.com/foaf/0.1/"
)

var rdfPrefixes = [][2]string{
	{"schema", NSSchema},
	{"gh", NSGH},
	{"ghr", NSGHR},
	{"rdf", NSRdf},
	{"rdfs", NSRdfs},
	{"xsd", NSXsd},
	{"doap", NSDoap},
	{"foaf", NSFoaf},
}

// The datatypes a Fact can carry. They are CURIEs so a Fact reads the same in
// every serialisation.
const (
	TypeInteger  = "xsd:integer"
	TypeDecimal  = "xsd:decimal"
	TypeBoolean  = "xsd:boolean"
	TypeDateTime = "xsd:dateTime"
)

// rdfTypes is the class mapping. Marking an issue as schema:DiscussionForumPosting
// is not this tool's invention: it is what GitHub's own structured_data block
// says, and following the publisher's vocabulary for its own content is the
// whole point.
var rdfTypes = map[string][]string{
	KindRepo:       {"schema:SoftwareSourceCode", "doap:Project"},
	KindUser:       {"schema:Person", "foaf:Person"},
	KindOrg:        {"schema:Organization"},
	KindIssue:      {"gh:Issue", "schema:DiscussionForumPosting"},
	KindPR:         {"gh:PullRequest", "schema:DiscussionForumPosting"},
	KindDiscussion: {"schema:DiscussionForumPosting"},
	KindCommit:     {"gh:Commit"},
	KindRelease:    {"schema:SoftwareApplication"},
	KindTag:        {"gh:Ref"},
	KindBranch:     {"gh:Ref"},
	KindFile:       {"schema:MediaObject"},
	KindTree:       {"schema:MediaObject"},
	KindTopic:      {"schema:DefinedTerm"},
	KindLabel:      {"schema:DefinedTerm"},
	KindPackage:    {"schema:SoftwareApplication"},
	KindGist:       {"schema:SoftwareSourceCode"},
	KindAction:     {"schema:SoftwareApplication"},
	KindWiki:       {"schema:Article"},
}

// rdfPredicates maps this tool's vocabulary onto RDF terms. A predicate with no
// entry is emitted in the gh: namespace under its own name, which is what makes
// adding a predicate to graph.go a one-line change rather than two.
var rdfPredicates = map[string]string{
	PredOwnedBy:       "schema:author",
	PredAuthoredBy:    "schema:author",
	PredPartOf:        "schema:isPartOf",
	PredMemberOf:      "schema:memberOf",
	PredHasTopic:      "schema:keywords",
	PredHasLabel:      "schema:keywords",
	PredWrittenIn:     "schema:programmingLanguage",
	PredLicensedUnder: "schema:license",
	PredReferences:    "schema:citation",
	PredFollows:       "schema:follows",
}

// rdfFacts maps the literal predicates. The counts get gh: terms because
// schema.org has no stargazer count, and the dates get the standard ones
// because it does.
var rdfFacts = map[string]string{
	FactName:        "schema:name",
	FactDescription: "schema:description",
	FactHomepage:    "schema:url",
	FactCreated:     "schema:dateCreated",
	FactUpdated:     "schema:dateModified",
	FactStars:       "gh:stargazerCount",
	FactForks:       "gh:forkCount",
	FactWatchers:    "gh:watcherCount",
	FactCommits:     "gh:commitCount",
	FactURI:         "gh:uri",
	FactAvatar:      "schema:image",
}

// The output formats.
const (
	FormatNT     = "nt"
	FormatNQuads = "nq"
	FormatTurtle = "ttl"
	FormatJSONLD = "jsonld"
)

// RDFFormats is the accepted set, for help text and validation.
var RDFFormats = []string{FormatNT, FormatNQuads, FormatTurtle, FormatJSONLD}

// RDFOptions controls a serialisation.
type RDFOptions struct {
	Format string
	// Graph is the fourth position for N-Quads. Putting the source URL there
	// means the provenance survives into the RDF and a quad store can answer
	// which page told us this.
	Graph string
}

// WriteRDF serialises a whole graph. N-Triples is the default because it
// streams: nt and nq write a line per triple as it is produced, ttl buffers one
// subject at a time, and jsonld buffers the lot.
func WriteRDF(w io.Writer, g *Graph, o RDFOptions) error {
	switch o.Format {
	case "", FormatNT:
		return writeTriples(w, g, "")
	case FormatNQuads:
		return writeTriples(w, g, o.Graph)
	case FormatTurtle:
		return writeTurtle(w, g)
	case FormatJSONLD:
		return writeJSONLD(w, g)
	default:
		return fmt.Errorf("unknown rdf format %q, want one of %s", o.Format, strings.Join(RDFFormats, ", "))
	}
}

// --- streaming ---

// RDFWriter is the streaming form. A crawl hands it nodes, edges, and facts as
// it finds them and it writes lines, so `github export --depth 3 --format nt`
// over a large organization never holds the graph in memory. The buffered
// formats are handled by collecting into a Graph and calling WriteRDF, and this
// type reports which is which through Streams.
type RDFWriter struct {
	w      io.Writer
	suffix string
}

// NewRDFWriter returns a streaming writer for nt or nq, and nil for the formats
// that cannot stream.
func NewRDFWriter(w io.Writer, o RDFOptions) *RDFWriter {
	switch o.Format {
	case "", FormatNT:
		return &RDFWriter{w: w, suffix: " .\n"}
	case FormatNQuads:
		suffix := " .\n"
		if o.Graph != "" {
			suffix = " <" + o.Graph + "> .\n"
		}
		return &RDFWriter{w: w, suffix: suffix}
	}
	return nil
}

// Streams reports whether a format can be written a triple at a time.
func Streams(format string) bool {
	return format == "" || format == FormatNT || format == FormatNQuads
}

func (r *RDFWriter) Node(n *Node) error { return r.lines(nodeLines(*n)) }

func (r *RDFWriter) Edge(e *Edge) error { return r.lines(edgeLines(*e)) }

func (r *RDFWriter) Fact(f *Fact) error { return r.lines(factLines(*f)) }

func (r *RDFWriter) lines(ls []string) error {
	for _, l := range ls {
		if _, err := io.WriteString(r.w, l+r.suffix); err != nil {
			return err
		}
	}
	return nil
}

func writeTriples(w io.Writer, g *Graph, graph string) error {
	r := NewRDFWriter(w, RDFOptions{Format: FormatNQuads, Graph: graph})
	for i := range g.Nodes {
		if err := r.Node(&g.Nodes[i]); err != nil {
			return err
		}
	}
	for i := range g.Edges {
		if err := r.Edge(&g.Edges[i]); err != nil {
			return err
		}
	}
	for i := range g.Facts {
		if err := r.Fact(&g.Facts[i]); err != nil {
			return err
		}
	}
	return nil
}

// nodeLines states a node's classes and its label.
func nodeLines(n Node) []string {
	subj := iri(n.URI)
	var out []string
	for _, t := range rdfTypes[n.Kind] {
		out = append(out, subj+" <"+NSRdf+"type> "+expand(t))
	}
	if n.Label != "" {
		out = append(out, subj+" "+expand("rdfs:label")+" "+quote(n.Label))
	}
	return out
}

// edgeLines renders one edge.
//
// A weighted edge is reified: a contributor's commit count is a property of the
// relation and not of either end, and the only honest way to say that in RDF is
// to give the relation a node of its own.
func edgeLines(e Edge) []string {
	subj := iri(e.Subject)
	obj := objectTerm(e.Predicate, e.Object)
	out := []string{subj + " " + expand(rdfPredicate(e.Predicate)) + " " + obj}
	if e.Weight != nil {
		blank := reifiedNode(e)
		out = append(out,
			blank+" <"+NSRdf+"subject> "+subj,
			blank+" <"+NSRdf+"predicate> "+expand(rdfPredicate(e.Predicate)),
			blank+" <"+NSRdf+"object> "+obj,
			blank+" "+expand(weightTerm(e.Predicate))+" "+quote(strconv.Itoa(*e.Weight))+"^^"+expand(TypeInteger),
		)
	}
	if e.At != nil {
		out = append(out, reifiedNode(e)+" "+expand("schema:dateCreated")+" "+
			quote(e.At.UTC().Format(time.RFC3339))+"^^"+expand(TypeDateTime))
	}
	return out
}

// weightTerm names what a weight counts. Only contributedTo and reactedWith
// carry one, and calling both of them "count" would throw away the only thing
// that makes the number readable.
func weightTerm(pred string) string {
	switch pred {
	case PredContributedTo:
		return "gh:commitCount"
	case PredReactedWith:
		return "gh:reactionCount"
	default:
		return "gh:count"
	}
}

// reifiedNode names the statement itself. The name is derived from the triple,
// so two runs produce the same node and a merge of two exports does not
// duplicate it.
func reifiedNode(e Edge) string {
	key := e.Subject + "|" + e.Predicate + "|" + e.Object
	return "_:stmt-" + strings.NewReplacer("://", "-", "/", "-", "|", "-", "#", "-", "@", "-", " ", "_").Replace(key)
}

func factLines(f Fact) []string {
	pred, ok := rdfFacts[f.Predicate]
	if !ok {
		pred = "gh:" + f.Predicate
	}
	obj := quote(f.Value)
	if f.Datatype != "" {
		obj += "^^" + expand(f.Datatype)
	}
	return []string{iri(f.Subject) + " " + expand(pred) + " " + obj}
}

// rdfPredicate maps a predicate onto its RDF term, defaulting to the gh:
// namespace so a new predicate needs no entry to serialise correctly.
func rdfPredicate(pred string) string {
	if p, ok := rdfPredicates[pred]; ok {
		return p
	}
	return "gh:" + pred
}

// objectTerm renders an edge's object. Most objects are URIs. A language and a
// licence are bare strings on the record plane, and they get synthetic IRIs
// here rather than becoming string literals, because gh:language/go is
// something two exports can join on and "Go" is not.
func objectTerm(pred, object string) string {
	if strings.HasPrefix(object, Scheme+"://") {
		return iri(object)
	}
	switch pred {
	case PredWrittenIn:
		return "<" + NSGH + "language/" + slug(object) + ">"
	case PredLicensedUnder:
		return "<" + NSGH + "license/" + slug(object) + ">"
	case PredReactedWith:
		return "<" + NSGH + "reaction/" + slug(object) + ">"
	}
	return quote(object)
}

// slug makes a URI path segment out of a rendered name. Spaces and slashes are
// the only characters that actually occur here, in names like "Jupyter Notebook"
// and "BSD 3-Clause", and both have to go.
func slug(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '/', '\\':
			b.WriteByte('-')
		case '<', '>', '"', '{', '}', '|', '^', '`':
			// Characters an IRI may not carry. Dropping them beats escaping
			// them, because nobody wants gh:license/BSD%203-Clause.
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// iri renders a subject or object. A blank node stays a blank node, and a
// github:// URI becomes the canonical https URL.
func iri(uri string) string {
	if strings.HasPrefix(uri, "_:") {
		return uri
	}
	return "<" + IRI(uri) + ">"
}

// IRI maps a github:// URI to its dereferenceable https form. The github:// form
// stays on the record plane, where it is a stable key rather than a location.
//
// The three derived kinds have no address of their own, so they map into the
// gh: namespace instead of pretending to be a page.
func IRI(uri string) string {
	if !strings.HasPrefix(uri, Scheme+"://") {
		return uri
	}
	rest := strings.TrimPrefix(uri, Scheme+"://")
	kind, id, ok := strings.Cut(rest, "/")
	if !ok {
		return uri
	}
	if u, err := Locate(kind, id); err == nil {
		return u
	}
	return NSGH + kind + "/" + slug(id)
}

func expand(curie string) string {
	prefix, rest, ok := strings.Cut(curie, ":")
	if !ok {
		return "<" + curie + ">"
	}
	for _, p := range rdfPrefixes {
		if p[0] == prefix {
			return "<" + p[1] + rest + ">"
		}
	}
	return "<" + curie + ">"
}

func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// --- turtle ---

// writeTurtle groups by subject, which is the whole reason to prefer Turtle: a
// node and everything said about it read as one paragraph.
func writeTurtle(w io.Writer, g *Graph) error {
	for _, p := range rdfPrefixes {
		if _, err := fmt.Fprintf(w, "@prefix %s: <%s> .\n", p[0], p[1]); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}

	bySubject := map[string][][2]string{}
	var order []string
	add := func(subj, pred, obj string) {
		if _, seen := bySubject[subj]; !seen {
			order = append(order, subj)
		}
		bySubject[subj] = append(bySubject[subj], [2]string{pred, obj})
	}
	// The line renderers already produce N-Triples, and Turtle is the same
	// triples with the subject factored out, so this splits each line rather
	// than growing a second renderer that could disagree with the first.
	collect := func(lines []string) {
		for _, l := range lines {
			subj, pred, obj, ok := splitTriple(l)
			if !ok {
				continue
			}
			add(subj, shorten(pred), shorten(obj))
		}
	}
	for _, n := range g.Nodes {
		collect(nodeLines(n))
	}
	for _, e := range g.Edges {
		collect(edgeLines(e))
	}
	for _, f := range g.Facts {
		collect(factLines(f))
	}

	for _, subj := range order {
		if _, err := io.WriteString(w, shorten(subj)+"\n"); err != nil {
			return err
		}
		pairs := bySubject[subj]
		for i, pair := range pairs {
			end := " ;\n"
			if i == len(pairs)-1 {
				end = " .\n\n"
			}
			if _, err := io.WriteString(w, "    "+pair[0]+" "+pair[1]+end); err != nil {
				return err
			}
		}
	}
	return nil
}

// splitTriple pulls a rendered N-Triples line apart. The grammar is regular
// enough for this: the subject and the predicate are always angle-bracketed or
// blank-node terms with no spaces in them, and everything after the second
// space is the object.
func splitTriple(line string) (subj, pred, obj string, ok bool) {
	subj, rest, ok := strings.Cut(line, " ")
	if !ok {
		return "", "", "", false
	}
	pred, obj, ok = strings.Cut(rest, " ")
	if !ok {
		return "", "", "", false
	}
	return subj, pred, obj, true
}

// shorten turns an expanded IRI back into a CURIE where a prefix covers it,
// which is what makes Turtle readable rather than just grouped.
func shorten(term string) string {
	if !strings.HasPrefix(term, "<") {
		// A literal, possibly with a datatype that is itself an IRI.
		if i := strings.Index(term, "^^<"); i >= 0 {
			return term[:i+2] + shorten(term[i+2:])
		}
		return term
	}
	full := strings.TrimSuffix(strings.TrimPrefix(term, "<"), ">")
	if full == NSRdf+"type" {
		return "a"
	}
	for _, p := range rdfPrefixes {
		// ghr is the whole of github.com, so every subject IRI would collapse
		// into it and read as ghr:golang/go, which is not a legal CURIE local
		// name once a path has slashes in it. Subjects stay in angle brackets.
		if p[0] == "ghr" {
			continue
		}
		if rest, found := strings.CutPrefix(full, p[1]); found && rest != "" && !strings.ContainsAny(rest, "/") {
			return p[0] + ":" + rest
		}
	}
	return term
}

// --- json-ld ---

// writeJSONLD emits one object per node with its edges and facts folded in, and
// an inline context so the document stands alone. It buffers everything, which
// is why the help text points at it for single records rather than crawls.
func writeJSONLD(w io.Writer, g *Graph) error {
	ctx := map[string]any{}
	for _, p := range rdfPrefixes {
		ctx[p[0]] = p[1]
	}

	byURI := map[string]map[string]any{}
	var order []string
	obj := func(uri string) map[string]any {
		o, ok := byURI[uri]
		if !ok {
			o = map[string]any{"@id": IRI(uri)}
			byURI[uri] = o
			order = append(order, uri)
		}
		return o
	}
	for _, n := range g.Nodes {
		o := obj(n.URI)
		if types := rdfTypes[n.Kind]; len(types) > 0 {
			o["@type"] = types
		}
		if n.Label != "" {
			o["rdfs:label"] = n.Label
		}
		if n.URL != "" {
			o["schema:url"] = n.URL
		}
	}
	for _, e := range g.Edges {
		o := obj(e.Subject)
		var value any
		if strings.HasPrefix(e.Object, Scheme+"://") {
			value = map[string]any{"@id": IRI(e.Object)}
		} else if term := objectTerm(e.Predicate, e.Object); strings.HasPrefix(term, "<") {
			value = map[string]any{"@id": strings.TrimSuffix(strings.TrimPrefix(term, "<"), ">")}
		} else {
			value = e.Object
		}
		if e.Weight != nil {
			value = map[string]any{"@id": jsonldID(value), weightTerm(e.Predicate): *e.Weight}
		}
		appendValue(o, rdfPredicate(e.Predicate), value)
	}
	for _, f := range g.Facts {
		pred, ok := rdfFacts[f.Predicate]
		if !ok {
			pred = "gh:" + f.Predicate
		}
		appendValue(obj(f.Subject), pred, jsonldLiteral(f))
	}

	graph := make([]map[string]any, 0, len(order))
	for _, uri := range order {
		graph = append(graph, byURI[uri])
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"@context": ctx, "@graph": graph})
}

func jsonldID(v any) any {
	if m, ok := v.(map[string]any); ok {
		return m["@id"]
	}
	return v
}

// appendValue keeps repeated predicates as a list rather than letting the last
// one win, because a repository with twelve topics has twelve of them.
func appendValue(o map[string]any, pred string, value any) {
	switch cur := o[pred].(type) {
	case nil:
		o[pred] = value
	case []any:
		o[pred] = append(cur, value)
	default:
		o[pred] = []any{cur, value}
	}
}

// jsonldLiteral gives a value its type, so a count arrives as a number and a
// timestamp as a typed value rather than as prose.
func jsonldLiteral(f Fact) any {
	switch f.Datatype {
	case TypeInteger:
		if n, err := strconv.ParseInt(f.Value, 10, 64); err == nil {
			return n
		}
	case TypeDecimal:
		if v, err := strconv.ParseFloat(f.Value, 64); err == nil {
			return v
		}
	case TypeBoolean:
		return f.Value == "true"
	case TypeDateTime:
		return map[string]any{"@value": f.Value, "@type": TypeDateTime}
	}
	return f.Value
}
