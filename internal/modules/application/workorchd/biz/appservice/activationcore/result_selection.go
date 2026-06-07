package activationcore

import (
	"fmt"
	"strings"

	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func LatestTerminalResultForDefinitions(items []*resultmodel.Result, goalID common.GoalID, dodID common.DoDID) *resultmodel.Result {
	var latest *resultmodel.Result
	latestKey := ""
	for idx, item := range items {
		if item == nil || item.GoalID != goalID || item.DoDID != dodID || !IsTerminalResultState(item.State) {
			continue
		}
		key := ResultOrderingKey(item, idx)
		if latest == nil || key > latestKey {
			latest = item
			latestKey = key
		}
	}
	return latest
}

func IsTerminalResultState(state common.ResultState) bool {
	switch state {
	case common.ResultStateAccepted, common.ResultStateRejected:
		return true
	default:
		return false
	}
}

func ResultOrderingKey(item *resultmodel.Result, index int) string {
	if item == nil {
		return fmt.Sprintf("%020d", index)
	}
	timestamp := strings.TrimSpace(item.UpdatedAt)
	if timestamp == "" {
		timestamp = strings.TrimSpace(item.CreatedAt)
	}
	return fmt.Sprintf("%s|%020d", timestamp, index)
}
