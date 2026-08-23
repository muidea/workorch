package readmodelprojection

import (
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
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	return resultProgressStateFromOutputs(snapshot, results, artifacts)
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

func resultProgressStateFromOutputs(snapshot readmodelcore.RunSnapshotState, results []projectioncontract.Result, artifacts []projectioncontract.Artifact) string {
	if len(results) == 0 && len(artifacts) == 0 {
		if isTerminalRunState(snapshot.Run.State) {
			return "failed"
		}
		return "pending"
	}
	for _, item := range results {
		if item.State == string(common.ResultStateAccepted) && isTerminalOutcomeEligibleResultForSnapshot(item, snapshot) {
			return "completed"
		}
	}
	if isTerminalRunState(snapshot.Run.State) {
		for _, item := range results {
			if item.State == string(common.ResultStateRejected) && isTerminalOutcomeEligibleResultForSnapshot(item, snapshot) {
				return "failed"
			}
		}
		if snapshot.Run.State == string(common.RunStateCompleted) {
			if snapshotHasActiveCoverageWorkItems(snapshot) {
				return "failed"
			}
			return "completed"
		}
		return "failed"
	}
	return "running"
}

// source: progress_workspace_items.go

func workspaceProgressMilestones(snapshot readmodelcore.WorkspaceSnapshotState, labels workspaceTaskProgressLabels) []projectioncontract.ProgressMilestone {
	return []projectioncontract.ProgressMilestone{
		{ID: "workspace_defined", Title: labels.WorkspaceDefined, State: "completed", EvidenceRefs: []string{snapshot.Workspace.ID}},
		{ID: "formal_result", Title: labels.FormalResult, State: progressStateFromCount(len(snapshot.Results), false, snapshot.Workspace.State == string(common.WorkspaceStateActive)), EvidenceRefs: idsFromResults(snapshot.Results)},
	}
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
