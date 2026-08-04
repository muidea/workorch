package readmodelprojection

import (
	"strconv"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func streamDiagnosticsTimelineItems(step readmodelcore.StepView, runState string, stepOccurredAt string) []readmodelcore.RuntimeTimelineItem {
	diagnostics := readModelMapValue(step.Output["streamDiagnostics"])
	if len(diagnostics) == 0 {
		return nil
	}
	summary := "stream diagnostics"
	if receivedBytes := readModelIntValue(diagnostics["receivedBytes"]); receivedBytes > 0 {
		summary = "received " + strconv.Itoa(receivedBytes) + " stream bytes"
	}
	return []readmodelcore.RuntimeTimelineItem{{
		Source:     "step_stream_summary",
		Type:       "summary",
		ID:         step.ID + "/stream/summary",
		RunState:   runState,
		StepID:     step.ID,
		StepState:  step.State,
		Summary:    summary,
		OccurredAt: stepOccurredAt,
		Data:       cloneReadModelMap(diagnostics),
	}}
}
