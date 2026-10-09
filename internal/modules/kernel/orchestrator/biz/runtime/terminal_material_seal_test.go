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
		Kind:     RuntimeInternalEventTerminalMaterialSealRequired,
		Scope:    RuntimeInternalEventScope{WorkspaceID: "workspace", RunID: "run"},
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

func TestTerminalMaterialSealCommitAndReloadPreserveGeneration(t *testing.T) {
	previous := materialSealTerminalAggregate(t)
	repository := &sealTransitionRepository{aggregate: previous}
	coordinator, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	seal, err := terminalMaterialSealFromAggregate(previous)
	if err != nil {
		t.Fatal(err)
	}
	next := previous
	next.StateRevision++
	next.TerminalMaterialSeal = &seal
	if err := coordinator.Commit(context.Background(), next, previous.StateRevision); err != nil {
		t.Fatal(err)
	}
	if repository.saves != 1 || repository.aggregate.StateRevision != previous.StateRevision+1 {
		t.Fatal("首次 seal 未在独立 CAS 中推进一个 revision")
	}
	if err := repository.aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	source := terminalMaterialSource(repository.aggregate)
	source.TerminalMaterialSeal = repository.aggregate.TerminalMaterialSeal
	if err := runtimecontract.ValidateRuntimeTerminalMaterialSeal(source.TerminalMaterialSeal, source); err != nil {
		t.Fatal(err)
	}
	if _, err := terminalMaterialSealFromAggregate(repository.aggregate); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("已存在的 seal 被允许重新生成: %v", err)
	}
	if !reflect.DeepEqual(repository.aggregate.TerminalMaterialSeal.Material.RuntimeCloseout.RecoveryLedger, previous.Facts.Recovery) {
		t.Fatal("seal 丢失独立前像的 recovery")
	}
}

func TestTerminalMaterialSealCompletionRequiresExactSavedBody(t *testing.T) {
	previous := materialSealTerminalAggregate(t)
	seal, err := terminalMaterialSealFromAggregate(previous)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := previous
	aggregate.StateRevision++
	aggregate.TerminalMaterialSeal = &seal
	body := seal.Material.RuntimeCloseout.CloneRaw()
	for index, owner := range []runtimecontract.RuntimeDeliveryOwner{
		runtimecontract.RuntimeDeliveryOwnerResult,
		runtimecontract.RuntimeDeliveryOwnerWorkspace,
		runtimecontract.RuntimeDeliveryOwnerRun,
	} {
		ack := runtimecontract.RuntimeDeliveryAcknowledgement{
			WorkspaceID: seal.Identity.WorkspaceID, RunID: seal.Identity.RunID,
			TerminalDeliveryID: seal.Identity.TerminalDeliveryID, Owner: owner,
			AuthoritySnapshotID: seal.Identity.AuthoritySnapshotID,
			CausationID:         "terminal-delivery/" + seal.Identity.TerminalDeliveryID,
			Outcome:             seal.Identity.Outcome,
		}
		switch owner {
		case runtimecontract.RuntimeDeliveryOwnerResult:
			ack.ResultID = runtimecontract.RuntimeTerminalResultID(ack.WorkspaceID, ack.RunID)
			ack.ResultCloseout = &body
			ack.AcknowledgedState = runtimecontract.RuntimeDeliveryAcknowledgedResultRecorded
		case runtimecontract.RuntimeDeliveryOwnerWorkspace:
			ack.AcknowledgedState = runtimecontract.RuntimeDeliveryAcknowledgedWorkspaceReleased
		case runtimecontract.RuntimeDeliveryOwnerRun:
			ack.AcknowledgedState = runtimecontract.RuntimeDeliveryAcknowledgedRunTerminal
		}
		if err := validateDeliveryAcknowledgementAgainstAggregate(aggregate, ack); err != nil {
			t.Fatal(err)
		}
		aggregate.DeliveryAcknowledgements = append(aggregate.DeliveryAcknowledgements, ack)
		aggregate.StateRevision++
		if err := aggregate.Validate(); err != nil {
			t.Fatal(err)
		}
		if TerminalDeliveryFullyAcknowledged(aggregate) != (index == 2) {
			t.Fatalf("%d 条合法回执的完成判定错误", index+1)
		}
	}
	for _, mutate := range []func(*RuntimeAggregate){
		func(value *RuntimeAggregate) { value.TerminalMaterialSeal = nil },
		func(value *RuntimeAggregate) { value.DeliveryAcknowledgements[0].ResultCloseout = nil },
		func(value *RuntimeAggregate) {
			changed := value.DeliveryAcknowledgements[0].ResultCloseout.CloneRaw()
			changed.RuntimeStateRevision++
			changed = changed.WithCloseoutHash()
			value.DeliveryAcknowledgements[0].ResultCloseout = &changed
		},
		func(value *RuntimeAggregate) {
			value.DeliveryAcknowledgements[0], value.DeliveryAcknowledgements[1] = value.DeliveryAcknowledgements[1], value.DeliveryAcknowledgements[0]
		},
	} {
		invalid := aggregate
		invalid.DeliveryAcknowledgements = append([]runtimecontract.RuntimeDeliveryAcknowledgement(nil), aggregate.DeliveryAcknowledgements...)
		mutate(&invalid)
		if TerminalDeliveryFullyAcknowledged(invalid) {
			t.Fatal("缺失 seal、缺失或重写材料、乱序回执被计为完成")
		}
		if err := invalid.Validate(); err == nil {
			t.Fatal("损坏的持久终态通过 fresh-load 校验")
		}
	}
	if !TerminalDeliveryFullyAcknowledged(aggregate) || !reflect.DeepEqual(*aggregate.DeliveryAcknowledgements[0].ResultCloseout, *seal.Material.RuntimeCloseout) {
		t.Fatal("负向测试改写了合法原材料")
	}
}

func TestTerminalMaterialSealResponseLossReplayUsesExactPersistedGeneration(t *testing.T) {
	previous := materialSealTerminalAggregate(t)
	responseLoss := errors.New("seal CAS 已提交但响应丢失")
	repository := &sealTransitionRepository{aggregate: previous, saveErr: responseLoss}
	coordinator, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	kernel, err := newRuntimeMutationKernel(repository, coordinator)
	if err != nil {
		t.Fatal(err)
	}
	component := newDriveDeliveryComponent(kernel)
	if _, err := component.sealTerminalMaterial(context.Background(), previous.Facts.WorkspaceID, previous.Facts.RunID); !errors.Is(err, responseLoss) {
		t.Fatalf("未保留原持久化响应丢失: %v", err)
	}
	if repository.saves != 1 || repository.aggregate.TerminalMaterialSeal == nil {
		t.Fatal("response-loss fixture 未实际持久化 seal")
	}
	original := repository.aggregate.TerminalMaterialSeal.CloneRaw()
	generation := repository.aggregate.StateRevision
	repository.saveErr = nil
	// 同版本重启只能重载原 seal；后续 recovery 变化不生成新材料。
	repository.aggregate.Facts.Recovery.ProviderTransportAttempts++
	repository.aggregate.StateRevision++
	restarted, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	restartedKernel, err := newRuntimeMutationKernel(repository, restarted)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := newDriveDeliveryComponent(restartedKernel).sealTerminalMaterial(context.Background(), previous.Facts.WorkspaceID, previous.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != 1 || replayed.StateRevision != generation+1 || !reflect.DeepEqual(original, *replayed.TerminalMaterialSeal) {
		t.Fatal("response-loss/restart 重放写入或改写了原材料 generation")
	}
	if replayed.TerminalMaterialSeal.Material.RuntimeCloseout.RecoveryLedger.ProviderTransportAttempts != previous.Facts.Recovery.ProviderTransportAttempts {
		t.Fatal("重放以当前 recovery 替代了生成前像")
	}
}

func TestTerminalMaterialSealCannotBeInjectedDuringInitialization(t *testing.T) {
	previous := materialSealTerminalAggregate(t)
	seal, err := terminalMaterialSealFromAggregate(previous)
	if err != nil {
		t.Fatal(err)
	}
	// 结构完整的终态材料不能绕开独立前像，通过 expected=0 初始化直接注入。
	seal.GenerationStateRevision = 1
	body := seal.Material.RuntimeCloseout.CloneRaw()
	body.RuntimeStateRevision = 1
	body = body.WithCloseoutHash()
	seal.Material.RuntimeCloseout = &body
	seal.MaterialHash, err = seal.Material.CanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	aggregate := previous
	aggregate.StateRevision = 1
	aggregate.TerminalMaterialSeal = &seal
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	repository := &transitionCoordinatorRepository{}
	coordinator, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinator.Commit(context.Background(), aggregate, 0); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("初始化允许 caller 注入 seal: %v", err)
	}
	if repository.loads != 0 || repository.saves != 0 {
		t.Fatal("初始化 seal 拒绝路径触发了 I/O")
	}
}

func TestTerminalMaterialSealPrivateEventIsIdempotentAndBudgetNeutral(t *testing.T) {
	previous := materialSealTerminalAggregate(t)
	repository := &sealTransitionRepository{aggregate: previous}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	event := RuntimeInternalEvent{
		Kind:     RuntimeInternalEventTerminalMaterialSealRequired,
		Scope:    RuntimeInternalEventScope{WorkspaceID: previous.Facts.WorkspaceID, RunID: previous.Facts.RunID},
		Delivery: &RuntimeInternalDeliveryPayload{},
	}
	sealed, err := controller.ApplyInternalEvent(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Aggregate.TerminalMaterialSeal == nil || sealed.Aggregate.StateRevision != previous.StateRevision+1 || repository.saves != 1 ||
		!reflect.DeepEqual(sealed.Aggregate.Facts.Recovery, previous.Facts.Recovery) || sealed.BudgetConsumed {
		t.Fatal("首次私有 seal 事件未精确独立提交或消耗了 recovery 预算")
	}
	replayed, err := controller.ApplyInternalEvent(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != 1 || !reflect.DeepEqual(sealed.Aggregate, replayed.Aggregate) || replayed.BudgetConsumed {
		t.Fatal("重复私有 seal 事件推进了 revision、预算或材料")
	}
}

func TestTerminalMaterialSealSharedValidatorRejectsAuthorityAndMaterialDrift(t *testing.T) {
	previous := materialSealTerminalAggregate(t)
	seal, err := terminalMaterialSealFromAggregate(previous)
	if err != nil {
		t.Fatal(err)
	}
	baseline := terminalMaterialSource(previous)
	baseline.StateRevision++
	if err := runtimecontract.ValidateRuntimeTerminalMaterialSeal(&seal, baseline); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*runtimecontract.RuntimeStateRecord, *runtimecontract.RuntimeTerminalMaterialSeal){
		func(source *runtimecontract.RuntimeStateRecord, _ *runtimecontract.RuntimeTerminalMaterialSeal) {
			source.Decision.AuthoritySnapshot.RunID = "foreign-run"
		},
		func(source *runtimecontract.RuntimeStateRecord, _ *runtimecontract.RuntimeTerminalMaterialSeal) {
			source.Decision.AuthoritySnapshot.DefinitionRevision = "foreign-definition"
		},
		func(source *runtimecontract.RuntimeStateRecord, _ *runtimecontract.RuntimeTerminalMaterialSeal) {
			source.StateRevision--
		},
		func(_ *runtimecontract.RuntimeStateRecord, candidate *runtimecontract.RuntimeTerminalMaterialSeal) {
			candidate.Material.Summary = "伪造完整材料"
		},
		func(_ *runtimecontract.RuntimeStateRecord, candidate *runtimecontract.RuntimeTerminalMaterialSeal) {
			candidate.Material.RuntimeCloseout.RuntimeStateRevision++
		},
	} {
		source := baseline.Clone()
		candidate := seal.CloneRaw()
		mutate(&source, &candidate)
		body := candidate.Material.RuntimeCloseout.WithCloseoutHash()
		candidate.Material.RuntimeCloseout = &body
		candidate.MaterialHash, err = candidate.Material.CanonicalHash()
		if err != nil {
			t.Fatal(err)
		}
		if err := runtimecontract.ValidateRuntimeTerminalMaterialSeal(&candidate, source); err == nil {
			t.Fatal("共享 seal 校验接受 authority/scope/generation/body 协同重哈希漂移")
		}
	}
	if err := runtimecontract.ValidateRuntimeTerminalMaterialSeal(&seal, baseline); err != nil {
		t.Fatalf("负向测试污染了原封存材料: %v", err)
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
	saves     int
	saveErr   error
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
	return r.saveErr
}
