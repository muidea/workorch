package orchestrator

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeValidationOutputQueryRejectsInvalidIngressBeforeOwnerIO(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	owner := newTestBiz(newRuntimeTestOwner(), hub, nil)
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleShell, hub)
	calls := 0
	if err := observer.Subscribe(shellevents.TopicQueryRuntimeValidationRawOutput, func(_ magicevent.Event, result magicevent.Result) {
		calls++
		result.Set(nil, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { observer.Unsubscribe(shellevents.TopicQueryRuntimeValidationRawOutput) })
	command := orchestratorevents.QueryRuntimeValidationOutputCommand{
		WorkspaceID: "ws-output", RunID: "run-output", DispatchID: "dispatch-output", OperationID: "operation-output",
		Fence: 1, SourceReceiptHash: "receipt-hash", StageID: "test", Stream: "stdout",
	}
	command.ExecutionKey = runtimecontract.StableRuntimeOperationExecutionKey(command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID)
	conflict, err := common.BindRuntimeWorkspaceContext(context.Background(), "other-workspace")
	if err != nil {
		t.Fatal(err)
	}
	wrongStream := command
	wrongStream.Stream = "combined"
	wrongExecution := command
	wrongExecution.DispatchID = "other-dispatch"
	for _, test := range []struct {
		name string
		ctx  context.Context
		data any
	}{
		{"wrong DTO", context.Background(), shellevents.QueryRuntimeValidationRawOutputCommand{}},
		{"workspace conflict", conflict, command},
		{"unsupported stream", context.Background(), wrongStream},
		{"execution scope drift", context.Background(), wrongExecution},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorQueryRuntimeValidationOutput, orchestratorevents.RuntimeValidationOutputReadSource, orchestratorevents.CommandDestination, magicevent.NewHeader(), test.ctx, test.data)
			result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
			owner.handleRuntimeValidationOutput(event, result)
			if result.Error() == nil || calls != 0 {
				t.Fatal("invalid ingress reached capability owner")
			}
		})
	}
	for _, source := range []string{"", "workorchd", runtimecontract.RuntimeModuleFile, runtimecontract.RuntimeModuleShell, ModuleID} {
		event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorQueryRuntimeValidationOutput, source, orchestratorevents.CommandDestination, magicevent.NewHeader(), context.Background(), command)
		result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
		owner.handleRuntimeValidationOutput(event, result)
		if result.Error() == nil || calls != 0 {
			t.Fatalf("untrusted source %q reached restricted raw-output owner", source)
		}
	}
	// Post must neither dereference a missing result nor perform a read.
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorQueryRuntimeValidationOutput, orchestratorevents.RuntimeValidationOutputReadSource, orchestratorevents.CommandDestination, magicevent.NewHeader(), context.Background(), command)
	owner.handleRuntimeValidationOutput(event, nil)
	result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
	owner.handleRuntimeValidationOutput(nil, result)
	if result.Error() == nil || calls != 0 {
		t.Fatal("nil ingress performed owner I/O")
	}
}
