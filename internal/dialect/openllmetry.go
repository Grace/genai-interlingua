package dialect

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func init() { register(openLLMetry{}) }

// openLLMetry parses spans from OpenLLMetry, the instrumentation shipped by
// Traceloop as opentelemetry-semantic-conventions-ai. It is the oldest of the
// gen_ai.* dialects and the reason the prefix is ambiguous: it claimed
// gen_ai.prompt, gen_ai.completion and gen_ai.usage.prompt_tokens before the
// conventions named any of them, and never followed the renames.
type openLLMetry struct{}

func (openLLMetry) Name() Name { return OpenLLMetry }

// openLLMetryOperations maps Traceloop's llm.request.type onto
// gen_ai.operation.name. rerank has no counterpart at any schema version.
var openLLMetryOperations = map[string]string{
	"chat":       "chat",
	"completion": "text_completion",
	"embedding":  "embeddings",
}

// openLLMetrySpanKinds maps traceloop.span.kind onto gen_ai.operation.name. The
// fourth kind, task, is absent: a decorated function that is neither an agent
// nor a tool is not an operation the conventions name.
var openLLMetrySpanKinds = map[string]string{
	"workflow": "invoke_workflow",
	"agent":    "invoke_agent",
	"tool":     "execute_tool",
}

// Score counts the attributes only OpenLLMetry writes. The traceloop namespace
// is worth two because nothing else emits it. The legacy token counts are worth
// one each because a mixed-vintage emitter can carry one of them without being
// OpenLLMetry, and one signal should not outvote a namespace.
func (openLLMetry) Score(s Span) int {
	n := 0
	if s.HasPrefix("traceloop.") {
		n += 2
	}
	for _, k := range []string{
		"gen_ai.usage.prompt_tokens",
		"gen_ai.usage.completion_tokens",
		"gen_ai.usage.total_tokens",
		"llm.request.type",
	} {
		if s.Has(k) {
			n++
		}
	}
	if s.HasPrefix("gen_ai.prompt.") || s.HasPrefix("gen_ai.completion.") {
		n++
	}
	return n
}

func (d openLLMetry) Parse(s Span) Parsed {
	var p Parsed

	if v, ok := s.Attr("gen_ai.system"); ok {
		p.Set(semconv.ProviderName, String(strings.ToLower(v.Str)))
		p.Consumed = append(p.Consumed, "gen_ai.system")
	}

	p.Take(s, "gen_ai.request.model", semconv.RequestModel)
	p.Take(s, "gen_ai.response.model", semconv.ResponseModel)
	p.Take(s, "gen_ai.response.id", semconv.ResponseID)
	p.Take(s, "gen_ai.request.max_tokens", semconv.RequestMaxTokens)
	p.Take(s, "gen_ai.request.temperature", semconv.RequestTemperature)
	p.Take(s, "gen_ai.request.top_p", semconv.RequestTopP)
	p.Take(s, "llm.top_k", semconv.RequestTopK)
	p.TakeFirst(s, semconv.RequestFrequencyPenalty,
		"gen_ai.request.frequency_penalty", "llm.frequency_penalty")
	p.TakeFirst(s, semconv.RequestPresencePenalty,
		"gen_ai.request.presence_penalty", "llm.presence_penalty")
	p.Take(s, "llm.chat.stop_sequences", semconv.RequestStopSequences)
	// Current OpenLLMetry spells this gen_ai.is_streaming; releases before the
	// migration spell it llm.is_streaming. Both are in the wild.
	p.TakeFirst(s, semconv.RequestStream, "gen_ai.is_streaming", "llm.is_streaming")

	p.Take(s, "gen_ai.usage.prompt_tokens", semconv.UsageInputTokens)
	p.Take(s, "gen_ai.usage.completion_tokens", semconv.UsageOutputTokens)
	p.Take(s, "gen_ai.usage.cache_creation_input_tokens", semconv.UsageCacheWriteInputTokens)
	p.Take(s, "gen_ai.usage.cache_read_input_tokens", semconv.UsageCacheReadInputTokens)
	if s.Has("gen_ai.usage.total_tokens") {
		p.Lose("gen_ai.usage.total_tokens", ReasonNoField,
			"the conventions carry input and output counts only")
	}

	// OpenLLMetry adopted most of the conventions' token names but spells the
	// reasoning count gen_ai.usage.reasoning_tokens, without the .output
	// segment the conventions use. It is the same number, so it is taken rather
	// than lost, and this is the kind of near miss that makes scoring detection
	// worth more than prefix matching.
	p.Take(s, "gen_ai.usage.reasoning_tokens", semconv.UsageReasoningOutputTokens)

	// Vendor extensions with no home in the conventions at any version. They are
	// named as losses rather than ignored, because an attribute the normalizer
	// silently walks past is indistinguishable, to a reader, from one it does
	// not know exists.
	for _, k := range []string{
		"gen_ai.openai.api_base",
		"gen_ai.openai.response.system_fingerprint",
	} {
		if s.Has(k) {
			p.Lose(k, ReasonNoField, "OpenAI-specific, not in the conventions")
		}
	}

	kind := ""
	if v, ok := s.Attr("traceloop.span.kind"); ok {
		kind = v.Str
		p.Consumed = append(p.Consumed, "traceloop.span.kind")
		if op, ok := openLLMetrySpanKinds[kind]; ok {
			p.Set(semconv.OperationName, String(op))
		} else {
			p.Loss = append(p.Loss, Loss{Key: "traceloop.span.kind", Reason: ReasonNoField,
				Detail: "no gen_ai.operation.name value for " + kind})
		}
	}

	// llm.request.type is set only on spans that really did call a model, so it
	// is the more specific of the two and wins where both are present.
	if v, ok := s.Attr("llm.request.type"); ok {
		p.Consumed = append(p.Consumed, "llm.request.type")
		if op, ok := openLLMetryOperations[v.Str]; ok {
			p.Set(semconv.OperationName, String(op))
		} else {
			p.Loss = append(p.Loss, Loss{Key: "llm.request.type", Reason: ReasonNoField,
				Detail: "no gen_ai.operation.name value for " + v.Str})
		}
	}

	// traceloop.span.kind decides what entity.name means. It is the difference
	// between an agent, a tool and an anonymous decorated function, and mapping
	// the name without reading the kind would invent an agent out of a task.
	if s.Has("traceloop.entity.name") {
		switch kind {
		case "agent":
			p.Take(s, "traceloop.entity.name", semconv.AgentName)
		case "tool":
			p.Take(s, "traceloop.entity.name", semconv.ToolName)
		default:
			detail := "no traceloop.span.kind to say what this name names"
			if kind != "" {
				detail = "traceloop.span.kind " + kind + " names no gen_ai entity"
			}
			p.Lose("traceloop.entity.name", ReasonAmbiguous, detail)
		}
	}

	p.Take(s, "traceloop.workflow.name", semconv.WorkflowName)
	p.Take(s, "traceloop.prompt.key", semconv.PromptName)
	p.Take(s, "traceloop.prompt.version", semconv.PromptVersion)

	for _, k := range []string{"traceloop.entity.input", "traceloop.entity.output"} {
		if s.Has(k) {
			p.Lose(k, ReasonUnstructured, "free-form entity payload with no schema to map onto")
		}
	}
	for _, k := range []string{
		"traceloop.entity.path",
		"traceloop.entity.version",
		"traceloop.prompt.managed",
		"traceloop.prompt.template",
		"traceloop.prompt.template_variables",
		"traceloop.prompt.version_hash",
		"traceloop.prompt.version_name",
		"traceloop.correlation.id",
	} {
		if s.Has(k) {
			p.Lose(k, ReasonNoField, "the conventions name no equivalent")
		}
	}
	for _, k := range s.Keys() {
		if strings.HasPrefix(k, "traceloop.association.properties.") {
			p.Lose(k, ReasonNoField, "association properties are Traceloop's own correlation dimensions")
		}
	}

	p.Set(semconv.ToolDefinitions, d.toolDefinitions(s, &p))

	p.Set(semconv.InputMessages, messagesValue(d.messages(s, "gen_ai.prompt", &p)))

	out := d.messages(s, "gen_ai.completion", &p)
	p.Set(semconv.OutputMessages, messagesValue(out))
	var reasons []string
	for _, m := range out {
		if m.FinishReason != "" {
			reasons = append(reasons, m.FinishReason)
		}
	}
	p.Set(semconv.ResponseFinishReasons, strSeq(reasons))

	return p
}

// messages reassembles OpenLLMetry's indexed message attributes into the message
// list shape the conventions define. Every key it reads is recorded as consumed,
// and the reassembly itself is recorded as a loss: the per-index attributes are
// gone from the output, and content that was already JSON is carried as a string
// inside the new structure rather than as JSON.
func (openLLMetry) messages(s Span, prefix string, p *Parsed) []message {
	groups, order := indexed(s, prefix)
	if len(order) == 0 {
		return nil
	}
	var msgs []message
	for _, i := range order {
		g := groups[i]
		m := message{Role: g["role"].Str, FinishReason: g["finish_reason"].Str}
		if c := g["content"]; !c.Empty() {
			m.Parts = append(m.Parts, textPart(c.Str))
		}
		for _, j := range subIndices(g, "tool_calls.") {
			m.Parts = append(m.Parts, toolCallPart(
				g[fmt.Sprintf("tool_calls.%d.id", j)].Str,
				g[fmt.Sprintf("tool_calls.%d.name", j)].Str,
				g[fmt.Sprintf("tool_calls.%d.arguments", j)].Str,
			))
		}
		for _, suffix := range slices.Sorted(maps.Keys(g)) {
			p.Consumed = append(p.Consumed, fmt.Sprintf("%s.%d.%s", prefix, i, suffix))
		}
		msgs = append(msgs, m)
	}
	// Appended directly rather than through Lose: the individual source keys are
	// already recorded as consumed above, and a glob key does not belong there.
	p.Loss = append(p.Loss, Loss{
		Key:    prefix + ".*",
		Reason: ReasonFlattened,
		Detail: fmt.Sprintf("%d indexed message attributes reassembled into one JSON list", len(order)),
	})
	return msgs
}

// toolDefinitions reassembles llm.request.functions.{i}.* into the JSON list
// gen_ai.tool.definitions expects. Traceloop writes the parameter schema as a
// JSON string, which is re-parsed here so the result is one document rather than
// a document with a string of JSON inside it.
func (openLLMetry) toolDefinitions(s Span, p *Parsed) Value {
	groups, order := indexed(s, "llm.request.functions")
	if len(order) == 0 {
		return Value{}
	}
	defs := make([]map[string]any, 0, len(order))
	for _, i := range order {
		g := groups[i]
		d := map[string]any{"type": "function"}
		if v := g["name"]; !v.Empty() {
			d["name"] = v.Str
		}
		if v := g["description"]; !v.Empty() {
			d["description"] = v.Str
		}
		if v := g["parameters"]; !v.Empty() {
			var schema any
			if err := json.Unmarshal([]byte(v.Str), &schema); err == nil {
				d["parameters"] = schema
			} else {
				d["parameters"] = v.Str
			}
		}
		for _, suffix := range slices.Sorted(maps.Keys(g)) {
			p.Consumed = append(p.Consumed, fmt.Sprintf("llm.request.functions.%d.%s", i, suffix))
		}
		defs = append(defs, d)
	}
	b, err := json.Marshal(defs)
	if err != nil {
		return Value{}
	}
	return String(string(b))
}
