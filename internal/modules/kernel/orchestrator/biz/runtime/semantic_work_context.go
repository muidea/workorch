package runtime

import (
	"fmt"
	contract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// SemanticWorkContext compiles trusted original ledgers; no I/O, new plan,
// guessed completion, or domain-specific filename rules are involved.
func SemanticWorkContext(a RuntimeAggregate, m contract.RuntimeSemanticPlanManifest) (*contract.RuntimeSemanticWorkContext, error) {
	var work *contract.RuntimeSemanticImplementationWork
	for _, o := range m.ProofObligations {
		if o.ImplementationWork != nil && o.ImplementationWork.Scope.Content != nil {
			work = o.ImplementationWork
		}
	}
	if work == nil {
		return nil, nil
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	if a.SemanticPlanning.Manifest == nil || a.SemanticPlanning.Manifest.Hash != m.Hash || a.Facts.WorkspaceID != m.WorkspaceID || a.Facts.RunID != m.RunID {
		return nil, fmt.Errorf("semantic work source differs from original scope")
	}
	ret := &contract.RuntimeSemanticWorkContext{Content: work.Scope.Content.Clone()}
	for _, r := range a.ImplementationWork.Rounds {
		if r.CoverageWorkItemID != work.Scope.Request.CoverageWorkItemID || r.Ordinal >= work.Ordinal {
			continue
		}
		if r.CompletedReceiptSetHash == "" {
			return nil, fmt.Errorf("semantic work prior round is incomplete")
		}
		round := contract.RuntimeSemanticWorkRound{Ordinal: r.Ordinal, DispatchID: r.DispatchID, ReceiptSetHash: r.CompletedReceiptSetHash}
		intent, err := contract.ResolveRuntimeImplementationWorkIntent(r, a.SemanticPlanning)
		if err != nil {
			return nil, err
		}
		round.Intent = intent.Intent.Clone()

		for _, d := range a.CompletedDispatches {
			if d.ID != r.DispatchID {
				continue
			}
			for _, e := range d.OperationExecutions {
				op, found := candidateOperationByID(d.Candidate, e.OperationID)
				receipt, ok := contract.RuntimeOperationReceiptForExecutionKeyFence(a.OperationReceipts, e.ExecutionKey, e.Fence)
				if !found || !ok || e.State != contract.OperationExecutionEvidenceCommitted || e.OwnerReceiptHash != contract.StableRuntimeOperationReceiptHash(receipt) {
					return nil, fmt.Errorf("semantic work prior target lacks exact trusted receipt")
				}
				round.Targets = append(round.Targets, contract.RuntimeSemanticWorkTarget{OperationID: op.ID, TargetRefs: append([]string(nil), op.TargetRefs...), ReceiptRef: e.OwnerReceiptRef, ReceiptHash: e.OwnerReceiptHash, BeforeHash: receipt.BeforeHash, AfterHash: receipt.AfterHash, Changed: receipt.Changed})
			}
		}
		ret.PriorRounds = append(ret.PriorRounds, round)
	}
	if !work.PurposeChoiceOnly {
		matches := 0
		for _, r := range a.ImplementationWork.Rounds {
			if r.ScopeRequest.RequestID != work.Scope.Request.RequestID {
				continue
			}
			intent, err := contract.ResolveRuntimeImplementationWorkIntent(r, a.SemanticPlanning)
			if err != nil {
				return nil, err
			}
			ret.Intent = intent.Intent.Clone()
			matches++
		}
		if matches != 1 {
			return nil, fmt.Errorf("work context requires unique accepted current intent")
		}
	}

	return ret, ret.Validate(m)
}
