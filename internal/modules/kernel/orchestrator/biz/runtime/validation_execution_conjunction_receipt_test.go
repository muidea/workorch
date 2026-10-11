package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 验证接收端沿原 receipt/evidence 私有事务保全 owner 合取事实，不宣称
// 这份 typed fixture 是实际 Shell producer、公开准入或完整工程验收。
func validationConjunctionReceiptFixture() runtimecontract.RuntimeValidationObservation {
	digest := sha256.Sum256(nil)
	empty := runtimecontract.RuntimeValidationOutputIdentity{SHA256: hex.EncodeToString(digest[:]), Complete: true}
	identity := runtimecontract.RuntimeValidationExecutionStageIdentity{ID: "owner-stage", Scope: "exact-owner-scope", RequireTests: true, RequireBehavior: true}
	now := time.Now().UTC()
	conjunction := runtimecontract.RuntimeValidationExecutionConjunction{
		ProtocolRef: "owner/opaque-execution/v1", RequiredStagesHash: runtimecontract.StableRuntimeValidationExecutionStagesHash([]runtimecontract.RuntimeValidationExecutionStageIdentity{identity}),
		Complete: true, BehaviorTargetRefs: []string{"implementation"},
		Stages: []runtimecontract.RuntimeValidationExecutionStage{{Identity: identity, Argv: []string{"owner-tool", "exact"}, Started: true, StartedAt: now.Format(time.RFC3339Nano), FinishedAt: now.Add(time.Second).Format(time.RFC3339Nano), TerminalComplete: true, SourceBefore: empty.SHA256, SourceAfter: empty.SHA256, Stdout: empty, Stderr: empty, Tests: 1, PassedTests: 1}},
	}
	conjunction.Stages[0].Stdout.Storage = &runtimecontract.RuntimeValidationOutputStorage{Reference: "owner-output/stdout"}
	conjunction.Stages[0].Stderr.Storage = &runtimecontract.RuntimeValidationOutputStorage{Reference: "owner-output/stderr"}
	return runtimecontract.RuntimeValidationObservation{RecipeID: "owner-recipe", ObservationProtocolRef: conjunction.ProtocolRef, ExecutedChecks: []string{"native"}, PassedChecks: []string{"native"}, ExecutionConjunction: &conjunction}
}

func TestRuntimeValidationExecutionConjunctionReceiptSourceSurvivesReload(t *testing.T) {
	controller, repository, sealed, _, _ := lineageReceiptSealedAggregate(t)
	claimed, receipt := lineageReceiptClaim(t, controller, sealed)
	observation := validationConjunctionReceiptFixture()
	conjunction := *observation.ExecutionConjunction
	receipt.Evidence.Validation = &observation
	ownerCommitted := lineageReceiptObserve(t, controller, claimed, receipt)
	evidence := lineageReceiptEvidence(t, ownerCommitted, receipt)
	// 私有入站正文不能覆盖原 persisted receipt 的生产事实。
	evidence.Evidence.Validation = nil
	result, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventOperationEvidenceObserved, Scope: lineageReceiptScope(ownerCommitted), Transaction: &RuntimeInternalTransactionPayload{ExecutionEvidence: &evidence}})
	if err != nil {
		t.Fatal(err)
	}
	aggregate := result.Aggregate
	if len(aggregate.Facts.Evidence.Accepted) != 1 || !reflect.DeepEqual(aggregate.Facts.Evidence.Accepted[0].Validation.ExecutionConjunction, &conjunction) {
		t.Fatal("receipt-to-fact 丢失原 owner 执行合取")
	}
	reloaded := runtimeTestEvidenceAggregateRoundTrip(t, aggregate)
	if err := reloaded.Validate(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	saves := repository.saves
	duplicate := lineageReceiptObserve(t, reopened, reloaded, receipt)
	if saves != repository.saves || !reflect.DeepEqual(duplicate, reloaded) {
		t.Fatal("重载后 duplicate 改写原 receipt 或重复 CAS")
	}
	conflicting := receipt.Clone()
	conflicting.Evidence.Validation.ExecutionConjunction.Stages[0].Stdout.Storage.Reference = "owner-output/conflicting"
	before := runtimeTestEvidenceAggregateRoundTrip(t, *repository.aggregate)
	if _, err := reopened.ApplyInternalEvent(context.Background(), lineageReceiptEvent(reloaded, conflicting)); err == nil || repository.saves != saves || !reflect.DeepEqual(before, runtimeTestEvidenceAggregateRoundTrip(t, *repository.aggregate)) {
		t.Fatal("同 execution 异 raw 引用未零变更拒绝")
	}
	for _, scenario := range []string{"missing", "argv", "source", "output", "storage-reference", "storage-drop-all"} {
		changed := aggregate.CloneRaw()
		fact := &changed.Facts.Evidence.Accepted[0]
		c := fact.Validation.ExecutionConjunction
		switch scenario {
		case "missing":
			fact.Validation.ExecutionConjunction = nil
		case "argv":
			c.Stages[0].Argv[1] = "other"
		case "source":
			c.Stages[0].SourceBefore, c.Stages[0].SourceAfter = conjunction.RequiredStagesHash, conjunction.RequiredStagesHash
		case "output":
			c.Stages[0].Stdout.SHA256 = conjunction.RequiredStagesHash
		case "storage-reference":
			c.Stages[0].Stdout.Storage.Reference = "owner-output/other"
		case "storage-drop-all":
			c.Stages[0].Stdout.Storage, c.Stages[0].Stderr.Storage = nil, nil
		}
		runtimeTestRebindEvidenceFactHash(t, &changed, fact.ID)
		if err := changed.Validate(); err == nil {
			t.Fatalf("自洽 fact hash 绕过原 receipt 合取来源：%s", scenario)
		}
	}
}

func TestRuntimeValidationExecutionConjunctionReceiptCASFailureAndReplyLoss(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_commit", true: "after_commit_reply_loss"}[afterCommit], func(t *testing.T) {
			controller, _, sealed, _, _ := lineageReceiptSealedAggregate(t)
			claimed, receipt := lineageReceiptClaim(t, controller, sealed)
			observation := validationConjunctionReceiptFixture()
			receipt.Evidence.Validation = &observation
			repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &claimed}, failBefore: !afterCommit, failAfter: afterCommit}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := controller.ApplyInternalEvent(context.Background(), lineageReceiptEvent(claimed, receipt)); err == nil {
				t.Fatal("未观察到原 CAS 故障")
			}
			persisted, err := repository.Load(context.Background(), claimed.Facts.WorkspaceID, claimed.Facts.RunID)
			if err != nil || persisted.Validate() != nil {
				t.Fatalf("故障后原 aggregate 无法完整重载：%v", err)
			}
			if !afterCommit && !reflect.DeepEqual(persisted, claimed) {
				t.Fatal("CAS 前失败污染持久前像")
			}
			if afterCommit && (len(persisted.OperationReceipts) != 1 || !reflect.DeepEqual(persisted.OperationReceipts[0].Evidence.Validation, &observation)) {
				t.Fatal("响应丢失没有保全原 owner 合取")
			}
			restarted, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			committed := lineageReceiptObserve(t, restarted, persisted, receipt)
			if repository.saves != 1 || committed.StateRevision != claimed.StateRevision+1 || !reflect.DeepEqual(committed.OperationReceipts[0].Evidence.Validation, &observation) {
				t.Fatal("重启重复 CAS 或替换了原合取事实")
			}
			evidence := lineageReceiptEvidence(t, committed, receipt)
			result, err := restarted.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventOperationEvidenceObserved, Scope: lineageReceiptScope(committed), Transaction: &RuntimeInternalTransactionPayload{ExecutionEvidence: &evidence}})
			if err != nil || result.Aggregate.Validate() != nil || len(result.Aggregate.Facts.Evidence.Accepted) != 1 || !reflect.DeepEqual(result.Aggregate.Facts.Evidence.Accepted[0].Validation, &observation) {
				t.Fatalf("恢复后 receipt-to-fact 丢失合取来源：%v", err)
			}
		})
	}
}
