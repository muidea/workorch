package runtimev2

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeV2TestEnsureCurrentFocusBindings upgrades only direct reducer
// fixtures that predate the current activation contract. Production has no
// equivalent path: activationcore compiles the binding set before a Runtime
// record exists. Keeping this at the test boundary lets the reducer tests
// exercise the same sealed BindingRef -> receipt -> credit flow without a
// production raw-CWI compatibility branch.
func runtimeV2TestEnsureCurrentFocusBindings(t *testing.T, aggregate *RuntimeAggregate) {
	t.Helper()
	if aggregate == nil || len(aggregate.Facts.Focus.CoverageWorkItemIDs) == 0 {
		return
	}
	focus := aggregate.Facts.Focus.Normalized()
	definitionRevision := aggregate.Facts.DefinitionRevision
	if definitionRevision == "" {
		definitionRevision = aggregate.Definition.Revision
	}
	if definitionRevision == "" {
		definitionRevision = "test-definition"
	}
	// Test fixtures routinely narrow RequiredEvidenceKinds or proof constraints
	// after constructing a generic focus. This helper intentionally rebuilds
	// its own synthetic declaration from the final fixture values. Production
	// never derives bindings after activation and has no equivalent path.
	validationRecipeID := focus.ValidationRecipeID
	if validationRecipeID == "" && hasRuntimeV2TestEvidenceKind(focus.RequiredEvidenceKinds, runtimecontract.EvidenceKindValidation) {
		for _, policy := range aggregate.Facts.Activation.EvidencePolicies {
			if normalized := policy.Normalized(); normalized.RecipeDeclared && normalized.ID != "" {
				validationRecipeID = normalized.ID
				break
			}
		}
		if validationRecipeID == "" {
			validationRecipeID = "test-runtime-v2-validation-recipe"
		}
		focus.ValidationRecipeID = validationRecipeID
	}
	proofs := make([]runtimecontract.CoverageEvidenceProof, 0, len(focus.RequiredEvidenceKinds))
	for _, evidenceKind := range focus.RequiredEvidenceKinds {
		proof := runtimecontract.CoverageEvidenceProof{EvidenceKind: evidenceKind}
		for _, descriptor := range runtimecontract.RuntimeOperationDescriptors() {
			if runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(descriptor.Kind, evidenceKind) {
				proof.AllowedOperationKinds = append(proof.AllowedOperationKinds, descriptor.Kind)
			}
		}
		switch evidenceKind {
		case runtimecontract.EvidenceKindStateChange:
			proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable}
		case runtimecontract.EvidenceKindValidation:
			proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession}
			proof.ValidationRecipeID = validationRecipeID
			proof.RequiredCheckRefs = append([]string(nil), focus.RequiredCheckRefs...)
			proof.MetricConstraints = append([]string(nil), focus.MetricConstraints...)
			proof.ArtifactRefs = append([]string(nil), focus.ArtifactRefs...)
		default:
			t.Fatalf("test fixture has unsupported evidence kind %q", evidenceKind)
		}
		proofs = append(proofs, proof.Normalized())
	}
	bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(focus.CoverageWorkItemIDs))
	for _, coverageID := range focus.CoverageWorkItemIDs {
		binding := runtimecontract.CoverageEvidenceBinding{
			ID:                       "binding/test/runtimev2/" + focus.ID + "/" + string(coverageID),
			CoverageWorkItemID:       coverageID,
			FocusID:                  focus.ID,
			DefinitionRevision:       definitionRevision,
			Proofs:                   append([]runtimecontract.CoverageEvidenceProof(nil), proofs...),
			TargetRefs:               append([]string(nil), focus.TargetRefs...),
			TargetSelectorRef:        focus.TargetSelectorRef,
			TargetSelectorOwnerRef:   focus.TargetSelectorOwnerRef,
			TargetSelectorSubjectRef: focus.TargetSelectorSubjectRef,
			ProspectiveTargetRef:     focus.ProspectiveTargetRef,
			DeclarationRef:           "test/runtimev2-reducer-fixture",
		}.Normalized()
		if len(binding.TargetRefs) == 0 && binding.TargetSelectorRef == "" && binding.ProspectiveTargetRef == "" {
			binding.TargetSelectorRef = "test/runtimev2-owner-selector"
			binding.TargetSelectorOwnerRef = "test-runtimev2"
			binding.TargetSelectorSubjectRef = focus.ID
		}
		if binding.TargetSelectorRef != "" && binding.TargetSelectorOwnerRef == "" {
			binding.TargetSelectorOwnerRef = "test-runtimev2"
		}
		if binding.TargetSelectorRef != "" && binding.TargetSelectorSubjectRef == "" {
			binding.TargetSelectorSubjectRef = focus.ID
		}
		binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
		bindings = append(bindings, binding)
	}
	focus.CoverageEvidenceBindings = runtimecontract.NormalizeCoverageEvidenceBindings(bindings)
	aggregate.Facts.Focus = focus
	decision, err := ReduceRuntimeState(aggregate.Facts, nil)
	if err != nil {
		t.Fatalf("reduce test fixture with sealed coverage binding: %v", err)
	}
	aggregate.Decision = decision
}

func runtimeV2TestFactsWithCurrentFocusBindings(t *testing.T, facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext) runtimecontract.RuntimeFacts {
	t.Helper()
	aggregate := RuntimeAggregate{Facts: facts, Definition: definition}
	runtimeV2TestEnsureCurrentFocusBindings(t, &aggregate)
	return aggregate.Facts
}

func runtimeV2TestBindCandidateToFacts(t *testing.T, facts runtimecontract.RuntimeFacts, candidate runtimecontract.RuntimeCandidate) runtimecontract.RuntimeCandidate {
	t.Helper()
	return runtimeV2TestBindCandidateToCurrentFocus(t, RuntimeAggregate{Facts: facts}, candidate)
}

func hasRuntimeV2TestEvidenceKind(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

// runtimeV2TestBindCandidateToCurrentFocus is intentionally a test-provider
// boundary adapter. Its inputs are old in-process literals, while its output
// is the current sealed wire shape consumed by the real reducer. It never
// runs in production and never allows raw CWI IDs through a current manifest.
func runtimeV2TestBindCandidateToCurrentFocus(t *testing.T, aggregate RuntimeAggregate, candidate runtimecontract.RuntimeCandidate) runtimecontract.RuntimeCandidate {
	t.Helper()
	candidate = runtimeV2TestBindCandidateToFocusBindings(aggregate.Facts.Focus.CoverageEvidenceBindings, candidate)
	for _, operation := range candidateRuntimeOperations(candidate) {
		if len(operation.CoverageWorkItemIDs) != 0 {
			t.Fatalf("test fixture operation %q has no matching sealed binding", operation.ID)
		}
	}
	return candidate
}

// runtimeV2TestBindCandidateToFocusBindings migrates only legacy in-process
// fixture literals into the sealed shape. Raw CWI IDs are never a wire
// authority, including in tests: a legacy fixture operation becomes either an
// exact binding selection or an explicit non-credit diagnostic operation. No
// production call site can reach this test-only adapter.
func runtimeV2TestBindCandidateToFocusBindings(bindings []runtimecontract.CoverageEvidenceBinding, candidate runtimecontract.RuntimeCandidate) runtimecontract.RuntimeCandidate {
	bind := func(operation *runtimecontract.RuntimeOperation) {
		if operation == nil || len(operation.BindingRefs) != 0 || len(operation.CoverageWorkItemIDs) == 0 {
			return
		}
		refs := make([]string, 0, len(operation.CoverageWorkItemIDs))
		for _, coverageID := range operation.CoverageWorkItemIDs {
			for _, binding := range bindings {
				if binding.CoverageWorkItemID != coverageID || !runtimeV2TestBindingAuthorizesOperation(binding, *operation) {
					continue
				}
				refs = append(refs, binding.ID)
			}
		}
		if len(refs) != 0 {
			operation.BindingRefs = runtimeV2TestNormalizeBindingRefs(refs)
		}
		// C12 removes raw CWI authority outright. A legacy fixture that cannot
		// prove its declared CWI remains a non-credit operation, rather than
		// leaking a forbidden field or borrowing a different binding.
		operation.CoverageWorkItemIDs = nil
	}
	if candidate.StateChange != nil {
		for index := range candidate.StateChange.Operations {
			bind(&candidate.StateChange.Operations[index].Operation)
		}
	}
	if candidate.Validation != nil {
		for index := range candidate.Validation.Operations {
			bind(&candidate.Validation.Operations[index].Operation)
		}
	}
	return candidate
}

func runtimeV2TestBindingAuthorizesOperation(binding runtimecontract.CoverageEvidenceBinding, operation runtimecontract.RuntimeOperation) bool {
	binding = binding.Normalized()
	operation = operation.Normalized()
	proofCapable := false
	for _, proof := range binding.Proofs {
		for _, kind := range proof.AllowedOperationKinds {
			if runtimecontract.CanonicalRuntimeBlockKind(kind) == runtimecontract.CanonicalRuntimeBlockKind(operation.Kind) {
				proofCapable = true
				break
			}
		}
		if proofCapable {
			break
		}
	}
	if !proofCapable {
		return false
	}
	if len(binding.TargetRefs) != 0 {
		return sameRuntimeTargetRefs(binding.TargetRefs, operation.TargetRefs)
	}
	if binding.ProspectiveTargetRef != "" {
		return sameRuntimeTargetRefs([]string{binding.ProspectiveTargetRef}, operation.TargetRefs)
	}
	return binding.TargetSelectorRef != ""
}

func runtimeV2TestNormalizeBindingRefs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

func attachRuntimeV2TestOwnerReceipt(t *testing.T, aggregate *RuntimeAggregate, evidence *ExecutionEvidence) {
	t.Helper()
	if aggregate == nil || aggregate.ActiveDispatch == nil || evidence == nil {
		t.Fatal("test owner receipt requires active dispatch and evidence")
	}
	var execution *runtimecontract.RuntimeOperationExecutionRecord
	for idx := range aggregate.ActiveDispatch.OperationExecutions {
		if aggregate.ActiveDispatch.OperationExecutions[idx].OperationID == evidence.OperationID {
			execution = &aggregate.ActiveDispatch.OperationExecutions[idx]
			break
		}
	}
	if execution == nil {
		t.Fatalf("operation %s has no sealed execution", evidence.OperationID)
	}
	if execution.Fence == 0 {
		execution.Fence = 1
	}
	var (
		targetRefs      []string
		sealedOperation runtimecontract.RuntimeOperation
		operationFound  bool
	)
	for _, operation := range candidateRuntimeOperations(aggregate.ActiveDispatch.Candidate) {
		if operation.ID == evidence.OperationID {
			targetRefs = append([]string(nil), operation.TargetRefs...)
			sealedOperation = operation.Normalized()
			operationFound = true
			break
		}
	}
	if !operationFound {
		t.Fatalf("operation %s is missing from sealed candidate", evidence.OperationID)
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: execution.ExpectedEffectScope,
		Changed: runtimecontract.RuntimeEffectIsDurable(execution.ExpectedEffectScope), TargetRefs: targetRefs, Evidence: evidence.Evidence,
	}
	if evidence.Status == ExecutionEvidenceStatusRejected {
		receipt.Outcome = runtimecontract.RuntimeOperationOutcomeRejected
		receipt.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
		receipt.Changed = false
		receipt.Failure = evidence.Failure
	}
	// An explicitly supplied validation observation is an owner fact for this
	// test case and must survive receipt construction. Only a sealed,
	// proof-capable validation operation may receive the deterministic fixture
	// observation below; a file read or any other diagnostic operation must not
	// be promoted to a coverage-capable owner receipt by test infrastructure.
	if evidence.Kind == runtimecontract.EvidenceKindValidation && receipt.Evidence.Validation == nil {
		for _, ref := range sealedOperation.BindingRefs {
			binding, found := runtimecontract.CoverageEvidenceBindingForRef(aggregate.Facts.Focus.CoverageEvidenceBindings, ref)
			if !found {
				continue
			}
			proof, found := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
			if !found || !runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(sealedOperation.Kind, runtimecontract.EvidenceKindValidation) {
				continue
			}
			receipt.Evidence.Validation = &runtimecontract.RuntimeValidationObservation{RecipeID: proof.ValidationRecipeID, PassedChecks: append([]string(nil), proof.RequiredCheckRefs...)}
			break
		}
	}
	receipt = receipt.Normalized()
	hash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	execution.State = runtimecontract.OperationExecutionOwnerCommitted
	execution.OwnerReceiptRef = "runtime-receipt/" + execution.ExecutionKey
	execution.OwnerReceiptHash = hash
	aggregate.OperationReceipts = append(aggregate.OperationReceipts, receipt)
	evidence.Receipt = &receipt
	evidence.ObservedEffectScope = receipt.ObservedEffectScope
	evidence.OwnerReceiptRef = execution.OwnerReceiptRef
	evidence.OwnerReceiptHash = hash
}
