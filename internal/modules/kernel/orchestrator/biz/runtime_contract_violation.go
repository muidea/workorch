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
	contractViolationRuleRequiredOutputMissing         = "required_output_missing"
	contractViolationRuleWrongOutputShape              = "wrong_output_shape"
	contractViolationRuleAcceptedKindMismatch          = "accepted_kind_mismatch"
	contractViolationRuleStateChangingRequiredMissing  = "state_changing_required_missing"
	contractViolationRuleConcreteValidationMissing     = "concrete_validation_contract_missing"
	contractViolationRuleDoDAssessmentEvidenceMissing  = "dod_assessment_missing_evidence_refs"
	contractViolationRuleCoverageMapIncomplete         = "coverage_map_incomplete"
	contractViolationRuleTraceabilityRefMismatch       = "traceability_ref_mismatch"
	contractViolationRuleDecisionBlockedWhenActionable = "decision_blocked_when_actionable"
	contractViolationRuleUnsafeOutput                  = "unsafe_output"
	contractViolationRuleUnparseableOutput             = "unparseable_output"
	contractViolationRulePatchApplyContractFailed      = "patch_apply_contract_failed"
	contractViolationRuleAvailableSnapshotIgnored      = "available_snapshot_ignored"
	contractViolationRuleRuntimeContract               = "runtime_contract_violation"

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
	return runtimeContractViolationError{
		cause:          fmt.Errorf("%w: %s", common.ErrInvalidArgument, strings.TrimSpace(message)),
		violationType:  strings.TrimSpace(violationType),
		rule:           contractViolationRuleFromType(violationType),
		recoverability: contractViolationRecoverabilityForRule(contractViolationRuleFromType(violationType)),
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
	if run == nil || step == nil || strings.TrimSpace(step.Kind) != llmWorkspaceExecutionStepKind {
		return runtimemodel.ContractViolation{}, false
	}
	output := step.Output
	violationType := strings.TrimSpace(stringFromMap(output, outputKeyContractViolationType))
	class := strings.TrimSpace(stringFromMap(output, failureOutputKeyClass))
	if violationType == "" && class != failureClassDoDCoverageMissing && !runtimeFailureIsLLMContractLoop(step) {
		return runtimemodel.ContractViolation{}, false
	}
	rule := contractViolationRuleFromStep(step)
	recoverability := contractViolationRecoverabilityForRule(rule)
	correction := contractViolationCorrectionProfileForStep(step, rule, attempt, signatureRepeatCount)
	violation := runtimemodel.ContractViolation{
		ID:                   fmt.Sprintf("%s/%s/contract-violation", run.ID, step.ID),
		WorkspaceID:          run.WorkspaceID,
		RunID:                run.ID,
		StepID:               step.ID,
		SourceStepID:         step.ID,
		Stage:                firstNonEmptyString(stringFromMap(output, "stage"), stringFromMap(mapFromValue(output["runtimeDiagnostics"]), "kind")),
		RuntimeStage:         runtimeStageFromStepInput(step.Input),
		RuntimeStageMode:     stringFromMap(step.Input, "runtimeStageMode"),
		RepairObjective:      stringFromMap(step.Input, "repairObjective"),
		ViolatedRule:         rule,
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
		SourceRefs:           contractViolationSourceRefs(step),
		Attempt:              attempt,
		MaxAttempts:          maxAttempts,
		SignatureRepeatCount: signatureRepeatCount,
		CreatedAt:            runtimeEvidenceTimestamp(),
	}
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
	}
	input[stepInputKeyContractViolationRecovery] = violation
	input["contractViolationRetry"] = true
	input[stepInputKeyRecoveryLineageType] = "contract_violation_retry"
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"The previous LLM output was rejected by the runtime contract before any side effects were executed.",
		"Use contractViolationRecovery.expectedContract as the required output contract and do not repeat contractViolationRecovery.actualOutputSummary.",
		"Use contractViolationRecovery.correctionContract as the current narrow retry contract; if correctionMode is forced_narrow, return only an allowed output shape from that contract or a real governance blocker.",
		"Return one valid JSON decision that satisfies the same runtimeStage and repairObjective; do not weaken Goal, DoD, evidence, or approval requirements.",
	)
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
	switch strings.TrimSpace(stringFromMap(violation, "recoverability")) {
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

func contractViolationRuleFromStep(step *runModel.Step) string {
	if step == nil {
		return contractViolationRuleRuntimeContract
	}
	if contractViolationOutputClaimsMissingAvailableSnapshot(step) {
		return contractViolationRuleAvailableSnapshotIgnored
	}
	output := step.Output
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
		return contractViolationRuleDoDAssessmentEvidenceMissing
	}
	if strings.TrimSpace(stringFromMap(output, "stage")) == "llm_schema_invalid" {
		return contractViolationRuleUnparseableOutput
	}
	return contractViolationRuleRuntimeContract
}

func contractViolationRecoverabilityForRule(rule string) string {
	switch strings.TrimSpace(rule) {
	case contractViolationRuleUnsafeOutput:
		return runtimemodel.ContractViolationRecoverabilityFailFast
	case contractViolationRuleAcceptedKindMismatch:
		return runtimemodel.ContractViolationRecoverabilityNarrowContract
	case contractViolationRuleTraceabilityRefMismatch:
		return runtimemodel.ContractViolationRecoverabilityNormalizeThenRetry
	default:
		return runtimemodel.ContractViolationRecoverabilityRetrySameContract
	}
}

func contractViolationRetryHint(rule string) string {
	switch strings.TrimSpace(rule) {
	case contractViolationRuleStateChangingRequiredMissing:
		return "return output that satisfies stateChangingOutputRequired and requiredOutput without repeating a read-only graph"
	case contractViolationRuleConcreteValidationMissing:
		return "return bounded concrete validation evidence with validationEvidence mode and observation metadata"
	case contractViolationRuleDoDAssessmentEvidenceMissing, contractViolationRuleCoverageMapIncomplete:
		return "rewrite the assessment so every satisfied item has traceable evidenceRefs and current coverage"
	case contractViolationRuleUnparseableOutput:
		return "return one complete valid JSON decision object only"
	case contractViolationRulePatchApplyContractFailed:
		return "return a valid patch/apply output that satisfies targetRef, patchType, content/diff, baseHash/snapshot, and current requiredOutput"
	case contractViolationRuleAvailableSnapshotIgnored:
		return "use the available complete target snapshots already supplied in the current action instead of claiming they are missing or returning another read-only acquisition graph"
	default:
		return "return a valid output that satisfies expectedContract and does not repeat actualOutputSummary"
	}
}

func contractViolationForbiddenRepeat(rule string) []string {
	switch strings.TrimSpace(rule) {
	case contractViolationRuleStateChangingRequiredMissing:
		return []string{"read-only graph without accepted state-changing evidence", "dodAssessment without required executable output"}
	case contractViolationRuleConcreteValidationMissing:
		return []string{"validation graph without validationEvidence contract", "detached or unbounded validation command"}
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
	targetScope := runtimeFailureTargetScopeFromStep(step)
	contract := map[string]any{
		"violatedRule":     rule,
		"mode":             mode,
		"preserveContract": true,
		"mustNotWeaken":    []string{"runtimeStage", "repairObjective", "requiredOutput", "Goal", "DoD", "evidence", "approval"},
	}
	profileID := "contract_correction_generic"
	switch rule {
	case contractViolationRuleStateChangingRequiredMissing:
		profileID = "state_changing_required_output"
		contract["requiredOutputShape"] = "state_changing_output"
		contract["allowedOutputs"] = contractViolationAllowedOutputsWithGovernanceBlockers("executionGraph_with_state_changing_node", "patch/apply", "accepted_state_changing_tool_or_shell", "incremental_artifact_state_change")
		contract["forbiddenOutputs"] = append([]string{"read_only_executionGraph", "failed_dodAssessment_only", "same_actualOutputSummary"}, contractViolationForbiddenContinuableBlockers()...)
		contract["mustSatisfy"] = []string{"stateChangingOutputRequired", "repairActionPackage.requiredOutput"}
	case contractViolationRuleConcreteValidationMissing:
		profileID = "concrete_validation_evidence"
		contract["requiredOutputShape"] = "bounded_validation_evidence"
		contract["allowedOutputs"] = contractViolationAllowedOutputsWithGovernanceBlockers("executionGraph_with_declared_validationEvidence", "satisfied_dodAssessment_with_concrete_evidenceRefs")
		contract["requiredFields"] = []string{"validationEvidence.mode", "validationEvidence.observationType"}
		contract["forbiddenOutputs"] = append([]string{"static_inspection_only", "detached_or_background_shell", "workspace_mutating_validation_shell", "write_outside_validationScratchDir"}, contractViolationForbiddenContinuableBlockers()...)
		if scratch := validationScratchDirFromStep(step); scratch != "" {
			contract[validationScratchDirKey] = scratch
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
		profileID = "json_output_shape"
		contract["requiredOutputShape"] = "single_valid_json_decision"
		contract["forbiddenOutputs"] = []string{"markdown_wrapped_json", "partial_json", "multiple_json_objects", "side_effects"}
	default:
		contract["requiredOutputShape"] = "expected_contract_compliant_output"
		contract["forbiddenOutputs"] = []string{"same_actualOutputSummary"}
	}
	if mode == contractViolationCorrectionModeForcedNarrow {
		contract["forcedNarrow"] = true
		contract["forcedNarrowRule"] = "same-rule or same-target violation repeated; return only an allowed output shape from this correctionContract or a real governance blocker"
	}
	return map[string]any{
		"profileId":   profileID,
		"mode":        mode,
		"targetScope": targetScope,
		"contract":    contract,
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
	if boolValue(step.Input["stateChangingOutputRequired"]) || boolValue(required["mustIncludeStateChangingNode"]) {
		contract["requiredOutputShape"] = "state_changing_output"
		contract["allowedOutputs"] = contractViolationAllowedOutputsWithGovernanceBlockers("executionGraph_with_state_changing_node", "patch/apply", "accepted_state_changing_tool_or_shell", "incremental_artifact_state_change")
		contract["forbiddenOutputs"] = append([]string{"failed_dodAssessment_only", "read_only_executionGraph", "same_actualOutputSummary"}, contractViolationForbiddenContinuableBlockers()...)
		contract["mustSatisfy"] = []string{"stateChangingOutputRequired", "repairActionPackage.requiredOutput"}
		return "state_changing_required_output", true
	}
	if boolValue(required["mustProduceConcreteValidationEvidence"]) {
		contract["requiredOutputShape"] = "bounded_validation_evidence"
		contract["allowedOutputs"] = contractViolationAllowedOutputsWithGovernanceBlockers("executionGraph_with_declared_validationEvidence", "satisfied_dodAssessment_with_concrete_evidenceRefs")
		contract["requiredFields"] = []string{"validationEvidence.mode", "validationEvidence.observationType"}
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

func contractViolationOutputClaimsMissingAvailableSnapshot(step *runModel.Step) bool {
	if len(contractViolationAvailableTargetSnapshotSummary(step)) == 0 {
		return false
	}
	text := strings.ToLower(strings.Join(contractViolationOutputTextFragments(step.Output, 0), "\n"))
	if strings.TrimSpace(text) == "" {
		return false
	}
	return contractViolationTextMentionsSnapshotOrCurrentTarget(text) && contractViolationTextClaimsMissingSnapshot(text)
}

func contractViolationTextMentionsSnapshotOrCurrentTarget(text string) bool {
	for _, marker := range []string{
		"snapshot",
		"current target",
		"current content",
		"target snapshot",
		"target content",
		"快照",
		"当前目标",
		"当前内容",
		"目标文本",
		"目标快照",
		"当前诊断",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func contractViolationTextClaimsMissingSnapshot(text string) bool {
	for _, marker := range []string{
		"missing",
		"not available",
		"unavailable",
		"not provided",
		"not supplied",
		"need to read",
		"needs to read",
		"must read",
		"requires read",
		"cannot proceed without",
		"no current",
		"缺少",
		"缺失",
		"未提供",
		"没有",
		"无法",
		"不能",
		"需要读取",
		"需读取",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func contractViolationOutputTextFragments(value any, depth int) []string {
	if depth > 4 || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			return []string{trimmed}
		}
	case map[string]any:
		ret := []string{}
		for _, item := range typed {
			ret = append(ret, contractViolationOutputTextFragments(item, depth+1)...)
			if len(ret) > 80 {
				return ret[:80]
			}
		}
		return ret
	case []any:
		ret := []string{}
		for _, item := range typed {
			ret = append(ret, contractViolationOutputTextFragments(item, depth+1)...)
			if len(ret) > 80 {
				return ret[:80]
			}
		}
		return ret
	case []map[string]any:
		ret := []string{}
		for _, item := range typed {
			ret = append(ret, contractViolationOutputTextFragments(item, depth+1)...)
			if len(ret) > 80 {
				return ret[:80]
			}
		}
		return ret
	case []string:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				ret = append(ret, trimmed)
			}
		}
		return ret
	}
	return nil
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
	ret := map[string]any{
		"id":                   violation.ID,
		"workspaceId":          string(violation.WorkspaceID),
		"runId":                string(violation.RunID),
		"stepId":               string(violation.StepID),
		"sourceStepId":         string(violation.SourceStepID),
		"stage":                violation.Stage,
		"runtimeStage":         violation.RuntimeStage,
		"runtimeStageMode":     violation.RuntimeStageMode,
		"repairObjective":      violation.RepairObjective,
		"violatedRule":         violation.ViolatedRule,
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
		"sourceRefs":           violation.SourceRefs,
		"attempt":              violation.Attempt,
		"maxAttempts":          violation.MaxAttempts,
		"signatureRepeatCount": violation.SignatureRepeatCount,
		"createdAt":            violation.CreatedAt,
	}
	for key, value := range ret {
		if contractViolationEmptyValue(value) {
			delete(ret, key)
		}
	}
	return ret
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
