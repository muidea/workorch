package readmodelprojection

import (
	"strconv"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func streamDiagnosticsTimelineItems(step projectioncontract.StepView, runState string, stepOccurredAt string) []projectioncontract.RuntimeTimelineItem {
	diagnostics := readModelMapValue(step.Output["streamDiagnostics"])
	if len(diagnostics) == 0 {
		return nil
	}
	summary := "stream diagnostics"
	if receivedBytes := readModelIntValue(diagnostics["receivedBytes"]); receivedBytes > 0 {
		summary = "received " + strconv.Itoa(receivedBytes) + " stream bytes"
	}
	return []projectioncontract.RuntimeTimelineItem{{
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
