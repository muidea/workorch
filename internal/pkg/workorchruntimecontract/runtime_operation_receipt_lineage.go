package workorchruntimecontract

import "fmt"

// ValidateRuntimeOperationReceiptExecutionLineage proves that the owner receipt
// ledger and the dispatch execution projections describe the same completed
// owner transactions. A governance source dispatch and its resumed completed
// clone may project the same execution identity, so one receipt may satisfy
// multiple identical projections; an unprojected receipt is never valid.
func ValidateRuntimeOperationReceiptExecutionLineage(
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	receipts []RuntimeOperationReceipt,
) error {
	return validateRuntimeOperationReceiptExecutionLineageInIndex(active, completed, receipts, nil)
}

func validateRuntimeOperationReceiptExecutionLineageInIndex(active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, receipts []RuntimeOperationReceipt, index *runtimeOwnerLineageIndex) error {
	if err := validateRuntimeOperationReceiptsInIndex(receipts, index); err != nil {
		return fmt.Errorf("runtime operation receipt ledger: %w", err)
	}

	receiptByIdentity := make(map[string]RuntimeOperationReceipt, len(receipts))
	matchedReceipt := make(map[string]bool, len(receipts))
	for _, receipt := range receipts {
		receipt = receipt.Normalized()
		identity := runtimeOperationReceiptIdentity(receipt.ExecutionKey, receipt.Fence)
		receiptByIdentity[identity] = receipt
	}

	validateDispatch := func(label string, dispatch RuntimeDispatchRecord) error {
		if err := ValidateRuntimeOperationExecutionRecords(dispatch.OperationExecutions); err != nil {
			return fmt.Errorf("%s dispatch %q operation executions: %w", label, dispatch.ID, err)
		}
		for _, execution := range dispatch.OperationExecutions {
			execution = execution.Normalized()
			if execution.State != OperationExecutionOwnerCommitted && execution.State != OperationExecutionEvidenceCommitted {
				continue
			}

			identity := runtimeOperationReceiptIdentity(execution.ExecutionKey, execution.Fence)
			receipt, found := receiptByIdentity[identity]
			if !found {
				return fmt.Errorf("%s dispatch %q operation %q committed execution %s has no owner receipt", label, dispatch.ID, execution.OperationID, identity)
			}
			if receipt.Evidence.OwnerRevisionSet != nil && receipt.Evidence.OwnerRevisionSet.DefinitionRevision != dispatch.Transaction.DefinitionRevision {
				return fmt.Errorf("%s dispatch %q owner dependencies definition mismatch", label, dispatch.ID)
			}
			if err := ValidateRuntimeEvidenceDependencyPair(receipt.Evidence.OwnerRevisionSet, receipt.Evidence.ApplicabilityDependencies, dispatch.Transaction.DefinitionRevision); err != nil {
				return fmt.Errorf("%s dispatch %q dependency pair: %w", label, dispatch.ID, err)
			}
			if receipt.OperationID != execution.OperationID {
				return fmt.Errorf("%s dispatch %q operation %q owner receipt operation mismatch", label, dispatch.ID, execution.OperationID)
			}
			for _, operation := range runtimeCandidateAdmissionOperations(dispatch.Candidate) {
				if operation.ID == execution.OperationID {
					if err := ValidateRuntimeHistoricalEffectReceiptPurpose(dispatch, operation, receipt.Evidence.ApplicabilityDependencies); err != nil {
						return err
					}
					if err := ValidateRuntimeEvidencePurposeForOperation(receipt.Evidence.ApplicabilityDependencies, operation.Kind, evidenceKindForConvergenceState(dispatch.Transaction.State)); err != nil {
						return err
					}
				}
			}
			expectedRef := "runtime-receipt/" + execution.ExecutionKey
			if execution.OwnerReceiptRef != expectedRef {
				return fmt.Errorf("%s dispatch %q operation %q owner receipt ref mismatch", label, dispatch.ID, execution.OperationID)
			}
			if execution.OwnerReceiptHash != index.receiptHash(receipt) {
				return fmt.Errorf("%s dispatch %q operation %q owner receipt hash mismatch", label, dispatch.ID, execution.OperationID)
			}
			matchedReceipt[identity] = true
		}
		return nil
	}

	if active != nil {
		if err := validateDispatch("active", *active); err != nil {
			return err
		}
	}
	for idx, dispatch := range completed {
		if err := validateDispatch(fmt.Sprintf("completed[%d]", idx), dispatch); err != nil {
			return err
		}
	}

	for identity := range receiptByIdentity {
		if !matchedReceipt[identity] {
			return fmt.Errorf("operation receipt %s has no committed dispatch execution projection", identity)
		}
	}
	return nil
}
