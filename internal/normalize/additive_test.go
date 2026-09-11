// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// TestReserializeIsIdempotent pins the printer.
//
// Reserialize is only useful if running it twice changes nothing: a caller
// diffing its output against Payload's is relying on both sides having been
// through the same fixed point. If the encoder were unstable -- a map iterated
// somewhere, a number formatted differently on a second pass -- the diff would
// show churn nobody caused.
func TestReserializeIsIdempotent(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join(testdata, "*", "in.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no fixtures under %s", testdata)
	}

	for _, input := range inputs {
		name := filepath.Base(filepath.Dir(input))
		once, err := Reserialize(mustReadFile(t, input))
		if err != nil {
			t.Fatalf("%s: reserialize: %v", name, err)
		}
		twice, err := Reserialize(once)
		if err != nil {
			t.Fatalf("%s: reserialize twice: %v", name, err)
		}
		if string(once) != string(twice) {
			t.Errorf("%s: Reserialize is not a fixed point\n%s",
				name, firstDifference(once, twice))
		}
	}
}

// TestNormalizationNeverRemovesAnAttribute is the invariant the inspector's
// diff rests on, and it is narrower than it first looks.
//
// The tempting claim is that with originals preserved this package only ever
// adds. It does not, and asserting that fails immediately: where a library
// already writes the conventions' own attribute name but a non-conforming
// value, the rule rewrites the value in place. The Vercel SDK's
// gen_ai.response.finish_reasons holds "tool-calls" and the conventions say
// "tool_calls"; LangChain states an operation the value set does not have.
// Those are edits, not additions, and the diff on the page will show a removed
// line for each -- correctly.
//
// What must never happen is a key going away. Nothing here deletes an
// attribute on the grounds that nobody looked at it, and a source key a dialect
// read is left in place beside the conventions spelling it produced. A rule
// gaining a Remove, or a dialect deciding to tidy up after itself, would turn
// the demo's "here is what the translator did" into "here is what it did, plus
// some things it quietly dropped" -- and the page would render the deletion as
// calmly as it renders an addition.
//
// Compared structurally rather than over the rendered text, because the
// question is about attributes and not about lines. A value rewritten in place
// changes a line while removing no key, which is exactly the case that has to
// pass.
func TestNormalizationNeverRemovesAnAttribute(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join(testdata, "*", "in.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no fixtures under %s", testdata)
	}

	compared := 0
	for _, input := range inputs {
		name := filepath.Base(filepath.Dir(input))
		data := mustReadFile(t, input)
		before := keysBySpan(t, data)

		for _, target := range semconv.Targets {
			opts := DefaultOptions()
			opts.Target = target
			if opts.Originals != OriginalsKeep {
				t.Fatal("DefaultOptions no longer keeps originals; this test assumes it does")
			}

			out, err := Payload(data, opts)
			if err != nil {
				t.Fatalf("%s: normalize: %v", name, err)
			}
			after := keysBySpan(t, out)
			compared++

			for span, keys := range before {
				got, ok := after[span]
				if !ok {
					t.Errorf("%s at %s: span %q is gone after normalization", name, target, span)
					continue
				}
				for k := range keys {
					if !got[k] {
						t.Errorf("%s at %s: span %q lost the attribute %q", name, target, span, k)
					}
				}
			}
		}
	}
	if compared == 0 {
		t.Fatal("nothing was compared; this test proves nothing")
	}
}

// keysBySpan is the attribute key set of every span in a payload, by span name.
func keysBySpan(t *testing.T, data []byte) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}
	for _, s := range spansOf(t, data) {
		if out[s.Name] == nil {
			out[s.Name] = map[string]bool{}
		}
		for _, k := range s.Keys() {
			out[s.Name][k] = true
		}
	}
	return out
}

// TestNoInputLineDisappears is the strongest form of the claim the inspector's
// diff makes to a reader, and the cheapest to check.
//
// TestNormalizationNeverRemovesAnAttribute holds that no attribute KEY goes
// away. That leaves room for a value to be rewritten under a key that stays,
// which does happen -- the Vercel SDK writes gen_ai.response.finish_reasons =
// ["tool-calls"] where the conventions say tool_calls -- and would show as a
// removed line.
//
// It does not, because such a value is kept as interlingua.replaced.<key>. So
// for this corpus the translation is purely additive at the level of rendered
// text, and the page says so. If that stops being true, the page is making a
// claim the code does not support, and this fails.
//
// Line sets rather than a diff algorithm: the question is whether a line that
// was there is gone, not where it moved to. Both sides go through Reserialize so
// that field ordering cannot be mistaken for a change.
func TestNoInputLineDisappears(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join(testdata, "*", "in.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no fixtures under %s", testdata)
	}

	for _, input := range inputs {
		name := filepath.Base(filepath.Dir(input))
		data := mustReadFile(t, input)

		before, err := Reserialize(data)
		if err != nil {
			t.Fatalf("%s: reserialize: %v", name, err)
		}
		after, err := Payload(data, DefaultOptions())
		if err != nil {
			t.Fatalf("%s: normalize: %v", name, err)
		}

		have := map[string]int{}
		for _, l := range strings.Split(string(after), "\n") {
			have[l]++
		}
		for _, l := range strings.Split(string(before), "\n") {
			if have[l] == 0 {
				t.Errorf("%s: the line %q is in the span before normalization and not after; "+
					"the inspector tells readers the diff is purely additive",
					name, strings.TrimSpace(l))
				continue
			}
			have[l]--
		}
	}
}
