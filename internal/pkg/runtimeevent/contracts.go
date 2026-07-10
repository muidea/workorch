package runtimeevent

import (
	"github.com/muidea/workorch/internal/pkg/runtimeevent/eventdefs"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	CommandDestination      = "/runtime/block"
	NotificationDestination = "/runtime/orchestrator"
)

type ExecuteCommand struct {
	Request   runtimemodel.StepExecutionRequest
	RequestID string
}

type ExecuteCommandResult struct {
	Accepted runtimemodel.RuntimeStepExecutionAccepted
}

type AcceptedData = eventdefs.AcceptedData
type ProgressData = eventdefs.ProgressData
type CompletedData = eventdefs.CompletedData
type FailedData = eventdefs.FailedData
