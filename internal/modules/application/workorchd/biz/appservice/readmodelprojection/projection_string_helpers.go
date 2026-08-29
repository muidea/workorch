package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation; source files are listed to preserve review traceability.

// source: dod_leaf_coverage.go

func runRequiredDoDLeafItems(snapshot readmodelcore.RunSnapshotState) []string {
	return nil
}

// source: evidence_ref_governance.go

func addRunGovernanceEvidenceRefs(refs runEvidenceRefs, snapshot readmodelcore.RunSnapshotState) {
	for _, item := range snapshot.Decisions {
		refs.add(item.ID)
		refs.add(item.StepID)
	}
	for _, item := range snapshot.Approvals {
		refs.add(item.ID)
		refs.add(item.StepID)
	}
}

// source: evidence_ref_index.go

func runEvidenceRefIndex(snapshot readmodelcore.RunSnapshotState) map[string]struct{} {
	ret := runEvidenceRefs{}
	ret.add(snapshot.Run.ID)
	addRunRuntimeEvidenceRefs(ret, snapshot)
	addRunResultArtifactEvidenceRefs(ret, snapshot)
	addRunGovernanceEvidenceRefs(ret, snapshot)
	return ret
}

// source: evidence_ref_result_artifact.go

func addRunResultArtifactEvidenceRefs(refs runEvidenceRefs, snapshot readmodelcore.RunSnapshotState) {
	for _, item := range snapshot.Results {
		refs.add(item.ID)
		for _, ref := range item.EvidenceRefs {
			refs.add(ref)
		}
	}
	for _, item := range snapshot.Artifacts {
		refs.add(item.ID)
		refs.add(item.StepID)
		refs.add(item.ResultID)
		refs.add(item.ToolCallID)
		refs.add(item.PatchID)
		refs.add(item.ValidationResultID)
		for _, ref := range item.EvidenceRefs {
			refs.add(ref)
		}
	}
}

// source: evidence_ref_runtime.go

func addRunRuntimeEvidenceRefs(refs runEvidenceRefs, snapshot readmodelcore.RunSnapshotState) {
	for _, item := range snapshot.Steps {
		refs.add(item.ID)
	}
	for _, item := range snapshot.ToolCalls {
		refs.add(item.ID)
		refs.add(item.StepID)
	}
	for _, item := range snapshot.Patches {
		refs.add(item.ID)
		refs.add(item.StepID)
	}
	for _, item := range snapshot.ValidationResults {
		refs.add(item.ID)
		refs.add(item.StepID)
		refs.add(item.EvidenceRef)
	}
	for _, item := range snapshot.RuntimeCheckpoints {
		refs.add(item.ID)
		refs.add(item.CurrentStepID)
	}
	for _, item := range snapshot.MemoryRecallTraces {
		refs.add(item.ID)
		refs.add(item.StepID)
		refs.add(item.EvidenceRef)
	}
}

// source: execution_evidence_refs.go

func executionEvidenceRefsByStep(snapshot readmodelcore.RunSnapshotState) executionEvidenceRefs {
	ret := newExecutionEvidenceRefs()
	for _, item := range snapshot.Decisions {
		ret.add(ret.decisionsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.Approvals {
		ret.add(ret.approvalsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.ToolCalls {
		ret.add(ret.toolCallsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.Patches {
		ret.add(ret.patchesByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.ValidationResults {
		ret.add(ret.validationsByStep, item.StepID, item.ID, item.EvidenceRef)
	}
	for _, item := range snapshot.Results {
		if item.SourceType == "dod_assessment" {
			ret.add(ret.resultsByStep, item.SourceRef, append([]string{item.ID}, item.EvidenceRefs...)...)
		}
	}
	for _, item := range snapshot.Artifacts {
		ret.add(ret.artifactsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.MemoryRecallTraces {
		ret.add(ret.memoryRecallsByStep, item.StepID, item.ID, item.EvidenceRef)
		ret.add(ret.memoryIDsByStep, item.StepID, item.RetrievedMemoryIDs...)
	}
	ret.compact()
	return ret
}

// source: progress_evidence.go

func taskProgressEvidenceRefs(snapshot readmodelcore.RunSnapshotState) []string {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	refs := []string{snapshot.Run.ID}
	if snapshot.Step != nil {
		refs = append(refs, snapshot.Step.ID)
	}
	refs = append(refs, idsFromSteps(snapshot.Steps)...)
	refs = append(refs, idsFromToolCalls(snapshot.ToolCalls)...)
	refs = append(refs, idsFromPatches(snapshot.Patches)...)
	refs = append(refs, idsFromValidations(snapshot.ValidationResults)...)
	refs = append(refs, idsFromCheckpoints(snapshot.RuntimeCheckpoints)...)
	refs = append(refs, idsFromResults(results)...)
	refs = append(refs, idsFromArtifacts(artifacts)...)
	refs = append(refs, governanceEvidenceRefs(snapshot)...)
	refs = append(refs, committedRuntimeOperationEvidenceRefs(snapshot.Acceptance.Operations)...)
	return compactUniqueStrings(refs)
}

// source: progress_governance_evidence_refs.go

func governanceEvidenceRefs(snapshot readmodelcore.RunSnapshotState) []string {
	refs := make([]string, 0, len(snapshot.Decisions)+len(snapshot.Approvals))
	for _, item := range snapshot.Decisions {
		refs = append(refs, item.ID, item.StepID, item.ResolutionID)
	}
	for _, item := range snapshot.Approvals {
		refs = append(refs, item.ID, item.StepID)
	}
	return compactUniqueStrings(refs)
}

// source: progress_governance_state.go

func governanceProgressState(snapshot readmodelcore.RunSnapshotState) string {
	if acceptedTerminalRun(snapshot) {
		return "completed"
	}
	if len(snapshot.Decisions) == 0 && len(snapshot.Approvals) == 0 {
		return "pending"
	}
	if hasOpenDecisionBlocker(snapshot.Decisions, snapshot.Run.WorkspaceID, snapshot.Run.ID, snapshot.Run.BlockingTicketID) ||
		hasOpenApprovalBlocker(snapshot.Approvals, snapshot.Run.WorkspaceID, snapshot.Run.ID, snapshot.Run.BlockingApprovalID) {
		return "blocked"
	}
	for _, item := range snapshot.Decisions {
		if !isResolvedDecisionState(item.State) {
			return "blocked"
		}
	}
	for _, item := range snapshot.Approvals {
		if !isResolvedApprovalState(item.State) {
			return "blocked"
		}
	}
	return "completed"
}

// source: repair_failure_projection.go

type repairFailureProjection struct {
	stepID      string
	class       string
	signature   string
	occurredAt  string
	evidenceRef string
}

func latestRepairFailureProjection(snapshot readmodelcore.RunSnapshotState) *repairFailureProjection {
	var latest *repairFailureProjection
	for _, checkpoint := range snapshot.RuntimeCheckpoints {
		candidate := repairFailureProjectionFromCheckpoint(checkpoint)
		if candidate == nil {
			continue
		}
		if latest == nil || candidate.occurredAt > latest.occurredAt || (candidate.occurredAt == latest.occurredAt && candidate.evidenceRef > latest.evidenceRef) {
			latest = candidate
		}
	}
	if latest != nil {
		return latest
	}
	for _, step := range snapshot.Steps {
		candidate := repairFailureProjectionFromStep(step)
		if candidate == nil {
			continue
		}
		latest = candidate
	}
	return latest
}

// source: run_observability.go

func mapRunObservability(snapshot readmodelcore.RunSnapshotState) readmodelcore.RunObservabilityState {
	timeline := runtimeTimeline(snapshot)
	return readmodelcore.RunObservabilityState{
		Run:                     snapshot.Run,
		DisplayOutcome:          snapshot.DisplayOutcome,
		GovernanceDisplay:       snapshot.GovernanceDisplay,
		ConsistencyDiagnostic:   projectioncontract.CloneConsistencyDiagnostic(snapshot.ConsistencyDiagnostic),
		Progress:                mapRunProgress(snapshot),
		TaskProgress:            snapshot.TaskProgress,
		RepairLoop:              snapshot.RepairLoop,
		Acceptance:              snapshot.Acceptance,
		TaskDecompositions:      snapshot.TaskDecompositions,
		TaskPlans:               snapshot.TaskPlans,
		ExecutionSections:       snapshot.ExecutionSections,
		LLMContextBudgets:       snapshot.LLMContextBudgets,
		LLMContextBudgetSummary: snapshot.LLMContextBudgetSummary,
		LLMUsageTraces:          snapshot.LLMUsageTraces,
		LLMUsageSummary:         snapshot.LLMUsageSummary,
		MemoryRecallTraces:      snapshot.MemoryRecallTraces,
		GitOperations:           snapshot.GitOperations,
		WebSources:              snapshot.WebSources,
		ClosureTraces:           snapshot.ClosureTraces,
		DoDEvidence:             snapshot.DoDEvidence,
		ResultQuality:           snapshot.ResultQuality,
		Evidence:                mapRuntimeEvidenceSummary(snapshot, len(timelineEvents(timeline))),
		LatestCheckpoint:        latestRuntimeCheckpoint(snapshot.RuntimeCheckpoints),
		SelectedCapabilities:    snapshot.SelectedCapabilities,
		UsedCapabilities:        snapshot.UsedCapabilities,
		Timeline:                timeline,
	}
}

// source: run_recovery_snapshot.go

func runRecoverySnapshotFromModels(input RunSnapshotModelInput) readmodelcore.RunSnapshotState {
	stepViews := mapSteps(input.Steps)
	currentStep := currentRunSnapshotStep(stepViews, "", "", mapRuntimeCheckpoints(input.Checkpoints))
	ret := baseRunSnapshotFromModels(input, stepViews, currentStep)
	ret.TaskProgress = mapLocalizedTaskProgress(ret)
	applyRuntimeProgress(&ret.TaskProgress, input.Run)
	ret.RepairLoop = ret.TaskProgress.RepairLoop
	ret.TaskPlans = mapTaskPlansFromSnapshot(ret)
	ret.DoDEvidence = mapDoDEvidence(ret)
	ret.ResultQuality = mapResultQuality(ret)
	applyRunSnapshotGovernanceProjection(&ret)
	return ret
}

// source: run_snapshot_detail_results.go

func projectRunSnapshotResultDetails(ret *readmodelcore.RunSnapshotState) {
	if ret == nil {
		return
	}

	projectRunSnapshotResultViews(ret)
	projectRunSnapshotPatchViews(ret)
	projectRunSnapshotValidationViews(ret)
	projectRunSnapshotArtifactViews(ret)
}

// source: snapshot_detail.go

func projectRunSnapshotDetail(snapshot readmodelcore.RunSnapshotState) readmodelcore.RunSnapshotState {
	ret := snapshot
	projectRunSnapshotStepDetails(&ret)
	projectRunSnapshotRuntimeDetails(&ret)
	projectRunSnapshotResultDetails(&ret)
	projectRunSnapshotGovernanceDetails(&ret)
	ret.ConvergenceStatus = mapConvergenceStatus(ret)
	return ret
}

func projectRunSnapshotCompact(snapshot readmodelcore.RunSnapshotState) readmodelcore.RunSnapshotState {
	ret := snapshot
	projectRunSnapshotStepDetails(&ret)
	ret.ConvergenceStatus = mapConvergenceStatus(ret)
	return ret
}

// source: timeline_step_occurrences.go

func timelineStepOccurrences(snapshot readmodelcore.RunSnapshotState) map[string]string {
	ret := map[string]string{}
	remember := func(stepID string, occurredAt string) {
		if stepID == "" || occurredAt == "" {
			return
		}
		if ret[stepID] == "" || occurredAt > ret[stepID] {
			ret[stepID] = occurredAt
		}
	}
	for _, item := range snapshot.EventRecords {
		stepID, ok := timelineStepIDFromEventRecord(item)
		if !ok {
			continue
		}
		remember(stepID, item.OccurredAt)
	}
	for _, item := range snapshot.Observed {
		stepID, ok := timelineStepIDFromObservedEvent(item)
		if !ok {
			continue
		}
		remember(stepID, item.OccurredAt)
	}
	for _, checkpoint := range snapshot.RuntimeCheckpoints {
		if !isStepRuntimeCheckpoint(checkpoint) {
			continue
		}
		remember(checkpoint.CurrentStepID, checkpoint.OccurredAt)
	}
	return ret
}
