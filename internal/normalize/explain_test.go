// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// TestExplainAgreesWithPayload ties the account to the act.
//
// Explain and Payload are separate entry points over the same Result, and that
// is exactly the arrangement where a page can end up describing a normalization
// that did not happen. The inspector's whole claim is that what it shows is
// what the tool did; if Explain drifted from Payload, the screen would keep
// making that claim and stop being right about it, silently, because nothing
// else in the repository looks at both.
//
// So: every attribute Explain says was written must be on the normalized span,
// with nothing left over.
func TestExplainAgreesWithPayload(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join(testdata, "*", "in.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no fixtures under %s", testdata)
	}

	for _, input := range inputs {
		name := filepath.Base(filepath.Dir(input))
		for _, target := range semconv.Targets {
			t.Run(name+"/"+string(target), func(t *testing.T) {
				opts := DefaultOptions()
				opts.Target = target
				data := mustReadFile(t, input)

				explained, err := Explain(data, opts)
				if err != nil {
					t.Fatalf("explain: %v", err)
				}
				if len(explained) == 0 {
					t.Skip("no dialect claimed any span in this fixture")
				}

				out, err := Payload(data, opts)
				if err != nil {
					t.Fatalf("normalize: %v", err)
				}

				// Keyed by span name, which is what an Explanation carries.
				// Fixtures with two spans of the same name would collide here;
				// none do, and this fails loudly if one ever does.
				onSpan := map[string]map[string]bool{}
				var p payload
				if err := json.Unmarshal(out, &p); err != nil {
					t.Fatalf("decode normalized payload: %v", err)
				}
				for _, rs := range p.ResourceSpans {
					for _, ss := range rs.ScopeSpans {
						for _, s := range ss.Spans {
							if _, dup := onSpan[s.Name]; dup {
								t.Fatalf("two spans named %q; this test cannot tell them apart", s.Name)
							}
							keys := map[string]bool{}
							for _, kv := range s.Attributes {
								keys[kv.Key] = true
							}
							onSpan[s.Name] = keys
						}
					}
				}

				for _, e := range explained {
					keys, ok := onSpan[e.Span]
					if !ok {
						t.Errorf("Explain describes span %q, which the normalized payload does not contain", e.Span)
						continue
					}
					for _, a := range e.Attributes {
						if !keys[a.Key] {
							t.Errorf("%s: Explain says %s was written, and it is not on the span",
								e.Span, a.Key)
						}
					}
					if e.Mapping == "" {
						t.Errorf("%s: no mapping digest", e.Span)
					}
				}
			})
		}
	}
}

// TestExplainMarksDerivedRatherThanLeavingItBlank guards the distinction the
// whole design rests on. A field with no source is derived, and the JSON says
// so in a field of its own, so that a consumer cannot read the absence of
// "from" as an oversight -- which is the same mistake docs/not-a-score.md
// argues about blanks in the conformance table.
func TestExplainMarksDerivedRatherThanLeavingItBlank(t *testing.T) {
	opts := DefaultOptions()
	// The legacy OpenLLMetry capture is the one that still reassembles messages
	// from indexed gen_ai.prompt.{i}.* attributes, so a span in it carries all
	// three kinds of attribution at once. The modern capture writes
	// gen_ai.input.messages whole, which is a rename and has a source -- which
	// is the whole reason the two fixtures are both kept.
	explained, err := Explain(mustReadFile(t, filepath.Join(testdata, "openllmetry-legacy", "in.json")), opts)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}

	var derived, read, own int
	for _, e := range explained {
		for _, a := range e.Attributes {
			switch {
			case a.Own:
				own++
				if a.From != "" || a.Derived {
					t.Errorf("%s is the translator's own attribute and should claim neither", a.Key)
				}
			case a.Derived:
				derived++
				if a.From != "" {
					t.Errorf("%s is marked derived and also claims to come from %q", a.Key, a.From)
				}
			default:
				read++
				if a.From == "" {
					t.Errorf("%s is neither derived nor own and names no source", a.Key)
				}
			}
		}
	}
	if derived == 0 || read == 0 || own == 0 {
		t.Fatalf("fixture no longer exercises all three cases: derived=%d read=%d own=%d",
			derived, read, own)
	}
}

// TestExplainNeverEncodesNull is the general form of a bug that reached a
// reader as a blank page.
//
// encoding/json writes a nil slice as null. A consumer doing the obvious thing
// with a list -- reading its length, iterating it -- gets a type error instead
// of an empty result, and in a browser that unmounts the page and leaves an
// empty document with the failure only in a console nobody has open. The
// specific instance was Explanation.Lossy on Vercel's ai.toolCall span, which
// is the only first span in the corpus that loses nothing.
//
// Asserting on those three fields would fix the instance. This asserts on the
// encoding, so the next slice added to Explanation or Attribution is covered by
// the commit that adds it rather than by somebody noticing a white screen.
//
// It is a Go test rather than a TypeScript one on purpose. The page's types
// already declared these as arrays, under strict TypeScript with
// noUncheckedIndexedAccess, and were believed -- a declaration about what
// crosses a JSON boundary is a claim, not a check, and the compiler cannot tell
// the difference. The only place this can be enforced is where the bytes are
// produced.
func TestExplainNeverEncodesNull(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join(testdata, "*", "in.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no fixtures under %s", testdata)
	}

	checked := 0
	for _, input := range inputs {
		name := filepath.Base(filepath.Dir(input))
		for _, target := range semconv.Targets {
			opts := DefaultOptions()
			opts.Target = target

			explained, err := Explain(mustReadFile(t, input), opts)
			if err != nil {
				t.Fatalf("%s: explain: %v", name, err)
			}
			for _, e := range explained {
				checked++
				b, err := json.Marshal(e)
				if err != nil {
					t.Fatalf("%s: marshal: %v", name, err)
				}
				// Walked rather than string-matched: a null inside a message
				// body would fail a substring check while being a perfectly
				// ordinary character sequence in someone's prompt.
				if path := findNull(json.RawMessage(b), ""); path != "" {
					t.Errorf("%s/%s span %q: %s encodes as null, and a consumer "+
						"reading it as a list gets a type error rather than an empty one",
						name, target, e.Span, path)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no span was explained; this test proves nothing")
	}
}

// findNull returns the path to the first null it finds, or "" if there is none.
func findNull(raw json.RawMessage, path string) string {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return ""
	}
	switch v := decoded.(type) {
	case nil:
		if path == "" {
			return "the document"
		}
		return path
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b, err := json.Marshal(v[k])
			if err != nil {
				continue
			}
			if p := findNull(b, path+"."+k); p != "" {
				return p
			}
		}
	case []any:
		for i, el := range v {
			b, err := json.Marshal(el)
			if err != nil {
				continue
			}
			if p := findNull(b, fmt.Sprintf("%s[%d]", path, i)); p != "" {
				return p
			}
		}
	}
	return ""
}

// TestExplainShowsAValueThatATransformChanged keeps the one column on the page
// that is about values rather than names.
//
// FromValue is set only where a transform ran, which makes it the field most
// likely to quietly stop being populated: a refactor that dropped it would leave
// every ordinary rename looking correct, and only the handful of rows that are
// actually interesting would go silently blank. So this asserts a specific known
// transform end to end rather than counting non-empty fields.
func TestExplainShowsAValueThatATransformChanged(t *testing.T) {
	opts := DefaultOptions()
	// The Vercel AI SDK writes ai.model.provider as <provider>.<api surface>,
	// and the rule keeps the segment before the dot. Both halves of that are
	// worth seeing on screen, and neither is visible anywhere else.
	explained, err := Explain(mustReadFile(t, filepath.Join(testdata, "vercel", "in.json")), opts)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}

	var found *Attribution
	for _, e := range explained {
		for i, a := range e.Attributes {
			if a.Key == "gen_ai.provider.name" && a.From == "ai.model.provider" {
				found = &e.Attributes[i]
			}
		}
	}
	if found == nil {
		t.Fatal("no span mapped ai.model.provider to gen_ai.provider.name; " +
			"the fixture or the rule changed and this test no longer checks a transform")
	}
	if found.Value != "openai" {
		t.Errorf("gen_ai.provider.name = %q, want %q", found.Value, "openai")
	}
	if found.FromValue != "openai.chat" {
		t.Errorf("FromValue = %q, want %q -- the transform is invisible without it",
			found.FromValue, "openai.chat")
	}

	// And the other direction: a plain rename must NOT set FromValue, or the
	// page shows the same string twice on almost every row.
	for _, e := range explained {
		for _, a := range e.Attributes {
			if a.From != "" && a.FromValue != "" && a.FromValue == a.Value {
				t.Errorf("%s: FromValue repeats Value (%q) instead of being omitted", a.Key, a.Value)
			}
		}
	}
}
