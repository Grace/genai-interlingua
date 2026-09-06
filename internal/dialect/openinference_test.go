package dialect

import (
	"encoding/json"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func TestOpenInferenceScoresOnlyItsOwnAttributes(t *testing.T) {
	scores := map[string]int{
		"openinference.span.kind":            2,
		"llm.model_name":                     1,
		"llm.provider":                       1,
		"llm.token_count.prompt":             1,
		"llm.token_count.completion":         1,
		"llm.invocation_parameters":          1,
		"llm.input_messages.0.message.role":  1,
		"llm.output_messages.0.message.role": 1,
		"llm.token_count.total":              0,
		"gen_ai.usage.prompt_tokens":         0,
		"traceloop.span.kind":                0,
	}
	for key, want := range scores {
		if got := (openInference{}).Score(spanOf(map[string]string{key: "x"})); got != want {
			t.Errorf("Score of a span carrying only %s = %d, want %d", key, got, want)
		}
	}
}

func TestSpanKindBecomesOperationName(t *testing.T) {
	want := map[string]string{
		"LLM":       "chat",
		"CHAIN":     "invoke_workflow",
		"AGENT":     "invoke_agent",
		"TOOL":      "execute_tool",
		"EMBEDDING": "embeddings",
		"RETRIEVER": "retrieval",
	}
	for kind, op := range want {
		p := (openInference{}).Parse(spanOf(map[string]string{"openinference.span.kind": kind}))
		if got := p.Fields[semconv.OperationName].Str; got != op {
			t.Errorf("span kind %s became operation %q, want %q", kind, got, op)
		}
	}
}

// RERANKER, GUARDRAIL and EVALUATOR have no counterpart at any schema version.
// An evaluator span is an event in the new repository, not an operation.
func TestUnmappedSpanKindIsRecordedRatherThanGuessed(t *testing.T) {
	p := (openInference{}).Parse(spanOf(map[string]string{"openinference.span.kind": "RERANKER"}))
	if _, ok := p.Fields[semconv.OperationName]; ok {
		t.Error("RERANKER produced an operation name")
	}
	if got, want := lossFor(t, p, "openinference.span.kind").Reason, ReasonNoField; got != want {
		t.Errorf("loss reason = %s, want %s", got, want)
	}
}

// OpenInference names providers after the company; the conventions name them
// after the API surface, which is why one company can appear twice there.
func TestProviderNamesBecomeAPISurfaces(t *testing.T) {
	want := map[string]string{
		"aws":       "aws.bedrock",
		"azure":     "azure.ai.openai",
		"google":    "gcp.gemini",
		"vertexai":  "gcp.vertex_ai",
		"mistralai": "mistral_ai",
		"xai":       "x_ai",
		"openai":    "openai",
		"Anthropic": "anthropic",
	}
	for in, out := range want {
		p := (openInference{}).Parse(spanOf(map[string]string{"llm.provider": in}))
		if got := p.Fields[semconv.ProviderName].Str; got != out {
			t.Errorf("llm.provider %q became %q, want %q", in, got, out)
		}
	}
}

func TestTokenDetailsMapOntoTheModalityFields(t *testing.T) {
	p := (openInference{}).Parse(Span{Attributes: map[string]Value{
		"llm.token_count.prompt":                       Int(41),
		"llm.token_count.completion":                   Int(7),
		"llm.token_count.total":                        Int(48),
		"llm.token_count.prompt_details.cache_read":    Int(512),
		"llm.token_count.prompt_details.cache_write":   Int(1024),
		"llm.token_count.prompt_details.audio":         Int(3),
		"llm.token_count.completion_details.reasoning": Int(19),
		"llm.token_count.completion_details.audio":     Int(2),
	}})
	counts := map[semconv.Field]int64{
		semconv.UsageInputTokens:           41,
		semconv.UsageOutputTokens:          7,
		semconv.UsageCacheReadInputTokens:  512,
		semconv.UsageCacheWriteInputTokens: 1024,
		semconv.UsageAudioInputTokens:      3,
		semconv.UsageReasoningOutputTokens: 19,
		semconv.UsageAudioOutputTokens:     2,
	}
	for f, want := range counts {
		if got := mustField(t, p, f).Int; got != want {
			t.Errorf("%s = %d, want %d", f, got, want)
		}
	}
	if got, want := lossFor(t, p, "llm.token_count.total").Reason, ReasonNoField; got != want {
		t.Errorf("total tokens loss reason = %s, want %s", got, want)
	}
}

// The conventions name each of these individually, so leaving them inside the
// blob would be a loss the dialect could have avoided.
func TestInvocationParametersAreLiftedOutOfTheBlob(t *testing.T) {
	p := (openInference{}).Parse(spanOf(map[string]string{
		"llm.invocation_parameters": `{"temperature":0.2,"max_tokens":512,"n":1,"logit_bias":{"50256":-100}}`,
	}))
	if got, want := mustField(t, p, semconv.RequestTemperature).Float, 0.2; got != want {
		t.Errorf("temperature = %v, want %v", got, want)
	}
	// JSON has one number type; an integral value belongs on the span as an int.
	max := mustField(t, p, semconv.RequestMaxTokens)
	if got, want := max.Kind, KindInt; got != want {
		t.Errorf("max tokens kind = %v, want %v", got, want)
	}
	if got, want := max.Int, int64(512); got != want {
		t.Errorf("max tokens = %d, want %d", got, want)
	}
	if got, want := mustField(t, p, semconv.RequestChoiceCount).Int, int64(1); got != want {
		t.Errorf("choice count = %d, want %d", got, want)
	}
	l := lossFor(t, p, "llm.invocation_parameters")
	if got, want := l.Detail, "no gen_ai field for logit_bias"; got != want {
		t.Errorf("loss detail = %q, want %q", got, want)
	}
}

func TestUnparseableInvocationParametersAreOneLoss(t *testing.T) {
	p := (openInference{}).Parse(spanOf(map[string]string{
		"llm.invocation_parameters": "temperature=0.2",
	}))
	l := lossFor(t, p, "llm.invocation_parameters")
	if got, want := l.Reason, ReasonUnstructured; got != want {
		t.Errorf("loss reason = %s, want %s", got, want)
	}
	if got, want := l.Detail, "value is not a JSON object"; got != want {
		t.Errorf("loss detail = %q, want %q", got, want)
	}
}

// Phoenix has no dedicated attribute for the arguments a tool was invoked with:
// on a TOOL span the generic payload attributes are the tool call.
func TestToolSpanPayloadBecomesTheCallArgumentsAndResult(t *testing.T) {
	p := (openInference{}).Parse(spanOf(map[string]string{
		"openinference.span.kind": "TOOL",
		"input.value":             `{"city":"Durham"}`,
		"output.value":            "72F and clear",
	}))
	if got, want := mustField(t, p, semconv.ToolCallArguments).Str, `{"city":"Durham"}`; got != want {
		t.Errorf("tool call arguments = %q, want %q", got, want)
	}
	if got, want := mustField(t, p, semconv.ToolCallResult).Str, "72F and clear"; got != want {
		t.Errorf("tool call result = %q, want %q", got, want)
	}
}

func TestNonToolSpanPayloadIsAnUnstructuredLoss(t *testing.T) {
	p := (openInference{}).Parse(spanOf(map[string]string{
		"openinference.span.kind": "CHAIN",
		"input.value":             `{"question":"how is the weather"}`,
		"input.mime_type":         "application/json",
	}))
	if _, ok := p.Fields[semconv.ToolCallArguments]; ok {
		t.Error("a CHAIN span payload became tool call arguments")
	}
	if got, want := lossFor(t, p, "input.value").Reason, ReasonUnstructured; got != want {
		t.Errorf("input.value loss reason = %s, want %s", got, want)
	}
	if got, want := lossFor(t, p, "input.mime_type").Reason, ReasonNoField; got != want {
		t.Errorf("input.mime_type loss reason = %s, want %s", got, want)
	}
}

// Phoenix nests everything one level deeper than OpenLLMetry does, and splits
// content into an optional typed parts list alongside the flat content string.
func TestOutputMessageToolCallsAreReassembled(t *testing.T) {
	p := (openInference{}).Parse(spanOf(map[string]string{
		"llm.output_messages.0.message.role":                                      "assistant",
		"llm.output_messages.0.message.tool_calls.0.tool_call.id":                 "call_1",
		"llm.output_messages.0.message.tool_calls.0.tool_call.function.name":      "get_weather",
		"llm.output_messages.0.message.tool_calls.0.tool_call.function.arguments": `{"city":"Durham"}`,
	}))
	var msgs []message
	if err := json.Unmarshal([]byte(mustField(t, p, semconv.OutputMessages).Str), &msgs); err != nil {
		t.Fatalf("output messages are not JSON: %v", err)
	}
	if got, want := len(msgs), 1; got != want {
		t.Fatalf("%d messages, want %d", got, want)
	}
	if got, want := len(msgs[0].Parts), 1; got != want {
		t.Fatalf("%d parts, want %d", got, want)
	}
	if got, want := msgs[0].Parts[0].Name, "get_weather"; got != want {
		t.Errorf("tool call name = %q, want %q", got, want)
	}
	if got, want := lossFor(t, p, "llm.output_messages.*").Reason, ReasonFlattened; got != want {
		t.Errorf("loss reason = %s, want %s", got, want)
	}
}

func TestTypedMessageContentsBecomeTextParts(t *testing.T) {
	p := (openInference{}).Parse(spanOf(map[string]string{
		"llm.input_messages.0.message.role":                                 "user",
		"llm.input_messages.0.message.contents.0.message_content.type":      "text",
		"llm.input_messages.0.message.contents.0.message_content.text":      "describe this",
		"llm.input_messages.0.message.contents.1.message_content.type":      "image",
		"llm.input_messages.0.message.contents.1.message_content.image.url": "https://example.invalid/a.png",
	}))
	var msgs []message
	if err := json.Unmarshal([]byte(mustField(t, p, semconv.InputMessages).Str), &msgs); err != nil {
		t.Fatalf("input messages are not JSON: %v", err)
	}
	if got, want := len(msgs[0].Parts), 1; got != want {
		t.Fatalf("%d parts, want %d", got, want)
	}
	if got, want := msgs[0].Parts[0].Content, "describe this"; got != want {
		t.Errorf("text part content = %q, want %q", got, want)
	}
	l := lossFor(t, p, "llm.input_messages.0.message.contents.1")
	if got, want := l.Detail, "no message part type for image"; got != want {
		t.Errorf("loss detail = %q, want %q", got, want)
	}
}

func TestRetrievalDocumentsBecomeOneList(t *testing.T) {
	p := (openInference{}).Parse(Span{Attributes: map[string]Value{
		"openinference.span.kind":                 String("RETRIEVER"),
		"retrieval.documents.0.document.id":       String("d1"),
		"retrieval.documents.0.document.content":  String("durham is in north carolina"),
		"retrieval.documents.0.document.score":    Float(0.91),
		"retrieval.documents.0.document.metadata": String(`{"source":"wiki"}`),
	}})
	want := `[{"content":"durham is in north carolina","id":"d1","score":0.91}]`
	if got := mustField(t, p, semconv.RetrievalDocuments).Str; got != want {
		t.Errorf("retrieval documents =\n%s\nwant\n%s", got, want)
	}
	if got, want := lossFor(t, p, "retrieval.documents.*.document.metadata").Reason, ReasonNoField; got != want {
		t.Errorf("metadata loss reason = %s, want %s", got, want)
	}
}

// The IR has no float-sequence kind, and the conventions name no attribute for
// an embedding vector at any version, so the whole payload is unrepresentable.
func TestEmbeddingPayloadsAreUnrepresentable(t *testing.T) {
	p := (openInference{}).Parse(spanOf(map[string]string{
		"openinference.span.kind":                 "EMBEDDING",
		"embedding.model_name":                    "text-embedding-3-small",
		"embedding.embeddings.0.embedding.text":   "durham",
		"embedding.embeddings.0.embedding.vector": "[0.1,0.2]",
	}))
	if got, want := mustField(t, p, semconv.RequestModel).Str, "text-embedding-3-small"; got != want {
		t.Errorf("model = %q, want %q", got, want)
	}
	for _, k := range []string{
		"embedding.embeddings.0.embedding.text",
		"embedding.embeddings.0.embedding.vector",
	} {
		if got, want := lossFor(t, p, k).Reason, ReasonUnstructured; got != want {
			t.Errorf("%s loss reason = %s, want %s", k, got, want)
		}
	}
}

func TestToolExecutionSchemaIsNotToolDefinitions(t *testing.T) {
	p := (openInference{}).Parse(spanOf(map[string]string{
		"openinference.span.kind": "TOOL",
		"tool.name":               "get_weather",
		"tool.parameters":         `{"type":"object"}`,
	}))
	if _, ok := p.Fields[semconv.ToolDefinitions]; ok {
		t.Error("the executing tool's own schema became gen_ai.tool.definitions")
	}
	if got, want := lossFor(t, p, "tool.parameters").Reason, ReasonNoField; got != want {
		t.Errorf("loss reason = %s, want %s", got, want)
	}
}

func TestOfferedToolsBecomeToolDefinitions(t *testing.T) {
	p := (openInference{}).Parse(spanOf(map[string]string{
		"openinference.span.kind":      "LLM",
		"llm.tools.0.tool.json_schema": `{"name":"get_weather"}`,
		"llm.tools.1.tool.json_schema": `{"name":"get_time"}`,
	}))
	want := `[{"name":"get_weather"},{"name":"get_time"}]`
	if got := mustField(t, p, semconv.ToolDefinitions).Str; got != want {
		t.Errorf("tool definitions = %s, want %s", got, want)
	}
}
