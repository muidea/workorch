package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
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
	if semantic != nil && semantic.committedPreimage != nil {
		attempts, sessions = runtimeCandidateSourcesRequiringAdmissionInSession(semantic, active, completed, attempts, sessions)
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
	dispatches := make([]RuntimeDispatchRecord, 0, len(completed)+1)
	dispatches = append(dispatches, completed...)
	if active != nil {
		dispatches = append(dispatches, *active)
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

func hasAcceptedSemanticPlanBatch(batches []RuntimeSemanticPlanBatch) bool {
	for i := range batches {
		if batches[i].Normalized().State == RuntimeSemanticPlanBatchAccepted {
			return true
		}
	}
	return false
}

func semanticBatchIsImplementationWork(manifest *RuntimeSemanticPlanManifest, batch RuntimeSemanticPlanBatch) bool {
	if manifest == nil || len(batch.CoverageWorkItemIDs) == 0 {
		return false
	}
	for _, rawID := range batch.CoverageWorkItemIDs {
		id := common.CoverageWorkItemID(strings.TrimSpace(string(rawID)))
		for i := range manifest.ProofObligations {
			if common.CoverageWorkItemID(strings.TrimSpace(string(manifest.ProofObligations[i].CoverageWorkItemID))) == id &&
				manifest.ProofObligations[i].ImplementationWork != nil {
				return true
			}
		}
	}
	return false
}

func hasAcceptedAssessmentPurposeBatch(manifest *RuntimeSemanticPlanManifest, batches []RuntimeSemanticPlanBatch) bool {
	for i := range batches {
		batch := batches[i].Normalized()
		if batch.State != RuntimeSemanticPlanBatchAccepted {
			continue
		}
		if !semanticBatchIsImplementationWork(manifest, batch) {
			return true
		}
	}
	return false
}

type runtimeAcceptedAssessmentPreimageDelta struct {
	preserved           bool
	newBatchCoverage    map[common.CoverageWorkItemID]struct{}
	hasUnscopedNewBatch bool
}

func (d *runtimeAcceptedAssessmentPreimageDelta) recordBatchCoverage(batch RuntimeSemanticPlanBatch) {
	if len(batch.CoverageWorkItemIDs) == 0 {
		d.hasUnscopedNewBatch = true
		return
	}
	if d.newBatchCoverage == nil {
		d.newBatchCoverage = make(map[common.CoverageWorkItemID]struct{}, len(batch.CoverageWorkItemIDs))
	}
	for _, rawID := range batch.CoverageWorkItemIDs {
		id := common.CoverageWorkItemID(strings.TrimSpace(string(rawID)))
		if id == "" {
			d.hasUnscopedNewBatch = true
			continue
		}
		d.newBatchCoverage[id] = struct{}{}
	}
}

func semanticGenerationPreservesPriorAcceptedAssessmentPreimages(before, after RuntimeSemanticPlanningLedger, delta *runtimeAcceptedAssessmentPreimageDelta) bool {
	if !hasAcceptedAssessmentPurposeBatch(before.Manifest, before.Batches) {
		if after.Manifest != nil {
			for i := range after.Batches {
				batch := after.Batches[i].Normalized()
				if batch.State == RuntimeSemanticPlanBatchAccepted && !semanticBatchIsImplementationWork(after.Manifest, batch) {
					delta.recordBatchCoverage(batch)
				}
			}
		} else if hasAcceptedSemanticPlanBatch(after.Batches) {
			return false
		}
		return true
	}
	if before.Manifest == nil || after.Manifest == nil || !runtimeRawEqual(before.Manifest, after.Manifest) ||
		len(after.Batches) < len(before.Batches) ||
		len(after.BatchDocuments) < len(before.BatchDocuments) ||
		len(after.BatchFeasibilityResults) < len(before.BatchFeasibilityResults) {
		return false
	}
	if !runtimeRawEqual(before.BatchDocuments, after.BatchDocuments[:len(before.BatchDocuments)]) ||
		!runtimeRawEqual(before.BatchFeasibilityResults, after.BatchFeasibilityResults[:len(before.BatchFeasibilityResults)]) {
		return false
	}
	results := make(map[string]struct{}, len(before.BatchFeasibilityResults))
	for i := range before.BatchFeasibilityResults {
		results[strings.TrimSpace(before.BatchFeasibilityResults[i].BatchID)] = struct{}{}
	}
	for i := range before.Batches {
		prevBatch := before.Batches[i].Normalized()
		if prevBatch.State == RuntimeSemanticPlanBatchAccepted {
			if !runtimeRawEqual(before.Batches[i], after.Batches[i]) {
				return false
			}
			if !semanticBatchIsImplementationWork(before.Manifest, prevBatch) {
				if _, ok := results[prevBatch.BatchID]; !ok {
					if !runtimeRawEqual(before.Document, after.Document) ||
						before.DocumentHash != after.DocumentHash ||
						before.FeasibilitySetHash != after.FeasibilitySetHash ||
						!runtimeRawEqual(before.Feasibilities, after.Feasibilities) {
						return false
					}
				}
			}
			continue
		}
		currBatch := after.Batches[i].Normalized()
		if currBatch.State == RuntimeSemanticPlanBatchAccepted && !semanticBatchIsImplementationWork(after.Manifest, currBatch) {
			delta.recordBatchCoverage(currBatch)
		}
	}
	for i := len(before.Batches); i < len(after.Batches); i++ {
		currBatch := after.Batches[i].Normalized()
		if currBatch.State == RuntimeSemanticPlanBatchAccepted && !semanticBatchIsImplementationWork(after.Manifest, currBatch) {
			delta.recordBatchCoverage(currBatch)
		}
	}
	return true
}

func semanticPlanningPreservesPriorAcceptedAssessmentPreimages(before, after RuntimeSemanticPlanningLedger) runtimeAcceptedAssessmentPreimageDelta {
	var delta runtimeAcceptedAssessmentPreimageDelta
	beforeCount := before.HistoryCount()
	afterCount := after.HistoryCount()
	if afterCount < beforeCount {
		return delta
	}
	if before.HistoryArchive == nil && after.HistoryArchive == nil {
		if !runtimeRawEqual(before.History, after.History[:len(before.History)]) {
			return delta
		}
	} else if before.HistoryArchive != nil && after.HistoryArchive != nil &&
		len(before.HistoryArchive.Segments) == len(after.HistoryArchive.Segments) &&
		len(after.History) >= len(before.History) {
		if !runtimeRawEqual(before.HistoryArchive, after.HistoryArchive) ||
			!runtimeRawEqual(before.History, after.History[:len(before.History)]) {
			return delta
		}
	} else {
		beforeAll := make([]RuntimeSemanticPlanningGeneration, 0, beforeCount)
		for _, gen := range before.AllHistory() {
			beforeAll = append(beforeAll, gen)
		}
		idx := 0
		for _, gen := range after.AllHistory() {
			if idx >= len(beforeAll) {
				break
			}
			if !runtimeRawEqual(beforeAll[idx], gen) {
				return delta
			}
			idx++
		}
		if idx != len(beforeAll) {
			return delta
		}
	}
	if afterCount == beforeCount {
		if runtimeRawEqual(before.WithoutHistory(), after.WithoutHistory()) {
			delta.preserved = true
			return delta
		}
		delta.preserved = semanticGenerationPreservesPriorAcceptedAssessmentPreimages(before, after, &delta)
		return delta
	}
	hasBeforeAcceptedBatch := hasAcceptedAssessmentPurposeBatch(before.Manifest, before.Batches)
	idx := 0
	for _, gen := range after.AllHistory() {
		if idx < beforeCount {
			idx++
			continue
		}
		if idx == beforeCount && hasBeforeAcceptedBatch {
			if !semanticGenerationPreservesPriorAcceptedAssessmentPreimages(before, gen.Snapshot, &delta) {
				return delta
			}
		} else if gen.Snapshot.Manifest != nil {
			for i := range gen.Snapshot.Batches {
				batch := gen.Snapshot.Batches[i].Normalized()
				if batch.State == RuntimeSemanticPlanBatchAccepted && !semanticBatchIsImplementationWork(gen.Snapshot.Manifest, batch) {
					delta.recordBatchCoverage(batch)
				}
			}
		} else if hasAcceptedSemanticPlanBatch(gen.Snapshot.Batches) {
			return delta
		}
		idx++
	}
	if after.Manifest != nil {
		for i := range after.Batches {
			batch := after.Batches[i].Normalized()
			if batch.State == RuntimeSemanticPlanBatchAccepted && !semanticBatchIsImplementationWork(after.Manifest, batch) {
				delta.recordBatchCoverage(batch)
			}
		}
	} else if hasAcceptedSemanticPlanBatch(after.Batches) {
		return delta
	}
	delta.preserved = true
	return delta
}

func semanticGenerationPreservesAcceptedBatchPreimages(before, after RuntimeSemanticPlanningLedger) bool {
	beforeCurrent := before.WithoutHistory()
	afterCurrent := after.WithoutHistory()
	if runtimeRawEqual(beforeCurrent, afterCurrent) {
		return true
	}
	if before.Manifest == nil || !hasAcceptedSemanticPlanBatch(before.Batches) {
		return after.Manifest == nil || !hasAcceptedSemanticPlanBatch(after.Batches)
	}
	if !runtimeRawEqual(before.Manifest, after.Manifest) ||
		len(after.Batches) < len(before.Batches) ||
		len(after.BatchDocuments) < len(before.BatchDocuments) ||
		len(after.BatchFeasibilityResults) < len(before.BatchFeasibilityResults) {
		return false
	}
	if !runtimeRawEqual(before.Batches, after.Batches[:len(before.Batches)]) ||
		!runtimeRawEqual(before.BatchDocuments, after.BatchDocuments[:len(before.BatchDocuments)]) ||
		!runtimeRawEqual(before.BatchFeasibilityResults, after.BatchFeasibilityResults[:len(before.BatchFeasibilityResults)]) {
		return false
	}
	if hasAcceptedSemanticPlanBatch(after.Batches[len(before.Batches):]) {
		return false
	}
	results := make(map[string]struct{}, len(before.BatchFeasibilityResults))
	for i := range before.BatchFeasibilityResults {
		results[strings.TrimSpace(before.BatchFeasibilityResults[i].BatchID)] = struct{}{}
	}
	for i := range before.Batches {
		batch := before.Batches[i].Normalized()
		if batch.State != RuntimeSemanticPlanBatchAccepted {
			continue
		}
		if _, ok := results[batch.BatchID]; !ok {
			if !runtimeRawEqual(before.Document, after.Document) ||
				before.DocumentHash != after.DocumentHash ||
				before.FeasibilitySetHash != after.FeasibilitySetHash ||
				!runtimeRawEqual(before.Feasibilities, after.Feasibilities) {
				return false
			}
		}
	}
	return true
}

func SemanticPlanningPreservesCommittedPreimages(before, after RuntimeSemanticPlanningLedger) bool {
	beforeCount := before.HistoryCount()
	afterCount := after.HistoryCount()
	if afterCount < beforeCount {
		return false
	}
	if before.HistoryArchive == nil && after.HistoryArchive == nil {
		if !runtimeRawEqual(before.History, after.History[:len(before.History)]) {
			return false
		}
	} else if before.HistoryArchive != nil && after.HistoryArchive != nil &&
		len(before.HistoryArchive.Segments) == len(after.HistoryArchive.Segments) &&
		len(after.History) >= len(before.History) {
		if !runtimeRawEqual(before.HistoryArchive, after.HistoryArchive) ||
			!runtimeRawEqual(before.History, after.History[:len(before.History)]) {
			return false
		}
	} else {
		beforeAll := make([]RuntimeSemanticPlanningGeneration, 0, beforeCount)
		for _, gen := range before.AllHistory() {
			beforeAll = append(beforeAll, gen)
		}
		idx := 0
		for _, gen := range after.AllHistory() {
			if idx >= len(beforeAll) {
				break
			}
			if !runtimeRawEqual(beforeAll[idx], gen) {
				return false
			}
			idx++
		}
		if idx != len(beforeAll) {
			return false
		}
	}
	if afterCount == beforeCount {
		return semanticGenerationPreservesAcceptedBatchPreimages(before, after)
	}
	hasBeforeAcceptedBatch := before.Manifest != nil && hasAcceptedSemanticPlanBatch(before.Batches)
	idx := 0
	for _, gen := range after.AllHistory() {
		if idx < beforeCount {
			idx++
			continue
		}
		if idx == beforeCount && hasBeforeAcceptedBatch {
			if !semanticGenerationPreservesAcceptedBatchPreimages(before, gen.Snapshot) {
				return false
			}
		} else if gen.Snapshot.Manifest != nil && hasAcceptedSemanticPlanBatch(gen.Snapshot.Batches) {
			return false
		}
		idx++
	}
	return after.Manifest == nil || !hasAcceptedSemanticPlanBatch(after.Batches)
}

func validateRuntimeAcceptedAssessmentPurposePreimagesInSession(
	ledger RuntimeSemanticPlanningLedger,
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	attempts []RuntimeCandidateAttemptRecord,
	sessions []RuntimeCandidateRequestSession,
	semanticSession *runtimeSemanticValidationSession,
) error {
	hasAcceptedAttempt := false
	for i := range attempts {
		if strings.TrimSpace(string(attempts[i].State)) == string(RuntimeCandidateAttemptStateAccepted) {
			hasAcceptedAttempt = true
			break
		}
	}
	if !hasAcceptedAttempt {
		return nil
	}
	validateActiveAssessmentPreimages := false
	if semanticSession != nil && semanticSession.committedPreimage != nil {
		if runtimeRawEqual(ledger, semanticSession.committedPreimage.SemanticPlanning) {
			attempts, sessions = runtimeCandidateSourcesRequiringAdmissionInSession(semanticSession, active, completed, attempts, sessions)
		} else {
			delta := semanticPlanningPreservesPriorAcceptedAssessmentPreimages(semanticSession.committedPreimage.SemanticPlanning, ledger)
			if delta.preserved && !delta.hasUnscopedNewBatch {
				filteredAttempts, filteredSessions := runtimeCandidateSourcesRequiringAdmissionInSession(semanticSession, active, completed, attempts, sessions)
				if len(delta.newBatchCoverage) == 0 {
					attempts, sessions = filteredAttempts, filteredSessions
				} else {
					validateActiveAssessmentPreimages = true
					neededSessionMap := make(map[string]RuntimeCandidateRequestSession, len(sessions))
					for i := range sessions {
						neededSessionMap[strings.TrimSpace(sessions[i].LogicalRequestID)] = sessions[i]
					}
					includedAttempt := make(map[string]struct{}, len(filteredAttempts))
					for i := range filteredAttempts {
						a := filteredAttempts[i]
						includedAttempt[a.Transaction.AuthoritySnapshotID+"|"+fmt.Sprintf("%d", a.Attempt)] = struct{}{}
					}
					includedReq := make(map[string]struct{}, len(filteredSessions))
					for i := range filteredSessions {
						includedReq[strings.TrimSpace(filteredSessions[i].LogicalRequestID)] = struct{}{}
					}
					for i := range attempts {
						if strings.TrimSpace(string(attempts[i].State)) != string(RuntimeCandidateAttemptStateAccepted) {
							continue
						}
						key := attempts[i].Transaction.AuthoritySnapshotID + "|" + fmt.Sprintf("%d", attempts[i].Attempt)
						if _, ok := includedAttempt[key]; ok {
							continue
						}
						reqID := strings.TrimSpace(attempts[i].LogicalRequestID)
						sess, ok := neededSessionMap[reqID]
						if !ok || sess.CandidateManifest == nil {
							continue
						}
						matchesNewBatch := len(sess.CandidateManifest.Bindings) == 0
						for _, b := range sess.CandidateManifest.Bindings {
							id := common.CoverageWorkItemID(strings.TrimSpace(string(b.CoverageWorkItemID)))
							if id == "" {
								matchesNewBatch = true
								break
							}
							if _, hit := delta.newBatchCoverage[id]; hit {
								matchesNewBatch = true
								break
							}
						}
						if matchesNewBatch {
							filteredAttempts = append(filteredAttempts, attempts[i])
							includedAttempt[key] = struct{}{}
							if _, hasSess := includedReq[reqID]; !hasSess {
								filteredSessions = append(filteredSessions, sess)
								includedReq[reqID] = struct{}{}
							}
						}
					}
					attempts, sessions = filteredAttempts, filteredSessions
				}
			}
		}
	}
	if validateActiveAssessmentPreimages && ledger.Manifest != nil && hasAcceptedAssessmentPurposeBatch(ledger.Manifest, ledger.Batches) {
		activeGen := ledger.WithoutHistory().Clone()
		if _, err := runtimeAssessmentPurposePreimagesFromSemanticGenerationInSession(activeGen, semanticSession); err != nil {
			return fmt.Errorf("accepted runtime assessment attempt purpose preimage: %w", err)
		}
	}
	acceptedAttempts := make([]RuntimeCandidateAttemptRecord, 0, len(attempts))
	neededDispatchIDs := make(map[string]struct{}, len(attempts))
	for i := range attempts {
		if strings.TrimSpace(string(attempts[i].State)) != string(RuntimeCandidateAttemptStateAccepted) {
			continue
		}
		attempt := attempts[i].Normalized()
		if attempt.State != RuntimeCandidateAttemptStateAccepted {
			continue
		}
		acceptedAttempts = append(acceptedAttempts, attempt)
		neededDispatchIDs[attempt.DispatchID] = struct{}{}
	}
	if len(acceptedAttempts) == 0 {
		return nil
	}
	dispatchByID := make(map[string]*RuntimeDispatchRecord, len(neededDispatchIDs))
	dispatchCounts := make(map[string]int, len(neededDispatchIDs))
	for i := range completed {
		id := completed[i].ID
		if _, needed := neededDispatchIDs[id]; needed {
			dispatchCounts[id]++
			dispatchByID[id] = &completed[i]
		}
	}
	if active != nil {
		if _, needed := neededDispatchIDs[active.ID]; needed {
			dispatchCounts[active.ID]++
			dispatchByID[active.ID] = active
		}
	}
	type pendingAssessment struct {
		dispatch *RuntimeDispatchRecord
		attempt  RuntimeCandidateAttemptRecord
	}
	var candidateAssessments []pendingAssessment
	neededRequests := make(map[string]struct{}, len(acceptedAttempts))
	for _, attempt := range acceptedAttempts {
		dispatch := dispatchByID[attempt.DispatchID]
		if dispatch == nil || dispatchCounts[attempt.DispatchID] != 1 {
			return fmt.Errorf("accepted runtime candidate attempt has no unique dispatch for assessment purpose proof")
		}
		if dispatch.Candidate.Assessment == nil {
			continue
		}
		candidateAssessments = append(candidateAssessments, pendingAssessment{dispatch: dispatch, attempt: attempt})
		neededRequests[attempt.LogicalRequestID] = struct{}{}
	}
	if len(candidateAssessments) == 0 {
		return nil
	}
	sessionManifestByRequest := make(map[string]*RuntimeCandidateContractManifest, len(neededRequests))
	sessionCounts := make(map[string]int, len(neededRequests))
	for i := range sessions {
		if _, needed := neededRequests[sessions[i].LogicalRequestID]; !needed {
			continue
		}
		sessionCounts[sessions[i].LogicalRequestID]++
		if sessions[i].CandidateManifest != nil {
			sessionManifestByRequest[sessions[i].LogicalRequestID] = sessions[i].CandidateManifest
		}
	}
	type acceptedAssessment struct {
		dispatch *RuntimeDispatchRecord
		manifest *RuntimeCandidateContractManifest
	}
	acceptedAssessments := make([]acceptedAssessment, 0, len(candidateAssessments))
	for _, item := range candidateAssessments {
		manifest := sessionManifestByRequest[item.attempt.LogicalRequestID]
		if manifest == nil || sessionCounts[item.attempt.LogicalRequestID] != 1 {
			return fmt.Errorf("accepted runtime assessment attempt has no unique pre-seal manifest")
		}
		acceptedAssessments = append(acceptedAssessments, acceptedAssessment{dispatch: item.dispatch, manifest: manifest})
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
	var neededCoverageIDs map[common.CoverageWorkItemID]struct{}
	if len(acceptedAssessments) > 0 {
		neededCoverageIDs = make(map[common.CoverageWorkItemID]struct{}, len(acceptedAssessments))
		for _, accepted := range acceptedAssessments {
			if len(accepted.manifest.Bindings) == 0 {
				neededCoverageIDs = nil
				break
			}
			for _, binding := range accepted.manifest.Bindings {
				if binding.CoverageWorkItemID == "" {
					neededCoverageIDs = nil
					break
				}
				neededCoverageIDs[binding.CoverageWorkItemID] = struct{}{}
			}
			if neededCoverageIDs == nil {
				break
			}
		}
	}
	preimages, err := runtimeAssessmentPurposePreimagesFromSemanticLedgerForCoverageInSession(ledger, true, neededCoverageIDs, semanticSession)
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
