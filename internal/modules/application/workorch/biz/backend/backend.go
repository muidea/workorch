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
	ID          common.MemoryID
	WorkspaceID common.WorkspaceID
	MemoryType  string
	Content     string
	Summary     string
}

type MemoryBackend interface {
	Create(ctx context.Context, wsID common.WorkspaceID, m *MemoryItemInfo) (*MemoryItemInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]MemoryItemInfo, error)
}

// ── Template ──

type TemplateInfo struct {
	ID            string
	Name          string
	Description   string
	GoalText      string
	DoDItems      []string
	DefaultSoulID common.SoulID
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
	EventWorkspaceStateChanged EventType = "workspace/state/changed"
	EventRunStateChanged       EventType = "run/state/changed"
	EventStepDispatched        EventType = "step/dispatched"
	EventStepCompleted         EventType = "step/completed"
	EventDecisionCreated       EventType = "decision/created"
	EventDecisionResolved      EventType = "decision/resolved"
	EventApprovalCreated       EventType = "approval/created"
	EventApprovalResolved      EventType = "approval/resolved"
	EventResultAccepted        EventType = "result/accepted"
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
