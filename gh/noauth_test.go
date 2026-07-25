package gh

import (
	"bytes"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noauth_test.go is the one promise this tool makes that a reader cannot check
// by using it: that nothing here ever authenticates. A person can see that a
// command works without logging in, but not that no code path would send a
// credential if one happened to be around.
//
// So the promise is asserted instead. The test parses every source file with
// comments dropped and fails on the words that would mean the promise was
// broken. Comments are dropped because this file, the doctor, and the spec all
// talk about tokens at length, and a grep that could not tell prose from code
// would have to be switched off the first time someone documented the rule.

var forbidden = []string{
	"Authorization",
	"GITHUB_TOKEN",
	"GH_TOKEN",
	"api.github.com",
}

// allowed lists the files that name a forbidden word in code for a reason. Each
// one is here because saying the word is the point: doctor reads the
// environment to warn that a token is ignored, and this test names all four.
var allowed = map[string]string{
	"gh/doctor.go":      "reads the token variables by name to warn that they are ignored",
	"gh/noauth_test.go": "is this test",
}

func TestNoAuth(t *testing.T) {
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
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if _, ok := allowed[rel]; ok {
			return nil
		}

		// Parsing without ParseComments and printing the result is how the
		// comments come out: the printer only writes what the AST holds.
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Errorf("%s: %v", rel, perr)
			return nil
		}
		var code bytes.Buffer
		if perr := (&printer.Config{Mode: printer.RawFormat}).Fprint(&code, fset, file); perr != nil {
			t.Errorf("%s: %v", rel, perr)
			return nil
		}
		for _, word := range forbidden {
			if bytes.Contains(code.Bytes(), []byte(word)) {
				t.Errorf("%s names %q in code. This tool reads public pages and never authenticates; if this is deliberate, the file needs a line in the allowed map saying why", rel, word)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNoAuthCoversItself keeps the allow list honest. A file that stops needing
// its exemption should lose it, otherwise the list grows into a place to hide
// things.
func TestNoAuthCoversItself(t *testing.T) {
	root := moduleRoot(t)
	for rel, why := range allowed {
		if why == "" {
			t.Errorf("%s is exempt with no reason given", rel)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s is exempt and does not exist: %v", rel, err)
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test directory")
		}
		dir = parent
	}
}
