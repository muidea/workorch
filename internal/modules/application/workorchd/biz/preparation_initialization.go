package biz

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func (b *Biz) InitializeStartupPreparationResources(ctx context.Context, id common.WorkspaceID, request workorchmanagementcontract.PreparationCheckRequest, recoverOriginal bool) (workorchmanagementcontract.PreparationResourceInitializationOperation, error) {
	empty := workorchmanagementcontract.PreparationResourceInitializationOperation{}
	if b == nil || b.EventHub() == nil || ctx == nil || id == "" || request.Validate() != nil {
		return empty, common.ErrInvalidArgument
	}
	home, homeErr := contextregistryports.SkillHubRegistrationHome()
	if homeErr != nil {
		return empty, homeErr
	}
	c := workspaceevents.InitializeWorkspacePreparationCommand{RegistrationHome: home, WorkspaceID: id, RequestID: request.RequestID, ExpectedWorkspaceRevision: request.ExpectedWorkspaceRevision}
	topic := workspaceevents.TopicWorkspacePreparationInitializeResources
	if recoverOriginal {
		topic = workspaceevents.TopicWorkspacePreparationRecoverResources
	}
	ev := magicevent.NewEventWithContext(topic, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, c)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), topic, eventcontract.LaneScope{WorkspaceID: string(id)}))
	result := b.SendEvent(ev)
	if result == nil {
		return empty, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return empty, err
	}
	v, err := result.Get()
	if err != nil {
		return empty, err
	}
	r, ok := v.(workspaceevents.InitializeWorkspacePreparationResult)
	if !ok || r.ValidateFor(c) != nil {
		return empty, common.ErrConflict
	}
	if r.Registration != nil {
		observation := contextregistryports.ObserveSkillHubRegistration(ctx, r.Registration.Plan, r.Registration.Query)
		settlement := workspaceevents.SettlePreparationRegistrationCommand{Original: c, Dispatch: *r.Registration, Observation: observation}
		settleEvent := magicevent.NewEventWithContext(workspaceevents.TopicWorkspacePreparationRegistrationSettle, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, settlement)
		settleEvent.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), settleEvent.ID(), eventcontract.LaneScope{WorkspaceID: string(id)}))
		settled := b.SendEvent(settleEvent)
		if settled == nil {
			return empty, common.ErrConflict
		}
		if err := settled.Error(); err != nil {
			return empty, err
		}
		value, err := settled.Get()
		if err != nil {
			return empty, err
		}
		r, ok = value.(workspaceevents.InitializeWorkspacePreparationResult)
		if !ok || r.ValidateFor(c) != nil || r.Registration != nil {
			return empty, common.ErrConflict
		}
	}
	view, projectionErr := startupPreparationView(r.State)
	if projectionErr != nil {
		return empty, projectionErr
	}
	out := workorchmanagementcontract.PreparationResourceInitializationOperation{RequestID: r.RequestID, Preparation: view}
	return out, out.ValidateFor(string(id), request)
}
