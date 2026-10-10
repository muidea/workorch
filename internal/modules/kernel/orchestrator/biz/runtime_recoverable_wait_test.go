package orchestrator

import (
	"context"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestProviderCeilingTransientOpensPersistedWaitAndResumeCannotProbe(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })

	aggregate := runtimeAggregate(t, 1)
	aggregate.Facts.Resources.ProviderCeiling = nil
	aggregate.Facts = runtime.PreflightRuntimeFacts(aggregate.Facts, []runtimecontract.RuntimeRequirement{{
		Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		MinimumBytes: 64 * 1024, Reason: "provider ceiling required",
	}})
	decision, err := runtime.ReduceRuntimeState(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	repository := &runtimeDriveRepository{aggregate: aggregate}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: aggregate.Facts.WorkspaceID, DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir(),
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	providerCalls := 0
	providerObserver := magicevent.NewSimpleObserverWithMatchID(runtimecontract.RuntimeModuleLLM+"/recoverable-wait-test", orchestratorevents.PrerequisiteCommandDestination, hub)
	providerObserver.Subscribe(llmevents.TopicAcquireProviderCeiling, func(_ magicevent.Event, result magicevent.Result) {
		providerCalls++
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "provider ceiling timed out", Cause: context.DeadlineExceeded})
	})
	t.Cleanup(func() { providerObserver.Unsubscribe(llmevents.TopicAcquireProviderCeiling) })

	waiting, err := module.prepareRuntimePrerequisites(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || len(waiting.RecoverableWaits) != 1 || !waiting.RecoverableWaits[0].Active() || waiting.StateRevision != aggregate.StateRevision+1 {
		t.Fatalf("waiting aggregate=%+v calls=%d", waiting.RecoverableWaits, providerCalls)
	}
	allocation, _ := runtimecontract.RuntimeBudgetAllocationForID(waiting.ExecutionBudget.Allocations, waiting.RecoverableWaits[0].BudgetAllocationID)
	if allocation.State != runtimecontract.RuntimeBudgetAllocationReserved {
		t.Fatalf("wait allocation=%+v", allocation)
	}

	for _, command := range []orchestratorevents.DriveRuntimeCommand{
		{WorkspaceID: waiting.Facts.WorkspaceID, RunID: waiting.Facts.RunID},
		{WorkspaceID: waiting.Facts.WorkspaceID, RunID: waiting.Facts.RunID, RecoveryLeaseGeneration: 999},
	} {
		outcome, driveErr := module.driveOneRuntimeTransition(ctx, nil, repository.aggregate, command, "unused-prefix")
		if driveErr != nil || outcome.result == nil || outcome.result.RecoverableWait == nil || outcome.result.Terminal != nil {
			t.Fatalf("resume outcome=%+v err=%v", outcome, driveErr)
		}
	}
	if providerCalls != 1 {
		t.Fatalf("ordinary/forged Resume executed %d extra probes", providerCalls-1)
	}

	view, err := runtimecontract.RuntimeRecoverableWaitProjection(waiting.RecoverableWaits[0], waiting.StateRevision)
	if err != nil {
		t.Fatal(err)
	}
	retry, handled := module.handleScheduledRuntimeDriveFailure(ctx, scheduledRuntimeDriveRequest{workspaceID: waiting.Facts.WorkspaceID, runID: waiting.Facts.RunID, source: "test"}, &runtimeRecoverableWaitPendingError{Wait: view})
	if !handled || retry == nil || retry.Reason != runtimecontract.RuntimeDriveLeaseReasonRecoverableWait || retry.FailureSignature != "recoverable-wait/"+view.WaitID || retry.Delay <= 0 || retry.Delay > time.Minute {
		t.Fatalf("recoverable wait handled=%v retry=%+v", handled, retry)
	}
}
