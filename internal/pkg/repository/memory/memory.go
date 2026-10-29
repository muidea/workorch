package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	approval "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decision "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	eventstream "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	soul "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/preparationcompiler"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventrecordcontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

type WorkspaceRepository struct {
	mu           sync.RWMutex
	items        map[common.WorkspaceID]*workspace.Workspace
	ownerBinding repository.OwnerRepositoryBinding
}

type WorkspaceTemplateRepository struct {
	mu           sync.RWMutex
	items        map[string]*workspace.WorkspaceTemplate
	ownerBinding repository.OwnerRepositoryBinding
}

type WorkspaceOwnerRepositorySet struct {
	Workspaces *WorkspaceRepository
	Goals      *GoalRepository
	DoDs       *DoDRepository
	Templates  *WorkspaceTemplateRepository
	binding    repository.OwnerRepositoryBinding
}

func NewWorkspaceOwnerRepositorySet() WorkspaceOwnerRepositorySet {
	binding := repository.NewWorkspaceOwnerRepositoryBinding()
	return WorkspaceOwnerRepositorySet{
		Workspaces: newWorkspaceRepository(binding),
		Goals:      newGoalRepository(binding),
		DoDs:       newDoDRepository(binding),
		Templates:  newWorkspaceTemplateRepository(binding),
		binding:    binding,
	}
}

func (s WorkspaceOwnerRepositorySet) WorkspaceOwnerBinding() repository.OwnerRepositoryBinding {
	return s.binding
}
func (s WorkspaceOwnerRepositorySet) WorkspaceOwnerWorkspaces() repository.WorkspaceRepository {
	return s.Workspaces
}
func (s WorkspaceOwnerRepositorySet) WorkspaceOwnerDeletionMaterialCleaner() repository.WorkspaceDeletionMaterialCleaner {
	return s.Workspaces
}
func (s WorkspaceOwnerRepositorySet) WorkspaceOwnerGoals() repository.GoalRepository { return s.Goals }
func (s WorkspaceOwnerRepositorySet) WorkspaceOwnerDoDs() repository.DoDRepository   { return s.DoDs }
func (s WorkspaceOwnerRepositorySet) WorkspaceOwnerTemplates() repository.WorkspaceTemplateRepository {
	return s.Templates
}
func (s WorkspaceOwnerRepositorySet) OwnerRepositories() repository.WorkspaceOwnerRepositories {
	owner, err := repository.NewWorkspaceOwnerRepositories(s)
	if err != nil {
		panic(err)
	}
	return owner
}

func (r *WorkspaceRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *WorkspaceTemplateRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *GoalRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *DoDRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

type SoulRepository struct {
	mu    sync.RWMutex
	items map[common.SoulID]*soul.Soul
}

type MemoryItemRepository struct {
	mu                 sync.RWMutex
	items              map[string]*memorymodel.MemoryItem
	workspaceDeletions *MemoryWorkspaceDeletionRepository
	ownerBinding       repository.OwnerRepositoryBinding
}

type MemoryRecallTraceRepository struct {
	mu                 sync.RWMutex
	items              map[string]*memorymodel.MemoryRecallTrace
	workspaceDeletions *MemoryWorkspaceDeletionRepository
	ownerBinding       repository.OwnerRepositoryBinding
}

type MemoryWorkspaceDeletionRepository struct {
	mu           sync.RWMutex
	items        map[common.WorkspaceID]*memorymodel.WorkspaceDeletion
	ownerBinding repository.OwnerRepositoryBinding
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
	mu                 sync.RWMutex
	items              map[string]*resultmodel.Result
	workspaceDeletions *ResultWorkspaceDeletionRepository
	ownerBinding       repository.OwnerRepositoryBinding
}

type ArtifactRepository struct {
	mu                 sync.RWMutex
	items              map[string]*resultmodel.Artifact
	workspaceDeletions *ResultWorkspaceDeletionRepository
	ownerBinding       repository.OwnerRepositoryBinding
}

type ResultWorkspaceDeletionRepository struct {
	mu           sync.RWMutex
	items        map[common.WorkspaceID]*resultmodel.WorkspaceDeletion
	ownerBinding repository.OwnerRepositoryBinding
}

type MemoryOwnerRepositorySet struct {
	Items              *MemoryItemRepository
	RecallTraces       *MemoryRecallTraceRepository
	WorkspaceDeletions *MemoryWorkspaceDeletionRepository
	binding            repository.OwnerRepositoryBinding
}

func NewMemoryOwnerRepositorySet() MemoryOwnerRepositorySet {
	binding := repository.NewMemoryOwnerRepositoryBinding()
	deletions := &MemoryWorkspaceDeletionRepository{items: map[common.WorkspaceID]*memorymodel.WorkspaceDeletion{}, ownerBinding: binding}
	return MemoryOwnerRepositorySet{
		Items:              &MemoryItemRepository{items: map[string]*memorymodel.MemoryItem{}, workspaceDeletions: deletions, ownerBinding: binding},
		RecallTraces:       &MemoryRecallTraceRepository{items: map[string]*memorymodel.MemoryRecallTrace{}, workspaceDeletions: deletions, ownerBinding: binding},
		WorkspaceDeletions: deletions,
		binding:            binding,
	}
}

func (s MemoryOwnerRepositorySet) MemoryOwnerBinding() repository.OwnerRepositoryBinding {
	return s.binding
}
func (s MemoryOwnerRepositorySet) MemoryOwnerItems() repository.MemoryItemRepository { return s.Items }
func (s MemoryOwnerRepositorySet) MemoryOwnerRecallTraces() repository.MemoryRecallTraceRepository {
	return s.RecallTraces
}
func (s MemoryOwnerRepositorySet) MemoryOwnerWorkspaceDeletions() repository.MemoryWorkspaceDeletionRepository {
	return s.WorkspaceDeletions
}
func (s MemoryOwnerRepositorySet) OwnerRepositories() repository.MemoryOwnerRepositories {
	owner, err := repository.NewMemoryOwnerRepositories(s)
	if err != nil {
		panic(err)
	}
	return owner
}

type ResultOwnerRepositorySet struct {
	Results            *ResultRepository
	Artifacts          *ArtifactRepository
	WorkspaceDeletions *ResultWorkspaceDeletionRepository
	binding            repository.OwnerRepositoryBinding
}

func NewResultOwnerRepositorySet() ResultOwnerRepositorySet {
	binding := repository.NewResultOwnerRepositoryBinding()
	deletions := &ResultWorkspaceDeletionRepository{items: map[common.WorkspaceID]*resultmodel.WorkspaceDeletion{}, ownerBinding: binding}
	return ResultOwnerRepositorySet{
		Results:            &ResultRepository{items: map[string]*resultmodel.Result{}, workspaceDeletions: deletions, ownerBinding: binding},
		Artifacts:          &ArtifactRepository{items: map[string]*resultmodel.Artifact{}, workspaceDeletions: deletions, ownerBinding: binding},
		WorkspaceDeletions: deletions,
		binding:            binding,
	}
}

func (r *MemoryItemRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *MemoryRecallTraceRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *MemoryWorkspaceDeletionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *ResultRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *ArtifactRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *ResultWorkspaceDeletionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (s ResultOwnerRepositorySet) ResultOwnerBinding() repository.OwnerRepositoryBinding {
	return s.binding
}
func (s ResultOwnerRepositorySet) ResultOwnerResults() repository.ResultRepository { return s.Results }
func (s ResultOwnerRepositorySet) ResultOwnerArtifacts() repository.ArtifactRepository {
	return s.Artifacts
}
func (s ResultOwnerRepositorySet) ResultOwnerWorkspaceDeletions() repository.ResultWorkspaceDeletionRepository {
	return s.WorkspaceDeletions
}
func (s ResultOwnerRepositorySet) OwnerRepositories() repository.ResultOwnerRepositories {
	owner, err := repository.NewResultOwnerRepositories(s)
	if err != nil {
		panic(err)
	}
	return owner
}

type workspaceDeletionGuardContextKey struct{}

type workspaceDeletionGuardContext struct {
	owner       string
	workspaceID common.WorkspaceID
}

func deletionContextGuards(ctx context.Context, owner string, workspaceID common.WorkspaceID) bool {
	marker, ok := ctx.Value(workspaceDeletionGuardContextKey{}).(workspaceDeletionGuardContext)
	return ok && marker.owner == owner && marker.workspaceID == workspaceID
}

func (r *MemoryWorkspaceDeletionRepository) guardMutation(ctx context.Context, workspaceID common.WorkspaceID) (func(), error) {
	if deletionContextGuards(ctx, "memory", workspaceID) {
		return func() {}, nil
	}
	if r == nil || workspaceID == "" {
		return func() {}, common.ErrInvalidArgument
	}
	r.mu.RLock()
	item := r.items[workspaceID]
	if item != nil {
		r.mu.RUnlock()
		return func() {}, common.ErrNotFound
	}
	return r.mu.RUnlock, nil
}

func (r *ResultWorkspaceDeletionRepository) guardMutation(ctx context.Context, workspaceID common.WorkspaceID) (func(), error) {
	if deletionContextGuards(ctx, "result", workspaceID) {
		return func() {}, nil
	}
	if r == nil || workspaceID == "" {
		return func() {}, common.ErrInvalidArgument
	}
	r.mu.RLock()
	item := r.items[workspaceID]
	if item != nil {
		r.mu.RUnlock()
		return func() {}, common.ErrNotFound
	}
	return r.mu.RUnlock, nil
}

func NewMemoryWorkspaceDeletionRepository() *MemoryWorkspaceDeletionRepository {
	return &MemoryWorkspaceDeletionRepository{items: map[common.WorkspaceID]*memorymodel.WorkspaceDeletion{}}
}

func (r *MemoryWorkspaceDeletionRepository) Create(_ context.Context, item *memorymodel.WorkspaceDeletion) error {
	if r == nil || item == nil || item.Validate() != nil || item.Revision != 1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[item.WorkspaceID]; exists {
		return common.ErrConflict
	}
	r.items[item.WorkspaceID] = memorymodel.CloneWorkspaceDeletion(item)
	return nil
}

func (r *MemoryWorkspaceDeletionRepository) Get(_ context.Context, workspaceID common.WorkspaceID) (*memorymodel.WorkspaceDeletion, error) {
	if r == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, exists := r.items[workspaceID]
	if !exists {
		return nil, common.ErrNotFound
	}
	return memorymodel.CloneWorkspaceDeletion(item), nil
}

func (r *MemoryWorkspaceDeletionRepository) CompareAndSwap(_ context.Context, item *memorymodel.WorkspaceDeletion, expectedRevision uint64) error {
	if r == nil || item == nil || item.Validate() != nil || expectedRevision == 0 || item.Revision != expectedRevision+1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.items[item.WorkspaceID]
	if !exists {
		return common.ErrNotFound
	}
	if current.Revision != expectedRevision {
		return common.ErrConflict
	}
	r.items[item.WorkspaceID] = memorymodel.CloneWorkspaceDeletion(item)
	return nil
}

func (r *MemoryWorkspaceDeletionRepository) WithWorkspaceActiveFence(ctx context.Context, workspaceID common.WorkspaceID, operation func(context.Context) error) error {
	if r == nil || workspaceID == "" || operation == nil {
		return common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.items[workspaceID] != nil {
		return common.ErrNotFound
	}
	return operation(context.WithValue(ctx, workspaceDeletionGuardContextKey{}, workspaceDeletionGuardContext{owner: "memory", workspaceID: workspaceID}))
}

func (r *MemoryWorkspaceDeletionRepository) WithWorkspacePurgeFence(ctx context.Context, scope deletioncontract.Scope, operation func(context.Context) error) error {
	scope = scope.Normalize()
	if r == nil || scope.Validate() != nil || scope.Mode != deletioncontract.ModePurge || operation == nil {
		return common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.items[scope.WorkspaceID]
	if item == nil {
		return common.ErrNotFound
	}
	if item.Scope() != scope {
		return common.ErrConflict
	}
	return operation(context.WithValue(ctx, workspaceDeletionGuardContextKey{}, workspaceDeletionGuardContext{owner: "memory", workspaceID: scope.WorkspaceID}))
}

func NewResultWorkspaceDeletionRepository() *ResultWorkspaceDeletionRepository {
	return &ResultWorkspaceDeletionRepository{items: map[common.WorkspaceID]*resultmodel.WorkspaceDeletion{}}
}

func (r *ResultWorkspaceDeletionRepository) Create(_ context.Context, item *resultmodel.WorkspaceDeletion) error {
	if r == nil || item == nil || item.Validate() != nil || item.Revision != 1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[item.WorkspaceID]; exists {
		return common.ErrConflict
	}
	r.items[item.WorkspaceID] = resultmodel.CloneWorkspaceDeletion(item)
	return nil
}

func (r *ResultWorkspaceDeletionRepository) Get(_ context.Context, workspaceID common.WorkspaceID) (*resultmodel.WorkspaceDeletion, error) {
	if r == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, exists := r.items[workspaceID]
	if !exists {
		return nil, common.ErrNotFound
	}
	return resultmodel.CloneWorkspaceDeletion(item), nil
}

func (r *ResultWorkspaceDeletionRepository) CompareAndSwap(_ context.Context, item *resultmodel.WorkspaceDeletion, expectedRevision uint64) error {
	if r == nil || item == nil || item.Validate() != nil || expectedRevision == 0 || item.Revision != expectedRevision+1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.items[item.WorkspaceID]
	if !exists {
		return common.ErrNotFound
	}
	if current.Revision != expectedRevision {
		return common.ErrConflict
	}
	r.items[item.WorkspaceID] = resultmodel.CloneWorkspaceDeletion(item)
	return nil
}

func (r *ResultWorkspaceDeletionRepository) WithWorkspaceActiveFence(ctx context.Context, workspaceID common.WorkspaceID, operation func(context.Context) error) error {
	if r == nil || workspaceID == "" || operation == nil {
		return common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.items[workspaceID] != nil {
		return common.ErrNotFound
	}
	return operation(context.WithValue(ctx, workspaceDeletionGuardContextKey{}, workspaceDeletionGuardContext{owner: "result", workspaceID: workspaceID}))
}

func (r *ResultWorkspaceDeletionRepository) WithWorkspacePurgeFence(ctx context.Context, scope deletioncontract.Scope, operation func(context.Context) error) error {
	scope = scope.Normalize()
	if r == nil || scope.Validate() != nil || scope.Mode != deletioncontract.ModePurge || operation == nil {
		return common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.items[scope.WorkspaceID]
	if item == nil {
		return common.ErrNotFound
	}
	if item.Scope() != scope {
		return common.ErrConflict
	}
	return operation(context.WithValue(ctx, workspaceDeletionGuardContextKey{}, workspaceDeletionGuardContext{owner: "result", workspaceID: scope.WorkspaceID}))
}

type EventRecordRepository struct {
	mu                 sync.RWMutex
	items              map[string]*eventstream.EventRecord
	streamPosition     int64
	workspaceDeletions *EventStreamWorkspaceDeletionRepository
	ownerBinding       repository.OwnerRepositoryBinding
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

func validateRuntimeWorkspaceScope(ctx context.Context, workspaceID common.WorkspaceID) error {
	scopedWorkspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if scopedWorkspaceID != "" && workspaceID != "" && scopedWorkspaceID != workspaceID {
		return common.ErrConflict
	}
	return nil
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
	return newWorkspaceTemplateRepository(repository.NewWorkspaceOwnerRepositoryBinding())
}

func newWorkspaceTemplateRepository(binding repository.OwnerRepositoryBinding) *WorkspaceTemplateRepository {
	return &WorkspaceTemplateRepository{items: map[string]*workspace.WorkspaceTemplate{}, ownerBinding: binding}
}

func (r *WorkspaceTemplateRepository) Create(_ context.Context, item *workspace.WorkspaceTemplate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneWorkspaceTemplate(item)
	return nil
}

func (r *WorkspaceTemplateRepository) Get(_ context.Context, id string) (*workspace.WorkspaceTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneWorkspaceTemplate(item), nil
}

func (r *WorkspaceTemplateRepository) List(_ context.Context) ([]*workspace.WorkspaceTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	ret := make([]*workspace.WorkspaceTemplate, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, cloneWorkspaceTemplate(r.items[id]))
	}
	return ret, nil
}

func (r *WorkspaceTemplateRepository) Update(_ context.Context, item *workspace.WorkspaceTemplate) error {
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

func (r *MemoryItemRepository) Create(ctx context.Context, item *memorymodel.MemoryItem) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, item.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
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

func (r *MemoryItemRepository) Search(_ context.Context, query workorchquerycontract.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
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
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID == "" {
		r.mu.RLock()
		key, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *memorymodel.MemoryItem) string { return string(current.ID) })
		if err == nil {
			item.WorkspaceID = r.items[key].WorkspaceID
		}
		r.mu.RUnlock()
		if err != nil {
			return err
		}
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, item.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
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
	r.mu.RLock()
	resolved, resolveErr := findScopedMemoryKey(ctx, r.items, string(id), func(item *memorymodel.MemoryItem) string { return string(item.ID) })
	var workspaceID common.WorkspaceID
	if resolveErr == nil {
		workspaceID = r.items[resolved].WorkspaceID
	}
	r.mu.RUnlock()
	if resolveErr != nil {
		return resolveErr
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, workspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
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

func (r *MemoryRecallTraceRepository) Create(ctx context.Context, item *memorymodel.MemoryRecallTrace) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, item.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
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

func (r *MemoryRecallTraceRepository) Upsert(ctx context.Context, item *memorymodel.MemoryRecallTrace) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, item.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
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

func (r *MemoryRecallTraceRepository) List(_ context.Context, query workorchquerycontract.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
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
	var pageErr error
	ret, pageErr = pageMemoryRecallTraces(ret, query.CursorAfterID)
	if pageErr != nil {
		return nil, pageErr
	}
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
	r.mu.RLock()
	resolved, resolveErr := findScopedMemoryKey(ctx, r.items, string(id), func(item *memorymodel.MemoryRecallTrace) string { return string(item.ID) })
	var workspaceID common.WorkspaceID
	if resolveErr == nil {
		workspaceID = r.items[resolved].WorkspaceID
	}
	r.mu.RUnlock()
	if resolveErr != nil {
		return resolveErr
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, workspaceID)
		if err != nil {
			return err
		}
		defer unlock()
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
		item := r.items[common.SkillID(id)]
		if item == nil || item.ID != common.SkillID(id) {
			return nil, common.ErrConflict
		}
		ret = append(ret, cloneSkill(item))
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

func (r *ResultRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, item.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	if item.Revision == 0 {
		item.Revision = 1
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

func (r *ResultRepository) ListForRecovery(_ context.Context, cursor string, limit int) ([]*resultmodel.Result, string, error) {
	if limit < 1 {
		return nil, "", common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]*resultmodel.Result, 0, len(r.items))
	for _, item := range r.items {
		if item == nil || resultRecoveryKey(item) <= cursor {
			continue
		}
		if r.workspaceDeletions != nil {
			r.workspaceDeletions.mu.RLock()
			fenced := r.workspaceDeletions.items[item.WorkspaceID] != nil
			r.workspaceDeletions.mu.RUnlock()
			if fenced {
				continue
			}
		}
		items = append(items, cloneResult(item))
	}
	sort.Slice(items, func(i, j int) bool { return resultRecoveryKey(items[i]) < resultRecoveryKey(items[j]) })
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = resultRecoveryKey(items[len(items)-1])
	}
	if len(items) == 0 {
		return nil, "", nil
	}
	return items, next, nil
}

func resultRecoveryKey(item *resultmodel.Result) string {
	if item == nil {
		return ""
	}
	return string(item.WorkspaceID) + "\x00" + string(item.ID)
}

func (r *ResultRepository) Update(ctx context.Context, item *resultmodel.Result) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID == "" {
		r.mu.RLock()
		key, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *resultmodel.Result) string { return string(current.ID) })
		if err == nil {
			item.WorkspaceID = r.items[key].WorkspaceID
		}
		r.mu.RUnlock()
		if err != nil {
			return err
		}
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, item.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
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

func (r *ResultRepository) CompareAndSwap(ctx context.Context, item *resultmodel.Result, expectedRevision uint64) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID == "" {
		r.mu.RLock()
		key, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *resultmodel.Result) string { return string(current.ID) })
		if err == nil {
			item.WorkspaceID = r.items[key].WorkspaceID
		}
		r.mu.RUnlock()
		if err != nil {
			return err
		}
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, item.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *resultmodel.Result) string { return string(current.ID) })
		if err != nil {
			return err
		}
		key = found
		item.WorkspaceID = r.items[key].WorkspaceID
	}
	current, ok := r.items[key]
	if !ok {
		return common.ErrNotFound
	}
	if current.Revision != expectedRevision {
		return common.ErrConflict
	}
	item.Revision = expectedRevision + 1
	r.items[key] = cloneResult(item)
	return nil
}

func (r *ResultRepository) Delete(ctx context.Context, id common.ResultID) error {
	r.mu.RLock()
	resolved, resolveErr := findScopedMemoryKey(ctx, r.items, string(id), func(item *resultmodel.Result) string { return string(item.ID) })
	var workspaceID common.WorkspaceID
	if resolveErr == nil {
		workspaceID = r.items[resolved].WorkspaceID
	}
	r.mu.RUnlock()
	if resolveErr != nil {
		return resolveErr
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, workspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
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

func (r *ArtifactRepository) Create(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, item.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
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
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID == "" {
		r.mu.RLock()
		key, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *resultmodel.Artifact) string { return string(current.ID) })
		if err == nil {
			item.WorkspaceID = r.items[key].WorkspaceID
		}
		r.mu.RUnlock()
		if err != nil {
			return err
		}
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, item.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
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
	r.mu.RLock()
	resolved, resolveErr := findScopedMemoryKey(ctx, r.items, string(id), func(item *resultmodel.Artifact) string { return string(item.ID) })
	var workspaceID common.WorkspaceID
	if resolveErr == nil {
		workspaceID = r.items[resolved].WorkspaceID
	}
	r.mu.RUnlock()
	if resolveErr != nil {
		return resolveErr
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, workspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
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

type EventStreamOwnerRepositorySet struct {
	EventRecords       *EventRecordRepository
	WorkspaceDeletions *EventStreamWorkspaceDeletionRepository
	binding            repository.OwnerRepositoryBinding
}

func NewEventStreamOwnerRepositorySet() EventStreamOwnerRepositorySet {
	binding := repository.NewEventStreamOwnerRepositoryBinding()
	deletions := &EventStreamWorkspaceDeletionRepository{items: map[common.WorkspaceID]*eventstream.WorkspaceDeletion{}, ownerBinding: binding}
	return EventStreamOwnerRepositorySet{
		EventRecords:       &EventRecordRepository{items: map[string]*eventstream.EventRecord{}, workspaceDeletions: deletions, ownerBinding: binding},
		WorkspaceDeletions: deletions,
		binding:            binding,
	}
}

func (s EventStreamOwnerRepositorySet) EventStreamOwnerBinding() repository.OwnerRepositoryBinding {
	return s.binding
}
func (s EventStreamOwnerRepositorySet) EventStreamOwnerEventRecords() repository.EventRecordRepository {
	return s.EventRecords
}
func (s EventStreamOwnerRepositorySet) EventStreamOwnerWorkspaceDeletions() repository.EventStreamWorkspaceDeletionRepository {
	return s.WorkspaceDeletions
}
func (s EventStreamOwnerRepositorySet) EventStreamOwnerWorkspacePurger() repository.EventStreamWorkspacePurger {
	return s.EventRecords
}
func (s EventStreamOwnerRepositorySet) OwnerRepositories() repository.EventStreamOwnerRepositories {
	owner, err := repository.NewEventStreamOwnerRepositories(s)
	if err != nil {
		panic(err)
	}
	return owner
}

func (r *EventRecordRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func NewEventRecordRepository() *EventRecordRepository {
	return NewEventStreamOwnerRepositorySet().EventRecords
}

func (r *EventRecordRepository) Append(ctx context.Context, item *eventstream.EventRecord) error {
	if r == nil || item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	canonicalOccurredAt, err := workorcheventrecordcontract.CanonicalOccurredAt(item.OccurredAt)
	if err != nil || canonicalOccurredAt != item.OccurredAt {
		return fmt.Errorf("%w: event record occurredAt must use canonical UTC RFC3339", common.ErrInvalidArgument)
	}
	appendRecord := func(context.Context) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		if item.StreamPosition > r.streamPosition {
			r.streamPosition = item.StreamPosition
		}
		key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
		if _, ok := r.items[key]; ok {
			return common.ErrConflict
		}
		r.items[key] = cloneEventRecord(item)
		return nil
	}
	if item.WorkspaceID == "" || r.workspaceDeletions == nil {
		return appendRecord(ctx)
	}
	return r.workspaceDeletions.WithWorkspaceActiveFence(ctx, item.WorkspaceID, appendRecord)
}

func (r *EventRecordRepository) ReserveEventRecordStreamPosition(_ context.Context) (int64, error) {
	if r == nil {
		return 0, common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.streamPosition == int64(^uint64(0)>>1) {
		return 0, common.ErrConflict
	}
	r.streamPosition++
	return r.streamPosition, nil
}

func (r *EventRecordRepository) Get(ctx context.Context, id common.EventRecordID) (*eventstream.EventRecord, error) {
	if r == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	r.mu.RLock()
	candidates := make([]*eventstream.EventRecord, 0, 1)
	for _, item := range r.items {
		if item == nil || item.ID != id || (workspaceID != "" && item.WorkspaceID != workspaceID) {
			continue
		}
		candidates = append(candidates, cloneEventRecord(item))
	}
	r.mu.RUnlock()
	visible := candidates[:0]
	for _, item := range candidates {
		hidden, err := r.workspaceDeletionHidden(ctx, item.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if !hidden {
			visible = append(visible, item)
		}
	}
	if len(visible) == 0 {
		return nil, common.ErrNotFound
	}
	if len(visible) != 1 {
		return nil, common.ErrConflict
	}
	if err := validatePersistedEventOccurredAt(visible[0]); err != nil {
		return nil, err
	}
	return visible[0], nil
}

func (r *EventRecordRepository) List(ctx context.Context, query workorchquerycontract.EventRecordQuery) ([]*eventstream.EventRecord, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	var err error
	query, err = query.Canonicalized()
	if err != nil {
		return nil, err
	}
	if query.WorkspaceID == "" {
		query.WorkspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	if query.WorkspaceID != "" {
		hidden, err := r.workspaceDeletionHidden(ctx, query.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if hidden {
			return nil, nil
		}
	}
	r.mu.RLock()
	unbounded := query
	unbounded.CursorAfterID = ""
	unbounded.Limit = 0
	items, err := listEventRecords(r.items, unbounded)
	r.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	visible := items[:0]
	visibility := map[common.WorkspaceID]bool{}
	for _, item := range items {
		hidden, known := visibility[item.WorkspaceID]
		if !known {
			var err error
			hidden, err = r.workspaceDeletionHidden(ctx, item.WorkspaceID)
			if err != nil {
				return nil, err
			}
			visibility[item.WorkspaceID] = hidden
		}
		if !hidden {
			visible = append(visible, item)
		}
	}
	if query.WorkspaceID == "" && query.RunID != "" {
		matchedWorkspaces := map[common.WorkspaceID]struct{}{}
		for _, item := range visible {
			if item != nil {
				matchedWorkspaces[item.WorkspaceID] = struct{}{}
			}
		}
		if len(matchedWorkspaces) > 1 {
			return nil, common.ErrConflict
		}
	}
	items = applyMemoryEventRecordCursorAndLimit(visible, query)
	if !query.IncludePayload {
		for _, item := range items {
			stripEventRecordPayload(item)
		}
	}
	return items, nil
}

func (r *EventRecordRepository) Delete(ctx context.Context, id common.EventRecordID) error {
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	deleteRecord := func(context.Context) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		key := workspaceScopedKey(current.WorkspaceID, string(id))
		item := r.items[key]
		if item == nil || item.ID != id || item.WorkspaceID != current.WorkspaceID {
			return common.ErrNotFound
		}
		delete(r.items, key)
		return nil
	}
	if current.WorkspaceID == "" || r.workspaceDeletions == nil {
		return deleteRecord(ctx)
	}
	return r.workspaceDeletions.WithWorkspaceActiveFence(ctx, current.WorkspaceID, deleteRecord)
}

func (r *EventRecordRepository) PurgeWorkspaceEventRecords(ctx context.Context, expected eventstream.WorkspaceDeletion) (int, error) {
	if r == nil || r.workspaceDeletions == nil || expected.Validate() != nil || expected.Phase != eventstream.WorkspaceDeletionPhaseFenced || expected.Mode != deletioncontract.ModePurge {
		return 0, common.ErrInvalidArgument
	}
	deleted := 0
	err := r.workspaceDeletions.WithWorkspacePurgeFence(ctx, expected, func(context.Context) error {
		current := r.workspaceDeletions.items[expected.WorkspaceID]
		if current == nil || current.Validate() != nil || current.Phase != eventstream.WorkspaceDeletionPhaseFenced ||
			current.Revision != expected.Revision || current.FenceID != expected.FenceID || current.SealAckHash != expected.SealAckHash ||
			current.RequestedAt != expected.RequestedAt || current.UpdatedAt != expected.UpdatedAt {
			return common.ErrConflict
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		for key, item := range r.items {
			if item != nil && item.WorkspaceID == expected.WorkspaceID {
				delete(r.items, key)
				deleted++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func (r *EventRecordRepository) workspaceDeletionHidden(ctx context.Context, workspaceID common.WorkspaceID) (bool, error) {
	if workspaceID == "" || r.workspaceDeletions == nil {
		return false, nil
	}
	_, err := r.workspaceDeletions.Get(ctx, workspaceID)
	if err == common.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func applyMemoryEventRecordCursorAndLimit(items []*eventstream.EventRecord, query workorchquerycontract.EventRecordQuery) []*eventstream.EventRecord {
	if query.CursorAfterID != "" {
		cursor := -1
		for idx, item := range items {
			if item != nil && item.ID == query.CursorAfterID {
				cursor = idx
				break
			}
		}
		if cursor < 0 || cursor+1 >= len(items) {
			return nil
		}
		items = items[cursor+1:]
	}
	if query.Limit > 0 && len(items) > query.Limit {
		if query.CursorAfterID == "" {
			items = items[len(items)-query.Limit:]
		} else {
			items = items[:query.Limit]
		}
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func NewWorkspaceRepository() *WorkspaceRepository {
	return newWorkspaceRepository(repository.NewWorkspaceOwnerRepositoryBinding())
}

func newWorkspaceRepository(binding repository.OwnerRepositoryBinding) *WorkspaceRepository {
	return &WorkspaceRepository{items: map[common.WorkspaceID]*workspace.Workspace{}, ownerBinding: binding}
}

func (r *WorkspaceRepository) Create(_ context.Context, item *workspace.Workspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := preparationcompiler.ValidatePreparedPersistence(item); err != nil {
		return err
	}
	if err := workspace.ValidatePreparationFenceTransition(nil, item); err != nil {
		return err
	}
	if err := workspace.ValidatePreparationRecordTransition(nil, item, false); err != nil {
		return err
	}
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	if item.Revision == 0 {
		item.Revision = 1
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
	if err := preparationcompiler.ValidatePreparedPersistence(item); err != nil {
		return nil, err
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
		if err := preparationcompiler.ValidatePreparedPersistence(r.items[common.WorkspaceID(id)]); err != nil {
			return nil, err
		}
		ret = append(ret, cloneWorkspace(r.items[common.WorkspaceID(id)]))
	}
	return ret, nil
}

func (r *WorkspaceRepository) Update(_ context.Context, item *workspace.Workspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item == nil {
		return common.ErrInvalidArgument
	}
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	if err := workspace.ValidatePreparationFenceUpdate(r.items[item.ID], item); err != nil {
		return err
	}
	if err := workspace.ValidatePreparationRecordTransition(r.items[item.ID], item, false); err != nil {
		return err
	}
	if err := preparationcompiler.ValidatePreparedPersistence(item); err != nil {
		return err
	}
	r.items[item.ID] = cloneWorkspace(item)
	return nil
}

func (r *WorkspaceRepository) CompareAndSwap(_ context.Context, item *workspace.Workspace, expectedRevision uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item == nil {
		return common.ErrInvalidArgument
	}
	current, ok := r.items[item.ID]
	if !ok {
		return common.ErrNotFound
	}
	if current.Revision != expectedRevision {
		return common.ErrConflict
	}
	if err := workspace.ValidatePreparationInitializationPreimage(current, item); err != nil {
		return err
	}
	if err := workspace.ValidatePreparationExecutionInputsPreimage(current, item); err != nil {
		return err
	}
	if err := workspace.ValidatePreparationFenceTransition(current, item); err != nil {
		return err
	}
	if err := workspace.ValidatePreparationRecordTransition(current, item, true); err != nil {
		return err
	}
	if err := preparationcompiler.ValidatePreparedPersistence(item); err != nil {
		return err
	}
	item.Revision = expectedRevision + 1
	r.items[item.ID] = cloneWorkspace(item)
	return nil
}

func (r *WorkspaceRepository) CleanupDeletionMaterial(ctx context.Context, scope repository.WorkspaceDeletionMaterialCleanupScope) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := repository.ValidateWorkspaceDeletionMaterialCleanupContext(ctx, scope); err != nil {
		return err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return repository.ValidateWorkspaceDeletionMaterialCleanupCurrent(r.items[scope.WorkspaceID], scope)
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
	cloned.PreparationFence = item.PreparationFence.Clone()
	cloned.AuthoringSummary = nil
	if item.DefaultLLMSelection != nil {
		target := item.DefaultLLMSelection.Clone()
		cloned.DefaultLLMSelection = &target
	}
	if item.SoulBindings != nil {
		cloned.SoulBindings = map[string]common.SoulID{}
		for key, value := range item.SoulBindings {
			cloned.SoulBindings[key] = value
		}
	}
	cloned.ContextHints = item.ContextHints.Clone()
	cloned.EvidencePolicies = workorcheventpayload.CloneSlice(item.EvidencePolicies)
	if item.DefinitionAdmissionCandidates != nil {
		cloned.DefinitionAdmissionCandidates = make([]workspace.DefinitionAdmissionCandidate, len(item.DefinitionAdmissionCandidates))
		for idx := range item.DefinitionAdmissionCandidates {
			cloned.DefinitionAdmissionCandidates[idx] = item.DefinitionAdmissionCandidates[idx].Clone()
		}
	}
	if item.DefinitionAuthoringCreation != nil {
		creation := item.DefinitionAuthoringCreation.Clone()
		cloned.DefinitionAuthoringCreation = &creation
	}
	if item.StartupPreparation != nil {
		record := item.StartupPreparation.Clone()
		cloned.StartupPreparation = &record
	}
	cloned.PreparationDirectoryEvidence = workspace.ClonePreparationDirectoryEvidence(item.PreparationDirectoryEvidence)
	cloned.PreparationResourceEvidence = workspace.ClonePreparationResourceEvidence(item.PreparationResourceEvidence)
	cloned.PreparationManagedConfigEvidence = workspace.ClonePreparationManagedConfigEvidence(item.PreparationManagedConfigEvidence)
	cloned.PreparationSoulEvidence = workspace.ClonePreparationSoulEvidence(item.PreparationSoulEvidence)
	cloned.PreparationSkillRegistryEvidence = workspace.ClonePreparationSkillRegistryEvidence(item.PreparationSkillRegistryEvidence)
	cloned.PreparationProviderCacheEvidence = workspace.ClonePreparationProviderCacheEvidence(item.PreparationProviderCacheEvidence)
	cloned.PreparationRunHistoryEvidence = workspace.ClonePreparationRunHistoryEvidence(item.PreparationRunHistoryEvidence)
	cloned.PreparationSealEvidence = workspace.ClonePreparationSealEvidence(item.PreparationSealEvidence)
	cloned.PreparationMemoryInventoryEvidence = workspace.ClonePreparationMemoryInventoryEvidence(item.PreparationMemoryInventoryEvidence)
	cloned.PreparationMCPRegistryEvidence = workspace.ClonePreparationMCPRegistryEvidence(item.PreparationMCPRegistryEvidence)
	cloned.PreparationResourceInitializations = workspace.ClonePreparationInitializations(item.PreparationResourceInitializations)
	cloned.PreparationSkillHubEffects = workspace.ClonePreparationSkillHubEffects(item.PreparationSkillHubEffects)
	if item.DefinitionAuthoring != nil {
		record := item.DefinitionAuthoring.Clone()
		cloned.DefinitionAuthoring = &record
	}
	if item.DefinitionChangeCandidate != nil {
		candidate := item.DefinitionChangeCandidate.Clone()
		cloned.DefinitionChangeCandidate = &candidate
	}
	if item.DefinitionAuthoringConfirmation != nil {
		confirmation := item.DefinitionAuthoringConfirmation.Clone()
		cloned.DefinitionAuthoringConfirmation = &confirmation
	}
	if item.DefinitionAuthoringHistory != nil {
		cloned.DefinitionAuthoringHistory = make([]workspace.DefinitionAuthoringConfirmedRevision, len(item.DefinitionAuthoringHistory))
		for index, revision := range item.DefinitionAuthoringHistory {
			cloned.DefinitionAuthoringHistory[index] = revision.Clone()
		}
	}
	if item.DefinitionSpecificationLedger != nil {
		ledger := item.DefinitionSpecificationLedger.Clone()
		cloned.DefinitionSpecificationLedger = &ledger
	}
	if item.StartupInput != nil {
		value := item.StartupInput.Clone()
		cloned.StartupInput = &value
	}
	if item.ActivationAdmission != nil {
		value := item.ActivationAdmission.Normalized()
		cloned.ActivationAdmission = &value
	}
	if item.Deletion != nil {
		value := item.Deletion.Clone()
		cloned.Deletion = &value
	}
	return &cloned
}

type GoalRepository struct {
	mu           sync.RWMutex
	items        map[string]*workspace.Goal
	ownerBinding repository.OwnerRepositoryBinding
}

func NewGoalRepository() *GoalRepository {
	return newGoalRepository(repository.NewWorkspaceOwnerRepositoryBinding())
}

func newGoalRepository(binding repository.OwnerRepositoryBinding) *GoalRepository {
	return &GoalRepository{items: map[string]*workspace.Goal{}, ownerBinding: binding}
}

func (r *GoalRepository) Create(ctx context.Context, item *workspace.Goal) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, item.WorkspaceID); err != nil {
		return err
	}
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

func (r *GoalRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspace.Goal, error) {
	if err := validateRuntimeWorkspaceScope(ctx, workspaceID); err != nil {
		return nil, err
	}
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
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, item.WorkspaceID); err != nil {
		return err
	}
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
	mu           sync.RWMutex
	items        map[string]*workspace.DoD
	ownerBinding repository.OwnerRepositoryBinding
}

func NewDoDRepository() *DoDRepository {
	return newDoDRepository(repository.NewWorkspaceOwnerRepositoryBinding())
}

func newDoDRepository(binding repository.OwnerRepositoryBinding) *DoDRepository {
	return &DoDRepository{items: map[string]*workspace.DoD{}, ownerBinding: binding}
}

func (r *DoDRepository) Create(ctx context.Context, item *workspace.DoD) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, item.WorkspaceID); err != nil {
		return err
	}
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

func (r *DoDRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspace.DoD, error) {
	if err := validateRuntimeWorkspaceScope(ctx, workspaceID); err != nil {
		return nil, err
	}
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
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, item.WorkspaceID); err != nil {
		return err
	}
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
	mu                     sync.RWMutex
	items                  map[string]*run.Run
	workspaceGenerations   map[common.WorkspaceID]uint64
	workspaceRetirements   map[common.WorkspaceID]run.WorkspaceRetirementFence
	workspacePurgeReceipts map[common.WorkspaceID]repository.RunWorkspaceResourcePurgeReceipt
	ownerBinding           repository.RunOwnerRepositoryBinding
}

func NewRunRepository() *RunRepository {
	return &RunRepository{
		items:                  map[string]*run.Run{},
		workspaceGenerations:   map[common.WorkspaceID]uint64{},
		workspaceRetirements:   map[common.WorkspaceID]run.WorkspaceRetirementFence{},
		workspacePurgeReceipts: map[common.WorkspaceID]repository.RunWorkspaceResourcePurgeReceipt{},
		ownerBinding:           repository.NewRunOwnerRepositoryBinding(),
	}
}

func (r *RunRepository) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	if r == nil {
		return repository.RunOwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *RunRepository) Create(_ context.Context, item *run.Run) error {
	return r.create(item, false)
}

func (r *RunRepository) CreateNextWorkspaceRun(_ context.Context, item *run.Run) error {
	return r.create(item, true)
}

func (r *RunRepository) create(item *run.Run, allocateID bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item == nil || item.WorkspaceID == "" || (allocateID && item.ID != "") || (!allocateID && item.ID == "") {
		return common.ErrInvalidArgument
	}
	if err := run.ValidateRunRuntimeBootstrapsRaw(item); err != nil {
		return err
	}
	if err := validateMemoryRunInventory(r.items); err != nil {
		return err
	}
	if _, retired := r.workspaceRetirements[item.WorkspaceID]; retired {
		return common.ErrConflict
	}
	if !allocateID {
		key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
		if _, ok := r.items[key]; ok {
			return common.ErrConflict
		}
	}
	if item.WorkspaceRunGeneration != 0 {
		return common.ErrInvalidArgument
	}
	if !runRepositoryStateIsTerminal(item.State) {
		for _, current := range r.items {
			if current.WorkspaceID == item.WorkspaceID && !runRepositoryStateIsTerminal(current.State) {
				return common.ErrConflict
			}
		}
	}
	watermark := r.workspaceGenerations[item.WorkspaceID]
	for {
		if watermark == math.MaxUint64 {
			return common.ErrConflict
		}
		watermark++
		if !allocateID {
			break
		}
		candidate := run.WorkspaceRunID(watermark)
		if _, occupied := r.items[workspaceScopedKey(item.WorkspaceID, string(candidate))]; occupied {
			continue
		}
		item.ID = candidate
		break
	}
	item.WorkspaceRunGeneration = watermark
	if item.Revision == 0 {
		item.Revision = 1
	}
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	r.items[key] = cloneRun(item)
	r.workspaceGenerations[item.WorkspaceID] = item.WorkspaceRunGeneration
	return nil
}

func runRepositoryStateIsTerminal(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func validateMemoryRunInventory(items map[string]*run.Run) error {
	for key, item := range items {
		if err := run.ValidateRunRuntimeBootstrapsRaw(item); err != nil {
			return fmt.Errorf("load Run %q: %w", key, err)
		}
	}
	return nil
}

func (r *RunRepository) Get(ctx context.Context, id common.RunID) (*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := validateMemoryRunInventory(r.items); err != nil {
		return nil, err
	}
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

func (r *RunRepository) GetByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID, id common.RunID) (*run.Run, error) {
	if workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := validateMemoryRunInventory(r.items); err != nil {
		return nil, err
	}
	item, ok := r.items[workspaceScopedKey(workspaceID, string(id))]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneRun(item), nil
}

func (r *RunRepository) List(_ context.Context) ([]*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := validateMemoryRunInventory(r.items); err != nil {
		return nil, err
	}

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
	if err := validateMemoryRunInventory(r.items); err != nil {
		return nil, err
	}

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
	if err := run.ValidateRunRuntimeBootstrapsRaw(item); err != nil {
		return err
	}
	if err := validateMemoryRunInventory(r.items); err != nil {
		return err
	}
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
	if _, retired := r.workspaceRetirements[r.items[key].WorkspaceID]; retired {
		return common.ErrConflict
	}
	if item.WorkspaceRunGeneration == 0 || item.WorkspaceRunGeneration != r.items[key].WorkspaceRunGeneration {
		return common.ErrConflict
	}
	if err := run.ValidateExecutionIntentReceiptTransition(r.items[key], item, false); err != nil {
		return err
	}
	r.items[key] = cloneRun(item)
	return nil
}

func (r *RunRepository) CompareAndSwap(ctx context.Context, item *run.Run, expectedRevision uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := run.ValidateRunRuntimeBootstrapsRaw(item); err != nil {
		return err
	}
	if err := validateMemoryRunInventory(r.items); err != nil {
		return err
	}
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
	current, ok := r.items[key]
	if !ok {
		return common.ErrNotFound
	}
	if _, retired := r.workspaceRetirements[current.WorkspaceID]; retired {
		return common.ErrConflict
	}
	if current.Revision != expectedRevision {
		return common.ErrConflict
	}
	if item.WorkspaceRunGeneration == 0 || item.WorkspaceRunGeneration != current.WorkspaceRunGeneration {
		return common.ErrConflict
	}
	if err := run.ValidateExecutionIntentReceiptTransition(current, item, true); err != nil {
		return err
	}
	item.Revision = expectedRevision + 1
	r.items[key] = cloneRun(item)
	return nil
}

func (r *RunRepository) CompareAndSwapRuntimeState(ctx context.Context, item *run.Run, expectedRevision, expectedRuntimeStateRevision uint64) error {
	if r == nil || item == nil || item.ID == "" || item.WorkspaceID == "" || expectedRevision == 0 {
		return common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, item.WorkspaceID); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := run.ValidateRunRuntimeBootstrapsRaw(item); err != nil {
		return err
	}
	if err := validateMemoryRunInventory(r.items); err != nil {
		return err
	}
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	current, ok := r.items[key]
	if !ok || current == nil {
		return common.ErrNotFound
	}
	if current.Revision != expectedRevision {
		return common.ErrConflict
	}
	if fence, retiring := r.workspaceRetirements[current.WorkspaceID]; retiring {
		if err := fence.Validate(); err != nil || fence.IsRetired() || !runRepositoryStateIsTerminal(current.State) {
			return common.ErrConflict
		}
	}
	if err := run.ValidateOpaqueRuntimeStateCAS(current, item, expectedRuntimeStateRevision); err != nil {
		return err
	}
	item.Revision = expectedRevision + 1
	r.items[key] = cloneRun(item)
	return nil
}

func (r *RunRepository) Delete(ctx context.Context, id common.RunID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := validateMemoryRunInventory(r.items); err != nil {
		return err
	}
	if key, scoped := runtimeWorkspaceScopedKey(ctx, string(id)); scoped {
		current, ok := r.items[key]
		if !ok {
			return common.ErrNotFound
		}
		if _, retired := r.workspaceRetirements[current.WorkspaceID]; retired {
			return common.ErrConflict
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
	if _, retired := r.workspaceRetirements[r.items[found].WorkspaceID]; retired {
		return common.ErrConflict
	}
	delete(r.items, found)
	return nil
}

func (r *RunRepository) PrepareWorkspaceRetirement(ctx context.Context, intent run.WorkspaceRetirementIntent) (*run.WorkspaceRetirementFence, error) {
	intent = intent.Normalized()
	if err := intent.Validate(); err != nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, intent.WorkspaceID); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := validateMemoryRunInventory(r.items); err != nil {
		return nil, err
	}
	if current, ok := r.workspaceRetirements[intent.WorkspaceID]; ok {
		if current.Intent() != intent {
			return nil, common.ErrConflict
		}
		clone := current
		return &clone, nil
	}
	items := make([]*run.Run, 0)
	for _, item := range r.items {
		if item != nil && item.WorkspaceID == intent.WorkspaceID {
			items = append(items, cloneRun(item))
		}
	}
	if err := run.WorkspaceRetirementEligible(items, intent.WorkspaceID); err != nil {
		return nil, err
	}
	fence := run.WorkspaceRetirementFence{
		WorkspaceID: intent.WorkspaceID, DeletionID: intent.DeletionID, DeletionGeneration: intent.DeletionGeneration,
		DataDispositionIntent: intent.DataDispositionIntent, WorkspaceIntentHash: intent.WorkspaceIntentHash,
		Phase: run.WorkspaceRetirementPhaseDraining, FenceRevision: 1,
		WorkspaceRunGeneration: r.workspaceGenerations[intent.WorkspaceID],
	}
	r.workspaceRetirements[intent.WorkspaceID] = fence
	clone := fence
	return &clone, nil
}

func (r *RunRepository) FinalizeWorkspaceRetirement(ctx context.Context, token deletioncontract.WorkspaceRuntimeQuiescenceToken) (*run.WorkspaceRetirementFence, error) {
	token = token.Normalized()
	if r == nil || token.Validate() != nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, token.WorkspaceID); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := validateMemoryRunInventory(r.items); err != nil {
		return nil, err
	}
	current, ok := r.workspaceRetirements[token.WorkspaceID]
	if !ok {
		return nil, common.ErrNotFound
	}
	if current.IsRetired() {
		if run.ValidateWorkspaceRetirementFinalizationReplay(current, token) != nil {
			return nil, common.ErrConflict
		}
		clone := current
		return &clone, nil
	}
	items := make([]*run.Run, 0)
	for _, item := range r.items {
		if item != nil && item.WorkspaceID == token.WorkspaceID {
			items = append(items, cloneRun(item))
		}
	}
	if r.workspaceGenerations[token.WorkspaceID] != token.WorkspaceRunGeneration || run.ValidateWorkspaceRetirementFinalization(current, items, token) != nil {
		return nil, common.ErrConflict
	}
	current.Phase = run.WorkspaceRetirementPhaseRetired
	current.FenceRevision++
	current.QuiescenceID = token.QuiescenceID
	current.RuntimeInventoryHash = token.RuntimeInventoryHash
	if err := current.Validate(); err != nil {
		return nil, err
	}
	r.workspaceRetirements[token.WorkspaceID] = current
	clone := current
	return &clone, nil
}

func (r *RunRepository) GetWorkspaceRetirement(ctx context.Context, workspaceID common.WorkspaceID) (*run.WorkspaceRetirementFence, error) {
	workspaceID = common.WorkspaceID(strings.TrimSpace(string(workspaceID)))
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, workspaceID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	fence, ok := r.workspaceRetirements[workspaceID]
	if !ok {
		return nil, common.ErrNotFound
	}
	clone := fence
	return &clone, nil
}

func (r *RunRepository) ListWorkspaceRetirements(_ context.Context) ([]run.WorkspaceRetirementFence, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.workspaceRetirements))
	for workspaceID := range r.workspaceRetirements {
		ids = append(ids, string(workspaceID))
	}
	sort.Strings(ids)
	ret := make([]run.WorkspaceRetirementFence, 0, len(ids))
	for _, workspaceID := range ids {
		ret = append(ret, r.workspaceRetirements[common.WorkspaceID(workspaceID)])
	}
	return ret, nil
}

func (r *RunRepository) withRunResourceMutation(ctx context.Context, runID common.RunID, mutate func(common.WorkspaceID) error) error {
	if r == nil || runID == "" || mutate == nil {
		return common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := validateMemoryRunInventory(r.items); err != nil {
		return err
	}
	key, err := findScopedMemoryKey(ctx, r.items, string(runID), func(item *run.Run) string { return string(item.ID) })
	if err != nil {
		return err
	}
	current := r.items[key]
	if current == nil {
		return common.ErrNotFound
	}
	if _, retired := r.workspaceRetirements[current.WorkspaceID]; retired {
		return common.ErrConflict
	}
	return mutate(current.WorkspaceID)
}

type StepRepository struct {
	mu           sync.RWMutex
	items        map[string]*run.Step
	runs         *RunRepository
	ownerBinding repository.RunOwnerRepositoryBinding
}

func NewStepRepository() *StepRepository {
	return &StepRepository{items: map[string]*run.Step{}, ownerBinding: repository.NewRunOwnerRepositoryBinding()}
}

func NewRunGuardedStepRepository(runs *RunRepository) *StepRepository {
	if runs == nil {
		return &StepRepository{items: map[string]*run.Step{}}
	}
	return &StepRepository{items: map[string]*run.Step{}, runs: runs, ownerBinding: runs.ownerBinding}
}

func (r *StepRepository) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	if r == nil {
		return repository.RunOwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *StepRepository) Create(ctx context.Context, item *run.Step) error {
	if item == nil || item.ID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	mutate := func(workspaceID common.WorkspaceID) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		key := workspaceScopedKey(workspaceID, string(item.ID))
		if _, ok := r.items[key]; ok {
			return common.ErrConflict
		}
		r.items[key] = cloneStep(item)
		return nil
	}
	if r.runs == nil {
		return mutate(common.RuntimeWorkspaceIDFromContext(ctx))
	}
	return r.runs.withRunResourceMutation(ctx, item.RunID, mutate)
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

func (r *StepRepository) ListByRunSummary(ctx context.Context, runID common.RunID) ([]*run.Step, error) {
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
		items = append(items, compactStepSummary(item))
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
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.RunID == "" {
		r.mu.RLock()
		_, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *run.Step) string { return string(current.ID) })
		r.mu.RUnlock()
		if err != nil {
			return err
		}
		return common.ErrInvalidArgument
	}
	mutate := func(_ common.WorkspaceID) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		key, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *run.Step) string { return string(current.ID) })
		if err != nil {
			return err
		}
		if r.items[key].RunID != item.RunID {
			return common.ErrConflict
		}
		r.items[key] = cloneStep(item)
		return nil
	}
	if r.runs == nil {
		return mutate("")
	}
	return r.runs.withRunResourceMutation(ctx, item.RunID, mutate)
}

func (r *StepRepository) Delete(ctx context.Context, id common.StepID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	mutate := func(_ common.WorkspaceID) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *run.Step) string { return string(item.ID) })
		if err != nil {
			return err
		}
		if r.items[key].RunID != current.RunID {
			return common.ErrConflict
		}
		delete(r.items, key)
		return nil
	}
	if r.runs == nil {
		return mutate("")
	}
	return r.runs.withRunResourceMutation(ctx, current.RunID, mutate)
}

type ToolCallRepository struct {
	mu    sync.RWMutex
	items map[string]*runtimeevidencemodel.ToolCall
}

func NewToolCallRepository() *ToolCallRepository {
	return &ToolCallRepository{items: map[string]*runtimeevidencemodel.ToolCall{}}
}

func (r *ToolCallRepository) Create(_ context.Context, item *runtimeevidencemodel.ToolCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneToolCall(item)
	return nil
}

func (r *ToolCallRepository) Get(ctx context.Context, id common.ToolCallID) (*runtimeevidencemodel.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *runtimeevidencemodel.ToolCall) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	return cloneToolCall(r.items[key]), nil
}

func (r *ToolCallRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimeevidencemodel.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := runScopedListHasConflict(ctx, r.items, runID, func(item *runtimeevidencemodel.ToolCall) common.RunID {
		return item.RunID
	}, func(item *runtimeevidencemodel.ToolCall) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*runtimeevidencemodel.ToolCall, 0, len(matches))
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

func (r *ToolCallRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runtimeevidencemodel.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := stepScopedListHasConflict(ctx, r.items, stepID, func(item *runtimeevidencemodel.ToolCall) common.StepID {
		return item.StepID
	}, func(item *runtimeevidencemodel.ToolCall) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*runtimeevidencemodel.ToolCall, 0, len(matches))
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

func (r *ToolCallRepository) Update(ctx context.Context, item *runtimeevidencemodel.ToolCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *runtimeevidencemodel.ToolCall) string {
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
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *runtimeevidencemodel.ToolCall) string {
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
	items map[string]*runtimeevidencemodel.Patch
}

func NewPatchRepository() *PatchRepository {
	return &PatchRepository{items: map[string]*runtimeevidencemodel.Patch{}}
}

func (r *PatchRepository) Create(_ context.Context, item *runtimeevidencemodel.Patch) error {
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

func (r *PatchRepository) Get(ctx context.Context, id common.PatchID) (*runtimeevidencemodel.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *runtimeevidencemodel.Patch) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	cloned := clonePatch(r.items[key])
	return &cloned, nil
}

func (r *PatchRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimeevidencemodel.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := runScopedListHasConflict(ctx, r.items, runID, func(item *runtimeevidencemodel.Patch) common.RunID {
		return item.RunID
	}, func(item *runtimeevidencemodel.Patch) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*runtimeevidencemodel.Patch, 0, len(matches))
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

func (r *PatchRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runtimeevidencemodel.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := stepScopedListHasConflict(ctx, r.items, stepID, func(item *runtimeevidencemodel.Patch) common.StepID {
		return item.StepID
	}, func(item *runtimeevidencemodel.Patch) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*runtimeevidencemodel.Patch, 0, len(matches))
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

func (r *PatchRepository) Update(ctx context.Context, item *runtimeevidencemodel.Patch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *runtimeevidencemodel.Patch) string {
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
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *runtimeevidencemodel.Patch) string {
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
	items map[string]*runtimeevidencemodel.ValidationResult
}

func NewValidationResultRepository() *ValidationResultRepository {
	return &ValidationResultRepository{items: map[string]*runtimeevidencemodel.ValidationResult{}}
}

func (r *ValidationResultRepository) Create(_ context.Context, item *runtimeevidencemodel.ValidationResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneValidationResult(item)
	return nil
}

func (r *ValidationResultRepository) Get(ctx context.Context, id common.ValidationResultID) (*runtimeevidencemodel.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *runtimeevidencemodel.ValidationResult) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	return cloneValidationResult(r.items[key]), nil
}

func (r *ValidationResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimeevidencemodel.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := runScopedListHasConflict(ctx, r.items, runID, func(item *runtimeevidencemodel.ValidationResult) common.RunID {
		return item.RunID
	}, func(item *runtimeevidencemodel.ValidationResult) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*runtimeevidencemodel.ValidationResult, 0, len(matches))
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

func (r *ValidationResultRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runtimeevidencemodel.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches, conflict := stepScopedListHasConflict(ctx, r.items, stepID, func(item *runtimeevidencemodel.ValidationResult) common.StepID {
		return item.StepID
	}, func(item *runtimeevidencemodel.ValidationResult) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	items := make([]*runtimeevidencemodel.ValidationResult, 0, len(matches))
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

func (r *ValidationResultRepository) Update(ctx context.Context, item *runtimeevidencemodel.ValidationResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *runtimeevidencemodel.ValidationResult) string {
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
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *runtimeevidencemodel.ValidationResult) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
	return nil
}

type RuntimeCheckpointRepository struct {
	mu           sync.RWMutex
	items        map[string]*run.RuntimeCheckpoint
	runs         *RunRepository
	ownerBinding repository.RunOwnerRepositoryBinding
}

func NewRuntimeCheckpointRepository() *RuntimeCheckpointRepository {
	return &RuntimeCheckpointRepository{items: map[string]*run.RuntimeCheckpoint{}, ownerBinding: repository.NewRunOwnerRepositoryBinding()}
}

func NewRunGuardedRuntimeCheckpointRepository(runs *RunRepository) *RuntimeCheckpointRepository {
	if runs == nil {
		return &RuntimeCheckpointRepository{items: map[string]*run.RuntimeCheckpoint{}}
	}
	return &RuntimeCheckpointRepository{items: map[string]*run.RuntimeCheckpoint{}, runs: runs, ownerBinding: runs.ownerBinding}
}

func (r *RuntimeCheckpointRepository) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	if r == nil {
		return repository.RunOwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *RuntimeCheckpointRepository) Create(ctx context.Context, item *run.RuntimeCheckpoint) error {
	if item == nil || item.ID == "" || item.RunID == "" || item.WorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	mutate := func(workspaceID common.WorkspaceID) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		if item.WorkspaceID != workspaceID {
			return common.ErrConflict
		}
		key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
		if _, ok := r.items[key]; ok {
			return common.ErrConflict
		}
		r.items[key] = cloneRuntimeCheckpoint(item)
		return nil
	}
	if r.runs == nil {
		return mutate(item.WorkspaceID)
	}
	return r.runs.withRunResourceMutation(ctx, item.RunID, mutate)
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
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.RunID == "" {
		r.mu.RLock()
		_, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *run.RuntimeCheckpoint) string { return string(current.ID) })
		r.mu.RUnlock()
		if err != nil {
			return err
		}
		return common.ErrInvalidArgument
	}
	mutate := func(workspaceID common.WorkspaceID) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
		if item.WorkspaceID == "" {
			found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *run.RuntimeCheckpoint) string { return string(current.ID) })
			if err != nil {
				return err
			}
			key = found
			item.WorkspaceID = r.items[key].WorkspaceID
		}
		current, ok := r.items[key]
		if !ok {
			return common.ErrNotFound
		}
		if current.RunID != item.RunID {
			return common.ErrConflict
		}
		if item.WorkspaceID != "" && item.WorkspaceID != workspaceID {
			return common.ErrConflict
		}
		r.items[key] = cloneRuntimeCheckpoint(item)
		return nil
	}
	if r.runs == nil {
		return mutate(item.WorkspaceID)
	}
	return r.runs.withRunResourceMutation(ctx, item.RunID, mutate)
}

func (r *RuntimeCheckpointRepository) Delete(ctx context.Context, id common.CheckpointID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	mutate := func(_ common.WorkspaceID) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *run.RuntimeCheckpoint) string { return string(item.ID) })
		if err != nil {
			return err
		}
		if r.items[key].RunID != current.RunID {
			return common.ErrConflict
		}
		delete(r.items, key)
		return nil
	}
	if r.runs == nil {
		return mutate("")
	}
	return r.runs.withRunResourceMutation(ctx, current.RunID, mutate)
}

type DecisionTicketRepository struct {
	mu                 sync.RWMutex
	items              map[string]*decision.Ticket
	workspaceDeletions *DecisionWorkspaceDeletionRepository
	ownerBinding       repository.OwnerRepositoryBinding
}

func NewDecisionTicketRepository() *DecisionTicketRepository {
	return &DecisionTicketRepository{items: map[string]*decision.Ticket{}}
}

func (r *DecisionTicketRepository) Create(ctx context.Context, item *decision.Ticket) error {
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, item.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
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
	if item.IsContractChange() && (item.ValidateContractChange() != nil || len(item.ContractChangeRevisions) != 1) {
		return common.ErrInvalidArgument
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
	if item.WorkspaceID == "" {
		current, err := r.Get(ctx, item.ID)
		if err != nil {
			return err
		}
		item.WorkspaceID = current.WorkspaceID
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, item.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
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
	if current, ok := r.items[key]; !ok {
		return common.ErrNotFound
	} else if current.IsContractChange() || item.IsContractChange() {
		if err := decision.ValidateContractChangeAppend(current, item); err != nil {
			return err
		}
	}
	if strings.TrimSpace(item.CreatedAt) == "" {
		item.CreatedAt = r.items[key].CreatedAt
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.items[key] = cloneDecisionTicket(item)
	return nil
}

func (r *DecisionTicketRepository) Delete(ctx context.Context, id common.TicketID) error {
	current, resolveErr := r.Get(ctx, id)
	if resolveErr != nil {
		return resolveErr
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, current.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
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
	mu                 sync.RWMutex
	byTicket           map[string]*decision.Resolution
	workspaceDeletions *DecisionWorkspaceDeletionRepository
	ownerBinding       repository.OwnerRepositoryBinding
}

func NewDecisionResolutionRepository() *DecisionResolutionRepository {
	return &DecisionResolutionRepository{byTicket: map[string]*decision.Resolution{}}
}

func (r *DecisionResolutionRepository) Create(ctx context.Context, item *decision.Resolution) error {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, workspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(workspaceID, string(item.TicketID))
	if _, ok := r.byTicket[key]; ok {
		return common.ErrConflict
	}
	cloned := *item
	cloned.Payload = item.Payload.Clone()
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
	cloned.Payload = cloned.Payload.Clone()
	return &cloned, nil
}

func (r *DecisionResolutionRepository) DeleteByTicketID(ctx context.Context, ticketID common.TicketID) error {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, workspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
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
	mu                 sync.RWMutex
	items              map[string]*approval.Approval
	workspaceDeletions *ApprovalWorkspaceDeletionRepository
	ownerBinding       repository.OwnerRepositoryBinding
}

type DecisionWorkspaceDeletionRepository struct {
	mu           sync.RWMutex
	items        map[common.WorkspaceID]*decision.WorkspaceDeletion
	ownerBinding repository.OwnerRepositoryBinding
}

func NewDecisionWorkspaceDeletionRepository() *DecisionWorkspaceDeletionRepository {
	return &DecisionWorkspaceDeletionRepository{items: map[common.WorkspaceID]*decision.WorkspaceDeletion{}}
}
func (r *DecisionWorkspaceDeletionRepository) Create(_ context.Context, item *decision.WorkspaceDeletion) error {
	if r == nil || item == nil || item.Validate() != nil || item.Revision != 1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items[item.WorkspaceID] != nil {
		return common.ErrConflict
	}
	r.items[item.WorkspaceID] = decision.CloneWorkspaceDeletion(item)
	return nil
}
func (r *DecisionWorkspaceDeletionRepository) Get(_ context.Context, id common.WorkspaceID) (*decision.WorkspaceDeletion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.items[id]
	if item == nil {
		return nil, common.ErrNotFound
	}
	return decision.CloneWorkspaceDeletion(item), nil
}
func (r *DecisionWorkspaceDeletionRepository) CompareAndSwap(_ context.Context, item *decision.WorkspaceDeletion, expected uint64) error {
	if item == nil || item.Validate() != nil || item.Revision != expected+1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.items[item.WorkspaceID]
	if current == nil {
		return common.ErrNotFound
	}
	if current.Revision != expected {
		return common.ErrConflict
	}
	r.items[item.WorkspaceID] = decision.CloneWorkspaceDeletion(item)
	return nil
}
func (r *DecisionWorkspaceDeletionRepository) WithWorkspaceActiveFence(ctx context.Context, id common.WorkspaceID, fn func(context.Context) error) error {
	if r == nil || id == "" || fn == nil {
		return common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.items[id] != nil {
		return common.ErrNotFound
	}
	return fn(context.WithValue(ctx, workspaceDeletionGuardContextKey{}, workspaceDeletionGuardContext{owner: "decision", workspaceID: id}))
}

func (r *DecisionWorkspaceDeletionRepository) WithWorkspacePurgeFence(ctx context.Context, scope deletioncontract.Scope, fn func(context.Context) error) error {
	scope = scope.Normalize()
	if r == nil || scope.Validate() != nil || scope.Mode != deletioncontract.ModePurge || fn == nil {
		return common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.items[scope.WorkspaceID]
	if item == nil {
		return common.ErrNotFound
	}
	if item.Scope() != scope {
		return common.ErrConflict
	}
	return fn(context.WithValue(ctx, workspaceDeletionGuardContextKey{}, workspaceDeletionGuardContext{owner: "decision", workspaceID: scope.WorkspaceID}))
}

type ApprovalWorkspaceDeletionRepository struct {
	mu           sync.RWMutex
	items        map[common.WorkspaceID]*approval.WorkspaceDeletion
	ownerBinding repository.OwnerRepositoryBinding
}

func NewApprovalWorkspaceDeletionRepository() *ApprovalWorkspaceDeletionRepository {
	return &ApprovalWorkspaceDeletionRepository{items: map[common.WorkspaceID]*approval.WorkspaceDeletion{}}
}
func (r *ApprovalWorkspaceDeletionRepository) Create(_ context.Context, item *approval.WorkspaceDeletion) error {
	if r == nil || item == nil || item.Validate() != nil || item.Revision != 1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items[item.WorkspaceID] != nil {
		return common.ErrConflict
	}
	r.items[item.WorkspaceID] = approval.CloneWorkspaceDeletion(item)
	return nil
}
func (r *ApprovalWorkspaceDeletionRepository) Get(_ context.Context, id common.WorkspaceID) (*approval.WorkspaceDeletion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.items[id]
	if item == nil {
		return nil, common.ErrNotFound
	}
	return approval.CloneWorkspaceDeletion(item), nil
}
func (r *ApprovalWorkspaceDeletionRepository) CompareAndSwap(_ context.Context, item *approval.WorkspaceDeletion, expected uint64) error {
	if item == nil || item.Validate() != nil || item.Revision != expected+1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.items[item.WorkspaceID]
	if current == nil {
		return common.ErrNotFound
	}
	if current.Revision != expected {
		return common.ErrConflict
	}
	r.items[item.WorkspaceID] = approval.CloneWorkspaceDeletion(item)
	return nil
}
func (r *ApprovalWorkspaceDeletionRepository) WithWorkspaceActiveFence(ctx context.Context, id common.WorkspaceID, fn func(context.Context) error) error {
	if r == nil || id == "" || fn == nil {
		return common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.items[id] != nil {
		return common.ErrNotFound
	}
	return fn(context.WithValue(ctx, workspaceDeletionGuardContextKey{}, workspaceDeletionGuardContext{owner: "approval", workspaceID: id}))
}

func (r *ApprovalWorkspaceDeletionRepository) WithWorkspacePurgeFence(ctx context.Context, scope deletioncontract.Scope, fn func(context.Context) error) error {
	scope = scope.Normalize()
	if r == nil || scope.Validate() != nil || scope.Mode != deletioncontract.ModePurge || fn == nil {
		return common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.items[scope.WorkspaceID]
	if item == nil {
		return common.ErrNotFound
	}
	if item.Scope() != scope {
		return common.ErrConflict
	}
	return fn(context.WithValue(ctx, workspaceDeletionGuardContextKey{}, workspaceDeletionGuardContext{owner: "approval", workspaceID: scope.WorkspaceID}))
}

func (r *DecisionWorkspaceDeletionRepository) guardMutation(ctx context.Context, id common.WorkspaceID) (func(), error) {
	if deletionContextGuards(ctx, "decision", id) {
		return func() {}, nil
	}
	r.mu.RLock()
	item := r.items[id]
	if item != nil {
		r.mu.RUnlock()
		return func() {}, common.ErrNotFound
	}
	return r.mu.RUnlock, nil
}
func (r *ApprovalWorkspaceDeletionRepository) guardMutation(ctx context.Context, id common.WorkspaceID) (func(), error) {
	if deletionContextGuards(ctx, "approval", id) {
		return func() {}, nil
	}
	r.mu.RLock()
	item := r.items[id]
	if item != nil {
		r.mu.RUnlock()
		return func() {}, common.ErrNotFound
	}
	return r.mu.RUnlock, nil
}

type DecisionOwnerRepositorySet struct {
	Tickets            *DecisionTicketRepository
	Resolutions        *DecisionResolutionRepository
	WorkspaceDeletions *DecisionWorkspaceDeletionRepository
	binding            repository.OwnerRepositoryBinding
}

func NewDecisionOwnerRepositorySet() DecisionOwnerRepositorySet {
	binding := repository.NewDecisionOwnerRepositoryBinding()
	d := &DecisionWorkspaceDeletionRepository{items: map[common.WorkspaceID]*decision.WorkspaceDeletion{}, ownerBinding: binding}
	return DecisionOwnerRepositorySet{
		Tickets:            &DecisionTicketRepository{items: map[string]*decision.Ticket{}, workspaceDeletions: d, ownerBinding: binding},
		Resolutions:        &DecisionResolutionRepository{byTicket: map[string]*decision.Resolution{}, workspaceDeletions: d, ownerBinding: binding},
		WorkspaceDeletions: d,
		binding:            binding,
	}
}

func (s DecisionOwnerRepositorySet) DecisionOwnerBinding() repository.OwnerRepositoryBinding {
	return s.binding
}
func (s DecisionOwnerRepositorySet) DecisionOwnerTickets() repository.DecisionTicketRepository {
	return s.Tickets
}
func (s DecisionOwnerRepositorySet) DecisionOwnerResolutions() repository.DecisionResolutionRepository {
	return s.Resolutions
}
func (s DecisionOwnerRepositorySet) DecisionOwnerWorkspaceDeletions() repository.DecisionWorkspaceDeletionRepository {
	return s.WorkspaceDeletions
}
func (s DecisionOwnerRepositorySet) OwnerRepositories() repository.DecisionOwnerRepositories {
	owner, err := repository.NewDecisionOwnerRepositories(s)
	if err != nil {
		panic(err)
	}
	return owner
}

type ApprovalOwnerRepositorySet struct {
	Approvals          *ApprovalRepository
	WorkspaceDeletions *ApprovalWorkspaceDeletionRepository
	binding            repository.OwnerRepositoryBinding
}

func NewApprovalOwnerRepositorySet() ApprovalOwnerRepositorySet {
	binding := repository.NewApprovalOwnerRepositoryBinding()
	d := &ApprovalWorkspaceDeletionRepository{items: map[common.WorkspaceID]*approval.WorkspaceDeletion{}, ownerBinding: binding}
	return ApprovalOwnerRepositorySet{
		Approvals:          &ApprovalRepository{items: map[string]*approval.Approval{}, workspaceDeletions: d, ownerBinding: binding},
		WorkspaceDeletions: d,
		binding:            binding,
	}
}

func (s ApprovalOwnerRepositorySet) ApprovalOwnerBinding() repository.OwnerRepositoryBinding {
	return s.binding
}
func (s ApprovalOwnerRepositorySet) ApprovalOwnerApprovals() repository.ApprovalRepository {
	return s.Approvals
}
func (s ApprovalOwnerRepositorySet) ApprovalOwnerWorkspaceDeletions() repository.ApprovalWorkspaceDeletionRepository {
	return s.WorkspaceDeletions
}
func (s ApprovalOwnerRepositorySet) OwnerRepositories() repository.ApprovalOwnerRepositories {
	owner, err := repository.NewApprovalOwnerRepositories(s)
	if err != nil {
		panic(err)
	}
	return owner
}

func (r *DecisionTicketRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *DecisionResolutionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *DecisionWorkspaceDeletionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *ApprovalRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *ApprovalWorkspaceDeletionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func NewApprovalRepository() *ApprovalRepository {
	return &ApprovalRepository{items: map[string]*approval.Approval{}}
}

func approvalScopedKey(workspaceID common.WorkspaceID, runID common.RunID, id common.ApprovalID) string {
	return string(workspaceID) + "\x00" + string(runID) + "\x00" + string(id)
}

func (r *ApprovalRepository) Create(ctx context.Context, item *approval.Approval) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, item.WorkspaceID); err != nil {
		return err
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, item.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := approvalScopedKey(item.WorkspaceID, item.RunID, item.ID)
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
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	var ret *approval.Approval
	for _, item := range r.items {
		if item.ID != id || (workspaceID != "" && item.WorkspaceID != workspaceID) {
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

func (r *ApprovalRepository) GetScoped(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, id common.ApprovalID) (*approval.Approval, error) {
	if workspaceID == "" || runID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, workspaceID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[approvalScopedKey(workspaceID, runID, id)]
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

func (r *ApprovalRepository) ListByRunScope(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) ([]*approval.Approval, error) {
	if workspaceID == "" || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, workspaceID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ret := make([]*approval.Approval, 0)
	for _, item := range r.items {
		if item.WorkspaceID == workspaceID && item.RunID == runID {
			cloned := cloneApproval(item)
			ret = append(ret, &cloned)
		}
	}
	sort.Slice(ret, func(i, j int) bool { return string(ret[i].ID) < string(ret[j].ID) })
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *ApprovalRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*approval.Approval, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, workspaceID); err != nil {
		return nil, err
	}
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
	if item != nil && r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, item.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, item.WorkspaceID); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := approvalScopedKey(item.WorkspaceID, item.RunID, item.ID)
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

func (r *ApprovalRepository) DeleteScoped(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, id common.ApprovalID) error {
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, workspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
	if workspaceID == "" || runID == "" || id == "" {
		return common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, workspaceID); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := approvalScopedKey(workspaceID, runID, id)
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, key)
	return nil
}

func (r *ApprovalRepository) Delete(ctx context.Context, id common.ApprovalID) error {
	current, resolveErr := r.Get(ctx, id)
	if resolveErr != nil {
		return resolveErr
	}
	if r.workspaceDeletions != nil {
		unlock, err := r.workspaceDeletions.guardMutation(ctx, current.WorkspaceID)
		if err != nil {
			return err
		}
		defer unlock()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	var found string
	for key, item := range r.items {
		if item.ID != id || (workspaceID != "" && item.WorkspaceID != workspaceID) {
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
	cloned.Input = item.Input.Clone()
	cloned.Output = item.Output.Clone()
	return &cloned
}

func compactStepSummary(item *run.Step) *run.Step {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.Input, _ = workorcheventpayload.FromMap(compactStepSummaryInput(item.Input.Map()))
	cloned.Output, _ = workorcheventpayload.FromMap(compactStepSummaryOutput(item.Output.Map()))
	return &cloned
}

func cloneRun(item *run.Run) *run.Run {
	return run.CloneRun(item)
}

func cloneToolCall(item *runtimeevidencemodel.ToolCall) *runtimeevidencemodel.ToolCall {
	cloned := *item
	cloned.Arguments = item.Arguments.Clone()
	cloned.Output = item.Output.Clone()
	return &cloned
}

func cloneValidationResult(item *runtimeevidencemodel.ValidationResult) *runtimeevidencemodel.ValidationResult {
	cloned := *item
	cloned.EvidenceTags = cloneStringSlice(item.EvidenceTags)
	return &cloned
}

func cloneRuntimeCheckpoint(item *run.RuntimeCheckpoint) *run.RuntimeCheckpoint {
	cloned := *item
	cloned.Data = item.Data.Clone()
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
	if item.RuntimeCloseout != nil {
		closeout := item.RuntimeCloseout.CloneRaw()
		cloned.RuntimeCloseout = &closeout
	}
	if item.TerminalDelivery != nil {
		identity := *item.TerminalDelivery
		cloned.TerminalDelivery = &identity
	}
	return &cloned
}

func cloneResultSummary(item workorchresultcontract.ResultSummary) workorchresultcontract.ResultSummary {
	cloned := item
	cloned.EvidenceRefs = cloneStringSlice(item.EvidenceRefs)
	if item.ArtifactRefs != nil {
		cloned.ArtifactRefs = append([]common.ArtifactID(nil), item.ArtifactRefs...)
	}
	cloned.KnownLimitations = cloneStringSlice(item.KnownLimitations)
	cloned.ResidualRisks = cloneStringSlice(item.ResidualRisks)
	cloned.NextActions = cloneStringSlice(item.NextActions)
	if item.DoDItems != nil {
		cloned.DoDItems = make([]workorchresultcontract.DoDAssessmentItem, 0, len(item.DoDItems))
		for _, dodItem := range item.DoDItems {
			dodItem.EvidenceRefs = cloneStringSlice(dodItem.EvidenceRefs)
			cloned.DoDItems = append(cloned.DoDItems, dodItem)
		}
	}
	cloned.Diagnostics = item.Diagnostics.Clone()
	return cloned
}

func cloneResultDoDAssessment(item workorchresultcontract.DoDAssessment) workorchresultcontract.DoDAssessment {
	cloned := item
	if item.Items != nil {
		cloned.Items = make([]workorchresultcontract.DoDAssessmentItem, 0, len(item.Items))
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

func cloneResultDoDCoverage(item *workorchresultcontract.DoDCoverage) *workorchresultcontract.DoDCoverage {
	if item == nil {
		return nil
	}
	cloned := *item
	if item.Items != nil {
		cloned.Items = make([]workorchresultcontract.DoDCoverageItem, 0, len(item.Items))
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
	cloned.Metadata = item.Metadata.Clone()
	return &cloned
}

func cloneEventRecord(item *eventstream.EventRecord) *eventstream.EventRecord {
	cloned := *item
	cloned.Payload = item.Payload.Clone()
	cloned.PayloadSummary = item.PayloadSummary.Clone()
	return &cloned
}

func cloneDecisionTicket(item *decision.Ticket) *decision.Ticket {
	return decision.CloneTicket(item)
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
	return soul.CloneSoul(item)
}

func cloneWorkspaceTemplate(item *workspace.WorkspaceTemplate) *workspace.WorkspaceTemplate {
	cloned := *item
	cloned.DoDItems = cloneStringSlice(item.DoDItems)
	if item.SoulBindings != nil {
		cloned.SoulBindings = map[string]string{}
		for key, value := range item.SoulBindings {
			cloned.SoulBindings[key] = value
		}
	}
	cloned.ContextHints = item.ContextHints.Clone()
	cloned.EvidencePolicies = workorcheventpayload.CloneSlice(item.EvidencePolicies)
	return &cloned
}

func cloneMemoryItem(item *memorymodel.MemoryItem) *memorymodel.MemoryItem {
	cloned := *item
	cloned.Tags = cloneStringSlice(item.Tags)
	cloned.Vector = cloneFloatSlice(item.Vector)
	return &cloned
}

func cloneMemoryRecallTrace(item *memorymodel.MemoryRecallTrace) *memorymodel.MemoryRecallTrace {
	return memorymodel.CloneMemoryRecallTrace(item)
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
	cloned.Schema = item.Schema.Clone()
	return &cloned
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

func listEventRecords(items map[string]*eventstream.EventRecord, query workorchquerycontract.EventRecordQuery) ([]*eventstream.EventRecord, error) {
	ret := make([]*eventstream.EventRecord, 0)
	cursorMatched := query.CursorAfterID == ""
	for _, item := range items {
		matches, err := eventRecordMatches(item, query)
		if err != nil {
			return nil, err
		}
		if !matches {
			continue
		}
		ret = append(ret, cloneEventRecord(item))
	}
	repository.SortEventRecords(ret, query)
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
		if query.CursorAfterID == "" {
			ret = ret[len(ret)-query.Limit:]
		} else {
			ret = ret[:query.Limit]
		}
	}
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func stripEventRecordPayload(item *eventstream.EventRecord) {
	if item == nil || len(item.Payload.Fields) == 0 {
		return
	}
	if len(item.PayloadSummary.Fields) == 0 {
		item.PayloadSummary = workorcheventpayload.New(workorcheventpayload.IntField("fieldCount", int64(len(item.Payload.Fields))))
	}
	item.Payload = workorcheventpayload.Payload{}
}

func eventRecordMatches(item *eventstream.EventRecord, query workorchquerycontract.EventRecordQuery) (bool, error) {
	if item == nil {
		return false, nil
	}
	if err := validatePersistedEventOccurredAt(item); err != nil {
		return false, err
	}
	canonicalOccurredAt := item.OccurredAt
	if query.WorkspaceID != "" && item.WorkspaceID != query.WorkspaceID {
		return false, nil
	}
	if query.RunID != "" && item.RunID != query.RunID {
		return false, nil
	}
	if query.Topic != "" && item.Topic != query.Topic {
		return false, nil
	}
	for _, topic := range query.ExcludeTopics {
		if strings.TrimSpace(topic) != "" && item.Topic == strings.TrimSpace(topic) {
			return false, nil
		}
	}
	if query.SubjectType != "" && item.SubjectType != query.SubjectType {
		return false, nil
	}
	if query.SubjectID != "" && item.SubjectID != query.SubjectID {
		return false, nil
	}
	if query.OccurredAfter != "" || query.OccurredBefore != "" {
		occurredAt, err := time.Parse(time.RFC3339Nano, canonicalOccurredAt)
		if err != nil {
			return false, fmt.Errorf("%w: persisted event record %q has invalid occurredAt", common.ErrConflict, item.ID)
		}
		if query.OccurredAfter != "" {
			after, err := time.Parse(time.RFC3339Nano, query.OccurredAfter)
			if err != nil || !occurredAt.After(after) {
				return false, nil
			}
		}
		if query.OccurredBefore != "" {
			before, err := time.Parse(time.RFC3339Nano, query.OccurredBefore)
			if err != nil || !occurredAt.Before(before) {
				return false, nil
			}
		}
	}
	return true, nil
}

func validatePersistedEventOccurredAt(item *eventstream.EventRecord) error {
	if item == nil {
		return fmt.Errorf("%w: persisted event record is nil", common.ErrConflict)
	}
	canonicalOccurredAt, err := workorcheventrecordcontract.CanonicalOccurredAt(item.OccurredAt)
	if err != nil || canonicalOccurredAt != item.OccurredAt {
		return fmt.Errorf("%w: persisted event record %q has invalid occurredAt", common.ErrConflict, item.ID)
	}
	return nil
}

func memoryItemMatches(item *memorymodel.MemoryItem, query workorchquerycontract.MemorySearchQuery) bool {
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

func memoryRecallTraceMatches(item *memorymodel.MemoryRecallTrace, query workorchquerycontract.MemoryRecallTraceQuery) bool {
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

func pageMemoryRecallTraces(items []*memorymodel.MemoryRecallTrace, cursor common.MemoryRecallTraceID) ([]*memorymodel.MemoryRecallTrace, error) {
	if cursor == "" {
		return items, nil
	}
	for idx, item := range items {
		if item != nil && item.ID == cursor {
			return items[idx+1:], nil
		}
	}
	return nil, common.ErrInvalidArgument
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

func clonePatch(item *runtimeevidencemodel.Patch) runtimeevidencemodel.Patch {
	cloned := *item
	cloned.CoverageTargets = append([]runtimeevidencemodel.PatchCoverageTarget(nil), item.CoverageTargets...)
	cloned.AffectedFiles = cloneStringSlice(item.AffectedFiles)
	cloned.RiskReasons = cloneStringSlice(item.RiskReasons)
	cloned.Diagnostics = cloneEventPayloads(item.Diagnostics)
	return cloned
}

func cloneEventPayloads(input []workorcheventpayload.Payload) []workorcheventpayload.Payload {
	if len(input) == 0 {
		return nil
	}
	ret := make([]workorcheventpayload.Payload, 0, len(input))
	for _, item := range input {
		ret = append(ret, item.Clone())
	}
	return ret
}

func cloneApproval(item *approval.Approval) approval.Approval {
	return *approval.CloneApproval(item)
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

const stepSummarySchemaVersion = "workorch.step_summary"

func compactStepSummaryInput(input map[string]any) map[string]any {
	ret := map[string]any{
		"_summarySchemaVersion": stepSummarySchemaVersion,
	}
	for _, key := range stepSummaryInputKeys {
		if value, ok := input[key]; ok {
			if compactValue, ok := compactSummaryValue(value, 0); ok {
				ret[key] = compactValue
			}
		}
	}
	return ret
}

func compactStepSummaryOutput(output map[string]any) map[string]any {
	ret := map[string]any{}
	for _, key := range stepSummaryOutputKeys {
		if value, ok := output[key]; ok {
			if compactValue, ok := compactSummaryValue(value, 0); ok {
				ret[key] = compactValue
			}
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

var stepSummaryInputKeys = []string{
	"workspaceId",
	"previousStepId",
	"previousStepKind",
	"graphNodeId",
	"graphRootStepId",
	"graphPreviousNodeId",
	"graphFollowUp",
	"completionFollowUp",
	"graphExpansionLimitFollowUp",
	"runtimeRepairFollowUp",
	"runtimeRepairAttempt",
	"repairOfStepId",
	"repairOfRootStepId",
	"repairOfFailureClass",
	"repairOfFailureSignature",
	"providerRetryFollowUp",
	"providerRetryAttempt",
	"providerRetryRootStepId",
	"recoveryAction",
	"recoveryRootStepId",
	"recoveryLineageType",
	"recoveryBudgetScope",
	"failedStepId",
	"failedStepKind",
	"failedStepState",
	"failureClass",
	"failureSignature",
	"failureTargetScope",
	"runtimeStage",
	"repairObjective",
	"stateChanging",
	"validationEvidence",
	"assessmentOnly",
	"noActionableContinuationFollowUp",
	"fromStaleRepairEvidence",
	"coverageTargets",
	"coverageTargetSet",
}

var stepSummaryOutputKeys = []string{
	"status",
	"stage",
	"summary",
	"error",
	"reason",
	"failureClass",
	"failureSignature",
	"ignoredExecutionGraphReason",
	"command",
	"args",
	"exitCode",
	"operation",
	"stateChanging",
	"validationEvidence",
	"zeroWork",
	"evidenceQuality",
	"nextStep",
	"nextKind",
	"graphNodeId",
	"graphRootStepId",
	"runtimeDiagnostics",
	"lastFailureDiagnostics",
	"streamDiagnostics",
	"patches",
	"coverageTargets",
	"artifactContinuationApply",
	"llmDecision",
}

func compactSummaryValue(value any, depth int) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, true
	case string:
		if len(typed) > 1024 {
			return typed[:1024], true
		}
		return typed, true
	case []string:
		if len(typed) > 8 {
			typed = typed[:8]
		}
		return append([]string(nil), typed...), true
	case map[string]any:
		if depth > 1 {
			return nil, false
		}
		return compactSummaryObject(typed, depth+1), true
	case []map[string]any:
		if depth > 1 {
			return nil, false
		}
		limit := min(len(typed), 8)
		out := make([]map[string]any, 0, limit)
		for _, item := range typed[:limit] {
			if compactItem := compactSummaryObject(item, depth+1); len(compactItem) > 0 {
				out = append(out, compactItem)
			}
		}
		return out, true
	case []any:
		if depth > 1 {
			return nil, false
		}
		limit := min(len(typed), 8)
		out := make([]any, 0, limit)
		for _, item := range typed[:limit] {
			if compactItem, ok := compactSummaryValue(item, depth+1); ok {
				out = append(out, compactItem)
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func compactSummaryObject(input map[string]any, depth int) map[string]any {
	if len(input) == 0 || depth > 2 {
		return nil
	}
	out := make(map[string]any, min(len(input), 24))
	count := 0
	for _, key := range stepSummaryNestedKeys {
		if value, ok := input[key]; ok {
			if compactValue, ok := compactSummaryValue(value, depth+1); ok {
				out[key] = compactValue
				count++
				if count >= 24 {
					break
				}
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

var stepSummaryNestedKeys = []string{
	"id",
	"workspaceId",
	"runId",
	"item",
	"kind",
	"state",
	"source",
	"sourceStepId",
	"stepId",
	"summary",
	"status",
	"stage",
	"error",
	"reason",
	"class",
	"failureClass",
	"failureSignature",
	"violatedRule",
	"correctionProfileId",
	"budgetScope",
	"lineageType",
	"rootStepId",
	"focusId",
	"attempt",
	"maxAttempts",
	"recoverability",
	"violationType",
	"signature",
	"targetRef",
	"failureTargets",
	"conflictRef",
	"serverId",
	"tool",
	"uri",
	"node",
	"operation",
	"command",
	"exitCode",
	"stateChanging",
	"zeroWork",
	"evidenceQuality",
	"coverageTargets",
	"patchType",
	"diff",
	"matchedTerms",
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
