package readmodelprojection

import (
	"strings"
	"time"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func resultNotBeforeEvent(result projectioncontract.Result, event projectioncontract.EventRecord) bool {
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

func resultTimeValue(item projectioncontract.Result) string {
	if item.UpdatedAt != "" {
		return item.UpdatedAt
	}
	if item.CreatedAt != "" {
		return item.CreatedAt
	}
	return item.ID
}
