package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// commitRuntimeOperationReceiptForTest models a capability-owner Biz issuing
// the typed receipt command. Production owners keep this EventHub I/O in their
// respective biz packages.
func commitRuntimeOperationReceiptForTest(ctx context.Context, hub magicevent.Hub, source string, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, execution runtimecontract.RuntimeOperationExecutionEnvelope, receipt runtimecontract.RuntimeOperationReceipt) (runtimecontract.RuntimeOperationReceipt, error) {
	if hub == nil {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("missing event hub")
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, source, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.CommitRuntimeOperationReceiptCommand{
		WorkspaceID: workspaceID, RunID: runID, DispatchID: dispatchID, Fence: execution.Fence, Receipt: receipt,
	})
	event.BindLaneKey(eventcontract.StateLaneKey("orchestrator", eventcontract.LaneScope{
		WorkspaceID: string(workspaceID), RunID: string(runID), SubjectID: execution.ExecutionKey,
	}))
	result := hub.Send(event)
	if result == nil {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("runtime receipt commit returned no result")
	}
	data, err := result.Get()
	if err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, err
	}
	response, ok := data.(orchestratorevents.CommitRuntimeOperationReceiptResult)
	if !ok {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("runtime receipt commit returned invalid result type %T", data)
	}
	return response.Receipt.Clone(), nil
}
