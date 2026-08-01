package orchestrator

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// reconcileRuntimeWorkspace persists the request and a complete batch of
// current owner observations for the activation. It never infers an
// observation from persisted coverage: an unavailable owner is represented by
// a typed unverifiable fact so the SM-2 reducer can reopen/prerequisite it.
func (m *Biz) reconcileRuntimeWorkspace(ctx context.Context, run *runmodel.Run, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, bool, error) {
	return m.reconcileRuntimeWorkspaceWithCommand(ctx, run, aggregate, orchestratorevents.DriveRuntimeCommand{})
}

func (m *Biz) reconcileRuntimeWorkspaceWithCommand(ctx context.Context, run *runmodel.Run, aggregate runtime.RuntimeAggregate, command orchestratorevents.DriveRuntimeCommand) (runtime.RuntimeAggregate, bool, error) {
	if m == nil || run == nil || aggregate.Facts.ReconciliationContract.SchemaVersion == "" {
		return aggregate, false, nil
	}
	request := runtimeReconciliationRequestForRun(run, aggregate, command.ReconciliationRequestID, command.ReconciliationGeneration)
	// Once the request has been sealed, subsequent transitions of the same
	// drive must keep using that exact generation until its batch commits.  A
	// continuation command only creates a new generation when its request ID
	// differs from the current persisted request.
	if aggregate.Reconciliation.Request != nil {
		current := aggregate.Reconciliation.Request.Normalized()
		if strings.TrimSpace(command.ReconciliationRequestID) == "" || current.RequestID == request.RequestID {
			request = current
		}
	}
	if aggregate.Reconciliation.Request == nil {
		result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind:        runtime.RuntimeInternalEventReconciliationRequested,
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			ReconciliationRequest: &request,
		})
		return result.Aggregate, true, err
	}
	if aggregate.Reconciliation.Batch != nil && !reconciliationRequestNeedsRefresh(aggregate, command) {
		return aggregate, false, nil
	}
	var deliverableDirectory, internalDirectory string
	workspace, workspaceErr := (workspaceEventRepository{hub: m.EventHub()}).Get(ctx, aggregate.Facts.WorkspaceID)
	if workspaceErr == nil && workspace != nil {
		deliverableDirectory = strings.TrimSpace(workspace.DeliverableDirectory)
		internalDirectory = strings.TrimSpace(workspace.InternalDirectory)
	}
	batch := m.queryRuntimeReconciliationBatch(ctx, request, aggregate, deliverableDirectory, internalDirectory, workspaceErr)
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventReconciliationBatchObserved,
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		ReconciliationBatch: &batch,
	})
	return result.Aggregate, true, err
}

func reconciliationRequestNeedsRefresh(aggregate runtime.RuntimeAggregate, command orchestratorevents.DriveRuntimeCommand) bool {
	requestID := strings.TrimSpace(command.ReconciliationRequestID)
	if requestID == "" {
		return false
	}
	scope := runtimecontract.RuntimeCWIReconciliationScopeCurrentRun
	if aggregate.Reconciliation.Request != nil && aggregate.Reconciliation.Request.Scope != "" {
		scope = aggregate.Reconciliation.Request.Scope
	}
	requestedID := "runtime-reconciliation/resume/" + requestID + "/" + string(scope)
	for _, history := range aggregate.Reconciliation.History {
		if history.Request.RequestID == requestedID {
			// This continuation was already sealed in an older generation. A
			// duplicate must not rewind the current ledger or query owners with
			// an obsolete request identity.
			return false
		}
	}
	if aggregate.Reconciliation.Request == nil {
		return true
	}
	current := aggregate.Reconciliation.Request.Normalized()
	return current.RequestID != requestedID || (command.ReconciliationGeneration != 0 && current.Generation != command.ReconciliationGeneration)
}

// queryRuntimeReconciliationBatch asks each declared binding owner for one
// typed current-workspace observation. The command carries only the sealed
// proof surface; no owner can select a Runtime action or mutate coverage.
func (m *Biz) queryRuntimeReconciliationBatch(ctx context.Context, request runtimecontract.RuntimeCWIReconciliationRequest, aggregate runtime.RuntimeAggregate, deliverableDirectory, internalDirectory string, workspaceErr error) runtimecontract.RuntimeCWIReconciliationBatch {
	batch := runtimecontract.RuntimeCWIReconciliationBatch{
		RequestID: request.RequestID, WorkspaceID: request.WorkspaceID, RunID: request.RunID,
		Generation: request.Generation, BindingSetHash: request.BindingSetHash,
	}
	// Reconciliation runs before ordinary prerequisite acquisition. The
	// activation-sealed declarations provide the executable recipe identity;
	// any already-confirmed Shell inventory is merged in so the owner can use
	// its current policy without making the persisted inventory a second source
	// of binding authority.
	validationRecipes := runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(aggregate.Facts.Activation.EvidencePolicies)
	for _, recipe := range runtimecontract.RuntimeValidationRecipesFromOperationAdmissibilities(aggregate.Facts.Resources.OperationAdmissibilities) {
		validationRecipes = append(validationRecipes, recipe)
	}
	validationRecipes = runtimecontract.NormalizeRuntimeValidationRecipes(validationRecipes)
	for _, binding := range request.Bindings {
		for _, owner := range binding.OwnerModuleIDs {
			command := orchestratorevents.QueryRuntimeCWIReconciliationCommand{
				WorkspaceID: request.WorkspaceID, RunID: request.RunID, RequestID: request.RequestID,
				Generation: request.Generation, DefinitionRevision: request.DefinitionRevision,
				BindingSetHash: request.BindingSetHash, OwnerModuleID: owner, Binding: binding.Clone(),
				WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory,
				WorkspaceBaselineRevision: request.WorkspaceBaselineRevision, WorkspaceBaselineRef: request.WorkspaceBaselineRef,
				ValidationRecipes: append([]runtimecontract.RuntimeValidationRecipe(nil), validationRecipes...),
			}
			observation, receiptRef, err := queryRuntimeCWIReconciliationOwner(ctx, m.EventHub(), owner, command, request)
			if workspaceErr != nil {
				observation = unavailableRuntimeCWIReconciliationObservation(command, "workspace owner snapshot is unavailable", "workspace_snapshot_unavailable")
				receiptRef = reconciliationOwnerReceiptRef(owner, observation)
			} else if err != nil {
				observation = unavailableRuntimeCWIReconciliationObservation(command, "capability owner did not return a valid reconciliation observation", "owner_observation_unavailable")
				receiptRef = reconciliationOwnerReceiptRef(owner, observation)
			}
			batch.Observations = append(batch.Observations, observation)
			batch.OwnerReceiptRefs = append(batch.OwnerReceiptRefs, receiptRef)
		}
	}
	batch.Hash = runtimecontract.StableRuntimeCWIReconciliationBatchHash(batch)
	return batch
}

func queryRuntimeCWIReconciliationOwner(ctx context.Context, hub magicevent.Hub, owner string, command orchestratorevents.QueryRuntimeCWIReconciliationCommand, request runtimecontract.RuntimeCWIReconciliationRequest) (runtimecontract.RuntimeCWIReconciliationObservation, string, error) {
	if hub == nil || strings.TrimSpace(owner) == "" {
		return runtimecontract.RuntimeCWIReconciliationObservation{}, "", common.ErrInvalidArgument
	}
	response := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorQueryRuntimeCWIReconciliation,
		ModuleID, owner, magicevent.NewHeader(), ctx, command,
	))
	if response == nil {
		return runtimecontract.RuntimeCWIReconciliationObservation{}, "", common.ErrInvalidArgument
	}
	if magicErr := response.Error(); magicErr != nil {
		return runtimecontract.RuntimeCWIReconciliationObservation{}, "", magicErr
	}
	data, magicErr := response.Get()
	if magicErr != nil {
		return runtimecontract.RuntimeCWIReconciliationObservation{}, "", magicErr
	}
	value, ok := data.(orchestratorevents.QueryRuntimeCWIReconciliationResult)
	if !ok || strings.TrimSpace(value.OwnerReceiptRef) == "" {
		return runtimecontract.RuntimeCWIReconciliationObservation{}, "", fmt.Errorf("owner returned an invalid reconciliation result")
	}
	observation := value.Observation.Clone()
	if observation.OwnerModuleID != owner || observation.ValidateForRequest(request) != nil {
		return runtimecontract.RuntimeCWIReconciliationObservation{}, "", fmt.Errorf("owner returned an observation outside the sealed binding")
	}
	return observation, strings.TrimSpace(value.OwnerReceiptRef), nil
}

func unavailableRuntimeCWIReconciliationObservation(command orchestratorevents.QueryRuntimeCWIReconciliationCommand, message, code string) runtimecontract.RuntimeCWIReconciliationObservation {
	value := runtimecontract.RuntimeCWIReconciliationObservation{
		RequestID: command.RequestID, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		Generation: command.Generation, CoverageWorkItemID: command.Binding.CoverageWorkItemID,
		BindingID: command.Binding.BindingID, BindingHash: command.Binding.BindingHash,
		OwnerModuleID: command.OwnerModuleID, ObservedRevision: command.WorkspaceBaselineRevision,
		Status:     runtimecontract.RuntimeCWIReconciliationObservationUnverifiable,
		TargetRefs: append([]string(nil), command.Binding.TargetRefs...),
		OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
			SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: command.OwnerModuleID,
			OriginalClass: "reconciliation", OriginalCode: code, OriginalMessage: message,
			Signature: "owner-reconciliation/" + code, TargetRefs: append([]string(nil), command.Binding.TargetRefs...),
			ObservedRevision: command.WorkspaceBaselineRevision,
		},
	}
	return value.Normalized()
}

func reconciliationOwnerReceiptRef(owner string, observation runtimecontract.RuntimeCWIReconciliationObservation) string {
	return strings.TrimSpace(owner) + "/reconciliation/" + runtimecontract.StableRuntimeCWIReconciliationObservationRef(observation)
}

func runtimeReconciliationRequestForRun(run *runmodel.Run, aggregate runtime.RuntimeAggregate, requestedID string, requestedGeneration uint64) runtimecontract.RuntimeCWIReconciliationRequest {
	scope := run.ReconciliationScope
	if scope == "" {
		scope = runtimecontract.RuntimeCWIReconciliationScopeCurrentRun
	}
	requestID := strings.TrimSpace(requestedID)
	if requestID != "" {
		requestID = "resume/" + requestID
	}
	if requestID == "" {
		requestID = strings.TrimSpace(run.ActivationRequestID)
	}
	if requestID == "" {
		requestID = fmt.Sprintf("%s/%s", aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	}
	requestID = "runtime-reconciliation/" + requestID + "/" + string(scope)
	if requestedID != "" {
		for _, history := range aggregate.Reconciliation.History {
			if history.Request.RequestID == requestID {
				// Preserve the original generation for a duplicate continuation;
				// the caller will treat it as an already-sealed request.
				return history.Request.Clone()
			}
		}
	}
	baselineRevision, baselineRef := runtimeReconciliationBaseline(aggregate.Facts)
	generation := requestedGeneration
	if generation == 0 {
		generation = 1
		if aggregate.Reconciliation.Request != nil && aggregate.Reconciliation.Request.Generation >= generation {
			generation = aggregate.Reconciliation.Request.Generation + 1
		}
	}
	return runtimecontract.RuntimeCWIReconciliationRequest{
		RequestID: requestID, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Scope: scope, DefinitionRevision: aggregate.Facts.DefinitionRevision,
		BindingSetHash:            aggregate.Facts.ReconciliationContract.BindingSetHash,
		Bindings:                  aggregate.Facts.ReconciliationContract.Bindings,
		WorkspaceBaselineRevision: baselineRevision, WorkspaceBaselineRef: baselineRef,
		Generation: generation,
	}
}

func runtimeReconciliationBaseline(facts runtimecontract.RuntimeFacts) (string, string) {
	if facts.Resources.Manifest != nil && strings.TrimSpace(facts.Resources.Manifest.Revision) != "" {
		revision := strings.TrimSpace(facts.Resources.Manifest.Revision)
		return revision, "workspace-tree/" + revision
	}
	if facts.EvidenceRevision != "" {
		return facts.EvidenceRevision, "runtime-evidence/" + facts.EvidenceRevision
	}
	return facts.DefinitionRevision, "runtime-definition/" + facts.DefinitionRevision
}
