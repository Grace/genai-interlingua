// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAnUnreadableValueIsNamedRatherThanSilentlyOverwritten covers the gap that
// made the replaced-value record unsound.
//
// The OTLP codec reads the scalar shapes and returns an empty Value for the
// rest: a kvlist, a bytes value, an empty array, an array of mixed types. The
// guard that records what a rewrite replaced asked Span.Attr whether the key was
// there, and Attr reports false for all of those -- so no record was written.
// apply() still dropped the emitter's value, because the key is in Set. The
// emitter's structured value was destroyed with interlingua.lossy.count saying
// zero, which is the one outcome the record exists to prevent.
//
// The realistic instance is not a kvlist. It is an empty array at
// gen_ai.response.finish_reasons, which is what a provider that returns no
// finish reason emits, and the Vercel rule rewrites that key in place.
//
// No fixture under testdata/ carries one, which is why every test passed. This
// builds the payload rather than adding a fixture, because it is a shape to
// defend against rather than a capture anybody took.
func TestAnUnreadableValueIsNamedRatherThanSilentlyOverwritten(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{"empty array", `{"arrayValue":{"values":[]}}`},
		{"mixed array", `{"arrayValue":{"values":[{"stringValue":"stop"},{"intValue":"7"}]}}`},
		{"kvlist", `{"kvlistValue":{"values":[{"key":"a","value":{"stringValue":"keepme"}}]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := []byte(`{"resourceSpans":[{"scopeSpans":[{"spans":[{
				"name":"ai.generateText.doGenerate","kind":3,
				"attributes":[
					{"key":"ai.operationId","value":{"stringValue":"ai.generateText.doGenerate"}},
					{"key":"ai.model.provider","value":{"stringValue":"openai.chat"}},
					{"key":"ai.model.id","value":{"stringValue":"gpt-4o-mini"}},
					{"key":"ai.response.finishReason","value":{"stringValue":"tool-calls"}},
					{"key":"gen_ai.response.finish_reasons","value":` + tc.value + `}
				]}]}]}]}`)

			out, err := Payload(in, DefaultOptions())
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}

			attrs := map[string]json.RawMessage{}
			var p payload
			if err := json.Unmarshal(out, &p); err != nil {
				t.Fatalf("decode: %v", err)
			}
			for _, rs := range p.ResourceSpans {
				for _, ss := range rs.ScopeSpans {
					for _, s := range ss.Spans {
						for _, kv := range s.Attributes {
							b, _ := json.Marshal(kv.Value)
							attrs[kv.Key] = b
						}
					}
				}
			}

			// The precondition: the rule really did overwrite this key. Without
			// it the test would pass on a span nothing touched.
			got, ok := attrs["gen_ai.response.finish_reasons"]
			if !ok {
				t.Fatal("the key is gone entirely; this fixture no longer exercises an in-place rewrite")
			}
			// The rule maps ai.response.finishReason onto this key, so the
			// emitter's own value at the same key is overwritten. If the output
			// no longer carries the rule's value, the fixture stopped
			// exercising an in-place rewrite and the test proves nothing.
			if !strings.Contains(string(got), "tool_calls") {
				t.Skipf("no in-place rewrite happened; got %s", got)
			}

			// The whole point: the span must say something was destroyed.
			lossy, ok := attrs["interlingua.lossy"]
			if !ok {
				t.Fatalf("a stated value was overwritten and interlingua.lossy names nothing.\n"+
					"written: %v", keysOf(attrs))
			}
			if !strings.Contains(string(lossy), "gen_ai.response.finish_reasons") {
				t.Errorf("interlingua.lossy does not name the overwritten key: %s", lossy)
			}
			if c := string(attrs["interlingua.lossy.count"]); strings.Contains(c, `"0"`) {
				t.Error("interlingua.lossy.count is 0 on a span that lost a stated value")
			}
		})
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
