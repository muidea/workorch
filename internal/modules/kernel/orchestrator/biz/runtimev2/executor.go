package runtimev2

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeOperationExecutionRequest struct {
	WorkspaceID      common.WorkspaceID
	RunID            common.RunID
	DispatchID       string
	Transaction      runtimecontract.AuthorityTransaction
	Operation        runtimecontract.RuntimeOperation
	EvidenceKind     string
	EvidencePolicies []runtimecontract.RuntimeEvidencePolicy
}

type RuntimeOperationExecutionResult struct {
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       ExecutionEvidenceStatus
	SourceRef    string
	Failure      *runtimecontract.FailureFact
	Evidence     runtimecontract.RuntimeOperationEvidence
}

type RuntimeOperationExecutor interface {
	ExecuteRuntimeOperation(context.Context, RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error)
}

func (c *RuntimeController) ExecuteActiveDispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, executor RuntimeOperationExecutor) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || executor == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.State != DispatchStateSealed {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime has no active sealed dispatch", common.ErrConflict)
	}
	dispatch := *aggregate.ActiveDispatch
	operations, err := orderedCandidateOperations(dispatch.Candidate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	completed := stringSet(dispatch.CompletedOperationIDs)
	for _, operation := range operations {
		if _, ok := completed[operation.ID]; ok {
			continue
		}
		for _, dependency := range operation.DependsOn {
			if _, ok := completed[dependency]; !ok {
				return RuntimeAggregate{}, fmt.Errorf("operation %q dependency %q is not committed", operation.ID, dependency)
			}
		}
		result, executeErr := executor.ExecuteRuntimeOperation(ctx, RuntimeOperationExecutionRequest{
			WorkspaceID: workspaceID, RunID: runID, DispatchID: dispatch.ID,
			Transaction: dispatch.Transaction, Operation: operation, EvidenceKind: evidenceKindForDecision(aggregate.Decision),
			EvidencePolicies: aggregate.Facts.Activation.EvidencePolicies,
		})
		if executeErr != nil {
			return RuntimeAggregate{}, executeErr
		}
		if err := validateRuntimeOperationExecutionResult(aggregate.Decision, operation, result); err != nil {
			return RuntimeAggregate{}, err
		}
		commit, commitErr := c.CommitEvidence(ctx, ExecutionEvidence{
			EventID: result.EventID, WorkspaceID: workspaceID, RunID: runID,
			DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
			OperationID: operation.ID, Kind: result.EvidenceKind, Status: result.Status,
			SourceRef: result.SourceRef, Failure: result.Failure, Evidence: result.Evidence,
		})
		if commitErr != nil {
			return RuntimeAggregate{}, commitErr
		}
		aggregate = commit.Aggregate
		completed[operation.ID] = struct{}{}
		if commit.DispatchCompleted {
			return aggregate, nil
		}
	}
	return aggregate, nil
}

func evidenceKindForDecision(decision runtimecontract.RuntimeDecision) string {
	switch decision.Action {
	case runtimecontract.RuntimeActionProduceStateChange:
		return runtimecontract.EvidenceKindStateChange
	case runtimecontract.RuntimeActionProduceValidation:
		return runtimecontract.EvidenceKindValidation
	default:
		return ""
	}
}

func orderedCandidateOperations(candidate runtimecontract.RuntimeCandidate) ([]runtimecontract.RuntimeOperation, error) {
	operations := candidateRuntimeOperations(candidate)
	if len(operations) == 0 {
		return nil, fmt.Errorf("runtime dispatch candidate has no executable operations")
	}
	byID := make(map[string]runtimecontract.RuntimeOperation, len(operations))
	for _, value := range operations {
		operation := value.Normalized()
		if operation.ID == "" {
			return nil, fmt.Errorf("runtime operation requires id")
		}
		byID[operation.ID] = operation
	}
	completed := map[string]struct{}{}
	ret := make([]runtimecontract.RuntimeOperation, 0, len(byID))
	for len(ret) < len(byID) {
		ready := make([]string, 0)
		for id, operation := range byID {
			if _, ok := completed[id]; ok {
				continue
			}
			dependenciesReady := true
			for _, dependency := range operation.DependsOn {
				if _, ok := byID[dependency]; !ok {
					return nil, fmt.Errorf("runtime operation %q depends on unknown operation %q", id, dependency)
				}
				if _, ok := completed[dependency]; !ok {
					dependenciesReady = false
					break
				}
			}
			if dependenciesReady {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("runtime operation dependency graph contains a cycle")
		}
		sort.Strings(ready)
		for _, id := range ready {
			ret = append(ret, byID[id])
			completed[id] = struct{}{}
		}
	}
	return ret, nil
}

func candidateRuntimeOperations(candidate runtimecontract.RuntimeCandidate) []runtimecontract.RuntimeOperation {
	switch {
	case candidate.StateChange != nil:
		ret := make([]runtimecontract.RuntimeOperation, 0, len(candidate.StateChange.Operations))
		for _, operation := range candidate.StateChange.Operations {
			ret = append(ret, operation.Operation)
		}
		return ret
	case candidate.Validation != nil:
		ret := make([]runtimecontract.RuntimeOperation, 0, len(candidate.Validation.Operations))
		for _, operation := range candidate.Validation.Operations {
			ret = append(ret, operation.Operation)
		}
		return ret
	default:
		return nil
	}
}

func validateRuntimeOperationExecutionResult(decision runtimecontract.RuntimeDecision, operation runtimecontract.RuntimeOperation, result RuntimeOperationExecutionResult) error {
	if strings.TrimSpace(result.EventID) == "" || strings.TrimSpace(result.OperationID) != operation.ID {
		return fmt.Errorf("runtime operation result requires matching event and operation ids")
	}
	wantKind := ""
	switch decision.Action {
	case runtimecontract.RuntimeActionProduceStateChange:
		wantKind = runtimecontract.EvidenceKindStateChange
	case runtimecontract.RuntimeActionProduceValidation:
		wantKind = runtimecontract.EvidenceKindValidation
	default:
		return fmt.Errorf("runtime decision action %s cannot execute operations", decision.Action)
	}
	if strings.TrimSpace(result.EvidenceKind) != wantKind {
		return fmt.Errorf("runtime operation result evidence kind %q does not match action %s", result.EvidenceKind, decision.Action)
	}
	if result.Status != ExecutionEvidenceStatusAccepted && result.Status != ExecutionEvidenceStatusRejected {
		return fmt.Errorf("runtime operation result has invalid status %q", result.Status)
	}
	if result.Status == ExecutionEvidenceStatusRejected && result.Failure == nil {
		return fmt.Errorf("rejected runtime operation result requires failure facts")
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	ret := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			ret[value] = struct{}{}
		}
	}
	return ret
}
