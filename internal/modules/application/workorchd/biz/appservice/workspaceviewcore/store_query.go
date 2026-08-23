package workspaceviewcore

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func workspaceWithSnapshotGovernance(workspace appmodelcore.Workspace, snapshot readmodelcore.WorkspaceSnapshotState) appmodelcore.Workspace {
	next := cloneWorkspace(workspace)
	if strings.TrimSpace(next.ID) == "" {
		next.ID = snapshot.Workspace.ID
	}
	return next
}

func (s *Store) WorkspaceIDs() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ret := append([]string(nil), s.workspaceOrder...)
	sort.Strings(ret)
	return ret
}

// GetActivity returns the store-owned activity feed for a workspace.
// RuntimeView read must page this feed and must not regenerate ActivityRevision.
func (s *Store) GetActivity(workspaceID string) (projectioncontract.WorkspaceActivityFeed, bool) {
	if s == nil {
		return projectioncontract.WorkspaceActivityFeed{}, false
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return projectioncontract.WorkspaceActivityFeed{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry := s.workspaces[workspaceID]
	if entry == nil || !entry.HasActivity {
		return projectioncontract.WorkspaceActivityFeed{}, false
	}
	return cloneWorkspaceActivityFeed(entry.Activity), true
}

func (s *Store) List() ([]appmodelcore.Workspace, projectioncontract.ProjectionStatus, error) {
	if s == nil {
		return nil, projectioncontract.ProjectionStatus{State: StateFailed}, common.ErrConflict
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
				// The list keeps the canonical Workspace owner projection from
				// entry.Workspace. Snapshot governance remains display-only.
				item = compactWorkspaceListOverview(item)
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

func (s *Store) GetSnapshot(workspaceID string) (readmodelcore.WorkspaceSnapshotState, projectioncontract.ProjectionStatus, error) {
	if s == nil {
		return readmodelcore.WorkspaceSnapshotState{}, projectioncontract.ProjectionStatus{State: StateFailed}, common.ErrConflict
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := s.statusForWorkspaceLocked(workspaceID)
	if s.state == StateBuilding || s.state == StateStopped {
		return readmodelcore.WorkspaceSnapshotState{}, status, fmt.Errorf("%w: workspace projection %s", common.ErrConflict, s.state)
	}
	if s.state == StateFailed {
		return readmodelcore.WorkspaceSnapshotState{}, status, fmt.Errorf("%w: workspace projection failed: %s", common.ErrConflict, s.errText)
	}
	entry := s.workspaces[workspaceID]
	if entry == nil || !entry.HasSnapshot {
		if entry != nil && entry.HasWorkspace {
			return readmodelcore.WorkspaceSnapshotState{}, status, fmt.Errorf("%w: workspace projection detail not ready", common.ErrConflict)
		}
		return readmodelcore.WorkspaceSnapshotState{}, status, common.ErrNotFound
	}
	snapshot := normalizedEntrySnapshot(entry)
	snapshot.ViewModel = workspaceViewModelFromStatus(snapshot, status)
	return snapshot, status, nil
}

func (s *Store) GetSummarySnapshot(workspaceID string) (readmodelcore.WorkspaceSnapshotState, projectioncontract.ProjectionStatus, error) {
	if s == nil {
		return readmodelcore.WorkspaceSnapshotState{}, projectioncontract.ProjectionStatus{State: StateFailed}, common.ErrConflict
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := s.statusForWorkspaceLocked(workspaceID)
	if s.state == StateBuilding || s.state == StateStopped {
		return readmodelcore.WorkspaceSnapshotState{}, status, fmt.Errorf("%w: workspace projection %s", common.ErrConflict, s.state)
	}
	if s.state == StateFailed {
		return readmodelcore.WorkspaceSnapshotState{}, status, fmt.Errorf("%w: workspace projection failed: %s", common.ErrConflict, s.errText)
	}
	entry := s.workspaces[workspaceID]
	if entry == nil || !entry.HasWorkspace {
		return readmodelcore.WorkspaceSnapshotState{}, status, common.ErrNotFound
	}
	snapshot := readmodelcore.WorkspaceSnapshotState{Workspace: cloneWorkspace(entry.Workspace)}
	if entry.HasSnapshot {
		snapshot = normalizedEntrySummarySnapshot(entry)
	}
	snapshot.ViewModel = workspaceViewModelFromStatus(snapshot, status)
	return snapshot, status, nil
}

func normalizedEntryListSnapshot(entry *workspaceProjectionEntry) readmodelcore.WorkspaceSnapshotState {
	if entry == nil {
		return readmodelcore.WorkspaceSnapshotState{}
	}
	snapshot := readmodelcore.WorkspaceSnapshotState{Workspace: cloneWorkspace(entry.Workspace)}
	if !entry.HasSnapshot {
		return snapshot
	}
	source := entry.Snapshot
	snapshot.Workspace = cloneWorkspace(source.Workspace)
	snapshot.Run = source.Run
	snapshot.RunHeartbeat = source.RunHeartbeat
	snapshot.TaskProgress = source.TaskProgress
	snapshot.Decisions = projectioncontract.CloneDecisions(source.Decisions)
	snapshot.Approvals = projectioncontract.CloneApprovals(source.Approvals)
	if entry.HasWorkspace {
		snapshot.Workspace = workspaceWithSnapshotGovernance(entry.Workspace, snapshot)
	}
	return readmodelprojection.GovernanceProjection().NormalizeWorkspace(snapshot)
}

func normalizedEntrySummarySnapshot(entry *workspaceProjectionEntry) readmodelcore.WorkspaceSnapshotState {
	snapshot := normalizedEntryListSnapshot(entry)
	if entry == nil || !entry.HasSnapshot {
		return readmodelprojection.WorkspaceProjection().Summary(snapshot)
	}
	source := entry.Snapshot
	snapshot.DisplayOutcome = source.DisplayOutcome
	snapshot.GovernanceDisplay = source.GovernanceDisplay
	snapshot.ConsistencyDiagnostic = projectioncontract.CloneConsistencyDiagnostic(source.ConsistencyDiagnostic)
	snapshot.CoverageSummary = cloneStringAnyMap(source.CoverageSummary)
	snapshot.ConvergenceStatus = source.ConvergenceStatus
	if source.RunCommand != nil {
		status := *source.RunCommand
		snapshot.RunCommand = &status
	}
	snapshot.LLMUsageSummary = source.LLMUsageSummary
	return readmodelprojection.WorkspaceProjection().Summary(snapshot)
}

func normalizedEntrySnapshot(entry *workspaceProjectionEntry) readmodelcore.WorkspaceSnapshotState {
	if entry == nil || !entry.HasSnapshot {
		return readmodelcore.WorkspaceSnapshotState{}
	}
	snapshot := cloneWorkspaceSnapshot(entry.Snapshot)
	if entry.HasWorkspace {
		snapshot.Workspace = workspaceWithSnapshotGovernance(entry.Workspace, snapshot)
	}
	return readmodelprojection.GovernanceProjection().NormalizeWorkspace(snapshot)
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

func workspaceViewModelFromStatus(snapshot readmodelcore.WorkspaceSnapshotState, status projectioncontract.ProjectionStatus) projectioncontract.WorkspaceViewModel {
	revision := status.WorkspaceRevision
	if revision == 0 {
		revision = status.GlobalRevision
	}
	ret := projectioncontract.WorkspaceViewModel{
		WorkspaceID:             snapshot.Workspace.ID,
		State:                   status.State,
		PromptMode:              workspacePromptMode(snapshot),
		Revision:                revision,
		StreamGeneration:        status.StreamGeneration,
		GlobalRevision:          status.GlobalRevision,
		WorkspaceRevision:       status.WorkspaceRevision,
		SourceEventRevision:     status.SourceEventRevision,
		ProjectedSourceRevision: status.ProjectedSourceRevision,
		UpdatedAt:               status.UpdatedAt,
		RebuiltAt:               status.RebuiltAt,
		Degraded:                status.Degraded,
		StaleReason:             status.StaleReason,
		Error:                   status.Error,
		FailedWorkspaceIds:      append([]string(nil), status.FailedWorkspaceIds...),
		DegradedSince:           status.DegradedSince,
		Sections:                map[string]projectioncontract.WorkspaceViewModelSection{},
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
		ret.Sections[name] = projectioncontract.WorkspaceViewModelSection{
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

func appWorkspaceViewModelFromStatus(workspace appmodelcore.Workspace, status projectioncontract.ProjectionStatus) projectioncontract.WorkspaceViewModel {
	return workspaceViewModelFromStatus(readmodelcore.WorkspaceSnapshotState{Workspace: workspace}, status)
}

func appWorkspaceViewModelFromSnapshot(snapshot readmodelcore.WorkspaceSnapshotState, status projectioncontract.ProjectionStatus) projectioncontract.WorkspaceViewModel {
	return workspaceViewModelFromStatus(snapshot, status)
}

func workspacePromptMode(snapshot readmodelcore.WorkspaceSnapshotState) string {
	return readmodelprojection.GovernanceProjection().PromptMode(snapshot)
}
