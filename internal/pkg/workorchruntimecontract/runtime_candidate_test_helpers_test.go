package workorchruntimecontract

import (
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

// runtimeCandidateTestDecisionWithBindings builds explicit, immutable fixture
// declarations for unit tests that exercise the pure manifest compiler. It is
// deliberately test-only: production bindings are compiled by activationcore
// from template/profile declarations and never inferred from required output.
func runtimeCandidateTestDecisionWithBindings(t testing.TB, decision RuntimeDecision) RuntimeDecision {
	t.Helper()
	required := decision.RequiredOutput.Normalized()
	if len(required.CoverageWorkItemIDs) == 0 ||
		(decision.State != ConvergenceStateNeedStateChange && decision.State != ConvergenceStateNeedValidation && decision.State != ConvergenceStateNeedAssessment) ||
		len(decision.Focus.CoverageEvidenceBindings) != 0 {
		return decision
	}

	definitionRevision := decision.AuthoritySnapshot.DefinitionRevision
	if definitionRevision == "" {
		definitionRevision = "definition-candidate-test"
		decision.AuthoritySnapshot.DefinitionRevision = definitionRevision
	}
	if decision.Focus.ID == "" {
		decision.Focus.ID = "focus-candidate-test"
	}
	decision.Focus.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), required.CoverageWorkItemIDs...)
	if len(decision.Focus.RequiredEvidenceKinds) == 0 {
		switch decision.State {
		case ConvergenceStateNeedStateChange:
			decision.Focus.RequiredEvidenceKinds = []string{EvidenceKindStateChange}
		case ConvergenceStateNeedValidation:
			decision.Focus.RequiredEvidenceKinds = []string{EvidenceKindValidation}
		case ConvergenceStateNeedAssessment:
			decision.Focus.RequiredEvidenceKinds = []string{EvidenceKindValidation}
		}
	}
	acceptedKinds := append([]string(nil), required.AcceptedKinds...)
	if decision.State == ConvergenceStateNeedAssessment {
		acceptedKinds = []string{RuntimeBlockKindManagedValidationSession}
	}
	proofs := make([]CoverageEvidenceProof, 0, len(decision.Focus.RequiredEvidenceKinds))
	for _, evidenceKind := range decision.Focus.RequiredEvidenceKinds {
		proof := CoverageEvidenceProof{EvidenceKind: evidenceKind}
		for _, kind := range acceptedKinds {
			if RuntimeOperationKindSupportsCoverageEvidence(kind, evidenceKind) {
				proof.AllowedOperationKinds = append(proof.AllowedOperationKinds, kind)
			}
		}
		if len(proof.AllowedOperationKinds) == 0 {
			t.Fatalf("test fixture has no owner-verifiable %s proof operation in %v", evidenceKind, acceptedKinds)
		}
		switch evidenceKind {
		case EvidenceKindStateChange:
			proof.RequiredEffectScopes = durableCandidateTestEffectScopes(required.RequiredEffectScopes)
		case EvidenceKindValidation:
			proof.RequiredEffectScopes = nonDurableCandidateTestEffectScopes(required.RequiredEffectScopes)
			proof.ValidationRecipeID = "recipe/candidate-test"
		default:
			t.Fatalf("unsupported required evidence kind %q", evidenceKind)
		}
		proofs = append(proofs, proof)
	}
	bindings := make([]CoverageEvidenceBinding, 0, len(required.CoverageWorkItemIDs))
	for _, coverageID := range required.CoverageWorkItemIDs {
		binding := CoverageEvidenceBinding{
			ID:                 fmt.Sprintf("binding/test/%s", coverageID),
			CoverageWorkItemID: coverageID,
			FocusID:            decision.Focus.ID,
			DefinitionRevision: definitionRevision,
			Proofs:             append([]CoverageEvidenceProof(nil), proofs...),
			DeclarationRef:     fmt.Sprintf("declaration/test/%s", coverageID),
		}
		if len(required.TargetRefs) != 0 {
			binding.TargetRefs = append([]string(nil), required.TargetRefs...)
		} else {
			binding.TargetSelectorRef = "selector/candidate-test"
			binding.TargetSelectorOwnerRef = "owner/candidate-test"
			binding.TargetSelectorSubjectRef = "subject/candidate-test"
		}
		bindings = append(bindings, binding)
	}
	decision.Focus.CoverageEvidenceBindings = NormalizeCoverageEvidenceBindings(bindings)
	// A sealed generation focus and every binding it contains carry one exact
	// target surface. Keep this fixture on the same production invariant instead
	// of letting tests rely on binding-only target authority.
	if len(decision.Focus.CoverageEvidenceBindings) != 0 {
		binding := decision.Focus.CoverageEvidenceBindings[0]
		decision.Focus.TargetRefs = append([]string(nil), binding.TargetRefs...)
		decision.Focus.TargetSelectorRef = binding.TargetSelectorRef
		decision.Focus.TargetSelectorOwnerRef = binding.TargetSelectorOwnerRef
		decision.Focus.TargetSelectorSubjectRef = binding.TargetSelectorSubjectRef
		decision.Focus.TargetSelectorCandidateRefs = append([]string(nil), binding.TargetSelectorCandidateRefs...)
		decision.Focus.ProspectiveTargetRefs = append([]string(nil), binding.ProspectiveTargetRefs...)
	}
	return decision
}

func durableCandidateTestEffectScopes(values []RuntimeEffectScope) []RuntimeEffectScope {
	ret := make([]RuntimeEffectScope, 0, len(values))
	for _, value := range NormalizeRuntimeEffectScopes(values) {
		if RuntimeEffectIsDurable(value) {
			ret = append(ret, value)
		}
	}
	if len(ret) == 0 {
		return []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}
	}
	return ret
}

func nonDurableCandidateTestEffectScopes(values []RuntimeEffectScope) []RuntimeEffectScope {
	ret := make([]RuntimeEffectScope, 0, len(values))
	for _, value := range NormalizeRuntimeEffectScopes(values) {
		if RuntimeEffectIsNonDurable(value) {
			ret = append(ret, value)
		}
	}
	if len(ret) == 0 {
		return []RuntimeEffectScope{RuntimeEffectNone}
	}
	return ret
}

func runtimeCandidateTestBindingRef(coverageID common.CoverageWorkItemID) string {
	return fmt.Sprintf("binding/test/%s", coverageID)
}
