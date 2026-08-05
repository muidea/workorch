package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

type ResultInfo struct {
	ID            common.ResultID
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	GoalID        common.GoalID
	DoDID         common.DoDID
	State         common.ResultState
	Title         string
	Summary       string
	SourceType    string
	SourceRef     string
	EvidenceRefs  []string
	ArtifactRefs  []string
	DoDAssessment *workorchresultcontract.DoDAssessment
	SummaryDetail *workorchresultcontract.ResultSummary
	CreatedAt     string
	UpdatedAt     string
}

type ArtifactInfo struct {
	ID                 common.ArtifactID
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	ResultID           common.ResultID
	StepID             common.StepID
	ToolCallID         common.ToolCallID
	PatchID            common.PatchID
	ValidationResultID common.ValidationResultID
	ArtifactType       string
	Category           string
	Name               string
	Summary            string
	URI                string
	ContentType        string
	State              common.ArtifactState
	EvidenceRefs       []string
	Metadata           workorcheventpayload.Payload
	CreatedAt          string
	UpdatedAt          string
}

type ArtifactContinuationInfo struct {
	ID                common.ArtifactContinuationID
	WorkspaceID       common.WorkspaceID
	RunID             common.RunID
	RootStepID        common.StepID
	ArtifactID        string
	TargetRef         string
	Operation         string
	State             common.ArtifactContinuationState
	ExpectedSequence  int
	ReceivedSequences []int
	FinalReceived     bool
	BytesWritten      int64
	RollingHash       string
	LastChunkHash     string
	CoverageTargets   []string
	EvidenceRefs      []string
	FailureReason     string
	CreatedAt         string
	UpdatedAt         string
	Progress          workorcheventpayload.Payload
}

type PatchInfo struct {
	ID               common.PatchID
	WorkspaceID      common.WorkspaceID
	RunID            common.RunID
	StepID           common.StepID
	ToolCallID       common.ToolCallID
	ApprovalID       common.ApprovalID
	RevertApprovalID common.ApprovalID
	PatchType        string
	TargetRef        string
	Summary          string
	State            common.PatchState
	AddedLines       int
	DeletedLines     int
	BeforeHash       string
	AfterHash        string
	AffectedFiles    []string
	RiskLevel        string
	RiskReasons      []string
	Diagnostics      []workorcheventpayload.Payload
	CreatedAt        string
	AppliedAt        string
	RevertedAt       string
}
