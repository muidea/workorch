package workorchruntimecontract

import "fmt"

// ValidateRuntimeAcceptedCandidateManifestLineage re-derives every accepted
// dispatch projection from the complete operations manifest persisted on its
// earlier provider-request session.  Attempt and dispatch copies plus their
// self hashes are not sufficient authority because they can be rewritten
// together in one corrupted aggregate.
func ValidateRuntimeAcceptedCandidateManifestLineage(
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	attempts []RuntimeCandidateAttemptRecord,
	sessions []RuntimeCandidateRequestSession,
) error {
	dispatches := make([]RuntimeDispatchRecord, 0, len(completed)+1)
	dispatches = append(dispatches, completed...)
	if active != nil {
		dispatches = append(dispatches, *active)
	}
	for _, rawAttempt := range attempts {
		attempt := rawAttempt.Normalized()
		if attempt.State != RuntimeCandidateAttemptStateAccepted {
			continue
		}
		var dispatch *RuntimeDispatchRecord
		dispatchMatches := 0
		for index := range dispatches {
			if dispatches[index].ID != attempt.DispatchID {
				continue
			}
			value := dispatches[index]
			dispatch = &value
			dispatchMatches++
		}
		if dispatchMatches != 1 || dispatch == nil {
			return fmt.Errorf("accepted runtime candidate attempt has no unique dispatch for manifest reverse proof")
		}
		var session *RuntimeCandidateRequestSession
		sessionMatches := 0
		for index := range sessions {
			rawSession := sessions[index]
			if rawSession.CandidateManifest != nil {
				if err := ValidateRuntimeCandidateContractManifestRawCanonical(*rawSession.CandidateManifest); err != nil {
					return fmt.Errorf("accepted runtime candidate attempt pre-seal manifest is not raw-canonical: %w", err)
				}
			}
			candidate := rawSession.Normalized()
			if candidate.LogicalRequestID != attempt.LogicalRequestID {
				continue
			}
			value := candidate
			session = &value
			sessionMatches++
		}
		if sessionMatches != 1 || session == nil || session.DocumentKind != RuntimeProviderDocumentOperations ||
			session.CandidateManifest == nil || session.Transaction != attempt.Transaction ||
			session.StablePrefixHash != attempt.StablePrefixHash || session.ManifestHash != attempt.ManifestHash {
			return fmt.Errorf("accepted runtime candidate attempt has no exact pre-seal operations manifest")
		}
		manifest := session.CandidateManifest.Clone()
		if manifest.Hash != attempt.ManifestHash || manifest.Hash != dispatch.CandidateManifestHash ||
			manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) {
			return fmt.Errorf("accepted runtime candidate attempt manifest body/hash mismatch")
		}
		if err := ValidateRuntimeCandidateAgainstManifest(manifest, dispatch.Candidate); err != nil {
			return fmt.Errorf("accepted runtime candidate attempt candidate/manifest mismatch: %w", err)
		}
		if dispatch.Candidate.Assessment != nil {
			if dispatch.AssessmentAuthorityInput == nil ||
				dispatch.AssessmentAuthorityInput.Normalized().Purpose != manifest.AssessmentPurpose ||
				manifest.AssessmentPurposeManifestHash == "" {
				return fmt.Errorf("accepted runtime assessment dispatch purpose does not match its pre-seal manifest")
			}
		}
		if dispatch.CandidateHash != attempt.CandidateHash || dispatch.CandidateHash != StableRuntimeCandidateHash(dispatch.Candidate) {
			return fmt.Errorf("accepted runtime candidate attempt candidate hash mismatch")
		}
		if session.RepairCommitmentHash != attempt.RepairCommitmentHash ||
			session.RepairCommitmentHash != dispatch.RepairCommitmentHash ||
			!SameRuntimeDispatchRepairCommitment(session.RepairCommitment, attempt.AdmissionRepairCommitment) ||
			!SameRuntimeDispatchRepairCommitment(session.RepairCommitment, dispatch.RepairCommitment) {
			return fmt.Errorf("accepted runtime candidate attempt repair commitment does not match pre-seal request session")
		}
		repairRequired := manifest.State == ConvergenceStateNeedStateChange &&
			manifest.RepairPurpose == RuntimeDispatchRepairPurposeConfirmedFileValidation
		if repairRequired {
			if session.RepairCommitment == nil || attempt.AdmissionRepairCommitment == nil || dispatch.RepairCommitment == nil ||
				dispatch.RepairCommitment.Normalized().Closure.ID != manifest.RepairClosureID ||
				dispatch.RepairCommitment.Normalized().FocusID != attempt.Transaction.FocusID ||
				dispatch.RepairCommitment.Normalized().OriginDispatchID != dispatch.ID {
				return fmt.Errorf("accepted runtime candidate attempt %q omitted its manifest-required repair commitment (session=%t attempt=%t dispatch=%t)", attempt.DispatchID, session.RepairCommitment != nil, attempt.AdmissionRepairCommitment != nil, dispatch.RepairCommitment != nil)
			}
		} else if session.RepairCommitment != nil || attempt.AdmissionRepairCommitment != nil || dispatch.RepairCommitment != nil {
			return fmt.Errorf("accepted runtime candidate attempt carries a repair commitment without manifest purpose")
		}
		targets, err := RuntimeCandidateAdmissionTargetAuthorizationsFromManifest(manifest, dispatch.Candidate)
		if err != nil {
			return fmt.Errorf("accepted runtime candidate attempt target projection: %w", err)
		}
		bindings := CloneCoverageEvidenceBindings(manifest.Bindings)
		if err := ValidateCoverageEvidenceBindingProjection(bindings); err != nil {
			return fmt.Errorf("accepted runtime candidate attempt coverage projection: %w", err)
		}
		recipes, err := RuntimeCandidateManagedValidationRecipeSnapshot(manifest, dispatch.Candidate)
		if err != nil {
			return fmt.Errorf("accepted runtime candidate attempt validation recipe projection: %w", err)
		}
		if !RuntimeDispatchOperationTargetAuthorizationProjectionsEqual(targets, attempt.AdmissionTargetAuthorizations) ||
			!RuntimeDispatchOperationTargetAuthorizationProjectionsEqual(targets, dispatch.OperationTargetAuthorizations) ||
			attempt.TargetAuthorizationHash != StableRuntimeDispatchOperationTargetAuthorizationsHash(targets) ||
			!RuntimeCoverageEvidenceBindingProjectionsEqual(bindings, attempt.AdmissionCoverageBindings) ||
			!RuntimeCoverageEvidenceBindingProjectionsEqual(bindings, dispatch.CoverageBindings) ||
			attempt.AdmissionCoverageBindingHash != StableRuntimeCoverageEvidenceBindingSnapshotHash(bindings) ||
			dispatch.CoverageBindingHash != StableRuntimeCoverageEvidenceBindingSnapshotHash(bindings) ||
			!RuntimeValidationRecipeProjectionsEqual(recipes, attempt.AdmissionValidationRecipes) ||
			!RuntimeValidationRecipeProjectionsEqual(recipes, dispatch.ValidationRecipes) {
			return fmt.Errorf("accepted runtime candidate attempt projections do not match pre-seal manifest")
		}
		wantRecipeHash := ""
		if len(recipes) != 0 {
			wantRecipeHash = StableRuntimeValidationRecipeSnapshotHash(recipes)
		}
		if attempt.ValidationRecipeHash != wantRecipeHash {
			return fmt.Errorf("accepted runtime candidate attempt validation recipe hash does not match pre-seal manifest")
		}
	}
	return nil
}

// ValidateRuntimeAcceptedAssessmentPurposePreimages replays the immutable
// semantic provider/owner batch behind every accepted assessment manifest.
// Candidate session, attempt and dispatch are same-CAS projections; none may
// certify a coordinated Purpose rewrite without this older semantic preimage.
func ValidateRuntimeAcceptedAssessmentPurposePreimages(
	ledger RuntimeSemanticPlanningLedger,
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	attempts []RuntimeCandidateAttemptRecord,
	sessions []RuntimeCandidateRequestSession,
) error {
	dispatches := make([]RuntimeDispatchRecord, 0, len(completed)+1)
	dispatches = append(dispatches, completed...)
	if active != nil {
		dispatches = append(dispatches, *active)
	}
	for _, rawAttempt := range attempts {
		attempt := rawAttempt.Normalized()
		if attempt.State != RuntimeCandidateAttemptStateAccepted {
			continue
		}
		var dispatch *RuntimeDispatchRecord
		matches := 0
		for index := range dispatches {
			if dispatches[index].ID != attempt.DispatchID {
				continue
			}
			value := dispatches[index]
			dispatch = &value
			matches++
		}
		if matches != 1 || dispatch == nil {
			return fmt.Errorf("accepted runtime candidate attempt has no unique dispatch for assessment purpose proof")
		}
		if dispatch.Candidate.Assessment == nil {
			continue
		}
		var manifest *RuntimeCandidateContractManifest
		matches = 0
		for index := range sessions {
			if sessions[index].LogicalRequestID != attempt.LogicalRequestID {
				continue
			}
			if sessions[index].CandidateManifest != nil {
				value := sessions[index].CandidateManifest.Clone()
				manifest = &value
			}
			matches++
		}
		if matches != 1 || manifest == nil {
			return fmt.Errorf("accepted runtime assessment attempt has no unique pre-seal manifest")
		}
		preimage, err := ResolveRuntimeAssessmentPurposePreimage(ledger, manifest.Bindings)
		if err != nil {
			return fmt.Errorf("accepted runtime assessment attempt purpose preimage: %w", err)
		}
		if manifest.AssessmentPurpose != preimage.Purpose ||
			manifest.AssessmentPurposeManifestHash != preimage.SemanticManifestHash ||
			dispatch.AssessmentAuthorityInput == nil ||
			dispatch.AssessmentAuthorityInput.Normalized().Purpose != preimage.Purpose {
			return fmt.Errorf("accepted runtime assessment attempt purpose differs from immutable semantic preimage")
		}
	}
	return nil
}
