package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// File、executor 和 receipt handler 为实际实现；Workspace、CAS repository 与辅助读取 decision 是显式 fixture。
// 原操作经过完整 semantic/candidate/claim/receipt/evidence 门禁，不将只读诊断提升为 validation proof。
func TestRuntimeFileReadDependencyQueryPairsActualReceiptAuthority(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	directory := t.TempDir()
	target := "source.txt"
	raw := []byte("first\r\n原文\r\nlast\n")
	write := func(path string, content []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(directory, path), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(target, raw)
	workspace := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	if err := workspace.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(workspaceevents.GetWorkspaceCommand)
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: command.WorkspaceID, DeliverableDirectory: directory}}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := workspace.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet); err != nil {
			t.Error(err)
		}
	})
	file, err := filebiz.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Teardown(ctx); err != nil {
			t.Error(err)
		}
	})

	aggregate := runtimeAggregate(t, 1)
	aggregate.Facts = runtimeCandidateSealExactFileEditFacts(t, target)
	aggregate.Definition.CapabilityContract = []string{runtimecontract.StepKindFileEdit, runtimecontract.StepKindFileRead}
	aggregate.Facts.Capabilities.ValidationKinds = []string{runtimecontract.StepKindFileRead}
	catalog := aggregate.Facts.Resources.OperationAdmissibilities[0]
	catalog.OperationKinds = append(catalog.OperationKinds, runtimecontract.StepKindFileRead)
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	aggregate.Facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{runtimeOperationAdmissibilityPrerequisite(catalog)}
	aggregate.Facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{runtimeFileAdmissibilityObservation(catalog)}
	digest := sha256.Sum256(raw)
	aggregate.Facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{TargetRef: target, Content: string(raw), ContentHash: hex.EncodeToString(digest[:]), ContentState: runtimecontract.SnapshotContentStateComplete}}
	repository := &runtimeReceiptSourceGuardRepository{aggregate: aggregate}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	owner := newTestBiz(newRuntimeTestOwner(), hub, nil)
	owner.runtimeController = controller
	receipts := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	for _, topic := range []string{orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt} {
		if err := receipts.Subscribe(topic, owner.handleRuntimeOperationReceipt); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := receipts.Unsubscribe(topic); err != nil {
				t.Error(err)
			}
		})
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, aggregate)
	seal := func(candidate runtimecontract.RuntimeCandidate, prefix string) {
		t.Helper()
		if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, prefix); err != nil {
			t.Fatal(err)
		}
		proposal, err := admitRuntimeCandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
		if err != nil || !proposal.Admission.Accepted() {
			t.Fatalf("完整候选准入: %+v failure=%+v err=%v", proposal.Admission, proposal.Admission.Failure, err)
		}
		pending := proposal.Aggregate.CandidateAttempts[len(proposal.Aggregate.CandidateAttempts)-1].Proposal
		operationID := pending.Validation.Operations[0].Operation.ID
		if _, err := runtimeTestControllerRecordCandidateFeasibility(controller, ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
			OperationID: operationID, AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding,
			CandidateManifestHash: runtimePendingCandidateManifestHashForBizTest(t, ctx, controller, "ws-v2", "run-v2", 0),
			Observation:           &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
		}); err != nil {
			t.Fatal(err)
		}
		sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
		if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil {
			t.Fatalf("完整封存: %+v err=%v", sealed.Admission, err)
		}
	}
	// 此组合只验证原 receipt 来源与 current query，不伪造 production reducer 的下一行动选择。
	// 与 capability receipt matrix 一样，显式封存非授信 auxiliary decision；每次 CAS 仍执行完整 aggregate Validate。
	aggregate.Decision, err = runtimecontract.SealRuntimeDecision(aggregate.Facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedValidation, Focus: aggregate.Facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:         []string{runtimecontract.StepKindFileRead},
			AcceptedEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			CoverageWorkItemIDs:   append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...), TargetRefs: []string{target},
		},
	}, aggregate.Decision.AuthoritySnapshot.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	repository.aggregate = runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(aggregate))
	start, end, bound := int64(2), int64(2), int64(128)
	seal(runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "current-state-read", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{{Name: "startLine", Integer: &start}, {Name: "endLine", Integer: &end}, {Name: "maxBytes", Integer: &bound}},
	}}}}}, "actual-read")
	var request runtime.RuntimeOperationExecutionRequest
	aggregate, err = runtimeTestControllerExecuteDispatch(controller, ctx, "ws-v2", "run-v2", runtimeOperationExecutorFunc(func(callCtx context.Context, value runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
		request = value
		return (eventRuntimeOperationExecutor{hub: hub}).ExecuteRuntimeOperation(callCtx, value)
	}))
	if err != nil || len(aggregate.OperationReceipts) != 1 {
		t.Fatalf("实际 read/receipt/evidence transaction: receipts=%d err=%v", len(aggregate.OperationReceipts), err)
	}
	if len(request.Operation.BindingRefs) != 0 || len(request.Operation.CoverageWorkItemIDs) != 0 {
		t.Fatal("只读诊断伪造 CWI proof attribution")
	}
	if len(aggregate.Facts.Evidence.Accepted) != 1 || len(aggregate.Facts.Evidence.Accepted[0].CoverageWorkItemIDs) != 0 || len(aggregate.Facts.Evidence.Accepted[0].BindingRefs) != 0 {
		t.Fatal("accepted diagnostic 被投影为 CWI coverage proof")
	}
	original := aggregate.OperationReceipts[0].Clone()
	if original.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || original.Evidence.OwnerRevisionSet == nil {
		t.Fatal("实际 read 缺少 accepted receipt 或 actual dependency")
	}
	sealed, err := fileevents.SealRuntimeOperation(request.Operation)
	if err != nil {
		t.Fatal(err)
	}
	command := fileevents.QueryRuntimeFileReadDependenciesCommand{
		RuntimeOperationExecutionContext: runtimeOperationExecutionContext(request, "/must-not-use-caller-directory", ""), SealedOperation: sealed,
		RequestID: "current-observation", IssuedAtStateRevision: aggregate.StateRevision,
		SourceReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(original), OriginalDependencies: *runtimecontract.CloneRuntimeOwnerRevisionSet(original.Evidence.OwnerRevisionSet),
	}
	before := runtimeStateRecordFromAggregate(repository.aggregate)
	saves := repository.saves
	compiled, err := runtime.CompileRuntimeOperationReadSource(aggregate, command.DispatchID, command.OperationID, command.SourceReceiptHash)
	if err != nil {
		t.Fatal(err)
	}
	expectedContext := runtimecontract.CloneRuntimeOperationExecutionContext(command.RuntimeOperationExecutionContext)
	expectedContext.WorkspaceDeliverableDirectory = ""
	if !reflect.DeepEqual(compiled.Context, expectedContext) || !reflect.DeepEqual(compiled.Operation, request.Operation) ||
		!reflect.DeepEqual(compiled.OriginalDependencies, command.OriginalDependencies) || compiled.SourceReceiptHash != command.SourceReceiptHash {
		t.Fatal("重编来源不等于实际原执行 request/receipt")
	}
	compiled.Context.TargetRefs[0] = "caller-context"
	compiled.Operation.TargetRefs[0] = "caller-operation"
	*compiled.Operation.Parameters[0].Integer = 999
	compiled.OriginalDependencies.Entries[0].Revision = "caller-dependency"
	if !reflect.DeepEqual(runtimeStateRecordFromAggregate(repository.aggregate), before) || !reflect.DeepEqual(runtimeStateRecordFromAggregate(aggregate), before) {
		t.Fatal("重编返回值污染原 aggregate")
	}
	for _, scenario := range []string{"dispatch", "operation", "receipt", "canonical", "missing_receipt", "raw_dependency", "coherent_dependency", "nil_dependency", "execution_hash"} {
		t.Run("compiler_"+scenario, func(t *testing.T) {
			value := runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(aggregate))
			dispatchID, operationID, hash := command.DispatchID, command.OperationID, command.SourceReceiptHash
			switch scenario {
			case "dispatch":
				dispatchID = "missing-dispatch"
			case "operation":
				operationID = "missing-operation"
			case "receipt":
				hash = "coherently-forged"
			case "canonical":
				dispatchID = " " + dispatchID
			case "missing_receipt":
				value.OperationReceipts = nil
			case "raw_dependency":
				value.OperationReceipts[0].Evidence.OwnerRevisionSet.Entries[0].ResourceID += " "
			case "coherent_dependency":
				set := value.OperationReceipts[0].Evidence.OwnerRevisionSet
				set.Entries[0].Revision = "forged"
				*set = set.WithHash()
				hash = runtimecontract.StableRuntimeOperationReceiptHash(value.OperationReceipts[0])
			case "nil_dependency":
				value.OperationReceipts[0].Evidence.OwnerRevisionSet = nil
			case "execution_hash":
				value.CompletedDispatches[0].OperationExecutions[0].OperationHash = "forged"
			}
			input := runtimeStateRecordFromAggregate(value)
			if _, err := runtime.CompileRuntimeOperationReadSource(value, dispatchID, operationID, hash); err == nil {
				t.Fatal("非法来源被重编为完整读取来源")
			}
			if !reflect.DeepEqual(runtimeStateRecordFromAggregate(value), input) || repository.saves != saves {
				t.Fatal("编译拒绝改写输入或保存 aggregate")
			}
		})
	}
	assertCurrent := func(bytes []byte, selected string, match bool) {
		t.Helper()
		loads := repository.loads
		current, observed, err := owner.queryRuntimeFileReadDependenciesFromAggregate(ctx, repository.aggregate, command.DispatchID, command.OperationID, command.SourceReceiptHash, command.RequestID)
		if err != nil || !observed || current == nil || repository.loads != loads+1 {
			t.Fatalf("actual receipt authority current query: observed=%v loads=%d err=%v", observed, repository.loads-loads, err)
		}
		digest := sha256.Sum256(bytes)
		hash := hex.EncodeToString(digest[:])
		body, err := json.Marshal(struct {
			WorkspaceID, TargetRef, ContentHash, SelectedContent string
			StartLine, EndLine, MaxBytes                         int64
			Truncated, Binary, ContentComplete                   bool
		}{"ws-v2", target, hash, selected, start, end, bound, false, false, false})
		if err != nil {
			t.Fatal(err)
		}
		selector := sha256.Sum256(append([]byte("file-read-dependency/v1\x00"), body...))
		if len(current.Entries) != 1 || current.Entries[0].Revision != hash || current.Entries[0].DependencyHash != hex.EncodeToString(selector[:]) {
			t.Fatal("current query 不等于独立 actual bytes/selector oracle")
		}
		matches, err := runtimecontract.RuntimeOwnerRevisionSetMatchesCurrent(command.OriginalDependencies, *current)
		if err != nil || matches != match {
			t.Fatalf("current applicability=%v want=%v err=%v", matches, match, err)
		}
		current.Entries[0].Revision = "caller-change"
		if repository.saves != saves || !reflect.DeepEqual(runtimeStateRecordFromAggregate(repository.aggregate), before) {
			t.Fatal("current query 改写原 aggregate/receipt/credit")
		}
	}
	assertCurrent(raw, "原文\r\n", true)
	write("unrelated.txt", []byte("unrelated"))
	assertCurrent(raw, "原文\r\n", true)
	changed := []byte("first\r\n新文\r\nlast\n")
	write(target, changed)
	assertCurrent(changed, "新文\r\n", false)
	for _, scenario := range []string{"definition", "transaction", "dispatch", "receipt", "dependency", "operation"} {
		t.Run(scenario, func(t *testing.T) {
			value := command
			value.RuntimeOperationExecutionContext = runtimecontract.CloneRuntimeOperationExecutionContext(command.RuntimeOperationExecutionContext)
			value.OriginalDependencies = *runtimecontract.CloneRuntimeOwnerRevisionSet(&command.OriginalDependencies)
			switch scenario {
			case "definition":
				value.Transaction.DefinitionRevision = "other-definition"
				value.OriginalDependencies.DefinitionRevision = value.Transaction.DefinitionRevision
				value.OriginalDependencies = value.OriginalDependencies.WithHash()
			case "transaction":
				value.Transaction.AuthoritySnapshotID = "other-authority"
			case "dispatch":
				value.DispatchID = "missing-dispatch"
			case "receipt":
				value.SourceReceiptHash = "other-receipt"
			case "dependency":
				value.OriginalDependencies.Entries[0].Revision = "coherently-forged"
				value.OriginalDependencies = value.OriginalDependencies.WithHash()
			case "operation":
				operation := request.Operation.Normalized()
				operation.Parameters = nil
				var err error
				value.SealedOperation, err = fileevents.SealRuntimeOperation(operation)
				if err != nil {
					t.Fatal(err)
				}
				record := runtimecontract.RuntimeOperationExecutionRecord{
					OperationID: operation.ID, ExecutionKey: value.Execution.ExecutionKey, Fence: value.Execution.Fence,
					OperationHash: runtimecontract.StableRuntimeOperationHash(operation), ExpectedEffectScope: value.Execution.ExpectedEffectScope,
					IdempotencyClass: value.Execution.IdempotencyClass, AdmissibilitySnapshotID: value.Execution.AdmissibilitySnapshotID,
					AcquisitionBinding: value.Execution.AcquisitionBinding, ReplayProof: value.Execution.ReplayProof,
				}
				value.Execution.OperationHash = record.OperationHash
				value.Execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(record)
			}
			current, observed, err := owner.queryRuntimeFileReadDependencies(ctx, value)
			if err == nil || observed || current != nil {
				t.Fatalf("协调篡改绕过原 persisted receipt 来源: observed=%v err=%v", observed, err)
			}
			if repository.saves != saves || !reflect.DeepEqual(runtimeStateRecordFromAggregate(repository.aggregate), before) {
				t.Fatal("拒绝查询改写 aggregate")
			}
		})
	}
	// 同版本完整 JSON reload 与两个 owner 重建后，仍只反查原 receipt，不增加执行或 CAS。
	body, err := json.Marshal(repository.aggregate)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded runtime.RuntimeAggregate
	if err := json.Unmarshal(body, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Validate(); err != nil {
		t.Fatal(err)
	}
	repository.aggregate = reloaded
	controller, err = runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	owner.runtimeController = controller
	rebuilt, err := runtime.CompileRuntimeOperationReadSource(reloaded, command.DispatchID, command.OperationID, command.SourceReceiptHash)
	if err != nil || !reflect.DeepEqual(rebuilt.Context, expectedContext) || !reflect.DeepEqual(rebuilt.Operation, request.Operation) || !reflect.DeepEqual(rebuilt.OriginalDependencies, command.OriginalDependencies) {
		t.Fatalf("JSON 重载后未从原持久来源重编: err=%v", err)
	}
	if err := file.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	file, err = filebiz.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	assertCurrent(changed, "新文\r\n", false)
	if err := os.Remove(filepath.Join(directory, target)); err != nil {
		t.Fatal(err)
	}
	current, observed, err := owner.queryRuntimeFileReadDependenciesFromAggregate(ctx, repository.aggregate, command.DispatchID, command.OperationID, command.SourceReceiptHash, command.RequestID)
	if err != nil || observed || current != nil {
		t.Fatalf("实际 missing 被当完整 observation: observed=%v err=%v", observed, err)
	}
	if err := os.Mkdir(filepath.Join(directory, target), 0700); err != nil {
		t.Fatal(err)
	}
	current, observed, err = owner.queryRuntimeFileReadDependenciesFromAggregate(ctx, repository.aggregate, command.DispatchID, command.OperationID, command.SourceReceiptHash, command.RequestID)
	if err == nil || observed || current != nil {
		t.Fatal("非普通 target 错误被当 missing 或 observed")
	}
	file.BeginShutdown(ctx)
	if _, observed, err := owner.queryRuntimeFileReadDependenciesFromAggregate(ctx, repository.aggregate, command.DispatchID, command.OperationID, command.SourceReceiptHash, command.RequestID); err == nil || observed {
		t.Fatal("BeginShutdown 后接纳新 current query")
	}
	if repository.saves != saves || !reflect.DeepEqual(runtimeStateRecordFromAggregate(repository.aggregate), before) {
		t.Fatal("reload/missing/error/shutdown query 改写原 aggregate 或 receipt")
	}
	if command.WorkspaceDeliverableDirectory != "/must-not-use-caller-directory" {
		t.Fatal("adapter 改写 caller directory")
	}
}
