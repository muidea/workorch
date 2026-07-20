package runtimev2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type DispatchState string

const (
	DispatchStateSealed      DispatchState = "sealed"
	DispatchStateCommitted   DispatchState = "committed"
	DispatchStateInvalidated DispatchState = "invalidated"
)

type SealedDispatch struct {
	ID                    string
	WorkspaceID           common.WorkspaceID
	RunID                 common.RunID
	Transaction           runtimecontract.AuthorityTransaction
	CandidateHash         string
	Candidate             runtimecontract.RuntimeCandidate
	State                 DispatchState
	CompletedOperationIDs []string
	// OperationExecutions is the per-operation claim/fence ledger (RVC-R9/R10).
	OperationExecutions []runtimecontract.RuntimeOperationExecutionRecord
}

type DispatchStepInput struct {
	DispatchID    string
	Transaction   runtimecontract.AuthorityTransaction
	CandidateHash string
	Candidate     runtimecontract.RuntimeCandidate
}

func (d SealedDispatch) StepInput() DispatchStepInput {
	return DispatchStepInput{DispatchID: d.ID, Transaction: d.Transaction, CandidateHash: d.CandidateHash, Candidate: d.Candidate}
}

type ExecutionEvidenceStatus = runtimecontract.RuntimeOperationExecutionStatus

const (
	ExecutionEvidenceStatusAccepted = runtimecontract.RuntimeOperationExecutionStatusAccepted
	ExecutionEvidenceStatusRejected = runtimecontract.RuntimeOperationExecutionStatusRejected
)

type ExecutionEvidence struct {
	EventID             string
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	DispatchID          string
	AuthoritySnapshotID string
	OperationID         string
	Kind                string
	Status              ExecutionEvidenceStatus
	SourceRef           string
	Failure             *runtimecontract.FailureFact
	Evidence            runtimecontract.RuntimeOperationEvidence
	// ObservedEffectScope / NoOp / OwnerReceipt are owner-reported facts (RVC-R9).
	// When present, evidence Kind is re-derived from sealed expectation + receipt.
	ObservedEffectScope runtimecontract.RuntimeEffectScope
	NoOp                bool
	OwnerReceiptRef     string
	OwnerReceiptHash    string
	Receipt             *runtimecontract.RuntimeOperationReceipt
}

type DiagnosticEvidence struct {
	EventID             string
	AuthoritySnapshotID string
	CurrentSnapshotID   string
	DispatchID          string
	Reason              string
}

type RuntimeAggregate struct {
	StateRevision               uint64
	EvidenceSequence            uint64
	Definition                  runtimecontract.RuntimeDefinitionContext
	Facts                       runtimecontract.RuntimeFacts
	Decision                    runtimecontract.RuntimeDecision
	ActiveDispatch              *SealedDispatch
	CompletedDispatches         []SealedDispatch
	ProcessedEventIDs           []string
	Diagnostics                 []DiagnosticEvidence
	CandidateAttempts           []runtimecontract.RuntimeCandidateAttemptRecord
	GovernanceRequests          []runtimecontract.RuntimeGovernanceRequestRecord
	GovernanceResolutions       []runtimecontract.RuntimeGovernanceResolutionRecord
	OperationGovernanceBindings []runtimecontract.RuntimeOperationGovernanceBinding
	OperationReceipts           []runtimecontract.RuntimeOperationReceipt
	PrerequisiteInvalidations   []runtimecontract.RuntimePrerequisiteInvalidationRecord
	// Termination is the SM-1 administrative cancel/supersede record (INV-08).
	Termination *runtimecontract.RuntimeTerminationRecord
}

type RuntimeStateRepository interface {
	Load(context.Context, common.WorkspaceID, common.RunID) (RuntimeAggregate, error)
	Save(context.Context, RuntimeAggregate, uint64) error
}

type CommitDisposition string

const (
	CommitDispositionCommitted CommitDisposition = "committed"
	CommitDispositionStale     CommitDisposition = "stale_diagnostic"
	CommitDispositionDuplicate CommitDisposition = "duplicate"
)

type EvidenceCommitResult struct {
	Disposition       CommitDisposition
	DispatchCompleted bool
	Aggregate         RuntimeAggregate
}

type DispatchCoordinator struct {
	repository RuntimeStateRepository
}

func NewDispatchCoordinator(repository RuntimeStateRepository) (*DispatchCoordinator, error) {
	if repository == nil {
		return nil, common.ErrInvalidArgument
	}
	return &DispatchCoordinator{repository: repository}, nil
}

func SealDispatch(decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate) (SealedDispatch, error) {
	return SealDispatchWithActivation(decision, candidate, runtimecontract.RuntimeActivationContext{})
}

// SealDispatchWithActivation seals a candidate using owner-sealed activation capability
// metadata for side-effect gating (MCP RiskLevel, etc.).
func SealDispatchWithActivation(decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate, activation runtimecontract.RuntimeActivationContext) (SealedDispatch, error) {
	if err := decision.Validate(); err != nil {
		return SealedDispatch{}, err
	}
	if err := candidate.ValidateWithActivation(decision, activation); err != nil {
		return SealedDispatch{}, err
	}
	transaction := decision.AuthorityTransaction()
	candidateHash := runtimecontract.StableRuntimeCandidateHash(candidate)
	digest := sha256.Sum256([]byte(strings.Join([]string{transaction.AuthoritySnapshotID, transaction.RequiredOutputHash, candidateHash}, "|")))
	dispatchID := "dispatch-" + hex.EncodeToString(digest[:])[:20]
	executions := sealOperationExecutionRecords(
		decision.AuthoritySnapshot.WorkspaceID,
		decision.AuthoritySnapshot.RunID,
		dispatchID,
		candidate,
		activation,
	)
	return SealedDispatch{
		ID:                  dispatchID,
		WorkspaceID:         decision.AuthoritySnapshot.WorkspaceID,
		RunID:               decision.AuthoritySnapshot.RunID,
		Transaction:         transaction,
		CandidateHash:       candidateHash,
		Candidate:           candidate,
		State:               DispatchStateSealed,
		OperationExecutions: executions,
	}, nil
}

func sealOperationExecutionRecords(
	workspaceID common.WorkspaceID,
	runID common.RunID,
	dispatchID string,
	candidate runtimecontract.RuntimeCandidate,
	activation runtimecontract.RuntimeActivationContext,
) []runtimecontract.RuntimeOperationExecutionRecord {
	ops := candidateRuntimeOperations(candidate)
	if len(ops) == 0 {
		return nil
	}
	ret := make([]runtimecontract.RuntimeOperationExecutionRecord, 0, len(ops))
	for _, operation := range ops {
		operation = operation.Normalized()
		scope := runtimecontract.ResolveExpectedEffectScope(operation, activation.MCPCapabilities)
		ret = append(ret, runtimecontract.RuntimeOperationExecutionRecord{
			OperationID:         operation.ID,
			ExecutionKey:        runtimecontract.StableRuntimeOperationExecutionKey(workspaceID, runID, dispatchID, operation.ID),
			ExpectedEffectScope: scope,
			State:               runtimecontract.OperationExecutionSealed,
		})
	}
	return runtimecontract.NormalizeRuntimeOperationExecutionRecords(ret)
}

func (d SealedDispatch) Validate(decision runtimecontract.RuntimeDecision) error {
	return d.ValidateWithActivation(decision, runtimecontract.RuntimeActivationContext{})
}

func (d SealedDispatch) ValidateWithActivation(decision runtimecontract.RuntimeDecision, activation runtimecontract.RuntimeActivationContext) error {
	if strings.TrimSpace(d.ID) == "" || d.WorkspaceID == "" || d.RunID == "" || strings.TrimSpace(d.CandidateHash) == "" {
		return fmt.Errorf("sealed dispatch requires id, scope and candidate hash")
	}
	if err := runtimecontract.ValidateAuthorityTransaction(decision, d.Transaction); err != nil {
		return err
	}
	if d.WorkspaceID != decision.AuthoritySnapshot.WorkspaceID || d.RunID != decision.AuthoritySnapshot.RunID {
		return fmt.Errorf("sealed dispatch scope mismatch")
	}
	if err := d.Candidate.ValidateWithActivation(decision, activation); err != nil {
		return err
	}
	if d.CandidateHash != runtimecontract.StableRuntimeCandidateHash(d.Candidate) {
		return fmt.Errorf("sealed dispatch candidate hash mismatch")
	}
	if d.State != DispatchStateSealed && d.State != DispatchStateCommitted && d.State != DispatchStateInvalidated {
		return fmt.Errorf("sealed dispatch has invalid state %q", d.State)
	}
	for _, operationID := range d.CompletedOperationIDs {
		if !candidateContainsOperation(d.Candidate, operationID) {
			return fmt.Errorf("sealed dispatch completed operation %q is not present in candidate", operationID)
		}
	}
	if err := runtimecontract.ValidateRuntimeOperationExecutionRecords(d.OperationExecutions); err != nil {
		return err
	}
	for _, exec := range runtimecontract.NormalizeRuntimeOperationExecutionRecords(d.OperationExecutions) {
		if !candidateContainsOperation(d.Candidate, exec.OperationID) {
			return fmt.Errorf("sealed dispatch operation execution %q is not present in candidate", exec.OperationID)
		}
	}
	return nil
}

func (c *DispatchCoordinator) CommitEvidence(ctx context.Context, evidence ExecutionEvidence) (EvidenceCommitResult, error) {
	if c == nil || c.repository == nil {
		return EvidenceCommitResult{}, common.ErrInvalidArgument
	}
	if err := evidence.Validate(); err != nil {
		return EvidenceCommitResult{}, err
	}
	aggregate, err := c.repository.Load(ctx, evidence.WorkspaceID, evidence.RunID)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	if err := aggregate.Validate(); err != nil {
		return EvidenceCommitResult{}, err
	}
	if containsString(aggregate.ProcessedEventIDs, evidence.EventID) {
		return EvidenceCommitResult{Disposition: CommitDispositionDuplicate, Aggregate: aggregate}, nil
	}
	expectedRevision := aggregate.StateRevision
	currentSnapshotID := aggregate.Decision.AuthoritySnapshot.ID
	if evidence.AuthoritySnapshotID != currentSnapshotID {
		aggregate.ProcessedEventIDs = appendNormalizedString(aggregate.ProcessedEventIDs, evidence.EventID)
		aggregate.Diagnostics = append(aggregate.Diagnostics, DiagnosticEvidence{
			EventID: evidence.EventID, AuthoritySnapshotID: evidence.AuthoritySnapshotID, CurrentSnapshotID: currentSnapshotID,
			DispatchID: evidence.DispatchID, Reason: "execution evidence belongs to a stale authority snapshot",
		})
		aggregate.StateRevision++
		if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
			return EvidenceCommitResult{}, err
		}
		return EvidenceCommitResult{Disposition: CommitDispositionStale, Aggregate: aggregate}, nil
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ID != evidence.DispatchID || aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID != evidence.AuthoritySnapshotID {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence does not match the active sealed dispatch")
	}
	if !candidateContainsOperation(aggregate.ActiveDispatch.Candidate, evidence.OperationID) {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence operation %q is not present in the sealed candidate", evidence.OperationID)
	}
	if evidence.Receipt != nil {
		persisted, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, evidence.Receipt.ExecutionKey, evidence.Receipt.Fence)
		if !found || !runtimecontract.SameRuntimeOperationReceipt(persisted, *evidence.Receipt) {
			return EvidenceCommitResult{}, fmt.Errorf("execution evidence owner receipt is not persisted")
		}
		execution, found := operationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, evidence.OperationID)
		if !found || execution.State != runtimecontract.OperationExecutionOwnerCommitted || execution.ExecutionKey != persisted.ExecutionKey {
			return EvidenceCommitResult{}, fmt.Errorf("execution evidence requires owner_committed execution state")
		}
	}
	var committedOperation runtimecontract.RuntimeOperation
	for _, operation := range candidateRuntimeOperations(aggregate.ActiveDispatch.Candidate) {
		if operation.Normalized().ID == evidence.OperationID {
			committedOperation = operation.Normalized()
			break
		}
	}
	descriptor, descriptorFound := runtimecontract.RuntimeOperationDescriptorForKind(committedOperation.Kind)
	if !descriptorFound {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence operation %q has no descriptor", committedOperation.Kind)
	}
	if descriptor.RequiresOwnerReceipt && evidence.Receipt == nil {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence operation %q requires persisted owner receipt", committedOperation.Kind)
	}
	expectedScope, hasExecution := expectedEffectScopeForOperation(*aggregate.ActiveDispatch, evidence.OperationID, aggregate.Facts.Activation)
	if !hasExecution {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence operation %q has no sealed execution record", evidence.OperationID)
	}
	evidenceKind, err := adjudicateEvidenceKind(aggregate.Decision, expectedScope, hasExecution, evidence)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	fact := runtimecontract.EvidenceFact{
		ID: common.EvidenceRecordID(evidence.EventID), Kind: evidenceKind, SubjectRef: strings.TrimSpace(evidence.OperationID),
		SourceRef: strings.TrimSpace(evidence.SourceRef), AuthoritySnapshotID: evidence.AuthoritySnapshotID,
		DispatchID: evidence.DispatchID, OperationID: evidence.OperationID, Revision: nextEvidenceRevision(aggregate.EvidenceSequence + 1),
		Summary: evidence.Evidence.Summary, ContentHash: evidence.Evidence.ContentHash,
		ExitCode: evidence.Evidence.ExitCode, EvidenceRefs: append([]string(nil), evidence.Evidence.EvidenceRefs...),
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), committedOperation.CoverageWorkItemIDs...),
	}
	// Mark operation execution as evidence_committed when ledger present.
	aggregate.ActiveDispatch.OperationExecutions = markOperationExecutionEvidenceCommitted(
		aggregate.ActiveDispatch.OperationExecutions, evidence.OperationID, evidence.OwnerReceiptRef, evidence.OwnerReceiptHash,
	)
	switch evidence.Status {
	case ExecutionEvidenceStatusAccepted:
		aggregate.Facts.Evidence.Accepted = append(aggregate.Facts.Evidence.Accepted, fact)
		if !aggregate.Facts.Focus.RequireAssessment {
			aggregate.Facts.Coverage = closeCoverageForEvidence(aggregate.Facts.Coverage, fact)
		}
		if fact.Kind == runtimecontract.EvidenceKindStateChange {
			currentCoverage := append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...)
			for _, focus := range aggregate.Facts.RemainingFocuses {
				currentCoverage = append(currentCoverage, focus.CoverageWorkItemIDs...)
			}
			aggregate.Facts.StateChange = aggregate.Facts.StateChange.ApplyAcceptedStateChangeEvidence(fact, currentCoverage)
		}
		// Accepted progress on the current focus invalidates prior non-satisfied assessments so
		// the same-focus loop can re-enter produce_assessment after repair evidence.
		if runtimecontract.EvidenceAppliesToCurrentFocus(aggregate.Facts, fact) {
			aggregate.Facts.Assessments = invalidateNonSatisfiedAssessmentsForFocus(aggregate.Facts.Assessments, aggregate.Facts.Focus.ID)
			aggregate.Facts.Failure = nil
			aggregate.Facts.Repair = aggregate.Facts.Repair.NoteProgress(fact.Revision)
			aggregate.Facts.Repair.ActiveGap = nil
		}
	case ExecutionEvidenceStatusRejected:
		if evidence.Failure != nil {
			fact.FailureClass = evidence.Failure.Class
			fact.FailureCode = evidence.Failure.Code
			fact.FailureSignature = evidence.Failure.Signature
			fact.FailureMessage = evidence.Failure.Message
			failure := *evidence.Failure
			aggregate.Facts.Failure = &failure
			gap := runtimecontract.RuntimeAssessmentGap{
				CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), fact.CoverageWorkItemIDs...),
				TargetRefs:          append([]string(nil), failure.TargetRefs...), Reason: failure.Message,
				FailureClass: failure.Class, FailureCode: failure.Code, FailureSignature: failure.Signature,
			}
			if len(gap.CoverageWorkItemIDs) == 0 {
				gap.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...)
			}
			if len(gap.TargetRefs) == 0 {
				gap.TargetRefs = append([]string(nil), aggregate.Facts.Focus.TargetRefs...)
			}
			switch fact.Kind {
			case runtimecontract.EvidenceKindValidation:
				gap.MissingEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
			default:
				gap.MissingEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
			}
			signature := strings.TrimSpace(failure.Signature)
			if signature == "" {
				signature = runtimecontract.StableAssessmentFailureSignature(aggregate.Facts.Focus.ID, runtimecontract.AssessmentDispositionRepairableGap, &gap)
				gap.FailureSignature = signature
				aggregate.Facts.Failure.Signature = signature
				fact.FailureSignature = signature
			}
			previousCount := repairFailureSignatureCount(aggregate.Facts.Repair, signature)
			cycleID := runtimecontract.StableRepairCycleID(string(aggregate.Facts.RunID), aggregate.Facts.Focus.ID, signature, evidence.EventID)
			ledger, _, cycleErr := aggregate.Facts.Repair.BeginRepairCycle(aggregate.Facts.Focus.ID, aggregate.Facts.Focus.SliceID, cycleID, signature, aggregate.Facts.ConvergencePolicy)
			if cycleErr != nil {
				return EvidenceCommitResult{}, cycleErr
			}
			ledger.ActiveGap = &gap
			if previousCount > 0 {
				ledger = ledger.NoteNoProgress()
			}
			aggregate.Facts.Repair = ledger
			if fact.Kind == runtimecontract.EvidenceKindStateChange && runtimecontract.RuntimeOperationRequiresCurrentTargetSnapshot(committedOperation) {
				aggregate.Facts = requireFreshTargetSnapshots(aggregate.Facts, committedOperation.TargetRefs)
			}
		}
		aggregate.Facts.Evidence.Rejected = append(aggregate.Facts.Evidence.Rejected, fact)
	default:
		return EvidenceCommitResult{}, fmt.Errorf("unsupported execution evidence status %q", evidence.Status)
	}
	aggregate.EvidenceSequence++
	aggregate.Facts.EvidenceRevision = nextEvidenceRevision(aggregate.EvidenceSequence)
	aggregate.Facts = aggregate.Facts.Normalized()
	aggregate.ProcessedEventIDs = appendNormalizedString(aggregate.ProcessedEventIDs, evidence.EventID)
	dispatch := *aggregate.ActiveDispatch
	dispatch.CompletedOperationIDs = appendNormalizedString(dispatch.CompletedOperationIDs, evidence.OperationID)
	freshnessInvalidated := false
	if evidence.Status == ExecutionEvidenceStatusAccepted && evidence.Receipt != nil {
		freshnessInvalidated, err = applyReceiptResourceChangeInvalidation(&aggregate, evidence.DispatchID, evidence.OperationID, *evidence.Receipt)
		if err != nil {
			return EvidenceCommitResult{}, err
		}
		// The accepted effect remains durable evidence, but an invalidated
		// multi-operation dispatch has not completed the sealed operation set.
		// Reopen its coverage binding so reducer cannot skip the refreshed
		// prerequisite and the remaining required operation(s).
		if freshnessInvalidated && len(dispatch.CompletedOperationIDs) < candidateOperationCount(dispatch.Candidate) {
			aggregate.Facts.Coverage = reopenCoverageForEvidence(aggregate.Facts.Coverage, fact)
			aggregate.Facts.FreshnessReacquireCoverage = append(aggregate.Facts.FreshnessReacquireCoverage, fact.CoverageWorkItemIDs...)
		} else if evidence.Status == ExecutionEvidenceStatusAccepted {
			aggregate.Facts.FreshnessReacquireCoverage = removeFreshnessReacquireCoverage(aggregate.Facts.FreshnessReacquireCoverage, fact.CoverageWorkItemIDs)
		}
	}
	dispatchCompleted := freshnessInvalidated || evidence.Status == ExecutionEvidenceStatusRejected || len(dispatch.CompletedOperationIDs) == candidateOperationCount(dispatch.Candidate)
	if dispatchCompleted {
		if freshnessInvalidated {
			dispatch.State = DispatchStateInvalidated
		} else {
			dispatch.State = DispatchStateCommitted
		}
		aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
		aggregate.ActiveDispatch = nil
		if !freshnessInvalidated {
			aggregate.Facts = advanceSatisfiedCoverageFocus(aggregate.Facts)
		}
		var previousSnapshot *runtimecontract.AuthoritySnapshot
		if !freshnessInvalidated {
			previousSnapshot = &aggregate.Decision.AuthoritySnapshot
		}
		facts, nextDecision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, previousSnapshot)
		if err != nil {
			return EvidenceCommitResult{}, err
		}
		aggregate.Facts = facts
		aggregate.Decision = nextDecision
	} else {
		aggregate.ActiveDispatch = &dispatch
	}
	aggregate.StateRevision++
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return EvidenceCommitResult{}, err
	}
	return EvidenceCommitResult{Disposition: CommitDispositionCommitted, DispatchCompleted: dispatchCompleted, Aggregate: aggregate}, nil
}

func repairFailureSignatureCount(ledger runtimecontract.RuntimeRepairLedger, signature string) int {
	for _, item := range ledger.SignatureCounts {
		if item.Signature == signature {
			return item.Count
		}
	}
	return 0
}

func requireFreshTargetSnapshots(facts runtimecontract.RuntimeFacts, targetRefs []string) runtimecontract.RuntimeFacts {
	targets := map[string]struct{}{}
	for _, ref := range targetRefs {
		ref = strings.TrimSpace(ref)
		if ref != "" {
			targets[ref] = struct{}{}
		}
	}
	if len(targets) == 0 {
		return facts
	}
	kept := make([]runtimecontract.TargetSnapshot, 0, len(facts.Resources.TargetSnapshots))
	for _, snapshot := range facts.Resources.TargetSnapshots {
		if _, refresh := targets[snapshot.TargetRef]; !refresh {
			kept = append(kept, snapshot)
		}
	}
	facts.Resources.TargetSnapshots = kept
	requirements := make([]RuntimeRequirement, 0, len(targets))
	for ref := range targets {
		requirements = append(requirements, RuntimeRequirement{
			Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: ref, Reason: "fresh target snapshot required after state-change conflict",
		})
	}
	return PreflightRuntimeFacts(facts, requirements)
}

func closeCoverageForEvidence(coverage []runtimecontract.RuntimeCoverageItemFact, evidence runtimecontract.EvidenceFact) []runtimecontract.RuntimeCoverageItemFact {
	ids := evidence.CoverageWorkItemIDs
	matched := map[common.CoverageWorkItemID]struct{}{}
	for _, id := range ids {
		matched[id] = struct{}{}
	}
	for idx := range coverage {
		if _, ok := matched[coverage[idx].ID]; ok {
			coverage[idx].State = runtimecontract.RuntimeCoverageItemStateSatisfied
			coverage[idx].EvidenceRefs = append(coverage[idx].EvidenceRefs, string(evidence.ID))
		}
	}
	return runtimecontract.NormalizeRuntimeCoverageFacts(coverage)
}

// reopenCoverageForEvidence keeps accepted evidence as lineage while making
// the work item available to the fresh transaction that must replace an
// invalidated partial dispatch. Only the matching evidence reference is
// removed; evidence itself is never rewritten or rejected.
func reopenCoverageForEvidence(coverage []runtimecontract.RuntimeCoverageItemFact, evidence runtimecontract.EvidenceFact) []runtimecontract.RuntimeCoverageItemFact {
	matched := map[common.CoverageWorkItemID]struct{}{}
	for _, id := range evidence.CoverageWorkItemIDs {
		matched[id] = struct{}{}
	}
	for idx := range coverage {
		if _, ok := matched[coverage[idx].ID]; !ok {
			continue
		}
		coverage[idx].State = runtimecontract.RuntimeCoverageItemStateOpen
		kept := make([]string, 0, len(coverage[idx].EvidenceRefs))
		for _, ref := range coverage[idx].EvidenceRefs {
			if ref != string(evidence.ID) {
				kept = append(kept, ref)
			}
		}
		coverage[idx].EvidenceRefs = kept
	}
	return runtimecontract.NormalizeRuntimeCoverageFacts(coverage)
}

func removeFreshnessReacquireCoverage(values []common.CoverageWorkItemID, completed []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	removed := map[common.CoverageWorkItemID]struct{}{}
	for _, id := range completed {
		removed[id] = struct{}{}
	}
	kept := make([]common.CoverageWorkItemID, 0, len(values))
	for _, id := range values {
		if _, ok := removed[id]; !ok {
			kept = append(kept, id)
		}
	}
	return kept
}

func (a RuntimeAggregate) Validate() error {
	if a.Facts.WorkspaceID == "" || a.Facts.RunID == "" {
		return fmt.Errorf("runtime aggregate requires workspace and run scope")
	}
	if err := a.Facts.Validate(); err != nil {
		return err
	}
	if err := a.Definition.Validate(); err != nil {
		return err
	}
	if a.Definition.Normalized().Revision != a.Facts.Normalized().DefinitionRevision {
		return fmt.Errorf("runtime aggregate definition revision mismatch")
	}
	if err := a.Decision.Validate(); err != nil {
		return err
	}
	if a.Decision.AuthoritySnapshot.WorkspaceID != a.Facts.WorkspaceID || a.Decision.AuthoritySnapshot.RunID != a.Facts.RunID {
		return fmt.Errorf("runtime aggregate decision scope mismatch")
	}
	if a.ActiveDispatch != nil {
		if err := a.ActiveDispatch.ValidateWithActivation(a.Decision, a.Facts.Activation); err != nil {
			return fmt.Errorf("runtime aggregate active dispatch: %w", err)
		}
	}
	if a.Termination != nil {
		if err := a.Termination.Validate(); err != nil {
			return fmt.Errorf("runtime aggregate termination: %w", err)
		}
	}
	if err := runtimecontract.ValidateRuntimeCandidateAttemptRecordsForScope(a.Facts.WorkspaceID, a.Facts.RunID, a.CandidateAttempts); err != nil {
		return err
	}
	if (isTerminalRuntimeAction(a.Decision.Action) || a.Decision.Terminal != nil) && hasPendingCandidateAttempt(a.CandidateAttempts) {
		return fmt.Errorf("terminal runtime aggregate cannot retain a pending candidate attempt")
	}
	if hasUnreducedCurrentCandidateRetryFailure(a.Decision, a.CandidateAttempts) {
		return fmt.Errorf("current runtime authority retains a failed candidate attempt 1 without reducer transition")
	}
	for _, attempt := range a.CandidateAttempts {
		if attempt.State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
			continue
		}
		matched := a.ActiveDispatch != nil && a.ActiveDispatch.Transaction == attempt.Transaction
		if !matched {
			for _, dispatch := range a.CompletedDispatches {
				if dispatch.Transaction == attempt.Transaction {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("accepted candidate attempt has no sealed dispatch")
		}
	}
	if err := runtimecontract.ValidateRuntimeGovernanceResolutionRecords(a.GovernanceResolutions); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeGovernanceRequestRecords(a.GovernanceRequests); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeOperationGovernanceBindings(a.OperationGovernanceBindings); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeOperationReceipts(a.OperationReceipts); err != nil {
		return err
	}
	for _, invalidation := range a.PrerequisiteInvalidations {
		if err := invalidation.Validate(); err != nil {
			return fmt.Errorf("runtime aggregate prerequisite invalidation: %w", err)
		}
	}
	for _, request := range a.GovernanceRequests {
		matched := a.ActiveDispatch != nil && a.ActiveDispatch.ID == request.DispatchID &&
			a.ActiveDispatch.Transaction.AuthoritySnapshotID == request.AuthoritySnapshotID && a.ActiveDispatch.Candidate.Governance != nil
		if !matched {
			for _, dispatch := range a.CompletedDispatches {
				if dispatch.ID == request.DispatchID && dispatch.Transaction.AuthoritySnapshotID == request.AuthoritySnapshotID && dispatch.Candidate.Governance != nil {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("runtime aggregate governance request has no governed dispatch")
		}
	}
	for _, rawBinding := range a.OperationGovernanceBindings {
		binding := rawBinding.Normalized()
		var source *SealedDispatch
		for idx := range a.CompletedDispatches {
			if a.CompletedDispatches[idx].ID == binding.OperationDispatchID {
				value := a.CompletedDispatches[idx]
				source = &value
				break
			}
		}
		if source == nil {
			return fmt.Errorf("operation governance binding source dispatch %q is not completed", binding.OperationDispatchID)
		}
		execution, found := operationExecutionRecord(source.OperationExecutions, binding.OperationID)
		if !found || execution.ExecutionKey != binding.ExecutionKey || execution.Fence != binding.Fence ||
			execution.OwnerReceiptHash != binding.GovernanceReceiptHash || execution.State != runtimecontract.OperationExecutionOwnerCommitted {
			return fmt.Errorf("operation governance binding source execution does not match sealed receipt")
		}
		receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(a.OperationReceipts, binding.ExecutionKey, binding.Fence)
		if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeGovernanceRequired || runtimecontract.StableRuntimeOperationReceiptHash(receipt) != binding.GovernanceReceiptHash {
			return fmt.Errorf("operation governance binding has no matching governance receipt")
		}
		if binding.State == runtimecontract.RuntimeOperationGovernanceBindingSuspended {
			continue
		}
		bound := false
		for _, request := range a.GovernanceRequests {
			if request.TicketID == binding.TicketID && request.DispatchID == binding.GovernanceDispatchID &&
				request.AuthoritySnapshotID == binding.GovernanceSnapshotID && request.DependencyKind == binding.DependencyKind && request.SubjectRef == binding.SubjectRef {
				bound = true
				break
			}
		}
		if !bound {
			return fmt.Errorf("operation governance binding has no matching ticket request")
		}
	}
	for _, resolution := range a.GovernanceResolutions {
		matched := false
		bound := false
		for _, request := range a.GovernanceRequests {
			if request.TicketID == resolution.TicketID && request.DispatchID == resolution.DispatchID && request.AuthoritySnapshotID == resolution.AuthoritySnapshotID {
				bound = true
				break
			}
		}
		for _, dispatch := range a.CompletedDispatches {
			if dispatch.ID == resolution.DispatchID && dispatch.Transaction.AuthoritySnapshotID == resolution.AuthoritySnapshotID &&
				dispatch.State == DispatchStateCommitted && dispatch.Candidate.Governance != nil {
				matched = true
				break
			}
		}
		if !matched || !bound {
			return fmt.Errorf("runtime aggregate governance resolution has no completed governed dispatch")
		}
	}
	return nil
}

func hasPendingCandidateAttempt(values []runtimecontract.RuntimeCandidateAttemptRecord) bool {
	for _, value := range values {
		if value.State == runtimecontract.RuntimeCandidateAttemptStatePending {
			return true
		}
	}
	return false
}

func hasUnreducedCurrentCandidateRetryFailure(decision runtimecontract.RuntimeDecision, values []runtimecontract.RuntimeCandidateAttemptRecord) bool {
	current := decision.AuthorityTransaction()
	for _, value := range values {
		if value.Attempt == 1 && value.State == runtimecontract.RuntimeCandidateAttemptStateFailed && runtimecontract.SameAuthorityContract(value.Transaction, current) {
			return true
		}
	}
	return false
}

func (e ExecutionEvidence) Validate() error {
	if strings.TrimSpace(e.EventID) == "" || e.WorkspaceID == "" || e.RunID == "" || strings.TrimSpace(e.DispatchID) == "" || strings.TrimSpace(e.AuthoritySnapshotID) == "" || strings.TrimSpace(e.OperationID) == "" {
		return fmt.Errorf("execution evidence requires event, scope, dispatch, snapshot and operation")
	}
	if e.Receipt == nil && strings.TrimSpace(e.Kind) == "" {
		return fmt.Errorf("execution evidence requires owner receipt or diagnostic kind")
	}
	if e.Receipt != nil {
		receipt := e.Receipt.Normalized()
		if err := receipt.Validate(); err != nil {
			return err
		}
		if receipt.OperationID != strings.TrimSpace(e.OperationID) {
			return fmt.Errorf("execution evidence receipt operation mismatch")
		}
	}
	if e.Status != ExecutionEvidenceStatusAccepted && e.Status != ExecutionEvidenceStatusRejected {
		return fmt.Errorf("execution evidence requires accepted or rejected status")
	}
	if e.Status == ExecutionEvidenceStatusRejected && e.Failure == nil {
		return fmt.Errorf("rejected execution evidence requires a failure fact")
	}
	return nil
}

func nextEvidenceRevision(sequence uint64) string {
	return fmt.Sprintf("evidence-v2-%020d", sequence)
}

func appendNormalizedString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || containsString(values, value) {
		return values
	}
	ret := append(append([]string(nil), values...), value)
	sort.Strings(ret)
	return ret
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func candidateContainsOperation(candidate runtimecontract.RuntimeCandidate, operationID string) bool {
	operationID = strings.TrimSpace(operationID)
	switch {
	case candidate.StateChange != nil:
		for _, operation := range candidate.StateChange.Operations {
			if operation.Operation.Normalized().ID == operationID {
				return true
			}
		}
	case candidate.Validation != nil:
		for _, operation := range candidate.Validation.Operations {
			if operation.Operation.Normalized().ID == operationID {
				return true
			}
		}
	}
	return false
}

func candidateOperationCount(candidate runtimecontract.RuntimeCandidate) int {
	switch {
	case candidate.StateChange != nil:
		return len(candidate.StateChange.Operations)
	case candidate.Validation != nil:
		return len(candidate.Validation.Operations)
	default:
		return 0
	}
}

// invalidateNonSatisfiedAssessmentsForFocus drops repairable/governance/terminal assessments
// for the active focus after new accepted evidence arrives. Satisfied assessments are retained
// as lineage; the next reduce will request a fresh assessment when required.
func invalidateNonSatisfiedAssessmentsForFocus(values []runtimecontract.RuntimeAssessmentFact, focusID string) []runtimecontract.RuntimeAssessmentFact {
	focusID = strings.TrimSpace(focusID)
	if focusID == "" || len(values) == 0 {
		return values
	}
	ret := make([]runtimecontract.RuntimeAssessmentFact, 0, len(values))
	for _, value := range values {
		if value.FocusID == focusID && value.Disposition != runtimecontract.AssessmentDispositionSatisfied {
			continue
		}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func expectedEffectScopeForOperation(dispatch SealedDispatch, operationID string, activation runtimecontract.RuntimeActivationContext) (runtimecontract.RuntimeEffectScope, bool) {
	operationID = strings.TrimSpace(operationID)
	for _, exec := range dispatch.OperationExecutions {
		if exec.Normalized().OperationID == operationID {
			return runtimecontract.NormalizeRuntimeEffectScope(exec.ExpectedEffectScope), true
		}
	}
	_ = activation
	return runtimecontract.RuntimeEffectUnknown, false
}

func adjudicateEvidenceKind(decision runtimecontract.RuntimeDecision, expected runtimecontract.RuntimeEffectScope, hasExpected bool, evidence ExecutionEvidence) (string, error) {
	if evidence.Receipt != nil {
		receipt := evidence.Receipt.Normalized()
		if !hasExpected {
			return "", fmt.Errorf("receipt evidence requires sealed expected effect")
		}
		// A successful owner call can still be rejected by the Orchestrator effect
		// gate (for example no-op under produce_state_change). In that case the
		// receipt remains authoritative and the rejected evidence records the
		// mismatch without pretending a durable effect occurred.
		if evidence.Status == ExecutionEvidenceStatusRejected &&
			(receipt.Outcome == runtimecontract.RuntimeOperationOutcomeAccepted || receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp) {
			if err := receipt.Validate(); err != nil {
				return "", fmt.Errorf("evidence authority: %w", err)
			}
			return evidenceKindForDecision(decision), nil
		}
		kind, err := runtimecontract.EvidenceKindFromReceipt(decision.Action, expected, receipt)
		if err != nil {
			return "", fmt.Errorf("evidence authority: %w", err)
		}
		return kind, nil
	}
	observed := runtimecontract.NormalizeRuntimeEffectScope(evidence.ObservedEffectScope)
	// Receipt-less evidence is only used for stale/diagnostic inputs. Current
	// sealed capability operations are rejected before this point when their
	// descriptor requires an owner receipt.
	if strings.TrimSpace(string(evidence.ObservedEffectScope)) == "" && !evidence.NoOp {
		kind := strings.TrimSpace(evidence.Kind)
		if kind == "" {
			kind = evidenceKindForDecision(decision)
		}
		if kind == "" {
			return "", fmt.Errorf("execution evidence requires kind or observed effect")
		}
		// Still reject durable mismatch when expected is known non-durable under validation.
		if decision.Action == runtimecontract.RuntimeActionProduceValidation && hasExpected && runtimecontract.RuntimeEffectIsDurable(expected) {
			return "", fmt.Errorf("validation dispatch sealed durable expected effect %q", expected)
		}
		return kind, nil
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey:        "evidence/" + evidence.EventID,
		OperationID:         evidence.OperationID,
		ObservedEffectScope: observed,
		NoOp:                evidence.NoOp,
		Evidence:            evidence.Evidence,
		Failure:             evidence.Failure,
	}
	switch evidence.Status {
	case ExecutionEvidenceStatusAccepted:
		if evidence.NoOp {
			receipt.Outcome = runtimecontract.RuntimeOperationOutcomeNoOp
		} else {
			receipt.Outcome = runtimecontract.RuntimeOperationOutcomeAccepted
		}
	case ExecutionEvidenceStatusRejected:
		receipt.Outcome = runtimecontract.RuntimeOperationOutcomeRejected
	}
	if !hasExpected {
		expected = observed
	}
	kind, err := runtimecontract.EvidenceKindFromReceipt(decision.Action, expected, receipt)
	if err != nil {
		return "", fmt.Errorf("evidence authority: %w", err)
	}
	return kind, nil
}

func markOperationExecutionEvidenceCommitted(values []runtimecontract.RuntimeOperationExecutionRecord, operationID, receiptRef, receiptHash string) []runtimecontract.RuntimeOperationExecutionRecord {
	if len(values) == 0 {
		return values
	}
	ret := append([]runtimecontract.RuntimeOperationExecutionRecord(nil), values...)
	for idx := range ret {
		if ret[idx].OperationID != operationID {
			continue
		}
		ret[idx].State = runtimecontract.OperationExecutionEvidenceCommitted
		if strings.TrimSpace(receiptRef) != "" {
			ret[idx].OwnerReceiptRef = strings.TrimSpace(receiptRef)
		}
		if strings.TrimSpace(receiptHash) != "" {
			ret[idx].OwnerReceiptHash = strings.TrimSpace(receiptHash)
		}
		// Ensure validation still passes: committed requires fence + receipt.
		if ret[idx].Fence == 0 {
			ret[idx].Fence = 1
		}
		if ret[idx].OwnerReceiptRef == "" {
			ret[idx].OwnerReceiptRef = "inline/" + operationID
		}
		if ret[idx].OwnerReceiptHash == "" {
			ret[idx].OwnerReceiptHash = "inline"
		}
	}
	return ret
}
