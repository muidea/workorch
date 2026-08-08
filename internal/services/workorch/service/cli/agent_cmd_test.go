package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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
func (b agentCommandFailureBackend) ProjectionEvents() backend.WorkspaceProjectionEventsBackend {
	return nil
}

type agentCommandWorkspaceBackend struct{}

func (agentCommandWorkspaceBackend) Create(context.Context, workorchmanagementcontract.WorkspaceCreateRequest) (*projectioncontract.Workspace, error) {
	return &projectioncontract.Workspace{ID: "ws-agent"}, nil
}
func (agentCommandWorkspaceBackend) Get(context.Context, common.WorkspaceID) (*projectioncontract.Workspace, error) {
	return nil, common.ErrNotFound
}
func (agentCommandWorkspaceBackend) List(context.Context) ([]projectioncontract.Workspace, error) {
	return nil, nil
}
func (agentCommandWorkspaceBackend) Snapshot(context.Context, common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error) {
	return nil, common.ErrNotFound
}

func (agentCommandWorkspaceBackend) RuntimeView(context.Context, workorchquerycontract.WorkspaceRuntimeViewQuery) (*projectioncontract.WorkspaceRuntimeView, error) {
	return nil, nil
}

func (agentCommandWorkspaceBackend) UpdateLLMSelection(context.Context, common.WorkspaceID, *runtimecontract.ProviderInferenceSelection, uint64) (*projectioncontract.Workspace, error) {
	return nil, common.ErrNotFound
}

func (agentCommandWorkspaceBackend) Delete(context.Context, common.WorkspaceID, bool) error {
	return nil
}

type agentCommandGoalBackend struct{ err error }

func (b agentCommandGoalBackend) Define(context.Context, common.WorkspaceID, string) (*workorchmanagementcontract.Goal, error) {
	return nil, b.err
}
func (agentCommandGoalBackend) List(context.Context, common.WorkspaceID) ([]workorchmanagementcontract.Goal, error) {
	return nil, nil
}
func (agentCommandGoalBackend) Get(context.Context, common.WorkspaceID, common.GoalID) (*workorchmanagementcontract.Goal, error) {
	return nil, common.ErrNotFound
}
func (agentCommandGoalBackend) Update(context.Context, common.WorkspaceID, common.GoalID, string) (*workorchmanagementcontract.Goal, error) {
	return nil, common.ErrNotFound
}

type agentCommandDoDBackend struct{}

func (agentCommandDoDBackend) Define(context.Context, common.WorkspaceID, []string) (*workorchmanagementcontract.DoD, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandDoDBackend) List(context.Context, common.WorkspaceID) ([]workorchmanagementcontract.DoD, error) {
	return nil, nil
}
func (agentCommandDoDBackend) Get(context.Context, common.WorkspaceID, common.DoDID) (*workorchmanagementcontract.DoD, error) {
	return nil, common.ErrNotFound
}
func (agentCommandDoDBackend) Update(context.Context, common.WorkspaceID, common.DoDID, []string) (*workorchmanagementcontract.DoD, error) {
	return nil, common.ErrNotFound
}

type agentCommandRunBackend struct{}

func (agentCommandRunBackend) Command(context.Context, backend.WorkspaceRunCommand) (*backend.RunCommandResult, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandRunBackend) Get(context.Context, common.WorkspaceID, common.RunID) (*projectioncontract.Run, error) {
	return nil, common.ErrNotFound
}
func (agentCommandRunBackend) List(context.Context, common.WorkspaceID) ([]projectioncontract.Run, error) {
	return nil, nil
}
func (agentCommandRunBackend) Status(context.Context, common.WorkspaceID, common.RunID) (*projectioncontract.RunStatus, error) {
	return nil, common.ErrNotFound
}

type agentCommandDecisionBackend struct{}

func (agentCommandDecisionBackend) Resolve(context.Context, backend.ResolveDecisionRequest) (*projectioncontract.Decision, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandDecisionBackend) BatchResolve(context.Context, common.WorkspaceID, []common.TicketID, string, workorcheventpayload.Payload) ([]projectioncontract.Decision, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandDecisionBackend) List(context.Context, common.WorkspaceID) ([]projectioncontract.Decision, error) {
	return nil, nil
}
func (agentCommandDecisionBackend) Get(context.Context, common.WorkspaceID, common.TicketID) (*projectioncontract.Decision, error) {
	return nil, common.ErrNotFound
}

type agentCommandApprovalBackend struct{}

func (agentCommandApprovalBackend) Approve(context.Context, common.WorkspaceID, common.ApprovalID, ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandApprovalBackend) Reject(context.Context, common.WorkspaceID, common.ApprovalID, ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandApprovalBackend) BatchResolve(context.Context, common.WorkspaceID, []common.ApprovalID, string, ...workorcheventpayload.Payload) ([]projectioncontract.Approval, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandApprovalBackend) List(context.Context, common.WorkspaceID) ([]projectioncontract.Approval, error) {
	return nil, nil
}
func (agentCommandApprovalBackend) Get(context.Context, common.WorkspaceID, common.ApprovalID) (*projectioncontract.Approval, error) {
	return nil, common.ErrNotFound
}

type agentCommandSoulBackend struct{}

func (agentCommandSoulBackend) Create(context.Context, *workorchmanagementcontract.SoulMutation) (*workorchmanagementcontract.Soul, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandSoulBackend) Get(context.Context, common.SoulID) (*workorchmanagementcontract.Soul, error) {
	return nil, common.ErrNotFound
}
func (agentCommandSoulBackend) List(context.Context) ([]workorchmanagementcontract.Soul, error) {
	return nil, nil
}
func (agentCommandSoulBackend) Update(context.Context, common.SoulID, *workorchmanagementcontract.SoulMutation) (*workorchmanagementcontract.Soul, error) {
	return nil, common.ErrNotFound
}
func (agentCommandSoulBackend) Delete(context.Context, common.SoulID) error { return nil }

type agentCommandSkillBackend struct{}

func (agentCommandSkillBackend) Create(context.Context, *workorchmanagementcontract.SkillMutation) (*workorchmanagementcontract.Skill, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandSkillBackend) Get(context.Context, common.SkillID) (*workorchmanagementcontract.Skill, error) {
	return nil, common.ErrNotFound
}
func (agentCommandSkillBackend) List(context.Context) ([]workorchmanagementcontract.Skill, error) {
	return nil, nil
}

type agentCommandMCPBackend struct{}

func (agentCommandMCPBackend) CreateServer(context.Context, *workorchmanagementcontract.MCPServerMutation) (*workorchmanagementcontract.MCPServer, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandMCPBackend) GetServer(context.Context, common.MCPServerID) (*workorchmanagementcontract.MCPServer, error) {
	return nil, common.ErrNotFound
}
func (agentCommandMCPBackend) ListServers(context.Context) ([]workorchmanagementcontract.MCPServer, error) {
	return nil, nil
}
func (agentCommandMCPBackend) UpdateServer(context.Context, common.MCPServerID, *workorchmanagementcontract.MCPServerMutation) (*workorchmanagementcontract.MCPServer, error) {
	return nil, common.ErrNotFound
}
func (agentCommandMCPBackend) ListCapabilities(context.Context) ([]workorchmanagementcontract.MCPCapability, error) {
	return nil, nil
}

type agentCommandMemoryBackend struct{}

func (agentCommandMemoryBackend) Create(context.Context, common.WorkspaceID, *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandMemoryBackend) Get(context.Context, common.WorkspaceID, common.MemoryID) (*workorchmanagementcontract.MemoryItem, error) {
	return nil, common.ErrNotFound
}
func (agentCommandMemoryBackend) Update(context.Context, common.WorkspaceID, common.MemoryID, *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error) {
	return nil, common.ErrNotFound
}
func (agentCommandMemoryBackend) Delete(context.Context, common.WorkspaceID, common.MemoryID) error {
	return nil
}
func (agentCommandMemoryBackend) List(context.Context, common.WorkspaceID) ([]workorchmanagementcontract.MemoryItem, error) {
	return nil, nil
}
func (agentCommandMemoryBackend) Search(context.Context, workorchquerycontract.MemorySearchQuery) ([]workorchmanagementcontract.MemoryItem, error) {
	return nil, nil
}

type agentCommandTemplateBackend struct{}

func (agentCommandTemplateBackend) Create(context.Context, *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	return nil, common.ErrInvalidArgument
}
func (agentCommandTemplateBackend) Get(context.Context, string) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	return nil, common.ErrNotFound
}
func (agentCommandTemplateBackend) List(context.Context) ([]workorchmanagementcontract.WorkspaceTemplate, error) {
	return nil, nil
}
func (agentCommandTemplateBackend) Update(context.Context, string, *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	return nil, common.ErrNotFound
}
func (agentCommandTemplateBackend) Delete(context.Context, string) error { return nil }

type agentCommandConfigBackend struct{}

func (agentCommandConfigBackend) Get(context.Context) (*backend.ServiceConfig, error) {
	return &backend.ServiceConfig{}, nil
}

func (agentCommandConfigBackend) ListChatModelTargets(context.Context) ([]backend.ChatModelTargetOption, error) {
	return nil, nil
}

func (agentCommandConfigBackend) RefreshChatModelTargets(context.Context) ([]backend.ChatModelTargetOption, error) {
	return nil, nil
}

type agentCommandEventsBackend struct{}

func (agentCommandEventsBackend) Subscribe(context.Context, backend.EventHandler) (func(), error) {
	return func() {}, nil
}
