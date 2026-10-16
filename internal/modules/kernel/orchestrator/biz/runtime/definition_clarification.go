package runtime

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// observeRuntimeDefinitionGap is the SM-2 admission boundary for one complete,
// already-persisted semantic provider document. It records the exact current
// request/authority binding and seals canonical need_governance authority, but
// does not create a Decision ticket or an operation dispatch.
func (c *authorityComponent) observeRuntimeDefinitionGap(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, observation runtimecontract.RuntimeDefinitionGapObservation) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	observation = observation.Clone()
	if err := observation.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.DefinitionClarification != nil {
		current := aggregate.DefinitionClarification.Clone()
		if current.Observation.ObservationID == observation.ObservationID {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: Runtime already has a different Definition clarification", common.ErrConflict)
	}
	definition := aggregate.Definition.Normalized()
	if observation.WorkspaceID != workspaceID || observation.RunID != runID || observation.DefinitionRevision != definition.Revision ||
		observation.StartupInputHash != definition.StartupInputHash || observation.AuthoritySnapshotID != aggregate.Decision.AuthoritySnapshot.ID ||
		observation.FocusRef != aggregate.Facts.Focus.ID || aggregate.ActiveDispatch != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime Definition gap observation is stale", common.ErrConflict)
	}
	matchedSession := false
	for _, session := range aggregate.CandidateRequestSessions {
		if session.LogicalRequestID == observation.LogicalRequestID && session.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan &&
			session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete && session.Transaction.AuthoritySnapshotID == observation.AuthoritySnapshotID {
			matchedSession = true
			break
		}
	}
	if !matchedSession {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime Definition gap has no exact complete semantic request", common.ErrConflict)
	}
	decision, err := runtimecontract.SealRuntimeDecision(aggregate.Facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedGovernance,
		Focus: aggregate.Facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeDefinitionClarificationRequiredOutputKind},
		},
	}, 0)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	state := runtimecontract.RuntimeDefinitionClarificationState{Observation: observation}
	aggregate.DefinitionClarification = &state
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// sealRuntimeDefinitionClarificationDispatch is the dedicated SM-3 seal. It
// derives only from the persisted observation plus the current clarification
// authority and never constructs RuntimeCandidate/GovernanceCandidate.
func (c *candidateDispatchComponent) sealRuntimeDefinitionClarificationDispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.DefinitionClarification == nil || aggregate.Decision.State != runtimecontract.ConvergenceStateNeedGovernance || aggregate.ActiveDispatch != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime Definition clarification dispatch has no current observation", common.ErrConflict)
	}
	state := aggregate.DefinitionClarification.Clone()
	if state.Dispatch != nil {
		return aggregate, nil
	}
	dispatch, err := runtimecontract.CompileRuntimeDefinitionClarificationDispatch(state.Observation, aggregate.Decision.AuthoritySnapshot.ID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	state.Dispatch = &dispatch
	aggregate.DefinitionClarification = &state
	aggregate.StateRevision++
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// bindRuntimeDefinitionClarificationRequest records the exact Decision result
// only after the caller has validated all response fields and the command
// binding hash. Duplicate response delivery is a zero-revision join.
func (c *candidateDispatchComponent) bindRuntimeDefinitionClarificationRequest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, request runtimecontract.RuntimeDefinitionClarificationRequestRecord) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	request = request.Normalized()
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.DefinitionClarification == nil || aggregate.DefinitionClarification.Dispatch == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime Definition clarification request has no sealed dispatch", common.ErrConflict)
	}
	state := aggregate.DefinitionClarification.Clone()
	if err := request.ValidateAgainst(*state.Dispatch); err != nil {
		return RuntimeAggregate{}, err
	}
	if state.Request != nil {
		current := state.Request.Normalized()
		if current == request {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime Definition clarification dispatch is already bound", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	state.Request = &request
	aggregate.DefinitionClarification = &state
	aggregate.StateRevision++
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}
