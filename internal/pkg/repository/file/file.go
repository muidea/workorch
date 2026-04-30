package file

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
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
	mu   sync.RWMutex
	path string
}

type WorkspaceTemplateRepository struct {
	mu   sync.RWMutex
	path string
}

type SoulRepository struct {
	mu   sync.RWMutex
	path string
}

const (
	soulDefinitionFile = "soul.md"
	soulMetadataFile   = "metadata.json"
)

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

type MCPCapabilityRepository struct {
	mu   sync.RWMutex
	path string
}

type ResultRepository struct {
	mu   sync.RWMutex
	path string
}

type ArtifactRepository struct {
	mu   sync.RWMutex
	path string
}

type EventRecordRepository struct {
	mu   sync.RWMutex
	path string
}

func NewSoulRepository(path string) *SoulRepository {
	return &SoulRepository{path: path}
}

func (r *SoulRepository) Create(_ context.Context, item *soul.Soul) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dir, err := r.soulDir(item.ID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, soulMetadataFile)); err == nil {
		return common.ErrConflict
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return r.saveSoul(dir, item)
}

func (r *SoulRepository) Get(_ context.Context, id common.SoulID) (*soul.Soul, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dir, err := r.soulDir(id)
	if err != nil {
		return nil, err
	}
	return r.loadSoul(dir, id)
}

func (r *SoulRepository) List(_ context.Context) ([]*soul.Soul, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries, err := os.ReadDir(r.soulRoot())
	if err != nil && os.IsNotExist(err) {
		return []*soul.Soul{}, nil
	}
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			ids = append(ids, entry.Name())
		}
	}
	sort.Strings(ids)

	ret := make([]*soul.Soul, 0, len(ids))
	for _, id := range ids {
		item, err := r.loadSoul(filepath.Join(r.soulRoot(), id), common.SoulID(id))
		if err != nil {
			return nil, err
		}
		ret = append(ret, item)
	}
	return ret, nil
}

func (r *SoulRepository) Update(_ context.Context, item *soul.Soul) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dir, err := r.soulDir(item.ID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, soulMetadataFile)); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return r.saveSoul(dir, item)
}

func (r *SoulRepository) Delete(_ context.Context, id common.SoulID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dir, err := r.soulDir(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, soulMetadataFile)); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func (r *SoulRepository) soulRoot() string {
	return r.path
}

func (r *SoulRepository) soulDir(id common.SoulID) (string, error) {
	value := strings.TrimSpace(string(id))
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return "", common.ErrInvalidArgument
	}
	return filepath.Join(r.soulRoot(), value), nil
}

func (r *SoulRepository) loadSoul(dir string, id common.SoulID) (*soul.Soul, error) {
	data, err := os.ReadFile(filepath.Join(dir, soulMetadataFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, common.ErrNotFound
		}
		return nil, err
	}
	var item soul.Soul
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, err
	}
	if item.ID == "" {
		item.ID = id
	}
	content, err := os.ReadFile(filepath.Join(dir, soulDefinitionFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, common.ErrNotFound
		}
		return nil, err
	}
	item.SoulMD = string(content)
	return cloneSoul(&item), nil
}

func (r *SoulRepository) saveSoul(dir string, item *soul.Soul) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cloned := cloneSoul(item)
	soulMD := cloned.SoulMD
	cloned.SoulMD = ""
	data, err := json.MarshalIndent(cloned, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, soulMetadataFile), data, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, soulDefinitionFile), []byte(strings.TrimSpace(soulMD)+"\n"), 0o644)
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
		cloned := cloneMemoryItem(&item)
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

func (r *MemoryItemRepository) Delete(_ context.Context, id common.MemoryID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.MemoryID, memorymodel.MemoryItem](r.path, id)
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

func NewMCPCapabilityRepository(path string) *MCPCapabilityRepository {
	return &MCPCapabilityRepository{path: path}
}

func (r *MCPCapabilityRepository) Create(_ context.Context, item *mcpmodel.Capability) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.MCPCapabilityID, mcpmodel.Capability](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneMCPCapability(item)
	return saveMap(r.path, items)
}

func (r *MCPCapabilityRepository) Get(_ context.Context, id common.MCPCapabilityID) (*mcpmodel.Capability, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.MCPCapabilityID, mcpmodel.Capability](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneMCPCapability(&item), nil
}

func (r *MCPCapabilityRepository) List(_ context.Context) ([]*mcpmodel.Capability, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.MCPCapabilityID, mcpmodel.Capability](r.path)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	ret := make([]*mcpmodel.Capability, 0, len(ids))
	for _, id := range ids {
		item := items[common.MCPCapabilityID(id)]
		ret = append(ret, cloneMCPCapability(&item))
	}
	return ret, nil
}

func (r *MCPCapabilityRepository) Update(_ context.Context, item *mcpmodel.Capability) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.MCPCapabilityID, mcpmodel.Capability](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneMCPCapability(item)
	return saveMap(r.path, items)
}

func NewResultRepository(path string) *ResultRepository {
	return &ResultRepository{path: path}
}

func (r *ResultRepository) Create(_ context.Context, item *resultmodel.Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.ResultID, resultmodel.Result](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneResult(item)
	return saveMap(r.path, items)
}

func (r *ResultRepository) Get(_ context.Context, id common.ResultID) (*resultmodel.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ResultID, resultmodel.Result](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneResult(&item), nil
}

func (r *ResultRepository) ListByWorkspace(_ context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ResultID, resultmodel.Result](r.path)
	if err != nil {
		return nil, err
	}
	return listResults(items, func(item resultmodel.Result) bool {
		return item.WorkspaceID == workspaceID
	}), nil
}

func (r *ResultRepository) ListByRun(_ context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ResultID, resultmodel.Result](r.path)
	if err != nil {
		return nil, err
	}
	return listResults(items, func(item resultmodel.Result) bool {
		return item.RunID == runID
	}), nil
}

func (r *ResultRepository) Update(_ context.Context, item *resultmodel.Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.ResultID, resultmodel.Result](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneResult(item)
	return saveMap(r.path, items)
}

func (r *ResultRepository) Delete(_ context.Context, id common.ResultID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.ResultID, resultmodel.Result](r.path, id)
}

func NewArtifactRepository(path string) *ArtifactRepository {
	return &ArtifactRepository{path: path}
}

func (r *ArtifactRepository) Create(_ context.Context, item *resultmodel.Artifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.ArtifactID, resultmodel.Artifact](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneArtifact(item)
	return saveMap(r.path, items)
}

func (r *ArtifactRepository) Get(_ context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ArtifactID, resultmodel.Artifact](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneArtifact(&item), nil
}

func (r *ArtifactRepository) ListByWorkspace(_ context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ArtifactID, resultmodel.Artifact](r.path)
	if err != nil {
		return nil, err
	}
	return listArtifacts(items, func(item resultmodel.Artifact) bool {
		return item.WorkspaceID == workspaceID
	}), nil
}

func (r *ArtifactRepository) ListByRun(_ context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ArtifactID, resultmodel.Artifact](r.path)
	if err != nil {
		return nil, err
	}
	return listArtifacts(items, func(item resultmodel.Artifact) bool {
		return item.RunID == runID
	}), nil
}

func (r *ArtifactRepository) ListByResult(_ context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.ArtifactID, resultmodel.Artifact](r.path)
	if err != nil {
		return nil, err
	}
	return listArtifacts(items, func(item resultmodel.Artifact) bool {
		return item.ResultID == resultID
	}), nil
}

func (r *ArtifactRepository) Update(_ context.Context, item *resultmodel.Artifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.ArtifactID, resultmodel.Artifact](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneArtifact(item)
	return saveMap(r.path, items)
}

func (r *ArtifactRepository) Delete(_ context.Context, id common.ArtifactID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.ArtifactID, resultmodel.Artifact](r.path, id)
}

func NewEventRecordRepository(path string) *EventRecordRepository {
	return &EventRecordRepository{path: path}
}

func (r *EventRecordRepository) Append(_ context.Context, item *eventstream.EventRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.EventRecordID, eventstream.EventRecord](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneEventRecord(item)
	return saveMap(r.path, items)
}

func (r *EventRecordRepository) Get(_ context.Context, id common.EventRecordID) (*eventstream.EventRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.EventRecordID, eventstream.EventRecord](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneEventRecord(&item), nil
}

func (r *EventRecordRepository) List(_ context.Context, query repository.EventRecordQuery) ([]*eventstream.EventRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.EventRecordID, eventstream.EventRecord](r.path)
	if err != nil {
		return nil, err
	}
	return listEventRecords(items, query), nil
}

func (r *EventRecordRepository) Delete(_ context.Context, id common.EventRecordID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.EventRecordID, eventstream.EventRecord](r.path, id)
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

func (r *WorkspaceRepository) Delete(_ context.Context, id common.WorkspaceID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.WorkspaceID, workspace.Workspace](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[id]; !ok {
		return common.ErrNotFound
	}
	delete(items, id)
	return saveMap(r.path, items)
}

func NewWorkspaceTemplateRepository(path string) *WorkspaceTemplateRepository {
	return &WorkspaceTemplateRepository{path: path}
}

func (r *WorkspaceTemplateRepository) Create(_ context.Context, item *repository.WorkspaceTemplate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[string, repository.WorkspaceTemplate](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneWorkspaceTemplate(item)
	return saveMap(r.path, items)
}

func (r *WorkspaceTemplateRepository) Get(_ context.Context, id string) (*repository.WorkspaceTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[string, repository.WorkspaceTemplate](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneWorkspaceTemplate(&item), nil
}

func (r *WorkspaceTemplateRepository) List(_ context.Context) ([]*repository.WorkspaceTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[string, repository.WorkspaceTemplate](r.path)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	ret := make([]*repository.WorkspaceTemplate, 0, len(ids))
	for _, id := range ids {
		item := items[id]
		ret = append(ret, cloneWorkspaceTemplate(&item))
	}
	return ret, nil
}

func (r *WorkspaceTemplateRepository) Update(_ context.Context, item *repository.WorkspaceTemplate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[string, repository.WorkspaceTemplate](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneWorkspaceTemplate(item)
	return saveMap(r.path, items)
}

func (r *WorkspaceTemplateRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[string, repository.WorkspaceTemplate](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[id]; !ok {
		return common.ErrNotFound
	}
	delete(items, id)
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

func (r *GoalRepository) Update(_ context.Context, item *workspace.Goal) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.GoalID, workspace.Goal](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; !ok {
		return common.ErrNotFound
	}
	items[item.ID] = *cloneGoal(item)
	return saveMap(r.path, items)
}

func (r *GoalRepository) Delete(_ context.Context, id common.GoalID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.GoalID, workspace.Goal](r.path, id)
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

func (r *DoDRepository) Delete(_ context.Context, id common.DoDID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.DoDID, workspace.DoD](r.path, id)
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

func (r *RunRepository) Delete(_ context.Context, id common.RunID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.RunID, run.Run](r.path, id)
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

func (r *StepRepository) Delete(_ context.Context, id common.StepID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.StepID, run.Step](r.path, id)
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

func (r *ToolCallRepository) Delete(_ context.Context, id common.ToolCallID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.ToolCallID, run.ToolCall](r.path, id)
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

func (r *PatchRepository) Delete(_ context.Context, id common.PatchID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.PatchID, run.Patch](r.path, id)
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

func (r *ValidationResultRepository) Delete(_ context.Context, id common.ValidationResultID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.ValidationResultID, run.ValidationResult](r.path, id)
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

func (r *RuntimeCheckpointRepository) Delete(_ context.Context, id common.CheckpointID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.CheckpointID, run.RuntimeCheckpoint](r.path, id)
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

func (r *DecisionTicketRepository) Delete(_ context.Context, id common.TicketID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.TicketID, decision.Ticket](r.path, id)
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

func (r *DecisionResolutionRepository) DeleteByTicketID(_ context.Context, ticketID common.TicketID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.TicketID, decision.Resolution](r.path, ticketID)
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

func (r *ApprovalRepository) Delete(_ context.Context, id common.ApprovalID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.ApprovalID, approval.Approval](r.path, id)
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

func deleteMapItem[K comparable, V any](path string, id K) error {
	items, err := loadMap[K, V](path)
	if err != nil {
		return err
	}
	if _, ok := items[id]; !ok {
		return common.ErrNotFound
	}
	delete(items, id)
	return saveMap(path, items)
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
	if item.SoulBindings != nil {
		cloned.SoulBindings = map[string]common.SoulID{}
		for key, value := range item.SoulBindings {
			cloned.SoulBindings[key] = value
		}
	}
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

func cloneResult(item *resultmodel.Result) *resultmodel.Result {
	cloned := *item
	cloned.EvidenceRefs = cloneStringSlice(item.EvidenceRefs)
	if item.ArtifactRefs != nil {
		cloned.ArtifactRefs = append([]common.ArtifactID(nil), item.ArtifactRefs...)
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
	cloned.Schema = cloneMap(item.Schema)
	return &cloned
}

func listResults(items map[common.ResultID]resultmodel.Result, matches func(resultmodel.Result) bool) []*resultmodel.Result {
	ret := make([]*resultmodel.Result, 0)
	for _, item := range items {
		if !matches(item) {
			continue
		}
		ret = append(ret, cloneResult(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func listArtifacts(items map[common.ArtifactID]resultmodel.Artifact, matches func(resultmodel.Artifact) bool) []*resultmodel.Artifact {
	ret := make([]*resultmodel.Artifact, 0)
	for _, item := range items {
		if !matches(item) {
			continue
		}
		ret = append(ret, cloneArtifact(&item))
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func listEventRecords(items map[common.EventRecordID]eventstream.EventRecord, query repository.EventRecordQuery) []*eventstream.EventRecord {
	ret := make([]*eventstream.EventRecord, 0)
	for _, item := range items {
		if !eventRecordMatches(&item, query) {
			continue
		}
		ret = append(ret, cloneEventRecord(&item))
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].OccurredAt == ret[j].OccurredAt {
			return string(ret[i].ID) < string(ret[j].ID)
		}
		return ret[i].OccurredAt < ret[j].OccurredAt
	})
	if query.CursorAfterID != "" {
		cursorMatched := false
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
