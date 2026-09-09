// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"strings"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func braintrustSpan(extra map[string]string) Span {
	attrs := map[string]string{
		"braintrust.span_attributes": `{"name":"answer_question","type":"llm"}`,
		"gen_ai.provider.name":       "openai",
		"gen_ai.request.model":       "gpt-4o-mini",
	}
	for k, v := range extra {
		attrs[k] = v
	}
	return spanOf(attrs)
}

func TestBraintrustClaimsItsOwnNamespace(t *testing.T) {
	d, _, ok := Detect(braintrustSpan(nil))
	if !ok || d.Name() != Braintrust {
		t.Fatalf("a braintrust span was claimed by %v (ok=%v)", d, ok)
	}
}

func TestBraintrustCarriesASingleScoreAsAnEvaluation(t *testing.T) {
	p := mustParse(t, braintrustSpan(map[string]string{
		"braintrust.scores": `{"factuality":0.92}`,
	}))

	if got := mustField(t, p, semconv.EvaluationName).Str; got != "factuality" {
		t.Errorf("evaluation name is %q, want factuality", got)
	}
	got, ok := mustField(t, p, semconv.EvaluationScoreValue).Float64()
	if !ok || got != 0.92 {
		t.Errorf("evaluation score is %v, want 0.92", got)
	}
}

func TestBraintrustRefusesToPickOneOfSeveralScores(t *testing.T) {
	// The conventions carry one evaluation per span. Choosing among several
	// would be inventing a verdict the emitter never reached.
	p := mustParse(t, braintrustSpan(map[string]string{
		"braintrust.scores": `{"factuality":0.92,"tone":0.4,"safety":1.0}`,
	}))

	for _, f := range []semconv.Field{semconv.EvaluationName, semconv.EvaluationScoreValue} {
		if v, ok := p.Fields[f]; ok {
			t.Errorf("%s was set to %v from a span carrying three scores", f, v)
		}
	}
	l := lossFor(t, p, "braintrust.scores")
	if l.Reason != ReasonFlattened {
		t.Errorf("a multi-score span was recorded with reason %q, want %q", l.Reason, ReasonFlattened)
	}
	for _, name := range []string{"factuality", "tone", "safety"} {
		if !strings.Contains(l.Detail, name) {
			t.Errorf("the loss detail does not name the %s score: %q", name, l.Detail)
		}
	}
}

func TestBraintrustLiftsTokenCountsOutOfItsMetricsBlob(t *testing.T) {
	p := mustParse(t, braintrustSpan(map[string]string{
		"braintrust.metrics": `{"prompt_tokens":412,"completion_tokens":27,"prompt_cached_tokens":256,"start":1749834000.1,"end":1749834002.7}`,
	}))

	for f, want := range map[semconv.Field]int64{
		semconv.UsageInputTokens:          412,
		semconv.UsageOutputTokens:         27,
		semconv.UsageCacheReadInputTokens: 256,
	} {
		if got := mustField(t, p, f).Int; got != want {
			t.Errorf("%s is %d, want %d", f, got, want)
		}
	}
	// The timings have nowhere to go, and are named once rather than one loss
	// per key.
	if l := lossFor(t, p, "braintrust.metrics"); l.Reason != ReasonNoField {
		t.Errorf("leftover metrics recorded with reason %q, want %q", l.Reason, ReasonNoField)
	}
}

func TestBraintrustPrefersTheEmittersOwnGenAIAttributeOverTheBlob(t *testing.T) {
	// A count written to gen_ai.usage.input_tokens is the emitter speaking the
	// target's language, and outranks the same number inside a JSON blob.
	s := braintrustSpan(map[string]string{"braintrust.metrics": `{"prompt_tokens":999}`})
	// Built as a real int rather than through spanOf, which makes every
	// attribute a string: a token count arrives on the wire as an integer.
	s.Attributes["gen_ai.usage.input_tokens"] = Int(412)

	p := mustParse(t, s)
	if got := mustField(t, p, semconv.UsageInputTokens).Int; got != 412 {
		t.Errorf("input tokens is %d, want the gen_ai attribute's 412", got)
	}
}

func TestBraintrustMapsItsSpanTaxonomy(t *testing.T) {
	for typ, want := range map[string]string{
		"llm":  "chat",
		"tool": "execute_tool",
		"task": "invoke_workflow",
	} {
		s := braintrustSpan(map[string]string{
			"braintrust.span_attributes": `{"name":"step","type":"` + typ + `"}`,
		})
		if got := mustField(t, mustParse(t, s), semconv.OperationName).Str; got != want {
			t.Errorf("span type %q became operation %q, want %q", typ, got, want)
		}
	}
}

func TestBraintrustRecordsAScoringSpanAsHavingNoOperation(t *testing.T) {
	// A span whose whole job is scoring is not an operation the conventions
	// name at either version.
	s := braintrustSpan(map[string]string{
		"braintrust.span_attributes": `{"name":"factuality","type":"score"}`,
	})
	p := mustParse(t, s)

	if v, ok := p.Fields[semconv.OperationName]; ok {
		t.Errorf("a score span was given operation %v", v)
	}
	if l := lossFor(t, p, "braintrust.span_attributes"); l.Reason != ReasonNoField {
		t.Errorf("a score span was recorded with reason %q, want %q", l.Reason, ReasonNoField)
	}
}

// braintrust.context_json is the one attribute in this dialect that no
// application writes: a real BraintrustSpanProcessor injects it on the way out,
// which is how it turned up in testdata/braintrust when that fixture stopped
// being hand-built. It has no schema to map onto, so it is recorded rather than
// carried -- but it must be recorded, because an attribute the normalizer walks
// past silently is indistinguishable from one it does not know exists.
func TestBraintrustRecordsTheContextItsOwnProcessorAdds(t *testing.T) {
	p := (braintrust{}).Parse(Span{Attributes: map[string]Value{
		"braintrust.context_json": String(`{"caller":"capture"}`),
	}})

	var found bool
	for _, l := range p.Loss {
		if l.Key == "braintrust.context_json" && l.Reason == ReasonUnstructured {
			found = true
		}
	}
	if !found {
		t.Errorf("braintrust.context_json was not recorded as unstructured; losses = %+v", p.Loss)
	}
}
