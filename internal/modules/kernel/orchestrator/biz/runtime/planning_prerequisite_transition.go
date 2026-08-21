package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *planningTransactionComponent) invalidateCandidateRequestSessionForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, stale runtimecontract.RuntimePrerequisiteStale, budgetObservation ...*runtimecontract.RuntimeProviderBudgetObservation) (RuntimeAggregate, error) {
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
	var observation *runtimecontract.RuntimeProviderBudgetObservation
	if session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		observation, err = candidateRequestBudgetObservationForTransition(session, runtimecontract.RuntimeProviderConsumptionUnknown, budgetObservation...)
		if err != nil {
			return RuntimeAggregate{}, err
		}
	} else if firstRuntimeProviderBudgetObservation(budgetObservation) != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: completed candidate request budget was already settled", common.ErrConflict)
	}
	if _, err := applyCandidateRequestBudgetObservation(&aggregate, observation); err != nil {
		return RuntimeAggregate{}, err
	}
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
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil, aggregate.CoverageProofLineage)
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
