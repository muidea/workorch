package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

func TestRunAgentReturnsBackendErrorBeforeEvents(t *testing.T) {
	previousBackend := cliBackend
	previousWorkspaceID := defaultWorkspaceID
	t.Cleanup(func() {
		cliBackend = previousBackend
		defaultWorkspaceID = previousWorkspaceID
	})

	cliBackend = agentCommandFailureBackend{goalErr: errors.New("define goal failed")}
	defaultWorkspaceID = "ws-agent"

	err := runAgent(nil, []string{"do", "work"})
	if err == nil || !strings.Contains(err.Error(), "define goal failed") {
		t.Fatalf("runAgent error = %v, want define goal failure", err)
	}
}

func TestRunAgentRequiresSelectedWorkspace(t *testing.T) {
	previousBackend := cliBackend
	previousWorkspaceID := defaultWorkspaceID
	t.Cleanup(func() {
		cliBackend = previousBackend
		defaultWorkspaceID = previousWorkspaceID
	})

	cliBackend = agentCommandFailureBackend{}
	defaultWorkspaceID = ""

	err := runAgent(nil, []string{"do", "work"})
	if err == nil || !strings.Contains(err.Error(), "workspace is required") {
		t.Fatalf("runAgent error = %v, want workspace required", err)
	}
}

type agentCommandFailureBackend struct {
	goalErr error
}

func (b agentCommandFailureBackend) Workspace() backend.WorkspaceBackend {
	return agentCommandWorkspaceBackend{}
}
func (b agentCommandFailureBackend) Goal() backend.GoalBackend {
	return agentCommandGoalBackend{err: b.goalErr}
}
func (b agentCommandFailureBackend) DoD() backend.DoDBackend { return agentCommandDoDBackend{} }
func (b agentCommandFailureBackend) Run() backend.RunBackend { return agentCommandRunBackend{} }
func (b agentCommandFailureBackend) Decision() backend.DecisionBackend {
	return agentCommandDecisionBackend{}
}
func (b agentCommandFailureBackend) Approval() backend.ApprovalBackend {
	return agentCommandApprovalBackend{}
}
func (b agentCommandFailureBackend) Soul() backend.SoulBackend   { return agentCommandSoulBackend{} }
func (b agentCommandFailureBackend) Skill() backend.SkillBackend { return agentCommandSkillBackend{} }
func (b agentCommandFailureBackend) MCP() backend.MCPBackend     { return agentCommandMCPBackend{} }
func (b agentCommandFailureBackend) Memory() backend.MemoryBackend {
	return agentCommandMemoryBackend{}
}
func (b agentCommandFailureBackend) Template() backend.TemplateBackend {
	return agentCommandTemplateBackend{}
}
func (b agentCommandFailureBackend) Config() backend.ConfigBackend {
	return agentCommandConfigBackend{}
}
func (b agentCommandFailureBackend) Events() backend.EventsBackend {
	return agentCommandEventsBackend{}
}

type agentCommandWorkspaceBackend struct{}

func (agentCommandWorkspaceBackend) Create(context.Context, backend.CreateWorkspaceRequest) (*backend.WorkspaceInfo, error) {
	return &backend.WorkspaceInfo{ID: "ws-agent"}, nil
}
func (agentCommandWorkspaceBackend) Get(context.Context, common.WorkspaceID) (*backend.WorkspaceInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandWorkspaceBackend) List(context.Context) ([]backend.WorkspaceInfo, error) {
	return nil, nil
}
func (agentCommandWorkspaceBackend) Snapshot(context.Context, common.WorkspaceID) (*backend.WorkspaceSnapshotInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandWorkspaceBackend) Delete(context.Context, common.WorkspaceID, bool) error {
	return nil
}

type agentCommandGoalBackend struct{ err error }

func (b agentCommandGoalBackend) Define(context.Context, common.WorkspaceID, string) (*backend.GoalInfo, error) {
	return nil, b.err
}
func (agentCommandGoalBackend) List(context.Context, common.WorkspaceID) ([]backend.GoalInfo, error) {
	return nil, nil
}
func (agentCommandGoalBackend) Get(context.Context, common.WorkspaceID, common.GoalID) (*backend.GoalInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandGoalBackend) Update(context.Context, common.WorkspaceID, common.GoalID, string) (*backend.GoalInfo, error) {
	return nil, common.ErrNotFound
}

type agentCommandDoDBackend struct{}

func (agentCommandDoDBackend) Define(context.Context, common.WorkspaceID, []string) (*backend.DoDInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandDoDBackend) List(context.Context, common.WorkspaceID) ([]backend.DoDInfo, error) {
	return nil, nil
}
func (agentCommandDoDBackend) Get(context.Context, common.WorkspaceID, common.DoDID) (*backend.DoDInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandDoDBackend) Update(context.Context, common.WorkspaceID, common.DoDID, []string) (*backend.DoDInfo, error) {
	return nil, common.ErrNotFound
}

type agentCommandRunBackend struct{}

func (agentCommandRunBackend) Activate(context.Context, common.WorkspaceID, backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandRunBackend) Restart(context.Context, common.WorkspaceID, backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandRunBackend) Resume(context.Context, common.WorkspaceID, common.RunID, ...map[string]any) (*backend.RunInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandRunBackend) Get(context.Context, common.WorkspaceID, common.RunID) (*backend.RunInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandRunBackend) List(context.Context, common.WorkspaceID) ([]backend.RunInfo, error) {
	return nil, nil
}
func (agentCommandRunBackend) Status(context.Context, common.WorkspaceID, common.RunID) (*backend.RunStatusInfo, error) {
	return nil, common.ErrNotFound
}

type agentCommandDecisionBackend struct{}

func (agentCommandDecisionBackend) Resolve(context.Context, backend.ResolveDecisionRequest) (*backend.DecisionInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandDecisionBackend) BatchResolve(context.Context, common.WorkspaceID, []common.TicketID, string, map[string]any) ([]backend.DecisionInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandDecisionBackend) List(context.Context, common.WorkspaceID) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (agentCommandDecisionBackend) Get(context.Context, common.WorkspaceID, common.TicketID) (*backend.DecisionInfo, error) {
	return nil, common.ErrNotFound
}

type agentCommandApprovalBackend struct{}

func (agentCommandApprovalBackend) Approve(context.Context, common.WorkspaceID, common.ApprovalID) (*backend.ApprovalInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandApprovalBackend) Reject(context.Context, common.WorkspaceID, common.ApprovalID) (*backend.ApprovalInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandApprovalBackend) BatchResolve(context.Context, common.WorkspaceID, []common.ApprovalID, string) ([]backend.ApprovalInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandApprovalBackend) List(context.Context, common.WorkspaceID) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (agentCommandApprovalBackend) Get(context.Context, common.WorkspaceID, common.ApprovalID) (*backend.ApprovalInfo, error) {
	return nil, common.ErrNotFound
}

type agentCommandSoulBackend struct{}

func (agentCommandSoulBackend) Create(context.Context, *backend.SoulInfo) (*backend.SoulInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandSoulBackend) Get(context.Context, common.SoulID) (*backend.SoulInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandSoulBackend) List(context.Context) ([]backend.SoulInfo, error) { return nil, nil }
func (agentCommandSoulBackend) Update(context.Context, common.SoulID, *backend.SoulInfo) (*backend.SoulInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandSoulBackend) Delete(context.Context, common.SoulID) error { return nil }

type agentCommandSkillBackend struct{}

func (agentCommandSkillBackend) Create(context.Context, *backend.SkillInfo) (*backend.SkillInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandSkillBackend) Get(context.Context, common.SkillID) (*backend.SkillInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandSkillBackend) List(context.Context) ([]backend.SkillInfo, error) { return nil, nil }

type agentCommandMCPBackend struct{}

func (agentCommandMCPBackend) CreateServer(context.Context, *backend.MCPServerInfo) (*backend.MCPServerInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandMCPBackend) GetServer(context.Context, common.MCPServerID) (*backend.MCPServerInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandMCPBackend) ListServers(context.Context) ([]backend.MCPServerInfo, error) {
	return nil, nil
}
func (agentCommandMCPBackend) UpdateServer(context.Context, common.MCPServerID, *backend.MCPServerInfo) (*backend.MCPServerInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandMCPBackend) ListCapabilities(context.Context) ([]backend.MCPCapabilityInfo, error) {
	return nil, nil
}

type agentCommandMemoryBackend struct{}

func (agentCommandMemoryBackend) Create(context.Context, common.WorkspaceID, *backend.MemoryItemInfo) (*backend.MemoryItemInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandMemoryBackend) Get(context.Context, common.WorkspaceID, common.MemoryID) (*backend.MemoryItemInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandMemoryBackend) Update(context.Context, common.WorkspaceID, common.MemoryID, *backend.MemoryItemInfo) (*backend.MemoryItemInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandMemoryBackend) Delete(context.Context, common.WorkspaceID, common.MemoryID) error {
	return nil
}
func (agentCommandMemoryBackend) List(context.Context, common.WorkspaceID) ([]backend.MemoryItemInfo, error) {
	return nil, nil
}
func (agentCommandMemoryBackend) Search(context.Context, common.WorkspaceID, backend.MemorySearchRequest) ([]backend.MemoryItemInfo, error) {
	return nil, nil
}

type agentCommandTemplateBackend struct{}

func (agentCommandTemplateBackend) Create(context.Context, *backend.TemplateInfo) (*backend.TemplateInfo, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandTemplateBackend) Get(context.Context, string) (*backend.TemplateInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandTemplateBackend) List(context.Context) ([]backend.TemplateInfo, error) {
	return nil, nil
}
func (agentCommandTemplateBackend) Update(context.Context, string, *backend.TemplateInfo) (*backend.TemplateInfo, error) {
	return nil, common.ErrNotFound
}
func (agentCommandTemplateBackend) Delete(context.Context, string) error { return nil }

type agentCommandConfigBackend struct{}

func (agentCommandConfigBackend) Get(context.Context) (*backend.ServiceConfig, error) {
	return &backend.ServiceConfig{}, nil
}

type agentCommandEventsBackend struct{}

func (agentCommandEventsBackend) Subscribe(context.Context, backend.EventHandler) (func(), error) {
	return func() {}, nil
}
