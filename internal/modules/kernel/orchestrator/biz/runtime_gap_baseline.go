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
	gapBaselineKey             = "gapBaseline"
	gapBaselineSchema          = "workorch.gap_baseline.v1"
	convergenceAuthorityKey    = "convergenceAuthority"
	convergenceAuthoritySchema = "workorch.convergence_authority.v1"
)

func gapBaselineMapFromValue(value any) map[string]any {
	return schemaBoundRuntimeAuthorityMap(value, gapBaselineSchema)
}

func convergenceAuthorityMapFromValue(value any) map[string]any {
	return schemaBoundRuntimeAuthorityMap(value, convergenceAuthoritySchema)
}

func schemaBoundRuntimeAuthorityMap(value any, expectedSchema string) map[string]any {
	ret := mapFromValue(value)
	if len(ret) == 0 {
		return nil
	}
	schema := strings.TrimSpace(stringFromMap(ret, "schemaVersion"))
	if schema != "" && schema != expectedSchema {
		return nil
	}
	return ret
}

type ConvergenceAuthoritySnapshot struct {
	SchemaVersion           string
	WorkspaceID             string
	RunID                   string
	DefinitionRevision      string
	BaselineRevision        string
	CurrentFocus            map[string]any
	NextAction              map[string]any
	CoverageWorkItemContext map[string]any
	AcceptedEvidenceRefs    []string
	RejectedEvidenceRefs    []string
	BudgetScope             map[string]any
}

func (s ConvergenceAuthoritySnapshot) ToMap() map[string]any {
	ret := map[string]any{
		"schemaVersion": s.SchemaVersion,
		"workspaceId":   s.WorkspaceID,
		"runId":         s.RunID,
	}
	if ret["schemaVersion"] == "" {
		ret["schemaVersion"] = convergenceAuthoritySchema
	}
	if strings.TrimSpace(s.DefinitionRevision) != "" {
		ret["definitionRevision"] = s.DefinitionRevision
	}
	if strings.TrimSpace(s.BaselineRevision) != "" {
		ret["baselineRevision"] = s.BaselineRevision
	}
	if len(s.CurrentFocus) > 0 {
		ret["currentFocus"] = cloneStepPayload(s.CurrentFocus)
	}
	if len(s.NextAction) > 0 {
		ret["nextAction"] = cloneStepPayload(s.NextAction)
	}
	if len(s.CoverageWorkItemContext) > 0 {
		ret[coverageWorkItemContextKey] = cloneStepPayload(s.CoverageWorkItemContext)
	}
	if len(s.AcceptedEvidenceRefs) > 0 {
		ret["acceptedEvidenceRefs"] = compactUniqueStrings(s.AcceptedEvidenceRefs)
	}
	if len(s.RejectedEvidenceRefs) > 0 {
		ret["rejectedEvidenceRefs"] = compactUniqueStrings(s.RejectedEvidenceRefs)
	}
	if len(s.BudgetScope) > 0 {
		ret["budgetScope"] = cloneStepPayload(s.BudgetScope)
	}
	return ret
}

func (o *RuntimeOrchestrator) ensureGapBaselineBeforeDispatch(ctx context.Context, run *runModel.Run) error {
	if o == nil || run == nil {
		return common.ErrInvalidArgument
	}
	if run.ExecutionContext == nil {
		run.ExecutionContext = map[string]any{}
	}
	changed, err := o.ensureCoverageLedger(ctx, run)
	if err != nil {
		return err
	}
	if refreshRunConvergenceAuthority(run) {
		changed = true
	}
	baseline := gapBaselineFromRun(run)
	current := mapFromValue(run.ExecutionContext[gapBaselineKey])
	switch {
	case len(baseline) == 0:
		if _, ok := run.ExecutionContext[gapBaselineKey]; ok {
			delete(run.ExecutionContext, gapBaselineKey)
			changed = true
		}
	case stableCoverageJSON(current) != stableCoverageJSON(baseline):
		run.ExecutionContext[gapBaselineKey] = baseline
		changed = true
	}
	if syncRunConvergenceDerivedFields(run) {
		changed = true
	}
	if changed && o.runs != nil {
		return o.persistRunProgressUpdate(ctx, run)
	}
	return nil
}

func (o *RuntimeOrchestrator) prepareGapBaselineDispatchSpecs(ctx context.Context, run *runModel.Run, specs []stepDispatchSpec) ([]stepDispatchSpec, error) {
	if len(specs) == 0 {
		return specs, nil
	}
	if err := o.ensureGapBaselineBeforeDispatch(ctx, run); err != nil {
		return nil, err
	}
	if gated, err := gapBaselineDispatchGate(run); gated || err != nil {
		return nil, err
	}
	context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])
	baseline := gapBaselineMapFromValue(run.ExecutionContext[gapBaselineKey])
	authority := convergenceAuthorityMapFromValue(run.ExecutionContext[convergenceAuthorityKey])
	if len(context) == 0 && len(baseline) == 0 && len(authority) == 0 {
		return specs, nil
	}
	focus := mapFromValue(run.ExecutionContext[coverageFocusKey])
	ret := make([]stepDispatchSpec, len(specs))
	for idx, spec := range specs {
		ret[idx] = spec
		input := cloneStepPayload(spec.Input)
		if input == nil {
			input = map[string]any{}
		}
		recoveryScoped := dispatchInputHasRecoveryAuthority(input)
		if len(context) > 0 && !recoveryScoped {
			if existing := mapFromValue(input[coverageWorkItemContextKey]); len(existing) > 0 {
				input[coverageWorkItemContextKey] = coverageWorkItemContextAlignedToFocus(existing, focus)
			} else {
				input[coverageWorkItemContextKey] = coverageWorkItemContextAlignedToFocus(context, focus)
			}
		}
		if len(baseline) > 0 && !recoveryScoped {
			input[gapBaselineKey] = cloneStepPayload(baseline)
		}
		if len(authority) > 0 && !recoveryScoped {
			input[convergenceAuthorityKey] = cloneStepPayload(authority)
		}
		if canonicalLLMWorkspaceStepKind(ret[idx].Kind) == llmWorkspaceExecutionStepKind {
			var err error
			input, err = rebuildDispatchInputFromConvergenceAuthority(run, ret[idx].ID, input)
			if err != nil {
				return nil, err
			}
		}
		ret[idx].Input = input
	}
	return ret, nil
}

func dispatchInputHasRecoveryAuthority(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	return boolValue(input[stepInputKeyRuntimeRepairFollowUp]) ||
		boolValue(input[stepInputKeyProviderRetryFollowUp]) ||
		boolValue(input["graphLocalDispatchAuthority"]) ||
		strings.TrimSpace(stringFromMap(input, stepInputKeyRecoveryAction)) != "" ||
		strings.TrimSpace(stringFromMap(input, stepInputKeyRepairOfStepID)) != "" ||
		len(mapFromValue(input[stepInputKeyContractViolationRecovery])) > 0
}

func syncRunConvergenceDerivedFields(run *runModel.Run) bool {
	if run == nil || run.ExecutionContext == nil {
		return false
	}
	if len(convergenceAuthorityMapFromValue(run.ExecutionContext[convergenceAuthorityKey])) == 0 &&
		len(gapBaselineMapFromValue(run.ExecutionContext[gapBaselineKey])) == 0 {
		return false
	}
	input := runExecutionInput(run, "")
	if len(input) == 0 {
		return false
	}
	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)
	input = ApplyConvergenceContract(input, contract)
	input = normalizeDispatchInputAuthorityFocus(input)
	input = attachStagePacketToStepInput(run, "", input)
	input = enforceCurrentAuthorityDerivedConsistency(run, "", input)
	changed := false
	for _, key := range convergenceDerivedExecutionContextKeys() {
		value, ok := input[key]
		if !ok || value == nil {
			if _, exists := run.ExecutionContext[key]; exists {
				delete(run.ExecutionContext, key)
				changed = true
			}
			continue
		}
		if stableCoverageJSON(mapFromValueOrScalar(run.ExecutionContext[key])) == stableCoverageJSON(mapFromValueOrScalar(value)) {
			continue
		}
		run.ExecutionContext[key] = cloneStepInputValue(value)
		changed = true
	}
	return changed
}

func normalizeRunConvergenceOwnerState(run *runModel.Run) bool {
	if run == nil || run.ExecutionContext == nil {
		return false
	}
	changed := false
	if refreshRunConvergenceAuthority(run) {
		changed = true
	}
	if syncRunConvergenceDerivedFields(run) {
		changed = true
	}
	return changed
}

func convergenceDerivedExecutionContextKeys() []string {
	return []string{
		"convergenceContract",
		stepInputKeyStagePacket,
		coverageFocusKey,
		stepInputKeySoulStage,
		stepInputKeyRuntimeStage,
		stepInputKeyRuntimeStageMode,
		"repairObjective",
		"stateChangingOutputRequired",
		"concreteFailureEvidenceRequired",
		"boundedValidationRequired",
		"evidenceRepairRequired",
		stepInputKeyAssessmentOnly,
		"assessmentOnlyReason",
		"acceptance",
		"repairActionPackage",
		"requiredOutput",
		currentActionContractKey,
		"coverageRepairRequired",
		"coverageTargetSet",
		"governanceRequired",
		"governanceReason",
	}
}

func mapFromValueOrScalar(value any) map[string]any {
	if mapped := mapFromValue(value); len(mapped) > 0 {
		return mapped
	}
	if value == nil {
		return nil
	}
	return map[string]any{"value": value}
}

func rebuildDispatchInputFromConvergenceAuthority(run *runModel.Run, stepID common.StepID, input map[string]any) (map[string]any, error) {
	if input == nil {
		input = map[string]any{}
	}
	if dispatchInputUsesLocalAssessmentContract(run, input) {
		input = stripDispatchInputOwnerAuthority(input)
		input["convergenceContract"] = compactConvergenceContract(ConvergenceContract{
			State:            convergenceStateNeedAssessment,
			RuntimeStage:     runtimeStageVerification,
			RuntimeStageMode: "strict_convergence",
			RepairObjective:  "complete_dod_goal_coverage_assessment",
		})
		clearExecutableCoverageContinuationContract(input)
		input = attachStagePacketToStepInput(run, stepID, input)
		input = synchronizeCurrentRequiredOutputMirrors(input)
		input = attachCurrentActionContractToInput(run, stepID, input)
		input = scrubConvergenceDispatchDiagnosticAdvice(input)
		return input, validateConvergenceDispatchInput(input)
	}
	if dispatchInputHasStaleLocalAssessmentContract(run, input) {
		input = clearStaleLocalAssessmentContract(input)
	}
	input = injectLatestRunAuthorityForDispatch(run, input)
	if !dispatchInputHasConvergenceAuthority(run, input) {
		input = attachStagePacketToStepInput(run, stepID, input)
		input = synchronizeCurrentRequiredOutputMirrors(input)
		input = attachCurrentActionContractToInput(run, stepID, input)
		input = scrubConvergenceDispatchDiagnosticAdvice(input)
		return input, validateConvergenceDispatchInput(input)
	}
	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)
	input = ApplyConvergenceContract(input, contract)
	input = normalizeDispatchInputAuthorityFocus(input)
	input = attachStagePacketToStepInput(run, stepID, input)
	input = synchronizeCurrentRequiredOutputMirrors(input)
	input = attachCurrentActionContractToInput(run, stepID, input)
	input = enforceCurrentAuthorityDerivedConsistency(run, stepID, input)
	input = scrubConvergenceDispatchDiagnosticAdvice(input)
	if err := validateConvergenceDispatchInput(input); err != nil {
		return nil, err
	}
	return input, nil
}

func finalizeLLMDispatchAuthorityInput(run *runModel.Run, stepID common.StepID, input map[string]any) (map[string]any, error) {
	if input == nil {
		input = map[string]any{}
	}
	if dispatchInputHasConvergenceAuthority(run, input) {
		return rebuildDispatchInputFromConvergenceAuthority(run, stepID, input)
	}
	input = attachStagePacketToStepInput(run, stepID, input)
	input = synchronizeCurrentRequiredOutputMirrors(input)
	input = attachCurrentActionContractToInput(run, stepID, input)
	input = scrubConvergenceDispatchDiagnosticAdvice(input)
	if err := validateConvergenceDispatchInput(input); err != nil {
		return nil, err
	}
	return input, nil
}

func dispatchInputUsesLocalAssessmentContract(run *runModel.Run, input map[string]any) bool {
	if len(input) == 0 || !boolValue(input[stepInputKeyAssessmentOnly]) {
		return false
	}
	if !boolValue(input[stepInputKeyGraphLimitFollowUp]) &&
		!boolValue(input["assessmentOnlyContractCorrection"]) {
		return false
	}
	switch latestRunAuthorityNextActionKind(run) {
	case "", "assessment", "complete_coverage_assessment":
		return true
	default:
		appendAuthorityDiagnostic(input, "stale_local_assessment_contract_ignored", "warning", "diagnostic_lineage", "assessmentOnly", "local assessment retry was ignored because latest owner authority requires a different next action")
		return false
	}
}

func dispatchInputHasStaleLocalAssessmentContract(run *runModel.Run, input map[string]any) bool {
	if len(input) == 0 || !boolValue(input[stepInputKeyAssessmentOnly]) {
		return false
	}
	if !boolValue(input[stepInputKeyGraphLimitFollowUp]) &&
		!boolValue(input["assessmentOnlyContractCorrection"]) {
		return false
	}
	switch latestRunAuthorityNextActionKind(run) {
	case "", "assessment", "complete_coverage_assessment":
		return false
	default:
		return true
	}
}

func clearStaleLocalAssessmentContract(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	delete(input, stepInputKeyAssessmentOnly)
	delete(input, "assessmentOnlyReason")
	delete(input, "assessmentOnlyContractCorrection")
	delete(input, stepInputKeyGraphLimitFollowUp)
	if contract := mapFromValue(input["convergenceContract"]); strings.TrimSpace(stringFromMap(contract, "state")) == convergenceStateNeedAssessment {
		delete(input, "convergenceContract")
	}
	if strings.TrimSpace(stringFromMap(input, "repairObjective")) == "complete_dod_goal_coverage_assessment" {
		delete(input, "repairObjective")
	}
	delete(input, currentActionContractKey)
	delete(input, stepInputKeyStagePacket)
	return input
}

func injectLatestRunAuthorityForDispatch(run *runModel.Run, input map[string]any) map[string]any {
	if input == nil {
		input = map[string]any{}
	}
	if run == nil || run.ExecutionContext == nil {
		return input
	}
	if authority := convergenceAuthorityMapFromValue(run.ExecutionContext[convergenceAuthorityKey]); len(authority) > 0 {
		input[convergenceAuthorityKey] = cloneStepPayload(authority)
	}
	if baseline := gapBaselineMapFromValue(run.ExecutionContext[gapBaselineKey]); len(baseline) > 0 {
		input[gapBaselineKey] = cloneStepPayload(baseline)
	}
	if context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]); len(context) > 0 {
		input[coverageWorkItemContextKey] = cloneStepPayload(context)
	}
	if snapshots := mapSliceFromValue(run.ExecutionContext["targetSnapshots"]); len(snapshots) > 0 {
		input["targetSnapshots"] = cloneStepInputValue(snapshots)
	}
	if snapshots := mapSliceFromValue(run.ExecutionContext["currentTargetSnapshots"]); len(snapshots) > 0 {
		input["currentTargetSnapshots"] = cloneStepInputValue(snapshots)
	}
	if runPkg := mapFromValue(run.ExecutionContext["repairActionPackage"]); len(runPkg) > 0 {
		pkg := mapFromValue(input["repairActionPackage"])
		if len(pkg) == 0 {
			pkg = map[string]any{}
		}
		if snapshots := mapSliceFromValue(runPkg["targetSnapshots"]); len(snapshots) > 0 {
			pkg["targetSnapshots"] = cloneStepInputValue(snapshots)
		}
		if snapshots := mapSliceFromValue(runPkg["currentTargetSnapshots"]); len(snapshots) > 0 {
			pkg["currentTargetSnapshots"] = cloneStepInputValue(snapshots)
		}
		if len(pkg) > 0 {
			input["repairActionPackage"] = pkg
		}
	}
	return input
}

func latestRunAuthorityNextActionKind(run *runModel.Run) string {
	if run == nil || run.ExecutionContext == nil {
		return ""
	}
	for _, source := range []map[string]any{
		convergenceAuthorityMapFromValue(run.ExecutionContext[convergenceAuthorityKey]),
		gapBaselineMapFromValue(run.ExecutionContext[gapBaselineKey]),
	} {
		if kind := strings.TrimSpace(stringFromMap(mapFromValue(source["nextAction"]), "kind")); kind != "" {
			return kind
		}
	}
	return ""
}

func stripDispatchInputOwnerAuthority(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	delete(input, convergenceAuthorityKey)
	delete(input, gapBaselineKey)
	if executionContext := mapFromValue(input["executionContext"]); len(executionContext) > 0 {
		executionContext = cloneStepPayload(executionContext)
		delete(executionContext, convergenceAuthorityKey)
		delete(executionContext, gapBaselineKey)
		input["executionContext"] = executionContext
	}
	return input
}

func synchronizeCurrentRequiredOutputMirrors(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	required := activeRequiredOutputForCurrentAction(input)
	if len(required) == 0 {
		delete(input, "requiredOutput")
		if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
			delete(pkg, "requiredOutput")
			input["repairActionPackage"] = pkg
		}
		if packet := mapFromValue(input[stepInputKeyStagePacket]); len(packet) > 0 {
			delete(packet, "requiredOutput")
			if nextAction := mapFromValue(packet["nextAction"]); len(nextAction) > 0 {
				delete(nextAction, "requiredOutput")
				packet["nextAction"] = nextAction
			}
			input[stepInputKeyStagePacket] = packet
		}
		if executionContext := mapFromValue(input["executionContext"]); len(executionContext) > 0 {
			delete(executionContext, "requiredOutput")
			if pkg := mapFromValue(executionContext["repairActionPackage"]); len(pkg) > 0 {
				delete(pkg, "requiredOutput")
				executionContext["repairActionPackage"] = pkg
			}
			if packet := mapFromValue(executionContext[stepInputKeyStagePacket]); len(packet) > 0 {
				delete(packet, "requiredOutput")
				if nextAction := mapFromValue(packet["nextAction"]); len(nextAction) > 0 {
					delete(nextAction, "requiredOutput")
					packet["nextAction"] = nextAction
				}
				executionContext[stepInputKeyStagePacket] = packet
			}
			input["executionContext"] = executionContext
		}
		return input
	}
	required = cloneStepPayload(required)
	appendRequiredOutputMirrorReboundDiagnostics(input, required)
	input["requiredOutput"] = cloneStepPayload(required)
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	pkg["requiredOutput"] = cloneStepPayload(required)
	input["repairActionPackage"] = pkg
	if packet := mapFromValue(input[stepInputKeyStagePacket]); len(packet) > 0 {
		packet["requiredOutput"] = cloneStepPayload(required)
		if nextAction := mapFromValue(packet["nextAction"]); len(nextAction) > 0 {
			nextAction["requiredOutput"] = cloneStepPayload(required)
			packet["nextAction"] = nextAction
		}
		input[stepInputKeyStagePacket] = packet
	}
	if executionContext := mapFromValue(input["executionContext"]); len(executionContext) > 0 {
		executionContext["requiredOutput"] = cloneStepPayload(required)
		if contextPkg := mapFromValue(executionContext["repairActionPackage"]); len(contextPkg) > 0 {
			contextPkg["requiredOutput"] = cloneStepPayload(required)
			executionContext["repairActionPackage"] = contextPkg
		}
		if contextPacket := mapFromValue(executionContext[stepInputKeyStagePacket]); len(contextPacket) > 0 {
			contextPacket["requiredOutput"] = cloneStepPayload(required)
			if nextAction := mapFromValue(contextPacket["nextAction"]); len(nextAction) > 0 {
				nextAction["requiredOutput"] = cloneStepPayload(required)
				contextPacket["nextAction"] = nextAction
			}
			executionContext[stepInputKeyStagePacket] = contextPacket
		}
		input["executionContext"] = executionContext
	}
	return input
}

func appendRequiredOutputMirrorReboundDiagnostics(input map[string]any, required map[string]any) {
	if len(input) == 0 || len(required) == 0 {
		return
	}
	expected := stableCoverageJSON(required)
	for label, existing := range map[string]map[string]any{
		"requiredOutput":                        mapFromValue(input["requiredOutput"]),
		"currentActionContract.requiredOutput":  mapFromValue(mapFromValue(input[currentActionContractKey])["requiredOutput"]),
		"stagePacket.requiredOutput":            mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		"stagePacket.nextAction.requiredOutput": mapFromValue(mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["nextAction"])["requiredOutput"]),
	} {
		if len(existing) == 0 || stableCoverageJSON(existing) == expected {
			continue
		}
		appendAuthorityDiagnostic(input, "stale_required_output_mirror_rebound", "warning", "diagnostic_lineage", label, "stale requiredOutput mirror was rebound from current convergence authority before dispatch")
	}
	if executionContext := mapFromValue(input["executionContext"]); len(executionContext) > 0 {
		for label, existing := range map[string]map[string]any{
			"executionContext.requiredOutput":                        mapFromValue(executionContext["requiredOutput"]),
			"executionContext.stagePacket.requiredOutput":            mapFromValue(mapFromValue(executionContext[stepInputKeyStagePacket])["requiredOutput"]),
			"executionContext.stagePacket.nextAction.requiredOutput": mapFromValue(mapFromValue(mapFromValue(executionContext[stepInputKeyStagePacket])["nextAction"])["requiredOutput"]),
		} {
			if len(existing) == 0 || stableCoverageJSON(existing) == expected {
				continue
			}
			appendAuthorityDiagnostic(input, "stale_required_output_mirror_rebound", "warning", "diagnostic_lineage", label, "stale requiredOutput mirror was rebound from current convergence authority before dispatch")
		}
	}
}

func dispatchInputHasConvergenceAuthority(run *runModel.Run, input map[string]any) bool {
	if len(convergenceAuthorityMapFromValue(input[convergenceAuthorityKey])) > 0 || len(gapBaselineMapFromValue(input[gapBaselineKey])) > 0 {
		return true
	}
	if executionContext := mapFromValue(input["executionContext"]); len(executionContext) > 0 {
		if len(convergenceAuthorityMapFromValue(executionContext[convergenceAuthorityKey])) > 0 || len(gapBaselineMapFromValue(executionContext[gapBaselineKey])) > 0 {
			return true
		}
	}
	if run == nil || run.ExecutionContext == nil {
		return false
	}
	return len(convergenceAuthorityMapFromValue(run.ExecutionContext[convergenceAuthorityKey])) > 0 || len(gapBaselineMapFromValue(run.ExecutionContext[gapBaselineKey])) > 0
}

func dispatchInputRequiresConvergenceDispatchValidation(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	return boolValue(input["graphLocalDispatchAuthority"]) ||
		len(mapFromValue(input["convergenceContract"])) > 0 ||
		len(mapFromValue(input[currentActionContractKey])) > 0 ||
		len(mapFromValue(input[stepInputKeyStagePacket])) > 0
}

func finalizeCurrentAuthorityDispatchInput(input map[string]any) (map[string]any, error) {
	if input == nil {
		input = map[string]any{}
	}
	input = synchronizeCurrentRequiredOutputMirrors(input)
	input = scrubConvergenceDispatchDiagnosticAdvice(input)
	if dispatchInputRequiresConvergenceDispatchValidation(input) {
		input = enforceCurrentAuthorityDerivedConsistency(nil, "", input)
		if err := validateConvergenceDispatchInput(input); err != nil {
			return nil, err
		}
	}
	return input, nil
}

func enforceCurrentAuthorityDerivedConsistency(run *runModel.Run, stepID common.StepID, input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	nextAction := strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind"))
	if nextAction == "" {
		nextAction = strings.TrimSpace(convergenceNextActionKind(run, input))
	}
	switch nextAction {
	case runtimemodel.NextActionTypeStateChange, "produce_state_change":
	default:
		return input
	}
	if targetSnapshotPrerequisiteActive(input) {
		return input
	}
	contract := mapFromValue(input["convergenceContract"])
	required := activeRequiredOutputForCurrentAction(input)
	if boolValue(required["mustAcquireCurrentTargetSnapshot"]) || boolValue(required["mustUseReadOnlySnapshotAcquisition"]) {
		return input
	}
	if strings.TrimSpace(stringFromMap(contract, "state")) == convergenceStateNeedStateChange &&
		boolValue(contract["stateChangingOutputRequired"]) &&
		requiredOutputHasAllowedStateChangingKind(required) &&
		len(mapFromValue(input[stepInputKeyStagePacket])) > 0 &&
		len(mapFromValue(input[currentActionContractKey])) > 0 {
		return input
	}
	for _, key := range []string{
		"boundedValidationRequired",
		"evidenceRepairRequired",
		"concreteFailureEvidenceRequired",
		stepInputKeyAssessmentOnly,
		"assessmentOnlyReason",
	} {
		delete(input, key)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	clearEvidenceAcquisitionStateForStateChange(pkg)
	required = mapFromValue(pkg["requiredOutput"])
	if !requiredOutputHasAllowedStateChangingKind(required) {
		required = stateChangingRepairRequiredOutput()
	} else {
		required = cloneStepPayload(required)
		required["mustIncludeStateChangingNode"] = true
		required["forbiddenKinds"] = removeStringsFromSlice(
			stringSliceFromValue(required["forbiddenKinds"]),
			"patch/apply",
			"git/write",
			"shell/command",
			"mcp/tool_call",
		)
	}
	pkg["requiredOutput"] = cloneStepPayload(required)
	pkg["repairObjective"] = repairObjectiveStateChangingOutput
	pkg["stateChangingOutputRequired"] = true
	for _, key := range []string{
		"evidenceAcquisitionRequired",
		"concreteFailureEvidenceRequired",
		"evidenceAcquisitionReason",
		"boundedValidationRequired",
		"outputDecisionContract",
		"exitCriteria",
	} {
		delete(pkg, key)
	}
	input["repairActionPackage"] = pkg
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["stateChangingOutputRequired"] = true
	currentActionSetRequiredOutput(input, required)
	input["convergenceContract"] = map[string]any{
		"state":                       convergenceStateNeedStateChange,
		"runtimeStage":                runtimeStageRepair,
		"runtimeStageMode":            firstNonEmptyString(stringFromMap(input, stepInputKeyRuntimeStageMode), "strict_convergence"),
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
	}
	action := map[string]any{
		"kind":   runtimemodel.NextActionTypeStateChange,
		"reason": firstNonEmptyString(stringFromMap(currentActionNextActionFromInput(input), "reason"), "current convergence authority requires state-changing output"),
	}
	currentActionSetNextAction(input, action)
	input = normalizeDispatchInputAuthorityFocus(input)
	input = attachStagePacketToStepInput(run, stepID, input)
	input = synchronizeCurrentRequiredOutputMirrors(input)
	return attachCurrentActionContractToInput(run, stepID, input)
}

func validateConvergenceDispatchInput(input map[string]any) error {
	if len(input) == 0 {
		return nil
	}
	nextAction := strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind"))
	if nextAction == "" {
		nextAction = strings.TrimSpace(convergenceNextActionKind(nil, input))
	}
	if nextAction == "" {
		return nil
	}
	contract := mapFromValue(input["convergenceContract"])
	state := strings.TrimSpace(stringFromMap(contract, "state"))
	stateChanging := boolValue(input["stateChangingOutputRequired"]) || boolValue(contract["stateChangingOutputRequired"])
	switch nextAction {
	case "validate", "validation", "produce_validation_evidence", "artifact", "produce_artifact_evidence":
		if state != convergenceStateNeedValidation {
			return convergenceDispatchMisalignmentError("nextAction=%s contractState=%s", nextAction, state)
		}
		if stateChanging {
			return convergenceDispatchMisalignmentError("validation action requires state-changing output")
		}
	case "assessment", "complete_coverage_assessment":
		if state != convergenceStateNeedAssessment {
			return convergenceDispatchMisalignmentError("nextAction=%s contractState=%s", nextAction, state)
		}
		if stateChanging {
			return convergenceDispatchMisalignmentError("assessment action requires state-changing output")
		}
	case "state_change", "produce_state_change":
		if state != convergenceStateNeedStateChange {
			return convergenceDispatchMisalignmentError("nextAction=%s contractState=%s", nextAction, state)
		}
		if !stateChanging && !targetSnapshotPrerequisiteActive(input) {
			return convergenceDispatchMisalignmentError("state_change action lacks state-changing output contract")
		}
	case "repair", "acquire_evidence", "acquire_target_snapshot":
		if state != convergenceStateNeedEvidence {
			return convergenceDispatchMisalignmentError("nextAction=%s contractState=%s", nextAction, state)
		}
		if stateChanging {
			return convergenceDispatchMisalignmentError("evidence action requires state-changing output")
		}
	case "request_decision", "wait_decision", "request_approval", "wait_approval", "request_governance":
		if state != convergenceStateNeedGovernance {
			return convergenceDispatchMisalignmentError("governed action has contractState=%s", state)
		}
		if stateChanging {
			return convergenceDispatchMisalignmentError("governed action requires state-changing output")
		}
	}
	if len(mapFromValue(input[stepInputKeyStagePacket])) == 0 {
		return convergenceDispatchMisalignmentError("missing stagePacket")
	}
	if err := validateStagePacketNextActionAlignment(input, nextAction); err != nil {
		return err
	}
	if err := validateConvergenceDispatchRequiredOutputSemantics(input); err != nil {
		return err
	}
	if err := validateCurrentActionContract(input); err != nil {
		return err
	}
	if err := validateConvergenceDispatchOutputShapeAdvice(input); err != nil {
		return err
	}
	return validateConvergenceDispatchFocusAlignment(input)
}

func validateCurrentActionContract(input map[string]any) error {
	contract := mapFromValue(input[currentActionContractKey])
	if len(contract) == 0 {
		return nil
	}
	currentNext := strings.TrimSpace(stringFromMap(mapFromValue(contract["nextAction"]), "kind"))
	if expected := strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind")); expected != "" && currentNext != "" && currentNext != expected {
		return convergenceDispatchMisalignmentError("currentActionContract nextAction=%s expected=%s", currentNext, expected)
	}
	currentRequired := mapFromValue(contract["requiredOutput"])
	if expected := activeRequiredOutputForCurrentAction(input); len(expected) > 0 {
		if len(currentRequired) == 0 {
			return convergenceDispatchMisalignmentError("currentActionContract missing requiredOutput")
		}
		if stableCoverageJSON(currentRequired) != stableCoverageJSON(expected) {
			return convergenceDispatchMisalignmentError("currentActionContract requiredOutput does not match active required output")
		}
	}
	return nil
}

func validateStagePacketNextActionAlignment(input map[string]any, nextAction string) error {
	expected := canonicalDispatchNextActionKind(nextAction)
	if expected == "" {
		return nil
	}
	for label, packet := range map[string]map[string]any{
		"stagePacket.nextAction":                  mapFromValue(input[stepInputKeyStagePacket]),
		"executionContext.stagePacket.nextAction": mapFromValue(mapFromValue(input["executionContext"])[stepInputKeyStagePacket]),
	} {
		action := mapFromValue(packet["nextAction"])
		actual := canonicalDispatchNextActionKind(firstNonEmptyString(
			stringFromMap(action, "type"),
			stringFromMap(action, "kind"),
		))
		if actual == "" || actual == expected {
			continue
		}
		appendAuthorityDiagnostic(input, "stage_packet_mismatch_next_action", "error", "internal_authority_mismatch", label, "stagePacket nextAction contradicts current convergence authority")
		return convergenceDispatchMisalignmentError("stage_packet_mismatch_next_action: %s=%s expected=%s", label, actual, expected)
	}
	return nil
}

func canonicalDispatchNextActionKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case "":
		return ""
	case runtimemodel.NextActionTypeValidate, "validation", "produce_validation_evidence", "produce_artifact_evidence", "artifact":
		return runtimemodel.NextActionTypeValidate
	case runtimemodel.NextActionTypeAssessment, "complete_coverage_assessment":
		return runtimemodel.NextActionTypeAssessment
	case runtimemodel.NextActionTypeStateChange, "produce_state_change":
		return runtimemodel.NextActionTypeStateChange
	case runtimemodel.NextActionTypeRepair, "acquire_evidence":
		return runtimemodel.NextActionTypeRepair
	case runtimemodel.NextActionTypeAcquireTargetSnapshot:
		return runtimemodel.NextActionTypeAcquireTargetSnapshot
	case runtimemodel.NextActionTypeRequestDecision, "wait_decision", "request_governance":
		return runtimemodel.NextActionTypeRequestDecision
	case runtimemodel.NextActionTypeRequestApproval, "wait_approval":
		return runtimemodel.NextActionTypeRequestApproval
	default:
		return strings.TrimSpace(kind)
	}
}

func convergenceDispatchMisalignmentError(format string, args ...any) error {
	return fmt.Errorf("%w: %s: "+format, append([]any{common.ErrConflict, failureClassRuntimeContractMisalign}, args...)...)
}

func normalizeDispatchInputAuthorityFocus(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	authorityFocus := dispatchAuthorityCurrentFocus(input)
	if len(authorityFocus) == 0 {
		return input
	}
	state := strings.TrimSpace(stringFromMap(mapFromValue(input["convergenceContract"]), "state"))
	if state == convergenceStateNeedGovernance {
		delete(input, "requiredOutput")
	}
	coverageFocus := dispatchCoverageFocusFromAuthority(input, authorityFocus)
	if len(coverageFocus) > 0 {
		input[coverageFocusKey] = coverageFocus
	}
	if context := coverageWorkItemContextFromInput(input); len(context) > 0 {
		input[coverageWorkItemContextKey] = coverageWorkItemContextAlignedToFocus(context, coverageFocus)
	}
	if budget := mapFromValue(input["convergenceBudget"]); len(budget) > 0 {
		input["convergenceBudget"] = normalizeConvergenceBudgetFocusBinding(budget, coverageFocus)
	}
	if executionContext := mapFromValue(input["executionContext"]); len(executionContext) > 0 {
		if context := mapFromValue(executionContext[coverageWorkItemContextKey]); len(context) > 0 {
			executionContext[coverageWorkItemContextKey] = coverageWorkItemContextAlignedToFocus(context, coverageFocus)
		}
		if budget := mapFromValue(executionContext["convergenceBudget"]); len(budget) > 0 {
			executionContext["convergenceBudget"] = normalizeConvergenceBudgetFocusBinding(budget, coverageFocus)
		}
		if len(coverageFocus) > 0 {
			executionContext[coverageFocusKey] = cloneStepPayload(coverageFocus)
		}
		if acceptance := mapFromValue(executionContext["acceptance"]); len(acceptance) > 0 {
			executionContext["acceptance"] = acceptanceSignalAlignedToAuthorityFocus(acceptance, coverageFocus)
		}
		if required := mapFromValue(executionContext["requiredOutput"]); len(required) > 0 {
			executionContext["requiredOutput"] = normalizeRequiredOutputFocusBinding(required, coverageFocus)
		}
		if pkg := mapFromValue(executionContext["repairActionPackage"]); len(pkg) > 0 {
			executionContext["repairActionPackage"] = normalizeRepairActionPackageRequiredOutputForAuthority(input, pkg, coverageFocus)
		}
		if packet := mapFromValue(executionContext[stepInputKeyStagePacket]); len(packet) > 0 {
			if required := mapFromValue(packet["requiredOutput"]); len(required) > 0 {
				packet["requiredOutput"] = normalizeRequiredOutputFocusBinding(required, coverageFocus)
			}
			if len(coverageFocus) > 0 {
				packet["activeFocus"] = cloneStepPayload(coverageFocus)
			}
			executionContext[stepInputKeyStagePacket] = packet
		}
		input["executionContext"] = executionContext
	}
	if acceptance := mapFromValue(input["acceptance"]); len(acceptance) > 0 {
		input["acceptance"] = acceptanceSignalAlignedToAuthorityFocus(acceptance, coverageFocus)
	}
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		if context := mapFromValue(pkg[coverageWorkItemContextKey]); len(context) > 0 {
			pkg[coverageWorkItemContextKey] = coverageWorkItemContextAlignedToFocus(context, coverageFocus)
		}
		if len(coverageFocus) > 0 {
			pkg[coverageFocusKey] = compactLLMContextMap(coverageFocus, 1)
		}
		input["repairActionPackage"] = normalizeRepairActionPackageRequiredOutputForAuthority(input, pkg, coverageFocus)
	}
	if state == convergenceStateNeedAssessment {
		required := assessmentRequiredOutputForAuthorityFocus(coverageFocus)
		if len(required) > 0 {
			input["requiredOutput"] = required
			pkg := mapFromValue(input["repairActionPackage"])
			if len(pkg) == 0 {
				pkg = map[string]any{}
			}
			pkg["requiredOutput"] = cloneStepPayload(required)
			input["repairActionPackage"] = pkg
		}
		return input
	}
	if required := mapFromValue(input["requiredOutput"]); len(required) > 0 {
		input["requiredOutput"] = normalizeRequiredOutputFocusBinding(required, coverageFocus)
	}
	return input
}

func dispatchAuthorityCurrentFocus(input map[string]any) map[string]any {
	for _, source := range []map[string]any{
		mapFromValue(input[convergenceAuthorityKey]),
		mapFromValue(input[gapBaselineKey]),
		mapFromValue(mapFromValue(input["executionContext"])[convergenceAuthorityKey]),
		mapFromValue(mapFromValue(input["executionContext"])[gapBaselineKey]),
	} {
		if focus := mapFromValue(source["currentFocus"]); len(focus) > 0 {
			return cloneStepPayload(focus)
		}
	}
	return nil
}

func dispatchCoverageFocusFromAuthority(input map[string]any, authorityFocus map[string]any) map[string]any {
	if len(authorityFocus) == 0 {
		return nil
	}
	authoritySliceID := dispatchAuthoritySliceIDForFocus(input, authorityFocus)
	for _, existing := range []map[string]any{
		mapFromValue(input[coverageFocusKey]),
		mapFromValue(mapFromValue(input["acceptance"])["currentFocus"]),
		mapFromValue(mapFromValue(input["executionContext"])[coverageFocusKey]),
	} {
		if len(existing) > 0 && dispatchFocusAligned(authorityFocus, existing) {
			next := cloneStepPayload(existing)
			for _, key := range []string{"coverageWorkItemIds", "requiredEvidenceKinds", "acceptedEvidenceRefs", "rejectedEvidenceRefs", "targetRefs", "state"} {
				if value, ok := authorityFocus[key]; ok && value != nil {
					next[key] = cloneStepInputValue(value)
				}
			}
			if authoritySliceID != "" {
				next["sliceId"] = authoritySliceID
			} else if strings.TrimSpace(stringFromMap(authorityFocus, "sliceId")) == "" {
				delete(next, "sliceId")
			}
			return next
		}
	}
	sliceID := firstNonEmptyString(
		authoritySliceID,
		stringFromMap(authorityFocus, "sliceId"),
	)
	if len(stringSliceFromValue(authorityFocus["coverageWorkItemIds"])) > 0 ||
		strings.HasPrefix(strings.TrimSpace(stringFromMap(authorityFocus, "id")), "focus-") {
		next := cloneStepPayload(authorityFocus)
		if sliceID != "" && strings.TrimSpace(stringFromMap(next, "sliceId")) == "" {
			next["sliceId"] = sliceID
		}
		return next
	}
	if focus := coverageFocusFromWorkItem(sliceID, authorityFocus); len(focus) > 0 {
		return focus
	}
	return cloneStepPayload(authorityFocus)
}

func dispatchAuthoritySliceIDForFocus(input map[string]any, authorityFocus map[string]any) string {
	if len(input) == 0 || len(authorityFocus) == 0 {
		return strings.TrimSpace(stringFromMap(authorityFocus, "sliceId"))
	}
	focusIDs := stringSet(dispatchFocusWorkItemIDs(authorityFocus))
	if len(focusIDs) == 0 {
		return strings.TrimSpace(stringFromMap(authorityFocus, "sliceId"))
	}
	for _, slices := range [][]map[string]any{
		mapSliceFromValue(input[acceptanceSlicesKey]),
		mapSliceFromValue(mapFromValue(input["executionContext"])[acceptanceSlicesKey]),
		mapSliceFromValue(mapFromValue(input["acceptance"])["slices"]),
	} {
		for _, slice := range slices {
			sliceID := strings.TrimSpace(stringFromMap(slice, "id"))
			if sliceID == "" {
				continue
			}
			for _, id := range stringSliceFromValue(slice["coverageWorkItemIds"]) {
				if focusIDs[id] {
					return sliceID
				}
			}
		}
	}
	return strings.TrimSpace(stringFromMap(authorityFocus, "sliceId"))
}

func acceptanceSignalAlignedToAuthorityFocus(acceptance map[string]any, coverageFocus map[string]any) map[string]any {
	if len(acceptance) == 0 || len(coverageFocus) == 0 {
		return acceptance
	}
	next := cloneStepPayload(acceptance)
	stale := map[string]any{}
	if activeSlice := mapFromValue(next["activeSlice"]); len(activeSlice) > 0 && !dispatchFocusAligned(coverageFocus, activeSlice) {
		stale["activeSlice"] = cloneStepPayload(activeSlice)
		delete(next, "activeSlice")
	}
	if currentFocus := mapFromValue(next["currentFocus"]); len(currentFocus) > 0 && !dispatchFocusAligned(coverageFocus, currentFocus) {
		stale["currentFocus"] = cloneStepPayload(currentFocus)
	}
	if len(stale) > 0 {
		appendDiagnosticLineage(next, "staleAcceptance", stale)
	}
	next["currentFocus"] = cloneStepPayload(coverageFocus)
	if sliceID := strings.TrimSpace(stringFromMap(coverageFocus, "sliceId")); sliceID != "" {
		next["activeSliceId"] = sliceID
	} else if len(mapFromValue(next["activeSlice"])) == 0 {
		delete(next, "activeSliceId")
	}
	next["projectionPolicy"] = "dynamic C4 context; currentFocus is derived from convergenceAuthority before runtime dispatch"
	return next
}

func appendDiagnosticLineage(target map[string]any, key string, value map[string]any) {
	if len(target) == 0 || strings.TrimSpace(key) == "" || len(value) == 0 {
		return
	}
	lineage := cloneStepPayload(mapFromValue(target["diagnosticLineage"]))
	if lineage == nil {
		lineage = map[string]any{}
	}
	lineage[key] = cloneStepPayload(value)
	target["diagnosticLineage"] = lineage
}

func normalizeRepairActionPackageRequiredOutputForAuthority(input map[string]any, pkg map[string]any, coverageFocus map[string]any) map[string]any {
	if len(pkg) == 0 {
		return pkg
	}
	state := strings.TrimSpace(stringFromMap(mapFromValue(input["convergenceContract"]), "state"))
	if state == convergenceStateNeedAssessment {
		if required := assessmentRequiredOutputForAuthorityFocus(coverageFocus); len(required) > 0 {
			pkg["requiredOutput"] = required
		} else {
			delete(pkg, "requiredOutput")
		}
		for _, key := range []string{"forbiddenActions", "exitCriteria", "evidenceAcquisitionRequired"} {
			delete(pkg, key)
		}
		return pkg
	}
	if state == convergenceStateNeedGovernance {
		for _, key := range []string{"requiredOutput", "forbiddenActions", "exitCriteria", "evidenceAcquisitionRequired"} {
			delete(pkg, key)
		}
		return pkg
	}
	if required := mapFromValue(pkg["requiredOutput"]); len(required) > 0 {
		pkg["requiredOutput"] = normalizeRequiredOutputFocusBinding(required, coverageFocus)
	}
	reapplyRepairCoverageFocusContract(input, pkg)
	return pkg
}

func normalizeRequiredOutputFocusBinding(required map[string]any, coverageFocus map[string]any) map[string]any {
	if len(required) == 0 || len(coverageFocus) == 0 {
		return required
	}
	next := cloneStepPayload(required)
	focusID := strings.TrimSpace(stringFromMap(coverageFocus, "id"))
	workItemIDs := dispatchFocusWorkItemIDs(coverageFocus)
	targetRefs := coverageFocusTargetRefsForRequiredOutput(coverageFocus)
	structuredTargetRefs := coverageFocusStructuredTargetRefsForRequiredOutput(coverageFocus)
	if focusID != "" {
		next["coverageFocusId"] = focusID
	}
	if len(workItemIDs) > 0 {
		next["coverageWorkItemIds"] = workItemIDs
	}
	if len(targetRefs) > 0 {
		next["targetRefs"] = targetRefs
		if _, ok := next["allowedTargetRefs"]; ok {
			next["allowedTargetRefs"] = targetRefs
		}
	} else {
		delete(next, "targetRefs")
	}
	if len(structuredTargetRefs) > 0 {
		next["structuredTargetRefs"] = structuredTargetRefs
	} else {
		delete(next, "structuredTargetRefs")
	}
	normalizeRequiredOutputNestedFocusBindings(next, coverageFocus)
	return next
}

func normalizeRequiredOutputNestedFocusBindings(value any, coverageFocus map[string]any) {
	switch typed := value.(type) {
	case map[string]any:
		if evidence := mapFromValue(typed["validationEvidence"]); len(evidence) > 0 {
			typed["validationEvidence"] = normalizeValidationEvidenceFocusBinding(evidence, coverageFocus)
		}
		if looksLikeValidationEvidenceBinding(typed) {
			normalized := normalizeValidationEvidenceFocusBinding(typed, coverageFocus)
			for key, normalizedValue := range normalized {
				typed[key] = normalizedValue
			}
		}
		for _, child := range typed {
			normalizeRequiredOutputNestedFocusBindings(child, coverageFocus)
		}
	case []any:
		for _, child := range typed {
			normalizeRequiredOutputNestedFocusBindings(child, coverageFocus)
		}
	case []map[string]any:
		for _, child := range typed {
			normalizeRequiredOutputNestedFocusBindings(child, coverageFocus)
		}
	}
}

func looksLikeValidationEvidenceBinding(value map[string]any) bool {
	if len(value) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromMap(value, "mode")) == "" &&
		strings.TrimSpace(stringFromMap(value, "observationType")) == "" {
		return false
	}
	return len(stringSliceFromValue(value["targetRefs"])) > 0 ||
		len(stringSliceFromValue(value["structuredTargetRefs"])) > 0 ||
		len(stringSliceFromValue(value["coverageWorkItemIds"])) > 0 ||
		len(stringSliceFromValue(value["coverageWorkItemIDs"])) > 0
}

func normalizeValidationEvidenceFocusBinding(evidence map[string]any, coverageFocus map[string]any) map[string]any {
	if len(evidence) == 0 || len(coverageFocus) == 0 {
		return evidence
	}
	next := cloneStepPayload(evidence)
	if ids := dispatchFocusWorkItemIDs(coverageFocus); len(ids) > 0 {
		next["coverageWorkItemIds"] = ids
	}
	if refs := coverageFocusTargetRefsForRequiredOutput(coverageFocus); len(refs) > 0 {
		next["targetRefs"] = refs
	} else {
		delete(next, "targetRefs")
	}
	if refs := coverageFocusStructuredTargetRefsForRequiredOutput(coverageFocus); len(refs) > 0 {
		next["structuredTargetRefs"] = refs
	} else {
		delete(next, "structuredTargetRefs")
	}
	return next
}

func coverageFocusTargetRefsForRequiredOutput(coverageFocus map[string]any) []string {
	refs := compactUniqueStrings(append(
		stringSliceFromValue(coverageFocus["targetRefs"]),
		stringSliceFromValue(coverageFocus["allowedTargetRefs"])...,
	))
	if len(refs) > 0 {
		return refs
	}
	return nil
}

func coverageFocusStructuredTargetRefsForRequiredOutput(coverageFocus map[string]any) []string {
	refs := compactUniqueStrings(append(
		stringSliceFromValue(coverageFocus["structuredTargetRefs"]),
		stringSliceFromValue(coverageFocus["targetRefs"])...,
	))
	return refs
}

func normalizeConvergenceBudgetFocusBinding(budget map[string]any, coverageFocus map[string]any) map[string]any {
	if len(budget) == 0 || len(coverageFocus) == 0 {
		return budget
	}
	next := cloneStepPayload(budget)
	if focusID := strings.TrimSpace(stringFromMap(coverageFocus, "id")); focusID != "" {
		next["focusId"] = focusID
	}
	if ids := dispatchFocusWorkItemIDs(coverageFocus); len(ids) > 0 {
		next["coverageWorkItemIds"] = ids
	}
	return next
}

func assessmentRequiredOutputForAuthorityFocus(coverageFocus map[string]any) map[string]any {
	if len(coverageFocus) == 0 {
		return nil
	}
	ret := map[string]any{
		"mustAssessAcceptedEvidence": true,
		"assessmentScope":            "current_focus",
		"requiredFields":             []string{"dodAssessment.items", "evidenceRefs"},
		"forbiddenOutputs":           []string{"executionGraph", "patchRequests", "shellCommands", "mcpToolCalls", "memoryWrites"},
	}
	if focusID := strings.TrimSpace(stringFromMap(coverageFocus, "id")); focusID != "" {
		ret["coverageFocusId"] = focusID
	}
	if ids := dispatchFocusWorkItemIDs(coverageFocus); len(ids) > 0 {
		ret["coverageWorkItemIds"] = ids
	}
	if refs := stringSliceFromValue(coverageFocus["acceptedEvidenceRefs"]); len(refs) > 0 {
		ret["acceptedEvidenceRefs"] = refs
	}
	if refs := stringSliceFromValue(coverageFocus["targetRefs"]); len(refs) > 0 {
		ret["allowedTargetRefs"] = refs
	}
	return ret
}

func validateConvergenceDispatchFocusAlignment(input map[string]any) error {
	authorityFocus := dispatchAuthorityCurrentFocus(input)
	if len(authorityFocus) == 0 {
		return nil
	}
	checkFocus := func(label string, focus map[string]any) error {
		if len(focus) == 0 || dispatchFocusAligned(authorityFocus, focus) {
			return nil
		}
		return convergenceDispatchMisalignmentError("focus mismatch: %s=%s authority=%s", label, dispatchFocusDebugToken(focus), dispatchFocusDebugToken(authorityFocus))
	}
	if err := checkFocus("gapBaseline.currentFocus", mapFromValue(mapFromValue(input[gapBaselineKey])["currentFocus"])); err != nil {
		return err
	}
	if err := checkFocus("stagePacket.activeFocus", mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["activeFocus"])); err != nil {
		return err
	}
	if err := checkFocus("coverageFocus", mapFromValue(input[coverageFocusKey])); err != nil {
		return err
	}
	if err := checkFocus("coverageWorkItemContext.currentFocus", mapFromValue(mapFromValue(input[coverageWorkItemContextKey])["currentFocus"])); err != nil {
		return err
	}
	if err := validateConvergenceBudgetFocusAlignment(authorityFocus, "convergenceBudget", mapFromValue(input["convergenceBudget"])); err != nil {
		return err
	}
	if err := checkFocus("acceptance.currentFocus", mapFromValue(mapFromValue(input["acceptance"])["currentFocus"])); err != nil {
		return err
	}
	if err := checkFocus("acceptance.activeSlice", mapFromValue(mapFromValue(input["acceptance"])["activeSlice"])); err != nil {
		return err
	}
	executionContext := mapFromValue(input["executionContext"])
	if err := checkFocus("executionContext.acceptance.currentFocus", mapFromValue(mapFromValue(executionContext["acceptance"])["currentFocus"])); err != nil {
		return err
	}
	if err := checkFocus("executionContext.acceptance.activeSlice", mapFromValue(mapFromValue(executionContext["acceptance"])["activeSlice"])); err != nil {
		return err
	}
	if err := checkFocus("executionContext.coverageWorkItemContext.currentFocus", mapFromValue(mapFromValue(executionContext[coverageWorkItemContextKey])["currentFocus"])); err != nil {
		return err
	}
	if err := validateConvergenceBudgetFocusAlignment(authorityFocus, "executionContext.convergenceBudget", mapFromValue(executionContext["convergenceBudget"])); err != nil {
		return err
	}
	for label, required := range map[string]map[string]any{
		"repairActionPackage.requiredOutput":                     mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":                             mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		"stagePacket.nextAction.requiredOutput":                  mapFromValue(mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["nextAction"])["requiredOutput"]),
		"currentActionContract.requiredOutput":                   mapFromValue(mapFromValue(input[currentActionContractKey])["requiredOutput"]),
		"requiredOutput":                                         mapFromValue(input["requiredOutput"]),
		"executionContext.requiredOutput":                        mapFromValue(executionContext["requiredOutput"]),
		"executionContext.repairActionPackage.requiredOutput":    mapFromValue(mapFromValue(executionContext["repairActionPackage"])["requiredOutput"]),
		"executionContext.stagePacket.requiredOutput":            mapFromValue(mapFromValue(executionContext[stepInputKeyStagePacket])["requiredOutput"]),
		"executionContext.stagePacket.nextAction.requiredOutput": mapFromValue(mapFromValue(mapFromValue(executionContext[stepInputKeyStagePacket])["nextAction"])["requiredOutput"]),
	} {
		if err := validateRequiredOutputFocusAlignment(authorityFocus, label, required); err != nil {
			return err
		}
	}
	return nil
}

func validateConvergenceBudgetFocusAlignment(authorityFocus map[string]any, label string, budget map[string]any) error {
	if len(authorityFocus) == 0 || len(budget) == 0 {
		return nil
	}
	candidate := map[string]any{}
	if focusID := strings.TrimSpace(firstNonEmptyString(stringFromMap(budget, "focusId"), stringFromMap(budget, "coverageFocusId"))); focusID != "" {
		candidate["id"] = focusID
	}
	if ids := stringSliceFromValue(budget["coverageWorkItemIds"]); len(ids) > 0 {
		candidate["coverageWorkItemIds"] = ids
	}
	if len(candidate) == 0 || dispatchFocusAligned(authorityFocus, candidate) {
		return nil
	}
	return convergenceDispatchMisalignmentError("focus mismatch: %s=%s authority=%s", label, dispatchFocusDebugToken(candidate), dispatchFocusDebugToken(authorityFocus))
}

func validateConvergenceDispatchRequiredOutputSemantics(input map[string]any) error {
	if len(input) == 0 {
		return nil
	}
	sources := map[string]map[string]any{
		"requiredOutput":                                         mapFromValue(input["requiredOutput"]),
		"repairActionPackage.requiredOutput":                     mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":                             mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		"stagePacket.nextAction.requiredOutput":                  mapFromValue(mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["nextAction"])["requiredOutput"]),
		"currentActionContract.requiredOutput":                   mapFromValue(mapFromValue(input[currentActionContractKey])["requiredOutput"]),
		"executionContext.requiredOutput":                        mapFromValue(mapFromValue(input["executionContext"])["requiredOutput"]),
		"executionContext.repairActionPackage.requiredOutput":    mapFromValue(mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"])["requiredOutput"]),
		"executionContext.stagePacket.requiredOutput":            mapFromValue(mapFromValue(mapFromValue(input["executionContext"])[stepInputKeyStagePacket])["requiredOutput"]),
		"executionContext.stagePacket.nextAction.requiredOutput": mapFromValue(mapFromValue(mapFromValue(mapFromValue(input["executionContext"])[stepInputKeyStagePacket])["nextAction"])["requiredOutput"]),
	}
	state := strings.TrimSpace(stringFromMap(mapFromValue(input["convergenceContract"]), "state"))
	nextAction := strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind"))
	if nextAction == "" {
		nextAction = strings.TrimSpace(convergenceNextActionKind(nil, input))
	}
	if err := validateConvergenceContractStateMatchesNextAction(input, state, nextAction); err != nil {
		return err
	}
	authorityFocus := dispatchAuthorityCurrentFocus(input)
	authorityMissing := coverageWorkItemMissingEvidenceKindSet(authorityFocus)
	if authorityMissing["state_change"] && !authorityMissing["validation"] && !authorityMissing["artifact"] &&
		(state == convergenceStateNeedEvidence || nextAction == "repair" || nextAction == "acquire_evidence") &&
		!targetSnapshotPrerequisiteActive(input) {
		appendAuthorityDiagnostic(input, "next_action_mismatch_missing_state_change", "error", "internal_authority_mismatch", "nextAction", "coverage focus missing state_change cannot use evidence acquisition contract")
		return convergenceDispatchMisalignmentError("next_action_mismatch_missing_state_change: missing state_change requires state_change next action")
	}
	pkg := mapFromValue(input["repairActionPackage"])
	pkgObjective := strings.TrimSpace(stringFromMap(pkg, "repairObjective"))
	prerequisiteActive := targetSnapshotPrerequisiteActive(input)
	if state == convergenceStateNeedStateChange {
		if pkgObjective == repairObjectiveAcquireFailureEvidence || boolValue(pkg["evidenceAcquisitionRequired"]) {
			if !prerequisiteActive {
				appendAuthorityDiagnostic(input, "repair_action_package_mismatch_next_action", "error", "internal_authority_mismatch", "repairActionPackage", "state_change action cannot expose failure-evidence acquisition as current repairActionPackage")
				return convergenceDispatchMisalignmentError("repairActionPackage contradicts state_change current action")
			}
		}
	}
	if state == convergenceStateNeedValidation {
		if pkgObjective == repairObjectiveAcquireFailureEvidence || boolValue(pkg["evidenceAcquisitionRequired"]) {
			appendAuthorityDiagnostic(input, "repair_action_package_mismatch_next_action", "error", "internal_authority_mismatch", "repairActionPackage", "validation action cannot expose failure-evidence acquisition as current repairActionPackage")
			return convergenceDispatchMisalignmentError("repairActionPackage contradicts validation current action")
		}
		if pkgObjective == repairObjectiveStateChangingOutput || boolValue(pkg["stateChangingOutputRequired"]) {
			appendAuthorityDiagnostic(input, "repair_action_package_mismatch_next_action", "error", "internal_authority_mismatch", "repairActionPackage", "validation action cannot expose state-changing output as current repairActionPackage")
			return convergenceDispatchMisalignmentError("repairActionPackage contradicts validation current action")
		}
	}
	top := firstRequiredOutputForSemantics(sources)
	if len(top) == 0 {
		return nil
	}
	topStateChanging := requiredOutputRequiresStateChanging(top)
	topValidationOnly := requiredOutputValidationOnly(top)
	topFailureEvidence := requiredOutputRequiresConcreteFailureEvidence(top)
	if state == convergenceStateNeedStateChange && !topStateChanging && !prerequisiteActive {
		return convergenceDispatchMisalignmentError("contractState=%s requires state-changing requiredOutput", state)
	}
	if state == convergenceStateNeedValidation && topStateChanging {
		return convergenceDispatchMisalignmentError("contractState=%s has state-changing requiredOutput", state)
	}
	if (state == convergenceStateNeedValidation || nextAction == runtimemodel.NextActionTypeValidate) && topFailureEvidence {
		appendAuthorityDiagnostic(input, "required_output_mismatch_next_action", "error", "internal_authority_mismatch", "requiredOutput", "validation action requires validation evidence requiredOutput, not concrete failure evidence acquisition")
		return convergenceDispatchMisalignmentError("required_output_mismatch_next_action: validation has concrete failure evidence requiredOutput")
	}
	if (state == convergenceStateNeedStateChange || nextAction == runtimemodel.NextActionTypeStateChange) && !requiredOutputHasAllowedStateChangingKind(top) && !prerequisiteActive {
		appendAuthorityDiagnostic(input, "required_output_mismatch_next_action", "error", "internal_authority_mismatch", "requiredOutput", "state_change requiredOutput lacks an allowed state-changing accepted kind")
		return convergenceDispatchMisalignmentError("required_output_mismatch_next_action: state_change lacks allowed state-changing accepted kind")
	}
	if (state == convergenceStateNeedValidation || nextAction == runtimemodel.NextActionTypeValidate) && requiredOutputAcceptsKind(top, "patch/apply") {
		appendAuthorityDiagnostic(input, "required_output_mismatch_next_action", "error", "internal_authority_mismatch", "requiredOutput", "validation requiredOutput accepts workspace-mutating output")
		return convergenceDispatchMisalignmentError("required_output_mismatch_next_action: validation accepts workspace-mutating output")
	}
	if err := validateRequiredOutputSatisfiableRoute(input, "requiredOutput", top, state, nextAction); err != nil {
		return err
	}
	for label, required := range sources {
		if len(required) == 0 {
			continue
		}
		if requiredOutputRequiresStateChanging(required) != topStateChanging {
			return convergenceDispatchMisalignmentError("requiredOutput state-changing mismatch: %s", label)
		}
		if requiredOutputValidationOnly(required) != topValidationOnly {
			return convergenceDispatchMisalignmentError("requiredOutput validation-only mismatch: %s", label)
		}
		if (state == convergenceStateNeedValidation || nextAction == runtimemodel.NextActionTypeValidate) && requiredOutputRequiresConcreteFailureEvidence(required) {
			return convergenceDispatchMisalignmentError("validation requiredOutput requires concrete failure evidence: %s", label)
		}
		if topValidationOnly && requiredOutputAcceptsKind(required, "patch/apply") {
			return convergenceDispatchMisalignmentError("validation-only requiredOutput accepts patch/apply: %s", label)
		}
		if topStateChanging && !requiredOutputHasAllowedStateChangingKind(required) && !prerequisiteActive {
			appendAuthorityDiagnostic(input, "required_output_mismatch_next_action", "error", "internal_authority_mismatch", label, "state-changing requiredOutput has no allowed accepted kind")
			return convergenceDispatchMisalignmentError("required_output_mismatch_next_action: state-changing requiredOutput has no allowed accepted kind: %s", label)
		}
		if err := validateRequiredOutputSatisfiableRoute(input, label, required, state, nextAction); err != nil {
			return err
		}
	}
	shape := strings.TrimSpace(stringFromMap(top, "contractViolationRequiredOutputShape"))
	objective := strings.TrimSpace(stringFromMap(input, "repairObjective"))
	switch shape {
	case "bounded_validation_evidence":
		if objective == repairObjectiveStateChangingOutput || boolValue(input["stateChangingOutputRequired"]) {
			return convergenceDispatchMisalignmentError("bounded validation retry has state-changing objective")
		}
	case "bounded_failure_evidence":
		if objective == repairObjectiveStateChangingOutput || boolValue(input["stateChangingOutputRequired"]) {
			return convergenceDispatchMisalignmentError("bounded evidence retry has state-changing objective")
		}
	}
	return nil
}

func validateRequiredOutputSatisfiableRoute(input map[string]any, label string, required map[string]any, state string, nextAction string) error {
	if len(required) == 0 {
		return nil
	}
	if requiredOutputRequiresStateChanging(required) && boolValue(required["stateChangingForbidden"]) {
		appendAuthorityDiagnostic(input, "required_output_unsatisfiable", "error", "internal_authority_mismatch", label, "requiredOutput requires state-changing evidence but also forbids state-changing output")
		return convergenceDispatchMisalignmentError("runtime_internal_contract_unsatisfiable: %s requires and forbids state-changing output", label)
	}
	routes := requiredOutputSatisfiableRouteKinds(required)
	declaresRouteSet := len(stringSliceFromValue(required["acceptedKinds"])) > 0 || len(stringSliceFromValue(required["allowedOutputShapes"])) > 0
	if len(stringSliceFromValue(required["acceptedKinds"])) > 0 && len(routes) == 0 {
		appendAuthorityDiagnostic(input, "required_output_unsatisfiable", "error", "internal_authority_mismatch", label, "requiredOutput acceptedKinds are fully forbidden")
		return convergenceDispatchMisalignmentError("runtime_internal_contract_unsatisfiable: %s has no accepted output kind route", label)
	}
	shapes := requiredOutputSatisfiableShapes(required)
	if len(stringSliceFromValue(required["allowedOutputShapes"])) > 0 && len(shapes) == 0 {
		appendAuthorityDiagnostic(input, "required_output_unsatisfiable", "error", "internal_authority_mismatch", label, "requiredOutput allowedOutputShapes are fully forbidden")
		return convergenceDispatchMisalignmentError("runtime_internal_contract_unsatisfiable: %s has no allowed output shape route", label)
	}
	switch {
	case state == convergenceStateNeedStateChange || nextAction == runtimemodel.NextActionTypeStateChange:
		if targetSnapshotPrerequisiteActive(input) && requiredOutputIsReadOnlyTargetSnapshotPrerequisite(required) {
			return nil
		}
		if !requiredOutputRouteHasStateChangingKind(routes) {
			appendAuthorityDiagnostic(input, "required_output_unsatisfiable", "error", "internal_authority_mismatch", label, "state-change next action has no state-changing output route")
			return convergenceDispatchMisalignmentError("runtime_internal_contract_unsatisfiable: %s has no state-changing output route", label)
		}
	case state == convergenceStateNeedValidation || nextAction == runtimemodel.NextActionTypeValidate:
		if !declaresRouteSet {
			return nil
		}
		if !requiredOutputRouteHasValidationKind(routes) && !requiredOutputShapesAllowAssessment(shapes) {
			appendAuthorityDiagnostic(input, "required_output_unsatisfiable", "error", "internal_authority_mismatch", label, "validation next action has no validation output route")
			return convergenceDispatchMisalignmentError("runtime_internal_contract_unsatisfiable: %s has no validation output route", label)
		}
	case state == convergenceStateNeedEvidence || nextAction == runtimemodel.NextActionTypeRepair || nextAction == "acquire_evidence":
		if !declaresRouteSet {
			return nil
		}
		if !requiredOutputRouteHasEvidenceKind(routes) && len(shapes) == 0 {
			appendAuthorityDiagnostic(input, "required_output_unsatisfiable", "error", "internal_authority_mismatch", label, "evidence next action has no evidence acquisition output route")
			return convergenceDispatchMisalignmentError("runtime_internal_contract_unsatisfiable: %s has no evidence acquisition output route", label)
		}
	}
	return nil
}

func validateConvergenceContractStateMatchesNextAction(input map[string]any, state string, nextAction string) error {
	if state == "" || nextAction == "" {
		return nil
	}
	if targetSnapshotPrerequisiteActive(input) && nextAction == runtimemodel.NextActionTypeStateChange && state == convergenceStateNeedStateChange {
		return nil
	}
	switch nextAction {
	case runtimemodel.NextActionTypeStateChange, "produce_state_change":
		if state != convergenceStateNeedStateChange {
			appendAuthorityDiagnostic(input, "contract_state_mismatch_next_action", "error", "internal_authority_mismatch", "convergenceContract", "state_change nextAction requires need_state_change convergenceContract")
			return convergenceDispatchMisalignmentError("contract_state_mismatch_next_action: state_change nextAction has contract state %s", state)
		}
	case runtimemodel.NextActionTypeValidate, "validation", "produce_validation_evidence":
		if state != convergenceStateNeedValidation {
			appendAuthorityDiagnostic(input, "contract_state_mismatch_next_action", "error", "internal_authority_mismatch", "convergenceContract", "validation nextAction requires need_validation convergenceContract")
			return convergenceDispatchMisalignmentError("contract_state_mismatch_next_action: validation nextAction has contract state %s", state)
		}
	case runtimemodel.NextActionTypeRepair, "acquire_evidence":
		if state != convergenceStateNeedEvidence {
			appendAuthorityDiagnostic(input, "contract_state_mismatch_next_action", "error", "internal_authority_mismatch", "convergenceContract", "evidence acquisition nextAction requires need_evidence convergenceContract")
			return convergenceDispatchMisalignmentError("contract_state_mismatch_next_action: evidence nextAction has contract state %s", state)
		}
	}
	return nil
}

func targetSnapshotPrerequisiteActive(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	nextAction := strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind"))
	if nextAction != runtimemodel.NextActionTypeStateChange {
		return false
	}
	contract := mapFromValue(input["convergenceContract"])
	if strings.TrimSpace(stringFromMap(contract, "state")) != convergenceStateNeedStateChange {
		return false
	}
	if strings.TrimSpace(stringFromMap(mapFromValue(contract["prerequisite"]), "kind")) == runtimemodel.NextActionTypeAcquireTargetSnapshot {
		return true
	}
	if strings.TrimSpace(stringFromMap(mapFromValue(mapFromValue(input[currentActionContractKey])["prerequisite"]), "kind")) == runtimemodel.NextActionTypeAcquireTargetSnapshot {
		return true
	}
	return false
}

func requiredOutputIsReadOnlyTargetSnapshotPrerequisite(required map[string]any) bool {
	if len(required) == 0 {
		return false
	}
	if !boolValue(required["mustAcquireCurrentTargetSnapshot"]) || !boolValue(required["mustUseReadOnlySnapshotAcquisition"]) {
		return false
	}
	if boolValue(required["mustIncludeStateChangingNode"]) || requiredOutputRequiresStateChanging(required) {
		return false
	}
	return true
}

func requiredOutputSatisfiableRouteKinds(required map[string]any) []string {
	if len(required) == 0 {
		return nil
	}
	accepted := compactUniqueStrings(stringSliceFromValue(required["acceptedKinds"]))
	if len(accepted) == 0 {
		return nil
	}
	forbidden := stringSet(stringSliceFromValue(required["forbiddenKinds"]))
	ret := make([]string, 0, len(accepted))
	for _, kind := range accepted {
		kind = strings.TrimSpace(kind)
		if kind == "" || forbidden[kind] {
			continue
		}
		if boolValue(required["stateChangingForbidden"]) && requiredOutputKindIsStateChanging(kind) {
			continue
		}
		ret = append(ret, kind)
	}
	return compactUniqueStrings(ret)
}

func requiredOutputSatisfiableShapes(required map[string]any) []string {
	if len(required) == 0 {
		return nil
	}
	allowed := compactUniqueStrings(stringSliceFromValue(required["allowedOutputShapes"]))
	if len(allowed) == 0 {
		return nil
	}
	forbidden := stringSet(stringSliceFromValue(required["forbiddenOutputShapes"]))
	ret := make([]string, 0, len(allowed))
	for _, shape := range allowed {
		shape = strings.TrimSpace(shape)
		if shape == "" || forbidden[shape] {
			continue
		}
		ret = append(ret, shape)
	}
	return compactUniqueStrings(ret)
}

func requiredOutputRouteHasStateChangingKind(routes []string) bool {
	for _, kind := range routes {
		if requiredOutputKindIsStateChanging(kind) {
			return true
		}
	}
	return false
}

func requiredOutputRouteHasValidationKind(routes []string) bool {
	for _, kind := range routes {
		switch strings.TrimSpace(kind) {
		case "shell/command", "mcp/tool_call", managedValidationSessionNodeKind:
			return true
		}
	}
	return false
}

func requiredOutputRouteHasEvidenceKind(routes []string) bool {
	for _, kind := range routes {
		if strings.TrimSpace(kind) != "" {
			return true
		}
	}
	return false
}

func requiredOutputKindIsStateChanging(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "patch/apply", "artifact/write", "file/write", "file/delete", "git/write", "state_change":
		return true
	default:
		return false
	}
}

func requiredOutputShapesAllowAssessment(shapes []string) bool {
	for _, shape := range shapes {
		normalized := strings.ToLower(strings.TrimSpace(shape))
		if strings.Contains(normalized, "assessment") {
			return true
		}
	}
	return false
}

func scrubConvergenceDispatchDiagnosticAdvice(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	next := cloneStepPayload(input)
	if recovery := mapFromValue(next[stepInputKeyContractViolationRecovery]); len(recovery) > 0 {
		next[stepInputKeyContractViolationRecovery] = contractViolationDiagnosticOnlyMap(recovery, next)
	}
	guidance := stringSliceFromValue(next["repairGuidance"])
	if len(guidance) == 0 {
		return next
	}
	kept := make([]string, 0, len(guidance))
	removed := 0
	for _, item := range guidance {
		if outputShapeAdviceConflictsWithRequiredOutput(item, next) {
			removed++
			continue
		}
		kept = append(kept, item)
	}
	if removed == 0 {
		return next
	}
	if len(kept) > 0 {
		next["repairGuidance"] = kept
	} else {
		delete(next, "repairGuidance")
	}
	appendAuthorityDiagnostic(next, "stale_diagnostic_action_advice_removed", "info", "diagnostic_lineage", "repairGuidance", fmt.Sprintf("%d conflicting output-shape guidance item(s) removed before dispatch", removed))
	return next
}

func validateConvergenceDispatchOutputShapeAdvice(input map[string]any) error {
	if len(input) == 0 {
		return nil
	}
	for _, item := range stringSliceFromValue(input["repairGuidance"]) {
		if outputShapeAdviceConflictsWithRequiredOutput(item, input) {
			appendAuthorityDiagnostic(input, "output_shape_conflict", "error", "internal_authority_mismatch", "repairGuidance", "repairGuidance contradicts current requiredOutput.allowedOutputShapes")
			return convergenceDispatchMisalignmentError("output-shape advice conflicts with requiredOutput: repairGuidance")
		}
	}
	for _, source := range []map[string]any{
		mapFromValue(input[stepInputKeyContractViolationRecovery]),
		mapFromValue(input["contractViolationRecovery"]),
	} {
		if len(source) == 0 {
			continue
		}
		for _, key := range []string{"retryHint", "summary", "reason"} {
			if outputShapeAdviceConflictsWithRequiredOutput(stringFromMap(source, key), input) {
				appendAuthorityDiagnostic(input, "output_shape_conflict", "error", "internal_authority_mismatch", "contractViolationRecovery."+key, "contract violation diagnostic contradicts current requiredOutput.allowedOutputShapes")
				return convergenceDispatchMisalignmentError("output-shape advice conflicts with requiredOutput: contractViolationRecovery.%s", key)
			}
		}
	}
	return nil
}

func outputShapeAdviceConflictsWithRequiredOutput(text string, input map[string]any) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	required := currentRequiredOutputForAuthority(input)
	if len(required) == 0 {
		return false
	}
	if adviceSuggestsTopLevelShellCommands(text) && !requiredOutputAllowsTopLevelShape(required, "shellCommands") {
		return true
	}
	return false
}

func adviceSuggestsTopLevelShellCommands(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	lower = strings.NewReplacer("`", "", "'", "", "\"", "").Replace(lower)
	if lower == "" || strings.Contains(lower, "do not") || strings.Contains(lower, "forbid") || strings.Contains(lower, "forbidden") {
		return false
	}
	return strings.Contains(lower, "top-level shellcommands") ||
		strings.Contains(lower, "top level shellcommands") ||
		strings.Contains(lower, "return shellcommands") ||
		strings.Contains(lower, "produce shellcommands") ||
		strings.Contains(lower, "use shellcommands")
}

func requiredOutputAllowsTopLevelShape(required map[string]any, shape string) bool {
	if len(required) == 0 {
		return false
	}
	shape = strings.TrimSpace(shape)
	if shape == "" {
		return false
	}
	for _, value := range append(stringSliceFromValue(required["allowedOutputShapes"]), stringSliceFromValue(required["allowedOutputs"])...) {
		normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", ""))
		if normalized == "" {
			continue
		}
		if shape == "shellCommands" && (normalized == "shellcommands" || normalized == "toplevelshellcommands") {
			return true
		}
	}
	return false
}

func appendAuthorityDiagnostic(input map[string]any, code string, severity string, kind string, source string, message string) {
	if len(input) == 0 {
		return
	}
	item := map[string]any{
		"code":     strings.TrimSpace(code),
		"severity": strings.TrimSpace(severity),
		"kind":     strings.TrimSpace(kind),
		"source":   strings.TrimSpace(source),
		"message":  strings.TrimSpace(message),
	}
	for key, value := range item {
		if strings.TrimSpace(stringFromAny(value)) == "" {
			delete(item, key)
		}
	}
	if len(item) == 0 {
		return
	}
	diagnostics := mapSliceFromValue(input["authorityDiagnostics"])
	diagnostics = append(diagnostics, item)
	input["authorityDiagnostics"] = diagnostics
}

func firstRequiredOutputForSemantics(sources map[string]map[string]any) map[string]any {
	for _, label := range []string{
		"repairActionPackage.requiredOutput",
		"stagePacket.requiredOutput",
		"stagePacket.nextAction.requiredOutput",
		"currentActionContract.requiredOutput",
		"requiredOutput",
		"executionContext.repairActionPackage.requiredOutput",
		"executionContext.stagePacket.requiredOutput",
		"executionContext.stagePacket.nextAction.requiredOutput",
		"executionContext.requiredOutput",
	} {
		if required := sources[label]; len(required) > 0 {
			return required
		}
	}
	return nil
}

func requiredOutputRequiresStateChanging(required map[string]any) bool {
	if len(required) == 0 {
		return false
	}
	return boolValue(required["mustIncludeStateChangingNode"])
}

func requiredOutputValidationOnly(required map[string]any) bool {
	if len(required) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromMap(required, "contractViolationRequiredOutputShape")) == "bounded_validation_evidence" {
		return true
	}
	return boolValue(required["mustProduceConcreteValidationEvidence"]) && !requiredOutputRequiresStateChanging(required)
}

func requiredOutputRequiresConcreteFailureEvidence(required map[string]any) bool {
	if len(required) == 0 {
		return false
	}
	return boolValue(required["mustProduceConcreteFailureEvidence"]) ||
		boolValue(required["mustAcquireConcreteFailureEvidence"]) ||
		boolValue(required["concreteFailureEvidenceRequired"])
}

func requiredOutputAcceptsKind(required map[string]any, kind string) bool {
	return stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), kind)
}

func requiredOutputHasAllowedStateChangingKind(required map[string]any) bool {
	if len(required) == 0 || !requiredOutputRequiresStateChanging(required) {
		return false
	}
	accepted := compactUniqueStrings(stringSliceFromValue(required["acceptedKinds"]))
	if len(accepted) == 0 {
		return false
	}
	forbidden := stringSet(stringSliceFromValue(required["forbiddenKinds"]))
	for _, kind := range accepted {
		kind = strings.TrimSpace(kind)
		if kind != "" && !forbidden[kind] && requiredOutputKindIsStateChanging(kind) {
			return true
		}
	}
	return false
}

func validateRequiredOutputFocusAlignment(authorityFocus map[string]any, label string, required map[string]any) error {
	if len(authorityFocus) == 0 || len(required) == 0 {
		return nil
	}
	if err := validateRequiredOutputBindingMapFocusAlignment(authorityFocus, label, required, false); err != nil {
		return err
	}
	for _, binding := range requiredOutputNestedValidationEvidenceBindings(required) {
		if err := validateRequiredOutputBindingMapFocusAlignment(authorityFocus, label+".validationEvidence", binding, true); err != nil {
			return err
		}
	}
	return nil
}

func validateRequiredOutputBindingMapFocusAlignment(authorityFocus map[string]any, label string, binding map[string]any, validateTargetRefs bool) error {
	if len(authorityFocus) == 0 || len(binding) == 0 {
		return nil
	}
	for _, value := range []string{
		strings.TrimSpace(stringFromMap(binding, "coverageFocusId")),
		strings.TrimSpace(stringFromMap(binding, "focusId")),
	} {
		if value == "" {
			continue
		}
		if dispatchFocusAligned(authorityFocus, map[string]any{"id": value}) {
			continue
		}
		return convergenceDispatchMisalignmentError("focus mismatch: %s coverageFocusId=%s authority=%s", label, value, dispatchFocusDebugToken(authorityFocus))
	}
	for _, key := range []string{"coverageWorkItemIds", "coverageWorkItemIDs"} {
		if ids := stringSliceFromValue(binding[key]); len(ids) > 0 && !dispatchFocusAligned(authorityFocus, map[string]any{"coverageWorkItemIds": ids}) {
			return convergenceDispatchMisalignmentError("focus mismatch: %s %s=%s authority=%s", label, key, strings.Join(ids, ","), dispatchFocusDebugToken(authorityFocus))
		}
	}
	if refs := stringSliceFromValue(binding["structuredTargetRefs"]); len(refs) > 0 && !dispatchFocusRefsAligned(authorityFocus, refs) {
		return convergenceDispatchMisalignmentError("focus mismatch: %s structuredTargetRefs=%s authority=%s", label, strings.Join(refs, ","), dispatchFocusDebugToken(authorityFocus))
	}
	if refs := stringSliceFromValue(binding["allowedTargetRefs"]); len(refs) > 0 && !dispatchFocusRefsAligned(authorityFocus, refs) {
		return convergenceDispatchMisalignmentError("focus mismatch: %s allowedTargetRefs=%s authority=%s", label, strings.Join(refs, ","), dispatchFocusDebugToken(authorityFocus))
	}
	if validateTargetRefs {
		if refs := stringSliceFromValue(binding["targetRefs"]); len(refs) > 0 && !dispatchFocusRefsAligned(authorityFocus, refs) {
			return convergenceDispatchMisalignmentError("focus mismatch: %s targetRefs=%s authority=%s", label, strings.Join(refs, ","), dispatchFocusDebugToken(authorityFocus))
		}
	}
	return nil
}

func requiredOutputNestedValidationEvidenceBindings(value any) []map[string]any {
	var ret []map[string]any
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			if evidence := mapFromValue(typed["validationEvidence"]); len(evidence) > 0 {
				ret = append(ret, evidence)
			}
			if looksLikeValidationEvidenceBinding(typed) {
				ret = append(ret, cloneStepPayload(typed))
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case []map[string]any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return ret
}

func dispatchFocusRefsAligned(authorityFocus map[string]any, refs []string) bool {
	refs = compactUniqueStrings(refs)
	if len(authorityFocus) == 0 || len(refs) == 0 {
		return true
	}
	authorityRefs := stringSet(dispatchFocusBindingRefs(authorityFocus))
	if len(authorityRefs) == 0 {
		return true
	}
	for _, ref := range refs {
		if authorityRefs[ref] {
			return true
		}
	}
	return false
}

func dispatchFocusBindingRefs(focus map[string]any) []string {
	if len(focus) == 0 {
		return nil
	}
	var refs []string
	refs = append(refs, dispatchFocusWorkItemIDs(focus)...)
	refs = append(refs, stringSliceFromValue(focus["targetRefs"])...)
	refs = append(refs, stringSliceFromValue(focus["allowedTargetRefs"])...)
	refs = append(refs, stringSliceFromValue(focus["structuredTargetRefs"])...)
	for _, key := range []string{"id", "normalizedSourceKey", "sourceRequirementId", "sourcePath"} {
		if value := strings.TrimSpace(stringFromMap(focus, key)); value != "" {
			refs = append(refs, value)
		}
	}
	if aliases := stringSliceFromValue(focus["sourceRequirementAliases"]); len(aliases) > 0 {
		refs = append(refs, aliases...)
	}
	return compactUniqueStrings(refs)
}

func dispatchFocusAligned(authorityFocus map[string]any, candidate map[string]any) bool {
	authorityIDs := stringSet(dispatchFocusWorkItemIDs(authorityFocus))
	candidateIDs := dispatchFocusWorkItemIDs(candidate)
	if len(authorityIDs) == 0 || len(candidateIDs) == 0 {
		return strings.TrimSpace(stringFromMap(authorityFocus, "id")) == strings.TrimSpace(stringFromMap(candidate, "id"))
	}
	for _, id := range candidateIDs {
		if authorityIDs[id] {
			return true
		}
	}
	return false
}

func dispatchFocusWorkItemIDs(focus map[string]any) []string {
	if len(focus) == 0 {
		return nil
	}
	var ids []string
	ids = append(ids, stringSliceFromValue(focus["coverageWorkItemIds"])...)
	ids = append(ids, stringSliceFromValue(focus["coverageWorkItemIDs"])...)
	for _, key := range []string{"coverageWorkItemId", "coverageItemId", "workItemId"} {
		if value := strings.TrimSpace(stringFromMap(focus, key)); value != "" {
			ids = append(ids, value)
		}
	}
	if id := strings.TrimSpace(stringFromMap(focus, "id")); id != "" {
		ids = append(ids, strings.TrimPrefix(id, "focus-"))
	}
	return compactUniqueStrings(ids)
}

func dispatchFocusDebugToken(focus map[string]any) string {
	if len(focus) == 0 {
		return "empty"
	}
	if ids := dispatchFocusWorkItemIDs(focus); len(ids) > 0 {
		return strings.Join(ids, ",")
	}
	return strings.TrimSpace(stringFromMap(focus, "id"))
}

func gapBaselineFromRun(run *runModel.Run) map[string]any {
	if run == nil {
		return nil
	}
	authority := convergenceAuthoritySnapshotForRun(run)
	ctx := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])
	if len(ctx) == 0 {
		return nil
	}
	unresolved := mapSliceFromValue(ctx["unresolvedItems"])
	if len(unresolved) == 0 {
		unresolved = mapSliceFromValue(ctx["currentFocusItems"])
	}
	openFocus := gapBaselineOpenCoverageWorkItems(ctx, unresolved)
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	baseline := map[string]any{
		"schemaVersion":              gapBaselineSchema,
		"workspaceId":                string(run.WorkspaceID),
		"runId":                      string(run.ID),
		"definitionRevision":         stringFromMap(ctx, "definitionRevision"),
		"baselineRevision":           firstNonEmptyString(stringFromMap(ctx, "coverageBaselineHash"), stringFromMap(ctx, "baselineRevision")),
		"currentFocus":               cloneStepPayload(mapFromValue(ctx["currentFocus"])),
		"openCoverageWorkItems":      cloneCoverageMapSlice(openFocus),
		"openCoverageWorkItemCount":  len(unresolved),
		"openCoverageItemsTruncated": len(unresolved) > len(openFocus),
		"ledgerRef":                  firstNonEmptyString(stringFromMap(ctx, "fullLedgerRef"), "run.executionContext.coverageWorkItems"),
		"ledgerSummaryRef":           "run.executionContext.coverageWorkItemContext.ledgerSummary",
		"acceptedEvidenceRefs":       gapEvidenceRefsFromCoverageItems(items, true),
		"rejectedEvidenceRefs":       gapEvidenceRefsFromCoverageItems(items, false),
		"todoItems":                  gapTodoItemsFromCoverageContext(openFocus),
		"nextAction":                 authority.NextAction,
	}
	if strings.TrimSpace(authority.DefinitionRevision) != "" {
		baseline["definitionRevision"] = authority.DefinitionRevision
	}
	if strings.TrimSpace(authority.BaselineRevision) != "" {
		baseline["baselineRevision"] = authority.BaselineRevision
	}
	if len(authority.CurrentFocus) > 0 {
		baseline["currentFocus"] = cloneStepPayload(authority.CurrentFocus)
	}
	if len(authority.AcceptedEvidenceRefs) > 0 {
		baseline["acceptedEvidenceRefs"] = compactUniqueStrings(authority.AcceptedEvidenceRefs)
	}
	if len(authority.RejectedEvidenceRefs) > 0 {
		baseline["rejectedEvidenceRefs"] = compactUniqueStrings(authority.RejectedEvidenceRefs)
	}
	if len(mapFromValue(baseline["currentFocus"])) == 0 {
		delete(baseline, "currentFocus")
	}
	if len(mapSliceFromValue(baseline["openCoverageWorkItems"])) == 0 {
		delete(baseline, "openCoverageWorkItems")
	}
	if len(mapSliceFromValue(baseline["todoItems"])) == 0 {
		delete(baseline, "todoItems")
	}
	if len(stringSliceFromValue(baseline["acceptedEvidenceRefs"])) == 0 {
		delete(baseline, "acceptedEvidenceRefs")
	}
	if len(stringSliceFromValue(baseline["rejectedEvidenceRefs"])) == 0 {
		delete(baseline, "rejectedEvidenceRefs")
	}
	if stringFromMap(baseline, "definitionRevision") == "" {
		delete(baseline, "definitionRevision")
	}
	if stringFromMap(baseline, "baselineRevision") == "" {
		delete(baseline, "baselineRevision")
	}
	if len(baseline) <= 4 {
		return nil
	}
	return baseline
}

func gapBaselineOpenCoverageWorkItems(context map[string]any, unresolved []map[string]any) []map[string]any {
	if len(context) > 0 {
		if focus := mapSliceFromValue(context["currentFocusItems"]); len(focus) > 0 {
			return focus
		}
		if focus := mapFromValue(context["currentFocus"]); len(focus) > 0 {
			return []map[string]any{focus}
		}
	}
	return limitedCoverageMapSlice(unresolved, maxLLMContextSliceItems)
}

func limitedCoverageMapSlice(items []map[string]any, limit int) []map[string]any {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return cloneCoverageMapSlice(items)
}

func gapBaselineDispatchGate(run *runModel.Run) (bool, error) {
	if run == nil {
		return false, common.ErrInvalidArgument
	}
	if convergenceBudgetRequiresEscalation(run) {
		return true, common.ErrBlocked
	}
	baseline := gapBaselineMapFromValue(run.ExecutionContext[gapBaselineKey])
	if len(baseline) == 0 {
		return false, nil
	}
	nextAction := mapFromValue(baseline["nextAction"])
	switch strings.TrimSpace(stringFromMap(nextAction, "kind")) {
	case "", "assess_gap", "repair", "state_change", "validate", "validation", "assessment", "artifact":
		return false, nil
	case "wait_decision", "wait_approval", "request_decision", "request_approval":
		return true, common.ErrBlocked
	case "no_action", "complete":
		return true, nil
	case "fail_fast", "fail":
		return true, common.ErrConflict
	default:
		return true, common.ErrConflict
	}
}

func gapEvidenceRefsFromCoverageItems(items []map[string]any, accepted bool) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	refKey := "rejectedEvidenceRefs"
	byKindKey := "rejectedEvidenceByKind"
	if accepted {
		refKey = "acceptedEvidenceRefs"
		byKindKey = "acceptedEvidenceByKind"
	}
	for _, item := range items {
		for _, ref := range stringSliceFromValue(item[refKey]) {
			add(ref)
		}
		for _, refs := range mapStringSliceValue(item[byKindKey]) {
			for _, ref := range refs {
				add(ref)
			}
		}
	}
	return ret
}

func gapTodoItemsFromCoverageContext(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(items))
	for idx, item := range items {
		if len(item) == 0 {
			continue
		}
		id := stringFromMap(item, "id")
		if id == "" {
			id = "todo-" + coveragePaddedNumber(idx+1)
		}
		workItemIDs := stringSliceFromValue(item["coverageWorkItemIds"])
		if len(workItemIDs) == 0 {
			workItemIDs = stringSliceFromValue(item["coverageWorkItemIDs"])
		}
		if len(workItemIDs) == 0 && stringFromMap(item, "id") != "" {
			workItemIDs = []string{stringFromMap(item, "id")}
		}
		source := firstNonEmptyString(stringFromMap(item, "sourceScope"), stringFromMap(item, "source"), "coverage")
		status := firstNonEmptyString(stringFromMap(item, "status"), "open")
		state := deriveCoverageWorkItemState(item)
		todo := map[string]any{
			"id":                    id,
			"source":                source,
			"sourceRef":             gapTodoSourceRefFromCoverageItem(item),
			"coverageWorkItemIds":   workItemIDs,
			"requiredEvidenceKinds": state.MissingEvidenceKinds,
			"status":                status,
			"priority":              idx + 1,
			"reason":                gapTodoReasonFromCoverageItem(item),
		}
		metadata := gapTodoMetadataFromCoverageItem(item)
		if len(metadata) > 0 {
			todo["metadata"] = metadata
		}
		ret = append(ret, todo)
	}
	return ret
}

func gapNextActionForRun(run *runModel.Run, unresolved []map[string]any) map[string]any {
	if run == nil {
		return nil
	}
	switch run.State {
	case common.RunStateBlockedOnDecision:
		return map[string]any{"kind": "request_decision", "reason": "run waiting for decision"}
	case common.RunStateBlockedOnApproval:
		return map[string]any{"kind": "request_approval", "reason": "run waiting for approval"}
	case common.RunStateCompleted:
		return map[string]any{"kind": "complete", "reason": "run completed"}
	case common.RunStateFailed:
		return map[string]any{"kind": "fail", "reason": "run failed"}
	default:
		item := gapNextActionFocusItem(run, unresolved)
		if len(item) > 0 {
			if coverageWorkItemCanCloseDeterministically(item) {
				for _, candidate := range unresolved {
					if stringFromMap(candidate, "id") == stringFromMap(item, "id") {
						continue
					}
					if coverageWorkItemCanCloseDeterministically(candidate) {
						continue
					}
					return gapNextActionForCoverageItem(candidate)
				}
				return map[string]any{"kind": "complete", "reason": "current coverage work item is deterministically satisfied"}
			}
			return gapNextActionForCoverageItem(item)
		}
		return map[string]any{"kind": "complete", "reason": "no open coverage work items"}
	}
}

func gapNextActionFocusItem(run *runModel.Run, unresolved []map[string]any) map[string]any {
	if run == nil {
		return nil
	}
	ctx := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])
	if item := mapFromValue(ctx["currentFocus"]); len(item) > 0 {
		return item
	}
	if len(unresolved) > 0 {
		return unresolved[0]
	}
	for _, key := range []string{"currentFocusItems", "ledgerSummary"} {
		if items := mapSliceFromValue(ctx[key]); len(items) > 0 {
			return items[0]
		}
	}
	return nil
}

func gapNextActionForCoverageItem(item map[string]any) map[string]any {
	status := strings.ToLower(strings.TrimSpace(stringFromMap(item, "status")))
	if coverageWorkItemCanCloseDeterministically(item) {
		return map[string]any{"kind": "complete", "reason": "coverage work item is deterministically satisfied"}
	}
	if status == "blocked" {
		return map[string]any{"kind": "request_decision", "reason": "coverage work item is blocked"}
	}
	missing := stringSet(deriveCoverageWorkItemState(item).MissingEvidenceKinds)
	switch {
	case missing["approval"]:
		return map[string]any{"kind": "request_approval", "reason": "coverage work item requires approval evidence"}
	case missing["decision"] || missing["external_dependency"]:
		return map[string]any{"kind": "request_decision", "reason": "coverage work item requires governance evidence"}
	case missing["state_change"]:
		return map[string]any{"kind": "state_change", "reason": "coverage work item is missing state_change evidence"}
	case missing["validation"]:
		return map[string]any{"kind": "validate", "reason": "coverage work item is missing validation evidence"}
	case missing["artifact"]:
		return map[string]any{"kind": "artifact", "reason": "coverage work item is missing artifact evidence"}
	case missing["assessment"]:
		return map[string]any{"kind": "assessment", "reason": "coverage work item is missing assessment evidence"}
	case len(missing) > 0:
		return map[string]any{"kind": "repair", "reason": "coverage work item is missing evidence"}
	case status == "evidence_ready":
		return map[string]any{"kind": "assessment", "reason": "coverage work item evidence is ready for assessment"}
	default:
		return map[string]any{"kind": "assessment", "reason": "open coverage work item requires assessment"}
	}
}

func refreshRunConvergenceAuthority(run *runModel.Run) bool {
	if run == nil || run.ExecutionContext == nil {
		return false
	}
	context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])
	if len(context) == 0 {
		return false
	}
	focus := authoritativeCoverageFocusFromContext(run, context)
	if len(focus) == 0 {
		return false
	}
	changed := false
	currentFocus := mapFromValue(run.ExecutionContext[coverageFocusKey])
	stripCoverageFocusRuntimeDiagnostics(currentFocus)
	if !coverageFocusSameScope(currentFocus, focus) {
		run.ExecutionContext[coverageFocusKey] = focus
		resetConvergenceBudgetScopeForFocus(run.ExecutionContext, focus)
		changed = true
	} else if len(currentFocus) > 0 {
		next := cloneStepPayload(currentFocus)
		for _, key := range []string{"coverageWorkItemIds", "requiredEvidenceKinds", "acceptedEvidenceRefs", "rejectedEvidenceRefs", "targetRefs", "state"} {
			if value, ok := focus[key]; ok && value != nil {
				next[key] = cloneStepInputValue(value)
			}
		}
		stripCoverageFocusRuntimeDiagnostics(next)
		if stableCoverageJSON(currentFocus) != stableCoverageJSON(next) {
			run.ExecutionContext[coverageFocusKey] = next
			changed = true
		}
	}
	if budget := mapFromValue(run.ExecutionContext[convergenceBudgetKey]); len(budget) > 0 {
		before := stableCoverageJSON(budget)
		ensureConvergenceBudgetFocusScope(budget, mapFromValue(run.ExecutionContext[coverageFocusKey]))
		if before != stableCoverageJSON(budget) {
			run.ExecutionContext[convergenceBudgetKey] = budget
			changed = true
		}
	}
	authority := convergenceAuthoritySnapshotForRun(run).ToMap()
	if len(authority) > 0 && stableCoverageJSON(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])) != stableCoverageJSON(authority) {
		run.ExecutionContext[convergenceAuthorityKey] = authority
		changed = true
	}
	return changed
}

func convergenceAuthoritySnapshotForRun(run *runModel.Run) ConvergenceAuthoritySnapshot {
	if run == nil {
		return ConvergenceAuthoritySnapshot{}
	}
	context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	unresolved := mapSliceFromValue(context["unresolvedItems"])
	if len(unresolved) == 0 {
		unresolved = mapSliceFromValue(context["currentFocusItems"])
	}
	focus := authoritativeCoverageFocusFromContext(run, context)
	currentFocus := coverageWorkItemPrimaryFocus(context)
	nextAction := gapNextActionForRun(run, unresolved)
	budgetScope := convergenceAuthorityBudgetScope(run)
	return ConvergenceAuthoritySnapshot{
		SchemaVersion:           convergenceAuthoritySchema,
		WorkspaceID:             string(run.WorkspaceID),
		RunID:                   string(run.ID),
		DefinitionRevision:      stringFromMap(context, "definitionRevision"),
		BaselineRevision:        firstNonEmptyString(stringFromMap(context, "coverageBaselineHash"), stringFromMap(context, "baselineRevision")),
		CurrentFocus:            currentFocus,
		NextAction:              nextAction,
		CoverageWorkItemContext: coverageWorkItemContextAlignedToFocus(context, focus),
		AcceptedEvidenceRefs:    gapEvidenceRefsFromCoverageItems(items, true),
		RejectedEvidenceRefs:    gapEvidenceRefsFromCoverageItems(items, false),
		BudgetScope:             budgetScope,
	}
}

func convergenceAuthorityBudgetScope(run *runModel.Run) map[string]any {
	if run == nil {
		return nil
	}
	budget := mapFromValue(run.ExecutionContext[convergenceBudgetKey])
	focus := mapFromValue(run.ExecutionContext[coverageFocusKey])
	ret := map[string]any{}
	for _, key := range []string{"focusId", "sliceId", "state", "decision"} {
		if value := strings.TrimSpace(stringFromMap(budget, key)); value != "" {
			ret[key] = value
		}
	}
	if _, ok := ret["focusId"]; !ok {
		if value := strings.TrimSpace(stringFromMap(focus, "id")); value != "" {
			ret["focusId"] = value
		}
	}
	if _, ok := ret["sliceId"]; !ok {
		if value := strings.TrimSpace(stringFromMap(focus, "sliceId")); value != "" {
			ret["sliceId"] = value
		}
	}
	if ids := compactUniqueStrings(stringSliceFromValue(budget["coverageWorkItemIds"])); len(ids) > 0 {
		ret["coverageWorkItemIds"] = ids
	} else if ids := compactUniqueStrings(stringSliceFromValue(focus["coverageWorkItemIds"])); len(ids) > 0 {
		ret["coverageWorkItemIds"] = ids
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func authoritativeCoverageFocusFromContext(run *runModel.Run, context map[string]any) map[string]any {
	if len(context) == 0 {
		return nil
	}
	item := coverageWorkItemPrimaryFocus(context)
	if len(item) == 0 {
		return nil
	}
	sliceID := firstNonEmptyString(
		stringFromMap(mapFromValue(run.ExecutionContext[coverageFocusKey]), "sliceId"),
		stringFromMap(run.ExecutionContext, activeSliceIDKey),
		stringFromMap(item, "sliceId"),
	)
	return coverageFocusFromWorkItem(sliceID, item)
}

func coverageFocusSameScope(left, right map[string]any) bool {
	if len(left) == 0 || len(right) == 0 {
		return len(left) == len(right)
	}
	return strings.TrimSpace(stringFromMap(left, "id")) == strings.TrimSpace(stringFromMap(right, "id")) &&
		strings.TrimSpace(stringFromMap(left, "sliceId")) == strings.TrimSpace(stringFromMap(right, "sliceId"))
}

func resetConvergenceBudgetScopeForFocus(executionContext map[string]any, focus map[string]any) {
	if len(executionContext) == 0 || len(focus) == 0 {
		return
	}
	existing := mapFromValue(executionContext[convergenceBudgetKey])
	reset := map[string]any{}
	for key := range defaultConvergenceBudgetLimits() {
		if value := positiveIntFromValue(existing[key], 0); value > 0 {
			reset[key] = value
		}
	}
	if focusID := strings.TrimSpace(stringFromMap(focus, "id")); focusID != "" {
		reset["focusId"] = focusID
	}
	if sliceID := strings.TrimSpace(stringFromMap(focus, "sliceId")); sliceID != "" {
		reset["sliceId"] = sliceID
	}
	if ids := compactUniqueStrings(stringSliceFromValue(focus["coverageWorkItemIds"])); len(ids) > 0 {
		reset["coverageWorkItemIds"] = ids
	}
	reset["state"] = "tracking"
	reset["decision"] = "continue"
	executionContext[convergenceBudgetKey] = reset
}

func gapTodoSourceRefFromCoverageItem(item map[string]any) string {
	return firstNonEmptyString(
		stringFromMap(item, "sourceRef"),
		stringFromMap(item, "sourcePath"),
		stringFromMap(item, "sourceRequirementId"),
		stringFromMap(item, "id"),
	)
}

func gapTodoReasonFromCoverageItem(item map[string]any) string {
	return firstNonEmptyString(
		stringFromMap(item, "reason"),
		stringFromMap(item, "targetText"),
		stringFromMap(item, "summary"),
		"coverage work item remains open",
	)
}

func gapTodoMetadataFromCoverageItem(item map[string]any) map[string]any {
	ret := map[string]any{}
	for _, key := range []string{"definitionRevision", "evidenceRole", "sourceScope", "targetRef", "targetText"} {
		if value, ok := item[key]; ok && value != nil {
			ret[key] = cloneStepInputValue(value)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func cloneCoverageMapSlice(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if len(item) == 0 {
			continue
		}
		ret = append(ret, cloneStepPayload(item))
	}
	return ret
}

func coveragePaddedNumber(value int) string {
	return fmt.Sprintf("%03d", value)
}
