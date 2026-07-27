package readmodelprojection

import (
	"strings"
	"time"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
)

func ResultProjectionTime(item *resultmodel.Result) string {
	if item == nil {
		return ""
	}
	if item.UpdatedAt != "" {
		return item.UpdatedAt
	}
	return item.CreatedAt
}

func ResultModelNotBeforeEvent(result *resultmodel.Result, event *eventstreammodel.EventRecord) bool {
	if result == nil || event == nil {
		return result != nil
	}
	resultAt := ResultProjectionTime(result)
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
