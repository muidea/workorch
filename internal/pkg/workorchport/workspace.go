package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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
	// RepairDirective is a display-only copy of the persisted SM-2 canonical
	// repair authority. Clients must not derive an action from Audit data.
	RepairDirective *RuntimeRepairDirectiveInfo
	Audit           *RuntimeV2AuditInfo
	Summary         AcceptanceSummaryInfo
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

// RuntimeV2AuditInfo is observation-only. It contains the assessment, repair,
// and bounded failure diagnostic values needed to distinguish producer
// observations from the persisted canonical directive at the product boundary.
type RuntimeV2AuditInfo struct {
	Repair            RuntimeV2AuditRepairInfo
	Assessments       []RuntimeV2AuditAssessmentInfo
	Failure           *RuntimeV2AuditFailureInfo
	CoverageBindings  []RuntimeV2AuditCoverageBindingInfo
	Dispatches        []RuntimeV2AuditDispatchInfo
	AcceptedEvidence  []RuntimeV2AuditEvidenceInfo
	RejectedEvidence  []RuntimeV2AuditEvidenceInfo
	OperationReceipts []RuntimeV2AuditOperationReceiptInfo
}

// The following audit DTOs remain display-only.  They preserve the CWI ->
// binding -> evidence -> owner receipt chain without providing any mutation,
// retry, or continuation command at the process boundary.
type RuntimeV2AuditCoverageBindingInfo struct {
	ID                       string
	CoverageWorkItemID       string
	FocusID                  string
	DefinitionRevision       string
	Hash                     string
	Proofs                   []RuntimeV2AuditCoverageProofInfo
	TargetRefs               []string
	TargetSelectorRef        string
	TargetSelectorOwnerRef   string
	TargetSelectorSubjectRef string
	ProspectiveTargetRef     string
	DeclarationRef           string
	PrerequisiteRefs         []string
}

type RuntimeV2AuditCoverageProofInfo struct {
	EvidenceKind             string
	RequiredEffectScopes     []string
	AllowedOperationKinds    []string
	AllowedOperationVariants []string
	ValidationRecipeID       string
	RequiredCheckRefs        []string
	MetricConstraints        []string
	ArtifactRefs             []string
}

// RuntimeV2AuditDispatchInfo is a display-only projection of a persisted
// sealed dispatch. Operations expose only the authorization identity needed
// to correlate bindingRefs with evidence and owner receipts.
type RuntimeV2AuditDispatchInfo struct {
	ID                  string
	AuthoritySnapshotID string
	State               string
	CandidateHash       string
	CompletedOperations int
	Operations          []RuntimeV2AuditSealedOperationInfo
}

type RuntimeV2AuditSealedOperationInfo struct {
	ID          string
	Kind        string
	BindingRefs []string
	TargetRefs  []string
}

type RuntimeV2AuditEvidenceInfo struct {
	ID                  string
	Kind                string
	AuthoritySnapshotID string
	DispatchID          string
	OperationID         string
	Revision            string
	FailureClass        string
	FailureCode         string
	FailureSignature    string
	TargetRefs          []string
	BindingRefs         []string
	CoverageWorkItemIDs []string
	Quality             string
	ZeroWork            bool
	EvidenceReason      string
	EvidenceRefs        []string
	EvidenceTags        []string
	ValidationRecipeID  string
	ExecutedChecks      []string
	PassedChecks        []string
	FailedChecks        []string
	SkippedChecks       []string
}

type RuntimeV2AuditOperationReceiptInfo struct {
	ExecutionKey        string
	Fence               uint64
	OperationID         string
	Outcome             string
	ObservedEffectScope string
	Changed             bool
	FeedbackDomain      string
	StableCategory      string
	OwnerModuleID       string
	OriginalClass       string
	OriginalCode        string
	OriginalMessage     string
	Signature           string
	TargetRefs          []string
	EvidenceRefs        []string
	EvidenceTags        []string
	ValidationRecipeID  string
	ExecutedChecks      []string
	PassedChecks        []string
	FailedChecks        []string
	SkippedChecks       []string
	ExpectedRevision    string
	ObservedRevision    string
	DetailsRef          string
	DetailsHash         string
	ContentType         string
	Truncated           bool
	Redacted            bool
}

type RuntimeV2AuditRepairInfo struct {
	FocusID                       string
	SliceID                       string
	RepairAttempts                int
	ValidationOnlyRetries         int
	NoProgressCount               int
	NoProgressOperationShapeCount int
	ActiveGapCoverageWorkItemIDs  []string
	ActiveGapFailureClass         string
	ActiveGapFailureCode          string
	ActiveGapFailureSignature     string
	LastProgressRevision          string
	Exhausted                     bool
	TerminalDeliveryAttempts      int
}

type RuntimeV2AuditAssessmentInfo struct {
	ID                   string
	FocusID              string
	SliceID              string
	Scope                string
	RequirementID        string
	RequirementText      string
	ClaimedDisposition   string
	CanonicalDisposition string
	CoverageWorkItemIDs  []string
	Summary              string
	EvidenceRefs         []string
	ArtifactRefs         []string
	Items                []RuntimeV2AuditAssessmentItemInfo
	Gap                  *RuntimeV2AuditAssessmentGapInfo
}

type RuntimeV2AuditAssessmentItemInfo struct {
	CoverageWorkItemID string
	Satisfied          bool
	Reason             string
	Disposition        string
	Diagnostic         string
	EvidenceRefs       []string
	ArtifactRefs       []string
}

// RuntimeV2AuditAssessmentGapInfo keeps the safe original producer
// diagnostic. It is intentionally not an action/retry/terminal contract.
type RuntimeV2AuditAssessmentGapInfo struct {
	CoverageWorkItemIDs  []string
	MissingEvidenceKinds []string
	TargetRefs           []string
	EvidenceRefs         []string
	Reason               string
	FailureClass         string
	FailureCode          string
	FailureSignature     string
}

// RuntimeV2AuditFailureInfo is a display-only stable failure summary plus the
// original safe producer diagnostic.  It never represents an action request.
type RuntimeV2AuditFailureInfo struct {
	Class              string
	Code               string
	Signature          string
	Message            string
	TargetRefs         []string
	OriginalDiagnostic *RuntimeV2AuditOriginalDiagnosticInfo
}

type RuntimeV2AuditOriginalDiagnosticInfo struct {
	OwnerModuleID    string
	OriginalClass    string
	OriginalCode     string
	OriginalMessage  string
	Signature        string
	ExpectedRevision string
	ObservedRevision string
	Redacted         bool
	OmittedReason    string
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
