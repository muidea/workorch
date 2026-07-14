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
	DispatchStateSealed    DispatchState = "sealed"
	DispatchStateCommitted DispatchState = "committed"
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
}

type DiagnosticEvidence struct {
	EventID             string
	AuthoritySnapshotID string
	CurrentSnapshotID   string
	DispatchID          string
	Reason              string
}

type RuntimeAggregate struct {
	StateRevision         uint64
	EvidenceSequence      uint64
	Definition            runtimecontract.RuntimeDefinitionContext
	Facts                 runtimecontract.RuntimeFacts
	Decision              runtimecontract.RuntimeDecision
	ActiveDispatch        *SealedDispatch
	CompletedDispatches   []SealedDispatch
	ProcessedEventIDs     []string
	Diagnostics           []DiagnosticEvidence
	CandidateAttempts     []runtimecontract.RuntimeCandidateAttemptRecord
	GovernanceRequests    []runtimecontract.RuntimeGovernanceRequestRecord
	GovernanceResolutions []runtimecontract.RuntimeGovernanceResolutionRecord
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
	if err := decision.Validate(); err != nil {
		return SealedDispatch{}, err
	}
	if err := candidate.Validate(decision); err != nil {
		return SealedDispatch{}, err
	}
	transaction := decision.AuthorityTransaction()
	candidateHash := runtimecontract.StableRuntimeCandidateHash(candidate)
	digest := sha256.Sum256([]byte(strings.Join([]string{transaction.AuthoritySnapshotID, transaction.RequiredOutputHash, candidateHash}, "|")))
	return SealedDispatch{
		ID:            "dispatch-" + hex.EncodeToString(digest[:])[:20],
		WorkspaceID:   decision.AuthoritySnapshot.WorkspaceID,
		RunID:         decision.AuthoritySnapshot.RunID,
		Transaction:   transaction,
		CandidateHash: candidateHash,
		Candidate:     candidate,
		State:         DispatchStateSealed,
	}, nil
}

func (d SealedDispatch) Validate(decision runtimecontract.RuntimeDecision) error {
	if strings.TrimSpace(d.ID) == "" || d.WorkspaceID == "" || d.RunID == "" || strings.TrimSpace(d.CandidateHash) == "" {
		return fmt.Errorf("sealed dispatch requires id, scope and candidate hash")
	}
	if err := runtimecontract.ValidateAuthorityTransaction(decision, d.Transaction); err != nil {
		return err
	}
	if d.WorkspaceID != decision.AuthoritySnapshot.WorkspaceID || d.RunID != decision.AuthoritySnapshot.RunID {
		return fmt.Errorf("sealed dispatch scope mismatch")
	}
	if err := d.Candidate.Validate(decision); err != nil {
		return err
	}
	if d.CandidateHash != runtimecontract.StableRuntimeCandidateHash(d.Candidate) {
		return fmt.Errorf("sealed dispatch candidate hash mismatch")
	}
	if d.State != DispatchStateSealed && d.State != DispatchStateCommitted {
		return fmt.Errorf("sealed dispatch has invalid state %q", d.State)
	}
	for _, operationID := range d.CompletedOperationIDs {
		if !candidateContainsOperation(d.Candidate, operationID) {
			return fmt.Errorf("sealed dispatch completed operation %q is not present in candidate", operationID)
		}
	}
	return nil
}

func (c *DispatchCoordinator) PersistDispatch(ctx context.Context, decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	dispatch, err := SealDispatch(decision, candidate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, dispatch.WorkspaceID, dispatch.RunID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.AuthoritySnapshot.ID != decision.AuthoritySnapshot.ID {
		return RuntimeAggregate{}, fmt.Errorf("cannot persist dispatch for stale authority snapshot")
	}
	if aggregate.ActiveDispatch != nil {
		if aggregate.ActiveDispatch.ID == dispatch.ID {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("authority snapshot already has a different active dispatch")
	}
	expectedRevision := aggregate.StateRevision
	aggregate.ActiveDispatch = &dispatch
	aggregate.StateRevision++
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
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
	fact := runtimecontract.EvidenceFact{
		ID: common.EvidenceRecordID(evidence.EventID), Kind: strings.TrimSpace(evidence.Kind), SubjectRef: strings.TrimSpace(evidence.OperationID),
		SourceRef: strings.TrimSpace(evidence.SourceRef), AuthoritySnapshotID: evidence.AuthoritySnapshotID,
		DispatchID: evidence.DispatchID, OperationID: evidence.OperationID, Revision: nextEvidenceRevision(aggregate.EvidenceSequence + 1),
		Summary: evidence.Evidence.Summary, ContentHash: evidence.Evidence.ContentHash,
		ExitCode: evidence.Evidence.ExitCode, EvidenceRefs: append([]string(nil), evidence.Evidence.EvidenceRefs...),
	}
	for _, operation := range candidateRuntimeOperations(aggregate.ActiveDispatch.Candidate) {
		if operation.Normalized().ID == evidence.OperationID {
			fact.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), operation.CoverageWorkItemIDs...)
			break
		}
	}
	switch evidence.Status {
	case ExecutionEvidenceStatusAccepted:
		aggregate.Facts.Evidence.Accepted = append(aggregate.Facts.Evidence.Accepted, fact)
	case ExecutionEvidenceStatusRejected:
		if evidence.Failure != nil {
			fact.FailureClass = evidence.Failure.Class
			fact.FailureCode = evidence.Failure.Code
			fact.FailureSignature = evidence.Failure.Signature
			fact.FailureMessage = evidence.Failure.Message
			failure := *evidence.Failure
			aggregate.Facts.Failure = &failure
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
	dispatchCompleted := evidence.Status == ExecutionEvidenceStatusRejected || len(dispatch.CompletedOperationIDs) == candidateOperationCount(dispatch.Candidate)
	if dispatchCompleted {
		dispatch.State = DispatchStateCommitted
		aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
		aggregate.ActiveDispatch = nil
		aggregate.Facts = advanceSatisfiedCoverageFocus(aggregate.Facts)
		nextDecision, err := ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
		if err != nil {
			return EvidenceCommitResult{}, err
		}
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
		if err := a.ActiveDispatch.Validate(a.Decision); err != nil {
			return fmt.Errorf("runtime aggregate active dispatch: %w", err)
		}
	}
	if err := runtimecontract.ValidateRuntimeCandidateAttemptRecords(a.CandidateAttempts); err != nil {
		return err
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

func (e ExecutionEvidence) Validate() error {
	if strings.TrimSpace(e.EventID) == "" || e.WorkspaceID == "" || e.RunID == "" || strings.TrimSpace(e.DispatchID) == "" || strings.TrimSpace(e.AuthoritySnapshotID) == "" || strings.TrimSpace(e.OperationID) == "" || strings.TrimSpace(e.Kind) == "" {
		return fmt.Errorf("execution evidence requires event, scope, dispatch, snapshot, operation and kind")
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
