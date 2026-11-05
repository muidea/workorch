package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCommittedAssessmentTransitionUsesOriginalSealedTransaction(t *testing.T) {
	controller, repository, _ := prepareOwnerFactAssessment(t, true)
	before := repository.aggregate.CloneRaw()
	committed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCommittedAssessmentResultTransition(before, committed); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"missing result":    func(a *RuntimeAggregate) { a.Facts.Assessments = nil },
		"duplicate result":  func(a *RuntimeAggregate) { a.Facts.Assessments = append(a.Facts.Assessments, a.Facts.Assessments[0]) },
		"wrong transaction": func(a *RuntimeAggregate) { a.Facts.Assessments[0].ID += "-other" },
		"retained active":   func(a *RuntimeAggregate) { d := cloneSealedDispatchRaw(*before.ActiveDispatch); a.ActiveDispatch = &d },
		"missing dispatch": func(a *RuntimeAggregate) {
			a.CompletedDispatches = a.CompletedDispatches[:len(a.CompletedDispatches)-1]
		},
		"changed sealed candidate": func(a *RuntimeAggregate) {
			a.CompletedDispatches[len(a.CompletedDispatches)-1].Candidate.Assessment.Summary += "-changed"
		},
		"changed authority": func(a *RuntimeAggregate) {
			a.CompletedDispatches[len(a.CompletedDispatches)-1].AssessmentAuthorityInput.SourceText += "-changed"
		},
		"missing adjudication": func(a *RuntimeAggregate) {
			a.CompletedDispatches[len(a.CompletedDispatches)-1].AssessmentAdjudication = nil
		},
		"uncommitted dispatch": func(a *RuntimeAggregate) {
			a.CompletedDispatches[len(a.CompletedDispatches)-1].State = DispatchStateInvalidated
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := committed.CloneRaw()
			mutate(&changed)
			// Removing the fact is also rejected by the complete double-entry
			// gate; the transition itself only protects an actual append.
			err := validateCommittedAssessmentResultTransition(before, changed)
			if name == "missing result" {
				err = changed.Validate()
			}
			if err == nil {
				t.Fatal("invalid result crossed its original commit boundary")
			}
		})
	}
	// A duplicate/new append without an active transaction cannot make a new
	// authority from the previous result, even after a cold controller reload.
	duplicate := committed.CloneRaw()
	duplicate.Facts.Assessments = append(duplicate.Facts.Assessments, duplicate.Facts.Assessments[0])
	if err := validateCommittedAssessmentResultTransition(committed, duplicate); !errors.Is(err, common.ErrConflict) {
		t.Fatal(err)
	}
	unrelated := before.CloneRaw()
	unrelated.ActiveDispatch = nil
	if err := validateCommittedAssessmentResultTransition(unrelated, committed); !errors.Is(err, common.ErrConflict) {
		t.Fatal(err)
	}
}

func TestCommittedAssessmentTransitionRejectsRehashedHistoricalRewrites(t *testing.T) {
	controller, repository, _ := prepareOwnerFactAssessment(t, true)
	committed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	di, fi := runtimeAssessmentAdjudicationIndexes(t, committed)
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"drop facts":                  func(a *RuntimeAggregate) { a.Facts.Assessments = nil },
		"change producer observation": func(a *RuntimeAggregate) { a.Facts.Assessments[fi].Summary += "-changed" },
		"drop dispatch":               func(a *RuntimeAggregate) { a.CompletedDispatches = a.CompletedDispatches[:di] },
		"rehashed missing proof": func(a *RuntimeAggregate) {
			d := &a.CompletedDispatches[di]
			d.AssessmentAdjudication.Proofs = nil
			d.AssessmentAdjudication.Disposition = runtimecontract.AssessmentDispositionRepairableGap
			d.AssessmentAdjudicationHash = runtimecontract.StableRuntimeAssessmentAdjudicationHash(*d.AssessmentAdjudication)
			a.Facts.Assessments[fi].Disposition = d.AssessmentAdjudication.Disposition
			a.Facts.Assessments[fi].AdjudicationHash = d.AssessmentAdjudicationHash
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := committed.CloneRaw()
			mutate(&changed)
			if err := validateCommittedAssessmentResultTransition(committed, changed); !errors.Is(err, common.ErrConflict) {
				t.Fatal(err)
			}
			assertCommittedAssessmentRewriteRejected(t, repository, committed, changed)
		})
	}
	// A later transaction may change current facts without editing any old
	// adjudication. This check itself does not grant current credit.
	later := committed.CloneRaw()
	later.Facts.Resources.OperationAdmissibilities = nil
	if err := validateCommittedAssessmentResultTransition(committed, later); err != nil {
		t.Fatal(err)
	}
}
