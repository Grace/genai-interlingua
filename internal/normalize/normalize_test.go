package normalize

import (
	"slices"
	"strings"
	"testing"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// chatSpan is an OpenLLMetry chat span carrying enough signature attributes to
// be claimed, plus whatever the test wants to say. Overrides are applied last so
// a test can replace a base attribute as well as add one.
func chatSpan(overrides map[string]dialect.Value) dialect.Span {
	attrs := map[string]dialect.Value{
		"gen_ai.system":              dialect.String("OpenAI"),
		"llm.request.type":           dialect.String("chat"),
		"gen_ai.request.model":       dialect.String("gpt-4o-mini"),
		"gen_ai.usage.prompt_tokens": dialect.Int(412),
	}
	for k, v := range overrides {
		attrs[k] = v
	}
	return dialect.Span{Name: "openai.chat", Attributes: attrs}
}

func mustNormalize(t *testing.T, s dialect.Span, target semconv.Target) Result {
	t.Helper()
	opts := DefaultOptions()
	opts.Target = target
	r, ok := Span(s, opts)
	if !ok {
		t.Fatalf("no dialect claimed a span carrying %v", s.Keys())
	}
	return r
}

func targetLossFor(t *testing.T, r Result, key string) Loss {
	t.Helper()
	for _, l := range r.TargetLoss {
		if l.Key == key {
			return l
		}
	}
	t.Fatalf("no target loss recorded for %s; losses are %v", key, r.Lossy())
	return Loss{}
}

func TestFieldTheTargetCannotRepresentIsDropped(t *testing.T) {
	// traceloop.prompt.version parses into a field the new repository added
	// after the split, so it survives to genai-main and not to the frozen cut.
	s := chatSpan(map[string]dialect.Value{
		"traceloop.prompt.key":     dialect.String("order_status"),
		"traceloop.prompt.version": dialect.Int(3),
	})

	frozen := mustNormalize(t, s, semconv.TargetV1_41_0)
	if v, ok := frozen.Set["gen_ai.prompt.version"]; ok {
		t.Errorf("gen_ai.prompt.version is not defined at v1.41.0 but was set to %v", v)
	}
	// The loss names the field by the key it takes where it does exist, which is
	// more use to a reader than the Traceloop attribute it happened to come from.
	loss := targetLossFor(t, frozen, "gen_ai.prompt.version")
	if loss.Reason != ReasonNoAttribute {
		t.Errorf("dropped an undefined field with reason %q, want %q", loss.Reason, ReasonNoAttribute)
	}
	// The field beside it is defined at both targets and must be unaffected.
	if _, ok := frozen.Set["gen_ai.prompt.name"]; !ok {
		t.Error("gen_ai.prompt.name is defined at v1.41.0 but was not set")
	}

	main := mustNormalize(t, s, semconv.TargetGenAIMain)
	if _, ok := main.Set["gen_ai.prompt.version"]; !ok {
		t.Error("gen_ai.prompt.version is defined in the new repository but was not set")
	}
	if len(main.TargetLoss) != 0 {
		t.Errorf("normalizing to the newer schema still lost %v", main.TargetLoss)
	}
}

func TestValueTheTargetDoesNotDefineIsDropped(t *testing.T) {
	// moonshot_ai is the one provider the new repository names and v1.41.0 does
	// not. The attribute exists at both; only the value moved.
	s := chatSpan(map[string]dialect.Value{"gen_ai.system": dialect.String("moonshot_ai")})

	frozen := mustNormalize(t, s, semconv.TargetV1_41_0)
	if v, ok := frozen.Set["gen_ai.provider.name"]; ok {
		t.Errorf("moonshot_ai is not a v1.41.0 provider but gen_ai.provider.name was set to %v", v)
	}
	if loss := targetLossFor(t, frozen, "gen_ai.provider.name"); loss.Reason != ReasonNoValue {
		t.Errorf("dropped an undefined value with reason %q, want %q", loss.Reason, ReasonNoValue)
	}

	main := mustNormalize(t, s, semconv.TargetGenAIMain)
	if got := main.Set["gen_ai.provider.name"].Str; got != "moonshot_ai" {
		t.Errorf("gen_ai.provider.name is %q at genai-main, want moonshot_ai", got)
	}
}

func TestRenamedFieldLandsOnTheKeyItsTargetUses(t *testing.T) {
	// cache_creation to cache_write is the only rename between the two schemas.
	// One field, two keys, and the value must not move.
	s := chatSpan(map[string]dialect.Value{
		"gen_ai.usage.cache_creation_input_tokens": dialect.Int(1024),
	})

	for target, key := range map[semconv.Target]string{
		semconv.TargetV1_41_0:   "gen_ai.usage.cache_creation.input_tokens",
		semconv.TargetGenAIMain: "gen_ai.usage.cache_write.input_tokens",
	} {
		r := mustNormalize(t, s, target)
		if got := r.Set[key].Int; got != 1024 {
			t.Errorf("%s at %s is %d, want 1024", key, target, got)
		}
		if len(r.TargetLoss) != 0 {
			t.Errorf("a renamed field cost %v at %s, and a rename is not a loss", r.TargetLoss, target)
		}
	}
}

func TestEverySpanRecordsHowItWasNormalized(t *testing.T) {
	r := mustNormalize(t, chatSpan(nil), semconv.TargetGenAIMain)

	for key, want := range map[string]string{
		AttrDialect: string(dialect.OpenLLMetry),
		AttrTarget:  semconv.TargetGenAIMain.String(),
	} {
		if got := r.Set[key].Str; got != want {
			t.Errorf("%s is %q, want %q", key, got, want)
		}
	}
	// Nothing else claims these attributes, so the margin is the whole score.
	if got := r.Set[AttrConfidence].Int; got != 2 {
		t.Errorf("%s is %d, want the winner's margin of 2", AttrConfidence, got)
	}
}

func TestStrippingOriginalsRemovesOnlyWhatWasReadAndNotRewritten(t *testing.T) {
	s := chatSpan(map[string]dialect.Value{
		"http.request.method": dialect.String("POST"),
	})
	opts := Options{Target: semconv.TargetV1_41_0, PreserveOriginal: false}
	r, ok := Span(s, opts)
	if !ok {
		t.Fatal("no dialect claimed the span")
	}

	// gen_ai.system is read and rewritten under a different key, so the original
	// spelling is genuinely gone once stripped.
	if !slices.Contains(r.Remove, "gen_ai.system") {
		t.Errorf("stripping originals kept gen_ai.system; removals are %v", r.Remove)
	}
	// gen_ai.request.model is read and written back under the same key. Listing
	// it as removed would say the span lost an attribute it still has.
	if slices.Contains(r.Remove, "gen_ai.request.model") {
		t.Errorf("stripping originals removed gen_ai.request.model, which is being rewritten in place")
	}
	// Nothing is deleted on the grounds that nobody looked at it.
	if slices.Contains(r.Remove, "http.request.method") {
		t.Errorf("stripping originals removed http.request.method, which no dialect read")
	}
}

// The default normalization must not destroy anything the emitter wrote. It may
// clear its own interlingua.* attributes -- a second pass over an
// already-normalized span has to own those completely rather than leave the
// previous pass's answer beside its own -- and that is a different thing from
// removing an original, so the invariant is stated as "no source key" rather
// than "nothing at all".
func TestOriginalsAreKeptByDefault(t *testing.T) {
	r := mustNormalize(t, chatSpan(nil), semconv.TargetV1_41_0)
	for _, k := range r.Remove {
		if !strings.HasPrefix(k, "interlingua.") {
			t.Errorf("the default normalization removed the source attribute %q", k)
		}
	}
}

func TestSpanWithNoGenAIEvidenceIsLeftAlone(t *testing.T) {
	s := dialect.Span{
		Name: "POST",
		Attributes: map[string]dialect.Value{
			"http.request.method":       dialect.String("POST"),
			"server.address":            dialect.String("api.openai.com"),
			"http.response.status_code": dialect.Int(200),
		},
	}
	r, ok := Span(s, DefaultOptions())
	if ok {
		t.Errorf("a span with no GenAI evidence was claimed by %s and stamped with %v",
			r.Dialect, r.SetKeys())
	}
	if len(r.Set) != 0 {
		t.Errorf("an unclaimed span still came back with attributes to set: %v", r.SetKeys())
	}
}

// The count is written even when it is zero, which is the half that is easy to
// get wrong. "This span lost nothing" and "this span was never normalized" are
// different facts, and a query for lossless spans should not have to express
// itself as the absence of a field.
func TestLossCountIsWrittenEvenWhenNothingWasLost(t *testing.T) {
	// A span whose every attribute the dialect can carry and the target can
	// express.
	r := mustNormalize(t, chatSpan(nil), semconv.TargetGenAIMain)

	if _, ok := r.Set[AttrLossy]; ok {
		t.Fatalf("expected a lossless span, but %s is set to %v", AttrLossy, r.Set[AttrLossy])
	}
	v, ok := r.Set[AttrLossyCount]
	if !ok {
		t.Fatalf("%s is missing on a lossless span", AttrLossyCount)
	}
	if v.Kind != dialect.KindInt || v.Int != 0 {
		t.Errorf("%s = %v, want the integer 0", AttrLossyCount, v)
	}
}

// And when there is loss, the number has to be the length of the list beside
// it. Two attributes describing the same fact are worth having only while they
// agree.
func TestLossCountMatchesTheListItSummarizes(t *testing.T) {
	s := chatSpan(map[string]dialect.Value{
		"gen_ai.usage.total_tokens":                dialect.Int(439),
		"traceloop.association.properties.user_id": dialect.String("u-7741"),
	})
	r := mustNormalize(t, s, semconv.TargetGenAIMain)

	list := r.Set[AttrLossy].StrSeq
	if len(list) == 0 {
		t.Fatalf("expected this span to lose something; set = %v", r.Set)
	}
	if got, want := r.Set[AttrLossyCount].Int, int64(len(list)); got != want {
		t.Errorf("%s = %d but %s has %d entries", AttrLossyCount, got, AttrLossy, want)
	}
}

// Instrumenting LangChain through OpenLLMetry puts gen_ai.provider.name=langchain
// on the chain spans. LangChain is not a provider, it is the framework calling
// one, and no target's enum defines it.
//
// The value arrives already spelled like a conventions attribute, which is
// exactly why it has to be read back and checked rather than waved through for
// looking conformant. Otherwise a span claims interlingua.target=v1.41.0 while
// carrying a value v1.41.0 forbids, and a query grouping by provider grows a
// bucket that is not a provider.
func TestConformantLookingValueTheTargetForbidsIsStillRecorded(t *testing.T) {
	s := dialect.Span{
		Name: "RunnableSequence.workflow",
		Attributes: map[string]dialect.Value{
			"gen_ai.provider.name":  dialect.String("langchain"),
			"traceloop.span.kind":   dialect.String("workflow"),
			"traceloop.entity.name": dialect.String("RunnableSequence"),
		},
	}
	r, ok := Span(s, DefaultOptions())
	if !ok {
		t.Fatal("expected the span to be claimed")
	}

	var found bool
	for _, l := range r.TargetLoss {
		if l.Key == "gen_ai.provider.name" && l.Reason == ReasonNoValue {
			found = true
		}
	}
	if !found {
		t.Errorf("gen_ai.provider.name=langchain was not recorded as no_value; target losses = %+v", r.TargetLoss)
	}
	if v, ok := r.Set["gen_ai.provider.name"]; ok {
		t.Errorf("a forbidden value was written to the normalized output as %v", v)
	}
}

// The counterweight: a legitimate provider on the model call itself is taken,
// not flagged. The check has to discriminate or it is just noise.
func TestAProviderTheTargetDefinesIsNotFlagged(t *testing.T) {
	s := dialect.Span{
		Name: "ChatOpenAI.chat",
		Attributes: map[string]dialect.Value{
			"gen_ai.provider.name":       dialect.String("openai"),
			"gen_ai.usage.prompt_tokens": dialect.Int(412),
		},
	}
	r, ok := Span(s, DefaultOptions())
	if !ok {
		t.Fatal("expected the span to be claimed")
	}
	if got := r.Set["gen_ai.provider.name"].Str; got != "openai" {
		t.Errorf("provider = %q, want openai", got)
	}
	for _, l := range r.TargetLoss {
		if l.Key == "gen_ai.provider.name" {
			t.Errorf("a legal provider was flagged: %+v", l)
		}
	}
}
