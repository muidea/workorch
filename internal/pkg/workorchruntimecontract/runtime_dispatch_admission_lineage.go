package workorchruntimecontract

import "fmt"

// RuntimeAcceptedCandidateAttemptMatchesDispatch proves that a sealed
// dispatch is the exact projection of one accepted admission. Target
// authorization and managed-validation recipe commitments are deliberately
// stored outside the dispatch: a dispatch may rehash its own immutable fields,
// but it cannot thereby rewrite the accepted document's independently
// persisted target scope or executable recipe body.
func RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt RuntimeCandidateAttemptRecord, dispatch RuntimeDispatchRecord) bool {
	if attempt.State != RuntimeCandidateAttemptStateAccepted ||
		ValidateRuntimeDispatchOperationTargetAuthorizationProjection(attempt.AdmissionTargetAuthorizations) != nil ||
		ValidateRuntimeDispatchOperationTargetAuthorizationProjection(dispatch.OperationTargetAuthorizations) != nil ||
		ValidateRuntimeDispatchOperationTargetAuthorizations(dispatch.Candidate, attempt.AdmissionTargetAuthorizations) != nil ||
		attempt.TargetAuthorizationHash != StableRuntimeDispatchOperationTargetAuthorizationsHash(attempt.AdmissionTargetAuthorizations) ||
		!RuntimeDispatchOperationTargetAuthorizationProjectionsEqual(attempt.AdmissionTargetAuthorizations, dispatch.OperationTargetAuthorizations) ||
		ValidateCoverageEvidenceBindingProjection(attempt.AdmissionCoverageBindings) != nil ||
		ValidateCoverageEvidenceBindingProjection(dispatch.CoverageBindings) != nil ||
		attempt.AdmissionCoverageBindingHash != StableRuntimeCoverageEvidenceBindingSnapshotHash(attempt.AdmissionCoverageBindings) ||
		dispatch.CoverageBindingHash != StableRuntimeCoverageEvidenceBindingSnapshotHash(dispatch.CoverageBindings) ||
		!RuntimeCoverageEvidenceBindingProjectionsEqual(attempt.AdmissionCoverageBindings, dispatch.CoverageBindings) ||
		!runtimeAcceptedCandidateAttemptValidationRecipeMatchesDispatch(attempt, dispatch) ||
		!runtimeAcceptedCandidateAttemptRepairCommitmentMatchesDispatch(attempt, dispatch) {
		return false
	}
	return attempt.Transaction == dispatch.Transaction && attempt.DispatchID == dispatch.ID &&
		attempt.CandidateHash == dispatch.CandidateHash && attempt.ManifestHash == dispatch.CandidateManifestHash &&
		attempt.DispatchAuthorityHash == dispatch.DispatchAuthorityHash &&
		dispatch.DispatchAuthorityHash == StableRuntimeDispatchAuthorityHash(dispatch)
}

func runtimeAcceptedCandidateAttemptRepairCommitmentMatchesDispatch(attempt RuntimeCandidateAttemptRecord, dispatch RuntimeDispatchRecord) bool {
	if ValidateRuntimeDispatchRepairCommitmentProjection(attempt.RepairCommitmentHash, attempt.AdmissionRepairCommitment) != nil ||
		ValidateRuntimeDispatchRepairCommitmentProjection(dispatch.RepairCommitmentHash, dispatch.RepairCommitment) != nil ||
		attempt.RepairCommitmentHash != dispatch.RepairCommitmentHash ||
		!SameRuntimeDispatchRepairCommitment(attempt.AdmissionRepairCommitment, dispatch.RepairCommitment) {
		return false
	}
	if attempt.AdmissionRepairCommitment == nil {
		return true
	}
	return attempt.AdmissionRepairCommitment.Normalized().OriginDispatchID == dispatch.ID
}

func runtimeAcceptedCandidateAttemptValidationRecipeMatchesDispatch(attempt RuntimeCandidateAttemptRecord, dispatch RuntimeDispatchRecord) bool {
	if ValidateRuntimeCandidateManagedValidationRecipeSnapshot(dispatch.Candidate, attempt.AdmissionValidationRecipes) != nil ||
		ValidateRuntimeCandidateManagedValidationRecipeSnapshot(dispatch.Candidate, dispatch.ValidationRecipes) != nil ||
		!RuntimeValidationRecipeProjectionsEqual(attempt.AdmissionValidationRecipes, dispatch.ValidationRecipes) {
		return false
	}
	if len(attempt.AdmissionValidationRecipes) == 0 {
		return attempt.ValidationRecipeHash == ""
	}
	return attempt.ValidationRecipeHash == StableRuntimeValidationRecipeSnapshotHash(attempt.AdmissionValidationRecipes)
}

// ValidateRuntimeDispatchAdmissionLineage proves that each provider-produced
// SM-3 dispatch entered through exactly one durable admission source. An
// assessment requires its accepted candidate attempt; executable operation
// dispatches additionally permit one granted governance continuation. A
// self-consistent dispatch record alone is not candidate or execution
// authority.
func ValidateRuntimeDispatchAdmissionLineage(
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	attempts []RuntimeCandidateAttemptRecord,
	bindings []RuntimeOperationGovernanceBinding,
) error {
	dispatches := make([]RuntimeDispatchRecord, 0, len(completed)+1)
	dispatches = append(dispatches, completed...)
	if active != nil {
		dispatches = append(dispatches, *active)
	}
	for _, dispatch := range dispatches {
		if dispatch.Candidate.Governance != nil {
			continue
		}
		acceptedMatches := 0
		for _, attempt := range attempts {
			if RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, dispatch) {
				acceptedMatches++
			}
		}
		resumeMatches := 0
		if dispatch.Candidate.StateChange != nil || dispatch.Candidate.Validation != nil {
			for _, rawBinding := range bindings {
				binding := rawBinding.Normalized()
				if binding.State == RuntimeOperationGovernanceBindingGranted && binding.Grant != nil &&
					binding.Grant.Normalized().ResumeDispatchID == dispatch.ID &&
					runtimeGovernanceResumeAdmissionInheritsAuthorityCommitments(binding, dispatch, dispatches) {
					resumeMatches++
				}
			}
		}
		if acceptedMatches+resumeMatches != 1 {
			return fmt.Errorf("runtime candidate dispatch %q requires exactly one accepted-attempt or operation governance-resume admission, got accepted=%d resume=%d", dispatch.ID, acceptedMatches, resumeMatches)
		}
	}
	return nil
}

// runtimeGovernanceResumeAdmissionInheritsAuthorityCommitments binds every
// resume dispatch to its immediate source admission. Sequential continuations
// prove the original accepted commitment transitively because each hop must
// retain the exact manifest, candidate, canonical target authorization and
// complete managed-validation recipe bodies.
func runtimeGovernanceResumeAdmissionInheritsAuthorityCommitments(binding RuntimeOperationGovernanceBinding, resume RuntimeDispatchRecord, dispatches []RuntimeDispatchRecord) bool {
	for _, source := range dispatches {
		if source.ID != binding.OperationDispatchID || source.State != RuntimeDispatchStateCommitted {
			continue
		}
		if ValidateRuntimeCandidateManagedValidationRecipeSnapshot(source.Candidate, source.ValidationRecipes) != nil ||
			ValidateRuntimeCandidateManagedValidationRecipeSnapshot(resume.Candidate, resume.ValidationRecipes) != nil {
			return false
		}
		return source.CandidateHash == resume.CandidateHash &&
			source.CandidateManifestHash == resume.CandidateManifestHash &&
			source.RepairCommitmentHash == resume.RepairCommitmentHash &&
			SameRuntimeDispatchRepairCommitment(source.RepairCommitment, resume.RepairCommitment) &&
			RuntimeCoverageEvidenceBindingProjectionsEqual(source.CoverageBindings, resume.CoverageBindings) &&
			source.CoverageBindingHash == StableRuntimeCoverageEvidenceBindingSnapshotHash(source.CoverageBindings) &&
			resume.CoverageBindingHash == StableRuntimeCoverageEvidenceBindingSnapshotHash(resume.CoverageBindings) &&
			source.CoverageBindingHash == resume.CoverageBindingHash &&
			RuntimeDispatchOperationTargetAuthorizationProjectionsEqual(source.OperationTargetAuthorizations, resume.OperationTargetAuthorizations) &&
			StableRuntimeDispatchOperationTargetAuthorizationsHash(source.OperationTargetAuthorizations) ==
				StableRuntimeDispatchOperationTargetAuthorizationsHash(resume.OperationTargetAuthorizations) &&
			RuntimeValidationRecipeProjectionsEqual(source.ValidationRecipes, resume.ValidationRecipes) &&
			StableRuntimeValidationRecipeSnapshotHash(source.ValidationRecipes) ==
				StableRuntimeValidationRecipeSnapshotHash(resume.ValidationRecipes)
	}
	return false
}
