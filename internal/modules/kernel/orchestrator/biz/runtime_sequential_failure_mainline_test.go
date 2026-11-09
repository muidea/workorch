package orchestrator

import (
	"slices"
	"strings"
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func sequentialProjectPurposeBootstrap(t *testing.T, b runtimecontract.RuntimeBootstrap) runtimecontract.RuntimeBootstrap {
	t.Helper()
	// The fixture explicitly declares a profile deliverable -> ordinary DoD
	// purpose before startup. A cross-DoD purpose deliberately forbids an
	// implementation-work policy on its acceptance endpoint.
	policy := b.DefinitionFocuses[1].ImplementationWorkPolicy
	b.DefinitionFocuses[1].ImplementationWorkPolicy = nil
	b.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[1].ImplementationWorkPolicy = nil
	b = runtimeTestWithHistoricalPurposeMapping(t, b, "cwi-1", "cwi-2")
	b.DefinitionFocuses[1].ImplementationWorkPolicy = policy
	p := &b.Activation.ExecutionContract.ProfileContracts[0]
	p.EvidenceRequirements[1].ImplementationWorkPolicy = policy
	history := &b.CoverageItems[0]
	history.SourceScope = "deliverable"
	p.EvidenceRequirements[0].Scope = history.SourceScope
	p.DeliverableExpectations = []runtimecontract.RuntimeDeliverableExpectation{{ID: history.SourceID, Required: true, Description: history.TargetText, RequiredEvidencePurposeContractRef: runtimecontract.RuntimeProfileDeliverableEvidencePurpose}}
	i := b.Definition.SourceObligations.Clone()
	i.Sources = slices.DeleteFunc(i.Sources, func(v runtimecontract.RuntimeDefinitionSource) bool { return v.CandidateItemRef == "cwi-1" })
	i.Requirements = slices.DeleteFunc(i.Requirements, func(v runtimecontract.RuntimeSourceRequirement) bool { return v.ID == "requirement/cwi-1" })
	i.Obligations = slices.DeleteFunc(i.Obligations, func(v runtimecontract.RuntimeSourceObligation) bool { return v.ID == "obligation/cwi-1" })
	i.Verifications = slices.DeleteFunc(i.Verifications, func(v runtimecontract.RuntimeSourceVerification) bool { return v.ID == "verification/cwi-1" })
	i.EvidencePurposeMappings = []runtimecontract.RuntimeEvidencePurposeMapping{{
		ContractRef:       runtimecontract.RuntimeProfileDeliverableEvidencePurpose,
		HistoricalItemRef: runtimecontract.RuntimeProfileDeliverablePurposeRef(p.ID, history.SourceID), AcceptanceItemRef: "cwi-2",
		HistoricalProfileID: p.ID, HistoricalDeliverableID: history.SourceID, HistoricalText: history.TargetText,
		HistoricalProfileHash: runtimecontract.StableRuntimeProfileContractsHash([]runtimecontract.RuntimeProfileContract{*p}),
	}}
	i = i.WithHash()
	b.Definition.SourceObligations = &i
	items, err := runtimecontract.CompileRuntimeEvidencePurposeMappingCoverage(i, b.CoverageItems)
	if err != nil {
		t.Fatal(err)
	}
	for n := range items {
		items[n].SourceObligationChecks, err = runtimecontract.CompileRuntimeSourceObligationChecks(i, items[n])
		if err != nil {
			t.Fatal(err)
		}
	}
	b.CoverageItems = items
	b = runtimeTestBootstrapWithBindings(b)
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	return b
}

// One Run crosses different real failures; only provider documents/candidates
// are scripted. File/Shell, receipts, review sources, assessments, CAS and the
// terminal owners remain real. No diagnostic, repair commitment or credit is
// injected to move the fixture to its next phase.
func TestDriveRuntimeSequentialFailuresToAcceptanceAndRecovery(t *testing.T) {
	options := validationInputReviewMainlineOptions{
		Adapter:         shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic,
		ValidationCount: 1, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol,
		DynamicProjectObservations: true, SequentialProjectFailures: true,
	}
	baseline := runValidationInputReviewMainline(t, "input_revision_after_failure", "", options)
	t.Run("result-response-loss", func(t *testing.T) {
		if calls := runValidationInputReviewMainline(t, "input_revision_after_failure", "after_result_persistence", options); calls != baseline {
			t.Fatalf("sequence recovery repeated a consumed provider request: %d != %d", calls, baseline)
		}
	})
}

func sequentialProjectRepairBodies(t *testing.T, ordinal int) (string, string) {
	t.Helper()
	value := func(n string) string { return "package fixture\nfunc DeliveredValue() int { return " + n + " }\n" }
	brokenBuild := strings.Replace(value("4"), "package fixture\n", "package fixture\nimport \"strings\"\n", 1)
	switch ordinal {
	case 1:
		return value("3"), brokenBuild
	case 2:
		return brokenBuild, value("5")
	case 3:
		return value("5"), value("4")
	default:
		t.Fatalf("unexpected extra repair proposal: %d", ordinal)
		return "", ""
	}
}

func assertSequentialProjectFailureClosure(t *testing.T, state runtimecontract.RuntimeStateRecord, repairs, reviews int) {
	t.Helper()
	if err := state.Validate(); err != nil {
		t.Fatal("sequence lost persisted owner invariants", err)
	}
	if repairs != 3 || reviews != 5 || len(state.GovernanceRequests) != 0 || len(state.ImplementationWork.Rounds) != 1 {
		t.Fatalf("sequence omitted or repeated a phase: repairs=%d reviews=%d rounds=%d tickets=%d", repairs, reviews, len(state.ImplementationWork.Rounds), len(state.GovernanceRequests))
	}
	round := state.ImplementationWork.Rounds[0]
	if round.ScopeRequest.InputRevision == nil || round.CompletedReceiptSetHash == "" || runtimecontract.ValidateRuntimeInputRevisionSource(round.ScopeRequest.InputRevision, state.SemanticPlanning) != nil {
		t.Fatal("sequence lacks the exact protected input-maintenance source/effect")
	}
	var validations []runtimecontract.RuntimeOperationReceipt
	receipts := map[string]runtimecontract.RuntimeOperationReceipt{}
	for _, receipt := range state.OperationReceipts {
		hash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
		if _, exists := receipts[hash]; exists {
			t.Fatal("duplicate committed receipt")
		}
		receipts[hash] = receipt
		if observation := receipt.Evidence.Validation; observation != nil && observation.ExecutionConjunction != nil {
			validations = append(validations, receipt)
		}
	}
	if len(validations) != 5 {
		t.Fatalf("sequence needs four failed validations and one fresh all-of: %d", len(validations))
	}
	for index, receipt := range validations {
		c := receipt.Evidence.Validation.ExecutionConjunction
		accepted := index == 4
		if c.Validate() != nil || c.Complete != accepted || (receipt.Outcome == runtimecontract.RuntimeOperationOutcomeAccepted) != accepted {
			t.Fatalf("failure/proof was reclassified at validation %d", index+1)
		}
		for _, stage := range c.Stages {
			if stage.Stdout.Storage == nil || stage.Stderr.Storage == nil || stage.Stdout.Storage.Reference == "" || stage.Stderr.Storage.Reference == "" || stage.Stdout.Validate() != nil || stage.Stderr.Validate() != nil {
				t.Fatal("sequence lost the original output publication")
			}
		}
		if accepted {
			if len(c.Stages) != 4 {
				t.Fatal("fresh proof omitted an all-of stage")
			}
			continue
		}
		if receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil || receipt.Evidence.Validation.ProofClass != "" {
			t.Fatal("failed validation lost its original diagnostic or gained credit")
		}
		diagnostic := receipt.Feedback.OriginalDiagnostic
		if index == 2 {
			if len(c.Stages) != 1 || c.Stages[0].Identity.ID != "project-build" || !c.Stages[0].KnownFailure || diagnostic.ValidationDiagnostic == nil || diagnostic.ValidationDiagnostic.OwnerCode != "source_compile_failed" {
				t.Fatalf("third validation must preserve the real build failure: %+v", diagnostic)
			}
		} else if len(c.Stages) < 3 || c.Stages[1].Identity.ID != "project-test" || !c.Stages[1].KnownFailure || diagnostic.ValidationFailureObservation == nil {
			t.Fatalf("native business failure %d lacks independent attribution", index+1)
		}
	}
	// Body/value 3 -> 5 is a new failure, despite unchanged validation input.
	firstBody, newBody := "", ""
	for _, check := range validations[1].Evidence.Validation.CheckFailures {
		firstBody += check.AssertionText
	}
	for _, check := range validations[3].Evidence.Validation.CheckFailures {
		newBody += check.AssertionText
	}
	if !strings.Contains(firstBody, "actual=3") || !strings.Contains(newBody, "actual=5") || firstBody == newBody {
		t.Fatal("new implementation failure was replaced by the old conclusion")
	}
	reviewed := map[string]bool{}
	for _, generation := range state.SemanticPlanning.AllHistory() {
		source := generation.Snapshot.FailureInputReview
		if source == nil || (generation.Reason != runtimecontract.RuntimeSemanticPlanningRetirementInputRevision && generation.Reason != runtimecontract.RuntimeSemanticPlanningRetirementFailureInputReviewed) {
			continue
		}
		if source.Validate() != nil || reviewed[source.Feedback.ReceiptHash] {
			t.Fatal("same failed input/receipt was judged twice or lost source integrity")
		}
		receipt, found := receipts[source.Feedback.ReceiptHash]
		if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected {
			t.Fatal("review no longer consumes the exact original rejected receipt")
		}
		want := runtimecontract.RuntimeValidationInputConsistent
		if generation.Reason == runtimecontract.RuntimeSemanticPlanningRetirementInputRevision {
			want = runtimecontract.RuntimeValidationInputConflict
		}
		matched := 0
		for _, document := range generation.Snapshot.BatchDocuments {
			if document.BatchID == generation.Snapshot.ActiveBatchID {
				for _, proposal := range document.Proposals {
					if report := proposal.ValidationInputReview; report != nil && report.SubjectHash == source.SubjectHash && runtimecontract.RuntimeValidationInputReviewConclusion(*report) == want {
						matched++
					}
				}
			}
		}
		if matched != 1 {
			t.Fatal("failed input has no unique formal report for its affected scope")
		}
		reviewed[source.Feedback.ReceiptHash] = true
	}
	for _, index := range []int{0, 1, 3} {
		if !reviewed[runtimecontract.StableRuntimeOperationReceiptHash(validations[index])] {
			t.Fatal("a new native failure bypassed independent input review")
		}
	}
	if len(reviewed) != 3 {
		t.Fatal("build-only failure manufactured a native assertion review")
	}
	commitments := map[string]bool{}
	for _, dispatch := range state.CompletedDispatches {
		if commitment := dispatch.RepairCommitment; commitment != nil {
			if commitments[commitment.SourceReceiptHash] || dispatch.State != runtimecontract.RuntimeDispatchStateCommitted || len(dispatch.OperationExecutions) != 1 {
				t.Fatal("repair dispatch duplicated its source or lost its execution")
			}
			execution := dispatch.OperationExecutions[0]
			receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(state.OperationReceipts, execution.ExecutionKey, execution.Fence)
			if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !receipt.Changed || receipt.BeforeHash == receipt.AfterHash || execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
				t.Fatal("repair has no unique File effect/receipt/version edge")
			}
			commitments[commitment.SourceReceiptHash] = true
		}
	}
	for _, index := range []int{1, 2, 3} {
		if !commitments[runtimecontract.StableRuntimeOperationReceiptHash(validations[index])] {
			t.Fatal("implementation/build repair borrowed a different failure")
		}
	}
	if len(commitments) != 3 || len(state.Facts.Assessments) == 0 || !runtimeTerminalDeliveryFullyAcknowledged(runtimeAggregateFromRecord(state)) {
		t.Fatal("sequence omitted fresh assessment or ordered three-owner delivery")
	}
	freshProof := false
	for _, dispatch := range state.CompletedDispatches {
		if dispatch.AssessmentAuthorityInput == nil || dispatch.AssessmentAuthorityInput.Purpose != runtimecontract.RuntimeAssessmentPurposeDefinitionProof || dispatch.AssessmentAdjudication == nil {
			continue
		}
		for _, proof := range dispatch.AssessmentAdjudication.Proofs {
			for _, fact := range state.Facts.Evidence.Accepted {
				if proof.EvidenceFactID == fact.ID && proof.EvidenceKind == runtimecontract.EvidenceKindValidation && fact.OperationID == validations[4].OperationID && fact.Validation != nil && fact.Validation.ExecutionConjunction != nil && fact.Validation.ExecutionConjunction.Complete {
					freshProof = true
				}
			}
		}
	}
	if !freshProof {
		t.Fatal("definition assessment did not consume the fresh final ordinary validation")
	}
	for _, item := range state.Facts.Coverage {
		if item.State != runtimecontract.RuntimeCoverageItemStateSatisfied {
			t.Fatal("sequence did not satisfy its complete all-of fixture")
		}
	}
	// Fresh owner reload must retain the three distinct judged failure causes.
	if err := runtimeAggregateFromRecord(state).Validate(); err != nil {
		t.Fatal("recovery rebuilt or lost the submitted authority", err)
	}
}
