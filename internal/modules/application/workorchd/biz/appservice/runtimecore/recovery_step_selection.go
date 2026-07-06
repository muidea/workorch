package runtimecore

import (
	"strconv"
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
		for _, ref := range referencedStepModelIDs(item) {
			referenced[ref] = true
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
		if item.State == common.StepStateExecuting {
			return item
		}
	}
	if graphTerminal := latestGraphTerminalStep(terminals); graphTerminal != nil {
		return graphTerminal
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

func referencedStepModelIDs(item *runmodel.Step) []common.StepID {
	if item == nil || item.Input == nil {
		return nil
	}
	ret := make([]common.StepID, 0, 2)
	for _, key := range []string{"previousStepId", "repairOfStepId", "failedStepId", "graphRootStepId"} {
		value, _ := item.Input[key].(string)
		value = strings.TrimSpace(value)
		if value == "" || common.StepID(value) == item.ID {
			continue
		}
		ret = append(ret, common.StepID(value))
	}
	return ret
}

func latestGraphTerminalStep(items []*runmodel.Step) *runmodel.Step {
	var latest *runmodel.Step
	var latestGraphSeq, latestNodeSeq int
	for _, item := range items {
		if item == nil || item.Input == nil {
			continue
		}
		if strings.TrimSpace(stringValue(item.Input["graphNodeId"])) == "" ||
			strings.TrimSpace(stringValue(item.Input["graphRootStepId"])) == "" {
			continue
		}
		graphSeq, nodeSeq := graphStepModelSequence(item)
		if latest == nil ||
			graphSeq > latestGraphSeq ||
			(graphSeq == latestGraphSeq && nodeSeq > latestNodeSeq) ||
			(graphSeq == latestGraphSeq && nodeSeq == latestNodeSeq && item.ID > latest.ID) {
			latest = item
			latestGraphSeq = graphSeq
			latestNodeSeq = nodeSeq
		}
	}
	return latest
}

func graphStepModelSequence(item *runmodel.Step) (int, int) {
	if item == nil {
		return 0, 0
	}
	if item.Input != nil {
		graphSeq := intValue(item.Input["graphSeq"])
		nodeSeq := intValue(item.Input["graphNodeSeq"])
		if graphSeq > 0 || nodeSeq > 0 {
			return graphSeq, nodeSeq
		}
	}
	parts := strings.Split(string(item.ID), "/")
	for idx, part := range parts {
		if !strings.HasPrefix(part, "graph-") {
			continue
		}
		graphSeq, _ := strconv.Atoi(strings.TrimPrefix(part, "graph-"))
		nodeSeq := 0
		if idx+1 < len(parts) {
			nodePart, _, _ := strings.Cut(parts[idx+1], "-")
			nodeSeq, _ = strconv.Atoi(nodePart)
		}
		return graphSeq, nodeSeq
	}
	return 0, 0
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}
