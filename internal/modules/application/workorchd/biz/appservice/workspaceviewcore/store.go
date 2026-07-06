package workspaceviewcore

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/common"
)

type Store struct {
	mu                      sync.RWMutex
	state                   string
	errText                 string
	globalRevision          int64
	sourceRevision          int64
	projectedSourceRevision int64
	updatedAt               time.Time
	rebuiltAt               time.Time
	workspaceOrder          []string
	workspaces              map[string]*workspaceProjectionEntry
	globalStale             bool
	globalStaleReason       string
}

type workspaceProjectionEntry struct {
	Workspace               appmodelcore.Workspace
	HasWorkspace            bool
	Snapshot                readmodelcore.WorkspaceSnapshot
	HasSnapshot             bool
	Revision                int64
	SourceRevision          int64
	ProjectedSourceRevision int64
	Stale                   bool
	StaleReason             string
	Error                   string
	RebuiltAt               time.Time
}

func (e *workspaceProjectionEntry) syncSnapshotWorkspace() {
	if e == nil || !e.HasSnapshot || !e.HasWorkspace {
		return
	}
	e.Snapshot.Workspace = workspaceWithSnapshotGovernance(e.Workspace, e.Snapshot)
	e.Snapshot = readmodelprojection.NormalizeWorkspaceGovernanceProjection(e.Snapshot)
	e.Workspace = cloneWorkspace(e.Snapshot.Workspace)
}

func workspaceWithSnapshotGovernance(workspace appmodelcore.Workspace, snapshot readmodelcore.WorkspaceSnapshot) appmodelcore.Workspace {
	next := cloneWorkspace(workspace)
	if strings.TrimSpace(next.ID) == "" {
		next.ID = snapshot.Workspace.ID
	}
	activeRunID := strings.TrimSpace(next.ActiveRunID)
	if activeRunID == "" && snapshot.Run != nil {
		activeRunID = strings.TrimSpace(snapshot.Run.ID)
	}
	blockerRunID := activeRunID
	if blockerRunID == "" {
		blockerRunID = strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	}
	if ticketID, ok := snapshotDecisionBlocker(snapshot, blockerRunID); ok {
		next.State = string(common.WorkspaceStateActive)
		if blockerRunID != "" {
			next.ActiveRunID = blockerRunID
		}
		next.BlockingTicketID = ticketID
		next.BlockingApprovalID = ""
		return next
	}
	if approvalID, ok := snapshotApprovalBlocker(snapshot, blockerRunID); ok {
		next.State = string(common.WorkspaceStateBlockedOnApproval)
		if blockerRunID != "" {
			next.ActiveRunID = blockerRunID
		}
		next.BlockingApprovalID = approvalID
		next.BlockingTicketID = ""
		return next
	}
	if activeRunID != "" {
		next.ActiveRunID = activeRunID
	}
	return next
}

func snapshotDecisionBlocker(snapshot readmodelcore.WorkspaceSnapshot, activeRunID string) (string, bool) {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	activeRunID = strings.TrimSpace(activeRunID)
	if activeRunID == "" && snapshot.Run != nil {
		activeRunID = strings.TrimSpace(snapshot.Run.ID)
	}
	preferredID := firstNonEmptyViewString(snapshot.Workspace.BlockingTicketID)
	if snapshot.Run != nil {
		preferredID = firstNonEmptyViewString(preferredID, snapshot.Run.BlockingTicketID)
	}
	if readmodelprojection.HasOpenDecisionBlocker(snapshot.Decisions, workspaceID, activeRunID, preferredID) {
		return preferredID, true
	}
	for _, item := range snapshot.Decisions {
		if strings.TrimSpace(item.WorkspaceID) == workspaceID &&
			strings.TrimSpace(item.RunID) == activeRunID &&
			strings.TrimSpace(item.State) == string(common.DecisionTicketStateOpen) &&
			strings.TrimSpace(item.ID) != "" {
			return strings.TrimSpace(item.ID), true
		}
	}
	return "", false
}

func snapshotApprovalBlocker(snapshot readmodelcore.WorkspaceSnapshot, activeRunID string) (string, bool) {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	activeRunID = strings.TrimSpace(activeRunID)
	if activeRunID == "" && snapshot.Run != nil {
		activeRunID = strings.TrimSpace(snapshot.Run.ID)
	}
	preferredID := firstNonEmptyViewString(snapshot.Workspace.BlockingApprovalID)
	if snapshot.Run != nil {
		preferredID = firstNonEmptyViewString(preferredID, snapshot.Run.BlockingApprovalID)
	}
	if readmodelprojection.HasOpenApprovalBlocker(snapshot.Approvals, workspaceID, activeRunID, preferredID) {
		return preferredID, true
	}
	for _, item := range snapshot.Approvals {
		if strings.TrimSpace(item.WorkspaceID) == workspaceID &&
			strings.TrimSpace(item.RunID) == activeRunID &&
			strings.TrimSpace(item.State) == string(common.ApprovalStatePending) &&
			strings.TrimSpace(item.ID) != "" {
			return strings.TrimSpace(item.ID), true
		}
	}
	return "", false
}

func firstNonEmptyViewString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func NewStore() *Store {
	return &Store{
		state:      StateBuilding,
		workspaces: map[string]*workspaceProjectionEntry{},
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
	degraded := s.state == StateDegraded || s.hasWorkspaceErrorLocked()
	if state == StateReady && degraded {
		state = StateDegraded
	} else if state == StateReady && (s.globalStale || s.hasStaleWorkspaceLocked()) {
		state = StateStale
	}
	return Status{
		State:                   state,
		GlobalRevision:          s.globalRevision,
		SourceEventRevision:     s.sourceRevision,
		ProjectedSourceRevision: s.projectedSourceRevision,
		WorkspaceTotal:          s.workspaceTotalLocked(),
		UpdatedAt:               formatProjectionTime(s.updatedAt),
		RebuiltAt:               formatProjectionTime(s.rebuiltAt),
		Degraded:                degraded,
		StaleReason:             s.globalStaleReason,
		Error:                   s.errText,
	}
}

func (s *Store) statusForWorkspaceLocked(workspaceID string) Status {
	if workspaceID == "" {
		return s.statusLocked()
	}
	state := s.state
	if state == StateDegraded || state == StateFailed {
		state = StateReady
	}
	entry := s.workspaces[workspaceID]
	if (entry == nil || !entry.HasWorkspace) && s.state != StateBuilding && s.state != StateStopped {
		state = StateFailed
	}
	var errText, staleReason string
	var workspaceRevision, sourceRevision, projectedSourceRevision int64
	var rebuiltAt time.Time
	var stale bool
	if entry != nil {
		errText = entry.Error
		staleReason = entry.StaleReason
		workspaceRevision = entry.Revision
		sourceRevision = entry.SourceRevision
		projectedSourceRevision = entry.ProjectedSourceRevision
		rebuiltAt = entry.RebuiltAt
		stale = entry.Stale
	}
	if sourceRevision == 0 {
		sourceRevision = s.sourceRevision
	}
	if projectedSourceRevision == 0 {
		projectedSourceRevision = s.projectedSourceRevision
		if !stale && (projectedSourceRevision == 0 || (strings.TrimSpace(errText) == "" && projectedSourceRevision < sourceRevision)) {
			projectedSourceRevision = sourceRevision
		}
	}
	degraded := strings.TrimSpace(errText) != ""
	if degraded {
		state = StateDegraded
	} else if stale && state == StateReady {
		state = StateStale
	}
	return Status{
		State:                   state,
		GlobalRevision:          s.globalRevision,
		WorkspaceRevision:       workspaceRevision,
		SourceEventRevision:     sourceRevision,
		ProjectedSourceRevision: projectedSourceRevision,
		WorkspaceTotal:          s.workspaceTotalLocked(),
		UpdatedAt:               formatProjectionTime(s.updatedAt),
		RebuiltAt:               formatProjectionTime(rebuiltAt),
		Degraded:                degraded,
		StaleReason:             staleReason,
		Error:                   errText,
	}
}

func (s *Store) setBuilding() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workspaceTotalLocked() > 0 {
		s.state = StateStale
	} else {
		s.state = StateBuilding
	}
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
		s.globalStaleReason = "source_event"
	} else {
		entry := s.entryForUpdateLocked(workspaceID)
		entry.Stale = true
		entry.StaleReason = "source_event"
		if sourceRevision > entry.SourceRevision {
			entry.SourceRevision = sourceRevision
		}
	}
	s.updatedAt = time.Now().UTC()
}

func (s *Store) ReplaceAll(workspaces []appmodelcore.Workspace, snapshots map[string]readmodelcore.WorkspaceSnapshot, degraded error) {
	if s == nil {
		return
	}
	now := time.Now().UTC()
	nextWorkspaces := make(map[string]*workspaceProjectionEntry, len(workspaces))
	nextOrder := make([]string, 0, len(workspaces))
	for _, item := range workspaces {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		entry := s.cloneEntryForReplace(item.ID)
		if snapshot, ok := snapshots[item.ID]; ok && strings.TrimSpace(snapshot.Workspace.ID) != "" {
			snapshot = readmodelprojection.ProjectWorkspaceSnapshotDetail(snapshot)
			item = snapshot.Workspace
			entry.Snapshot = cloneWorkspaceSnapshot(snapshot)
			entry.HasSnapshot = true
			entry.ProjectedSourceRevision = entry.SourceRevision
			entry.RebuiltAt = now
			entry.Stale = false
			entry.StaleReason = ""
			entry.Error = ""
		} else if degraded != nil {
			entry.Stale = true
			entry.StaleReason = "rebuild_failed"
			entry.Error = degraded.Error()
		}
		entry.Workspace = cloneWorkspace(item)
		entry.HasWorkspace = true
		entry.Revision++
		nextWorkspaces[item.ID] = entry
		nextOrder = append(nextOrder, item.ID)
	}
	sort.Strings(nextOrder)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaces = nextWorkspaces
	s.workspaceOrder = nextOrder
	s.globalRevision++
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
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

func (s *Store) ReplaceWorkspaceList(workspaces []appmodelcore.Workspace, degraded error) {
	if s == nil {
		return
	}
	now := time.Now().UTC()
	nextWorkspaces := make(map[string]*workspaceProjectionEntry, len(workspaces))
	nextOrder := make([]string, 0, len(workspaces))
	for _, item := range workspaces {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		entry := s.cloneEntryForReplace(item.ID)
		entry.Workspace = cloneWorkspace(item)
		entry.HasWorkspace = true
		entry.syncSnapshotWorkspace()
		entry.Revision++
		nextWorkspaces[item.ID] = entry
		nextOrder = append(nextOrder, item.ID)
	}
	sort.Strings(nextOrder)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaces = nextWorkspaces
	s.workspaceOrder = nextOrder
	s.globalRevision++
	s.globalStale = false
	s.globalStaleReason = ""
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
	snapshot = readmodelprojection.ProjectWorkspaceSnapshotDetail(snapshot)
	workspaceID := snapshot.Workspace.ID
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	entry := s.entryForUpdateLocked(workspaceID)
	if snapshot.RunCommand == nil && entry.HasSnapshot && entry.Snapshot.RunCommand != nil {
		command := *entry.Snapshot.RunCommand
		snapshot.RunCommand = &command
	}
	entry.Workspace = cloneWorkspace(snapshot.Workspace)
	entry.HasWorkspace = true
	if !stringSliceContains(s.workspaceOrder, workspaceID) {
		s.workspaceOrder = append(s.workspaceOrder, workspaceID)
		sort.Strings(s.workspaceOrder)
	}
	entry.Snapshot = cloneWorkspaceSnapshot(snapshot)
	entry.HasSnapshot = true
	entry.Revision++
	s.globalRevision++
	entry.ProjectedSourceRevision = entry.SourceRevision
	entry.RebuiltAt = now
	entry.Stale = false
	entry.StaleReason = ""
	entry.Error = ""
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	s.state = StateReady
	s.errText = ""
	s.updatedAt = now
}

func (s *Store) UpsertWorkspace(workspace appmodelcore.Workspace) {
	if s == nil || strings.TrimSpace(workspace.ID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	entry := s.entryForUpdateLocked(workspace.ID)
	entry.Workspace = cloneWorkspace(workspace)
	entry.HasWorkspace = true
	entry.syncSnapshotWorkspace()
	if !stringSliceContains(s.workspaceOrder, workspace.ID) {
		s.workspaceOrder = append(s.workspaceOrder, workspace.ID)
		sort.Strings(s.workspaceOrder)
	}
	entry.Revision++
	s.globalRevision++
	entry.ProjectedSourceRevision = entry.SourceRevision
	entry.RebuiltAt = now
	entry.Stale = false
	entry.StaleReason = ""
	entry.Error = ""
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	s.state = StateReady
	s.errText = ""
	s.updatedAt = now
}

func (s *Store) ApplyRuntimeProgress(update RuntimeProgressUpdate) bool {
	if s == nil || strings.TrimSpace(update.WorkspaceID) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.workspaces[update.WorkspaceID]
	if entry == nil || !entry.HasSnapshot {
		return false
	}
	now := time.Now().UTC()
	snapshot := entry.Snapshot
	if !applyRuntimeProgressToSnapshot(&snapshot, update, now) {
		return false
	}
	entry.Snapshot = snapshot
	entry.Workspace = cloneWorkspace(snapshot.Workspace)
	entry.HasWorkspace = true
	entry.HasSnapshot = true
	entry.Revision++
	s.globalRevision++
	if update.Revision > entry.SourceRevision {
		entry.SourceRevision = update.Revision
	}
	entry.ProjectedSourceRevision = entry.SourceRevision
	entry.RebuiltAt = now
	entry.Stale = false
	entry.StaleReason = ""
	entry.Error = ""
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	s.state = StateReady
	s.errText = ""
	s.updatedAt = now
	return true
}

func (s *Store) ApplyRunCommandAccepted(update RunCommandAcceptedUpdate) bool {
	if s == nil || strings.TrimSpace(update.WorkspaceID) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.workspaces[update.WorkspaceID]
	if entry == nil || !entry.HasWorkspace {
		return false
	}
	now := time.Now().UTC()
	snapshot := entry.Snapshot
	if !entry.HasSnapshot || strings.TrimSpace(snapshot.Workspace.ID) == "" {
		snapshot.Workspace = cloneWorkspace(entry.Workspace)
	}
	acceptedAt := strings.TrimSpace(update.AcceptedAt)
	if acceptedAt == "" {
		acceptedAt = formatProjectionTime(now)
	}
	snapshot.RunCommand = &readmodelcore.RunCommandStatus{
		WorkspaceID:        strings.TrimSpace(update.WorkspaceID),
		RunID:              strings.TrimSpace(update.RunID),
		State:              "accepted",
		Intent:             strings.TrimSpace(update.Intent),
		Reason:             strings.TrimSpace(update.Reason),
		RequestID:          strings.TrimSpace(update.RequestID),
		IdempotencyKey:     strings.TrimSpace(update.IdempotencyKey),
		Source:             strings.TrimSpace(update.Source),
		DefinitionRevision: strings.TrimSpace(update.DefinitionRevision),
		DecisionID:         strings.TrimSpace(update.DecisionID),
		AcceptedAt:         acceptedAt,
		Revision:           update.Revision,
	}
	entry.Snapshot = snapshot
	entry.Workspace = cloneWorkspace(snapshot.Workspace)
	entry.HasWorkspace = true
	entry.HasSnapshot = true
	entry.Revision++
	s.globalRevision++
	if update.Revision > entry.SourceRevision {
		entry.SourceRevision = update.Revision
	}
	entry.ProjectedSourceRevision = entry.SourceRevision
	entry.RebuiltAt = now
	entry.Stale = false
	entry.StaleReason = ""
	entry.Error = ""
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	s.state = StateReady
	s.errText = ""
	s.updatedAt = now
	return true
}

func (s *Store) ApplyRunCommandRejected(update RunCommandRejectedUpdate) bool {
	if s == nil || strings.TrimSpace(update.WorkspaceID) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.workspaces[update.WorkspaceID]
	if entry == nil || !entry.HasWorkspace {
		return false
	}
	now := time.Now().UTC()
	snapshot := entry.Snapshot
	if !entry.HasSnapshot || strings.TrimSpace(snapshot.Workspace.ID) == "" {
		snapshot.Workspace = cloneWorkspace(entry.Workspace)
	}
	rejectedAt := strings.TrimSpace(update.RejectedAt)
	if rejectedAt == "" {
		rejectedAt = formatProjectionTime(now)
	}
	snapshot.RunCommand = &readmodelcore.RunCommandStatus{
		WorkspaceID:        strings.TrimSpace(update.WorkspaceID),
		RunID:              strings.TrimSpace(update.RunID),
		State:              "rejected",
		Intent:             strings.TrimSpace(update.Intent),
		Reason:             strings.TrimSpace(update.Reason),
		RequestID:          strings.TrimSpace(update.RequestID),
		IdempotencyKey:     strings.TrimSpace(update.IdempotencyKey),
		Source:             strings.TrimSpace(update.Source),
		DefinitionRevision: strings.TrimSpace(update.DefinitionRevision),
		DecisionID:         strings.TrimSpace(update.DecisionID),
		RejectedAt:         rejectedAt,
		Error:              strings.TrimSpace(update.Error),
		Revision:           update.Revision,
	}
	entry.Snapshot = snapshot
	entry.Workspace = cloneWorkspace(snapshot.Workspace)
	entry.HasWorkspace = true
	entry.HasSnapshot = true
	entry.Revision++
	s.globalRevision++
	if update.Revision > entry.SourceRevision {
		entry.SourceRevision = update.Revision
	}
	entry.ProjectedSourceRevision = entry.SourceRevision
	entry.RebuiltAt = now
	entry.Stale = false
	entry.StaleReason = ""
	entry.Error = ""
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	s.state = StateReady
	s.errText = ""
	s.updatedAt = now
	return true
}

func (s *Store) MarkWorkspaceDegraded(workspaceID string, err error) {
	if s == nil || strings.TrimSpace(workspaceID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entryForUpdateLocked(workspaceID)
	if err != nil {
		entry.Error = err.Error()
	}
	entry.Stale = true
	entry.StaleReason = "rebuild_failed"
	s.globalRevision++
	s.state = StateReady
	s.updatedAt = time.Now().UTC()
}

func (s *Store) DeleteWorkspace(workspaceID string) {
	if s == nil || strings.TrimSpace(workspaceID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workspaces, workspaceID)
	nextOrder := s.workspaceOrder[:0]
	for _, id := range s.workspaceOrder {
		if id != workspaceID {
			nextOrder = append(nextOrder, id)
		}
	}
	s.workspaceOrder = nextOrder
	s.globalRevision++
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
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
		if entry := s.workspaces[id]; entry != nil && entry.HasWorkspace {
			item := cloneWorkspace(entry.Workspace)
			if entry.HasSnapshot {
				snapshot := normalizedEntryListSnapshot(entry)
				item = cloneWorkspace(snapshot.Workspace)
				item = compactWorkspaceListOverview(item)
				item.State = snapshot.Workspace.State
				item.BlockingTicketID = snapshot.Workspace.BlockingTicketID
				item.BlockingApprovalID = snapshot.Workspace.BlockingApprovalID
				item.ViewModel = appWorkspaceViewModelFromSnapshot(snapshot, s.statusForWorkspaceLocked(id))
			} else {
				item = compactWorkspaceListOverview(item)
				item.ViewModel = appWorkspaceViewModelFromStatus(item, s.statusForWorkspaceLocked(id))
			}
			ret = append(ret, item)
		}
	}
	return ret, status, nil
}

func compactWorkspaceListOverview(item appmodelcore.Workspace) appmodelcore.Workspace {
	item.ContextHints = nil
	item.EvidencePolicies = nil
	return item
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
	entry := s.workspaces[workspaceID]
	if entry == nil || !entry.HasSnapshot {
		if entry != nil && entry.HasWorkspace {
			return readmodelcore.WorkspaceSnapshot{}, status, fmt.Errorf("%w: workspace projection detail not ready", common.ErrConflict)
		}
		return readmodelcore.WorkspaceSnapshot{}, status, common.ErrNotFound
	}
	snapshot := normalizedEntrySnapshot(entry)
	snapshot.ViewModel = workspaceViewModelFromStatus(snapshot, status)
	return snapshot, status, nil
}

func (s *Store) GetSummarySnapshot(workspaceID string) (readmodelcore.WorkspaceSnapshot, Status, error) {
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
	entry := s.workspaces[workspaceID]
	if entry == nil || !entry.HasWorkspace {
		return readmodelcore.WorkspaceSnapshot{}, status, common.ErrNotFound
	}
	snapshot := readmodelcore.WorkspaceSnapshot{Workspace: cloneWorkspace(entry.Workspace)}
	if entry.HasSnapshot {
		snapshot = normalizedEntrySummarySnapshot(entry)
	}
	snapshot.ViewModel = workspaceViewModelFromStatus(snapshot, status)
	return snapshot, status, nil
}

func normalizedEntryListSnapshot(entry *workspaceProjectionEntry) readmodelcore.WorkspaceSnapshot {
	if entry == nil {
		return readmodelcore.WorkspaceSnapshot{}
	}
	snapshot := readmodelcore.WorkspaceSnapshot{Workspace: cloneWorkspace(entry.Workspace)}
	if !entry.HasSnapshot {
		return snapshot
	}
	source := entry.Snapshot
	snapshot.Workspace = cloneWorkspace(source.Workspace)
	snapshot.Run = source.Run
	snapshot.RunHeartbeat = source.RunHeartbeat
	snapshot.TaskProgress = source.TaskProgress
	snapshot.Decisions = append([]readmodelcore.Decision(nil), source.Decisions...)
	snapshot.Approvals = append([]readmodelcore.Approval(nil), source.Approvals...)
	if entry.HasWorkspace {
		snapshot.Workspace = workspaceWithSnapshotGovernance(entry.Workspace, snapshot)
	}
	return readmodelprojection.NormalizeWorkspaceGovernanceProjection(snapshot)
}

func normalizedEntrySummarySnapshot(entry *workspaceProjectionEntry) readmodelcore.WorkspaceSnapshot {
	snapshot := normalizedEntryListSnapshot(entry)
	if entry == nil || !entry.HasSnapshot {
		return readmodelprojection.ProjectWorkspaceSnapshotSummary(snapshot)
	}
	source := entry.Snapshot
	snapshot.Outcome = source.Outcome
	snapshot.CoverageSummary = cloneStringAnyMap(source.CoverageSummary)
	snapshot.ConvergenceStatus = source.ConvergenceStatus
	if source.RunCommand != nil {
		status := *source.RunCommand
		snapshot.RunCommand = &status
	}
	snapshot.SchedulerDiagnostics = source.SchedulerDiagnostics
	snapshot.LLMUsageSummary = source.LLMUsageSummary
	return readmodelprojection.ProjectWorkspaceSnapshotSummary(snapshot)
}

func normalizedEntrySnapshot(entry *workspaceProjectionEntry) readmodelcore.WorkspaceSnapshot {
	if entry == nil || !entry.HasSnapshot {
		return readmodelcore.WorkspaceSnapshot{}
	}
	snapshot := cloneWorkspaceSnapshot(entry.Snapshot)
	if entry.HasWorkspace {
		snapshot.Workspace = workspaceWithSnapshotGovernance(entry.Workspace, snapshot)
	}
	return readmodelprojection.NormalizeWorkspaceGovernanceProjection(snapshot)
}

func (s *Store) NeedsSnapshot(workspaceID string) bool {
	if s == nil {
		return false
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state == StateBuilding || s.state == StateStopped || s.state == StateFailed {
		return false
	}
	entry := s.workspaces[workspaceID]
	return entry != nil && entry.HasWorkspace && !entry.HasSnapshot
}

func workspaceViewModelFromStatus(snapshot readmodelcore.WorkspaceSnapshot, status Status) readmodelcore.WorkspaceViewModel {
	revision := status.WorkspaceRevision
	if revision == 0 {
		revision = status.GlobalRevision
	}
	ret := readmodelcore.WorkspaceViewModel{
		WorkspaceID:             snapshot.Workspace.ID,
		State:                   status.State,
		PromptMode:              workspacePromptMode(snapshot),
		Revision:                revision,
		GlobalRevision:          status.GlobalRevision,
		WorkspaceRevision:       status.WorkspaceRevision,
		SourceEventRevision:     status.SourceEventRevision,
		ProjectedSourceRevision: status.ProjectedSourceRevision,
		UpdatedAt:               status.UpdatedAt,
		RebuiltAt:               status.RebuiltAt,
		Degraded:                status.Degraded,
		StaleReason:             status.StaleReason,
		Error:                   status.Error,
		Sections:                map[string]readmodelcore.WorkspaceViewModelSection{},
	}
	for _, name := range []string{
		"list",
		"activeWorkspace",
		"runState",
		"promptMode",
		"commands",
		"coverage",
		"currentFocus",
		"latestDiagnostic",
	} {
		ret.Sections[name] = readmodelcore.WorkspaceViewModelSection{
			Name:                    name,
			Revision:                revision,
			WorkspaceRevision:       status.WorkspaceRevision,
			SourceEventRevision:     status.SourceEventRevision,
			ProjectedSourceRevision: status.ProjectedSourceRevision,
			State:                   status.State,
		}
	}
	return ret
}

func appWorkspaceViewModelFromStatus(workspace appmodelcore.Workspace, status Status) appmodelcore.WorkspaceViewModel {
	source := workspaceViewModelFromStatus(readmodelcore.WorkspaceSnapshot{Workspace: workspace}, status)
	return appWorkspaceViewModelFromReadModel(source)
}

func appWorkspaceViewModelFromSnapshot(snapshot readmodelcore.WorkspaceSnapshot, status Status) appmodelcore.WorkspaceViewModel {
	source := workspaceViewModelFromStatus(snapshot, status)
	return appWorkspaceViewModelFromReadModel(source)
}

func appWorkspaceViewModelFromReadModel(source readmodelcore.WorkspaceViewModel) appmodelcore.WorkspaceViewModel {
	ret := appmodelcore.WorkspaceViewModel{
		WorkspaceID:             source.WorkspaceID,
		State:                   source.State,
		PromptMode:              source.PromptMode,
		Revision:                source.Revision,
		GlobalRevision:          source.GlobalRevision,
		WorkspaceRevision:       source.WorkspaceRevision,
		SourceEventRevision:     source.SourceEventRevision,
		ProjectedSourceRevision: source.ProjectedSourceRevision,
		UpdatedAt:               source.UpdatedAt,
		RebuiltAt:               source.RebuiltAt,
		Degraded:                source.Degraded,
		StaleReason:             source.StaleReason,
		Error:                   source.Error,
	}
	if len(source.Sections) > 0 {
		ret.Sections = make(map[string]appmodelcore.WorkspaceViewModelSection, len(source.Sections))
		for key, value := range source.Sections {
			ret.Sections[key] = appmodelcore.WorkspaceViewModelSection{
				Name:                    value.Name,
				Revision:                value.Revision,
				WorkspaceRevision:       value.WorkspaceRevision,
				SourceEventRevision:     value.SourceEventRevision,
				ProjectedSourceRevision: value.ProjectedSourceRevision,
				State:                   value.State,
			}
		}
	}
	return ret
}

func workspacePromptMode(snapshot readmodelcore.WorkspaceSnapshot) string {
	return readmodelprojection.WorkspacePromptMode(snapshot)
}

func (s *Store) RunHeartbeatMetrics(now time.Time) Metrics {
	if s == nil {
		return Metrics{}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ret Metrics
	for _, entry := range s.workspaces {
		if entry == nil || !entry.HasSnapshot {
			continue
		}
		addConvergenceMetrics(&ret, entry.Snapshot.Acceptance)
		heartbeat := entry.Snapshot.RunHeartbeat
		if strings.TrimSpace(heartbeat.RunID) == "" || !activeRunHeartbeatState(heartbeat.State) {
			continue
		}
		ret.ActiveRunHeartbeatTotal++
		if heartbeat.Stalled || strings.TrimSpace(heartbeat.ActivityState) == "stalled" {
			ret.StalledRunTotal++
		}
		if heartbeat.IneffectiveRetry || strings.TrimSpace(heartbeat.ActivityState) == "ineffective_retry" {
			ret.IneffectiveRetryRunTotal++
		}
		if age := runHeartbeatAgeSeconds(heartbeat, now); age > ret.ActiveRunHeartbeatAgeSecondsMax {
			ret.ActiveRunHeartbeatAgeSecondsMax = age
		}
	}
	return ret
}

func addConvergenceMetrics(ret *Metrics, acceptance readmodelcore.AcceptanceProjection) {
	if ret == nil {
		return
	}
	ret.ConvergenceActiveSlices += acceptance.Summary.ActiveSlices
	if acceptance.CurrentFocus != nil && activeCoverageFocusState(acceptance.CurrentFocus.State) {
		ret.ConvergenceOpenFocuses++
	}
	if acceptance.Budget != nil {
		ret.ConvergenceRepairAttemptsTotal += acceptance.Budget.RepairAttempts
		ret.ConvergenceNoDeltaRoundsTotal += acceptance.Budget.NoCoverageDeltaRounds
		if convergenceBudgetEscalated(acceptance.Budget.Decision) {
			ret.ConvergenceBudgetEscalations++
		}
		return
	}
	if acceptance.CurrentFocus != nil {
		ret.ConvergenceRepairAttemptsTotal += acceptance.CurrentFocus.Attempt
	}
}

func activeCoverageFocusState(state string) bool {
	state = strings.TrimSpace(state)
	return state == "" || state == "active" || state == "pending" || state == "blocked"
}

func convergenceBudgetEscalated(decision string) bool {
	switch strings.TrimSpace(decision) {
	case "", "resume":
		return false
	default:
		return true
	}
}

func (s *Store) cloneEntryForReplace(workspaceID string) *workspaceProjectionEntry {
	if s == nil {
		return &workspaceProjectionEntry{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if entry := s.workspaces[workspaceID]; entry != nil {
		return cloneWorkspaceProjectionEntry(entry)
	}
	return &workspaceProjectionEntry{}
}

func (s *Store) entryForUpdateLocked(workspaceID string) *workspaceProjectionEntry {
	entry := s.workspaces[workspaceID]
	if entry == nil {
		entry = &workspaceProjectionEntry{}
		s.workspaces[workspaceID] = entry
	}
	return entry
}

func (s *Store) workspaceTotalLocked() int {
	total := 0
	for _, entry := range s.workspaces {
		if entry != nil && entry.HasWorkspace {
			total++
		}
	}
	return total
}

func (s *Store) hasWorkspaceErrorLocked() bool {
	for _, entry := range s.workspaces {
		if entry != nil && strings.TrimSpace(entry.Error) != "" {
			return true
		}
	}
	return false
}

func (s *Store) hasStaleWorkspaceLocked() bool {
	for _, entry := range s.workspaces {
		if entry != nil && entry.Stale {
			return true
		}
	}
	return false
}

func (s *Store) refreshProjectedSourceRevisionLocked() {
	var source, projected int64
	if s.sourceRevision > source {
		source = s.sourceRevision
	}
	for _, entry := range s.workspaces {
		if entry == nil {
			continue
		}
		if entry.SourceRevision > source {
			source = entry.SourceRevision
		}
		if entry.ProjectedSourceRevision > projected {
			projected = entry.ProjectedSourceRevision
		}
	}
	if projected > source {
		source = projected
	}
	s.sourceRevision = source
	s.projectedSourceRevision = projected
}

func formatProjectionTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func activeRunHeartbeatState(state string) bool {
	switch common.RunState(strings.TrimSpace(state)) {
	case common.RunStateCompleted,
		common.RunStateFailed,
		common.RunStateCancelled,
		common.RunStateSuperseded,
		common.RunStateBlockedOnDecision,
		common.RunStateBlockedOnApproval:
		return false
	default:
		return true
	}
}

func runHeartbeatAgeSeconds(heartbeat readmodelcore.RunHeartbeat, now time.Time) int64 {
	for _, candidate := range []string{
		heartbeat.LatestEffectiveProgressAt,
		heartbeat.LatestEventAt,
		heartbeat.LatestLLMUsageAt,
		heartbeat.LatestResultAt,
		heartbeat.LatestArtifactAt,
		heartbeat.LatestPatchAt,
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, candidate)
		if err != nil {
			continue
		}
		age := now.Sub(parsed.UTC()).Seconds()
		if age < 0 {
			return 0
		}
		return int64(age)
	}
	return 0
}

func applyRuntimeProgressToSnapshot(snapshot *readmodelcore.WorkspaceSnapshot, update RuntimeProgressUpdate, now time.Time) bool {
	if snapshot == nil {
		return false
	}
	if update.RunID != "" && snapshot.Run != nil && snapshot.Run.ID != update.RunID {
		return false
	}
	if update.StepID != "" && (snapshot.Step == nil || snapshot.Step.ID != update.StepID) {
		snapshot.Step = &readmodelcore.StepView{ID: update.StepID}
	}
	if snapshot.Step != nil {
		if update.StepID != "" {
			snapshot.Step.ID = update.StepID
		}
		if update.StepKind != "" {
			snapshot.Step.Kind = update.StepKind
		}
		if update.StepState != "" {
			snapshot.Step.State = update.StepState
		}
	}
	if snapshot.TaskProgress.WorkspaceID == "" {
		snapshot.TaskProgress.WorkspaceID = update.WorkspaceID
	}
	if update.RunID != "" {
		snapshot.TaskProgress.RunID = update.RunID
	}
	if update.StepID != "" {
		snapshot.TaskProgress.CurrentStepID = update.StepID
	}
	if update.StepKind != "" {
		snapshot.TaskProgress.CurrentStepKind = update.StepKind
	}
	if update.StepState != "" {
		snapshot.TaskProgress.CurrentStepState = update.StepState
	}
	progressAt := strings.TrimSpace(update.OccurredAt)
	if progressAt == "" {
		progressAt = now.Format(time.RFC3339Nano)
	}
	if snapshot.RunHeartbeat.WorkspaceID == "" {
		snapshot.RunHeartbeat.WorkspaceID = update.WorkspaceID
	}
	if update.RunID != "" {
		snapshot.RunHeartbeat.RunID = update.RunID
	}
	if snapshot.Run != nil && snapshot.RunHeartbeat.State == "" {
		snapshot.RunHeartbeat.State = snapshot.Run.State
	}
	if update.StepID != "" {
		snapshot.RunHeartbeat.LatestStepID = update.StepID
	}
	if update.StepKind != "" {
		snapshot.RunHeartbeat.LatestStepKind = update.StepKind
	}
	if update.StepState != "" {
		snapshot.RunHeartbeat.LatestStepState = update.StepState
	}
	snapshot.RunHeartbeat.ActivityState = "running"
	snapshot.RunHeartbeat.LatestEventTopic = update.Topic
	if update.Revision > 0 {
		snapshot.RunHeartbeat.LatestEventRevision = update.Revision
	}
	snapshot.RunHeartbeat.LatestEventAt = progressAt
	snapshot.RunHeartbeat.LatestEffectiveProgressAt = progressAt
	if update.Summary != "" {
		snapshot.RunHeartbeat.Summary = update.Summary
	}
	if snapshot.RuntimeExecution != nil && update.StepID != "" && snapshot.RuntimeExecution.StepID != "" && snapshot.RuntimeExecution.StepID != update.StepID {
		snapshot.RuntimeExecution = nil
	}
	if snapshot.RuntimeExecution == nil {
		snapshot.RuntimeExecution = &readmodelcore.RuntimeExecutionState{}
	}
	if update.StepID != "" {
		snapshot.RuntimeExecution.StepID = update.StepID
	}
	if update.StepKind != "" {
		snapshot.RuntimeExecution.StepKind = update.StepKind
	}
	if update.StepState != "" {
		snapshot.RuntimeExecution.StepState = update.StepState
	}
	if update.RequestID != "" {
		snapshot.RuntimeExecution.RequestID = update.RequestID
	}
	if update.QueueKey != "" {
		snapshot.RuntimeExecution.QueueKey = update.QueueKey
	}
	if update.WorkerID != "" {
		snapshot.RuntimeExecution.WorkerID = update.WorkerID
	}
	snapshot.RuntimeExecution.LastProgressAt = progressAt
	snapshot.RuntimeExecution.LastProgressEvent = update.Event
	snapshot.RuntimeExecution.LastProgressStage = update.Stage
	snapshot.RuntimeExecution.LastProgressSummary = update.Summary
	if update.WorkerEventAt != "" {
		snapshot.RuntimeExecution.WorkerHeartbeatAt = update.WorkerEventAt
	} else if strings.Contains(update.Summary, "heartbeat") || strings.Contains(update.Event, "heartbeat") {
		snapshot.RuntimeExecution.WorkerHeartbeatAt = progressAt
	}
	snapshot.RuntimeExecution.Pending = !isTerminalRuntimeProgressState(update.StepState)
	if !snapshot.RuntimeExecution.Pending {
		snapshot.RuntimeExecution.Gap = ""
		return true
	}
	if snapshot.RuntimeExecution.Pending && snapshot.RuntimeExecution.Gap == "" {
		snapshot.RuntimeExecution.Gap = "accepted_in_progress"
	}
	return true
}

func isTerminalRuntimeProgressState(state string) bool {
	switch common.StepState(strings.TrimSpace(state)) {
	case common.StepStateSucceeded, common.StepStateFailed:
		return true
	default:
		return false
	}
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
	item.ViewModel.Sections = cloneAppWorkspaceViewModelSections(item.ViewModel.Sections)
	return item
}

func cloneWorkspaceSnapshot(item readmodelcore.WorkspaceSnapshot) readmodelcore.WorkspaceSnapshot {
	item.Workspace = cloneWorkspace(item.Workspace)
	item.ViewModel.Sections = cloneWorkspaceViewModelSections(item.ViewModel.Sections)
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
	if item.RunCommand != nil {
		status := *item.RunCommand
		item.RunCommand = &status
	}
	if item.GapBaseline != nil {
		baseline := cloneGapBaseline(*item.GapBaseline)
		item.GapBaseline = &baseline
	}
	item.AuthorityDiagnostics = append([]readmodelcore.GapDiagnostic(nil), item.AuthorityDiagnostics...)
	return item
}

func cloneWorkspaceViewModelSections(input map[string]readmodelcore.WorkspaceViewModelSection) map[string]readmodelcore.WorkspaceViewModelSection {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]readmodelcore.WorkspaceViewModelSection, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneAppWorkspaceViewModelSections(input map[string]appmodelcore.WorkspaceViewModelSection) map[string]appmodelcore.WorkspaceViewModelSection {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]appmodelcore.WorkspaceViewModelSection, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneGapBaseline(item readmodelcore.GapBaseline) readmodelcore.GapBaseline {
	item.CurrentFocus = cloneStringAnyMap(item.CurrentFocus)
	item.OpenCoverageWorkItems = cloneMapSlice(item.OpenCoverageWorkItems)
	item.AcceptedEvidenceRefs = append([]string(nil), item.AcceptedEvidenceRefs...)
	item.RejectedEvidenceRefs = append([]string(nil), item.RejectedEvidenceRefs...)
	item.TodoItems = append([]readmodelcore.GapTodoItem(nil), item.TodoItems...)
	item.Diagnostics = append([]readmodelcore.GapDiagnostic(nil), item.Diagnostics...)
	for idx := range item.TodoItems {
		item.TodoItems[idx].CoverageWorkItemIDs = append([]string(nil), item.TodoItems[idx].CoverageWorkItemIDs...)
		item.TodoItems[idx].RequiredEvidenceKinds = append([]string(nil), item.TodoItems[idx].RequiredEvidenceKinds...)
		item.TodoItems[idx].Metadata = cloneStringAnyMap(item.TodoItems[idx].Metadata)
	}
	return item
}

func cloneWorkspaceProjectionEntry(item *workspaceProjectionEntry) *workspaceProjectionEntry {
	if item == nil {
		return &workspaceProjectionEntry{}
	}
	ret := *item
	if item.HasWorkspace {
		ret.Workspace = cloneWorkspace(item.Workspace)
	}
	if item.HasSnapshot {
		ret.Snapshot = cloneWorkspaceSnapshot(item.Snapshot)
	}
	return &ret
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
