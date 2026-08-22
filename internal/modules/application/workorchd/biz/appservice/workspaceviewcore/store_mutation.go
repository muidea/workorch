package workspaceviewcore

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func (e *workspaceProjectionEntry) syncSnapshotWorkspace() {
	if e == nil || !e.HasSnapshot || !e.HasWorkspace {
		return
	}
	e.Snapshot.Workspace = workspaceWithSnapshotGovernance(e.Workspace, e.Snapshot)
	e.Snapshot = readmodelprojection.GovernanceProjection().NormalizeWorkspace(e.Snapshot)
	e.Workspace = cloneWorkspace(e.Snapshot.Workspace)
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

func (s *Store) ReplaceAll(workspaces []appmodelcore.Workspace, snapshots map[string]readmodelcore.WorkspaceSnapshotState, degraded error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	nextWorkspaces := make(map[string]*workspaceProjectionEntry, len(workspaces))
	nextOrder := make([]string, 0, len(workspaces))
	for _, item := range workspaces {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		entry := s.cloneEntryForReplaceLocked(item.ID)
		if snapshot, ok := snapshots[item.ID]; ok && strings.TrimSpace(snapshot.Workspace.ID) != "" {
			snapshot = readmodelprojection.WorkspaceProjection().Detail(snapshot)
			if snapshot.RunCommand == nil && entry.HasSnapshot && entry.Snapshot.RunCommand != nil {
				command := *entry.Snapshot.RunCommand
				snapshot.RunCommand = &command
			}
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
		s.advanceEntryRevision(entry)
		nextWorkspaces[item.ID] = entry
		nextOrder = append(nextOrder, item.ID)
	}
	sort.Strings(nextOrder)

	s.workspaces = nextWorkspaces
	s.workspaceOrder = nextOrder
	s.advanceGlobalRevision()
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	if degraded != nil {
		_ = s.submitProjectionLifecycleInternalEventLocked(projectionLifecycleInternalEvent{Kind: projectionLifecycleRebuildCompleted})
		s.errText = degraded.Error()
		s.recordGlobalFailureLocked(degraded)
		// Mark remaining workspace failures for entries without snapshots.
		for id, entry := range s.workspaces {
			if entry != nil && strings.TrimSpace(entry.Error) != "" {
				s.recordWorkspaceFailureLocked(id, fmt.Errorf("%s", entry.Error))
			}
		}
	} else {
		_ = s.submitProjectionLifecycleInternalEventLocked(projectionLifecycleInternalEvent{Kind: projectionLifecycleRebuildCompleted})
		s.clearAllCurrentFailuresLocked()
	}
	s.updatedAt = now
	s.rebuiltAt = now
}

func (s *Store) ReplaceWorkspaceList(workspaces []appmodelcore.Workspace, degraded error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	nextWorkspaces := make(map[string]*workspaceProjectionEntry, len(workspaces))
	nextOrder := make([]string, 0, len(workspaces))
	for _, item := range workspaces {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		entry := s.cloneEntryForReplaceLocked(item.ID)
		entry.Workspace = cloneWorkspace(item)
		entry.HasWorkspace = true
		entry.syncSnapshotWorkspace()
		s.advanceEntryRevision(entry)
		nextWorkspaces[item.ID] = entry
		nextOrder = append(nextOrder, item.ID)
	}
	sort.Strings(nextOrder)

	s.workspaces = nextWorkspaces
	s.workspaceOrder = nextOrder
	s.advanceGlobalRevision()
	s.globalStale = false
	s.globalStaleReason = ""
	// Drop failure records for workspaces no longer present in the list.
	for workspaceID := range s.workspaceFailures {
		if _, ok := nextWorkspaces[workspaceID]; !ok {
			delete(s.workspaceFailures, workspaceID)
		}
	}
	if degraded != nil {
		_ = s.submitProjectionLifecycleInternalEventLocked(projectionLifecycleInternalEvent{Kind: projectionLifecycleRebuildCompleted})
		s.errText = degraded.Error()
		s.recordGlobalFailureLocked(degraded)
	} else {
		_ = s.submitProjectionLifecycleInternalEventLocked(projectionLifecycleInternalEvent{Kind: projectionLifecycleRebuildCompleted})
		// List-only refresh keeps remaining workspace failures; only clears global list loader failure.
		s.globalFailure = nil
		if s.errText != "" && (s.workspaceFailures == nil || len(s.workspaceFailures) == 0) {
			s.errText = ""
		}
	}
	s.updatedAt = now
	s.rebuiltAt = now
}

func (s *Store) UpsertSnapshot(snapshot readmodelcore.WorkspaceSnapshotState) {
	if s == nil || strings.TrimSpace(snapshot.Workspace.ID) == "" {
		return
	}
	snapshot = readmodelprojection.WorkspaceProjection().Detail(snapshot)
	workspaceID := snapshot.Workspace.ID
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	entry := s.entryForUpdateLocked(workspaceID)
	if snapshot.RunCommand == nil && entry.HasSnapshot && entry.Snapshot.RunCommand != nil {
		command := *entry.Snapshot.RunCommand
		snapshot.RunCommand = &command
	}
	// Capture previous activity before replacing snapshot so revision can stay stable
	// when content is unchanged within this projection transaction.
	previousActivity := entry.Activity
	entry.Workspace = cloneWorkspace(snapshot.Workspace)
	entry.HasWorkspace = true
	if !stringSliceContains(s.workspaceOrder, workspaceID) {
		s.workspaceOrder = append(s.workspaceOrder, workspaceID)
		sort.Strings(s.workspaceOrder)
	}
	entry.Snapshot = cloneWorkspaceSnapshot(snapshot)
	entry.HasSnapshot = true
	s.advanceEntryRevision(entry)
	s.advanceGlobalRevision()
	// Commit activity feed in the same transaction as the snapshot revision.
	// Content changes advance ActivityRevision to this entry.Revision; unchanged keeps previous.
	candidateContent := readmodelprojection.ActivityProjection().BuildWorkspaceFeed(entry.Snapshot)
	committed := readmodelprojection.ActivityProjection().CommitWorkspaceFeed(previousActivity, candidateContent, entry.Revision)
	committed.StreamGeneration = s.streamGeneration
	entry.Activity = cloneWorkspaceActivityFeed(committed)
	entry.HasActivity = true
	entry.ActivityRevision = committed.ActivityRevision
	entry.ProjectedSourceRevision = entry.SourceRevision
	entry.RebuiltAt = now
	entry.Stale = false
	entry.StaleReason = ""
	entry.Error = ""
	s.clearWorkspaceFailureLocked(workspaceID)
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	s.globalFailure = nil
	_ = s.submitProjectionLifecycleInternalEventLocked(projectionLifecycleInternalEvent{Kind: projectionLifecycleMutationApplied})
	if len(s.workspaceFailures) == 0 {
		s.errText = ""
	}
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
	s.advanceEntryRevision(entry)
	s.advanceGlobalRevision()
	entry.ProjectedSourceRevision = entry.SourceRevision
	entry.RebuiltAt = now
	entry.Stale = false
	entry.StaleReason = ""
	entry.Error = ""
	s.clearWorkspaceFailureLocked(workspace.ID)
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	s.globalFailure = nil
	_ = s.submitProjectionLifecycleInternalEventLocked(projectionLifecycleInternalEvent{Kind: projectionLifecycleMutationApplied})
	if len(s.workspaceFailures) == 0 {
		s.errText = ""
	}
	s.updatedAt = now
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
	if runCommandRequestAlreadyProjected(entry, update.RequestID) {
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
	snapshot.RunCommand = &projectioncontract.RunCommandStatus{
		WorkspaceID:        strings.TrimSpace(update.WorkspaceID),
		RunID:              strings.TrimSpace(update.RunID),
		State:              "accepted",
		Intent:             strings.TrimSpace(update.Intent),
		Reason:             strings.TrimSpace(update.Reason),
		Disposition:        strings.TrimSpace(update.Disposition),
		Diagnostic:         strings.TrimSpace(update.Diagnostic),
		RequestID:          strings.TrimSpace(update.RequestID),
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
	s.advanceEntryRevision(entry)
	s.advanceGlobalRevision()
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
	_ = s.submitProjectionLifecycleInternalEventLocked(projectionLifecycleInternalEvent{Kind: projectionLifecycleMutationApplied})
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
	if runCommandRequestAlreadyProjected(entry, update.RequestID) {
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
	snapshot.RunCommand = &projectioncontract.RunCommandStatus{
		WorkspaceID:        strings.TrimSpace(update.WorkspaceID),
		RunID:              strings.TrimSpace(update.RunID),
		State:              "rejected",
		Intent:             strings.TrimSpace(update.Intent),
		Reason:             strings.TrimSpace(update.Reason),
		Disposition:        strings.TrimSpace(update.Disposition),
		Diagnostic:         strings.TrimSpace(update.Diagnostic),
		RequestID:          strings.TrimSpace(update.RequestID),
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
	s.advanceEntryRevision(entry)
	s.advanceGlobalRevision()
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
	_ = s.submitProjectionLifecycleInternalEventLocked(projectionLifecycleInternalEvent{Kind: projectionLifecycleMutationApplied})
	s.errText = ""
	s.updatedAt = now
	return true
}

// runCommandRequestAlreadyProjected makes one request identity immutable in
// the derived store. Workspace-owner idempotency remains authoritative; this
// guard only prevents a duplicate delivery (including a contradictory stale
// result with the same request ID) from rewriting the previously committed
// observation or advancing projection revisions.
func runCommandRequestAlreadyProjected(entry *workspaceProjectionEntry, requestID string) bool {
	requestID = strings.TrimSpace(requestID)
	return requestID != "" && entry != nil && entry.HasSnapshot && entry.Snapshot.RunCommand != nil &&
		strings.TrimSpace(entry.Snapshot.RunCommand.RequestID) == requestID
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
		s.recordWorkspaceFailureLocked(workspaceID, err)
	}
	entry.Stale = true
	entry.StaleReason = "rebuild_failed"
	s.advanceGlobalRevision()
	// Do not sticky-set StateDegraded; statusLocked derives degraded from current failure set.
	_ = s.submitProjectionLifecycleInternalEventLocked(projectionLifecycleInternalEvent{Kind: projectionLifecycleDegradedObserved})
	s.updatedAt = time.Now().UTC()
}

func (s *Store) DeleteWorkspace(workspaceID string) {
	if s == nil || strings.TrimSpace(workspaceID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workspaces, workspaceID)
	s.clearWorkspaceFailureLocked(workspaceID)
	nextOrder := s.workspaceOrder[:0]
	for _, id := range s.workspaceOrder {
		if id != workspaceID {
			nextOrder = append(nextOrder, id)
		}
	}
	s.workspaceOrder = nextOrder
	s.advanceGlobalRevision()
	s.refreshProjectedSourceRevisionLocked()
	s.globalStale = false
	s.globalStaleReason = ""
	_ = s.submitProjectionLifecycleInternalEventLocked(projectionLifecycleInternalEvent{Kind: projectionLifecycleMutationApplied})
	if len(s.workspaceFailures) == 0 && s.globalFailure == nil {
		s.errText = ""
	}
	s.updatedAt = time.Now().UTC()
}

func (s *Store) entryForUpdateLocked(workspaceID string) *workspaceProjectionEntry {
	entry := s.workspaces[workspaceID]
	if entry == nil {
		entry = &workspaceProjectionEntry{}
		s.workspaces[workspaceID] = entry
	}
	return entry
}

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
