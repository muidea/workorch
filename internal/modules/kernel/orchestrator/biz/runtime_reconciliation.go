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
	desired := runtimeReconciliationRequestForRun(run, aggregate, command.ReconciliationRequestID, command.ReconciliationGeneration)
	desiredBase, _ := runtimeReconciliationRequestBaseIdentityForRun(run, aggregate, command.ReconciliationRequestID)
	request := desired
	sealRequest := aggregate.Reconciliation.Request == nil
	if aggregate.Reconciliation.Request != nil {
		current := aggregate.Reconciliation.Request.Normalized()
		if current.ValidateAgainstContract(aggregate.Facts.ReconciliationContract) != nil {
			// A stale active row must be retired by the Runtime owner before any
			// owner query.  It cannot suppress the request derived from the
			// current SM-2 binding projection.
			sealRequest = true
		} else if strings.TrimSpace(command.ReconciliationRequestID) == "" || current.RequestID == desired.RequestID {
			request = current
		} else if runtimeReconciliationHistoryContainsIntent(aggregate, desiredBase) {
			// The continuation was already consumed and later superseded. Keep
			// driving the current generation rather than reviving historical
			// bindings merely because an old notification was redelivered.
			request = current
		} else {
			sealRequest = true
		}
	}
	if sealRequest {
		result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind: runtime.RuntimeInternalEventReconciliationRequested, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{ReconciliationRequest: &desired},
		})
		return result.Aggregate, true, err
	}
	if aggregate.Reconciliation.Batch != nil && !reconciliationRequestNeedsRefresh(run, aggregate, command) {
		return aggregate, false, nil
	}
	var deliverableDirectory, internalDirectory string
	workspace, workspaceErr := (workspaceSnapshotReader{hub: m.EventHub()}).Read(ctx, aggregate.Facts.WorkspaceID)
	if workspaceErr == nil && workspace != nil {
		deliverableDirectory = strings.TrimSpace(workspace.DeliverableDirectory)
		internalDirectory = strings.TrimSpace(workspace.InternalDirectory)
	}
	batch := m.queryRuntimeReconciliationBatch(ctx, request, aggregate, deliverableDirectory, internalDirectory, workspaceErr)
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventReconciliationBatchObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{ReconciliationBatch: &batch},
	})
	return result.Aggregate, true, err
}

func reconciliationRequestNeedsRefresh(run *runmodel.Run, aggregate runtime.RuntimeAggregate, command orchestratorevents.DriveRuntimeCommand) bool {
	if run == nil || aggregate.Reconciliation.Request == nil {
		return true
	}
	current := aggregate.Reconciliation.Request.Normalized()
	if current.ValidateAgainstContract(aggregate.Facts.ReconciliationContract) != nil {
		return true
	}
	if strings.TrimSpace(command.ReconciliationRequestID) == "" {
		return false
	}
	desired := runtimeReconciliationRequestForRun(run, aggregate, command.ReconciliationRequestID, command.ReconciliationGeneration)
	if current.RequestID == desired.RequestID {
		return false
	}
	desiredBase, _ := runtimeReconciliationRequestBaseIdentityForRun(run, aggregate, command.ReconciliationRequestID)
	return !runtimeReconciliationHistoryContainsIntent(aggregate, desiredBase)
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
	baseID, scope := runtimeReconciliationRequestBaseIdentityForRun(run, aggregate, requestedID)
	contract := aggregate.Facts.ReconciliationContract.Normalized()
	if aggregate.Reconciliation.Request != nil {
		current := aggregate.Reconciliation.Request.Normalized()
		if runtimeReconciliationRequestBaseIdentity(current.RequestID) == baseID && current.ValidateAgainstContract(contract) == nil &&
			(requestedGeneration == 0 || current.Generation == requestedGeneration) {
			return current
		}
	}
	baselineRevision, baselineRef := runtimeReconciliationBaseline(aggregate.Facts)
	generation := runtimeReconciliationGenerationHighWatermark(aggregate) + 1
	if requestedGeneration > generation {
		generation = requestedGeneration
	}
	return runtimecontract.RuntimeCWIReconciliationRequest{
		RequestID: runtimeReconciliationRequestIdentityForGeneration(baseID, generation), WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Scope: scope, DefinitionRevision: aggregate.Facts.DefinitionRevision,
		BindingSetHash:            contract.BindingSetHash,
		Bindings:                  contract.Bindings,
		WorkspaceBaselineRevision: baselineRevision, WorkspaceBaselineRef: baselineRef,
		Generation: generation,
	}
}

// runtimeReconciliationRequestBaseIdentityForRun derives the stable portion
// of a durable SM-4 request identity from the Run-owned continuation intent
// and current SM-2 binding set. The sealed request adds a monotonic generation
// suffix below, so history can never be reactivated when a binding set happens
// to recur.
func runtimeReconciliationRequestBaseIdentityForRun(run *runmodel.Run, aggregate runtime.RuntimeAggregate, requestedID string) (string, runtimecontract.RuntimeCWIReconciliationScope) {
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
	bindingSetHash := strings.TrimSpace(aggregate.Facts.ReconciliationContract.BindingSetHash)
	return "runtime-reconciliation/" + requestID + "/" + string(scope) + "/binding-set/" + bindingSetHash, scope
}

func runtimeReconciliationRequestIdentityForGeneration(baseID string, generation uint64) string {
	return strings.TrimRight(strings.TrimSpace(baseID), "/") + fmt.Sprintf("/generation/%d", generation)
}

func runtimeReconciliationRequestBaseIdentity(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	marker := "/generation/"
	index := strings.LastIndex(requestID, marker)
	if index < 0 {
		return requestID
	}
	return requestID[:index]
}

func runtimeReconciliationGenerationHighWatermark(aggregate runtime.RuntimeAggregate) uint64 {
	highest := uint64(0)
	if aggregate.Reconciliation.Request != nil && aggregate.Reconciliation.Request.Generation > highest {
		highest = aggregate.Reconciliation.Request.Generation
	}
	for _, history := range aggregate.Reconciliation.History {
		if history.Request.Generation > highest {
			highest = history.Request.Generation
		}
	}
	return highest
}

func runtimeReconciliationHistoryContainsIntent(aggregate runtime.RuntimeAggregate, baseID string) bool {
	intentID := runtimeReconciliationRequestIntentIdentity(baseID)
	if intentID == "" {
		return false
	}
	for _, history := range aggregate.Reconciliation.History {
		candidate := history.Request.Normalized()
		if runtimeReconciliationRequestIntentIdentity(candidate.RequestID) == intentID {
			return true
		}
	}
	return false
}

func runtimeReconciliationRequestIntentIdentity(requestID string) string {
	baseID := runtimeReconciliationRequestBaseIdentity(requestID)
	marker := "/binding-set/"
	if index := strings.LastIndex(baseID, marker); index >= 0 {
		return baseID[:index]
	}
	return baseID
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
