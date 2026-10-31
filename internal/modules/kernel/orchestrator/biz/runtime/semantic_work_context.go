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
		for _, generation := range a.SemanticPlanning.AllHistory() {
			snapshot := generation.Snapshot
			if snapshot.Manifest == nil || snapshot.CoverageOrderDocument == nil {
				continue
			}
			for _, obligation := range snapshot.Manifest.ProofObligations {
				if prior := obligation.ImplementationWork; prior != nil && prior.PurposeChoiceOnly && prior.Scope.Request.RequestID == r.ScopeRequest.RequestID {
					round.Intent = snapshot.CoverageOrderDocument.ImplementationIntent.Clone()
				}
			}
		}
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
	for _, g := range a.SemanticPlanning.AllHistory() {
		if g.Snapshot.Manifest == nil || g.Snapshot.CoverageOrderDocument == nil {
			continue
		}
		for _, o := range g.Snapshot.Manifest.ProofObligations {
			if o.ImplementationWork != nil && o.ImplementationWork.Scope.Request.RequestID == work.Scope.Request.RequestID && g.Snapshot.CoverageOrderDocument.ImplementationIntent != nil {
				ret.Intent = g.Snapshot.CoverageOrderDocument.ImplementationIntent.Clone()
			}
		}
	}
	return ret, ret.Validate(m)
}
