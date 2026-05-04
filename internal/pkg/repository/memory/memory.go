package memory

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"

	approval "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decision "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstream "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	run "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soul "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type WorkspaceRepository struct {
	mu    sync.RWMutex
	items map[common.WorkspaceID]*workspace.Workspace
}

type WorkspaceTemplateRepository struct {
	mu    sync.RWMutex
	items map[string]*repository.WorkspaceTemplate
}

type SoulRepository struct {
	mu    sync.RWMutex
	items map[common.SoulID]*soul.Soul
}

type MemoryItemRepository struct {
	mu    sync.RWMutex
	items map[common.MemoryID]*memorymodel.MemoryItem
}

type SkillRepository struct {
	mu    sync.RWMutex
	items map[common.SkillID]*skillmodel.Skill
}

type MCPServerRepository struct {
	mu    sync.RWMutex
	items map[common.MCPServerID]*mcpmodel.Server
}

type MCPCapabilityRepository struct {
	mu    sync.RWMutex
	items map[common.MCPCapabilityID]*mcpmodel.Capability
}

type ResultRepository struct {
	mu    sync.RWMutex
	items map[common.ResultID]*resultmodel.Result
}

type ArtifactRepository struct {
	mu    sync.RWMutex
	items map[common.ArtifactID]*resultmodel.Artifact
}

type EventRecordRepository struct {
	mu    sync.RWMutex
	items map[common.EventRecordID]*eventstream.EventRecord
}

func NewSoulRepository() *SoulRepository {
	return &SoulRepository{items: map[common.SoulID]*soul.Soul{}}
}

func (r *SoulRepository) Create(_ context.Context, item *soul.Soul) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneSoul(item)
	return nil
}

func (r *SoulRepository) Get(_ context.Context, id common.SoulID) (*soul.Soul, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneSoul(item), nil
}

func (r *SoulRepository) List(_ context.Context) ([]*soul.Soul, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*soul.Soul, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, cloneSoul(r.items[common.SoulID(id)]))
	}
	return ret, nil
}

func (r *SoulRepository) Update(_ context.Context, item *soul.Soul) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneSoul(item)
	return nil
}

func (r *SoulRepository) Delete(_ context.Context, id common.SoulID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func NewWorkspaceTemplateRepository() *WorkspaceTemplateRepository {
	return &WorkspaceTemplateRepository{items: map[string]*repository.WorkspaceTemplate{}}
}

func (r *WorkspaceTemplateRepository) Create(_ context.Context, item *repository.WorkspaceTemplate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneWorkspaceTemplate(item)
	return nil
}

func (r *WorkspaceTemplateRepository) Get(_ context.Context, id string) (*repository.WorkspaceTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneWorkspaceTemplate(item), nil
}

func (r *WorkspaceTemplateRepository) List(_ context.Context) ([]*repository.WorkspaceTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	ret := make([]*repository.WorkspaceTemplate, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, cloneWorkspaceTemplate(r.items[id]))
	}
	return ret, nil
}

func (r *WorkspaceTemplateRepository) Update(_ context.Context, item *repository.WorkspaceTemplate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneWorkspaceTemplate(item)
	return nil
}

func (r *WorkspaceTemplateRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func NewMemoryItemRepository() *MemoryItemRepository {
	return &MemoryItemRepository{items: map[common.MemoryID]*memorymodel.MemoryItem{}}
}

func (r *MemoryItemRepository) Create(_ context.Context, item *memorymodel.MemoryItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneMemoryItem(item)
	return nil
}

func (r *MemoryItemRepository) Get(_ context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneMemoryItem(item), nil
}

func (r *MemoryItemRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ret := make([]*memorymodel.MemoryItem, 0)
	for _, item := range r.items {
		if item.WorkspaceID != workspaceID {
			continue
		}
		ret = append(ret, cloneMemoryItem(item))
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

	ret := make([]*memorymodel.MemoryItem, 0)
	for _, item := range r.items {
		if !memoryItemMatches(item, query) {
			continue
		}
		cloned := cloneMemoryItem(item)
		if len(query.Vector) > 0 {
			cloned.VectorScore = cosineSimilarity(query.Vector, item.Vector)
		}
		ret = append(ret, cloned)
	}
	sort.Slice(ret, func(i, j int) bool {
		if len(query.Vector) > 0 && ret[i].VectorScore != ret[j].VectorScore {
			return ret[i].VectorScore > ret[j].VectorScore
		}
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
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneMemoryItem(item)
	return nil
}

func (r *MemoryItemRepository) Delete(_ context.Context, id common.MemoryID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func NewSkillRepository() *SkillRepository {
	return &SkillRepository{items: map[common.SkillID]*skillmodel.Skill{}}
}

func (r *SkillRepository) Create(_ context.Context, item *skillmodel.Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneSkill(item)
	return nil
}

func (r *SkillRepository) Get(_ context.Context, id common.SkillID) (*skillmodel.Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneSkill(item), nil
}

func (r *SkillRepository) List(_ context.Context) ([]*skillmodel.Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*skillmodel.Skill, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, cloneSkill(r.items[common.SkillID(id)]))
	}
	return ret, nil
}

func (r *SkillRepository) Update(_ context.Context, item *skillmodel.Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneSkill(item)
	return nil
}

func NewMCPServerRepository() *MCPServerRepository {
	return &MCPServerRepository{items: map[common.MCPServerID]*mcpmodel.Server{}}
}

func (r *MCPServerRepository) Create(_ context.Context, item *mcpmodel.Server) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneMCPServer(item)
	return nil
}

func (r *MCPServerRepository) Get(_ context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneMCPServer(item), nil
}

func (r *MCPServerRepository) List(_ context.Context) ([]*mcpmodel.Server, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*mcpmodel.Server, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, cloneMCPServer(r.items[common.MCPServerID(id)]))
	}
	return ret, nil
}

func (r *MCPServerRepository) Update(_ context.Context, item *mcpmodel.Server) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneMCPServer(item)
	return nil
}

func NewMCPCapabilityRepository() *MCPCapabilityRepository {
	return &MCPCapabilityRepository{items: map[common.MCPCapabilityID]*mcpmodel.Capability{}}
}

func (r *MCPCapabilityRepository) Create(_ context.Context, item *mcpmodel.Capability) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneMCPCapability(item)
	return nil
}

func (r *MCPCapabilityRepository) Get(_ context.Context, id common.MCPCapabilityID) (*mcpmodel.Capability, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneMCPCapability(item), nil
}

func (r *MCPCapabilityRepository) List(_ context.Context) ([]*mcpmodel.Capability, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*mcpmodel.Capability, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, cloneMCPCapability(r.items[common.MCPCapabilityID(id)]))
	}
	return ret, nil
}

func (r *MCPCapabilityRepository) Update(_ context.Context, item *mcpmodel.Capability) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneMCPCapability(item)
	return nil
}

func NewResultRepository() *ResultRepository {
	return &ResultRepository{items: map[common.ResultID]*resultmodel.Result{}}
}

func (r *ResultRepository) Create(_ context.Context, item *resultmodel.Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneResult(item)
	return nil
}

func (r *ResultRepository) Get(_ context.Context, id common.ResultID) (*resultmodel.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneResult(item), nil
}

func (r *ResultRepository) ListByWorkspace(_ context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*resultmodel.Result, 0)
	for _, item := range r.items {
		if item.WorkspaceID != workspaceID {
			continue
		}
		items = append(items, cloneResult(item))
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *ResultRepository) ListByRun(_ context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*resultmodel.Result, 0)
	for _, item := range r.items {
		if item.RunID != runID {
			continue
		}
		items = append(items, cloneResult(item))
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *ResultRepository) Update(_ context.Context, item *resultmodel.Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneResult(item)
	return nil
}

func (r *ResultRepository) Delete(_ context.Context, id common.ResultID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func NewArtifactRepository() *ArtifactRepository {
	return &ArtifactRepository{items: map[common.ArtifactID]*resultmodel.Artifact{}}
}

func (r *ArtifactRepository) Create(_ context.Context, item *resultmodel.Artifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneArtifact(item)
	return nil
}

func (r *ArtifactRepository) Get(_ context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneArtifact(item), nil
}

func (r *ArtifactRepository) ListByWorkspace(_ context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return listArtifacts(r.items, func(item *resultmodel.Artifact) bool {
		return item.WorkspaceID == workspaceID
	}), nil
}

func (r *ArtifactRepository) ListByRun(_ context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return listArtifacts(r.items, func(item *resultmodel.Artifact) bool {
		return item.RunID == runID
	}), nil
}

func (r *ArtifactRepository) ListByResult(_ context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return listArtifacts(r.items, func(item *resultmodel.Artifact) bool {
		return item.ResultID == resultID
	}), nil
}

func (r *ArtifactRepository) Update(_ context.Context, item *resultmodel.Artifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneArtifact(item)
	return nil
}

func (r *ArtifactRepository) Delete(_ context.Context, id common.ArtifactID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func NewEventRecordRepository() *EventRecordRepository {
	return &EventRecordRepository{items: map[common.EventRecordID]*eventstream.EventRecord{}}
}

func (r *EventRecordRepository) Append(_ context.Context, item *eventstream.EventRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneEventRecord(item)
	return nil
}

func (r *EventRecordRepository) Get(_ context.Context, id common.EventRecordID) (*eventstream.EventRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneEventRecord(item), nil
}

func (r *EventRecordRepository) List(_ context.Context, query repository.EventRecordQuery) ([]*eventstream.EventRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return listEventRecords(r.items, query), nil
}

func (r *EventRecordRepository) Delete(_ context.Context, id common.EventRecordID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
	return nil
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
	r.items[item.ID] = cloneWorkspace(item)
	return nil
}

func (r *WorkspaceRepository) Get(_ context.Context, id common.WorkspaceID) (*workspace.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneWorkspace(item), nil
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
		ret = append(ret, cloneWorkspace(r.items[common.WorkspaceID(id)]))
	}
	return ret, nil
}

func (r *WorkspaceRepository) Update(_ context.Context, item *workspace.Workspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneWorkspace(item)
	return nil
}

func (r *WorkspaceRepository) Delete(_ context.Context, id common.WorkspaceID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func cloneWorkspace(item *workspace.Workspace) *workspace.Workspace {
	cloned := *item
	if item.SoulBindings != nil {
		cloned.SoulBindings = map[string]common.SoulID{}
		for key, value := range item.SoulBindings {
			cloned.SoulBindings[key] = value
		}
	}
	return &cloned
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
	r.items[item.ID] = cloneGoal(item)
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

func (r *GoalRepository) Update(_ context.Context, item *workspace.Goal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneGoal(item)
	return nil
}

func (r *GoalRepository) Delete(_ context.Context, id common.GoalID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
	return nil
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

func (r *DoDRepository) Delete(_ context.Context, id common.DoDID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
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
	r.items[item.ID] = cloneRun(item)
	return nil
}

func (r *RunRepository) Get(_ context.Context, id common.RunID) (*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneRun(item), nil
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
		ret = append(ret, cloneRun(r.items[common.RunID(id)]))
	}
	return ret, nil
}

func (r *RunRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id, item := range r.items {
		if item.WorkspaceID == workspaceID {
			ids = append(ids, string(id))
		}
	}
	sort.Strings(ids)

	ret := make([]*run.Run, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, cloneRun(r.items[common.RunID(id)]))
	}
	return ret, nil
}

func (r *RunRepository) Update(_ context.Context, item *run.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneRun(item)
	return nil
}

func (r *RunRepository) Delete(_ context.Context, id common.RunID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
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

func (r *StepRepository) Delete(_ context.Context, id common.StepID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
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

func (r *ToolCallRepository) Delete(_ context.Context, id common.ToolCallID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
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
	cloned := clonePatch(item)
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
	cloned := clonePatch(item)
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
		cloned := clonePatch(item)
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
		cloned := clonePatch(item)
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
	cloned := clonePatch(item)
	r.items[item.ID] = &cloned
	return nil
}

func (r *PatchRepository) Delete(_ context.Context, id common.PatchID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
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

func (r *ValidationResultRepository) Delete(_ context.Context, id common.ValidationResultID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
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

func (r *RuntimeCheckpointRepository) Delete(_ context.Context, id common.CheckpointID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
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

func (r *DecisionTicketRepository) ListByRunID(_ context.Context, runID common.RunID) ([]*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id, item := range r.items {
		if item.RunID == runID {
			ids = append(ids, string(id))
		}
	}
	sort.Strings(ids)

	ret := make([]*decision.Ticket, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, cloneDecisionTicket(r.items[common.TicketID(id)]))
	}
	return ret, nil
}

func (r *DecisionTicketRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id, item := range r.items {
		if item.WorkspaceID == workspaceID {
			ids = append(ids, string(id))
		}
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

func (r *DecisionTicketRepository) Delete(_ context.Context, id common.TicketID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
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

func (r *DecisionResolutionRepository) DeleteByTicketID(_ context.Context, ticketID common.TicketID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byTicket[ticketID]; !ok {
		return common.ErrNotFound
	}
	delete(r.byTicket, ticketID)
	return nil
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
	cloned := cloneApproval(item)
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
	cloned := cloneApproval(item)
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
		cloned := cloneApproval(r.items[common.ApprovalID(id)])
		ret = append(ret, &cloned)
	}
	return ret, nil
}

func (r *ApprovalRepository) ListByRunID(_ context.Context, runID common.RunID) ([]*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id, item := range r.items {
		if item.RunID == runID {
			ids = append(ids, string(id))
		}
	}
	sort.Strings(ids)

	ret := make([]*approval.Approval, 0, len(ids))
	for _, id := range ids {
		cloned := cloneApproval(r.items[common.ApprovalID(id)])
		ret = append(ret, &cloned)
	}
	return ret, nil
}

func (r *ApprovalRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id, item := range r.items {
		if item.WorkspaceID == workspaceID {
			ids = append(ids, string(id))
		}
	}
	sort.Strings(ids)

	ret := make([]*approval.Approval, 0, len(ids))
	for _, id := range ids {
		cloned := cloneApproval(r.items[common.ApprovalID(id)])
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
	cloned := cloneApproval(item)
	r.items[item.ID] = &cloned
	return nil
}

func (r *ApprovalRepository) Delete(_ context.Context, id common.ApprovalID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
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

func cloneRun(item *run.Run) *run.Run {
	cloned := *item
	if item.ExecutionContext != nil {
		cloned.ExecutionContext = cloneMap(item.ExecutionContext)
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

func cloneResult(item *resultmodel.Result) *resultmodel.Result {
	cloned := *item
	cloned.EvidenceRefs = cloneStringSlice(item.EvidenceRefs)
	if item.ArtifactRefs != nil {
		cloned.ArtifactRefs = append([]common.ArtifactID(nil), item.ArtifactRefs...)
	}
	cloned.DoDAssessment = cloneResultDoDAssessment(item.DoDAssessment)
	return &cloned
}

func cloneResultDoDAssessment(item resultmodel.DoDAssessment) resultmodel.DoDAssessment {
	cloned := item
	if item.Items != nil {
		cloned.Items = make([]resultmodel.DoDAssessmentItem, 0, len(item.Items))
		for _, assessmentItem := range item.Items {
			assessmentItem.EvidenceRefs = cloneStringSlice(assessmentItem.EvidenceRefs)
			cloned.Items = append(cloned.Items, assessmentItem)
		}
	}
	return cloned
}

func cloneArtifact(item *resultmodel.Artifact) *resultmodel.Artifact {
	cloned := *item
	cloned.EvidenceRefs = cloneStringSlice(item.EvidenceRefs)
	cloned.Metadata = cloneMap(item.Metadata)
	return &cloned
}

func cloneEventRecord(item *eventstream.EventRecord) *eventstream.EventRecord {
	cloned := *item
	cloned.Payload = cloneMap(item.Payload)
	return &cloned
}

func cloneDecisionTicket(item *decision.Ticket) *decision.Ticket {
	cloned := *item
	cloned.Context = cloneMap(item.Context)
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

func cloneSoul(item *soul.Soul) *soul.Soul {
	cloned := *item
	cloned.BusinessDomains = cloneStringSlice(item.BusinessDomains)
	cloned.AllowedSkillRefs = cloneStringSlice(item.AllowedSkillRefs)
	cloned.AllowedMCPServerRefs = cloneStringSlice(item.AllowedMCPServerRefs)
	cloned.AllowedCapabilityRefs = cloneStringSlice(item.AllowedCapabilityRefs)
	cloned.AllowedCapabilityCategories = cloneStringSlice(item.AllowedCapabilityCategories)
	cloned.AllowedToolKinds = cloneStringSlice(item.AllowedToolKinds)
	return &cloned
}

func cloneWorkspaceTemplate(item *repository.WorkspaceTemplate) *repository.WorkspaceTemplate {
	cloned := *item
	cloned.DoDItems = cloneStringSlice(item.DoDItems)
	if item.SoulBindings != nil {
		cloned.SoulBindings = map[string]string{}
		for key, value := range item.SoulBindings {
			cloned.SoulBindings[key] = value
		}
	}
	return &cloned
}

func cloneMemoryItem(item *memorymodel.MemoryItem) *memorymodel.MemoryItem {
	cloned := *item
	cloned.Tags = cloneStringSlice(item.Tags)
	cloned.Vector = cloneFloatSlice(item.Vector)
	return &cloned
}

func cloneSkill(item *skillmodel.Skill) *skillmodel.Skill {
	cloned := *item
	cloned.TriggerRules = cloneStringSlice(item.TriggerRules)
	cloned.ReferenceRefs = cloneStringSlice(item.ReferenceRefs)
	cloned.RequiredToolKinds = cloneStringSlice(item.RequiredToolKinds)
	cloned.RequiredMCPServerRefs = cloneStringSlice(item.RequiredMCPServerRefs)
	cloned.RequiredCapabilityRefs = cloneStringSlice(item.RequiredCapabilityRefs)
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

func cloneMCPCapability(item *mcpmodel.Capability) *mcpmodel.Capability {
	cloned := *item
	cloned.Tags = cloneStringSlice(item.Tags)
	cloned.StageHints = cloneStringSlice(item.StageHints)
	cloned.Schema = cloneAnyMap(item.Schema)
	return &cloned
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func listArtifacts(items map[common.ArtifactID]*resultmodel.Artifact, matches func(*resultmodel.Artifact) bool) []*resultmodel.Artifact {
	ret := make([]*resultmodel.Artifact, 0)
	for _, item := range items {
		if !matches(item) {
			continue
		}
		ret = append(ret, cloneArtifact(item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func listEventRecords(items map[common.EventRecordID]*eventstream.EventRecord, query repository.EventRecordQuery) []*eventstream.EventRecord {
	ret := make([]*eventstream.EventRecord, 0)
	cursorMatched := query.CursorAfterID == ""
	for _, item := range items {
		if !eventRecordMatches(item, query) {
			continue
		}
		ret = append(ret, cloneEventRecord(item))
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].OccurredAt == ret[j].OccurredAt {
			return string(ret[i].ID) < string(ret[j].ID)
		}
		return ret[i].OccurredAt < ret[j].OccurredAt
	})
	if query.CursorAfterID != "" {
		filtered := ret[:0]
		for _, item := range ret {
			if !cursorMatched {
				if item.ID == query.CursorAfterID {
					cursorMatched = true
				}
				continue
			}
			filtered = append(filtered, item)
		}
		ret = filtered
	}
	if query.Limit > 0 && len(ret) > query.Limit {
		ret = ret[:query.Limit]
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func eventRecordMatches(item *eventstream.EventRecord, query repository.EventRecordQuery) bool {
	if item == nil {
		return false
	}
	if query.WorkspaceID != "" && item.WorkspaceID != query.WorkspaceID {
		return false
	}
	if query.RunID != "" && item.RunID != query.RunID {
		return false
	}
	if query.Topic != "" && item.Topic != query.Topic {
		return false
	}
	if query.SubjectType != "" && item.SubjectType != query.SubjectType {
		return false
	}
	if query.SubjectID != "" && item.SubjectID != query.SubjectID {
		return false
	}
	if query.OccurredAfter != "" && item.OccurredAt <= query.OccurredAfter {
		return false
	}
	if query.OccurredBefore != "" && item.OccurredAt >= query.OccurredBefore {
		return false
	}
	return true
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
	if len(query.Vector) > 0 {
		if len(item.Vector) != len(query.Vector) || len(item.Vector) == 0 {
			return false
		}
		if query.MinScore > 0 && cosineSimilarity(query.Vector, item.Vector) < query.MinScore {
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

func cloneFloatSlice(input []float64) []float64 {
	if input == nil {
		return nil
	}
	return append([]float64(nil), input...)
}

func cosineSimilarity(left []float64, right []float64) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for idx := range left {
		dot += left[idx] * right[idx]
		leftNorm += left[idx] * left[idx]
		rightNorm += right[idx] * right[idx]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
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

func clonePatch(item *run.Patch) run.Patch {
	return *item
}

func cloneApproval(item *approval.Approval) approval.Approval {
	cloned := *item
	cloned.Context = cloneMap(item.Context)
	return cloned
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
