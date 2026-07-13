package runtimev2

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *RuntimeController) BindGovernanceRequest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, request runtimecontract.RuntimeGovernanceRequestRecord) (RuntimeAggregate, error) {
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
		if current == request {
			return aggregate, nil
		}
		if current.TicketID == request.TicketID || (current.AuthoritySnapshotID == request.AuthoritySnapshotID && current.DispatchID == request.DispatchID) {
			return RuntimeAggregate{}, fmt.Errorf("%w: governed dispatch is already bound to another ticket", common.ErrConflict)
		}
	}
	if aggregate.Decision.Action != runtimecontract.RuntimeActionRequestGovernance || aggregate.ActiveDispatch == nil ||
		aggregate.ActiveDispatch.ID != request.DispatchID || aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID != request.AuthoritySnapshotID ||
		aggregate.ActiveDispatch.Candidate.Governance == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance request does not match the active governed dispatch", common.ErrConflict)
	}
	dependency := aggregate.ActiveDispatch.Candidate.Governance.ExternalDependency
	if strings.TrimSpace(dependency.Kind) != strings.TrimSpace(request.DependencyKind) || strings.TrimSpace(dependency.SubjectRef) != strings.TrimSpace(request.SubjectRef) {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance request dependency does not match the sealed candidate", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	aggregate.GovernanceRequests = append(aggregate.GovernanceRequests, request)
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *RuntimeController) CommitGovernanceResolution(ctx context.Context, resolution runtimecontract.RuntimeGovernanceResolution) (EvidenceCommitResult, error) {
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
		aggregate.Diagnostics = append(aggregate.Diagnostics, DiagnosticEvidence{
			EventID: eventID, AuthoritySnapshotID: resolution.AuthoritySnapshotID,
			CurrentSnapshotID: aggregate.Decision.AuthoritySnapshot.ID, DispatchID: resolution.DispatchID,
			Reason: "governance resolution belongs to a stale authority snapshot",
		})
		aggregate.StateRevision++
		if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
			return EvidenceCommitResult{}, err
		}
		return EvidenceCommitResult{Disposition: CommitDispositionStale, Aggregate: aggregate}, nil
	}
	if aggregate.Decision.Action != runtimecontract.RuntimeActionRequestGovernance || aggregate.ActiveDispatch == nil ||
		aggregate.ActiveDispatch.ID != resolution.DispatchID || aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID != resolution.AuthoritySnapshotID ||
		aggregate.ActiveDispatch.Candidate.Governance == nil {
		return EvidenceCommitResult{}, fmt.Errorf("%w: governance resolution does not match the active governed dispatch", common.ErrConflict)
	}
	dependency := aggregate.ActiveDispatch.Candidate.Governance.ExternalDependency
	if strings.TrimSpace(dependency.Kind) != resolution.DependencyKind || strings.TrimSpace(dependency.SubjectRef) != resolution.SubjectRef {
		return EvidenceCommitResult{}, fmt.Errorf("%w: governance resolution dependency does not match the sealed candidate", common.ErrConflict)
	}
	bound := false
	for _, request := range aggregate.GovernanceRequests {
		if request.TicketID == resolution.TicketID && request.AuthoritySnapshotID == resolution.AuthoritySnapshotID && request.DispatchID == resolution.DispatchID &&
			strings.TrimSpace(request.DependencyKind) == resolution.DependencyKind && strings.TrimSpace(request.SubjectRef) == resolution.SubjectRef {
			bound = true
			break
		}
	}
	if !bound {
		return EvidenceCommitResult{}, fmt.Errorf("%w: governance resolution is not bound to the governed ticket", common.ErrConflict)
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
	decision, err := ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return EvidenceCommitResult{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return EvidenceCommitResult{}, err
	}
	return EvidenceCommitResult{Disposition: CommitDispositionCommitted, DispatchCompleted: true, Aggregate: aggregate}, nil
}
