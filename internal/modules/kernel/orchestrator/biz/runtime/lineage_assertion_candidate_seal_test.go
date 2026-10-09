package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 故障只发生在正式 Save 边界；Load/Save 保全独立 raw 前像，不以重新封存修复非法状态。
type lineageCandidateSealFaultRepository struct {
	controllerRepository
	failBefore bool
	failAfter  bool
}

func (r *lineageCandidateSealFaultRepository) Save(ctx context.Context, aggregate RuntimeAggregate, revision uint64) error {
	if r.failBefore {
		r.failBefore = false
		return errors.New("candidate seal CAS 前故障")
	}
	if err := r.controllerRepository.Save(ctx, aggregate, revision); err != nil {
		return err
	}
	if r.failAfter {
		r.failAfter = false
		return errors.New("candidate seal 已持久化但响应丢失")
	}
	return nil
}

func TestRuntimeLineageCandidateSealIndependentPreimageAndResponseLoss(t *testing.T) {
	ctx := context.Background()
	rule := runtimecontract.RuntimeLineageAssertionRule{
		Ref: "exact-file-receipt", Kind: runtimecontract.RuntimeLineageOwnerOperationReceipt,
		ConsumptionPoint: runtimecontract.RuntimeProofPhaseExecution,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite,
		RequiredEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
	}
	var previous RuntimeAggregate
	var event RuntimeInternalEvent
	_, _, baseline, _, _ := lineageReceiptSealedAggregateWithRule(t, rule, func(value RuntimeAggregate, seal RuntimeInternalEvent) {
		previous, event = value, seal
	})
	if err := previous.Validate(); err != nil {
		t.Fatalf("独立 pending 前像: %v", err)
	}
	if previous.ActiveDispatch != nil || len(previous.CandidateAttempts) != 1 || previous.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStatePending || previous.CandidateAttempts[0].Proposal == nil {
		t.Fatal("没有截取 production candidate seal 前的完整 pending attempt")
	}
	if baseline.ActiveDispatch == nil || baseline.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || baseline.StateRevision != previous.StateRevision+1 {
		t.Fatal("正常路径没有原子封存 accepted attempt 与 dispatch")
	}
	if !reflect.DeepEqual(previous.ExecutionBudget, baseline.ExecutionBudget) || previous.Facts.Recovery != baseline.Facts.Recovery {
		t.Fatal("candidate seal 不应重复消费 provider、capability 或 recovery 预算")
	}
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "CAS_before_failure", true: "committed_reply_loss"}[after], func(t *testing.T) {
			initial := previous.CloneRaw()
			repository := &lineageCandidateSealFaultRepository{
				controllerRepository: controllerRepository{aggregate: &initial},
				failBefore: !after, failAfter: after,
			}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := controller.ApplyInternalEvent(ctx, event); err == nil {
				t.Fatal("未返回真实 seal 持久化故障")
			}
			persisted, err := repository.Load(ctx, previous.Facts.WorkspaceID, previous.Facts.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if err := persisted.Validate(); err != nil {
				t.Fatalf("故障后 fresh-load 出现 torn attempt/dispatch: %v", err)
			}
			want, saves := previous, 0
			if after {
				want, saves = baseline, 1
			}
			if !reflect.DeepEqual(persisted, want) || repository.saves != saves {
				t.Fatal("故障改变原始前像或未保全完整同一 CAS after-image")
			}
			// 独立加载产物可变，持久化前像不得受调用方改写影响。
			persisted.CandidateAttempts[0].State = "caller-mutated-invalid-state"
			fresh, err := repository.Load(ctx, previous.Facts.WorkspaceID, previous.Facts.RunID)
			if err != nil || !reflect.DeepEqual(fresh, want) {
				t.Fatalf("Load 共享 attempt 切片污染独立前像: %v", err)
			}
			reopened, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			result, err := reopened.ApplyInternalEvent(ctx, event)
			if err != nil || result.Admission == nil || !result.Admission.Admission.Accepted() {
				t.Fatalf("重启重放原 sealed candidate: %v", err)
			}
			if err := result.Aggregate.Validate(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result.Aggregate, baseline) || repository.saves != 1 {
				t.Fatal("重放改变 normal seal 材料、规则、预算或重复 CAS")
			}
			duplicate, err := reopened.ApplyInternalEvent(ctx, event)
			if err != nil || !reflect.DeepEqual(duplicate.Aggregate, baseline) || repository.saves != 1 {
				t.Fatalf("duplicate seal 非幂等: %v", err)
			}
			conflicting := baseline.CloneRaw()
			conflicting.StateRevision++
			if err := repository.Save(ctx, conflicting, previous.StateRevision); !errors.Is(err, common.ErrConflict) || repository.saves != 1 {
				t.Fatalf("stale CAS 非零写入拒绝: %v", err)
			}
			final, err := repository.Load(ctx, previous.Facts.WorkspaceID, previous.Facts.RunID)
			if err != nil || !reflect.DeepEqual(final, baseline) {
				t.Fatalf("stale CAS 改写 seal 原材料: %v", err)
			}
		})
	}
}
