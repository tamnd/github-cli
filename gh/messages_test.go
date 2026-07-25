package gh

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// messages_test.go guards a defect that is invisible in the source and obvious
// on screen. The error renderer capitalises the first token of a message, so a
// message that begins with the thing it is about comes back mangled:
//
//	errs.NotFound("%s has no README", id)   ->   Gohugoio/Hugo has no README.
//	errs.Usage("%q is a %s, not a %s", ...) ->   "Golang/Go" is a repo, not a user.
//
// A reader who sees that reasonably concludes the tool corrupted their input.
// Every message here therefore leads with a plain lowercase word, and this test
// says so, because the mistake is easy to make and impossible to see in review.

// errorFuncs are the constructors whose first string argument is shown to a
// person. errs.New takes a kind first, so its message is the second argument.
var errorFuncs = map[string]int{
	"Usage":       0,
	"NotFound":    0,
	"Unsupported": 0,
	"NeedAuth":    0,
	"RateLimited": 0,
	"NoResults":   0,
	"New":         1,
}

func TestErrorMessagesLeadWithAWord(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == "bin" || name == "dist" || name == "docs" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Errorf("%s: %v", rel, perr)
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "errs" {
				return true
			}
			at, ok := errorFuncs[sel.Sel.Name]
			if !ok || len(call.Args) <= at {
				return true
			}
			lit, ok := call.Args[at].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			msg, uerr := strconv.Unquote(lit.Value)
			if uerr != nil || msg == "" {
				return true
			}
			if bad := leadsBadly(msg); bad != "" {
				t.Errorf("%s:%d: errs.%s starts with %s: %q\nThe renderer capitalises the first token, so this reaches the reader looking like their input was mangled. Lead with a plain word instead.",
					rel, fset.Position(lit.Pos()).Line, sel.Sel.Name, bad, msg)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// leadsBadly names what is wrong with a message's first character, or returns
// empty when there is nothing wrong. Three cases matter: a format verb, because
// whatever fills it gets capitalised; a quote, because the capital lands inside
// it; and an upper-case letter, because a message that already starts capital is
// usually a proper noun that the renderer will then get wrong (GitHub, HTTP).
func leadsBadly(msg string) string {
	switch {
	case strings.HasPrefix(msg, "%"):
		return "a format verb"
	case strings.HasPrefix(msg, `"`), strings.HasPrefix(msg, "'"), strings.HasPrefix(msg, "`"):
		return "a quote"
	case msg[0] >= 'A' && msg[0] <= 'Z':
		return "a capital letter"
	case strings.HasPrefix(msg, "http://"), strings.HasPrefix(msg, "https://"):
		return "a URL"
	}
	return ""
}
