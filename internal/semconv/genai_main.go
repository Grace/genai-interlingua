package semconv

// keysGenAIMain is model/gen-ai/registry.yaml on the main branch of
// open-telemetry/semantic-conventions-genai. Every attribute there is
// stability: development; nothing in this schema is stable, including the
// question of where the schema lives.
var keysGenAIMain = map[Field]string{
	ProviderName: "gen_ai.provider.name",

	RequestModel:              "gen_ai.request.model",
	RequestMaxTokens:          "gen_ai.request.max_tokens",
	RequestChoiceCount:        "gen_ai.request.choice.count",
	RequestTemperature:        "gen_ai.request.temperature",
	RequestTopP:               "gen_ai.request.top_p",
	RequestTopK:               "gen_ai.request.top_k",
	RequestStopSequences:      "gen_ai.request.stop_sequences",
	RequestFrequencyPenalty:   "gen_ai.request.frequency_penalty",
	RequestPresencePenalty:    "gen_ai.request.presence_penalty",
	RequestEncodingFormats:    "gen_ai.request.encoding_formats",
	RequestSeed:               "gen_ai.request.seed",
	RequestStream:             "gen_ai.request.stream",
	RequestReasoningLevel:     "gen_ai.request.reasoning.level",
	RequestPreviousResponseID: "gen_ai.request.previous_response.id",
	RequestStreamCursor:       "gen_ai.request.stream_cursor",

	ResponseID:               "gen_ai.response.id",
	ResponseModel:            "gen_ai.response.model",
	ResponseFinishReasons:    "gen_ai.response.finish_reasons",
	ResponseStatus:           "gen_ai.response.status",
	ResponseTimeToFirstChunk: "gen_ai.response.time_to_first_chunk",

	UsageInputTokens:               "gen_ai.usage.input_tokens",
	UsageCacheReadInputTokens:      "gen_ai.usage.cache_read.input_tokens",
	UsageCacheWriteInputTokens:     "gen_ai.usage.cache_write.input_tokens",
	UsageTextInputTokens:           "gen_ai.usage.text.input_tokens",
	UsageImageInputTokens:          "gen_ai.usage.image.input_tokens",
	UsageAudioInputTokens:          "gen_ai.usage.audio.input_tokens",
	UsageOutputTokens:              "gen_ai.usage.output_tokens",
	UsageReasoningOutputTokens:     "gen_ai.usage.reasoning.output_tokens",
	UsageTextOutputTokens:          "gen_ai.usage.text.output_tokens",
	UsageImageOutputTokens:         "gen_ai.usage.image.output_tokens",
	UsageAudioOutputTokens:         "gen_ai.usage.audio.output_tokens",
	UsageTextCacheReadInputTokens:  "gen_ai.usage.text.cache_read.input_tokens",
	UsageImageCacheReadInputTokens: "gen_ai.usage.image.cache_read.input_tokens",
	UsageAudioCacheReadInputTokens: "gen_ai.usage.audio.cache_read.input_tokens",

	TokenType: "gen_ai.token.type",

	ConversationID:        "gen_ai.conversation.id",
	ConversationCompacted: "gen_ai.conversation.compacted",

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
	RetrievalTopK:      "gen_ai.retrieval.top_k",

	MemoryStoreID:     "gen_ai.memory.store.id",
	MemoryRecordID:    "gen_ai.memory.record.id",
	MemoryRecordCount: "gen_ai.memory.record.count",
	MemoryQueryText:   "gen_ai.memory.query.text",
	MemoryRecords:     "gen_ai.memory.records",

	SystemInstructions: "gen_ai.system_instructions",

	InputMessages: "gen_ai.input.messages",

	OutputMessages: "gen_ai.output.messages",

	EvaluationName:        "gen_ai.evaluation.name",
	EvaluationScoreValue:  "gen_ai.evaluation.score.value",
	EvaluationScoreLabel:  "gen_ai.evaluation.score.label",
	EvaluationExplanation: "gen_ai.evaluation.explanation",

	PromptName:     "gen_ai.prompt.name",
	PromptVersion:  "gen_ai.prompt.version",
	PromptVariable: "gen_ai.prompt.variable",

	WorkflowName: "gen_ai.workflow.name",
}

// enumsGenAIMain is the closed value set for every gen_ai.* attribute in the new
// repository that has one. Values drift independently of keys: main added nine
// operations that v1.41.0 never named.
var enumsGenAIMain = map[Field][]string{
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
		"moonshot_ai",
	},

	OperationName: {
		"chat",
		"generate_content",
		"text_completion",
		"embeddings",
		"retrieval",
		"fetch_response",
		"create_agent",
		"invoke_agent",
		"execute_tool",
		"invoke_workflow",
		"plan",
		"search_memory",
		"create_memory",
		"update_memory",
		"upsert_memory",
		"delete_memory",
		"create_memory_store",
		"delete_memory_store",
	},

	OutputType: {"text", "json", "image", "speech"},

	TokenType: {"input", "output"},

	ResponseStatus: {
		"queued",
		"in_progress",
		"completed",
		"incomplete",
		"failed",
		"cancelled",
	},
}
