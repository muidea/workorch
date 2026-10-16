package workorchruntimecontract

import (
	"fmt"
	"reflect"
)

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
	return validateRuntimeAcceptedCandidateManifestLineageInSession(active, completed, attempts, sessions, nil)
}

func validateRuntimeAcceptedCandidateManifestLineageInSession(active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, attempts []RuntimeCandidateAttemptRecord, sessions []RuntimeCandidateRequestSession, semantic *runtimeSemanticValidationSession) error {
	dispatches := make([]RuntimeDispatchRecord, 0, len(completed)+1)
	dispatches = append(dispatches, completed...)
	if active != nil {
		dispatches = append(dispatches, *active)
	}
	acceptedAttempts := make([]RuntimeCandidateAttemptRecord, 0, len(attempts))
	for _, rawAttempt := range attempts {
		attempt := rawAttempt.Normalized()
		if attempt.State != RuntimeCandidateAttemptStateAccepted {
			continue
		}
		acceptedAttempts = append(acceptedAttempts, attempt)
	}
	if len(acceptedAttempts) == 0 {
		return nil
	}

	// Index immutable lineage once for the whole accepted-attempt ledger. The
	// former nested scans normalized and raw-validated every request session for
	// every accepted attempt, making restart validation quadratic in completed
	// dispatch history without adding an independent authority check.
	dispatchByID := make(map[string]RuntimeDispatchRecord, len(dispatches))
	dispatchCounts := make(map[string]int, len(dispatches))
	for _, dispatch := range dispatches {
		dispatchCounts[dispatch.ID]++
		dispatchByID[dispatch.ID] = dispatch
	}
	sessionByRequest := make(map[string]RuntimeCandidateRequestSession, len(sessions))
	sessionCounts := make(map[string]int, len(sessions))
	validatedByRequest := make(map[string]runtimeValidatedCandidateManifest, len(sessions))
	for _, rawSession := range sessions {
		var validated *runtimeValidatedCandidateManifest
		if rawSession.CandidateManifest != nil {
			value, err := validateRuntimeCandidateManifestRawCanonical(*rawSession.CandidateManifest)
			if err != nil {
				return fmt.Errorf("accepted runtime candidate attempt pre-seal manifest is not raw-canonical: %w", err)
			}
			validated = &value
		}
		// 完整 raw 校验已生成 detached manifest，仅规范化其余 session 字段。
		rawSession.CandidateManifest = nil
		session := rawSession.Normalized()
		if validated != nil {
			session.CandidateManifest = &validated.manifest
			validatedByRequest[session.LogicalRequestID] = *validated
		}
		sessionCounts[session.LogicalRequestID]++
		sessionByRequest[session.LogicalRequestID] = session
	}

	for _, attempt := range acceptedAttempts {
		dispatch, found := dispatchByID[attempt.DispatchID]
		if !found || dispatchCounts[attempt.DispatchID] != 1 {
			return fmt.Errorf("accepted runtime candidate attempt has no unique dispatch for manifest reverse proof")
		}
		session, found := sessionByRequest[attempt.LogicalRequestID]
		if !found || sessionCounts[attempt.LogicalRequestID] != 1 || session.DocumentKind != RuntimeProviderDocumentOperations ||
			session.CandidateManifest == nil || session.Transaction != attempt.Transaction ||
			session.StablePrefixHash != attempt.StablePrefixHash || session.ManifestHash != attempt.ManifestHash {
			return fmt.Errorf("accepted runtime candidate attempt has no exact pre-seal operations manifest")
		}
		validated := validatedByRequest[session.LogicalRequestID]
		manifest := validated.manifest
		if manifest.Hash != attempt.ManifestHash || manifest.Hash != dispatch.CandidateManifestHash {
			return fmt.Errorf("accepted runtime candidate attempt manifest body/hash mismatch")
		}
		if err := validateRuntimeCandidateAgainstValidatedManifest(validated, dispatch.Candidate); err != nil {
			return fmt.Errorf("accepted runtime candidate attempt candidate/manifest mismatch: %w", err)
		}
		if dispatch.Candidate.Assessment != nil {
			if dispatch.AssessmentAuthorityInput == nil ||
				dispatch.AssessmentAuthorityInput.Normalized().Purpose != manifest.AssessmentPurpose ||
				manifest.AssessmentPurposeManifestHash == "" {
				return fmt.Errorf("accepted runtime assessment dispatch purpose does not match its pre-seal manifest")
			}
		}
		if manifest.ImplementationWorkingBinding != nil {
			if attempt.AdmissionImplementationWorkCommitment == nil || dispatch.ImplementationWorkCommitment == nil ||
				!runtimeImplementationWorkCommitmentsEqual(attempt.AdmissionImplementationWorkCommitment, dispatch.ImplementationWorkCommitment) ||
				!reflect.DeepEqual(manifest.ImplementationWorkingBinding, dispatch.ImplementationWorkCommitment.WorkingBinding) {
				return fmt.Errorf("accepted implementation dispatch differs from original pre-seal working manifest")
			}
			if err := validateRuntimeImplementationWorkDispatchCommitmentInSession(dispatch, semantic); err != nil {
				return err
			}
		} else if attempt.AdmissionImplementationWorkCommitment != nil || dispatch.ImplementationWorkCommitment != nil {
			return fmt.Errorf("non-working manifest cannot carry implementation commitment")
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
			manifest.RepairPurpose == RuntimeDispatchRepairPurposeConfirmedValidation
		if repairRequired {
			if session.RepairCommitment == nil || attempt.AdmissionRepairCommitment == nil || dispatch.RepairCommitment == nil ||
				dispatch.RepairCommitment.Normalized().Closure.ID != manifest.RepairClosureID ||
				dispatch.RepairCommitment.Normalized().FocusID != attempt.Transaction.FocusID ||
				dispatch.RepairCommitment.Normalized().OriginDispatchID != dispatch.ID {
				return fmt.Errorf("accepted runtime candidate attempt %q omitted its manifest-required repair commitment (session=%t attempt=%t dispatch=%t)", attempt.DispatchID, session.RepairCommitment != nil, attempt.AdmissionRepairCommitment != nil, dispatch.RepairCommitment != nil)
			}
			for _, authorization := range manifest.Operations {
				if !SameRuntimeValidationInputRepair(authorization.InputRepair, dispatch.RepairCommitment.InputRepair) {
					return fmt.Errorf("accepted input repair manifest differs from sealed source commitment")
				}
			}
		} else if session.RepairCommitment != nil || attempt.AdmissionRepairCommitment != nil || dispatch.RepairCommitment != nil {
			return fmt.Errorf("accepted runtime candidate attempt carries a repair commitment without manifest purpose")
		}
		targets, err := runtimeOperationTargetAuthorizationsFromNormalizedManifest(manifest, dispatch.Candidate)
		if err != nil {
			return fmt.Errorf("accepted runtime candidate attempt target projection: %w", err)
		}
		// manifest 的完整 raw-canonical/source/hash 校验已反证 binding。
		// attempt/dispatch 两侧仍各自规范化并比较完整正文，不能只比较 hash。
		bindings := manifest.Bindings
		bindingHash := stableRuntimeCoverageEvidenceBindingSnapshotHashNormalized(bindings)
		recipes, err := runtimeCandidateManagedValidationRecipeSnapshotFromNormalizedManifest(manifest, dispatch.Candidate)
		if err != nil {
			return fmt.Errorf("accepted runtime candidate attempt validation recipe projection: %w", err)
		}
		if !RuntimeDispatchOperationTargetAuthorizationProjectionsEqual(targets, attempt.AdmissionTargetAuthorizations) ||
			!RuntimeDispatchOperationTargetAuthorizationProjectionsEqual(targets, dispatch.OperationTargetAuthorizations) ||
			attempt.TargetAuthorizationHash != StableRuntimeDispatchOperationTargetAuthorizationsHash(targets) ||
			!reflect.DeepEqual(bindings, attempt.AdmissionCoverageBindings) ||
			!reflect.DeepEqual(bindings, NormalizeCoverageEvidenceBindings(dispatch.CoverageBindings)) ||
			attempt.AdmissionCoverageBindingHash != bindingHash ||
			dispatch.CoverageBindingHash != bindingHash ||
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
	if err := ledger.Validate(); err != nil {
		return fmt.Errorf("accepted runtime assessment semantic ledger: %w", err)
	}
	return validateRuntimeAcceptedAssessmentPurposePreimagesWithValidatedSemanticLedger(ledger, active, completed, attempts, sessions)
}

// ValidateRuntimeAcceptedAssessmentPurposePreimagesWithValidatedSemanticLedger
// performs the assessment-lineage cross-check after the enclosing aggregate
// has already validated the exact semantic ledger. It avoids revalidating each
// immutable history snapshot while still recompiling the independent binding
// preimages used by accepted dispatches.
func ValidateRuntimeAcceptedAssessmentPurposePreimagesWithValidatedSemanticLedger(
	ledger RuntimeSemanticPlanningLedger,
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	attempts []RuntimeCandidateAttemptRecord,
	sessions []RuntimeCandidateRequestSession,
) error {
	return validateRuntimeAcceptedAssessmentPurposePreimagesWithValidatedSemanticLedger(ledger, active, completed, attempts, sessions)
}

func validateRuntimeAcceptedAssessmentPurposePreimagesWithValidatedSemanticLedger(
	ledger RuntimeSemanticPlanningLedger,
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	attempts []RuntimeCandidateAttemptRecord,
	sessions []RuntimeCandidateRequestSession,
) error {
	return validateRuntimeAcceptedAssessmentPurposePreimagesInSession(ledger, active, completed, attempts, sessions, newRuntimeSemanticValidationSession())
}

func validateRuntimeAcceptedAssessmentPurposePreimagesInSession(
	ledger RuntimeSemanticPlanningLedger,
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	attempts []RuntimeCandidateAttemptRecord,
	sessions []RuntimeCandidateRequestSession,
	semanticSession *runtimeSemanticValidationSession,
) error {
	dispatches := make([]RuntimeDispatchRecord, 0, len(completed)+1)
	dispatches = append(dispatches, completed...)
	if active != nil {
		dispatches = append(dispatches, *active)
	}
	dispatchByID := make(map[string]RuntimeDispatchRecord, len(dispatches))
	dispatchCounts := make(map[string]int, len(dispatches))
	for _, dispatch := range dispatches {
		dispatchCounts[dispatch.ID]++
		dispatchByID[dispatch.ID] = dispatch
	}
	sessionManifestByRequest := make(map[string]RuntimeCandidateContractManifest, len(sessions))
	sessionCounts := make(map[string]int, len(sessions))
	for _, session := range sessions {
		sessionCounts[session.LogicalRequestID]++
		if session.CandidateManifest != nil {
			sessionManifestByRequest[session.LogicalRequestID] = session.CandidateManifest.Clone()
		}
	}
	type acceptedAssessment struct {
		dispatch RuntimeDispatchRecord
		manifest RuntimeCandidateContractManifest
	}
	acceptedAssessments := make([]acceptedAssessment, 0)
	for _, rawAttempt := range attempts {
		attempt := rawAttempt.Normalized()
		if attempt.State != RuntimeCandidateAttemptStateAccepted {
			continue
		}
		dispatch, found := dispatchByID[attempt.DispatchID]
		if !found || dispatchCounts[attempt.DispatchID] != 1 {
			return fmt.Errorf("accepted runtime candidate attempt has no unique dispatch for assessment purpose proof")
		}
		if dispatch.Candidate.Assessment == nil {
			continue
		}
		manifest, found := sessionManifestByRequest[attempt.LogicalRequestID]
		if !found || sessionCounts[attempt.LogicalRequestID] != 1 {
			return fmt.Errorf("accepted runtime assessment attempt has no unique pre-seal manifest")
		}
		acceptedAssessments = append(acceptedAssessments, acceptedAssessment{dispatch: dispatch, manifest: manifest})
	}
	if len(acceptedAssessments) == 0 {
		return nil
	}
	// Build the semantic generation index once for the whole accepted-attempt
	// ledger. Replaying every immutable generation for every assessment makes
	// validation quadratic in retained audit history and can prevent an
	// otherwise healthy Runtime from reaching its next owner transition. This
	// is only a local validation acceleration: exact binding projection
	// equality and the unique-preimage requirement remain unchanged.
	preimages, err := runtimeAssessmentPurposePreimagesFromSemanticLedgerInSession(ledger, true, semanticSession)
	if err != nil {
		return fmt.Errorf("accepted runtime assessment attempt purpose preimage: %w", err)
	}
	for _, accepted := range acceptedAssessments {
		preimage, err := resolveRuntimeAssessmentPurposePreimageInSession(preimages, accepted.manifest.Bindings, semanticSession)
		if err != nil {
			return fmt.Errorf("accepted runtime assessment attempt purpose preimage: %w", err)
		}
		manifest := accepted.manifest
		dispatch := accepted.dispatch
		if manifest.AssessmentPurpose != preimage.Purpose ||
			manifest.AssessmentPurposeManifestHash != preimage.SemanticManifestHash ||
			dispatch.AssessmentAuthorityInput == nil ||
			dispatch.AssessmentAuthorityInput.Normalized().Purpose != preimage.Purpose {
			return fmt.Errorf("accepted runtime assessment attempt purpose differs from immutable semantic preimage")
		}
	}
	return nil
}
