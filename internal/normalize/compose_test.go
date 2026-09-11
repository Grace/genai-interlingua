// SPDX-License-Identifier: Apache-2.0

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
// These tests used to characterize what this package did about that, which was
// nothing, and were written to fail once it did something. They now assert the
// requirement instead of the defect, and each one says what it used to claim,
// because the history is the argument: this repository found the failure in
// itself before anybody else had to.
//
// Three rules make it hold, and none of them is a chain of per-hop records. A
// span already at the requested target is left alone. The dialect is written
// once and never rewritten. The loss list merges rather than replaces. One
// integer, interlingua.hops, says how many times it happened -- an integer
// because every backend measured stores an array attribute as an opaque
// string, so a list of hop records would be unqueryable exactly where it would
// be read.

// hop runs one normalization over a payload and returns the output, so the
// tests below read as a pipeline rather than as plumbing.
func hop(t *testing.T, data []byte, target semconv.Target, originals Originals) []byte {
	t.Helper()
	opts := DefaultOptions()
	opts.Target = target
	opts.Originals = originals
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

// TestSecondHopKeepsTheFirstHopsRecord was TestSecondHopErasesTheFirst, and the
// rename is the point of the file.
//
// It used to assert the defect. Normalize an OpenLLMetry span to v1.41.0 with
// originals stripped, then normalize that output to genai-main, and the span
// came out claiming it was emitted by "raw" and that it had lost nothing. Both
// were false: it came from OpenLLMetry, and three attributes were dropped one
// hop earlier. The span carried a clean bill of health it had not earned, which
// is exactly the failure this repository exists to name, committed by this
// repository.
//
// The fix is not a chain of per-hop records. It is two rules -- the dialect is
// written once and never rewritten, and the loss list merges rather than
// replaces -- plus an integer saying how many times this happened.
func TestSecondHopKeepsTheFirstHopsRecord(t *testing.T) {
	in := mustReadFile(t, testdata+"/openllmetry/in.json")

	first := hop(t, in, semconv.TargetV1_41_0, OriginalsPrune)
	second := hop(t, first, semconv.TargetGenAIMain, OriginalsPrune)

	a1 := attrsOf(t, first, "openai.chat")
	a2 := attrsOf(t, second, "openai.chat")

	if got, want := a1[AttrDialect], "openllmetry"; got != want {
		t.Fatalf("hop 1 dialect = %v, want %v", got, want)
	}
	firstLosses := a1[AttrLossyCount].(int64)
	if firstLosses == 0 {
		t.Fatalf("hop 1 recorded no losses; this fixture is supposed to have some")
	}

	// The emitter is a fact about where the data came from. Translating it
	// again does not make it have come from somewhere else, and the second pass
	// has no evidence of it beyond what the first pass wrote down.
	if got, want := a2[AttrDialect], "openllmetry"; got != want {
		t.Errorf("hop 2 dialect = %v, want %v -- the second pass re-detected from"+
			"\n    normalized output instead of keeping the first pass's answer", got, want)
	}

	// A key the first hop could not carry did not become carryable by being
	// looked at a second time.
	if got := a2[AttrLossyCount].(int64); got < firstLosses {
		t.Errorf("hop 2 lossy count = %d, want >= %d: the first hop's losses are part"+
			"\n    of this span's history and did not stop being true", got, firstLosses)
	}

	// The count and the list are written by different rules, so they are
	// checked against each other rather than each against a constant. This is
	// the assertion that caught the original self-contradiction: a count of 0
	// sitting beside a three-element list.
	list, _ := a2[AttrLossy].([]string)
	if got := int(a2[AttrLossyCount].(int64)); got != len(list) {
		t.Errorf("%s = %d but %s has %d entries; they must agree",
			AttrLossyCount, got, AttrLossy, len(list))
	}
	for _, k := range a1[AttrLossy].([]string) {
		if !containsString(list, k) {
			t.Errorf("hop 2 dropped %q from the loss list; the merge is not a merge", k)
		}
	}

	if got, want := a2[AttrHops].(int64), int64(2); got != want {
		t.Errorf("%s = %d after two translations, want %d", AttrHops, got, want)
	}
}

// TestRenormalizingToTheSameTargetIsANoOp is the cheaper half of the fix and
// covers the case that actually happens.
//
// Nobody deliberately translates v1.41.0 output to genai-main. What happens in
// practice is the same normalization running twice: a collector normalizes at
// the edge and a backend normalizes again at ingest, a spooled payload is
// replayed after a restart, a pipeline is rebuilt with the processor in it
// twice. All of those ask for a target the span already has, and the honest
// amount of work to do is none.
//
// Byte-identical rather than attribute-equal, because a no-op that reorders
// attributes or bumps a counter is not a no-op.
func TestRenormalizingToTheSameTargetIsANoOp(t *testing.T) {
	in := mustReadFile(t, testdata+"/openllmetry/in.json")

	for _, mode := range AllOriginals {
		once := hop(t, in, semconv.TargetGenAIMain, mode)
		twice := hop(t, once, semconv.TargetGenAIMain, mode)

		if string(once) != string(twice) {
			t.Errorf("originals %s: normalizing to the same target twice changed the span;"+
				"\n    a span that already carries this target has arrived", mode)
		}
		if got, want := attrsOf(t, twice, "openai.chat")[AttrHops].(int64), int64(1); got != want {
			t.Errorf("originals %s: %s = %d, want %d -- a pass that did nothing counted itself",
				mode, AttrHops, got, want)
		}
	}
}

// TestHopsDistinguishesOneTranslationFromTwo was
// TestPreservingOriginalsHidesItRatherThanFixingIt, which pinned the complaint
// that a span translated once and a span translated twice were byte-identical:
// nothing recorded how many times the data had been rewritten.
//
// With originals preserved the second hop can re-derive everything from the
// surviving traceloop.* attributes, so it reaches the same answer by
// repetition. That is not a problem in itself -- the answer is right -- but a
// consumer could not tell the two apart, and "this span went through two
// translations" is a thing an operator wants to be able to query. It is one
// integer, and it is a column rather than a blob.
func TestHopsDistinguishesOneTranslationFromTwo(t *testing.T) {
	in := mustReadFile(t, testdata+"/openllmetry/in.json")

	once := hop(t, in, semconv.TargetGenAIMain, OriginalsKeep)
	twice := hop(t, hop(t, in, semconv.TargetV1_41_0, OriginalsKeep), semconv.TargetGenAIMain, OriginalsKeep)

	a1 := attrsOf(t, once, "openai.chat")
	a2 := attrsOf(t, twice, "openai.chat")

	// Everything a consumer queries on still agrees, which is why the hop count
	// has to be the thing that differs.
	if a1[AttrDialect] != a2[AttrDialect] || a1[AttrTarget] != a2[AttrTarget] {
		t.Errorf("one hop and two hops disagree about provenance"+
			"\n    once:  %v / %v\n    twice: %v / %v",
			a1[AttrDialect], a1[AttrTarget], a2[AttrDialect], a2[AttrTarget])
	}

	if got, want := a1[AttrHops].(int64), int64(1); got != want {
		t.Errorf("one hop: %s = %d, want %d", AttrHops, got, want)
	}
	if got, want := a2[AttrHops].(int64), int64(2); got != want {
		t.Errorf("two hops: %s = %d, want %d", AttrHops, got, want)
	}
}

// containsString is here rather than imported so this file states its own
// assertions in full.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
