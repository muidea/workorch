package runtime

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFailureClassDispositionTaxonomy(t *testing.T) {
	if FailureClassDisposition(runtimecontract.FailureClassCASConflict) != FailureDispositionCASReload {
		t.Fatal("CAS conflict must reload")
	}
	if FailureClassIsBusinessTerminal(runtimecontract.FailureClassCASConflict) {
		t.Fatal("CAS conflict must not terminal business run")
	}
	if FailureClassDisposition(runtimecontract.FailureClassOwnerTransientUnavailable) != FailureDispositionOwnerTransient ||
		FailureClassIsBusinessTerminal(runtimecontract.FailureClassOwnerTransientUnavailable) {
		t.Fatal("persistence owner transient must remain nonterminal and distinct from operation recovery")
	}
	if FailureClassDisposition(runtimecontract.FailureClassProviderAuthentication) != FailureDispositionConfigurationGov {
		t.Fatal("provider auth must be configuration governance")
	}
	if FailureClassDisposition(runtimecontract.FailureClassOperationOutcomeUnknown) != FailureDispositionBoundedFail {
		t.Fatal("outcome unknown without a separate dependency must bounded-fail")
	}
	if FailureClassDisposition(runtimecontract.FailureClassExecutionRejected) != FailureDispositionSameFocusRepair {
		t.Fatal("execution rejected must same-focus repair")
	}
	if !FailureClassIsBusinessTerminal(runtimecontract.FailureClassRuntimeInvariantDefect) {
		t.Fatal("invariant defect is business terminal")
	}
}
