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

	operation, data, sealed := testSSHRuntimeOperation(t, adapter, "mutate-state")
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
		ResolutionID: "resolution-1", ResolutionHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ApprovalID: "approval-1", ExecutionKey: first.Execution.ExecutionKey,
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
	captureBeforeReplay := readFile(t, captureFile)
	duplicateResult := hub.Send(magicevent.NewEventWithContext(sshevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, second))
	duplicateData, err := duplicateResult.Get()
	if err != nil {
		t.Fatal(err)
	}
	duplicateReceipt := duplicateData.(sshevents.ExecuteOperationResult).Receipt
	if !runtimecontract.SameRuntimeOperationReceipt(receipt, duplicateReceipt) || readFile(t, captureFile) != captureBeforeReplay {
		t.Fatalf("duplicate SSH execution crossed remote-effect fence: first=%+v duplicate=%+v", receipt, duplicateReceipt)
	}
}

func TestSSHOwnerCommitsFailedCommandWithObservedEffectClassification(t *testing.T) {
	for _, tc := range []struct {
		name      string
		command   string
		changed   bool
		wantScope runtimecontract.RuntimeEffectScope
		resources int
	}{
		{name: "remote changed before command failure", command: "fail-after-change", changed: true, wantScope: runtimecontract.RuntimeEffectRemoteDurable, resources: 1},
		{name: "no remote change before command failure", command: "fail-without-change", changed: false, wantScope: runtimecontract.RuntimeEffectNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			toolsDir := t.TempDir()
			stateFile := filepath.Join(toolsDir, "remote-state")
			writeExecutable(t, filepath.Join(toolsDir, "setsid"), "#!/bin/sh\nexec \"$@\"\n")
			writeExecutable(t, filepath.Join(toolsDir, "ssh"), "#!/bin/sh\nlast=\"\"\nfor arg in \"$@\"; do last=\"$arg\"; done\nif [ \"$last\" = \"observe-state\" ]; then if [ -f \""+stateFile+"\" ]; then echo after; else echo before; fi; exit 0; fi\nif [ \"$last\" = \"fail-after-change\" ]; then touch \""+stateFile+"\"; echo changed-but-failed; exit 2; fi\nif [ \"$last\" = \"fail-without-change\" ]; then echo unchanged-failure >&2; exit 2; fi\nexit 3\n")
			t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			adapter := configuredPasswordRuntimeAdapter(t, "WORKORCH_TEST_SSH_OWNER_FAILURE_PASSWORD", "test-password")
			owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: adapter}
			if err := owner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { owner.Teardown(ctx) })
			installSSHReceiptObserver(t, hub)

			operation, data, sealed := testSSHRuntimeOperation(t, adapter, tc.command)
			first := testSSHExecuteCommand(t, operation, sealed, nil, 1)
			firstResult := hub.Send(magicevent.NewEventWithContext(sshevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, first))
			firstData, err := firstResult.Get()
			if err != nil {
				t.Fatal(err)
			}
			if receipt := firstData.(sshevents.ExecuteOperationResult).Receipt; receipt.Outcome != runtimecontract.RuntimeOperationOutcomeGovernanceRequired {
				t.Fatalf("pre-grant receipt = %+v", receipt)
			}

			requirement := sshApprovalRequirement(data)
			grant := runtimecontract.RuntimeOperationGovernanceGrant{
				ResolutionID: "resolution-failure", ResolutionHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ApprovalID: "approval-failure", ExecutionKey: first.Execution.ExecutionKey,
				GovernanceFence: 1, ResumeFence: 2, DependencyKind: string(runtimecontract.GovernanceApproval),
				SubjectRef: requirement.SubjectRef, RequestedAction: requirement.RequestedAction, RiskLevel: requirement.RiskLevel,
				AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(requirement.AuthorizationContext), GovernanceReceiptHash: "governance-receipt-failure",
			}
			second := testSSHExecuteCommand(t, operation, sealed, &grant, 2)
			secondResult := hub.Send(magicevent.NewEventWithContext(sshevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, second))
			secondData, err := secondResult.Get()
			if err != nil {
				t.Fatal(err)
			}
			receipt := secondData.(sshevents.ExecuteOperationResult).Receipt
			if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.Changed != tc.changed || receipt.ObservedEffectScope != tc.wantScope || len(receipt.ResourceChanges) != tc.resources || receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil || receipt.Feedback.OriginalDiagnostic.OriginalCode != "ssh_command_failed" {
				t.Fatalf("failed SSH owner receipt = %+v, want changed=%t scope=%q resources=%d", receipt, tc.changed, tc.wantScope, tc.resources)
			}
			if err := receipt.Validate(); err != nil {
				t.Fatalf("failed SSH owner receipt validation: %v", err)
			}
		})
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

	operation, data, sealed := testSSHRuntimeOperation(t, adapter, "mutate-state")
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

	tamperedExecution := command
	tamperedExecution.Execution.AcquisitionBinding = "another-capability-state"
	tamperedExecutionResult := hub.Send(magicevent.NewEventWithContext(sshevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, tamperedExecution))
	if _, err := tamperedExecutionResult.Get(); err == nil {
		t.Fatal("SSH execution bound to another owner catalog was accepted")
	}
	assertSSHNotCalled(t, marker)

	adapter.revision = "ssh-policy-restarted"
	staleResult := hub.Send(magicevent.NewEventWithContext(sshevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command))
	staleData, err := staleResult.Get()
	if err != nil {
		t.Fatal(err)
	}
	staleReceipt := staleData.(sshevents.ExecuteOperationResult).Receipt
	if staleReceipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || staleReceipt.PrerequisiteStale == nil ||
		staleReceipt.PrerequisiteStale.Domain != runtimecontract.RuntimeResourceOwnerCatalog ||
		staleReceipt.PrerequisiteStale.ExpectedRevision != sealed.Authorization.OwnerRevision ||
		staleReceipt.PrerequisiteStale.ObservedRevision != adapter.revision {
		t.Fatalf("SSH restart/config drift did not return typed stale receipt: %+v", staleReceipt)
	}
	assertSSHNotCalled(t, marker)
	adapter.revision = sealed.Authorization.OwnerRevision

	requirement := sshApprovalRequirement(data)
	grant := runtimecontract.RuntimeOperationGovernanceGrant{
		ResolutionID: "resolution-1", ResolutionHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ApprovalID: "approval-1", ExecutionKey: command.Execution.ExecutionKey,
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
	type receiptKey struct {
		executionKey string
		fence        uint64
	}
	receipts := map[receiptKey]runtimecontract.RuntimeOperationReceipt{}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found := receipts[receiptKey{executionKey: command.ExecutionKey, fence: command.Fence}]
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			response.Receipt = &receipt
		}
		result.Set(response, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		receipts[receiptKey{executionKey: command.Receipt.ExecutionKey, fence: command.Fence}] = command.Receipt
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
}

func testSSHRuntimeOperation(t *testing.T, adapter *RuntimeAdapter, command string) (runtimecontract.RuntimeOperation, sshevents.OperationData, sshevents.SealedRuntimeOperation) {
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
	constraintRefs := append([]string{"owner-execution-policy"}, adapter.bindingConstraintRefs(runtimecontract.RuntimeOwnerInventoryItemsMaxCount)...)
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility("ssh-admissibility", ModuleID, []string{operation.Kind}, adapter.targetRefs(runtimecontract.RuntimeOwnerInventoryItemsMaxCount), nil, constraintRefs, adapter.admissibilityRevision())
	if err != nil {
		t.Fatal(err)
	}
	catalog.OperationPolicies = []runtimecontract.RuntimeOperationPolicyFact{{
		ID: "operation-policy/ssh-command", OwnerModuleID: ModuleID, OperationKind: runtimecontract.StepKindSSHCommand,
		SubjectRef: "configured-ssh-target", EffectScope: runtimecontract.RuntimeEffectRemoteDurable,
		IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	observation := sshAdmissibilityObservation(catalog.PrerequisiteID, catalog)
	sealed, err := sshevents.SealRuntimeOperation(operation, catalog, observation)
	if err != nil {
		t.Fatal(err)
	}
	return operation, data, sealed
}

func testSSHExecuteCommand(t *testing.T, operation runtimecontract.RuntimeOperation, sealed sshevents.SealedRuntimeOperation, grant *runtimecontract.RuntimeOperationGovernanceGrant, fence uint64) sshevents.ExecuteOperationCommand {
	t.Helper()
	dispatchID := "dispatch-1"
	executionKey := runtimecontract.StableRuntimeOperationExecutionKey("ws-1", "run-1", dispatchID, operation.ID)
	if grant != nil {
		dispatchID = "dispatch-2"
		executionKey = grant.ExecutionKey
		grant.ResumeFence = fence
		grant.ResumeDispatchID = dispatchID
	}
	execution := runtimecontract.RuntimeOperationExecutionEnvelope{
		ExecutionKey: executionKey, Fence: fence, ExpectedEffectScope: runtimecontract.RuntimeEffectRemoteDurable,
		AdmissibilitySnapshotID: sealed.Authorization.CatalogSnapshotID, AcquisitionBinding: sealed.Authorization.AcquisitionBinding,
		GovernanceGrant: grant,
	}
	execution.OperationHash = runtimecontract.StableRuntimeOperationHash(operation)
	execution.IdempotencyClass = runtimecontract.ResolveOperationIdempotencyClass(operation, []runtimecontract.RuntimeOperationPolicyFact{sealed.Authorization.Policy})
	execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: operation.ID, OperationHash: execution.OperationHash, ExecutionKey: execution.ExecutionKey, ExpectedEffectScope: execution.ExpectedEffectScope,
		IdempotencyClass: execution.IdempotencyClass, AdmissibilitySnapshotID: execution.AdmissibilitySnapshotID,
		AcquisitionBinding: execution.AcquisitionBinding, ReplayProof: execution.ReplayProof,
	})
	return sshevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution:   execution,
			WorkspaceID: "ws-1", RunID: "run-1", DispatchID: dispatchID, Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
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
