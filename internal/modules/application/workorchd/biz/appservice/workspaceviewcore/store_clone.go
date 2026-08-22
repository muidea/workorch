package workspaceviewcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// cloneEntryForReplaceLocked snapshots one entry for a replacement
// transaction. The caller holds s.mu so a concurrent derived mutation cannot
// commit between the clone and the map replacement and then be lost.
func (s *Store) cloneEntryForReplaceLocked(workspaceID string) *workspaceProjectionEntry {
	if s == nil {
		return &workspaceProjectionEntry{}
	}
	if entry := s.workspaces[workspaceID]; entry != nil {
		return cloneWorkspaceProjectionEntry(entry)
	}
	return &workspaceProjectionEntry{}
}

func cloneWorkspace(item appmodelcore.Workspace) appmodelcore.Workspace {
	if item.DefaultLLMSelection != nil {
		target := item.DefaultLLMSelection.Clone()
		item.DefaultLLMSelection = &target
	}
	item.SoulBindings = cloneStringStringMap(item.SoulBindings)
	item.ContextHints = cloneStringAnyMap(item.ContextHints)
	item.EvidencePolicies = cloneMapSlice(item.EvidencePolicies)
	item.ViewModel.Sections = cloneWorkspaceViewModelSections(item.ViewModel.Sections)
	return item
}

func cloneWorkspaceSnapshot(item readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState {
	item.Workspace = cloneWorkspace(item.Workspace)
	item.ViewModel.Sections = cloneWorkspaceViewModelSections(item.ViewModel.Sections)
	item.Goals = append([]workorchmanagementcontract.Goal(nil), item.Goals...)
	item.DoDs = append([]workorchmanagementcontract.DoD(nil), item.DoDs...)
	item.Runs = append([]appmodelcore.Run(nil), item.Runs...)
	item.Results = append([]projectioncontract.Result(nil), item.Results...)
	item.Artifacts = append([]projectioncontract.Artifact(nil), item.Artifacts...)
	item.ArtifactContinuations = append([]projectioncontract.ArtifactContinuation(nil), item.ArtifactContinuations...)
	item.Patches = append([]projectioncontract.Patch(nil), item.Patches...)
	item.WebSources = append([]projectioncontract.WebSourceTrace(nil), item.WebSources...)
	item.LLMContextBudgets = append([]projectioncontract.LLMContextBudgetTrace(nil), item.LLMContextBudgets...)
	item.LLMUsageTraces = append([]projectioncontract.LLMUsageTrace(nil), item.LLMUsageTraces...)
	item.EventRecords = append([]projectioncontract.EventRecord(nil), item.EventRecords...)
	item.Decisions = append([]projectioncontract.Decision(nil), item.Decisions...)
	item.Approvals = append([]projectioncontract.Approval(nil), item.Approvals...)
	item.TaskDecompositions = append([]projectioncontract.TaskDecomposition(nil), item.TaskDecompositions...)
	item.ExecutionSections = append([]projectioncontract.ExecutionSection(nil), item.ExecutionSections...)
	item.MemoryRecallTraces = append([]projectioncontract.MemoryRecallTrace(nil), item.MemoryRecallTraces...)
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
	item.AuthorityDiagnostics = append([]projectioncontract.GapDiagnostic(nil), item.AuthorityDiagnostics...)
	return item
}

func cloneWorkspaceViewModelSections(input map[string]projectioncontract.WorkspaceViewModelSection) map[string]projectioncontract.WorkspaceViewModelSection {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]projectioncontract.WorkspaceViewModelSection, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneGapBaseline(item projectioncontract.GapBaseline) projectioncontract.GapBaseline {
	item.CurrentFocus = cloneStringAnyMap(item.CurrentFocus)
	item.OpenCoverageWorkItems = cloneMapSlice(item.OpenCoverageWorkItems)
	item.AcceptedEvidenceRefs = append([]string(nil), item.AcceptedEvidenceRefs...)
	item.RejectedEvidenceRefs = append([]string(nil), item.RejectedEvidenceRefs...)
	item.TodoItems = append([]projectioncontract.GapTodoItem(nil), item.TodoItems...)
	item.Diagnostics = append([]projectioncontract.GapDiagnostic(nil), item.Diagnostics...)
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
	if item.HasActivity {
		ret.Activity = cloneWorkspaceActivityFeed(item.Activity)
	}
	return &ret
}

func cloneWorkspaceActivityFeed(feed projectioncontract.WorkspaceActivityFeed) projectioncontract.WorkspaceActivityFeed {
	ret := feed
	if feed.Items != nil {
		ret.Items = append([]projectioncontract.WorkspaceActivityItem(nil), feed.Items...)
	}
	return ret
}

func cloneContextBundle(item projectioncontract.ContextBundle) projectioncontract.ContextBundle {
	item.Sections = append([]projectioncontract.ContextSection(nil), item.Sections...)
	for idx := range item.Sections {
		item.Sections[idx].EvidenceRefs = append([]string(nil), item.Sections[idx].EvidenceRefs...)
		item.Sections[idx].Data = cloneStringAnyMap(item.Sections[idx].Data)
	}
	item.RecentEvidence = append([]projectioncontract.ContextEvidence(nil), item.RecentEvidence...)
	for idx := range item.RecentEvidence {
		item.RecentEvidence[idx].EvidenceRefs = append([]string(nil), item.RecentEvidence[idx].EvidenceRefs...)
		item.RecentEvidence[idx].Data = cloneStringAnyMap(item.RecentEvidence[idx].Data)
	}
	item.CompressionCandidates = append([]projectioncontract.ContextCompressionCandidate(nil), item.CompressionCandidates...)
	for idx := range item.CompressionCandidates {
		item.CompressionCandidates[idx].SourceRefs = append([]string(nil), item.CompressionCandidates[idx].SourceRefs...)
	}
	item.CompressedContext.Milestones = append([]projectioncontract.ProgressMilestone(nil), item.CompressedContext.Milestones...)
	item.CompressedContext.Blockers = append([]projectioncontract.ProgressBlocker(nil), item.CompressedContext.Blockers...)
	item.CompressedContext.RecentEvidence = append([]projectioncontract.ContextEvidence(nil), item.CompressedContext.RecentEvidence...)
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
