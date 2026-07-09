package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultcontracts "github.com/muidea/workorch/internal/modules/kernel/result/pkg/contracts"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateResultCommand = resultcontracts.CreateResultCommand
type GetResultCommand = resultcontracts.GetResultCommand
type ListResultsByWorkspaceCommand = resultcontracts.ListResultsByWorkspaceCommand
type ListResultsByRunCommand = resultcontracts.ListResultsByRunCommand
type UpdateResultCommand = resultcontracts.UpdateResultCommand
type DeleteResultCommand = resultcontracts.DeleteResultCommand
type CreateArtifactCommand = resultcontracts.CreateArtifactCommand
type GetArtifactCommand = resultcontracts.GetArtifactCommand
type ListArtifactsByWorkspaceCommand = resultcontracts.ListArtifactsByWorkspaceCommand
type ListArtifactsByRunCommand = resultcontracts.ListArtifactsByRunCommand
type ListArtifactsByResultCommand = resultcontracts.ListArtifactsByResultCommand
type UpdateArtifactCommand = resultcontracts.UpdateArtifactCommand
type DeleteArtifactCommand = resultcontracts.DeleteArtifactCommand

type ResultData struct{}

type ResultCommandResult struct {
	Result    *resultmodel.Result
	Results   []*resultmodel.Result
	Artifact  *resultmodel.Artifact
	Artifacts []*resultmodel.Artifact
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{}
	for _, topic := range []string{
		TopicResultCommandCreate,
		TopicResultCommandGet,
		TopicResultCommandListByWorkspace,
		TopicResultCommandListByRun,
		TopicResultCommandUpdate,
		TopicResultCommandDelete,
		TopicArtifactCommandCreate,
		TopicArtifactCommandGet,
		TopicArtifactCommandListByWorkspace,
		TopicArtifactCommandListByRun,
		TopicArtifactCommandListByResult,
		TopicArtifactCommandUpdate,
		TopicArtifactCommandDelete,
	} {
		class := eventcontract.CommandClassWrite
		lane := eventcontract.LanePolicyState
		switch topic {
		case TopicResultCommandGet, TopicResultCommandListByWorkspace, TopicResultCommandListByRun, TopicArtifactCommandGet, TopicArtifactCommandListByWorkspace, TopicArtifactCommandListByRun, TopicArtifactCommandListByResult:
			class = eventcontract.CommandClassRead
			lane = eventcontract.LanePolicyQuery
		}
		switch topic {
		case TopicResultCommandCreate:
			ret = append(ret, eventcontract.CommandTopic[CreateResultCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicResultCommandGet:
			ret = append(ret, eventcontract.CommandTopic[GetResultCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicResultCommandListByWorkspace:
			ret = append(ret, eventcontract.CommandTopic[ListResultsByWorkspaceCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicResultCommandListByRun:
			ret = append(ret, eventcontract.CommandTopic[ListResultsByRunCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicResultCommandUpdate:
			ret = append(ret, eventcontract.CommandTopic[UpdateResultCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicResultCommandDelete:
			ret = append(ret, eventcontract.CommandTopic[DeleteResultCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicArtifactCommandCreate:
			ret = append(ret, eventcontract.CommandTopic[CreateArtifactCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicArtifactCommandGet:
			ret = append(ret, eventcontract.CommandTopic[GetArtifactCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicArtifactCommandListByWorkspace:
			ret = append(ret, eventcontract.CommandTopic[ListArtifactsByWorkspaceCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicArtifactCommandListByRun:
			ret = append(ret, eventcontract.CommandTopic[ListArtifactsByRunCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicArtifactCommandListByResult:
			ret = append(ret, eventcontract.CommandTopic[ListArtifactsByResultCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicArtifactCommandUpdate:
			ret = append(ret, eventcontract.CommandTopic[UpdateArtifactCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		case TopicArtifactCommandDelete:
			ret = append(ret, eventcontract.CommandTopic[DeleteArtifactCommand, ResultCommandResult]("result", definitionPackage, class, lane, topic).MessageContract())
		}
	}
	for _, topic := range []string{
		TopicResultCreated,
		TopicResultAccepted,
		TopicResultRejected,
		TopicResultSuperseded,
		TopicArtifactCreated,
	} {
		ret = append(ret, eventcontract.DataTopic[ResultData]("result", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	return ret
}
