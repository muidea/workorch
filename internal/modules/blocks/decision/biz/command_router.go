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
	decisionCommandFamilyTicketMutation
	decisionCommandFamilyResolution
)

func decisionCommandTopics() []string {
	return []string{
		decisionevents.TopicDecisionCommandEnsureResolved,
		decisionevents.TopicDecisionCommandGetTicket,
		decisionevents.TopicDecisionCommandListTickets,
		decisionevents.TopicDecisionCommandListTicketsByRun,
		decisionevents.TopicDecisionCommandListTicketsByWS,
		decisionevents.TopicDecisionCommandDeleteTicket,
		decisionevents.TopicDecisionCommandGetResolution,
		decisionevents.TopicDecisionCommandDeleteResolution,
	}
}

func decisionCommandFamilyForTopic(topic string) (decisionCommandFamily, bool) {
	switch topic {
	case decisionevents.TopicDecisionCommandGetTicket,
		decisionevents.TopicDecisionCommandListTickets,
		decisionevents.TopicDecisionCommandListTicketsByRun,
		decisionevents.TopicDecisionCommandListTicketsByWS:
		return decisionCommandFamilyTicketQuery, true
	case decisionevents.TopicDecisionCommandDeleteTicket:
		return decisionCommandFamilyTicketMutation, true
	case decisionevents.TopicDecisionCommandEnsureResolved,
		decisionevents.TopicDecisionCommandGetResolution,
		decisionevents.TopicDecisionCommandDeleteResolution:
		return decisionCommandFamilyResolution, true
	default:
		return 0, false
	}
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev)
	setDecisionMagicResult(result, response, err)
}

func (m *Biz) executeCommand(ev magicevent.Event) (decisionevents.DecisionCommandResult, error) {
	if m == nil || m.tickets == nil || m.resolutions == nil || m.service == nil || ev == nil {
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
	ctx, err := m.bindDecisionCommandScope(ev.Context(), ev.Data())
	if err != nil {
		return decisionevents.DecisionCommandResult{}, err
	}
	family, ok := decisionCommandFamilyForTopic(ev.ID())
	if !ok {
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
	switch family {
	case decisionCommandFamilyTicketQuery:
		return m.executeTicketQueryCommand(ctx, ev)
	case decisionCommandFamilyTicketMutation:
		return m.executeTicketMutationCommand(ctx, ev)
	case decisionCommandFamilyResolution:
		return m.executeResolutionCommand(ctx, ev)
	default:
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
}

func (m *Biz) bindDecisionCommandScope(ctx context.Context, data any) (context.Context, error) {
	workspaceID := common.WorkspaceID("")
	switch cmd := data.(type) {
	case decisionevents.DeleteTicketCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.EnsureResolvedCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.DeleteResolutionCommand:
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

func setDecisionMagicResult(result magicevent.Result, data decisionevents.DecisionCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data.Clone(), nil)
}
