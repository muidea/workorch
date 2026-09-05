package runtime

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDispatchCommitRecordsCoverageProofLineageExactlyOnce(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: stateChangeAggregate(t)}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, stateChangeCandidate())
	evidence := ExecutionEvidence{
		EventID:             "event-lineage-source",
		WorkspaceID:         sealed.Facts.WorkspaceID,
		RunID:               sealed.Facts.RunID,
		DispatchID:          sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID:         runtimeTestActiveOperationID(t, sealed, 0),
		Kind:                runtimecontract.EvidenceKindStateChange,
		Status:              ExecutionEvidenceStatusAccepted,
		SourceRef:           "patch/lineage-source",
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Quality: common.EvidenceQualityValid,
		},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	canonicalEvidenceID := common.EvidenceRecordID("runtime-receipt|" + runtimecontract.StableRuntimeOperationReceiptHash(*evidence.Receipt))
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatalf("commit accepted receipt: %v", err)
	}
	if len(committed.Aggregate.CoverageProofLineage) != 1 {
		t.Fatalf("source lineage rows = %+v", committed.Aggregate.CoverageProofLineage)
	}
	source := committed.Aggregate.CoverageProofLineage[0]
	if source.Kind != runtimecontract.RuntimeCoverageProofLineageKindSource ||
		source.SourceEvidenceID != canonicalEvidenceID ||
		source.SourceDispatchID != evidence.DispatchID || source.SourceOperationID != evidence.OperationID ||
		source.SourceReceiptHash == "" || source.SourceProofIdentity.Hash == "" {
		t.Fatalf("source lineage = %+v", source)
	}
	if got := committed.Aggregate.Facts.Evidence.Accepted[0].BindingRefs; len(got) != 1 || got[0] != source.SourceBindingRef {
		t.Fatalf("source evidence binding refs = %v, source=%+v", got, source)
	}

	stateRevision := committed.Aggregate.StateRevision
	duplicate, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatalf("duplicate accepted receipt: %v", err)
	}
	if duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != stateRevision ||
		len(duplicate.Aggregate.CoverageProofLineage) != 1 {
		t.Fatalf("duplicate receipt rewrote source lineage: %+v", duplicate.Aggregate)
	}
}

func TestCoverageProofLineageRequiresCurrentCWILedgerBindingForCredit(t *testing.T) {
	binding := coverageProofLineageTestBinding("binding/current", "main.go")
	aggregate, fact, _ := coverageProofLineageTestAggregate(binding)

	// A focus binding alone is not authority. A legacy aggregate cannot use an
	// otherwise exact evidence fact to earn CWI credit.
	if runtimeEvidenceAppliesToCoverageWorkItemWithLineage(aggregate.Facts, fact, "cwi-lineage", nil) {
		t.Fatal("schema-less aggregate evidence received CWI credit")
	}
	if runtimeEvidenceAppliesToCoverageIDsWithLineage(aggregate.Facts, fact, []common.CoverageWorkItemID{"cwi-lineage"}, nil) {
		t.Fatal("schema-less aggregate evidence passed the CWI set attribution gate")
	}

	// The same exact fact becomes eligible only after the test fixture projects
	// the sealed focus into the current canonical CWI row.
	aggregate.Facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{{
		ID: "cwi-lineage", State: runtimecontract.RuntimeCoverageItemStateOpen,
	}}
	runtimeTestSynchronizeCanonicalCWILedger(t, &aggregate)
	if !runtimeEvidenceAppliesToCoverageWorkItemWithLineage(aggregate.Facts, fact, "cwi-lineage", nil) {
		t.Fatal("exact current CWI binding did not receive credit")
	}
	if !runtimeEvidenceMatchesCoverageBindingWithLineage(aggregate.Facts, aggregate.Facts.Focus, "cwi-lineage", fact, nil) {
		t.Fatal("exact current CWI binding did not pass the assessment attribution gate")
	}

	// Removing the persisted current binding must close both attribution gates,
	// even though the focus/evidence still carry the old textual binding ref.
	aggregate.Facts.Focus.CoverageEvidenceBindings = nil
	aggregate.Facts.Coverage[0].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateUnbound
	aggregate.Facts.Coverage[0].CurrentBindingRef = nil
	aggregate.Facts.Coverage[0].BindingLineage = nil
	aggregate.Facts.ReconciliationContract = runtimecontract.RuntimeCWIReconciliationContract{}
	if runtimeEvidenceAppliesToCoverageWorkItemWithLineage(aggregate.Facts, fact, "cwi-lineage", nil) {
		t.Fatal("unbound CWI evidence received credit")
	}
	if runtimeEvidenceMatchesCoverageBindingWithLineage(aggregate.Facts, aggregate.Facts.Focus, "cwi-lineage", fact, nil) {
		t.Fatal("unbound CWI evidence passed the assessment attribution gate")
	}
}

func TestCoverageProofLineageEquivalentResealRetainsOnlyMatchingProofKind(t *testing.T) {
	oldBinding := coverageProofLineageTestBinding("binding/old", "main.go")
	currentBinding := coverageProofLineageTestBinding("binding/current", "main.go")
	aggregate, fact, receipt := coverageProofLineageTestAggregate(oldBinding)
	coverageProofLineageSynchronizeLedger(t, &aggregate)

	if err := appendAcceptedCoverageProofLineage(&aggregate, fact, receipt); err != nil {
		t.Fatalf("append source lineage: %v", err)
	}
	if len(aggregate.CoverageProofLineage) != 1 {
		t.Fatalf("source lineage rows = %+v", aggregate.CoverageProofLineage)
	}
	aggregate.ActiveDispatch = nil
	aggregate.Facts.Focus = coverageProofLineageTestFocus(currentBinding)
	coverageProofLineageSynchronizeLedger(t, &aggregate)
	if err := resolveCoverageProofLineageForFocus(&aggregate, aggregate.Facts.Focus); err != nil {
		t.Fatalf("resolve equivalent lineage: %v", err)
	}
	if err := runtimecontract.ValidateRuntimeCoverageProofLineage(aggregate.CoverageProofLineage); err != nil {
		t.Fatalf("validate equivalent lineage: %v", err)
	}
	if len(aggregate.CoverageProofLineage) != 2 || aggregate.CoverageProofLineage[1].Disposition != runtimecontract.RuntimeCoverageProofLineageDispositionEquivalent {
		t.Fatalf("equivalent lineage rows = %+v", aggregate.CoverageProofLineage)
	}
	if got := aggregate.Facts.Evidence.Accepted[0].BindingRefs; len(got) != 1 || got[0] != oldBinding.ID {
		t.Fatalf("source evidence binding was rewritten: %+v", got)
	}
	if !runtimeEvidenceAppliesToCoverageWorkItemWithLineage(
		aggregate.Facts,
		aggregate.Facts.Evidence.Accepted[0],
		"cwi-lineage",
		aggregate.CoverageProofLineage,
	) {
		t.Fatal("equivalent resolution did not retain state-change credit")
	}
	missing := missingEvidenceKinds(aggregate.Facts, aggregate.CoverageProofLineage)
	if len(missing) != 1 || missing[0] != runtimecontract.EvidenceKindValidation {
		t.Fatalf("equivalent state proof missing kinds = %v, want validation only", missing)
	}
	before := len(aggregate.CoverageProofLineage)
	if err := resolveCoverageProofLineageForFocus(&aggregate, aggregate.Facts.Focus); err != nil {
		t.Fatalf("repeat equivalent resolution: %v", err)
	}
	if len(aggregate.CoverageProofLineage) != before {
		t.Fatalf("duplicate re-seal appended a resolution: before=%d after=%d", before, len(aggregate.CoverageProofLineage))
	}
}

func TestCoverageProofLineageReSealDoesNotTreatSameBindingIDWithNewHashAsExactCredit(t *testing.T) {
	oldBinding := coverageProofLineageTestBinding("binding/stable", "main.go")
	currentBinding := oldBinding.Clone()
	currentBinding.PrerequisiteRefs = []string{"catalog-snapshot-current"}
	currentBinding = currentBinding.Normalized()
	currentBinding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(currentBinding)
	if currentBinding.ID != oldBinding.ID || currentBinding.Hash == oldBinding.Hash {
		t.Fatalf("test binding did not retain ref while changing hash: old=%+v current=%+v", oldBinding, currentBinding)
	}
	aggregate, fact, receipt := coverageProofLineageTestAggregate(oldBinding)
	coverageProofLineageSynchronizeLedger(t, &aggregate)
	if err := appendAcceptedCoverageProofLineage(&aggregate, fact, receipt); err != nil {
		t.Fatalf("append source lineage: %v", err)
	}
	aggregate.ActiveDispatch = nil
	aggregate.Facts.Focus = coverageProofLineageTestFocus(currentBinding)
	coverageProofLineageSynchronizeLedger(t, &aggregate)
	if runtimeEvidenceAppliesToCoverageWorkItemWithLineage(aggregate.Facts, fact, "cwi-lineage", aggregate.CoverageProofLineage) {
		t.Fatal("same binding ID with a new hash bypassed lineage resolution")
	}
	if runtimeEvidenceMatchesCoverageBindingWithLineage(aggregate.Facts, aggregate.Facts.Focus, "cwi-lineage", fact, aggregate.CoverageProofLineage) {
		t.Fatal("assessment binding gate bypassed lineage resolution for a reused binding ID")
	}
	if err := resolveCoverageProofLineageForFocus(&aggregate, aggregate.Facts.Focus); err != nil {
		t.Fatalf("resolve equivalent re-seal: %v", err)
	}
	if !runtimeEvidenceAppliesToCoverageWorkItemWithLineage(aggregate.Facts, fact, "cwi-lineage", aggregate.CoverageProofLineage) {
		t.Fatal("equivalent resolution did not restore current proof credit")
	}
	if !runtimeEvidenceMatchesCoverageBindingWithLineage(aggregate.Facts, aggregate.Facts.Focus, "cwi-lineage", fact, aggregate.CoverageProofLineage) {
		t.Fatal("equivalent resolution did not restore assessment binding eligibility")
	}
}

func TestCoverageProofLineageIncompatibleResealRecordsGapWithoutCredit(t *testing.T) {
	oldBinding := coverageProofLineageTestBinding("binding/old", "main.go")
	incompatibleBinding := coverageProofLineageTestBinding("binding/current", "other.go")
	aggregate, fact, receipt := coverageProofLineageTestAggregate(oldBinding)
	coverageProofLineageSynchronizeLedger(t, &aggregate)
	if err := appendAcceptedCoverageProofLineage(&aggregate, fact, receipt); err != nil {
		t.Fatalf("append source lineage: %v", err)
	}
	aggregate.ActiveDispatch = nil
	aggregate.Facts.Focus = coverageProofLineageTestFocus(incompatibleBinding)
	coverageProofLineageSynchronizeLedger(t, &aggregate)
	if err := resolveCoverageProofLineageForFocus(&aggregate, aggregate.Facts.Focus); err != nil {
		t.Fatalf("resolve incompatible lineage: %v", err)
	}
	if len(aggregate.CoverageProofLineage) != 2 {
		t.Fatalf("incompatible lineage rows = %+v", aggregate.CoverageProofLineage)
	}
	resolution := aggregate.CoverageProofLineage[1]
	if resolution.Disposition != runtimecontract.RuntimeCoverageProofLineageDispositionIncompatible || resolution.Gap == nil ||
		resolution.Gap.Code != runtimecontract.RuntimeCoverageProofLineageIncompatibleGapCode ||
		resolution.Gap.FirstDifferingFieldCategory != "target" {
		t.Fatalf("incompatible resolution = %+v", resolution)
	}
	if runtimeEvidenceAppliesToCoverageWorkItemWithLineage(
		aggregate.Facts,
		aggregate.Facts.Evidence.Accepted[0],
		"cwi-lineage",
		aggregate.CoverageProofLineage,
	) {
		t.Fatal("incompatible resolution granted synthetic credit")
	}
	missing := missingEvidenceKinds(aggregate.Facts, aggregate.CoverageProofLineage)
	if len(missing) != 2 || missing[0] != runtimecontract.EvidenceKindStateChange || missing[1] != runtimecontract.EvidenceKindValidation {
		t.Fatalf("incompatible proof missing kinds = %v", missing)
	}
}

func TestCoverageProofLineageEquivalentResolutionUnlocksOnlyCurrentTargetAndRepairGates(t *testing.T) {
	oldBinding := coverageProofLineageTestSelectorBinding("binding/old", "main.go")
	currentBinding := coverageProofLineageTestSelectorBinding("binding/current", "main.go")
	aggregate, fact, receipt := coverageProofLineageTestAggregate(oldBinding)
	coverageProofLineageSynchronizeLedger(t, &aggregate)
	fact.TargetRefs = []string{"main.go"}
	fact.Revision = "runtime-evidence-00000000000000000001"
	aggregate.Facts.Evidence.Accepted = []runtimecontract.EvidenceFact{fact}
	if err := appendAcceptedCoverageProofLineage(&aggregate, fact, receipt); err != nil {
		t.Fatalf("append source lineage: %v", err)
	}
	aggregate.ActiveDispatch = nil
	aggregate.Facts.Focus = coverageProofLineageTestFocus(currentBinding)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	coverageProofLineageSynchronizeLedger(t, &aggregate)
	directive := runtimecontract.RuntimeRepairDirective{
		ID:                    "repair-lineage",
		FocusID:               aggregate.Facts.Focus.ID,
		RequiredState:         runtimecontract.ConvergenceStateNeedStateChange,
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-lineage"},
		TargetRefs:            []string{"main.go"},
	}
	base := runtimecontract.RuntimeDecision{Focus: aggregate.Facts.Focus}
	if hasCurrentFocusTargetBoundStateChangeEvidence(aggregate.Facts, aggregate.CoverageProofLineage) {
		t.Fatal("source binding evidence was accepted before a current-binding resolution")
	}
	if repairDirectiveEvidenceSatisfied(aggregate.Facts, directive, aggregate.CoverageProofLineage) {
		t.Fatal("source binding evidence satisfied a repair directive before a current-binding resolution")
	}
	if decision, blocked, err := targetGroundingDecision(base, aggregate.Facts, true, aggregate.CoverageProofLineage); err != nil || !blocked || decision.Terminal == nil || decision.Terminal.Code != "target_grounding_unresolved" {
		t.Fatalf("unresolved source binding bypassed target grounding: decision=%+v blocked=%t err=%v", decision, blocked, err)
	}

	if err := resolveCoverageProofLineageForFocus(&aggregate, aggregate.Facts.Focus); err != nil {
		t.Fatalf("resolve equivalent lineage: %v", err)
	}
	if !hasCurrentFocusTargetBoundStateChangeEvidence(aggregate.Facts, aggregate.CoverageProofLineage) {
		t.Fatal("equivalent resolution did not restore current target-bound evidence")
	}
	if !repairDirectiveEvidenceSatisfied(aggregate.Facts, directive, aggregate.CoverageProofLineage) {
		t.Fatal("equivalent resolution did not satisfy the matching repair directive")
	}
	if _, blocked, err := targetGroundingDecision(base, aggregate.Facts, true, aggregate.CoverageProofLineage); err != nil || blocked {
		t.Fatalf("equivalent resolution did not clear target grounding: blocked=%t err=%v", blocked, err)
	}
	aggregate.Facts.RepairDirective = &directive
	updated := syncRepairDirective(aggregate.Facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		Focus: aggregate.Facts.Focus,
	}, aggregate.CoverageProofLineage)
	if updated.RepairDirective == nil || updated.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("equivalent source proof did not advance repair handoff: %+v", updated.RepairDirective)
	}
}

func TestCoverageProofLineageIncompatibleResolutionCannotUnlockTargetOrRepairGates(t *testing.T) {
	oldBinding := coverageProofLineageTestSelectorBinding("binding/old", "main.go")
	incompatibleBinding := coverageProofLineageTestSelectorBinding("binding/current", "main.go")
	incompatibleBinding.Proofs[0].AllowedOperationKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
	incompatibleBinding = incompatibleBinding.Normalized()
	incompatibleBinding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(incompatibleBinding)
	aggregate, fact, receipt := coverageProofLineageTestAggregate(oldBinding)
	coverageProofLineageSynchronizeLedger(t, &aggregate)
	fact.TargetRefs = []string{"main.go"}
	fact.Revision = "runtime-evidence-00000000000000000001"
	aggregate.Facts.Evidence.Accepted = []runtimecontract.EvidenceFact{fact}
	if err := appendAcceptedCoverageProofLineage(&aggregate, fact, receipt); err != nil {
		t.Fatalf("append source lineage: %v", err)
	}
	aggregate.ActiveDispatch = nil
	aggregate.Facts.Focus = coverageProofLineageTestFocus(incompatibleBinding)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	coverageProofLineageSynchronizeLedger(t, &aggregate)
	if err := resolveCoverageProofLineageForFocus(&aggregate, aggregate.Facts.Focus); err != nil {
		t.Fatalf("resolve incompatible lineage: %v", err)
	}
	directive := runtimecontract.RuntimeRepairDirective{
		ID:                    "repair-lineage",
		FocusID:               aggregate.Facts.Focus.ID,
		RequiredState:         runtimecontract.ConvergenceStateNeedStateChange,
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-lineage"},
		TargetRefs:            []string{"main.go"},
	}
	if hasCurrentFocusTargetBoundStateChangeEvidence(aggregate.Facts, aggregate.CoverageProofLineage) {
		t.Fatal("incompatible source binding bypassed current target grounding")
	}
	if repairDirectiveEvidenceSatisfied(aggregate.Facts, directive, aggregate.CoverageProofLineage) {
		t.Fatal("incompatible source binding satisfied a current repair directive")
	}
	base := runtimecontract.RuntimeDecision{Focus: aggregate.Facts.Focus}
	if decision, blocked, err := targetGroundingDecision(base, aggregate.Facts, true, aggregate.CoverageProofLineage); err != nil || !blocked || decision.Terminal == nil || decision.Terminal.Code != "target_grounding_unresolved" {
		t.Fatalf("incompatible source binding bypassed target grounding: decision=%+v blocked=%t err=%v", decision, blocked, err)
	}
}

func TestCoverageProofLineageCandidateAdmissionRequiresEquivalentCurrentCredit(t *testing.T) {
	testCase := func(t *testing.T, currentTarget string, wantRejected bool) {
		t.Helper()
		oldBinding := coverageProofLineageTestBinding("binding/old", "main.go")
		currentBinding := coverageProofLineageTestBinding("binding/current", currentTarget)
		aggregate, fact, receipt := coverageProofLineageTestAggregate(oldBinding)
		coverageProofLineageSynchronizeLedger(t, &aggregate)
		fact.TargetRefs = []string{"main.go"}
		aggregate.Facts.Evidence.Accepted = []runtimecontract.EvidenceFact{fact}
		if err := appendAcceptedCoverageProofLineage(&aggregate, fact, receipt); err != nil {
			t.Fatalf("append source lineage: %v", err)
		}
		aggregate.ActiveDispatch = nil
		aggregate.Facts.Focus = coverageProofLineageTestFocus(currentBinding)
		coverageProofLineageSynchronizeLedger(t, &aggregate)
		aggregate.Facts.Evidence.Rejected = []runtimecontract.EvidenceFact{{
			ID: "rejected-state", Kind: runtimecontract.EvidenceKindStateChange,
			FocusID:             aggregate.Facts.Focus.ID,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-lineage"}, Revision: "runtime-evidence-00000000000000000002",
		}}
		if err := resolveCoverageProofLineageForFocus(&aggregate, aggregate.Facts.Focus); err != nil {
			t.Fatalf("resolve lineage: %v", err)
		}
		aggregate.Decision = runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange, Focus: aggregate.Facts.Focus}
		result := rejectCandidateMissingValidationGap(aggregate, runtimecontract.RuntimeCandidate{
			StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: "retry-state"}}}},
		})
		if wantRejected {
			if result.Accepted() || result.Failure == nil || result.Failure.Code != candidateAdmissionCodeActionEvidenceMismatch {
				t.Fatalf("equivalent current state proof did not reject redundant state candidate: %+v", result)
			}
			return
		}
		if !result.Accepted() {
			t.Fatalf("incompatible source proof incorrectly rejected current state candidate: %+v", result)
		}
	}
	t.Run("equivalent", func(t *testing.T) { testCase(t, "main.go", true) })
	t.Run("incompatible", func(t *testing.T) { testCase(t, "other.go", false) })
}

func coverageProofLineageTestAggregate(binding runtimecontract.CoverageEvidenceBinding) (RuntimeAggregate, runtimecontract.EvidenceFact, runtimecontract.RuntimeOperationReceipt) {
	focus := coverageProofLineageTestFocus(binding)
	fact := runtimecontract.EvidenceFact{
		ID:                  "evidence-lineage",
		Kind:                runtimecontract.EvidenceKindStateChange,
		DispatchID:          "dispatch-lineage",
		OperationID:         "operation-lineage",
		BindingRefs:         []string{binding.ID},
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-lineage"},
		Quality:             common.EvidenceQualityValid,
		AuthoritySnapshotID: "authority-lineage",
		EvidenceRefs:        []string{"evidence-ref-lineage"},
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey:        "execution-lineage",
		Fence:               1,
		OperationID:         "operation-lineage",
		Outcome:             runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		Changed:             true,
		TargetRefs:          []string{"main.go"},
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Quality: common.EvidenceQualityValid,
		},
	}
	return RuntimeAggregate{
		Facts: runtimecontract.RuntimeFacts{
			WorkspaceID: "ws-lineage", RunID: "run-lineage", DefinitionRevision: "definition-lineage",
			Focus:    focus,
			Evidence: runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{fact}},
		},
		SemanticPlanning: runtimecontract.RuntimeSemanticPlanningLedger{
			Phase:            runtimecontract.RuntimeSemanticPlanningSealed,
			Manifest:         &runtimecontract.RuntimeSemanticPlanManifest{Hash: "semantic-manifest-lineage"},
			LogicalRequestID: "semantic-request-lineage",
		},
		ActiveDispatch: &SealedDispatch{
			ID: "dispatch-lineage",
			OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{
				OperationID: "operation-lineage", ExecutionKey: receipt.ExecutionKey, Fence: receipt.Fence,
				State: runtimecontract.OperationExecutionEvidenceCommitted,
			}},
		},
	}, fact, receipt
}

// coverageProofLineageSynchronizeLedger models the SM-2 half of a re-seal in
// a direct unit fixture: focus projection and canonical CWI binding change
// together before proof-lineage resolution is evaluated.
func coverageProofLineageSynchronizeLedger(t *testing.T, aggregate *RuntimeAggregate) {
	t.Helper()
	if aggregate == nil {
		t.Fatal("nil coverage proof lineage aggregate")
	}
	if len(aggregate.Facts.Coverage) == 0 {
		aggregate.Facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{{
			ID: "cwi-lineage", State: runtimecontract.RuntimeCoverageItemStateOpen,
		}}
	}
	runtimeTestSynchronizeCanonicalCWILedger(t, aggregate)
}

func coverageProofLineageTestFocus(binding runtimecontract.CoverageEvidenceBinding) runtimecontract.CoverageFocus {
	return runtimecontract.CoverageFocus{
		ID: "focus-lineage", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-lineage"},
		RequiredEvidenceKinds:    []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{binding},
	}
}

func coverageProofLineageTestBinding(id, target string) runtimecontract.CoverageEvidenceBinding {
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: id, CoverageWorkItemID: "cwi-lineage", FocusID: "focus-lineage", DefinitionRevision: "definition-lineage",
		TargetRefs: []string{target}, DeclarationRef: "declaration-lineage",
		Proofs: []runtimecontract.CoverageEvidenceProof{
			{
				EvidenceKind:          runtimecontract.EvidenceKindStateChange,
				RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
				AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
			},
			{
				EvidenceKind:          runtimecontract.EvidenceKindValidation,
				RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
				AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
				ValidationRecipeID:    "recipe-lineage", ValidationProofRequirementHash: "runtime-validation-proof-requirement-test-lineage", RequiredCheckRefs: []string{"check-lineage"},
				ValidationCheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{{
					RequiredCheckRef: "check-lineage", ObservationProtocolRef: "test/lineage-validation-observation/v1", NativeCheckRef: "test/native/lineage/check",
				}},
			},
		},
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	return binding
}

func coverageProofLineageTestSelectorBinding(id, target string) runtimecontract.CoverageEvidenceBinding {
	binding := coverageProofLineageTestBinding(id, target)
	binding.TargetRefs = nil
	binding.TargetSelectorRef = "file/test-lineage-target"
	binding.TargetSelectorOwnerRef = runtimecontract.RuntimeModuleFile
	binding.TargetSelectorSubjectRef = "implementation"
	binding.TargetSelectorCandidateRefs = []string{target}
	binding = binding.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	return binding
}
