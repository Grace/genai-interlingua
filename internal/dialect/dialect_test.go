package dialect

import (
	"slices"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// spanOf builds a span whose attributes are all strings, which is what most of
// these tests need. A test that cares about a non-string type builds the
// attribute map itself.
func spanOf(attrs map[string]string) Span {
	m := make(map[string]Value, len(attrs))
	for k, v := range attrs {
		m[k] = String(v)
	}
	return Span{Attributes: m}
}

func mustParse(t *testing.T, s Span) Parsed {
	t.Helper()
	p, ok := Parse(s)
	if !ok {
		t.Fatalf("no dialect claimed a span carrying %v", s.Keys())
	}
	return p
}

func mustField(t *testing.T, p Parsed, f semconv.Field) Value {
	t.Helper()
	v, ok := p.Fields[f]
	if !ok {
		t.Fatalf("field %s is not set; set fields are %v", f, p.SortedFields())
	}
	return v
}

func lossFor(t *testing.T, p Parsed, key string) Loss {
	t.Helper()
	for _, l := range p.Loss {
		if l.Key == key {
			return l
		}
	}
	t.Fatalf("no loss recorded for %s; losses are %v", key, LossKeys(p.Loss))
	return Loss{}
}

func TestSpanWithNoGenAIEvidenceIsNotClaimed(t *testing.T) {
	s := spanOf(map[string]string{
		"http.request.method": "POST",
		"db.system.name":      "postgresql",
		"llm.rig":             "not a real attribute",
	})
	if d, _, ok := Detect(s); ok {
		t.Errorf("%s claimed a span with no GenAI evidence", d.Name())
	}
	if _, ok := Parse(s); ok {
		t.Error("Parse claimed a span with no GenAI evidence")
	}
}

func TestDetectPrefersTheDialectWithMoreEvidence(t *testing.T) {
	// Both dialects recognize something here. The traceloop namespace plus a
	// legacy token count outweighs a lone model name.
	s := spanOf(map[string]string{
		"traceloop.span.kind":        "workflow",
		"gen_ai.usage.prompt_tokens": "41",
		"llm.model_name":             "gpt-4o",
	})
	d, margin, ok := Detect(s)
	if !ok {
		t.Fatal("nothing claimed the span")
	}
	if got, want := d.Name(), OpenLLMetry; got != want {
		t.Errorf("winner = %s, want %s", got, want)
	}
	if got, want := margin, 2; got != want {
		t.Errorf("margin = %d, want %d", got, want)
	}
}

// A margin of zero is the interesting case: the span carried equal evidence for
// two emitters. Detection still resolves, deterministically, and the confidence
// written to the output span is what says the choice was not clear-cut.
func TestATieResolvesDeterministicallyWithZeroMargin(t *testing.T) {
	s := spanOf(map[string]string{
		"llm.request.type": "chat",
		"llm.model_name":   "gpt-4o",
	})
	first, margin, ok := Detect(s)
	if !ok {
		t.Fatal("nothing claimed the span")
	}
	if got, want := margin, 0; got != want {
		t.Errorf("margin = %d, want %d", got, want)
	}
	for i := 0; i < 8; i++ {
		again, _, _ := Detect(s)
		if again.Name() != first.Name() {
			t.Fatalf("detection is not deterministic: got %s then %s", first.Name(), again.Name())
		}
	}
}

func TestParseStampsTheWinnerAndItsMargin(t *testing.T) {
	p := mustParse(t, spanOf(map[string]string{
		"openinference.span.kind": "LLM",
		"llm.model_name":          "gpt-4o",
	}))
	if got, want := p.Dialect, OpenInference; got != want {
		t.Errorf("dialect = %s, want %s", got, want)
	}
	if got, want := p.Confidence, 3; got != want {
		t.Errorf("confidence = %d, want %d", got, want)
	}
}

func TestParseSortsConsumedKeys(t *testing.T) {
	p := mustParse(t, spanOf(map[string]string{
		"gen_ai.usage.prompt_tokens":     "41",
		"gen_ai.usage.completion_tokens": "7",
		"gen_ai.request.model":           "claude-opus-5",
		"gen_ai.system":                  "Anthropic",
	}))
	if !slices.IsSorted(p.Consumed) {
		t.Errorf("Consumed is not sorted: %v", p.Consumed)
	}
}

func TestEveryRegisteredDialectHasADistinctName(t *testing.T) {
	seen := make(map[Name]bool)
	for _, d := range Dialects() {
		if seen[d.Name()] {
			t.Errorf("two dialects both call themselves %s", d.Name())
		}
		seen[d.Name()] = true
	}
	if len(seen) == 0 {
		t.Fatal("no dialects registered")
	}
}

func TestHasPrefixWantsSomethingUnderThePrefix(t *testing.T) {
	s := spanOf(map[string]string{"traceloop.": "bare", "gen_ai.system": "openai"})
	if s.HasPrefix("traceloop.") {
		t.Error("a key equal to the prefix counted as a key under it")
	}
	if !s.HasPrefix("gen_ai.") {
		t.Error("gen_ai.system did not count as a key under gen_ai.")
	}
}

func TestSetIgnoresEmptyValues(t *testing.T) {
	var p Parsed
	p.Set(semconv.ProviderName, Value{})
	p.Set(semconv.RequestStopSequences, strSeq(nil))
	if got := len(p.Fields); got != 0 {
		t.Errorf("%d fields set from empty values, want 0: %v", got, p.SortedFields())
	}
}

func TestTakeFirstPrefersTheEarlierKey(t *testing.T) {
	s := spanOf(map[string]string{"new": "modern", "old": "legacy"})
	var p Parsed
	if !p.TakeFirst(s, semconv.RequestModel, "new", "old") {
		t.Fatal("TakeFirst found neither key")
	}
	if got, want := mustField(t, p, semconv.RequestModel).Str, "modern"; got != want {
		t.Errorf("model = %q, want %q", got, want)
	}
	if got, want := len(p.Consumed), 1; got != want {
		t.Errorf("consumed %d keys, want %d: %v", got, want, p.Consumed)
	}
}

func TestSortedFieldsFollowsRegistryOrder(t *testing.T) {
	var p Parsed
	p.Set(semconv.UsageInputTokens, Int(41))
	p.Set(semconv.ProviderName, String("openai"))
	got := p.SortedFields()
	if len(got) != 2 || got[0] != semconv.ProviderName || got[1] != semconv.UsageInputTokens {
		t.Errorf("SortedFields = %v, want [%s %s]", got, semconv.ProviderName, semconv.UsageInputTokens)
	}
}

func TestLossRecordsTheKeyAsConsumed(t *testing.T) {
	var p Parsed
	p.Lose("traceloop.entity.path", ReasonNoField, "no equivalent")
	if got, want := len(p.Consumed), 1; got != want {
		t.Fatalf("consumed %d keys, want %d", got, want)
	}
	if got, want := LossKeys(p.Loss)[0], "traceloop.entity.path"; got != want {
		t.Errorf("LossKeys[0] = %q, want %q", got, want)
	}
}

// A dialect that translates a value onto a closed enum has to land inside that
// enum, at every target, or the normalizer will emit something no schema names.
func TestEveryMappedOperationIsLegalInBothTargets(t *testing.T) {
	tables := []map[string]string{
		openLLMetryOperations,
		openLLMetrySpanKinds,
		openInferenceOperations,
	}
	for _, table := range tables {
		for src, op := range table {
			for _, target := range semconv.Targets {
				if !target.Accepts(semconv.OperationName, op) {
					t.Errorf("%q maps to operation %q, which %s does not accept", src, op, target)
				}
			}
		}
	}
}

func TestEveryTranslatedProviderIsLegalInBothTargets(t *testing.T) {
	for src, name := range openInferenceProviders {
		for _, target := range semconv.Targets {
			if !target.Accepts(semconv.ProviderName, name) {
				t.Errorf("llm.provider %q maps to %q, which %s does not accept", src, name, target)
			}
		}
	}
}
