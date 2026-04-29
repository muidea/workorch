package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
)

const (
	workspaceScopedWorkspaceFile         = "workspace.json"
	workspaceScopedWorkspaceIndexFile    = "workspace_index.json"
	workspaceScopedConfigDir             = "config"
	workspaceScopedSoulBindingsFile      = "soul_bindings.json"
	workspaceScopedGoalFile              = "goals.json"
	workspaceScopedDoDFile               = "dods.json"
	workspaceScopedMemoryItemFile        = "memory_item.json"
	workspaceScopedRunFile               = "run.json"
	workspaceScopedStepFile              = "step.json"
	workspaceScopedToolCallFile          = "tool_call.json"
	workspaceScopedPatchFile             = "patch.json"
	workspaceScopedValidationResultFile  = "validation_result.json"
	workspaceScopedRuntimeCheckpointFile = "runtime_checkpoint.json"
	workspaceScopedResultFile            = "result.json"
	workspaceScopedArtifactFile          = "artifact.json"
	workspaceScopedEventRecordFile       = "event_record.json"
	workspaceScopedTicketFile            = "ticket.json"
	workspaceScopedResolutionFile        = "resolution.json"
	workspaceScopedApprovalFile          = "approval.json"
)

type workspaceScopedFileStore struct {
	dataRoot string

	indexMu            sync.Mutex
	mu                 sync.Mutex
	workspaces         map[string]*fileRepo.WorkspaceRepository
	goals              map[string]*fileRepo.GoalRepository
	dods               map[string]*fileRepo.DoDRepository
	memoryItems        map[string]*fileRepo.MemoryItemRepository
	runs               map[string]*fileRepo.RunRepository
	steps              map[string]*fileRepo.StepRepository
	toolCalls          map[string]*fileRepo.ToolCallRepository
	patches            map[string]*fileRepo.PatchRepository
	validationResults  map[string]*fileRepo.ValidationResultRepository
	runtimeCheckpoints map[string]*fileRepo.RuntimeCheckpointRepository
	results            map[string]*fileRepo.ResultRepository
	artifacts          map[string]*fileRepo.ArtifactRepository
	eventRecords       map[string]*fileRepo.EventRecordRepository
	tickets            map[string]*fileRepo.DecisionTicketRepository
	resolutions        map[string]*fileRepo.DecisionResolutionRepository
	approvals          map[string]*fileRepo.ApprovalRepository
}

type WorkspaceScopedFileRepositorySet struct {
	MemoryItems        repository.MemoryItemRepository
	Workspaces         repository.WorkspaceRepository
	Goals              repository.GoalRepository
	DoDs               repository.DoDRepository
	Results            repository.ResultRepository
	Artifacts          repository.ArtifactRepository
	EventRecords       repository.EventRecordRepository
	Runs               repository.RunRepository
	Steps              repository.StepRepository
	ToolCalls          repository.ToolCallRepository
	Patches            repository.PatchRepository
	ValidationResults  repository.ValidationResultRepository
	RuntimeCheckpoints repository.RuntimeCheckpointRepository
	Tickets            repository.DecisionTicketRepository
	Resolutions        repository.DecisionResolutionRepository
	Approvals          repository.ApprovalRepository
}

type workspaceIndex struct {
	Workspaces []workspaceIndexEntry `json:"workspaces"`
}

type workspaceIndexEntry struct {
	WorkspaceID      common.WorkspaceID    `json:"workspaceId"`
	WorkspaceRoot    string                `json:"workspaceRoot"`
	WorkingDirectory string                `json:"workingDirectory"`
	State            common.WorkspaceState `json:"state"`
	ActiveRunID      common.RunID          `json:"activeRunId,omitempty"`
	UpdatedAt        string                `json:"updatedAt"`
}

type workspaceSoulBindingsFile struct {
	DefaultSoulID common.SoulID            `json:"defaultSoulId,omitempty"`
	Bindings      map[string]common.SoulID `json:"bindings,omitempty"`
}

func NewWorkspaceScopedFileRepositorySet(dataRoot string) WorkspaceScopedFileRepositorySet {
	store := newWorkspaceScopedFileStore(dataRoot)
	runs := newWorkspaceScopedRunRepository(store)
	tickets := newWorkspaceScopedDecisionTicketRepository(store)
	artifacts := NewArtifactStorageRepository(newWorkspaceScopedArtifactRepository(store), dataRoot)
	return WorkspaceScopedFileRepositorySet{
		MemoryItems:        newWorkspaceScopedMemoryItemRepository(store),
		Workspaces:         newWorkspaceScopedWorkspaceRepository(store),
		Goals:              newWorkspaceScopedGoalRepository(store),
		DoDs:               newWorkspaceScopedDoDRepository(store),
		Results:            newWorkspaceScopedResultRepository(store),
		Artifacts:          artifacts,
		EventRecords:       newWorkspaceScopedEventRecordRepository(store),
		Runs:               runs,
		Steps:              newWorkspaceScopedStepRepository(store, runs),
		ToolCalls:          newWorkspaceScopedToolCallRepository(store),
		Patches:            newWorkspaceScopedPatchRepository(store),
		ValidationResults:  newWorkspaceScopedValidationResultRepository(store),
		RuntimeCheckpoints: newWorkspaceScopedRuntimeCheckpointRepository(store),
		Tickets:            tickets,
		Resolutions:        newWorkspaceScopedDecisionResolutionRepository(store, tickets),
		Approvals:          newWorkspaceScopedApprovalRepository(store),
	}
}

func newWorkspaceScopedFileStore(dataRoot string) *workspaceScopedFileStore {
	return &workspaceScopedFileStore{
		dataRoot:           dataRoot,
		workspaces:         map[string]*fileRepo.WorkspaceRepository{},
		goals:              map[string]*fileRepo.GoalRepository{},
		dods:               map[string]*fileRepo.DoDRepository{},
		memoryItems:        map[string]*fileRepo.MemoryItemRepository{},
		runs:               map[string]*fileRepo.RunRepository{},
		steps:              map[string]*fileRepo.StepRepository{},
		toolCalls:          map[string]*fileRepo.ToolCallRepository{},
		patches:            map[string]*fileRepo.PatchRepository{},
		validationResults:  map[string]*fileRepo.ValidationResultRepository{},
		runtimeCheckpoints: map[string]*fileRepo.RuntimeCheckpointRepository{},
		results:            map[string]*fileRepo.ResultRepository{},
		artifacts:          map[string]*fileRepo.ArtifactRepository{},
		eventRecords:       map[string]*fileRepo.EventRecordRepository{},
		tickets:            map[string]*fileRepo.DecisionTicketRepository{},
		resolutions:        map[string]*fileRepo.DecisionResolutionRepository{},
		approvals:          map[string]*fileRepo.ApprovalRepository{},
	}
}

func (s *workspaceScopedFileStore) workspaceRoot(workspaceID common.WorkspaceID) string {
	return filepath.Join(s.dataRoot, "workspaces", string(workspaceID))
}

func (s *workspaceScopedFileStore) workspaceSoulBindingsPath(workspaceID common.WorkspaceID) string {
	return filepath.Join(s.workspaceRoot(workspaceID), workspaceScopedConfigDir, workspaceScopedSoulBindingsFile)
}

func (s *workspaceScopedFileStore) runRoot(workspaceID common.WorkspaceID, runID common.RunID) string {
	return filepath.Join(s.dataRoot, "workspaces", string(workspaceID), "runs", string(runID))
}

func (s *workspaceScopedFileStore) systemPath(name string) string {
	return filepath.Join(s.dataRoot, "system", name)
}

func (s *workspaceScopedFileStore) workspaceIndexPath() string {
	return s.systemPath(workspaceScopedWorkspaceIndexFile)
}

func (s *workspaceScopedFileStore) workspaceRootRef(workspaceID common.WorkspaceID) string {
	return filepath.ToSlash(filepath.Join("workspaces", string(workspaceID)))
}

func (s *workspaceScopedFileStore) runPath(root string, name string) string {
	return filepath.Join(root, name)
}

func (s *workspaceScopedFileStore) allWorkspaceRoots() []string {
	matches, _ := filepath.Glob(filepath.Join(s.dataRoot, "workspaces", "*"))
	sort.Strings(matches)
	return matches
}

func (s *workspaceScopedFileStore) allRunRoots() []string {
	matches, _ := filepath.Glob(filepath.Join(s.dataRoot, "workspaces", "*", "runs", "*"))
	sort.Strings(matches)
	return matches
}

func (s *workspaceScopedFileStore) workspaceRunRoots(workspaceID common.WorkspaceID) []string {
	if workspaceID == "" {
		return s.allRunRoots()
	}
	matches, _ := filepath.Glob(filepath.Join(s.dataRoot, "workspaces", string(workspaceID), "runs", "*"))
	sort.Strings(matches)
	return matches
}

func (s *workspaceScopedFileStore) workspaceRepo(path string) *fileRepo.WorkspaceRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.workspaces[path]
	if !ok {
		repo = fileRepo.NewWorkspaceRepository(path)
		s.workspaces[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) goalRepo(path string) *fileRepo.GoalRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.goals[path]
	if !ok {
		repo = fileRepo.NewGoalRepository(path)
		s.goals[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) dodRepo(path string) *fileRepo.DoDRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.dods[path]
	if !ok {
		repo = fileRepo.NewDoDRepository(path)
		s.dods[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) memoryItemRepo(path string) *fileRepo.MemoryItemRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.memoryItems[path]
	if !ok {
		repo = fileRepo.NewMemoryItemRepository(path)
		s.memoryItems[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) runRepo(path string) *fileRepo.RunRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.runs[path]
	if !ok {
		repo = fileRepo.NewRunRepository(path)
		s.runs[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) stepRepo(path string) *fileRepo.StepRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.steps[path]
	if !ok {
		repo = fileRepo.NewStepRepository(path)
		s.steps[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) toolCallRepo(path string) *fileRepo.ToolCallRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.toolCalls[path]
	if !ok {
		repo = fileRepo.NewToolCallRepository(path)
		s.toolCalls[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) patchRepo(path string) *fileRepo.PatchRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.patches[path]
	if !ok {
		repo = fileRepo.NewPatchRepository(path)
		s.patches[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) validationResultRepo(path string) *fileRepo.ValidationResultRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.validationResults[path]
	if !ok {
		repo = fileRepo.NewValidationResultRepository(path)
		s.validationResults[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) runtimeCheckpointRepo(path string) *fileRepo.RuntimeCheckpointRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.runtimeCheckpoints[path]
	if !ok {
		repo = fileRepo.NewRuntimeCheckpointRepository(path)
		s.runtimeCheckpoints[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) resultRepo(path string) *fileRepo.ResultRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.results[path]
	if !ok {
		repo = fileRepo.NewResultRepository(path)
		s.results[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) artifactRepo(path string) *fileRepo.ArtifactRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.artifacts[path]
	if !ok {
		repo = fileRepo.NewArtifactRepository(path)
		s.artifacts[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) eventRecordRepo(path string) *fileRepo.EventRecordRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.eventRecords[path]
	if !ok {
		repo = fileRepo.NewEventRecordRepository(path)
		s.eventRecords[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) ticketRepo(path string) *fileRepo.DecisionTicketRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.tickets[path]
	if !ok {
		repo = fileRepo.NewDecisionTicketRepository(path)
		s.tickets[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) resolutionRepo(path string) *fileRepo.DecisionResolutionRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.resolutions[path]
	if !ok {
		repo = fileRepo.NewDecisionResolutionRepository(path)
		s.resolutions[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) approvalRepo(path string) *fileRepo.ApprovalRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.approvals[path]
	if !ok {
		repo = fileRepo.NewApprovalRepository(path)
		s.approvals[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) loadWorkspaceIndexLocked() (workspaceIndex, error) {
	path := s.workspaceIndexPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return workspaceIndex{}, nil
		}
		return workspaceIndex{}, err
	}
	if len(data) == 0 {
		return workspaceIndex{}, nil
	}
	var idx workspaceIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return workspaceIndex{}, err
	}
	return idx, nil
}

func (s *workspaceScopedFileStore) saveWorkspaceIndexLocked(idx workspaceIndex) error {
	sort.Slice(idx.Workspaces, func(i, j int) bool {
		return string(idx.Workspaces[i].WorkspaceID) < string(idx.Workspaces[j].WorkspaceID)
	})
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	path := s.workspaceIndexPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *workspaceScopedFileStore) upsertWorkspaceIndex(item *workspacemodel.Workspace) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	idx, err := s.loadWorkspaceIndexLocked()
	if err != nil {
		return err
	}
	entry := workspaceIndexEntry{
		WorkspaceID:      item.ID,
		WorkspaceRoot:    s.workspaceRootRef(item.ID),
		WorkingDirectory: item.WorkingDirectory,
		State:            item.State,
		ActiveRunID:      item.ActiveRunID,
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	replaced := false
	for pos := range idx.Workspaces {
		if idx.Workspaces[pos].WorkspaceID == item.ID {
			idx.Workspaces[pos] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		idx.Workspaces = append(idx.Workspaces, entry)
	}
	return s.saveWorkspaceIndexLocked(idx)
}

func (s *workspaceScopedFileStore) listWorkspaceIndexEntries() ([]workspaceIndexEntry, error) {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	idx, err := s.loadWorkspaceIndexLocked()
	if err != nil {
		return nil, err
	}
	sort.Slice(idx.Workspaces, func(i, j int) bool {
		return string(idx.Workspaces[i].WorkspaceID) < string(idx.Workspaces[j].WorkspaceID)
	})
	return append([]workspaceIndexEntry(nil), idx.Workspaces...), nil
}

type workspaceScopedWorkspaceRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedWorkspaceRepository(store *workspaceScopedFileStore) *workspaceScopedWorkspaceRepository {
	return &workspaceScopedWorkspaceRepository{store: store}
}

func (r *workspaceScopedWorkspaceRepository) Create(ctx context.Context, item *workspacemodel.Workspace) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if err := r.store.workspaceRepo(filepath.Join(r.store.workspaceRoot(item.ID), workspaceScopedWorkspaceFile)).Create(ctx, workspaceMetadata(item)); err != nil {
		return err
	}
	if err := r.store.saveWorkspaceSoulBindings(item); err != nil {
		return err
	}
	return r.store.upsertWorkspaceIndex(item)
}

func (r *workspaceScopedWorkspaceRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	item, err := r.store.workspaceRepo(filepath.Join(r.store.workspaceRoot(id), workspaceScopedWorkspaceFile)).Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.store.attachWorkspaceSoulBindings(item)
}

func (r *workspaceScopedWorkspaceRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	entries, err := r.store.listWorkspaceIndexEntries()
	if err != nil {
		return nil, err
	}
	ret := []*workspacemodel.Workspace{}
	for _, entry := range entries {
		if entry.WorkspaceID == "" {
			continue
		}
		item, err := r.Get(ctx, entry.WorkspaceID)
		if err == common.ErrNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		ret = append(ret, item)
	}
	sortWorkspaces(ret)
	return ret, nil
}

func (r *workspaceScopedWorkspaceRepository) Update(ctx context.Context, item *workspacemodel.Workspace) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if err := r.store.workspaceRepo(filepath.Join(r.store.workspaceRoot(item.ID), workspaceScopedWorkspaceFile)).Update(ctx, workspaceMetadata(item)); err != nil {
		return err
	}
	if err := r.store.saveWorkspaceSoulBindings(item); err != nil {
		return err
	}
	return r.store.upsertWorkspaceIndex(item)
}

func workspaceMetadata(item *workspacemodel.Workspace) *workspacemodel.Workspace {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.DefaultSoulID = ""
	cloned.SoulBindings = nil
	return &cloned
}

func (s *workspaceScopedFileStore) attachWorkspaceSoulBindings(item *workspacemodel.Workspace) (*workspacemodel.Workspace, error) {
	if item == nil {
		return nil, nil
	}
	bindings, err := s.loadWorkspaceSoulBindings(item.ID)
	if err != nil {
		return nil, err
	}
	cloned := *item
	cloned.DefaultSoulID = bindings.DefaultSoulID
	if len(bindings.Bindings) > 0 {
		cloned.SoulBindings = map[string]common.SoulID{}
		for key, value := range bindings.Bindings {
			cloned.SoulBindings[key] = value
		}
	} else {
		cloned.SoulBindings = nil
	}
	return &cloned, nil
}

func (s *workspaceScopedFileStore) loadWorkspaceSoulBindings(workspaceID common.WorkspaceID) (workspaceSoulBindingsFile, error) {
	if workspaceID == "" {
		return workspaceSoulBindingsFile{}, common.ErrInvalidArgument
	}
	data, err := os.ReadFile(s.workspaceSoulBindingsPath(workspaceID))
	if os.IsNotExist(err) {
		return workspaceSoulBindingsFile{}, nil
	}
	if err != nil {
		return workspaceSoulBindingsFile{}, err
	}
	if len(data) == 0 {
		return workspaceSoulBindingsFile{}, nil
	}
	var ret workspaceSoulBindingsFile
	if err := json.Unmarshal(data, &ret); err != nil {
		return workspaceSoulBindingsFile{}, err
	}
	return ret, nil
}

func (s *workspaceScopedFileStore) saveWorkspaceSoulBindings(item *workspacemodel.Workspace) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	payload := workspaceSoulBindingsFile{
		DefaultSoulID: item.DefaultSoulID,
	}
	if len(item.SoulBindings) > 0 {
		payload.Bindings = map[string]common.SoulID{}
		for key, value := range item.SoulBindings {
			payload.Bindings[key] = value
		}
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	path := s.workspaceSoulBindingsPath(item.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

type workspaceScopedGoalRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedGoalRepository(store *workspaceScopedFileStore) *workspaceScopedGoalRepository {
	return &workspaceScopedGoalRepository{store: store}
}

func (r *workspaceScopedGoalRepository) Create(ctx context.Context, item *workspacemodel.Goal) error {
	if item == nil || item.WorkspaceID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.goalRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedGoalFile)).Create(ctx, item)
}

func (r *workspaceScopedGoalRepository) Get(ctx context.Context, id common.GoalID) (*workspacemodel.Goal, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	for _, root := range r.store.allWorkspaceRoots() {
		item, err := r.store.goalRepo(filepath.Join(root, workspaceScopedGoalFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedGoalRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return r.store.goalRepo(filepath.Join(r.store.workspaceRoot(workspaceID), workspaceScopedGoalFile)).ListByWorkspaceID(ctx, workspaceID)
}

func (r *workspaceScopedGoalRepository) Update(ctx context.Context, item *workspacemodel.Goal) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" {
		err := r.store.goalRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedGoalFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	return r.store.goalRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedGoalFile)).Update(ctx, item)
}

type workspaceScopedDoDRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedDoDRepository(store *workspaceScopedFileStore) *workspaceScopedDoDRepository {
	return &workspaceScopedDoDRepository{store: store}
}

func (r *workspaceScopedDoDRepository) Create(ctx context.Context, item *workspacemodel.DoD) error {
	if item == nil || item.WorkspaceID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.dodRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedDoDFile)).Create(ctx, item)
}

func (r *workspaceScopedDoDRepository) Get(ctx context.Context, id common.DoDID) (*workspacemodel.DoD, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	for _, root := range r.store.allWorkspaceRoots() {
		item, err := r.store.dodRepo(filepath.Join(root, workspaceScopedDoDFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedDoDRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return r.store.dodRepo(filepath.Join(r.store.workspaceRoot(workspaceID), workspaceScopedDoDFile)).ListByWorkspaceID(ctx, workspaceID)
}

func (r *workspaceScopedDoDRepository) Update(ctx context.Context, item *workspacemodel.DoD) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" {
		err := r.store.dodRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedDoDFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	return r.store.dodRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedDoDFile)).Update(ctx, item)
}

type workspaceScopedMemoryItemRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedMemoryItemRepository(store *workspaceScopedFileStore) *workspaceScopedMemoryItemRepository {
	return &workspaceScopedMemoryItemRepository{store: store}
}

func (r *workspaceScopedMemoryItemRepository) Create(ctx context.Context, item *memorymodel.MemoryItem) error {
	if item == nil || item.WorkspaceID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.memoryItemRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedMemoryItemFile)).Create(ctx, item)
}

func (r *workspaceScopedMemoryItemRepository) Get(ctx context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	for _, root := range r.store.allWorkspaceRoots() {
		item, err := r.store.memoryItemRepo(filepath.Join(root, workspaceScopedMemoryItemFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedMemoryItemRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return r.store.memoryItemRepo(filepath.Join(r.store.workspaceRoot(workspaceID), workspaceScopedMemoryItemFile)).ListByWorkspaceID(ctx, workspaceID)
}

func (r *workspaceScopedMemoryItemRepository) Search(ctx context.Context, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	if query.WorkspaceID != "" {
		return r.store.memoryItemRepo(filepath.Join(r.store.workspaceRoot(query.WorkspaceID), workspaceScopedMemoryItemFile)).Search(ctx, query)
	}
	queryWithoutLimit := query
	queryWithoutLimit.Limit = 0
	ret := []*memorymodel.MemoryItem{}
	for _, root := range r.store.allWorkspaceRoots() {
		items, err := r.store.memoryItemRepo(filepath.Join(root, workspaceScopedMemoryItemFile)).Search(ctx, queryWithoutLimit)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortMemoryItems(ret, query)
	if query.Limit > 0 && len(ret) > query.Limit {
		ret = ret[:query.Limit]
	}
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedMemoryItemRepository) Update(ctx context.Context, item *memorymodel.MemoryItem) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" {
		err := r.store.memoryItemRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedMemoryItemFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	return r.store.memoryItemRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedMemoryItemFile)).Update(ctx, item)
}

type workspaceScopedRunRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedRunRepository(store *workspaceScopedFileStore) *workspaceScopedRunRepository {
	return &workspaceScopedRunRepository{store: store}
}

func (r *workspaceScopedRunRepository) Create(ctx context.Context, item *runmodel.Run) error {
	if item == nil || item.WorkspaceID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.runRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.ID), workspaceScopedRunFile)).Create(ctx, item)
}

func (r *workspaceScopedRunRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	for _, root := range r.store.allRunRoots() {
		item, err := r.store.runRepo(r.store.runPath(root, workspaceScopedRunFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedRunRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	ret := []*runmodel.Run{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.runRepo(r.store.runPath(root, workspaceScopedRunFile)).List(ctx)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortRuns(ret)
	return ret, nil
}

func (r *workspaceScopedRunRepository) Update(ctx context.Context, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" {
		err := r.store.runRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.ID), workspaceScopedRunFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	return r.store.runRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.ID), workspaceScopedRunFile)).Update(ctx, item)
}

func (r *workspaceScopedRunRepository) rootForRun(ctx context.Context, runID common.RunID) (string, error) {
	item, err := r.Get(ctx, runID)
	if err != nil {
		return "", err
	}
	return r.store.runRoot(item.WorkspaceID, item.ID), nil
}

type workspaceScopedStepRepository struct {
	store *workspaceScopedFileStore
	runs  *workspaceScopedRunRepository
}

func newWorkspaceScopedStepRepository(store *workspaceScopedFileStore, runs *workspaceScopedRunRepository) *workspaceScopedStepRepository {
	return &workspaceScopedStepRepository{store: store, runs: runs}
}

func (r *workspaceScopedStepRepository) Create(ctx context.Context, item *runmodel.Step) error {
	if item == nil || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	root, err := r.runs.rootForRun(ctx, item.RunID)
	if err != nil {
		return err
	}
	return r.store.stepRepo(r.store.runPath(root, workspaceScopedStepFile)).Create(ctx, item)
}

func (r *workspaceScopedStepRepository) Get(ctx context.Context, id common.StepID) (*runmodel.Step, error) {
	for _, root := range r.store.allRunRoots() {
		item, err := r.store.stepRepo(r.store.runPath(root, workspaceScopedStepFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedStepRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	ret := []*runmodel.Step{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.stepRepo(r.store.runPath(root, workspaceScopedStepFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortSteps(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedStepRepository) Update(ctx context.Context, item *runmodel.Step) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.RunID != "" {
		if root, err := r.runs.rootForRun(ctx, item.RunID); err == nil {
			err = r.store.stepRepo(r.store.runPath(root, workspaceScopedStepFile)).Update(ctx, item)
			if err == nil || err != common.ErrNotFound {
				return err
			}
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.RunID = current.RunID
	root, err := r.runs.rootForRun(ctx, item.RunID)
	if err != nil {
		return err
	}
	return r.store.stepRepo(r.store.runPath(root, workspaceScopedStepFile)).Update(ctx, item)
}

type workspaceScopedToolCallRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedToolCallRepository(store *workspaceScopedFileStore) *workspaceScopedToolCallRepository {
	return &workspaceScopedToolCallRepository{store: store}
}

func (r *workspaceScopedToolCallRepository) Create(ctx context.Context, item *runmodel.ToolCall) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.toolCallRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedToolCallFile)).Create(ctx, item)
}

func (r *workspaceScopedToolCallRepository) Get(ctx context.Context, id common.ToolCallID) (*runmodel.ToolCall, error) {
	for _, root := range r.store.allRunRoots() {
		item, err := r.store.toolCallRepo(r.store.runPath(root, workspaceScopedToolCallFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedToolCallRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.ToolCall, error) {
	ret := []*runmodel.ToolCall{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.toolCallRepo(r.store.runPath(root, workspaceScopedToolCallFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortToolCalls(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedToolCallRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runmodel.ToolCall, error) {
	ret := []*runmodel.ToolCall{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.toolCallRepo(r.store.runPath(root, workspaceScopedToolCallFile)).ListByStep(ctx, stepID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortToolCalls(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedToolCallRepository) Update(ctx context.Context, item *runmodel.ToolCall) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.store.toolCallRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedToolCallFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.store.toolCallRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedToolCallFile)).Update(ctx, item)
}

type workspaceScopedPatchRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedPatchRepository(store *workspaceScopedFileStore) *workspaceScopedPatchRepository {
	return &workspaceScopedPatchRepository{store: store}
}

func (r *workspaceScopedPatchRepository) Create(ctx context.Context, item *runmodel.Patch) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.patchRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedPatchFile)).Create(ctx, item)
}

func (r *workspaceScopedPatchRepository) Get(ctx context.Context, id common.PatchID) (*runmodel.Patch, error) {
	for _, root := range r.store.allRunRoots() {
		item, err := r.store.patchRepo(r.store.runPath(root, workspaceScopedPatchFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedPatchRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.Patch, error) {
	ret := []*runmodel.Patch{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.patchRepo(r.store.runPath(root, workspaceScopedPatchFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortPatches(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedPatchRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runmodel.Patch, error) {
	ret := []*runmodel.Patch{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.patchRepo(r.store.runPath(root, workspaceScopedPatchFile)).ListByStep(ctx, stepID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortPatches(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedPatchRepository) Update(ctx context.Context, item *runmodel.Patch) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.store.patchRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedPatchFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.store.patchRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedPatchFile)).Update(ctx, item)
}

type workspaceScopedValidationResultRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedValidationResultRepository(store *workspaceScopedFileStore) *workspaceScopedValidationResultRepository {
	return &workspaceScopedValidationResultRepository{store: store}
}

func (r *workspaceScopedValidationResultRepository) Create(ctx context.Context, item *runmodel.ValidationResult) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.validationResultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedValidationResultFile)).Create(ctx, item)
}

func (r *workspaceScopedValidationResultRepository) Get(ctx context.Context, id common.ValidationResultID) (*runmodel.ValidationResult, error) {
	for _, root := range r.store.allRunRoots() {
		item, err := r.store.validationResultRepo(r.store.runPath(root, workspaceScopedValidationResultFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedValidationResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.ValidationResult, error) {
	ret := []*runmodel.ValidationResult{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.validationResultRepo(r.store.runPath(root, workspaceScopedValidationResultFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortValidationResults(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedValidationResultRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runmodel.ValidationResult, error) {
	ret := []*runmodel.ValidationResult{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.validationResultRepo(r.store.runPath(root, workspaceScopedValidationResultFile)).ListByStep(ctx, stepID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortValidationResults(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedValidationResultRepository) Update(ctx context.Context, item *runmodel.ValidationResult) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.store.validationResultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedValidationResultFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.store.validationResultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedValidationResultFile)).Update(ctx, item)
}

type workspaceScopedRuntimeCheckpointRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedRuntimeCheckpointRepository(store *workspaceScopedFileStore) *workspaceScopedRuntimeCheckpointRepository {
	return &workspaceScopedRuntimeCheckpointRepository{store: store}
}

func (r *workspaceScopedRuntimeCheckpointRepository) Create(ctx context.Context, item *runmodel.RuntimeCheckpoint) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.runtimeCheckpointRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedRuntimeCheckpointFile)).Create(ctx, item)
}

func (r *workspaceScopedRuntimeCheckpointRepository) Get(ctx context.Context, id common.CheckpointID) (*runmodel.RuntimeCheckpoint, error) {
	for _, root := range r.store.allRunRoots() {
		item, err := r.store.runtimeCheckpointRepo(r.store.runPath(root, workspaceScopedRuntimeCheckpointFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedRuntimeCheckpointRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.RuntimeCheckpoint, error) {
	ret := []*runmodel.RuntimeCheckpoint{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.runtimeCheckpointRepo(r.store.runPath(root, workspaceScopedRuntimeCheckpointFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortRuntimeCheckpoints(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedRuntimeCheckpointRepository) Update(ctx context.Context, item *runmodel.RuntimeCheckpoint) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.store.runtimeCheckpointRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedRuntimeCheckpointFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.store.runtimeCheckpointRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedRuntimeCheckpointFile)).Update(ctx, item)
}

type workspaceScopedResultRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedResultRepository(store *workspaceScopedFileStore) *workspaceScopedResultRepository {
	return &workspaceScopedResultRepository{store: store}
}

func (r *workspaceScopedResultRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.resultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedResultFile)).Create(ctx, item)
}

func (r *workspaceScopedResultRepository) Get(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	for _, root := range r.store.allRunRoots() {
		item, err := r.store.resultRepo(r.store.runPath(root, workspaceScopedResultFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedResultRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	ret := []*resultmodel.Result{}
	for _, root := range r.store.workspaceRunRoots(workspaceID) {
		items, err := r.store.resultRepo(r.store.runPath(root, workspaceScopedResultFile)).ListByWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortResults(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	ret := []*resultmodel.Result{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.resultRepo(r.store.runPath(root, workspaceScopedResultFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortResults(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedResultRepository) Update(ctx context.Context, item *resultmodel.Result) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.store.resultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedResultFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.store.resultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedResultFile)).Update(ctx, item)
}

type workspaceScopedArtifactRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedArtifactRepository(store *workspaceScopedFileStore) *workspaceScopedArtifactRepository {
	return &workspaceScopedArtifactRepository{store: store}
}

func (r *workspaceScopedArtifactRepository) Create(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.artifactRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedArtifactFile)).Create(ctx, item)
}

func (r *workspaceScopedArtifactRepository) Get(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	for _, root := range r.store.allRunRoots() {
		item, err := r.store.artifactRepo(r.store.runPath(root, workspaceScopedArtifactFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedArtifactRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	ret := []*resultmodel.Artifact{}
	for _, root := range r.store.workspaceRunRoots(workspaceID) {
		items, err := r.store.artifactRepo(r.store.runPath(root, workspaceScopedArtifactFile)).ListByWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortArtifacts(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedArtifactRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	ret := []*resultmodel.Artifact{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.artifactRepo(r.store.runPath(root, workspaceScopedArtifactFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortArtifacts(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedArtifactRepository) ListByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	ret := []*resultmodel.Artifact{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.artifactRepo(r.store.runPath(root, workspaceScopedArtifactFile)).ListByResult(ctx, resultID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortArtifacts(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedArtifactRepository) Update(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.store.artifactRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedArtifactFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.store.artifactRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedArtifactFile)).Update(ctx, item)
}

type workspaceScopedEventRecordRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedEventRecordRepository(store *workspaceScopedFileStore) *workspaceScopedEventRecordRepository {
	return &workspaceScopedEventRecordRepository{store: store}
}

func (r *workspaceScopedEventRecordRepository) Append(ctx context.Context, item *eventstreammodel.EventRecord) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		return r.store.eventRecordRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedEventRecordFile)).Append(ctx, item)
	}
	return r.store.eventRecordRepo(r.store.systemPath(workspaceScopedEventRecordFile)).Append(ctx, item)
}

func (r *workspaceScopedEventRecordRepository) Get(ctx context.Context, id common.EventRecordID) (*eventstreammodel.EventRecord, error) {
	item, err := r.store.eventRecordRepo(r.store.systemPath(workspaceScopedEventRecordFile)).Get(ctx, id)
	if err == nil {
		return item, nil
	}
	if err != common.ErrNotFound {
		return nil, err
	}
	for _, root := range r.store.allRunRoots() {
		item, err := r.store.eventRecordRepo(r.store.runPath(root, workspaceScopedEventRecordFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedEventRecordRepository) List(ctx context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	listQuery := query
	listQuery.CursorAfterID = ""
	listQuery.Limit = 0
	ret := []*eventstreammodel.EventRecord{}
	systemItems, err := r.store.eventRecordRepo(r.store.systemPath(workspaceScopedEventRecordFile)).List(ctx, listQuery)
	if err != nil {
		return nil, err
	}
	ret = append(ret, systemItems...)
	for _, root := range r.store.workspaceRunRoots(query.WorkspaceID) {
		items, err := r.store.eventRecordRepo(r.store.runPath(root, workspaceScopedEventRecordFile)).List(ctx, listQuery)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortEventRecords(ret)
	ret = applyEventRecordCursorAndLimit(ret, query)
	return nilIfEmpty(ret), nil
}

type workspaceScopedDecisionTicketRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedDecisionTicketRepository(store *workspaceScopedFileStore) *workspaceScopedDecisionTicketRepository {
	return &workspaceScopedDecisionTicketRepository{store: store}
}

func (r *workspaceScopedDecisionTicketRepository) Create(ctx context.Context, item *decisionmodel.Ticket) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.ticketRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedTicketFile)).Create(ctx, item)
}

func (r *workspaceScopedDecisionTicketRepository) Get(ctx context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	for _, root := range r.store.allRunRoots() {
		item, err := r.store.ticketRepo(r.store.runPath(root, workspaceScopedTicketFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedDecisionTicketRepository) List(ctx context.Context) ([]*decisionmodel.Ticket, error) {
	ret := []*decisionmodel.Ticket{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.ticketRepo(r.store.runPath(root, workspaceScopedTicketFile)).List(ctx)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortTickets(ret)
	return ret, nil
}

func (r *workspaceScopedDecisionTicketRepository) Update(ctx context.Context, item *decisionmodel.Ticket) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.store.ticketRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedTicketFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.store.ticketRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedTicketFile)).Update(ctx, item)
}

type workspaceScopedDecisionResolutionRepository struct {
	store   *workspaceScopedFileStore
	tickets *workspaceScopedDecisionTicketRepository
}

func newWorkspaceScopedDecisionResolutionRepository(store *workspaceScopedFileStore, tickets *workspaceScopedDecisionTicketRepository) *workspaceScopedDecisionResolutionRepository {
	return &workspaceScopedDecisionResolutionRepository{store: store, tickets: tickets}
}

func (r *workspaceScopedDecisionResolutionRepository) Create(ctx context.Context, item *decisionmodel.Resolution) error {
	if item == nil || item.TicketID == "" {
		return common.ErrInvalidArgument
	}
	ticket, err := r.tickets.Get(ctx, item.TicketID)
	if err != nil {
		return err
	}
	return r.store.resolutionRepo(r.store.runPath(r.store.runRoot(ticket.WorkspaceID, ticket.RunID), workspaceScopedResolutionFile)).Create(ctx, item)
}

func (r *workspaceScopedDecisionResolutionRepository) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	if ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	if ticket, err := r.tickets.Get(ctx, ticketID); err == nil {
		item, err := r.store.resolutionRepo(r.store.runPath(r.store.runRoot(ticket.WorkspaceID, ticket.RunID), workspaceScopedResolutionFile)).GetByTicketID(ctx, ticketID)
		if err == nil || err != common.ErrNotFound {
			return item, err
		}
	} else if err != common.ErrNotFound {
		return nil, err
	}
	for _, root := range r.store.allRunRoots() {
		item, err := r.store.resolutionRepo(r.store.runPath(root, workspaceScopedResolutionFile)).GetByTicketID(ctx, ticketID)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

type workspaceScopedApprovalRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedApprovalRepository(store *workspaceScopedFileStore) *workspaceScopedApprovalRepository {
	return &workspaceScopedApprovalRepository{store: store}
}

func (r *workspaceScopedApprovalRepository) Create(ctx context.Context, item *approvalmodel.Approval) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.approvalRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedApprovalFile)).Create(ctx, item)
}

func (r *workspaceScopedApprovalRepository) Get(ctx context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	for _, root := range r.store.allRunRoots() {
		item, err := r.store.approvalRepo(r.store.runPath(root, workspaceScopedApprovalFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedApprovalRepository) List(ctx context.Context) ([]*approvalmodel.Approval, error) {
	ret := []*approvalmodel.Approval{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.approvalRepo(r.store.runPath(root, workspaceScopedApprovalFile)).List(ctx)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortApprovals(ret)
	return ret, nil
}

func (r *workspaceScopedApprovalRepository) Update(ctx context.Context, item *approvalmodel.Approval) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.store.approvalRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedApprovalFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.store.approvalRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedApprovalFile)).Update(ctx, item)
}

func nilIfEmpty[T any](items []T) []T {
	if len(items) == 0 {
		return nil
	}
	return items
}

func sortRuns(items []*runmodel.Run) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortWorkspaces(items []*workspacemodel.Workspace) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortMemoryItems(items []*memorymodel.MemoryItem, query repository.MemorySearchQuery) {
	sort.Slice(items, func(i, j int) bool {
		if len(query.Vector) > 0 && items[i].VectorScore != items[j].VectorScore {
			return items[i].VectorScore > items[j].VectorScore
		}
		if items[i].Importance != items[j].Importance {
			return items[i].Importance > items[j].Importance
		}
		return string(items[i].ID) < string(items[j].ID)
	})
}

func sortSteps(items []*runmodel.Step) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortToolCalls(items []*runmodel.ToolCall) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortPatches(items []*runmodel.Patch) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortValidationResults(items []*runmodel.ValidationResult) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortResults(items []*resultmodel.Result) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortArtifacts(items []*resultmodel.Artifact) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortEventRecords(items []*eventstreammodel.EventRecord) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].OccurredAt == items[j].OccurredAt {
			return string(items[i].ID) < string(items[j].ID)
		}
		return items[i].OccurredAt < items[j].OccurredAt
	})
}

func sortTickets(items []*decisionmodel.Ticket) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortApprovals(items []*approvalmodel.Approval) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func applyEventRecordCursorAndLimit(items []*eventstreammodel.EventRecord, query repository.EventRecordQuery) []*eventstreammodel.EventRecord {
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
		items = items[:query.Limit]
	}
	return items
}
