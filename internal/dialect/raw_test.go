package dialect

import (
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func TestRawClaimsAHandRolledSpan(t *testing.T) {
	// Three folk spellings and no library in sight.
	s := spanOf(map[string]string{
		"model":             "gpt-4o-mini",
		"prompt_tokens":     "412",
		"completion_tokens": "27",
		"temperature":       "0.7",
	})

	d, confidence, ok := Detect(s)
	if !ok {
		t.Fatal("nothing claimed a hand-rolled GenAI span")
	}
	if d.Name() != Raw {
		t.Fatalf("a hand-rolled span was claimed by %s, want %s", d.Name(), Raw)
	}
	// It did not beat anything; it was the only thing left, and the span should
	// say so rather than claim a confident identification.
	if confidence != 0 {
		t.Errorf("the fallback reported a confidence of %d, want 0", confidence)
	}
}

func TestRawNeedsCorroborationForAFolkSpelling(t *testing.T) {
	// A lone model attribute belongs to a machine learning span, a database
	// span and a hand-rolled LLM span equally.
	if n := (raw{}).Score(spanOf(map[string]string{
		"model":      "resnet50",
		"batch_size": "32",
		"db.system":  "postgresql",
	})); n != 0 {
		t.Errorf("raw scored %d on a span with one folk spelling, want 0", n)
	}
}

func TestRawDoesNotClaimANamespaceItCannotRead(t *testing.T) {
	// Sharing a prefix is not evidence.
	if n := (raw{}).Score(spanOf(map[string]string{
		"llm.rig":     "not a real attribute",
		"llm.harness": "also not one",
	})); n != 0 {
		t.Errorf("raw scored %d on unrecognized keys that merely start with llm., want 0", n)
	}
}

// The fallback must not take a share of the evidence on a span a real dialect
// identifies: the score it took would come straight out of the winner's margin.
func TestRawDoesNotDiluteAPositiveIdentification(t *testing.T) {
	s := spanOf(map[string]string{
		"gen_ai.system":              "OpenAI",
		"llm.request.type":           "chat",
		"gen_ai.usage.prompt_tokens": "412",
		"traceloop.workflow.name":    "support_triage",
	})

	d, margin, ok := Detect(s)
	if !ok || d.Name() != OpenLLMetry {
		t.Fatalf("the span was claimed by %v, want openllmetry", d)
	}
	// OpenLLMetry's own score, undiminished: nothing else that scored competed.
	if want := (openLLMetry{}).Score(s); margin != want {
		t.Errorf("margin is %d, want openllmetry's full score of %d", margin, want)
	}
}

func TestRawTakesAConformantAttributeAtItsWord(t *testing.T) {
	// Someone who wrote gen_ai.usage.input_tokens by hand got it right, and no
	// library being involved does not make the attribute mean less.
	s := Span{Attributes: map[string]Value{
		"gen_ai.usage.input_tokens": Int(412),
		"gen_ai.request.model":      String("gpt-4o-mini"),
	}}
	p := mustParse(t, s)

	if got := mustField(t, p, semconv.UsageInputTokens).Int; got != 412 {
		t.Errorf("input tokens is %d, want 412", got)
	}
	if got := mustField(t, p, semconv.RequestModel).Str; got != "gpt-4o-mini" {
		t.Errorf("request model is %q, want gpt-4o-mini", got)
	}
}

func TestRawPrefersTheConformantKeyOverTheFolkOne(t *testing.T) {
	s := Span{Attributes: map[string]Value{
		"gen_ai.usage.input_tokens": Int(412),
		"prompt_tokens":             Int(999),
		"model":                     String("wrong"),
		"gen_ai.request.model":      String("gpt-4o-mini"),
	}}
	p := mustParse(t, s)

	if got := mustField(t, p, semconv.UsageInputTokens).Int; got != 412 {
		t.Errorf("input tokens is %d, want the conformant attribute's 412", got)
	}
	if got := mustField(t, p, semconv.RequestModel).Str; got != "gpt-4o-mini" {
		t.Errorf("request model is %q, want the conformant attribute's value", got)
	}
	if l := lossFor(t, p, "prompt_tokens"); l.Reason != ReasonNoField {
		t.Errorf("the shadowed folk key was recorded with reason %q", l.Reason)
	}
}

func TestRawReportsFreeFormPayloadsRatherThanGuessingAtThem(t *testing.T) {
	// What is in these varies per author, and a message list guessed out of one
	// would be a worse lie than an honest gap.
	s := spanOf(map[string]string{
		"model":         "gpt-4o-mini",
		"prompt_tokens": "412",
		"prompt":        "Where is order A-1187?",
		"completion":    "Checking now.",
	})
	p := mustParse(t, s)

	if v, ok := p.Fields[semconv.InputMessages]; ok {
		t.Errorf("a message list was invented from a free-form prompt: %v", v)
	}
	for _, k := range []string{"prompt", "completion"} {
		if l := lossFor(t, p, k); l.Reason != ReasonUnstructured {
			t.Errorf("%s was recorded with reason %q, want %q", k, l.Reason, ReasonUnstructured)
		}
	}
}

func TestRawNamesTheAttributesItCouldNotPlace(t *testing.T) {
	// This is the dialect for spans nobody designed, so the loss list is the
	// only record of what their author was trying to say.
	s := spanOf(map[string]string{
		"gen_ai.request.model": "gpt-4o-mini",
		"gen_ai.retry_count":   "2",
		"llm.cache_hit":        "true",
	})
	p := mustParse(t, s)

	for _, k := range []string{"gen_ai.retry_count", "llm.cache_hit"} {
		if l := lossFor(t, p, k); l.Reason != ReasonNoField {
			t.Errorf("%s was recorded with reason %q, want %q", k, l.Reason, ReasonNoField)
		}
	}
}
