package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *RuntimeController) QueryOperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID, operationID, executionKey string, fence uint64, ownerModuleID string, grant *runtimecontract.RuntimeOperationGovernanceGrant) (runtimecontract.RuntimeOperationReceipt, bool, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(dispatchID) == "" || strings.TrimSpace(operationID) == "" || strings.TrimSpace(executionKey) == "" || fence == 0 || !exactOperationReceiptOwnerSource(ownerModuleID) {
		return runtimecontract.RuntimeOperationReceipt{}, false, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, err
	}
	if err := validateOperationReceiptOwnerSource(aggregate, dispatchID, operationID, ownerModuleID); err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, err
	}
	dispatch, active, found := operationReceiptDispatch(aggregate, dispatchID)
	if !found {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt query dispatch is not present in runtime aggregate", common.ErrConflict)
	}
	if _, found := candidateOperationByID(dispatch.Candidate, operationID); !found {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt query operation is not sealed in dispatch", common.ErrConflict)
	}
	execution, found := operationExecutionRecord(dispatch.OperationExecutions, operationID)
	if !found || execution.ExecutionKey != strings.TrimSpace(executionKey) || execution.Fence != fence {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt query does not match sealed execution key/fence", common.ErrConflict)
	}
	persistedGrant := governancecheck.GrantForExecution(aggregate.OperationGovernanceBindings, execution)
	if (persistedGrant == nil) != (grant == nil) {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: governance grant presence does not match persisted binding", common.ErrConflict)
	}
	if grant != nil {
		value := grant.Normalized()
		if err := value.Validate(); err != nil {
			return runtimecontract.RuntimeOperationReceipt{}, false, err
		}
		if !governancecheck.SameGrant(*persistedGrant, value) {
			return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: governance grant does not match persisted binding", common.ErrConflict)
		}
	}
	receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, executionKey, fence)
	if !ok {
		// found=false is itself an execution authorization: only the exact active
		// claimed record may proceed to an owner side effect. Sealed, stale,
		// completed, invalidated, and fabricated identities fail closed.
		if !active || execution.State != runtimecontract.OperationExecutionClaimed {
			return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt query execution is not active and claimed", common.ErrConflict)
		}
		return runtimecontract.RuntimeOperationReceipt{}, false, nil
	}
	if receipt.OperationID != strings.TrimSpace(operationID) {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt operation does not match query", common.ErrConflict)
	}
	if execution.State != runtimecontract.OperationExecutionOwnerCommitted && execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt query execution state does not admit replay", common.ErrConflict)
	}
	if execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt query hash does not match execution ledger", common.ErrConflict)
	}
	return receipt, true, nil
}

func validateOperationReceiptOwnerSource(aggregate RuntimeAggregate, dispatchID, operationID, ownerModuleID string) error {
	dispatchID = strings.TrimSpace(dispatchID)
	operationID = strings.TrimSpace(operationID)
	if dispatchID == "" || operationID == "" || !exactOperationReceiptOwnerSource(ownerModuleID) {
		return common.ErrInvalidArgument
	}
	dispatch, _, found := operationReceiptDispatch(aggregate, dispatchID)
	if !found {
		return fmt.Errorf("%w: receipt owner source dispatch is not present in runtime aggregate", common.ErrConflict)
	}
	operation, found := candidateOperationByID(dispatch.Candidate, operationID)
	if !found {
		return fmt.Errorf("%w: receipt owner source operation is not sealed in dispatch", common.ErrConflict)
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || !descriptor.RequiresOwnerReceipt || strings.TrimSpace(descriptor.OwnerModuleID) == "" {
		return fmt.Errorf("%w: sealed operation has no capability-owner receipt contract", common.ErrConflict)
	}
	if ownerModuleID != descriptor.OwnerModuleID {
		return fmt.Errorf("%w: receipt producer %q does not own sealed operation %q", common.ErrConflict, ownerModuleID, operationID)
	}
	return nil
}

// EventHub producer identity is exact protocol data. In particular, do not
// normalize whitespace into a capability owner: publishers must emit the
// canonical module ID and subscribers may only compare it byte-for-byte.
func exactOperationReceiptOwnerSource(ownerModuleID string) bool {
	return ownerModuleID != "" && ownerModuleID == strings.TrimSpace(ownerModuleID)
}

func (c *candidateDispatchComponent) commitOperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, fence uint64, ownerModuleID string, receipt runtimecontract.RuntimeOperationReceipt) (RuntimeAggregate, runtimecontract.RuntimeOperationReceipt, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(dispatchID) == "" || fence == 0 || !exactOperationReceiptOwnerSource(ownerModuleID) {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, common.ErrInvalidArgument
	}
	receipt = receipt.Normalized()
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	if err := validateOperationReceiptOwnerSource(aggregate, dispatchID, receipt.OperationID, ownerModuleID); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	// Producer authority is independent of receipt outcome shape. Reject a
	// forged owner before interpreting accepted/no-op/rejected content so every
	// terminal variant has the same sealed-source gate.
	receipt.Fence = fence
	if err := receipt.Validate(); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	if existing, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, receipt.ExecutionKey, receipt.Fence); ok {
		if !runtimecontract.SameRuntimeOperationReceipt(existing, receipt) {
			return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: execution key already has a different terminal receipt", common.ErrConflict)
		}
		return aggregate, existing, nil
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	// A receipt is an owner fact, but accepting it into the Runtime aggregate is
	// still an SM-3 admission. Do not persist a new receipt once the focus has
	// lost its canonical binding: evidence cannot legally attribute it and a
	// later retry must not turn that stale fact into a second execution path.
	// Existing matching receipts remain idempotent above.
	if err := validateCurrentCWIBindingFenceForEvidenceCommit(aggregate.Facts); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: operation receipt binding fence: %v", common.ErrConflict, err)
	}
	if err := validateOperationReceiptFeedback(aggregate, workspaceID, runID, dispatchID, receipt); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ID != strings.TrimSpace(dispatchID) {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: receipt does not match active dispatch", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	found := false
	var matchedExecution runtimecontract.RuntimeOperationExecutionRecord
	for idx := range aggregate.ActiveDispatch.OperationExecutions {
		execution := &aggregate.ActiveDispatch.OperationExecutions[idx]
		if execution.OperationID != receipt.OperationID || execution.ExecutionKey != receipt.ExecutionKey {
			continue
		}
		found = true
		if execution.Fence != fence || execution.State != runtimecontract.OperationExecutionClaimed {
			return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: stale operation receipt fence/state", common.ErrConflict)
		}
		execution.State = runtimecontract.OperationExecutionOwnerCommitted
		execution.OwnerReceiptRef = "runtime-receipt/" + receipt.ExecutionKey
		execution.OwnerReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
		matchedExecution = execution.Clone()
		break
	}
	if !found {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: receipt execution is not sealed", common.ErrConflict)
	}
	operation, operationFound := candidateOperationByID(aggregate.ActiveDispatch.Candidate, receipt.OperationID)
	if !operationFound {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: receipt operation is not sealed", common.ErrConflict)
	}
	if _, err := joinRuntimeOperationExecutionBudget(&aggregate, operation, matchedExecution, &receipt, false); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	aggregate.OperationReceipts = append(aggregate.OperationReceipts, receipt.Clone())
	for idx := range aggregate.OperationRecoveryAuthorizations {
		authorization := aggregate.OperationRecoveryAuthorizations[idx].Normalized()
		if authorization.State == runtimecontract.RuntimeOperationRecoveryAuthorizationInFlight &&
			authorization.DispatchID == aggregate.ActiveDispatch.ID && authorization.OperationID == matchedExecution.OperationID &&
			authorization.ExecutionKey == matchedExecution.ExecutionKey && authorization.Fence == matchedExecution.Fence {
			authorization.State = runtimecontract.RuntimeOperationRecoveryAuthorizationSettled
			aggregate.OperationRecoveryAuthorizations[idx] = authorization
		}
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	return aggregate, receipt, nil
}

func validateOperationReceiptFeedback(aggregate RuntimeAggregate, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, receipt runtimecontract.RuntimeOperationReceipt) error {
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected {
		if receipt.Feedback != nil {
			return fmt.Errorf("non-rejected operation receipt cannot carry diagnostic feedback")
		}
		return nil
	}
	if receipt.Feedback == nil {
		return fmt.Errorf("rejected operation receipt requires owner diagnostic feedback")
	}
	feedback := receipt.Feedback.Normalized()
	if err := feedback.Validate(); err != nil {
		return err
	}
	if feedback.Domain != runtimecontract.RuntimeFeedbackDomainOperation || feedback.StableCategory != runtimecontract.RuntimeFeedbackCategory(receipt.Outcome) ||
		feedback.WorkspaceID != workspaceID || feedback.RunID != runID || feedback.DispatchID != strings.TrimSpace(dispatchID) || feedback.OriginalDiagnostic == nil ||
		feedback.OriginalDiagnostic.OperationID != receipt.OperationID {
		return fmt.Errorf("operation receipt feedback does not match receipt identity")
	}
	dispatch := aggregate.ActiveDispatch
	if dispatch == nil || dispatch.ID != strings.TrimSpace(dispatchID) {
		return fmt.Errorf("%w: receipt feedback dispatch is not active", common.ErrConflict)
	}
	return validateOperationReceiptFeedbackAgainstDispatch(aggregate, *dispatch, workspaceID, runID, receipt)
}

func validateOperationReceiptFeedbackAgainstDispatch(aggregate RuntimeAggregate, dispatch SealedDispatch, workspaceID common.WorkspaceID, runID common.RunID, receipt runtimecontract.RuntimeOperationReceipt) error {
	feedback := receipt.Feedback.Normalized()
	if feedback.WorkspaceID != workspaceID || feedback.RunID != runID || feedback.DispatchID != dispatch.ID {
		return fmt.Errorf("operation receipt feedback does not match receipt scope")
	}
	operation, found := candidateOperationByID(dispatch.Candidate, receipt.OperationID)
	if !found {
		return fmt.Errorf("%w: receipt feedback operation is not sealed", common.ErrConflict)
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || feedback.OriginalDiagnostic.OwnerModuleID != descriptor.OwnerModuleID || feedback.AuthoritySnapshotID != dispatch.Transaction.AuthoritySnapshotID {
		return fmt.Errorf("operation receipt feedback does not match sealed owner or authority")
	}
	if err := validateOperationReceiptDiagnosticOwner(aggregate, dispatch, operation, *feedback.OriginalDiagnostic); err != nil {
		return err
	}
	if feedback.OriginalDiagnostic.TargetObservation != nil {
		if err := validateReceiptEvidenceTargetObservation(descriptor, receipt.Evidence, *feedback.OriginalDiagnostic.TargetObservation); err != nil {
			return err
		}
	}
	return nil
}

// validateOperationReceiptDiagnosticOwner closes the nested owner and sealed
// causation boundary. The outer feedback owner alone is insufficient because
// authority-shaped nested facts could otherwise claim another capability
// owner's observation or a different dispatch's target authorization.
func validateOperationReceiptDiagnosticOwner(aggregate RuntimeAggregate, dispatch SealedDispatch, operation runtimecontract.RuntimeOperation, diagnostic runtimecontract.RuntimeOriginalDiagnostic) error {
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found {
		return fmt.Errorf("operation receipt diagnostic has no sealed operation descriptor")
	}
	normalized := diagnostic.Normalized()
	if normalized == nil || normalized.OwnerModuleID != descriptor.OwnerModuleID {
		return fmt.Errorf("operation receipt diagnostic owner does not match sealed operation owner")
	}
	switch descriptor.FailureTargetProtocol {
	case runtimecontract.RuntimeFailureTargetProtocolNone:
		if normalized.TargetObservation != nil || normalized.ValidationFailureObservation != nil || len(normalized.TargetRefs) != 0 {
			return fmt.Errorf("operation descriptor does not admit failure target authority")
		}
		return nil
	case runtimecontract.RuntimeFailureTargetProtocolSealedOperation:
		if normalized.TargetObservation != nil {
			return fmt.Errorf("operation descriptor does not admit direct target observations")
		}
		if normalized.ValidationFailureObservation != nil {
			return fmt.Errorf("operation descriptor does not admit validation failure candidates")
		}
		if len(normalized.TargetRefs) != 0 && !runtimeTargetRefsWithin(normalized.TargetRefs, operation.TargetRefs) {
			return fmt.Errorf("operation receipt diagnostic targets exceed the sealed operation targets")
		}
		return nil
	case runtimecontract.RuntimeFailureTargetProtocolReceiptEvidence:
		if normalized.ValidationFailureObservation != nil {
			return fmt.Errorf("operation descriptor does not admit validation failure candidates")
		}
		if normalized.TargetObservation == nil {
			if len(normalized.TargetRefs) != 0 {
				return fmt.Errorf("receipt-evidence target protocol requires an explicit target observation")
			}
			return nil
		}
		observation := normalized.TargetObservation.Normalized()
		if observation.OwnerModuleID != descriptor.OwnerModuleID {
			return fmt.Errorf("operation receipt target observation owner does not match sealed operation owner")
		}
		if !sameRuntimeTargetRefs(normalized.TargetRefs, observation.TargetRefs) {
			return fmt.Errorf("operation receipt target observation does not match diagnostic targets")
		}
		return nil
	case runtimecontract.RuntimeFailureTargetProtocolValidationCandidates:
		// Continue with the VFO-specific sealed scope checks below.
	default:
		return fmt.Errorf("operation descriptor has unsupported failure target protocol %q", descriptor.FailureTargetProtocol)
	}
	if normalized.TargetObservation != nil {
		return fmt.Errorf("validation-candidate receipt cannot carry a direct target observation")
	}
	// A validation owner's top-level TargetRefs would be interpreted as immediate
	// repair authority by rejected-evidence admission. Managed validation must
	// keep candidates solely inside RuntimeValidationFailureObservation until a
	// Resource-owner confirmation is committed through the SM-2 transition.
	if len(normalized.TargetRefs) != 0 {
		return fmt.Errorf("managed-validation receipt cannot carry direct diagnostic target refs")
	}
	observation := normalized.ValidationFailureObservation
	if observation == nil {
		return nil
	}
	if normalized.ValidationDiagnostic == nil {
		return fmt.Errorf("managed-validation failure observation requires a validation diagnostic")
	}
	validationDiagnostic := normalized.ValidationDiagnostic.Normalized()
	if validationDiagnostic.Class != observation.DiagnosticClass || !sameRuntimeTargetRefs(validationDiagnostic.TargetRefs, observation.CandidateTargetRefs) {
		return fmt.Errorf("managed-validation failure observation does not match its diagnostic candidates")
	}
	if observation.WorkspaceID != aggregate.Facts.WorkspaceID || observation.RunID != aggregate.Facts.RunID {
		return fmt.Errorf("managed-validation failure observation does not match receipt scope")
	}
	if !sealedValidationFailureObservationMatchesDispatch(dispatch, *observation) {
		return fmt.Errorf("managed-validation failure observation does not match sealed dispatch authorization")
	}
	return nil
}

// validateReceiptEvidenceTargetObservation binds a descriptor-authorized
// discovered target to the exact normalized evidence persisted before the
// receipt was built. It is shared by commit and fresh-load validation.
func validateReceiptEvidenceTargetObservation(descriptor runtimecontract.RuntimeOperationDescriptor, evidence runtimecontract.RuntimeOperationEvidence, observation runtimecontract.RuntimeFailureTargetObservation) error {
	observation = observation.Normalized()
	if err := observation.Validate(); err != nil {
		return fmt.Errorf("receipt-evidence target observation: %w", err)
	}
	if descriptor.FailureTargetProtocol != runtimecontract.RuntimeFailureTargetProtocolReceiptEvidence || observation.OwnerModuleID != descriptor.OwnerModuleID {
		return fmt.Errorf("receipt-evidence target observation does not match the sealed descriptor owner policy")
	}
	dependency := observation.CatalogDependency
	if dependency == nil || dependency.Validate() != nil || dependency.Domain != runtimecontract.RuntimeResourceOwnerCatalog ||
		dependency.OwnerModuleID != descriptor.OwnerModuleID || dependency.Revision != observation.ObservedRevision {
		return fmt.Errorf("receipt-evidence target observation has no exact owner-catalog dependency")
	}
	if observation.DiagnosticClass != runtimecontract.RuntimeFailureTargetDiagnosticReceiptEvidence {
		return fmt.Errorf("receipt-evidence target observation has invalid diagnostic class")
	}
	contentHash := strings.TrimSpace(evidence.ContentHash)
	if contentHash == "" || evidence.ContentAddress == nil {
		return fmt.Errorf("receipt-evidence target observation does not match receipt evidence")
	}
	contentAddress := evidence.ContentAddress.Normalized()
	if contentAddress.Validate() != nil || contentAddress.Digest != contentHash || !containsString(evidence.EvidenceRefs, contentAddress.Ref) ||
		len(evidence.EvidenceRefs) == 0 || observation.ContentHash != contentHash ||
		!sameRuntimeTargetRefs(observation.EvidenceRefs, evidence.EvidenceRefs) {
		return fmt.Errorf("receipt-evidence target observation does not match receipt evidence")
	}
	if !containsString(observation.EvidenceRefs, contentAddress.Ref) {
		return fmt.Errorf("receipt-evidence target observation has no owner content-addressed evidence ref")
	}
	return nil
}

func failureFactFromOperationReceiptFeedback(receipt runtimecontract.RuntimeOperationReceipt) *runtimecontract.FailureFact {
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil {
		return nil
	}
	diagnostic := receipt.Feedback.OriginalDiagnostic.CloneOriginal()
	failure := &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClass(diagnostic.OriginalClass), Code: diagnostic.OriginalCode,
		Signature: diagnostic.Signature, Message: diagnostic.OriginalMessage,
		OriginalDiagnostic: diagnostic.CloneOriginal(), TargetRefs: append([]string(nil), diagnostic.TargetRefs...),
	}
	if diagnostic.TargetObservation != nil {
		value := diagnostic.TargetObservation.Clone()
		failure.TargetObservation = &value
	}
	if diagnostic.ValidationDiagnostic != nil {
		value := diagnostic.ValidationDiagnostic.Clone()
		failure.ValidationDiagnostic = &value
	}
	if diagnostic.ValidationFailureObservation != nil {
		value := diagnostic.ValidationFailureObservation.Clone()
		failure.ValidationFailureObservation = &value
	}
	if diagnostic.GovernanceDependency != nil {
		value := diagnostic.GovernanceDependency.Clone()
		failure.GovernanceDependency = &value
	}
	return failure
}

func operationReceiptDispatch(aggregate RuntimeAggregate, dispatchID string) (SealedDispatch, bool, bool) {
	dispatchID = strings.TrimSpace(dispatchID)
	if aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.ID == dispatchID {
		return *aggregate.ActiveDispatch, true, true
	}
	for _, dispatch := range aggregate.CompletedDispatches {
		if dispatch.ID == dispatchID {
			return dispatch, false, true
		}
	}
	return SealedDispatch{}, false, false
}
