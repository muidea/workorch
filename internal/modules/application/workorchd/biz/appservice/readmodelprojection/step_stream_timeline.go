package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func streamTimelineItems(step projectioncontract.StepView, runState string, stepOccurredAt string) []projectioncontract.RuntimeTimelineItem {
	if len(step.Output) == 0 {
		return nil
	}
	raw, ok := step.Output["streamEvents"]
	if !ok {
		return streamDiagnosticsTimelineItems(step, runState, stepOccurredAt)
	}
	events := readModelMapSliceValue(raw)
	if len(events) == 0 {
		return streamDiagnosticsTimelineItems(step, runState, stepOccurredAt)
	}
	ret := make([]projectioncontract.RuntimeTimelineItem, 0, streamTimelineItemCapacity(len(events)))
	omittedInserted := false
	for idx, event := range events {
		if streamTimelineEventOmitted(idx, len(events)) {
			if !omittedInserted {
				ret = append(ret, streamTimelineOmittedItem(step, runState, len(events)))
				omittedInserted = true
			}
			continue
		}
		ret = append(ret, streamTimelineEventItem(step, runState, stepOccurredAt, idx, event))
	}
	return ret
}
