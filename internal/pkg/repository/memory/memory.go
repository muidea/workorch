package memory

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	approval "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
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
	items map[string]*memorymodel.MemoryItem
}

type MemoryRecallTraceRepository struct {
	mu    sync.RWMutex
	items map[string]*memorymodel.MemoryRecallTrace
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
	items map[string]*resultmodel.Result
}

type ArtifactRepository struct {
	mu    sync.RWMutex
	items map[string]*resultmodel.Artifact
}

type EventRecordRepository struct {
	mu    sync.RWMutex
	items map[string]*eventstream.EventRecord
}

func workspaceScopedKey(workspaceID common.WorkspaceID, id string) string {
	id = strings.TrimSpace(id)
	if workspaceID == "" {
		return "/" + id
	}
	return string(workspaceID) + "/" + id
}

func runtimeWorkspaceScopedKey(ctx context.Context, id string) (string, bool) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return "", false
	}
	return workspaceScopedKey(workspaceID, id), true
}

func workspaceFromScopedKey(key string) common.WorkspaceID {
	workspaceID, _, ok := strings.Cut(key, "/")
	if !ok {
		return ""
	}
	return common.WorkspaceID(workspaceID)
}

func findScopedMemoryKey[T any](ctx context.Context, items map[string]T, id string, itemID func(T) string) (string, error) {
	if key, scoped := runtimeWorkspaceScopedKey(ctx, id); scoped {
		item, ok := items[key]
		if ok && itemID(item) == id {
			return key, nil
		}
		bareKey := workspaceScopedKey("", id)
		item, ok = items[bareKey]
		if ok && itemID(item) == id {
			return bareKey, nil
		}
		return "", common.ErrNotFound
	}
	var found string
	for key, item := range items {
		if itemID(item) != id {
			continue
		}
		if found != "" {
			return "", common.ErrConflict
		}
		found = key
	}
	if found == "" {
		return "", common.ErrNotFound
	}
	return found, nil
}

func runScopedListHasConflict[T any](ctx context.Context, items map[string]T, runID common.RunID, itemRunID func(T) common.RunID, itemWorkspaceID func(T) common.WorkspaceID) ([]T, bool) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	matchedWorkspaces := map[common.WorkspaceID]struct{}{}
	ret := make([]T, 0)
	for _, item := range items {
		if itemRunID(item) != runID {
			continue
		}
		currentWorkspaceID := itemWorkspaceID(item)
		if workspaceID != "" && currentWorkspaceID != "" && currentWorkspaceID != workspaceID {
			continue
		}
		matchedWorkspaces[currentWorkspaceID] = struct{}{}
		ret = append(ret, item)
	}
	return ret, workspaceID == "" && len(matchedWorkspaces) > 1
}

func stepScopedListHasConflict[T any](ctx context.Context, items map[string]T, stepID common.StepID, itemStepID func(T) common.StepID, itemWorkspaceID func(T) common.WorkspaceID) ([]T, bool) {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	matchedWorkspaces := map[common.WorkspaceID]struct{}{}
	ret := make([]T, 0)
	for _, item := range items {
		if itemStepID(item) != stepID {
			continue
		}
		currentWorkspaceID := itemWorkspaceID(item)
		if workspaceID != "" && currentWorkspaceID != "" && currentWorkspaceID != workspaceID {
			continue
		}
		matchedWorkspaces[currentWorkspaceID] = struct{}{}
		ret = append(ret, item)
	}
	return ret, workspaceID == "" && len(matchedWorkspaces) > 1
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
	return &MemoryItemRepository{items: map[string]*memorymodel.MemoryItem{}}
}

func (r *MemoryItemRepository) Create(_ context.Context, item *memorymodel.MemoryItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneMemoryItem(item)
	return nil
}

func (r *MemoryItemRepository) Get(ctx context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *memorymodel.MemoryItem) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	return cloneMemoryItem(r.items[key]), nil
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

func (r *MemoryItemRepository) Update(ctx context.Context, item *memorymodel.MemoryItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *memorymodel.MemoryItem) string {
			return string(current.ID)
		})
		if err != nil {
			return err
		}
		key = found
		item.WorkspaceID = r.items[key].WorkspaceID
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	r.items[key] = cloneMemoryItem(item)
	return nil
}

func (r *MemoryItemRepository) Delete(ctx context.Context, id common.MemoryID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *memorymodel.MemoryItem) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
	return nil
}

func NewMemoryRecallTraceRepository() *MemoryRecallTraceRepository {
	return &MemoryRecallTraceRepository{items: map[string]*memorymodel.MemoryRecallTrace{}}
}

func (r *MemoryRecallTraceRepository) Create(_ context.Context, item *memorymodel.MemoryRecallTrace) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneMemoryRecallTrace(item)
	return nil
}

func (r *MemoryRecallTraceRepository) Upsert(_ context.Context, item *memorymodel.MemoryRecallTrace) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[workspaceScopedKey(item.WorkspaceID, string(item.ID))] = cloneMemoryRecallTrace(item)
	return nil
}

func (r *MemoryRecallTraceRepository) Get(ctx context.Context, id common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *memorymodel.MemoryRecallTrace) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	return cloneMemoryRecallTrace(r.items[key]), nil
}

func (r *MemoryRecallTraceRepository) List(_ context.Context, query repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ret := make([]*memorymodel.MemoryRecallTrace, 0)
	for _, item := range r.items {
		if !memoryRecallTraceMatches(item, query) {
			continue
		}
		ret = append(ret, cloneMemoryRecallTrace(item))
	}
	sortMemoryRecallTraces(ret)
	if query.Limit > 0 && len(ret) > query.Limit {
		ret = ret[:query.Limit]
	}
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *MemoryRecallTraceRepository) Delete(ctx context.Context, id common.MemoryRecallTraceID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *memorymodel.MemoryRecallTrace) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
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
	return &ResultRepository{items: map[string]*resultmodel.Result{}}
}

func (r *ResultRepository) Create(_ context.Context, item *resultmodel.Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneResult(item)
	return nil
}

func (r *ResultRepository) Get(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *resultmodel.Result) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	return cloneResult(r.items[key]), nil
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

func (r *ResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := runScopedListHasConflict(ctx, r.items, runID, func(item *resultmodel.Result) common.RunID {
		return item.RunID
	}, func(item *resultmodel.Result) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*resultmodel.Result, 0, len(matches))
	for _, item := range matches {
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

func (r *ResultRepository) Update(ctx context.Context, item *resultmodel.Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *resultmodel.Result) string {
			return string(current.ID)
		})
		if err != nil {
			return err
		}
		key = found
		item.WorkspaceID = r.items[key].WorkspaceID
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	r.items[key] = cloneResult(item)
	return nil
}

func (r *ResultRepository) Delete(ctx context.Context, id common.ResultID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *resultmodel.Result) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
	return nil
}

func NewArtifactRepository() *ArtifactRepository {
	return &ArtifactRepository{items: map[string]*resultmodel.Artifact{}}
}

func (r *ArtifactRepository) Create(_ context.Context, item *resultmodel.Artifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneArtifact(item)
	return nil
}

func (r *ArtifactRepository) Get(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *resultmodel.Artifact) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	return cloneArtifact(r.items[key]), nil
}

func (r *ArtifactRepository) ListByWorkspace(_ context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return listArtifacts(r.items, func(item *resultmodel.Artifact) bool {
		return item.WorkspaceID == workspaceID
	}), nil
}

func (r *ArtifactRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	matches, conflict := runScopedListHasConflict(ctx, r.items, runID, func(item *resultmodel.Artifact) common.RunID {
		return item.RunID
	}, func(item *resultmodel.Artifact) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	return listArtifactSlice(matches), nil
}

func (r *ArtifactRepository) ListByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	matchedWorkspaces := map[common.WorkspaceID]struct{}{}
	ret := make([]*resultmodel.Artifact, 0)
	for _, item := range r.items {
		if item.ResultID != resultID {
			continue
		}
		if workspaceID != "" && item.WorkspaceID != workspaceID {
			continue
		}
		matchedWorkspaces[item.WorkspaceID] = struct{}{}
		ret = append(ret, item)
	}
	if workspaceID == "" && len(matchedWorkspaces) > 1 {
		return nil, common.ErrConflict
	}
	return listArtifactSlice(ret), nil
}

func (r *ArtifactRepository) Update(ctx context.Context, item *resultmodel.Artifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *resultmodel.Artifact) string {
			return string(current.ID)
		})
		if err != nil {
			return err
		}
		key = found
		item.WorkspaceID = r.items[key].WorkspaceID
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	r.items[key] = cloneArtifact(item)
	return nil
}

func (r *ArtifactRepository) Delete(ctx context.Context, id common.ArtifactID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *resultmodel.Artifact) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
	return nil
}

func NewEventRecordRepository() *EventRecordRepository {
	return &EventRecordRepository{items: map[string]*eventstream.EventRecord{}}
}

func (r *EventRecordRepository) Append(_ context.Context, item *eventstream.EventRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneEventRecord(item)
	return nil
}

func (r *EventRecordRepository) Get(ctx context.Context, id common.EventRecordID) (*eventstream.EventRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *eventstream.EventRecord) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	return cloneEventRecord(r.items[key]), nil
}

func (r *EventRecordRepository) List(ctx context.Context, query repository.EventRecordQuery) ([]*eventstream.EventRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if query.WorkspaceID == "" {
		query.WorkspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	if query.WorkspaceID == "" && query.RunID != "" {
		matchedWorkspaces := map[common.WorkspaceID]struct{}{}
		for _, item := range r.items {
			if item.RunID == query.RunID {
				matchedWorkspaces[item.WorkspaceID] = struct{}{}
			}
		}
		if len(matchedWorkspaces) > 1 {
			return nil, common.ErrConflict
		}
	}
	return listEventRecords(r.items, query), nil
}

func (r *EventRecordRepository) Delete(ctx context.Context, id common.EventRecordID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *eventstream.EventRecord) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
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
	cloned.ContextHints = cloneMap(item.ContextHints)
	cloned.EvidencePolicies = cloneMapSlice(item.EvidencePolicies)
	return &cloned
}

type GoalRepository struct {
	mu    sync.RWMutex
	items map[string]*workspace.Goal
}

func NewGoalRepository() *GoalRepository {
	return &GoalRepository{items: map[string]*workspace.Goal{}}
}

func (r *GoalRepository) Create(_ context.Context, item *workspace.Goal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneGoal(item)
	return nil
}

func (r *GoalRepository) Get(ctx context.Context, id common.GoalID) (*workspace.Goal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *workspace.Goal) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	cloned := *r.items[key]
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

func (r *GoalRepository) Update(ctx context.Context, item *workspace.Goal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *workspace.Goal) string {
			return string(current.ID)
		})
		if err != nil {
			return err
		}
		key = found
		item.WorkspaceID = r.items[key].WorkspaceID
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	r.items[key] = cloneGoal(item)
	return nil
}

func (r *GoalRepository) Delete(ctx context.Context, id common.GoalID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *workspace.Goal) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
	return nil
}

type DoDRepository struct {
	mu    sync.RWMutex
	items map[string]*workspace.DoD
}

func NewDoDRepository() *DoDRepository {
	return &DoDRepository{items: map[string]*workspace.DoD{}}
}

func (r *DoDRepository) Create(_ context.Context, item *workspace.DoD) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneDoD(item)
	return nil
}

func (r *DoDRepository) Get(ctx context.Context, id common.DoDID) (*workspace.DoD, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *workspace.DoD) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	return cloneDoD(r.items[key]), nil
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

func (r *DoDRepository) Update(ctx context.Context, item *workspace.DoD) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *workspace.DoD) string {
			return string(current.ID)
		})
		if err != nil {
			return err
		}
		key = found
		item.WorkspaceID = r.items[key].WorkspaceID
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	r.items[key] = cloneDoD(item)
	return nil
}

func (r *DoDRepository) Delete(ctx context.Context, id common.DoDID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *workspace.DoD) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
	return nil
}

type RunRepository struct {
	mu    sync.RWMutex
	items map[string]*run.Run
}

func NewRunRepository() *RunRepository {
	return &RunRepository{items: map[string]*run.Run{}}
}

func (r *RunRepository) Create(_ context.Context, item *run.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneRun(item)
	return nil
}

func (r *RunRepository) Get(ctx context.Context, id common.RunID) (*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if key, scoped := runtimeWorkspaceScopedKey(ctx, string(id)); scoped {
		item, ok := r.items[key]
		if !ok {
			return nil, common.ErrNotFound
		}
		return cloneRun(item), nil
	}
	var ret *run.Run
	for _, item := range r.items {
		if item.ID != id {
			continue
		}
		if ret != nil {
			return nil, common.ErrConflict
		}
		ret = item
	}
	if ret == nil {
		return nil, common.ErrNotFound
	}
	return cloneRun(ret), nil
}

func (r *RunRepository) List(_ context.Context) ([]*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for key := range r.items {
		ids = append(ids, key)
	}
	sort.Strings(ids)

	ret := make([]*run.Run, 0, len(ids))
	for _, key := range ids {
		ret = append(ret, cloneRun(r.items[key]))
	}
	return ret, nil
}

func (r *RunRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for key, item := range r.items {
		if item.WorkspaceID == workspaceID {
			ids = append(ids, key)
		}
	}
	sort.Strings(ids)

	ret := make([]*run.Run, 0, len(ids))
	for _, key := range ids {
		ret = append(ret, cloneRun(r.items[key]))
	}
	return ret, nil
}

func (r *RunRepository) Update(ctx context.Context, item *run.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		if scopedKey, scoped := runtimeWorkspaceScopedKey(ctx, string(item.ID)); scoped {
			key = scopedKey
		} else {
			var found string
			for currentKey, current := range r.items {
				if current.ID != item.ID {
					continue
				}
				if found != "" {
					return common.ErrConflict
				}
				found = currentKey
			}
			if found == "" {
				return common.ErrNotFound
			}
			key = found
			item.WorkspaceID = r.items[key].WorkspaceID
		}
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	r.items[key] = cloneRun(item)
	return nil
}

func (r *RunRepository) Delete(ctx context.Context, id common.RunID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if key, scoped := runtimeWorkspaceScopedKey(ctx, string(id)); scoped {
		if _, ok := r.items[key]; !ok {
			return common.ErrNotFound
		}
		delete(r.items, key)
		return nil
	}
	var found string
	for key, item := range r.items {
		if item.ID != id {
			continue
		}
		if found != "" {
			return common.ErrConflict
		}
		found = key
	}
	if found == "" {
		return common.ErrNotFound
	}
	delete(r.items, found)
	return nil
}

type StepRepository struct {
	mu    sync.RWMutex
	items map[string]*run.Step
}

func NewStepRepository() *StepRepository {
	return &StepRepository{items: map[string]*run.Step{}}
}

func (r *StepRepository) Create(ctx context.Context, item *run.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	key := workspaceScopedKey(workspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	cloned := cloneStep(item)
	r.items[key] = cloned
	return nil
}

func (r *StepRepository) Get(ctx context.Context, id common.StepID) (*run.Step, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *run.Step) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	return cloneStep(r.items[key]), nil
}

func (r *StepRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*run.Step, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var items []*run.Step
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	matchedWorkspaces := map[common.WorkspaceID]struct{}{}
	for key, item := range r.items {
		if item.RunID != runID {
			continue
		}
		itemWorkspaceID := workspaceFromScopedKey(key)
		if workspaceID != "" && itemWorkspaceID != "" && itemWorkspaceID != workspaceID {
			continue
		}
		matchedWorkspaces[itemWorkspaceID] = struct{}{}
		items = append(items, cloneStep(item))
	}
	if workspaceID == "" && len(matchedWorkspaces) > 1 {
		return nil, common.ErrConflict
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *StepRepository) Update(ctx context.Context, item *run.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *run.Step) string {
		return string(current.ID)
	})
	if err != nil {
		return err
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	r.items[key] = cloneStep(item)
	return nil
}

func (r *StepRepository) Delete(ctx context.Context, id common.StepID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *run.Step) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
	return nil
}

type ToolCallRepository struct {
	mu    sync.RWMutex
	items map[string]*run.ToolCall
}

func NewToolCallRepository() *ToolCallRepository {
	return &ToolCallRepository{items: map[string]*run.ToolCall{}}
}

func (r *ToolCallRepository) Create(_ context.Context, item *run.ToolCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneToolCall(item)
	return nil
}

func (r *ToolCallRepository) Get(ctx context.Context, id common.ToolCallID) (*run.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *run.ToolCall) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	return cloneToolCall(r.items[key]), nil
}

func (r *ToolCallRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*run.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := runScopedListHasConflict(ctx, r.items, runID, func(item *run.ToolCall) common.RunID {
		return item.RunID
	}, func(item *run.ToolCall) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*run.ToolCall, 0, len(matches))
	for _, item := range matches {
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

func (r *ToolCallRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*run.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := stepScopedListHasConflict(ctx, r.items, stepID, func(item *run.ToolCall) common.StepID {
		return item.StepID
	}, func(item *run.ToolCall) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*run.ToolCall, 0, len(matches))
	for _, item := range matches {
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

func (r *ToolCallRepository) Update(ctx context.Context, item *run.ToolCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *run.ToolCall) string {
			return string(current.ID)
		})
		if err != nil {
			return err
		}
		key = found
		item.WorkspaceID = r.items[key].WorkspaceID
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	r.items[key] = cloneToolCall(item)
	return nil
}

func (r *ToolCallRepository) Delete(ctx context.Context, id common.ToolCallID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *run.ToolCall) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
	return nil
}

type PatchRepository struct {
	mu    sync.RWMutex
	items map[string]*run.Patch
}

func NewPatchRepository() *PatchRepository {
	return &PatchRepository{items: map[string]*run.Patch{}}
}

func (r *PatchRepository) Create(_ context.Context, item *run.Patch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	cloned := clonePatch(item)
	r.items[key] = &cloned
	return nil
}

func (r *PatchRepository) Get(ctx context.Context, id common.PatchID) (*run.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *run.Patch) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	cloned := clonePatch(r.items[key])
	return &cloned, nil
}

func (r *PatchRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*run.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := runScopedListHasConflict(ctx, r.items, runID, func(item *run.Patch) common.RunID {
		return item.RunID
	}, func(item *run.Patch) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*run.Patch, 0, len(matches))
	for _, item := range matches {
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

func (r *PatchRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*run.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := stepScopedListHasConflict(ctx, r.items, stepID, func(item *run.Patch) common.StepID {
		return item.StepID
	}, func(item *run.Patch) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*run.Patch, 0, len(matches))
	for _, item := range matches {
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

func (r *PatchRepository) Update(ctx context.Context, item *run.Patch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *run.Patch) string {
			return string(current.ID)
		})
		if err != nil {
			return err
		}
		key = found
		item.WorkspaceID = r.items[key].WorkspaceID
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	cloned := clonePatch(item)
	r.items[key] = &cloned
	return nil
}

func (r *PatchRepository) Delete(ctx context.Context, id common.PatchID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *run.Patch) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
	return nil
}

type ValidationResultRepository struct {
	mu    sync.RWMutex
	items map[string]*run.ValidationResult
}

func NewValidationResultRepository() *ValidationResultRepository {
	return &ValidationResultRepository{items: map[string]*run.ValidationResult{}}
}

func (r *ValidationResultRepository) Create(_ context.Context, item *run.ValidationResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneValidationResult(item)
	return nil
}

func (r *ValidationResultRepository) Get(ctx context.Context, id common.ValidationResultID) (*run.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *run.ValidationResult) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	return cloneValidationResult(r.items[key]), nil
}

func (r *ValidationResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*run.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := runScopedListHasConflict(ctx, r.items, runID, func(item *run.ValidationResult) common.RunID {
		return item.RunID
	}, func(item *run.ValidationResult) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*run.ValidationResult, 0, len(matches))
	for _, item := range matches {
		items = append(items, cloneValidationResult(item))
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *ValidationResultRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*run.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := stepScopedListHasConflict(ctx, r.items, stepID, func(item *run.ValidationResult) common.StepID {
		return item.StepID
	}, func(item *run.ValidationResult) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*run.ValidationResult, 0, len(matches))
	for _, item := range matches {
		items = append(items, cloneValidationResult(item))
	}
	sort.Slice(items, func(i, j int) bool {
		return string(items[i].ID) < string(items[j].ID)
	})
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func (r *ValidationResultRepository) Update(ctx context.Context, item *run.ValidationResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *run.ValidationResult) string {
			return string(current.ID)
		})
		if err != nil {
			return err
		}
		key = found
		item.WorkspaceID = r.items[key].WorkspaceID
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	r.items[key] = cloneValidationResult(item)
	return nil
}

func (r *ValidationResultRepository) Delete(ctx context.Context, id common.ValidationResultID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *run.ValidationResult) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
	return nil
}

type RuntimeCheckpointRepository struct {
	mu    sync.RWMutex
	items map[string]*run.RuntimeCheckpoint
}

func NewRuntimeCheckpointRepository() *RuntimeCheckpointRepository {
	return &RuntimeCheckpointRepository{items: map[string]*run.RuntimeCheckpoint{}}
}

func (r *RuntimeCheckpointRepository) Create(_ context.Context, item *run.RuntimeCheckpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneRuntimeCheckpoint(item)
	return nil
}

func (r *RuntimeCheckpointRepository) Get(ctx context.Context, id common.CheckpointID) (*run.RuntimeCheckpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *run.RuntimeCheckpoint) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	return cloneRuntimeCheckpoint(r.items[key]), nil
}

func (r *RuntimeCheckpointRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*run.RuntimeCheckpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := runScopedListHasConflict(ctx, r.items, runID, func(item *run.RuntimeCheckpoint) common.RunID {
		return item.RunID
	}, func(item *run.RuntimeCheckpoint) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*run.RuntimeCheckpoint, 0, len(matches))
	for _, item := range matches {
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

func (r *RuntimeCheckpointRepository) Update(ctx context.Context, item *run.RuntimeCheckpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *run.RuntimeCheckpoint) string {
			return string(current.ID)
		})
		if err != nil {
			return err
		}
		key = found
		item.WorkspaceID = r.items[key].WorkspaceID
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	r.items[key] = cloneRuntimeCheckpoint(item)
	return nil
}

func (r *RuntimeCheckpointRepository) Delete(ctx context.Context, id common.CheckpointID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *run.RuntimeCheckpoint) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
	return nil
}

type DecisionTicketRepository struct {
	mu    sync.RWMutex
	items map[string]*decision.Ticket
}

func NewDecisionTicketRepository() *DecisionTicketRepository {
	return &DecisionTicketRepository{items: map[string]*decision.Ticket{}}
}

func (r *DecisionTicketRepository) Create(_ context.Context, item *decision.Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if strings.TrimSpace(item.CreatedAt) == "" {
		item.CreatedAt = now
	}
	if strings.TrimSpace(item.UpdatedAt) == "" {
		item.UpdatedAt = item.CreatedAt
	}
	r.items[key] = cloneDecisionTicket(item)
	return nil
}

func (r *DecisionTicketRepository) Get(ctx context.Context, id common.TicketID) (*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if key, scoped := runtimeWorkspaceScopedKey(ctx, string(id)); scoped {
		item, ok := r.items[key]
		if !ok {
			return nil, common.ErrNotFound
		}
		return cloneDecisionTicket(item), nil
	}
	var ret *decision.Ticket
	for _, item := range r.items {
		if item.ID != id {
			continue
		}
		if ret != nil {
			return nil, common.ErrConflict
		}
		ret = item
	}
	if ret == nil {
		return nil, common.ErrNotFound
	}
	return cloneDecisionTicket(ret), nil
}

func (r *DecisionTicketRepository) List(_ context.Context) ([]*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for key := range r.items {
		ids = append(ids, key)
	}
	sort.Strings(ids)

	ret := make([]*decision.Ticket, 0, len(ids))
	for _, key := range ids {
		ret = append(ret, cloneDecisionTicket(r.items[key]))
	}
	return ret, nil
}

func (r *DecisionTicketRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	matchedWorkspaces := map[common.WorkspaceID]struct{}{}
	for key, item := range r.items {
		if item.RunID == runID {
			if workspaceID != "" && item.WorkspaceID != workspaceID {
				continue
			}
			matchedWorkspaces[item.WorkspaceID] = struct{}{}
			ids = append(ids, key)
		}
	}
	if workspaceID == "" && len(matchedWorkspaces) > 1 {
		return nil, common.ErrConflict
	}
	sort.Strings(ids)

	ret := make([]*decision.Ticket, 0, len(ids))
	for _, key := range ids {
		ret = append(ret, cloneDecisionTicket(r.items[key]))
	}
	return ret, nil
}

func (r *DecisionTicketRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for key, item := range r.items {
		if item.WorkspaceID == workspaceID {
			ids = append(ids, key)
		}
	}
	sort.Strings(ids)

	ret := make([]*decision.Ticket, 0, len(ids))
	for _, key := range ids {
		ret = append(ret, cloneDecisionTicket(r.items[key]))
	}
	return ret, nil
}

func (r *DecisionTicketRepository) Update(ctx context.Context, item *decision.Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		if scopedKey, scoped := runtimeWorkspaceScopedKey(ctx, string(item.ID)); scoped {
			key = scopedKey
		} else {
			var found string
			for currentKey, current := range r.items {
				if current.ID != item.ID {
					continue
				}
				if found != "" {
					return common.ErrConflict
				}
				found = currentKey
			}
			if found == "" {
				return common.ErrNotFound
			}
			key = found
			item.WorkspaceID = r.items[key].WorkspaceID
		}
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	if strings.TrimSpace(item.CreatedAt) == "" {
		item.CreatedAt = r.items[key].CreatedAt
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.items[key] = cloneDecisionTicket(item)
	return nil
}

func (r *DecisionTicketRepository) Delete(ctx context.Context, id common.TicketID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if key, scoped := runtimeWorkspaceScopedKey(ctx, string(id)); scoped {
		if _, ok := r.items[key]; !ok {
			return common.ErrNotFound
		}
		delete(r.items, key)
		return nil
	}
	var found string
	for key, item := range r.items {
		if item.ID != id {
			continue
		}
		if found != "" {
			return common.ErrConflict
		}
		found = key
	}
	if found == "" {
		return common.ErrNotFound
	}
	delete(r.items, found)
	return nil
}

type DecisionResolutionRepository struct {
	mu       sync.RWMutex
	byTicket map[string]*decision.Resolution
}

func NewDecisionResolutionRepository() *DecisionResolutionRepository {
	return &DecisionResolutionRepository{byTicket: map[string]*decision.Resolution{}}
}

func (r *DecisionResolutionRepository) Create(ctx context.Context, item *decision.Resolution) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	key := workspaceScopedKey(workspaceID, string(item.TicketID))
	if _, ok := r.byTicket[key]; ok {
		return common.ErrConflict
	}
	cloned := *item
	r.byTicket[key] = &cloned
	return nil
}

func (r *DecisionResolutionRepository) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decision.Resolution, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.byTicket, string(ticketID), func(item *decision.Resolution) string {
		return string(item.TicketID)
	})
	if err != nil {
		return nil, err
	}
	cloned := *r.byTicket[key]
	return &cloned, nil
}

func (r *DecisionResolutionRepository) DeleteByTicketID(ctx context.Context, ticketID common.TicketID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.byTicket, string(ticketID), func(item *decision.Resolution) string {
		return string(item.TicketID)
	})
	if err != nil {
		return err
	}
	delete(r.byTicket, key)
	return nil
}

type ApprovalRepository struct {
	mu    sync.RWMutex
	items map[string]*approval.Approval
}

func NewApprovalRepository() *ApprovalRepository {
	return &ApprovalRepository{items: map[string]*approval.Approval{}}
}

func (r *ApprovalRepository) Create(_ context.Context, item *approval.Approval) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if strings.TrimSpace(item.CreatedAt) == "" {
		item.CreatedAt = now
	}
	if strings.TrimSpace(item.UpdatedAt) == "" {
		item.UpdatedAt = item.CreatedAt
	}
	cloned := cloneApproval(item)
	r.items[key] = &cloned
	return nil
}

func (r *ApprovalRepository) Get(ctx context.Context, id common.ApprovalID) (*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if key, scoped := runtimeWorkspaceScopedKey(ctx, string(id)); scoped {
		item, ok := r.items[key]
		if !ok {
			return nil, common.ErrNotFound
		}
		cloned := cloneApproval(item)
		return &cloned, nil
	}
	var ret *approval.Approval
	for _, item := range r.items {
		if item.ID != id {
			continue
		}
		if ret != nil {
			return nil, common.ErrConflict
		}
		ret = item
	}
	if ret == nil {
		return nil, common.ErrNotFound
	}
	cloned := cloneApproval(ret)
	return &cloned, nil
}

func (r *ApprovalRepository) List(_ context.Context) ([]*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for key := range r.items {
		ids = append(ids, key)
	}
	sort.Strings(ids)

	ret := make([]*approval.Approval, 0, len(ids))
	for _, key := range ids {
		cloned := cloneApproval(r.items[key])
		ret = append(ret, &cloned)
	}
	return ret, nil
}

func (r *ApprovalRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	matchedWorkspaces := map[common.WorkspaceID]struct{}{}
	for key, item := range r.items {
		if item.RunID == runID {
			if workspaceID != "" && item.WorkspaceID != workspaceID {
				continue
			}
			matchedWorkspaces[item.WorkspaceID] = struct{}{}
			ids = append(ids, key)
		}
	}
	if workspaceID == "" && len(matchedWorkspaces) > 1 {
		return nil, common.ErrConflict
	}
	sort.Strings(ids)

	ret := make([]*approval.Approval, 0, len(ids))
	for _, key := range ids {
		cloned := cloneApproval(r.items[key])
		ret = append(ret, &cloned)
	}
	return ret, nil
}

func (r *ApprovalRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for key, item := range r.items {
		if item.WorkspaceID == workspaceID {
			ids = append(ids, key)
		}
	}
	sort.Strings(ids)

	ret := make([]*approval.Approval, 0, len(ids))
	for _, key := range ids {
		cloned := cloneApproval(r.items[key])
		ret = append(ret, &cloned)
	}
	return ret, nil
}

func (r *ApprovalRepository) Update(ctx context.Context, item *approval.Approval) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		if scopedKey, scoped := runtimeWorkspaceScopedKey(ctx, string(item.ID)); scoped {
			key = scopedKey
		} else {
			var found string
			for currentKey, current := range r.items {
				if current.ID != item.ID {
					continue
				}
				if found != "" {
					return common.ErrConflict
				}
				found = currentKey
			}
			if found == "" {
				return common.ErrNotFound
			}
			key = found
			item.WorkspaceID = r.items[key].WorkspaceID
		}
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	if strings.TrimSpace(item.CreatedAt) == "" {
		item.CreatedAt = r.items[key].CreatedAt
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	cloned := cloneApproval(item)
	r.items[key] = &cloned
	return nil
}

func (r *ApprovalRepository) Delete(ctx context.Context, id common.ApprovalID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if key, scoped := runtimeWorkspaceScopedKey(ctx, string(id)); scoped {
		if _, ok := r.items[key]; !ok {
			return common.ErrNotFound
		}
		delete(r.items, key)
		return nil
	}
	var found string
	for key, item := range r.items {
		if item.ID != id {
			continue
		}
		if found != "" {
			return common.ErrConflict
		}
		found = key
	}
	if found == "" {
		return common.ErrNotFound
	}
	delete(r.items, found)
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

func cloneValidationResult(item *run.ValidationResult) *run.ValidationResult {
	cloned := *item
	cloned.EvidenceTags = cloneStringSlice(item.EvidenceTags)
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
	cloned.SummaryDetail = cloneResultSummary(item.SummaryDetail)
	return &cloned
}

func cloneResultSummary(item resultmodel.ResultSummary) resultmodel.ResultSummary {
	cloned := item
	cloned.EvidenceRefs = cloneStringSlice(item.EvidenceRefs)
	if item.ArtifactRefs != nil {
		cloned.ArtifactRefs = append([]common.ArtifactID(nil), item.ArtifactRefs...)
	}
	cloned.KnownLimitations = cloneStringSlice(item.KnownLimitations)
	cloned.ResidualRisks = cloneStringSlice(item.ResidualRisks)
	cloned.NextActions = cloneStringSlice(item.NextActions)
	if item.DoDItems != nil {
		cloned.DoDItems = make([]resultmodel.DoDAssessmentItem, 0, len(item.DoDItems))
		for _, dodItem := range item.DoDItems {
			dodItem.EvidenceRefs = cloneStringSlice(dodItem.EvidenceRefs)
			cloned.DoDItems = append(cloned.DoDItems, dodItem)
		}
	}
	cloned.Diagnostics = cloneMap(item.Diagnostics)
	return cloned
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
	cloned.GoalCoverage = cloneResultDoDCoverage(item.GoalCoverage)
	cloned.AcceptanceDimensions = cloneResultDoDCoverage(item.AcceptanceDimensions)
	cloned.DeliverableCoverage = cloneResultDoDCoverage(item.DeliverableCoverage)
	return cloned
}

func cloneResultDoDCoverage(item *resultmodel.DoDCoverage) *resultmodel.DoDCoverage {
	if item == nil {
		return nil
	}
	cloned := *item
	if item.Items != nil {
		cloned.Items = make([]resultmodel.DoDCoverageItem, 0, len(item.Items))
		for _, coverageItem := range item.Items {
			coverageItem.EvidenceRefs = cloneStringSlice(coverageItem.EvidenceRefs)
			if coverageItem.ArtifactRefs != nil {
				coverageItem.ArtifactRefs = append([]common.ArtifactID(nil), coverageItem.ArtifactRefs...)
			}
			cloned.Items = append(cloned.Items, coverageItem)
		}
	}
	return &cloned
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
	cloned.PayloadSummary = cloneMap(item.PayloadSummary)
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
	cloned.EvidencePolicies = cloneMapSlice(item.EvidencePolicies)
	return &cloned
}

func cloneMemoryItem(item *memorymodel.MemoryItem) *memorymodel.MemoryItem {
	cloned := *item
	cloned.Tags = cloneStringSlice(item.Tags)
	cloned.Vector = cloneFloatSlice(item.Vector)
	return &cloned
}

func cloneMemoryRecallTrace(item *memorymodel.MemoryRecallTrace) *memorymodel.MemoryRecallTrace {
	cloned := *item
	if item.CandidateMemoryIDs != nil {
		cloned.CandidateMemoryIDs = append([]common.MemoryID(nil), item.CandidateMemoryIDs...)
	}
	if item.SelectedMemoryIDs != nil {
		cloned.SelectedMemoryIDs = append([]common.MemoryID(nil), item.SelectedMemoryIDs...)
	}
	if item.ExcludedMemoryIDs != nil {
		cloned.ExcludedMemoryIDs = append([]common.MemoryID(nil), item.ExcludedMemoryIDs...)
	}
	if item.TypeQuota != nil {
		cloned.TypeQuota = map[string]int{}
		for key, value := range item.TypeQuota {
			cloned.TypeQuota[key] = value
		}
	}
	if item.SelectionScores != nil {
		cloned.SelectionScores = append([]memorymodel.MemoryRecallSelectionScore(nil), item.SelectionScores...)
	}
	if item.RetrievedMemoryIDs != nil {
		cloned.RetrievedMemoryIDs = append([]common.MemoryID(nil), item.RetrievedMemoryIDs...)
	}
	if item.RetrievedMemoryHits != nil {
		cloned.RetrievedMemoryHits = append([]memorymodel.MemoryRecallHit(nil), item.RetrievedMemoryHits...)
	}
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
	cloned.Args = cloneStringSlice(item.Args)
	cloned.Env = cloneStringMap(item.Env)
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

func listArtifacts(items map[string]*resultmodel.Artifact, matches func(*resultmodel.Artifact) bool) []*resultmodel.Artifact {
	ret := make([]*resultmodel.Artifact, 0)
	for _, item := range items {
		if !matches(item) {
			continue
		}
		ret = append(ret, cloneArtifact(item))
	}
	return sortedArtifactList(ret)
}

func listArtifactSlice(items []*resultmodel.Artifact) []*resultmodel.Artifact {
	ret := make([]*resultmodel.Artifact, 0, len(items))
	for _, item := range items {
		ret = append(ret, cloneArtifact(item))
	}
	return sortedArtifactList(ret)
}

func sortedArtifactList(ret []*resultmodel.Artifact) []*resultmodel.Artifact {
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func listEventRecords(items map[string]*eventstream.EventRecord, query repository.EventRecordQuery) []*eventstream.EventRecord {
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

func memoryRecallTraceMatches(item *memorymodel.MemoryRecallTrace, query repository.MemoryRecallTraceQuery) bool {
	if item == nil {
		return false
	}
	if query.WorkspaceID != "" && item.WorkspaceID != query.WorkspaceID {
		return false
	}
	if query.RunID != "" && item.RunID != query.RunID {
		return false
	}
	if query.StepID != "" && item.StepID != query.StepID {
		return false
	}
	if query.Stage != "" && item.Stage != query.Stage {
		return false
	}
	if query.Source != "" && item.Source != query.Source {
		return false
	}
	if query.Status != "" && item.Status != query.Status {
		return false
	}
	return true
}

func sortMemoryRecallTraces(items []*memorymodel.MemoryRecallTrace) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].OccurredAt != items[j].OccurredAt {
			return items[i].OccurredAt < items[j].OccurredAt
		}
		return string(items[i].ID) < string(items[j].ID)
	})
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

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func clonePatch(item *run.Patch) run.Patch {
	cloned := *item
	cloned.CoverageTargets = cloneAny(item.CoverageTargets)
	return cloned
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
		out[k] = cloneAny(v)
	}
	return out
}

func cloneMapSlice(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	out := make([]map[string]any, 0, len(input))
	for _, item := range input {
		out = append(out, cloneMap(item))
	}
	return out
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []map[string]any:
		return cloneMapSlice(typed)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, cloneAny(item))
		}
		return out
	case []string:
		return cloneStringSlice(typed)
	default:
		return value
	}
}
