package orchestrator

import (
	"context"
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	contractViolationRuleRequiredOutputMissing              = "required_output_missing"
	contractViolationRuleWrongOutputShape                   = "wrong_output_shape"
	contractViolationRuleAcceptedKindMismatch               = "accepted_kind_mismatch"
	contractViolationRuleStateChangingRequiredMissing       = "state_changing_required_missing"
	contractViolationRuleConcreteValidationMissing          = "concrete_validation_contract_missing"
	contractViolationRuleValidationEvidenceMetadataMissing  = "validation_evidence_metadata_missing"
	contractViolationRuleValidationScratchRequired          = "validation_scratch_required"
	contractViolationRuleValidationScratchEscape            = "validation_scratch_escape"
	contractViolationRuleValidationProcessControlForbidden  = "validation_process_control_forbidden"
	contractViolationRuleValidationDetachedProcessForbidden = "validation_detached_process_forbidden"
	contractViolationRuleValidationShellSafetyUnproven      = "validation_shell_safety_unproven"
	contractViolationRuleManagedValidationRequired          = "managed_validation_required"
	contractViolationRuleDoDAssessmentEvidenceMissing       = "dod_assessment_missing_evidence_refs"
	contractViolationRuleCoverageAssessmentIncomplete       = "coverage_assessment_incomplete"
	contractViolationRuleCoverageMapIncomplete              = "coverage_map_incomplete"
	contractViolationRuleTraceabilityRefMismatch            = "traceability_ref_mismatch"
	contractViolationRuleFocusBindingMismatch               = "focus_binding_mismatch"
	contractViolationRuleDecisionBlockedWhenActionable      = "decision_blocked_when_actionable"
	contractViolationRuleUnsafeOutput                       = "unsafe_output"
	contractViolationRuleUnparseableOutput                  = runtimemodel.ContractViolationRuleUnparseableOutput
	contractViolationRulePatchApplyContractFailed           = "patch_apply_contract_failed"
	contractViolationRuleAvailableSnapshotIgnored           = "available_snapshot_ignored"
	contractViolationRuleRuntimeContract                    = "runtime_contract_violation"

	stepOutputKeyContractViolation        = "contractViolation"
	stepInputKeyContractViolationRecovery = "contractViolationRecovery"

	contractViolationCorrectionModeTargeted     = "targeted"
	contractViolationCorrectionModeForcedNarrow = "forced_narrow"
)

type runtimeContractViolationError struct {
	cause          error
	violationType  string
	rule           string
	recoverability string
}

func newRuntimeContractViolationError(violationType string, message string) error {
	return newRuntimeContractViolationRuleError(violationType, contractViolationRuleFromType(violationType), message)
}

func newRuntimeContractViolationRuleError(violationType string, rule string, message string) error {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		rule = contractViolationRuleFromType(violationType)
	}
	return runtimeContractViolationError{
		cause:          fmt.Errorf("%w: %s", common.ErrInvalidArgument, strings.TrimSpace(message)),
		violationType:  strings.TrimSpace(violationType),
		rule:           rule,
		recoverability: contractViolationRecoverabilityForRule(rule),
	}
}

func (e runtimeContractViolationError) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return "runtime contract violation"
}

func (e runtimeContractViolationError) Unwrap() error {
	return e.cause
}

func contractViolationRuleFromType(violationType string) string {
	switch strings.TrimSpace(violationType) {
	case contractViolationRepairNoStateChangingOutput,
		contractViolationRepairGraphWithoutPatchApply,
		contractViolationExecutionNoStateChangingOutput:
		return contractViolationRuleStateChangingRequiredMissing
	case contractViolationValidationEvidenceRequired:
		return contractViolationRuleConcreteValidationMissing
	case contractViolationGraphNodeReturnedGraph,
		contractViolationRepairReturnedGraph:
		return contractViolationRuleWrongOutputShape
	case contractViolationAssessmentOnlyExecutableOutput:
		return contractViolationRuleWrongOutputShape
	case contractViolationLLMSchemaInvalid:
		return contractViolationRuleUnparseableOutput
	case contractViolationFocusBindingMismatch:
		return contractViolationRuleFocusBindingMismatch
	case contractViolationCoverageTargetsRequired:
		return contractViolationRuleCoverageMapIncomplete
	case contractViolationDoDAssessmentEvidenceMissing:
		return contractViolationRuleDoDAssessmentEvidenceMissing
	case contractViolationCoverageMapIncomplete:
		return contractViolationRuleCoverageMapIncomplete
	case contractViolationGraphDependsOnLLMBeforePatch,
		contractViolationPatchTargetRefRequired,
		contractViolationPatchContentRequired,
		contractViolationPatchUnresolvedTemplate,
		contractViolationPatchDispatchInvalid,
		contractViolationFileEditConflict:
		return contractViolationRulePatchApplyContractFailed
	case contractViolationAvailableSnapshotIgnored:
		return contractViolationRuleAvailableSnapshotIgnored
	default:
		return contractViolationRuleRuntimeContract
	}
}

func contractViolationFromFailedStep(run *runModel.Run, step *runModel.Step, execErr error, attempt int, maxAttempts int, signatureRepeatCount int) (runtimemodel.ContractViolation, bool) {
	if run == nil || step == nil {
		return runtimemodel.ContractViolation{}, false
	}
	output := step.Output
	violationType := strings.TrimSpace(stringFromMap(output, outputKeyContractViolationType))
	class := strings.TrimSpace(stringFromMap(output, failureOutputKeyClass))
	if strings.TrimSpace(step.Kind) != llmWorkspaceExecutionStepKind && violationType == "" {
		return runtimemodel.ContractViolation{}, false
	}
	if strings.TrimSpace(step.Kind) == llmWorkspaceExecutionStepKind &&
		violationType == "" &&
		class != failureClassDoDCoverageMissing &&
		!runtimeFailureIsLLMContractLoop(step) {
		return runtimemodel.ContractViolation{}, false
	}
	rule := contractViolationRuleFromStep(step)
	recoverability := contractViolationRecoverabilityForRule(rule)
	correction := contractViolationCorrectionProfileForStep(step, rule, attempt, signatureRepeatCount)
	budgetScope := contractViolationRecoveryBudgetScope(step.Input, step)
	violation := runtimemodel.ContractViolation{
		ID:                   fmt.Sprintf("%s/%s/contract-violation", run.ID, step.ID),
		WorkspaceID:          run.WorkspaceID,
		RunID:                run.ID,
		StepID:               step.ID,
		SourceStepID:         step.ID,
		ViolationType:        violationType,
		Stage:                firstNonEmptyString(stringFromMap(output, "stage"), stringFromMap(mapFromValue(output["runtimeDiagnostics"]), "kind")),
		RuntimeStage:         runtimeStageFromStepInput(step.Input),
		RuntimeStageMode:     stringFromMap(step.Input, "runtimeStageMode"),
		RepairObjective:      stringFromMap(step.Input, "repairObjective"),
		ViolatedRule:         rule,
		Signature:            contractViolationSignature(violationType, rule, stringFromMap(output, failureOutputKeySignature)),
		ExpectedContract:     contractViolationExpectedContract(step),
		ActualOutputSummary:  contractViolationActualOutputSummary(step, execErr),
		RejectedOutputRef:    fmt.Sprintf("%s/%s/rejected-output", run.ID, step.ID),
		FailureClass:         class,
		FailureSignature:     stringFromMap(output, failureOutputKeySignature),
		Recoverability:       recoverability,
		RetryHint:            contractViolationRetryHint(rule),
		ForbiddenRepeat:      contractViolationForbiddenRepeat(rule),
		CorrectionProfileID:  stringFromMap(correction, "profileId"),
		CorrectionMode:       stringFromMap(correction, "mode"),
		CorrectionContract:   mapFromValue(correction["contract"]),
		TargetScope:          stringFromMap(correction, "targetScope"),
		BudgetScope:          budgetScope,
		SourceRefs:           contractViolationSourceRefs(step),
		Attempt:              attempt,
		MaxAttempts:          maxAttempts,
		SignatureRepeatCount: signatureRepeatCount,
		CreatedAt:            runtimeEvidenceTimestamp(),
	}
	violation = violation.Normalized()
	return violation, true
}

func attachContractViolationDiagnostics(run *runModel.Run, step *runModel.Step, execErr error) {
	if run == nil || step == nil || step.Output == nil {
		return
	}
	if _, ok := step.Output[stepOutputKeyContractViolation]; ok {
		return
	}
	violation, ok := contractViolationFromFailedStep(run, step, execErr, 0, 0, 0)
	if !ok {
		return
	}
	step.Output[stepOutputKeyContractViolation] = contractViolationMap(violation)
	diagnostics := cloneStepPayload(mapFromValue(step.Output["runtimeDiagnostics"]))
	if diagnostics == nil {
		diagnostics = map[string]any{}
	}
	diagnostics["contractViolation"] = contractViolationMap(violation)
	diagnostics["contractViolationRule"] = violation.ViolatedRule
	diagnostics["contractViolationRecoverability"] = violation.Recoverability
	step.Output["runtimeDiagnostics"] = diagnostics
}

func applyContractViolationRecoveryProjection(input map[string]any, failedStep *runModel.Step, attempt int, maxAttempts int, signatureRepeatCount int) {
	if input == nil || failedStep == nil {
		return
	}
	violation := contractViolationFromFailedOutput(failedStep)
	if len(violation) == 0 {
		return
	}
	ensureAvailableTargetSnapshotsForStateChangingRepair(input)
	violation["attempt"] = attempt
	violation["maxAttempts"] = maxAttempts
	violation["signatureRepeatCount"] = signatureRepeatCount
	correction := contractViolationCorrectionProfileForStep(failedStep, stringFromMap(violation, "violatedRule"), attempt, signatureRepeatCount)
	if len(correction) > 0 {
		violation["correctionProfileId"] = stringFromMap(correction, "profileId")
		violation["correctionMode"] = stringFromMap(correction, "mode")
		violation["targetScope"] = stringFromMap(correction, "targetScope")
		violation["correctionContract"] = mapFromValue(correction["contract"])
		applyForcedNarrowContractViolationRequiredOutput(input, failedStep, correction)
	}
	violation = contractViolationDiagnosticOnlyMap(violation, input)
	if hint := contractViolationRetryHintFromAuthority(input, violation); hint != "" {
		violation["retryHint"] = hint
	}
	budgetScope := contractViolationRecoveryBudgetScope(input, failedStep)
	if len(budgetScope) > 0 {
		input[stepInputKeyRecoveryBudgetScope] = budgetScope
		violation["budgetScope"] = cloneStepPayload(budgetScope)
	}
	input[stepInputKeyContractViolationRecovery] = violation
	input["contractViolationRetry"] = true
	input[stepInputKeyRecoveryLineageType] = "contract_violation_retry"
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"The previous LLM output was rejected by the runtime contract before any side effects were executed.",
		"Treat contractViolationRecovery.expectedContract as diagnostic lineage only; it explains the rejected output but must not override the current narrow retry contract.",
		"Use contractViolationRecovery.correctionContract and the current repairActionPackage.requiredOutput as the authoritative retry contract; if correctionMode is forced_narrow, return only an allowed output shape from that contract or a real governance blocker.",
		contractViolationRetryHintFromAuthority(input, violation),
		"Return one valid JSON decision that satisfies the same runtimeStage and repairObjective; do not weaken Goal, DoD, evidence, or approval requirements.",
	)
}

func contractViolationDiagnosticOnlyMap(violation map[string]any, input map[string]any) map[string]any {
	if len(violation) == 0 {
		return violation
	}
	next := cloneStepPayload(violation)
	next["diagnosticOnly"] = true
	next["actionAdviceAuthoritative"] = false
	if actual := mapFromValue(next["actualOutputSummary"]); len(actual) > 0 {
		next["actualOutputSummary"] = scrubDiagnosticActionAdvice(actual, input)
	}
	if expected := mapFromValue(next["expectedContract"]); len(expected) > 0 {
		expected["diagnosticOnly"] = true
		expected["authoritativeForNextOutput"] = false
		next["expectedContract"] = expected
	}
	return next
}

func scrubDiagnosticActionAdvice(value map[string]any, input map[string]any) map[string]any {
	if len(value) == 0 {
		return value
	}
	next := cloneStepPayload(value)
	for _, key := range []string{"error", "reason", "summary", "parseError", "repairError"} {
		text := strings.TrimSpace(stringFromMap(next, key))
		if text == "" {
			continue
		}
		if outputShapeAdviceConflictsWithRequiredOutput(text, input) {
			next[key+"DiagnosticRef"] = "scrubbed_conflicting_output_shape_advice"
			delete(next, key)
		}
	}
	return next
}

func contractViolationRetryHintFromAuthority(input map[string]any, violation map[string]any) string {
	required := currentRequiredOutputForAuthority(input)
	contract := mapFromValue(violation["correctionContract"])
	if len(required) == 0 && len(contract) == 0 {
		return ""
	}
	allowed := compactUniqueStrings(append(stringSliceFromValue(contract["allowedOutputs"]), stringSliceFromValue(required["allowedOutputShapes"])...))
	requiredFields := compactUniqueStrings(append(stringSliceFromValue(contract["requiredFields"]), stringSliceFromValue(required["requiredFields"])...))
	forbidden := compactUniqueStrings(append(stringSliceFromValue(contract["forbiddenOutputs"]), stringSliceFromValue(required["forbiddenOutputShapes"])...))
	parts := []string{}
	if len(allowed) > 0 {
		parts = append(parts, "Allowed output shapes: "+strings.Join(allowed, ", ")+".")
	}
	if len(requiredFields) > 0 {
		parts = append(parts, "Required fields: "+strings.Join(requiredFields, ", ")+".")
	}
	if boolValue(required["mustProduceConcreteValidationEvidence"]) {
		parts = append(parts, "If returning executionGraph validation, each validation node must declare input.validationEvidence.mode, input.validationEvidence.observationType, input.validationEvidence.targetRefs, and input.validationEvidence.observedClaims.")
	}
	if !requiredOutputAllowsTopLevelShape(required, "shellCommands") && !requiredOutputAllowsTopLevelShape(contract, "shellCommands") {
		forbidden = append(forbidden, "top_level_shellCommands")
		parts = append(parts, "Do not return top-level shellCommands unless the current requiredOutput.allowedOutputShapes explicitly permits that shape.")
	}
	if len(forbidden) > 0 {
		parts = append(parts, "Forbidden output shapes: "+strings.Join(compactUniqueStrings(forbidden), ", ")+".")
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func currentRequiredOutputForAuthority(input map[string]any) map[string]any {
	for _, source := range []map[string]any{
		mapFromValue(input["requiredOutput"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		mapFromValue(mapFromValue(input["executionContext"])["requiredOutput"]),
		mapFromValue(mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"])["requiredOutput"]),
		mapFromValue(mapFromValue(mapFromValue(input["executionContext"])[stepInputKeyStagePacket])["requiredOutput"]),
	} {
		if len(source) > 0 {
			return source
		}
	}
	return nil
}

func applyForcedNarrowContractViolationRequiredOutput(input map[string]any, failedStep *runModel.Step, correction map[string]any) {
	if len(input) == 0 || failedStep == nil || len(correction) == 0 {
		return
	}
	if stringFromMap(correction, "mode") != contractViolationCorrectionModeForcedNarrow {
		return
	}
	contract := mapFromValue(correction["contract"])
	required := forcedNarrowContractViolationRequiredOutput(input, failedStep, correction, contract)
	if len(required) == 0 {
		return
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	required["contractViolationRetryNarrowed"] = true
	required["contractViolationCorrectionProfileId"] = stringFromMap(correction, "profileId")
	required["contractViolationRequiredOutputShape"] = stringFromMap(contract, "requiredOutputShape")
	if targetScope := strings.TrimSpace(stringFromMap(correction, "targetScope")); targetScope != "" {
		required["targetScope"] = targetScope
	}
	if allowed := stringSliceFromValue(contract["allowedOutputs"]); len(allowed) > 0 {
		required["allowedOutputShapes"] = allowed
	}
	if forbidden := stringSliceFromValue(contract["forbiddenOutputs"]); len(forbidden) > 0 {
		required["forbiddenOutputShapes"] = forbidden
	}
	pkg["requiredOutput"] = required
	input["repairActionPackage"] = pkg
	input["requiredOutput"] = cloneStepPayload(required)
	if packet := mapFromValue(input[stepInputKeyStagePacket]); len(packet) > 0 {
		packet["requiredOutput"] = cloneStepPayload(required)
		input[stepInputKeyStagePacket] = packet
	}
}

func reapplyForcedNarrowContractViolationRequiredOutput(input map[string]any, failedStep *runModel.Step) {
	if len(input) == 0 || failedStep == nil {
		return
	}
	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	if len(recovery) == 0 || stringFromMap(recovery, "correctionMode") != contractViolationCorrectionModeForcedNarrow {
		return
	}
	correction := map[string]any{
		"profileId":   stringFromMap(recovery, "correctionProfileId"),
		"mode":        stringFromMap(recovery, "correctionMode"),
		"targetScope": stringFromMap(recovery, "targetScope"),
		"contract":    mapFromValue(recovery["correctionContract"]),
	}
	applyForcedNarrowContractViolationRequiredOutput(input, failedStep, correction)
}

func normalizeCurrentActionContract(input map[string]any, failedStep *runModel.Step) map[string]any {
	if len(input) == 0 {
		return input
	}
	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	if len(recovery) == 0 || stringFromMap(recovery, "correctionMode") != contractViolationCorrectionModeForcedNarrow {
		return input
	}
	contract := mapFromValue(recovery["correctionContract"])
	shape := strings.TrimSpace(stringFromMap(contract, "requiredOutputShape"))
	switch shape {
	case "bounded_validation_evidence", "managed_validation_evidence":
		return normalizeCurrentActionValidationEvidenceContract(input, failedStep, recovery)
	case "bounded_failure_evidence":
		return normalizeCurrentActionFailureEvidenceContract(input, failedStep, recovery)
	case "valid_patch_apply", "state_changing_output", "state_changing_output_from_available_snapshot":
		return normalizeCurrentActionStateChangingContract(input, failedStep, recovery)
	default:
		return input
	}
}

func normalizeForcedNarrowManagedValidationInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	if len(recovery) == 0 ||
		stringFromMap(recovery, "correctionMode") != contractViolationCorrectionModeForcedNarrow ||
		stringFromMap(mapFromValue(recovery["correctionContract"]), "requiredOutputShape") != "managed_validation_evidence" {
		return input
	}
	return normalizeCurrentActionValidationEvidenceContract(input, nil, recovery)
}

func normalizeCurrentActionStateChangingContract(input map[string]any, failedStep *runModel.Step, recovery map[string]any) map[string]any {
	required := forcedNarrowRequiredOutputFromRecovery(input, failedStep, recovery)
	if len(required) == 0 {
		return input
	}
	setRuntimeStage(input, runtimeStageRepair)
	input[stepInputKeySoulStage] = soulStageExecution
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["stateChangingOutputRequired"] = true
	delete(input, "concreteFailureEvidenceRequired")
	delete(input, "boundedValidationRequired")
	delete(input, "evidenceRepairRequired")
	delete(input, stepInputKeyAssessmentOnly)
	delete(input, "assessmentOnlyReason")
	currentActionSetRequiredOutput(input, required)
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) > 0 {
		pkg["repairObjective"] = repairObjectiveStateChangingOutput
		pkg["stateChangingOutputRequired"] = true
		delete(pkg, "evidenceAcquisitionRequired")
		delete(pkg, "concreteFailureEvidenceRequired")
		delete(pkg, "evidenceAcquisitionReason")
		delete(pkg, "outputDecisionContract")
		delete(pkg, "exitCriteria")
		input["repairActionPackage"] = pkg
	}
	input["convergenceContract"] = map[string]any{
		"state":                       convergenceStateNeedStateChange,
		"runtimeStage":                runtimeStageRepair,
		"runtimeStageMode":            firstNonEmptyString(stringFromMap(input, stepInputKeyRuntimeStageMode), "strict_convergence"),
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
	}
	currentActionSetNextAction(input, map[string]any{
		"kind":   runtimemodel.NextActionTypeStateChange,
		"reason": "current forced-narrow correction contract requires state-changing output",
	})
	return input
}

func normalizeCurrentActionValidationEvidenceContract(input map[string]any, failedStep *runModel.Step, recovery map[string]any) map[string]any {
	required := forcedNarrowRequiredOutputFromRecovery(input, failedStep, recovery)
	if len(required) == 0 {
		return input
	}
	setRuntimeStage(input, runtimeStageVerification)
	input[stepInputKeySoulStage] = soulStageValidation
	input["repairObjective"] = repairObjectiveConcreteValidationEvidence
	input["boundedValidationRequired"] = true
	input["evidenceRepairRequired"] = true
	delete(input, "stateChangingOutputRequired")
	delete(input, "concreteFailureEvidenceRequired")
	delete(input, stepInputKeyAssessmentOnly)
	delete(input, "assessmentOnlyReason")
	currentActionSetRequiredOutput(input, required)
	input["convergenceContract"] = map[string]any{
		"state":            convergenceStateNeedValidation,
		"runtimeStage":     runtimeStageVerification,
		"runtimeStageMode": firstNonEmptyString(stringFromMap(input, stepInputKeyRuntimeStageMode), "strict_convergence"),
		"repairObjective":  repairObjectiveConcreteValidationEvidence,
	}
	currentActionSetNextAction(input, map[string]any{
		"kind":   "produce_validation_evidence",
		"reason": "current forced-narrow correction contract requires bounded validation evidence",
	})
	return input
}

func normalizeCurrentActionFailureEvidenceContract(input map[string]any, failedStep *runModel.Step, recovery map[string]any) map[string]any {
	required := forcedNarrowRequiredOutputFromRecovery(input, failedStep, recovery)
	if len(required) == 0 {
		return input
	}
	setRuntimeStage(input, runtimeStageRepair)
	input[stepInputKeySoulStage] = soulStageExecution
	input["repairObjective"] = repairObjectiveAcquireFailureEvidence
	input["concreteFailureEvidenceRequired"] = true
	delete(input, "stateChangingOutputRequired")
	delete(input, "boundedValidationRequired")
	delete(input, "evidenceRepairRequired")
	delete(input, stepInputKeyAssessmentOnly)
	delete(input, "assessmentOnlyReason")
	currentActionSetRequiredOutput(input, required)
	input["convergenceContract"] = map[string]any{
		"state":                           convergenceStateNeedEvidence,
		"runtimeStage":                    runtimeStageRepair,
		"runtimeStageMode":                firstNonEmptyString(stringFromMap(input, stepInputKeyRuntimeStageMode), "strict_convergence"),
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
	}
	currentActionSetNextAction(input, map[string]any{
		"kind":   "acquire_evidence",
		"reason": "current forced-narrow correction contract requires bounded evidence acquisition",
	})
	return input
}

func forcedNarrowRequiredOutputFromRecovery(input map[string]any, failedStep *runModel.Step, recovery map[string]any) map[string]any {
	if len(input) == 0 || len(recovery) == 0 {
		return nil
	}
	correction := map[string]any{
		"profileId":   stringFromMap(recovery, "correctionProfileId"),
		"mode":        stringFromMap(recovery, "correctionMode"),
		"targetScope": stringFromMap(recovery, "targetScope"),
		"contract":    mapFromValue(recovery["correctionContract"]),
	}
	required := forcedNarrowContractViolationRequiredOutput(input, failedStep, correction, mapFromValue(correction["contract"]))
	if len(required) == 0 {
		return nil
	}
	required["contractViolationRetryNarrowed"] = true
	required["contractViolationCorrectionProfileId"] = stringFromMap(correction, "profileId")
	required["contractViolationRequiredOutputShape"] = stringFromMap(mapFromValue(correction["contract"]), "requiredOutputShape")
	if targetScope := strings.TrimSpace(stringFromMap(correction, "targetScope")); targetScope != "" {
		required["targetScope"] = targetScope
	}
	contract := mapFromValue(correction["contract"])
	if allowed := stringSliceFromValue(contract["allowedOutputs"]); len(allowed) > 0 {
		required["allowedOutputShapes"] = allowed
	}
	if forbidden := stringSliceFromValue(contract["forbiddenOutputs"]); len(forbidden) > 0 {
		required["forbiddenOutputShapes"] = forbidden
	}
	return required
}

func currentActionSetRequiredOutput(input map[string]any, required map[string]any) {
	if len(input) == 0 || len(required) == 0 {
		return
	}
	nextRequired := cloneStepPayload(required)
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	pkg["requiredOutput"] = cloneStepPayload(nextRequired)
	input["repairActionPackage"] = pkg
	input["requiredOutput"] = cloneStepPayload(nextRequired)
	if packet := mapFromValue(input[stepInputKeyStagePacket]); len(packet) > 0 {
		packet["requiredOutput"] = cloneStepPayload(nextRequired)
		input[stepInputKeyStagePacket] = packet
	}
}

func currentActionSetNextAction(input map[string]any, action map[string]any) {
	if len(input) == 0 || len(action) == 0 {
		return
	}
	syncConvergenceNextActionMap(input, convergenceAuthorityKey, action)
	syncConvergenceNextActionMap(input, gapBaselineKey, action)
	if executionContext := mapFromValue(input["executionContext"]); len(executionContext) > 0 {
		syncConvergenceNextActionMap(executionContext, convergenceAuthorityKey, action)
		syncConvergenceNextActionMap(executionContext, gapBaselineKey, action)
		input["executionContext"] = executionContext
	}
}

func forcedNarrowContractViolationRequiredOutput(input map[string]any, failedStep *runModel.Step, correction map[string]any, contract map[string]any) map[string]any {
	shape := strings.TrimSpace(stringFromMap(contract, "requiredOutputShape"))
	pkg := mapFromValue(input["repairActionPackage"])
	existing := mapFromValue(pkg["requiredOutput"])
	switch shape {
	case "bounded_validation_evidence":
		return concreteValidationEvidenceRequiredOutputWithScratch(input, pkg, existing)
	case "managed_validation_evidence":
		return managedValidationEvidenceRequiredOutputWithScratch(input, pkg, existing)
	case "valid_patch_apply",
		"state_changing_output",
		"state_changing_output_from_available_snapshot":
		required := stateChangingRepairRequiredOutput()
		for _, key := range []string{
			"mustBindCoverageFocus",
			"coverageFocusId",
			"coverageWorkItemIds",
			"mustDeclareCoverageTargets",
			"coverageTargetMatchRule",
			"mustAddressPrimaryDiagnostics",
			"primaryDiagnosticMatchRule",
			"mustTargetFailureScope",
			"targetRelation",
		} {
			if value, ok := existing[key]; ok {
				required[key] = value
			}
		}
		if shape == "valid_patch_apply" || shape == "state_changing_output_from_available_snapshot" {
			required["acceptedKinds"] = []string{"patch/apply"}
			applyCurrentSnapshotDirectPatchContract(required)
		}
		if snapshots := contractViolationAvailableTargetSnapshotSummary(failedStep); len(snapshots) > 0 {
			required["availableTargetSnapshots"] = snapshots
		} else if snapshots := contractViolationTargetSnapshotSummary(failedStep); len(snapshots) > 0 {
			required["targetSnapshots"] = snapshots
		}
		return required
	case "bounded_failure_evidence":
		required := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, existing)
		required["mustProduceConcreteFailureEvidence"] = true
		required["mustIncludeStateChangingNode"] = false
		return required
	default:
		return nil
	}
}

func contractViolationRecoveryBudgetScope(input map[string]any, failedStep *runModel.Step) map[string]any {
	if failedStep == nil {
		return nil
	}
	rootStepID := runtimeRepairRootStepID(failedStep)
	scope := recoveryBudgetScope(
		"contract_violation_retry",
		rootStepID,
		stringFromMap(failedStep.Output, failureOutputKeyClass),
		stringFromMap(failedStep.Output, failureOutputKeySignature),
	)
	focus := dispatchAuthorityCurrentFocus(input)
	if len(focus) == 0 {
		focus = mapFromValue(input[coverageFocusKey])
	}
	if len(focus) == 0 {
		focus = mapFromValue(mapFromValue(input[coverageWorkItemContextKey])["currentFocus"])
	}
	if len(focus) == 0 {
		focus = mapFromValue(mapFromValue(input[gapBaselineKey])["currentFocus"])
	}
	if focusID := strings.TrimSpace(stringFromMap(focus, "id")); focusID != "" {
		scope["focusId"] = focusID
	}
	if sliceID := strings.TrimSpace(stringFromMap(focus, "sliceId")); sliceID != "" {
		scope["sliceId"] = sliceID
	}
	if ids := dispatchFocusWorkItemIDs(focus); len(ids) > 0 {
		scope["coverageWorkItemIds"] = ids
	}
	return scope
}

func contractViolationFromFailedOutput(step *runModel.Step) map[string]any {
	if step == nil {
		return nil
	}
	violation := cloneStepPayload(mapFromValue(step.Output[stepOutputKeyContractViolation]))
	if len(violation) == 0 {
		return nil
	}
	return violation
}

func failedStepHasRecoverableContractViolation(step *runModel.Step) bool {
	violation := contractViolationFromFailedOutput(step)
	if len(violation) == 0 {
		return false
	}
	rule := strings.TrimSpace(stringFromMap(violation, "violatedRule"))
	recoverability := strings.TrimSpace(stringFromMap(violation, "recoverability"))
	if rule == contractViolationRuleCoverageAssessmentIncomplete &&
		recoverability == runtimemodel.ContractViolationRecoverabilityRecomputeAuthority {
		return true
	}
	switch recoverability {
	case runtimemodel.ContractViolationRecoverabilityRetrySameContract,
		runtimemodel.ContractViolationRecoverabilityNarrowContract,
		runtimemodel.ContractViolationRecoverabilityNormalizeThenRetry:
		return true
	default:
		return false
	}
}

func contractViolationRetryAttemptCount(ctx context.Context, repair *repairSubsystem, runID common.RunID, failedStep *runModel.Step) int {
	if repair == nil || repair.steps == nil || runID == "" || failedStep == nil {
		return 0
	}
	steps, err := repair.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return 0
	}
	sourceStepID := string(failedStep.ID)
	count := 0
	for _, step := range steps {
		if step == nil || step.RunID != runID || strings.TrimSpace(step.Kind) != llmWorkspaceExecutionStepKind {
			continue
		}
		recovery := mapFromValue(step.Input[stepInputKeyContractViolationRecovery])
		if len(recovery) == 0 {
			continue
		}
		if strings.TrimSpace(stringFromMap(recovery, "sourceStepId")) == sourceStepID ||
			strings.TrimSpace(stringFromMap(recovery, "stepId")) == sourceStepID {
			count++
		}
	}
	return count
}

func contractViolationRunRetryCount(ctx context.Context, repair *repairSubsystem, runID common.RunID) int {
	if repair == nil || repair.steps == nil || runID == "" {
		return 0
	}
	steps, err := repair.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return 0
	}
	count := 0
	for _, step := range steps {
		if step != nil && step.RunID == runID && len(mapFromValue(step.Input[stepInputKeyContractViolationRecovery])) > 0 {
			count++
		}
	}
	return count
}

func contractViolationSignatureRepeatCount(ctx context.Context, repair *repairSubsystem, runID common.RunID, failedStep *runModel.Step) int {
	if repair == nil || repair.steps == nil || runID == "" || failedStep == nil {
		return 0
	}
	signature := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature))
	rule := strings.TrimSpace(stringFromMap(mapFromValue(failedStep.Output[stepOutputKeyContractViolation]), "violatedRule"))
	if signature == "" && rule == "" {
		return 0
	}
	steps, err := repair.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return 0
	}
	count := 0
	for _, step := range steps {
		if step == nil || step.ID == failedStep.ID || step.RunID != runID || step.State != common.StepStateFailed {
			continue
		}
		if signature != "" && strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature)) == signature {
			count++
			continue
		}
		if rule != "" && strings.TrimSpace(stringFromMap(mapFromValue(step.Output[stepOutputKeyContractViolation]), "violatedRule")) == rule {
			count++
		}
	}
	return count
}

func contractViolationRetryWithinBudget(ctx context.Context, repair *repairSubsystem, policy RuntimePolicy, run *runModel.Run, failedStep *runModel.Step) bool {
	if repair == nil || run == nil || failedStep == nil || !failedStepHasRecoverableContractViolation(failedStep) {
		return false
	}
	if !contractViolationCurrentActionAllowsLLMRetry(run, failedStep) {
		return false
	}
	policy = NormalizeRuntimePolicy(policy)
	if contractViolationRetryAttemptCount(ctx, repair, run.ID, failedStep) >= policy.MaxContractViolationRetriesPerStep {
		return false
	}
	if contractViolationRunRetryCount(ctx, repair, run.ID) >= policy.MaxContractViolationRetriesPerRun {
		return false
	}
	if contractViolationSignatureRepeatCount(ctx, repair, run.ID, failedStep) > policy.MaxRepeatedContractViolationSignature {
		return false
	}
	return true
}

func contractViolationCurrentActionAllowsLLMRetry(run *runModel.Run, failedStep *runModel.Step) bool {
	input := map[string]any{}
	if failedStep != nil {
		input = failedStep.Input
	}
	if validationRouteRequiresDeterministicHandling(input) {
		return false
	}
	kind := strings.TrimSpace(convergenceNextActionKind(run, input))
	if kind == "" {
		return true
	}
	switch kind {
	case "assess_gap", "repair", "acquire_target_snapshot", "state_change", "produce_state_change",
		"validate", "validation", "produce_validation_evidence",
		"assessment", "complete_coverage_assessment",
		"artifact", "produce_artifact_evidence":
		return true
	case "request_decision", "wait_decision", "request_approval", "wait_approval",
		"request_governance", "no_action", "complete", "fail", "fail_fast":
		return false
	default:
		return false
	}
}

func validationRouteRequiresDeterministicHandling(input map[string]any) bool {
	route := validationRouteFromStepInput(input)
	switch route {
	case runtimemodel.ValidationRouteRuntimeAction,
		runtimemodel.ValidationRouteDeferred,
		runtimemodel.ValidationRouteFail:
		return true
	default:
		return false
	}
}

func validationRouteFromStepInput(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	for _, source := range []map[string]any{
		mapFromValue(input["requiredOutput"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		mapFromValue(mapFromValue(input["executionContext"])["requiredOutput"]),
		mapFromValue(mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"])["requiredOutput"]),
	} {
		if route := strings.TrimSpace(stringFromMap(source, "validationRoute")); route != "" {
			return route
		}
	}
	return ""
}

func contractViolationRuleFromStep(step *runModel.Step) string {
	if step == nil {
		return contractViolationRuleRuntimeContract
	}
	output := step.Output
	if rule := strings.TrimSpace(stringFromMap(output, "contractViolationRule")); rule != "" {
		return rule
	}
	if rule := strings.TrimSpace(stringFromMap(mapFromValue(output[stepOutputKeyContractViolation]), "violatedRule")); rule != "" {
		return rule
	}
	switch strings.TrimSpace(stringFromMap(output, outputKeyContractViolationType)) {
	case contractViolationRepairNoStateChangingOutput,
		contractViolationRepairGraphWithoutPatchApply,
		contractViolationExecutionNoStateChangingOutput:
		return contractViolationRuleStateChangingRequiredMissing
	case contractViolationValidationEvidenceRequired:
		return contractViolationRuleConcreteValidationMissing
	case contractViolationGraphNodeReturnedGraph,
		contractViolationRepairReturnedGraph:
		return contractViolationRuleWrongOutputShape
	case contractViolationAssessmentOnlyExecutableOutput:
		return contractViolationRuleWrongOutputShape
	case contractViolationFocusBindingMismatch:
		return contractViolationRuleFocusBindingMismatch
	case contractViolationCoverageTargetsRequired:
		return contractViolationRuleCoverageMapIncomplete
	case contractViolationDoDAssessmentEvidenceMissing:
		return contractViolationRuleDoDAssessmentEvidenceMissing
	case contractViolationCoverageMapIncomplete:
		return contractViolationRuleCoverageMapIncomplete
	case contractViolationGraphDependsOnLLMBeforePatch,
		contractViolationPatchTargetRefRequired,
		contractViolationPatchContentRequired,
		contractViolationPatchUnresolvedTemplate,
		contractViolationPatchDispatchInvalid,
		contractViolationFileEditConflict:
		return contractViolationRulePatchApplyContractFailed
	case contractViolationAvailableSnapshotIgnored:
		return contractViolationRuleAvailableSnapshotIgnored
	}
	if strings.TrimSpace(stringFromMap(output, failureOutputKeyClass)) == failureClassDoDCoverageMissing {
		if doDCoverageMissingOutputHasStructuredCoverageGaps(output) {
			return contractViolationRuleCoverageAssessmentIncomplete
		}
		return contractViolationRuleDoDAssessmentEvidenceMissing
	}
	if strings.TrimSpace(stringFromMap(output, "stage")) == "llm_schema_invalid" {
		return contractViolationRuleUnparseableOutput
	}
	return contractViolationRuleRuntimeContract
}

func doDCoverageMissingOutputHasStructuredCoverageGaps(output map[string]any) bool {
	if len(output) == 0 {
		return false
	}
	if len(missingDoDItemsFromFailureOutput(output)) > 0 ||
		len(missingGoalCoverageFromFailureOutput(output)) > 0 ||
		len(missingDeliverableCoverageFromFailureOutput(output)) > 0 {
		return true
	}
	diagnostics := mapFromValue(output["runtimeDiagnostics"])
	return len(mapSliceFromValue(diagnostics["missingDoDItems"])) > 0 ||
		len(mapSliceFromValue(diagnostics["missingGoalCoverage"])) > 0 ||
		len(mapSliceFromValue(diagnostics["missingDeliverableCoverage"])) > 0
}

func contractViolationRecoverabilityForRule(rule string) string {
	switch strings.TrimSpace(rule) {
	case contractViolationRuleUnsafeOutput:
		return runtimemodel.ContractViolationRecoverabilityFailFast
	case contractViolationRuleAcceptedKindMismatch:
		return runtimemodel.ContractViolationRecoverabilityNarrowContract
	case contractViolationRuleManagedValidationRequired,
		contractViolationRuleValidationScratchEscape,
		contractViolationRuleValidationProcessControlForbidden,
		contractViolationRuleValidationDetachedProcessForbidden,
		contractViolationRuleValidationShellSafetyUnproven:
		return runtimemodel.ContractViolationRecoverabilityNarrowContract
	case contractViolationRuleTraceabilityRefMismatch:
		return runtimemodel.ContractViolationRecoverabilityNormalizeThenRetry
	case contractViolationRuleFocusBindingMismatch:
		return runtimemodel.ContractViolationRecoverabilityNarrowContract
	case contractViolationRuleCoverageAssessmentIncomplete:
		return runtimemodel.ContractViolationRecoverabilityRecomputeAuthority
	default:
		return runtimemodel.ContractViolationRecoverabilityRetrySameContract
	}
}

func contractViolationRetryHint(rule string) string {
	switch strings.TrimSpace(rule) {
	case contractViolationRuleStateChangingRequiredMissing:
		return "return output that satisfies stateChangingOutputRequired and requiredOutput without repeating a read-only graph"
	case contractViolationRuleConcreteValidationMissing:
		return "return bounded concrete validation evidence with validationEvidence mode, observation, target refs, and observed claims metadata"
	case contractViolationRuleValidationEvidenceMetadataMissing:
		return "return concrete validation evidence with validationEvidence.mode, validationEvidence.observationType, validationEvidence.targetRefs, and validationEvidence.observedClaims on every validation node"
	case contractViolationRuleValidationScratchRequired:
		return "return scratch-bounded validation evidence using the runtime validationScratchDir contract"
	case contractViolationRuleValidationScratchEscape:
		return "keep controlled substitute and validation output writes inside validationScratchDir or use managed validation"
	case contractViolationRuleValidationProcessControlForbidden:
		return "use a runtime-managed validation session or a bounded foreground command without kill/pkill/killall/fuser -k"
	case contractViolationRuleValidationDetachedProcessForbidden:
		return "use validation/managed_session or a bounded foreground command instead of nohup, setsid, or background processes"
	case contractViolationRuleValidationShellSafetyUnproven:
		return "return validation/managed_session so runtime can provide scratch, timeout, cleanup, and bounded execution"
	case contractViolationRuleManagedValidationRequired:
		return "return validation/managed_session only; plain shell/command is not accepted for this validation route"
	case contractViolationRuleDoDAssessmentEvidenceMissing, contractViolationRuleCoverageMapIncomplete:
		return "rewrite the assessment so every satisfied item has traceable evidenceRefs and current coverage"
	case contractViolationRuleUnparseableOutput:
		return "return one complete valid JSON decision object only"
	case contractViolationRulePatchApplyContractFailed:
		return "return a valid patch/apply output that satisfies targetRef, patchType, content/diff, baseHash/snapshot, and current requiredOutput"
	case contractViolationRuleAvailableSnapshotIgnored:
		return "use the available complete target snapshots already supplied in the current action instead of claiming they are missing or returning another read-only acquisition graph"
	case contractViolationRuleFocusBindingMismatch:
		return "retry the same current action and bind every state-changing or validation output to the current coverage focus"
	default:
		return "return a valid output that satisfies the current correction contract and does not repeat actualOutputSummary"
	}
}

func contractViolationForbiddenRepeat(rule string) []string {
	switch strings.TrimSpace(rule) {
	case contractViolationRuleStateChangingRequiredMissing:
		return []string{"read-only graph without accepted state-changing evidence", "dodAssessment without required executable output"}
	case contractViolationRuleConcreteValidationMissing:
		return []string{"validation graph without validationEvidence contract", "detached or unbounded validation command"}
	case contractViolationRuleFocusBindingMismatch:
		return []string{"output bound to unrelated focus", "state-changing or validation output without current coverage focus binding"}
	case contractViolationRuleValidationEvidenceMetadataMissing:
		return []string{"validation node without validationEvidence.mode, validationEvidence.observationType, validationEvidence.targetRefs, or validationEvidence.observedClaims"}
	case contractViolationRuleValidationScratchRequired:
		return []string{"controlled substitute write without runtime validationScratchDir"}
	case contractViolationRuleValidationScratchEscape:
		return []string{"workspace-mutating validation shell", "write outside validationScratchDir"}
	case contractViolationRuleValidationProcessControlForbidden:
		return []string{"kill/pkill/killall/fuser -k in validation shell"}
	case contractViolationRuleValidationDetachedProcessForbidden:
		return []string{"nohup/setsid/background process in validation shell"}
	case contractViolationRuleValidationShellSafetyUnproven:
		return []string{"complex shell validation that cannot be statically proven safe"}
	case contractViolationRuleManagedValidationRequired:
		return []string{"plain shell/command when managed validation route is mandatory"}
	case contractViolationRuleDoDAssessmentEvidenceMissing:
		return []string{"satisfied assessment item without evidenceRefs"}
	case contractViolationRuleUnparseableOutput:
		return []string{"markdown wrapped JSON", "partial JSON", "multiple JSON objects"}
	case contractViolationRulePatchApplyContractFailed:
		return []string{"patch/apply without targetRef", "patch/apply without content or diff", "same-graph read-before-patch when current snapshot is supplied", "file_write without matching baseHash for existing target"}
	case contractViolationRuleAvailableSnapshotIgnored:
		return []string{"claiming current target snapshots are missing when complete safe snapshots are already available", "file/read or context rehydration for already supplied targets"}
	default:
		return []string{"same rejected output shape"}
	}
}

func contractViolationCorrectionProfileForStep(step *runModel.Step, rule string, attempt int, signatureRepeatCount int) map[string]any {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		rule = contractViolationRuleFromStep(step)
	}
	mode := contractViolationCorrectionModeTargeted
	if attempt > 1 || signatureRepeatCount > 0 {
		mode = contractViolationCorrectionModeForcedNarrow
	}
	if contractViolationRuleRequiresManagedValidationNarrow(rule) {
		mode = contractViolationCorrectionModeForcedNarrow
	}
	targetScope := runtimeFailureTargetScopeFromStep(step)
	contract := map[string]any{
		"violatedRule":     rule,
		"mode":             mode,
		"preserveContract": true,
		"mustNotWeaken":    []string{"runtimeStage", "repairObjective", "requiredOutput", "Goal", "DoD", "evidence", "approval"},
	}
	profileID := "contract_correction_generic"
	switch rule {
	case contractViolationRuleFocusBindingMismatch:
		profileID = "focus_binding_contract"
		contract["requiredOutputShape"] = "same_action_focus_bound_output"
		contract["allowedOutputs"] = contractViolationAllowedOutputsWithGovernanceBlockers("same_current_action_with_current_focus_binding")
		contract["requiredFields"] = []string{"coverageFocusId_or_coverageWorkItemIds", "stateChangingOrValidationActionBinding"}
		contract["forbiddenOutputs"] = append([]string{"unbound_state_change", "unbound_validation", "different_focus", "generic_repair_without_focus"}, contractViolationForbiddenContinuableBlockers()...)
		contract["mustSatisfy"] = []string{"currentActionContract.currentFocus", "repairActionPackage.requiredOutput.coverageWorkItemIds"}
		if focus := contractViolationCurrentFocusScope(step); len(focus) > 0 {
			contract["expectedFocus"] = focus
		}
	case contractViolationRuleStateChangingRequiredMissing:
		profileID = "state_changing_required_output"
		contract["requiredOutputShape"] = "state_changing_output"
		contract["allowedOutputs"] = contractViolationAllowedOutputsWithGovernanceBlockers("executionGraph_with_state_changing_node", "patch/apply", "accepted_state_changing_tool_or_shell", "incremental_artifact_state_change")
		contract["forbiddenOutputs"] = append([]string{"read_only_executionGraph", "failed_dodAssessment_only", "same_actualOutputSummary"}, contractViolationForbiddenContinuableBlockers()...)
		contract["mustSatisfy"] = []string{"stateChangingOutputRequired", "repairActionPackage.requiredOutput"}
	case contractViolationRuleConcreteValidationMissing,
		contractViolationRuleValidationEvidenceMetadataMissing,
		contractViolationRuleValidationScratchRequired,
		contractViolationRuleValidationScratchEscape,
		contractViolationRuleValidationProcessControlForbidden,
		contractViolationRuleValidationDetachedProcessForbidden,
		contractViolationRuleValidationShellSafetyUnproven,
		contractViolationRuleManagedValidationRequired:
		if profile, ok := contractViolationRequiredOutputCorrectionProfile(step, contract); ok {
			profileID = profile
		} else {
			profileID = "concrete_validation_evidence"
			contract["requiredOutputShape"] = "bounded_validation_evidence"
			contract["allowedOutputs"] = contractViolationAllowedOutputsWithGovernanceBlockers("executionGraph_with_declared_validationEvidence", "executionGraph_with_managed_validation_session", "satisfied_dodAssessment_with_concrete_evidenceRefs")
			contract["requiredFields"] = []string{"validationEvidence.mode", "validationEvidence.observationType", "validationEvidence.targetRefs", "validationEvidence.observedClaims"}
			contract["forbiddenOutputs"] = append([]string{"static_inspection_only", "detached_or_background_shell", "workspace_mutating_validation_shell", "write_outside_validationScratchDir"}, contractViolationForbiddenContinuableBlockers()...)
			if scratch := validationScratchDirFromStep(step); scratch != "" {
				contract[validationScratchDirKey] = scratch
			}
		}
		if contractViolationRuleRequiresManagedValidationNarrow(rule) && mode == contractViolationCorrectionModeForcedNarrow {
			profileID = "managed_validation_evidence"
			applyManagedValidationCorrectionContract(contract)
		}
	case contractViolationRulePatchApplyContractFailed:
		profileID = "patch_apply_contract"
		contract["requiredOutputShape"] = "valid_patch_apply"
		contract["allowedPatchModes"] = []string{"unified_diff", "text_replace", "file_write_with_matching_baseHash_when_safe"}
		contract["requiredFields"] = []string{"targetRef", "patchType", "content_or_diff"}
		contract["forbiddenOutputs"] = []string{"missing_targetRef", "missing_content_or_diff", "read_before_patch_when_snapshot_available", "file_write_existing_target_without_matching_baseHash", "unresolved_template_markers"}
		if targetScope != "" {
			contract["targetScope"] = targetScope
		}
		if snapshots := contractViolationTargetSnapshotSummary(step); len(snapshots) > 0 {
			contract["targetSnapshots"] = snapshots
		}
	case contractViolationRuleAvailableSnapshotIgnored:
		profileID = "available_snapshot_direct_action"
		contract["requiredOutputShape"] = "state_changing_output_from_available_snapshot"
		contract["allowedOutputs"] = contractViolationAllowedOutputsWithGovernanceBlockers("patch/apply_against_available_snapshot", "executionGraph_with_patch_apply_against_available_snapshot", "failed_dodAssessment_with_specific_non_snapshot_blocker")
		contract["forbiddenOutputs"] = append([]string{"missing_snapshot_claim", "file_read_for_available_target", "context_rehydration_graph", "same_actualOutputSummary"}, contractViolationForbiddenContinuableBlockers()...)
		contract["mustSatisfy"] = []string{"availableTargetSnapshots", "repairActionPackage.requiredOutput"}
		if targetScope != "" {
			contract["targetScope"] = targetScope
		}
		if snapshots := contractViolationAvailableTargetSnapshotSummary(step); len(snapshots) > 0 {
			contract["availableTargetSnapshots"] = snapshots
		}
	case contractViolationRuleCoverageAssessmentIncomplete:
		profileID = "coverage_next_action_recompute"
		contract["runtimeRecomputedAuthority"] = map[string]any{
			"owner":    "runtime",
			"required": true,
			"source":   "coverage_ledger",
		}
		if convergenceNextActionKind(nil, step.Input) != "" {
			if profile, ok := contractViolationRequiredOutputCorrectionProfile(step, contract); ok {
				profileID = profile
				contract["runtimeRecomputedAuthority"] = map[string]any{
					"owner":                  "runtime",
					"required":               true,
					"source":                 "coverage_ledger",
					"llmRequiredOutputShape": contract["requiredOutputShape"],
				}
			}
		}
		if stringFromMap(contract, "requiredOutputShape") == "" {
			contract["requiredOutputShape"] = "traceable_coverage_assessment"
			contract["llmRequiredOutputShape"] = "traceable_coverage_assessment"
			contract["allowedOutputs"] = []string{"bounded_focus_assessment_if_current_action_is_assessment"}
			contract["requiredFields"] = []string{"missingGoalCoverage_or_missingDeliverableCoverage", "evidenceRefs"}
		}
		contract["forbiddenOutputs"] = compactUniqueStrings(append(
			stringSliceFromValue(contract["forbiddenOutputs"]),
			"same_failed_global_dodAssessment",
			"satisfied_global_assessment_with_open_coverage",
			"untraceable_summary_only",
		))
	case contractViolationRuleCoverageMapIncomplete, contractViolationRuleDoDAssessmentEvidenceMissing:
		if profile, ok := contractViolationRequiredOutputCorrectionProfile(step, contract); ok {
			profileID = profile
		} else {
			profileID = "coverage_evidence_assessment"
			contract["requiredOutputShape"] = "traceable_coverage_assessment"
			contract["requiredFields"] = []string{"dodAssessment.items", "evidenceRefs"}
			contract["forbiddenOutputs"] = []string{"satisfied_item_without_evidenceRefs", "coverage_map_missing_current_goal_or_dod", "untraceable_summary_only"}
		}
	case contractViolationRuleUnparseableOutput, contractViolationRuleWrongOutputShape:
		if rule == contractViolationRuleWrongOutputShape {
			if profile, ok := contractViolationRequiredOutputCorrectionProfile(step, contract); ok {
				profileID = profile
				contract["mustReturnValidJSON"] = true
				contract["forbiddenOutputs"] = compactUniqueStrings(append(
					stringSliceFromValue(contract["forbiddenOutputs"]),
					"markdown_wrapped_json",
					"partial_json",
					"multiple_json_objects",
				))
			} else {
				profileID = "json_output_shape"
				contract["requiredOutputShape"] = "single_valid_json_decision"
				contract["forbiddenOutputs"] = []string{"markdown_wrapped_json", "partial_json", "multiple_json_objects", "side_effects"}
			}
		} else {
			profileID = "json_output_shape"
			contract["requiredOutputShape"] = "single_valid_json_decision"
			contract["forbiddenOutputs"] = []string{"markdown_wrapped_json", "partial_json", "multiple_json_objects", "side_effects"}
		}
	default:
		contract["requiredOutputShape"] = "expected_contract_compliant_output"
		contract["forbiddenOutputs"] = []string{"same_actualOutputSummary"}
	}
	if mode == contractViolationCorrectionModeForcedNarrow {
		contract["forcedNarrow"] = true
		if contractViolationRuleRequiresManagedValidationNarrow(rule) {
			contract["forcedNarrowRule"] = "validation process control cannot be safely repaired through plain shell retry; return only managed validation or a real governance blocker"
		} else {
			contract["forcedNarrowRule"] = "same-rule or same-target violation repeated; return only an allowed output shape from this correctionContract or a real governance blocker"
		}
	}
	return map[string]any{
		"profileId":   profileID,
		"mode":        mode,
		"targetScope": targetScope,
		"contract":    contract,
	}
}

func contractViolationCurrentFocusScope(step *runModel.Step) map[string]any {
	if step == nil || len(step.Input) == 0 {
		return nil
	}
	focus := dispatchAuthorityCurrentFocus(step.Input)
	if len(focus) == 0 {
		focus = mapFromValue(step.Input[coverageFocusKey])
	}
	if len(focus) == 0 {
		focus = mapFromValue(mapFromValue(step.Input[coverageWorkItemContextKey])["currentFocus"])
	}
	if len(focus) == 0 {
		return nil
	}
	ret := map[string]any{}
	if id := strings.TrimSpace(stringFromMap(focus, "id")); id != "" {
		ret["id"] = id
	}
	if ids := dispatchFocusWorkItemIDs(focus); len(ids) > 0 {
		ret["coverageWorkItemIds"] = ids
	}
	if refs := compactUniqueStrings(stringSliceFromValue(focus["targetRefs"])); len(refs) > 0 {
		ret["targetRefs"] = refs
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func contractViolationRuleRequiresManagedValidationNarrow(rule string) bool {
	switch strings.TrimSpace(rule) {
	case contractViolationRuleValidationScratchEscape,
		contractViolationRuleValidationProcessControlForbidden,
		contractViolationRuleValidationDetachedProcessForbidden,
		contractViolationRuleValidationShellSafetyUnproven,
		contractViolationRuleManagedValidationRequired:
		return true
	default:
		return false
	}
}

func applyManagedValidationCorrectionContract(contract map[string]any) {
	if contract == nil {
		return
	}
	contract["requiredOutputShape"] = "managed_validation_evidence"
	contract["allowedOutputs"] = contractViolationAllowedOutputsWithGovernanceBlockers("executionGraph_with_managed_validation_session")
	contract["requiredFields"] = []string{"validation/managed_session", "validationEvidence.mode", "validationEvidence.observationType", "validationEvidence.targetRefs", "validationEvidence.observedClaims"}
	contract["forbiddenOutputs"] = append([]string{"plain_shell_validation", "top_level_shellCommands", "detached_or_background_shell", "workspace_mutating_validation_shell", "write_outside_validationScratchDir"}, contractViolationForbiddenContinuableBlockers()...)
	contract["validationRoute"] = runtimemodel.ValidationRouteManagedHarness
	contract["mustUseManagedHarness"] = true
	contract["managedValidation"] = map[string]any{
		"scratchRequired": true,
		"timeoutRequired": true,
		"cleanupPolicy":   "process_group_on_timeout_or_completion",
		"portAllocation":  "optional",
	}
}

func contractViolationRequiredOutputCorrectionProfile(step *runModel.Step, contract map[string]any) (string, bool) {
	if step == nil || contract == nil {
		return "", false
	}
	pkg := mapFromValue(step.Input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	if len(required) == 0 {
		return "", false
	}
	if repairObjectiveRequiresStateChange(stringFromMap(step.Input, "repairObjective")) {
		contract["requiredOutputShape"] = "state_changing_output"
		contract["allowedOutputs"] = contractViolationAllowedOutputsWithGovernanceBlockers("executionGraph_with_state_changing_node", "patch/apply", "accepted_state_changing_tool_or_shell", "incremental_artifact_state_change")
		contract["forbiddenOutputs"] = append([]string{"failed_dodAssessment_only", "read_only_executionGraph", "same_actualOutputSummary"}, contractViolationForbiddenContinuableBlockers()...)
		contract["mustSatisfy"] = []string{"stateChangingOutputRequired", "repairActionPackage.requiredOutput"}
		return "state_changing_required_output", true
	}
	if boolValue(required["mustProduceConcreteValidationEvidence"]) {
		contract["requiredOutputShape"] = "bounded_validation_evidence"
		contract["allowedOutputs"] = contractViolationAllowedOutputsWithGovernanceBlockers("executionGraph_with_declared_validationEvidence", "executionGraph_with_managed_validation_session", "satisfied_dodAssessment_with_concrete_evidenceRefs")
		contract["requiredFields"] = []string{"validationEvidence.mode", "validationEvidence.observationType", "validationEvidence.targetRefs", "validationEvidence.observedClaims"}
		contract["forbiddenOutputs"] = append([]string{"failed_dodAssessment_only", "static_inspection_only", "detached_or_background_shell", "workspace_mutating_validation_shell", "write_outside_validationScratchDir"}, contractViolationForbiddenContinuableBlockers()...)
		if scratch := validationScratchDirFromStep(step); scratch != "" {
			contract[validationScratchDirKey] = scratch
		}
		return "concrete_validation_evidence", true
	}
	if boolValue(required["mustProduceConcreteFailureEvidence"]) {
		contract["requiredOutputShape"] = "bounded_failure_evidence"
		contract["allowedOutputs"] = contractViolationAllowedOutputsWithGovernanceBlockers("executionGraph_with_bounded_evidence_acquisition", "failed_dodAssessment_with_specific_non_recoverable_blocker")
		contract["requiredFields"] = []string{"executionGraph.nodes", "evidenceRefs_or_specific_blocker"}
		contract["forbiddenOutputs"] = append([]string{"failed_dodAssessment_only", "patch/apply", "patch/preview", "llm/workspace_execution", "deliverable_mutating_evidence_acquisition", "untraceable_summary_only"}, contractViolationForbiddenContinuableBlockers()...)
		if acceptedKinds := stringSliceFromValue(required["acceptedKinds"]); len(acceptedKinds) > 0 {
			contract["acceptedKinds"] = acceptedKinds
		}
		if forbiddenKinds := stringSliceFromValue(required["forbiddenKinds"]); len(forbiddenKinds) > 0 {
			contract["forbiddenKinds"] = forbiddenKinds
		}
		if boolValue(required["deliverableWriteForbidden"]) {
			contract["deliverableWriteForbidden"] = true
		}
		if scratch := validationScratchDirFromStep(step); scratch != "" {
			contract[validationScratchDirKey] = scratch
		}
		return "concrete_failure_evidence", true
	}
	return "", false
}

func contractViolationAllowedOutputsWithGovernanceBlockers(primary ...string) []string {
	ret := append([]string{}, primary...)
	return append(ret,
		"blocked_on_decision_with_required_external_dependency",
		"blocked_on_approval_with_required_governance_dependency",
	)
}

func contractViolationForbiddenContinuableBlockers() []string {
	return []string{
		"blocked_on_decision_for_continuable_candidate_choice",
		"decision_request_when_runtime_can_continue",
		"decision_request_for_candidate_selection_without_external_dependency",
	}
}

func contractViolationAvailableTargetSnapshotSummary(step *runModel.Step) []map[string]any {
	snapshots := contractViolationTargetSnapshotSummary(step)
	if len(snapshots) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if !boolValue(snapshot["contentComplete"]) || !boolValue(snapshot["safeForFileWrite"]) {
			continue
		}
		ret = append(ret, snapshot)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func contractViolationTargetSnapshotSummary(step *runModel.Step) []map[string]any {
	if step == nil {
		return nil
	}
	var ret []map[string]any
	add := func(item map[string]any) {
		if len(item) == 0 {
			return
		}
		targetRef := strings.TrimSpace(stringFromMap(item, "targetRef"))
		if targetRef == "" {
			return
		}
		summary := map[string]any{
			"targetRef":        targetRef,
			"contentHash":      stringFromMap(item, "contentHash"),
			"baseHash":         stringFromMap(item, "baseHash"),
			"contentComplete":  boolValue(item["contentComplete"]),
			"safeForFileWrite": boolValue(item["safeForFileWrite"]),
		}
		for key, value := range summary {
			if contractViolationEmptyValue(value) {
				delete(summary, key)
			}
		}
		ret = append(ret, summary)
	}
	for _, source := range []map[string]any{
		step.Input,
		mapFromValue(step.Input["repairActionPackage"]),
		mapFromValue(step.Input["patchRepairContext"]),
		mapFromValue(step.Input["failureTargetRepairContext"]),
	} {
		for _, item := range mapSliceFromValue(source["targetSnapshots"]) {
			add(item)
		}
		for _, item := range mapSliceFromValue(source["currentTargetSnapshots"]) {
			add(item)
		}
		for _, item := range mapSliceFromValue(source["patchRepairContext"]) {
			add(item)
		}
		for _, item := range mapSliceFromValue(source["failureTargetRepairContext"]) {
			add(item)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	if len(ret) > 6 {
		return ret[:6]
	}
	return ret
}

func contractViolationExpectedContract(step *runModel.Step) map[string]any {
	if step == nil {
		return nil
	}
	ret := map[string]any{
		"runtimeStage":                runtimeStageFromStepInput(step.Input),
		"runtimeStageMode":            stringFromMap(step.Input, "runtimeStageMode"),
		"repairObjective":             stringFromMap(step.Input, "repairObjective"),
		"stateChangingOutputRequired": boolValue(step.Input["stateChangingOutputRequired"]),
		"assessmentOnly":              boolValue(step.Input[stepInputKeyAssessmentOnly]),
		"providerRetryAssessmentOnly": boolValue(step.Input["providerRetryAssessmentOnly"]),
	}
	if pkg := mapFromValue(step.Input["repairActionPackage"]); len(pkg) > 0 {
		ret["repairActionPackage"] = compactLLMContextMap(pkg, 1)
		if required := mapFromValue(pkg["requiredOutput"]); len(required) > 0 {
			ret["requiredOutput"] = compactLLMContextMap(required, 1)
		}
	}
	for _, key := range []string{"requiredOutput", "outputMode", "artifactOutputPhase", "convergenceContract"} {
		if value, ok := step.Input[key]; ok {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	return ret
}

func contractViolationActualOutputSummary(step *runModel.Step, execErr error) map[string]any {
	ret := map[string]any{}
	if step == nil {
		return ret
	}
	output := step.Output
	for _, key := range []string{"status", "stage", "completionState", outputKeyContractViolationType, failureOutputKeyClass, failureOutputKeySignature} {
		if value := strings.TrimSpace(stringFromMap(output, key)); value != "" {
			ret[key] = value
		}
	}
	for _, key := range []string{"parseError", "repairError"} {
		if value := strings.TrimSpace(stringFromMap(output, key)); value != "" {
			ret[key] = truncateLLMContextString(value)
		}
	}
	if errText := firstNonEmptyString(errorString(execErr), stringFromMap(output, "error"), stringFromMap(output, "reason")); errText != "" {
		ret["error"] = truncateLLMContextString(errText)
	}
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
		ret["executionGraph"] = contractViolationGraphSummary(graph)
	}
	if assessment := doDAssessmentFromOutput(output); len(assessment) > 0 {
		ret["hasDoDAssessment"] = true
		ret["dodAssessmentSatisfied"] = boolValue(assessment["satisfied"])
		ret["dodAssessmentItemCount"] = len(mapSliceFromValue(assessment["items"]))
	}
	if spec := decisionBlockSpec(output); spec.Reason != "" {
		ret["hasDecisionBlock"] = true
	}
	if spec := approvalBlockSpec(output); spec.Reason != "" {
		ret["hasApprovalBlock"] = true
	}
	return ret
}

func contractViolationGraphSummary(graph runtimemodel.BlockExecutionGraph) map[string]any {
	kinds := []string{}
	ids := []string{}
	stateChanging := false
	for _, node := range graph.Nodes {
		if node.ID != "" {
			ids = append(ids, node.ID)
		}
		kind := canonicalRuntimeGraphNodeKind(node.Kind)
		if kind != "" {
			kinds = append(kinds, kind)
		}
		if graphNodeDeclaresStateChangingIntent(node) || stepKindCanProduceStateChangingEvidence(kind) {
			stateChanging = true
		}
	}
	return map[string]any{
		"nodeCount":     len(graph.Nodes),
		"nodeIds":       limitedStrings(compactUniqueStrings(ids), 12),
		"nodeKinds":     limitedStrings(compactUniqueStrings(kinds), 12),
		"stateChanging": stateChanging,
	}
}

func contractViolationSourceRefs(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	refs := []string{string(step.ID)}
	if refsFromOutput := stringSliceFromValue(step.Output["evidenceRefs"]); len(refsFromOutput) > 0 {
		refs = append(refs, refsFromOutput...)
	}
	if refsFromOutput := stringSliceFromValue(step.Output["llmCallRefs"]); len(refsFromOutput) > 0 {
		refs = append(refs, refsFromOutput...)
	}
	return limitedStrings(compactUniqueStrings(refs), 12)
}

func contractViolationMap(violation runtimemodel.ContractViolation) map[string]any {
	violation = violation.Normalized()
	ret := map[string]any{
		"id":                   violation.ID,
		"workspaceId":          string(violation.WorkspaceID),
		"runId":                string(violation.RunID),
		"stepId":               string(violation.StepID),
		"sourceStepId":         string(violation.SourceStepID),
		"violationType":        violation.ViolationType,
		"stage":                violation.Stage,
		"runtimeStage":         violation.RuntimeStage,
		"runtimeStageMode":     violation.RuntimeStageMode,
		"repairObjective":      violation.RepairObjective,
		"violatedRule":         violation.ViolatedRule,
		"signature":            violation.Signature,
		"expectedContract":     violation.ExpectedContract,
		"actualOutputSummary":  violation.ActualOutputSummary,
		"rejectedOutputRef":    violation.RejectedOutputRef,
		"failureClass":         violation.FailureClass,
		"failureSignature":     violation.FailureSignature,
		"recoverability":       violation.Recoverability,
		"retryHint":            violation.RetryHint,
		"forbiddenRepeat":      violation.ForbiddenRepeat,
		"correctionProfileId":  violation.CorrectionProfileID,
		"correctionMode":       violation.CorrectionMode,
		"correctionContract":   violation.CorrectionContract,
		"targetScope":          violation.TargetScope,
		"budgetScope":          violation.BudgetScope,
		"sourceRefs":           violation.SourceRefs,
		"attempt":              violation.Attempt,
		"maxAttempts":          violation.MaxAttempts,
		"signatureRepeatCount": violation.SignatureRepeatCount,
		"createdAt":            violation.CreatedAt,
	}
	if len(mapFromValue(ret["expectedContract"])) > 0 {
		ret["expectedContractRole"] = "diagnostic_lineage"
		ret["expectedContractAuthoritative"] = false
	}
	for key, value := range ret {
		if contractViolationEmptyValue(value) {
			delete(ret, key)
		}
	}
	return ret
}

func contractViolationSignature(violationType string, rule string, failureSignature string) string {
	parts := []string{}
	if violationType = strings.TrimSpace(violationType); violationType != "" {
		parts = append(parts, "type="+violationType)
	}
	if rule = strings.TrimSpace(rule); rule != "" {
		parts = append(parts, "rule="+rule)
	}
	if failureSignature = strings.TrimSpace(failureSignature); failureSignature != "" {
		parts = append(parts, "failure="+failureSignature)
	}
	return strings.Join(parts, "|")
}

func contractViolationEmptyValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case int:
		return typed == 0
	case []string:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}
