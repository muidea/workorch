package orchestrator

import (
	"context"
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	gapBaselineKey             = "gapBaseline"
	gapBaselineSchema          = "workorch.gap_baseline.v1"
	convergenceAuthorityKey    = "convergenceAuthority"
	convergenceAuthoritySchema = "workorch.convergence_authority.v1"
)

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
	baseline := mapFromValue(run.ExecutionContext[gapBaselineKey])
	authority := mapFromValue(run.ExecutionContext[convergenceAuthorityKey])
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
		if len(context) > 0 {
			if existing := mapFromValue(input[coverageWorkItemContextKey]); len(existing) > 0 {
				input[coverageWorkItemContextKey] = coverageWorkItemContextAlignedToFocus(existing, focus)
			} else {
				input[coverageWorkItemContextKey] = coverageWorkItemContextAlignedToFocus(context, focus)
			}
		}
		if len(baseline) > 0 {
			input[gapBaselineKey] = cloneStepPayload(baseline)
		}
		if len(authority) > 0 {
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

func syncRunConvergenceDerivedFields(run *runModel.Run) bool {
	if run == nil || run.ExecutionContext == nil {
		return false
	}
	if len(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])) == 0 &&
		len(mapFromValue(run.ExecutionContext[gapBaselineKey])) == 0 {
		return false
	}
	input := runExecutionInput(run, "")
	if len(input) == 0 {
		return false
	}
	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)
	input = ApplyConvergenceContract(input, contract)
	input = attachStagePacketToStepInput(run, "", input)
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

func convergenceDerivedExecutionContextKeys() []string {
	return []string{
		"convergenceContract",
		stepInputKeyStagePacket,
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
		"repairActionPackage",
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
	if !dispatchInputHasConvergenceAuthority(run, input) {
		input = attachStagePacketToStepInput(run, stepID, input)
		return input, validateConvergenceDispatchInput(input)
	}
	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)
	input = ApplyConvergenceContract(input, contract)
	input = attachStagePacketToStepInput(run, stepID, input)
	if err := validateConvergenceDispatchInput(input); err != nil {
		return nil, err
	}
	return input, nil
}

func dispatchInputHasConvergenceAuthority(run *runModel.Run, input map[string]any) bool {
	if len(mapFromValue(input[convergenceAuthorityKey])) > 0 || len(mapFromValue(input[gapBaselineKey])) > 0 {
		return true
	}
	if executionContext := mapFromValue(input["executionContext"]); len(executionContext) > 0 {
		if len(mapFromValue(executionContext[convergenceAuthorityKey])) > 0 || len(mapFromValue(executionContext[gapBaselineKey])) > 0 {
			return true
		}
	}
	if run == nil || run.ExecutionContext == nil {
		return false
	}
	return len(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])) > 0 || len(mapFromValue(run.ExecutionContext[gapBaselineKey])) > 0
}

func validateConvergenceDispatchInput(input map[string]any) error {
	if len(input) == 0 {
		return nil
	}
	nextAction := strings.TrimSpace(convergenceNextActionKind(nil, input))
	if nextAction == "" {
		return nil
	}
	contract := mapFromValue(input["convergenceContract"])
	state := strings.TrimSpace(stringFromMap(contract, "state"))
	stateChanging := boolValue(input["stateChangingOutputRequired"]) || boolValue(contract["stateChangingOutputRequired"])
	switch nextAction {
	case "validate", "validation", "produce_validation_evidence", "artifact", "produce_artifact_evidence":
		if state != convergenceStateNeedValidation {
			return fmt.Errorf("%w: convergence dispatch mismatch: nextAction=%s contractState=%s", common.ErrConflict, nextAction, state)
		}
		if stateChanging {
			return fmt.Errorf("%w: convergence dispatch mismatch: validation action requires state-changing output", common.ErrConflict)
		}
	case "assessment", "complete_coverage_assessment":
		if state != convergenceStateNeedAssessment {
			return fmt.Errorf("%w: convergence dispatch mismatch: nextAction=%s contractState=%s", common.ErrConflict, nextAction, state)
		}
		if stateChanging {
			return fmt.Errorf("%w: convergence dispatch mismatch: assessment action requires state-changing output", common.ErrConflict)
		}
	case "state_change", "produce_state_change":
		if state != convergenceStateNeedStateChange {
			return fmt.Errorf("%w: convergence dispatch mismatch: nextAction=%s contractState=%s", common.ErrConflict, nextAction, state)
		}
		if !stateChanging {
			return fmt.Errorf("%w: convergence dispatch mismatch: state_change action lacks state-changing output contract", common.ErrConflict)
		}
	case "request_decision", "wait_decision", "request_approval", "wait_approval", "request_governance":
		if state != convergenceStateNeedGovernance {
			return fmt.Errorf("%w: convergence dispatch mismatch: governed action has contractState=%s", common.ErrConflict, state)
		}
		if stateChanging {
			return fmt.Errorf("%w: convergence dispatch mismatch: governed action requires state-changing output", common.ErrConflict)
		}
	}
	if len(mapFromValue(input[stepInputKeyStagePacket])) == 0 {
		return fmt.Errorf("%w: convergence dispatch mismatch: missing stagePacket", common.ErrConflict)
	}
	return nil
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
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	baseline := map[string]any{
		"schemaVersion":         gapBaselineSchema,
		"workspaceId":           string(run.WorkspaceID),
		"runId":                 string(run.ID),
		"definitionRevision":    stringFromMap(ctx, "definitionRevision"),
		"baselineRevision":      firstNonEmptyString(stringFromMap(ctx, "coverageBaselineHash"), stringFromMap(ctx, "baselineRevision")),
		"currentFocus":          cloneStepPayload(mapFromValue(ctx["currentFocus"])),
		"openCoverageWorkItems": cloneCoverageMapSlice(unresolved),
		"acceptedEvidenceRefs":  gapEvidenceRefsFromCoverageItems(items, true),
		"rejectedEvidenceRefs":  gapEvidenceRefsFromCoverageItems(items, false),
		"todoItems":             gapTodoItemsFromCoverageContext(unresolved),
		"nextAction":            authority.NextAction,
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

func gapBaselineDispatchGate(run *runModel.Run) (bool, error) {
	if run == nil {
		return false, common.ErrInvalidArgument
	}
	baseline := mapFromValue(run.ExecutionContext[gapBaselineKey])
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
