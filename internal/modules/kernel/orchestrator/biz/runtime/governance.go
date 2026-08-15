package runtime

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *authorityComponent) bindGovernanceRequest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, request runtimecontract.RuntimeGovernanceRequestRecord) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if err := runtimecontract.ValidateRuntimeGovernanceRequestRecords([]runtimecontract.RuntimeGovernanceRequestRecord{request}); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	for _, current := range aggregate.GovernanceRequests {
		if governancecheck.SameRequestRecord(current, request) {
			return aggregate, nil
		}
		if governancecheck.RequestRecordConflicts(current, request) {
			return RuntimeAggregate{}, fmt.Errorf("%w: governed dispatch is already bound to another ticket", common.ErrConflict)
		}
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.Candidate.Governance == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance request does not match the active governed dispatch", common.ErrConflict)
	}
	dependency := aggregate.ActiveDispatch.Candidate.Governance.ExternalDependency
	dispatchFacts := governancecheck.DispatchFacts{
		DecisionState: aggregate.Decision.State, DispatchID: aggregate.ActiveDispatch.ID,
		AuthoritySnapshotID: aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID,
		CandidateGovernance: aggregate.ActiveDispatch.Candidate.Governance != nil,
		DependencyKind:      dependency.Kind, SubjectRef: dependency.SubjectRef,
	}
	if !governancecheck.GovernedDispatchMatches(dispatchFacts, request.DispatchID, request.AuthoritySnapshotID) {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance request does not match the active governed dispatch", common.ErrConflict)
	}
	if !governancecheck.DependencyMatches(dependency.Kind, dependency.SubjectRef, request.DependencyKind, request.SubjectRef) {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance request dependency does not match the sealed candidate", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	for idx := range aggregate.OperationGovernanceBindings {
		binding := aggregate.OperationGovernanceBindings[idx].Normalized()
		if !governancecheck.SuspendedBindingMatchesRequest(binding, request) {
			continue
		}
		binding.TicketID = request.TicketID
		binding.GovernanceDispatchID = request.DispatchID
		binding.GovernanceSnapshotID = request.AuthoritySnapshotID
		binding.State = runtimecontract.RuntimeOperationGovernanceBindingTicketBound
		aggregate.OperationGovernanceBindings[idx] = binding
	}
	aggregate.GovernanceRequests = append(aggregate.GovernanceRequests, request)
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *authorityComponent) commitGovernanceResolution(ctx context.Context, resolution runtimecontract.RuntimeGovernanceResolution) (EvidenceCommitResult, error) {
	if c == nil || c.repository == nil {
		return EvidenceCommitResult{}, common.ErrInvalidArgument
	}
	resolution = resolution.Normalized()
	if err := resolution.Validate(); err != nil {
		return EvidenceCommitResult{}, err
	}
	aggregate, err := c.repository.Load(ctx, resolution.WorkspaceID, resolution.RunID)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	if err := aggregate.Validate(); err != nil {
		return EvidenceCommitResult{}, err
	}
	eventID := resolution.EventID()
	if containsString(aggregate.ProcessedEventIDs, eventID) {
		return EvidenceCommitResult{Disposition: CommitDispositionDuplicate, Aggregate: aggregate}, nil
	}
	expectedRevision := aggregate.StateRevision
	if resolution.AuthoritySnapshotID != aggregate.Decision.AuthoritySnapshot.ID {
		aggregate.ProcessedEventIDs = appendNormalizedString(aggregate.ProcessedEventIDs, eventID)
		aggregate.Diagnostics = append(aggregate.Diagnostics, runtimecontract.RuntimeDiagnosticRecord{
			EventID: eventID, AuthoritySnapshotID: resolution.AuthoritySnapshotID,
			CurrentSnapshotID: aggregate.Decision.AuthoritySnapshot.ID, DispatchID: resolution.DispatchID,
			Reason: "governance resolution belongs to a stale authority snapshot",
		})
		aggregate.StateRevision++
		if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
			return EvidenceCommitResult{}, err
		}
		return EvidenceCommitResult{Disposition: CommitDispositionStale, Aggregate: aggregate}, nil
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.Candidate.Governance == nil {
		return EvidenceCommitResult{}, fmt.Errorf("%w: governance resolution does not match the active governed dispatch", common.ErrConflict)
	}
	dependency := aggregate.ActiveDispatch.Candidate.Governance.ExternalDependency
	dispatchFacts := governancecheck.DispatchFacts{
		DecisionState: aggregate.Decision.State, DispatchID: aggregate.ActiveDispatch.ID,
		AuthoritySnapshotID: aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID,
		CandidateGovernance: aggregate.ActiveDispatch.Candidate.Governance != nil,
		DependencyKind:      dependency.Kind, SubjectRef: dependency.SubjectRef,
	}
	if !governancecheck.GovernedDispatchMatches(dispatchFacts, resolution.DispatchID, resolution.AuthoritySnapshotID) {
		return EvidenceCommitResult{}, fmt.Errorf("%w: governance resolution does not match the active governed dispatch", common.ErrConflict)
	}
	if !governancecheck.DependencyMatches(dependency.Kind, dependency.SubjectRef, resolution.DependencyKind, resolution.SubjectRef) {
		return EvidenceCommitResult{}, fmt.Errorf("%w: governance resolution dependency does not match the sealed candidate", common.ErrConflict)
	}
	bound := false
	for _, request := range aggregate.GovernanceRequests {
		if governancecheck.RequestBindsResolution(request, resolution) {
			bound = true
			break
		}
	}
	if !bound {
		return EvidenceCommitResult{}, fmt.Errorf("%w: governance resolution is not bound to the governed ticket", common.ErrConflict)
	}
	grantedBindingIndex := -1
	for idx := range aggregate.OperationGovernanceBindings {
		binding := aggregate.OperationGovernanceBindings[idx].Normalized()
		if !governancecheck.BindingMatchesResolution(binding, resolution) {
			continue
		}
		switch resolution.Outcome {
		case runtimecontract.RuntimeGovernanceResolutionOutcomeContinue:
			if !governancecheck.BindingStateAllowsOutcome(binding.State, resolution.Outcome) {
				return EvidenceCommitResult{}, fmt.Errorf("%w: governance continuation binding is not ticket_bound", common.ErrConflict)
			}
			binding.State = runtimecontract.RuntimeOperationGovernanceBindingGranted
			binding.Grant = &runtimecontract.RuntimeOperationGovernanceGrant{
				ResolutionID: resolution.ResolutionID, TicketID: resolution.TicketID,
				ExecutionKey: binding.ExecutionKey, GovernanceFence: binding.Fence, ResumeFence: binding.Fence + 1,
				DependencyKind: binding.DependencyKind, SubjectRef: binding.SubjectRef,
				GovernanceReceiptHash: binding.GovernanceReceiptHash, EvidenceRefs: append([]string(nil), resolution.EvidenceRefs...),
			}
			grantedBindingIndex = idx
		case runtimecontract.RuntimeGovernanceResolutionOutcomeFail:
			if !governancecheck.BindingStateAllowsOutcome(binding.State, resolution.Outcome) {
				return EvidenceCommitResult{}, fmt.Errorf("%w: governance rejection binding is not ticket_bound", common.ErrConflict)
			}
			binding.State = runtimecontract.RuntimeOperationGovernanceBindingRejected
		}
		aggregate.OperationGovernanceBindings[idx] = binding
	}

	aggregate.EvidenceSequence++
	aggregate.Facts.EvidenceRevision = nextEvidenceRevision(aggregate.EvidenceSequence)
	fact := runtimecontract.EvidenceFact{
		ID: common.EvidenceRecordID(eventID), Kind: runtimecontract.EvidenceKindGovernance,
		SubjectRef: resolution.SubjectRef, SourceRef: "governance-resolution/" + string(resolution.ResolutionID),
		AuthoritySnapshotID: resolution.AuthoritySnapshotID, DispatchID: resolution.DispatchID,
		Revision: aggregate.Facts.EvidenceRevision, Summary: resolution.Summary,
		EvidenceRefs: append([]string(nil), resolution.EvidenceRefs...),
	}
	switch resolution.Outcome {
	case runtimecontract.RuntimeGovernanceResolutionOutcomeContinue:
		aggregate.Facts.Evidence.Accepted = append(aggregate.Facts.Evidence.Accepted, fact)
		aggregate.Facts.Failure = nil
	case runtimecontract.RuntimeGovernanceResolutionOutcomeFail:
		fact.FailureClass = runtimecontract.FailureClassGovernanceRejected
		fact.FailureMessage = resolution.Summary
		aggregate.Facts.Evidence.Rejected = append(aggregate.Facts.Evidence.Rejected, fact)
		aggregate.Facts.Failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassGovernanceRejected, Signature: string(resolution.ResolutionID), Message: resolution.Summary,
		}
	default:
		return EvidenceCommitResult{}, fmt.Errorf("unsupported governance resolution outcome %q", resolution.Outcome)
	}
	aggregate.Facts = aggregate.Facts.Normalized()
	aggregate.ProcessedEventIDs = appendNormalizedString(aggregate.ProcessedEventIDs, eventID)
	aggregate.GovernanceResolutions = append(aggregate.GovernanceResolutions, runtimecontract.RuntimeGovernanceResolutionRecord{
		ResolutionID: resolution.ResolutionID, TicketID: resolution.TicketID, AuthoritySnapshotID: resolution.AuthoritySnapshotID,
		DispatchID: resolution.DispatchID, Outcome: resolution.Outcome,
	})
	dispatch := *aggregate.ActiveDispatch
	dispatch.State = DispatchStateCommitted
	aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
	aggregate.ActiveDispatch = nil
	if grantedBindingIndex < 0 {
		aggregate.Facts.RepairDirective = nil
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	if grantedBindingIndex >= 0 {
		binding := aggregate.OperationGovernanceBindings[grantedBindingIndex].Normalized()
		resume, resumeErr := buildGovernanceResumeDispatch(aggregate, binding)
		if resumeErr != nil {
			return EvidenceCommitResult{}, resumeErr
		}
		aggregate.ActiveDispatch = &resume
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return EvidenceCommitResult{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return EvidenceCommitResult{}, err
	}
	return EvidenceCommitResult{Disposition: CommitDispositionCommitted, DispatchCompleted: true, Aggregate: aggregate}, nil
}

func buildGovernanceResumeDispatch(aggregate RuntimeAggregate, binding runtimecontract.RuntimeOperationGovernanceBinding) (SealedDispatch, error) {
	binding = binding.Normalized()
	if binding.State != runtimecontract.RuntimeOperationGovernanceBindingGranted || binding.Grant == nil {
		return SealedDispatch{}, fmt.Errorf("governance resume requires granted binding")
	}
	var source *SealedDispatch
	for idx := range aggregate.CompletedDispatches {
		if aggregate.CompletedDispatches[idx].ID == binding.OperationDispatchID {
			value := aggregate.CompletedDispatches[idx]
			source = &value
			break
		}
	}
	if source == nil {
		return SealedDispatch{}, fmt.Errorf("governance resume source dispatch %q is missing", binding.OperationDispatchID)
	}
	resume, err := sealDispatchWithActivationAndFeasibilities(aggregate.Decision, source.Candidate, aggregate.Facts.Activation, source.ValidationRecipes, nil)
	if err != nil {
		return SealedDispatch{}, err
	}
	resume.CompletedOperationIDs = append([]string(nil), source.CompletedOperationIDs...)
	found := false
	for idx := range resume.OperationExecutions {
		execution := &resume.OperationExecutions[idx]
		if execution.OperationID != binding.OperationID {
			if sourceExecution, ok := operationExecutionRecord(source.OperationExecutions, execution.OperationID); ok &&
				containsString(source.CompletedOperationIDs, execution.OperationID) {
				*execution = sourceExecution
			}
			continue
		}
		found = true
		execution.ExecutionKey = binding.ExecutionKey
		execution.ExpectedEffectScope = binding.ExpectedEffectScope
		execution.State = runtimecontract.OperationExecutionClaimed
		execution.Fence = binding.Grant.ResumeFence
		execution.OwnerReceiptRef = ""
		execution.OwnerReceiptHash = ""
		execution.ClaimedAt = "governance_granted"
	}
	if !found {
		return SealedDispatch{}, fmt.Errorf("governance resume operation %q is missing from source candidate", binding.OperationID)
	}
	if err := resume.ValidateWithActivation(aggregate.Decision, aggregate.Facts.Activation); err != nil {
		return SealedDispatch{}, err
	}
	return resume, nil
}
