// SPDX-License-Identifier: Apache-2.0

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

// The evidence traceloop.entity.name is read under. It is one attribute naming
// three different things, and the kind beside it is the only way to tell which.
const (
	openLLMetryAgentSpan = "traceloop.span.kind=agent"
	openLLMetryToolSpan  = "traceloop.span.kind=tool"
	openLLMetryOtherSpan = "traceloop.span.kind names neither an agent nor a tool"
)

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

// Rules are the OpenLLMetry mappings that can be stated. Most of this dialect
// fits: it adopted the conventions' attribute names for the request parameters
// and the token counts, and where it did not, it is a single rename.
//
// The provider rule deliberately has no lookup table, unlike every other
// dialect here. OpenLLMetry is used through frameworks, and a LangChain span
// says gen_ai.system: langchain -- which is not a provider, it is the thing
// calling one. Passing it through unmapped lets the renderer drop it against
// the registry and record the drop. Aliasing it to something would put a value
// in gen_ai.provider.name that no target defines, and a query grouping by
// provider would grow a bucket that is not a provider.
func (openLLMetry) Rules() []Rule {
	return []Rule{
		{Field: semconv.ProviderName, Keys: []string{"gen_ai.provider.name", "gen_ai.system"},
			Transform: Transform{Lower: true}},

		{Field: semconv.RequestModel, Keys: []string{"gen_ai.request.model"}},
		{Field: semconv.ResponseModel, Keys: []string{"gen_ai.response.model"}},
		{Field: semconv.ResponseID, Keys: []string{"gen_ai.response.id"}},
		{Field: semconv.RequestMaxTokens, Keys: []string{"gen_ai.request.max_tokens"}},
		{Field: semconv.RequestTemperature, Keys: []string{"gen_ai.request.temperature"}},
		{Field: semconv.RequestTopP, Keys: []string{"gen_ai.request.top_p"}},
		{Field: semconv.RequestTopK, Keys: []string{"llm.top_k"}},
		{Field: semconv.RequestFrequencyPenalty, Keys: []string{
			"gen_ai.request.frequency_penalty", "llm.frequency_penalty"}},
		{Field: semconv.RequestPresencePenalty, Keys: []string{
			"gen_ai.request.presence_penalty", "llm.presence_penalty"}},
		{Field: semconv.RequestStopSequences, Keys: []string{"llm.chat.stop_sequences"}},
		// Current OpenLLMetry spells this gen_ai.is_streaming; releases before
		// the migration spell it llm.is_streaming. Both are in the wild.
		{Field: semconv.RequestStream, Keys: []string{"gen_ai.is_streaming", "llm.is_streaming"}},

		{Field: semconv.UsageInputTokens, Keys: []string{"gen_ai.usage.prompt_tokens"}},
		{Field: semconv.UsageOutputTokens, Keys: []string{"gen_ai.usage.completion_tokens"}},
		{Field: semconv.UsageCacheWriteInputTokens, Keys: []string{"gen_ai.usage.cache_creation_input_tokens"}},
		{Field: semconv.UsageCacheReadInputTokens, Keys: []string{"gen_ai.usage.cache_read_input_tokens"}},
		// OpenLLMetry adopted most of the conventions' token names but spells
		// the reasoning count without the .output segment. Same number.
		{Field: semconv.UsageReasoningOutputTokens, Keys: []string{"gen_ai.usage.reasoning_tokens"}},

		{Field: semconv.WorkflowName, Keys: []string{"traceloop.workflow.name"}},
		{Field: semconv.PromptName, Keys: []string{"traceloop.prompt.key"}},
		{Field: semconv.PromptVersion, Keys: []string{"traceloop.prompt.version"}},
	}
}

// Signature is the traceloop namespace, which Score weighs at two. The
// gen_ai.prompt.* and gen_ai.completion.* prefixes it also scores on are
// indexed rather than fixed keys, so they cannot be written down here -- a gate
// cannot name gen_ai.prompt.0.role without guessing how many there are.
func (openLLMetry) Signature() []string {
	return []string{
		"traceloop.span.kind",
		"traceloop.workflow.name",
		"traceloop.entity.name",
		"traceloop.entity.input",
		"traceloop.entity.output",
	}
}

// Unstated. Six of these are the usual reassembly work. gen_ai.operation.name
// is the interesting one, and it is here for a reason worth being precise
// about, because it is not the reason the other five are.
//
// It is not that the operation cannot be described. It is that this emitter
// writes it two ways -- llm.request.type on spans that really called a model,
// traceloop.span.kind on spans that describe a workflow, an agent or a tool --
// and the two need *different lookup tables*. A Rule carries one Transform for
// all of its keys, on purpose: that is what makes it renderable as a flat run
// of conditional assignments in OTTL.
//
// Merging the two tables would work for every span either one appears on, since
// their inputs are disjoint. It would also quietly widen the accepted
// vocabulary in both directions -- llm.request.type: agent would start mapping
// to invoke_agent rather than being reported as a loss -- and it breaks
// outright on a span carrying an unmapped llm.request.type alongside a mapped
// traceloop.span.kind, where the current code records the loss and still sets
// the operation from the kind.
//
// So the choice was: add per-key transforms to Rule and complicate the one type
// this whole argument rests on being simple, or lose one field of export
// coverage for one dialect. The second is cheaper, and the first is the first
// step toward the language this repository exists to argue against building.
func (openLLMetry) Unstated() []semconv.Field {
	return []semconv.Field{
		semconv.OperationName,
		semconv.AgentName,
		semconv.ToolName,
		semconv.InputMessages,
		semconv.OutputMessages,
		semconv.ResponseFinishReasons,
		semconv.ToolDefinitions,
	}
}

// Interpretations are the readings Parse performs beyond the rule table. The
// three for traceloop.entity.name are the ones worth reading: the same key is an
// agent's name, a tool's name, or not enough to say, depending on the kind.
func (openLLMetry) Interpretations() []Interpretation {
	return []Interpretation{
		{Key: "traceloop.span.kind", Meaning: semconv.OperationName, Values: openLLMetrySpanKinds},
		{Key: "llm.request.type", Meaning: semconv.OperationName, Values: openLLMetryOperations},
		{Key: "traceloop.entity.name", When: openLLMetryAgentSpan, Meaning: semconv.AgentName},
		{Key: "traceloop.entity.name", When: openLLMetryToolSpan, Meaning: semconv.ToolName},
		{Key: "traceloop.entity.name", When: openLLMetryOtherSpan,
			Candidates: []semconv.Field{semconv.AgentName, semconv.ToolName}},
		{Key: "gen_ai.prompt.*", Meaning: semconv.InputMessages},
		{Key: "gen_ai.completion.*", Meaning: semconv.OutputMessages},
		{Key: "gen_ai.completion.*.finish_reason", Meaning: semconv.ResponseFinishReasons, Values: finishReasons},
		{Key: "gen_ai.response.finish_reasons", Meaning: semconv.ResponseFinishReasons, Values: finishReasons},
		{Key: "llm.request.functions.*", Meaning: semconv.ToolDefinitions},
	}
}

func (d openLLMetry) Parse(s Span) Parsed {
	var p Parsed

	// Both spellings are read. gen_ai.system is the legacy one; current
	// OpenLLMetry writes gen_ai.provider.name directly, and reading it back is
	// what subjects it to the target's value check rather than letting it
	// through unexamined because it already looks conformant.
	//
	// That check earns its keep here. Instrumenting LangChain through this
	// library puts gen_ai.provider.name=langchain on the chain spans, and
	p.applyRules(s, d.Rules())

	// The two provider spellings are the same fact, so the runner-up is
	// reported rather than passed over in silence. The old code consumed both
	// and recorded neither, which left an attribute that had been read and
	// discarded looking identical to one nothing had examined -- the exact
	// thing this repository exists to complain about elsewhere.
	if s.Has("gen_ai.provider.name") && s.Has("gen_ai.system") {
		p.Lose("gen_ai.system", ReasonNoField,
			"gen_ai.provider.name already carries the provider")
	}

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
			p.setFrom(semconv.OperationName, String(op), "traceloop.span.kind")
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
			p.setFrom(semconv.OperationName, String(op), "llm.request.type")
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
			p.takeWhen(s, "traceloop.entity.name", semconv.AgentName, openLLMetryAgentSpan)
		case "tool":
			p.takeWhen(s, "traceloop.entity.name", semconv.ToolName, openLLMetryToolSpan)
		default:
			detail := "no traceloop.span.kind to say what this name names"
			if kind != "" {
				detail = "traceloop.span.kind " + kind + " names no gen_ai entity"
			}
			p.ambiguous("traceloop.entity.name", []semconv.Field{semconv.AgentName, semconv.ToolName}, detail)
		}
	}

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

	p.rebuildWith(semconv.ToolDefinitions, func() Value { return d.toolDefinitions(s, &p) })
	p.rebuildWith(semconv.InputMessages, func() Value {
		return messagesValue(d.messages(s, "gen_ai.prompt", &p))
	})

	var out []message
	inputs := p.rebuildWith(semconv.OutputMessages, func() Value {
		out = d.messages(s, "gen_ai.completion", &p)
		return messagesValue(out)
	})
	// Current OpenLLMetry writes the list itself, and the emitter's own
	// statement outranks reasons read back out of reassembled completions.
	if !p.takeFinishReasons(s, "gen_ai.response.finish_reasons") {
		p.rebuild(semconv.ResponseFinishReasons, strSeq(reasonsOf(out)), withSuffix(inputs, ".finish_reason"))
	}

	// traceloop. and llm. are this emitter's own namespaces; gen_ai. and openai.
	// are the conventions' and the vendor extension it writes into them.
	p.sweepResidue(s, d, "traceloop.", "llm.", "gen_ai.", "openai.")

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
		m := message{Role: g["role"].Str, FinishReason: finishReason(g["finish_reason"].Str)}
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
