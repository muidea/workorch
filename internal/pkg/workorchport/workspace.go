package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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
	DefaultLLMTarget     *runtimecontract.ProviderModelTarget
	DefaultSoulID        common.SoulID
	SoulBindings         map[string]common.SoulID
	ContextHints         workorcheventpayload.Payload
	EvidencePolicies     []workorcheventpayload.Payload
	Projection           ProjectionInfo
	ViewModel            WorkspaceViewModelInfo
}

type ProjectionInfo struct {
	State                   string
	StreamGeneration        string
	GlobalRevision          int64
	WorkspaceRevision       int64
	SourceEventRevision     int64
	ProjectedSourceRevision int64
	UpdatedAt               string
	RebuiltAt               string
	Degraded                bool
	StaleReason             string
	Error                   string
	FailedWorkspaceIds      []string
	DegradedSince           string
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
	FocusID                      string
	SliceID                      string
	State                        string
	Decision                     string
	MaxRepairAttemptsPerFocus    int
	MaxLLMRoundsPerSlice         int
	MaxRepeatedFailureSignature  int
	MaxValidationOnlyRetries     int
	MaxNoCoverageDeltaRounds     int
	MaxContextBytesPerRepair     int
	RepairAttempts               int
	LLMRounds                    int
	RepeatedFailureSignatures    int
	ValidationOnlyRetries        int
	NoCoverageDeltaRounds        int
	LastNoDeltaReason            string
	LastDecisionReason           string
	MaxProviderTransportAttempts int
	ProviderTransportAttempts    int
	MaxOperationRecoveryAttempts int
	OperationRecoveryAttempts    int
	MaxCASReloadAttempts         int
	CASReloadAttempts            int
	MaxTerminalDeliveryAttempts  int
	TerminalDeliveryAttempts     int
	MaxPrerequisiteRefreshes     int
	PrerequisiteRefreshes        int
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
	// RepairDirective is a display-only copy of the persisted SM-2 canonical
	// repair authority. Clients must not derive an action from Audit data.
	RepairDirective  *RuntimeRepairDirectiveInfo
	SemanticPlanning *RuntimeSemanticPlanningInfo
	Audit            *projectioncontract.RuntimeAudit
	Summary          AcceptanceSummaryInfo
}

type RuntimeSemanticPlanningInfo struct {
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	DefinitionRevision     string
	StateRevision          uint64
	State                  string
	Phase                  string
	Attempt                int
	CatalogRefreshAttempts int
	LogicalRequestID       string
	ManifestHash           string
	DocumentHash           string
	FeasibilityBatchID     string
	FeasibilitySetHash     string
	SealedBindingSetHash   string
	Items                  []RuntimeSemanticPlanningItemInfo
	Rejections             []RuntimeSemanticDiagnosticInfo
	FeasibilityGaps        []RuntimeSemanticFeasibilityGapInfo
	Failure                *RuntimeSemanticDiagnosticInfo
}

type RuntimeSemanticPlanningItemInfo struct {
	CoverageWorkItemID string
	ProposalHash       string
	OwnerModuleID      string
	OperationKind      string
	Status             string
	TargetRefs         []string
	ValidationRecipeID string
	RequiredCheckRefs  []string
	ObservedRevision   string
	Diagnostic         *RuntimeSemanticDiagnosticInfo
}

type RuntimeSemanticFeasibilityGapInfo struct {
	RefreshGeneration  int
	ManifestHash       string
	DocumentHash       string
	FeasibilityBatchID string
	FeasibilitySetHash string
	Items              []RuntimeSemanticPlanningItemInfo
	Rejections         []RuntimeSemanticDiagnosticInfo
}

type RuntimeSemanticDiagnosticInfo struct {
	SchemaVersion    string
	OwnerModuleID    string
	OriginalClass    string
	OriginalCode     string
	OriginalMessage  string
	Signature        string
	OperationID      string
	TargetRefs       []string
	ExpectedRevision string
	ObservedRevision string
	FieldViolations  []runtimecontract.RuntimeFieldViolation
	DetailsRef       string
	DetailsHash      string
	DetailsBytes     int64
	ContentType      string
	OmittedReason    string
	Truncated        bool
	Redacted         bool
}

type RuntimeRepairDirectiveInfo struct {
	ID                         string
	FocusID                    string
	Trigger                    string
	RequiredAction             string
	RequiredEffectScopes       []string
	RequiredEvidenceKinds      []string
	AllowedOperationKinds      []string
	CoverageWorkItemIDs        []string
	TargetRefs                 []string
	Closure                    *RuntimeRepairClosureInfo
	SupersedesEvidenceRevision string
	RepairAttempt              int
}

type RuntimeRepairClosureInfo struct {
	ID                                string
	GapSignature                      string
	BaselineEvidenceRevision          string
	CoverageWorkItemIDs               []string
	RequiredEvidenceKinds             []string
	GroundedTargetRefs                []string
	RequireValidationAfterStateChange bool
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
	Disposition        string
	Diagnostic         string
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
	RuntimeView(ctx context.Context, req ReadWorkspaceRuntimeViewRequest) (*WorkspaceRuntimeViewInfo, error)
	UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*WorkspaceInfo, error)
	Delete(ctx context.Context, id common.WorkspaceID, purge bool) error
}
