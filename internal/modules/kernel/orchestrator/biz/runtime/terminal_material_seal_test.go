package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func materialSealTerminalAggregate(t *testing.T) RuntimeAggregate {
	t.Helper()
	aggregate := stateChangeAggregate(t)
	aggregate.Termination = &runtimecontract.RuntimeTerminationRecord{Kind: runtimecontract.RuntimeTerminationCancelled, RequestedBy: "blocks/run", Reason: "取消", Revision: aggregate.StateRevision}
	return aggregate
}

func TestTerminalMaterialSealRequiresExactIndependentGeneration(t *testing.T) {
	previous := materialSealTerminalAggregate(t)
	seal, err := terminalMaterialSealFromAggregate(previous)
	if err != nil {
		t.Fatal(err)
	}
	next := previous
	next.StateRevision++
	next.TerminalMaterialSeal = &seal
	if err := validateTerminalMaterialSealTransition(previous, next); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RuntimeAggregate){
		func(value *RuntimeAggregate) {
			value.TerminalMaterialSeal.GenerationStateRevision--
			value.TerminalMaterialSeal.Material.RuntimeCloseout.RuntimeStateRevision--
		},
		func(value *RuntimeAggregate) {
			value.TerminalMaterialSeal.RecoveryLedger.CASReloadAttempts++
			value.TerminalMaterialSeal.Material.RuntimeCloseout.RecoveryLedger.CASReloadAttempts++
		},
		func(value *RuntimeAggregate) { value.Facts.Recovery.CASReloadAttempts++ },
	} {
		invalid := next
		copy := seal.CloneRaw()
		invalid.TerminalMaterialSeal = &copy
		mutate(&invalid)
		body := invalid.TerminalMaterialSeal.Material.RuntimeCloseout.WithCloseoutHash()
		invalid.TerminalMaterialSeal.Material.RuntimeCloseout = &body
		invalid.TerminalMaterialSeal.MaterialHash, err = invalid.TerminalMaterialSeal.Material.CanonicalHash()
		if err != nil {
			t.Fatal(err)
		}
		if err := validateTerminalMaterialSealTransition(previous, invalid); err == nil {
			t.Fatal("协调重哈希不能替代独立生成前像")
		}
	}
}

func TestTerminalMaterialSealImmutableAcrossRecovery(t *testing.T) {
	previous := materialSealTerminalAggregate(t)
	seal, err := terminalMaterialSealFromAggregate(previous)
	if err != nil {
		t.Fatal(err)
	}
	previous.StateRevision++
	previous.TerminalMaterialSeal = &seal
	next := previous
	next.StateRevision++
	next.Facts.Recovery.CASReloadAttempts++
	if err := validateTerminalMaterialSealTransition(previous, next); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seal.Material.RuntimeCloseout.RecoveryLedger, previous.Facts.Recovery) {
		t.Fatal("seal 未保留生成时完整 recovery 前像")
	}
	next.TerminalMaterialSeal = nil
	if err := validateTerminalMaterialSealTransition(previous, next); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("删除 seal 未拒绝: %v", err)
	}
}

func TestTerminalMaterialSealPrivateEventRejectsCallerMaterialBeforeLoad(t *testing.T) {
	event := RuntimeInternalEvent{
		Kind: RuntimeInternalEventTerminalMaterialSealRequired,
		Scope: RuntimeInternalEventScope{WorkspaceID: "workspace", RunID: "run"},
		Delivery: &RuntimeInternalDeliveryPayload{Reason: "caller material"},
	}
	if err := event.validateEnvelope(); err == nil {
		t.Fatal("seal private event 允许 caller 携带材料或其它 transition 数据")
	}
	event.Delivery = &RuntimeInternalDeliveryPayload{}
	if err := event.validateEnvelope(); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalMaterialSealCASLoadsIndependentPrevious(t *testing.T) {
	previous := materialSealTerminalAggregate(t)
	seal, err := terminalMaterialSealFromAggregate(previous)
	if err != nil {
		t.Fatal(err)
	}
	repository := &sealTransitionRepository{aggregate: previous}
	coordinator, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	next := previous
	next.StateRevision++
	next.TerminalMaterialSeal = &seal
	repository.aggregate.StateRevision++
	err = coordinator.Commit(context.Background(), next, previous.StateRevision)
	var conflict *RuntimeCASConflictError
	if !errors.As(err, &conflict) || repository.saves != 0 {
		t.Fatalf("stale 独立前像应零 CAS: err=%v saves=%d", err, repository.saves)
	}
}

type sealTransitionRepository struct {
	aggregate RuntimeAggregate
	saves int
}

func (r *sealTransitionRepository) Load(context.Context, common.WorkspaceID, common.RunID) (RuntimeAggregate, error) {
	return r.aggregate, nil
}

func (r *sealTransitionRepository) Save(_ context.Context, next RuntimeAggregate, expected uint64) error {
	r.saves++
	if r.aggregate.StateRevision != expected {
		return common.ErrConflict
	}
	r.aggregate = next
	return nil
}
