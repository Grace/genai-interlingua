package normalize

import (
	"encoding/json"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// A span can be normalized twice. A collector normalizes at the edge and a
// backend normalizes again at ingest; a pipeline is rebuilt and the processor
// ends up in it twice; somebody re-processes an archive against a newer target.
// None of that is exotic and all of it is invisible in the result.
//
// These tests characterize what this package currently does about that, which
// is nothing, and they are written to fail once it does something. They assert
// the defect rather than the requirement, and the comment on each says which is
// which, because a test that pins wrong behaviour without saying so is worse
// than no test.

// hop runs one normalization over a payload and returns the output, so the
// tests below read as a pipeline rather than as plumbing.
func hop(t *testing.T, data []byte, target semconv.Target, preserve bool) []byte {
	t.Helper()
	opts := DefaultOptions()
	opts.Target = target
	opts.PreserveOriginal = preserve
	out, err := Payload(data, opts)
	if err != nil {
		t.Fatalf("normalize to %s: %v", target, err)
	}
	return out
}

// attrsOf returns one span's attributes by span name, flattened to comparable
// Go values.
func attrsOf(t *testing.T, data []byte, spanName string) map[string]any {
	t.Helper()
	var p payload
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, rs := range p.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				if sp.Name != spanName {
					continue
				}
				out := make(map[string]any, len(sp.Attributes))
				for _, a := range sp.Attributes {
					switch {
					case a.Value.StringValue != nil:
						out[a.Key] = *a.Value.StringValue
					case a.Value.IntValue != nil:
						out[a.Key] = int64(*a.Value.IntValue)
					case a.Value.ArrayValue != nil:
						var vs []string
						for _, e := range a.Value.ArrayValue.Values {
							if e.StringValue != nil {
								vs = append(vs, *e.StringValue)
							}
						}
						out[a.Key] = vs
					}
				}
				return out
			}
		}
	}
	t.Fatalf("no span named %q", spanName)
	return nil
}

// TestSecondHopErasesTheFirst is the defect, and it is the sharpest thing in
// this repository's favour precisely because it is against it.
//
// Normalize an OpenLLMetry span to v1.41.0 with originals stripped, then
// normalize that output to genai-main. After the second hop the span says it
// was emitted by "raw" and that it lost nothing. Both are false: it came from
// OpenLLMetry, and three attributes were dropped one hop earlier.
//
// The span now carries a clean bill of health it did not earn. That is exactly
// the failure this repository exists to name -- a translation layer that drops
// data silently is worse than no translation layer, because you will trust the
// result -- and it is committed here, by this code, today.
//
// WHEN THE FIX LANDS THIS TEST INVERTS. The diff on this file is the proof that
// the chain works, so it is kept and rewritten rather than deleted.
func TestSecondHopErasesTheFirst(t *testing.T) {
	in := mustReadFile(t, testdata+"/openllmetry/in.json")

	first := hop(t, in, semconv.TargetV1_41_0, false)
	second := hop(t, first, semconv.TargetGenAIMain, false)

	a1 := attrsOf(t, first, "openai.chat")
	a2 := attrsOf(t, second, "openai.chat")

	// After one hop, the record is accurate.
	if got, want := a1[AttrDialect], "openllmetry"; got != want {
		t.Fatalf("hop 1 dialect = %v, want %v", got, want)
	}
	if got := a1[AttrLossyCount].(int64); got == 0 {
		t.Fatalf("hop 1 recorded no losses; this fixture is supposed to have some")
	}
	firstLosses := a1[AttrLossyCount].(int64)

	// After two, it is not. Both assertions below are the defect.
	if got, want := a2[AttrDialect], "raw"; got != want {
		t.Errorf("DEFECT CHANGED: hop 2 dialect = %v, expected the defective %v."+
			"\n    If a fix landed, this test should be rewritten to assert the chain"+
			"\n    preserves openllmetry as the first hop's source.", got, want)
	}
	if got := a2[AttrLossyCount].(int64); got != 0 {
		t.Errorf("DEFECT CHANGED: hop 2 lossy count = %d, expected the defective 0."+
			"\n    If a fix landed, this should now be >= %d, because the first hop's"+
			"\n    losses are part of this span's history and did not stop being true.",
			got, firstLosses)
	}

	// This part is FIXED and is asserted as a requirement rather than pinned as
	// a defect. It used to be the sharpest failure of the three: the count is
	// written on every hop including when zero, the array only when non-empty,
	// so a second hop that lost nothing overwrote the count with 0 and left the
	// first hop's array in place. The span reported losing nothing while naming
	// three things it lost.
	//
	// A span that contradicts itself is worse than one that is merely
	// incomplete. Each pass now clears the attributes it is not writing, so the
	// count and the list always agree.
	stale, hasArray := a2[AttrLossy].([]string)
	if hasArray {
		t.Errorf("REGRESSION: hop 2 carries a stale %s from hop 1: %v"+
			"\n    Each pass must clear the interlingua.* attributes it is not writing,"+
			"\n    or the span reports one thing in the count and another in the list.",
			AttrLossy, stale)
	}
	if got := a2[AttrLossyCount].(int64); got != 0 {
		t.Errorf("REGRESSION: %s = %d with no %s array; they must agree", AttrLossyCount, got, AttrLossy)
	}
}

// TestPreservingOriginalsHidesItRatherThanFixingIt is the same defect wearing a
// disguise, and it is worth pinning separately because the disguise is the
// default configuration.
//
// With originals preserved, the second hop re-detects the original emitter from
// the surviving ai.*/traceloop.* attributes and redoes the whole translation
// from scratch. The output looks right. But it is repetition rather than
// composition, and the consequence is that a span translated once and a span
// translated twice are byte-identical: nothing records how many times the data
// was rewritten, or through what path.
//
// It looks harmless here only because both hops happen to reach the same
// answer. Change either target and they do not.
func TestPreservingOriginalsHidesItRatherThanFixingIt(t *testing.T) {
	in := mustReadFile(t, testdata+"/openllmetry/in.json")

	once := hop(t, in, semconv.TargetGenAIMain, true)
	twice := hop(t, hop(t, in, semconv.TargetV1_41_0, true), semconv.TargetGenAIMain, true)

	a1 := attrsOf(t, once, "openai.chat")
	a2 := attrsOf(t, twice, "openai.chat")

	if a1[AttrDialect] != a2[AttrDialect] ||
		a1[AttrTarget] != a2[AttrTarget] ||
		a1[AttrLossyCount] != a2[AttrLossyCount] {
		t.Errorf("DEFECT CHANGED: one hop and two hops now differ, which means something"+
			"\n    records the difference.\n    once:  %v\n    twice: %v", a1, a2)
	}

	// Stated as its own assertion because it is the whole point: these two
	// spans had different histories and the telemetry cannot tell them apart.
	if _, ok := a2["interlingua.translations"]; ok {
		t.Errorf("DEFECT CHANGED: a translation chain exists; rewrite this test to assert" +
			"\n    that two hops produce two records and one hop produces one.")
	}
}
