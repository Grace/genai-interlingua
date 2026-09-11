// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

const sourcesDoc = "../../docs/sources.md"

// derived is the entry written for a field no single attribute can be said to
// have produced. It is a value in the table rather than an empty cell, because
// an empty cell reads as "nobody checked" and this is the opposite of that: the
// mapping was asked and answered that the question has no single answer.
const derived = "_derived_"

// lifted marks a source the value was taken *out of* rather than carried across
// from -- a JSON object holding the number, not the number's previous name. The
// table says which, because "read from braintrust.metrics" and "renamed from
// gen_ai.usage.prompt_tokens" are different claims about the mapping.
const lifted = " (lifted out of)"

// TestSourceAttribution regenerates docs/sources.md from the fixtures and fails
// when the committed file has drifted.
//
// The table says, per dialect, which source attribute each canonical field was
// read from. That is a fact the mapping already knew and used to discard.
//
// It is worth being careful about what this adds, because the exported configs
// under internal/emit already pin part of it: they are serialized from the rule
// tables, so a Rule that starts naming a different key drifts those goldens too.
// Two things are left over, and both are the interesting half.
//
// A rule names *several* keys in precedence order and the export records the
// list. Only running it against real spans records which one won. A dialect
// supporting two vintages of a library reads the newer key where the emitter
// writes it and the older key otherwise, and "which vintage is this corpus
// actually exercising" is a question no static export of the table can answer.
//
// And the mapping work that is not a Rule is invisible to the exports
// altogether -- llm.invocation_parameters, the JSON blob openinference hides
// max_tokens and temperature inside, appears in exactly zero exported files
// (docs/export-gap.md has the counts). Those fields have a source, it is a real
// attribute, and until now nothing recorded it.
//
// The table is also the claim the inspector makes to a reader, one attribute at
// a time. A screen that says "this came from that" and a table generated from
// the same run cannot disagree.
func TestSourceAttribution(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join(testdata, "*", "in.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no fixtures under %s", testdata)
	}
	sort.Strings(inputs)

	// dialect -> canonical key -> source key (or derived) -> seen
	seen := make(map[dialect.Name]map[string]map[string]bool)
	var order []dialect.Name

	for _, input := range inputs {
		payload := mustReadFile(t, input)
		for _, target := range semconv.Targets {
			opts := DefaultOptions()
			opts.Target = target

			for _, s := range spansOf(t, payload) {
				r, ok := Span(s, opts)
				if !ok {
					continue
				}
				if seen[r.Dialect] == nil {
					seen[r.Dialect] = make(map[string]map[string]bool)
					order = append(order, r.Dialect)
				}
				for key := range r.Set {
					if strings.HasPrefix(key, "interlingua.") {
						continue
					}
					f, ok := semconv.FieldForKey(key)
					if !ok {
						continue
					}
					// Keyed by the canonical name rather than the emitted one
					// so a field does not get two rows for saying the same
					// thing at two targets.
					canonical := semconv.CanonicalKey(f)
					if seen[r.Dialect][canonical] == nil {
						seen[r.Dialect][canonical] = make(map[string]bool)
					}
					origin, ok := r.Sources[key]
					from := derived
					if ok {
						from = origin.Key
						if origin.Lifted {
							from += lifted
						}
					}
					seen[r.Dialect][canonical][from] = true
				}
			}
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })

	got := renderSources(order, seen)
	if *update {
		if err := os.WriteFile(sourcesDoc, []byte(got), 0o644); err != nil {
			t.Fatalf("write %s: %v", sourcesDoc, err)
		}
		return
	}
	if want := string(mustReadFile(t, sourcesDoc)); got != want {
		t.Errorf("%s has drifted from what the fixtures produce\n%s\nrerun with -update to regenerate",
			sourcesDoc, firstDifference([]byte(want), []byte(got)))
	}
}

// TestEveryAttributionNamesAKeyTheSpanCarried is the check that stops the
// attribution from being decorative.
//
// An audit trail is only worth the weakest claim in it, and the easy way to
// produce a complete-looking one is to name a plausible key whenever a real one
// is not at hand. That failure is invisible in the generated table -- every row
// is filled in, every key looks like something the emitter might write -- and it
// is exactly what a reader would rely on when deciding whether a number means
// what they think.
//
// So: every key an attribution names must be a key the span actually carried.
// Nothing here checks that the value is right, because nothing can; what it
// checks is that the mapping is pointing at something that was there.
func TestEveryAttributionNamesAKeyTheSpanCarried(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join(testdata, "*", "in.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no fixtures under %s", testdata)
	}

	attributed := 0
	for _, input := range inputs {
		for _, s := range spansOf(t, mustReadFile(t, input)) {
			p, ok := dialect.Parse(s)
			if !ok {
				continue
			}
			for f, origin := range p.Source {
				attributed++
				if !s.Has(origin.Key) {
					t.Errorf("%s: %s is attributed to %q, which the span does not carry",
						filepath.Base(filepath.Dir(input)), f, origin.Key)
				}
				if _, set := p.Fields[f]; !set {
					t.Errorf("%s: %s is attributed to %q but was never set",
						filepath.Base(filepath.Dir(input)), f, origin.Key)
				}
			}
		}
	}
	// Attributing nothing would satisfy every assertion above, so the corpus is
	// required to have exercised the thing being tested.
	if attributed == 0 {
		t.Fatal("no field in the corpus was attributed to any key; setFrom is not wired up")
	}
	t.Logf("%d attributions checked", attributed)
}

// TestNoAttributionNamesAnIndexedKey pins the other half of the line
// internal/dialect/rules.go draws.
//
// Messages and tool definitions are usually built by reading a run of indexed
// attributes -- gen_ai.prompt.{i}.role, .content, .tool_calls.{j}.* -- and
// folding them into one document. No single one of those keys produced the
// result, and naming one of them would present the first element of a list as
// the origin of the whole list: an attribution that points at something real,
// passes every check above, and is still a lie about what happened.
//
// The invariant is therefore about the shape of the key, not the name of the
// field. A field CAN legitimately be attributed to gen_ai.input.messages,
// because LiteLLM and the conformant fallback write the whole document under
// that one key -- that is a rename, and the table says so. What must never
// happen is an attribution to a key with an index in it.
func TestNoAttributionNamesAnIndexedKey(t *testing.T) {
	indexed := regexp.MustCompile(`\.\d+(\.|$)`)

	inputs, err := filepath.Glob(filepath.Join(testdata, "*", "in.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}

	// The corpus has to actually contain indexed attributes, or this passes by
	// never meeting the case it exists to catch.
	sawIndexed := false
	for _, input := range inputs {
		for _, s := range spansOf(t, mustReadFile(t, input)) {
			for _, k := range s.Keys() {
				if indexed.MatchString(k) {
					sawIndexed = true
				}
			}
			p, ok := dialect.Parse(s)
			if !ok {
				continue
			}
			for f, origin := range p.Source {
				if indexed.MatchString(origin.Key) {
					t.Errorf("%s: %s is attributed to %q, one element of an indexed run, "+
						"but the field holds the whole of it",
						filepath.Base(filepath.Dir(input)), f, origin.Key)
				}
			}
		}
	}
	if !sawIndexed {
		t.Fatal("no fixture carries an indexed attribute; this test proves nothing")
	}
}

// renderSources builds the table. One row per canonical field per dialect, with
// every source that field was read from across the corpus, because an emitter
// that changed a spelling between versions has two and the table should say so
// rather than pick one.
func renderSources(order []dialect.Name, seen map[dialect.Name]map[string]map[string]bool) string {
	var b strings.Builder

	b.WriteString(`# Where each field came from

Generated by ` + "`TestSourceAttribution`" + ` in ` + "`internal/normalize`" + `. Do not edit by hand:
` + "`go test ./internal/normalize -update`" + ` rewrites it, and CI fails when this file
and the fixtures disagree.

For every canonical field a dialect carried, this names the attribute the value
was read from. ` + derived + ` means no single attribute produced it: the value was
reassembled from several, or read off the span's shape rather than any of its
values. That is a different statement from a blank, and there are no blanks here
-- a field with no row was not carried at all, which ` + "`conformance.md`" + ` covers.

Read a row as a claim about the mapping, not about the emitter. It says which
key this repository chose to trust for that field, which is exactly the decision
worth reviewing.

`)

	for _, d := range order {
		fmt.Fprintf(&b, "## %s\n\n", d)
		b.WriteString("| Field | Read from |\n|---|---|\n")

		fields := make([]string, 0, len(seen[d]))
		for f := range seen[d] {
			fields = append(fields, f)
		}
		sort.Strings(fields)

		for _, f := range fields {
			froms := make([]string, 0, len(seen[d][f]))
			for s := range seen[d][f] {
				froms = append(froms, s)
			}
			sort.Strings(froms)
			for i, s := range froms {
				if s != derived {
					froms[i] = "`" + s + "`"
				}
			}
			fmt.Fprintf(&b, "| `%s` | %s |\n", f, strings.Join(froms, ", "))
		}
		b.WriteString("\n")
	}

	return b.String()
}
