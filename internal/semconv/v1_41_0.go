package semconv

// keysV1_41_0 is model/gen-ai/registry.yaml at tag v1.41.0 of
// open-telemetry/semantic-conventions. The 22 fields missing from this map are
// the ones the new repository added after the split; normalizing to this target
// drops them, and the normalizer says so.
var keysV1_41_0 = map[Field]string{
	ProviderName: "gen_ai.provider.name",

	RequestModel:            "gen_ai.request.model",
	RequestMaxTokens:        "gen_ai.request.max_tokens",
	RequestChoiceCount:      "gen_ai.request.choice.count",
	RequestTemperature:      "gen_ai.request.temperature",
	RequestTopP:             "gen_ai.request.top_p",
	RequestTopK:             "gen_ai.request.top_k",
	RequestStopSequences:    "gen_ai.request.stop_sequences",
	RequestFrequencyPenalty: "gen_ai.request.frequency_penalty",
	RequestPresencePenalty:  "gen_ai.request.presence_penalty",
	RequestEncodingFormats:  "gen_ai.request.encoding_formats",
	RequestSeed:             "gen_ai.request.seed",
	RequestStream:           "gen_ai.request.stream",

	ResponseID:               "gen_ai.response.id",
	ResponseModel:            "gen_ai.response.model",
	ResponseFinishReasons:    "gen_ai.response.finish_reasons",
	ResponseTimeToFirstChunk: "gen_ai.response.time_to_first_chunk",

	UsageInputTokens:          "gen_ai.usage.input_tokens",
	UsageCacheReadInputTokens: "gen_ai.usage.cache_read.input_tokens",
	// Renamed to gen_ai.usage.cache_write.input_tokens in the new repository.
	UsageCacheWriteInputTokens: "gen_ai.usage.cache_creation.input_tokens",
	UsageOutputTokens:          "gen_ai.usage.output_tokens",
	UsageReasoningOutputTokens: "gen_ai.usage.reasoning.output_tokens",

	TokenType: "gen_ai.token.type",

	ConversationID: "gen_ai.conversation.id",

	AgentID:          "gen_ai.agent.id",
	AgentName:        "gen_ai.agent.name",
	AgentDescription: "gen_ai.agent.description",
	AgentVersion:     "gen_ai.agent.version",

	ToolName:          "gen_ai.tool.name",
	ToolCallID:        "gen_ai.tool.call.id",
	ToolDescription:   "gen_ai.tool.description",
	ToolType:          "gen_ai.tool.type",
	ToolCallArguments: "gen_ai.tool.call.arguments",
	ToolCallResult:    "gen_ai.tool.call.result",
	ToolDefinitions:   "gen_ai.tool.definitions",

	DataSourceID: "gen_ai.data_source.id",

	OperationName: "gen_ai.operation.name",

	OutputType: "gen_ai.output.type",

	EmbeddingsDimensionCount: "gen_ai.embeddings.dimension.count",

	RetrievalDocuments: "gen_ai.retrieval.documents",
	RetrievalQueryText: "gen_ai.retrieval.query.text",

	SystemInstructions: "gen_ai.system_instructions",

	InputMessages: "gen_ai.input.messages",

	OutputMessages: "gen_ai.output.messages",

	EvaluationName:        "gen_ai.evaluation.name",
	EvaluationScoreValue:  "gen_ai.evaluation.score.value",
	EvaluationScoreLabel:  "gen_ai.evaluation.score.label",
	EvaluationExplanation: "gen_ai.evaluation.explanation",

	PromptName: "gen_ai.prompt.name",

	WorkflowName: "gen_ai.workflow.name",
}

// enumsV1_41_0 is the closed value set for every gen_ai.* attribute at v1.41.0
// that has one. ResponseStatus is absent because the attribute itself is: the
// whole field arrived after the split.
//
// A span carrying a value that is legal in the new repository and absent here is
// a loss the normalizer records, the same as a field with no key. It is the
// commoner of the two: an agent framework emitting gen_ai.operation.name=plan is
// saying something v1.41.0 has no word for.
var enumsV1_41_0 = map[Field][]string{
	ProviderName: {
		"openai",
		"gcp.gen_ai",
		"gcp.vertex_ai",
		"gcp.gemini",
		"anthropic",
		"cohere",
		"azure.ai.inference",
		"azure.ai.openai",
		"ibm.watsonx.ai",
		"aws.bedrock",
		"perplexity",
		"x_ai",
		"deepseek",
		"groq",
		"mistral_ai",
	},

	OperationName: {
		"chat",
		"generate_content",
		"text_completion",
		"embeddings",
		"retrieval",
		"create_agent",
		"invoke_agent",
		"execute_tool",
		"invoke_workflow",
	},

	OutputType: {"text", "json", "image", "speech"},

	// The registry also lists a deprecated member spelled completion whose value
	// is likewise output, so the value set is two members, not three.
	TokenType: {"input", "output"},
}
