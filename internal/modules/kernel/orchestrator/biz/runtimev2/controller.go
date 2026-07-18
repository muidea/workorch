package runtimev2

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
		if !isTerminalRuntimeAction(existing.Decision.Action) {
			if err := runtimecontract.ValidateRuntimeOperationContractRevision(existing.Facts.Activation.ExecutionContract.OperationContractRevision); err != nil {
				return RuntimeAggregate{}, err
			}
			// The controller's direct callers do not own a persisted bootstrap.
			// Align their restart identity with the executable catalog only after
			// checking the persisted aggregate; this never writes a default into
			// existing authority.
			facts.Activation.ExecutionContract.OperationContractRevision = runtimecontract.RuntimeOperationContractRevision()
			facts = facts.Normalized()
		}
		if !sameRuntimeDefinition(existing.Facts, facts) || !runtimecontract.SameRuntimeDefinitionContext(existing.Definition, definition) {
			return RuntimeAggregate{}, fmt.Errorf("%w: runtime v2 state already exists with a different definition", common.ErrConflict)
		}
		return existing, nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return RuntimeAggregate{}, err
	}
	// Bootstrap construction is the only point that seals the operation catalog
	// compiled into this binary. Existing persisted aggregates are never
	// defaulted or rewritten.
	facts.Activation.ExecutionContract.OperationContractRevision = runtimecontract.RuntimeOperationContractRevision()
	facts = facts.Normalized()
	facts = PreflightRuntimeFacts(facts, requirements)
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
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

func isTerminalRuntimeAction(action runtimecontract.RuntimeAction) bool {
	return action == runtimecontract.RuntimeActionComplete || action == runtimecontract.RuntimeActionFail
}

func (c *RuntimeController) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.repository.Load(ctx, workspaceID, runID)
}

// RecordFeasibilityFact CAS-persists the pre-candidate capability gate and
// immediately re-reduces. A missing operation is therefore a formal SM-2 fact,
// never a provider error or consumed candidate attempt.
func (c *RuntimeController) RecordFeasibilityFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.RuntimeFeasibilityFact) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch != nil || (aggregate.Decision.Action != runtimecontract.RuntimeActionProduceStateChange && aggregate.Decision.Action != runtimecontract.RuntimeActionProduceValidation) {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not accept feasibility facts", common.ErrConflict)
	}
	fact.FocusID = strings.TrimSpace(fact.FocusID)
	if fact.FocusID == "" {
		fact.FocusID = aggregate.Facts.Focus.ID
	}
	if fact.FocusID != aggregate.Facts.Focus.ID || fact.Action != aggregate.Decision.Action || fact.Reason == "" {
		return RuntimeAggregate{}, fmt.Errorf("%w: feasibility fact does not match current authority", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	aggregate.Facts.Feasibility = &fact
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// RecordFailureFact persists an SM-2 failure input and lets the reducer choose
// repair, governance, or terminal. Drive/adapter errors must never project a
// Run terminal without this authority transition.
func (c *RuntimeController) RecordFailureFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.FailureFact) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || fact.Class == "" || strings.TrimSpace(fact.Message) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.Action == runtimecontract.RuntimeActionComplete || aggregate.Decision.Action == runtimecontract.RuntimeActionFail {
		return aggregate, nil
	}
	expectedRevision := aggregate.StateRevision
	fact.Code = strings.TrimSpace(fact.Code)
	fact.Signature = strings.TrimSpace(fact.Signature)
	fact.Message = strings.TrimSpace(fact.Message)
	aggregate.Facts.Failure = &fact
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// RequestTermination CAS-persists an administrative termination record on an active V2
// aggregate (INV-08). SM-1 may project cancelled/superseded only after this record exists.
func (c *RuntimeController) RequestTermination(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeTerminationKind, requestedBy, reason string) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || reason == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.Action == runtimecontract.RuntimeActionComplete || aggregate.Decision.Action == runtimecontract.RuntimeActionFail {
		return RuntimeAggregate{}, fmt.Errorf("%w: terminal runtime cannot accept administrative termination", common.ErrConflict)
	}
	if aggregate.Termination != nil {
		if aggregate.Termination.Kind == kind {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: termination already recorded as %s", common.ErrConflict, aggregate.Termination.Kind)
	}
	expectedRevision := aggregate.StateRevision
	term := runtimecontract.RuntimeTerminationRecord{
		Kind: kind, RequestedBy: strings.TrimSpace(requestedBy), Reason: strings.TrimSpace(reason),
		Revision: expectedRevision + 1, RequestedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := term.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Termination = &term
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
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
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, transaction)
	current := candidateAttemptsForContract(aggregate.CandidateAttempts, workspaceID, runID, contractID)
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
		if !ok || first.State != runtimecontract.RuntimeCandidateAttemptStateFailed || first.StablePrefixHash != stablePrefixHash || !runtimecontract.SameAuthorityContract(first.Transaction, transaction) {
			return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt 1 requires a failed same-contract attempt 0", common.ErrConflict)
		}
	}
	expectedRevision := aggregate.StateRevision
	aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{
		AuthorityContractID: contractID, Transaction: transaction, Attempt: attempt, StablePrefixHash: stablePrefixHash, State: runtimecontract.RuntimeCandidateAttemptStatePending,
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
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, record, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
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
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, _, ok := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !ok {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	expectedRevision := aggregate.StateRevision
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateFailed
	failureCopy := failure
	aggregate.CandidateAttempts[idx].Failure = &failureCopy
	createdTerminalFailure := false
	if failure.Class == runtimecontract.RuntimeCandidateFailureClassInternal {
		aggregate.Facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Signature: "candidate-internal", Message: failure.Reason}
		createdTerminalFailure = true
	} else if attempt == 1 {
		aggregate.Facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateExhausted, Signature: "candidate-retry-exhausted", Message: failure.Reason}
		createdTerminalFailure = true
	}
	if createdTerminalFailure {
		facts, decision, reduceErr := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
		if reduceErr != nil {
			return RuntimeAggregate{}, reduceErr
		}
		aggregate.Facts = facts
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
	disposition, err := runtimecontract.ValidateAssessmentDisposition(assessment.Disposition, assessment.Items, assessment.Gap)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	assessment.Disposition = disposition
	known := stringSet(runtimecontract.AcceptedEvidenceRefsForFocus(aggregate.Facts))
	if assessment.Disposition == runtimecontract.AssessmentDispositionSatisfied {
		for _, ref := range assessment.EvidenceRefs {
			if _, ok := known[ref]; !ok {
				return RuntimeAggregate{}, fmt.Errorf("assessment evidence ref %q is not accepted current-focus evidence", ref)
			}
		}
		if len(assessment.EvidenceRefs) == 0 {
			return RuntimeAggregate{}, fmt.Errorf("satisfied assessment requires accepted current-focus evidence refs")
		}
	}
	// Item facts must stay inside current focus/slice coverage.
	requiredCoverage := map[common.CoverageWorkItemID]struct{}{}
	for _, id := range aggregate.Facts.Focus.CoverageWorkItemIDs {
		requiredCoverage[id] = struct{}{}
	}
	for idx := range assessment.Items {
		item := &assessment.Items[idx]
		if len(requiredCoverage) > 0 {
			if _, ok := requiredCoverage[item.CoverageWorkItemID]; !ok {
				return RuntimeAggregate{}, fmt.Errorf("assessment item %q is outside current slice coverage", item.CoverageWorkItemID)
			}
		}
		if item.Satisfied {
			refs := item.EvidenceRefs
			if len(refs) == 0 {
				refs = assessment.EvidenceRefs
			}
			if len(refs) == 0 {
				return RuntimeAggregate{}, fmt.Errorf("satisfied assessment item %q requires current-focus evidence refs", item.CoverageWorkItemID)
			}
			for _, ref := range refs {
				if _, ok := known[ref]; !ok {
					return RuntimeAggregate{}, fmt.Errorf("assessment item %q evidence ref %q is not accepted current-focus evidence", item.CoverageWorkItemID, ref)
				}
			}
			item.EvidenceRefs = append([]string(nil), refs...)
		}
	}
	assessment.CoverageWorkItemIDs = runtimeAssessmentCoverageIDs(assessment.CoverageWorkItemIDs, aggregate.Facts.Focus.CoverageWorkItemIDs)
	aggregate.Facts.Coverage = applyAssessmentCoverage(aggregate.Facts.Coverage, assessment, aggregate.Facts.Focus.ID, aggregate.Facts.Focus.SliceID, aggregate.ActiveDispatch.ID)
	if assessment.Disposition == runtimecontract.AssessmentDispositionSatisfied {
		if open := openCoverageIDs(aggregate.Facts.Coverage, aggregate.Facts.Focus.CoverageWorkItemIDs); len(open) > 0 {
			assessment.Disposition = runtimecontract.AssessmentDispositionRepairableGap
			assessment.Gap = &runtimecontract.RuntimeAssessmentGap{
				CoverageWorkItemIDs: open,
				Reason:              "assessment did not close every current slice coverage item",
			}
		}
	}
	expectedRevision := aggregate.StateRevision
	fact := runtimecontract.RuntimeAssessmentFact{
		ID: aggregate.ActiveDispatch.ID, FocusID: assessment.FocusID, SliceID: firstNonEmpty(assessment.SliceID, aggregate.Facts.Focus.SliceID),
		Scope: aggregate.Facts.Focus.AssessmentScope, RequirementID: aggregate.Facts.Focus.AssessmentID,
		RequirementText: aggregate.Facts.Focus.AssessmentText, CoverageWorkItemIDs: assessment.CoverageWorkItemIDs,
		Disposition: assessment.Disposition, Items: append([]runtimecontract.RuntimeAssessmentItemFact(nil), assessment.Items...),
		Summary: assessment.Summary, EvidenceRefs: assessment.EvidenceRefs, ArtifactRefs: assessment.ArtifactRefs,
	}
	if assessment.Gap != nil {
		gap := assessment.Gap.Normalized()
		fact.Gap = &gap
	}
	aggregate.Facts.Assessments = append(aggregate.Facts.Assessments, fact)
	if assessment.Disposition == runtimecontract.AssessmentDispositionRepairableGap {
		signature := runtimecontract.StableAssessmentFailureSignature(aggregate.Facts.Focus.ID, assessment.Disposition, assessment.Gap)
		cycleID := runtimecontract.StableRepairCycleID(string(aggregate.Facts.RunID), aggregate.Facts.Focus.ID, signature, fact.ID)
		ledger, _, cycleErr := aggregate.Facts.Repair.BeginRepairCycle(aggregate.Facts.Focus.ID, aggregate.Facts.Focus.SliceID, cycleID, signature, aggregate.Facts.ConvergencePolicy)
		if cycleErr != nil {
			return RuntimeAggregate{}, cycleErr
		}
		if assessment.Gap != nil {
			gap := assessment.Gap.Normalized()
			ledger.ActiveGap = &gap
		}
		if assessment.Gap != nil {
			hasStateChangeGap := false
			hasValidationGap := false
			for _, kind := range assessment.Gap.MissingEvidenceKinds {
				if kind == runtimecontract.EvidenceKindStateChange {
					hasStateChangeGap = true
				}
				if kind == runtimecontract.EvidenceKindValidation {
					hasValidationGap = true
				}
			}
			if hasValidationGap && !hasStateChangeGap {
				ledger = ledger.MarkValidationOnlyRetry(aggregate.Facts.ConvergencePolicy)
			}
		}
		aggregate.Facts.Repair = ledger
	}
	if assessment.Disposition == runtimecontract.AssessmentDispositionSatisfied {
		aggregate.Facts.Repair = aggregate.Facts.Repair.NoteProgress(aggregate.Facts.EvidenceRevision)
	}
	aggregate.EvidenceSequence++
	aggregate.Facts.EvidenceRevision = nextEvidenceRevision(aggregate.EvidenceSequence)
	dispatch := *aggregate.ActiveDispatch
	dispatch.State = DispatchStateCommitted
	aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
	aggregate.ActiveDispatch = nil
	aggregate.Facts = advanceSatisfiedCoverageFocus(aggregate.Facts)
	facts, nextDecision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = nextDecision
	aggregate.StateRevision++
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func runtimeAssessmentCoverageIDs(ids []common.CoverageWorkItemID, fallback []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	if len(ids) == 0 {
		ids = fallback
	}
	seen := map[common.CoverageWorkItemID]struct{}{}
	ret := make([]common.CoverageWorkItemID, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ret = append(ret, id)
	}
	return ret
}

// applyAssessmentCoverage is deliberately ledger-first: an assessment can only
// close the concrete items it names, and later assessments cannot reopen a
// satisfied item. This preserves partial acceptance across repair/restart.
func applyAssessmentCoverage(coverage []runtimecontract.RuntimeCoverageItemFact, assessment runtimecontract.AssessmentCandidate, focusID, sliceID, assessmentID string) []runtimecontract.RuntimeCoverageItemFact {
	itemClaims := map[common.CoverageWorkItemID]runtimecontract.RuntimeAssessmentItemFact{}
	for _, item := range assessment.Items {
		itemClaims[item.CoverageWorkItemID] = item
	}
	for idx := range coverage {
		item := &coverage[idx]
		if item.SliceID != "" && sliceID != "" && item.SliceID != sliceID {
			continue
		}
		claim, named := itemClaims[item.ID]
		if !named && assessment.Disposition == runtimecontract.AssessmentDispositionSatisfied {
			for _, id := range assessment.CoverageWorkItemIDs {
				if id == item.ID {
					claim = runtimecontract.RuntimeAssessmentItemFact{CoverageWorkItemID: item.ID, Satisfied: true, EvidenceRefs: assessment.EvidenceRefs}
					named = true
					break
				}
			}
		}
		if named && claim.Satisfied && len(claim.EvidenceRefs) == 0 {
			claim.EvidenceRefs = append([]string(nil), assessment.EvidenceRefs...)
		}
		if !named || item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			continue
		}
		item.LastAssessmentID = assessmentID
		item.EvidenceRefs = append(item.EvidenceRefs, claim.EvidenceRefs...)
		if claim.Satisfied {
			item.State = runtimecontract.RuntimeCoverageItemStateSatisfied
		} else {
			item.State = runtimecontract.RuntimeCoverageItemStateUnsatisfied
		}
	}
	return runtimecontract.NormalizeRuntimeCoverageFacts(coverage)
}

func openCoverageIDs(coverage []runtimecontract.RuntimeCoverageItemFact, ids []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	state := map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItemState{}
	for _, item := range coverage {
		state[item.ID] = item.State
	}
	ret := make([]common.CoverageWorkItemID, 0, len(ids))
	for _, id := range ids {
		if state[id] != runtimecontract.RuntimeCoverageItemStateSatisfied {
			ret = append(ret, id)
		}
	}
	return ret
}

func sameRuntimeDefinition(left runtimecontract.RuntimeFacts, right runtimecontract.RuntimeFacts) bool {
	left = left.Normalized()
	right = right.Normalized()
	// Focus is the persisted SM-2 cursor and legitimately changes as coverage
	// advances. Restart identity is limited to immutable run definition and
	// activation fields; comparing the current cursor with bootstrap focus 0
	// would reject every multi-focus run after its first atomic advance.
	return left.WorkspaceID == right.WorkspaceID &&
		left.RunID == right.RunID &&
		left.DefinitionRevision == right.DefinitionRevision &&
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
