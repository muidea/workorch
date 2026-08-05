package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type MemoryRecallTraceInfo struct {
	ID                     common.MemoryRecallTraceID
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	StepID                 common.StepID
	GraphNodeID            string
	Stage                  string
	Source                 string
	Policy                 string
	Status                 string
	ProviderID             string
	Operation              string
	QueryTextPreview       string
	QueryTextSha256        string
	MinScore               float64
	Limit                  int
	EmbeddingModel         string
	VectorDimension        int
	RefreshSource          string
	SelectionPolicyVersion string
	CandidateMemoryIDs     []common.MemoryID
	SelectedMemoryIDs      []common.MemoryID
	ExcludedMemoryIDs      []common.MemoryID
	TypeQuota              map[string]int
	SelectionScores        []workorchmemorycontract.RecallSelectionScore
	RetrievedMemoryIDs     []common.MemoryID
	RetrievedMemoryHits    []workorchmemorycontract.RecallHit
	Reason                 string
	OccurredAt             string
	EvidenceRef            string
}

type WebSourceTraceInfo struct {
	Query          string
	ProviderID     string
	SearchedAt     string
	Sources        []projectioncontract.WebSource
	FetchedSources []projectioncontract.FetchedWebSource
	EvidenceRefs   []string
	ApprovalID     common.ApprovalID
	PolicyDecision string
}

type LLMUsageTraceInfo struct {
	ID            string
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	StepID        common.StepID
	ProviderID    string
	Provider      string
	APISurface    string
	Model         string
	Operation     string
	InputTokens   int
	OutputTokens  int
	TotalTokens   int
	Estimated     bool
	UsageSource   string
	RequestBytes  int
	ResponseBytes int
	Streaming     bool
	ContextBudget workorcheventpayload.Payload
	CacheUsage    workorcheventpayload.Payload
	CacheMarkers  workorcheventpayload.Payload
	StartedAt     string
	CompletedAt   string
	DurationMs    int
	Status        string
	FailureClass  string
	OccurredAt    string
	EvidenceRef   string
}

type LLMContextBudgetTraceInfo struct {
	StepID                common.StepID
	ProviderID            string
	Model                 string
	CapabilityRevision    string
	ConfigurationRevision string
	MaxBytes              int
	ActualBytes           int
	MaxTokens             int
	EstimatedInputTokens  int
	EstimatedOutputTokens int
	ActualTokens          int
	ShrinkLevel           int
	Truncated             bool
	Rules                 []string
	SourceRefs            []string
	IncludedSections      []string
	NormalizedSections    []string
	CompressedSections    []string
	OmittedSections       []string
	GoalDoDCompressed     bool
	OverBudget            bool
	P0Overflow            bool
	ProfileMode           string
	CapabilitySource      string
	ContextUtilization    float64
	MaxRequestBytes       int
	StageBudgetRatio      float64
	StageBudgetSource     string
	TracePolicy           string
	PromptCaptured        bool
	Estimator             string
	ProviderSubmitted     bool
	ContextGateAction     string
	ContextGateReason     string
	BudgetFailureClass    string
	RedactedPaths         []string
	ProtectedFields       []string
	IntegrityWarnings     []string
	SectionHashes         []projectioncontract.LLMContextBudgetSection
	Profile               workorcheventpayload.Payload
	CachePlan             workorcheventpayload.Payload
}

type LLMUsageSummaryInfo struct {
	EntryTotal              int
	InputTokens             int
	OutputTokens            int
	TotalTokens             int
	EstimatedEntries        int
	ProviderNativeEntries   int
	FailedEntries           int
	CacheReadTokens         int
	CacheWriteTokens        int
	CacheCreationTokens     int
	CacheableInputTokens    int
	NonCacheableInputTokens int
	CacheReadInputRatio     float64
	CacheableInputRatio     float64
	NonCacheableInputRatio  float64
	CacheEstimatedEntries   int
	CacheProviderEntries    int
	ProviderModelBreakdown  []LLMUsageProviderModelBreakdownInfo
}

type LLMUsageProviderModelBreakdownInfo struct {
	ProviderID              string
	Provider                string
	APISurface              string
	Model                   string
	ProviderMode            string
	EntryTotal              int
	InputTokens             int
	OutputTokens            int
	TotalTokens             int
	CacheReadTokens         int
	CacheWriteTokens        int
	CacheCreationTokens     int
	CacheablePromptCount    int
	ActualCacheHitCount     int
	CacheCreationCount      int
	PromptCacheKeySentCount int
	CacheControlSentCount   int
	AutoPrefixExpectedCount int
	CacheReadInputRatio     float64
	EstimatedEntries        int
	FailedEntries           int
}

type EventRecordInfo struct {
	ID             common.EventRecordID
	Version        string
	Topic          string
	OccurredAt     string
	SourceModule   string
	SubjectType    string
	SubjectID      string
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	StepID         common.StepID
	Revision       int64
	Payload        workorcheventpayload.Payload
	PayloadSummary workorcheventpayload.Payload
	Summary        string
	PayloadRef     string
}

type RecoveryPlanInfo struct {
	Mode               string
	NextAction         string
	RunID              common.RunID
	StepID             common.StepID
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
	CheckpointID       common.CheckpointID
	FailureClass       string
	FailureSignature   string
	Reason             string
}

type RepairLoopInfo struct {
	State                    string
	AttemptCount             int
	LastAttempt              int
	LastRepairStepID         common.StepID
	LastFailureStepID        common.StepID
	LastFailureClass         string
	LastFailureSignature     string
	LastFailureTargetScope   string
	LastFailureAt            string
	StructuredFailureSignals workorcheventpayload.Payload
	RepairInputCompaction    workorcheventpayload.Payload
	LastRepairInputBytes     int
	RepeatedFailureReason    string
	MissingDoDItems          []workorcheventpayload.Payload
	RecentAttempts           []workorcheventpayload.Payload
	RecommendedNextSteps     []string
	NextAction               string
	Escalated                bool
	EscalationReason         string
	EscalationTicketID       common.TicketID
	EvidenceRefs             []string
}

type TaskProgressInfo struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	State              string
	Summary            string
	CurrentStepID      common.StepID
	CurrentStepKind    string
	CurrentStepState   string
	CurrentActionLabel string
	DiagnosticLineage  common.StepID
	CurrentMilestone   string
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
	Milestones         []projectioncontract.ProgressMilestone
	Items              []projectioncontract.ProgressItem
	Blockers           []projectioncontract.ProgressBlocker
	Recovery           RecoveryPlanInfo
	RepairLoop         RepairLoopInfo
	AssessmentOutcome  AssessmentOutcomeInfo
	EvidenceRefs       []string
	Runtime            *RuntimeProgressInfo
}

// RuntimeProgressInfo is a product-boundary summary projected from Run.RuntimeState.
type RuntimeProgressInfo struct {
	StateRevision                   uint64
	EvidenceSequence                uint64
	CompletedDispatchCount          int
	ActiveDispatchID                string
	CandidateAttemptCount           int
	AcceptedEvidenceCount           int
	AcceptedValidationEvidenceCount int
	RejectedEvidenceCount           int
	MissingEvidenceKinds            []string
	CurrentFocusID                  string
	ConvergenceState                string
	TerminalCode                    string
	FailureClass                    string
	FailureCode                     string
	FailureSignature                string
	FailureMessage                  string
	RepairHintCodes                 []string
	PrerequisiteRefreshState        string
	PrerequisiteRefreshUsed         int
	PrerequisiteRefreshLimit        int
	PrerequisiteInvalidations       []RuntimePrerequisiteRefreshInvalidationInfo
}

// RuntimePrerequisiteRefreshInvalidationInfo is display-only freshness audit
// data. It must not be used by a client to choose or trigger Runtime work.
type RuntimePrerequisiteRefreshInvalidationInfo struct {
	Signature        string
	Source           string
	PrerequisiteIDs  []string
	ObservationIDs   []string
	DispatchID       string
	OperationID      string
	ReceiptHash      string
	OwnerModuleID    string
	ResourceDomain   string
	SubjectRef       string
	ExpectedRevision string
	ObservedRevision string
	Reason           string
	ResourceChanges  []projectioncontract.RuntimePrerequisiteResourceChange
}

type AssessmentOutcomeInfo struct {
	State                       string
	AcceptedSlices              int
	AcceptedResultCount         int
	RejectedGlobalAssessments   int
	SatisfiedCoverageCount      int
	RemainingCoverageCount      int
	NewlySatisfiedCoverageCount int
	LatestAcceptedResultID      common.ResultID
	LatestRejectedResultID      common.ResultID
	LatestRejectedReason        string
}

type TaskDecompositionNodeInfo struct {
	ID               string
	Kind             string
	Title            string
	State            string
	DependsOn        []string
	Branch           string
	StepID           common.StepID
	StepState        string
	RetryOf          string
	RetryAttempt     int
	FailureClass     string
	FailureSignature string
	TargetScope      string
	BlockingTicketID common.TicketID
	EvidenceRefs     []string
	CoverageTargets  []string
	Input            workorcheventpayload.Payload
}

type TaskDecompositionInfo struct {
	ID            string
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	RootStepID    common.StepID
	State         string
	Summary       string
	GraphBranch   string
	NodeTotal     int
	NodeCompleted int
	NodeFailed    int
	NodeRunning   int
	Nodes         []TaskDecompositionNodeInfo
	EvidenceRefs  []string
}

type TaskPlanInfo struct {
	ID                string
	WorkspaceID       common.WorkspaceID
	RunID             common.RunID
	AnchorStepID      common.StepID
	RevisionStepID    common.StepID
	Revision          int
	State             string
	Stage             string
	SourceOutputState string
	Summary           string
	GraphBranch       string
	NodeTotal         int
	NodeCompleted     int
	NodeFailed        int
	NodeRunning       int
	Nodes             []TaskDecompositionNodeInfo
	RetryOf           string
	RetryAttempt      int
	FailureClass      string
	FailureSignature  string
	TargetScope       string
	BlockingTicketID  common.TicketID
	EvidenceRefs      []string
	OccurredAt        string
	SourceProjection  string
	SealedLLMTarget   *runtimecontract.ProviderModelTarget
	RuntimeActivation *projectioncontract.RuntimeActivationStatus
}
