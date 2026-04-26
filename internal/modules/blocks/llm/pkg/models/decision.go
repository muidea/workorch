package models

type ProviderRequest struct {
	WorkspaceID string                 `json:"workspaceId"`
	RunID       string                 `json:"runId"`
	StepID      string                 `json:"stepId"`
	Kind        string                 `json:"kind"`
	Model       string                 `json:"model"`
	Input       map[string]any         `json:"input,omitempty"`
	Schema      string                 `json:"schema"`
	TraceRefs   map[string]string      `json:"traceRefs,omitempty"`
	Config      RedactedProviderConfig `json:"config"`
}

type LLMExecutionDecision struct {
	ThoughtSummary   string               `json:"thoughtSummary,omitempty"`
	Plan             []string             `json:"plan,omitempty"`
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
	PatchType string `json:"patchType,omitempty"`
	TargetRef string `json:"targetRef"`
	Content   string `json:"content,omitempty"`
	Diff      string `json:"diff,omitempty"`
	Summary   string `json:"summary,omitempty"`
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
	Satisfied bool                `json:"satisfied"`
	Items     []DoDAssessmentItem `json:"items,omitempty"`
	Summary   string              `json:"summary,omitempty"`
}

type DoDAssessmentItem struct {
	Item         string   `json:"item"`
	State        string   `json:"state"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
}
