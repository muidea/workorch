package orchestrator

import (
	"fmt"
	"path/filepath"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func validateLLMRepairStrategyReassessmentContract(step *runModel.Step, output map[string]any) error {
	if step == nil || step.Input == nil || output == nil {
		return nil
	}
	if !stateChangingRepairContractActive(step) {
		return nil
	}
	reassessment := mapFromValue(step.Input["strategyReassessment"])
	if !boolValue(reassessment["required"]) {
		return nil
	}
	if capabilityPolicyGraphRepairSatisfiedForStrategy(step, output, reassessment) {
		return nil
	}
	candidates, hasPatch := strategyReassessmentDecisionCandidates(output)
	if !hasPatch {
		return nil
	}
	requiredFields := stringSliceFromValue(reassessment["requiredDecisionFields"])
	if len(requiredFields) == 0 {
		requiredFields = []string{"failedSurface", "rootCauseTarget", "repairTargetDecision", "repairMode", "forbiddenRepair", "verificationPlan"}
	}
	expectedSurfaces := strategyReassessmentExpectedSurfaces(step, reassessment)
	for _, candidate := range candidates {
		if len(candidate) == 0 {
			continue
		}
		if !strategyCandidateHasRequiredFields(candidate, requiredFields) {
			continue
		}
		expectedSurface, surfaceMatched := strategyCandidateMatchingExpectedSurface(candidate, expectedSurfaces)
		if len(expectedSurfaces) > 0 && !surfaceMatched {
			continue
		}
		repairMode := strategyRepairModeFromCandidate(candidate)
		if repairMode == "" {
			continue
		}
		if !strategyReassessmentRepairModeAllowed(reassessment, repairMode) {
			return fmt.Errorf("%w: strategyReassessment.required=true repairMode=%s is not allowed by current strategyReassessment.allowedRepairModes", common.ErrInvalidArgument, repairMode)
		}
		surfaceCorrection := strategyCandidateHasSurfaceContractCorrection(candidate, expectedSurface)
		switch repairMode {
		case strategyRepairModeContinueSameTarget:
			if strategyReassessmentRequiresRootCauseTargetShift(reassessment) &&
				!strategySameTargetContinuationAddressesCurrentFailure(step, candidate, expectedSurface) &&
				!strategyCandidateAddressesGroundedRootCauseTarget(step, candidate, expectedSurface) &&
				!surfaceCorrection {
				return fmt.Errorf("%w: strategyReassessment.required=true repairMode=continue_same_target_repair requires patch/apply to address current primary diagnostics or return a real governance blocker", common.ErrInvalidArgument)
			}
		case strategyRepairModeShiftRootCauseTarget:
			if !strategyRootCauseDistinguishableFromFailedSurface(candidate, expectedSurface) && !surfaceCorrection {
				continue
			}
			if strategyReassessmentRequiresRootCauseTargetShift(reassessment) &&
				strategyCandidatePatchesFailedSurface(candidate, expectedSurface) && !surfaceCorrection {
				return fmt.Errorf("%w: strategyReassessment.required=true repairMode=shift_root_cause_target requires repeated failed-surface repairs to patch a distinct root-cause target or return a real governance blocker", common.ErrInvalidArgument)
			}
		default:
			continue
		}
		if strategyFailedSurfaceRequiresBoundedVerification(expectedSurface, candidate) &&
			(strategyVerificationPlanAppearsUnbounded(strings.TrimSpace(stringFromAny(candidate["verificationPlan"]))) ||
				strategyOutputGraphUsesUnboundedVerificationForStep(step, output)) {
			return fmt.Errorf("%w: strategyReassessment.required=true requires bounded verification for scoped failed surface; unbounded verification without explicit target scope is not allowed in this repair decision", common.ErrInvalidArgument)
		}
		return nil
	}
	return fmt.Errorf("%w: strategyReassessment.required=true requires state-changing patch output to declare failedSurface/rootCauseTarget/repairTargetDecision/repairMode/forbiddenRepair/verificationPlan aligned with current failed surface", common.ErrInvalidArgument)
}

func capabilityPolicyGraphRepairSatisfiedForStrategy(step *runModel.Step, output map[string]any, reassessment map[string]any) bool {
	if step == nil || step.Input == nil || len(output) == 0 || len(reassessment) == 0 {
		return false
	}
	if !capabilityPolicyRepairInputActive(step.Input) {
		return false
	}
	if !strategyReassessmentSurfaceIsCapabilityPolicyFailure(reassessment) {
		return false
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok || len(graph.Nodes) == 0 || !coverageGapRepairOutputHasPatch(output) {
		return false
	}
	policies := capabilityPolicyRepairPoliciesFromInput(step.Input)
	if len(policies) == 0 {
		return false
	}
	repaired := false
	for _, policy := range policies {
		requiredAfter := stringSliceFromValue(policy["requiredAfterKinds"])
		if len(requiredAfter) == 0 || !graphHasAnyNodeKind(graph, requiredAfter) {
			continue
		}
		repaired = true
		if graphHasCapabilityPolicyFollowUp(graph, policy) || outputDeclaresCapabilityPolicySkip(output, policy) {
			continue
		}
		return false
	}
	return repaired
}

func capabilityPolicyRepairInputActive(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if boolValue(input["capabilityPolicyRepairRequired"]) {
		return true
	}
	pkg := mapFromValue(input["repairActionPackage"])
	return boolValue(pkg["capabilityPolicyRepairRequired"])
}

func strategyReassessmentSurfaceIsCapabilityPolicyFailure(reassessment map[string]any) bool {
	text := strings.ToLower(strings.Join([]string{
		stringFromMap(reassessment, "failedSurface"),
		stringFromMap(reassessment, "reason"),
		stringFromMap(reassessment, "failureClass"),
	}, "\n"))
	return strings.Contains(text, "capability usage policy") ||
		strings.Contains(text, "capabilitypolicyskips") ||
		strings.Contains(text, "capability policy")
}

func capabilityPolicyRepairPoliciesFromInput(input map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	var policies []map[string]any
	for _, contract := range []map[string]any{
		mapFromValue(input["capabilityPolicyRepairContract"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["capabilityPolicyRepairContract"]),
	} {
		policies = append(policies, mapSliceFromValue(contract["triggeredPolicies"])...)
	}
	if len(policies) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]map[string]any, 0, len(policies))
	for _, policy := range policies {
		if len(policy) == 0 {
			continue
		}
		key := capabilityUsagePolicyIdentityKey(policy)
		if key != "" {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		ret = append(ret, policy)
	}
	return ret
}

func strategyReassessmentExpectedSurfaces(step *runModel.Step, reassessment map[string]any) []string {
	seen := map[string]bool{}
	var ret []string
	add := func(value any) {
		text := strings.TrimSpace(stringFromAny(value))
		if text == "" || seen[text] {
			return
		}
		seen[text] = true
		ret = append(ret, text)
	}
	add(reassessment["failedSurface"])
	if step != nil && step.Input != nil {
		add(step.Input["failureTargetScope"])
		addCoverageTargets := func(value any) {
			for _, item := range mapSliceFromValue(value) {
				add(item["coverageTargetToken"])
				add(item["targetText"])
				add(item["item"])
				add(item["goalRequirementId"])
			}
			for _, item := range stringSliceFromValue(value) {
				add(item)
			}
		}
		addCoverageTargets(step.Input["coverageTargetSet"])
		addCoverageTargets(step.Input["missingDoDItems"])
		addCoverageTargets(step.Input["missingGoalCoverage"])
		actionPackage := mapFromValue(step.Input["repairActionPackage"])
		add(actionPackage["failureTargetScope"])
		for _, target := range stringSliceFromValue(actionPackage["repairTargetSet"]) {
			add(target)
		}
		for _, snapshot := range mapSliceFromValue(actionPackage["targetSnapshots"]) {
			add(snapshot["targetRef"])
		}
		addCoverageTargets(actionPackage["coverageTargetSet"])
		addCoverageTargets(actionPackage["missingDoDItems"])
		addCoverageTargets(actionPackage["missingGoalCoverage"])
		progress := mapFromValue(step.Input["currentExecutionProgress"])
		add(progress["failureTargetScope"])
		failure := mapFromValue(step.Input["currentFailure"])
		add(failure["failedSurface"])
	}
	return ret
}

func strategyCandidateMatchingExpectedSurface(candidate map[string]any, expectedSurfaces []string) (string, bool) {
	if len(expectedSurfaces) == 0 {
		return "", true
	}
	for _, expectedSurface := range expectedSurfaces {
		if strings.TrimSpace(expectedSurface) == "" {
			continue
		}
		if strategyFailedSurfaceMatches(candidate, expectedSurface) ||
			strategyCandidateTargetsMatchFailedSurface(candidate, expectedSurface) ||
			strategyCandidateCoverageTargetsMatchSurface(candidate, expectedSurface) {
			return expectedSurface, true
		}
	}
	return "", false
}

func strategyCandidateCoverageTargetsMatchSurface(candidate map[string]any, expectedSurface string) bool {
	if len(candidate) == 0 || strings.TrimSpace(expectedSurface) == "" {
		return false
	}
	for _, target := range stringSliceFromValue(candidate["coverageTargets"]) {
		if strategyFailedSurfaceTextMatches(target, expectedSurface) {
			return true
		}
	}
	return false
}

const (
	strategyRepairModeContinueSameTarget   = "continue_same_target_repair"
	strategyRepairModeShiftRootCauseTarget = "shift_root_cause_target"
)

func strategyRepairModeFromCandidate(candidate map[string]any) string {
	mode := strings.ToLower(strings.TrimSpace(stringFromAny(candidate["repairMode"])))
	switch mode {
	case strategyRepairModeContinueSameTarget, strategyRepairModeShiftRootCauseTarget:
		return mode
	case "":
		return ""
	}
	if strings.Contains(mode, "shift") || strings.Contains(mode, "root_cause") || strings.Contains(mode, "root-cause") {
		return strategyRepairModeShiftRootCauseTarget
	}
	if strings.Contains(mode, "continue") ||
		strings.Contains(mode, "same") ||
		strings.Contains(mode, "current") ||
		strings.Contains(mode, "snapshot") ||
		strings.Contains(mode, "exact") ||
		strings.Contains(mode, "minimal") ||
		strings.Contains(mode, "replace") ||
		strings.Contains(mode, "patch") {
		return strategyRepairModeContinueSameTarget
	}
	return ""
}

func strategySameTargetContinuationAddressesCurrentFailure(step *runModel.Step, candidate map[string]any, expectedSurface string) bool {
	if len(candidate) == 0 || !strategyCandidatePatchesFailedSurface(candidate, expectedSurface) {
		return false
	}
	request, err := runtimePatchRequestFromMap(candidate)
	if err != nil {
		return false
	}
	if contract := primaryDiagnosticRepairContractForStep(step); len(contract) > 0 && boolValue(contract["required"]) {
		return primaryDiagnosticPatchRequestAddressesContract(step, request, contract)
	}
	signals := mapFromValue(step.Input["structuredFailureSignals"])
	if len(signals) == 0 {
		actionPackage := mapFromValue(step.Input["repairActionPackage"])
		signals = mapFromValue(actionPackage["structuredFailureSignals"])
	}
	if primary := primaryDiagnosticRepairContractFromSignals(signals); len(primary) > 0 {
		return primaryDiagnosticPatchRequestAddressesContract(step, request, primary)
	}
	if strategyPatchUsesCurrentRepairTargetSnapshot(step, request) {
		return true
	}
	return strings.TrimSpace(stringFromAny(candidate["addressesFailureSignals"])) != "" ||
		len(stringSliceFromValue(candidate["addressesFailureSignals"])) > 0 ||
		strings.TrimSpace(stringFromAny(candidate["primaryDiagnosticResolution"])) != "" ||
		len(mapSliceFromValue(candidate["primaryDiagnosticResolutions"])) > 0
}

func strategyPatchUsesCurrentRepairTargetSnapshot(step *runModel.Step, request runtimePatchRequest) bool {
	if step == nil || step.Input == nil {
		return false
	}
	targetRef := normalizeRepairPatchTargetRef(request.TargetRef)
	if targetRef == "" {
		return false
	}
	actionPackage := mapFromValue(step.Input["repairActionPackage"])
	targetMatched := false
	for _, target := range stringSliceFromValue(actionPackage["repairTargetSet"]) {
		if repairPatchTargetRefMatches(targetRef, []string{target}) {
			targetMatched = true
			break
		}
	}
	if !targetMatched {
		for _, snapshot := range mapSliceFromValue(actionPackage["targetSnapshots"]) {
			if repairPatchTargetRefMatches(targetRef, []string{stringFromMap(snapshot, "targetRef")}) {
				targetMatched = true
				break
			}
		}
	}
	if !targetMatched {
		return false
	}
	switch strings.TrimSpace(request.PatchType) {
	case "text_replace":
		return request.OldText != "" && request.OldText != request.NewText
	case "unified_diff":
		return strings.TrimSpace(request.Diff) != ""
	case "file_write":
		return strings.TrimSpace(request.Content) != ""
	default:
		return false
	}
}

func strategyCandidateAddressesGroundedRootCauseTarget(step *runModel.Step, candidate map[string]any, expectedSurface string) bool {
	if step == nil || step.Input == nil || len(candidate) == 0 {
		return false
	}
	request, err := runtimePatchRequestFromMap(candidate)
	if err != nil {
		return false
	}
	targetRef := normalizeRepairPatchTargetRef(request.TargetRef)
	if targetRef == "" {
		return false
	}
	if rootCauseTargets := rootCauseTargetRefsFromStrategyValue(candidate["rootCauseTarget"]); len(rootCauseTargets) > 0 &&
		!repairPatchTargetRefMatches(targetRef, rootCauseTargets) {
		return false
	}
	targetRefs := repairPatchRelevantTargetRefs(step)
	surfaceRefs := strategySurfaceTargetRefs(expectedSurface)
	for _, surfaceRef := range surfaceRefs {
		targetRefs = append(targetRefs, surfaceRef)
	}
	targetRefs = uniqueProcessEvidenceStrings(targetRefs)
	if len(targetRefs) == 0 {
		return false
	}
	patchesFailedSurface := repairPatchTargetRefMatches(targetRef, surfaceRefs)
	if patchesFailedSurface {
		return false
	}
	if repairPatchRequestDeclaresRelatedFailureTargets(request, targetRefs) {
		return repairPatchTargetRefIsRelatedPrerequisite(step.Input, targetRef, targetRefs) ||
			repairPatchTargetRefIsGroundedRootCauseCandidate(step.Input, targetRef, targetRefs)
	}
	return repairPatchTargetRefIsRelatedPrerequisite(step.Input, targetRef, targetRefs) ||
		repairPatchTargetRefIsGroundedRootCauseCandidate(step.Input, targetRef, targetRefs)
}

func strategyCandidateHasSurfaceContractCorrection(candidate map[string]any, expectedSurface string) bool {
	if len(candidate) == 0 {
		return false
	}
	correction := mapFromValue(candidate["surfaceContractCorrection"])
	if len(correction) == 0 {
		return false
	}
	if declaredSurface := strings.TrimSpace(stringFromAny(correction["failedSurface"])); declaredSurface != "" {
		if expectedSurface != "" && !strategyFailedSurfaceTextMatches(declaredSurface, expectedSurface) {
			return false
		}
		if parentSurface := strings.TrimSpace(stringFromAny(candidate["failedSurface"])); parentSurface != "" && !strategyFailedSurfaceTextMatches(declaredSurface, parentSurface) {
			return false
		}
	}
	for _, field := range []string{"whySurfaceIsWrong", "contractConflict", "currentResourceEvidence", "boundedVerificationPlan"} {
		if !surfaceContractCorrectionFieldPresent(correction[field]) {
			return false
		}
	}
	if strategyVerificationPlanAppearsUnbounded(strings.TrimSpace(stringFromAny(correction["boundedVerificationPlan"]))) {
		return false
	}
	return true
}

func surfaceContractCorrectionFieldPresent(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
		return false
	case []any:
		for _, item := range typed {
			if surfaceContractCorrectionFieldPresent(item) {
				return true
			}
		}
		return false
	case map[string]any:
		return len(typed) > 0
	case []map[string]any:
		return len(typed) > 0
	default:
		return false
	}
}

func strategyReassessmentDecisionCandidates(output map[string]any) ([]map[string]any, bool) {
	var candidates []map[string]any
	hasPatch := false
	appendCandidate := func(value map[string]any) {
		if len(value) > 0 {
			candidates = append(candidates, value)
		}
	}
	if requests, err := runtimePatchRequests(output); err == nil {
		for _, request := range requests {
			hasPatch = true
			appendCandidate(mapFromValue(request.Extra))
		}
	}
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
		for _, node := range graph.Nodes {
			if strings.TrimSpace(node.Kind) != "patch/apply" {
				continue
			}
			if _, err := runtimePatchRequestFromMap(node.Input); err != nil {
				continue
			}
			hasPatch = true
			appendCandidate(mapFromValue(node.Input))
		}
	}
	_, nextKind := nextStepSpec(output)
	if strings.TrimSpace(nextKind) == "patch/apply" {
		nextInput := mapFromValue(output[stepOutputKeyNextInput])
		if _, err := runtimePatchRequestFromMap(nextInput); err == nil {
			hasPatch = true
			appendCandidate(nextInput)
		}
	}
	return candidates, hasPatch
}

func strategyCandidateHasRequiredFields(candidate map[string]any, required []string) bool {
	for _, field := range required {
		if !strategyCandidateFieldPresent(candidate[field]) {
			return false
		}
	}
	return true
}

func strategyCandidateFieldPresent(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
		return false
	case []any:
		for _, item := range typed {
			if strategyCandidateFieldPresent(item) {
				return true
			}
		}
		return false
	case map[string]any:
		return len(typed) > 0
	case []map[string]any:
		return len(typed) > 0
	case bool:
		return typed
	case int:
		return typed != 0
	case int8:
		return typed != 0
	case int16:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case uint:
		return typed != 0
	case uint8:
		return typed != 0
	case uint16:
		return typed != 0
	case uint32:
		return typed != 0
	case uint64:
		return typed != 0
	case float32:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

func strategyFailedSurfaceMatches(candidate map[string]any, expected string) bool {
	got := strings.TrimSpace(stringFromAny(candidate["failedSurface"]))
	return strategyFailedSurfaceTextMatches(got, expected)
}

func strategyFailedSurfaceTextMatches(got string, expected string) bool {
	got = normalizeStrategyContractText(got)
	expected = normalizeStrategyContractText(expected)
	if got == "" || expected == "" {
		return false
	}
	return got == expected || strings.Contains(got, expected) || strings.Contains(expected, got)
}

func strategyCandidateTargetsMatchFailedSurface(candidate map[string]any, expectedSurface string) bool {
	expectedRefs := strategySurfaceTargetRefs(expectedSurface)
	if len(expectedRefs) == 0 {
		return false
	}
	for _, key := range []string{"targetRef", "rootCauseTarget", "failedSurface"} {
		for _, ref := range strategySurfaceTargetRefs(stringFromAny(candidate[key])) {
			if strategyTargetRefSetContains(expectedRefs, ref) {
				return true
			}
		}
	}
	return false
}

func strategySurfaceTargetRefs(text string) []string {
	var refs []string
	seen := map[string]bool{}
	add := func(ref string) {
		ref = filepath.ToSlash(filepath.Clean(strings.TrimSpace(ref)))
		if ref == "." || ref == "" || seen[ref] {
			return
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	for _, ref := range repairPatchTargetRefsFromScopeText(text) {
		add(ref)
	}
	for _, ref := range extractStructuredFailureTargetRefs(text) {
		add(ref)
	}
	for _, field := range strings.Fields(text) {
		field = strings.Trim(field, "`'\"()[]{}<>;,：:")
		if strings.Contains(field, ".") && !strings.Contains(field, "://") {
			add(field)
		}
	}
	return refs
}

func strategyTargetRefSetContains(refs []string, candidate string) bool {
	candidate = filepath.ToSlash(filepath.Clean(strings.TrimSpace(candidate)))
	if candidate == "" || candidate == "." {
		return false
	}
	for _, ref := range refs {
		if ref == candidate {
			return true
		}
	}
	return false
}

func strategyRootCauseDistinguishableFromFailedSurface(candidate map[string]any, expectedSurface string) bool {
	rootCause := normalizeStrategyContractText(strings.TrimSpace(stringFromAny(candidate["rootCauseTarget"])))
	if rootCause == "" {
		return false
	}
	failedSurface := strings.TrimSpace(stringFromAny(candidate["failedSurface"]))
	if failedSurface == "" {
		failedSurface = expectedSurface
	}
	failedSurface = normalizeStrategyContractText(failedSurface)
	if failedSurface == "" {
		return true
	}
	return rootCause != failedSurface
}

func strategyReassessmentRequiresRootCauseTargetShift(reassessment map[string]any) bool {
	if len(reassessment) == 0 {
		return false
	}
	if int64FromValue(reassessment["targetRepairAttempts"]) >= 2 {
		return true
	}
	reason := strings.ToLower(strings.TrimSpace(stringFromAny(reassessment["reason"])))
	return strings.Contains(reason, "same_target")
}

func strategyReassessmentRepairModeAllowed(reassessment map[string]any, mode string) bool {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return false
	}
	allowed := strategyReassessmentAllowedRepairModes(reassessment)
	if len(allowed) == 0 {
		return true
	}
	return stringSliceContains(allowed, mode)
}

func strategyReassessmentAllowedRepairModes(reassessment map[string]any) []string {
	if len(reassessment) == 0 {
		return nil
	}
	var ret []string
	for _, mode := range append(stringSliceFromValue(reassessment["allowedRepairModes"]), stringSliceFromValue(reassessment["requiredRepairModes"])...) {
		mode = strategyRepairModeFromCandidate(map[string]any{"repairMode": mode})
		if mode == "" {
			continue
		}
		ret = append(ret, mode)
	}
	return compactUniqueStrings(ret)
}

func strategyCandidatePatchesFailedSurface(candidate map[string]any, expectedSurface string) bool {
	if len(candidate) == 0 {
		return false
	}
	targetRef := normalizeRepairPatchTargetRef(strings.TrimSpace(stringFromAny(candidate["targetRef"])))
	if targetRef == "" {
		return false
	}
	surface := strings.TrimSpace(stringFromAny(candidate["failedSurface"]))
	if surface == "" {
		surface = expectedSurface
	}
	for _, surfaceTarget := range repairPatchTargetRefsFromScopeText(surface) {
		if repairPatchTargetRefMatches(targetRef, []string{surfaceTarget}) {
			return true
		}
	}
	return false
}

func strategyFailedSurfaceRequiresBoundedVerification(expectedSurface string, candidate map[string]any) bool {
	surface := strings.TrimSpace(stringFromAny(candidate["failedSurface"]))
	if surface == "" {
		surface = expectedSurface
	}
	surface = strings.ToLower(surface)
	if surface == "" {
		return false
	}
	return strings.Contains(surface, "failuretargets=") || strings.Contains(surface, "targetref=")
}

func strategyVerificationPlanAppearsUnbounded(plan string) bool {
	lower := strings.ToLower(strings.TrimSpace(plan))
	if lower == "" {
		return false
	}
	normalized := normalizeStrategyContractText(lower)
	return strings.Contains(lower, "full regression") ||
		strings.Contains(lower, "run all checks") ||
		strings.Contains(lower, "workspace wide") ||
		strings.Contains(lower, "entire workspace") ||
		strings.Contains(lower, "global scan") ||
		strings.Contains(normalized, "fullregression") ||
		strings.Contains(normalized, "runallchecks") ||
		strings.Contains(normalized, "workspacewide") ||
		strings.Contains(normalized, "entireworkspace") ||
		strings.Contains(normalized, "globalscan")
}

func strategyOutputGraphUsesUnboundedVerification(output map[string]any) bool {
	return strategyOutputGraphUsesUnboundedVerificationForTargets(output, nil)
}

func strategyOutputGraphUsesUnboundedVerificationForStep(step *runModel.Step, output map[string]any) bool {
	return strategyOutputGraphUsesUnboundedVerificationForTargets(output, scopedRepairVerificationTargetRefs(step))
}

func strategyOutputGraphUsesUnboundedVerificationForTargets(output map[string]any, scopedTargets []string) bool {
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok || len(graph.Nodes) == 0 {
		return false
	}
	nodeByID := map[string]runtimemodel.BlockExecutionNode{}
	for _, node := range graph.Nodes {
		nodeByID[strings.TrimSpace(node.ID)] = node
	}
	for _, node := range graph.Nodes {
		if strings.TrimSpace(node.Kind) != "shell/command" {
			continue
		}
		args := stringSliceFromValue(node.Input["args"])
		hasBroadScope := false
		hasExplicitScope := false
		for _, arg := range args {
			a := strings.ToLower(strings.TrimSpace(arg))
			if a == "" {
				continue
			}
			if a == "./..." || a == "..." || a == "*" || strings.Contains(a, "all") {
				hasBroadScope = true
			}
			if strings.HasPrefix(a, "--scope=") ||
				strings.HasPrefix(a, "--target=") ||
				strings.HasPrefix(a, "--path=") ||
				strings.HasPrefix(a, "--filter=") ||
				strings.HasPrefix(a, "--match=") ||
				strings.HasPrefix(a, "-run") {
				hasExplicitScope = true
			}
		}
		if shellCommandInputHasExplicitVerificationScope(node.Input) {
			hasExplicitScope = true
		}
		if hasBroadScope && !hasExplicitScope && graphNodeDependsOnScopedTargetPatch(node, nodeByID, scopedTargets, map[string]bool{}) {
			hasExplicitScope = true
		}
		if hasBroadScope && !hasExplicitScope {
			return true
		}
	}
	return false
}

func shellCommandInputHasExplicitVerificationScope(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	for _, key := range []string{"targetRef", "verificationScope", "boundedReason", "failedSurface", "rootCauseTarget"} {
		if strings.TrimSpace(stringFromAny(input[key])) != "" {
			return true
		}
	}
	for _, key := range []string{"relatedFailureTargets", "targetRefs", "affectedTargets"} {
		if len(stringSliceFromValue(input[key])) > 0 {
			return true
		}
	}
	return false
}

func graphNodeDependsOnScopedTargetPatch(node runtimemodel.BlockExecutionNode, nodeByID map[string]runtimemodel.BlockExecutionNode, scopedTargets []string, seen map[string]bool) bool {
	if len(scopedTargets) == 0 || len(node.DependsOn) == 0 {
		return false
	}
	for _, depID := range node.DependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" || seen[depID] {
			continue
		}
		seen[depID] = true
		dep, ok := nodeByID[depID]
		if !ok {
			continue
		}
		if strings.TrimSpace(dep.Kind) == "patch/apply" {
			targetRef := normalizeRepairPatchTargetRef(strings.TrimSpace(stringFromAny(dep.Input["targetRef"])))
			if repairPatchTargetRefMatches(targetRef, scopedTargets) {
				return true
			}
			for _, related := range stringSliceFromValue(dep.Input["relatedFailureTargets"]) {
				if repairPatchTargetRefMatches(normalizeRepairPatchTargetRef(related), scopedTargets) {
					return true
				}
			}
		}
		if graphNodeDependsOnScopedTargetPatch(dep, nodeByID, scopedTargets, seen) {
			return true
		}
	}
	return false
}

func scopedRepairVerificationTargetRefs(step *runModel.Step) []string {
	if step == nil || step.Input == nil {
		return nil
	}
	var ret []string
	add := func(value string) {
		for _, ref := range repairPatchTargetRefsFromScopeText(value) {
			ret = append(ret, ref)
		}
		if normalized := normalizeRepairPatchTargetRef(value); normalized != "" && !strings.Contains(value, "=") {
			ret = append(ret, normalized)
		}
	}
	add(stringFromMap(step.Input, "failureTargetScope"))
	add(stringFromMap(step.Input, failureOutputKeySignature))
	pkg := mapFromValue(step.Input["repairActionPackage"])
	for _, key := range []string{"failureTargetScope", failureOutputKeySignature} {
		add(stringFromMap(pkg, key))
	}
	if currentFailure := mapFromValue(pkg["currentFailure"]); len(currentFailure) > 0 {
		for _, key := range []string{"targetRef", "targetScope", "failureSignature"} {
			add(stringFromMap(currentFailure, key))
		}
	}
	for _, target := range mapSliceFromValue(pkg["repairTargetSet"]) {
		add(stringFromMap(target, "targetRef"))
	}
	for _, value := range stringSliceFromValue(pkg["repairTargetSet"]) {
		add(value)
	}
	for _, target := range mapSliceFromValue(pkg["targetSnapshots"]) {
		add(stringFromMap(target, "targetRef"))
	}
	return uniqueProcessEvidenceStrings(ret)
}

func normalizeStrategyContractText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "", "_", "", "/", "", "\\", "", ".", "")
	return replacer.Replace(value)
}

func repairPatchRequestDeclaresRelatedFailureTargets(request runtimePatchRequest, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}
	for _, declared := range stringSliceFromValue(request.Extra["relatedFailureTargets"]) {
		if repairPatchTargetRefMatches(declared, allowed) {
			return true
		}
	}
	return false
}

func repairPatchTargetRefIsRelatedPrerequisite(input map[string]any, targetRef string, allowed []string) bool {
	targetRef = normalizeRepairPatchTargetRef(targetRef)
	if len(input) == 0 || targetRef == "" || len(allowed) == 0 {
		return false
	}
	if !repairFailureIndicatesDependencyMetadataPrerequisite(input) || !dependencyMetadataTargetRef(targetRef) {
		return false
	}
	for _, candidate := range allowed {
		if repairPatchTargetsShareDirectory(targetRef, candidate) {
			return true
		}
	}
	return false
}

func repairPatchTargetRefIsGroundedRootCauseCandidate(input map[string]any, targetRef string, allowed []string) bool {
	targetRef = normalizeRepairPatchTargetRef(targetRef)
	if len(input) == 0 || targetRef == "" || len(allowed) == 0 {
		return false
	}
	reassessment := mapFromValue(input["strategyReassessment"])
	if !strategyReassessmentRequiresRootCauseTargetShift(reassessment) {
		return false
	}
	for _, item := range mapSliceFromValue(input["rootCauseCandidateContext"]) {
		if len(item) == 0 || !boolValue(item["exists"]) {
			continue
		}
		candidate := normalizeRepairPatchTargetRef(stringFromMap(item, "targetRef"))
		if candidate == "" || !repairPatchTargetRefMatches(targetRef, []string{candidate}) {
			continue
		}
		if rootCauseCandidateHasNeutralRelationEvidence(item, targetRef, allowed) {
			return true
		}
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	for _, item := range mapSliceFromValue(actionPackage["rootCauseCandidateContext"]) {
		if len(item) == 0 || !boolValue(item["exists"]) {
			continue
		}
		candidate := normalizeRepairPatchTargetRef(stringFromMap(item, "targetRef"))
		if candidate == "" || !repairPatchTargetRefMatches(targetRef, []string{candidate}) {
			continue
		}
		if rootCauseCandidateHasNeutralRelationEvidence(item, targetRef, allowed) {
			return true
		}
	}
	return false
}

func rootCauseCandidateHasNeutralRelationEvidence(item map[string]any, targetRef string, allowed []string) bool {
	source := strings.ToLower(strings.TrimSpace(stringFromMap(item, "source")))
	if source == "" {
		return false
	}
	for _, token := range []string{"same_directory", "structured_failure", "patch_repair", "manifest", "resource_shape", "root_cause"} {
		if strings.Contains(source, token) {
			return true
		}
	}
	for _, candidate := range allowed {
		if repairPatchTargetsShareDirectory(targetRef, candidate) {
			return true
		}
	}
	return false
}

func repairFailureIndicatesDependencyMetadataPrerequisite(input map[string]any) bool {
	texts := []string{
		stringFromMap(input, "failureReason"),
		stringFromMap(input, "failureTargetScope"),
		stringFromMap(input, failureOutputKeySignature),
		stringFromMap(input, stepInputKeyRepairOfFailureSig),
	}
	for _, key := range []string{"failedStepOutput", "previousStepOutput", "structuredFailureSignals", "repairActionPackage"} {
		if text := flattenCoverageEvidenceText(input[key], 0); text != "" {
			texts = append(texts, text)
		}
	}
	text := strings.ToLower(strings.Join(texts, "\n"))
	if text == "" {
		return false
	}
	hasFailure := strings.Contains(text, "missing") ||
		strings.Contains(text, "not found") ||
		strings.Contains(text, "no such") ||
		strings.Contains(text, "required") ||
		strings.Contains(text, "checksum") ||
		strings.Contains(text, "lock")
	hasPrerequisite := strings.Contains(text, "depend") ||
		strings.Contains(text, "module") ||
		strings.Contains(text, "package") ||
		strings.Contains(text, "manifest") ||
		strings.Contains(text, "metadata") ||
		strings.Contains(text, "checksum") ||
		strings.Contains(text, "lock")
	return hasFailure && hasPrerequisite
}

func dependencyMetadataTargetRef(targetRef string) bool {
	targetRef = normalizeRepairPatchTargetRef(targetRef)
	if targetRef == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(targetRef))
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(base)), ".")
	stem := strings.TrimSuffix(base, "."+ext)
	for _, token := range []string{
		"dependency",
		"dependencies",
		"deps",
		"lock",
		"checksum",
		"checksums",
		"sum",
		"sums",
		"manifest",
		"module",
		"modules",
		"package",
		"packages",
		"requirements",
	} {
		if base == token || stem == token || ext == token ||
			strings.Contains(base, token+".") ||
			strings.Contains(base, "."+token) ||
			strings.Contains(base, "-"+token) ||
			strings.Contains(base, "_"+token) {
			return true
		}
	}
	return false
}

func repairPatchTargetsShareDirectory(targetRef string, candidate string) bool {
	targetRef = normalizeRepairPatchTargetRef(targetRef)
	candidate = normalizeRepairPatchTargetRef(candidate)
	if targetRef == "" || candidate == "" {
		return false
	}
	targetDir := filepath.ToSlash(filepath.Dir(targetRef))
	candidateDir := filepath.ToSlash(filepath.Dir(candidate))
	if targetDir == "." {
		targetDir = ""
	}
	if candidateDir == "." {
		candidateDir = ""
	}
	return targetDir == candidateDir
}
