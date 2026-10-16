package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type ownerQueryTestRepository struct {
	RuntimeStateRepository
	raw        runtimecontract.RuntimeStateRecord
	aggregate  RuntimeAggregate
	reads      atomic.Int64
	err        error
	beforeRead func()
}

func (r *ownerQueryTestRepository) LoadOwnerStateOriginal(context.Context, common.WorkspaceID, common.RunID) (runtimecontract.RuntimeStateRecord, RuntimeAggregate, error) {
	r.reads.Add(1)
	if r.beforeRead != nil {
		r.beforeRead()
	}
	return r.raw.Clone(), r.aggregate.CloneRaw(), r.err
}

func TestOwnerQueryReloadMatchesIndependentOrderedGates(t *testing.T) {
	for _, arm := range []string{"valid", "raw header", "raw scope", "converted header", "seed", "coverage", "budget", "dispatch", "owner before", "owner after", "nil empty", "first error"} {
		t.Run(arm, func(t *testing.T) {
			a := stateChangeAggregate(t)
			r := &ownerQueryTestRepository{raw: runtimeStateRecordForTerminalNonterminalRecoveryTest(t, a), aggregate: a}
			c, _ := NewRuntimeController(r)
			for call := range 4 {
				if call == 1 {
					switch arm {
					case "raw header":
						r.raw.ResultFeedbackVersion = 0
					case "raw scope":
						r.raw.Facts.RunID = ""
					case "converted header":
						r.aggregate.ResultFeedbackVersion = 0
					case "seed":
						r.aggregate.SemanticPlanning.DefinitionSeed.PolicyHash = "same hash drift"
					case "coverage":
						r.aggregate.EvidenceSequence++
					case "budget":
						r.aggregate.ExecutionBudget.Policy.PolicyRevision = "drift"
					case "dispatch":
						r.aggregate.CompletedDispatches = append(r.aggregate.CompletedDispatches, SealedDispatch{})
					case "owner before":
						r.aggregate.Decision.AuthoritySnapshot.RunID = "other"
					case "owner after":
						r.aggregate.DeliveryAcknowledgements = []runtimecontract.RuntimeDeliveryAcknowledgement{{}}
					case "nil empty":
						r.aggregate.SemanticPlanning.CompletedBatchIDs = []string{}
					case "first error":
						r.raw.Facts.RunID = ""
						r.aggregate.ResultFeedbackVersion = 0
					}
				}
				if call == 2 {
					r.raw, r.aggregate = runtimeStateRecordForTerminalNonterminalRecoveryTest(t, a), a.CloneRaw()
				}
				if call == 3 {
					c.ReleaseHistoricalCompilation()
				}
				beforeR, beforeA := r.raw.Clone(), r.aggregate.CloneRaw()
				got, err := c.Load(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID)
				want := independentOwnerStateImage(&r.raw, r.aggregate)
				if fmt.Sprint(err) != fmt.Sprint(want) {
					t.Fatalf("call %d: %v != %v", call, err, want)
				}
				if err == nil && !reflect.DeepEqual(got, r.aggregate) {
					t.Fatal("query changed complete result")
				}
				if err != nil && !reflect.DeepEqual(got, RuntimeAggregate{}) {
					t.Fatal("query returned partial aggregate")
				}
				if !reflect.DeepEqual(beforeR, r.raw) || !reflect.DeepEqual(beforeA, r.aggregate) {
					t.Fatal("query changed original")
				}
				// Neither caller mutation nor compiler retention may alias the owner.
				if err == nil {
					got.Facts.Coverage[0].TargetText = "caller mutation"
				}
				if !reflect.DeepEqual(beforeA, r.aggregate) {
					t.Fatal("return aliases owner")
				}
			}
			if r.reads.Load() != 4 {
				t.Fatal("query skipped real reads")
			}
		})
	}
}

func TestOwnerQueryReloadErrorsFallbackAndConcurrentReads(t *testing.T) {
	a := stateChangeAggregate(t)
	r := &ownerQueryTestRepository{raw: runtimeStateRecordForTerminalNonterminalRecoveryTest(t, a), aggregate: a}
	c, _ := NewRuntimeController(r)
	load := func() error { _, err := c.Load(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID); return err }
	if err := load(); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("exact owner read failure")
	r.err = sentinel
	if err := load(); !errors.Is(err, sentinel) {
		t.Fatal("read error changed")
	}
	r.err = nil
	entered, release := make(chan struct{}), make(chan struct{})
	var first atomic.Bool
	r.beforeRead = func() {
		if first.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
	}
	done := make(chan error, 1)
	go func() { done <- load() }()
	<-entered
	// This must not wait for the in-flight compiler; it independently reads
	// and validates instead of borrowing another query's mutable session.
	if err := load(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	r.beforeRead = nil
	if r.reads.Load() != 4 {
		t.Fatal("concurrent query did not reread")
	}
	for _, scope := range [][2]string{{"", "run"}, {"ws", ""}} {
		if _, err := c.Load(context.Background(), common.WorkspaceID(scope[0]), common.RunID(scope[1])); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatal("empty scope accepted")
		}
	}
	if r.reads.Load() != 4 {
		t.Fatal("invalid scope reached owner")
	}
	// A repository without the original-read interface keeps its old entry.
	fallback := &ownerQueryFallbackRepository{aggregate: a}
	c, _ = NewRuntimeController(fallback)
	if err := load(); err != nil || fallback.reads != 1 {
		t.Fatalf("fallback: %v", err)
	}
}

type ownerQueryFallbackRepository struct {
	aggregate RuntimeAggregate
	reads     int
}

func (r *ownerQueryFallbackRepository) Load(context.Context, common.WorkspaceID, common.RunID) (RuntimeAggregate, error) {
	r.reads++
	return r.aggregate.CloneRaw(), nil
}
func (*ownerQueryFallbackRepository) Save(context.Context, RuntimeAggregate, uint64) error {
	panic("query wrote authority")
}

func TestOwnerQueryAndEventShareOnlyPureHistory(t *testing.T) {
	a := stateChangeAggregate(t)
	r := &ownerStateTransactionTestRepository{previous: a, raw: runtimeStateRecordForTerminalNonterminalRecoveryTest(t, a)}
	c, _ := NewRuntimeController(r)
	ctx := context.Background()
	if _, err := c.Load(ctx, a.Facts.WorkspaceID, a.Facts.RunID); err != nil {
		t.Fatal(err)
	}
	failure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed", Signature: "query-event", Message: "original"}
	e := RuntimeInternalEvent{Kind: RuntimeInternalEventFailureObserved, Scope: RuntimeInternalEventScope{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{FailureFact: &failure}}
	if _, err := c.ApplyInternalEvent(ctx, e); err != nil {
		t.Fatal(err)
	}
	got, err := c.Load(ctx, a.Facts.WorkspaceID, a.Facts.RunID)
	if err != nil || got.StateRevision != a.StateRevision+1 || r.loads != 3 || r.rereads != 1 || r.saves != 1 {
		t.Fatalf("query/event/CAS: %v", err)
	}
	r.previous.Decision.AuthoritySnapshot.RunID = "wrong-current-owner"
	if _, err := c.Load(ctx, a.Facts.WorkspaceID, a.Facts.RunID); err == nil || r.saves != 1 {
		t.Fatal("query borrowed current invariant success")
	}
}

func FuzzOwnerQueryReloadIndependentOracle(f *testing.F) {
	for arm := uint8(0); arm < 8; arm++ {
		f.Add(arm, "原文\x00")
	}
	f.Fuzz(func(t *testing.T, arm uint8, text string) {
		if len(text) > 128 {
			t.Skip()
		}
		a := stateChangeAggregate(t)
		r := &ownerQueryTestRepository{raw: runtimeStateRecordForTerminalNonterminalRecoveryTest(t, a), aggregate: a.CloneRaw()}
		c, _ := NewRuntimeController(r)
		if _, err := c.Load(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID); err != nil {
			t.Fatal(err)
		}
		switch arm % 8 {
		case 0:
			r.raw.Definition.StartupInputHash = text
		case 1:
			r.raw.Facts.DefinitionRevision = text
		case 2:
			r.aggregate.SemanticPlanning.DefinitionSeed.PolicyHash = text
		case 3:
			r.aggregate.ExecutionBudget.Policy.PolicyRevision = text
		case 4:
			r.aggregate.Decision.AuthoritySnapshot.RunID = common.RunID(text)
		case 5:
			r.aggregate.Facts.Coverage[0].TargetText = text
		case 6:
			r.aggregate.Facts.Focus.ID = text
		case 7:
			r.aggregate.ProcessedEventIDs = append(r.aggregate.ProcessedEventIDs, text)
		}
		_, got := c.Load(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID)
		want := independentOwnerStateImage(&r.raw, r.aggregate)
		if fmt.Sprint(got) != fmt.Sprint(want) || r.reads.Load() != 2 {
			t.Fatalf("query oracle: %v != %v", got, want)
		}
	})
}
