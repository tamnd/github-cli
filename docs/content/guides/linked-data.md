---
title: "Export linked data"
description: "RDF, Turtle, JSON-LD, and N-Quads out of github.com."
weight: 20
---

Once the site is nodes and edges, serialising it as RDF is a formatting problem.
`github` writes four serialisations, all keyed on github.com URLs, so a triple this tool emits joins with a triple anyone else emits about the same page.

## One entity

```bash
github rdf gohugoio/hugo                    # N-Triples, the default
github rdf gohugoio/hugo --format ttl       # Turtle
github rdf gohugoio/hugo --format jsonld    # JSON-LD
github rdf gohugoio/hugo --format nq        # N-Quads
```

Turtle is the readable one:

```turtle
<https://github.com/gohugoio/hugo>
    a schema:SoftwareSourceCode ;
    a doap:Project ;
    rdfs:label "gohugoio/hugo" ;
    schema:keywords <https://github.com/topics/blog-engine> ;
```

## The vocabulary

Established vocabularies first, a private namespace only where nothing fits:

| Prefix | Namespace |
|---|---|
| `schema:` | `https://schema.org/` |
| `doap:` | `http://usefulinc.com/ns/doap#` |
| `foaf:` | `http://xmlns.com/foaf/0.1/` |
| `rdf:`, `rdfs:`, `xsd:` | the RDF core |
| `gh:` | `https://github.com/ns#`, for the things GitHub has that nobody standardised |

A repository is a `schema:SoftwareSourceCode` and a `doap:Project`.
An issue is a `gh:Issue` and a `schema:DiscussionForumPosting`.
A user is a `schema:Person`, an org is a `schema:Organization`, a release is a `schema:SoftwareApplication`.

The double typing is deliberate.
The schema.org type is what a general consumer already understands, and the `gh:` type is what a GitHub-aware consumer needs to tell an issue apart from a discussion.

## More than one entity

`--depth` walks before serialising, so a single document can carry a subgraph:

```bash
github rdf golang/go --depth 2 --format ttl > go.ttl
github rdf golang/go --depth 2 --follow dependsOn --format nt | wc -l
```

The same trust filter applies here as in `github edges`, which matters more for RDF than for anything else, since a triple carries no evidence with it:

```bash
github rdf golang/go --min-trust payload --format ttl
```

For a whole crawl, `github export` writes the same formats in one go:

```bash
github export golang/go --depth 2 --format ttl --out go.ttl
github export golang/go --depth 2 --format nq  --out go.nq
```

## Named graphs

N-Quads puts each triple in a named graph, and the default name is the entity's own URL, so you can tell which read produced which statement after merging several:

```bash
github rdf gohugoio/hugo --format nq
github rdf gohugoio/hugo --format nq --graph https://example.org/import/2026-07-25
```

## Loading it somewhere

Every format here is standard, so anything that reads RDF will take it:

```bash
github export golang/go --depth 2 --format ttl --out go.ttl
# then load go.ttl into Oxigraph, Jena, GraphDB, Blazegraph, rdflib, whatever
```

Next: [look inside a repository](/guides/contents/).
