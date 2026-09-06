package dialect

import (
	"encoding/json"
	"testing"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

func TestOpenLLMetryScoresOnlyItsOwnAttributes(t *testing.T) {
	scores := map[string]int{
		"traceloop.span.kind":            2,
		"traceloop.workflow.name":        2,
		"gen_ai.usage.prompt_tokens":     1,
		"gen_ai.usage.completion_tokens": 1,
		"gen_ai.usage.total_tokens":      1,
		"llm.request.type":               1,
		"gen_ai.prompt.0.role":           1,
		"gen_ai.completion.0.role":       1,
		"gen_ai.request.model":           0,
		"gen_ai.usage.input_tokens":      0,
		"llm.model_name":                 0,
	}
	for key, want := range scores {
		if got := (openLLMetry{}).Score(spanOf(map[string]string{key: "x"})); got != want {
			t.Errorf("Score of a span carrying only %s = %d, want %d", key, got, want)
		}
	}
}

func TestOpenLLMetryMapsTheLegacyTokenKeys(t *testing.T) {
	p := (openLLMetry{}).Parse(Span{Attributes: map[string]Value{
		"gen_ai.system":                            String("Anthropic"),
		"gen_ai.usage.prompt_tokens":               Int(41),
		"gen_ai.usage.completion_tokens":           Int(7),
		"gen_ai.usage.cache_creation_input_tokens": Int(1024),
		"gen_ai.usage.cache_read_input_tokens":     Int(512),
	}})
	if got, want := mustField(t, p, semconv.UsageInputTokens).Int, int64(41); got != want {
		t.Errorf("input tokens = %d, want %d", got, want)
	}
	if got, want := mustField(t, p, semconv.UsageOutputTokens).Int, int64(7); got != want {
		t.Errorf("output tokens = %d, want %d", got, want)
	}
	if got, want := mustField(t, p, semconv.UsageCacheWriteInputTokens).Int, int64(1024); got != want {
		t.Errorf("cache write tokens = %d, want %d", got, want)
	}
	if got, want := mustField(t, p, semconv.UsageCacheReadInputTokens).Int, int64(512); got != want {
		t.Errorf("cache read tokens = %d, want %d", got, want)
	}
	// gen_ai.system is a legacy spelling and its values were never lowercased
	// by the emitter, which the conventions require.
	if got, want := mustField(t, p, semconv.ProviderName).Str, "anthropic"; got != want {
		t.Errorf("provider = %q, want %q", got, want)
	}
}

func TestTotalTokensHasNowhereToGo(t *testing.T) {
	p := (openLLMetry{}).Parse(Span{Attributes: map[string]Value{
		"gen_ai.usage.total_tokens": Int(48),
	}})
	if got, want := lossFor(t, p, "gen_ai.usage.total_tokens").Reason, ReasonNoField; got != want {
		t.Errorf("loss reason = %s, want %s", got, want)
	}
}

// traceloop.span.kind decides what entity.name means. Without it the name could
// be an agent, a tool or an anonymous decorated function, and guessing would
// invent an agent out of a task.
func TestEntityNameNeedsSpanKindToMeanAnything(t *testing.T) {
	agent := (openLLMetry{}).Parse(spanOf(map[string]string{
		"traceloop.span.kind":   "agent",
		"traceloop.entity.name": "researcher",
	}))
	if got, want := mustField(t, agent, semconv.AgentName).Str, "researcher"; got != want {
		t.Errorf("agent name = %q, want %q", got, want)
	}

	tool := (openLLMetry{}).Parse(spanOf(map[string]string{
		"traceloop.span.kind":   "tool",
		"traceloop.entity.name": "get_weather",
	}))
	if got, want := mustField(t, tool, semconv.ToolName).Str, "get_weather"; got != want {
		t.Errorf("tool name = %q, want %q", got, want)
	}

	task := (openLLMetry{}).Parse(spanOf(map[string]string{
		"traceloop.span.kind":   "task",
		"traceloop.entity.name": "summarize",
	}))
	if _, ok := task.Fields[semconv.AgentName]; ok {
		t.Error("a task entity became an agent")
	}
	if got, want := lossFor(t, task, "traceloop.entity.name").Reason, ReasonAmbiguous; got != want {
		t.Errorf("loss reason = %s, want %s", got, want)
	}
}

// llm.request.type is written only on spans that really did call a model, so it
// is the more specific of the two operation signals and wins where both exist.
func TestRequestTypeOverridesSpanKind(t *testing.T) {
	p := (openLLMetry{}).Parse(spanOf(map[string]string{
		"traceloop.span.kind": "workflow",
		"llm.request.type":    "embedding",
	}))
	if got, want := mustField(t, p, semconv.OperationName).Str, "embeddings"; got != want {
		t.Errorf("operation = %q, want %q", got, want)
	}
}

func TestUnmappedRequestTypeIsRecordedRatherThanGuessed(t *testing.T) {
	p := (openLLMetry{}).Parse(spanOf(map[string]string{"llm.request.type": "rerank"}))
	if _, ok := p.Fields[semconv.OperationName]; ok {
		t.Error("rerank produced an operation name; the conventions name none")
	}
	l := lossFor(t, p, "llm.request.type")
	if got, want := l.Reason, ReasonNoField; got != want {
		t.Errorf("loss reason = %s, want %s", got, want)
	}
	if got, want := l.Detail, "no gen_ai.operation.name value for rerank"; got != want {
		t.Errorf("loss detail = %q, want %q", got, want)
	}
}

func TestPromptMessagesAreReassembledIntoOneList(t *testing.T) {
	p := (openLLMetry{}).Parse(spanOf(map[string]string{
		"gen_ai.prompt.0.role":    "system",
		"gen_ai.prompt.0.content": "be brief",
		"gen_ai.prompt.1.role":    "user",
		"gen_ai.prompt.1.content": "how is the weather",
	}))
	want := `[{"role":"system","parts":[{"type":"text","content":"be brief"}]},` +
		`{"role":"user","parts":[{"type":"text","content":"how is the weather"}]}]`
	if got := mustField(t, p, semconv.InputMessages).Str; got != want {
		t.Errorf("input messages =\n%s\nwant\n%s", got, want)
	}
	l := lossFor(t, p, "gen_ai.prompt.*")
	if got, want := l.Reason, ReasonFlattened; got != want {
		t.Errorf("loss reason = %s, want %s", got, want)
	}
	// The glob key is a description of the reassembly, not a source attribute,
	// so it must not appear in the consumed list alongside the real keys.
	for _, k := range p.Consumed {
		if k == "gen_ai.prompt.*" {
			t.Error("the glob key leaked into Consumed")
		}
	}
	if got, want := len(p.Consumed), 4; got != want {
		t.Errorf("consumed %d keys, want %d: %v", got, want, p.Consumed)
	}
}

// The emitter stringified the arguments object. Re-parsing is what makes two
// emitters' tool calls comparable once they land in the same backend.
func TestToolCallArgumentsAreReparsedAsJSON(t *testing.T) {
	p := (openLLMetry{}).Parse(spanOf(map[string]string{
		"gen_ai.completion.0.role":                   "assistant",
		"gen_ai.completion.0.tool_calls.0.id":        "call_1",
		"gen_ai.completion.0.tool_calls.0.name":      "get_weather",
		"gen_ai.completion.0.tool_calls.0.arguments": `{"city":"Durham"}`,
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
	call := msgs[0].Parts[0]
	if got, want := call.Type, "tool_call"; got != want {
		t.Errorf("part type = %q, want %q", got, want)
	}
	args, ok := call.Arguments.(map[string]any)
	if !ok {
		t.Fatalf("arguments are %T, want an object", call.Arguments)
	}
	if got, want := args["city"], "Durham"; got != want {
		t.Errorf("city = %v, want %v", got, want)
	}
}

func TestArgumentsThatAreNotJSONSurviveAsAString(t *testing.T) {
	got := toolCallPart("call_1", "get_weather", "Durham, NC")
	if want := "Durham, NC"; got.Arguments != want {
		t.Errorf("arguments = %v, want %q", got.Arguments, want)
	}
}

func TestFinishReasonsAreCollectedFromTheOutputMessages(t *testing.T) {
	p := (openLLMetry{}).Parse(spanOf(map[string]string{
		"gen_ai.completion.0.role":          "assistant",
		"gen_ai.completion.0.content":       "sunny",
		"gen_ai.completion.0.finish_reason": "stop",
		"gen_ai.completion.1.role":          "assistant",
		"gen_ai.completion.1.content":       "it is",
		"gen_ai.completion.1.finish_reason": "length",
	}))
	got := mustField(t, p, semconv.ResponseFinishReasons).StrSeq
	if len(got) != 2 || got[0] != "stop" || got[1] != "length" {
		t.Errorf("finish reasons = %v, want [stop length]", got)
	}
}

func TestFunctionDefinitionsBecomeOneToolDefinitionsDocument(t *testing.T) {
	p := (openLLMetry{}).Parse(spanOf(map[string]string{
		"llm.request.functions.0.name":        "get_weather",
		"llm.request.functions.0.description": "look up the weather",
		"llm.request.functions.0.parameters":  `{"type":"object"}`,
	}))
	want := `[{"description":"look up the weather","name":"get_weather",` +
		`"parameters":{"type":"object"},"type":"function"}]`
	if got := mustField(t, p, semconv.ToolDefinitions).Str; got != want {
		t.Errorf("tool definitions =\n%s\nwant\n%s", got, want)
	}
}

func TestAssociationPropertiesAreAllRecordedAsLost(t *testing.T) {
	p := (openLLMetry{}).Parse(spanOf(map[string]string{
		"traceloop.association.properties.user_id": "u1",
		"traceloop.association.properties.tenant":  "acme",
		"traceloop.workflow.name":                  "research",
	}))
	if got, want := len(LossKeys(p.Loss)), 2; got != want {
		t.Errorf("%d losses, want %d: %v", got, want, LossKeys(p.Loss))
	}
	if got, want := mustField(t, p, semconv.WorkflowName).Str, "research"; got != want {
		t.Errorf("workflow name = %q, want %q", got, want)
	}
}
