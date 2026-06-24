package orchestrator

import (
	"context"
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimeModel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeRepairFollowUpStepID(runID common.RunID, failedStep *runModel.Step, attempt int) common.StepID {
	base := ""
	if failedStep != nil {
		base = runtimeReadableStepLabel(runID, runtimeRepairIDLabelStepID(failedStep))
	}
	if base == "" {
		base = "repair"
	}
	if attempt < 1 {
		attempt = 1
	}
	return common.StepID(fmt.Sprintf("%s/repair-%03d-of-%s", runID, attempt, base))
}

func runtimeRepairIDLabelStepID(failedStep *runModel.Step) common.StepID {
	if failedStep == nil {
		return ""
	}
	if runtimeRepairIDShouldUseFailedStep(failedStep) {
		return failedStep.ID
	}
	if repairOf := common.StepID(strings.TrimSpace(stringFromMap(failedStep.Input, stepInputKeyRepairOfStepID))); runtimeRepairIDShouldUseRepairOfStepID(failedStep, repairOf) {
		return repairOf
	}
	if graphLineage := runtimeRepairGraphLineageLabelStepID(failedStep); graphLineage != "" {
		return graphLineage
	}
	return runtimeRepairRootStepID(failedStep)
}

func runtimeRepairIDShouldUseFailedStep(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	if boolValue(failedStep.Input[stepInputKeyProviderRetryFollowUp]) ||
		boolValue(failedStep.Input[stepInputKeyRuntimeRepairFollowUp]) ||
		boolValue(failedStep.Input["assessmentOnlyContractCorrection"]) {
		return false
	}
	if graphNodeID(failedStep) == "" {
		return false
	}
	return strings.Contains(strings.TrimSpace(string(failedStep.ID)), "/graph-")
}

func runtimeRepairIDShouldUseRepairOfStepID(failedStep *runModel.Step, repairOf common.StepID) bool {
	if failedStep == nil || repairOf == "" {
		return false
	}
	if boolValue(failedStep.Input[stepInputKeyProviderRetryFollowUp]) ||
		boolValue(failedStep.Input["assessmentOnlyContractCorrection"]) {
		return false
	}
	return strings.Contains(strings.TrimSpace(string(repairOf)), "/graph-")
}

func runtimeRepairGraphLineageLabelStepID(failedStep *runModel.Step) common.StepID {
	if failedStep == nil ||
		boolValue(failedStep.Input[stepInputKeyProviderRetryFollowUp]) ||
		boolValue(failedStep.Input["assessmentOnlyContractCorrection"]) {
		return ""
	}
	for _, candidate := range []common.StepID{
		common.StepID(strings.TrimSpace(stringFromMap(failedStep.Input, stepInputKeyRepairOfStepID))),
		common.StepID(strings.TrimSpace(stringFromMap(failedStep.Input, "failedStepId"))),
		failedStep.ID,
	} {
		if label := runtimeEmbeddedGraphLineageLabel(failedStep.RunID, candidate); label != "" {
			return label
		}
	}
	return ""
}

type runtimeRepairBaseInputDeps struct {
	policy     RuntimePolicy
	llmContext *llmContextSubsystem
}

func runtimeRepairBaseInputDepsFor(o *RuntimeOrchestrator) runtimeRepairBaseInputDeps {
	if o == nil {
		return runtimeRepairBaseInputDeps{}
	}
	return runtimeRepairBaseInputDeps{
		policy:     o.RuntimePolicy(),
		llmContext: o.llmContextManager(),
	}
}

func runtimeRepairBaseInput(
	deps runtimeRepairBaseInputDeps,
	run *runModel.Run,
	failedStep *runModel.Step,
	attempt int,
	execErr error,
) map[string]any {
	previousStepID := common.StepID("")
	if failedStep != nil {
		previousStepID = failedStep.ID
	}
	input := nextStepInput(run, previousStepID)
	input[stepInputKeySoulStage] = soulStageExecution
	setRuntimeStage(input, runtimeStageRepair)
	input[stepInputKeyRuntimeRepairFollowUp] = true
	input[stepInputKeyRuntimeRepairAttempt] = attempt
	input["maxRuntimeRepairAttempts"] = maxRuntimeRepairAttemptsForPolicy(deps.policy)
	if execErr != nil {
		input["failureReason"] = execErr.Error()
	}
	if failedStep == nil {
		return input
	}
	input["failedStepId"] = string(failedStep.ID)
	input[stepInputKeyRepairOfStepID] = string(failedStep.ID)
	rootStepID := runtimeRepairRootStepID(failedStep)
	input[stepInputKeyRepairOfRootStepID] = string(rootStepID)
	input[stepInputKeyRecoveryAction] = string(runtimeRecoveryActionRuntimeRepair)
	input[stepInputKeyRecoveryRootStepID] = string(rootStepID)
	input[stepInputKeyRecoveryLineageType] = "runtime_repair"
	input["failedStepKind"] = failedStep.Kind
	input["failedStepState"] = string(failedStep.State)
	llmContext := deps.llmContext
	if llmContext == nil {
		llmContext = newLLMContextSubsystem()
	}
	input["failedStepInput"] = llmContext.compactStepInput(failedStep.Input)
	input["failedStepOutput"] = llmContext.compactStepOutput(failedStep.Output)
	return input
}

func applyRuntimeRepairFailureProjection(input map[string]any, failedStep *runModel.Step) {
	if input == nil || failedStep == nil {
		return
	}
	applyRuntimeRepairGapBaselineProjection(input, failedStep)
	if diagnostics := runtimeRepairLastFailureDiagnostics(failedStep); len(diagnostics) > 0 {
		input["lastFailureDiagnostics"] = diagnostics
	}
	if diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		input["runtimeDiagnostics"] = cloneStepPayload(diagnostics)
		if stringFromMap(diagnostics, "kind") == "dod_evidence_quality_gap" {
			input["evidenceRepairRequired"] = true
			input["verificationEvidenceGap"] = cloneStepPayload(diagnostics)
			if doDEvidenceQualityGapRequiresStateChange(failedStep) {
				input["repairObjective"] = repairObjectiveStateChangingOutput
				input["stateChangingOutputRequired"] = true
				input["stateChangingEvidenceRepairRequired"] = true
				input["repairGuidance"] = stateChangingEvidenceQualityRepairGuidance()
			} else {
				input["repairObjective"] = repairObjectiveConcreteValidationEvidence
				input["repairGuidance"] = concreteValidationEvidenceRepairGuidance()
			}
		}
	}
	markCapabilityInputLocatorEvidenceRequired(input, failedStep)
	coverageContext := mapFromValue(failedStep.Output[coverageWorkItemContextKey])
	if len(coverageContext) == 0 {
		coverageContext = mapFromValue(mapFromValue(failedStep.Output["runtimeDiagnostics"])[coverageWorkItemContextKey])
	}
	if len(coverageContext) == 0 {
		return
	}
	applyCoverageWorkItemContext(input, coverageContext)
	if unresolved := coverageWorkItemUnresolvedItemsFromContext(coverageContext); len(unresolved) > 0 {
		input["unresolvedCoverageWorkItems"] = unresolved
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Resolve unresolvedCoverageWorkItems before returning an accepted result; each item requires its missing evidence kinds or a failed/skipped assessment with reason.",
			"For assessment_pending items, return a bounded dodAssessment that explicitly maps the item by id, sourceRequirementId, sourcePath, or targetText.",
			"For validation items, produce bounded validation evidence or mark the item failed/skipped; do not claim accepted completion while it remains unresolved.",
			"For state_change items, produce the smallest state-changing action that matches the coverage work item, then validate it.",
		)
	}
}

func applyRuntimeRepairGapBaselineProjection(input map[string]any, failedStep *runModel.Step) {
	if input == nil || failedStep == nil {
		return
	}
	if len(mapFromValue(input[gapBaselineKey])) > 0 {
		return
	}
	for _, candidate := range []map[string]any{
		mapFromValue(failedStep.Input[gapBaselineKey]),
		mapFromValue(failedStep.Output[gapBaselineKey]),
		mapFromValue(mapFromValue(failedStep.Output["runtimeDiagnostics"])[gapBaselineKey]),
	} {
		if len(candidate) == 0 {
			continue
		}
		input[gapBaselineKey] = cloneStepPayload(candidate)
		return
	}
}

func applyRuntimeRepairMissingCoverageProjection(
	ctx context.Context,
	steps repository.StepRepository,
	input map[string]any,
	run *runModel.Run,
	failedStep *runModel.Step,
	assessmentOnlyCoverageRepair bool,
) {
	if input == nil || failedStep == nil {
		return
	}
	if assessmentOnlyCoverageRepair {
		if missingGoalCoverage := missingGoalCoverageFromStep(failedStep); len(missingGoalCoverage) > 0 {
			if open := filterCoverageGapsOpenByWorkItems(input, missingGoalCoverage); len(open) > 0 {
				input["missingGoalCoverage"] = missingGoalCoverageWithEvidenceCandidates(ctx, steps, run, failedStep, open)
			}
		}
	}
	if missingDoD := missingDoDItemsFromStep(failedStep); len(missingDoD) > 0 {
		if open := filterCoverageGapsOpenByWorkItems(input, missingDoD); len(open) > 0 {
			input["missingDoDItems"] = missingDoDItemsWithEvidenceCandidates(ctx, steps, run, failedStep, open)
		}
	}
	if len(mapSliceFromValue(input["missingDoDItems"])) == 0 {
		if assessment := doDAssessmentFromOutput(failedStep.Output); len(assessment) > 0 {
			if missing := missingDoDItemsFromDoDAssessment(assessment); len(missing) > 0 {
				input["missingDoDItems"] = missingDoDItemsWithEvidenceCandidates(ctx, steps, run, failedStep, missing)
			}
		}
	}
	if missingDeliverableCoverage := missingDeliverableCoverageFromStep(failedStep); len(missingDeliverableCoverage) > 0 &&
		len(mapSliceFromValue(input["missingDeliverableCoverage"])) == 0 {
		if open := filterCoverageGapsOpenByWorkItems(input, missingDeliverableCoverage); len(open) > 0 {
			input["missingDeliverableCoverage"] = open
		}
	}
	if inheritCoverageRepairContext(input, failedStep) {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Preserve inherited coverage gaps from the previous repair attempt; a contract failure on the repair wrapper does not clear the original Goal/DoD/deliverable gaps.",
			"Use missingDoDItems, missingGoalCoverage, and missingDeliverableCoverage as the current unresolved scope unless current evidence explicitly closes an item.",
		)
	}
}

func applyRuntimeRepairAssessmentOnlyCoverageProjection(
	ctx context.Context,
	steps repository.StepRepository,
	evidence RuntimeEvidenceRepositories,
	input map[string]any,
	run *runModel.Run,
	failedStep *runModel.Step,
	assessmentOnlyCoverageRepair bool,
	coverageNeedsConcreteEvidence bool,
	validationRepairEscalatesToStateChanging bool,
) {
	if input == nil || failedStep == nil ||
		!assessmentOnlyCoverageRepair ||
		coverageNeedsConcreteEvidence ||
		validationRepairEscalatesToStateChanging {
		return
	}
	input[stepInputKeySoulStage] = soulStageValidation
	setRuntimeStage(input, runtimeStageVerification)
	input[stepInputKeyAssessmentOnly] = true
	input["completionFollowUp"] = true
	input["assessmentOnlyReason"] = "dod_coverage_contract_correction"
	input["repairObjective"] = "complete_dod_goal_coverage_assessment"
	input["repairGuidance"] = []string{
		"Return only completed or failed dodAssessment, or a real governance blocker.",
		"Do not return executionGraph, shellCommands, patchRequests, mcpToolCalls, memoryWrites, or other executable work.",
		"Preserve every still-valid DoD item assessment and add any missing DoD items exactly as listed in missingDoDItems.",
		"Add one independent dodAssessment.goalCoverage.items entry for every missingGoalCoverage entry; copy its goalRequirementId when present and cover the exact raw requirement without merging multiple missing entries into a broader item.",
		"Add one independent dodAssessment.deliverableCoverage.items entry for every missingDeliverableCoverage entry or required deliverableExpectation id; each satisfied entry must copy the exact id and cite evidenceRefs or artifactRefs.",
		"When a required deliverableExpectation is supplemental evidence, cite existing runtime result artifacts or concrete evidenceRefs; do not create report-only workspace files through patch/apply.",
		"When a required deliverableExpectation is primary state change, do not mark it satisfied from summary/report artifacts alone; return failed coverage with nextActions if the primary state-changing evidence is still missing.",
		"Set dodAssessment.deliverableCoverage.satisfied=true when every required deliverableCoverage item is satisfied/available/produced and cites evidenceRefs or artifactRefs; do not leave the parent flag false while all child items are satisfied.",
		"Set dodAssessment.satisfied=true only when every required DoD, Goal, acceptance dimension, and deliverable coverage container is satisfied; keep parent summaries consistent with child states.",
		"Treat evidenceCandidates as neutral candidates, not automatic proof; use them only when their concrete output supports the exact Goal requirement.",
		"When latestClosureEvidence is present, compare the latest successful evidence against older failed evidence in the same neutral scope; do not reject solely because an earlier superseded failure exists.",
		"If latestClosureEvidence is still insufficient, explain the exact missing dimension and cite the latest evidence that was considered.",
		"Each satisfied dodAssessment.goalCoverage item must cite concrete evidenceRefs from the candidate evidence or other available evidence.",
		"If evidence does not support a Goal or DoD item, mark it failed/skipped with reason, residualRisks, and nextActions instead of claiming satisfaction.",
	}
	if closure := latestCompletionClosureEvidence(ctx, steps, evidence, run, failedStep); len(closure) > 0 {
		input["latestClosureEvidence"] = closure
		if superseded := mapSliceFromValue(closure["supersededFailures"]); len(superseded) > 0 {
			input["supersededFailureEvidence"] = superseded
		}
	}
	if missingDeliverableCoverage := missingDeliverableCoverageFromStep(failedStep); len(missingDeliverableCoverage) > 0 {
		if open := filterCoverageGapsOpenByWorkItems(input, missingDeliverableCoverage); len(open) > 0 {
			input["missingDeliverableCoverage"] = open
		}
	}
	if requiredDeliverables := requiredProfileDeliverableExpectations(run); len(requiredDeliverables) > 0 {
		input["requiredDeliverableCoverage"] = requiredDeliverables
	}
}

func applyRuntimeRepairConcreteEvidenceProjection(input map[string]any, failedStep *runModel.Step, concreteEvidenceRepair bool) {
	if input == nil || failedStep == nil || !concreteEvidenceRepair {
		return
	}
	if doDEvidenceQualityGapRequiresStateChange(failedStep) {
		return
	}
	if inputRequiresStateChangingOutput(failedStep.Input) &&
		!stateChangingRepairHasActionableImplementationSurface(input, mapFromValue(input["repairActionPackage"])) &&
		!stateChangingRepairHasActionableImplementationSurface(failedStep.Input, mapFromValue(failedStep.Input["repairActionPackage"])) {
		input["repairObjective"] = repairObjectiveAcquireFailureEvidence
		input["concreteFailureEvidenceRequired"] = true
		input["evidenceAcquisitionReason"] = "state_changing_repair_target_evidence_missing"
		input["repairGuidance"] = concreteFailureEvidenceAcquisitionGuidance()
		delete(input, "stateChangingOutputRequired")
		delete(input, "evidenceRepairRequired")
		delete(input, "boundedValidationRequired")
		return
	}
	claimAlignedEvidenceNeeded := false
	if diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		claimAlignedEvidenceNeeded = boolValue(diagnostics["claimAlignedEvidenceNeeded"])
	}
	input["repairObjective"] = repairObjectiveConcreteValidationEvidence
	input["evidenceRepairRequired"] = true
	input["verificationEvidenceGap"] = map[string]any{
		"kind":                       "dod_verification_evidence_gap",
		"failedStepId":               string(failedStep.ID),
		"failedStepKind":             string(failedStep.Kind),
		"missingDoD":                 missingDoDItemsFromFailureOutput(failedStep.Output),
		"missingGoal":                missingGoalCoverageFromStep(failedStep),
		"missingDeliverable":         missingDeliverableCoverageFromStep(failedStep),
		"claimAlignedEvidenceNeeded": claimAlignedEvidenceNeeded,
	}
	input["repairGuidance"] = concreteValidationEvidenceRepairGuidance()
	delete(input, "stateChangingOutputRequired")
}

func applyRuntimeRepairCoverageNeedsConcreteEvidenceProjection(
	ctx context.Context,
	steps repository.StepRepository,
	evidence RuntimeEvidenceRepositories,
	input map[string]any,
	run *runModel.Run,
	failedStep *runModel.Step,
	coverageNeedsConcreteEvidence bool,
	assessmentOnlyFailedCoverageRepair bool,
) {
	if input == nil || failedStep == nil || !coverageNeedsConcreteEvidence {
		return
	}
	delete(input, stepInputKeyAssessmentOnly)
	delete(input, "assessmentOnlyReason")
	input["completionFollowUp"] = false
	input["evidenceRepairSource"] = "coverage_gap_without_candidate_evidence"
	if assessmentOnlyFailedCoverageRepair {
		input["evidenceRepairSource"] = "failed_assessment_only_coverage_gap"
	}
	if closure := latestCompletionClosureEvidence(ctx, steps, evidence, run, failedStep); len(closure) > 0 {
		input["latestClosureEvidence"] = closure
		if superseded := mapSliceFromValue(closure["supersededFailures"]); len(superseded) > 0 {
			input["supersededFailureEvidence"] = superseded
		}
	}
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"At least one Goal/DoD/deliverable coverage gap still lacks acceptable concrete evidence; produce bounded verification evidence before another dodAssessment correction.",
		"When latestClosureEvidence is present, prefer the latest successful evidence over older superseded failures in the same neutral scope.",
		"If new verification evidence closes a specific Goal requirement, ensure the next dodAssessment cites that exact evidenceRef and goalRequirementId.",
	)
}

func applyRuntimeRepairValidationEvidenceProjection(input map[string]any, coverageValidationEvidenceRepair bool) {
	if input == nil || !coverageValidationEvidenceRepair {
		return
	}
	input["boundedValidationRequired"] = true
	input["evidenceRepairSource"] = "coverage_gap_without_implementation_target"
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Unresolved Goal/DoD/deliverable coverage is currently a validation evidence gap because no concrete implementation target evidence is available.",
		"Produce bounded shell/tool/MCP validation evidence that maps to the coverageTargetSet before attempting another assessment.",
		"When an external dependency, credential, service, network endpoint, port, clock, filesystem path, or equivalent runtime prerequisite is unavailable, first try a neutral controlled substitute such as a temporary fixture, local stub/mock, alternate temporary config, isolated working directory, or available ephemeral port through the allowed validation tools; do not mark the item skipped or request a decision until such bounded substitute validation is impossible or would violate the Goal/DoD.",
		"Do not return patch/apply unless a later validation failure provides concrete target refs, line refs, diagnostics, or current target snapshots.",
	)
}

func applyRuntimeRepairStateChangingEvidenceQualityProjection(input map[string]any, failedStep *runModel.Step, required bool) {
	if input == nil || failedStep == nil || !required {
		return
	}
	input["stateChangingEvidenceRepairRequired"] = true
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["stateChangingOutputRequired"] = true
	input["evidenceRepairSource"] = "dod_evidence_quality_gap_requires_state_change"
	delete(input, stepInputKeyAssessmentOnly)
	delete(input, "assessmentOnlyReason")
	delete(input, "boundedValidationRequired")
	delete(input, "concreteFailureEvidenceRequired")
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"], stateChangingEvidenceQualityRepairGuidance()...)
}

func stateChangingEvidenceQualityRepairGuidance() []string {
	return []string{
		"The rejected DoD evidence cites read-only or otherwise insufficient evidence for a requirement whose profile requires state-changing evidence.",
		"Do not retry assessment-only or validation-only output for this gap; produce accepted state-changing evidence that matches the structured requiredEvidenceKinds, acceptedStepKinds, and acceptedOperations.",
		"After the state-changing evidence is produced, attach bounded validation or assessment evidence that cites the new evidenceRefs exactly.",
	}
}

func applyRuntimeRepairValidationEscalationProjection(input map[string]any, validationRepairEscalatesToStateChanging bool) {
	if input == nil || !validationRepairEscalatesToStateChanging {
		return
	}
	input["validationRepairEscalatedToImplementation"] = true
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["stateChangingOutputRequired"] = true
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"This coverage gap has already been repaired through validation-only attempts and now has concrete implementation target evidence.",
		"Switch to a state-changing implementation repair before another verification pass; do not continue the validation-only loop against the same target evidence.",
		"Produce the smallest state-changing artifact that makes the next bounded verification meaningful.",
	)
	delete(input, stepInputKeyAssessmentOnly)
	delete(input, "assessmentOnlyReason")
	delete(input, "boundedValidationRequired")
	delete(input, "evidenceRepairSource")
}

func applyRuntimeRepairEmptyWorkspaceStateChangingProjection(input map[string]any, required bool) {
	if input == nil || !required {
		return
	}
	input["emptyWorkspaceEvidenceEscalatedToImplementation"] = true
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["stateChangingOutputRequired"] = true
	delete(input, "concreteFailureEvidenceRequired")
	delete(input, "boundedValidationRequired")
	delete(input, "evidenceRepairRequired")
	delete(input, "evidenceRepairSource")
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Concrete evidence shows the workspace has no deliverable resources for the active Goal/DoD; stop collecting more empty-workspace evidence.",
		"Initialize the requested workspace deliverables through auditable state-changing output, such as patch/apply file_write nodes for the smallest cohesive resources required by Goal/DoD.",
	)
}

func applyRuntimeRepairGovernanceHistoryProjection(
	ctx context.Context,
	tickets repository.DecisionTicketRepository,
	resolutions repository.DecisionResolutionRepository,
	approvals repository.ApprovalRepository,
	input map[string]any,
	run *runModel.Run,
) {
	if input == nil || run == nil {
		return
	}
	governanceHistory := mapSliceFromValue(run.ExecutionContext["governanceHistory"])
	if current := currentRunGovernanceHistory(ctx, tickets, resolutions, approvals, run); len(current) > 0 {
		governanceHistory = append(current, governanceHistory...)
	}
	if len(governanceHistory) > 0 {
		input["governanceHistory"] = compactGovernanceHistoryForRuntimeContext(governanceHistory, 6)
	}
}

func applyRuntimeRepairGraphEvidenceProjection(
	ctx context.Context,
	graph *graphSubsystem,
	input map[string]any,
	run *runModel.Run,
	failedStep *runModel.Step,
) {
	if input == nil || graph == nil {
		return
	}
	if evidence := graph.completedGraphEvidence(ctx, run, failedStep); len(evidence) > 0 {
		input[stepInputKeyGraphEvidence] = evidence
		if snapshots := targetSnapshotsFromConcreteFailureEvidence(evidence); len(snapshots) > 0 {
			input["failureTargetRepairContext"] = mergeRepairTargetSnapshots(mapSliceFromValue(input["failureTargetRepairContext"]), snapshots)
		}
	}
}

func applyRuntimeRepairPatchContextProjection(
	input map[string]any,
	patchContext []map[string]any,
	patchSuggestions []map[string]any,
	satisfiedSignals []map[string]any,
	refreshableSignals []map[string]any,
) {
	if input == nil || len(patchContext) == 0 {
		return
	}
	input["patchRepairContext"] = patchContext
	if len(patchSuggestions) > 0 {
		input["patchRepairSuggestions"] = patchSuggestions
	}
	if len(satisfiedSignals) > 0 {
		input["currentTargetAlreadySatisfiesFailedPatch"] = satisfiedSignals
		input["stateChangeAlreadySatisfiedByCurrentTarget"] = true
	}
	if len(refreshableSignals) > 0 {
		input["currentTargetRefreshRequiredForFailedPatch"] = refreshableSignals
	}
	if len(mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"])) == 0 &&
		!boolValue(input["concreteFailureEvidenceRequired"]) &&
		stringFromMap(input, "repairObjective") != repairObjectiveConcreteValidationEvidence &&
		stringFromMap(input, "repairObjective") != repairObjectiveTransientExecutionRetry {
		input["repairObjective"] = repairObjectiveStateChangingOutput
		input["stateChangingOutputRequired"] = true
	}
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Use patchRepairContext as the current target snapshot for patch repair; treat the failed patch text as stale or untrusted.",
		"When the failure is a diff context mismatch or file/edit oldText conflict, do not retry the same stale hunk or oldText; rebuild the smallest patch from patchRepairContext.content/contentHash.",
		"When using baseHash, copy the literal contentHash value from the current target snapshot; never output unresolved references such as ${...}, {{...}}, or symbolic placeholders.",
		"Use complete patch/apply file_write only when the current snapshot is marked safeForFileWrite/contentComplete and contains no redaction markers; otherwise use patch/apply text_replace with exact oldText/newText/baseHash, a minimal unified_diff with exact visible context, file/edit with exact oldText, or first read the target.",
		"Never copy runtime redaction marker tokens, omitted sections, or truncated snapshot text into patch/apply content or diff.",
		"If a previous patch was rejected for redacted runtime markers, discard that diff entirely and rebuild from current unredacted file/read or patchRepairContext content; do not transform redacted marker text into source content.",
		"If patchRepairSuggestions are present, prefer patch/apply text_replace with exact oldText/newText/baseHash, or use that evidence to construct the smallest current unified_diff/file/edit repair.",
		"If patch application failed because of malformed unified_diff syntax, return patch/apply text_replace with exact oldText/newText/baseHash or a complete standard unified diff with explicit line ranges such as @@ -12,3 +12,4 @@.",
		"Before adding new target content, reconcile the current snapshot and modify an existing equivalent item instead of creating duplicate named artifacts.",
		"Any MCP tool call that synchronizes modified file content must include the complete current file content in input.content together with the target path; do not emit path-only, root-only, or partial sync requests.",
		"When patching resources that reference existing named elements, patch against names that already exist in current workspace resources unless the Goal explicitly requires introducing a new named element.",
		"If the current target snapshot proves the failed patch is no longer required, return a bounded verification evidence graph instead of another patch; do not use read-only inspection as the only evidence.",
	)
	if len(mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"])) > 0 {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"currentTargetAlreadySatisfiesFailedPatch is a mechanical signal that the failed artifact's intended line changes are already reflected in the current target snapshot.",
			"Do not repeat the same failed patch when this signal is present; return a bounded concrete validation evidence graph, or a failed dodAssessment/blocker if validation cannot be produced.",
		)
	}
	if len(mapSliceFromValue(input["currentTargetRefreshRequiredForFailedPatch"])) > 0 {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"currentTargetRefreshRequiredForFailedPatch is a mechanical signal that the failed patch was built against a stale target hash.",
			"Do not retry the same failed baseHash or patch body; rebuild from patchRepairContext and use the current target contentHash.",
		)
	}
}

func applyRuntimeRepairPatchFailureRecoveryContract(input map[string]any, pkg map[string]any, failedStep *runModel.Step) {
	if input == nil || len(pkg) == 0 || failedStep == nil {
		return
	}
	if !shouldIncludePatchRepairContext(failedStep) {
		return
	}
	patchContext := mapSliceFromValue(input["patchRepairContext"])
	if len(patchContext) == 0 {
		patchContext = mapSliceFromValue(pkg["patchRepairContext"])
	}
	if len(patchContext) == 0 {
		return
	}
	failedRequests := patchFailureRecoveryRequests(failedStep)
	if len(failedRequests) == 0 {
		return
	}
	required := mapFromValue(pkg["requiredOutput"])
	if len(required) == 0 {
		required = stateChangingRepairRequiredOutput()
	}
	applyCurrentSnapshotDirectPatchContract(required)
	required["mustRepairFailedPatchApplication"] = true
	required["mustUsePatchRepairContext"] = true
	required["mustNotRetryFailedPatchLiteral"] = true
	required["patchFailureRecoveryRule"] = "previous patch application failed after dispatch; rebuild a fresh patch from patchRepairContext/current target snapshots, use the current snapshot contentHash as baseHash, and do not retry stale oldText/diff/content from failedPatchRequests"
	required["patchFailureFallbackRule"] = "if current patchRepairContext is insufficient for a safe state change, return a real governance blocker or concrete evidence acquisition request; do not output another stale patch"
	required["acceptedPatchRecoveryModes"] = []string{
		"text_replace_with_exact_current_oldText_and_baseHash",
		"unified_diff_with_current_visible_context_and_line_ranges",
		"file_write_with_matching_current_baseHash_when_safeForFileWrite",
	}
	required["forbiddenOutputs"] = compactUniqueStrings(append(stringSliceFromValue(required["forbiddenOutputs"]),
		"retry_same_failed_patch_literal",
		"oldText_not_present_in_current_snapshot",
		"diff_context_not_derived_from_current_snapshot",
		"symbolic_or_stale_baseHash",
	))
	required["failedPatchRequests"] = failedRequests
	if suggestions := mapSliceFromValue(input["patchRepairSuggestions"]); len(suggestions) > 0 {
		required["preferredPatchRepairSuggestions"] = limitedMapSliceForRepairContract(suggestions, 5)
	}
	pkg["requiredOutput"] = required
	pkg["patchFailureRecoveryContract"] = map[string]any{
		"failedPatchRequests":          failedRequests,
		"patchRepairContextRefs":       patchRepairContextRefs(patchContext),
		"requiresCurrentSnapshotPatch": true,
		"businessJudgment":             false,
	}
	input["patchFailureRecoveryContract"] = pkg["patchFailureRecoveryContract"]
	input["repairActionPackage"] = pkg
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"The previous patch reached runtime patch application and failed against the current workspace content.",
		"Treat failedPatchRequests as rejected stale material. Do not retry the same oldText, diff, content, or symbolic baseHash.",
		"Build the next patch from patchRepairContext/current target snapshots and copy the current contentHash as baseHash when baseHash is used.",
	)
}

func patchFailureRecoveryRequests(failedStep *runModel.Step) []map[string]any {
	if failedStep == nil {
		return nil
	}
	requests := patchRepairRequestsFromStep(failedStep)
	if len(requests) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, min(len(requests), 5))
	for _, request := range requests {
		targetRef := normalizeRepairPatchTargetRef(request.TargetRef)
		if targetRef == "" {
			continue
		}
		item := map[string]any{
			"targetRef": targetRef,
			"patchType": request.PatchType,
		}
		if request.BaseHash != "" {
			item["baseHash"] = request.BaseHash
		}
		if request.OldText != "" {
			item["oldTextHash"] = sha256HexBytes([]byte(request.OldText))
			item["oldTextPreview"] = truncateLLMContextString(request.OldText)
		}
		if request.NewText != "" {
			item["newTextHash"] = sha256HexBytes([]byte(request.NewText))
			item["newTextPreview"] = truncateLLMContextString(request.NewText)
		}
		if request.Diff != "" {
			item["diffHash"] = sha256HexBytes([]byte(request.Diff))
			item["diffPreview"] = truncateLLMContextString(request.Diff)
		}
		if request.Content != "" {
			item["contentHash"] = sha256HexBytes([]byte(request.Content))
			item["contentPreview"] = truncateLLMContextString(request.Content)
		}
		if reason := firstNonEmptyString(
			stringFromMap(failedStep.Output, "error"),
			stringFromMap(failedStep.Output, "reason"),
			errorStringFromOutput(failedStep.Output),
		); reason != "" {
			item["failureReason"] = truncateLLMContextString(reason)
		}
		ret = append(ret, item)
		if len(ret) >= 5 {
			break
		}
	}
	return ret
}

func patchRepairContextRefs(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, min(len(items), 5))
	for _, item := range items {
		targetRef := normalizeRepairPatchTargetRef(stringFromMap(item, "targetRef"))
		if targetRef == "" {
			continue
		}
		ref := map[string]any{
			"targetRef": targetRef,
		}
		for _, key := range []string{"contentHash", "baseHash"} {
			if value := strings.TrimSpace(stringFromMap(item, key)); value != "" {
				ref[key] = value
			}
		}
		for _, key := range []string{"contentComplete", "safeForFileWrite", "redacted", "truncated", "exists"} {
			if value, ok := item[key]; ok {
				ref[key] = value
			}
		}
		ret = append(ret, ref)
		if len(ret) >= 5 {
			break
		}
	}
	return ret
}

func limitedMapSliceForRepairContract(items []map[string]any, limit int) []map[string]any {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, min(len(items), limit))
	for _, item := range items {
		if len(item) == 0 {
			continue
		}
		ret = append(ret, compactLLMContextMap(item, 1))
		if len(ret) >= limit {
			break
		}
	}
	return ret
}

func errorStringFromOutput(output map[string]any) string {
	if len(output) == 0 {
		return ""
	}
	return firstNonEmptyString(stringFromMap(output, "error"), stringFromMap(output, "reason"))
}

func applyRuntimeRepairFailureTargetContextProjection(input map[string]any, targetContext []map[string]any) {
	if input == nil || len(targetContext) == 0 {
		return
	}
	input["failureTargetRepairContext"] = targetContext
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Use failureTargetRepairContext as the current snapshot for targets identified by the failed evidence.",
		"When failed evidence points at a concrete target resource, generate the smallest auditable patch/apply state change against that current snapshot before rerunning verification.",
		"Use workspaceFileManifest to reconcile target resources with other known workspace resources; avoid replacing unrelated existing artifacts just to satisfy one failure target.",
		"When a failure mentions missing names, invalid attributes, or contract-entry mismatches, do not invent new names in the target patch; first reconcile the target with currently visible resource shape from relevant existing resources, or read the relevant resource before patching.",
		"When the failure text contains line-referenced unresolved names, invalid attributes, shape mismatches, or equivalent resource-shape diagnostics in the failure target, repair those reported blockers before secondary validation, formatting, or evidence refinements.",
	)
}

func applyRuntimeRepairResourceShapeProjection(input map[string]any, shapeContext []map[string]any) {
	if input == nil || len(shapeContext) == 0 {
		return
	}
	input["resourceShapeAlignmentContext"] = shapeContext
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Use resourceShapeAlignmentContext as current same-directory resource evidence when diagnostics report unresolved names, invalid attributes, shape mismatches, contract-entry mismatches, or equivalent resource-shape conflicts.",
		"Before introducing new named elements or attributes, reconcile the failed target with existing current workspace resources in resourceShapeAlignmentContext.",
		"If the failed target disagrees with current resource shape, prefer the smallest coherent state change that aligns the target and existing resources; do not invent replacement contracts from failure text alone.",
	)
}

func applyRuntimeRepairRootCauseCandidateProjection(input map[string]any, rootCauseCandidates []map[string]any) {
	if input == nil || len(rootCauseCandidates) == 0 {
		return
	}
	input["rootCauseCandidateContext"] = rootCauseCandidates
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Use rootCauseCandidateContext to evaluate grounded adjacent or referenced resources when the failed surface may be unblocked by changing a related current workspace resource.",
		"Do not assume the failed surface itself is the root cause when current workspace resources show adjacent or referenced resources that the failed surface depends on.",
		"If choosing a rootCauseTarget that differs from the failed surface, patch that resource directly and declare relatedFailureTargets for the failed surface being unblocked.",
	)
}

func applyRuntimeRepairReadableResourceProjection(input map[string]any, readableResourceContext []map[string]any) {
	if input == nil || len(readableResourceContext) == 0 {
		return
	}
	input["readableResourceRepairContext"] = readableResourceContext
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"The previous decision requested information that is already available as a readable workspace resource.",
		"Use readableResourceRepairContext as current evidence; do not ask the operator to paste the same resource content.",
	)
}

func applyRuntimeRepairWorkspaceManifestProjection(input map[string]any, manifest []map[string]any) {
	if input == nil || len(manifest) == 0 {
		return
	}
	input[stepInputKeyWorkspaceFileManifest] = manifest
}

func applyRuntimeRepairStateChangingActionPackageProjection(
	input map[string]any,
	failedStep *runModel.Step,
	execErr error,
) {
	if input == nil || failedStep == nil || !boolValue(input["stateChangingOutputRequired"]) {
		return
	}
	if _, ok := input["lastFailureDiagnostics"]; !ok {
		if fallback := runtimeRepairLastFailureDiagnostics(failedStep); len(fallback) > 0 {
			input["lastFailureDiagnostics"] = fallback
		}
	}
	narrowStateChangingRecoveryInput(input, failedStep, execErr, "runtime_repair")
	if pkg := repairActionPackageFromRepairInput(input, failedStep.Output); len(pkg) > 0 {
		if repairActionPackageRequiresEvidenceAcquisition(pkg) {
			delete(input, "stateChangingOutputRequired")
			input["repairObjective"] = repairObjectiveAcquireFailureEvidence
			input["concreteFailureEvidenceRequired"] = true
			input["evidenceAcquisitionReason"] = stringFromMap(pkg, "evidenceAcquisitionReason")
			input["repairGuidance"] = concreteFailureEvidenceAcquisitionGuidance()
		}
		applyRuntimeRepairPatchFailureRecoveryContract(input, pkg, failedStep)
		input["repairActionPackage"] = pkg
		ensureAvailableTargetSnapshotsForStateChangingRepair(input)
		compactStateChangingRepairInputAroundActionPackage(input)
	}
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Use repairActionPackage as the primary actionable repair contract; it contains the required output kind, rejected output summary, and current target snapshots.",
	)
	if len(mapSliceFromValue(input["failureTargetRepairContext"])) > 0 ||
		len(mapSliceFromValue(mapFromValue(input["repairActionPackage"])["targetSnapshots"])) > 0 ||
		len(stringSliceFromValue(mapFromValue(input["repairActionPackage"])["repairTargetSet"])) > 0 {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"When current failure targets, target snapshots, or repairTargetSet already identify concrete repair resources, prioritize direct state-changing edits to those targets before modifying auxiliary validation scaffolding, supplemental evidence resources, or unrelated supporting files.",
			"If the current diagnostics do not explicitly identify an auxiliary file as the failed surface, treat that auxiliary file as supporting context only, not the primary state-changing target.",
		)
	}
	if !boolValue(input["concreteFailureEvidenceRequired"]) {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"When repairActionPackage.requiredOutput.mustIncludeStateChangingNode is true, the next output must include an accepted state-changing operation before verification, or return a real governance blocker.",
		)
	}
	if stateChangingSchemaInvalidRepair(failedStep) {
		input["schemaInvalidRepairMode"] = "single_valid_json_decision"
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Previous repair output was rejected before execution because the LLM decision JSON was invalid; rebuild a single valid JSON object from repairActionPackage and current target snapshots.",
			"Do not copy any rawContentPreview or repairRawContentPreview from the rejected output; do not include markdown, comments, duplicate JSON objects, trailing braces, or partial arrays.",
			"If returning executionGraph, every node must be a complete JSON object and must follow repairActionPackage.requiredOutput acceptedKinds.",
		)
	}
}

func applyRuntimeRepairRepeatedReadOnlyStateChangingProjection(input map[string]any, failedStep *runModel.Step, attempt int) {
	if input == nil || failedStep == nil || attempt < 2 {
		return
	}
	if !runtimeRepairStepIsRepeatedStateChangingContractLoop(failedStep) {
		return
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 || !repairHasImplementationTargetEvidence(input, pkg) {
		return
	}
	required := mapFromValue(pkg["requiredOutput"])
	if len(required) == 0 {
		required = stateChangingRepairRequiredOutput()
	}
	required["mustUseCurrentTargetSnapshot"] = true
	required["mustPatchDirectlyFromProvidedSnapshot"] = true
	required["mustRebuildFromCurrentEvidence"] = true
	required["mustReturnPatchApplyNode"] = true
	required["patchApplyOnlyReason"] = "repeated_state_changing_repair_returned_read_only_graph_with_concrete_target_evidence"
	if len(stringSliceFromValue(required["acceptedOperations"])) == 0 {
		required["acceptedKinds"] = []string{"patch/apply"}
		delete(required, "toolStateChangingInputContract")
		delete(required, "toolStateChangingRule")
	}
	required["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(required["forbiddenKinds"]),
		"read_only_graph",
		"inspection_only_graph",
		"shell_only_graph",
	))
	pkg["requiredOutput"] = required
	input["repairActionPackage"] = pkg
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["stateChangingOutputRequired"] = true
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"The previous state-changing repair already failed because it returned a read-only or inspection-only graph.",
		"Current target snapshots and repair target evidence are already available. Return a concrete patch/apply node that edits one of those targets directly; do not spend another turn on file/read, git/read, or inspection-only graph planning.",
		"If verification is still needed, make it depend on the patch/apply node and keep it bounded to the same failure target or declared related failure target.",
	)
}

func applyRuntimeRepairFailureScopeProjection(input map[string]any, class string, signature string) {
	if input == nil {
		return
	}
	if class != "" {
		input[failureOutputKeyClass] = class
		input[stepInputKeyRepairOfFailureClass] = class
	}
	if signature != "" {
		input[failureOutputKeySignature] = signature
		input[stepInputKeyRepairOfFailureSig] = signature
	}
}

func applyRuntimeRepairStructuredFailureSignalsProjection(
	input map[string]any,
	signals map[string]any,
) {
	if input == nil || len(signals) == 0 {
		return
	}
	input["structuredFailureSignals"] = signals
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Use structuredFailureSignals as mechanical failure evidence only; paths, line refs, and identifiers must be reconciled with current target snapshots or fresh reads before patching.",
		"Do not treat identifier-like tokens in failure text as proof that a named element should be invented; ground every referenced name in current resources or introduce it explicitly in the same patch.",
	)
	if structuredFailureSignalsContainShapeConflict(signals) {
		if primary := primaryDiagnosticRepairContractFromSignals(signals); len(primary) > 0 {
			input["primaryDiagnosticRepair"] = primary
		}
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Prioritize the primary diagnostics from structuredFailureSignals before secondary cleanup: fix reported unresolved names, invalid attributes, shape mismatches, contract-entry mismatches, and line-referenced target errors first.",
			"When diagnostics identify a concrete target snapshot, reconcile that target with current visible resource or contract shape before changing secondary validation details, evidence wording, or unrelated quality improvements.",
		)
	}
}

func applyRuntimeRepairFailureTargetScopeProjection(
	input map[string]any,
	targetScope string,
	targetRepairAttempts int,
) {
	if input == nil || targetScope == "" {
		return
	}
	input["failureTargetScope"] = targetScope
	input["targetRepairAttempts"] = targetRepairAttempts
	if boolValue(input["stateChangingOutputRequired"]) {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"When failureTargetScope is present, verificationPlan must stay bounded to that scope and explain the exact targeted checks.",
			"Do not run unbounded verification for scoped repairs unless an explicit target scope or filter is provided.",
		)
	}
}

func applyRuntimeRepairTransientRetryProjection(input map[string]any, transientExecutionTimeout bool) {
	if input == nil || !transientExecutionTimeout {
		return
	}
	input["repairObjective"] = repairObjectiveTransientExecutionRetry
	input["executionTimeoutRetryAllowed"] = true
	input["repairGuidance"] = transientExecutionRetryGuidance()
}

func applyRuntimeRepairCapabilityPolicyProjection(input map[string]any, contract map[string]any) {
	if input == nil || len(contract) == 0 {
		return
	}
	input["capabilityPolicyRepairRequired"] = true
	input["repairObjective"] = "repair_capability_policy_follow_up"
	input["stateChangingOutputRequired"] = true
	input["capabilityPolicyRepairContract"] = contract
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"The previous execution graph violated a declared capability usage policy after a trigger node.",
		"Repair the graph contract directly: for each triggered policy, add a dependent mcp/tool_call that matches the policy preferredTools, or declare a structured capabilityPolicySkips item with policyId/category and a non-empty reason.",
		"Do not return assessment-only output for this repair; the failure is a mechanical execution-graph contract violation that can continue through a corrected graph or structured skip.",
	)
	delete(input, stepInputKeyAssessmentOnly)
	delete(input, "assessmentOnlyReason")
}

func applyRuntimeRepairStateChangingCoverageTargetGuidance(input map[string]any) {
	if input == nil || !boolValue(input["stateChangingOutputRequired"]) {
		return
	}
	if len(mapSliceFromValue(input["missingDoDItems"])) == 0 && len(missingGoalCoverageFromInput(input)) == 0 {
		return
	}
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"When missingDoDItems or missingGoalCoverage are present, at least one patch/apply node must declare coverageTargets and each entry must match the exact missing DoD item text or goalRequirementId/item it is intended to close.",
		"Do not return a generic skeleton patch. If a state-changing artifact cannot be tied to a concrete missing coverage target, return a real governance blocker instead.",
	)
}

func applyRuntimeRepairUnresolvedCoverageWorkItemGuidance(input map[string]any) {
	if input == nil || len(mapSliceFromValue(input["unresolvedCoverageWorkItems"])) == 0 {
		return
	}
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Resolve unresolvedCoverageWorkItems before returning an accepted result; each item requires its missing evidence kinds or a failed/skipped assessment with reason.",
		"For assessment_pending items, return a bounded dodAssessment that explicitly maps the item by id, sourceRequirementId, sourcePath, or targetText.",
		"For validation items, produce bounded validation evidence or mark the item failed/skipped; do not claim accepted completion while it remains unresolved.",
		"For state_change items, produce the smallest state-changing action that matches the coverage work item, then validate it.",
	)
}

func applyRuntimeRepairStrategyReassessmentProjection(input map[string]any, reassessment map[string]any) {
	if input == nil || len(reassessment) == 0 {
		return
	}
	input["strategyReassessment"] = reassessment
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"strategyReassessment.required is true: explicitly choose repairMode as continue_same_target_repair or shift_root_cause_target, or return a real governance blocker.",
		"Explain the next repair target decision in neutral terms: failedSurface, rootCauseTarget, repairTargetDecision, repairMode, forbiddenRepair, and verificationPlan.",
		"When continuing the same failed target, patch only if the current target snapshot and primary diagnostics show a concrete same-target correction; otherwise choose a grounded root-cause target or return a blocker.",
		"When shifting root-cause target, patch a neutral grounded root-cause resource and declare relatedFailureTargets for the failed surface.",
		"Do not weaken still-valid Goal or DoD constraints, remove valid evidence, or reduce the requested outcome to make verification pass.",
	)
}

func applyRuntimeRepairPreviousGraphMetadataProjection(
	input map[string]any,
	failedStep *runModel.Step,
	previousStepOutput map[string]any,
	failedExecutionGraph runtimeModel.BlockExecutionGraph,
	hasFailedExecutionGraph bool,
) {
	if input == nil || failedStep == nil {
		return
	}
	input["previousStepKind"] = failedStep.Kind
	input["previousStepOutput"] = previousStepOutput
	if nodeID := graphNodeID(failedStep); nodeID != "" {
		input[stepInputKeyGraphPreviousNodeID] = nodeID
		input["failedGraphNodeId"] = nodeID
	}
	if completed := graphCompletedNodesFromStep(failedStep); len(completed) > 0 {
		input[stepInputKeyGraphCompletedNodes] = completed
	}
	if branch := graphBranch(failedStep); branch != "" {
		input[stepOutputKeyGraphBranch] = branch
	}
	if hasFailedExecutionGraph {
		input["failedExecutionGraph"] = failedExecutionGraph
		input[stepInputKeyGraphRootStepID] = string(graphRootStepID(failedStep))
	}
}

func applyRuntimeRepairValidationEscalationFollowUpGuidance(input map[string]any) {
	if input == nil || !boolValue(input["validationRepairEscalatedToImplementation"]) {
		return
	}
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Repeated validation-only repair is no longer an acceptable next action for this coverage gap.",
		"If concrete target evidence is still missing, acquire bounded implementation target evidence first; otherwise produce a state-changing repair before the next validation.",
	)
}

func runtimeEmbeddedGraphLineageLabel(runID common.RunID, stepID common.StepID) common.StepID {
	raw := strings.TrimSpace(string(stepID))
	if raw == "" {
		return ""
	}
	if runID != "" {
		raw = strings.TrimPrefix(raw, strings.TrimSpace(string(runID))+"/")
	}
	raw = strings.Trim(raw, "/")
	if strings.HasPrefix(raw, "graph-") {
		return common.StepID(raw)
	}
	if idx := strings.Index(raw, "-of-graph-"); idx >= 0 {
		return common.StepID(strings.TrimSpace(raw[idx+len("-of-"):]))
	}
	return ""
}

func (o *RuntimeOrchestrator) runtimeRepairFollowUpStepInput(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	attempt int,
	execErr error,
) map[string]any {
	input := runtimeRepairBaseInput(runtimeRepairBaseInputDepsFor(o), run, failedStep, attempt, execErr)
	if failedStep == nil {
		return input
	}
	rootStepID := runtimeRepairRootStepID(failedStep)
	if runtimeRepairShouldReturnToRejectedResultContinuation(run, failedStep) {
		input = resetRuntimeRepairInputToRejectedResultContinuation(input, run, failedStep)
		contract := o.convergenceManager().build(run, nil, failedStep, map[string]any{"input": input}, nil)
		input = o.convergenceManager().apply(input, contract)
		return finalizeRuntimeRepairFollowUpStepInput(run, failedStep, input)
	}
	applyRuntimeRepairFailureProjection(input, failedStep)
	assessmentOnlyCoverageRepair := doDCoverageContractCorrectionRequired(failedStep)
	applyRuntimeRepairMissingCoverageProjection(ctx, o.steps, input, run, failedStep, assessmentOnlyCoverageRepair)
	patchContext := o.repairManager().runtimeRepairPatchContext(ctx, run, failedStep)
	if len(patchContext) > 0 {
		applyRuntimeRepairPatchContextProjection(
			input,
			patchContext,
			o.repairManager().runtimeRepairPatchSuggestions(ctx, run, failedStep),
			o.repairManager().runtimeRepairPatchAlreadySatisfiedSignals(ctx, run, failedStep),
			o.repairManager().runtimeRepairPatchRefreshableSignals(ctx, run, failedStep),
		)
	}
	targetContext := o.repairManager().runtimeRepairFailureTargetContext(ctx, run, failedStep)
	if len(targetContext) > 0 {
		applyRuntimeRepairFailureTargetContextProjection(input, targetContext)
	}
	if shapeContext := o.repairManager().runtimeRepairResourceShapeAlignmentContext(ctx, run, failedStep); len(shapeContext) > 0 {
		applyRuntimeRepairResourceShapeProjection(input, shapeContext)
	}
	rootCauseCandidates := o.repairManager().runtimeRepairRootCauseCandidateContext(ctx, run, failedStep)
	if len(rootCauseCandidates) > 0 {
		applyRuntimeRepairRootCauseCandidateProjection(input, rootCauseCandidates)
	}
	if context := convertedDecisionReadableResourceContext(ctx, o.workspaces, run, failedStep); len(context) > 0 {
		applyRuntimeRepairReadableResourceProjection(input, context)
	}
	assessmentOnlyFailedCoverageRepair := assessmentOnlyCoverageCorrectionFailedWithActionableGaps(failedStep)
	coverageNeedsConcreteEvidence := assessmentOnlyCoverageRepair && (coverageCorrectionNeedsConcreteEvidence(&runModel.Step{
		Input:  input,
		Output: failedStep.Output,
	}) || assessmentOnlyFailedCoverageRepair)
	coverageGapEligibleForValidationRepair := !assessmentOnlyCoverageRepair &&
		coverageValidationEvidenceRepairEligible(failedStep) &&
		coverageVerificationGapPresent(input, failedStep)
	coverageValidationHasTargetEvidence := runtimeRepairHasConcreteImplementationTargetEvidence(input, failedStep)
	coverageValidationEvidenceRepair := coverageGapEligibleForValidationRepair && !coverageValidationHasTargetEvidence
	emptyWorkspaceStateChangingRepair := emptyWorkspaceEvidenceShouldStartStateChangingRepair(run, failedStep, input)
	stateChangingEvidenceQualityRepair := doDEvidenceQualityGapRequiresStateChange(failedStep)
	validationRepairEscalatesToStateChanging := emptyWorkspaceStateChangingRepair || (coverageGapEligibleForValidationRepair &&
		coverageValidationHasTargetEvidence &&
		coverageValidationRepairShouldEscalateToStateChanging(ctx, o.repairManager(), run, failedStep, input))
	applyRuntimeRepairValidationEscalationProjection(input, validationRepairEscalatesToStateChanging)
	applyRuntimeRepairEmptyWorkspaceStateChangingProjection(input, emptyWorkspaceStateChangingRepair)
	applyRuntimeRepairStateChangingEvidenceQualityProjection(input, failedStep, stateChangingEvidenceQualityRepair)
	coverageValidationEvidenceRepair = coverageValidationEvidenceRepair && !validationRepairEscalatesToStateChanging && !stateChangingEvidenceQualityRepair
	concreteEvidenceRepair := coverageNeedsConcreteEvidence || coverageValidationEvidenceRepair || (!assessmentOnlyCoverageRepair && !stateChangingEvidenceQualityRepair && doDVerificationEvidenceRepairRequired(failedStep))
	applyRuntimeRepairConcreteEvidenceProjection(input, failedStep, concreteEvidenceRepair)
	applyRuntimeRepairAssessmentOnlyCoverageProjection(ctx, o.steps, o.evidence, input, run, failedStep, assessmentOnlyCoverageRepair, coverageNeedsConcreteEvidence, validationRepairEscalatesToStateChanging)
	applyRuntimeRepairCoverageNeedsConcreteEvidenceProjection(ctx, o.steps, o.evidence, input, run, failedStep, coverageNeedsConcreteEvidence, assessmentOnlyFailedCoverageRepair)
	applyRuntimeRepairValidationEvidenceProjection(input, coverageValidationEvidenceRepair)
	ensureCoverageRepairTargetSet(input)
	applyRuntimeRepairTransientRetryProjection(input, runtimeRepairFailureIsTransientExecutionTimeout(failedStep, execErr))
	if !assessmentOnlyCoverageRepair && !concreteEvidenceRepair && stateChangingRepairRequired(run, failedStep, execErr) {
		input["repairObjective"] = repairObjectiveStateChangingOutput
		input["stateChangingOutputRequired"] = true
		guidance := appendRepairGuidance(nil, stateChangingRepairGuidance()...)
		input["repairGuidance"] = appendRepairGuidance(guidance, stringSliceFromValue(input["repairGuidance"])...)
	}
	applyRuntimeRepairCapabilityPolicyProjection(input, capabilityPolicyRepairContractFromFailure(run, failedStep, execErr))
	applyRuntimeRepairStateChangingCoverageTargetGuidance(input)
	applyRuntimeRepairUnresolvedCoverageWorkItemGuidance(input)
	class, signature := runtimeRepairScopeClassAndSignature(failedStep)
	applyRuntimeRepairFailureScopeProjection(input, class, signature)
	policy := o.RuntimePolicy()
	applyRuntimeRepairStructuredFailureSignalsProjection(input, structuredFailureSignalsFromFailureOutput(failedStep.Output, execErr, class, signature))
	targetScope := runtimeFailureTargetScope(signature)
	if targetScope != "" {
		applyRuntimeRepairFailureTargetScopeProjection(
			input,
			targetScope,
			o.repairManager().runtimeRepairAttemptCountForFailureTargetInWindow(ctx, run.ID, class, targetScope, runtimeRepairTargetBudgetWindowRootForFailureTarget(failedStep)),
		)
	}
	applyRuntimeRepairStrategyReassessmentProjection(input, repairStrategyReassessmentFromInput(input))
	budgetScope := recoveryBudgetScope("runtime_repair", rootStepID, class, signature)
	if targetScope != "" {
		budgetScope["failureTargetScope"] = targetScope
	}
	input[stepInputKeyRecoveryBudgetScope] = budgetScope
	graph, hasGraph := runtimeGraphSpecFromStep(failedStep)
	failedExecutionGraph := runtimeModel.BlockExecutionGraph{}
	if hasGraph {
		failedExecutionGraph = o.llmContextManager().compactExecutionGraph(graph)
	}
	applyRuntimeRepairPreviousGraphMetadataProjection(input, failedStep, o.llmContextManager().compactStepOutput(failedStep.Output), failedExecutionGraph, hasGraph)
	applyRuntimeRepairGovernanceHistoryProjection(ctx, o.tickets, o.resolutions, o.approvals, input, run)
	applyRuntimeRepairGraphEvidenceProjection(ctx, o.graphManager(), input, run, failedStep)
	if progress := runtimeRepairCurrentExecutionProgress(ctx, o.steps, o.repairManager(), run, failedStep, input); len(progress) > 0 {
		input["currentExecutionProgress"] = progress
	}
	if shouldIncludeWorkspaceManifestInRuntimeRepair(failedStep) || len(targetContext) > 0 || len(patchContext) > 0 || len(rootCauseCandidates) > 0 {
		manifest := o.repairManager().workspaceFileManifest(ctx, run)
		applyRuntimeRepairWorkspaceManifestProjection(input, manifest)
	}
	inheritPriorRepairContext(input, failedStep)
	preserveStateChangingSchemaInvalidRepairContract(input, failedStep)
	applyRuntimeRepairStateChangingActionPackageProjection(input, failedStep, execErr)
	applyRuntimeRepairRepeatedReadOnlyStateChangingProjection(input, failedStep, attempt)
	applyRuntimeRepairValidationEscalationFollowUpGuidance(input)
	assessmentOnlySchemaInvalidRepair := preserveAssessmentOnlySchemaInvalidRepairContract(input, failedStep)
	if !assessmentOnlySchemaInvalidRepair {
		preserveConcreteValidationSchemaInvalidRepairContract(input, failedStep)
	}
	applyRuntimeRepairStateChangingTargetEvidenceGate(input, failedStep)
	applyContractViolationRecoveryProjection(
		input,
		failedStep,
		attempt,
		NormalizeRuntimePolicy(policy).MaxContractViolationRetriesPerStep,
		contractViolationSignatureRepeatCount(ctx, o.repairManager(), run.ID, failedStep),
	)
	applyLLMOutputModeSelection(ctx, policy.LLMOutput, o.evidence.ArtifactContinuations, run, input, failedStep, attempt)
	contract := o.convergenceManager().build(run, nil, failedStep, map[string]any{"input": input}, nil)
	input = o.convergenceManager().apply(input, contract)
	ensureAvailableTargetSnapshotsForStateChangingRepair(input)
	applyLLMOutputModeSelection(ctx, policy.LLMOutput, o.evidence.ArtifactContinuations, run, input, failedStep, attempt)
	refreshRuntimeRepairOutputDecisionContract(input, failedStep)
	return finalizeRuntimeRepairFollowUpStepInput(run, failedStep, input)
}

func finalizeRuntimeRepairFollowUpStepInput(run *runModel.Run, failedStep *runModel.Step, input map[string]any) map[string]any {
	stepID := common.StepID("")
	if failedStep != nil {
		stepID = failedStep.ID
	}
	return attachStagePacketToStepInput(run, stepID, input)
}

func refreshRuntimeRepairOutputDecisionContract(input map[string]any, failedStep *runModel.Step) {
	if input == nil {
		return
	}
	ensureAvailableTargetSnapshotsForStateChangingRepair(input)
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		return
	}
	required := mapFromValue(pkg["requiredOutput"])
	if len(required) == 0 {
		return
	}
	reapplyRepairCoverageFocusContract(input, pkg)
	required = mapFromValue(pkg["requiredOutput"])
	if len(required) == 0 {
		return
	}
	var output map[string]any
	if failedStep != nil {
		output = failedStep.Output
	}
	if contract := repairActionOutputDecisionContract(input, pkg, required, output); len(contract) > 0 {
		pkg["outputDecisionContract"] = contract
		input["repairActionPackage"] = pkg
	}
}

func applyRuntimeRepairStateChangingTargetEvidenceGate(input map[string]any, failedStep *runModel.Step) {
	if input == nil || failedStep == nil || !inputRequiresStateChangingOutput(failedStep.Input) {
		return
	}
	if stateChangingRepairHasActionableImplementationSurface(input, mapFromValue(input["repairActionPackage"])) ||
		stateChangingRepairHasActionableImplementationSurface(failedStep.Input, mapFromValue(failedStep.Input["repairActionPackage"])) {
		return
	}
	input["repairObjective"] = repairObjectiveAcquireFailureEvidence
	input["concreteFailureEvidenceRequired"] = true
	input["evidenceAcquisitionReason"] = "state_changing_repair_target_evidence_missing"
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"], concreteFailureEvidenceAcquisitionGuidance()...)
	delete(input, "stateChangingOutputRequired")
	delete(input, "evidenceRepairRequired")
	delete(input, "boundedValidationRequired")
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	required := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, mapFromValue(pkg["requiredOutput"]))
	pkg["requiredOutput"] = required
	pkg["evidenceAcquisitionRequired"] = true
	pkg["evidenceAcquisitionReason"] = "state_changing_repair_target_evidence_missing"
	pkg["repairObjective"] = repairObjectiveAcquireFailureEvidence
	input["repairActionPackage"] = pkg
}
