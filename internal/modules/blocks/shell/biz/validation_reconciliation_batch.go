package shell

import (
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
)

func (m *Biz) handleRuntimeCWIReconciliationBatch(event magicevent.Event, result magicevent.Result) {
	c, ok := shellRuntimeIngressCommand[shellevents.QueryRuntimeCWIReconciliationBatchCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil || c.ValidateForOwner(ModuleID) != nil {
		setCapabilityError(result, fmt.Errorf("invalid Shell reconciliation batch"))
		return
	}
	response := shellevents.QueryRuntimeCWIReconciliationBatchResult{}
	for _, member := range c.Members {
		o, err := m.queryRuntimeCWIReconciliation(event.Context(), shellevents.QueryRuntimeCWIReconciliationCommand(member))
		if err != nil {
			setCapabilityError(result, err)
			return
		}
		response.Observations = append(response.Observations, o)
	}
	if err := c.ValidateResult(response); err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}
