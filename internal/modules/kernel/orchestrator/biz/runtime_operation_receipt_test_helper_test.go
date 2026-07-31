package orchestrator

// Owner Biz receipt tests.

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	shellmodule "github.com/muidea/workorch/internal/modules/blocks/shell"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
)

func startRuntimeV2ReceiptTestOwner(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	biz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, module.handleRuntimeV2OperationReceipt)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, module.handleRuntimeV2OperationReceipt)
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})
}

// startRuntimeV2ManagedValidationTestOwner uses the actual Shell Block for
// current-schema fixtures.  Tests may script the provider, but a validation
// receipt must still be an owner fact for the exact sealed recipe; File read
// operations are not a substitute for managed-validation coverage.
func startRuntimeV2ManagedValidationTestOwner(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	module := shellmodule.New()
	if err := module.Configure(shellmodule.Dependencies{EventHub: hub, RuntimeAdapter: shellbiz.NewRuntimeAdapter()}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(context.Background()) })
}
