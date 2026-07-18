package orchestrator

import (
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
)

func startRuntimeV2ReceiptTestOwner(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	module := &Module{bizPtr: biz, rawEventHub: hub}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, module.handleRuntimeV2OperationReceipt)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, module.handleRuntimeV2OperationReceipt)
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})
}
