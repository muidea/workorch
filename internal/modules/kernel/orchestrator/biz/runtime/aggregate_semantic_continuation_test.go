package runtime

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAggregateSemanticContinuationPreservesEveryOrderedGate(t *testing.T) {
	for _, scenario := range []string{"complete", "source failure", "before failure", "semantic failure", "after failure", "missing continuation"} {
		t.Run(scenario, func(t *testing.T) {
			a := stateChangeAggregate(t)
			s := runtimeLineageAssertionSources(a)
			beforeErr, afterErr := error(nil), error(nil)
			switch scenario {
			case "source failure":
				s.EvidenceSequence++
			case "before failure":
				beforeErr = errors.New("exact interstitial invariant failure")
			case "semantic failure":
				s.Semantic.Phase = "unknown"
			case "after failure":
				afterErr = errors.New("exact remaining invariant failure")
			}
			var gotOrder, wantOrder []string
			before := func() error { gotOrder = append(gotOrder, "before"); return beforeErr }
			after := func() error { gotOrder = append(gotOrder, "after"); return afterErr }
			if scenario == "missing continuation" {
				after = nil
			}
			got := runtimecontract.ValidateRuntimeConvergenceSourcesAndSemanticLedger(s, a.Decision.RequiredOutput, a.StateRevision, before, after)
			// Independent full gates, with no access to a shared context.
			want := func() error {
				if err := runtimecontract.ValidateRuntimeConvergenceSourceLedgers(s, a.Decision.RequiredOutput, a.StateRevision); err != nil {
					return err
				}
				if after == nil {
					return fmt.Errorf("runtime aggregate requires complete invariant continuations")
				}
				wantOrder = append(wantOrder, "before")
				if beforeErr != nil {
					return beforeErr
				}
				if err := s.Semantic.Validate(); err != nil {
					return fmt.Errorf("runtime aggregate semantic planning ledger: %w", err)
				}
				wantOrder = append(wantOrder, "after")
				return afterErr
			}()
			if fmt.Sprint(got) != fmt.Sprint(want) || !reflect.DeepEqual(gotOrder, wantOrder) {
				t.Fatalf("ordered gate mismatch: got=%v/%v want=%v/%v", got, gotOrder, want, wantOrder)
			}
		})
	}
}
