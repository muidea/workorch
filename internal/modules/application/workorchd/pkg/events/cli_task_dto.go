package events

import (
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type CLITaskDecompositionNodeInfo struct {
	ID               string
	Kind             string
	Title            string
	State            string
	DependsOn        []string
	Branch           string
	StepID           string
	StepState        string
	RetryOf          string
	RetryAttempt     int
	FailureClass     string
	FailureSignature string
	TargetScope      string
	BlockingTicketID string
	EvidenceRefs     []string
	CoverageTargets  []string
	Input            workorcheventpayload.Payload
}

type CLITaskDecompositionInfo struct {
	ID            string
	WorkspaceID   string
	RunID         string
	RootStepID    string
	State         string
	Summary       string
	GraphBranch   string
	NodeTotal     int
	NodeCompleted int
	NodeFailed    int
	NodeRunning   int
	Nodes         []CLITaskDecompositionNodeInfo
	EvidenceRefs  []string
}

type CLITaskPlanInfo struct {
	ID                 string
	WorkspaceID        string
	RunID              string
	AnchorStepID       string
	RevisionStepID     string
	Revision           int
	State              string
	Stage              string
	SourceOutputState  string
	Summary            string
	GraphBranch        string
	NodeTotal          int
	NodeCompleted      int
	NodeFailed         int
	NodeRunning        int
	Nodes              []CLITaskDecompositionNodeInfo
	RetryOf            string
	RetryAttempt       int
	FailureClass       string
	FailureSignature   string
	TargetScope        string
	BlockingTicketID   string
	EvidenceRefs       []string
	OccurredAt         string
	SourceProjection   string
	SealedLLMSelection *runtimecontract.ProviderInferenceSelection
}
