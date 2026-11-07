package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestImplementationDependencyScopeClaimsUseOwnerMembership(t *testing.T) {
	m, d := progressPurposeFixture(t)
	w := m.ProofObligations[0].ImplementationWork
	// The current owner scope has existing targets; the policy additionally
	// permits prospective targets. Existing members are never a create allowlist.
	w.Scope.Request.Policy.AllowedTargetPolicies = []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyProspectiveOwnerTarget}
	w.Scope.Request.Policy.ProspectiveTargetAllOfConstraintRefs = []string{RuntimeImplementationWorkConstraintWorkspaceRelativePath, RuntimeImplementationWorkConstraintNewTargetOwnerSelector}
	w.Scope.Request.Policy.TargetSelectorCandidateRefs = nil
	w.Scope.Request.ManifestBinding.Manifest.TargetRefs = append([]string(nil), w.Scope.Selection.TargetRefs...)
	d.ImplementationIntent.DependencyTargetRefs = []string{"missing-delivery"}
	if err := validateImplementationResourceClaims(d, w); err != nil {
		t.Fatal("missing dependency must remain a proposal option", err)
	}
	d.ImplementationPurpose = RuntimeImplementationWorkPurposeInconclusive
	d.ImplementationIntent.DependencyTargetRefs = nil
	d.ImplementationIntent.SelectedRequirementRefs = nil
	d.ImplementationIntent.Uncertainty = &RuntimeImplementationUncertainty{Kind: RuntimeImplementationTargetScopeUnavailable, TargetRefs: []string{"missing-delivery"}}
	if validateImplementationResourceClaims(d, w) == nil {
		t.Fatal("owner-permitted prospective target was misreported as out of scope")
	}
	w.ExcludedTargetRefs = []string{"missing-delivery"}
	if err := validateImplementationResourceClaims(d, w); err != nil {
		t.Fatal("protected target limitation lost", err)
	}
	w.ExcludedTargetRefs = nil
	w.Scope.Request.Policy.AllowedTargetPolicies = []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExistingOwnerScope}
	if err := validateImplementationResourceClaims(d, w); err != nil {
		t.Fatal("genuine missing create policy must remain an honest failure", err)
	}
	d.ImplementationIntent.Uncertainty.TargetRefs = []string{w.Scope.Selection.TargetRefs[0]}
	if validateImplementationResourceClaims(d, w) == nil {
		t.Fatal("existing target eligibility ignored")
	}
	d.ImplementationIntent.Uncertainty.Kind = RuntimeImplementationSemanticInformationMissing
	if err := validateImplementationResourceClaims(d, w); err != nil {
		t.Fatal("semantic insufficiency is not owner scope adjudication", err)
	}
}

func TestImplementationUncertaintyClosedWireAndRecovery(t *testing.T) {
	m, d := progressPurposeFixture(t)
	d.ImplementationPurpose = RuntimeImplementationWorkPurposeInconclusive
	d.ImplementationIntent.SelectedRequirementRefs = nil
	d.ImplementationIntent.Uncertainty = &RuntimeImplementationUncertainty{Kind: RuntimeImplementationSemanticInformationMissing}
	assertImplementationSchemaAndParser(t, m, d, true)
	raw, err := EncodeRuntimeSemanticPlanPayloadWithManifest(m, d)
	if err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), raw...)
	for _, bad := range []string{
		strings.Replace(string(raw), `"semantic_information_missing"`, `"unknown_kind"`, 1),
		strings.Replace(string(raw), `"kind":"semantic_information_missing"`, `"kind":"semantic_information_missing","permission":true`, 1),
		strings.Replace(string(raw), `"kind":"semantic_information_missing"`, `"kind":"semantic_information_missing","targetRefs":null`, 1),
	} {
		if _, err := DecodeRuntimeSemanticPlanPayloadWithManifest(m, []byte(bad)); err == nil {
			t.Fatal("malformed uncertainty admitted", bad)
		}
	}
	got, err := DecodeRuntimeSemanticPlanPayloadWithManifest(m, raw)
	if err != nil || !reflect.DeepEqual(got.ImplementationIntent.Uncertainty, d.ImplementationIntent.Uncertainty) {
		t.Fatal("frozen replay changed uncertainty", err)
	}
	if !reflect.DeepEqual(original, raw) {
		t.Fatal("provider original changed")
	}
	got.ImplementationIntent.Uncertainty.TargetRefs = []string{"mutated"}
	if len(d.ImplementationIntent.Uncertainty.TargetRefs) != 0 {
		t.Fatal("clone aliases source")
	}
}

func TestInputReviewDependencyTasksRetainCheckAndCurrentScope(t *testing.T) {
	m, _ := progressPurposeFixture(t)
	w := m.ProofObligations[0].ImplementationWork
	w.Scope.Request.Policy.TargetSelectorCandidateRefs = nil
	w.Scope.Request.Policy.AllowedTargetPolicies = []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyProspectiveOwnerTarget}
	w.Scope.Request.Policy.ProspectiveTargetAllOfConstraintRefs = []string{RuntimeImplementationWorkConstraintWorkspaceRelativePath, RuntimeImplementationWorkConstraintNewTargetOwnerSelector}
	existing := w.Scope.Selection.TargetRefs[0]
	w.Scope.Request.ManifestBinding.Manifest.TargetRefs = []string{existing}
	w.Scope.Request.InputRevision = &RuntimeValidationInputRevision{Report: RuntimeValidationInputReview{Checks: []RuntimeValidationInputCheckReview{{RequiredCheckRef: "original-check", Conclusion: RuntimeValidationInputConflict, DependencyTargetRefs: []string{existing, "missing-delivery"}}}}}
	tasks := w.DependencyTasks()
	if len(tasks) != 2 || tasks[0].RequiredCheckRef != "original-check" || !tasks[0].PresentInOwnerManifest || tasks[1].PresentInOwnerManifest || !reflect.DeepEqual(tasks[1].CandidatePolicies, []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyProspectiveOwnerTarget}) {
		t.Fatal(tasks)
	}
	before, _ := json.Marshal(w)
	tasks[0].CandidatePolicies[0] = "mutated"
	after, _ := json.Marshal(w)
	if string(before) != string(after) {
		t.Fatal("projection changed owner scope")
	}
	c := w.Scope.Request.InputRevision.Report.Checks[0]
	for _, refs := range [][]string{{"same", "same"}, {""}, {" altered "}} {
		c.DependencyTargetRefs = refs
		if validateRuntimeInputDependencies(c) == nil {
			t.Fatal("invalid dependency admitted", refs)
		}
	}
	c.DependencyTargetRefs = []string{"missing"}
	c.Conclusion = RuntimeValidationInputConsistent
	if validateRuntimeInputDependencies(c) == nil {
		t.Fatal("unresolved dependency reported consistent")
	}
}

func TestImplementationProofCannotDropSelectedDependency(t *testing.T) {
	m, _ := progressPurposeFixture(t)
	w := m.ProofObligations[0].ImplementationWork
	w.PurposeChoiceOnly = false
	w.EffectAvailable = false
	target := w.Scope.Selection.TargetRefs[0]
	p := RuntimeSemanticProofProposal{CoverageWorkItemID: w.Scope.Request.CoverageWorkItemID, OwnerModuleID: w.Scope.Request.Policy.OwnerModuleID, OperationKind: w.Scope.Request.Policy.AllowedOperations[0].Kind, OperationVariantID: w.Scope.Request.Policy.AllowedOperations[0].VariantID, RequiredEvidenceKinds: []string{EvidenceKindStateChange}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{target}}}
	w.SelectedDependencyTargetRefs = []string{target}
	set := RuntimeSemanticCWIProofSet{CoverageWorkItemID: p.CoverageWorkItemID, Proposals: []RuntimeSemanticProofProposal{p}}
	if err := validateRuntimeSemanticImplementationWorkProofSetAgainstValidatedWork(set, w); err != nil {
		t.Fatal(err)
	}
	w.SelectedDependencyTargetRefs = []string{"omitted"}
	if validateRuntimeSemanticImplementationWorkProofSetAgainstValidatedWork(set, w) == nil {
		t.Fatal("selected dependency disappeared before candidate")
	}
	// Isolate per-proposal versus collective duties within one bounded scope.
	w.Scope.Selection.TargetRefs = append(w.Scope.Selection.TargetRefs, "second-dependency")
	w.Scope.Request.Policy.TargetSelectorCandidateRefs = nil
	w.Scope.Request.Policy.MaxTargetsPerRound = 2
	w.SelectedDependencyTargetRefs = []string{target, "second-dependency"}
	if _, err := validateRuntimeSemanticWorkProposalTargets(p, w); err != nil {
		t.Fatal("one owner feasibility probe was forced to carry the whole proof set", err)
	}
	if validateRuntimeSemanticImplementationWorkProofSetAgainstValidatedWork(set, w) == nil {
		t.Fatal("per-proposal feasibility discharged collective dependency coverage")
	}
	second := p
	second.Target.OwnerScopeTargetRefs = []string{"second-dependency"}
	set.Proposals = append(set.Proposals, second)
	if err := validateRuntimeSemanticImplementationWorkProofSetAgainstValidatedWork(set, w); err != nil {
		t.Fatal("complete selected dependency set rejected", err)
	}
}

func TestInputDependencyReviewHashWireAndCloneRetainExactFindings(t *testing.T) {
	m, d, _, b := semanticInputReviewFixture(t)
	c := &d.Proposals[1].ValidationInputReview.Checks[0]
	c.Conclusion = RuntimeValidationInputConflict
	c.Obligations[0].Finding = RuntimeValidationInputMissing
	c.DependencyTargetRefs = []string{"missing-delivery"}
	raw, err := EncodeRuntimeSemanticPlanPayloadWithManifest(m, d)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := DecodeRuntimeSemanticPlanPayloadWithManifest(m, raw)
	if err != nil {
		t.Fatal(err)
	}
	index := -1
	for n, p := range restored.Proposals {
		if p.ValidationInputReview != nil {
			index = n
		}
	}
	if index < 0 {
		t.Fatal("input review missing from decoded proposal set")
	}
	review := restored.Proposals[index].ValidationInputReview
	if err := review.ValidateAgainstBinding(b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(review, d.Proposals[1].ValidationInputReview) {
		t.Fatal("dependency findings changed on the wire")
	}
	before := StableRuntimeSemanticProofProposalHash(restored.Proposals[index])
	cloned := restored.Clone()
	cloned.Proposals[index].ValidationInputReview.Checks[0].DependencyTargetRefs[0] = "other"
	if StableRuntimeSemanticProofProposalHash(cloned.Proposals[index]) == before || review.Checks[0].DependencyTargetRefs[0] != "missing-delivery" {
		t.Fatal("dependency mutation was not hashed or cloned")
	}
	bad := strings.Replace(string(raw), `"dependencyTargetRefs":["missing-delivery"]`, `"dependencyTargetRefs":null`, 1)
	if bad == string(raw) {
		t.Fatal("test did not change dependency refs")
	}
	if _, err := DecodeRuntimeSemanticPlanPayloadWithManifest(m, []byte(bad)); err == nil {
		t.Fatal("null dependency silently treated as not applicable")
	}
}
