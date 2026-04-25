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
	State             string         `json:"state"`
	ResolutionID      string         `json:"resolutionId,omitempty"`
	ResolutionType    string         `json:"resolutionType,omitempty"`
	ResolutionPayload map[string]any `json:"resolutionPayload,omitempty"`
}

func ReadDecisionWithSession(ctx context.Context, session *RuntimeSession, ticketID string) (ret Decision, err error) {
	_, tickets, resolutions, err := decisionDependenciesFromSession(session)
	if err != nil {
		return Decision{}, err
	}
	defer func() {
		err = errors.Join(err, session.Close(ctx))
	}()

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
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
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
	defer func() {
		err = errors.Join(err, session.Close(ctx))
	}()

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
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
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

func ResolveDecisionWithSession(ctx context.Context, session *RuntimeSession, ticketID string) (ret Decision, err error) {
	return resolveDecisionWithSession(ctx, session, ticketID, false)
}

func ResolveDecisionWithBackend(ctx context.Context, backend string, dataRoot string, ticketID string) (Decision, error) {
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		return Decision{}, err
	}
	return ResolveDecisionWithSession(ctx, session, ticketID)
}

func ResolveDecisionFromEnv(ctx context.Context, ticketID string) (Decision, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Decision{}, err
	}
	return ResolveDecisionWithSession(ctx, session, ticketID)
}

func RejectDecisionWithSession(ctx context.Context, session *RuntimeSession, ticketID string) (ret Decision, err error) {
	return resolveDecisionWithSession(ctx, session, ticketID, true)
}

func RejectDecisionWithBackend(ctx context.Context, backend string, dataRoot string, ticketID string) (Decision, error) {
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		return Decision{}, err
	}
	return RejectDecisionWithSession(ctx, session, ticketID)
}

func RejectDecisionFromEnv(ctx context.Context, ticketID string) (Decision, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Decision{}, err
	}
	return RejectDecisionWithSession(ctx, session, ticketID)
}

func resolveDecisionWithSession(ctx context.Context, session *RuntimeSession, ticketID string, reject bool) (ret Decision, err error) {
	service, tickets, resolutions, err := decisionDependenciesFromSession(session)
	if err != nil {
		return Decision{}, err
	}
	defer func() {
		err = errors.Join(err, session.Close(ctx))
	}()

	typeName := decisioncommon.ResolutionTypeApprovePath
	resolutionID := common.ResolutionID("resolution-" + ticketID)
	if reject {
		typeName = decisioncommon.ResolutionTypeRejectPath
		resolutionID = common.ResolutionID("resolution-" + ticketID + "-reject")
	}

	if err := service.EnsureResolvedTicket(ctx, decisionmodule.ResolveTicketCommand{
		ResolutionID: resolutionID,
		TicketID:     common.TicketID(ticketID),
		Type:         typeName,
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

func decisionDependenciesFromSession(session *RuntimeSession) (decisionmodule.Service, repository.DecisionTicketRepository, repository.DecisionResolutionRepository, error) {
	if session == nil || session.Bootstrap == nil {
		return nil, nil, nil, common.ErrInvalidArgument
	}
	exports := session.Bootstrap.Exports()
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
		ID:          string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		RunID:       string(item.RunID),
		StepID:      string(item.StepID),
		Reason:      item.Reason,
		State:       string(item.State),
	}
	if resolution != nil {
		ret.ResolutionID = string(resolution.ID)
		ret.ResolutionType = resolution.Type
		ret.ResolutionPayload = resolution.Payload
	}
	return ret
}
