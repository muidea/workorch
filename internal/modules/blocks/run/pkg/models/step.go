package run

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type Step struct {
	ID     common.StepID
	RunID  common.RunID
	Kind   string
	State  common.StepState
	Input  workorcheventpayload.Payload
	Output workorcheventpayload.Payload
}

func CloneStep(item *Step) *Step {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.Input = item.Input.Clone()
	cloned.Output = item.Output.Clone()
	return &cloned
}

func CloneSteps(items []*Step) []*Step {
	if items == nil {
		return nil
	}
	ret := make([]*Step, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneStep(item))
	}
	return ret
}
