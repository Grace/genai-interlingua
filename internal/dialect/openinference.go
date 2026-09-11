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

func init() { register(openInference{}) }

// openInference parses spans from OpenInference, the convention Arize Phoenix
// defines and the one LangChain emits when instrumented through Phoenix rather
// than through OpenLLMetry. It does not use the gen_ai namespace at all: its
// attributes live under llm.*, and its span taxonomy lives in a single
// openinference.span.kind attribute rather than in gen_ai.operation.name.
type openInference struct{}

func (openInference) Name() Name { return OpenInference }

// openInferenceOperations maps openinference.span.kind onto
// gen_ai.operation.name. RERANKER, GUARDRAIL and EVALUATOR are absent: the
// conventions name no operation for any of them, and an evaluator span is
// modelled as an event in the new repository rather than as an operation.
var openInferenceOperations = map[string]string{
	"LLM":       "chat",
	"CHAIN":     "invoke_workflow",
	"AGENT":     "invoke_agent",
	"TOOL":      "execute_tool",
	"EMBEDDING": "embeddings",
	"RETRIEVER": "retrieval",
}

// openInferenceProviders are the provider names that need translating.
// OpenInference names providers after the company; the conventions name them
// after the API surface, which is why one company can appear twice there.
// Anything absent from this map passes through lowercased, which is correct for
// openai, anthropic, cohere, deepseek and groq.
var openInferenceProviders = map[string]string{
	"aws":       "aws.bedrock",
	"azure":     "azure.ai.openai",
	"google":    "gcp.gemini",
	"vertexai":  "gcp.vertex_ai",
	"mistralai": "mistral_ai",
	"xai":       "x_ai",
}

// Score counts OpenInference's signature attributes. Its span kind is worth two
// because no other emitter names its taxonomy in one attribute; the llm.* keys
// are worth one each because OpenLLMetry also writes into that namespace, just
// never these members of it.
func (openInference) Score(s Span) int {
	n := 0
	if s.Has("openinference.span.kind") {
		n += 2
	}
	for _, k := range []string{
		"llm.model_name",
		"llm.provider",
		"llm.token_count.prompt",
		"llm.token_count.completion",
		"llm.invocation_parameters",
	} {
		if s.Has(k) {
			n++
		}
	}
	if s.HasPrefix("llm.input_messages.") || s.HasPrefix("llm.output_messages.") {
		n++
	}
	return n
}

// Rules are the OpenInference mappings that can be stated.
//
// Two of them repay a close look, because they are the two places where a
// precedence written in Go as "whichever line runs last wins" becomes, in a
// table, "whichever key is listed first wins" -- and the order therefore
// reverses on the page while the behaviour stays put.
func (openInference) Rules() []Rule {
	return []Rule{
		// A span kind this table does not name is not a kind spelled unusually,
		// it is a concept the conventions have no word for, so it loses rather
		// than passing through. UnmatchedLose's detail is byte-identical to the
		// one this dialect used to write by hand.
		{
			Field:     semconv.OperationName,
			Keys:      []string{"openinference.span.kind"},
			Transform: Transform{Map: openInferenceOperations, MapUnmatched: UnmatchedLose},
		},
		// llm.provider names the hosting provider and llm.system the API
		// surface, which differ for a model served through Bedrock or Azure.
		// Where both are present, provider is the one gen_ai.provider.name
		// means. Where only system is -- which a capture of the real OpenAI
		// instrumentation does -- reading it is a better answer than discarding
		// the provider because a better-named attribute usually exists.
		{
			Field:     semconv.ProviderName,
			Keys:      []string{"llm.provider", "llm.system"},
			Transform: Transform{Lower: true, Map: openInferenceProviders},
		},

		{Field: semconv.RequestModel, Keys: []string{"llm.model_name", "embedding.model_name"}},

		// The conventions make this a list because a request for several choices
		// has one reason per choice; OpenInference carries a single value, so
		// the list has one element rather than being synthesised from
		// per-message reasons.
		{Field: semconv.ResponseFinishReasons, Keys: []string{"llm.finish_reason"},
			Transform: Transform{ToList: true}},

		{Field: semconv.UsageInputTokens, Keys: []string{"llm.token_count.prompt"}},
		{Field: semconv.UsageOutputTokens, Keys: []string{"llm.token_count.completion"}},
		{Field: semconv.UsageCacheReadInputTokens, Keys: []string{"llm.token_count.prompt_details.cache_read"}},
		{Field: semconv.UsageCacheWriteInputTokens, Keys: []string{"llm.token_count.prompt_details.cache_write"}},
		{Field: semconv.UsageAudioInputTokens, Keys: []string{"llm.token_count.prompt_details.audio"}},
		{Field: semconv.UsageReasoningOutputTokens, Keys: []string{"llm.token_count.completion_details.reasoning"}},
		{Field: semconv.UsageAudioOutputTokens, Keys: []string{"llm.token_count.completion_details.audio"}},

		{Field: semconv.ConversationID, Keys: []string{"session.id"}},

		// tool_call.function.name is the call actually made and tool.name the
		// definition it was made against; on a span carrying both, the call is
		// the more specific and wins. In the imperative version it won by being
		// assigned second. Here it wins by being listed first, which is the same
		// decision written the other way up.
		{Field: semconv.ToolName, Keys: []string{"tool_call.function.name", "tool.name"}},
		{Field: semconv.ToolDescription, Keys: []string{"tool.description"}},
		{Field: semconv.ToolCallID, Keys: []string{"tool_call.id"}},
		{Field: semconv.ToolCallArguments, Keys: []string{"tool_call.function.arguments"}},
	}
}

// Signature is openinference.span.kind, which Score weighs at two and which
// nothing else writes. The llm.* keys it also scores on are already in the rule
// table under their own spellings, so they reach the gate that way.
func (openInference) Signature() []string {
	return []string{"openinference.span.kind"}
}

// Unstated is unusually long here, and the reason is one attribute.
//
// OpenInference packs every sampling setting into llm.invocation_parameters as
// a JSON string. Temperature, max tokens, top_p, top_k, the penalties, the seed,
// the choice count, streaming, stop sequences -- ten fields, none of which is a
// transformation of an attribute, all of which require parsing a blob and
// lifting named members out of it. So an exported config for this dialect
// carries the model, the provider, the operation, the token counts and the
// session, and not one request parameter. That is a real hole and it is better
// stated than averaged.
//
// ToolCallArguments is here as well as in the rules above: on a span whose kind
// is TOOL, input.value is the arguments, and which attribute means what depends
// on a sibling. The export carries the tool_call.function.arguments half only.
func (openInference) Unstated() []semconv.Field {
	return []semconv.Field{
		semconv.RequestTemperature,
		semconv.RequestMaxTokens,
		semconv.RequestTopP,
		semconv.RequestTopK,
		semconv.RequestFrequencyPenalty,
		semconv.RequestPresencePenalty,
		semconv.RequestSeed,
		semconv.RequestChoiceCount,
		semconv.RequestStream,
		semconv.RequestStopSequences,
		semconv.InputMessages,
		semconv.OutputMessages,
		semconv.ToolDefinitions,
		semconv.ToolCallArguments,
		semconv.ToolCallResult,
		semconv.RetrievalDocuments,
	}
}

func (d openInference) Parse(s Span) Parsed {
	var p Parsed

	p.applyRules(s, d.Rules())

	// The span kind is read again rather than kept from the rules, because two
	// mappings below turn on what it says rather than on what it is worth.
	kind := ""
	if v, ok := s.Attr("openinference.span.kind"); ok {
		kind = v.Str
	}

	// llm.system is redundant only when llm.provider actually won. The rule
	// above consumes whichever it used and says nothing about the other, so the
	// runner-up is reported here -- the same shape LiteLLM uses for
	// llm.request.type.
	if s.Has("llm.provider") && s.Has("llm.system") {
		p.Lose("llm.system", ReasonNoField,
			"llm.system names the API surface, which gen_ai.provider.name already carries")
	}

	if s.Has("llm.token_count.total") {
		p.Lose("llm.token_count.total", ReasonNoField,
			"the conventions carry input and output counts only")
	}

	for _, k := range []string{"user.id", "metadata", "tag.tags"} {
		if s.Has(k) {
			p.Lose(k, ReasonNoField, "the conventions name no equivalent")
		}
	}

	if v, ok := s.Attr("llm.invocation_parameters"); ok {
		d.invocationParameters(v.Str, &p)
	}

	if s.Has("tool.parameters") {
		p.Lose("tool.parameters", ReasonNoField,
			"gen_ai.tool.definitions describes the tools offered to the model, not the schema of the tool being executed")
	}

	// On a TOOL span the generic payload attributes are the tool call: Phoenix
	// has no dedicated attribute for the arguments a tool was actually invoked
	// with. On any other kind they are a free-form blob with no schema.
	if kind == "TOOL" {
		p.Take(s, "input.value", semconv.ToolCallArguments)
		p.Take(s, "output.value", semconv.ToolCallResult)
	} else {
		for _, k := range []string{"input.value", "output.value"} {
			if s.Has(k) {
				p.Lose(k, ReasonUnstructured, "free-form span payload with no schema to map onto")
			}
		}
	}
	for _, k := range []string{"input.mime_type", "output.mime_type"} {
		if s.Has(k) {
			p.Lose(k, ReasonNoField, "the conventions do not type the span payload")
		}
	}

	p.Set(semconv.ToolDefinitions, d.toolDefinitions(s, &p))
	p.Set(semconv.InputMessages, messagesValue(d.messages(s, "llm.input_messages", &p)))
	p.Set(semconv.OutputMessages, messagesValue(d.messages(s, "llm.output_messages", &p)))
	p.Set(semconv.RetrievalDocuments, d.documents(s, &p))

	// embedding.embeddings.{i}.embedding.vector is a float sequence, and the
	// conventions carry no attribute for the vector itself at any version.
	groups, order := indexed(s, "embedding.embeddings")
	for _, i := range order {
		for _, suffix := range slices.Sorted(maps.Keys(groups[i])) {
			p.Lose(fmt.Sprintf("embedding.embeddings.%d.%s", i, suffix), ReasonUnstructured,
				"the conventions carry no embedding payload attribute")
		}
	}

	p.sweepResidue(s, d, "openinference.", "llm.", "embedding.", "tool.", "tool_call.", "gen_ai.")

	return p
}

// invocationParameters lifts the request settings OpenInference buries in a JSON
// blob. The conventions name each of these individually, so leaving them in the
// blob would be a loss the dialect could have avoided; what is left over after
// lifting is recorded as one unstructured loss naming the keys that stayed.
func (openInference) invocationParameters(raw string, p *Parsed) {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		p.Lose("llm.invocation_parameters", ReasonUnstructured, "value is not a JSON object")
		return
	}
	lifted := map[string]semconv.Field{
		"temperature":       semconv.RequestTemperature,
		"max_tokens":        semconv.RequestMaxTokens,
		"max_output_tokens": semconv.RequestMaxTokens,
		"top_p":             semconv.RequestTopP,
		"top_k":             semconv.RequestTopK,
		"frequency_penalty": semconv.RequestFrequencyPenalty,
		"presence_penalty":  semconv.RequestPresencePenalty,
		"seed":              semconv.RequestSeed,
		"n":                 semconv.RequestChoiceCount,
		"stream":            semconv.RequestStream,
		"stop":              semconv.RequestStopSequences,
	}
	var left []string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		f, ok := lifted[k]
		if !ok {
			left = append(left, k)
			continue
		}
		if v := jsonValue(m[k]); !v.Empty() {
			p.liftFrom(f, v, "llm.invocation_parameters")
		} else {
			left = append(left, k)
		}
	}
	p.Consumed = append(p.Consumed, "llm.invocation_parameters")
	if len(left) > 0 {
		p.Loss = append(p.Loss, Loss{Key: "llm.invocation_parameters",
			Reason: ReasonUnstructured, Detail: "no gen_ai field for " + strings.Join(left, ", ")})
	}
}

// messages reassembles OpenInference's indexed message attributes. Phoenix nests
// everything one level deeper than OpenLLMetry does, under a message. suffix,
// and splits content into an optional contents.{j} list of typed parts alongside
// the flat content string.
func (openInference) messages(s Span, prefix string, p *Parsed) []message {
	groups, order := indexed(s, prefix)
	if len(order) == 0 {
		return nil
	}
	var msgs []message
	for _, i := range order {
		g := groups[i]
		m := message{Role: g["message.role"].Str}
		if c := g["message.content"]; !c.Empty() {
			m.Parts = append(m.Parts, textPart(c.Str))
		}
		for _, j := range subIndices(g, "message.contents.") {
			typ := g[fmt.Sprintf("message.contents.%d.message_content.type", j)].Str
			if typ != "text" {
				p.Loss = append(p.Loss, Loss{
					Key:    fmt.Sprintf("%s.%d.message.contents.%d", prefix, i, j),
					Reason: ReasonNoField,
					Detail: "no message part type for " + typ,
				})
				continue
			}
			m.Parts = append(m.Parts,
				textPart(g[fmt.Sprintf("message.contents.%d.message_content.text", j)].Str))
		}
		for _, j := range subIndices(g, "message.tool_calls.") {
			m.Parts = append(m.Parts, toolCallPart(
				g[fmt.Sprintf("message.tool_calls.%d.tool_call.id", j)].Str,
				g[fmt.Sprintf("message.tool_calls.%d.tool_call.function.name", j)].Str,
				g[fmt.Sprintf("message.tool_calls.%d.tool_call.function.arguments", j)].Str,
			))
		}
		for _, suffix := range slices.Sorted(maps.Keys(g)) {
			p.Consumed = append(p.Consumed, fmt.Sprintf("%s.%d.%s", prefix, i, suffix))
		}
		msgs = append(msgs, m)
	}
	p.Loss = append(p.Loss, Loss{
		Key:    prefix + ".*",
		Reason: ReasonFlattened,
		Detail: fmt.Sprintf("%d indexed message attributes reassembled into one JSON list", len(order)),
	})
	return msgs
}

// toolDefinitions lifts llm.tools.{i}.tool.json_schema. Phoenix already carries
// each tool as a JSON schema document, so this is a re-parse and a re-wrap
// rather than a translation.
func (openInference) toolDefinitions(s Span, p *Parsed) Value {
	groups, order := indexed(s, "llm.tools")
	if len(order) == 0 {
		return Value{}
	}
	defs := make([]any, 0, len(order))
	for _, i := range order {
		g := groups[i]
		raw := g["tool.json_schema"]
		for _, suffix := range slices.Sorted(maps.Keys(g)) {
			p.Consumed = append(p.Consumed, fmt.Sprintf("llm.tools.%d.%s", i, suffix))
		}
		if raw.Empty() {
			continue
		}
		var schema any
		if err := json.Unmarshal([]byte(raw.Str), &schema); err != nil {
			p.Loss = append(p.Loss, Loss{
				Key:    fmt.Sprintf("llm.tools.%d.tool.json_schema", i),
				Reason: ReasonUnstructured,
				Detail: "value is not JSON",
			})
			continue
		}
		defs = append(defs, schema)
	}
	if len(defs) == 0 {
		return Value{}
	}
	b, err := json.Marshal(defs)
	if err != nil {
		return Value{}
	}
	return String(string(b))
}

// documents reassembles retrieval.documents.{i}.document.* into the JSON list
// gen_ai.retrieval.documents expects. Phoenix carries per-document metadata that
// the conventions have no room for, recorded once rather than once per document.
func (openInference) documents(s Span, p *Parsed) Value {
	groups, order := indexed(s, "retrieval.documents")
	if len(order) == 0 {
		return Value{}
	}
	docs := make([]map[string]any, 0, len(order))
	metadata := false
	for _, i := range order {
		g := groups[i]
		d := make(map[string]any)
		if v := g["document.id"]; !v.Empty() {
			d["id"] = v.Str
		}
		if v := g["document.content"]; !v.Empty() {
			d["content"] = v.Str
		}
		if f, ok := g["document.score"].Float64(); ok {
			d["score"] = f
		}
		if !g["document.metadata"].Empty() {
			metadata = true
		}
		for _, suffix := range slices.Sorted(maps.Keys(g)) {
			p.Consumed = append(p.Consumed, fmt.Sprintf("retrieval.documents.%d.%s", i, suffix))
		}
		docs = append(docs, d)
	}
	if metadata {
		p.Loss = append(p.Loss, Loss{
			Key:    "retrieval.documents.*.document.metadata",
			Reason: ReasonNoField,
			Detail: "gen_ai.retrieval.documents has no per-document metadata slot",
		})
	}
	b, err := json.Marshal(docs)
	if err != nil {
		return Value{}
	}
	return String(string(b))
}
