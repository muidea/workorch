package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type workspaceProjectionSections []string

func (sections *workspaceProjectionSections) add(name string) {
	if sections == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	for _, existing := range *sections {
		if existing == name {
			return
		}
	}
	*sections = append(*sections, name)
}

// diffWorkspaceProjectionSections compares previous and candidate snapshots and
// returns only sections that actually changed. changeKind is used solely for
// coarse lifecycle events (overview/deleted/baseline) when no typed candidate
// content is available.
func diffWorkspaceProjectionSections(previous, candidate readmodelcore.WorkspaceSnapshotState, changeKind string) []string {
	sections := make(workspaceProjectionSections, 0, 8)
	if terminal := appendCoarseWorkspaceProjectionSections(&sections, changeKind); terminal {
		return sections
	}
	if appendInitialWorkspaceProjectionSections(&sections, previous, candidate) {
		return sections
	}

	appendWorkspaceOverviewDiff(&sections, previous, candidate)
	appendWorkspaceRuntimeDiff(&sections, previous, candidate)
	appendWorkspaceActivityDiff(&sections, previous, candidate)
	appendWorkspaceGovernanceDiff(&sections, previous, candidate)
	appendWorkspaceResultDiff(&sections, previous, candidate)
	appendWorkspaceArtifactDiff(&sections, previous, candidate)
	appendWorkspaceMemoryDiff(&sections, previous, candidate)
	appendWorkspaceLLMUsageDiff(&sections, previous, candidate)

	// No ChangeKind fallback. Empty sections means no typed content changed.
	return sections
}

func appendCoarseWorkspaceProjectionSections(sections *workspaceProjectionSections, changeKind string) bool {
	switch strings.ToLower(strings.TrimSpace(changeKind)) {
	case "overview", "baseline", "deleted":
		sections.add("overview")
		return true
	case "run_command_accepted", "run_command_rejected":
		sections.add("overview")
		sections.add("runtime_progress")
	}
	return false
}

func appendInitialWorkspaceProjectionSections(sections *workspaceProjectionSections, previous, candidate readmodelcore.WorkspaceSnapshotState) bool {
	if strings.TrimSpace(previous.Workspace.ID) != "" || strings.TrimSpace(candidate.Workspace.ID) == "" {
		return false
	}
	sections.add("overview")
	sections.add("runtime_progress")
	sections.add("activity")
	if candidate.TaskProgress.Runtime != nil {
		sections.add("evidence")
	}
	if len(candidate.Decisions) > 0 || len(candidate.Approvals) > 0 ||
		strings.TrimSpace(candidate.GovernanceDisplay.DecisionID) != "" ||
		strings.TrimSpace(candidate.GovernanceDisplay.ApprovalID) != "" {
		sections.add("governance")
	}
	if strings.TrimSpace(candidate.DisplayOutcome.State) != "" || len(candidate.Results) > 0 {
		sections.add("result")
	}
	if len(candidate.Artifacts) > 0 || len(candidate.Patches) > 0 {
		sections.add("artifacts")
	}
	if len(candidate.MemoryRecallTraces) > 0 {
		sections.add("memory")
	}
	if len(candidate.LLMUsageTraces) > 0 || candidate.LLMUsageSummary.EntryTotal > 0 {
		sections.add("llm_usage")
	}
	return true
}

func appendWorkspaceOverviewDiff(sections *workspaceProjectionSections, previous, candidate readmodelcore.WorkspaceSnapshotState) {
	if previous.Workspace.Title != candidate.Workspace.Title ||
		previous.Workspace.State != candidate.Workspace.State ||
		previous.Workspace.ActiveRunID != candidate.Workspace.ActiveRunID {
		sections.add("overview")
	}
}

func appendWorkspaceRuntimeDiff(sections *workspaceProjectionSections, previous, candidate readmodelcore.WorkspaceSnapshotState) {
	runtimeChanged := workspaceRuntimeProjectionChanged(previous, candidate)
	if runtimeChanged || previous.TaskProgress.Summary != candidate.TaskProgress.Summary || previous.TaskProgress.State != candidate.TaskProgress.State {
		sections.add("runtime_progress")
	}
	if (previous.TaskProgress.Runtime != nil || candidate.TaskProgress.Runtime != nil) && runtimeChanged {
		sections.add("evidence")
	}
}

func workspaceRuntimeProjectionChanged(previous, candidate readmodelcore.WorkspaceSnapshotState) bool {
	left := previous.TaskProgress.Runtime
	right := candidate.TaskProgress.Runtime
	if left == nil || right == nil {
		return left != right
	}
	return left.StateRevision != right.StateRevision ||
		left.EvidenceSequence != right.EvidenceSequence ||
		left.CurrentFocusID != right.CurrentFocusID ||
		left.ConvergenceState != right.ConvergenceState ||
		left.CompletedDispatchCount != right.CompletedDispatchCount ||
		left.CandidateAttemptCount != right.CandidateAttemptCount ||
		left.AcceptedEvidenceCount != right.AcceptedEvidenceCount ||
		left.RejectedEvidenceCount != right.RejectedEvidenceCount
}

func appendWorkspaceActivityDiff(sections *workspaceProjectionSections, previous, candidate readmodelcore.WorkspaceSnapshotState) {
	left := buildWorkspaceActivityFeedContent(previous)
	right := buildWorkspaceActivityFeedContent(candidate)
	if !activityFeedContentEqual(left, right) {
		sections.add("activity")
	}
}

func appendWorkspaceGovernanceDiff(sections *workspaceProjectionSections, previous, candidate readmodelcore.WorkspaceSnapshotState) {
	if governanceDisplayEqual(previous.GovernanceDisplay, candidate.GovernanceDisplay) &&
		consistencyDiagnosticEqual(previous.ConsistencyDiagnostic, candidate.ConsistencyDiagnostic) &&
		decisionIdentitiesEqual(previous.Decisions, candidate.Decisions) &&
		approvalIdentitiesEqual(previous.Approvals, candidate.Approvals) {
		return
	}
	sections.add("governance")
	sections.add("activity")
}

func governanceDisplayEqual(left, right projectioncontract.GovernanceDisplay) bool {
	return left.PromptMode == right.PromptMode && left.RunID == right.RunID &&
		left.DecisionID == right.DecisionID && left.ApprovalID == right.ApprovalID
}

func consistencyDiagnosticEqual(left, right *projectioncontract.ConsistencyDiagnostic) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func appendWorkspaceResultDiff(sections *workspaceProjectionSections, previous, candidate readmodelcore.WorkspaceSnapshotState) {
	if previous.DisplayOutcome.State == candidate.DisplayOutcome.State &&
		previous.DisplayOutcome.Summary == candidate.DisplayOutcome.Summary &&
		previous.DisplayOutcome.RunID == candidate.DisplayOutcome.RunID &&
		resultIdentitiesEqual(previous.Results, candidate.Results) {
		return
	}
	sections.add("result")
	sections.add("runtime_progress")
	sections.add("activity")
}

func appendWorkspaceArtifactDiff(sections *workspaceProjectionSections, previous, candidate readmodelcore.WorkspaceSnapshotState) {
	if artifactIdentitiesEqual(previous.Artifacts, candidate.Artifacts) && patchIdentitiesEqual(previous.Patches, candidate.Patches) {
		return
	}
	sections.add("artifacts")
	sections.add("activity")
}

func appendWorkspaceMemoryDiff(sections *workspaceProjectionSections, previous, candidate readmodelcore.WorkspaceSnapshotState) {
	if memoryRecallIdentitiesEqual(previous.MemoryRecallTraces, candidate.MemoryRecallTraces) {
		return
	}
	sections.add("memory")
	sections.add("activity")
}

func appendWorkspaceLLMUsageDiff(sections *workspaceProjectionSections, previous, candidate readmodelcore.WorkspaceSnapshotState) {
	if llmUsageSummaryEqual(previous.LLMUsageSummary, candidate.LLMUsageSummary) &&
		llmUsageIdentitiesEqual(previous.LLMUsageTraces, candidate.LLMUsageTraces) {
		return
	}
	sections.add("llm_usage")
}
