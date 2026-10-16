package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// Original complete work validator, independent of the new session path.
func referenceSemanticWorkValidation(w RuntimeSemanticImplementationWork) error {
	maxOrdinal := w.Scope.Request.Policy.MaxRoundsPerCWI
	if w.PurposeChoiceOnly {
		maxOrdinal++
		if w.EffectAvailable != (w.Ordinal <= w.Scope.Request.Policy.MaxRoundsPerCWI) {
			return fmt.Errorf("semantic purpose vocabulary must reflect immutable remaining effect rounds")
		}
	} else if w.EffectAvailable {
		return fmt.Errorf("working semantic purpose cannot carry a purpose-choice availability arm")
	}
	if !implementationWorkExactRef(w.RoundHash) || w.PolicyHash != StableRuntimeImplementationWorkPolicyHash(w.Scope.Request.Policy) || w.Ordinal < 1 || w.Ordinal > maxOrdinal || (w.Ordinal == 1) != (w.PrecedingReceiptSetHash == "") {
		return fmt.Errorf("semantic implementation work requires exact policy and bounded round/receipt lineage")
	}
	if err := w.Scope.ValidateForRequest(w.Scope.Request); err != nil {
		return err
	}
	if w.Scope.Selection.Disposition != RuntimeTargetSelectionSelected && w.Scope.Selection.Disposition != RuntimeTargetSelectionScope {
		return fmt.Errorf("semantic implementation work requires an owner-selected or prospective work scope")
	}
	if err := referenceAdmissibilityCollectionIdentity(w.MutationInputCatalogs); err != nil {
		return err
	}
	for _, catalog := range w.MutationInputCatalogs {
		if !reflect.DeepEqual(catalog, catalog.Normalized()) {
			return fmt.Errorf("semantic work input protection catalogs must retain canonical owner preimages")
		}
	}
	excluded := runtimeValidationInputMutationExclusions(w.Scope.Request.Policy.OwnerModuleID, RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: w.MutationInputCatalogs}, RequiredOutputContract{}, nil)
	if !reflect.DeepEqual(excluded, w.ExcludedTargetRefs) {
		return fmt.Errorf("semantic work exclusions do not match complete mutation input catalogs")
	}
	return nil
}

func semanticWorkValidationManifest(t *testing.T) RuntimeSemanticPlanManifest {
	t.Helper()
	ledger, sources := implementationWorkLedgerFixture(t)
	round := ledger.Rounds[0]
	work := RuntimeSemanticImplementationWork{RoundHash: round.Hash, Ordinal: round.Ordinal, PolicyHash: round.PolicyHash, Scope: round.ScopeObservation.Clone()}
	work.MutationInputCatalogs = []RuntimeOperationAdmissibility{round.WorkingBinding.OwnerCatalog.Clone()}
	work.ExcludedTargetRefs = runtimeValidationInputMutationExclusions(work.Scope.Request.Policy.OwnerModuleID, RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: work.MutationInputCatalogs}, RequiredOutputContract{}, nil)
	m := RuntimeSemanticPlanManifest{SchemaVersion: RuntimeSemanticPlanManifestSchemaCurrent, WorkspaceID: sources.WorkspaceID, RunID: sources.RunID,
		DefinitionRevision: sources.Seed.DefinitionRevision, DefinitionSeedHash: sources.Seed.Hash, OperationContractRevision: RuntimeOperationContractRevision(),
		CoverageItems: sources.Seed.CoverageItems, AllowedOwnerModuleIDs: []string{RuntimeModuleFile}, AllowedOperationKinds: []string{StepKindFileWrite}, AllowedTargetPolicies: work.Scope.Request.Policy.AllowedTargetPolicies,
		ProofObligations: []RuntimeSemanticProofObligation{{CoverageWorkItemID: round.CoverageWorkItemID, DeclarationRef: "definition/dodItem/dod-1", RequiredEvidenceKinds: []string{EvidenceKindValidation}, ImplementationWork: &work}},
		OwnerCatalogs:    []RuntimeOperationAdmissibility{round.WorkingBinding.OwnerCatalog.Clone()},
	}.Normalized()
	m.Hash = StableRuntimeSemanticPlanManifestHash(m)
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSemanticWorkValidationReusesOnlyUnchangedCompleteWorkGate(t *testing.T) {
	base := semanticWorkValidationManifest(t)
	before, _ := json.Marshal(base)
	for _, mutate := range []func(*RuntimeSemanticImplementationWork){
		func(w *RuntimeSemanticImplementationWork) {},
		func(w *RuntimeSemanticImplementationWork) { w.Ordinal = 0 },
		func(w *RuntimeSemanticImplementationWork) { w.PolicyHash = "other" },
		func(w *RuntimeSemanticImplementationWork) { w.Scope.Request.RequestID = "other" },
		func(w *RuntimeSemanticImplementationWork) { w.Scope.Selection.ObservedRevision = "other" },
		func(w *RuntimeSemanticImplementationWork) { w.MutationInputCatalogs[0].SnapshotID = "other" },
		func(w *RuntimeSemanticImplementationWork) {
			w.MutationInputCatalogs[0].TargetRefs = []string{"raw", "raw"}
		},
		func(w *RuntimeSemanticImplementationWork) { w.ExcludedTargetRefs = []string{"other"} },
		func(w *RuntimeSemanticImplementationWork) {
			w.MutationInputCatalogs = append(w.MutationInputCatalogs, w.MutationInputCatalogs[0].Clone())
		},
	} {
		changed := base.Normalized()
		mutate(changed.ProofObligations[0].ImplementationWork)
		work := changed.ProofObligations[0].ImplementationWork
		if got, want := cwiFocusBindingsError(work.Validate()), cwiFocusBindingsError(referenceSemanticWorkValidation(*work)); got != want {
			t.Fatalf("original complete work gate/first error changed: %q != %q", got, want)
		}
		clone := changed.ProofObligations[0].Normalized().ImplementationWork
		if !reflect.DeepEqual(work, clone) || cwiFocusBindingsError(work.Validate()) != cwiFocusBindingsError(clone.Validate()) {
			t.Fatal("normalization changes the work preimage or gate")
		}
		changed.Hash = StableRuntimeSemanticPlanManifestHash(changed)
		if work.Validate() != nil && changed.Validate() == nil {
			t.Fatal("invalid work escaped the original complete obligation gate")
		}
		body, _ := json.Marshal(changed)
		var loaded RuntimeSemanticPlanManifest
		if err := json.Unmarshal(body, &loaded); err != nil || cwiFocusBindingsError(changed.Validate()) != cwiFocusBindingsError(loaded.Validate()) {
			t.Fatalf("work gate reload differs: %v", err)
		}
	}
	// The accepted detached result cannot freeze a subsequent invocation's input.
	validated, err := validateRuntimeSemanticPlanManifest(base)
	if err != nil {
		t.Fatal(err)
	}
	validated.manifest.ProofObligations[0].ImplementationWork.MutationInputCatalogs[0].TargetRefs[0] = "caller"
	after, _ := json.Marshal(base)
	if !reflect.DeepEqual(before, after) || base.Validate() != nil {
		t.Fatal("validated result aliases the original work source")
	}
	base.ProofObligations[0].ImplementationWork.Ordinal = 0
	base.Hash = StableRuntimeSemanticPlanManifestHash(base)
	if base.Validate() == nil {
		t.Fatal("subsequent invocation reused an old success")
	}
}

func TestSemanticWorkCatalogSessionCannotAcceptMutatedRawPreimage(t *testing.T) {
	m := semanticWorkValidationManifest(t)
	work := m.ProofObligations[0].ImplementationWork
	session := newRuntimeSemanticValidationSession()
	if err := work.validateInSemanticSession(session); err != nil {
		t.Fatal(err)
	}
	work.MutationInputCatalogs[0].TargetRefs[0] = "changed-with-old-hash"
	if err := work.validateInSemanticSession(session); err == nil {
		t.Fatal("catalog session aliased the raw preimage and reused old success")
	}
	if got, want := cwiFocusBindingsError(work.validateInSemanticSession(session)), cwiFocusBindingsError(referenceSemanticWorkValidation(*work)); got != want {
		t.Fatalf("mutation rejection changed: %q != %q", got, want)
	}
}

func FuzzSemanticWorkValidationSessionMatchesOriginalGate(f *testing.F) {
	for i := uint8(0); i < 9; i++ {
		f.Add(i, "same-key-mutated-raw")
	}
	f.Fuzz(func(t *testing.T, arm uint8, value string) {
		if len(value) > 128 {
			t.Skip()
		}
		base := semanticWorkValidationManifest(t).ProofObligations[0].ImplementationWork
		session := newRuntimeSemanticValidationSession()
		if err := base.validateInSemanticSession(session); err != nil {
			t.Fatal(err)
		}
		work := base.Clone()
		switch arm % 9 {
		case 0:
			work.PolicyHash = value
		case 1:
			work.Scope.Request.RequestID = value
		case 2:
			work.MutationInputCatalogs[0].PrerequisiteID = value
		case 3:
			work.MutationInputCatalogs[0].TargetRefs = []string{value}
		case 4:
			work.MutationInputCatalogs[0].SnapshotID = value
		case 5:
			work.MutationInputCatalogs = append(work.MutationInputCatalogs, work.MutationInputCatalogs[0].Clone())
		case 6:
			work.ExcludedTargetRefs = []string{value}
		case 7:
			work.PurposeChoiceOnly = true
			work.EffectAvailable = arm&1 != 0
		case 8:
			work.Ordinal = int(arm)
		}
		before, _ := json.Marshal(work)
		want := cwiFocusBindingsError(referenceSemanticWorkValidation(work))
		if got := cwiFocusBindingsError(work.validateInSemanticSession(session)); got != want {
			t.Fatalf("same-call session changed complete original gate: %q != %q", got, want)
		}
		if got := cwiFocusBindingsError(work.Validate()); got != want {
			t.Fatalf("new call changed complete original gate: %q != %q", got, want)
		}
		after, _ := json.Marshal(work)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("validation changed original raw work")
		}
	})
}

func TestSemanticWorkProofReusePreservesEveryProposalGate(t *testing.T) {
	manifest := semanticWorkValidationManifest(t)
	validated, err := validateRuntimeSemanticPlanManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	ledger, _ := implementationWorkLedgerFixture(t)
	original := RuntimeSemanticPlanDocument{Proposals: []RuntimeSemanticProofProposal{ledger.Rounds[0].WorkingBinding.Proposals[0].Normalized()}}
	if err := validateRuntimeSemanticPlanProofObligationsWithValidatedManifest(original, validated); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "coverage", "owner", "operation", "target", "duplicate", "evidence", "effect", "recipe", "declaration", "input-review"} {
		t.Run(scenario, func(t *testing.T) {
			changed := original.Clone()
			proposal := &changed.Proposals[0]
			switch scenario {
			case "coverage":
				proposal.CoverageWorkItemID = "another"
			case "owner":
				proposal.OwnerModuleID = RuntimeModuleShell
			case "operation":
				proposal.OperationKind = StepKindFileRead
			case "target":
				proposal.Target.OwnerScopeTargetRefs = []string{"another"}
			case "duplicate":
				changed.Proposals = append(changed.Proposals, proposal.Normalized())
			case "evidence":
				proposal.RequiredEvidenceKinds = []string{EvidenceKindValidation}
			case "effect":
				proposal.RequiredEffectScopes = []RuntimeEffectScope{RuntimeEffectEphemeralSession}
			case "recipe":
				proposal.ValidationRecipeID = "another"
			case "declaration":
				proposal.DeclarationRef = "another"
			case "input-review":
				proposal.ValidationInputReview = &RuntimeValidationInputReview{SubjectHash: "another"}
			}
			assertSemanticProofObligationValidationEquivalent(t, changed, validated)
			body, err := json.Marshal(changed)
			var reloaded RuntimeSemanticPlanDocument
			if err != nil || json.Unmarshal(body, &reloaded) != nil {
				t.Fatal("complete document reload failed", err)
			}
			assertSemanticProofObligationValidationEquivalent(t, reloaded, validated)
		})
	}
}
