package workspace

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// queryRunExecutionIntentReceipt reads an exact persisted original response,
// not the side-effecting Restart replay. This sender alone cannot settle a
// Workspace reservation or grant another forward. The fresh ingress consumes
// this fact through the private settlement reducer; GET remains read-only.
func (m *Biz) queryRunExecutionIntentReceipt(ctx context.Context, c runevents.QueryWorkspaceExecutionIntentReceiptCommand) (runevents.QueryWorkspaceExecutionIntentReceiptResult, error) {
	if m == nil || m.EventHub() == nil || ctx == nil || c.Validate() != nil {
		return runevents.QueryWorkspaceExecutionIntentReceiptResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return runevents.QueryWorkspaceExecutionIntentReceiptResult{}, err
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, c.WorkspaceID)
	if err != nil {
		return runevents.QueryWorkspaceExecutionIntentReceiptResult{}, err
	}
	bounded, cancel := context.WithTimeout(bound, 2*time.Second)
	defer cancel()
	ev := magicevent.NewEventWithContext(runevents.TopicQueryWorkspaceExecutionIntentReceipt, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), bounded, c)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(runevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(c.WorkspaceID)}))
	response := m.SendEvent(ev)
	if response == nil {
		return runevents.QueryWorkspaceExecutionIntentReceiptResult{}, common.ErrConflict
	}
	if err := response.Error(); err != nil {
		if err.Cause != nil {
			return runevents.QueryWorkspaceExecutionIntentReceiptResult{}, err.Cause
		}
		return runevents.QueryWorkspaceExecutionIntentReceiptResult{}, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return runevents.QueryWorkspaceExecutionIntentReceiptResult{}, resultErr
	}
	r, ok := data.(runevents.QueryWorkspaceExecutionIntentReceiptResult)
	if !ok || r.ValidateFor(c) != nil {
		return runevents.QueryWorkspaceExecutionIntentReceiptResult{}, common.ErrConflict
	}
	return r.Clone(), nil
}
