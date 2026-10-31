package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerFactAssessmentSameCASPreimageRejectsDrift(t *testing.T) {
	controller, repository := ownerFactVersionedAssessmentFixture(t)
	before := repository.aggregate.CloneRaw()
	if !canonicalAssessmentProofCompleteWithoutOwnerFacts(before) {
		t.Fatal("original current-read window lacks ordinary proof")
	}
	completed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	// Reconstruct only the in-CAS adjudication boundary. The persisted result
	// has already reopened the CWI and is deliberately not a reusable preimage.
	boundary := before.CloneRaw()
	boundary.EvidenceSequence++
	boundary.Facts.EvidenceRevision = nextEvidenceRevision(boundary.EvidenceSequence)
	boundary.CompletedDispatches = append(boundary.CompletedDispatches, cloneSealedDispatchRaw(completed.CompletedDispatches[len(completed.CompletedDispatches)-1]))
	boundary.Facts.Assessments = append(boundary.Facts.Assessments, completed.Facts.Assessments[len(completed.Facts.Assessments)-1])
	boundary.ActiveDispatch = nil
	if canonicalAssessmentProofCompleteWithoutOwnerFacts(boundary) {
		t.Fatal("assessment sequence increment manufactured fresh owner sampling")
	}
	id := before.ActiveDispatch.ID
	if err := validateRuntimeOwnerFactAssessmentPreimage(boundary, id, before); err != nil {
		t.Fatalf("exact local adjudication boundary: %v", err)
	}
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"later CAS":          func(a *RuntimeAggregate) { a.StateRevision++ },
		"later evidence":     func(a *RuntimeAggregate) { a.EvidenceSequence++ },
		"revision alias":     func(a *RuntimeAggregate) { a.Facts.EvidenceRevision = before.Facts.EvidenceRevision },
		"changed Definition": func(a *RuntimeAggregate) { a.Definition.GoalDefinition += " changed" },
		"changed focus":      func(a *RuntimeAggregate) { a.Facts.Focus.ID += " changed" },
		"changed proof":      func(a *RuntimeAggregate) { a.Facts.Evidence.Accepted[0].ContentHash += " changed" },
		"changed receipt":    func(a *RuntimeAggregate) { a.OperationReceipts[0].AfterHash += " changed" },
		"changed sealed candidate": func(a *RuntimeAggregate) {
			a.CompletedDispatches[len(a.CompletedDispatches)-1].Candidate.Assessment.Summary += " changed"
		},
		"changed assessment hash": func(a *RuntimeAggregate) {
			a.Facts.Assessments[len(a.Facts.Assessments)-1].AdjudicationHash = "changed"
		},
		"coordinated adjudication rehash": func(a *RuntimeAggregate) {
			d := &a.CompletedDispatches[len(a.CompletedDispatches)-1]
			d.AssessmentAdjudication.Disposition = runtimecontract.AssessmentDispositionSatisfied
			d.AssessmentAdjudicationHash = runtimecontract.StableRuntimeAssessmentAdjudicationHash(*d.AssessmentAdjudication)
			a.Facts.Assessments[len(a.Facts.Assessments)-1].AdjudicationHash = d.AssessmentAdjudicationHash
		},
		"changed catalog": func(a *RuntimeAggregate) { a.Facts.Resources.OperationAdmissibilities[0].SnapshotID += " changed" },
		"changed observation": func(a *RuntimeAggregate) {
			a.Facts.EvidenceApplicabilityObservations[0].Request.IssuedAtStateRevision++
		},
		"duplicate dispatch": func(a *RuntimeAggregate) {
			a.CompletedDispatches = append(a.CompletedDispatches, a.CompletedDispatches[len(a.CompletedDispatches)-1])
		},
		"duplicate assessment": func(a *RuntimeAggregate) {
			a.Facts.Assessments = append(a.Facts.Assessments, a.Facts.Assessments[len(a.Facts.Assessments)-1])
		},
		"retained active dispatch": func(a *RuntimeAggregate) { d := cloneSealedDispatchRaw(*before.ActiveDispatch); a.ActiveDispatch = &d },
	} {
		t.Run(name, func(t *testing.T) {
			value := boundary.CloneRaw()
			mutate(&value)
			original := value.CloneRaw()
			if err := validateRuntimeOwnerFactAssessmentPreimage(value, id, before); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("drift accepted: %v", err)
			}
			if !reflect.DeepEqual(value, original) {
				t.Fatal("read-only guard modified rejected input")
			}
		})
	}
	if err := validateRuntimeOwnerFactAssessmentPreimage(completed, id, before); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("persisted successor reused historical preimage: %v", err)
	}
}

func TestOwnerFactAssessmentSameCASRecovery(t *testing.T) {
	for name, mode := range map[string]failureAuthorityCASMode{"before_write": failureAuthorityCASFailBeforeWrite, "lost_response": failureAuthorityCASLoseResponse} {
		t.Run(name, func(t *testing.T) {
			_, fixture := ownerFactVersionedAssessmentFixture(t)
			before := fixture.aggregate.CloneRaw()
			repository := &failureAuthorityCASRepository{aggregate: before.CloneRaw(), mode: mode}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2"); !errors.Is(err, errFailureAuthorityCASCut) {
				t.Fatalf("CAS cut not reached: %v", err)
			}
			if mode == failureAuthorityCASFailBeforeWrite && !reflect.DeepEqual(before, repository.aggregate) {
				t.Fatal("failed CAS leaked owner-fact settlement")
			}
			repository.mode = failureAuthorityCASNormal
			controller, err = NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			if mode == failureAuthorityCASFailBeforeWrite {
				if _, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2"); err != nil {
					t.Fatal(err)
				}
			}
			recovered, err := controller.Load(context.Background(), "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if recovered.StateRevision != before.StateRevision+1 || recovered.EvidenceSequence != before.EvidenceSequence+1 ||
				len(recovered.Facts.Assessments) != len(before.Facts.Assessments)+1 ||
				len(recovered.Facts.Coverage[0].ReopenLineage) != 1 || recovered.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateOpen ||
				!reflect.DeepEqual(recovered.OperationReceipts, before.OperationReceipts) || !reflect.DeepEqual(recovered.ExecutionBudget, before.ExecutionBudget) {
				t.Fatal("recovery duplicated settlement, changed receipt/budget, or granted terminal credit")
			}
			attempts := repository.saveAttempts
			if _, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2"); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("duplicate assessment accepted: %v", err)
			}
			if repository.saveAttempts != attempts {
				t.Fatal("duplicate performed CAS")
			}
		})
	}
}

// Explicit typed receipt/current-read fixture, not a real capability query or
// public drive. Its versioned read fence is what makes the same-CAS regression
// fail if settlement incorrectly re-evaluates the incremented evidence window.
func ownerFactVersionedAssessmentFixture(t *testing.T) (*RuntimeController, *controllerRepository) {
	t.Helper()
	reads := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: "definition-v2", Entries: []runtimecontract.RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: "typed-owner/ws-v2", ResourceKind: "workspace_target", ResourceID: "target", Revision: "historical-input", DependencyHash: "read-selector"}}}).WithHash()
	current := *runtimecontract.CloneRuntimeOwnerRevisionSet(&reads)
	current.Entries[0].Revision = "actual-current"
	current = current.WithHash()
	pair := &runtimecontract.RuntimeEvidenceApplicabilityDependencies{ProtocolRef: runtimecontract.RuntimeEvidenceApplicabilityDependenciesProtocol, OwnerRevisionSet: current}
	controller, repository, _ := prepareOwnerFactAssessmentWithDependencyPair(t, false, nil, &reads, pair, runtimecontract.RuntimeBlockKindPatchApply)
	return controller, repository
}
