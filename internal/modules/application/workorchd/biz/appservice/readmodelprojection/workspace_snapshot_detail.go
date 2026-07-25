package readmodelprojection

import (
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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
	ret = NormalizeWorkspaceGovernanceProjection(ret)
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
	ret.ConvergenceStatus = MapWorkspaceConvergenceStatus(ret)
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
	projectWorkspaceSnapshotRuntimeFields(ret)
	projectWorkspaceSnapshotTaskFields(ret)
	if ret.Run != nil {
		projected := compactWorkspaceSnapshotRun(*ret.Run)
		ret.Run = &projected
		ret.Runs = ensureWorkspaceSnapshotRunsInclude(ret.Runs, projected)
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

func projectWorkspaceSnapshotRuntimeFields(ret *readmodelcore.WorkspaceSnapshot) {
	if ret == nil {
		return
	}
	if ret.Step != nil {
		projected := ProjectStepView(*ret.Step)
		ret.Step = &projected
	}
}

func projectWorkspaceSnapshotTaskFields(ret *readmodelcore.WorkspaceSnapshot) {
	if ret == nil {
		return
	}
	ret.TaskProgress = projectTaskProgressView(ret.TaskProgress)
	if len(ret.TaskDecompositions) > 0 {
		ret.TaskDecompositions = projectTaskDecompositionViews(ret.TaskDecompositions, workspaceSnapshotTraceSliceLimit)
	}
	if len(ret.TaskPlans) > 0 {
		ret.TaskPlans = projectTaskPlanViews(ret.TaskPlans, workspaceSnapshotTraceSliceLimit)
	}
}

func projectTaskDecompositionViews(items []readmodelcore.TaskDecomposition, limit int) []readmodelcore.TaskDecomposition {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	start := len(items) - limit
	if start < 0 {
		start = 0
	}
	ret := make([]readmodelcore.TaskDecomposition, 0, len(items[start:]))
	for _, item := range items[start:] {
		ret = append(ret, projectTaskDecompositionView(item))
	}
	return ret
}

func projectTaskPlanViews(items []readmodelcore.TaskPlan, limit int) []readmodelcore.TaskPlan {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	start := len(items) - limit
	if start < 0 {
		start = 0
	}
	ret := make([]readmodelcore.TaskPlan, 0, len(items[start:]))
	for _, item := range items[start:] {
		ret = append(ret, projectTaskPlanView(item))
	}
	return ret
}

func projectTaskDecompositionView(item readmodelcore.TaskDecomposition) readmodelcore.TaskDecomposition {
	item.Summary = trimTraceString(item.Summary)
	item.EvidenceRefs = compactStringSlice(item.EvidenceRefs, workspaceSnapshotTraceSliceLimit)
	item.Nodes = projectTaskDecompositionNodeViews(item.Nodes, workspaceSnapshotTraceSliceLimit)
	return item
}

func projectTaskPlanView(item readmodelcore.TaskPlan) readmodelcore.TaskPlan {
	item.Summary = trimTraceString(item.Summary)
	item.EvidenceRefs = compactStringSlice(item.EvidenceRefs, workspaceSnapshotTraceSliceLimit)
	item.Nodes = projectTaskDecompositionNodeViews(item.Nodes, workspaceSnapshotTraceSliceLimit)
	return item
}

func projectTaskDecompositionNodeViews(items []readmodelcore.TaskDecompositionNode, limit int) []readmodelcore.TaskDecompositionNode {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	if len(items) > limit {
		items = items[:limit]
	}
	ret := make([]readmodelcore.TaskDecompositionNode, 0, len(items))
	for _, item := range items {
		ret = append(ret, projectTaskDecompositionNodeView(item))
	}
	return ret
}

func projectTaskDecompositionNodeView(item readmodelcore.TaskDecompositionNode) readmodelcore.TaskDecompositionNode {
	item.Title = trimTraceString(item.Title)
	item.FailureSignature = trimTraceString(item.FailureSignature)
	item.DependsOn = compactStringSlice(item.DependsOn, workspaceSnapshotTraceSliceLimit)
	item.EvidenceRefs = compactStringSlice(item.EvidenceRefs, workspaceSnapshotTraceSliceLimit)
	item.CoverageTargets = compactStringSlice(item.CoverageTargets, workspaceSnapshotTraceSliceLimit)
	item.Input = compactTraceValueMap(item.Input, 0)
	return item
}

func ensureWorkspaceSnapshotRunsInclude(items []readmodelcore.Run, item readmodelcore.Run) []readmodelcore.Run {
	if item.ID == "" {
		return items
	}
	for idx := range items {
		if items[idx].ID == item.ID {
			items[idx] = item
			return items
		}
	}
	return append([]readmodelcore.Run{item}, items...)
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
	return item
}

func ProjectLLMUsageTraceView(item readmodelcore.LLMUsageTrace) readmodelcore.LLMUsageTrace {
	item.ContextBudget = compactTracePayload(item.ContextBudget)
	item.CacheUsage = compactTracePayload(item.CacheUsage)
	item.CacheMarkers = compactTracePayload(item.CacheMarkers)
	return item
}

func projectLLMContextBudgetTraceView(item readmodelcore.LLMContextBudgetTrace) readmodelcore.LLMContextBudgetTrace {
	item.Rules = compactStringSlice(item.Rules, workspaceSnapshotTraceSliceLimit)
	item.SourceRefs = compactStringSlice(item.SourceRefs, workspaceSnapshotTraceSliceLimit)
	item.IncludedSections = compactStringSlice(item.IncludedSections, workspaceSnapshotTraceSliceLimit)
	item.NormalizedSections = compactStringSlice(item.NormalizedSections, workspaceSnapshotTraceSliceLimit)
	item.CompressedSections = compactStringSlice(item.CompressedSections, workspaceSnapshotTraceSliceLimit)
	item.OmittedSections = compactStringSlice(item.OmittedSections, workspaceSnapshotTraceSliceLimit)
	item.RedactedPaths = compactStringSlice(item.RedactedPaths, workspaceSnapshotTraceSliceLimit)
	item.ProtectedFields = compactStringSlice(item.ProtectedFields, workspaceSnapshotTraceSliceLimit)
	item.IntegrityWarnings = compactStringSlice(item.IntegrityWarnings, workspaceSnapshotTraceSliceLimit)
	item.SectionHashes = projectLLMContextBudgetSections(item.SectionHashes)
	item.Profile = compactTracePayload(item.Profile)
	item.CachePlan = compactTracePayload(item.CachePlan)
	return item
}

func projectLLMContextBudgetSections(items []readmodelcore.LLMContextBudgetSection) []readmodelcore.LLMContextBudgetSection {
	if len(items) > workspaceSnapshotTraceSliceLimit {
		items = items[:workspaceSnapshotTraceSliceLimit]
	}
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.LLMContextBudgetSection, 0, len(items))
	for _, item := range items {
		item.ID = trimTraceString(item.ID)
		item.Hash = trimTraceString(item.Hash)
		item.SourceRefs = compactStringSlice(item.SourceRefs, workspaceSnapshotTraceSliceLimit)
		ret = append(ret, item)
	}
	return ret
}

func compactTracePayload(input workorcheventpayload.Payload) workorcheventpayload.Payload {
	if len(input.Fields) == 0 {
		return workorcheventpayload.Payload{}
	}
	output, err := workorcheventpayload.FromMap(compactTraceValueMap(input.Map(), 0))
	if err != nil {
		return workorcheventpayload.Payload{}
	}
	return output
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
	for _, key := range orderedTraceMapKeys(input) {
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

func orderedTraceMapKeys(input map[string]any) []string {
	ret := make([]string, 0, len(input))
	for key := range input {
		ret = append(ret, key)
	}
	sort.Strings(ret)
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
	case []readmodelcore.Result:
		return compactResultTraceValues(typed, depth+1)
	case []readmodelcore.TaskPlan:
		return compactTaskPlanTraceValues(typed, depth+1)
	case []readmodelcore.Decision:
		return compactDecisionTraceValues(typed, depth+1)
	case []readmodelcore.MemoryRecallTrace:
		return compactMemoryRecallTraceValues(typed, depth+1)
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
		return compactJSONCompatibleTraceValue(typed)
	}
}

func compactJSONCompatibleTraceValue(value any) any {
	if projected, ok := projectReadModelScalarMapValue(value); ok {
		return compactTraceValue(projected, 0)
	}
	return nil
}

func compactResultTraceValues(items []readmodelcore.Result, depth int) []any {
	limit := min(len(items), workspaceSnapshotTraceSliceLimit)
	ret := make([]any, 0, limit)
	for _, item := range items[:limit] {
		if value := compactTraceValueMap(map[string]any{
			"id":           item.ID,
			"workspaceId":  item.WorkspaceID,
			"runId":        item.RunID,
			"state":        item.State,
			"title":        item.Title,
			"summary":      item.Summary,
			"sourceType":   item.SourceType,
			"sourceRef":    item.SourceRef,
			"evidenceRefs": item.EvidenceRefs,
			"artifactRefs": item.ArtifactRefs,
		}, depth); value != nil {
			ret = append(ret, value)
		}
	}
	return ret
}

func compactTaskPlanTraceValues(items []readmodelcore.TaskPlan, depth int) []any {
	limit := min(len(items), workspaceSnapshotTraceSliceLimit)
	ret := make([]any, 0, limit)
	for _, item := range items[:limit] {
		if value := compactTraceValueMap(map[string]any{
			"id":               item.ID,
			"workspaceId":      item.WorkspaceID,
			"runId":            item.RunID,
			"state":            item.State,
			"stage":            item.Stage,
			"summary":          item.Summary,
			"nodeTotal":        item.NodeTotal,
			"nodeCompleted":    item.NodeCompleted,
			"nodeFailed":       item.NodeFailed,
			"nodeRunning":      item.NodeRunning,
			"retryOf":          item.RetryOf,
			"retryAttempt":     item.RetryAttempt,
			"failureClass":     item.FailureClass,
			"failureSignature": item.FailureSignature,
			"evidenceRefs":     item.EvidenceRefs,
		}, depth); value != nil {
			ret = append(ret, value)
		}
	}
	return ret
}

func compactDecisionTraceValues(items []readmodelcore.Decision, depth int) []any {
	limit := min(len(items), workspaceSnapshotTraceSliceLimit)
	ret := make([]any, 0, limit)
	for _, item := range items[:limit] {
		if value := compactTraceValueMap(map[string]any{
			"id":                 item.ID,
			"workspaceId":        item.WorkspaceID,
			"runId":              item.RunID,
			"reason":             item.Reason,
			"question":           item.Question,
			"state":              item.State,
			"resolutionType":     item.ResolutionType,
			"continuationAction": item.ContinuationAction,
			"resumeRunId":        item.ResumeRunID,
			"successorRunId":     item.SuccessorRunID,
		}, depth); value != nil {
			ret = append(ret, value)
		}
	}
	return ret
}

func compactMemoryRecallTraceValues(items []readmodelcore.MemoryRecallTrace, depth int) []any {
	limit := min(len(items), workspaceSnapshotTraceSliceLimit)
	ret := make([]any, 0, limit)
	for _, item := range items[:limit] {
		if value := compactTraceValueMap(map[string]any{
			"id":                 item.ID,
			"workspaceId":        item.WorkspaceID,
			"runId":              item.RunID,
			"stepId":             item.StepID,
			"stage":              item.Stage,
			"source":             item.Source,
			"status":             item.Status,
			"queryTextPreview":   item.QueryTextPreview,
			"candidateMemoryIds": item.CandidateMemoryIDs,
			"retrievedMemoryIds": item.RetrievedMemoryIDs,
			"reason":             item.Reason,
			"occurredAt":         item.OccurredAt,
			"evidenceRef":        item.EvidenceRef,
		}, depth); value != nil {
			ret = append(ret, value)
		}
	}
	return ret
}

func trimTraceString(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= workspaceSnapshotTraceStringLimit {
		return value
	}
	return value[:workspaceSnapshotTraceStringLimit] + "...[truncated]"
}
