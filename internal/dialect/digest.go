// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// Digest identifies the mappings that produced a span, so that a reader holding
// the span can ask which build of this translator read it and get an answer
// that is checkable rather than asserted.
//
// interlingua.target already records which schema version a span was written
// to. That is the destination. This is the route: the rule tables, the fields
// the unstated half declares it produces, the gates detection runs on, and the
// key and value tables of every target. Two spans with the same digest were
// read by the same mappings; two with different digests were not, and the
// difference is worth going to look at.
//
// It is derived from the mappings themselves rather than from a build stamp or
// a version constant, and that is the whole point. A commit hash moves on every
// commit, including the ones that touch nothing a span can see, so a span
// stamped with one cannot answer "did the mapping change" -- only "did anything
// change", which is always yes. docs/not-a-score.md argues the general form of
// this: a number is a measurement only if it moves when the measured thing
// moves, and not otherwise. This one is held to both halves by
// TestDigestMovesWithTheMappingsAndNotOtherwise.
//
// What it does not cover, said plainly. The unstated half of each dialect is
// Go, and Go cannot be hashed the way a table can. Digest covers the *fields*
// those methods declare they produce, via Unstated, and the readings they
// declare they perform, via Interpretations -- which key means which field,
// under what evidence, through what value table -- so a change of meaning moves
// the digest. It does not cover a change of behaviour inside a method that keeps
// performing the same declared readings. Rewriting how openinference reassembles
// messages, and changing nothing about which fields come out, leaves the digest
// where it was. That is a real limit and it is stated here rather than papered
// over with a commit hash that would have moved for a reason unrelated to the
// change.
func Digest() string { return digest() }

var digest = sync.OnceValue(func() string { return digestOf(canonical()) })

// digestOf is the hash itself, separated from the memoized Digest so that a
// test can run the real function over a perturbed rendering.
//
// It was not separated at first, and the test that was supposed to prove the
// digest moves when a mapping moves hashed strings with its own copy of
// sha256 -- proving only that SHA-256 is injective, which was never in doubt.
// A test that reimplements the thing it is testing agrees with itself.
func digestOf(canonical string) string {
	h := sha256.New()
	fmt.Fprint(h, canonical)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// canonical renders every mapping in the package as text, in an order that does
// not depend on map iteration or on registration order.
//
// It is a string rather than bytes fed straight to the hash because a digest
// nobody can explain is a digest nobody trusts: when two builds disagree, the
// question is always *what* differs, and the only useful answer is a diff of
// the thing that was hashed. Exposed to tests through digestInput.
func canonical() string {
	var b strings.Builder

	ds := Dialects()
	sort.Slice(ds, func(i, j int) bool { return ds[i].Name() < ds[j].Name() })

	for _, d := range ds {
		fmt.Fprintf(&b, "dialect %s\n", d.Name())

		if r, ok := d.(Ruled); ok {
			for _, rule := range r.Rules() {
				writeRule(&b, rule)
			}

			unstated := append([]semconv.Field(nil), r.Unstated()...)
			sort.Slice(unstated, func(i, j int) bool { return unstated[i] < unstated[j] })
			for _, f := range unstated {
				fmt.Fprintf(&b, "  unstated %s\n", f)
			}

			sig := append([]string(nil), r.Signature()...)
			sort.Strings(sig)
			for _, k := range sig {
				fmt.Fprintf(&b, "  signature %s\n", k)
			}
		} else {
			// A dialect with no rule table is recorded as such rather than
			// skipped, so that one growing a table is a change to this text
			// and not a silent addition to it.
			b.WriteString("  unruled\n")
		}

		writeInterpretations(&b, d)
	}

	// The targets are part of the mapping, not a separate thing it is pointed
	// at. A field that gains an attribute at a target, or a value set that
	// widens, changes what a span comes out as without any rule having moved.
	for _, t := range semconv.Targets {
		fmt.Fprintf(&b, "target %s\n", t)
		for _, f := range semconv.AllFields {
			key, ok := t.Key(f)
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "  %s -> %s\n", f, key)
			if vals, ok := t.EnumValues(f); ok {
				sorted := append([]string(nil), vals...)
				sort.Strings(sorted)
				fmt.Fprintf(&b, "    enum %s\n", strings.Join(sorted, ","))
			}
		}
	}

	return b.String()
}

// writeInterpretations renders the readings a dialect declares beyond its rules,
// sorted. They are where the meaning of the hand-written half lives: a key read
// into a different field, or through a different value table, or under
// different evidence, is a different mapping even though no Rule moved, and
// before these were declared it left the digest exactly where it was.
func writeInterpretations(b *strings.Builder, d Dialect) {
	in, ok := d.(Interpreted)
	if !ok {
		return
	}
	var lines []string
	for _, i := range in.Interpretations() {
		var lb strings.Builder
		writeInterpretation(&lb, i)
		lines = append(lines, lb.String())
	}
	sort.Strings(lines)
	for _, l := range lines {
		b.WriteString(l)
	}
}

// writeInterpretation renders one reading field by field, zeros included, for
// the reason writeTransform does.
func writeInterpretation(b *strings.Builder, i Interpretation) {
	fmt.Fprintf(b, "  means %s when=%q -> %s\n", i.Key, i.When, i.Meaning)
	fmt.Fprintf(b, "    candidates=%s\n", joinFields(i.Candidates))
	fmt.Fprintf(b, "    byspelling=%t\n", i.BySpelling)
	keys := make([]string, 0, len(i.Values))
	for k := range i.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(b, "    value %s=%s\n", k, i.Values[k])
	}
}

// writeRule renders one rule. Every caller goes through here, tests included,
// so that nothing can agree with a second copy of the format.
func writeRule(b *strings.Builder, r Rule) {
	fmt.Fprintf(b, "  rule %s <- %s\n", r.Field, strings.Join(r.Keys, ","))
	writeTransform(b, r.Transform)
}

// writeTransform renders a Transform field by field. Every field on the struct
// is written, including its zero, so that a field added to Transform and left
// unset everywhere still changes this text once -- and a field added without
// being written here fails TestDigestCoversEveryTransformField.
func writeTransform(b *strings.Builder, t Transform) {
	fmt.Fprintf(b, "    lower=%t\n", t.Lower)
	fmt.Fprintf(b, "    cutafter=%q\n", t.CutAfter)
	fmt.Fprintf(b, "    trimsuffix=%q\n", t.TrimSuffix)

	keys := make([]string, 0, len(t.Map))
	for k := range t.Map {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(b, "    map %s=%s\n", k, t.Map[k])
	}

	fmt.Fprintf(b, "    mapunmatched=%s\n", t.MapUnmatched)
	fmt.Fprintf(b, "    divide=%v\n", t.Divide)
	fmt.Fprintf(b, "    tolist=%t\n", t.ToList)
}
