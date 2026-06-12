package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

const (
	workspaceSnapshotDetailRunLimit      = 10
	workspaceSnapshotDetailLLMTraceLimit = 20
)

func ProjectWorkspaceSnapshotDetail(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.WorkspaceSnapshot {
	ret := snapshot
	projectWorkspaceSnapshotCompactFields(&ret)
	if ret.ContextBundle == nil {
		ret = WorkspaceSnapshotWithContextBundle(ret)
	}
	if len(ret.Decisions) > 0 {
		limit := min(len(ret.Decisions), decisionPageLimit)
		projected := make([]readmodelcore.Decision, 0, limit)
		for _, item := range ret.Decisions[:limit] {
			projected = append(projected, ProjectDecisionView(item))
		}
		ret.Decisions = projected
	}
	if len(ret.Approvals) > 0 {
		limit := min(len(ret.Approvals), approvalPageLimit)
		projected := make([]readmodelcore.Approval, 0, limit)
		for _, item := range ret.Approvals[:limit] {
			projected = append(projected, ProjectApprovalView(item))
		}
		ret.Approvals = projected
	}
	return ret
}

func projectWorkspaceSnapshotCompactFields(ret *readmodelcore.WorkspaceSnapshot) {
	if ret == nil {
		return
	}
	ret.Workspace.ContextHints = nil
	ret.Workspace.EvidencePolicies = nil
	if ret.Run != nil {
		projected := compactWorkspaceSnapshotRun(*ret.Run)
		ret.Run = &projected
	}
	if len(ret.Runs) > 0 {
		ret.Runs = compactWorkspaceSnapshotRuns(ret.Runs, ret.Workspace.ActiveRunID, workspaceSnapshotDetailRunLimit)
	}
	if len(ret.LLMUsageTraces) > workspaceSnapshotDetailLLMTraceLimit {
		ret.LLMUsageTraces = append([]readmodelcore.LLMUsageTrace(nil), ret.LLMUsageTraces[len(ret.LLMUsageTraces)-workspaceSnapshotDetailLLMTraceLimit:]...)
	}
}

func compactWorkspaceSnapshotRuns(items []readmodelcore.Run, activeRunID string, limit int) []readmodelcore.Run {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	start := len(items) - limit
	if start < 0 {
		start = 0
	}
	projected := make([]readmodelcore.Run, 0, len(items[start:])+1)
	seen := map[string]struct{}{}
	if activeRunID != "" {
		for _, item := range items {
			if item.ID != activeRunID {
				continue
			}
			projected = append(projected, compactWorkspaceSnapshotRun(item))
			seen[item.ID] = struct{}{}
			break
		}
	}
	for _, item := range items[start:] {
		if _, ok := seen[item.ID]; ok {
			continue
		}
		projected = append(projected, compactWorkspaceSnapshotRun(item))
		seen[item.ID] = struct{}{}
	}
	return projected
}

func compactWorkspaceSnapshotRun(item readmodelcore.Run) readmodelcore.Run {
	item.ExecutionContext = nil
	return item
}
