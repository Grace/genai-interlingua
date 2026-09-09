// SPDX-License-Identifier: Apache-2.0

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
	// gen_ai.cost.* is LiteLLM's own invention -- the conventions price nothing
	// at any version -- so it is as distinctive as the litellm. namespace.
	//
	// It is here because a capture of a real LiteLLM span scored 3, exactly
	// tying OpenLLMetry, which recognizes the same span by its llm.request.type,
	// its gen_ai.completion.N prefix and its total token count. LiteLLM won that
	// tie only by being registered first, and a detector whose answer depends on
	// registry order is a detector that will change its mind for no reason.
	if s.HasPrefix("gen_ai.cost.") {
		n += 2
	}
	return n
}

// Rules are the LiteLLM mappings that can be stated rather than performed.
// Almost all of this dialect fits, because LiteLLM writes the conventions'
// own attribute names for everything except the provider, the operation and
// the traceloop-shaped halves it inherited.
//
// The two rules with more than one key are the interesting ones, and they are
// the same fact twice: LiteLLM implemented a rename without dropping the old
// spelling, so a span can carry either. The newer key wins, and a build that
// writes both is not ambiguous, it is just current.
func (liteLLM) Rules() []Rule {
	return []Rule{
		// gen_ai.provider.name when configured for the newer conventions,
		// gen_ai.system otherwise. LiteLLM names a provider after its own
		// routing prefix, which is shorter than the conventions' name wherever
		// one company fronts several API surfaces.
		{
			Field:     semconv.ProviderName,
			Keys:      []string{"gen_ai.provider.name", "gen_ai.system"},
			Transform: Transform{Lower: true, Map: liteLLMProviders},
		},
		// llm.request.type is the traceloop attribute LiteLLM inherited and
		// still writes. A call type the table does not mention passes through
		// for the renderer to check against the target's value set: LiteLLM
		// routes calls the conventions have no operation for, and that is a
		// loss to report at render time rather than a value to drop here.
		{
			Field:     semconv.OperationName,
			Keys:      []string{"gen_ai.operation.name", "llm.request.type"},
			Transform: Transform{Map: liteLLMOperations},
		},

		{Field: semconv.RequestModel, Keys: []string{"gen_ai.request.model"}},
		{Field: semconv.ResponseModel, Keys: []string{"gen_ai.response.model"}},
		{Field: semconv.RequestMaxTokens, Keys: []string{"gen_ai.request.max_tokens"}},
		{Field: semconv.RequestTemperature, Keys: []string{"gen_ai.request.temperature"}},
		{Field: semconv.RequestTopP, Keys: []string{"gen_ai.request.top_p"}},
		{Field: semconv.RequestTopK, Keys: []string{"gen_ai.request.top_k"}},
		{Field: semconv.RequestSeed, Keys: []string{"gen_ai.request.seed"}},
		{Field: semconv.RequestFrequencyPenalty, Keys: []string{"gen_ai.request.frequency_penalty"}},
		{Field: semconv.RequestPresencePenalty, Keys: []string{"gen_ai.request.presence_penalty"}},
		{Field: semconv.RequestStopSequences, Keys: []string{"gen_ai.request.stop_sequences"}},
		{Field: semconv.RequestStream, Keys: []string{"gen_ai.request.stream"}},
		{Field: semconv.RequestChoiceCount, Keys: []string{"gen_ai.request.choice.count"}},

		{Field: semconv.UsageInputTokens, Keys: []string{"gen_ai.usage.input_tokens"}},
		{Field: semconv.UsageOutputTokens, Keys: []string{"gen_ai.usage.output_tokens"}},
		// LiteLLM already writes these two under the keys the conventions use.
		{Field: semconv.UsageCacheWriteInputTokens, Keys: []string{"gen_ai.usage.cache_creation.input_tokens"}},
		{Field: semconv.UsageCacheReadInputTokens, Keys: []string{"gen_ai.usage.cache_read.input_tokens"}},

		{Field: semconv.SystemInstructions, Keys: []string{"gen_ai.system_instructions"}},
		{Field: semconv.ConversationID, Keys: []string{"gen_ai.conversation.id"}},
	}
}

// Signature is the two namespaces Score weighs and the rule table cannot spell:
// litellm.* is the emitter's own, and gen_ai.cost.* is its invention -- the
// conventions price nothing at any version.
//
// gen_ai.framework is deliberately absent even though Score weighs it highest.
// Score tests its value, and a gate tests presence: any emitter that names the
// framework it is running under writes that key, so its presence identifies
// nothing. A signature entry has to be distinctive under the weaker test the
// gate can actually perform.
func (liteLLM) Signature() []string {
	return []string{
		"litellm.call_id",
		"litellm.provider.model",
		"gen_ai.cost.total_cost",
	}
}

// Unstated is what is left after the rules: the messages, and the finish
// reasons derived from them.
//
// Both come from reassembly rather than translation. The messages are either
// the conventions' own JSON attribute or an indexed traceloop pile --
// gen_ai.prompt.{i}.role and its neighbours -- and which one a span carries is
// not knowable until the span is in hand. The finish reasons are then read back
// out of the reassembled completions, so they cannot be stated either: their
// source is not an attribute, it is the result of the previous step.
func (liteLLM) Unstated() []semconv.Field {
	return []semconv.Field{
		semconv.InputMessages,
		semconv.OutputMessages,
		semconv.ResponseFinishReasons,
		semconv.ToolDefinitions,
	}
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

	p.applyRules(s, d.Rules())

	if s.Has("gen_ai.usage.total_tokens") {
		p.Lose("gen_ai.usage.total_tokens", ReasonNoField,
			"the conventions carry input and output counts only")
	}

	// What is left is the part no rule table can state: the messages are
	// either the conventions' own JSON or an indexed traceloop reassembly, and
	// which one it is depends on what the span turns out to carry.
	d.messages(s, &p)

	// The tool definitions are traceloop-shaped, so they are read by the
	// OpenLLMetry dialect's own reassembly rather than by a second copy of it.
	// The two emitters genuinely share this vocabulary; duplicating the parser
	// would let the two copies drift apart over exactly the attributes that
	// make detection hard in the first place.
	p.Set(semconv.ToolDefinitions, openLLMetry{}.toolDefinitions(s, &p))

	d.losses(s, &p)

	return p
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
func (d liteLLM) losses(s Span, p *Parsed) {
	defer p.sweepResidue(s, d, "litellm.", "gen_ai.", "llm.", "metadata.")

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
		// LiteLLM prices every call and puts the result on the span. The
		// conventions model no cost at any version, and this is exactly the
		// kind of attribute worth naming rather than dropping quietly: it is
		// the reason a lot of people run a proxy in the first place.
		if strings.HasPrefix(k, "gen_ai.cost.") {
			p.Lose(k, ReasonNoField, "the conventions price nothing at any version")
			continue
		}
		if strings.HasPrefix(k, "metadata.") {
			p.Lose(k, ReasonNoField, "proxy tenancy dimensions the conventions do not model")
		}
	}
}
