package common

const ModuleID = "blocks/llm"

const StepKindWorkspaceExecution = "llm/workspace_execution"
const ExecutionContextLLMProviderID = "llmProviderId"
const DefaultProviderID = "default"

const (
	ProviderOpenAI     = "openai"
	ProviderCompatible = "compatible"
)

const (
	EnvProvider = "provider"
	EnvBaseURL  = "baseURL"
	EnvAPIKey   = "apiKey"
	EnvModel    = "model"
	EnvTimeout  = "timeout"
	EnvDisabled = "disabled"
)
