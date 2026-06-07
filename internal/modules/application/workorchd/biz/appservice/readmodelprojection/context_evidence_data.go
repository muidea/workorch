package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func compactContextEvidenceData(item readmodelcore.RuntimeTimelineItem) map[string]any {
	data := map[string]any{}
	if item.Topic != "" {
		data["topic"] = item.Topic
	}
	if item.RunState != "" {
		data["runState"] = item.RunState
	}
	if item.StepState != "" {
		data["stepState"] = item.StepState
	}
	if item.Summary != "" {
		data["summary"] = item.Summary
	}
	if len(data) == 0 {
		return nil
	}
	return data
}
