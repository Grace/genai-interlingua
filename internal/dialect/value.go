// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Float64 returns the value as a float, and whether it was numeric at all.
// Emitters disagree about whether a temperature is a double or an int, and a
// caller that wants the number should not have to care which arrived.
func (v Value) Float64() (float64, bool) {
	switch v.Kind {
	case KindFloat:
		return v.Float, true
	case KindInt:
		return float64(v.Int), true
	default:
		return 0, false
	}
}

// jsonValue converts a value decoded from JSON into the IR union. JSON has one
// number type, so an integral float becomes an Int: a max_tokens of 512 lifted
// out of a JSON blob should land on the span as 512, not 512.0.
func jsonValue(v any) Value {
	switch t := v.(type) {
	case string:
		return String(t)
	case bool:
		return Bool(t)
	case float64:
		if t == float64(int64(t)) {
			return Int(int64(t))
		}
		return Float(t)
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			s, ok := e.(string)
			if !ok {
				return Value{}
			}
			out = append(out, s)
		}
		return strSeq(out)
	default:
		return Value{}
	}
}

// strSeq builds a string sequence, or an empty Value for an empty list, so that
// a field with nothing to say is left unset rather than set to [].
func strSeq(v []string) Value {
	if len(v) == 0 {
		return Value{}
	}
	return StrSeq(v)
}

// indexed collects attributes shaped prefix.<i>.<suffix> into one map per index,
// keyed by suffix, and returns the indices in ascending order. Every emitter
// that predates gen_ai.input.messages flattens message arrays this way. They
// spell the prefix and the suffixes differently but they all agree on the
// middle: an integer index segment.
func indexed(s Span, prefix string) (map[int]map[string]Value, []int) {
	groups := make(map[int]map[string]Value)
	for _, k := range s.Keys() {
		rest, ok := strings.CutPrefix(k, prefix+".")
		if !ok {
			continue
		}
		digits, suffix, ok := strings.Cut(rest, ".")
		if !ok {
			continue
		}
		i, err := strconv.Atoi(digits)
		if err != nil {
			continue
		}
		if groups[i] == nil {
			groups[i] = make(map[string]Value)
		}
		groups[i][suffix] = s.Attributes[k]
	}
	return groups, slices.Sorted(maps.Keys(groups))
}

// subIndices returns the indices of a nested indexed list inside one group, such
// as the tool calls hanging off a single message. Indexing goes two levels deep
// in every dialect that flattens, and the second level needs the same treatment
// as the first.
func subIndices(g map[string]Value, prefix string) []int {
	seen := make(map[int]bool)
	for suffix := range g {
		rest, ok := strings.CutPrefix(suffix, prefix)
		if !ok {
			continue
		}
		digits, _, ok := strings.Cut(rest, ".")
		if !ok {
			continue
		}
		if j, err := strconv.Atoi(digits); err == nil {
			seen[j] = true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}
