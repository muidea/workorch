package readmodelprojection

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func coverageSummaryFromRunSnapshot(snapshot readmodelcore.RunSnapshotState) map[string]any {
	ctx := snapshot.CoverageWorkItemContext
	ret := map[string]any{}
	for _, key := range []string{"summary", "currentFocus", "omittedCount", "fullLedgerRef"} {
		if value, ok := ctx[key]; ok && value != nil {
			ret[key] = value
		}
	}
	for key, value := range coverageSummaryStatusCounts(snapshot.CoverageWorkItems) {
		ret[key] = value
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func coverageSummaryStatusCounts(items []map[string]any) map[string]int {
	if len(items) == 0 {
		return nil
	}
	ret := map[string]int{}
	for _, item := range items {
		if len(item) == 0 {
			continue
		}
		ret["total"]++
		status := strings.ToLower(strings.TrimSpace(stringValue(item, "status")))
		switch status {
		case "satisfied":
			ret["satisfied"]++
		case "blocked":
			ret["blocked"]++
		case "skipped":
			ret["skipped"]++
		case "superseded":
			ret["superseded"]++
		default:
			if runtimecontract.CoverageWorkItemStatusActive(status) {
				ret["open"]++
			}
		}
	}
	if ret["total"] == 0 {
		return nil
	}
	for _, key := range []string{"open", "satisfied", "blocked"} {
		if _, ok := ret[key]; !ok {
			ret[key] = 0
		}
	}
	return ret
}

func gapBaselineFromRunSnapshot(snapshot readmodelcore.RunSnapshotState) *projectioncontract.GapBaseline {
	return derivedGapBaselineFromRunSnapshot(snapshot)
}

func authorityDiagnosticsFromRunSnapshot(snapshot readmodelcore.RunSnapshotState) []projectioncontract.GapDiagnostic {
	var ret []projectioncontract.GapDiagnostic
	for idx := range snapshot.Steps {
		source := "step.input.authorityDiagnostics"
		if snapshot.Steps[idx].ID != "" {
			source = fmt.Sprintf("step[%s].input.authorityDiagnostics", snapshot.Steps[idx].ID)
		}
		ret = append(ret, authorityDiagnosticsFromRaw(source, snapshot.Steps[idx].Input["authorityDiagnostics"])...)
	}
	ret = append(ret, completionGateDriftDiagnosticsFromRunSnapshot(snapshot)...)
	ret = append(ret, staleValidationResourceDiagnosticsFromRunSnapshot(snapshot)...)
	return ret
}

func completionGateDriftDiagnosticsFromRunSnapshot(snapshot readmodelcore.RunSnapshotState) []projectioncontract.GapDiagnostic {
	if snapshot.Run.State != string(common.RunStateCompleted) {
		return nil
	}
	open := 0
	for _, item := range snapshot.CoverageWorkItems {
		status := strings.ToLower(strings.TrimSpace(stringValue(item, "status")))
		if runtimecontract.CoverageWorkItemStatusActive(status) {
			open++
		}
	}
	if open == 0 {
		return nil
	}
	return []projectioncontract.GapDiagnostic{{
		Code:      "completion_gate_failed_but_run_completed",
		Severity:  "error",
		Kind:      "owner_projection_drift",
		Source:    "run.state+coverageWorkItems",
		Candidate: fmt.Sprintf("completed_with_open_coverage:%d", open),
		Message:   fmt.Sprintf("run is completed while %d coverage work item(s) remain active", open),
	}}
}

func staleValidationResourceDiagnosticsFromRunSnapshot(snapshot readmodelcore.RunSnapshotState) []projectioncontract.GapDiagnostic {
	var ret []projectioncontract.GapDiagnostic
	for _, step := range snapshot.Steps {
		outputResources := mapDynamicSliceValue(step.Output["validationResources"])
		resources := outputResources
		sourceField := "output"
		if len(resources) == 0 && readModelStepStateTerminal(step.State) {
			resources = mapDynamicSliceValue(step.Input["validationResources"])
			sourceField = "input"
		}
		for _, resource := range resources {
			status := strings.ToLower(strings.TrimSpace(readModelStringValue(resource["status"])))
			if !validationResourceStatusIssue(status, readModelStepStateTerminal(step.State)) {
				continue
			}
			kind := strings.TrimSpace(readModelStringValue(resource["kind"]))
			if kind == "" {
				kind = "unknown"
			}
			source := fmt.Sprintf("step.%s.validationResources", sourceField)
			if step.ID != "" {
				source = fmt.Sprintf("step[%s].%s.validationResources", step.ID, sourceField)
			}
			message := fmt.Sprintf("validation resource %s is %s", kind, status)
			if cleanupError := strings.TrimSpace(readModelStringValue(resource["cleanupError"])); cleanupError != "" {
				message = fmt.Sprintf("%s: %s", message, cleanupError)
			}
			ret = append(ret, projectioncontract.GapDiagnostic{
				Code:      "stale_validation_resource_lifecycle",
				Severity:  "warning",
				Kind:      "resource_lifecycle",
				Source:    source,
				Candidate: fmt.Sprintf("%s:%s", kind, status),
				Message:   message,
			})
		}
	}
	return ret
}

func validationResourceStatusIssue(status string, ownerTerminal bool) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "released", "skipped", "removed", "cleanup_skipped", "failed", "timeout":
		return false
	case "pending", "active":
		return ownerTerminal
	default:
		return true
	}
}

func readModelStepStateTerminal(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "completed", "failed", "cancelled", "canceled", "superseded":
		return true
	default:
		return false
	}
}

func authorityDiagnosticsFromRaw(defaultSource string, value any) []projectioncontract.GapDiagnostic {
	items := mapDynamicSliceValue(value)
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.GapDiagnostic, 0, len(items))
	for _, item := range items {
		diagnostic := projectioncontract.GapDiagnostic{
			Code:             stringValue(item, "code"),
			Severity:         stringValue(item, "severity"),
			Kind:             stringValue(item, "kind"),
			Source:           stringValue(item, "source"),
			Candidate:        stringValue(item, "candidate"),
			CurrentAuthority: stringValue(item, "currentAuthority"),
			Message:          stringValue(item, "message"),
		}
		if diagnostic.Source == "" {
			diagnostic.Source = defaultSource
		}
		if diagnostic.Severity == "" {
			diagnostic.Severity = "warning"
		}
		if diagnostic.Code == "" && diagnostic.Message == "" {
			continue
		}
		ret = append(ret, diagnostic)
	}
	return ret
}

func derivedGapBaselineFromRunSnapshot(snapshot readmodelcore.RunSnapshotState) *projectioncontract.GapBaseline {
	ctx := snapshot.CoverageWorkItemContext
	authority := map[string]any{}
	if len(ctx) == 0 {
		ctx = mapDynamicValue(authority["coverageWorkItemContext"])
	}
	if len(ctx) == 0 {
		return nil
	}
	baseline := &projectioncontract.GapBaseline{
		WorkspaceID:        snapshot.Workspace.ID,
		RunID:              snapshot.Run.ID,
		DefinitionRevision: stringValue(ctx, "definitionRevision"),
		BaselineRevision:   stringValue(ctx, "coverageBaselineHash", "baselineRevision"),
		CurrentFocus:       cloneMap(mapDynamicValue(ctx["currentFocus"])),
	}
	if value := stringValue(authority, "definitionRevision"); value != "" {
		baseline.DefinitionRevision = value
	}
	if value := stringValue(authority, "baselineRevision"); value != "" {
		baseline.BaselineRevision = value
	}
	if focus := mapDynamicValue(authority["currentFocus"]); len(focus) > 0 {
		baseline.CurrentFocus = cloneMap(focus)
	}
	unresolved := mapDynamicSliceValue(ctx["unresolvedItems"])
	if len(unresolved) == 0 {
		unresolved = mapDynamicSliceValue(ctx["currentFocusItems"])
	}
	baseline.OpenCoverageWorkItems = readModelMapSliceValue(unresolved)
	baseline.AcceptedEvidenceRefs = gapEvidenceRefsFromCoverageItems(snapshot.CoverageWorkItems, true)
	baseline.RejectedEvidenceRefs = gapEvidenceRefsFromCoverageItems(snapshot.CoverageWorkItems, false)
	if refs := readModelStringSliceValue(authority["acceptedEvidenceRefs"]); len(refs) > 0 {
		baseline.AcceptedEvidenceRefs = refs
	}
	if refs := readModelStringSliceValue(authority["rejectedEvidenceRefs"]); len(refs) > 0 {
		baseline.RejectedEvidenceRefs = refs
	}
	baseline.TodoItems = gapTodoItemsFromCoverageItems(unresolved)
	if baseline.DefinitionRevision == "" && baseline.BaselineRevision == "" && len(baseline.OpenCoverageWorkItems) == 0 && len(baseline.CurrentFocus) == 0 && len(baseline.AcceptedEvidenceRefs) == 0 && len(baseline.RejectedEvidenceRefs) == 0 {
		return nil
	}
	return baseline
}

func readModelFocusAligned(authorityFocus map[string]any, candidate map[string]any) bool {
	authorityIDs := readModelStringSet(readModelFocusWorkItemIDs(authorityFocus))
	candidateIDs := readModelFocusWorkItemIDs(candidate)
	if len(authorityIDs) == 0 || len(candidateIDs) == 0 {
		return strings.TrimSpace(stringValue(authorityFocus, "id")) == strings.TrimSpace(stringValue(candidate, "id"))
	}
	for _, id := range candidateIDs {
		if authorityIDs[id] {
			return true
		}
	}
	return false
}

func readModelFocusWorkItemIDs(focus map[string]any) []string {
	if len(focus) == 0 {
		return nil
	}
	var ids []string
	ids = append(ids, readModelStringSliceValue(focus["coverageWorkItemIds"])...)
	ids = append(ids, readModelStringSliceValue(focus["coverageWorkItemIDs"])...)
	for _, key := range []string{"coverageWorkItemId", "coverageItemId", "workItemId"} {
		if value := strings.TrimSpace(stringValue(focus, key)); value != "" {
			ids = append(ids, value)
		}
	}
	if id := strings.TrimSpace(stringValue(focus, "id")); id != "" {
		ids = append(ids, strings.TrimPrefix(id, "focus-"))
	}
	return compactReadModelStrings(ids)
}

func readModelFocusDebugToken(focus map[string]any) string {
	if ids := readModelFocusWorkItemIDs(focus); len(ids) > 0 {
		return strings.Join(ids, ",")
	}
	return strings.TrimSpace(stringValue(focus, "id"))
}

func compactReadModelStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		ret = append(ret, value)
	}
	return ret
}

func gapBaselineFromMap(input map[string]any) *projectioncontract.GapBaseline {
	if len(input) == 0 {
		return nil
	}
	ret := &projectioncontract.GapBaseline{
		WorkspaceID:           stringValue(input, "workspaceId"),
		RunID:                 stringValue(input, "runId"),
		DefinitionRevision:    stringValue(input, "definitionRevision"),
		BaselineRevision:      stringValue(input, "baselineRevision", "coverageBaselineHash"),
		CurrentFocus:          cloneMap(mapDynamicValue(input["currentFocus"])),
		OpenCoverageWorkItems: readModelMapSliceValue(input["openCoverageWorkItems"]),
		AcceptedEvidenceRefs:  readModelStringSliceValue(input["acceptedEvidenceRefs"]),
		RejectedEvidenceRefs:  readModelStringSliceValue(input["rejectedEvidenceRefs"]),
		TodoItems:             gapTodoItemsFromPersistedBaseline(input["todoItems"]),
	}
	if ret.DefinitionRevision == "" && ret.BaselineRevision == "" && len(ret.OpenCoverageWorkItems) == 0 && len(ret.TodoItems) == 0 && len(ret.CurrentFocus) == 0 && len(ret.AcceptedEvidenceRefs) == 0 && len(ret.RejectedEvidenceRefs) == 0 {
		return nil
	}
	return ret
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
		for _, ref := range readModelStringSliceValue(item[refKey]) {
			add(ref)
		}
		for _, refs := range readModelStringSliceMapValue(item[byKindKey]) {
			for _, ref := range refs {
				add(ref)
			}
		}
	}
	return ret
}

func gapTodoItemsFromPersistedBaseline(value any) []projectioncontract.GapTodoItem {
	items := readModelMapSliceValue(value)
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.GapTodoItem, 0, len(items))
	for _, item := range items {
		if len(item) == 0 {
			continue
		}
		ret = append(ret, projectioncontract.GapTodoItem{
			ID:                    stringValue(item, "id"),
			Source:                stringValue(item, "source"),
			SourceRef:             stringValue(item, "sourceRef"),
			CoverageWorkItemIDs:   readModelStringSliceValue(item["coverageWorkItemIds"]),
			RequiredEvidenceKinds: readModelStringSliceValue(item["requiredEvidenceKinds"]),
			Status:                stringValue(item, "status"),
			Priority:              intValue(item, "priority"),
			Reason:                stringValue(item, "reason"),
			Metadata:              cloneMap(mapDynamicValue(item["metadata"])),
		})
	}
	return ret
}

func intValue(input map[string]any, key string) int {
	if len(input) == 0 {
		return 0
	}
	switch typed := input[key].(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func gapTodoItemsFromCoverageItems(items []map[string]any) []projectioncontract.GapTodoItem {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.GapTodoItem, 0, len(items))
	for idx, item := range items {
		if len(item) == 0 {
			continue
		}
		id := stringValue(item, "id")
		if id == "" {
			id = fmt.Sprintf("todo-%03d", idx+1)
		}
		workItemIDs := readModelStringSliceValue(item["coverageWorkItemIds"])
		if len(workItemIDs) == 0 {
			workItemIDs = readModelStringSliceValue(item["coverageWorkItemIDs"])
		}
		if len(workItemIDs) == 0 && stringValue(item, "id") != "" {
			workItemIDs = []string{stringValue(item, "id")}
		}
		source := stringValue(item, "sourceScope", "source")
		if source == "" {
			source = "coverage"
		}
		status := stringValue(item, "status")
		if status == "" {
			status = "open"
		}
		ret = append(ret, projectioncontract.GapTodoItem{
			ID:                    id,
			Source:                source,
			SourceRef:             gapTodoSourceRef(item),
			CoverageWorkItemIDs:   workItemIDs,
			RequiredEvidenceKinds: readModelStringSliceValue(item["missingEvidenceKinds"]),
			Status:                status,
			Priority:              idx + 1,
			Reason:                gapTodoReason(item),
			Metadata:              gapTodoMetadata(item),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func readModelStringSet(values []string) map[string]bool {
	ret := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			ret[value] = true
		}
	}
	return ret
}

func gapTodoSourceRef(item map[string]any) string {
	return firstNonEmptyProjectionString(
		stringValue(item, "sourceRef"),
		stringValue(item, "sourcePath"),
		stringValue(item, "sourceRequirementId"),
		stringValue(item, "id"),
	)
}

func gapTodoReason(item map[string]any) string {
	return firstNonEmptyProjectionString(
		stringValue(item, "reason"),
		stringValue(item, "targetText"),
		stringValue(item, "summary"),
		"coverage work item remains open",
	)
}

func gapTodoMetadata(item map[string]any) map[string]any {
	ret := map[string]any{}
	for _, key := range []string{"definitionRevision", "evidenceRole", "sourceScope", "targetRef", "targetText"} {
		if value, ok := item[key]; ok && value != nil {
			ret[key] = value
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func firstNonEmptyProjectionString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
