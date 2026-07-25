package gh

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
)

// errors.go is the one place an HTTP status becomes a program outcome. The CLI
// exit code, the HTTP response under `github serve`, and the MCP error object
// all read the kind decided here, so a 404 means the same thing on every
// surface.
//
// Two of GitHub's status codes are routing decisions rather than failures and
// are handled by the caller, not here:
//
//	406  this route does not serve JSON; ask for HTML instead
//	410  there is no JSON at this address at all
//
// Both mean "wrong surface", which is a thing the client can fix by trying the
// other one. Turning them into errors here would hide that.
//
// Every message here leads with a word rather than with the path it is about.
// The renderer title-cases whatever a message starts with, and a path that
// comes back as Golang/Go/Blob/Master reads like the tool mangled the input
// rather than like the page was missing.

// statusError classifies a non-2xx response.
func statusError(rawURL string, status int, body []byte) error {
	where := shortURL(rawURL)
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		// This tool sends no credential, so a 401 or a 403 means the page is not
		// public. Saying "pass a token" would be wrong: there is no token to
		// pass. Saying what is actually true is more useful.
		if isRateLimitBody(body) {
			return errs.RateLimited("github is throttling anonymous reads, try again shortly (%s)", where)
		}
		return errs.NeedAuth("not public: %s, and this tool reads only public pages (use gh for the rest)", where)
	case status == http.StatusNotFound:
		return errs.NotFound("not found: %s", where)
	case status == http.StatusGone:
		return errs.NotFound("gone: %s", where)
	case status == http.StatusTooManyRequests:
		return errs.RateLimited("rate limited on %s", where)
	case status == http.StatusUnavailableForLegalReasons:
		return errs.Unsupported("unavailable for legal reasons (DMCA): %s", where)
	case status == http.StatusBadRequest:
		return errs.Usage("bad request: %s", where)
	case status >= 500:
		return errs.New(errs.KindNetwork, "server error %d on %s", status, where)
	default:
		return errs.New(errs.KindGeneric, "http %d on %s", status, where)
	}
}

// isRateLimitBody spots the throttle page GitHub serves as a 403 when a client
// asks for too much too fast. It is the same status as a private repository
// and the body is the only way to tell them apart.
func isRateLimitBody(body []byte) bool {
	s := strings.ToLower(string(body))
	if len(s) > 4096 {
		s = s[:4096]
	}
	return strings.Contains(s, "rate limit") || strings.Contains(s, "abuse detection") ||
		strings.Contains(s, "too many requests")
}

// wrapNetwork turns a transport failure into the network kind, keeping the
// distinction between "the name does not resolve" and "the page said no",
// because those two send a person to very different places.
func wrapNetwork(rawURL string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return errs.New(errs.KindNetwork, "cannot resolve %s, reading %s", dnsErr.Name, shortURL(rawURL))
	}
	return errs.New(errs.KindNetwork, "reading %s: %v", shortURL(rawURL), err)
}

// shortURL trims the scheme and the host so an error message reads as a path.
// The host is the same for every message in this tool, so printing it in every
// message is noise.
func shortURL(raw string) string {
	s := strings.TrimPrefix(raw, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.Index(s, "?"); i > 0 {
		s = s[:i]
	}
	return s
}

// notPublic is the message for the surfaces that exist but need a session:
// code search, traffic, clones, referrers. It names what would be needed rather
// than being vague, because a vague "unsupported" wastes an afternoon.
func notPublic(what, why string) error {
	return errs.Unsupported("not available without a session: %s, %s", what, why)
}

// usageBadID rejects a malformed identifier before a request goes out. Showing
// the expected shape saves the round trip and the 404 that would follow it.
func usageBadID(kind, got, want string) error {
	return errs.Usage("expected a %s like %s, got %q", kind, want, got)
}

// structureChanged is the loud failure from doc 02 section 7: the page came
// back fine but carried none of the blocks the decoder knows. That is different
// from a missing optional field, and it must not return an empty record with a
// zero exit code.
func structureChanged(what string) error {
	return errs.New(errs.KindNetwork,
		"the page structure changed for %s, none of the expected data was there (run `github page %s` to see what arrived)",
		what, what)
}

// badPayload is a decode failure on a block that was there. It is separate from
// structureChanged because the two send you to different places: structure
// changed means the block is gone, bad payload means the block arrived and no
// longer parses, which is usually a type change on one field.
func badPayload(what string, err error) error {
	return errs.New(errs.KindNetwork, "the payload for %s did not decode: %v", what, err)
}

// noJSONHere is what a 410 means. It is separated out so the message can say
// the useful half: the data is reachable, just on a different surface.
func noJSONHere(rawURL string) error {
	return errs.Unsupported("no JSON at %s; this is a page-only route", shortURL(rawURL))
}
