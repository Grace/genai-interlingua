package dialect

import (
	"strings"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func init() { register(raw{}) }

// raw is the fallback for spans nobody instrumented on purpose.
//
// A large amount of GenAI telemetry is a handful of span.SetAttributes calls
// someone wrote around their own client wrapper. There is no library to
// recognize and no convention being followed, only a person who typed what
// seemed obvious: model, prompt_tokens, temperature. Those spans are the
// reason a normalizer exists at all -- they are the ones nothing downstream can
// read -- and refusing to touch them because they carry no signature would miss
// the case most in need of the treatment.
//
// It matches on shape, so it is a fallback rather than a competitor: it is
// consulted only when no dialect that knows what it is looking at has claimed
// the span, and it reports a confidence of zero when it wins. See the fallback
// interface in dialect.go for why that separation lives in detection rather
// than in scoring.
type raw struct{}

func (raw) Name() Name { return Raw }

func (raw) isFallback() {}

// rawAliases are the spellings people reach for when nobody has told them what
// the attribute is called. Every one of them is a key seen in the wild on
// hand-rolled spans; none of them is any emitter's convention.
var rawAliases = map[string]semconv.Field{
	"model":                   semconv.RequestModel,
	"model_name":              semconv.RequestModel,
	"llm.model":               semconv.RequestModel,
	"llm.model_name":          semconv.RequestModel,
	"openai.model":            semconv.RequestModel,
	"provider":                semconv.ProviderName,
	"llm.provider":            semconv.ProviderName,
	"temperature":             semconv.RequestTemperature,
	"llm.temperature":         semconv.RequestTemperature,
	"max_tokens":              semconv.RequestMaxTokens,
	"llm.max_tokens":          semconv.RequestMaxTokens,
	"top_p":                   semconv.RequestTopP,
	"prompt_tokens":           semconv.UsageInputTokens,
	"input_tokens":            semconv.UsageInputTokens,
	"usage.prompt_tokens":     semconv.UsageInputTokens,
	"llm.prompt_tokens":       semconv.UsageInputTokens,
	"completion_tokens":       semconv.UsageOutputTokens,
	"output_tokens":           semconv.UsageOutputTokens,
	"usage.completion_tokens": semconv.UsageOutputTokens,
	"llm.completion_tokens":   semconv.UsageOutputTokens,
	"finish_reason":           semconv.ResponseFinishReasons,
	"request_id":              semconv.ResponseID,
	"response_id":             semconv.ResponseID,
}

// rawPayloads are the keys that carry the prompt or the answer as free text.
// They are recognized so they can be reported rather than mapped: what is in
// them varies per author, and a message list guessed out of one would be a
// worse lie than an honest gap.
var rawPayloads = []string{
	"prompt", "completion", "response", "messages", "system_prompt",
	"llm.prompt", "llm.completion", "llm.response", "input", "output",
}

// Score recognizes GenAI shape, and counts only attributes it can actually
// name. Sharing a namespace is not evidence: llm.rig is somebody's rig, and a
// dialect that claimed a span because a key started with llm. would be
// claiming a prefix rather than a meaning.
//
// An attribute the conventions themselves define is worth two, because writing
// one by hand is a deliberate act. A folk spelling is worth one and needs
// corroboration, since a lone model attribute belongs to a machine learning
// span, a database span and a hand-rolled LLM span equally, and claiming all
// three would be worse than claiming none.
func (raw) Score(s Span) int {
	conformant, folk := 0, 0
	for k, v := range s.Attributes {
		if v.Empty() {
			continue
		}
		if _, ok := semconv.FieldForKey(k); ok {
			conformant++
			continue
		}
		if _, ok := rawAliases[k]; ok {
			folk++
		}
	}

	if conformant == 0 && folk < 2 {
		return 0
	}
	return conformant*2 + folk
}

func (d raw) Parse(s Span) Parsed {
	var p Parsed

	// A key the conventions themselves define is taken at its word. Someone
	// who wrote gen_ai.usage.input_tokens by hand got it right, and the fact
	// that no library was involved does not make the attribute mean less.
	for _, k := range s.Keys() {
		if f, ok := semconv.FieldForKey(k); ok {
			p.Take(s, k, f)
		}
	}

	for _, k := range s.Keys() {
		f, ok := rawAliases[k]
		if !ok {
			continue
		}
		// A conformant key already read wins over a guess at a folk spelling.
		if _, already := p.Fields[f]; already {
			p.Lose(k, ReasonNoField, "the span also carries this as a gen_ai attribute")
			continue
		}
		if f == semconv.ResponseFinishReasons {
			if v, ok := s.Attr(k); ok {
				p.Set(f, strSeq([]string{v.Str}))
				p.Consumed = append(p.Consumed, k)
			}
			continue
		}
		if f == semconv.ProviderName {
			if v, ok := s.Attr(k); ok {
				p.Set(f, String(strings.ToLower(v.Str)))
				p.Consumed = append(p.Consumed, k)
			}
			continue
		}
		p.Take(s, k, f)
	}

	// Recognized, and deliberately not carried. Every other dialect here
	// records a total token count as a loss, and the same fact written by hand
	// should read the same way in the conformance table.
	for _, k := range []string{"total_tokens", "tokens", "usage.total_tokens", "llm.total_tokens"} {
		if s.Has(k) {
			p.Lose(k, ReasonNoField, "the conventions carry input and output counts only")
		}
	}

	for _, k := range rawPayloads {
		if s.Has(k) {
			p.Lose(k, ReasonUnstructured,
				"free-form payload from a hand-rolled span, with no shape to read it by")
		}
	}

	// Whatever is left in the two GenAI namespaces was meant to say something
	// and could not be placed. Naming it is the point: this is the dialect for
	// spans nobody designed, and the loss list is the only record of what their
	// author was trying to record.
	for _, k := range s.Keys() {
		if !strings.HasPrefix(k, "gen_ai.") && !strings.HasPrefix(k, "llm.") {
			continue
		}
		if slicesContains(p.Consumed, k) {
			continue
		}
		p.Lose(k, ReasonNoField, "not an attribute the conventions define at any version")
	}

	return p
}

func slicesContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
