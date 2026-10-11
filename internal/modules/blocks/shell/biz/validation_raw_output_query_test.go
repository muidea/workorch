package shell

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 实际私有 producer/raw store 配对 typed receipt/Workspace fixture；不冒充公开执行或完整装配。
func TestValidationRawOutputQueryDerivesOriginalReceiptScopeAndBytes(t *testing.T) {
	ctx := context.Background()
	request, execution := projectExecutionFixture(t, false)
	produced, err := NewRuntimeAdapter().executeShellProjectConjunction(ctx, request, execution)
	if err != nil || produced.Conjunction == nil || !produced.Conjunction.Complete {
		t.Fatalf("实际 producer 前置失败：%v", err)
	}
	observation := produced.Result.ValidationObservation.Clone()
	observation.ObservationProtocolRef = shellProjectConjunctionProtocolRef
	observation.ExecutionConjunction = produced.Conjunction
	receipt := runtimecontract.RuntimeOperationReceipt{ExecutionKey: execution.Execution.ExecutionKey, Fence: execution.Execution.Fence, OperationID: execution.OperationID, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectEphemeralSession, Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &observation}}
	if receipt.Validate() != nil {
		t.Fatal("原实际执行 receipt fixture 非法")
	}
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	var queryCalls, workspaceCalls atomic.Int32
	queryResponse := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: true, Receipt: &receipt}
	var queryOverride any
	workspaceResponse := workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: request.WorkspaceID, InternalDirectory: request.WorkspaceInternalDirectory}}
	queryObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	if err := queryObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		queryCalls.Add(1)
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		if event.Source() != ModuleID || command.ReadSource != nil || command.WorkspaceID != execution.WorkspaceID || command.RunID != execution.RunID || command.DispatchID != execution.DispatchID || command.OperationID != execution.OperationID || command.ExecutionKey != execution.Execution.ExecutionKey || command.Fence != execution.Execution.Fence {
			t.Error("未从原身份反查 receipt")
		}
		if queryOverride != nil {
			result.Set(queryOverride, nil)
		} else {
			result.Set(queryResponse, nil)
		}
	}); err != nil {
		t.Fatal(err)
	}
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	if err := workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(event magicevent.Event, result magicevent.Result) {
		workspaceCalls.Add(1)
		if event.Source() != ModuleID || event.Data().(workspaceevents.GetWorkspaceCommand).WorkspaceID != request.WorkspaceID {
			t.Error("未从 Workspace owner 取得原 scope 目录")
		}
		result.Set(workspaceResponse, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		queryObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
	})
	owner := newResourceLedgerOwner(hub)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	command := shellevents.QueryRuntimeValidationRawOutputCommand{WorkspaceID: execution.WorkspaceID, RunID: execution.RunID, DispatchID: execution.DispatchID, OperationID: execution.OperationID, ExecutionKey: receipt.ExecutionKey, Fence: receipt.Fence, SourceReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt)}
	send := func(c shellevents.QueryRuntimeValidationRawOutputCommand) magicevent.Result {
		return hub.Send(magicevent.NewEventWithContext(shellevents.TopicQueryRuntimeValidationRawOutput, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, c))
	}
	store, err := openShellProjectRawStore(request.WorkspaceInternalDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, refs := range produced.Raw {
		for stream, ref := range map[string]string{"stdout": refs.Stdout, "stderr": refs.Stderr} {
			command.StageID, command.Stream = refs.StageID, stream
			raw, err := store.Read(ctx, shellProjectRawScope{WorkspaceID: command.WorkspaceID, RunID: command.RunID, ExecutionKey: command.ExecutionKey, StageID: command.StageID, Stream: command.Stream}, ref)
			if err != nil {
				t.Fatal(err)
			}
			response := send(command)
			if response == nil || response.Error() != nil {
				t.Fatalf("原输出查询失败：%#v", response)
			}
			responseData, resultErr := response.Get()
			if resultErr != nil {
				t.Fatal(resultErr)
			}
			value := responseData.(shellevents.QueryRuntimeValidationRawOutputResult)
			if value.ValidateFor(command) != nil || !reflect.DeepEqual(value.Raw, raw.Raw) || value.Identity.Storage.Reference != ref {
				t.Fatal("受限查询改写原 bytes 或来源")
			}
			value.Identity.Storage.Reference = "caller"
			if len(value.Raw) > 0 {
				value.Raw[0]++
			}
			fresh := send(command)
			if fresh == nil || fresh.Error() != nil {
				t.Fatal("返回副本污染原 receipt/raw")
			}
			freshData, freshErr := fresh.Get()
			if freshErr != nil || !reflect.DeepEqual(freshData.(shellevents.QueryRuntimeValidationRawOutputResult).Raw, raw.Raw) {
				t.Fatal("返回副本污染原 receipt/raw")
			}
		}
	}
	for _, scenario := range []string{"missing", "type", "wrong-hash", "stage", "omitted", "workspace", "directory", "output-identity", "cross-stage-ref", "receipt-key", "receipt-fence", "effect", "accepted-unknown"} {
		t.Run(scenario, func(t *testing.T) {
			c := command
			originalQuery, originalWorkspace := queryResponse, workspaceResponse
			defer func() { queryResponse, workspaceResponse, queryOverride = originalQuery, originalWorkspace, nil }()
			changed := receipt.Clone()
			switch scenario {
			case "missing":
				queryResponse = orchestratorevents.QueryRuntimeOperationReceiptResult{}
			case "type":
				queryOverride = struct{}{}
			case "wrong-hash":
				c.SourceReceiptHash = "different"
			case "stage":
				c.StageID = "unknown"
			case "omitted":
				changed.Evidence.Validation.ExecutionConjunction.Complete = false
				changed.Evidence.Validation.ProofClass = ""
				for index := range changed.Evidence.Validation.ExecutionConjunction.Stages {
					changed.Evidence.Validation.ExecutionConjunction.Stages[index].Stderr.Storage = &runtimecontract.RuntimeValidationOutputStorage{OmittedReason: "not_saved"}
				}
				queryResponse.Receipt, c.SourceReceiptHash, c.Stream = &changed, runtimecontract.StableRuntimeOperationReceiptHash(changed), "stderr"
			case "workspace":
				workspaceResponse.Workspace = &workspaceevents.Workspace{ID: "other", InternalDirectory: request.WorkspaceInternalDirectory}
			case "directory":
				workspaceResponse.Workspace = &workspaceevents.Workspace{ID: request.WorkspaceID, InternalDirectory: t.TempDir()}
			case "cross-stage-ref":
				changed.Evidence.Validation.ExecutionConjunction.Stages[1].Stdout.Storage.Reference = produced.Raw[0].Stdout
				queryResponse.Receipt, c.SourceReceiptHash, c.StageID, c.Stream = &changed, runtimecontract.StableRuntimeOperationReceiptHash(changed), produced.Raw[1].StageID, "stdout"
			case "receipt-key":
				changed.ExecutionKey = runtimecontract.StableRuntimeOperationExecutionKey(command.WorkspaceID, command.RunID, "other", command.OperationID)
				queryResponse.Receipt, c.SourceReceiptHash = &changed, runtimecontract.StableRuntimeOperationReceiptHash(changed)
			case "effect":
				changed.ObservedEffectScope = runtimecontract.RuntimeEffectWorkspaceDurable
				queryResponse.Receipt, c.SourceReceiptHash = &changed, runtimecontract.StableRuntimeOperationReceiptHash(changed)
			case "accepted-unknown":
				changed.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
				queryResponse.Receipt, c.SourceReceiptHash = &changed, runtimecontract.StableRuntimeOperationReceiptHash(changed)
			case "receipt-fence":
				changed.Fence++
				queryResponse.Receipt, c.SourceReceiptHash = &changed, runtimecontract.StableRuntimeOperationReceiptHash(changed)
			case "output-identity":
				for index := range changed.Evidence.Validation.ExecutionConjunction.Stages {
					changed.Evidence.Validation.ExecutionConjunction.Stages[index].Stderr.SHA256 = changed.Evidence.Validation.ExecutionConjunction.RequiredStagesHash
				}
				queryResponse.Receipt, c.SourceReceiptHash, c.Stream = &changed, runtimecontract.StableRuntimeOperationReceiptHash(changed), "stderr"
			}
			if response := send(c); response == nil || response.Error() == nil {
				t.Fatalf("非法原输出查询未拒绝：%s", scenario)
			}
		})
	}
	originalResponse := queryResponse
	rejected := receipt.Clone()
	rejected.Outcome = runtimecontract.RuntimeOperationOutcomeRejected
	rejected.Evidence.Validation.ProofClass = ""
	rejected.Evidence.Validation.ExecutionConjunction.Complete = false
	feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{WorkspaceID: command.WorkspaceID, RunID: command.RunID, DispatchID: command.DispatchID}, runtimecontract.RuntimeFeedbackDomainOperation, runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOperationOutcomeRejected), ModuleID, runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Code: "original_known_failure", Signature: "original", Message: "原始工程失败"})
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = command.OperationID
	rejected.Feedback = &feedback
	for _, effect := range []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession, runtimecontract.RuntimeEffectUnknown} {
		rejected.ObservedEffectScope = effect
		queryResponse.Receipt = &rejected
		rejectedCommand := command
		rejectedCommand.SourceReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(rejected)
		if response := send(rejectedCommand); response == nil || response.Error() != nil {
			t.Fatal("rejected 原回执的已保存输出不能读取")
		}
	}
	queryResponse = originalResponse
	before := queryCalls.Load()
	if err := owner.BeginShutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if response := send(command); response == nil || response.Error() == nil || queryCalls.Load() != before {
		t.Fatal("停机后仍接纳原文新查询")
	}
	if workspaceCalls.Load() == 0 {
		t.Fatal("未命中真实目录读取边界")
	}
	if err := owner.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	reopened := newResourceLedgerOwner(hub)
	if err := reopened.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Teardown(ctx) })
	response := send(command)
	if response == nil || response.Error() != nil {
		t.Fatal("owner 重建后不能从原 receipt 和原 store 读取")
	}
	data, resultErr := response.Get()
	if resultErr != nil || data.(shellevents.QueryRuntimeValidationRawOutputResult).ValidateFor(command) != nil {
		t.Fatal("owner 重建后丢失原 raw 身份")
	}
}

func TestValidationRawOutputQueryQuiescenceWaitsForNestedReceiptRead(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	if err := observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
		close(entered)
		<-release
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	command := shellevents.QueryRuntimeValidationRawOutputCommand{WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "op", ExecutionKey: runtimecontract.StableRuntimeOperationExecutionKey("ws", "run", "dispatch", "op"), Fence: 1, SourceReceiptHash: "original", StageID: "project-test", Stream: "stdout"}
	go func() {
		defer close(done)
		hub.Send(magicevent.NewEventWithContext(shellevents.TopicQueryRuntimeValidationRawOutput, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command))
	}()
	<-entered
	owner.BeginShutdown(ctx)
	budget, cancel := context.WithTimeout(ctx, 15*time.Millisecond)
	defer cancel()
	if err := owner.Quiesce(budget); !errors.Is(err, context.DeadlineExceeded) || owner.runtimeAdapter == nil {
		t.Fatal("nested receipt 未结束却释放 owner")
	}
	close(release)
	<-done
	if err := owner.Quiesce(ctx); err != nil {
		t.Fatal(err)
	}
}
