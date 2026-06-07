package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func llmUsageTraceMergeKey(item readmodelcore.LLMUsageTrace) string {
	return firstNonEmptyTimelineString(item.ID, item.StepID+"|"+item.OccurredAt)
}
