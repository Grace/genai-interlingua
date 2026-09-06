package dialect

import (
	"encoding/json"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func liteLLMSpan(extra map[string]string) Span {
	attrs := map[string]string{
		"gen_ai.framework":          "litellm",
		"gen_ai.system":             "openai",
		"gen_ai.request.model":      "gpt-4o-mini",
		"gen_ai.usage.input_tokens": "412",
		"litellm.model_group":       "support-pool",
	}
	for k, v := range extra {
		attrs[k] = v
	}
	return spanOf(attrs)
}

func TestLiteLLMBeatsOpenLLMetryOnTheVocabularyTheyShare(t *testing.T) {
	// Both emitters descend from the same instrumentation, so a LiteLLM span
	// can carry attributes OpenLLMetry also writes. What separates them has to
	// outweigh the shared half rather than merely tie with it.
	s := liteLLMSpan(map[string]string{
		"llm.request.type":             "acompletion",
		"llm.request.functions.0.name": "lookup_order",
		"gen_ai.completion.0.role":     "assistant",
	})

	d, margin, ok := Detect(s)
	if !ok {
		t.Fatal("no dialect claimed a LiteLLM span")
	}
	if d.Name() != LiteLLM {
		t.Fatalf("a LiteLLM span was claimed by %s", d.Name())
	}
	if margin <= 0 {
		t.Errorf("LiteLLM won by a margin of %d, which records the choice as a coin toss", margin)
	}
}

// A regression test for a scoring bug the golden files caught. Counting the
// conventions' own token attributes as LiteLLM evidence had this dialect
// claiming a share of the evidence on every conformant span in the trace,
// which shrank the winner's margin without ever changing the winner.
func TestLiteLLMScoresNothingOnASpanThatMerelyConforms(t *testing.T) {
	conformant := spanOf(map[string]string{
		"gen_ai.provider.name":       "openai",
		"gen_ai.request.model":       "gpt-4o-mini",
		"gen_ai.usage.input_tokens":  "412",
		"gen_ai.usage.output_tokens": "27",
		"gen_ai.usage.total_tokens":  "439",
		"gen_ai.operation.name":      "chat",
	})
	if n := (liteLLM{}).Score(conformant); n != 0 {
		t.Errorf("litellm scored %d on a span carrying only the conventions' own attributes, want 0", n)
	}
}

func TestLiteLLMTranslatesItsRoutingPrefixIntoAProviderName(t *testing.T) {
	// LiteLLM names a provider after its own routing prefix, which is shorter
	// than the conventions' name wherever one company fronts several surfaces.
	for value, want := range map[string]string{
		"bedrock":   "aws.bedrock",
		"vertex_ai": "gcp.vertex_ai",
		"gemini":    "gcp.gemini",
		"azure":     "azure.ai.openai",
		"watsonx":   "ibm.watsonx.ai",
		"anthropic": "anthropic",
	} {
		p := mustParse(t, liteLLMSpan(map[string]string{"gen_ai.system": value}))
		if got := mustField(t, p, semconv.ProviderName).Str; got != want {
			t.Errorf("provider %q became %q, want %q", value, got, want)
		}
	}
}

func TestLiteLLMPrefersTheNewerProviderSpelling(t *testing.T) {
	// The integration writes gen_ai.provider.name or gen_ai.system depending on
	// how it is configured. They are the same fact, and the newer one wins.
	s := liteLLMSpan(map[string]string{
		"gen_ai.system":        "openai",
		"gen_ai.provider.name": "azure",
	})
	if got := mustField(t, mustParse(t, s), semconv.ProviderName).Str; got != "azure.ai.openai" {
		t.Errorf("provider is %q, want the newer spelling translated to azure.ai.openai", got)
	}
}

func TestLiteLLMPassesTheConventionsOwnMessagesStraightThrough(t *testing.T) {
	// gen_ai.input.messages already holds the shape the target wants, so
	// re-deriving it from anything else would only be a chance to get it wrong.
	want := `[{"role":"user","parts":[{"type":"text","content":"Where is order A-1187?"}]}]`
	s := liteLLMSpan(map[string]string{"gen_ai.input.messages": want})

	if got := mustField(t, mustParse(t, s), semconv.InputMessages).Str; got != want {
		t.Errorf("input messages were rewritten to %s, want them carried through as %s", got, want)
	}
}

func TestLiteLLMFallsBackToTheIndexedMessagesOfAnOlderBuild(t *testing.T) {
	s := liteLLMSpan(map[string]string{
		"gen_ai.prompt.0.role":              "user",
		"gen_ai.prompt.0.content":           "Where is order A-1187?",
		"gen_ai.completion.0.role":          "assistant",
		"gen_ai.completion.0.content":       "Checking now.",
		"gen_ai.completion.0.finish_reason": "stop",
	})
	p := mustParse(t, s)

	var msgs []message
	if err := json.Unmarshal([]byte(mustField(t, p, semconv.InputMessages).Str), &msgs); err != nil {
		t.Fatalf("input messages are not valid JSON: %v", err)
	}
	if len(msgs) != 1 || len(msgs[0].Parts) != 1 || msgs[0].Parts[0].Content != "Where is order A-1187?" {
		t.Errorf("indexed prompt attributes became %+v", msgs)
	}
	if got := mustField(t, p, semconv.ResponseFinishReasons).StrSeq; len(got) != 1 || got[0] != "stop" {
		t.Errorf("finish reasons are %v, want [stop]", got)
	}
}

func TestLiteLLMMapsItsAsyncCallTypesOntoOneOperation(t *testing.T) {
	for value, want := range map[string]string{
		"acompletion":      "chat",
		"completion":       "chat",
		"aembedding":       "embeddings",
		"atext_completion": "text_completion",
		// A call type the conventions have no operation for is carried rather
		// than dropped here; the renderer is where a value the target does not
		// define gets removed, and it records that it did.
		"image_generation": "image_generation",
	} {
		p := mustParse(t, liteLLMSpan(map[string]string{"gen_ai.operation.name": value}))
		if got := mustField(t, p, semconv.OperationName).Str; got != want {
			t.Errorf("call type %q became operation %q, want %q", value, got, want)
		}
	}
}

func TestLiteLLMRecordsProxyTenancyAsLoss(t *testing.T) {
	// The metadata namespace is what an operator actually wants and the
	// conventions have no slot for.
	s := liteLLMSpan(map[string]string{
		"metadata.user_api_key_team_id": "team-42",
		"metadata.user_api_key_alias":   "support-bot",
	})
	p := mustParse(t, s)

	for _, key := range []string{"metadata.user_api_key_team_id", "metadata.user_api_key_alias", "litellm.model_group"} {
		if l := lossFor(t, p, key); l.Reason != ReasonNoField {
			t.Errorf("%s was recorded with reason %q, want %q", key, l.Reason, ReasonNoField)
		}
	}
}
