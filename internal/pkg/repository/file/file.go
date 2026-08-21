package file

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/runtimepath"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
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

type MemoryRecallTraceRepository struct {
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

const (
	eventRecordPayloadInlineThreshold = 8 * 1024
	eventRecordSegmentMaxBytes        = 8 * 1024 * 1024
	eventRecordSegmentPrefix          = "events-"
	eventRecordSegmentSuffix          = ".jsonl"
	eventRecordPayloadRefKey          = "payloadRef"
)

type eventRecordLogLine struct {
	Record     eventstream.EventRecord `json:"record"`
	PayloadRef string                  `json:"payloadRef,omitempty"`
}

type eventRecordStreamPositionState struct {
	Position int64 `json:"position"`
}

type mapFileCacheEntry struct {
	mu     sync.RWMutex
	loaded bool
	items  any
}

var mapFileCache sync.Map

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

func (r *MemoryItemRepository) Search(_ context.Context, query workorchquerycontract.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
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

func NewMemoryRecallTraceRepository(path string) *MemoryRecallTraceRepository {
	return &MemoryRecallTraceRepository{path: path}
}

func (r *MemoryRecallTraceRepository) Create(_ context.Context, item *memorymodel.MemoryRecallTrace) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.MemoryRecallTraceID, memorymodel.MemoryRecallTrace](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneMemoryRecallTrace(item)
	return saveMap(r.path, items)
}

func (r *MemoryRecallTraceRepository) Upsert(_ context.Context, item *memorymodel.MemoryRecallTrace) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[common.MemoryRecallTraceID, memorymodel.MemoryRecallTrace](r.path)
	if err != nil {
		return err
	}
	items[item.ID] = *cloneMemoryRecallTrace(item)
	return saveMap(r.path, items)
}

func (r *MemoryRecallTraceRepository) Get(_ context.Context, id common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.MemoryRecallTraceID, memorymodel.MemoryRecallTrace](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneMemoryRecallTrace(&item), nil
}

func (r *MemoryRecallTraceRepository) List(_ context.Context, query workorchquerycontract.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.MemoryRecallTraceID, memorymodel.MemoryRecallTrace](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*memorymodel.MemoryRecallTrace, 0, len(items))
	for _, item := range items {
		if !memoryRecallTraceMatches(&item, query) {
			continue
		}
		ret = append(ret, cloneMemoryRecallTrace(&item))
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

func (r *MemoryRecallTraceRepository) Delete(_ context.Context, id common.MemoryRecallTraceID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.MemoryRecallTraceID, memorymodel.MemoryRecallTrace](r.path, id)
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

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := r.resultPath(item.ID)
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if item.Revision == 0 {
		item.Revision = 1
	}
	return writeJSONFileAtomic(path, cloneResult(item))
}

func (r *ResultRepository) Get(_ context.Context, id common.ResultID) (*resultmodel.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, err := readJSONItemFile[resultmodel.Result](r.resultPath(id))
	if err != nil {
		return nil, err
	}
	return cloneResult(&item), nil
}

func (r *ResultRepository) ListByWorkspace(_ context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.list(func(item resultmodel.Result) bool {
		return item.WorkspaceID == workspaceID
	})
}

func (r *ResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := r.list(func(item resultmodel.Result) bool {
		return item.RunID == runID && itemMatchesRuntimeWorkspace(ctx, item.WorkspaceID)
	})
	if err != nil {
		return nil, err
	}
	if common.RuntimeWorkspaceIDFromContext(ctx) == "" {
		if conflict := listHasMultipleRunWorkspaces(items, func(item *resultmodel.Result) common.WorkspaceID {
			return item.WorkspaceID
		}); conflict {
			return nil, common.ErrConflict
		}
	}
	return items, nil
}

func (r *ResultRepository) ListForRecovery(_ context.Context, cursor string, limit int) ([]*resultmodel.Result, string, error) {
	if limit < 1 {
		return nil, "", common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items, err := r.list(nil)
	if err != nil {
		return nil, "", err
	}
	filtered := items[:0]
	for _, item := range items {
		if resultRecoveryKey(item) > cursor {
			filtered = append(filtered, item)
		}
	}
	items = filtered
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

func (r *ResultRepository) Update(_ context.Context, item *resultmodel.Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := r.resultPath(item.ID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return writeJSONFileAtomic(path, cloneResult(item))
}

func (r *ResultRepository) CompareAndSwap(_ context.Context, item *resultmodel.Result, expectedRevision uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := r.resultPath(item.ID)
	current, err := readJSONItemFile[resultmodel.Result](path)
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return common.ErrConflict
	}
	item.Revision = expectedRevision + 1
	return writeJSONFileAtomic(path, cloneResult(item))
}

func (r *ResultRepository) Delete(_ context.Context, id common.ResultID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	path := r.resultPath(id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return os.Remove(path)
}

func (r *ResultRepository) resultRoot() string {
	return repositoryItemRoot(r.path, "result.json", "results")
}

func (r *ResultRepository) resultPath(id common.ResultID) string {
	return repositoryItemPath(r.resultRoot(), string(id))
}

func (r *ResultRepository) list(matches func(resultmodel.Result) bool) ([]*resultmodel.Result, error) {
	paths, err := repositoryItemPaths(r.resultRoot())
	if err != nil {
		return nil, err
	}
	ret := make([]*resultmodel.Result, 0, len(paths))
	for _, path := range paths {
		item, err := readJSONItemFile[resultmodel.Result](path)
		if err != nil {
			return nil, err
		}
		if matches == nil || matches(item) {
			ret = append(ret, cloneResult(&item))
		}
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func NewArtifactRepository(path string) *ArtifactRepository {
	return &ArtifactRepository{path: path}
}

func (r *ArtifactRepository) Create(_ context.Context, item *resultmodel.Artifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := r.artifactPath(item.ID)
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return writeJSONFileAtomic(path, cloneArtifact(item))
}

func (r *ArtifactRepository) Get(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, err := readJSONItemFile[resultmodel.Artifact](r.artifactPath(id))
	if err != nil {
		return nil, err
	}
	if !itemMatchesRuntimeWorkspace(ctx, item.WorkspaceID) {
		return nil, common.ErrNotFound
	}
	return cloneArtifact(&item), nil
}

func (r *ArtifactRepository) ListByWorkspace(_ context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.list(func(item resultmodel.Artifact) bool {
		return item.WorkspaceID == workspaceID
	})
}

func (r *ArtifactRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := r.list(func(item resultmodel.Artifact) bool {
		return item.RunID == runID && itemMatchesRuntimeWorkspace(ctx, item.WorkspaceID)
	})
	if err != nil {
		return nil, err
	}
	if common.RuntimeWorkspaceIDFromContext(ctx) == "" {
		if conflict := listHasMultipleRunWorkspaces(items, func(item *resultmodel.Artifact) common.WorkspaceID {
			return item.WorkspaceID
		}); conflict {
			return nil, common.ErrConflict
		}
	}
	return items, nil
}

func (r *ArtifactRepository) ListByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := r.list(func(item resultmodel.Artifact) bool {
		return item.ResultID == resultID && itemMatchesRuntimeWorkspace(ctx, item.WorkspaceID)
	})
	if err != nil {
		return nil, err
	}
	if common.RuntimeWorkspaceIDFromContext(ctx) == "" && listHasMultipleRunWorkspaces(items, func(item *resultmodel.Artifact) common.WorkspaceID {
		return item.WorkspaceID
	}) {
		return nil, common.ErrConflict
	}
	return items, nil
}

func (r *ArtifactRepository) Update(ctx context.Context, item *resultmodel.Artifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := r.artifactPath(item.ID)
	current, err := readJSONItemFile[resultmodel.Artifact](path)
	if err != nil {
		return err
	}
	if !itemMatchesRuntimeWorkspace(ctx, current.WorkspaceID) || current.WorkspaceID != item.WorkspaceID {
		return common.ErrNotFound
	}
	return writeJSONFileAtomic(path, cloneArtifact(item))
}

func (r *ArtifactRepository) Delete(ctx context.Context, id common.ArtifactID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	path := r.artifactPath(id)
	current, err := readJSONItemFile[resultmodel.Artifact](path)
	if err != nil {
		return err
	}
	if !itemMatchesRuntimeWorkspace(ctx, current.WorkspaceID) {
		return common.ErrNotFound
	}
	return os.Remove(path)
}

func (r *ArtifactRepository) artifactRoot() string {
	return repositoryItemRoot(r.path, "artifact.json", "artifacts")
}

func (r *ArtifactRepository) artifactPath(id common.ArtifactID) string {
	return repositoryItemPath(r.artifactRoot(), string(id))
}

func (r *ArtifactRepository) list(matches func(resultmodel.Artifact) bool) ([]*resultmodel.Artifact, error) {
	paths, err := repositoryItemPaths(r.artifactRoot())
	if err != nil {
		return nil, err
	}
	ret := make([]*resultmodel.Artifact, 0, len(paths))
	for _, path := range paths {
		item, err := readJSONItemFile[resultmodel.Artifact](path)
		if err != nil {
			return nil, err
		}
		if matches == nil || matches(item) {
			ret = append(ret, cloneArtifact(&item))
		}
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func NewEventRecordRepository(path string) *EventRecordRepository {
	return &EventRecordRepository{path: path}
}

func (r *EventRecordRepository) Append(_ context.Context, item *eventstream.EventRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if err := r.observeEventRecordStreamPosition(item.StreamPosition); err != nil {
		return err
	}
	root := r.eventLogRoot()
	idPath := filepath.Join(root, "ids", safeEventRecordFileName(string(item.ID))+".marker")
	if err := os.MkdirAll(filepath.Dir(idPath), 0o755); err != nil {
		return err
	}
	marker, err := os.OpenFile(idPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return common.ErrConflict
		}
		return err
	}
	if closeErr := marker.Close(); closeErr != nil {
		return closeErr
	}
	if err := r.removeDeletedMarker(item.ID); err != nil {
		_ = os.Remove(idPath)
		return err
	}
	if err := r.appendLogLine(root, item); err != nil {
		_ = os.Remove(idPath)
		return err
	}
	return nil
}

func (r *EventRecordRepository) ReserveEventRecordStreamPosition(_ context.Context) (int64, error) {
	if r == nil {
		return 0, common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.eventRecordStreamPositionState()
	if err != nil {
		return 0, err
	}
	if state.Position == int64(^uint64(0)>>1) {
		return 0, common.ErrConflict
	}
	state.Position++
	if err := writeJSONFileAtomic(r.eventRecordStreamPositionPath(), state); err != nil {
		return 0, err
	}
	return state.Position, nil
}

// ObserveEventRecordStreamPosition advances the durable floor for records
// appended through a repository decorator that may hide the optional reserve
// capability from Eventstream.
func (r *EventRecordRepository) ObserveEventRecordStreamPosition(position int64) error {
	if r == nil || position < 0 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.observeEventRecordStreamPosition(position)
}

func (r *EventRecordRepository) observeEventRecordStreamPosition(position int64) error {
	if position <= 0 {
		return nil
	}
	state, err := r.eventRecordStreamPositionState()
	if err != nil {
		return err
	}
	if state.Position >= position {
		return nil
	}
	state.Position = position
	return writeJSONFileAtomic(r.eventRecordStreamPositionPath(), state)
}

func (r *EventRecordRepository) eventRecordStreamPositionState() (eventRecordStreamPositionState, error) {
	path := r.eventRecordStreamPositionPath()
	data, err := os.ReadFile(path)
	if err == nil {
		var state eventRecordStreamPositionState
		if unmarshalErr := json.Unmarshal(data, &state); unmarshalErr != nil || state.Position < 0 {
			if unmarshalErr != nil {
				return eventRecordStreamPositionState{}, unmarshalErr
			}
			return eventRecordStreamPositionState{}, common.ErrInvalidArgument
		}
		return state, nil
	}
	if !os.IsNotExist(err) {
		return eventRecordStreamPositionState{}, err
	}
	state := eventRecordStreamPositionState{}
	if scanErr := r.scanEventRecords(func(item *eventstream.EventRecord) error {
		if item != nil && item.StreamPosition > state.Position {
			state.Position = item.StreamPosition
		}
		return nil
	}); scanErr != nil {
		return eventRecordStreamPositionState{}, scanErr
	}
	return state, nil
}

func (r *EventRecordRepository) eventRecordStreamPositionPath() string {
	return filepath.Join(r.eventLogRoot(), ".eventstream-stream-position.json")
}

func (r *EventRecordRepository) Get(_ context.Context, id common.EventRecordID) (*eventstream.EventRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	if r.isEventDeleted(id) {
		return nil, common.ErrNotFound
	}
	var found *eventstream.EventRecord
	if err := r.scanEventRecords(func(item *eventstream.EventRecord) error {
		if item.ID == id {
			found = cloneEventRecord(item)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if found == nil {
		return nil, common.ErrNotFound
	}
	if err := r.hydrateEventRecordPayload(found); err != nil {
		return nil, err
	}
	return found, nil
}

func (r *EventRecordRepository) List(_ context.Context, query workorchquerycontract.EventRecordQuery) ([]*eventstream.EventRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	deleted, err := r.deletedEventIDs()
	if err != nil {
		return nil, err
	}
	ret := make([]*eventstream.EventRecord, 0)
	boundedLatest := query.Limit > 0 && query.CursorAfterID == ""
	if boundedLatest {
		ret, err = r.listLatestEventRecords(query, deleted)
		if err != nil {
			return nil, err
		}
	} else {
		if err := r.scanEventRecords(func(item *eventstream.EventRecord) error {
			if _, ok := deleted[item.ID]; ok {
				return nil
			}
			if !eventRecordMatches(item, query) {
				return nil
			}
			ret = append(ret, item)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	repository.SortEventRecords(ret, query)
	if !boundedLatest {
		ret = applyEventRecordCursorAndLimit(ret, query)
	}
	if len(ret) == 0 {
		return nil, nil
	}
	if query.IncludePayload {
		for _, item := range ret {
			if err := r.hydrateEventRecordPayload(item); err != nil {
				return nil, err
			}
		}
	} else {
		for _, item := range ret {
			stripEventRecordPayload(item)
		}
	}
	return ret, nil
}

func (r *EventRecordRepository) listLatestEventRecords(query workorchquerycontract.EventRecordQuery, deleted map[common.EventRecordID]struct{}) ([]*eventstream.EventRecord, error) {
	if query.Limit <= 0 || query.CursorAfterID != "" {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]*eventstream.EventRecord, 0, query.Limit)
	if err := r.scanEventRecordsReverse(func(item *eventstream.EventRecord) (bool, error) {
		if _, ok := deleted[item.ID]; ok {
			return false, nil
		}
		if !eventRecordMatches(item, query) {
			return false, nil
		}
		ret = append(ret, item)
		return len(ret) >= query.Limit, nil
	}); err != nil {
		return nil, err
	}
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *EventRecordRepository) Delete(_ context.Context, id common.EventRecordID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id == "" {
		return common.ErrInvalidArgument
	}
	exists, err := r.eventExists(id)
	if err != nil {
		return err
	}
	if !exists {
		return common.ErrNotFound
	}
	path := r.deletedMarkerPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(path, []byte(string(id)+"\n"), 0o644); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(r.eventLogRoot(), "ids", safeEventRecordFileName(string(id))+".marker"))
	return nil
}

// PurgeWorkspace compacts the append log so purged records, tombstones and
// external payload files are physically removed instead of merely hidden by
// delete markers.
func (r *EventRecordRepository) PurgeWorkspace(_ context.Context, workspaceID common.WorkspaceID) (int, error) {
	if r == nil || workspaceID == "" {
		return 0, common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	deletedIDs, err := r.deletedEventIDs()
	if err != nil {
		return 0, err
	}
	retained := make([]*eventstream.EventRecord, 0)
	deleted := 0
	matchedPhysicalRecord := false
	if err := r.scanEventRecords(func(item *eventstream.EventRecord) error {
		if item == nil {
			return nil
		}
		_, alreadyDeleted := deletedIDs[item.ID]
		if item.WorkspaceID == workspaceID {
			matchedPhysicalRecord = true
			if !alreadyDeleted {
				deleted++
			}
			return nil
		}
		if alreadyDeleted {
			return nil
		}
		if err := r.hydrateEventRecordPayload(item); err != nil {
			return err
		}
		retained = append(retained, cloneEventRecord(item))
		return nil
	}); err != nil {
		return 0, err
	}
	if !matchedPhysicalRecord {
		return 0, nil
	}

	root := filepath.Clean(r.eventLogRoot())
	absRoot, err := filepath.Abs(root)
	if err != nil || root == "." || root == string(filepath.Separator) || absRoot == string(filepath.Separator) {
		return 0, common.ErrInvalidArgument
	}
	parent := filepath.Dir(absRoot)
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		return 0, err
	}
	temporaryRoot, err := os.MkdirTemp(parent, "."+filepath.Base(absRoot)+".compact-")
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = os.RemoveAll(temporaryRoot)
	}()
	temporaryRepository := NewEventRecordRepository(temporaryRoot)
	for _, item := range retained {
		if err := temporaryRepository.Append(context.Background(), item); err != nil {
			return 0, err
		}
	}

	backupRoot, err := os.MkdirTemp(parent, ".eventstream-purge-backup-")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.RemoveAll(backupRoot) }()
	if err := replaceEventRecordStorage(absRoot, temporaryRoot, backupRoot); err != nil {
		return 0, err
	}
	if err := os.RemoveAll(backupRoot); err != nil {
		return 0, err
	}
	return deleted, nil
}

func replaceEventRecordStorage(root, replacementRoot, backupRoot string) error {
	names := []string{"events", "payloads", "ids", "deleted"}
	movedOld := make([]string, 0, len(names))
	for _, name := range names {
		current := filepath.Join(root, name)
		if _, err := os.Stat(current); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			for idx := len(movedOld) - 1; idx >= 0; idx-- {
				_ = os.Rename(filepath.Join(backupRoot, movedOld[idx]), filepath.Join(root, movedOld[idx]))
			}
			return err
		}
		if err := os.Rename(current, filepath.Join(backupRoot, name)); err != nil {
			for idx := len(movedOld) - 1; idx >= 0; idx-- {
				_ = os.Rename(filepath.Join(backupRoot, movedOld[idx]), filepath.Join(root, movedOld[idx]))
			}
			return err
		}
		movedOld = append(movedOld, name)
	}
	movedNew := make([]string, 0, len(names))
	for _, name := range names {
		replacement := filepath.Join(replacementRoot, name)
		if _, err := os.Stat(replacement); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return restoreEventRecordStorage(root, replacementRoot, backupRoot, movedOld, movedNew, err)
		}
		if err := os.Rename(replacement, filepath.Join(root, name)); err != nil {
			return restoreEventRecordStorage(root, replacementRoot, backupRoot, movedOld, movedNew, err)
		}
		movedNew = append(movedNew, name)
	}
	return nil
}

func restoreEventRecordStorage(root, replacementRoot, backupRoot string, movedOld, movedNew []string, cause error) error {
	for idx := len(movedNew) - 1; idx >= 0; idx-- {
		name := movedNew[idx]
		_ = os.Rename(filepath.Join(root, name), filepath.Join(replacementRoot, name))
	}
	for idx := len(movedOld) - 1; idx >= 0; idx-- {
		name := movedOld[idx]
		_ = os.Rename(filepath.Join(backupRoot, name), filepath.Join(root, name))
	}
	return cause
}

func (r *EventRecordRepository) eventLogRoot() string {
	path := filepath.Clean(strings.TrimSpace(r.path))
	if path == "" || path == "." {
		return "event_record"
	}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return strings.TrimSuffix(path, filepath.Ext(path))
	}
	return path
}

func (r *EventRecordRepository) eventSegmentDir() string {
	return filepath.Join(r.eventLogRoot(), "events")
}

func (r *EventRecordRepository) eventPayloadDir() string {
	return filepath.Join(r.eventLogRoot(), "payloads")
}

func (r *EventRecordRepository) deletedMarkerPath(id common.EventRecordID) string {
	return filepath.Join(r.eventLogRoot(), "deleted", safeEventRecordFileName(string(id))+".marker")
}

func (r *EventRecordRepository) removeDeletedMarker(id common.EventRecordID) error {
	if err := os.Remove(r.deletedMarkerPath(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (r *EventRecordRepository) isEventDeleted(id common.EventRecordID) bool {
	_, err := os.Stat(r.deletedMarkerPath(id))
	return err == nil
}

func (r *EventRecordRepository) deletedEventIDs() (map[common.EventRecordID]struct{}, error) {
	ret := map[common.EventRecordID]struct{}{}
	dir := filepath.Join(r.eventLogRoot(), "deleted")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return ret, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		id := common.EventRecordID(strings.TrimSpace(string(data)))
		if id != "" {
			ret[id] = struct{}{}
		}
	}
	return ret, nil
}

func (r *EventRecordRepository) eventExists(id common.EventRecordID) (bool, error) {
	if r.isEventDeleted(id) {
		return false, nil
	}
	found := false
	if err := r.scanEventRecords(func(item *eventstream.EventRecord) error {
		if item.ID == id {
			found = true
		}
		return nil
	}); err != nil {
		return false, err
	}
	return found, nil
}

func (r *EventRecordRepository) appendLogLine(root string, item *eventstream.EventRecord) error {
	line, err := r.eventRecordLogLine(item)
	if err != nil {
		return err
	}
	data, err := json.Marshal(line)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	segment, err := r.activeEventSegment(len(data))
	if err != nil {
		return err
	}
	file, err := os.OpenFile(segment, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (r *EventRecordRepository) eventRecordLogLine(item *eventstream.EventRecord) (eventRecordLogLine, error) {
	record := cloneEventRecord(item)
	line := eventRecordLogLine{Record: *record}
	if len(record.Payload.Fields) == 0 {
		return line, nil
	}
	payload, err := json.Marshal(record.Payload)
	if err != nil {
		return eventRecordLogLine{}, err
	}
	if len(payload) <= eventRecordPayloadInlineThreshold {
		return line, nil
	}
	payloadRef := filepath.ToSlash(filepath.Join("payloads", safeEventRecordFileName(string(record.ID))+".json"))
	payloadPath := filepath.Join(r.eventLogRoot(), filepath.FromSlash(payloadRef))
	if err := writeFileAtomic(payloadPath, payload, 0o644); err != nil {
		return eventRecordLogLine{}, err
	}
	line.Record.PayloadSummary = eventPayloadSummary(record.Payload)
	line.Record.PayloadRef = payloadRef
	line.Record.Payload = workorcheventpayload.Payload{}
	line.PayloadRef = payloadRef
	return line, nil
}

func (r *EventRecordRepository) activeEventSegment(nextBytes int) (string, error) {
	dir := r.eventSegmentDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	matches, err := filepath.Glob(filepath.Join(dir, eventRecordSegmentPrefix+"*"+eventRecordSegmentSuffix))
	if err != nil {
		return "", err
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return filepath.Join(dir, eventRecordSegmentName(1)), nil
	}
	current := matches[len(matches)-1]
	info, err := os.Stat(current)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err == nil && info.Size()+int64(nextBytes) <= eventRecordSegmentMaxBytes {
		return current, nil
	}
	return filepath.Join(dir, eventRecordSegmentName(len(matches)+1)), nil
}

func eventRecordSegmentName(index int) string {
	return fmt.Sprintf("%s%06d%s", eventRecordSegmentPrefix, index, eventRecordSegmentSuffix)
}

func (r *EventRecordRepository) scanEventRecords(fn func(*eventstream.EventRecord) error) error {
	if fn == nil {
		return common.ErrInvalidArgument
	}
	matches, err := filepath.Glob(filepath.Join(r.eventSegmentDir(), eventRecordSegmentPrefix+"*"+eventRecordSegmentSuffix))
	if err != nil {
		return err
	}
	sort.Strings(matches)
	for _, path := range matches {
		if err := r.scanEventRecordSegment(path, fn); err != nil {
			return err
		}
	}
	return nil
}

func (r *EventRecordRepository) scanEventRecordsReverse(fn func(*eventstream.EventRecord) (bool, error)) error {
	if fn == nil {
		return common.ErrInvalidArgument
	}
	matches, err := filepath.Glob(filepath.Join(r.eventSegmentDir(), eventRecordSegmentPrefix+"*"+eventRecordSegmentSuffix))
	if err != nil {
		return err
	}
	sort.Strings(matches)
	for i := len(matches) - 1; i >= 0; i-- {
		stop, err := r.scanEventRecordSegmentReverse(matches[i], fn)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	return nil
}

func (r *EventRecordRepository) scanEventRecordSegment(path string, fn func(*eventstream.EventRecord) error) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()
	recordsScanned := 0
	defer func() {
		repository.RecordEventRecordMetadataScan(recordsScanned)
	}()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var recordLine eventRecordLogLine
		if err := json.Unmarshal([]byte(line), &recordLine); err != nil {
			return err
		}
		recordsScanned++
		item := cloneEventRecord(&recordLine.Record)
		if recordLine.PayloadRef != "" {
			item.PayloadRef = recordLine.PayloadRef
			if len(item.PayloadSummary.Fields) == 0 {
				item.PayloadSummary = eventPayloadRef(recordLine.PayloadRef)
			}
			item.Payload = eventPayloadRef(recordLine.PayloadRef)
		}
		if err := fn(item); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (r *EventRecordRepository) scanEventRecordSegmentReverse(path string, fn func(*eventstream.EventRecord) (bool, error)) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	recordsScanned := 0
	defer func() {
		repository.RecordEventRecordMetadataScan(recordsScanned)
	}()
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var recordLine eventRecordLogLine
		if err := json.Unmarshal(line, &recordLine); err != nil {
			return false, err
		}
		recordsScanned++
		item := cloneEventRecord(&recordLine.Record)
		if recordLine.PayloadRef != "" {
			item.PayloadRef = recordLine.PayloadRef
			if len(item.PayloadSummary.Fields) == 0 {
				item.PayloadSummary = eventPayloadRef(recordLine.PayloadRef)
			}
			item.Payload = eventPayloadRef(recordLine.PayloadRef)
		}
		stop, err := fn(item)
		if err != nil {
			return false, err
		}
		if stop {
			return true, nil
		}
	}
	return false, nil
}

func (r *EventRecordRepository) hydrateEventRecordPayload(item *eventstream.EventRecord) error {
	if item == nil || len(item.Payload.Fields) == 0 {
		return nil
	}
	ref, _ := item.Payload.String(eventRecordPayloadRefKey)
	if strings.TrimSpace(ref) == "" {
		return nil
	}
	payload, err := r.readEventPayload(ref)
	if err != nil {
		return err
	}
	item.Payload = payload
	return nil
}

func stripEventRecordPayload(item *eventstream.EventRecord) {
	if item == nil || len(item.Payload.Fields) == 0 {
		return
	}
	if len(item.PayloadSummary.Fields) == 0 {
		item.PayloadSummary = eventPayloadSummary(item.Payload)
	}
	item.Payload = workorcheventpayload.Payload{}
}

func eventPayloadSummary(payload workorcheventpayload.Payload) workorcheventpayload.Payload {
	if len(payload.Fields) == 0 {
		return workorcheventpayload.Payload{}
	}
	keys := make([]string, 0, len(payload.Fields))
	for _, field := range payload.Fields {
		if field.Name != "" {
			keys = append(keys, field.Name)
		}
	}
	sort.Strings(keys)
	truncated := false
	if len(keys) > 16 {
		keys = keys[:16]
		truncated = true
	}
	fields := []workorcheventpayload.Field{
		workorcheventpayload.IntField("fieldCount", int64(len(payload.Fields))),
		workorcheventpayload.StringArrayField("fields", keys),
	}
	if truncated {
		fields = append(fields, workorcheventpayload.BoolField("truncated", true))
	}
	return workorcheventpayload.New(fields...)
}

func (r *EventRecordRepository) readEventPayload(ref string) (workorcheventpayload.Payload, error) {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(ref)))
	if clean == "." || clean == "" || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return workorcheventpayload.Payload{}, common.ErrInvalidArgument
	}
	data, err := os.ReadFile(filepath.Join(r.eventLogRoot(), clean))
	if err != nil {
		return workorcheventpayload.Payload{}, err
	}
	repository.RecordEventRecordPayloadHydrate(len(data))
	payload := workorcheventpayload.Payload{}
	if len(data) == 0 {
		return payload, nil
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return workorcheventpayload.Payload{}, err
	}
	return payload, nil
}

func eventPayloadRef(ref string) workorcheventpayload.Payload {
	return workorcheventpayload.New(workorcheventpayload.StringField(eventRecordPayloadRefKey, ref))
}

func safeEventRecordFileName(value string) string {
	return readableRepositoryItemName(value)
}

func safeRepositoryItemFileName(value string) string {
	return safeRepositoryItemPath(value)
}

func readableRepositoryItemName(value string) string {
	return runtimepath.SafePath(value)
}

func safeRepositoryItemPath(value string) string {
	segments := runtimepath.SafeSegmentsWithMax(value, runtimepath.DefaultMaxSegmentLen)
	if len(segments) == 0 {
		return "empty"
	}
	return filepath.Join(segments...)
}

func repositoryItemRoot(path string, legacyFileName string, directoryName string) string {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if strings.EqualFold(filepath.Base(cleaned), legacyFileName) {
		return filepath.Join(filepath.Dir(cleaned), directoryName)
	}
	if strings.EqualFold(filepath.Ext(cleaned), ".json") {
		return strings.TrimSuffix(cleaned, filepath.Ext(cleaned))
	}
	return cleaned
}

func repositoryItemPath(root string, id string) string {
	return filepath.Join(root, safeRepositoryItemFileName(id)+".json")
}

func repositoryItemPaths(root string) ([]string, error) {
	return repositoryFiles(root, "json")
}

func itemMatchesRuntimeWorkspace(ctx context.Context, workspaceID common.WorkspaceID) bool {
	scopedWorkspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	return scopedWorkspaceID == "" || workspaceID == scopedWorkspaceID
}

func listHasMultipleRunWorkspaces[T any](items []*T, workspaceID func(*T) common.WorkspaceID) bool {
	if len(items) <= 1 {
		return false
	}
	var found common.WorkspaceID
	for _, item := range items {
		if item == nil {
			continue
		}
		current := workspaceID(item)
		if found == "" {
			found = current
			continue
		}
		if current != found {
			return true
		}
	}
	return false
}

func repositoryMetadataDirs(root string) ([]string, error) {
	paths, err := repositoryFiles(root, "metadata.json")
	if err != nil {
		return nil, err
	}
	ret := make([]string, 0, len(paths))
	for _, path := range paths {
		ret = append(ret, filepath.Dir(path))
	}
	return ret, nil
}

func repositorySequenceScopedDir(root string, scopeID string, itemID string) string {
	return filepath.Join(root, filepath.FromSlash(runtimepath.SequenceScopedPath(root, scopeID, itemID)))
}

func repositorySequenceScopeRoot(root string, scopeID string) string {
	scopeSafe := runtimepath.TruncateSafeSegment(runtimepath.SafeSegment(scopeID), runtimepath.DefaultMaxSegmentLen)
	if scopeSafe == "" {
		scopeSafe = "scope"
	}
	return filepath.Join(root, scopeSafe)
}

func repositoryMetadataDirByID[T any](root string, id string, idOf func(T) string) (string, error) {
	dirs, err := repositoryMetadataDirs(root)
	if err != nil {
		return "", err
	}
	for _, dir := range dirs {
		data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			return "", err
		}
		if idOf(item) == id {
			return dir, nil
		}
	}
	return "", common.ErrNotFound
}

func repositoryFiles(root string, match string) ([]string, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return nil, nil
	}
	var ret []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		switch match {
		case "json":
			if strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
				ret = append(ret, path)
			}
		default:
			if strings.EqualFold(entry.Name(), match) {
				ret = append(ret, path)
			}
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Strings(ret)
	return ret, nil
}

func readJSONItemFile[T any](path string) (T, error) {
	var item T
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return item, common.ErrNotFound
		}
		return item, err
	}
	if err := json.Unmarshal(data, &item); err != nil {
		return item, err
	}
	return item, nil
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
	if item.Revision == 0 {
		item.Revision = 1
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

func (r *WorkspaceRepository) CompareAndSwap(_ context.Context, item *workspace.Workspace, expectedRevision uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	items, err := loadMap[common.WorkspaceID, workspace.Workspace](r.path)
	if err != nil {
		return err
	}
	current, ok := items[item.ID]
	if !ok {
		return common.ErrNotFound
	}
	if current.Revision != expectedRevision {
		return common.ErrConflict
	}
	item.Revision = expectedRevision + 1
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

func (r *WorkspaceTemplateRepository) Create(_ context.Context, item *workspace.WorkspaceTemplate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[string, workspace.WorkspaceTemplate](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneWorkspaceTemplate(item)
	return saveMap(r.path, items)
}

func (r *WorkspaceTemplateRepository) Get(_ context.Context, id string) (*workspace.WorkspaceTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[string, workspace.WorkspaceTemplate](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneWorkspaceTemplate(&item), nil
}

func (r *WorkspaceTemplateRepository) List(_ context.Context) ([]*workspace.WorkspaceTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[string, workspace.WorkspaceTemplate](r.path)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	ret := make([]*workspace.WorkspaceTemplate, 0, len(ids))
	for _, id := range ids {
		item := items[id]
		ret = append(ret, cloneWorkspaceTemplate(&item))
	}
	return ret, nil
}

func (r *WorkspaceTemplateRepository) Update(_ context.Context, item *workspace.WorkspaceTemplate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[string, workspace.WorkspaceTemplate](r.path)
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

	items, err := loadMap[string, workspace.WorkspaceTemplate](r.path)
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

func workspaceScopedMapKey(workspaceID common.WorkspaceID, id string) string {
	return fmt.Sprintf("%d:%s:%s", len(workspaceID), workspaceID, strings.TrimSpace(id))
}

func validateRuntimeWorkspaceScope(ctx context.Context, workspaceID common.WorkspaceID) error {
	scopedWorkspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if scopedWorkspaceID != "" && workspaceID != "" && scopedWorkspaceID != workspaceID {
		return common.ErrConflict
	}
	return nil
}

func findWorkspaceScopedMapKey[T any](ctx context.Context, items map[string]T, id string, itemID func(T) string, itemWorkspaceID func(T) common.WorkspaceID) (string, error) {
	id = strings.TrimSpace(id)
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	var found string
	for key, item := range items {
		if itemID(item) != id {
			continue
		}
		currentWorkspaceID := itemWorkspaceID(item)
		if workspaceID != "" && currentWorkspaceID != workspaceID {
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

func NewGoalRepository(path string) *GoalRepository {
	return &GoalRepository{path: path}
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

	items, err := loadMap[string, workspace.Goal](r.path)
	if err != nil {
		return err
	}
	lookupCtx := common.ContextWithRuntimeWorkspaceID(context.Background(), item.WorkspaceID)
	if _, err := findWorkspaceScopedMapKey(lookupCtx, items, string(item.ID), func(current workspace.Goal) string {
		return string(current.ID)
	}, func(current workspace.Goal) common.WorkspaceID {
		return current.WorkspaceID
	}); err == nil || err == common.ErrConflict {
		return common.ErrConflict
	} else if err != common.ErrNotFound {
		return err
	}
	key := workspaceScopedMapKey(item.WorkspaceID, string(item.ID))
	if _, ok := items[key]; ok {
		return common.ErrConflict
	}
	items[key] = *cloneGoal(item)
	return saveMap(r.path, items)
}

func (r *GoalRepository) Get(ctx context.Context, id common.GoalID) (*workspace.Goal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[string, workspace.Goal](r.path)
	if err != nil {
		return nil, err
	}
	key, err := findWorkspaceScopedMapKey(ctx, items, string(id), func(current workspace.Goal) string {
		return string(current.ID)
	}, func(current workspace.Goal) common.WorkspaceID {
		return current.WorkspaceID
	})
	if err != nil {
		return nil, err
	}
	item := items[key]
	return cloneGoal(&item), nil
}

func (r *GoalRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspace.Goal, error) {
	if err := validateRuntimeWorkspaceScope(ctx, workspaceID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[string, workspace.Goal](r.path)
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

func (r *GoalRepository) Update(ctx context.Context, item *workspace.Goal) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, item.WorkspaceID); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[string, workspace.Goal](r.path)
	if err != nil {
		return err
	}
	lookupCtx := ctx
	if item.WorkspaceID != "" {
		lookupCtx = common.ContextWithRuntimeWorkspaceID(context.Background(), item.WorkspaceID)
	}
	key, err := findWorkspaceScopedMapKey(lookupCtx, items, string(item.ID), func(current workspace.Goal) string {
		return string(current.ID)
	}, func(current workspace.Goal) common.WorkspaceID {
		return current.WorkspaceID
	})
	if err != nil {
		return err
	}
	current := items[key]
	next := cloneGoal(item)
	if next.WorkspaceID == "" {
		next.WorkspaceID = current.WorkspaceID
	}
	if current.WorkspaceID != "" && next.WorkspaceID != current.WorkspaceID {
		return common.ErrConflict
	}
	items[key] = *next
	return saveMap(r.path, items)
}

func (r *GoalRepository) Delete(ctx context.Context, id common.GoalID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	items, err := loadMap[string, workspace.Goal](r.path)
	if err != nil {
		return err
	}
	key, err := findWorkspaceScopedMapKey(ctx, items, string(id), func(current workspace.Goal) string {
		return string(current.ID)
	}, func(current workspace.Goal) common.WorkspaceID {
		return current.WorkspaceID
	})
	if err != nil {
		return err
	}
	delete(items, key)
	return saveMap(r.path, items)
}

type DoDRepository struct {
	mu   sync.RWMutex
	path string
}

func NewDoDRepository(path string) *DoDRepository {
	return &DoDRepository{path: path}
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

	items, err := loadMap[string, workspace.DoD](r.path)
	if err != nil {
		return err
	}
	lookupCtx := common.ContextWithRuntimeWorkspaceID(context.Background(), item.WorkspaceID)
	if _, err := findWorkspaceScopedMapKey(lookupCtx, items, string(item.ID), func(current workspace.DoD) string {
		return string(current.ID)
	}, func(current workspace.DoD) common.WorkspaceID {
		return current.WorkspaceID
	}); err == nil || err == common.ErrConflict {
		return common.ErrConflict
	} else if err != common.ErrNotFound {
		return err
	}
	key := workspaceScopedMapKey(item.WorkspaceID, string(item.ID))
	if _, ok := items[key]; ok {
		return common.ErrConflict
	}
	items[key] = *cloneDoD(item)
	return saveMap(r.path, items)
}

func (r *DoDRepository) Get(ctx context.Context, id common.DoDID) (*workspace.DoD, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[string, workspace.DoD](r.path)
	if err != nil {
		return nil, err
	}
	key, err := findWorkspaceScopedMapKey(ctx, items, string(id), func(current workspace.DoD) string {
		return string(current.ID)
	}, func(current workspace.DoD) common.WorkspaceID {
		return current.WorkspaceID
	})
	if err != nil {
		return nil, err
	}
	item := items[key]
	return cloneDoD(&item), nil
}

func (r *DoDRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspace.DoD, error) {
	if err := validateRuntimeWorkspaceScope(ctx, workspaceID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[string, workspace.DoD](r.path)
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

func (r *DoDRepository) Update(ctx context.Context, item *workspace.DoD) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, item.WorkspaceID); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	items, err := loadMap[string, workspace.DoD](r.path)
	if err != nil {
		return err
	}
	lookupCtx := ctx
	if item.WorkspaceID != "" {
		lookupCtx = common.ContextWithRuntimeWorkspaceID(context.Background(), item.WorkspaceID)
	}
	key, err := findWorkspaceScopedMapKey(lookupCtx, items, string(item.ID), func(current workspace.DoD) string {
		return string(current.ID)
	}, func(current workspace.DoD) common.WorkspaceID {
		return current.WorkspaceID
	})
	if err != nil {
		return err
	}
	current := items[key]
	next := cloneDoD(item)
	if next.WorkspaceID == "" {
		next.WorkspaceID = current.WorkspaceID
	}
	if current.WorkspaceID != "" && next.WorkspaceID != current.WorkspaceID {
		return common.ErrConflict
	}
	items[key] = *next
	return saveMap(r.path, items)
}

func (r *DoDRepository) Delete(ctx context.Context, id common.DoDID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	items, err := loadMap[string, workspace.DoD](r.path)
	if err != nil {
		return err
	}
	key, err := findWorkspaceScopedMapKey(ctx, items, string(id), func(current workspace.DoD) string {
		return string(current.ID)
	}, func(current workspace.DoD) common.WorkspaceID {
		return current.WorkspaceID
	})
	if err != nil {
		return err
	}
	delete(items, key)
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
	if item.Revision == 0 {
		item.Revision = 1
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

func (r *RunRepository) GetByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID, id common.RunID) (*run.Run, error) {
	if workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.RunID, run.Run](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok || item.WorkspaceID != workspaceID {
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

func (r *RunRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*run.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items, err := loadMap[common.RunID, run.Run](r.path)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(items))
	for id, item := range items {
		if item.WorkspaceID == workspaceID {
			ids = append(ids, string(id))
		}
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

func (r *RunRepository) CompareAndSwap(_ context.Context, item *run.Run, expectedRevision uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	items, err := loadMap[common.RunID, run.Run](r.path)
	if err != nil {
		return err
	}
	current, ok := items[item.ID]
	if !ok {
		return common.ErrNotFound
	}
	if current.WorkspaceID != item.WorkspaceID {
		return common.ErrConflict
	}
	if current.Revision != expectedRevision {
		return common.ErrConflict
	}
	item.Revision = expectedRevision + 1
	items[item.ID] = *cloneRun(item)
	return saveMap(r.path, items)
}

func (r *RunRepository) Delete(_ context.Context, id common.RunID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.RunID, run.Run](r.path, id)
}

type StepRepository struct {
	mu                sync.RWMutex
	path              string
	fullRunCache      map[common.RunID][]*run.Step
	fullRunCacheOrder []common.RunID
}

const maxCachedFullStepRuns = 8

func NewStepRepository(path string) *StepRepository {
	return &StepRepository{
		path:         path,
		fullRunCache: map[common.RunID][]*run.Step{},
	}
}

func (r *StepRepository) Create(_ context.Context, item *run.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	dir := r.stepDir(item.ID)
	if _, err := os.Stat(filepath.Join(dir, "metadata.json")); err == nil {
		return common.ErrConflict
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := r.saveStep(item); err != nil {
		return err
	}
	r.invalidateStepRunCache(item.RunID)
	return nil
}

func (r *StepRepository) Get(_ context.Context, id common.StepID) (*run.Step, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.loadStep(id)
}

func (r *StepRepository) ListByRun(_ context.Context, runID common.RunID) ([]*run.Step, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cached := r.cloneCachedStepsByRun(runID); cached != nil {
		return cached, nil
	}

	root := r.stepRoot()
	dirs, err := repositoryMetadataDirs(repositorySequenceScopeRoot(root, string(runID)))
	if err != nil {
		return nil, err
	}
	if len(dirs) == 0 {
		dirs, err = repositoryMetadataDirs(root)
	}
	if err != nil {
		return nil, err
	}
	ret := make([]*run.Step, 0, len(dirs))
	for _, dir := range dirs {
		item, err := r.loadStepFromDir(dir)
		if err != nil {
			return nil, err
		}
		if item.RunID == runID {
			ret = append(ret, item)
		}
	}
	if len(ret) == 0 {
		return nil, nil
	}
	r.cacheStepsByRun(runID, ret)
	return cloneStepSlice(ret), nil
}

func (r *StepRepository) ListByRunSummary(_ context.Context, runID common.RunID) ([]*run.Step, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	root := r.stepRoot()
	dirs, err := repositoryMetadataDirs(repositorySequenceScopeRoot(root, string(runID)))
	if err != nil {
		return nil, err
	}
	if len(dirs) == 0 {
		dirs, err = repositoryMetadataDirs(root)
	}
	if err != nil {
		return nil, err
	}
	ret := make([]*run.Step, 0, len(dirs))
	for _, dir := range dirs {
		item, err := r.loadStepSummaryFromDir(dir)
		if err != nil {
			return nil, err
		}
		if item.RunID == runID {
			ret = append(ret, item)
		}
	}
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *StepRepository) Update(_ context.Context, item *run.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if _, err := os.Stat(filepath.Join(r.stepDir(item.ID), "metadata.json")); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	if err := r.saveStep(item); err != nil {
		return err
	}
	r.invalidateStepRunCache(item.RunID)
	return nil
}

func (r *StepRepository) Delete(_ context.Context, id common.StepID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	runID := stepRunIDFromStepID(id)
	if existing, err := r.loadStep(id); err == nil && existing != nil && existing.RunID != "" {
		runID = existing.RunID
	}
	dir := r.stepDir(id)
	if _, err := os.Stat(filepath.Join(dir, "metadata.json")); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	r.invalidateStepRunCache(runID)
	return nil
}

func (r *StepRepository) cloneCachedStepsByRun(runID common.RunID) []*run.Step {
	if r == nil || r.fullRunCache == nil {
		return nil
	}
	cached, ok := r.fullRunCache[runID]
	if !ok {
		return nil
	}
	return cloneStepSlice(cached)
}

func (r *StepRepository) cacheStepsByRun(runID common.RunID, steps []*run.Step) {
	if r == nil || runID == "" {
		return
	}
	if r.fullRunCache == nil {
		r.fullRunCache = map[common.RunID][]*run.Step{}
	}
	if _, exists := r.fullRunCache[runID]; !exists {
		r.fullRunCacheOrder = append(r.fullRunCacheOrder, runID)
	}
	r.fullRunCache[runID] = cloneStepSlice(steps)
	for len(r.fullRunCacheOrder) > maxCachedFullStepRuns {
		evict := r.fullRunCacheOrder[0]
		r.fullRunCacheOrder = append([]common.RunID(nil), r.fullRunCacheOrder[1:]...)
		delete(r.fullRunCache, evict)
	}
}

func (r *StepRepository) invalidateStepRunCache(runID common.RunID) {
	if r == nil || r.fullRunCache == nil || runID == "" {
		return
	}
	delete(r.fullRunCache, runID)
	for idx, cachedRunID := range r.fullRunCacheOrder {
		if cachedRunID != runID {
			continue
		}
		r.fullRunCacheOrder = append(r.fullRunCacheOrder[:idx], r.fullRunCacheOrder[idx+1:]...)
		return
	}
}

func stepRunIDFromStepID(id common.StepID) common.RunID {
	runID, _, _ := strings.Cut(string(id), "/")
	return common.RunID(runID)
}

func (r *StepRepository) stepRoot() string {
	path := filepath.Clean(strings.TrimSpace(r.path))
	if strings.EqualFold(filepath.Base(path), "step.json") {
		return filepath.Join(filepath.Dir(path), "steps")
	}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return strings.TrimSuffix(path, filepath.Ext(path))
	}
	return path
}

func (r *StepRepository) stepDir(id common.StepID) string {
	if dir, err := repositoryMetadataDirByID[run.Step](r.stepRoot(), string(id), func(item run.Step) string {
		return string(item.ID)
	}); err == nil {
		return dir
	}
	runID, _, _ := strings.Cut(string(id), "/")
	return repositorySequenceScopedDir(r.stepRoot(), runID, string(id))
}

func (r *StepRepository) stepDirForItem(item *run.Step) string {
	if item == nil {
		return r.stepDir("")
	}
	if dir, err := repositoryMetadataDirByID[run.Step](r.stepRoot(), string(item.ID), func(existing run.Step) string {
		return string(existing.ID)
	}); err == nil {
		return dir
	}
	return repositorySequenceScopedDir(r.stepRoot(), string(item.RunID), string(item.ID))
}

func (r *StepRepository) saveStep(item *run.Step) error {
	dir := r.stepDirForItem(item)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	metadata := *cloneStep(item)
	metadata.Input = workorcheventpayload.Payload{}
	metadata.Output = workorcheventpayload.Payload{}
	if err := writeJSONFileAtomic(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		return err
	}
	if err := writeOptionalMapFile(filepath.Join(dir, "input.json"), item.Input.Map()); err != nil {
		return err
	}
	if err := writeOptionalMapFile(filepath.Join(dir, "output.json"), item.Output.Map()); err != nil {
		return err
	}
	return writeJSONFileAtomic(filepath.Join(dir, "summary.json"), compactStepSummary(item))
}

func (r *StepRepository) loadStep(id common.StepID) (*run.Step, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return r.loadStepFromDir(r.stepDir(id))
}

func (r *StepRepository) loadStepFromDir(dir string) (*run.Step, error) {
	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, common.ErrNotFound
		}
		return nil, err
	}
	var item run.Step
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, err
	}
	input, err := readOptionalMapFile(filepath.Join(dir, "input.json"))
	if err != nil {
		return nil, err
	}
	output, err := readOptionalMapFile(filepath.Join(dir, "output.json"))
	if err != nil {
		return nil, err
	}
	item.Input, err = workorcheventpayload.FromMap(input)
	if err != nil {
		return nil, err
	}
	item.Output, err = workorcheventpayload.FromMap(output)
	if err != nil {
		return nil, err
	}
	return cloneStep(&item), nil
}

func (r *StepRepository) loadStepSummaryFromDir(dir string) (*run.Step, error) {
	data, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err == nil {
		var item run.Step
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if stepSummaryCurrent(&item) {
			return cloneStep(&item), nil
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	item, err := r.loadStepFromDir(dir)
	if err != nil {
		return nil, err
	}
	summary := compactStepSummary(item)
	_ = writeJSONFileAtomic(filepath.Join(dir, "summary.json"), summary)
	return summary, nil
}

type ToolCallRepository struct {
	mu   sync.RWMutex
	path string
}

func NewToolCallRepository(path string) *ToolCallRepository {
	return &ToolCallRepository{path: path}
}

func (r *ToolCallRepository) Create(_ context.Context, item *runtimeevidencemodel.ToolCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	dir := r.toolCallDir(item.ID)
	if _, err := os.Stat(filepath.Join(dir, "metadata.json")); err == nil {
		return common.ErrConflict
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return r.saveToolCall(item)
}

func (r *ToolCallRepository) Get(_ context.Context, id common.ToolCallID) (*runtimeevidencemodel.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.loadToolCall(id)
}

func (r *ToolCallRepository) ListByRun(_ context.Context, runID common.RunID) ([]*runtimeevidencemodel.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dirs, err := repositoryMetadataDirs(r.toolCallRoot())
	if err != nil {
		return nil, err
	}
	ret := make([]*runtimeevidencemodel.ToolCall, 0, len(dirs))
	for _, dir := range dirs {
		item, err := r.loadToolCallFromDir(dir)
		if err != nil {
			return nil, err
		}
		if item.RunID == runID {
			ret = append(ret, item)
		}
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *ToolCallRepository) ListByStep(_ context.Context, stepID common.StepID) ([]*runtimeevidencemodel.ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dirs, err := repositoryMetadataDirs(r.toolCallRoot())
	if err != nil {
		return nil, err
	}
	ret := make([]*runtimeevidencemodel.ToolCall, 0, len(dirs))
	for _, dir := range dirs {
		item, err := r.loadToolCallFromDir(dir)
		if err != nil {
			return nil, err
		}
		if item.StepID == stepID {
			ret = append(ret, item)
		}
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *ToolCallRepository) Update(_ context.Context, item *runtimeevidencemodel.ToolCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if _, err := os.Stat(filepath.Join(r.toolCallDir(item.ID), "metadata.json")); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return r.saveToolCall(item)
}

func (r *ToolCallRepository) Delete(_ context.Context, id common.ToolCallID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	dir := r.toolCallDir(id)
	if _, err := os.Stat(filepath.Join(dir, "metadata.json")); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func (r *ToolCallRepository) toolCallRoot() string {
	path := filepath.Clean(strings.TrimSpace(r.path))
	if strings.EqualFold(filepath.Base(path), "tool_call.json") {
		return filepath.Join(filepath.Dir(path), "tool-calls")
	}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return strings.TrimSuffix(path, filepath.Ext(path))
	}
	return path
}

func (r *ToolCallRepository) toolCallDir(id common.ToolCallID) string {
	if dir, err := repositoryMetadataDirByID[runtimeevidencemodel.ToolCall](r.toolCallRoot(), string(id), func(item runtimeevidencemodel.ToolCall) string {
		return string(item.ID)
	}); err == nil {
		return dir
	}
	runID, _, _ := strings.Cut(string(id), "/")
	return repositorySequenceScopedDir(r.toolCallRoot(), runID, string(id))
}

func (r *ToolCallRepository) toolCallDirForItem(item *runtimeevidencemodel.ToolCall) string {
	if item == nil {
		return r.toolCallDir("")
	}
	if dir, err := repositoryMetadataDirByID[runtimeevidencemodel.ToolCall](r.toolCallRoot(), string(item.ID), func(existing runtimeevidencemodel.ToolCall) string {
		return string(existing.ID)
	}); err == nil {
		return dir
	}
	return repositorySequenceScopedDir(r.toolCallRoot(), string(item.RunID), string(item.ID))
}

func (r *ToolCallRepository) saveToolCall(item *runtimeevidencemodel.ToolCall) error {
	dir := r.toolCallDirForItem(item)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	metadata := *cloneToolCall(item)
	metadata.Arguments = workorcheventpayload.Payload{}
	metadata.Output = workorcheventpayload.Payload{}
	if err := writeJSONFileAtomic(filepath.Join(dir, "metadata.json"), metadata); err != nil {
		return err
	}
	if err := writeOptionalMapFile(filepath.Join(dir, "arguments.json"), item.Arguments.Map()); err != nil {
		return err
	}
	return writeOptionalMapFile(filepath.Join(dir, "output.json"), item.Output.Map())
}

func (r *ToolCallRepository) loadToolCall(id common.ToolCallID) (*runtimeevidencemodel.ToolCall, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return r.loadToolCallFromDir(r.toolCallDir(id))
}

func (r *ToolCallRepository) loadToolCallFromDir(dir string) (*runtimeevidencemodel.ToolCall, error) {
	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, common.ErrNotFound
		}
		return nil, err
	}
	var item runtimeevidencemodel.ToolCall
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, err
	}
	arguments, err := readOptionalMapFile(filepath.Join(dir, "arguments.json"))
	if err != nil {
		return nil, err
	}
	output, err := readOptionalMapFile(filepath.Join(dir, "output.json"))
	if err != nil {
		return nil, err
	}
	item.Arguments, err = workorcheventpayload.FromMap(arguments)
	if err != nil {
		return nil, err
	}
	item.Output, err = workorcheventpayload.FromMap(output)
	if err != nil {
		return nil, err
	}
	return cloneToolCall(&item), nil
}

type PatchRepository struct {
	mu   sync.RWMutex
	path string
}

func NewPatchRepository(path string) *PatchRepository {
	return &PatchRepository{path: path}
}

func (r *PatchRepository) Create(_ context.Context, item *runtimeevidencemodel.Patch) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := r.patchPath(item.ID)
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return writeJSONFileAtomic(path, clonePatch(item))
}

func (r *PatchRepository) Get(_ context.Context, id common.PatchID) (*runtimeevidencemodel.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, err := readJSONItemFile[runtimeevidencemodel.Patch](r.patchPath(id))
	if err != nil {
		return nil, err
	}
	return clonePatch(&item), nil
}

func (r *PatchRepository) ListByRun(_ context.Context, runID common.RunID) ([]*runtimeevidencemodel.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.list(func(item runtimeevidencemodel.Patch) bool { return item.RunID == runID })
}

func (r *PatchRepository) ListByStep(_ context.Context, stepID common.StepID) ([]*runtimeevidencemodel.Patch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.list(func(item runtimeevidencemodel.Patch) bool { return item.StepID == stepID })
}

func (r *PatchRepository) Update(_ context.Context, item *runtimeevidencemodel.Patch) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := r.patchPath(item.ID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return writeJSONFileAtomic(path, clonePatch(item))
}

func (r *PatchRepository) Delete(_ context.Context, id common.PatchID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	path := r.patchPath(id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return os.Remove(path)
}

func (r *PatchRepository) patchRoot() string {
	return repositoryItemRoot(r.path, "patch.json", "patches")
}

func (r *PatchRepository) patchPath(id common.PatchID) string {
	return repositoryItemPath(r.patchRoot(), string(id))
}

func (r *PatchRepository) list(matches func(runtimeevidencemodel.Patch) bool) ([]*runtimeevidencemodel.Patch, error) {
	paths, err := repositoryItemPaths(r.patchRoot())
	if err != nil {
		return nil, err
	}
	ret := make([]*runtimeevidencemodel.Patch, 0, len(paths))
	for _, path := range paths {
		item, err := readJSONItemFile[runtimeevidencemodel.Patch](path)
		if err != nil {
			return nil, err
		}
		if matches == nil || matches(item) {
			ret = append(ret, clonePatch(&item))
		}
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

type ValidationResultRepository struct {
	mu   sync.RWMutex
	path string
}

func NewValidationResultRepository(path string) *ValidationResultRepository {
	return &ValidationResultRepository{path: path}
}

func (r *ValidationResultRepository) Create(_ context.Context, item *runtimeevidencemodel.ValidationResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := r.validationResultPathForItem(item)
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return writeJSONFileAtomic(path, cloneValidationResult(item))
}

func (r *ValidationResultRepository) Get(_ context.Context, id common.ValidationResultID) (*runtimeevidencemodel.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, err := readJSONItemFile[runtimeevidencemodel.ValidationResult](r.validationResultPath(id))
	if err != nil {
		return nil, err
	}
	return cloneValidationResult(&item), nil
}

func (r *ValidationResultRepository) ListByRun(_ context.Context, runID common.RunID) ([]*runtimeevidencemodel.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.list(func(item runtimeevidencemodel.ValidationResult) bool { return item.RunID == runID })
}

func (r *ValidationResultRepository) ListByStep(_ context.Context, stepID common.StepID) ([]*runtimeevidencemodel.ValidationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.list(func(item runtimeevidencemodel.ValidationResult) bool { return item.StepID == stepID })
}

func (r *ValidationResultRepository) Update(_ context.Context, item *runtimeevidencemodel.ValidationResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := r.validationResultPathForItem(item)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return writeJSONFileAtomic(path, cloneValidationResult(item))
}

func (r *ValidationResultRepository) Delete(_ context.Context, id common.ValidationResultID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	path := r.validationResultPath(id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Dir(path))
}

func (r *ValidationResultRepository) validationResultRoot() string {
	return repositoryItemRoot(r.path, "validation_result.json", "validations")
}

func (r *ValidationResultRepository) validationResultPath(id common.ValidationResultID) string {
	if dir, err := repositoryMetadataDirByID[runtimeevidencemodel.ValidationResult](r.validationResultRoot(), string(id), func(item runtimeevidencemodel.ValidationResult) string {
		return string(item.ID)
	}); err == nil {
		return filepath.Join(dir, "metadata.json")
	}
	runID, _, _ := strings.Cut(string(id), "/")
	return filepath.Join(repositorySequenceScopedDir(r.validationResultRoot(), runID, string(id)), "metadata.json")
}

func (r *ValidationResultRepository) validationResultPathForItem(item *runtimeevidencemodel.ValidationResult) string {
	if item == nil {
		return r.validationResultPath("")
	}
	if dir, err := repositoryMetadataDirByID[runtimeevidencemodel.ValidationResult](r.validationResultRoot(), string(item.ID), func(existing runtimeevidencemodel.ValidationResult) string {
		return string(existing.ID)
	}); err == nil {
		return filepath.Join(dir, "metadata.json")
	}
	return filepath.Join(repositorySequenceScopedDir(r.validationResultRoot(), string(item.RunID), string(item.ID)), "metadata.json")
}

func (r *ValidationResultRepository) list(matches func(runtimeevidencemodel.ValidationResult) bool) ([]*runtimeevidencemodel.ValidationResult, error) {
	paths, err := repositoryFiles(r.validationResultRoot(), "metadata.json")
	if err != nil {
		return nil, err
	}
	ret := make([]*runtimeevidencemodel.ValidationResult, 0, len(paths))
	for _, path := range paths {
		item, err := readJSONItemFile[runtimeevidencemodel.ValidationResult](path)
		if err != nil {
			return nil, err
		}
		if matches == nil || matches(item) {
			ret = append(ret, cloneValidationResult(&item))
		}
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
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

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := r.runtimeCheckpointPath(item.ID)
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return writeJSONFileAtomic(path, cloneRuntimeCheckpoint(item))
}

func (r *RuntimeCheckpointRepository) Get(_ context.Context, id common.CheckpointID) (*run.RuntimeCheckpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, err := readJSONItemFile[run.RuntimeCheckpoint](r.runtimeCheckpointPath(id))
	if err != nil {
		return nil, err
	}
	return cloneRuntimeCheckpoint(&item), nil
}

func (r *RuntimeCheckpointRepository) ListByRun(_ context.Context, runID common.RunID) ([]*run.RuntimeCheckpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.list(func(item run.RuntimeCheckpoint) bool { return item.RunID == runID })
}

func (r *RuntimeCheckpointRepository) Update(_ context.Context, item *run.RuntimeCheckpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := r.runtimeCheckpointPath(item.ID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return writeJSONFileAtomic(path, cloneRuntimeCheckpoint(item))
}

func (r *RuntimeCheckpointRepository) Delete(_ context.Context, id common.CheckpointID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	path := r.runtimeCheckpointPath(id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return os.Remove(path)
}

func (r *RuntimeCheckpointRepository) runtimeCheckpointRoot() string {
	return repositoryItemRoot(r.path, "runtime_checkpoint.json", "checkpoints")
}

func (r *RuntimeCheckpointRepository) runtimeCheckpointPath(id common.CheckpointID) string {
	return repositoryItemPath(r.runtimeCheckpointRoot(), string(id))
}

func (r *RuntimeCheckpointRepository) list(matches func(run.RuntimeCheckpoint) bool) ([]*run.RuntimeCheckpoint, error) {
	paths, err := repositoryItemPaths(r.runtimeCheckpointRoot())
	if err != nil {
		return nil, err
	}
	ret := make([]*run.RuntimeCheckpoint, 0, len(paths))
	for _, path := range paths {
		item, err := readJSONItemFile[run.RuntimeCheckpoint](path)
		if err != nil {
			return nil, err
		}
		if matches == nil || matches(item) {
			ret = append(ret, cloneRuntimeCheckpoint(&item))
		}
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
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

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := r.ticketPath(item.ID)
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if strings.TrimSpace(item.CreatedAt) == "" {
		item.CreatedAt = now
	}
	if strings.TrimSpace(item.UpdatedAt) == "" {
		item.UpdatedAt = item.CreatedAt
	}
	return writeJSONFileAtomic(path, cloneDecisionTicket(item))
}

func (r *DecisionTicketRepository) Get(_ context.Context, id common.TicketID) (*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, err := readJSONItemFile[decision.Ticket](r.ticketPath(id))
	if err != nil {
		return nil, err
	}
	applyDecisionTicketFileTimestampFallback(&item, r.ticketPath(id))
	return cloneDecisionTicket(&item), nil
}

func (r *DecisionTicketRepository) List(_ context.Context) ([]*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.list(nil)
}

func (r *DecisionTicketRepository) ListByRunID(_ context.Context, runID common.RunID) ([]*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.list(func(item decision.Ticket) bool { return item.RunID == runID })
}

func (r *DecisionTicketRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*decision.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.list(func(item decision.Ticket) bool { return item.WorkspaceID == workspaceID })
}

func (r *DecisionTicketRepository) Update(_ context.Context, item *decision.Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := r.ticketPath(item.ID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	existing, err := readJSONItemFile[decision.Ticket](path)
	if err == nil && strings.TrimSpace(item.CreatedAt) == "" {
		item.CreatedAt = existing.CreatedAt
	}
	if strings.TrimSpace(item.CreatedAt) == "" {
		applyDecisionTicketFileTimestampFallback(item, path)
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return writeJSONFileAtomic(path, cloneDecisionTicket(item))
}

func (r *DecisionTicketRepository) Delete(_ context.Context, id common.TicketID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	path := r.ticketPath(id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return os.Remove(path)
}

func (r *DecisionTicketRepository) ticketRoot() string {
	return repositoryItemRoot(r.path, "ticket.json", filepath.Join("decisions", "tickets"))
}

func (r *DecisionTicketRepository) ticketPath(id common.TicketID) string {
	return repositoryItemPath(r.ticketRoot(), string(id))
}

func (r *DecisionTicketRepository) list(matches func(decision.Ticket) bool) ([]*decision.Ticket, error) {
	paths, err := repositoryItemPaths(r.ticketRoot())
	if err != nil {
		return nil, err
	}
	ret := make([]*decision.Ticket, 0, len(paths))
	for _, path := range paths {
		item, err := readJSONItemFile[decision.Ticket](path)
		if err != nil {
			return nil, err
		}
		applyDecisionTicketFileTimestampFallback(&item, path)
		if matches == nil || matches(item) {
			ret = append(ret, cloneDecisionTicket(&item))
		}
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	return ret, nil
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

	if item == nil || item.ID == "" || item.TicketID == "" {
		return common.ErrInvalidArgument
	}
	if _, err := r.findByTicketID(item.TicketID); err == nil {
		return common.ErrConflict
	} else if err != common.ErrNotFound {
		return err
	}
	path := r.resolutionPath(item.ID)
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return writeJSONFileAtomic(path, cloneDecisionResolution(item))
}

func (r *DecisionResolutionRepository) GetByTicketID(_ context.Context, ticketID common.TicketID) (*decision.Resolution, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, err := r.findByTicketID(ticketID)
	if err != nil {
		return nil, err
	}
	return cloneDecisionResolution(&item), nil
}

func (r *DecisionResolutionRepository) DeleteByTicketID(_ context.Context, ticketID common.TicketID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, path, err := r.findPathByTicketID(ticketID)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (r *DecisionResolutionRepository) resolutionRoot() string {
	return repositoryItemRoot(r.path, "resolution.json", filepath.Join("decisions", "resolutions"))
}

func (r *DecisionResolutionRepository) resolutionPath(id common.ResolutionID) string {
	return repositoryItemPath(r.resolutionRoot(), string(id))
}

func (r *DecisionResolutionRepository) findByTicketID(ticketID common.TicketID) (decision.Resolution, error) {
	item, _, err := r.findPathByTicketID(ticketID)
	return item, err
}

func (r *DecisionResolutionRepository) findPathByTicketID(ticketID common.TicketID) (decision.Resolution, string, error) {
	var zero decision.Resolution
	if ticketID == "" {
		return zero, "", common.ErrInvalidArgument
	}
	paths, err := repositoryItemPaths(r.resolutionRoot())
	if err != nil {
		return zero, "", err
	}
	for _, path := range paths {
		item, err := readJSONItemFile[decision.Resolution](path)
		if err != nil {
			return zero, "", err
		}
		if item.TicketID == ticketID {
			return item, path, nil
		}
	}
	return zero, "", common.ErrNotFound
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

	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	path := r.approvalPath(item.ID)
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if strings.TrimSpace(item.CreatedAt) == "" {
		item.CreatedAt = now
	}
	if strings.TrimSpace(item.UpdatedAt) == "" {
		item.UpdatedAt = item.CreatedAt
	}
	return writeJSONFileAtomic(path, cloneApproval(item))
}

func (r *ApprovalRepository) Get(_ context.Context, id common.ApprovalID) (*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, err := readJSONItemFile[approval.Approval](r.approvalPath(id))
	if err != nil {
		return nil, err
	}
	applyApprovalFileTimestampFallback(&item, r.approvalPath(id))
	return cloneApproval(&item), nil
}

func (r *ApprovalRepository) GetScoped(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, id common.ApprovalID) (*approval.Approval, error) {
	if workspaceID == "" || runID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	item, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.WorkspaceID != workspaceID || item.RunID != runID {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r *ApprovalRepository) List(_ context.Context) ([]*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.list(nil)
}

func (r *ApprovalRepository) ListByRunID(_ context.Context, runID common.RunID) ([]*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.list(func(item approval.Approval) bool { return item.RunID == runID })
}

func (r *ApprovalRepository) ListByRunScope(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) ([]*approval.Approval, error) {
	if workspaceID == "" || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.list(func(item approval.Approval) bool { return item.WorkspaceID == workspaceID && item.RunID == runID })
}

func (r *ApprovalRepository) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*approval.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.list(func(item approval.Approval) bool { return item.WorkspaceID == workspaceID })
}

func (r *ApprovalRepository) Update(_ context.Context, item *approval.Approval) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	path := r.approvalPath(item.ID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	existing, err := readJSONItemFile[approval.Approval](path)
	if err == nil && strings.TrimSpace(item.CreatedAt) == "" {
		item.CreatedAt = existing.CreatedAt
	}
	if strings.TrimSpace(item.CreatedAt) == "" {
		applyApprovalFileTimestampFallback(item, path)
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return writeJSONFileAtomic(path, cloneApproval(item))
}

func (r *ApprovalRepository) Delete(_ context.Context, id common.ApprovalID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	path := r.approvalPath(id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return os.Remove(path)
}

func (r *ApprovalRepository) DeleteScoped(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, id common.ApprovalID) error {
	if _, err := r.GetScoped(ctx, workspaceID, runID, id); err != nil {
		return err
	}
	return r.Delete(ctx, id)
}

func (r *ApprovalRepository) approvalRoot() string {
	return repositoryItemRoot(r.path, "approval.json", "approvals")
}

func (r *ApprovalRepository) approvalPath(id common.ApprovalID) string {
	return repositoryItemPath(r.approvalRoot(), string(id))
}

func (r *ApprovalRepository) list(matches func(approval.Approval) bool) ([]*approval.Approval, error) {
	paths, err := repositoryItemPaths(r.approvalRoot())
	if err != nil {
		return nil, err
	}
	ret := make([]*approval.Approval, 0, len(paths))
	for _, path := range paths {
		item, err := readJSONItemFile[approval.Approval](path)
		if err != nil {
			return nil, err
		}
		applyApprovalFileTimestampFallback(&item, path)
		if matches == nil || matches(item) {
			ret = append(ret, cloneApproval(&item))
		}
	}
	sort.Slice(ret, func(i, j int) bool {
		return string(ret[i].ID) < string(ret[j].ID)
	})
	return ret, nil
}

func loadMap[K comparable, V any](path string) (map[K]V, error) {
	cache := mapCacheEntry(path)
	cache.mu.RLock()
	if cache.loaded {
		items, ok := cache.items.(map[K]V)
		cache.mu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("file repository cache type mismatch for %s", path)
		}
		return cloneStoredMap(items), nil
	}
	cache.mu.RUnlock()

	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.loaded {
		items, ok := cache.items.(map[K]V)
		if !ok {
			return nil, fmt.Errorf("file repository cache type mismatch for %s", path)
		}
		return cloneStoredMap(items), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cache.items = map[K]V{}
			cache.loaded = true
			return map[K]V{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		cache.items = map[K]V{}
		cache.loaded = true
		return map[K]V{}, nil
	}
	items := map[K]V{}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	cache.items = cloneStoredMap(items)
	cache.loaded = true
	return cloneStoredMap(items), nil
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
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	cache := mapCacheEntry(path)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.items = cloneStoredMap(items)
	cache.loaded = true
	return nil
}

func writeJSONFileAtomic(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data, 0o644)
}

func writeOptionalMapFile(path string, value map[string]any) error {
	if value == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return writeJSONFileAtomic(path, value)
}

func readOptionalMapFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var item map[string]any
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	return item, nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func mapCacheEntry(path string) *mapFileCacheEntry {
	item, _ := mapFileCache.LoadOrStore(path, &mapFileCacheEntry{})
	return item.(*mapFileCacheEntry)
}

func cloneStoredMap[K comparable, V any](items map[K]V) map[K]V {
	cloned := make(map[K]V, len(items))
	for key, value := range items {
		cloned[key] = value
	}
	return cloned
}

func cloneWorkspace(item *workspace.Workspace) *workspace.Workspace {
	cloned := *item
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
	if item.StartupInput != nil {
		value := item.StartupInput.Clone()
		cloned.StartupInput = &value
	}
	if item.ActivationAdmission != nil {
		value := item.ActivationAdmission.Normalized()
		cloned.ActivationAdmission = &value
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
	return run.CloneRun(item)
}

func cloneStep(item *run.Step) *run.Step {
	cloned := *item
	cloned.Input = item.Input.Clone()
	cloned.Output = item.Output.Clone()
	return &cloned
}

func cloneStepSlice(items []*run.Step) []*run.Step {
	if items == nil {
		return nil
	}
	cloned := make([]*run.Step, 0, len(items))
	for _, item := range items {
		if item == nil {
			cloned = append(cloned, nil)
			continue
		}
		cloned = append(cloned, cloneStep(item))
	}
	return cloned
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

func cloneToolCall(item *runtimeevidencemodel.ToolCall) *runtimeevidencemodel.ToolCall {
	cloned := *item
	cloned.Arguments = item.Arguments.Clone()
	cloned.Output = item.Output.Clone()
	return &cloned
}

func clonePatch(item *runtimeevidencemodel.Patch) *runtimeevidencemodel.Patch {
	cloned := *item
	cloned.CoverageTargets = append([]runtimeevidencemodel.PatchCoverageTarget(nil), item.CoverageTargets...)
	cloned.AffectedFiles = cloneStringSlice(item.AffectedFiles)
	cloned.RiskReasons = cloneStringSlice(item.RiskReasons)
	cloned.Diagnostics = cloneEventPayloads(item.Diagnostics)
	return &cloned
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
	cloned := *item
	cloned.Context = item.Context.Clone()
	cloned.AutoGovernance = item.AutoGovernance.Clone()
	if item.RuntimeGovernanceProposal != nil {
		proposal := item.RuntimeGovernanceProposal.Clone()
		cloned.RuntimeGovernanceProposal = &proposal
	}
	return &cloned
}

func applyDecisionTicketFileTimestampFallback(item *decision.Ticket, path string) {
	if item == nil || (strings.TrimSpace(item.CreatedAt) != "" && strings.TrimSpace(item.UpdatedAt) != "") {
		return
	}
	timestamp := fileModTimeTimestamp(path)
	if timestamp == "" {
		return
	}
	if strings.TrimSpace(item.CreatedAt) == "" {
		item.CreatedAt = timestamp
	}
	if strings.TrimSpace(item.UpdatedAt) == "" {
		item.UpdatedAt = timestamp
	}
}

func cloneDecisionResolution(item *decision.Resolution) *decision.Resolution {
	cloned := *item
	cloned.Payload = item.Payload.Clone()
	return &cloned
}

func cloneApproval(item *approval.Approval) *approval.Approval {
	return approval.CloneApproval(item)
}

func applyApprovalFileTimestampFallback(item *approval.Approval, path string) {
	if item == nil || (strings.TrimSpace(item.CreatedAt) != "" && strings.TrimSpace(item.UpdatedAt) != "") {
		return
	}
	timestamp := fileModTimeTimestamp(path)
	if timestamp == "" {
		return
	}
	if strings.TrimSpace(item.CreatedAt) == "" {
		item.CreatedAt = timestamp
	}
	if strings.TrimSpace(item.UpdatedAt) == "" {
		item.UpdatedAt = timestamp
	}
}

func fileModTimeTimestamp(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return info.ModTime().UTC().Format(time.RFC3339Nano)
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

func applyEventRecordCursorAndLimit(items []*eventstream.EventRecord, query workorchquerycontract.EventRecordQuery) []*eventstream.EventRecord {
	if query.CursorAfterID != "" {
		cursorMatched := false
		filtered := items[:0]
		for _, item := range items {
			if !cursorMatched {
				if item.ID == query.CursorAfterID {
					cursorMatched = true
				}
				continue
			}
			filtered = append(filtered, item)
		}
		items = filtered
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

func eventRecordMatches(item *eventstream.EventRecord, query workorchquerycontract.EventRecordQuery) bool {
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
	for _, topic := range query.ExcludeTopics {
		if strings.TrimSpace(topic) != "" && item.Topic == strings.TrimSpace(topic) {
			return false
		}
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
		out[k] = cloneAny(v)
	}
	return out
}

const stepSummarySchemaVersion = "workorch.step_summary"

func stepSummaryCurrent(item *run.Step) bool {
	if item == nil {
		return false
	}
	version, _ := item.Input.String("_summarySchemaVersion")
	return version == stepSummarySchemaVersion
}

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
	"patchRequests",
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
	"patchRequests",
	"patchType",
	"diff",
	"matchedTerms",
}

func stringFromAny(value any) string {
	typed, _ := value.(string)
	return typed
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
