package orchestrator

// Runtime state persistence adapter is owned by Biz.

import (
	"context"
	"runtime/trace"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeStateRepository struct {
	hub magicevent.Hub
}

func (r runtimeStateRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	return r.loadRuntimeStateRecord(ctx, workspaceID, runID, "", 0)
}

// LoadRuntimeRecoveryRecord binds startup recovery to the opaque Run-owner
// identity returned by its inventory. The Run owner still does not interpret
// aggregate meaning; it only proves that the exact persisted record remains
// current while Orchestrator performs the SM-4 classification.
func (r runtimeStateRepository) LoadRuntimeRecoveryRecord(ctx context.Context, item runevents.RuntimeRecoveryInventoryItem) (runtime.RuntimeAggregate, error) {
	if !item.HasRuntimeState || item.WorkspaceID == "" || item.RunID == "" || item.RuntimeStateRevision == 0 || item.RuntimeStateRef == "" {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return r.loadRuntimeStateRecord(ctx, item.WorkspaceID, item.RunID, item.RuntimeStateRef, item.RuntimeStateRevision)
}

func (r runtimeStateRepository) loadRuntimeStateRecord(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, expectedRef string, expectedRevision uint64) (runtime.RuntimeAggregate, error) {
	record, err := r.readRuntimeStateOriginal(ctx, workspaceID, runID, expectedRef, expectedRevision)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	return runtime.ReadCommittedRuntimeOwnerStateRecord(record)

}

func (r runtimeStateRepository) readRuntimeStateOriginal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, expectedRef string, expectedRevision uint64) (runtimecontract.RuntimeStateRecord, error) {
	region := startRuntimeTraceRegion(ctx, "workorch.runtime.run_query")
	defer region.End()
	if r.hub == nil || workspaceID == "" || runID == "" {
		return runtimecontract.RuntimeStateRecord{}, common.ErrInvalidArgument
	}
	command := runevents.QueryRuntimeStateRecordCommand{
		WorkspaceID: workspaceID, RunID: runID,
		ExpectedRuntimeStateRef: expectedRef, ExpectedRuntimeStateRevision: expectedRevision,
	}
	if err := command.Validate(); err != nil {
		return runtimecontract.RuntimeStateRecord{}, err
	}
	event := magicevent.NewEventWithContext(
		runevents.TopicRunCommandRuntimeStateRecord,
		ModuleID,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		command,
	)
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandRuntimeStateRecord, "run", string(runID), workspaceID, runID)
	result := r.hub.Send(event)
	if result == nil {
		return runtimecontract.RuntimeStateRecord{}, common.ErrInvalidArgument
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return runtimecontract.RuntimeStateRecord{}, resultErr
	}
	response, ok := data.(runevents.QueryRuntimeStateRecordResult)
	if !ok || response.Validate() != nil || response.WorkspaceID != workspaceID || response.RunID != runID {
		return runtimecontract.RuntimeStateRecord{}, common.ErrInvalidArgument
	}
	if !response.StatePresent || response.RuntimeState == nil {
		return runtimecontract.RuntimeStateRecord{}, common.ErrNotFound
	}
	// Preserve the exact owner DTO, never Normalized: both complete-load paths
	// validate it before accepting any independently detached converted aggregate.
	return *response.RuntimeState, nil
}

func (r runtimeStateRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	if err := aggregate.Validate(); err != nil {
		return err
	}
	return r.SaveValidated(ctx, aggregate, expectedRevision)
}

// SaveValidated is used only by RuntimeTransitionCoordinator after the
// Orchestrator owner has validated the complete cross-SM aggregate. Keeping
// this separate from Save avoids repeating that expensive validation while
// preserving Save as a safe repository trust boundary for direct callers.
func (r runtimeStateRepository) SaveValidated(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	region := startRuntimeTraceRegion(ctx, "workorch.runtime.run_cas")
	defer region.End()
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	record := runtimeStateRecordFromAggregate(aggregate)
	revision := expectedRevision
	command := runevents.StoreRuntimeStateRecordCommand{
		RunID:                        aggregate.Facts.RunID,
		WorkspaceID:                  aggregate.Facts.WorkspaceID,
		RuntimeState:                 &record,
		ExpectedRuntimeStateRevision: &revision,
	}
	needsFence, err := runtimeStateRequiresLifecycleFence(&record)
	if err != nil {
		return err
	}
	if needsFence {
		run, readErr := (runSnapshotReader{hub: r.hub}).ReadLifecycle(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
		if readErr != nil {
			return readErr
		}
		command.LifecycleFence, err = runtimeLifecycleFenceForState(run.WorkspaceID, run.RunID, record, run.BootstrapHash)
		if err != nil {
			return err
		}
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandStoreRuntimeStateRecord, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandStoreRuntimeStateRecord, "run", string(command.RunID), command.WorkspaceID, command.RunID)
	response, err := runCommandResponse[runevents.StoreRuntimeStateRecordResult](r.hub.Send(event), runevents.TopicRunCommandStoreRuntimeStateRecord)
	if err != nil {
		return err
	}
	if response.RuntimeStateUpdate == nil {
		return common.ErrInvalidArgument
	}
	outcome := *response.RuntimeStateUpdate
	if err := outcome.Validate(); err != nil {
		return err
	}
	switch outcome.Feedback.StableCategory {
	case runtimecontract.RuntimeFeedbackCategoryAccepted:
		return nil
	case runtimecontract.RuntimeFeedbackCategoryAggregateRevisionMismatch:
		return &runtime.RuntimeCASConflictError{
			WorkspaceID: outcome.WorkspaceID, RunID: outcome.RunID,
			ExpectedRevision: outcome.ExpectedRuntimeRevision, ObservedRevision: outcome.ObservedRuntimeRevision,
		}
	case runtimecontract.RuntimeFeedbackCategorySemanticRejection:
		return &runtime.RuntimeSemanticRejectionError{
			WorkspaceID: outcome.WorkspaceID, RunID: outcome.RunID,
			ExpectedRevision: outcome.ExpectedRuntimeRevision, ObservedRevision: outcome.ObservedRuntimeRevision,
			Feedback: outcome.Feedback.CloneOriginal(),
		}
	case runtimecontract.RuntimeFeedbackCategoryOwnerTransient:
		return &runtime.RuntimeOwnerTransientError{
			WorkspaceID: outcome.WorkspaceID, RunID: outcome.RunID,
			ExpectedRevision: outcome.ExpectedRuntimeRevision, ObservedRevision: outcome.ObservedRuntimeRevision,
			Feedback: outcome.Feedback.CloneOriginal(),
		}
	default:
		return common.ErrInvalidArgument
	}
}

func runtimeStateRecordFromAggregate(aggregate runtime.RuntimeAggregate) runtimecontract.RuntimeStateRecord {
	return runtime.RuntimeStateRecordFromAggregate(aggregate)
}

func runtimeAggregateFromRecord(record runtimecontract.RuntimeStateRecord) runtime.RuntimeAggregate {
	return runtime.RuntimeAggregateFromRecord(record)
}

// Trace is diagnostic only; a nil business context must not become a new panic
// when optional tracing is enabled. The original context is never replaced.
func startRuntimeTraceRegion(ctx context.Context, name string) *trace.Region {
	if ctx == nil {
		ctx = context.Background()
	}
	return trace.StartRegion(ctx, name)
}
