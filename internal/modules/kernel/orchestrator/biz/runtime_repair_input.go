package orchestrator

import (
	"context"
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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
	previousStepID := common.StepID("")
	if failedStep != nil {
		previousStepID = failedStep.ID
	}
	input := nextStepInput(run, previousStepID)
	input[stepInputKeySoulStage] = soulStageExecution
	setRuntimeStage(input, runtimeStageRepair)
	input[stepInputKeyRuntimeRepairFollowUp] = true
	input[stepInputKeyRuntimeRepairAttempt] = attempt
	input["maxRuntimeRepairAttempts"] = maxRuntimeRepairAttemptsForPolicy(o.policy)
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
	input["failedStepInput"] = o.llmContextManager().compactStepInput(failedStep.Input)
	input["failedStepOutput"] = o.llmContextManager().compactStepOutput(failedStep.Output)
	if runtimeRepairShouldReturnToRejectedResultContinuation(run, failedStep) {
		input = resetRuntimeRepairInputToRejectedResultContinuation(input, run, failedStep)
		contract := o.convergenceManager().build(run, nil, failedStep, map[string]any{"input": input}, nil)
		input = o.convergenceManager().apply(input, contract)
		return input
	}
	if diagnostics := runtimeRepairLastFailureDiagnostics(failedStep); len(diagnostics) > 0 {
		input["lastFailureDiagnostics"] = diagnostics
	}
	if diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		input["runtimeDiagnostics"] = cloneStepPayload(diagnostics)
		if stringFromMap(diagnostics, "kind") == "dod_evidence_quality_gap" {
			input["repairObjective"] = repairObjectiveConcreteValidationEvidence
			input["evidenceRepairRequired"] = true
			input["verificationEvidenceGap"] = cloneStepPayload(diagnostics)
			input["repairGuidance"] = concreteValidationEvidenceRepairGuidance()
		}
	}
	markCapabilityInputLocatorEvidenceRequired(input, failedStep)
	coverageContext := mapFromValue(failedStep.Output[coverageWorkItemContextKey])
	if len(coverageContext) == 0 {
		coverageContext = mapFromValue(mapFromValue(failedStep.Output["runtimeDiagnostics"])[coverageWorkItemContextKey])
	}
	if len(coverageContext) > 0 {
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
	assessmentOnlyCoverageRepair := doDCoverageContractCorrectionRequired(failedStep)
	if assessmentOnlyCoverageRepair {
		if missingGoalCoverage := missingGoalCoverageFromStep(failedStep); len(missingGoalCoverage) > 0 {
			input["missingGoalCoverage"] = missingGoalCoverageWithEvidenceCandidates(ctx, o.steps, run, failedStep, missingGoalCoverage)
		}
	}
	if missingDoD := missingDoDItemsFromStep(failedStep); len(missingDoD) > 0 {
		input["missingDoDItems"] = missingDoDItemsWithEvidenceCandidates(ctx, o.steps, run, failedStep, missingDoD)
	}
	if missingDeliverableCoverage := missingDeliverableCoverageFromStep(failedStep); len(missingDeliverableCoverage) > 0 &&
		len(mapSliceFromValue(input["missingDeliverableCoverage"])) == 0 {
		input["missingDeliverableCoverage"] = missingDeliverableCoverage
	}
	if inheritCoverageRepairContext(input, failedStep) {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Preserve inherited coverage gaps from the previous repair attempt; a contract failure on the repair wrapper does not clear the original Goal/DoD/deliverable gaps.",
			"Use missingDoDItems, missingGoalCoverage, and missingDeliverableCoverage as the current unresolved scope unless current evidence explicitly closes an item.",
		)
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
	validationRepairEscalatesToStateChanging := coverageGapEligibleForValidationRepair &&
		coverageValidationHasTargetEvidence &&
		coverageValidationRepairShouldEscalateToStateChanging(ctx, o.repairManager(), run, failedStep, input)
	if validationRepairEscalatesToStateChanging {
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
	coverageValidationEvidenceRepair = coverageValidationEvidenceRepair && !validationRepairEscalatesToStateChanging
	concreteEvidenceRepair := coverageNeedsConcreteEvidence || coverageValidationEvidenceRepair || (!assessmentOnlyCoverageRepair && doDVerificationEvidenceRepairRequired(failedStep))
	if concreteEvidenceRepair {
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
	if assessmentOnlyCoverageRepair && !coverageNeedsConcreteEvidence && !validationRepairEscalatesToStateChanging {
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
			"Treat evidenceCandidates as neutral candidates, not automatic proof; use them only when their concrete output supports the exact Goal requirement.",
			"When latestClosureEvidence is present, compare the latest successful evidence against older failed evidence in the same neutral scope; do not reject solely because an earlier superseded failure exists.",
			"If latestClosureEvidence is still insufficient, explain the exact missing dimension and cite the latest evidence that was considered.",
			"Each satisfied dodAssessment.goalCoverage item must cite concrete evidenceRefs from the candidate evidence or other available evidence.",
			"If evidence does not support a Goal or DoD item, mark it failed/skipped with reason, residualRisks, and nextActions instead of claiming satisfaction.",
		}
		if closure := latestCompletionClosureEvidence(ctx, o.steps, o.evidence, run, failedStep); len(closure) > 0 {
			input["latestClosureEvidence"] = closure
			if superseded := mapSliceFromValue(closure["supersededFailures"]); len(superseded) > 0 {
				input["supersededFailureEvidence"] = superseded
			}
		}
		if missingDeliverableCoverage := missingDeliverableCoverageFromStep(failedStep); len(missingDeliverableCoverage) > 0 {
			input["missingDeliverableCoverage"] = missingDeliverableCoverage
		}
		if requiredDeliverables := requiredProfileDeliverableExpectations(run); len(requiredDeliverables) > 0 {
			input["requiredDeliverableCoverage"] = requiredDeliverables
		}
	}
	if coverageNeedsConcreteEvidence {
		delete(input, stepInputKeyAssessmentOnly)
		delete(input, "assessmentOnlyReason")
		input["completionFollowUp"] = false
		input["evidenceRepairSource"] = "coverage_gap_without_candidate_evidence"
		if assessmentOnlyFailedCoverageRepair {
			input["evidenceRepairSource"] = "failed_assessment_only_coverage_gap"
		}
		if closure := latestCompletionClosureEvidence(ctx, o.steps, o.evidence, run, failedStep); len(closure) > 0 {
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
	if coverageValidationEvidenceRepair {
		input["boundedValidationRequired"] = true
		input["evidenceRepairSource"] = "coverage_gap_without_implementation_target"
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Unresolved Goal/DoD/deliverable coverage is currently a validation evidence gap because no concrete implementation target evidence is available.",
			"Produce bounded shell/tool/MCP validation evidence that maps to the coverageTargetSet before attempting another assessment.",
			"When an external dependency, credential, service, network endpoint, port, clock, filesystem path, or equivalent runtime prerequisite is unavailable, first try a neutral controlled substitute such as a temporary fixture, local stub/mock, alternate temporary config, isolated working directory, or available ephemeral port through the allowed validation tools; do not mark the item skipped or request a decision until such bounded substitute validation is impossible or would violate the Goal/DoD.",
			"Do not return patch/apply unless a later validation failure provides concrete target refs, line refs, diagnostics, or current target snapshots.",
		)
	}
	ensureCoverageRepairTargetSet(input)
	transientExecutionTimeout := runtimeRepairFailureIsTransientExecutionTimeout(failedStep, execErr)
	if transientExecutionTimeout {
		input["repairObjective"] = repairObjectiveTransientExecutionRetry
		input["executionTimeoutRetryAllowed"] = true
		input["repairGuidance"] = transientExecutionRetryGuidance()
	}
	if !assessmentOnlyCoverageRepair && !concreteEvidenceRepair && stateChangingRepairRequired(run, failedStep, execErr) {
		input["repairObjective"] = repairObjectiveStateChangingOutput
		input["stateChangingOutputRequired"] = true
		input["repairGuidance"] = stateChangingRepairGuidance()
	}
	if contract := capabilityPolicyRepairContractFromFailure(run, failedStep, execErr); len(contract) > 0 {
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
	if boolValue(input["stateChangingOutputRequired"]) &&
		(len(mapSliceFromValue(input["missingDoDItems"])) > 0 || len(missingGoalCoverageFromInput(input)) > 0) {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"When missingDoDItems or missingGoalCoverage are present, at least one patch/apply node must declare coverageTargets and each entry must match the exact missing DoD item text or goalRequirementId/item it is intended to close.",
			"Do not return a generic skeleton patch. If a state-changing artifact cannot be tied to a concrete missing coverage target, return a real governance blocker instead.",
		)
	}
	if len(mapSliceFromValue(input["unresolvedCoverageWorkItems"])) > 0 {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Resolve unresolvedCoverageWorkItems before returning an accepted result; each item requires its missing evidence kinds or a failed/skipped assessment with reason.",
			"For assessment_pending items, return a bounded dodAssessment that explicitly maps the item by id, sourceRequirementId, sourcePath, or targetText.",
			"For validation items, produce bounded validation evidence or mark the item failed/skipped; do not claim accepted completion while it remains unresolved.",
			"For state_change items, produce the smallest state-changing action that matches the coverage work item, then validate it.",
		)
	}
	class, signature := runtimeRepairScopeClassAndSignature(failedStep)
	if class != "" {
		input[failureOutputKeyClass] = class
		input[stepInputKeyRepairOfFailureClass] = class
	}
	if signature != "" {
		input[failureOutputKeySignature] = signature
		input[stepInputKeyRepairOfFailureSig] = signature
	}
	if signals := structuredFailureSignalsFromFailureOutput(failedStep.Output, execErr, class, signature); len(signals) > 0 {
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
	targetScope := runtimeFailureTargetScope(signature)
	if targetScope != "" {
		input["failureTargetScope"] = targetScope
		input["targetRepairAttempts"] = o.repairManager().runtimeRepairAttemptCountForFailureTargetInWindow(ctx, run.ID, class, targetScope, runtimeRepairTargetBudgetWindowRootForFailureTarget(failedStep))
		if boolValue(input["stateChangingOutputRequired"]) {
			input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
				"When failureTargetScope is present, verificationPlan must stay bounded to that scope and explain the exact targeted checks.",
				"Do not run unbounded verification for scoped repairs unless an explicit target scope or filter is provided.",
			)
		}
	}
	if reassessment := repairStrategyReassessmentFromInput(input); len(reassessment) > 0 {
		input["strategyReassessment"] = reassessment
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"strategyReassessment.required is true: explicitly choose repairMode as continue_same_target_repair or shift_root_cause_target, or return a real governance blocker.",
			"Explain the next repair target decision in neutral terms: failedSurface, rootCauseTarget, repairTargetDecision, repairMode, forbiddenRepair, and verificationPlan.",
			"When continuing the same failed target, patch only if the current target snapshot and primary diagnostics show a concrete same-target correction; otherwise choose a grounded root-cause target or return a blocker.",
			"When shifting root-cause target, patch a neutral grounded root-cause resource and declare relatedFailureTargets for the failed surface.",
			"Do not weaken still-valid Goal or DoD constraints, remove valid evidence, or reduce the requested outcome to make verification pass.",
		)
	}
	budgetScope := recoveryBudgetScope("runtime_repair", rootStepID, class, signature)
	if targetScope != "" {
		budgetScope["failureTargetScope"] = targetScope
	}
	input[stepInputKeyRecoveryBudgetScope] = budgetScope
	input["previousStepKind"] = failedStep.Kind
	input["previousStepOutput"] = o.llmContextManager().compactStepOutput(failedStep.Output)
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
	if graph, ok := runtimeGraphSpecFromStep(failedStep); ok {
		input["failedExecutionGraph"] = o.llmContextManager().compactExecutionGraph(graph)
		input[stepInputKeyGraphRootStepID] = string(graphRootStepID(failedStep))
	}
	governanceHistory := mapSliceFromValue(run.ExecutionContext["governanceHistory"])
	if current := o.currentRunGovernanceHistory(ctx, run); len(current) > 0 {
		governanceHistory = append(current, governanceHistory...)
	}
	if len(governanceHistory) > 0 {
		input["governanceHistory"] = compactGovernanceHistoryForRuntimeContext(governanceHistory, 6)
	}
	if evidence := o.graphManager().completedGraphEvidence(ctx, run, failedStep); len(evidence) > 0 {
		input[stepInputKeyGraphEvidence] = evidence
		if snapshots := targetSnapshotsFromConcreteFailureEvidence(evidence); len(snapshots) > 0 {
			input["failureTargetRepairContext"] = mergeRepairTargetSnapshots(mapSliceFromValue(input["failureTargetRepairContext"]), snapshots)
		}
	}
	if progress := runtimeRepairCurrentExecutionProgress(ctx, o.steps, o.repairManager(), run, failedStep, input); len(progress) > 0 {
		input["currentExecutionProgress"] = progress
	}
	patchContext := o.repairManager().runtimeRepairPatchContext(ctx, run, failedStep)
	if len(patchContext) > 0 {
		input["patchRepairContext"] = patchContext
		if suggestions := o.repairManager().runtimeRepairPatchSuggestions(ctx, run, failedStep); len(suggestions) > 0 {
			input["patchRepairSuggestions"] = suggestions
		}
		if satisfied := o.repairManager().runtimeRepairPatchAlreadySatisfiedSignals(ctx, run, failedStep); len(satisfied) > 0 {
			input["currentTargetAlreadySatisfiesFailedPatch"] = satisfied
			input["stateChangeAlreadySatisfiedByCurrentTarget"] = true
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
	}
	targetContext := o.repairManager().runtimeRepairFailureTargetContext(ctx, run, failedStep)
	if len(targetContext) > 0 {
		input["failureTargetRepairContext"] = targetContext
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Use failureTargetRepairContext as the current snapshot for targets identified by the failed evidence.",
			"When failed evidence points at a concrete target resource, generate the smallest auditable patch/apply state change against that current snapshot before rerunning verification.",
			"Use workspaceFileManifest to reconcile target resources with other known workspace resources; avoid replacing unrelated existing artifacts just to satisfy one failure target.",
			"When a failure mentions missing names, invalid attributes, or contract-entry mismatches, do not invent new names in the target patch; first reconcile the target with currently visible resource shape from relevant existing resources, or read the relevant resource before patching.",
			"When the failure text contains line-referenced unresolved names, invalid attributes, shape mismatches, or equivalent resource-shape diagnostics in the failure target, repair those reported blockers before secondary validation, formatting, or evidence refinements.",
		)
	}
	if shapeContext := o.repairManager().runtimeRepairResourceShapeAlignmentContext(ctx, run, failedStep); len(shapeContext) > 0 {
		input["resourceShapeAlignmentContext"] = shapeContext
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Use resourceShapeAlignmentContext as current same-directory resource evidence when diagnostics report unresolved names, invalid attributes, shape mismatches, contract-entry mismatches, or equivalent resource-shape conflicts.",
			"Before introducing new named elements or attributes, reconcile the failed target with existing current workspace resources in resourceShapeAlignmentContext.",
			"If the failed target disagrees with current resource shape, prefer the smallest coherent state change that aligns the target and existing resources; do not invent replacement contracts from failure text alone.",
		)
	}
	rootCauseCandidates := o.repairManager().runtimeRepairRootCauseCandidateContext(ctx, run, failedStep)
	if len(rootCauseCandidates) > 0 {
		input["rootCauseCandidateContext"] = rootCauseCandidates
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Use rootCauseCandidateContext to evaluate grounded adjacent or referenced resources when the failed surface may be unblocked by changing a related current workspace resource.",
			"Do not assume the failed surface itself is the root cause when current workspace resources show adjacent or referenced resources that the failed surface depends on.",
			"If choosing a rootCauseTarget that differs from the failed surface, patch that resource directly and declare relatedFailureTargets for the failed surface being unblocked.",
		)
	}
	if context := convertedDecisionReadableResourceContext(ctx, o.workspaces, run, failedStep); len(context) > 0 {
		input["readableResourceRepairContext"] = context
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"The previous decision requested information that is already available as a readable workspace resource.",
			"Use readableResourceRepairContext as current evidence; do not ask the operator to paste the same resource content.",
		)
	}
	if shouldIncludeWorkspaceManifestInRuntimeRepair(failedStep) || len(targetContext) > 0 || len(patchContext) > 0 || len(rootCauseCandidates) > 0 {
		manifest := o.repairManager().workspaceFileManifest(ctx, run)
		if len(manifest) > 0 {
			input[stepInputKeyWorkspaceFileManifest] = manifest
		}
	}
	inheritPriorRepairContext(input, failedStep)
	preserveStateChangingSchemaInvalidRepairContract(input, failedStep)
	if boolValue(input["stateChangingOutputRequired"]) {
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
			input["repairActionPackage"] = pkg
			compactStateChangingRepairInputAroundActionPackage(input)
		}
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Use repairActionPackage as the primary actionable repair contract; it contains the required output kind, rejected output summary, and current target snapshots.",
		)
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
	if boolValue(input["validationRepairEscalatedToImplementation"]) {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Repeated validation-only repair is no longer an acceptable next action for this coverage gap.",
			"If concrete target evidence is still missing, acquire bounded implementation target evidence first; otherwise produce a state-changing repair before the next validation.",
		)
	}
	assessmentOnlySchemaInvalidRepair := preserveAssessmentOnlySchemaInvalidRepairContract(input, failedStep)
	if !assessmentOnlySchemaInvalidRepair {
		preserveConcreteValidationSchemaInvalidRepairContract(input, failedStep)
	}
	o.applyLLMOutputModeSelection(ctx, run, input, failedStep, attempt)
	contract := o.convergenceManager().build(run, nil, failedStep, map[string]any{"input": input}, nil)
	input = o.convergenceManager().apply(input, contract)
	o.applyLLMOutputModeSelection(ctx, run, input, failedStep, attempt)
	return input
}
