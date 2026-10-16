package runtime

import (
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Called from the real Shell -> File repair -> Shell proof fixture. The
// historical source below is the actual admitted owner observation, not a
// synthetic declaration or an independently invented current-read fact.
func testConfirmedRepairValidationConsumptionHistory(t *testing.T, original RuntimeAggregate, directive runtimecontract.RuntimeRepairDirective) {
	t.Helper()
	advanced := original.CloneRaw()
	advanced.EvidenceSequence++
	advanced.Facts.EvidenceRevision = nextEvidenceRevision(advanced.EvidenceSequence)
	before := advanced.CloneRaw()
	if repairDirectiveEvidenceSatisfied(advanced.Facts, directive, advanced.CoverageProofLineage) {
		t.Fatal("historical observation satisfied a new current sampling window")
	}
	if !confirmedRepairFreshValidationConsumed(advanced, directive) {
		t.Fatal("new evidence revision resurrected a consumed repair continuation")
	}
	if err := validateConfirmedRepairFreshValidationContinuation(advanced); err != nil {
		t.Fatalf("historical continuation consumption: %v", err)
	}
	if !reflect.DeepEqual(before, advanced) {
		t.Fatal("historical consumption query modified its source")
	}
	restored := advanced.CloneRaw()
	value := directive.Clone()
	restored.Facts.RepairDirective = &value
	if err := validateConfirmedRepairFreshValidationContinuation(restored); err == nil {
		t.Fatal("restored already consumed directive passed reverse proof")
	}
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"missing observations": func(a *RuntimeAggregate) { a.Facts.EvidenceApplicabilityObservations = nil },
		"missing receipts":     func(a *RuntimeAggregate) { a.OperationReceipts = nil },
		"source hash drift": func(a *RuntimeAggregate) {
			for n := range a.Facts.EvidenceApplicabilityObservations {
				a.Facts.EvidenceApplicabilityObservations[n].Request.SourceEvidenceHash = "not-original-source"
			}
		},
		"before repair barrier": func(a *RuntimeAggregate) {
			for n := range a.Facts.EvidenceApplicabilityObservations {
				a.Facts.EvidenceApplicabilityObservations[n].Request.EvidenceRevision = directive.SupersedesEvidenceRevision
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			a := advanced.CloneRaw()
			mutate(&a)
			if confirmedRepairFreshValidationConsumed(a, directive) {
				t.Fatal("invalid historical source consumed continuation")
			}
		})
	}
}
