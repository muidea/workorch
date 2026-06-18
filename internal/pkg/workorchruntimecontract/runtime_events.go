package workorchruntimecontract

import "github.com/muidea/workorch/internal/pkg/common"

type RuntimeStepExecutionRequested struct {
	WorkspaceID                   common.WorkspaceID `json:"workspaceId"`
	RunID                         common.RunID       `json:"runId"`
	StepID                        common.StepID      `json:"stepId"`
	StepKind                      string             `json:"stepKind"`
	RequestID                     string             `json:"requestId"`
	Attempt                       int                `json:"attempt,omitempty"`
	WorkspaceDeliverableDirectory string             `json:"workspaceDeliverableDirectory,omitempty"`
	WorkspaceInternalDirectory    string             `json:"workspaceInternalDirectory,omitempty"`
	Input                         map[string]any     `json:"input,omitempty"`
	ProgressPolicy                map[string]any     `json:"progressPolicy,omitempty"`
}

type RuntimeStepExecutionAccepted struct {
	WorkspaceID common.WorkspaceID `json:"workspaceId"`
	RunID       common.RunID       `json:"runId"`
	StepID      common.StepID      `json:"stepId"`
	StepKind    string             `json:"stepKind"`
	RequestID   string             `json:"requestId"`
	QueueKey    string             `json:"queueKey"`
	AcceptedAt  string             `json:"acceptedAt"`
}

type RuntimeStepExecutionProgressed struct {
	WorkspaceID common.WorkspaceID `json:"workspaceId"`
	RunID       common.RunID       `json:"runId"`
	StepID      common.StepID      `json:"stepId"`
	StepKind    string             `json:"stepKind"`
	RequestID   string             `json:"requestId,omitempty"`
	Event       string             `json:"event,omitempty"`
	Stage       string             `json:"stage,omitempty"`
	Summary     string             `json:"summary,omitempty"`
	Data        map[string]any     `json:"data,omitempty"`
	OccurredAt  string             `json:"occurredAt,omitempty"`
}

type RuntimeStepExecutionCompleted struct {
	WorkspaceID  common.WorkspaceID `json:"workspaceId"`
	RunID        common.RunID       `json:"runId"`
	StepID       common.StepID      `json:"stepId"`
	StepKind     string             `json:"stepKind"`
	RequestID    string             `json:"requestId"`
	Output       map[string]any     `json:"output,omitempty"`
	LLMCallRefs  []string           `json:"llmCallRefs,omitempty"`
	Usage        map[string]any     `json:"usage,omitempty"`
	CompletedAt  string             `json:"completedAt"`
	EvidenceRefs []string           `json:"evidenceRefs,omitempty"`
	RawResultRef string             `json:"rawResultRef,omitempty"`
}

type RuntimeStepExecutionFailed struct {
	WorkspaceID      common.WorkspaceID `json:"workspaceId"`
	RunID            common.RunID       `json:"runId"`
	StepID           common.StepID      `json:"stepId"`
	StepKind         string             `json:"stepKind"`
	RequestID        string             `json:"requestId"`
	Error            string             `json:"error,omitempty"`
	FailureClass     string             `json:"failureClass,omitempty"`
	FailureSignature string             `json:"failureSignature,omitempty"`
	LLMCallRefs      []string           `json:"llmCallRefs,omitempty"`
	Usage            map[string]any     `json:"usage,omitempty"`
	CompletedAt      string             `json:"completedAt"`
	EvidenceRefs     []string           `json:"evidenceRefs,omitempty"`
	RawResultRef     string             `json:"rawResultRef,omitempty"`
}
