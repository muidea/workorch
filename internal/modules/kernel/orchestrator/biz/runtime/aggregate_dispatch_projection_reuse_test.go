package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func referenceAggregateDispatchProjection(a RuntimeAggregate) (*runtimecontract.RuntimeDispatchRecord, []runtimecontract.RuntimeDispatchRecord) {
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	return active, sealedDispatchesToRuntimeRecords(a.CompletedDispatches)
}

func assertAggregateDispatchReuseReference(t testing.TB, a RuntimeAggregate) {
	t.Helper()
	before := a.CloneRaw()
	want, got := referenceAggregateDispatchReuseValidate(a), a.Validate()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("同次 dispatch 复用改变完整门禁：got=%v want=%v", got, want)
	}
	if !reflect.DeepEqual(a, before) {
		t.Fatal("完整门禁污染 aggregate 原值")
	}
	active, completed := referenceAggregateDispatchProjection(a)
	projectedActive, projectedCompleted := referenceAggregateDispatchProjection(a)
	phases := []struct {
		name string
		want error
		got  error
	}{
		{"SM-2", referenceDispatchReuseValidateRuntimeAggregateSM2Authority(a), validateRuntimeAggregateSM2Authority(a, active, completed)},
		{"SM-3", referenceDispatchReuseValidateRuntimeAggregateSM3CandidateTransaction(a), validateRuntimeAggregateSM3CandidateTransaction(a, active, completed)},
		{"recovery", referenceDispatchReuseValidateRuntimeAggregateOperationRecoveryAuthorizations(a), validateRuntimeAggregateOperationRecoveryAuthorizations(a, active, completed)},
		{"governance", referenceDispatchReuseValidateRuntimeAggregateGovernanceLedgers(a), validateRuntimeAggregateGovernanceLedgers(a, active, completed)},
		{"cross-machine", referenceDispatchReuseValidateRuntimeAggregateCrossMachineFences(a), validateRuntimeAggregateCrossMachineFences(a, active, completed)},
	}
	for _, phase := range phases {
		if fmt.Sprint(phase.got) != fmt.Sprint(phase.want) {
			t.Fatalf("%s 门禁拒绝漂移：got=%v want=%v", phase.name, phase.got, phase.want)
		}
	}
	if !reflect.DeepEqual(active, projectedActive) || !reflect.DeepEqual(completed, projectedCompleted) || !reflect.DeepEqual(a, before) {
		t.Fatal("私有门禁改写共享 detached 投影或原值")
	}
}

func aggregateDispatchReuseFixtures(t *testing.T) []RuntimeAggregate {
	t.Helper()
	initial := stateChangeAggregate(t)
	repository := &durableRuntimeStateRepository{aggregate: initial.CloneRaw()}
	active := sealCandidateForTest(t, repository, stateChangeCandidate())
	completed := validationAggregate(t)
	if active.ActiveDispatch == nil || len(completed.CompletedDispatches) == 0 {
		t.Fatal("fixture 缺少真实 sealed active/completed dispatch")
	}
	for _, value := range []RuntimeAggregate{initial, active, completed} {
		if err := value.Validate(); err != nil {
			t.Fatalf("合法 fixture 被拒绝：%v", err)
		}
	}
	return []RuntimeAggregate{initial, active, completed}
}

func TestAggregateDispatchProjectionReuseKeepsCompleteGatesAndOriginalValues(t *testing.T) {
	fixtures := aggregateDispatchReuseFixtures(t)
	for index, base := range fixtures {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			assertAggregateDispatchReuseReference(t, base)
			assertAggregateDispatchReuseReference(t, base)
			body, err := json.Marshal(base)
			if err != nil {
				t.Fatal(err)
			}
			var loaded RuntimeAggregate
			if err := json.Unmarshal(body, &loaded); err != nil {
				t.Fatal(err)
			}
			assertAggregateDispatchReuseReference(t, loaded)
			zeroRevision := base.CloneRaw()
			zeroRevision.StateRevision = 0
			assertAggregateDispatchReuseReference(t, zeroRevision)
			before := base.CloneRaw()
			active, completed := referenceAggregateDispatchProjection(base)
			if active != nil {
				active.Candidate.StateChange.Operations[0].Operation.TargetRefs[0] = "changed-return"
			}
			if len(completed) != 0 {
				completed[0].Candidate.StateChange.Operations[0].Operation.TargetRefs[0] = "changed-return"
			}
			if !reflect.DeepEqual(base, before) {
				t.Fatal("detached dispatch 返回污染来源")
			}
			assertAggregateDispatchReuseReference(t, base)
		})
	}
	mutations := []struct {
		name   string
		mutate func(*RuntimeAggregate)
	}{
		{"version", func(a *RuntimeAggregate) { a.ResultFeedbackVersion = 0 }},
		{"scope", func(a *RuntimeAggregate) { a.Facts.WorkspaceID = "" }},
		{"definition", func(a *RuntimeAggregate) { a.Definition.Revision = "changed" }},
		{"candidate", func(a *RuntimeAggregate) {
			a.CandidateAttempts = append(a.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{})
		}},
		{"session", func(a *RuntimeAggregate) {
			a.CandidateRequestSessions = append(a.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{})
		}},
		{"receipt", func(a *RuntimeAggregate) {
			a.OperationReceipts = append(a.OperationReceipts, runtimecontract.RuntimeOperationReceipt{})
		}},
		{"governance-request", func(a *RuntimeAggregate) {
			a.GovernanceRequests = append(a.GovernanceRequests, runtimecontract.RuntimeGovernanceRequestRecord{})
		}},
		{"governance-binding", func(a *RuntimeAggregate) {
			a.OperationGovernanceBindings = append(a.OperationGovernanceBindings, runtimecontract.RuntimeOperationGovernanceBinding{})
		}},
		{"recovery", func(a *RuntimeAggregate) { a.Facts.Recovery.OperationRecoveryAttempts++ }},
		{"evidence-sequence", func(a *RuntimeAggregate) { a.EvidenceSequence++ }},
		{"multiple-invalid", func(a *RuntimeAggregate) {
			a.Facts.WorkspaceID = ""
			a.CandidateAttempts = append(a.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{})
			a.OperationReceipts = append(a.OperationReceipts, runtimecontract.RuntimeOperationReceipt{})
			a.Facts.Recovery.OperationRecoveryAttempts++
		}},
		{"dispatch-hash", func(a *RuntimeAggregate) {
			if a.ActiveDispatch != nil {
				a.ActiveDispatch.CandidateHash = "changed"
			} else {
				a.CompletedDispatches[0].CandidateHash = "changed"
			}
		}},
		{"dispatch-target", func(a *RuntimeAggregate) {
			if a.ActiveDispatch != nil {
				a.ActiveDispatch.Candidate.StateChange.Operations[0].Operation.TargetRefs[0] = "changed"
			} else {
				a.CompletedDispatches[0].Candidate.StateChange.Operations[0].Operation.TargetRefs[0] = "changed"
			}
		}},
		{"duplicate-history", func(a *RuntimeAggregate) {
			if a.ActiveDispatch != nil {
				a.CompletedDispatches = append(a.CompletedDispatches, *a.ActiveDispatch)
			} else {
				a.CompletedDispatches = append(a.CompletedDispatches, a.CompletedDispatches[0])
			}
		}},
	}
	for index, base := range fixtures[1:] {
		for _, mutation := range mutations {
			t.Run(fmt.Sprintf("%d/%s", index, mutation.name), func(t *testing.T) {
				value := base.CloneRaw()
				mutation.mutate(&value)
				assertAggregateDispatchReuseReference(t, value)
				if err := value.Validate(); err == nil {
					t.Fatal("后续非法变化被当成先前校验成功")
				}
			})
		}
	}
}

func TestAggregateDispatchProjectionReuseKeepsActiveAndCompletedTogether(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: validationAggregate(t)}
	a := sealCandidateForTest(t, repository, validationCandidate())
	if a.ActiveDispatch == nil || len(a.CompletedDispatches) == 0 {
		t.Fatal("fixture 未包含 active 与 completed 两侧")
	}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	assertAggregateDispatchReuseReference(t, a)
	body, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var loaded RuntimeAggregate
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	assertAggregateDispatchReuseReference(t, loaded)
	changed := a.CloneRaw()
	changed.CompletedDispatches[0].CandidateHash = "later-history-hash"
	assertAggregateDispatchReuseReference(t, changed)
	if err := changed.Validate(); err == nil {
		t.Fatal("active 存在时忽略 completed 后续 hash 漂移")
	}
}

func TestAggregateDispatchProjectionReuseRemovesOnlyRepeatedDetachedCopies(t *testing.T) {
	for index, a := range aggregateDispatchReuseFixtures(t)[1:] {
		original := testing.AllocsPerRun(3, func() {
			if err := referenceAggregateDispatchReuseValidate(a); err != nil {
				t.Fatal(err)
			}
		})
		current := testing.AllocsPerRun(3, func() {
			if err := a.Validate(); err != nil {
				t.Fatal(err)
			}
		})
		t.Logf("fixture=%d current=%v original=%v", index, current, original)
		if current >= original {
			t.Fatal("同次 dispatch 复用未减少重复副本分配")
		}
	}
}

func TestAggregateDispatchProjectionReuseKeepsIndependentCASAndStaleFence(t *testing.T) {
	base := aggregateDispatchReuseFixtures(t)[1]
	repository := &durableRuntimeStateRepository{aggregate: base.CloneRaw()}
	coordinator, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	before, err := repository.Load(context.Background(), base.Facts.WorkspaceID, base.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	zeroRevision := before.CloneRaw()
	zeroRevision.StateRevision = 0
	if err := coordinator.Commit(context.Background(), zeroRevision, before.StateRevision); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("revision 顺序错误未在 coordinator 拒绝：%v", err)
	}
	changed := before.CloneRaw()
	changed.StateRevision++
	changed.ActiveDispatch.CandidateHash = "later-hash-drift"
	if err := coordinator.Commit(context.Background(), changed, before.StateRevision); err == nil {
		t.Fatal("非法 after-image 绕过完整 coordinator 门禁")
	}
	if !reflect.DeepEqual(repository.aggregate, before) {
		t.Fatal("失败提交污染独立前像")
	}
	next := before.CloneRaw()
	next.StateRevision++
	repository.aggregate.StateRevision++
	persisted := repository.aggregate.CloneRaw()
	var stale *RuntimeCASConflictError
	if err := coordinator.Commit(context.Background(), next, before.StateRevision); !errors.As(err, &stale) {
		t.Fatalf("stale revision 绕过独立前像 fence：%v", err)
	}
	if !reflect.DeepEqual(repository.aggregate, persisted) {
		t.Fatal("stale 提交污染原持久值")
	}
	repository.aggregate = before.CloneRaw()
	repository.aggregate.ActiveDispatch.CandidateHash = "corrupt-preimage"
	persisted = repository.aggregate.CloneRaw()
	if err := coordinator.Commit(context.Background(), next, before.StateRevision); err == nil || !strings.Contains(err.Error(), "runtime transition preimage invariant") {
		t.Fatalf("合法 after-image 绕过非法独立前像：%v", err)
	}
	if !reflect.DeepEqual(repository.aggregate, persisted) {
		t.Fatal("前像失败被合法 after-image 覆盖")
	}
}

func FuzzAggregateDispatchProjectionReuseCompleteGate(f *testing.F) {
	f.Add(" raw ", uint8(0))
	f.Add(string([]byte{0xff, 0x80}), uint8(3))
	f.Add("中文\x00", uint8(5))
	f.Fuzz(func(t *testing.T, raw string, field uint8) {
		repository := &durableRuntimeStateRepository{aggregate: stateChangeAggregate(t)}
		a := sealCandidateForTest(t, repository, stateChangeCandidate())
		switch field % 7 {
		case 0:
			a.ActiveDispatch.ID = raw
		case 1:
			a.ActiveDispatch.CandidateHash = raw
		case 2:
			a.ActiveDispatch.Transaction.DefinitionRevision = raw
		case 3:
			a.ActiveDispatch.Candidate.StateChange.Operations[0].Operation.TargetRefs[0] = raw
		case 4:
			a.Facts.WorkspaceID = common.WorkspaceID(raw)
		case 5:
			a.ActiveDispatch.OperationExecutions[0].ExecutionKey = raw
		case 6:
			a.ActiveDispatch.DispatchAuthorityHash = raw
		}
		assertAggregateDispatchReuseReference(t, a)
	})
}

func FuzzAggregateDispatchProjectionReuseAcceptedCandidateReference(f *testing.F) {
	f.Add("dispatch", "candidate", "manifest", uint8(2), true)
	f.Add(string([]byte{0xff, 0x80}), "\x00", "中文", uint8(0), false)
	f.Fuzz(func(t *testing.T, id, hash, manifest string, count uint8, withActive bool) {
		a := RuntimeAggregate{}
		for index := range int(count) % 16 {
			a.CompletedDispatches = append(a.CompletedDispatches, SealedDispatch{ID: id, CandidateHash: hash, CandidateManifestHash: manifest})
			a.CandidateAttempts = append(a.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{State: runtimecontract.RuntimeCandidateAttemptStateAccepted, CandidateHash: hash, ManifestHash: manifest, Attempt: index})
		}
		if withActive {
			value := SealedDispatch{ID: id, CandidateHash: hash, CandidateManifestHash: manifest}
			a.ActiveDispatch = &value
		}
		before := a.CloneRaw()
		active, completed := referenceAggregateDispatchProjection(a)
		want := referenceDispatchReuseValidateAcceptedCandidateDispatchBindings(a)
		got := validateAcceptedCandidateDispatchBindings(a.CandidateAttempts, active, completed)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("accepted candidate 拒绝漂移：got=%v want=%v", got, want)
		}
		if !reflect.DeepEqual(a, before) {
			t.Fatal("accepted candidate 查询污染原值")
		}
	})
}
