package workorchruntimecontract

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	"reflect"
	"slices"
	"strings"
)

// Independent pre-session consumer paths preserve gate order and full results.

func referenceLineagePreimageRuntimeExecutionLineageAssertionsSatisfied(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, evidenceRevision string) bool {
	// 未有任何 Profile guard 且 binding 也为空时，新增门禁不改变普通证明。
	if len(binding.LineageAssertions) == 0 && !runtimeProfilesDeclareLineageAssertions(sources.Facts.Activation.ExecutionContract.ProfileContracts) {
		return true
	}
	expected, err := runtimeExpectedLineageAssertions(binding, sources)
	if err != nil {
		return false
	}
	if len(expected) == 0 {
		return true
	}
	if binding.Validate() != nil || sources.Definition.StartupInputHash == "" {
		return false
	}
	executionRequired := false
	for _, assertion := range expected {
		executionRequired = executionRequired || assertion.Rule.ConsumptionPoint == RuntimeProofPhaseExecution
	}
	if !executionRequired {
		return true
	}
	if !runtimeCanonicalLineageCutoff(evidenceRevision) || ValidateRuntimeEvidenceLedgerSequence(sources.EvidenceSequence, sources.Facts, sources.Active, sources.Completed, sources.Resolutions) != nil || EvidenceRevisionAfter(evidenceRevision, sources.Facts.EvidenceRevision) {
		return false
	}
	for _, assertion := range expected {
		if assertion.Rule.ConsumptionPoint == RuntimeProofPhasePostTerminalDelivery {
			continue
		}
		proofBinding := binding
		if assertion.Rule.ExecutionSourceContractRef != "" {
			var err error
			proofBinding, err = referenceLineagePreimageruntimeExecutionProofSourceBinding(assertion, sources)
			if err != nil {
				return false
			}
		}
		switch assertion.Rule.Kind {
		case RuntimeLineageCompletedWorkRounds:
			if !runtimeCompletedWorkRoundsSatisfied(proofBinding.CoverageWorkItemID, assertion.Rule.MinCompletedRounds, sources, evidenceRevision) {
				return false
			}
		case RuntimeLineageOwnerOperationReceipt:
			if !runtimeOwnerOperationReceiptSatisfied(binding.CoverageWorkItemID, assertion.Rule, sources, evidenceRevision) {
				return false
			}
		case RuntimeLineageRepairThenFreshValidation:
			satisfied := false
			if assertion.Rule.RepairHistoryContractRef == RuntimeConfirmedBindingRepairProofV1 {
				satisfied = referenceLineagePreimageruntimeConfirmedBindingRepairSatisfied(proofBinding, sources, evidenceRevision)
			} else if assertion.Rule.RepairHistoryContractRef == "" {
				satisfied = referenceLineagePreimageruntimeRepairThenFreshValidationSatisfied(proofBinding, sources, evidenceRevision)
			}
			if !satisfied {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func referenceLineagePreimageruntimeRepairThenFreshValidationSatisfied(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, cutoff string) bool {
	entries, err := runtimeAssessmentPurposePreimagesFromSemanticLedger(sources.Semantic, false)
	if err != nil {
		return false
	}
	ordinary := 0
	for _, entry := range entries {
		if !RuntimeCoverageEvidenceBindingProjectionsEqual(entry.bindings, []CoverageEvidenceBinding{binding}) {
			continue
		}
		purpose, err := ResolveRuntimeSemanticEvidenceObligation(entry.obligation)
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
				repairRevision, err := referenceLineagePreimageRuntimeRepairHistoryEvidenceRevision(sources, binding.CoverageWorkItemID, lineage, closure, cutoff)
				if err != nil {
					continue
				}
				proof, found := CoverageEvidenceProofForKind(binding, EvidenceKindValidation)
				if !found || !referenceLineagePreimageruntimeFreshBindingInputReview(binding, sources.Semantic) {
					continue
				}
				var accepted []EvidenceFact
				complete := true
				for _, operation := range runtimeCandidateAdmissionOperations(dispatch.Candidate) {
					fact, receipt, valid := runtimeLineageOperationFact(dispatch, operation, sources, true)
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

func referenceLineagePreimageRuntimeRepairHistoryEvidenceRevision(sources RuntimeLineageAssertionSources, coverageID common.CoverageWorkItemID, lineage RuntimeCWIReopenLineage, closure RuntimeRepairClosure, cutoff string) (string, error) {
	invalid := func(detail string) (string, error) { return "", fmt.Errorf("runtime repair history: %s", detail) }
	if lineage.Validate() != nil || lineage.Reason != "validation_failure_repair_settled" || closure.Validate() != nil || !runtimeCanonicalLineageCutoff(cutoff) {
		return invalid("invalid retired binding, closure or cutoff")
	}
	if err := ValidateRuntimeEvidenceLedgerSequence(sources.EvidenceSequence, sources.Facts, sources.Active, sources.Completed, sources.Resolutions); err != nil {
		return "", err
	}
	if err := runtimeValidateLineageOperations(sources); err != nil {
		return "", err
	}
	entries, err := runtimeAssessmentPurposePreimagesFromSemanticLedger(sources.Semantic, false)
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
			fact, receipt, valid := runtimeLineageOperationFact(sourceDispatch, operation, sources, false)
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
		if ValidateRuntimeDispatchRepairCommitmentProjection(dispatch.RepairCommitmentHash, dispatch.RepairCommitment) != nil || commitment.Closure.ID != closure.ID || commitment.SourceEvidenceFactID != rejected.ID || commitment.SourceEvidenceFactHash != StableRuntimeEvidenceFactHash(rejected) || commitment.SourceDispatchID != rejected.DispatchID || commitment.SourceAuthoritySnapshotID != rejected.AuthoritySnapshotID || commitment.SourceOperationID != rejectedOperation.ID || commitment.SourceOperationHash != StableRuntimeOperationHash(rejectedOperation) || commitment.SourceReceiptExecutionKey != rejectedReceipt.ExecutionKey || commitment.SourceReceiptFence != rejectedReceipt.Fence || commitment.SourceReceiptHash != StableRuntimeOperationReceiptHash(rejectedReceipt) || commitment.RepairOwnerModuleID != repair.RepairOwnerModuleID {
			return invalid("repair commitment source mismatch")
		}
		confirmations := 0
		for _, record := range sources.Confirmations {
			if record.IdentitySignature != commitment.ConfirmationIdentityHash || record.Signature != commitment.ConfirmationHash {
				continue
			}
			confirmation := record.Confirmation
			observation := confirmation.Observation
			if record.Validate() != nil || confirmation.Disposition != RuntimeRepairSurfaceConfirmed || confirmation.OwnerModuleID != repair.RepairOwnerModuleID || !slices.Equal(confirmation.ConfirmedTargetRefs, closure.GroundedTargetRefs) || observation.WorkspaceID != sources.Facts.WorkspaceID || observation.RunID != sources.Facts.RunID || observation.DispatchID != rejected.DispatchID || observation.OperationID != rejected.OperationID || observation.AuthoritySnapshotID != rejected.AuthoritySnapshotID || observation.FailureSignature != rejected.FailureSignature || observation.DiagnosticClass != diagnostic.Class || observation.DiagnosticHash != StableRuntimeValidationFailureObservationDiagnosticHash(diagnostic, observation.ValidationScopeTargetRefs) {
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
		fact, receipt, valid := runtimeLineageOperationFact(dispatch, operation, sources, true)
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

func referenceLineagePreimageruntimeFreshBindingInputReview(binding CoverageEvidenceBinding, ledger RuntimeSemanticPlanningLedger) bool {
	_, err := referenceLineagePreimageCompileRuntimeBindingValidationInputReview(binding, ledger)
	return err == nil
}

// CompileRuntimeBindingValidationInputReview returns only the original owner
// subject from the unique accepted semantic batch that produced this exact
// binding. A consistent report from another generation cannot certify it.
// Callers must independently validate the enclosing aggregate/current proof.

func referenceLineagePreimageCompileRuntimeBindingValidationInputReview(binding CoverageEvidenceBinding, ledger RuntimeSemanticPlanningLedger) (RuntimeValidationProofRequirementBinding, error) {
	entries, err := runtimeAssessmentPurposePreimagesFromSemanticLedger(ledger, false)
	if err != nil {
		return RuntimeValidationProofRequirementBinding{}, err
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
	purpose, err := ResolveRuntimeSemanticEvidenceObligation(matched.obligation)
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
	return result, nil
}

// runtimeLineageBindingMatches 精确反查同一原 CWI/BindingID/Hash，不使用首项或集合交集。

func referenceLineagePreimageruntimeConfirmedBindingRepairSatisfied(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, cutoff string) bool {
	if binding.Validate() != nil || !runtimeCanonicalLineageCutoff(cutoff) || sources.Budget.Validate() != nil || runtimeValidateLineageOperations(sources) != nil || ValidateRuntimeRepairSurfaceConfirmationRecords(sources.Confirmations) != nil || !referenceLineagePreimageruntimeFreshBindingInputReview(binding, sources.Semantic) {
		return false
	}
	current := 0
	for _, item := range sources.Facts.Coverage {
		if item.ID == binding.CoverageWorkItemID && item.CurrentBindingRef != nil && runtimeLineageBindingMatches(binding, item.ID, *item.CurrentBindingRef) {
			current++
		}
	}
	if current != 1 {
		return false
	}
	for _, repair := range sources.Completed {
		if repair.RepairCommitment == nil || repair.State != RuntimeDispatchStateCommitted || repair.Candidate.StateChange == nil || repair.Candidate.Validation != nil || repair.ImplementationWorkCommitment != nil || !RuntimeCoverageEvidenceBindingProjectionsEqual(repair.CoverageBindings, []CoverageEvidenceBinding{binding}) {
			continue
		}
		c := *repair.RepairCommitment
		// This first bounded version supports the exact original dispatch, not
		// a partial/governance-resume set assembled across different dispatches.
		if ValidateRuntimeDispatchRepairCommitmentProjection(repair.RepairCommitmentHash, repair.RepairCommitment) != nil || c.InputRepair != nil || c.OriginDispatchID != repair.ID || c.FocusID != repair.Transaction.FocusID || !slices.Equal(c.Closure.CoverageWorkItemIDs, []common.CoverageWorkItemID{binding.CoverageWorkItemID}) {
			continue
		}
		rejected, receipt, operation, ok := runtimeConfirmedBindingRepairSource(binding, sources, repair)
		if !ok {
			continue
		}
		diagnostic := *receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic
		var source RuntimeCoverageItem
		for _, item := range sources.Facts.DefinitionSeed.CoverageItems {
			if item.ID == binding.CoverageWorkItemID {
				source = item
			}
		}
		policy, err := CompileRuntimeSourceEvidencePolicy(source.SourceScope, source.SourceID, sources.Facts.Activation.ExecutionContract.ProfileContracts, nil)
		descriptor, known := RuntimeOperationDescriptorForKind(operation.Kind)
		if err != nil || !known || policy.ValidationFailureRepairConvergence == nil {
			continue
		}
		route, found := policy.ValidationFailureRepairConvergence.RouteForTrigger(descriptor.OwnerModuleID, operation.Kind, diagnostic.Class, diagnostic.RecommendedEvidenceTargetType, diagnostic.OwnerCode)
		if !found || !route.RequiresStructuredObservation() || route.RepairOwnerModuleID != c.RepairOwnerModuleID || route.RepairTargetPolicy != RuntimeOperationTargetPolicyExistingOwnerScope && route.RepairTargetPolicy != RuntimeOperationTargetPolicyExactTarget {
			continue
		}
		confirmationMatches := 0
		for _, record := range sources.Confirmations {
			if record.IdentitySignature != c.ConfirmationIdentityHash || record.Signature != c.ConfirmationHash {
				continue
			}
			confirmation, observation := record.Confirmation, record.Confirmation.Observation
			if confirmation.Disposition != RuntimeRepairSurfaceConfirmed || confirmation.OwnerModuleID != c.RepairOwnerModuleID || !slices.Equal(confirmation.ConfirmedTargetRefs, c.Closure.GroundedTargetRefs) || observation.WorkspaceID != sources.Facts.WorkspaceID || observation.RunID != sources.Facts.RunID || observation.DispatchID != rejected.DispatchID || observation.OperationID != rejected.OperationID || observation.AuthoritySnapshotID != rejected.AuthoritySnapshotID || observation.FailureSignature != rejected.FailureSignature || observation.DiagnosticClass != diagnostic.Class || !slices.Equal(observation.CandidateTargetRefs, diagnostic.TargetRefs) || observation.DiagnosticHash != StableRuntimeValidationFailureObservationDiagnosticHash(diagnostic, observation.ValidationScopeTargetRefs) {
				return false
			}
			confirmationMatches++
		}
		if confirmationMatches != 1 {
			continue
		}
		operations := runtimeCandidateAdmissionOperations(repair.Candidate)
		if len(operations) == 0 || len(repair.CompletedOperationIDs) != len(operations) {
			continue
		}
		last, complete := rejected.Revision, true
		for _, member := range operations {
			owner, known := RuntimeOperationDescriptorForKind(member.Kind)
			fact, receipt, valid := runtimeLineageOperationFactFromLedgers(repair, member, sources, true, true, false)
			if !known || owner.OwnerModuleID != c.RepairOwnerModuleID || !slices.Contains(route.RepairOperationKinds, member.Kind) || !valid || !receipt.Changed || !RuntimeEffectIsDurable(receipt.ObservedEffectScope) || !slices.Equal(member.BindingRefs, []string{binding.ID}) || len(receipt.TargetRefs) == 0 || !runtimeTargetRefsSubset(receipt.TargetRefs, c.Closure.GroundedTargetRefs) || !EvidenceRevisionAfter(fact.Revision, rejected.Revision) || EvidenceRevisionAfter(fact.Revision, cutoff) {
				complete = false
				break
			}
			if EvidenceRevisionAfter(fact.Revision, last) {
				last = fact.Revision
			}
		}
		if complete && runtimeConfirmedBindingFreshValidation(binding, sources, c, last, cutoff) {
			return true
		}
	}
	return false
}

func referenceLineagePreimageruntimeExecutionProofSourceBinding(assertion RuntimeLineageAssertion, sources RuntimeLineageAssertionSources) (CoverageEvidenceBinding, error) {
	invalid := func() (CoverageEvidenceBinding, error) {
		return CoverageEvidenceBinding{}, fmt.Errorf("execution proof lacks unique confirmed original source binding")
	}
	inventory := sources.Definition.SourceObligations
	if (assertion.Rule.ExecutionSourceContractRef != RuntimeDoDSourceExecutionProof && assertion.Rule.ExecutionSourceContractRef != RuntimeGoalSourceExecutionProof) || inventory == nil || inventory.Validate() != nil ||
		inventory.StartupInputHash != sources.Definition.StartupInputHash || inventory.DefinitionRevision != sources.Definition.Revision ||
		!SameRuntimeSourceObligationInventory(inventory, sources.Facts.DefinitionSeed.SourceObligations) ||
		ValidateRuntimeExecutionProofMappings(*inventory, sources.Facts.Activation.ExecutionContract.ProfileContracts) != nil {
		return invalid()
	}
	consumerRef, consumers := "", 0
	for _, source := range inventory.Sources {
		text := strings.TrimSpace(source.Text)
		if assertion.Rule.ExecutionSourceContractRef == RuntimeGoalSourceExecutionProof {
			text = source.Text
		}
		if source.SourceScope == assertion.SourceScope && source.SourceID == assertion.SourceID && RuntimeLineageSourceTextHash(text) == assertion.SourceTextHash {
			consumerRef, consumers = source.CandidateItemRef, consumers+1
		}
	}
	if consumers != 1 {
		return invalid()
	}
	sourceRef, mappings := "", 0
	for _, mapping := range inventory.ExecutionProofMappings {
		if mapping.ConsumerItemRef == consumerRef && mapping.RuleRef == assertion.Rule.Ref && mapping.ContractRef == assertion.Rule.ExecutionSourceContractRef {
			sourceRef, mappings = mapping.SourceItemRef, mappings+1
		}
	}
	if mappings != 1 {
		return invalid()
	}
	var original RuntimeDefinitionSource
	originals := 0
	for _, source := range inventory.Sources {
		if source.CandidateItemRef == sourceRef {
			original, originals = source, originals+1
		}
	}
	if originals != 1 || original.SourceScope != "dodItem" {
		return invalid()
	}
	var item RuntimeCoverageItem
	items := 0
	for _, candidate := range sources.Facts.DefinitionSeed.CoverageItems {
		if candidate.SourceScope == original.SourceScope && candidate.SourceID == original.SourceID && candidate.TargetText == strings.TrimSpace(original.Text) {
			item, items = candidate, items+1
		}
	}
	goalConsumer := assertion.Rule.ExecutionSourceContractRef == RuntimeGoalSourceExecutionProof
	if items != 1 || (!goalConsumer && item.ID == assertion.CoverageWorkItemID) || (goalConsumer && item.ID != assertion.CoverageWorkItemID) {
		return invalid()
	}
	var current RuntimeCoverageItemFact
	currentItems := 0
	for _, candidate := range sources.Facts.Coverage {
		if candidate.ID == item.ID && candidate.SourceScope == item.SourceScope && candidate.SourceID == item.SourceID && candidate.TargetText == item.TargetText {
			current, currentItems = candidate, currentItems+1
		}
	}
	if currentItems != 1 || current.CurrentBindingRef == nil || current.CurrentBindingRef.Validate() != nil {
		return invalid()
	}
	entries, err := runtimeAssessmentPurposePreimagesFromSemanticLedger(sources.Semantic, false)
	if err != nil {
		return invalid()
	}
	var binding CoverageEvidenceBinding
	matches := 0
	for _, entry := range entries {
		if entry.semanticManifestHash != current.CurrentBindingRef.SemanticManifestHash {
			continue
		}
		purpose, err := ResolveRuntimeSemanticEvidenceObligation(entry.obligation)
		if err != nil || purpose.Purpose != RuntimeSemanticEvidenceObligationDefinitionProof || entry.obligation.CoverageWorkItemID != item.ID {
			continue
		}
		for _, candidate := range entry.bindings {
			if runtimeLineageBindingMatches(candidate, item.ID, *current.CurrentBindingRef) {
				binding, matches = candidate, matches+1
			}
		}
	}
	if matches != 1 || binding.Validate() != nil || binding.DefinitionRevision != sources.Definition.Revision || binding.DeclarationRef != CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID) {
		return invalid()
	}
	if goalConsumer {
		if ValidateRuntimeGoalExecutionAssertionSeed(sources.Facts.DefinitionSeed, sources.Facts.Activation.ExecutionContract.ProfileContracts) != nil || !slices.Contains(binding.LineageAssertions, assertion) {
			return invalid()
		}
		ordinary := false
		for _, proof := range binding.Proofs {
			ordinary = ordinary || proof.EvidenceKind == EvidenceKindValidation
		}
		if !ordinary {
			return invalid()
		}
	}
	return binding, nil
}

func referenceLineagePreimageDeriveRuntimeAssessmentAdjudication(
	dispatch RuntimeDispatchRecord,
	facts RuntimeFacts,
	completed []RuntimeDispatchRecord,
	lineage []RuntimeCoverageProofLineage,
	lineageSources RuntimeLineageAssertionSources,
) (RuntimeAssessmentAdjudication, error) {
	if dispatch.Candidate.Assessment == nil || dispatch.AssessmentAuthorityInput == nil {
		return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment adjudication requires assessment dispatch authority")
	}
	if err := dispatch.AssessmentAuthorityInput.Validate(dispatch.Transaction, dispatch.CoverageBindings); err != nil {
		return RuntimeAssessmentAdjudication{}, err
	}
	candidate := dispatch.Candidate.Assessment.Normalized()
	authority := dispatch.AssessmentAuthorityInput.Normalized()
	ownerFactSources, err := deriveRuntimeAssessmentOwnerFactSources(facts.DefinitionRevision, string(facts.WorkspaceID), *dispatch.AssessmentAuthorityInput)
	if err != nil {
		return RuntimeAssessmentAdjudication{}, err
	}
	result := RuntimeAssessmentAdjudication{
		DispatchID: dispatch.ID, AuthorityInputID: dispatch.Transaction.AuthorityInputID,
		FocusID: dispatch.Transaction.FocusID, CoverageWorkItemIDs: candidate.CoverageWorkItemIDs,
		Disposition: AssessmentDispositionSatisfied, OwnerFactSources: ownerFactSources,
	}
	// The assessment dispatch is sealed after the operation receipt and any
	// owner-fact re-observation. Its catalog snapshot is therefore the immutable
	// answer to whether the required postcondition was present at assessment
	// admission. Acquisition purpose does not waive that postcondition: doing so
	// would let the same accepted operation be classified satisfied while the
	// sealed owner snapshot still proves the required fact absent.
	if len(RuntimeMissingOwnerFactRequirements(authority.Focus.RequiredOwnerFacts, authority.OperationAdmissibilities)) != 0 {
		result.Disposition = AssessmentDispositionRepairableGap
	}
	for _, coverageID := range candidate.CoverageWorkItemIDs {
		binding, found := assessmentBindingForCoverage(authority.Focus.CoverageEvidenceBindings, coverageID)
		if !found {
			result.Disposition = AssessmentDispositionRepairableGap
			continue
		}
		coverageProofs, complete := deriveAssessmentCoverageProofs(dispatch, authority.Purpose, authority.Focus, binding, facts.Evidence.Accepted, completed, lineage)
		if authority.Purpose == RuntimeAssessmentPurposeDefinitionProof {
			if !reflect.DeepEqual(lineageSources.Facts, facts) {
				return RuntimeAssessmentAdjudication{}, fmt.Errorf("lineage assertion facts differ from assessment source")
			}
			if !referenceLineagePreimageRuntimeExecutionLineageAssertionsSatisfied(binding, lineageSources, dispatch.Transaction.EvidenceRevision) {
				complete = false
			}
		}
		result.Proofs = append(result.Proofs, coverageProofs...)
		if !complete {
			result.Disposition = AssessmentDispositionRepairableGap
		}
	}
	result = result.Normalized()
	for _, proof := range result.Proofs {
		if err := ValidateRuntimeEvidenceDependencyPair(proof.OwnerRevisionSet, proof.ApplicabilityDependencies, facts.DefinitionRevision); err != nil {
			return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment selected proof dependencies: %w", err)
		}
		if proof.OwnerRevisionSet != nil && proof.OwnerRevisionSet.DefinitionRevision != facts.DefinitionRevision {
			return RuntimeAssessmentAdjudication{}, fmt.Errorf("runtime assessment selected proof dependencies definition mismatch")
		}
	}
	return result, nil
}

func referenceLineagePreimageValidateRuntimeAssessmentAdjudication(
	dispatch RuntimeDispatchRecord,
	fact RuntimeAssessmentFact,
	facts RuntimeFacts,
	completed []RuntimeDispatchRecord,
	lineage []RuntimeCoverageProofLineage,
	lineageSources RuntimeLineageAssertionSources,
) error {
	if dispatch.AssessmentAdjudication == nil || dispatch.AssessmentAdjudicationHash == "" || fact.AdjudicationHash == "" {
		return fmt.Errorf("committed runtime assessment dispatch %q requires double-entry adjudication", dispatch.ID)
	}
	actual := dispatch.AssessmentAdjudication.Normalized()
	if err := dispatch.AssessmentAdjudication.ValidateRaw(); err != nil {
		return err
	}
	if dispatch.AssessmentAdjudicationHash != StableRuntimeAssessmentAdjudicationHash(actual) || fact.AdjudicationHash != dispatch.AssessmentAdjudicationHash {
		return fmt.Errorf("runtime assessment dispatch %q adjudication hash mismatch", dispatch.ID)
	}
	authority := dispatch.AssessmentAuthorityInput.Normalized()
	source, err := RuntimeAssessmentSourceProjection(facts, fact.FocusID, fact.CoverageWorkItemIDs)
	if err != nil {
		return err
	}
	if authority.SourceSliceID != source.SliceID || authority.SourceScope != source.AssessmentScope ||
		authority.SourceID != source.AssessmentID || authority.SourceText != source.AssessmentText ||
		!sameCoverageWorkItemIDs(authority.Focus.CoverageWorkItemIDs, fact.CoverageWorkItemIDs) {
		return fmt.Errorf("runtime assessment dispatch %q authority source does not match definition seed", dispatch.ID)
	}
	expected, err := referenceLineagePreimageDeriveRuntimeAssessmentAdjudication(dispatch, facts, completed, lineage, lineageSources)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) || fact.Disposition != expected.Disposition {
		return fmt.Errorf("runtime assessment dispatch %q adjudication does not match immutable proof history", dispatch.ID)
	}
	return nil
}

// ValidateRuntimeAssessmentAdjudications enforces the completed-dispatch side
// of the assessment double entry after the global evidence ledger has proved
// the one-to-one fact/dispatch chronology.

func referenceLineagePreimageValidateRuntimeAssessmentAdjudications(facts RuntimeFacts, completed []RuntimeDispatchRecord, lineage []RuntimeCoverageProofLineage, lineageSources RuntimeLineageAssertionSources) error {
	byID := make(map[string]RuntimeAssessmentFact, len(facts.Assessments))
	for _, fact := range facts.Assessments {
		byID[fact.ID] = fact
	}
	for _, dispatch := range completed {
		// Invalidated assessment dispatches are durable SM-3 history, but they
		// never crossed the SM-2 assessment commit boundary and therefore must
		// not have a fact or adjudication. Dispatch intrinsic validation and the
		// evidence-sequence invariant enforce that negative side of the double
		// entry; only committed dispatches enter the positive adjudication proof.
		if dispatch.Candidate.Assessment == nil || dispatch.State != RuntimeDispatchStateCommitted {
			continue
		}
		fact, found := byID[dispatch.ID]
		if !found {
			return fmt.Errorf("committed runtime assessment dispatch %q has no assessment fact", dispatch.ID)
		}
		if err := referenceLineagePreimageValidateRuntimeAssessmentAdjudication(dispatch, fact, facts, completed, lineage, lineageSources); err != nil {
			return err
		}
	}
	return nil
}

// ValidateRuntimeAssessmentAuthoritySources roots every persisted assessment
// dispatch (including the still-active pre-commit dispatch) in DefinitionSeed.
// The dispatch-local source copy therefore cannot be coordinated with a
// mutable DefinitionFocus projection to manufacture different assessment
// authority across restart.
