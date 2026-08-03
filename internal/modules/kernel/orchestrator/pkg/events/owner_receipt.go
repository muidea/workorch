package events

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
