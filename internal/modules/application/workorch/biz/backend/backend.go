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

type WorkspaceSnapshotInfo struct {
	Workspace          WorkspaceInfo
	Outcome            WorkspaceOutcomeInfo
	Runs               []RunInfo
	Results            []ResultInfo
	Artifacts          []ArtifactInfo
	MemoryRecallTraces []MemoryRecallTraceInfo
	Decisions          []DecisionInfo
	Approvals          []ApprovalInfo
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
	ID          common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Reason      string
	Action      string
	Context     map[string]any
	State       string
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
