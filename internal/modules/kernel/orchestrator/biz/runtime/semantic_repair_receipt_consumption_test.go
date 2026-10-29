package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Uses the production semantic-repair fixture's sealed ordinary validation and
// persisted receipt. It does not invent a closure or a repair lineage.
func testSemanticRepairReceiptConsumption(t *testing.T, original RuntimeAggregate, evidence ExecutionEvidence) {
	t.Helper()
	if original.Facts.RepairDirective == nil {
		t.Fatal("semantic consumption fixture lacks its outstanding handoff")
	}
	t.Run("receipt-consumption-source", func(t *testing.T) {
		a := original.CloneRaw()
		e := canonicalPersistedOwnerReceiptEvidenceIdentity(a, evidence)
		admission, err := admitExecutionEvidenceIdentity(a, e)
		if err != nil {
			t.Fatal(err)
		}
		fact, _, err := deriveExecutionEvidenceFact(a, admission)
		if err != nil {
			t.Fatal(err)
		}
		// Construct only the receipt transaction's pre-consumption window. The
		// whole after-image/CAS and recovery are exercised separately below.
		a.Facts.Evidence.Rejected = append(a.Facts.Evidence.Rejected, fact)
		d := *a.ActiveDispatch
		d.OperationExecutions = markOperationExecutionEvidenceCommitted(d.OperationExecutions, e.OperationID, e.OwnerReceiptRef, e.OwnerReceiptHash, fact)
		d.CompletedOperationIDs = appendNormalizedString(d.CompletedOperationIDs, e.OperationID)
		d.State = DispatchStateCommitted
		a.CompletedDispatches = append(a.CompletedDispatches, d)
		a.ActiveDispatch = nil
		before := a.CloneRaw()
		t.Run("confirmed-handoff-is-not-semantic", func(t *testing.T) {
			other := before.CloneRaw()
			other.Facts.RepairDirective.ID = "confirmed-repair/" + other.Facts.Focus.ID + "/" + other.Facts.RepairDirective.Closure.ID + "/fresh-validation"
			pre := other.CloneRaw()
			if runtimeValidationFailureRepairOwnsFreshValidationDirective(other.Facts) {
				t.Fatal("same CWI and old reopen lineage classified a confirmed handoff as semantic")
			}
			if err := consumeSemanticRepairFreshValidationReceipt(&other, d, fact); err != nil || !reflect.DeepEqual(other, pre) {
				t.Fatalf("semantic consumer touched another continuation: %v", err)
			}
		})
		if err := consumeSemanticRepairFreshValidationReceipt(&a, d, fact); err != nil {
			t.Fatal(err)
		}
		want := before.CloneRaw()
		want.Facts.RepairDirective = nil
		if !reflect.DeepEqual(a, want) {
			t.Fatal("consumption changed proof, current sampling, effect or budget")
		}
		if err := consumeSemanticRepairFreshValidationReceipt(&a, d, fact); err != nil || !reflect.DeepEqual(a, want) {
			t.Fatalf("duplicate consumption changed facts: %v", err)
		}
		for name, mutate := range map[string]func(*RuntimeAggregate){
			"missing original receipt": func(v *RuntimeAggregate) { v.OperationReceipts = nil },
			"missing preseal manifest": func(v *RuntimeAggregate) { v.CandidateRequestSessions = nil },
			"missing repair evidence":  func(v *RuntimeAggregate) { v.Facts.Evidence.Accepted = nil },
			"receipt hash drift": func(v *RuntimeAggregate) {
				for i := range v.OperationReceipts {
					v.OperationReceipts[i].Evidence.Summary += "drift"
				}
			},
		} {
			t.Run(name, func(t *testing.T) {
				bad := before.CloneRaw()
				mutate(&bad)
				pre := bad.CloneRaw()
				if err := consumeSemanticRepairFreshValidationReceipt(&bad, d, fact); err == nil {
					t.Fatal("invalid source consumed handoff")
				}
				if !reflect.DeepEqual(bad, pre) {
					t.Fatal("rejected source changed facts")
				}
			})
		}
		for name, mutate := range map[string]func(*SealedDispatch, *runtimecontract.EvidenceFact){
			"foreign dispatch": func(_ *SealedDispatch, f *runtimecontract.EvidenceFact) { f.DispatchID = "foreign" },
			"before barrier": func(_ *SealedDispatch, f *runtimecontract.EvidenceFact) {
				f.Revision = before.Facts.RepairDirective.SupersedesEvidenceRevision
			},
			"foreign binding": func(_ *SealedDispatch, f *runtimecontract.EvidenceFact) { f.BindingRefs = []string{"foreign"} },
			"effect instead of validation": func(_ *SealedDispatch, f *runtimecontract.EvidenceFact) {
				f.Kind = runtimecontract.EvidenceKindStateChange
			},
			"work commitment": func(d *SealedDispatch, _ *runtimecontract.EvidenceFact) {
				d.ImplementationWorkCommitment = &runtimecontract.RuntimeImplementationWorkRound{}
			},
		} {
			t.Run(name, func(t *testing.T) {
				bad, dispatch, f := before.CloneRaw(), d, fact
				mutate(&dispatch, &f)
				pre := bad.CloneRaw()
				if err := consumeSemanticRepairFreshValidationReceipt(&bad, dispatch, f); err != nil || !reflect.DeepEqual(bad, pre) {
					t.Fatalf("unrelated receipt consumed old handoff: %v", err)
				}
			})
		}
	})
	for _, cut := range []string{"before-cas", "after-cas"} {
		t.Run("receipt-consumption-recovery-"+cut, func(t *testing.T) {
			base := &evidenceCommitCountingRepository{aggregate: original.CloneRaw()}
			repository := &semanticReceiptConsumptionFaultRepository{RuntimeStateRepository: base, after: cut == "after-cas"}
			coordinator, err := NewDispatchCoordinator(repository)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := coordinator.commitEvidence(context.Background(), evidence); !errors.Is(err, errSemanticReceiptConsumptionCut) {
				t.Fatalf("missing requested crash cut: %v", err)
			}
			if cut == "before-cas" && !reflect.DeepEqual(base.aggregate, original) {
				t.Fatal("failed CAS changed preimage")
			}
			body, err := json.Marshal(base.aggregate)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded RuntimeAggregate
			if err := json.Unmarshal(body, &reloaded); err != nil {
				t.Fatal(err)
			}
			if err := reloaded.Validate(); err != nil {
				t.Fatal(err)
			}
			base.aggregate = reloaded
			restarted, err := NewDispatchCoordinator(base)
			if err != nil {
				t.Fatal(err)
			}
			result, err := restarted.commitEvidence(context.Background(), evidence)
			if err != nil {
				t.Fatal(err)
			}
			if result.Aggregate.Facts.RepairDirective != nil {
				t.Fatal("recovery resurrected consumed directive")
			}
			if cut == "after-cas" && result.Disposition != CommitDispositionDuplicate {
				t.Fatal("response loss repeated receipt CAS")
			}
			if base.saves != 1 {
				t.Fatalf("receipt saved %d times", base.saves)
			}
			before := base.aggregate.CloneRaw()
			duplicate, err := restarted.commitEvidence(context.Background(), evidence)
			if err != nil || duplicate.Disposition != CommitDispositionDuplicate || !reflect.DeepEqual(before, base.aggregate) || base.saves != 1 {
				t.Fatalf("duplicate changed effect, budget or receipt: %v", err)
			}
		})
	}
}

var errSemanticReceiptConsumptionCut = errors.New("semantic receipt consumption test CAS cut")

type semanticReceiptConsumptionFaultRepository struct {
	RuntimeStateRepository
	after bool
	fired bool
}

func (r *semanticReceiptConsumptionFaultRepository) Save(ctx context.Context, aggregate RuntimeAggregate, revision uint64) error {
	if r.fired {
		return r.RuntimeStateRepository.Save(ctx, aggregate, revision)
	}
	r.fired = true
	if r.after {
		if err := r.RuntimeStateRepository.Save(ctx, aggregate, revision); err != nil {
			return err
		}
	}
	return errSemanticReceiptConsumptionCut
}

var _ RuntimeStateRepository = (*semanticReceiptConsumptionFaultRepository)(nil)
