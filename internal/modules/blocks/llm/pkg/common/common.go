package common

const ModuleID = "blocks/llm"

const StepKindWorkspaceExecution = "llm/workspace_execution"
const ExecutionContextLLMProviderID = "llmProviderId"
const DefaultProviderID = "default"

const (
	RuntimeBlockKindPatchApply      = "patch/apply"
	RuntimeBlockKindDecisionRequest = "decision/request"
	RuntimeBlockKindApprovalRequest = "approval/request"
	RuntimeBlockKindMemoryWrite     = "memory/write"
	RuntimeBlockKindDoDAssessment   = "dod/assessment"
)

const (
	ProviderOpenAI     = "openai"
	ProviderCompatible = "compatible"
)

const (
	EnvProvider          = "provider"
	EnvBaseURL           = "baseURL"
	EnvAPIKey            = "apiKey"
	EnvModel             = "model"
	EnvTimeout           = "timeout"
	EnvDisabled          = "disabled"
	EnvChatCompletions   = "chatCompletions"
	EnvStreaming         = "streaming"
	EnvNonChatOperations = "nonChatOperations"
)

const (
	ProviderOperationChatCompletions = "chat_completions"
	ProviderOperationEmbeddings      = "embeddings"
)
