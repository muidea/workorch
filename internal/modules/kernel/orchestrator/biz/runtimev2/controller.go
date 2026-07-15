package runtimev2

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeController struct {
	repository RuntimeStateRepository
	dispatch   *DispatchCoordinator
}

func NewRuntimeController(repository RuntimeStateRepository) (*RuntimeController, error) {
	if repository == nil {
		return nil, common.ErrInvalidArgument
	}
	dispatch, err := NewDispatchCoordinator(repository)
	if err != nil {
		return nil, err
	}
	return &RuntimeController{repository: repository, dispatch: dispatch}, nil
}

// Initialize 建立一个 Run 唯一的 Runtime V2 aggregate。
// 已初始化的 Run 只允许幂等读取，不会因为 Start/Resume/restart 再次执行 reducer。
func (c *RuntimeController) Initialize(ctx context.Context, facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext, requirements []RuntimeRequirement) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	facts = facts.Normalized()
	definition = definition.Normalized()
	if err := facts.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := definition.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if definition.Revision != facts.DefinitionRevision {
		return RuntimeAggregate{}, fmt.Errorf("runtime definition context revision does not match facts")
	}
	existing, err := c.repository.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err == nil {
		if !sameRuntimeDefinition(existing.Facts, facts) || !runtimecontract.SameRuntimeDefinitionContext(existing.Definition, definition) {
			return RuntimeAggregate{}, fmt.Errorf("%w: runtime v2 state already exists with a different definition", common.ErrConflict)
		}
		return existing, nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return RuntimeAggregate{}, err
	}
	facts = PreflightRuntimeFacts(facts, requirements)
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate := RuntimeAggregate{
		StateRevision: 1,
		Definition:    definition,
		Facts:         facts,
		Decision:      decision,
	}
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, 0); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *RuntimeController) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.repository.Load(ctx, workspaceID, runID)
}

func (c *RuntimeController) CommitPrerequisite(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, result PrerequisiteResult) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not accept prerequisite results", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	facts, err := CommitPrerequisiteResult(aggregate.Facts, result)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	decision, err := ReduceRuntimeState(facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *RuntimeController) BeginCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || strings.TrimSpace(stablePrefixHash) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	switch aggregate.Decision.Action {
	case runtimecontract.RuntimeActionProduceStateChange, runtimecontract.RuntimeActionProduceValidation, runtimecontract.RuntimeActionProduceAssessment, runtimecontract.RuntimeActionRequestGovernance:
	default:
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not request a candidate", common.ErrConflict)
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	current := candidateAttemptsForSnapshot(aggregate.CandidateAttempts, transaction.AuthoritySnapshotID)
	for _, record := range current {
		if record.Attempt != attempt {
			continue
		}
		if record.State == runtimecontract.RuntimeCandidateAttemptStatePending && record.StablePrefixHash == stablePrefixHash && record.Transaction == transaction {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is already %s", common.ErrConflict, attempt, record.State)
	}
	if attempt == 0 {
		if len(current) != 0 {
			return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt 0 is no longer available", common.ErrConflict)
		}
	} else {
		first, ok := candidateAttemptRecord(current, 0)
		if !ok || first.State != runtimecontract.RuntimeCandidateAttemptStateFailed || first.StablePrefixHash != stablePrefixHash || first.Transaction != transaction {
			return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt 1 requires a failed same-contract attempt 0", common.ErrConflict)
		}
	}
	expectedRevision := aggregate.StateRevision
	aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{
		Transaction: transaction, Attempt: attempt, StablePrefixHash: stablePrefixHash, State: runtimecontract.RuntimeCandidateAttemptStatePending,
	})
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *RuntimeController) AcceptCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimecontract.RuntimeCandidate) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	idx, record, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, aggregate.Decision.AuthoritySnapshot.ID, attempt)
	if !ok {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	if err := runtimecontract.ValidateCandidateAgainstRejectedShapes(aggregate.Facts, sealedDispatchesToRuntimeRecords(aggregate.CompletedDispatches), candidate); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := runtimecontract.ValidateCandidateAgainstMissingValidationGap(aggregate.Facts, aggregate.Decision, candidate); err != nil {
		return RuntimeAggregate{}, err
	}
	dispatch, err := SealDispatchWithActivation(aggregate.Decision, candidate, aggregate.Facts.Activation)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if record.Transaction != dispatch.Transaction || aggregate.ActiveDispatch != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt does not match current dispatch state", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateAccepted
	aggregate.ActiveDispatch = &dispatch
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *RuntimeController) FailCandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, failure runtimecontract.RuntimeCandidateFailure) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) || failure.Class == "" || strings.TrimSpace(failure.Reason) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	idx, _, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, aggregate.Decision.AuthoritySnapshot.ID, attempt)
	if !ok {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	expectedRevision := aggregate.StateRevision
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateFailed
	failureCopy := failure
	aggregate.CandidateAttempts[idx].Failure = &failureCopy
	if failure.Class == runtimecontract.RuntimeCandidateFailureClassInternal {
		aggregate.Facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Signature: "candidate-internal", Message: failure.Reason}
	} else if attempt == 1 {
		aggregate.Facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateExhausted, Signature: "candidate-retry-exhausted", Message: failure.Reason}
	}
	if aggregate.Facts.Failure != nil {
		decision, reduceErr := ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
		if reduceErr != nil {
			return RuntimeAggregate{}, reduceErr
		}
		aggregate.Decision = decision
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func candidateAttemptsForSnapshot(values []runtimecontract.RuntimeCandidateAttemptRecord, snapshotID string) []runtimecontract.RuntimeCandidateAttemptRecord {
	ret := make([]runtimecontract.RuntimeCandidateAttemptRecord, 0, 2)
	for _, value := range values {
		if value.Transaction.AuthoritySnapshotID == snapshotID {
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

func pendingCandidateAttempt(values []runtimecontract.RuntimeCandidateAttemptRecord, snapshotID string, attempt int) (int, runtimecontract.RuntimeCandidateAttemptRecord, bool) {
	for idx, value := range values {
		if value.Transaction.AuthoritySnapshotID == snapshotID && value.Attempt == attempt && value.State == runtimecontract.RuntimeCandidateAttemptStatePending {
			return idx, value, true
		}
	}
	return -1, runtimecontract.RuntimeCandidateAttemptRecord{}, false
}

func (c *RuntimeController) CommitEvidence(ctx context.Context, evidence ExecutionEvidence) (EvidenceCommitResult, error) {
	if c == nil || c.dispatch == nil {
		return EvidenceCommitResult{}, common.ErrInvalidArgument
	}
	return c.dispatch.CommitEvidence(ctx, evidence)
}

func (c *RuntimeController) CommitAssessment(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.Action != runtimecontract.RuntimeActionProduceAssessment || aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.Candidate.Assessment == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime dispatch does not carry assessment", common.ErrConflict)
	}
	assessment := aggregate.ActiveDispatch.Candidate.Assessment.Normalized()
	if assessment.Satisfied {
		known := acceptedEvidenceRefsForFocus(aggregate.Facts)
		for _, ref := range assessment.EvidenceRefs {
			if _, ok := known[ref]; !ok {
				return RuntimeAggregate{}, fmt.Errorf("assessment evidence ref %q is not accepted current-focus evidence", ref)
			}
		}
		if len(assessment.EvidenceRefs) == 0 {
			return RuntimeAggregate{}, fmt.Errorf("satisfied assessment requires accepted current-focus evidence refs")
		}
	}
	expectedRevision := aggregate.StateRevision
	aggregate.Facts.Assessments = append(aggregate.Facts.Assessments, runtimecontract.RuntimeAssessmentFact{
		FocusID: assessment.FocusID, Scope: aggregate.Facts.Focus.AssessmentScope, RequirementID: aggregate.Facts.Focus.AssessmentID,
		RequirementText: aggregate.Facts.Focus.AssessmentText, CoverageWorkItemIDs: assessment.CoverageWorkItemIDs, Satisfied: assessment.Satisfied,
		Summary: assessment.Summary, EvidenceRefs: assessment.EvidenceRefs, ArtifactRefs: assessment.ArtifactRefs,
	})
	aggregate.EvidenceSequence++
	aggregate.Facts.EvidenceRevision = nextEvidenceRevision(aggregate.EvidenceSequence)
	dispatch := *aggregate.ActiveDispatch
	dispatch.State = DispatchStateCommitted
	aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
	aggregate.ActiveDispatch = nil
	aggregate.Facts = advanceSatisfiedCoverageFocus(aggregate.Facts)
	nextDecision, err := ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Decision = nextDecision
	aggregate.StateRevision++
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func acceptedEvidenceRefsForFocus(facts runtimecontract.RuntimeFacts) map[string]struct{} {
	ret := map[string]struct{}{}
	for _, evidence := range facts.Evidence.Accepted {
		if len(facts.Focus.RequiredEvidenceKinds) > 0 && !evidenceAppliesToCurrentFocus(facts, evidence) {
			continue
		}
		ret[string(evidence.ID)] = struct{}{}
		if evidence.SourceRef != "" {
			ret[evidence.SourceRef] = struct{}{}
		}
		for _, ref := range evidence.EvidenceRefs {
			ret[ref] = struct{}{}
		}
	}
	return ret
}

func sameRuntimeDefinition(left runtimecontract.RuntimeFacts, right runtimecontract.RuntimeFacts) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.WorkspaceID == right.WorkspaceID &&
		left.RunID == right.RunID &&
		left.DefinitionRevision == right.DefinitionRevision &&
		left.Focus.ID == right.Focus.ID &&
		runtimecontract.SameRuntimeActivationContext(left.Activation, right.Activation)
}

func sealedDispatchesToRuntimeRecords(values []SealedDispatch) []runtimecontract.RuntimeDispatchRecord {
	ret := make([]runtimecontract.RuntimeDispatchRecord, 0, len(values))
	for _, value := range values {
		ret = append(ret, runtimecontract.RuntimeDispatchRecord{
			ID: value.ID, WorkspaceID: value.WorkspaceID, RunID: value.RunID,
			Transaction: value.Transaction, CandidateHash: value.CandidateHash, Candidate: value.Candidate,
			State: runtimecontract.RuntimeDispatchState(value.State), CompletedOperationIDs: append([]string(nil), value.CompletedOperationIDs...),
		})
	}
	return ret
}
