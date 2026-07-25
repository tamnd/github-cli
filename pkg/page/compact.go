package page

import (
	"strconv"
	"strings"
)

// compact.go parses the numbers GitHub renders for people rather than for
// programs: 313k followers, 1.2k stars, 8,112 commits.
//
// The rule elsewhere in this tool is never to parse a rendered number when a
// raw one exists, and it holds: sidebarAbout.stargazerCount is an integer and
// is always preferred. These functions are for the pages where the rendered
// form is the only form there is.

// ParseCompactCount reads a rendered count and returns the integer plus the
// original string. Both are kept because the grouping separator depends on the
// locale GitHub infers, so "8,112" and "8.112" are the same number, and
// throwing away the original would make that ambiguity invisible.
//
// Returns ok=false rather than zero when the string is not a number at all.
// Absent is not zero: a parser that returns 0 on failure is a bug that ships
// quietly and is discovered a year later in someone's aggregate.
func ParseCompactCount(s string) (n int, display string, ok bool) {
	display = strings.TrimSpace(s)
	t := strings.ToLower(display)
	t = strings.TrimSpace(strings.NewReplacer(" ", " ", "+", "").Replace(t))
	if t == "" {
		return 0, display, false
	}
	// A suffix multiplier turns the rest into a float: 1.2k is 1200.
	mult := 1
	switch {
	case strings.HasSuffix(t, "k"):
		mult, t = 1_000, strings.TrimSuffix(t, "k")
	case strings.HasSuffix(t, "m"):
		mult, t = 1_000_000, strings.TrimSuffix(t, "m")
	case strings.HasSuffix(t, "b"):
		mult, t = 1_000_000_000, strings.TrimSuffix(t, "b")
	}
	t = strings.TrimSpace(t)
	if mult > 1 {
		f, err := strconv.ParseFloat(strings.Replace(t, ",", ".", 1), 64)
		if err != nil {
			return 0, display, false
		}
		return int(f * float64(mult)), display, true
	}
	// No suffix, so any separator is a thousands separator. GitHub uses a
	// comma, a period, or a thin space depending on the inferred locale, and
	// all three mean the same thing here.
	t = strings.NewReplacer(",", "", ".", "", " ", "", " ", "", "'", "").Replace(t)
	v, err := strconv.Atoi(t)
	if err != nil {
		return 0, display, false
	}
	return v, display, true
}

// CountIn parses the first token of a rendered label, which is how the profile
// counters read: "313k followers", "1.2k following".
func CountIn(s string) (int, string, bool) {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return 0, "", false
	}
	return ParseCompactCount(fields[0])
}
