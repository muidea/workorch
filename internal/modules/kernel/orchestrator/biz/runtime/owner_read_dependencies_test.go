package runtime

import (
	"context"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 此回归使用 typed owner observation；真实 File bytes producer 由 File owner 测试独立验证。
func TestRuntimeOwnerReadDependenciesPersistExactReceiptEvidenceAndReload(t *testing.T) {
	controller, repository, sealed, binding, _ := lineageReceiptSealedAggregate(t)
	claimed, receipt := lineageReceiptClaim(t, controller, sealed)
	dependencies := (runtimecontract.RuntimeOwnerRevisionSet{
		DefinitionRevision: claimed.Facts.DefinitionRevision,
		Entries: []runtimecontract.RuntimeOwnerRevisionRef{{
			OwnerKind: "capability", OwnerInstanceID: "file/" + string(claimed.Facts.WorkspaceID),
			ResourceKind: "workspace_target", ResourceID: receipt.TargetRefs[0], Revision: receipt.AfterHash, DependencyHash: "actual-owner-read",
		}},
	}).WithHash()
	receipt.Evidence.OwnerRevisionSet = &dependencies
	ownerCommitted := lineageReceiptObserve(t, controller, claimed, receipt)
	evidence := lineageReceiptEvidence(t, ownerCommitted, receipt)
	// 私有事件正文不是来源；删去其依赖仍只能投影 persisted receipt 的完整集合。
	evidence.Evidence.OwnerRevisionSet = nil
	result, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind: RuntimeInternalEventOperationEvidenceObserved, Scope: lineageReceiptScope(ownerCommitted),
		Transaction: &RuntimeInternalTransactionPayload{ExecutionEvidence: &evidence},
	})
	if err != nil {
		t.Fatal(err)
	}
	aggregate := result.Aggregate
	if len(aggregate.Facts.Evidence.Accepted) != 1 || !reflect.DeepEqual(aggregate.Facts.Evidence.Accepted[0].OwnerRevisionSet, &dependencies) {
		t.Fatal("真实 receipt commit 未保全完整读取依赖")
	}
	for _, value := range []RuntimeAggregate{aggregate.CloneRaw(), runtimeTestEvidenceAggregateRoundTrip(t, aggregate)} {
		if err := value.Validate(); err != nil || !reflect.DeepEqual(value.Facts.Evidence.Accepted[0].OwnerRevisionSet, &dependencies) {
			t.Fatalf("fresh-load 来源反证失败: %v", err)
		}
		value.Facts.Evidence.Accepted[0].OwnerRevisionSet.Entries[0].Revision = "caller-change"
		if aggregate.Facts.Evidence.Accepted[0].OwnerRevisionSet.Entries[0].Revision != dependencies.Entries[0].Revision {
			t.Fatal("aggregate raw／normalized／reload 副本共享依赖")
		}
	}
	reopened, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	saves := repository.saves
	duplicate := lineageReceiptObserve(t, reopened, aggregate, receipt)
	if saves != repository.saves || !reflect.DeepEqual(duplicate, aggregate) {
		t.Fatal("duplicate owner dependencies 改写或重复持久化")
	}
	for _, scenario := range []string{"missing", "revision", "selector", "definition", "raw", "hash"} {
		t.Run(scenario, func(t *testing.T) {
			changed := aggregate.CloneRaw()
			fact := &changed.Facts.Evidence.Accepted[0]
			switch scenario {
			case "missing":
				fact.OwnerRevisionSet = nil
			case "revision":
				fact.OwnerRevisionSet.Entries[0].Revision = "changed"
				*fact.OwnerRevisionSet = fact.OwnerRevisionSet.WithHash()
			case "selector":
				fact.OwnerRevisionSet.Entries[0].DependencyHash = "changed"
				*fact.OwnerRevisionSet = fact.OwnerRevisionSet.WithHash()
			case "definition":
				fact.OwnerRevisionSet.DefinitionRevision = "other"
				*fact.OwnerRevisionSet = fact.OwnerRevisionSet.WithHash()
			case "raw":
				fact.OwnerRevisionSet.Entries[0].ResourceID = " " + fact.OwnerRevisionSet.Entries[0].ResourceID
			case "hash":
				fact.OwnerRevisionSet.SetHash = "forged"
			}
			// 同步重算 fact 的 commit hash；必须仍由原 receipt 独立反证正文。
			runtimeTestRebindEvidenceFactHash(t, &changed, fact.ID)
			if err := changed.Validate(); err == nil {
				t.Fatal("自洽 fact hash 绕过了 exact receipt 原读取依赖")
			}
			if runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, runtimeLineageAssertionSources(changed), changed.Facts.EvidenceRevision) {
				t.Fatal("lineage guard 借用了被改写的原 receipt 依赖")
			}
		})
	}
	wrong := receipt.Clone()
	wrong.Evidence.OwnerRevisionSet.DefinitionRevision = "other"
	*wrong.Evidence.OwnerRevisionSet = wrong.Evidence.OwnerRevisionSet.WithHash()
	before := runtimeTestEvidenceAggregateRoundTrip(t, *repository.aggregate)
	if _, err := reopened.ApplyInternalEvent(context.Background(), lineageReceiptEvent(aggregate, wrong)); err == nil || !reflect.DeepEqual(before, runtimeTestEvidenceAggregateRoundTrip(t, *repository.aggregate)) {
		t.Fatal("错 Definition 的 owner receipt 重放未零变更拒绝")
	}
}

func TestRuntimeOwnerReadDependenciesSurviveReceiptCASFailureAndReplyLoss(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_commit", true: "after_commit_reply_loss"}[afterCommit], func(t *testing.T) {
			controller, _, sealed, _, _ := lineageReceiptSealedAggregate(t)
			claimed, receipt := lineageReceiptClaim(t, controller, sealed)
			dependencies := (runtimecontract.RuntimeOwnerRevisionSet{
				DefinitionRevision: claimed.Facts.DefinitionRevision,
				Entries: []runtimecontract.RuntimeOwnerRevisionRef{{
					OwnerKind: "capability", OwnerInstanceID: "typed-owner/" + string(claimed.Facts.WorkspaceID),
					ResourceKind: "workspace_target", ResourceID: receipt.TargetRefs[0], Revision: receipt.AfterHash, DependencyHash: "original-read-selector",
				}},
			}).WithHash()
			receipt.Evidence.OwnerRevisionSet = &dependencies
			repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &claimed}, failBefore: !afterCommit, failAfter: afterCommit}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := controller.ApplyInternalEvent(context.Background(), lineageReceiptEvent(claimed, receipt)); err == nil {
				t.Fatal("owner dependency receipt 丢失原 CAS 故障")
			}
			persisted, err := repository.Load(context.Background(), claimed.Facts.WorkspaceID, claimed.Facts.RunID)
			if err != nil || persisted.Validate() != nil {
				t.Fatalf("CAS 故障后原 aggregate 无法重载: %v", err)
			}
			if !afterCommit && !reflect.DeepEqual(persisted, claimed) {
				t.Fatal("CAS 前失败污染持久读取前像")
			}
			if afterCommit && (len(persisted.OperationReceipts) != 1 || !reflect.DeepEqual(persisted.OperationReceipts[0].Evidence.OwnerRevisionSet, &dependencies)) {
				t.Fatal("响应丢失未保存原完整读取依赖")
			}
			source, sourceErr := CompileRuntimeOperationReadSource(persisted, claimed.ActiveDispatch.ID, receipt.OperationID, runtimecontract.StableRuntimeOperationReceiptHash(receipt))
			if !afterCommit && sourceErr == nil {
				t.Fatal("CAS 前失败的 receipt 被重编为已保存来源")
			}
			if afterCommit && (sourceErr != nil || !reflect.DeepEqual(source.OriginalDependencies, dependencies)) {
				t.Fatalf("响应丢失后不能从已保存 receipt 重编原来源: %v", sourceErr)
			}
			restarted, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			committed := lineageReceiptObserve(t, restarted, persisted, receipt)
			if repository.saves != 1 || committed.StateRevision != claimed.StateRevision+1 || !reflect.DeepEqual(committed.OperationReceipts[0].Evidence.OwnerRevisionSet, &dependencies) {
				t.Fatal("重启重新持久化或替换了原读取依赖")
			}
			evidence := lineageReceiptEvidence(t, committed, receipt)
			result, err := restarted.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventOperationEvidenceObserved, Scope: lineageReceiptScope(committed), Transaction: &RuntimeInternalTransactionPayload{ExecutionEvidence: &evidence}})
			if err != nil || result.Aggregate.Validate() != nil || len(result.Aggregate.Facts.Evidence.Accepted) != 1 || !reflect.DeepEqual(result.Aggregate.Facts.Evidence.Accepted[0].OwnerRevisionSet, &dependencies) {
				t.Fatalf("重启 receipt-to-fact 丢失原读取依赖: %v", err)
			}
		})
	}
}
