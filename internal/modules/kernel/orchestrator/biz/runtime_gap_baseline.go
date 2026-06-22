package orchestrator

import (
	"context"
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	gapBaselineKey    = "gapBaseline"
	gapBaselineSchema = "workorch.gap_baseline.v1"
)

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
	if changed && o.runs != nil {
		return o.persistRunProgressUpdate(ctx, run)
	}
	return nil
}

func gapBaselineFromRun(run *runModel.Run) map[string]any {
	if run == nil {
		return nil
	}
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
		"nextAction":            gapNextActionForRun(run, len(unresolved)),
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
	case "", "schedule_step":
		return false, nil
	case "wait_decision", "wait_approval":
		return true, common.ErrBlocked
	case "no_action":
		return true, nil
	case "fail_fast":
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
		todo := map[string]any{
			"id":                    id,
			"source":                source,
			"sourceRef":             gapTodoSourceRefFromCoverageItem(item),
			"coverageWorkItemIds":   workItemIDs,
			"requiredEvidenceKinds": stringSliceFromValue(item["missingEvidenceKinds"]),
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

func gapNextActionForRun(run *runModel.Run, openItems int) map[string]any {
	if run == nil {
		return nil
	}
	switch run.State {
	case common.RunStateBlockedOnDecision:
		return map[string]any{"kind": "wait_decision", "reason": "run waiting for decision"}
	case common.RunStateBlockedOnApproval:
		return map[string]any{"kind": "wait_approval", "reason": "run waiting for approval"}
	case common.RunStateCompleted:
		return map[string]any{"kind": "no_action", "reason": "run completed"}
	case common.RunStateFailed:
		return map[string]any{"kind": "fail_fast", "reason": "run failed"}
	default:
		if openItems > 0 {
			return map[string]any{"kind": "schedule_step", "reason": "open coverage work items"}
		}
		return map[string]any{"kind": "no_action", "reason": "no open coverage work items"}
	}
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
