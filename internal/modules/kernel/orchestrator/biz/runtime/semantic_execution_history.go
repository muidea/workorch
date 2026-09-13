package runtime

import (
	"fmt"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// SemanticExecutionHistory projects the bounded suffix of the existing receipt
// ledger, including successful work from CWIs outside the current proof batch.
// Bound CWIs describe historical operation bindings, never accepted coverage.
func SemanticExecutionHistory(a RuntimeAggregate, c runtimecontract.RuntimeSemanticPlanRequestContext, m runtimecontract.RuntimeSemanticPlanManifest) (runtimecontract.RuntimeSemanticExecutionHistory, error) {
	invalid := runtimecontract.RuntimeSemanticExecutionHistory{}
	if err := a.Validate(); err != nil {
		return invalid, err
	}
	if c.WorkspaceID != a.Facts.WorkspaceID || c.RunID != a.Facts.RunID || c.Definition.Revision != a.Definition.Revision || c.Definition.StartupInputHash != a.Definition.StartupInputHash ||
		c.DefinitionSeed.Hash != a.Facts.DefinitionSeed.Hash || a.SemanticPlanning.Manifest == nil || m.Hash != a.SemanticPlanning.Manifest.Hash ||
		c.CoverageProgress.SourceStateRevision > a.StateRevision || c.CoverageProgress.EvidenceRevision != a.Facts.EvidenceRevision {
		return invalid, fmt.Errorf("semantic execution history aggregate scope or source mismatch")
	}
	if err := m.Validate(); err != nil {
		return invalid, err
	}
	if err := c.CoverageProgress.Validate(c, m); err != nil {
		return invalid, err
	}
	type receiptIdentity struct {
		key   string
		fence uint64
	}
	type source struct {
		dispatch  SealedDispatch
		operation runtimecontract.RuntimeOperation
		execution runtimecontract.RuntimeOperationExecutionRecord
	}
	sources := make(map[receiptIdentity]source)
	dispatches := append([]SealedDispatch(nil), a.CompletedDispatches...)
	if a.ActiveDispatch != nil {
		dispatches = append(dispatches, *a.ActiveDispatch)
	}
	for _, d := range dispatches {
		for _, execution := range d.OperationExecutions {
			if execution.State != runtimecontract.OperationExecutionOwnerCommitted && execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
				continue
			}
			op, found := candidateOperationByID(d.Candidate, execution.OperationID)
			if !found || d.WorkspaceID != c.WorkspaceID || d.RunID != c.RunID || d.Transaction.DefinitionRevision != c.Definition.Revision {
				return invalid, fmt.Errorf("semantic execution history sealed operation scope mismatch")
			}
			id := receiptIdentity{execution.ExecutionKey, execution.Fence}
			if _, exists := sources[id]; exists {
				return invalid, fmt.Errorf("semantic execution history has ambiguous receipt source")
			}
			sources[id] = source{d, op, execution}
		}
	}
	if len(sources) != len(a.OperationReceipts) {
		return invalid, fmt.Errorf("semantic execution history receipt/commit inventory mismatch")
	}
	start := max(0, len(a.OperationReceipts)-runtimecontract.RuntimeSemanticExecutionHistoryLimit)
	var observations []runtimecontract.RuntimeSemanticReceiptObservation
	for i, receipt := range a.OperationReceipts {
		s, found := sources[receiptIdentity{receipt.ExecutionKey, receipt.Fence}]
		hash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
		if !found || s.operation.ID != receipt.OperationID || s.execution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey || s.execution.OwnerReceiptHash != hash {
			return invalid, fmt.Errorf("semantic execution history receipt preimage mismatch")
		}
		if i < start {
			continue
		}
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(s.operation.Kind)
		if !found {
			return invalid, fmt.Errorf("semantic execution history operation has no owner descriptor")
		}
		diagnostic, err := runtimecontract.ProjectRuntimeSemanticExecutionDiagnostic(receipt.Feedback, descriptor.OwnerModuleID)
		if err != nil {
			return invalid, err
		}
		var ids []common.CoverageWorkItemID
		for _, binding := range s.dispatch.CoverageBindings {
			if slices.Contains(s.operation.BindingRefs, binding.ID) && !slices.Contains(ids, binding.CoverageWorkItemID) {
				ids = append(ids, binding.CoverageWorkItemID)
			}
		}
		observations = append(observations, runtimecontract.RuntimeSemanticReceiptObservation{
			ReceiptSequence: i + 1, DispatchID: s.dispatch.ID, OperationID: s.operation.ID, OperationKind: s.operation.Kind, OwnerModuleID: descriptor.OwnerModuleID,
			ReceiptRef: s.execution.OwnerReceiptRef, ReceiptHash: hash, Fence: receipt.Fence, BoundCoverageWorkItemIDs: ids,
			ObservedState: receipt.Outcome, ObservedEffectScope: receipt.ObservedEffectScope, Changed: receipt.Changed,
			ExpectedEffectScope: s.execution.ExpectedEffectScope, RequestedTargetRefs: append([]string(nil), s.operation.TargetRefs...), Diagnostic: diagnostic,
			TargetRefs: append([]string(nil), receipt.TargetRefs...), BeforeHash: receipt.BeforeHash, AfterHash: receipt.AfterHash,
			Validation: runtimecontract.ProjectRuntimeValidationExecutionFeedback(receipt.Evidence.Validation),
		})
	}
	return runtimecontract.BuildRuntimeSemanticExecutionHistory(c, m, len(a.OperationReceipts), observations)
}
