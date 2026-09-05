package shell

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func shellRawAuthorityPolicyForOwnerBoundaryTest() runtimecontract.RuntimeEvidencePolicy {
	return runtimecontract.RuntimeEvidencePolicy{
		ID: "shell-owner-raw-authority", AppliesTo: runtimecontract.StepKindShellCommand,
		CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{
			ID: "shell-owner-proof", Scope: "dodItem",
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			ValidationRecipeID:    "shell-owner-proof-recipe",
			RequiredCheckRefs:     []string{"shell-owner-proof-check"},
		}},
	}.Normalized()
}

func duplicateShellOwnerRawAuthorityCheckForTest(policies []runtimecontract.RuntimeEvidencePolicy) []runtimecontract.RuntimeEvidencePolicy {
	policies = runtimecontract.CloneRuntimeEvidencePolicies(policies)
	requirement := &policies[0].CoverageRequirements[0]
	requirement.RequiredCheckRefs = append(requirement.RequiredCheckRefs, requirement.RequiredCheckRefs[0])
	return policies
}

func shellRawAuthorityExecutionCommandForTest(t *testing.T, adapter *RuntimeAdapter, workspace string) shellevents.ExecuteOperationCommand {
	t.Helper()
	context := runtimecontract.RuntimeOperationExecutionContext{
		Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
			ExecutionKey:        "exec/ws-shell-raw/run-shell-raw/dispatch-shell-raw/shell-write-raw",
			Fence:               1,
			ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		},
		WorkspaceID: "ws-shell-raw", RunID: "run-shell-raw", DispatchID: "dispatch-shell-raw",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-shell-raw", RequiredOutputHash: "output-shell-raw"},
		OperationID: "shell-write-raw", EvidenceKind: runtimecontract.EvidenceKindStateChange,
		WorkspaceDeliverableDirectory: workspace, TargetRefs: []string{"generated.txt"},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{shellRawAuthorityPolicyForOwnerBoundaryTest()},
	}
	return sealedShellTestCommand(t, adapter, context, runtimecontract.StepKindShellCommand, shellevents.OperationData{
		Command: "sh", Args: []string{"-c", "printf executed > generated.txt"}, StateChanging: true,
		TargetRef: "generated.txt", TargetRefs: []string{"generated.txt"},
	}, "", nil, nil)
}

func TestShellOwnerRejectsRawEvidenceAuthorityBeforeReceiptOrExecution(t *testing.T) {
	for name, mutate := range map[string]func(*shellevents.ExecuteOperationCommand){
		"shared execution context": func(command *shellevents.ExecuteOperationCommand) {
			command.EvidencePolicies = duplicateShellOwnerRawAuthorityCheckForTest(command.EvidencePolicies)
		},
		"Shell authorization": func(command *shellevents.ExecuteOperationCommand) {
			command.SealedOperation.Authorization.EvidencePolicies = duplicateShellOwnerRawAuthorityCheckForTest(command.SealedOperation.Authorization.EvidencePolicies)
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			var receiptCalls atomic.Int32
			receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
			receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
				receiptCalls.Add(1)
				result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{}, nil)
			})
			receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
				receiptCalls.Add(1)
				command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
				result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
			})
			t.Cleanup(func() {
				receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
				receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
			})

			adapter := NewRuntimeAdapter()
			owner := &Biz{Base: basebiz.New(modulecommon.ModuleID, hub, nil), runtimeAdapter: adapter}
			if err := owner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { owner.Teardown(ctx) })

			workspace := t.TempDir()
			command := shellRawAuthorityExecutionCommandForTest(t, adapter, workspace)
			mutate(&command)
			result := hub.Send(magicevent.NewEventWithContext(
				shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
				magicevent.NewHeader(), ctx, command,
			))
			if result == nil || result.Error() == nil || !strings.Contains(result.Error().Message, "evidence polic") {
				t.Fatalf("Shell owner accepted noncanonical evidence authority: %#v", result)
			}
			if got := receiptCalls.Load(); got != 0 {
				t.Fatalf("noncanonical evidence authority reached receipt boundary: calls=%d", got)
			}
			if _, err := os.Stat(filepath.Join(workspace, "generated.txt")); !os.IsNotExist(err) {
				t.Fatalf("noncanonical evidence authority reached Shell executor: %v", err)
			}
		})
	}
}
