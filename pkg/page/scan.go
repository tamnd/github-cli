package page

import (
	"bytes"
	"html"
)

// scan.go finds the JSON script blocks on the raw bytes, without an HTML
// parser. Locating `<script type="application/json"` and its matching
// `</script>` is a byte scan, and the content between them is JSON that never
// contains `</script>` because GitHub escapes it before it goes out.
//
// This matters for cost. A repository page is 300 KB and an issue page is 340
// KB, and the app payload is the only part of either that most reads need. The
// scanner allocates once per block and never builds a node tree.

type jsonBlock struct {
	dataTarget string
	id         string
	body       []byte
}

var (
	scriptOpen  = []byte("<script")
	scriptClose = []byte("</script>")
	typeJSON    = []byte(`type="application/json"`)
	typeLDJSON  = []byte(`type="application/ld+json"`)
)

// scanJSONScripts returns every <script type="application/json"> block with the
// two attributes that classify it.
func scanJSONScripts(doc []byte) []jsonBlock {
	var out []jsonBlock
	for _, tag := range scanScripts(doc) {
		if !bytes.Contains(tag.open, typeJSON) || bytes.Contains(tag.open, typeLDJSON) {
			continue
		}
		out = append(out, jsonBlock{
			dataTarget: attr(tag.open, "data-target"),
			id:         attr(tag.open, "id"),
			body:       unescapeJSON(tag.body),
		})
	}
	return out
}

// scanLDJSON returns every <script type="application/ld+json"> body. These are
// GitHub's own schema.org documents and are passed through untranslated,
// because a vocabulary the publisher chose beats one this tool invented.
func scanLDJSON(doc []byte) [][]byte {
	var out [][]byte
	for _, tag := range scanScripts(doc) {
		if bytes.Contains(tag.open, typeLDJSON) {
			out = append(out, unescapeJSON(tag.body))
		}
	}
	return out
}

type scriptTag struct {
	open []byte // the opening tag with its attributes
	body []byte
}

func scanScripts(doc []byte) []scriptTag {
	var out []scriptTag
	i := 0
	for {
		j := bytes.Index(doc[i:], scriptOpen)
		if j < 0 {
			return out
		}
		start := i + j
		// The opening tag ends at the first `>` that is not inside a quoted
		// attribute value. GitHub does not put `>` inside these attributes, but
		// checking costs one comparison and a wrong split here silently loses
		// the whole payload.
		gt := endOfTag(doc[start:])
		if gt < 0 {
			return out
		}
		openEnd := start + gt
		close := bytes.Index(doc[openEnd:], scriptClose)
		if close < 0 {
			return out
		}
		out = append(out, scriptTag{
			open: doc[start : openEnd+1],
			body: doc[openEnd+1 : openEnd+close],
		})
		i = openEnd + close + len(scriptClose)
	}
}

func endOfTag(b []byte) int {
	inQuote := byte(0)
	for i := range b {
		c := b[i]
		switch {
		case inQuote != 0:
			if c == inQuote {
				inQuote = 0
			}
		case c == '"' || c == '\'':
			inQuote = c
		case c == '>':
			return i
		}
	}
	return -1
}

// attr reads one attribute value out of an opening tag. Attribute order is not
// fixed, so this searches by name rather than by position.
func attr(tag []byte, name string) string {
	needle := []byte(name + "=")
	i := bytes.Index(tag, needle)
	if i < 0 {
		return ""
	}
	// Guard against a prefix match: data-target= must not match
	// data-targets=, and id= must not match data-id=.
	if i > 0 {
		prev := tag[i-1]
		if prev != ' ' && prev != '\t' && prev != '\n' && prev != '\r' {
			return ""
		}
	}
	rest := tag[i+len(needle):]
	if len(rest) == 0 {
		return ""
	}
	q := rest[0]
	if q != '"' && q != '\'' {
		end := bytes.IndexAny(rest, " \t\r\n>")
		if end < 0 {
			return string(rest)
		}
		return string(rest[:end])
	}
	end := bytes.IndexByte(rest[1:], q)
	if end < 0 {
		return ""
	}
	return html.UnescapeString(string(rest[1 : 1+end]))
}

// unescapeJSON undoes the HTML entity escaping GitHub applies to script
// content. It is a fast path first: the overwhelming majority of blocks
// contain no entity at all, and running the unescaper over 45 KB to change
// nothing is pure waste.
func unescapeJSON(b []byte) []byte {
	if !bytes.ContainsRune(b, '&') {
		return b
	}
	return []byte(html.UnescapeString(string(b)))
}
