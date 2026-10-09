package biz

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// 成功原文只在显式、按 Workspace/permit 约束的受限查询中返回。
func (b *Biz) QueryAuthoringSuccessAudit(ctx context.Context, command workspaceevents.QueryAuthoringSuccessAuditCommand) (workspaceevents.QueryAuthoringSuccessAuditResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || command.Validate() != nil {
		return workspaceevents.QueryAuthoringSuccessAuditResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.QueryAuthoringSuccessAuditResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringSuccessAuditQuery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceAuthoringSuccessAuditQuery, eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspaceevents.QueryAuthoringSuccessAuditResult{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workspaceevents.QueryAuthoringSuccessAuditResult{}, err
	}
	value, err := result.Get()
	if err != nil {
		return workspaceevents.QueryAuthoringSuccessAuditResult{}, err
	}
	response, ok := value.(workspaceevents.QueryAuthoringSuccessAuditResult)
	if !ok || response.ValidateFor(command) != nil {
		return workspaceevents.QueryAuthoringSuccessAuditResult{}, common.ErrConflict
	}
	return response.Clone(), nil
}

func (b *Biz) ReadDefinitionAuthoringSuccessAuditView(ctx context.Context, id common.WorkspaceID, permit string) (workorchmanagementcontract.DefinitionAuthoringSuccessAuditView, error) {
	result, err := b.QueryAuthoringSuccessAudit(ctx, workspaceevents.QueryAuthoringSuccessAuditCommand{WorkspaceID: id, PermitID: permit})
	if err != nil {
		return workorchmanagementcontract.DefinitionAuthoringSuccessAuditView{}, err
	}
	return workorchmanagementcontract.DefinitionAuthoringSuccessAuditView{
		WorkspaceID: string(result.WorkspaceID), PermitID: result.PermitID, CanonicalHash: result.CanonicalHash,
		Document: *authoringProviderObservationView(&result.Audit.Document),
		Wire:     *authoringProviderObservationView(&result.Audit.Wire),
	}, nil
}
