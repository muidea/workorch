package readmodelprojection

import (
	"encoding/json"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func MapAcceptanceProjection(run readmodelcore.Run) readmodelcore.AcceptanceProjection {
	context := run.ExecutionContext
	if len(context) == 0 {
		return emptyAcceptanceProjection()
	}
	source := context
	if nested := mapValue(context, "acceptance"); len(nested) > 0 {
		source = mergeAcceptanceProjectionSource(context, nested)
	}
	ret := readmodelcore.AcceptanceProjection{
		ActiveSliceID: stringValue(source, "activeSliceId", "activeSliceID"),
		Slices:        decodeAcceptanceSlices(valueByKey(source, "slices", "acceptanceSlices")),
		CurrentFocus:  decodeCoverageFocusPtr(valueByKey(source, "currentFocus", "coverageFocus")),
		Budget:        decodeConvergenceBudgetPtr(valueByKey(source, "budget", "convergenceBudget")),
	}
	if ret.ActiveSliceID == "" && ret.CurrentFocus != nil {
		ret.ActiveSliceID = strings.TrimSpace(ret.CurrentFocus.SliceID)
	}
	ret.Summary = summarizeAcceptanceSlices(ret.Slices)
	return ret
}

func MapWorkspaceAcceptanceProjection(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.AcceptanceProjection {
	if snapshot.Run != nil {
		return MapAcceptanceProjection(*snapshot.Run)
	}
	if run := selectWorkspaceAcceptanceRun(snapshot); run != nil {
		return MapAcceptanceProjection(*run)
	}
	return emptyAcceptanceProjection()
}

func selectWorkspaceAcceptanceRun(snapshot readmodelcore.WorkspaceSnapshot) *readmodelcore.Run {
	for _, runID := range []string{
		strings.TrimSpace(snapshot.Workspace.ActiveRunID),
		strings.TrimSpace(snapshot.TaskProgress.RunID),
		strings.TrimSpace(snapshot.Outcome.RunID),
	} {
		if runID == "" {
			continue
		}
		for idx := range snapshot.Runs {
			if snapshot.Runs[idx].ID == runID {
				return &snapshot.Runs[idx]
			}
		}
	}
	if len(snapshot.Runs) == 0 {
		return nil
	}
	return &snapshot.Runs[len(snapshot.Runs)-1]
}

func emptyAcceptanceProjection() readmodelcore.AcceptanceProjection {
	return readmodelcore.AcceptanceProjection{Summary: readmodelcore.AcceptanceSummary{}}
}

func mergeAcceptanceProjectionSource(parent map[string]any, nested map[string]any) map[string]any {
	ret := map[string]any{}
	for key, value := range parent {
		ret[key] = value
	}
	for key, value := range nested {
		ret[key] = value
	}
	return ret
}

func summarizeAcceptanceSlices(items []readmodelcore.AcceptanceSlice) readmodelcore.AcceptanceSummary {
	var ret readmodelcore.AcceptanceSummary
	for _, item := range items {
		switch strings.TrimSpace(item.State) {
		case "accepted":
			ret.AcceptedSlices++
		case "active":
			ret.ActiveSlices++
		case "failed":
			ret.FailedSlices++
		case "", "pending", "blocked":
			ret.OpenSlices++
		default:
			ret.OpenSlices++
		}
	}
	return ret
}

func decodeAcceptanceSlices(value any) []readmodelcore.AcceptanceSlice {
	var ret []readmodelcore.AcceptanceSlice
	if decodeProjectionValue(value, &ret) != nil || len(ret) == 0 {
		return nil
	}
	return ret
}

func decodeCoverageFocusPtr(value any) *readmodelcore.CoverageFocus {
	var ret readmodelcore.CoverageFocus
	if decodeProjectionValue(value, &ret) != nil || isEmptyCoverageFocus(ret) {
		return nil
	}
	return &ret
}

func decodeConvergenceBudgetPtr(value any) *readmodelcore.ConvergenceBudget {
	var ret readmodelcore.ConvergenceBudget
	if decodeProjectionValue(value, &ret) != nil || isEmptyConvergenceBudget(ret) {
		return nil
	}
	return &ret
}

func decodeProjectionValue(value any, target any) error {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if string(data) == "null" || string(data) == "{}" || string(data) == "[]" {
		return nil
	}
	return json.Unmarshal(data, target)
}

func isEmptyCoverageFocus(item readmodelcore.CoverageFocus) bool {
	return strings.TrimSpace(item.ID) == "" &&
		strings.TrimSpace(item.SliceID) == "" &&
		len(item.CoverageWorkItemIDs) == 0 &&
		strings.TrimSpace(item.FailureClass) == "" &&
		strings.TrimSpace(item.FailureSignature) == "" &&
		len(item.RequiredEvidenceKinds) == 0 &&
		len(item.AcceptedEvidenceRefs) == 0 &&
		len(item.RejectedEvidenceRefs) == 0 &&
		len(item.TargetRefs) == 0 &&
		strings.TrimSpace(item.State) == "" &&
		item.Attempt == 0
}

func isEmptyConvergenceBudget(item readmodelcore.ConvergenceBudget) bool {
	return strings.TrimSpace(item.State) == "" &&
		strings.TrimSpace(item.Decision) == "" &&
		item.MaxRepairAttemptsPerFocus == 0 &&
		item.MaxLLMRoundsPerSlice == 0 &&
		item.MaxRepeatedFailureSignature == 0 &&
		item.MaxValidationOnlyRetries == 0 &&
		item.MaxNoCoverageDeltaRounds == 0 &&
		item.MaxContextBytesPerRepair == 0 &&
		item.RepairAttempts == 0 &&
		item.LLMRounds == 0 &&
		item.RepeatedFailureSignatures == 0 &&
		item.ValidationOnlyRetries == 0 &&
		item.NoCoverageDeltaRounds == 0 &&
		strings.TrimSpace(item.LastDecisionReason) == ""
}

func valueByKey(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func stringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if text, ok := value.(string); ok {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func mapValue(values map[string]any, key string) map[string]any {
	value, ok := values[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		var ret map[string]any
		if decodeProjectionValue(value, &ret) != nil {
			return nil
		}
		return ret
	}
}
