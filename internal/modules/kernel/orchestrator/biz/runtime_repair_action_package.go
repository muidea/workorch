package orchestrator

import (
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func repairActionPackageFromRepairInput(input map[string]any, rawFailedOutput ...map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	anchorRepairInputToPrimaryActionableFailure(input, rawFailedOutput...)
	enrichRepairInputWithProgressFailureSignals(input)
	refreshRepairStrategyReassessment(input)
	priorPackage := mapFromValue(input["repairActionPackage"])
	if pkg := validationCommandContractRepairActionPackage(input, priorPackage, rawFailedOutput...); len(pkg) > 0 {
		return pkg
	}
	requiredOutput := stateChangingRepairRequiredOutput()
	pkg := map[string]any{
		"requiredOutput": requiredOutput,
	}
	mergePriorRepairActionPackageEvidence(pkg, requiredOutput, priorPackage)
	if currentFailure := repairActionCurrentFailure(input, rawFailedOutput...); len(currentFailure) > 0 {
		pkg["currentFailure"] = currentFailure
	}
	if diagnostics := repairActionLastFailureDiagnostics(input, rawFailedOutput...); len(diagnostics) > 0 {
		pkg["lastFailureDiagnostics"] = diagnostics
	}
	if diagnostic := repairActionValidationFailureDiagnostic(input, rawFailedOutput...); len(diagnostic) > 0 {
		pkg["validationFailureDiagnostic"] = diagnostic
		requiredOutput["mustAddressValidationDiagnostic"] = true
		if targetType := strings.TrimSpace(stringFromMap(diagnostic, "recommendedEvidenceTargetType")); targetType != "" {
			requiredOutput["recommendedEvidenceTargetType"] = targetType
		}
	}
	for _, key := range []string{
		failureOutputKeyClass,
		failureOutputKeySignature,
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"failureReason",
		"repairObjective",
		"runtimeRepairAttempt",
		"maxRuntimeRepairAttempts",
		"targetRepairAttempts",
		"strategyReassessment",
	} {
		if value, ok := input[key]; ok && value != nil {
			pkg[key] = compactLLMContextValue(value, 1)
		}
	}
	if len(rawFailedOutput) > 0 && len(rawFailedOutput[0]) > 0 {
		pkg["previousRejectedOutputSummary"] = compactStateChangingRecoveryFailureOutput(rawFailedOutput[0])
	} else if failedOutput := mapFromValue(input["failedStepOutput"]); len(failedOutput) > 0 {
		pkg["previousRejectedOutputSummary"] = compactStateChangingRecoveryFailureOutput(failedOutput)
	}
	if signals := mapFromValue(input["structuredFailureSignals"]); len(signals) > 0 {
		pkg["structuredFailureSignals"] = compactStructuredFailureSignalsForRepairActionPackage(signals)
	} else if len(rawFailedOutput) > 0 && len(rawFailedOutput[0]) > 0 {
		if signals := structuredFailureSignalsFromFailureOutput(rawFailedOutput[0], nil, "", ""); len(signals) > 0 {
			pkg["structuredFailureSignals"] = compactStructuredFailureSignalsForRepairActionPackage(signals)
		}
	}
	if primary := mapFromValue(input["primaryDiagnosticRepair"]); len(primary) > 0 {
		pkg["primaryDiagnosticRepair"] = compactLLMContextMap(primary, 1)
		requiredOutput["mustAddressPrimaryDiagnostics"] = true
		requiredOutput["primaryDiagnosticMatchRule"] = "state-changing patch/apply must address line-referenced target diagnostics, or declare matching primaryDiagnosticResolution/addressesFailureSignals with relatedFailureTargets for a related target"
	} else if signals := mapFromValue(pkg["structuredFailureSignals"]); len(signals) > 0 {
		if primary := primaryDiagnosticRepairContractFromSignals(signals); len(primary) > 0 {
			pkg["primaryDiagnosticRepair"] = compactLLMContextMap(primary, 1)
			requiredOutput["mustAddressPrimaryDiagnostics"] = true
			requiredOutput["primaryDiagnosticMatchRule"] = "state-changing patch/apply must address line-referenced target diagnostics, or declare matching primaryDiagnosticResolution/addressesFailureSignals with relatedFailureTargets for a related target"
		}
	}
	if forbidden := forbiddenRepairContentReuseFromFailure(input, rawFailedOutput...); len(forbidden) > 0 {
		pkg["forbiddenContentReuse"] = forbidden
		requiredOutput["mustRebuildFromCurrentEvidence"] = true
	}
	if targets := repairActionTargetSnapshots(input); len(targets) > 0 {
		pkg["targetSnapshots"] = targets
		pkg["currentTargetSnapshots"] = repairActionTargetSnapshotMetadata(targets)
		requiredOutput["mustTargetFailureScope"] = true
		requiredOutput["targetRelation"] = "failure_target_repair_snapshot_or_declared_related_failure_target"
		applyTargetSnapshotSafetyRequiredOutputContract(requiredOutput, targets)
		if !repairTargetSnapshotsHaveExactPatchSafe(targets) {
			pkg["targetSnapshotAcquisitionRequired"] = true
			pkg["targetSnapshotAcquisitionReason"] = "no_exact_patch_safe_runtime_managed_target_snapshot"
		}
	}
	if candidates := mapSliceFromValue(input["rootCauseCandidateContext"]); len(candidates) > 0 {
		pkg["rootCauseCandidateContext"] = compactLLMContextValue(candidates, 1)
		requiredOutput["rootCauseCandidateRule"] = "a grounded rootCauseCandidateContext target may be patched when the patch declares relatedFailureTargets for the failed surface or the candidate is included in repairTargetSet"
		applyGroundedRootCauseCandidateRepairMode(input, pkg, requiredOutput, candidates)
		if len(mapSliceFromValue(pkg["targetSnapshots"])) == 0 {
			if promoted := repairActionPromotableRootCauseCandidateTargetSnapshots(candidates); len(promoted) > 0 {
				pkg["targetSnapshots"] = promoted
				pkg["currentTargetSnapshots"] = repairActionTargetSnapshotMetadata(promoted)
				pkg["rootCauseCandidateTargetsPromoted"] = true
				requiredOutput["mustTargetFailureScope"] = true
				requiredOutput["targetRelation"] = "grounded_root_cause_candidate_or_declared_related_failure_target"
				applyTargetSnapshotSafetyRequiredOutputContract(requiredOutput, promoted)
				if !repairTargetSnapshotsHaveExactPatchSafe(promoted) {
					pkg["targetSnapshotAcquisitionRequired"] = true
					pkg["targetSnapshotAcquisitionReason"] = "promoted_target_snapshot_not_exact_patch_safe"
				}
			}
		}
	}
	if shapeContext := mapSliceFromValue(input["resourceShapeAlignmentContext"]); len(shapeContext) > 0 {
		pkg["resourceShapeAlignmentContext"] = compactLLMContextValue(shapeContext, 1)
		requiredOutput["resourceShapeAlignmentRule"] = "resourceShapeAlignmentContext is concrete current evidence for resource or contract shape; patch decisions must reconcile target snapshots with this evidence instead of inventing names, attributes, or contract entries"
	}
	if targetSet := repairActionTargetSet(input, pkg, rawFailedOutput...); len(targetSet) > 0 {
		pkg["repairTargetSet"] = targetSet
		requiredOutput["mustTargetRepairTargetSet"] = true
	}
	if coverageTargets := coverageTargetSetFromRepairInput(input); len(coverageTargets) > 0 {
		pkg["coverageRepairRequired"] = true
		pkg["coverageTargetSet"] = coverageTargets
		requiredOutput["mustDeclareCoverageTargets"] = true
		requiredOutput["coverageTargetMatchRule"] = "patch/apply coverageTargets must match one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
	}
	if context := coverageWorkItemContextFromInput(input); len(context) > 0 {
		context = coverageWorkItemContextAlignedToFocus(context, repairCoverageFocusFromInput(input))
		input[coverageWorkItemContextKey] = context
		pkg[coverageWorkItemContextKey] = context
	}
	applyRepairCoverageFocusContract(input, pkg, requiredOutput)
	if progress := mapFromValue(input["currentExecutionProgress"]); len(progress) > 0 {
		pkg["currentExecutionProgress"] = compactLLMContextMap(progress, 0)
	}
	if digest := failedPatchDigestFromRepairFailure(input, rawFailedOutput...); len(digest) > 0 {
		pkg["failedPatchDigest"] = digest
	}
	if satisfied := mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"]); len(satisfied) > 0 {
		pkg["currentTargetAlreadySatisfiesFailedPatch"] = satisfied
		pkg["stateChangeAlreadySatisfiedByCurrentTarget"] = true
		requiredOutput["mustIncludeStateChangingNode"] = false
		requiredOutput["mustProduceConcreteValidationEvidence"] = true
		requiredOutput["acceptedKinds"] = []string{"shell/command", "mcp/tool_call"}
		requiredOutput["forbiddenKinds"] = append(stringSliceFromValue(requiredOutput["forbiddenKinds"]), "repeat_same_failed_patch")
	}
	forbiddenActions := repairActionForbiddenActions(input, pkg)
	if len(forbiddenActions) > 0 {
		pkg["forbiddenActions"] = forbiddenActions
	}
	if mode := repairActionRequiredRepairMode(input, pkg, rawFailedOutput...); mode != "" {
		pkg["requiredRepairMode"] = mode
	}
	if repairActionFailureIndicatesNoOpPatch(input, firstRawFailureOutput(input, rawFailedOutput...)) {
		pkg["lastRejectedOutputContract"] = map[string]any{
			"reason":           "patch_content_required",
			"forbiddenPattern": "no_op_text_replace",
			"requirement":      "patch/apply text_replace oldText and newText must be different substantive literal strings",
		}
		requiredOutput["mustAvoidNoOpPatch"] = true
		requiredOutput["mustProduceSubstantivePatch"] = true
		requiredOutput["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(requiredOutput["forbiddenKinds"]),
			"no_op_patch",
			"identical_text_replace",
		))
	}
	if contract := mapFromValue(input["capabilityPolicyRepairContract"]); len(contract) > 0 {
		pkg["capabilityPolicyRepairRequired"] = true
		pkg["capabilityPolicyRepairContract"] = compactLLMContextMap(contract, 1)
		requiredOutput["mustRepairCapabilityPolicyFollowUp"] = true
		requiredOutput["capabilityPolicyFollowUpRule"] = "when an execution graph includes a trigger kind from capabilityPolicyRepairContract.triggeredPolicies.requiredAfterKinds, it must include a dependent mcp/tool_call matching preferredTools or output a structured capabilityPolicySkips item with policyId/category and non-empty reason"
		requiredOutput["capabilityPolicyRepairScopeRule"] = "capabilityPolicySkips or dependent mcp/tool_call repairs the graph contract at top level; patch/apply coverageTargets repair Goal/DoD/deliverable state change and must not be forced to use the graph-contract failure as their business failedSurface"
		requiredOutput["acceptedKinds"] = compactUniqueStrings(append(stringSliceFromValue(requiredOutput["acceptedKinds"]), "mcp/tool_call"))
		requiredOutput["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(requiredOutput["forbiddenKinds"]), "assessment_only_capability_policy_repair"))
	}
	if budget := repairActionBudgetState(input, pkg); len(budget) > 0 {
		pkg["repairBudget"] = budget
	}
	if repairActionIsSchemaInvalidRepair(input, rawFailedOutput...) {
		pkg["schemaInvalidRecovery"] = map[string]any{
			"required": true,
			"reason":   "previous_llm_decision_json_invalid",
			"requirements": []string{
				"return exactly one valid JSON object",
				"do not reuse rejected raw output previews",
				"do not include markdown, comments, duplicate objects, trailing braces, or partial arrays",
				"make every executionGraph node a complete JSON object",
			},
		}
		requiredOutput["mustReturnValidJSON"] = true
		requiredOutput["mustNotReuseRejectedJSON"] = true
		requiredOutput["forbiddenKinds"] = append(stringSliceFromValue(requiredOutput["forbiddenKinds"]),
			"malformed_json",
			"markdown_wrapped_json",
			"raw_rejected_json_reuse",
			"partial_json_array",
		)
	}
	pkg["exitCriteria"] = []string{
		"state_changing_artifact_applied_then_verify",
		"failed_assessment_or_blocker_when_not_repairable",
	}
	if manifest := repairActionWorkspaceManifest(input); len(manifest) > 0 {
		pkg["workspaceFileManifest"] = manifest
	}
	if diagnostics := repairActionTargetRelevanceDiagnostics(input, pkg); len(diagnostics) > 0 {
		pkg["targetRelevanceDiagnostics"] = diagnostics
	}
	if int64FromValue(input["targetRepairAttempts"]) >= 2 {
		pkg["repeatedTargetRepairMode"] = map[string]any{
			"enabled":                           true,
			"reason":                            "same_target_scope_repeated_failure",
			"maxTargetScopedRepairInputBytes":   120 * 1024,
			"mustPreferExactTargetSnapshot":     true,
			"mustOmitRejectedRawFailureProduct": true,
			"mustReassessRepeatedFailedSurface": true,
		}
		if snapshots := mapSliceFromValue(pkg["targetSnapshots"]); repairTargetSnapshotsHaveExactPatchSafe(snapshots) {
			applyCurrentSnapshotDirectPatchContract(requiredOutput)
		} else {
			applyTargetSnapshotAcquisitionRequiredOutputContract(requiredOutput)
			pkg["targetSnapshotAcquisitionRequired"] = true
			pkg["targetSnapshotAcquisitionReason"] = "repeated_target_repair_needs_exact_safe_snapshot"
		}
		requiredOutput["mustRebuildFromCurrentEvidence"] = true
		if len(mapFromValue(pkg["strategyReassessment"])) == 0 {
			if reassessment := repairStrategyReassessmentFromInput(input); len(reassessment) > 0 {
				pkg["strategyReassessment"] = reassessment
			}
		}
	}
	if shapeContract := repeatedShapeContractEvidenceRequirement(input, pkg); len(shapeContract) > 0 {
		pkg["shapeContractEvidenceRequired"] = shapeContract
		pkg["evidenceAcquisitionRequired"] = true
		pkg["evidenceAcquisitionReason"] = "repeated_shape_or_contract_diagnostic_without_independent_shape_evidence"
		next := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, requiredOutput)
		next["mustAcquireShapeContractEvidence"] = true
		next["mustNotGuessContractShape"] = true
		next["shapeContractEvidenceRule"] = "before any next patch, acquire bounded evidence for the current resource or dependency contract shape using acceptedKinds; do not invent fields, methods, schemas, constructors, or callable signatures from diagnostics alone"
		next["patchForbiddenUntilShapeEvidence"] = true
		for key := range requiredOutput {
			delete(requiredOutput, key)
		}
		for key, value := range next {
			requiredOutput[key] = value
		}
		pkg["exitCriteria"] = []string{
			"shape_contract_evidence_acquired_then_target_scoped_repair",
			"failed_assessment_or_blocker_when_shape_contract_cannot_be_acquired",
		}
	}
	applyConcreteRepairEvidenceOutputContract(input, pkg, requiredOutput)
	ensureConcreteValidationScratchContract(input, pkg, requiredOutput)
	addStrategyReassessmentRequiredOutputContract(pkg, requiredOutput)
	reapplyRepairCoverageFocusContract(input, pkg)
	if contract := repairActionOutputDecisionContract(input, pkg, requiredOutput, rawFailedOutput...); len(contract) > 0 {
		pkg["outputDecisionContract"] = contract
	}
	if len(pkg) <= 1 {
		return nil
	}
	return pkg
}

func validationCommandContractRepairActionPackage(input map[string]any, priorPackage map[string]any, rawFailedOutput ...map[string]any) map[string]any {
	diagnostic := validationCommandContractFailureDiagnosticFromSources(append([]map[string]any{input, priorPackage}, rawFailedOutput...)...)
	if len(diagnostic) == 0 {
		return nil
	}
	pkg := cloneStepPayload(priorPackage)
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	pkg["repairObjective"] = repairObjectiveConcreteValidationEvidence
	pkg["boundedValidationRequired"] = true
	pkg["evidenceRepairRequired"] = true
	pkg["validationCommandContractRetry"] = true
	pkg["validationFailureDiagnostic"] = diagnostic
	if currentFailure := repairActionCurrentFailure(input, rawFailedOutput...); len(currentFailure) > 0 {
		pkg["currentFailure"] = currentFailure
	}
	if diagnostics := repairActionLastFailureDiagnostics(input, rawFailedOutput...); len(diagnostics) > 0 {
		pkg["lastFailureDiagnostics"] = diagnostics
	}
	if context := coverageWorkItemContextFromInput(input); len(context) > 0 {
		context = coverageWorkItemContextAlignedToFocus(context, repairCoverageFocusFromInput(input))
		input[coverageWorkItemContextKey] = context
		pkg[coverageWorkItemContextKey] = context
	}
	stripRepairActionPackageImplementationTargetEvidence(pkg)
	requiredOutput := concreteValidationEvidenceRequiredOutputWithScratch(input, pkg, mapFromValue(pkg["requiredOutput"]))
	requiredOutput["mustAddressValidationDiagnostic"] = true
	requiredOutput["validationCommandContractRetry"] = true
	if targetType := strings.TrimSpace(stringFromMap(diagnostic, "recommendedEvidenceTargetType")); targetType != "" {
		requiredOutput["recommendedEvidenceTargetType"] = targetType
	}
	pkg["requiredOutput"] = requiredOutput
	pkg["exitCriteria"] = []string{
		"bounded_validation_evidence_acquired_then_assess",
		"validation_contract_retry_exhausted_blocker",
	}
	reapplyRepairCoverageFocusContract(input, pkg)
	if contract := repairActionOutputDecisionContract(input, pkg, requiredOutput, rawFailedOutput...); len(contract) > 0 {
		pkg["outputDecisionContract"] = contract
	}
	return pkg
}

func applyGroundedRootCauseCandidateRepairMode(input map[string]any, pkg map[string]any, requiredOutput map[string]any, candidates []map[string]any) {
	if len(input) == 0 || len(pkg) == 0 || len(requiredOutput) == 0 || len(candidates) == 0 {
		return
	}
	reassessment := mapFromValue(input["strategyReassessment"])
	if len(reassessment) == 0 {
		reassessment = repairStrategyReassessmentFromInput(input)
	}
	if !strategyReassessmentRequiresRootCauseTargetShift(reassessment) {
		return
	}
	if !boolValue(input["stateChangingOutputRequired"]) && strings.TrimSpace(stringFromMap(input, "repairObjective")) != repairObjectiveStateChangingOutput {
		return
	}
	failedSurfaceRefs := strategySurfaceTargetRefs(firstNonEmptyString(
		strings.TrimSpace(stringFromMap(reassessment, "failedSurface")),
		strings.TrimSpace(stringFromMap(input, "failureTargetScope")),
	))
	if !rootCauseCandidatesContainDistinctTarget(candidates, failedSurfaceRefs) {
		return
	}
	next := cloneStepPayload(reassessment)
	if len(next) == 0 {
		return
	}
	next["allowedRepairModes"] = []string{strategyRepairModeShiftRootCauseTarget}
	next["requiredRepairMode"] = strategyRepairModeShiftRootCauseTarget
	next["forbiddenRepairModes"] = []string{strategyRepairModeContinueSameTarget}
	next["modePolicy"] = "grounded_root_cause_candidate_required"
	next["modePolicyReason"] = "repeated failed-surface repair has grounded distinct root-cause candidates; continuing the same surface would risk non-convergent auxiliary repair"
	input["strategyReassessment"] = next
	pkg["strategyReassessment"] = next
	requiredOutput["strategyReassessmentRepairModes"] = []string{strategyRepairModeShiftRootCauseTarget}
	requiredOutput["mustShiftRootCauseTarget"] = true
	requiredOutput["sameTargetContinuationRule"] = "repairMode=continue_same_target_repair is forbidden when strategyReassessment.allowedRepairModes only permits shift_root_cause_target"
	requiredOutput["rootCauseTargetShiftRule"] = "repairMode=shift_root_cause_target must patch a distinct grounded root-cause candidate target or return a real governance blocker"
}

func rootCauseCandidatesContainDistinctTarget(candidates []map[string]any, failedSurfaceRefs []string) bool {
	for _, candidate := range candidates {
		targetRef := normalizeRepairPatchTargetRef(strings.TrimSpace(stringFromMap(candidate, "targetRef")))
		if targetRef == "" {
			continue
		}
		if len(failedSurfaceRefs) == 0 || !repairPatchTargetRefMatches(targetRef, failedSurfaceRefs) {
			return true
		}
	}
	return false
}

func ensureAvailableTargetSnapshotsForStateChangingRepair(input map[string]any) {
	if len(input) == 0 {
		return
	}
	pkg := mapFromValue(input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	if !inputNeedsAvailableTargetSnapshotContract(input, required) {
		return
	}
	targets := repairActionTargetSnapshots(input)
	if len(targets) == 0 {
		targets = mapSliceFromValue(pkg["targetSnapshots"])
	}
	if candidates := repairActionPromotableRootCauseCandidateTargetSnapshots(referencedWorkspaceResourceRootCauseCandidates(input)); len(candidates) > 0 {
		targets = mergeRepairTargetSnapshots(targets, candidates)
	}
	if len(targets) == 0 {
		return
	}
	exactSafeTargets := exactPatchSafeRepairTargetSnapshots(targets)
	metadata := repairActionTargetSnapshotMetadata(exactSafeTargets)
	if len(metadata) > 0 {
		input["availableTargetSnapshots"] = metadata
	}
	if len(pkg) == 0 {
		return
	}
	merged := mergeRepairTargetSnapshots(mapSliceFromValue(pkg["targetSnapshots"]), targets)
	if len(merged) == 0 {
		return
	}
	pkg["targetSnapshots"] = merged
	pkg["currentTargetSnapshots"] = repairActionTargetSnapshotMetadata(merged)
	if len(required) == 0 {
		required = stateChangingRepairRequiredOutput()
	}
	required["mustTargetFailureScope"] = true
	required["targetRelation"] = "failure_target_repair_snapshot_or_declared_related_failure_target"
	applyTargetSnapshotSafetyRequiredOutputContract(required, merged)
	if !repairTargetSnapshotsHaveExactPatchSafe(merged) {
		pkg["targetSnapshotAcquisitionRequired"] = true
		pkg["targetSnapshotAcquisitionReason"] = "available_target_snapshot_not_exact_patch_safe"
	}
	pkg["requiredOutput"] = required
	input["repairActionPackage"] = pkg
}

func referencedWorkspaceResourceRootCauseCandidates(input map[string]any) []map[string]any {
	candidates := mapSliceFromValue(input["rootCauseCandidateContext"])
	if len(candidates) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(candidates))
	for _, candidate := range candidates {
		if stringFromMap(candidate, "source") == "failure_target_referenced_workspace_resource" {
			ret = append(ret, candidate)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func inputNeedsAvailableTargetSnapshotContract(input map[string]any, required map[string]any) bool {
	if boolValue(input["stateChangingOutputRequired"]) ||
		stringFromMap(input, "repairObjective") == repairObjectiveStateChangingOutput ||
		boolValue(required["mustIncludeStateChangingNode"]) ||
		boolValue(required["mustReturnPatchApplyNode"]) ||
		boolValue(required["mustUseCurrentTargetSnapshot"]) {
		return true
	}
	return false
}

func anchorRepairInputToPrimaryActionableFailure(input map[string]any, rawFailedOutput ...map[string]any) {
	if len(input) == 0 {
		return
	}
	priorPackage := mapFromValue(input["repairActionPackage"])
	if len(priorPackage) == 0 {
		return
	}
	output := firstRawFailureOutput(input, rawFailedOutput...)
	if !runtimeRepairWrapperFailureOutput(output) && !runtimeRepairWrapperFailureOutput(mapFromValue(input["failedStepOutput"])) {
		return
	}
	primary := primaryActionableFailureFromRepairPackage(priorPackage)
	if len(primary) == 0 {
		primary = primaryActionableFailureFromRepairInputContinuations(input)
	}
	if len(primary) > 0 {
		input["currentFailure"] = primary
		for _, key := range []string{"failureClass", "failureSignature", "failedStepId", "failedStepKind", "failureTargetScope", "failureReason"} {
			if value, ok := primary[key]; ok && strings.TrimSpace(stringFromAny(value)) != "" {
				input[key] = value
			}
		}
		if scope := strings.TrimSpace(firstNonEmptyString(stringFromMap(primary, "targetScope"), stringFromMap(input, "failureTargetScope"))); scope != "" {
			input["failureTargetScope"] = scope
		}
	}
	for _, key := range []string{
		"targetSnapshots",
		"currentTargetSnapshots",
		"repairTargetSet",
		"primaryDiagnosticRepair",
		"rootCauseCandidateContext",
		"strategyReassessment",
	} {
		if priorPackage[key] != nil {
			input[key] = cloneStepInputValue(priorPackage[key])
		}
	}
	if signals := primaryStructuredFailureSignalsFromRepairPackage(priorPackage); len(signals) > 0 {
		input["structuredFailureSignals"] = signals
	} else if signals := structuredFailureSignalsFromPrimaryActionableFailure(primary); len(signals) > 0 {
		input["structuredFailureSignals"] = signals
	}
	if diagnostics := primaryLastFailureDiagnosticsFromRepairPackage(priorPackage); len(diagnostics) > 0 {
		input["lastFailureDiagnostics"] = diagnostics
	}
	if output != nil {
		input["recentRejectedAttempt"] = compactStateChangingRecoveryFailureOutput(output)
	}
}

func primaryActionableFailureFromRepairInputContinuations(input map[string]any) map[string]any {
	for _, container := range []map[string]any{
		input,
		mapFromValue(input["executionContext"]),
		mapFromValue(input[stepInputKeyExecutionContextPackage]),
		mapFromValue(mapFromValue(input[stepInputKeyExecutionContextPackage])["executionContext"]),
	} {
		if primary := primaryActionableFailureFromReplanContinuation(mapFromValue(container["replanContinuation"])); len(primary) > 0 {
			return primary
		}
	}
	return nil
}

func runtimeRepairWrapperFailureOutput(output map[string]any) bool {
	if len(output) == 0 {
		return false
	}
	class := strings.TrimSpace(stringFromMap(output, failureOutputKeyClass))
	stage := strings.TrimSpace(stringFromMap(output, "stage"))
	text := strings.ToLower(strings.Join([]string{
		stage,
		class,
		stringFromMap(output, "error"),
		stringFromMap(output, "reason"),
		stringFromMap(output, failureOutputKeySignature),
	}, "\n"))
	return class == failureClassLLMDecisionInvalid ||
		stage == "llm_schema_invalid" ||
		stage == "llm_context_integrity_failed" ||
		strings.Contains(text, "invalid llm decision json") ||
		strings.Contains(text, "llm context gate") ||
		strings.Contains(text, "context integrity") ||
		strings.Contains(text, "executiongraph patch/apply must not depend on same-graph file/read") ||
		strings.Contains(text, "state-changing llm repair step returned no state-changing output") ||
		strings.Contains(text, "state-changing llm repair step returned executiongraph without patch/apply") ||
		strings.Contains(text, "decision requests a continuable candidate choice") ||
		strings.Contains(text, "executiongraph patch/apply input invalid") ||
		strings.Contains(text, "runtime repair patch/apply target") ||
		strings.Contains(text, "not related to failure target scope") ||
		strings.Contains(text, "patch a current failure target") ||
		strings.Contains(text, "capability usage policy") ||
		strings.Contains(text, "dependent matching mcp/tool_call") ||
		strings.Contains(text, "capabilitypolicyskips")
}

func primaryActionableFailureFromRepairPackage(pkg map[string]any) map[string]any {
	if len(pkg) == 0 {
		return nil
	}
	for _, candidate := range []map[string]any{
		mapFromValue(pkg["primaryActionableFailure"]),
		mapFromValue(pkg["originalActionableFailure"]),
		mapFromValue(pkg["currentFailure"]),
		mapFromValue(pkg["lastFailureDiagnostics"]),
	} {
		if len(candidate) == 0 || runtimeRepairWrapperFailureOutput(candidate) {
			continue
		}
		return compactLLMContextMap(candidate, 1)
	}
	if primary := mapFromValue(pkg["primaryDiagnosticRepair"]); len(primary) > 0 {
		ret := map[string]any{
			"evidenceValidity": "actionable_failure",
			"reason":           firstNonEmptyString(stringSliceFirst(stringSliceFromValue(primary["messages"])), stringFromMap(pkg, "failureReason")),
		}
		if refs := stringSliceFromValue(primary["targetRefs"]); len(refs) > 0 {
			ret["targetRef"] = strings.Join(limitedStrings(refs, maxLLMContextSliceItems), ",")
			ret["targetScope"] = "failureTargets=" + strings.Join(limitedStrings(refs, maxLLMContextSliceItems), ",")
		}
		for _, key := range []string{"failureClass", "failureSignature", "failedStepId", "failedStepKind"} {
			if value := strings.TrimSpace(firstNonEmptyString(stringFromMap(pkg, key), stringFromMap(primary, key))); value != "" {
				ret[key] = value
			}
		}
		if len(ret) > 1 {
			return compactLLMContextMap(ret, 1)
		}
	}
	return nil
}

func primaryStructuredFailureSignalsFromRepairPackage(pkg map[string]any) map[string]any {
	if primary := mapFromValue(pkg["primaryDiagnosticRepair"]); len(primary) > 0 && structuredFailureSignalsHaveConcreteTargetEvidence(primary) {
		return compactLLMContextMap(primary, 1)
	}
	if signals := mapFromValue(pkg["structuredFailureSignals"]); len(signals) > 0 && structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
		return compactStructuredFailureSignalsForRepairActionPackage(signals)
	}
	return nil
}

func primaryLastFailureDiagnosticsFromRepairPackage(pkg map[string]any) map[string]any {
	for _, candidate := range []map[string]any{
		mapFromValue(pkg["lastFailureDiagnostics"]),
		mapFromValue(pkg["currentFailure"]),
	} {
		if len(candidate) == 0 || runtimeRepairWrapperFailureOutput(candidate) {
			continue
		}
		return compactFailureStepOutputForRuntimeContext(candidate)
	}
	if primary := mapFromValue(pkg["primaryDiagnosticRepair"]); len(primary) > 0 {
		return compactLLMContextMap(primary, 1)
	}
	return nil
}

func stringSliceFirst(values []string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func mergePriorRepairActionPackageEvidence(pkg map[string]any, requiredOutput map[string]any, prior map[string]any) {
	if len(pkg) == 0 || len(prior) == 0 {
		return
	}
	for _, key := range []string{
		"targetSnapshots",
		"currentTargetSnapshots",
		"repairTargetSet",
		"coverageTargetSet",
		"structuredFailureSignals",
		"primaryDiagnosticRepair",
		"failureTargetRepairContext",
		"patchRepairContext",
		"currentTargetAlreadySatisfiesFailedPatch",
		"stateChangeAlreadySatisfiedByCurrentTarget",
	} {
		if _, ok := pkg[key]; ok || prior[key] == nil {
			continue
		}
		pkg[key] = compactLLMContextValue(prior[key], 1)
	}
	if snapshots := mapSliceFromValue(pkg["targetSnapshots"]); len(snapshots) > 0 {
		requiredOutput["mustTargetFailureScope"] = true
		applyTargetSnapshotSafetyRequiredOutputContract(requiredOutput, snapshots)
	}
	if len(stringSliceFromValue(pkg["repairTargetSet"])) > 0 {
		requiredOutput["mustTargetRepairTargetSet"] = true
	}
	if len(mapSliceFromValue(pkg["coverageTargetSet"])) > 0 {
		requiredOutput["mustDeclareCoverageTargets"] = true
	}
}

func stateChangingRepairRequiredOutput() map[string]any {
	out := map[string]any{
		"mustIncludeStateChangingNode": true,
		"acceptedKinds":                stateChangingRepairAcceptedKinds(),
		"toolStateChangingInputContract": map[string]any{
			"requiredWhenKind": []string{"shell/command", "mcp/tool_call"},
			"requiredFields":   []string{"stateChanging", "targetRef", "stateChangingReason"},
			"relationFields":   []string{"relatedFailureTargets", "targetRefs", "affectedTargets"},
			"verificationFields": []string{
				"verificationScope",
				"targetRef",
				"relatedFailureTargets",
				"boundedReason",
			},
		},
		"toolStateChangingRule":      "when shell/command or mcp/tool_call is the state-changing node, declare input.stateChanging=true, input.targetRef, input.stateChangingReason, and a relation field such as relatedFailureTargets/targetRefs/affectedTargets; a dependent verification shell/tool node must declare verificationScope, targetRef, relatedFailureTargets, or boundedReason",
		"patchSubstantiveChangeRule": "patch/apply text_replace oldText must be an exact non-empty literal string; newText must be an exact literal string, may be empty only to delete oldText, and identical oldText/newText is a no-op and is invalid",
		"forbiddenKinds": []string{
			"read_only_graph",
			"llm_gated_graph",
			"self_inspection_graph",
			"context_rehydration_graph",
			"same_graph_file_read_before_patch",
			"incomplete_patch_apply_input",
			"no_op_patch",
		},
		"patchApplyInputContract": map[string]any{
			"text_replace": []string{"targetRef", "oldText", "newText"},
			"file_write":   []string{"targetRef", "content"},
			"unified_diff": []string{"targetRef", "diff"},
		},
	}
	applyOutputGranularityRequiredOutputContract(out)
	return out
}

func applyOutputGranularityRequiredOutputContract(requiredOutput map[string]any) {
	if len(requiredOutput) == 0 {
		return
	}
	requiredOutput["outputGranularityPolicy"] = map[string]any{
		"preferSmallCohesiveTargets":                    true,
		"mustAvoidMonolithicGeneratedFiles":             true,
		"splitLargeDeliverablesAcrossCohesiveTargets":   true,
		"largeContentRequiresIncrementalArtifactOrPlan": true,
		"maxPatchContentBytesRef":                       "input.llmOutputPolicy.maxPatchContentBytes",
		"maxGraphBytesRef":                              "input.llmOutputPolicy.maxGraphBytes",
		"maxGraphFileWritesRef":                         "input.llmOutputPolicy.maxGraphFileWrites",
		"granularityRule":                               "keep each state-changing patch/apply target bounded and cohesive; if a requested deliverable is too large for one complete patch/apply payload, return the runtime-selected incrementalArtifact plan/chunk contract or split it across cohesive target resources instead of emitting one oversized file_write",
		"completeContentRule":                           "do not emit partial file_write content as if it were complete; partial large content must use incremental artifact continuation or an explicit follow-up contract",
	}
}

func applyCurrentSnapshotDirectPatchContract(requiredOutput map[string]any) {
	if len(requiredOutput) == 0 {
		return
	}
	requiredOutput["mustUseCurrentTargetSnapshot"] = true
	requiredOutput["mustPatchDirectlyFromProvidedSnapshot"] = true
	requiredOutput["forbiddenStalePatchReuse"] = true
	requiredOutput["sameGraphReadPatchRule"] = "when current target snapshots are provided, do not put file/read before patch/apply in the same executionGraph; patch/apply must use provided current snapshot content/hash, or return a real governance blocker if the snapshot is insufficient"
	requiredOutput["patchApplyFieldRule"] = "patch/apply input must include complete literal fields for its patchType: text_replace requires targetRef, oldText, newText; file_write requires targetRef, non-empty content; unified_diff requires targetRef and a complete diff with explicit hunk ranges"
	requiredOutput["readBeforePatchRecoveryRule"] = "when this state-changing contract already provides current target snapshots, do not output file/read-only evidence acquisition; use the provided snapshot for patch/apply, return a real governance blocker if it is insufficient, or wait for an explicit acquire_concrete_failure_evidence contract"
	requiredOutput["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(requiredOutput["forbiddenKinds"]),
		"same_graph_file_read_before_patch",
		"incomplete_patch_apply_input",
	))
}

func applyTargetSnapshotSafetyRequiredOutputContract(requiredOutput map[string]any, snapshots []map[string]any) {
	if len(requiredOutput) == 0 {
		return
	}
	if repairTargetSnapshotsHaveExactPatchSafe(snapshots) {
		applyCurrentSnapshotDirectPatchContract(requiredOutput)
		requiredOutput["fileWriteExistingTargetRule"] = "only allowed when the target snapshot is complete/safe and patch/apply file_write includes baseHash equal to that snapshot contentHash; otherwise use patch/apply text_replace with exact oldText/newText/baseHash, bounded unified_diff, or file/edit"
		return
	}
	applyTargetSnapshotAcquisitionRequiredOutputContract(requiredOutput)
	requiredOutput["targetSnapshotSafetyRule"] = "diagnostic-only, truncated, redacted, binary, incomplete, or non-runtime-managed snapshots cannot authorize exact patch/apply or file_write; acquire a runtime-managed file/read target snapshot first"
}

func markRepairInputTargetSnapshotAcquisition(input map[string]any) {
	if len(input) == 0 {
		return
	}
	contract := mapFromValue(input["convergenceContract"])
	if len(contract) == 0 {
		contract = map[string]any{}
	}
	contract["state"] = convergenceStateNeedEvidence
	contract["runtimeStage"] = runtimeStageRepair
	contract["runtimeStageMode"] = "target_snapshot_acquisition"
	contract["repairObjective"] = repairObjectiveAcquireFailureEvidence
	contract["stateChangingOutputRequired"] = false
	input["convergenceContract"] = contract
}

func applyTargetSnapshotAcquisitionRequiredOutputContract(requiredOutput map[string]any) {
	if len(requiredOutput) == 0 {
		return
	}
	delete(requiredOutput, "mustUseCurrentTargetSnapshot")
	delete(requiredOutput, "mustPatchDirectlyFromProvidedSnapshot")
	delete(requiredOutput, "sameGraphReadPatchRule")
	delete(requiredOutput, "readBeforePatchRecoveryRule")
	delete(requiredOutput, "fileWriteExistingTargetRule")
	requiredOutput["mustAcquireCurrentTargetSnapshot"] = true
	requiredOutput["mustUseReadOnlySnapshotAcquisition"] = true
	requiredOutput["mustNotMutateWorkspace"] = true
	requiredOutput["mustIncludeStateChangingNode"] = false
	requiredOutput["stateChangingForbidden"] = true
	requiredOutput["acceptedKinds"] = evidenceAcquisitionAcceptedKinds()
	requiredOutput["acceptedKindsOnly"] = true
	requiredOutput["forbiddenKinds"] = removeStringsFromSlice(
		stringSliceFromValue(requiredOutput["forbiddenKinds"]),
		"read_only_graph",
		"same_graph_file_read_before_patch",
	)
}

func stateChangingRepairAcceptedKinds() []string {
	return []string{"patch/apply", "shell/command", "mcp/tool_call"}
}

func capabilityPolicyRepairContractFromFailure(run *runModel.Run, failedStep *runModel.Step, execErr error) map[string]any {
	if run == nil || failedStep == nil {
		return nil
	}
	failureText := strings.ToLower(strings.Join([]string{
		errorString(execErr),
		stringFromMap(failedStep.Output, "error"),
		stringFromMap(failedStep.Output, "reason"),
		stringFromMap(failedStep.Output, failureOutputKeySignature),
	}, "\n"))
	if !strings.Contains(failureText, "capability usage policy") {
		return nil
	}
	policies := capabilityUsagePoliciesForStep(run, failedStep)
	if len(policies) == 0 {
		return nil
	}
	triggered := make([]map[string]any, 0, len(policies))
	for _, policy := range policies {
		policyID := strings.TrimSpace(stringFromMap(policy, "id"))
		if policyID != "" && !strings.Contains(failureText, strings.ToLower(policyID)) {
			continue
		}
		item := map[string]any{}
		for _, key := range []string{
			"id",
			"appliesToCategory",
			"usageMode",
			"triggerStages",
			"preferredTools",
			"requiredAfterKinds",
			"fallbackPolicy",
			"evidenceStrength",
		} {
			if value, ok := policy[key]; ok && value != nil {
				item[key] = compactLLMContextValue(value, 1)
			}
		}
		if len(item) > 0 {
			triggered = append(triggered, item)
		}
	}
	if len(triggered) == 0 {
		return nil
	}
	contract := map[string]any{
		"triggeredPolicies": triggered,
		"requiredOutput": map[string]any{
			"mustReturnCorrectedExecutionGraph": true,
			"mustUseDependentFollowUpOrSkip":    true,
			"acceptedKinds":                     []string{"mcp/tool_call", "patch/apply", "shell/command"},
			"skipField":                         "capabilityPolicySkips",
		},
		"repairRule":              "for each triggered policy, any graph containing requiredAfterKinds must include a dependent mcp/tool_call matching preferredTools, or declare capabilityPolicySkips with policyId/category and non-empty reason",
		"scopeRule":               "top-level capabilityPolicySkips or dependent mcp/tool_call repairs this graph contract; patch/apply nodes remain bound to their own coverageTargets and state-change target",
		"assessmentOnlyForbidden": true,
	}
	if graph, ok := runtimeGraphSpecFromStep(failedStep); ok {
		contract["rejectedExecutionGraph"] = rejectedGraphSummary(graph)
	}
	return contract
}

func evidenceAcquisitionRequiredOutput(existing map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range existing {
		switch key {
		case "mustReturnValidJSON",
			"mustNotReuseRejectedJSON",
			"forbiddenStalePatchReuse",
			"mustAddressValidationDiagnostic",
			"recommendedEvidenceTargetType",
			"mustReferenceCoverageWorkItem",
			"mustDeclareCoverageTargets",
			"coverageTargetMatchRule":
			out[key] = value
		}
	}
	out["mustIncludeStateChangingNode"] = false
	out["mustProduceConcreteFailureEvidence"] = true
	out["acceptedKinds"] = evidenceAcquisitionAcceptedKinds()
	out["acceptedKindsOnly"] = true
	out["stateChangingForbidden"] = true
	out["nestedLLMForbidden"] = true
	out["patchForbiddenUntilConcreteTargetEvidence"] = true
	out["deliverableWriteForbidden"] = true
	out["scratchOnlyForGeneratedFixtures"] = true
	out["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(existing["forbiddenKinds"]),
		"patch/apply",
		"patch/preview",
		"llm_gated_graph",
		"self_inspection_graph",
		"context_rehydration_graph",
		"run_snapshot_graph",
		"observability_only_graph",
	))
	out["evidenceAcquisitionRule"] = "bounded evidence acquisition is required because no concrete state-changing repair target evidence is available; every executionGraph node kind must be in acceptedKinds and must not include patch/apply, patch/preview, or llm/workspace_execution"
	out["deliverableWriteRule"] = "evidence acquisition shell/command or mcp/tool_call nodes must not create, modify, delete, or initialize workspace deliverable files; any workspace deliverable state change requires a state-changing repair contract and patch/apply or another explicitly accepted state-changing kind"
	out["scratchFixtureRule"] = "generated fixtures, temporary configs, probes, or controlled substitutes used only to acquire evidence must be written under validationScratchDir; workspace deliverable tree writes remain forbidden"
	return out
}

func evidenceAcquisitionRequiredOutputWithScratch(input map[string]any, pkg map[string]any, existing map[string]any) map[string]any {
	required := evidenceAcquisitionRequiredOutput(existing)
	if boolValue(required["mustProduceConcreteFailureEvidence"]) {
		ensureValidationScratchContract(input, pkg, required)
	}
	applyRepairCoverageFocusContract(input, pkg, required)
	return required
}

func evidenceAcquisitionAcceptedKinds() []string {
	return []string{
		runtimemodel.StepKindFileList,
		runtimemodel.StepKindFileRead,
		runtimemodel.StepKindShellCommand,
		runtimemodel.StepKindMCPToolCall,
		runtimemodel.StepKindGitRead,
	}
}

func concreteValidationEvidenceRequiredOutput(existing map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range existing {
		switch key {
		case "mustReturnValidJSON",
			"mustNotReuseRejectedJSON",
			"mustReferenceCoverageWorkItem",
			"mustDeclareCoverageTargets",
			"coverageTargetMatchRule",
			"mustAddressValidationDiagnostic",
			"recommendedEvidenceTargetType":
			out[key] = value
		}
	}
	out["mustIncludeStateChangingNode"] = false
	out["mustProduceConcreteValidationEvidence"] = true
	out["mustPreferExecutableBehaviorValidation"] = true
	out["staticInspectionOnlyMayNotSatisfyRuntimeBehavior"] = true
	out["mustAttemptControlledSubstituteWhenPrerequisiteUnavailable"] = true
	out["mustDeclareValidationEvidenceMode"] = true
	out["mustDeclareValidationEvidenceObservation"] = true
	out["acceptedKinds"] = []string{"shell/command", "mcp/tool_call", managedValidationSessionNodeKind}
	out["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(existing["forbiddenKinds"]),
		"patch/apply",
		runtimemodel.StepKindGitWrite,
		"read_only_graph",
		"llm_gated_graph",
		"self_inspection_graph",
		"context_rehydration_graph",
		"run_snapshot_graph",
		"observability_only_graph",
		"file_mutating_shell_validation",
	))
	out["validationEvidenceRule"] = "bounded validation evidence is required for unresolved Goal/DoD/deliverable coverage; patch/apply is only valid after separate concrete implementation target evidence appears; validation/managed_session may be used when runtime-managed timeout, scratch, cleanup, or port allocation is needed"
	out["controlledSubstituteRule"] = "when a dependency or environment prerequisite is unavailable, first try a neutral local fixture, mock/stub, temporary config, isolated working directory, or alternate available port through accepted validation tools; all controlled-substitute writes must stay under validationScratchDir; static inspection alone does not close runtime-behavior coverage"
	out["validationEvidenceModeRule"] = "each shell/command or mcp/tool_call node intended to close runtime-behavior coverage must declare input.validationEvidence.mode as runtime_behavior, controlled_substitute, executable_contract, or dynamic_execution; static_inspection/source_inspection metadata cannot close runtime-behavior coverage"
	out["validationEvidenceObservationRule"] = "each concrete validation node must also declare input.validationEvidence.observationType as runtime_observation, controlled_substitute_observation, executable_contract_observation, or dynamic_execution_observation, plus input.validationEvidence.targetRefs and input.validationEvidence.observedClaims; controlled_substitute must declare validationEvidence.substituteFor or validationEvidence.substitute"
	out["nonDestructiveBoundedShellRule"] = "shell/command validation nodes must not mutate workspace files, write outside validationScratchDir, use nohup, setsid, background '&' service launches, kill/pkill/killall/fuser -k, or approval requests for process termination; use read-only bounded foreground commands, timeout-controlled harnesses, or controlled substitutes whose writes are confined to validationScratchDir"
	out["validationEvidenceNodeExample"] = map[string]any{
		"id":   "verify_behavior",
		"kind": "shell/command",
		"input": map[string]any{
			"command":       "sh",
			"args":          []string{"-c", "printf verified"},
			"timeoutMillis": managedValidationTimeoutMillis,
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
				"targetRefs":      []string{"current_coverage_focus"},
				"observedClaims":  []string{"current coverage focus behavior was observed"},
			},
		},
	}
	if stringSliceContains(stringSliceFromValue(out["acceptedKinds"]), managedValidationSessionNodeKind) {
		out["managedValidationNodeExample"] = map[string]any{
			"id":   "verify_managed_behavior",
			"kind": managedValidationSessionNodeKind,
			"input": map[string]any{
				"allocatePort": true,
				"observation": map[string]any{
					"command":       "sh",
					"args":          []string{"-c", "printf managed-validation-ok:{{managedValidation.port}}"},
					"timeoutMillis": managedValidationTimeoutMillis,
					"validationEvidence": map[string]any{
						"mode":            "runtime_behavior",
						"observationType": "runtime_observation",
						"targetRefs":      []string{"current_coverage_focus"},
						"observedClaims":  []string{"current coverage focus behavior was observed through managed validation"},
					},
				},
			},
		}
	}
	out["destructiveValidationApprovalRule"] = "if a shell/process-termination validation command is rejected by runtime policy, do not request approval to run it; use a non-destructive controlled substitute or return a failed assessment with the missing evidence"
	out["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(out["forbiddenKinds"]),
		"destructive_shell_approval",
		"process_termination_approval",
	))
	return out
}

func applyConcreteRepairEvidenceOutputContract(input map[string]any, pkg map[string]any, requiredOutput map[string]any) {
	if len(pkg) == 0 || len(requiredOutput) == 0 {
		return
	}
	if !boolValue(requiredOutput["mustIncludeStateChangingNode"]) {
		return
	}
	if boolValue(pkg["capabilityPolicyRepairRequired"]) {
		requiredOutput["mustIncludeStateChangingNode"] = true
		requiredOutput["mustRepairCapabilityPolicyFollowUp"] = true
		requiredOutput["acceptedKinds"] = compactUniqueStrings(append(stringSliceFromValue(requiredOutput["acceptedKinds"]), "mcp/tool_call"))
		return
	}
	if stateChangingRepairHasActionableImplementationSurface(input, pkg) {
		requiredOutput["mustIncludeStateChangingNode"] = true
		acceptedKinds := compactUniqueStrings(append(stringSliceFromValue(requiredOutput["acceptedKinds"]), stateChangingRepairAcceptedKinds()...))
		requiredOutput["acceptedKinds"] = acceptedKinds
		return
	}
	next := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, requiredOutput)
	for key := range requiredOutput {
		delete(requiredOutput, key)
	}
	for key, value := range next {
		requiredOutput[key] = value
	}
	pkg["evidenceAcquisitionRequired"] = true
	pkg["evidenceAcquisitionReason"] = "state_changing_repair_target_evidence_missing"
	pkg["exitCriteria"] = []string{
		"concrete_failure_evidence_acquired_then_target_scoped_repair",
		"failed_assessment_or_blocker_when_evidence_cannot_be_acquired",
	}
}

func coverageRunFromRepairInput(input map[string]any) *runModel.Run {
	if len(input) == 0 {
		return nil
	}
	executionContract := mapFromValue(input["executionContract"])
	if len(executionContract) == 0 {
		executionContract = mapFromValue(mapFromValue(input[stepInputKeyExecutionContextPackage])["executionContract"])
	}
	if len(executionContract) == 0 {
		return nil
	}
	return &runModel.Run{ExecutionContext: map[string]any{"executionContract": executionContract}}
}

func stateChangingRepairHasConcreteTargetEvidence(input map[string]any, pkg map[string]any) bool {
	if len(pkg) > 0 && boolValue(pkg["capabilityPolicyRepairRequired"]) {
		return true
	}
	return repairHasImplementationTargetEvidence(input, pkg)
}

func stateChangingRepairHasActionableImplementationSurface(input map[string]any, pkg map[string]any) bool {
	if stateChangingRepairHasConcreteTargetEvidence(input, pkg) {
		return true
	}
	return boolValue(input["emptyWorkspaceEvidenceEscalatedToImplementation"]) && !inputHasNonEmptyWorkspaceManifest(input)
}

func repairHasImplementationTargetEvidence(input map[string]any, pkg map[string]any) bool {
	if repairContextHasCapabilityInputLocatorInvalid(input, pkg) {
		return false
	}
	requireCurrentSnapshot := boolValue(input["requiresCurrentTargetSnapshotForStateChange"]) ||
		boolValue(pkg["requiresCurrentTargetSnapshotForStateChange"])
	if len(input) > 0 {
		for _, key := range []string{"failureTargetRepairContext", "patchRepairContext", "rootCauseCandidateContext"} {
			if repairContextHasAdmissibleTargetEvidence(input[key]) {
				return true
			}
		}
		if len(repairActionTargetSnapshots(input)) > 0 {
			return true
		}
		if !requireCurrentSnapshot {
			if signals := mapFromValue(input["structuredFailureSignals"]); len(signals) > 0 && structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
				return true
			}
			if progress := mapFromValue(input["currentExecutionProgress"]); progressHasConcreteTargetEvidence(progress) {
				return true
			}
		}
	}
	if len(pkg) > 0 {
		if repairContextHasAdmissibleTargetEvidence(pkg["targetSnapshots"]) {
			return true
		}
		if !requireCurrentSnapshot {
			if len(filterAdmissibleRuntimeRepairTargetRefs(stringSliceFromValue(pkg["repairTargetSet"]))) > 0 {
				return true
			}
			if primary := mapFromValue(pkg["primaryDiagnosticRepair"]); len(primary) > 0 && structuredFailureSignalsHaveConcreteTargetEvidence(primary) {
				return true
			}
			if signals := mapFromValue(pkg["structuredFailureSignals"]); len(signals) > 0 && structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
				return true
			}
			if progress := mapFromValue(pkg["currentExecutionProgress"]); progressHasConcreteTargetEvidence(progress) {
				return true
			}
		}
	}
	return false
}

func repairContextHasAdmissibleTargetEvidence(value any) bool {
	for _, item := range mapSliceFromValue(value) {
		if runtimeRepairTargetRefAdmissible(stringFromMap(item, "targetRef")) && !boolValue(item["isBinary"]) {
			return true
		}
	}
	return false
}

func sanitizeRepairActionPackageTargetEvidence(pkg map[string]any) {
	if len(pkg) == 0 {
		return
	}
	if targets, ok := pkg["repairTargetSet"]; ok {
		if filtered := filterAdmissibleRuntimeRepairTargetRefs(stringSliceFromValue(targets)); len(filtered) > 0 {
			pkg["repairTargetSet"] = filtered
		} else {
			delete(pkg, "repairTargetSet")
		}
	}
	for _, key := range []string{
		"targetSnapshots",
		"currentTargetSnapshots",
		"failureTargetRepairContext",
		"patchRepairContext",
		"rootCauseCandidateContext",
	} {
		items := mapSliceFromValue(pkg[key])
		if len(items) == 0 {
			continue
		}
		filtered := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if runtimeRepairTargetRefAdmissible(stringFromMap(item, "targetRef")) && !boolValue(item["isBinary"]) {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) > 0 {
			pkg[key] = filtered
		} else {
			delete(pkg, key)
		}
	}
	if signals := mapFromValue(pkg["structuredFailureSignals"]); len(signals) > 0 {
		if compact := compactStructuredFailureSignalsForRepairActionPackage(signals); len(compact) > 0 {
			pkg["structuredFailureSignals"] = compact
		} else {
			delete(pkg, "structuredFailureSignals")
		}
	}
}

func enrichRepairInputWithProgressFailureSignals(input map[string]any) {
	if len(input) == 0 {
		return
	}
	progress := mapFromValue(input["currentExecutionProgress"])
	if len(progress) == 0 {
		if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
			progress = mapFromValue(pkg["currentExecutionProgress"])
		}
	}
	signals := structuredFailureSignalsFromProgressEvidence(progress)
	if len(signals) == 0 {
		return
	}
	input["structuredFailureSignals"] = mergeStructuredFailureSignals(mapFromValue(input["structuredFailureSignals"]), signals)
	if targetSet := filterAdmissibleRuntimeRepairTargetRefs(stringSliceFromValue(signals["targetRefs"])); len(targetSet) > 0 {
		if existing := stringSliceFromValue(input["repairTargetSet"]); len(existing) > 0 {
			targetSet = append(existing, targetSet...)
		}
		input["repairTargetSet"] = limitedStrings(compactUniqueStrings(targetSet), maxLLMContextSliceItems)
	}
	if strings.TrimSpace(stringFromMap(input, "failureTargetScope")) == "" {
		if targetRefs := stringSliceFromValue(signals["targetRefs"]); len(targetRefs) > 0 {
			input["failureTargetScope"] = "failureTargets=" + strings.Join(limitedStrings(targetRefs, maxLLMContextSliceItems), ",")
		}
	}
}

func progressHasConcreteTargetEvidence(progress map[string]any) bool {
	return len(progress) > 0 && structuredFailureSignalsHaveConcreteTargetEvidence(structuredFailureSignalsFromProgressEvidence(progress))
}

func structuredFailureSignalsFromProgressEvidence(progress map[string]any) map[string]any {
	if len(progress) == 0 {
		return nil
	}
	merged := map[string]any{}
	add := func(output map[string]any) {
		if len(output) == 0 {
			return
		}
		signals := structuredFailureSignalsFromFailureOutput(output, nil, "", "")
		if len(signals) == 0 {
			return
		}
		merged = mergeStructuredFailureSignals(merged, signals)
	}
	for _, key := range []string{"latestActionEvidence", "completedGraphEvidence"} {
		for _, item := range mapSliceFromValue(progress[key]) {
			output := mapFromValue(item["outputSummary"])
			if len(output) == 0 {
				output = mapFromValue(item["output"])
			}
			add(output)
			add(item)
		}
	}
	add(mapFromValue(progress["lastObservedFailure"]))
	add(map[string]any{
		"error":                   stringFromMap(progress, "failureReason"),
		failureOutputKeyClass:     stringFromMap(progress, "failureClass"),
		failureOutputKeySignature: stringFromMap(progress, failureOutputKeySignature),
	})
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func structuredFailureSignalsHaveConcreteTargetEvidence(signals map[string]any) bool {
	if len(signals) == 0 {
		return false
	}
	for _, ref := range stringSliceFromValue(signals["targetRefs"]) {
		if structuredFailureTextTargetRefAllowed(ref) {
			return true
		}
	}
	for _, ref := range mapSliceFromValue(signals["lineRefs"]) {
		if structuredFailureTextTargetRefAllowed(stringFromMap(ref, "targetRef")) {
			return true
		}
	}
	return false
}

func repairActionPackageRequiresEvidenceAcquisition(pkg map[string]any) bool {
	return len(pkg) > 0 && boolValue(pkg["evidenceAcquisitionRequired"])
}

func repeatedShapeContractEvidenceRequirement(input map[string]any, pkg map[string]any) map[string]any {
	if len(input) == 0 || len(pkg) == 0 || int64FromValue(input["targetRepairAttempts"]) < 2 {
		return nil
	}
	signals := mapFromValue(pkg["structuredFailureSignals"])
	if len(signals) == 0 {
		signals = mapFromValue(input["structuredFailureSignals"])
	}
	if !structuredFailureSignalsContainShapeConflict(signals) {
		primary := mapFromValue(pkg["primaryDiagnosticRepair"])
		if len(primary) == 0 {
			primary = mapFromValue(input["primaryDiagnosticRepair"])
		}
		if !structuredFailureSignalsContainShapeConflict(primary) {
			return nil
		}
		signals = primary
	}
	if repeatedShapeContractHasIndependentEvidence(input, pkg) {
		return nil
	}
	targets := filterAdmissibleRuntimeRepairTargetRefs(stringSliceFromValue(signals["targetRefs"]))
	for _, lineRef := range mapSliceFromValue(signals["lineRefs"]) {
		if ref := strings.TrimSpace(stringFromMap(lineRef, "targetRef")); ref != "" {
			targets = append(targets, ref)
		}
	}
	targets = filterAdmissibleRuntimeRepairTargetRefs(compactUniqueStrings(targets))
	requirement := map[string]any{
		"required":                 true,
		"reason":                   "same target repeatedly failed with resource or contract shape diagnostics",
		"acceptedKinds":            evidenceAcquisitionAcceptedKinds(),
		"patchForbiddenUntilRead":  true,
		"mustAcquireBeforePatch":   true,
		"targetRepairAttempts":     int64FromValue(input["targetRepairAttempts"]),
		"failureTargetScope":       stringFromMap(input, "failureTargetScope"),
		"requiredEvidenceExamples": []string{"current target declaration", "related contract/resource definition", "available dependency or capability schema", "bounded diagnostic command output"},
	}
	if len(targets) > 0 {
		requirement["targetRefs"] = targets
	}
	if identifiers := stringSliceFromValue(signals["identifiers"]); len(identifiers) > 0 {
		requirement["identifiers"] = limitedStrings(identifiers, maxLLMContextSliceItems)
	}
	if messages := stringSliceFromValue(signals["messages"]); len(messages) > 0 {
		requirement["messages"] = limitedStrings(messages, maxLLMContextSliceItems)
	}
	return requirement
}

func repeatedShapeContractHasIndependentEvidence(input map[string]any, pkg map[string]any) bool {
	for _, key := range []string{
		"resourceShapeAlignmentContext",
		"rootCauseCandidateContext",
		"readableResourceRepairContext",
	} {
		if len(mapSliceFromValue(input[key])) > 0 || len(mapSliceFromValue(pkg[key])) > 0 {
			return true
		}
	}
	return false
}

func repairActionPackageRequiresStateChangingOutput(pkg map[string]any) bool {
	if len(pkg) == 0 {
		return false
	}
	required := mapFromValue(pkg["requiredOutput"])
	if boolValue(required["mustIncludeStateChangingNode"]) {
		return true
	}
	return stringFromMap(required, "outputMode") == outputModeIncrementalArtifact ||
		boolValue(required["mustReturnArtifactPlan"]) ||
		boolValue(required["mustReturnArtifactChunk"]) ||
		boolValue(required["stateChangingEvidenceDeferredUntilApply"])
}

func addStrategyReassessmentRequiredOutputContract(pkg map[string]any, requiredOutput map[string]any) {
	if len(pkg) == 0 || len(requiredOutput) == 0 {
		return
	}
	if !boolValue(requiredOutput["mustIncludeStateChangingNode"]) {
		return
	}
	reassessment := mapFromValue(pkg["strategyReassessment"])
	if !boolValue(reassessment["required"]) {
		return
	}
	requiredFields := stringSliceFromValue(reassessment["requiredDecisionFields"])
	if len(requiredFields) == 0 {
		requiredFields = []string{
			"failedSurface",
			"rootCauseTarget",
			"repairTargetDecision",
			"repairMode",
			"forbiddenRepair",
			"verificationPlan",
		}
	}
	requiredOutput["mustDeclareStrategyReassessmentFields"] = true
	requiredOutput["strategyReassessmentRequiredFields"] = requiredFields
	repairModes := strategyReassessmentAllowedRepairModes(reassessment)
	if len(repairModes) == 0 {
		repairModes = []string{strategyRepairModeContinueSameTarget, strategyRepairModeShiftRootCauseTarget}
	}
	requiredOutput["strategyReassessmentRepairModes"] = repairModes
	requiredOutput["strategyReassessmentMatchRule"] = "each state-changing patch/apply node must declare these fields inside the patch/apply input object, not as top-level decision fields or node sibling fields; failedSurface must align with strategyReassessment.failedSurface, repairMode must be explicit, and rootCauseTarget must identify the resource or scope being changed"
	if strategyReassessmentRequiresRootCauseTargetShift(reassessment) {
		requiredOutput["mustReassessRepeatedFailedSurface"] = true
		requiredOutput["sameTargetContinuationRule"] = "repairMode=continue_same_target_repair may patch the failed target only when it addresses current primary diagnostics with current target evidence and bounded verification"
		requiredOutput["rootCauseTargetShiftRule"] = "repairMode=shift_root_cause_target must patch a distinct neutral grounded root-cause target or return a real governance blocker"
	}
}

func compactStructuredFailureSignalsForRepairActionPackage(signals map[string]any) map[string]any {
	if len(signals) == 0 {
		return nil
	}
	ret := compactLLMContextMap(signals, 0)
	targetRefs := stringSliceFromValue(ret["targetRefs"])
	var filtered []string
	for _, ref := range targetRefs {
		ref = normalizeStructuredFailureTargetRef(ref)
		if ref == "" || !structuredFailureTextTargetRefAllowed(ref) {
			continue
		}
		filtered = append(filtered, ref)
	}
	filtered = limitedStrings(compactUniqueStrings(filtered), maxLLMContextSliceItems)
	if len(filtered) > 0 {
		ret["targetRefs"] = filtered
	} else {
		delete(ret, "targetRefs")
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func repairActionCurrentFailure(input map[string]any, rawFailedOutput ...map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	output := firstRawFailureOutput(input, rawFailedOutput...)
	if current := mapFromValue(input["currentFailure"]); len(current) > 0 &&
		runtimeRepairWrapperFailureOutput(output) &&
		!runtimeRepairWrapperFailureOutput(current) {
		return compactLLMContextMap(current, 1)
	}
	ret := map[string]any{}
	add := func(key string, value string) {
		if value = strings.TrimSpace(value); value != "" {
			ret[key] = value
		}
	}
	add("failureClass", firstNonEmptyString(stringFromMap(input, failureOutputKeyClass), stringFromMap(output, failureOutputKeyClass)))
	add("failureSignature", firstNonEmptyString(stringFromMap(input, failureOutputKeySignature), stringFromMap(output, failureOutputKeySignature)))
	add("failedStepId", stringFromMap(input, "failedStepId"))
	add("failedStepKind", stringFromMap(input, "failedStepKind"))
	add("stage", stringFromMap(output, "stage"))
	add("targetRef", firstNonEmptyString(
		stringFromMap(output, "targetRef"),
		stringFromMap(output, "path"),
		repairActionTargetRefFromScope(stringFromMap(input, "failureTargetScope")),
	))
	add("targetScope", stringFromMap(input, "failureTargetScope"))
	add("failureTargetScope", firstNonEmptyString(stringFromMap(input, "failureTargetScope"), stringFromMap(output, "failureTargetScope"), stringFromMap(output, "targetScope")))
	add("error", firstNonEmptyString(stringFromMap(output, "error"), stringFromMap(input, "failureReason")))
	add("reason", firstNonEmptyString(stringFromMap(output, "reason"), stringFromMap(input, "failureReason")))
	if diagnostics := repairActionLastFailureDiagnostics(input, rawFailedOutput...); len(diagnostics) > 0 {
		add("stderrPreview", stringFromMap(diagnostics, "stderrPreview"))
		add("stdoutPreview", stringFromMap(diagnostics, "stdoutPreview"))
	}
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		add("diagnosticKind", stringFromMap(diagnostics, "kind"))
		add("diagnosticStage", stringFromMap(diagnostics, "stage"))
		add("diagnosticErrorDetail", stringFromMap(diagnostics, "errorDetail"))
		if _, ok := ret["targetRef"]; !ok {
			add("targetRef", stringFromMap(diagnostics, "targetRef"))
		}
	}
	if diagnostic := repairActionValidationFailureDiagnostic(input, rawFailedOutput...); len(diagnostic) > 0 {
		add("validationDiagnosticClass", stringFromMap(diagnostic, "class"))
		add("recommendedEvidenceTargetType", stringFromMap(diagnostic, "recommendedEvidenceTargetType"))
	}
	if len(ret) == 0 {
		return nil
	}
	return compactLLMContextMap(ret, 1)
}

func repairActionValidationFailureDiagnostic(input map[string]any, rawFailedOutput ...map[string]any) map[string]any {
	for _, candidate := range []map[string]any{
		mapFromValue(input["validationFailureDiagnostic"]),
		mapFromValue(mapFromValue(input["lastFailureDiagnostics"])["validationFailureDiagnostic"]),
		mapFromValue(mapFromValue(input["runtimeDiagnostics"])["validationFailureDiagnostic"]),
		mapFromValue(mapFromValue(input["failedStepOutput"])["validationFailureDiagnostic"]),
		mapFromValue(mapFromValue(mapFromValue(input["failedStepOutput"])["lastFailureDiagnostics"])["validationFailureDiagnostic"]),
	} {
		if normalized := normalizeValidationFailureDiagnostic(candidate); len(normalized) > 0 {
			return normalized
		}
	}
	output := firstRawFailureOutput(input, rawFailedOutput...)
	for _, candidate := range []map[string]any{
		mapFromValue(output["validationFailureDiagnostic"]),
		mapFromValue(mapFromValue(output["lastFailureDiagnostics"])["validationFailureDiagnostic"]),
		mapFromValue(mapFromValue(output["runtimeDiagnostics"])["validationFailureDiagnostic"]),
	} {
		if normalized := normalizeValidationFailureDiagnostic(candidate); len(normalized) > 0 {
			return normalized
		}
	}
	return nil
}

func repairActionLastFailureDiagnostics(input map[string]any, rawFailedOutput ...map[string]any) map[string]any {
	if len(input) == 0 && len(rawFailedOutput) == 0 {
		return nil
	}
	if diagnostics := mapFromValue(input["lastFailureDiagnostics"]); len(diagnostics) > 0 {
		return compactFailureStepOutputForRuntimeContext(diagnostics)
	}
	output := firstRawFailureOutput(input, rawFailedOutput...)
	if len(output) == 0 {
		return nil
	}
	if diagnostics := mapFromValue(output["lastFailureDiagnostics"]); len(diagnostics) > 0 {
		return compactFailureStepOutputForRuntimeContext(diagnostics)
	}
	compact := compactFailureStepOutputForRuntimeContext(output)
	if len(compact) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"error",
		"reason",
		"stage",
		"command",
		"args",
		"exitCode",
		failureOutputKeyClass,
		failureOutputKeySignature,
		"stdoutPreview",
		"stderrPreview",
	} {
		if value, ok := compact[key]; ok && value != nil {
			ret[key] = value
		}
	}
	if diagnostic := mapFromValue(compact["validationFailureDiagnostic"]); len(diagnostic) > 0 {
		ret["validationFailureDiagnostic"] = compactLLMContextMap(diagnostic, 1)
	}
	if len(ret) == 0 {
		return nil
	}
	return compactLLMContextMap(ret, 1)
}

func stateChangingSchemaInvalidRepair(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	return repairActionIsSchemaInvalidRepair(map[string]any{
		failureOutputKeyClass: stringFromMap(step.Output, failureOutputKeyClass),
	}, step.Output)
}

func repairActionIsSchemaInvalidRepair(input map[string]any, rawFailedOutput ...map[string]any) bool {
	output := firstRawFailureOutput(input, rawFailedOutput...)
	class := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(input, failureOutputKeyClass),
		stringFromMap(output, failureOutputKeyClass),
	))
	stage := strings.TrimSpace(stringFromMap(output, "stage"))
	if class == failureClassLLMDecisionInvalid || stage == "llm_schema_invalid" {
		return true
	}
	text := strings.ToLower(strings.Join([]string{
		stringFromMap(input, "failureReason"),
		stringFromMap(output, "error"),
		stringFromMap(output, "reason"),
	}, "\n"))
	return strings.Contains(text, "invalid llm decision json") ||
		strings.Contains(text, "invalid character") && strings.Contains(text, "json")
}

func firstRawFailureOutput(input map[string]any, rawFailedOutput ...map[string]any) map[string]any {
	if len(rawFailedOutput) > 0 && len(rawFailedOutput[0]) > 0 {
		return rawFailedOutput[0]
	}
	if failedOutput := mapFromValue(input["failedStepOutput"]); len(failedOutput) > 0 {
		return failedOutput
	}
	return nil
}

func repairActionTargetSet(input map[string]any, pkg map[string]any, rawFailedOutput ...map[string]any) []string {
	seen := map[string]bool{}
	var ret []string
	manifest := repairPatchRelevantWorkspaceManifest(input)
	add := func(value string) {
		value = normalizeStructuredFailureTargetRef(value)
		if value == "" {
			return
		}
		if canonical, ok := canonicalRepairTargetRefFromManifest(manifest, value); ok {
			value = canonical
		} else if len(manifest) > 0 {
			return
		}
		if seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	addStructuredFailureSignalTargetRef := func(value string) {
		value = normalizeStructuredFailureTargetRef(value)
		if value == "" || !structuredFailureTextTargetRefAllowed(value) {
			return
		}
		add(value)
	}
	add(repairActionTargetRefFromScope(stringFromMap(input, "failureTargetScope")))
	if currentFailure := mapFromValue(pkg["currentFailure"]); len(currentFailure) > 0 {
		add(stringFromMap(currentFailure, "targetRef"))
		add(repairActionTargetRefFromScope(stringFromMap(currentFailure, "targetScope")))
	}
	output := firstRawFailureOutput(input, rawFailedOutput...)
	add(stringFromMap(output, "targetRef"))
	add(stringFromMap(output, "path"))
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		add(stringFromMap(diagnostics, "targetRef"))
		add(stringFromMap(diagnostics, "conflictRef"))
	}
	for _, key := range []string{"failureTargetRepairContext", "readableResourceRepairContext", "patchRepairContext", "targetSnapshots", "patchRepairSuggestions"} {
		for _, item := range mapSliceFromValue(input[key]) {
			add(stringFromMap(item, "targetRef"))
		}
	}
	for _, key := range []string{"rootCauseCandidateContext"} {
		for _, item := range mapSliceFromValue(input[key]) {
			add(stringFromMap(item, "targetRef"))
		}
	}
	for _, item := range mapSliceFromValue(pkg["targetSnapshots"]) {
		add(stringFromMap(item, "targetRef"))
	}
	for _, item := range mapSliceFromValue(pkg["currentTargetSnapshots"]) {
		add(stringFromMap(item, "targetRef"))
	}
	for _, item := range mapSliceFromValue(pkg["rootCauseCandidateContext"]) {
		add(stringFromMap(item, "targetRef"))
	}
	if signals := mapFromValue(input["structuredFailureSignals"]); len(signals) > 0 {
		for _, ref := range stringSliceFromValue(signals["targetRefs"]) {
			addStructuredFailureSignalTargetRef(ref)
		}
	}
	if signals := mapFromValue(pkg["structuredFailureSignals"]); len(signals) > 0 {
		for _, ref := range stringSliceFromValue(signals["targetRefs"]) {
			addStructuredFailureSignalTargetRef(ref)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return limitedStrings(ret, maxLLMContextSliceItems)
}

func coverageTargetSetFromRepairInput(input map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]map[string]any, 0)
	workItemContext := coverageWorkItemContextFromInput(input)
	add := func(kind string, item map[string]any, targetText string) {
		targetText = strings.TrimSpace(targetText)
		if targetText == "" {
			return
		}
		token := coverageTargetSetDedupToken(item, targetText)
		if token == "" || seen[token] {
			return
		}
		seen[token] = true
		next := map[string]any{
			"kind":       kind,
			"targetText": targetText,
			"state":      firstNonEmptyString(stringFromMap(item, "state"), "failed"),
		}
		if id := strings.TrimSpace(stringFromMap(item, "goalRequirementId")); id != "" {
			next["goalRequirementId"] = id
			next["coverageTargetToken"] = id
		} else {
			next["coverageTargetToken"] = targetText
		}
		for _, key := range []string{
			"item",
			"raw",
			"requirement",
			"source",
			"sourceRequirementId",
			"sourceScope",
			"sourcePath",
			"evidenceRole",
			"requiredEvidenceKinds",
			"missingEvidenceKinds",
			"requiredEvidenceCounts",
			"acceptedEvidenceByKind",
			"rejectedEvidenceByKind",
		} {
			if value, ok := item[key]; ok && value != nil {
				next[key] = compactLLMContextValue(value, 1)
			}
		}
		if refs := evidenceRefsFromValue(item["evidenceRefs"]); len(refs) > 0 {
			next["evidenceRefs"] = refs
		}
		if candidates := runtimeRepairProgressEvidenceSummaries(mapSliceFromValue(item["evidenceCandidates"])); len(candidates) > 0 {
			next["evidenceCandidates"] = candidates
		}
		ret = append(ret, next)
	}
	for _, item := range coverageFocusCandidateItemsFromInput(input, workItemContext) {
		add(firstNonEmptyString(stringFromMap(item, "kind"), stringFromMap(item, "sourceScope"), "coverage"), item, firstNonEmptyString(
			stringFromMap(item, "targetText"),
			stringFromMap(item, "coverageTargetToken"),
			stringFromMap(item, "sourceRequirementId"),
			stringFromMap(item, "id"),
		))
	}
	for _, item := range mapSliceFromValue(input["missingDoDItems"]) {
		add("dod", item, stringFromMap(item, "item"))
	}
	for _, item := range missingGoalCoverageFromInput(input) {
		if !missingGoalCoverageItemMatchesWorkItemContext(item, workItemContext) {
			continue
		}
		add("goal", item, firstNonEmptyString(
			stringFromMap(item, "goalRequirementId"),
			missingGoalCoverageRequirementText(item),
		))
	}
	for _, item := range mapSliceFromValue(input["missingDeliverableCoverage"]) {
		add("deliverable", item, firstNonEmptyString(
			stringFromMap(item, "item"),
			stringFromMap(item, "id"),
			stringFromMap(item, "expectationId"),
			stringFromMap(item, "requirement"),
		))
	}
	for _, item := range coverageTargetSetFromWorkItemContext(workItemContext) {
		add(firstNonEmptyString(stringFromMap(item, "kind"), stringFromMap(item, "sourceScope"), "coverage"), item, firstNonEmptyString(
			stringFromMap(item, "targetText"),
			stringFromMap(item, "coverageTargetToken"),
			stringFromMap(item, "sourceRequirementId"),
		))
	}
	if len(ret) == 0 {
		return nil
	}
	if len(ret) > maxLLMContextSliceItems {
		ret = ret[:maxLLMContextSliceItems]
	}
	return ret
}

func coverageTargetSetDedupToken(item map[string]any, targetText string) string {
	return normalizeCoverageGapRepairToken(firstNonEmptyString(
		stringFromMap(item, "goalRequirementId"),
		stringFromMap(item, "sourceRequirementId"),
		stringFromMap(item, "normalizedSourceKey"),
		targetText,
		stringFromMap(item, "coverageWorkItemId"),
		stringFromMap(item, "id"),
	))
}

func coverageFocusCandidateItemsFromInput(input map[string]any, context map[string]any) []map[string]any {
	focus := mapFromValue(input[coverageFocusKey])
	if len(focus) == 0 || len(context) == 0 {
		return nil
	}
	wanted := coverageFocusMatchTokens(focus)
	if len(wanted) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0)
	seen := map[string]bool{}
	for _, item := range coverageWorkItemContextItems(context) {
		if !coverageWorkItemMatchesFocusTokens(item, wanted) {
			continue
		}
		key := firstNonEmptyString(
			stringFromMap(item, "id"),
			stringFromMap(item, "sourceRequirementId"),
			stringFromMap(item, "normalizedSourceKey"),
			stringFromMap(item, "targetText"),
		)
		key = normalizeCoverageGapRepairToken(key)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		ret = append(ret, item)
	}
	return ret
}

func coverageFocusMatchTokens(focus map[string]any) map[string]bool {
	ret := map[string]bool{}
	add := func(value string) {
		if token := normalizeCoverageGapRepairToken(value); token != "" {
			ret[token] = true
		}
	}
	for _, key := range []string{"id", "sliceId", "sourceRequirementId", "normalizedSourceKey"} {
		add(stringFromMap(focus, key))
	}
	for _, key := range []string{"coverageWorkItemIds", "targetRefs", "sourceRequirementAliases"} {
		for _, value := range stringSliceFromValue(focus[key]) {
			add(value)
		}
	}
	return ret
}

func coverageWorkItemMatchesFocusTokens(item map[string]any, wanted map[string]bool) bool {
	if len(item) == 0 || len(wanted) == 0 {
		return false
	}
	candidates := []string{
		stringFromMap(item, "id"),
		stringFromMap(item, "sourceRequirementId"),
		stringFromMap(item, "normalizedSourceKey"),
		stringFromMap(item, "sourcePath"),
		stringFromMap(item, "targetText"),
	}
	for _, value := range stringSliceFromValue(item["sourceRequirementAliases"]) {
		candidates = append(candidates, value)
	}
	for _, candidate := range candidates {
		token := normalizeCoverageGapRepairToken(candidate)
		if token == "" {
			continue
		}
		if wanted[token] {
			return true
		}
		for want := range wanted {
			if coverageRepairTokenMatches(token, want) {
				return true
			}
		}
	}
	return false
}

func missingGoalCoverageItemMatchesWorkItemContext(item map[string]any, context map[string]any) bool {
	if len(item) == 0 || len(context) == 0 {
		return true
	}
	items := coverageWorkItemCandidateItemsFromContext(context)
	hasGoalItems := false
	tokens := coverageTokensFromAssessmentItem(item)
	for _, candidate := range items {
		if strings.TrimSpace(stringFromMap(candidate, "sourceScope")) != "goal_leaf" &&
			strings.TrimSpace(stringFromMap(candidate, "kind")) != "goal" {
			continue
		}
		hasGoalItems = true
		update := coverageWorkItemUpdate{TargetTokens: tokens}
		if coverageWorkItemMatchesUpdate(candidate, update) {
			return true
		}
	}
	return !hasGoalItems
}

func coverageWorkItemCandidateItemsFromContext(context map[string]any) []map[string]any {
	if len(context) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0)
	seen := map[string]bool{}
	add := func(item map[string]any) {
		if len(item) == 0 {
			return
		}
		key := firstNonEmptyString(
			stringFromMap(item, "id"),
			stringFromMap(item, "sourceRequirementId"),
			stringFromMap(item, "normalizedSourceKey"),
			stringFromMap(item, "targetText"),
		)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		ret = append(ret, item)
	}
	for _, key := range []string{"currentFocus", "currentFocusItems", "unresolvedItems", "ledgerSummary"} {
		add(mapFromValue(context[key]))
		for _, item := range mapSliceFromValue(context[key]) {
			add(item)
		}
	}
	return ret
}

func ensureCoverageRepairTargetSet(input map[string]any) {
	if len(input) == 0 {
		return
	}
	targets := coverageTargetSetFromRepairInput(input)
	if len(targets) == 0 {
		return
	}
	input["coverageRepairRequired"] = true
	input["coverageTargetSet"] = targets
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		pkg["coverageRepairRequired"] = true
		pkg["coverageTargetSet"] = targets
		required := mapFromValue(pkg["requiredOutput"])
		if len(required) == 0 {
			required = map[string]any{}
		}
		required["mustDeclareCoverageTargets"] = true
		if strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveConcreteValidationEvidence ||
			boolValue(input["boundedValidationRequired"]) ||
			boolValue(required["mustProduceConcreteValidationEvidence"]) {
			required["coverageTargetMatchRule"] = "validation evidence must map to one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
		} else {
			required["coverageTargetMatchRule"] = "patch/apply coverageTargets must match one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
		}
		applyRepairCoverageFocusContract(input, pkg, required)
		pkg["requiredOutput"] = required
		input["repairActionPackage"] = pkg
	}
}

func repairActionTargetRefFromScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return ""
	}
	_, value, ok := strings.Cut(scope, "=")
	if !ok {
		return scope
	}
	return strings.TrimSpace(value)
}

func failedPatchDigestFromRepairFailure(input map[string]any, rawFailedOutput ...map[string]any) map[string]any {
	output := firstRawFailureOutput(input, rawFailedOutput...)
	text := strings.ToLower(strings.Join(repairForbiddenReuseTextValues(input, output), "\n"))
	class := strings.TrimSpace(firstNonEmptyString(stringFromMap(input, failureOutputKeyClass), stringFromMap(output, failureOutputKeyClass)))
	if class != failureClassPatchApply &&
		!strings.Contains(text, "patch") &&
		!strings.Contains(text, "hunk") &&
		!strings.Contains(text, "oldtext") &&
		!strings.Contains(text, "context mismatch") {
		return nil
	}
	ret := map[string]any{}
	add := func(key string, value string) {
		if value = strings.TrimSpace(value); value != "" {
			ret[key] = value
		}
	}
	add("patchId", firstNonEmptyString(stringFromMap(output, "patchId"), stringFromMap(input, "patchId")))
	add("targetRef", firstNonEmptyString(
		stringFromMap(output, "targetRef"),
		stringFromMap(input, "targetRef"),
		repairActionTargetRefFromScope(stringFromMap(input, "failureTargetScope")),
	))
	add("patchType", firstNonEmptyString(stringFromMap(output, "patchType"), stringFromMap(input, "patchType")))
	add("failureReason", firstNonEmptyString(stringFromMap(output, "reason"), stringFromMap(output, "error"), stringFromMap(input, "failureReason")))
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		add("failureDetail", firstNonEmptyString(stringFromMap(diagnostics, "errorDetail"), stringFromMap(diagnostics, "guidance")))
		if stringFromMap(ret, "targetRef") == "" {
			add("targetRef", stringFromMap(diagnostics, "targetRef"))
		}
		if stringFromMap(ret, "patchType") == "" {
			add("patchType", stringFromMap(diagnostics, "patchType"))
		}
	}
	if repairActionFailureIndicatesStalePatch(input, output) {
		ret["stalePatchRejected"] = true
	}
	if len(ret) == 0 {
		return nil
	}
	return compactLLMContextMap(ret, 1)
}

func repairActionForbiddenActions(input map[string]any, pkg map[string]any) []string {
	ret := []string{"read_only_graph_for_state_changing_repair"}
	if len(stringSliceFromValue(pkg["repairTargetSet"])) > 0 {
		ret = append(ret, "modify_target_outside_repair_target_set")
	}
	forbiddenReuse := stringSliceFromValue(pkg["forbiddenContentReuse"])
	if repairActionFailureIndicatesStalePatch(input, firstRawFailureOutput(input)) ||
		stringSliceContains(forbiddenReuse, "stale_patch_text") ||
		stringSliceContains(forbiddenReuse, "missing_old_text") ||
		stringSliceContains(forbiddenReuse, "malformed_diff_text") {
		ret = append(ret, "reuse_stale_patch_hunk")
	}
	if len(mapSliceFromValue(pkg["currentTargetAlreadySatisfiesFailedPatch"])) > 0 {
		ret = append(ret, "repeat_same_failed_patch")
	}
	if repairActionFailureIndicatesNoOpPatch(input, firstRawFailureOutput(input)) {
		ret = append(ret, "repeat_no_op_text_replace")
	}
	return limitedStrings(compactUniqueStrings(ret), 8)
}

func repairActionRequiredRepairMode(input map[string]any, pkg map[string]any, rawFailedOutput ...map[string]any) string {
	output := firstRawFailureOutput(input, rawFailedOutput...)
	if !repairActionFailureIndicatesStalePatch(input, output) &&
		!stringSliceContains(stringSliceFromValue(pkg["forbiddenContentReuse"]), "stale_patch_text") &&
		!stringSliceContains(stringSliceFromValue(pkg["forbiddenContentReuse"]), "missing_old_text") &&
		!stringSliceContains(stringSliceFromValue(pkg["forbiddenContentReuse"]), "malformed_diff_text") {
		return ""
	}
	if int64FromValue(input["targetRepairAttempts"]) >= 2 {
		return "exact_replacement_against_current_snapshot"
	}
	return "patch_current_snapshot_only"
}

func repairActionFailureIndicatesStalePatch(input map[string]any, output map[string]any) bool {
	text := strings.ToLower(strings.Join(repairForbiddenReuseTextValues(input, output), "\n"))
	return strings.Contains(text, "context mismatch") ||
		strings.Contains(text, "patch does not apply") ||
		strings.Contains(text, "basehash mismatch") ||
		strings.Contains(text, "stale") ||
		strings.Contains(text, "oldtext not found") ||
		strings.Contains(text, "old text not found") ||
		strings.Contains(text, "hunk header") ||
		strings.Contains(text, "malformed unified") ||
		strings.Contains(text, "invalid unified")
}

func repairActionFailureIndicatesNoOpPatch(input map[string]any, output map[string]any) bool {
	text := strings.ToLower(strings.Join(repairForbiddenReuseTextValues(input, output), "\n"))
	indicatesNoOp := strings.Contains(text, "oldtext/newtext must describe") ||
		strings.Contains(text, "describe a substantive change") ||
		strings.Contains(text, "non-substantive") ||
		strings.Contains(text, "no-op")
	return indicatesNoOp &&
		(strings.TrimSpace(stringFromMap(output, outputKeyContractViolationType)) == contractViolationPatchContentRequired ||
			strings.Contains(text, "patch text_replace oldtext/newtext must describe") ||
			strings.Contains(text, "oldtext/newtext must describe a substantive change"))
}

func repairActionOutputDecisionContract(input map[string]any, pkg map[string]any, requiredOutput map[string]any, rawFailedOutput ...map[string]any) map[string]any {
	if len(pkg) == 0 || len(requiredOutput) == 0 {
		return nil
	}
	mode := repairActionOutputContractMode(input, pkg, requiredOutput)
	if mode == "" {
		return nil
	}
	contract := map[string]any{
		"requiredMode":   mode,
		"allowedOutputs": repairActionOutputContractAllowedOutputs(mode, requiredOutput),
		"forbiddenOutputs": repairActionOutputContractForbiddenOutputs(input, pkg, requiredOutput,
			firstRawFailureOutput(input, rawFailedOutput...)),
		"fallbackWhenNotPossible": repairActionOutputContractFallback(mode),
	}
	if boolValue(requiredOutput["mustUseCurrentTargetSnapshot"]) || len(mapSliceFromValue(pkg["targetSnapshots"])) > 0 {
		contract["evidenceRule"] = "use provided current target snapshots directly; do not add same-turn read/list steps before a required state-changing patch"
	}
	if len(mapFromValue(pkg["lastRejectedOutputContract"])) > 0 {
		contract["lastRejectedOutputContract"] = pkg["lastRejectedOutputContract"]
	}
	return contract
}

func repairActionOutputContractMode(input map[string]any, pkg map[string]any, requiredOutput map[string]any) string {
	switch {
	case boolValue(requiredOutput["mustProduceConcreteValidationEvidence"]):
		return "produce_concrete_validation_evidence"
	case boolValue(pkg["evidenceAcquisitionRequired"]) || stringFromMap(input, "repairObjective") == repairObjectiveAcquireFailureEvidence:
		return "acquire_concrete_failure_evidence"
	case repairActionRequiresPatchApplyOutput(pkg, requiredOutput):
		return "state_changing_patch_apply"
	case boolValue(requiredOutput["mustIncludeStateChangingNode"]):
		return "state_changing_output"
	default:
		return ""
	}
}

func onlyAcceptedRepairKind(requiredOutput map[string]any, kind string) bool {
	kinds := stringSliceFromValue(requiredOutput["acceptedKinds"])
	return len(kinds) == 1 && kinds[0] == kind
}

func repairActionRequiresPatchApplyOutput(pkg map[string]any, requiredOutput map[string]any) bool {
	if boolValue(requiredOutput["mustReturnPatchApplyNode"]) || onlyAcceptedRepairKind(requiredOutput, "patch/apply") {
		return true
	}
	if primary := mapFromValue(pkg["primaryDiagnosticRepair"]); boolValue(primary["required"]) && len(mapSliceFromValue(pkg["targetSnapshots"])) > 0 {
		return true
	}
	return false
}

func repairActionOutputContractAllowedOutputs(mode string, requiredOutput map[string]any) []string {
	switch mode {
	case "produce_concrete_validation_evidence":
		return []string{"bounded_validation_evidence_graph", "failed_dodAssessment", "real_governance_blocker"}
	case "acquire_concrete_failure_evidence":
		return []string{"bounded_evidence_acquisition_graph", "failed_dodAssessment", "real_governance_blocker"}
	case "state_changing_patch_apply":
		return []string{"patch/apply_text_replace_substantive", "patch/apply_unified_diff_complete", "patch/apply_file_write_complete_when_safe", "failed_dodAssessment", "real_governance_blocker"}
	case "state_changing_output":
		kinds := stringSliceFromValue(requiredOutput["acceptedKinds"])
		if len(kinds) == 0 {
			kinds = stateChangingRepairAcceptedKinds()
		}
		ret := make([]string, 0, len(kinds)+2)
		for _, kind := range kinds {
			ret = append(ret, "accepted_state_change:"+kind)
		}
		ret = append(ret, "failed_dodAssessment", "real_governance_blocker")
		return compactUniqueStrings(ret)
	default:
		return nil
	}
}

func repairActionOutputContractForbiddenOutputs(input map[string]any, pkg map[string]any, requiredOutput map[string]any, output map[string]any) []string {
	ret := []string{}
	ret = append(ret, stringSliceFromValue(requiredOutput["forbiddenKinds"])...)
	ret = append(ret, stringSliceFromValue(pkg["forbiddenActions"])...)
	if repairActionRequiresPatchApplyOutput(pkg, requiredOutput) {
		ret = append(ret,
			"read_only_or_inspection_only_graph",
			"shell_only_validation_for_state_change",
			"mcp_only_validation_for_state_change",
		)
	}
	if repairActionFailureIndicatesNoOpPatch(input, output) || boolValue(requiredOutput["mustAvoidNoOpPatch"]) {
		ret = append(ret, "no_op_text_replace", "identical_oldText_newText", "repeat_same_non_substantive_patch")
	}
	if boolValue(requiredOutput["mustUseCurrentTargetSnapshot"]) || len(mapSliceFromValue(pkg["targetSnapshots"])) > 0 {
		ret = append(ret, "same_turn_read_before_patch_for_known_target")
	}
	return limitedStrings(compactUniqueStrings(ret), 16)
}

func repairActionOutputContractFallback(mode string) string {
	switch mode {
	case "state_changing_patch_apply", "state_changing_output":
		return "failed_dodAssessment_or_real_governance_blocker; do not substitute read-only inspection for required state change"
	case "produce_concrete_validation_evidence", "acquire_concrete_failure_evidence":
		return "failed_dodAssessment_or_real_governance_blocker when bounded evidence cannot be produced"
	default:
		return "failed_dodAssessment_or_real_governance_blocker"
	}
}

func repairActionBudgetState(input map[string]any, pkg map[string]any) map[string]any {
	ret := map[string]any{}
	if attempt := int64FromValue(input[stepInputKeyRuntimeRepairAttempt]); attempt > 0 {
		ret["attempt"] = attempt
	}
	if maxAttempts := int64FromValue(input["maxRuntimeRepairAttempts"]); maxAttempts > 0 {
		ret["maxAttempts"] = maxAttempts
	}
	if targetAttempts := int64FromValue(input["targetRepairAttempts"]); targetAttempts > 0 {
		ret["targetRepairAttempts"] = targetAttempts
	}
	class := strings.TrimSpace(firstNonEmptyString(stringFromMap(input, failureOutputKeyClass), stringFromMap(pkg, failureOutputKeyClass)))
	switch class {
	case failureClassProviderTimeout, failureClassProviderUnavailable, failureClassContextTooLarge, failureClassRateLimited, failureClassProviderStreamIncomplete, failureClassProviderProtocolInvalid:
		ret["budgetKind"] = "provider_retry"
	case failureClassLLMDecisionInvalid:
		ret["budgetKind"] = "llm_schema_repair"
	case failureClassPatchApply:
		ret["budgetKind"] = "patch_apply_repair"
	case failureClassShellCommand:
		ret["budgetKind"] = "verification_repair"
	case failureClassRuntime:
		ret["budgetKind"] = "state_changing_contract_repair"
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func compactStateChangingRepairInputAroundActionPackage(input map[string]any) {
	if len(input) == 0 || len(mapFromValue(input["repairActionPackage"])) == 0 {
		return
	}
	omitted := []string{}
	omit := func(key string) {
		if _, ok := input[key]; !ok {
			return
		}
		delete(input, key)
		omitted = append(omitted, key)
	}
	for _, key := range []string{
		"failedStepInput",
		"failedStepOutput",
		"previousStepOutput",
		"failedExecutionGraph",
		stepInputKeyGraphEvidence,
		"runEvidenceSummary",
		stepOutputKeyExecutionGraph,
	} {
		omit(key)
	}
	if len(omitted) > 0 {
		input["repairInputCompaction"] = map[string]any{
			"mode":                  "repair_action_package_primary",
			"omittedFields":         omitted,
			"primaryField":          "repairActionPackage",
			"reason":                "repair uses bounded current failure package instead of duplicated historical payloads",
			"targetScopedByteLimit": 120 * 1024,
		}
	}
}

func repairActionTargetRelevanceDiagnostics(input map[string]any, pkg map[string]any) map[string]any {
	if len(input) == 0 && len(pkg) == 0 {
		return nil
	}
	ret := map[string]any{}
	if scope := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(input, "failureTargetScope"),
		stringFromMap(pkg, "failureTargetScope"),
	)); scope != "" {
		ret["failureTargetScope"] = scope
	}
	if targets := mapSliceFromValue(pkg["targetSnapshots"]); len(targets) > 0 {
		ret["targetSnapshotCount"] = len(targets)
		ret["targetRefs"] = repairActionTargetRefs(targets)
	}
	if manifest := mapSliceFromValue(pkg["workspaceFileManifest"]); len(manifest) > 0 {
		ret["workspaceManifestEntryCount"] = len(manifest)
	}
	if signals := mapFromValue(pkg["structuredFailureSignals"]); len(signals) > 0 {
		if refs := stringSliceFromValue(signals["targetRefs"]); len(refs) > 0 {
			ret["failureSignalTargetRefs"] = limitedStrings(refs, maxLLMContextSliceItems)
		}
		if ids := stringSliceFromValue(signals["identifiers"]); len(ids) > 0 {
			ret["failureSignalIdentifierCount"] = len(ids)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func repairActionTargetRefs(targets []map[string]any) []string {
	if len(targets) == 0 {
		return nil
	}
	refs := make([]string, 0, len(targets))
	for _, target := range targets {
		if ref := stringFromMap(target, "targetRef"); ref != "" {
			refs = append(refs, ref)
		}
	}
	return limitedStrings(compactUniqueStrings(refs), maxLLMContextSliceItems)
}
