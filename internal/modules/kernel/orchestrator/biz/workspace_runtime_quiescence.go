package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) beginWorkspaceRuntimeQuiescence(ctx context.Context, command orchestratorevents.BeginWorkspaceRuntimeQuiescenceCommand) (orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult, error) {
	intent := command.Intent.Normalized()
	if err := intent.Validate(); err != nil || m == nil || m.workspaceDeletions == nil {
		return orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult{}, common.ErrInvalidArgument
	}

	m.runtimeMutationFence.Lock()
	defer m.runtimeMutationFence.Unlock()
	m.driveMu.Lock()
	if m.workspaceRuntimeActivityLocked(intent.WorkspaceID) {
		m.driveMu.Unlock()
		return orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult{}, fmt.Errorf("%w: Workspace retains process-local Runtime drive activity", common.ErrConflict)
	}
	m.driveMu.Unlock()

	current, err := m.workspaceDeletions.Get(ctx, intent.WorkspaceID)
	if errors.Is(err, common.ErrNotFound) {
		retirement, retirementErr := queryRunWorkspaceRetirementFact(ctx, m.EventHub(), intent.WorkspaceID, intent.DeletionID)
		if retirementErr != nil {
			return orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult{}, retirementErr
		}
		if err := validateRunDrainingForWorkspaceQuiescence(intent, retirement); err != nil {
			return orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult{}, err
		}
		// Run's draining phase leaves the opaque Runtime CAS lane open. Do not
		// close the Orchestrator gate until that lane has converged every
		// terminal delivery and cleared every persisted lease. Holding the write
		// barrier makes this snapshot atomic with all owner-private Runtime CAS.
		runs, queryErr := queryRunsForWorkspaceQuiescence(ctx, m.EventHub(), intent.WorkspaceID)
		if queryErr != nil {
			return orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult{}, queryErr
		}
		preflightToken, buildErr := m.buildWorkspaceRuntimeQuiescenceToken(ctx, intent, runs)
		if buildErr != nil {
			return orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult{}, buildErr
		}
		if err := validateWorkspaceQuiescenceTokenAgainstRunFact(preflightToken, retirement); err != nil {
			return orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult{}, err
		}
		current, err = m.applyWorkspaceDeletionInternalEvent(ctx, nil, workspaceDeletionInternalEvent{
			Kind:   workspaceDeletionInternalEventQuiescingStarted,
			Intent: intent,
			GateID: orchestratorevents.StableWorkspaceRuntimeQuiescingGateID(intent),
		})
	}
	if err != nil {
		return orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult{}, err
	}
	if current.QuiescenceIntent() != intent || current.QuiescingGateID != orchestratorevents.StableWorkspaceRuntimeQuiescingGateID(intent) {
		return orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult{}, common.ErrConflict
	}
	m.driveMu.Lock()
	if m.quiescingWorkspaces == nil {
		m.quiescingWorkspaces = map[common.WorkspaceID]deletioncontract.WorkspaceRuntimeQuiescenceIntent{}
	}
	m.quiescingWorkspaces[intent.WorkspaceID] = intent
	m.driveMu.Unlock()
	result := orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult{
		WorkspaceID: intent.WorkspaceID, DeletionID: intent.DeletionID, GateID: current.QuiescingGateID,
		OrchestratorQuiescenceRevision: 1,
	}
	return result, result.ValidateFor(command)
}

func validateRunDrainingForWorkspaceQuiescence(intent deletioncontract.WorkspaceRuntimeQuiescenceIntent, retirement runevents.QueryWorkspaceRetirementFactResult) error {
	fence := retirement.Fence.Normalized()
	if err := retirement.ValidateFor(runevents.QueryWorkspaceRetirementFactCommand{WorkspaceID: intent.WorkspaceID, DeletionID: intent.DeletionID}); err != nil ||
		!runWorkspaceRetirementFenceIsDraining(fence) || fence.WorkspaceID != intent.WorkspaceID || fence.DeletionID != intent.DeletionID ||
		fence.DeletionGeneration != intent.DeletionGeneration || fence.DataDispositionIntent != intent.DataDispositionIntent ||
		fence.WorkspaceIntentHash != intent.WorkspaceIntentHash || fence.FenceRevision != intent.RunDrainingRevision ||
		fence.WorkspaceRunGeneration != intent.WorkspaceRunGeneration || fence.QuiescenceID != "" || fence.RuntimeInventoryHash != "" {
		return common.ErrConflict
	}
	return nil
}

func validateWorkspaceQuiescenceTokenAgainstRunFact(token deletioncontract.WorkspaceRuntimeQuiescenceToken, retirement runevents.QueryWorkspaceRetirementFactResult) error {
	if len(token.Runs) != len(retirement.Runs) {
		return common.ErrConflict
	}
	for idx, fact := range token.Runs {
		observed := retirement.Runs[idx]
		if fact.RunID != observed.RunID || fact.WorkspaceRunGeneration != observed.WorkspaceRunGeneration || fact.RunRevision != observed.RunRevision ||
			fact.RunState != observed.RunState || fact.RuntimeStatePresent != observed.RuntimeStatePresent || fact.RuntimeStateRef != observed.RuntimeStateRef ||
			fact.RuntimeStateRevision != observed.RuntimeStateRevision || fact.RuntimeStateHash != observed.RuntimeStateHash || fact.TerminalDeliveryID != observed.TerminalDeliveryID {
			return common.ErrConflict
		}
	}
	return nil
}

func (m *Biz) confirmWorkspaceRuntimeQuiescence(ctx context.Context, command orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceCommand) (orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult, error) {
	intent := command.Intent.Normalized()
	if err := intent.Validate(); err != nil || m == nil || m.workspaceDeletions == nil || m.runtimeController == nil || m.EventHub() == nil {
		return orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult{}, common.ErrInvalidArgument
	}

	m.runtimeMutationFence.Lock()
	defer m.runtimeMutationFence.Unlock()
	current, err := m.workspaceDeletions.Get(ctx, intent.WorkspaceID)
	if err != nil {
		return orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult{}, err
	}
	if current.QuiescenceIntent() != intent {
		return orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult{}, common.ErrConflict
	}
	if current.Phase == orchestratormodel.WorkspaceDeletionPhaseQuiesced || current.Phase == orchestratormodel.WorkspaceDeletionPhaseFenced || current.Phase == orchestratormodel.WorkspaceDeletionPhasePurged {
		result := orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult{Token: deletioncontract.CloneWorkspaceRuntimeQuiescenceToken(*current.QuiescenceToken), OrchestratorQuiescenceRevision: 2}
		return result, result.ValidateFor(command)
	}
	if current.Phase != orchestratormodel.WorkspaceDeletionPhaseQuiescing || current.Revision != 1 {
		return orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult{}, common.ErrConflict
	}
	m.driveMu.Lock()
	if m.workspaceRuntimeActivityLocked(intent.WorkspaceID) {
		m.driveMu.Unlock()
		return orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult{}, fmt.Errorf("%w: Workspace retains process-local Runtime drive activity", common.ErrConflict)
	}
	m.driveMu.Unlock()

	runs, err := queryRunsForWorkspaceQuiescence(ctx, m.EventHub(), intent.WorkspaceID)
	if err != nil {
		return orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult{}, err
	}
	token, err := m.buildWorkspaceRuntimeQuiescenceToken(ctx, intent, runs)
	if err != nil {
		return orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult{}, err
	}
	next, err := m.applyWorkspaceDeletionInternalEvent(ctx, current, workspaceDeletionInternalEvent{
		Kind:   workspaceDeletionInternalEventQuiesced,
		Intent: intent,
		GateID: current.QuiescingGateID,
		Token:  &token,
	})
	if err != nil {
		return orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult{}, err
	}
	result := orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult{Token: deletioncontract.CloneWorkspaceRuntimeQuiescenceToken(*next.QuiescenceToken), OrchestratorQuiescenceRevision: 2}
	return result, result.ValidateFor(command)
}

func (m *Biz) buildWorkspaceRuntimeQuiescenceToken(ctx context.Context, intent deletioncontract.WorkspaceRuntimeQuiescenceIntent, runs []*runmodel.Run) (deletioncontract.WorkspaceRuntimeQuiescenceToken, error) {
	token := deletioncontract.WorkspaceRuntimeQuiescenceToken{
		WorkspaceID: intent.WorkspaceID, DeletionID: intent.DeletionID, DeletionGeneration: intent.DeletionGeneration,
		DataDispositionIntent: intent.DataDispositionIntent, WorkspaceIntentHash: intent.WorkspaceIntentHash,
		RunDrainingRevision: intent.RunDrainingRevision, WorkspaceRunGeneration: intent.WorkspaceRunGeneration,
		OrchestratorQuiescenceRevision: 2,
	}
	for _, item := range runs {
		if item == nil || item.WorkspaceID != intent.WorkspaceID || item.WorkspaceRunGeneration == 0 || item.Revision == 0 || !workspaceQuiescenceTerminalRunState(item.State) {
			return deletioncontract.WorkspaceRuntimeQuiescenceToken{}, common.ErrConflict
		}
		fact := deletioncontract.WorkspaceRuntimeQuiescenceRun{
			RunID: item.ID, WorkspaceRunGeneration: item.WorkspaceRunGeneration, RunRevision: item.Revision, RunState: item.State,
		}
		if item.RuntimeState != nil {
			aggregate, err := m.runtimeController.Load(ctx, intent.WorkspaceID, item.ID)
			if err != nil || aggregate.StateRevision != item.RuntimeState.StateRevision || aggregate.DriveLease != nil {
				if err != nil {
					return deletioncontract.WorkspaceRuntimeQuiescenceToken{}, err
				}
				return deletioncontract.WorkspaceRuntimeQuiescenceToken{}, common.ErrConflict
			}
			delivery, err := terminalDeliveryRequest(aggregate)
			if err != nil || !runtimeTerminalDeliveryFullyAcknowledged(aggregate) {
				if err != nil {
					return deletioncontract.WorkspaceRuntimeQuiescenceToken{}, err
				}
				return deletioncontract.WorkspaceRuntimeQuiescenceToken{}, common.ErrConflict
			}
			record := runtimeStateRecordFromAggregate(aggregate)
			fact.RuntimeStatePresent = true
			fact.RuntimeStateRevision = aggregate.StateRevision
			fact.RuntimeStateRef = runevents.RuntimeStateRef(intent.WorkspaceID, item.ID, aggregate.StateRevision)
			fact.RuntimeStateHash = runtimecontract.StableRuntimeStateRecordHash(record)
			fact.TerminalDeliveryID = delivery.TerminalDeliveryID
			fact.TerminalDeliveryAcked = true
		}
		token.Runs = append(token.Runs, fact)
	}
	token = token.Normalized()
	token.RuntimeInventoryHash = deletioncontract.StableWorkspaceRuntimeInventoryHash(token)
	token.QuiescenceID = deletioncontract.StableWorkspaceRuntimeQuiescenceID(token)
	return token, token.ValidateFor(intent)
}

func queryRunsForWorkspaceQuiescence(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	command := runevents.ListRunsByWorkspaceCommand{WorkspaceID: workspaceID}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandListByWorkspace, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "")
	result, err := runCommandResponse[runevents.ListRunsByWorkspaceResult](hub.Send(event), runevents.TopicRunCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	ret := make([]*runmodel.Run, 0, len(result.Runs))
	for _, item := range result.Runs {
		if item == nil || item.WorkspaceID != workspaceID {
			return nil, common.ErrConflict
		}
		ret = append(ret, runmodel.CloneRun(item))
	}
	sort.Slice(ret, func(i, j int) bool {
		if ret[i].WorkspaceRunGeneration != ret[j].WorkspaceRunGeneration {
			return ret[i].WorkspaceRunGeneration < ret[j].WorkspaceRunGeneration
		}
		return ret[i].ID < ret[j].ID
	})
	return ret, nil
}

func workspaceQuiescenceTerminalRunState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func (m *Biz) recoverWorkspaceRuntimeQuiescenceGates(ctx context.Context) error {
	if m == nil || m.workspaceDeletions == nil {
		return common.ErrInvalidArgument
	}
	items, err := m.workspaceDeletions.List(ctx)
	if err != nil {
		return err
	}
	m.runtimeMutationFence.Lock()
	defer m.runtimeMutationFence.Unlock()
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if m.quiescingWorkspaces == nil {
		m.quiescingWorkspaces = map[common.WorkspaceID]deletioncontract.WorkspaceRuntimeQuiescenceIntent{}
	}
	for _, item := range items {
		if item == nil || item.Validate() != nil {
			return common.ErrConflict
		}
		m.quiescingWorkspaces[item.WorkspaceID] = item.QuiescenceIntent()
	}
	return nil
}

func currentTimestamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
