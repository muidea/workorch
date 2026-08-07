package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/common"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateWorkspaceCommand struct {
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
}

// Workspace and WorkspaceTemplate are owner-local EventHub DTOs.
// They preserve the owner result information without exposing repository
// models or map-backed mutable state across the component boundary.
type Workspace struct {
	Revision             uint64
	ID                   common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
	TemplateID           string
	ProfileID            string
	DefaultLLMTarget     *runtimecontract.ProviderModelTarget
	DefaultSoulID        common.SoulID
	SoulBindings         []soulbinding.Binding
	ContextHints         workorcheventpayload.Payload
	EvidencePolicies     []workorcheventpayload.Payload
	TerminalDelivery     *runtimecontract.RuntimeTerminalDeliveryIdentity
}

type WorkspaceTemplate struct {
	ID               string
	Name             string
	Description      string
	GoalText         string
	DoDItems         []string
	DefaultSoulID    string
	SoulBindings     []soulbinding.Binding
	ContextHints     workorcheventpayload.Payload
	EvidencePolicies []workorcheventpayload.Payload
}

type UpdateWorkspaceCommand struct {
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	TemplateID           string
	ProfileID            string
	DefaultLLMTarget     *runtimecontract.ProviderModelTarget
	DefaultSoulID        common.SoulID
	SoulBindings         []soulbinding.Binding
	ContextHints         workorcheventpayload.Payload
	EvidencePolicies     []workorcheventpayload.Payload
}
type UpdateWorkspaceLLMTargetCommand struct {
	WorkspaceID common.WorkspaceID
	Target      *runtimecontract.ProviderModelTarget
}
type DeleteWorkspaceCommand struct{ WorkspaceID common.WorkspaceID }
type DefineGoalCommand struct {
	GoalID      common.GoalID
	WorkspaceID common.WorkspaceID
	Text        string
}
type UpdateGoalCommand struct {
	WorkspaceID common.WorkspaceID
	GoalID      common.GoalID
	Text        string
}
type DeleteGoalCommand struct {
	WorkspaceID common.WorkspaceID
	GoalID      common.GoalID
}
type DefineDoDCommand struct {
	DoDID       common.DoDID
	WorkspaceID common.WorkspaceID
	Items       []string
}
type UpdateDoDCommand struct {
	WorkspaceID common.WorkspaceID
	DoDID       common.DoDID
	Items       []string
}
type DeleteDoDCommand struct {
	WorkspaceID common.WorkspaceID
	DoDID       common.DoDID
}
type CreateWorkspaceTemplateCommand struct {
	TemplateID       string
	Name             string
	Description      string
	GoalText         string
	DoDItems         []string
	DefaultSoulID    string
	SoulBindings     []soulbinding.Binding
	ContextHints     workorcheventpayload.Payload
	EvidencePolicies []workorcheventpayload.Payload
}

type UpdateWorkspaceTemplateCommand CreateWorkspaceTemplateCommand
type DeleteWorkspaceTemplateCommand struct{ TemplateID string }

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
type EnsureWorkspaceTemplateDefaultsCommand struct{}

type EnsureWorkspaceTemplateDefaultsResult struct {
	Ensured bool
}

type WorkspaceChangedData struct {
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
	RunID                common.RunID
	DefaultLLMTarget     *runtimecontract.ProviderModelTarget
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
	Workspace  *Workspace
	Workspaces []*Workspace
	Goal       *workspacemodel.Goal
	Goals      []*workspacemodel.Goal
	DoD        *workspacemodel.DoD
	DoDs       []*workspacemodel.DoD
	Template   *WorkspaceTemplate
	Templates  []*WorkspaceTemplate
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[CreateWorkspaceCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandCreate).MessageContract(),
		eventcontract.CommandTopic[GetWorkspaceCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListWorkspacesCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkspaceCommandList).MessageContract(),
		eventcontract.CommandTopic[UpdateWorkspaceCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandUpdate).MessageContract(),
		eventcontract.CommandTopic[UpdateWorkspaceLLMTargetCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandUpdateLLMTarget).MessageContract(),
		eventcontract.CommandTopic[DeleteWorkspaceCommand, WorkspaceCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandDelete).MessageContract(),
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
		eventcontract.CommandTopic[EnsureWorkspaceTemplateDefaultsCommand, EnsureWorkspaceTemplateDefaultsResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceTemplateCommandEnsureDefaults).MessageContract(),
		eventcontract.CommandTopic[WorkspaceExecutionIntentCommand, WorkspaceExecutionIntentResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceCommandExecutionIntent).MessageContract(),
		eventcontract.CommandTopic[RuntimeTerminalDeliveryFact, WorkspaceLifecycleCommandResult]("workspace", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkspaceFactTerminalDelivery).MessageContract(),
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
