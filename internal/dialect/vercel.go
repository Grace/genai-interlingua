package dialect

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func init() { register(vercel{}) }

// vercel parses spans from the Vercel AI SDK's experimental_telemetry.
//
// It is the odd one out. Every other dialect here speaks a private vocabulary
// that has to be translated; Vercel speaks two at once, and which one you get
// depends on which span you are looking at. The outer span a user's code
// creates (ai.generateText, ai.streamText) carries only ai.*, including the
// pre-rename token counts ai.usage.promptTokens and ai.usage.completionTokens.
// The inner span the provider adapter creates (.doGenerate, .doStream) already
// emits gen_ai.* with the current token spellings.
//
// So the interesting work here is not translation, it is version drift. The
// inner span writes gen_ai.system, which the conventions replaced with
// gen_ai.provider.name, and writes provider ids of its own coining rather than
// the conventions' value set. A span that is already 90% conformant and wrong
// in the remaining 10% is harder to notice in a backend than one that is
// obviously foreign, which is the case for normalizing it rather than waving it
// through.
//
// Attribute names are taken from the AI SDK telemetry reference, which
// enumerates them per span kind.
type vercel struct{}

func (vercel) Name() Name { return Vercel }

// vercelOperations maps ai.operationId onto gen_ai.operation.name. The .do*
// suffix is stripped before lookup, because the inner span is the same
// operation as its parent seen from one layer down, not a different one.
var vercelOperations = map[string]string{
	"ai.generateText": "chat",
	"ai.streamText":   "chat",
	// The object-mode variants are the same model call with a schema attached.
	"ai.generateObject": "chat",
	"ai.streamObject":   "chat",
	"ai.embed":          "embeddings",
	"ai.embedMany":      "embeddings",
	"ai.toolCall":       "execute_tool",
}

// vercelProviders maps the AI SDK's provider package ids onto the conventions'
// provider names. The SDK writes ai.model.provider as <provider id>.<model
// type> -- openai.chat, google.generative-ai, amazon-bedrock.messages -- so
// only the segment before the first dot identifies the provider, and the rest
// says which API surface of it was called. A provider absent from this map
// passes through as the SDK spelled it: several SDK providers (togetherai,
// fireworks, cerebras) have no name in the conventions at all, and inventing
// one here would hide that behind a value the registry does not define. The
// renderer drops it and records the drop.
var vercelProviders = map[string]string{
	"openai":         "openai",
	"azure":          "azure.ai.openai",
	"anthropic":      "anthropic",
	"google":         "gcp.gemini",
	"google-vertex":  "gcp.vertex_ai",
	"amazon-bedrock": "aws.bedrock",
	"mistral":        "mistral_ai",
	"cohere":         "cohere",
	"groq":           "groq",
	"deepseek":       "deepseek",
	"xai":            "x_ai",
	"perplexity":     "perplexity",
}

// Score counts the AI SDK's signature attributes. ai.operationId is worth two
// because it is on every span the SDK emits and nothing else writes it. The
// legacy token counts are worth one each and are the only signal on an outer
// span from an older SDK; the ai. namespace as a whole is worth one because the
// inner span's own evidence is otherwise entirely gen_ai.*, which would leave
// it looking like a conformant span from nobody in particular.
func (vercel) Score(s Span) int {
	n := 0
	if s.Has("ai.operationId") {
		n += 2
	}
	for _, k := range []string{
		"ai.model.provider",
		"ai.usage.promptTokens",
		"ai.usage.completionTokens",
		"ai.prompt.messages",
		"ai.toolCall.name",
		"ai.response.finishReason",
	} {
		if s.Has(k) {
			n++
		}
	}
	if s.HasPrefix("ai.") {
		n++
	}
	return n
}

func (d vercel) Parse(s Span) Parsed {
	var p Parsed

	d.provider(s, &p)
	d.operation(s, &p)

	// gen_ai.* wins where the inner span carries both spellings: it is the
	// adapter's own reading of what it sent, and the ai.* copy on the parent is
	// a summary of it.
	p.TakeFirst(s, semconv.RequestModel, "gen_ai.request.model", "ai.model.id")
	p.TakeFirst(s, semconv.RequestMaxTokens, "gen_ai.request.max_tokens", "ai.settings.maxOutputTokens")
	p.Take(s, "gen_ai.request.temperature", semconv.RequestTemperature)
	p.Take(s, "gen_ai.request.top_p", semconv.RequestTopP)
	p.Take(s, "gen_ai.request.top_k", semconv.RequestTopK)
	p.Take(s, "gen_ai.request.frequency_penalty", semconv.RequestFrequencyPenalty)
	p.Take(s, "gen_ai.request.presence_penalty", semconv.RequestPresencePenalty)
	p.Take(s, "gen_ai.request.stop_sequences", semconv.RequestStopSequences)

	p.TakeFirst(s, semconv.ResponseID, "gen_ai.response.id", "ai.response.id")
	p.TakeFirst(s, semconv.ResponseModel, "gen_ai.response.model", "ai.response.model")

	p.TakeFirst(s, semconv.UsageInputTokens,
		"gen_ai.usage.input_tokens", "ai.usage.promptTokens", "ai.usage.tokens")
	p.TakeFirst(s, semconv.UsageOutputTokens,
		"gen_ai.usage.output_tokens", "ai.usage.completionTokens")

	d.finishReasons(s, &p)
	d.timeToFirstChunk(s, &p)

	p.Take(s, "ai.telemetry.functionId", semconv.WorkflowName)

	// The tool-call span is a different shape from the model spans: four
	// attributes, no messages, no usage.
	p.Take(s, "ai.toolCall.name", semconv.ToolName)
	p.Take(s, "ai.toolCall.id", semconv.ToolCallID)
	p.Take(s, "ai.toolCall.args", semconv.ToolCallArguments)
	p.Take(s, "ai.toolCall.result", semconv.ToolCallResult)

	d.toolDefinitions(s, &p)
	d.inputMessages(s, &p)
	d.outputMessages(s, &p)

	d.losses(s, &p)

	return p
}

// provider reads whichever of the two provider attributes the span carries and
// normalizes the value. gen_ai.system is the inner span's spelling: the
// conventions renamed that attribute to gen_ai.provider.name, and the SDK still
// writes the old one, so the rename is applied here rather than being mistaken
// for a provider the registry does not know.
func (vercel) provider(s Span, p *Parsed) {
	key := "ai.model.provider"
	v, ok := s.Attr(key)
	if !ok {
		key = "gen_ai.system"
		if v, ok = s.Attr(key); !ok {
			return
		}
	}
	id, _, _ := strings.Cut(strings.ToLower(v.Str), ".")
	if name, ok := vercelProviders[id]; ok {
		p.Set(semconv.ProviderName, String(name))
	} else {
		p.Set(semconv.ProviderName, String(id))
	}
	p.Consumed = append(p.Consumed, key)
}

// operation maps ai.operationId, stripping the adapter-level suffix first.
func (vercel) operation(s Span, p *Parsed) {
	v, ok := s.Attr("ai.operationId")
	if !ok {
		return
	}
	p.Consumed = append(p.Consumed, "ai.operationId")
	base := v.Str
	for _, suffix := range []string{".doGenerate", ".doStream", ".doEmbed"} {
		base = strings.TrimSuffix(base, suffix)
	}
	if op, ok := vercelOperations[base]; ok {
		p.Set(semconv.OperationName, String(op))
		return
	}
	p.Loss = append(p.Loss, Loss{Key: "ai.operationId", Reason: ReasonNoField,
		Detail: "no gen_ai.operation.name value for " + v.Str})
}

// vercelFinishReasons maps the SDK's hyphenated finish reasons onto the
// spelling every other emitter here uses. gen_ai.response.finish_reasons has no
// closed value set in the registry, so nothing downstream would reject
// tool-calls -- it would simply sit in a backend beside OpenLLMetry's
// tool_calls as a second spelling of one concept, which is the exact thing a
// span normalizer exists to prevent. A reason absent from this map passes
// through unchanged; error, other and unknown are the SDK's own and have no
// counterpart to be renamed to.
var vercelFinishReasons = map[string]string{
	"tool-calls":     "tool_calls",
	"content-filter": "content_filter",
}

// finishReasons lifts the SDK's single finish reason into the list the
// conventions define. gen_ai.response.finish_reasons on the inner span is
// already that list; ai.response.finishReason on the outer one is one string.
func (vercel) finishReasons(s Span, p *Parsed) {
	if v, ok := s.Attr("gen_ai.response.finish_reasons"); ok {
		p.Consumed = append(p.Consumed, "gen_ai.response.finish_reasons")
		reasons := make([]string, 0, len(v.StrSeq))
		for _, r := range v.StrSeq {
			reasons = append(reasons, vercelFinishReason(r))
		}
		if v.Kind == KindStr {
			reasons = append(reasons, vercelFinishReason(v.Str))
		}
		p.Set(semconv.ResponseFinishReasons, strSeq(reasons))
		return
	}
	if v, ok := s.Attr("ai.response.finishReason"); ok {
		p.Set(semconv.ResponseFinishReasons, strSeq([]string{vercelFinishReason(v.Str)}))
		p.Consumed = append(p.Consumed, "ai.response.finishReason")
	}
}

func vercelFinishReason(r string) string {
	if mapped, ok := vercelFinishReasons[r]; ok {
		return mapped
	}
	return r
}

// timeToFirstChunk converts the SDK's milliseconds into the seconds the
// conventions specify. This is a unit change and not a loss: every millisecond
// value in int64 range is exact in a float64 after dividing by 1000, so nothing
// is rounded away and interlingua.lossy would be crying wolf if it named this.
func (vercel) timeToFirstChunk(s Span, p *Parsed) {
	v, ok := s.Attr("ai.response.msToFirstChunk")
	if !ok {
		return
	}
	ms, ok := v.Float64()
	if !ok {
		p.Lose("ai.response.msToFirstChunk", ReasonCoerced, "value is not a number")
		return
	}
	p.Set(semconv.ResponseTimeToFirstChunk, Float(ms/1000))
	p.Consumed = append(p.Consumed, "ai.response.msToFirstChunk")
}

// vercelMessage is one entry of ai.prompt.messages. The SDK's content is either
// a bare string or a list of typed parts, and both spellings appear in the same
// array depending on how the caller built the prompt.
type vercelMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type vercelPart struct {
	Type       string `json:"type"`
	Text       string `json:"text"`
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	Args       any    `json:"args"`
	Input      any    `json:"input"`
}

// inputMessages reads ai.prompt.messages, the inner span's fully resolved
// message array. The outer span's ai.prompt is deliberately not read for this:
// it is whatever the caller passed, which may be a bare prompt string with the
// system message still held separately, and reconstructing the real message
// list from it would be guessing at what the SDK did next.
func (d vercel) inputMessages(s Span, p *Parsed) {
	v, ok := s.Attr("ai.prompt.messages")
	if !ok {
		if s.Has("ai.prompt") {
			p.Lose("ai.prompt", ReasonUnstructured,
				"the caller's own prompt shape, resolved into ai.prompt.messages on the adapter span")
		}
		return
	}
	p.Consumed = append(p.Consumed, "ai.prompt.messages")

	var raw []vercelMessage
	if err := json.Unmarshal([]byte(v.Str), &raw); err != nil {
		p.Loss = append(p.Loss, Loss{Key: "ai.prompt.messages", Reason: ReasonUnstructured,
			Detail: "value is not a JSON message array"})
		return
	}

	msgs := make([]message, 0, len(raw))
	for i, m := range raw {
		msgs = append(msgs, message{
			Role:  m.Role,
			Parts: d.parts(m.Content, fmt.Sprintf("ai.prompt.messages[%d]", i), p),
		})
	}
	p.Set(semconv.InputMessages, messagesValue(msgs))
}

// parts converts one message's content into the typed parts the conventions
// define, whether it arrived as a bare string or as the SDK's own part list.
func (vercel) parts(content json.RawMessage, where string, p *Parsed) []part {
	if len(content) == 0 {
		return nil
	}

	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		if text == "" {
			return nil
		}
		return []part{textPart(text)}
	}

	var raw []vercelPart
	if err := json.Unmarshal(content, &raw); err != nil {
		p.Loss = append(p.Loss, Loss{Key: where, Reason: ReasonUnstructured,
			Detail: "content is neither a string nor a part list"})
		return nil
	}

	var parts []part
	for _, e := range raw {
		switch e.Type {
		case "text":
			parts = append(parts, textPart(e.Text))
		case "tool-call":
			args := e.Args
			if args == nil {
				args = e.Input
			}
			parts = append(parts, part{Type: "tool_call", ID: e.ToolCallID, Name: e.ToolName, Arguments: args})
		case "tool-result":
			// The conventions carry a tool result on the execute_tool span, in
			// gen_ai.tool.call.result, not as a part of the message that
			// reported it back to the model.
			p.Loss = append(p.Loss, Loss{Key: where, Reason: ReasonNoField,
				Detail: "no message part type for tool-result"})
		default:
			p.Loss = append(p.Loss, Loss{Key: where, Reason: ReasonNoField,
				Detail: "no message part type for " + e.Type})
		}
	}
	return parts
}

// outputMessages assembles the single assistant message the SDK reports across
// two attributes: the text in one, the tool calls in another.
func (vercel) outputMessages(s Span, p *Parsed) {
	var parts []part

	if v, ok := s.Attr("ai.response.text"); ok {
		p.Consumed = append(p.Consumed, "ai.response.text")
		if v.Str != "" {
			parts = append(parts, textPart(v.Str))
		}
	}

	if v, ok := s.Attr("ai.response.toolCalls"); ok {
		p.Consumed = append(p.Consumed, "ai.response.toolCalls")
		var calls []vercelPart
		if err := json.Unmarshal([]byte(v.Str), &calls); err != nil {
			p.Loss = append(p.Loss, Loss{Key: "ai.response.toolCalls",
				Reason: ReasonUnstructured, Detail: "value is not a JSON tool call array"})
		} else {
			for _, c := range calls {
				args := c.Args
				if args == nil {
					args = c.Input
				}
				parts = append(parts, part{Type: "tool_call", ID: c.ToolCallID, Name: c.ToolName, Arguments: args})
			}
		}
	}

	if len(parts) == 0 {
		return
	}
	p.Set(semconv.OutputMessages, messagesValue([]message{{Role: "assistant", Parts: parts}}))
}

// toolDefinitions lifts ai.prompt.tools. The SDK writes it as an array of
// strings, each one a separately stringified tool definition, so each element
// is re-parsed rather than the array as a whole.
func (vercel) toolDefinitions(s Span, p *Parsed) {
	v, ok := s.Attr("ai.prompt.tools")
	if !ok {
		return
	}
	p.Consumed = append(p.Consumed, "ai.prompt.tools")

	var encoded []string
	switch v.Kind {
	case KindStrSeq:
		encoded = v.StrSeq
	case KindStr:
		encoded = []string{v.Str}
	}

	defs := make([]any, 0, len(encoded))
	for _, e := range encoded {
		var def any
		if err := json.Unmarshal([]byte(e), &def); err != nil {
			p.Loss = append(p.Loss, Loss{Key: "ai.prompt.tools", Reason: ReasonUnstructured,
				Detail: "a tool definition is not JSON"})
			continue
		}
		defs = append(defs, def)
	}
	if len(defs) == 0 {
		return
	}
	b, err := json.Marshal(defs)
	if err != nil {
		return
	}
	p.Set(semconv.ToolDefinitions, String(string(b)))
}

// losses records the attributes the SDK writes that the conventions have no
// room for. ai.request.headers.* is listed rather than mapped on purpose: the
// conventions carry no request header attribute, and a provider's headers are
// where its API key lives.
func (vercel) losses(s Span, p *Parsed) {
	for _, k := range []string{
		"ai.settings.maxRetries",
		"ai.response.timestamp",
		"ai.response.msToFinish",
		"ai.response.avgCompletionTokensPerSecond",
		"ai.prompt.toolChoice",
		"ai.schema",
		"ai.schema.name",
		"ai.schema.description",
	} {
		if s.Has(k) {
			p.Lose(k, ReasonNoField, "the conventions name no equivalent")
		}
	}

	for _, k := range []string{"ai.response.providerMetadata", "ai.response.object", "ai.value", "ai.values"} {
		if s.Has(k) {
			p.Lose(k, ReasonUnstructured, "provider-shaped payload with no schema to map onto")
		}
	}

	// The conventions carry no attribute for an embedding vector at any version.
	for _, k := range []string{"ai.embedding", "ai.embeddings"} {
		if s.Has(k) {
			p.Lose(k, ReasonUnstructured, "the conventions carry no embedding payload attribute")
		}
	}

	for _, k := range s.Keys() {
		switch {
		case strings.HasPrefix(k, "ai.request.headers."):
			p.Lose(k, ReasonNoField, "the conventions carry no request headers, and provider headers carry credentials")
		case strings.HasPrefix(k, "ai.settings.runtimeContext."):
			p.Lose(k, ReasonNoField, "runtime context is the SDK's own correlation dimension")
		}
	}
}
