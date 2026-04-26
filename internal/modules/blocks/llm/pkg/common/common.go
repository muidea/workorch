package common

const ModuleID = "blocks/llm"

const StepKindWorkspaceExecution = "llm/workspace_execution"

const (
	ProviderOpenAI     = "openai"
	ProviderCompatible = "compatible"
)

const (
	EnvProvider = "WORKORCH_LLM_PROVIDER"
	EnvBaseURL  = "WORKORCH_LLM_BASE_URL"
	EnvAPIKey   = "WORKORCH_LLM_API_KEY"
	EnvModel    = "WORKORCH_LLM_MODEL"
	EnvTimeout  = "WORKORCH_LLM_TIMEOUT"
	EnvDisabled = "WORKORCH_LLM_DISABLED"
)
