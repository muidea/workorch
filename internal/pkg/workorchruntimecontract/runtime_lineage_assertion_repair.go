package workorchruntimecontract

import (
	"fmt"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeRepairHistoryEvidenceRevision 反证已退休 repair 的历史完成链。
// 它不读取当前 focus/coverage 满足位；handoff 被消费或后续再失败均不抹掉历史。
func RuntimeRepairHistoryEvidenceRevision(sources RuntimeLineageAssertionSources, coverageID common.CoverageWorkItemID, lineage RuntimeCWIReopenLineage, closure RuntimeRepairClosure, cutoff string) (string, error) {
	return runtimeRepairHistoryEvidenceRevisionInSession(sources, coverageID, lineage, closure, cutoff, nil)
}

func runtimeRepairHistoryEvidenceRevisionInSession(sources RuntimeLineageAssertionSources, coverageID common.CoverageWorkItemID, lineage RuntimeCWIReopenLineage, closure RuntimeRepairClosure, cutoff string, session *runtimeHistoricalPreimageSession) (string, error) {
	invalid := func(detail string) (string, error) { return "", fmt.Errorf("runtime repair history: %s", detail) }
	if lineage.Validate() != nil || lineage.Reason != "validation_failure_repair_settled" || closure.Validate() != nil || !runtimeCanonicalLineageCutoff(cutoff) {
		return invalid("invalid retired binding, closure or cutoff")
	}
	if err := ValidateRuntimeEvidenceLedgerSequence(sources.EvidenceSequence, sources.Facts, sources.Active, sources.Completed, sources.Resolutions); err != nil {
		return "", err
	}
	if err := runtimeValidateLineageOperationsInSession(sources, session); err != nil {
		return "", err
	}
	entries, err := session.preimages(sources.Semantic)
	if err != nil {
		return "", err
	}
	var matched runtimeAssessmentPurposeBindingPreimage
	var binding CoverageEvidenceBinding
	matches := 0
	for _, entry := range entries {
		if entry.semanticManifestHash != lineage.PreviousBindingRef.SemanticManifestHash {
			continue
		}
		for _, candidate := range entry.bindings {
			if runtimeLineageBindingMatches(candidate, coverageID, lineage.PreviousBindingRef) {
				matched, binding, matches = entry, candidate, matches+1
			}
		}
	}
	if matches != 1 || matched.obligation.ValidationFailureRepair == nil {
		return invalid("no unique repair semantic preimage")
	}
	repair := *matched.obligation.ValidationFailureRepair
	if repair.Validate() != nil || !repair.RequireFreshValidation || closure.GapSignature != repair.FailureSignature || !closure.RequireValidationAfterStateChange || !slices.Equal(closure.CoverageWorkItemIDs, []common.CoverageWorkItemID{coverageID}) || !sameRuntimeStringSet(closure.RequiredEvidenceKinds, []string{EvidenceKindStateChange, EvidenceKindValidation}) {
		return invalid("closure differs from sealed repair")
	}
	if matched.obligation.CoverageWorkItemID != coverageID || binding.Validate() != nil {
		return invalid("retired binding differs from exact CWI")
	}
	repairTargets, err := runtimeRepairHistoryBindingTargets(binding, repair)
	if err != nil || !slices.Equal(repairTargets, closure.GroundedTargetRefs) {
		return invalid("retired binding targets differ from closure")
	}
	var trigger EvidenceFact
	matches = 0
	for _, fact := range sources.Facts.Evidence.Accepted {
		if string(fact.ID) == lineage.TriggerRef {
			trigger, matches = fact, matches+1
		}
	}
	if matches != 1 || EvidenceRevisionAfter(trigger.Revision, cutoff) || !EvidenceRevisionAfter(trigger.Revision, closure.BaselineEvidenceRevision) {
		return invalid("no exact repair trigger within cutoff")
	}
	var dispatch RuntimeDispatchRecord
	matches = 0
	for _, value := range sources.Completed {
		if value.ID == trigger.DispatchID {
			dispatch, matches = value, matches+1
		}
	}
	if matches != 1 || dispatch.State != RuntimeDispatchStateCommitted || dispatch.Candidate.StateChange == nil || len(dispatch.CompletedOperationIDs) != len(runtimeCandidateAdmissionOperations(dispatch.Candidate)) {
		return invalid("repair dispatch incomplete")
	}
	if !RuntimeCoverageEvidenceBindingProjectionsEqual(dispatch.CoverageBindings, matched.bindings) {
		return invalid("dispatch differs from retired binding")
	}
	manifestMatches := 0
	for _, session := range sources.Sessions {
		if session.CandidateManifest != nil && session.CandidateManifest.Hash == dispatch.CandidateManifestHash && session.Transaction == dispatch.Transaction && session.CandidateManifest.RepairClosureID == closure.ID && RuntimeCoverageEvidenceBindingProjectionsEqual(session.CandidateManifest.Bindings, dispatch.CoverageBindings) {
			manifestMatches++
		}
	}
	if manifestMatches != 1 {
		return invalid("closure has no unique pre-seal manifest")
	}
	var rejected EvidenceFact
	var rejectedReceipt RuntimeOperationReceipt
	var rejectedOperation RuntimeOperation
	matches = 0
	for _, sourceDispatch := range sources.Completed {
		for _, operation := range runtimeCandidateAdmissionOperations(sourceDispatch.Candidate) {
			fact, receipt, valid := runtimeLineageOperationFactFromLedgersInIndex(sourceDispatch, operation, sources, false, false, false, session.lineageIndex())
			if !valid || fact.Kind != EvidenceKindValidation || fact.Revision != closure.BaselineEvidenceRevision || fact.FocusID != repair.FailedFocusID || fact.FailureSignature != repair.FailureSignature || !slices.Contains(fact.CoverageWorkItemIDs, coverageID) {
				continue
			}
			rejected, rejectedReceipt, rejectedOperation, matches = fact, receipt, operation, matches+1
		}
	}
	if matches != 1 || rejectedReceipt.Feedback == nil || rejectedReceipt.Feedback.OriginalDiagnostic == nil || rejectedReceipt.Feedback.OriginalDiagnostic.ValidationDiagnostic == nil {
		return invalid("no exact rejected validation diagnostic")
	}
	diagnostic := *rejectedReceipt.Feedback.OriginalDiagnostic.ValidationDiagnostic
	descriptor, known := RuntimeOperationDescriptorForKind(rejectedOperation.Kind)
	if !known || !repair.route().MatchesTrigger(descriptor.OwnerModuleID, rejectedOperation.Kind, diagnostic.Class, diagnostic.RecommendedEvidenceTargetType, diagnostic.OwnerCode) {
		return invalid("rejection differs from sealed route")
	}
	var source RuntimeCoverageItem
	for _, item := range sources.Facts.DefinitionSeed.CoverageItems {
		if item.ID == coverageID {
			source = item
		}
	}
	policy, err := CompileRuntimeSourceEvidencePolicy(source.SourceScope, source.SourceID, sources.Facts.Activation.ExecutionContract.ProfileContracts, nil)
	if err != nil || policy.ValidationFailureRepairConvergence == nil || policy.ValidationFailureRepairConvergence.ID != repair.ConvergenceID || StableRuntimeValidationFailureRepairConvergenceHash(*policy.ValidationFailureRepairConvergence) != repair.ConvergenceHash {
		return invalid("route is not sealed Profile authority")
	}
	route, found := policy.ValidationFailureRepairConvergence.RouteForTrigger(descriptor.OwnerModuleID, rejectedOperation.Kind, diagnostic.Class, diagnostic.RecommendedEvidenceTargetType, diagnostic.OwnerCode)
	if !found || StableRuntimeValidationFailureRepairRouteHash(route) != repair.RouteHash {
		return invalid("route hash differs")
	}
	if route.RequiresStructuredObservation() {
		if dispatch.RepairCommitment == nil {
			return invalid("confirmed repair omitted commitment")
		}
		commitment := *dispatch.RepairCommitment
		if ValidateRuntimeDispatchRepairCommitmentProjection(dispatch.RepairCommitmentHash, dispatch.RepairCommitment) != nil || commitment.Closure.ID != closure.ID || commitment.SourceEvidenceFactID != rejected.ID || commitment.SourceEvidenceFactHash != StableRuntimeEvidenceFactHash(rejected) || commitment.SourceDispatchID != rejected.DispatchID || commitment.SourceAuthoritySnapshotID != rejected.AuthoritySnapshotID || commitment.SourceOperationID != rejectedOperation.ID || commitment.SourceOperationHash != StableRuntimeOperationHash(rejectedOperation) || commitment.SourceReceiptExecutionKey != rejectedReceipt.ExecutionKey || commitment.SourceReceiptFence != rejectedReceipt.Fence || commitment.SourceReceiptHash != session.lineageIndex().receiptHash(rejectedReceipt) || commitment.RepairOwnerModuleID != repair.RepairOwnerModuleID {
			return invalid("repair commitment source mismatch")
		}
		confirmations := 0
		for _, record := range sources.Confirmations {
			if record.IdentitySignature != commitment.ConfirmationIdentityHash || record.Signature != commitment.ConfirmationHash {
				continue
			}
			confirmation := record.Confirmation
			observation := confirmation.Observation
			if session.lineageIndex().confirmation(record) != nil || confirmation.Disposition != RuntimeRepairSurfaceConfirmed || confirmation.OwnerModuleID != repair.RepairOwnerModuleID || !slices.Equal(confirmation.ConfirmedTargetRefs, closure.GroundedTargetRefs) || observation.WorkspaceID != sources.Facts.WorkspaceID || observation.RunID != sources.Facts.RunID || observation.DispatchID != rejected.DispatchID || observation.OperationID != rejected.OperationID || observation.AuthoritySnapshotID != rejected.AuthoritySnapshotID || observation.FailureSignature != rejected.FailureSignature || observation.DiagnosticClass != diagnostic.Class || observation.DiagnosticHash != StableRuntimeValidationFailureObservationDiagnosticHash(diagnostic, observation.ValidationScopeTargetRefs) {
				return invalid("confirmation differs from source diagnostic")
			}
			confirmations++
		}
		if confirmations != 1 {
			return invalid("no unique owner confirmation")
		}
	}
	var targets []string
	finalRevision := ""
	for _, operation := range runtimeCandidateAdmissionOperations(dispatch.Candidate) {
		descriptor, known := RuntimeOperationDescriptorForKind(operation.Kind)
		fact, receipt, valid := runtimeLineageOperationFactFromLedgersInIndex(dispatch, operation, sources, true, false, false, session.lineageIndex())
		if !known || MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindStateChange, operation, receipt) != nil || descriptor.OwnerModuleID != repair.RepairOwnerModuleID || !slices.Contains(repair.RepairOperationKinds, operation.Kind) || !valid || !receipt.Changed || !RuntimeEffectIsDurable(receipt.ObservedEffectScope) || fact.Kind != EvidenceKindStateChange || !slices.Equal(fact.BindingRefs, operation.BindingRefs) || !slices.Contains(fact.BindingRefs, binding.ID) || !slices.Equal(fact.CoverageWorkItemIDs, closure.CoverageWorkItemIDs) || !EvidenceRevisionAfter(fact.Revision, rejected.Revision) || EvidenceRevisionAfter(fact.Revision, cutoff) {
			return invalid("repair lacks complete changed receipt set")
		}
		targets = append(targets, receipt.TargetRefs...)
		if EvidenceRevisionAfter(fact.Revision, finalRevision) {
			finalRevision = fact.Revision
		}
	}
	if !sameRuntimeStringSet(targets, repairTargets) || finalRevision != trigger.Revision {
		return invalid("repair final trigger differs from all-of receipts")
	}
	return finalRevision, nil
}

// The original sealed policy determines the target field. Existing-file repair
// cannot be reinterpreted as prospective input creation (or vice versa), and
// neither kind may borrow a second target set from another binding surface.
func runtimeRepairHistoryBindingTargets(binding CoverageEvidenceBinding, repair RuntimeSemanticValidationFailureRepair) ([]string, error) {
	var targets []string
	switch repair.RepairTargetPolicy {
	case RuntimeOperationTargetPolicyExactTarget, RuntimeOperationTargetPolicyExistingOwnerScope:
		if len(binding.ProspectiveTargetRefs) != 0 {
			return nil, fmt.Errorf("existing repair carries prospective targets")
		}
		targets = binding.TargetRefs
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if len(binding.TargetRefs) != 0 {
			return nil, fmt.Errorf("prospective repair carries existing targets")
		}
		targets = binding.ProspectiveTargetRefs
	default:
		return nil, fmt.Errorf("repair history requires original target policy")
	}
	if len(targets) == 0 || !slices.Equal(targets, normalizeStrings(targets)) {
		return nil, fmt.Errorf("repair history requires exact finite canonical target set")
	}
	return slices.Clone(targets), nil
}

// runtimeRepairThenFreshValidationSatisfied 只接受原完整 CWI 的新普通 proof，不能借 repair credit。
func runtimeRepairThenFreshValidationSatisfied(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, cutoff string) bool {
	return runtimeRepairThenFreshValidationInSession(binding, sources, cutoff, &runtimeHistoricalPreimageSession{})
}

func runtimeRepairThenFreshValidationInSession(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, cutoff string, session *runtimeHistoricalPreimageSession) bool {
	entries, err := session.preimages(sources.Semantic)
	if err != nil {
		return false
	}
	ordinary := 0
	for _, entry := range entries {
		if !RuntimeCoverageEvidenceBindingProjectionsEqual(entry.bindings, []CoverageEvidenceBinding{binding}) {
			continue
		}
		purpose, err := resolveRuntimeSemanticEvidenceObligationInSession(entry.obligation, session.semanticSession())
		if err == nil && purpose.Purpose == RuntimeSemanticEvidenceObligationDefinitionProof {
			ordinary++
		}
	}
	if ordinary != 1 {
		return false
	}
	for _, item := range sources.Facts.Coverage {
		if item.ID != binding.CoverageWorkItemID {
			continue
		}
		for _, lineage := range item.ReopenLineage {
			if lineage.Reason != "validation_failure_repair_settled" {
				continue
			}
			for _, dispatch := range sources.Completed {
				if dispatch.State != RuntimeDispatchStateCommitted || dispatch.Candidate.Validation == nil || dispatch.RepairCommitment != nil || !RuntimeCoverageEvidenceBindingProjectionsEqual(dispatch.CoverageBindings, []CoverageEvidenceBinding{binding}) {
					continue
				}
				closure, found := runtimeRepairClosureForLineage(sources, binding.CoverageWorkItemID, lineage)
				if !found {
					continue
				}
				freshManifestMatches := 0
				for _, session := range sources.Sessions {
					if session.CandidateManifest != nil && session.CandidateManifest.Hash == dispatch.CandidateManifestHash && session.Transaction == dispatch.Transaction && session.CandidateManifest.RepairClosureID == closure.ID && session.CandidateManifest.RepairPurpose == "" && RuntimeCoverageEvidenceBindingProjectionsEqual(session.CandidateManifest.Bindings, []CoverageEvidenceBinding{binding}) {
						freshManifestMatches++
					}
				}
				if freshManifestMatches != 1 {
					continue
				}
				repairRevision, err := runtimeRepairHistoryEvidenceRevisionInSession(sources, binding.CoverageWorkItemID, lineage, closure, cutoff, session)
				if err != nil {
					continue
				}
				proof, found := CoverageEvidenceProofForKind(binding, EvidenceKindValidation)
				if !found || !runtimeFreshBindingInputReviewInSession(binding, sources.Semantic, session) {
					continue
				}
				var accepted []EvidenceFact
				complete := true
				for _, operation := range runtimeCandidateAdmissionOperations(dispatch.Candidate) {
					fact, receipt, valid := runtimeLineageOperationFactFromLedgersInIndex(dispatch, operation, sources, true, false, false, session.lineageIndex())
					if !valid || MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindValidation, operation, receipt) != nil || fact.Kind != EvidenceKindValidation || !slices.Contains(fact.BindingRefs, binding.ID) || !slices.Equal(fact.CoverageWorkItemIDs, []common.CoverageWorkItemID{binding.CoverageWorkItemID}) || !EvidenceRevisionAfter(fact.Revision, repairRevision) || EvidenceRevisionAfter(fact.Revision, cutoff) || fact.Validation == nil || !RuntimeValidationObservationSatisfiesCoverageProof(proof, *fact.Validation) {
						complete = false
						break
					}
					accepted = append(accepted, fact)
				}
				if complete && len(accepted) != 0 && assessmentProspectiveTargetsComplete(append(slices.Clone(binding.TargetRefs), binding.ProspectiveTargetRefs...), accepted) {
					freshRevision := ""
					for _, fact := range accepted {
						if EvidenceRevisionAfter(fact.Revision, freshRevision) {
							freshRevision = fact.Revision
						}
					}
					laterFailure := false
					for _, fact := range sources.Facts.Evidence.Rejected {
						if fact.Kind == EvidenceKindValidation && slices.Contains(fact.CoverageWorkItemIDs, binding.CoverageWorkItemID) && EvidenceRevisionAfter(fact.Revision, freshRevision) && !EvidenceRevisionAfter(fact.Revision, cutoff) {
							laterFailure = true
						}
					}
					if !laterFailure {
						return true
					}
				}
			}
		}
	}
	return false
}

// runtimeRepairClosureForLineage 从原拒绝与 retired repair manifest 重建 closure；不猜自然语言。
func runtimeRepairClosureForLineage(sources RuntimeLineageAssertionSources, coverageID common.CoverageWorkItemID, lineage RuntimeCWIReopenLineage) (RuntimeRepairClosure, bool) {
	entries, err := runtimeAssessmentPurposePreimagesFromSemanticLedger(sources.Semantic, false)
	if err != nil {
		return RuntimeRepairClosure{}, false
	}
	var matched runtimeAssessmentPurposeBindingPreimage
	var binding CoverageEvidenceBinding
	matches := 0
	for _, entry := range entries {
		if entry.semanticManifestHash != lineage.PreviousBindingRef.SemanticManifestHash || entry.obligation.CoverageWorkItemID != coverageID || entry.obligation.ValidationFailureRepair == nil {
			continue
		}
		for _, candidate := range entry.bindings {
			if runtimeLineageBindingMatches(candidate, coverageID, lineage.PreviousBindingRef) {
				matched, binding, matches = entry, candidate, matches+1
			}
		}
	}
	if matches != 1 {
		return RuntimeRepairClosure{}, false
	}
	var trigger EvidenceFact
	matches = 0
	for _, fact := range sources.Facts.Evidence.Accepted {
		if string(fact.ID) == lineage.TriggerRef {
			trigger, matches = fact, matches+1
		}
	}
	if matches != 1 {
		return RuntimeRepairClosure{}, false
	}
	var dispatch RuntimeDispatchRecord
	matches = 0
	for _, candidate := range sources.Completed {
		if candidate.ID == trigger.DispatchID && candidate.State == RuntimeDispatchStateCommitted && RuntimeCoverageEvidenceBindingProjectionsEqual(candidate.CoverageBindings, matched.bindings) {
			dispatch, matches = candidate, matches+1
		}
	}
	if matches != 1 {
		return RuntimeRepairClosure{}, false
	}
	closureID := ""
	matches = 0
	for _, session := range sources.Sessions {
		manifest := session.CandidateManifest
		if manifest != nil && manifest.Hash == dispatch.CandidateManifestHash && session.Transaction == dispatch.Transaction && RuntimeCoverageEvidenceBindingProjectionsEqual(manifest.Bindings, dispatch.CoverageBindings) && manifest.RepairClosureID != "" {
			closureID, matches = manifest.RepairClosureID, matches+1
		}
	}
	if matches != 1 {
		return RuntimeRepairClosure{}, false
	}
	if dispatch.RepairCommitment != nil {
		closure := dispatch.RepairCommitment.Closure
		return closure, closure.ID == closureID && closure.Validate() == nil
	}
	repair := matched.obligation.ValidationFailureRepair
	targets, err := runtimeRepairHistoryBindingTargets(binding, *repair)
	if err != nil {
		return RuntimeRepairClosure{}, false
	}
	var closure RuntimeRepairClosure
	matches = 0
	for _, fact := range sources.Facts.Evidence.Rejected {
		if fact.Kind != EvidenceKindValidation || fact.FocusID != repair.FailedFocusID || fact.FailureSignature != repair.FailureSignature || !slices.Equal(fact.CoverageWorkItemIDs, []common.CoverageWorkItemID{coverageID}) {
			continue
		}
		candidate := RuntimeRepairClosure{GapSignature: repair.FailureSignature, BaselineEvidenceRevision: fact.Revision, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, RequiredEvidenceKinds: []string{EvidenceKindStateChange, EvidenceKindValidation}, GroundedTargetRefs: targets, RequireValidationAfterStateChange: true}.Normalized()
		candidate.ID = StableRuntimeRepairClosureID(candidate)
		if candidate.ID == closureID {
			closure, matches = candidate, matches+1
		}
	}
	return closure, matches == 1
}

// runtimeFreshBindingInputReview 仅消费产生 exact binding 的 accepted batch；其它历史报告不能补证。
func runtimeFreshBindingInputReview(binding CoverageEvidenceBinding, ledger RuntimeSemanticPlanningLedger) bool {
	return runtimeFreshBindingInputReviewInSession(binding, ledger, nil)
}

func runtimeFreshBindingInputReviewInSession(binding CoverageEvidenceBinding, ledger RuntimeSemanticPlanningLedger, session *runtimeHistoricalPreimageSession) bool {
	_, err := compileRuntimeBindingValidationInputReviewInSession(binding, ledger, session)
	return err == nil
}

// CompileRuntimeBindingValidationInputReview returns only the original owner
// subject from the unique accepted semantic batch that produced this exact
// binding. A consistent report from another generation cannot certify it.
// Callers must independently validate the enclosing aggregate/current proof.
func CompileRuntimeBindingValidationInputReview(binding CoverageEvidenceBinding, ledger RuntimeSemanticPlanningLedger) (RuntimeValidationProofRequirementBinding, error) {
	return compileRuntimeBindingValidationInputReviewInSession(binding, ledger, nil)
}

func compileRuntimeBindingValidationInputReviewInSession(binding CoverageEvidenceBinding, ledger RuntimeSemanticPlanningLedger, session *runtimeHistoricalPreimageSession) (RuntimeValidationProofRequirementBinding, error) {
	entries, err := session.preimages(ledger)
	if err != nil {
		return RuntimeValidationProofRequirementBinding{}, err
	}
	// preimages has just compared the complete detached raw ledger. Within
	// this call-local session only, the exact binding can reuse its pure review
	// projection. Never retain this with cross-event historical products.
	if session != nil {
		for _, prior := range session.reviews {
			if runtimeRawEqual(prior.binding, binding) {
				return prior.result.Clone(), nil
			}
		}
	}
	var matched runtimeAssessmentPurposeBindingPreimage
	matches := 0
	for _, entry := range entries {
		for _, candidate := range entry.bindings {
			if candidate.CoverageWorkItemID == binding.CoverageWorkItemID && candidate.ID == binding.ID && candidate.Hash == binding.Hash && candidate.FocusID == binding.FocusID && RuntimeCoverageEvidenceBindingProjectionsEqual([]CoverageEvidenceBinding{candidate}, []CoverageEvidenceBinding{binding}) {
				matched, matches = entry, matches+1
			}
		}
	}
	if matches != 1 {
		return RuntimeValidationProofRequirementBinding{}, fmt.Errorf("input review requires unique exact accepted binding source")
	}
	purpose, err := resolveRuntimeSemanticEvidenceObligationInSession(matched.obligation, session.semanticSession())
	if err != nil || purpose.Purpose != RuntimeSemanticEvidenceObligationDefinitionProof {
		return RuntimeValidationProofRequirementBinding{}, fmt.Errorf("input review requires original Definition proof purpose")
	}
	proof, found := CoverageEvidenceProofForKind(binding, EvidenceKindValidation)
	if !found {
		return RuntimeValidationProofRequirementBinding{}, fmt.Errorf("input review requires validation proof")
	}
	reviews := 0
	var result RuntimeValidationProofRequirementBinding
	for _, proposal := range matched.acceptedDocument.Proposals {
		if proposal.CoverageWorkItemID != binding.CoverageWorkItemID || proposal.DeclarationRef != binding.DeclarationRef || !slices.Contains(proposal.RequiredEvidenceKinds, EvidenceKindValidation) {
			continue
		}
		if !slices.Contains(proof.AllowedOperationKinds, proposal.OperationKind) || proposal.ValidationRecipeID != proof.ValidationRecipeID || proposal.ValidationInputReview == nil {
			return RuntimeValidationProofRequirementBinding{}, fmt.Errorf("input review proposal differs from sealed validation proof")
		}
		requirement, found := RuntimeSemanticValidationInputReviewBinding(proposal, matched.ownerCatalogs)
		if !found || requirement.InputReview == nil || requirement.DefinitionRevision != binding.DefinitionRevision || requirement.RequirementHash != proof.ValidationProofRequirementHash || !slices.Equal(requirement.RequiredCheckRefs, proof.RequiredCheckRefs) || !slices.Equal(requirement.CheckAttestations, proof.ValidationCheckAttestations) || requirement.InputReview.ValidateAgainstBinding(requirement) != nil || proposal.ValidationInputReview.ValidateAgainstBinding(requirement) != nil || RuntimeValidationInputReviewConclusion(*proposal.ValidationInputReview) != RuntimeValidationInputConsistent {
			return RuntimeValidationProofRequirementBinding{}, fmt.Errorf("input review has no complete original consistent subject/report")
		}
		result = requirement.Clone()
		reviews++
	}
	if reviews != 1 {
		return RuntimeValidationProofRequirementBinding{}, fmt.Errorf("input review requires unique original consistent report")
	}
	if session != nil {
		session.reviews = append(session.reviews, runtimeHistoricalInputReviewPreimage{binding: cloneCoverageEvidenceBindingRaw(binding), result: result.Clone()})
	}
	return result, nil
}

// runtimeLineageBindingMatches 精确反查同一原 CWI/BindingID/Hash，不使用首项或集合交集。
func runtimeLineageBindingMatches(binding CoverageEvidenceBinding, coverageID common.CoverageWorkItemID, ref RuntimeCWIBindingRef) bool {
	return binding.CoverageWorkItemID == coverageID && binding.ID == ref.BindingID && binding.Hash == ref.BindingHash && binding.FocusID == ref.FocusID
}
