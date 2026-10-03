package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: progress_recovery_run_state.go

func applyRunStateRecovery(ret *projectioncontract.RecoveryPlan, snapshot readmodelcore.RunSnapshotState) {
	if ret == nil {
		return
	}
	switch common.RunState(snapshot.Run.State) {
	case common.RunStateCompleted:
		ret.Mode = "none"
		ret.Reason = "run completed"
	case common.RunStateFailed:
		ret.Mode = "inspect_failure"
		ret.Reason = snapshot.Run.FailureReason
	case common.RunStateCancelled, common.RunStateSuperseded:
		ret.Mode = "restart"
		ret.Reason = "run is terminal but not completed"
	default:
		ret.Mode = "resume"
		ret.Reason = "run is not terminal"
	}
}

// source: progress_result_state.go

func primaryTaskProgressResult(items []projectioncontract.Result) *projectioncontract.Result {
	var selected *projectioncontract.Result
	for idx := range items {
		if items[idx].State == string(common.ResultStateAccepted) && isFinalOutcomeResult(items[idx]) {
			if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
				selected = &items[idx]
			}
		}
	}
	if selected != nil {
		return selected
	}
	for idx := range items {
		if isIntermediateAcceptedEvidence(items[idx]) {
			continue
		}
		if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
			selected = &items[idx]
		}
	}
	return selected
}

func resultProgressState(snapshot readmodelcore.RunSnapshotState) string {
	return resultProgressStateFromOutputs(snapshot, snapshot.Results, nil)
}

func hasAcceptedRunResult(snapshot readmodelcore.RunSnapshotState) bool {
	for _, item := range runScopedResults(snapshot) {
		if item.State == string(common.ResultStateAccepted) && isTerminalOutcomeEligibleResultForSnapshot(item, snapshot) {
			return true
		}
	}
	return false
}

// source: progress_result_state_outputs.go

func resultProgressStateFromOutputs(snapshot readmodelcore.RunSnapshotState, results []projectioncontract.Result, _ []projectioncontract.Artifact) string {
	if result := exactOwnerTerminalResult(snapshot.Workspace, snapshot.Run, results); result != nil &&
		snapshot.Run.State == string(common.RunStateCompleted) && result.State == string(common.ResultStateAccepted) {
		return "completed"
	}
	if isTerminalRunState(snapshot.Run.State) {
		return "failed"
	}
	for _, item := range results {
		if resultMatchesRunScope(item, snapshot.Run) && isFinalOutcomeResult(item) {
			return "running"
		}
	}
	return "pending"
}

// exactOwnerTerminalResult observes the exact Result identity acknowledged by
// the Run owner. The startup seal, not coverage/artifacts or a latest-result
// heuristic, binds the immutable workspace/run/Goal/DoD scope.
func exactOwnerTerminalResult(workspace appmodelcore.Workspace, run appmodelcore.Run, results []projectioncontract.Result) *projectioncontract.Result {
	if strings.TrimSpace(workspace.ID) == "" || strings.TrimSpace(workspace.AdmittedStartupInputHash) == "" ||
		strings.TrimSpace(run.ID) == "" || run.WorkspaceID != workspace.ID || strings.TrimSpace(run.GoalID) == "" ||
		strings.TrimSpace(run.DoDID) == "" || run.StartupInputHash != workspace.AdmittedStartupInputHash ||
		strings.TrimSpace(run.TerminalResultID) == "" || !isTerminalRunState(run.State) {
		return nil
	}
	var selected *projectioncontract.Result
	for idx := range results {
		item := &results[idx]
		if item.ID != run.TerminalResultID {
			continue
		}
		// A duplicate exact identity is ambiguous even if one copy looks valid.
		if selected != nil || !resultMatchesRunScope(*item, run) || !isFinalOutcomeResult(*item) {
			return nil
		}
		selected = item
	}
	return selected
}

func resultMatchesRunScope(item projectioncontract.Result, run appmodelcore.Run) bool {
	return run.ID != "" && run.WorkspaceID != "" && run.GoalID != "" && run.DoDID != "" &&
		item.RunID == run.ID && item.WorkspaceID == run.WorkspaceID && item.GoalID == run.GoalID && item.DoDID == run.DoDID
}

// CurrentRun is the owner-selected observation. Without it (or an exact active
// owner identity), history is ordered only by the Run owner's monotonic fence.
// Unknown/tied generations cannot authorize selecting an accepted predecessor.
func workspaceFormalProgressRun(snapshot readmodelcore.WorkspaceSnapshotState) *appmodelcore.Run {
	if snapshot.Run != nil {
		return snapshot.Run
	}
	if snapshot.Workspace.ActiveRunID != "" {
		return workspaceRunByID(snapshot, snapshot.Workspace.ActiveRunID)
	}
	var selected *appmodelcore.Run
	ambiguous := false
	for idx := range snapshot.Runs {
		run := &snapshot.Runs[idx]
		if strings.TrimSpace(run.WorkspaceID) == "" {
			return nil
		}
		if run.WorkspaceID != snapshot.Workspace.ID {
			continue
		}
		if run.ID == "" || run.WorkspaceRunGeneration == 0 {
			return nil
		}
		if selected == nil || run.WorkspaceRunGeneration > selected.WorkspaceRunGeneration {
			selected = run
			ambiguous = false
		} else if run.WorkspaceRunGeneration == selected.WorkspaceRunGeneration && run.ID != selected.ID {
			ambiguous = true
		}
	}
	if ambiguous {
		return nil
	}
	return selected
}

func workspaceFormalRunSnapshot(snapshot readmodelcore.WorkspaceSnapshotState) *readmodelcore.RunSnapshotState {
	run := workspaceFormalProgressRun(snapshot)
	if run == nil {
		return nil
	}
	return &readmodelcore.RunSnapshotState{Workspace: snapshot.Workspace, Run: *run, Results: snapshot.Results, Artifacts: snapshot.Artifacts}
}

func preserveUnrelatedCachedProgress(progress projectioncontract.TaskProgress, results []projectioncontract.Result, terminalResultID string) bool {
	return len(progress.Milestones) == 0 && len(results) == 0 && terminalResultID == "" && progress.Runtime == nil && progress.Summary != ""
}

func finalizeRunFormalProgress(progress projectioncontract.TaskProgress, snapshot readmodelcore.RunSnapshotState) projectioncontract.TaskProgress {
	// Custom cached views without formal-result facts are not formal projection
	// authority. Preserve their unrelated summary rather than rewriting it.
	if preserveUnrelatedCachedProgress(progress, snapshot.Results, snapshot.Run.TerminalResultID) {
		return progress
	}
	progress.Milestones = append([]projectioncontract.ProgressMilestone(nil), progress.Milestones...)
	state := resultProgressState(snapshot)
	refs := []string(nil)
	if result := exactOwnerTerminalResult(snapshot.Workspace, snapshot.Run, snapshot.Results); result != nil {
		refs = []string{result.ID}
	}
	found := false
	for idx := range progress.Milestones {
		if progress.Milestones[idx].ID == "formal_result" {
			progress.Milestones[idx].State = state
			progress.Milestones[idx].EvidenceRefs = refs
			found = true
		}
	}
	if !found {
		progress.Milestones = append(progress.Milestones, projectioncontract.ProgressMilestone{
			ID: "formal_result", Title: localizationcore.MilestoneTitle(snapshotLanguage(snapshot), "formal_result"), State: state, EvidenceRefs: refs,
		})
	}
	progress.CurrentMilestone = currentProgressMilestone(progress.Milestones)
	progress.Summary = taskProgressSummary(progress, &snapshot)
	return progress
}

func finalizeWorkspaceFormalProgress(progress projectioncontract.TaskProgress, snapshot readmodelcore.WorkspaceSnapshotState) projectioncontract.TaskProgress {
	terminalResultID := ""
	if run := workspaceFormalProgressRun(snapshot); run != nil {
		terminalResultID = run.TerminalResultID
	}
	if preserveUnrelatedCachedProgress(progress, snapshot.Results, terminalResultID) {
		return progress
	}
	progress.Milestones = append([]projectioncontract.ProgressMilestone(nil), progress.Milestones...)
	defined := projectioncontract.ProgressMilestone{
		ID: "workspace_defined", Title: localizationcore.WorkspaceProgressText(workspaceSnapshotLanguage(snapshot), "workspace_defined"), State: "pending",
	}
	if strings.TrimSpace(snapshot.Workspace.AdmittedStartupInputHash) != "" {
		defined.State = "completed"
		defined.EvidenceRefs = []string{snapshot.Workspace.ID}
	}
	found := false
	for idx := range progress.Milestones {
		if progress.Milestones[idx].ID == defined.ID {
			progress.Milestones[idx] = defined
			found = true
		}
	}
	if !found {
		progress.Milestones = append([]projectioncontract.ProgressMilestone{defined}, progress.Milestones...)
	}
	if runSnapshot := workspaceFormalRunSnapshot(snapshot); runSnapshot != nil {
		progress.RunID = runSnapshot.Run.ID
		return finalizeRunFormalProgress(progress, *runSnapshot)
	}
	return finalizeRunFormalProgress(progress, readmodelcore.RunSnapshotState{Workspace: snapshot.Workspace})
}

// source: progress_workspace_items.go

func workspaceProgressMilestones(snapshot readmodelcore.WorkspaceSnapshotState, labels workspaceTaskProgressLabels) []projectioncontract.ProgressMilestone {
	ret := []projectioncontract.ProgressMilestone{
		{ID: "workspace_defined", Title: labels.WorkspaceDefined, State: "pending"},
		{ID: "formal_result", Title: labels.FormalResult, State: "pending"},
	}
	if strings.TrimSpace(snapshot.Workspace.AdmittedStartupInputHash) != "" {
		ret[0].State = "completed"
		ret[0].EvidenceRefs = []string{snapshot.Workspace.ID}
	}
	return ret
}

func workspaceProgressItems(snapshot readmodelcore.WorkspaceSnapshotState, labels workspaceTaskProgressLabels) []projectioncontract.ProgressItem {
	return []projectioncontract.ProgressItem{
		{ID: "runs", Kind: "run", Title: labels.Runs, State: progressStateFromCount(len(snapshot.Runs), false, false)},
		{ID: "results", Kind: "result", Title: labels.Results, State: progressStateFromCount(len(snapshot.Results), false, false), EvidenceRefs: idsFromResults(snapshot.Results)},
		{ID: "artifacts", Kind: "artifact", Title: labels.Artifacts, State: progressStateFromCount(len(snapshot.Artifacts), false, false), EvidenceRefs: idsFromArtifacts(snapshot.Artifacts)},
	}
}

// source: run_outcome.go

func mapRunOutcome(snapshot readmodelcore.RunSnapshotState) projectioncontract.RunDisplayOutcome {
	ret := projectioncontract.RunDisplayOutcome{
		RunID:    snapshot.Run.ID,
		State:    snapshot.Run.State,
		RawState: snapshot.Run.State,
		Terminal: isTerminalRunState(snapshot.Run.State),
	}
	if common.RunState(snapshot.Run.State) == common.RunStateSuperseded {
		ret.State = string(common.RunStateSuperseded)
		ret.Terminal = true
		ret.Reason = "run superseded"
		ret.Summary = ret.Reason
		return ret
	}
	if result := latestTerminalRunSnapshotResult(snapshot); result != nil {
		applyRunOutcomeTerminalResult(&ret, *result)
		return ret
	}
	applyRunOutcomeTerminalRunState(&ret, snapshot)
	return ret
}

func latestTerminalRunSnapshotResult(snapshot readmodelcore.RunSnapshotState) *projectioncontract.Result {
	var selected *projectioncontract.Result
	items := runScopedResults(snapshot)
	for idx := range items {
		switch common.ResultState(items[idx].State) {
		case common.ResultStateAccepted, common.ResultStateRejected:
		default:
			continue
		}
		if !isTerminalOutcomeEligibleResultForSnapshot(items[idx], snapshot) {
			continue
		}
		if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
			selected = &items[idx]
		}
	}
	return selected
}

// source: run_outcome_state.go

func applyRunOutcomeTerminalRunState(ret *projectioncontract.RunDisplayOutcome, snapshot readmodelcore.RunSnapshotState) {
	switch common.RunState(snapshot.Run.State) {
	case common.RunStateCompleted:
		ret.State = "failed"
		ret.Terminal = true
		ret.Reason = "run completed without accepted result"
	case common.RunStateFailed:
		ret.State = "failed"
		ret.Terminal = true
		ret.Reason = snapshot.Run.FailureReason
	case common.RunStateCancelled:
		ret.State = string(common.RunStateCancelled)
		ret.Terminal = true
		ret.Reason = "run cancelled"
	case common.RunStateSuperseded:
		ret.State = string(common.RunStateSuperseded)
		ret.Terminal = true
		ret.Reason = "run superseded"
	}
	if ret.Summary == "" {
		ret.Summary = ret.Reason
	}
}
