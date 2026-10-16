package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// Explicit read-only local diagnostic. It cannot admit, drive or mutate a Run.
func TestRuntimeLineageArchiveDiagnostic(t *testing.T) {
	path := os.Getenv("WORKORCH_LINEAGE_DIAGNOSTIC_ARCHIVE")
	if path == "" {
		t.Skip("explicit archived run.json required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState RuntimeStateRecord }
	if err := json.Unmarshal(body, &runs); err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		r := run.RuntimeState
		s := RuntimeLineageAssertionSources{Definition: r.Definition, Facts: r.Facts, Work: r.ImplementationWork, Semantic: r.SemanticPlanning, Active: r.ActiveDispatch, Completed: r.CompletedDispatches, Receipts: r.OperationReceipts, Governance: r.OperationGovernanceBindings, Budget: r.ExecutionBudget, EvidenceSequence: r.EvidenceSequence, Resolutions: r.GovernanceResolutions, Attempts: r.CandidateAttempts, Sessions: r.CandidateRequestSessions, ProcessedEventIDs: r.ProcessedEventIDs, ProofLineage: r.CoverageProofLineage, Confirmations: r.RepairSurfaceConfirmations, Reconciliation: r.Reconciliation}
		for _, b := range r.Facts.Focus.CoverageEvidenceBindings {
			t.Logf("repair preconditions: binding=%v budget=%v operations=%v confirmations=%v", b.Validate(), s.Budget.Validate(), runtimeValidateLineageOperations(s), ValidateRuntimeRepairSurfaceConfirmationRecords(s.Confirmations))
			for _, d := range s.Completed {
				if d.RepairCommitment == nil {
					continue
				}
				f, _, _, ok := runtimeConfirmedBindingRepairSource(b, s, d)
				t.Logf("repair %s binding=%t source=%t commitment=%v origin=%t baseline=%s repairEvidence=%s", d.ID, RuntimeCoverageEvidenceBindingProjectionsEqual(d.CoverageBindings, []CoverageEvidenceBinding{b}), ok, ValidateRuntimeDispatchRepairCommitmentProjection(d.RepairCommitmentHash, d.RepairCommitment), d.RepairCommitment.OriginDispatchID == d.ID, f.Revision, d.Transaction.EvidenceRevision)
				last := f.Revision
				for _, op := range runtimeCandidateAdmissionOperations(d.Candidate) {
					fact, receipt, valid := runtimeLineageOperationFactFromLedgers(d, op, s, true, true, false)
					t.Logf("repair member valid=%t changed=%t durable=%t later=%t credit=%d", valid, receipt.Changed, RuntimeEffectIsDurable(receipt.ObservedEffectScope), EvidenceRevisionAfter(fact.Revision, f.Revision), len(fact.CoverageWorkItemIDs))
					if EvidenceRevisionAfter(fact.Revision, last) {
						last = fact.Revision
					}
				}
				t.Logf("fresh validation=%t", runtimeConfirmedBindingFreshValidation(b, s, *d.RepairCommitment, last, r.Facts.EvidenceRevision))
			}
			_, err := runtimeExpectedLineageAssertions(b, s)
			t.Logf("binding %s source gate=%v ledger gate=%v", b.ID, err, ValidateRuntimeEvidenceLedgerSequence(s.EvidenceSequence, s.Facts, s.Active, s.Completed, s.Resolutions))
			for _, a := range b.LineageAssertions {
				if a.Rule.ConsumptionPoint != RuntimeProofPhaseExecution {
					continue
				}
				proof := b
				if a.Rule.ExecutionSourceContractRef != "" {
					proof, err = runtimeExecutionProofSourceBinding(a, s)
					if err != nil {
						t.Logf("rule %s source=%v", a.Rule.Ref, err)
						continue
					}
				}
				satisfied := false
				switch a.Rule.Kind {
				case RuntimeLineageCompletedWorkRounds:
					satisfied = runtimeCompletedWorkRoundsSatisfied(proof.CoverageWorkItemID, a.Rule.MinCompletedRounds, s, r.Facts.EvidenceRevision)
				case RuntimeLineageRepairThenFreshValidation:
					satisfied = runtimeConfirmedBindingRepairSatisfied(proof, s, r.Facts.EvidenceRevision)
				}
				t.Logf("rule %s satisfied=%t independent-review=%t", a.Rule.Ref, satisfied, runtimeFreshBindingInputReview(proof, s.Semantic))
			}
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("read-only diagnostic changed archive", err)
	}
}
