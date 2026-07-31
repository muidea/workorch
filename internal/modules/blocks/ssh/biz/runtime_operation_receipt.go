package ssh

import (
	"context"
	"fmt"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// queryRuntimeOperationReceipt is owned by the capability Biz: it performs the
// typed EventHub read required before an operation can be safely replayed.
func (m *Biz) queryRuntimeOperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID, operationID string, execution orchestratorevents.RuntimeOperationExecutionEnvelope) (runtimecontract.RuntimeOperationReceipt, bool, error) {
	if m == nil || m.EventHub() == nil || workspaceID == "" || runID == "" || strings.TrimSpace(dispatchID) == "" || strings.TrimSpace(operationID) == "" {
		return runtimecontract.RuntimeOperationReceipt{}, false, common.ErrInvalidArgument
	}
	if strings.TrimSpace(execution.ExecutionKey) == "" {
		return runtimecontract.RuntimeOperationReceipt{}, false, nil
	}
	if err := execution.Validate(); err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, err
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, ModuleID, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.QueryRuntimeOperationReceiptCommand{
		WorkspaceID: workspaceID, RunID: runID, DispatchID: dispatchID, OperationID: operationID, ExecutionKey: execution.ExecutionKey, Fence: execution.Fence,
		GovernanceGrant: execution.GovernanceGrant,
	})
	event.BindLaneKey(eventcontract.QueryLaneKey(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, eventcontract.LaneScope{
		WorkspaceID: string(workspaceID), RunID: string(runID), SubjectID: execution.ExecutionKey,
	}))
	result := m.SendEvent(event)
	if result == nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("runtime receipt query returned no result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, resultErr
	}
	response, ok := data.(orchestratorevents.QueryRuntimeOperationReceiptResult)
	if !ok {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("runtime receipt query returned invalid result type %T", data)
	}
	if !response.Found || response.Receipt == nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, nil
	}
	return response.Receipt.Clone(), true, nil
}

// commitRuntimeOperationReceipt is owned by the capability Biz. It sends the
// concrete receipt command; the Orchestrator remains the persisted receipt owner.
func (m *Biz) commitRuntimeOperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, authoritySnapshotID, dispatchID string, execution orchestratorevents.RuntimeOperationExecutionEnvelope, receipt runtimecontract.RuntimeOperationReceipt) (runtimecontract.RuntimeOperationReceipt, error) {
	if m == nil || m.EventHub() == nil || workspaceID == "" || runID == "" || strings.TrimSpace(authoritySnapshotID) == "" || strings.TrimSpace(dispatchID) == "" {
		return runtimecontract.RuntimeOperationReceipt{}, common.ErrInvalidArgument
	}
	if strings.TrimSpace(execution.ExecutionKey) == "" {
		return runtimecontract.RuntimeOperationReceipt{}, nil
	}
	if err := execution.Validate(); err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, err
	}
	receipt = receipt.Normalized()
	attached, err := orchestratorevents.AttachRuntimeOperationReceiptFeedback(receipt, runtimecontract.RuntimeFeedbackScope{WorkspaceID: workspaceID, RunID: runID, AuthoritySnapshotID: authoritySnapshotID, DispatchID: dispatchID}, ModuleID)
	if err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, err
	}
	receipt = attached
	if receipt.ExecutionKey != execution.ExecutionKey {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("runtime receipt execution key mismatch")
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, ModuleID, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.CommitRuntimeOperationReceiptCommand{
		WorkspaceID: workspaceID, RunID: runID, DispatchID: dispatchID, Fence: execution.Fence, Receipt: receipt,
	})
	event.BindLaneKey(eventcontract.StateLaneKey("orchestrator", eventcontract.LaneScope{
		WorkspaceID: string(workspaceID), RunID: string(runID), SubjectID: execution.ExecutionKey,
	}))
	result := m.SendEvent(event)
	if result == nil {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("runtime receipt commit returned no result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return runtimecontract.RuntimeOperationReceipt{}, resultErr
	}
	response, ok := data.(orchestratorevents.CommitRuntimeOperationReceiptResult)
	if !ok {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("runtime receipt commit returned invalid result type %T", data)
	}
	return response.Receipt.Clone(), nil
}

func (m *Biz) newRuntimeOperationReceipt(execution orchestratorevents.RuntimeOperationExecutionEnvelope, operationID string, status runtimecontract.RuntimeOperationExecutionStatus, targetRefs []string, evidence runtimecontract.RuntimeOperationEvidence, failure *runtimecontract.FailureFact, governance *runtimecontract.RuntimeGovernanceRequirement, observation orchestratorevents.RuntimeOperationReceiptObservation) runtimecontract.RuntimeOperationReceipt {
	return orchestratorevents.NewRuntimeOperationReceiptWithObservation(execution, operationID, status, targetRefs, evidence, failure, governance, observation, time.Now().UTC().Format(time.RFC3339Nano))
}
