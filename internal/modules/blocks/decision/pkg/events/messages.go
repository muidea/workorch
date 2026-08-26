package events

import (
	"fmt"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
const CommandDestination = modulecommon.ModuleID

const (
	DefaultListLimit = 100
	MaxListLimit     = 200
)

type TicketListScope string

const TicketListScopeAll TicketListScope = "all"

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

type ListTicketsCommand struct {
	Scope  TicketListScope
	Cursor string
	Limit  int
}

type ListTicketsByRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Cursor      string
	Limit       int
}

type ListTicketsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
	Cursor      string
	Limit       int
}

type SealWorkspaceDecisionDeletionCommand struct {
	WorkspaceID               common.WorkspaceID
	DeletionID                string
	DeletionGeneration        uint64
	DataDispositionIntent     deletioncontract.Mode
	QuiescenceID, RequestHash string
}

func (c SealWorkspaceDecisionDeletionCommand) Scope() deletioncontract.Scope {
	return deletioncontract.Scope{WorkspaceID: c.WorkspaceID, DeletionID: c.DeletionID, DeletionGeneration: c.DeletionGeneration, Mode: c.DataDispositionIntent, QuiescenceID: c.QuiescenceID, RequestHash: c.RequestHash}.Normalize()
}
func (c SealWorkspaceDecisionDeletionCommand) Validate() error { return c.Scope().Validate() }

type PurgeWorkspaceDecisionsCommand struct {
	WorkspaceID               common.WorkspaceID
	DeletionID                string
	DeletionGeneration        uint64
	QuiescenceID, RequestHash string
}

func (c PurgeWorkspaceDecisionsCommand) Scope() deletioncontract.Scope {
	return deletioncontract.Scope{WorkspaceID: c.WorkspaceID, DeletionID: c.DeletionID, DeletionGeneration: c.DeletionGeneration, Mode: deletioncontract.ModePurge, QuiescenceID: c.QuiescenceID, RequestHash: c.RequestHash}.Normalize()
}
func (c PurgeWorkspaceDecisionsCommand) Validate() error {
	return c.Scope().Validate()
}

type GetResolutionCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	TicketID    common.TicketID
}

type TicketChangedData struct {
	NotificationID string
	TicketID       common.TicketID
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	StepID         common.StepID
	Reason         string
	DecisionKind   string
	State          common.DecisionTicketState
}

type TicketResolvedData struct {
	NotificationID     string
	Ticket             TicketChangedData
	ResolutionID       common.ResolutionID
	ResolutionType     string
	ResolutionPayload  []workorcheventpayload.Field
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
}

type GetTicketResult struct{ Ticket *decisionmodel.Ticket }
type ListTicketsResult struct {
	Tickets    []*decisionmodel.Ticket
	NextCursor string
}
type ListTicketsByRunResult struct {
	Tickets    []*decisionmodel.Ticket
	NextCursor string
}
type ListTicketsByWorkspaceResult struct {
	Tickets    []*decisionmodel.Ticket
	NextCursor string
}
type GetResolutionResult struct{ Resolution *decisionmodel.Resolution }
type EnsureResolvedResult struct {
	Ticket     *decisionmodel.Ticket
	Resolution *decisionmodel.Resolution
}
type SealWorkspaceDecisionDeletionResult struct {
	WorkspaceID        common.WorkspaceID
	DeletionID         string
	DeletionGeneration uint64
	FenceID            string
	Revision           uint64
	AckHash            string
}
type PurgeWorkspaceDecisionsResult struct {
	WorkspaceID             common.WorkspaceID
	DeletionID              string
	DeletionGeneration      uint64
	FenceID, PurgeID        string
	Revision                uint64
	AckHash, PurgeProofHash string
}

func NormalizeListWindow(cursor string, limit int) (string, int, error) {
	cursor = strings.TrimSpace(cursor)
	if limit == 0 {
		limit = DefaultListLimit
	}
	if limit < 1 || limit > MaxListLimit {
		return "", 0, fmt.Errorf("%w: decision list limit must be between 1 and %d", common.ErrInvalidArgument, MaxListLimit)
	}
	return cursor, limit, nil
}

func TicketCursor(item *decisionmodel.Ticket) string {
	if item == nil {
		return ""
	}
	return string(item.WorkspaceID) + "\x00" + string(item.RunID) + "\x00" + string(item.ID)
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeCapabilities).MessageContract(),
		eventcontract.CommandTopic[GetTicketCommand, GetTicketResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandGetTicket).MessageContract(),
		eventcontract.CommandTopic[ListTicketsCommand, ListTicketsResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandListTickets).MessageContract(),
		eventcontract.CommandTopic[ListTicketsByRunCommand, ListTicketsByRunResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandListTicketsByRun).MessageContract(),
		eventcontract.CommandTopic[ListTicketsByWorkspaceCommand, ListTicketsByWorkspaceResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandListTicketsByWS).MessageContract(),
		eventcontract.CommandTopic[GetResolutionCommand, GetResolutionResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicDecisionCommandGetResolution).MessageContract(),
		eventcontract.CommandTopic[QueryWorkspaceDefinitionClarificationFactCommand, QueryWorkspaceDefinitionClarificationFactResult]("decision", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicQueryWorkspaceDefinitionClarificationFact).MessageContract(),
		eventcontract.CommandTopic[EnsureResolvedCommand, EnsureResolvedResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandEnsureResolved).MessageContract(),
		eventcontract.CommandTopic[SealWorkspaceDecisionDeletionCommand, SealWorkspaceDecisionDeletionResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandSealWorkspaceDeletion).MessageContract(),
		eventcontract.CommandTopic[PurgeWorkspaceDecisionsCommand, PurgeWorkspaceDecisionsResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandPurgeWorkspace).MessageContract(),
		eventcontract.CommandTopic[RecordDecisionEvidenceCommand, RecordDecisionEvidenceResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicDecisionCommandRecordEvidence).MessageContract(),
		eventcontract.CommandTopic[OpenRuntimeGovernanceRequestCommand, OpenRuntimeGovernanceRequestResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOpenRuntimeGovernanceRequest).MessageContract(),
		eventcontract.CommandTopic[OpenWorkspaceDefinitionClarificationRequestCommand, OpenWorkspaceDefinitionClarificationRequestResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOpenWorkspaceDefinitionClarification).MessageContract(),
		eventcontract.CommandTopic[ReconcileWorkspaceDefinitionClarificationCandidateCommand, ReconcileWorkspaceDefinitionClarificationCandidateResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicReconcileWorkspaceDefinitionClarificationCandidate).MessageContract(),
		eventcontract.CommandTopic[ResolveWorkspaceDefinitionClarificationByAdmissionCommand, ResolveWorkspaceDefinitionClarificationByAdmissionResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicResolveWorkspaceDefinitionClarificationByAdmission).MessageContract(),
		eventcontract.CommandTopic[OpenRuntimeDefinitionClarificationRequestCommand, OpenRuntimeDefinitionClarificationRequestResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOpenRuntimeDefinitionClarification).MessageContract(),
		eventcontract.CommandTopic[ResolveRuntimeDefinitionClarificationByReplacementCommand, ResolveRuntimeDefinitionClarificationByReplacementResult]("decision", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicResolveRuntimeDefinitionClarificationByReplacement).MessageContract(),
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
