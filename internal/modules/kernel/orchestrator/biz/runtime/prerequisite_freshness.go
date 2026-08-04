package runtime

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// invalidatePrerequisitesForReadOnlyStale applies an owner-observed freshness
// change before any current-authority candidate work or dispatch exists. It is
// the SM-2 route for a prerequisite probe: the owner supplies only the typed
// stale identity, while the controller atomically retires every
// exactly-intersecting observation and records the read-only refresh ledger
// entry. Historical sessions/attempts from an already-invalidated dispatch are
// immutable audit lineage, not live SM-3 work; rejecting them would make a
// later local prerequisite stale unable to converge after an external receipt
// resource change.
func (c *RuntimeController) invalidatePrerequisitesForReadOnlyStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stale runtimecontract.RuntimePrerequisiteStale) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	stale = stale.Normalized()
	if err := stale.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	signature := runtimecontract.StableRuntimePrerequisiteStaleSignature(stale)
	if prerequisiteInvalidationExists(aggregate, signature) {
		return aggregate, nil
	}
	if aggregate.ActiveDispatch != nil || runtimeAggregateHasCurrentCandidateWork(aggregate) {
		return RuntimeAggregate{}, fmt.Errorf("%w: read-only prerequisite stale requires no current candidate or dispatch", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	facts, prerequisiteIDs, observationIDs, invalidatedDependencies, err := invalidatePrerequisiteObservationsForStale(aggregate.Facts, stale)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	ledger, consumed, err := consumeRecoveryBudget(facts.Recovery, runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	facts.Recovery = ledger
	staleCopy := stale
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature: signature, PrerequisiteIDs: prerequisiteIDs, ObservationIDs: observationIDs,
		InvalidatedDependencies: invalidatedDependencies, Stale: &staleCopy,
	})
	if !consumed {
		facts.Failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted, Code: "prerequisite_refresh_exhausted",
			Signature: signature, Message: "prerequisite refresh budget exhausted: " + stale.Reason,
		}
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// runtimeAggregateHasCurrentCandidateWork distinguishes active SM-3 work from
// immutable lineage that belongs to a prior authority snapshot.  Resource
// receipt invalidation intentionally keeps the accepted candidate/session that
// authorized the durable effect; a later SM-2 stale fact may proceed only when
// it cannot invalidate a current provider request or pending candidate.
func runtimeAggregateHasCurrentCandidateWork(aggregate RuntimeAggregate) bool {
	transaction := aggregate.Decision.AuthorityTransaction()
	if transaction.AuthoritySnapshotID == "" {
		return false
	}
	for _, session := range aggregate.CandidateRequestSessions {
		if session.Transaction == transaction && session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal {
			return true
		}
	}
	for _, attempt := range aggregate.CandidateAttempts {
		if attempt.Transaction != transaction {
			continue
		}
		switch attempt.State {
		case runtimecontract.RuntimeCandidateAttemptStatePending, runtimecontract.RuntimeCandidateAttemptStateAccepted:
			return true
		}
	}
	return false
}

// invalidateCandidateRequestSessionForPrerequisiteStale is the session-first
// stale route for a provider response or freshness observation received before
// C5 admission. A C4 document may already be complete, but it is not yet a
// candidate attempt and must never be decoded/admitted after the authority
// turns stale. The method deliberately has no candidate-attempt parameter:
// stale authority is an SM-2 prerequisite fact, not an SM-3 candidate
// rejection. The session is retained as terminal audit lineage while the
// reducer seals a new authority around the reacquired active prerequisite.
func (c *RuntimeController) invalidateCandidateRequestSessionForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, stale runtimecontract.RuntimePrerequisiteStale) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(logicalRequestID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	stale = stale.Normalized()
	if err := stale.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	signature := runtimecontract.StableRuntimePrerequisiteStaleSignature(stale)
	if prerequisiteInvalidationExists(aggregate, signature) {
		return aggregate, nil
	}
	idx, session, err := currentCandidateRequestSession(aggregate, logicalRequestID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if !session.ProviderSubmitted {
		return RuntimeAggregate{}, fmt.Errorf("%w: stale candidate response does not match a submitted physical call", common.ErrConflict)
	}

	expectedRevision := aggregate.StateRevision
	facts, affectedPrerequisites, affectedObservations, invalidatedDependencies, err := invalidatePrerequisiteObservationsForStale(aggregate.Facts, stale)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	ledger, consumed, err := consumeRecoveryBudget(facts.Recovery, runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	facts.Recovery = ledger
	staleCopy := stale
	aggregate.CandidateRequestSessions[idx].State = runtimecontract.RuntimeCandidateRequestSessionTerminal
	aggregate.CandidateRequestSessions[idx].ProviderSubmitted = false
	aggregate.CandidateRequestSessions[idx].LastOutcome = runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale,
		Code:  "prerequisite_stale", Signature: runtimecontract.StableRuntimePrerequisiteStaleSignature(stale),
	}
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature: signature, PrerequisiteIDs: affectedPrerequisites, ObservationIDs: affectedObservations,
		InvalidatedDependencies: invalidatedDependencies, Stale: &staleCopy,
	})
	if !consumed {
		facts.Failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted, Code: "prerequisite_refresh_exhausted",
			Signature: signature, Message: "prerequisite refresh budget exhausted: " + stale.Reason,
		}
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// invalidateCandidateAttemptForPrerequisiteStale abandons an unsealed
// candidate request/proposal after an owner read-only probe proves that one
// prerequisite observation is obsolete. It is deliberately not
// failCandidateAttempt: no candidate retry, repair debt, provider retry, or
// evidence is created.
func (c *RuntimeController) invalidateCandidateAttemptForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stale runtimecontract.RuntimePrerequisiteStale) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	stale = stale.Normalized()
	if err := stale.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	signature := runtimecontract.StableRuntimePrerequisiteStaleSignature(stale)
	if prerequisiteInvalidationExists(aggregate, signature) {
		return aggregate, nil
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, _, found := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	expectedRevision := aggregate.StateRevision
	facts, affectedPrerequisites, affectedObservations, invalidatedDependencies, err := invalidatePrerequisiteObservationsForStale(aggregate.Facts, stale)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	ledger, consumed, err := consumeRecoveryBudget(facts.Recovery, runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	facts.Recovery = ledger
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateInvalidated
	aggregate.CandidateAttempts[idx].Proposal = nil
	aggregate.CandidateAttempts[idx].Feasibilities = nil
	aggregate.CandidateAttempts[idx].Failure = nil
	staleCopy := stale
	aggregate.CandidateAttempts[idx].Stale = &staleCopy
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature: signature, PrerequisiteIDs: affectedPrerequisites, ObservationIDs: affectedObservations,
		InvalidatedDependencies: invalidatedDependencies, Stale: &staleCopy,
	})
	if !consumed {
		facts.Failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted, Code: "prerequisite_refresh_exhausted",
			Signature: signature, Message: "prerequisite refresh budget exhausted: " + stale.Reason,
		}
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// invalidateCandidateAttemptForFeasibilityPrerequisite abandons an unsealed
// candidate when its owning capability proves that correcting the operation
// requires a missing current owner fact. Unlike a failed candidate attempt it
// spends neither the 0/1 contract retry nor the focus repair budget: the next
// candidate runs only after a new authority is sealed around the acquired fact.
func (c *RuntimeController) invalidateCandidateAttemptForFeasibilityPrerequisite(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, request runtimecontract.RuntimeOperationPrerequisiteRequest) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	request = request.Normalized()
	if err := request.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	// Duplicate owner delivery may arrive after the CAS committed a new
	// prerequisite authority. The persisted invalidated attempt is the exact
	// idempotency fence; do not reconstruct or consume any budget.
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt == attempt && record.State == runtimecontract.RuntimeCandidateAttemptStateInvalidated && record.PrerequisiteRequest != nil && record.PrerequisiteRequest.Same(request) {
			return aggregate, nil
		}
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, record, found := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !found || record.Proposal == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	operation, found := candidateOperationByID(*record.Proposal, request.OperationID)
	if !found || !runtimecontract.RuntimeOperationRequiresCurrentTargetSnapshot(operation) || !runtimeOperationTargetsRef(operation, request.SubjectRef) {
		return RuntimeAggregate{}, fmt.Errorf("%w: prerequisite request is not bound to the pending candidate operation", common.ErrConflict)
	}
	descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !known || descriptor.OwnerModuleID != request.OwnerModuleID {
		return RuntimeAggregate{}, fmt.Errorf("%w: prerequisite request owner does not match candidate operation", common.ErrConflict)
	}
	feasibility, found := candidateAttemptFeasibilityForOperation(record.Feasibilities, request.OperationID)
	if !found || feasibility.Observation == nil || feasibility.Observation.PrerequisiteRequest == nil || !feasibility.Observation.PrerequisiteRequest.Same(request) {
		return RuntimeAggregate{}, fmt.Errorf("%w: prerequisite request has no matching persisted owner feasibility fact", common.ErrConflict)
	}

	expectedRevision := aggregate.StateRevision
	facts := addFeasibilityPrerequisite(factsWithoutTargetSnapshot(aggregate.Facts, request), request)
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateInvalidated
	aggregate.CandidateAttempts[idx].Proposal = nil
	aggregate.CandidateAttempts[idx].Feasibilities = nil
	aggregate.CandidateAttempts[idx].Failure = nil
	aggregate.CandidateAttempts[idx].Stale = nil
	requestCopy := request
	aggregate.CandidateAttempts[idx].PrerequisiteRequest = &requestCopy
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// invalidateCandidateAttemptForOwnerInfeasibility is the SM-2/SM-3 handoff
// for a completed owner feasibility probe. Unlike failCandidateAttempt it
// never seals a CorrectionDisposition and never permits attempt 1. The
// controller preserves the factual observation as immutable lineage, then lets
// the owner reducer choose a fresh authority or a typed terminal. An
// owner-contract-defect is terminal at this boundary: it is evidence that the
// producing capability violated its own total emitted-code registry, not a
// recoverable resource observation.
func (c *RuntimeController) invalidateCandidateAttemptForOwnerInfeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, observation runtimecontract.RuntimeOwnerFeasibilityObservation) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	observation = observation.Normalized()
	if err := observation.Validate(); err != nil || (observation.Outcome != runtimecontract.RuntimeOwnerFeasibilityInfeasible && observation.Outcome != runtimecontract.RuntimeOwnerFeasibilityContractDefect) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt == attempt && record.State == runtimecontract.RuntimeCandidateAttemptStateInvalidated && record.OwnerFeasibility != nil && reflect.DeepEqual(record.OwnerFeasibility.Clone(), observation) {
			return aggregate, nil
		}
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, record, found := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !found || record.Proposal == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	matched := false
	for _, feasibility := range record.Feasibilities {
		if feasibility.Observation == nil {
			continue
		}
		if reflect.DeepEqual(feasibility.Observation.Clone(), observation) {
			matched = true
			break
		}
	}
	if !matched {
		return RuntimeAggregate{}, fmt.Errorf("%w: owner infeasibility has no matching persisted feasibility observation", common.ErrConflict)
	}

	expectedRevision := aggregate.StateRevision
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateInvalidated
	aggregate.CandidateAttempts[idx].Proposal = nil
	aggregate.CandidateAttempts[idx].Feasibilities = nil
	aggregate.CandidateAttempts[idx].Failure = nil
	aggregate.CandidateAttempts[idx].Stale = nil
	aggregate.CandidateAttempts[idx].PrerequisiteRequest = nil
	copyObservation := observation.Clone()
	aggregate.CandidateAttempts[idx].OwnerFeasibility = &copyObservation
	facts := aggregate.Facts
	diagnostic := observation.Feedback.OriginalDiagnostic
	failure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassCandidateInfeasible, Code: diagnostic.OriginalCode,
		Signature: diagnostic.Signature, Message: diagnostic.OriginalMessage, TargetRefs: append([]string(nil), diagnostic.TargetRefs...),
	}
	if observation.Outcome == runtimecontract.RuntimeOwnerFeasibilityContractDefect {
		failure = runtimecontract.FailureFact{
			Class:      runtimecontract.FailureClassRuntimeContractMisalignment,
			Code:       "runtime_owner_contract_defect",
			Signature:  "runtime-owner-contract-defect/" + diagnostic.Signature,
			Message:    "capability owner emitted an undeclared feasibility code",
			TargetRefs: append([]string(nil), diagnostic.TargetRefs...),
		}
	}
	// An owner that reports that the sealed validation recipe is unavailable
	// cannot be repaired by asking for another candidate under the same
	// validation authority. A valid alternative must arrive as a new sealed
	// prerequisite/blueprint fact; absent that fact the factual outcome is a
	// deterministic terminal, not a correction or random state change.
	if observation.FailureCategory() == runtimecontract.RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable {
		failure.Class = runtimecontract.FailureClassNoProgressExhausted
		failure.Code = "validation_recipe_exhausted"
		failure.Signature = "validation-recipe-exhausted/" + diagnostic.Signature
		failure.Message = "sealed validation recipe is unavailable: " + diagnostic.OriginalMessage
	}
	facts.Failure = &failure
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func candidateOperationByID(candidate runtimecontract.RuntimeCandidate, operationID string) (runtimecontract.RuntimeOperation, bool) {
	operationID = strings.TrimSpace(operationID)
	for _, operation := range candidateRuntimeOperations(candidate) {
		if operation.Normalized().ID == operationID {
			return operation.Normalized(), true
		}
	}
	return runtimecontract.RuntimeOperation{}, false
}

func runtimeOperationTargetsRef(operation runtimecontract.RuntimeOperation, targetRef string) bool {
	targetRef = strings.TrimSpace(targetRef)
	for _, value := range operation.Normalized().TargetRefs {
		if value == targetRef {
			return true
		}
	}
	return false
}

// factsWithoutTargetSnapshot clears exactly the owner resource that will be
// reacquired. It deliberately does not use stale-refresh bookkeeping: no
// previously accepted owner observation was disproved, and the repair/candidate
// contract had simply not requested this input before the feasibility proof.
func factsWithoutTargetSnapshot(facts runtimecontract.RuntimeFacts, request runtimecontract.RuntimeOperationPrerequisiteRequest) runtimecontract.RuntimeFacts {
	prerequisiteID := runtimePrerequisiteID(runtimecontract.RuntimeRequirement{
		Kind: request.Kind, OwnerModuleID: request.OwnerModuleID, SubjectRef: request.SubjectRef, MinimumBytes: request.MinimumBytes,
	})
	snapshots := make([]runtimecontract.TargetSnapshot, 0, len(facts.Resources.TargetSnapshots))
	for _, snapshot := range facts.Resources.TargetSnapshots {
		if strings.TrimSpace(snapshot.TargetRef) != request.SubjectRef {
			snapshots = append(snapshots, snapshot)
		}
	}
	facts.Resources.TargetSnapshots = snapshots
	satisfied := make([]runtimecontract.RuntimePrerequisite, 0, len(facts.Prerequisites.Satisfied))
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if prerequisite.ID != prerequisiteID {
			satisfied = append(satisfied, prerequisite)
		}
	}
	facts.Prerequisites.Satisfied = satisfied
	observations := make([]runtimecontract.RuntimePrerequisiteObservation, 0, len(facts.Resources.PrerequisiteObservations))
	for _, observation := range facts.Resources.PrerequisiteObservations {
		if observation.PrerequisiteID != prerequisiteID {
			observations = append(observations, observation)
		}
	}
	facts.Resources.PrerequisiteObservations = observations
	return facts.Normalized()
}

func addFeasibilityPrerequisite(facts runtimecontract.RuntimeFacts, request runtimecontract.RuntimeOperationPrerequisiteRequest) runtimecontract.RuntimeFacts {
	request = request.Normalized()
	requirement := runtimecontract.RuntimeRequirement{
		Kind: request.Kind, OwnerModuleID: request.OwnerModuleID, SubjectRef: request.SubjectRef,
		MinimumBytes: request.MinimumBytes, Reason: request.Reason,
	}
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: runtimePrerequisiteID(requirement), Kind: requirement.Kind, OwnerModuleID: requirement.OwnerModuleID,
		SubjectRef: requirement.SubjectRef, MinimumBytes: requirement.MinimumBytes, Reason: requirement.Reason,
	}
	for _, pending := range facts.Prerequisites.Pending {
		if pending.ID == prerequisite.ID {
			return facts.Normalized()
		}
	}
	facts.Prerequisites.Pending = append(facts.Prerequisites.Pending, prerequisite)
	return facts.Normalized()
}

// invalidateActiveDispatchForPrerequisiteStale closes a sealed dispatch after
// an owner has persisted a no-effect stale receipt. The stale receipt remains
// diagnostic lineage only; it cannot create rejected evidence or repair debt.
func (c *RuntimeController) invalidateActiveDispatchForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, receipt runtimecontract.RuntimeOperationReceipt) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(dispatchID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	receipt = receipt.Normalized()
	if err := receipt.Validate(); err != nil || receipt.PrerequisiteStale == nil {
		if err != nil {
			return RuntimeAggregate{}, err
		}
		return RuntimeAggregate{}, fmt.Errorf("active dispatch refresh requires a generic stale receipt")
	}
	stale := receipt.PrerequisiteStale.Normalized()
	signature := runtimecontract.StableRuntimePrerequisiteStaleSignature(stale)
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if prerequisiteInvalidationExists(aggregate, signature) {
		return aggregate, nil
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ID != strings.TrimSpace(dispatchID) {
		return RuntimeAggregate{}, fmt.Errorf("%w: stale receipt does not match active dispatch", common.ErrConflict)
	}
	if _, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, receipt.ExecutionKey, receipt.Fence); !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: stale receipt is not owner-persisted", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	facts, prerequisiteIDs, observationIDs, invalidatedDependencies, err := invalidatePrerequisiteObservationsForStale(aggregate.Facts, stale)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	ledger, consumed, err := consumeRecoveryBudget(facts.Recovery, runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	facts.Recovery = ledger
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature: signature, PrerequisiteIDs: prerequisiteIDs, ObservationIDs: observationIDs,
		InvalidatedDependencies: invalidatedDependencies,
		DispatchID:              dispatchID, OperationID: receipt.OperationID, Stale: &stale,
	})
	dispatch := *aggregate.ActiveDispatch
	dispatch.State = DispatchStateInvalidated
	aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
	aggregate.ActiveDispatch = nil
	if !consumed {
		facts.Failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted, Code: "prerequisite_refresh_exhausted",
			Signature: signature, Message: "prerequisite refresh budget exhausted: " + stale.Reason,
		}
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// applyReceiptResourceChangeInvalidation is called while committing accepted
// evidence. It keeps the accepted receipt/evidence and atomically retires any
// intersecting owner observation for the next SM-2 authority. It never makes
// the remaining operations of the current sealed SM-3 transaction stale:
// their identity, targets, dependencies and effect contract were sealed before
// the first owner side effect.
func applyReceiptResourceChangeInvalidation(aggregate *RuntimeAggregate, dispatchID, operationID string, receipt runtimecontract.RuntimeOperationReceipt) (bool, error) {
	if aggregate == nil {
		return false, common.ErrInvalidArgument
	}
	receipt = receipt.Normalized()
	if len(receipt.ResourceChanges) == 0 {
		return false, nil
	}
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	signature := runtimecontract.StableRuntimeResourceChangeInvalidationSignature(receiptHash, receipt.ResourceChanges)
	if prerequisiteInvalidationExists(*aggregate, signature) {
		return false, nil
	}
	facts, prerequisiteIDs, observationIDs, invalidatedDependencies, err := invalidatePrerequisiteObservationsForChanges(aggregate.Facts, receipt.ResourceChanges)
	if err != nil {
		return false, err
	}
	if len(prerequisiteIDs) == 0 {
		return false, nil
	}
	aggregate.Facts = facts
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature: signature, PrerequisiteIDs: prerequisiteIDs, ObservationIDs: observationIDs,
		InvalidatedDependencies: invalidatedDependencies,
		DispatchID:              strings.TrimSpace(dispatchID), OperationID: strings.TrimSpace(operationID), ReceiptHash: receiptHash,
		ResourceChanges: receipt.ResourceChanges,
	})
	return true, nil
}

func prerequisiteInvalidationExists(aggregate RuntimeAggregate, signature string) bool {
	signature = strings.TrimSpace(signature)
	for _, value := range aggregate.PrerequisiteInvalidations {
		if value.Normalized().Signature == signature {
			return true
		}
	}
	return false
}

// validatePrerequisiteResultFreshness prevents an owner response produced for
// a previous instance of a stable prerequisite ID from satisfying the current
// re-acquisition. Prerequisites intentionally retain their semantic ID after
// invalidation so reducer ordering and owner contracts stay stable; the
// persisted owner observation is consequently the generation fence.
//
// This is an Orchestrator admission rule, not a DTO transition: capability
// owners continue to report only their current observation, while SM-2 decides
// whether that observation may re-enter the aggregate.
func validatePrerequisiteResultFreshness(result PrerequisiteResult, invalidations []runtimecontract.RuntimePrerequisiteInvalidationRecord) error {
	prerequisiteID := strings.TrimSpace(result.PrerequisiteID)
	if prerequisiteID == "" {
		return common.ErrInvalidArgument
	}
	for _, invalidation := range invalidations {
		invalidation = invalidation.Normalized()
		if !containsFreshnessString(invalidation.PrerequisiteIDs, prerequisiteID) {
			continue
		}
		if result.Observation == nil {
			return fmt.Errorf("%w: prerequisite %q was invalidated and requires a fresh owner observation", common.ErrConflict, prerequisiteID)
		}
		observation := result.Observation.Normalized()
		for _, dependency := range observation.Dependencies {
			for _, invalidatedDependency := range invalidation.InvalidatedDependencies {
				invalidatedDependency = invalidatedDependency.Normalized()
				if dependency.Domain == invalidatedDependency.Domain && dependency.OwnerModuleID == invalidatedDependency.OwnerModuleID &&
					dependency.SubjectRef == invalidatedDependency.SubjectRef && dependency.Revision == invalidatedDependency.Revision {
					return fmt.Errorf("%w: prerequisite %q result retains an invalidated owner freshness dependency", common.ErrConflict, prerequisiteID)
				}
			}
			if invalidation.Stale != nil {
				stale := invalidation.Stale.Normalized()
				if dependency.Domain == stale.Domain && dependency.OwnerModuleID == stale.OwnerModuleID &&
					dependency.SubjectRef == stale.SubjectRef && dependency.Revision == stale.ExpectedRevision {
					return fmt.Errorf("%w: prerequisite %q result retains superseded %s revision", common.ErrConflict, prerequisiteID, stale.Domain)
				}
			}
			for _, change := range invalidation.ResourceChanges {
				if runtimecontract.RuntimeFreshnessDependencyIntersectsChange(dependency, change) &&
					dependency.Revision == change.BeforeRevision {
					return fmt.Errorf("%w: prerequisite %q result retains resource revision superseded by durable receipt", common.ErrConflict, prerequisiteID)
				}
			}
		}
	}
	return nil
}

func containsFreshnessString(values []string, value string) bool {
	value = strings.TrimSpace(value)
	for _, candidate := range values {
		if strings.TrimSpace(candidate) == value {
			return true
		}
	}
	return false
}

func invalidatePrerequisiteObservationsForStale(facts runtimecontract.RuntimeFacts, stale runtimecontract.RuntimePrerequisiteStale) (runtimecontract.RuntimeFacts, []string, []string, []runtimecontract.RuntimeFreshnessDependency, error) {
	facts = facts.Normalized()
	stale = stale.Normalized()
	observation, found := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, stale.PrerequisiteID)
	if !found || observation.ObservationID != stale.ObservationID {
		return runtimecontract.RuntimeFacts{}, nil, nil, nil, fmt.Errorf("%w: stale prerequisite observation is not current", common.ErrConflict)
	}
	matchedDependency := false
	for _, dependency := range observation.Dependencies {
		if dependency.Domain == stale.Domain && dependency.OwnerModuleID == stale.OwnerModuleID && dependency.SubjectRef == stale.SubjectRef && dependency.Revision == stale.ExpectedRevision {
			matchedDependency = true
			break
		}
	}
	if !matchedDependency {
		return runtimecontract.RuntimeFacts{}, nil, nil, nil, fmt.Errorf("%w: stale prerequisite dependency is not current", common.ErrConflict)
	}
	// A stale owner observation proves that the exact dependency revision is no
	// longer authoritative, not merely that the responder which reported it is
	// stale. Retire every current observation with that same typed dependency.
	// This matters for a Git admissibility inventory: it is bound to a Git
	// repository-state snapshot that carries the identical repository revision.
	// Keeping that snapshot would force re-acquisition to reuse the obsolete
	// binding and make the next owner result uncommittable.
	affected := make([]runtimecontract.RuntimePrerequisiteObservation, 0, 1)
	for _, candidate := range facts.Resources.PrerequisiteObservations {
		for _, dependency := range candidate.Normalized().Dependencies {
			if dependency.Domain == stale.Domain && dependency.OwnerModuleID == stale.OwnerModuleID &&
				dependency.SubjectRef == stale.SubjectRef && dependency.Revision == stale.ExpectedRevision {
				affected = append(affected, candidate)
				break
			}
		}
	}
	updated, prerequisiteIDs, observationIDs, dependencies, err := invalidatePrerequisiteObservations(facts, affected)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, nil, nil, nil, err
	}
	return updated, prerequisiteIDs, observationIDs, freshnessDependenciesMatchingStale(dependencies, stale), nil
}

func invalidatePrerequisiteObservationsForChanges(facts runtimecontract.RuntimeFacts, changes []runtimecontract.RuntimeResourceChange) (runtimecontract.RuntimeFacts, []string, []string, []runtimecontract.RuntimeFreshnessDependency, error) {
	facts = facts.Normalized()
	changes = runtimecontract.NormalizeRuntimeResourceChanges(changes)
	affected := make([]runtimecontract.RuntimePrerequisiteObservation, 0)
	for _, observation := range facts.Resources.PrerequisiteObservations {
		for _, dependency := range observation.Dependencies {
			for _, change := range changes {
				if runtimecontract.RuntimeFreshnessDependencyIntersectsChange(dependency, change) {
					affected = append(affected, observation)
					goto nextObservation
				}
			}
		}
	nextObservation:
	}
	if len(affected) == 0 {
		return facts, nil, nil, nil, nil
	}
	updated, prerequisiteIDs, observationIDs, dependencies, err := invalidatePrerequisiteObservations(facts, affected)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, nil, nil, nil, err
	}
	return updated, prerequisiteIDs, observationIDs, freshnessDependenciesIntersectingChanges(dependencies, changes), nil
}

func freshnessDependenciesMatchingStale(values []runtimecontract.RuntimeFreshnessDependency, stale runtimecontract.RuntimePrerequisiteStale) []runtimecontract.RuntimeFreshnessDependency {
	stale = stale.Normalized()
	ret := make([]runtimecontract.RuntimeFreshnessDependency, 0, 1)
	for _, dependency := range values {
		dependency = dependency.Normalized()
		if dependency.Domain == stale.Domain && dependency.OwnerModuleID == stale.OwnerModuleID &&
			dependency.SubjectRef == stale.SubjectRef && dependency.Revision == stale.ExpectedRevision {
			ret = append(ret, dependency)
		}
	}
	return runtimecontract.NormalizeRuntimeFreshnessDependencies(ret)
}

func freshnessDependenciesIntersectingChanges(values []runtimecontract.RuntimeFreshnessDependency, changes []runtimecontract.RuntimeResourceChange) []runtimecontract.RuntimeFreshnessDependency {
	changes = runtimecontract.NormalizeRuntimeResourceChanges(changes)
	ret := make([]runtimecontract.RuntimeFreshnessDependency, 0, len(values))
	for _, dependency := range values {
		for _, change := range changes {
			if runtimecontract.RuntimeFreshnessDependencyIntersectsChange(dependency, change) {
				ret = append(ret, dependency)
				break
			}
		}
	}
	return runtimecontract.NormalizeRuntimeFreshnessDependencies(ret)
}

func invalidatePrerequisiteObservations(facts runtimecontract.RuntimeFacts, observations []runtimecontract.RuntimePrerequisiteObservation) (runtimecontract.RuntimeFacts, []string, []string, []runtimecontract.RuntimeFreshnessDependency, error) {
	facts = facts.Normalized()
	byID := map[string]runtimecontract.RuntimePrerequisiteObservation{}
	for _, observation := range observations {
		observation = observation.Normalized()
		byID[observation.PrerequisiteID] = observation
	}
	if len(byID) == 0 {
		return facts, nil, nil, nil, nil
	}
	satisfied := make(map[string]runtimecontract.RuntimePrerequisite, len(facts.Prerequisites.Satisfied))
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		satisfied[prerequisite.ID] = prerequisite
	}
	prerequisiteIDs := make([]string, 0, len(byID))
	observationIDs := make([]string, 0, len(byID))
	invalidatedDependencies := make([]runtimecontract.RuntimeFreshnessDependency, 0)
	for prerequisiteID, observation := range byID {
		prerequisite, found := satisfied[prerequisiteID]
		if !found || prerequisite.Kind != observation.Kind || prerequisite.OwnerModuleID != observation.OwnerModuleID {
			return runtimecontract.RuntimeFacts{}, nil, nil, nil, fmt.Errorf("%w: observation %q has no current satisfied prerequisite", common.ErrConflict, prerequisiteID)
		}
		if err := removePrerequisiteResourceFact(&facts, prerequisite); err != nil {
			return runtimecontract.RuntimeFacts{}, nil, nil, nil, err
		}
		prerequisiteIDs = append(prerequisiteIDs, prerequisiteID)
		observationIDs = append(observationIDs, observation.ObservationID)
		invalidatedDependencies = append(invalidatedDependencies, observation.Dependencies...)
	}
	// A failure target observed by a capability is only authoritative while the
	// owner catalog revision that produced it remains fresh. The invalidated
	// prerequisite already carries that owner-neutral dependency; do not retain
	// its target as a second, stale source of repair authority.
	invalidateFailureTargetObservations(&facts, byID)
	keptSatisfied := make([]runtimecontract.RuntimePrerequisite, 0, len(facts.Prerequisites.Satisfied))
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if _, invalidated := byID[prerequisite.ID]; !invalidated {
			keptSatisfied = append(keptSatisfied, prerequisite)
		}
	}
	facts.Prerequisites.Satisfied = keptSatisfied
	for _, prerequisiteID := range prerequisiteIDs {
		facts.Prerequisites.Pending = append(facts.Prerequisites.Pending, satisfied[prerequisiteID])
	}
	keptObservations := make([]runtimecontract.RuntimePrerequisiteObservation, 0, len(facts.Resources.PrerequisiteObservations))
	for _, observation := range facts.Resources.PrerequisiteObservations {
		if _, invalidated := byID[observation.PrerequisiteID]; !invalidated {
			keptObservations = append(keptObservations, observation)
		}
	}
	facts.Resources.PrerequisiteObservations = keptObservations
	facts.Feasibility = nil
	return facts.Normalized(), normalizedFreshnessStrings(prerequisiteIDs), normalizedFreshnessStrings(observationIDs), runtimecontract.NormalizeRuntimeFreshnessDependencies(invalidatedDependencies), nil
}

func invalidateFailureTargetObservations(facts *runtimecontract.RuntimeFacts, invalidated map[string]runtimecontract.RuntimePrerequisiteObservation) {
	if facts == nil || facts.Resources.FailureTarget == nil || facts.Resources.FailureTarget.TargetObservation == nil {
		return
	}
	targetObservation := facts.Resources.FailureTarget.TargetObservation.Normalized()
	if !failureTargetObservationDependsOnInvalidatedOwnerRevision(targetObservation, invalidated) {
		return
	}
	facts.Resources.FailureTarget = nil
	if facts.Failure != nil && sameFailureTargetObservationBinding(facts.Failure.TargetObservation, &targetObservation) {
		facts.Failure.TargetRefs = nil
		facts.Failure.TargetObservation = nil
	}
	// The directive is a sealed SM-2 decision derived from the same stale
	// failure fact. ReduceRuntimeStateWithDirective will rebuild it from the
	// remaining fresh facts after re-acquisition; retaining it would make its
	// target refs an unauthorized fallback.
	if facts.RepairDirective != nil && facts.RepairDirective.CurrentFailure != nil &&
		sameFailureTargetObservationBinding(facts.RepairDirective.CurrentFailure.TargetObservation, &targetObservation) {
		facts.RepairDirective = nil
	}
}

func failureTargetObservationDependsOnInvalidatedOwnerRevision(target runtimecontract.RuntimeFailureTargetObservation, invalidated map[string]runtimecontract.RuntimePrerequisiteObservation) bool {
	target = target.Normalized()
	for _, observation := range invalidated {
		for _, dependency := range observation.Normalized().Dependencies {
			if dependency.OwnerModuleID == target.OwnerModuleID && dependency.Revision == target.ObservedRevision {
				return true
			}
		}
	}
	return false
}

func sameFailureTargetObservationBinding(left *runtimecontract.RuntimeFailureTargetObservation, right *runtimecontract.RuntimeFailureTargetObservation) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l := left.Normalized()
	r := right.Normalized()
	if l.OwnerModuleID != r.OwnerModuleID || l.SubjectRef != r.SubjectRef || l.TargetRole != r.TargetRole || l.ObservedRevision != r.ObservedRevision ||
		l.DiagnosticClass != r.DiagnosticClass || l.ContentHash != r.ContentHash || !sameRuntimeTargetRefs(l.EvidenceRefs, r.EvidenceRefs) || len(l.TargetRefs) != len(r.TargetRefs) {
		return false
	}
	for idx := range l.TargetRefs {
		if l.TargetRefs[idx] != r.TargetRefs[idx] {
			return false
		}
	}
	return true
}

func normalizedFreshnessStrings(values []string) []string {
	ret := []string{}
	for _, value := range values {
		ret = appendNormalizedString(ret, value)
	}
	return ret
}

func removePrerequisiteResourceFact(facts *runtimecontract.RuntimeFacts, prerequisite runtimecontract.RuntimePrerequisite) error {
	if facts == nil {
		return common.ErrInvalidArgument
	}
	switch prerequisite.Kind {
	case runtimecontract.PrerequisiteKindManifest:
		facts.Resources.Manifest = nil
	case runtimecontract.PrerequisiteKindTargetSnapshot:
		kept := make([]runtimecontract.TargetSnapshot, 0, len(facts.Resources.TargetSnapshots))
		for _, snapshot := range facts.Resources.TargetSnapshots {
			if snapshot.TargetRef != prerequisite.SubjectRef {
				kept = append(kept, snapshot)
			}
		}
		facts.Resources.TargetSnapshots = kept
	case runtimecontract.PrerequisiteKindTargetSelection:
		facts.Resources.TargetSelection = nil
	case runtimecontract.PrerequisiteKindCapabilityState:
		kept := make([]runtimecontract.RuntimeCapabilityStateSnapshot, 0, len(facts.Resources.CapabilityStates))
		for _, snapshot := range facts.Resources.CapabilityStates {
			if snapshot.RequirementID != prerequisite.ID {
				kept = append(kept, snapshot)
			}
		}
		facts.Resources.CapabilityStates = kept
	case runtimecontract.PrerequisiteKindValidationScratch:
		facts.Resources.ValidationScratch = nil
	case runtimecontract.PrerequisiteKindProviderCeiling:
		facts.Resources.ProviderCeiling = nil
	case runtimecontract.PrerequisiteKindOperationAdmissibility:
		kept := make([]runtimecontract.RuntimeOperationAdmissibility, 0, len(facts.Resources.OperationAdmissibilities))
		for _, inventory := range facts.Resources.OperationAdmissibilities {
			if inventory.PrerequisiteID != prerequisite.ID {
				kept = append(kept, inventory)
			}
		}
		facts.Resources.OperationAdmissibilities = kept
	default:
		return fmt.Errorf("freshness invalidation does not support prerequisite kind %q", prerequisite.Kind)
	}
	return nil
}
