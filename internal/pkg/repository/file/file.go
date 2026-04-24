package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	approval "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decision "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	run "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkspaceRepository struct {
	mu   sync.RWMutex
	path string
}

func NewWorkspaceRepository(path string) *WorkspaceRepository {
	return &WorkspaceRepository{path: path}
}

func (r *WorkspaceRepository) Create(_ context.Context, item *workspace.Workspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.WorkspaceID, workspace.Workspace](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneWorkspace(item)
	return saveMap(r.path, items)
}

func (r *WorkspaceRepository) Get(_ context.Context, id common.WorkspaceID) (*workspace.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.WorkspaceID, workspace.Workspace](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneWorkspace(&item), nil
}

func (r *WorkspaceRepository) Update(_ context.Context, item *workspace.Workspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.WorkspaceID, workspace.Workspace](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneWorkspace(item)
	return saveMap(r.path, items)
}

type RunRepository struct {
	mu   sync.RWMutex
	path string
}

func NewRunRepository(path string) *RunRepository {
	return &RunRepository{path: path}
}

func (r *RunRepository) Create(_ context.Context, item *run.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.RunID, run.Run](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneRun(item)
	return saveMap(r.path, items)
}

func (r *RunRepository) Get(_ context.Context, id common.RunID) (*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.RunID, run.Run](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneRun(&item), nil
}

func (r *RunRepository) Update(_ context.Context, item *run.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.RunID, run.Run](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneRun(item)
	return saveMap(r.path, items)
}

type StepRepository struct {
	mu   sync.RWMutex
	path string
}

func NewStepRepository(path string) *StepRepository {
	return &StepRepository{path: path}
}

func (r *StepRepository) Create(_ context.Context, item *run.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.StepID, run.Step](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneStep(item)
	return saveMap(r.path, items)
}

func (r *StepRepository) Get(_ context.Context, id common.StepID) (*run.Step, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.StepID, run.Step](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneStep(&item), nil
}

func (r *StepRepository) ListByRun(_ context.Context, runID common.RunID) ([]*run.Step, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.StepID, run.Step](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*run.Step, 0, len(items))
	for _, item := range items {
		if item.RunID != runID {
			continue
		}
		ret = append(ret, cloneStep(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *StepRepository) Update(_ context.Context, item *run.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.StepID, run.Step](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneStep(item)
	return saveMap(r.path, items)
}

type DecisionTicketRepository struct {
	mu   sync.RWMutex
	path string
}

func NewDecisionTicketRepository(path string) *DecisionTicketRepository {
	return &DecisionTicketRepository{path: path}
}

func (r *DecisionTicketRepository) Create(_ context.Context, item *decision.Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.TicketID, decision.Ticket](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneDecisionTicket(item)
	return saveMap(r.path, items)
}

func (r *DecisionTicketRepository) Get(_ context.Context, id common.TicketID) (*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.TicketID, decision.Ticket](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneDecisionTicket(&item), nil
}

func (r *DecisionTicketRepository) Update(_ context.Context, item *decision.Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.TicketID, decision.Ticket](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneDecisionTicket(item)
	return saveMap(r.path, items)
}

type DecisionResolutionRepository struct {
	mu   sync.RWMutex
	path string
}

func NewDecisionResolutionRepository(path string) *DecisionResolutionRepository {
	return &DecisionResolutionRepository{path: path}
}

func (r *DecisionResolutionRepository) Create(_ context.Context, item *decision.Resolution) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.TicketID, decision.Resolution](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.TicketID]; ok {
		return common.ErrConflict
	}
	items[item.TicketID] = *cloneDecisionResolution(item)
	return saveMap(r.path, items)
}

func (r *DecisionResolutionRepository) GetByTicketID(_ context.Context, ticketID common.TicketID) (*decision.Resolution, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.TicketID, decision.Resolution](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[ticketID]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneDecisionResolution(&item), nil
}

type ApprovalRepository struct {
	mu   sync.RWMutex
	path string
}

func NewApprovalRepository(path string) *ApprovalRepository {
	return &ApprovalRepository{path: path}
}

func (r *ApprovalRepository) Create(_ context.Context, item *approval.Approval) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.ApprovalID, approval.Approval](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneApproval(item)
	return saveMap(r.path, items)
}

func (r *ApprovalRepository) Get(_ context.Context, id common.ApprovalID) (*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ApprovalID, approval.Approval](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneApproval(&item), nil
}

func (r *ApprovalRepository) Update(_ context.Context, item *approval.Approval) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.ApprovalID, approval.Approval](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneApproval(item)
	return saveMap(r.path, items)
}

func loadMap[K comparable, V any](path string) (map[K]V, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[K]V{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return map[K]V{}, nil
	}
	items := map[K]V{}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func saveMap[K comparable, V any](path string, items map[K]V) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func cloneWorkspace(item *workspace.Workspace) *workspace.Workspace {
	cloned := *item
	return &cloned
}

func cloneRun(item *run.Run) *run.Run {
	cloned := *item
	return &cloned
}

func cloneStep(item *run.Step) *run.Step {
	cloned := *item
	if item.Input != nil {
		cloned.Input = make(map[string]any, len(item.Input))
		for k, v := range item.Input {
			cloned.Input[k] = v
		}
	}
	if item.Output != nil {
		cloned.Output = make(map[string]any, len(item.Output))
		for k, v := range item.Output {
			cloned.Output[k] = v
		}
	}
	return &cloned
}

func cloneDecisionTicket(item *decision.Ticket) *decision.Ticket {
	cloned := *item
	return &cloned
}

func cloneDecisionResolution(item *decision.Resolution) *decision.Resolution {
	cloned := *item
	if item.Payload != nil {
		cloned.Payload = make(map[string]any, len(item.Payload))
		for k, v := range item.Payload {
			cloned.Payload[k] = v
		}
	}
	return &cloned
}

func cloneApproval(item *approval.Approval) *approval.Approval {
	cloned := *item
	return &cloned
}
