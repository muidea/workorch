package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

const (
	workspaceSnapshotDetailRunLimit      = 10
	workspaceSnapshotDetailLLMTraceLimit = 20
	workspaceSnapshotTraceMapLimit       = 24
	workspaceSnapshotTraceSliceLimit     = 12
	workspaceSnapshotTraceStringLimit    = 1024
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
	if len(ret.LLMUsageTraces) > 0 {
		projected := make([]readmodelcore.LLMUsageTrace, 0, len(ret.LLMUsageTraces))
		for _, item := range ret.LLMUsageTraces {
			projected = append(projected, ProjectLLMUsageTraceView(item))
		}
		ret.LLMUsageTraces = projected
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

func ProjectLLMUsageTraceView(item readmodelcore.LLMUsageTrace) readmodelcore.LLMUsageTrace {
	item.ContextBudget = compactTraceValueMap(item.ContextBudget, 0)
	item.CacheUsage = compactTraceValueMap(item.CacheUsage, 0)
	item.CacheMarkers = compactTraceValueMap(item.CacheMarkers, 0)
	return item
}

func compactTraceValueMap(input map[string]any, depth int) map[string]any {
	if len(input) == 0 || depth > 2 {
		return nil
	}
	ret := make(map[string]any, min(len(input), workspaceSnapshotTraceMapLimit))
	count := 0
	for _, key := range orderedDecisionContextMapKeys(input) {
		if count >= workspaceSnapshotTraceMapLimit {
			break
		}
		if value := compactTraceValue(input[key], depth+1); value != nil {
			ret[key] = value
			count++
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func compactTraceValue(value any, depth int) any {
	if depth > 3 {
		return nil
	}
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return trimTraceString(typed)
	case bool, int, int64, float64:
		return typed
	case []string:
		limit := min(len(typed), workspaceSnapshotTraceSliceLimit)
		ret := make([]string, 0, limit)
		for _, item := range typed[:limit] {
			ret = append(ret, trimTraceString(item))
		}
		return ret
	case []any:
		limit := min(len(typed), workspaceSnapshotTraceSliceLimit)
		ret := make([]any, 0, limit)
		for _, item := range typed[:limit] {
			if value := compactTraceValue(item, depth+1); value != nil {
				ret = append(ret, value)
			}
		}
		return ret
	case map[string]any:
		return compactTraceValueMap(typed, depth+1)
	default:
		return typed
	}
}

func trimTraceString(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= workspaceSnapshotTraceStringLimit {
		return value
	}
	return value[:workspaceSnapshotTraceStringLimit] + "...[truncated]"
}
