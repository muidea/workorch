package orchestrator

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
	return true, o.finalizeRuntimeStepResultWithWorkspace(ctx, run, step, toolCall, workspace, result, nil)
}

func runtimeManagedTargetSnapshotAcquisitionOutput(input map[string]any) (map[string]any, bool) {
	if !inputRequiresRuntimeManagedTargetSnapshotAcquisition(input) {
		return nil, false
	}
	targetRefs := runtimeManagedTargetSnapshotRefs(input)
	if len(targetRefs) == 0 {
		return nil, false
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

func inputRequiresRuntimeManagedTargetSnapshotAcquisition(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	nextAction := strings.TrimSpace(convergenceNextActionKind(nil, input))
	if nextAction != runtimemodel.NextActionTypeAcquireTargetSnapshot {
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
	for _, source := range []map[string]any{
		mapFromValue(input["repairActionPackage"]),
		mapFromValue(input["requiredOutput"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"]),
		mapFromValue(mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"])["requiredOutput"]),
	} {
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
	sort.Strings(ret)
	return ret
}

func runtimeManagedTargetSnapshotRefReadable(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || filepath.IsAbs(ref) || strings.Contains(ref, "\x00") {
		return false
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "cwi-") || strings.HasPrefix(lower, "goal:") ||
		strings.HasPrefix(lower, "dod:") || strings.HasPrefix(lower, "deliverable:") {
		return false
	}
	if strings.Contains(ref, "://") {
		return false
	}
	cleaned := filepath.Clean(ref)
	if cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return false
	}
	return cleaned == ref
}
