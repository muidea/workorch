package readmodelprojection

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

// ProjectWorkspaceRuntimeView builds the lightweight RuntimeView used by TUI
// active refresh and workspace-switch history recovery.
// Without a store-owned activity feed, ActivityRevision remains 0.
func ProjectWorkspaceRuntimeView(snapshot readmodelcore.WorkspaceSnapshot, activityLimit int, beforeCursor string) readmodelcore.WorkspaceRuntimeView {
	return ProjectWorkspaceRuntimeViewWithActivity(snapshot, readmodelcore.WorkspaceActivityFeed{}, false, activityLimit, beforeCursor)
}

// ProjectWorkspaceRuntimeViewWithActivity builds RuntimeView using a store-owned
// activity feed when available. ActivityRevision is never regenerated on read.
func ProjectWorkspaceRuntimeViewWithActivity(snapshot readmodelcore.WorkspaceSnapshot, storedActivity readmodelcore.WorkspaceActivityFeed, hasStoredActivity bool, activityLimit int, beforeCursor string) readmodelcore.WorkspaceRuntimeView {
	displayRunID := SelectDisplayRunID(snapshot)
	activeRunID := strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	var activity readmodelcore.WorkspaceActivityFeed
	if hasStoredActivity {
		activity = PageWorkspaceActivityFeed(storedActivity, activityLimit, beforeCursor)
	} else {
		// Transitional fallback: rebuild content only; revision stays 0.
		activity = ProjectWorkspaceActivityFeed(snapshot, activityLimit, beforeCursor)
	}
	if activity.DisplayRunID == "" {
		activity.DisplayRunID = displayRunID
	}

	progress := snapshot.TaskProgress
	if runtimeSummary := RuntimeV2ProgressSummary(progress.RuntimeV2); runtimeSummary != "" {
		// Runtime V2 progress must produce an observable summary when revision/focus/action change.
		progress.Summary = runtimeSummary
	}

	revision := WorkspaceRefreshRevisionFromSnapshot(snapshot, activity.ActivityRevision)
	revision.StreamGeneration = strings.TrimSpace(activity.StreamGeneration)
	blockingDecision := selectBlockingDecision(snapshot, activeRunID)
	blockingApproval := selectBlockingApproval(snapshot, activeRunID)
	runState := ""
	if run := SelectDisplayRun(snapshot); run != nil {
		runState = strings.TrimSpace(run.State)
	}
	if runState == "" {
		runState = strings.TrimSpace(progress.State)
	}

	return readmodelcore.WorkspaceRuntimeView{
		WorkspaceID:       strings.TrimSpace(snapshot.Workspace.ID),
		ActiveRunID:       activeRunID,
		DisplayRunID:      displayRunID,
		Projection:        snapshot.ViewModel,
		Revision:          revision,
		PromptMode:        firstNonEmptyTimelineString(snapshot.ViewModel.PromptMode, WorkspacePromptMode(snapshot)),
		RunState:          runState,
		TaskProgress:      progress,
		RunHeartbeat:      snapshot.RunHeartbeat,
		ConvergenceStatus: snapshot.ConvergenceStatus,
		Acceptance:        snapshot.Acceptance,
		RunCommand:        snapshot.RunCommand,
		BlockingDecision:  blockingDecision,
		BlockingApproval:  blockingApproval,
		Outcome:           snapshot.Outcome,
		Activity:          activity,
	}
}

// RuntimeV2ProgressSummary renders a compact progress line from Runtime V2.
func RuntimeV2ProgressSummary(runtime *readmodelcore.RuntimeV2Progress) string {
	if runtime == nil {
		return ""
	}
	parts := make([]string, 0, 6)
	parts = append(parts, "revision "+strconv.FormatUint(runtime.StateRevision, 10))
	parts = append(parts, "evidence "+strconv.FormatUint(runtime.EvidenceSequence, 10))
	if focus := strings.TrimSpace(runtime.CurrentFocusID); focus != "" {
		parts = append(parts, "focus "+focus)
	}
	if action := strings.TrimSpace(runtime.CurrentAction); action != "" {
		parts = append(parts, "action "+action)
	}
	parts = append(parts, fmt.Sprintf("dispatch %d", runtime.CompletedDispatchCount))
	parts = append(parts, fmt.Sprintf("attempts %d", runtime.CandidateAttemptCount))
	if runtime.AcceptedEvidenceCount > 0 || runtime.RejectedEvidenceCount > 0 {
		parts = append(parts, fmt.Sprintf("accepted %d rejected %d", runtime.AcceptedEvidenceCount, runtime.RejectedEvidenceCount))
	}
	if len(runtime.MissingEvidenceKinds) > 0 {
		parts = append(parts, "missing "+strings.Join(runtime.MissingEvidenceKinds, ","))
	}
	if msg := strings.TrimSpace(runtime.FailureMessage); msg != "" {
		parts = append(parts, msg)
	} else if class := strings.TrimSpace(runtime.FailureClass); class != "" {
		parts = append(parts, class)
	}
	return strings.Join(parts, " | ")
}

// WorkspaceRefreshRevisionFromSnapshot extracts multi-domain revisions.
func WorkspaceRefreshRevisionFromSnapshot(snapshot readmodelcore.WorkspaceSnapshot, activityRevision int64) readmodelcore.WorkspaceRefreshRevision {
	revision := readmodelcore.WorkspaceRefreshRevision{
		WorkspaceRevision:       snapshot.ViewModel.WorkspaceRevision,
		ProjectedSourceRevision: snapshot.ViewModel.ProjectedSourceRevision,
		ActivityRevision:        activityRevision,
	}
	if revision.WorkspaceRevision == 0 {
		revision.WorkspaceRevision = snapshot.ViewModel.Revision
	}
	if runtime := snapshot.TaskProgress.RuntimeV2; runtime != nil {
		revision.RuntimeStateRevision = runtime.StateRevision
		revision.EvidenceSequence = runtime.EvidenceSequence
	}
	// ActivityRevision remains its own domain. Do not fall back to source/runtime revisions.
	return revision
}

func selectBlockingDecision(snapshot readmodelcore.WorkspaceSnapshot, activeRunID string) *appmodelcore.Decision {
	ticketID := strings.TrimSpace(snapshot.Workspace.BlockingTicketID)
	if ticketID == "" && snapshot.Run != nil {
		ticketID = strings.TrimSpace(snapshot.Run.BlockingTicketID)
	}
	if ticketID == "" {
		return nil
	}
	for idx := range snapshot.Decisions {
		item := snapshot.Decisions[idx]
		if strings.TrimSpace(item.ID) != ticketID {
			continue
		}
		if activeRunID != "" && strings.TrimSpace(item.RunID) != "" && strings.TrimSpace(item.RunID) != activeRunID {
			continue
		}
		if !isOpenGovernanceState(item.State) {
			return nil
		}
		ret := item
		return &ret
	}
	return nil
}

func selectBlockingApproval(snapshot readmodelcore.WorkspaceSnapshot, activeRunID string) *appmodelcore.Approval {
	approvalID := strings.TrimSpace(snapshot.Workspace.BlockingApprovalID)
	if approvalID == "" && snapshot.Run != nil {
		approvalID = strings.TrimSpace(snapshot.Run.BlockingApprovalID)
	}
	if approvalID == "" {
		return nil
	}
	for idx := range snapshot.Approvals {
		item := snapshot.Approvals[idx]
		if strings.TrimSpace(item.ID) != approvalID {
			continue
		}
		if activeRunID != "" && strings.TrimSpace(item.RunID) != "" && strings.TrimSpace(item.RunID) != activeRunID {
			continue
		}
		if !isOpenGovernanceState(item.State) {
			return nil
		}
		ret := item
		return &ret
	}
	return nil
}

// DiffWorkspaceProjectionSections compares previous and candidate snapshots and
// returns only sections that actually changed. changeKind is used solely for
// coarse lifecycle events (overview/deleted/baseline) when no typed candidate
// content is available.
func DiffWorkspaceProjectionSections(previous, candidate readmodelcore.WorkspaceSnapshot, changeKind string) []string {
	changeKind = strings.ToLower(strings.TrimSpace(changeKind))
	sections := make([]string, 0, 8)
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		for _, existing := range sections {
			if existing == name {
				return
			}
		}
		sections = append(sections, name)
	}

	switch changeKind {
	case "overview", "baseline", "deleted":
		add("overview")
		return sections
	case "run_command_accepted", "run_command_rejected":
		add("overview")
		add("runtime_progress")
	}

	// First projection / empty previous: mark sections present in candidate content.
	if strings.TrimSpace(previous.Workspace.ID) == "" && strings.TrimSpace(candidate.Workspace.ID) != "" {
		add("overview")
		add("runtime_progress")
		add("activity")
		if candidate.TaskProgress.RuntimeV2 != nil {
			add("evidence")
		}
		if len(candidate.Decisions) > 0 || len(candidate.Approvals) > 0 ||
			strings.TrimSpace(candidate.Workspace.BlockingTicketID) != "" ||
			strings.TrimSpace(candidate.Workspace.BlockingApprovalID) != "" {
			add("governance")
		}
		if strings.TrimSpace(candidate.Outcome.State) != "" || len(candidate.Results) > 0 {
			add("result")
		}
		if len(candidate.Artifacts) > 0 || len(candidate.Patches) > 0 {
			add("artifacts")
		}
		if len(candidate.MemoryRecallTraces) > 0 {
			add("memory")
		}
		if len(candidate.LLMUsageTraces) > 0 || candidate.LLMUsageSummary.EntryTotal > 0 {
			add("llm_usage")
		}
		return sections
	}

	// Typed previous/candidate diffs for detail rebuilds.
	if previous.Workspace.Title != candidate.Workspace.Title ||
		previous.Workspace.State != candidate.Workspace.State ||
		previous.Workspace.ActiveRunID != candidate.Workspace.ActiveRunID ||
		previous.Workspace.BlockingTicketID != candidate.Workspace.BlockingTicketID ||
		previous.Workspace.BlockingApprovalID != candidate.Workspace.BlockingApprovalID {
		add("overview")
	}

	prevRuntime := previous.TaskProgress.RuntimeV2
	candRuntime := candidate.TaskProgress.RuntimeV2
	runtimeChanged := false
	switch {
	case prevRuntime == nil && candRuntime != nil:
		runtimeChanged = true
	case prevRuntime != nil && candRuntime == nil:
		runtimeChanged = true
	case prevRuntime != nil && candRuntime != nil:
		if prevRuntime.StateRevision != candRuntime.StateRevision ||
			prevRuntime.EvidenceSequence != candRuntime.EvidenceSequence ||
			prevRuntime.CurrentFocusID != candRuntime.CurrentFocusID ||
			prevRuntime.CurrentAction != candRuntime.CurrentAction ||
			prevRuntime.CompletedDispatchCount != candRuntime.CompletedDispatchCount ||
			prevRuntime.CandidateAttemptCount != candRuntime.CandidateAttemptCount ||
			prevRuntime.AcceptedEvidenceCount != candRuntime.AcceptedEvidenceCount ||
			prevRuntime.RejectedEvidenceCount != candRuntime.RejectedEvidenceCount {
			runtimeChanged = true
		}
	}
	if runtimeChanged || previous.TaskProgress.Summary != candidate.TaskProgress.Summary || previous.TaskProgress.State != candidate.TaskProgress.State {
		add("runtime_progress")
	}
	if (prevRuntime != nil || candRuntime != nil) && runtimeChanged {
		add("evidence")
	}

	// Compare activity content by typed fields, not fingerprint/hash revisions.
	prevActivity := BuildWorkspaceActivityFeedContent(previous)
	candActivity := BuildWorkspaceActivityFeedContent(candidate)
	if !ActivityFeedContentEqual(prevActivity, candActivity) {
		add("activity")
	}

	if previous.Workspace.BlockingTicketID != candidate.Workspace.BlockingTicketID ||
		previous.Workspace.BlockingApprovalID != candidate.Workspace.BlockingApprovalID ||
		!decisionIdentitiesEqual(previous.Decisions, candidate.Decisions) ||
		!approvalIdentitiesEqual(previous.Approvals, candidate.Approvals) {
		add("governance")
		add("activity")
	}

	if previous.Outcome.State != candidate.Outcome.State ||
		previous.Outcome.Summary != candidate.Outcome.Summary ||
		previous.Outcome.RunID != candidate.Outcome.RunID ||
		!resultIdentitiesEqual(previous.Results, candidate.Results) {
		add("result")
		add("runtime_progress")
		add("activity")
	}

	if !artifactIdentitiesEqual(previous.Artifacts, candidate.Artifacts) ||
		!patchIdentitiesEqual(previous.Patches, candidate.Patches) {
		add("artifacts")
		add("activity")
	}
	if !memoryRecallIdentitiesEqual(previous.MemoryRecallTraces, candidate.MemoryRecallTraces) {
		add("memory")
		add("activity")
	}
	if !llmUsageSummaryEqual(previous.LLMUsageSummary, candidate.LLMUsageSummary) ||
		!llmUsageIdentitiesEqual(previous.LLMUsageTraces, candidate.LLMUsageTraces) {
		add("llm_usage")
	}

	// No ChangeKind fallback. Empty sections means no typed content changed.
	// First build / empty previous is handled by the typed comparisons above
	// (nil previous fields differ from candidate content and mark sections).
	return sections
}

// Identity equality helpers compare ordered typed projection facts by stable
// display/contract fields. Same-length slices with content mutations must still
// mark ChangedSections (RR05/CR02). Map-bearing fields are projected to sorted
// key/value comparison facts — no JSON marshal, reflect, or map authority.

func decisionIdentitiesEqual(a, b []readmodelcore.Decision) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !decisionEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func decisionEqual(a, b readmodelcore.Decision) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.StepID) == strings.TrimSpace(b.StepID) &&
		strings.TrimSpace(a.Reason) == strings.TrimSpace(b.Reason) &&
		strings.TrimSpace(a.DecisionKind) == strings.TrimSpace(b.DecisionKind) &&
		strings.TrimSpace(a.Question) == strings.TrimSpace(b.Question) &&
		a.Context.Equal(b.Context) &&
		strings.TrimSpace(a.FailureClass) == strings.TrimSpace(b.FailureClass) &&
		strings.TrimSpace(a.FailureSignature) == strings.TrimSpace(b.FailureSignature) &&
		stringSliceEqual(a.AllowedRecoveryActions, b.AllowedRecoveryActions) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		strings.TrimSpace(a.ResolutionID) == strings.TrimSpace(b.ResolutionID) &&
		strings.TrimSpace(a.ResolutionType) == strings.TrimSpace(b.ResolutionType) &&
		a.ResolutionPayload.Equal(b.ResolutionPayload) &&
		strings.TrimSpace(a.ContinuationAction) == strings.TrimSpace(b.ContinuationAction) &&
		strings.TrimSpace(a.ResumeRunID) == strings.TrimSpace(b.ResumeRunID) &&
		strings.TrimSpace(a.SuccessorRunID) == strings.TrimSpace(b.SuccessorRunID) &&
		autoGovernanceEqual(a.AutoGovernance, b.AutoGovernance) &&
		strings.TrimSpace(a.ResolvedAt) == strings.TrimSpace(b.ResolvedAt) &&
		strings.TrimSpace(a.ResolvedBy) == strings.TrimSpace(b.ResolvedBy) &&
		strings.TrimSpace(a.CreatedAt) == strings.TrimSpace(b.CreatedAt) &&
		strings.TrimSpace(a.UpdatedAt) == strings.TrimSpace(b.UpdatedAt)
}

func approvalIdentitiesEqual(a, b []readmodelcore.Approval) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !approvalEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func approvalEqual(a, b readmodelcore.Approval) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.StepID) == strings.TrimSpace(b.StepID) &&
		strings.TrimSpace(a.Reason) == strings.TrimSpace(b.Reason) &&
		strings.TrimSpace(a.Action) == strings.TrimSpace(b.Action) &&
		a.Context.Equal(b.Context) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		autoGovernanceEqual(a.AutoGovernance, b.AutoGovernance) &&
		strings.TrimSpace(a.ResolvedAt) == strings.TrimSpace(b.ResolvedAt) &&
		strings.TrimSpace(a.ResolvedBy) == strings.TrimSpace(b.ResolvedBy) &&
		strings.TrimSpace(a.CreatedAt) == strings.TrimSpace(b.CreatedAt) &&
		strings.TrimSpace(a.UpdatedAt) == strings.TrimSpace(b.UpdatedAt)
}

func resultIdentitiesEqual(a, b []readmodelcore.Result) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !resultEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func resultEqual(a, b readmodelcore.Result) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.GoalID) == strings.TrimSpace(b.GoalID) &&
		strings.TrimSpace(a.DoDID) == strings.TrimSpace(b.DoDID) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		strings.TrimSpace(a.Title) == strings.TrimSpace(b.Title) &&
		strings.TrimSpace(a.Summary) == strings.TrimSpace(b.Summary) &&
		strings.TrimSpace(a.SourceType) == strings.TrimSpace(b.SourceType) &&
		strings.TrimSpace(a.SourceRef) == strings.TrimSpace(b.SourceRef) &&
		stringSliceEqual(a.EvidenceRefs, b.EvidenceRefs) &&
		stringSliceEqual(a.ArtifactRefs, b.ArtifactRefs) &&
		doDAssessmentEqual(a.DoDAssessment, b.DoDAssessment) &&
		resultSummaryEqual(a.SummaryDetail, b.SummaryDetail) &&
		strings.TrimSpace(a.CreatedAt) == strings.TrimSpace(b.CreatedAt) &&
		strings.TrimSpace(a.UpdatedAt) == strings.TrimSpace(b.UpdatedAt)
}

func artifactIdentitiesEqual(a, b []readmodelcore.Artifact) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !artifactEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func artifactEqual(a, b readmodelcore.Artifact) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.ResultID) == strings.TrimSpace(b.ResultID) &&
		strings.TrimSpace(a.StepID) == strings.TrimSpace(b.StepID) &&
		strings.TrimSpace(a.ToolCallID) == strings.TrimSpace(b.ToolCallID) &&
		strings.TrimSpace(a.PatchID) == strings.TrimSpace(b.PatchID) &&
		strings.TrimSpace(a.ValidationResultID) == strings.TrimSpace(b.ValidationResultID) &&
		strings.TrimSpace(a.ArtifactType) == strings.TrimSpace(b.ArtifactType) &&
		strings.TrimSpace(a.Category) == strings.TrimSpace(b.Category) &&
		strings.TrimSpace(a.Name) == strings.TrimSpace(b.Name) &&
		strings.TrimSpace(a.Summary) == strings.TrimSpace(b.Summary) &&
		strings.TrimSpace(a.URI) == strings.TrimSpace(b.URI) &&
		strings.TrimSpace(a.ContentType) == strings.TrimSpace(b.ContentType) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		stringSliceEqual(a.EvidenceRefs, b.EvidenceRefs) &&
		stableMapFactsEqual(a.Metadata, b.Metadata) &&
		strings.TrimSpace(a.CreatedAt) == strings.TrimSpace(b.CreatedAt) &&
		strings.TrimSpace(a.UpdatedAt) == strings.TrimSpace(b.UpdatedAt)
}

func patchIdentitiesEqual(a, b []readmodelcore.Patch) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !patchEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func patchEqual(a, b readmodelcore.Patch) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.StepID) == strings.TrimSpace(b.StepID) &&
		strings.TrimSpace(a.ToolCallID) == strings.TrimSpace(b.ToolCallID) &&
		strings.TrimSpace(a.ApprovalID) == strings.TrimSpace(b.ApprovalID) &&
		strings.TrimSpace(a.RevertApprovalID) == strings.TrimSpace(b.RevertApprovalID) &&
		strings.TrimSpace(a.PatchType) == strings.TrimSpace(b.PatchType) &&
		strings.TrimSpace(a.TargetRef) == strings.TrimSpace(b.TargetRef) &&
		strings.TrimSpace(a.Summary) == strings.TrimSpace(b.Summary) &&
		strings.TrimSpace(a.State) == strings.TrimSpace(b.State) &&
		a.AddedLines == b.AddedLines &&
		a.DeletedLines == b.DeletedLines &&
		strings.TrimSpace(a.BeforeHash) == strings.TrimSpace(b.BeforeHash) &&
		strings.TrimSpace(a.AfterHash) == strings.TrimSpace(b.AfterHash) &&
		stringSliceEqual(a.AffectedFiles, b.AffectedFiles) &&
		strings.TrimSpace(a.RiskLevel) == strings.TrimSpace(b.RiskLevel) &&
		stringSliceEqual(a.RiskReasons, b.RiskReasons) &&
		stableMapSliceFactsEqual(a.Diagnostics, b.Diagnostics) &&
		strings.TrimSpace(a.CreatedAt) == strings.TrimSpace(b.CreatedAt) &&
		strings.TrimSpace(a.AppliedAt) == strings.TrimSpace(b.AppliedAt) &&
		strings.TrimSpace(a.RevertedAt) == strings.TrimSpace(b.RevertedAt)
}

func memoryRecallIdentitiesEqual(a, b []readmodelcore.MemoryRecallTrace) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !memoryRecallEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func memoryRecallEqual(a, b readmodelcore.MemoryRecallTrace) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.StepID) == strings.TrimSpace(b.StepID) &&
		strings.TrimSpace(a.GraphNodeID) == strings.TrimSpace(b.GraphNodeID) &&
		strings.TrimSpace(a.Stage) == strings.TrimSpace(b.Stage) &&
		strings.TrimSpace(a.Source) == strings.TrimSpace(b.Source) &&
		strings.TrimSpace(a.Policy) == strings.TrimSpace(b.Policy) &&
		strings.TrimSpace(a.Status) == strings.TrimSpace(b.Status) &&
		strings.TrimSpace(a.ProviderID) == strings.TrimSpace(b.ProviderID) &&
		strings.TrimSpace(a.Operation) == strings.TrimSpace(b.Operation) &&
		strings.TrimSpace(a.QueryTextPreview) == strings.TrimSpace(b.QueryTextPreview) &&
		strings.TrimSpace(a.QueryTextSha256) == strings.TrimSpace(b.QueryTextSha256) &&
		a.MinScore == b.MinScore &&
		a.Limit == b.Limit &&
		strings.TrimSpace(a.EmbeddingModel) == strings.TrimSpace(b.EmbeddingModel) &&
		a.VectorDimension == b.VectorDimension &&
		strings.TrimSpace(a.RefreshSource) == strings.TrimSpace(b.RefreshSource) &&
		strings.TrimSpace(a.SelectionPolicyVersion) == strings.TrimSpace(b.SelectionPolicyVersion) &&
		stringSliceEqual(a.CandidateMemoryIDs, b.CandidateMemoryIDs) &&
		stringSliceEqual(a.SelectedMemoryIDs, b.SelectedMemoryIDs) &&
		stringSliceEqual(a.ExcludedMemoryIDs, b.ExcludedMemoryIDs) &&
		intMapFactsEqual(a.TypeQuota, b.TypeQuota) &&
		memorySelectionScoresEqual(a.SelectionScores, b.SelectionScores) &&
		stringSliceEqual(a.RetrievedMemoryIDs, b.RetrievedMemoryIDs) &&
		memoryHitsEqual(a.RetrievedMemoryHits, b.RetrievedMemoryHits) &&
		strings.TrimSpace(a.Reason) == strings.TrimSpace(b.Reason) &&
		strings.TrimSpace(a.OccurredAt) == strings.TrimSpace(b.OccurredAt) &&
		strings.TrimSpace(a.EvidenceRef) == strings.TrimSpace(b.EvidenceRef)
}

func llmUsageIdentitiesEqual(a, b []readmodelcore.LLMUsageTrace) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !llmUsageEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func llmUsageEqual(a, b readmodelcore.LLMUsageTrace) bool {
	return strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID) &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.RunID) == strings.TrimSpace(b.RunID) &&
		strings.TrimSpace(a.StepID) == strings.TrimSpace(b.StepID) &&
		strings.TrimSpace(a.ProviderID) == strings.TrimSpace(b.ProviderID) &&
		strings.TrimSpace(a.Provider) == strings.TrimSpace(b.Provider) &&
		strings.TrimSpace(a.APISurface) == strings.TrimSpace(b.APISurface) &&
		strings.TrimSpace(a.Model) == strings.TrimSpace(b.Model) &&
		strings.TrimSpace(a.Operation) == strings.TrimSpace(b.Operation) &&
		a.InputTokens == b.InputTokens &&
		a.OutputTokens == b.OutputTokens &&
		a.TotalTokens == b.TotalTokens &&
		a.Estimated == b.Estimated &&
		strings.TrimSpace(a.UsageSource) == strings.TrimSpace(b.UsageSource) &&
		a.RequestBytes == b.RequestBytes &&
		a.ResponseBytes == b.ResponseBytes &&
		a.Streaming == b.Streaming &&
		a.ContextBudget.Equal(b.ContextBudget) &&
		a.CacheUsage.Equal(b.CacheUsage) &&
		a.CacheMarkers.Equal(b.CacheMarkers) &&
		strings.TrimSpace(a.StartedAt) == strings.TrimSpace(b.StartedAt) &&
		strings.TrimSpace(a.CompletedAt) == strings.TrimSpace(b.CompletedAt) &&
		a.DurationMs == b.DurationMs &&
		strings.TrimSpace(a.Status) == strings.TrimSpace(b.Status) &&
		strings.TrimSpace(a.FailureClass) == strings.TrimSpace(b.FailureClass) &&
		strings.TrimSpace(a.OccurredAt) == strings.TrimSpace(b.OccurredAt) &&
		strings.TrimSpace(a.EvidenceRef) == strings.TrimSpace(b.EvidenceRef)
}

func llmUsageSummaryEqual(a, b readmodelcore.LLMUsageSummary) bool {
	if a.EntryTotal != b.EntryTotal ||
		a.InputTokens != b.InputTokens ||
		a.OutputTokens != b.OutputTokens ||
		a.TotalTokens != b.TotalTokens ||
		a.EstimatedEntries != b.EstimatedEntries ||
		a.ProviderNativeEntries != b.ProviderNativeEntries ||
		a.FailedEntries != b.FailedEntries ||
		a.CacheReadTokens != b.CacheReadTokens ||
		a.CacheWriteTokens != b.CacheWriteTokens ||
		a.CacheCreationTokens != b.CacheCreationTokens ||
		a.CacheableInputTokens != b.CacheableInputTokens ||
		a.NonCacheableInputTokens != b.NonCacheableInputTokens ||
		a.CacheReadInputRatio != b.CacheReadInputRatio ||
		a.CacheableInputRatio != b.CacheableInputRatio ||
		a.NonCacheableInputRatio != b.NonCacheableInputRatio ||
		a.CacheEstimatedEntries != b.CacheEstimatedEntries ||
		a.CacheProviderEntries != b.CacheProviderEntries {
		return false
	}
	if len(a.ProviderModelBreakdown) != len(b.ProviderModelBreakdown) {
		return false
	}
	for i := range a.ProviderModelBreakdown {
		left := a.ProviderModelBreakdown[i]
		right := b.ProviderModelBreakdown[i]
		if strings.TrimSpace(left.ProviderID) != strings.TrimSpace(right.ProviderID) ||
			strings.TrimSpace(left.Provider) != strings.TrimSpace(right.Provider) ||
			strings.TrimSpace(left.APISurface) != strings.TrimSpace(right.APISurface) ||
			strings.TrimSpace(left.Model) != strings.TrimSpace(right.Model) ||
			strings.TrimSpace(left.ProviderMode) != strings.TrimSpace(right.ProviderMode) ||
			left.EntryTotal != right.EntryTotal ||
			left.InputTokens != right.InputTokens ||
			left.OutputTokens != right.OutputTokens ||
			left.TotalTokens != right.TotalTokens ||
			left.CacheReadTokens != right.CacheReadTokens ||
			left.CacheWriteTokens != right.CacheWriteTokens ||
			left.CacheCreationTokens != right.CacheCreationTokens ||
			left.CacheablePromptCount != right.CacheablePromptCount ||
			left.ActualCacheHitCount != right.ActualCacheHitCount ||
			left.CacheCreationCount != right.CacheCreationCount ||
			left.PromptCacheKeySentCount != right.PromptCacheKeySentCount ||
			left.CacheControlSentCount != right.CacheControlSentCount ||
			left.AutoPrefixExpectedCount != right.AutoPrefixExpectedCount ||
			left.CacheReadInputRatio != right.CacheReadInputRatio ||
			left.EstimatedEntries != right.EstimatedEntries ||
			left.FailedEntries != right.FailedEntries {
			return false
		}
	}
	return true
}

func autoGovernanceEqual(a, b *governancecore.AutoGovernanceSummary) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return strings.TrimSpace(a.TraceID) == strings.TrimSpace(b.TraceID) &&
		strings.TrimSpace(a.Action) == strings.TrimSpace(b.Action) &&
		strings.TrimSpace(a.Status) == strings.TrimSpace(b.Status) &&
		strings.TrimSpace(a.RiskLevel) == strings.TrimSpace(b.RiskLevel) &&
		strings.TrimSpace(a.PolicyMatched) == strings.TrimSpace(b.PolicyMatched) &&
		strings.TrimSpace(a.PolicyBlocked) == strings.TrimSpace(b.PolicyBlocked) &&
		a.Confidence == b.Confidence &&
		strings.TrimSpace(a.Reason) == strings.TrimSpace(b.Reason) &&
		stringSliceEqual(a.EvidenceRefs, b.EvidenceRefs) &&
		strings.TrimSpace(a.PromptFingerprint) == strings.TrimSpace(b.PromptFingerprint) &&
		strings.TrimSpace(a.OccurredAt) == strings.TrimSpace(b.OccurredAt)
}

func doDAssessmentEqual(a, b *readmodelcore.DoDAssessment) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Satisfied != b.Satisfied || strings.TrimSpace(a.Summary) != strings.TrimSpace(b.Summary) {
		return false
	}
	if !doDAssessmentItemsEqual(a.Items, b.Items) {
		return false
	}
	return doDCoverageEqual(a.GoalCoverage, b.GoalCoverage) &&
		doDCoverageEqual(a.AcceptanceDimensions, b.AcceptanceDimensions) &&
		doDCoverageEqual(a.DeliverableCoverage, b.DeliverableCoverage)
}

func doDAssessmentItemsEqual(a, b []readmodelcore.DoDAssessmentItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i].Item) != strings.TrimSpace(b[i].Item) ||
			strings.TrimSpace(a[i].State) != strings.TrimSpace(b[i].State) ||
			!stringSliceEqual(a[i].EvidenceRefs, b[i].EvidenceRefs) ||
			strings.TrimSpace(a[i].FailureReason) != strings.TrimSpace(b[i].FailureReason) ||
			a[i].Confidence != b[i].Confidence ||
			strings.TrimSpace(a[i].SourceNote) != strings.TrimSpace(b[i].SourceNote) {
			return false
		}
	}
	return true
}

func doDCoverageEqual(a, b *readmodelcore.DoDCoverage) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Satisfied != b.Satisfied || strings.TrimSpace(a.Summary) != strings.TrimSpace(b.Summary) {
		return false
	}
	if len(a.Items) != len(b.Items) {
		return false
	}
	for i := range a.Items {
		if strings.TrimSpace(a.Items[i].ID) != strings.TrimSpace(b.Items[i].ID) ||
			strings.TrimSpace(a.Items[i].Deliverable) != strings.TrimSpace(b.Items[i].Deliverable) ||
			strings.TrimSpace(a.Items[i].Name) != strings.TrimSpace(b.Items[i].Name) ||
			strings.TrimSpace(a.Items[i].Path) != strings.TrimSpace(b.Items[i].Path) ||
			strings.TrimSpace(a.Items[i].Goal) != strings.TrimSpace(b.Items[i].Goal) ||
			strings.TrimSpace(a.Items[i].GoalItem) != strings.TrimSpace(b.Items[i].GoalItem) ||
			strings.TrimSpace(a.Items[i].Item) != strings.TrimSpace(b.Items[i].Item) ||
			strings.TrimSpace(a.Items[i].Requirement) != strings.TrimSpace(b.Items[i].Requirement) ||
			strings.TrimSpace(a.Items[i].State) != strings.TrimSpace(b.Items[i].State) ||
			!stringSliceEqual(a.Items[i].EvidenceRefs, b.Items[i].EvidenceRefs) {
			return false
		}
	}
	return true
}

func resultSummaryEqual(a, b *readmodelcore.ResultSummary) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return strings.TrimSpace(a.GoalOutcome) == strings.TrimSpace(b.GoalOutcome) &&
		doDAssessmentItemsEqual(a.DoDItems, b.DoDItems) &&
		strings.TrimSpace(a.ImplementationSummary) == strings.TrimSpace(b.ImplementationSummary) &&
		strings.TrimSpace(a.VerificationSummary) == strings.TrimSpace(b.VerificationSummary) &&
		stringSliceEqual(a.EvidenceRefs, b.EvidenceRefs) &&
		stringSliceEqual(a.ArtifactRefs, b.ArtifactRefs) &&
		stringSliceEqual(a.KnownLimitations, b.KnownLimitations) &&
		stringSliceEqual(a.ResidualRisks, b.ResidualRisks) &&
		stringSliceEqual(a.NextActions, b.NextActions) &&
		stableMapFactsEqual(a.Diagnostics, b.Diagnostics)
}

func memorySelectionScoresEqual(a, b []readmodelcore.MemoryRecallSelectionScore) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i].ID) != strings.TrimSpace(b[i].ID) ||
			a[i].Score != b[i].Score ||
			strings.TrimSpace(a[i].Reason) != strings.TrimSpace(b[i].Reason) {
			return false
		}
	}
	return true
}

func memoryHitsEqual(a, b []readmodelcore.MemoryRecallHit) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i].ID) != strings.TrimSpace(b[i].ID) ||
			strings.TrimSpace(a[i].Summary) != strings.TrimSpace(b[i].Summary) ||
			a[i].VectorScore != b[i].VectorScore ||
			a[i].VectorDimension != b[i].VectorDimension {
			return false
		}
	}
	return true
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}

func intMapFactsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for key, left := range a {
		right, ok := b[key]
		if !ok || left != right {
			return false
		}
	}
	return true
}

// stableMapFactsEqual projects map[string]any into sorted key/value comparison
// facts without JSON/reflect authority. Unsupported owner payload values are
// deliberately treated as unequal: a conservative changed section is safe,
// whereas a generic string fallback could silently hide a real change.
func stableMapFactsEqual(a, b map[string]any) bool {
	left, leftSupported := stableMapFactPairsWithSupport(a)
	right, rightSupported := stableMapFactPairsWithSupport(b)
	if !leftSupported || !rightSupported {
		return false
	}
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func stableMapSliceFactsEqual(a, b []map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !stableMapFactsEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func stableMapFactPairs(input map[string]any) []string {
	pairs, _ := stableMapFactPairsWithSupport(input)
	return pairs
}

func stableMapFactPairsWithSupport(input map[string]any) ([]string, bool) {
	if len(input) == 0 {
		return nil, true
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		// Length-prefix keys/values so embedded '=' or separators cannot collide (FR06).
		value, supported := stableAnyFactWithSupport(input[key])
		if !supported {
			return nil, false
		}
		pairs = append(pairs, lengthPrefixed(key)+"="+value)
	}
	return pairs, true
}

// lengthPrefixed encodes s as "<len>:<s>" so delimiter-bearing content cannot
// collide with multi-element joins (FR06).
func lengthPrefixed(s string) string {
	return strconv.Itoa(len(s)) + ":" + s
}

func stableAnyFact(value any) string {
	fact, _ := stableAnyFactWithSupport(value)
	return fact
}

// stableAnyFactWithSupport accepts only the JSON-shaped owner payload values
// that the projection explicitly understands. It never falls back to
// fmt.Sprint: a fallback can collapse different Go values into the same fact.
func stableAnyFactWithSupport(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "null", true
	case string:
		return "s:" + lengthPrefixed(typed), true
	case bool:
		if typed {
			return "b:true", true
		}
		return "b:false", true
	case int:
		return "n:" + strconv.FormatInt(int64(typed), 10), true
	case int64:
		return "n:" + strconv.FormatInt(typed, 10), true
	case int32:
		return "n:" + strconv.FormatInt(int64(typed), 10), true
	case float64:
		return "n:" + strconv.FormatFloat(typed, 'g', -1, 64), true
	case float32:
		return "n:" + strconv.FormatFloat(float64(typed), 'g', -1, 64), true
	case []string:
		// Length-prefix each element and the count so ["a,b"] != ["a","b"].
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, lengthPrefixed(item))
		}
		return "as:" + strconv.Itoa(len(typed)) + ":[" + strings.Join(parts, ",") + "]", true
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			fact, supported := stableAnyFactWithSupport(item)
			if !supported {
				return "", false
			}
			parts = append(parts, lengthPrefixed(fact))
		}
		return "a:" + strconv.Itoa(len(typed)) + ":[" + strings.Join(parts, ",") + "]", true
	case map[string]any:
		pairs, supported := stableMapFactPairsWithSupport(typed)
		if !supported {
			return "", false
		}
		for idx := range pairs {
			pairs[idx] = lengthPrefixed(pairs[idx])
		}
		return "o:" + strconv.Itoa(len(pairs)) + ":{" + strings.Join(pairs, ",") + "}", true
	default:
		return "", false
	}
}
