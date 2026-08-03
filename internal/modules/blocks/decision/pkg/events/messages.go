package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateTicketCommand struct {
	TicketID               common.TicketID
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	StepID                 common.StepID
	Reason                 string
	DecisionKind           string
	Question               string
	Context                []DecisionEvidenceField
	AutoGovernance         []DecisionEvidenceField
	FailureClass           string
	FailureSignature       string
	AllowedRecoveryActions []string
}

type EnsureTicketCommand = CreateTicketCommand

type UpdateTicketCommand struct {
	TicketID               common.TicketID
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	StepID                 common.StepID
	Reason                 string
	DecisionKind           string
	Question               string
	Context                []DecisionEvidenceField
	AutoGovernance         []DecisionEvidenceField
	FailureClass           string
	FailureSignature       string
	AllowedRecoveryActions []string
	UpdatedAt              string
}
type DeleteTicketCommand = decisioncontracts.DeleteTicketCommand
type CreateResolutionCommand struct {
	ResolutionID       common.ResolutionID
	TicketID           common.TicketID
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	StepID             common.StepID
	Type               string
	Payload            []DecisionEvidenceField
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
}

type EnsureResolvedCommand struct {
	ResolutionID       common.ResolutionID
	TicketID           common.TicketID
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	StepID             common.StepID
	DecisionKind       string
	Type               string
	Payload            []DecisionEvidenceField
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
}
type DeleteResolutionCommand = decisioncontracts.DeleteResolutionCommand

type OpenRuntimeGovernanceRequestCommand struct {
	TicketID               common.TicketID
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	DispatchID             string
	Transaction            runtimecontract.AuthorityTransaction
	Dependency             runtimecontract.ExternalDependency
	ContinueResolutionType string
	FailResolutionType     string
}

type OpenRuntimeGovernanceRequestResult struct {
	TicketID            common.TicketID
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	DispatchID          string
	AuthoritySnapshotID string
}

type GetTicketCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	TicketID    common.TicketID
}

type ListTicketsCommand struct{}

type ListTicketsByRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type ListTicketsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type GetResolutionCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	TicketID    common.TicketID
}

type TicketChangedData struct {
	TicketID     common.TicketID
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	StepID       common.StepID
	Reason       string
	DecisionKind string
	State        common.DecisionTicketState
}

type TicketResolvedData struct {
	Ticket             TicketChangedData
	ResolutionID       common.ResolutionID
	ResolutionType     string
	ResolutionPayload  []DecisionEvidenceField
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
}

type DecisionCommandResult struct {
	Ticket      *decisionmodel.Ticket
	Tickets     []*decisionmodel.Ticket
	Resolution  *decisionmodel.Resolution
	Resolutions []*decisionmodel.Resolution
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeCapabilities).MessageContract(),
		eventcontract.CommandTopic[CreateTicketCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandCreateTicket).MessageContract(),
		eventcontract.CommandTopic[EnsureTicketCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandEnsureTicket).MessageContract(),
		eventcontract.CommandTopic[GetTicketCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandGetTicket).MessageContract(),
		eventcontract.CommandTopic[ListTicketsCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandListTickets).MessageContract(),
		eventcontract.CommandTopic[ListTicketsByRunCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandListTicketsByRun).MessageContract(),
		eventcontract.CommandTopic[ListTicketsByWorkspaceCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandListTicketsByWS).MessageContract(),
		eventcontract.CommandTopic[UpdateTicketCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandUpdateTicket).MessageContract(),
		eventcontract.CommandTopic[DeleteTicketCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandDeleteTicket).MessageContract(),
		eventcontract.CommandTopic[CreateResolutionCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandCreateResolution).MessageContract(),
		eventcontract.CommandTopic[GetResolutionCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandGetResolution).MessageContract(),
		eventcontract.CommandTopic[DeleteResolutionCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandDeleteResolution).MessageContract(),
		eventcontract.CommandTopic[EnsureResolvedCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandEnsureResolved).MessageContract(),
		eventcontract.CommandTopic[RecordDecisionEvidenceCommand, RecordDecisionEvidenceResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandRecordEvidence).MessageContract(),
		eventcontract.CommandTopic[OpenRuntimeGovernanceRequestCommand, OpenRuntimeGovernanceRequestResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOpenRuntimeGovernanceRequest).MessageContract(),
	}
	for _, topic := range []string{
		TopicTicketCreated,
		TopicTicketSuperseded,
		TopicTicketUpdated,
		TopicDecisionTicketCreated,
		TopicDecisionTicketSuperseded,
	} {
		ret = append(ret, eventcontract.DataTopic[TicketChangedData]("decision", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	ret = append(ret,
		eventcontract.DataTopic[TicketResolvedData]("decision", definitionPackage, eventcontract.LanePolicyState, TopicTicketResolved).MessageContract(),
		eventcontract.DataTopic[TicketResolvedData]("decision", definitionPackage, eventcontract.LanePolicyState, TopicDecisionTicketResolved).MessageContract(),
	)
	return ret
}
