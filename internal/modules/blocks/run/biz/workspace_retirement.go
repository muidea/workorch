package run

import (
	"context"
	"sort"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const workspaceRetirementCommandSource = "blocks/workspace"
const workspaceRetirementFactQuerySource = "kernel/orchestrator"

func (m *Biz) executeWorkspaceRetirementCommand(ctx context.Context, ev magicevent.Event) (any, error) {
	if m == nil || ev == nil || m.runs == nil {
		return nil, common.ErrInvalidArgument
	}
	switch ev.ID() {
	case runevents.TopicRunCommandWorkspaceRetirementFact:
		cmd, ok := ev.Data().(runevents.QueryWorkspaceRetirementFactCommand)
		if !ok || ev.Source() != workspaceRetirementFactQuerySource {
			return nil, common.ErrInvalidArgument
		}
		return m.queryWorkspaceRetirementFact(ctx, cmd)
	case runevents.TopicRunCommandPrepareWorkspaceRetirement:
		cmd, ok := ev.Data().(runevents.PrepareWorkspaceRetirementCommand)
		if !ok || ev.Source() != workspaceRetirementCommandSource {
			return nil, common.ErrInvalidArgument
		}
		cmd = cmd.Normalized()
		if err := cmd.Validate(); err != nil {
			return nil, err
		}
		m.workspaceExecutionMu.Lock()
		defer m.workspaceExecutionMu.Unlock()
		internal, err := m.submitWorkspaceRetirementInternalEvent(ctx, workspaceRetirementInternalEvent{Kind: workspaceRetirementInternalEventPrepare, Prepare: &cmd})
		if err != nil {
			return nil, err
		}
		result := runevents.PrepareWorkspaceRetirementResult{Fence: runWorkspaceRetirementFenceFromModel(*internal.Fence)}
		if err := result.ValidateFor(cmd); err != nil {
			return nil, err
		}
		return result, nil
	case runevents.TopicRunCommandFinalizeWorkspaceRetirement:
		cmd, ok := ev.Data().(runevents.FinalizeWorkspaceRetirementCommand)
		if !ok || ev.Source() != workspaceRetirementCommandSource {
			return nil, common.ErrInvalidArgument
		}
		cmd = cmd.Normalized()
		if err := cmd.Validate(); err != nil {
			return nil, err
		}
		m.workspaceExecutionMu.Lock()
		defer m.workspaceExecutionMu.Unlock()
		internal, err := m.submitWorkspaceRetirementInternalEvent(ctx, workspaceRetirementInternalEvent{Kind: workspaceRetirementInternalEventFinalize, Finalize: &cmd})
		if err != nil {
			return nil, err
		}
		result := runevents.FinalizeWorkspaceRetirementResult{Fence: runWorkspaceRetirementFenceFromModel(*internal.Fence)}
		if err := result.ValidateFor(cmd); err != nil {
			return nil, err
		}
		return result, nil
	case runevents.TopicRunCommandPurgeWorkspace:
		cmd, ok := ev.Data().(runevents.PurgeWorkspaceRunsCommand)
		if !ok || ev.Source() != workspaceRetirementCommandSource {
			return nil, common.ErrInvalidArgument
		}
		cmd = cmd.Normalized()
		if err := cmd.Validate(); err != nil {
			return nil, err
		}
		m.workspaceExecutionMu.Lock()
		defer m.workspaceExecutionMu.Unlock()
		internal, err := m.submitWorkspaceRetirementInternalEvent(ctx, workspaceRetirementInternalEvent{
			Kind: workspaceRetirementInternalEventPurge, Purge: &cmd,
		})
		if err != nil {
			return nil, err
		}
		if internal.PurgeReceipt == nil || internal.PurgeReceipt.ValidateFor(runWorkspaceRetirementFenceToModel(cmd.RetirementFence())) != nil {
			return nil, common.ErrConflict
		}
		observations := append([]runmodel.WorkspaceRetirementPurgeObservation(nil), internal.PurgeReceipt.Observations...)
		result := runevents.PurgeWorkspaceRunsResult{
			WorkspaceID: cmd.WorkspaceID, DeletionID: cmd.DeletionID, DeletionGeneration: cmd.DeletionGeneration, FenceRevision: cmd.FenceRevision,
			WorkspaceRunGeneration: cmd.WorkspaceRunGeneration, QuiescenceID: cmd.QuiescenceID,
			PurgeID:        runevents.StableRunWorkspacePurgeID(cmd),
			AckHash:        runevents.StableRunWorkspacePurgeAckHashForObservations(cmd, observations),
			PurgeProofHash: runevents.StableRunWorkspacePurgeProofHashForObservations(cmd, observations), Observations: observations,
		}
		if err := result.ValidateFor(cmd); err != nil {
			return nil, err
		}
		return result, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (m *Biz) queryWorkspaceRetirementFact(ctx context.Context, command runevents.QueryWorkspaceRetirementFactCommand) (runevents.QueryWorkspaceRetirementFactResult, error) {
	command = command.Normalized()
	if err := command.Validate(); err != nil {
		return runevents.QueryWorkspaceRetirementFactResult{}, err
	}
	fence, err := m.runs.GetWorkspaceRetirement(ctx, command.WorkspaceID)
	if err != nil {
		return runevents.QueryWorkspaceRetirementFactResult{}, err
	}
	if command.DeletionID != "" && fence.DeletionID != command.DeletionID {
		return runevents.QueryWorkspaceRetirementFactResult{}, common.ErrConflict
	}
	items, err := m.runs.ListByWorkspaceID(ctx, command.WorkspaceID)
	if err != nil {
		return runevents.QueryWorkspaceRetirementFactResult{}, err
	}
	if err := runmodel.WorkspaceRetirementEligible(items, command.WorkspaceID); err != nil {
		return runevents.QueryWorkspaceRetirementFactResult{}, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].WorkspaceRunGeneration < items[j].WorkspaceRunGeneration })
	result := runevents.QueryWorkspaceRetirementFactResult{Fence: runWorkspaceRetirementFenceFromModel(*fence)}
	result.Runs = make([]runevents.RunWorkspaceRetirementRuntimeFact, 0, len(items))
	for _, item := range items {
		if err := runmodel.ValidateWorkspaceRetirementRuntimePhysicalState(item, command.WorkspaceID); err != nil {
			return runevents.QueryWorkspaceRetirementFactResult{}, err
		}
		fact := runevents.RunWorkspaceRetirementRuntimeFact{
			RunID: item.ID, WorkspaceRunGeneration: item.WorkspaceRunGeneration, RunRevision: item.Revision, RunState: item.State,
		}
		if item.RuntimeState != nil {
			fact.RuntimeStatePresent = true
			fact.RuntimeStateRevision = item.RuntimeState.StateRevision
			fact.RuntimeStateRef = item.RuntimeLifecycleFence.RuntimeStateRef
			fact.RuntimeStateHash = item.RuntimeLifecycleFence.RuntimeStateHash
			fact.TerminalDeliveryID = item.TerminalDelivery.TerminalDeliveryID
		}
		result.Runs = append(result.Runs, fact)
	}
	if err := result.ValidateFor(command); err != nil {
		return runevents.QueryWorkspaceRetirementFactResult{}, err
	}
	return result, nil
}

func runWorkspaceRetirementFenceFromModel(fence runmodel.WorkspaceRetirementFence) runevents.RunWorkspaceRetirementFence {
	fence = fence.Normalized()
	return runevents.RunWorkspaceRetirementFence{
		WorkspaceID: fence.WorkspaceID, DeletionID: fence.DeletionID, DeletionGeneration: fence.DeletionGeneration,
		DataDispositionIntent: fence.DataDispositionIntent, WorkspaceIntentHash: fence.WorkspaceIntentHash, Phase: fence.Phase,
		FenceRevision: fence.FenceRevision, WorkspaceRunGeneration: fence.WorkspaceRunGeneration,
		QuiescenceID: fence.QuiescenceID, RuntimeInventoryHash: fence.RuntimeInventoryHash,
	}
}

func runWorkspaceRetirementFenceToModel(fence runevents.RunWorkspaceRetirementFence) runmodel.WorkspaceRetirementFence {
	fence = fence.Normalized()
	return runmodel.WorkspaceRetirementFence{
		WorkspaceID: fence.WorkspaceID, DeletionID: fence.DeletionID, DeletionGeneration: fence.DeletionGeneration,
		DataDispositionIntent: fence.DataDispositionIntent, WorkspaceIntentHash: fence.WorkspaceIntentHash, Phase: fence.Phase,
		FenceRevision: fence.FenceRevision, WorkspaceRunGeneration: fence.WorkspaceRunGeneration,
		QuiescenceID: fence.QuiescenceID, RuntimeInventoryHash: fence.RuntimeInventoryHash,
	}
}

func buildWorkspaceRetirementInventory(items []runmodel.WorkspaceRetirementFence, command runevents.QueryWorkspaceRetirementInventoryCommand) (runevents.QueryWorkspaceRetirementInventoryResult, error) {
	limit := command.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	cursor := strings.TrimSpace(command.Cursor)
	sort.Slice(items, func(i, j int) bool { return string(items[i].WorkspaceID) < string(items[j].WorkspaceID) })
	result := runevents.QueryWorkspaceRetirementInventoryResult{}
	for _, item := range items {
		fact := runWorkspaceRetirementFenceFromModel(item)
		if string(fact.WorkspaceID) <= cursor {
			continue
		}
		if len(result.Items) == limit {
			result.NextCursor = string(result.Items[len(result.Items)-1].WorkspaceID)
			break
		}
		result.Items = append(result.Items, fact)
	}
	if err := result.Validate(); err != nil {
		return runevents.QueryWorkspaceRetirementInventoryResult{}, err
	}
	return result, nil
}
