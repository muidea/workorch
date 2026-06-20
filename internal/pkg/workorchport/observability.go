package workorchport

import "github.com/muidea/workorch/internal/pkg/common"

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
	ContextBudget map[string]any
	CacheUsage    map[string]any
	Status        string
	FailureClass  string
	OccurredAt    string
	EvidenceRef   string
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
	Payload        map[string]any
	PayloadSummary map[string]any
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
	StructuredFailureSignals map[string]any
	RepairInputCompaction    map[string]any
	LastRepairInputBytes     int
	RepeatedFailureReason    string
	NextAction               string
	Escalated                bool
	EscalationReason         string
	EscalationTicketID       common.TicketID
	EvidenceRefs             []string
}

type TaskProgressInfo struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	State       string
	Summary     string
	Recovery    RecoveryPlanInfo
	RepairLoop  RepairLoopInfo
}

type RunObservabilityInfo struct {
	Run          RunInfo
	TaskProgress TaskProgressInfo
	RepairLoop   RepairLoopInfo
	Acceptance   AcceptanceProjectionInfo
}

type TaskDecompositionNodeInfo struct {
	ID           string
	Kind         string
	Title        string
	State        string
	DependsOn    []string
	Branch       string
	StepID       common.StepID
	StepState    string
	EvidenceRefs []string
	Input        map[string]any
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
