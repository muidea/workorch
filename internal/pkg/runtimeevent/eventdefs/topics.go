package eventdefs

import (
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const TopicRuntimeBlockCommandExecutePrefix = "/runtime/block/command/execute/"

const (
	TopicRuntimeBlockExecuteAccepted  = "/runtime/block/execute/accepted"
	TopicRuntimeBlockExecuteProgress  = "/runtime/block/execute/progress"
	TopicRuntimeBlockExecuteCompleted = "/runtime/block/execute/completed"
	TopicRuntimeBlockExecuteFailed    = "/runtime/block/execute/failed"
)

func RuntimeBlockExecuteTopic(kind string) string {
	return TopicRuntimeBlockCommandExecutePrefix + kind
}

func Contracts() []eventcontract.Contract {
	return eventcontract.Build("runtimeevent", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicRuntimeBlockExecuteAccepted,
		TopicRuntimeBlockExecuteProgress,
		TopicRuntimeBlockExecuteCompleted,
		TopicRuntimeBlockExecuteFailed,
	)
}

const definitionPackage = "github.com/muidea/workorch/internal/pkg/runtimeevent/eventdefs"

var (
	RuntimeBlockExecuteAcceptedTopic  = eventcontract.DataTopic[AcceptedData]("runtimeevent", definitionPackage, eventcontract.LanePolicyState, TopicRuntimeBlockExecuteAccepted)
	RuntimeBlockExecuteProgressTopic  = eventcontract.DataTopic[ProgressData]("runtimeevent", definitionPackage, eventcontract.LanePolicyState, TopicRuntimeBlockExecuteProgress)
	RuntimeBlockExecuteCompletedTopic = eventcontract.DataTopic[CompletedData]("runtimeevent", definitionPackage, eventcontract.LanePolicyState, TopicRuntimeBlockExecuteCompleted)
	RuntimeBlockExecuteFailedTopic    = eventcontract.DataTopic[FailedData]("runtimeevent", definitionPackage, eventcontract.LanePolicyState, TopicRuntimeBlockExecuteFailed)
)

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		RuntimeBlockExecuteAcceptedTopic,
		RuntimeBlockExecuteProgressTopic,
		RuntimeBlockExecuteCompletedTopic,
		RuntimeBlockExecuteFailedTopic,
	)
}

type AcceptedData struct {
	Accepted runtimemodel.RuntimeStepExecutionAccepted
}

type ProgressData struct {
	Progress runtimemodel.RuntimeStepExecutionProgressed
}

type CompletedData struct {
	Completed runtimemodel.RuntimeStepExecutionCompleted
	Result    runtimemodel.StepExecutionResult
}

type FailedData struct {
	Failed runtimemodel.RuntimeStepExecutionFailed
	Result runtimemodel.StepExecutionResult
	Error  string
}
