package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"
)

// Every work receipt must prove the complete ordered owner revision chain.
// This invariant does not extend Profile permissions or provider vocabulary.

type implementationWorkResourceChain struct {
	dependencies []RuntimeFreshnessDependency
	seen         []map[string]bool
}

func newImplementationWorkResourceChain(dependencies []RuntimeFreshnessDependency) implementationWorkResourceChain {
	c := implementationWorkResourceChain{dependencies: slices.Clone(dependencies), seen: make([]map[string]bool, len(dependencies))}
	for i, dependency := range dependencies {
		c.seen[i] = map[string]bool{dependency.Revision: true}
	}
	return c
}

// Advance only exact owner/domain/subject revisions. A duplicate, fork, cycle,
// missing edge or unrelated change cannot stand in for the next original edge.
func (c *implementationWorkResourceChain) advance(receipt RuntimeOperationReceipt) bool {
	matched := false
	for i, dependency := range c.dependencies {
		var edge *RuntimeResourceChange
		for j := range receipt.ResourceChanges {
			change := &receipt.ResourceChanges[j]
			if RuntimeFreshnessDependencyIntersectsChange(dependency, *change) {
				if edge != nil {
					return false
				}
				edge = change
			}
		}
		if edge == nil {
			continue
		}
		if dependency.Revision == "" || edge.BeforeRevision != dependency.Revision || edge.AfterRevision == "" || c.seen[i][edge.AfterRevision] {
			return false
		}
		c.dependencies[i].Revision = edge.AfterRevision
		c.seen[i][edge.AfterRevision] = true
		matched = true
	}
	return matched
}

// RuntimeImplementationWorkDispositionForDispatchReceipt joins the sealed
// candidate order to persisted execution/receipt/evidence facts. Prior members
// are proved afresh; neither ledger order nor a caller-supplied revision cursor
// can authorize the current effect. It returns no completion or evidence credit.
func RuntimeImplementationWorkDispositionForDispatchReceipt(round RuntimeImplementationWorkRound, dispatch RuntimeDispatchRecord, receipts []RuntimeOperationReceipt, facts RuntimeFacts, governance []RuntimeOperationGovernanceBinding, receipt RuntimeOperationReceipt) (RuntimeImplementationWorkReceiptDisposition, error) {
	if round.WorkingBinding == nil {
		return "", fmt.Errorf("implementation work receipt requires the original working binding")
	}
	if err := round.Validate(); err != nil {
		return "", err
	}
	if err := receipt.Validate(); err != nil {
		return "", err
	}
	if dispatch.WorkspaceID != round.WorkspaceID || dispatch.RunID != round.RunID || facts.WorkspaceID != round.WorkspaceID || facts.RunID != round.RunID || dispatch.ID != round.DispatchID || dispatch.ImplementationWorkCommitment == nil || !reflect.DeepEqual(dispatch.ImplementationWorkCommitment.WorkingBinding, round.WorkingBinding) {
		return "", fmt.Errorf("implementation receipt chain has no exact original dispatch binding")
	}
	if err := ValidateRuntimeImplementationWorkDispatchCommitment(dispatch); err != nil {
		return "", err
	}
	if round.DispatchAuthorityHash != dispatch.DispatchAuthorityHash || dispatch.DispatchAuthorityHash != StableRuntimeDispatchAuthorityHash(dispatch) {
		return "", fmt.Errorf("implementation receipt chain differs from original dispatch authority")
	}
	operations := runtimeCandidateAdmissionOperations(dispatch.Candidate)
	index := -1
	for i, operation := range operations {
		if operation.ID == receipt.OperationID {
			if index >= 0 || !slices.Equal(operation.TargetRefs, receipt.TargetRefs) {
				return "", fmt.Errorf("implementation receipt chain operation/target is ambiguous")
			}
			index = i
		}
	}
	if index < 0 {
		return "", fmt.Errorf("implementation receipt is absent from sealed work order")
	}
	executionMatches := 0
	for _, execution := range dispatch.OperationExecutions {
		if execution.OperationID != receipt.OperationID {
			continue
		}
		if err := execution.Validate(); err != nil {
			return "", err
		}
		if execution.ExecutionKey != receipt.ExecutionKey || execution.Fence != receipt.Fence || execution.OperationHash != StableRuntimeOperationHash(operations[index]) || execution.OwnerReceiptHash != StableRuntimeOperationReceiptHash(receipt) || execution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey || execution.State != OperationExecutionOwnerCommitted && execution.State != OperationExecutionEvidenceCommitted {
			return "", fmt.Errorf("implementation receipt chain lost exact owner-committed execution")
		}
		catalog := round.WorkingBinding.OwnerCatalog
		if execution.ExpectedEffectScope != RuntimeEffectWorkspaceDurable || execution.AdmissibilitySnapshotID != catalog.SnapshotID || execution.AcquisitionBinding != catalog.AcquisitionBinding {
			return "", fmt.Errorf("implementation receipt chain execution changed original owner catalog")
		}
		origin, err := RuntimeOperationExecutionKeyOrigin(round.WorkspaceID, round.RunID, execution.ExecutionKey, execution.OperationID)
		if err != nil || origin != dispatch.ID && !slices.Contains(round.GovernanceDispatchIDs, origin) {
			return "", fmt.Errorf("implementation receipt chain execution belongs to another scope/dispatch")
		}
		if origin != dispatch.ID {
			grants := 0
			for _, binding := range governance {
				if binding.State == RuntimeOperationGovernanceBindingGranted && binding.Grant != nil && binding.ExecutionKey == execution.ExecutionKey && binding.OperationID == execution.OperationID && binding.Grant.ExecutionKey == execution.ExecutionKey && binding.Grant.ResumeFence == execution.Fence && binding.Grant.ResumeDispatchID == dispatch.ID {
					if err := binding.Grant.Validate(); err != nil {
						return "", err
					}
					grants++
				}
			}
			if grants != 1 {
				return "", fmt.Errorf("implementation receipt chain lacks unique original governance hop")
			}
		}
		executionMatches++
	}
	receiptMatches := 0
	for _, value := range receipts {
		if value.ExecutionKey == receipt.ExecutionKey && value.Fence == receipt.Fence {
			if !reflect.DeepEqual(value, receipt) {
				return "", fmt.Errorf("implementation receipt chain differs from original owner receipt")
			}
			receiptMatches++
		}
	}
	if executionMatches != 1 || receiptMatches != 1 {
		return "", fmt.Errorf("implementation receipt chain requires unique execution and owner receipt")
	}
	if disposition := implementationWorkReceiptOutcomeDisposition(receipt); disposition != "" {
		return disposition, nil
	}
	members, err := implementationWorkReceiptMembersBefore(dispatch, receipts, facts, governance, receipt.OperationID)
	if err != nil {
		return "", err
	}
	if len(members) != index {
		return "", fmt.Errorf("implementation receipt chain skips uncommitted original operations")
	}
	if len(members) > 0 {
		prefix := round.Clone()
		prefix.CompletedReceiptSetHash = ""
		prefix.Hash = StableRuntimeImplementationWorkRoundHash(prefix)
		if _, err := validateImplementationWorkReceiptMembers(prefix, members, nil, false); err != nil {
			return "", err
		}
	}
	chain := newImplementationWorkResourceChain(round.WorkingBinding.CatalogObservation.Dependencies)
	for _, member := range members {
		if !chain.advance(member.Receipt) {
			return "", fmt.Errorf("implementation receipt chain has an invalid persisted predecessor")
		}
	}
	if !chain.advance(receipt) {
		return RuntimeImplementationWorkReceiptInvalidationMissing, nil
	}
	return "", nil
}
