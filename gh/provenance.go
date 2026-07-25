package gh

import (
	"encoding/json"
	"sort"
)

// provenance.go records where a record's fields came from and where its sources
// disagreed.
//
// Records are assembled by decoding several blocks of one page into one struct,
// so there is no field-level merge to do. What is worth keeping is the note of
// which extraction tier answered for a field, and the tripwire for the case
// where two blocks say different things about the same field. The suite asserts
// _conflict is absent, so a disagreement becomes a named test failure and gets
// looked at, instead of being quietly resolved in favour of whichever decoder
// ran last.

// recordConflicts writes the disagreements into Extra["_conflict"]. It is a
// reserved key and the suite asserts it is absent, which is the whole point:
// this is a tripwire, not a feature.
func recordConflicts(b *Base, conflicts map[string][]string) {
	if len(conflicts) == 0 {
		return
	}
	m := map[string]json.RawMessage{}
	if len(b.Extra) > 0 {
		_ = json.Unmarshal(b.Extra, &m)
	}
	existing := map[string][]string{}
	if raw, ok := m["_conflict"]; ok {
		_ = json.Unmarshal(raw, &existing)
	}
	for k, v := range conflicts {
		existing[k] = v
	}
	raw, err := json.Marshal(existing)
	if err != nil {
		return
	}
	m["_conflict"] = raw
	out, err := json.Marshal(m)
	if err != nil {
		return
	}
	b.Extra = out
}

// recordVia notes which extraction tier produced a field. It is what tells you
// that a field which used to arrive from a JSON payload is now arriving from a
// class selector, which is the early warning that something moved.
func recordVia(b *Base, field, tier string) {
	if b.Via == nil {
		b.Via = map[string]string{}
	}
	b.Via[field] = tier
}

// sortedKeys keeps merge and conflict output deterministic. Map order is random
// in Go, and a record whose field order changes between runs makes every diff
// of two outputs useless.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
