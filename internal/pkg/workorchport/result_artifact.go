package workorchport

import "github.com/muidea/workorch/internal/pkg/common"

type ResultSummaryInfo struct {
	GoalOutcome           string
	ImplementationSummary string
	VerificationSummary   string
	EvidenceRefs          []string
	ArtifactRefs          []string
	KnownLimitations      []string
	ResidualRisks         []string
	NextActions           []string
	Diagnostics           map[string]any
}

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
	SummaryDetail *ResultSummaryInfo
}

type ArtifactInfo struct {
	ID           common.ArtifactID
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	ResultID     common.ResultID
	ArtifactType string
	Category     string
	Name         string
	Summary      string
	URI          string
	ContentType  string
	State        common.ArtifactState
	EvidenceRefs []string
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
	Progress          map[string]any
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
	CreatedAt        string
	AppliedAt        string
	RevertedAt       string
}
