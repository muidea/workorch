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
		decisionevents.TopicDecisionCommandSealWorkspaceDeletion,
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
	case decisionevents.TopicDecisionCommandPurgeWorkspace, decisionevents.TopicDecisionCommandSealWorkspaceDeletion:
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
	data, err := m.resolveDecisionMutationScope(ev.Context(), ev.Data())
	if err != nil {
		setDecisionMagicResult(result, struct{}{}, err)
		return
	}
	ctx, err := m.bindDecisionCommandScope(ev.Context(), data)
	if err != nil {
		setDecisionMagicResult(result, struct{}{}, err)
		return
	}
	workspaceID := decisionCommandWorkspaceID(data)
	if workspaceID != "" {
		unlock := m.lockWorkspaceMutation(workspaceID)
		defer unlock()
		if ev.ID() != decisionevents.TopicDecisionCommandSealWorkspaceDeletion && ev.ID() != decisionevents.TopicDecisionCommandPurgeWorkspace {
			if err := m.rejectFencedWorkspace(ctx, workspaceID); err != nil {
				setDecisionMagicResult(result, struct{}{}, err)
				return
			}
		}
	}
	switch ev.ID() {
	case decisionevents.TopicDecisionCommandSealWorkspaceDeletion:
		response, err := m.executeSealWorkspaceDeletion(ctx, ev)
		setDecisionMagicResult(result, response, err)
	case decisionevents.TopicDecisionCommandGetTicket:
		response, err := m.executeGetTicketCommand(ctx, data)
		setDecisionMagicResult(result, response, err)
	case decisionevents.TopicDecisionCommandListTickets:
		response, err := m.executeListTicketsCommand(ctx, data)
		setDecisionMagicResult(result, response, err)
	case decisionevents.TopicDecisionCommandListTicketsByRun:
		response, err := m.executeListTicketsByRunCommand(ctx, data)
		setDecisionMagicResult(result, response, err)
	case decisionevents.TopicDecisionCommandListTicketsByWS:
		response, err := m.executeListTicketsByWorkspaceCommand(ctx, data)
		setDecisionMagicResult(result, response, err)
	case decisionevents.TopicDecisionCommandEnsureResolved:
		response, err := m.executeEnsureResolvedCommand(ctx, data)
		setDecisionMagicResult(result, response, err)
	case decisionevents.TopicDecisionCommandGetResolution:
		response, err := m.executeGetResolutionCommand(ctx, data)
		setDecisionMagicResult(result, response, err)
	case decisionevents.TopicDecisionCommandPurgeWorkspace:
		response, err := m.executePurgeWorkspaceCommand(ctx, ev)
		setDecisionMagicResult(result, response, err)
	default:
		setDecisionMagicResult(result, struct{}{}, common.ErrInvalidArgument)
	}
}

// resolveDecisionMutationScope preserves the unique unscoped resolution
// ingress while ensuring the production mutation path acquires the resolved
// workspace lock before it can mutate state or emit owner delivery.
func (m *Biz) resolveDecisionMutationScope(ctx context.Context, data any) (any, error) {
	cmd, ok := data.(decisionevents.EnsureResolvedCommand)
	if !ok || decisionCommandHasScope(cmd) {
		return data, nil
	}
	ticket, err := m.resolveUniqueTicketForUnscopedCommand(ctx, cmd.TicketID)
	if err != nil {
		return nil, err
	}
	cmd.WorkspaceID = ticket.WorkspaceID
	cmd.RunID = ticket.RunID
	cmd.StepID = ticket.StepID
	return cmd, nil
}

func (m *Biz) bindDecisionCommandScope(ctx context.Context, data any) (context.Context, error) {
	workspaceID := common.WorkspaceID("")
	switch cmd := data.(type) {
	case decisionevents.EnsureResolvedCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.PurgeWorkspaceDecisionsCommand:
		workspaceID = cmd.WorkspaceID
	case decisionevents.SealWorkspaceDecisionDeletionCommand:
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

func decisionCommandWorkspaceID(data any) common.WorkspaceID {
	switch cmd := data.(type) {
	case decisionevents.EnsureResolvedCommand:
		return cmd.WorkspaceID
	case decisionevents.PurgeWorkspaceDecisionsCommand:
		return cmd.WorkspaceID
	case decisionevents.SealWorkspaceDecisionDeletionCommand:
		return cmd.WorkspaceID
	case decisionevents.GetTicketCommand:
		return cmd.WorkspaceID
	case decisionevents.ListTicketsByRunCommand:
		return cmd.WorkspaceID
	case decisionevents.ListTicketsByWorkspaceCommand:
		return cmd.WorkspaceID
	case decisionevents.GetResolutionCommand:
		return cmd.WorkspaceID
	}
	return ""
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
