package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func llmUsageTraceMergeKey(item projectioncontract.LLMUsageTrace) string {
	return firstNonEmptyTimelineString(item.ID, item.StepID+"|"+item.OccurredAt)
}
