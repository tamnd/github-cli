package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
	"github.com/tamnd/github-cli/gh"
)

// files.go holds the commands that emit bytes rather than records. They are the
// reason the tool is useful for actual file work and not only metadata, and they
// are escape hatches precisely because a file is not a record: piping a tarball
// through a JSON renderer would be a mistake in every direction.

// clientFrom reaches the one client kit built for this run. Escape-hatch
// commands do not get the kit:"inject" treatment, so they ask for it here, and
// asking here means they share the run's pacing and cache with every operation.
func clientFrom(ctx context.Context) (*gh.Client, error) {
	st := kit.FromContext(ctx)
	if st == nil {
		return nil, errs.New(errs.KindGeneric, "no run state on the context")
	}
	v, err := st.Client(ctx)
	if err != nil {
		return nil, err
	}
	c, ok := v.(*gh.Client)
	if !ok {
		return nil, errs.New(errs.KindGeneric, "the run has no github client")
	}
	return c, nil
}

type catCmd struct{ ref string }

func newCatCmd() kit.Command {
	c := &catCmd{}
	return kit.Command{
		Use:   "cat <repo> <path>",
		Short: "Write one file from a repository to stdout",
		Long: "cat streams the bytes straight through raw.githubusercontent.com, so a\n" +
			"large file costs no memory and is never written to the cache. A blob URL\n" +
			"works as a single argument, since it already names the path and the ref.",
		Group: "contents",
		Args:  kit.RangeArgs(1, 2),
		Flags: c.flags,
		Run:   c.run,
	}
}

func (c *catCmd) flags(f *kit.FlagSet) {
	f.StringVar(&c.ref, "rev", "", "branch, tag, or commit sha")
}

func (c *catCmd) run(ctx context.Context, args []string) error {
	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}
	repo, ref, path, err := resolvePath(args, c.ref)
	if err != nil {
		return err
	}
	if path == "" {
		return errs.Usage("cat needs a path, either as a second argument or in the URL")
	}
	_, err = cl.Download(ctx, repo, ref, path, os.Stdout)
	return err
}

// resolvePath is the byte-plane twin of the resolver the operations use: it
// accepts a blob URL that carries everything, or a repository and a path.
func resolvePath(args []string, rev string) (repo, ref, path string, err error) {
	arg := ""
	if len(args) > 1 {
		arg = args[1]
	}
	kind, id, err := gh.Classify(args[0])
	if err != nil {
		return "", "", "", err
	}
	if kind == gh.KindFile || kind == gh.KindTree {
		if r, v, p, ok := gh.SplitPathID(id); ok {
			if rev != "" {
				v = rev
			}
			if arg != "" {
				p = arg
			}
			return r, v, p, nil
		}
	}
	repo, err = gh.ResolveRepo(args[0])
	if err != nil {
		return "", "", "", err
	}
	return repo, rev, arg, nil
}

type readmeCmd struct {
	ref  string
	html bool
}

func newReadmeCmd() kit.Command {
	c := &readmeCmd{}
	return kit.Command{
		Use:   "readme <repo>",
		Short: "Write a repository's README to stdout",
		Long: "readme prints the rendered README as text. GitHub renders it server side,\n" +
			"so what comes back is what the page shows, badges resolved and relative\n" +
			"links rewritten. Use --html for the markup the page carries.",
		Group: "contents",
		Args:  kit.ExactArgs(1),
		Flags: c.flags,
		Run:   c.run,
	}
}

func (c *readmeCmd) flags(f *kit.FlagSet) {
	f.StringVar(&c.ref, "rev", "", "branch, tag, or commit sha")
	f.BoolVar(&c.html, "html", false, "print the rendered markup instead of the text")
}

func (c *readmeCmd) run(ctx context.Context, args []string) error {
	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}
	id, err := gh.ResolveRepo(args[0])
	if err != nil {
		return err
	}
	// A ref makes this a file read rather than a repository read, because the
	// repository page only ever renders the default branch's README.
	if c.ref != "" {
		r, err := cl.Repo(ctx, id, gh.RepoOptions{})
		if err != nil {
			return err
		}
		path := r.ReadmePath
		if path == "" {
			path = "README.md"
		}
		_, err = cl.Download(ctx, id, c.ref, path, os.Stdout)
		return err
	}
	r, err := cl.Repo(ctx, id, gh.RepoOptions{Readme: true})
	if err != nil {
		return err
	}
	text := r.ReadmeText
	if c.html {
		text = r.ReadmeHTML
	}
	if text == "" {
		return errs.NotFound("no README in %s", id)
	}
	_, err = io.WriteString(os.Stdout, text)
	return err
}

type archiveCmd struct {
	format string
	out    string
}

func newArchiveCmd() kit.Command {
	c := &archiveCmd{}
	return kit.Command{
		Use:   "archive <repo> [ref]",
		Short: "Download a repository as a tarball or a zip",
		Long: "One request to codeload gets the whole tree. For anything past a few\n" +
			"directories this beats walking `github tree --recursive`, which is one\n" +
			"request per directory. Nothing is buffered: the stream goes straight to\n" +
			"the file or to stdout.",
		Group: "contents",
		Args:  kit.RangeArgs(1, 2),
		Flags: c.flags,
		Run:   c.run,
	}
}

func (c *archiveCmd) flags(f *kit.FlagSet) {
	f.StringVar(&c.format, "format", "tar.gz", "tar.gz or zip")
	f.StringVarP(&c.out, "output", "o", "", "write here instead of stdout")
}

func (c *archiveCmd) run(ctx context.Context, args []string) error {
	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}
	repo, err := gh.ResolveRepo(args[0])
	if err != nil {
		return err
	}
	ref := ""
	if len(args) > 1 {
		ref = args[1]
	}
	body, err := cl.Archive(ctx, repo, ref, c.format)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()

	if c.out != "" {
		f, err := os.Create(c.out)
		if err != nil {
			return err
		}
		// Closed by hand rather than deferred, and the error is returned. An
		// archive is written to a file someone means to keep, and a close that
		// fails is the one that flushes the last of it.
		if _, err := io.Copy(f, body); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	}
	_, err = io.Copy(os.Stdout, body)
	return err
}

type diffCmd struct{ patch bool }

func newDiffCmd() kit.Command {
	c := &diffCmd{}
	return kit.Command{
		Use:   "diff <ref> [base] [head]",
		Short: "Write a commit's or a range's diff to stdout",
		Long: "diff takes a commit reference for one commit's changes, or a repository\n" +
			"and two refs for a range. --patch gives the git-format-patch mailbox\n" +
			"instead, which carries the author, the date, and the message of every\n" +
			"commit and applies cleanly with git am.",
		Group: "contents",
		Args:  kit.RangeArgs(1, 3),
		Flags: c.flags,
		Run:   c.run,
	}
}

func (c *diffCmd) flags(f *kit.FlagSet) {
	f.BoolVar(&c.patch, "patch", false, "the format-patch mailbox rather than the plain diff")
}

func (c *diffCmd) run(ctx context.Context, args []string) error {
	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}
	url, err := diffURL(args)
	if err != nil {
		return err
	}
	fetch := cl.Diff
	if c.patch {
		fetch = cl.Patch
	}
	text, err := fetch(ctx, url)
	if err != nil {
		return err
	}
	_, err = io.WriteString(os.Stdout, text)
	return err
}

// diffURL turns the two shapes into the one page URL both diffs hang off.
func diffURL(args []string) (string, error) {
	if len(args) >= 3 {
		repo, err := gh.ResolveRepo(args[0])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s/%s/compare/%s...%s", gh.BaseURL, repo, args[1], args[2]), nil
	}
	kind, id, err := gh.Classify(args[0])
	if err != nil {
		return "", err
	}
	switch kind {
	case gh.KindCommit, gh.KindCompare, gh.KindPR:
		return gh.Locate(kind, id)
	}
	// Leads with a word, not with the argument. The renderer capitalises the
	// first token of an error, and "golang/go" coming back as "Golang/Go" reads
	// like the tool mangled the input rather than like the input was the wrong
	// kind of thing.
	return "", errs.Usage("cannot diff %q, which is a %s; diff needs a commit, a pull request, a compare URL, or a repository with two refs", args[0], kind)
}
