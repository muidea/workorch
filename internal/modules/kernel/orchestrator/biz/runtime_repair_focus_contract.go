package orchestrator

import (
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func applyRepairCoverageFocusContract(input map[string]any, pkg map[string]any, requiredOutput map[string]any) {
	if len(pkg) == 0 || len(requiredOutput) == 0 {
		return
	}
	focus := repairCoverageFocusFromInput(input)
	if len(focus) == 0 {
		return
	}
	focusID := strings.TrimSpace(stringFromMap(focus, "id"))
	workItemIDs := stringSliceFromValue(focus["coverageWorkItemIds"])
	if len(workItemIDs) == 0 {
		if id := strings.TrimSpace(stringFromMap(focus, "coverageWorkItemId")); id != "" {
			workItemIDs = []string{id}
		}
	}
	if focusID == "" && len(workItemIDs) == 0 {
		return
	}
	pkg[coverageFocusKey] = compactLLMContextMap(focus, 1)
	pkg["coverageFocusBindingRequired"] = true
	requiredOutput["mustBindCoverageFocus"] = true
	if focusID != "" {
		requiredOutput["coverageFocusId"] = focusID
	}
	if len(workItemIDs) > 0 {
		requiredOutput["coverageWorkItemIds"] = workItemIDs
	}
	if refs := compactUniqueStrings(append(
		stringSliceFromValue(focus["structuredTargetRefs"]),
		stringSliceFromValue(focus["targetRefs"])...,
	)); len(refs) > 0 {
		requiredOutput["structuredTargetRefs"] = refs
	}
	requiredOutput["coverageFocusBindingRule"] = "state-changing and validation repair outputs must declare coverageFocusId, focusId, coverageWorkItemIds, coverageWorkItemId, or coverageTargets matching repairActionPackage.coverageFocus"
	applyValidationEvidenceExampleFocus(requiredOutput, focus, workItemIDs)
}

func reapplyRepairCoverageFocusContract(input map[string]any, pkg map[string]any) {
	if len(input) == 0 || len(pkg) == 0 {
		return
	}
	requiredOutput := mapFromValue(pkg["requiredOutput"])
	if len(requiredOutput) == 0 {
		return
	}
	applyRepairCoverageFocusContract(input, pkg, requiredOutput)
	pkg["requiredOutput"] = requiredOutput
}

func repairCoverageFocusFromInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	for _, source := range []map[string]any{
		mapFromValue(input[coverageFocusKey]),
		mapFromValue(mapFromValue(input["acceptance"])["currentFocus"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])[coverageFocusKey]),
		mapFromValue(mapFromValue(mapFromValue(input["repairActionPackage"])["acceptance"])["currentFocus"]),
	} {
		if len(source) > 0 {
			return cloneStepPayload(source)
		}
	}
	contextFocus := mapFromValue(mapFromValue(input[coverageWorkItemContextKey])["currentFocus"])
	if len(contextFocus) == 0 {
		contextFocus = mapFromValue(mapFromValue(mapFromValue(input["repairActionPackage"])[coverageWorkItemContextKey])["currentFocus"])
	}
	if len(contextFocus) == 0 {
		return nil
	}
	if len(stringSliceFromValue(contextFocus["coverageWorkItemIds"])) > 0 ||
		strings.HasPrefix(strings.TrimSpace(stringFromMap(contextFocus, "id")), "focus-") {
		return cloneStepPayload(contextFocus)
	}
	itemID := strings.TrimSpace(stringFromMap(contextFocus, "id"))
	if itemID == "" {
		return cloneStepPayload(contextFocus)
	}
	return map[string]any{
		"id":                    "focus-" + itemID,
		"coverageWorkItemIds":   []string{itemID},
		"requiredEvidenceKinds": coverageMissingOrRequiredEvidenceKinds(contextFocus),
		"acceptedEvidenceRefs":  stringSliceFromValue(contextFocus["acceptedEvidenceRefs"]),
		"rejectedEvidenceRefs":  stringSliceFromValue(contextFocus["rejectedEvidenceRefs"]),
		"targetRefs":            acceptanceFocusTargetRefs(contextFocus),
		"state":                 firstNonEmptyString(stringFromMap(contextFocus, "state"), "active"),
	}
}

func validateLLMRepairCoverageFocusBindingContract(step *runModel.Step, output map[string]any) error {
	if step == nil || step.Input == nil || output == nil {
		return nil
	}
	contract := repairCoverageFocusBindingContractForStep(step)
	if len(contract.ExpectedTokens) == 0 {
		return nil
	}
	if repairOutputDeclaresStructuredBlocker(output) {
		return nil
	}
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
		if normalized, changed := normalizeRepairCoverageFocusGraphBinding(step, graph, contract); changed {
			output[stepOutputKeyExecutionGraph] = normalized
			graph = normalized
		}
		return validateRepairCoverageFocusGraphBinding(graph, contract)
	}
	requests, err := runtimePatchRequests(output)
	if err != nil {
		return err
	}
	if len(requests) > 0 {
		for _, request := range requests {
			if !repairPayloadBindsCoverageFocus(request.Extra, contract) &&
				!repairPayloadBindsCoverageFocus(map[string]any{"coverageTargets": request.CoverageTargets}, contract) {
				return repairCoverageFocusMismatchError(contract)
			}
		}
		return nil
	}
	if _, nextKind := nextStepSpec(output); canonicalRuntimeGraphNodeKind(nextKind) == "patch/apply" {
		if !repairPayloadBindsCoverageFocus(mapFromValue(output[stepOutputKeyNextInput]), contract) {
			return repairCoverageFocusMismatchError(contract)
		}
	}
	return nil
}

func normalizeRepairCoverageFocusGraphBinding(step *runModel.Step, graph runtimemodel.BlockExecutionGraph, contract repairCoverageFocusBindingContract) (runtimemodel.BlockExecutionGraph, bool) {
	if step == nil || len(contract.ExpectedTokens) == 0 || !stepAllowsCoverageFocusEvidenceNormalization(step) {
		return graph, false
	}
	if contract.FocusID == "" && len(contract.WorkItemIDs) != 1 && len(contract.TargetRefs) == 0 {
		return graph, false
	}
	changed := false
	nodes := append([]runtimemodel.BlockExecutionNode(nil), graph.Nodes...)
	for idx, node := range nodes {
		if !repairGraphNodeShouldBindCoverageFocus(node) {
			continue
		}
		if !repairGraphNodeAllowsCoverageFocusNormalization(step, node) {
			continue
		}
		if repairPayloadBindsCoverageFocus(node.Input, contract) {
			continue
		}
		nextInput := cloneStepPayload(node.Input)
		if nextInput == nil {
			nextInput = map[string]any{}
		}
		if contract.FocusID != "" {
			nextInput["coverageFocusId"] = contract.FocusID
		}
		if len(contract.WorkItemIDs) > 0 {
			nextInput["coverageWorkItemIds"] = append([]string(nil), contract.WorkItemIDs...)
			if len(contract.WorkItemIDs) == 1 {
				nextInput["coverageWorkItemId"] = contract.WorkItemIDs[0]
			}
		}
		if len(contract.TargetRefs) > 0 && len(stringSliceFromValue(nextInput["targetRefs"])) == 0 {
			nextInput["targetRefs"] = append([]string(nil), contract.TargetRefs...)
		}
		nodes[idx].Input = nextInput
		changed = true
	}
	if !changed {
		return graph, false
	}
	return runtimemodel.BlockExecutionGraph{Nodes: nodes}, true
}

func stepAllowsCoverageFocusEvidenceNormalization(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	if strings.TrimSpace(stringFromMap(step.Input, "repairObjective")) == repairObjectiveAcquireFailureEvidence {
		return true
	}
	pkg := mapFromValue(step.Input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	if boolValue(required["mustBindCoverageFocus"]) || boolValue(pkg["coverageFocusBindingRequired"]) {
		return true
	}
	if boolValue(pkg["evidenceAcquisitionRequired"]) || boolValue(pkg["concreteFailureEvidenceRequired"]) {
		return true
	}
	return boolValue(required["mustProduceConcreteValidationEvidence"]) ||
		boolValue(required["mustProduceConcreteFailureEvidence"]) ||
		boolValue(required["concreteFailureEvidenceRequired"])
}

func repairGraphNodeAllowsCoverageFocusNormalization(step *runModel.Step, node runtimemodel.BlockExecutionNode) bool {
	if step == nil || step.Input == nil {
		return false
	}
	if !inputRequiresStateChangingOutput(step.Input) {
		return true
	}
	return canonicalRuntimeGraphNodeKind(node.Kind) != "patch/apply"
}

type repairCoverageFocusBindingContract struct {
	FocusID        string
	WorkItemIDs    []string
	TargetRefs     []string
	ExpectedTokens []string
}

func repairCoverageFocusBindingContractForStep(step *runModel.Step) repairCoverageFocusBindingContract {
	if step == nil || step.Input == nil {
		return repairCoverageFocusBindingContract{}
	}
	pkg := mapFromValue(step.Input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustBindCoverageFocus"]) && !boolValue(pkg["coverageFocusBindingRequired"]) {
		return repairCoverageFocusBindingContract{}
	}
	focus := repairCoverageFocusFromInput(step.Input)
	if len(focus) == 0 {
		focus = mapFromValue(pkg[coverageFocusKey])
	}
	if len(focus) == 0 {
		return repairCoverageFocusBindingContract{}
	}
	return canonicalRepairCoverageFocusBindingContract(focus, required, pkg)
}

func canonicalRepairCoverageFocusBindingContract(focus map[string]any, required map[string]any, pkg map[string]any) repairCoverageFocusBindingContract {
	if len(focus) == 0 {
		return repairCoverageFocusBindingContract{}
	}
	seen := map[string]bool{}
	tokens := make([]string, 0)
	add := func(value string) {
		value = strings.TrimSpace(value)
		key := normalizeCoverageGapRepairToken(value)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		tokens = append(tokens, value)
	}
	focusID := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(focus, "id"),
		stringFromMap(required, "coverageFocusId"),
	))
	add(focusID)
	workItemIDs := stringSliceFromValue(focus["coverageWorkItemIds"])
	workItemIDs = append(workItemIDs, stringSliceFromValue(required["coverageWorkItemIds"])...)
	if id := strings.TrimSpace(stringFromMap(focus, "coverageWorkItemId")); id != "" {
		workItemIDs = append(workItemIDs, id)
	}
	for _, id := range compactUniqueStrings(workItemIDs) {
		add(id)
	}
	for _, ref := range stringSliceFromValue(focus["targetRefs"]) {
		add(ref)
	}
	for _, key := range []string{"coverageTargetToken", "goalRequirementId", "sourceRequirementId", "targetText", "sourcePath"} {
		add(stringFromMap(focus, key))
	}
	for _, item := range mapSliceFromValue(pkg["coverageTargetSet"]) {
		if focusTargetMatchesCoverageTarget(focus, item) {
			for _, key := range []string{"coverageTargetToken", "goalRequirementId", "sourceRequirementId", "targetText", "item", "raw", "requirement"} {
				add(stringFromMap(item, key))
			}
		}
	}
	if boolValue(required["mustDeclareCoverageTargets"]) || boolValue(pkg["coverageRepairRequired"]) {
		for _, item := range mapSliceFromValue(pkg["coverageTargetSet"]) {
			for _, key := range []string{"coverageWorkItemId", "id", "coverageTargetToken", "goalRequirementId", "sourceRequirementId", "targetText", "item", "raw", "requirement"} {
				add(stringFromMap(item, key))
			}
		}
	}
	return repairCoverageFocusBindingContract{
		FocusID:        focusID,
		WorkItemIDs:    compactUniqueStrings(workItemIDs),
		TargetRefs:     stringSliceFromValue(focus["targetRefs"]),
		ExpectedTokens: tokens,
	}
}

func focusTargetMatchesCoverageTarget(focus map[string]any, target map[string]any) bool {
	if len(focus) == 0 || len(target) == 0 {
		return false
	}
	focusIDs := stringSet(stringSliceFromValue(focus["coverageWorkItemIds"]))
	if id := strings.TrimSpace(stringFromMap(target, "coverageWorkItemId")); id != "" && focusIDs[id] {
		return true
	}
	if id := strings.TrimSpace(stringFromMap(target, "id")); id != "" && focusIDs[id] {
		return true
	}
	focusTokens := append([]string{}, stringSliceFromValue(focus["targetRefs"])...)
	for _, key := range []string{"coverageTargetToken", "goalRequirementId", "sourceRequirementId", "targetText", "sourcePath"} {
		focusTokens = append(focusTokens, stringFromMap(focus, key))
	}
	for _, token := range focusTokens {
		for _, key := range []string{"coverageTargetToken", "goalRequirementId", "sourceRequirementId", "targetText", "item", "raw", "requirement"} {
			if coverageRepairTokenMatches(token, stringFromMap(target, key)) {
				return true
			}
		}
	}
	return false
}

func validateRepairCoverageFocusGraphBinding(graph runtimemodel.BlockExecutionGraph, contract repairCoverageFocusBindingContract) error {
	if len(contract.ExpectedTokens) == 0 {
		return nil
	}
	actionableNodes := 0
	boundActionableNodes := 0
	for _, node := range graph.Nodes {
		kind := canonicalRuntimeGraphNodeKind(node.Kind)
		if !repairGraphNodeShouldBindCoverageFocus(node) {
			continue
		}
		actionableNodes++
		if repairPayloadBindsCoverageFocus(node.Input, contract) {
			boundActionableNodes++
			continue
		}
		if stepKindCanProduceStateChangingEvidence(kind) {
			return repairCoverageFocusMismatchError(contract)
		}
	}
	if actionableNodes > 0 && boundActionableNodes == 0 {
		return repairCoverageFocusMismatchError(contract)
	}
	return nil
}

func repairGraphNodeShouldBindCoverageFocus(node runtimemodel.BlockExecutionNode) bool {
	kind := canonicalRuntimeGraphNodeKind(node.Kind)
	switch kind {
	case "shell/command", "mcp/tool_call", managedValidationSessionNodeKind:
		return true
	default:
		return stepKindCanProduceStateChangingEvidence(kind)
	}
}

func applyValidationEvidenceExampleFocus(requiredOutput map[string]any, focus map[string]any, workItemIDs []string) {
	if len(requiredOutput) == 0 || len(focus) == 0 {
		return
	}
	targetRefs := compactUniqueStrings(append(
		append([]string{}, stringSliceFromValue(focus["structuredTargetRefs"])...),
		append(stringSliceFromValue(focus["targetRefs"]), workItemIDs...)...,
	))
	if len(targetRefs) == 0 {
		return
	}
	claims := stringSliceFromValue(focus["observedClaims"])
	if len(claims) == 0 {
		claims = []string{"runtime behavior was observed for the declared coverage focus"}
	}
	apply := func(evidence map[string]any) {
		if len(evidence) == 0 {
			return
		}
		evidence["targetRefs"] = append([]string(nil), targetRefs...)
		evidence["structuredTargetRefs"] = append([]string(nil), targetRefs...)
		evidence["observedClaims"] = append([]string(nil), claims...)
		if len(workItemIDs) > 0 {
			evidence["coverageWorkItemIds"] = append([]string(nil), workItemIDs...)
		}
	}
	if example := mapFromValue(requiredOutput["validationEvidenceNodeExample"]); len(example) > 0 {
		input := mapFromValue(example["input"])
		evidence := mapFromValue(input["validationEvidence"])
		apply(evidence)
		if len(evidence) > 0 {
			input["validationEvidence"] = evidence
			example["input"] = input
			requiredOutput["validationEvidenceNodeExample"] = example
		}
	}
	if example := mapFromValue(requiredOutput["managedValidationNodeExample"]); len(example) > 0 {
		input := mapFromValue(example["input"])
		observation := mapFromValue(input["observation"])
		evidence := mapFromValue(observation["validationEvidence"])
		apply(evidence)
		if len(evidence) > 0 {
			observation["validationEvidence"] = evidence
			input["observation"] = observation
			example["input"] = input
			requiredOutput["managedValidationNodeExample"] = example
		}
	}
}

func repairPayloadBindsCoverageFocus(payload map[string]any, contract repairCoverageFocusBindingContract) bool {
	if len(payload) == 0 || len(contract.ExpectedTokens) == 0 {
		return false
	}
	for _, token := range repairCoverageFocusBindingTokensFromPayload(payload) {
		for _, expected := range contract.ExpectedTokens {
			if coverageRepairTokenMatches(token, expected) {
				return true
			}
		}
	}
	return false
}

func repairCoverageFocusBindingTokensFromPayload(payload map[string]any) []string {
	if len(payload) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0)
	add := func(value string) {
		value = strings.TrimSpace(value)
		key := normalizeCoverageGapRepairToken(value)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		ret = append(ret, value)
	}
	var addMap func(map[string]any)
	addMap = func(item map[string]any) {
		for _, key := range []string{
			"coverageFocusId",
			"focusId",
			"coverageWorkItemId",
			"workItemId",
			"coverageTargetToken",
			"goalRequirementId",
			"sourceRequirementId",
			"targetText",
			"targetRef",
			"item",
			"raw",
			"requirement",
			"id",
			"stateChangingReason",
			"boundedReason",
			"verificationScope",
		} {
			add(stringFromMap(item, key))
		}
		for _, key := range []string{"coverageWorkItemIds", "workItemIds", "coverageTargets", "targetRefs", "affectedTargets", "relatedFailureTargets"} {
			for _, value := range stringSliceFromValue(item[key]) {
				add(value)
			}
			for _, nested := range mapSliceFromValue(item[key]) {
				addMap(nested)
			}
		}
	}
	addMap(payload)
	for _, key := range []string{"validationEvidence", "evidenceBinding", "coverageBinding", "focusBinding"} {
		addMap(mapFromValue(payload[key]))
	}
	return ret
}

func repairOutputDeclaresStructuredBlocker(output map[string]any) bool {
	if decisionBlockSpec(output).Reason != "" || approvalBlockSpec(output).Reason != "" {
		return true
	}
	state := strings.TrimSpace(strings.ToLower(firstNonEmptyString(
		stringFromMap(output, "completionState"),
		llmDecisionCompletionStateFromOutput(output),
	)))
	switch state {
	case "cannot_progress", "blocked", "blocked_on_decision", "blocked_on_approval":
		return true
	default:
		return false
	}
}

func repairCoverageFocusMismatchError(contract repairCoverageFocusBindingContract) error {
	target := firstNonEmptyString(contract.FocusID, strings.Join(contract.WorkItemIDs, ","))
	if target == "" {
		target = "current coverage focus"
	}
	return fmt.Errorf("%w: repair_focus_mismatch: repair output must bind state-changing or validation actions to %s", common.ErrInvalidArgument, target)
}
