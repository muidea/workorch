package runtime

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// recordCandidateConformance is the only SM-3 transition that persists a
// capability-owner candidate-shape fact. A nonconformant fact and the
// Orchestrator-owned failure/correction disposition are committed in the same
// aggregate CAS, so restart cannot observe an owner fact without its bounded
// attempt outcome.
func (c *candidateDispatchComponent) recordCandidateConformance(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, record runtimecontract.RuntimeCandidateConformanceRecord) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	record = record.Clone()
	if err := runtimecontract.ValidateRuntimeCandidateConformanceRecords([]runtimecontract.RuntimeCandidateConformanceRecord{record}); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if duplicateCandidateConformance(aggregate.CandidateAttempts, attempt, record) {
		return aggregate, nil
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, attemptRecord, pending := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !pending || attemptRecord.Proposal == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d has no pending proposal for conformance", common.ErrConflict, attempt)
	}
	if record.LogicalRequestID != attemptRecord.LogicalRequestID {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate conformance logical request does not match the pending proposal", common.ErrConflict)
	}
	operation, operationIndex, found := candidateOperationAndIndexByID(*attemptRecord.Proposal, record.OperationID)
	if !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate conformance operation %q is absent from the persisted proposal", common.ErrConflict, record.OperationID)
	}
	manifest, err := persistedCandidateAttemptManifest(aggregate, attemptRecord)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	authorization, found := runtimeCandidateOperationAuthorization(manifest, operation.Kind)
	if !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate conformance operation %q has no sealed authorization", common.ErrConflict, operation.Kind)
	}
	request := runtimecontract.RuntimeOperationCandidateConformanceRequest{
		WorkspaceID: workspaceID, RunID: runID, Transaction: attemptRecord.Transaction,
		CandidateManifestHash: manifest.Hash, Operation: operation, OperationAuthorization: authorization,
	}
	if err := record.Observation.ValidateForRequest(request, authorization.OwnerModuleID); err != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate conformance observation is not bound to the persisted proposal: %v", common.ErrConflict, err)
	}
	for _, persisted := range attemptRecord.Conformances {
		if strings.TrimSpace(persisted.OperationID) != record.OperationID {
			continue
		}
		if !reflect.DeepEqual(persisted.Clone(), record) {
			return RuntimeAggregate{}, fmt.Errorf("%w: candidate conformance result for operation %q changed", common.ErrConflict, record.OperationID)
		}
		return aggregate, nil
	}

	expectedRevision := aggregate.StateRevision
	aggregate.CandidateAttempts[idx].Conformances = append(aggregate.CandidateAttempts[idx].Conformances, record.Clone())
	switch record.Observation.Outcome {
	case runtimecontract.RuntimeOperationCandidateConformant:
		// The persisted fact is the restart fence; ordinary owner feasibility
		// remains a separate later read lane.
	case runtimecontract.RuntimeOperationCandidateNonconformant:
		violation := record.Observation.Violation
		if violation == nil {
			return RuntimeAggregate{}, fmt.Errorf("%w: nonconformant candidate observation has no violation", common.ErrConflict)
		}
		failure := runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassContract,
			Code:  "candidate_owner_constraint_nonconformant", Signature: "candidate-owner-constraint-nonconformant",
			Reason: "candidate parameter does not conform to its sealed capability-owner constraint",
			Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
				OperationID: operation.ID,
				Path:        fmt.Sprintf("$.operations[%d].parameters[name=%s]", operationIndex, violation.ParameterName),
				Actual:      string(violation.ActualShape), ConstraintRef: violation.ConstraintRef,
				RequiredShape: string(violation.RequiredShape), ManifestHash: manifest.Hash,
				Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionConformOwnerConstraint},
			},
		}
		sealed, correction, sealErr := sealCandidateRejectionWithFrontier(&failure, manifest, candidateCorrectionFrontier{candidate: attemptRecord.Proposal})
		if sealErr != nil || sealed == nil || correction == nil {
			return RuntimeAggregate{}, fmt.Errorf("%w: seal owner candidate conformance rejection: %v", common.ErrConflict, sealErr)
		}
		if err := closeCandidateAttemptWithCorrection(&aggregate, idx, attempt, *sealed, correction); err != nil {
			return RuntimeAggregate{}, err
		}
	case runtimecontract.RuntimeOperationCandidateContractDefect:
		failure := runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassInternal,
			Code:  "runtime_contract_defect", Signature: "owner-candidate-conformance-contract-defect",
			Reason: "capability owner could not evaluate its sealed candidate conformance contract",
		}
		if err := closeCandidateAttempt(&aggregate, idx, attempt, failure); err != nil {
			return RuntimeAggregate{}, err
		}
	default:
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func duplicateCandidateConformance(attempts []runtimecontract.RuntimeCandidateAttemptRecord, attempt int, record runtimecontract.RuntimeCandidateConformanceRecord) bool {
	for _, historical := range attempts {
		if historical.Attempt != attempt {
			continue
		}
		for _, persisted := range historical.Conformances {
			if reflect.DeepEqual(persisted.Clone(), record.Clone()) {
				return true
			}
		}
	}
	return false
}

func candidateOperationAndIndexByID(candidate runtimecontract.RuntimeCandidate, operationID string) (runtimecontract.RuntimeOperation, int, bool) {
	operationID = strings.TrimSpace(operationID)
	index := 0
	if candidate.StateChange != nil {
		for _, value := range candidate.StateChange.Operations {
			operation := value.Operation.Normalized()
			if operation.ID == operationID {
				return operation, index, true
			}
			index++
		}
	}
	if candidate.Validation != nil {
		for _, value := range candidate.Validation.Operations {
			operation := value.Operation.Normalized()
			if operation.ID == operationID {
				return operation, index, true
			}
			index++
		}
	}
	return runtimecontract.RuntimeOperation{}, 0, false
}

func persistedCandidateAttemptManifest(aggregate RuntimeAggregate, attempt runtimecontract.RuntimeCandidateAttemptRecord) (runtimecontract.RuntimeCandidateContractManifest, error) {
	_, session, found := runtimeCandidateRequestSessionByID(aggregate.CandidateRequestSessions, attempt.LogicalRequestID)
	if !found || session.CandidateManifest == nil || session.ManifestHash != attempt.ManifestHash {
		return runtimecontract.RuntimeCandidateContractManifest{}, fmt.Errorf("%w: candidate attempt has no persisted manifest body", common.ErrConflict)
	}
	manifest := session.CandidateManifest.Clone()
	if manifest.Hash != attempt.ManifestHash || manifest.Hash != runtimecontract.StableRuntimeCandidateContractManifestHash(manifest) {
		return runtimecontract.RuntimeCandidateContractManifest{}, fmt.Errorf("%w: candidate attempt manifest body is stale", common.ErrConflict)
	}
	return manifest, nil
}

func runtimeCandidateOperationAuthorization(manifest runtimecontract.RuntimeCandidateContractManifest, kind string) (runtimecontract.RuntimeCandidateOperationManifest, bool) {
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, operation := range manifest.Normalized().Operations {
		if operation.Kind == kind {
			return operation, true
		}
	}
	return runtimecontract.RuntimeCandidateOperationManifest{}, false
}

func validateCandidateConformanceAdmission(attempt runtimecontract.RuntimeCandidateAttemptRecord, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) error {
	operations := make([]runtimecontract.RuntimeOperation, 0)
	if candidate.StateChange != nil {
		for _, value := range candidate.StateChange.Operations {
			operations = append(operations, value.Operation.Normalized())
		}
	}
	if candidate.Validation != nil {
		for _, value := range candidate.Validation.Operations {
			operations = append(operations, value.Operation.Normalized())
		}
	}
	for _, operation := range operations {
		authorization, found := runtimeCandidateOperationAuthorization(manifest, operation.Kind)
		if !found {
			return fmt.Errorf("operation %q has no sealed authorization", operation.Kind)
		}
		_, constraints, err := runtimecontract.RuntimeOperationCandidateConformanceConstraintsForOperation(operation, authorization)
		if err != nil {
			return err
		}
		if len(constraints) == 0 {
			continue
		}
		matched := false
		for _, record := range attempt.Conformances {
			observation := record.Observation.Normalized()
			if record.OperationID == operation.ID && record.LogicalRequestID == attempt.LogicalRequestID && record.CandidateManifestHash == manifest.Hash && observation.Outcome == runtimecontract.RuntimeOperationCandidateConformant {
				request := runtimecontract.RuntimeOperationCandidateConformanceRequest{
					WorkspaceID: "candidate-admission", RunID: "candidate-admission",
					Transaction: attempt.Transaction, CandidateManifestHash: manifest.Hash,
					Operation: operation, OperationAuthorization: authorization,
				}
				// AuthorityTransaction does not own workspace/run fields. Scope is
				// checked by the transition before this pure admission helper; use
				// non-empty sentinels only for the shared request shape fence.
				if err := observation.ValidateForRequest(request, authorization.OwnerModuleID); err != nil {
					return err
				}
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("operation %q lacks a conformant owner candidate fact", operation.ID)
		}
	}
	return nil
}
