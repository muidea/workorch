package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeManagedTargetSnapshotRefLimit = 5

func (o *RuntimeOrchestrator) dispatchRuntimeManagedTargetSnapshotAcquisition(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	workspace *workspaceModel.Workspace,
) (bool, error) {
	if o == nil || run == nil || step == nil || workspace == nil {
		return false, nil
	}
	output, ok := runtimeManagedTargetSnapshotAcquisitionOutput(step.Input)
	if !ok {
		return false, nil
	}
	result := runtimemodel.StepExecutionResult{Output: output}
	var execErr error
	if stringFromMap(output, failureOutputKeyClass) == failureClassRuntimeContractMisalign {
		execErr = fmt.Errorf("%w: target snapshot acquisition cannot bind abstract target refs", common.ErrConflict)
	}
	return true, o.finalizeRuntimeStepResultWithWorkspace(ctx, run, step, toolCall, workspace, result, execErr)
}

func runtimeManagedTargetSnapshotAcquisitionOutput(input map[string]any) (map[string]any, bool) {
	if !inputRequiresRuntimeManagedTargetSnapshotAcquisition(input) {
		return nil, false
	}
	targetRefs := runtimeManagedTargetSnapshotRefs(input)
	if len(targetRefs) == 0 {
		allRefs := runtimeManagedTargetSnapshotCandidateRefs(input)
		if len(allRefs) == 0 {
			return nil, false
		}
		return runtimeManagedTargetSnapshotAbstractRefFailureOutput(allRefs), true
	}
	nodes := make([]runtimemodel.BlockExecutionNode, 0, len(targetRefs))
	for idx, targetRef := range targetRefs {
		nodeID := runtimeReadableIDPart("read-target-" + targetRef)
		if nodeID == "" {
			nodeID = "read-target"
		}
		nodes = append(nodes, runtimemodel.BlockExecutionNode{
			ID:   nodeID,
			Kind: "file/read",
			Input: map[string]any{
				"path":                      targetRef,
				"targetRef":                 targetRef,
				"runtimeManagedAcquisition": true,
				"acquisitionKind":           "target_snapshot",
				"optional":                  true,
			},
		})
		if idx > 0 {
			nodes[idx].DependsOn = []string{nodes[idx-1].ID}
		}
	}
	return map[string]any{
		"status": "completed",
		"stage":  "runtime_managed_target_snapshot_acquisition",
		"runtimeManagedTargetSnapshotAcquisition": true,
		stepOutputKeyExecutionGraph:               runtimemodel.BlockExecutionGraph{Nodes: nodes},
	}, true
}

func runtimeManagedTargetSnapshotAbstractRefFailureOutput(refs []string) map[string]any {
	refs = compactUniqueStrings(refs)
	sort.Strings(refs)
	return map[string]any{
		"status":                  "failed",
		"stage":                   "runtime_managed_target_snapshot_acquisition",
		"error":                   "target snapshot acquisition cannot be satisfied from abstract target refs without a readable runtime resource",
		failureOutputKeyClass:     failureClassRuntimeContractMisalign,
		failureOutputKeySignature: failureClassRuntimeContractMisalign + ":target_snapshot_abstract_refs:" + strings.Join(refs, ","),
		"runtimeManagedTargetSnapshotAcquisition": true,
		"abstractTargetRefs":                      refs,
		"deterministicOutcome":                    "fail_fast_internal_contract_gap",
	}
}

func inputRequiresRuntimeManagedTargetSnapshotAcquisition(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	nextAction := strings.TrimSpace(convergenceNextActionKind(nil, input))
	if nextAction != runtimemodel.NextActionTypeAcquireTargetSnapshot && !targetSnapshotPrerequisiteActive(input) {
		return false
	}
	required := activeRequiredOutputForCurrentAction(input)
	return boolValue(required["mustAcquireCurrentTargetSnapshot"]) &&
		boolValue(required["mustUseReadOnlySnapshotAcquisition"]) &&
		!boolValue(required["mustIncludeStateChangingNode"])
}

func runtimeManagedTargetSnapshotRefs(input map[string]any) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if !runtimeManagedTargetSnapshotRefReadable(value) || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, value := range runtimeManagedTargetSnapshotCandidateRefs(input) {
		add(value)
	}
	sort.Strings(ret)
	if len(ret) > runtimeManagedTargetSnapshotRefLimit {
		ret = ret[:runtimeManagedTargetSnapshotRefLimit]
	}
	return ret
}

func runtimeManagedTargetSnapshotCandidateRefs(input map[string]any) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	addFromSource := func(source map[string]any) {
		if len(source) == 0 {
			return
		}
		for _, key := range []string{"repairTargetSet", "targetRefs", "failureTargets", "relatedFailureTargets"} {
			for _, value := range stringSliceFromValue(source[key]) {
				add(value)
			}
			for _, item := range mapSliceFromValue(source[key]) {
				for _, itemKey := range []string{"targetRef", "path", "resourcePath", "filePath"} {
					add(stringFromMap(item, itemKey))
				}
			}
		}
		for _, item := range mapSliceFromValue(source["structuredTargetRefs"]) {
			for _, itemKey := range []string{"targetRef", "path", "resourcePath", "filePath"} {
				add(stringFromMap(item, itemKey))
			}
		}
		for _, key := range []string{"targetRef", "path", "resourcePath", "filePath"} {
			add(stringFromMap(source, key))
		}
	}

	focus := runtimeManagedTargetSnapshotCurrentFocus(input)
	focused := runtimeManagedTargetSnapshotCandidateRefsFromCurrentScope(input, focus)
	if len(focus) > 0 {
		sort.Strings(focused)
		return focused
	}

	for _, source := range []map[string]any{
		mapFromValue(input["repairActionPackage"]),
		mapFromValue(input["requiredOutput"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"]),
		mapFromValue(mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"])["requiredOutput"]),
		dispatchAuthorityCurrentFocus(input),
		mapFromValue(input[coverageFocusKey]),
		mapFromValue(mapFromValue(input["executionContext"])[coverageFocusKey]),
		mapFromValue(mapFromValue(input[convergenceAuthorityKey])["currentFocus"]),
		mapFromValue(mapFromValue(input[gapBaselineKey])["currentFocus"]),
		mapFromValue(mapFromValue(input["acceptance"])["currentFocus"]),
		mapFromValue(mapFromValue(input[currentActionContractKey])["currentFocus"]),
		mapFromValue(mapFromValue(input[currentActionContractKey])["requiredOutput"]),
		mapFromValue(mapFromValue(input[coverageWorkItemContextKey])["currentFocus"]),
		mapFromValue(mapFromValue(mapFromValue(input["repairActionPackage"])[coverageWorkItemContextKey])["currentFocus"]),
	} {
		addFromSource(source)
	}
	for _, source := range []map[string]any{
		mapFromValue(input[coverageWorkItemContextKey]),
		mapFromValue(input[convergenceAuthorityKey]),
		mapFromValue(input[gapBaselineKey]),
		mapFromValue(input["acceptance"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])[coverageWorkItemContextKey]),
	} {
		for _, key := range []string{"currentFocusItems", "ledgerSummary", "unresolvedItems", "coverageTargetSet", "repairTargetSet"} {
			for _, item := range mapSliceFromValue(source[key]) {
				addFromSource(item)
			}
		}
	}
	sort.Strings(ret)
	return ret
}

func runtimeManagedTargetSnapshotCurrentFocus(input map[string]any) map[string]any {
	focus := dispatchAuthorityCurrentFocus(input)
	if len(focus) == 0 {
		focus = mapFromValue(mapFromValue(input[currentActionContractKey])["currentFocus"])
	}
	if len(focus) == 0 {
		focus = mapFromValue(mapFromValue(input[gapBaselineKey])["currentFocus"])
	}
	if len(focus) == 0 {
		focus = mapFromValue(input[coverageFocusKey])
	}
	return focus
}

func runtimeManagedTargetSnapshotCandidateRefsFromCurrentScope(input map[string]any, focus map[string]any) []string {
	if len(input) == 0 || len(focus) == 0 {
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
	addFromSource := func(source map[string]any) {
		if len(source) == 0 {
			return
		}
		for _, key := range []string{"repairTargetSet", "targetRefs", "failureTargets", "relatedFailureTargets"} {
			for _, value := range stringSliceFromValue(source[key]) {
				add(value)
			}
			for _, item := range mapSliceFromValue(source[key]) {
				for _, itemKey := range []string{"targetRef", "path", "resourcePath", "filePath"} {
					add(stringFromMap(item, itemKey))
				}
			}
		}
		for _, item := range mapSliceFromValue(source["structuredTargetRefs"]) {
			for _, itemKey := range []string{"targetRef", "path", "resourcePath", "filePath"} {
				add(stringFromMap(item, itemKey))
			}
		}
		for _, key := range []string{"targetRef", "path", "resourcePath", "filePath"} {
			add(stringFromMap(source, key))
		}
	}
	for _, source := range runtimeManagedTargetSnapshotFocusedSources(input, focus) {
		addFromSource(source)
	}
	sort.Strings(ret)
	return ret
}

func runtimeManagedTargetSnapshotFocusedSources(input map[string]any, focus map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	sources := []map[string]any{
		mapFromValue(mapFromValue(input[currentActionContractKey])["currentFocus"]),
		dispatchAuthorityCurrentFocus(input),
		mapFromValue(mapFromValue(input[convergenceAuthorityKey])["currentFocus"]),
		mapFromValue(mapFromValue(input[gapBaselineKey])["currentFocus"]),
		mapFromValue(input[coverageFocusKey]),
		mapFromValue(mapFromValue(input[coverageWorkItemContextKey])["currentFocus"]),
		mapFromValue(mapFromValue(mapFromValue(input["repairActionPackage"])[coverageWorkItemContextKey])["currentFocus"]),
		activeRequiredOutputForCurrentAction(input),
		mapFromValue(mapFromValue(input[currentActionContractKey])["requiredOutput"]),
		mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		mapFromValue(input["requiredOutput"]),
	}
	for _, source := range []map[string]any{
		mapFromValue(input[coverageWorkItemContextKey]),
		mapFromValue(input[convergenceAuthorityKey]),
		mapFromValue(input[gapBaselineKey]),
		mapFromValue(mapFromValue(input["repairActionPackage"])[coverageWorkItemContextKey]),
	} {
		for _, key := range []string{"currentFocusItems", "ledgerSummary", "unresolvedItems", "coverageTargetSet", "repairTargetSet"} {
			for _, item := range mapSliceFromValue(source[key]) {
				if len(focus) == 0 || coverageContextItemMatchesFocus(item, focus) {
					sources = append(sources, item)
				}
			}
		}
	}
	return sources
}

func runtimeManagedTargetSnapshotRefReadable(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || filepath.IsAbs(ref) || strings.Contains(ref, "\x00") {
		return false
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "cwi-") || strings.HasPrefix(lower, "goal:") ||
		strings.HasPrefix(lower, "dod:") || strings.HasPrefix(lower, "deliverable:") ||
		strings.HasPrefix(lower, "goal-") || strings.HasPrefix(lower, "dod-") ||
		strings.HasPrefix(lower, "goal.") || strings.HasPrefix(lower, "dod.") ||
		runtimeManagedTargetSnapshotRefLooksLogicalToken(ref) {
		return false
	}
	if strings.ContainsAny(ref, "[]") {
		return false
	}
	if strings.Contains(ref, ":") {
		return false
	}
	cleaned := filepath.Clean(ref)
	if cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return false
	}
	return cleaned == ref
}

func runtimeManagedTargetSnapshotRefLooksLogicalToken(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.ContainsAny(ref, `/\.`) {
		return false
	}
	if strings.Contains(ref, "_") {
		return true
	}
	return strings.ToLower(ref) == ref
}
