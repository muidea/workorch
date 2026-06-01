package orchestrator

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func validateLLMCoverageBoundStateChangingIntent(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	if !coverageBoundStateChangingOutputRequired(run, step) {
		return nil
	}
	if coverageGapRepairOutputHasPatch(output) && coverageBoundOutputDeclaresTargets(step, output) {
		return nil
	}
	if !coverageGapRepairOutputHasPatch(output) {
		return nil
	}
	return fmt.Errorf("%w: coverage-bound state-changing patch/apply must declare coverageTargets matching current Goal, DoD, or deliverable coverage targets", common.ErrInvalidArgument)
}

func validatePatchStepCoverageTargetsBeforeRuntime(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	if !coverageBoundStateChangingOutputRequired(run, step) {
		return nil
	}
	if !coverageGapRepairOutputHasPatch(output) {
		return nil
	}
	if coverageBoundOutputDeclaresTargets(step, output) {
		return nil
	}
	return fmt.Errorf("%w: coverage-bound state-changing patch/apply must declare coverageTargets matching current Goal, DoD, or deliverable coverage targets", common.ErrInvalidArgument)
}

func coverageGapRepairOutputHasPatch(output map[string]any) bool {
	if output == nil {
		return false
	}
	if outputItemCount(output[stepOutputKeyPatches]) > 0 ||
		outputItemCount(output["patchRequests"]) > 0 ||
		outputItemCount(mapFromValue(output[stepOutputKeyLLMDecision])["patchRequests"]) > 0 {
		return true
	}
	_, nextKind := nextStepSpec(output)
	if strings.TrimSpace(nextKind) == "patch/apply" {
		return true
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok {
		return false
	}
	for _, node := range graph.Nodes {
		if strings.TrimSpace(node.Kind) == "patch/apply" {
			return true
		}
	}
	return false
}

func coverageBoundOutputDeclaresTargets(step *runModel.Step, output map[string]any) bool {
	allowedTargets := coverageBoundTargetTokens(step)
	if len(allowedTargets) == 0 {
		return true
	}
	if patchPayloadsDeclareCoverageTargets(output[stepOutputKeyPatches], allowedTargets) ||
		patchPayloadsDeclareCoverageTargets(output["patchRequests"], allowedTargets) ||
		patchPayloadsDeclareCoverageTargets(mapFromValue(output[stepOutputKeyLLMDecision])["patchRequests"], allowedTargets) {
		return true
	}
	_, nextKind := nextStepSpec(output)
	if strings.TrimSpace(nextKind) == "patch/apply" && patchNodeDeclaresCoverageTargets(mapFromValue(output[stepOutputKeyNextInput]), allowedTargets) {
		return true
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if ok {
		for _, node := range graph.Nodes {
			if strings.TrimSpace(node.Kind) != "patch/apply" {
				continue
			}
			if patchNodeDeclaresCoverageTargets(node.Input, allowedTargets) {
				return true
			}
		}
	}
	return false
}

func patchPayloadsDeclareCoverageTargets(value any, allowed []string) bool {
	for _, item := range mapSliceFromValue(value) {
		if patchNodeDeclaresCoverageTargets(item, allowed) {
			return true
		}
	}
	return false
}

func coverageBoundStateChangingOutputRequired(_ *runModel.Run, step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	if !coverageBoundInputCarriesCoverageGap(step.Input) {
		return false
	}
	return len(coverageBoundTargetTokens(step)) > 0
}

func coverageBoundInputCarriesCoverageGap(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if len(mapSliceFromValue(input["missingDoDItems"])) > 0 ||
		len(missingGoalCoverageFromInput(input)) > 0 ||
		len(mapSliceFromValue(input["missingDeliverableCoverage"])) > 0 ||
		len(mapSliceFromValue(input["coverageTargetSet"])) > 0 {
		return true
	}
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		return len(mapSliceFromValue(pkg["coverageTargetSet"])) > 0
	}
	return false
}

func coverageBoundTargetTokens(step *runModel.Step) []string {
	if step == nil || step.Input == nil {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0)
	add := func(value string) {
		value = normalizeCoverageGapRepairToken(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	addCoverageTargetMap := func(item map[string]any) {
		add(firstNonEmptyString(
			stringFromMap(item, "coverageTargetToken"),
			stringFromMap(item, "goalRequirementId"),
			stringFromMap(item, "targetText"),
			stringFromMap(item, "item"),
			stringFromMap(item, "raw"),
			stringFromMap(item, "requirement"),
			stringFromMap(item, "goal"),
			stringFromMap(item, "goalItem"),
			stringFromMap(item, "id"),
			stringFromMap(item, "expectationId"),
			stringFromMap(item, "description"),
			stringFromMap(item, "title"),
		))
	}
	for _, item := range mapSliceFromValue(step.Input["missingDoDItems"]) {
		add(stringFromMap(item, "item"))
	}
	for _, item := range missingGoalCoverageFromInput(step.Input) {
		add(firstNonEmptyString(
			stringFromMap(item, "goalRequirementId"),
			stringFromMap(item, "item"),
			missingGoalCoverageRequirementText(item),
		))
		add(missingGoalCoverageRequirementText(item))
	}
	for _, item := range mapSliceFromValue(step.Input["missingDeliverableCoverage"]) {
		add(firstNonEmptyString(
			stringFromMap(item, "item"),
			stringFromMap(item, "id"),
			stringFromMap(item, "expectationId"),
			stringFromMap(item, "requirement"),
		))
	}
	for _, item := range mapSliceFromValue(step.Input["coverageTargetSet"]) {
		addCoverageTargetMap(item)
	}
	if pkg := mapFromValue(step.Input["repairActionPackage"]); len(pkg) > 0 {
		for _, item := range mapSliceFromValue(pkg["coverageTargetSet"]) {
			addCoverageTargetMap(item)
		}
	}
	for _, item := range coverageTargetsFromExecutionContextPackage(mapFromValue(step.Input[stepInputKeyExecutionContextPackage])) {
		addCoverageTargetMap(item)
	}
	return ret
}

func inputExecutionContractRequiresStateChange(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	for _, source := range []map[string]any{
		mapFromValue(input["executionContract"]),
		mapFromValue(mapFromValue(input["executionContext"])["executionContract"]),
		mapFromValue(mapFromValue(input[stepInputKeyExecutionContextPackage])["executionContract"]),
	} {
		if strings.TrimSpace(stringFromMap(source, "stateChangePolicy")) == "must_produce_state_change" {
			return true
		}
	}
	return false
}

func coverageTargetsFromExecutionContextPackage(pkg map[string]any) []map[string]any {
	if len(pkg) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0)
	add := func(kind string, fields map[string]any) {
		if len(fields) == 0 {
			return
		}
		target := firstNonEmptyString(
			stringFromMap(fields, "coverageTargetToken"),
			stringFromMap(fields, "goalRequirementId"),
			stringFromMap(fields, "targetText"),
			stringFromMap(fields, "item"),
			stringFromMap(fields, "raw"),
			stringFromMap(fields, "requirement"),
			stringFromMap(fields, "goal"),
			stringFromMap(fields, "goalItem"),
			stringFromMap(fields, "id"),
			stringFromMap(fields, "expectationId"),
			stringFromMap(fields, "description"),
			stringFromMap(fields, "title"),
		)
		if strings.TrimSpace(target) == "" {
			return
		}
		next := cloneStepPayload(fields)
		next["kind"] = kind
		next["targetText"] = target
		if strings.TrimSpace(stringFromMap(next, "coverageTargetToken")) == "" {
			next["coverageTargetToken"] = target
		}
		ret = append(ret, next)
	}
	addItems := func(kind string, value any) {
		for _, text := range stringSliceFromValue(value) {
			add(kind, map[string]any{"item": text})
		}
		for _, item := range mapSliceFromValue(value) {
			add(kind, item)
		}
	}
	if dod := mapFromValue(pkg["dod"]); len(dod) > 0 {
		addItems("dod", dod["items"])
	}
	for _, dod := range mapSliceFromValue(pkg["dods"]) {
		addItems("dod", dod["items"])
	}
	if goal := mapFromValue(pkg["goal"]); len(goal) > 0 {
		add("goal", goal)
	}
	addItems("goal", pkg["goals"])
	if profileContract := mapFromValue(pkg["profileContract"]); len(profileContract) > 0 {
		addItems("deliverable", profileContract["deliverableExpectations"])
	}
	if contract := mapFromValue(pkg["executionContract"]); len(contract) > 0 {
		addItems("deliverable", contract["deliverableExpectations"])
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func patchNodeDeclaresCoverageTargets(input map[string]any, allowed []string) bool {
	if len(input) == 0 || len(allowed) == 0 {
		return false
	}
	for _, value := range patchNodeCoverageTargetTokens(input) {
		value = normalizeCoverageGapRepairToken(value)
		if value == "" {
			continue
		}
		for _, candidate := range allowed {
			if coverageRepairTokenMatches(value, candidate) {
				return true
			}
		}
	}
	return false
}

func patchNodeCoverageTargetTokens(input map[string]any) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := normalizeCoverageGapRepairToken(value)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		ret = append(ret, value)
	}
	addValue := func(value any) {
		for _, text := range stringSliceFromValue(value) {
			add(text)
		}
		for _, item := range mapSliceFromValue(value) {
			for _, key := range []string{
				"coverageTargetToken",
				"goalRequirementId",
				"sourceRequirementId",
				"targetText",
				"item",
				"raw",
				"requirement",
				"goal",
				"goalItem",
				"id",
				"expectationId",
			} {
				add(stringFromMap(item, key))
			}
		}
	}
	addValue(input["coverageTargets"])
	addValue(input["coverageTargetSet"])
	return ret
}

func normalizeCoverageGapRepairToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, prefix := range []string{"goal:", "dod:", "deliverable:", "acceptance:", "expectation:"} {
		if strings.HasPrefix(value, prefix) {
			value = strings.TrimSpace(strings.TrimPrefix(value, prefix))
			break
		}
	}
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func coverageRepairTokenMatches(value string, candidate string) bool {
	value = normalizeCoverageGapRepairToken(value)
	candidate = normalizeCoverageGapRepairToken(candidate)
	if value == "" || candidate == "" {
		return false
	}
	if value == candidate {
		return true
	}
	if utf8.RuneCountInString(value) < 8 || utf8.RuneCountInString(candidate) < 8 {
		return false
	}
	return strings.Contains(candidate, value) || strings.Contains(value, candidate)
}

func repairPatchTargetRelevanceRequired(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	return stateChangingRepairContractActive(step)
}

func stateChangingRepairContractActive(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	if runtimeStageFromStepInput(step.Input) != runtimeStageRepair {
		return false
	}
	if !boolValue(step.Input["stateChangingOutputRequired"]) {
		return false
	}
	return repairObjectiveRequiresStateChange(stringFromMap(step.Input, "repairObjective"))
}

func repairPatchRelevantTargetRefs(step *runModel.Step) []string {
	if step == nil || step.Input == nil {
		return nil
	}
	return repairPatchRelevantTargetRefsFromInput(step.Input)
}

func repairPatchRelevantTargetRefsFromInput(input map[string]any) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	manifest := repairPatchRelevantWorkspaceManifest(input)
	add := func(value string) {
		value = normalizeRepairPatchTargetRef(value)
		if value == "" {
			return
		}
		if canonical, ok := canonicalRepairTargetRefFromManifest(manifest, value); ok {
			value = canonical
		} else if len(manifest) > 0 {
			// When a current workspace manifest is available, ungrounded
			// diagnostic paths are weak signals and must not lock the repair
			// scope. They can still be carried in diagnostics, but not as hard
			// patch target constraints.
			return
		}
		if seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	addFromItems := func(items []map[string]any) {
		for _, item := range items {
			add(stringFromMap(item, "targetRef"))
		}
	}
	addFromScopeText := func(value string) {
		for _, targetRef := range repairPatchTargetRefsFromScopeText(value) {
			add(targetRef)
		}
	}

	add(stringFromMap(input, "targetRef"))
	addFromItems(mapSliceFromValue(input["failureTargetRepairContext"]))
	addFromItems(mapSliceFromValue(input["patchRepairContext"]))

	actionPackage := mapFromValue(input["repairActionPackage"])
	addFromItems(mapSliceFromValue(actionPackage["targetSnapshots"]))
	for _, key := range []string{"failureTargetScope", failureOutputKeySignature, stepInputKeyRepairOfFailureSig} {
		addFromScopeText(stringFromMap(input, key))
		addFromScopeText(stringFromMap(actionPackage, key))
	}
	for _, nestedKey := range []string{"failedStepInput", "failedStepOutput", "previousStepOutput"} {
		nested := mapFromValue(input[nestedKey])
		add(stringFromMap(nested, "targetRef"))
		for _, key := range []string{"failureTargetScope", failureOutputKeySignature, stepInputKeyRepairOfFailureSig} {
			addFromScopeText(stringFromMap(nested, key))
		}
	}
	return ret
}

func repairPatchRelevantWorkspaceManifest(input map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	if manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest]); len(manifest) > 0 {
		return manifest
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	if manifest := mapSliceFromValue(actionPackage["workspaceFileManifest"]); len(manifest) > 0 {
		return manifest
	}
	if pkg := mapFromValue(input[stepInputKeyExecutionContextPackage]); len(pkg) > 0 {
		if manifest := mapSliceFromValue(pkg["workspaceFileManifest"]); len(manifest) > 0 {
			return manifest
		}
	}
	return nil
}

func repairPatchTargetRefsFromScopeText(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = normalizeRepairPatchTargetRef(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, segment := range strings.Split(value, "|") {
		key, raw, ok := strings.Cut(segment, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "failuretargets", "targetref", "conflictref":
			for _, item := range strings.Split(raw, ",") {
				add(item)
			}
		}
	}
	return ret
}

func repairPatchTargetRefMatches(targetRef string, allowed []string) bool {
	targetRef = normalizeRepairPatchTargetRef(targetRef)
	if targetRef == "" {
		return false
	}
	for _, candidate := range allowed {
		candidate = normalizeRepairPatchTargetRef(candidate)
		if candidate == "" {
			continue
		}
		if targetRef == candidate || strings.HasSuffix(targetRef, "/"+candidate) {
			return true
		}
	}
	return false
}

func normalizeRepairPatchTargetRef(value string) string {
	value = strings.TrimSpace(strings.Trim(value, `"'()[]{}<>`))
	value = strings.ReplaceAll(value, `\`, `/`)
	value = strings.TrimPrefix(value, "./")
	if value == "" {
		return ""
	}
	value = filepath.ToSlash(filepath.Clean(value))
	value = strings.TrimPrefix(value, "./")
	if value == "." {
		return ""
	}
	return value
}

func repairPatchContextItems(step *runModel.Step) []map[string]any {
	if step == nil || step.Input == nil {
		return nil
	}
	ret := mapSliceFromValue(step.Input["patchRepairContext"])
	ret = append(ret, mapSliceFromValue(step.Input["failureTargetRepairContext"])...)
	ret = append(ret, mapSliceFromValue(step.Input["rootCauseCandidateContext"])...)
	actionPackage := mapFromValue(step.Input["repairActionPackage"])
	ret = append(ret, mapSliceFromValue(actionPackage["targetSnapshots"])...)
	ret = append(ret, mapSliceFromValue(actionPackage["rootCauseCandidateContext"])...)
	return ret
}
