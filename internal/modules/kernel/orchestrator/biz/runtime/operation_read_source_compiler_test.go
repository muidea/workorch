package runtime

import (
	"reflect"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// typed owner fixture 只证明来源编译；actual File bytes/selector 由 Biz 组合回归配对。
func TestRuntimeOperationReadSourceCompilerRequiresPersistedDependencies(t *testing.T) {
	for _, withDependencies := range []bool{false, true} {
		t.Run(map[bool]string{false: "nil_not_no_dependency_proof", true: "original_owner_committed"}[withDependencies], func(t *testing.T) {
			controller, repository, sealed, _, _ := lineageReceiptSealedAggregate(t)
			claimed, receipt := lineageReceiptClaim(t, controller, sealed)
			dispatchID, operationID := claimed.ActiveDispatch.ID, receipt.OperationID
			if _, err := CompileRuntimeOperationReadSource(claimed, dispatchID, operationID, runtimecontract.StableRuntimeOperationReceiptHash(receipt)); err == nil {
				t.Fatal("claimed receipt miss 被当完整只读来源")
			}
			if withDependencies {
				dependencies := (runtimecontract.RuntimeOwnerRevisionSet{
					DefinitionRevision: claimed.Facts.DefinitionRevision,
					Entries: []runtimecontract.RuntimeOwnerRevisionRef{{
						OwnerKind: "capability", OwnerInstanceID: "typed-owner/" + string(claimed.Facts.WorkspaceID),
						ResourceKind: "workspace_target", ResourceID: receipt.TargetRefs[0], Revision: receipt.AfterHash, DependencyHash: "original-selector",
					}},
				}).WithHash()
				receipt.Evidence.OwnerRevisionSet = &dependencies
				digest := strings.Repeat("a", 64)
				receipt.Evidence.ContentHash = digest
				receipt.Evidence.ContentAddress = &runtimecontract.RuntimeEvidenceContentAddress{
					Algorithm: runtimecontract.RuntimeEvidenceDigestSHA256, Digest: digest, Ref: "owner-original/" + digest,
				}
				receipt.Evidence.EvidenceRefs = append(receipt.Evidence.EvidenceRefs, receipt.Evidence.ContentAddress.Ref)
			}
			committed := lineageReceiptObserve(t, controller, claimed, receipt)
			before := committed.CloneRaw()
			saves := repository.saves
			for _, value := range []RuntimeAggregate{committed, runtimeTestEvidenceAggregateRoundTrip(t, committed)} {
				source, err := CompileRuntimeOperationReadSource(value, dispatchID, operationID, runtimecontract.StableRuntimeOperationReceiptHash(receipt))
				if !withDependencies {
					if err == nil {
						t.Fatal("nil dependencies 被补造为无依赖证明")
					}
					continue
				}
				if err != nil || source.Context.Transaction != committed.ActiveDispatch.Transaction || source.Context.Execution.Fence != receipt.Fence ||
					source.Context.EvidenceKind != runtimecontract.EvidenceKindStateChange || source.Context.WorkspaceDeliverableDirectory != "" ||
					!reflect.DeepEqual(&source.OriginalDependencies, receipt.Evidence.OwnerRevisionSet) || !reflect.DeepEqual(source.SourceContentAddress, receipt.Evidence.ContentAddress) {
					t.Fatalf("原 owner-committed 来源无法重编: err=%v", err)
				}
				queried, err := controller.queryOperationReceiptWithReadSource(t.Context(), source.Context, runtimecontract.RuntimeModuleFile)
				if err != nil || !runtimecontract.SameRuntimeOperationReceipt(queried, committed.OperationReceipts[0]) {
					t.Fatalf("重编来源不能通过原 receipt 独立反证: err=%v", err)
				}
				source.SourceContentAddress.Ref = "caller-mutated"
				if receipt.Evidence.ContentAddress.Ref == source.SourceContentAddress.Ref || committed.OperationReceipts[0].Evidence.ContentAddress.Ref == source.SourceContentAddress.Ref {
					t.Fatal("source content address shared original receipt storage")
				}
			}
			if repository.saves != saves || !reflect.DeepEqual(committed, before) {
				t.Fatal("来源编译改变原 aggregate 或执行新的 CAS")
			}
		})
	}
}
