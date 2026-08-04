package governanceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/governancesemantics"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func BatchResolveDecisionViews(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, cmd appmodelcore.BatchResolveDecisionCommand, scope common.WorkspaceID) ([]workorchprojectioncontract.Decision, error) {
	semantic := governancesemantics.NormalizeDecisionResolution(cmd.ResolutionType, cmd.ResolutionPayload)
	resolutionType := semantic.ResolutionType
	if !decisioncommon.IsSupportedResolutionType(resolutionType) || len(cmd.TicketIDs) == 0 {
		return nil, common.ErrInvalidArgument
	}
	payload := governancecore.WithDecisionSemanticPayload(cmd.ResolutionPayload, semantic)
	seen := make(map[string]struct{}, len(cmd.TicketIDs))
	for _, ticketID := range cmd.TicketIDs {
		if ticketID == "" {
			return nil, common.ErrInvalidArgument
		}
		if _, ok := seen[ticketID]; ok {
			return nil, common.ErrInvalidArgument
		}
		seen[ticketID] = struct{}{}
	}
	type ticketRead struct {
		item *decisionmodel.Ticket
		err  error
	}
	reads := make([]ticketRead, len(cmd.TicketIDs))
	for i, ticketID := range cmd.TicketIDs {
		reads[i].item, reads[i].err = ReadDecisionTicketForResolution(ctx, hub, common.TicketID(ticketID), scope)
	}
	for _, r := range reads {
		if r.err != nil {
			return nil, r.err
		}
		if decisionResolutionMissingRequiredInfo(semantic, cmd.ResolutionPayload) {
			return nil, common.ErrInvalidArgument
		}
		if _, _, err := prepareDecisionResolution(r.item, resolutionType, payload, scope); err != nil {
			return nil, err
		}
	}

	ret := make([]workorchprojectioncontract.Decision, 0, len(cmd.TicketIDs))
	for _, r := range reads {
		item, resolution, err := resolveDecisionTicket(ctx, hub, r.item, resolutionType, governancecore.CloneDecisionPayload(payload), scope)
		if err != nil {
			return nil, err
		}
		autoGovernance, autoErr := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
		if autoErr != nil {
			return nil, autoErr
		}
		ret = append(ret, readmodelprojection.MapDecision(item, resolution, autoGovernance))
	}
	return ret, nil
}
