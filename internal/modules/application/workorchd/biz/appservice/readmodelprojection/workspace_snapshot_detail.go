package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

const (
	workspaceSnapshotDetailRunLimit                = 10
	workspaceSnapshotDetailLLMTraceLimit           = 20
	workspaceSnapshotDetailLLMContextBudgetLimit   = 20
	workspaceSnapshotDetailMemoryRecallLimit       = 20
	workspaceSnapshotDetailExecutionSectionLimit   = 60
	workspaceSnapshotDetailContextSectionLimit     = 12
	workspaceSnapshotDetailRecentEvidenceLimit     = 24
	workspaceSnapshotDetailCompressionCandidateMax = 12
	workspaceSnapshotTraceMapLimit                 = 24
	workspaceSnapshotTraceSliceLimit               = 12
	workspaceSnapshotTraceStringLimit              = 1024
)

func ProjectWorkspaceSnapshotDetail(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.WorkspaceSnapshot {
	ret := snapshot
	projectWorkspaceSnapshotCompactFields(&ret)
	if ret.ContextBundle == nil {
		ret = WorkspaceSnapshotWithContextBundle(ret)
	}
	if ret.ContextBundle != nil {
		projected := projectContextBundleView(*ret.ContextBundle)
		ret.ContextBundle = &projected
	}
	if len(ret.Decisions) > 0 {
		ret.Decisions = selectWorkspaceDetailDecisions(ret, decisionPageLimit)
		limit := len(ret.Decisions)
		projected := make([]readmodelcore.Decision, 0, limit)
		for _, item := range ret.Decisions {
			projected = append(projected, ProjectDecisionView(item))
		}
		ret.Decisions = projected
	}
	if len(ret.Approvals) > 0 {
		ret.Approvals = selectWorkspaceDetailApprovals(ret, approvalPageLimit)
		limit := len(ret.Approvals)
		projected := make([]readmodelcore.Approval, 0, limit)
		for _, item := range ret.Approvals {
			projected = append(projected, ProjectApprovalView(item))
		}
		ret.Approvals = projected
	}
	return ret
}

func selectWorkspaceDetailDecisions(snapshot readmodelcore.WorkspaceSnapshot, limit int) []readmodelcore.Decision {
	if limit <= 0 || len(snapshot.Decisions) == 0 {
		return nil
	}
	selected := make([]readmodelcore.Decision, 0, min(len(snapshot.Decisions), limit))
	seen := map[string]struct{}{}
	add := func(item readmodelcore.Decision) {
		if len(selected) >= limit || strings.TrimSpace(item.ID) == "" {
			return
		}
		if _, ok := seen[item.ID]; ok {
			return
		}
		seen[item.ID] = struct{}{}
		selected = append(selected, item)
	}
	addDecisionByID := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		for _, item := range snapshot.Decisions {
			if item.ID == id {
				add(item)
				return
			}
		}
	}

	addDecisionByID(snapshot.TaskProgress.Recovery.BlockingTicketID)
	addDecisionByID(snapshot.TaskProgress.BlockingTicketID)
	activeRunID := strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	if activeRunID != "" {
		for _, item := range snapshot.Decisions {
			if strings.TrimSpace(item.RunID) == activeRunID && !IsResolvedDecisionState(item.State) {
				add(item)
			}
		}
	}
	for _, item := range snapshot.Decisions {
		add(item)
	}
	return selected
}

func selectWorkspaceDetailApprovals(snapshot readmodelcore.WorkspaceSnapshot, limit int) []readmodelcore.Approval {
	if limit <= 0 || len(snapshot.Approvals) == 0 {
		return nil
	}
	selected := make([]readmodelcore.Approval, 0, min(len(snapshot.Approvals), limit))
	seen := map[string]struct{}{}
	add := func(item readmodelcore.Approval) {
		if len(selected) >= limit || strings.TrimSpace(item.ID) == "" {
			return
		}
		if _, ok := seen[item.ID]; ok {
			return
		}
		seen[item.ID] = struct{}{}
		selected = append(selected, item)
	}
	addApprovalByID := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		for _, item := range snapshot.Approvals {
			if item.ID == id {
				add(item)
				return
			}
		}
	}

	addApprovalByID(snapshot.TaskProgress.Recovery.BlockingApprovalID)
	addApprovalByID(snapshot.TaskProgress.BlockingApprovalID)
	activeRunID := strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	if activeRunID != "" {
		for _, item := range snapshot.Approvals {
			if strings.TrimSpace(item.RunID) == activeRunID && !IsResolvedApprovalState(item.State) {
				add(item)
			}
		}
	}
	for _, item := range snapshot.Approvals {
		add(item)
	}
	return selected
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
	if len(ret.LLMContextBudgets) > workspaceSnapshotDetailLLMContextBudgetLimit {
		ret.LLMContextBudgets = append([]readmodelcore.LLMContextBudgetTrace(nil), ret.LLMContextBudgets[len(ret.LLMContextBudgets)-workspaceSnapshotDetailLLMContextBudgetLimit:]...)
	}
	for idx := range ret.LLMContextBudgets {
		ret.LLMContextBudgets[idx] = projectLLMContextBudgetTraceView(ret.LLMContextBudgets[idx])
	}
	if len(ret.MemoryRecallTraces) > workspaceSnapshotDetailMemoryRecallLimit {
		ret.MemoryRecallTraces = append([]readmodelcore.MemoryRecallTrace(nil), ret.MemoryRecallTraces[len(ret.MemoryRecallTraces)-workspaceSnapshotDetailMemoryRecallLimit:]...)
	}
	for idx := range ret.MemoryRecallTraces {
		ret.MemoryRecallTraces[idx] = projectMemoryRecallTraceView(ret.MemoryRecallTraces[idx])
	}
	if len(ret.ExecutionSections) > workspaceSnapshotDetailExecutionSectionLimit {
		ret.ExecutionSections = append([]readmodelcore.ExecutionSection(nil), ret.ExecutionSections[len(ret.ExecutionSections)-workspaceSnapshotDetailExecutionSectionLimit:]...)
	}
	for idx := range ret.ExecutionSections {
		ret.ExecutionSections[idx] = projectExecutionSectionView(ret.ExecutionSections[idx])
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

func projectLLMContextBudgetTraceView(item readmodelcore.LLMContextBudgetTrace) readmodelcore.LLMContextBudgetTrace {
	item.Rules = compactStringSlice(item.Rules, workspaceSnapshotTraceSliceLimit)
	item.SourceRefs = compactStringSlice(item.SourceRefs, workspaceSnapshotTraceSliceLimit)
	item.IncludedSections = compactStringSlice(item.IncludedSections, workspaceSnapshotTraceSliceLimit)
	item.NormalizedSections = compactStringSlice(item.NormalizedSections, workspaceSnapshotTraceSliceLimit)
	item.CompressedSections = compactStringSlice(item.CompressedSections, workspaceSnapshotTraceSliceLimit)
	item.OmittedSections = compactStringSlice(item.OmittedSections, workspaceSnapshotTraceSliceLimit)
	item.Profile = compactTraceValueMap(item.Profile, 0)
	item.CachePlan = compactTraceValueMap(item.CachePlan, 0)
	return item
}

func projectMemoryRecallTraceView(item readmodelcore.MemoryRecallTrace) readmodelcore.MemoryRecallTrace {
	item.QueryTextPreview = trimTraceString(item.QueryTextPreview)
	item.CandidateMemoryIDs = compactStringSlice(item.CandidateMemoryIDs, workspaceSnapshotTraceSliceLimit)
	item.SelectedMemoryIDs = compactStringSlice(item.SelectedMemoryIDs, workspaceSnapshotTraceSliceLimit)
	item.ExcludedMemoryIDs = compactStringSlice(item.ExcludedMemoryIDs, workspaceSnapshotTraceSliceLimit)
	item.RetrievedMemoryIDs = compactStringSlice(item.RetrievedMemoryIDs, workspaceSnapshotTraceSliceLimit)
	if len(item.SelectionScores) > workspaceSnapshotTraceSliceLimit {
		item.SelectionScores = append([]readmodelcore.MemoryRecallSelectionScore(nil), item.SelectionScores[:workspaceSnapshotTraceSliceLimit]...)
	}
	for idx := range item.SelectionScores {
		item.SelectionScores[idx].Reason = trimTraceString(item.SelectionScores[idx].Reason)
	}
	if len(item.RetrievedMemoryHits) > workspaceSnapshotTraceSliceLimit {
		item.RetrievedMemoryHits = append([]readmodelcore.MemoryRecallHit(nil), item.RetrievedMemoryHits[:workspaceSnapshotTraceSliceLimit]...)
	}
	for idx := range item.RetrievedMemoryHits {
		item.RetrievedMemoryHits[idx].Summary = trimTraceString(item.RetrievedMemoryHits[idx].Summary)
	}
	item.Reason = trimTraceString(item.Reason)
	return item
}

func projectExecutionSectionView(item readmodelcore.ExecutionSection) readmodelcore.ExecutionSection {
	item.RelatedDoDItems = compactStringSlice(item.RelatedDoDItems, workspaceSnapshotTraceSliceLimit)
	item.PlannedStepRefs = compactStringSlice(item.PlannedStepRefs, workspaceSnapshotTraceSliceLimit)
	item.ActualEvidenceRefs = compactStringSlice(item.ActualEvidenceRefs, workspaceSnapshotTraceSliceLimit)
	item.DecisionRefs = compactStringSlice(item.DecisionRefs, workspaceSnapshotTraceSliceLimit)
	item.ApprovalRefs = compactStringSlice(item.ApprovalRefs, workspaceSnapshotTraceSliceLimit)
	item.ToolCallRefs = compactStringSlice(item.ToolCallRefs, workspaceSnapshotTraceSliceLimit)
	item.PatchRefs = compactStringSlice(item.PatchRefs, workspaceSnapshotTraceSliceLimit)
	item.ValidationRefs = compactStringSlice(item.ValidationRefs, workspaceSnapshotTraceSliceLimit)
	item.ResultRefs = compactStringSlice(item.ResultRefs, workspaceSnapshotTraceSliceLimit)
	item.ArtifactRefs = compactStringSlice(item.ArtifactRefs, workspaceSnapshotTraceSliceLimit)
	item.MemoryRecallRefs = compactStringSlice(item.MemoryRecallRefs, workspaceSnapshotTraceSliceLimit)
	item.MemoryIDs = compactStringSlice(item.MemoryIDs, workspaceSnapshotTraceSliceLimit)
	item.Summary = trimTraceString(item.Summary)
	return item
}

func projectContextBundleView(item readmodelcore.ContextBundle) readmodelcore.ContextBundle {
	item.TaskProgress = projectTaskProgressView(item.TaskProgress)
	item.CompressedContext = projectCompressedContextView(item.CompressedContext)
	if len(item.Sections) > workspaceSnapshotDetailContextSectionLimit {
		item.Sections = append([]readmodelcore.ContextSection(nil), item.Sections[:workspaceSnapshotDetailContextSectionLimit]...)
	}
	for idx := range item.Sections {
		item.Sections[idx].EvidenceRefs = compactStringSlice(item.Sections[idx].EvidenceRefs, workspaceSnapshotTraceSliceLimit)
		item.Sections[idx].Data = compactTraceValueMap(item.Sections[idx].Data, 0)
		item.Sections[idx].Summary = trimTraceString(item.Sections[idx].Summary)
	}
	if len(item.RecentEvidence) > workspaceSnapshotDetailRecentEvidenceLimit {
		item.RecentEvidence = append([]readmodelcore.ContextEvidence(nil), item.RecentEvidence[:workspaceSnapshotDetailRecentEvidenceLimit]...)
	}
	for idx := range item.RecentEvidence {
		item.RecentEvidence[idx] = projectContextEvidenceView(item.RecentEvidence[idx])
	}
	if len(item.CompressionCandidates) > workspaceSnapshotDetailCompressionCandidateMax {
		item.CompressionCandidates = append([]readmodelcore.ContextCompressionCandidate(nil), item.CompressionCandidates[:workspaceSnapshotDetailCompressionCandidateMax]...)
	}
	for idx := range item.CompressionCandidates {
		item.CompressionCandidates[idx].SourceRefs = compactStringSlice(item.CompressionCandidates[idx].SourceRefs, workspaceSnapshotTraceSliceLimit)
	}
	return item
}

func projectTaskProgressView(item readmodelcore.TaskProgress) readmodelcore.TaskProgress {
	item.EvidenceRefs = compactStringSlice(item.EvidenceRefs, workspaceSnapshotTraceSliceLimit)
	if len(item.Milestones) > workspaceSnapshotTraceSliceLimit {
		item.Milestones = append([]readmodelcore.ProgressMilestone(nil), item.Milestones[:workspaceSnapshotTraceSliceLimit]...)
	}
	for idx := range item.Milestones {
		item.Milestones[idx].EvidenceRefs = compactStringSlice(item.Milestones[idx].EvidenceRefs, workspaceSnapshotTraceSliceLimit)
	}
	if len(item.Items) > workspaceSnapshotTraceSliceLimit {
		item.Items = append([]readmodelcore.ProgressItem(nil), item.Items[:workspaceSnapshotTraceSliceLimit]...)
	}
	for idx := range item.Items {
		item.Items[idx].EvidenceRefs = compactStringSlice(item.Items[idx].EvidenceRefs, workspaceSnapshotTraceSliceLimit)
	}
	if len(item.Blockers) > workspaceSnapshotTraceSliceLimit {
		item.Blockers = append([]readmodelcore.ProgressBlocker(nil), item.Blockers[:workspaceSnapshotTraceSliceLimit]...)
	}
	return item
}

func projectCompressedContextView(item readmodelcore.CompressedContext) readmodelcore.CompressedContext {
	item.Milestones = projectTaskProgressView(readmodelcore.TaskProgress{Milestones: item.Milestones}).Milestones
	if len(item.Blockers) > workspaceSnapshotTraceSliceLimit {
		item.Blockers = append([]readmodelcore.ProgressBlocker(nil), item.Blockers[:workspaceSnapshotTraceSliceLimit]...)
	}
	if len(item.RecentEvidence) > workspaceSnapshotDetailRecentEvidenceLimit {
		item.RecentEvidence = append([]readmodelcore.ContextEvidence(nil), item.RecentEvidence[:workspaceSnapshotDetailRecentEvidenceLimit]...)
	}
	for idx := range item.RecentEvidence {
		item.RecentEvidence[idx] = projectContextEvidenceView(item.RecentEvidence[idx])
	}
	item.ResultRefs = compactStringSlice(item.ResultRefs, workspaceSnapshotTraceSliceLimit)
	item.SourceSectionRefs = compactStringSlice(item.SourceSectionRefs, workspaceSnapshotTraceSliceLimit)
	item.SourceEvidenceRefs = compactStringSlice(item.SourceEvidenceRefs, workspaceSnapshotTraceSliceLimit)
	return item
}

func projectContextEvidenceView(item readmodelcore.ContextEvidence) readmodelcore.ContextEvidence {
	item.Summary = trimTraceString(item.Summary)
	item.EvidenceRefs = compactStringSlice(item.EvidenceRefs, workspaceSnapshotTraceSliceLimit)
	item.Data = compactTraceValueMap(item.Data, 0)
	return item
}

func compactStringSlice(items []string, limit int) []string {
	if len(items) == 0 {
		return nil
	}
	if limit <= 0 {
		return nil
	}
	if len(items) > limit {
		items = items[:limit]
	}
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, trimTraceString(item))
	}
	return ret
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
