package readmodelprojection

import (
	"strconv"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TimelineStepSummary(output map[string]any) string {
	if len(output) == 0 {
		return ""
	}
	if stage := readModelStringValue(output["stage"]); stage != "" {
		return stage
	}
	if status := readModelStringValue(output["status"]); status != "" {
		return status
	}
	return ""
}

func memoryRecallTimelineSummary(trace projectioncontract.MemoryRecallTrace) string {
	count := len(trace.RetrievedMemoryIDs)
	if trace.Status == "degraded" && trace.Reason != "" {
		return "memory recall degraded: " + trace.Reason
	}
	if count == 0 {
		return "memory recall " + trace.Status
	}
	return "memory recall " + trace.Status + ": " + strconv.Itoa(count) + " memories"
}
