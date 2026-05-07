// Package backend defines the shared interface consumed by CLI commands, TUI,
// and agent session logic. Local (EventHub) and remote (HTTP) implementations
// satisfy the same interface so callers never branch on transport.
package backend

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Backend is the shared CLI backend consumed by Cobra commands, the TUI model,
// and the agent session flow. Every interaction with workorch business state
// goes through this interface.
type Backend interface {
	Workspace() WorkspaceBackend
	Goal() GoalBackend
	DoD() DoDBackend
	Run() RunBackend
	Decision() DecisionBackend
	Approval() ApprovalBackend
	Soul() SoulBackend
	Skill() SkillBackend
	MCP() MCPBackend
	Memory() MemoryBackend
	Template() TemplateBackend
	Config() ConfigBackend
	Events() EventsBackend
}

// ── Workspace ──

type CreateWorkspaceRequest struct {
	Title            string
	Description      string
	WorkingDirectory string
	TemplateID       string
}

type WorkspaceInfo struct {
	ID               common.WorkspaceID
	Title            string
	Description      string
	WorkingDirectory string
	State            common.WorkspaceState
	ActiveRunID      common.RunID
	DefaultSoulID    common.SoulID
	SoulBindings     map[string]common.SoulID
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

type ResultSummaryInfo struct {
	GoalOutcome           string
	ImplementationSummary string
	VerificationSummary   string
	EvidenceRefs          []string
	ArtifactRefs          []string
	KnownLimitations      []string
	ResidualRisks         []string
	NextActions           []string
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
	EstimatedCost float64
	CostEstimated bool
	CostSource    string
	Currency      string
	RequestBytes  int
	ResponseBytes int
	Streaming     bool
	Status        string
	FailureClass  string
	OccurredAt    string
	EvidenceRef   string
}

type LLMUsageSummaryInfo struct {
	EntryTotal             int
	InputTokens            int
	OutputTokens           int
	TotalTokens            int
	EstimatedCost          float64
	Currency               string
	EstimatedEntries       int
	ProviderNativeEntries  int
	CostEstimatedEntries   int
	FailedEntries          int
	CostBreakdown          []LLMUsageCostBreakdownInfo
	ProviderModelBreakdown []LLMUsageProviderModelBreakdownInfo
}

type LLMUsageCostBreakdownInfo struct {
	Currency      string
	EstimatedCost float64
	EntryTotal    int
}

type LLMUsageProviderModelBreakdownInfo struct {
	ProviderID       string
	Provider         string
	Model            string
	Currency         string
	EntryTotal       int
	InputTokens      int
	OutputTokens     int
	TotalTokens      int
	EstimatedCost    float64
	EstimatedEntries int
	FailedEntries    int
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

type WorkspaceSnapshotInfo struct {
	Workspace          WorkspaceInfo
	Outcome            WorkspaceOutcomeInfo
	TaskProgress       TaskProgressInfo
	TaskDecompositions []TaskDecompositionInfo
	Runs               []RunInfo
	Results            []ResultInfo
	Artifacts          []ArtifactInfo
	Patches            []PatchInfo
	MemoryRecallTraces []MemoryRecallTraceInfo
	WebSources         []WebSourceTraceInfo
	LLMUsageTraces     []LLMUsageTraceInfo
	LLMUsageSummary    LLMUsageSummaryInfo
	Decisions          []DecisionInfo
	Approvals          []ApprovalInfo
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
	State                string
	AttemptCount         int
	LastAttempt          int
	LastRepairStepID     common.StepID
	LastFailureStepID    common.StepID
	LastFailureClass     string
	LastFailureSignature string
	LastFailureAt        string
	NextAction           string
	Escalated            bool
	EscalationReason     string
	EscalationTicketID   common.TicketID
	EvidenceRefs         []string
}

type TaskProgressInfo struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	State       string
	Summary     string
	Recovery    RecoveryPlanInfo
	RepairLoop  RepairLoopInfo
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

type WorkspaceBackend interface {
	Create(ctx context.Context, req CreateWorkspaceRequest) (*WorkspaceInfo, error)
	Get(ctx context.Context, id common.WorkspaceID) (*WorkspaceInfo, error)
	List(ctx context.Context) ([]WorkspaceInfo, error)
	Delete(ctx context.Context, id common.WorkspaceID, purge bool) error
}

// ── Goal ──

type GoalInfo struct {
	ID          common.GoalID
	WorkspaceID common.WorkspaceID
	Text        string
}

type GoalBackend interface {
	Define(ctx context.Context, wsID common.WorkspaceID, text string) (*GoalInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]GoalInfo, error)
	Get(ctx context.Context, id common.GoalID) (*GoalInfo, error)
	Update(ctx context.Context, id common.GoalID, text string) (*GoalInfo, error)
}

// ── DoD ──

type DoDInfo struct {
	ID          common.DoDID
	WorkspaceID common.WorkspaceID
	Items       []string
	State       string
}

type DoDBackend interface {
	Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*DoDInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]DoDInfo, error)
	Get(ctx context.Context, id common.DoDID) (*DoDInfo, error)
	Update(ctx context.Context, id common.DoDID, items []string) (*DoDInfo, error)
}

// ── Run ──

type RunInfo struct {
	ID                 common.RunID
	WorkspaceID        common.WorkspaceID
	GoalID             common.GoalID
	DoDID              common.DoDID
	ExecutionContext   map[string]any
	State              common.RunState
	Priority           int
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
	FailureReason      string
}

type ActivateRunRequest struct {
	GoalID         common.GoalID
	DoDID          common.DoDID
	LLMProviderID  string
	DialogLanguage string
	SoulID         common.SoulID
	SkillIDs       []common.SkillID
	MCPServerIDs   []common.MCPServerID
	Priority       int
}

type RunStatusInfo struct {
	ID                 common.RunID
	WorkspaceID        common.WorkspaceID
	State              common.RunState
	CurrentStepKind    string
	StepCount          int
	DecisionCount      int
	ApprovalCount      int
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
	FailureReason      string
}

type RunBackend interface {
	Activate(ctx context.Context, wsID common.WorkspaceID, req ActivateRunRequest) (*RunInfo, error)
	Resume(ctx context.Context, id common.RunID) (*RunInfo, error)
	Get(ctx context.Context, id common.RunID) (*RunInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]RunInfo, error)
	Status(ctx context.Context, id common.RunID) (*RunStatusInfo, error)
}

// ── Decision ──

type DecisionInfo struct {
	ID             common.TicketID
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	Reason         string
	Question       string
	State          string
	ResolutionType string
	AutoGovernance *AutoGovernanceSummaryInfo
}

type ResolveDecisionRequest struct {
	TicketID       common.TicketID
	ResolutionType string
	Payload        map[string]any
}

type DecisionBackend interface {
	Resolve(ctx context.Context, req ResolveDecisionRequest) (*DecisionInfo, error)
	BatchResolve(ctx context.Context, ticketIDs []common.TicketID, resolutionType string, payload map[string]any) ([]DecisionInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]DecisionInfo, error)
	Get(ctx context.Context, id common.TicketID) (*DecisionInfo, error)
}

// ── Approval ──

type ApprovalInfo struct {
	ID             common.ApprovalID
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	Reason         string
	Action         string
	Context        map[string]any
	State          string
	AutoGovernance *AutoGovernanceSummaryInfo
}

type AutoGovernanceSummaryInfo struct {
	TraceID           string
	Action            string
	Status            string
	RiskLevel         string
	PolicyMatched     string
	PolicyBlocked     string
	Confidence        float64
	Reason            string
	EvidenceRefs      []string
	PromptFingerprint string
	OccurredAt        string
}

type ApprovalBackend interface {
	Approve(ctx context.Context, id common.ApprovalID) (*ApprovalInfo, error)
	Reject(ctx context.Context, id common.ApprovalID) (*ApprovalInfo, error)
	BatchResolve(ctx context.Context, ids []common.ApprovalID, action string) ([]ApprovalInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]ApprovalInfo, error)
	Get(ctx context.Context, id common.ApprovalID) (*ApprovalInfo, error)
}

// ── Soul ──

type SoulInfo struct {
	ID              common.SoulID
	Name            string
	Description     string
	BusinessDomains []string
	SoulMD          string
}

type SoulBackend interface {
	Create(ctx context.Context, s *SoulInfo) (*SoulInfo, error)
	Get(ctx context.Context, id common.SoulID) (*SoulInfo, error)
	List(ctx context.Context) ([]SoulInfo, error)
	Update(ctx context.Context, id common.SoulID, s *SoulInfo) (*SoulInfo, error)
	Delete(ctx context.Context, id common.SoulID) error
}

// ── Skill ──

type SkillInfo struct {
	ID          common.SkillID
	Name        string
	Version     string
	Description string
	Instruction string
}

type SkillBackend interface {
	Create(ctx context.Context, s *SkillInfo) (*SkillInfo, error)
	Get(ctx context.Context, id common.SkillID) (*SkillInfo, error)
	List(ctx context.Context) ([]SkillInfo, error)
}

// ── MCP ──

type MCPServerInfo struct {
	ID        common.MCPServerID
	Name      string
	Transport string
	Endpoint  string
	Enabled   bool
}

type MCPCapabilityInfo struct {
	ID          common.MCPCapabilityID
	ServerID    common.MCPServerID
	Kind        string
	Name        string
	DisplayName string
	Description string
}

type MCPBackend interface {
	CreateServer(ctx context.Context, s *MCPServerInfo) (*MCPServerInfo, error)
	GetServer(ctx context.Context, id common.MCPServerID) (*MCPServerInfo, error)
	ListServers(ctx context.Context) ([]MCPServerInfo, error)
	UpdateServer(ctx context.Context, id common.MCPServerID, s *MCPServerInfo) (*MCPServerInfo, error)
	ListCapabilities(ctx context.Context) ([]MCPCapabilityInfo, error)
}

// ── Memory ──

type MemoryItemInfo struct {
	ID                  common.MemoryID
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	SourceType          string
	SourceRef           string
	MemoryType          string
	Content             string
	Summary             string
	Tags                []string
	Importance          int
	Confidence          int
	Visibility          string
	EmbeddingProviderID string
	EmbeddingModel      string
	EmbeddingText       string
	VectorReady         bool
	VectorDimension     int
	VectorScore         float64
	Vector              []float64
}

type MemorySearchRequest struct {
	Query string
	Tags  []string
	Types []string
	Limit int
}

type MemoryBackend interface {
	Create(ctx context.Context, wsID common.WorkspaceID, m *MemoryItemInfo) (*MemoryItemInfo, error)
	Get(ctx context.Context, id common.MemoryID) (*MemoryItemInfo, error)
	Update(ctx context.Context, id common.MemoryID, m *MemoryItemInfo) (*MemoryItemInfo, error)
	Delete(ctx context.Context, id common.MemoryID) error
	List(ctx context.Context, wsID common.WorkspaceID) ([]MemoryItemInfo, error)
	Search(ctx context.Context, wsID common.WorkspaceID, req MemorySearchRequest) ([]MemoryItemInfo, error)
}

// ── Template ──

type TemplateInfo struct {
	ID            string
	Name          string
	Description   string
	GoalText      string
	DoDItems      []string
	DefaultSoulID common.SoulID
	SoulBindings  map[string]string
}

type TemplateBackend interface {
	Create(ctx context.Context, t *TemplateInfo) (*TemplateInfo, error)
	Get(ctx context.Context, id string) (*TemplateInfo, error)
	List(ctx context.Context) ([]TemplateInfo, error)
	Update(ctx context.Context, id string, t *TemplateInfo) (*TemplateInfo, error)
	Delete(ctx context.Context, id string) error
}

// ── Config ──

type LLMProvider struct {
	ID        string
	Provider  string
	BaseURL   string
	Model     string
	Timeout   string
	Disabled  bool
	Streaming bool
}

type ServiceConfig struct {
	ServerAddr        string
	Backend           string
	DataDir           string
	DefaultProvider   string
	MaxConcurrentRuns int64
	Providers         []LLMProvider
}

type ConfigBackend interface {
	Get(ctx context.Context) (*ServiceConfig, error)
}

// ── Events ──

// EventType classifies backend-published events for the TUI/agent.
type EventType string

const (
	EventWorkspaceCreated      EventType = "workspace/created"
	EventWorkspaceStateChanged EventType = "workspace/state/changed"
	EventWorkspaceDeleted      EventType = "workspace/deleted"
	EventGoalDefined           EventType = "goal/defined"
	EventDoDDefined            EventType = "dod/defined"
	EventRunStateChanged       EventType = "run/state/changed"
	EventStepDispatched        EventType = "step/dispatched"
	EventStepCompleted         EventType = "step/completed"
	EventDecisionCreated       EventType = "decision/created"
	EventDecisionResolved      EventType = "decision/resolved"
	EventApprovalCreated       EventType = "approval/created"
	EventApprovalResolved      EventType = "approval/resolved"
	EventResultAccepted        EventType = "result/accepted"
	EventMemoryCreated         EventType = "memory/created"
	EventMemoryUpdated         EventType = "memory/updated"
	EventMemoryDeleted         EventType = "memory/deleted"
	EventMemoryRecallRecorded  EventType = "memory/recall/recorded"
	EventLLMUsageRecorded      EventType = "llm/usage/recorded"
	EventPatchPreviewed        EventType = "patch/previewed"
	EventPatchApplied          EventType = "patch/applied"
	EventPatchFailed           EventType = "patch/failed"
	EventPatchReverted         EventType = "patch/reverted"
	EventGovernanceEvaluated   EventType = "governance/auto/evaluated"
	EventGovernanceResolved    EventType = "governance/auto/resolved"
	EventGovernanceEscalated   EventType = "governance/auto/escalated"
	EventGovernanceBlocked     EventType = "governance/auto/blocked"
)

type Event struct {
	Type        EventType
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Payload     map[string]any
}

type EventHandler func(ctx context.Context, evt Event)

type EventsBackend interface {
	Subscribe(ctx context.Context, handler EventHandler) (unsubscribe func(), err error)
}

// Kind returns the canonical resource kind from a run's initial step.
func RunStepKind() string { return "llm/workspace_execution" }
