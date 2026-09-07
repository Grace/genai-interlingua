package normalize

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// FuzzPayload fuzzes the hand-written OTLP/JSON codec.
//
// This is the one part of the repository that reads input nobody here wrote.
// In a Collector it is fed by the network, and a panic in a processor is not a
// rejected request -- it takes down a pipeline carrying every span in the
// service. A hand-written 250-line decoder that has only ever been pointed at
// well-formed fixtures has not been tested against the thing it will actually
// meet.
//
// Not panicking is the implicit property. Two more are asserted, and the second
// is the one likely to catch something real.
func FuzzPayload(f *testing.F) {
	inputs, err := filepath.Glob(filepath.Join(testdata, "*", "in.json"))
	if err != nil {
		f.Fatalf("glob fixtures: %v", err)
	}
	for _, in := range inputs {
		f.Add(mustReadFile(f, in))
	}

	// Shapes the fixtures cannot reach: empty, null-valued, an attribute with a
	// key and no value, a value union with nothing set, and input that is not
	// JSON at all.
	for _, seed := range []string{
		`{}`,
		`{"resourceSpans":null}`,
		`{"resourceSpans":[]}`,
		`{"resourceSpans":[{"scopeSpans":[{"spans":[{"attributes":[{"key":"k"}]}]}]}]}`,
		`{"resourceSpans":[{"scopeSpans":[{"spans":[{"attributes":[{"key":"k","value":{}}]}]}]}]}`,
		`{"resourceSpans":[{"scopeSpans":[{"spans":[{"startTimeUnixNano":"not-a-number"}]}]}]}`,
		`not json`,
		``,
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		out, err := Payload(data, DefaultOptions())
		if err != nil {
			// Refusing malformed input is correct. What matters is that it
			// refuses rather than panics, which returning at all demonstrates.
			return
		}

		// Anything this codec claims to have encoded must be decodable.
		var check any
		if jsonErr := json.Unmarshal(out, &check); jsonErr != nil {
			t.Fatalf("Payload produced output that is not JSON: %v", jsonErr)
		}

		// The normalizer rewrites attributes. It must never drop a span or
		// invent one: a pipeline that silently loses spans is worse than one
		// that fails loudly, and this is the invariant most likely to catch a
		// mistake in the codec's enumeration of the span message.
		if got, want := countSpans(t, out), countSpans(t, data); got != want {
			t.Fatalf("span count changed: %d in, %d out", want, got)
		}
	})
}

// countSpans counts spans without going through the codec under test, so a bug
// in that codec cannot hide by miscounting both sides identically.
func countSpans(t *testing.T, data []byte) int {
	t.Helper()
	var doc struct {
		ResourceSpans []struct {
			ScopeSpans []struct {
				Spans []json.RawMessage `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		// Only reachable for the input side, and only if Payload accepted
		// something encoding/json rejects. That would itself be a finding.
		t.Fatalf("count spans: %v", err)
	}
	n := 0
	for _, rs := range doc.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			n += len(ss.Spans)
		}
	}
	return n
}
