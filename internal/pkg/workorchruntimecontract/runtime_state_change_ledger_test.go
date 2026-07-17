package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeStateChangeLedgerApplyAcceptedEvidence(t *testing.T) {
	ledger := NewRuntimeStateChangeLedger(RuntimeRunStateChangeRequirement{
		Required:            true,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"},
	})
	if ledger.Satisfied {
		t.Fatalf("required ledger must start unsatisfied: %+v", ledger)
	}

	// unbound evidence cannot satisfy
	ledger = ledger.ApplyAcceptedStateChangeEvidence(EvidenceFact{
		ID: "e-unbound", Kind: EvidenceKindStateChange, Revision: "evidence-v2-1",
	}, []common.CoverageWorkItemID{"cwi-1"})
	if ledger.Satisfied {
		t.Fatalf("unbound evidence must not satisfy: %+v", ledger)
	}

	// unrelated coverage cannot satisfy
	ledger = ledger.ApplyAcceptedStateChangeEvidence(EvidenceFact{
		ID: "e-other", Kind: EvidenceKindStateChange, Revision: "evidence-v2-2",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-9"},
	}, []common.CoverageWorkItemID{"cwi-1", "cwi-2"})
	if ledger.Satisfied {
		t.Fatalf("unrelated coverage must not satisfy: %+v", ledger)
	}

	// validation evidence ignored
	ledger = ledger.ApplyAcceptedStateChangeEvidence(EvidenceFact{
		ID: "e-val", Kind: EvidenceKindValidation, Revision: "evidence-v2-3",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	}, []common.CoverageWorkItemID{"cwi-1"})
	if ledger.Satisfied {
		t.Fatalf("validation evidence must not satisfy state-change ledger: %+v", ledger)
	}

	// intersecting coverage satisfies
	ledger = ledger.ApplyAcceptedStateChangeEvidence(EvidenceFact{
		ID: "e-ok", Kind: EvidenceKindStateChange, Revision: "evidence-v2-4",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"},
	}, []common.CoverageWorkItemID{"cwi-1", "cwi-2"})
	if !ledger.Satisfied || len(ledger.EvidenceRefs) != 1 || ledger.EvidenceRefs[0] != "e-ok" {
		t.Fatalf("coverage-bound state-change should satisfy: %+v", ledger)
	}
	if !ledger.SatisfiesRequirement() {
		t.Fatal("SatisfiesRequirement should be true after accepted evidence")
	}
}

func TestRuntimeStateChangeLedgerOptionalIsSatisfied(t *testing.T) {
	ledger := NewRuntimeStateChangeLedger(RuntimeRunStateChangeRequirement{Required: false})
	if !ledger.SatisfiesRequirement() || !ledger.Satisfied {
		t.Fatalf("optional ledger must be satisfied: %+v", ledger)
	}
}
