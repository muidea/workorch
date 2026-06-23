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
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
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
	ticket, err := ReadDecisionTicketForResolution(ctx, hub, ticketID, scope)
	if err != nil {
		return readmodelcore.Decision{}, err
	}
	item, resolution, err := resolveDecisionTicket(ctx, hub, ticket, resolutionType, payload, scope)
	if err != nil {
		return readmodelcore.Decision{}, err
	}
	publishDecisionResolutionRunResume(ctx, hub, item)
	autoGovernance, err := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
	if err != nil {
		return readmodelcore.Decision{}, err
	}
	return readmodelprojection.MapDecision(item, resolution, autoGovernance), nil
}

func ReadDecisionTicketForResolution(ctx context.Context, hub events.Hub, ticketID common.TicketID, scope common.WorkspaceID) (*decisionmodel.Ticket, error) {
	if ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	if scope != "" {
		return ReadDecisionTicket(common.ContextWithRuntimeWorkspaceID(ctx, scope), hub, ticketID)
	}
	candidates, err := DecisionTicketsForQuery(ctx, hub, "", "")
	if err != nil {
		return nil, err
	}
	var matches []*decisionmodel.Ticket
	var openMatches []*decisionmodel.Ticket
	for _, item := range candidates {
		if item == nil || item.ID != ticketID {
			continue
		}
		matches = append(matches, item)
		if item.State == common.DecisionTicketStateOpen {
			openMatches = append(openMatches, item)
		}
	}
	switch {
	case len(openMatches) == 1:
		return openMatches[0], nil
	case len(openMatches) > 1:
		return nil, common.ErrConflict
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		return nil, common.ErrConflict
	default:
		return nil, common.ErrNotFound
	}
}

func resolveDecisionTicket(ctx context.Context, hub events.Hub, ticket *decisionmodel.Ticket, resolutionType string, payload map[string]any, scope common.WorkspaceID) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	normalizedPayload, normalizedType, err := prepareDecisionResolution(ticket, resolutionType, payload, scope)
	if err != nil {
		return nil, nil, err
	}
	return ResolveDecision(ctx, hub, ticket, normalizedType, common.RedactSensitiveMap(normalizedPayload))
}

func publishDecisionResolutionRunResume(ctx context.Context, hub events.Hub, ticket *decisionmodel.Ticket) {
	if hub == nil || ticket == nil || ticket.WorkspaceID == "" || ticket.RunID == "" {
		return
	}
	err := hub.Publish(ctx, events.Envelope{
		Topic:       orchestratorevents.TopicOrchestratorCommandResumeRun,
		SubjectType: "run",
		SubjectID:   string(ticket.RunID),
		WorkspaceID: string(ticket.WorkspaceID),
		RunID:       string(ticket.RunID),
		Payload: map[string]any{
			events.PayloadKeyCommand: runcontracts.ResumeRunCommand{
				WorkspaceID: ticket.WorkspaceID,
				RunID:       ticket.RunID,
			},
		},
	})
	if _, ok := events.ResponseEnvelope(err); ok {
		return
	}
}

func prepareDecisionResolution(ticket *decisionmodel.Ticket, resolutionType string, payload map[string]any, scope common.WorkspaceID) (map[string]any, string, error) {
	if err := governancecore.EnsureDecisionTicketWorkspaceScope(scope, ticket); err != nil {
		return nil, "", err
	}
	// State-check is delegated to the kernel: kernel.EnsureResolvedTicketAndReturn
	// performs an orphan-resolution recovery (accepts matching payload on an
	// already-resolved ticket idempotently) and returns ErrConflict for state
	// mismatches. Re-checking state here would block the idempotent path before
	// the kernel can do its orphan check. The batch path's pre-validation loop
	// no longer short-circuits on state=resolved, so the per-ticket kernel call
	// is the single source of truth for conflict detection.
	normalizedPayload, normalizedType := governancecore.NormalizeDecisionResolutionPayloadForTicket(ticket, payload, resolutionType)
	if err := governancecore.ValidateDecisionResolutionPayloadForTicket(ticket, normalizedPayload); err != nil {
		return nil, "", err
	}
	return normalizedPayload, normalizedType, nil
}
