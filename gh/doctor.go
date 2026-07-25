package gh

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tamnd/github-cli/pkg/page"
)

// doctor.go answers the question people ask when a command comes back wrong: is
// it me, is it the network, or did GitHub change the page?
//
// Every check is a record rather than a paragraph, so the answer can be read by
// a person and by a script, and so the failing one can be picked out with the
// same --fields and -o json every other command takes.

// doctorLong is the command's help. It lives here rather than beside the
// registration because it names the token variables, and TestNoAuth wants every
// mention of those names in the one file whose job is to talk about them.
const doctorLong = "doctor answers the question people ask when a command comes back wrong: is\n" +
	"it me, is it the network, or did GitHub change the page. It reads a small\n" +
	"file to check reachability, a repository page to check that the embedded\n" +
	"payload is still where every reader expects it, and the cache directory to\n" +
	"check that it can be written.\n\n" +
	"It also says out loud that GITHUB_TOKEN and GH_TOKEN are ignored, because a\n" +
	"token in the environment does nothing here and the failure that causes is\n" +
	"invisible: the tool works, it stays exactly as rate limited as before, and\n" +
	"the obvious conclusion is that the token is wrong."

// Check is one diagnostic.
type Check struct {
	Name   string `json:"name"   table:"check"`
	Status string `json:"status" table:"status"`
	Detail string `json:"detail" table:"detail"`
}

// The three states a check can be in. Warn exists because most of what goes
// wrong here is survivable: a token in the environment, a cache that cannot be
// written, a page that parsed but looks thinner than it should.
const (
	StatusOK   = "ok"
	StatusWarn = "warn"
	StatusFail = "fail"
)

// tokenVars are the variables people expect to matter and which do not. They are
// checked by name and never read for their value: this file will not put a
// credential in a record, and there is nothing here that would use one.
var tokenVars = []string{"GITHUB_TOKEN", "GH_TOKEN", "GITHUB_API_TOKEN", "GH_ENTERPRISE_TOKEN"}

// Doctor runs the checks in order and emits one record each. It stops for
// nothing: a failed reachability check makes the page check fail too, and seeing
// both is more useful than seeing the first one alone.
func (c *Client) Doctor(ctx context.Context, emit func(*Check) error) error {
	for _, ck := range []func(context.Context) *Check{
		c.checkAuthEnv,
		c.checkReach,
		c.checkPagePlane,
		c.checkCache,
		c.checkPacing,
	} {
		if err := emit(ck(ctx)); err != nil {
			return err
		}
	}
	return nil
}

// checkAuthEnv is the one people need and do not know to ask for. A token in the
// environment does nothing here, and the failure it causes is invisible: the
// tool works, it is just as rate limited as it was before, and the obvious
// conclusion is that the token is wrong.
func (c *Client) checkAuthEnv(context.Context) *Check {
	var set []string
	for _, v := range tokenVars {
		if os.Getenv(v) != "" {
			set = append(set, v)
		}
	}
	if len(set) == 0 {
		return &Check{Name: "auth", Status: StatusOK,
			Detail: "no token in the environment, which is what this tool wants"}
	}
	return &Check{Name: "auth", Status: StatusWarn,
		Detail: fmt.Sprintf("%s is set and ignored: github reads public pages and never sends an Authorization header, so a token changes nothing here. Use gh for the authenticated API", strings.Join(set, " and "))}
}

// checkReach is one small request to the site. robots.txt is the right target:
// it is a few hundred bytes, it is not behind any of the machinery this tool
// reads, and it comes back the same for everyone.
func (c *Client) checkReach(ctx context.Context) *Check {
	start := time.Now()
	res, err := c.Get(ctx, BaseURL+"/robots.txt", SurfaceRaw)
	if err != nil {
		return &Check{Name: "reach", Status: StatusFail,
			Detail: fmt.Sprintf("cannot read %s: %v", BaseURL, err)}
	}
	return &Check{Name: "reach", Status: StatusOK,
		Detail: fmt.Sprintf("%s answered %d in %s", BaseURL, res.Status, time.Since(start).Round(time.Millisecond))}
}

// checkPagePlane reads a repository page and looks for the embedded React
// payload. This is the check that catches the failure this tool cannot survive:
// GitHub reorganising the page. Every structureChanged error in the package
// starts here, so when one fires, this says whether the whole plane moved or
// only the one selector.
func (c *Client) checkPagePlane(ctx context.Context) *Check {
	p, err := c.Page(ctx, BaseURL+"/golang/go")
	if err != nil {
		return &Check{Name: "page", Status: StatusFail,
			Detail: fmt.Sprintf("cannot read a repository page: %v", err)}
	}
	switch {
	case p.Plane == page.PlaneReact && len(p.Payload) > 0:
		return &Check{Name: "page", Status: StatusOK,
			Detail: fmt.Sprintf("the react payload is where it should be, %d keys in %d bytes", len(p.Payload), p.Bytes)}
	case len(p.Microdata) > 0 || len(p.Meta) > 0:
		return &Check{Name: "page", Status: StatusWarn,
			Detail: "no react payload, but the meta and microdata are readable: the records will be thinner than they should be. Run github page golang/go to see what came back"}
	default:
		return &Check{Name: "page", Status: StatusFail,
			Detail: "a repository page carried nothing this understands. Either the request was intercepted or the page changed shape. Run github page golang/go --raw to see the bytes"}
	}
}

// checkCache reports what is on disk and, more to the point, whether it can be
// written. A read-only cache directory turns every run into a cold one, which
// looks like the site being slow rather than like a local problem.
func (c *Client) checkCache(context.Context) *Check {
	if c.NoCache {
		return &Check{Name: "cache", Status: StatusWarn,
			Detail: "the cache is off for this run, so every request goes to the network"}
	}
	if c.CacheDir == "" {
		return &Check{Name: "cache", Status: StatusWarn, Detail: "no cache directory is configured"}
	}
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return &Check{Name: "cache", Status: StatusFail,
			Detail: fmt.Sprintf("cannot create %s: %v", c.CacheDir, err)}
	}
	probe := filepath.Join(c.CacheDir, ".doctor")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return &Check{Name: "cache", Status: StatusFail,
			Detail: fmt.Sprintf("%s is not writable: %v", c.CacheDir, err)}
	}
	_ = os.Remove(probe)

	n, bytes := cacheSize(c.CacheDir)
	return &Check{Name: "cache", Status: StatusOK,
		Detail: fmt.Sprintf("%s holds %d entries, %s, kept for %s", c.CacheDir, n, humanBytes(bytes), c.CacheTTL)}
}

// checkPacing prints the numbers a run is using. It is not a test of anything;
// it is here because "why is this slow" and "why did I get rate limited" are
// both answered by these four values and neither is visible otherwise.
func (c *Client) checkPacing(context.Context) *Check {
	return &Check{Name: "pacing", Status: StatusOK,
		Detail: fmt.Sprintf("%s between requests across %d workers, %s timeout, %d retries, user agent %q",
			c.Rate, c.Workers, c.HTTP.Timeout, c.Retries, c.UserAgent)}
}

func cacheSize(dir string) (entries int, bytes int64) {
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // a directory that cannot be walked is reported as empty
		}
		if info, err := d.Info(); err == nil {
			entries++
			bytes += info.Size()
		}
		return nil
	})
	return entries, bytes
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
