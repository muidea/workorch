package orchestrator

import (
	"sort"
	"strconv"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func orderedRuntimeContextSteps(steps []*runModel.Step) []*runModel.Step {
	if len(steps) == 0 {
		return nil
	}
	ret := append([]*runModel.Step(nil), steps...)
	sort.SliceStable(ret, func(i, j int) bool {
		return runtimeContextStepLess(ret[i], ret[j])
	})
	return ret
}

func runtimeContextStepLess(left, right *runModel.Step) bool {
	leftSeq, leftHasSeq := runtimeContextExplicitSequence(left)
	rightSeq, rightHasSeq := runtimeContextExplicitSequence(right)
	if leftHasSeq || rightHasSeq {
		if leftHasSeq != rightHasSeq {
			return leftHasSeq
		}
		if leftSeq != rightSeq {
			return leftSeq < rightSeq
		}
	}
	leftOrder := runtimeContextStepIDOrder(left)
	rightOrder := runtimeContextStepIDOrder(right)
	for idx := 0; idx < min(len(leftOrder), len(rightOrder)); idx++ {
		if leftOrder[idx] != rightOrder[idx] {
			return leftOrder[idx] < rightOrder[idx]
		}
	}
	return string(left.ID) < string(right.ID)
}

func runtimeContextExplicitSequence(step *runModel.Step) (int64, bool) {
	if step == nil {
		return 0, false
	}
	for _, source := range []map[string]any{step.Input, step.Output} {
		for _, key := range []string{"sequence", "eventSequence", "stepSequence", "graphSequence", "evidenceOrdinal"} {
			if seq := int64FromValue(source[key]); seq > 0 {
				return seq, true
			}
		}
	}
	return 0, false
}

func runtimeContextStepIDOrder(step *runModel.Step) []int {
	if step == nil {
		return []int{9000}
	}
	raw := strings.TrimSpace(string(step.ID))
	if step.RunID != "" {
		raw = strings.TrimPrefix(raw, strings.TrimSpace(string(step.RunID))+"/")
	}
	raw = strings.Trim(raw, "/")
	switch {
	case strings.HasPrefix(raw, "step-"):
		return []int{10, firstNumberAfterPrefix(raw, "step-")}
	case strings.HasPrefix(raw, "graph-"):
		graph, node := graphStepNumbers(raw)
		return []int{20, graph, node}
	case strings.HasPrefix(raw, "follow-up-graph-"):
		graph, node := graphStepNumbers(strings.TrimPrefix(raw, "follow-up-"))
		return []int{30, graph, node}
	case strings.HasPrefix(raw, "repair-"):
		attempt := firstNumberAfterPrefix(raw, "repair-")
		return []int{40, attempt, trailingNumericSuffix(raw)}
	case strings.HasPrefix(raw, "provider-retry-"):
		return []int{50, firstNumberAfterPrefix(raw, "provider-retry-"), trailingNumericSuffix(raw)}
	default:
		return []int{80, stableStepIDNumericHint(common.StepID(raw))}
	}
}

func graphStepNumbers(raw string) (int, int) {
	graph := firstNumberAfterPrefix(raw, "graph-")
	parts := strings.Split(raw, "/")
	if len(parts) < 2 {
		return graph, 0
	}
	nodePart := parts[1]
	nodeText := ""
	for _, r := range nodePart {
		if r < '0' || r > '9' {
			break
		}
		nodeText += string(r)
	}
	node, _ := strconv.Atoi(nodeText)
	return graph, node
}

func firstNumberAfterPrefix(raw string, prefix string) int {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), prefix)
	text := ""
	for _, r := range raw {
		if r < '0' || r > '9' {
			break
		}
		text += string(r)
	}
	value, _ := strconv.Atoi(text)
	return value
}

func trailingNumericSuffix(raw string) int {
	idx := strings.LastIndex(raw, "-")
	if idx < 0 || idx+1 >= len(raw) {
		return 0
	}
	value, _ := strconv.Atoi(raw[idx+1:])
	return value
}

func stableStepIDNumericHint(stepID common.StepID) int {
	value := 0
	for _, r := range string(stepID) {
		if r >= '0' && r <= '9' {
			value = value*10 + int(r-'0')
			if value > 1000000 {
				return value
			}
		}
	}
	return value
}
