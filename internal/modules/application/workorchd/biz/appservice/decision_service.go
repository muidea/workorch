package appservice

import (
	"context"
	"errors"

	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Decision struct {
	ID                string         `json:"id"`
	WorkspaceID       string         `json:"workspaceId"`
	RunID             string         `json:"runId"`
	StepID            string         `json:"stepId,omitempty"`
	Reason            string         `json:"reason"`
	DecisionKind      string         `json:"decisionKind,omitempty"`
	Question          string         `json:"question,omitempty"`
	Context           map[string]any `json:"context,omitempty"`
	State             string         `json:"state"`
	ResolutionID      string         `json:"resolutionId,omitempty"`
	ResolutionType    string         `json:"resolutionType,omitempty"`
	ResolutionPayload map[string]any `json:"resolutionPayload,omitempty"`
}

type ResolveDecisionCommand struct {
	ResolutionType    string         `json:"resolutionType"`
	ResolutionPayload map[string]any `json:"resolutionPayload,omitempty"`
}

type BatchResolveDecisionCommand struct {
	TicketIDs         []string       `json:"ticketIds"`
	ResolutionType    string         `json:"resolutionType"`
	ResolutionPayload map[string]any `json:"resolutionPayload,omitempty"`
}

func ReadDecisionWithSession(ctx context.Context, session *RuntimeSession, ticketID string) (ret Decision, err error) {
	_, tickets, resolutions, err := decisionDependenciesFromSession(session)
	if err != nil {
		return Decision{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	item, err := tickets.Get(ctx, common.TicketID(ticketID))
	if err != nil {
		return Decision{}, err
	}
	resolution, err := resolutionForTicket(ctx, resolutions, item)
	if err != nil {
		return Decision{}, err
	}
	return mapDecision(item, resolution), nil
}

func ReadDecisionWithBackend(ctx context.Context, backend string, dataRoot string, ticketID string) (Decision, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Decision{}, err
	}
	return ReadDecisionWithSession(ctx, session, ticketID)
}

func ReadDecisionFromEnv(ctx context.Context, ticketID string) (Decision, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Decision{}, err
	}
	return ReadDecisionWithSession(ctx, session, ticketID)
}

func ListDecisionsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, runID string) (ret []Decision, err error) {
	_, tickets, resolutions, err := decisionDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := tickets.List(ctx)
	if err != nil {
		return nil, err
	}
	ret = make([]Decision, 0, len(items))
	for _, item := range items {
		if workspaceID != "" && string(item.WorkspaceID) != workspaceID {
			continue
		}
		if runID != "" && string(item.RunID) != runID {
			continue
		}
		resolution, err := resolutionForTicket(ctx, resolutions, item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, mapDecision(item, resolution))
	}
	return ret, nil
}

func ListDecisionsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]Decision, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListDecisionsWithSession(ctx, session, workspaceID, runID)
}

func ListDecisionsFromEnv(ctx context.Context, workspaceID string, runID string) ([]Decision, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ListDecisionsWithSession(ctx, session, workspaceID, runID)
}

func ResolveDecisionWithSession(ctx context.Context, session *RuntimeSession, ticketID string, cmd ResolveDecisionCommand) (ret Decision, err error) {
	service, tickets, resolutions, err := decisionDependenciesFromSession(session)
	if err != nil {
		return Decision{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	if cmd.ResolutionType == "" {
		cmd.ResolutionType = decisioncommon.ResolutionTypeProvideMissingInfo
	}
	if !decisioncommon.IsSupportedResolutionType(cmd.ResolutionType) {
		return Decision{}, common.ErrInvalidArgument
	}

	if err := service.EnsureResolvedTicket(ctx, decisionmodule.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-" + ticketID + "-" + cmd.ResolutionType),
		TicketID:     common.TicketID(ticketID),
		Type:         cmd.ResolutionType,
		Payload:      cloneDecisionPayload(cmd.ResolutionPayload),
	}); err != nil {
		return Decision{}, err
	}

	item, err := tickets.Get(ctx, common.TicketID(ticketID))
	if err != nil {
		return Decision{}, err
	}
	resolution, err := resolutionForTicket(ctx, resolutions, item)
	if err != nil {
		return Decision{}, err
	}
	return mapDecision(item, resolution), nil
}

func BatchResolveDecisionsWithSession(ctx context.Context, session *RuntimeSession, cmd BatchResolveDecisionCommand) (ret []Decision, err error) {
	service, tickets, resolutions, err := decisionDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	resolutionType := cmd.ResolutionType
	if resolutionType == "" {
		resolutionType = decisioncommon.ResolutionTypeProvideMissingInfo
	}
	if !decisioncommon.IsSupportedResolutionType(resolutionType) || len(cmd.TicketIDs) == 0 {
		return nil, common.ErrInvalidArgument
	}
	seen := make(map[string]struct{}, len(cmd.TicketIDs))
	for _, ticketID := range cmd.TicketIDs {
		if ticketID == "" {
			return nil, common.ErrInvalidArgument
		}
		if _, ok := seen[ticketID]; ok {
			return nil, common.ErrInvalidArgument
		}
		seen[ticketID] = struct{}{}
		item, err := tickets.Get(ctx, common.TicketID(ticketID))
		if err != nil {
			return nil, err
		}
		if item.State != common.DecisionTicketStateOpen {
			return nil, common.ErrConflict
		}
	}

	ret = make([]Decision, 0, len(cmd.TicketIDs))
	for _, ticketID := range cmd.TicketIDs {
		if err := service.EnsureResolvedTicket(ctx, decisionmodule.ResolveTicketCommand{
			ResolutionID: common.ResolutionID("resolution-" + ticketID + "-" + resolutionType),
			TicketID:     common.TicketID(ticketID),
			Type:         resolutionType,
			Payload:      cloneDecisionPayload(cmd.ResolutionPayload),
		}); err != nil {
			return nil, err
		}
		item, err := tickets.Get(ctx, common.TicketID(ticketID))
		if err != nil {
			return nil, err
		}
		resolution, err := resolutionForTicket(ctx, resolutions, item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, mapDecision(item, resolution))
	}
	return ret, nil
}

func ResolveDecisionWithBackend(ctx context.Context, backend string, dataRoot string, ticketID string, cmd ResolveDecisionCommand) (Decision, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Decision{}, err
	}
	return ResolveDecisionWithSession(ctx, session, ticketID, cmd)
}

func BatchResolveDecisionsWithBackend(ctx context.Context, backend string, dataRoot string, cmd BatchResolveDecisionCommand) ([]Decision, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return BatchResolveDecisionsWithSession(ctx, session, cmd)
}

func ResolveDecisionFromEnv(ctx context.Context, ticketID string, cmd ResolveDecisionCommand) (Decision, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Decision{}, err
	}
	return ResolveDecisionWithSession(ctx, session, ticketID, cmd)
}

func BatchResolveDecisionsFromEnv(ctx context.Context, cmd BatchResolveDecisionCommand) ([]Decision, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return BatchResolveDecisionsWithSession(ctx, session, cmd)
}

func decisionDependenciesFromSession(session *RuntimeSession) (decisionmodule.Service, repository.DecisionTicketRepository, repository.DecisionResolutionRepository, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, nil, nil, err
	}
	if exports.Services.DecisionService == nil || exports.Repositories.Tickets == nil || exports.Repositories.Resolutions == nil {
		return nil, nil, nil, common.ErrInvalidArgument
	}
	return exports.Services.DecisionService, exports.Repositories.Tickets, exports.Repositories.Resolutions, nil
}

func resolutionForTicket(ctx context.Context, resolutions repository.DecisionResolutionRepository, item *decisionmodel.Ticket) (*decisionmodel.Resolution, error) {
	if item == nil || resolutions == nil || item.State != common.DecisionTicketStateResolved {
		return nil, nil
	}
	resolution, err := resolutions.GetByTicketID(ctx, item.ID)
	if errors.Is(err, common.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return resolution, nil
}

func mapDecision(item *decisionmodel.Ticket, resolution *decisionmodel.Resolution) Decision {
	if item == nil {
		return Decision{}
	}
	ret := Decision{
		ID:           string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		StepID:       string(item.StepID),
		Reason:       item.Reason,
		DecisionKind: item.DecisionKind,
		Question:     item.Question,
		Context:      cloneDecisionPayload(item.Context),
		State:        string(item.State),
	}
	if resolution != nil {
		ret.ResolutionID = string(resolution.ID)
		ret.ResolutionType = resolution.Type
		ret.ResolutionPayload = cloneDecisionPayload(resolution.Payload)
	}
	return ret
}

func cloneDecisionPayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for k, v := range input {
		ret[k] = v
	}
	return ret
}
