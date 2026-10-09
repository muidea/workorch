package orchestrator

// Runtime 终态交付由 Biz 编排，材料仅由共享纯 compiler 生成。

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// deliverRuntimeTerminal 是唯一 SM-4 终态交付入口，先持久化不可变材料，再按 Result→Workspace→Run 投递。
func (m *Biz) deliverRuntimeTerminal(ctx context.Context, aggregate runtime.RuntimeAggregate) error {
	if m == nil || m.runtimeController == nil || (aggregate.Decision.Terminal == nil && aggregate.Termination == nil) {
		return common.ErrInvalidArgument
	}
	if err := aggregate.Validate(); err != nil {
		return fmt.Errorf("persisted Runtime terminal aggregate is invalid: %w", err)
	}
	sealed, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:     runtime.RuntimeInternalEventTerminalMaterialSealRequired,
		Scope:    runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Delivery: &runtime.RuntimeInternalDeliveryPayload{},
	})
	if err != nil {
		return err
	}
	aggregate = sealed.Aggregate
	if aggregate.TerminalMaterialSeal == nil {
		return fmt.Errorf("%w: terminal delivery has no original material seal", common.ErrConflict)
	}
	request, err := terminalDeliveryRequest(aggregate)
	if err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if !deliveryAcknowledged(aggregate, request.TerminalDeliveryID, runtimecontract.RuntimeDeliveryOwnerResult) {
		material := resultMaterialFromTerminalSeal(*aggregate.TerminalMaterialSeal)
		if err := m.deliverRuntimeResult(ctx, request, material); err != nil {
			return err
		}
	}
	if !m.persistedDeliveryAcknowledged(ctx, aggregate, request.TerminalDeliveryID, runtimecontract.RuntimeDeliveryOwnerWorkspace) {
		if err := m.deliverRuntimeWorkspace(ctx, request); err != nil {
			return err
		}
	}
	if !m.persistedDeliveryAcknowledged(ctx, aggregate, request.TerminalDeliveryID, runtimecontract.RuntimeDeliveryOwnerRun) {
		resultID, found := m.persistedTerminalDeliveryResultID(ctx, aggregate, request.TerminalDeliveryID)
		if !found {
			return fmt.Errorf("%w: terminal Run delivery has no persisted Result-owner identity", common.ErrConflict)
		}
		return m.deliverRuntimeRun(ctx, request, resultID)
	}
	return nil
}

func deliveryAcknowledged(aggregate runtime.RuntimeAggregate, deliveryID string, owner runtimecontract.RuntimeDeliveryOwner) bool {
	for _, acknowledgement := range aggregate.DeliveryAcknowledgements {
		if acknowledgement.TerminalDeliveryID == deliveryID && acknowledgement.Owner == owner {
			return true
		}
	}
	return false
}

func runtimeTerminalDeliveryFullyAcknowledged(aggregate runtime.RuntimeAggregate) bool {
	return runtime.TerminalDeliveryFullyAcknowledged(aggregate)
}

func (m *Biz) persistedDeliveryAcknowledged(ctx context.Context, aggregate runtime.RuntimeAggregate, deliveryID string, owner runtimecontract.RuntimeDeliveryOwner) bool {
	if m == nil || m.runtimeController == nil {
		return false
	}
	loaded, err := m.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	return err == nil && deliveryAcknowledged(loaded, deliveryID, owner)
}

func (m *Biz) persistedTerminalDeliveryResultID(ctx context.Context, aggregate runtime.RuntimeAggregate, deliveryID string) (common.ResultID, bool) {
	if m == nil || m.runtimeController == nil {
		return "", false
	}
	loaded, err := m.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		return "", false
	}
	for _, acknowledgement := range loaded.DeliveryAcknowledgements {
		if acknowledgement.TerminalDeliveryID == deliveryID && acknowledgement.Owner == runtimecontract.RuntimeDeliveryOwnerResult && acknowledgement.ResultID != "" {
			return acknowledgement.ResultID, true
		}
	}
	return "", false
}

func resultMaterialFromTerminalSeal(seal runtimecontract.RuntimeTerminalMaterialSeal) resultevents.ResultMaterial {
	return resultevents.ResultMaterialFromRuntimeTerminalMaterial(seal.Material)
}

func terminalDeliveryRequest(aggregate runtime.RuntimeAggregate) (orchestratorevents.RuntimeTerminalDeliveryRequestData, error) {
	if aggregate.TerminalMaterialSeal != nil {
		return orchestratorevents.RuntimeTerminalDeliveryRequestData{
			RuntimeTerminalDeliveryIdentity: aggregate.TerminalMaterialSeal.Identity,
			SourceOwner:                     ModuleID, CausationID: "terminal-delivery/" + aggregate.TerminalMaterialSeal.Identity.TerminalDeliveryID,
			StateRevision: aggregate.TerminalMaterialSeal.GenerationStateRevision,
		}, nil
	}
	outcome, terminal, err := runtime.DeriveTerminalOutcome(aggregate.Decision, aggregate.Termination)
	if err != nil {
		return orchestratorevents.RuntimeTerminalDeliveryRequestData{}, err
	}
	if !terminal {
		return orchestratorevents.RuntimeTerminalDeliveryRequestData{}, fmt.Errorf("%w: runtime terminal delivery requires a sealed terminal", common.ErrConflict)
	}
	deliveryID := runtimecontract.StableTerminalDeliveryID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthoritySnapshot.ID, outcome)
	return orchestratorevents.RuntimeTerminalDeliveryRequestData{
		RuntimeTerminalDeliveryIdentity: runtimecontract.RuntimeTerminalDeliveryIdentity{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, DefinitionRevision: aggregate.Facts.DefinitionRevision,
			EvidenceRevision: aggregate.Facts.EvidenceRevision, DispatchID: aggregate.Decision.AuthoritySnapshot.ID,
			Outcome: outcome, TerminalDeliveryID: deliveryID,
		}, SourceOwner: ModuleID,
		CausationID: "terminal-delivery/" + deliveryID, StateRevision: aggregate.StateRevision,
	}, nil
}

func (m *Biz) deliverRuntimeResult(ctx context.Context, request orchestratorevents.RuntimeTerminalDeliveryRequestData, material resultevents.ResultMaterial) error {
	if m == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	command := resultevents.RuntimeTerminalDeliveryFact{
		RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryIdentity: request.Identity(), SourceOwner: ModuleID, CausationID: request.CausationID},
		Material:                    material,
	}
	event := magicevent.NewEventWithContext(resultevents.TopicResultFactTerminalDelivery, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, resultevents.Contracts(), resultevents.TopicResultFactTerminalDelivery, "terminal-delivery", request.TerminalDeliveryID, request.WorkspaceID, request.RunID)
	response := m.EventHub().Send(event)
	if response == nil || response.Error() != nil {
		if response != nil {
			return response.Error()
		}
		return common.ErrInvalidArgument
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return resultErr
	}
	ownerResult, ok := data.(resultevents.ResultLifecycleCommandResult)
	if !ok {
		return common.ErrInvalidArgument
	}
	return m.recordRuntimeDeliveryAcknowledgement(ctx, ownerResult.DeliveryAck)
}

func (m *Biz) deliverRuntimeWorkspace(ctx context.Context, request orchestratorevents.RuntimeTerminalDeliveryRequestData) error {
	command := workspaceevents.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryIdentity: request.Identity(), SourceOwner: ModuleID, CausationID: request.CausationID}}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceFactTerminalDelivery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, workspaceevents.Contracts(), workspaceevents.TopicWorkspaceFactTerminalDelivery, "workspace", string(request.WorkspaceID), request.WorkspaceID, request.RunID)
	response := m.EventHub().Send(event)
	if response == nil || response.Error() != nil {
		if response != nil {
			return response.Error()
		}
		return common.ErrInvalidArgument
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return resultErr
	}
	ownerResult, ok := data.(workspaceevents.WorkspaceLifecycleCommandResult)
	if !ok {
		return common.ErrInvalidArgument
	}
	return m.recordRuntimeDeliveryAcknowledgement(ctx, ownerResult.DeliveryAck)
}

func (m *Biz) deliverRuntimeRun(ctx context.Context, request orchestratorevents.RuntimeTerminalDeliveryRequestData, resultID common.ResultID) error {
	command := runevents.RuntimeTerminalDeliveryFact{
		RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryIdentity: request.Identity(), SourceOwner: ModuleID, CausationID: request.CausationID},
		ResultID:                    resultID,
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunFactTerminalDelivery, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunFactTerminalDelivery, "run", string(request.RunID), request.WorkspaceID, request.RunID)
	response := m.EventHub().Send(event)
	ownerResult, err := runLifecycleCommandResponse(response, runevents.TopicRunFactTerminalDelivery)
	if err != nil {
		return err
	}
	return m.recordRuntimeDeliveryAcknowledgement(ctx, ownerResult.DeliveryAck)
}

// recordRuntimeDeliveryAcknowledgement 只消费三个 owner 的同步真实回执，不开放公共伪造入口。
func (m *Biz) recordRuntimeDeliveryAcknowledgement(ctx context.Context, acknowledgement *runtimecontract.RuntimeDeliveryAcknowledgement) error {
	if acknowledgement == nil || m == nil || m.runtimeController == nil {
		return common.ErrInvalidArgument
	}
	_, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:     runtime.RuntimeInternalEventTerminalDeliveryAcknowledged,
		Scope:    runtime.RuntimeInternalEventScope{WorkspaceID: acknowledgement.WorkspaceID, RunID: acknowledgement.RunID},
		Delivery: &runtime.RuntimeInternalDeliveryPayload{DeliveryAcknowledgement: acknowledgement},
	})
	return err
}

// runtimeResultMaterialFromAggregate 保留既有调用者的 outcome 反证，再委托唯一 compiler。
func runtimeResultMaterialFromAggregate(aggregate runtime.RuntimeAggregate, outcome runtimecontract.TerminalOutcome) (resultevents.ResultMaterial, error) {
	derived, terminal, err := runtime.DeriveTerminalOutcome(aggregate.Decision, aggregate.Termination)
	if err != nil {
		return resultevents.ResultMaterial{}, err
	}
	if !terminal || derived != outcome.Normalized() {
		return resultevents.ResultMaterial{}, fmt.Errorf("%w: result material terminal outcome does not match aggregate", common.ErrConflict)
	}
	material, err := runtime.DeriveRuntimeTerminalResultMaterial(aggregate)
	if err != nil {
		return resultevents.ResultMaterial{}, err
	}
	return resultevents.ResultMaterialFromRuntimeTerminalMaterial(material), nil
}

func runtimeRequirementOutcomes(aggregate runtime.RuntimeAggregate, ownerSet runtimecontract.RuntimeOwnerRevisionSet) []runtimecontract.RuntimeRequirementOutcome {
	return runtime.DeriveRuntimeTerminalRequirementOutcomes(aggregate, ownerSet)
}
