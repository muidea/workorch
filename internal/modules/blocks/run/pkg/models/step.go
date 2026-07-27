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
