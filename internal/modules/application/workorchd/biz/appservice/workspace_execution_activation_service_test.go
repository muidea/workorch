package appservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/skillselectcore"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	mcpmodule "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/contracts"
	memorycommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memorymodule "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/contracts"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type workspaceScopedDecisionTicketRepository struct {
	items                 []*decisionmodel.Ticket
	listCalled            bool
	listByWorkspaceCalled bool
}

func runtimeV2OwnerRunWithBackend(t *testing.T, ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) *runmodel.Run {
	t.Helper()
	session, err := sharedBackendSession(ctx, backend, dataRoot)
	if err != nil {
		t.Fatalf("sharedBackendSession: %v", err)
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		t.Fatalf("RequiredCommandHub: %v", err)
	}
	item, err := runports.ReadRun(ctx, hub, common.RunID(runID))
	if err != nil {
		t.Fatalf("ReadRun: %v", err)
	}
	if item.WorkspaceID != common.WorkspaceID(workspaceID) || item.RuntimeBootstrap == nil {
		t.Fatalf("owner run = %+v, want workspace-scoped Runtime V2 bootstrap", item)
	}
	return item
}

func runtimeV2StageSoulBinding(t *testing.T, activation runtimecontract.RuntimeActivationContext, stage string) runtimecontract.RuntimeStageSoulBinding {
	t.Helper()
	for _, binding := range activation.StageSoulBindings {
		if binding.Stage == stage {
			return binding
		}
	}
	t.Fatalf("stage soul binding %q missing from %+v", stage, activation.StageSoulBindings)
	return runtimecontract.RuntimeStageSoulBinding{}
}

func assertRuntimeV2StageSoulBinding(t *testing.T, activation runtimecontract.RuntimeActivationContext, stage string, soulID string, source string) {
	t.Helper()
	binding := runtimeV2StageSoulBinding(t, activation, stage)
	if string(binding.Soul.ID) != soulID || binding.Source != source {
		t.Fatalf("stage binding %q = %+v, want soul %q source %q", stage, binding, soulID, source)
	}
}

func runtimeV2ProfileContract(t *testing.T, contract runtimecontract.RuntimeExecutionContract, id string) runtimecontract.RuntimeProfileContract {
	t.Helper()
	for _, profile := range contract.ProfileContracts {
		if profile.ID == id {
			return profile
		}
	}
	t.Fatalf("profile contract %q missing from %+v", id, contract.ProfileContracts)
	return runtimecontract.RuntimeProfileContract{}
}

func runtimeV2HasMCPServer(activation runtimecontract.RuntimeActivationContext, id string) bool {
	for _, item := range activation.MCPServerIDs {
		if string(item) == id {
			return true
		}
	}
	return false
}

func runtimeV2HasSkill(activation runtimecontract.RuntimeActivationContext, id string) bool {
	for _, item := range activation.Skills {
		if item.ID == id {
			return true
		}
	}
	return false
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

func (r staticRunRepository) GetByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (*runmodel.Run, error) {
	item, ok := r[runID]
	if !ok || item.WorkspaceID != workspaceID {
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

func workspaceGovernanceHistory(
	ctx context.Context,
	tickets repository.DecisionTicketRepository,
	resolutions repository.DecisionResolutionRepository,
	runs repository.RunRepository,
	workspaceID common.WorkspaceID,
	goalID common.GoalID,
	dodID common.DoDID,
	limit int,
) []map[string]any {
	if tickets == nil || resolutions == nil || runs == nil || workspaceID == "" || limit <= 0 {
		return nil
	}
	workspaceTickets, err := tickets.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil
	}
	ret := make([]map[string]any, 0, min(limit, len(workspaceTickets)))
	for _, ticket := range workspaceTickets {
		if ticket == nil || ticket.State != common.DecisionTicketStateResolved {
			continue
		}
		run, err := runs.Get(ctx, ticket.RunID)
		if err != nil || run == nil || run.WorkspaceID != workspaceID {
			continue
		}
		if goalID != "" && run.GoalID != goalID {
			continue
		}
		if dodID != "" && run.DoDID != dodID {
			continue
		}
		resolution, err := resolutions.GetByTicketID(ctx, ticket.ID)
		if err != nil || resolution == nil {
			continue
		}
		ret = append(ret, common.RedactSensitiveMap(map[string]any{
			"ticketId":          string(ticket.ID),
			"runId":             string(ticket.RunID),
			"stepId":            string(ticket.StepID),
			"decisionKind":      ticket.DecisionKind,
			"reason":            ticket.Reason,
			"question":          ticket.Question,
			"ticketContext":     activationcore.CompactFailureContext(ticket.Context.Map()),
			"resolutionId":      string(resolution.ID),
			"resolutionType":    resolution.Type,
			"resolutionPayload": activationcore.CompactDecisionPayload(resolution.Payload.Map()),
		}))
		if len(ret) >= limit {
			break
		}
	}
	return ret
}

func buildWorkspaceRuntimeActivationWithDefinitions(ctx context.Context, session *RuntimeSession, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD, cmd ActivateWorkspaceExecutionCommand) (activationports.WorkspaceRuntimeActivationBuild, error) {
	hub, err := session.RequiredCommandHub()
	if err != nil {
		return activationports.WorkspaceRuntimeActivationBuild{}, err
	}
	cfg, err := configcore.LoadFromContext(ctx)
	if err != nil {
		return activationports.WorkspaceRuntimeActivationBuild{}, err
	}
	return activationports.BuildWorkspaceRuntimeActivation(ctx, activationports.WorkspaceRuntimeActivationRequest{
		Hub:                   hub,
		Workspace:             workspace,
		Goal:                  goal,
		DoD:                   dod,
		Command:               cmd,
		SystemDefaultSoulID:   common.SoulID(strings.TrimSpace(cfg.Runtime.DefaultSoulID)),
		DefaultDialogLanguage: localizationcore.DefaultDialogLanguageFromContext(ctx),
		DefaultEmbeddingProviderLoader: func(context.Context) (runtimecontract.ProviderModelTarget, error) {
			if cfg.LLM.DefaultEmbedding == nil {
				return runtimecontract.ProviderModelTarget{}, common.ErrInvalidArgument
			}
			return cfg.LLM.DefaultEmbedding.ToRuntimeTarget(), nil
		},
		SkillHubCatalog: skillHubCatalogForEventHub(hub),
	})
}

type countingRunRepository struct {
	base        repository.RunRepository
	createRuns  []*runmodel.Run
	getCalls    int
	updateCalls int
}

type countingRunService struct {
	base interface {
		CreateRun(context.Context, runmodule.CreateRunCommand) error
	}
	createCommands []runmodule.CreateRunCommand
}

func (s *countingRunService) CreateRun(ctx context.Context, cmd runmodule.CreateRunCommand) error {
	s.createCommands = append(s.createCommands, runmodule.CreateRunCommand{
		RunID: cmd.RunID, WorkspaceID: cmd.WorkspaceID, GoalID: cmd.GoalID, DoDID: cmd.DoDID,
		DefinitionRevision: cmd.DefinitionRevision, InitialStepKind: cmd.InitialStepKind,
		RuntimeBootstrap: cmd.RuntimeBootstrap, Priority: cmd.Priority,
	})
	return s.base.CreateRun(ctx, cmd)
}

func (r *countingRunRepository) Create(ctx context.Context, item *runmodel.Run) error {
	if item != nil {
		r.createRuns = append(r.createRuns, &runmodel.Run{
			ID: item.ID, WorkspaceID: item.WorkspaceID, GoalID: item.GoalID, DoDID: item.DoDID,
			DefinitionRevision: item.DefinitionRevision, InitialStepKind: item.InitialStepKind,
			RuntimeBootstrap:   item.RuntimeBootstrap,
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

func (r *countingRunRepository) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	return r.base.GetByWorkspaceID(ctx, workspaceID, id)
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

type countingActivationEventHub struct {
	base   magicevent.Hub
	mu     sync.Mutex
	counts map[string]int
	onSend func(magicevent.Event)
}

func (h *countingActivationEventHub) Subscribe(id string, observer magicevent.Observer) {
	h.base.Subscribe(id, observer)
}
func (h *countingActivationEventHub) Unsubscribe(id string, observer magicevent.Observer) {
	h.base.Unsubscribe(id, observer)
}
func (h *countingActivationEventHub) Post(event magicevent.Event) {
	h.capture(event)
	h.base.Post(event)
}
func (h *countingActivationEventHub) Send(event magicevent.Event) magicevent.Result {
	h.capture(event)
	return h.base.Send(event)
}
func (h *countingActivationEventHub) Terminate(ctx context.Context) { h.base.Terminate(ctx) }

func (h *countingActivationEventHub) record(topic string) {
	h.mu.Lock()
	if h.counts == nil {
		h.counts = map[string]int{}
	}
	h.counts[topic]++
	h.mu.Unlock()
}

func (h *countingActivationEventHub) capture(event magicevent.Event) {
	h.record(event.ID())
	if h.onSend != nil {
		h.onSend(event)
	}
}

func (h *countingActivationEventHub) Count(topic string) int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.counts[topic]
}

type blockingActivationEventHub struct {
	base    magicevent.Hub
	started chan<- string
	release <-chan struct{}
	labels  map[string]string
}

func (h *blockingActivationEventHub) Subscribe(id string, observer magicevent.Observer) {
	h.base.Subscribe(id, observer)
}
func (h *blockingActivationEventHub) Unsubscribe(id string, observer magicevent.Observer) {
	h.base.Unsubscribe(id, observer)
}
func (h *blockingActivationEventHub) Post(event magicevent.Event) {
	h.wait(event.Context(), event.ID())
	h.base.Post(event)
}
func (h *blockingActivationEventHub) Send(event magicevent.Event) magicevent.Result {
	h.wait(event.Context(), event.ID())
	return h.base.Send(event)
}
func (h *blockingActivationEventHub) Terminate(ctx context.Context) { h.base.Terminate(ctx) }

func (h *blockingActivationEventHub) wait(ctx context.Context, topic string) {
	if h == nil || h.labels == nil {
		return
	}
	if label := h.labels[topic]; label != "" {
		select {
		case h.started <- label:
		case <-ctx.Done():
			return
		}
		select {
		case <-h.release:
		case <-ctx.Done():
		}
	}
}

func TestPublishActivationRunCommandRequiresResponse(t *testing.T) {
	hub := magicevent.NewHub(16)
	err := activationports.StartRun(context.Background(), hub, "ws-activation", "run-activation")
	if err == nil {
		t.Fatal("activationports.StartRun returned nil, want missing response error")
	}
	if !strings.Contains(err.Error(), "missing observer") {
		t.Fatalf("activationports.StartRun error = %v, want missing observer error", err)
	}
}

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

func TestActivationObservableRunState(t *testing.T) {
	for _, state := range []common.RunState{
		common.RunStateActive,
		common.RunStateActive,
		common.RunStateBlockedOnDecision,
		common.RunStateBlockedOnApproval,
		common.RunStateCompleted,
		common.RunStateFailed,
		common.RunStateCancelled,
		common.RunStateSuperseded,
	} {
		if !activationcore.IsObservableRunState(state) {
			t.Fatalf("IsObservableRunState(%s) = false, want true", state)
		}
	}
	for _, state := range []common.RunState{"", common.RunStatePending} {
		if activationcore.IsObservableRunState(state) {
			t.Fatalf("IsObservableRunState(%s) = true, want false", state)
		}
	}
}

func TestActivationSnapshotDoesNotProjectCreatedRunAsActive(t *testing.T) {
	workspace := &workspacemodel.Workspace{
		ID:    common.WorkspaceID("ws-created-projection"),
		Title: "created projection workspace",
		State: common.WorkspaceStateActive,
	}
	run := &runmodel.Run{
		ID:          common.RunID("run-created-projection"),
		WorkspaceID: workspace.ID,
		State:       common.RunStatePending,
	}

	snapshot := readmodelprojection.ActivationSnapshot(workspace, run, nil, nil)
	if snapshot.Workspace.State != string(common.WorkspaceStateActive) || snapshot.Workspace.ActiveRunID != "" {
		t.Fatalf("snapshot workspace = %+v, want active workspace without pending active run projection", snapshot.Workspace)
	}
}

func TestActivateWorkspaceExecutionWithBackendUsesNumericRunSequence(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-run-sequence", "activation run sequence workspace", "", "workspaces/activation-run-sequence")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "activation run sequence goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"activation run sequence dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	for _, runID := range []string{"run-001", "run-007", "run-legacyabcdef12345678"} {
		if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, workspace.ID, runID, goal.ID, dod.ID); err != nil {
			t.Fatalf("CreateRunWithBackend %s: %v", runID, err)
		}
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ID != "run-008" {
		t.Fatalf("activation run id = %q, want run-008", item.Run.ID)
	}
}

func TestActivateWorkspaceExecutionWithBackendScopesNumericRunSequenceByWorkspace(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	workspaceA, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-run-sequence-a", "activation run sequence workspace a", "", "workspaces/activation-run-sequence-a")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend a: %v", err)
	}
	goalA, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspaceA.ID, "activation run sequence goal a")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend a: %v", err)
	}
	dodA, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspaceA.ID, []string{"activation run sequence dod a"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend a: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, workspaceA.ID, "run-099", goalA.ID, dodA.ID); err != nil {
		t.Fatalf("CreateRunWithBackend workspace a seed: %v", err)
	}

	workspaceB, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-run-sequence-b", "activation run sequence workspace b", "", "workspaces/activation-run-sequence-b")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend b: %v", err)
	}
	goalB, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspaceB.ID, "activation run sequence goal b")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend b: %v", err)
	}
	dodB, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspaceB.ID, []string{"activation run sequence dod b"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend b: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, workspaceB.ID, "run-002", goalB.ID, dodB.ID); err != nil {
		t.Fatalf("CreateRunWithBackend workspace b seed: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspaceB.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ID != "run-003" {
		t.Fatalf("activation run id = %q, want run-003 scoped to workspace b", item.Run.ID)
	}
}

func TestActivateWorkspaceExecutionWithBackendContinuesRunSequenceAfterRestart(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-run-restart-sequence", "activation run restart sequence workspace", "", "workspaces/activation-run-restart-sequence")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "activation run restart sequence goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"activation run restart sequence dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, workspace.ID, "run-099", goal.ID, dod.ID); err != nil {
		t.Fatalf("CreateRunWithBackend seed: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ID != "run-100" {
		t.Fatalf("activation run id = %q, want run-100", item.Run.ID)
	}
}

func TestActivateWorkspaceExecutionWithBackendRequiresGoalAndDoD(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
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
	cleanupSharedBackendSession(t, "file", dataRoot)
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
	if secondGoal.ID != firstGoal.ID {
		t.Fatalf("second Goal id = %q, want workspace canonical Goal %q", secondGoal.ID, firstGoal.ID)
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
	if item.Run.GoalID != secondGoal.ID || item.Run.DoDID != secondDoD.ID || item.Run.State != string(common.RunStateActive) {
		t.Fatalf("activation run = %+v, want latest goal/dod active", item.Run)
	}
	ownerRun := runtimeV2OwnerRunWithBackend(t, context.Background(), "file", dataRoot, workspace.ID, item.Run.ID)
	definition := ownerRun.RuntimeBootstrap.Definition
	if string(definition.GoalID) != secondGoal.ID || string(definition.DoDID) != secondDoD.ID {
		t.Fatalf("runtime definition = %+v, want latest goal/dod", definition)
	}
	goalText := definition.GoalDefinition
	if !strings.Contains(goalText, "first activation goal") || !strings.Contains(goalText, "second activation goal") {
		t.Fatalf("effective goal text = %q, want cumulative workspace goals", goalText)
	}
	if !strings.Contains(definition.DoDDefinition, "first activation dod") || !strings.Contains(definition.DoDDefinition, "second activation dod") {
		t.Fatalf("effective dod definition = %q, want cumulative workspace dods", definition.DoDDefinition)
	}

	item, err = ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: firstGoal.ID,
		DoDID:  firstDoD.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend explicit: %v", err)
	}
	if item.Run.GoalID != secondGoal.ID || item.Run.DoDID != secondDoD.ID || item.Run.State != string(common.RunStateActive) {
		t.Fatalf("activation run = %+v, want stale explicit goal/dod upgraded to latest definitions and active", item.Run)
	}
}

func TestActivateWorkspaceExecutionWithBackendUsesLatestDefinitionRevisionWithoutDuplicateLineage(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-definition-revision", "activation definition revision workspace", "", "workspaces/activation-definition-revision")
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
	if secondGoal.ID != firstGoal.ID {
		t.Fatalf("second Goal id = %q, want workspace canonical Goal %q", secondGoal.ID, firstGoal.ID)
	}
	firstDoD, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"first activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend first: %v", err)
	}
	secondDoD, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"second activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend second: %v", err)
	}
	if secondDoD.ID != firstDoD.ID {
		t.Fatalf("second DoD id = %q, want workspace canonical DoD %q", secondDoD.ID, firstDoD.ID)
	}
	updatedGoal, err := UpdateGoalWithBackend(context.Background(), "file", dataRoot, firstGoal.ID, "first activation goal revised")
	if err != nil {
		t.Fatalf("UpdateGoalWithBackend first: %v", err)
	}
	updatedDoD, err := UpdateDoDWithBackend(context.Background(), "file", dataRoot, firstDoD.ID, []string{"first activation dod revised"})
	if err != nil {
		t.Fatalf("UpdateDoDWithBackend first: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.GoalID != updatedGoal.ID || item.Run.DoDID != updatedDoD.ID || item.Run.State != string(common.RunStateActive) {
		t.Fatalf("activation run = %+v, want latest updated definition revision active", item.Run)
	}
	if item.Run.DefinitionRevision != "goal:"+updatedGoal.ID+"|dod:"+updatedDoD.ID {
		t.Fatalf("activation run definitionRevision = %q, want latest definition revision", item.Run.DefinitionRevision)
	}
	ownerRun := runtimeV2OwnerRunWithBackend(t, context.Background(), "file", dataRoot, workspace.ID, item.Run.ID)
	definition := ownerRun.RuntimeBootstrap.Definition
	if string(definition.GoalID) != updatedGoal.ID || string(definition.DoDID) != updatedDoD.ID {
		t.Fatalf("runtime definition = %+v, want latest goal/dod", definition)
	}
	if definition.GoalDefinition != "- first activation goal revised" {
		t.Fatalf("effective goal text = %q, want revised canonical goal only", definition.GoalDefinition)
	}
	if !strings.Contains(definition.DoDDefinition, "first activation dod revised") {
		t.Fatalf("effective dod definition = %q, want revised canonical dod only", definition.DoDDefinition)
	}
	impact := definition.ChangeImpact
	if impact == nil {
		t.Fatalf("definitionChangeImpact missing")
	}
	goalChange := impact.GoalChange
	doDChange := impact.DoDChange
	if goalChange.EffectiveDefinitionID != updatedGoal.ID || doDChange.EffectiveDefinitionID != updatedDoD.ID {
		t.Fatalf("definitionChangeImpact = %+v, want revised effective goal/dod", impact)
	}
	if got := goalChange.AllDefinitionIDs; len(got) != 1 || got[0] != updatedGoal.ID {
		t.Fatalf("allGoalIds = %+v, want canonical revised Goal only", got)
	}
	if got := doDChange.AllDefinitionIDs; len(got) != 1 || got[0] != updatedDoD.ID {
		t.Fatalf("allDoDIds = %+v, want canonical revised DoD only", got)
	}
	if !goalChange.ChangedInPlace || !doDChange.ChangedInPlace {
		t.Fatalf("definitionChangeImpact = %+v, want changedInPlace for updated definitions", impact)
	}
	if len(goalChange.DefinitionRevisions) != 1 || len(doDChange.DefinitionRevisions) != 1 {
		t.Fatalf("definitionChangeImpact = %+v, want one goal and one DoD revision record", impact)
	}
}

func TestBuildWorkspaceRuntimeActivationInjectsLatestRejectedResultContinuation(t *testing.T) {
	ctx := context.Background()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)
	workspace := &workspacemodel.Workspace{
		ID:    "ws-rejected-continuation",
		Title: "rejected continuation workspace",
		State: common.WorkspaceStateActive,
	}
	goal := &workspacemodel.Goal{
		ID:          "goal-rejected-continuation",
		WorkspaceID: workspace.ID,
		Text:        "deliver neutral coverage closure",
	}
	dod := &workspacemodel.DoD{
		ID:          "dod-rejected-continuation",
		WorkspaceID: workspace.ID,
		Items:       []string{"coverage closure is evidenced"},
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
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:          "run-old/result/dod-assessment",
		WorkspaceID: workspace.ID,
		RunID:       "run-old",
		GoalID:      goal.ID,
		DoDID:       dod.ID,
		State:       common.ResultStateRejected,
		Summary:     "old rejected result",
		UpdatedAt:   "2026-05-15T00:00:00Z",
	}); err != nil {
		t.Fatalf("create old result: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:           "run-latest/result/dod-assessment",
		WorkspaceID:  workspace.ID,
		RunID:        "run-latest",
		GoalID:       goal.ID,
		DoDID:        dod.ID,
		State:        common.ResultStateRejected,
		Summary:      "latest rejected result",
		SourceType:   "dod_assessment",
		SourceRef:    "run-latest/assessment",
		EvidenceRefs: []string{"run-latest/assessment", "run-latest/assessment"},
		ArtifactRefs: []common.ArtifactID{"artifact-latest"},
		DoDAssessment: resultmodel.DoDAssessment{
			Satisfied: false,
			Items: []resultmodel.DoDAssessmentItem{
				{Item: "completed DoD", State: "satisfied", EvidenceRefs: []string{"run-latest/validation"}},
				{Item: "missing DoD", State: "failed", FailureReason: "missing neutral evidence", EvidenceRefs: []string{"run-latest/assessment"}},
			},
			GoalCoverage: &resultmodel.DoDCoverage{Items: []resultmodel.DoDCoverageItem{
				{ID: "goal-1", Requirement: "goal leaf needs evidence", State: "failed"},
				{ID: "goal-2", Requirement: "covered goal leaf", State: "satisfied", EvidenceRefs: []string{"run-latest/validation"}},
			}},
			AcceptanceDimensions: &resultmodel.DoDCoverage{Items: []resultmodel.DoDCoverageItem{
				{ID: "neutral_dimension", State: "satisfied"},
			}},
			DeliverableCoverage: &resultmodel.DoDCoverage{Items: []resultmodel.DoDCoverageItem{
				{ID: "required_deliverable", Deliverable: "required deliverable", State: "satisfied"},
			}},
			Summary: "assessment summary",
		},
		SummaryDetail: resultmodel.ResultSummary{
			GoalOutcome:      "goal still missing coverage evidence",
			NextActions:      []string{"provide missing coverage evidence"},
			ResidualRisks:    []string{"coverage gap remains"},
			KnownLimitations: []string{"no raw stream should be copied"},
			EvidenceRefs:     []string{"run-latest/summary-evidence"},
			Diagnostics: mustAppserviceEventPayload(t, map[string]any{
				"gateFailureReason": "requires concrete neutral evidence",
				"runtimeDiagnostics": map[string]any{
					"kind":   "dod_evidence_quality_gap",
					"reason": "requires evidence aligned with the satisfied claim",
				},
			}),
		},
		UpdatedAt: "2026-05-16T00:00:00Z",
	}); err != nil {
		t.Fatalf("create latest result: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:          "run-other/result/dod-assessment",
		WorkspaceID: workspace.ID,
		RunID:       "run-other",
		GoalID:      "goal-other",
		DoDID:       dod.ID,
		State:       common.ResultStateAccepted,
		Summary:     "other goal result",
		UpdatedAt:   "2026-05-17T00:00:00Z",
	}); err != nil {
		t.Fatalf("create other result: %v", err)
	}

	build, err := buildWorkspaceRuntimeActivationWithDefinitions(ctx, session, workspace, goal, dod, ActivateWorkspaceExecutionCommand{
		LLMTarget: nil,
	})
	if err != nil {
		t.Fatalf("buildWorkspaceRuntimeActivationWithDefinitions: %v", err)
	}
	continuation := build.RuntimeActivation.Continuation.RejectedResult
	if continuation == nil || continuation.ResultID != "run-latest/result/dod-assessment" ||
		continuation.PredecessorRunID != "run-latest" ||
		continuation.ResultState != common.ResultStateRejected {
		t.Fatalf("rejectedResultContinuation = %+v, want latest neutral rejected result without runtime llm binding", continuation)
	}
	if len(continuation.SatisfiedDoDItems) != 1 || continuation.SatisfiedDoDItems[0].Item != "completed DoD" {
		t.Fatalf("satisfiedDoDItems = %+v, want completed DoD", continuation.SatisfiedDoDItems)
	}
	if len(continuation.UnsatisfiedDoDItems) != 1 || continuation.UnsatisfiedDoDItems[0].Item != "missing DoD" || continuation.UnsatisfiedDoDItems[0].Reason != "missing neutral evidence" {
		t.Fatalf("unsatisfiedDoDItems = %+v, want missing DoD reason", continuation.UnsatisfiedDoDItems)
	}
	if continuation.GateFailureReason == "" {
		t.Fatalf("gateFailureReason = %q, want propagated", continuation.GateFailureReason)
	}
	gapIDs := map[string]string{}
	for _, gap := range continuation.CoverageGaps {
		gapIDs[gap.Scope] = gap.ID
	}
	if gapIDs["goal"] != "goal-1" || gapIDs["acceptance"] != "neutral_dimension" || gapIDs["deliverable"] != "required_deliverable" {
		t.Fatalf("coverageGaps = %+v, want typed goal/acceptance/deliverable gaps", continuation.CoverageGaps)
	}
	if len(continuation.RequiredEvidence) != 3 {
		t.Fatalf("requiredEvidence = %#v, want next actions, residual risks, known limitations", continuation.RequiredEvidence)
	}
	if len(continuation.EvidenceRefs) != 2 || continuation.EvidenceRefs[0] != "run-latest/assessment" || continuation.EvidenceRefs[1] != "run-latest/summary-evidence" {
		t.Fatalf("evidenceRefs = %#v, want compact unique result and summary refs", continuation.EvidenceRefs)
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
			Payload:  mustAppserviceGovernancePayload(t, map[string]any{"resumeInstruction": "continue with the generic plan"}),
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
	exports := testRuntimeExports(session.Bootstrap)
	workspace := &workspacemodel.Workspace{
		ID:    "ws-activation-definition-reuse",
		Title: "activation definition reuse",
		State: common.WorkspaceStateActive,
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
	countingHub := &countingActivationEventHub{base: exports.Infrastructure.CommandHub}
	exports.Infrastructure.CommandHub = countingHub
	borrowed, err := NewRuntimeSessionFromExports(completeAppserviceTestExports(exports))
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}

	selectedGoal, selectedDoD, err := activationports.SelectWorkspaceExecutionDefinitions(ctx, exports.Infrastructure.CommandHub, workspace.ID, "", "")
	if err != nil {
		t.Fatalf("activationports.SelectWorkspaceExecutionDefinitions: %v", err)
	}
	if _, err := buildWorkspaceRuntimeActivationWithDefinitions(ctx, borrowed, workspace, selectedGoal, selectedDoD, ActivateWorkspaceExecutionCommand{}); err != nil {
		t.Fatalf("buildWorkspaceRuntimeActivationWithDefinitions: %v", err)
	}
	if countingHub.Count(workspaceevents.TopicWorkspaceCommandListGoalsByWS) != 2 || countingHub.Count(workspaceevents.TopicWorkspaceCommandListDoDsByWS) != 2 {
		t.Fatalf("definition list commands goals=%d dods=%d, want selection plus effective definition lists",
			countingHub.Count(workspaceevents.TopicWorkspaceCommandListGoalsByWS),
			countingHub.Count(workspaceevents.TopicWorkspaceCommandListDoDsByWS))
	}
	if countingHub.Count(workspaceevents.TopicWorkspaceCommandGetGoal) != 0 || countingHub.Count(workspaceevents.TopicWorkspaceCommandGetDoD) != 0 {
		t.Fatalf("definition get commands goals=%d dods=%d, want no definition reload",
			countingHub.Count(workspaceevents.TopicWorkspaceCommandGetGoal),
			countingHub.Count(workspaceevents.TopicWorkspaceCommandGetDoD))
	}
}

func TestActivationContextMergesOperatorInputPatchWithoutOverridingDefinitions(t *testing.T) {
	ctx := context.Background()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)
	workspace := &workspacemodel.Workspace{
		ID:    "ws-activation-operator-input",
		Title: "activation operator input",
		State: common.WorkspaceStateActive,
	}
	goal := &workspacemodel.Goal{
		ID:          "goal-activation-operator-input",
		WorkspaceID: workspace.ID,
		Text:        "existing cumulative goal",
	}
	nextGoal := &workspacemodel.Goal{
		ID:          "goal-activation-operator-input-next",
		WorkspaceID: workspace.ID,
		Text:        "new cumulative goal",
	}
	dod := &workspacemodel.DoD{
		ID:          "dod-activation-operator-input",
		WorkspaceID: workspace.ID,
		Items:       []string{"existing dod"},
		State:       common.DoDStateReady,
	}
	nextDoD := &workspacemodel.DoD{
		ID:          "dod-activation-operator-input-next",
		WorkspaceID: workspace.ID,
		Items:       []string{"new dod"},
		State:       common.DoDStateReady,
	}
	if err := exports.Repositories.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, nextGoal); err != nil {
		t.Fatalf("create next goal: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, dod); err != nil {
		t.Fatalf("create dod: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, nextDoD); err != nil {
		t.Fatalf("create next dod: %v", err)
	}

	build, err := buildWorkspaceRuntimeActivationWithDefinitions(ctx, session, workspace, goal, dod, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("buildWorkspaceRuntimeActivationWithDefinitions: %v", err)
	}
	if build.EffectiveGoal == nil || build.EffectiveGoal.ID != nextGoal.ID {
		t.Fatalf("effective goal = %+v, want protected latest goal", build.EffectiveGoal)
	}
	impact := build.DefinitionChangeImpact
	if impact == nil || !impact.Changed || impact.Policy != "workspace_goal_dod_cumulative_latest_refines_previous" {
		t.Fatalf("definitionChangeImpact = %+v, want runtime-generated cumulative impact", impact)
	}
	goalChange := impact.GoalChange
	if goalChange.EffectiveDefinitionID != string(nextGoal.ID) || !stringSliceContains(goalChange.PreviousDefinitionIDs, string(goal.ID)) {
		t.Fatalf("goalChange = %+v, want latest goal with previous goal ids", goalChange)
	}
	doDChange := impact.DoDChange
	if doDChange.EffectiveDefinitionID != string(nextDoD.ID) || !stringSliceContains(doDChange.PreviousDefinitionIDs, string(dod.ID)) {
		t.Fatalf("doDChange = %+v, want latest DoD with previous DoD ids", doDChange)
	}
}

func TestActivationDefinitionSelectionReadsGoalAndDoDConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)
	workspace := &workspacemodel.Workspace{
		ID:    "ws-activation-definition-parallel",
		Title: "activation definition parallel",
		State: common.WorkspaceStateActive,
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
	exports.Infrastructure.CommandHub = &blockingActivationEventHub{
		base:    exports.Infrastructure.CommandHub,
		started: started,
		release: release,
		labels: map[string]string{
			workspaceevents.TopicWorkspaceCommandListGoalsByWS: "goal",
			workspaceevents.TopicWorkspaceCommandListDoDsByWS:  "dod",
		},
	}
	errCh := make(chan error, 1)
	go func() {
		selectedGoal, selectedDoD, err := activationports.SelectWorkspaceExecutionDefinitions(ctx, exports.Infrastructure.CommandHub, workspace.ID, "", "")
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
			t.Fatalf("activationports.SelectWorkspaceExecutionDefinitions returned before both reads started: %v", err)
		case <-ctx.Done():
			t.Fatalf("started reads = %+v, want goal and dod before release: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("activationports.SelectWorkspaceExecutionDefinitions: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("activationports.SelectWorkspaceExecutionDefinitions did not complete after release: %v", ctx.Err())
	}
}

func TestActivationResponseReusesRunSnapshotRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)
	workspace := &workspacemodel.Workspace{
		ID: "ws-activation-run-snapshot-reuse", Title: "activation run snapshot reuse", State: common.WorkspaceStateActive,
		ContextHints: runtimeV2TestContextHints(t),
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
	countingHub := &countingActivationEventHub{base: exports.Infrastructure.CommandHub}
	exports.Infrastructure.CommandHub = countingHub
	borrowed, err := NewRuntimeSessionFromExports(completeAppserviceTestExports(exports))
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
	if countingHub.Count(runevents.TopicRunCommandGet) != 1 {
		t.Fatalf("run get commands = %d, want RunSnapshot read only", countingHub.Count(runevents.TopicRunCommandGet))
	}
}

func TestActivateWorkspaceExecutionCreatesRunWithTypedRuntimeBootstrap(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)
	workspace := &workspacemodel.Workspace{
		ID: "ws-activation-inline-compressed", Title: "activation inline compressed", State: common.WorkspaceStateActive,
		ContextHints: runtimeV2TestContextHints(t),
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
	var createdRuns []*runmodel.Run
	countingHub := &countingActivationEventHub{base: exports.Infrastructure.CommandHub}
	countingHub.onSend = func(event magicevent.Event) {
		if event.ID() != runevents.TopicRunCommandCreate {
			return
		}
		cmd, ok := event.Data().(runmodule.CreateRunCommand)
		if ok {
			createdRuns = append(createdRuns, &runmodel.Run{
				ID:                 cmd.RunID,
				WorkspaceID:        cmd.WorkspaceID,
				GoalID:             cmd.GoalID,
				DoDID:              cmd.DoDID,
				DefinitionRevision: cmd.DefinitionRevision,
				InitialStepKind:    cmd.InitialStepKind,
				RuntimeBootstrap:   cmd.RuntimeBootstrap,
				State:              common.RunStatePending,
				Priority:           cmd.Priority,
			})
		}
	}
	exports.Infrastructure.CommandHub = countingHub
	exports.Repositories.Runs = countingRuns
	borrowed, err := NewRuntimeSessionFromExports(completeAppserviceTestExports(exports))
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}

	_, err = ActivateWorkspaceExecutionWithSession(ctx, borrowed, string(workspace.ID), ActivateWorkspaceExecutionCommand{Priority: 7})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithSession: %v", err)
	}
	if len(createdRuns) != 1 {
		t.Fatalf("created run events = %d, want one activation run payload", len(createdRuns))
	}
	createdRun := createdRuns[0]
	if createdRun.RuntimeBootstrap == nil || createdRun.RuntimeBootstrap.Definition.Revision != "goal:"+string(goal.ID)+"|dod:"+string(dod.ID) {
		t.Fatalf("created run runtime bootstrap = %+v, want typed bootstrap", createdRun.RuntimeBootstrap)
	}
	if createdRun.GoalID != goal.ID || createdRun.DoDID != dod.ID {
		t.Fatalf("created run goal/dod = %s/%s, want activation definitions", createdRun.GoalID, createdRun.DoDID)
	}
	if createdRun.DefinitionRevision != "goal:"+string(goal.ID)+"|dod:"+string(dod.ID) {
		t.Fatalf("created run definitionRevision = %q, want activation definition revision", createdRun.DefinitionRevision)
	}
	if createdRun.Priority != 7 {
		t.Fatalf("create priority = %d, want activation priority", createdRun.Priority)
	}
	if len(countingRuns.createRuns) > 0 {
		persisted := countingRuns.createRuns[0]
		if persisted.RuntimeBootstrap == nil || persisted.RuntimeBootstrap.Definition.Revision != createdRun.DefinitionRevision {
			t.Fatalf("repository create runtime bootstrap = %+v, want typed bootstrap", persisted.RuntimeBootstrap)
		}
	}
	persistedWorkspace, err := exports.Repositories.Workspaces.Get(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("read persisted workspace: %v", err)
	}
	if persistedWorkspace.DefaultLLMTarget == nil || !persistedWorkspace.DefaultLLMTarget.Same(createdRun.RuntimeBootstrap.Activation.ProviderTarget) {
		t.Fatalf("workspace default target = %+v, want first sealed run target %+v", persistedWorkspace.DefaultLLMTarget, createdRun.RuntimeBootstrap.Activation.ProviderTarget)
	}
}

func TestActivationResponseWorkspaceSnapshotReusesRunSnapshotWorkspace(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)
	workspace := &workspacemodel.Workspace{
		ID: "ws-activation-workspace-snapshot-reuse", Title: "activation workspace snapshot reuse", State: common.WorkspaceStateActive,
		ContextHints: runtimeV2TestContextHints(t),
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
	borrowed, err := NewRuntimeSessionFromExports(completeAppserviceTestExports(exports))
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
	if countingWorkspaces.getCalls != 0 {
		t.Fatalf("workspace repository Get calls = %d, want appservice workspace reads through owner EventHub command", countingWorkspaces.getCalls)
	}
	if len(item.Snapshot.Runs) != 1 || item.Snapshot.Runs[0].ID != item.Run.ID {
		t.Fatalf("activation snapshot runs = %+v, want only current activation run", item.Snapshot.Runs)
	}
	if len(item.Snapshot.EventRecords) != 0 ||
		len(item.Snapshot.Results) != 0 ||
		len(item.Snapshot.Artifacts) != 0 ||
		len(item.Snapshot.Decisions) != 0 ||
		len(item.Snapshot.Approvals) != 0 {
		t.Fatalf("activation snapshot = %+v, want lightweight activation projection without heavy workspace evidence", item.Snapshot)
	}
}

func TestBuildWorkspaceRuntimeActivationStartsIndependentReadsConcurrently(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)
	workspace := &workspacemodel.Workspace{
		ID:            "ws-activation-context-parallel",
		Title:         "activation context parallel",
		State:         common.WorkspaceStateActive,
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
	exports.Infrastructure.CommandHub = &blockingActivationEventHub{
		base:    exports.Infrastructure.CommandHub,
		started: started,
		release: release,
		labels: map[string]string{
			soulevents.TopicSoulCommandGet:                     "soul",
			memoryevents.TopicMemoryCommandListByWorkspace:     "memory",
			decisionevents.TopicDecisionCommandListTicketsByWS: "governance",
		},
	}
	borrowed, err := NewRuntimeSessionFromExports(completeAppserviceTestExports(exports))
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}

	var completed atomic.Bool
	errCh := make(chan error, 1)
	go func() {
		_, err := buildWorkspaceRuntimeActivationWithDefinitions(ctx, borrowed, workspace, goal, dod, ActivateWorkspaceExecutionCommand{})
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
			t.Fatalf("buildWorkspaceRuntimeActivationWithDefinitions: %v", err)
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
	exports := testRuntimeExports(session.Bootstrap)
	workspace := &workspacemodel.Workspace{
		ID:    "ws-activation-mcp-parallel",
		Title: "activation mcp parallel",
		State: common.WorkspaceStateActive,
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
	if err := createTestMCPServerWithExports(ctx, exports, mcpmodule.CreateServerCommand{
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
	exports.Infrastructure.CommandHub = &blockingActivationEventHub{
		base:    exports.Infrastructure.CommandHub,
		started: started,
		release: release,
		labels: map[string]string{
			mcpevents.TopicMCPCommandGetServer:     "mcp-server",
			mcpevents.TopicMCPCommandGetCapability: "mcp-capability",
		},
	}
	borrowed, err := NewRuntimeSessionFromExports(completeAppserviceTestExports(exports))
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := buildWorkspaceRuntimeActivationWithDefinitions(ctx, borrowed, workspace, goal, dod, ActivateWorkspaceExecutionCommand{
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
			t.Fatalf("buildWorkspaceRuntimeActivationWithDefinitions returned before both MCP reads started: %v", err)
		case <-ctx.Done():
			t.Fatalf("started reads = %+v, want mcp-server and mcp-capability before release: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("buildWorkspaceRuntimeActivationWithDefinitions: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("activation context did not complete after release: %v", ctx.Err())
	}
}

func TestWorkspaceSoulBindingsWithBackendRoutesExecutionSoulByStage(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
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
	activation := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Activation
	if activation.DefaultSoul == nil || string(activation.DefaultSoul.ID) != "soul-default" {
		t.Fatalf("runtime activation default soul = %+v, want workspace default", activation.DefaultSoul)
	}
	assertRuntimeV2StageSoulBinding(t, activation, SoulBindingStagePlanning, "soul-default", "workspace_default")
	assertRuntimeV2StageSoulBinding(t, activation, SoulBindingStageExecution, "soul-execution", "workspace_stage_binding")
	assertRuntimeV2StageSoulBinding(t, activation, SoulBindingStageDecision, "soul-decision", "workspace_stage_binding")
	assertRuntimeV2StageSoulBinding(t, activation, SoulBindingStageValidation, "soul-validation", "run_stage_override")
}

func TestActivateWorkspaceExecutionUsesInjectedConfigLoader(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("runtime exports: %v", err)
	}
	exports.Infrastructure.LoadConfig = func() (workorchconfig.Config, error) {
		return workorchconfig.Config{
			Server:  workorchconfig.ServerConfig{DefaultDialogLanguage: "zh-CN"},
			Runtime: workorchconfig.RuntimeConfig{DefaultSoulID: "soul-injected-default"},
		}, nil
	}
	if err := exports.Repositories.Souls.Create(ctx, &soulmodel.Soul{
		ID:     "soul-injected-default",
		Name:   "Injected Default",
		SoulMD: "# Injected Default",
	}); err != nil {
		t.Fatalf("create soul: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-injected-config")
	goalID := common.GoalID("goal-injected-config")
	dodID := common.DoDID("dod-injected-config")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID: workspaceID, Title: "injected config workspace", State: common.WorkspaceStateActive,
		ContextHints: runtimeV2TestContextHints(t),
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, &workspacemodel.Goal{
		ID:          goalID,
		WorkspaceID: workspaceID,
		Text:        "use injected config",
	}); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, &workspacemodel.DoD{
		ID:          dodID,
		WorkspaceID: workspaceID,
		Items:       []string{"use injected config"},
		State:       common.DoDStateReady,
	}); err != nil {
		t.Fatalf("create dod: %v", err)
	}
	borrowed, err := NewRuntimeSessionFromExports(completeAppserviceTestExports(exports))
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}
	hub, err := borrowed.RequiredCommandHub()
	if err != nil {
		t.Fatalf("RequiredCommandHub: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithSession(ctx, borrowed, string(workspaceID), ActivateWorkspaceExecutionCommand{
		GoalID: string(goalID),
		DoDID:  string(dodID),
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithSession: %v", err)
	}
	ownerRun, err := runports.ReadRun(ctx, hub, common.RunID(item.Run.ID))
	if err != nil {
		t.Fatalf("ReadRun: %v", err)
	}
	activation := ownerRun.RuntimeBootstrap.Activation
	if activation.DialogLanguage != "zh-CN" {
		t.Fatalf("dialogLanguage = %q, want zh-CN", activation.DialogLanguage)
	}
	if activation.DefaultSoul == nil || string(activation.DefaultSoul.ID) != "soul-injected-default" {
		t.Fatalf("runtime activation default soul = %+v, want injected default soul", activation.DefaultSoul)
	}
}

func TestActivateWorkspaceExecutionWithBackendAddsWorkspaceBoundEvidencePolicy(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
		ID:     "soul-evidence-validation",
		Name:   "Evidence Validation",
		SoulMD: "# Evidence Validation\n\nRequire valid evidence.",
	}); err != nil {
		t.Fatalf("CreateSoulWithBackend: %v", err)
	}
	if _, err := createRuntimeV2TestTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
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
		Title:                "evidence policy workspace",
		DeliverableDirectory: "workspaces/evidence-policy",
		TemplateID:           "template-evidence-policy",
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
	policies := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Activation.EvidencePolicies
	if len(policies) != 1 || policies[0].Profile != "template-defined" || policies[0].Rule != "zero_work_is_inconclusive" {
		t.Fatalf("evidencePolicies = %+v, want template-bound zero-work policy", policies)
	}
}

func TestActivateWorkspaceExecutionWithBackendAddsExecutionContractFromWorkspaceContextHints(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
		ID:     "soul-contract-execution",
		Name:   "Contract Execution",
		SoulMD: "# Contract Execution\n\nApply runtime contract.",
	}); err != nil {
		t.Fatalf("CreateSoulWithBackend: %v", err)
	}
	if _, err := createRuntimeV2TestTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:   "template-state-change-contract",
		Name: "State Change Contract Template",
		SoulBindings: map[string]string{
			SoulBindingStageExecution: "soul-contract-execution",
		},
		ContextHints: map[string]any{
			"stateChangePolicy": "must_produce_state_change",
			"profileContracts": map[string]any{
				"software-dev": map[string]any{
					"acceptanceDimensions": []any{
						map[string]any{"id": "runtime_entrypoint", "required": true},
					},
				},
			},
			"capabilityUsagePolicies": []any{
				map[string]any{
					"id":                "software-dev-code-semantics-guidance",
					"appliesToCategory": "code_semantics",
					"usageMode":         "prefer_when_triggered",
				},
			},
			"templateStagePolicies": map[string]any{
				"schemaVersion": "workorch.template_stage_policy.v1",
				"stages": []any{
					map[string]any{
						"id":          "execution",
						"soulBinding": "execution",
						"entry": map[string]any{
							"nextAction":                 "state_change",
							"requiresDefinitionRevision": true,
							"requiresGapBaseline":        true,
						},
						"requiredOutputs": []any{
							map[string]any{
								"kind":          "state_change",
								"evidenceKinds": []any{"state_change"},
							},
						},
						"completion": map[string]any{
							"policy": "required_outputs_accepted",
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}
	workspace, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:                "contract workspace",
		DeliverableDirectory: "workspaces/contract-workspace",
		TemplateID:           "template-state-change-contract",
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
	contract := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Activation.ExecutionContract
	if contract.StateChangePolicy != runtimecontract.RuntimeStateChangeMustProduce {
		t.Fatalf("executionContract = %+v, want must_produce_state_change", contract)
	}
	policies := contract.CapabilityUsagePolicies
	if len(policies) != 1 || policies[0].ID != "software-dev-code-semantics-guidance" {
		t.Fatalf("capabilityUsagePolicies = %+v, want software-dev code semantics policy", policies)
	}
	softwareDev := runtimeV2ProfileContract(t, contract, "software-dev")
	dimensions := softwareDev.AcceptanceDimensions
	if len(dimensions) != 1 || dimensions[0].ID != "runtime_entrypoint" || !dimensions[0].Required {
		t.Fatalf("profile contract dimensions = %+v, want runtime_entrypoint required", dimensions)
	}
	stagePolicy := contract.TemplateStagePolicy
	if stagePolicy == nil || stagePolicy.SchemaVersion != "workorch.template_stage_policy.v1" {
		t.Fatalf("templateStagePolicies = %+v, want schema v1", stagePolicy)
	}
	if len(stagePolicy.Stages) != 1 || stagePolicy.Stages[0].ID != "execution" {
		t.Fatalf("templateStagePolicies.stages = %+v, want execution stage", stagePolicy.Stages)
	}
}

func TestActivateWorkspaceExecutionAppliesTemplateDefaultMCPContext(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	session, err := newRuntimeSessionWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("newRuntimeSessionWithBackend: %v", err)
	}
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	if err := createTestMCPServerWithExports(ctx, exports, mcpmodule.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-template-default"),
		Name:          "Template Default MCP",
		Transport:     "stdio",
		Command:       "template-tool",
		Env:           map[string]string{"CONFIG_PATH": "{workspaceInternalDirectory}/tool/config.json"},
		Enabled:       true,
		AllowedTools:  []string{"tool.lookup"},
		TimeoutMillis: 1000,
	}); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if err := updateTestMCPCapabilityWithExports(ctx, exports, mcpevents.UpdateCapabilityCommand{
		ID:        common.MCPCapabilityID("mcp-template-default:tool:tool.lookup"),
		ServerID:  common.MCPServerID("mcp-template-default"),
		Kind:      "tool",
		Name:      "tool.lookup",
		Category:  "semantic",
		Enabled:   true,
		RiskLevel: "read_only",
	}); err != nil {
		t.Fatalf("CreateCapability: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close setup session: %v", err)
	}

	if _, err := createRuntimeV2TestTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:   "template-default-mcp",
		Name: "Default MCP Template",
		ContextHints: map[string]any{
			"semanticCodeNavigation": map[string]any{
				"defaultMCPServerIds": []any{"mcp-template-default"},
				"defaultCapabilityIds": []any{
					"mcp-template-default:tool:tool.lookup",
				},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}
	workspace, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:      "default mcp workspace",
		TemplateID: "template-default-mcp",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "use default mcp context")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"default mcp is visible"})
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
	activation := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Activation
	if !runtimeV2HasMCPServer(activation, "mcp-template-default") {
		t.Fatalf("runtime MCP server IDs = %+v, want template default server", activation.MCPServerIDs)
	}
	if len(activation.MCPCapabilities) != 1 || string(activation.MCPCapabilities[0].ServerID) != "mcp-template-default" {
		t.Fatalf("runtime MCP capabilities = %+v, want template default capability", activation.MCPCapabilities)
	}
}

func TestActivateWorkspaceExecutionDoesNotSelectMCPWithoutTemplateOrWorkspaceDefaults(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	if _, err := createRuntimeV2TestTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:           "template-no-mcp-default",
		Name:         "No MCP Default Template",
		ContextHints: map[string]any{},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}
	workspace, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:      "no mcp default workspace",
		TemplateID: "template-no-mcp-default",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "run without template mcp defaults")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"no implicit mcp defaults"})
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
	activation := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Activation
	if len(activation.MCPServerIDs) != 0 || len(activation.MCPCapabilities) != 0 {
		t.Fatalf("runtime MCP selection = %+v/%+v, want no selected MCP", activation.MCPServerIDs, activation.MCPCapabilities)
	}
}

func TestActivateWorkspaceExecutionUsesUpdatedTemplateMCPDefaultWhenWorkspaceHasNoOverride(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	session, err := newRuntimeSessionWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("newRuntimeSessionWithBackend: %v", err)
	}
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	for _, id := range []string{"mcp-template-original", "mcp-template-refreshed"} {
		if err := createTestMCPServerWithExports(ctx, exports, mcpmodule.CreateServerCommand{
			ServerID:     common.MCPServerID(id),
			Name:         id,
			Transport:    "stdio",
			Command:      id,
			Enabled:      true,
			AllowedTools: []string{"tool.lookup"},
		}); err != nil {
			t.Fatalf("CreateServer %s: %v", id, err)
		}
		if err := updateTestMCPCapabilityWithExports(ctx, exports, mcpevents.UpdateCapabilityCommand{
			ID:        common.MCPCapabilityID(id + ":tool:tool.lookup"),
			ServerID:  common.MCPServerID(id),
			Kind:      "tool",
			Name:      "tool.lookup",
			Enabled:   true,
			RiskLevel: "read_only",
		}); err != nil {
			t.Fatalf("CreateCapability %s: %v", id, err)
		}
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close setup session: %v", err)
	}

	templateID := "template-mcp-refresh"
	if _, err := createRuntimeV2TestTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:   templateID,
		Name: "MCP Refresh Template",
		ContextHints: map[string]any{
			"semanticCodeNavigation": map[string]any{
				"defaultMCPServerIds":  []any{"mcp-template-original"},
				"defaultCapabilityIds": []any{"mcp-template-original:tool:tool.lookup"},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}
	workspace, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:      "template mcp refresh workspace",
		TemplateID: templateID,
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	if _, err := UpdateWorkspaceTemplateWithBackend(ctx, "file", dataRoot, templateID, WorkspaceTemplate{
		ID:   templateID,
		Name: "MCP Refresh Template",
		ContextHints: map[string]any{
			"semanticCodeNavigation": map[string]any{
				"defaultMCPServerIds":  []any{"mcp-template-refreshed"},
				"defaultCapabilityIds": []any{"mcp-template-refreshed:tool:tool.lookup"},
			},
		},
	}); err != nil {
		t.Fatalf("UpdateWorkspaceTemplateWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "use refreshed template mcp")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"refreshed template mcp is selected"})
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
	activation := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Activation
	if !runtimeV2HasMCPServer(activation, "mcp-template-refreshed") || runtimeV2HasMCPServer(activation, "mcp-template-original") {
		t.Fatalf("runtime MCP server IDs = %+v, want refreshed template default without stale workspace copy", activation.MCPServerIDs)
	}
}

func TestActivateWorkspaceExecutionUsesWorkspaceMCPDefaultOverride(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	session, err := newRuntimeSessionWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("newRuntimeSessionWithBackend: %v", err)
	}
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	for _, id := range []string{"mcp-template-common", "mcp-workspace-override"} {
		if err := createTestMCPServerWithExports(ctx, exports, mcpmodule.CreateServerCommand{
			ServerID:     common.MCPServerID(id),
			Name:         id,
			Transport:    "stdio",
			Command:      id,
			Enabled:      true,
			AllowedTools: []string{"tool.lookup"},
		}); err != nil {
			t.Fatalf("CreateServer %s: %v", id, err)
		}
		if err := updateTestMCPCapabilityWithExports(ctx, exports, mcpevents.UpdateCapabilityCommand{
			ID:        common.MCPCapabilityID(id + ":tool:tool.lookup"),
			ServerID:  common.MCPServerID(id),
			Kind:      "tool",
			Name:      "tool.lookup",
			Enabled:   true,
			RiskLevel: "read_only",
		}); err != nil {
			t.Fatalf("CreateCapability %s: %v", id, err)
		}
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close setup session: %v", err)
	}

	templateID := "template-mcp-workspace-override"
	if _, err := createRuntimeV2TestTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:   templateID,
		Name: "MCP Workspace Override Template",
		ContextHints: map[string]any{
			"semanticCodeNavigation": map[string]any{
				"defaultMCPServerIds":  []any{"mcp-template-common"},
				"defaultCapabilityIds": []any{"mcp-template-common:tool:tool.lookup"},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}
	workspace, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:      "workspace mcp override",
		TemplateID: templateID,
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	session, err = newRuntimeSessionWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("newRuntimeSessionWithBackend #2: %v", err)
	}
	exports, err = testSessionExports(session)
	if err != nil {
		t.Fatalf("exports #2: %v", err)
	}
	workspaceModel, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceModel.ContextHints = mustAppserviceEventPayload(t, map[string]any{
		"semanticCodeNavigation": map[string]any{
			"defaultMCPServerIds":  []any{"mcp-workspace-override"},
			"defaultCapabilityIds": []any{"mcp-workspace-override:tool:tool.lookup"},
		},
	})
	if err := exports.Repositories.Workspaces.Update(ctx, workspaceModel); err != nil {
		t.Fatalf("update workspace override: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close override session: %v", err)
	}

	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "use workspace mcp override")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"workspace mcp override is selected"})
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
	activation := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Activation
	if !runtimeV2HasMCPServer(activation, "mcp-workspace-override") || runtimeV2HasMCPServer(activation, "mcp-template-common") {
		t.Fatalf("runtime MCP server IDs = %+v, want workspace override without template default", activation.MCPServerIDs)
	}
}

func TestActivateWorkspaceExecutionRepairsMissingManagedWorkspaceConfig(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	if _, err := createRuntimeV2TestTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:   "template-managed-config-activation",
		Name: "Managed Config Activation Template",
		ContextHints: map[string]any{
			"managedWorkspaceConfigs": []any{
				map[string]any{
					"path":         "lsp-bridge/mcp-config.json",
					"format":       "json",
					"mergeMissing": true,
					"content": map[string]any{
						"languages": map[string]any{
							"go": map[string]any{
								"command": []any{"gopls", "serve"},
							},
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}
	workspace, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:      "activation managed config workspace",
		TemplateID: "template-managed-config-activation",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	workspaceConfigPath := filepath.Join(workspace.InternalDirectory, "lsp-bridge", "mcp-config.json")
	systemConfigPath := filepath.Join(dataRoot, "system", "managed-configs", "lsp-bridge", "mcp-config.json")
	if _, err := os.Stat(workspaceConfigPath); err == nil {
		t.Fatalf("workspace managed config was created before activation: %s", workspaceConfigPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat workspace managed config before activation: %v", err)
	}
	if _, err := os.Stat(systemConfigPath); err == nil {
		t.Fatalf("system managed config was created before activation: %s", systemConfigPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat system managed config before activation: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "repair missing managed config")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"managed config exists"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: goal.ID,
		DoDID:  dod.ID,
	}); err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if _, err := os.Stat(workspaceConfigPath); err == nil {
		t.Fatalf("workspace managed config was created for template default: %s", workspaceConfigPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat workspace managed config after activation: %v", err)
	}
	if _, err := os.Stat(systemConfigPath); err != nil {
		t.Fatalf("system managed config stat after activation: %v", err)
	}
}

func TestActivationExecutionContractUsesWorkspaceTemplateIDBeforeSoulRouting(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	templateID := "template-explicit-contract"
	if _, err := createRuntimeV2TestTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:   templateID,
		Name: "explicit contract",
		ContextHints: map[string]any{
			"stateChangePolicy": "must_produce_state_change",
			"runtimeRepair": map[string]any{
				"prerequisiteManifestTargetPatterns": []string{"manifest.json", "lockfile.*"},
			},
			"profileContracts": map[string]any{
				"explicit-profile": map[string]any{
					"deliverableExpectations": []map[string]any{{
						"id":       "explicit-deliverable",
						"required": true,
					}},
				},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}
	workspace, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:                "explicit template workspace",
		DeliverableDirectory: "workspaces/explicit-template-workspace",
		TemplateID:           templateID,
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	if workspace.TemplateID != templateID || workspace.ProfileID != templateID {
		t.Fatalf("workspace template/profile = %q/%q, want %q", workspace.TemplateID, workspace.ProfileID, templateID)
	}

	// Mutate routing after creation. The execution contract must still come from
	// TemplateID rather than being lost because Soul routing does not match.
	session, err := newRuntimeSessionWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("newRuntimeSessionWithBackend: %v", err)
	}
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	item, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	item.DefaultSoulID = "soul-mutated"
	if err := exports.Repositories.Workspaces.Update(ctx, item); err != nil {
		t.Fatalf("update workspace: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close session: %v", err)
	}

	session, err = newRuntimeSessionWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("newRuntimeSessionWithBackend #2: %v", err)
	}
	exports, err = testSessionExports(session)
	if err != nil {
		t.Fatalf("exports #2: %v", err)
	}
	workspaceModel, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace #2: %v", err)
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		t.Fatalf("RequiredCommandHub: %v", err)
	}
	contract := activationports.WorkspaceExecutionContract(ctx, hub, workspaceModel)
	if contract["stateChangePolicy"] != "must_produce_state_change" {
		t.Fatalf("executionContract = %+v, want template policy", contract)
	}
	runtimeRepair := contextMapFromValue(t, contract["runtimeRepair"])
	if !contextStringSliceContains(runtimeRepair["prerequisiteManifestTargetPatterns"], "manifest.json") ||
		!contextStringSliceContains(runtimeRepair["prerequisiteManifestTargetPatterns"], "lockfile.*") {
		t.Fatalf("runtimeRepair = %+v, want refreshed template prerequisite manifest patterns", runtimeRepair)
	}
	profiles := contextMapFromValue(t, contract["profileContracts"])
	if _, ok := profiles["explicit-profile"]; !ok {
		t.Fatalf("profileContracts = %+v, want explicit-profile from TemplateID", profiles)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close session #2: %v", err)
	}
}

func TestActivateWorkspaceExecutionWithBackendRefreshesExecutionContractFromMatchedTemplate(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
		ID:     "soul-contract-refresh-execution",
		Name:   "Contract Refresh Execution",
		SoulMD: "# Contract Refresh Execution\n\nApply refreshed runtime contract.",
	}); err != nil {
		t.Fatalf("CreateSoulWithBackend: %v", err)
	}
	templateID := "template-refresh-contract"
	if _, err := createRuntimeV2TestTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:   templateID,
		Name: "Refresh Contract Template",
		SoulBindings: map[string]string{
			SoulBindingStageExecution: "soul-contract-refresh-execution",
		},
		ContextHints: map[string]any{
			"stateChangePolicy": "must_produce_state_change",
			"profileContracts": map[string]any{
				"neutral-profile": map[string]any{
					"deliverableExpectations": []map[string]any{
						{"id": "source_change", "required": false},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}
	workspace, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:                "refresh contract workspace",
		DeliverableDirectory: "workspaces/refresh-contract-workspace",
		TemplateID:           templateID,
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	if _, err := UpdateWorkspaceTemplateWithBackend(ctx, "file", dataRoot, templateID, WorkspaceTemplate{
		ID:   templateID,
		Name: "Refresh Contract Template",
		SoulBindings: map[string]string{
			SoulBindingStageExecution: "soul-contract-refresh-execution",
		},
		ContextHints: map[string]any{
			"stateChangePolicy": "must_produce_state_change",
			"profileContracts": map[string]any{
				"neutral-profile": map[string]any{
					"deliverableExpectations": []map[string]any{
						{"id": "source_change", "required": true},
					},
					"evidenceRequirements": []map[string]any{
						{
							"id":                "state_change_evidence",
							"scope":             "acceptanceDimension",
							"acceptedStepKinds": []string{"shell/command", "patch/apply"},
							"rejectedStepKinds": []string{"file/read"},
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("UpdateWorkspaceTemplateWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "deliver refreshed contract behavior")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"refreshed contract is used"})
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

	contract := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Activation.ExecutionContract
	profile := runtimeV2ProfileContract(t, contract, "neutral-profile")
	deliverables := profile.DeliverableExpectations
	if len(deliverables) != 1 || deliverables[0].ID != "source_change" || !deliverables[0].Required {
		t.Fatalf("deliverableExpectations = %+v, want refreshed required=true contract", deliverables)
	}
	requirements := profile.EvidenceRequirements
	if len(requirements) != 1 || requirements[0].ID != "state_change_evidence" {
		t.Fatalf("evidenceRequirements = %+v, want refreshed evidence requirements", requirements)
	}
}

func TestActivateGenericGovernedWorkspaceDoesNotInheritSoftwareDevStrategy(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	workspace, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:      "generic governed isolation workspace",
		TemplateID: "generic-governed",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "complete a generic governed task")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"generic result is traceable"})
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

	activation := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Activation
	contract := activation.ExecutionContract
	if contract.StateChangePolicy != runtimecontract.RuntimeStateChangeAllowedButNotRequired {
		t.Fatalf("generic-governed stateChangePolicy = %v, want neutral allowed_but_not_required", contract.StateChangePolicy)
	}
	for _, profile := range contract.ProfileContracts {
		if profile.ID == "software-dev" {
			t.Fatalf("generic-governed profileContracts unexpectedly include software-dev: %+v", contract.ProfileContracts)
		}
	}
	if len(activation.EvidencePolicies) != 0 || len(activation.MCPServerIDs) != 0 || len(activation.MCPCapabilities) != 0 {
		t.Fatalf("generic-governed activation = %+v, want no software-dev evidence or MCP defaults", activation)
	}
}

func TestActivateWorkspaceExecutionWithBackendUsesSystemDefaultSoul(t *testing.T) {
	useUnavailableLLMProvider(t)
	useSystemDefaultSoulConfig(t, "soul-system-default")
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
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
	activation := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Activation
	if activation.DefaultSoul == nil || string(activation.DefaultSoul.ID) != "soul-system-default" {
		t.Fatalf("runtime activation default soul = %+v, want system default soul", activation.DefaultSoul)
	}
	assertRuntimeV2StageSoulBinding(t, activation, SoulBindingStagePlanning, "soul-system-default", "system_default")
	assertRuntimeV2StageSoulBinding(t, activation, SoulBindingStageValidation, "soul-system-default", "system_default")
}

func TestActivateWorkspaceExecutionWithBackendDoesNotInjectSystemDefaultSoulWhenStageBindingsExist(t *testing.T) {
	useUnavailableLLMProvider(t)
	useSystemDefaultSoulConfig(t, "soul-system-default")
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
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
	activation := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Activation
	assertRuntimeV2StageSoulBinding(t, activation, SoulBindingStagePlanning, "soul-stage-planning", "workspace_stage_binding")
	assertRuntimeV2StageSoulBinding(t, activation, SoulBindingStageExecution, "soul-stage-execution", "workspace_stage_binding")
	assertRuntimeV2StageSoulBinding(t, activation, SoulBindingStageValidation, "soul-stage-validation", "workspace_stage_binding")
}

func TestActivateWorkspaceExecutionWithBackendValidatesExecutionSoulOnly(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
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
	activation := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Activation
	assertRuntimeV2StageSoulBinding(t, activation, SoulBindingStagePlanning, "soul-stage-planning", "workspace_stage_binding")
	assertRuntimeV2StageSoulBinding(t, activation, SoulBindingStageExecution, "soul-stage-execution", "workspace_stage_binding")
}

func TestWorkspaceSoulBindingsWithBackendRejectsInvalidBinding(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
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
	cleanupSharedBackendSession(t, "file", dataRoot)
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
		GoalID: goal.ID,
		DoDID:  dod.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	activation := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Activation
	if !runtimeV2HasSkill(activation, "skill-auto-delivery") {
		t.Fatalf("runtime skills = %+v, want auto selected skill", activation.Skills)
	}
	if !runtimeV2HasMCPServer(activation, "mcp-auto-skill") {
		t.Fatalf("runtime MCP server IDs = %+v, want required MCP auto bound", activation.MCPServerIDs)
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
	rawHub := magicevent.NewHub(16)
	observer := magicevent.NewSimpleObserver(memoryevents.CommandDestination, rawHub)
	observer.Subscribe(memoryevents.TopicMemoryCommandListByWorkspace, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(memoryevents.ListMemoryByWorkspaceCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		items, err := service.ListMemoryByWorkspace(ctx, command.WorkspaceID)
		if err != nil {
			result.Set(nil, cd.NewError(cd.Unexpected, err.Error()))
			return
		}
		result.Set(memoryevents.MemoryCommandResult{Items: items}, nil)
	})
	defer observer.Unsubscribe(memoryevents.TopicMemoryCommandListByWorkspace)

	items, evidence, err := activationports.SelectedMemoryItems(ctx, activationports.MemorySelectionOptions{
		WorkspaceID: workspaceID,
		Hub:         rawHub,
	})
	if err != nil {
		t.Fatalf("selectedMemoryItems: %v", err)
	}
	if len(items) != 1 || items[0].ID != "memory-accepted-tool" {
		t.Fatalf("items = %+v, want only accepted tool memory", items)
	}
	if evidence == nil || evidence.Status != "selected" {
		t.Fatalf("evidence = %+v, want selected status", evidence)
	}
	if !contextStringSliceContains(evidence.CandidateMemoryIDs, "memory-provider-retry-process") ||
		!contextStringSliceContains(evidence.ExcludedMemoryIDs, "memory-provider-retry-process") ||
		contextStringSliceContains(evidence.RetrievedMemoryIDs, "memory-provider-retry-process") {
		t.Fatalf("evidence = %+v, want provider retry candidate excluded from retrieval", evidence)
	}
	contexts := activationcore.MemoryContexts(items)
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
	cleanupSharedBackendSession(t, "file", dataRoot)
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
	cleanupSharedBackendSession(t, "file", dataRoot)
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
	exports := testRuntimeExports(session.Bootstrap)
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
	persistedWorkspace, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("read persisted workspace: %v", err)
	}
	if persistedWorkspace.DefaultLLMTarget != nil {
		t.Fatalf("resume materialized workspace model = %+v, want unchanged nil preference", persistedWorkspace.DefaultLLMTarget)
	}
}

func TestActivateWorkspaceExecutionWithBackendResumesSealedActiveRunWhenWorkspaceModelChanges(t *testing.T) {
	useActivationLLMConfig(t, "", "")
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	ctx := context.Background()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-activation-model-change", "activation model change workspace", "", "workspaces/activation-model-change")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "continue with the selected workspace model")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"the selected model is sealed into the run"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	session, err := sharedBackendSession(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("sharedBackendSession: %v", err)
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		t.Fatalf("RequiredCommandHub: %v", err)
	}
	inputs, err := activationports.ReadWorkspaceRuntimeActivationInputs(ctx, hub, common.WorkspaceID(workspace.ID), common.GoalID(goal.ID), common.DoDID(dod.ID))
	if err != nil {
		t.Fatalf("ReadWorkspaceRuntimeActivationInputs: %v", err)
	}
	initialBuild, err := buildWorkspaceRuntimeActivationWithDefinitions(ctx, session, inputs.Workspace, inputs.Goal, inputs.DoD, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("build initial activation: %v", err)
	}
	initialBootstrap, err := activationcore.RuntimeV2Bootstrap(initialBuild.EffectiveGoal, initialBuild.EffectiveDoD, initialBuild.DefinitionRevision, initialBuild.DefinitionChangeImpact, initialBuild.RuntimeActivation)
	if err != nil {
		t.Fatalf("initial Runtime V2 bootstrap: %v", err)
	}
	if got := initialBootstrap.Activation.ProviderTarget; !got.Same(runtimecontract.ProviderModelTarget{ProviderID: "custom-test", ModelID: "model-1"}) {
		t.Fatalf("first provider target = %+v, want custom-test/model-1", got)
	}
	exports := testRuntimeExports(session.Bootstrap)
	oldRunID := common.RunID("run-activation-model-change-old")
	ownerCtx := common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspace.ID))
	if err := exports.Repositories.Runs.Create(ownerCtx, &runmodel.Run{
		ID: oldRunID, WorkspaceID: common.WorkspaceID(workspace.ID), GoalID: common.GoalID(goal.ID), DoDID: common.DoDID(dod.ID),
		DefinitionRevision: initialBootstrap.Definition.Revision, InitialStepKind: "llm/workspace_execution", RuntimeBootstrap: initialBootstrap,
		State: common.RunStateActive,
	}); err != nil {
		t.Fatalf("create sealed active run: %v", err)
	}
	workspaceRecord, err := exports.Repositories.Workspaces.Get(ownerCtx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("read workspace owner record: %v", err)
	}
	workspaceRecord.ActiveRunID = oldRunID
	if err := exports.Repositories.Workspaces.Update(ownerCtx, workspaceRecord); err != nil {
		t.Fatalf("set active run: %v", err)
	}
	changedTarget := runtimecontract.ProviderModelTarget{ProviderID: "custom-test", ModelID: "model-2"}
	if _, err := UpdateWorkspaceLLMTargetWithSession(ctx, session, workspace.ID, &changedTarget); err != nil {
		t.Fatalf("UpdateWorkspaceLLMTargetWithSession: %v", err)
	}
	if _, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{LLMTarget: &changedTarget}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("active Run explicit model override err=%v, want conflict", err)
	}

	continued, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("resume activation after model change: %v", err)
	}
	if continued.Run.ID != string(oldRunID) {
		t.Fatalf("continued run id = %q, want sealed active run %q", continued.Run.ID, oldRunID)
	}
	continuedOwnerRun := runtimeV2OwnerRunWithBackend(t, ctx, "file", dataRoot, workspace.ID, continued.Run.ID)
	if got := continuedOwnerRun.RuntimeBootstrap.Activation.ProviderTarget; !got.Same(initialBootstrap.Activation.ProviderTarget) {
		t.Fatalf("resumed provider target = %+v, want sealed %+v", got, initialBootstrap.Activation.ProviderTarget)
	}
	oldRun, err := exports.Repositories.Runs.Get(common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspace.ID)), oldRunID)
	if err != nil {
		t.Fatalf("read resumed run: %v", err)
	}
	if oldRun.State != common.RunStateActive {
		t.Fatalf("resumed run state = %q, want active", oldRun.State)
	}
	updatedWorkspace, err := exports.Repositories.Workspaces.Get(ownerCtx, common.WorkspaceID(workspace.ID))
	if err != nil || updatedWorkspace.DefaultLLMTarget == nil || !updatedWorkspace.DefaultLLMTarget.Same(changedTarget) {
		t.Fatalf("workspace next-run provider target=%+v err=%v, want %+v", updatedWorkspace, err, changedTarget)
	}
}

func TestActivateWorkspaceExecutionWithBackendStartsCreatedActiveRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-created-active", "activation created active workspace", "", "workspaces/activation-created-active")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "created active goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"created active dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	created, err := CreateRunWithBackend(context.Background(), "file", dataRoot, workspace.ID, "run-activation-created-active", goal.ID, dod.ID)
	if err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	if created.State != string(common.RunStatePending) {
		t.Fatalf("created run = %+v, want pending", created)
	}
	// Seed ActiveRunID via file repositories only — avoid a second RuntimeSession.
	ctx := context.Background()
	repos := openFileRepositories(t, dataRoot)
	workspaceItem, err := repos.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.ActiveRunID = common.RunID(created.ID)
	if err := repos.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace active run: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ID != created.ID {
		t.Fatalf("activation run id = %q, want existing created run %q", item.Run.ID, created.ID)
	}
	if item.Run.State == "created" {
		t.Fatalf("activation run = %+v, want started observable run", item.Run)
	}
	if item.Snapshot.Workspace.ActiveRunID != created.ID {
		t.Fatalf("snapshot activeRunId = %q, want %q", item.Snapshot.Workspace.ActiveRunID, created.ID)
	}
}

func TestActivateWorkspaceExecutionWithBackendRestartReplacesCreatedActiveRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-restart-created-active", "activation restart created active workspace", "", "workspaces/activation-restart-created-active")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "restart created active goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"restart created active dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	created, err := CreateRunWithBackend(context.Background(), "file", dataRoot, workspace.ID, "run-activation-restart-created-active", goal.ID, dod.ID)
	if err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	updatedGoal, err := UpdateGoalWithBackend(context.Background(), "file", dataRoot, goal.ID, "restart created active goal revised")
	if err != nil {
		t.Fatalf("UpdateGoalWithBackend: %v", err)
	}
	updatedDoD, err := UpdateDoDWithBackend(context.Background(), "file", dataRoot, dod.ID, []string{"restart created active dod revised"})
	if err != nil {
		t.Fatalf("UpdateDoDWithBackend: %v", err)
	}
	// Seed stale ActiveRun via file repositories only — avoid a second RuntimeSession.
	ctx := context.Background()
	repos := openFileRepositories(t, dataRoot)
	workspaceItem, err := repos.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	createdRun, err := repos.Runs.Get(ctx, common.RunID(created.ID))
	if err != nil {
		t.Fatalf("get created run: %v", err)
	}
	createdRun.DefinitionRevision = "stale-restart-definition"
	if createdRun.RuntimeBootstrap != nil {
		createdRun.RuntimeBootstrap.Definition.Revision = "stale-restart-definition"
	}
	// Keep the direct repository fixture internally consistent with a created-but-not-initialized Run.
	// Runtime V2 state is immutable owner data and must not retain the previous definition revision.
	createdRun.RuntimeContractVersion = ""
	createdRun.RuntimeState = nil
	if err := repos.Runs.Update(ctx, createdRun); err != nil {
		t.Fatalf("update created run stale context: %v", err)
	}
	workspaceItem.ActiveRunID = common.RunID(created.ID)
	if err := repos.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace active run: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{Restart: true})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend restart: %v", err)
	}
	if item.Run.ID == created.ID {
		t.Fatalf("activation run id = %q, want replacement run", item.Run.ID)
	}
	if item.Run.GoalID != updatedGoal.ID || item.Run.DoDID != updatedDoD.ID {
		t.Fatalf("replacement run = %+v, want latest updated goal/dod", item.Run)
	}
	if item.Run.DefinitionRevision != "goal:"+updatedGoal.ID+"|dod:"+updatedDoD.ID {
		t.Fatalf("replacement definitionRevision = %q, want latest revision", item.Run.DefinitionRevision)
	}
	ownerRun := runtimeV2OwnerRunWithBackend(t, context.Background(), "file", dataRoot, workspace.ID, item.Run.ID)
	if ownerRun.RuntimeBootstrap.Definition.Revision == "stale-restart-definition" {
		t.Fatalf("replacement runtime definition reused stale revision: %+v", ownerRun.RuntimeBootstrap.Definition)
	}
	for _, focus := range ownerRun.RuntimeBootstrap.Focuses {
		if focus.ID == "cwi-stale-restart" || stringSliceContains(focus.TargetRefs, "run-activation-restart-created-active/stale-evidence") {
			t.Fatalf("replacement runtime focus reused stale old run evidence: %+v", focus)
		}
	}
	if item.Run.State == "created" {
		t.Fatalf("replacement run = %+v, want started observable run", item.Run)
	}
	oldRun, err := repos.Runs.Get(ctx, common.RunID(created.ID))
	if err != nil {
		t.Fatalf("get old run: %v", err)
	}
	if oldRun.State != common.RunStateSuperseded {
		t.Fatalf("old run state = %q, want superseded", oldRun.State)
	}
	if item.Snapshot.Workspace.ActiveRunID != item.Run.ID {
		t.Fatalf("snapshot activeRunId = %q, want replacement %q", item.Snapshot.Workspace.ActiveRunID, item.Run.ID)
	}
}

func TestActivateWorkspaceExecutionWithBackendIgnoresTerminalActiveRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
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

	// Seed terminal ActiveRun via file repositories only — avoid a second RuntimeSession.
	ctx := context.Background()
	repos := openFileRepositories(t, dataRoot)
	oldRunID := common.RunID("run-activation-terminal-active-old")
	if err := repos.Runs.Create(ctx, &runmodel.Run{
		ID:          oldRunID,
		WorkspaceID: common.WorkspaceID(workspace.ID),
		GoalID:      common.GoalID(goal.ID),
		DoDID:       common.DoDID(dod.ID),
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("CreateRun old active: %v", err)
	}
	oldRun, err := repos.Runs.Get(ctx, oldRunID)
	if err != nil {
		t.Fatalf("get old run: %v", err)
	}
	oldRun.State = common.RunStateSuperseded
	if err := repos.Runs.Update(ctx, oldRun); err != nil {
		t.Fatalf("mark old run superseded: %v", err)
	}
	workspaceItem, err := repos.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.ActiveRunID = oldRunID
	workspaceItem.State = common.WorkspaceStateActive
	if err := repos.Workspaces.Update(ctx, workspaceItem); err != nil {
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

func TestActivateWorkspaceExecutionWithBackendContinuesRejectedTerminalResult(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-rejected-terminal", "activation rejected terminal workspace", "", "workspaces/activation-rejected-terminal")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "continue from rejected result")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"missing evidence is closed"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	// Seed rejected terminal predecessor via file repositories only — avoid a second RuntimeSession.
	ctx := context.Background()
	repos := openFileRepositories(t, dataRoot)
	oldRunID := common.RunID("run-activation-rejected-terminal-old")
	if err := repos.Runs.Create(ctx, &runmodel.Run{
		ID:          oldRunID,
		WorkspaceID: common.WorkspaceID(workspace.ID),
		GoalID:      common.GoalID(goal.ID),
		DoDID:       common.DoDID(dod.ID),
		State:       common.RunStateFailed,
	}); err != nil {
		t.Fatalf("CreateRun old terminal: %v", err)
	}
	oldRun, err := repos.Runs.Get(ctx, oldRunID)
	if err != nil {
		t.Fatalf("get old run: %v", err)
	}
	oldRun.State = common.RunStateFailed
	oldRun.FailureReason = "terminal rejected result"
	if err := repos.Runs.Update(ctx, oldRun); err != nil {
		t.Fatalf("mark old run failed: %v", err)
	}
	workspaceItem, err := repos.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.ActiveRunID = ""
	workspaceItem.State = common.WorkspaceStateActive
	if err := repos.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace terminal state: %v", err)
	}
	if err := repos.Results.Create(ctx, &resultmodel.Result{
		ID:           common.ResultID(string(oldRunID) + "/result/dod-assessment"),
		WorkspaceID:  common.WorkspaceID(workspace.ID),
		RunID:        oldRunID,
		GoalID:       common.GoalID(goal.ID),
		DoDID:        common.DoDID(dod.ID),
		State:        common.ResultStateRejected,
		Summary:      "missing required evidence",
		SourceType:   "dod_assessment",
		SourceRef:    string(oldRunID) + "/graph-001/validation",
		EvidenceRefs: []string{string(oldRunID) + "/graph-001/validation"},
		DoDAssessment: resultmodel.DoDAssessment{
			Satisfied: false,
			Items: []resultmodel.DoDAssessmentItem{
				{Item: "missing evidence is closed", State: "failed", FailureReason: "missing validation evidence"},
			},
		},
		UpdatedAt: "2026-05-16T00:00:00Z",
	}); err != nil {
		t.Fatalf("create rejected result: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ID == string(oldRunID) || item.Run.GoalID != goal.ID || item.Run.DoDID != dod.ID {
		t.Fatalf("activation run = %+v, want successor run using current definitions", item.Run)
	}
	createdRun, err := repos.Runs.Get(ctx, common.RunID(item.Run.ID))
	if err != nil {
		t.Fatalf("get successor run: %v", err)
	}
	if createdRun.RuntimeBootstrap == nil {
		t.Fatal("successor run runtime bootstrap is missing")
	}
	continuation := createdRun.RuntimeBootstrap.Activation.Continuation.RejectedResult
	if continuation == nil || continuation.PredecessorRunID != oldRunID ||
		continuation.ResultID != common.ResultID(string(oldRunID)+"/result/dod-assessment") ||
		continuation.ResultState != common.ResultStateRejected {
		t.Fatalf("rejectedResultContinuation = %+v, want predecessor rejected result context", continuation)
	}
	if len(continuation.UnsatisfiedDoDItems) != 1 || continuation.UnsatisfiedDoDItems[0].Item != "missing evidence is closed" {
		t.Fatalf("unsatisfiedDoDItems = %+v, want rejected DoD gap", continuation.UnsatisfiedDoDItems)
	}
	if item.Snapshot.Workspace.ActiveRunID != item.Run.ID {
		t.Fatalf("snapshot activeRunId = %q, want successor run %q", item.Snapshot.Workspace.ActiveRunID, item.Run.ID)
	}
}

func TestActivateWorkspaceExecutionWithBackendReplacesActiveRunWhenExplicitDefinitionsChange(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	cleanupSharedBackendSession(t, "file", dataRoot)
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-replace-active", "activation replace active workspace", "", "workspaces/activation-replace-active")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	oldGoal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "old activation goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend old: %v", err)
	}
	oldDoD, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"old activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend old: %v", err)
	}

	// Seed active run via file repositories only — avoid a second RuntimeSession.
	ctx := context.Background()
	repos := openFileRepositories(t, dataRoot)
	oldRunID := common.RunID("run-activation-replace-active-old")
	if err := repos.Runs.Create(ctx, &runmodel.Run{
		ID:          oldRunID,
		WorkspaceID: common.WorkspaceID(workspace.ID),
		GoalID:      common.GoalID(oldGoal.ID),
		DoDID:       common.DoDID(oldDoD.ID),
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("CreateRun old active: %v", err)
	}
	oldRun, err := repos.Runs.Get(ctx, oldRunID)
	if err != nil {
		t.Fatalf("get old run: %v", err)
	}
	oldRun.State = common.RunStateActive
	if err := repos.Runs.Update(ctx, oldRun); err != nil {
		t.Fatalf("mark old run running: %v", err)
	}
	workspaceItem, err := repos.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.ActiveRunID = oldRunID
	workspaceItem.State = common.WorkspaceStateActive
	if err := repos.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace active run: %v", err)
	}
	newGoal, err := UpdateGoalWithBackend(context.Background(), "file", dataRoot, oldGoal.ID, "new activation goal")
	if err != nil {
		t.Fatalf("UpdateGoalWithBackend new: %v", err)
	}
	newDoD, err := UpdateDoDWithBackend(context.Background(), "file", dataRoot, oldDoD.ID, []string{"new activation dod"})
	if err != nil {
		t.Fatalf("UpdateDoDWithBackend new: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ID == string(oldRunID) || item.Run.GoalID != newGoal.ID || item.Run.DoDID != newDoD.ID {
		t.Fatalf("activation run = %+v, want replacement run with updated canonical goal/dod", item.Run)
	}
	definition := runtimeV2OwnerRunWithBackend(t, context.Background(), "file", dataRoot, workspace.ID, item.Run.ID).RuntimeBootstrap.Definition
	if definition.GoalDefinition != "- new activation goal" {
		t.Fatalf("replacement goal text = %q, want updated canonical goal", definition.GoalDefinition)
	}
	if !strings.Contains(definition.DoDDefinition, "new activation dod") {
		t.Fatalf("replacement dod definition = %q, want updated canonical dod", definition.DoDDefinition)
	}
	replaced, err := repos.Runs.Get(ctx, oldRunID)
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

func assertAnyStringListContains(t *testing.T, value any, want string) {
	t.Helper()
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if item == want {
				return
			}
		}
	case []any:
		for _, item := range typed {
			if stringItem, _ := item.(string); stringItem == want {
				return
			}
		}
	}
	t.Fatalf("list %+v missing %q", value, want)
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
	case skillselectcore.Evidence:
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
	// Intentionally no-op for most activation unit tests.
	// createWorkspaceWithIDWithBackend / useWorkorchTestConfig already install an offline
	// custom provider target so activation does not require a live ai-proxy catalog.
	// Tests that need an explicit offline LLM config should call useActivationLLMConfig.
	_ = t
}

func useActivationLLMConfig(t *testing.T, baseURL string, embeddingProviderID string) {
	t.Helper()
	configDir := t.TempDir()
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "http://127.0.0.1:9"
	}
	embeddingBlock := ""
	if strings.TrimSpace(embeddingProviderID) != "" {
		// Optional embedding target for semantic memory tests; empty means disabled.
		embeddingBlock = `
[llm.defaultEmbedding]
providerID = "custom-test"
modelID = "embed-1"
`
	}
	content := fmt.Sprintf(`
endpointName = "workorch"

[repository]
backend = "file"

[llm.aiProxy]
baseURL = %q
apiKey = "test-key"
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "custom-test"
modelID = "model-1"
%s
[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

[llm.customProviders.custom-test]
provider = "compatible"
apiSurface = "openai_compatible_http"
baseURL = %q
apiKey = "secret-value"
timeout = "5s"

[llm.customProviders.custom-test.models.model-1]
contextWindowTokens = 128000
maxOutputTokens = 8192
operations = ["chat_completions"]

[llm.customProviders.custom-test.models.model-2]
contextWindowTokens = 128000
maxOutputTokens = 8192
operations = ["chat_completions"]

[llm.customProviders.custom-test.models.embed-1]
contextWindowTokens = 8192
maxOutputTokens = 1024
operations = ["embeddings"]
`, baseURL, embeddingBlock, baseURL)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write application config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
	if _, err := workorchconfig.Current(); err != nil {
		t.Fatalf("current activation llm config: %v", err)
	}
}

func useSystemDefaultSoulConfig(t *testing.T, defaultSoulID string) {
	t.Helper()
	configDir := t.TempDir()
	content := fmt.Sprintf(`
endpointName = "workorch"

[repository]
backend = "file"

[runtime]
defaultSoulId = %q

[llm.aiProxy]
baseURL = "http://127.0.0.1:9"
apiKey = "test-key"
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "custom-test"
modelID = "model-1"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

[llm.customProviders.custom-test]
provider = "compatible"
apiSurface = "openai_compatible_http"
baseURL = "http://127.0.0.1:9"
apiKey = "secret-value"
timeout = "5s"

[llm.customProviders.custom-test.models.model-1]
contextWindowTokens = 128000
maxOutputTokens = 8192
operations = ["chat_completions"]
`, defaultSoulID)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write application config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
	if _, err := workorchconfig.Current(); err != nil {
		t.Fatalf("current system default soul config: %v", err)
	}
}
