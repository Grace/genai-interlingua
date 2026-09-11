// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// TestEveryStatedValueIsStillRecoverable is the strong form of this
// repository's central claim.
//
// TestNormalizationNeverRemovesAnAttribute holds the structural half: no key
// disappears. That is not enough. Where a library already writes the
// conventions' own attribute name but a value outside its value set, the rule
// rewrites the value *in place* -- the key survives and the value does not, and
// keeping originals cannot help because the name is identical, so there is no
// second attribute to leave alone.
//
// The Vercel AI SDK writes gen_ai.response.finish_reasons = ["tool-calls"] and
// the conventions say tool_calls. Normalizing destroys "tool-calls": a query
// counting it reads zero afterwards, and nothing on the span says it ever
// existed. COMPATIBILITY.md documents the class of change under "Changes that
// are not losses", but a statement in a document is not a record on a span.
//
// So: every value an emitter stated must still be readable off the normalized
// span, either at its own key or from the record of what replaced it.
func TestEveryStatedValueIsStillRecoverable(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join(testdata, "*", "in.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no fixtures under %s", testdata)
	}

	// Dedupe is held to the same claim as keep, because never destroying what the
	// emitter stated is the whole of its promise. It may remove a key only when
	// the value is still on the span under a conventions name.
	checked, deduped := 0, 0
	for _, mode := range []Originals{OriginalsKeep, OriginalsDedupe} {
		for _, input := range inputs {
			name := filepath.Base(filepath.Dir(input))
			data := mustReadFile(t, input)

			for _, target := range semconv.Targets {
				opts := DefaultOptions()
				opts.Target = target
				opts.Originals = mode

				out, err := Payload(data, opts)
				if err != nil {
					t.Fatalf("%s: normalize: %v", name, err)
				}

				before := bySpan(t, data)
				after := bySpan(t, out)

				for span, was := range before {
					now, ok := after[span]
					if !ok {
						t.Errorf("%s at %s (originals %s): span %q is gone", name, target, mode, span)
						continue
					}
					for key, old := range was {
						checked++
						if got, ok := now[key]; ok && sameValue(got, old) {
							continue // still there, unchanged
						}
						if recoverable(now, key, old) {
							continue // replaced, and the replacement said so
						}
						if mode == OriginalsDedupe && copied(now, old) {
							deduped++
							continue // removed as a duplicate, and the duplicate is there
						}
						t.Errorf("%s at %s (originals %s): span %q stated %s = %s, and after "+
							"normalization that value is neither at its own key nor recorded anywhere",
							name, target, mode, span, key, show(old))
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no attribute was checked; this test proves nothing")
	}
	if deduped == 0 {
		t.Fatal("dedupe removed nothing across the corpus, so its half of this test proves nothing")
	}
}

// recoverable reports whether the span records the value that used to be at key.
func recoverable(now map[string]dialect.Value, key string, old dialect.Value) bool {
	v, ok := now[AttrReplaced+key]
	return ok && sameValue(v, old)
}

// copied reports whether the span carries this exact value under a conventions
// name, which is the only thing that licenses dedupe to have removed it.
func copied(now map[string]dialect.Value, old dialect.Value) bool {
	for k, v := range now {
		if strings.HasPrefix(k, "gen_ai.") && sameValue(v, old) {
			return true
		}
	}
	return false
}

func bySpan(t *testing.T, data []byte) map[string]map[string]dialect.Value {
	t.Helper()
	out := map[string]map[string]dialect.Value{}
	for _, s := range spansOf(t, data) {
		if out[s.Name] == nil {
			out[s.Name] = map[string]dialect.Value{}
		}
		for k, v := range s.Attributes {
			out[s.Name][k] = v
		}
	}
	return out
}

func sameValue(a, b dialect.Value) bool { return show(a) == show(b) }

func show(v dialect.Value) string {
	return fmt.Sprintf("%d\x00%s", v.Kind, displayOf(v))
}
