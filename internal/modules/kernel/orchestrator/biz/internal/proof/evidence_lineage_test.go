package proof

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestEvidenceLineageRequiresCurrentBindingHashOrEquivalentResolution(t *testing.T) {
	oldBinding := proofTestCoverageBinding("binding-stable", "catalog-old")
	currentBinding := proofTestCoverageBinding("binding-stable", "catalog-current")
	if oldBinding.Hash == currentBinding.Hash {
		t.Fatal("fixture did not change the binding hash")
	}
	facts := proofTestCurrentFacts(currentBinding)
	evidence := runtimecontract.EvidenceFact{
		ID: "evidence-1", Kind: runtimecontract.EvidenceKindStateChange, Quality: common.EvidenceQualityValid,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, BindingRefs: []string{oldBinding.ID},
	}
	normalizedFacts := facts.Normalized()
	normalizedBinding, _ := CoverageBindingForCoverage(normalizedFacts.Focus, "cwi-1")
	wantedBindingRef := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(normalizedBinding, normalizedFacts.Coverage[0].CurrentBindingRef.SemanticManifestHash)
	if !runtimecontract.SameRuntimeCWIBindingRef(*normalizedFacts.Coverage[0].CurrentBindingRef, wantedBindingRef) {
		t.Fatalf("fixture binding refs differ: current=%+v wanted=%+v", *normalizedFacts.Coverage[0].CurrentBindingRef, wantedBindingRef)
	}
	if item, found := CurrentCWIBindingForFocus(facts, "cwi-1"); !found || item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound {
		t.Fatalf("fixture current binding is unavailable: found=%v item=%+v facts=%+v", found, item, facts)
	}
	sourceIdentity, err := runtimecontract.DeriveCoverageEvidenceProofIdentity(oldBinding, evidence.Kind)
	if err != nil {
		t.Fatal(err)
	}
	source := runtimecontract.RuntimeCoverageProofLineage{
		ID: "source-1", Kind: runtimecontract.RuntimeCoverageProofLineageKindSource, WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
		CoverageWorkItemID: "cwi-1", EvidenceKind: evidence.Kind, SourceEvidenceID: evidence.ID,
		SourceBindingRef: oldBinding.ID, SourceBindingHash: oldBinding.Hash, SourceProofIdentity: sourceIdentity,
	}
	if EvidenceAppliesToCoverageWorkItemWithLineage(facts, evidence, "cwi-1", []runtimecontract.RuntimeCoverageProofLineage{source}) {
		t.Fatal("old source hash bypassed current-binding resolution")
	}
	currentIdentity, err := runtimecontract.DeriveCoverageEvidenceProofIdentity(currentBinding, evidence.Kind)
	if err != nil {
		t.Fatal(err)
	}
	resolution := runtimecontract.RuntimeCoverageProofLineage{
		Kind: runtimecontract.RuntimeCoverageProofLineageKindResolution, SourceLineageID: source.ID,
		CurrentBindingRef: currentBinding.ID, CurrentBindingHash: currentBinding.Hash,
		CurrentProofIdentity: currentIdentity, Disposition: runtimecontract.RuntimeCoverageProofLineageDispositionEquivalent,
	}
	if !SameProofLineageResolution([]runtimecontract.RuntimeCoverageProofLineage{resolution}, source.ID, currentBinding, currentIdentity) {
		t.Fatalf("fixture resolution does not match: resolution=%+v identity=%+v binding=%+v", resolution, currentIdentity, currentBinding)
	}
	if _, found := CoverageProofLineageSourceForEvidence([]runtimecontract.RuntimeCoverageProofLineage{source}, facts, evidence, "cwi-1"); !found {
		t.Fatalf("fixture source does not match: source=%+v evidence=%+v", source, evidence)
	}
	if !EvidenceAppliesToCoverageWorkItemWithLineage(facts, evidence, "cwi-1", []runtimecontract.RuntimeCoverageProofLineage{source, resolution}) {
		t.Fatal("equivalent current-binding resolution did not restore proof attribution")
	}
	resolution.CurrentBindingHash = "stale-hash"
	if EvidenceAppliesToCoverageWorkItemWithLineage(facts, evidence, "cwi-1", []runtimecontract.RuntimeCoverageProofLineage{source, resolution}) {
		t.Fatal("stale resolution granted proof attribution")
	}
}

func proofTestCoverageBinding(id, prerequisite string) runtimecontract.CoverageEvidenceBinding {
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: id, CoverageWorkItemID: "cwi-1", FocusID: "focus-1", DefinitionRevision: "definition-1",
		DeclarationRef: "proof-test", TargetRefs: []string{"main.go"}, PrerequisiteRefs: []string{prerequisite},
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind: runtimecontract.EvidenceKindStateChange, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
		}},
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	return binding
}

func proofTestCurrentFacts(binding runtimecontract.CoverageEvidenceBinding) runtimecontract.RuntimeFacts {
	current := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, "manifest-current")
	return runtimecontract.RuntimeFacts{
		WorkspaceID: "workspace-1", RunID: "run-1", CWILedgerSchemaVersion: runtimecontract.RuntimeCWILedgerSchemaCurrent,
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{binding},
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{
			ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen,
			CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStateBound, CurrentBindingRef: &current, FocusID: "focus-1",
		}},
	}
}
