package governanceports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ResolveDecisionView(ctx context.Context, hub events.Hub, traces governancecore.AutoGovernanceTraceReader, ticketID common.TicketID, cmd appmodelcore.ResolveDecisionCommand, scope common.WorkspaceID) (readmodelcore.Decision, error) {
	semantic := governancecommon.NormalizeDecisionResolution(cmd.ResolutionType, cmd.ResolutionPayload)
	if !decisioncommon.IsSupportedResolutionType(semantic.ResolutionType) {
		return readmodelcore.Decision{}, common.ErrInvalidArgument
	}
	resolutionType := semantic.ResolutionType
	payload := governancecore.WithDecisionSemanticPayload(cmd.ResolutionPayload, semantic)
	ticket, err := ReadDecisionTicket(ctx, hub, ticketID)
	if err != nil {
		return readmodelcore.Decision{}, err
	}
	item, resolution, err := resolveDecisionTicket(ctx, hub, ticket, resolutionType, payload, scope)
	if err != nil {
		return readmodelcore.Decision{}, err
	}
	autoGovernance, err := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
	if err != nil {
		return readmodelcore.Decision{}, err
	}
	return readmodelprojection.MapDecision(item, resolution, autoGovernance), nil
}

func resolveDecisionTicket(ctx context.Context, hub events.Hub, ticket *decisionmodel.Ticket, resolutionType string, payload map[string]any, scope common.WorkspaceID) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	normalizedPayload, normalizedType, err := prepareDecisionResolution(ticket, resolutionType, payload, scope)
	if err != nil {
		return nil, nil, err
	}
	return ResolveDecision(ctx, hub, ticket, normalizedType, common.RedactSensitiveMap(normalizedPayload))
}

func prepareDecisionResolution(ticket *decisionmodel.Ticket, resolutionType string, payload map[string]any, scope common.WorkspaceID) (map[string]any, string, error) {
	if err := governancecore.EnsureDecisionTicketWorkspaceScope(scope, ticket); err != nil {
		return nil, "", err
	}
	if ticket.State != common.DecisionTicketStateOpen {
		return nil, "", common.ErrConflict
	}
	normalizedPayload, normalizedType := governancecore.NormalizeDecisionResolutionPayloadForTicket(ticket, payload, resolutionType)
	if err := governancecore.ValidateDecisionResolutionPayloadForTicket(ticket, normalizedPayload); err != nil {
		return nil, "", err
	}
	return normalizedPayload, normalizedType, nil
}
