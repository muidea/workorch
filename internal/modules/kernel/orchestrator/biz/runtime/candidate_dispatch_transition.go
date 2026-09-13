package runtime

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *candidateDispatchComponent) beginCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string) (RuntimeAggregate, error) {
	if err := validateBeginCandidateAttemptInput(c, workspaceID, runID, attempt, stablePrefixHash, logicalRequestID); err != nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := validateCandidateAttemptBeginAuthority(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := validateCandidateAttemptSessionBinding(aggregate, stablePrefixHash, manifest, logicalRequestID); err != nil {
		return RuntimeAggregate{}, err
	}
	sessionIndex, err := candidateRequestSessionManifestBinding(aggregate, logicalRequestID, manifest)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	if attempt == 1 {
		if err := validatePersistedCandidateCorrection(aggregate, manifest); err != nil {
			return RuntimeAggregate{}, fmt.Errorf("%w: %v", common.ErrConflict, err)
		}
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, transaction)
	current := candidateAttemptsForContract(aggregate.CandidateAttempts, workspaceID, runID, contractID)
	plan, err := planCandidateAttemptBegin(aggregate, workspaceID, runID, attempt, stablePrefixHash, manifest.Hash, logicalRequestID, contractID, transaction, current)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if plan.duplicate {
		return aggregate, nil
	}
	expectedRevision := aggregate.StateRevision
	manifestCopy := manifest.Clone()
	aggregate.CandidateRequestSessions[sessionIndex].CandidateManifest = &manifestCopy
	applyCandidateAttemptBegin(&aggregate, plan, contractID, transaction, attempt, stablePrefixHash, manifest.Hash, logicalRequestID)
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// candidateRequestSessionManifestBinding resolves the earlier operations
// request CAS that must retain the complete candidate manifest before any
// candidate attempt can be persisted.  The accepted dispatch later re-derives
// every projection from this body; hashes copied between attempt and dispatch
// are therefore not a standalone authority root.
func candidateRequestSessionManifestBinding(aggregate RuntimeAggregate, logicalRequestID string, manifest runtimecontract.RuntimeCandidateContractManifest) (int, error) {
	idx, session, found := runtimeCandidateRequestSessionByID(aggregate.CandidateRequestSessions, logicalRequestID)
	if !found || session.DocumentKind != runtimecontract.RuntimeProviderDocumentOperations || session.ManifestHash != manifest.Hash {
		return 0, fmt.Errorf("%w: candidate request session has no exact operations manifest binding", common.ErrConflict)
	}
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestRawCanonical(manifest); err != nil {
		return 0, fmt.Errorf("%w: candidate request session manifest is not canonical", common.ErrConflict)
	}
	if session.CandidateManifest != nil {
		if err := runtimecontract.ValidateRuntimeCandidateContractManifestRawCanonical(*session.CandidateManifest); err != nil ||
			!reflect.DeepEqual(*session.CandidateManifest, manifest) {
			return 0, fmt.Errorf("%w: candidate request session manifest body changed", common.ErrConflict)
		}
	}
	return idx, nil
}

type candidateAttemptBeginPlan struct {
	duplicate    bool
	recycleIndex int
}

func validateBeginCandidateAttemptInput(c *candidateDispatchComponent, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash, logicalRequestID string) error {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(stablePrefixHash) == "" || strings.TrimSpace(logicalRequestID) == "" {
		return common.ErrInvalidArgument
	}
	return nil
}

func validateCandidateAttemptBeginAuthority(aggregate RuntimeAggregate) error {
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return err
	}
	if err := rejectCandidateBeforeRepairSurfaceConfirmation(aggregate); err != nil {
		return err
	}
	switch aggregate.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment:
	default:
		return fmt.Errorf("%w: current runtime action does not request a candidate", common.ErrConflict)
	}
	if err := validateCurrentCWIBindingFence(aggregate.Facts); err != nil {
		return fmt.Errorf("%w: candidate request binding fence: %v", common.ErrConflict, err)
	}
	return nil
}

func planCandidateAttemptBegin(aggregate RuntimeAggregate, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash, manifestHash, logicalRequestID, contractID string, transaction runtimecontract.AuthorityTransaction, current []runtimecontract.RuntimeCandidateAttemptRecord) (candidateAttemptBeginPlan, error) {
	if attempt == 0 && len(current) == 0 && equivalentOwnerInfeasibilityExists(aggregate.CandidateAttempts, transaction.AuthorityInputID) {
		return candidateAttemptBeginPlan{}, fmt.Errorf("%w: equivalent authority already produced owner infeasibility", common.ErrConflict)
	}
	if record, found := candidateAttemptRecord(current, attempt); found {
		if record.State == runtimecontract.RuntimeCandidateAttemptStatePending && record.StablePrefixHash == stablePrefixHash && record.Transaction == transaction && record.ManifestHash == manifestHash && record.LogicalRequestID == logicalRequestID {
			return candidateAttemptBeginPlan{duplicate: true}, nil
		}
		if attempt == 0 && record.State == runtimecontract.RuntimeCandidateAttemptStateInvalidated && record.StablePrefixHash == stablePrefixHash {
			idx, ok := candidateAttemptIndex(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
			if ok {
				return candidateAttemptBeginPlan{recycleIndex: idx}, nil
			}
		}
		return candidateAttemptBeginPlan{}, fmt.Errorf("%w: candidate attempt %d is already %s", common.ErrConflict, attempt, record.State)
	}
	if err := validateCandidateAttemptSequence(current, attempt, stablePrefixHash, manifestHash, transaction); err != nil {
		return candidateAttemptBeginPlan{}, err
	}
	return candidateAttemptBeginPlan{recycleIndex: -1}, nil
}

func validateCandidateAttemptSequence(current []runtimecontract.RuntimeCandidateAttemptRecord, attempt int, stablePrefixHash, manifestHash string, transaction runtimecontract.AuthorityTransaction) error {
	if attempt == 0 {
		if len(current) != 0 {
			return fmt.Errorf("%w: candidate attempt 0 is no longer available", common.ErrConflict)
		}
		return nil
	}
	first, ok := candidateAttemptRecord(current, 0)
	if !ok || first.State != runtimecontract.RuntimeCandidateAttemptStateFailed || first.StablePrefixHash != stablePrefixHash || first.ManifestHash != manifestHash || !runtimecontract.SameAuthorityContract(first.Transaction, transaction) {
		return fmt.Errorf("%w: candidate attempt 1 requires a failed same-contract attempt 0", common.ErrConflict)
	}
	if !CandidateAttemptIsCorrectableOnce(first) {
		return fmt.Errorf("%w: candidate attempt 1 requires a sealed correctable_once attempt 0 failure", common.ErrConflict)
	}
	return nil
}

func candidateAttemptIndex(values []runtimecontract.RuntimeCandidateAttemptRecord, workspaceID common.WorkspaceID, runID common.RunID, contractID string, attempt int) (int, bool) {
	for idx, record := range values {
		if runtimecontract.CandidateAttemptAuthorityContractID(workspaceID, runID, record) == contractID && record.Attempt == attempt {
			return idx, true
		}
	}
	return 0, false
}

func applyCandidateAttemptBegin(aggregate *RuntimeAggregate, plan candidateAttemptBeginPlan, contractID string, transaction runtimecontract.AuthorityTransaction, attempt int, stablePrefixHash, manifestHash, logicalRequestID string) {
	record := runtimecontract.RuntimeCandidateAttemptRecord{
		AuthorityContractID: contractID, Transaction: transaction, Attempt: attempt, StablePrefixHash: stablePrefixHash,
		ManifestHash: manifestHash, LogicalRequestID: logicalRequestID, State: runtimecontract.RuntimeCandidateAttemptStatePending,
	}
	if plan.recycleIndex < 0 {
		aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, record)
		return
	}
	recycled := &aggregate.CandidateAttempts[plan.recycleIndex]
	recycled.Transaction = transaction
	recycled.StablePrefixHash = stablePrefixHash
	recycled.ManifestHash = manifestHash
	recycled.LogicalRequestID = logicalRequestID
	recycled.State = runtimecontract.RuntimeCandidateAttemptStatePending
	recycled.Proposal = nil
	recycled.Conformances = nil
	recycled.Feasibilities = nil
	recycled.FeasibilityCatalogs = nil
	recycled.Failure = nil
	recycled.Correction = nil
	recycled.Stale = nil
}

func equivalentOwnerInfeasibilityExists(attempts []runtimecontract.RuntimeCandidateAttemptRecord, authorityInputID string) bool {
	return equivalentOwnerInfeasibility(attempts, authorityInputID) != nil
}

func equivalentOwnerInfeasibility(attempts []runtimecontract.RuntimeCandidateAttemptRecord, authorityInputID string) *runtimecontract.RuntimeOwnerFeasibilityObservation {
	authorityInputID = strings.TrimSpace(authorityInputID)
	if authorityInputID == "" {
		return nil
	}
	for _, attempt := range attempts {
		if attempt.State == runtimecontract.RuntimeCandidateAttemptStateInvalidated && attempt.OwnerFeasibility != nil &&
			attempt.Transaction.AuthorityInputID == authorityInputID {
			value := attempt.OwnerFeasibility.Clone()
			return &value
		}
	}
	return nil
}

// rejectCompletedCandidateDocument is the single SM-3 attempt transition for
// a settled document result that cannot become an executable candidate. For
// empty/invalid framing, the request session is first closed by its own durable
// owner-private CAS; a restart then re-enters this idempotent second step. This
// step creates and closes the attempt in one aggregate CAS, so restart never
// observes a semantically meaningless pending attempt.
func (c *candidateDispatchComponent) rejectCompletedCandidateDocument(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, rejectedCandidate *runtimecontract.RuntimeCandidate, failure runtimecontract.RuntimeCandidateFailure) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(stablePrefixHash) == "" || strings.TrimSpace(logicalRequestID) == "" || failure.Class == "" || strings.TrimSpace(failure.Reason) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, transaction)
	duplicate, err := inspectCompletedCandidateDocumentReplay(aggregate, workspaceID, runID, attempt, manifest.Hash, logicalRequestID, contractID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if duplicate {
		return aggregate, nil
	}
	if err := validateCandidateAttemptSessionBinding(aggregate, stablePrefixHash, manifest, logicalRequestID); err != nil {
		return RuntimeAggregate{}, err
	}
	sessionIndex, err := candidateRequestSessionManifestBinding(aggregate, logicalRequestID, manifest)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if !runtimeDecisionAcceptsCandidateDocument(aggregate.Decision.State) {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not request a candidate", common.ErrConflict)
	}
	idx, err := ensurePendingCompletedDocumentAttempt(&aggregate, workspaceID, runID, attempt, stablePrefixHash, manifest.Hash, logicalRequestID, contractID, transaction)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	frontier := candidateCorrectionFrontier{}
	if rejectedCandidate != nil {
		candidateCopy := *rejectedCandidate
		frontier = candidateCorrectionFrontierForEvaluation(candidateAdmissionEvaluationInput(aggregate, manifest, candidateCopy))
	}
	sealedFailure, correction, sealErr := sealCandidateRejectionWithFrontier(&failure, manifest, frontier)
	if sealErr != nil {
		return RuntimeAggregate{}, sealErr
	}
	if sealedFailure == nil || correction == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: completed candidate rejection has no sealed disposition", common.ErrConflict)
	}
	if rejectedCandidate != nil {
		candidateCopy := *rejectedCandidate
		aggregate.CandidateAttempts[idx].Proposal = &candidateCopy
	}
	// A rejected complete document is still an SM-3 attempt. Persist the same
	// pre-attempt manifest body used by the accepted-proposal path in this
	// owner-private CAS, so correction lineage can be proved after restart.
	manifestCopy := manifest.Clone()
	aggregate.CandidateRequestSessions[sessionIndex].CandidateManifest = &manifestCopy
	if err := closeCandidateAttemptWithCorrection(&aggregate, idx, attempt, *sealedFailure, correction); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func inspectCompletedCandidateDocumentReplay(aggregate RuntimeAggregate, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifestHash, logicalRequestID, contractID string) (bool, error) {
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt || record.ManifestHash != manifestHash || record.LogicalRequestID != logicalRequestID {
			continue
		}
		if record.State == runtimecontract.RuntimeCandidateAttemptStateFailed {
			return true, nil
		}
		if runtimecontract.CandidateAttemptAuthorityContractID(workspaceID, runID, record) != contractID || record.State != runtimecontract.RuntimeCandidateAttemptStatePending {
			return false, fmt.Errorf("%w: completed candidate document attempt %d is already %s", common.ErrConflict, attempt, record.State)
		}
	}
	return false, nil
}

func runtimeDecisionAcceptsCandidateDocument(state runtimecontract.ConvergenceState) bool {
	switch state {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment:
		return true
	default:
		return false
	}
}

func ensurePendingCompletedDocumentAttempt(aggregate *RuntimeAggregate, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash, manifestHash, logicalRequestID, contractID string, transaction runtimecontract.AuthorityTransaction) (int, error) {
	if idx, _, pending := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt); pending {
		return idx, nil
	}
	current := candidateAttemptsForContract(aggregate.CandidateAttempts, workspaceID, runID, contractID)
	if err := validateCompletedDocumentAttemptSequence(current, attempt, stablePrefixHash, manifestHash, transaction); err != nil {
		return 0, err
	}
	aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{
		AuthorityContractID: contractID, Transaction: transaction, Attempt: attempt, StablePrefixHash: stablePrefixHash,
		ManifestHash: manifestHash, LogicalRequestID: logicalRequestID, State: runtimecontract.RuntimeCandidateAttemptStatePending,
	})
	return len(aggregate.CandidateAttempts) - 1, nil
}

func validateCompletedDocumentAttemptSequence(current []runtimecontract.RuntimeCandidateAttemptRecord, attempt int, stablePrefixHash, manifestHash string, transaction runtimecontract.AuthorityTransaction) error {
	if attempt == 0 {
		if len(current) != 0 {
			return fmt.Errorf("%w: candidate attempt 0 is no longer available", common.ErrConflict)
		}
		return nil
	}
	first, ok := candidateAttemptRecord(current, 0)
	if !ok || first.State != runtimecontract.RuntimeCandidateAttemptStateFailed || first.StablePrefixHash != stablePrefixHash || first.ManifestHash != manifestHash || !runtimecontract.SameAuthorityContract(first.Transaction, transaction) || !CandidateAttemptIsCorrectableOnce(first) {
		return fmt.Errorf("%w: candidate attempt 1 requires a failed correctable same-contract attempt 0", common.ErrConflict)
	}
	return nil
}

// admitCandidateProposal atomically persists a candidate proposal or closes the
// pending attempt with its typed admission rejection. A restart therefore never
// observes a rejected provider candidate as an open SM-3 transaction.
func (c *candidateDispatchComponent) admitCandidateProposal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (CandidateAdmissionCommitResult, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(logicalRequestID) == "" {
		return CandidateAdmissionCommitResult{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	// A response-loss replay is an immutable query of the already committed
	// SM-3 result, not Runtime progress. Prove that exact durable identity before
	// the terminal fence so restart can return the original rejection without a
	// second CAS. Any different delivery continues through the ordinary terminal
	// and pending-attempt guards below and therefore cannot reopen authority.
	if duplicate, found := duplicateRejectedCandidateAdmission(aggregate, attempt, manifest.Hash, logicalRequestID, candidate); found {
		return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: duplicate}, nil
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, record, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !ok {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	if err := validateCandidateAttemptSessionBinding(aggregate, "", manifest, logicalRequestID); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if err := validateCurrentCWIBindingFence(aggregate.Facts); err != nil {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate proposal binding fence: %v", common.ErrConflict, err)
	}
	if record.ManifestHash != manifest.Hash || record.LogicalRequestID != logicalRequestID {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate proposal does not match pending request-session binding", common.ErrConflict)
	}
	sessionIndex, err := candidateRequestSessionManifestBinding(aggregate, logicalRequestID, manifest)
	if err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	admission := EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate)
	if !admission.Accepted() {
		expectedRevision := aggregate.StateRevision
		// A structurally valid rejected proposal is useful durable diagnostics and
		// allows the next attempt to explain exactly what was rejected. Invalid
		// candidates cannot be persisted because state validation must remain
		// schema-safe even after a failed attempt.
		if candidate.ValidateWithActivation(aggregate.Decision, runtimecontract.RuntimeActivationForFacts(aggregate.Facts)) == nil {
			proposal := candidate
			aggregate.CandidateAttempts[idx].Proposal = &proposal
		}
		if err := closeCandidateAttemptWithCorrection(&aggregate, idx, attempt, *admission.Failure, admission.Correction); err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		aggregate.StateRevision++
		if err := aggregate.Validate(); err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
	}
	repairCommitment, err := deriveConfirmedRepairAdmissionCommitment(aggregate, manifest, candidate)
	if err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	session := aggregate.CandidateRequestSessions[sessionIndex]
	if !runtimecontract.SameRuntimeDispatchRepairCommitment(session.RepairCommitment, repairCommitment) {
		if session.RepairCommitment != nil || session.RepairCommitmentHash != "" {
			return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: pending candidate repair commitment changed", common.ErrConflict)
		}
	} else if repairCommitment == nil && session.RepairCommitmentHash != "" {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: pending candidate has an orphan repair commitment hash", common.ErrConflict)
	}
	if record.Proposal != nil {
		if runtimecontract.StableRuntimeCandidateHash(*record.Proposal) != runtimecontract.StableRuntimeCandidateHash(candidate) {
			return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: pending candidate attempt proposal changed", common.ErrConflict)
		}
		if repairCommitment != nil &&
			session.RepairCommitmentHash != runtimecontract.StableRuntimeDispatchRepairCommitmentHash(*repairCommitment) {
			return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: pending candidate repair commitment hash changed", common.ErrConflict)
		}
		return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
	}
	expectedRevision := aggregate.StateRevision
	proposal := candidate
	aggregate.CandidateAttempts[idx].Proposal = &proposal
	aggregate.CandidateRequestSessions[sessionIndex].RepairCommitment = cloneRuntimeDispatchRepairCommitment(repairCommitment)
	if repairCommitment != nil {
		aggregate.CandidateRequestSessions[sessionIndex].RepairCommitmentHash = runtimecontract.StableRuntimeDispatchRepairCommitmentHash(*repairCommitment)
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
}

// recordCandidateFeasibility is the sole SM-2/SM-3 commit for one completed,
// side-effect-free owner probe. The same CAS persists the factual result and,
// for every non-feasible outcome, its Orchestrator-owned disposition. Restart
// therefore cannot observe a pending candidate with an undecided owner fact.
func (c *candidateDispatchComponent) recordCandidateFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, record runtimecontract.RuntimeCandidateFeasibilityRecord) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if err := runtimecontract.ValidateRuntimeCandidateFeasibilityRecords([]runtimecontract.RuntimeCandidateFeasibilityRecord{record}); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		if duplicateCandidateFeasibilityDisposition(aggregate.CandidateAttempts, attempt, record) {
			return aggregate, nil
		}
		return RuntimeAggregate{}, err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, attemptRecord, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !ok {
		// Only a late delivery may match historical disposition. Never let an
		// identical record from an older authority shadow a current pending
		// attempt that must receive its own fact and CAS disposition.
		if duplicateCandidateFeasibilityDisposition(aggregate.CandidateAttempts, attempt, record) {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d has no pending proposal", common.ErrConflict, attempt)
	}
	if attemptRecord.Proposal == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d has no pending proposal", common.ErrConflict, attempt)
	}
	operation, operationIndex, operationFound := candidateOperationAndIndexByID(*attemptRecord.Proposal, record.OperationID)
	if !operationFound {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate feasibility operation %q is absent from the persisted proposal", common.ErrConflict, record.OperationID)
	}
	if record.CandidateManifestHash != attemptRecord.ManifestHash {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate feasibility operation %q does not match the pending manifest", common.ErrConflict, record.OperationID)
	}
	manifest, err := persistedCandidateAttemptManifest(aggregate, attemptRecord)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := validateCandidateFeasibilityObservationBinding(aggregate.Facts.Resources.OperationAdmissibilities, operation, record, manifest); err != nil {
		return RuntimeAggregate{}, err
	}
	for _, persisted := range attemptRecord.Feasibilities {
		if strings.TrimSpace(persisted.OperationID) != strings.TrimSpace(record.OperationID) {
			continue
		}
		if persisted.AdmissibilitySnapshotID != record.AdmissibilitySnapshotID || persisted.AcquisitionBinding != record.AcquisitionBinding || !reflect.DeepEqual(persisted.ReplayProof, record.ReplayProof) || !reflect.DeepEqual(persisted.Observation, record.Observation) {
			return RuntimeAggregate{}, fmt.Errorf("%w: candidate feasibility result for operation %q changed", common.ErrConflict, record.OperationID)
		}
		return aggregate, nil
	}
	expectedRevision := aggregate.StateRevision
	copyRecord := record
	if record.Observation != nil {
		observation := record.Observation.Clone()
		copyRecord.Observation = &observation
	}
	if record.ReplayProof != nil {
		proof := record.ReplayProof.Clone()
		copyRecord.ReplayProof = &proof
	}
	aggregate.CandidateAttempts[idx].Feasibilities = append(aggregate.CandidateAttempts[idx].Feasibilities, copyRecord)
	observation := copyRecord.Observation.Clone()
	switch observation.Outcome {
	case runtimecontract.RuntimeOwnerFeasibilityFeasible:
		// The persisted fact is the restart fence for final admission.
	case runtimecontract.RuntimeOwnerFeasibilityStale:
		if err := applyCandidatePrerequisiteStale(&aggregate, idx, *observation.Stale); err != nil {
			return RuntimeAggregate{}, err
		}
	case runtimecontract.RuntimeOwnerFeasibilityPrerequisiteRequired:
		if err := validateCandidateFeasibilityPrerequisiteDisposition(aggregate, aggregate.CandidateAttempts[idx], operation, *observation.PrerequisiteRequest); err != nil {
			return RuntimeAggregate{}, err
		}
		if err := applyCandidateFeasibilityPrerequisite(&aggregate, idx, *observation.PrerequisiteRequest); err != nil {
			return RuntimeAggregate{}, err
		}
	case runtimecontract.RuntimeOwnerFeasibilityCandidateConstraintViolated:
		failure := candidateApplicabilityConstraintFailure(operation, operationIndex, manifest, *observation.CandidateConstraintViolation)
		sealed, correction, sealErr := sealCandidateRejectionWithFrontier(&failure, manifest, candidateCorrectionFrontier{candidate: attemptRecord.Proposal})
		if sealErr != nil || sealed == nil || correction == nil {
			return RuntimeAggregate{}, fmt.Errorf("%w: seal owner candidate applicability rejection: %v", common.ErrConflict, sealErr)
		}
		if err := closeCandidateAttemptWithCorrection(&aggregate, idx, attempt, *sealed, correction); err != nil {
			return RuntimeAggregate{}, err
		}
	case runtimecontract.RuntimeOwnerFeasibilityInfeasible, runtimecontract.RuntimeOwnerFeasibilityContractDefect:
		currentAttempt := aggregate.CandidateAttempts[idx]
		if err := applyCandidateOwnerInfeasibility(&aggregate, idx, currentAttempt, copyRecord, observation); err != nil {
			return RuntimeAggregate{}, err
		}
	default:
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func duplicateCandidateFeasibilityDisposition(attempts []runtimecontract.RuntimeCandidateAttemptRecord, attempt int, record runtimecontract.RuntimeCandidateFeasibilityRecord) bool {
	for _, historical := range attempts {
		if historical.Attempt != attempt {
			continue
		}
		for _, persisted := range historical.Feasibilities {
			if reflect.DeepEqual(persisted, record) {
				return true
			}
		}
		if record.Observation == nil {
			continue
		}
		observation := record.Observation.Clone()
		if historical.State != runtimecontract.RuntimeCandidateAttemptStateInvalidated {
			continue
		}
		switch observation.Outcome {
		case runtimecontract.RuntimeOwnerFeasibilityStale:
			if historical.Stale != nil && reflect.DeepEqual(historical.Stale.Normalized(), observation.Stale.Normalized()) {
				return true
			}
		case runtimecontract.RuntimeOwnerFeasibilityPrerequisiteRequired:
			if historical.PrerequisiteRequest != nil && historical.PrerequisiteRequest.Same(*observation.PrerequisiteRequest) {
				return true
			}
		case runtimecontract.RuntimeOwnerFeasibilityInfeasible, runtimecontract.RuntimeOwnerFeasibilityContractDefect:
			if historical.OwnerFeasibility != nil && reflect.DeepEqual(historical.OwnerFeasibility.Clone(), observation) {
				return true
			}
		}
	}
	return false
}

func validateCandidateFeasibilityObservationBinding(catalogs []runtimecontract.RuntimeOperationAdmissibility, operation runtimecontract.RuntimeOperation, record runtimecontract.RuntimeCandidateFeasibilityRecord, manifest runtimecontract.RuntimeCandidateContractManifest) error {
	for _, authorization := range manifest.Operations {
		if authorization.Kind == operation.Kind && authorization.InputRepair != nil && record.Observation != nil &&
			record.Observation.Outcome == runtimecontract.RuntimeOwnerFeasibilityFeasible && !authorization.InputRepair.MatchesReplayProof(record.ReplayProof) {
			return fmt.Errorf("%w: input repair feasibility must prove the sealed exact content delta", common.ErrConflict)
		}
	}
	if record.Observation == nil || record.Observation.Outcome != runtimecontract.RuntimeOwnerFeasibilityCandidateConstraintViolated {
		return nil
	}
	observation := record.Observation.Clone()
	violation := observation.CandidateConstraintViolation
	if violation == nil {
		return fmt.Errorf("%w: candidate applicability observation has no violation", common.ErrConflict)
	}
	constraint, found := runtimecontract.RuntimeCandidateConstraintForOperationRef(manifest, operation.Kind, violation.ConstraintRef)
	if !found || constraint.Kind != runtimecontract.RuntimeOperationCandidateConstraintTargetApplicability ||
		constraint.ParameterName != violation.ParameterName || constraint.RequiredShape != violation.RequiredShape {
		return fmt.Errorf("%w: candidate applicability violation is absent from the sealed manifest", common.ErrConflict)
	}
	selected := false
	for _, authorization := range manifest.Operations {
		if authorization.Kind != operation.Kind {
			continue
		}
		_, constraints, err := runtimecontract.RuntimeOperationCandidateConstraintsForOperation(operation, authorization)
		if err != nil {
			return err
		}
		for _, value := range constraints {
			if value == constraint {
				selected = true
			}
		}
	}
	if !selected {
		return fmt.Errorf("%w: candidate applicability violation does not match the selected operation variant", common.ErrConflict)
	}
	if !runtimeOperationTargetsRef(operation, violation.TargetRef) {
		return fmt.Errorf("%w: candidate applicability violation target does not match the proposed operation", common.ErrConflict)
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || descriptor.OwnerModuleID == "" || observation.OwnerPolicy.OwnerModuleID != descriptor.OwnerModuleID {
		return fmt.Errorf("%w: candidate applicability violation owner does not match the proposed operation", common.ErrConflict)
	}
	for _, inventory := range catalogs {
		inventory = inventory.Normalized()
		if inventory.SnapshotID == record.AdmissibilitySnapshotID && inventory.AcquisitionBinding == record.AcquisitionBinding &&
			inventory.OwnerModuleID == descriptor.OwnerModuleID && inventory.ObservedRevision == violation.ObservedRevision {
			return nil
		}
	}
	return fmt.Errorf("%w: candidate applicability violation is not bound to its sealed owner observation", common.ErrConflict)
}

func validateCandidateFeasibilityPrerequisiteDisposition(aggregate RuntimeAggregate, attemptRecord runtimecontract.RuntimeCandidateAttemptRecord, operation runtimecontract.RuntimeOperation, request runtimecontract.RuntimeOperationPrerequisiteRequest) error {
	if !runtimecontract.RuntimeOperationRequiresCurrentTargetSnapshot(operation) || !runtimeOperationTargetsRef(operation, request.SubjectRef) {
		return fmt.Errorf("%w: prerequisite request is not bound to the pending candidate operation", common.ErrConflict)
	}
	descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !known || descriptor.OwnerModuleID != request.OwnerModuleID {
		return fmt.Errorf("%w: prerequisite request owner does not match candidate operation", common.ErrConflict)
	}
	feasibility, found := candidateAttemptFeasibilityForOperation(attemptRecord.Feasibilities, request.OperationID)
	if !found || feasibility.Observation == nil || feasibility.Observation.PrerequisiteRequest == nil || !feasibility.Observation.PrerequisiteRequest.Same(request) {
		return fmt.Errorf("%w: prerequisite request has no matching persisted owner feasibility fact", common.ErrConflict)
	}
	return nil
}

func candidateApplicabilityConstraintFailure(operation runtimecontract.RuntimeOperation, operationIndex int, manifest runtimecontract.RuntimeCandidateContractManifest, violation runtimecontract.RuntimeOperationCandidateConstraintViolation) runtimecontract.RuntimeCandidateFailure {
	return runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract,
		Code:  "candidate_owner_constraint_nonconformant", Signature: "candidate-owner-constraint-nonconformant",
		Reason:     "candidate parameter does not apply to the owner-observed target under its sealed constraint",
		TargetRefs: []string{violation.TargetRef},
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			OperationID: operation.ID,
			Path:        fmt.Sprintf("$.operations[%d].parameters[name=%s]", operationIndex, violation.ParameterName),
			Actual:      string(violation.ActualShape), ConstraintRef: violation.ConstraintRef,
			RequiredShape: string(violation.RequiredShape), ManifestHash: manifest.Hash,
			Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionConformOwnerConstraint},
		},
	}
}

func candidateProposalContainsOperation(candidate runtimecontract.RuntimeCandidate, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	if wanted == "" {
		return false
	}
	if candidate.StateChange != nil {
		for _, value := range candidate.StateChange.Operations {
			if value.Operation.Normalized().ID == wanted {
				return true
			}
		}
	}
	if candidate.Validation != nil {
		for _, value := range candidate.Validation.Operations {
			if value.Operation.Normalized().ID == wanted {
				return true
			}
		}
	}
	return false
}

// admitAndSealCandidateAttempt is the final SM-3 admission fence. It atomically
// closes an expected rejection or atomically accepts the attempt and seals its
// dispatch. Feasibility/invariant defects remain ordinary errors because they
// are not provider-correctable candidate outcomes.
func (c *candidateDispatchComponent) admitAndSealCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (CandidateAdmissionCommitResult, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(logicalRequestID) == "" {
		return CandidateAdmissionCommitResult{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, record, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !ok {
		if duplicate, found := duplicateRejectedCandidateAdmission(aggregate, attempt, manifest.Hash, logicalRequestID, candidate); found {
			return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: duplicate}, nil
		}
		if acceptedCandidateAdmissionDuplicate(aggregate, attempt, manifest.Hash, logicalRequestID, candidate) {
			return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: acceptedCandidateAdmission()}, nil
		}
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	if err := validateCandidateAttemptSessionBinding(aggregate, "", manifest, logicalRequestID); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if err := validateCurrentCWIBindingFence(aggregate.Facts); err != nil {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate dispatch binding fence: %v", common.ErrConflict, err)
	}
	if record.ManifestHash != manifest.Hash || record.LogicalRequestID != logicalRequestID {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate seal does not match pending request-session binding", common.ErrConflict)
	}
	sessionIndex, err := candidateRequestSessionManifestBinding(aggregate, logicalRequestID, manifest)
	if err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	admission := EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate)
	if !admission.Accepted() {
		expectedRevision := aggregate.StateRevision
		// Final admission may be reached directly after a restart or alternate
		// owner probe. Preserve a schema-valid rejected candidate here too, so
		// duplicate delivery has the same durable identity as proposal admission.
		if candidate.ValidateWithActivation(aggregate.Decision, runtimecontract.RuntimeActivationForFacts(aggregate.Facts)) == nil {
			proposal := candidate
			aggregate.CandidateAttempts[idx].Proposal = &proposal
		}
		if err := closeCandidateAttemptWithCorrection(&aggregate, idx, attempt, *admission.Failure, admission.Correction); err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		aggregate.StateRevision++
		if err := aggregate.Validate(); err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
			return CandidateAdmissionCommitResult{}, err
		}
		return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
	}
	if err := validateCandidateConformanceAdmission(record, manifest, candidate); err != nil {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate conformance admission: %v", common.ErrConflict, err)
	}
	repairCommitment, err := deriveConfirmedRepairAdmissionCommitment(aggregate, manifest, candidate)
	if err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	session := aggregate.CandidateRequestSessions[sessionIndex]
	if !runtimecontract.SameRuntimeDispatchRepairCommitment(session.RepairCommitment, repairCommitment) {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: final candidate repair commitment does not match its pre-seal request session", common.ErrConflict)
	}
	wantRepairCommitmentHash := ""
	if repairCommitment != nil {
		wantRepairCommitmentHash = runtimecontract.StableRuntimeDispatchRepairCommitmentHash(*repairCommitment)
	}
	if session.RepairCommitmentHash != wantRepairCommitmentHash {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: final candidate repair commitment hash does not match its pre-seal request session", common.ErrConflict)
	}
	// Persist the accepted admission's target and managed-validation recipe
	// projections from the exact manifest/candidate preimage before constructing
	// the dispatch. These commitments must never be copied back from a dispatch
	// that can rehash its own fields during a coordinated persistence mutation.
	admissionTargetAuthorizations, err := runtimecontract.RuntimeCandidateAdmissionTargetAuthorizationsFromManifest(manifest, candidate)
	if err != nil {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: derive candidate admission target projection: %v", common.ErrConflict, err)
	}
	admissionCoverageBindings := runtimecontract.CloneCoverageEvidenceBindings(manifest.Normalized().Bindings)
	if err := runtimecontract.ValidateCoverageEvidenceBindingProjection(admissionCoverageBindings); err != nil {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: derive candidate admission coverage-binding projection: %v", common.ErrConflict, err)
	}
	admissionValidationRecipes, err := runtimecontract.RuntimeCandidateManagedValidationRecipeSnapshot(manifest, candidate)
	if err != nil {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: derive candidate admission validation-recipe projection: %v", common.ErrConflict, err)
	}
	dispatch, completeAdmission, err := admitRuntimeDispatch(aggregate, manifest, candidate, &record, &admission, repairCommitment)
	if !completeAdmission.Admitted() {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: complete candidate admission %s: %s", common.ErrConflict, completeAdmission.Code, completeAdmission.Message)
	}
	if err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if record.Transaction != dispatch.Transaction || aggregate.ActiveDispatch != nil {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: candidate attempt does not match current dispatch state", common.ErrConflict)
	}
	if record.Proposal != nil && runtimecontract.StableRuntimeCandidateHash(*record.Proposal) != runtimecontract.StableRuntimeCandidateHash(candidate) {
		return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: accepted candidate does not match the persisted proposal", common.ErrConflict)
	}
	if err := runtimecontract.CheckRuntimeDurableDispatchBudget(aggregate.Facts.Activation.ExecutionContract.ExecutionLimits, sealedDispatchRecord(aggregate.ActiveDispatch), sealedDispatchesToRuntimeRecords(aggregate.CompletedDispatches), aggregate.CandidateAttempts, sealedDispatchToRuntimeRecord(dispatch)); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateAccepted
	aggregate.CandidateAttempts[idx].Proposal = nil
	aggregate.CandidateAttempts[idx].Conformances = nil
	aggregate.CandidateAttempts[idx].Feasibilities = nil
	aggregate.CandidateAttempts[idx].CandidateHash = dispatch.CandidateHash
	aggregate.CandidateAttempts[idx].DispatchID = dispatch.ID
	aggregate.CandidateAttempts[idx].AdmissionTargetAuthorizations = runtimecontract.CloneRuntimeDispatchOperationTargetAuthorizations(admissionTargetAuthorizations)
	aggregate.CandidateAttempts[idx].TargetAuthorizationHash = runtimecontract.StableRuntimeDispatchOperationTargetAuthorizationsHash(admissionTargetAuthorizations)
	aggregate.CandidateAttempts[idx].AdmissionCoverageBindings = admissionCoverageBindings
	aggregate.CandidateAttempts[idx].AdmissionCoverageBindingHash = runtimecontract.StableRuntimeCoverageEvidenceBindingSnapshotHash(admissionCoverageBindings)
	aggregate.CandidateAttempts[idx].AdmissionValidationRecipes = runtimecontract.CloneRuntimeValidationRecipes(admissionValidationRecipes)
	if len(admissionValidationRecipes) > 0 {
		aggregate.CandidateAttempts[idx].ValidationRecipeHash = runtimecontract.StableRuntimeValidationRecipeSnapshotHash(admissionValidationRecipes)
	}
	aggregate.CandidateAttempts[idx].AdmissionRepairCommitment = cloneRuntimeDispatchRepairCommitment(repairCommitment)
	aggregate.CandidateAttempts[idx].RepairCommitmentHash = wantRepairCommitmentHash
	aggregate.CandidateAttempts[idx].DispatchAuthorityHash = dispatch.DispatchAuthorityHash
	aggregate.ActiveDispatch = &dispatch
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
}

// sealGovernanceDispatch seals the reducer-authored external dependency without
// creating a provider request or candidate attempt. Governance is an SM-2
// fact route, not a synthetic LLM correction path; therefore it must never
// consume the complete-document/session ledger used by SM-3 provider output.
func (c *candidateDispatchComponent) sealGovernanceDispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedGovernance || aggregate.ActiveDispatch != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime authority cannot seal governance", common.ErrConflict)
	}
	if candidate.Governance == nil || candidate.ValidateWithActivation(aggregate.Decision, runtimecontract.RuntimeActivationForFacts(aggregate.Facts)) != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance candidate does not match sealed authority", common.ErrConflict)
	}
	expectedDependency, found := GovernanceDependencyForAggregate(aggregate)
	if !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance candidate has no sealed external dependency source", common.ErrConflict)
	}
	dependency := candidate.Governance.ExternalDependency.Normalized()
	if !governancecheck.ExternalDependencyMatches(expectedDependency, dependency) {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance candidate dependency does not match the sealed source", common.ErrConflict)
	}
	// GovernanceCandidate is a reducer-authored projection of the exact persisted
	// dependency above, not an operations document. Running the operations-only
	// CandidateEvaluator here would incorrectly apply repair-closure/replay rules
	// to a governance interlude and could reject a legitimate owner suspension.
	// The candidate shape and dependency have already crossed their two typed
	// gates; the shared dispatch seal still validates the resulting transaction.
	accepted := acceptedCandidateAdmission()
	dispatch, completeAdmission, err := admitRuntimeDispatch(aggregate, manifest, candidate, nil, &accepted, nil)
	if !completeAdmission.Admitted() {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance dispatch admission %s: %s", common.ErrConflict, completeAdmission.Code, completeAdmission.Message)
	}
	if err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.ActiveDispatch = &dispatch
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// admitRuntimeDispatch is the only production dispatcher constructor. Provider
// candidates and reducer-authored governance facts have different source
// ledgers, but both cross the same manifest, active-dispatch and sealed-builder
// fence. It is pure and owner-local; callers commit the returned value only
// through the corresponding private internal event and one aggregate CAS.
func admitRuntimeDispatch(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate, attempt *runtimecontract.RuntimeCandidateAttemptRecord, evaluated *CandidateAdmissionResult, repairCommitment *runtimecontract.RuntimeDispatchRepairCommitment) (SealedDispatch, runtimecontract.RuntimeCompleteAdmissionDecision, error) {
	if aggregate.ActiveDispatch != nil {
		return SealedDispatch{}, runtimecontract.RuntimeCompleteAdmissionDecision{
			Disposition: runtimecontract.RuntimeCompleteAdmissionStaleConflict,
			Code:        "active_dispatch_conflict",
			Message:     "runtime authority already has an active dispatch",
		}, nil
	}
	admission := evaluated
	if admission == nil {
		value := EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate)
		admission = &value
	}
	if !admission.Accepted() {
		message := "runtime candidate rejected by manifest-bound admission"
		if admission.Failure != nil {
			message = admission.Failure.Reason
		}
		return SealedDispatch{}, runtimecontract.RuntimeCompleteAdmissionDecision{
			Disposition: runtimecontract.RuntimeCompleteAdmissionCorrectableOnce,
			Code:        "candidate_document_rejected",
			Message:     message,
		}, nil
	}
	var feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord
	if attempt != nil {
		complete := runtimecontract.EvaluateRuntimeCompleteAdmission(runtimecontract.RuntimeCompleteAdmissionInput{
			Facts: aggregate.Facts, Decision: aggregate.Decision, SemanticPlanning: aggregate.SemanticPlanning,
			Reconciliation: aggregate.Reconciliation, Manifest: manifest, Candidate: candidate, Attempt: *attempt,
			ActiveDispatch:      sealedDispatchRecord(aggregate.ActiveDispatch),
			CompletedDispatches: sealedDispatchesToRuntimeRecords(aggregate.CompletedDispatches),
			OperationReceipts:   aggregate.OperationReceipts,
		})
		if !complete.Admitted() {
			return SealedDispatch{}, complete, nil
		}
		feasibilities = attempt.Feasibilities
	}
	preSealAuthority := RuntimeDispatchPreSealAuthority{
		RepairCommitment: cloneRuntimeDispatchRepairCommitment(repairCommitment),
	}
	if candidate.Assessment != nil {
		source, sourceErr := runtimecontract.RuntimeAssessmentSourceProjection(aggregate.Facts, aggregate.Decision.Focus.ID, candidate.Assessment.CoverageWorkItemIDs)
		if sourceErr != nil {
			return SealedDispatch{}, runtimecontract.RuntimeCompleteAdmissionDecision{
				Disposition: runtimecontract.RuntimeCompleteAdmissionTerminalContractFailure,
				Code:        "assessment_source_invalid", Message: sourceErr.Error(),
			}, sourceErr
		}
		purpose, purposeErr := runtimecontract.ResolveRuntimeAssessmentPurposePreimage(
			aggregate.SemanticPlanning,
			aggregate.Decision.Focus.CoverageEvidenceBindings,
		)
		if purposeErr != nil {
			return SealedDispatch{}, runtimecontract.RuntimeCompleteAdmissionDecision{
				Disposition: runtimecontract.RuntimeCompleteAdmissionTerminalContractFailure,
				Code:        "assessment_purpose_invalid", Message: purposeErr.Error(),
			}, purposeErr
		}
		if manifest.AssessmentPurpose != purpose.Purpose || manifest.AssessmentPurposeManifestHash != purpose.SemanticManifestHash {
			err := fmt.Errorf("runtime assessment purpose does not match its pre-seal semantic manifest")
			return SealedDispatch{}, runtimecontract.RuntimeCompleteAdmissionDecision{
				Disposition: runtimecontract.RuntimeCompleteAdmissionTerminalContractFailure,
				Code:        "assessment_purpose_manifest_mismatch", Message: err.Error(),
			}, err
		}
		focus := aggregate.Decision.Focus.Normalized()
		focus.SliceID = source.SliceID
		focus.AssessmentScope = source.AssessmentScope
		focus.AssessmentID = source.AssessmentID
		focus.AssessmentText = source.AssessmentText
		value := runtimecontract.RuntimeAssessmentAuthorityInput{
			AuthorityInput:           aggregate.Decision.AuthoritySnapshot.AuthorityInput,
			Purpose:                  purpose.Purpose,
			SourceSliceID:            source.SliceID,
			SourceScope:              source.AssessmentScope,
			SourceID:                 source.AssessmentID,
			SourceText:               source.AssessmentText,
			Focus:                    focus,
			OperationAdmissibilities: runtimecontract.CloneRuntimeOperationAdmissibilities(aggregate.Facts.Resources.OperationAdmissibilities),
		}.Normalized()
		preSealAuthority.AssessmentAuthorityInput = &value
	}
	dispatch, err := SealDispatchWithManifestAndPreSealAuthority(
		aggregate.Decision,
		candidate,
		runtimecontract.RuntimeActivationForFacts(aggregate.Facts),
		manifest,
		feasibilities,
		aggregate.Facts.Resources.OperationAdmissibilities,
		aggregate.Facts.Resources.PrerequisiteObservations,
		preSealAuthority,
	)
	if err != nil {
		return SealedDispatch{}, runtimecontract.RuntimeCompleteAdmissionDecision{
			Disposition: runtimecontract.RuntimeCompleteAdmissionTerminalContractFailure,
			Code:        "dispatch_seal_invalid",
			Message:     err.Error(),
		}, err
	}
	return dispatch, runtimecontract.RuntimeCompleteAdmissionDecision{Disposition: runtimecontract.RuntimeCompleteAdmissionAdmitted, Code: "admitted"}, nil
}

// GovernanceDependencyForAggregate returns the only dependency a governed
// dispatch may bind. It accepts facts that were explicitly sealed by an owner
// or prerequisite, plus the provider configuration fact whose subject is the
// sealed provider ceiling. It never invents a generic decision/focus fallback.
func GovernanceDependencyForAggregate(aggregate RuntimeAggregate) (runtimecontract.ExternalDependency, bool) {
	return governancecheck.DependencyForFacts(aggregate.Facts, aggregate.OperationGovernanceBindings)
}

func sealedDispatchRecord(value *SealedDispatch) *runtimecontract.RuntimeDispatchRecord {
	if value == nil {
		return nil
	}
	record := sealedDispatchToRuntimeRecord(*value)
	return &record
}

// runtimeOperationAdmissibilityRequiredForOperation remains a package-local
// fixture seam. Production complete admission owns the rule in the pure
// runtime contract; do not add an Orchestrator-local variant here.
func runtimeOperationAdmissibilityRequiredForOperation(prerequisites runtimecontract.PrerequisiteState, ownerModuleID, operationKind string) bool {
	return runtimecontract.RuntimeOperationAdmissibilityRequiredForOperation(prerequisites, ownerModuleID, operationKind)
}

func runtimeOperationAdmissibilityForKind(values []runtimecontract.RuntimeOperationAdmissibility, owner, kind string) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	canonicalKind := runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, inventory := range values {
		inventory = inventory.Normalized()
		if inventory.OwnerModuleID != owner {
			continue
		}
		for _, available := range inventory.OperationKinds {
			if available == canonicalKind {
				return inventory, true
			}
		}
	}
	return runtimecontract.RuntimeOperationAdmissibility{}, false
}

func candidateAttemptFeasibilityForOperation(values []runtimecontract.RuntimeCandidateFeasibilityRecord, operationID string) (runtimecontract.RuntimeCandidateFeasibilityRecord, bool) {
	for _, value := range values {
		if strings.TrimSpace(value.OperationID) == strings.TrimSpace(operationID) {
			return value, true
		}
	}
	return runtimecontract.RuntimeCandidateFeasibilityRecord{}, false
}

func sameStringSet(left, right []string) bool {
	left = normalizeStringSet(left)
	right = normalizeStringSet(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func normalizeStringSet(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	ret := make([]string, 0, len(set))
	for value := range set {
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}

func (c *candidateDispatchComponent) failCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, failure runtimecontract.RuntimeCandidateFailure) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || failure.Class == "" || strings.TrimSpace(failure.Reason) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, _, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !ok {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	expectedRevision := aggregate.StateRevision
	if err := closeCandidateAttempt(&aggregate, idx, attempt, failure); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// closeCandidateAttempt applies the only failed exit from a pending SM-3
// candidate attempt. Its caller owns the aggregate CAS boundary.
func closeCandidateAttempt(aggregate *RuntimeAggregate, idx, attempt int, failure runtimecontract.RuntimeCandidateFailure) error {
	return closeCandidateAttemptWithCorrection(aggregate, idx, attempt, failure, nil)
}

func closeCandidateAttemptWithCorrection(aggregate *RuntimeAggregate, idx, attempt int, failure runtimecontract.RuntimeCandidateFailure, correction *runtimecontract.RuntimeCandidateCorrectionDirective) error {
	if aggregate == nil || idx < 0 || idx >= len(aggregate.CandidateAttempts) || failure.Class == "" || strings.TrimSpace(failure.Reason) == "" {
		return common.ErrInvalidArgument
	}
	if err := retainFailedCandidateFeasibilityCatalogs(aggregate, idx); err != nil {
		return err
	}
	failure, correction = normalizeCandidateAttemptFailure(*aggregate, idx, attempt, failure, correction)
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateFailed
	failureCopy := failure
	aggregate.CandidateAttempts[idx].Failure = &failureCopy
	aggregate.CandidateAttempts[idx].Correction = correction.Clone()
	fact := candidateAttemptTerminalFailureFact(attempt, failure, correction)
	if fact == nil {
		return nil
	}
	if err := replaceRuntimeFailureAuthority(&aggregate.Facts, *fact, runtimeFailureAuthorityRetire, nil); err != nil {
		return err
	}
	facts, decision, err := reduceRuntimeAggregateWithDirective(*aggregate, aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	return nil
}

func normalizeCandidateAttemptFailure(aggregate RuntimeAggregate, idx, attempt int, failure runtimecontract.RuntimeCandidateFailure, correction *runtimecontract.RuntimeCandidateCorrectionDirective) (runtimecontract.RuntimeCandidateFailure, *runtimecontract.RuntimeCandidateCorrectionDirective) {
	if failure.ValidationDiagnostic != nil {
		diagnostic := failure.ValidationDiagnostic.Clone()
		failure.ValidationDiagnostic = &diagnostic
	}
	if failure.Diagnostic != nil {
		diagnostic := failure.Diagnostic.Normalized()
		if diagnostic.ManifestHash == "" {
			diagnostic.ManifestHash = aggregate.CandidateAttempts[idx].ManifestHash
		}
		failure.Diagnostic = &diagnostic
	}
	// Every complete rejection is persisted together with one total correction
	// disposition. Bare Class/Reason fixtures are normalized to a matrix code,
	// but cannot become correctable without a manifest-bound witness.
	if strings.TrimSpace(failure.Code) == "" {
		failure.Code = defaultCandidateFailureCode(failure.Class)
	}
	if strings.TrimSpace(failure.Signature) == "" {
		failure.Signature = failure.Code
	}
	if correction == nil {
		manifest, manifestErr := BuildRuntimeCandidateContractManifestForAggregate(aggregate)
		if manifestErr == nil && manifest.Hash == aggregate.CandidateAttempts[idx].ManifestHash {
			frontier := candidateCorrectionFrontier{}
			if aggregate.CandidateAttempts[idx].Proposal != nil {
				input := candidateAdmissionEvaluationInput(aggregate, manifest, *aggregate.CandidateAttempts[idx].Proposal)
				frontier = candidateCorrectionFrontierForEvaluation(input)
			}
			if sealed, directive, sealErr := sealCandidateRejectionWithFrontier(&failure, manifest, frontier); sealErr == nil && sealed != nil && directive != nil {
				failure = *sealed
				correction = directive
			}
		}
	} else {
		correction = correction.Clone()
	}
	if correction == nil || correction.ManifestHash != aggregate.CandidateAttempts[idx].ManifestHash || correction.Validate() != nil ||
		runtimecontract.ValidateRuntimeCandidateFailureCorrectionPair(&failure, correction) != nil {
		failure = *candidateRejectionContractDefect(failure, "candidate rejection has no valid manifest-bound correction disposition")
		correction = rejectedFinalDirective(aggregate.CandidateAttempts[idx].ManifestHash, "runtime_contract_defect")
	}
	if attempt == 1 && correction.Disposition == runtimecontract.RuntimeCandidateCorrectionCorrectableOnce {
		correction = rejectedFinalDirective(aggregate.CandidateAttempts[idx].ManifestHash, "candidate_retry_exhausted")
	}
	return failure, correction
}

func defaultCandidateFailureCode(class runtimecontract.RuntimeCandidateFailureClass) string {
	switch class {
	case runtimecontract.RuntimeCandidateFailureClassSchema:
		return "candidate_schema_invalid"
	case runtimecontract.RuntimeCandidateFailureClassContract:
		return "candidate_contract_invalid"
	case runtimecontract.RuntimeCandidateFailureClassInternal:
		return "runtime_contract_defect"
	default:
		return strings.TrimSpace(string(class))
	}
}

func candidateAttemptTerminalFailureFact(attempt int, failure runtimecontract.RuntimeCandidateFailure, correction *runtimecontract.RuntimeCandidateCorrectionDirective) *runtimecontract.FailureFact {
	if failure.Class == runtimecontract.RuntimeCandidateFailureClassInternal {
		return &runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Signature: "candidate-internal", Message: failure.Reason}
	}
	if attempt == 1 {
		return &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassCandidateExhausted, Code: "candidate_retry_exhausted",
			Signature: "candidate-retry-exhausted", Message: failure.Reason,
		}
	}
	if attempt == 0 && correction != nil && correction.Disposition == runtimecontract.RuntimeCandidateCorrectionRejectedFinal {
		// N6: explicit rejected_final attempt 0 seals terminal in the same CAS.
		// Attempt 0 may still have one sealed correction.  A validation candidate
		// infeasibility never fabricates state-change repair authority; exhaustion
		// is terminalized by the normal candidate pipeline.
		code := strings.TrimSpace(correction.TerminalCode)
		if code == "" {
			code = "runtime_contract_defect"
		}
		message := strings.TrimSpace(failure.Reason)
		if message == "" {
			message = "complete candidate rejection is rejected_final under the sealed manifest"
		}
		class := runtimecontract.FailureClassRuntimeContractMisalignment
		if code == "candidate_retry_exhausted" {
			class = runtimecontract.FailureClassCandidateExhausted
		} else if code == "candidate_authority_exhausted" {
			class = runtimecontract.FailureClassCandidateAuthorityExhausted
		}
		return &runtimecontract.FailureFact{
			Class: class, Code: code, Signature: code, Message: message,
		}
	}
	return nil
}

// closePendingCandidateAttemptsForTerminal closes any in-flight SM-3 request
// when an SM-2/SM-4 failure fact has already sealed terminal authority. This is
// part of the same aggregate CAS: a terminal Run must never persist an open
// provider transaction that a restart could replay. Retained feasibility facts
// keep their exact owner catalog preimages just like the candidate-failed exit.
func closePendingCandidateAttemptsForTerminal(aggregate *RuntimeAggregate, fact runtimecontract.FailureFact) error {
	if aggregate == nil || (!isTerminalConvergenceState(aggregate.Decision.State) && aggregate.Decision.Terminal == nil) {
		return nil
	}
	failure := runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassInternal, Code: strings.TrimSpace(fact.Code),
		Signature: strings.TrimSpace(fact.Signature), Reason: strings.TrimSpace(fact.Message),
	}
	if failure.Code == "" {
		failure.Code = "runtime_terminal_failure"
	}
	if failure.Signature == "" {
		failure.Signature = failure.Code
	}
	for idx := range aggregate.CandidateAttempts {
		if aggregate.CandidateAttempts[idx].State != runtimecontract.RuntimeCandidateAttemptStatePending {
			continue
		}
		if err := retainFailedCandidateFeasibilityCatalogs(aggregate, idx); err != nil {
			return err
		}
		failureCopy := failure
		aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateFailed
		aggregate.CandidateAttempts[idx].Failure = &failureCopy
		terminalCode := strings.TrimSpace(fact.Code)
		if terminalCode == "" {
			terminalCode = "runtime_terminal_failure"
		}
		aggregate.CandidateAttempts[idx].Correction = rejectedFinalDirective(aggregate.CandidateAttempts[idx].ManifestHash, terminalCode)
	}
	return nil
}

func candidateAttemptsForContract(values []runtimecontract.RuntimeCandidateAttemptRecord, workspaceID common.WorkspaceID, runID common.RunID, contractID string) []runtimecontract.RuntimeCandidateAttemptRecord {
	ret := make([]runtimecontract.RuntimeCandidateAttemptRecord, 0, 2)
	for _, value := range values {
		if runtimecontract.CandidateAttemptAuthorityContractID(workspaceID, runID, value) == contractID {
			ret = append(ret, value)
		}
	}
	return ret
}

func candidateAttemptRecord(values []runtimecontract.RuntimeCandidateAttemptRecord, attempt int) (runtimecontract.RuntimeCandidateAttemptRecord, bool) {
	for _, value := range values {
		if value.Attempt == attempt {
			return value, true
		}
	}
	return runtimecontract.RuntimeCandidateAttemptRecord{}, false
}

func pendingCandidateAttempt(values []runtimecontract.RuntimeCandidateAttemptRecord, workspaceID common.WorkspaceID, runID common.RunID, contractID string, attempt int) (int, runtimecontract.RuntimeCandidateAttemptRecord, bool) {
	for idx, value := range values {
		if runtimecontract.CandidateAttemptAuthorityContractID(workspaceID, runID, value) == contractID && value.Attempt == attempt && value.State == runtimecontract.RuntimeCandidateAttemptStatePending {
			return idx, value, true
		}
	}
	return -1, runtimecontract.RuntimeCandidateAttemptRecord{}, false
}

// duplicateRejectedCandidateAdmission identifies a completed expected rejection
// by its durable candidate/failure identity. It deliberately performs no write:
// a duplicated provider delivery must observe the original SM-3 result, not
// consume another attempt or reconstruct authority from the current decision.
func duplicateRejectedCandidateAdmission(aggregate RuntimeAggregate, attempt int, manifestHash, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) (CandidateAdmissionResult, bool) {
	candidateHash := runtimecontract.StableRuntimeCandidateHash(candidate)
	candidateShape := runtimecontract.StableRuntimeCandidateSemanticShape(candidate)
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt || record.ManifestHash != manifestHash || record.LogicalRequestID != logicalRequestID || record.State != runtimecontract.RuntimeCandidateAttemptStateFailed || record.Failure == nil {
			continue
		}
		if record.Proposal != nil {
			if runtimecontract.StableRuntimeCandidateHash(*record.Proposal) != candidateHash &&
				!sameRejectedCandidateSemanticShape(record.Failure, candidateShape) {
				continue
			}
		} else if record.Failure.Code != candidateAdmissionCodeInvalid || candidate.ValidateWithActivation(aggregate.Decision, runtimecontract.RuntimeActivationForFacts(aggregate.Facts)) == nil || record.Failure.Signature != candidateAdmissionInvalidSignature(candidate) {
			continue
		}
		failure := *record.Failure
		var correction *runtimecontract.RuntimeCandidateCorrectionDirective
		if record.Correction != nil {
			correction = record.Correction.Clone()
		}
		return CandidateAdmissionResult{Disposition: CandidateAdmissionRejectedContract, Failure: &failure, Correction: correction}, true
	}
	return CandidateAdmissionResult{}, false
}

// sameRejectedCandidateSemanticShape recognizes an equivalent complete provider
// replay after SM-3 has already recorded its rejection. Provider operation IDs
// and JSON field order are wire details, whereas RejectedShapeHash retains the
// persisted semantic identity. The caller has already fenced this comparison by
// attempt, manifest, and logical request identity, so it cannot merge distinct
// authority transactions.
func sameRejectedCandidateSemanticShape(failure *runtimecontract.RuntimeCandidateFailure, candidateShape string) bool {
	if failure == nil || failure.Diagnostic == nil || strings.TrimSpace(candidateShape) == "" {
		return false
	}
	return strings.TrimSpace(failure.Diagnostic.RejectedShapeHash) == candidateShape
}

// acceptedCandidateAdmissionDuplicate proves that an already sealed candidate
// is the same delivery. The dispatch candidate hash is the durable seal fence.
func acceptedCandidateAdmissionDuplicate(aggregate RuntimeAggregate, attempt int, manifestHash, logicalRequestID string, candidate runtimecontract.RuntimeCandidate) bool {
	var active *runtimecontract.RuntimeDispatchRecord
	if aggregate.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*aggregate.ActiveDispatch)
		active = &value
	}
	if err := runtimecontract.ValidateRuntimeAcceptedCandidateManifestLineage(
		active,
		sealedDispatchesToRuntimeRecords(aggregate.CompletedDispatches),
		aggregate.CandidateAttempts,
		aggregate.CandidateRequestSessions,
	); err != nil {
		return false
	}
	wanted := runtimecontract.StableRuntimeCandidateHash(candidate)
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt || record.ManifestHash != manifestHash || record.LogicalRequestID != logicalRequestID || record.State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
			continue
		}
		if aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.Transaction == record.Transaction &&
			record.CandidateHash == wanted && aggregate.ActiveDispatch.ID == record.DispatchID &&
			runtimecontract.RuntimeAcceptedCandidateAttemptMatchesDispatch(record, sealedDispatchToRuntimeRecord(*aggregate.ActiveDispatch)) {
			return true
		}
		for _, dispatch := range aggregate.CompletedDispatches {
			if dispatch.Transaction == record.Transaction && record.CandidateHash == wanted && dispatch.ID == record.DispatchID &&
				runtimecontract.RuntimeAcceptedCandidateAttemptMatchesDispatch(record, sealedDispatchToRuntimeRecord(dispatch)) {
				return true
			}
		}
	}
	return false
}
