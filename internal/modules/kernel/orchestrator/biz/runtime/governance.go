package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
			return RuntimeAggregate{}, fmt.Errorf("%w: governed dispatch is already bound to another governance owner record", common.ErrConflict)
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
	if strings.TrimSpace(request.RequestedAction) != strings.TrimSpace(dependency.RequestedAction) || strings.TrimSpace(request.RiskLevel) != strings.TrimSpace(dependency.RiskLevel) ||
		strings.TrimSpace(request.AuthorizationHash) != runtimecontract.StableRuntimeAuthorizationContextHash(dependency.AuthorizationContext) {
		return RuntimeAggregate{}, fmt.Errorf("%w: governance request authorization target does not match the sealed candidate", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	for idx := range aggregate.OperationGovernanceBindings {
		binding := aggregate.OperationGovernanceBindings[idx].Normalized()
		if !governancecheck.SuspendedBindingMatchesRequest(binding, request) {
			continue
		}
		binding.TicketID = request.TicketID
		binding.ApprovalID = request.ApprovalID
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
	resolution = resolution.Normalized()
	var lease *runtimecontract.RuntimeDriveLease
	if resolution.Outcome == runtimecontract.RuntimeGovernanceResolutionOutcomeContinue || resolution.Outcome == runtimecontract.RuntimeGovernanceResolutionOutcomeFail {
		signature, err := RuntimeGovernanceResolutionDriveLeaseSignature(resolution)
		if err != nil {
			return EvidenceCommitResult{}, err
		}
		lease = &runtimecontract.RuntimeDriveLease{
			WorkspaceID:      resolution.WorkspaceID,
			RunID:            resolution.RunID,
			Reason:           runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry,
			FailureSignature: signature,
			DueAt:            time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano),
		}
	}
	return c.commitGovernanceResolutionWithContinuationLease(ctx, resolution, lease)
}

// RuntimeGovernanceResolutionDriveLeaseSignature binds the atomic SM-4
// successor to the exact persisted governance resolution. Continue uses the
// Run lifecycle handoff identity; Fail uses a terminal-delivery identity and
// never authorizes a Run continuation.
func RuntimeGovernanceResolutionDriveLeaseSignature(resolution runtimecontract.RuntimeGovernanceResolution) (string, error) {
	resolution = resolution.Normalized()
	if err := resolution.Validate(); err != nil {
		return "", err
	}
	if resolution.Outcome == runtimecontract.RuntimeGovernanceResolutionOutcomeFail {
		return fmt.Sprintf(
			"terminal-delivery/governance-resolution/%s/%s/%s/%s/%s",
			resolution.AuthoritySnapshotID,
			resolution.DispatchID,
			resolution.TicketID,
			resolution.ApprovalID,
			resolution.ResolutionID,
		), nil
	}
	return RuntimeGovernanceContinuationDriveLeaseSignature(resolution)
}

// RuntimeGovernanceContinuationDriveLeaseSignature is the canonical opaque
// SM-4 identity for the durable successor sealed with a governance
// continuation. It intentionally excludes mutable aggregate counters while
// retaining the exact resolution and governance fence.
func RuntimeGovernanceContinuationDriveLeaseSignature(resolution runtimecontract.RuntimeGovernanceResolution) (string, error) {
	resolution = resolution.Normalized()
	if err := resolution.Validate(); err != nil {
		return "", err
	}
	if resolution.Outcome != runtimecontract.RuntimeGovernanceResolutionOutcomeContinue {
		return "", fmt.Errorf("%w: governance rejection cannot authorize a continuation drive lease", common.ErrInvalidArgument)
	}
	kind := "decision_continuation"
	if resolution.ApprovalID != "" {
		kind = "approval_continuation"
	}
	fenceID := "runtime-governance/" + resolution.AuthoritySnapshotID + "/" + resolution.DispatchID
	return fmt.Sprintf(
		"lifecycle-owner-delivery/%s/%s/%s/%s/%s/%s/%s",
		kind,
		resolution.AuthoritySnapshotID,
		resolution.DispatchID,
		resolution.TicketID,
		resolution.ApprovalID,
		resolution.ResolutionID,
		fenceID,
	), nil
}

func (c *authorityComponent) commitGovernanceResolutionWithContinuationLease(
	ctx context.Context,
	resolution runtimecontract.RuntimeGovernanceResolution,
	continuationLease *runtimecontract.RuntimeDriveLease,
) (EvidenceCommitResult, error) {
	if c == nil || c.repository == nil {
		return EvidenceCommitResult{}, common.ErrInvalidArgument
	}
	resolution = resolution.Normalized()
	if err := resolution.Validate(); err != nil {
		return EvidenceCommitResult{}, err
	}
	resolutionHash := runtimecontract.StableRuntimeGovernanceResolutionHash(resolution)
	leaseTemplate, err := validateGovernanceContinuationDriveLeaseTemplate(resolution, continuationLease)
	if err != nil {
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
		switch persistedRuntimeGovernanceResolutionDisposition(aggregate, resolution) {
		case CommitDispositionDuplicate:
			return EvidenceCommitResult{Disposition: CommitDispositionDuplicate, Aggregate: aggregate}, nil
		case CommitDispositionStale:
			// A replay of an already-persisted stale diagnostic remains stale and
			// never enters lifecycle continuation delivery.
			return EvidenceCommitResult{Disposition: CommitDispositionStale, Aggregate: aggregate}, nil
		default:
			return EvidenceCommitResult{Aggregate: aggregate}, fmt.Errorf("%w: processed governance resolution payload does not match persisted history", common.ErrConflict)
		}
	}
	expectedRevision := aggregate.StateRevision
	if resolution.AuthoritySnapshotID != aggregate.Decision.AuthoritySnapshot.ID {
		aggregate.ProcessedEventIDs = appendNormalizedString(aggregate.ProcessedEventIDs, eventID)
		aggregate.Diagnostics = append(aggregate.Diagnostics, runtimecontract.RuntimeDiagnosticRecord{
			EventID: eventID, AuthoritySnapshotID: resolution.AuthoritySnapshotID,
			CurrentSnapshotID: aggregate.Decision.AuthoritySnapshot.ID, DispatchID: resolution.DispatchID,
			Reason: runtimeGovernanceStaleDiagnosticReason(resolution),
		})
		aggregate.StateRevision++
		if aggregate.DriveLease != nil && aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseScheduled {
			// A stale external resolution is diagnostic-only and must not strand an
			// already-authoritative continuation successor. Reissue the same stable
			// identity at the new aggregate revision; an older mounted timer may only
			// adopt this newer generation, never claim through its stale fence.
			lease := aggregate.DriveLease.Clone()
			lease.AggregateRevision = aggregate.StateRevision
			lease.RecoveryGeneration++
			aggregate.DriveLease = &lease
		}
		if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
			return EvidenceCommitResult{}, err
		}
		return EvidenceCommitResult{Disposition: CommitDispositionStale, Aggregate: aggregate}, nil
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.Candidate.Governance == nil {
		return EvidenceCommitResult{}, fmt.Errorf("%w: governance resolution does not match the active governed dispatch", common.ErrConflict)
	}
	dependency := aggregate.ActiveDispatch.Candidate.Governance.ExternalDependency
	inputReview := dependency.RequestedAction == runtimecontract.RuntimeValidationInputReviewInformationAction
	if inputReview && !runtimeInputReviewResolutionMatches(aggregate, aggregate.SemanticPlanning, resolution) {
		return EvidenceCommitResult{}, fmt.Errorf("%w: input-review resolution has no exact complete report", common.ErrConflict)
	}
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
	bound := runtimeGovernanceRequestBindsResolution(aggregate.GovernanceRequests, resolution)
	if !bound {
		return EvidenceCommitResult{}, fmt.Errorf("%w: governance resolution is not bound to the governed ticket", common.ErrConflict)
	}
	grantedBindingIndex := -1
	resolvedOperationBindingIndex := -1
	retireConfirmedRepairAuthority := false
	for idx := range aggregate.OperationGovernanceBindings {
		binding := aggregate.OperationGovernanceBindings[idx].Normalized()
		if !governancecheck.BindingMatchesResolution(binding, resolution) {
			continue
		}
		if resolvedOperationBindingIndex >= 0 {
			return EvidenceCommitResult{}, fmt.Errorf("%w: governance resolution matches multiple operation bindings", common.ErrConflict)
		}
		resolvedOperationBindingIndex = idx
		_, _, confirmedRepair, transactionErr := confirmedRepairGovernanceSourceTransaction(aggregate, binding)
		if transactionErr != nil {
			return EvidenceCommitResult{}, transactionErr
		}
		retireConfirmedRepairAuthority = confirmedRepair
		switch resolution.Outcome {
		case runtimecontract.RuntimeGovernanceResolutionOutcomeContinue:
			if !governancecheck.BindingStateAllowsOutcome(binding.State, resolution.Outcome) {
				return EvidenceCommitResult{}, fmt.Errorf("%w: governance continuation binding is not ticket_bound", common.ErrConflict)
			}
			binding.State = runtimecontract.RuntimeOperationGovernanceBindingGranted
			binding.Grant = &runtimecontract.RuntimeOperationGovernanceGrant{
				ResolutionID: resolution.ResolutionID, ResolutionHash: resolutionHash, TicketID: resolution.TicketID, ApprovalID: resolution.ApprovalID,
				ExecutionKey: binding.ExecutionKey, GovernanceFence: binding.Fence, ResumeFence: binding.Fence + 1,
				DependencyKind: binding.DependencyKind, SubjectRef: binding.SubjectRef,
				RequestedAction: binding.RequestedAction, RiskLevel: binding.RiskLevel, AuthorizationHash: binding.AuthorizationHash,
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
	fact, err := runtimecontract.RuntimeGovernanceEvidenceFactForResolution(resolution, aggregate.Facts.EvidenceRevision)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	switch resolution.Outcome {
	case runtimecontract.RuntimeGovernanceResolutionOutcomeContinue:
		aggregate.Facts.Evidence.Accepted = append(aggregate.Facts.Evidence.Accepted, fact)
		aggregate.Facts.Failure = nil
	case runtimecontract.RuntimeGovernanceResolutionOutcomeFail:
		aggregate.Facts.Evidence.Rejected = append(aggregate.Facts.Evidence.Rejected, fact)
		failure := runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassGovernanceRejected, Signature: string(resolution.ResolutionID), Message: resolution.Summary,
		}
		if err := replaceRuntimeFailureAuthority(&aggregate.Facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
			return EvidenceCommitResult{}, err
		}
	default:
		return EvidenceCommitResult{}, fmt.Errorf("unsupported governance resolution outcome %q", resolution.Outcome)
	}
	if retireConfirmedRepairAuthority {
		// The governance resolution either terminates this exact operation or
		// continues it from its immutable dispatch admission.  In neither case may
		// the pre-effect owner confirmation remain a second mutable target authority.
		aggregate.Facts.Resources.FailureTarget = nil
	}
	aggregate.Facts = aggregate.Facts.Normalized()
	aggregate.ProcessedEventIDs = appendNormalizedString(aggregate.ProcessedEventIDs, eventID)
	aggregate.GovernanceResolutions = append(aggregate.GovernanceResolutions, runtimecontract.RuntimeGovernanceResolutionRecordFromResolution(resolution))
	dispatch := *aggregate.ActiveDispatch
	dispatch.State = DispatchStateCommitted
	aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
	aggregate.ActiveDispatch = nil
	if inputReview && resolution.Outcome == runtimecontract.RuntimeGovernanceResolutionOutcomeContinue {
		if err := retireRuntimeInputReviewForInformation(&aggregate, resolution); err != nil {
			return EvidenceCommitResult{}, err
		}
	}
	if grantedBindingIndex < 0 {
		aggregate.Facts.RepairDirective = nil
	}
	var ownerReductionEvent *runtimeOwnerInternalReductionEvent
	if grantedBindingIndex >= 0 {
		binding := aggregate.OperationGovernanceBindings[grantedBindingIndex].Normalized()
		event, matched, eventErr := confirmedRepairGovernanceReductionEvent(aggregate, resolution, binding)
		if eventErr != nil {
			return EvidenceCommitResult{}, eventErr
		}
		if matched {
			ownerReductionEvent = event
		} else {
			for _, source := range aggregate.CompletedDispatches {
				if source.ID == binding.OperationDispatchID && source.ImplementationWorkCommitment != nil {
					ownerReductionEvent = &runtimeOwnerInternalReductionEvent{Kind: RuntimeInternalEventGovernanceResolutionObserved, GovernanceResolution: resolution, SourceDispatch: source, OperationBinding: binding}
				}
			}
		}
	}
	facts, decision, err := reduceRuntimeStateWithLineage(
		aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, ownerReductionEvent, &aggregate, aggregate.CoverageProofLineage,
	)
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
		resumeExecution, found := operationExecutionRecord(resume.OperationExecutions, binding.OperationID)
		if !found || resumeExecution.State != runtimecontract.OperationExecutionClaimed || resumeExecution.Fence != binding.Grant.ResumeFence {
			return EvidenceCommitResult{}, fmt.Errorf("governance resume operation has no exact claimed execution")
		}
		binding.Grant.ResumeDispatchID = resume.ID
		aggregate.OperationGovernanceBindings[grantedBindingIndex] = binding
		// Governance continuation authorizes a new physical owner effect. Reserve
		// its exact ExecutionKey/Fence budget in the same SM-3 CAS that publishes
		// the resume dispatch; the prior governance-required receipt has already
		// consumed the allocation for the earlier fence.
		if err := reserveRuntimeOperationExecutionBudget(&aggregate, resumeExecution, resumeExecution.Fence); err != nil {
			return EvidenceCommitResult{}, err
		}
		if resume.ImplementationWorkCommitment != nil {
			matched := false
			for i := range aggregate.ImplementationWork.Rounds {
				round := &aggregate.ImplementationWork.Rounds[i]
				if round.DispatchID != binding.OperationDispatchID {
					continue
				}
				if round.GovernanceReceiptHash != binding.GovernanceReceiptHash || round.CompletedReceiptSetHash != "" || round.FailedReceiptHash != "" || len(round.GovernanceDispatchIDs) >= runtimecontract.MaxRuntimeImplementationWorkRounds {
					return EvidenceCommitResult{}, fmt.Errorf("implementation governance continuation differs from suspended round")
				}
				round.GovernanceDispatchIDs = append(round.GovernanceDispatchIDs, round.DispatchID)
				round.DispatchID, round.DispatchAuthorityHash, round.GovernanceReceiptHash = resume.ID, resume.DispatchAuthorityHash, ""
				round.Hash = runtimecontract.StableRuntimeImplementationWorkRoundHash(*round)
				matched = true
			}
			if !matched {
				return EvidenceCommitResult{}, fmt.Errorf("implementation governance continuation has no suspended round")
			}
		}
		aggregate.ActiveDispatch = &resume
	}
	nextGeneration := uint64(1)
	if aggregate.DriveLease != nil {
		nextGeneration = aggregate.DriveLease.RecoveryGeneration + 1
	}
	aggregate.StateRevision++
	if leaseTemplate != nil {
		lease := leaseTemplate.Clone()
		lease.AggregateRevision = aggregate.StateRevision
		lease.RecoveryGeneration = nextGeneration
		lease.State = runtimecontract.RuntimeDriveLeaseScheduled
		aggregate.DriveLease = &lease
	}
	if err := aggregate.Validate(); err != nil {
		return EvidenceCommitResult{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return EvidenceCommitResult{}, err
	}
	return EvidenceCommitResult{Disposition: CommitDispositionCommitted, DispatchCompleted: true, Aggregate: aggregate}, nil
}

func persistedRuntimeGovernanceResolutionDisposition(aggregate RuntimeAggregate, resolution runtimecontract.RuntimeGovernanceResolution) CommitDisposition {
	resolution = resolution.Normalized()
	if aggregate.Facts.WorkspaceID != resolution.WorkspaceID || aggregate.Facts.RunID != resolution.RunID {
		return ""
	}
	matchedRequest := runtimeGovernanceRequestBindsResolution(aggregate.GovernanceRequests, resolution)
	wantRecord := runtimecontract.RuntimeGovernanceResolutionRecordFromResolution(resolution)
	matchedRecord := false
	for _, record := range aggregate.GovernanceResolutions {
		if runtimecontract.SameRuntimeGovernanceResolutionRecord(record, wantRecord) {
			matchedRecord = true
			break
		}
	}
	if matchedRequest && matchedRecord && persistedRuntimeGovernanceResolutionEvidenceMatches(aggregate, resolution) {
		return CommitDispositionDuplicate
	}
	for _, diagnostic := range aggregate.Diagnostics {
		if diagnostic.EventID == resolution.EventID() && strings.TrimSpace(diagnostic.AuthoritySnapshotID) == resolution.AuthoritySnapshotID &&
			strings.TrimSpace(diagnostic.DispatchID) == resolution.DispatchID && diagnostic.Reason == runtimeGovernanceStaleDiagnosticReason(resolution) {
			return CommitDispositionStale
		}
	}
	return ""
}

func runtimeGovernanceRequestBindsResolution(values []runtimecontract.RuntimeGovernanceRequestRecord, resolution runtimecontract.RuntimeGovernanceResolution) bool {
	for _, request := range values {
		if governancecheck.RequestBindsResolution(request, resolution) {
			return true
		}
	}
	return false
}

func persistedRuntimeGovernanceResolutionEvidenceMatches(aggregate RuntimeAggregate, resolution runtimecontract.RuntimeGovernanceResolution) bool {
	values := aggregate.Facts.Evidence.Accepted
	if resolution.Outcome == runtimecontract.RuntimeGovernanceResolutionOutcomeFail {
		values = aggregate.Facts.Evidence.Rejected
	}
	for _, fact := range values {
		if runtimecontract.RuntimeGovernanceEvidenceFactMatchesResolution(fact, resolution) {
			return true
		}
	}
	return false
}

func runtimeGovernanceStaleDiagnosticReason(resolution runtimecontract.RuntimeGovernanceResolution) string {
	resolution = resolution.Normalized()
	payload, _ := json.Marshal(resolution)
	hash := sha256.Sum256(payload)
	return fmt.Sprintf("governance resolution belongs to a stale authority snapshot; payload-sha256=%x", hash[:])
}

func validateGovernanceContinuationDriveLeaseTemplate(
	resolution runtimecontract.RuntimeGovernanceResolution,
	requested *runtimecontract.RuntimeDriveLease,
) (*runtimecontract.RuntimeDriveLease, error) {
	if requested == nil {
		return nil, fmt.Errorf("%w: governance resolution requires an atomic drive lease", common.ErrInvalidArgument)
	}
	wantSignature, err := RuntimeGovernanceResolutionDriveLeaseSignature(resolution)
	if err != nil {
		return nil, err
	}
	lease := requested.Normalized()
	if lease.WorkspaceID != resolution.WorkspaceID || lease.RunID != resolution.RunID ||
		lease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry || lease.FailureSignature != wantSignature ||
		lease.DueAt == "" || lease.AggregateRevision != 0 || lease.RecoveryGeneration != 0 || lease.State != "" {
		return nil, fmt.Errorf("%w: governance continuation drive lease template is not exact", common.ErrInvalidArgument)
	}
	probe := lease.Clone()
	probe.AggregateRevision = 1
	probe.RecoveryGeneration = 1
	probe.State = runtimecontract.RuntimeDriveLeaseScheduled
	if err := probe.Validate(); err != nil {
		return nil, err
	}
	return &lease, nil
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
	if source == nil || source.State != DispatchStateCommitted {
		return SealedDispatch{}, fmt.Errorf("governance resume source dispatch %q is missing", binding.OperationDispatchID)
	}
	sourceExecution, found := operationExecutionRecord(source.OperationExecutions, binding.OperationID)
	if !found {
		return SealedDispatch{}, fmt.Errorf("governance resume source execution does not match granted binding")
	}
	if err := sourceExecution.Validate(); err != nil {
		return SealedDispatch{}, fmt.Errorf("governance resume source execution: %w", err)
	}
	if containsString(source.CompletedOperationIDs, binding.OperationID) {
		return SealedDispatch{}, fmt.Errorf("governance resume operation %q is already completed", binding.OperationID)
	}
	effectiveActivation := runtimecontract.RuntimeActivationForFacts(aggregate.Facts)
	effectiveActivation.OperationPolicies = runtimecontract.RuntimeDispatchSealedOperationPolicies(sealedDispatchToRuntimeRecord(*source))
	preSealAuthority := RuntimeDispatchPreSealAuthority{
		ImplementationWorkCommitment: cloneImplementationWorkCommitment(source.ImplementationWorkCommitment),
		RepairCommitment:             cloneRuntimeDispatchRepairCommitment(source.RepairCommitment),
		AssessmentAuthorityInput:     cloneRuntimeAssessmentAuthorityInput(source.AssessmentAuthorityInput),
		GovernanceResume: &RuntimeGovernanceResumePreSealAuthority{
			Source:  *source,
			Binding: binding,
		},
	}
	if source.RepairCommitment != nil {
		preSealAuthority.RepairCommitmentOriginDispatchID = source.ID
	}
	resume, err := sealDispatchWithActivationAndFeasibilities(
		aggregate.Decision,
		source.Candidate,
		effectiveActivation,
		source.CandidateManifestHash,
		source.CoverageBindings,
		source.OperationTargetAuthorizations,
		source.ValidationRecipes,
		governanceResumeAuthorityFeasibilities(*source),
		source.OperationAdmissibilities,
		source.OperationAdmissibilityObservations,
		preSealAuthority,
	)
	if err != nil {
		return SealedDispatch{}, err
	}
	return resume, nil
}

func governanceResumeAuthorityFeasibilities(source SealedDispatch) []runtimecontract.RuntimeCandidateFeasibilityRecord {
	ret := make([]runtimecontract.RuntimeCandidateFeasibilityRecord, 0, len(source.OperationExecutions))
	for _, execution := range source.OperationExecutions {
		execution = execution.Normalized()
		if execution.AdmissibilitySnapshotID == "" {
			continue
		}
		observation := runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible}
		value := runtimecontract.RuntimeCandidateFeasibilityRecord{
			OperationID:             execution.OperationID,
			AdmissibilitySnapshotID: execution.AdmissibilitySnapshotID,
			AcquisitionBinding:      execution.AcquisitionBinding,
			Observation:             &observation,
		}
		if execution.ReplayProof != nil {
			proof := execution.ReplayProof.Clone()
			value.ReplayProof = &proof
		}
		ret = append(ret, value)
	}
	return ret
}
