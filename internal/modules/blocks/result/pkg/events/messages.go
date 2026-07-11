package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultcontracts "github.com/muidea/workorch/internal/modules/blocks/result/pkg/contracts"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
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

type ResultChangedData struct {
	ResultID     common.ResultID
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	GoalID       common.GoalID
	DoDID        common.DoDID
	State        common.ResultState
	Title        string
	Summary      string
	SourceType   string
	SourceRef    string
	EvidenceRefs []string
	ArtifactRefs []common.ArtifactID
}

type ArtifactCreatedData struct {
	ArtifactID         common.ArtifactID
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	ResultID           common.ResultID
	StepID             common.StepID
	ToolCallID         common.ToolCallID
	PatchID            common.PatchID
	ValidationResultID common.ValidationResultID
	ArtifactType       string
	Name               string
	Summary            string
	URI                string
	ContentType        string
	State              common.ArtifactState
	EvidenceRefs       []string
}

func ResultChangedDataFromModel(item *resultmodel.Result) ResultChangedData {
	if item == nil {
		return ResultChangedData{}
	}
	return ResultChangedData{
		ResultID:     item.ID,
		WorkspaceID:  item.WorkspaceID,
		RunID:        item.RunID,
		GoalID:       item.GoalID,
		DoDID:        item.DoDID,
		State:        item.State,
		Title:        item.Title,
		Summary:      item.Summary,
		SourceType:   item.SourceType,
		SourceRef:    item.SourceRef,
		EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
		ArtifactRefs: append([]common.ArtifactID(nil), item.ArtifactRefs...),
	}
}

func ArtifactCreatedDataFromModel(item *resultmodel.Artifact) ArtifactCreatedData {
	if item == nil {
		return ArtifactCreatedData{}
	}
	return ArtifactCreatedData{
		ArtifactID:         item.ID,
		WorkspaceID:        item.WorkspaceID,
		RunID:              item.RunID,
		ResultID:           item.ResultID,
		StepID:             item.StepID,
		ToolCallID:         item.ToolCallID,
		PatchID:            item.PatchID,
		ValidationResultID: item.ValidationResultID,
		ArtifactType:       item.ArtifactType,
		Name:               item.Name,
		Summary:            item.Summary,
		URI:                item.URI,
		ContentType:        item.ContentType,
		State:              item.State,
		EvidenceRefs:       append([]string(nil), item.EvidenceRefs...),
	}
}

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
	} {
		ret = append(ret, eventcontract.DataTopic[ResultChangedData]("result", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	ret = append(ret, eventcontract.DataTopic[ArtifactCreatedData]("result", definitionPackage, eventcontract.LanePolicyState, TopicArtifactCreated).MessageContract())
	return ret
}
