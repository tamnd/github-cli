package page

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func parse(t *testing.T, s string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// The shape here is GitHub's, not an invention: a page renders its content and
// then renders a dialog that repeats the same tags for the picker overlay. A
// Find that answers with the last match reads the dialog and looks like it
// worked.
const twoOfEverything = `<html><body>
<div data-hpc>
  <div class="Box">
    <h1 class="title">GitHub CLI 2.63.2</h1>
    <relative-time datetime="2024-12-05T18:15:12Z">a while ago</relative-time>
    <dialog>
      <h1 class="Overlay-title">Choose a tag to compare</h1>
      <relative-time datetime="2024-12-05 17:22:29 UTC">then</relative-time>
    </dialog>
  </div>
</div>
</body></html>`

func TestFindReturnsTheFirstMatch(t *testing.T) {
	doc := parse(t, twoOfEverything)
	h := Find(doc, Sel{Tag: "h1"})
	if h == nil {
		t.Fatal("no h1")
	}
	if got := Text(h); got != "GitHub CLI 2.63.2" {
		t.Errorf("h1 is %q, the dialog's copy won", got)
	}
	rt := Find(doc, RelTimeEl)
	if rt == nil {
		t.Fatal("no relative-time")
	}
	if got := Attr(rt, "datetime"); got != "2024-12-05T18:15:12Z" {
		t.Errorf("datetime is %q, the dialog's copy won", got)
	}
}

func TestFindAllIsInDocumentOrder(t *testing.T) {
	doc := parse(t, twoOfEverything)
	all := FindAll(doc, Sel{Tag: "h1"})
	if len(all) != 2 {
		t.Fatalf("found %d h1s", len(all))
	}
	if Text(all[0]) != "GitHub CLI 2.63.2" || Text(all[1]) != "Choose a tag to compare" {
		t.Errorf("out of order: %q then %q", Text(all[0]), Text(all[1]))
	}
}

func TestFindNested(t *testing.T) {
	// A match inside a match is still a match, and the outer one is first.
	doc := parse(t, `<html><body><div class="Box"><div class="Box">inner</div></div></body></html>`)
	n := Find(doc, Sel{Class: "Box"})
	if n == nil {
		t.Fatal("no Box")
	}
	if inner := Find(n.FirstChild, Sel{Class: "Box"}); inner == nil {
		t.Error("the outer Box has no inner Box, so Find picked the inner one")
	}
}

func TestSelMatch(t *testing.T) {
	doc := parse(t, `<html><body>
<a id="x" class="Link Link--muted" href="/cli/cli/releases/tag/v1.0.0" data-hpc>tag</a>
<span class="Truncate-text">sha256:abc</span>
</body></html>`)
	cases := []struct {
		name string
		sel  Sel
		want bool
	}{
		{"tag", Sel{Tag: "a"}, true},
		{"id", Sel{ID: "x"}, true},
		{"wrong id", Sel{ID: "y"}, false},
		{"one class of several", Sel{Class: "Link--muted"}, true},
		{"class is not a substring", Sel{Class: "Link--mut"}, false},
		{"bare attribute", Sel{Tag: "a", Attr: "data-hpc"}, true},
		{"attr contains", Sel{Attr: "href", AttrContains: "/releases/tag/"}, true},
		{"attr prefix", Sel{Attr: "href", AttrPrefix: "/cli/"}, true},
		{"attr suffix", Sel{Attr: "href", AttrSuffix: "v1.0.0"}, true},
		{"attr value must be exact", Sel{Attr: "id", AttrValue: "x"}, true},
		{"attr value mismatch", Sel{Attr: "id", AttrValue: "xx"}, false},
	}
	for _, c := range cases {
		if got := Find(doc, c.sel) != nil; got != c.want {
			t.Errorf("%s: matched %v, want %v", c.name, got, c.want)
		}
	}
}

// The markup here is the shape GitHub's markdown renderer emits into a README:
// a heading, a paragraph broken across source lines, a list, and a fenced code
// block that came through as <pre><code>.
func TestBlockText(t *testing.T) {
	doc := parse(t, `<html><body><article class="markdown-body">
<h1>gh</h1>
<p>GitHub on
the command line.</p>
<p>It brings <a href="/x">pull requests</a> to the terminal.</p>
<ul><li>one</li><li>two</li></ul>
<pre><code>func main() {
	println("hi")
}
</code></pre>
<p>Done.</p>
</article></body></html>`)

	want := strings.Join([]string{
		"gh",
		"",
		"GitHub on the command line.",
		"",
		"It brings pull requests to the terminal.",
		"",
		"one",
		"",
		"two",
		"",
		"func main() {",
		"\tprintln(\"hi\")",
		"}",
		"",
		"Done.",
	}, "\n")

	got := BlockText(Find(doc, Sel{Class: "markdown-body"}))
	if got != want {
		t.Errorf("BlockText:\n%q\nwant:\n%q", got, want)
	}
}

func TestBlockTextKeepsCodeIndentation(t *testing.T) {
	// A code block's leading whitespace is its meaning, so it survives even
	// though every other line gets collapsed.
	got := FragmentText("<pre>  indented\n    more\n</pre>")
	if got != "  indented\n    more" {
		t.Errorf("FragmentText(pre) = %q", got)
	}
}

func TestBlockTextDropsChrome(t *testing.T) {
	// Wrapper divs are the bulk of GitHub's markup and none of its prose, so a
	// stack of them must not turn into a stack of blank lines.
	got := FragmentText(`<div><div><div><p>a</p></div></div></div>
<script>var x = 1</script>
<div><p>b</p></div>`)
	if got != "a\n\nb" {
		t.Errorf("FragmentText = %q, want %q", got, "a\n\nb")
	}
}

func TestBlockTextEmpty(t *testing.T) {
	if got := BlockText(nil); got != "" {
		t.Errorf("BlockText(nil) = %q", got)
	}
	if got := FragmentText("   "); got != "" {
		t.Errorf("FragmentText(blank) = %q", got)
	}
}
