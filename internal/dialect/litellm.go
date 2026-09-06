package dialect

import (
	"strings"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func init() { register(liteLLM{}) }

// liteLLM parses spans from LiteLLM's OpenTelemetry integration.
//
// It is a hybrid, and the halves come from different eras. Token counts and
// messages are current: gen_ai.usage.input_tokens, and gen_ai.input.messages
// carrying the conventions' own JSON message shape. Tool definitions and
// completions are traceloop-shaped and indexed, llm.request.functions.{i}.* and
// gen_ai.completion.{i}.*, because that is what the integration was built on
// before the conventions had an opinion. It emits gen_ai.system or
// gen_ai.provider.name depending on a flag, having implemented the rename
// without dropping the old spelling.
//
// The traceloop half is why detection has to be scored rather than
// first-match. A LiteLLM span and an OpenLLMetry span can carry the same
// llm.request.type and the same indexed completion attributes; what separates
// them is gen_ai.framework and the litellm.* namespace, and those have to
// outweigh the shared vocabulary rather than merely tie with it.
//
// Attribute names are taken from litellm/integrations/opentelemetry.py and
// litellm/integrations/opentelemetry_utils/gen_ai_semconv.py on main.
type liteLLM struct{}

func (liteLLM) Name() Name { return LiteLLM }

// liteLLMProviders maps LiteLLM's custom_llm_provider values onto the
// conventions' provider names. LiteLLM names a provider after its own routing
// prefix, which is shorter than the conventions' name wherever one company
// fronts several API surfaces.
var liteLLMProviders = map[string]string{
	"azure":        "azure.ai.openai",
	"azure_ai":     "azure.ai.inference",
	"bedrock":      "aws.bedrock",
	"vertex_ai":    "gcp.vertex_ai",
	"gemini":       "gcp.gemini",
	"mistral":      "mistral_ai",
	"xai":          "x_ai",
	"watsonx":      "ibm.watsonx.ai",
	"watsonx_text": "ibm.watsonx.ai",
}

// liteLLMOperations maps the call types LiteLLM writes into
// gen_ai.operation.name. It writes the call type through unchanged in its
// pre-semconv mode, and the async variants are the same operation as their
// synchronous twins. Anything absent passes through for the renderer to check
// against the target's value set.
var liteLLMOperations = map[string]string{
	"completion":       "chat",
	"acompletion":      "chat",
	"text_completion":  "text_completion",
	"atext_completion": "text_completion",
	"embedding":        "embeddings",
	"aembedding":       "embeddings",
}

// Score counts what only LiteLLM writes, which is a deliberately short list.
// gen_ai.framework is worth three and is checked by value rather than presence:
// the attribute names whichever library built the span, so it identifies
// LiteLLM only when it says litellm. The litellm. namespace is worth two, and
// the proxy's tenancy attributes one.
//
// Nothing else counts. Not the traceloop vocabulary LiteLLM inherited, because
// that is evidence of the lineage rather than of this emitter. And pointedly
// not gen_ai.usage.input_tokens and its neighbours: those are the conventions'
// own attributes, written by everything conformant, so scoring them here would
// have this dialect claiming a share of the evidence on every well-behaved span
// in the trace. It would not usually win on that, but it would shrink the
// margin of whichever dialect did, and the margin is what gets written to the
// span as interlingua.dialect.confidence. Detection has to answer "who wrote
// this" and not "who could have".
func (liteLLM) Score(s Span) int {
	n := 0
	if v, ok := s.Attr("gen_ai.framework"); ok && strings.EqualFold(v.Str, "litellm") {
		n += 3
	}
	if s.HasPrefix("litellm.") {
		n += 2
	}
	if s.HasPrefix("metadata.user_api_key") {
		n++
	}
	return n
}

func (d liteLLM) Parse(s Span) Parsed {
	var p Parsed

	if s.Has("gen_ai.framework") {
		// The signature attribute itself. It is read, and there is nothing in
		// the conventions to put it in, but it is not a loss worth reporting:
		// interlingua.dialect already says which library produced the span, and
		// says it in this repository's own vocabulary.
		p.Consumed = append(p.Consumed, "gen_ai.framework")
	}

	d.provider(s, &p)

	p.Take(s, "gen_ai.request.model", semconv.RequestModel)
	p.Take(s, "gen_ai.response.model", semconv.ResponseModel)
	p.Take(s, "gen_ai.request.max_tokens", semconv.RequestMaxTokens)
	p.Take(s, "gen_ai.request.temperature", semconv.RequestTemperature)
	p.Take(s, "gen_ai.request.top_p", semconv.RequestTopP)
	p.Take(s, "gen_ai.request.top_k", semconv.RequestTopK)
	p.Take(s, "gen_ai.request.seed", semconv.RequestSeed)
	p.Take(s, "gen_ai.request.frequency_penalty", semconv.RequestFrequencyPenalty)
	p.Take(s, "gen_ai.request.presence_penalty", semconv.RequestPresencePenalty)
	p.Take(s, "gen_ai.request.stop_sequences", semconv.RequestStopSequences)
	p.Take(s, "gen_ai.request.stream", semconv.RequestStream)
	p.Take(s, "gen_ai.request.choice.count", semconv.RequestChoiceCount)

	p.Take(s, "gen_ai.usage.input_tokens", semconv.UsageInputTokens)
	p.Take(s, "gen_ai.usage.output_tokens", semconv.UsageOutputTokens)
	// LiteLLM already writes these two under the keys the conventions use.
	p.Take(s, "gen_ai.usage.cache_creation.input_tokens", semconv.UsageCacheWriteInputTokens)
	p.Take(s, "gen_ai.usage.cache_read.input_tokens", semconv.UsageCacheReadInputTokens)
	if s.Has("gen_ai.usage.total_tokens") {
		p.Lose("gen_ai.usage.total_tokens", ReasonNoField,
			"the conventions carry input and output counts only")
	}

	d.operation(s, &p)
	d.messages(s, &p)

	p.Take(s, "gen_ai.system_instructions", semconv.SystemInstructions)
	p.Take(s, "gen_ai.conversation.id", semconv.ConversationID)

	// The tool definitions are traceloop-shaped, so they are read by the
	// OpenLLMetry dialect's own reassembly rather than by a second copy of it.
	// The two emitters genuinely share this vocabulary; duplicating the parser
	// would let the two copies drift apart over exactly the attributes that
	// make detection hard in the first place.
	p.Set(semconv.ToolDefinitions, openLLMetry{}.toolDefinitions(s, &p))

	d.losses(s, &p)

	return p
}

// provider reads whichever spelling this LiteLLM build emits. The integration
// writes gen_ai.provider.name when configured for the newer conventions and
// gen_ai.system otherwise, so both are the same fact and the newer one wins.
func (liteLLM) provider(s Span, p *Parsed) {
	key := "gen_ai.provider.name"
	v, ok := s.Attr(key)
	if !ok {
		key = "gen_ai.system"
		if v, ok = s.Attr(key); !ok {
			return
		}
	}
	name := strings.ToLower(v.Str)
	if alias, ok := liteLLMProviders[name]; ok {
		name = alias
	}
	p.Set(semconv.ProviderName, String(name))
	p.Consumed = append(p.Consumed, key)
}

// operation prefers gen_ai.operation.name and falls back to llm.request.type,
// which is the traceloop attribute LiteLLM inherited and still writes.
func (liteLLM) operation(s Span, p *Parsed) {
	key := "gen_ai.operation.name"
	v, ok := s.Attr(key)
	if !ok {
		key = "llm.request.type"
		if v, ok = s.Attr(key); !ok {
			return
		}
	}
	p.Consumed = append(p.Consumed, key)
	if op, ok := liteLLMOperations[v.Str]; ok {
		p.Set(semconv.OperationName, String(op))
		return
	}
	// Not mapped is not the same as not carried: LiteLLM routes calls the
	// conventions have no operation for, and the renderer is where a value the
	// target does not define gets dropped.
	p.Set(semconv.OperationName, String(v.Str))
}

// messages prefers the conventions' own JSON attributes, which LiteLLM writes
// directly, and falls back to the indexed traceloop attributes an older build
// or a non-semconv configuration produces.
func (liteLLM) messages(s Span, p *Parsed) {
	if !p.Take(s, "gen_ai.input.messages", semconv.InputMessages) {
		p.Set(semconv.InputMessages, messagesValue(openLLMetry{}.messages(s, "gen_ai.prompt", p)))
	}

	if p.Take(s, "gen_ai.output.messages", semconv.OutputMessages) {
		return
	}
	out := openLLMetry{}.messages(s, "gen_ai.completion", p)
	p.Set(semconv.OutputMessages, messagesValue(out))
	var reasons []string
	for _, m := range out {
		if m.FinishReason != "" {
			reasons = append(reasons, m.FinishReason)
		}
	}
	p.Set(semconv.ResponseFinishReasons, strSeq(reasons))
}

// losses records what LiteLLM says that the conventions do not name. The
// metadata.* namespace is the proxy's own tenancy dimensions -- team ids, key
// aliases, budgets -- which is exactly the sort of thing an operator wants and
// the conventions have no slot for.
func (liteLLM) losses(s Span, p *Parsed) {
	for _, k := range []string{
		"llm.request.type",
		"gen_ai.request.id",
		"gen_ai.openai.request.service_tier",
		"gen_ai.openai.response.service_tier",
		"litellm.model_group",
		"litellm.provider.model",
		"litellm.preprocessing.duration_ms",
		"litellm.team.metadata",
	} {
		// llm.request.type is consumed above when it stood in for the
		// operation, and a key that was used is not also a key that was lost.
		if k == "llm.request.type" && s.Has("gen_ai.operation.name") && s.Has(k) {
			p.Lose(k, ReasonNoField, "gen_ai.operation.name already carries the call type")
			continue
		}
		if k != "llm.request.type" && s.Has(k) {
			p.Lose(k, ReasonNoField, "the conventions name no equivalent")
		}
	}

	for _, k := range s.Keys() {
		if strings.HasPrefix(k, "metadata.") {
			p.Lose(k, ReasonNoField, "proxy tenancy dimensions the conventions do not model")
		}
	}
}
