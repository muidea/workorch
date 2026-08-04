package workorchprojectioncontract

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

type TaskDecomposition struct {
	ID            string                  `json:"id"`
	WorkspaceID   string                  `json:"workspaceId"`
	RunID         string                  `json:"runId"`
	RootStepID    string                  `json:"rootStepId"`
	State         string                  `json:"state"`
	Summary       string                  `json:"summary,omitempty"`
	GraphBranch   string                  `json:"graphBranch,omitempty"`
	NodeTotal     int                     `json:"nodeTotal"`
	NodeCompleted int                     `json:"nodeCompleted"`
	NodeFailed    int                     `json:"nodeFailed"`
	NodeRunning   int                     `json:"nodeRunning"`
	Nodes         []TaskDecompositionNode `json:"nodes,omitempty"`
	EvidenceRefs  []string                `json:"evidenceRefs,omitempty"`
}

type TaskDecompositionNode struct {
	ID               string         `json:"id"`
	Kind             string         `json:"kind"`
	Title            string         `json:"title,omitempty"`
	State            string         `json:"state"`
	DependsOn        []string       `json:"dependsOn,omitempty"`
	Branch           string         `json:"branch,omitempty"`
	StepID           string         `json:"stepId,omitempty"`
	StepState        string         `json:"stepState,omitempty"`
	RetryOf          string         `json:"retryOf,omitempty"`
	RetryAttempt     int            `json:"retryAttempt,omitempty"`
	FailureClass     string         `json:"failureClass,omitempty"`
	FailureSignature string         `json:"failureSignature,omitempty"`
	TargetScope      string         `json:"targetScope,omitempty"`
	BlockingTicketID string         `json:"blockingTicketId,omitempty"`
	EvidenceRefs     []string       `json:"evidenceRefs,omitempty"`
	CoverageTargets  []string       `json:"coverageTargets,omitempty"`
	Input            map[string]any `json:"input,omitempty"`
}

type TaskPlan struct {
	ID                string                               `json:"id"`
	WorkspaceID       string                               `json:"workspaceId"`
	RunID             string                               `json:"runId"`
	AnchorStepID      string                               `json:"anchorStepId"`
	RevisionStepID    string                               `json:"revisionStepId"`
	Revision          int                                  `json:"revision"`
	State             string                               `json:"state"`
	Stage             string                               `json:"stage,omitempty"`
	SourceOutputState string                               `json:"sourceOutputState,omitempty"`
	Summary           string                               `json:"summary,omitempty"`
	GraphBranch       string                               `json:"graphBranch,omitempty"`
	NodeTotal         int                                  `json:"nodeTotal"`
	NodeCompleted     int                                  `json:"nodeCompleted"`
	NodeFailed        int                                  `json:"nodeFailed"`
	NodeRunning       int                                  `json:"nodeRunning"`
	Nodes             []TaskDecompositionNode              `json:"nodes,omitempty"`
	RetryOf           string                               `json:"retryOf,omitempty"`
	RetryAttempt      int                                  `json:"retryAttempt,omitempty"`
	FailureClass      string                               `json:"failureClass,omitempty"`
	FailureSignature  string                               `json:"failureSignature,omitempty"`
	TargetScope       string                               `json:"targetScope,omitempty"`
	BlockingTicketID  string                               `json:"blockingTicketId,omitempty"`
	EvidenceRefs      []string                             `json:"evidenceRefs,omitempty"`
	OccurredAt        string                               `json:"occurredAt,omitempty"`
	SourceProjection  string                               `json:"sourceProjection,omitempty"`
	SealedLLMTarget   *runtimecontract.ProviderModelTarget `json:"sealedLLMTarget,omitempty"`
	RuntimeActivation *RuntimeActivationStatus             `json:"runtimeActivation,omitempty"`
}
