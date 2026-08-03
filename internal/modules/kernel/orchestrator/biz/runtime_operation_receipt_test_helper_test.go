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

func startRuntimeReceiptTestOwner(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	biz := newRuntimeTestOwner()
	if err := biz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, module.handleRuntimeOperationReceipt)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, module.handleRuntimeOperationReceipt)
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})
}

// startRuntimeManagedValidationTestOwner uses the actual Shell Block for
// current-schema fixtures.  Tests may script the provider, but a validation
// receipt must still be an owner fact for the exact sealed recipe; File read
// operations are not a substitute for managed-validation coverage.
func startRuntimeManagedValidationTestOwner(t *testing.T, hub magicevent.Hub) {
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
