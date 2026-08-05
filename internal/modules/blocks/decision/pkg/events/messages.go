package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
const CommandDestination = modulecommon.ModuleID

type DeleteTicketCommand struct {
	TicketID    common.TicketID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
}

type EnsureResolvedCommand struct {
	ResolutionID       common.ResolutionID
	TicketID           common.TicketID
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	StepID             common.StepID
	DecisionKind       string
	Type               string
	Payload            []workorcheventpayload.Field
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
}
type DeleteResolutionCommand struct {
	TicketID    common.TicketID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
}

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
	ResolutionPayload  []workorcheventpayload.Field
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

func (r DecisionCommandResult) Clone() DecisionCommandResult {
	return DecisionCommandResult{
		Ticket: decisionmodel.CloneTicket(r.Ticket), Tickets: decisionmodel.CloneTickets(r.Tickets),
		Resolution: decisionmodel.CloneResolution(r.Resolution), Resolutions: decisionmodel.CloneResolutions(r.Resolutions),
	}
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeCapabilities).MessageContract(),
		eventcontract.CommandTopic[GetTicketCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandGetTicket).MessageContract(),
		eventcontract.CommandTopic[ListTicketsCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandListTickets).MessageContract(),
		eventcontract.CommandTopic[ListTicketsByRunCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandListTicketsByRun).MessageContract(),
		eventcontract.CommandTopic[ListTicketsByWorkspaceCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandListTicketsByWS).MessageContract(),
		eventcontract.CommandTopic[DeleteTicketCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandDeleteTicket).MessageContract(),
		eventcontract.CommandTopic[GetResolutionCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandGetResolution).MessageContract(),
		eventcontract.CommandTopic[DeleteResolutionCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandDeleteResolution).MessageContract(),
		eventcontract.CommandTopic[EnsureResolvedCommand, DecisionCommandResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandEnsureResolved).MessageContract(),
		eventcontract.CommandTopic[RecordDecisionEvidenceCommand, RecordDecisionEvidenceResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandRecordEvidence).MessageContract(),
		eventcontract.CommandTopic[OpenRuntimeGovernanceRequestCommand, OpenRuntimeGovernanceRequestResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOpenRuntimeGovernanceRequest).MessageContract(),
	}
	for _, topic := range []string{
		TopicTicketCreated,
	} {
		ret = append(ret, eventcontract.DataTopic[TicketChangedData]("decision", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	ret = append(ret,
		eventcontract.DataTopic[TicketResolvedData]("decision", definitionPackage, eventcontract.LanePolicyState, TopicTicketResolved).MessageContract(),
	)
	return ret
}
