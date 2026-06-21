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
	contractViolationRuleRuntimeContract               = "runtime_contract_violation"

	stepOutputKeyContractViolation        = "contractViolation"
	stepInputKeyContractViolationRecovery = "contractViolationRecovery"
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
	violation["attempt"] = attempt
	violation["maxAttempts"] = maxAttempts
	violation["signatureRepeatCount"] = signatureRepeatCount
	input[stepInputKeyContractViolationRecovery] = violation
	input["contractViolationRetry"] = true
	input[stepInputKeyRecoveryLineageType] = "contract_violation_retry"
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"The previous LLM output was rejected by the runtime contract before any side effects were executed.",
		"Use contractViolationRecovery.expectedContract as the required output contract and do not repeat contractViolationRecovery.actualOutputSummary.",
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
		return contractViolationRuleRuntimeContract
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
	default:
		return []string{"same rejected output shape"}
	}
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
