// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func braintrustSpan(extra map[string]string) Span {
	attrs := map[string]string{
		"braintrust.span_attributes": `{"name":"answer_question","type":"llm"}`,
		"gen_ai.provider.name":       "openai",
		"gen_ai.request.model":       "gpt-4o-mini",
	}
	for k, v := range extra {
		attrs[k] = v
	}
	return spanOf(attrs)
}

func TestBraintrustClaimsItsOwnNamespace(t *testing.T) {
	d, _, ok := Detect(braintrustSpan(nil))
	if !ok || d.Name() != Braintrust {
		t.Fatalf("a braintrust span was claimed by %v (ok=%v)", d, ok)
	}
}

func TestBraintrustCarriesASingleScoreAsAnEvaluation(t *testing.T) {
	p := mustParse(t, braintrustSpan(map[string]string{
		"braintrust.scores": `{"factuality":0.92}`,
	}))

	if got := mustField(t, p, semconv.EvaluationName).Str; got != "factuality" {
		t.Errorf("evaluation name is %q, want factuality", got)
	}
	got, ok := mustField(t, p, semconv.EvaluationScoreValue).Float64()
	if !ok || got != 0.92 {
		t.Errorf("evaluation score is %v, want 0.92", got)
	}
}

func TestBraintrustRefusesToPickOneOfSeveralScores(t *testing.T) {
	// The conventions carry one evaluation per span. Choosing among several
	// would be inventing a verdict the emitter never reached.
	p := mustParse(t, braintrustSpan(map[string]string{
		"braintrust.scores": `{"factuality":0.92,"tone":0.4,"safety":1.0}`,
	}))

	for _, f := range []semconv.Field{semconv.EvaluationName, semconv.EvaluationScoreValue} {
		if v, ok := p.Fields[f]; ok {
			t.Errorf("%s was set to %v from a span carrying three scores", f, v)
		}
	}
	l := lossFor(t, p, "braintrust.scores")
	if l.Reason != ReasonFlattened {
		t.Errorf("a multi-score span was recorded with reason %q, want %q", l.Reason, ReasonFlattened)
	}
	for _, name := range []string{"factuality", "tone", "safety"} {
		if !strings.Contains(l.Detail, name) {
			t.Errorf("the loss detail does not name the %s score: %q", name, l.Detail)
		}
	}
}

func TestBraintrustLiftsTokenCountsOutOfItsMetricsBlob(t *testing.T) {
	p := mustParse(t, braintrustSpan(map[string]string{
		"braintrust.metrics": `{"prompt_tokens":412,"completion_tokens":27,"prompt_cached_tokens":256,"start":1749834000.1,"end":1749834002.7}`,
	}))

	for f, want := range map[semconv.Field]int64{
		semconv.UsageInputTokens:          412,
		semconv.UsageOutputTokens:         27,
		semconv.UsageCacheReadInputTokens: 256,
	} {
		if got := mustField(t, p, f).Int; got != want {
			t.Errorf("%s is %d, want %d", f, got, want)
		}
	}
	// The timings have nowhere to go, and are named once rather than one loss
	// per key.
	if l := lossFor(t, p, "braintrust.metrics"); l.Reason != ReasonNoField {
		t.Errorf("leftover metrics recorded with reason %q, want %q", l.Reason, ReasonNoField)
	}
}

func TestBraintrustPrefersTheEmittersOwnGenAIAttributeOverTheBlob(t *testing.T) {
	// A count written to gen_ai.usage.input_tokens is the emitter speaking the
	// target's language, and outranks the same number inside a JSON blob.
	s := braintrustSpan(map[string]string{"braintrust.metrics": `{"prompt_tokens":999}`})
	// Built as a real int rather than through spanOf, which makes every
	// attribute a string: a token count arrives on the wire as an integer.
	s.Attributes["gen_ai.usage.input_tokens"] = Int(412)

	p := mustParse(t, s)
	if got := mustField(t, p, semconv.UsageInputTokens).Int; got != 412 {
		t.Errorf("input tokens is %d, want the gen_ai attribute's 412", got)
	}
}

func TestBraintrustMapsItsSpanTaxonomy(t *testing.T) {
	for typ, want := range map[string]string{
		"llm":  "chat",
		"tool": "execute_tool",
		"task": "invoke_workflow",
	} {
		s := braintrustSpan(map[string]string{
			"braintrust.span_attributes": `{"name":"step","type":"` + typ + `"}`,
		})
		if got := mustField(t, mustParse(t, s), semconv.OperationName).Str; got != want {
			t.Errorf("span type %q became operation %q, want %q", typ, got, want)
		}
	}
}

func TestBraintrustRecordsAScoringSpanAsHavingNoOperation(t *testing.T) {
	// A span whose whole job is scoring is not an operation the conventions
	// name at either version.
	s := braintrustSpan(map[string]string{
		"braintrust.span_attributes": `{"name":"factuality","type":"score"}`,
	})
	p := mustParse(t, s)

	if v, ok := p.Fields[semconv.OperationName]; ok {
		t.Errorf("a score span was given operation %v", v)
	}
	if l := lossFor(t, p, "braintrust.span_attributes"); l.Reason != ReasonNoField {
		t.Errorf("a score span was recorded with reason %q, want %q", l.Reason, ReasonNoField)
	}
}

// braintrust.context_json is the one attribute in this dialect that no
// application writes: a real BraintrustSpanProcessor injects it on the way out,
// which is how it turned up in testdata/braintrust when that fixture stopped
// being hand-built. It has no schema to map onto, so it is recorded rather than
// carried -- but it must be recorded, because an attribute the normalizer walks
// past silently is indistinguishable from one it does not know exists.
func TestBraintrustRecordsTheContextItsOwnProcessorAdds(t *testing.T) {
	p := (braintrust{}).Parse(Span{Attributes: map[string]Value{
		"braintrust.context_json": String(`{"caller":"capture"}`),
	}})

	var found bool
	for _, l := range p.Loss {
		if l.Key == "braintrust.context_json" && l.Reason == ReasonUnstructured {
			found = true
		}
	}
	if !found {
		t.Errorf("braintrust.context_json was not recorded as unstructured; losses = %+v", p.Loss)
	}
}

// decodeMessages reads a message field back into the package's own message
// type, so assertions are about structure rather than about byte layout.
func decodeMessages(t *testing.T, v Value) []message {
	t.Helper()
	var msgs []message
	if err := json.Unmarshal([]byte(v.Str), &msgs); err != nil {
		t.Fatalf("message field is not a JSON message list: %v\n%s", err, v.Str)
	}
	return msgs
}

func noLossFor(t *testing.T, p Parsed, key string) {
	t.Helper()
	for _, l := range p.Loss {
		if l.Key == key {
			t.Errorf("%s was carried but is also recorded as lost: %+v", key, l)
		}
	}
}

// On a span Braintrust typed llm, input and output are the request's messages
// and the response's message in OpenAI's chat shape, and the conventions have a
// home for both. Recording them as unstructured sent a model call's whole
// conversation to "no standard name" on the one span where what they are is
// known.
func TestBraintrustCarriesMessagesOnAModelCall(t *testing.T) {
	p := mustParse(t, braintrustSpan(map[string]string{
		"braintrust.input_json": `[{"role":"system","content":"Be brief."},` +
			`{"role":"user","content":[{"type":"text","text":"Where is order A-1187?"}]}]`,
		"braintrust.output_json": `[{"role":"assistant","content":null,"refusal":null,"tool_calls":` +
			`[{"id":"call_1","type":"function","function":{"name":"lookup_order","arguments":"{\"order_id\": \"A-1187\"}"}}]}]`,
	}))

	in := decodeMessages(t, mustField(t, p, semconv.InputMessages))
	if len(in) != 2 || in[0].Role != "system" || in[1].Role != "user" {
		t.Fatalf("input messages are %+v, want system then user", in)
	}
	if got := in[1].Parts; len(got) != 1 || got[0].Type != "text" || got[0].Content != "Where is order A-1187?" {
		t.Errorf("the user's text part list was not carried as a text part: %+v", got)
	}

	out := decodeMessages(t, mustField(t, p, semconv.OutputMessages))
	if len(out) != 1 || out[0].Role != "assistant" || len(out[0].Parts) != 1 {
		t.Fatalf("output messages are %+v, want one assistant message with one part", out)
	}
	call := out[0].Parts[0]
	if call.Type != "tool_call" || call.ID != "call_1" || call.Name != "lookup_order" {
		t.Errorf("the tool call came through as %+v", call)
	}
	// OpenAI writes arguments as a JSON-encoded string. Every other dialect here
	// carries them as the object, and a backend comparing tool calls across
	// emitters needs the same shape from all of them.
	if args, ok := call.Arguments.(map[string]any); !ok || args["order_id"] != "A-1187" {
		t.Errorf("tool call arguments are %#v, want the parsed object", call.Arguments)
	}

	for field, key := range map[semconv.Field]string{
		semconv.InputMessages:  "braintrust.input_json",
		semconv.OutputMessages: "braintrust.output_json",
	} {
		// Lifted, and lifted because of what the span type says: the same
		// attribute on a task span is not read as messages at all.
		if got, want := p.Source[field], (Origin{Key: key, Lifted: true, When: braintrustModelCall}); got != want {
			t.Errorf("%s is attributed to %+v, want %+v", field, got, want)
		}
		noLossFor(t, p, key)
	}
}

// A tool's result sent back to the model is part of the conversation, and both
// targets' message schemas carry it as a tool_call_response part under the id
// of the call it answers.
func TestBraintrustCarriesAToolResultAsAToolCallResponse(t *testing.T) {
	p := mustParse(t, braintrustSpan(map[string]string{
		"braintrust.input_json": `[{"role":"user","content":"Where is order A-1187?"},` +
			`{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function",` +
			`"function":{"name":"lookup_order","arguments":"{\"order_id\": \"A-1187\"}"}}]},` +
			`{"role":"tool","tool_call_id":"call_1","content":"in transit"}]`,
	}))
	in := decodeMessages(t, mustField(t, p, semconv.InputMessages))
	if len(in) != 3 {
		t.Fatalf("carried %d messages, want all 3 including the tool result: %+v", len(in), in)
	}
	got := in[2]
	if got.Role != "tool" || len(got.Parts) != 1 {
		t.Fatalf("the tool result came through as %+v", got)
	}
	if part := got.Parts[0]; part.Type != "tool_call_response" || part.ID != "call_1" || part.Response != "in transit" {
		t.Errorf("the tool result part is %+v, want tool_call_response for call_1", part)
	}
	noLossFor(t, p, "braintrust.input_json[2]")
}

func TestBraintrustReadsAFinishReasonOffAChoice(t *testing.T) {
	p := mustParse(t, braintrustSpan(map[string]string{
		"braintrust.output_json": `[{"index":0,"message":{"role":"assistant","content":"It shipped."},"finish_reason":"stop"}]`,
	}))
	out := decodeMessages(t, mustField(t, p, semconv.OutputMessages))
	if len(out) != 1 || out[0].FinishReason != "stop" || out[0].Parts[0].Content != "It shipped." {
		t.Errorf("a choice came through as %+v, want its message with finish_reason stop", out)
	}
}

// Anywhere but an llm span, input and output are whatever the traced function
// took and returned. A message list that happens to parse there is still a
// guess about what the function was.
func TestBraintrustLeavesInputAloneOffAModelCall(t *testing.T) {
	p := mustParse(t, braintrustSpan(map[string]string{
		"braintrust.span_attributes": `{"name":"triage","type":"task"}`,
		"braintrust.input_json":      `[{"role":"user","content":"Where is order A-1187?"}]`,
	}))
	if v, ok := p.Fields[semconv.InputMessages]; ok {
		t.Errorf("a task span's input was read as messages: %v", v)
	}
	if l := lossFor(t, p, "braintrust.input_json"); l.Reason != ReasonUnstructured {
		t.Errorf("a task span's input was recorded with reason %q, want %q", l.Reason, ReasonUnstructured)
	}
}

func TestBraintrustPrefersTheEmittersOwnMessagesOverTheBlob(t *testing.T) {
	own := `[{"role":"user","parts":[{"type":"text","content":"from gen_ai"}]}]`
	p := mustParse(t, braintrustSpan(map[string]string{
		"gen_ai.input.messages": own,
		"braintrust.input_json": `[{"role":"user","content":"from the blob"}]`,
	}))
	if got := mustField(t, p, semconv.InputMessages).Str; got != own {
		t.Errorf("input messages are %s, want the span's own gen_ai.input.messages", got)
	}
	if l := lossFor(t, p, "braintrust.input_json"); !strings.Contains(l.Detail, "outranks") {
		t.Errorf("the passed-over blob does not say why: %+v", l)
	}
}

// Half a conversation carried as though it were the conversation is worse than
// the honest gap, so one element that is not a chat message keeps the whole
// attribute out.
func TestBraintrustRefusesAMessageListItCannotReadWhole(t *testing.T) {
	for name, input := range map[string]string{
		"a stray element": `[{"role":"user","content":"hi"},"not a message"]`,
		"no role":         `[{"content":"hi"}]`,
		"not a list":      `{"question":"Where is order A-1187?"}`,
		"not JSON":        `Where is order A-1187?`,
	} {
		p := mustParse(t, braintrustSpan(map[string]string{"braintrust.input_json": input}))
		if v, ok := p.Fields[semconv.InputMessages]; ok {
			t.Errorf("%s: carried %v", name, v)
		}
		if l := lossFor(t, p, "braintrust.input_json"); l.Reason != ReasonUnstructured {
			t.Errorf("%s: recorded with reason %q, want %q", name, l.Reason, ReasonUnstructured)
		}
	}
}
