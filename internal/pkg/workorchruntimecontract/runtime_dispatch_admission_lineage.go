package workorchruntimecontract

import "fmt"

// RuntimeAcceptedCandidateAttemptMatchesDispatch proves that a sealed
// dispatch is the exact projection of one accepted admission. Target
// authorization and managed-validation recipe commitments are deliberately
// stored outside the dispatch: a dispatch may rehash its own immutable fields,
// but it cannot thereby rewrite the accepted document's independently
// persisted target scope or executable recipe body.
func RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt RuntimeCandidateAttemptRecord, dispatch RuntimeDispatchRecord) bool {
	// 先排除原 matcher 最终也会拒绝的非同一配对；精确身份不授信，后续完整投影门禁仍逐次执行。
	if attempt.Transaction != dispatch.Transaction || attempt.DispatchID != dispatch.ID ||
		attempt.CandidateHash != dispatch.CandidateHash || attempt.ManifestHash != dispatch.CandidateManifestHash ||
		attempt.DispatchAuthorityHash != dispatch.DispatchAuthorityHash {
		return false
	}
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
		!runtimeImplementationWorkCommitmentsEqual(attempt.AdmissionImplementationWorkCommitment, dispatch.ImplementationWorkCommitment) ||
		!runtimeAcceptedCandidateAttemptRepairCommitmentMatchesDispatch(attempt, dispatch) {
		return false
	}
	return dispatch.DispatchAuthorityHash == StableRuntimeDispatchAuthorityHash(dispatch)
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
	return validateRuntimeDispatchAdmissionLineageInSession(active, completed, attempts, bindings, nil)
}

func validateRuntimeDispatchAdmissionLineageInSession(
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	attempts []RuntimeCandidateAttemptRecord,
	bindings []RuntimeOperationGovernanceBinding,
	previous *RuntimeStateRecord,
) error {
	dispatches := make([]RuntimeDispatchRecord, 0, len(completed)+1)
	dispatches = append(dispatches, completed...)
	if active != nil {
		dispatches = append(dispatches, *active)
	}
	dispatchCounts := make(map[string]int, len(dispatches))
	acceptedAttemptsByDispatchID := make(map[string][]RuntimeCandidateAttemptRecord, len(attempts))
	for _, dispatch := range dispatches {
		dispatchCounts[dispatch.ID]++
	}
	for _, attempt := range attempts {
		if attempt.State == RuntimeCandidateAttemptStateAccepted {
			acceptedAttemptsByDispatchID[attempt.DispatchID] = append(acceptedAttemptsByDispatchID[attempt.DispatchID], attempt)
		}
	}
	var grantedBindingsByResumeDispatchID map[string][]RuntimeOperationGovernanceBinding
	if len(bindings) > 0 {
		grantedBindingsByResumeDispatchID = make(map[string][]RuntimeOperationGovernanceBinding, len(bindings))
		for _, rawBinding := range bindings {
			binding := rawBinding.Normalized()
			if binding.State == RuntimeOperationGovernanceBindingGranted && binding.Grant != nil {
				id := binding.Grant.Normalized().ResumeDispatchID
				grantedBindingsByResumeDispatchID[id] = append(grantedBindingsByResumeDispatchID[id], binding)
			}
		}
	}
	var prevAcceptedByDispatchID map[string][]int
	var prevCompletedByID map[string][]int
	var prevGrantedResumeDispatchIDs map[string]struct{}
	if previous != nil {
		prevAcceptedByDispatchID = make(map[string][]int, len(previous.CandidateAttempts))
		for i := range previous.CandidateAttempts {
			if previous.CandidateAttempts[i].State == RuntimeCandidateAttemptStateAccepted {
				id := previous.CandidateAttempts[i].DispatchID
				prevAcceptedByDispatchID[id] = append(prevAcceptedByDispatchID[id], i)
			}
		}
		prevCompletedByID = make(map[string][]int, len(previous.CompletedDispatches))
		for i := range previous.CompletedDispatches {
			id := previous.CompletedDispatches[i].ID
			prevCompletedByID[id] = append(prevCompletedByID[id], i)
		}
		if len(previous.OperationGovernanceBindings) > 0 {
			prevGrantedResumeDispatchIDs = make(map[string]struct{}, len(previous.OperationGovernanceBindings))
			for i := range previous.OperationGovernanceBindings {
				binding := previous.OperationGovernanceBindings[i].Normalized()
				if binding.State == RuntimeOperationGovernanceBindingGranted && binding.Grant != nil {
					prevGrantedResumeDispatchIDs[binding.Grant.Normalized().ResumeDispatchID] = struct{}{}
				}
			}
		}
	}
	for _, dispatch := range dispatches {
		if dispatch.Candidate.Governance != nil {
			continue
		}
		if previous != nil && dispatchCounts[dispatch.ID] == 1 &&
			len(grantedBindingsByResumeDispatchID[dispatch.ID]) == 0 &&
			len(prevAcceptedByDispatchID[dispatch.ID]) == len(acceptedAttemptsByDispatchID[dispatch.ID]) {
			if _, hadResume := prevGrantedResumeDispatchIDs[dispatch.ID]; !hadResume {
				matchedPrev := previous.ActiveDispatch != nil && sameRuntimeDispatchPreSealLineage(*previous.ActiveDispatch, dispatch)
				if !matchedPrev {
					for _, idx := range prevCompletedByID[dispatch.ID] {
						if sameRuntimeDispatchPreSealLineage(previous.CompletedDispatches[idx], dispatch) {
							matchedPrev = true
							break
						}
					}
				}
				if matchedPrev {
					cur := acceptedAttemptsByDispatchID[dispatch.ID]
					prevIdx := prevAcceptedByDispatchID[dispatch.ID]
					sameAttempts := true
					for i := range cur {
						if !runtimeRawEqual(cur[i], previous.CandidateAttempts[prevIdx[i]]) {
							sameAttempts = false
							break
						}
					}
					if sameAttempts && len(cur) == 1 {
						continue
					}
				}
			}
		}
		acceptedMatches := 0
		for _, attempt := range acceptedAttemptsByDispatchID[dispatch.ID] {
			if RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, dispatch) {
				acceptedMatches++
			}
		}
		resumeMatches := 0
		if dispatch.Candidate.StateChange != nil || dispatch.Candidate.Validation != nil {
			for _, binding := range grantedBindingsByResumeDispatchID[dispatch.ID] {
				if runtimeGovernanceResumeAdmissionInheritsAuthorityCommitments(binding, dispatch, dispatches) {
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

func validateAcceptedCandidateAttemptsMatchDispatches(
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	attempts []RuntimeCandidateAttemptRecord,
	previous *RuntimeStateRecord,
) error {
	var completedByID map[string][]int
	var prevAttemptsByKey map[struct {
		snapshot string
		ordinal  int
	}][]int
	var prevCompletedByID map[string][]int
	for _, attempt := range attempts {
		if attempt.State != RuntimeCandidateAttemptStateAccepted {
			continue
		}
		if completedByID == nil {
			completedByID = make(map[string][]int, len(completed))
			for idx := range completed {
				completedByID[completed[idx].ID] = append(completedByID[completed[idx].ID], idx)
			}
			if previous != nil {
				prevAttemptsByKey = make(map[struct {
					snapshot string
					ordinal  int
				}][]int, len(previous.CandidateAttempts))
				for idx := range previous.CandidateAttempts {
					old := &previous.CandidateAttempts[idx]
					if old.State == RuntimeCandidateAttemptStateAccepted {
						key := struct {
							snapshot string
							ordinal  int
						}{old.Transaction.AuthoritySnapshotID, old.Attempt}
						prevAttemptsByKey[key] = append(prevAttemptsByKey[key], idx)
					}
				}
				prevCompletedByID = make(map[string][]int, len(previous.CompletedDispatches))
				for idx := range previous.CompletedDispatches {
					prevCompletedByID[previous.CompletedDispatches[idx].ID] = append(prevCompletedByID[previous.CompletedDispatches[idx].ID], idx)
				}
			}
		}
		attemptCommitted := false
		if previous != nil {
			key := struct {
				snapshot string
				ordinal  int
			}{attempt.Transaction.AuthoritySnapshotID, attempt.Attempt}
			for _, idx := range prevAttemptsByKey[key] {
				if runtimeRawEqual(previous.CandidateAttempts[idx], attempt) {
					attemptCommitted = true
					break
				}
			}
		}
		dispatchCommitted := func(dispatch RuntimeDispatchRecord) bool {
			if !attemptCommitted {
				return false
			}
			if previous.ActiveDispatch != nil && sameRuntimeDispatchPreSealLineage(*previous.ActiveDispatch, dispatch) {
				return true
			}
			for _, idx := range prevCompletedByID[dispatch.ID] {
				if sameRuntimeDispatchPreSealLineage(previous.CompletedDispatches[idx], dispatch) {
					return true
				}
			}
			return false
		}
		matched := false
		if active != nil && active.ID == attempt.DispatchID {
			if dispatchCommitted(*active) || RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, *active) {
				matched = true
			}
		}
		if !matched {
			for _, idx := range completedByID[attempt.DispatchID] {
				dispatch := completed[idx]
				if dispatchCommitted(dispatch) || RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, dispatch) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("accepted runtime candidate attempt has no exact sealed dispatch target/validation-recipe authority projection")
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
			runtimeImplementationWorkCommitmentsEqual(source.ImplementationWorkCommitment, resume.ImplementationWorkCommitment) &&
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
