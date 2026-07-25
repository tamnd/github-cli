package gh

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// merge.go combines the several surfaces that answer for one record.
//
// The order is explicit and lives at the call site, one ordered list per record
// type, because a merge that cannot say which surface won is a merge that
// cannot be debugged. What lives here is only the per-field mechanics that all
// of them share:
//
//   - a source that did not return a field never clears it, so merging is
//     driven by a populated check and never by struct assignment,
//   - Sources accumulates in merge order, so provenance is the merge history,
//   - when two surfaces give different non-empty values for one field, the
//     later one wins and the disagreement is recorded in Extra["_conflict"].
//
// That last rule is the interesting one. The scenario suite asserts _conflict
// is empty, so two surfaces disagreeing becomes a named test failure and gets
// looked at, instead of being quietly averaged into a number nobody can trace.

// mergeInto copies every populated field of src into dst, returning the fields
// where the two disagreed. dst and src must be pointers to the same struct
// type.
//
// Booleans are a known soft spot: a false bool is indistinguishable from an
// unset one, so a false never overwrites a true. Every bool in the model is
// phrased so that false is the safe default (IsFork, IsArchived, HasWiki), which
// makes that the right behaviour rather than a compromise.
func mergeInto(dst, src any) map[string][]string {
	dv := reflect.ValueOf(dst)
	sv := reflect.ValueOf(src)
	if dv.Kind() != reflect.Pointer || sv.Kind() != reflect.Pointer {
		return nil
	}
	if dv.Type() != sv.Type() {
		return nil
	}
	conflicts := map[string][]string{}
	mergeStruct(dv.Elem(), sv.Elem(), "", conflicts)
	if len(conflicts) == 0 {
		return nil
	}
	return conflicts
}

func mergeStruct(dst, src reflect.Value, prefix string, conflicts map[string][]string) {
	t := dst.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		d, s := dst.Field(i), src.Field(i)

		// Base is the envelope, not data. Its fields have their own merge rules
		// and are handled by mergeBase.
		if f.Anonymous && f.Type == reflect.TypeOf(Base{}) {
			mergeBase(d.Addr().Interface().(*Base), s.Addr().Interface().(*Base))
			continue
		}
		// Any other embedded struct is part of the record: Thread inside Issue,
		// Repo inside Trending, Account inside Org.
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			mergeStruct(d, s, prefix, conflicts)
			continue
		}

		name := jsonName(f)
		if name == "-" {
			continue
		}
		if prefix != "" {
			name = prefix + "." + name
		}
		if !populated(s) {
			continue
		}
		if populated(d) && !reflect.DeepEqual(d.Interface(), s.Interface()) {
			conflicts[name] = []string{display(d), display(s)}
		}
		d.Set(s)
	}
}

// mergeBase keeps the identity of the first source and unions the provenance.
// A later surface never renames a record: if the id changed, the merge was
// between two different things and the caller made a mistake.
func mergeBase(dst, src *Base) {
	if dst.Kind == "" {
		dst.Kind = src.Kind
	}
	if dst.ID == "" {
		dst.ID = src.ID
	}
	if dst.URI == "" {
		dst.URI = src.URI
	}
	if dst.URL == "" {
		dst.URL = src.URL
	}
	dst.addSource(src.Sources...)
	dst.Extra = mergeExtra(dst.Extra, src.Extra)
}

// mergeExtra unions two unmodelled-key sets. Both are keys nobody claimed, so
// there is nothing smarter to do than keep them all; a key present in both
// keeps the later value, matching the field rule above.
func mergeExtra(dst, src json.RawMessage) json.RawMessage {
	if len(src) == 0 {
		return dst
	}
	if len(dst) == 0 {
		return src
	}
	var a, b map[string]json.RawMessage
	if json.Unmarshal(dst, &a) != nil || json.Unmarshal(src, &b) != nil {
		return dst
	}
	for k, v := range b {
		a[k] = v
	}
	out, err := json.Marshal(a)
	if err != nil {
		return dst
	}
	return out
}

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

// recordVia notes which extraction tier produced a field. It is populated only
// under --verbose, and it is what tells you that a field which used to arrive
// from a JSON payload is now arriving from a class selector, which is the early
// warning that something moved.
func recordVia(b *Base, field, tier string) {
	m := map[string]json.RawMessage{}
	if len(b.Extra) > 0 {
		_ = json.Unmarshal(b.Extra, &m)
	}
	via := map[string]string{}
	if raw, ok := m["_via"]; ok {
		_ = json.Unmarshal(raw, &via)
	}
	via[field] = tier
	raw, err := json.Marshal(via)
	if err != nil {
		return
	}
	m["_via"] = raw
	out, err := json.Marshal(m)
	if err != nil {
		return
	}
	b.Extra = out
}

// populated is the "did this surface actually say something" test. Absent is
// not zero anywhere in this tool, and this function is where that rule is
// enforced for the merge.
func populated(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
		return !v.IsNil() && (v.Kind() != reflect.Slice && v.Kind() != reflect.Map || v.Len() > 0)
	case reflect.String:
		return v.String() != ""
	case reflect.Bool:
		return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return v.Float() != 0
	case reflect.Struct:
		return !v.IsZero()
	default:
		return !v.IsZero()
	}
}

func jsonName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return f.Name
	}
	return name
}

// display renders a value for a conflict entry. It stays short: the point is to
// let a human see which two surfaces disagreed, not to reproduce the payload.
func display(v reflect.Value) string {
	if v.Kind() == reflect.Pointer && !v.IsNil() {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String:
		return truncate(v.String(), 120)
	case reflect.Slice, reflect.Map, reflect.Struct:
		raw, err := json.Marshal(v.Interface())
		if err != nil {
			return fmt.Sprint(v.Interface())
		}
		return truncate(string(raw), 200)
	default:
		return fmt.Sprint(v.Interface())
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
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
