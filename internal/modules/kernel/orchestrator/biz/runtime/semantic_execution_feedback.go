package runtime

import (
	"fmt"
	"slices"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// SemanticExecutionFeedback is a read-only projection of committed execution
// facts for the sealed repair batch. It neither selects work nor awards credit.
func SemanticExecutionFeedback(a RuntimeAggregate, c runtimecontract.RuntimeSemanticPlanRequestContext, m runtimecontract.RuntimeSemanticPlanManifest) ([]runtimecontract.RuntimeSemanticExecutionFeedback, error) {
	c.ExecutionFeedback = nil
	if c.WorkspaceID != a.Facts.WorkspaceID || c.RunID != a.Facts.RunID || c.Definition.Revision != a.Definition.Revision || c.Definition.StartupInputHash != a.Definition.StartupInputHash {
		return nil, fmt.Errorf("semantic execution feedback aggregate scope mismatch")
	}
	if c.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch {
		for _, obligation := range m.ProofObligations {
			r := obligation.ValidationFailureRepair
			if r == nil || !slices.Contains(c.BatchCoverageWorkItemIDs, obligation.CoverageWorkItemID) {
				continue
			}
			declaration, found, err := runtimeDefinitionFocusForCoverage(a.Facts.DefinitionFocuses, obligation.CoverageWorkItemID)
			if err != nil || !found || declaration.ID != r.FailedFocusID {
				return nil, fmt.Errorf("semantic execution feedback repair declaration mismatch")
			}
			coverageIndex := runtimeCWICoverageIndex(a.Facts.Coverage, obligation.CoverageWorkItemID)
			if coverageIndex < 0 || len(a.Facts.Coverage[coverageIndex].ReopenLineage) == 0 {
				return nil, fmt.Errorf("semantic execution feedback has no CWI repair trigger lineage")
			}
			lineage := a.Facts.Coverage[coverageIndex].ReopenLineage
			reopen := lineage[len(lineage)-1]
			triggerRef := reopen.TriggerRef
			matches := 0
			for _, fact := range a.Facts.Evidence.Rejected {
				if string(fact.ID) != triggerRef || fact.FailureSignature != r.FailureSignature || fact.Kind != runtimecontract.EvidenceKindValidation {
					continue
				}
				dispatches := append([]SealedDispatch(nil), a.CompletedDispatches...)
				if a.ActiveDispatch != nil {
					dispatches = append(dispatches, *a.ActiveDispatch)
				}
				for _, dispatch := range dispatches {
					if dispatch.ID != fact.DispatchID {
						continue
					}
					op, found := candidateOperationByID(dispatch.Candidate, fact.OperationID)
					if !found || op.Kind != r.TriggerOperationKind || dispatch.WorkspaceID != c.WorkspaceID || dispatch.RunID != c.RunID || dispatch.Transaction.DefinitionRevision != c.Definition.Revision {
						return nil, fmt.Errorf("semantic execution feedback sealed operation mismatch")
					}
					bound := false
					for _, binding := range dispatch.CoverageBindings {
						if binding.CoverageWorkItemID == obligation.CoverageWorkItemID && slices.Contains(op.BindingRefs, binding.ID) &&
							binding.ID == reopen.PreviousBindingRef.BindingID && binding.Hash == reopen.PreviousBindingRef.BindingHash && binding.FocusID == reopen.PreviousBindingRef.FocusID {
							bound = true
						}
					}
					execution, found := operationExecutionRecord(dispatch.OperationExecutions, op.ID)
					if !bound || !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || !slices.Contains(dispatch.CompletedOperationIDs, op.ID) {
						return nil, fmt.Errorf("semantic execution feedback requires an exact CWI-bound committed execution")
					}
					receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(a.OperationReceipts, execution.ExecutionKey, execution.Fence)
					hash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
					if !found || receipt.OperationID != op.ID || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || execution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey || execution.OwnerReceiptHash != hash {
						return nil, fmt.Errorf("semantic execution feedback receipt mismatch")
					}
					if receipt.Feedback == nil || receipt.Feedback.WorkspaceID != c.WorkspaceID || receipt.Feedback.RunID != c.RunID ||
						receipt.Feedback.DispatchID != dispatch.ID || receipt.Feedback.OriginalDiagnostic == nil || receipt.Feedback.OriginalDiagnostic.OwnerModuleID != r.TriggerOwnerModuleID {
						return nil, fmt.Errorf("semantic execution feedback owner provenance mismatch")
					}
					if err := validateRuntimeReceiptEvidenceRejectedLineage(a, dispatch, op, execution, receipt, hash); err != nil {
						return nil, err
					}
					diagnostic, err := runtimecontract.ProjectRuntimeSemanticExecutionDiagnostic(receipt.Feedback, r.TriggerOwnerModuleID)
					if err != nil {
						return nil, err
					}
					feedback := runtimecontract.RuntimeSemanticExecutionFeedback{
						WorkspaceID: c.WorkspaceID, RunID: c.RunID, DefinitionRevision: c.Definition.Revision,
						StartupInputHash: c.Definition.StartupInputHash, ManifestHash: m.Hash, CoverageWorkItemID: obligation.CoverageWorkItemID,
						FailedFocusID: r.FailedFocusID, FailureSignature: r.FailureSignature, OwnerModuleID: r.TriggerOwnerModuleID, OperationKind: op.Kind,
						EvidenceID: fact.ID, EvidenceRevision: fact.Revision, DispatchID: dispatch.ID, OperationID: op.ID,
						ReceiptRef: execution.OwnerReceiptRef, ReceiptHash: hash,
						Validation: runtimecontract.ProjectRuntimeValidationExecutionFeedback(receipt.Evidence.Validation),
						Diagnostic: diagnostic,
					}
					feedback.Hash = runtimecontract.StableRuntimeSemanticExecutionFeedbackHash(feedback)
					c.ExecutionFeedback = append(c.ExecutionFeedback, feedback)
					matches++
				}
			}
			if matches != 1 {
				return nil, fmt.Errorf("semantic execution feedback requires exactly one causal receipt for repair CWI %s", obligation.CoverageWorkItemID)
			}
		}
	}
	if err := runtimecontract.ValidateRuntimeSemanticExecutionFeedback(c, m); err != nil {
		return nil, err
	}
	return c.ExecutionFeedback, nil
}
