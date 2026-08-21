package events

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ValidateQueryRuntimeOperationReceiptResult validates the complete typed
// query response before a capability owner treats Found=false as execution
// authorization or reuses a persisted receipt. A malformed or stale response
// is a cross-owner identity conflict, never an implicit "not found" result.
func ValidateQueryRuntimeOperationReceiptResult(result QueryRuntimeOperationReceiptResult, executionKey string, fence uint64, operationID string) (runtimecontract.RuntimeOperationReceipt, bool, error) {
	executionKey = strings.TrimSpace(executionKey)
	operationID = strings.TrimSpace(operationID)
	if executionKey == "" || fence == 0 || operationID == "" {
		return runtimecontract.RuntimeOperationReceipt{}, false, common.ErrInvalidArgument
	}
	if result.Found != (result.Receipt != nil) {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: runtime receipt query found/receipt mismatch", common.ErrConflict)
	}
	if !result.Found {
		return runtimecontract.RuntimeOperationReceipt{}, false, nil
	}
	receipt := result.Receipt.Clone()
	if err := receipt.Validate(); err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: runtime receipt query returned invalid receipt: %v", common.ErrConflict, err)
	}
	if receipt.ExecutionKey != executionKey || receipt.Fence != fence || receipt.OperationID != operationID {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: runtime receipt query identity mismatch", common.ErrConflict)
	}
	return receipt, true, nil
}

// ValidateCommitRuntimeOperationReceiptResult ensures the receipt returned by
// the Runtime owner is exactly the owner fact submitted for this transaction.
// StateRevision is intentionally not interpreted by capability owners.
func ValidateCommitRuntimeOperationReceiptResult(result CommitRuntimeOperationReceiptResult, expected runtimecontract.RuntimeOperationReceipt) (runtimecontract.RuntimeOperationReceipt, error) {
	expected = expected.Normalized()
	if err := expected.Validate(); err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, common.ErrInvalidArgument
	}
	receipt := result.Receipt.Clone()
	if err := receipt.Validate(); err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: runtime receipt commit returned invalid receipt: %v", common.ErrConflict, err)
	}
	if !runtimecontract.SameRuntimeOperationReceipt(receipt, expected) {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: runtime receipt commit identity or payload mismatch", common.ErrConflict)
	}
	return receipt, nil
}

// AttachRuntimeOperationReceiptFeedback binds the owner-authored failure to
// the workspace/run/authority/dispatch scope before the receipt crosses the
// EventHub boundary. It preserves the original diagnostic without changing
// Receipt.Outcome or choosing a Runtime transition.
func AttachRuntimeOperationReceiptFeedback(receipt runtimecontract.RuntimeOperationReceipt, scope runtimecontract.RuntimeFeedbackScope, ownerModuleID string) (runtimecontract.RuntimeOperationReceipt, error) {
	receipt = receipt.Normalized()
	if receipt.Feedback == nil {
		return receipt, nil
	}
	feedback := receipt.Feedback.CloneOriginal()
	feedback.WorkspaceID = scope.WorkspaceID
	feedback.RunID = scope.RunID
	feedback.AuthoritySnapshotID = scope.AuthoritySnapshotID
	feedback.DispatchID = scope.DispatchID
	if feedback.OriginalDiagnostic == nil || feedback.OriginalDiagnostic.OwnerModuleID != strings.TrimSpace(ownerModuleID) {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("runtime operation receipt feedback owner mismatch")
	}
	if err := feedback.Validate(); err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, err
	}
	receipt.Feedback = &feedback
	return receipt, nil
}

func NewRuntimeOperationReceipt(execution runtimecontract.RuntimeOperationExecutionEnvelope, ownerModuleID, operationID string, status runtimecontract.RuntimeOperationExecutionStatus, targetRefs []string, evidence runtimecontract.RuntimeOperationEvidence, failure *runtimecontract.FailureFact, governance *runtimecontract.RuntimeGovernanceRequirement, completedAt string) runtimecontract.RuntimeOperationReceipt {
	return NewRuntimeOperationReceiptWithObservation(execution, ownerModuleID, operationID, status, targetRefs, evidence, failure, governance, RuntimeOperationReceiptObservation{
		ObservedEffectScope: execution.ExpectedEffectScope,
		Changed:             status == runtimecontract.RuntimeOperationExecutionStatusAccepted && runtimecontract.RuntimeEffectIsDurable(execution.ExpectedEffectScope),
	}, completedAt)
}

type RuntimeOperationReceiptObservation struct {
	ObservedEffectScope runtimecontract.RuntimeEffectScope
	Changed             bool
	NoOp                bool
	BeforeHash          string
	AfterHash           string
	ResourceChanges     []runtimecontract.RuntimeResourceChange
}

func NewRuntimeOperationReceiptWithObservation(execution runtimecontract.RuntimeOperationExecutionEnvelope, ownerModuleID, operationID string, status runtimecontract.RuntimeOperationExecutionStatus, targetRefs []string, evidence runtimecontract.RuntimeOperationEvidence, failure *runtimecontract.FailureFact, governance *runtimecontract.RuntimeGovernanceRequirement, observation RuntimeOperationReceiptObservation, completedAt string) runtimecontract.RuntimeOperationReceipt {
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: strings.TrimSpace(operationID),
		ObservedEffectScope: observation.ObservedEffectScope, Changed: observation.Changed,
		BeforeHash: strings.TrimSpace(observation.BeforeHash), AfterHash: strings.TrimSpace(observation.AfterHash),
		ResourceChanges: append([]runtimecontract.RuntimeResourceChange(nil), observation.ResourceChanges...),
		TargetRefs:      append([]string(nil), targetRefs...), Evidence: evidence,
		CompletedAt: strings.TrimSpace(completedAt),
	}
	if receipt.ObservedEffectScope == "" {
		receipt.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
	}
	switch {
	case governance != nil:
		receipt.Outcome = runtimecontract.RuntimeOperationOutcomeGovernanceRequired
		receipt.ObservedEffectScope = runtimecontract.RuntimeEffectGovernanceOnly
		receipt.Changed = false
		value := governance.Normalized()
		receipt.Governance = &value
	case status == runtimecontract.RuntimeOperationExecutionStatusAccepted && observation.NoOp:
		receipt.Outcome = runtimecontract.RuntimeOperationOutcomeNoOp
		receipt.Changed = false
	case status == runtimecontract.RuntimeOperationExecutionStatusAccepted:
		receipt.Outcome = runtimecontract.RuntimeOperationOutcomeAccepted
	case failure != nil:
		receipt.Outcome = runtimecontract.RuntimeOperationOutcomeRejected
		receipt.Changed = observation.Changed
		receipt.Feedback = &runtimecontract.RuntimeFeedback{
			Domain: runtimecontract.RuntimeFeedbackDomainOperation, StableCategory: runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOperationOutcomeRejected),
			OriginalDiagnostic: runtimecontract.RuntimeOriginalDiagnosticFromFailureFact(ownerModuleID, *failure),
		}
		receipt.Feedback.OriginalDiagnostic.OperationID = receipt.OperationID
	default:
		receipt.Outcome = runtimecontract.RuntimeOperationOutcomeUnknown
		receipt.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
	}
	return receipt.Normalized()
}

func RuntimeOperationResultFromReceipt(receipt runtimecontract.RuntimeOperationReceipt) (runtimecontract.RuntimeOperationExecutionStatus, runtimecontract.RuntimeOperationEvidence, *runtimecontract.RuntimeGovernanceRequirement) {
	receipt = receipt.Normalized()
	status := runtimecontract.RuntimeOperationExecutionStatusAccepted
	if receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected || receipt.Outcome == runtimecontract.RuntimeOperationOutcomeGovernanceRequired || receipt.Outcome == runtimecontract.RuntimeOperationOutcomeUnknown {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
	}
	return status, receipt.Evidence, receipt.Governance
}
