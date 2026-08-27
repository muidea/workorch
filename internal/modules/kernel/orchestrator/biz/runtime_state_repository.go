package orchestrator

// Runtime state persistence adapter is owned by Biz.

import (
	"context"
	"fmt"

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
	if r.hub == nil || workspaceID == "" || runID == "" {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	command := runevents.QueryRuntimeStateRecordCommand{
		WorkspaceID: workspaceID, RunID: runID,
		ExpectedRuntimeStateRef: expectedRef, ExpectedRuntimeStateRevision: expectedRevision,
	}
	if err := command.Validate(); err != nil {
		return runtime.RuntimeAggregate{}, err
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
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return runtime.RuntimeAggregate{}, resultErr
	}
	response, ok := data.(runevents.QueryRuntimeStateRecordResult)
	if !ok || response.Validate() != nil || response.WorkspaceID != workspaceID || response.RunID != runID {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if !response.StatePresent || response.RuntimeState == nil {
		return runtime.RuntimeAggregate{}, common.ErrNotFound
	}
	// Validate the exact owner-returned DTO before Clone/Normalized can erase a
	// missing or malformed current-schema authority field.
	if err := response.RuntimeState.Validate(); err != nil {
		return runtime.RuntimeAggregate{}, fmt.Errorf("load Runtime state: %w", err)
	}
	record := response.RuntimeState.Clone()
	aggregate := runtimeAggregateFromRecord(record)
	if err := aggregate.Validate(); err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (r runtimeStateRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	if err := aggregate.Validate(); err != nil {
		return err
	}
	run, err := (runSnapshotReader{hub: r.hub}).Read(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		return err
	}
	record := runtimeStateRecordFromAggregate(aggregate)
	run.RuntimeState = &record
	command, err := storeRuntimeStateRecordCommandFromModel(run)
	if err != nil {
		return err
	}
	command.ExpectedRuntimeStateRevision = &expectedRevision
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandStoreRuntimeStateRecord, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandStoreRuntimeStateRecord, "run", string(run.ID), run.WorkspaceID, run.ID)
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
	ret := runtimecontract.RuntimeStateRecord{
		StateRevision:               aggregate.StateRevision,
		EvidenceSequence:            aggregate.EvidenceSequence,
		Definition:                  aggregate.Definition,
		Facts:                       aggregate.Facts,
		SemanticPlanning:            aggregate.SemanticPlanning.Clone(),
		Reconciliation:              aggregate.Reconciliation.Clone(),
		CoverageProofLineage:        runtimecontract.NormalizeRuntimeCoverageProofLineage(aggregate.CoverageProofLineage),
		Decision:                    aggregate.Decision,
		ProcessedEventIDs:           append([]string(nil), aggregate.ProcessedEventIDs...),
		RepairSurfaceConfirmations:  cloneRuntimeRepairSurfaceConfirmations(aggregate.RepairSurfaceConfirmations),
		CandidateRequestSessions:    cloneRuntimeCandidateRequestSessions(aggregate.CandidateRequestSessions),
		CandidateAttempts:           append([]runtimecontract.RuntimeCandidateAttemptRecord(nil), aggregate.CandidateAttempts...),
		ExecutionBudget:             aggregate.ExecutionBudget.Clone(),
		RecoverableWaits:            cloneRuntimeRecoverableWaits(aggregate.RecoverableWaits),
		GovernanceRequests:          append([]runtimecontract.RuntimeGovernanceRequestRecord(nil), aggregate.GovernanceRequests...),
		GovernanceResolutions:       runtimecontract.CloneRuntimeGovernanceResolutionRecords(aggregate.GovernanceResolutions),
		OperationGovernanceBindings: cloneRuntimeOperationGovernanceBindings(aggregate.OperationGovernanceBindings),
		OperationReceipts:           runtimecontract.NormalizeRuntimeOperationReceipts(aggregate.OperationReceipts),
		DeliveryAcknowledgements:    append([]runtimecontract.RuntimeDeliveryAcknowledgement(nil), aggregate.DeliveryAcknowledgements...),
		PrerequisiteInvalidations:   runtimecontract.NormalizeRuntimePrerequisiteInvalidations(aggregate.PrerequisiteInvalidations),
	}
	if aggregate.DefinitionClarification != nil {
		value := aggregate.DefinitionClarification.Clone()
		ret.DefinitionClarification = &value
	}
	if aggregate.DriveLease != nil {
		lease := aggregate.DriveLease.Clone()
		ret.DriveLease = &lease
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
	ret.Diagnostics = append([]runtimecontract.RuntimeDiagnosticRecord(nil), aggregate.Diagnostics...)
	return ret.Clone()
}

func runtimeAggregateFromRecord(record runtimecontract.RuntimeStateRecord) runtime.RuntimeAggregate {
	ret := runtime.RuntimeAggregate{
		StateRevision:               record.StateRevision,
		EvidenceSequence:            record.EvidenceSequence,
		Definition:                  record.Definition,
		Facts:                       record.Facts,
		SemanticPlanning:            record.SemanticPlanning.Clone(),
		Reconciliation:              record.Reconciliation.Clone(),
		CoverageProofLineage:        runtimecontract.NormalizeRuntimeCoverageProofLineage(record.CoverageProofLineage),
		Decision:                    record.Decision,
		ProcessedEventIDs:           append([]string(nil), record.ProcessedEventIDs...),
		RepairSurfaceConfirmations:  cloneRuntimeRepairSurfaceConfirmations(record.RepairSurfaceConfirmations),
		CandidateRequestSessions:    cloneRuntimeCandidateRequestSessions(record.CandidateRequestSessions),
		CandidateAttempts:           append([]runtimecontract.RuntimeCandidateAttemptRecord(nil), record.CandidateAttempts...),
		ExecutionBudget:             record.ExecutionBudget.Clone(),
		RecoverableWaits:            cloneRuntimeRecoverableWaits(record.RecoverableWaits),
		GovernanceRequests:          append([]runtimecontract.RuntimeGovernanceRequestRecord(nil), record.GovernanceRequests...),
		GovernanceResolutions:       runtimecontract.CloneRuntimeGovernanceResolutionRecords(record.GovernanceResolutions),
		OperationGovernanceBindings: cloneRuntimeOperationGovernanceBindings(record.OperationGovernanceBindings),
		OperationReceipts:           runtimecontract.NormalizeRuntimeOperationReceipts(record.OperationReceipts),
		DeliveryAcknowledgements:    append([]runtimecontract.RuntimeDeliveryAcknowledgement(nil), record.DeliveryAcknowledgements...),
		PrerequisiteInvalidations:   runtimecontract.NormalizeRuntimePrerequisiteInvalidations(record.PrerequisiteInvalidations),
	}
	if record.DefinitionClarification != nil {
		value := record.DefinitionClarification.Clone()
		ret.DefinitionClarification = &value
	}
	if record.DriveLease != nil {
		lease := record.DriveLease.Clone()
		ret.DriveLease = &lease
	}
	if record.Termination != nil {
		term := record.Termination.Clone()
		ret.Termination = &term
	}
	if record.ActiveDispatch != nil {
		value := runtimeDispatchFromRecord(*record.ActiveDispatch)
		ret.ActiveDispatch = &value
	}
	ret.CompletedDispatches = make([]runtime.SealedDispatch, len(record.CompletedDispatches))
	for idx := range record.CompletedDispatches {
		ret.CompletedDispatches[idx] = runtimeDispatchFromRecord(record.CompletedDispatches[idx])
	}
	ret.Diagnostics = append([]runtimecontract.RuntimeDiagnosticRecord(nil), record.Diagnostics...)
	return ret
}

func cloneRuntimeRecoverableWaits(values []runtimecontract.RuntimeRecoverableWaitRecord) []runtimecontract.RuntimeRecoverableWaitRecord {
	if values == nil {
		return nil
	}
	ret := make([]runtimecontract.RuntimeRecoverableWaitRecord, len(values))
	for idx := range values {
		ret[idx] = values[idx].Clone()
	}
	return ret
}

func cloneRuntimeRepairSurfaceConfirmations(values []runtimecontract.RuntimeRepairSurfaceConfirmationRecord) []runtimecontract.RuntimeRepairSurfaceConfirmationRecord {
	if len(values) == 0 {
		return nil
	}
	ret := make([]runtimecontract.RuntimeRepairSurfaceConfirmationRecord, len(values))
	for idx, value := range values {
		ret[idx] = value.Clone()
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

// cloneRuntimeCandidateRequestSessions keeps the Run-owner record conversion
// ownership-neutral. Request-session state is advanced only by runtime's
// controller; this adapter merely preserves its persisted value across a
// file-backed reload and must never share fragment metadata backing arrays.
func cloneRuntimeCandidateRequestSessions(values []runtimecontract.RuntimeCandidateRequestSession) []runtimecontract.RuntimeCandidateRequestSession {
	if len(values) == 0 {
		return nil
	}
	ret := make([]runtimecontract.RuntimeCandidateRequestSession, len(values))
	for idx := range values {
		ret[idx] = values[idx].Clone()
	}
	return ret
}

func runtimeDispatchRecordFromAggregate(dispatch runtime.SealedDispatch) runtimecontract.RuntimeDispatchRecord {
	return runtimecontract.RuntimeDispatchRecord{
		ID: dispatch.ID, WorkspaceID: dispatch.WorkspaceID, RunID: dispatch.RunID,
		Transaction: dispatch.Transaction, CandidateHash: dispatch.CandidateHash, Candidate: dispatch.Candidate,
		FocusTargetRole:                    dispatch.FocusTargetRole,
		ValidationRecipes:                  runtimecontract.CloneRuntimeValidationRecipes(dispatch.ValidationRecipes),
		OperationAdmissibilities:           runtimecontract.CloneRuntimeOperationAdmissibilities(dispatch.OperationAdmissibilities),
		OperationAdmissibilityObservations: runtimecontract.CloneRuntimePrerequisiteObservations(dispatch.OperationAdmissibilityObservations),
		OperationPolicies:                  runtimecontract.NormalizeRuntimeOperationPolicyFacts(dispatch.OperationPolicies),
		State:                              runtimecontract.RuntimeDispatchState(dispatch.State), CompletedOperationIDs: append([]string(nil), dispatch.CompletedOperationIDs...),
		OperationExecutions: runtimecontract.CloneRuntimeOperationExecutionRecords(dispatch.OperationExecutions), OperationAuthorityHash: dispatch.OperationAuthorityHash,
		DispatchAuthorityHash: dispatch.DispatchAuthorityHash,
	}
}

func runtimeDispatchFromRecord(dispatch runtimecontract.RuntimeDispatchRecord) runtime.SealedDispatch {
	return runtime.SealedDispatch{
		ID: dispatch.ID, WorkspaceID: dispatch.WorkspaceID, RunID: dispatch.RunID,
		Transaction: dispatch.Transaction, CandidateHash: dispatch.CandidateHash, Candidate: dispatch.Candidate,
		FocusTargetRole:                    dispatch.FocusTargetRole,
		ValidationRecipes:                  runtimecontract.CloneRuntimeValidationRecipes(dispatch.ValidationRecipes),
		OperationAdmissibilities:           runtimecontract.CloneRuntimeOperationAdmissibilities(dispatch.OperationAdmissibilities),
		OperationAdmissibilityObservations: runtimecontract.CloneRuntimePrerequisiteObservations(dispatch.OperationAdmissibilityObservations),
		OperationPolicies:                  runtimecontract.NormalizeRuntimeOperationPolicyFacts(dispatch.OperationPolicies),
		State:                              runtime.DispatchState(dispatch.State), CompletedOperationIDs: append([]string(nil), dispatch.CompletedOperationIDs...),
		OperationExecutions: runtimecontract.CloneRuntimeOperationExecutionRecords(dispatch.OperationExecutions), OperationAuthorityHash: dispatch.OperationAuthorityHash,
		DispatchAuthorityHash: dispatch.DispatchAuthorityHash,
	}
}
