package biz

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// ObservePreparationRunHistory obtains a bounded point-in-time owner fact.
// It does not replace Workspace reservation/epoch or the final preparation CAS,
// and it is not called by the persisted preparation-state GET.
func (b *Biz) ObservePreparationRunHistory(ctx context.Context, workspaceID common.WorkspaceID, requestID string) (runevents.ObserveWorkspacePreparationHistoryResult, error) {
	c := runevents.ObserveWorkspacePreparationHistoryCommand{WorkspaceID: workspaceID, RequestID: requestID}
	if b == nil || b.EventHub() == nil || ctx == nil || c.Validate() != nil {
		return runevents.ObserveWorkspacePreparationHistoryResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return runevents.ObserveWorkspacePreparationHistoryResult{}, err
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return runevents.ObserveWorkspacePreparationHistoryResult{}, err
	}
	bounded, cancel := context.WithTimeout(bound, 2*time.Second)
	defer cancel()
	ev := magicevent.NewEventWithContext(runevents.TopicObserveWorkspacePreparationHistory, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), bounded, c)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(runevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(workspaceID)}))
	result := b.SendEvent(ev)
	if result == nil || result.Error() != nil {
		return runevents.ObserveWorkspacePreparationHistoryResult{}, common.ErrConflict
	}
	value, resultErr := result.Get()
	r, ok := value.(runevents.ObserveWorkspacePreparationHistoryResult)
	if resultErr != nil || !ok || r.ValidateFor(c) != nil {
		return runevents.ObserveWorkspacePreparationHistoryResult{}, common.ErrConflict
	}
	return r.Clone(), nil
}
