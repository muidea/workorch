// Package workorchruntimeaudit verifies immutable Runtime read-model lineage.
// It is deliberately stateless: the Orchestrator remains the only owner that
// can turn an owner receipt into SM-2 coverage credit.
package workorchruntimeaudit

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CoverageBindingChainErrors checks the read-only CWI -> binding -> sealed
// operation -> owner receipt -> accepted evidence chain. Proof semantics are
// delegated to workorchruntimecontract.MatchCoverageEvidenceBindingReceiptIdentity,
// the same pure matcher used by the Orchestrator receipt-credit path.
func CoverageBindingChainErrors(audit *projectioncontract.RuntimeAudit, requireComplete bool) []string {
	if audit == nil {
		return []string{"missing Runtime audit projection"}
	}
	errors := make([]string, 0)
	bindingByID := make(map[string]runtimecontract.CoverageEvidenceBinding, len(audit.CoverageBindings))
	bindingByCWI := make(map[string]runtimecontract.CoverageEvidenceBinding, len(audit.CoverageBindings))
	for _, projected := range audit.CoverageBindings {
		binding, err := runtimeCoverageBinding(projected)
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		if _, duplicate := bindingByID[binding.ID]; duplicate {
			errors = append(errors, fmt.Sprintf("audit has duplicate coverage binding %s", binding.ID))
			continue
		}
		cwiID := string(binding.CoverageWorkItemID)
		if _, duplicate := bindingByCWI[cwiID]; duplicate {
			errors = append(errors, fmt.Sprintf("audit has multiple bindings for coverage work item %s", cwiID))
			continue
		}
		bindingByID[binding.ID] = binding
		bindingByCWI[cwiID] = binding
	}

	type operationKey struct{ dispatchID, operationID string }
	operations := make(map[operationKey]projectioncontract.RuntimeAuditSealedOperation)
	for _, dispatch := range audit.Dispatches {
		if strings.TrimSpace(dispatch.ID) == "" {
			continue
		}
		for _, operation := range dispatch.Operations {
			if strings.TrimSpace(operation.ID) == "" || runtimecontract.CanonicalRuntimeBlockKind(operation.Kind) == "" ||
				strings.TrimSpace(operation.ExecutionKey) == "" || operation.Fence == 0 {
				errors = append(errors, fmt.Sprintf("dispatch %s has malformed sealed operation", dispatch.ID))
				continue
			}
			key := operationKey{dispatchID: dispatch.ID, operationID: operation.ID}
			if _, duplicate := operations[key]; duplicate {
				errors = append(errors, fmt.Sprintf("dispatch %s duplicates sealed operation %s", dispatch.ID, operation.ID))
				continue
			}
			operations[key] = operation
		}
	}

	evidenceByID := make(map[string]projectioncontract.RuntimeAuditEvidence, len(audit.AcceptedEvidence))
	for _, evidence := range audit.AcceptedEvidence {
		if strings.TrimSpace(evidence.ID) != "" {
			evidenceByID[evidence.ID] = evidence
		}
	}
	for _, coverage := range audit.Coverage {
		if strings.TrimSpace(coverage.State) != "satisfied" {
			continue
		}
		binding, found := bindingByCWI[strings.TrimSpace(coverage.ID)]
		if !found {
			errors = append(errors, fmt.Sprintf("satisfied coverage %s lacks immutable binding", coverage.ID))
			continue
		}
		matchedEvidence := make([]projectioncontract.RuntimeAuditEvidence, 0)
		for _, evidence := range evidenceByID {
			if contains(evidence.CoverageWorkItemIDs, coverage.ID) && contains(evidence.BindingRefs, binding.ID) &&
				strings.TrimSpace(evidence.DispatchID) != "" && strings.TrimSpace(evidence.OperationID) != "" {
				matchedEvidence = append(matchedEvidence, evidence)
			}
		}
		if len(matchedEvidence) == 0 {
			errors = append(errors, fmt.Sprintf("satisfied coverage %s lacks binding-matched accepted evidence", coverage.ID))
			continue
		}
		if len(coverage.EvidenceRefs) != 0 {
			ledgerMatched := false
			for _, evidence := range matchedEvidence {
				if contains(coverage.EvidenceRefs, evidence.ID) {
					ledgerMatched = true
					break
				}
			}
			if !ledgerMatched {
				errors = append(errors, fmt.Sprintf("satisfied coverage %s ledger does not reference binding-matched evidence", coverage.ID))
			}
		}

		sort.Slice(matchedEvidence, func(i, j int) bool { return matchedEvidence[i].ID < matchedEvidence[j].ID })
		matchedProofKinds := make(map[string]struct{}, len(binding.Proofs))
		for _, evidence := range matchedEvidence {
			operation, found := operations[operationKey{dispatchID: evidence.DispatchID, operationID: evidence.OperationID}]
			if !found {
				errors = append(errors, fmt.Sprintf("evidence %s lacks sealed dispatch operation %s/%s", evidence.ID, evidence.DispatchID, evidence.OperationID))
				continue
			}
			if !contains(operation.BindingRefs, binding.ID) {
				errors = append(errors, fmt.Sprintf("sealed operation %s/%s lacks binding %s", evidence.DispatchID, evidence.OperationID, binding.ID))
				continue
			}
			if !sameStrings(evidence.TargetRefs, operation.TargetRefs) {
				errors = append(errors, fmt.Sprintf("evidence %s target refs differ from sealed operation", evidence.ID))
				continue
			}

			matchedReceipt := false
			var proofErrors []string
			for _, projectedReceipt := range audit.OperationReceipts {
				if projectedReceipt.OperationID != operation.ID || projectedReceipt.Outcome != string(runtimecontract.RuntimeOperationOutcomeAccepted) ||
					projectedReceipt.ExecutionKey != operation.ExecutionKey || projectedReceipt.Fence != operation.Fence {
					continue
				}
				receipt := runtimeOperationReceipt(projectedReceipt)
				if !sameStrings(receipt.TargetRefs, evidence.TargetRefs) || !sameStrings(receipt.Evidence.EvidenceRefs, evidence.EvidenceRefs) ||
					!sameStrings(receipt.Evidence.EvidenceTags, evidence.EvidenceTags) || string(receipt.Evidence.Quality) != strings.TrimSpace(evidence.Quality) ||
					receipt.Evidence.ZeroWork != evidence.ZeroWork || receipt.Evidence.EvidenceReason != strings.TrimSpace(evidence.EvidenceReason) ||
					!sameValidationObservation(receipt.Evidence.Validation, evidence) {
					continue
				}
				identity := runtimecontract.RuntimeOperationProofIdentity{
					Kind: operation.Kind, VariantID: operation.VariantID, TargetRefs: append([]string(nil), operation.TargetRefs...),
				}
				if err := runtimecontract.MatchCoverageEvidenceBindingReceiptIdentity(binding, evidence.Kind, identity, receipt); err != nil {
					proofErrors = append(proofErrors, err.Error())
					continue
				}
				matchedReceipt = true
				matchedProofKinds[strings.TrimSpace(evidence.Kind)] = struct{}{}
				break
			}
			if !matchedReceipt {
				if len(proofErrors) != 0 {
					errors = append(errors, fmt.Sprintf("evidence %s owner receipt lacks exact binding proof: %s", evidence.ID, proofErrors[0]))
				} else {
					errors = append(errors, fmt.Sprintf("evidence %s lacks accepted owner receipt for %s/%s", evidence.ID, evidence.DispatchID, evidence.OperationID))
				}
			}
		}
		for _, proof := range binding.Proofs {
			if _, matched := matchedProofKinds[proof.EvidenceKind]; !matched {
				errors = append(errors, fmt.Sprintf("satisfied coverage %s lacks exact %s proof chain", coverage.ID, proof.EvidenceKind))
			}
		}
	}
	if requireComplete {
		if len(bindingByID) == 0 {
			errors = append(errors, "completed Run lacks immutable coverage bindings")
		}
		for _, coverage := range audit.Coverage {
			if strings.TrimSpace(coverage.State) != "satisfied" {
				errors = append(errors, fmt.Sprintf("completed Run retains unsatisfied coverage %s", coverage.ID))
			}
		}
	}
	return errors
}

func runtimeCoverageBinding(value projectioncontract.RuntimeAuditCoverageBinding) (runtimecontract.CoverageEvidenceBinding, error) {
	if strings.TrimSpace(value.ID) == "" || strings.TrimSpace(value.CoverageWorkItemID) == "" || strings.TrimSpace(value.FocusID) == "" ||
		strings.TrimSpace(value.DefinitionRevision) == "" || strings.TrimSpace(value.Hash) == "" || len(value.Proofs) == 0 {
		return runtimecontract.CoverageEvidenceBinding{}, fmt.Errorf("audit coverage binding lacks immutable identity")
	}
	proofs := make([]runtimecontract.CoverageEvidenceProof, 0, len(value.Proofs))
	for _, projected := range value.Proofs {
		proof := runtimecontract.CoverageEvidenceProof{
			EvidenceKind: projected.EvidenceKind, ValidationRecipeID: projected.ValidationRecipeID,
			ValidationProofRequirementHash: projected.ValidationProofRequirementHash,
			RequiredCheckRefs:              append([]string(nil), projected.RequiredCheckRefs...), MetricConstraints: append([]string(nil), projected.MetricConstraints...),
			ArtifactRefs: append([]string(nil), projected.ArtifactRefs...), AllowedOperationKinds: append([]string(nil), projected.AllowedOperationKinds...),
		}
		for _, scope := range projected.RequiredEffectScopes {
			proof.RequiredEffectScopes = append(proof.RequiredEffectScopes, runtimecontract.RuntimeEffectScope(scope))
		}
		for _, raw := range projected.AllowedOperationVariants {
			parts := strings.SplitN(strings.TrimSpace(raw), ":", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				return runtimecontract.CoverageEvidenceBinding{}, fmt.Errorf("binding %s has malformed operation variant %q", value.ID, raw)
			}
			proof.AllowedOperationVariants = append(proof.AllowedOperationVariants, runtimecontract.RuntimeOperationVariantRef{Kind: parts[0], VariantID: parts[1]})
		}
		for _, attestation := range projected.ValidationCheckAttestations {
			proof.ValidationCheckAttestations = append(proof.ValidationCheckAttestations, runtimecontract.RuntimeValidationCheckAttestationBinding{
				RequiredCheckRef: attestation.RequiredCheckRef, ObservationProtocolRef: attestation.ObservationProtocolRef, NativeCheckRef: attestation.NativeCheckRef,
			})
		}
		proofs = append(proofs, proof)
	}
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: value.ID, CoverageWorkItemID: common.CoverageWorkItemID(value.CoverageWorkItemID), FocusID: value.FocusID,
		DefinitionRevision: value.DefinitionRevision, Hash: value.Hash, Proofs: proofs, TargetRefs: append([]string(nil), value.TargetRefs...),
		TargetSelectorRef: value.TargetSelectorRef, TargetSelectorOwnerRef: value.TargetSelectorOwnerRef, TargetSelectorSubjectRef: value.TargetSelectorSubjectRef,
		TargetSelectorCandidateRefs:          append([]string(nil), value.TargetSelectorCandidateRefs...),
		ProspectiveTargetAllOfConstraintRefs: append([]string(nil), value.ProspectiveTargetAllOfConstraintRefs...),
		ProspectiveTargetRefs:                append([]string(nil), value.ProspectiveTargetRefs...), DeclarationRef: value.DeclarationRef,
		PrerequisiteRefs: append([]string(nil), value.PrerequisiteRefs...),
	}
	if err := binding.Validate(); err != nil {
		return runtimecontract.CoverageEvidenceBinding{}, fmt.Errorf("audit coverage binding %s is invalid: %w", value.ID, err)
	}
	return binding.Normalized(), nil
}

func runtimeOperationReceipt(value projectioncontract.RuntimeAuditOperationReceipt) runtimecontract.RuntimeOperationReceipt {
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: value.ExecutionKey, Fence: value.Fence, OperationID: value.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcome(value.Outcome), ObservedEffectScope: runtimecontract.RuntimeEffectScope(value.ObservedEffectScope),
		Changed: value.Changed, TargetRefs: append([]string(nil), value.TargetRefs...),
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Quality: common.EvidenceQuality(value.Quality), ZeroWork: value.ZeroWork, EvidenceReason: value.EvidenceReason,
			EvidenceRefs: append([]string(nil), value.EvidenceRefs...), EvidenceTags: append([]string(nil), value.EvidenceTags...),
		},
	}
	if value.ValidationRecipeID != "" || value.ObservationProtocolRef != "" || value.FailureDetailsTruncated || value.OutputTruncated ||
		len(value.ExecutedChecks)+len(value.PassedChecks)+len(value.FailedChecks)+len(value.SkippedChecks)+len(value.IncompleteChecks)+len(value.CheckFailures) != 0 {
		receipt.Evidence.Validation = &runtimecontract.RuntimeValidationObservation{
			RecipeID: value.ValidationRecipeID, ObservationProtocolRef: value.ObservationProtocolRef,
			ExecutedChecks: append([]string(nil), value.ExecutedChecks...), PassedChecks: append([]string(nil), value.PassedChecks...),
			FailedChecks: append([]string(nil), value.FailedChecks...), SkippedChecks: append([]string(nil), value.SkippedChecks...),
			IncompleteChecks: append([]string(nil), value.IncompleteChecks...), CheckFailures: append([]runtimecontract.RuntimeValidationCheckFailure(nil), value.CheckFailures...),
			FailureDetailsTruncated: value.FailureDetailsTruncated, OutputTruncated: value.OutputTruncated,
		}
	}
	return receipt.Normalized()
}

func sameValidationObservation(receipt *runtimecontract.RuntimeValidationObservation, evidence projectioncontract.RuntimeAuditEvidence) bool {
	hasEvidenceObservation := evidence.ValidationRecipeID != "" || evidence.ObservationProtocolRef != "" || evidence.FailureDetailsTruncated || evidence.OutputTruncated ||
		len(evidence.ExecutedChecks)+len(evidence.PassedChecks)+len(evidence.FailedChecks)+len(evidence.SkippedChecks)+len(evidence.IncompleteChecks)+len(evidence.CheckFailures) != 0
	if receipt == nil {
		return !hasEvidenceObservation
	}
	if !hasEvidenceObservation {
		return false
	}
	value := receipt.Normalized()
	details := (runtimecontract.RuntimeValidationObservation{CheckFailures: evidence.CheckFailures}).Normalized().CheckFailures
	return value.RecipeID == strings.TrimSpace(evidence.ValidationRecipeID) && value.ObservationProtocolRef == strings.TrimSpace(evidence.ObservationProtocolRef) &&
		sameStrings(value.ExecutedChecks, evidence.ExecutedChecks) && sameStrings(value.PassedChecks, evidence.PassedChecks) &&
		sameStrings(value.FailedChecks, evidence.FailedChecks) && sameStrings(value.SkippedChecks, evidence.SkippedChecks) &&
		sameStrings(value.IncompleteChecks, evidence.IncompleteChecks) && slices.Equal(value.CheckFailures, details) &&
		value.FailureDetailsTruncated == evidence.FailureDetailsTruncated && value.OutputTruncated == evidence.OutputTruncated
}

func contains(values []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	return strings.Join(normalizeStrings(left), "\x00") == strings.Join(normalizeStrings(right), "\x00")
}

func normalizeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			seen[value] = struct{}{}
		}
	}
	ret := make([]string, 0, len(seen))
	for value := range seen {
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}
