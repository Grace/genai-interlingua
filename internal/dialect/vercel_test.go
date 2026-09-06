package dialect

import (
	"encoding/json"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// outerSpan is what the AI SDK puts on the span a user's own call creates:
// ai.* only, and the token counts under their pre-rename names.
func outerSpan(extra map[string]string) Span {
	attrs := map[string]string{
		"ai.operationId":            "ai.generateText",
		"ai.model.provider":         "openai.chat",
		"ai.model.id":               "gpt-4o-mini",
		"ai.response.finishReason":  "stop",
		"ai.telemetry.functionId":   "summarize_ticket",
		"ai.usage.promptTokens":     "0",
		"ai.usage.completionTokens": "0",
	}
	for k, v := range extra {
		attrs[k] = v
	}
	return spanOf(attrs)
}

// innerSpan is what the provider adapter puts on the span beneath it: gen_ai.*
// already, but with the pre-split gen_ai.system spelling.
func innerSpan(extra map[string]string) Span {
	attrs := map[string]string{
		"ai.operationId":       "ai.generateText.doGenerate",
		"gen_ai.system":        "openai",
		"gen_ai.request.model": "gpt-4o-mini",
	}
	for k, v := range extra {
		attrs[k] = v
	}
	return spanOf(attrs)
}

func TestVercelClaimsBothOfItsSpanShapes(t *testing.T) {
	for name, s := range map[string]Span{
		"outer": outerSpan(nil),
		"inner": innerSpan(nil),
		"tool":  spanOf(map[string]string{"ai.operationId": "ai.toolCall", "ai.toolCall.name": "lookup"}),
	} {
		t.Run(name, func(t *testing.T) {
			d, _, ok := Detect(s)
			if !ok {
				t.Fatalf("no dialect claimed the %s span", name)
			}
			if d.Name() != Vercel {
				t.Errorf("the %s span was claimed by %s, want %s", name, d.Name(), Vercel)
			}
		})
	}
}

// The inner span is the one that could plausibly be mistaken for a conformant
// span from some other emitter, since most of what it carries is already
// gen_ai.*. Nothing else may claim it.
func TestVercelInnerSpanIsNotMistakenForOpenLLMetry(t *testing.T) {
	s := innerSpan(map[string]string{
		"gen_ai.usage.input_tokens":  "412",
		"gen_ai.usage.output_tokens": "27",
	})
	if n := (openLLMetry{}).Score(s); n != 0 {
		t.Errorf("openllmetry scored %d on a Vercel adapter span, want 0", n)
	}
}

func TestVercelRenamesGenAISystemToProviderName(t *testing.T) {
	// gen_ai.system is the attribute the conventions replaced with
	// gen_ai.provider.name. Carrying it through unrenamed would leave the span
	// looking conformant while naming a field that no longer exists.
	p := mustParse(t, innerSpan(nil))
	if got := mustField(t, p, semconv.ProviderName).Str; got != "openai" {
		t.Errorf("gen_ai.provider.name is %q, want openai", got)
	}
}

func TestVercelSplitsProviderIdFromModelType(t *testing.T) {
	// ai.model.provider is <provider id>.<model type>, so only the first
	// segment names the provider.
	for value, want := range map[string]string{
		"openai.chat":             "openai",
		"google.generative-ai":    "gcp.gemini",
		"amazon-bedrock.messages": "aws.bedrock",
		"azure.chat":              "azure.ai.openai",
		"xai.chat":                "x_ai",
		// An SDK provider the conventions do not name passes through as the SDK
		// spelled it, for the renderer to drop against the target's value set.
		"togetherai.chat": "togetherai",
	} {
		p := mustParse(t, outerSpan(map[string]string{"ai.model.provider": value}))
		if got := mustField(t, p, semconv.ProviderName).Str; got != want {
			t.Errorf("ai.model.provider %q became %q, want %q", value, got, want)
		}
	}
}

func TestVercelStripsTheAdapterSuffixFromTheOperation(t *testing.T) {
	// The inner span is the same operation as its parent seen one layer down.
	for id, want := range map[string]string{
		"ai.generateText":            "chat",
		"ai.generateText.doGenerate": "chat",
		"ai.streamText.doStream":     "chat",
		"ai.embedMany.doEmbed":       "embeddings",
		"ai.toolCall":                "execute_tool",
	} {
		p := mustParse(t, outerSpan(map[string]string{"ai.operationId": id}))
		if got := mustField(t, p, semconv.OperationName).Str; got != want {
			t.Errorf("ai.operationId %q became operation %q, want %q", id, got, want)
		}
	}
}

func TestVercelConvertsTimeToFirstChunkIntoSeconds(t *testing.T) {
	// The SDK reports milliseconds; gen_ai.response.time_to_first_chunk is a
	// double in seconds.
	s := innerSpan(nil)
	s.Attributes["ai.response.msToFirstChunk"] = Int(1234)

	p := mustParse(t, s)
	got, ok := mustField(t, p, semconv.ResponseTimeToFirstChunk).Float64()
	if !ok {
		t.Fatal("time to first chunk did not survive as a number")
	}
	if got != 1.234 {
		t.Errorf("1234 ms became %v s, want 1.234", got)
	}
	// A unit change that rounds nothing away is not a loss, and naming it in
	// interlingua.lossy would devalue the list.
	for _, l := range p.Loss {
		if l.Key == "ai.response.msToFirstChunk" {
			t.Errorf("an exact unit conversion was recorded as a loss: %v", l)
		}
	}
}

func TestVercelReadsBothMessageContentShapes(t *testing.T) {
	// The SDK writes content as a bare string or as a typed part list, and both
	// turn up in the same array depending on how the caller built the prompt.
	s := innerSpan(map[string]string{
		"ai.prompt.messages": `[
			{"role":"system","content":"Be brief."},
			{"role":"user","content":[{"type":"text","text":"Where is order A-1187?"}]}
		]`,
	})

	var msgs []message
	if err := json.Unmarshal([]byte(mustField(t, mustParse(t, s), semconv.InputMessages).Str), &msgs); err != nil {
		t.Fatalf("input messages are not valid JSON: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	for i, want := range []string{"Be brief.", "Where is order A-1187?"} {
		if len(msgs[i].Parts) != 1 || msgs[i].Parts[0].Content != want {
			t.Errorf("message %d is %+v, want one text part %q", i, msgs[i].Parts, want)
		}
	}
}

func TestVercelAssemblesOneAssistantMessageFromTwoAttributes(t *testing.T) {
	// The SDK reports the assistant's text and its tool calls separately.
	s := innerSpan(map[string]string{
		"ai.response.text":      "Checking that now.",
		"ai.response.toolCalls": `[{"toolCallId":"call_1","toolName":"lookup_order","args":{"order_id":"A-1187"}}]`,
	})

	var msgs []message
	if err := json.Unmarshal([]byte(mustField(t, mustParse(t, s), semconv.OutputMessages).Str), &msgs); err != nil {
		t.Fatalf("output messages are not valid JSON: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Role != "assistant" {
		t.Fatalf("got %+v, want one assistant message", msgs)
	}
	if len(msgs[0].Parts) != 2 {
		t.Fatalf("got %d parts, want a text part and a tool call", len(msgs[0].Parts))
	}
	if msgs[0].Parts[1].Type != "tool_call" || msgs[0].Parts[1].Name != "lookup_order" {
		t.Errorf("second part is %+v, want the lookup_order tool call", msgs[0].Parts[1])
	}
}

func TestVercelDoesNotCarryRequestHeaders(t *testing.T) {
	// Provider headers are where the API key lives.
	s := outerSpan(map[string]string{
		"ai.request.headers.authorization": "Bearer sk-not-a-real-key",
		"ai.request.headers.x-request-id":  "req-88",
	})
	p := mustParse(t, s)

	for _, f := range p.SortedFields() {
		if v := p.Fields[f]; v.Kind == KindStr && v.Str == "Bearer sk-not-a-real-key" {
			t.Fatalf("an authorization header was carried into field %s", f)
		}
	}
	if l := lossFor(t, p, "ai.request.headers.authorization"); l.Reason != ReasonNoField {
		t.Errorf("the authorization header was recorded with reason %q, want %q", l.Reason, ReasonNoField)
	}
}

func TestVercelNormalizesFinishReasonSpelling(t *testing.T) {
	// The SDK hyphenates what every other emitter here spells with an
	// underscore. gen_ai.response.finish_reasons has no closed value set, so
	// nothing downstream would catch this; it would just sit in a backend as a
	// second spelling of one concept.
	for value, want := range map[string]string{
		"tool-calls":     "tool_calls",
		"content-filter": "content_filter",
		"stop":           "stop",
		"length":         "length",
		// The SDK's own reasons have no counterpart to be renamed to.
		"unknown": "unknown",
	} {
		p := mustParse(t, outerSpan(map[string]string{"ai.response.finishReason": value}))
		got := mustField(t, p, semconv.ResponseFinishReasons).StrSeq
		if len(got) != 1 || got[0] != want {
			t.Errorf("finish reason %q became %v, want [%s]", value, got, want)
		}
	}
}

func TestVercelNormalizesFinishReasonsOnTheAdapterSpan(t *testing.T) {
	// The inner span already carries a list, and it needs the same treatment.
	s := innerSpan(nil)
	s.Attributes["gen_ai.response.finish_reasons"] = StrSeq([]string{"tool-calls"})

	got := mustField(t, mustParse(t, s), semconv.ResponseFinishReasons).StrSeq
	if len(got) != 1 || got[0] != "tool_calls" {
		t.Errorf("adapter finish reasons became %v, want [tool_calls]", got)
	}
}
