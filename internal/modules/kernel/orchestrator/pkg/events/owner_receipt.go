package events

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (e RuntimeOperationExecutionEnvelope) Validate() error {
	if strings.TrimSpace(e.ExecutionKey) == "" || e.Fence == 0 || !runtimecontract.ValidRuntimeEffectScope(e.ExpectedEffectScope) {
		return fmt.Errorf("runtime operation execution envelope requires key, fence and expected effect")
	}
	if e.GovernanceGrant != nil {
		grant := e.GovernanceGrant.Normalized()
		if err := grant.Validate(); err != nil {
			return err
		}
		if grant.ExecutionKey != strings.TrimSpace(e.ExecutionKey) || grant.ResumeFence != e.Fence {
			return fmt.Errorf("runtime operation governance grant does not match execution envelope")
		}
	}
	return nil
}

func NewRuntimeOperationReceipt(execution RuntimeOperationExecutionEnvelope, operationID string, status runtimecontract.RuntimeOperationExecutionStatus, targetRefs []string, evidence runtimecontract.RuntimeOperationEvidence, failure *runtimecontract.FailureFact, governance *runtimecontract.RuntimeGovernanceRequirement, completedAt string) runtimecontract.RuntimeOperationReceipt {
	return NewRuntimeOperationReceiptWithObservation(execution, operationID, status, targetRefs, evidence, failure, governance, RuntimeOperationReceiptObservation{
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

func NewRuntimeOperationReceiptWithObservation(execution RuntimeOperationExecutionEnvelope, operationID string, status runtimecontract.RuntimeOperationExecutionStatus, targetRefs []string, evidence runtimecontract.RuntimeOperationEvidence, failure *runtimecontract.FailureFact, governance *runtimecontract.RuntimeGovernanceRequirement, observation RuntimeOperationReceiptObservation, completedAt string) runtimecontract.RuntimeOperationReceipt {
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: strings.TrimSpace(operationID),
		ObservedEffectScope: observation.ObservedEffectScope, Changed: observation.Changed, NoOp: observation.NoOp,
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
		receipt.NoOp = false
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
		receipt.Failure = failure
	default:
		receipt.Outcome = runtimecontract.RuntimeOperationOutcomeUnknown
		receipt.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
	}
	return receipt.Normalized()
}

func RuntimeOperationResultFromReceipt(receipt runtimecontract.RuntimeOperationReceipt) (runtimecontract.RuntimeOperationExecutionStatus, runtimecontract.RuntimeOperationEvidence, *runtimecontract.FailureFact, *runtimecontract.RuntimeGovernanceRequirement) {
	receipt = receipt.Normalized()
	status := runtimecontract.RuntimeOperationExecutionStatusAccepted
	if receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected || receipt.Outcome == runtimecontract.RuntimeOperationOutcomeGovernanceRequired || receipt.Outcome == runtimecontract.RuntimeOperationOutcomeUnknown {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
	}
	return status, receipt.Evidence, receipt.Failure, receipt.Governance
}
