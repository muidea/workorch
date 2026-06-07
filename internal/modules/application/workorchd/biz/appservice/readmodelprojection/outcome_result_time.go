package readmodelprojection

import (
	"strings"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func resultNotBeforeEvent(result readmodelcore.Result, event readmodelcore.EventRecord) bool {
	resultAt := resultTimeValue(result)
	eventAt := strings.TrimSpace(event.OccurredAt)
	if resultAt == "" || eventAt == "" {
		return resultAt >= eventAt
	}
	resultTime, resultErr := time.Parse(time.RFC3339Nano, resultAt)
	eventTime, eventErr := time.Parse(time.RFC3339Nano, eventAt)
	if resultErr == nil && eventErr == nil {
		return !resultTime.Before(eventTime)
	}
	return resultAt >= eventAt
}

func resultTimeValue(item readmodelcore.Result) string {
	if item.UpdatedAt != "" {
		return item.UpdatedAt
	}
	if item.CreatedAt != "" {
		return item.CreatedAt
	}
	return item.ID
}
