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
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(stablePrefixHash) == "" || strings.TrimSpace(logicalRequestID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	switch aggregate.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment:
	default:
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not request a candidate", common.ErrConflict)
	}
	if err := validateCurrentCWIBindingFence(aggregate.Facts); err != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate request binding fence: %v", common.ErrConflict, err)
	}
	if err := validateCandidateAttemptSessionBinding(aggregate, stablePrefixHash, manifest, logicalRequestID); err != nil {
		return RuntimeAggregate{}, err
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, transaction)
	current := candidateAttemptsForContract(aggregate.CandidateAttempts, workspaceID, runID, contractID)
	if attempt == 0 && len(current) == 0 && equivalentOwnerInfeasibilityExists(aggregate.CandidateAttempts, transaction.AuthorityInputID) {
		return RuntimeAggregate{}, fmt.Errorf("%w: equivalent authority already produced owner infeasibility", common.ErrConflict)
	}
	for _, record := range current {
		if record.Attempt != attempt {
			continue
		}
		if record.State == runtimecontract.RuntimeCandidateAttemptStatePending && record.StablePrefixHash == stablePrefixHash && record.Transaction == transaction && record.ManifestHash == manifest.Hash && record.LogicalRequestID == logicalRequestID {
			return aggregate, nil
		}
		// Freshness invalidation does not consume the sealed attempt budget. If
		// prerequisite re-acquisition converges to the same stable authority
		// contract, recycle the persisted attempt 0 into a new pending proposal.
		// The separate prerequisite invalidation ledger retains the stale audit.
		if attempt == 0 && record.State == runtimecontract.RuntimeCandidateAttemptStateInvalidated && record.StablePrefixHash == stablePrefixHash {
			expectedRevision := aggregate.StateRevision
			for idx := range aggregate.CandidateAttempts {
				candidateAttempt := &aggregate.CandidateAttempts[idx]
				if runtimecontract.CandidateAttemptAuthorityContractID(workspaceID, runID, *candidateAttempt) != contractID || candidateAttempt.Attempt != attempt {
					continue
				}
				candidateAttempt.Transaction = transaction
				candidateAttempt.StablePrefixHash = stablePrefixHash
				candidateAttempt.ManifestHash = manifest.Hash
				candidateAttempt.LogicalRequestID = logicalRequestID
				candidateAttempt.State = runtimecontract.RuntimeCandidateAttemptStatePending
				candidateAttempt.Proposal = nil
				candidateAttempt.Feasibilities = nil
				candidateAttempt.Failure = nil
				candidateAttempt.Stale = nil
				aggregate.StateRevision++
				if err := aggregate.Validate(); err != nil {
					return RuntimeAggregate{}, err
				}
				if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
					return RuntimeAggregate{}, err
				}
				return aggregate, nil
			}
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is already %s", common.ErrConflict, attempt, record.State)
	}
	if attempt == 0 {
		if len(current) != 0 {
			return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt 0 is no longer available", common.ErrConflict)
		}
	} else {
		first, ok := candidateAttemptRecord(current, 0)
		if !ok || first.State != runtimecontract.RuntimeCandidateAttemptStateFailed || first.StablePrefixHash != stablePrefixHash || first.ManifestHash != manifest.Hash || !runtimecontract.SameAuthorityContract(first.Transaction, transaction) {
			return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt 1 requires a failed same-contract attempt 0", common.ErrConflict)
		}
		// N6: the attempt-1 CAS entry itself is the correction budget fence.
		// rejected_final / non-correctable attempt 0 must not open a second
		// provider session even when callers bypass mainline routing.
		if first.Failure == nil || !CandidateFailureIsCorrectableOnce(first.Failure) {
			return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt 1 requires a sealed correctable_once attempt 0 failure", common.ErrConflict)
		}
	}
	expectedRevision := aggregate.StateRevision
	aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{
		AuthorityContractID: contractID, Transaction: transaction, Attempt: attempt, StablePrefixHash: stablePrefixHash, ManifestHash: manifest.Hash, LogicalRequestID: logicalRequestID, State: runtimecontract.RuntimeCandidateAttemptStatePending,
	})
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func equivalentOwnerInfeasibilityExists(attempts []runtimecontract.RuntimeCandidateAttemptRecord, authorityInputID string) bool {
	authorityInputID = strings.TrimSpace(authorityInputID)
	if authorityInputID == "" {
		return false
	}
	for _, attempt := range attempts {
		if attempt.State == runtimecontract.RuntimeCandidateAttemptStateInvalidated && attempt.OwnerFeasibility != nil &&
			attempt.Transaction.AuthorityInputID == authorityInputID {
			return true
		}
	}
	return false
}

// rejectCompletedCandidateDocument is the single SM-3 transition for a
// complete document that cannot become an executable candidate. It creates
// the attempt record and closes it in one aggregate CAS, so restart never
// observes a newly allocated but semantically meaningless pending attempt.
func (c *candidateDispatchComponent) rejectCompletedCandidateDocument(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, failure runtimecontract.RuntimeCandidateFailure) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(stablePrefixHash) == "" || strings.TrimSpace(logicalRequestID) == "" || failure.Class == "" || strings.TrimSpace(failure.Reason) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, transaction)
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt || record.ManifestHash != manifest.Hash || record.LogicalRequestID != logicalRequestID {
			continue
		}
		if record.State == runtimecontract.RuntimeCandidateAttemptStateFailed {
			return aggregate, nil
		}
		if runtimecontract.CandidateAttemptAuthorityContractID(workspaceID, runID, record) != contractID || record.State != runtimecontract.RuntimeCandidateAttemptStatePending {
			return RuntimeAggregate{}, fmt.Errorf("%w: completed candidate document attempt %d is already %s", common.ErrConflict, attempt, record.State)
		}
	}
	if err := validateCandidateAttemptSessionBinding(aggregate, stablePrefixHash, manifest, logicalRequestID); err != nil {
		return RuntimeAggregate{}, err
	}
	switch aggregate.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment:
	default:
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not request a candidate", common.ErrConflict)
	}
	current := candidateAttemptsForContract(aggregate.CandidateAttempts, workspaceID, runID, contractID)
	idx, _, pending := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !pending {
		if attempt == 0 {
			if len(current) != 0 {
				return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt 0 is no longer available", common.ErrConflict)
			}
		} else {
			first, ok := candidateAttemptRecord(current, 0)
			if !ok || first.State != runtimecontract.RuntimeCandidateAttemptStateFailed || first.StablePrefixHash != stablePrefixHash || first.ManifestHash != manifest.Hash || !runtimecontract.SameAuthorityContract(first.Transaction, transaction) || first.Failure == nil || !CandidateFailureIsCorrectableOnce(first.Failure) {
				return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt 1 requires a failed correctable same-contract attempt 0", common.ErrConflict)
			}
		}
		aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{
			AuthorityContractID: contractID, Transaction: transaction, Attempt: attempt, StablePrefixHash: stablePrefixHash,
			ManifestHash: manifest.Hash, LogicalRequestID: logicalRequestID, State: runtimecontract.RuntimeCandidateAttemptStatePending,
		})
		idx = len(aggregate.CandidateAttempts) - 1
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
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, record, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !ok {
		if duplicate, found := duplicateRejectedCandidateAdmission(aggregate, attempt, manifest.Hash, logicalRequestID, candidate); found {
			return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: duplicate}, nil
		}
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
	admission := EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate)
	if !admission.Accepted() {
		expectedRevision := aggregate.StateRevision
		// A structurally valid rejected proposal is useful durable diagnostics and
		// allows the next attempt to explain exactly what was rejected. Invalid
		// candidates cannot be persisted because state validation must remain
		// schema-safe even after a failed attempt.
		if candidate.ValidateWithActivation(aggregate.Decision, aggregate.Facts.Activation) == nil {
			proposal := candidate
			aggregate.CandidateAttempts[idx].Proposal = &proposal
		}
		if err := closeCandidateAttempt(&aggregate, idx, attempt, *admission.Failure); err != nil {
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
	if record.Proposal != nil {
		if runtimecontract.StableRuntimeCandidateHash(*record.Proposal) != runtimecontract.StableRuntimeCandidateHash(candidate) {
			return CandidateAdmissionCommitResult{}, fmt.Errorf("%w: pending candidate attempt proposal changed", common.ErrConflict)
		}
		return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
	}
	expectedRevision := aggregate.StateRevision
	proposal := candidate
	aggregate.CandidateAttempts[idx].Proposal = &proposal
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return CandidateAdmissionCommitResult{}, err
	}
	return CandidateAdmissionCommitResult{Aggregate: aggregate, Admission: admission}, nil
}

// recordCandidateFeasibility persists one completed, side-effect-free owner
// probe. The operation must belong to the already persisted proposal, making
// this ledger a restart fence rather than a second candidate authority.
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
		return RuntimeAggregate{}, err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, attemptRecord, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !ok || attemptRecord.Proposal == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d has no pending proposal", common.ErrConflict, attempt)
	}
	if !candidateProposalContainsOperation(*attemptRecord.Proposal, record.OperationID) {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate feasibility operation %q is absent from the persisted proposal", common.ErrConflict, record.OperationID)
	}
	if record.CandidateManifestHash != attemptRecord.ManifestHash {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate feasibility operation %q does not match the pending manifest", common.ErrConflict, record.OperationID)
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
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
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
	admission := EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate)
	if !admission.Accepted() {
		expectedRevision := aggregate.StateRevision
		// Final admission may be reached directly after a restart or alternate
		// owner probe. Preserve a schema-valid rejected candidate here too, so
		// duplicate delivery has the same durable identity as proposal admission.
		if candidate.ValidateWithActivation(aggregate.Decision, aggregate.Facts.Activation) == nil {
			proposal := candidate
			aggregate.CandidateAttempts[idx].Proposal = &proposal
		}
		if err := closeCandidateAttempt(&aggregate, idx, attempt, *admission.Failure); err != nil {
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
	dispatch, completeAdmission, err := admitRuntimeDispatch(aggregate, manifest, candidate, &record, &admission)
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
	expectedRevision := aggregate.StateRevision
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateAccepted
	aggregate.CandidateAttempts[idx].Proposal = nil
	aggregate.CandidateAttempts[idx].Feasibilities = nil
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
	if candidate.Governance == nil || candidate.ValidateWithActivation(aggregate.Decision, aggregate.Facts.Activation) != nil {
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
	dispatch, completeAdmission, err := admitRuntimeDispatch(aggregate, manifest, candidate, nil, nil)
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
func admitRuntimeDispatch(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate, attempt *runtimecontract.RuntimeCandidateAttemptRecord, evaluated *CandidateAdmissionResult) (SealedDispatch, runtimecontract.RuntimeCompleteAdmissionDecision, error) {
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
	dispatch, err := SealDispatchWithManifestAndFeasibilities(aggregate.Decision, candidate, aggregate.Facts.Activation, manifest, feasibilities)
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
	if aggregate == nil || idx < 0 || idx >= len(aggregate.CandidateAttempts) || failure.Class == "" || strings.TrimSpace(failure.Reason) == "" {
		return common.ErrInvalidArgument
	}
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
	// N4: every complete rejection is sealed as correctable_once or rejected_final
	// before it is persisted on the attempt record. Bare Class/Reason fixtures are
	// normalized to a matrix code first so attempt 0 schema/contract failures stay
	// correctable instead of becoming an internal terminal defect.
	if strings.TrimSpace(failure.Code) == "" {
		switch failure.Class {
		case runtimecontract.RuntimeCandidateFailureClassSchema:
			failure.Code = "candidate_schema_invalid"
		case runtimecontract.RuntimeCandidateFailureClassContract:
			failure.Code = "candidate_contract_invalid"
		case runtimecontract.RuntimeCandidateFailureClassInternal:
			failure.Code = "runtime_contract_defect"
		default:
			// provider/transient classes remain non-correctable finals below.
			failure.Code = strings.TrimSpace(string(failure.Class))
		}
	}
	if strings.TrimSpace(failure.Signature) == "" {
		failure.Signature = failure.Code
	}
	manifest := runtimecontract.RuntimeCandidateContractManifest{Hash: aggregate.CandidateAttempts[idx].ManifestHash}
	originalCode := strings.TrimSpace(failure.Code)
	originalClass := failure.Class
	if sealed, directive, sealErr := SealCandidateRejection(&failure, manifest); sealErr == nil && sealed != nil {
		// Attempt 0 must keep the first-failure code stable for diagnostics and
		// attempt-1 gating. Seal may rewrite an incomplete correctable into
		// runtime_contract_defect; that rewrite is only forced onto the attempt
		// record for attempt 1 / already-final codes. Attempt 0 still persists
		// objectives (or the lack of them) so IsCorrectableOnce can refuse retry.
		if attempt == 1 || sealed.Code == originalCode || originalCode == "runtime_contract_defect" ||
			CandidateFailureIsRejectedFinal(&runtimecontract.RuntimeCandidateFailure{Code: originalCode}) {
			failure = *sealed
		} else {
			failure.Diagnostic = sealed.Diagnostic
			if failure.Diagnostic == nil && sealed.Diagnostic != nil {
				failure.Diagnostic = sealed.Diagnostic.Clone()
			}
			// Preserve original class/code; empty objectives make attempt 1 refuse.
			_ = directive
			_ = originalClass
		}
	}
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateFailed
	failureCopy := failure
	aggregate.CandidateAttempts[idx].Failure = &failureCopy
	needsReduce := false
	if failure.Class == runtimecontract.RuntimeCandidateFailureClassInternal {
		aggregate.Facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Signature: "candidate-internal", Message: failure.Reason}
		needsReduce = true
	} else if attempt == 1 {
		aggregate.Facts.Failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassCandidateExhausted, Code: "candidate_retry_exhausted",
			Signature: "candidate-retry-exhausted", Message: failure.Reason,
		}
		needsReduce = true
	} else if attempt == 0 && CandidateFailureIsRejectedFinal(&failure) {
		// N6: explicit rejected_final attempt 0 seals terminal in the same CAS.
		// Attempt 0 may still have one sealed correction.  A validation candidate
		// infeasibility never fabricates state-change repair authority; exhaustion
		// is terminalized by the normal candidate pipeline.
		code := CandidateRejectionTerminalCode(failure.Code)
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
		}
		aggregate.Facts.Failure = &runtimecontract.FailureFact{
			Class: class, Code: code, Signature: code, Message: message,
		}
		needsReduce = true
	}
	if needsReduce {
		facts, decision, reduceErr := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
		if reduceErr != nil {
			return reduceErr
		}
		aggregate.Facts = facts
		aggregate.Decision = decision
	}
	return nil
}

// closePendingCandidateAttemptsForTerminal closes any in-flight SM-3 request
// when an SM-2/SM-4 failure fact has already sealed terminal authority. This is
// part of the same aggregate CAS: a terminal Run must never persist an open
// provider transaction that a restart could replay.
func closePendingCandidateAttemptsForTerminal(aggregate *RuntimeAggregate, fact runtimecontract.FailureFact) {
	if aggregate == nil || (!isTerminalConvergenceState(aggregate.Decision.State) && aggregate.Decision.Terminal == nil) {
		return
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
		failureCopy := failure
		aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateFailed
		aggregate.CandidateAttempts[idx].Failure = &failureCopy
	}
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
		} else if record.Failure.Code != candidateAdmissionCodeInvalid || candidate.ValidateWithActivation(aggregate.Decision, aggregate.Facts.Activation) == nil || record.Failure.Signature != candidateAdmissionInvalidSignature(candidate) {
			continue
		}
		failure := *record.Failure
		return CandidateAdmissionResult{Disposition: CandidateAdmissionRejectedContract, Failure: &failure}, true
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
	wanted := runtimecontract.StableRuntimeCandidateHash(candidate)
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt != attempt || record.ManifestHash != manifestHash || record.LogicalRequestID != logicalRequestID || record.State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
			continue
		}
		if aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.Transaction == record.Transaction && aggregate.ActiveDispatch.CandidateHash == wanted {
			return true
		}
		for _, dispatch := range aggregate.CompletedDispatches {
			if dispatch.Transaction == record.Transaction && dispatch.CandidateHash == wanted {
				return true
			}
		}
	}
	return false
}
