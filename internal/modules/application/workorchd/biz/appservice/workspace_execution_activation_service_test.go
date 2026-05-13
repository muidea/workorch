package appservice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	mcpmodule "github.com/muidea/workorch/internal/modules/kernel/mcp/service"
	memorycommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	memorymodule "github.com/muidea/workorch/internal/modules/kernel/memory/service"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type workspaceScopedDecisionTicketRepository struct {
	items                 []*decisionmodel.Ticket
	listCalled            bool
	listByWorkspaceCalled bool
}

func (r *workspaceScopedDecisionTicketRepository) Create(context.Context, *decisionmodel.Ticket) error {
	return common.ErrInvalidArgument
}

func (r *workspaceScopedDecisionTicketRepository) Get(context.Context, common.TicketID) (*decisionmodel.Ticket, error) {
	return nil, common.ErrNotFound
}

func (r *workspaceScopedDecisionTicketRepository) List(context.Context) ([]*decisionmodel.Ticket, error) {
	r.listCalled = true
	return r.items, nil
}

func (r *workspaceScopedDecisionTicketRepository) ListByRunID(context.Context, common.RunID) ([]*decisionmodel.Ticket, error) {
	return nil, nil
}

func (r *workspaceScopedDecisionTicketRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	r.listByWorkspaceCalled = true
	ret := make([]*decisionmodel.Ticket, 0, len(r.items))
	for _, item := range r.items {
		if item.WorkspaceID == workspaceID {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

func (r *workspaceScopedDecisionTicketRepository) Update(context.Context, *decisionmodel.Ticket) error {
	return common.ErrInvalidArgument
}

func (r *workspaceScopedDecisionTicketRepository) Delete(context.Context, common.TicketID) error {
	return common.ErrInvalidArgument
}

type staticDecisionResolutionRepository map[common.TicketID]*decisionmodel.Resolution

func (r staticDecisionResolutionRepository) Create(context.Context, *decisionmodel.Resolution) error {
	return common.ErrInvalidArgument
}

func (r staticDecisionResolutionRepository) GetByTicketID(_ context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	item, ok := r[ticketID]
	if !ok {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r staticDecisionResolutionRepository) DeleteByTicketID(context.Context, common.TicketID) error {
	return common.ErrInvalidArgument
}

type staticRunRepository map[common.RunID]*runmodel.Run

func (r staticRunRepository) Create(context.Context, *runmodel.Run) error {
	return common.ErrInvalidArgument
}

func (r staticRunRepository) Get(_ context.Context, runID common.RunID) (*runmodel.Run, error) {
	item, ok := r[runID]
	if !ok {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r staticRunRepository) List(context.Context) ([]*runmodel.Run, error) {
	return nil, nil
}

func (r staticRunRepository) ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*runmodel.Run, error) {
	return nil, nil
}

func (r staticRunRepository) Update(context.Context, *runmodel.Run) error {
	return common.ErrInvalidArgument
}

func (r staticRunRepository) Delete(context.Context, common.RunID) error {
	return common.ErrInvalidArgument
}

type countingRunRepository struct {
	base        repository.RunRepository
	createRuns  []*runmodel.Run
	getCalls    int
	updateCalls int
}

type countingRunService struct {
	runmodule.Service
	createCommands []runmodule.CreateRunCommand
}

func (s *countingRunService) CreateRun(ctx context.Context, cmd runmodule.CreateRunCommand) error {
	s.createCommands = append(s.createCommands, runmodule.CreateRunCommand{
		RunID:            cmd.RunID,
		WorkspaceID:      cmd.WorkspaceID,
		GoalID:           cmd.GoalID,
		DoDID:            cmd.DoDID,
		InitialStepKind:  cmd.InitialStepKind,
		ExecutionContext: cloneContextMap(cmd.ExecutionContext),
		Priority:         cmd.Priority,
	})
	return s.Service.CreateRun(ctx, cmd)
}

func (r *countingRunRepository) Create(ctx context.Context, item *runmodel.Run) error {
	if item != nil {
		r.createRuns = append(r.createRuns, &runmodel.Run{
			ID:                 item.ID,
			WorkspaceID:        item.WorkspaceID,
			GoalID:             item.GoalID,
			DoDID:              item.DoDID,
			InitialStepKind:    item.InitialStepKind,
			ExecutionContext:   cloneContextMap(item.ExecutionContext),
			State:              item.State,
			Priority:           item.Priority,
			BlockingTicketID:   item.BlockingTicketID,
			BlockingApprovalID: item.BlockingApprovalID,
			FailureReason:      item.FailureReason,
		})
	}
	return r.base.Create(ctx, item)
}

func (r *countingRunRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	r.getCalls++
	return r.base.Get(ctx, id)
}

func (r *countingRunRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	return r.base.List(ctx)
}

func (r *countingRunRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	return r.base.ListByWorkspaceID(ctx, workspaceID)
}

func (r *countingRunRepository) Update(ctx context.Context, item *runmodel.Run) error {
	r.updateCalls++
	return r.base.Update(ctx, item)
}

func (r *countingRunRepository) Delete(ctx context.Context, id common.RunID) error {
	return r.base.Delete(ctx, id)
}

type countingWorkspaceRepository struct {
	base     repository.WorkspaceRepository
	getCalls int
}

func (r *countingWorkspaceRepository) Create(ctx context.Context, item *workspacemodel.Workspace) error {
	return r.base.Create(ctx, item)
}

func (r *countingWorkspaceRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	r.getCalls++
	return r.base.Get(ctx, id)
}

func (r *countingWorkspaceRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	return r.base.List(ctx)
}

func (r *countingWorkspaceRepository) Update(ctx context.Context, item *workspacemodel.Workspace) error {
	return r.base.Update(ctx, item)
}

func (r *countingWorkspaceRepository) Delete(ctx context.Context, id common.WorkspaceID) error {
	return r.base.Delete(ctx, id)
}

var _ repository.DecisionTicketRepository = (*workspaceScopedDecisionTicketRepository)(nil)
var _ repository.DecisionResolutionRepository = staticDecisionResolutionRepository{}
var _ repository.RunRepository = staticRunRepository{}
var _ repository.RunRepository = (*countingRunRepository)(nil)
var _ repository.WorkspaceRepository = (*countingWorkspaceRepository)(nil)

type blockingSoulRepository struct {
	base    repository.SoulRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingSoulRepository) Create(ctx context.Context, item *soulmodel.Soul) error {
	return r.base.Create(ctx, item)
}

func (r *blockingSoulRepository) Get(ctx context.Context, id common.SoulID) (*soulmodel.Soul, error) {
	if r.started != nil {
		r.started <- "soul"
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return r.base.Get(ctx, id)
}

func (r *blockingSoulRepository) List(ctx context.Context) ([]*soulmodel.Soul, error) {
	return r.base.List(ctx)
}

func (r *blockingSoulRepository) Update(ctx context.Context, item *soulmodel.Soul) error {
	return r.base.Update(ctx, item)
}

func (r *blockingSoulRepository) Delete(ctx context.Context, id common.SoulID) error {
	return r.base.Delete(ctx, id)
}

type blockingMemoryService struct {
	started chan<- string
	release <-chan struct{}
}

func (s *blockingMemoryService) CreateMemoryItem(context.Context, memorymodule.CreateMemoryItemCommand) error {
	return common.ErrInvalidArgument
}

func (s *blockingMemoryService) UpdateMemoryItem(context.Context, memorymodule.UpdateMemoryItemCommand) error {
	return common.ErrInvalidArgument
}

func (s *blockingMemoryService) GetMemoryItem(context.Context, common.MemoryID) (*memorymodel.MemoryItem, error) {
	return nil, common.ErrNotFound
}

func (s *blockingMemoryService) DeleteMemoryItem(context.Context, common.MemoryID) error {
	return common.ErrInvalidArgument
}

func (s *blockingMemoryService) ListMemoryByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	if s.started != nil {
		s.started <- "memory"
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, nil
}

func (s *blockingMemoryService) SearchMemory(context.Context, repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	return nil, nil
}

type activationMemorySelectionService struct {
	items []*memorymodel.MemoryItem
}

func (s *activationMemorySelectionService) GetMemoryItem(_ context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	for _, item := range s.items {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, common.ErrNotFound
}

func (s *activationMemorySelectionService) ListMemoryByWorkspace(_ context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	ret := make([]*memorymodel.MemoryItem, 0, len(s.items))
	for _, item := range s.items {
		if item != nil && item.WorkspaceID == workspaceID {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

func (s *activationMemorySelectionService) SearchMemory(context.Context, repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	return nil, nil
}

type blockingDecisionTicketRepository struct {
	base    repository.DecisionTicketRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingDecisionTicketRepository) Create(ctx context.Context, item *decisionmodel.Ticket) error {
	return r.base.Create(ctx, item)
}

func (r *blockingDecisionTicketRepository) Get(ctx context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	return r.base.Get(ctx, id)
}

func (r *blockingDecisionTicketRepository) List(ctx context.Context) ([]*decisionmodel.Ticket, error) {
	return r.base.List(ctx)
}

func (r *blockingDecisionTicketRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	return r.base.ListByRunID(ctx, runID)
}

func (r *blockingDecisionTicketRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	if r.started != nil {
		r.started <- "governance"
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return r.base.ListByWorkspaceID(ctx, workspaceID)
}

func (r *blockingDecisionTicketRepository) Update(ctx context.Context, item *decisionmodel.Ticket) error {
	return r.base.Update(ctx, item)
}

func (r *blockingDecisionTicketRepository) Delete(ctx context.Context, id common.TicketID) error {
	return r.base.Delete(ctx, id)
}

var _ repository.SoulRepository = (*blockingSoulRepository)(nil)
var _ repository.DecisionTicketRepository = (*blockingDecisionTicketRepository)(nil)

type blockingMCPService struct {
	base    mcpmodule.Service
	started chan<- string
	release <-chan struct{}
}

func (s *blockingMCPService) CreateServer(ctx context.Context, cmd mcpmodule.CreateServerCommand) error {
	return s.base.CreateServer(ctx, cmd)
}

func (s *blockingMCPService) UpdateServer(ctx context.Context, cmd mcpmodule.UpdateServerCommand) error {
	return s.base.UpdateServer(ctx, cmd)
}

func (s *blockingMCPService) GetServer(ctx context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if s.started != nil {
		s.started <- "mcp-server"
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.base.GetServer(ctx, id)
}

func (s *blockingMCPService) ListServers(ctx context.Context) ([]*mcpmodel.Server, error) {
	return s.base.ListServers(ctx)
}

func (s *blockingMCPService) CreateCapability(ctx context.Context, cmd mcpmodule.CreateCapabilityCommand) (*mcpmodel.Capability, error) {
	return s.base.CreateCapability(ctx, cmd)
}

func (s *blockingMCPService) UpdateCapability(ctx context.Context, cmd mcpmodule.UpdateCapabilityCommand) (*mcpmodel.Capability, error) {
	return s.base.UpdateCapability(ctx, cmd)
}

func (s *blockingMCPService) GetCapability(ctx context.Context, id common.MCPCapabilityID) (*mcpmodel.Capability, error) {
	if s.started != nil {
		s.started <- "mcp-capability"
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.base.GetCapability(ctx, id)
}

func (s *blockingMCPService) ListCapabilities(ctx context.Context, query mcpmodule.CapabilityQuery) ([]*mcpmodel.Capability, error) {
	return s.base.ListCapabilities(ctx, query)
}

var _ mcpmodule.Service = (*blockingMCPService)(nil)

type countingGoalRepository struct {
	base      repository.GoalRepository
	getCalls  int
	listCalls int
}

func (r *countingGoalRepository) Create(ctx context.Context, item *workspacemodel.Goal) error {
	return r.base.Create(ctx, item)
}

func (r *countingGoalRepository) Get(ctx context.Context, id common.GoalID) (*workspacemodel.Goal, error) {
	r.getCalls++
	return r.base.Get(ctx, id)
}

func (r *countingGoalRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	r.listCalls++
	return r.base.ListByWorkspaceID(ctx, workspaceID)
}

func (r *countingGoalRepository) Update(ctx context.Context, item *workspacemodel.Goal) error {
	return r.base.Update(ctx, item)
}

func (r *countingGoalRepository) Delete(ctx context.Context, id common.GoalID) error {
	return r.base.Delete(ctx, id)
}

type countingDoDRepository struct {
	base      repository.DoDRepository
	getCalls  int
	listCalls int
}

func (r *countingDoDRepository) Create(ctx context.Context, item *workspacemodel.DoD) error {
	return r.base.Create(ctx, item)
}

func (r *countingDoDRepository) Get(ctx context.Context, id common.DoDID) (*workspacemodel.DoD, error) {
	r.getCalls++
	return r.base.Get(ctx, id)
}

func (r *countingDoDRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	r.listCalls++
	return r.base.ListByWorkspaceID(ctx, workspaceID)
}

func (r *countingDoDRepository) Update(ctx context.Context, item *workspacemodel.DoD) error {
	return r.base.Update(ctx, item)
}

func (r *countingDoDRepository) Delete(ctx context.Context, id common.DoDID) error {
	return r.base.Delete(ctx, id)
}

var _ repository.GoalRepository = (*countingGoalRepository)(nil)
var _ repository.DoDRepository = (*countingDoDRepository)(nil)

type blockingGoalRepository struct {
	base    repository.GoalRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingGoalRepository) Create(ctx context.Context, item *workspacemodel.Goal) error {
	return r.base.Create(ctx, item)
}

func (r *blockingGoalRepository) Get(ctx context.Context, id common.GoalID) (*workspacemodel.Goal, error) {
	return r.base.Get(ctx, id)
}

func (r *blockingGoalRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	if r.started != nil {
		r.started <- "goal"
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return r.base.ListByWorkspaceID(ctx, workspaceID)
}

func (r *blockingGoalRepository) Update(ctx context.Context, item *workspacemodel.Goal) error {
	return r.base.Update(ctx, item)
}

func (r *blockingGoalRepository) Delete(ctx context.Context, id common.GoalID) error {
	return r.base.Delete(ctx, id)
}

type blockingDoDRepository struct {
	base    repository.DoDRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingDoDRepository) Create(ctx context.Context, item *workspacemodel.DoD) error {
	return r.base.Create(ctx, item)
}

func (r *blockingDoDRepository) Get(ctx context.Context, id common.DoDID) (*workspacemodel.DoD, error) {
	return r.base.Get(ctx, id)
}

func (r *blockingDoDRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	if r.started != nil {
		r.started <- "dod"
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return r.base.ListByWorkspaceID(ctx, workspaceID)
}

func (r *blockingDoDRepository) Update(ctx context.Context, item *workspacemodel.DoD) error {
	return r.base.Update(ctx, item)
}

func (r *blockingDoDRepository) Delete(ctx context.Context, id common.DoDID) error {
	return r.base.Delete(ctx, id)
}

var _ repository.GoalRepository = (*blockingGoalRepository)(nil)
var _ repository.DoDRepository = (*blockingDoDRepository)(nil)

func TestActivateWorkspaceExecutionWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ActivateWorkspaceExecutionWithSession(context.Background(), nil, "ws-activation", ActivateWorkspaceExecutionCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestActivateWorkspaceExecutionWithBackendCreatesAndStartsRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-start", "activation start workspace", "", "workspaces/activation-start")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "activation goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if !strings.HasPrefix(item.Run.ID, "run-") ||
		item.Run.WorkspaceID != workspace.ID ||
		item.Run.GoalID != goal.ID ||
		item.Run.DoDID != dod.ID ||
		item.Run.State != "blocked_on_decision" ||
		item.Run.BlockingTicketID == "" {
		t.Fatalf("activation run = %+v, want generated run blocked on provider decision with goal/dod", item.Run)
	}
	if item.Snapshot.Workspace.ID != workspace.ID || len(item.Snapshot.Runs) != 1 {
		t.Fatalf("activation snapshot = %+v, want workspace with one run", item.Snapshot)
	}
	compressed := runCompressedContext(t, item.Run)
	if compressed["version"] != "workorch.compressedContext.v1" || compressed["mode"] != "deterministic_recovery_summary" {
		t.Fatalf("compressedContext = %+v, want deterministic recovery summary", compressed)
	}
	if compressed["runId"] != item.Run.ID || compressed["workspaceId"] != workspace.ID {
		t.Fatalf("compressedContext identity = %+v, want activation run/workspace", compressed)
	}
	runSnapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, item.Run.ID)
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	if len(runSnapshot.Steps) != 1 || runSnapshot.Steps[0].Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("steps = %+v, want first llm workspace execution step", runSnapshot.Steps)
	}
	stepContext, _ := runSnapshot.Steps[0].Input["executionContext"].(map[string]any)
	if _, ok := stepContext["compressedContext"].(map[string]any); !ok {
		t.Fatalf("first step input = %+v, want compressed execution context propagated", runSnapshot.Steps[0].Input)
	}
	if stage, _ := runSnapshot.Steps[0].Output["stage"].(string); stage != "llm_provider_unavailable" {
		t.Fatalf("step output = %+v, want llm provider unavailable stage", runSnapshot.Steps[0].Output)
	}
	if len(runSnapshot.ToolCalls) != 1 || runSnapshot.ToolCalls[0].State != "failed" {
		t.Fatalf("toolCalls = %+v, want failed runtime evidence", runSnapshot.ToolCalls)
	}
	if len(runSnapshot.ValidationResults) != 1 || runSnapshot.ValidationResults[0].State != "failed" {
		t.Fatalf("validationResults = %+v, want failed runtime evidence", runSnapshot.ValidationResults)
	}
	if len(runSnapshot.RuntimeCheckpoints) == 0 {
		t.Fatalf("runtimeCheckpoints empty, want runtime checkpoints")
	}
	if len(runSnapshot.Patches) != 0 {
		t.Fatalf("patches = %+v, want no patch evidence until a real patch is applied", runSnapshot.Patches)
	}
}

func TestActivationObservableRunState(t *testing.T) {
	for _, state := range []common.RunState{
		common.RunStateRunning,
		common.RunStateValidating,
		common.RunStateBlockedOnDecision,
		common.RunStateBlockedOnApproval,
		common.RunStateCompleted,
		common.RunStateFailed,
		common.RunStateCancelled,
		common.RunStateSuperseded,
	} {
		if !isActivationObservableRunState(state) {
			t.Fatalf("isActivationObservableRunState(%s) = false, want true", state)
		}
	}
	for _, state := range []common.RunState{"", common.RunStateCreated} {
		if isActivationObservableRunState(state) {
			t.Fatalf("isActivationObservableRunState(%s) = true, want false", state)
		}
	}
}

func TestActivateWorkspaceExecutionWithBackendRequiresGoalAndDoD(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-missing", "activation missing workspace", "", "workspaces/activation-missing")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	if _, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument when goal and DoD are missing", err)
	}
}

func TestActivateWorkspaceExecutionWithBackendUsesLatestGoalAndDoDDefinitions(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-ambiguous", "activation ambiguous workspace", "", "workspaces/activation-ambiguous")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	firstGoal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "first activation goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend first: %v", err)
	}
	secondGoal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "second activation goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend second: %v", err)
	}
	firstDoD, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"first activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend first: %v", err)
	}
	secondDoD, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"second activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend second: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend latest: %v", err)
	}
	if item.Run.GoalID != secondGoal.ID || item.Run.DoDID != secondDoD.ID || item.Run.State != "blocked_on_decision" || item.Run.BlockingTicketID == "" {
		t.Fatalf("activation run = %+v, want latest goal/dod blocked on provider decision", item.Run)
	}
	if item.Run.ExecutionContext["goalId"] != secondGoal.ID {
		t.Fatalf("executionContext goalId = %+v, want latest goal", item.Run.ExecutionContext["goalId"])
	}
	if item.Run.ExecutionContext["doDId"] != secondDoD.ID {
		t.Fatalf("executionContext doDId = %+v, want latest dod", item.Run.ExecutionContext["doDId"])
	}
	goalContext, _ := item.Run.ExecutionContext["goal"].(map[string]any)
	goalText, _ := goalContext["text"].(string)
	if !strings.Contains(goalText, "first activation goal") || !strings.Contains(goalText, "second activation goal") {
		t.Fatalf("effective goal text = %q, want cumulative workspace goals", goalText)
	}
	dodContext, _ := item.Run.ExecutionContext["dod"].(map[string]any)
	dodItems, _ := dodContext["items"].([]string)
	if !stringSliceContains(dodItems, "first activation dod") || !stringSliceContains(dodItems, "second activation dod") {
		t.Fatalf("effective dod items = %+v, want cumulative workspace dods", dodItems)
	}

	item, err = ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: firstGoal.ID,
		DoDID:  firstDoD.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend explicit: %v", err)
	}
	if item.Run.GoalID != secondGoal.ID || item.Run.DoDID != secondDoD.ID || item.Run.State != "blocked_on_decision" || item.Run.BlockingTicketID == "" {
		t.Fatalf("activation run = %+v, want stale explicit goal/dod upgraded to latest definitions and blocked on provider decision", item.Run)
	}
}

func TestActivateWorkspaceExecutionWithBackendBindsSelectedContext(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-activation-context", "activation context workspace", "", "workspaces/activation-context")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "activation context goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"activation context dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateMCPServerWithBackend(ctx, "file", dataRoot, CreateMCPServerCommand{
		ID:           "mcp-activation-context",
		Name:         "Activation Context MCP",
		Transport:    "stdio",
		Enabled:      true,
		AllowedTools: []string{"repo.summary"},
	}); err != nil {
		t.Fatalf("CreateMCPServerWithBackend: %v", err)
	}
	if _, err := CreateSkillWithBackend(ctx, "file", dataRoot, CreateSkillCommand{
		ID:                    "skill-activation-context",
		Name:                  "Activation Context Skill",
		Version:               "1.0.0",
		Instruction:           "Use local repository evidence.",
		RequiredToolKinds:     []string{"shell/command"},
		RequiredMCPServerRefs: []string{"mcp-activation-context"},
	}); err != nil {
		t.Fatalf("CreateSkillWithBackend: %v", err)
	}
	if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
		ID:                    "soul-activation-context",
		Name:                  "Activation Context Soul",
		SoulMD:                "# Activation Context Soul\n\nExecute workspace goals with evidence.\n",
		AllowedSkillRefs:      []string{"skill-activation-context"},
		AllowedMCPServerRefs:  []string{"mcp-activation-context"},
		AllowedCapabilityRefs: []string{"mcp-activation-context:tool:repo.summary"},
		AllowedToolKinds:      []string{"shell/command"},
	}); err != nil {
		t.Fatalf("CreateSoulWithBackend: %v", err)
	}
	memory, err := CreateMemoryItemWithBackend(ctx, "file", dataRoot, workspace.ID, CreateMemoryItemCommand{
		MemoryID:   "memory-activation-context",
		SourceType: memorycommon.MemorySourceUser,
		MemoryType: memorycommon.MemoryTypeProjectContext,
		Summary:    "workspace should use LLM activation context",
		Content:    "Use selected soul, skill, MCP server, and memory as planning context.",
	})
	if err != nil {
		t.Fatalf("CreateMemoryItemWithBackend: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID:         goal.ID,
		DoDID:          dod.ID,
		LLMProviderID:  "reasoning",
		SoulID:         "soul-activation-context",
		SkillIDs:       []string{"skill-activation-context"},
		MCPServerIDs:   []string{"mcp-activation-context"},
		CapabilityIDs:  []string{"mcp-activation-context:tool:repo.summary"},
		MemoryIDs:      []string{memory.ID},
		DialogLanguage: "zh-CN",
		Priority:       7,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.Priority != 7 {
		t.Fatalf("run priority = %d, want 7", item.Run.Priority)
	}
	executionContext := item.Run.ExecutionContext
	if executionContext["soulId"] != "soul-activation-context" {
		t.Fatalf("executionContext = %+v, want selected soul", executionContext)
	}
	if executionContext[llmcommon.ExecutionContextLLMProviderID] != "reasoning" {
		t.Fatalf("executionContext = %+v, want selected llm provider", executionContext)
	}
	if executionContext["dialogLanguage"] != "zh-CN" {
		t.Fatalf("executionContext = %+v, want dialog language", executionContext)
	}
	goalContext, _ := executionContext["goal"].(map[string]any)
	if goalContext["text"] != "activation context goal" {
		t.Fatalf("goal context = %+v, want goal text", executionContext["goal"])
	}
	dodContext, _ := executionContext["dod"].(map[string]any)
	if !contextStringSliceContains(dodContext["items"], "activation context dod") {
		t.Fatalf("dod context = %+v, want DoD items", executionContext["dod"])
	}
	if !contextStringSliceContains(executionContext["skillIds"], "skill-activation-context") {
		t.Fatalf("skillIds = %+v, want selected skill", executionContext["skillIds"])
	}
	if !contextStringSliceContains(executionContext["mcpServerIds"], "mcp-activation-context") {
		t.Fatalf("mcpServerIds = %+v, want selected MCP server", executionContext["mcpServerIds"])
	}
	if !contextStringSliceContains(executionContext["capabilityIds"], "mcp-activation-context:tool:repo.summary") {
		t.Fatalf("capabilityIds = %+v, want selected MCP capability", executionContext["capabilityIds"])
	}
	if !contextStringSliceContains(executionContext["memoryIds"], memory.ID) {
		t.Fatalf("memoryIds = %+v, want selected memory", executionContext["memoryIds"])
	}

	runSnapshot, err := ReadRunSnapshotWithBackend(ctx, "file", dataRoot, item.Run.ID)
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	stepContext, _ := runSnapshot.Steps[0].Input["executionContext"].(map[string]any)
	if stepContext["soulId"] != "soul-activation-context" {
		t.Fatalf("first step input = %+v, want execution context propagated", runSnapshot.Steps[0].Input)
	}
	if stepContext[llmcommon.ExecutionContextLLMProviderID] != "reasoning" {
		t.Fatalf("first step input = %+v, want llm provider propagated", runSnapshot.Steps[0].Input)
	}
	if stepContext["dialogLanguage"] != "zh-CN" {
		t.Fatalf("first step input = %+v, want dialog language propagated", runSnapshot.Steps[0].Input)
	}
}

func TestActivateWorkspaceExecutionWithBackendCarriesResolvedDecisionHistory(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-activation-governance-history", "activation governance history", "", "workspaces/activation-governance-history")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "build a new proxy project")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"proxy project is implemented"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	session, err := newRuntimeSessionWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("newRuntimeSessionWithBackend: %v", err)
	}
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("session exports: %v", err)
	}
	priorRunID := common.RunID("run-prior-governance-history")
	priorTicketID := common.TicketID("ticket-prior-governance-history")
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          priorRunID,
		WorkspaceID: common.WorkspaceID(workspace.ID),
		GoalID:      common.GoalID(goal.ID),
		DoDID:       common.DoDID(dod.ID),
		State:       common.RunStateFailed,
	}); err != nil {
		t.Fatalf("create prior run: %v", err)
	}
	if err := exports.Repositories.Tickets.Create(ctx, &decisionmodel.Ticket{
		ID:           priorTicketID,
		WorkspaceID:  common.WorkspaceID(workspace.ID),
		RunID:        priorRunID,
		StepID:       "run-prior-governance-history/step-1",
		Reason:       "workspace is empty",
		DecisionKind: "provide_missing_info",
		Question:     "Should we start from scratch?",
		State:        common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("create prior ticket: %v", err)
	}
	if err := exports.Repositories.Resolutions.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-prior-governance-history",
		TicketID: priorTicketID,
		Type:     "provide_missing_info",
		Payload: map[string]any{
			"resumeInstruction": "Start from scratch and implement the MVP artifacts in this workspace.",
		},
	}); err != nil {
		t.Fatalf("create prior resolution: %v", err)
	}
	_ = session.Close(ctx)

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: goal.ID,
		DoDID:  dod.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	history := contextMapSliceFromValue(t, item.Run.ExecutionContext["governanceHistory"])
	if len(history) != 1 {
		t.Fatalf("governanceHistory = %#v, want one resolved decision", item.Run.ExecutionContext["governanceHistory"])
	}
	if history[0]["ticketId"] != string(priorTicketID) || history[0]["resolutionType"] != "provide_missing_info" {
		t.Fatalf("governanceHistory[0] = %+v, want prior resolved ticket", history[0])
	}
	payload := contextMapFromValue(t, history[0]["resolutionPayload"])
	if payload["resumeInstruction"] != "Start from scratch and implement the MVP artifacts in this workspace." {
		t.Fatalf("resolutionPayload = %+v, want resume instruction", payload)
	}
	runSnapshot, err := ReadRunSnapshotWithBackend(ctx, "file", dataRoot, item.Run.ID)
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	stepContext := contextMapFromValue(t, runSnapshot.Steps[0].Input["executionContextPackage"])
	if len(contextMapSliceFromValue(t, stepContext["governanceHistory"])) != 1 {
		t.Fatalf("first step input = %+v, want governance history propagated", runSnapshot.Steps[0].Input)
	}
}

func TestWorkspaceGovernanceHistoryUsesWorkspaceScopedTicketRead(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-governance-scope")
	goalID := common.GoalID("goal-governance-scope")
	dodID := common.DoDID("dod-governance-scope")
	ticketID := common.TicketID("ticket-governance-scope")
	runID := common.RunID("run-governance-scope")
	tickets := &workspaceScopedDecisionTicketRepository{items: []*decisionmodel.Ticket{
		{
			ID:          "ticket-other-workspace",
			WorkspaceID: "ws-other",
			RunID:       "run-other",
			State:       common.DecisionTicketStateResolved,
		},
		{
			ID:           ticketID,
			WorkspaceID:  workspaceID,
			RunID:        runID,
			StepID:       "run-governance-scope/step-1",
			Reason:       "needs guidance",
			DecisionKind: "provide_missing_info",
			Question:     "What guidance should be used?",
			State:        common.DecisionTicketStateResolved,
		},
	}}
	resolutions := staticDecisionResolutionRepository{
		ticketID: {
			ID:       "resolution-governance-scope",
			TicketID: ticketID,
			Type:     "provide_missing_info",
			Payload:  map[string]any{"resumeInstruction": "continue with the generic plan"},
		},
	}
	runs := staticRunRepository{
		runID: {
			ID:          runID,
			WorkspaceID: workspaceID,
			GoalID:      goalID,
			DoDID:       dodID,
			State:       common.RunStateFailed,
		},
	}

	history := workspaceGovernanceHistory(ctx, tickets, resolutions, runs, workspaceID, goalID, dodID, 10)
	if !tickets.listByWorkspaceCalled {
		t.Fatalf("ListByWorkspaceID was not called")
	}
	if tickets.listCalled {
		t.Fatalf("List was called, want workspace-scoped ticket read")
	}
	if len(history) != 1 || history[0]["ticketId"] != string(ticketID) {
		t.Fatalf("history = %+v, want one workspace-scoped ticket", history)
	}
}

func TestActivationContextUsesSelectedGoalAndDoDWithoutReload(t *testing.T) {
	ctx := context.Background()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := session.Bootstrap.Exports()
	workspace := &workspacemodel.Workspace{
		ID:    "ws-activation-definition-reuse",
		Title: "activation definition reuse",
		State: common.WorkspaceStateReady,
	}
	goal := &workspacemodel.Goal{
		ID:          "goal-activation-definition-reuse",
		WorkspaceID: workspace.ID,
		Text:        "reuse selected goal",
	}
	dod := &workspacemodel.DoD{
		ID:          "dod-activation-definition-reuse",
		WorkspaceID: workspace.ID,
		Items:       []string{"reuse selected dod"},
		State:       common.DoDStateReady,
	}
	if err := exports.Repositories.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, dod); err != nil {
		t.Fatalf("create dod: %v", err)
	}
	countingGoals := &countingGoalRepository{base: exports.Repositories.Goals}
	countingDoDs := &countingDoDRepository{base: exports.Repositories.DoDs}
	exports.Repositories.Goals = countingGoals
	exports.Repositories.DoDs = countingDoDs
	borrowed, err := NewRuntimeSessionFromExports(exports)
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}

	selectedGoal, selectedDoD, err := selectWorkspaceExecutionDefinitions(ctx, borrowed, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("selectWorkspaceExecutionDefinitions: %v", err)
	}
	if _, err := buildWorkspaceExecutionContextWithDefinitions(ctx, borrowed, workspace, selectedGoal, selectedDoD, ActivateWorkspaceExecutionCommand{}); err != nil {
		t.Fatalf("buildWorkspaceExecutionContextWithDefinitions: %v", err)
	}
	if countingGoals.listCalls != 2 || countingDoDs.listCalls != 2 {
		t.Fatalf("list calls goals=%d dods=%d, want selection plus effective definition lists", countingGoals.listCalls, countingDoDs.listCalls)
	}
	if countingGoals.getCalls != 0 || countingDoDs.getCalls != 0 {
		t.Fatalf("get calls goals=%d dods=%d, want no definition reload", countingGoals.getCalls, countingDoDs.getCalls)
	}
}

func TestActivationContextMergesOperatorInputPatchWithoutOverridingDefinitions(t *testing.T) {
	ctx := context.Background()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := session.Bootstrap.Exports()
	workspace := &workspacemodel.Workspace{
		ID:    "ws-activation-operator-input",
		Title: "activation operator input",
		State: common.WorkspaceStateReady,
	}
	goal := &workspacemodel.Goal{
		ID:          "goal-activation-operator-input",
		WorkspaceID: workspace.ID,
		Text:        "existing cumulative goal",
	}
	dod := &workspacemodel.DoD{
		ID:          "dod-activation-operator-input",
		WorkspaceID: workspace.ID,
		Items:       []string{"existing dod"},
		State:       common.DoDStateReady,
	}
	if err := exports.Repositories.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, dod); err != nil {
		t.Fatalf("create dod: %v", err)
	}

	contextData, err := buildWorkspaceExecutionContextWithDefinitions(ctx, session, workspace, goal, dod, ActivateWorkspaceExecutionCommand{
		ExecutionContextPatch: map[string]any{
			"goalId": "malicious-goal-override",
			"operatorInputQueue": []map[string]any{{
				"source":  "tui",
				"content": "继续执行并纳入新的对话信息",
			}},
			"latestOperatorInput": map[string]any{
				"source":  "tui",
				"content": "继续执行并纳入新的对话信息",
			},
		},
	})
	if err != nil {
		t.Fatalf("buildWorkspaceExecutionContextWithDefinitions: %v", err)
	}
	if contextData["goalId"] != string(goal.ID) {
		t.Fatalf("goalId = %v, want protected selected goal", contextData["goalId"])
	}
	queue, ok := contextData["operatorInputQueue"].([]map[string]any)
	if !ok || len(queue) != 1 || queue[0]["content"] != "继续执行并纳入新的对话信息" {
		t.Fatalf("operatorInputQueue = %#v, want merged operator input", contextData["operatorInputQueue"])
	}
	latest, ok := contextData["latestOperatorInput"].(map[string]any)
	if !ok || latest["content"] != "继续执行并纳入新的对话信息" {
		t.Fatalf("latestOperatorInput = %#v, want merged latest input", contextData["latestOperatorInput"])
	}
}

func TestActivationDefinitionSelectionReadsGoalAndDoDConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := session.Bootstrap.Exports()
	workspace := &workspacemodel.Workspace{
		ID:    "ws-activation-definition-parallel",
		Title: "activation definition parallel",
		State: common.WorkspaceStateReady,
	}
	goal := &workspacemodel.Goal{
		ID:          "goal-activation-definition-parallel",
		WorkspaceID: workspace.ID,
		Text:        "select goal concurrently",
	}
	dod := &workspacemodel.DoD{
		ID:          "dod-activation-definition-parallel",
		WorkspaceID: workspace.ID,
		Items:       []string{"select dod concurrently"},
		State:       common.DoDStateReady,
	}
	if err := exports.Repositories.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, dod); err != nil {
		t.Fatalf("create dod: %v", err)
	}
	started := make(chan string, 2)
	release := make(chan struct{})
	exports.Repositories.Goals = &blockingGoalRepository{base: exports.Repositories.Goals, started: started, release: release}
	exports.Repositories.DoDs = &blockingDoDRepository{base: exports.Repositories.DoDs, started: started, release: release}
	borrowed, err := NewRuntimeSessionFromExports(exports)
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		selectedGoal, selectedDoD, err := selectWorkspaceExecutionDefinitions(ctx, borrowed, workspace.ID, ActivateWorkspaceExecutionCommand{})
		if err == nil && (selectedGoal.ID != goal.ID || selectedDoD.ID != dod.ID) {
			err = errors.New("selected unexpected goal or dod")
		}
		errCh <- err
	}()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case name := <-started:
			seen[name] = true
		case err := <-errCh:
			t.Fatalf("selectWorkspaceExecutionDefinitions returned before both reads started: %v", err)
		case <-ctx.Done():
			t.Fatalf("started reads = %+v, want goal and dod before release: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("selectWorkspaceExecutionDefinitions: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("selectWorkspaceExecutionDefinitions did not complete after release: %v", ctx.Err())
	}
}

func TestActivationResponseReusesRunSnapshotRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := session.Bootstrap.Exports()
	workspace := &workspacemodel.Workspace{
		ID:    "ws-activation-run-snapshot-reuse",
		Title: "activation run snapshot reuse",
		State: common.WorkspaceStateReady,
	}
	goal := &workspacemodel.Goal{
		ID:          "goal-activation-run-snapshot-reuse",
		WorkspaceID: workspace.ID,
		Text:        "reuse run from snapshot",
	}
	dod := &workspacemodel.DoD{
		ID:          "dod-activation-run-snapshot-reuse",
		WorkspaceID: workspace.ID,
		Items:       []string{"reuse run snapshot run"},
		State:       common.DoDStateReady,
	}
	if err := exports.Repositories.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, dod); err != nil {
		t.Fatalf("create dod: %v", err)
	}
	countingRuns := &countingRunRepository{base: exports.Repositories.Runs}
	exports.Repositories.Runs = countingRuns
	borrowed, err := NewRuntimeSessionFromExports(exports)
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithSession(ctx, borrowed, string(workspace.ID), ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithSession: %v", err)
	}
	if item.Run.ID == "" || item.Run.ID != item.Snapshot.TaskProgress.RunID {
		t.Fatalf("activation run=%+v taskProgress=%+v, want response run from run snapshot", item.Run, item.Snapshot.TaskProgress)
	}
	if countingRuns.getCalls != 1 {
		t.Fatalf("run Get calls = %d, want RunSnapshot read only", countingRuns.getCalls)
	}
}

func TestActivateWorkspaceExecutionCreatesRunWithInlineCompressedContext(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := session.Bootstrap.Exports()
	workspace := &workspacemodel.Workspace{
		ID:    "ws-activation-inline-compressed",
		Title: "activation inline compressed",
		State: common.WorkspaceStateReady,
	}
	goal := &workspacemodel.Goal{
		ID:          "goal-activation-inline-compressed",
		WorkspaceID: workspace.ID,
		Text:        "create run with inline compressed context",
	}
	dod := &workspacemodel.DoD{
		ID:          "dod-activation-inline-compressed",
		WorkspaceID: workspace.ID,
		Items:       []string{"created run context contains compressed context before start"},
		State:       common.DoDStateReady,
	}
	if err := exports.Repositories.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, dod); err != nil {
		t.Fatalf("create dod: %v", err)
	}
	countingRuns := &countingRunRepository{base: exports.Repositories.Runs}
	countingService := &countingRunService{Service: exports.Services.RunService}
	exports.Repositories.Runs = countingRuns
	exports.Services.RunService = countingService
	borrowed, err := NewRuntimeSessionFromExports(exports)
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithSession(ctx, borrowed, string(workspace.ID), ActivateWorkspaceExecutionCommand{Priority: 7})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithSession: %v", err)
	}
	if len(countingService.createCommands) != 1 {
		t.Fatalf("create commands = %d, want one activation CreateRun command", len(countingService.createCommands))
	}
	createCommand := countingService.createCommands[0]
	compressed, ok := createCommand.ExecutionContext["compressedContext"].(map[string]any)
	if !ok {
		t.Fatalf("create executionContext = %+v, want inline compressedContext", createCommand.ExecutionContext)
	}
	if compressed["runId"] != string(createCommand.RunID) ||
		compressed["runId"] != item.Run.ID ||
		compressed["workspaceId"] != string(workspace.ID) ||
		compressed["mode"] != "deterministic_recovery_summary" {
		t.Fatalf("create compressedContext = %+v, want created activation run identity", compressed)
	}
	if createCommand.GoalID != goal.ID || createCommand.DoDID != dod.ID {
		t.Fatalf("create command goal/dod = %s/%s, want activation definitions", createCommand.GoalID, createCommand.DoDID)
	}
	if createCommand.Priority != 7 {
		t.Fatalf("create priority = %d, want activation priority", createCommand.Priority)
	}
	if len(countingRuns.createRuns) > 0 {
		runCompressed := runCompressedContext(t, mapRun(countingRuns.createRuns[0]))
		if runCompressed["runId"] != item.Run.ID || runCompressed["mode"] != "deterministic_recovery_summary" {
			t.Fatalf("repository create compressedContext = %+v, want created run context", runCompressed)
		}
	}
}

func TestActivationResponseWorkspaceSnapshotReusesRunSnapshotWorkspace(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := session.Bootstrap.Exports()
	workspace := &workspacemodel.Workspace{
		ID:    "ws-activation-workspace-snapshot-reuse",
		Title: "activation workspace snapshot reuse",
		State: common.WorkspaceStateReady,
	}
	goal := &workspacemodel.Goal{
		ID:          "goal-activation-workspace-snapshot-reuse",
		WorkspaceID: workspace.ID,
		Text:        "reuse workspace from run snapshot",
	}
	dod := &workspacemodel.DoD{
		ID:          "dod-activation-workspace-snapshot-reuse",
		WorkspaceID: workspace.ID,
		Items:       []string{"reuse workspace snapshot workspace"},
		State:       common.DoDStateReady,
	}
	if err := exports.Repositories.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, dod); err != nil {
		t.Fatalf("create dod: %v", err)
	}
	countingWorkspaces := &countingWorkspaceRepository{base: exports.Repositories.Workspaces}
	exports.Repositories.Workspaces = countingWorkspaces
	borrowed, err := NewRuntimeSessionFromExports(exports)
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithSession(ctx, borrowed, string(workspace.ID), ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithSession: %v", err)
	}
	if item.Snapshot.Workspace.ID != string(workspace.ID) {
		t.Fatalf("activation snapshot workspace=%+v, want selected workspace", item.Snapshot.Workspace)
	}
	if countingWorkspaces.getCalls != 3 {
		t.Fatalf("workspace Get calls = %d, want activation/start/run snapshot reads without extra WorkspaceSnapshot Get", countingWorkspaces.getCalls)
	}
}

func TestBuildWorkspaceExecutionContextStartsIndependentReadsConcurrently(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := session.Bootstrap.Exports()
	workspace := &workspacemodel.Workspace{
		ID:            "ws-activation-context-parallel",
		Title:         "activation context parallel",
		State:         common.WorkspaceStateReady,
		DefaultSoulID: "soul-activation-context-parallel",
	}
	goal := &workspacemodel.Goal{
		ID:          "goal-activation-context-parallel",
		WorkspaceID: workspace.ID,
		Text:        "build activation context with independent reads",
	}
	dod := &workspacemodel.DoD{
		ID:          "dod-activation-context-parallel",
		WorkspaceID: workspace.ID,
		Items:       []string{"independent reads start concurrently"},
		State:       common.DoDStateReady,
	}
	soul := &soulmodel.Soul{
		ID:     "soul-activation-context-parallel",
		Name:   "Activation Context Parallel Soul",
		SoulMD: "# Activation Context Parallel Soul\n",
	}
	if err := exports.Repositories.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, dod); err != nil {
		t.Fatalf("create dod: %v", err)
	}
	if err := exports.Repositories.Souls.Create(ctx, soul); err != nil {
		t.Fatalf("create soul: %v", err)
	}

	started := make(chan string, 3)
	release := make(chan struct{})
	exports.Repositories.Souls = &blockingSoulRepository{base: exports.Repositories.Souls, started: started, release: release}
	exports.Services.MemoryService = &blockingMemoryService{started: started, release: release}
	exports.Repositories.Tickets = &blockingDecisionTicketRepository{base: exports.Repositories.Tickets, started: started, release: release}
	borrowed, err := NewRuntimeSessionFromExports(exports)
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}

	var completed atomic.Bool
	errCh := make(chan error, 1)
	go func() {
		_, err := buildWorkspaceExecutionContextWithDefinitions(ctx, borrowed, workspace, goal, dod, ActivateWorkspaceExecutionCommand{})
		completed.Store(true)
		errCh <- err
	}()

	seen := map[string]bool{}
	for len(seen) < 3 {
		select {
		case name := <-started:
			seen[name] = true
		case <-ctx.Done():
			t.Fatalf("started reads = %+v, want soul, memory, and governance before release: %v", seen, ctx.Err())
		}
	}
	if completed.Load() {
		t.Fatalf("activation context completed before blocked reads were released; test no longer verifies concurrent start")
	}
	close(release)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("buildWorkspaceExecutionContextWithDefinitions: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("activation context did not complete after release: %v", ctx.Err())
	}
}

func TestActivationContextReadsMCPServersAndCapabilitiesConcurrently(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := session.Bootstrap.Exports()
	workspace := &workspacemodel.Workspace{
		ID:    "ws-activation-mcp-parallel",
		Title: "activation mcp parallel",
		State: common.WorkspaceStateReady,
	}
	goal := &workspacemodel.Goal{
		ID:          "goal-activation-mcp-parallel",
		WorkspaceID: workspace.ID,
		Text:        "build activation context with mcp reads",
	}
	dod := &workspacemodel.DoD{
		ID:          "dod-activation-mcp-parallel",
		WorkspaceID: workspace.ID,
		Items:       []string{"mcp server and capability reads start concurrently"},
		State:       common.DoDStateReady,
	}
	if err := exports.Repositories.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, dod); err != nil {
		t.Fatalf("create dod: %v", err)
	}
	if err := exports.Services.MCPService.CreateServer(ctx, mcpmodule.CreateServerCommand{
		ServerID:     "mcp-activation-mcp-parallel",
		Name:         "Activation MCP Parallel",
		Transport:    "stdio",
		Enabled:      true,
		AllowedTools: []string{"repo.summary"},
	}); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}

	started := make(chan string, 2)
	release := make(chan struct{})
	exports.Services.MCPService = &blockingMCPService{base: exports.Services.MCPService, started: started, release: release}
	borrowed, err := NewRuntimeSessionFromExports(exports)
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := buildWorkspaceExecutionContextWithDefinitions(ctx, borrowed, workspace, goal, dod, ActivateWorkspaceExecutionCommand{
			MCPServerIDs:  []string{"mcp-activation-mcp-parallel"},
			CapabilityIDs: []string{"mcp-activation-mcp-parallel:tool:repo.summary"},
		})
		errCh <- err
	}()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case name := <-started:
			seen[name] = true
		case err := <-errCh:
			t.Fatalf("buildWorkspaceExecutionContextWithDefinitions returned before both MCP reads started: %v", err)
		case <-ctx.Done():
			t.Fatalf("started reads = %+v, want mcp-server and mcp-capability before release: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("buildWorkspaceExecutionContextWithDefinitions: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("activation context did not complete after release: %v", ctx.Err())
	}
}

func TestWorkspaceSoulBindingsWithBackendRoutesExecutionSoulByStage(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-soul-routing", "soul routing workspace", "", "workspaces/soul-routing")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "route soul by workspace stage")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"soul routing is exposed"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	for _, id := range []string{"soul-default", "soul-execution", "soul-decision", "soul-validation"} {
		if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
			ID:     id,
			Name:   id,
			SoulMD: "# " + id + "\n\nUse " + id + ".\n",
		}); err != nil {
			t.Fatalf("CreateSoulWithBackend %s: %v", id, err)
		}
	}

	bindings, err := UpdateWorkspaceSoulBindingsWithBackend(ctx, "file", dataRoot, workspace.ID, WorkspaceSoulBindings{
		DefaultSoulID: "soul-default",
		Bindings: map[string]string{
			SoulBindingStageExecution: "soul-execution",
			SoulBindingStageDecision:  "soul-decision",
		},
	})
	if err != nil {
		t.Fatalf("UpdateWorkspaceSoulBindingsWithBackend: %v", err)
	}
	if bindings.DefaultSoulID != "soul-default" ||
		bindings.Bindings[SoulBindingStageExecution] != "soul-execution" ||
		bindings.Bindings[SoulBindingStageDecision] != "soul-decision" {
		t.Fatalf("bindings = %+v, want default/execution/decision souls", bindings)
	}
	readBack, err := ReadWorkspaceSoulBindingsWithBackend(ctx, "file", dataRoot, workspace.ID)
	if err != nil {
		t.Fatalf("ReadWorkspaceSoulBindingsWithBackend: %v", err)
	}
	if readBack.DefaultSoulID != bindings.DefaultSoulID || readBack.Bindings[SoulBindingStageExecution] != "soul-execution" {
		t.Fatalf("readBack = %+v, want persisted bindings", readBack)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: goal.ID,
		DoDID:  dod.ID,
		SoulBindings: map[string]string{
			SoulBindingStageValidation: "soul-validation",
		},
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	routing := contextMapFromValue(t, item.Run.ExecutionContext["soulRouting"])
	if routing["status"] != "resolved" || routing["effectiveDefaultSoulId"] != "soul-default" {
		t.Fatalf("soulRouting = %+v, want resolved workspace default", routing)
	}
	routingBindings := contextMapFromValue(t, routing["bindings"])
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStagePlanning, "soul-default", "workspace_default")
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStageExecution, "soul-execution", "workspace_stage_binding")
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStageDecision, "soul-decision", "workspace_stage_binding")
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStageValidation, "soul-validation", "run_stage_override")
}

func TestActivateWorkspaceExecutionWithBackendAddsWorkspaceBoundEvidencePolicy(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
		ID:     "soul-evidence-validation",
		Name:   "Evidence Validation",
		SoulMD: "# Evidence Validation\n\nRequire valid evidence.",
	}); err != nil {
		t.Fatalf("CreateSoulWithBackend: %v", err)
	}
	if _, err := CreateWorkspaceTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:   "template-evidence-policy",
		Name: "Evidence Policy Template",
		SoulBindings: map[string]string{
			SoulBindingStageValidation: "soul-evidence-validation",
		},
		EvidencePolicies: []map[string]any{
			{
				"id":                "template-validation-evidence",
				"profile":           "template-defined",
				"rule":              "zero_work_is_inconclusive",
				"appliesTo":         "shell/command",
				"commandName":       "tool",
				"argsContain":       []any{"verify"},
				"zeroWorkMarkers":   []any{"no work"},
				"qualityOnZeroWork": "inconclusive",
				"zeroWork":          true,
				"evidenceTags":      []any{"validation", "zero_work"},
				"evidenceReason":    "configured policy marked evidence as zero work",
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}
	workspace, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:            "evidence policy workspace",
		WorkingDirectory: "workspaces/evidence-policy",
		TemplateID:       "template-evidence-policy",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "implement feature with tests")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"related tests execute with evidence"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: goal.ID,
		DoDID:  dod.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	policies := contextMapSliceFromValue(t, item.Run.ExecutionContext["evidencePolicies"])
	if len(policies) != 1 || policies[0]["profile"] != "template-defined" || policies[0]["rule"] != "zero_work_is_inconclusive" {
		t.Fatalf("evidencePolicies = %+v, want template-bound zero-work policy", item.Run.ExecutionContext["evidencePolicies"])
	}
}

func TestActivateWorkspaceExecutionWithBackendAddsExecutionContractFromWorkspaceContextHints(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
		ID:     "soul-contract-execution",
		Name:   "Contract Execution",
		SoulMD: "# Contract Execution\n\nApply runtime contract.",
	}); err != nil {
		t.Fatalf("CreateSoulWithBackend: %v", err)
	}
	if _, err := CreateWorkspaceTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:   "template-state-change-contract",
		Name: "State Change Contract Template",
		SoulBindings: map[string]string{
			SoulBindingStageExecution: "soul-contract-execution",
		},
		ContextHints: map[string]any{
			"stateChangePolicy": "must_produce_state_change",
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}
	workspace, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:            "contract workspace",
		WorkingDirectory: "workspaces/contract-workspace",
		TemplateID:       "template-state-change-contract",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "deliver the required state change")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"state-changing result is produced"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: goal.ID,
		DoDID:  dod.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	contract := contextMapFromValue(t, item.Run.ExecutionContext["executionContract"])
	if contract["stateChangePolicy"] != "must_produce_state_change" {
		t.Fatalf("executionContract = %+v, want must_produce_state_change", contract)
	}
}

func TestActivateWorkspaceExecutionWithBackendUsesSystemDefaultSoul(t *testing.T) {
	useUnavailableLLMProvider(t)
	useSystemDefaultSoulConfig(t, "soul-system-default")
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-system-soul-routing", "system soul routing workspace", "", "workspaces/system-soul-routing")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "use system default soul")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"system default soul is used"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
		ID:     "soul-system-default",
		Name:   "System Default Soul",
		SoulMD: "# System Default Soul\n\nUse system default soul.\n",
	}); err != nil {
		t.Fatalf("CreateSoulWithBackend: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: goal.ID,
		DoDID:  dod.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ExecutionContext["soulId"] != "soul-system-default" {
		t.Fatalf("executionContext = %+v, want system default soul id", item.Run.ExecutionContext)
	}
	routing := contextMapFromValue(t, item.Run.ExecutionContext["soulRouting"])
	if routing["effectiveDefaultSoulId"] != "soul-system-default" {
		t.Fatalf("soulRouting = %+v, want system default soul", routing)
	}
	routingBindings := contextMapFromValue(t, routing["bindings"])
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStagePlanning, "soul-system-default", "system_default")
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStageValidation, "soul-system-default", "system_default")
}

func TestActivateWorkspaceExecutionWithBackendDoesNotInjectSystemDefaultSoulWhenStageBindingsExist(t *testing.T) {
	useUnavailableLLMProvider(t)
	useSystemDefaultSoulConfig(t, "soul-system-default")
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-stage-bound-system-default", "stage-bound workspace", "", "workspaces/stage-bound")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "use stage bindings without default soul context pollution")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"stage bindings drive current soul"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	for _, id := range []string{
		"soul-system-default",
		"soul-stage-planning",
		"soul-stage-execution",
		"soul-stage-decision",
		"soul-stage-approval",
		"soul-stage-validation",
		"soul-stage-artifact",
	} {
		if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
			ID:     id,
			Name:   id,
			SoulMD: "# " + id + "\n\nUse " + id + ".\n",
		}); err != nil {
			t.Fatalf("CreateSoulWithBackend %s: %v", id, err)
		}
	}
	_, err = UpdateWorkspaceSoulBindingsWithBackend(ctx, "file", dataRoot, workspace.ID, WorkspaceSoulBindings{
		Bindings: map[string]string{
			SoulBindingStagePlanning:   "soul-stage-planning",
			SoulBindingStageExecution:  "soul-stage-execution",
			SoulBindingStageDecision:   "soul-stage-decision",
			SoulBindingStageApproval:   "soul-stage-approval",
			SoulBindingStageValidation: "soul-stage-validation",
			SoulBindingStageArtifact:   "soul-stage-artifact",
		},
	})
	if err != nil {
		t.Fatalf("UpdateWorkspaceSoulBindingsWithBackend: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: goal.ID,
		DoDID:  dod.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ExecutionContext["soulId"] != nil || item.Run.ExecutionContext["soul"] != nil {
		t.Fatalf("executionContext default soul = %v/%v, want no system default top-level soul when stage bindings exist", item.Run.ExecutionContext["soulId"], item.Run.ExecutionContext["soul"])
	}
	routing := contextMapFromValue(t, item.Run.ExecutionContext["soulRouting"])
	if routing["effectiveDefaultSoulId"] != "soul-system-default" {
		t.Fatalf("soulRouting = %+v, want system default retained for missing-stage fallback metadata", routing)
	}
	routingBindings := contextMapFromValue(t, routing["bindings"])
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStagePlanning, "soul-stage-planning", "workspace_stage_binding")
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStageExecution, "soul-stage-execution", "workspace_stage_binding")
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStageValidation, "soul-stage-validation", "workspace_stage_binding")
}

func TestActivateWorkspaceExecutionWithBackendValidatesExecutionSoulOnly(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-stage-capability-validation", "stage capability validation", "", "workspaces/stage-capability-validation")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "stage capability validation goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"stage capability validation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateSkillWithBackend(ctx, "file", dataRoot, CreateSkillCommand{
		ID:                "skill-stage-execution",
		Name:              "Stage Execution Skill",
		Version:           "1.0.0",
		Instruction:       "Used only by the execution stage.",
		RequiredToolKinds: []string{"shell/command"},
	}); err != nil {
		t.Fatalf("CreateSkillWithBackend: %v", err)
	}
	if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
		ID:     "soul-stage-planning",
		Name:   "Stage Planning Soul",
		SoulMD: "# Stage Planning Soul\n\nPlanning must not authorize execution capabilities.\n",
	}); err != nil {
		t.Fatalf("CreateSoulWithBackend planning: %v", err)
	}
	if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
		ID:               "soul-stage-execution",
		Name:             "Stage Execution Soul",
		SoulMD:           "# Stage Execution Soul\n\nExecution authorizes selected run capabilities.\n",
		AllowedSkillRefs: []string{"skill-stage-execution"},
		AllowedToolKinds: []string{"shell/command"},
	}); err != nil {
		t.Fatalf("CreateSoulWithBackend execution: %v", err)
	}
	if _, err := UpdateWorkspaceSoulBindingsWithBackend(ctx, "file", dataRoot, workspace.ID, WorkspaceSoulBindings{
		Bindings: map[string]string{
			SoulBindingStagePlanning:  "soul-stage-planning",
			SoulBindingStageExecution: "soul-stage-execution",
		},
	}); err != nil {
		t.Fatalf("UpdateWorkspaceSoulBindingsWithBackend: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID:   goal.ID,
		DoDID:    dod.ID,
		SkillIDs: []string{"skill-stage-execution"},
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	routing := contextMapFromValue(t, item.Run.ExecutionContext["soulRouting"])
	bindings := contextMapFromValue(t, routing["bindings"])
	assertSoulRoutingBinding(t, bindings, SoulBindingStagePlanning, "soul-stage-planning", "workspace_stage_binding")
	assertSoulRoutingBinding(t, bindings, SoulBindingStageExecution, "soul-stage-execution", "workspace_stage_binding")
}

func TestWorkspaceSoulBindingsWithBackendRejectsInvalidBinding(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-soul-routing-invalid", "soul routing invalid workspace", "", "workspaces/soul-routing-invalid")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	if _, err := UpdateWorkspaceSoulBindingsWithBackend(ctx, "file", dataRoot, workspace.ID, WorkspaceSoulBindings{
		Bindings: map[string]string{"not-a-stage": "soul-missing"},
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument for unsupported stage", err)
	}
	if _, err := UpdateWorkspaceSoulBindingsWithBackend(ctx, "file", dataRoot, workspace.ID, WorkspaceSoulBindings{
		DefaultSoulID: "soul-missing",
	}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("err = %v, want not found for missing soul", err)
	}
}

func TestActivateWorkspaceExecutionWithBackendAutoSelectsSkillAndBindsRequiredMCP(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-auto-skill-context", "auto skill context workspace", "", "workspaces/auto-skill-context")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "complete auto delivery verification")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"auto delivery evidence is produced"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateMCPServerWithBackend(ctx, "file", dataRoot, CreateMCPServerCommand{
		ID:           "mcp-auto-skill",
		Name:         "Auto Skill MCP",
		Transport:    "stdio",
		Enabled:      true,
		AllowedTools: []string{"repo.summary"},
	}); err != nil {
		t.Fatalf("CreateMCPServerWithBackend: %v", err)
	}
	if _, err := CreateSkillWithBackend(ctx, "file", dataRoot, CreateSkillCommand{
		ID:                    "skill-auto-delivery",
		Name:                  "Auto Delivery Skill",
		Version:               "1.0.0",
		Description:           "Skill for auto delivery verification",
		TriggerRules:          []string{"auto delivery"},
		Instruction:           "Use this skill when Goal and DoD request auto delivery.",
		RequiredToolKinds:     []string{"llm/chat"},
		RequiredMCPServerRefs: []string{"mcp-auto-skill"},
	}); err != nil {
		t.Fatalf("CreateSkillWithBackend: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID:        goal.ID,
		DoDID:         dod.ID,
		LLMProviderID: "reasoning",
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	executionContext := item.Run.ExecutionContext
	if !contextStringSliceContains(executionContext["skillIds"], "skill-auto-delivery") {
		t.Fatalf("skillIds = %+v, want auto selected skill", executionContext["skillIds"])
	}
	if !contextStringSliceContains(executionContext["mcpServerIds"], "mcp-auto-skill") {
		t.Fatalf("mcpServerIds = %+v, want required MCP auto bound", executionContext["mcpServerIds"])
	}
	selectionPolicy, selectionStatus := skillSelectionPolicyAndStatus(executionContext["skillSelection"])
	if selectionPolicy != "goal_dod_auto_match" || selectionStatus != "selected" {
		t.Fatalf("skillSelection = %+v, want auto selected evidence", executionContext["skillSelection"])
	}
	mcpSelection, _ := executionContext["mcpSelection"].(map[string]any)
	if !contextStringSliceContains(mcpSelection["autoBoundMCPServerIds"], "mcp-auto-skill") {
		t.Fatalf("mcpSelection = %+v, want auto bound MCP evidence", mcpSelection)
	}
}

func TestActivateWorkspaceExecutionWithBackendUsesSemanticMemoryRecall(t *testing.T) {
	ctx := context.Background()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/embeddings":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"embedding":[1,0,0]}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"items\":[{\"item\":\"semantic memory selected\",\"state\":\"satisfied\",\"evidenceRefs\":[\"memory-semantic-match\"]}]}}"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	useActivationLLMConfig(t, provider.URL+"/v1", "local")
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-activation-semantic-memory", "semantic memory workspace", "", "workspaces/activation-semantic-memory")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "use semantic project context")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"semantic memory selected"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateMemoryItemWithBackend(ctx, "file", dataRoot, workspace.ID, CreateMemoryItemCommand{
		MemoryID:            "memory-semantic-match",
		SourceType:          memorycommon.MemorySourceUser,
		MemoryType:          memorycommon.MemoryTypeProjectContext,
		Summary:             "matching semantic project context",
		Content:             "This memory should be retrieved by vector similarity.",
		EmbeddingProviderID: "local",
		EmbeddingModel:      "embedding-model",
		EmbeddingText:       "matching semantic project context",
		Vector:              []float64{1, 0, 0},
	}); err != nil {
		t.Fatalf("CreateMemoryItemWithBackend match: %v", err)
	}
	if _, err := CreateMemoryItemWithBackend(ctx, "file", dataRoot, workspace.ID, CreateMemoryItemCommand{
		MemoryID:            "memory-semantic-other",
		SourceType:          memorycommon.MemorySourceUser,
		MemoryType:          memorycommon.MemoryTypeProjectContext,
		Summary:             "unrelated semantic project context",
		Content:             "This memory should not pass the vector score threshold.",
		EmbeddingProviderID: "local",
		EmbeddingModel:      "embedding-model",
		EmbeddingText:       "unrelated semantic project context",
		Vector:              []float64{0, 1, 0},
	}); err != nil {
		t.Fatalf("CreateMemoryItemWithBackend other: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: goal.ID,
		DoDID:  dod.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if !contextStringSliceContains(item.Run.ExecutionContext["memoryIds"], "memory-semantic-match") ||
		contextStringSliceContains(item.Run.ExecutionContext["memoryIds"], "memory-semantic-other") {
		t.Fatalf("memoryIds = %+v, want only semantic match", item.Run.ExecutionContext["memoryIds"])
	}
	retrieval, _ := item.Run.ExecutionContext["memoryRetrieval"].(map[string]any)
	if retrieval["policy"] != "semantic_vector_search" ||
		retrieval["status"] != "selected" ||
		retrieval["providerId"] != "local" ||
		!contextNumericEquals(retrieval["vectorDimension"], 3) {
		t.Fatalf("memoryRetrieval = %+v, want semantic evidence", retrieval)
	}
	runSnapshot, err := ReadRunSnapshotWithBackend(ctx, "file", dataRoot, item.Run.ID)
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	activationTrace := memoryRecallTraceBySource(runSnapshot.MemoryRecallTraces, "activation")
	stepInputTrace := memoryRecallTraceBySource(runSnapshot.MemoryRecallTraces, "step_input")
	if activationTrace == nil ||
		activationTrace.Status != "selected" ||
		len(activationTrace.RetrievedMemoryIDs) != 1 ||
		activationTrace.RetrievedMemoryIDs[0] != "memory-semantic-match" {
		t.Fatalf("memory recall traces = %+v, want selected activation trace", runSnapshot.MemoryRecallTraces)
	}
	if stepInputTrace == nil ||
		stepInputTrace.Status != "selected" ||
		stepInputTrace.StepID == "" ||
		stepInputTrace.Policy != "step_input_dynamic_workspace_memory" ||
		!memoryRecallTraceContainsID(stepInputTrace, "memory-semantic-match") {
		t.Fatalf("memory recall traces = %+v, want dynamic step input trace with semantic match refreshed", runSnapshot.MemoryRecallTraces)
	}
}

func TestActivateWorkspaceExecutionWithBackendReportsSemanticMemoryDegradation(t *testing.T) {
	ctx := context.Background()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"completionState\":\"continue\"}"}}]}`))
	}))
	defer provider.Close()
	useActivationLLMConfig(t, provider.URL+"/v1", "missing-embedding")
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-activation-semantic-degraded", "semantic degradation workspace", "", "workspaces/activation-semantic-degraded")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "use semantic memory degradation")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"semantic degradation visible"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateMemoryItemWithBackend(ctx, "file", dataRoot, workspace.ID, CreateMemoryItemCommand{
		MemoryID:   "memory-degraded-fallback-should-not-select",
		SourceType: memorycommon.MemorySourceUser,
		MemoryType: memorycommon.MemoryTypeProjectContext,
		Summary:    "available deterministic fallback",
		Content:    "Configured semantic recall is degraded, so this must not be silently selected.",
	}); err != nil {
		t.Fatalf("CreateMemoryItemWithBackend: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: goal.ID,
		DoDID:  dod.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ExecutionContext["memoryIds"] != nil {
		t.Fatalf("memoryIds = %+v, want no silent fallback when semantic recall degrades", item.Run.ExecutionContext["memoryIds"])
	}
	retrieval, _ := item.Run.ExecutionContext["memoryRetrieval"].(map[string]any)
	if retrieval["policy"] != "semantic_vector_search" || retrieval["status"] != "degraded" || retrieval["reason"] == "" {
		t.Fatalf("memoryRetrieval = %+v, want explicit degradation evidence", retrieval)
	}
	runSnapshot, err := ReadRunSnapshotWithBackend(ctx, "file", dataRoot, item.Run.ID)
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	activationTrace := memoryRecallTraceBySource(runSnapshot.MemoryRecallTraces, "activation")
	stepInputTrace := memoryRecallTraceBySource(runSnapshot.MemoryRecallTraces, "step_input")
	if activationTrace == nil ||
		activationTrace.Status != "degraded" ||
		activationTrace.Reason == "" {
		t.Fatalf("memory recall traces = %+v, want degraded activation trace", runSnapshot.MemoryRecallTraces)
	}
	if stepInputTrace == nil ||
		stepInputTrace.Status != "selected" ||
		stepInputTrace.StepID == "" ||
		stepInputTrace.Policy != "step_input_dynamic_workspace_memory" ||
		!memoryRecallTraceContainsID(stepInputTrace, "memory-degraded-fallback-should-not-select") {
		t.Fatalf("memory recall traces = %+v, want dynamic step input fallback trace after activation degradation", runSnapshot.MemoryRecallTraces)
	}
}

func TestSelectedMemoryItemsFiltersProviderRetryProcessMemory(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-activation-memory-filter")
	service := &activationMemorySelectionService{items: []*memorymodel.MemoryItem{
		{
			ID:          "memory-provider-retry-process",
			WorkspaceID: workspaceID,
			MemoryType:  memorycommon.MemoryTypeToolFeedback,
			Summary:     "Provider retry outcome captured",
			Content:     "Provider retry attempt: 1",
			Tags:        []string{"provider_retry", "tool_context", "necessary_output"},
		},
		{
			ID:          "memory-accepted-tool",
			WorkspaceID: workspaceID,
			MemoryType:  memorycommon.MemoryTypeToolFeedback,
			Summary:     "Accepted tool input",
			Content:     "Use go test ./...",
			Tags:        []string{"accepted_input", "tool_context"},
		},
	}}

	items, evidence, err := selectedMemoryItems(ctx, workspaceID, service, nil, nil, nil)
	if err != nil {
		t.Fatalf("selectedMemoryItems: %v", err)
	}
	if len(items) != 1 || items[0].ID != "memory-accepted-tool" {
		t.Fatalf("items = %+v, want only accepted tool memory", items)
	}
	if evidence["status"] != "selected" ||
		!contextStringSliceContains(evidence["candidateMemoryIds"], "memory-provider-retry-process") ||
		!contextStringSliceContains(evidence["excludedMemoryIds"], "memory-provider-retry-process") ||
		contextStringSliceContains(evidence["retrievedMemoryIds"], "memory-provider-retry-process") {
		t.Fatalf("evidence = %+v, want provider retry candidate excluded from retrieval", evidence)
	}
	contexts := memoryContexts(items)
	if len(contexts) != 1 {
		t.Fatalf("contexts = %+v, want one context", contexts)
	}
	for _, noisyKey := range []string{"workspaceId", "runId", "sourceType", "sourceRef", "embeddingProviderId", "embeddingModel", "vectorReady", "vectorDimension"} {
		if _, ok := contexts[0][noisyKey]; ok {
			t.Fatalf("memory context = %+v, must not expose process key %s", contexts[0], noisyKey)
		}
	}
}

func memoryRecallTraceContainsID(item *MemoryRecallTrace, id string) bool {
	if item == nil {
		return false
	}
	for _, memoryID := range item.RetrievedMemoryIDs {
		if memoryID == id {
			return true
		}
	}
	return false
}

func memoryRecallTraceBySource(items []MemoryRecallTrace, source string) *MemoryRecallTrace {
	for idx := range items {
		if items[idx].Source == source {
			return &items[idx]
		}
	}
	return nil
}

func TestActivateWorkspaceExecutionWithBackendRejectsSoulDisallowedSkill(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-activation-context-reject", "activation context reject workspace", "", "workspaces/activation-context-reject")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "activation context reject goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"activation context reject dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateSkillWithBackend(ctx, "file", dataRoot, CreateSkillCommand{
		ID:          "skill-disallowed",
		Name:        "Disallowed Skill",
		Version:     "1.0.0",
		Instruction: "Not allowed by selected soul.",
	}); err != nil {
		t.Fatalf("CreateSkillWithBackend: %v", err)
	}
	if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
		ID:     "soul-restrictive",
		Name:   "Restrictive Soul",
		SoulMD: "# Restrictive Soul\n\nOnly use explicitly allowed capabilities.\n",
	}); err != nil {
		t.Fatalf("CreateSoulWithBackend: %v", err)
	}

	_, err = ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID:   goal.ID,
		DoDID:    dod.ID,
		SoulID:   "soul-restrictive",
		SkillIDs: []string{"skill-disallowed"},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument for disallowed skill", err)
	}
}

func TestActivateWorkspaceExecutionWithBackendReturnsActiveRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-active", "activation active workspace", "", "workspaces/activation-active")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	activeRunID := "run-activation-active-blocked-decision"
	started := prepareDecisionRun(t, dataRoot, workspace.ID, activeRunID)
	if started.State != "blocked_on_decision" {
		t.Fatalf("started run = %+v, want blocked_on_decision", started)
	}
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()
	workspaceItem, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.ActiveRunID = common.RunID(activeRunID)
	if err := exports.Repositories.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace active run: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ID != activeRunID || item.Run.State != "blocked_on_decision" {
		t.Fatalf("activation run = %+v, want existing active blocked run", item.Run)
	}
	if len(item.Snapshot.Runs) != 1 || item.Snapshot.Workspace.ActiveRunID != activeRunID {
		t.Fatalf("activation snapshot = %+v, want active run projection", item.Snapshot)
	}
}

func TestActivateWorkspaceExecutionWithBackendIgnoresTerminalActiveRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-terminal-active", "activation terminal active workspace", "", "workspaces/activation-terminal-active")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "terminal active replacement goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"terminal active replacement dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()
	oldRunID := common.RunID("run-activation-terminal-active-old")
	if err := exports.Services.RunService.CreateRun(ctx, runmodule.CreateRunCommand{
		RunID:       oldRunID,
		WorkspaceID: common.WorkspaceID(workspace.ID),
		GoalID:      common.GoalID(goal.ID),
		DoDID:       common.DoDID(dod.ID),
	}); err != nil {
		t.Fatalf("CreateRun old active: %v", err)
	}
	oldRun, err := exports.Repositories.Runs.Get(ctx, oldRunID)
	if err != nil {
		t.Fatalf("get old run: %v", err)
	}
	oldRun.State = common.RunStateSuperseded
	if err := exports.Repositories.Runs.Update(ctx, oldRun); err != nil {
		t.Fatalf("mark old run superseded: %v", err)
	}
	workspaceItem, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.ActiveRunID = oldRunID
	workspaceItem.State = common.WorkspaceStateFailed
	if err := exports.Repositories.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace terminal active run: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ID == string(oldRunID) || item.Run.GoalID != goal.ID || item.Run.DoDID != dod.ID {
		t.Fatalf("activation run = %+v, want new run using selected definitions", item.Run)
	}
	if item.Snapshot.Workspace.ActiveRunID == string(oldRunID) {
		t.Fatalf("snapshot workspace activeRunId = %q, want replacement run", item.Snapshot.Workspace.ActiveRunID)
	}
}

func TestActivateWorkspaceExecutionWithBackendReplacesActiveRunWhenExplicitDefinitionsChange(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-replace-active", "activation replace active workspace", "", "workspaces/activation-replace-active")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	oldGoal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "old activation goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend old: %v", err)
	}
	newGoal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "new activation goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend new: %v", err)
	}
	oldDoD, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"old activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend old: %v", err)
	}
	newDoD, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"new activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend new: %v", err)
	}

	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()
	oldRunID := common.RunID("run-activation-replace-active-old")
	if err := exports.Services.RunService.CreateRun(ctx, runmodule.CreateRunCommand{
		RunID:       oldRunID,
		WorkspaceID: common.WorkspaceID(workspace.ID),
		GoalID:      common.GoalID(oldGoal.ID),
		DoDID:       common.DoDID(oldDoD.ID),
	}); err != nil {
		t.Fatalf("CreateRun old active: %v", err)
	}
	oldRun, err := exports.Repositories.Runs.Get(ctx, oldRunID)
	if err != nil {
		t.Fatalf("get old run: %v", err)
	}
	oldRun.State = common.RunStateRunning
	if err := exports.Repositories.Runs.Update(ctx, oldRun); err != nil {
		t.Fatalf("mark old run running: %v", err)
	}
	workspaceItem, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.ActiveRunID = oldRunID
	workspaceItem.State = common.WorkspaceStateRunning
	if err := exports.Repositories.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace active run: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ID == string(oldRunID) || item.Run.GoalID != newGoal.ID || item.Run.DoDID != newDoD.ID {
		t.Fatalf("activation run = %+v, want replacement run with new goal/dod", item.Run)
	}
	replaced, err := exports.Repositories.Runs.Get(ctx, oldRunID)
	if err != nil {
		t.Fatalf("get replaced run: %v", err)
	}
	if replaced.State != common.RunStateSuperseded {
		t.Fatalf("old run state = %q, want superseded", replaced.State)
	}
}

func contextStringSliceContains(value any, want string) bool {
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if item == want {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if item == want {
				return true
			}
		}
	}
	return false
}

func contextNumericEquals(value any, want float64) bool {
	switch typed := value.(type) {
	case int:
		return float64(typed) == want
	case int64:
		return float64(typed) == want
	case float64:
		return typed == want
	default:
		return false
	}
}

func contextMapFromValue(t *testing.T, value any) map[string]any {
	t.Helper()
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal context value: %v", err)
	}
	var ret map[string]any
	if err := json.Unmarshal(payload, &ret); err != nil {
		t.Fatalf("unmarshal context value: %v", err)
	}
	return ret
}

func contextMapSliceFromValue(t *testing.T, value any) []map[string]any {
	t.Helper()
	if value == nil {
		return nil
	}
	if typed, ok := value.([]map[string]any); ok {
		return typed
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal context slice value: %v", err)
	}
	var ret []map[string]any
	if err := json.Unmarshal(payload, &ret); err != nil {
		t.Fatalf("unmarshal context slice value: %v", err)
	}
	return ret
}

func assertSoulRoutingBinding(t *testing.T, bindings map[string]any, stage string, soulID string, source string) {
	t.Helper()
	binding := contextMapFromValue(t, bindings[stage])
	if binding["soulId"] != soulID || binding["source"] != source {
		t.Fatalf("binding[%s] = %+v, want %s/%s", stage, binding, soulID, source)
	}
}

func skillSelectionPolicyAndStatus(value any) (string, string) {
	switch typed := value.(type) {
	case skillSelectionEvidence:
		return typed.Policy, typed.Status
	case map[string]any:
		policy, _ := typed["policy"].(string)
		status, _ := typed["status"].(string)
		return policy, status
	default:
		return "", ""
	}
}

func useUnavailableLLMProvider(t *testing.T) {
	t.Helper()
	t.Setenv(llmcommon.EnvProvider, "")
	t.Setenv(llmcommon.EnvBaseURL, "")
	t.Setenv(llmcommon.EnvAPIKey, "")
	t.Setenv(llmcommon.EnvModel, "")
	t.Setenv(llmcommon.EnvDisabled, "")
	t.Setenv(llmcommon.EnvTimeout, "")
}

func useActivationLLMConfig(t *testing.T, baseURL string, embeddingProviderID string) {
	t.Helper()
	configDir := t.TempDir()
	content := strings.ReplaceAll(`
endpointName = "workorch"

[repository]
backend = "file"

[llm]
defaultProvider = "local"
defaultEmbeddingProvider = "__EMBEDDING_PROVIDER__"

[llm.providers.local]
provider = "compatible"
baseURL = "__BASE_URL__"
apiKey = "secret-value"
model = "test-model"
timeout = "5s"
disabled = false
chatCompletions = true
streaming = false
nonChatOperations = ["embeddings"]
`, "__BASE_URL__", baseURL)
	content = strings.ReplaceAll(content, "__EMBEDDING_PROVIDER__", embeddingProviderID)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write application config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
}

func useSystemDefaultSoulConfig(t *testing.T, defaultSoulID string) {
	t.Helper()
	configDir := t.TempDir()
	content := strings.ReplaceAll(`
endpointName = "workorch"

[repository]
backend = "file"

[runtime]
defaultSoulId = "__DEFAULT_SOUL_ID__"

[llm]
defaultProvider = "local"

[llm.providers.local]
provider = "compatible"
baseURL = ""
apiKey = ""
model = ""
timeout = "5s"
disabled = false
`, "__DEFAULT_SOUL_ID__", defaultSoulID)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write application config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
}
