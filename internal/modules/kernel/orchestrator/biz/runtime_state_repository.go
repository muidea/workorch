package orchestrator

// Runtime state persistence adapter is owned by Biz.

import (
	"context"
	"fmt"
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

// LoadOwnerStateOriginal is only used by the Orchestrator's lexical private
// event or query compiler. It performs a real Run-owner query; it supplies no success
// credential. Ordinary Load and recovery keep both independent complete gates.
func (r runtimeStateRepository) LoadOwnerStateOriginal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimecontract.RuntimeStateRecord, runtime.RuntimeAggregate, error) {
	record, err := r.readRuntimeStateOriginal(ctx, workspaceID, runID, "", 0)
	if err != nil {
		return runtimecontract.RuntimeStateRecord{}, runtime.RuntimeAggregate{}, err
	}
	return record, runtimeAggregateFromRecord(record), nil
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
	aggregate, err := r.loadRuntimeStateOriginal(ctx, workspaceID, runID, expectedRef, expectedRevision)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	if err := aggregate.Validate(); err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// LoadTransitionPreimage still performs a real owner read and the complete
// raw DTO gate. The joint compiler shares only exact-raw pure subresults with
// the following complete aggregate gate; ordinary Load/recovery never borrow it.
func (r runtimeStateRepository) LoadTransitionPreimage(ctx context.Context, request runtime.RuntimeTransitionPreimageRequest) (runtime.RuntimeTransitionPreimage, error) {
	workspaceID, runID, _ := request.Scope()
	record, err := r.readRuntimeStateOriginal(ctx, workspaceID, runID, "", 0)
	if err != nil {
		return runtime.RuntimeTransitionPreimage{}, err
	}
	return request.CompileOwnerRecord(record, runtimeAggregateFromRecord(record))
}

func (r runtimeStateRepository) loadRuntimeStateOriginal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, expectedRef string, expectedRevision uint64) (runtime.RuntimeAggregate, error) {
	record, err := r.readRuntimeStateOriginal(ctx, workspaceID, runID, expectedRef, expectedRevision)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	// Ordinary Load/recovery retain their independent complete DTO gate.
	if err := record.Validate(); err != nil {
		return runtime.RuntimeAggregate{}, fmt.Errorf("load Runtime state: %w", err)
	}
	return runtimeAggregateFromRecord(record), nil
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
		run, readErr := (runSnapshotReader{hub: r.hub}).Read(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
		if readErr != nil {
			return readErr
		}
		run.RuntimeState = &record
		command.LifecycleFence, err = runtimeLifecycleFenceFromModel(run)
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
	semantic := (runtimecontract.RuntimeStateRecord{SemanticPlanning: aggregate.SemanticPlanning}).Clone().SemanticPlanning
	ret := runtimecontract.RuntimeStateRecord{
		ResultFeedbackVersion:           aggregate.ResultFeedbackVersion,
		StateRevision:                   aggregate.StateRevision,
		EvidenceSequence:                aggregate.EvidenceSequence,
		Definition:                      aggregate.Definition,
		Facts:                           aggregate.Facts,
		SemanticPlanning:                semantic.Clone(),
		ImplementationWork:              aggregate.ImplementationWork.Clone(),
		Reconciliation:                  aggregate.Reconciliation.Clone(),
		CoverageProofLineage:            runtimecontract.NormalizeRuntimeCoverageProofLineage(aggregate.CoverageProofLineage),
		Decision:                        aggregate.Decision,
		ProcessedEventIDs:               append([]string(nil), aggregate.ProcessedEventIDs...),
		RepairSurfaceConfirmations:      cloneRuntimeRepairSurfaceConfirmations(aggregate.RepairSurfaceConfirmations),
		CandidateRequestSessions:        cloneRuntimeCandidateRequestSessions(aggregate.CandidateRequestSessions),
		CandidateAttempts:               append([]runtimecontract.RuntimeCandidateAttemptRecord(nil), aggregate.CandidateAttempts...),
		ExecutionBudget:                 aggregate.ExecutionBudget.Clone(),
		RecoverableWaits:                cloneRuntimeRecoverableWaits(aggregate.RecoverableWaits),
		OperationRecoveryAuthorizations: cloneRuntimeOperationRecoveryAuthorizations(aggregate.OperationRecoveryAuthorizations),
		GovernanceRequests:              append([]runtimecontract.RuntimeGovernanceRequestRecord(nil), aggregate.GovernanceRequests...),
		GovernanceResolutions:           runtimecontract.CloneRuntimeGovernanceResolutionRecords(aggregate.GovernanceResolutions),
		OperationGovernanceBindings:     cloneRuntimeOperationGovernanceBindings(aggregate.OperationGovernanceBindings),
		OperationReceipts:               runtimecontract.NormalizeRuntimeOperationReceipts(aggregate.OperationReceipts),
		DeliveryAcknowledgements:        cloneRuntimeDeliveryAcknowledgements(aggregate.DeliveryAcknowledgements),
		PrerequisiteInvalidations:       runtimecontract.NormalizeRuntimePrerequisiteInvalidations(aggregate.PrerequisiteInvalidations),
	}
	if aggregate.TerminalMaterialSeal != nil {
		value := aggregate.TerminalMaterialSeal.CloneRaw()
		ret.TerminalMaterialSeal = &value
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
	detachRuntimeStateConversionRawFields(&ret)
	return ret
}

// Other fields were already detached by their original projection helpers.
// Copy only the still-raw graph here, avoiding a second whole-record clone.
// The partial record's raw Clone preserves original nil/empty and bytes; this
// helper does not validate or normalize any authority.
func detachRuntimeStateConversionRawFields(ret *runtimecontract.RuntimeStateRecord) {
	raw := (runtimecontract.RuntimeStateRecord{
		Definition: ret.Definition, Facts: ret.Facts, Decision: ret.Decision,
		CandidateAttempts: ret.CandidateAttempts, GovernanceRequests: ret.GovernanceRequests, Diagnostics: ret.Diagnostics,
	}).Clone()
	ret.Definition, ret.Facts, ret.Decision = raw.Definition, raw.Facts, raw.Decision
	ret.CandidateAttempts, ret.GovernanceRequests, ret.Diagnostics = raw.CandidateAttempts, raw.GovernanceRequests, raw.Diagnostics
}

func runtimeAggregateFromRecord(record runtimecontract.RuntimeStateRecord) runtime.RuntimeAggregate {
	raw := runtimecontract.RuntimeStateRecord{
		Definition: record.Definition, Facts: record.Facts, Decision: record.Decision,
		CandidateAttempts:  append([]runtimecontract.RuntimeCandidateAttemptRecord(nil), record.CandidateAttempts...),
		GovernanceRequests: append([]runtimecontract.RuntimeGovernanceRequestRecord(nil), record.GovernanceRequests...),
		Diagnostics:        append([]runtimecontract.RuntimeDiagnosticRecord(nil), record.Diagnostics...),
	}
	detachRuntimeStateConversionRawFields(&raw)
	// The existing semantic Clone preserves raw diagnostics in its return, but
	// its comparison normalization writes into the rejection/gap backing slices.
	// Keep that normalization confined to an independent input graph as before.
	raw.SemanticPlanning = (runtimecontract.RuntimeStateRecord{SemanticPlanning: record.SemanticPlanning}).Clone().SemanticPlanning
	ret := runtime.RuntimeAggregate{
		ResultFeedbackVersion:           record.ResultFeedbackVersion,
		StateRevision:                   record.StateRevision,
		EvidenceSequence:                record.EvidenceSequence,
		Definition:                      raw.Definition,
		Facts:                           raw.Facts,
		SemanticPlanning:                raw.SemanticPlanning.Clone(),
		ImplementationWork:              record.ImplementationWork.Clone(),
		Reconciliation:                  record.Reconciliation.Clone(),
		CoverageProofLineage:            runtimecontract.NormalizeRuntimeCoverageProofLineage(record.CoverageProofLineage),
		Decision:                        raw.Decision,
		ProcessedEventIDs:               append([]string(nil), record.ProcessedEventIDs...),
		RepairSurfaceConfirmations:      cloneRuntimeRepairSurfaceConfirmations(record.RepairSurfaceConfirmations),
		CandidateRequestSessions:        cloneRuntimeCandidateRequestSessions(record.CandidateRequestSessions),
		CandidateAttempts:               raw.CandidateAttempts,
		ExecutionBudget:                 record.ExecutionBudget.Clone(),
		RecoverableWaits:                cloneRuntimeRecoverableWaits(record.RecoverableWaits),
		OperationRecoveryAuthorizations: cloneRuntimeOperationRecoveryAuthorizations(record.OperationRecoveryAuthorizations),
		GovernanceRequests:              raw.GovernanceRequests,
		GovernanceResolutions:           runtimecontract.CloneRuntimeGovernanceResolutionRecords(record.GovernanceResolutions),
		OperationGovernanceBindings:     cloneRuntimeOperationGovernanceBindings(record.OperationGovernanceBindings),
		OperationReceipts:               runtimecontract.NormalizeRuntimeOperationReceipts(record.OperationReceipts),
		DeliveryAcknowledgements:        cloneRuntimeDeliveryAcknowledgements(record.DeliveryAcknowledgements),
		PrerequisiteInvalidations:       runtimecontract.NormalizeRuntimePrerequisiteInvalidations(record.PrerequisiteInvalidations),
	}
	if record.TerminalMaterialSeal != nil {
		value := record.TerminalMaterialSeal.CloneRaw()
		ret.TerminalMaterialSeal = &value
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
	ret.Diagnostics = raw.Diagnostics
	return ret
}

func cloneRuntimeDeliveryAcknowledgements(values []runtimecontract.RuntimeDeliveryAcknowledgement) []runtimecontract.RuntimeDeliveryAcknowledgement {
	if values == nil {
		return nil
	}
	ret := make([]runtimecontract.RuntimeDeliveryAcknowledgement, len(values))
	for index := range values {
		ret[index] = values[index].CloneRaw()
	}
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

func cloneRuntimeOperationRecoveryAuthorizations(values []runtimecontract.RuntimeOperationRecoveryAuthorization) []runtimecontract.RuntimeOperationRecoveryAuthorization {
	if values == nil {
		return nil
	}
	ret := make([]runtimecontract.RuntimeOperationRecoveryAuthorization, len(values))
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

func cloneRuntimeImplementationWorkCommitmentRecord(round *runtimecontract.RuntimeImplementationWorkRound) *runtimecontract.RuntimeImplementationWorkRound {
	if round == nil {
		return nil
	}
	copy := round.Clone()
	return &copy
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
		Transaction: dispatch.Transaction, CandidateHash: dispatch.CandidateHash, Candidate: dispatch.Candidate.CloneRaw(),
		CandidateManifestHash:              dispatch.CandidateManifestHash,
		ImplementationWorkCommitment:       cloneRuntimeImplementationWorkCommitmentRecord(dispatch.ImplementationWorkCommitment),
		CoverageBindingHash:                dispatch.CoverageBindingHash,
		CoverageBindings:                   runtimecontract.CloneCoverageEvidenceBindings(dispatch.CoverageBindings),
		OperationTargetAuthorizations:      runtimecontract.CloneRuntimeDispatchOperationTargetAuthorizations(dispatch.OperationTargetAuthorizations),
		FocusTargetRole:                    dispatch.FocusTargetRole,
		ValidationRecipes:                  runtimecontract.CloneRuntimeValidationRecipes(dispatch.ValidationRecipes),
		RepairCommitmentHash:               dispatch.RepairCommitmentHash,
		RepairCommitment:                   cloneRuntimeDispatchRepairCommitmentRecord(dispatch.RepairCommitment),
		AssessmentAuthorityInputHash:       dispatch.AssessmentAuthorityInputHash,
		AssessmentAuthorityInput:           cloneRuntimeAssessmentAuthorityInputRecord(dispatch.AssessmentAuthorityInput),
		AssessmentAdjudicationHash:         dispatch.AssessmentAdjudicationHash,
		AssessmentAdjudication:             cloneRuntimeAssessmentAdjudicationRecord(dispatch.AssessmentAdjudication),
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
		Transaction: dispatch.Transaction, CandidateHash: dispatch.CandidateHash, Candidate: dispatch.Candidate.CloneRaw(),
		CandidateManifestHash:              dispatch.CandidateManifestHash,
		ImplementationWorkCommitment:       cloneRuntimeImplementationWorkCommitmentRecord(dispatch.ImplementationWorkCommitment),
		CoverageBindingHash:                dispatch.CoverageBindingHash,
		CoverageBindings:                   runtimecontract.CloneCoverageEvidenceBindings(dispatch.CoverageBindings),
		OperationTargetAuthorizations:      runtimecontract.CloneRuntimeDispatchOperationTargetAuthorizations(dispatch.OperationTargetAuthorizations),
		FocusTargetRole:                    dispatch.FocusTargetRole,
		ValidationRecipes:                  runtimecontract.CloneRuntimeValidationRecipes(dispatch.ValidationRecipes),
		RepairCommitmentHash:               dispatch.RepairCommitmentHash,
		RepairCommitment:                   cloneRuntimeDispatchRepairCommitmentRecord(dispatch.RepairCommitment),
		AssessmentAuthorityInputHash:       dispatch.AssessmentAuthorityInputHash,
		AssessmentAuthorityInput:           cloneRuntimeAssessmentAuthorityInputRecord(dispatch.AssessmentAuthorityInput),
		AssessmentAdjudicationHash:         dispatch.AssessmentAdjudicationHash,
		AssessmentAdjudication:             cloneRuntimeAssessmentAdjudicationRecord(dispatch.AssessmentAdjudication),
		OperationAdmissibilities:           runtimecontract.CloneRuntimeOperationAdmissibilities(dispatch.OperationAdmissibilities),
		OperationAdmissibilityObservations: runtimecontract.CloneRuntimePrerequisiteObservations(dispatch.OperationAdmissibilityObservations),
		OperationPolicies:                  runtimecontract.NormalizeRuntimeOperationPolicyFacts(dispatch.OperationPolicies),
		State:                              runtime.DispatchState(dispatch.State), CompletedOperationIDs: append([]string(nil), dispatch.CompletedOperationIDs...),
		OperationExecutions: runtimecontract.CloneRuntimeOperationExecutionRecords(dispatch.OperationExecutions), OperationAuthorityHash: dispatch.OperationAuthorityHash,
		DispatchAuthorityHash: dispatch.DispatchAuthorityHash,
	}
}

func cloneRuntimeDispatchRepairCommitmentRecord(value *runtimecontract.RuntimeDispatchRepairCommitment) *runtimecontract.RuntimeDispatchRepairCommitment {
	if value == nil {
		return nil
	}
	copy := value.Clone()
	return &copy
}

func cloneRuntimeAssessmentAuthorityInputRecord(value *runtimecontract.RuntimeAssessmentAuthorityInput) *runtimecontract.RuntimeAssessmentAuthorityInput {
	if value == nil {
		return nil
	}
	copy := value.Clone()
	return &copy
}

func cloneRuntimeAssessmentAdjudicationRecord(value *runtimecontract.RuntimeAssessmentAdjudication) *runtimecontract.RuntimeAssessmentAdjudication {
	if value == nil {
		return nil
	}
	copy := value.Clone()
	return &copy
}

// Trace is diagnostic only; a nil business context must not become a new panic
// when optional tracing is enabled. The original context is never replaced.
func startRuntimeTraceRegion(ctx context.Context, name string) *trace.Region {
	if ctx == nil {
		ctx = context.Background()
	}
	return trace.StartRegion(ctx, name)
}
