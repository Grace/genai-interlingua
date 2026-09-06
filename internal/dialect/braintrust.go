package dialect

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func init() { register(braintrust{}) }

// braintrust parses spans from Braintrust's SDK.
//
// Braintrust models a span as a row in an eval: input, output, expected,
// scores, metadata and metrics, each carried as its own JSON attribute under
// the braintrust namespace. That model predates the GenAI conventions and is
// wider than them in one direction and narrower in another. Wider, because a
// scored span is a first-class thing there and the conventions only learned
// about evaluations later. Narrower, because the model call itself is metadata
// rather than structure.
//
// So this dialect does more lifting out of JSON than any other here, and it is
// the one that most often has to say a thing cannot be carried.
//
// Attribute names are taken from the Braintrust OpenTelemetry integration
// documentation, which lists the braintrust.* attributes it reads and writes.
type braintrust struct{}

func (braintrust) Name() Name { return Braintrust }

// braintrustSpanTypes maps the type in braintrust.span_attributes onto
// gen_ai.operation.name. score and eval have no counterpart: an evaluation is
// modelled as an event in the new repository and as nothing at all in the
// frozen cut, so a span whose whole job is scoring is not an operation the
// conventions name.
var braintrustSpanTypes = map[string]string{
	"llm":      "chat",
	"tool":     "execute_tool",
	"function": "execute_tool",
	"task":     "invoke_workflow",
}

// Score keys off the namespace. Nothing else emits braintrust.*, and a span
// that carries any member of it was produced by this SDK, so one signal at
// weight three is the whole test.
func (braintrust) Score(s Span) int {
	if s.HasPrefix("braintrust.") {
		return 3
	}
	return 0
}

func (d braintrust) Parse(s Span) Parsed {
	var p Parsed

	// Braintrust writes the conventions' own attributes where it has them, so
	// the gen_ai half is read first and the braintrust half only fills gaps.
	p.TakeFirst(s, semconv.ProviderName, "gen_ai.provider.name", "gen_ai.system")
	p.Take(s, "gen_ai.request.model", semconv.RequestModel)
	p.Take(s, "gen_ai.response.model", semconv.ResponseModel)
	p.TakeFirst(s, semconv.UsageInputTokens, "gen_ai.usage.input_tokens", "gen_ai.usage.prompt_tokens")
	p.TakeFirst(s, semconv.UsageOutputTokens, "gen_ai.usage.output_tokens", "gen_ai.usage.completion_tokens")
	p.Take(s, "gen_ai.input.messages", semconv.InputMessages)
	p.Take(s, "gen_ai.output.messages", semconv.OutputMessages)
	p.Take(s, "gen_ai.operation.name", semconv.OperationName)
	p.Take(s, "gen_ai.agent.tools", semconv.ToolDefinitions)

	d.spanAttributes(s, &p)
	d.metrics(s, &p)
	d.scores(s, &p)
	d.losses(s, &p)

	return p
}

// spanAttributes reads braintrust.span_attributes, a JSON object carrying the
// span's name and its type. The type is Braintrust's taxonomy and maps onto
// gen_ai.operation.name where the conventions have a word for it.
func (braintrust) spanAttributes(s Span, p *Parsed) {
	v, ok := s.Attr("braintrust.span_attributes")
	if !ok {
		return
	}
	p.Consumed = append(p.Consumed, "braintrust.span_attributes")

	var attrs struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(v.Str), &attrs); err != nil {
		p.Loss = append(p.Loss, Loss{Key: "braintrust.span_attributes",
			Reason: ReasonUnstructured, Detail: "value is not a JSON object"})
		return
	}
	if attrs.Type == "" {
		return
	}
	// An operation read from gen_ai.operation.name is the emitter's own
	// statement and outranks one inferred from the span taxonomy.
	if _, already := p.Fields[semconv.OperationName]; already {
		return
	}
	if op, ok := braintrustSpanTypes[attrs.Type]; ok {
		p.Set(semconv.OperationName, String(op))
		return
	}
	p.Loss = append(p.Loss, Loss{Key: "braintrust.span_attributes", Reason: ReasonNoField,
		Detail: "no gen_ai.operation.name value for span type " + attrs.Type})
}

// metrics lifts the token counts out of braintrust.metrics, a JSON object of
// numbers that also carries timings and cache statistics. Only the two counts
// have anywhere to go; the rest is named in one loss rather than one per key,
// because a reader wants to know the metrics object was not carried, not to
// read a list of its members.
func (braintrust) metrics(s Span, p *Parsed) {
	v, ok := s.Attr("braintrust.metrics")
	if !ok {
		return
	}
	p.Consumed = append(p.Consumed, "braintrust.metrics")

	var m map[string]any
	if err := json.Unmarshal([]byte(v.Str), &m); err != nil {
		p.Loss = append(p.Loss, Loss{Key: "braintrust.metrics",
			Reason: ReasonUnstructured, Detail: "value is not a JSON object"})
		return
	}

	lifted := map[string]semconv.Field{
		"prompt_tokens":               semconv.UsageInputTokens,
		"completion_tokens":           semconv.UsageOutputTokens,
		"prompt_cached_tokens":        semconv.UsageCacheReadInputTokens,
		"completion_reasoning_tokens": semconv.UsageReasoningOutputTokens,
	}
	var left []string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		f, ok := lifted[k]
		if !ok {
			left = append(left, k)
			continue
		}
		// A count already read from a gen_ai attribute is the emitter speaking
		// the target's own language, and is not overwritten from the blob.
		if _, already := p.Fields[f]; already {
			continue
		}
		if val := jsonValue(m[k]); !val.Empty() {
			p.Set(f, val)
		} else {
			left = append(left, k)
		}
	}
	if len(left) > 0 {
		p.Loss = append(p.Loss, Loss{Key: "braintrust.metrics", Reason: ReasonNoField,
			Detail: "no gen_ai field for " + strings.Join(left, ", ")})
	}
}

// scores is the interesting one. Braintrust carries a whole set of named
// scores on a span; the conventions carry exactly one evaluation, as a name and
// a value. So a span with a single score translates cleanly and a span with
// several does not translate at all: picking one of them would be inventing a
// verdict the emitter never reached, and averaging them would be worse.
func (braintrust) scores(s Span, p *Parsed) {
	v, ok := s.Attr("braintrust.scores")
	if !ok {
		return
	}
	p.Consumed = append(p.Consumed, "braintrust.scores")

	var scores map[string]any
	if err := json.Unmarshal([]byte(v.Str), &scores); err != nil {
		p.Loss = append(p.Loss, Loss{Key: "braintrust.scores",
			Reason: ReasonUnstructured, Detail: "value is not a JSON object"})
		return
	}
	names := slices.Sorted(maps.Keys(scores))
	switch len(names) {
	case 0:
		return
	case 1:
		value := jsonValue(scores[names[0]])
		if _, ok := value.Float64(); !ok {
			p.Loss = append(p.Loss, Loss{Key: "braintrust.scores", Reason: ReasonUnstructured,
				Detail: "score " + names[0] + " is not a number"})
			return
		}
		p.Set(semconv.EvaluationName, String(names[0]))
		p.Set(semconv.EvaluationScoreValue, value)
	default:
		p.Loss = append(p.Loss, Loss{Key: "braintrust.scores", Reason: ReasonFlattened,
			Detail: "the conventions carry one evaluation per span and this span has " +
				strings.Join(names, ", ")})
	}
}

// losses records the rest of the Braintrust model. input and output are listed
// rather than mapped onto the message fields: they are whatever the traced
// function took and returned, which is a message list only when the traced
// function happened to be a model call, and gen_ai.input.messages is read above
// for the case where Braintrust knew it was one.
func (braintrust) losses(s Span, p *Parsed) {
	for _, k := range []string{
		"braintrust.input",
		"braintrust.input_json",
		"braintrust.output",
		"braintrust.output_json",
		"braintrust.context_json",
	} {
		if s.Has(k) {
			p.Lose(k, ReasonUnstructured, "the traced function's own payload, with no schema to map onto")
		}
	}
	for _, k := range []string{
		"braintrust.expected",
		"braintrust.expected_json",
		"braintrust.tags",
		"braintrust.metadata",
		"braintrust.parent",
		"braintrust.project_id",
		"braintrust.span_id",
		"braintrust.root_span_id",
	} {
		if s.Has(k) {
			p.Lose(k, ReasonNoField, "part of Braintrust's eval model, which the conventions do not have")
		}
	}
}
