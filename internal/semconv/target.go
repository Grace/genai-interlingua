// Package semconv renders normalized GenAI fields into a chosen target schema
// version.
//
// The gen_ai.* attributes were deprecated out of open-telemetry/semantic-conventions
// at v1.42.0 and moved to open-telemetry/semantic-conventions-genai, which as of
// this writing has no tags and no releases. There is no schema URL to pin
// against, so which schema you normalize to is a decision this package forces
// you to make rather than a constant it hides. See docs/moving-target.md.
package semconv

import "fmt"

// Target names a concrete GenAI attribute schema to render into.
type Target string

const (
	// TargetV1_41_0 is semantic-conventions v1.41.0, the last tagged release
	// whose model/gen-ai directory contained live attribute definitions rather
	// than deprecation shims. 50 attributes.
	TargetV1_41_0 Target = "v1.41.0"

	// TargetGenAIMain tracks the main branch of semantic-conventions-genai. It
	// is a moving target with no version to record on the span. 72 attributes.
	TargetGenAIMain Target = "genai-main"
)

// DefaultTarget is the frozen cut. It is deliberately not the newest schema:
// the newest schema cannot be pinned.
const DefaultTarget = TargetV1_41_0

// Targets lists every target in the order they should appear in generated docs.
var Targets = []Target{TargetV1_41_0, TargetGenAIMain}

func (t Target) String() string { return string(t) }

// ParseTarget resolves a configured target name.
func ParseTarget(s string) (Target, error) {
	for _, t := range Targets {
		if string(t) == s {
			return t, nil
		}
	}
	return "", fmt.Errorf("unknown target %q, want one of %v", s, Targets)
}

// Field is a logical GenAI concept, independent of any schema version. Field
// names follow semantic-conventions-genai main with the gen_ai. prefix removed,
// because main is a strict superset of v1.41.0 apart from a single rename.
type Field string

const (
	ProviderName Field = "provider.name"

	RequestModel              Field = "request.model"
	RequestMaxTokens          Field = "request.max_tokens"
	RequestChoiceCount        Field = "request.choice.count"
	RequestTemperature        Field = "request.temperature"
	RequestTopP               Field = "request.top_p"
	RequestTopK               Field = "request.top_k"
	RequestStopSequences      Field = "request.stop_sequences"
	RequestFrequencyPenalty   Field = "request.frequency_penalty"
	RequestPresencePenalty    Field = "request.presence_penalty"
	RequestEncodingFormats    Field = "request.encoding_formats"
	RequestSeed               Field = "request.seed"
	RequestStream             Field = "request.stream"
	RequestReasoningLevel     Field = "request.reasoning.level"
	RequestPreviousResponseID Field = "request.previous_response.id"
	RequestStreamCursor       Field = "request.stream_cursor"

	ResponseID               Field = "response.id"
	ResponseModel            Field = "response.model"
	ResponseFinishReasons    Field = "response.finish_reasons"
	ResponseStatus           Field = "response.status"
	ResponseTimeToFirstChunk Field = "response.time_to_first_chunk"

	UsageInputTokens          Field = "usage.input_tokens"
	UsageCacheReadInputTokens Field = "usage.cache_read.input_tokens"
	// UsageCacheWriteInputTokens is the one field whose key differs
	// between targets: v1.41.0 spells it gen_ai.usage.cache_creation.input_tokens.
	// The definitions are otherwise byte-identical, so it is one field, not two.
	UsageCacheWriteInputTokens     Field = "usage.cache_write.input_tokens"
	UsageTextInputTokens           Field = "usage.text.input_tokens"
	UsageImageInputTokens          Field = "usage.image.input_tokens"
	UsageAudioInputTokens          Field = "usage.audio.input_tokens"
	UsageOutputTokens              Field = "usage.output_tokens"
	UsageReasoningOutputTokens     Field = "usage.reasoning.output_tokens"
	UsageTextOutputTokens          Field = "usage.text.output_tokens"
	UsageImageOutputTokens         Field = "usage.image.output_tokens"
	UsageAudioOutputTokens         Field = "usage.audio.output_tokens"
	UsageTextCacheReadInputTokens  Field = "usage.text.cache_read.input_tokens"
	UsageImageCacheReadInputTokens Field = "usage.image.cache_read.input_tokens"
	UsageAudioCacheReadInputTokens Field = "usage.audio.cache_read.input_tokens"

	TokenType Field = "token.type"

	ConversationID        Field = "conversation.id"
	ConversationCompacted Field = "conversation.compacted"

	AgentID          Field = "agent.id"
	AgentName        Field = "agent.name"
	AgentDescription Field = "agent.description"
	AgentVersion     Field = "agent.version"

	ToolName          Field = "tool.name"
	ToolCallID        Field = "tool.call.id"
	ToolDescription   Field = "tool.description"
	ToolType          Field = "tool.type"
	ToolCallArguments Field = "tool.call.arguments"
	ToolCallResult    Field = "tool.call.result"
	ToolDefinitions   Field = "tool.definitions"

	DataSourceID Field = "data_source.id"

	OperationName Field = "operation.name"

	OutputType Field = "output.type"

	EmbeddingsDimensionCount Field = "embeddings.dimension.count"

	RetrievalDocuments Field = "retrieval.documents"
	RetrievalQueryText Field = "retrieval.query.text"
	RetrievalTopK      Field = "retrieval.top_k"

	MemoryStoreID     Field = "memory.store.id"
	MemoryRecordID    Field = "memory.record.id"
	MemoryRecordCount Field = "memory.record.count"
	MemoryQueryText   Field = "memory.query.text"
	MemoryRecords     Field = "memory.records"

	SystemInstructions Field = "system_instructions"

	InputMessages Field = "input.messages"

	OutputMessages Field = "output.messages"

	EvaluationName        Field = "evaluation.name"
	EvaluationScoreValue  Field = "evaluation.score.value"
	EvaluationScoreLabel  Field = "evaluation.score.label"
	EvaluationExplanation Field = "evaluation.explanation"

	PromptName     Field = "prompt.name"
	PromptVersion  Field = "prompt.version"
	PromptVariable Field = "prompt.variable"

	WorkflowName Field = "workflow.name"
)

// AllFields is every field this package knows about, in registry order. A field
// present here but absent from a target's key map is unrepresentable in that
// target, which is a loss the normalizer records rather than a panic.
var AllFields = []Field{
	ProviderName,
	RequestModel,
	RequestMaxTokens,
	RequestChoiceCount,
	RequestTemperature,
	RequestTopP,
	RequestTopK,
	RequestStopSequences,
	RequestFrequencyPenalty,
	RequestPresencePenalty,
	RequestEncodingFormats,
	RequestSeed,
	RequestStream,
	RequestReasoningLevel,
	RequestPreviousResponseID,
	RequestStreamCursor,
	ResponseID,
	ResponseModel,
	ResponseFinishReasons,
	ResponseStatus,
	ResponseTimeToFirstChunk,
	UsageInputTokens,
	UsageCacheReadInputTokens,
	UsageCacheWriteInputTokens,
	UsageTextInputTokens,
	UsageImageInputTokens,
	UsageAudioInputTokens,
	UsageOutputTokens,
	UsageReasoningOutputTokens,
	UsageTextOutputTokens,
	UsageImageOutputTokens,
	UsageAudioOutputTokens,
	UsageTextCacheReadInputTokens,
	UsageImageCacheReadInputTokens,
	UsageAudioCacheReadInputTokens,
	TokenType,
	ConversationID,
	ConversationCompacted,
	AgentID,
	AgentName,
	AgentDescription,
	AgentVersion,
	ToolName,
	ToolCallID,
	ToolDescription,
	ToolType,
	ToolCallArguments,
	ToolCallResult,
	ToolDefinitions,
	DataSourceID,
	OperationName,
	OutputType,
	EmbeddingsDimensionCount,
	RetrievalDocuments,
	RetrievalQueryText,
	RetrievalTopK,
	MemoryStoreID,
	MemoryRecordID,
	MemoryRecordCount,
	MemoryQueryText,
	MemoryRecords,
	SystemInstructions,
	InputMessages,
	OutputMessages,
	EvaluationName,
	EvaluationScoreValue,
	EvaluationScoreLabel,
	EvaluationExplanation,
	PromptName,
	PromptVersion,
	PromptVariable,
	WorkflowName,
}

var keys = map[Target]map[Field]string{
	TargetV1_41_0:   keysV1_41_0,
	TargetGenAIMain: keysGenAIMain,
}

// Key returns the attribute key that field takes under this target, and whether
// the target can represent it at all.
func (t Target) Key(f Field) (string, bool) {
	k, ok := keys[t][f]
	return k, ok
}

// Represents reports whether the target can carry field without loss of identity.
func (t Target) Represents(f Field) bool {
	_, ok := keys[t][f]
	return ok
}

var enums = map[Target]map[Field][]string{
	TargetV1_41_0:   enumsV1_41_0,
	TargetGenAIMain: enumsGenAIMain,
}

// EnumValues returns the closed value set field takes under this target, and
// whether field is an enum there at all. A field that is open-ended, or that the
// target cannot represent, returns false.
func (t Target) EnumValues(f Field) ([]string, bool) {
	v, ok := enums[t][f]
	return v, ok
}

// Accepts reports whether value is legal for field under this target. Fields with
// no closed value set accept anything, so Accepts is true for them; ask
// Represents first if you care whether the field exists at all.
func (t Target) Accepts(f Field, value string) bool {
	v, ok := enums[t][f]
	if !ok {
		return true
	}
	for _, allowed := range v {
		if allowed == value {
			return true
		}
	}
	return false
}
