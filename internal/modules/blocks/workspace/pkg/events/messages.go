package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/common"
	workspacecontracts "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/contracts"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/eventvalue"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateWorkspaceCommand = workspacecontracts.CreateWorkspaceCommand
type EnsureWorkspaceCommand = workspacecontracts.EnsureWorkspaceCommand
type UpdateWorkspaceCommand struct {
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
	TemplateID           string
	ProfileID            string
	DefaultSoulID        common.SoulID
	SoulBindings         map[string]common.SoulID
	ContextHints         eventvalue.Payload
	EvidencePolicies     []eventvalue.Payload
}
type DeleteWorkspaceCommand = workspacecontracts.DeleteWorkspaceCommand
type PurgeWorkspaceCommand = workspacecontracts.DeleteWorkspaceCommand
type DefineGoalCommand = workspacecontracts.DefineGoalCommand
type UpdateGoalCommand = workspacecontracts.UpdateGoalCommand
type DeleteGoalCommand = workspacecontracts.DeleteGoalCommand
type DefineDoDCommand = workspacecontracts.DefineDoDCommand
type UpdateDoDCommand = workspacecontracts.UpdateDoDCommand
type DeleteDoDCommand = workspacecontracts.DeleteDoDCommand
type CreateWorkspaceTemplateCommand struct {
	TemplateID       string
	Name             string
	Description      string
	GoalText         string
	DoDItems         []string
	DefaultSoulID    string
	SoulBindings     map[string]string
	ContextHints     eventvalue.Payload
	EvidencePolicies []eventvalue.Payload
}

type UpdateWorkspaceTemplateCommand = CreateWorkspaceTemplateCommand
type DeleteWorkspaceTemplateCommand = workspacecontracts.DeleteWorkspaceTemplateCommand

type GetWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type ListWorkspacesCommand struct{}

type GetGoalCommand struct {
	WorkspaceID common.WorkspaceID
	GoalID      common.GoalID
}

type ListGoalsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type GetDoDCommand struct {
	WorkspaceID common.WorkspaceID
	DoDID       common.DoDID
}

type ListDoDsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type GetWorkspaceTemplateCommand struct {
	TemplateID string
}

type ListWorkspaceTemplatesCommand struct{}

type WorkspaceChangedData struct {
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
	RunID                common.RunID
}

type GoalDefinedData struct {
	GoalID                 common.GoalID
	WorkspaceID            common.WorkspaceID
	Text                   string
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousGoalID         common.GoalID
	LineageState           string
}

type DoDDefinedData struct {
	DoDID                  common.DoDID
	WorkspaceID            common.WorkspaceID
	Items                  []string
	State                  common.DoDState
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousDoDID          common.DoDID
	LineageState           string
}

type WorkspaceCommandResult struct {
	Workspace  *workspacemodel.Workspace
	Workspaces []*workspacemodel.Workspace
	Goal       *workspacemodel.Goal
	Goals      []*workspacemodel.Goal
	DoD        *workspacemodel.DoD
	DoDs       []*workspacemodel.DoD
	Template   *repository.WorkspaceTemplate
	Templates  []*repository.WorkspaceTemplate
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[CreateWorkspaceCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandCreate).MessageContract(),
		eventcontract.CommandTopic[EnsureWorkspaceCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandEnsure).MessageContract(),
		eventcontract.CommandTopic[GetWorkspaceCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListWorkspacesCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandList).MessageContract(),
		eventcontract.CommandTopic[UpdateWorkspaceCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandUpdate).MessageContract(),
		eventcontract.CommandTopic[DeleteWorkspaceCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandDelete).MessageContract(),
		eventcontract.CommandTopic[PurgeWorkspaceCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandPurge).MessageContract(),
		eventcontract.CommandTopic[DefineGoalCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandDefineGoal).MessageContract(),
		eventcontract.CommandTopic[UpdateGoalCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandUpdateGoal).MessageContract(),
		eventcontract.CommandTopic[DeleteGoalCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandDeleteGoal).MessageContract(),
		eventcontract.CommandTopic[GetGoalCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandGetGoal).MessageContract(),
		eventcontract.CommandTopic[ListGoalsByWorkspaceCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandListGoalsByWS).MessageContract(),
		eventcontract.CommandTopic[DefineDoDCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandDefineDoD).MessageContract(),
		eventcontract.CommandTopic[UpdateDoDCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandUpdateDoD).MessageContract(),
		eventcontract.CommandTopic[DeleteDoDCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandDeleteDoD).MessageContract(),
		eventcontract.CommandTopic[GetDoDCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandGetDoD).MessageContract(),
		eventcontract.CommandTopic[ListDoDsByWorkspaceCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandListDoDsByWS).MessageContract(),
		eventcontract.CommandTopic[CreateWorkspaceTemplateCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceTemplateCommandCreate).MessageContract(),
		eventcontract.CommandTopic[GetWorkspaceTemplateCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceTemplateCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListWorkspaceTemplatesCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceTemplateCommandList).MessageContract(),
		eventcontract.CommandTopic[UpdateWorkspaceTemplateCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceTemplateCommandUpdate).MessageContract(),
		eventcontract.CommandTopic[DeleteWorkspaceTemplateCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceTemplateCommandDelete).MessageContract(),
	}
	for _, topic := range []string{
		TopicWorkspaceCreated,
		TopicWorkspaceStateChanged,
		TopicWorkspaceDeleted,
	} {
		ret = append(ret, eventcontract.DataTopic[WorkspaceChangedData]("workspace", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	ret = append(ret,
		eventcontract.DataTopic[GoalDefinedData]("workspace", definitionPackage, eventcontract.LanePolicyState, TopicGoalDefined).MessageContract(),
		eventcontract.DataTopic[DoDDefinedData]("workspace", definitionPackage, eventcontract.LanePolicyState, TopicDoDDefined).MessageContract(),
	)
	return ret
}
