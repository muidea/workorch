package orchestrator

import (
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func nextStepSpec(output map[string]any) (common.StepID, string) {
	if output == nil {
		return "", ""
	}
	nextStep, _ := output[stepOutputKeyNextStep].(string)
	nextKind, _ := output[stepOutputKeyNextKind].(string)
	return common.StepID(nextStep), nextKind
}

func initialStepKind(run *runModel.Run) string {
	if run != nil && strings.TrimSpace(run.InitialStepKind) != "" {
		return canonicalLLMWorkspaceStepKind(run.InitialStepKind)
	}
	return defaultInitialStepKind
}

func initialStepInput(run *runModel.Run) map[string]any {
	newCoverageSubsystem().ensure(run)
	input := nextStepInput(run, "")
	if input == nil {
		return nil
	}
	if initialStepKind(run) == llmWorkspaceExecutionStepKind {
		input[stepInputKeySoulStage] = soulStagePlanning
		setRuntimeStage(input, runtimeStagePlanning)
		if replanRunRequiresStateChangingOutput(run) {
			input[stepInputKeySoulStage] = soulStageExecution
			setRuntimeStage(input, runtimeStageRepair)
			input["replanStateChangingContinuation"] = true
			actionPackage := repairActionPackageFromReplanContinuation(run)
			if repairActionPackageRequiresStateChangingOutput(actionPackage) {
				input["stateChangingOutputRequired"] = true
				input["repairObjective"] = repairObjectiveStateChangingOutput
				input["repairGuidance"] = stateChangingRepairGuidance()
			} else {
				input["repairObjective"] = repairObjectiveAcquireFailureEvidence
				input["concreteFailureEvidenceRequired"] = true
				input["repairGuidance"] = concreteFailureEvidenceAcquisitionGuidance()
			}
			if reassessment := strategyReassessmentFromReplanContinuation(run); len(reassessment) > 0 {
				input["strategyReassessment"] = reassessment
				input["targetRepairAttempts"] = int64FromValue(reassessment["targetRepairAttempts"])
				input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
					"Replan continuation carries a repeated failed surface: choose repairMode as continue_same_target_repair or shift_root_cause_target, or return a real governance blocker.",
					"If continuing the same target, patch only when current target evidence addresses the current primary diagnostics and keep verification bounded to the failed surface.",
					"If shifting root-cause target, use a neutral grounded target and declare relatedFailureTargets for the failed surface being unblocked.",
				)
				if len(actionPackage) > 0 {
					actionPackage["strategyReassessment"] = reassessment
					actionPackage["repeatedTargetRepairMode"] = map[string]any{
						"enabled":                           true,
						"reason":                            stringFromMap(reassessment, "reason"),
						"mustReassessRepeatedFailedSurface": true,
						"mustDeclareRequiredDecisionFields": true,
					}
					if required := mapFromValue(actionPackage["requiredOutput"]); len(required) > 0 {
						required["mustRebuildFromCurrentEvidence"] = true
						required["mustUseCurrentTargetSnapshot"] = true
						actionPackage["requiredOutput"] = required
					}
				}
			}
			input["repairActionPackage"] = actionPackage
			if currentFailure := mapFromValue(actionPackage["currentFailure"]); len(currentFailure) > 0 {
				input["currentFailure"] = currentFailure
				for _, key := range []string{failureOutputKeyClass, failureOutputKeySignature, "failureTargetScope", "failedStepId", "failedStepKind"} {
					if value := actionPackage[key]; value != nil {
						input[key] = cloneStepInputValue(value)
					}
				}
			}
		} else {
			input = applyFirstRunGapBaselineStateChangingContract(input)
		}
	}
	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": input}, nil)
	input = ApplyConvergenceContract(input, contract)
	return input
}

func applyFirstRunGapBaselineStateChangingContract(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	if boolValue(input["stateChangingOutputRequired"]) || len(mapFromValue(input["repairActionPackage"])) > 0 {
		return input
	}
	baseline := mapFromValue(input[gapBaselineKey])
	if len(baseline) == 0 || strings.TrimSpace(stringFromMap(mapFromValue(baseline["nextAction"]), "kind")) != "schedule_step" {
		return input
	}
	context := coverageWorkItemContextFromInput(input)
	if len(context) == 0 || !coverageWorkItemContextHasMissingEvidenceKind(context, "state_change") {
		return input
	}
	targets := coverageTargetSetFromWorkItemContext(context)
	if len(targets) == 0 {
		return input
	}
	setRuntimeStage(input, runtimeStageRepair)
	input[stepInputKeySoulStage] = soulStageExecution
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["stateChangingOutputRequired"] = true
	input["firstRunGapStateChangingContract"] = true
	input["emptyWorkspaceEvidenceEscalatedToImplementation"] = true
	pkg := map[string]any{
		coverageWorkItemContextKey: context,
		"coverageRepairRequired":   true,
		"coverageTargetSet":        targets,
		"requiredOutput":           stateChangingRepairRequiredOutput(),
	}
	required := mapFromValue(pkg["requiredOutput"])
	required["mustDeclareCoverageTargets"] = true
	required["mustReferenceCoverageWorkItem"] = true
	required["coverageTargetMatchRule"] = "patch/apply coverageTargets must match one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
	pkg["requiredOutput"] = required
	input["repairActionPackage"] = pkg
	input["coverageRepairRequired"] = true
	input["coverageTargetSet"] = targets
	return input
}

func replanRunRequiresStateChangingOutput(run *runModel.Run) bool {
	if run == nil || !runExecutionContractRequiresStateChange(run) {
		return false
	}
	if len(mapFromValue(run.ExecutionContext["rejectedResultContinuation"])) > 0 {
		continuation := mapFromValue(run.ExecutionContext["replanContinuation"])
		return len(primaryActionableFailureFromReplanContinuation(continuation)) > 0
	}
	return len(mapFromValue(run.ExecutionContext["replanContinuation"])) > 0
}

func repairActionPackageFromReplanContinuation(run *runModel.Run) map[string]any {
	if run == nil {
		return nil
	}
	continuation, failureContext := replanContinuationFailureContext(run)
	if len(continuation) == 0 {
		return nil
	}
	pkg := map[string]any{
		"requiredOutput": map[string]any{
			"mustIncludeStateChangingNode": true,
			"acceptedKinds":                stateChangingRepairAcceptedKinds(),
			"forbiddenKinds": []string{
				"read_only_graph",
				"llm_gated_graph",
				"self_inspection_graph",
				"context_rehydration_graph",
			},
		},
		"predecessorRunId": stringFromMap(continuation, "predecessorRunId"),
		"reason":           stringFromMap(continuation, "reason"),
		"requiresCurrentTargetSnapshotForStateChange": true,
	}
	if continuationPkg := mapFromValue(continuation["repairActionPackage"]); len(continuationPkg) > 0 {
		for key, value := range continuationPkg {
			if value != nil {
				pkg[key] = value
			}
		}
	}
	primaryFailure := primaryActionableFailureFromReplanContinuation(continuation)
	if len(primaryFailure) > 0 {
		pkg["currentFailure"] = primaryFailure
		if value := stringFromMap(primaryFailure, "failureClass"); value != "" {
			pkg[failureOutputKeyClass] = value
		}
		if value := stringFromMap(primaryFailure, "failureSignature"); value != "" {
			pkg[failureOutputKeySignature] = value
		}
		if value := firstNonEmptyString(stringFromMap(primaryFailure, "failureTargetScope"), stringFromMap(primaryFailure, "targetScope")); value != "" {
			pkg["failureTargetScope"] = value
		}
		if value := stringFromMap(primaryFailure, "failedStepId"); value != "" {
			pkg["failedStepId"] = value
		}
		if value := stringFromMap(primaryFailure, "failedStepKind"); value != "" {
			pkg["failedStepKind"] = value
		}
		if signals := structuredFailureSignalsFromPrimaryActionableFailure(primaryFailure); len(signals) > 0 {
			pkg["structuredFailureSignals"] = signals
			pkg["primaryDiagnosticRepair"] = signals
		}
	}
	for _, key := range []string{
		failureOutputKeyClass,
		failureOutputKeySignature,
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"attempts",
		"maxAttempts",
		"repairOfRootStepId",
	} {
		if len(primaryFailure) > 0 {
			switch key {
			case failureOutputKeyClass, failureOutputKeySignature, "failureTargetScope", "failedStepId", "failedStepKind":
				if pkg[key] != nil {
					continue
				}
			}
		}
		if value, ok := failureContext[key]; ok && value != nil {
			pkg[key] = compactLLMContextValue(value, 1)
		}
	}
	if len(mapFromValue(failureContext["failedStepOutput"])) > 0 {
		pkg["previousRejectedOutputSummary"] = compactStateChangingRecoveryFailureOutput(mapFromValue(failureContext["failedStepOutput"]))
	}
	targetInput := cloneStepPayload(failureContext)
	if targetInput == nil {
		targetInput = map[string]any{}
	}
	if len(primaryFailure) > 0 {
		for _, key := range []string{
			failureOutputKeyClass,
			failureOutputKeySignature,
			"failureTargetScope",
			"failedStepId",
			"failedStepKind",
			"structuredFailureSignals",
		} {
			if value := pkg[key]; value != nil {
				targetInput[key] = cloneStepInputValue(value)
			}
		}
	}
	if targets := repairActionTargetSnapshots(targetInput); len(targets) > 0 {
		pkg["targetSnapshots"] = targets
	}
	if targetSet := repairActionTargetSet(targetInput, pkg); len(targetSet) > 0 {
		pkg["repairTargetSet"] = targetSet
		if required := mapFromValue(pkg["requiredOutput"]); len(required) > 0 {
			required["mustTargetRepairTargetSet"] = true
			pkg["requiredOutput"] = required
		}
	}
	required := mapFromValue(pkg["requiredOutput"])
	applyConcreteRepairEvidenceOutputContract(targetInput, pkg, required)
	pkg["requiredOutput"] = required
	if len(pkg) <= 3 {
		return nil
	}
	return pkg
}

func primaryActionableFailureFromReplanContinuation(continuation map[string]any) map[string]any {
	if len(continuation) == 0 {
		return nil
	}
	if primary := primaryActionableFailureFromProgressDigest(mapFromValue(continuation[stepInputKeyCurrentProgressDigest])); len(primary) > 0 {
		return primary
	}
	timeline := mapSliceFromValue(continuation[stepInputKeyProcessEvidenceTimeline])
	for idx := len(timeline) - 1; idx >= 0; idx-- {
		item := timeline[idx]
		if len(item) == 0 {
			continue
		}
		failure := map[string]any{
			"stepId":                    stringFromMap(item, "stepId"),
			"kind":                      stringFromMap(item, "kind"),
			"failureClass":              stringFromMap(item, failureOutputKeyClass),
			"failureSignature":          stringFromMap(item, failureOutputKeySignature),
			"failureReason":             stringFromMap(item, "failureReason"),
			"concreteFailureDiagnostic": stringFromMap(item, "concreteFailureDiagnostic"),
			"evidenceValidity":          stringFromMap(item, "evidenceValidity"),
			"targetRef":                 stringFromMap(item, "targetRef"),
			"targetScope":               stringFromMap(item, "targetScope"),
			"failureTargetScope":        stringFromMap(item, "failureTargetScope"),
			"source":                    stringFromMap(item, "source"),
		}
		if strings.TrimSpace(stringFromMap(failure, "failureClass")) == "" && strings.TrimSpace(stringFromMap(item, "kind")) == "shell/command" {
			failure["failureClass"] = failureClassShellCommand
		}
		digest := map[string]any{
			"currentGap":          firstNonEmptyString(stringFromMap(failure, "concreteFailureDiagnostic"), stringFromMap(failure, "failureReason")),
			"lastObservedFailure": failure,
		}
		if primary := primaryActionableFailureFromProgressDigest(digest); len(primary) > 0 {
			return primary
		}
	}
	return nil
}

func primaryActionableFailureFromProgressDigest(digest map[string]any) map[string]any {
	if len(digest) == 0 {
		return nil
	}
	failure := mapFromValue(digest["lastObservedFailure"])
	if len(failure) == 0 || runtimeRepairWrapperFailureOutput(failure) {
		return nil
	}
	validity := strings.TrimSpace(stringFromMap(failure, "evidenceValidity"))
	class := strings.TrimSpace(stringFromMap(failure, "failureClass"))
	if validity != "actionable_failure" && class != failureClassShellCommand && class != failureClassPatchApply {
		return nil
	}
	ret := currentFailureFromProgressDigest(digest)
	if len(ret) == 0 || runtimeRepairWrapperFailureOutput(ret) {
		return nil
	}
	signature := strings.TrimSpace(stringFromMap(ret, "failureSignature"))
	scope := firstNonEmptyString(stringFromMap(ret, "failureTargetScope"), stringFromMap(ret, "targetScope"))
	if scope == "" {
		if refs := repairPatchTargetRefsFromScopeText(signature); len(refs) > 0 {
			scope = "failureTargets=" + strings.Join(limitedStrings(refs, maxLLMContextSliceItems), ",")
		}
	}
	if scope != "" {
		ret["failureTargetScope"] = scope
		ret["targetScope"] = scope
	}
	if targetRef := strings.TrimSpace(stringFromMap(ret, "targetRef")); targetRef == "" {
		if refs := repairPatchTargetRefsFromScopeText(scope); len(refs) > 0 {
			ret["targetRef"] = strings.Join(limitedStrings(refs, maxLLMContextSliceItems), ",")
		}
	}
	return compactLLMContextMap(ret, 1)
}

func currentProgressDigestFromPrimaryActionableFailure(failure map[string]any) map[string]any {
	if len(failure) == 0 || runtimeRepairWrapperFailureOutput(failure) {
		return nil
	}
	reason := firstNonEmptyString(
		stringFromMap(failure, "reason"),
		stringFromMap(failure, "failureReason"),
		stringFromMap(failure, "concreteFailureDiagnostic"),
	)
	if strings.TrimSpace(reason) == "" {
		return nil
	}
	lastFailure := map[string]any{}
	add := func(key string, value string) {
		if value = strings.TrimSpace(value); value != "" {
			lastFailure[key] = value
		}
	}
	add("stepId", firstNonEmptyString(stringFromMap(failure, "stepId"), stringFromMap(failure, "failedStepId")))
	add("kind", firstNonEmptyString(stringFromMap(failure, "kind"), stringFromMap(failure, "failedStepKind")))
	add(failureOutputKeyClass, stringFromMap(failure, failureOutputKeyClass))
	add(failureOutputKeySignature, stringFromMap(failure, failureOutputKeySignature))
	add("failureReason", reason)
	add("concreteFailureDiagnostic", reason)
	add("evidenceValidity", firstNonEmptyString(stringFromMap(failure, "evidenceValidity"), "actionable_failure"))
	add("targetRef", stringFromMap(failure, "targetRef"))
	add("targetScope", firstNonEmptyString(stringFromMap(failure, "targetScope"), stringFromMap(failure, "failureTargetScope")))
	add("failureTargetScope", firstNonEmptyString(stringFromMap(failure, "failureTargetScope"), stringFromMap(failure, "targetScope")))
	add("source", firstNonEmptyString(stringFromMap(failure, "source"), "replanContinuation"))
	if len(lastFailure) == 0 {
		return nil
	}
	return map[string]any{
		"currentGap":          truncateLLMContextString(reason),
		"lastObservedFailure": compactLLMContextMap(lastFailure, 1),
		"nextRequiredAction":  "address_last_failure_under_current_stage_contract",
		"source":              "replanContinuation",
	}
}

func structuredFailureSignalsFromPrimaryActionableFailure(failure map[string]any) map[string]any {
	if len(failure) == 0 {
		return nil
	}
	reason := firstNonEmptyString(
		stringFromMap(failure, "reason"),
		stringFromMap(failure, "error"),
		stringFromMap(failure, "failureReason"),
		stringFromMap(failure, "concreteFailureDiagnostic"),
	)
	output := map[string]any{
		"stderr":                  reason,
		"reason":                  reason,
		failureOutputKeyClass:     stringFromMap(failure, "failureClass"),
		failureOutputKeySignature: stringFromMap(failure, "failureSignature"),
	}
	return structuredFailureSignalsFromFailureOutput(output, nil, stringFromMap(failure, "failureClass"), stringFromMap(failure, "failureSignature"))
}

func replanContinuationFailureContext(run *runModel.Run) (map[string]any, map[string]any) {
	if run == nil {
		return nil, nil
	}
	continuation := mapFromValue(run.ExecutionContext["replanContinuation"])
	if len(continuation) == 0 {
		return nil, nil
	}
	failureContext := mapFromValue(continuation["failureContext"])
	if len(failureContext) == 0 {
		failureContext = mapFromValue(continuation["ticketContext"])
	}
	return continuation, failureContext
}

func strategyReassessmentFromReplanContinuation(run *runModel.Run) map[string]any {
	continuation, failureContext := replanContinuationFailureContext(run)
	if len(failureContext) == 0 {
		return nil
	}
	targetScope := strings.TrimSpace(stringFromMap(failureContext, "failureTargetScope"))
	failureClass := strings.TrimSpace(stringFromMap(failureContext, failureOutputKeyClass))
	failureSignature := strings.TrimSpace(stringFromMap(failureContext, failureOutputKeySignature))
	if targetScope == "" && len(repairPatchTargetRefsFromScopeText(failureSignature)) == 0 {
		return nil
	}
	targetAttempts := replanContinuationTargetRepairAttempts(failureContext)
	if targetAttempts < 2 && !replanContinuationRequiresStrategyReassessment(continuation, failureContext) {
		return nil
	}
	if targetAttempts < 2 {
		targetAttempts = 2
	}
	input := map[string]any{
		"targetRepairAttempts":    targetAttempts,
		"failureTargetScope":      targetScope,
		failureOutputKeyClass:     failureClass,
		failureOutputKeySignature: failureSignature,
		"failedStepKind":          strings.TrimSpace(stringFromMap(failureContext, "failedStepKind")),
	}
	return repairStrategyReassessmentFromInput(input)
}

func replanContinuationRequiresStrategyReassessment(continuation map[string]any, failureContext map[string]any) bool {
	if len(continuation) == 0 && len(failureContext) == 0 {
		return false
	}
	var signals []string
	for _, key := range []string{
		"reason",
		"ticketReason",
		"resolutionType",
		"recoveryAction",
	} {
		signals = append(signals, strings.ToLower(strings.TrimSpace(stringFromMap(continuation, key))))
	}
	for _, key := range []string{
		"failureReason",
		"repairObjective",
		"recoveryPolicy",
	} {
		signals = append(signals, strings.ToLower(strings.TrimSpace(stringFromMap(failureContext, key))))
	}
	joined := strings.Join(signals, " ")
	for _, marker := range []string{
		"repeated",
		"duplicate",
		"exhausted",
		"same_target",
		"same-target",
		"same target",
		"loop",
	} {
		if strings.Contains(joined, marker) {
			return true
		}
	}
	return false
}

func replanContinuationTargetRepairAttempts(failureContext map[string]any) int64 {
	var ret int64
	for _, key := range []string{
		"targetRepairAttempts",
		"repairAttempts",
		"attempts",
		"runtimeRepairAttempt",
		"maxAttempts",
	} {
		if attempts := int64FromValue(failureContext[key]); attempts > ret {
			ret = attempts
		}
	}
	return ret
}

func nextStepInputFromOutput(run *runModel.Run, previousStep *runModel.Step) map[string]any {
	if previousStep == nil {
		return nextStepInput(run, "")
	}
	input := nextStepInput(run, previousStep.ID)
	inheritWorkspaceFileManifest(input, previousStep)
	inheritStableExecutionContextPackage(input, previousStep)
	inheritRuntimeRepairLineage(input, previousStep)
	if nextInput := mapFromValue(previousStep.Output[stepOutputKeyNextInput]); nextInput != nil {
		for key, value := range nextInput {
			input[key] = value
		}
	}
	inheritDirectNextLLMStage(input, previousStep)
	return input
}

func inheritDirectNextLLMStage(input map[string]any, previousStep *runModel.Step) {
	if len(input) == 0 || previousStep == nil {
		return
	}
	_, nextKind := nextStepSpec(previousStep.Output)
	if !strings.HasPrefix(strings.TrimSpace(nextKind), "llm/") {
		return
	}
	if strings.TrimSpace(stringFromMap(input, stepInputKeyRuntimeStage)) != "" ||
		strings.TrimSpace(stringFromMap(input, stepInputKeySoulStage)) != "" {
		return
	}
	if decisionBlockSpec(previousStep.Output).Reason != "" || approvalBlockSpec(previousStep.Output).Reason != "" {
		return
	}
	stage := strings.TrimSpace(stringFromMap(previousStep.Input, stepInputKeyRuntimeStage))
	if stage == "" {
		return
	}
	setRuntimeStage(input, stage)
	if soulStage := strings.TrimSpace(stringFromMap(previousStep.Input, stepInputKeySoulStage)); soulStage != "" {
		input[stepInputKeySoulStage] = soulStage
	}
}

func inheritRuntimeRepairLineage(input map[string]any, previousStep *runModel.Step) {
	if input == nil || previousStep == nil || previousStep.Input == nil {
		return
	}
	for _, key := range []string{
		stepInputKeyRepairOfStepID,
		stepInputKeyRepairOfRootStepID,
		stepInputKeyRepairOfFailureClass,
		stepInputKeyRepairOfFailureSig,
	} {
		if value, ok := previousStep.Input[key]; ok {
			input[key] = value
		}
	}
}

func inheritRuntimeRepairGraphContract(input map[string]any, previousStep *runModel.Step, nodeKind string) {
	if input == nil || previousStep == nil || previousStep.Input == nil {
		return
	}
	if !previousStepCarriesRuntimeRepairContract(previousStep) {
		return
	}
	setRuntimeStage(input, runtimeStageRepair)
	preserveRuntimeContract := canonicalRuntimeGraphNodeKind(nodeKind) != llmWorkspaceExecutionStepKind
	for _, key := range []string{
		stepInputKeyRecoveryAction,
		stepInputKeyRecoveryRootStepID,
		stepInputKeyRecoveryLineageType,
		stepInputKeyRecoveryBudgetScope,
		failureOutputKeyClass,
		failureOutputKeySignature,
		"failureTargetScope",
		"targetScope",
		"repairActionPackage",
		"patchRepairContext",
		"failureTargetRepairContext",
		"primaryDiagnosticRepair",
		"structuredFailureSignals",
		"repairObjective",
		"stateChangingOutputRequired",
	} {
		if value, ok := previousStep.Input[key]; ok && input[key] == nil {
			if preserveRuntimeContract && runtimeGraphNodeRequiresFullRepairContract(key) {
				input[key] = value
			} else {
				input[key] = compactLLMContextValue(value, 1)
			}
		}
	}
	if canonicalRuntimeGraphNodeKind(nodeKind) != llmWorkspaceExecutionStepKind {
		return
	}
	input[stepInputKeyRuntimeRepairFollowUp] = true
	if value, ok := previousStep.Input[stepInputKeyRuntimeRepairAttempt]; ok && input[stepInputKeyRuntimeRepairAttempt] == nil {
		input[stepInputKeyRuntimeRepairAttempt] = value
	}
}

func runtimeGraphNodeRequiresFullRepairContract(key string) bool {
	switch strings.TrimSpace(key) {
	case "repairActionPackage",
		"patchRepairContext",
		"failureTargetRepairContext",
		"primaryDiagnosticRepair",
		"structuredFailureSignals":
		return true
	default:
		return false
	}
}

func previousStepCarriesRuntimeRepairContract(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	if boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) {
		return true
	}
	if runtimeStageFromStepInput(step.Input) == runtimeStageRepair {
		return true
	}
	for _, key := range []string{
		stepInputKeyRepairOfStepID,
		stepInputKeyRepairOfRootStepID,
		stepInputKeyRepairOfFailureClass,
		stepInputKeyRepairOfFailureSig,
	} {
		if strings.TrimSpace(stringFromMap(step.Input, key)) != "" {
			return true
		}
	}
	return false
}

func inheritWorkspaceFileManifest(input map[string]any, previousStep *runModel.Step) {
	if input == nil || previousStep == nil {
		return
	}
	manifest := mapSliceFromValue(previousStep.Input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) == 0 {
		if pkg := mapFromValue(previousStep.Input[stepInputKeyExecutionContextPackage]); pkg != nil {
			manifest = mapSliceFromValue(pkg[stepInputKeyWorkspaceFileManifest])
		}
	}
	if len(manifest) == 0 {
		return
	}
	input[stepInputKeyWorkspaceFileManifest] = compactWorkspaceFileManifest(manifest)
}

func inheritStableExecutionContextPackage(input map[string]any, previousStep *runModel.Step) {
	if input == nil || previousStep == nil {
		return
	}
	stablePayload := stableExecutionContextPackagePayload(stableExecutionContextPackageFromStep(previousStep))
	if len(stablePayload) == 0 {
		return
	}
	input[stepInputKeyExecutionContextPackage] = stablePayload
}

func runtimeChildID(parent common.StepID, suffix string) string {
	base := strings.TrimSpace(string(parent))
	base = strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(base)
	suffix = strings.TrimSpace(suffix)
	if base == "" {
		return suffix
	}
	if suffix == "" {
		return base
	}
	return base + "-" + suffix
}

func runtimeReadableIDPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-", "_", "-").Replace(value)
	value = strings.Trim(value, "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	if len(value) > 64 {
		value = strings.Trim(value[:64], "-")
	}
	return value
}

func runtimeReadableStepLabel(runID common.RunID, stepID common.StepID) string {
	raw := strings.TrimSpace(string(stepID))
	if raw == "" {
		return ""
	}
	runPrefix := strings.TrimSpace(string(runID)) + "/"
	if strings.TrimSpace(string(runID)) != "" && strings.HasPrefix(raw, runPrefix) {
		raw = strings.TrimPrefix(raw, runPrefix)
	}
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return "run-root"
	}
	if raw == "step-1" {
		return "planning-001"
	}
	parts := strings.Split(raw, "/")
	first := strings.TrimSpace(parts[0])
	switch {
	case strings.HasPrefix(first, "planning-"),
		strings.HasPrefix(first, "repair-"),
		strings.HasPrefix(first, "provider-retry-"),
		strings.HasPrefix(first, "follow-up-"),
		strings.HasPrefix(first, "assessment-correction-"):
		return runtimeReadableStageLabel(first)
	case strings.HasPrefix(first, "graph-") && len(parts) > 1:
		return runtimeReadableIDPart(first + "-" + parts[len(parts)-1])
	}
	return runtimeReadableIDPart(first)
}

func runtimeReadableStageLabel(value string) string {
	label := runtimeReadableIDPart(value)
	if strings.Contains(label, "provider-retry-") {
		if idx := strings.Index(label, "-of-"); idx > 0 {
			label = strings.Trim(label[:idx], "-")
		}
	}
	return label
}

func nextStepInput(run *runModel.Run, previousStepID common.StepID) map[string]any {
	input := runExecutionInput(run, previousStepID)
	if input == nil {
		return nil
	}
	if previousStepID == "" {
		delete(input, "previousStepId")
	}
	return input
}

func runExecutionInput(run *runModel.Run, previousStepID common.StepID) map[string]any {
	if run == nil {
		return nil
	}
	newCoverageSubsystem().ensure(run)
	coverageFocus := mapFromValue(run.ExecutionContext[coverageFocusKey])
	input := map[string]any{
		"workspaceId":    string(run.WorkspaceID),
		"previousStepId": string(previousStepID),
	}
	if run.GoalID != "" {
		input["goalId"] = string(run.GoalID)
	}
	if run.DoDID != "" {
		input["doDId"] = string(run.DoDID)
	}
	if executionContext := slimExecutionContextForStepInput(run.ExecutionContext); len(executionContext) > 0 {
		input["executionContext"] = executionContext
	}
	inheritRuntimeContractSignalsFromExecutionContext(input, run.ExecutionContext)
	if context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]); len(context) > 0 {
		input[coverageWorkItemContextKey] = coverageWorkItemContextAlignedToFocus(context, coverageFocus)
	}
	if baseline := mapFromValue(run.ExecutionContext[gapBaselineKey]); len(baseline) > 0 {
		input[gapBaselineKey] = baseline
	}
	if acceptance := runtimeAcceptanceSignalFromExecutionContext(run.ExecutionContext); len(acceptance) > 0 {
		input["acceptance"] = acceptance
		if len(coverageFocus) > 0 {
			input[coverageFocusKey] = coverageFocus
		}
	}
	return input
}

func inheritRuntimeContractSignalsFromExecutionContext(input map[string]any, executionContext map[string]any) {
	if len(input) == 0 || len(executionContext) == 0 {
		return
	}
	for _, key := range []string{
		"stateChangingOutputRequired",
		"repairObjective",
		"requestedOutputMode",
		"llmOutputPolicy",
	} {
		if value, ok := executionContext[key]; ok && value != nil {
			input[key] = cloneStepInputValue(value)
		}
	}
	if pkg := mapFromValue(executionContext["repairActionPackage"]); len(pkg) > 0 {
		input["repairActionPackage"] = cloneStepPayload(pkg)
	}
	if contract := mapFromValue(executionContext["executionContract"]); len(contract) > 0 {
		input["executionContract"] = cloneStepPayload(contract)
	}
}

func slimExecutionContextForStepInput(executionContext map[string]any) map[string]any {
	if len(executionContext) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"dialogLanguage",
		"language",
		"locale",
		"systemLanguage",
		"systemLocale",
		"llmOperation",
		"soulId",
	} {
		if value, ok := executionContext[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	for _, key := range []string{
		"compressedContext",
		"soul",
		"soulRouting",
	} {
		if value, ok := executionContext[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if value := mapFromValue(executionContext["rejectedResultContinuation"]); len(value) > 0 {
		ret["rejectedResultContinuation"] = compactRejectedResultContinuationForRuntimeContext(value)
	}
	if value := mapFromValue(executionContext["replanContinuation"]); len(value) > 0 {
		if len(mapFromValue(executionContext["rejectedResultContinuation"])) > 0 &&
			!replanContinuationCarriesActionableFailure(executionContext) {
			ret["replanContinuation"] = compactHistoricalReplanContinuationForRuntimeContext(value)
		} else {
			ret["replanContinuation"] = compactReplanContinuationForRuntimeContext(value)
		}
	}
	if value := mapSliceFromValue(executionContext["governanceHistory"]); len(value) > 0 {
		ret["governanceHistory"] = compactGovernanceHistoryForRuntimeContext(value, 6)
	}
	if value := mapFromValue(executionContext[coverageWorkItemContextKey]); len(value) > 0 {
		ret[coverageWorkItemContextKey] = coverageWorkItemContextAlignedToFocus(value, mapFromValue(executionContext[coverageFocusKey]))
	}
	if value := mapFromValue(executionContext[gapBaselineKey]); len(value) > 0 {
		ret[gapBaselineKey] = value
	}
	if acceptance := runtimeAcceptanceSignalFromExecutionContext(executionContext); len(acceptance) > 0 {
		ret["acceptance"] = acceptance
	}
	return ret
}

func cloneStepPayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
