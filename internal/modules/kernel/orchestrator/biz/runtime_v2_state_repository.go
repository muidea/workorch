package orchestrator

// Runtime V2 state persistence adapter is owned by Biz.

import (
	"context"
	"errors"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeV2StateRepository struct {
	hub magicevent.Hub
}

func (r runtimeV2StateRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	if r.hub == nil || workspaceID == "" || runID == "" {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	run, err := (runEventRepository{hub: r.hub}).GetByWorkspaceID(ctx, workspaceID, runID)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	if run.RuntimeContractVersion != runtimecontract.RuntimeContractVersionV2 || run.RuntimeState == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrNotFound
	}
	record := run.RuntimeState.Clone()
	if record.SchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent {
		return runtimev2.RuntimeAggregate{}, fmt.Errorf("unsupported Runtime V2 state schema %d", record.SchemaVersion)
	}
	aggregate := runtimeAggregateFromRecord(record)
	if err := aggregate.Validate(); err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (r runtimeV2StateRepository) Save(ctx context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	if err := aggregate.Validate(); err != nil {
		return err
	}
	run, err := (runEventRepository{hub: r.hub}).GetByWorkspaceID(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		return err
	}
	record := runtimeStateRecordFromAggregate(aggregate)
	run.RuntimeContractVersion = runtimecontract.RuntimeContractVersionV2
	run.RuntimeState = &record
	command := updateRunCommandFromModel(run)
	command.ExpectedRuntimeStateRevision = &expectedRevision
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandUpdate, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, runevents.TopicRunCommandUpdate, "run", string(run.ID), run.WorkspaceID, run.ID)
	_, err = runCommandResponse(r.hub.Send(event), runevents.TopicRunCommandUpdate)
	if errors.Is(err, common.ErrConflict) {
		return fmt.Errorf("runtime state save at expected revision %d: %w", expectedRevision, common.ErrConflict)
	}
	return err
}

func runtimeStateRecordFromAggregate(aggregate runtimev2.RuntimeAggregate) runtimecontract.RuntimeStateRecord {
	ret := runtimecontract.RuntimeStateRecord{
		SchemaVersion:               runtimecontract.RuntimeStateSchemaVersionCurrent,
		StateRevision:               aggregate.StateRevision,
		EvidenceSequence:            aggregate.EvidenceSequence,
		Definition:                  aggregate.Definition,
		Facts:                       aggregate.Facts,
		Decision:                    aggregate.Decision,
		ProcessedEventIDs:           append([]string(nil), aggregate.ProcessedEventIDs...),
		CandidateAttempts:           append([]runtimecontract.RuntimeCandidateAttemptRecord(nil), aggregate.CandidateAttempts...),
		GovernanceRequests:          append([]runtimecontract.RuntimeGovernanceRequestRecord(nil), aggregate.GovernanceRequests...),
		GovernanceResolutions:       append([]runtimecontract.RuntimeGovernanceResolutionRecord(nil), aggregate.GovernanceResolutions...),
		OperationGovernanceBindings: cloneRuntimeOperationGovernanceBindings(aggregate.OperationGovernanceBindings),
		OperationReceipts:           runtimecontract.NormalizeRuntimeOperationReceipts(aggregate.OperationReceipts),
	}
	if aggregate.Termination != nil {
		term := aggregate.Termination.Clone()
		ret.Termination = &term
	}
	if aggregate.ActiveDispatch != nil {
		value := runtimeDispatchRecordFromAggregate(*aggregate.ActiveDispatch)
		ret.ActiveDispatch = &value
	}
	ret.CompletedDispatches = make([]runtimecontract.RuntimeDispatchRecord, len(aggregate.CompletedDispatches))
	for idx := range aggregate.CompletedDispatches {
		ret.CompletedDispatches[idx] = runtimeDispatchRecordFromAggregate(aggregate.CompletedDispatches[idx])
	}
	ret.Diagnostics = make([]runtimecontract.RuntimeDiagnosticRecord, len(aggregate.Diagnostics))
	for idx, diagnostic := range aggregate.Diagnostics {
		ret.Diagnostics[idx] = runtimecontract.RuntimeDiagnosticRecord{
			EventID: diagnostic.EventID, AuthoritySnapshotID: diagnostic.AuthoritySnapshotID,
			CurrentSnapshotID: diagnostic.CurrentSnapshotID, DispatchID: diagnostic.DispatchID, Reason: diagnostic.Reason,
		}
	}
	return ret.Clone()
}

func runtimeAggregateFromRecord(record runtimecontract.RuntimeStateRecord) runtimev2.RuntimeAggregate {
	ret := runtimev2.RuntimeAggregate{
		StateRevision:               record.StateRevision,
		EvidenceSequence:            record.EvidenceSequence,
		Definition:                  record.Definition,
		Facts:                       record.Facts,
		Decision:                    record.Decision,
		ProcessedEventIDs:           append([]string(nil), record.ProcessedEventIDs...),
		CandidateAttempts:           append([]runtimecontract.RuntimeCandidateAttemptRecord(nil), record.CandidateAttempts...),
		GovernanceRequests:          append([]runtimecontract.RuntimeGovernanceRequestRecord(nil), record.GovernanceRequests...),
		GovernanceResolutions:       append([]runtimecontract.RuntimeGovernanceResolutionRecord(nil), record.GovernanceResolutions...),
		OperationGovernanceBindings: cloneRuntimeOperationGovernanceBindings(record.OperationGovernanceBindings),
		OperationReceipts:           runtimecontract.NormalizeRuntimeOperationReceipts(record.OperationReceipts),
	}
	if record.Termination != nil {
		term := record.Termination.Clone()
		ret.Termination = &term
	}
	if record.ActiveDispatch != nil {
		value := runtimeDispatchFromRecord(*record.ActiveDispatch)
		ret.ActiveDispatch = &value
	}
	ret.CompletedDispatches = make([]runtimev2.SealedDispatch, len(record.CompletedDispatches))
	for idx := range record.CompletedDispatches {
		ret.CompletedDispatches[idx] = runtimeDispatchFromRecord(record.CompletedDispatches[idx])
	}
	ret.Diagnostics = make([]runtimev2.DiagnosticEvidence, len(record.Diagnostics))
	for idx, diagnostic := range record.Diagnostics {
		ret.Diagnostics[idx] = runtimev2.DiagnosticEvidence{
			EventID: diagnostic.EventID, AuthoritySnapshotID: diagnostic.AuthoritySnapshotID,
			CurrentSnapshotID: diagnostic.CurrentSnapshotID, DispatchID: diagnostic.DispatchID, Reason: diagnostic.Reason,
		}
	}
	return ret
}

func cloneRuntimeOperationGovernanceBindings(values []runtimecontract.RuntimeOperationGovernanceBinding) []runtimecontract.RuntimeOperationGovernanceBinding {
	if len(values) == 0 {
		return nil
	}
	ret := make([]runtimecontract.RuntimeOperationGovernanceBinding, len(values))
	for idx := range values {
		ret[idx] = values[idx].Clone()
	}
	return ret
}

func runtimeDispatchRecordFromAggregate(dispatch runtimev2.SealedDispatch) runtimecontract.RuntimeDispatchRecord {
	return runtimecontract.RuntimeDispatchRecord{
		ID: dispatch.ID, WorkspaceID: dispatch.WorkspaceID, RunID: dispatch.RunID,
		Transaction: dispatch.Transaction, CandidateHash: dispatch.CandidateHash, Candidate: dispatch.Candidate,
		State: runtimecontract.RuntimeDispatchState(dispatch.State), CompletedOperationIDs: append([]string(nil), dispatch.CompletedOperationIDs...),
		OperationExecutions: runtimecontract.NormalizeRuntimeOperationExecutionRecords(dispatch.OperationExecutions),
	}
}

func runtimeDispatchFromRecord(dispatch runtimecontract.RuntimeDispatchRecord) runtimev2.SealedDispatch {
	return runtimev2.SealedDispatch{
		ID: dispatch.ID, WorkspaceID: dispatch.WorkspaceID, RunID: dispatch.RunID,
		Transaction: dispatch.Transaction, CandidateHash: dispatch.CandidateHash, Candidate: dispatch.Candidate,
		State: runtimev2.DispatchState(dispatch.State), CompletedOperationIDs: append([]string(nil), dispatch.CompletedOperationIDs...),
		OperationExecutions: runtimecontract.NormalizeRuntimeOperationExecutionRecords(dispatch.OperationExecutions),
	}
}
