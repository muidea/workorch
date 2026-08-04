package orchestrator

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
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
			command := runtimecontract.RuntimeCWIOwnerReconciliationRequest{
				WorkspaceID: request.WorkspaceID, RunID: request.RunID, RequestID: request.RequestID,
				Generation: request.Generation, DefinitionRevision: request.DefinitionRevision,
				BindingSetHash: request.BindingSetHash, OwnerModuleID: owner, Binding: binding.Clone(),
				WorkspaceDeliverableDirectory: deliverableDirectory, WorkspaceInternalDirectory: internalDirectory,
				WorkspaceBaselineRevision: request.WorkspaceBaselineRevision, WorkspaceBaselineRef: request.WorkspaceBaselineRef,
				ValidationRecipes: append([]runtimecontract.RuntimeValidationRecipe(nil), validationRecipes...),
			}
			observation, err := queryRuntimeCWIReconciliationOwner(ctx, m.EventHub(), owner, command, request)
			if workspaceErr != nil {
				observation = unavailableRuntimeCWIReconciliationObservation(command, "workspace owner snapshot is unavailable", "workspace_snapshot_unavailable")
			} else if err != nil {
				observation = unavailableRuntimeCWIReconciliationObservation(command, "capability owner did not return a valid reconciliation observation", "owner_observation_unavailable")
			}
			batch.Observations = append(batch.Observations, observation)
		}
	}
	batch.Hash = runtimecontract.StableRuntimeCWIReconciliationBatchHash(batch)
	return batch
}

func queryRuntimeCWIReconciliationOwner(ctx context.Context, hub magicevent.Hub, owner string, command runtimecontract.RuntimeCWIOwnerReconciliationRequest, request runtimecontract.RuntimeCWIReconciliationRequest) (runtimecontract.RuntimeCWIReconciliationObservation, error) {
	if hub == nil || strings.TrimSpace(owner) == "" {
		return runtimecontract.RuntimeCWIReconciliationObservation{}, common.ErrInvalidArgument
	}
	response := sendRuntimeCWIReconciliationOwnerQuery(ctx, hub, owner, command)
	if response == nil {
		return runtimecontract.RuntimeCWIReconciliationObservation{}, common.ErrInvalidArgument
	}
	if magicErr := response.Error(); magicErr != nil {
		return runtimecontract.RuntimeCWIReconciliationObservation{}, magicErr
	}
	data, magicErr := response.Get()
	if magicErr != nil {
		return runtimecontract.RuntimeCWIReconciliationObservation{}, magicErr
	}
	value, ok := normalizeRuntimeCWIReconciliationOwnerResult(owner, data)
	if !ok {
		return runtimecontract.RuntimeCWIReconciliationObservation{}, fmt.Errorf("owner returned an invalid reconciliation result")
	}
	observation := value.Clone()
	if observation.OwnerModuleID != owner || observation.ValidateForRequest(request) != nil {
		return runtimecontract.RuntimeCWIReconciliationObservation{}, fmt.Errorf("owner returned an observation outside the sealed binding")
	}
	return observation, nil
}

func unavailableRuntimeCWIReconciliationObservation(command runtimecontract.RuntimeCWIOwnerReconciliationRequest, message, code string) runtimecontract.RuntimeCWIReconciliationObservation {
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

func sendRuntimeCWIReconciliationOwnerQuery(ctx context.Context, hub magicevent.Hub, owner string, c runtimecontract.RuntimeCWIOwnerReconciliationRequest) magicevent.Result {
	switch owner {
	case runtimecontract.RuntimeModuleFile:
		return hub.Send(magicevent.NewEventWithContext(fileevents.TopicQueryRuntimeCWIReconciliation, ModuleID, owner, magicevent.NewHeader(), ctx, fileRuntimeCWIReconciliationCommand(c)))
	case runtimecontract.RuntimeModuleGit:
		return hub.Send(magicevent.NewEventWithContext(gitevents.TopicQueryRuntimeCWIReconciliation, ModuleID, owner, magicevent.NewHeader(), ctx, gitRuntimeCWIReconciliationCommand(c)))
	case runtimecontract.RuntimeModuleMCP:
		return hub.Send(magicevent.NewEventWithContext(mcpevents.TopicQueryRuntimeCWIReconciliation, ModuleID, owner, magicevent.NewHeader(), ctx, mcpRuntimeCWIReconciliationCommand(c)))
	case runtimecontract.RuntimeModuleShell:
		return hub.Send(magicevent.NewEventWithContext(shellevents.TopicQueryRuntimeCWIReconciliation, ModuleID, owner, magicevent.NewHeader(), ctx, shellRuntimeCWIReconciliationCommand(c)))
	default:
		return nil
	}
}

func fileRuntimeCWIReconciliationCommand(c runtimecontract.RuntimeCWIOwnerReconciliationRequest) fileevents.QueryRuntimeCWIReconciliationCommand {
	return fileevents.QueryRuntimeCWIReconciliationCommand(c)
}
func gitRuntimeCWIReconciliationCommand(c runtimecontract.RuntimeCWIOwnerReconciliationRequest) gitevents.QueryRuntimeCWIReconciliationCommand {
	return gitevents.QueryRuntimeCWIReconciliationCommand(c)
}
func mcpRuntimeCWIReconciliationCommand(c runtimecontract.RuntimeCWIOwnerReconciliationRequest) mcpevents.QueryRuntimeCWIReconciliationCommand {
	return mcpevents.QueryRuntimeCWIReconciliationCommand(c)
}
func shellRuntimeCWIReconciliationCommand(c runtimecontract.RuntimeCWIOwnerReconciliationRequest) shellevents.QueryRuntimeCWIReconciliationCommand {
	return shellevents.QueryRuntimeCWIReconciliationCommand(c)
}

func normalizeRuntimeCWIReconciliationOwnerResult(owner string, data any) (runtimecontract.RuntimeCWIReconciliationObservation, bool) {
	switch owner {
	case runtimecontract.RuntimeModuleFile:
		v, ok := data.(fileevents.QueryRuntimeCWIReconciliationResult)
		return v.Observation, ok
	case runtimecontract.RuntimeModuleGit:
		v, ok := data.(gitevents.QueryRuntimeCWIReconciliationResult)
		return v.Observation, ok
	case runtimecontract.RuntimeModuleMCP:
		v, ok := data.(mcpevents.QueryRuntimeCWIReconciliationResult)
		return v.Observation, ok
	case runtimecontract.RuntimeModuleShell:
		v, ok := data.(shellevents.QueryRuntimeCWIReconciliationResult)
		return v.Observation, ok
	default:
		return runtimecontract.RuntimeCWIReconciliationObservation{}, false
	}
}

func runtimeReconciliationRequestForRun(run *runmodel.Run, aggregate runtime.RuntimeAggregate, requestedID string, requestedGeneration uint64) runtimecontract.RuntimeCWIReconciliationRequest {
	scope := run.ReconciliationScope
	if scope == "" {
		scope = runtimecontract.RuntimeCWIReconciliationScopeCurrentRun
	}
	requestID := strings.TrimSpace(requestedID)
	if requestID != "" {
		// A continuation always reconciles the already-sealed current Run.
		// ReconciliationScope on the Run records how its activation was admitted;
		// a replacement therefore remains workspace_full for audit, but that
		// historical activation fact must not widen a later /run resume.
		scope = runtimecontract.RuntimeCWIReconciliationScopeCurrentRun
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
