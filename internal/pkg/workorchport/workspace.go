package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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
	ViewModel            WorkspaceViewModelInfo
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

type GapBaselineInfo struct {
	WorkspaceID           common.WorkspaceID
	RunID                 common.RunID
	DefinitionRevision    string
	BaselineRevision      string
	CurrentFocus          workorcheventpayload.Payload
	OpenCoverageWorkItems []workorcheventpayload.Payload
	AcceptedEvidenceRefs  []string
	RejectedEvidenceRefs  []string
	TodoItems             []GapTodoItemInfo
	NextAction            GapNextActionInfo
	Diagnostics           []GapDiagnosticInfo
}

type GapTodoItemInfo struct {
	ID                    string
	Source                string
	SourceRef             string
	CoverageWorkItemIDs   []string
	RequiredEvidenceKinds []string
	Status                string
	Priority              int
	Reason                string
	Metadata              workorcheventpayload.Payload
}

type GapNextActionInfo struct {
	Kind   string
	Reason string
}

type GapDiagnosticInfo struct {
	Code     string
	Severity string
	Message  string
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
	Workspace               WorkspaceInfo
	ViewModel               WorkspaceViewModelInfo
	Projection              ProjectionInfo
	Outcome                 WorkspaceOutcomeInfo
	TaskProgress            TaskProgressInfo
	RunHeartbeat            RunHeartbeatInfo
	ConvergenceStatus       *ConvergenceStatusInfo
	RunCommand              *RunCommandStatusInfo
	SchedulerDiagnostics    *SchedulerDiagnosticsInfo
	ContextBundle           workorcheventpayload.Payload
	CoverageSummary         workorcheventpayload.Payload
	GapBaseline             *GapBaselineInfo
	Acceptance              AcceptanceProjectionInfo
	TaskDecompositions      []TaskDecompositionInfo
	TaskPlans               []TaskPlanInfo
	Goals                   []GoalInfo
	DoDs                    []DoDInfo
	Runs                    []RunInfo
	Results                 []ResultInfo
	Artifacts               []ArtifactInfo
	ArtifactContinuations   []ArtifactContinuationInfo
	Patches                 []PatchInfo
	MemoryRecallTraces      []MemoryRecallTraceInfo
	WebSources              []WebSourceTraceInfo
	LLMContextBudgets       []LLMContextBudgetTraceInfo
	LLMContextBudgetSummary LLMContextBudgetSummaryInfo
	LLMUsageTraces          []LLMUsageTraceInfo
	LLMUsageSummary         LLMUsageSummaryInfo
	EventRecords            []EventRecordInfo
	Decisions               []DecisionInfo
	Approvals               []ApprovalInfo
}

type WorkspaceViewModelInfo struct {
	WorkspaceID             common.WorkspaceID
	State                   string
	PromptMode              string
	Revision                int64
	GlobalRevision          int64
	WorkspaceRevision       int64
	SourceEventRevision     int64
	ProjectedSourceRevision int64
	UpdatedAt               string
	RebuiltAt               string
	Degraded                bool
	StaleReason             string
	Error                   string
	Sections                map[string]WorkspaceViewModelSectionInfo
}

type WorkspaceViewModelSectionInfo struct {
	Name                    string
	Revision                int64
	WorkspaceRevision       int64
	SourceEventRevision     int64
	ProjectedSourceRevision int64
	State                   string
}

type RunCommandStatusInfo struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	State              string
	Intent             string
	Reason             string
	RequestID          string
	IdempotencyKey     string
	Source             string
	DefinitionRevision string
	DecisionID         common.TicketID
	AcceptedAt         string
	RejectedAt         string
	Error              string
	Revision           int64
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
