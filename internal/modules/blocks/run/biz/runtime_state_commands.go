package run

import (
	"context"
	"errors"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) executeRuntimeStateStoreCommand(ctx context.Context, ev magicevent.Event) (runevents.StoreRuntimeStateRecordResult, error) {
	cmd, ok := ev.Data().(runevents.StoreRuntimeStateRecordCommand)
	if !ok || cmd.RunID == "" || cmd.WorkspaceID == "" || ev.Source() != "kernel/orchestrator" {
		return runevents.StoreRuntimeStateRecordResult{}, common.ErrInvalidArgument
	}
	base, err := m.runs.GetRuntimeStateWriteBase(ctx, cmd.WorkspaceID, cmd.RunID)
	if err != nil {
		return runevents.StoreRuntimeStateRecordResult{}, err
	}
	return m.updateRuntimeState(ctx, base, cmd)
}

// updateRuntimeState persists an Orchestrator-owned aggregate through the
// Run owner's physical record. SM-1 mutations can legitimately change the
// record revision while the aggregate revision remains unchanged. In that
// case rebase the opaque RuntimeState write on the latest Run record; only an
// aggregate revision mismatch is a semantic conflict that returns to the
// Orchestrator SM-4 recovery owner.
func (m *Biz) updateRuntimeState(ctx context.Context, base runmodel.RuntimeStateWriteBase, cmd runevents.StoreRuntimeStateRecordCommand) (runevents.StoreRuntimeStateRecordResult, error) {
	const maxPhysicalCASRebases = 4
	for attempt := 0; attempt < maxPhysicalCASRebases; attempt++ {
		current := base.Run
		if current == nil {
			return runtimeStateUpdateOutcome(nil, cmd, runtimemodel.RuntimeFeedbackCategoryOwnerTransient, 0, common.ErrNotFound), nil
		}
		if err := validateRunUpdate(current, cmd); err != nil {
			return runtimeStateUpdateOutcome(current, cmd, runtimemodel.RuntimeFeedbackCategorySemanticRejection, base.RuntimeStateRevision, err), nil
		}
		currentRevision := base.RuntimeStateRevision
		if currentRevision != *cmd.ExpectedRuntimeStateRevision {
			return runtimeStateUpdateOutcome(current, cmd, runtimemodel.RuntimeFeedbackCategoryAggregateRevisionMismatch, currentRevision, nil), nil
		}
		item := runFromUpdateCommand(current, cmd)
		err := m.runs.CompareAndSwapRuntimeState(ctx, item, current.Revision, currentRevision)
		if err == nil {
			return runtimeStateUpdateOutcome(item, cmd, runtimemodel.RuntimeFeedbackCategoryAccepted, item.RuntimeState.StateRevision, nil), nil
		}
		if errors.Is(err, runmodel.ErrRuntimeStateIntegrity) {
			return runtimeStateUpdateOutcome(current, cmd, runtimemodel.RuntimeFeedbackCategorySemanticRejection, currentRevision, err), nil
		}
		if !errors.Is(err, common.ErrConflict) {
			return runtimeStateUpdateOutcome(current, cmd, runtimemodel.RuntimeFeedbackCategoryOwnerTransient, currentRevision, err), nil
		}
		// A physical CAS conflict may be a concurrent SM-1/model write. Reload
		// and retry only while the semantic aggregate fence is still current.
		base, err = m.runs.GetRuntimeStateWriteBase(ctx, cmd.WorkspaceID, cmd.RunID)
		if err != nil {
			return runtimeStateUpdateOutcome(nil, cmd, runtimemodel.RuntimeFeedbackCategoryOwnerTransient, currentRevision, err), nil
		}
	}
	return runtimeStateUpdateOutcome(base.Run, cmd, runtimemodel.RuntimeFeedbackCategoryOwnerTransient, base.RuntimeStateRevision, common.ErrConflict), nil
}

func runFromUpdateCommand(current *runmodel.Run, cmd runevents.StoreRuntimeStateRecordCommand) *runmodel.Run {
	if current == nil {
		return nil
	}
	item := *current
	// The command owns its detached input until this synchronous call returns.
	// This local wrapper never mutates it. The physical repository alone clones
	// the new graph before publication; another intermediate copy has no owner.
	item.RuntimeState = cmd.RuntimeState
	if cmd.LifecycleFence != nil {
		fence := cmd.LifecycleFence.Normalized()
		item.RuntimeLifecycleFence = &fence
	} else {
		item.RuntimeLifecycleFence = nil
	}
	return &item
}

func validateRunUpdate(current *runmodel.Run, cmd runevents.StoreRuntimeStateRecordCommand) error {
	if current == nil || current.ID != cmd.RunID || current.WorkspaceID != cmd.WorkspaceID {
		return common.ErrConflict
	}
	if current.RuntimeBootstrap == nil || cmd.RuntimeState == nil || cmd.ExpectedRuntimeStateRevision == nil {
		return common.ErrConflict
	}
	if cmd.LifecycleFence != nil {
		fence := cmd.LifecycleFence.Normalized()
		if err := fence.Validate(); err != nil || fence.WorkspaceID != cmd.WorkspaceID || fence.RunID != cmd.RunID ||
			fence.RuntimeStateRevision != cmd.RuntimeState.StateRevision || fence.RuntimeStateRef != runevents.RuntimeStateRef(cmd.WorkspaceID, cmd.RunID, cmd.RuntimeState.StateRevision) ||
			fence.BootstrapHash != runtimemodel.StableRuntimeBootstrapHash(*current.RuntimeBootstrap) {
			return common.ErrConflict
		}
	}
	return nil
}

// ValidateRunUpdate exposes the Run owner's SM-1 persistence invariant for
// focused contract tests; it does not alter owner state.
func ValidateRunUpdate(current *runmodel.Run, cmd runevents.StoreRuntimeStateRecordCommand) error {
	return validateRunUpdate(current, cmd)
}
