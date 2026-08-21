package ssh

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSSHOwnerRequiresExactApprovalBeforeNetworkAndCommitsObservedChange(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })

	toolsDir := t.TempDir()
	captureFile := filepath.Join(toolsDir, "capture.txt")
	stateFile := filepath.Join(toolsDir, "remote-state")
	writeExecutable(t, filepath.Join(toolsDir, "setsid"), "#!/bin/sh\nexec \"$@\"\n")
	writeExecutable(t, filepath.Join(toolsDir, "ssh"), fakeObservedSSHScript(captureFile, stateFile, true))
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	adapter := configuredPasswordRuntimeAdapter(t, "WORKORCH_TEST_SSH_PASSWORD", "test-password")
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: adapter}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	installSSHReceiptObserver(t, hub)

	operation, data, sealed := testSSHRuntimeOperation(t, "mutate-state")
	first := testSSHExecuteCommand(t, operation, sealed, nil, 1)
	firstResult := hub.Send(magicevent.NewEventWithContext(sshevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, first))
	firstData, err := firstResult.Get()
	if err != nil {
		t.Fatal(err)
	}
	governanceReceipt := firstData.(sshevents.ExecuteOperationResult).Receipt
	if governanceReceipt.Outcome != runtimecontract.RuntimeOperationOutcomeGovernanceRequired || governanceReceipt.Governance == nil || governanceReceipt.Changed {
		t.Fatalf("receipt = %+v, want approval requirement with no effect", governanceReceipt)
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("SSH process ran before approval: stat err=%v", err)
	}

	requirement := sshApprovalRequirement(data)
	grant := runtimecontract.RuntimeOperationGovernanceGrant{
		ResolutionID: "resolution-1", ApprovalID: "approval-1", ExecutionKey: first.Execution.ExecutionKey,
		GovernanceFence: 1, ResumeFence: 2, DependencyKind: string(runtimecontract.GovernanceApproval),
		SubjectRef: requirement.SubjectRef, RequestedAction: requirement.RequestedAction, RiskLevel: requirement.RiskLevel,
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(requirement.AuthorizationContext), GovernanceReceiptHash: "governance-receipt-1",
	}
	second := testSSHExecuteCommand(t, operation, sealed, &grant, 2)
	secondResult := hub.Send(magicevent.NewEventWithContext(sshevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, second))
	secondData, err := secondResult.Get()
	if err != nil {
		t.Fatal(err)
	}
	receipt := secondData.(sshevents.ExecuteOperationResult).Receipt
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.ObservedEffectScope != runtimecontract.RuntimeEffectRemoteDurable ||
		!receipt.Changed || receipt.BeforeHash == receipt.AfterHash || len(receipt.ResourceChanges) != 1 || receipt.Evidence.ContentHash == "" {
		t.Fatalf("receipt = %+v, want factual remote change", receipt)
	}
	if err := receipt.Validate(); err != nil {
		t.Fatalf("receipt validation: %v", err)
	}
}

func TestSSHOwnerFailsClosedForPostWrongSourceSealedDriftAndGrantDrift(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })

	toolsDir := t.TempDir()
	marker := filepath.Join(toolsDir, "ssh-called")
	writeExecutable(t, filepath.Join(toolsDir, "setsid"), "#!/bin/sh\nexec \"$@\"\n")
	writeExecutable(t, filepath.Join(toolsDir, "ssh"), "#!/bin/sh\ntouch \""+marker+"\"\n")
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	adapter := configuredPasswordRuntimeAdapter(t, "WORKORCH_TEST_SSH_PASSWORD", "test-password")
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: adapter}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	installSSHReceiptObserver(t, hub)

	operation, data, sealed := testSSHRuntimeOperation(t, "mutate-state")
	command := testSSHExecuteCommand(t, operation, sealed, nil, 1)
	event := magicevent.NewEventWithContext(sshevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command)
	owner.handleSSHOperation(event, nil)
	assertSSHNotCalled(t, marker)

	wrongSource := hub.Send(magicevent.NewEventWithContext(sshevents.TopicExecuteOperation, runtimecontract.RuntimeModuleFile, ModuleID, magicevent.NewHeader(), ctx, command))
	if _, err := wrongSource.Get(); err == nil {
		t.Fatal("wrong-source SSH execution was accepted")
	}
	assertSSHNotCalled(t, marker)

	drifted := command
	drifted.SealedOperation.PayloadHash = "drifted"
	driftedResult := hub.Send(magicevent.NewEventWithContext(sshevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, drifted))
	if _, err := driftedResult.Get(); err == nil {
		t.Fatal("drifted sealed SSH operation was accepted")
	}
	assertSSHNotCalled(t, marker)

	requirement := sshApprovalRequirement(data)
	grant := runtimecontract.RuntimeOperationGovernanceGrant{
		ResolutionID: "resolution-1", ApprovalID: "approval-1", ExecutionKey: command.Execution.ExecutionKey,
		GovernanceFence: 1, ResumeFence: 2, DependencyKind: string(runtimecontract.GovernanceApproval),
		SubjectRef: requirement.SubjectRef, RequestedAction: requirement.RequestedAction, RiskLevel: requirement.RiskLevel,
		AuthorizationHash:     runtimecontract.StableRuntimeAuthorizationContextHash(workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "other-target"))),
		GovernanceReceiptHash: "governance-receipt-1",
	}
	withWrongGrant := testSSHExecuteCommand(t, operation, sealed, &grant, 2)
	wrongGrantResult := hub.Send(magicevent.NewEventWithContext(sshevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, withWrongGrant))
	if _, err := wrongGrantResult.Get(); err == nil {
		t.Fatal("drifted SSH approval grant was accepted")
	}
	assertSSHNotCalled(t, marker)
}

func installSSHReceiptObserver(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{}, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
}

func testSSHRuntimeOperation(t *testing.T, command string) (runtimecontract.RuntimeOperation, sshevents.OperationData, sshevents.SealedRuntimeOperation) {
	t.Helper()
	sessionRef, credentialRef := "session-a", "credential-a"
	operation := runtimecontract.RuntimeOperation{
		ID: "ssh-1", Kind: runtimecontract.StepKindSSHCommand, TargetRefs: []string{"target-a"},
		Parameters: []runtimecontract.OperationParameter{{Name: "sessionRef", String: &sessionRef}, {Name: "credentialRef", String: &credentialRef}, {Name: "command", String: &command}},
	}
	data, err := sshevents.OperationDataFromRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := sshevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	return operation, data, sealed
}

func testSSHExecuteCommand(t *testing.T, operation runtimecontract.RuntimeOperation, sealed sshevents.SealedRuntimeOperation, grant *runtimecontract.RuntimeOperationGovernanceGrant, fence uint64) sshevents.ExecuteOperationCommand {
	t.Helper()
	executionKey := "exec/ws-1/run-1/dispatch-1/ssh-1"
	if grant != nil {
		grant.ExecutionKey = executionKey
		grant.ResumeFence = fence
	}
	return sshevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution:   runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: executionKey, Fence: fence, ExpectedEffectScope: runtimecontract.RuntimeEffectRemoteDurable, GovernanceGrant: grant},
			WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
			OperationID: operation.ID, TargetRefs: append([]string(nil), operation.TargetRefs...), EvidenceKind: runtimecontract.EvidenceKindStateChange, WorkspaceDeliverableDirectory: t.TempDir(),
		},
		SealedOperation: sealed,
	}
}

func assertSSHNotCalled(t *testing.T, marker string) {
	t.Helper()
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("SSH process was called: stat err=%v", err)
	}
}
