package runtime

import (
	"context"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestValidationOutputStorageRejectedReceiptKeepsOriginalFailureAndOmission(t *testing.T) {
	repository, controller, claimed, execution, receipt := claimedManagedValidationReceiptForTest(t)
	receipt.Fence = execution.Fence
	observation := validationConjunctionReceiptFixture()
	observation.ExecutionConjunction.Complete = false
	stage := &observation.ExecutionConjunction.Stages[0]
	stage.ExitCode, stage.KnownFailure, stage.FailureCode = 7, true, "original_known_failure"
	stage.Stderr.Storage = &runtimecontract.RuntimeValidationOutputStorage{OmittedReason: "owner_storage_failed"}
	observation.ExecutedChecks, observation.PassedChecks, observation.IncompleteChecks = nil, nil, []string{"native"}
	receipt.Evidence.Validation = &observation
	receipt.Evidence.Quality = common.EvidenceQualityInvalid
	exit := int64(7)
	receipt.Evidence.ExitCode = &exit
	receipt.Feedback = runtimeOperationFeedbackForTest(t, claimed, receipt.OperationID, runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "original_known_failure",
		Signature: "owner-original-failure", Message: "原始工程检查失败；stderr 保存失败不改变已知 exit 7",
	})
	beforeDiagnostic := receipt.Clone().Feedback
	event := RuntimeInternalEvent{Kind: RuntimeInternalEventOperationReceiptObserved, Scope: lineageReceiptScope(claimed), Transaction: &RuntimeInternalTransactionPayload{
		DispatchID: claimed.ActiveDispatch.ID, Fence: execution.Fence, OperationReceiptOwnerSource: runtimecontract.RuntimeModuleShell, OperationReceipt: &receipt,
	}}
	result, err := controller.ApplyInternalEvent(context.Background(), event)
	if err != nil || result.Aggregate.Validate() != nil {
		t.Fatalf("失败回执不能保全：%v", err)
	}
	committed := result.Aggregate
	saved, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(committed.OperationReceipts, receipt.ExecutionKey, receipt.Fence)
	if !found || len(committed.OperationReceipts) != len(claimed.OperationReceipts)+1 || !reflect.DeepEqual(saved.Evidence.Validation, &observation) ||
		!reflect.DeepEqual(saved.Feedback, beforeDiagnostic) || saved.Evidence.ExitCode == nil || *saved.Evidence.ExitCode != 7 {
		t.Fatal("保存失败覆盖了原已知失败或已发布 stdout")
	}
	operation, found := candidateOperationByID(committed.ActiveDispatch.Candidate, receipt.OperationID)
	if !found {
		t.Fatal("缺少原失败 operation")
	}
	evidence, err := operationEvidenceFromPersistedReceipt(committed, *committed.ActiveDispatch, operation, saved)
	if err != nil {
		t.Fatal(err)
	}
	factResult, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventOperationEvidenceObserved, Scope: lineageReceiptScope(committed), Transaction: &RuntimeInternalTransactionPayload{ExecutionEvidence: &evidence}})
	if err != nil || factResult.Aggregate.Validate() != nil {
		t.Fatalf("失败证据不能保全：%v", err)
	}
	loaded := runtimeTestEvidenceAggregateRoundTrip(t, factResult.Aggregate)
	if len(loaded.Facts.Evidence.Accepted) != len(claimed.Facts.Evidence.Accepted) || len(loaded.Facts.Evidence.Rejected) != len(claimed.Facts.Evidence.Rejected)+1 || !reflect.DeepEqual(loaded.Facts.Evidence.Rejected[0].Validation, &observation) {
		t.Fatal("未保存诊断冒充 accepted credit 或重载丢失原事实")
	}
	before := runtimeTestEvidenceAggregateRoundTrip(t, repository.aggregate)
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := restarted.ApplyInternalEvent(context.Background(), event)
	if err != nil || !reflect.DeepEqual(runtimeTestEvidenceAggregateRoundTrip(t, duplicate.Aggregate), before) {
		t.Fatalf("重启重复回执修改原事实：%v", err)
	}
	changed := loaded.CloneRaw()
	fact := &changed.Facts.Evidence.Rejected[0]
	fact.Validation.ExecutionConjunction.Stages[0].Stderr.Storage.OmittedReason = "different_reason"
	runtimeTestRebindEvidenceFactHash(t, &changed, fact.ID)
	if changed.Validate() == nil {
		t.Fatal("失败 fact 自洽 hash 绕过原 receipt omission 来源")
	}
}
