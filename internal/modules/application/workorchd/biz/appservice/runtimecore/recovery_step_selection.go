package runtimecore

import (
	"strings"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func LatestRuntimeCheckpointModel(items []*runmodel.RuntimeCheckpoint) *runmodel.RuntimeCheckpoint {
	var latest *runmodel.RuntimeCheckpoint
	for _, item := range items {
		if item == nil {
			continue
		}
		if latest == nil ||
			item.OccurredAt > latest.OccurredAt ||
			(item.OccurredAt == latest.OccurredAt && item.ID > latest.ID) {
			latest = item
		}
	}
	return latest
}

func TerminalStepModel(items []*runmodel.Step) *runmodel.Step {
	if len(items) == 0 {
		return nil
	}
	referenced := map[common.StepID]bool{}
	for _, item := range items {
		if item == nil {
			continue
		}
		if previous := PreviousStepModelID(item); previous != "" {
			referenced[previous] = true
		}
	}
	terminals := make([]*runmodel.Step, 0, len(items))
	for _, item := range items {
		if item == nil || referenced[item.ID] {
			continue
		}
		terminals = append(terminals, item)
	}
	if len(terminals) == 0 {
		return nil
	}
	if len(terminals) == 1 {
		return terminals[0]
	}
	for _, item := range terminals {
		if item.State == common.StepStateDispatched {
			return item
		}
	}
	return terminals[len(terminals)-1]
}

func PreviousStepModelID(item *runmodel.Step) common.StepID {
	if item == nil || item.Input == nil {
		return ""
	}
	for _, key := range []string{"previousStepId", "repairOfStepId", "failedStepId"} {
		value, _ := item.Input[key].(string)
		value = strings.TrimSpace(value)
		if value != "" && common.StepID(value) != item.ID {
			return common.StepID(value)
		}
	}
	return ""
}
