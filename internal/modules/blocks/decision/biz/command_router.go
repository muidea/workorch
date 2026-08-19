package decision

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type decisionCommandFamily uint8

const (
	decisionCommandFamilyTicketQuery decisionCommandFamily = iota + 1
	decisionCommandFamilyResolution
	decisionCommandFamilyAggregatePurge
)

func decisionCommandTopics() []string {
	return []string{
		decisionevents.TopicDecisionCommandEnsureResolved,
		decisionevents.TopicDecisionCommandGetTicket,
		decisionevents.TopicDecisionCommandListTickets,
		decisionevents.TopicDecisionCommandListTicketsByRun,
		decisionevents.TopicDecisionCommandListTicketsByWS,
		decisionevents.TopicDecisionCommandGetResolution,
		decisionevents.TopicDecisionCommandPurgeWorkspace,
	}
}

func decisionCommandFamilyForTopic(topic string) (decisionCommandFamily, bool) {
	switch topic {
	case decisionevents.TopicDecisionCommandGetTicket,
		decisionevents.TopicDecisionCommandListTickets,
		decisionevents.TopicDecisionCommandListTicketsByRun,
		decisionevents.TopicDecisionCommandListTicketsByWS:
		return decisionCommandFamilyTicketQuery, true
	case decisionevents.TopicDecisionCommandEnsureResolved,
		decisionevents.TopicDecisionCommandGetResolution:
		return decisionCommandFamilyResolution, true
	case decisionevents.TopicDecisionCommandPurgeWorkspace:
		return decisionCommandFamilyAggregatePurge, true
	default:
		return 0, false
	}
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	if m == nil || m.tickets == nil || m.resolutions == nil || m.service == nil || ev == nil {
		setDecisionMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return
	}
	ctx, err := m.bindDecisionCommandScope(ev.Context(), ev.Data())
	if err != nil {
		setDecisionMagicResult(result, struct{}{}, err)
		return
	}
	switch ev.ID() {
	case decisionevents.TopicDecisionCommandGetTicket:
		response, err := m.executeGetTicketCommand(ctx, ev.Data())
		setDecisionMagicResult(result, response, err)
	case decisionevents.TopicDecisionCommandListTickets:
		response, err := m.executeListTicketsCommand(ctx, ev.Data())
		setDecisionMagicResult(result, response, err)
	case decisionevents.TopicDecisionCommandListTicketsByRun:
		response, err := m.executeListTicketsByRunCommand(ctx, ev.Data())
		setDecisionMagicResult(result, response, err)
	case decisionevents.TopicDecisionCommandListTicketsByWS:
		response, err := m.executeListTicketsByWorkspaceCommand(ctx, ev.Data())
		setDecisionMagicResult(result, response, err)
	case decisionevents.TopicDecisionCommandEnsureResolved:
		response, err := m.executeEnsureResolvedCommand(ctx, ev.Data())
		setDecisionMagicResult(result, response, err)
	case decisionevents.TopicDecisionCommandGetResolution:
		response, err := m.executeGetResolutionCommand(ctx, ev.Data())
		setDecisionMagicResult(result, response, err)
	case decisionevents.TopicDecisionCommandPurgeWorkspace:
		response, err := m.executePurgeWorkspaceCommand(ctx, ev.Data())
		setDecisionMagicResult(result, response, err)
	default:
		setDecisionMagicResult(result, struct{}{}, common.ErrInvalidArgument)
	}
}

func (m *Biz) bindDecisionCommandScope(ctx context.Context, data any) (context.Context, error) {
	workspaceID := common.WorkspaceID("")
	switch cmd := data.(type) {
	case decisionevents.EnsureResolvedCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.PurgeWorkspaceDecisionsCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.GetTicketCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.ListTicketsByRunCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.ListTicketsByWorkspaceCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.GetResolutionCommand:
		workspaceID = cmd.WorkspaceID
	}
	if workspaceID == "" {
		return ctx, nil
	}
	return common.BindRuntimeWorkspaceContext(ctx, workspaceID)
}

func setDecisionMagicResult[Result any](result magicevent.Result, data Result, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}
