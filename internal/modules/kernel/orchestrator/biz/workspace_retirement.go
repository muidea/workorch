package orchestrator

import (
	"context"
	"errors"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const workspaceRetirementOrchestratorCommandSource = "blocks/workspace"

func (m *Biz) handleWorkspaceRuntimeQuiescence(event magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if event.Source() != workspaceRetirementOrchestratorCommandSource || m == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	var response any
	var err error
	switch event.ID() {
	case orchestratorevents.TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence:
		command, ok := event.Data().(orchestratorevents.BeginWorkspaceRuntimeQuiescenceCommand)
		if !ok || command.Validate() != nil {
			err = common.ErrInvalidArgument
			break
		}
		response, err = m.beginWorkspaceRuntimeQuiescence(event.Context(), command)
	case orchestratorevents.TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence:
		command, ok := event.Data().(orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceCommand)
		if !ok || command.Validate() != nil {
			err = common.ErrInvalidArgument
			break
		}
		response, err = m.confirmWorkspaceRuntimeQuiescence(event.Context(), command)
	default:
		err = common.ErrInvalidArgument
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Biz) handleWorkspaceDeletion(event magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if event.Source() != workspaceRetirementOrchestratorCommandSource || m == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	response, err := m.executeWorkspaceDeletionCommand(event.Context(), event)
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Biz) workspaceRuntimeActivityLocked(workspaceID common.WorkspaceID) bool {
	prefix := string(workspaceID) + "\x00"
	for key := range m.drivingRuns {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	for key := range m.retryLeaseTimers {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	for key := range m.persistedLeaseMounts {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (m *Biz) workspaceRetirementGate(workspaceID common.WorkspaceID) (runevents.RunWorkspaceRetirementFence, bool) {
	if m == nil || workspaceID == "" {
		return runevents.RunWorkspaceRetirementFence{}, false
	}
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	fence, ok := m.retiredWorkspaces[workspaceID]
	return fence, ok
}

func (m *Biz) observeDurableWorkspaceRetirement(ctx context.Context, workspaceID common.WorkspaceID) (bool, error) {
	fence, observed, err := m.observeWorkspaceRetirementFence(ctx, workspaceID)
	return observed && fence.Phase == runmodel.WorkspaceRetirementPhaseRetired, err
}

// observeWorkspaceRetirementFence imports only the Run owner's typed fact
// into a process-local admission cache. The cache never becomes authority:
// Run still validates every opaque CAS, and deletion seal re-queries the exact
// retired fact. A cached draining fact is intentionally conservative if Run
// advances to retired between observations.
func (m *Biz) observeWorkspaceRetirementFence(ctx context.Context, workspaceID common.WorkspaceID) (runevents.RunWorkspaceRetirementFence, bool, error) {
	if m == nil || workspaceID == "" || m.EventHub() == nil {
		return runevents.RunWorkspaceRetirementFence{}, false, common.ErrInvalidArgument
	}
	if fence, observed := m.workspaceRetirementGate(workspaceID); observed {
		return fence, true, nil
	}
	retirement, err := queryRunWorkspaceRetirementFact(ctx, m.EventHub(), workspaceID, "")
	if errors.Is(err, common.ErrNotFound) {
		return runevents.RunWorkspaceRetirementFence{}, false, nil
	}
	if err != nil {
		return runevents.RunWorkspaceRetirementFence{}, false, err
	}
	m.driveMu.Lock()
	if m.retiredWorkspaces == nil {
		m.retiredWorkspaces = make(map[common.WorkspaceID]runevents.RunWorkspaceRetirementFence)
	}
	m.retiredWorkspaces[workspaceID] = retirement.Fence
	m.driveMu.Unlock()
	return retirement.Fence, true, nil
}

func runWorkspaceRetirementFenceIsRetired(fence runevents.RunWorkspaceRetirementFence) bool {
	return fence.Phase == runmodel.WorkspaceRetirementPhaseRetired
}

func runWorkspaceRetirementFenceIsDraining(fence runevents.RunWorkspaceRetirementFence) bool {
	return fence.Phase == runmodel.WorkspaceRetirementPhaseDraining
}

func (m *Biz) validateWorkspaceRetirementDriveAdmission(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, leaseGeneration uint64) error {
	fence, observed, err := m.observeWorkspaceRetirementFence(ctx, workspaceID)
	if err != nil || !observed {
		return err
	}
	if runWorkspaceRetirementFenceIsRetired(fence) {
		return common.ErrNotFound
	}
	if !runWorkspaceRetirementFenceIsDraining(fence) {
		return common.ErrConflict
	}
	aggregate, err := m.runtimeController.Load(ctx, workspaceID, runID)
	if err != nil {
		return err
	}
	if _, err := terminalDeliveryRequest(aggregate); err != nil || runtimeTerminalDeliveryFullyAcknowledged(aggregate) {
		return common.ErrConflict
	}
	if aggregate.DriveLease == nil {
		if leaseGeneration != 0 {
			return common.ErrConflict
		}
		return nil
	}
	if aggregate.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry {
		return common.ErrConflict
	}
	if leaseGeneration != 0 && aggregate.DriveLease.RecoveryGeneration != leaseGeneration {
		return common.ErrConflict
	}
	return nil
}

func (m *Biz) validateWorkspaceRetirementLeaseTimerAdmission(ctx context.Context, lease runtimecontract.RuntimeDriveLease) error {
	fence, observed, err := m.observeWorkspaceRetirementFence(ctx, lease.WorkspaceID)
	if err != nil || !observed {
		return err
	}
	if runWorkspaceRetirementFenceIsRetired(fence) {
		return common.ErrNotFound
	}
	if !runWorkspaceRetirementFenceIsDraining(fence) || lease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry {
		return common.ErrConflict
	}
	aggregate, err := m.runtimeController.Load(ctx, lease.WorkspaceID, lease.RunID)
	if err != nil {
		return err
	}
	if aggregate.DriveLease == nil || aggregate.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled || !runtimecontract.SameRuntimeDriveLeaseIdentity(*aggregate.DriveLease, lease) {
		return common.ErrConflict
	}
	return nil
}

// withOrchestratorEvidenceMutation holds the same owner barrier as Runtime
// CAS for the complete evidence repository transaction. It also imports the
// Run owner's typed draining fact before entering the repository, closing the
// coordinator window before Orchestrator's own durable deletion row exists.
func (m *Biz) withOrchestratorEvidenceMutation(ctx context.Context, workspaceID common.WorkspaceID, mutation func(context.Context) error) error {
	if m == nil || workspaceID == "" || mutation == nil {
		return common.ErrInvalidArgument
	}
	m.runtimeMutationFence.RLock()
	defer m.runtimeMutationFence.RUnlock()
	if _, quiescing := m.quiescingWorkspaces[workspaceID]; quiescing {
		return common.ErrNotFound
	}
	fence, observed, err := m.observeWorkspaceRetirementFence(ctx, workspaceID)
	if err != nil {
		return err
	}
	if observed {
		if runWorkspaceRetirementFenceIsRetired(fence) {
			return common.ErrNotFound
		}
		return common.ErrConflict
	}
	return mutation(ctx)
}

// withOrchestratorWorkspaceResourceRead hides Orchestrator-owned Runtime
// resource projections once the durable private gate exists. It shares the
// mutation barrier with Begin so a read cannot straddle the quiescing
// linearization point and expose a retained or purged Workspace afterward.
func (m *Biz) withOrchestratorWorkspaceResourceRead(ctx context.Context, workspaceID common.WorkspaceID, read func(context.Context) error) error {
	if m == nil || workspaceID == "" || read == nil || m.workspaceDeletions == nil {
		return common.ErrInvalidArgument
	}
	m.runtimeMutationFence.RLock()
	defer m.runtimeMutationFence.RUnlock()
	if _, err := m.workspaceDeletions.Get(ctx, workspaceID); err == nil {
		return common.ErrNotFound
	} else if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return read(ctx)
}

func queryRunWorkspaceRetirementFact(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, deletionID string) (runevents.QueryWorkspaceRetirementFactResult, error) {
	if hub == nil || workspaceID == "" {
		return runevents.QueryWorkspaceRetirementFactResult{}, common.ErrInvalidArgument
	}
	command := runevents.QueryWorkspaceRetirementFactCommand{WorkspaceID: workspaceID, DeletionID: deletionID}.Normalized()
	if err := command.Validate(); err != nil {
		return runevents.QueryWorkspaceRetirementFactResult{}, err
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandWorkspaceRetirementFact, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandWorkspaceRetirementFact, "workspace", string(workspaceID), workspaceID, "")
	response, err := runCommandResponse[runevents.QueryWorkspaceRetirementFactResult](hub.Send(event), runevents.TopicRunCommandWorkspaceRetirementFact)
	if err != nil {
		return runevents.QueryWorkspaceRetirementFactResult{}, err
	}
	if err := response.ValidateFor(command); err != nil {
		return runevents.QueryWorkspaceRetirementFactResult{}, err
	}
	return response, nil
}

func (m *Biz) recoverWorkspaceRetirementGates(ctx context.Context) error {
	items, err := queryRunWorkspaceRetirementInventory(ctx, m.EventHub())
	if err != nil {
		return err
	}
	for _, fence := range items {
		retirement, err := queryRunWorkspaceRetirementFact(ctx, m.EventHub(), fence.WorkspaceID, fence.DeletionID)
		if err != nil {
			return err
		}
		if !runevents.SameRunWorkspaceRetirementFence(retirement.Fence, fence) {
			return common.ErrConflict
		}
		m.driveMu.Lock()
		m.retiredWorkspaces[fence.WorkspaceID] = retirement.Fence
		m.driveMu.Unlock()
	}
	return nil
}

func queryRunWorkspaceRetirementInventory(ctx context.Context, hub magicevent.Hub) ([]runevents.RunWorkspaceRetirementFence, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]runevents.RunWorkspaceRetirementFence, 0)
	cursor := ""
	for page := 0; page < runtimeRecoveryInventoryMaxPages; page++ {
		command := runevents.QueryWorkspaceRetirementInventoryCommand{Cursor: cursor, Limit: runtimeRecoveryInventoryPageSize}
		event := magicevent.NewEventWithContext(runevents.TopicRunCommandWorkspaceRetirementInventory, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		result := hub.Send(event)
		if result == nil {
			return nil, common.ErrInvalidArgument
		}
		data, resultErr := result.Get()
		if resultErr != nil {
			return nil, resultErr
		}
		response, ok := data.(runevents.QueryWorkspaceRetirementInventoryResult)
		if !ok || response.Validate() != nil {
			return nil, common.ErrInvalidArgument
		}
		if len(response.Items) > runtimeRecoveryInventoryMaxItems-len(ret) {
			return nil, common.ErrConflict
		}
		ret = append(ret, response.Items...)
		next := strings.TrimSpace(response.NextCursor)
		if next == "" {
			return ret, nil
		}
		if next <= cursor {
			return nil, common.ErrConflict
		}
		cursor = next
	}
	return nil, common.ErrConflict
}
