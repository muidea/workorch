package result

import (
	"context"

	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type lifecycleInternalEventKind string

const lifecycleInternalEventTerminalDeliveryObserved lifecycleInternalEventKind = "terminal_delivery_observed"

type lifecycleInternalEvent struct {
	Kind             lifecycleInternalEventKind
	TerminalDelivery *resultevents.RuntimeTerminalDeliveryFact
}

// submitLifecycleInternalEvent is the Result owner's private reducer gate.
// Public EventHub handlers may provide sealed facts only; they cannot select
// Result state or invoke persistence directly.
func (m *Biz) submitLifecycleInternalEvent(ctx context.Context, event lifecycleInternalEvent) (resultevents.ResultLifecycleCommandResult, error) {
	if m == nil || m.service == nil || event.Kind != lifecycleInternalEventTerminalDeliveryObserved || event.TerminalDelivery == nil {
		return resultevents.ResultLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	return m.applyTerminalDelivery(ctx, *event.TerminalDelivery)
}
