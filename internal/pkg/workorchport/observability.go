package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type MemoryRecallHitInfo struct {
	ID              common.MemoryID
	Summary         string
	VectorScore     float64
	VectorDimension int
}

type MemoryRecallTraceInfo struct {
	ID                  common.MemoryRecallTraceID
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	StepID              common.StepID
	GraphNodeID         string
	Stage               string
	Source              string
	Policy              string
	Status              string
	ProviderID          string
	Operation           string
	QueryTextPreview    string
	QueryTextSha256     string
	MinScore            float64
	Limit               int
	EmbeddingModel      string
	VectorDimension     int
	RetrievedMemoryIDs  []common.MemoryID
	RetrievedMemoryHits []MemoryRecallHitInfo
	Reason              string
	OccurredAt          string
	EvidenceRef         string
}

type WebSourceTraceInfo struct {
	Query          string
	ProviderID     string
	SearchedAt     string
	Sources        []WebSourceInfo
	FetchedSources []FetchedWebSourceInfo
	EvidenceRefs   []string
	ApprovalID     common.ApprovalID
	PolicyDecision string
}

type WebSourceInfo struct {
	Title       string
	URL         string
	Snippet     string
	Source      string
	PublishedAt string
	Rank        int
	EvidenceRef string
}

type FetchedWebSourceInfo struct {
	URL         string
	FinalURL    string
	Title       string
	Source      string
	RetrievedAt string
	ContentHash string
	Truncated   bool
	EvidenceRef string
}

type LLMUsageTraceInfo struct {
	ID            string
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	StepID        common.StepID
	ProviderID    string
	Provider      string
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
	Status        string
	FailureClass  string
	OccurredAt    string
	EvidenceRef   string
}

type LLMContextBudgetTraceInfo struct {
	StepID                common.StepID
	ProviderID            string
	Model                 string
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
	Profile               workorcheventpayload.Payload
	CachePlan             workorcheventpayload.Payload
}

type LLMContextBudgetSummaryInfo struct {
	TraceTotal             int
	TruncatedCount         int
	OverBudgetCount        int
	P0OverflowCount        int
	GoalDoDCompressedCount int
	PromptCapturedCount    int
	MaxShrinkLevel         int
	MaxContextUtilization  float64
	AvgContextUtilization  float64
	MaxInputTokens         int
	MaxRequestBytes        int
	RecentWindowSize       int     `json:"recentWindowSize,omitempty"`
	RecentTraceTotal       int     `json:"recentTraceTotal,omitempty"`
	RecentTruncatedCount   int     `json:"recentTruncatedCount,omitempty"`
	RecentOverBudgetCount  int     `json:"recentOverBudgetCount,omitempty"`
	RecentMaxUtilization   float64 `json:"recentMaxContextUtilizationRatio,omitempty"`
	RecentAvgUtilization   float64 `json:"recentAvgContextUtilizationRatio,omitempty"`
	RecentMaxInputTokens   int     `json:"recentMaxInputTokens,omitempty"`
	UtilizationTrend       string  `json:"contextUtilizationTrend,omitempty"`
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
	CacheEstimatedEntries   int
	CacheProviderEntries    int
	ProviderModelBreakdown  []LLMUsageProviderModelBreakdownInfo
}

type LLMUsageProviderModelBreakdownInfo struct {
	ProviderID       string
	Provider         string
	Model            string
	EntryTotal       int
	InputTokens      int
	OutputTokens     int
	TotalTokens      int
	CacheReadTokens  int
	CacheWriteTokens int
	EstimatedEntries int
	FailedEntries    int
}

type EventRecordInfo struct {
	ID           common.EventRecordID
	Version      string
	Topic        string
	OccurredAt   string
	SourceModule string
	SubjectType  string
	SubjectID    string
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	StepID       common.StepID
	Payload      EventRecordPayloadInfo
	Summary      string
	PayloadRef   string
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
	CurrentActionLabel string
	DiagnosticLineage  common.StepID
	Recovery           RecoveryPlanInfo
	RepairLoop         RepairLoopInfo
	AssessmentOutcome  AssessmentOutcomeInfo
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

type RunObservabilityInfo struct {
	Run          RunInfo
	TaskProgress TaskProgressInfo
	RepairLoop   RepairLoopInfo
	Acceptance   AcceptanceProjectionInfo
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
}
