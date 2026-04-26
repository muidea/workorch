package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	approval "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decision "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	run "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soul "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type WorkspaceRepository struct {
	mu   sync.RWMutex
	path string
}

type SoulRepository struct {
	mu   sync.RWMutex
	path string
}

type MemoryItemRepository struct {
	mu   sync.RWMutex
	path string
}

type SkillRepository struct {
	mu   sync.RWMutex
	path string
}

type MCPServerRepository struct {
	mu   sync.RWMutex
	path string
}

func NewSoulRepository(path string) *SoulRepository {
	return &SoulRepository{path: path}
}

func (r *SoulRepository) Create(_ context.Context, item *soul.Soul) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.SoulID, soul.Soul](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneSoul(item)
	return saveMap(r.path, items)
}

func (r *SoulRepository) Get(_ context.Context, id common.SoulID) (*soul.Soul, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.SoulID, soul.Soul](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneSoul(&item), nil
}

func (r *SoulRepository) List(_ context.Context) ([]*soul.Soul, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.SoulID, soul.Soul](r.path)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*soul.Soul, 0, len(ids))
	for _, id := range ids {
		item := items[common.SoulID(id)]
		ret = append(ret, cloneSoul(&item))
	}
	return ret, nil
}

func (r *SoulRepository) Update(_ context.Context, item *soul.Soul) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.SoulID, soul.Soul](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneSoul(item)
	return saveMap(r.path, items)
}

func NewMemoryItemRepository(path string) *MemoryItemRepository {
	return &MemoryItemRepository{path: path}
}

func (r *MemoryItemRepository) Create(_ context.Context, item *memorymodel.MemoryItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.MemoryID, memorymodel.MemoryItem](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneMemoryItem(item)
	return saveMap(r.path, items)
}

func (r *MemoryItemRepository) Get(_ context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.MemoryID, memorymodel.MemoryItem](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneMemoryItem(&item), nil
}

func (r *MemoryItemRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.MemoryID, memorymodel.MemoryItem](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*memorymodel.MemoryItem, 0, len(items))
	for _, item := range items {
		if item.WorkspaceID != workspaceID {
			continue
		}
		ret = append(ret, cloneMemoryItem(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *MemoryItemRepository) Search(_ context.Context, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.MemoryID, memorymodel.MemoryItem](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*memorymodel.MemoryItem, 0, len(items))
	for _, item := range items {
		if !memoryItemMatches(&item, query) {
			continue
		}
		ret = append(ret, cloneMemoryItem(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		if ret[i].Importance != ret[j].Importance {
			return ret[i].Importance > ret[j].Importance
		}
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if query.Limit > 0 && len(ret) > query.Limit {
		ret = ret[:query.Limit]
	}
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *MemoryItemRepository) Update(_ context.Context, item *memorymodel.MemoryItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.MemoryID, memorymodel.MemoryItem](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneMemoryItem(item)
	return saveMap(r.path, items)
}

func NewSkillRepository(path string) *SkillRepository {
	return &SkillRepository{path: path}
}

func (r *SkillRepository) Create(_ context.Context, item *skillmodel.Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.SkillID, skillmodel.Skill](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneSkill(item)
	return saveMap(r.path, items)
}

func (r *SkillRepository) Get(_ context.Context, id common.SkillID) (*skillmodel.Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.SkillID, skillmodel.Skill](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneSkill(&item), nil
}

func (r *SkillRepository) List(_ context.Context) ([]*skillmodel.Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.SkillID, skillmodel.Skill](r.path)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*skillmodel.Skill, 0, len(ids))
	for _, id := range ids {
		item := items[common.SkillID(id)]
		ret = append(ret, cloneSkill(&item))
	}
	return ret, nil
}

func (r *SkillRepository) Update(_ context.Context, item *skillmodel.Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.SkillID, skillmodel.Skill](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneSkill(item)
	return saveMap(r.path, items)
}

func NewMCPServerRepository(path string) *MCPServerRepository {
	return &MCPServerRepository{path: path}
}

func (r *MCPServerRepository) Create(_ context.Context, item *mcpmodel.Server) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.MCPServerID, mcpmodel.Server](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneMCPServer(item)
	return saveMap(r.path, items)
}

func (r *MCPServerRepository) Get(_ context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.MCPServerID, mcpmodel.Server](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneMCPServer(&item), nil
}

func (r *MCPServerRepository) List(_ context.Context) ([]*mcpmodel.Server, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.MCPServerID, mcpmodel.Server](r.path)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*mcpmodel.Server, 0, len(ids))
	for _, id := range ids {
		item := items[common.MCPServerID(id)]
		ret = append(ret, cloneMCPServer(&item))
	}
	return ret, nil
}

func (r *MCPServerRepository) Update(_ context.Context, item *mcpmodel.Server) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.MCPServerID, mcpmodel.Server](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneMCPServer(item)
	return saveMap(r.path, items)
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

func (r *WorkspaceRepository) List(_ context.Context) ([]*workspace.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.WorkspaceID, workspace.Workspace](r.path)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*workspace.Workspace, 0, len(ids))
	for _, id := range ids {
		item := items[common.WorkspaceID(id)]
		ret = append(ret, cloneWorkspace(&item))
	}
	return ret, nil
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

type GoalRepository struct {
	mu   sync.RWMutex
	path string
}

func NewGoalRepository(path string) *GoalRepository {
	return &GoalRepository{path: path}
}

func (r *GoalRepository) Create(_ context.Context, item *workspace.Goal) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.GoalID, workspace.Goal](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneGoal(item)
	return saveMap(r.path, items)
}

func (r *GoalRepository) Get(_ context.Context, id common.GoalID) (*workspace.Goal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.GoalID, workspace.Goal](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneGoal(&item), nil
}

func (r *GoalRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*workspace.Goal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.GoalID, workspace.Goal](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*workspace.Goal, 0, len(items))
	for _, item := range items {
		if item.WorkspaceID != workspaceID {
			continue
		}
		ret = append(ret, cloneGoal(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

type DoDRepository struct {
	mu   sync.RWMutex
	path string
}

func NewDoDRepository(path string) *DoDRepository {
	return &DoDRepository{path: path}
}

func (r *DoDRepository) Create(_ context.Context, item *workspace.DoD) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.DoDID, workspace.DoD](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneDoD(item)
	return saveMap(r.path, items)
}

func (r *DoDRepository) Get(_ context.Context, id common.DoDID) (*workspace.DoD, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.DoDID, workspace.DoD](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneDoD(&item), nil
}

func (r *DoDRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*workspace.DoD, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.DoDID, workspace.DoD](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*workspace.DoD, 0, len(items))
	for _, item := range items {
		if item.WorkspaceID != workspaceID {
			continue
		}
		ret = append(ret, cloneDoD(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *DoDRepository) Update(_ context.Context, item *workspace.DoD) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.DoDID, workspace.DoD](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneDoD(item)
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

func (r *RunRepository) List(_ context.Context) ([]*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.RunID, run.Run](r.path)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*run.Run, 0, len(ids))
	for _, id := range ids {
		item := items[common.RunID(id)]
		ret = append(ret, cloneRun(&item))
	}
	return ret, nil
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

type ToolCallRepository struct {
	mu   sync.RWMutex
	path string
}

func NewToolCallRepository(path string) *ToolCallRepository {
	return &ToolCallRepository{path: path}
}

func (r *ToolCallRepository) Create(_ context.Context, item *run.ToolCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.ToolCallID, run.ToolCall](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneToolCall(item)
	return saveMap(r.path, items)
}

func (r *ToolCallRepository) Get(_ context.Context, id common.ToolCallID) (*run.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ToolCallID, run.ToolCall](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneToolCall(&item), nil
}

func (r *ToolCallRepository) ListByRun(_ context.Context, runID common.RunID) ([]*run.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ToolCallID, run.ToolCall](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*run.ToolCall, 0, len(items))
	for _, item := range items {
		if item.RunID != runID {
			continue
		}
		ret = append(ret, cloneToolCall(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *ToolCallRepository) ListByStep(_ context.Context, stepID common.StepID) ([]*run.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ToolCallID, run.ToolCall](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*run.ToolCall, 0, len(items))
	for _, item := range items {
		if item.StepID != stepID {
			continue
		}
		ret = append(ret, cloneToolCall(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *ToolCallRepository) Update(_ context.Context, item *run.ToolCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.ToolCallID, run.ToolCall](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneToolCall(item)
	return saveMap(r.path, items)
}

type PatchRepository struct {
	mu   sync.RWMutex
	path string
}

func NewPatchRepository(path string) *PatchRepository {
	return &PatchRepository{path: path}
}

func (r *PatchRepository) Create(_ context.Context, item *run.Patch) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.PatchID, run.Patch](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *clonePatch(item)
	return saveMap(r.path, items)
}

func (r *PatchRepository) Get(_ context.Context, id common.PatchID) (*run.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.PatchID, run.Patch](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return clonePatch(&item), nil
}

func (r *PatchRepository) ListByRun(_ context.Context, runID common.RunID) ([]*run.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.PatchID, run.Patch](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*run.Patch, 0, len(items))
	for _, item := range items {
		if item.RunID != runID {
			continue
		}
		ret = append(ret, clonePatch(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *PatchRepository) ListByStep(_ context.Context, stepID common.StepID) ([]*run.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.PatchID, run.Patch](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*run.Patch, 0, len(items))
	for _, item := range items {
		if item.StepID != stepID {
			continue
		}
		ret = append(ret, clonePatch(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *PatchRepository) Update(_ context.Context, item *run.Patch) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.PatchID, run.Patch](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *clonePatch(item)
	return saveMap(r.path, items)
}

type ValidationResultRepository struct {
	mu   sync.RWMutex
	path string
}

func NewValidationResultRepository(path string) *ValidationResultRepository {
	return &ValidationResultRepository{path: path}
}

func (r *ValidationResultRepository) Create(_ context.Context, item *run.ValidationResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.ValidationResultID, run.ValidationResult](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneValidationResult(item)
	return saveMap(r.path, items)
}

func (r *ValidationResultRepository) Get(_ context.Context, id common.ValidationResultID) (*run.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ValidationResultID, run.ValidationResult](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneValidationResult(&item), nil
}

func (r *ValidationResultRepository) ListByRun(_ context.Context, runID common.RunID) ([]*run.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ValidationResultID, run.ValidationResult](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*run.ValidationResult, 0, len(items))
	for _, item := range items {
		if item.RunID != runID {
			continue
		}
		ret = append(ret, cloneValidationResult(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *ValidationResultRepository) ListByStep(_ context.Context, stepID common.StepID) ([]*run.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ValidationResultID, run.ValidationResult](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*run.ValidationResult, 0, len(items))
	for _, item := range items {
		if item.StepID != stepID {
			continue
		}
		ret = append(ret, cloneValidationResult(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *ValidationResultRepository) Update(_ context.Context, item *run.ValidationResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.ValidationResultID, run.ValidationResult](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneValidationResult(item)
	return saveMap(r.path, items)
}

type RuntimeCheckpointRepository struct {
	mu   sync.RWMutex
	path string
}

func NewRuntimeCheckpointRepository(path string) *RuntimeCheckpointRepository {
	return &RuntimeCheckpointRepository{path: path}
}

func (r *RuntimeCheckpointRepository) Create(_ context.Context, item *run.RuntimeCheckpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.CheckpointID, run.RuntimeCheckpoint](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneRuntimeCheckpoint(item)
	return saveMap(r.path, items)
}

func (r *RuntimeCheckpointRepository) Get(_ context.Context, id common.CheckpointID) (*run.RuntimeCheckpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.CheckpointID, run.RuntimeCheckpoint](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneRuntimeCheckpoint(&item), nil
}

func (r *RuntimeCheckpointRepository) ListByRun(_ context.Context, runID common.RunID) ([]*run.RuntimeCheckpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.CheckpointID, run.RuntimeCheckpoint](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*run.RuntimeCheckpoint, 0, len(items))
	for _, item := range items {
		if item.RunID != runID {
			continue
		}
		ret = append(ret, cloneRuntimeCheckpoint(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *RuntimeCheckpointRepository) Update(_ context.Context, item *run.RuntimeCheckpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.CheckpointID, run.RuntimeCheckpoint](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneRuntimeCheckpoint(item)
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

func (r *DecisionTicketRepository) List(_ context.Context) ([]*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.TicketID, decision.Ticket](r.path)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*decision.Ticket, 0, len(ids))
	for _, id := range ids {
		item := items[common.TicketID(id)]
		ret = append(ret, cloneDecisionTicket(&item))
	}
	return ret, nil
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

func (r *ApprovalRepository) List(_ context.Context) ([]*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ApprovalID, approval.Approval](r.path)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*approval.Approval, 0, len(ids))
	for _, id := range ids {
		item := items[common.ApprovalID(id)]
		ret = append(ret, cloneApproval(&item))
	}
	return ret, nil
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

func cloneGoal(item *workspace.Goal) *workspace.Goal {
	cloned := *item
	return &cloned
}

func cloneDoD(item *workspace.DoD) *workspace.DoD {
	cloned := *item
	if item.Items != nil {
		cloned.Items = append([]string(nil), item.Items...)
	}
	return &cloned
}

func cloneRun(item *run.Run) *run.Run {
	cloned := *item
	if item.ExecutionContext != nil {
		cloned.ExecutionContext = cloneMap(item.ExecutionContext)
	}
	return &cloned
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

func clonePatch(item *run.Patch) *run.Patch {
	cloned := *item
	return &cloned
}

func cloneValidationResult(item *run.ValidationResult) *run.ValidationResult {
	cloned := *item
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

func cloneDecisionResolution(item *decision.Resolution) *decision.Resolution {
	cloned := *item
	if item.Payload != nil {
		cloned.Payload = cloneMap(item.Payload)
	}
	return &cloned
}

func cloneApproval(item *approval.Approval) *approval.Approval {
	cloned := *item
	return &cloned
}

func cloneSoul(item *soul.Soul) *soul.Soul {
	cloned := *item
	cloned.BusinessDomains = cloneStringSlice(item.BusinessDomains)
	cloned.AllowedSkillRefs = cloneStringSlice(item.AllowedSkillRefs)
	cloned.AllowedMCPServerRefs = cloneStringSlice(item.AllowedMCPServerRefs)
	cloned.AllowedToolKinds = cloneStringSlice(item.AllowedToolKinds)
	return &cloned
}

func cloneMemoryItem(item *memorymodel.MemoryItem) *memorymodel.MemoryItem {
	cloned := *item
	cloned.Tags = cloneStringSlice(item.Tags)
	return &cloned
}

func cloneSkill(item *skillmodel.Skill) *skillmodel.Skill {
	cloned := *item
	cloned.TriggerRules = cloneStringSlice(item.TriggerRules)
	cloned.ReferenceRefs = cloneStringSlice(item.ReferenceRefs)
	cloned.RequiredToolKinds = cloneStringSlice(item.RequiredToolKinds)
	cloned.RequiredMCPServerRefs = cloneStringSlice(item.RequiredMCPServerRefs)
	cloned.Constraints = cloneStringSlice(item.Constraints)
	return &cloned
}

func cloneMCPServer(item *mcpmodel.Server) *mcpmodel.Server {
	cloned := *item
	cloned.AllowedTools = cloneStringSlice(item.AllowedTools)
	cloned.AllowedResources = cloneStringSlice(item.AllowedResources)
	cloned.AllowedPrompts = cloneStringSlice(item.AllowedPrompts)
	return &cloned
}

func memoryItemMatches(item *memorymodel.MemoryItem, query repository.MemorySearchQuery) bool {
	if query.WorkspaceID != "" && item.WorkspaceID != query.WorkspaceID {
		return false
	}
	if len(query.Types) > 0 && !stringIn(item.MemoryType, query.Types) {
		return false
	}
	for _, tag := range query.Tags {
		if !stringIn(tag, item.Tags) {
			return false
		}
	}
	if query.Query == "" {
		return true
	}
	needle := strings.ToLower(query.Query)
	return strings.Contains(strings.ToLower(item.Summary), needle) ||
		strings.Contains(strings.ToLower(item.Content), needle) ||
		strings.Contains(strings.ToLower(item.MemoryType), needle) ||
		containsTagText(item.Tags, needle)
}

func containsTagText(tags []string, needle string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), needle) {
			return true
		}
	}
	return false
}

func stringIn(value string, values []string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func cloneStringSlice(input []string) []string {
	if input == nil {
		return nil
	}
	return append([]string(nil), input...)
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
