package mcp

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindToolCall = modulecommon.StepKindToolCall
const StepKindResourceRead = modulecommon.StepKindResourceRead
const StepKindPromptGet = modulecommon.StepKindPromptGet

type Biz struct {
	basebiz.Base
	runtimeAdapter *RuntimeAdapter
}

type Dependencies struct {
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
	// RuntimeAdapter is an explicit owner-local dependency for focused
	// runtimes and tests. Production leaves it nil so the typed MCP registry
	// remains the sole adapter construction source.
	RuntimeAdapter *RuntimeAdapter
}

func New(eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine) (*Biz, error) {
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{
		Base:           basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine),
		runtimeAdapter: NewRuntimeAdapter(mcpEventServerGetter{hub: eventHub}),
	}, nil
}

func NewWithDependencies(deps Dependencies) (*Biz, error) {
	owner, err := New(deps.EventHub, deps.BackgroundRoutine)
	if err != nil {
		return nil, err
	}
	if deps.RuntimeAdapter != nil {
		owner.runtimeAdapter = deps.RuntimeAdapter
	}
	return owner, nil
}

func (m *Biz) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorProbeRuntimeSemanticProofFeasibility, m.handleRuntimeSemanticProofFeasibility)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorQueryRuntimeCWIReconciliation, m.handleRuntimeCWIReconciliationQuery)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorExecuteMCPOperation, m.handleMCPOperation)
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorExecuteMCPOperation)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorProbeRuntimeSemanticProofFeasibility)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorQueryRuntimeCWIReconciliation)
	if m.runtimeAdapter != nil {
		m.runtimeAdapter.Close()
	}
	m.runtimeAdapter = nil
}

// handleRuntimeCWIReconciliationQuery reports MCP readiness separately from
// behavior proof. Catalog/endpoint/tool-surface readiness is useful diagnostic
// context, but it is never a satisfied CWI. A behavior proof requires a
// binding-declared executable probe (including its exact input contract),
// which this reconciliation command intentionally does not invent.
func (m *Biz) handleRuntimeCWIReconciliationQuery(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.QueryRuntimeCWIReconciliationCommand)
	if !ok || m == nil || m.runtimeAdapter == nil || command.ValidateForOwner(ModuleID) != nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if !runtimeCWIReconciliationBindingHasMCPProof(command.Binding) {
		observation := mcpReconciliationObservation(command, runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, command.WorkspaceBaselineRevision, nil, nil, "binding has no MCP behavior recipe", "behavior_probe_unavailable")
		result.Set(orchestratorevents.QueryRuntimeCWIReconciliationResult{Observation: observation, OwnerReceiptRef: mcpReconciliationReceiptRef(observation)}, nil)
		return
	}
	serverIDs, _, revision, err := m.runtimeAdapter.OperationAdmissibility(event.Context())
	if err != nil {
		observation := mcpReconciliationObservation(command, runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, command.WorkspaceBaselineRevision, nil, nil, "MCP server catalog is unavailable", "mcp_catalog_unavailable")
		result.Set(orchestratorevents.QueryRuntimeCWIReconciliationResult{Observation: observation, OwnerReceiptRef: mcpReconciliationReceiptRef(observation)}, nil)
		return
	}
	serverSet := make(map[string]struct{}, len(serverIDs))
	for _, id := range serverIDs {
		serverSet[strings.TrimSpace(id)] = struct{}{}
	}
	var readyTargets []string
	var readyChecks []string
	allSurfaceReady := true
	for _, recipeID := range command.Binding.ValidationRecipeIDs {
		serverID, tool, parseErr := parseMCPReconciliationRecipe(recipeID)
		if parseErr != nil {
			allSurfaceReady = false
			continue
		}
		if _, found := serverSet[serverID]; !found {
			allSurfaceReady = false
			continue
		}
		probeErr := m.runtimeAdapter.ProbeOperation(event.Context(), RuntimeOperationRequest{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID,
			StepID:                        common.StepID("reconciliation/" + command.RequestID + "/" + command.Binding.BindingID + "/" + recipeID),
			Kind:                          runtimecontract.StepKindMCPToolCall,
			WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
			TargetRefs:                    command.Binding.TargetRefs,
			Data:                          orchestratorevents.MCPOperationData{ServerID: serverID, Tool: tool},
		})
		if probeErr != nil {
			allSurfaceReady = false
			continue
		}
		readyTargets = append(readyTargets, serverID+"/"+tool)
		readyChecks = append(readyChecks, recipeID)
	}
	status := runtimecontract.RuntimeCWIReconciliationObservationUnverifiable
	message, code := "MCP endpoint/tool surface is ready but no behavior proof was executed", "behavior_probe_unavailable"
	if !allSurfaceReady {
		message, code = "MCP binding tool surface is unavailable", "mcp_tool_surface_unavailable"
	}
	// Preserve the readiness facts in the observation while keeping Status
	// unverifiable. Orchestrator may use them for prerequisite diagnostics but
	// can never convert them into CWI credit.
	observation := mcpReconciliationObservation(command, status, revision, readyTargets, readyChecks, message, code)
	result.Set(orchestratorevents.QueryRuntimeCWIReconciliationResult{Observation: observation, OwnerReceiptRef: mcpReconciliationReceiptRef(observation)}, nil)
}

func runtimeCWIReconciliationBindingHasMCPProof(binding runtimecontract.RuntimeCWIReconciliationBindingRef) bool {
	for _, kind := range binding.AllowedOperationKinds {
		switch runtimecontract.CanonicalRuntimeBlockKind(kind) {
		case runtimecontract.StepKindMCPToolCall, runtimecontract.StepKindMCPResourceRead, runtimecontract.StepKindMCPPromptGet:
			return true
		}
	}
	return len(binding.ValidationRecipeIDs) != 0
}

func parseMCPReconciliationRecipe(recipeID string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(recipeID), "/", 4)
	if len(parts) != 4 || parts[0] != "mcp" || parts[1] != "tool_call" || parts[2] == "" || parts[3] == "" {
		return "", "", fmt.Errorf("MCP reconciliation recipe %q is not an executable tool probe", recipeID)
	}
	return parts[2], parts[3], nil
}

func mcpReconciliationObservation(command orchestratorevents.QueryRuntimeCWIReconciliationCommand, status runtimecontract.RuntimeCWIReconciliationObservationStatus, revision string, targetRefs, checkRefs []string, message, code string) runtimecontract.RuntimeCWIReconciliationObservation {
	if strings.TrimSpace(revision) == "" {
		revision = command.WorkspaceBaselineRevision
	}
	value := runtimecontract.RuntimeCWIReconciliationObservation{
		RequestID: command.RequestID, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		Generation: command.Generation, CoverageWorkItemID: command.Binding.CoverageWorkItemID,
		BindingID: command.Binding.BindingID, BindingHash: command.Binding.BindingHash,
		OwnerModuleID: ModuleID, ObservedRevision: revision, Status: status,
		TargetRefs: append([]string(nil), targetRefs...), CheckRefs: append([]string(nil), checkRefs...),
	}
	if status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
		value.OriginalDiagnostic = &runtimecontract.RuntimeOriginalDiagnostic{
			SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: ModuleID,
			OriginalClass: "reconciliation", OriginalCode: code, OriginalMessage: message,
			Signature: "mcp-reconciliation/" + code, TargetRefs: append([]string(nil), targetRefs...), ObservedRevision: revision,
		}
	}
	return value.Normalized()
}

func mcpReconciliationReceiptRef(observation runtimecontract.RuntimeCWIReconciliationObservation) string {
	return "mcp-reconciliation/" + runtimecontract.StableRuntimeCWIReconciliationObservationRef(observation)
}

func (m *Biz) handleMCPOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteMCPOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setCapabilityError(result, err)
		return
	}
	if receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
		setCapabilityError(result, err)
		return
	} else if found {
		result.Set(orchestratorevents.ExecuteMCPOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ExecuteOperation(event.Context(), RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID(command.DispatchID + "/" + command.OperationID),
		Kind: command.OperationKind, WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory: command.WorkspaceInternalDirectory, TargetRefs: command.TargetRefs,
		TargetRole: command.TargetRole,
		Data:       command.Data, EvidencePolicies: command.EvidencePolicies,
	})
	status := runtimecontract.RuntimeOperationExecutionStatusAccepted
	evidence := operationResult.Evidence
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
		failure = orchestratorevents.RuntimeOperationFailureWithTargetObservation(command.Transaction, command.OperationKind, operationResult.FailureCode, operationResult.FailureMessage, mcpFailureTargets(operationResult), mcpFailureTargetObservation(operationResult))
	}
	observation := orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectUnknown}
	if status == runtimecontract.RuntimeOperationExecutionStatusAccepted {
		observation.ObservedEffectScope = command.Execution.ExpectedEffectScope
		observation.Changed = runtimecontract.RuntimeEffectIsDurable(command.Execution.ExpectedEffectScope)
	}
	attachMCPToolValidationObservation(&evidence, status, command.OperationKind, command.EvidenceKind, command.Data.ServerID, command.Data.Tool)
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, operationResult.TargetRefs, evidence, failure, nil, observation)
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(orchestratorevents.ExecuteMCPOperationResult{Receipt: committed, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID)}, nil)
}

// mcpFailureTargets preserves only the bounded failure surface actually
// observed by the MCP owner. An operation target identifies what was queried;
// without an explicit owner observation there is no related failure target.
func mcpFailureTargets(result RuntimeOperationResult) []string {
	return normalizeMCPTargetRefs(result.Diagnostics.FailureTargets)
}

func mcpFailureTargetObservation(result RuntimeOperationResult) *runtimecontract.RuntimeFailureTargetObservation {
	targets := mcpFailureTargets(result)
	observation := runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: ModuleID, SubjectRef: result.ServerID, TargetRole: result.Diagnostics.TargetRole,
		ObservedRevision: result.OwnerRevision, TargetRefs: targets,
	}.Normalized()
	if err := observation.Validate(); err != nil {
		return nil
	}
	return &observation
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, modulecommon.RuntimeCapabilityKinds()...)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func (m *Biz) handleRuntimeOperationAdmissibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	serverIDs, recipeRefs, revision, err := m.runtimeAdapter.OperationAdmissibility(event.Context())
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory, err := orchestratorevents.NewRuntimeOperationAdmissibility(command.PrerequisiteID, ModuleID, command.RequiredKinds, nil, serverIDs, []string{"configured-mcp-server", "owner-execution-policy"}, revision)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory.ValidationRecipeRefs = recipeRefs
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		setCapabilityError(result, err)
		return
	}
	observation := mcpAdmissibilityObservation(command.PrerequisiteID, inventory)
	result.Set(orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{PrerequisiteID: command.PrerequisiteID, Admissibility: inventory, Observation: &observation}, nil)
}

func setRuntimeOperationFeasibilityResult(result magicevent.Result, command orchestratorevents.ProbeRuntimeOperationFeasibilityCommand, failure *runtimecontract.FailureFact, stale *runtimecontract.RuntimePrerequisiteStale) {
	category := ownerFeasibilityCategory(failure)
	observation, err := orchestratorevents.NewProbeRuntimeOperationFeasibilityResult(command, failure == nil && stale == nil, category, failure, stale, nil, nil)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(observation, nil)
}

func (m *Biz) handleRuntimeOperationFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ProbeRuntimeOperationFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	operation := command.Operation.Normalized()
	serverIDs, _, revision, revisionErr := m.runtimeAdapter.OperationAdmissibility(event.Context())
	if revisionErr != nil {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "mcp_server_catalog_unavailable", revisionErr.Error())
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	if revision != command.Admissibility.ObservedRevision {
		stale := mcpAdmissibilityStale(command.Observation, command.Admissibility, revision)
		if stale != nil {
			setRuntimeOperationFeasibilityResult(result, command, nil, stale)
			return
		}
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_stale", "runtime mcp operation admissibility snapshot is stale")
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	data, err := orchestratorevents.MCPOperationDataFromRuntimeOperation(operation)
	if err != nil {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "mcp_operation_invalid", err.Error())
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	if !containsMCPAdmissibleServer(serverIDs, data.ServerID) {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "mcp_server_not_admissible", "runtime mcp operation server is absent from the owner admissibility inventory")
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	err = m.runtimeAdapter.ProbeOperation(event.Context(), RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID("probe/" + operation.ID), Kind: operation.Kind,
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory, TargetRefs: operation.TargetRefs, Data: data,
	})
	if err != nil {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "mcp_prerequisite_unavailable", err.Error())
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	setRuntimeOperationFeasibilityResult(result, command, nil, nil)
}

func (m *Biz) handleRuntimeSemanticProofFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ProbeRuntimeSemanticProofFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	serverIDs, recipeRefs, revision, err := m.runtimeAdapter.OperationAdmissibility(event.Context())
	if err != nil {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired, "", nil,
			orchestratorevents.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_catalog_unavailable", err.Error(), command.OwnerCatalog.ObservedRevision, "", nil))
		return
	}
	if revision != command.OwnerCatalog.ObservedRevision {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityStale, revision, nil,
			orchestratorevents.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "semantic_catalog_stale", "runtime MCP semantic catalog is stale", command.OwnerCatalog.ObservedRevision, revision, nil))
		return
	}
	if !containsMCPAdmissibleServer(recipeRefs, command.Proposal.ValidationRecipeID) {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			orchestratorevents.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_recipe_not_admissible", "runtime semantic MCP validation recipe is absent from the owner catalog", "", revision, nil))
		return
	}
	for _, selector := range command.Proposal.Target.OwnerSelectorRefs {
		if !containsMCPAdmissibleServer(serverIDs, selector) {
			setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
				orchestratorevents.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_selector_not_admissible", "runtime semantic MCP selector is absent from the owner catalog", "", revision, []string{selector}))
			return
		}
	}
	setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityFeasible, revision, orchestratorevents.RuntimeSemanticProposalTargetRefs(command.Proposal), nil)
}

func setRuntimeSemanticProofFeasibilityResult(result magicevent.Result, command orchestratorevents.ProbeRuntimeSemanticProofFeasibilityCommand, status runtimecontract.RuntimeSemanticFeasibilityStatus, revision string, targetRefs []string, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) {
	response, err := orchestratorevents.NewRuntimeSemanticProofFeasibilityResult(command, status, revision, targetRefs, diagnostic)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func mcpAdmissibilityObservation(prerequisiteID string, inventory runtimecontract.RuntimeOperationAdmissibility) runtimecontract.RuntimePrerequisiteObservation {
	observation := runtimecontract.RuntimePrerequisiteObservation{PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: ModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: ModuleID, SubjectRef: "mcp_server_catalog", Revision: inventory.ObservedRevision}}}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func mcpAdmissibilityStale(observation *runtimecontract.RuntimePrerequisiteObservation, inventory runtimecontract.RuntimeOperationAdmissibility, observedRevision string) *runtimecontract.RuntimePrerequisiteStale {
	if observation == nil {
		return nil
	}
	value := observation.Normalized()
	for _, dependency := range value.Dependencies {
		if dependency.Domain == runtimecontract.RuntimeResourceOwnerCatalog && dependency.OwnerModuleID == ModuleID && dependency.SubjectRef == "mcp_server_catalog" && dependency.Revision == inventory.ObservedRevision {
			return &runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: value.PrerequisiteID, ObservationID: value.ObservationID, Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, ExpectedRevision: dependency.Revision, ObservedRevision: observedRevision, Reason: "runtime mcp server catalog changed"}
		}
	}
	return nil
}

func containsMCPAdmissibleServer(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
}

type mcpEventServerGetter struct {
	hub magicevent.Hub
}

func (g mcpEventServerGetter) ListServers(ctx context.Context) ([]*mcpmodel.Server, error) {
	if g.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(
		mcpevents.TopicMCPCommandListServers,
		ModuleID,
		mcpevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		mcpevents.ListServersCommand{},
	)
	result := g.hub.Send(ev)
	if result == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return nil, err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return nil, resultErr
	}
	response, ok := data.(mcpevents.MCPCommandResult)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return append([]*mcpmodel.Server(nil), response.Servers...), nil
}

func (g mcpEventServerGetter) GetServer(ctx context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if g.hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(
		mcpevents.TopicMCPCommandGetServer,
		ModuleID,
		mcpevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		mcpevents.GetServerCommand{ServerID: id},
	)
	result := g.hub.Send(ev)
	if result == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return nil, err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return nil, resultErr
	}
	response, ok := data.(mcpevents.MCPCommandResult)
	if !ok || response.Server == nil {
		return nil, common.ErrNotFound
	}
	return response.Server, nil
}
