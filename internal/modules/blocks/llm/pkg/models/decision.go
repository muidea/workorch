package models

type ProviderRequest struct {
	WorkspaceID         string                 `json:"workspaceId"`
	RunID               string                 `json:"runId"`
	StepID              string                 `json:"stepId"`
	Kind                string                 `json:"kind"`
	Model               string                 `json:"model"`
	SystemLocale        string                 `json:"systemLocale,omitempty"`
	DialogLanguage      string                 `json:"dialogLanguage,omitempty"`
	Input               map[string]any         `json:"input,omitempty"`
	Schema              string                 `json:"schema"`
	RuntimeCapabilities RuntimeCapabilities    `json:"runtimeCapabilities,omitempty"`
	TraceRefs           map[string]string      `json:"traceRefs,omitempty"`
	Config              RedactedProviderConfig `json:"config"`
	ContextBudget       *ProviderContextBudget `json:"contextBudget,omitempty"`
	Repair              *ProviderRepairRequest `json:"repair,omitempty"`
}

type ProviderContextBudget struct {
	MaxBytes              int                    `json:"maxBytes"`
	ActualBytes           int                    `json:"actualBytes"`
	MaxTokens             int                    `json:"maxTokens,omitempty"`
	EstimatedInputTokens  int                    `json:"estimatedInputTokens,omitempty"`
	EstimatedOutputTokens int                    `json:"estimatedOutputTokens,omitempty"`
	ActualTokens          int                    `json:"actualTokens,omitempty"`
	ShrinkLevel           int                    `json:"shrinkLevel,omitempty"`
	Truncated             bool                   `json:"truncated"`
	Rules                 []string               `json:"rules,omitempty"`
	SourceRefs            []string               `json:"sourceRefs,omitempty"`
	Estimator             string                 `json:"estimator,omitempty"`
	Profile               ProviderContextProfile `json:"profile,omitempty"`
}

type ProviderContextProfile struct {
	ContextWindowTokens    int     `json:"contextWindowTokens,omitempty"`
	MaxInputTokens         int     `json:"maxInputTokens,omitempty"`
	ReservedOutputTokens   int     `json:"reservedOutputTokens,omitempty"`
	SafetyRatio            float64 `json:"safetyRatio,omitempty"`
	BytesHardLimit         int     `json:"bytesHardLimit,omitempty"`
	EstimatedTokensPerByte float64 `json:"estimatedTokensPerByte,omitempty"`
	InputTokenPrice        float64 `json:"inputTokenPrice,omitempty"`
	OutputTokenPrice       float64 `json:"outputTokenPrice,omitempty"`
	Currency               string  `json:"currency,omitempty"`
}

type ProviderUsage struct {
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	TotalTokens  int `json:"totalTokens,omitempty"`
}

type ProviderCompletionResult struct {
	Payload []byte         `json:"payload,omitempty"`
	Usage   *ProviderUsage `json:"usage,omitempty"`
}

type ProviderRepairRequest struct {
	Attempt        int    `json:"attempt"`
	OriginalOutput string `json:"originalOutput"`
	ParseError     string `json:"parseError"`
}

type RuntimeCapabilities struct {
	BlockKinds  []RuntimeBlockCapability `json:"blockKinds,omitempty"`
	ServiceAPIs []ServiceAPICapability   `json:"serviceAPIs,omitempty"`
	ShellPolicy ShellRuntimePolicy       `json:"shellPolicy,omitempty"`
}

type RuntimeBlockCapability struct {
	Kind        string         `json:"kind"`
	Description string         `json:"description,omitempty"`
	Input       map[string]any `json:"input,omitempty"`
}

type ServiceAPICapability struct {
	Method       string `json:"method"`
	PathTemplate string `json:"pathTemplate"`
	Description  string `json:"description,omitempty"`
	ReadOnly     bool   `json:"readOnly,omitempty"`
	AllowQuery   bool   `json:"allowQuery,omitempty"`
}

type ShellRuntimePolicy struct {
	Enabled                 bool     `json:"enabled"`
	LocalServiceHTTP        string   `json:"localServiceHttp,omitempty"`
	UnavailableCommands     []string `json:"unavailableCommands,omitempty"`
	AllowedLocalServiceHost []string `json:"allowedLocalServiceHost,omitempty"`
}

type LLMExecutionDecision struct {
	ThoughtSummary   string               `json:"thoughtSummary,omitempty"`
	Plan             []string             `json:"plan,omitempty"`
	ExecutionGraph   *BlockExecutionGraph `json:"executionGraph,omitempty"`
	NextAction       *LLMAction           `json:"nextAction,omitempty"`
	RequiredDecision *RequiredDecision    `json:"requiredDecision,omitempty"`
	RequiredApproval *RequiredApproval    `json:"requiredApproval,omitempty"`
	PatchRequests    []PatchRequest       `json:"patchRequests,omitempty"`
	MCPToolCalls     []MCPToolCallRequest `json:"mcpToolCalls,omitempty"`
	ShellCommands    []ShellCommand       `json:"shellCommands,omitempty"`
	MemoryWrites     []MemoryWriteRequest `json:"memoryWrites,omitempty"`
	DoDAssessment    *DoDAssessment       `json:"dodAssessment,omitempty"`
	CompletionState  string               `json:"completionState"`
}

type BlockExecutionGraph struct {
	Nodes []BlockExecutionNode `json:"nodes"`
}

type BlockExecutionNode struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	DependsOn []string       `json:"dependsOn,omitempty"`
	Branch    string         `json:"branch,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
}

type LLMAction struct {
	Kind    string         `json:"kind"`
	Summary string         `json:"summary,omitempty"`
	Input   map[string]any `json:"input,omitempty"`
}

type RequiredDecision struct {
	Reason       string         `json:"reason"`
	DecisionKind string         `json:"decisionKind,omitempty"`
	Question     string         `json:"question,omitempty"`
	Context      map[string]any `json:"context,omitempty"`
}

type RequiredApproval struct {
	Reason  string         `json:"reason"`
	Action  string         `json:"action,omitempty"`
	Context map[string]any `json:"context,omitempty"`
}

type PatchRequest struct {
	PatchType  string `json:"patchType,omitempty"`
	Operation  string `json:"operation,omitempty"`
	TargetRef  string `json:"targetRef"`
	Content    string `json:"content,omitempty"`
	Diff       string `json:"diff,omitempty"`
	Summary    string `json:"summary,omitempty"`
	BaseHash   string `json:"baseHash,omitempty"`
	ApprovalID string `json:"approvalId,omitempty"`
}

type MCPToolCallRequest struct {
	ServerID string         `json:"serverId"`
	Tool     string         `json:"tool"`
	Input    map[string]any `json:"input,omitempty"`
	Summary  string         `json:"summary,omitempty"`
}

type ShellCommand struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Summary string   `json:"summary,omitempty"`
}

type MemoryWriteRequest struct {
	MemoryType string   `json:"memoryType"`
	Summary    string   `json:"summary"`
	Content    string   `json:"content,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

type DoDAssessment struct {
	Satisfied             bool                `json:"satisfied"`
	Items                 []DoDAssessmentItem `json:"items,omitempty"`
	Summary               string              `json:"summary,omitempty"`
	GoalOutcome           string              `json:"goalOutcome,omitempty"`
	ImplementationSummary string              `json:"implementationSummary,omitempty"`
	VerificationSummary   string              `json:"verificationSummary,omitempty"`
	KnownLimitations      []string            `json:"knownLimitations,omitempty"`
	ResidualRisks         []string            `json:"residualRisks,omitempty"`
	NextActions           []string            `json:"nextActions,omitempty"`
}

type DoDAssessmentItem struct {
	Item         string   `json:"item"`
	State        string   `json:"state"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
}
