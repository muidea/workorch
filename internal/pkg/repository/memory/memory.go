package memory

import (
	"context"
	"sort"
	"sync"

	approval "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decision "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	run "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkspaceRepository struct {
	mu    sync.RWMutex
	items map[common.WorkspaceID]*workspace.Workspace
}

func NewWorkspaceRepository() *WorkspaceRepository {
	return &WorkspaceRepository{items: map[common.WorkspaceID]*workspace.Workspace{}}
}

func (r *WorkspaceRepository) Create(_ context.Context, item *workspace.Workspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	cloned := *item
	r.items[item.ID] = &cloned
	return nil
}

func (r *WorkspaceRepository) Get(_ context.Context, id common.WorkspaceID) (*workspace.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	cloned := *item
	return &cloned, nil
}

func (r *WorkspaceRepository) List(_ context.Context) ([]*workspace.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*workspace.Workspace, 0, len(ids))
	for _, id := range ids {
		cloned := *r.items[common.WorkspaceID(id)]
		ret = append(ret, &cloned)
	}
	return ret, nil
}

func (r *WorkspaceRepository) Update(_ context.Context, item *workspace.Workspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	cloned := *item
	r.items[item.ID] = &cloned
	return nil
}

type GoalRepository struct {
	mu    sync.RWMutex
	items map[common.GoalID]*workspace.Goal
}

func NewGoalRepository() *GoalRepository {
	return &GoalRepository{items: map[common.GoalID]*workspace.Goal{}}
}

func (r *GoalRepository) Create(_ context.Context, item *workspace.Goal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	cloned := *item
	r.items[item.ID] = &cloned
	return nil
}

func (r *GoalRepository) Get(_ context.Context, id common.GoalID) (*workspace.Goal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	cloned := *item
	return &cloned, nil
}

func (r *GoalRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*workspace.Goal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var items []*workspace.Goal
	for _, item := range r.items {
		if item.WorkspaceID != workspaceID {
			continue
		}
		cloned := *item
		items = append(items, &cloned)
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

type DoDRepository struct {
	mu    sync.RWMutex
	items map[common.DoDID]*workspace.DoD
}

func NewDoDRepository() *DoDRepository {
	return &DoDRepository{items: map[common.DoDID]*workspace.DoD{}}
}

func (r *DoDRepository) Create(_ context.Context, item *workspace.DoD) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneDoD(item)
	return nil
}

func (r *DoDRepository) Get(_ context.Context, id common.DoDID) (*workspace.DoD, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneDoD(item), nil
}

func (r *DoDRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*workspace.DoD, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var items []*workspace.DoD
	for _, item := range r.items {
		if item.WorkspaceID != workspaceID {
			continue
		}
		items = append(items, cloneDoD(item))
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *DoDRepository) Update(_ context.Context, item *workspace.DoD) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneDoD(item)
	return nil
}

type RunRepository struct {
	mu    sync.RWMutex
	items map[common.RunID]*run.Run
}

func NewRunRepository() *RunRepository {
	return &RunRepository{items: map[common.RunID]*run.Run{}}
}

func (r *RunRepository) Create(_ context.Context, item *run.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	cloned := *item
	r.items[item.ID] = &cloned
	return nil
}

func (r *RunRepository) Get(_ context.Context, id common.RunID) (*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	cloned := *item
	return &cloned, nil
}

func (r *RunRepository) List(_ context.Context) ([]*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*run.Run, 0, len(ids))
	for _, id := range ids {
		cloned := *r.items[common.RunID(id)]
		ret = append(ret, &cloned)
	}
	return ret, nil
}

func (r *RunRepository) Update(_ context.Context, item *run.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	cloned := *item
	r.items[item.ID] = &cloned
	return nil
}

type StepRepository struct {
	mu    sync.RWMutex
	items map[common.StepID]*run.Step
}

func NewStepRepository() *StepRepository {
	return &StepRepository{items: map[common.StepID]*run.Step{}}
}

func (r *StepRepository) Create(_ context.Context, item *run.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	cloned := cloneStep(item)
	r.items[item.ID] = cloned
	return nil
}

func (r *StepRepository) Get(_ context.Context, id common.StepID) (*run.Step, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneStep(item), nil
}

func (r *StepRepository) ListByRun(_ context.Context, runID common.RunID) ([]*run.Step, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var items []*run.Step
	for _, item := range r.items {
		if item.RunID != runID {
			continue
		}
		items = append(items, cloneStep(item))
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *StepRepository) Update(_ context.Context, item *run.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneStep(item)
	return nil
}

type ToolCallRepository struct {
	mu    sync.RWMutex
	items map[common.ToolCallID]*run.ToolCall
}

func NewToolCallRepository() *ToolCallRepository {
	return &ToolCallRepository{items: map[common.ToolCallID]*run.ToolCall{}}
}

func (r *ToolCallRepository) Create(_ context.Context, item *run.ToolCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneToolCall(item)
	return nil
}

func (r *ToolCallRepository) Get(_ context.Context, id common.ToolCallID) (*run.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneToolCall(item), nil
}

func (r *ToolCallRepository) ListByRun(_ context.Context, runID common.RunID) ([]*run.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*run.ToolCall, 0)
	for _, item := range r.items {
		if item.RunID != runID {
			continue
		}
		items = append(items, cloneToolCall(item))
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *ToolCallRepository) ListByStep(_ context.Context, stepID common.StepID) ([]*run.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*run.ToolCall, 0)
	for _, item := range r.items {
		if item.StepID != stepID {
			continue
		}
		items = append(items, cloneToolCall(item))
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *ToolCallRepository) Update(_ context.Context, item *run.ToolCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneToolCall(item)
	return nil
}

type PatchRepository struct {
	mu    sync.RWMutex
	items map[common.PatchID]*run.Patch
}

func NewPatchRepository() *PatchRepository {
	return &PatchRepository{items: map[common.PatchID]*run.Patch{}}
}

func (r *PatchRepository) Create(_ context.Context, item *run.Patch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	cloned := *item
	r.items[item.ID] = &cloned
	return nil
}

func (r *PatchRepository) Get(_ context.Context, id common.PatchID) (*run.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	cloned := *item
	return &cloned, nil
}

func (r *PatchRepository) ListByRun(_ context.Context, runID common.RunID) ([]*run.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*run.Patch, 0)
	for _, item := range r.items {
		if item.RunID != runID {
			continue
		}
		cloned := *item
		items = append(items, &cloned)
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *PatchRepository) ListByStep(_ context.Context, stepID common.StepID) ([]*run.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*run.Patch, 0)
	for _, item := range r.items {
		if item.StepID != stepID {
			continue
		}
		cloned := *item
		items = append(items, &cloned)
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *PatchRepository) Update(_ context.Context, item *run.Patch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	cloned := *item
	r.items[item.ID] = &cloned
	return nil
}

type ValidationResultRepository struct {
	mu    sync.RWMutex
	items map[common.ValidationResultID]*run.ValidationResult
}

func NewValidationResultRepository() *ValidationResultRepository {
	return &ValidationResultRepository{items: map[common.ValidationResultID]*run.ValidationResult{}}
}

func (r *ValidationResultRepository) Create(_ context.Context, item *run.ValidationResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	cloned := *item
	r.items[item.ID] = &cloned
	return nil
}

func (r *ValidationResultRepository) Get(_ context.Context, id common.ValidationResultID) (*run.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	cloned := *item
	return &cloned, nil
}

func (r *ValidationResultRepository) ListByRun(_ context.Context, runID common.RunID) ([]*run.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*run.ValidationResult, 0)
	for _, item := range r.items {
		if item.RunID != runID {
			continue
		}
		cloned := *item
		items = append(items, &cloned)
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *ValidationResultRepository) ListByStep(_ context.Context, stepID common.StepID) ([]*run.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*run.ValidationResult, 0)
	for _, item := range r.items {
		if item.StepID != stepID {
			continue
		}
		cloned := *item
		items = append(items, &cloned)
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *ValidationResultRepository) Update(_ context.Context, item *run.ValidationResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	cloned := *item
	r.items[item.ID] = &cloned
	return nil
}

type RuntimeCheckpointRepository struct {
	mu    sync.RWMutex
	items map[common.CheckpointID]*run.RuntimeCheckpoint
}

func NewRuntimeCheckpointRepository() *RuntimeCheckpointRepository {
	return &RuntimeCheckpointRepository{items: map[common.CheckpointID]*run.RuntimeCheckpoint{}}
}

func (r *RuntimeCheckpointRepository) Create(_ context.Context, item *run.RuntimeCheckpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneRuntimeCheckpoint(item)
	return nil
}

func (r *RuntimeCheckpointRepository) Get(_ context.Context, id common.CheckpointID) (*run.RuntimeCheckpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneRuntimeCheckpoint(item), nil
}

func (r *RuntimeCheckpointRepository) ListByRun(_ context.Context, runID common.RunID) ([]*run.RuntimeCheckpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*run.RuntimeCheckpoint, 0)
	for _, item := range r.items {
		if item.RunID != runID {
			continue
		}
		items = append(items, cloneRuntimeCheckpoint(item))
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *RuntimeCheckpointRepository) Update(_ context.Context, item *run.RuntimeCheckpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneRuntimeCheckpoint(item)
	return nil
}

type DecisionTicketRepository struct {
	mu    sync.RWMutex
	items map[common.TicketID]*decision.Ticket
}

func NewDecisionTicketRepository() *DecisionTicketRepository {
	return &DecisionTicketRepository{items: map[common.TicketID]*decision.Ticket{}}
}

func (r *DecisionTicketRepository) Create(_ context.Context, item *decision.Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneDecisionTicket(item)
	return nil
}

func (r *DecisionTicketRepository) Get(_ context.Context, id common.TicketID) (*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneDecisionTicket(item), nil
}

func (r *DecisionTicketRepository) List(_ context.Context) ([]*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*decision.Ticket, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, cloneDecisionTicket(r.items[common.TicketID(id)]))
	}
	return ret, nil
}

func (r *DecisionTicketRepository) Update(_ context.Context, item *decision.Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneDecisionTicket(item)
	return nil
}

type DecisionResolutionRepository struct {
	mu       sync.RWMutex
	byTicket map[common.TicketID]*decision.Resolution
}

func NewDecisionResolutionRepository() *DecisionResolutionRepository {
	return &DecisionResolutionRepository{byTicket: map[common.TicketID]*decision.Resolution{}}
}

func (r *DecisionResolutionRepository) Create(_ context.Context, item *decision.Resolution) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byTicket[item.TicketID]; ok {
		return common.ErrConflict
	}
	cloned := *item
	r.byTicket[item.TicketID] = &cloned
	return nil
}

func (r *DecisionResolutionRepository) GetByTicketID(_ context.Context, ticketID common.TicketID) (*decision.Resolution, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.byTicket[ticketID]
	if !ok {
		return nil, common.ErrNotFound
	}
	cloned := *item
	return &cloned, nil
}

type ApprovalRepository struct {
	mu    sync.RWMutex
	items map[common.ApprovalID]*approval.Approval
}

func NewApprovalRepository() *ApprovalRepository {
	return &ApprovalRepository{items: map[common.ApprovalID]*approval.Approval{}}
}

func (r *ApprovalRepository) Create(_ context.Context, item *approval.Approval) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	cloned := *item
	r.items[item.ID] = &cloned
	return nil
}

func (r *ApprovalRepository) Get(_ context.Context, id common.ApprovalID) (*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	cloned := *item
	return &cloned, nil
}

func (r *ApprovalRepository) List(_ context.Context) ([]*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*approval.Approval, 0, len(ids))
	for _, id := range ids {
		cloned := *r.items[common.ApprovalID(id)]
		ret = append(ret, &cloned)
	}
	return ret, nil
}

func (r *ApprovalRepository) Update(_ context.Context, item *approval.Approval) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	cloned := *item
	r.items[item.ID] = &cloned
	return nil
}

func cloneStep(item *run.Step) *run.Step {
	cloned := *item
	if item.Input != nil {
		cloned.Input = cloneMap(item.Input)
	}
	if item.Output != nil {
		cloned.Output = cloneMap(item.Output)
	}
	return &cloned
}

func cloneToolCall(item *run.ToolCall) *run.ToolCall {
	cloned := *item
	cloned.Arguments = cloneMap(item.Arguments)
	cloned.Output = cloneMap(item.Output)
	return &cloned
}

func cloneRuntimeCheckpoint(item *run.RuntimeCheckpoint) *run.RuntimeCheckpoint {
	cloned := *item
	cloned.Data = cloneMap(item.Data)
	return &cloned
}

func cloneDecisionTicket(item *decision.Ticket) *decision.Ticket {
	cloned := *item
	cloned.Context = cloneMap(item.Context)
	return &cloned
}

func cloneDoD(item *workspace.DoD) *workspace.DoD {
	cloned := *item
	if item.Items != nil {
		cloned.Items = append([]string(nil), item.Items...)
	}
	return &cloned
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
