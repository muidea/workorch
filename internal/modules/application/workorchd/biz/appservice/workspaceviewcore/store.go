package workspaceviewcore

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

type Store struct {
	mu                 sync.RWMutex
	state              string
	errText            string
	globalRevision     int64
	updatedAt          time.Time
	rebuiltAt          time.Time
	workspaces         map[string]appmodelcore.Workspace
	workspaceOrder     []string
	snapshots          map[string]readmodelcore.WorkspaceSnapshot
	workspaceRevisions map[string]int64
	sourceRevision     int64
	workspaceSources   map[string]int64
	staleWorkspaces    map[string]struct{}
	globalStale        bool
}

func NewStore() *Store {
	return &Store{
		state:              StateBuilding,
		workspaces:         map[string]appmodelcore.Workspace{},
		snapshots:          map[string]readmodelcore.WorkspaceSnapshot{},
		workspaceRevisions: map[string]int64{},
		workspaceSources:   map[string]int64{},
		staleWorkspaces:    map[string]struct{}{},
	}
}

func (s *Store) Status() Status {
	if s == nil {
		return Status{State: StateFailed, Error: "workspace projection store is not configured"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statusLocked()
}

func (s *Store) StatusForWorkspace(workspaceID string) Status {
	if s == nil {
		return Status{State: StateFailed, Error: "workspace projection store is not configured"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statusForWorkspaceLocked(workspaceID)
}

func (s *Store) statusLocked() Status {
	state := s.state
	if state == StateReady && (s.globalStale || len(s.staleWorkspaces) > 0) {
		state = StateStale
	}
	return Status{
		State:               state,
		GlobalRevision:      s.globalRevision,
		SourceEventRevision: s.sourceRevision,
		WorkspaceTotal:      len(s.workspaces),
		UpdatedAt:           formatProjectionTime(s.updatedAt),
		RebuiltAt:           formatProjectionTime(s.rebuiltAt),
		Degraded:            s.state == StateDegraded,
		Error:               s.errText,
	}
}

func (s *Store) statusForWorkspaceLocked(workspaceID string) Status {
	ret := s.statusLocked()
	if workspaceID != "" {
		ret.WorkspaceRevision = s.workspaceRevisions[workspaceID]
		ret.SourceEventRevision = s.workspaceSources[workspaceID]
		if s.state == StateReady {
			if _, ok := s.staleWorkspaces[workspaceID]; ok {
				ret.State = StateStale
			}
		}
	}
	return ret
}

func (s *Store) setBuilding() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = StateBuilding
	s.errText = ""
	s.updatedAt = time.Now().UTC()
}

func (s *Store) setStopped() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = StateStopped
	s.updatedAt = time.Now().UTC()
}

func (s *Store) MarkStale(workspaceID string, sourceRevision int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sourceRevision > s.sourceRevision {
		s.sourceRevision = sourceRevision
	}
	if strings.TrimSpace(workspaceID) == "" {
		s.globalStale = true
	} else {
		s.staleWorkspaces[workspaceID] = struct{}{}
		if sourceRevision > s.workspaceSources[workspaceID] {
			s.workspaceSources[workspaceID] = sourceRevision
		}
	}
	s.updatedAt = time.Now().UTC()
}

func (s *Store) ReplaceAll(workspaces []appmodelcore.Workspace, snapshots map[string]readmodelcore.WorkspaceSnapshot, degraded error) {
	if s == nil {
		return
	}
	now := time.Now().UTC()
	nextWorkspaces := make(map[string]appmodelcore.Workspace, len(workspaces))
	nextOrder := make([]string, 0, len(workspaces))
	nextSnapshots := make(map[string]readmodelcore.WorkspaceSnapshot, len(snapshots))
	for _, item := range workspaces {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		if snapshot, ok := snapshots[item.ID]; ok && strings.TrimSpace(snapshot.Workspace.ID) != "" {
			item = snapshot.Workspace
			nextSnapshots[item.ID] = cloneWorkspaceSnapshot(snapshot)
		}
		nextWorkspaces[item.ID] = cloneWorkspace(item)
		nextOrder = append(nextOrder, item.ID)
	}
	sort.Strings(nextOrder)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaces = nextWorkspaces
	s.workspaceOrder = nextOrder
	s.snapshots = nextSnapshots
	s.globalRevision++
	for _, id := range nextOrder {
		s.workspaceRevisions[id]++
	}
	s.globalStale = false
	s.staleWorkspaces = map[string]struct{}{}
	if degraded != nil {
		s.state = StateDegraded
		s.errText = degraded.Error()
	} else {
		s.state = StateReady
		s.errText = ""
	}
	s.updatedAt = now
	s.rebuiltAt = now
}

func (s *Store) setFailed(err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = StateFailed
	if err != nil {
		s.errText = err.Error()
	}
	s.updatedAt = time.Now().UTC()
}

func (s *Store) UpsertSnapshot(snapshot readmodelcore.WorkspaceSnapshot) {
	if s == nil || strings.TrimSpace(snapshot.Workspace.ID) == "" {
		return
	}
	workspaceID := snapshot.Workspace.ID
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaces[workspaceID] = cloneWorkspace(snapshot.Workspace)
	if !stringSliceContains(s.workspaceOrder, workspaceID) {
		s.workspaceOrder = append(s.workspaceOrder, workspaceID)
		sort.Strings(s.workspaceOrder)
	}
	s.snapshots[workspaceID] = cloneWorkspaceSnapshot(snapshot)
	s.workspaceRevisions[workspaceID]++
	s.globalRevision++
	delete(s.staleWorkspaces, workspaceID)
	s.globalStale = false
	s.state = StateReady
	s.errText = ""
	s.updatedAt = time.Now().UTC()
}

func (s *Store) UpsertWorkspace(workspace appmodelcore.Workspace) {
	if s == nil || strings.TrimSpace(workspace.ID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaces[workspace.ID] = cloneWorkspace(workspace)
	if !stringSliceContains(s.workspaceOrder, workspace.ID) {
		s.workspaceOrder = append(s.workspaceOrder, workspace.ID)
		sort.Strings(s.workspaceOrder)
	}
	s.workspaceRevisions[workspace.ID]++
	s.globalRevision++
	delete(s.staleWorkspaces, workspace.ID)
	s.globalStale = false
	s.state = StateReady
	s.errText = ""
	s.updatedAt = time.Now().UTC()
}

func (s *Store) DeleteWorkspace(workspaceID string) {
	if s == nil || strings.TrimSpace(workspaceID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workspaces, workspaceID)
	delete(s.snapshots, workspaceID)
	delete(s.workspaceRevisions, workspaceID)
	delete(s.workspaceSources, workspaceID)
	delete(s.staleWorkspaces, workspaceID)
	nextOrder := s.workspaceOrder[:0]
	for _, id := range s.workspaceOrder {
		if id != workspaceID {
			nextOrder = append(nextOrder, id)
		}
	}
	s.workspaceOrder = nextOrder
	s.globalRevision++
	s.globalStale = false
	s.state = StateReady
	s.errText = ""
	s.updatedAt = time.Now().UTC()
}

func (s *Store) List() ([]appmodelcore.Workspace, Status, error) {
	if s == nil {
		return nil, Status{State: StateFailed}, common.ErrConflict
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := s.statusLocked()
	if s.state == StateBuilding || s.state == StateStopped {
		return nil, status, fmt.Errorf("%w: workspace projection %s", common.ErrConflict, s.state)
	}
	if s.state == StateFailed {
		return nil, status, fmt.Errorf("%w: workspace projection failed: %s", common.ErrConflict, s.errText)
	}
	ret := make([]appmodelcore.Workspace, 0, len(s.workspaceOrder))
	for _, id := range s.workspaceOrder {
		if item, ok := s.workspaces[id]; ok {
			ret = append(ret, cloneWorkspace(item))
		}
	}
	return ret, status, nil
}

func (s *Store) GetSnapshot(workspaceID string) (readmodelcore.WorkspaceSnapshot, Status, error) {
	if s == nil {
		return readmodelcore.WorkspaceSnapshot{}, Status{State: StateFailed}, common.ErrConflict
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := s.statusForWorkspaceLocked(workspaceID)
	if s.state == StateBuilding || s.state == StateStopped {
		return readmodelcore.WorkspaceSnapshot{}, status, fmt.Errorf("%w: workspace projection %s", common.ErrConflict, s.state)
	}
	if s.state == StateFailed {
		return readmodelcore.WorkspaceSnapshot{}, status, fmt.Errorf("%w: workspace projection failed: %s", common.ErrConflict, s.errText)
	}
	item, ok := s.snapshots[workspaceID]
	if !ok {
		if _, exists := s.workspaces[workspaceID]; exists {
			return readmodelcore.WorkspaceSnapshot{}, status, fmt.Errorf("%w: workspace projection detail not ready", common.ErrConflict)
		}
		return readmodelcore.WorkspaceSnapshot{}, status, common.ErrNotFound
	}
	return cloneWorkspaceSnapshot(item), status, nil
}

func formatProjectionTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func cloneWorkspace(item appmodelcore.Workspace) appmodelcore.Workspace {
	item.SoulBindings = cloneStringStringMap(item.SoulBindings)
	item.ContextHints = cloneStringAnyMap(item.ContextHints)
	item.EvidencePolicies = cloneMapSlice(item.EvidencePolicies)
	return item
}

func cloneWorkspaceSnapshot(item readmodelcore.WorkspaceSnapshot) readmodelcore.WorkspaceSnapshot {
	item.Workspace = cloneWorkspace(item.Workspace)
	item.Goals = append([]readmodelcore.Goal(nil), item.Goals...)
	item.DoDs = append([]readmodelcore.DoD(nil), item.DoDs...)
	item.Runs = append([]readmodelcore.Run(nil), item.Runs...)
	item.Results = append([]readmodelcore.Result(nil), item.Results...)
	item.Artifacts = append([]readmodelcore.Artifact(nil), item.Artifacts...)
	item.ArtifactContinuations = append([]readmodelcore.ArtifactContinuation(nil), item.ArtifactContinuations...)
	item.Patches = append([]readmodelcore.Patch(nil), item.Patches...)
	item.WebSources = append([]readmodelcore.WebSourceTrace(nil), item.WebSources...)
	item.LLMContextBudgets = append([]readmodelcore.LLMContextBudgetTrace(nil), item.LLMContextBudgets...)
	item.LLMUsageTraces = append([]readmodelcore.LLMUsageTrace(nil), item.LLMUsageTraces...)
	item.EventRecords = append([]readmodelcore.EventRecord(nil), item.EventRecords...)
	item.Decisions = append([]readmodelcore.Decision(nil), item.Decisions...)
	item.Approvals = append([]readmodelcore.Approval(nil), item.Approvals...)
	item.TaskDecompositions = append([]readmodelcore.TaskDecomposition(nil), item.TaskDecompositions...)
	item.ExecutionSections = append([]readmodelcore.ExecutionSection(nil), item.ExecutionSections...)
	item.MemoryRecallTraces = append([]readmodelcore.MemoryRecallTrace(nil), item.MemoryRecallTraces...)
	if item.ContextBundle != nil {
		bundle := cloneContextBundle(*item.ContextBundle)
		item.ContextBundle = &bundle
	}
	return item
}

func cloneContextBundle(item readmodelcore.ContextBundle) readmodelcore.ContextBundle {
	item.Sections = append([]readmodelcore.ContextSection(nil), item.Sections...)
	for idx := range item.Sections {
		item.Sections[idx].EvidenceRefs = append([]string(nil), item.Sections[idx].EvidenceRefs...)
		item.Sections[idx].Data = cloneStringAnyMap(item.Sections[idx].Data)
	}
	item.RecentEvidence = append([]readmodelcore.ContextEvidence(nil), item.RecentEvidence...)
	for idx := range item.RecentEvidence {
		item.RecentEvidence[idx].EvidenceRefs = append([]string(nil), item.RecentEvidence[idx].EvidenceRefs...)
		item.RecentEvidence[idx].Data = cloneStringAnyMap(item.RecentEvidence[idx].Data)
	}
	item.CompressionCandidates = append([]readmodelcore.ContextCompressionCandidate(nil), item.CompressionCandidates...)
	for idx := range item.CompressionCandidates {
		item.CompressionCandidates[idx].SourceRefs = append([]string(nil), item.CompressionCandidates[idx].SourceRefs...)
	}
	item.CompressedContext.Milestones = append([]readmodelcore.ProgressMilestone(nil), item.CompressedContext.Milestones...)
	item.CompressedContext.Blockers = append([]readmodelcore.ProgressBlocker(nil), item.CompressedContext.Blockers...)
	item.CompressedContext.RecentEvidence = append([]readmodelcore.ContextEvidence(nil), item.CompressedContext.RecentEvidence...)
	for idx := range item.CompressedContext.RecentEvidence {
		item.CompressedContext.RecentEvidence[idx].EvidenceRefs = append([]string(nil), item.CompressedContext.RecentEvidence[idx].EvidenceRefs...)
		item.CompressedContext.RecentEvidence[idx].Data = cloneStringAnyMap(item.CompressedContext.RecentEvidence[idx].Data)
	}
	item.CompressedContext.ResultRefs = append([]string(nil), item.CompressedContext.ResultRefs...)
	item.CompressedContext.SourceSectionRefs = append([]string(nil), item.CompressedContext.SourceSectionRefs...)
	item.CompressedContext.SourceEvidenceRefs = append([]string(nil), item.CompressedContext.SourceEvidenceRefs...)
	return item
}

func cloneStringStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneStringAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneMapSlice(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, cloneStringAnyMap(item))
	}
	return ret
}
