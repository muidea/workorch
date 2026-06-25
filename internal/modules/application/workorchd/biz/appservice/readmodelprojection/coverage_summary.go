package readmodelprojection

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func CoverageSummaryFromRunSnapshot(snapshot readmodelcore.RunSnapshot) map[string]any {
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

func GapBaselineFromRunSnapshot(snapshot readmodelcore.RunSnapshot) *readmodelcore.GapBaseline {
	derived := derivedGapBaselineFromRunSnapshot(snapshot)
	if persisted := gapBaselineFromMap(MapValue(snapshot.Run.ExecutionContext["gapBaseline"])); persisted != nil {
		if persisted.WorkspaceID == "" {
			persisted.WorkspaceID = snapshot.Workspace.ID
		}
		if persisted.RunID == "" {
			persisted.RunID = snapshot.Run.ID
		}
		normalizePersistedGapBaselineProjection(persisted, derived)
		return persisted
	}
	return derived
}

func derivedGapBaselineFromRunSnapshot(snapshot readmodelcore.RunSnapshot) *readmodelcore.GapBaseline {
	ctx := snapshot.CoverageWorkItemContext
	if len(ctx) == 0 {
		return nil
	}
	baseline := &readmodelcore.GapBaseline{
		WorkspaceID:        snapshot.Workspace.ID,
		RunID:              snapshot.Run.ID,
		DefinitionRevision: stringValue(ctx, "definitionRevision"),
		BaselineRevision:   stringValue(ctx, "coverageBaselineHash", "baselineRevision"),
		CurrentFocus:       CloneMap(MapValue(ctx["currentFocus"])),
	}
	unresolved := MapSliceValue(ctx["unresolvedItems"])
	if len(unresolved) == 0 {
		unresolved = MapSliceValue(ctx["currentFocusItems"])
	}
	baseline.OpenCoverageWorkItems = readModelMapSliceValue(unresolved)
	baseline.AcceptedEvidenceRefs = gapEvidenceRefsFromCoverageItems(snapshot.CoverageWorkItems, true)
	baseline.RejectedEvidenceRefs = gapEvidenceRefsFromCoverageItems(snapshot.CoverageWorkItems, false)
	baseline.TodoItems = gapTodoItemsFromCoverageItems(unresolved)
	baseline.NextAction = gapNextActionFromSnapshot(snapshot, len(unresolved))
	if baseline.DefinitionRevision == "" && baseline.BaselineRevision == "" && len(baseline.OpenCoverageWorkItems) == 0 && len(baseline.CurrentFocus) == 0 && len(baseline.AcceptedEvidenceRefs) == 0 && len(baseline.RejectedEvidenceRefs) == 0 {
		return nil
	}
	return baseline
}

func normalizePersistedGapBaselineProjection(persisted *readmodelcore.GapBaseline, derived *readmodelcore.GapBaseline) {
	if persisted == nil || derived == nil {
		return
	}
	if !gapNextActionNeedsProjectionNormalization(persisted.NextAction, derived.NextAction) {
		return
	}
	previous := persisted.NextAction
	persisted.NextAction = derived.NextAction
	persisted.Diagnostics = append(persisted.Diagnostics, readmodelcore.GapDiagnostic{
		Code:     "stale_persisted_next_action",
		Severity: "warning",
		Message:  gapNextActionProjectionConflictMessage(previous, derived.NextAction),
	})
}

func gapNextActionNeedsProjectionNormalization(persisted, derived readmodelcore.GapNextAction) bool {
	persistedKind := strings.TrimSpace(persisted.Kind)
	derivedKind := strings.TrimSpace(derived.Kind)
	if derivedKind == "" || persistedKind == "" {
		return false
	}
	if persistedKind == "schedule_step" {
		return true
	}
	if gapNextActionGovernedOrTerminal(persistedKind) || gapNextActionGovernedOrTerminal(derivedKind) {
		return false
	}
	if gapNextActionFocusSensitive(persistedKind) && gapNextActionFocusSensitive(derivedKind) && persistedKind != derivedKind {
		return true
	}
	return false
}

func gapNextActionGovernedOrTerminal(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "wait_decision", "wait_approval", "request_decision", "request_approval", "no_action", "complete", "fail_fast", "fail":
		return true
	default:
		return false
	}
}

func gapNextActionFocusSensitive(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "acquire_evidence", "produce_state_change", "produce_validation_evidence", "produce_artifact_evidence", "complete_coverage_assessment":
		return true
	default:
		return false
	}
}

func gapNextActionProjectionConflictMessage(previous, derived readmodelcore.GapNextAction) string {
	previousKind := strings.TrimSpace(previous.Kind)
	if previousKind == "" {
		previousKind = "empty"
	}
	derivedKind := strings.TrimSpace(derived.Kind)
	if derivedKind == "" {
		derivedKind = "empty"
	}
	return fmt.Sprintf("persisted gapBaseline nextAction %q was normalized to %q from current projection focus", previousKind, derivedKind)
}

func gapBaselineFromMap(input map[string]any) *readmodelcore.GapBaseline {
	if len(input) == 0 {
		return nil
	}
	ret := &readmodelcore.GapBaseline{
		WorkspaceID:           stringValue(input, "workspaceId"),
		RunID:                 stringValue(input, "runId"),
		DefinitionRevision:    stringValue(input, "definitionRevision"),
		BaselineRevision:      stringValue(input, "baselineRevision", "coverageBaselineHash"),
		CurrentFocus:          CloneMap(MapValue(input["currentFocus"])),
		OpenCoverageWorkItems: readModelMapSliceValue(input["openCoverageWorkItems"]),
		AcceptedEvidenceRefs:  readModelStringSliceValue(input["acceptedEvidenceRefs"]),
		RejectedEvidenceRefs:  readModelStringSliceValue(input["rejectedEvidenceRefs"]),
		TodoItems:             gapTodoItemsFromPersistedBaseline(input["todoItems"]),
		NextAction:            gapNextActionFromMap(MapValue(input["nextAction"])),
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

func gapTodoItemsFromPersistedBaseline(value any) []readmodelcore.GapTodoItem {
	items := readModelMapSliceValue(value)
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.GapTodoItem, 0, len(items))
	for _, item := range items {
		if len(item) == 0 {
			continue
		}
		ret = append(ret, readmodelcore.GapTodoItem{
			ID:                    stringValue(item, "id"),
			Source:                stringValue(item, "source"),
			SourceRef:             stringValue(item, "sourceRef"),
			CoverageWorkItemIDs:   readModelStringSliceValue(item["coverageWorkItemIds"]),
			RequiredEvidenceKinds: readModelStringSliceValue(item["requiredEvidenceKinds"]),
			Status:                stringValue(item, "status"),
			Priority:              intValue(item, "priority"),
			Reason:                stringValue(item, "reason"),
			Metadata:              CloneMap(MapValue(item["metadata"])),
		})
	}
	return ret
}

func gapNextActionFromMap(input map[string]any) readmodelcore.GapNextAction {
	if len(input) == 0 {
		return readmodelcore.GapNextAction{}
	}
	return readmodelcore.GapNextAction{
		Kind:   stringValue(input, "kind"),
		Reason: stringValue(input, "reason"),
	}
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

func gapTodoItemsFromCoverageItems(items []map[string]any) []readmodelcore.GapTodoItem {
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.GapTodoItem, 0, len(items))
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
		ret = append(ret, readmodelcore.GapTodoItem{
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

func gapNextActionFromSnapshot(snapshot readmodelcore.RunSnapshot, openItems int) readmodelcore.GapNextAction {
	switch strings.TrimSpace(snapshot.Run.State) {
	case "blocked_on_decision":
		return readmodelcore.GapNextAction{Kind: "wait_decision", Reason: "run waiting for decision"}
	case "blocked_on_approval":
		return readmodelcore.GapNextAction{Kind: "wait_approval", Reason: "run waiting for approval"}
	case "completed":
		return readmodelcore.GapNextAction{Kind: "no_action", Reason: "run completed"}
	case "failed":
		return readmodelcore.GapNextAction{Kind: "fail_fast", Reason: "run failed"}
	default:
		if openItems > 0 {
			return gapNextActionForOpenCoverage(snapshot)
		}
		return readmodelcore.GapNextAction{Kind: "no_action", Reason: "no open coverage work items"}
	}
}

func gapNextActionForOpenCoverage(snapshot readmodelcore.RunSnapshot) readmodelcore.GapNextAction {
	item := gapNextActionFocusItem(snapshot)
	if len(item) == 0 {
		return readmodelcore.GapNextAction{Kind: "acquire_evidence", Reason: "open coverage work items"}
	}
	status := strings.ToLower(strings.TrimSpace(stringValue(item, "status")))
	if status == "blocked" {
		return readmodelcore.GapNextAction{Kind: "request_governance", Reason: "coverage work item is blocked"}
	}
	missing := readModelStringSet(readModelStringSliceValue(item["missingEvidenceKinds"]))
	switch {
	case missing["decision"] || missing["approval"] || missing["external_dependency"]:
		return readmodelcore.GapNextAction{Kind: "request_governance", Reason: "coverage work item requires governance evidence"}
	case missing["state_change"]:
		return readmodelcore.GapNextAction{Kind: "produce_state_change", Reason: "coverage work item is missing state_change evidence"}
	case missing["validation"]:
		return readmodelcore.GapNextAction{Kind: "produce_validation_evidence", Reason: "coverage work item is missing validation evidence"}
	case missing["artifact"]:
		return readmodelcore.GapNextAction{Kind: "produce_artifact_evidence", Reason: "coverage work item is missing artifact evidence"}
	case missing["assessment"]:
		return readmodelcore.GapNextAction{Kind: "complete_coverage_assessment", Reason: "coverage work item is missing assessment evidence"}
	case len(missing) > 0:
		return readmodelcore.GapNextAction{Kind: "acquire_evidence", Reason: "coverage work item is missing evidence"}
	case status == "evidence_ready":
		return readmodelcore.GapNextAction{Kind: "complete_coverage_assessment", Reason: "coverage work item evidence is ready for assessment"}
	default:
		return readmodelcore.GapNextAction{Kind: "complete_coverage_assessment", Reason: "open coverage work item requires assessment"}
	}
}

func gapNextActionFocusItem(snapshot readmodelcore.RunSnapshot) map[string]any {
	ctx := snapshot.CoverageWorkItemContext
	for _, key := range []string{"currentFocus"} {
		if item := MapValue(ctx[key]); len(item) > 0 {
			return item
		}
	}
	for _, key := range []string{"unresolvedItems", "currentFocusItems"} {
		items := MapSliceValue(ctx[key])
		if len(items) > 0 {
			return items[0]
		}
	}
	if len(snapshot.CoverageWorkItems) > 0 {
		return snapshot.CoverageWorkItems[0]
	}
	return nil
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
