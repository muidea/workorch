package events

import (
	"context"
	"fmt"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
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

func QueryPersistedRuntimeOperationReceipt(ctx context.Context, hub magicevent.Hub, source string, workspaceID common.WorkspaceID, runID common.RunID, dispatchID, operationID string, execution RuntimeOperationExecutionEnvelope) (runtimecontract.RuntimeOperationReceipt, bool, error) {
	if hub == nil || workspaceID == "" || runID == "" || strings.TrimSpace(dispatchID) == "" || strings.TrimSpace(operationID) == "" {
		return runtimecontract.RuntimeOperationReceipt{}, false, common.ErrInvalidArgument
	}
	if strings.TrimSpace(execution.ExecutionKey) == "" {
		return runtimecontract.RuntimeOperationReceipt{}, false, nil
	}
	if err := execution.Validate(); err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, err
	}
	event := magicevent.NewEventWithContext(TopicOrchestratorQueryRuntimeOperationReceipt, source, CommandDestination, magicevent.NewHeader(), ctx, QueryRuntimeOperationReceiptCommand{
		WorkspaceID: workspaceID, RunID: runID, DispatchID: dispatchID, OperationID: operationID, ExecutionKey: execution.ExecutionKey, Fence: execution.Fence,
		GovernanceGrant: execution.GovernanceGrant,
	})
	event.BindLaneKey(eventcontract.QueryLaneKey(TopicOrchestratorQueryRuntimeOperationReceipt, eventcontract.LaneScope{
		WorkspaceID: string(workspaceID), RunID: string(runID), SubjectID: execution.ExecutionKey,
	}))
	result := hub.Send(event)
	if result == nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("runtime receipt query returned no result")
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, magicErr
	}
	response, ok := data.(QueryRuntimeOperationReceiptResult)
	if !ok {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("runtime receipt query returned invalid result type %T", data)
	}
	if !response.Found || response.Receipt == nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, nil
	}
	receipt := response.Receipt.Clone()
	return receipt, true, nil
}

func CommitPersistedRuntimeOperationReceipt(ctx context.Context, hub magicevent.Hub, source string, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, execution RuntimeOperationExecutionEnvelope, receipt runtimecontract.RuntimeOperationReceipt) (runtimecontract.RuntimeOperationReceipt, error) {
	if hub == nil || workspaceID == "" || runID == "" || strings.TrimSpace(dispatchID) == "" {
		return runtimecontract.RuntimeOperationReceipt{}, common.ErrInvalidArgument
	}
	if strings.TrimSpace(execution.ExecutionKey) == "" {
		return runtimecontract.RuntimeOperationReceipt{}, nil
	}
	if err := execution.Validate(); err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, err
	}
	receipt = receipt.Normalized()
	if receipt.ExecutionKey != execution.ExecutionKey {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("runtime receipt execution key mismatch")
	}
	event := magicevent.NewEventWithContext(TopicOrchestratorCommitRuntimeOperationReceipt, source, CommandDestination, magicevent.NewHeader(), ctx, CommitRuntimeOperationReceiptCommand{
		WorkspaceID: workspaceID, RunID: runID, DispatchID: dispatchID, Fence: execution.Fence, Receipt: receipt,
	})
	event.BindLaneKey(eventcontract.StateLaneKey("orchestrator", eventcontract.LaneScope{
		WorkspaceID: string(workspaceID), RunID: string(runID), SubjectID: execution.ExecutionKey,
	}))
	result := hub.Send(event)
	if result == nil {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("runtime receipt commit returned no result")
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return runtimecontract.RuntimeOperationReceipt{}, magicErr
	}
	response, ok := data.(CommitRuntimeOperationReceiptResult)
	if !ok {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("runtime receipt commit returned invalid result type %T", data)
	}
	return response.Receipt.Clone(), nil
}

func NewRuntimeOperationReceipt(execution RuntimeOperationExecutionEnvelope, operationID string, status runtimecontract.RuntimeOperationExecutionStatus, targetRefs []string, evidence runtimecontract.RuntimeOperationEvidence, failure *runtimecontract.FailureFact, governance *runtimecontract.RuntimeGovernanceRequirement) runtimecontract.RuntimeOperationReceipt {
	return NewRuntimeOperationReceiptWithObservation(execution, operationID, status, targetRefs, evidence, failure, governance, RuntimeOperationReceiptObservation{
		ObservedEffectScope: execution.ExpectedEffectScope,
		Changed:             status == runtimecontract.RuntimeOperationExecutionStatusAccepted && runtimecontract.RuntimeEffectIsDurable(execution.ExpectedEffectScope),
	})
}

type RuntimeOperationReceiptObservation struct {
	ObservedEffectScope runtimecontract.RuntimeEffectScope
	Changed             bool
	NoOp                bool
	BeforeHash          string
	AfterHash           string
	ResourceChanges     []runtimecontract.RuntimeResourceChange
}

func NewRuntimeOperationReceiptWithObservation(execution RuntimeOperationExecutionEnvelope, operationID string, status runtimecontract.RuntimeOperationExecutionStatus, targetRefs []string, evidence runtimecontract.RuntimeOperationEvidence, failure *runtimecontract.FailureFact, governance *runtimecontract.RuntimeGovernanceRequirement, observation RuntimeOperationReceiptObservation) runtimecontract.RuntimeOperationReceipt {
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: strings.TrimSpace(operationID),
		ObservedEffectScope: observation.ObservedEffectScope, Changed: observation.Changed, NoOp: observation.NoOp,
		BeforeHash: strings.TrimSpace(observation.BeforeHash), AfterHash: strings.TrimSpace(observation.AfterHash),
		ResourceChanges: append([]runtimecontract.RuntimeResourceChange(nil), observation.ResourceChanges...),
		TargetRefs:      append([]string(nil), targetRefs...), Evidence: evidence,
		CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
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
