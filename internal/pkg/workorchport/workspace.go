package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateWorkspaceRequest struct {
	Title                string
	Description          string
	DeliverableDirectory string
	TemplateID           string
}

type WorkspaceInfo struct {
	ID                   common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
	BlockingTicketID     common.TicketID
	BlockingApprovalID   common.ApprovalID
	TemplateID           string
	ProfileID            string
	DefaultSoulID        common.SoulID
	SoulBindings         map[string]common.SoulID
	Projection           ProjectionInfo
}

type ProjectionInfo struct {
	State                   string
	GlobalRevision          int64
	WorkspaceRevision       int64
	SourceEventRevision     int64
	ProjectedSourceRevision int64
	UpdatedAt               string
	RebuiltAt               string
	Degraded                bool
	StaleReason             string
	Error                   string
}

type AcceptanceSliceInfo struct {
	ID                    string
	Title                 string
	State                 string
	Priority              int
	Scope                 string
	CoverageWorkItemIDs   []string
	RequiredEvidenceKinds []string
	CompletionPolicy      string
	ResultPolicy          string
	CreatedAt             string
	ActivatedAt           string
	ClosedAt              string
}

type CoverageFocusInfo struct {
	ID                    string
	SliceID               string
	CoverageWorkItemIDs   []string
	FailureClass          string
	FailureSignature      string
	RequiredEvidenceKinds []string
	AcceptedEvidenceRefs  []string
	RejectedEvidenceRefs  []string
	TargetRefs            []string
	State                 string
	Attempt               int
}

type ConvergenceBudgetInfo struct {
	State                       string
	Decision                    string
	MaxRepairAttemptsPerFocus   int
	MaxLLMRoundsPerSlice        int
	MaxRepeatedFailureSignature int
	MaxValidationOnlyRetries    int
	MaxNoCoverageDeltaRounds    int
	MaxContextBytesPerRepair    int
	RepairAttempts              int
	LLMRounds                   int
	RepeatedFailureSignatures   int
	ValidationOnlyRetries       int
	NoCoverageDeltaRounds       int
	LastDecisionReason          string
}

type AcceptanceSummaryInfo struct {
	AcceptedSlices int
	ActiveSlices   int
	OpenSlices     int
	FailedSlices   int
}

type AcceptanceProjectionInfo struct {
	ActiveSliceID string
	Slices        []AcceptanceSliceInfo
	CurrentFocus  *CoverageFocusInfo
	Budget        *ConvergenceBudgetInfo
	Summary       AcceptanceSummaryInfo
}

type WorkspaceOutcomeInfo struct {
	WorkspaceID  common.WorkspaceID
	State        string
	RunID        common.RunID
	ResultID     common.ResultID
	Accepted     bool
	Terminal     bool
	Summary      string
	Reason       string
	EvidenceRefs []string
	ArtifactRefs []string
}

type WorkspaceSnapshotInfo struct {
	Workspace             WorkspaceInfo
	Projection            ProjectionInfo
	Outcome               WorkspaceOutcomeInfo
	TaskProgress          TaskProgressInfo
	RunHeartbeat          RunHeartbeatInfo
	RuntimeExecution      *RuntimeExecutionInfo
	SchedulerDiagnostics  *SchedulerDiagnosticsInfo
	ContextBundle         map[string]any
	CoverageSummary       map[string]any
	Acceptance            AcceptanceProjectionInfo
	TaskDecompositions    []TaskDecompositionInfo
	Goals                 []GoalInfo
	DoDs                  []DoDInfo
	Runs                  []RunInfo
	Results               []ResultInfo
	Artifacts             []ArtifactInfo
	ArtifactContinuations []ArtifactContinuationInfo
	Patches               []PatchInfo
	MemoryRecallTraces    []MemoryRecallTraceInfo
	WebSources            []WebSourceTraceInfo
	LLMUsageTraces        []LLMUsageTraceInfo
	LLMUsageSummary       LLMUsageSummaryInfo
	EventRecords          []EventRecordInfo
	Decisions             []DecisionInfo
	Approvals             []ApprovalInfo
}

type RunHeartbeatInfo struct {
	WorkspaceID               common.WorkspaceID
	RunID                     common.RunID
	State                     string
	ActivityState             string
	LatestStepID              common.StepID
	LatestStepKind            string
	LatestStepState           string
	LatestEvidenceRef         string
	LatestEventTopic          string
	LatestEventRevision       int64
	LatestEventAt             string
	LatestLLMUsageAt          string
	LatestResultAt            string
	LatestArtifactAt          string
	LatestPatchAt             string
	LatestEffectiveProgressAt string
	IneffectiveRetry          bool
	Stalled                   bool
	Summary                   string
}

type WorkspaceBackend interface {
	Create(ctx context.Context, req CreateWorkspaceRequest) (*WorkspaceInfo, error)
	Get(ctx context.Context, id common.WorkspaceID) (*WorkspaceInfo, error)
	List(ctx context.Context) ([]WorkspaceInfo, error)
	Snapshot(ctx context.Context, id common.WorkspaceID) (*WorkspaceSnapshotInfo, error)
	Delete(ctx context.Context, id common.WorkspaceID, purge bool) error
}
